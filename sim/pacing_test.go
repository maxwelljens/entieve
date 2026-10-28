package sim

import (
	"fmt"
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// ---------------------------------------------------------------------------
// Paced commencement (CONTEXT.md: departure beat / gate cycle / turnaround
// / agent cooldown): the delays are jittered from pure hashes, so every
// pin here is a RANGE in ticks — floors prove nothing commences on the tick
// it becomes possible, ceilings prove nothing stalls — plus the ordering
// pins (dwell → hop → cycle → materialize → beat → warp; approach → docked
// → action → undock), and the log-throttle / cooldown / conservation /
// determinism gates.
// ---------------------------------------------------------------------------

// TestPacingDelayWindowsPinned pins every pacing window at its authored
// tick values and proves trafficBeat jitters inside them, purely.
func TestPacingDelayWindowsPinned(t *testing.T) {
	windows := []struct {
		name     string
		min, max int64
		wantMin  int64
		wantMax  int64
	}{
		{"departure beat", trafficDepartBeatMin, trafficDepartBeatMax, 120, 480},
		{"gate dwell", trafficGateDwellMin, trafficGateDwellMax, 120, 300},
		{"jump cycle", trafficJumpCycleMin, trafficJumpCycleMax, 60, 180},
		{"turnaround approach", trafficApproachMin, trafficApproachMax, 120, 240},
		{"turnaround dock", trafficDockMin, trafficDockMax, 120, 300},
		{"turnaround berth", trafficBerthMin, trafficBerthMax, 180, 600},
	}
	for _, w := range windows {
		if w.min != w.wantMin || w.max != w.wantMax {
			t.Errorf("%s window = [%d, %d], want [%d, %d] (tick-pinned)",
				w.name, w.min, w.max, w.wantMin, w.wantMax)
		}
		if w.min < 60 {
			t.Errorf("%s floor %d ticks is under 1 s", w.name, w.min)
		}
		// trafficBeat stays inside the window, is pure (same key ⇒ same
		// delay), and actually jitters across keys.
		first := trafficBeat("k1", w.min, w.max)
		same := true
		for i := 0; i < 64; i++ {
			key := "beat|" + w.name + "|" + itoaN(i)
			got := trafficBeat(key, w.min, w.max)
			if got < w.min || got > w.max {
				t.Fatalf("%s beat %d outside [%d, %d]", w.name, got, w.min, w.max)
			}
			if got != trafficBeat(key, w.min, w.max) {
				t.Fatalf("%s beat for one key is not pure", w.name)
			}
			if got != first {
				same = false
			}
		}
		if same {
			t.Errorf("%s never jitters across 64 keys", w.name)
		}
	}
	// The docked dwell is dock + berth: 5..15 s.
	if lo, hi := trafficDockMin+trafficBerthMin, trafficDockMax+trafficBerthMax; lo != 300 || hi != 900 {
		t.Errorf("docked dwell = [%d, %d], want [300, 900] (5..15 s)", lo, hi)
	}
	// Agent cooldowns sit in 1..10 s for any id.
	for _, id := range []string{"ag-001", "ag-haul", "ag-zip-sell", "ag-ind"} {
		c := agentCooldownTicks(&Agent{ID: id})
		if c < agentCooldownMin || c > agentCooldownMax {
			t.Errorf("cooldown(%s) = %d, want [%d, %d]", id, c, agentCooldownMin, agentCooldownMax)
		}
	}
}

// pacedSim builds the cross-system fixture with all scheduled lanes
// silenced: the tests spawn their own shipments, so every hauler on the
// grid belongs to the assertion.
func pacedSim(t *testing.T) *Sim {
	t.Helper()
	c := crossContent()
	c.Traffic["sys-a"].Routes = nil
	c.Traffic["sys-b"].Routes = nil
	c.Traffic["sys-b"].Raider = ""
	s, err := New(c, testWorld(), 9)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	setPlayerPos(s, core.Vec2{X: -5e6, Y: -5e6})
	drain(s)
	return s
}

// spawnCross spawns the cross-system lane by hand and returns it live.
func spawnCross(t *testing.T, s *Sim) (ecs.Entity, *Traffic) {
	t.Helper()
	td, _ := s.Content.TrafficFor(s.systemID)
	s.spawnTrafficHauler(s.System(), td, crossRoute())
	e, tc, ok := findHauler(s, "")
	if !ok {
		t.Fatal("no hauler spawned")
	}
	return e, tc
}

// TestPacingDepartureBeatAntiInstant is the anti-instant pin: a hauler
// spawned (or resumed) somewhere does not commence its warp on the tick —
// it holds the departure beat first (floor = 120 ticks) — and it always
// warps inside the beat's ceiling (480).
func TestPacingDepartureBeatAntiInstant(t *testing.T) {
	s := pacedSim(t)
	e, _ := spawnCross(t, s)
	spawnTick := s.ticks

	for i := int64(0); i < trafficDepartBeatMin; i++ {
		s.Step()
		drain(s)
		if s.Warps.Has(e) {
			t.Fatalf("leg warp commenced %d ticks after spawn — departure beat floor is %d",
				s.ticks-spawnTick, trafficDepartBeatMin)
		}
	}
	start := int64(-1)
	for i := int64(0); i <= trafficDepartBeatMax-trafficDepartBeatMin; i++ {
		s.Step()
		drain(s)
		if s.Warps.Has(e) {
			start = s.ticks - spawnTick
			break
		}
	}
	if start < trafficDepartBeatMin || start > trafficDepartBeatMax {
		t.Fatalf("departure beat = %d ticks, want [%d, %d]", start,
			trafficDepartBeatMin, trafficDepartBeatMax)
	}

	// Resume flavor: a parked journey's continuation holds its beat too
	// (the carried Next only gates the resume itself).
	s.trafficJourneys = append(s.trafficJourneys, TrafficJourney{
		System: "sys-a", At: core.Vec2{X: 1e6, Y: 1e6}, DestSys: "", DestCel: "cel-a-dock",
		Item: "min-ferrite", Qty: 10, Basis: 10, Hull: "ship-convoy",
	})
	var re ecs.Entity
	var rtc *Traffic
	for i := 0; i < 10*trafficResumeInterval && rtc == nil; i++ {
		s.Step()
		drain(s)
		re, rtc, _ = findHauler(s, "cel-a-dock")
	}
	if rtc == nil {
		t.Fatal("journey never resumed")
	}
	materialized := s.ticks
	for i := int64(0); i < trafficDepartBeatMin; i++ {
		s.Step()
		drain(s)
		if s.Warps.Has(re) {
			t.Fatalf("resumed hauler warped %d ticks after materializing — beat floor %d",
				s.ticks-materialized, trafficDepartBeatMin)
		}
	}
}

// TestPacingGateCycleOrdering pins the gate half of the beat end to end:
// arrival → dwell (120..300) → hop → jump cycle on the parked journey
// (60..180) → materialization never before it → departure beat → onward
// warp.
func TestPacingGateCycleOrdering(t *testing.T) {
	s := pacedSim(t)
	e, _ := spawnCross(t, s)
	gate := s.celEntity("cel-gate-b")
	gp, _ := s.posOf(gate)

	var evs []Event
	// 1. Fly to the gate, then stand there (arrival is not commencement).
	arrived := int64(-1)
	for i := 0; i < 3000 && arrived < 0; i++ {
		s.Step()
		evs = append(evs, drain(s)...)
		if _, w := s.Warps.Get(e); w || !s.World.Alive(e) {
			continue
		}
		if p, ok := s.posOf(e); ok && p.Dist(gp) <= warpLandRange {
			arrived = s.ticks
		}
	}
	if arrived < 0 {
		t.Fatalf("hauler never reached the gate; %v", trafficTexts(evs))
	}
	// 2. The dwell: no hop on arrival, none before the floor, hop by the
	// ceiling.
	hopTick := int64(-1)
	for i := int64(0); i <= trafficGateDwellMax; i++ {
		s.Step()
		evs = append(evs, drain(s)...)
		if s.trafficParkedIn("sys-b") > 0 {
			hopTick = s.ticks
			break
		}
	}
	if hopTick < 0 {
		t.Fatalf("shipment never hopped; %v", trafficTexts(evs))
	}
	if d := hopTick - arrived; d < trafficGateDwellMin || d > trafficGateDwellMax {
		t.Fatalf("gate dwell = %d ticks, want [%d, %d]", d, trafficGateDwellMin, trafficGateDwellMax)
	}
	if s.World.Alive(e) {
		t.Fatal("hauler survived its gate hop")
	}
	if hop := firstPhase(evs, EvTraffic, TrafficJumped); hop == nil {
		t.Fatalf("no jumped event; %v", trafficTexts(evs))
	}
	// 3. The jump cycle rides the parked journey.
	if len(s.trafficJourneys) != 1 || s.trafficJourneys[0].System != "sys-b" {
		t.Fatalf("parked journeys = %+v, want one for sys-b", s.trafficJourneys)
	}
	j := s.trafficJourneys[0]
	// The hop armed Next against the tick's pre-increment clock (systems
	// run on ticks-1's time), so measure from there.
	cycle := (j.Next - float64(hopTick-1)*DT) / DT
	if cycle < float64(trafficJumpCycleMin)-1e-6 || cycle > float64(trafficJumpCycleMax)+1e-6 {
		t.Fatalf("jump cycle = %v ticks, want [%d, %d]", cycle,
			trafficJumpCycleMin, trafficJumpCycleMax)
	}
	// 4. The player follows: nothing materializes inside the jump cycle,
	// and the continuation spawns after it.
	back := entityByName(s, "Gate to Beta")
	setPlayerPos(s, gatePos(s, back))
	s.Queue(CmdJump{Gate: back})
	s.Step()
	evs = append(evs, drain(s)...)
	if s.systemID != "sys-b" {
		t.Fatalf("system = %q, want sys-b", s.systemID)
	}
	cycleEnd := int64(j.Next/DT + 0.5) // the first tick resume may spawn on
	materialized := int64(-1)
	for i := 0; i < 900 && materialized < 0; i++ {
		s.Step()
		evs = append(evs, drain(s)...)
		if s.ticks < cycleEnd && s.trafficParkedIn("sys-b") == 0 {
			t.Fatal("journey materialized inside its jump cycle")
		}
		if _, _, ok := findHauler(s, "cel-b-dock"); ok {
			materialized = s.ticks
		}
	}
	if materialized < 0 {
		t.Fatalf("continuation never materialized; %v", trafficTexts(evs))
	}
	if materialized < cycleEnd {
		t.Fatalf("materialized at %d, jump cycle ends at %d", materialized, cycleEnd)
	}
	// 5. Departure beat, then the onward warp.
	for i := int64(0); i < trafficDepartBeatMin; i++ {
		s.Step()
		drain(s)
		re, _, ok := findHauler(s, "cel-b-dock")
		if ok && s.Warps.Has(re) {
			t.Fatalf("onward warp %d ticks after materializing — beat floor %d",
				s.ticks-materialized, trafficDepartBeatMin)
		}
	}
	re, _, ok := findHauler(s, "cel-b-dock")
	if !ok {
		t.Fatal("continuation vanished before its onward warp")
	}
	start := int64(-1)
	for i := int64(0); i <= trafficDepartBeatMax-trafficDepartBeatMin; i++ {
		s.Step()
		drain(s)
		if s.Warps.Has(re) {
			start = s.ticks - materialized
			break
		}
	}
	if start < trafficDepartBeatMin || start > trafficDepartBeatMax {
		t.Fatalf("onward departure beat = %d ticks, want [%d, %d]", start,
			trafficDepartBeatMin, trafficDepartBeatMax)
	}
}

// TestPacingStationTurnaroundOrdering pins the station half: arrival arms
// the approach, the docked dwell carries the cargo action partway through
// (never at contact), then undock + departure beat before the hauler
// leaves the grid — with the hold conserved into the book at the action.
func TestPacingStationTurnaroundOrdering(t *testing.T) {
	s := pacedSim(t)
	td, _ := s.Content.TrafficFor(s.systemID)
	s.spawnTrafficHauler(s.System(), td, content.RouteDef{
		From: "cel-station-a", To: "cel-a-dock", Hull: "ship-convoy",
		Cargo:         content.RouteCargo{Item: "min-ferrite", Qty: 100},
		IntervalTicks: 1 << 30,
	})
	e, _, ok := findHauler(s, "cel-a-dock")
	if !ok {
		t.Fatal("no hauler spawned")
	}
	station := s.celEntity("cel-a-dock")
	sp, _ := s.posOf(station)

	type stage struct {
		phase string
		at    int64
	}
	var stages []stage
	var evs []Event
	arrived, deliveredAt, goneAt := int64(-1), int64(-1), int64(-1)
	for i := 0; i < 4000 && goneAt < 0; i++ {
		s.Step()
		evs = append(evs, drain(s)...)
		if !s.World.Alive(e) {
			goneAt = s.ticks
			break
		}
		cur, _ := s.Traffic.Get(e)
		if n := len(stages); n == 0 || stages[n-1].phase != cur.Phase {
			stages = append(stages, stage{phase: cur.Phase, at: s.ticks})
		}
		if arrived < 0 {
			if _, w := s.Warps.Get(e); !w {
				if p, ok := s.posOf(e); ok && p.Dist(sp) <= warpLandRange {
					arrived = s.ticks
					// The hold rides all the way to the cargo action.
					if c, ok := s.Cargos.Get(e); !ok || stacksCount(c.Stacks, "min-ferrite") != 100 {
						t.Fatal("hold lost before the cargo action")
					}
				}
			}
		}
		if deliveredAt < 0 && firstPhase(evs, EvTraffic, TrafficDelivered) != nil {
			deliveredAt = s.ticks
		}
	}
	if arrived < 0 {
		t.Fatalf("hauler never reached the station; %v", trafficTexts(evs))
	}
	if deliveredAt < 0 {
		t.Fatalf("no delivery event; %v", trafficTexts(evs))
	}
	if goneAt < 0 {
		t.Fatal("hauler never left the grid")
	}
	// Stage walk: transit → approach → dock → berth → undock, in order.
	want := []string{TrafficTransit, TrafficApproach, TrafficDock, TrafficBerth, TrafficUndock}
	if len(stages) != len(want) {
		var got []string
		for _, st := range stages {
			got = append(got, st.phase)
		}
		t.Fatalf("stage walk = %v, want %v", got, want)
	}
	for i, st := range want {
		if stages[i].phase != st {
			t.Fatalf("stage %d = %q, want %q", i, stages[i].phase, st)
		}
	}
	in := func(d, lo, hi int64) bool { return d >= lo && d <= hi }
	// First contact arms the approach on the arrival tick (no commencement
	// on arrival — the STAGE begins, its wait runs afterwards).
	if d := stages[1].at - arrived; d != 0 {
		t.Fatalf("approach armed %d ticks after contact, want same tick", d)
	}
	approach := stages[2].at - stages[1].at // contact → dock
	if !in(approach, trafficApproachMin, trafficApproachMax) {
		t.Fatalf("approach stage = %d ticks, want [%d, %d]", approach,
			trafficApproachMin, trafficApproachMax)
	}
	act := deliveredAt - stages[2].at // dock → cargo action
	if !in(act, trafficDockMin, trafficDockMax) {
		t.Fatalf("cargo action fired %d ticks into the dwell, want [%d, %d]",
			act, trafficDockMin, trafficDockMax)
	}
	if act == 0 {
		t.Fatal("cargo action fired at the instant of contact")
	}
	berth := stages[4].at - deliveredAt // action → undock
	if !in(berth, trafficBerthMin, trafficBerthMax) {
		t.Fatalf("docked rest = %d ticks, want [%d, %d]", berth,
			trafficBerthMin, trafficBerthMax)
	}
	undock := goneAt - stages[4].at // undock → despawn (departure beat)
	if !in(undock, trafficDepartBeatMin, trafficDepartBeatMax) {
		t.Fatalf("undock departure beat = %d ticks, want [%d, %d]", undock,
			trafficDepartBeatMin, trafficDepartBeatMax)
	}
	// The cargo action offloaded the hold into the book.
	if c, ok := s.Cargos.Get(e); ok && len(c.Stacks) > 0 {
		t.Fatalf("hold still carries %v after the cargo action", c.Stacks)
	}
	if q, _ := bestAt(s, "cel-a-dock", "min-ferrite", OrderSell); q != 100 {
		t.Fatalf("destination ask = %d, want 100 (posted at the action)", q)
	}
}

// TestPacingCargoConservedAcrossDelayedTransitions walks one shipment
// through every paced stage and proves the units are counted exactly once
// wherever they stand: in the hauler (beat, warp, gate dwell), in the
// journey (jump cycle), in the resumed hauler (beat, warp, turnaround up
// to the action), then in the destination book.
func TestPacingCargoConservedAcrossDelayedTransitions(t *testing.T) {
	s := pacedSim(t)
	const qty = 100
	hold := func() int {
		n := 0
		for _, e := range sortedEntities(s.Traffic) {
			if c, ok := s.Cargos.Get(e); ok {
				n += stacksCount(c.Stacks, "min-ferrite")
			}
		}
		return n
	}
	total := func() int {
		return hold() + parkedCargo(s) + bookAskQty(s, "cel-b-dock", "min-ferrite")
	}
	check := func(when string) {
		t.Helper()
		if got := total(); got != qty {
			t.Fatalf("%s: total %d, want %d (hold %d, parked %d, book %d)",
				when, got, qty, hold(), parkedCargo(s),
				bookAskQty(s, "cel-b-dock", "min-ferrite"))
		}
	}
	spawnCross(t, s)
	check("at spawn") // the origin book was debited at spawn

	var evs []Event
	for i := 0; i < 4000 && parkedCargo(s) == 0; i++ {
		s.Step()
		evs = append(evs, drain(s)...)
		check("in flight")
	}
	if parkedCargo(s) == 0 {
		t.Fatalf("shipment never hopped; %v", trafficTexts(evs))
	}
	check("in the journey (jump cycle)")

	back := entityByName(s, "Gate to Beta")
	setPlayerPos(s, gatePos(s, back))
	s.Queue(CmdJump{Gate: back})
	s.Step()
	evs = append(evs, drain(s)...)
	for i := 0; i < 6000 && bookAskQty(s, "cel-b-dock", "min-ferrite") == 0; i++ {
		s.Step()
		evs = append(evs, drain(s)...)
		check("continuation")
	}
	check("after delivery")
	if q, _ := bestAt(s, "cel-b-dock", "min-ferrite", OrderSell); q != qty {
		t.Fatalf("delivered = %d, want %d", q, qty)
	}
	if q, _ := bestAt(s, "cel-station-a", "min-ferrite", OrderSell); q != 400 {
		t.Fatalf("origin ask = %d, want 400 (debited once at spawn)", q)
	}
}

// TestPacingHoldLogsOncePerEpisode is the log-throttle gate: a gate-queue
// hold announces itself exactly once no matter how long the far queue
// stays full (the old respawn/re-hold cycle re-logged on every retry), and
// the episode closes with exactly one jump follow-up when it finally hops.
func TestPacingHoldLogsOncePerEpisode(t *testing.T) {
	s := pacedSim(t)
	td, _ := s.Content.TrafficFor(s.systemID)
	// Far queue exactly at its ceiling; one shipment flies into it.
	parkJourneys(s, "sys-b", core.Vec2{X: 500, Y: 500}, trafficMaxParkedJourneys, 50)
	s.spawnTrafficHauler(s.System(), td, crossRoute())

	holdsIn := func(evs []Event) int {
		n := 0
		for _, ev := range evs {
			if ev.Kind == EvLog && strings.Contains(ev.Text, "gate queue full") {
				n++
			}
		}
		return n
	}
	var evs []Event
	// Phase 1: the hold announces once.
	if !runUntil(t, s, &evs, 3000, func() bool { return holdsIn(evs) > 0 }) {
		t.Fatalf("no hold announcement; %v", trafficTexts(evs))
	}
	if n := holdsIn(evs); n != 1 {
		t.Fatalf("hold logged %d times on entry, want exactly 1", n)
	}
	// Phase 2: retries are silent — run past many resume windows (the old
	// behavior re-logged roughly every trafficResumeInterval ticks).
	for i := 0; i < 3000; i++ {
		s.Step()
		evs = append(evs, drain(s)...)
	}
	if n := holdsIn(evs); n != 1 {
		t.Fatalf("hold logged %d times after retries, want exactly 1 per episode; %v",
			n, trafficTexts(evs))
	}
	if n := countPhase(evs, EvTraffic, TrafficJumped); n != 0 {
		t.Fatalf("%d jumps while the far queue was full", n)
	}

	// Phase 3: the queue drains (loading the far system consumes its
	// ledger and a delivery books out — enough room opens upstream), the
	// player returns, and the episode closes with one jump.
	back := entityByName(s, "Gate to Beta")
	setPlayerPos(s, gatePos(s, back))
	s.Queue(CmdJump{Gate: back})
	s.Step()
	evs = append(evs, drain(s)...)
	if s.systemID != "sys-b" {
		t.Fatalf("system = %q, want sys-b", s.systemID)
	}
	if !runUntil(t, s, &evs, 6000, func() bool {
		return bookAskQty(s, "cel-b-dock", "min-ferrite") >= 50
	}) {
		t.Fatalf("far queue never drained a delivery; %d parked", s.trafficParkedIn("sys-b"))
	}
	down := entityByName(s, "Gate to Alpha")
	setPlayerPos(s, gatePos(s, down))
	s.Queue(CmdJump{Gate: down})
	s.Step()
	evs = append(evs, drain(s)...)
	if s.systemID != "sys-a" {
		t.Fatalf("system = %q, want sys-a", s.systemID)
	}
	if !runUntil(t, s, &evs, 1200, func() bool {
		return countPhase(evs, EvTraffic, TrafficJumped) > 0
	}) {
		t.Fatalf("held shipment never hopped; %v", trafficTexts(evs))
	}
	for i := 0; i < 600; i++ {
		s.Step()
		evs = append(evs, drain(s)...)
	}
	if n := holdsIn(evs); n != 1 {
		t.Fatalf("hold logged %d times across the episode, want 1", n)
	}
	if n := countPhase(evs, EvTraffic, TrafficJumped); n != 1 {
		t.Fatalf("%d jump follow-ups, want exactly 1", n)
	}
}

// TestAgentCooldownEnforced walks a live industrialist tick by tick and
// records every tick that moves capital (a resting-order change, a job
// submission, a wish-list change): consecutive capital actions must be at
// least one cooldown apart — no instant chains. (One act, one kind is the
// construction in agentAct; this pins it from outside.)
func TestAgentCooldownEnforced(t *testing.T) {
	s, err := New(industryContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	home := StationID("cel-station-a")
	addTestAgent(s, "ag-pace", RoleIndustrialist, home, 1_000_000)
	a := agentOf(s, "ag-pace")
	// Materials in hand so the commission act has something to commit, and
	// no counterparty traffic in this fixture: every capital footprint
	// below is the agent's own act.
	a.Inventory["min-ferrite"] = 100
	a.Inventory["min-nickel"] = 100
	cd := agentCooldownTicks(a)
	if cd < agentCooldownMin || cd > agentCooldownMax {
		t.Fatalf("cooldown = %d, want [%d, %d]", cd, agentCooldownMin, agentCooldownMax)
	}
	tag := agentOwnerTag(a.ID)
	capitalSig := func() string {
		sig := ""
		for _, o := range a.Orders {
			sig += itoaN(int(o.ID)) + "|"
		}
		// The fee/escrow moves capital inside the quoting act itself.
		return sig + fmt.Sprintf("%.6f", a.Capital)
	}
	wishSig := func() string {
		sig := ""
		for _, w := range a.Wishes {
			sig += string(w.Item) + "|"
		}
		return sig
	}
	ownedJobs := func() int {
		n := 0
		for _, j := range s.jobs {
			if j.Owner == tag {
				n++
			}
		}
		return n
	}

	var events []int64
	prevCapital, prevWishes := capitalSig(), wishSig()
	prevJobs := ownedJobs()
	// Bound under agentUpkeepTicks: upkeep fills would be counterparty
	// trades, not the agent's own actions.
	for i := 0; i < 3400; i++ {
		s.Step()
		drain(s)
		capital, wishes, jobs := capitalSig(), wishSig(), ownedJobs()
		// Collections shrink the job count — bookkeeping, not a capital
		// move; only submissions (growth), postings/fills and wish
		// contracts count as actions.
		moved := capital != prevCapital || wishes != prevWishes || jobs > prevJobs
		if moved {
			events = append(events, s.ticks)
			// Capital may only move ON a paced act (systems run on the
			// pre-increment tick).
			if !trafficDue(s.ticks-1, agentKey(a), cd) {
				t.Fatalf("capital moved at tick %d outside the agent's cooldown act", s.ticks)
			}
		}
		prevCapital, prevWishes, prevJobs = capital, wishes, jobs
	}
	if len(events) < 2 {
		t.Fatalf("only %d capital actions observed — fixture never acted", len(events))
	}
	for i := 1; i < len(events); i++ {
		if gap := events[i] - events[i-1]; gap < cd {
			t.Fatalf("capital actions at ticks %d and %d are %d apart — under the %d-tick cooldown",
				events[i-1], events[i], gap, cd)
		}
	}
}

// TestPacingTwinRunsAndSaveRestoreIdentical: paced convoys keep both
// determinism gates — twin runs match hashes and event sequences at every
// checkpoint, and a snapshot taken mid-stage (departure beat, gate dwell,
// turnaround) restores hash-identically and stays in lockstep.
func TestPacingTwinRunsAndSaveRestoreIdentical(t *testing.T) {
	build := func() *Sim {
		s := pacedSim(t)
		return s
	}
	spawnCrossLane := func(s *Sim) {
		td, _ := s.Content.TrafficFor(s.systemID)
		s.spawnTrafficHauler(s.System(), td, crossRoute())
	}
	spawnTurnaround := func(s *Sim) {
		td, _ := s.Content.TrafficFor(s.systemID)
		s.spawnTrafficHauler(s.System(), td, content.RouteDef{
			From: "cel-station-a", To: "cel-a-dock", Hull: "ship-convoy",
			Cargo:         content.RouteCargo{Item: "min-ferrite", Qty: 100},
			IntervalTicks: 1 << 30,
		})
	}

	// Twin runs: identical hashes at checkpoints, identical events.
	run := func() ([]uint64, []Event) {
		s := build()
		spawnCrossLane(s)
		spawnTurnaround(s)
		var hashes []uint64
		var evs []Event
		for i := 0; i < 3600; i++ {
			s.Step()
			evs = append(evs, drain(s)...)
			if i%600 == 0 {
				hashes = append(hashes, s.Hash())
			}
		}
		return append(hashes, s.Hash()), evs
	}
	hA, evA := run()
	hB, evB := run()
	if len(hA) != len(hB) {
		t.Fatal("twin checkpoint counts differ")
	}
	for i := range hA {
		if hA[i] != hB[i] {
			t.Fatalf("twin checkpoint %d: %016x vs %016x", i, hA[i], hB[i])
		}
	}
	if len(evA) != len(evB) {
		t.Fatalf("twin event counts differ: %d vs %d", len(evA), len(evB))
	}
	for i := range evA {
		if evA[i].Kind != evB[i].Kind || evA[i].Phase != evB[i].Phase ||
			evA[i].Text != evB[i].Text || evA[i].Time != evB[i].Time {
			t.Fatalf("twin event %d differs:\n%+v\n%+v", i, evA[i], evB[i])
		}
	}

	// Mid-stage save/restore: snapshot inside a paced stage, restore into
	// a fresh sim, lockstep both — hashes and events stay identical.
	lockstep := func(t *testing.T, save *Save, a *Sim, ticks int) {
		t.Helper()
		b := build()
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
			if a.Hash() != b.Hash() {
				t.Fatalf("lockstep diverged at tick %d: %016x vs %016x", i, a.Hash(), b.Hash())
			}
		}
		if len(evA) != len(evB) {
			t.Fatalf("continuation events differ: %d vs %d", len(evA), len(evB))
		}
		for i := range evA {
			if evA[i].Kind != evB[i].Kind || evA[i].Phase != evB[i].Phase ||
				evA[i].Text != evB[i].Text || evA[i].Time != evB[i].Time {
				t.Fatalf("continuation event %d differs:\n%+v\n%+v", i, evA[i], evB[i])
			}
		}
	}

	// Departure beat: right after spawn, phase transit, no warp yet.
	{
		a := build()
		spawnCrossLane(a)
		stepN(a, 2)
		drain(a)
		save, err := a.Snapshot()
		if err != nil {
			t.Fatalf("snapshot at beat: %v", err)
		}
		lockstep(t, save, a, 900)
	}
	// Gate dwell: phase gate, standing at the gate.
	{
		a := build()
		spawnCrossLane(a)
		caught := false
		for i := 0; i < 3000 && !caught; i++ {
			a.Step()
			drain(a)
			for _, e := range sortedEntities(a.Traffic) {
				if tc, ok := a.Traffic.Get(e); ok && tc.Phase == TrafficGate {
					caught = true
					break
				}
			}
		}
		if !caught {
			t.Fatal("never reached the gate dwell")
		}
		save, err := a.Snapshot()
		if err != nil {
			t.Fatalf("snapshot at gate dwell: %v", err)
		}
		lockstep(t, save, a, 900)
	}
	// Turnaround: phase berth (post-cargo-action rest).
	{
		a := build()
		spawnTurnaround(a)
		caught := false
		for i := 0; i < 6000 && !caught; i++ {
			a.Step()
			drain(a)
			for _, e := range sortedEntities(a.Traffic) {
				if tc, ok := a.Traffic.Get(e); ok && tc.Phase == TrafficBerth {
					caught = true
					break
				}
			}
		}
		if !caught {
			t.Fatal("never reached the turnaround berth")
		}
		save, err := a.Snapshot()
		if err != nil {
			t.Fatalf("snapshot at berth: %v", err)
		}
		lockstep(t, save, a, 900)
	}
}
