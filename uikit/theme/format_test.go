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

// TestFormatVolumeShortFloorsAndTrims pins the list/aggregate rule: float
// noise snapped away, floored to two decimals, trailing zeroes trimmed —
// a genuine third decimal floors honestly (0.669 → 0.66).
func TestFormatVolumeShortFloorsAndTrims(t *testing.T) {
	cases := map[float64]string{
		0.7000000000000001: "0.7 m³", // the reported cargo-sum artifact
		0.6999999999999999: "0.7 m³", // downward noise, epsilon-snapped
		350.29999999999995: "350.3 m³",
		45.999999999999993: "46 m³",
		0.005:              "0 m³", // below two decimals: floored
		0.06:               "0.06 m³",
		0.104:              "0.1 m³",
		0.669:              "0.66 m³", // genuine round-down
		100:                "100 m³",
		16500:              "16500 m³",
		0:                  "0 m³",
	}
	for v, want := range cases {
		if got := FormatVolumeShort(v); got != want {
			t.Errorf("FormatVolumeShort(%v) = %q, want %q", v, got, want)
		}
	}
}
