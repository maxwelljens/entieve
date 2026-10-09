package ui

import "testing"

// TestFormatDistance pins the EVE-style distance formatting rules.
func TestFormatDistance(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0 m"},
		{42, "42 m"},
		{999.4, "999 m"},
		{1000, "1,000 m"},
		{1250, "1,250 m"},
		{1999, "1,999 m"},
		{2000, "2.0 km"},
		{3400, "3.4 km"},
		{149600, "149.6 km"},
		{1e6, "1,000.0 km"},
		{1e7, "10,000.0 km"},
		{1.496e10, "0.1 AU"}, // exactly 0.1 AU: boundary switches to AU
		{1.7952e11, "1.2 AU"},
		{-5, "0 m"}, // negatives clamp
	}
	for _, c := range cases {
		if got := FormatDistance(c.in); got != c.want {
			t.Errorf("FormatDistance(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestFormatCredits pins the credit formatting ("12,345.67 cr").
func TestFormatCredits(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0.00 cr"},
		{1, "1.00 cr"},
		{12345.67, "12,345.67 cr"},
		{1000000, "1,000,000.00 cr"},
		{999.999, "1,000.00 cr"}, // rounding carry
		{-1234.5, "-1,234.50 cr"},
		{0.005, "0.01 cr"},
		{0.994, "0.99 cr"},
	}
	for _, c := range cases {
		if got := FormatCredits(c.in); got != c.want {
			t.Errorf("FormatCredits(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestFormatSpeedTimePct covers the remaining display helpers.
func TestFormatSpeedTimePct(t *testing.T) {
	if got := FormatSpeed(1234.6); got != "1,235 m/s" {
		t.Errorf("FormatSpeed = %q", got)
	}
	if got := FormatSpeed(-3); got != "0 m/s" {
		t.Errorf("FormatSpeed(neg) = %q", got)
	}
	if got := FormatTime(65); got != "1:05" {
		t.Errorf("FormatTime(65) = %q", got)
	}
	if got := FormatTime(0); got != "0:00" {
		t.Errorf("FormatTime(0) = %q", got)
	}
	if got := FormatPct(0.456); got != "46%" {
		t.Errorf("FormatPct = %q", got)
	}
	if got := FormatVolume(12.34); got != "12.3 m³" {
		t.Errorf("FormatVolume = %q", got)
	}
}
