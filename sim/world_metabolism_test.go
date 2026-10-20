package sim

import (
	"math"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
)

// ---------------------------------------------------------------------------
// World metabolism tests: persistent belts (deplete + regrow), wreck
// reprocessing, and market-funded NPC replacement (nothing spawns from
// nowhere).
// ---------------------------------------------------------------------------

// beltState returns the persisted state of one belt (test helper).
func beltState(s *Sim, sys content.SystemID, cel content.CelID) *BeltState {
	return s.belts[beltKey{System: sys, Cel: cel}]
}

// rockByIndex returns the live asteroid entity of one belt-state slot.
func rockByIndex(t *testing.T, s *Sim, cel content.CelID, index int) ecs.Entity {
	t.Helper()
	for _, e := range sortedEntities(s.Asteroids) {
		if a, ok := s.Asteroids.Get(e); ok && a.Belt == cel && a.Index == index {
			return e
		}
	}
	t.Fatalf("no live rock for %s[%d]", cel, index)
	return 0
}

// bookAskQty sums a station's resting asks for one item.
func bookAskQty(s *Sim, station StationID, item content.ItemID) int {
	q := 0
	for _, o := range s.BookOrders(station) {
		if o.Item == item && o.Side == OrderSell {
			q += o.Qty
		}
	}
	return q
}

// ---------------------------------------------------------------------------
// R1 — persistent belts
// ---------------------------------------------------------------------------

