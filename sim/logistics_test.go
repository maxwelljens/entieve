package sim

import (
	"math"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
)

// ---------------------------------------------------------------------------
// Logistics acceptance: real cargo provenance (sourced at spawn), the
// cross-system journey (spawn A → gate hop → deliver B), loss honesty
// (debits are never reversed), the shipped ring lanes, and determinism
// with routes active. The calibration/rate/ambush pins stay in
// traffic_test.go.
// ---------------------------------------------------------------------------

// crossRoute is the fixture's cross-system lane: Haven Rest (sys-a) →
// Dock Beta (sys-b), through the sys-a gate. Interval never fires — the
// tests spawn it by hand.
func crossRoute() content.RouteDef {
	return content.RouteDef{
		From: "cel-station-a", To: "cel-b-dock", ToSystem: "sys-b",
		Hull:          "ship-convoy",
		Cargo:         content.RouteCargo{Item: "min-ferrite", Qty: 100},
		IntervalTicks: 1 << 30,
	}
}

// crossContent extends the traffic fixture with sys-b's delivery station
// (no founding orders: the destination book is created by the delivery)
// and silences the scheduled station→station route, so the provenance
// tests observe exactly their own shipment's debits.
func crossContent() *content.Content {
	c := trafficContent()
	c.Traffic["sys-a"].Routes = c.Traffic["sys-a"].Routes[1:]
	sysB := c.Systems["sys-b"]
	sysB.Celestials = append(sysB.Celestials, content.CelestialDef{
		ID: "cel-b-dock", Name: "Dock Beta", Kind: content.CelStation,
		Position: core.Vec2{X: 1.5e7, Y: 1e7}, Radius: 500,
		Station: &content.StationDef{Services: []string{"repair", "market"}},
	})
	return c
}

// beltVolume sums a system's persisted belt-state volume (m³).
func beltVolume(s *Sim, sys content.SystemID) float64 {
	total := 0.0
	for _, k := range s.beltKeys() {
		if k.System != sys {
			continue
		}
		for _, r := range s.belts[k].Rocks {
			total += r.Remaining
		}
	}
	return total
}

// ---------------------------------------------------------------------------
// Provenance
// ---------------------------------------------------------------------------

// TestLogisticsStationOriginDebitsAtSpawn pins rule 1 (station origin):
// the cargo leaves the origin book WHEN THE HAULER SPAWNS, the shipment
// carries its paid basis, and the delivery in the far corner posts without
// touching the origin again (no delivery-time second debit, no reversal).
func TestLogisticsStationOriginDebitsAtSpawn(t *testing.T) {
	c := trafficContent()
	c.Traffic["sys-a"].Routes = c.Traffic["sys-a"].Routes[1:] // schedule quiet
	s, err := New(c, testWorld(), 9)
	if err != nil {
		t.Fatal(err)
	}
	td, _ := s.Content.TrafficFor("sys-a")

	before, price := askAt(s, "cel-station-a", "min-ferrite")
	if before != 500 || price != 10 {
		t.Fatalf("fixture ask = %d @ %v, want 500 @ 10", before, price)
	}
	s.spawnTrafficHauler(s.System(), td, crossRouteFromStation())
	after, price2 := askAt(s, "cel-station-a", "min-ferrite")
	if after != 400 || price2 != 10 {
		t.Fatalf("origin ask at spawn = %d @ %v, want 400 @ 10 (sourced at spawn)", after, price2)
	}
	e, tc, ok := findHauler(s, "cel-a-dock")
	if !ok {
		t.Fatal("no hauler spawned")
	}
	if tc.Basis != 10 {
		t.Fatalf("basis = %v, want 10 (the weighted price paid at spawn)", tc.Basis)
	}
	if cargo, _ := s.Cargos.Get(e); stacksCount(cargo.Stacks, "min-ferrite") != 100 {
		t.Fatal("hauler does not carry the withdrawn units")
	}

	var evs []Event
	if !runUntil(t, s, &evs, 3000, func() bool {
		return firstPhase(evs, EvTraffic, TrafficDelivered) != nil
	}) {
		t.Fatalf("no delivery; %v", trafficTexts(evs))
	}
	// Delivery posts at the destination and never debits the origin again.
	if q, p := askAt(s, "cel-station-a", "min-ferrite"); q != 400 || p != 10 {
		t.Fatalf("origin ask after delivery = %d @ %v, want 400 @ 10 (delivery-time debit removed)", q, p)
	}
	if q, p := askAt(s, "cel-a-dock", "min-ferrite"); q != 100 || p != 9.99 {
		t.Fatalf("destination ask = %d @ %v, want 100 @ 9.99 (one tick under the basis)", q, p)
	}
}

