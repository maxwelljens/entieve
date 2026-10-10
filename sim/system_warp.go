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

// Warp tunnel profile segment durations (seconds).
//
// EVE-style warp is a process, not a jump: after the align gate (point at
// the destination at >= 75% of base max speed) the ship flies an explicit
// tunnel — accelerate, cruise, decelerate — and drops out exactly on the
// landing point. The fixed segment timings give every hop a ~6.5 s tunnel
// floor so even a 10 km warp reads as travel and can never teleport; longer
// hops take strictly longer through the distance-scaled cruise term.
const (
	warpAccelTime = 2.0 // accel: along-speed ramps linearly 0 -> warpSpeed
	warpCruiseMin = 1.5 // cruise: at least this long, scales with distance
	warpDecelTime = 3.0 // decel: ramps linearly warpSpeed -> 0 onto the point
)

// warpProfile is the closed-form three-segment warp motion profile of one
// hop: segment boundaries (distances) are rebuilt from pure state (total
// distance + hull warp speed) every tick, while the along-speed itself is
// carried tick-to-tick in WarpDrive.Speed. Per-tick motion is fixed 1/60 s
// arithmetic with no RNG, so twin runs stay bit-identical.
type warpProfile struct {
	dist   float64 // total profile distance (m); the profile integrates to exactly this
	peak   float64 // cruise speed for this hop (m/s)
	accelD float64 // segment distances (m)
	decelD float64
}

// warpProfileFor builds the profile for a hop of d metres at hull warp
// speed v (m/s; WarpSpeed AU/s * 1.496e11). The full profile — warpAccelTime
// ramping up, warpCruiseMin at warpSpeed, warpDecelTime ramping down —
// covers 4*v metres at full speed. Shorter hops keep the segment timings but
// scale the segment distances (equivalently the peak speed) by k = d/(4v):
// the speed triangle is clamped by the distance, so the profile still
// integrates to exactly d (never overshoots the landing point) while every
// hop still spends the full 6.5 s of tunnel time (never teleports).
// Full-profile hops add their excess distance as cruise time.
func warpProfileFor(d, v float64) warpProfile {
	k := 1.0
	// Distance covered by the full-speed profile: accel + minimum cruise
	// + decel at warpSpeed.
	if full := v*(warpAccelTime+warpDecelTime)/2 + v*warpCruiseMin; d < full {
		k = d / full
	}
	peak := k * v
	return warpProfile{
		dist:   d,
		peak:   peak,
		accelD: peak * warpAccelTime / 2,
		decelD: peak * warpDecelTime / 2,
	}
}

// systemWarp runs the warp state machine: align -> warp -> land.
//
// Warp start requires EVE's classic rule: the ship must point at the
// destination (within warpAlignCos) AND reach 75% of its base max speed.
// The align phase is flown by normal sublight movement (system 2) toward
// WarpDrive.To. The warp phase then flies the three-segment tunnel profile
// (warpProfile) straight along the warp vector — WarpSpeed AU/s converts at
// 1.496e11 m per AU — and lands when within warpLandRange of the landing
// point, snapping onto it with zero velocity.
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
				cos := m.Vel.Dot(toGo) / (speed * dist)
				aligned = cos >= warpAlignCos
			}
			// Presentation state (warp UI): live distance to the landing
			// point and speed along the warp vector. Dist freezes at the
			// tunnel profile distance when the warp phase starts.
			wd.Dist = dist
			wd.Speed = 0
			if dist > 0 {
				if along := m.Vel.Dot(toGo) / dist; along > 0 {
					wd.Speed = along
				}
			}
			if aligned && speed >= 0.75*m.MaxSpeed {
				wd.Phase = "warp"
				wd.Speed = 0
				s.emit(Event{Kind: EvWarp, Phase: "start", Actor: e, Text: "Warp drive active"})
			}
			// While aligning, movement (system 2) already flies the ship at
			// the destination; nothing else to do here.
			continue

		case "warp":
			if dist <= warpLandRange {
				// Land: snap onto the destination, release the ship. A
				// course command recorded while the drive was engaged
				// (manual/approach/orbit) survives — it only becomes
				// effective now; otherwise stop dead on the landing point
				// (the warp's own align intent becomes a stop).
				tr.Pos = wd.To
				m.Vel = core.Vec2{}
				if in, ok := s.Intents.Get(e); !ok || in.Kind == IntentAlign {
					s.Intents.Set(e, Intent{Kind: IntentStop})
				}
				s.Warps.Remove(e)
				s.emit(Event{Kind: EvWarp, Phase: "land", Actor: e, Text: "Warp complete"})
				continue
			}
			// Dist is frozen at the profile distance when the tunnel
			// starts; the guard heals drives restored from saves written
			// before Dist existed (a zero would yield a degenerate
			// profile).
			if wd.Dist < dist {
				wd.Dist = dist
			}
			prof := warpProfileFor(wd.Dist, s.warpSpeedMS(e, m))
			dir := toGo.Scale(1 / dist)

			// The along-speed is carried from the previous warp tick in
			// wd.Speed: the accel ramp steps linearly 0 -> peak, cruise
			// holds peak, and the decel ramp is evaluated against the
			// remaining distance; segment membership comes from the
			// distances travelled/left.
			v := wd.Speed
			if v > prof.peak {
				v = prof.peak
			}
			var vNext float64
			switch {
			case wd.Dist-dist < prof.accelD:
				vNext = v + prof.peak*DT/warpAccelTime // linear 0 -> peak
				if vNext > prof.peak {
					vNext = prof.peak
				}
			case dist <= prof.decelD:
				// Closed form of the linear-in-time ramp against the
				// remaining distance: decel always hands over from cruise
				// (r == decelD gives exactly peak) and ends at 0 exactly on
				// the landing point, even when the cruise segment crossed
				// the boundary between ticks.
				vNext = math.Sqrt(2 * prof.peak * dist / warpDecelTime)
			default:
				vNext = prof.peak
			}
			// Trapezoid step: exact for the linear ramps, so the three
			// segments integrate to exactly wd.Dist. The clamp is the
			// float-error backstop — a step can never overshoot the point.
			step := (v + vNext) / 2 * DT
			if step > dist {
				step = dist
			}
			tr.Pos = tr.Pos.Add(dir.Scale(step))
			tr.Facing = dir.Angle()
			m.Vel = dir.Scale(vNext)
			wd.Speed = vNext
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