// TestBeltDepletionPersistsAcrossSaveRestoreAndReload pins the bug the
// world-metabolism wave fixes: mined-out volume must survive save/restore
// AND a system reload (loadSystem used to re-seed every rock from the
// BeltDef, silently refilling the belt on every entry).
func TestBeltDepletionPersistsAcrossSaveRestoreAndReload(t *testing.T) {
	s := newTestSim(t)
	rock := firstAsteroid(t, s)
	ast, ok := s.Asteroids.Get(rock)
	if !ok {
		t.Fatal("no asteroid")
	}
	index, celID := ast.Index, ast.Belt

	// Mine the rock to depletion (25 → 15 → 5 → 0 over three cycles).
	ast.Remaining = 25
	rp, _ := s.posOf(rock)
	setPlayerPos(s, rp.Add(v(500, 0)))
	laser := injectModule(t, s, "mod-mining-laser")
	s.Queue(CmdMine{Asteroid: rock, Slot: laser})
	s.Step()
	drain(s)
	stepN(s, 180)
	stepN(s, 180)
	evs := drain(s)
	if findEvent(evs, EvLog, "depleted") == nil {
		t.Fatalf("missing depletion event: %v", evs)
	}
	if s.World.Alive(rock) {
		t.Fatal("depleted asteroid should be destroyed")
	}
	st := beltState(s, "sys-a", celID)
	if st == nil {
		t.Fatal("no belt state")
	}
	if st.Rocks[index].Remaining > beltDepletedEpsilon {
		t.Fatalf("belt state rock %d = %v, want depleted", index, st.Rocks[index].Remaining)
	}

	// Save/restore keeps the depleted slot (and the hash).
	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(save.Belts) == 0 || save.Belts[0].Rocks[index].Remaining > beltDepletedEpsilon {
		t.Fatalf("Save.Belts lost the depletion: %+v", save.Belts)
	}
	b, err := New(testContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Restore(save); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := beltState(b, "sys-a", celID); got == nil || got.Rocks[index].Remaining > beltDepletedEpsilon {
		t.Fatalf("restored belt state = %+v, want depleted", got)
	}
	if h1, h2 := s.Hash(), b.Hash(); h1 != h2 {
		t.Fatalf("restored hash=%016x want %016x", h2, h1)
	}

	// System re-entry: the depleted rock does NOT come back, the other one
	// still has its volume — the exact refill bug scenario.
	for _, sim := range []*Sim{s, b} {
		if err := sim.LoadSystem("sys-a", ""); err != nil {
			t.Fatalf("reload: %v", err)
		}
		if n := len(sortedEntities(sim.Asteroids)); n != 1 {
			rocks := 0
			for _, e := range sortedEntities(sim.Asteroids) {
				if a, ok := sim.Asteroids.Get(e); ok && a.Belt == celID {
					rocks++
					if a.Index == index {
						t.Fatal("depleted rock respawned on re-entry")
					}
				}
			}
			t.Fatalf("belt rocks=%d (%d in this belt), want 1", n, rocks)
		}
		a, _ := sim.Asteroids.Get(rockByIndex(t, sim, celID, 1-index))
		if a.Remaining != 100 {
			t.Fatalf("surviving rock = %v, want 100", a.Remaining)
		}
	}
}

// TestBeltRegrowRateCapAndRecovery pins the regrowth math: +1% of the
// definition volume per 600 ticks per rock, capped at the definition
// volume, live rocks grow with their entity, and a depleted rock re-enters
// the grid on the next reload at its regrown volume.
func TestBeltRegrowRateCapAndRecovery(t *testing.T) {
	s := newTestSim(t)
	const celID = content.CelID("cel-belt-a")
	st := beltState(s, "sys-a", celID)
	if st == nil || len(st.Rocks) != 2 {
		t.Fatalf("belt state = %+v, want 2 rocks", st)
	}

	// Rock 0: depleted off-grid (state-only). Rock 1: live but below its
	// definition volume (entity authoritative).
	s.destroy(rockByIndex(t, s, celID, 0))
	st.Rocks[0].Remaining = 0
	st.GrownAt = s.ticks
	e1 := rockByIndex(t, s, celID, 1)
	ast1, _ := s.Asteroids.Get(e1)
	ast1.Remaining = 99.5
	st.Rocks[1].Remaining = 99.5

	stepN(s, 600) // systems read the pre-step tick count: no window yet
	if st.Rocks[0].Remaining != 0 {
		t.Fatalf("regrew before 600 ticks: %v", st.Rocks[0].Remaining)
	}
	stepN(s, 1) // window 1 (ticks = 600): +1% of 100
	if st.Rocks[0].Remaining != 1.0 {
		t.Fatalf("depleted rock = %v, want 1.0 (1%% of def volume per 600 ticks)", st.Rocks[0].Remaining)
	}
	if ast1.Remaining != 100.0 {
		t.Fatalf("live rock = %v, want 100.0 (capped at def volume)", ast1.Remaining)
	}
	if st.Rocks[1].Remaining != 100.0 {
		t.Fatalf("live rock state = %v, want 100.0", st.Rocks[1].Remaining)
	}
	stepN(s, 601) // window 2 (ticks = 1200)
	if st.Rocks[0].Remaining != 2.0 {
		t.Fatalf("depleted rock = %v, want 2.0", st.Rocks[0].Remaining)
	}
	if ast1.Remaining != 100.0 {
		t.Fatalf("capped rock grew past its definition volume: %v", ast1.Remaining)
	}
	stepN(s, 601) // window 3 (ticks = 1800)
	if st.Rocks[0].Remaining != 3.0 {
		t.Fatalf("depleted rock = %v, want 3.0", st.Rocks[0].Remaining)
	}

	// Recovery: on re-entry the depleted rock comes back at its regrown
	// volume (3 windows × 1% of 100 m³).
	if err := s.LoadSystem("sys-a", ""); err != nil {
		t.Fatalf("reload: %v", err)
	}
	a, _ := s.Asteroids.Get(rockByIndex(t, s, celID, 0))
	if a.Remaining != 3.0 {
		t.Fatalf("recovered rock = %v, want 3.0", a.Remaining)
	}
}

// TestBeltStateSurvivesReloadWithoutRefill is the acceptance gate for the
// refill bug in its plainest form: entering a system twice must not
// resurrect a mined-out rock at full volume.
func TestBeltStateSurvivesReloadWithoutRefill(t *testing.T) {
	s := newTestSim(t)
	rock := firstAsteroid(t, s)
	ast, _ := s.Asteroids.Get(rock)
	ast.Remaining = 40
	s.syncBeltRock(ast)
	if got := beltState(s, "sys-a", ast.Belt).Rocks[ast.Index].Remaining; got != 40 {
		t.Fatalf("state = %v, want 40", got)
	}
	if err := s.LoadSystem("sys-a", ""); err != nil {
		t.Fatal(err)
	}
	re, _ := s.Asteroids.Get(rockByIndex(t, s, ast.Belt, ast.Index))
	if re.Remaining != 40 {
		t.Fatalf("rock after reload = %v, want 40 (persisted, not refilled)", re.Remaining)
	}
	// The def still says 100: the cap is the definition volume.
	st := beltState(s, "sys-a", ast.Belt)
	if st.Rocks[ast.Index].Volume != 100 {
		t.Fatalf("cap = %v, want def volume 100", st.Rocks[ast.Index].Volume)
	}
}

// ---------------------------------------------------------------------------
// R7 — wreck reprocessing (value-added burns off)
// ---------------------------------------------------------------------------

// lootQty returns the quantity of an item in a loot list.
func lootQty(loot []content.CargoStack, item content.ItemID) int {
	for _, st := range loot {
		if st.Item == item {
			return st.Qty
		}
	}
	return 0
}

// TestWreckReclaimYieldsFromHullRecipe pins the reprocessing envelope on
// shipped content: a destroyed Scuttler's wreck carries floor(0.4 × its
// hull recipe inputs) as mineral stacks alongside its loot-table rolls,
// and the wreck's 300 s expiry is unchanged.
func TestWreckReclaimYieldsFromHullRecipe(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("load content: %v", err)
	}
	wd, ok := c.World("world-default")
	if !ok {
		t.Fatal("world-default missing")
	}
	s, err := New(c, wd, 42)
	if err != nil {
		t.Fatal(err)
	}
	e := s.spawnNpc("npc-rat-scuttler", playerPos(s).Add(v(60000, 0)), 100)
	if e.IsZero() {
		t.Fatal("spawnNpc failed")
	}
	before := s.time
	damageNPC(s, s.player, e, content.Damage{EM: 1e9}, false)
	s.Step()
	drain(s)

	w := entityByName(s, "Scuttler Wreck")
	if w.IsZero() {
		t.Fatal("no wreck spawned")
	}
	wr, _ := s.Wrecks.Get(w)
	rec := hullRecipe(c, "ship-rat-scuttler")
	if rec == nil {
		t.Fatal("fixture: scuttler has no recipe")
	}
	for _, in := range rec.Inputs {
		want := int(math.Floor(wreckReclaimFraction * in.Qty))
		if got := lootQty(wr.Loot, in.Item); got != want {
			t.Errorf("reclaim %s = %d, want floor(%.1f × %v) = %d", in.Item, got, wreckReclaimFraction, in.Qty, want)
		}
	}
	// A worked example pinned in the spec: 19,900 min-ferrite + 2,800
	// min-silex at 0.4 -> 7,960 + 1,120.
	if got := lootQty(wr.Loot, "min-ferrite"); got != 7960 {
		t.Errorf("min-ferrite = %d, want 7960", got)
	}
	if got := lootQty(wr.Loot, "min-silex"); got != 1120 {
		t.Errorf("min-silex = %d, want 1120", got)
	}
	// Expiry unchanged: 300 s after the death tick.
	if wr.ExpiresAt < before+wreckExpiry || wr.ExpiresAt > before+wreckExpiry+DT {
		t.Fatalf("expiry = %v, want %v (+1 tick)", wr.ExpiresAt, before+wreckExpiry)
	}
}