// crossRouteFromStation is the deliverable in-system station→station lane
// used by the provenance tests (never on any schedule).
func crossRouteFromStation() content.RouteDef {
	return content.RouteDef{
		From: "cel-station-a", To: "cel-a-dock", Hull: "ship-convoy",
		Cargo:         content.RouteCargo{Item: "min-ferrite", Qty: 100},
		IntervalTicks: 1 << 30,
	}
}

// TestLogisticsStationOriginShortageSpawnsNothing pins the "never
// conjure" half of rule 1: a station without the full quantity spawns no
// shipment this cycle (event instead), and not even a partial withdrawal
// happens — the resting ask stays whole.
func TestLogisticsStationOriginShortageSpawnsNothing(t *testing.T) {
	c := trafficContent()
	c.Traffic["sys-a"].Routes = c.Traffic["sys-a"].Routes[1:]
	s, err := New(c, testWorld(), 9)
	if err != nil {
		t.Fatal(err)
	}
	td, _ := s.Content.TrafficFor("sys-a")

	// Zero stock: Haven Rest never lists sterngrit ore.
	var evs []Event
	s.spawnTrafficHauler(s.System(), td, content.RouteDef{
		From: "cel-station-a", To: "cel-a-dock", Hull: "ship-convoy",
		Cargo:         content.RouteCargo{Item: "ore-sterngrit", Qty: 50},
		IntervalTicks: 1 << 30,
	})
	evs = append(evs, drain(s)...)
	if _, _, ok := findHauler(s, ""); ok {
		t.Fatal("haulers exist although the route never spawned (scheduled residue)")
	}
	if !hasEvent(evs, EvLog, "no Sterngrit to ship from Haven Rest") {
		t.Fatalf("missing no-stock event: %v", trafficTexts(evs))
	}

	// Partial stock: Dock Two holds 50 units against a 100-unit shipment.
	s.PlaceOrder("cel-a-dock", OrderSell, "min-ferrite", 50, 12, OwnerSystem)
	drain(s)
	s.spawnTrafficHauler(s.System(), td, content.RouteDef{
		From: "cel-a-dock", To: "cel-gate-b", Hull: "ship-convoy",
		Cargo:         content.RouteCargo{Item: "min-ferrite", Qty: 100},
		IntervalTicks: 1 << 30,
	})
	evs = drain(s)
	if _, _, ok := findHauler(s, ""); ok {
		t.Fatal("shipment spawned without full origin stock")
	}
	if !hasEvent(evs, EvLog, "no Ferrite to ship from Dock Two") {
		t.Fatalf("missing no-stock event: %v", trafficTexts(evs))
	}
	if q, p := askAt(s, "cel-a-dock", "min-ferrite"); q != 50 || p != 12 {
		t.Fatalf("origin ask = %d @ %v, want 50 @ 12 (no partial withdrawal)", q, p)
	}
}

// TestLogisticsBeltOriginDrawsBeltState pins rule 2 (belt origin): the
// shipment's volume leaves the persistent belt state at spawn, and a dry
// belt spawns nothing.
func TestLogisticsBeltOriginDrawsBeltState(t *testing.T) {
	s, err := New(trafficContent(), testWorld(), 9)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.LoadSystem("sys-b", ""); err != nil {
		t.Fatal(err)
	}
	td, _ := s.Content.TrafficFor("sys-b")

	before := beltVolume(s, "sys-b")
	if before <= 0 {
		t.Fatal("fixture belt not seeded")
	}
	// The shipped fixture lane: Beta Belt → Gate to Alpha, 50 units of a
	// 0.1 m³ item = 5 m³ out of the world.
	s.spawnTrafficHauler(s.System(), td, td.Routes[1])
	if _, _, ok := findHauler(s, "cel-gate-a"); !ok {
		t.Fatal("belt-origin hauler never spawned")
	}
	if got := beltVolume(s, "sys-b"); math.Abs(got-(before-5)) > 1e-9 {
		t.Fatalf("belt volume = %v, want %v (spawn drew the cargo)", got, before-5)
	}

	// Dry belt: drain the state, no shipment.
	for _, k := range s.beltKeys() {
		for i := range s.belts[k].Rocks {
			s.belts[k].Rocks[i].Remaining = 0
		}
	}
	drain(s)
	s.spawnTrafficHauler(s.System(), td, td.Routes[1])
	evs := drain(s)
	if n := s.Traffic.Len(); n != 1 { // only the still-flying first hauler
		t.Fatalf("live haulers = %d, want 1 (dry belt spawned nothing)", n)
	}
	if !hasEvent(evs, EvLog, "to ship from Beta Belt") {
		t.Fatalf("missing dry-belt event: %v", trafficTexts(evs))
	}
}

