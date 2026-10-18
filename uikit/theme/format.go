package theme

import (
	"math"
	"strconv"
	"strings"
)

// Value formatting — the display formatters every surface reads
// (ported from the legacy ui package; the golden pins in theme_test.go).
// ---------------------------------------------------------------------------
// Value formatting — local copies of the legacy ui formatters (uikit must
// not import ui outside tests; TestFormattersMatchLegacy pins behavior).
// ---------------------------------------------------------------------------

// formatDistance renders a distance in meters EVE-style: "42 m" /
// "1,250 m" (under 2 km), "12.3 km" up to 99,999 km, "<0.01 AU" below
// 0.01 AU, "0.45 AU" beyond (two decimals). 1 AU = 1.496e11 m here (house
// convention).
func FormatDistance(m float64) string {
	if m < 0 {
		m = 0
	}
	switch {
	case m < 2e3:
		return FormatInt(int64(math.Round(m))) + " m"
	case m <= 9.9999e7: // 99,999 km: the km band ends here
		return GroupFloat(m/1000, 1) + " km"
	case m < 1.496e9: // 0.01 AU: below this an AU figure reads as zero
		return "<0.01 AU"
	default:
		return GroupFloat(m/1.496e11, 2) + " AU"
	}
}

// formatSpeed renders a speed in whole m/s with separators.
func FormatSpeed(v float64) string {
	if v < 0 {
		v = 0
	}
	return FormatInt(int64(math.Round(v))) + " m/s"
}

// FormatCredits renders an amount of money per docs/lore/NAMING.md: the
// euro with thousands separators and two decimals ("12,345.67 €"), bare
// cents under one euro ("50¢"), negatives as "-… €". Zero reads "0.00 €".
func FormatCredits(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	v = math.Round(v*100) / 100
	ip := int64(v)
	frac := int64(math.Round((v - float64(ip)) * 100))
	if frac >= 100 { // carry from rounding, e.g. 0.999 -> 1.00
		ip++
		frac = 0
	}
	var s string
	if ip == 0 && frac != 0 { // under one euro: bare cents, no decimals
		s = strconv.FormatInt(frac, 10) + "¢"
	} else {
		s = FormatInt(ip) + "." + pad2(frac) + " €"
	}
	if neg {
		s = "-" + s
	}
	return s
}

// formatVolume renders a volume in m³ with one decimal: "123.4 m³".
func FormatVolume(v float64) string {
	return strconv.FormatFloat(v, 'f', 1, 64) + " m³"
}

// formatPct renders a fraction 0..1 as "87%".
func FormatPct(f float64) string {
	return strconv.FormatInt(int64(math.Round(f*100)), 10) + "%"
}

// kindLabel title-cases an Info.Kind for display ("asteroid" → "Asteroid").
func KindLabel(kind string) string {
	r := []rune(kind)
	if len(r) == 0 {
		return ""
	}
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// FormatInt formats n with thousands separators: 12345 → "12,345".
func FormatInt(n int64) string {
	return addCommas(strconv.FormatInt(n, 10))
}

// groupFloat formats v with prec decimals and separators.
func GroupFloat(v float64, prec int) string {
	return addCommas(strconv.FormatFloat(v, 'f', prec, 64))
}

// addCommas inserts thousands separators into a decimal string's integer
// part (copied from ui/format.go).
func addCommas(s string) string {
	head, tail, found := strings.Cut(s, ".")
	neg := strings.HasPrefix(head, "-")
	if neg {
		head = head[1:]
	}
	var b strings.Builder
	b.Grow(len(head) + len(head)/3)
	for i, c := range head {
		if i > 0 && (len(head)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	out := b.String()
	if neg {
		out = "-" + out
	}
	if found {
		out += "." + tail
	}
	return out
}

// pad2 renders a 0..99 integer with a leading zero.
func pad2(n int64) string {
	if n < 10 {
		return "0" + strconv.FormatInt(n, 10)
	}
	return strconv.FormatInt(n, 10)
}

// pctLabel renders a fraction as a rounded percentage ("45%").
func PctLabel(f float64) string {
	return strconv.FormatInt(int64(math.Round(f*100)), 10) + "%"
}
