package sim

import (
	"math"

	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/format"
)

// warpLandRange is the distance (m) at which the warp drive catches its
// destination and lands (EVE lands within 2,500 m; we land exactly on the
// destination point).
const warpLandRange = 2500

// WarpMinRange is the minimum distance (m) between the ship and a warp
// target at the moment the command is issued — EVE's rule: the warp
// drive cannot engage anything closer than 150 km. CmdWarpTo refuses
// inside it (command.go), and the UI disables the Warp action on the
// same constant, so sim is the single source of the number.
const WarpMinRange = 150_000

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

// warpGridSpan bounds how far a warp's visible accel/decel slides reach on
// grid: the on-grid sensor radius, so a slide-out burns away to the edge of
// the grid and a glide-in appears at the destination's edge and eases all
// the way in. Hops whose profile segments already fit inside the span fly
// the original tunnel trajectory bit for bit; longer hops split the SAME
// segment times between a grid-scale slide (accel/decel) and an off-grid
// cruise that carries the bulk.
const warpGridSpan = SensorRange

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

// warpDisplayRemaining maps the tunnel distance still to fly (r, the shadow
// progress wd.Dist-wd.Traveled) onto the distance the ship still LOOKS
// from its landing point on grid. The warp position is always this value
// metres back along the warp vector, so the profile's fixed segment times
// play out as visible motion on both ends of every hop:
//
//	accel — from the engage point along the departure vector, distance
//	        ramping with the phase (speed = the phase's), capped at
//	        warpGridSpan (kA-scaled: the ship burns off the edge of the
//	        grid, it does not vanish at warp engage);
//	cruise — off-grid: carries the slack (C x profile speed) between the
//	        two grid slides, the "in warp" part;
//	decglide — from warpGridSpan out, a linear re-scale of the remaining
//	        distance down to warpLandRange: the ease-out velocity SHAPE of
//	        the profile's decel ramp, played at grid scale, so the ship
//	        glides in and is never stamped at its endpoint early; land()
//	        then settles the last warpLandRange tolerance exactly as before.
//
// The map is continuous (accel ends where cruise starts at d-accelD*kA,
// cruise ends where the glide starts at Dspan, the glide ends at
// warpLandRange) and monotone. When both segments fit inside warpGridSpan
// the warp branch skips this map entirely and flies the pre-easing
// trajectory (step for step), so short hops are unchanged byte for byte.
func warpDisplayRemaining(d, r float64, prof warpProfile) float64 {
	kA := 1.0
	if prof.accelD > warpGridSpan {
		kA = warpGridSpan / prof.accelD
	}
	Dspan := prof.decelD
	if Dspan > warpGridSpan {
		Dspan = warpGridSpan
	}
	switch {
	case d-r < prof.accelD:
		// Accel slide out (r + ... reduces to r exactly when kA == 1).
		return r + (d-r)*(1-kA)
	case r <= prof.decelD:
		// Glide in: linear in the remaining tunnel distance, pinned to
		// warpLandRange at both ends (Dspan at tunnel exit, the landing
		// tolerance where land() takes over). This map is only consulted
		// when a segment exceeded warpGridSpan, so decelD > warpGridSpan
		// > warpLandRange here — the denominator is never zero.
		return warpLandRange + (Dspan-warpLandRange)*(r-warpLandRange)/(prof.decelD-warpLandRange)
	default:
		// Cruise absorbs whatever the two capped slides left behind.
		cruiseD := d - prof.accelD - prof.decelD // > 0: accelD+decelD = 0.625d (scaled) or <= d-1.5v (full)
		C := (d - prof.accelD*kA - Dspan) / cruiseD
		return d - prof.accelD*kA - (d-r-prof.accelD)*C
	}
}

