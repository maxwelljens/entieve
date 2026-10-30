package sim

import (
	"testing"

	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// ---------------------------------------------------------------------------
// Patrol wing formation: a response wing must not stack every hull on the
// incident point. Each responder owns a station on a ring around the
// incident (patrolStation), so the wing reads as a formation, not one ship.
// ---------------------------------------------------------------------------

// patrolWingResponse dispatches the high-sec wing at a forced ambush on a
// delivering hauler and returns the responders plus the incident's position
// at dispatch time.
func patrolWingResponse(t *testing.T, s *Sim) ([]ecs.Entity, core.Vec2) {
	t.Helper()
	var evs []Event
	if !runUntil(t, s, &evs, 900, func() bool {
		_, _, ok := findHauler(s, "cel-a-dock")
		return ok
	}) {
		t.Fatal("no delivering hauler within 900 ticks")
	}
	e, tc, _ := findHauler(s, "cel-a-dock")
	ambPos, _ := s.posOf(e)
	forceAmbushOn(t, s, e, tc, "high") // wing floor(3*sigma) = 10, delay 0
	s.Step()
	evs = append(evs, drain(s)...)
	if firstPhase(evs, EvTraffic, TrafficPatrolResponse) == nil {
		t.Fatalf("no patrol response: %v", trafficTexts(evs))
	}
	var wing []ecs.Entity
	s.Patrols.Each(func(p ecs.Entity, st *Patrol) {
		if st.State == PatrolRespond {
			wing = append(wing, p)
		}
	})
	if len(wing) < 3 {
		t.Fatalf("wing of %d responders, want a full high-sec wing", len(wing))
	}
	return wing, ambPos
}

// patrolWingLanding steps until every wing member has finished its sortie
// warp and returns the position each landed on.
func patrolWingLanding(t *testing.T, s *Sim, wing []ecs.Entity) map[ecs.Entity]core.Vec2 {
	t.Helper()
	started := map[ecs.Entity]bool{}
	landed := map[ecs.Entity]core.Vec2{}
	for i := 0; i < 2400 && len(landed) < len(wing); i++ {
		for _, p := range wing {
			if _, done := landed[p]; done {
				continue
			}
			if ph := warpPhaseOf(s, p); ph != "" {
				started[p] = true
			} else if started[p] {
				if pos, ok := s.posOf(p); ok {
					landed[p] = pos
				}
			}
		}
		if len(landed) == len(wing) {
			break
		}
		s.Step()
		drain(s)
	}
	if len(landed) != len(wing) {
		t.Fatalf("only %d/%d patrols completed the sortie", len(landed), len(wing))
	}
	return landed
}

// minSeparation is the smallest distance between any two of a wing's
// landing points.
func minSeparation(landed map[ecs.Entity]core.Vec2) float64 {
	var pts []core.Vec2
	for _, p := range landed {
		pts = append(pts, p)
	}
	min := 1e18
	for i := range pts {
		for j := i + 1; j < len(pts); j++ {
			if d := pts[i].Dist(pts[j]); d < min {
				min = d
			}
		}
	}
	return min
}

// TestPatrolWingLandsOnOwnStations: the high-sec wing's post-warp hold
// positions are pairwise separated — each ship owns a station on the ring
// around the incident instead of every hull landing on the incident point
// (which read as a single ship). The floor is the ring geometry: adjacent
// slots of a 10-ship even ring at patrolStationRMin are
// 2*RMin*sin(pi/10) ≈ 1113 m apart, so 1100 m holds for any wing ≤ 10.
func TestPatrolWingLandsOnOwnStations(t *testing.T) {
	s := newTrafficSim(t)
	wing, ambPos := patrolWingResponse(t, s)
	landed := patrolWingLanding(t, s, wing)

	if d := minSeparation(landed); d < 1100 {
		t.Fatalf("wing landed with a closest pair %v m apart, want >= 1100 m (stacked hulls)", d)
	}
	for p, pos := range landed {
		// Still on the fight grid and inside the engagement range: the ring
		// only spreads the hold, it never sends a ship away from the fight.
		if d := pos.Dist(ambPos); d > trafficPatrolEngageRange {
			t.Errorf("patrol %v landed %v m from the incident, beyond engage range %v",
				p, d, trafficPatrolEngageRange)
		}
	}
}

// TestPatrolWingResponseTwinRuns pins the ring's determinism: two identical
// wing responses land on identical positions and keep identical hashes, so
// the station ring draws nothing from any RNG stream.
func TestPatrolWingResponseTwinRuns(t *testing.T) {
	drive := func(t *testing.T) (map[ecs.Entity]core.Vec2, uint64) {
		s := newTrafficSim(t)
		wing, _ := patrolWingResponse(t, s)
		landed := patrolWingLanding(t, s, wing)
		stepN(s, 300)
		return landed, s.Hash()
	}
	a, ha := drive(t)
	b, hb := drive(t)
	for p, pos := range a {
		if other, ok := b[p]; !ok || other != pos {
			t.Fatalf("twin wing landed differently for %v: %+v vs %+v", p, pos, b[p])
		}
	}
	if ha != hb {
		t.Fatalf("twin wing runs diverged: %016x vs %016x", ha, hb)
	}
}
