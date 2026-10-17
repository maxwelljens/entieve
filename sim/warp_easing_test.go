package sim

import (
	"testing"

	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// ---------------------------------------------------------------------------
// Warp spatial easing: the phase TIMELINE is frozen (align gate, segment
// durations, landing tick — see TestWarpTimelineFrozen), while the ship's
// on-grid position eases through the warp's fixed time budget: accel is a
// visible slide out along the departure vector, cruise runs off-grid, decel
// is a visible glide in that ends on the landing tolerance. These tests
// pin the position SHAPE on both ends — player and NPC — and the frozen
// timeline the tour relies on.
// ---------------------------------------------------------------------------

// warpStepBound is the largest per-tick on-grid step a hop of d metres may
// show: the profile step, scaled by the cruise slack factor on eased hops
// (accel/decel slides never exceed the profile step; only the off-grid
// cruise carries the slack absorbed by the capped slides).
func warpStepBound(d, v float64) float64 {
	prof := warpProfileFor(d, v)
	scale := 1.0
	if prof.accelD > warpGridSpan || prof.decelD > warpGridSpan {
		A, D := prof.accelD, prof.decelD
		if A > warpGridSpan {
			A = warpGridSpan
		}
		if D > warpGridSpan {
			D = warpGridSpan
		}
		scale = (d - A - D) / (d - prof.accelD - prof.decelD)
	}
	return prof.peak * DT * scale
}

// warpShape is one sampled tick of a warp: where the ship was.
type warpShape struct {
	phase string
	pos   core.Vec2
}

// warpShapeUntil samples (phase, pos) every tick until the entity's warp
// phase opens and then closes again (or the entity dies, for haulers that
// deliver on the landing tick).
func warpShapeUntil(s *Sim, e ecs.Entity, maxTicks int) []warpShape {
	var out []warpShape
	seenWarp := false
	for i := 0; i < maxTicks; i++ {
		if !s.World.Alive(e) {
			return out
		}
		tr, ok := s.Transforms.Get(e)
		if !ok {
			return out
		}
		ph := warpPhaseOf(s, e)
		out = append(out, warpShape{phase: ph, pos: tr.Pos})
		if ph == "warp" {
			seenWarp = true
		} else if seenWarp {
			return out
		}
		s.Step()
		drain(s)
	}
	return out
}

// warpWindow returns the indices of the run of "warp" samples (assumed one
// contiguous run) and the index right after it (the landed/gone tick).
func warpWindow(sh []warpShape) (first, last int) {
	first, last = -1, -1
	for i, sm := range sh {
		if sm.phase == "warp" {
			if first < 0 {
				first = i
			}
			last = i
		} else if first >= 0 {
			return first, last
		}
	}
	return first, last
}

// assertEasedWarp is the shared continuity gate over sampled warp positions:
// dest is the landing point, d0 the engage distance, speed the hull warp
// speed (m/s). It returns the warp-window bounds for phase-specific pins.
func assertEasedWarp(t *testing.T, name string, sh []warpShape, dest core.Vec2, d0, speed float64, accelTicks int) {
	t.Helper()
	first, last := warpWindow(sh)
	if first < 0 || last-first < accelTicks+150 {
		t.Fatalf("%s: warp window too short: first=%d last=%d", name, first, last)
	}
	bound := warpStepBound(d0, speed)

	prevDist := d0
	for k := first; k <= last; k++ {
		dist := sh[k].pos.Dist(dest)
		// Strictly between the endpoints: never stamped at the engage
		// point, never at the destination while still in warp, never
		// inside the landing tolerance except on the trigger sample.
		if dist <= 0 {
			t.Fatalf("%s: sample %d sits ON the destination mid-warp", name, k)
		}
		if dist < warpLandRange && k != last {
			t.Fatalf("%s: sample %d inside the landing tolerance (%v m) before the final approach", name, k, dist)
		}
		// Monotone along the vector: strictly closing every tick.
		if k > first && dist >= prevDist {
			t.Fatalf("%s: distance grew at sample %d: %v -> %v", name, k, prevDist, dist)
		}
		// Bounded per-tick displacement (no snap).
		if k > first {
			step := sh[k].pos.Dist(sh[k-1].pos)
			if step > bound+1e-3 {
				t.Fatalf("%s: step %v m exceeds bound %v m at sample %d", name, step, bound, k)
			}
			// Collinear with the arrival vector: the distance closed IS
			// the displacement (motion along the vector, no lateral jump).
			if closed := prevDist - dist; step < closed-1e-3 || step > closed+1e-3 {
				t.Fatalf("%s: motion not along the warp vector at sample %d: step %v vs closed %v",
					name, k, step, closed)
			}
		}
		prevDist = dist
	}

	// Accel slide out: the first accelTicks warp steps ramp IN (speed
	// rises with the phase) and cover exactly the capped grid span.
	accelDisp := sh[first].pos.Dist(dest) - sh[first+accelTicks].pos.Dist(dest)
	if prof := warpProfileFor(d0, speed); prof.accelD > warpGridSpan {
		if accelDisp < warpGridSpan-1 || accelDisp > warpGridSpan+1 {
			t.Errorf("%s: accel slide covered %v m, want the grid span %v m", name, accelDisp, warpGridSpan)
		}
	}
	for k := first + 1; k <= first+accelTicks; k++ {
		prev := sh[k-1].pos.Dist(sh[k].pos)
		cur := sh[k].pos.Dist(sh[k+1].pos)
		if cur < prev-1e-6*(prev+1) { // ramp-in, float-noise tolerant
			t.Errorf("%s: accel step shrank at sample %d: %v -> %v", name, k, prev, cur)
		}
	}

	// Decel glide in: the samples inside warpGridSpan form a long,
	// ease-out descent (steps never grow) down to the landing tolerance.
	glide := 0
	glidePrev := -1.0
	for k := first; k <= last; k++ {
		dist := sh[k].pos.Dist(dest)
		if dist >= warpGridSpan {
			continue
		}
		glide++
		if k > first {
			step := sh[k-1].pos.Dist(sh[k].pos)
			if glidePrev >= 0 && step > glidePrev+1e-6*(glidePrev+1) {
				t.Errorf("%s: glide step grew (no ease-out) at sample %d: %v -> %v", name, k, glidePrev, step)
			}
			glidePrev = step
		}
	}
	if glide < 150 {
		t.Errorf("%s: glide in lasted %d ticks, want >= 150 (a visible descent, not a stamp)", name, glide)
	}
}

// TestWarpAccelDecelGridSlidesPlayer pins the player-side easing: on the
// fixture's long hop the warp's fixed 120 accel ticks play as a grid-scale
// slide out to warpGridSpan, and the fixed decel ticks as an ease-out glide
// from warpGridSpan to the landing tolerance — strictly between the
// endpoints, monotone along the arrival vector, bounded per tick.
func TestWarpAccelDecelGridSlidesPlayer(t *testing.T) {
	s := newTestSim(t)
	gate := entityByName(s, "Gate to Beta")
	dp, ok := s.posOf(gate)
	if !ok {
		t.Fatal("no gate")
	}
	m, _ := s.Motions.Get(s.player)
	speed := s.warpSpeedMS(s.player, m)
	d0 := mustPos(s, s.player).Dist(dp)
	prof := warpProfileFor(d0, speed)
	if prof.accelD <= warpGridSpan {
		t.Fatalf("fixture: hop %v m not eased (accelD %v <= grid span) — test is vacuous", d0, prof.accelD)
	}

	s.Queue(CmdWarpTo{Target: gate})
	sh := warpShapeUntil(s, s.player, 1800)
	assertEasedWarp(t, "player", sh, dp, d0, speed, 120)

	// Landing is exact: the landed sample sits on the destination point.
	last := len(sh) - 1
	if sh[last].pos != dp {
		t.Errorf("landed at %v, want exactly %v", sh[last].pos, dp)
	}
}

// TestWarpAccelDecelGridSlidesNpc is the same continuity gate on an NPC
// path: the first convoy hauler's leg flies through the identical shared
// warp machinery (warpToPoint -> systemWarp).
func TestWarpAccelDecelGridSlidesNpc(t *testing.T) {
	s := newTrafficSim(t)
	var evs []Event
	if !runUntil(t, s, &evs, 900, func() bool {
		_, _, ok := findHauler(s, "cel-a-dock")
		return ok
	}) {
		t.Fatal("no delivering hauler within 900 ticks")
	}
	e, tc, _ := findHauler(s, "cel-a-dock")
	dp, ok := s.posOf(s.celEntity(tc.Dest))
	if !ok {
		t.Fatal("no destination")
	}
	m, _ := s.Motions.Get(e)
	speed := s.warpSpeedMS(e, m)
	d0 := mustPos(s, e).Dist(dp)
	prof := warpProfileFor(d0, speed)
	if prof.accelD <= warpGridSpan {
		t.Fatalf("fixture: hauler hop %v m not eased — test is vacuous", d0)
	}

	sh := warpShapeUntil(s, e, 3000)
	assertEasedWarp(t, "hauler", sh, dp, d0, speed, 120)
	// The hauler delivers on the landing tick: the sample trail ends with
	// the warp window (the entity is gone afterwards) — delivery itself is
	// pinned by TestNpcHaulerLegRunsPlayerWarpProcess.
	if last := len(sh) - 1; sh[last].phase == "warp" && sh[last].pos.Dist(dp) > warpLandRange {
		t.Fatalf("hauler trail ends mid-approach at %v m out", sh[last].pos.Dist(dp))
	}
}

// TestWarpTimelineFrozen pins the exact tick stamps the tour and the
// pre-easing build produced, per scenario: engage tick, land tick and the
// warp duration in ticks. ANY movement of these numbers is drift.
// Baseline (captured on the unmodified build): player 180/564 (384 warp
// ticks), traffic hauler 426/809 (383), respawn slide 3826/4178 (352).
// The hauler's START moved with the paced departure beat (740/1123 — the
// spawn tick plus its jittered beat); the 383-tick duration is frozen
// exactly as before: the beat delays commencement, never warp physics.
func TestWarpTimelineFrozen(t *testing.T) {
	t.Run("player", func(t *testing.T) {
		s := newTestSim(t)
		gate := entityByName(s, "Gate to Beta")
		s.Queue(CmdWarpTo{Target: gate})
		var startT, landT float64 = -1, -1
		for i := 0; i < 1800 && landT < 0; i++ {
			s.Step()
			for _, ev := range drain(s) {
				if ev.Kind != EvWarp || ev.Actor != s.player {
					continue
				}
				if ev.Phase == "start" && startT < 0 {
					startT = ev.Time
				}
				if ev.Phase == "land" && landT < 0 {
					landT = ev.Time
				}
			}
		}
		assertWarpStamps(t, startT, landT, 180, 564, 384)
	})

	t.Run("hauler", func(t *testing.T) {
		s := newTrafficSim(t)
		var startT, landT float64 = -1, -1
		for i := 0; i < 3000 && landT < 0; i++ {
			s.Step()
			for _, ev := range drain(s) {
				if ev.Kind == EvWarp && ev.Phase == "start" && startT < 0 {
					startT = ev.Time
				}
				if ev.Kind == EvWarp && ev.Phase == "land" && landT < 0 {
					landT = ev.Time
				}
			}
		}
		assertWarpStamps(t, startT, landT, 740, 1123, 383)
	})

	t.Run("respawn-slide", func(t *testing.T) {
		s := newTestSim(t)
		victim := anomalyNPC(t, s)
		vai, _ := s.AIs.Get(victim)
		damageNPC(s, s.player, victim, contentDamage(), false)
		s.Step()
		drain(s)
		var startT, landT float64 = -1, -1
		var repl ecs.Entity
		for i := 0; i < 60*80 && landT < 0; i++ {
			s.Step()
			drain(s)
			if repl == 0 {
				for _, e := range sortedEntities(s.AIs) {
					ai, ok := s.AIs.Get(e)
					if !ok || ai.Npc != vai.Npc || e == victim {
						continue
					}
					if wd, ok := s.Warps.Get(e); ok && wd.Phase == "warp" {
						startT = s.time
						repl = e
						break
					}
				}
				continue
			}
			if !s.Warps.Has(repl) {
				landT = s.time
			}
		}
		assertWarpStamps(t, startT, landT, 3826, 4178, 352)
	})
}

// assertWarpStamps checks the exact baseline tick stamps (pre-easing build)
// for one scenario.
func assertWarpStamps(t *testing.T, startT, landT float64, wantStart, wantLand, wantDur int64) {
	t.Helper()
	if startT < 0 || landT < 0 {
		t.Fatalf("stamps missing: start=%.4f land=%.4f", startT, landT)
	}
	st, ln := int64(startT*60+0.5), int64(landT*60+0.5)
	if st != wantStart || ln != wantLand || ln-st != wantDur {
		t.Errorf("warp stamps start=%d land=%d (%d ticks), want %d/%d (%d) — TIMELINE DRIFT",
			st, ln, ln-st, wantStart, wantLand, wantDur)
	}
}

// TestWarpTraveledHealsPreEasingSaves: a drive restored from a save
// written before Traveled existed carries a tunnel-trajectory position and
// Speed > 0 — the first warp tick recovers the progress from the projected
// remaining distance, so the phase timeline continues unchanged.
func TestWarpTraveledHealsPreEasingSaves(t *testing.T) {
	s := newTestSim(t)
	gate := entityByName(s, "Gate to Beta")
	gp, ok := s.posOf(gate)
	if !ok {
		t.Fatal("no gate")
	}
	// Lay the ship a third of the way along the vector, mid-warp, with a
	// pre-easing drive: no Traveled, frozen Dist, carried Speed.
	start := core.Vec2{X: 1e7, Y: 500}
	teleport(s, s.player, start.Add(gp.Sub(start).Scale(1.0/3.0)))
	m, _ := s.Motions.Get(s.player)
	speed := s.warpSpeedMS(s.player, m)
	d0 := start.Dist(gp)
	prof := warpProfileFor(d0, speed)
	s.Warps.Set(s.player, WarpDrive{Phase: "warp", To: gp, Dist: d0, Speed: prof.peak})

	s.Step()
	drain(s)
	wd, ok := s.Warps.Get(s.player)
	if !ok {
		t.Fatal("drive vanished")
	}
	// The healed progress is the distance already flown: one third of the
	// profile (plus this tick's own step).
	want := d0 / 3
	if wd.Traveled < want-prof.peak*DT-1 || wd.Traveled > want+prof.peak*DT+1 {
		t.Errorf("healed Traveled = %v, want ≈ %v", wd.Traveled, want)
	}
}