// TestLogisticsGateOriginImportUnchanged pins rule 1's import boundary: a
// gate origin (the world edge) spawns unconditionally, debits no book
// anywhere, carries the base price as its basis, and posts as supply at
// the destination exactly like before.
func TestLogisticsGateOriginImportUnchanged(t *testing.T) {
	c := trafficContent()
	c.Traffic["sys-a"].Routes = c.Traffic["sys-a"].Routes[1:]
	s, err := New(c, testWorld(), 9)
	if err != nil {
		t.Fatal(err)
	}
	td, _ := s.Content.TrafficFor("sys-a")
	havenBefore, _ := askAt(s, "cel-station-a", "ammo-emp-s")
	dockBefore, _ := askAt(s, "cel-a-dock", "ammo-emp-s")

	s.spawnTrafficHauler(s.System(), td, content.RouteDef{
		From: "cel-gate-b", To: "cel-a-dock", Hull: "ship-convoy",
		Cargo:         content.RouteCargo{Item: "ammo-emp-s", Qty: 600},
		IntervalTicks: 1 << 30,
	})
	e, tc, ok := findHauler(s, "cel-a-dock")
	if !ok {
		t.Fatal("gate-origin import never spawned")
	}
	if tc.Basis != 5 {
		t.Fatalf("basis = %v, want 5 (import prices at the def base price)", tc.Basis)
	}
	if h, _ := askAt(s, "cel-station-a", "ammo-emp-s"); h != havenBefore {
		t.Fatal("a gate import debited a station book")
	}
	if d, _ := askAt(s, "cel-a-dock", "ammo-emp-s"); d != dockBefore {
		t.Fatal("a gate import debited the destination book at spawn")
	}
	_ = e

	var evs []Event
	if !runUntil(t, s, &evs, 3000, func() bool {
		return firstPhase(evs, EvTraffic, TrafficDelivered) != nil
	}) {
		t.Fatalf("import never delivered; %v", trafficTexts(evs))
	}
	if q, p := askAt(s, "cel-a-dock", "ammo-emp-s"); q != 600 || p != 4.999 {
		t.Fatalf("delivered import = %d @ %v, want 600 @ 4.999 (one tick under the base)",
			q, p)
	}
}

// TestLogisticsLostShipmentNoReversal pins rule 4: killing a hauler
// destroys the goods it already drew from the origin — the wreck carries
// them (recoverable by looting), the origin book is NOT re-credited, and
// the destination never sees the shipment.
func TestLogisticsLostShipmentNoReversal(t *testing.T) {
	c := trafficContent()
	c.Traffic["sys-a"].Routes = c.Traffic["sys-a"].Routes[1:] // nothing else delivers
	s, err := New(c, testWorld(), 9)
	if err != nil {
		t.Fatal(err)
	}
	td, _ := s.Content.TrafficFor("sys-a")
	s.spawnTrafficHauler(s.System(), td, crossRouteFromStation())
	e, _, ok := findHauler(s, "cel-a-dock")
	if !ok {
		t.Fatal("no hauler")
	}
	if q, _ := askAt(s, "cel-station-a", "min-ferrite"); q != 400 {
		t.Fatalf("origin ask at spawn = %d, want 400", q)
	}

	damageNPC(s, s.player, e, content.Damage{EM: 1e6}, false)
	s.Step()
	evs := drain(s)
	if firstPhase(evs, EvTraffic, TrafficDestroyed) == nil {
		t.Fatalf("no destroyed event; %v", trafficTexts(evs))
	}
	if !hasEvent(evs, EvMarket, "shipment lost en route to") {
		t.Fatalf("no supply-risk event; %v", trafficTexts(evs))
	}
	// No reversal: the origin keeps the debit.
	if q, p := askAt(s, "cel-station-a", "min-ferrite"); q != 400 || p != 10 {
		t.Fatalf("origin ask after loss = %d @ %v, want 400 @ 10 (no reversal)", q, p)
	}
	// The goods exist as a wreck, lootable through the normal path.
	wreck := wreckWithCargo(s, "min-ferrite")
	if wreck.IsZero() {
		t.Fatal("lost shipment left no wreck carrying the cargo")
	}
	// The destination never saw it.
	if q, _ := askAt(s, "cel-a-dock", "min-ferrite"); q != 0 {
		t.Fatalf("destination ask = %d, want 0 (nothing arrived)", q)
	}
}

// ---------------------------------------------------------------------------
// Cross-system journey
// ---------------------------------------------------------------------------

