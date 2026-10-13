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
	// RNG is the master stream. It carries exactly TWO roles — (1) player
	// combat rolls (hit/damage rolls via rngFor) and (2) spawn entropy:
	// asteroid and NPC scatter drawn in build.go plus the per-NPC Fork
	// seeds. Every other subsystem isolates itself on a fork (trafficRNG);
	// nothing may reach for s.RNG beyond those roles, and
	// rng_roles_test.go pins the master's draw set.
	RNG    *core.RNG
	Hangar *Hangar

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
	Traffic    *ecs.Store[Traffic]
	Patrols    *ecs.Store[Patrol]

	// trafficRNG is the living-universe RNG stream: convoy spawns, ambush
	// rolls, raider counts and patrol scatter all draw here so traffic
	// activity never shifts the master stream (player combat rolls).
	trafficRNG *core.RNG

	// pendingPlayerRespawn defers the death respawn to the end of a Step.
	pendingPlayerRespawn bool

	// Unowned simulation state (serialized through Snapshot's Save fields:
	// ModTargets, FreeSlots, LastStation, Standing and Missions).
	worldID     content.WorldID
	systemID    content.SystemID
	seed        int64
	ticks       int64
	time        float64
	wallet      float64
	standing    float64
	missions    []MissionState
	player      ecs.Entity
	lastStation content.CelID
	queue       []Command
	events      []Event
	// market holds one order book per station (station celestial id →
	// book), keyed universe-wide; stations sort by id for every read that
	// iterates (sortStationIDs). marketSeq is the next order arrival id.
	market    map[StationID]*MarketBook
	marketSeq uint64
	schema    int
	respawns  []RespawnTimer
	// trafficLeaves are scheduled raider break-offs (persisted; see Save).
	trafficLeaves []TrafficLeave
	modTargets    map[modKey]ecs.Entity
	free          []freeSlot

	// belts is the persistent world-metabolism belt state: per-rock
	// remaining volume keyed by (system, belt celestial), seeded from the
	// BeltDef on first generation and restored from Save.Belts. Mining and
	// rat requisitions keep it in sync with the live asteroid entities;
	// systemBelts applies the regrowth window.
	belts map[beltKey]*BeltState
	// factionLedger is the per-faction requisition money behind NPC
	// replacements (Save.FactionLedger), lazily seeded with
	// factionEndowment; only respawn requisitions debit it.
	factionLedger map[string]float64

	// --- industry (see system_industry.go): manufacturing jobs and the
	// per-station craft counters. jobs keeps ascending job-id order,
	// craft keeps (station, family) order — both are canonical iteration
	// orders for save/hash; jobSeq is the next job id.
	jobs   []Job
	jobSeq uint64
	craft  []SavedCraft

	// ammoPrefs is the per-(owner, slot) preferred charge item set by
	// CmdSetAmmo ("" entry = automatic). Point-looked-up only — never
	// iterated by behavior, so map order cannot leak into the simulation.
	// Documented limitation: not part of Save (the schema is versioned by
	// Save.Schema), so preferences reset on Restore.
	ammoPrefs map[modKey]content.ItemID

	// lastPlayerWork is where/when the player last fired a weapon or
	// completed a mining cycle — the recent-activity aggro trigger for NPCs
	// (see systemNpc). at == playerWorkNever means "never this run"; like
	// ammoPrefs it is intentionally outside the versioned Save schema and
	// resets on Restore.
	lastPlayerWork struct {
		pos core.Vec2
		at  float64
	}

	// Per-tick working state (never snapshotted; always empty between Steps).
	deferLoot    []CmdLoot
	deferEcon    []Command
	deferTravel  []Command
	deferMission []Command
	damage       []pendingDamage
	// Mission objective observations queued by other systems this tick and
	// folded into accepted contracts by systemMission (same Step).
	pendingKills []content.NpcID
	pendingOre   []pendingOreYield
	// dockedThisTick is the station celestial the player docked at this Step
	// ("" otherwise) — the auto-claim trigger for systemMission.
	dockedThisTick content.CelID
	// deferIndustry carries this Step's industry commands (CmdStartJob,
	// CmdCollectJob) to systemIndustry, the way deferEcon carries the
	// station-service commands to systemEconomy.
	deferIndustry []Command
}

// pendingOreYield is one mining cycle's yield recorded for mission tracking.
type pendingOreYield struct {
	item content.ItemID
	vol  float64
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

	books, seq := initMarket(c)
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
		ammoPrefs:  make(map[modKey]content.ItemID),
		market:     books,
		marketSeq:  seq,
		schema:     saveSchemaV2,
	}
	s.lastPlayerWork.at = playerWorkNever
	// Fork the living-universe stream from the fresh master state: same seed
	// => same traffic stream, and drawing here never consumes master rolls.
	s.trafficRNG = s.RNG.Fork(trafficStreamID)
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
	s.Traffic = ecs.NewStore[Traffic](w)
	s.Patrols = ecs.NewStore[Patrol](w)
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
	s.systemMission()   // 14 mission (accept/abandon, objectives, auto-claim)
	s.systemRespawn()   // 15 respawn
	s.systemTraffic()   // 16 traffic (convoys, ambushes, patrols, book supply)
	s.systemIndustry()  // 17 industry (manufacturing jobs, craft growth)
	// Death respawn runs after all systems so that a cross-system clone-bay
	// respawn can rebuild the grid without yanking entities out from under
	// the systems still running this tick.
	if s.pendingPlayerRespawn {
		s.pendingPlayerRespawn = false
		s.respawnPlayer()
	}
	// 18 events: capped at emit time; nothing else to trim.
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

