package sim

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

// trafficContent extends the standard fixture with the living-universe
// layer: a second station (a delivery endpoint), convoy/guard hulls, a
// guard NPC def, a pinning pirate rat, and traffic defs for both fixture
// systems.
//
//	sys-a (high): no natural ambushes (chance 0); patrol on duty; the raider
//	  id is set so tests can force ambushes.
//	sys-b (low):  1-in-6 ambush rolls; no patrols (lawless trade lane).
func trafficContent() *content.Content {
	c := testContent()
	sysA := c.Systems["sys-a"]
	sysA.Celestials = append(sysA.Celestials, content.CelestialDef{
		ID: "cel-a-dock", Name: "Dock Two", Kind: content.CelStation,
		Position: core.Vec2{X: 1.5e7, Y: -5e6}, Radius: 500,
		Station: &content.StationDef{Services: []string{"repair", "market"}},
	})

	c.Ships["ship-convoy"] = &content.ShipDef{
		ID: "ship-convoy", Name: "Convoy Hauler", Class: "industrial",
		Slots:  content.Slots{High: 1, Mid: 1, Low: 1},
		Shield: 200, Armor: 200, Structure: 200,
		Resists:   content.Resists{EM: 0.1, Thermal: 0.1, Kinetic: 0.1, Explosive: 0.1},
		Capacitor: 500, Recharge: 100,
		MaxSpeed: 200, Agility: 0.5, Mass: 5e6, SigRadius: 100,
		Cargo: 500, OreHold: 0, WarpSpeed: 3,
		MaxTargetRange: 20000, MaxTargets: 1, ScanResolution: 300,
	}
	// Fixture turret with shipped-style tracking (145): the shared fixture's
	// mod-emp-turret tracks at 0.4, which never hits a drifting target —
	// NPC-vs-NPC fire in these tests needs a real hit chance.
	c.Modules["mod-test-turret"] = &content.ModuleDef{
		ID: "mod-test-turret", Name: "Test Turret", Slot: content.SlotHigh,
		Kind: content.KindWeapon, ActivationCost: 10, ActivationTime: 2, Price: 1000,
		Weapon: &content.WeaponAttrs{
			Damage: content.Damage{EM: 10}, Optimal: 6000, Falloff: 3000,
			Tracking: 145, AmmoClass: "light", ShotTime: 2,
		},
	}
	c.Ships["ship-guard"] = &content.ShipDef{
		ID: "ship-guard", Name: "Guard", Class: "frigate",
		Slots:  content.Slots{High: 3, Mid: 1, Low: 1},
		Shield: 500, Armor: 300, Structure: 300,
		Resists:   content.Resists{EM: 0.15, Thermal: 0.15, Kinetic: 0.15, Explosive: 0.15},
		Capacitor: 900, Recharge: 120,
		MaxSpeed: 380, Agility: 0.5, Mass: 1.5e6, SigRadius: 45,
		Cargo: 100, OreHold: 0, WarpSpeed: 4,
		MaxTargetRange: 40000, MaxTargets: 3, ScanResolution: 550,
	}
	c.NPCs["npc-guard-test"] = &content.NpcDef{
		ID: "npc-guard-test", Name: "Guard", Faction: "Test", Ship: "ship-guard",
		Fit:           []content.ModuleID{"mod-test-turret", "mod-test-turret", "mod-test-turret"},
		AggroRange:    6000,
		OrbitRange:    1500,
		FleeThreshold: 0.1,
		Bounty:        0,
	}
	// The traffic raiders (pinning pirate included).
	c.NPCs["npc-rat-raider"] = &content.NpcDef{
		ID: "npc-rat-raider", Name: "Raider", Faction: "Test", Ship: "ship-rat",
		Fit:           []content.ModuleID{"mod-test-turret", "mod-test-turret"},
		AggroRange:    8000,
		OrbitRange:    2000,
		FleeThreshold: 0.2,
		Bounty:        5000,
		Loot:          []content.LootDrop{{Item: "loot-alloy-scrap", Chance: 1, MinQty: 1, MaxQty: 1}},
	}
	c.NPCs["npc-rat-pirate"] = &content.NpcDef{
		ID: "npc-rat-pirate", Name: "Pirate", Faction: "Test", Ship: "ship-rat",
		Fit:           []content.ModuleID{"mod-test-turret", "mod-test-turret", "mod-warp-scram"},
		AggroRange:    8000,
		OrbitRange:    2000,
		FleeThreshold: 0.2,
		Bounty:        5000,
		Loot:          []content.LootDrop{{Item: "loot-alloy-scrap", Chance: 1, MinQty: 1, MaxQty: 1}},
	}

	route := func(from, to content.CelID, item content.ItemID, qty int, every int64) content.RouteDef {
		return content.RouteDef{
			From: from, To: to, Hull: "ship-convoy",
			Cargo:         content.RouteCargo{Item: item, Qty: qty},
			IntervalTicks: every,
		}
	}
	c.Traffic = map[content.SystemID]*content.TrafficDef{
		"sys-a": {
			System: "sys-a",
			Routes: []content.RouteDef{
				route("cel-station-a", "cel-a-dock", "min-ferrite", 100, 600),
				route("cel-a-dock", "cel-gate-b", "charge-emp", 200, 900),
			},
			Patrol: &content.PatrolDef{Hulls: []content.ShipID{"ship-guard"}, IntervalTicks: 600},
			Raider: "npc-rat-raider",
		},
		"sys-b": {
			System: "sys-b",
			Routes: []content.RouteDef{
				route("cel-gate-a", "cel-belt-b", "ore-sterngrit", 50, 600),
				route("cel-belt-b", "cel-gate-a", "min-ferrite", 50, 900),
			},
			Raider: "npc-rat-raider",
		},
	}
	return c
}

func newTrafficSim(t *testing.T) *Sim {
	t.Helper()
	return newTrafficSimSeed(t, 7)
}

func newTrafficSimSeed(t *testing.T, seed int64) *Sim {
	t.Helper()
	s, err := New(trafficContent(), testWorld(), seed)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Park the player away from the trade lanes: raiders spawning within
	// 8 km of the player would aggro the pilot instead of the convoy
	// (NPC guard trigger d) and derail the NPC-vs-NPC scenarios.
	setPlayerPos(s, core.Vec2{X: -5e6, Y: -5e6})
	drain(s)
	return s
}