// TestLogisticsCrossSystemJourney pins the whole cross-system haul:
// sourced at spawn in A → warp the first leg → gate hop parks the shipment
// (jumped event, journey ledger) → the player's jump loads B, where the
// hauler resumes at the entry gate (departed) → delivers into B's book at
// the spawn-time basis, while A's origin book stays debited exactly once.
func TestLogisticsCrossSystemJourney(t *testing.T) {
	c := crossContent()
	s, err := New(c, testWorld(), 9)
	if err != nil {
		t.Fatal(err)
	}
	// Park the player away from the lanes; disarm sys-b's raider so the
	// continuation leg is observable without an ambush (ambushes have their
	// own pins in traffic_test.go).
	setPlayerPos(s, core.Vec2{X: -5e6, Y: -5e6})
	c.Traffic["sys-b"].Raider = ""
	drain(s)
	td, _ := s.Content.TrafficFor("sys-a")

	var evs []Event
	s.spawnTrafficHauler(s.System(), td, crossRoute())
	if q, _ := askAt(s, "cel-station-a", "min-ferrite"); q != 400 {
		t.Fatalf("origin ask at spawn = %d, want 400", q)
	}
	// First leg: toward the sys-a gate.
	e, tc, ok := findHauler(s, "")
	if !ok {
		t.Fatal("no hauler spawned")
	}
	if tc.Dest != "cel-gate-b" || tc.DestSys != "sys-b" || tc.DestCel != "cel-b-dock" {
		t.Fatalf("leg1 = dest %q sys %q cel %q, want cel-gate-b/sys-b/cel-b-dock",
			tc.Dest, tc.DestSys, tc.DestCel)
	}

	// Hop: the shipment parks for sys-b when the hauler reaches the gate.
	if !runUntil(t, s, &evs, 3000, func() bool {
		return len(s.trafficJourneys) == 1
	}) {
		t.Fatalf("shipment never hopped; %v", trafficTexts(evs))
	}
	hop := firstPhase(evs, EvTraffic, TrafficJumped)
	if hop == nil || hop.System != "sys-a" || hop.Cel != "cel-gate-b" {
		t.Fatalf("no jumped event in sys-a at the gate: %v", trafficTexts(evs))
	}
	if s.World.Alive(e) {
		t.Fatal("hauler survived its gate hop")
	}
	j := s.trafficJourneys[0]
	if j.System != "sys-b" || j.DestCel != "cel-b-dock" || j.Item != "min-ferrite" ||
		j.Qty != 100 || j.Basis != 10 || j.Hull != "ship-convoy" {
		t.Fatalf("parked journey = %+v", j)
	}

	// The player jumps: sys-b loads, the journey resumes this same tick.
	gate := entityByName(s, "Gate to Beta")
	if gate.IsZero() {
		t.Fatal("no sys-a gate on grid")
	}
	setPlayerPos(s, gatePos(s, gate))
	s.Queue(CmdJump{Gate: gate})
	s.Step()
	evs = append(evs, drain(s)...)
	if s.systemID != "sys-b" {
		t.Fatalf("system = %q, want sys-b", s.systemID)
	}
	// The cross-system journey resumed (no sys-b shipment left parked);
	// journeys bound to OTHER systems — e.g. a sys-a hauler parked by the
	// jump — must stay parked while the player is abroad. Resume is
	// staggered (trafficResumeInterval), so wait for the sys-b ledger to
	// drain rather than pinning the jump tick.
	if !runUntil(t, s, &evs, 600, func() bool {
		for _, j := range s.trafficJourneys {
			if j.System == "sys-b" {
				return false
			}
		}
		return true
	}) {
		t.Fatalf("sys-b journey never resumed; %+v", s.trafficJourneys)
	}
	e2, tc2, ok := findHauler(s, "cel-b-dock")
	if !ok {
		t.Fatalf("resumed hauler missing; %v", trafficTexts(evs))
	}
	if tc2.Basis != 10 || tc2.Qty != 100 {
		t.Fatalf("resumed state = basis %v qty %d, want 10/100", tc2.Basis, tc2.Qty)
	}
	_ = e2
	// (Journeys bound to OTHER systems — e.g. a sys-a hauler parked by the
	// jump — must stay parked while the player is abroad.)

	// Second leg: deliver into sys-b's book at the spawn-time basis.
	if !runUntil(t, s, &evs, 6000, func() bool {
		return firstPhase(evs, EvTraffic, TrafficDelivered) != nil
	}) {
		t.Fatalf("cross-system shipment never delivered; %v", trafficTexts(evs))
	}
	delivered := firstPhase(evs, EvTraffic, TrafficDelivered)
	if delivered.System != "sys-b" || delivered.Cel != "cel-b-dock" {
		t.Fatalf("delivered = %q/%q, want sys-b/cel-b-dock", delivered.System, delivered.Cel)
	}
	if q, p := askAt(s, "cel-b-dock", "min-ferrite"); q != 100 || p != 9.99 {
		t.Fatalf("sys-b book = %d @ %v, want 100 @ 9.99", q, p)
	}
	// A's origin stays debited exactly once — no reversal, no second debit.
	if q, p := askAt(s, "cel-station-a", "min-ferrite"); q != 400 || p != 10 {
		t.Fatalf("origin ask = %d @ %v, want 400 @ 10", q, p)
	}
	// Phase order: departed (A) → jumped (A) → departed (B) → delivered (B).
	var tDepA, tHop, tDepB, tDel float64 = -1, -1, -1, -1
	for i := range evs {
		ev := &evs[i]
		if ev.Kind != EvTraffic {
			continue
		}
		switch {
		case ev.Phase == TrafficDeparted && ev.System == "sys-a" && tDepA < 0:
			tDepA = ev.Time
		case ev.Phase == TrafficJumped && tHop < 0:
			tHop = ev.Time
		case ev.Phase == TrafficDeparted && ev.System == "sys-b" && tDepB < 0:
			tDepB = ev.Time
		case ev.Phase == TrafficDelivered && tDel < 0:
			tDel = ev.Time
		}
	}
	if tDepA < 0 || tHop < 0 || tDepB < 0 || tDel < 0 {
		t.Fatalf("missing phases: departedA=%v hop=%v departedB=%v delivered=%v; %v",
			tDepA, tHop, tDepB, tDel, trafficTexts(evs))
	}
	if !(tDepA < tHop && tHop < tDepB && tDepB < tDel) {
		t.Fatalf("phase order wrong: %v < %v < %v < %v", tDepA, tHop, tDepB, tDel)
	}
}

