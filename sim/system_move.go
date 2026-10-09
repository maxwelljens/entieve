package sim

import (
	"math"

	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// systemMovement advances every mobile entity one tick toward its desired
// velocity.
//
// EVE-style acceleration: the velocity may change by at most
// maxDeltaV = MaxSpeed * DT / alignTime per tick, with alignTime =
// Agility * 10 (documented simplification of EVE's inertia formula: a ship
// reaches 63% of max speed after alignTime seconds and 75% — the warp-start
// threshold — at 0.75 * alignTime). Webbed ships are clamped to their
// reduced effective max speed instantly.
func (s *Sim) systemMovement() {
	now := s.time
	for _, e := range sortedEntities(s.Motions) {
		m, ok := s.Motions.Get(e)
		if !ok {
			continue
		}
		tr, ok := s.Transforms.Get(e)
		if !ok {
			continue
		}
		// A docked ship does not move.
		if s.Docked.Has(e) {
			m.Vel = core.Vec2{}
			continue
		}
		// The warp state machine owns the entity while in warp; during the
		// align phase it still flies like normal sublight toward WarpDrive.To.
		warpPhase := ""
		if wd, ok := s.Warps.Get(e); ok {
			warpPhase = wd.Phase
			if warpPhase == "warp" {
				continue
			}
		}

		alignTime := m.Agility * 10
		if alignTime <= 0 {
			alignTime = 1
		}
		maxDeltaV := m.MaxSpeed * DT / alignTime
		maxEff := m.EffectiveMaxSpeed(now)

		var desired core.Vec2
		if warpPhase == "align" {
			// Warp alignment: full throttle directly at the destination.
			wd, _ := s.Warps.Get(e)
			to := wd.To.Sub(tr.Pos)
			if !to.IsZero() {
				desired = to.Norm().Scale(maxEff)
			}
		} else {
			desired = s.desiredVelocity(e, tr, m, maxEff, alignTime)
		}

		// Accelerate toward the desired velocity, limited by maxDeltaV.
		dv := desired.Sub(m.Vel)
		if l := dv.Len(); l > maxDeltaV {
			m.Vel = m.Vel.Add(dv.Scale(maxDeltaV / l))
		} else {
			m.Vel = desired
		}
		// Clamp to the effective max speed (web/prop).
		if l := m.Vel.Len(); l > maxEff && l > 0 {
			m.Vel = m.Vel.Scale(maxEff / l)
		}

		// Facing rotates toward the velocity (or the desired heading while
		// nearly stationary, so align/orbit start turning immediately).
		heading := m.Vel
		if heading.Len() < 0.01 {
			heading = desired
		}
		if !heading.IsZero() {
			turn := math.Pi / alignTime * DT // radians per tick
			tr.Facing = rotateToward(tr.Facing, heading.Angle(), turn)
		}

		tr.Pos = tr.Pos.Add(m.Vel.Scale(DT))
	}
}

// desiredVelocity computes the sublight target velocity for an entity's
// movement intent (no intent = stop).
func (s *Sim) desiredVelocity(e ecs.Entity, tr *Transform, m *Motion, maxEff, alignTime float64) core.Vec2 {
	in, ok := s.Intents.Get(e)
	if !ok {
		return core.Vec2{}
	}
	switch in.Kind {
	case IntentStop, "":
		return core.Vec2{}

	case IntentManual:
		if in.Dir.IsZero() {
			return core.Vec2{}
		}
		return in.Dir.Norm().Scale(maxEff * clamp(in.SpeedFrac, 0, 1))

	case IntentApproach:
		tp, ok := s.posOf(in.Target)
		if !ok {
			return core.Vec2{}
		}
		to := tp.Sub(tr.Pos)
		dist := to.Len()
		if dist <= approachStopRange {
			return core.Vec2{}
		}
		// Brake smoothly: scale the throttle down once the remaining distance
		// is within the current braking distance v²/(2a), so the ship settles
		// near the target instead of overshooting at full speed.
		accel := m.MaxSpeed / alignTime
		brake := m.Vel.Len2()/(2*accel) + approachStopRange
		frac := 1.0
		if dist < brake {
			frac = dist / brake
		}
		return to.Norm().Scale(maxEff * frac)

	case IntentOrbit:
		tp, ok := s.posOf(in.Target)
		if !ok {
			return core.Vec2{}
		}
		rangeM := in.Range
		if rangeM < orbitMinRange {
			rangeM = orbitMinRange
		}
		to := tp.Sub(tr.Pos) // self -> target
		dist := to.Len()
		if dist < 1 {
			return core.Vec2{}
		}
		// u points from the ship toward the target (inward radial).
		u := to.Scale(1 / dist)
		// Tangent circles the target; the radial error steers onto the
		// orbit radius: outside -> inward (+u), inside -> outward (-u).
		tangent := core.Vec2{X: -u.Y, Y: u.X}
		err := clamp((dist-rangeM)/rangeM, -1, 1)
		dir := tangent.Add(u.Scale(err))
		if dir.IsZero() {
			return core.Vec2{}
		}
		return dir.Norm().Scale(maxEff)

	case IntentAlign:
		tp, ok := s.posOf(in.Target)
		if !ok {
			return core.Vec2{}
		}
		return tp.Sub(tr.Pos).Norm().Scale(maxEff)
	}
	return core.Vec2{}
}

// rotateToward folds cur toward goal by at most step radians.
func rotateToward(cur, goal, step float64) float64 {
	d := wrapPi(goal - cur)
	if math.Abs(d) <= step {
		return goal
	}
	return cur + math.Copysign(step, d)
}

const (
	// approachStopRange is the distance (m) at which an approach intent
	// considers the target reached.
	approachStopRange = 50
	// orbitMinRange guards degenerate orbit ranges.
	orbitMinRange = 100
)
