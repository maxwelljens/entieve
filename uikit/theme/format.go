package theme

import (
	"math"
	"strconv"
	"strings"

	"github.com/entieve/entieve/format"
)

// Value formatting — the display formatters every surface reads. The number,
// money, distance and percent rules live once in the format leaf; these
// exported names are the house names the UI already calls (the golden pins in
// theme_test.go still hold).

// FormatDistance renders a distance in meters EVE-style: "42 m" /
// "1,250 m" (under 2 km), "12.3 km" up to 99,999 km, "<0.01 AU" below
// 0.01 AU, "0.45 AU" beyond (two decimals). 1 AU = format.AU (house
// convention).
func FormatDistance(m float64) string { return format.Dist(m) }

// FormatSpeed renders a speed in whole m/s with separators.
func FormatSpeed(v float64) string {
	if v < 0 {
		v = 0
	}
	return FormatInt(int64(math.Round(v))) + " m/s"
}

// FormatCredits renders an amount of money per docs/lore/NAMING.md: the
// euro with thousands separators and two decimals ("12,345.67 €"), bare
// cents under one euro ("50¢"), negatives as "-… €". Zero reads "0.00 €".
func FormatCredits(v float64) string { return format.Credits(v) }

// FormatVolume renders a volume in m³ at full precision with no trailing
// zeroes: "0.005 m³", "123.4 m³", "400 m³" — info surfaces where perfect
// precision is required (small ammo volumes must not round away to 0.0).
func FormatVolume(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64) + " m³"
}

// FormatVolumeShort renders a volume for lists and aggregates: float noise
// snapped away, floored to two decimals, trailing zeroes trimmed —
// 0.7000000000000001 m³ reads "0.7 m³", a genuine 0.669 m³ reads
// "0.66 m³" (round down). The 1e-6 epsilon is far below any genuine
// third decimal (volumes move in 0.005 steps) but above accumulated
// float error at cargo scales, so 350.29999999999995 → "350.3 m³".
func FormatVolumeShort(v float64) string {
	floor := math.Floor(v*100+1e-6) / 100
	return strconv.FormatFloat(floor, 'f', -1, 64) + " m³"
}

// FormatPct renders a fraction 0..1 as "87%". It is the pre-consolidation
// name; PctLabel is the same function under the name most call sites use —
// fold this alias into PctLabel when the panels that call it next change.
func FormatPct(f float64) string { return PctLabel(f) }

// kindLabel title-cases an Info.Kind for display ("asteroid" → "Asteroid").
func KindLabel(kind string) string {
	r := []rune(kind)
	if len(r) == 0 {
		return ""
	}
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// FormatInt formats n with thousands separators: 12345 → "12,345".
func FormatInt(n int64) string { return format.GroupInt(n) }

// groupFloat formats v with prec decimals and separators.
func GroupFloat(v float64, prec int) string { return format.GroupFloat(v, prec) }

// PctLabel renders a fraction as a rounded percentage ("45%").
func PctLabel(f float64) string { return format.Pct(f) }