// Wallet returns the player's euro balance.
func (s *Sim) Wallet() float64 { return s.wallet }

// Standing returns the player's career standing (credited on contract
// completion, gated by contract min_standing on acceptance).
func (s *Sim) Standing() float64 { return s.standing }

// Missions returns a copy of the player's accepted contracts and progress.
func (s *Sim) Missions() []MissionState {
	return append([]MissionState(nil), s.missions...)
}

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
		case content.CelBelt:
			info.Kind = KindBelt
		default:
			// Unknown celestial kinds fall back to the asteroid-field kind.
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

	// Visibility: landmarks are always visible (a docked or absent player
	// still sees the system), everything else only within SensorRange of an
	// undocked player. Grid() still RETURNS every on-grid entity — Visible
	// is presentation state for the overview/brackets, never a filter.
	if landmarkKind(info.Kind) {
		info.Visible = true
	} else if !s.player.IsZero() && !s.playerDocked() {
		info.Visible = info.Distance <= SensorRange
	}

	// Health fraction over the three layers.
	if ship, ok := s.Ships.Get(e); ok {
		total := ship.MaxShield + ship.MaxArmor + ship.MaxStructure
		if total > 0 {
			info.Health = clamp((ship.Shield+ship.Armor+ship.Structure)/total, 0, 1)
		}
	}

	// Hostility: an NPC in an aggro state, or a traffic raider mid-ambush
	// (its AI state machine still reads idle while it hunts a convoy).
	if ai, ok := s.AIs.Get(e); ok {
		switch ai.State {
		case NpcApproach, NpcOrbit, NpcFlee:
			info.Hostile = true
		}
	}
	if !info.Hostile && s.isTrafficRaider(e) {
		info.Hostile = true
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

// stationsSorted returns every station book id in ascending order — the
// canonical iteration order for anything that walks the markets (never
// map order).
func (s *Sim) stationsSorted() []StationID {
	ids := make([]StationID, 0, len(s.market))
	for id := range s.market {
		ids = append(ids, id)
	}
	sortStationIDs(ids)
	return ids
}

// Market returns every resting order of the given system: its stations'
// books concatenated in ascending station order, each in canonical book
// order (item ascending; asks before bids; best price first, then
// arrival). Empty when the system's markets hold no orders.
func (s *Sim) Market(id content.SystemID) []MarketStock {
	var out []MarketStock
	for _, st := range s.stationsSorted() {
		book := s.market[st]
		if book == nil || book.System != id {
			continue
		}
		out = append(out, book.Orders...)
	}
	return out
}

// BookOrders returns one station's resting orders in canonical book order
// (nil when the station holds no orders).
func (s *Sim) BookOrders(station StationID) []MarketStock {
	book := s.market[station]
	if book == nil {
		return nil
	}
	return append([]MarketStock(nil), book.Orders...)
}

// MarketPrice is the item's displayed price at a station: the price
// ladder — last trade price, best ask, best bid, then the def base price.
func (s *Sim) MarketPrice(station StationID, item content.ItemID) float64 {
	return s.marketPrice(s.market[station], item)
}

// DockedMarket returns the station the player is docked at when it offers
// the market service ("" otherwise) — the book a market view shows.
func (s *Sim) DockedMarket() StationID {
	e := s.dockedStation()
	if e.IsZero() {
		return ""
	}
	st, ok := s.Stations.Get(e)
	if !ok || !hasService(st, "market") {
		return ""
	}
	if c, ok := s.Celestials.Get(e); ok {
		return c.Cel
	}
	return ""
}

// --- industry (system 17) ---

// DockedIndustry returns the station the player is docked at when it
// offers the industry service ("" otherwise) — the station the factory
// panel reads jobs, craft levels and job gating from.
func (s *Sim) DockedIndustry() StationID {
	e := s.dockedStation()
	if e.IsZero() {
		return ""
	}
	st, ok := s.Stations.Get(e)
	if !ok || !hasService(st, "industry") {
		return ""
	}
	if c, ok := s.Celestials.Get(e); ok {
		return c.Cel
	}
	return ""
}

// IndustrySlots is how many jobs one station runs concurrently (the
// industrySlots design constant, for the factory panel's slot readout).
func IndustrySlots() int { return industrySlots }

// PlayerStock is the units of item the player can spend: general hold, ore
// hold, hangar stacks, then stored hulls — the have side of the factory
// panel's have/need column and exactly what StartJob commits from.
func (s *Sim) PlayerStock(item content.ItemID) int { return s.playerStock(item) }
