package sim

import (
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// ---------------------------------------------------------------------------
// NPC fleet burst regression: first entry into a system with a deep parked
// ledger must resume at capped, staggered traffic density (never one
// synchronized fleet), the parked backlog must drain behind that cap with
// cargo conserved end-to-end, the per-system gate queue must hold overflow
// upstream, and concurrent ambushes must never exceed the raider ceiling.
// All four tests pin COUNTS (they are regression pins, not golden hashes);
// the run-to-run determinism gates stay in traffic_test.go / haulwish_test.go.
// ---------------------------------------------------------------------------

// parkedCargo sums the recorded cargo on every parked journey (all systems).
func parkedCargo(s *Sim) int {
	n := 0
	for i := range s.trafficJourneys {
		n += s.trafficJourneys[i].Qty
	}
	return n
}

// haulerCargo sums the live convoy haulers' holds for an item.
func haulerCargo(s *Sim, item content.ItemID) int {
	n := 0
	for _, e := range sortedEntities(s.Traffic) {
		if c, ok := s.Cargos.Get(e); ok {
			n += stacksCount(c.Stacks, item)
		}
	}
	return n
}

// gridNPCCount counts living NPC pilot entities on the grid (static spawns,
// traffic raiders and patrols alike).
func gridNPCCount(s *Sim) int {
	n := 0
	s.AIs.Each(func(e ecs.Entity, _ *NpcAI) {
		if s.World.Alive(e) {
			n++
		}
	})
	return n
}

// staticSpawnCount is a system's authored spawn-group headcount.
func staticSpawnCount(c *content.Content, sys content.SystemID) int {
	def := c.Systems[sys]
	if def == nil {
		return 0
	}
	n := 0
	for i := range def.Celestials {
		cel := &def.Celestials[i]
		n += len(cel.Spawns)
		if cel.Anomaly != nil {
			n += len(cel.Anomaly.Spawns)
		}
	}
	return n
}

// parkJourneys appends n parked journeys for a system, all bound to the
// sys-b delivery dock and carrying `qty` units of min-ferrite each.
func parkJourneys(s *Sim, sys content.SystemID, at core.Vec2, n, qty int) {
	for i := 0; i < n; i++ {
		s.trafficJourneys = append(s.trafficJourneys, TrafficJourney{
			System: sys, At: at, DestSys: sys, DestCel: "cel-b-dock",
			Item: "min-ferrite", Qty: qty, Basis: 10, Hull: "ship-convoy",
		})
	}
}

// TestTrafficFirstEntryResumesCapped pins the defect's headline: a system
// first loaded with a deep parked ledger (deeper than the gate-queue
// ceiling on purpose) resumes at most trafficMaxLive haulers — staged
// behind the trafficDue stagger, never the whole backlog at once — while
// the grid's NPC population stays inside static spawns + the raider
// ceiling for every tick of the drain.
func TestTrafficFirstEntryResumesCapped(t *testing.T) {
	c := crossContent()
	c.Traffic["sys-b"].Routes = nil // only the parked backlog flies
	s, err := New(c, testWorld(), 5)
	if err != nil {
		t.Fatal(err)
	}
	setPlayerPos(s, core.Vec2{X: -5e6, Y: -5e6})
	drain(s)

	const backlog = 24
	parkJourneys(s, "sys-b", core.Vec2{X: 500, Y: 500}, backlog, 50)
	if err := s.LoadSystem("sys-b", ""); err != nil {
		t.Fatal(err)
	}
	// sys-b's authored spawns (belt spawn group) + the raider ceiling is
	// the most NPCs this grid may ever show — patrols never spawn here
	// (the fixture's sys-b def carries no patrol).
	static := staticSpawnCount(c, "sys-b")

	sawLive, drained := false, false
	for i := 0; i < 12000 && !drained; i++ {
		s.Step()
		drain(s)
		if n := s.Traffic.Len(); n > trafficMaxLive {
			t.Fatalf("tick %d: live haulers = %d, cap %d (resume bypassed the live cap)", i, n, trafficMaxLive)
		}
		if n := gridNPCCount(s); n > static+trafficMaxRaiders {
			t.Fatalf("tick %d: grid NPCs = %d, want <= static %d + raider cap %d",
				i, n, static, trafficMaxRaiders)
		}
		if n := s.trafficLiveRaiders(); n > trafficMaxRaiders {
			t.Fatalf("tick %d: live traffic raiders = %d, ceiling %d", i, n, trafficMaxRaiders)
		}
		if !sawLive && s.Traffic.Len() > 0 {
			sawLive = true
			// The backlog cannot empty faster than the live cap on the
			// first resume tick — the burst case is exactly this check.
			if parked := s.trafficParkedIn("sys-b"); parked < backlog-trafficMaxLive {
				t.Fatalf("first resume tick left %d of %d parked, want >= %d (cap bypassed)",
					parked, backlog, backlog-trafficMaxLive)
			}
		}
		drained = s.trafficParkedIn("sys-b") == 0 && s.Traffic.Len() == 0
	}
	if !sawLive {
		t.Fatal("parked backlog never resumed")
	}
	if !drained {
		t.Fatalf("backlog never drained; %d still parked", s.trafficParkedIn("sys-b"))
	}
}

// TestTrafficBacklogDrainsCappedWithCargoConserved runs a deep sys-b
// backlog to completion with no combat anywhere and proves, on every tick,
// that parked + live + delivered cargo is exactly the crafted total (the
// closed mass balance), that the live cap holds throughout, and that the
// first resume tick still leaves most of the ledger parked (staged drain).
// Two identical runs must produce identical hash checkpoints and event
// sequences (twin-run style — no golden hashes).
func TestTrafficBacklogDrainsCappedWithCargoConserved(t *testing.T) {
	const (
		backlog = 18
		qty     = 50
		total   = backlog * qty
	)
	run := func(t *testing.T) ([]uint64, []Event) {
		t.Helper()
		c := crossContent()
		c.Traffic["sys-a"].Routes = nil
		c.Traffic["sys-b"].Routes = nil
		c.Traffic["sys-b"].Raider = "" // no ambushes: nothing may destroy a hauler
		sysB := c.Systems["sys-b"]
		for i := range sysB.Celestials {
			sysB.Celestials[i].Spawns = nil // no static NPCs either
		}
		s, err := New(c, testWorld(), 17)
		if err != nil {
			t.Fatal(err)
		}
		setPlayerPos(s, core.Vec2{X: -5e6, Y: -5e6})
		drain(s)
		parkJourneys(s, "sys-b", core.Vec2{X: 500, Y: 500}, backlog, qty)
		if err := s.LoadSystem("sys-b", ""); err != nil {
			t.Fatal(err)
		}
		check := func(tick int) {
			t.Helper()
			if n := s.Traffic.Len(); n > trafficMaxLive {
				t.Fatalf("tick %d: live haulers = %d, cap %d", tick, n, trafficMaxLive)
			}
			parked, live := parkedCargo(s), haulerCargo(s, "min-ferrite")
			book := bookAskQty(s, "cel-b-dock", "min-ferrite")
			if parked+live+book != total {
				t.Fatalf("tick %d: cargo parked %d + live %d + delivered %d != %d (mass balance broken)",
					tick, parked, live, book, total)
			}
		}
		var hashes []uint64
		var evs []Event
		sawStaged := false
		for i := 0; i < 20000; i++ {
			s.Step()
			evs = append(evs, drain(s)...)
			check(i)
			if !sawStaged && s.Traffic.Len() > 0 {
				sawStaged = true
				if parked := parkedCargo(s); parked < (backlog-trafficMaxLive)*qty {
					t.Fatalf("first resume tick left %d units parked, want >= %d (cap bypassed)",
						parked, (backlog-trafficMaxLive)*qty)
				}
			}
			if i%1500 == 0 {
				hashes = append(hashes, s.Hash())
			}
			if parkedCargo(s) == 0 && s.Traffic.Len() == 0 &&
				bookAskQty(s, "cel-b-dock", "min-ferrite") == total {
				hashes = append(hashes, s.Hash())
				if !sawStaged {
					t.Fatal("backlog drained without ever staging behind the cap")
				}
				return hashes, evs
			}
		}
		t.Fatalf("backlog never drained (parked=%d live=%d book=%d)",
			parkedCargo(s), s.Traffic.Len(), bookAskQty(s, "cel-b-dock", "min-ferrite"))
		return nil, nil
	}
	hA, eA := run(t)
	hB, eB := run(t)
	if len(hA) != len(hB) {
		t.Fatal("hash checkpoint counts differ")
	}
	for i := range hA {
		if hA[i] != hB[i] {
			t.Fatalf("checkpoint %d: %016x vs %016x", i, hA[i], hB[i])
		}
	}
	if len(eA) != len(eB) {
		t.Fatalf("event counts differ: %d vs %d", len(eA), len(eB))
	}
	for i := range eA {
		a, b := eA[i], eB[i]
		if a.Kind != b.Kind || a.Phase != b.Phase || a.Text != b.Text ||
			a.Time != b.Time || a.System != b.System || a.Value != b.Value {
			t.Fatalf("event %d differs:\n%+v\n%+v", i, a, b)
		}
	}
}

// TestTrafficGateQueueCeilingHolds pins the gate queue: sys-b's parked
// ledger sits at trafficMaxParkedJourneys, so every hop from sys-a HOLDS
// upstream (no TrafficJumped event, journey re-parked in sys-a, cargo
// intact); once the player drains sys-b below the ceiling, a held shipment
// retries and hops through — and the ceiling is never exceeded on any
// side, ever.
func TestTrafficGateQueueCeilingHolds(t *testing.T) {
	const (
		qty    = 50
		spawns = 6
	)
	c := crossContent()
	c.Traffic["sys-a"].Routes = nil
	c.Traffic["sys-b"].Routes = nil
	c.Traffic["sys-b"].Raider = ""
	sysB := c.Systems["sys-b"]
	for i := range sysB.Celestials {
		sysB.Celestials[i].Spawns = nil // no combat: exact conservation
	}
	s, err := New(c, testWorld(), 11)
	if err != nil {
		t.Fatal(err)
	}
	setPlayerPos(s, core.Vec2{X: -5e6, Y: -5e6})
	drain(s)
	td, _ := s.Content.TrafficFor("sys-a")

	// The far queue starts exactly at its ceiling; six more haulers are
	// mid-flight toward it (sourced from the origin book, like any lane).
	parkJourneys(s, "sys-b", core.Vec2{X: 500, Y: 500}, trafficMaxParkedJourneys, qty)
	for i := 0; i < spawns; i++ {
		s.spawnTrafficHauler(s.System(), td, crossRoute())
	}
	if n := s.Traffic.Len(); n != spawns {
		t.Fatalf("live haulers = %d, want %d (fixture spawns failed)", n, spawns)
	}
	total := trafficMaxParkedJourneys*qty + spawns*100

	var evs []Event
	check := func(tick int) {
		t.Helper()
		parked, live := parkedCargo(s), haulerCargo(s, "min-ferrite")
		book := bookAskQty(s, "cel-b-dock", "min-ferrite")
		if parked+live+book != total {
			t.Fatalf("tick %d: cargo parked %d + live %d + delivered %d != %d (mass balance broken)",
				tick, parked, live, book, total)
		}
		if n := s.trafficParkedIn("sys-b"); n > trafficMaxParkedJourneys {
			t.Fatalf("tick %d: journeys parked for sys-b = %d, ceiling %d", tick, n, trafficMaxParkedJourneys)
		}
	}

	// Phase 1: every hop into sys-b holds upstream; the destination queue
	// stays exactly at its ceiling and no jump ever completes.
	sawHold := false
	for i := 0; i < 3000 && !sawHold; i++ {
		s.Step()
		evs = append(evs, drain(s)...)
		check(i)
		if n := s.trafficParkedIn("sys-b"); n != trafficMaxParkedJourneys {
			t.Fatalf("tick %d: sys-b queue = %d, want exactly %d (hop pushed past the ceiling?)",
				i, n, trafficMaxParkedJourneys)
		}
		sawHold = s.trafficParkedIn("sys-a") > 0
	}
	if !sawHold {
		t.Fatalf("no upstream hold observed; %v", trafficTexts(evs))
	}
	if n := countPhase(evs, EvTraffic, TrafficJumped); n != 0 {
		t.Fatalf("%d gate hops completed while the far queue was full; %v", n, trafficTexts(evs))
	}
	if findEvent(evs, EvLog, "gate queue full") == nil {
		t.Fatalf("no hold announcement; %v", trafficTexts(evs))
	}

	// Phase 2: the player follows into sys-b; the queue drains (resume is
	// staggered) and deliveries book out — still never over the ceiling.
	gate := entityByName(s, "Gate to Beta")
	if gate.IsZero() {
		t.Fatal("no Gate to Beta")
	}
	setPlayerPos(s, gatePos(s, gate))
	s.Queue(CmdJump{Gate: gate})
	s.Step()
	evs = append(evs, drain(s)...)
	check(-1)
	if s.systemID != "sys-b" {
		t.Fatalf("system = %q, want sys-b", s.systemID)
	}
	if !runUntil(t, s, &evs, 6000, func() bool {
		check(int(s.Ticks()))
		return bookAskQty(s, "cel-b-dock", "min-ferrite") >= qty
	}) {
		t.Fatalf("sys-b queue never drained a delivery; %v", trafficTexts(evs))
	}

	// Phase 3: back in sys-a, the held shipments retry the hop — now the
	// far queue has room (deliveries booked out), so a jump completes.
	backGate := entityByName(s, "Gate to Alpha")
	if backGate.IsZero() {
		t.Fatal("no Gate to Alpha")
	}
	setPlayerPos(s, gatePos(s, backGate))
	s.Queue(CmdJump{Gate: backGate})
	s.Step()
	evs = append(evs, drain(s)...)
	check(-1)
	if s.systemID != "sys-a" {
		t.Fatalf("system = %q, want sys-a", s.systemID)
	}
	if n := s.trafficParkedIn("sys-b"); n > trafficMaxParkedJourneys {
		t.Fatalf("re-parking on jump-back pushed sys-b to %d, ceiling %d", n, trafficMaxParkedJourneys)
	}
	if !runUntil(t, s, &evs, 1200, func() bool {
		check(int(s.Ticks()))
		return countPhase(evs, EvTraffic, TrafficJumped) > 0
	}) {
		t.Fatalf("held shipment never retried through the gate; %v", trafficTexts(evs))
	}
	// Keep the ceiling honest while the retry backlog refills it.
	for i := 0; i < 300; i++ {
		s.Step()
		evs = append(evs, drain(s)...)
		check(i)
	}
}

// TestTrafficRaiderCeilingPerGrid presses the raider ceiling with N
// simultaneous ambush rolls in one tick (as a burst of tunnel entries
// would): live traffic raiders must never exceed trafficMaxRaiders, at
// least one roll must be refused at the ceiling, and the population must
// stay inside the ceiling while the raiders warp in and break off.
// Static spawnGroup NPCs never count against the ceiling.
func TestTrafficRaiderCeilingPerGrid(t *testing.T) {
	c := trafficContent()
	c.Traffic["sys-a"].Routes = nil // no scheduled spawns in the way
	c.Traffic["sys-a"].Patrol = nil // pure raider-vs-hauler: no response wing
	s, err := New(c, testWorld(), 3)
	if err != nil {
		t.Fatal(err)
	}
	setPlayerPos(s, core.Vec2{X: -5e6, Y: -5e6})
	drain(s)
	td, _ := s.Content.TrafficFor("sys-a")

	// N = 2 × trafficMaxLive direct spawns: spawnTrafficHauler itself takes
	// no cap (the schedules enforce it), which is exactly what lets this
	// test pile far more simultaneous tunnel rolls onto one tick than the
	// resume cap would ever allow.
	const n = 12
	for i := 0; i < n; i++ {
		s.spawnTrafficHauler(s.System(), td, crossRouteFromStation())
	}
	if got := s.Traffic.Len(); got != n {
		t.Fatalf("live haulers = %d, want %d", got, n)
	}

	refused := 0
	for _, e := range sortedEntities(s.Traffic) {
		tc, ok := s.Traffic.Get(e)
		if !ok {
			continue
		}
		// Same call the tunnel-entry roll makes after a chance success.
		s.startAmbush(e, tc, td, "null")
		if tc.Attack == nil {
			refused++
		}
		if got := s.trafficLiveRaiders(); got > trafficMaxRaiders {
			t.Fatalf("live traffic raiders = %d after roll %d, ceiling %d",
				got, refused, trafficMaxRaiders)
		}
	}
	// 12 successes would need 12 live raiders under a ceiling of 6 — at
	// least one roll must have been refused.
	if refused == 0 {
		t.Fatalf("raider ceiling never engaged (%d live raiders)", s.trafficLiveRaiders())
	}

	var evs []Event
	for i := 0; i < 3600; i++ {
		s.Step()
		evs = append(evs, drain(s)...)
		if got := s.trafficLiveRaiders(); got > trafficMaxRaiders {
			t.Fatalf("tick %d: live traffic raiders = %d, ceiling %d", i, got, trafficMaxRaiders)
		}
	}
}