// TestLogisticsJourneySurvivesSaveRestore round-trips a PARKED shipment
// through Snapshot → JSON → Restore: hashes match and the journey flies on
// in the restored sim.
func TestLogisticsJourneySurvivesSaveRestore(t *testing.T) {
	c := crossContent()
	s, err := New(c, testWorld(), 9)
	if err != nil {
		t.Fatal(err)
	}
	setPlayerPos(s, core.Vec2{X: -5e6, Y: -5e6})
	c.Traffic["sys-b"].Raider = ""
	drain(s)
	td, _ := s.Content.TrafficFor("sys-a")
	s.spawnTrafficHauler(s.System(), td, crossRoute())
	var evs []Event
	if !runUntil(t, s, &evs, 3000, func() bool {
		return len(s.trafficJourneys) == 1
	}) {
		t.Fatal("shipment never hopped")
	}

	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(save.TrafficJourneys) != 1 {
		t.Fatalf("saved journeys = %d, want 1", len(save.TrafficJourneys))
	}
	b, err := New(c, testWorld(), 9)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Restore(save); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if b.Hash() != s.Hash() {
		t.Fatalf("hash after restore = %016x, want %016x", b.Hash(), s.Hash())
	}
	if len(b.trafficJourneys) != 1 || b.trafficJourneys[0] != s.trafficJourneys[0] {
		t.Fatalf("restored journeys = %+v, want %+v", b.trafficJourneys, s.trafficJourneys)
	}
	// The restored sim completes the journey after its own jump: resume is
	// staggered (trafficResumeInterval), so wait for the continuation
	// hauler bound to the sys-b destination instead of pinning the tick.
	gate := entityByName(b, "Gate to Beta")
	setPlayerPos(b, gatePos(b, gate))
	b.Queue(CmdJump{Gate: gate})
	b.Step()
	drain(b)
	if b.systemID != "sys-b" {
		t.Fatalf("restored continuation: system=%q, want sys-b", b.systemID)
	}
	resumed := false
	for i := 0; i < 600 && !resumed; i++ {
		if _, _, ok := findHauler(b, "cel-b-dock"); ok {
			resumed = true
			break
		}
		b.Step()
		drain(b)
	}
	if !resumed {
		t.Fatal("restored journey never resumed in sys-b")
	}
}

// ---------------------------------------------------------------------------
// Shipped lanes (content/data/traffic)
// ---------------------------------------------------------------------------

