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

	// ledger is the capped ring of the player's wallet movements
	// (Save.Ledger) — see ledger.go. Append order is deterministic; the
	// oldest entries are evicted past LedgerCap.
	ledger []LedgerEntry
	// market holds one order book per station (station celestial id →
	// book), keyed universe-wide; stations sort by id for every read that
	// iterates (sortStationIDs). marketSeq is the next order arrival id.
	market    map[StationID]*MarketBook
	marketSeq uint64
	schema    int
	respawns  []RespawnTimer
	// trafficLeaves are scheduled raider break-offs (persisted; see Save).
	trafficLeaves []TrafficLeave
	// trafficJourneys is the ledger of shipments parked between grids: a
	// hauler that crossed a gate (or whose grid was rebuilt under it) keeps
	// its cargo provenance and final destination here and flies again when
	// its system loads next (Save.TrafficJourneys — see save.go).
	trafficJourneys []TrafficJourney
	modTargets      map[modKey]ecs.Entity
	free            []freeSlot

	// belts is the persistent world-metabolism belt state: per-rock
	// remaining volume keyed by (system, belt celestial), seeded from the
	// BeltDef on first generation and restored from Save.Belts. Mining and
	// rat requisitions keep it in sync with the live asteroid entities;
	// systemBelts applies the belt cycle refill.
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

	// --- ledger agents (system 19; see agent.go) ---
	// agents is the deterministic roster in ascending id order (canonical
	// iteration — maps never reach behavior). Empty outside agentWorlds
	// (the showcase tour and test fixtures), which also gates upkeep.
	agents []Agent
	// upkeep is the per-station maintenance ledger system 19's upkeep
	// cycle spends and burns (Save.Upkeep), seeded once per station and
	// never refilled.
	upkeep map[StationID]float64

	// lastPlayerWork is where/when the player last fired a weapon or
	// completed a mining cycle — the recent-activity aggro trigger for NPCs
	// (see systemNpc). at == playerWorkNever means "never this run"; it is
	// intentionally outside the versioned Save schema and resets on Restore.
	lastPlayerWork struct {
		pos core.Vec2
		at  float64
	}

	// war is the Long Attrition's state (Save.War; see war.go): control per
	// system, the front's contest meters and the Corporate Fleets' treasury.
	// Nil in a world that carries no war (warWorlds), which keeps the
	// showcase tour and the test fixtures free of war demand.
	war *WarState

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
		market:     books,
		marketSeq:  seq,
		schema:     saveSchemaV3,
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
	// World gate: only free-play worlds carry the ledger-agent roster (the
	// showcase tour and fixtures run without agents — and therefore
	// without upkeep). Restore replaces the roster from the Save.
	if agentWorlds[w.ID] {
		s.spawnAgents()
	}
	// The war runs from tick 0 in the shipping free-play world (the same
	// world gate, its own list): control seeds from authored security, the
	// front derives from it, and the treasury seeds once.
	if warWorlds[w.ID] {
		s.newWarState()
	}
	// Fresh fits pre-load: every bay fills from cargo before the first Step,
	// settled at once — no UI exists yet to show a load cycle (all later loads
	// are clocked; see preloadBays).
	s.preloadBays()
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
	s.systemShield()    // 9  shield recharge
	s.systemMining()    // 10 mining
	s.systemNpc()       // 11 npc
	s.systemLoot()      // 12 loot
	s.systemEconomy()   // 13 economy
	s.systemTravel()    // 14 travel
	s.systemMission()   // 15 mission (accept/abandon, objectives, auto-claim)
	s.systemRespawn()   // 16 respawn
	s.systemTraffic()   // 17 traffic (convoys, ambushes, patrols, book supply)
	s.systemIndustry()  // 18 industry (manufacturing jobs, craft growth)
	s.systemAgent()     // 19 agents (ledger economy: settle, behavior pass, upkeep)
	s.systemWar()       // 20 the war (meters, procurement bids, fleets)
	// Death respawn runs after all systems so that a cross-system clone-bay
	// respawn can rebuild the grid without yanking entities out from under
	// the systems still running this tick.
	if s.pendingPlayerRespawn {
		s.pendingPlayerRespawn = false
		s.respawnPlayer()
	}
	// 19 events: capped at emit time; nothing else to trim.
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

// ClipState is one charge module's shell bay as the UI reads it: the loaded
// rounds and the bay's capacity for that shell (in whole rounds), the shell
// type the bay tops up from, every shell type in cargo that fits it (sorted by
// id), and whether a load is in flight. An empty bay with two or more Options
// is the waiting state — the player must choose (CmdSetAmmo) before the gun
// fires again. Loading is true from the moment a load moves its rounds into
// the bay until its cycle completes: the rounds are in but not yet usable.
//
// BayM3 and CycleSeconds are the bay's spec, derived here from the same
// privates the load runs on (clipBay, cyclePeriod) so a read side never
// re-derives them from a ModuleDef: the bay's capacity in m³ and the load
// cycle it costs, the 1 s floor included. Both are READ-ONLY and recomputed on
// every Clip call — never stored in a component, never in the Save schema.
type ClipState struct {
	Item         content.ItemID
	Rounds       int
	Capacity     int
	Selection    content.ItemID
	Options      []content.ItemID
	Loading      bool
	BayM3        float64
	CycleSeconds float64
}

