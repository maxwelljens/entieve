package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// FormatDistance renders a distance in meters EVE-style:
// "42 m" / "1,250 m" (under 2 km), "3.4 km" (under 0.1 AU), "1.2 AU" beyond.
func FormatDistance(m float64) string {
	if m < 0 {
		m = 0
	}
	switch {
	case m < 2e3:
		return groupInt(int64(math.Round(m))) + " m"
	case m < 1.496e10: // 0.1 AU: EVE switches to AU here
		return groupFloat(m/1000, 1) + " km"
	default:
		return groupFloat(m/1.496e11, 1) + " AU"
	}
}

// FormatCredits renders an amount of the currency as "12,345.67 cr".
// The currency of Entieve is the credit (cr).
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
	s := groupInt(ip) + "." + pad2(frac) + " cr"
	if neg {
		s = "-" + s
	}
	return s
}

// FormatSpeed renders a speed in whole m/s with separators: "1,234 m/s".
func FormatSpeed(v float64) string {
	if v < 0 {
		v = 0
	}
	return groupInt(int64(math.Round(v))) + " m/s"
}

// FormatVolume renders a volume in m³ with one decimal: "123.4 m³".
func FormatVolume(v float64) string {
	return fmt.Sprintf("%.1f m³", v)
}

// FormatTime renders sim seconds as m:ss for log lines.
func FormatTime(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	s := int64(sec)
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// FormatPct renders a fraction 0..1 as "87%".
func FormatPct(f float64) string {
	return fmt.Sprintf("%d%%", int64(math.Round(f*100)))
}

// groupInt formats n with thousands separators: 12345 -> "12,345".
func groupInt(n int64) string {
	return addCommas(strconv.FormatInt(n, 10))
}

// groupFloat formats v with prec decimals and separators: 1234.567,1 -> "1,234.6".
func groupFloat(v float64, prec int) string {
	return addCommas(strconv.FormatFloat(v, 'f', prec, 64))
}

// addCommas inserts thousands separators into a decimal string's integer part.
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