// TestLogisticsNewLanesAuthoring pins the lane-authoring contract over
// every shipped route: the origin actually holds the stock it ships
// (station asks / belt volume; gate origins are imports and count at most
// one per direction+edge), shipments sit at 5-25% of the origin's founding
// ask, intervals stay in the modest band, no new lane crosses more than 3
// lanes per system pair — and the whole content set validates clean.
func TestLogisticsNewLanesAuthoring(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("load content: %v", err)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("content does not validate: %v", err)
	}
	celIn := func(sys *content.SystemDef, id content.CelID) *content.CelestialDef {
		for i := range sys.Celestials {
			if sys.Celestials[i].ID == id {
				return &sys.Celestials[i]
			}
		}
		return nil
	}
	type pair struct{ a, b content.SystemID }
	imports := map[string]int{}
	lanes := map[pair]int{}
	for _, sys := range sortedTrafficSystems(c) {
		td := c.Traffic[sys]
		sysDef := c.Systems[sys]
		for i, r := range td.Routes {
			ctx := string(sys) + "/routes[" + itoaN(i) + "]"
			if r.IntervalTicks < 1800 || r.IntervalTicks > 14400 {
				t.Errorf("%s: interval %d outside the modest band [1800,14400]", ctx, r.IntervalTicks)
			}
			from := celIn(sysDef, r.From)
			if from == nil {
				t.Errorf("%s: unknown origin %q", ctx, r.From)
				continue
			}
			switch {
			case from.Station != nil:
				stock := 0
				for _, o := range from.Station.Market {
					if o.Item == r.Cargo.Item && o.Side != "buy" {
						stock += o.Qty
					}
				}
				if stock < r.Cargo.Qty {
					t.Errorf("%s: origin %q stocks %d of %s, ships %d (no stock to ship)",
						ctx, from.Name, stock, r.Cargo.Item, r.Cargo.Qty)
					break
				}
				pct := 100 * float64(r.Cargo.Qty) / float64(stock)
				if pct < 5 || pct > 25 {
					t.Errorf("%s: shipment is %.1f%% of the founding ask, want 5..25%%", ctx, pct)
				}
			case from.Belt != nil:
				need := 0.0
				if def, ok := c.Item(r.Cargo.Item); ok {
					need = def.Volume * float64(r.Cargo.Qty)
				}
				have := 0.0
				for _, ad := range from.Belt.Asteroids {
					if ad.Ore == r.Cargo.Item {
						have += ad.Volume * float64(ad.Count)
					}
				}
				if have < need {
					t.Errorf("%s: belt %q holds %v m³ of %s, ships %v m³",
						ctx, from.Name, have, r.Cargo.Item, need)
				}
			case from.Gate != nil:
				// Import boundary: at most one gate-origin lane per
				// (origin system, entry edge) — the edge is where the
				// goods ENTER, i.e. the gate's far side.
				key := string(sys) + "|" + string(from.Gate.ToSystem)
				imports[key]++
				if imports[key] > 1 {
					t.Errorf("%s: more than one import lane on %s", ctx, key)
				}
			default:
				t.Errorf("%s: origin %q is kind %q (want station|belt|gate)", ctx, from.ID, from.Kind)
			}
			if r.ToSystem != "" && r.ToSystem != sys {
				p := pair{sys, r.ToSystem}
				if p.a > p.b {
					p.a, p.b = p.b, p.a
				}
				lanes[p]++
				if lanes[p] > 3 {
					t.Errorf("%s: pair %s<->%s carries %d lanes, want <= 3", ctx, p.a, p.b, lanes[p])
				}
			}
		}
	}
}

// sortedTrafficSystems returns every system with a traffic def, ascending.
func sortedTrafficSystems(c *content.Content) []content.SystemID {
	ids := make([]content.SystemID, 0, len(c.Traffic))
	for id := range c.Traffic {
		ids = append(ids, id)
	}
	sortSystemIDs(ids)
	return ids
}

// TestLogisticsNextHopRouting pins the gate-path walk over the shipped
// Kresy ring: the first hop of every cross-system leg (shortest path,
// ties broken on celestial definition order — deterministic).
func TestLogisticsNextHopRouting(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("load content: %v", err)
	}
	s := &Sim{Content: c}
	cases := []struct {
		from, to, want content.SystemID
		wantGate       content.CelID
	}{
		{"sys-solace", "sys-rudnik", "sys-rudnik", "cel-gate-rudnik"},
		{"sys-solace", "sys-glusza", "sys-glusza", "cel-gate-penumbra"},
		{"sys-mlynow", "sys-kamieniec", "sys-kamieniec", "cel-gate-penumbra"},
		{"sys-glusza", "sys-kamieniec", "sys-kamieniec", "cel-gate-kamieniec"},
		{"sys-kamieniec", "sys-penumbra", "sys-penumbra", "cel-gate-penumbra"},
		{"sys-gloam", "sys-kamieniec", "sys-kamieniec", "cel-gate-penumbra"},
		{"sys-rudnik", "sys-solace", "sys-solace", "cel-gate-solace"},
	}
	for _, tc := range cases {
		if got := s.trafficNextHop(tc.from, tc.to); got != tc.wantGate {
			t.Errorf("nextHop(%s -> %s) = %q, want %q", tc.from, tc.to, got, tc.wantGate)
		}
	}
	// Same system and unknown systems hop nowhere.
	if got := s.trafficNextHop("sys-solace", "sys-solace"); got != "" {
		t.Errorf("nextHop within a system = %q, want \"\"", got)
	}
	if got := s.trafficNextHop("sys-solace", "sys-nowhere"); got != "" {
		t.Errorf("nextHop to unknown system = %q, want \"\"", got)
	}
}

