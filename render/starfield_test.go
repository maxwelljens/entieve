package render

import "testing"

// TestStarfieldDeterministic: same seed yields identical starfields.
func TestStarfieldDeterministic(t *testing.T) {
	a := Starfield(42, 64)
	b := Starfield(42, 64)
	if len(a) != 64 || len(b) != 64 {
		t.Fatalf("expected 64 stars, got %d and %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("star %d differs under same seed: %v vs %v", i, a[i], b[i])
		}
	}
}

// TestStarfieldDifferentSeedsDiffer: different seeds produce different fields.
func TestStarfieldDifferentSeedsDiffer(t *testing.T) {
	a := Starfield(1, 32)
	b := Starfield(2, 32)
	same := true
	for i := range a {
		if a[i] != b[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("different seeds produced identical starfields")
	}
}

// TestStarfieldUnitCell: all coordinates lie in [0,1), brightness in (0,1],
// size positive.
func TestStarfieldUnitCell(t *testing.T) {
	for _, s := range Starfield(0xABCDEF, 500) {
		if s.X < 0 || s.X >= 1 || s.Y < 0 || s.Y >= 1 {
			t.Fatalf("coordinates out of unit cell: %v", s)
		}
		if s.Bright <= 0 || s.Bright > 1 {
			t.Fatalf("brightness out of range: %v", s.Bright)
		}
		if s.Size <= 0 {
			t.Fatalf("non-positive size: %v", s.Size)
		}
	}
}

// TestStarfieldEdgeCases: zero and negative counts yield empty slices.
func TestStarfieldEdgeCases(t *testing.T) {
	if got := Starfield(7, 0); len(got) != 0 {
		t.Fatalf("count 0: got %d stars", len(got))
	}
	if got := Starfield(7, -5); len(got) != 0 {
		t.Fatalf("negative count: got %d stars", len(got))
	}
}