// stepCollect steps n ticks, draining events into acc.
func stepCollect(s *Sim, acc *[]Event, n int) {
	for i := 0; i < n; i++ {
		s.Step()
		*acc = append(*acc, drain(s)...)
	}
}

// runUntil steps up to maxTicks in chunks, accumulating events into evs
// (which cond closures read), until cond holds.
func runUntil(t *testing.T, s *Sim, evs *[]Event, maxTicks int, cond func() bool) bool {
	t.Helper()
	for i := 0; i < maxTicks; i += 10 {
		stepCollect(s, evs, 10)
		if cond() {
			return true
		}
	}
	return false
}

// findHauler returns the first live convoy hauler bound for dest ("" = any).
func findHauler(s *Sim, dest content.CelID) (ecs.Entity, *Traffic, bool) {
	for _, e := range sortedEntities(s.Traffic) {
		if tc, ok := s.Traffic.Get(e); ok && (dest == "" || tc.Dest == dest) {
			return e, tc, true
		}
	}
	return 0, nil, false
}

// firstPhase returns the first event with the given kind/phase.
func firstPhase(evs []Event, kind EventKind, phase string) *Event {
	for i := range evs {
		if evs[i].Kind == kind && evs[i].Phase == phase {
			return &evs[i]
		}
	}
	return nil
}

// countPhase counts events with the given kind/phase.
func countPhase(evs []Event, kind EventKind, phase string) int {
	n := 0
	for _, ev := range evs {
		if ev.Kind == kind && ev.Phase == phase {
			n++
		}
	}
	return n
}

// forceAmbushOn ambushes a specific hauler immediately (deterministic
// replacement for the per-leg security roll).
func forceAmbushOn(t *testing.T, s *Sim, e ecs.Entity, tc *Traffic, security string) {
	t.Helper()
	td, _ := s.Content.TrafficFor(s.systemID)
	s.startAmbush(e, tc, td, security)
	if tc.Attack == nil {
		t.Fatal("startAmbush produced no attack state")
	}
}

// marketQtyIn returns the stock and price of an item in a system's market.
func marketQtyIn(s *Sim, sys content.SystemID, item content.ItemID) (int, float64) {
	for _, st := range s.market[sys] {
		if st.Item == item {
			return st.Qty, st.Price
		}
	}
	return 0, 0
}

// playerCargoCount counts units of an item in the player's general cargo.
func playerCargoCount(s *Sim, item content.ItemID) int {
	c, _ := s.Cargos.Get(s.player)
	return stacksCount(c.Stacks, item)
}

// aliveRaiders lists the live raiders of any attack.
func aliveRaiders(s *Sim) []ecs.Entity {
	var out []ecs.Entity
	s.Traffic.Each(func(_ ecs.Entity, tc *Traffic) {
		if tc.Attack == nil {
			return
		}
		for _, r := range tc.Attack.Raiders {
			if s.World.Alive(r) {
				out = append(out, r)
			}
		}
	})
	return out
}

// trafficTexts renders EvTraffic lines for failure messages.
func trafficTexts(evs []Event) []string {
	var out []string
	for _, ev := range evs {
		if ev.Kind == EvTraffic {
			out = append(out, string(ev.Phase)+" "+ev.Text)
		}
	}
	return out
}

// wreckWithCargo returns the first wreck carrying the given item.
func wreckWithCargo(s *Sim, item content.ItemID) ecs.Entity {
	for _, info := range s.Grid() {
		if info.Kind != KindWreck {
			continue
		}
		if w, ok := s.Wrecks.Get(info.Entity); ok && stacksCount(w.Loot, item) > 0 {
			return info.Entity
		}
	}
	return 0
}

// shipVitals sums a ship's current layers.
func shipVitals(s *Sim, e ecs.Entity) (shield, armor, structure float64) {
	sh, ok := s.Ships.Get(e)
	if !ok {
		return
	}
	return sh.Shield, sh.Armor, sh.Structure
}

// ---------------------------------------------------------------------------
// Calibration math (the binding tables in docs/CONTENT_AUTHORING.md)
// ---------------------------------------------------------------------------

func TestTrafficSecurityMathPinned(t *testing.T) {
	for sec, want := range map[string]float64{"high": 1, "low": 0.5, "null": 0} {
		if got := securitySigma(sec); got != want {
			t.Errorf("securitySigma(%q) = %v, want %v", sec, got, want)
		}
	}
	// spawn interval multiplier 4^(1-sigma): x1, x2, x4
	for _, tc := range []struct {
		sec string
		in  int64
		out int64
	}{{"high", 5400, 5400}, {"low", 5400, 10800}, {"null", 5400, 21600},
		{"high", 1, 1}, {"low", 1, 2}, {"null", 1, 4}} {
		if got := trafficSpawnInterval(tc.in, tc.sec); got != tc.out {
			t.Errorf("trafficSpawnInterval(%d, %q) = %d, want %d", tc.in, tc.sec, got, tc.out)
		}
	}
	// attack chance (1-sigma)/3: 0, 1/6, 1/3
	for _, tc := range []struct {
		sec  string
		want float64
	}{{"high", 0}, {"low", 1.0 / 6}, {"null", 1.0 / 3}} {
		if got := trafficAttackChance(tc.sec); got != tc.want {
			t.Errorf("trafficAttackChance(%q) = %v, want %v", tc.sec, got, tc.want)
		}
	}
	// price band 0.05 * 4^(1-sigma): 5%, 10%, 20%
	for _, tc := range []struct {
		sec  string
		want float64
	}{{"high", 0.05}, {"low", 0.10}, {"null", 0.20}} {
		if got := trafficBand(tc.sec); got != tc.want {
			t.Errorf("trafficBand(%q) = %v, want %v", tc.sec, got, tc.want)
		}
	}
	// patrol cadence 1/sigma: x1, x2, disabled
	if got, ok := trafficPatrolInterval(9000, "high"); !ok || got != 9000 {
		t.Errorf("patrol interval high = %d/%v, want 9000/true", got, ok)
	}
	if got, ok := trafficPatrolInterval(9000, "low"); !ok || got != 18000 {
		t.Errorf("patrol interval low = %d/%v, want 18000/true", got, ok)
	}
	if _, ok := trafficPatrolInterval(9000, "null"); ok {
		t.Error("patrol interval null must be disabled")
	}
	// response: delay 1800*(1-sigma) ticks, wing floor(3*sigma)
	for _, tc := range []struct {
		sec      string
		delay    int64
		wantWing int
	}{{"high", 0, 3}, {"low", 900, 1}, {"null", 1800, 0}} {
		if got := trafficResponseDelay(tc.sec); got != tc.delay {
			t.Errorf("trafficResponseDelay(%q) = %d, want %d", tc.sec, got, tc.delay)
		}
		if got := trafficPatrolWing(tc.sec); got != tc.wantWing {
			t.Errorf("trafficPatrolWing(%q) = %d, want %d", tc.sec, got, tc.wantWing)
		}
	}
	// Stateless schedules: exactly N/interval due ticks in a full window,
	// off-interval ticks quiet, distinct keys hash apart.
	if trafficDue(0, "k", 0) {
		t.Error("trafficDue with zero interval must be false")
	}
	if trafficPhase("a") == trafficPhase("b") {
		t.Error("distinct schedule keys must hash apart")
	}
	dues := 0
	for tick := int64(0); tick < 1000; tick++ {
		if trafficDue(tick, "a", 100) {
			dues++
		}
	}
	if dues != 10 {
		t.Errorf("due ticks in [0,1000) = %d, want 10", dues)
	}
}