// ringContent extends the cross fixture with a THIRD system hanging off
// sys-b, so a route from sys-a must hop twice (a → b → c) — the shape of
// the shipped ring lanes (e.g. Młynów → Kamieniec via Kuźnica).
func ringContent() *content.Content {
	c := crossContent()
	sysB := c.Systems["sys-b"]
	sysB.Celestials = append(sysB.Celestials, content.CelestialDef{
		ID: "cel-gate-gamma", Name: "Gate to Gamma", Kind: content.CelGate,
		Position: core.Vec2{X: 2e7, Y: -5e6}, Radius: 300,
		Gate: &content.GateDef{ToSystem: "sys-c", ToCel: "cel-gate-beta"},
	})
	c.Systems["sys-c"] = &content.SystemDef{
		ID: "sys-c", Name: "Fixture Gamma", Security: "high",
		Constellation: "Test", Region: "Test",
		Celestials: []content.CelestialDef{
			{ID: "cel-gate-beta", Name: "Gate to Beta", Kind: content.CelGate,
				Position: core.Vec2{}, Radius: 300,
				Gate: &content.GateDef{ToSystem: "sys-b", ToCel: "cel-gate-gamma"}},
			{ID: "cel-c-dock", Name: "Dock Gamma", Kind: content.CelStation,
				Position: core.Vec2{X: 2e7, Y: 5e6}, Radius: 500,
				Station: &content.StationDef{Services: []string{"repair", "market"}}},
		},
	}
	return c
}

// TestLogisticsMultiHopJourney drives the full two-gate journey: spawn in
// A (origin debited) → hop into B → the player follows, the hauler warps
// to B's far gate and hops again → the player follows once more → delivery
// in C at the spawn-time basis, origin debited exactly once.
func TestLogisticsMultiHopJourney(t *testing.T) {
	c := ringContent()
	s, err := New(c, testWorld(), 9)
	if err != nil {
		t.Fatal(err)
	}
	setPlayerPos(s, core.Vec2{X: -5e6, Y: -5e6})
	c.Traffic["sys-b"].Raider = ""
	drain(s)
	td, _ := s.Content.TrafficFor("sys-a")

	route := crossRoute()
	route.To = "cel-c-dock"
	route.ToSystem = "sys-c"
	var evs []Event
	s.spawnTrafficHauler(s.System(), td, route)

	jumpTo := func(sys content.SystemID, gateName string) {
		t.Helper()
		gate := entityByName(s, gateName)
		if gate.IsZero() {
			t.Fatalf("no %q on the %s grid", gateName, s.systemID)
		}
		setPlayerPos(s, gatePos(s, gate))
		s.Queue(CmdJump{Gate: gate})
		s.Step()
		evs = append(evs, drain(s)...)
		if s.systemID != sys {
			t.Fatalf("system = %q, want %q", s.systemID, sys)
		}
	}
	parkIn := func(sys content.SystemID) {
		t.Helper()
		if !runUntil(t, s, &evs, 3000, func() bool {
			for _, j := range s.trafficJourneys {
				if j.System == sys {
					return true
				}
			}
			return false
		}) {
			t.Fatalf("shipment never parked for %s; %v", sys, trafficTexts(evs))
		}
	}

	parkIn("sys-b") // first hop
	jumpTo("sys-b", "Gate to Beta")
	// The continuation aims at sys-b's far gate, not the destination.
	// Resume is capped/staggered (trafficResumeInterval), so the hauler
	// appears within a few schedule windows, not on the jump tick.
	var cont *Traffic
	if !runUntil(t, s, &evs, 600, func() bool {
		_, tc, ok := findHauler(s, "cel-gate-gamma")
		if ok {
			cont = tc
		}
		return ok
	}) {
		t.Fatalf("no continuation hauler in sys-b; %v", trafficTexts(evs))
	}
	tc := cont
	if tc.DestSys != "sys-c" || tc.DestCel != "cel-c-dock" {
		t.Fatalf("continuation = %q/%q, want sys-c/cel-c-dock", tc.DestSys, tc.DestCel)
	}
	parkIn("sys-c") // second hop
	jumpTo("sys-c", "Gate to Gamma")

	if !runUntil(t, s, &evs, 6000, func() bool {
		return firstPhase(evs, EvTraffic, TrafficDelivered) != nil
	}) {
		t.Fatalf("multi-hop shipment never delivered; %v", trafficTexts(evs))
	}
	delivered := firstPhase(evs, EvTraffic, TrafficDelivered)
	if delivered.System != "sys-c" || delivered.Cel != "cel-c-dock" {
		t.Fatalf("delivered = %q/%q, want sys-c/cel-c-dock", delivered.System, delivered.Cel)
	}
	if q, p := askAt(s, "cel-c-dock", "min-ferrite"); q != 100 || p != 9.99 {
		t.Fatalf("sys-c book = %d @ %v, want 100 @ 9.99", q, p)
	}
	if q, p := askAt(s, "cel-station-a", "min-ferrite"); q != 400 || p != 10 {
		t.Fatalf("origin ask = %d @ %v, want a single spawn-time debit (400 @ 10)", q, p)
	}
	// Two hops happened (sys-a → sys-b → sys-c).
	if n := countPhase(evs, EvTraffic, TrafficJumped); n != 2 {
		t.Fatalf("gate hops = %d, want 2; %v", n, trafficTexts(evs))
	}
}

