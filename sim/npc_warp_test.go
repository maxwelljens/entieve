package sim

import (
	"math"
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// ---------------------------------------------------------------------------
// NPC warp physics: every NPC travel runs the player's warp process
// (align gate -> three-segment tunnel -> landing slide). These tests pin the
// physics per scenario: hull-dependent align law (Agility x 10 s, gate at
// 75% speed), tunnel duration from the shared profile constants, off-grid
// insertion for raiders and respawn replacements, patrol sortie travel,
// journey continuation, in-warp untargetability and scramble pinning. All
// expected numbers derive from system_warp.go / system_move.go — no
// duplicated magic values.
// ---------------------------------------------------------------------------

// warpTunnelExpected is the closed-form tunnel duration of a hop of d
// metres at warp speed v (m/s): the full profile is
// warpAccelTime + warpCruiseMin + warpDecelTime, minus the tail the
// warpLandRange landing tolerance swallows at the end of the decel ramp
// (integrating the linear decel ramp v(s) = sqrt(2*peak*s/warpDecelTime)
// from s = warpLandRange down to 0).
func warpTunnelExpected(d, v float64) float64 {
	prof := warpProfileFor(d, v)
	tail := 0.0
	if prof.peak > 0 {
		tail = 2 * math.Sqrt(warpDecelTime*warpLandRange/(2*prof.peak))
	}
	return warpAccelTime + warpCruiseMin + warpDecelTime - tail
}

// warpPhaseOf reads an entity's warp phase ("" when not in warp).
func warpPhaseOf(s *Sim, e ecs.Entity) string {
	wd, ok := s.Warps.Get(e)
	if !ok {
		return ""
	}
	return wd.Phase
}

// hullOf returns the ship definition of an entity's hull.
func hullOf(t *testing.T, s *Sim, e ecs.Entity) *content.ShipDef {
	t.Helper()
	sh, ok := s.Ships.Get(e)
	if !ok {
		t.Fatalf("entity %v has no ship state", e)
	}
	hull, ok := s.Content.Ship(sh.Hull)
	if !ok {
		t.Fatalf("no hull def for %q", sh.Hull)
	}
	return hull
}

// TestNpcHaulerLegRunsPlayerWarpProcess pins the convoy leg as a process:
// the hauler aligns for 0.75 x Agility x 10 s (the warp-start gate), flies
// the distance-scaled tunnel at its hull's WarpSpeed, and never translates
// instantaneously — every per-tick step stays inside the sublight/warp
// profile bound, the destination stays outside warpLandRange until the warp
// land event snaps it there, and the observed tunnel matches the closed
// form of the shared profile constants.
func TestNpcHaulerLegRunsPlayerWarpProcess(t *testing.T) {
	s := newTrafficSim(t)
	var evs []Event

	// Catch the FIRST bound-for-dock hauler at its spawn tick: departure
	// (warp issue) happens in that same tick, so the align clock starts now.
	var e ecs.Entity
	var tc *Traffic
	for i := 0; i < 3000 && e == 0; i++ {
		s.Step()
		evs = append(evs, drain(s)...)
		if h, htc, ok := findHauler(s, "cel-a-dock"); ok {
			e, tc = h, htc
		}
	}
	if e == 0 {
		t.Fatal("no delivering hauler within 3000 ticks")
	}

	dest := s.celEntity(tc.Dest)
	dp, ok := s.posOf(dest)
	if !ok {
		t.Fatal("no destination celestial")
	}
	hull := hullOf(t, s, e)
	m, _ := s.Motions.Get(e)

	alignGate := 0.75 * hull.Agility * 10 // 75% of base max speed from a standstill
	alignStepBound := m.MaxSpeed * DT     // sublight per-tick motion cap
	warpSpeed := s.warpSpeedMS(e, m)

	var tAlign, tWarp, tGone float64 = -1, -1, -1
	var warpDist, stepBound float64
	sawAlign, sawWarp := false, false
	prevPhase := ""

	for i := 0; i < 6000; i++ {
		phase := warpPhaseOf(s, e)
		if !s.World.Alive(e) {
			tGone = s.time
			break
		}
		tr, _ := s.Transforms.Get(e)
		tp := tr.Pos

		switch {
		case phase == "align" && !sawAlign:
			sawAlign, tAlign = true, s.time
		case phase == "warp" && !sawWarp:
			sawWarp, tWarp = true, s.time
			warpDist = tp.Dist(dp)
			if warpDist <= warpLandRange {
				t.Fatalf("warp engaged inside the landing tolerance (%v m) at t=%v", warpDist, s.time)
			}
			stepBound = warpStepBound(warpDist, warpSpeed)
		}
		if phase == "align" && sawWarp {
			t.Fatalf("align re-entered after the tunnel opened (t=%v)", s.time)
		}
		if phase != "warp" && tp.Dist(dp) <= warpLandRange {
			// The only arrival path is the warp-land snap (announced by
			// EvWarp "land"): no sublight sample before it may sit inside
			// the tolerance. (The final tunnel step legitimately ends
			// inside the tolerance — the next tick lands.)
			t.Fatalf("hauler inside the landing tolerance during %q at t=%v", phase, s.time)
		}
		prevPhase = phase

		s.Step()
		evs = append(evs, drain(s)...)

		// Per-tick displacement bound by the phase that flew the step: the
		// warp flip happens after movement, so the transition step is still
		// sublight.
		if np, ok := s.Transforms.Get(e); ok && s.World.Alive(e) {
			bound := alignStepBound
			if prevPhase == "warp" {
				bound = stepBound
			}
			if d := np.Pos.Dist(tp); d > bound+1e-3 {
				t.Fatalf("teleport: step %v m exceeds %v m bound in phase %q (t=%v)", d, bound, prevPhase, s.time)
			}
		}
	}
	if tGone < 0 {
		t.Fatal("hauler never completed the leg")
	}
	if !sawAlign || !sawWarp {
		t.Fatalf("phases observed: align=%v warp=%v, want both", sawAlign, sawWarp)
	}

	// Align law: the gate opens at 75% of base max speed —
	// alignTime = Agility x 10, gate at 0.75 x alignTime.
	if d := tWarp - tAlign; d < alignGate-2*DT || d > alignGate+4*DT {
		t.Errorf("align = %.2fs, want %.2fs (0.75 x Agility x 10)", d, alignGate)
	}
	// Tunnel: the shared three-segment profile, closed form.
	want := warpTunnelExpected(warpDist, warpSpeed)
	if d := tGone - tWarp; math.Abs(d-want) > 0.2 {
		t.Errorf("tunnel = %.2fs, want %.2fs (profile over %v m at %.0f m/s)", d, want, warpDist, warpSpeed)
	}

	// The arrival is a warp LAND, announced and delivered — the hauler
	// never stood inside the tolerance before the snap.
	land := findEvent(evs, EvWarp, "Warp complete")
	if land == nil || land.Actor != e {
		t.Fatalf("no warp land event for the hauler: %v", evs)
	}
	if tWarp >= land.Time {
		t.Fatalf("land at %.2f not after tunnel start %.2f", land.Time, tWarp)
	}
	delivered := firstPhase(evs, EvTraffic, TrafficDelivered)
	if delivered == nil || delivered.Actor != e {
		t.Fatalf("no delivery event for the hauler: %v", trafficTexts(evs))
	}
	if delivered.Time < land.Time {
		t.Fatalf("delivered at %.2f before the warp landed at %.2f", delivered.Time, land.Time)
	}
}

// TestRaiderInsertionWarpsInFromOffGrid pins raid as a warp-in: the raiders
// spawn off-grid on the hauler's warp path (WarpMinRange out), run the full
// align + tunnel process, and slide onto the fight grid's scatter slots —
// never a pop-in at the hauler. The hauler's own warp is interrupted the
// moment the ambush drops (interdiction semantics unchanged).
func TestRaiderInsertionWarpsInFromOffGrid(t *testing.T) {
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
	e, tc, _ := findHauler(s, "")
	ambPos, _ := s.posOf(e)
	forceAmbushOn(t, s, e, tc, "null") // no patrol: isolate the insertion

	raid := aliveRaiders(s)
	if len(raid) == 0 {
		t.Fatal("no raiders")
	}
	type ins struct {
		off   core.Vec2
		t0    float64
		tWarp float64
		tLand float64
		dist  float64 // frozen tunnel distance at warp entry
	}
	insOf := map[ecs.Entity]ins{}
	prevPos := map[ecs.Entity]core.Vec2{}
	for _, r := range raid {
		rp, _ := s.Transforms.Get(r)
		if d := rp.Pos.Dist(ambPos); d < WarpMinRange-trafficAmbushSpread-1 {
			t.Fatalf("raider spawned %v m from the fight — insertion must start off-grid (%v m out)", d, WarpMinRange)
		}
		if ph := warpPhaseOf(s, r); ph != "align" {
			t.Fatalf("raider phase = %q, want align (the warp process owns it)", ph)
		}
		rai, ok := s.AIs.Get(r)
		if !ok || rai.Home != ambPos {
			t.Fatalf("raider Home = %v, want the fight grid %v", rai.Home, ambPos)
		}
		insOf[r] = ins{off: rp.Pos, t0: s.time, tWarp: -1, tLand: -1}
		prevPos[r] = rp.Pos
	}
	if s.Warps.Has(e) {
		t.Fatal("hauler warp was not interrupted by the interdiction")
	}
	alignGate := 0.75 * hullOf(t, s, raid[0]).Agility * 10

	for i := 0; i < 1800; i++ {
		pending := false
		for _, r := range raid {
			o := insOf[r]
			if o.tLand > 0 {
				continue
			}
			pending = true
			ph := warpPhaseOf(s, r)
			switch ph {
			case "warp":
				if o.tWarp < 0 {
					o.tWarp = s.time
					if wd, ok := s.Warps.Get(r); ok {
						o.dist = wd.Dist
					}
				}
			case "align":
				// still aligning
			default:
				o.tLand = s.time
				insOf[r] = o
				continue
			}
			insOf[r] = o

			// No teleport: every step stays inside its phase's bound.
			rp, _ := s.Transforms.Get(r)
			rm, _ := s.Motions.Get(r)
			bound := rm.MaxSpeed * DT
			if ph == "warp" {
				bound = warpProfileFor(o.dist, s.warpSpeedMS(r, rm)).peak * DT
			}
			if d := rp.Pos.Dist(prevPos[r]); d > bound+1e-3 {
				t.Fatalf("raider teleport: %v m in %q (t=%v)", d, ph, s.time)
			}
			prevPos[r] = rp.Pos
		}
		if !pending {
			break
		}
		s.Step()
		evs = append(evs, drain(s)...)
	}
	for _, r := range raid {
		o := insOf[r]
		if o.tWarp < 0 || o.tLand < 0 {
			t.Fatalf("raider never completed the warp-in (align=%v land=%v)", o.tWarp >= 0, o.tLand >= 0)
		}
		if d := o.tWarp - o.t0; d < alignGate-2*DT {
			t.Errorf("raider align = %.2fs, want >= %.2fs (0.75 x Agility x 10)", d, alignGate)
		}
		want := warpTunnelExpected(o.dist, hullOf(t, s, r).WarpSpeed*1.496e11)
		if d := o.tLand - o.tWarp; math.Abs(d-want) > 0.25 {
			t.Errorf("raider tunnel = %.2fs, want %.2fs", d, want)
		}
		// Lands on its scatter slot around the fight — still off-grid
		// relative to where it started, never stacked on the hauler.
		landPos, ok := s.posOf(r)
		if !ok {
			t.Fatalf("raider %v dead before landing", r)
		}
		if d := landPos.Dist(ambPos); d > trafficAmbushSpread+1000 {
			t.Errorf("raider landed %v m from the fight, want <= %v m (scatter)", d, trafficAmbushSpread)
		}
		if d := landPos.Dist(o.off); d < WarpMinRange-2*trafficAmbushSpread-2000 {
			t.Errorf("raider landed only %v m from its insertion point", d)
		}
	}
}

// TestPatrolSortieTravelsToIncident pins the response wing's physical
// latency: patrols spawn at the anchor OUTSIDE engagement range, then warp
// to the incident through align + tunnel — travel time stacks on top of the
// (unchanged) 1800x(1-sigma) decision delay.
func TestPatrolSortieTravelsToIncident(t *testing.T) {
	s := newTrafficSim(t)
	var evs []Event
	if !runUntil(t, s, &evs, 900, func() bool {
		_, _, ok := findHauler(s, "cel-a-dock")
		return ok
	}) {
		t.Fatal("no delivering hauler within 900 ticks")
	}
	e, tc, _ := findHauler(s, "cel-a-dock")
	ambPos, _ := s.posOf(e)
	forceAmbushOn(t, s, e, tc, "high") // wing floor(3*sigma) = 3, delay 0
	s.Step()
	evs = append(evs, drain(s)...)
	resp := firstPhase(evs, EvTraffic, TrafficPatrolResponse)
	if resp == nil {
		t.Fatalf("no patrol response: %v", trafficTexts(evs))
	}
	tResp := resp.Time

	var wing []ecs.Entity
	s.Patrols.Each(func(p ecs.Entity, st *Patrol) {
		if st.State == PatrolRespond {
			wing = append(wing, p)
		}
	})
	if len(wing) == 0 {
		t.Fatal("no responding patrol")
	}
	for _, p := range wing {
		pp, _ := s.posOf(p)
		if pp.Dist(ambPos) <= trafficPatrolEngageRange {
			t.Fatalf("patrol spawned %v m from the incident (want beyond engage range %v)",
				pp.Dist(ambPos), trafficPatrolEngageRange)
		}
	}
	alignGate := 0.75 * hullOf(t, s, wing[0]).Agility * 10

	started := map[ecs.Entity]bool{}
	landedAt := map[ecs.Entity]float64{}
	sawWarp := false
	for i := 0; i < 1800; i++ {
		for _, p := range wing {
			if _, done := landedAt[p]; done {
				continue
			}
			ph := warpPhaseOf(s, p)
			if ph != "" {
				started[p] = true
				if ph == "warp" {
					sawWarp = true
				}
			} else if started[p] {
				landedAt[p] = s.time
			}
		}
		if len(landedAt) == len(wing) {
			break
		}
		s.Step()
		evs = append(evs, drain(s)...)
	}
	if !sawWarp {
		t.Fatal("no patrol ran the warp tunnel to the incident")
	}
	if len(landedAt) != len(wing) {
		t.Fatalf("only %d/%d patrols completed the sortie", len(landedAt), len(wing))
	}
	for _, p := range wing {
		d := landedAt[p] - tResp
		// Lower bound: align gate + tunnel floor. Upper bound: align gate +
		// full profile for a 20-30 km anchor hop, with tick slack.
		if d < alignGate+4.0 || d > alignGate+7.0 {
			t.Errorf("patrol sortie %.2fs after dispatch, want %.2f..%.2fs (align + tunnel)",
				d, alignGate+4.0, alignGate+7.0)
		}
		pp, _ := s.posOf(p)
		if pp.Dist(ambPos) > trafficPatrolEngageRange {
			t.Errorf("patrol landed %v m from the incident, beyond engage range %v",
				pp.Dist(ambPos), trafficPatrolEngageRange)
		}
	}
}

// TestRespawnReplacementSlidesInNearAnchor pins the respawn replacement's
// arrival: the timer still speaks at fire time, but the hull exists
// off-grid on the anchor's +X axis in the align phase and slides in through
// the warp tunnel to the exact static placement spot.
func TestRespawnReplacementSlidesInNearAnchor(t *testing.T) {
	s := newTestSim(t)
	victim := anomalyNPC(t, s)
	vai, _ := s.AIs.Get(victim)
	home := vai.Home
	before := map[ecs.Entity]bool{}
	for _, e := range sortedEntities(s.AIs) {
		before[e] = true
	}
	damageNPC(s, s.player, victim, contentDamage(), false)
	s.Step()
	drain(s)

	for i := 0; i < 60*70 && len(s.respawns) > 0; i++ {
		s.Step()
		drain(s)
	}
	if len(s.respawns) != 0 {
		t.Fatalf("respawn timer never fired: %+v", s.respawns)
	}
	var repl ecs.Entity
	for _, e := range sortedEntities(s.AIs) {
		if before[e] {
			continue
		}
		if rai, ok := s.AIs.Get(e); ok && rai.Npc == vai.Npc {
			repl = e
		}
	}
	if repl == 0 {
		t.Fatal("no replacement spawned")
	}
	tSpawn := s.time
	rp, _ := s.Transforms.Get(repl)
	spread := celestialSpreadRadius(s, "cel-anomaly-a")
	if d := rp.Pos.Dist(home); d < WarpMinRange-spread-1 {
		t.Fatalf("replacement spawned %v m from the anchor — want off-grid (~%v m)", d, WarpMinRange)
	}
	if ph := warpPhaseOf(s, repl); ph != "align" {
		t.Fatalf("replacement phase = %q at spawn, want align", ph)
	}
	rai, _ := s.AIs.Get(repl)
	if rai.Home != home {
		t.Fatalf("replacement Home = %v, want the anchor %v", rai.Home, home)
	}
	alignGate := 0.75 * hullOf(t, s, repl).Agility * 10

	var tWarp, tLand, warpDist float64 = -1, -1, 0
	for i := 0; i < 1800; i++ {
		ph := warpPhaseOf(s, repl)
		if ph == "warp" {
			if tWarp < 0 {
				tWarp = s.time
				if wd, ok := s.Warps.Get(repl); ok {
					warpDist = wd.Dist
				}
			}
		} else if ph == "" {
			if tWarp < 0 {
				t.Fatalf("replacement vanished without warping (t=%v)", s.time)
			}
			tLand = s.time
			break
		}
		s.Step()
		drain(s)
	}
	if tWarp < 0 || tLand < 0 {
		t.Fatalf("replacement never warped in (warp=%v land=%v)", tWarp >= 0, tLand >= 0)
	}
	if d := tWarp - tSpawn; d < alignGate-2*DT || d > alignGate+4*DT {
		t.Errorf("replacement align = %.2fs, want %.2fs (0.75 x Agility x 10)", d, alignGate)
	}
	m, _ := s.Motions.Get(repl)
	want := warpTunnelExpected(warpDist, s.warpSpeedMS(repl, m))
	if d := tLand - tWarp; math.Abs(d-want) > 0.2 {
		t.Errorf("replacement tunnel = %.2fs, want %.2fs", d, want)
	}
	rp, _ = s.Transforms.Get(repl)
	if d := rp.Pos.Dist(home); d > spread+1 {
		t.Errorf("replacement landed %v m from the anchor, want the static scatter (<= %v m)", d, spread)
	}
}

// TestJourneyResumeWarpsOnward pins the gate-hop continuation: a parked
// shipment respawns at the gate it legitimately landed at, then runs the
// full warp process onward to its destination — never an instant hop.
func TestJourneyResumeWarpsOnward(t *testing.T) {
	c := trafficContent()
	c.Traffic["sys-a"].Routes = nil // only the crafted journey flies
	s, err := New(c, testWorld(), 7)
	if err != nil {
		t.Fatal(err)
	}
	setPlayerPos(s, core.Vec2{X: -5e6, Y: -5e6})
	drain(s)

	gate := s.celEntity("cel-gate-b")
	gp, ok := s.posOf(gate)
	if !ok {
		t.Fatal("no gate")
	}
	s.trafficJourneys = append(s.trafficJourneys, TrafficJourney{
		System: "sys-a", At: gp, DestSys: "", DestCel: "cel-a-dock",
		Item: "min-ferrite", Qty: 100, Basis: 10, Hull: "ship-convoy",
	})
	var evs []Event
	// Resume is capped and staggered (trafficResumeInterval): the lone
	// journey fires within a few schedule windows of the first traffic
	// tick, never on a fixed tick — bound the wait accordingly.
	for i := 0; i < 4*trafficResumeInterval && len(s.trafficJourneys) > 0; i++ {
		s.Step()
		evs = append(evs, drain(s)...)
	}
	if len(s.trafficJourneys) != 0 {
		t.Fatal("journey never resumed")
	}
	e, tc, ok := findHauler(s, "cel-a-dock")
	if !ok {
		t.Fatal("resumed hauler missing")
	}
	sp, _ := s.posOf(e)
	if sp.Dist(gp) > 1 {
		t.Fatalf("resumed hauler at %v, want the gate at %v", sp, gp)
	}
	if tc.Item != "min-ferrite" || tc.Qty != 100 {
		t.Fatalf("resumed cargo = %v/%d", tc.Item, tc.Qty)
	}
	alignGate := 0.75 * hullOf(t, s, e).Agility * 10

	var tAlign, tWarp, tGone float64 = -1, -1, -1
	for i := 0; i < 3000; i++ {
		if !s.World.Alive(e) {
			tGone = s.time
			break
		}
		switch warpPhaseOf(s, e) {
		case "align":
			if tAlign < 0 {
				tAlign = s.time
			}
		case "warp":
			if tWarp < 0 {
				tWarp = s.time
			}
		}
		s.Step()
		evs = append(evs, drain(s)...)
	}
	if tGone < 0 {
		t.Fatal("resumed hauler never delivered")
	}
	if tAlign < 0 || tWarp < 0 {
		t.Fatalf("resumed hauler skipped the warp process (align=%v warp=%v)", tAlign >= 0, tWarp >= 0)
	}
	if d := tWarp - tAlign; d < alignGate-2*DT {
		t.Errorf("onward align = %.2fs, want >= %.2fs", d, alignGate)
	}
	if tGone < tWarp {
		t.Errorf("delivered before the tunnel (gone %v, warp %v)", tGone, tWarp)
	}
	if firstPhase(evs, EvTraffic, TrafficDelivered) == nil {
		t.Fatalf("no delivery: %v", trafficTexts(evs))
	}
}

// TestWarpingNpcIsUntargetable pins the targeting semantics: a ship in the
// warp phase drops every held lock and refuses every new lock attempt — the
// rule is symmetric (the player's warp does the same) — while align-phase
// ships stay lockable so scrams still interrupt warp start. An
// already-cycling weapon whose target enters warp never lands its shot.
func TestWarpingNpcIsUntargetable(t *testing.T) {
	s := newTrafficSim(t)
	var evs []Event

	npc := s.spawnNpc("npc-rat-scuttler", core.Vec2{X: 1e7, Y: 2e5}, 100)
	if npc == 0 {
		t.Fatal("spawn failed")
	}
	teleport(s, s.player, mustPos(s, npc).Add(core.Vec2{X: -2000}))
	near := mustPos(s, npc)
	far := near.Add(core.Vec2{X: 300000}) // short hop: motion stays in lock range
	if !s.warpToPoint(npc, far) {
		t.Fatal("warp refused")
	}

	// Align phase stays lockable: the lock is held and progressing while
	// the hull turns and accelerates toward the destination.
	s.Queue(CmdLock{Target: npc})
	for i := 0; i < 600 && warpPhaseOf(s, npc) != "warp"; i++ {
		s.Step()
		evs = append(evs, drain(s)...)
	}
	if warpPhaseOf(s, npc) != "warp" {
		t.Fatal("npc never entered the tunnel")
	}

	// In warp (one tick after the flip, still point-blank): held locks
	// drop — range cannot explain it here.
	tg, _ := s.Targetings.Get(s.player)
	for _, l := range tg.Locks {
		if l.Target == npc {
			t.Fatalf("held lock survived the warp flip: progress=%v", l.Progress)
		}
	}
	// New attempts never take either.
	s.Queue(CmdLock{Target: npc})
	s.Step()
	evs = append(evs, drain(s)...)
	tg, _ = s.Targetings.Get(s.player)
	for _, l := range tg.Locks {
		if l.Target == npc {
			t.Fatalf("lock on warping NPC took: progress=%v", l.Progress)
		}
	}

	// Symmetric: held lock progress on the warping PLAYER drops too.
	// (An NPC may re-ADD a zero-progress lock each tick — targeting drops
	// it again — so the assertion is on progress.)
	p := s.player
	tnpc := s.spawnNpc("npc-rat-scuttler", mustPos(s, p).Add(core.Vec2{X: 3000}), 100)
	pt, _ := s.Targetings.Get(tnpc)
	pt.Locks = append(pt.Locks, Lock{Target: p, Progress: 0.5})
	s.Warps.Set(p, WarpDrive{Phase: "warp", To: mustPos(s, p).Add(core.Vec2{X: 3e6})})
	for i := 0; i < 2; i++ {
		s.Step()
		drain(s)
		pt, _ = s.Targetings.Get(tnpc)
		for _, l := range pt.Locks {
			if l.Target == p && l.Progress > 0 {
				t.Fatalf("NPC held lock progress %v on the warping player", l.Progress)
			}
		}
	}
	s.Warps.Remove(p)

	// An already-cycling weapon never damages a target that entered warp:
	// the lock drops before the cycle completes, so checkCycle refuses.
	teleport(s, s.player, mustPos(s, npc).Add(core.Vec2{X: -2000}))
	s.Warps.Set(npc, WarpDrive{Phase: "warp", To: mustPos(s, npc).Add(core.Vec2{X: 300000})})
	forceLock(s, npc)
	s.Queue(CmdActivate{Slot: SlotRef{Class: content.SlotHigh, Index: 0}, Target: npc})
	s.Step()
	evs = append(evs, drain(s)...)
	// The activation must have succeeded (lock, range and capacitor are
	// all fine at #0) — otherwise the "no damage" assertions below would
	// prove nothing. The warp rule drops the lock before the first cycle
	// resolves, so the module is refused and goes quiet.
	activated := false
	for _, ev := range evs {
		if ev.Kind == EvModule && ev.Actor == s.player && strings.Contains(ev.Text, "activated") {
			activated = true
		}
	}
	if !activated {
		t.Fatalf("fixture: player turret never activated (test would pass vacuously): %v", evs)
	}
	sh0, ar0, st0 := shipVitals(s, npc)
	for i := 0; i < 600; i++ {
		teleport(s, s.player, mustPos(s, npc).Add(core.Vec2{X: -2000}))
		s.Step()
		evs = append(evs, drain(s)...)
	}
	sh1, ar1, st1 := shipVitals(s, npc)
	if sh1+ar1+st1 != sh0+ar0+st0 {
		t.Fatal("a weapon damaged a target mid-warp")
	}
	for _, ev := range evs {
		if ev.Kind == EvTargetLocked && ev.Actor == s.player && ev.Target == npc {
			t.Fatal("lock completed on a warping target")
		}
	}

	// On-grid portions of the warp (the accel slide out, the decel glide
	// in) still carry the "warp" phase: the easing changed where the ship
	// sits, not what the phase means. Sample into the glide — visibly on
	// grid, within warpGridSpan of the landing point — and re-prove the
	// refusal there: the phase -> lockability mapping is unchanged.
	s.Warps.Remove(npc)
	glideTo := mustPos(s, npc).Add(core.Vec2{X: 3e6})
	if !s.warpToPoint(npc, glideTo) {
		t.Fatal("re-warp refused")
	}
	inGlide := false
	for i := 0; i < 900; i++ {
		teleport(s, s.player, mustPos(s, npc).Add(core.Vec2{X: -2000}))
		ph := warpPhaseOf(s, npc)
		if ph == "warp" && mustPos(s, npc).Dist(glideTo) < warpGridSpan {
			inGlide = true
			break
		}
		if ph == "" {
			break // landed before the glide window was observed
		}
		s.Step()
		drain(s)
	}
	if !inGlide {
		t.Fatalf("never observed the on-grid decel glide (phase=%q)", warpPhaseOf(s, npc))
	}
	s.Queue(CmdLock{Target: npc})
	s.Step()
	drain(s)
	tg, _ = s.Targetings.Get(s.player)
	for _, l := range tg.Locks {
		if l.Target == npc {
			t.Fatalf("lock took during the on-grid glide (phase %q): progress=%v",
				warpPhaseOf(s, npc), l.Progress)
		}
	}
}

// TestHaulerWarpOutPinnedByScramble pins the interdiction semantics against
// the new travel timings: a scrambled hauler's warp-out attempt is refused
// every tick (decision timing unchanged), and once the scramble drops the
// hull warps clear as before.
func TestHaulerWarpOutPinnedByScramble(t *testing.T) {
	s := newTrafficSim(t)
	var evs []Event
	if !runUntil(t, s, &evs, 900, func() bool {
		_, _, ok := findHauler(s, "cel-a-dock")
		return ok
	}) {
		t.Fatal("no delivering hauler within 900 ticks")
	}
	e, tc, _ := findHauler(s, "cel-a-dock")
	forceAmbushOn(t, s, e, tc, "null")
	// The pin: an active scrambler, exactly what a landed raider applies.
	sh, _ := s.Ships.Get(e)
	sh.ScrambledUntil = s.time + 600
	tc.Attack.FleeAt = s.time // warp-out decision is due now

	for i := 0; i < 600; i++ {
		s.Step()
		evs = append(evs, drain(s)...)
	}
	if firstPhase(evs, EvTraffic, TrafficEscaped) != nil {
		t.Fatal("scrambled hauler warped out")
	}
	if !s.World.Alive(e) {
		t.Fatal("hauler died in the pin")
	}
	if tc.Attack.Outcome != "" {
		t.Fatalf("outcome = %q, want still running", tc.Attack.Outcome)
	}
	if findEvent(evs, EvWarp, "Warp scrambled") == nil {
		t.Fatalf("no warp-scram refusal observed: %v", evs)
	}

	// Scram drops: the same decision path warps clear.
	sh, _ = s.Ships.Get(e)
	sh.ScrambledUntil = 0
	if !runUntil(t, s, &evs, 3000, func() bool {
		return firstPhase(evs, EvTraffic, TrafficEscaped) != nil
	}) {
		t.Fatalf("hauler never warped clear after the scramble dropped: %v", trafficTexts(evs))
	}
}
