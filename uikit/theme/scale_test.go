package theme

import (
	"math"
	"testing"
)

// useScale installs s for the test's duration and restores the identity.
func useScale(t *testing.T, s float64) {
	t.Helper()
	SetScale(s)
	t.Cleanup(func() { SetScale(1) })
}

// The scale funnel's contract and the regressions that ride on it:
// every metric goes through Px/Scale (identity at 1), the cursor-facing
// hit tests (grip bands, menu rows) measure in the same scaled units
// they lay out in, and the two stepper/table defects stay fixed at every
// scale. Every test restores scale 1 on cleanup — the baseline suite
// runs at scale 1, where design units are pixels.

// TestPxFunnel pins the funnel itself: identity at 1 (the baseline), the
// straight multiple at integers and halves, and the broken-value fallback.
func TestPxFunnel(t *testing.T) {
	useScale(t, 1)
	for _, n := range []int{0, 1, 5, 8, 44, 320, 1024} {
		if got := Px(n); got != n {
			t.Errorf("Px(%d) at scale 1 = %d, want %d (identity)", n, got, n)
		}
	}
	SetScale(2)
	if Px(10) != 20 || Px(11) != 22 || Px(320) != 640 {
		t.Errorf("Px at 2 = %d/%d/%d, want 20/22/640", Px(10), Px(11), Px(320))
	}
	SetScale(1.5)
	if Px(8) != 12 || Px(5) != 8 { // round-half-away: 7.5 → 8
		t.Errorf("Px at 1.5 = %d/%d, want 12/8", Px(8), Px(5))
	}
	if Scale() != 1.5 {
		t.Errorf("Scale() = %v, want 1.5", Scale())
	}
	// Broken settings must never collapse the interface to zero.
	SetScale(0)
	if Scale() != 1 || Px(9) != 9 {
		t.Errorf("SetScale(0): scale = %v Px(9) = %d, want 1/9", Scale(), Px(9))
	}
	SetScale(math.NaN())
	if Scale() != 1 {
		t.Errorf("SetScale(NaN) = %v, want 1", Scale())
	}
	SetScale(1)
}