// systemWarp runs the warp state machine: align -> warp -> land.
//
// Warp start requires EVE's classic rule: the ship must point at the
// destination (within warpAlignCos) AND reach 75% of its base max speed.
// The align phase is flown by normal sublight movement (system 2) toward
// WarpDrive.To, and lands straight through the same warpLandRange tolerance
// when the landing point is already within it (backstop: a sub-tolerance hop
// can never satisfy the gate). The warp phase then flies the three-segment
// tunnel profile (warpProfile) straight along the warp vector — WarpSpeed
// AU/s converts at 1.496e11 m per AU — and lands when within warpLandRange
// of the landing point, snapping onto it with zero velocity.
//
// Spatial easing: the tunnel kinematics integrate the shadow progress
// wd.Traveled (bit-identical arithmetic to the pre-easing position), while
// the ship's on-grid position is read back through warpDisplayRemaining —
// accel slides visibly off the grid, cruise runs off-grid, decel glides
// visibly onto the grid and land() settles the last tolerance. Same phase
// timeline, same events, same ticks; only where the ship sits per tick
// during accel/decel changes.
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

		// land releases the drive: snap onto the landing point, kill the
		// velocity, stop the ship (a course command recorded while the drive
		// was engaged survives — it only becomes effective now) and emit the
		// landing event. Shared by the align backstop and the warp phase.
		land := func() {
			tr.Pos = wd.To
			m.Vel = core.Vec2{}
			if in, ok := s.Intents.Get(e); !ok || in.Kind == IntentAlign {
				s.Intents.Set(e, Intent{Kind: IntentStop})
			}
			s.Warps.Remove(e)
			s.emit(Event{Kind: EvWarp, Phase: "land", Actor: e, Text: "Warp complete"})
		}

		switch wd.Phase {
		case "align":
			// Backstop: a landing point within the landing tolerance of the
			// ship (a sub-2.5 km hop, or a caller whose target is nearly
			// coincident with the ship) can never satisfy the align gate —
			// the ship reaches the point before 75% speed, or oscillates
			// around it, and the drive would sit in "align" forever with
			// CmdStop and re-warp refused. Land exactly like the warp phase
			// does instead; the player path never gets here (CmdWarpTo
			// refuses inside WarpMinRange).
			if dist <= warpLandRange {
				land()
				continue
			}
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
				wd.Traveled = 0 // tunnel progress starts at the engage point
				s.emit(Event{Kind: EvWarp, Phase: "start", Actor: e, Text: "Warp drive active"})
			}
			// While aligning, movement (system 2) already flies the ship at
			// the destination; nothing else to do here.
			continue

		case "warp":
			if dist <= warpLandRange {
				land()
				continue
			}
			// Dist is frozen at the profile distance when the tunnel
			// starts; the guard heals drives restored from saves written
			// before Dist existed (a zero would yield a degenerate
			// profile).
			if wd.Dist < dist {
				wd.Dist = dist
			}
			// Traveled carries the tunnel progress (see WarpDrive). Saves
			// written before it existed hold tunnel-trajectory positions,
			// so the projected remaining distance recovers the progress.
			if wd.Traveled <= 0 && wd.Speed > 0 {
				wd.Traveled = wd.Dist - dist
				if wd.Traveled < 0 {
					wd.Traveled = 0
				}
			}
			prof := warpProfileFor(wd.Dist, s.warpSpeedMS(e, m))
			dir := toGo.Scale(1 / dist)

			// The kinematics below integrate the SHADOW remaining distance
			// r (the tunnel distance still to fly) with exactly the
			// arithmetic the position itself used to carry: same v carry,
			// same segment branches, same trapezoid step — so segment
			// boundaries, warp presentation (wd.Speed) and the landing
			// tick stay on the pre-easing timeline. The along-speed is
			// carried from the previous warp tick in wd.Speed: the accel
			// ramp steps linearly 0 -> peak, cruise holds peak, and the
			// decel ramp is evaluated against the remaining distance.
			r := wd.Dist - wd.Traveled
			v := wd.Speed
			if v > prof.peak {
				v = prof.peak
			}
			var vNext float64
			switch {
			case wd.Traveled < prof.accelD:
				vNext = v + prof.peak*DT/warpAccelTime // linear 0 -> peak
				if vNext > prof.peak {
					vNext = prof.peak
				}
			case r <= prof.decelD:
				// Closed form of the linear-in-time ramp against the
				// remaining distance: decel always hands over from cruise
				// (r == decelD gives exactly peak) and ends at 0 exactly on
				// the landing point, even when the cruise segment crossed
				// the boundary between ticks.
				vNext = math.Sqrt(2 * prof.peak * r / warpDecelTime)
			default:
				vNext = prof.peak
			}
			// Trapezoid step: exact for the linear ramps, so the three
			// segments integrate to exactly wd.Dist. The clamp is the
			// float-error backstop — a step can never overshoot the point.
			step := (v + vNext) / 2 * DT
			if step > r {
				step = r
			}
			rNext := r - step
			wd.Traveled = wd.Dist - rNext
			wd.Speed = vNext

			// Display: place the ship on the eased grid position — accel is
			// a visible slide out, cruise a slide at profile speed, decel a
			// visible glide in (warpDisplayRemaining). The eased path
			// places ABSOLUTELY each tick: at warp scales (1e13 m hops)
			// incremental adds would accumulate enough rounding to stall
			// the final approach above the landing tolerance. Hops whose
			// segments both fit inside warpGridSpan keep the original
			// trajectory, bit for bit.
			if prof.accelD <= warpGridSpan && prof.decelD <= warpGridSpan {
				tr.Pos = tr.Pos.Add(dir.Scale(step))
			} else {
				tr.Pos = wd.To.Sub(dir.Scale(warpDisplayRemaining(wd.Dist, rNext, prof)))
			}
			tr.Facing = dir.Angle()
			m.Vel = dir.Scale(vNext)
		}
	}
}

// warpSpeedMS converts the hull's WarpSpeed (AU/s) to m/s: 1 AU = 1.496e11 m.
func (s *Sim) warpSpeedMS(e ecs.Entity, m *Motion) float64 {
	// Motion does not carry warp speed; read it from the hull definition.
	if ship, ok := s.Ships.Get(e); ok {
		if hull, ok := s.Content.Ship(ship.Hull); ok && hull.WarpSpeed > 0 {
			return hull.WarpSpeed * format.AU
		}
	}
	return format.AU // fallback: 1 AU/s
}
