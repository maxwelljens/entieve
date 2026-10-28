// Package format renders the display strings the game shows: money,
// distance, grouped numbers and percent. Stdlib only — sim, render and
// uikit/theme all read it, so it must never import any of them.
package format

import (
	"math"
	"strconv"
	"strings"
)

// AU is one astronomical unit in metres (house convention).
const AU = 1.496e11

// Credits renders money per docs/lore/NAMING.md: thousands separators, two
// decimals and the € suffix — 15000 -> "15,000.00 €"; any amount under one
// euro renders as bare cents — 0.5 -> "50¢"; zero stays "0.00 €"; the minus
// sign stays out front.
func Credits(v float64) string {
	if v == 0 { // -0.0 must not print as "-0.00 €" (zero reads "0.00 €")
		return "0.00 €"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	s := strconv.FormatFloat(v, 'f', 2, 64)
	i := strings.IndexByte(s, '.')
	if i < 0 { // NaN / Inf: nothing to split into euros and cents
		return s
	}
	intPart, frac := s[:i], s[i:]
	if intPart == "0" && frac != ".00" { // under one euro: bare cents
		cents := strings.TrimLeft(frac[1:], "0")
		if neg {
			return "-" + cents + "¢"
		}
		return cents + "¢"
	}
	s = addCommas(intPart) + frac + " €"
	if neg {
		return "-" + s
	}
	return s
}

// Dist renders a distance in metres EVE-style: "42 m" / "1,250 m" under
// 2 km, "12.3 km" through 99,999 km, "<0.01 AU" below 0.01 AU, two-decimal
// AU beyond.
func Dist(m float64) string {
	if m < 0 {
		m = 0
	}
	switch {
	case m < 2e3:
		return GroupInt(int64(math.Round(m))) + " m"
	case m <= 9.9999e7: // 99,999 km: the km band ends here
		return GroupFloat(m/1000, 1) + " km"
	case m < AU/100: // 0.01 AU: below this an AU figure reads as zero
		return "<0.01 AU"
	default:
		return GroupFloat(m/AU, 2) + " AU"
	}
}

// GroupInt formats n with thousands separators: 12345 -> "12,345".
func GroupInt(n int64) string {
	return addCommas(strconv.FormatInt(n, 10))
}

// GroupFloat formats v with prec decimals and thousands separators.
func GroupFloat(v float64, prec int) string {
	return addCommas(strconv.FormatFloat(v, 'f', prec, 64))
}

// Pct renders a fraction 0..1 as a rounded percentage: 0.45 -> "45%".
func Pct(f float64) string {
	return strconv.FormatInt(int64(math.Round(f*100)), 10) + "%"
}

// addCommas inserts thousands separators into a decimal string's integer
// part ("12345.6", "-12345").
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
