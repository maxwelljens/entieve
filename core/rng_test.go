package core

import "testing"

// Golden values pin the exact SplitMix64 algorithm: if these ever fail, the
// stream changed and simulation determinism/replay is broken.
func TestRNGGoldenValues(t *testing.T) {
	tests := []struct {
		name string
		seed int64
		want []uint64
	}{
		{
			name: "seed 0",
			seed: 0,
			want: []uint64{
				16294208416658607535,
				12035550249420947055,
				2558736989570252433,
				2391539541053276776,
				8695987549771912286,
			},
		},
		{
			name: "seed 42",
			seed: 42,
			want: []uint64{
				13679457532755275413,
				6332618229526065668,
				7138415436909018950,
				13934464819154148243,
				14682575489612188334,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRNG(tc.seed)
			for i, want := range tc.want {
				if got := r.Uint64(); got != want {
					t.Fatalf("Uint64() #%d = %d, want %d", i, got, want)
				}
			}
		})
	}
}

func draw(r *RNG, n int) []uint64 {
	out := make([]uint64, n)
	for i := range out {
		out[i] = r.Uint64()
	}
	return out
}

func TestRNGSameSeedSameStream(t *testing.T) {
	a := draw(NewRNG(1234), 100)
	b := draw(NewRNG(1234), 100)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("streams diverge at #%d: %d != %d", i, a[i], b[i])
		}
	}
}

func TestRNGDifferentSeedsDiffer(t *testing.T) {
	a := draw(NewRNG(1), 10)
	b := draw(NewRNG(2), 10)
	same := true
	for i := range a {
		if a[i] != b[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("different seeds produced identical streams")
	}
}

func TestRNGIntn(t *testing.T) {
	for _, n := range []int{1, 2, 7, 10, 1000} {
		r := NewRNG(int64(n))
		for i := 0; i < 2000; i++ {
			v := r.Intn(n)
			if v < 0 || v >= n {
				t.Fatalf("Intn(%d) = %d, out of range", n, v)
			}
		}
	}
}

func TestRNGIntnNonPositive(t *testing.T) {
	r := NewRNG(99)
	for _, n := range []int{0, -1, -100} {
		for i := 0; i < 100; i++ {
			if got := r.Intn(n); got != 0 {
				t.Fatalf("Intn(%d) = %d, want 0", n, got)
			}
		}
	}
}

func TestRNGFloat64Range(t *testing.T) {
	r := NewRNG(7)
	for i := 0; i < 10000; i++ {
		v := r.Float64()
		if v < 0 || v >= 1 {
			t.Fatalf("Float64() = %v, want [0,1)", v)
		}
	}
}

func TestRNGRange(t *testing.T) {
	r := NewRNG(11)
	min, max := -3.5, 2.25
	sawMinSide, sawMaxSide := false, false
	for i := 0; i < 10000; i++ {
		v := r.Range(min, max)
		if v < min || v >= max {
			t.Fatalf("Range(%v, %v) = %v, out of range", min, max, v)
		}
		if v < (min+max)/2 {
			sawMinSide = true
		} else {
			sawMaxSide = true
		}
	}
	if !sawMinSide || !sawMaxSide {
		t.Fatal("Range never covered one half of its interval")
	}
}

func TestRNGChanceExtremes(t *testing.T) {
	r := NewRNG(5)
	for _, p := range []float64{0, -1, -0.5} {
		for i := 0; i < 1000; i++ {
			if r.Chance(p) {
				t.Fatalf("Chance(%v) = true, want false", p)
			}
		}
	}
	for _, p := range []float64{1, 2} {
		for i := 0; i < 1000; i++ {
			if !r.Chance(p) {
				t.Fatalf("Chance(%v) = false, want true", p)
			}
		}
	}
}

func TestRNGChanceProbability(t *testing.T) {
	// Deterministic seed, so this is not flaky: with p=0.5 both outcomes
	// must occur over 1000 draws.
	r := NewRNG(2024)
	var hits int
	for i := 0; i < 1000; i++ {
		if r.Chance(0.5) {
			hits++
		}
	}
	if hits == 0 || hits == 1000 {
		t.Fatalf("Chance(0.5) hit %d/1000 times, want a mix", hits)
	}
}

func TestRNGNormFloat64(t *testing.T) {
	r := NewRNG(3)
	const n = 10000
	var sum, sumSq float64
	for i := 0; i < n; i++ {
		v := r.NormFloat64()
		if v != v { // NaN check
			t.Fatal("NormFloat64 returned NaN")
		}
		if v > 6 || v < -6 {
			t.Fatalf("NormFloat64 = %v, implausibly far from mean", v)
		}
		sum += v
		sumSq += v * v
	}
	mean := sum / n
	variance := sumSq/n - mean*mean
	if mean < -0.15 || mean > 0.15 {
		t.Errorf("mean = %v, want near 0", mean)
	}
	if variance < 0.8 || variance > 1.2 {
		t.Errorf("variance = %v, want near 1", variance)
	}
}

func TestRNGForkDeterminism(t *testing.T) {
	f1 := NewRNG(77).Fork(1)
	f2 := NewRNG(77).Fork(1)
	a, b := draw(f1, 20), draw(f2, 20)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("same fork diverges at #%d", i)
		}
	}

	g1 := draw(NewRNG(77).Fork(1), 20)
	g2 := draw(NewRNG(77).Fork(2), 20)
	same := true
	for i := range g1 {
		if g1[i] != g2[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("different fork streams produced identical output")
	}
}

func TestRNGSnapshotRestore(t *testing.T) {
	r := NewRNG(123)
	draw(r, 5)
	state := r.Snapshot()
	want := draw(r, 10)

	restored := RestoreRNG(state)
	got := draw(restored, 10)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("restored stream diverges at #%d: %d != %d", i, got[i], want[i])
		}
	}
}