// TestLogisticsRingTwinRuns is the determinism gate for the SHIPPED ring
// lanes: two sims activate Kamieniec (three new cross-system routes,
// spawn-time stock sourcing, gate hops parking journeys, low-sec ambush
// rolls) and stay in lockstep — hashes at checkpoints and event sequences —
// with the new lanes observably active.
func TestLogisticsRingTwinRuns(t *testing.T) {
	run := func() ([]Event, []uint64) {
		c, err := content.Load(content.FS())
		if err != nil {
			t.Fatalf("load content: %v", err)
		}
		wd, ok := c.World("world-showcase")
		if !ok {
			t.Fatal("world-showcase missing")
		}
		s, err := New(c, wd, 42)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.LoadSystem("sys-kamieniec", ""); err != nil {
			t.Fatal(err)
		}
		drain(s)
		var evs []Event
		var hashes []uint64
		for i := 0; i < 30000; i++ {
			s.Step()
			evs = append(evs, drain(s)...)
			if i%5000 == 0 {
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
	// The new lanes are observably active: departures from Kamieniec and
	// at least one gate hop (the shipment parked for its neighbour).
	departed, hops := 0, 0
	for i := range evA {
		if evA[i].Kind != EvTraffic {
			continue
		}
		if evA[i].Phase == TrafficDeparted && evA[i].System == "sys-kamieniec" {
			departed++
		}
		if evA[i].Phase == TrafficJumped {
			hops++
		}
	}
	if departed < 3 {
		t.Fatalf("ring departures from sys-kamieniec = %d, want >= 3", departed)
	}
	if hops < 1 {
		t.Fatal("ring lanes never exercised a gate hop")
	}
}

// ---------------------------------------------------------------------------
// Determinism
// ---------------------------------------------------------------------------

// TestLogisticsTwinRunsJourneysIdentical is the determinism gate with
// routes active: two sims (same seed, same manual cross-system spawn, same
// mid-flight player jump) stay in lockstep — hashes at checkpoints AND
// event sequences — and the run genuinely exercised a hop and a
// cross-system delivery.
func TestLogisticsTwinRunsJourneysIdentical(t *testing.T) {
	run := func() ([]Event, []uint64) {
		c := crossContent()
		s, err := New(c, testWorld(), 21)
		if err != nil {
			t.Fatal(err)
		}
		setPlayerPos(s, core.Vec2{X: -5e6, Y: -5e6})
		c.Traffic["sys-b"].Raider = ""
		drain(s)
		td, _ := s.Content.TrafficFor("sys-a")
		var evs []Event
		var hashes []uint64
		for i := 0; i < 9000; i++ {
			switch i {
			case 500:
				s.spawnTrafficHauler(s.System(), td, crossRoute())
			case 3000:
				// The player follows the shipment through the gate.
				gate := entityByName(s, "Gate to Beta")
				setPlayerPos(s, gatePos(s, gate))
				s.Queue(CmdJump{Gate: gate})
			}
			s.Step()
			evs = append(evs, drain(s)...)
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
	// The twin run actually crossed a system and delivered there.
	if firstPhase(evA, EvTraffic, TrafficJumped) == nil {
		t.Fatal("twin run never exercised a gate hop")
	}
	deliveredInB := false
	for i := range evA {
		if evA[i].Kind == EvTraffic && evA[i].Phase == TrafficDelivered &&
			evA[i].System == "sys-b" && evA[i].Cel == "cel-b-dock" {
			deliveredInB = true
		}
	}
	if !deliveredInB {
		t.Fatalf("twin run never delivered in sys-b; %v", trafficTexts(evA))
	}
}
