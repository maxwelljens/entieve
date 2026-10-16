package widgets

import (
	"math"
	"testing"
)

// col builds a ColWidth for tests.
func col(label string, width, min float64) ColWidth {
	return ColWidth{Label: label, Width: width, Min: min}
}

func TestFitColumns(t *testing.T) {
	// natural widths: Name 100, Distance 50, Kind 30, Velocity 40.
	natural := func() []ColWidth {
		return []ColWidth{
			col("Name", 100, 40),
			col("Distance", 50, 30),
			col("Kind", 30, 20),
			col("Velocity", 40, 25),
		}
	}
	sum := func(cols []ColWidth) float64 {
		s := 0.0
		for _, c := range cols {
			s += c.Width
		}
		return s
	}

	tests := []struct {
		name  string
		avail float64
		want  [4]float64 // fitted widths, indexed Name..Velocity
	}{
		{
			// Surplus all goes to Name; the fit is exact.
			name:  "surplus distributed to name",
			avail: 250,
			want:  [4]float64{130, 50, 30, 40},
		},
		{
			// Natural sum fits untouched (no column is grown).
			name:  "fits exactly no change",
			avail: 220,
			want:  [4]float64{100, 50, 30, 40},
		},
		{
			// Velocity gives way first, then Kind, then Distance.
			name:  "priority shrink",
			avail: 180,
			want:  [4]float64{100, 35, 20, 25},
		},
		{
			// Name only shrinks after every other column hit its floor.
			name:  "name shrinks last",
			avail: 150,
			want:  [4]float64{75, 30, 20, 25},
		},
		{
			// Name never drops below Min even when space remains short;
			// the residual overflow is left alone.
			name:  "name floor",
			avail: 100,
			want:  [4]float64{40, 30, 20, 25},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FitColumns(natural(), tt.avail)
			for i, w := range tt.want {
				if math.Abs(got[i].Width-w) > 1e-9 {
					t.Errorf("cols[%d] (%s) width = %v, want %v",
						i, got[i].Label, got[i].Width, w)
				}
			}
			if tt.avail >= 220 && math.Abs(sum(got)-tt.avail) > 1e-9 {
				t.Errorf("total = %v, want exactly %v", sum(got), tt.avail)
			}
		})
	}
}

func TestEllipsize(t *testing.T) {
	// Monotonic measure: every rune is 10 px wide.
	measure := func(s string) float64 {
		return float64(len([]rune(s))) * 10
	}
	tests := []struct {
		name string
		in   string
		w    float64
		want string
	}{
		{"fits untouched", "hello", 100, "hello"},
		{"fits exactly", "hello", 50, "hello"},
		{"one rune over", "hello", 40, "hel…"},
		{"tight", "hello", 30, "he…"},
		{"single rune", "a", 5, ""},
		{"empty width", "anything", 0, ""},
		{"multibyte", "αβγδε", 30, "αβ…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Ellipsize(tt.in, tt.w, measure)
			if got != tt.want {
				t.Errorf("Ellipsize(%q, %v) = %q, want %q", tt.in, tt.w, got, tt.want)
			}
			if w := measure(got); w > tt.w {
				t.Errorf("result %q measures %v > %v", got, w, tt.w)
			}
		})
	}
}