// Clip reports one slot's shell bay; ok is false when the slot holds no bay
// (no such module, a charge-free weapon, or a non-charge module).
func (s *Sim) Clip(e ecs.Entity, slot SlotRef) (ClipState, bool) {
	def, rt, ok := s.moduleDef(e, slot)
	if !ok {
		return ClipState{}, false
	}
	bay := clipBay(def)
	if bay <= 0 {
		return ClipState{}, false
	}
	cargo, _ := s.Cargos.Get(e)
	var stacks []content.CargoStack
	if cargo != nil {
		stacks = cargo.Stacks
	}
	item := rt.Clip.Item
	if item == "" {
		item = rt.Selection
	}
	return ClipState{
		Item:         rt.Clip.Item,
		Rounds:       rt.Clip.Qty,
		Capacity:     s.clipRounds(def, item),
		Selection:    rt.Selection,
		Options:      s.clipTypes(def, stacks),
		Loading:      loadInFlight(rt),
		BayM3:        bay,
		CycleSeconds: cyclePeriod(def),
	}, true
}

// ModuleTarget reports the target an active module is bound to — the
// activation target its cycles are validated and resolved against. Read
// only: the renderer aims a gun's shot at the ship that gun actually
// fires at, not at whichever lock happens to be first.
func (s *Sim) ModuleTarget(e ecs.Entity, slot SlotRef) (ecs.Entity, bool) {
	return s.modTarget(e, slot)
}

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
func (s *Sim) DockedMarket() StationID { return s.dockedStationFor(content.SvcMarket) }

// dockedStationFor returns the station the player is docked at when it
// offers svc ("" otherwise) — the service accessor every panel reads.
func (s *Sim) dockedStationFor(svc string) StationID {
	e := s.dockedStation()
	if e.IsZero() {
		return ""
	}
	st, ok := s.Stations.Get(e)
	if !ok || !hasService(st, svc) {
		return ""
	}
	if c, ok := s.Celestials.Get(e); ok {
		if !s.stationOffers(c.Cel, svc) {
			return ""
		}
		return c.Cel
	}
	return ""
}

// --- industry (system 18) ---

// DockedIndustry returns the station the player is docked at when it
// offers the industry service ("" otherwise) — the station the factory
// panel reads jobs, craft levels and job gating from.
func (s *Sim) DockedIndustry() StationID { return s.dockedStationFor(content.SvcIndustry) }

// IndustrySlots is how many jobs ALL stations run concurrently (the
// industrySlots design constant, for the manufacturing panel's readout).
func IndustrySlots() int { return industrySlots }

// PlayerStock is the units of item the player can spend: general hold, ore
// hold, hangar stacks, then stored hulls — the have side of the factory
// panel's have/need column and exactly what StartJob commits from.
func (s *Sim) PlayerStock(item content.ItemID) int { return s.playerStock(item) }

// --- ledger agents (system 19) ---

// Agents returns the roster in canonical (ascending id) order: the
// read-only view of every ledger agent (empty in worlds without agents).
func (s *Sim) Agents() []AgentView {
	out := make([]AgentView, 0, len(s.agents))
	for i := range s.agents {
		a := &s.agents[i]
		out = append(out, AgentView{
			ID: a.ID, Home: a.Home, Role: a.Role, Capital: a.Capital,
			Dormant: a.Capital < dormantCapital,
		})
	}
	return out
}

// HaulWishes returns every agent's pending haul wishes, agents in
// ascending id order and each agent's list in its canonical (From, To,
// Item) order — the logistics consumption seam (system_traffic.go). A wish
// still riding a convoy stays pending until it is fulfilled or lost, so it
// appears here too (Agent.InFlight marks the committed ones).
func (s *Sim) HaulWishes() []HaulWish {
	var out []HaulWish
	for i := range s.agents {
		out = append(out, s.agents[i].Wishes...)
	}
	return out
}

// FulfillHaul acknowledges a haul wish: it pays cost from the owning
// agent's FREE capital (resting-order escrow is untouched; the payment is
// a sink — the haul fee has no ledger on the far side) and drops the wish
// from the agent's list, releasing its in-flight commitment. Returns false
// when the owner/wish is unknown or cost is negative.
func (s *Sim) FulfillHaul(wish HaulWish, cost float64) bool {
	if cost < 0 || wish.Owner == "" {
		return false
	}
	a := s.agentByOwner(wish.Owner)
	if a == nil {
		return false
	}
	idx := -1
	for i := range a.Wishes {
		w := a.Wishes[i]
		if w.From == wish.From && w.To == wish.To && w.Item == wish.Item &&
			w.Qty == wish.Qty && w.MaxCost == wish.MaxCost {
			idx = i
			break
		}
	}
	if idx < 0 {
		return false
	}
	a.Capital -= min(cost, a.freeCapital())
	a.Wishes = append(a.Wishes[:idx], a.Wishes[idx+1:]...)
	a.dropInFlight(wish)
	return true
}
