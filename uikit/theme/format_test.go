package theme

import "testing"

// TestFormatVolumeKeepsSmallVolumes pins the formatting rule: full
// precision, trailing zeroes removed, never rounded to one decimal
// (a 0.005 m³ autocannon round must not read as "0.0 m³").
func TestFormatVolumeKeepsSmallVolumes(t *testing.T) {
	cases := map[float64]string{
		0.005: "0.005 m³",
		0.008: "0.008 m³",
		0.01:  "0.01 m³",
		0.1:   "0.1 m³",
		1.5:   "1.5 m³",
		0.104: "0.104 m³",
		123.4: "123.4 m³",
		200:   "200 m³",
		400:   "400 m³",
		16500: "16500 m³",
		0:     "0 m³",
	}
	for v, want := range cases {
		if got := FormatVolume(v); got != want {
			t.Errorf("FormatVolume(%v) = %q, want %q", v, got, want)
		}
	}
}