// ---------------------------------------------------------------------------
// Lifecycle, rates, caps
// ---------------------------------------------------------------------------

// TestTrafficLifecycleDeliveryOrder pins the full convoy lifecycle: spawn
// (departed) -> warp the leg -> deliver at the destination station
// (delivered + one supply price step) -> despawn.
func TestTrafficLifecycleDeliveryOrder(t *testing.T) {
	s := newTrafficSim(t)
	var evs []Event
	if !runUntil(t, s, &evs, 3000, func() bool {
		return firstPhase(evs, EvTraffic, TrafficDelivered) != nil
	}) {
		t.Fatalf("no delivery within 3000 ticks; events=%v", trafficTexts(evs))
	}
	departed := firstPhase(evs, EvTraffic, TrafficDeparted)
	if departed == nil {
		t.Fatal("no departed event before delivery")
	}
	if departed.System != "sys-a" || departed.Cel != "cel-station-a" {
		t.Fatalf("departed = system %q cel %q, want sys-a/cel-station-a", departed.System, departed.Cel)
	}
	delivered := firstPhase(evs, EvTraffic, TrafficDelivered)
	if delivered.System != "sys-a" || delivered.Cel != "cel-a-dock" {
		t.Fatalf("delivered = system %q cel %q, want sys-a/cel-a-dock", delivered.System, delivered.Cel)
	}
	if departed.Time > delivered.Time {
		t.Fatalf("departed at %.2f after delivered at %.2f", departed.Time, delivered.Time)
	}
	if delivered.Item != "min-ferrite" || delivered.Value != 100 {
		t.Fatalf("delivered item/value = %q/%v, want min-ferrite/100", delivered.Item, delivered.Value)
	}
	// The delivering hauler despawned (its entity is gone).
	if s.World.Alive(delivered.Actor) {
		t.Fatal("hauler still alive after delivery")
	}
	// Stock: exactly +100 units (first delivery: 500 -> 600).
	qty, price := marketQtyIn(s, "sys-a", "min-ferrite")
	if qty != 600 {
		t.Fatalf("stock = %d, want 600 (exact integer units)", qty)
	}
	// Price: one supply step down: share = 1000/(500*10) = 0.2, band = 0.05,
	// k = share*band/2.5 = 0.004 -> 10 * 0.996 = 9.96.
	if diff := price - 9.96; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("price = %v, want 9.96", price)
	}
	if !hasEvent(evs, EvMarket, "price falls to") {
		t.Fatalf("no price-falls market event; events=%v", trafficTexts(evs))
	}
}

// TestTrafficSpawnRateBySecurity pins the spawn-rate derivation with exact
// counts: authored 600/900-tick routes over 36,000 ticks yield 100
// departures in high security (x1) and 50 in low security (x2).
func TestTrafficSpawnRateBySecurity(t *testing.T) {
	departures := func(t *testing.T, sec string) int {
		t.Helper()
		c := trafficContent()
		if sec == "low" {
			// Attacks lengthen lifecycles; the rate itself is what is pinned
			// here, so disarm the roll by removing the raider class.
			c.Traffic["sys-b"].Raider = ""
		}
		s, err := New(c, testWorld(), 11)
		if err != nil {
			t.Fatal(err)
		}
		if sec == "low" {
			if err := s.LoadSystem("sys-b", ""); err != nil {
				t.Fatal(err)
			}
		}
		var evs []Event
		for i := 0; i < 36000; i += 500 {
			stepCollect(s, &evs, 500)
		}
		return countPhase(evs, EvTraffic, TrafficDeparted)
	}
	if got := departures(t, "high"); got != 100 {
		t.Errorf("high-sec departures = %d, want 100 (600+900 tick routes x1)", got)
	}
	if got := departures(t, "low"); got != 50 {
		t.Errorf("low-sec departures = %d, want 50 (1200+1800 effective tick routes x2)", got)
	}
}

// TestTrafficLiveCapEnforced hammers the schedules and proves the live cap
// holds (<= 6 haulers, <= 4 patrols) while still spawning.
func TestTrafficLiveCapEnforced(t *testing.T) {
	c := trafficContent()
	for _, td := range c.Traffic {
		for i := range td.Routes {
			td.Routes[i].IntervalTicks = 60
		}
		if td.Patrol != nil {
			td.Patrol.IntervalTicks = 60
		}
	}
	s, err := New(c, testWorld(), 5)
	if err != nil {
		t.Fatal(err)
	}
	var evs []Event
	maxHaulers, maxPatrols := 0, 0
	for i := 0; i < 3000; i++ {
		s.Step()
		evs = append(evs, drain(s)...)
		if n := s.Traffic.Len(); n > maxHaulers {
			maxHaulers = n
		}
		if n := s.Patrols.Len(); n > maxPatrols {
			maxPatrols = n
		}
	}
	if maxHaulers > trafficMaxLive {
		t.Errorf("live haulers peaked at %d, cap %d", maxHaulers, trafficMaxLive)
	}
	if maxHaulers < trafficMaxLive {
		t.Errorf("live haulers peaked at %d, cap never exercised (%d)", maxHaulers, trafficMaxLive)
	}
	if maxPatrols > trafficMaxPatrols {
		t.Errorf("live patrols peaked at %d, cap %d", maxPatrols, trafficMaxPatrols)
	}
	if maxPatrols < 2 {
		t.Errorf("patrol spawns never observed (peak %d)", maxPatrols)
	}
	if countPhase(evs, EvTraffic, TrafficDeparted) == 0 {
		t.Error("no departures observed")
	}
}

