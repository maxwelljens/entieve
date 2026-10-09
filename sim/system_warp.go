package sim

import (
	"math"

	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// warpLandRange is the distance (m) at which the warp drive catches its
// destination and lands (EVE lands within 2,500 m; we land exactly on the
// destination point).
const warpLandRange = 2500

// warpAlignCos is the cosine of the maximum facing error allowed for warp
// start (~20°, EVE-ish "pointed at the destination").
const warpAlignCos = 0.94 // cos(0.35 rad)

// warpScramGrace extends scramble/web coverage one tick past a cycle so
// movement never sees an unintentional one-tick gap between refreshes.
const warpScramGrace = DT

// systemWarp runs the warp state machine: align -> warp -> land.
//
// Warp start requires EVE's classic rule: the ship must point at the
// destination (within warpAlignCos) AND reach 75% of its base max speed.
// The warp profile accelerates at a = 2 * warpSpeed (m/s²) while capping
// the instantaneous target speed at sqrt(2*a*toGo) so the ship can always
// brake inside the remaining distance; WarpSpeed AU/s converts at
// 1.496e11 m per AU. Landing snaps the ship onto the destination with zero
// velocity.
func (s *Sim) systemWarp() {
	now := s.time
	for _, e := range sortedEntities(s.Warps) {
		wd, ok := s.Warps.Get(e)
		if !ok {
			continue
		}
		tr, ok := s.Transforms.Get(e)
		if !ok {
			s.Warps.Remove(e)
			continue
		}
		m, hasMotion := s.Motions.Get(e)
		ship, hasShip := s.Ships.Get(e)

		// Warp scrambling cancels an engaged warp and blocks warp start.
		if hasShip && ship.ScrambledUntil > now {
			s.Warps.Remove(e)
			if hasMotion {
				// Drop out of warp at sublight speed along the warp vector.
				if wd.Phase == "warp" {
					if v := m.Vel; !v.IsZero() {
						m.Vel = v.Norm().Scale(m.EffectiveMaxSpeed(now))
					}
				}
			}
			s.Intents.Set(e, Intent{Kind: IntentStop})
			s.emit(Event{Kind: EvWarp, Phase: "scrammed", Severity: SevWarning, Actor: e,
				Text: "Warp scrambled!"})
			continue
		}
		if !hasMotion {
			s.Warps.Remove(e)
			continue
		}

		toGo := wd.To.Sub(tr.Pos)
		dist := toGo.Len()

		switch wd.Phase {
		case "align":
			// EVE rule: 75% of base max speed and pointed at the destination.
			speed := m.Vel.Len()
			aligned := false
			if !toGo.IsZero() && speed > 0 {
				cos := m.Vel.Dot(toGo) / (speed * toGo.Len())
				aligned = cos >= warpAlignCos
			}
			if aligned && speed >= 0.75*m.MaxSpeed {
				wd.Phase = "warp"
				s.emit(Event{Kind: EvWarp, Phase: "start", Actor: e, Text: "Warp drive active"})
			}
			// While aligning, movement (system 2) already flies the ship at
			// the destination; nothing else to do here.
			continue

		case "warp":
			if dist <= warpLandRange {
				// Land: snap onto the destination, zero velocity, stop.
				tr.Pos = wd.To
				m.Vel = core.Vec2{}
				s.Intents.Set(e, Intent{Kind: IntentStop})
				s.Warps.Remove(e)
				s.emit(Event{Kind: EvWarp, Phase: "land", Actor: e, Text: "Warp complete"})
				continue
			}
			warpSpeed := s.warpSpeedMS(e, m)
			accel := 2 * warpSpeed
			// Cap so the ship can brake within the remaining distance:
			// v <= sqrt(2 * a * toGo).
			v := m.Vel.Len()
			desired := math.Min(warpSpeed, math.Sqrt(2*accel*dist))
			if v < desired {
				v = math.Min(desired, v+accel*DT)
			} else if v > desired {
				v = math.Max(desired, v-accel*DT)
			}
			dir := toGo.Scale(1 / dist)
			// Land this tick when the step would overshoot the land range.
			if v*DT+float64(warpLandRange) >= dist {
				v = dist / DT
				if v > warpSpeed {
					v = warpSpeed
				}
			}
			m.Vel = dir.Scale(v)
			tr.Facing = dir.Angle()
			tr.Pos = tr.Pos.Add(m.Vel.Scale(DT))
		}
	}
}

// warpSpeedMS converts the hull's WarpSpeed (AU/s) to m/s: 1 AU = 1.496e11 m.
func (s *Sim) warpSpeedMS(e ecs.Entity, m *Motion) float64 {
	// Motion does not carry warp speed; read it from the hull definition.
	if ship, ok := s.Ships.Get(e); ok {
		if hull, ok := s.Content.Ship(ship.Hull); ok && hull.WarpSpeed > 0 {
			return hull.WarpSpeed * 1.496e11
		}
	}
	return 1.496e11 // fallback: 1 AU/s
}
