package game

import (
	"testing"

	"github.com/entieve/entieve/sim"
)

// TestClockStepsPerSecondAtMultiplier: one real second produces mult×60
// fixed sim Steps at each time acceleration.
func TestClockStepsPerSecondAtMultiplier(t *testing.T) {
	for _, mult := range TimeMultipliers {
		var c Clock
		if m := c.Mult(); m != 1 {
			t.Fatalf("initial multiplier = %d, want 1", m)
		}
		for c.multIdx = 0; TimeMultipliers[c.multIdx] != mult; c.multIdx++ {
		}
		total := 0
		for i := 0; i < 60; i++ {
			total += c.Advance(1.0 / 60.0)
		}
		if want := 60 * mult; total != want {
			t.Errorf("mult %d: 1s real produced %d steps, want %d", mult, total, want)
		}
	}
}

// TestClockCarryFractionalTime: sub-tick remainders carry across frames.
func TestClockCarryFractionalTime(t *testing.T) {
	var c Clock // 1x
	// Three frames of DT/2 real seconds each: 1.5 ticks total → one Step
	// now, half a tick carried over (all values are exact binary halves).
	dt := sim.DT / 2
	total := 0
	for i := 0; i < 3; i++ {
		total += c.Advance(dt)
	}
	if total != 1 {
		t.Fatalf("fractional carry produced %d steps, want 1", total)
	}
	if c.accum <= 0 || c.accum >= sim.DT {
		t.Fatalf("carry %v outside (0, DT)", c.accum)
	}
}

// TestClockBumpCycles: [ and ] walk 1 → 2 → 4 → 8 and back, clamped.
func TestClockBumpCycles(t *testing.T) {
	var c Clock
	got := []int{c.Bump(true), c.Bump(true), c.Bump(true), c.Bump(true)}
	want := []int{2, 4, 8, 8}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Bump up = %v, want %v", got, want)
		}
	}
	got = []int{c.Bump(false), c.Bump(false), c.Bump(false), c.Bump(false)}
	want = []int{4, 2, 1, 1}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Bump down = %v, want %v", got, want)
		}
	}
}

// TestClockCapsStalledFrames: a long stall cannot queue unbounded Steps.
func TestClockCapsStalledFrames(t *testing.T) {
	for _, mult := range TimeMultipliers {
		var c Clock
		for c.multIdx = 0; TimeMultipliers[c.multIdx] != mult; c.multIdx++ {
		}
		// 10 s of real time in one frame, at every multiplier: the cap is
		// maxAccumSeconds of scaled time.
		want := int(maxAccumSeconds / sim.DT)
		if got := c.Advance(10); got != want {
			t.Errorf("mult %d: stalled frame produced %d steps, want %d", mult, got, want)
		}
		// After the capped frame, residual time is < one tick.
		if got := c.Advance(0); got != 0 {
			t.Errorf("mult %d: residual steps after cap = %d", mult, got)
		}
	}
}

// TestClockNegativeTimeIgnored: negative deltas are clamped.
func TestClockNegativeTimeIgnored(t *testing.T) {
	var c Clock
	if got := c.Advance(-5); got != 0 {
		t.Fatalf("negative advance produced %d steps", got)
	}
}