// TestWreckReclaimFloorAndScope pins the exact envelope rules: floor per
// input (0.8 units reclaim nothing), non-mineral inputs are not
// reprocessed, and a hull with no recipe adds nothing at all.
func TestWreckReclaimFloorAndScope(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("load content: %v", err)
	}
	wd, _ := c.World("world-default")
	// A crafted hull: fractional input floors, sub-unit input vanishes,
	// non-mineral input is out of scope for v1.
	c.Ships["ship-frac"] = &content.ShipDef{ID: "ship-frac", Name: "Fractional", Structure: 100}
	c.Recipes["rec-ship-frac"] = &content.RecipeDef{
		ID: "rec-ship-frac", Product: "ship-frac", Qty: 1, Time: 1, Family: content.FamilyHulls,
		Inputs: []content.RecipeInput{
			{Item: "min-ferrite", Qty: 10.9},     // floor(4.36) = 4
			{Item: "min-nickel", Qty: 2},         // floor(0.8) = 0 -> no stack
			{Item: "loot-alloy-scrap", Qty: 100}, // not a mineral -> out of scope
		},
	}
	c.NPCs["npc-test-frac"] = &content.NpcDef{
		ID: "npc-test-frac", Name: "Frac", Faction: "Test", Ship: "ship-frac",
	}
	s, err := New(c, wd, 42)
	if err != nil {
		t.Fatal(err)
	}
	e := s.spawnNpc("npc-test-frac", playerPos(s).Add(v(60000, 0)), 100)
	if e.IsZero() {
		t.Fatal("spawnNpc failed")
	}
	damageNPC(s, s.player, e, content.Damage{EM: 1e9}, false)
	s.Step()
	drain(s)
	w := entityByName(s, "Frac Wreck")
	if w.IsZero() {
		t.Fatal("no wreck")
	}
	wr, _ := s.Wrecks.Get(w)
	if len(wr.Loot) != 1 || wr.Loot[0].Item != "min-ferrite" || wr.Loot[0].Qty != 4 {
		t.Fatalf("loot = %+v, want exactly [{min-ferrite 4}]", wr.Loot)
	}

	// A hull with no recipe reclaims nothing: the fixture wreck carries
	// only its loot-table roll.
	fs := newTestSim(t)
	fn := fs.spawnNpc("npc-rat-scuttler", playerPos(fs).Add(v(60000, 0)), 100)
	if fn.IsZero() {
		t.Fatal("fixture spawn failed")
	}
	damageNPC(fs, fs.player, fn, content.Damage{EM: 1e9}, false)
	fs.Step()
	fw := entityByName(fs, "Rat Scuttler Wreck")
	if fw.IsZero() {
		t.Fatal("no fixture wreck")
	}
	fwr, _ := fs.Wrecks.Get(fw)
	if lootQty(fwr.Loot, "min-ferrite") != 0 || lootQty(fwr.Loot, "min-nickel") != 0 {
		t.Fatalf("recipe-less hull reclaimed minerals: %+v", fwr.Loot)
	}
	if lootQty(fwr.Loot, "loot-alloy-scrap") == 0 {
		t.Fatalf("loot-table roll missing: %+v", fwr.Loot)
	}
}

