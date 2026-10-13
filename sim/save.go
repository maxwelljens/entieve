package sim

import (
	"encoding/json"
	"sort"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// buildSave assembles the Save struct from live state. The result may
// alias live component slices; snapshot() deep-copies it through JSON.
func (s *Sim) buildSave() (*Save, error) {
	save := &Save{
		World:     s.worldID,
		System:    s.systemID,
		Ticks:     s.ticks,
		Seed:      s.seed,
		RNGState:  s.RNG.Snapshot(),
		Wallet:    s.wallet,
		Standing:  s.standing,
		Missions:  append([]MissionState(nil), s.missions...),
		Hangar:    *s.Hangar,
		Schema:    s.schema,
		Market:    make(map[content.SystemID][]MarketStock, len(s.market)),
		MarketSeq: s.marketSeq,
		Respawn:   append([]RespawnTimer(nil), s.respawns...),
	}
	if s.trafficRNG != nil {
		save.TrafficRNGState = s.trafficRNG.Snapshot()
	}
	if len(s.trafficLeaves) > 0 {
		save.TrafficLeaves = append([]TrafficLeave(nil), s.trafficLeaves...)
	}

	// Market books: stations ascending, each book in its canonical order,
	// grouped under the book's system — the canonical serialization the
	// Hash fingerprints. Books with no orders and no trades contribute
	// nothing (an empty book and no book are the same state).
	for _, st := range s.stationsSorted() {
		book := s.market[st]
		if book == nil || (len(book.Orders) == 0 && len(book.Lasts) == 0) {
			continue
		}
		save.Market[book.System] = append(save.Market[book.System], book.Orders...)
		lasts := make([]content.ItemID, 0, len(book.Lasts))
		for item := range book.Lasts {
			lasts = append(lasts, item)
		}
		sort.Slice(lasts, func(i, j int) bool { return lasts[i] < lasts[j] })
		for _, item := range lasts {
			save.MarketLasts = append(save.MarketLasts, SavedLastTrade{
				System: book.System, Station: st, Item: item, Price: book.Lasts[item],
			})
		}
	}

	for _, e := range s.allEntities() {
		se := SavedEntity{ID: uint64(e)}
		if v, ok := s.Names.Get(e); ok {
			c := v.Value
			se.Name = &c
		}
		if v, ok := s.Transforms.Get(e); ok {
			c := *v
			se.Transform = &c
		}
		if v, ok := s.Motions.Get(e); ok {
			c := *v
			se.Motion = &c
		}
		if v, ok := s.Ships.Get(e); ok {
			c := *v
			se.Ship = &c
		}
		if v, ok := s.Caps.Get(e); ok {
			c := *v
			se.Cap = &c
		}
		if v, ok := s.Cargos.Get(e); ok {
			c := *v
			se.Cargo = &c
		}
		if v, ok := s.Fittings.Get(e); ok {
			c := *v
			se.Fitting = &c
		}
		if v, ok := s.Targetings.Get(e); ok {
			c := *v
			se.Targeting = &c
		}
		if v, ok := s.Pilots.Get(e); ok {
			c := *v
			se.Pilot = &c
		}
		if v, ok := s.AIs.Get(e); ok {
			c := SavedNpcAI{
				Npc: v.Npc, State: v.State, Target: uint64(v.Target), Home: v.Home,
				LastHitBy: uint64(v.LastHitBy),
			}
			if v.Rand != nil {
				c.RandState = v.Rand.Snapshot()
			}
			se.NPC = &c
		}
		if v, ok := s.Asteroids.Get(e); ok {
			c := *v
			se.Asteroid = &c
		}
		if v, ok := s.Celestials.Get(e); ok {
			c := *v
			se.Celestial = &c
		}
		if v, ok := s.Gates.Get(e); ok {
			c := *v
			se.Gate = &c
		}
		if v, ok := s.Stations.Get(e); ok {
			c := *v
			c.Services = append([]string(nil), v.Services...)
			se.Station = &c
		}
		if v, ok := s.Docked.Get(e); ok {
			c := *v
			se.Docked = &c
		}
		if v, ok := s.Warps.Get(e); ok {
			c := *v
			se.Warp = &c
		}
		if v, ok := s.Wrecks.Get(e); ok {
			c := *v
			c.Loot = append([]content.CargoStack(nil), v.Loot...)
			se.Wreck = &c
		}
		if v, ok := s.Intents.Get(e); ok {
			c := *v
			se.Intent = &c
		}
		if v, ok := s.Traffic.Get(e); ok {
			c := *v
			if v.Attack != nil {
				ac := *v.Attack
				ac.Raiders = append([]ecs.Entity(nil), v.Attack.Raiders...)
				c.Attack = &ac
			}
			se.Traffic = &c
		}
		if v, ok := s.Patrols.Get(e); ok {
			c := *v
			se.Patrol = &c
		}
		save.Entities = append(save.Entities, se)
	}

	// Canonical entity order so hashes and reconstructions are stable.
	sort.Slice(save.Entities, func(i, j int) bool {
		return save.Entities[i].ID < save.Entities[j].ID
	})

	// Active module target bindings. The source is a map, so sort by
	// owner/class/index to keep the serialization (and Hash) deterministic.
	for k, t := range s.modTargets {
		save.ModTargets = append(save.ModTargets, SavedModTarget{
			Owner: k.owner, Class: k.class, Index: k.index, Target: uint64(t),
		})
	}
	sort.Slice(save.ModTargets, func(i, j int) bool {
		a, b := save.ModTargets[i], save.ModTargets[j]
		if a.Owner != b.Owner {
			return a.Owner < b.Owner
		}
		if a.Class != b.Class {
			return a.Class < b.Class
		}
		return a.Index < b.Index
	})

	// Free-slot mirror in storage order (replayed verbatim by restore).
	for _, f := range s.free {
		save.FreeSlots = append(save.FreeSlots, SavedSlot{Index: f.idx, Generation: f.gen})
	}

	// Last docked station (respawn anchor). The celestial ID is always
	// persisted (LastStationCel) so an off-grid anchor survives the
	// round-trip; the handle (LastStation) is only set while the station is
	// on the current grid and is kept for saves written before the ID field
	// existed. Both sides derive the same pair, so hash(restore(x)) == hash(x).
	save.LastStationCel = s.lastStation
	if s.lastStation != "" {
		if e := s.stationAt(s.lastStation); !e.IsZero() {
			save.LastStation = uint64(e)
		}
	}

	// ---- world metabolism: persistent belts + faction ledgers ----
	// Canonical order: system id ascending, then celestial id ascending
	// (beltKeys), rocks verbatim in def order — map iteration order never
	// reaches the serialization the Hash fingerprints.
	for _, k := range s.beltKeys() {
		st := *s.belts[k]
		st.Rocks = append([]BeltRock(nil), s.belts[k].Rocks...)
		save.Belts = append(save.Belts, st)
	}
	if len(s.factionLedger) > 0 {
		save.FactionLedger = make(map[string]float64, len(s.factionLedger))
		for f, v := range s.factionLedger {
			save.FactionLedger[f] = v
		}
	}
	// --- industry: jobs and station craft (the canonical orders jobs and
	// craft are maintained in, re-asserted here so the serialization — and
	// therefore Hash — never depends on insertion order) ---
	if len(s.jobs) > 0 {
		save.Jobs = append([]Job(nil), s.jobs...)
		sort.Slice(save.Jobs, func(i, j int) bool { return save.Jobs[i].ID < save.Jobs[j].ID })
	}
	save.JobSeq = s.jobSeq
	if len(s.craft) > 0 {
		save.Craft = append([]SavedCraft(nil), s.craft...)
		sort.Slice(save.Craft, func(i, j int) bool {
			a, b := save.Craft[i], save.Craft[j]
			if a.Station != b.Station {
				return a.Station < b.Station
			}
			return a.Family < b.Family
		})
	}
	return save, nil
}

// snapshot captures the full serializable state. The result is produced by a
// JSON round-trip, which doubles as a deep copy (no aliasing with live sim
// state) and guarantees the Save stays serializable.
func (s *Sim) snapshot() (*Save, error) {
	save, err := s.buildSave()
	if err != nil {
		return nil, err
	}
	// Deep copy through JSON.
	raw, err := json.Marshal(save)
	if err != nil {
		return nil, err
	}
	var out Save
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// restore rebuilds the entire simulation from a snapshot.
//
// Entity handles are reproduced EXACTLY: the world's slot table (index +
// generation of every live slot) and free-list storage order are replayed
// from Save.FreeSlots, so post-restore World.Create calls hand out the same
// handles a continued pre-snapshot run would — keeping Hash and behavior
// identical across the round-trip. Pending events and queued commands are
// dropped (they are not part of Save).
func (s *Sim) restore(save *Save) error {
	if save == nil {
		return errf("sim: nil save")
	}
	// Work on a private deep copy: attach binds nested slices (cargo stacks,
	// fitting runtimes, lock lists, wreck loot) and callers may legitimately
	// restore several sims from one Save — stepping one must not mutate the
	// others' source data. A JSON round-trip is the same fidelity as the
	// on-disk format.
	raw, err := json.Marshal(save)
	if err != nil {
		return errf("sim: save not serializable: %v", err)
	}
	var sv Save
	if err := json.Unmarshal(raw, &sv); err != nil {
		return errf("sim: save not deserializable: %v", err)
	}
	save = &sv
	// Clean break: the market tranche bumped the schema to v2, so pre-v2
	// saves (per-system stock lists, drifting prices) are rejected outright
	// instead of restoring into a different market.
	if save.Schema != saveSchemaV2 {
		return errf("sim: save schema %d is not supported (want %d): pre-v2 saves are rejected",
			save.Schema, saveSchemaV2)
	}
	if _, ok := s.Content.System(save.System); !ok {
		return errf("sim: save references unknown system %s", save.System)
	}

	// Canonicalize the saved entity order.
	saved := append([]SavedEntity(nil), save.Entities...)
	sort.Slice(saved, func(i, j int) bool { return saved[i].ID < saved[j].ID })

	// Fresh world and stores.
	s.World = ecs.New()
	s.bindStores()
	s.queue = nil
	s.events = nil
	s.deferLoot, s.deferEcon, s.deferTravel, s.deferMission = nil, nil, nil, nil
	s.damage = nil
	s.pendingKills, s.pendingOre = nil, nil
	s.dockedThisTick = ""
	s.free = nil
	s.modTargets = make(map[modKey]ecs.Entity)
	// Preferences and the recent-player-work marker live outside the frozen
	// Save schema: a restored sim starts with automatic ammo and no record.
	s.ammoPrefs = make(map[modKey]content.ItemID)
	s.lastPlayerWork.pos, s.lastPlayerWork.at = core.Vec2{}, playerWorkNever

	// Slot table extent.
	maxIdx := -1
	for _, se := range saved {
		if i := int(uint32(se.ID)); i > maxIdx {
			maxIdx = i
		}
	}
	for _, f := range save.FreeSlots {
		if i := int(f.Index); i > maxIdx {
			maxIdx = i
		}
	}
	if maxIdx >= 1<<24 {
		return errf("sim: save slot table too large (%d)", maxIdx)
	}

	// Phase A: materialize slots 0..maxIdx as generation-1 dummies.
	handles := make([]ecs.Entity, maxIdx+1)
	for i := range handles {
		handles[i] = s.World.Create()
	}

	// Phase B: bump each live slot to its saved generation and attach the
	// saved components. The transient free entry from each destroy is popped
	// by the very next create, so the free list stays empty.
	for _, se := range saved {
		idx := uint32(se.ID)
		gen := uint32(se.ID >> 32)
		h := handles[idx]
		for h.Generation() < gen {
			s.World.Destroy(h)
			h = s.World.Create()
		}
		if h != ecs.Entity(se.ID) {
			return errf("sim: handle reconstruction mismatch for %d", se.ID)
		}
		handles[idx] = h
		s.attach(h, se)
	}

	// Phase C: replay the free list in storage order so future creates
	// reproduce the original handle sequence.
	for _, f := range save.FreeSlots {
		idx, gen := f.Index, f.Generation
		if int(idx) > maxIdx {
			return errf("sim: corrupt free list entry %d", idx)
		}
		h := handles[idx]
		for h.Generation() < gen {
			s.World.Destroy(h)
			h = s.World.Create()
		}
		handles[idx] = h
		s.World.Destroy(h)
	}
	for _, f := range save.FreeSlots {
		s.free = append(s.free, freeSlot{idx: f.Index, gen: f.Generation})
	}

	// Scalars and auxiliary state.
	s.worldID = save.World
	s.systemID = save.System
	s.ticks = save.Ticks
	s.time = float64(save.Ticks) * DT
	s.seed = save.Seed
	s.wallet = save.Wallet
	s.standing = save.Standing
	s.missions = append([]MissionState(nil), save.Missions...)
	s.RNG = core.RestoreRNG(save.RNGState)
	s.trafficRNG = core.RestoreRNG(save.TrafficRNGState)
	s.lastStation = ""
	// Prefer the always-persisted celestial ID; fall back to the legacy
	// handle for saves written before LastStationCel existed (a handle that
	// no longer resolves restores as the empty anchor, as before).
	if save.LastStationCel != "" {
		s.lastStation = save.LastStationCel
	} else if save.LastStation != 0 {
		if h := ecs.Entity(save.LastStation); s.World.Alive(h) {
			if c, ok := s.Celestials.Get(h); ok {
				s.lastStation = c.Cel
			}
		}
	}
	s.Hangar = cloneHangar(&save.Hangar)
	s.respawns = append([]RespawnTimer(nil), save.Respawn...)
	s.trafficLeaves = append([]TrafficLeave(nil), save.TrafficLeaves...)

	// Market: rebuild the per-station books from the saved orders (systems
	// in sorted order — map iteration order never reaches sim state) and
	// the last-trade rungs of the price ladder. Books are wholly saved
	// state: nothing is re-seeded from content here.
	s.market = make(map[StationID]*MarketBook)
	s.marketSeq = save.MarketSeq
	s.schema = save.Schema
	sysIDs := make([]content.SystemID, 0, len(save.Market))
	for sys := range save.Market {
		sysIDs = append(sysIDs, sys)
	}
	sortSystemIDs(sysIDs)
	bookFor := func(station StationID, sys content.SystemID) (*MarketBook, error) {
		book := s.market[station]
		if book == nil {
			book = &MarketBook{System: sys, Lasts: map[content.ItemID]float64{}}
			s.market[station] = book
			return book, nil
		}
		if book.System != sys {
			return nil, errf("sim: save has station %s orders under both %s and %s",
				station, book.System, sys)
		}
		return book, nil
	}
	for _, sys := range sysIDs {
		for _, o := range save.Market[sys] {
			if o.Side != OrderBuy && o.Side != OrderSell {
				return errf("sim: save order %d has invalid side %q", o.ID, o.Side)
			}
			if o.Qty <= 0 {
				return errf("sim: save order %d has non-positive qty %d", o.ID, o.Qty)
			}
			book, err := bookFor(o.Station, sys)
			if err != nil {
				return err
			}
			if o.Owner == "" {
				o.Owner = OwnerSystem
			}
			book.Orders = append(book.Orders, o)
		}
	}
	for _, lt := range save.MarketLasts {
		if lt.Price <= 0 {
			continue // a zero/negative "last price" is no price at all
		}
		book, err := bookFor(lt.Station, lt.System)
		if err != nil {
			return err
		}
		book.Lasts[lt.Item] = lt.Price
	}
	for _, book := range s.market { // per-book canonical order (order-independent)
		book.sortOrders()
	}

	for _, b := range save.ModTargets {
		k := modKey{owner: b.Owner, class: b.Class, index: b.Index}
		t := ecs.Entity(b.Target)
		if !t.IsZero() && s.World.Alive(t) && s.World.Alive(ecs.Entity(b.Owner)) {
			s.modTargets[k] = t
		}
	}

	// Locate the player.
	s.player = 0
	for _, e := range sortedEntities(s.Pilots) {
		if p, ok := s.Pilots.Get(e); ok && p.Player {
			s.player = e
			break
		}
	}
	if s.player.IsZero() {
		return errf("sim: save has no player entity")
	}

	// ---- world metabolism: persistent belts + faction ledgers ----
	// sv is this function's private deep copy, so the state may bind its
	// slices directly. A save written before the belts existed restores as
	// no belt state: the next system load re-seeds from the BeltDef (the
	// pre-feature refill semantics).
	s.belts = make(map[beltKey]*BeltState, len(save.Belts))
	for i := range save.Belts {
		st := save.Belts[i]
		if _, dup := s.belts[beltKey{st.System, st.Cel}]; dup {
			return errf("sim: save has duplicate belt state for %s/%s", st.System, st.Cel)
		}
		s.belts[beltKey{st.System, st.Cel}] = &st
	}
	if len(save.FactionLedger) > 0 {
		s.factionLedger = make(map[string]float64, len(save.FactionLedger))
		for f, v := range save.FactionLedger {
			s.factionLedger[f] = v
		}
	} else {
		s.factionLedger = nil
	}

	// --- industry: jobs and station craft (canonical orders replayed
	// verbatim), plus the per-Step industry command bucket reset ---
	s.deferIndustry = nil
	s.jobSeq = save.JobSeq
	for _, j := range save.Jobs {
		if j.ID == 0 || j.Station == "" || j.Runs < 1 {
			return errf("sim: save has invalid industry job %+v", j)
		}
		if j.State != JobRunning && j.State != JobDone {
			return errf("sim: save job %d has invalid state %q", j.ID, j.State)
		}
		// The id counter must cover every saved job: restoring a save that
		// would hand out a duplicate id fails loudly instead (and keeps
		// hash(restore(x)) == hash(x) for everything that IS accepted).
		if j.ID > save.JobSeq {
			return errf("sim: save job %d exceeds JobSeq %d", j.ID, save.JobSeq)
		}
	}
	s.jobs = append([]Job(nil), save.Jobs...)
	sort.Slice(s.jobs, func(i, j int) bool { return s.jobs[i].ID < s.jobs[j].ID })
	for i := 1; i < len(s.jobs); i++ {
		if s.jobs[i-1].ID == s.jobs[i].ID {
			return errf("sim: save has duplicate industry job id %d", s.jobs[i].ID)
		}
	}
	s.craft = append([]SavedCraft(nil), save.Craft...)
	sort.Slice(s.craft, func(i, j int) bool {
		a, b := s.craft[i], s.craft[j]
		if a.Station != b.Station {
			return a.Station < b.Station
		}
		return a.Family < b.Family
	})
	return nil
}

// attach writes one saved entity's components back into the fresh stores.
func (s *Sim) attach(e ecs.Entity, se SavedEntity) {
	if se.Name != nil {
		s.Names.Set(e, Name{Value: *se.Name})
	}
	if se.Transform != nil {
		s.Transforms.Set(e, *se.Transform)
	}
	if se.Motion != nil {
		s.Motions.Set(e, *se.Motion)
	}
	if se.Ship != nil {
		s.Ships.Set(e, *se.Ship)
	}
	if se.Cap != nil {
		s.Caps.Set(e, *se.Cap)
	}
	if se.Cargo != nil {
		s.Cargos.Set(e, *se.Cargo)
	}
	if se.Fitting != nil {
		s.Fittings.Set(e, *se.Fitting)
	}
	if se.Targeting != nil {
		s.Targetings.Set(e, *se.Targeting)
	}
	if se.Pilot != nil {
		s.Pilots.Set(e, *se.Pilot)
	}
	if se.NPC != nil {
		s.AIs.Set(e, NpcAI{
			Npc:    se.NPC.Npc,
			State:  se.NPC.State,
			Target: ecs.Entity(se.NPC.Target),
			Home:   se.NPC.Home,
			Rand:   core.RestoreRNG(se.NPC.RandState),
			// A handle absent from the save (never hit, or a pre-field save)
			// restores as zero — the "no recorded attacker" state.
			LastHitBy: ecs.Entity(se.NPC.LastHitBy),
		})
	}
	if se.Asteroid != nil {
		s.Asteroids.Set(e, *se.Asteroid)
	}
	if se.Celestial != nil {
		s.Celestials.Set(e, *se.Celestial)
	}
	if se.Gate != nil {
		s.Gates.Set(e, *se.Gate)
	}
	if se.Station != nil {
		st := *se.Station
		st.Services = append([]string(nil), se.Station.Services...)
		s.Stations.Set(e, st)
	}
	if se.Docked != nil {
		s.Docked.Set(e, *se.Docked)
	}
	if se.Warp != nil {
		s.Warps.Set(e, *se.Warp)
	}
	if se.Wreck != nil {
		w := *se.Wreck
		w.Loot = append([]content.CargoStack(nil), se.Wreck.Loot...)
		s.Wrecks.Set(e, w)
	}
	if se.Intent != nil {
		s.Intents.Set(e, *se.Intent)
	}
	if se.Traffic != nil {
		c := *se.Traffic
		if se.Traffic.Attack != nil {
			ac := *se.Traffic.Attack
			ac.Raiders = append([]ecs.Entity(nil), se.Traffic.Attack.Raiders...)
			c.Attack = &ac
		}
		s.Traffic.Set(e, c)
	}
	if se.Patrol != nil {
		s.Patrols.Set(e, *se.Patrol)
	}
}

// cloneHangar deep-copies a hangar (nil slices stay nil so JSON output —
// and therefore Hash — is byte-identical across round-trips).
func cloneHangar(h *Hangar) *Hangar {
	out := &Hangar{}
	out.Items = append([]content.CargoStack(nil), h.Items...)
	if len(h.Ships) > 0 {
		out.Ships = make([]StoredShip, len(h.Ships))
		for i, sh := range h.Ships {
			out.Ships[i] = StoredShip{
				Hull:  sh.Hull,
				Fit:   append([]content.ModuleID(nil), sh.Fit...),
				Cargo: append([]content.CargoStack(nil), sh.Cargo...),
			}
		}
	}
	return out
}
