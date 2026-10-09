package sim

import (
	"math"
	"testing"
)

// ---------------------------------------------------------------------------
// CmdWarpTo.Range: warp-to-range lands Range metres short of the target
// along the warp vector; 0 lands on the target. The 2500 m landing
// tolerance applies around the landing point.
// ---------------------------------------------------------------------------

// warpUntilLanded queues the command, steps until the warp completes (or
// the budget runs out) and reports whether it landed.
func warpUntilLanded(s *Sim, cmd CmdWarpTo, budget int) bool {
	s.Queue(cmd)
	for i := 0; i < budget; i++ {
		s.Step()
		if hasEvent(drain(s), EvWarp, "Warp complete") {
			return true
		}
	}
	return false
}

func TestCmdWarpToRangeLandsShortOfTarget(t *testing.T) {
	s := newTestSim(t)
	station := entityByName(s, "Haven Rest")
	setPlayerPos(s, v(3e7, 3e7)) // far corner

	const wantRange = 10000.0
	if !warpUntilLanded(s, CmdWarpTo{Target: station, Range: wantRange}, 1800) {
		t.Fatal("warp never landed")
	}
	if s.Warps.Has(s.player) {
		t.Fatal("warp drive still engaged after landing")
	}
	d := s.distBetween(s.player, station)
	if math.Abs(d-wantRange) > 1 {
		t.Fatalf("landed %v m from target, want ≈ %v (Range)", d, wantRange)
	}
}

func TestCmdWarpToRangeZeroLandsOnTarget(t *testing.T) {
	s := newTestSim(t)
	station := entityByName(s, "Haven Rest")
	setPlayerPos(s, v(3e7, 3e7))

	if !warpUntilLanded(s, CmdWarpTo{Target: station, Range: 0}, 1800) {
		t.Fatal("warp never landed")
	}
	// Range 0 = on top of the target: the drive snaps exactly onto it
	// (well inside the 2500 m landing tolerance around the landing point).
	d := s.distBetween(s.player, station)
	if d > 1 {
		t.Fatalf("landed %v m from target, want ~0 for Range 0", d)
	}
}

func TestCmdWarpToRangeClampedToNonNegative(t *testing.T) {
	s := newTestSim(t)
	station := entityByName(s, "Haven Rest")
	setPlayerPos(s, v(3e7, 3e7))

	// A negative range behaves like 0 (clamped), never "behind" the target.
	if !warpUntilLanded(s, CmdWarpTo{Target: station, Range: -5000}, 1800) {
		t.Fatal("warp never landed")
	}
	if d := s.distBetween(s.player, station); d > 1 {
		t.Fatalf("landed %v m from target, want ~0 for clamped negative Range", d)
	}
}

func TestCmdWarpToRangeStillRejectsNonCelestials(t *testing.T) {
	s := newTestSim(t)
	// A non-celestial target is still rejected regardless of Range.
	rover := s.create()
	s.Names.Set(rover, Name{Value: "Rover"})
	s.Transforms.Set(rover, Transform{Pos: v(1e7, 600)})
	s.Queue(CmdWarpTo{Target: rover, Range: 5000})
	s.Step()
	evs := drain(s)
	if s.Warps.Has(s.player) {
		t.Fatal("warped to a non-celestial with Range set")
	}
	if findEvent(evs, EvLog, "Cannot warp there") == nil {
		t.Fatalf("expected warning: %v", evs)
	}
}