// ---------------------------------------------------------------------------
// Ambush probability
// ---------------------------------------------------------------------------

// TestTrafficAttackChanceEmpirical runs real legs through the per-leg roll:
// high security must never roll an ambush; low security must land near the
// pinned 1-in-6 rate (deterministic for the fixed seed, asserted as a band).
func TestTrafficAttackChanceEmpirical(t *testing.T) {
	// High: zero ambushes, ever.
	s, err := New(trafficContent(), testWorld(), 3)
	if err != nil {
		t.Fatal(err)
	}
	var evs []Event
	for i := 0; i < 36000; i += 500 {
		stepCollect(s, &evs, 500)
	}
	if n := countPhase(evs, EvTraffic, TrafficAttacked); n != 0 {
		t.Errorf("high security rolled %d ambushes, want 0", n)
	}
	if departed := countPhase(evs, EvTraffic, TrafficDeparted); departed < 90 {
		t.Errorf("high-sec departures = %d, sanity failed", departed)
	}

	// Low: rolls from the traffic stream at 1-in-6 per leg.
	s2, err := New(trafficContent(), testWorld(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := s2.LoadSystem("sys-b", ""); err != nil {
		t.Fatal(err)
	}
	evs = nil
	for i := 0; i < 36000; i += 500 {
		stepCollect(s2, &evs, 500)
	}
	departed := countPhase(evs, EvTraffic, TrafficDeparted)
	attacked := countPhase(evs, EvTraffic, TrafficAttacked)
	if departed < 45 || departed > 55 {
		t.Errorf("low-sec departures = %d, want ~50", departed)
	}
	// p = 1/6, n ~= 50: mean 8.3, sigma 2.7 — the band is ~5 sigma wide.
	if attacked < 1 || attacked > 22 {
		t.Errorf("low-sec ambushes = %d over %d legs, want near %d (1/6)", attacked, departed, departed/6)
	}
}

// ---------------------------------------------------------------------------
// Outcome branches
// ---------------------------------------------------------------------------

// TestTrafficAttackDestroyedOutcome: a killed hauler resolves the ambush as
// destroyed, leaves a wreck carrying its exact cargo (lootable through the
// normal CmdLoot path), and books the risk premium into the market.
func TestTrafficAttackDestroyedOutcome(t *testing.T) {
	s := newTrafficSim(t)
	var evs []Event
	if !runUntil(t, s, &evs, 900, func() bool {
		_, _, ok := findHauler(s, "cel-a-dock")
		return ok
	}) {
		t.Fatal("no delivering hauler within 900 ticks")
	}
	e, tc, _ := findHauler(s, "cel-a-dock")
	// Force with "null" security: no patrol response, so this pins the pure
	// destroyed branch (raiders kill -> wreck -> break-off).
	forceAmbushOn(t, s, e, tc, "null")
	_, priceBefore := marketQtyIn(s, "sys-a", tc.Item)
	holdBefore := playerCargoCount(s, tc.Item)

	// Kill it with the raiders' damage through the real pipeline.
	s.damage = append(s.damage, pendingDamage{
		src: tc.Attack.Raiders[0], dst: e,
		dmg: content.Damage{EM: 10000}, wrecking: false,
	})
	s.Step()
	evs = append(evs, drain(s)...)

	if firstPhase(evs, EvTraffic, TrafficDestroyed) == nil {
		t.Fatalf("no destroyed event; %v", trafficTexts(evs))
	}
	if s.World.Alive(e) {
		t.Fatal("hauler survived lethal damage")
	}
	// Wreck with the exact cargo, looted through CmdLoot.
	wreck := wreckWithCargo(s, tc.Item)
	if wreck.IsZero() {
		t.Fatal("no wreck left by the destroyed hauler")
	}
	teleport(s, s.player, mustPos(s, wreck))
	s.Queue(CmdLoot{Wreck: wreck})
	s.Step()
	evs = append(evs, drain(s)...)
	if !hasEvent(evs, EvLooted, "Looted") {
		t.Fatalf("CmdLoot failed: %v", trafficTexts(evs))
	}
	if got := playerCargoCount(s, tc.Item) - holdBefore; got != tc.Qty {
		t.Fatalf("player looted %d, want exactly %d", got, tc.Qty)
	}

	// Risk premium: the item's price moved up (bounded by the band).
	_, priceAfter := marketQtyIn(s, "sys-a", tc.Item)
	if priceAfter <= priceBefore {
		t.Fatalf("price %v did not rise after the loss (was %v)", priceAfter, priceBefore)
	}
	if priceAfter > 10*1.05+1e-9 {
		t.Fatalf("price %v above the high-sec band", priceAfter)
	}
	if !hasEvent(evs, EvMarket, "price rises to") {
		t.Fatalf("no risk-premium market event: %v", trafficTexts(evs))
	}

	// The raiders break off ~20 s after the incident resolves. The hauler's
	// Traffic component (which owned the attack record) died with it, so the
	// scheduled break-offs live in trafficLeaves — they must all fire.
	if len(s.trafficLeaves) == 0 {
		t.Fatal("no raider break-off scheduled")
	}
	stepCollect(s, &evs, int(trafficRaidersLeaveSec*TickRate)+120)
	if n := len(s.trafficLeaves); n != 0 {
		t.Errorf("%d raider break-offs still pending after the window", n)
	}
}

// TestTrafficAttackEscapeOutcome: an ambushed hauler with no scrambler in
// front of it burns away, warps clear (escaped), and still delivers.
func TestTrafficAttackEscapeOutcome(t *testing.T) {
	s := newTrafficSim(t)
	var evs []Event
	if !runUntil(t, s, &evs, 900, func() bool {
		_, _, ok := findHauler(s, "cel-a-dock")
		return ok
	}) {
		t.Fatal("no delivering hauler within 900 ticks")
	}
	e, tc, _ := findHauler(s, "cel-a-dock")
	// "null" security: no patrol response — pure raider-vs-hauler escape.
	forceAmbushOn(t, s, e, tc, "null") // raiders carry no ewar — cannot pin
	if !runUntil(t, s, &evs, 8000, func() bool {
		return firstPhase(evs, EvTraffic, TrafficEscaped) != nil &&
			firstPhase(evs, EvTraffic, TrafficDelivered) != nil
	}) {
		t.Fatalf("ambushed hauler never escaped and delivered: %v", trafficTexts(evs))
	}
	if firstPhase(evs, EvTraffic, TrafficDestroyed) != nil {
		t.Fatal("hauler was destroyed; escape branch expected")
	}
}

// TestTrafficAttackSavedOutcome: with every raider destroyed while the
// hauler lives, the ambush resolves as saved and the convoy delivers.
func TestTrafficAttackSavedOutcome(t *testing.T) {
	s := newTrafficSim(t)
	var evs []Event
	if !runUntil(t, s, &evs, 900, func() bool {
		_, _, ok := findHauler(s, "cel-a-dock")
		return ok
	}) {
		t.Fatal("no delivering hauler within 900 ticks")
	}
	e, tc, _ := findHauler(s, "cel-a-dock")
	forceAmbushOn(t, s, e, tc, "null") // no patrol: the test plays the patrol
	// Wipe the raiders (the patrol's job, fast-forwarded).
	for _, r := range tc.Attack.Raiders {
		s.damage = append(s.damage, pendingDamage{
			src: s.player, dst: r, dmg: content.Damage{EM: 100000},
		})
	}
	s.Step()
	evs = append(evs, drain(s)...)
	if tc.Attack.Outcome != TrafficOutcomeSaved {
		t.Fatalf("outcome = %q, want saved", tc.Attack.Outcome)
	}
	if !s.World.Alive(e) {
		t.Fatal("hauler died with its raiders")
	}
	if !runUntil(t, s, &evs, 6000, func() bool {
		return firstPhase(evs, EvTraffic, TrafficDelivered) != nil
	}) {
		t.Fatalf("saved hauler never delivered: %v", trafficTexts(evs))
	}
	if firstPhase(evs, EvTraffic, TrafficDestroyed) != nil {
		t.Fatal("saved hauler was destroyed")
	}
}

// TestTrafficPatrolResponseSavesConvoy: the high-sec response dispatches
// immediately (delay 0, wing floor(3*sigma) = 3), the patrol warps to the
// incident, the raiders switch their guns off the convoy and onto the
// patrol — and the un-pinned hauler warps clear and delivers.
func TestTrafficPatrolResponseSavesConvoy(t *testing.T) {
	s := newTrafficSim(t)
	// Ambush with the pinning pirate so the escape needs the patrol.
	td, _ := s.Content.TrafficFor(s.systemID)
	td.Raider = "npc-rat-pirate"
	var evs []Event
	if !runUntil(t, s, &evs, 900, func() bool {
		_, _, ok := findHauler(s, "cel-a-dock")
		return ok
	}) {
		t.Fatal("no delivering hauler within 900 ticks")
	}
	e, tc, _ := findHauler(s, "cel-a-dock")
	s.startAmbush(e, tc, td, "high")

	// Response: high-sec dispatches this same tick (delay 0).
	s.Step()
	evs = append(evs, drain(s)...)
	resp := firstPhase(evs, EvTraffic, TrafficPatrolResponse)
	if resp == nil {
		t.Fatalf("no patrol response: %v", trafficTexts(evs))
	}
	if resp.Value < 1 || resp.Value > 3 {
		t.Fatalf("response wing = %v, want 1..3", resp.Value)
	}
	responding := 0
	s.Patrols.Each(func(_ ecs.Entity, p *Patrol) {
		if p.State == PatrolRespond {
			responding++
		}
	})
	if responding == 0 {
		t.Fatal("no patrol in respond state")
	}

	// The patrol warps in, locks and opens fire on the raiders — the fight
	// pulls the ambush apart and the hauler warps clear and delivers.
	patrolEngaged := false
	if !runUntil(t, s, &evs, 6000, func() bool {
		s.Patrols.Each(func(pe ecs.Entity, _ *Patrol) {
			if tgt, ok := s.modTarget(pe, SlotRef{Class: content.SlotHigh, Index: 0}); ok {
				for _, r := range tc.Attack.Raiders {
					if tgt == r {
						patrolEngaged = true
					}
				}
			}
		})
		return firstPhase(evs, EvTraffic, TrafficEscaped) != nil &&
			firstPhase(evs, EvTraffic, TrafficDelivered) != nil
	}) {
		t.Fatalf("convoy never escaped and delivered: %v", trafficTexts(evs))
	}
	if !patrolEngaged {
		t.Fatal("patrol never engaged the raiders")
	}
	if firstPhase(evs, EvTraffic, TrafficDestroyed) != nil {
		t.Fatal("convoy was destroyed despite the response")
	}
}

// TestTrafficRaiderDamageRealPipeline proves NPC-vs-NPC combat runs through
// the real lock/activate/turret/damage pipeline: raiders lock the hauler
// (lock progress + EvtTargetLocked), hit it with logged turret damage
// within the hull's resist ceiling, and drain its layers.
func TestTrafficRaiderDamageRealPipeline(t *testing.T) {
	s := newTrafficSim(t)
	// Pinning raider + no patrol response: raiders stay on the hauler
	// (scram keeps it from warping out) so lock and damage are observable.
	td, _ := s.Content.TrafficFor(s.systemID)
	td.Raider = "npc-rat-pirate"
	var evs []Event
	if !runUntil(t, s, &evs, 900, func() bool {
		_, _, ok := findHauler(s, "")
		return ok
	}) {
		t.Fatal("no hauler within 900 ticks")
	}
	e, tc, _ := findHauler(s, "")
	s.startAmbush(e, tc, td, "null")
	beforeShield, beforeArmor, beforeStruct := shipVitals(s, e)

	locked, damaged := false, false
	if !runUntil(t, s, &evs, 4000, func() bool {
		if !s.World.Alive(e) {
			return false // delivered/despawned: vitals would read a false drop
		}
		for _, ev := range evs {
			if ev.Kind == EvTargetLocked && ev.Target == e {
				locked = true
			}
		}
		sh, ar, st := shipVitals(s, e)
		if sh+ar+st < beforeShield+beforeArmor+beforeStruct {
			damaged = true
		}
		return locked && damaged
	}) {
		t.Fatalf("raiders never locked AND damaged the hauler (locked=%v damaged=%v)", locked, damaged)
	}
	// Hit lines come from systemDamage ("<raider> hits <hauler> for N").
	// Ceiling: one volley (12 raw) at wrecking x3 through the 0.1 resist ->
	// 32.4 penetrating.
	ceiling := 36.0
	sawHit := false
	for _, ev := range evs {
		if ev.Kind != EvLog || ev.Target != e || ev.Value <= 0 {
			continue
		}
		if !s.AIs.Has(ev.Actor) {
			continue
		}
		if ev.Value > ceiling {
			t.Fatalf("hit for %v exceeds the fixture volley ceiling %v (%q)", ev.Value, ceiling, ev.Text)
		}
		sawHit = true
	}
	if !sawHit {
		t.Fatal("no turret hit events observed in the ambush")
	}
}

// TestTrafficPatrolAnchorSafety pins the guard geometry: patrol homes sit
// 20-30 km east of the anchor station, always outside the 8 km NPC guard
// radius — station traffic (undock/respawn at +5 km) can never trip them.
func TestTrafficPatrolAnchorSafety(t *testing.T) {
	s := newTrafficSim(t)
	var evs []Event
	if !runUntil(t, s, &evs, 1200, func() bool { return s.Patrols.Len() > 0 }) {
		t.Fatal("no patrol spawned within 1200 ticks")
	}
	station := entityByName(s, "Haven Rest")
	sp, _ := s.posOf(station)
	respawn := sp.Add(core.Vec2{X: 5000})
	for _, e := range sortedEntities(s.Patrols) {
		ai, ok := s.AIs.Get(e)
		if !ok {
			t.Fatalf("patrol %v has no NPC state", e)
		}
		if d := ai.Home.Dist(sp); d < 15000 || d > 35000 {
			t.Errorf("patrol home at %v m from station, want 15..35 km", d)
		}
		if ai.Home.Dist(respawn) <= npcGuardRange {
			t.Errorf("patrol home within the %v m guard radius of the station respawn point", npcGuardRange)
		}
	}
}

// TestTrafficPlayerCanEngageConvoy: traffic ships are ordinary entities —
// the player locks and shoots a hauler with the normal combat commands
// (real turret hits observed), and the destroyed ship leaves its cargo in a
// wreck.
func TestTrafficPlayerCanEngageConvoy(t *testing.T) {
	s := newTrafficSim(t)
	var evs []Event
	if !runUntil(t, s, &evs, 900, func() bool {
		_, _, ok := findHauler(s, "cel-a-dock")
		return ok
	}) {
		t.Fatal("no hauler within 900 ticks")
	}
	e, tc, _ := findHauler(s, "cel-a-dock")
	teleport(s, s.player, mustPos(s, e))
	// Shoot it down with real turret cycles: the lock is commanded normally
	// (CmdLock) and forced to completion so the first volley lands before the
	// convoy reaches its destination; follow the hauler every tick because
	// convoys warp away and locks drop outside MaxRange.
	sawHit := false
	for i := 0; i < 6000 && !sawHit; i++ {
		if !s.World.Alive(e) {
			break
		}
		teleport(s, s.player, mustPos(s, e))
		s.Queue(CmdLock{Target: e})
		forceLock(s, e) // the queued CmdLock lands next Step; pre-force progress
		s.Queue(CmdActivate{Slot: SlotRef{Class: content.SlotHigh, Index: 0}, Target: e})
		s.Step()
		evs = append(evs, drain(s)...)
		for _, ev := range evs {
			if ev.Kind == EvLog && ev.Actor == s.player && ev.Target == e && ev.Value > 0 {
				sawHit = true
			}
		}
	}
	if s.World.Alive(e) {
		// Turret pace is slow; finish through the damage pipeline (the hit
		// events above already proved the player's gun reaches the hull).
		s.damage = append(s.damage, pendingDamage{src: s.player, dst: e,
			dmg: content.Damage{EM: 100000}})
		s.Step()
		evs = append(evs, drain(s)...)
	}
	if !sawHit {
		t.Fatal("player's turret never hit the hauler")
	}
	if firstPhase(evs, EvTraffic, TrafficDestroyed) == nil {
		t.Fatalf("player kill produced no traffic destroyed event: %v", trafficTexts(evs))
	}
	wreck := wreckWithCargo(s, tc.Item)
	if wreck.IsZero() {
		t.Fatal("no wreck carrying the convoy cargo")
	}
}

// ---------------------------------------------------------------------------
// Market
// ---------------------------------------------------------------------------

// TestTrafficMarketDriftMath pins both drift directions, the band clamps and
// the stock exactness through direct step applications.
func TestTrafficMarketDriftMath(t *testing.T) {
	s := newTrafficSim(t)
	idx := s.marketIndex("min-ferrite")
	if idx < 0 {
		t.Fatal("min-ferrite not listed in sys-a")
	}
	base := s.marketBasePrice("sys-a", "min-ferrite") // authored order price 10
	if base != 10 {
		t.Fatalf("base = %v, want 10", base)
	}
	band := trafficBand("high") // 0.05

	// Delivery step: price' = price * (1 - share*band/2.5); share 0.2 ->
	// 10 -> 9.96.
	s.marketPriceStep("sys-a", idx, false, 0.2, "high", "Test")
	if p := s.market["sys-a"][idx].Price; !closeTo(p, 9.96) {
		t.Fatalf("after supply step price = %v, want 9.96", p)
	}
	// Attack step doubles the coefficient: 9.96 * (1 + 2*0.2*band/2.5).
	s.marketPriceStep("sys-a", idx, true, 0.2, "high", "Test")
	want := 9.96 * (1 + 0.2*2*band/trafficDriftDiv)
	if p := s.market["sys-a"][idx].Price; !closeTo(p, want) {
		t.Fatalf("after premium step price = %v, want %v", p, want)
	}
	// Clamps: hammer both directions — price never leaves the band and pins
	// exactly to its edge.
	for i := 0; i < 100; i++ {
		s.marketPriceStep("sys-a", idx, true, 1, "high", "Test")
	}
	hi := base * (1 + band)
	if p := s.market["sys-a"][idx].Price; !closeTo(p, hi) {
		t.Fatalf("clamped high price = %v, want band edge %v", p, hi)
	}
	for i := 0; i < 200; i++ {
		s.marketPriceStep("sys-a", idx, false, 1, "high", "Test")
	}
	lo := base * (1 - band)
	if p := s.market["sys-a"][idx].Price; !closeTo(p, lo) {
		t.Fatalf("clamped low price = %v, want band edge %v", p, lo)
	}
	// Stock stays an exact integer through steps (drift never moves units).
	if q, _ := marketQtyIn(s, "sys-a", "min-ferrite"); q != 500 {
		t.Fatalf("stock = %d, want 500", q)
	}
}

// TestTrafficMarketDecayHalving pins the premium decay schedule: after
// exactly 3600 ticks the deviation from base halves; after 7200 it is a
// quarter (silent — only traffic-driven moves emit events).
func TestTrafficMarketDecayHalving(t *testing.T) {
	c := trafficContent()
	// One manually launched delivery, then only the non-delivering route, so
	// no further supply steps interleave with the decay schedule.
	c.Traffic["sys-a"].Routes = c.Traffic["sys-a"].Routes[1:]
	s, err := New(c, testWorld(), 9)
	if err != nil {
		t.Fatal(err)
	}
	td, _ := s.Content.TrafficFor("sys-a")
	s.spawnTrafficHauler(s.System(), td, content.RouteDef{
		From: "cel-station-a", To: "cel-a-dock", Hull: "ship-convoy",
		Cargo:         content.RouteCargo{Item: "min-ferrite", Qty: 100},
		IntervalTicks: 1 << 30, // never on any reachable schedule
	})

	var evs []Event
	if !runUntil(t, s, &evs, 3000, func() bool {
		return firstPhase(evs, EvTraffic, TrafficDelivered) != nil
	}) {
		t.Fatal("seed delivery never landed")
	}
	const base = 10.0
	if _, p := marketQtyIn(s, "sys-a", "min-ferrite"); !closeTo(p, 9.96) {
		t.Fatalf("post-delivery price = %v, want 9.96", p)
	}

	// Step exactly to the first decay boundary (ticks+1 divisible by 3600):
	// after that Step, Ticks() == 3600 and the premium halved once.
	for s.Ticks()%marketDecayTicks != 0 {
		s.Step()
		drain(s)
	}
	if _, p := marketQtyIn(s, "sys-a", "min-ferrite"); !closeTo(p, base+(9.96-base)*0.5) {
		t.Fatalf("after first halving price = %v, want 9.98", p)
	}
	for s.Ticks() != 2*marketDecayTicks {
		s.Step()
		drain(s)
	}
	if _, p := marketQtyIn(s, "sys-a", "min-ferrite"); !closeTo(p, base+(9.96-base)*0.25) {
		t.Fatalf("after second halving price = %v, want 9.99", p)
	}
}

// TestTrafficMarketSoakBounded pushes repeated drift steps through 1000
// ticks: prices must never leave the security band and stock must stay an
// exact integer throughout.
func TestTrafficMarketSoakBounded(t *testing.T) {
	// Only the non-delivering route: the soak pins band bounds and stock
	// exactness under drift pressure, not under live deliveries.
	c := trafficContent()
	c.Traffic["sys-a"].Routes = c.Traffic["sys-a"].Routes[1:]
	s, err := New(c, testWorld(), 13)
	if err != nil {
		t.Fatal(err)
	}
	idx := s.marketIndex("min-ferrite")
	base := s.marketBasePrice("sys-a", "min-ferrite")
	band := trafficBand("high")
	for i := 0; i < 1000; i++ {
		if i%7 == 0 {
			s.marketPriceStep("sys-a", idx, i%2 == 0, 1, "high", "Soak")
		}
		s.Step()
		drain(s)
		p := s.market["sys-a"][idx].Price
		if p < base*(1-band)-1e-9 || p > base*(1+band)+1e-9 {
			t.Fatalf("tick %d: price %v outside band [%v, %v]", i, p,
				base*(1-band), base*(1+band))
		}
		if q, _ := marketQtyIn(s, "sys-a", "min-ferrite"); q != 500 {
			t.Fatalf("tick %d: stock %d is not the exact integer 500", i, q)
		}
	}
}

// ---------------------------------------------------------------------------
// Persistence and determinism
// ---------------------------------------------------------------------------

// TestTrafficPersistenceMidWarpAndMidAttack round-trips save/restore while a
// hauler is inside the warp tunnel and while an ambush (raiders + patrol
// response + scheduled break-offs) is live: hashes match exactly and both
// continuations stay in lockstep with identical event sequences.
func TestTrafficPersistenceMidWarpAndMidAttack(t *testing.T) {
	continuation := func(t *testing.T, a *Sim, save *Save, ticks int) {
		t.Helper()
		b, err := New(trafficContent(), testWorld(), 7)
		if err != nil {
			t.Fatal(err)
		}
		if err := b.Restore(save); err != nil {
			t.Fatalf("restore: %v", err)
		}
		if a.Hash() != b.Hash() {
			t.Fatalf("hash after restore = %016x, want %016x", b.Hash(), a.Hash())
		}
		var evA, evB []Event
		for i := 0; i < ticks; i++ {
			a.Step()
			b.Step()
			evA = append(evA, drain(a)...)
			evB = append(evB, drain(b)...)
		}
		if a.Hash() != b.Hash() {
			t.Fatalf("continuation diverged: %016x vs %016x", a.Hash(), b.Hash())
		}
		if len(evA) != len(evB) {
			t.Fatalf("event sequences differ in length: %d vs %d", len(evA), len(evB))
		}
		for i := range evA {
			if evA[i].Kind != evB[i].Kind || evA[i].Phase != evB[i].Phase ||
				evA[i].Text != evB[i].Text || evA[i].Time != evB[i].Time {
				t.Fatalf("event %d differs: %+v vs %+v", i, evA[i], evB[i])
			}
		}
	}

	// Mid-warp: catch a hauler inside the tunnel.
	t.Run("mid-warp", func(t *testing.T) {
		s := newTrafficSim(t)
		var evs []Event
		if !runUntil(t, s, &evs, 900, func() bool {
			for _, e := range sortedEntities(s.Traffic) {
				if wd, ok := s.Warps.Get(e); ok && wd.Phase == "warp" {
					return true
				}
			}
			return false
		}) {
			t.Fatal("no hauler in the tunnel within 900 ticks")
		}
		save, err := s.Snapshot()
		if err != nil {
			t.Fatalf("snapshot: %v", err)
		}
		continuation(t, s, save, 900)
	})

	// Mid-attack: raiders engaged, patrol responding, break-offs scheduled.
	t.Run("mid-attack", func(t *testing.T) {
		s := newTrafficSim(t)
		var evs []Event
		if !runUntil(t, s, &evs, 900, func() bool {
			_, _, ok := findHauler(s, "cel-a-dock")
			return ok
		}) {
			t.Fatal("no hauler within 900 ticks")
		}
		e, tc, _ := findHauler(s, "cel-a-dock")
		forceAmbushOn(t, s, e, tc, "high")
		stepCollect(s, &evs, 600) // raiders locked/firing, patrol warp launched

		if tc.Attack == nil {
			t.Fatal("no live attack to persist")
		}
		save, err := s.Snapshot()
		if err != nil {
			t.Fatalf("snapshot: %v", err)
		}
		// Round-trip through plain JSON too (the on-disk format).
		raw, err := json.Marshal(save)
		if err != nil {
			t.Fatal(err)
		}
		var decoded Save
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		b, err := New(trafficContent(), testWorld(), 7)
		if err != nil {
			t.Fatal(err)
		}
		if err := b.Restore(&decoded); err != nil {
			t.Fatalf("restore(json): %v", err)
		}
		if b.Hash() != s.Hash() {
			t.Fatalf("hash after json round-trip = %016x, want %016x", b.Hash(), s.Hash())
		}
		_, rtc, _ := findHauler(b, "cel-a-dock")
		if rtc == nil || rtc.Attack == nil {
			t.Fatal("restored sim lost the attack state")
		}
		if !reflect.DeepEqual(rtc.Attack, tc.Attack) {
			t.Fatalf("attack state differs:\n%+v\n%+v", tc.Attack, rtc.Attack)
		}
		if len(b.trafficLeaves) != len(s.trafficLeaves) {
			t.Fatalf("traffic leaves = %d, want %d", len(b.trafficLeaves), len(s.trafficLeaves))
		}
		continuation(t, s, save, 1200)
	})
}

// TestTrafficTwinRunsIdentical: same content, world and seed => identical
// hashes at checkpoints AND identical event sequences, with forced ambushes
// in the mix.
func TestTrafficTwinRunsIdentical(t *testing.T) {
	run := func() ([]Event, []uint64) {
		s := newTrafficSimSeed(t, 21)
		var evs []Event
		var hashes []uint64
		for i := 0; i < 9000; i++ {
			s.Step()
			evs = append(evs, drain(s)...)
			if i == 1500 || i == 4500 {
				if e, tc, ok := findHauler(s, ""); ok {
					forceAmbushOn(t, s, e, tc, "high")
				}
			}
			if i%1500 == 0 {
				hashes = append(hashes, s.Hash())
			}
		}
		hashes = append(hashes, s.Hash())
		return evs, hashes
	}
	evA, hashA := run()
	evB, hashB := run()
	if len(hashA) != len(hashB) {
		t.Fatal("hash checkpoint counts differ")
	}
	for i := range hashA {
		if hashA[i] != hashB[i] {
			t.Fatalf("checkpoint %d: %016x vs %016x", i, hashA[i], hashB[i])
		}
	}
	if len(evA) != len(evB) {
		t.Fatalf("event counts differ: %d vs %d", len(evA), len(evB))
	}
	for i := range evA {
		a, b := evA[i], evB[i]
		if a.Kind != b.Kind || a.Phase != b.Phase || a.Text != b.Text ||
			a.Time != b.Time || a.System != b.System || a.Value != b.Value {
			t.Fatalf("event %d differs:\n%+v\n%+v", i, a, b)
		}
	}
	if countPhase(evA, EvTraffic, TrafficAttacked) == 0 {
		t.Fatal("twin run never exercised an ambush")
	}
}

// TestTrafficNeverDrawsMasterStream is the RNG-isolation regression: a sim
// running the living universe must leave the master stream bit-identical to
// a sim without traffic — convoy spawns, ambush rolls and patrol scatter all
// draw from the forked traffic stream, never the player's.
func TestTrafficNeverDrawsMasterStream(t *testing.T) {
	plain, err := New(testContent(), testWorld(), 7)
	if err != nil {
		t.Fatal(err)
	}
	living := newTrafficSimSeed(t, 7)

	// Same starting master state (traffic content adds no RNG draws).
	if plain.RNG.Snapshot() != living.RNG.Snapshot() {
		t.Fatal("master streams differ at t=0")
	}
	for i := 0; i < 3000; i++ {
		plain.Step()
		living.Step()
		drain(plain)
		drain(living)
	}
	if living.Traffic.Len() == 0 {
		t.Fatal("no convoys spawned — the traffic system never ran")
	}
	if living.Patrols.Len() == 0 {
		t.Fatal("no patrols spawned — the traffic system never ran")
	}
	if plain.RNG.Snapshot() != living.RNG.Snapshot() {
		t.Fatalf("traffic drew from the master stream: %016x vs %016x",
			living.RNG.Snapshot(), plain.RNG.Snapshot())
	}
}

// closeTo compares floats at the 1e-9 level.
func closeTo(a, b float64) bool {
	d := a - b
	return d < 1e-9 && d > -1e-9
}
