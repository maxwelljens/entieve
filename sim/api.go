package sim

import (
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// Sim is the deterministic Entieve simulation.
//
// The exported stores are the render/UI read surface; mutate game state only
// through Queue(Command) so determinism and replays hold.
//
// Determinism contract: Step advances a fixed DT (1/60 s); every random draw
// goes through RNG (player/master stream) or a per-NPC Fork; systems iterate
// entities in canonical handle order (never map order); Hash fingerprints the
// whole state. New(c, w, seed) plus the same command sequence yields an
// identical Hash at every checkpoint.
type Sim struct {
	Content *content.Content
	World   *ecs.World
	RNG     *core.RNG
	Hangar  *Hangar

	// Component stores (read surface).
	Names      *ecs.Store[Name]
	Transforms *ecs.Store[Transform]
	Intents    *ecs.Store[Intent]
	Motions    *ecs.Store[Motion]
	Ships      *ecs.Store[ShipState]
	Caps       *ecs.Store[Capacitor]
	Cargos     *ecs.Store[Cargo]
	Fittings   *ecs.Store[Fitting]
	Targetings *ecs.Store[Targeting]
	Pilots     *ecs.Store[Pilot]
	AIs        *ecs.Store[NpcAI]
	Asteroids  *ecs.Store[Asteroid]
	Celestials *ecs.Store[Celestial]
	Gates      *ecs.Store[Stargate]
	Stations   *ecs.Store[Station]
	Docked     *ecs.Store[Docked]
	Warps      *ecs.Store[WarpDrive]
	Wrecks     *ecs.Store[Wreck]

	// pendingPlayerRespawn defers the death respawn to the end of a Step.
	pendingPlayerRespawn bool

	// Unowned simulation state (serialized through Snapshot's Save fields:
	// ModTargets, FreeSlots and LastStation).
	worldID     content.WorldID
	systemID    content.SystemID
	seed        int64
	ticks       int64
	time        float64
	wallet      float64
	player      ecs.Entity
	lastStation content.CelID
	queue       []Command
	events      []Event
	market      map[content.SystemID][]MarketStock
	respawns    []RespawnTimer
	modTargets  map[modKey]ecs.Entity
	free        []freeSlot

	// Per-tick working state (never snapshotted; always empty between Steps).
	deferLoot   []CmdLoot
	deferEcon   []Command
	deferTravel []Command
	damage      []pendingDamage
}

// freeSlot mirrors one entry of the world's free list (slot index + the
// generation of its most recent occupant) so Restore can reproduce exact
// entity handles.
type freeSlot struct {
	idx uint32
	gen uint32
}

// New builds a simulation from loaded content and a world scenario, placing
// the player at the world's start and populating the starting system.
func New(c *content.Content, w *content.WorldDef, seed int64) (*Sim, error) {
	if c == nil {
		return nil, errf("sim: nil content")
	}
	if w == nil {
		return nil, errf("sim: nil world")
	}
	if _, ok := c.System(w.Start.System); !ok {
		return nil, errf("sim: world %s references unknown system %s", w.ID, w.Start.System)
	}
	if _, ok := c.Ship(w.Start.Ship); !ok {
		return nil, errf("sim: world %s starts with unknown ship %s", w.ID, w.Start.Ship)
	}

	s := &Sim{
		Content:    c,
		World:      ecs.New(),
		RNG:        core.NewRNG(seed),
		Hangar:     &Hangar{},
		seed:       seed,
		worldID:    w.ID,
		systemID:   w.Start.System,
		wallet:     w.Start.Wallet,
		modTargets: make(map[modKey]ecs.Entity),
		market:     initMarket(c),
	}
	s.bindStores()
	if err := s.buildPlayer(w); err != nil {
		return nil, err
	}
	if err := s.LoadSystem(w.Start.System, ""); err != nil {
		return nil, err
	}
	return s, nil
}

// bindStores (re)creates every component store against the current world.
// Restore uses it to start from a fresh world.
func (s *Sim) bindStores() {
	w := s.World
	s.Names = ecs.NewStore[Name](w)
	s.Transforms = ecs.NewStore[Transform](w)
	s.Intents = ecs.NewStore[Intent](w)
	s.Motions = ecs.NewStore[Motion](w)
	s.Ships = ecs.NewStore[ShipState](w)
	s.Caps = ecs.NewStore[Capacitor](w)
	s.Cargos = ecs.NewStore[Cargo](w)
	s.Fittings = ecs.NewStore[Fitting](w)
	s.Targetings = ecs.NewStore[Targeting](w)
	s.Pilots = ecs.NewStore[Pilot](w)
	s.AIs = ecs.NewStore[NpcAI](w)
	s.Asteroids = ecs.NewStore[Asteroid](w)
	s.Celestials = ecs.NewStore[Celestial](w)
	s.Gates = ecs.NewStore[Stargate](w)
	s.Stations = ecs.NewStore[Station](w)
	s.Docked = ecs.NewStore[Docked](w)
	s.Warps = ecs.NewStore[WarpDrive](w)
	s.Wrecks = ecs.NewStore[Wreck](w)
}

// Step advances the simulation by one tick (DT seconds). Queued commands are
// consumed first, then systems run in documented order (docs/ARCHITECTURE.md).
func (s *Sim) Step() {
	s.runCommands()     // 1  command
	s.systemMovement()  // 2  movement
	s.systemWarp()      // 3  warp
	s.systemTargeting() // 4  targeting
	s.systemModules()   // 5  modules
	s.systemWeapons()   // 6  weapons
	s.systemDamage()    // 7  damage
	s.systemCap()       // 8  capacitor
	s.systemMining()    // 9  mining
	s.systemNpc()       // 10 npc
	s.systemLoot()      // 11 loot
	s.systemEconomy()   // 12 economy
	s.systemTravel()    // 13 travel
	s.systemRespawn()   // 14 respawn
	// Death respawn runs after all systems so that a cross-system clone-bay
	// respawn can rebuild the grid without yanking entities out from under
	// the systems still running this tick.
	if s.pendingPlayerRespawn {
		s.pendingPlayerRespawn = false
		s.respawnPlayer()
	}
	// 15 events: capped at emit time; nothing else to trim.
	s.ticks++
	// Time is derived from ticks (not accumulated) so that Time() is an
	// exact function of the tick count — trivially identical across
	// snapshot/restore round-trips.
	s.time = float64(s.ticks) * DT
}

// Queue enqueues a player/agent command for the next Step.
func (s *Sim) Queue(cmd Command) {
	if cmd == nil {
		return
	}
	s.queue = append(s.queue, cmd)
}

// Events drains and returns all pending events.
func (s *Sim) Events() []Event {
	ev := s.events
	s.events = nil
	return ev
}

// DrainEvents is an alias of Events (docs/ARCHITECTURE.md names both).
func (s *Sim) DrainEvents() []Event { return s.Events() }

// Time returns simulated seconds since world start.
func (s *Sim) Time() float64 { return s.time }

// Ticks returns the number of executed Steps.
func (s *Sim) Ticks() int64 { return s.ticks }

// Hash returns a deterministic fingerprint of full simulation state.
func (s *Sim) Hash() uint64 { return s.hash() }

// Player returns the player entity (zero if none).
func (s *Sim) Player() ecs.Entity { return s.player }

// System returns the current system definition.
func (s *Sim) System() *content.SystemDef {
	sys, _ := s.Content.System(s.systemID)
	return sys
}

// Wallet returns the player's credit balance.
func (s *Sim) Wallet() float64 { return s.wallet }

// LoadSystem (re)builds the grid for the given system, placing the player
// near the given celestial (gate arrivals, undock, world start). An empty
// entryAtCel keeps the player's current position (world start).
func (s *Sim) LoadSystem(id content.SystemID, entryAtCel content.CelID) error {
	return s.loadSystem(id, entryAtCel)
}

// Grid returns Info for every on-grid entity (docked entities excluded),
// deterministic order.
func (s *Sim) Grid() []Info {
	seen := make(map[ecs.Entity]bool)
	var onGrid []ecs.Entity
	for _, list := range s.storeLists() {
		for _, e := range list {
			if seen[e] || s.Docked.Has(e) {
				continue
			}
			seen[e] = true
			if s.Transforms.Has(e) {
				onGrid = append(onGrid, e)
			}
		}
	}
	sortEntities(onGrid)
	infos := make([]Info, 0, len(onGrid))
	for _, e := range onGrid {
		if info, ok := s.Info(e); ok {
			infos = append(infos, info)
		}
	}
	// Handle order, NOT distance order: Grid is the name-resolution surface
	// for commands and scripts, and distance ordering flips as targets circle
	// each other (two rats named "Scuttler" would swap places mid-fight).
	// Distance sorting is a presentation concern — the overview sorts its own
	// rows.
	return infos
}

// Info returns the flattened view of one entity.
func (s *Sim) Info(e ecs.Entity) (Info, bool) {
	if !s.World.Alive(e) || !s.Transforms.Has(e) {
		return Info{}, false
	}
	tr, _ := s.Transforms.Get(e)
	info := Info{Entity: e, Name: s.nameOf(e), Pos: tr.Pos, Health: 1}
	if v, ok := s.Motions.Get(e); ok {
		info.Vel = v.Vel
	}
	if s.Pilots.Has(e) {
		info.Kind = KindPlayer
	} else if s.Asteroids.Has(e) {
		info.Kind = KindAsteroid
	} else if s.Wrecks.Has(e) {
		info.Kind = KindWreck
	} else if c, ok := s.Celestials.Get(e); ok {
		switch c.Kind {
		case content.CelStation:
			info.Kind = KindStation
		case content.CelGate:
			info.Kind = KindGate
		case content.CelStar:
			info.Kind = KindStar
		case content.CelPlanet:
			info.Kind = KindPlanet
		case content.CelMoon:
			info.Kind = KindMoon
		case content.CelAnomaly:
			info.Kind = KindAnomaly
		default:
			// Belts and unknown kinds: no dedicated constant exists in
			// sim/model.go, so belts report as asteroid fields.
			info.Kind = KindAsteroid
		}
		info.Cel = c.Cel
	} else if s.Ships.Has(e) {
		info.Kind = KindShip
	} else {
		info.Kind = KindShip
	}

	// Owning celestial for loose grid objects (rocks, NPCs, wrecks).
	if c, ok := s.Celestials.Get(e); ok {
		info.Cel = c.Cel
	} else if info.Cel == "" {
		if near := s.nearestCelestial(tr.Pos); !near.IsZero() {
			if c, ok := s.Celestials.Get(near); ok {
				info.Cel = c.Cel
			}
		}
	}

	// Distance from the player.
	if s.player.IsZero() {
		info.Distance = 0
	} else if e == s.player {
		info.Distance = 0
	} else {
		info.Distance = s.distBetween(s.player, e)
	}

	// Health fraction over the three layers.
	if ship, ok := s.Ships.Get(e); ok {
		total := ship.MaxShield + ship.MaxArmor + ship.MaxStructure
		if total > 0 {
			info.Health = clamp((ship.Shield+ship.Armor+ship.Structure)/total, 0, 1)
		}
	}

	// Hostility: an NPC in an aggro state.
	if ai, ok := s.AIs.Get(e); ok {
		switch ai.State {
		case NpcApproach, NpcOrbit, NpcFlee:
			info.Hostile = true
		}
	}

	// Capability flags driving the Selected Item actions.
	docked := s.playerDocked()
	if s.Stations.Has(e) && e != s.player && !docked && info.Distance <= dockRange {
		info.CanDock = true
	}
	if s.Gates.Has(e) && e != s.player && !docked && info.Distance <= dockRange {
		info.CanJump = true
	}
	if w, ok := s.Wrecks.Get(e); ok {
		if !docked && len(w.Loot) > 0 && s.time < w.ExpiresAt && info.Distance <= dockRange {
			info.CanLoot = true
		}
	}
	if a, ok := s.Asteroids.Get(e); ok && a.Remaining > 0 && !docked {
		if t, ok := s.Targetings.Get(s.player); ok && info.Distance <= t.MaxRange {
			info.CanMine = true
		}
	}
	if _, ok := s.Celestials.Get(e); ok && e != s.player && !docked {
		info.CanWarp = true
	}
	return info, true
}

// Snapshot captures full serializable state.
func (s *Sim) Snapshot() (*Save, error) { return s.snapshot() }

// Restore replaces simulation state from a snapshot.
func (s *Sim) Restore(save *Save) error { return s.restore(save) }

// Market returns a copy of the market stock of the given system (empty when
// the system has no orders).
func (s *Sim) Market(id content.SystemID) []MarketStock {
	stock := s.market[id]
	out := make([]MarketStock, len(stock))
	copy(out, stock)
	return out
}