// ---------------------------------------------------------------------------
// R7 — market-funded NPC replacement
// ---------------------------------------------------------------------------

// guardContent extends the fixture with a guard spawn group at the market
// station and the book stock its requisition buys: hull, fitted module and
// the charge basket's ammo (ammo-ap-s is the canonical first light charge).
func guardContent() *content.Content {
	c := testContent()
	c.NPCs["npc-guard-test"] = &content.NpcDef{
		ID: "npc-guard-test", Name: "Fixture Guard", Faction: "Charter Authority",
		Ship: "ship-barge", Fit: []content.ModuleID{"mod-armor-plate", "mod-autocannon-s"},
		OrbitRange: 1500, FleeThreshold: 0.25,
	}
	sysA := c.Systems["sys-a"]
	for i := range sysA.Celestials {
		if sysA.Celestials[i].ID != "cel-station-a" {
			continue
		}
		sysA.Celestials[i].Spawns = []content.SpawnDef{
			{NpcID: "npc-guard-test", Count: 1, RespawnSec: 60},
		}
		sysA.Celestials[i].Station.Market = append(sysA.Celestials[i].Station.Market,
			content.MarketOrderDef{Item: "mod-autocannon-s", Qty: 10, Price: 500},
			content.MarketOrderDef{Item: "ammo-ap-s", Qty: 5000, Price: 5},
		)
	}
	return c
}

// oreAskContent is the fixture plus a station ask for the belt's ore (the
// rat fallback venue when the belt cannot pay).
func oreAskContent() *content.Content {
	c := testContent()
	sysA := c.Systems["sys-a"]
	for i := range sysA.Celestials {
		if sysA.Celestials[i].ID == "cel-station-a" {
			sysA.Celestials[i].Station.Market = append(sysA.Celestials[i].Station.Market,
				content.MarketOrderDef{Item: "ore-sterngrit", Qty: 100, Price: 20})
		}
	}
	return c
}

// emptyBelt drains a belt's state and freezes its regrowth (test helper).
func emptyBelt(t *testing.T, s *Sim) *BeltState {
	t.Helper()
	st := beltState(s, s.systemID, "cel-belt-a")
	if st == nil {
		t.Fatal("no belt state")
	}
	for i := range st.Rocks {
		st.Rocks[i].Remaining = 0
	}
	st.GrownAt = s.ticks + 1_000_000 // no regrowth window during the test
	return st
}

// TestRespawnGuardBuysHullFitAndCharges pins the guard requisition: the
// replacement BUYs its hull, fitted modules and 1000-round charge basket
// through the respawn station's book — stock drained, ledger debited,
// events emitted — and then spawns.
func TestRespawnGuardBuysHullFitAndCharges(t *testing.T) {
	s, err := New(guardContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	const station = StationID("cel-station-a")
	if bookAskQty(s, station, "ship-barge") != 3 {
		t.Fatalf("fixture asks = %d, want 3", bookAskQty(s, station, "ship-barge"))
	}

	guard := entityByName(s, "Fixture Guard")
	if guard.IsZero() {
		t.Fatal("fixture guard never spawned")
	}
	damageNPC(s, s.player, guard, content.Damage{EM: 1e6}, false)
	s.Step()
	drain(s)
	if len(s.respawns) != 1 {
		t.Fatalf("respawn timers = %d, want 1", len(s.respawns))
	}
	stepN(s, 60*61) // the 60 s cadence fires inside this window
	evs := drain(s)

	// Stock drained, exactly one of each bought.
	if got := bookAskQty(s, station, "ship-barge"); got != 2 {
		t.Errorf("hull asks = %d, want 2", got)
	}
	if got := bookAskQty(s, station, "mod-armor-plate"); got != 9 {
		t.Errorf("plate asks = %d, want 9", got)
	}
	if got := bookAskQty(s, station, "mod-autocannon-s"); got != 9 {
		t.Errorf("turret asks = %d, want 9", got)
	}
	if got := bookAskQty(s, station, "ammo-ap-s"); got != 4000 {
		t.Errorf("ammo-ap-s asks = %d, want 4000 (1000-round basket)", got)
	}
	// Ledger debited: hull 50,000 + plate 1,000 + turret 500 + basket 5,000.
	bal := s.factionLedger["Charter Authority"]
	if want := factionEndowment - 56_500; bal != want {
		t.Errorf("ledger = %v, want %v", bal, want)
	}
	// Events: the requisition purchase and the respawn itself.
	if findEvent(evs, EvLog, "Charter Authority requisitioned a Ore Barge at Haven Rest") == nil {
		t.Errorf("missing requisition event: %v", evs)
	}
	if findEvent(evs, EvLog, "Fixture Guard respawned near Haven Rest") == nil {
		t.Errorf("missing respawn event: %v", evs)
	}
	// The replacement exists, carries the fit and exactly the bought basket.
	nw := entityByName(s, "Fixture Guard")
	if nw.IsZero() {
		t.Fatal("guard did not respawn")
	}
	fit, ok := s.Fittings.Get(nw)
	if !ok || len(fit.High) != 1 || fit.High[0] != "mod-autocannon-s" || len(fit.Low) != 1 || fit.Low[0] != "mod-armor-plate" {
		t.Fatalf("fit = %+v, want turret + plate", fit)
	}
	cargo, _ := s.Cargos.Get(nw)
	if len(cargo.Stacks) != 1 || cargo.Stacks[0].Item != "ammo-ap-s" || cargo.Stacks[0].Qty != 1000 {
		t.Fatalf("basket = %+v, want 1000 ammo-ap-s", cargo.Stacks)
	}
	if len(s.respawns) != 0 {
		t.Fatalf("timer not consumed: %+v", s.respawns)
	}
}

// TestRespawnRequeuesWhenGuardCannotAfford pins the delay path: no money
// (or stock) means NO spawn, a warning event with the spec's text, and a
// re-queued timer 600 ticks later with no cap on retries — once the ledger
// is funded, the next attempt succeeds.
func TestRespawnRequeuesWhenGuardCannotAfford(t *testing.T) {
	s, err := New(guardContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	const station = StationID("cel-station-a")
	s.factionLedger = map[string]float64{"Charter Authority": 1000} // hull costs 50,000

	guard := entityByName(s, "Fixture Guard")
	if guard.IsZero() {
		t.Fatal("fixture guard never spawned")
	}
	damageNPC(s, s.player, guard, content.Damage{EM: 1e6}, false)
	s.Step()
	drain(s)

	stepN(s, 60*61)
	evs := drain(s)
	const delayed = "Charter Authority requisition delayed — no Ore Barge at Haven Rest"
	late := findEvent(evs, EvLog, delayed)
	if late == nil {
		t.Fatalf("missing delay event, want %q: %v", delayed, evs)
	}
	if entityByName(s, "Fixture Guard") != 0 {
		t.Fatal("guard spawned from nothing")
	}
	if len(s.respawns) != 1 {
		t.Fatalf("timers = %d, want 1 (re-queued)", len(s.respawns))
	}
	// Re-queued exactly 600 ticks (10 s) after the failed attempt.
	if want := late.Time + float64(respawnRetryTicks)*DT; math.Abs(s.respawns[0].At-want) > 1e-9 {
		t.Fatalf("requeue At = %v, want %v (600 ticks after the attempt at %v)", s.respawns[0].At, want, late.Time)
	}
	// Nothing was spent: stock and balance unchanged.
	if bookAskQty(s, station, "ship-barge") != 3 || bookAskQty(s, station, "mod-armor-plate") != 10 {
		t.Fatal("failed requisition drained stock")
	}
	if s.factionLedger["Charter Authority"] != 1000 {
		t.Fatalf("ledger = %v, want 1000 (untouched)", s.factionLedger["Charter Authority"])
	}

	// Fund the ledger: the next retry (no cap) succeeds.
	s.factionLedger["Charter Authority"] = factionEndowment
	stepN(s, int(respawnRetryTicks)+1)
	evs = drain(s)
	if findEvent(evs, EvLog, "Fixture Guard respawned near Haven Rest") == nil {
		t.Fatalf("retry did not spawn: %v", evs)
	}
	if bookAskQty(s, station, "ship-barge") != 2 {
		t.Fatalf("hull asks = %d after funded retry, want 2", bookAskQty(s, station, "ship-barge"))
	}
	if len(s.respawns) != 0 {
		t.Fatalf("timer not consumed: %+v", s.respawns)
	}
}

// TestRespawnRatDrawsOreFromBelt pins the rat path: a replacement pays
// oreCost of the system's belt state (entity and state stay in sync) and
// spawns — no market involved when the belt can pay.
func TestRespawnRatDrawsOreFromBelt(t *testing.T) {
	s := newTestSim(t)
	st := beltState(s, "sys-a", "cel-belt-a")
	if st == nil {
		t.Fatal("no belt state")
	}
	before := st.Rocks[0].Remaining
	cost := s.ratOreCost(mustNpc(t, s, "npc-rat-scuttler"))
	if cost != ratOreFallback {
		t.Fatalf("fixture ore cost = %v, want %v (recipe-less hull fallback)", cost, ratOreFallback)
	}

	s.respawns = append(s.respawns, RespawnTimer{
		System: "sys-a", Cel: "cel-anomaly-a", Npc: "npc-rat-scuttler", At: s.time,
	})
	s.Step()
	evs := drain(s)
	if findEvent(evs, EvLog, "Rat Scuttler respawned near Fixture Den") == nil {
		t.Fatalf("missing respawn event: %v", evs)
	}
	if got := before - st.Rocks[0].Remaining; got != cost {
		t.Fatalf("drawn = %v, want %v", got, cost)
	}
	// The live rock mirrors the draw.
	ast, _ := s.Asteroids.Get(rockByIndex(t, s, "cel-belt-a", 0))
	if ast.Remaining != before-cost {
		t.Fatalf("live rock = %v, want %v", ast.Remaining, before-cost)
	}
	// The basket is bought only when listed; the fixture does not sell the
	// canonical charge (ammo-ap-s), so the RESPAWNED hull carries none
	// (possibly-0 rule) while the initial spawn keeps its founding grant.
	empty, loaded := 0, 0
	for _, e := range sortedEntities(s.AIs) {
		ai, _ := s.AIs.Get(e)
		if ai.Npc != "npc-rat-scuttler" || ai.Home != v(5e6, 5e6) {
			continue
		}
		cargo, _ := s.Cargos.Get(e)
		if len(cargo.Stacks) == 0 {
			empty++
		} else {
			loaded++
		}
	}
	if empty != 1 || loaded != 1 {
		t.Fatalf("den scuttlers: %d without basket (want 1 respawned), %d with (want 1 initial)", empty, loaded)
	}
}

// TestRespawnRatBuysOreWhenBeltCannotPay pins the fallback: an empty belt
// makes the replacement buy its ore from the station's local ore asks on
// the faction ledger (stock drained, ledger debited), then spawn.
func TestRespawnRatBuysOreWhenBeltCannotPay(t *testing.T) {
	s, err := New(oreAskContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	emptyBelt(t, s)
	const station = StationID("cel-station-a")
	if bookAskQty(s, station, "ore-sterngrit") != 100 {
		t.Fatalf("ore asks = %d, want 100", bookAskQty(s, station, "ore-sterngrit"))
	}

	s.respawns = append(s.respawns, RespawnTimer{
		System: "sys-a", Cel: "cel-anomaly-a", Npc: "npc-rat-scuttler", At: s.time,
	})
	s.Step()
	evs := drain(s)
	if findEvent(evs, EvLog, "Rat Scuttler respawned near Fixture Den") == nil {
		t.Fatalf("missing respawn event: %v", evs)
	}
	// 50 m³ of ore-sterngrit (1 m³/unit) at 20 €: stock 100 → 50, ledger
	// endowment − 1,000.
	if got := bookAskQty(s, station, "ore-sterngrit"); got != 50 {
		t.Errorf("ore asks = %d, want 50", got)
	}
	bal := s.factionLedger["Mire Corsairs"]
	if want := factionEndowment - 50*20; bal != want {
		t.Errorf("ledger = %v, want %v", bal, want)
	}
	// The belt stayed drained (all-or-nothing draw never ran).
	st := beltState(s, "sys-a", "cel-belt-a")
	for i, r := range st.Rocks {
		if r.Remaining != 0 {
			t.Errorf("belt rock %d = %v, want 0 (market path, no partial draw)", i, r.Remaining)
		}
	}
}

// TestRespawnRatRequeuesWhenNoOreAnywhere pins the rat delay path: an
// empty belt and no ore asks spawn nothing, warn with the spec's text and
// re-queue the timer for 600 ticks.
func TestRespawnRatRequeuesWhenNoOreAnywhere(t *testing.T) {
	s := newTestSim(t)
	emptyBelt(t, s)

	s.respawns = append(s.respawns, RespawnTimer{
		System: "sys-a", Cel: "cel-anomaly-a", Npc: "npc-rat-scuttler", At: s.time,
	})
	before := s.AIs.Len()
	s.Step()
	evs := drain(s)
	const delayed = "Mire Corsairs requisition delayed — no ore for Rat Hull at Haven Rest"
	late := findEvent(evs, EvLog, delayed)
	if late == nil {
		t.Fatalf("missing delay event, want %q: %v", delayed, evs)
	}
	if s.AIs.Len() != before {
		t.Fatalf("NPCs = %d, want %d (spawned from nothing)", s.AIs.Len(), before)
	}
	if len(s.respawns) != 1 {
		t.Fatalf("timers = %d, want 1 (re-queued)", len(s.respawns))
	}
	if want := late.Time + float64(respawnRetryTicks)*DT; math.Abs(s.respawns[0].At-want) > 1e-9 {
		t.Fatalf("requeue At = %v, want %v", s.respawns[0].At, want)
	}
}

// mustNpc resolves an NPC def or fails the test.
func mustNpc(t *testing.T, s *Sim, id content.NpcID) *content.NpcDef {
	t.Helper()
	def, ok := s.Content.Npc(id)
	if !ok {
		t.Fatalf("unknown npc %s", id)
	}
	return def
}

// ---------------------------------------------------------------------------
// Determinism
// ---------------------------------------------------------------------------

// TestWorldMetabolismTwinRunsIdenticalHash drives mining (state sync),
// a real respawn requisition (ore draw + charge buy) and several regrowth
// windows through twin sims and requires identical hashes at every
// checkpoint — including after a snapshot/restore round-trip mid-window.
func TestWorldMetabolismTwinRunsIdenticalHash(t *testing.T) {
	drive := func(s *Sim, out map[int]uint64) {
		rock := firstAsteroid(t, s)
		rp, _ := s.posOf(rock)
		setPlayerPos(s, rp.Add(v(500, 0)))
		laser := injectModule(t, s, "mod-mining-laser")
		s.Queue(CmdMine{Asteroid: rock, Slot: laser})
		s.respawns = append(s.respawns, RespawnTimer{
			System: "sys-a", Cel: "cel-anomaly-a", Npc: "npc-rat-scuttler", At: s.time + 5,
		})
		for i := 1; i <= 1500; i++ {
			s.Step()
			if i == 100 || i == 700 || i == 1500 {
				out[i] = s.Hash()
			}
		}
	}
	a := newTestSim(t)
	b := newTestSim(t)
	ha, hb := map[int]uint64{}, map[int]uint64{}
	drive(a, ha)
	drive(b, hb)
	for _, cp := range []int{100, 700, 1500} {
		if ha[cp] != hb[cp] {
			t.Fatalf("checkpoint %d: hashes differ (a=%016x b=%016x)", cp, ha[cp], hb[cp])
		}
	}

	// Restore both from the same snapshot and keep stepping across regrowth
	// windows: belt state and ledger must round-trip into identical futures.
	save, err := a.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	c, err := New(testContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Restore(save); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if a.Hash() != c.Hash() {
		t.Fatalf("restored hash=%016x want %016x", c.Hash(), a.Hash())
	}
	for i := 0; i < 700; i++ {
		a.Step()
		c.Step()
	}
	if a.Hash() != c.Hash() {
		t.Fatalf("post-restore twin diverged: %016x vs %016x", a.Hash(), c.Hash())
	}
}
