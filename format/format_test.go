package format

import (
	"math"
	"testing"
)

// TestCreditsGolden is the one money table: the union of the sim (euro) and
// HUD (theme.FormatCredits) goldens. docs/lore/NAMING.md "Money strings".
func TestCreditsGolden(t *testing.T) {
	for _, c := range []struct {
		in   float64
		want string
	}{
		{15000, "15,000.00 €"},
		{40000, "40,000.00 €"},
		{12345.67, "12,345.67 €"},
		{1, "1.00 €"},
		{0.5, "50¢"},
		{0.99, "99¢"},
		{0.01, "1¢"},
		{0.999, "1.00 €"}, // rounds to a full euro before the cents rule
		{0, "0.00 €"},
		{math.Copysign(0, -1), "0.00 €"}, // -0.0 is zero: no minus sign
		{math.NaN(), "NaN"},              // no decimal point to split: no panic
		{-1234.5, "-1,234.50 €"},
		{-0.5, "-50¢"},
	} {
		if got := Credits(c.in); got != c.want {
			t.Errorf("Credits(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestDistGolden is the one distance table: the union of theme.FormatDistance
// and render's old fmtDist bands.
func TestDistGolden(t *testing.T) {
	for m, want := range map[float64]string{
		-5: "0 m", 0: "0 m", 42: "42 m", 999: "999 m", 1999: "1,999 m",
		2000: "2.0 km", 12345: "12.3 km", 9.9999e7: "99,999.0 km",
		1e8: "<0.01 AU", 1.4e9: "<0.01 AU", 1.495999e9: "<0.01 AU",
		1.496e9: "0.01 AU", 1.496e10: "0.10 AU", 6.7e10: "0.45 AU",
		3e11: "2.01 AU", 1.496e13: "100.00 AU",
	} {
		if got := Dist(m); got != want {
			t.Errorf("Dist(%v) = %q, want %q", m, got, want)
		}
	}
}

// TestGroupGolden pins the thousands grouping both number forms share.
func TestGroupGolden(t *testing.T) {
	for n, want := range map[int64]string{
		0: "0", 999: "999", 1000: "1,000", 12345: "12,345",
		1234567: "1,234,567", -1234: "-1,234",
	} {
		if got := GroupInt(n); got != want {
			t.Errorf("GroupInt(%d) = %q, want %q", n, got, want)
		}
	}
	for _, c := range []struct {
		v    float64
		prec int
		want string
	}{
		{1234.5, 1, "1,234.5"},
		{-1234.5, 2, "-1,234.50"},
		{99.9999e3, 1, "99,999.9"},
		{2, 2, "2.00"},
	} {
		if got := GroupFloat(c.v, c.prec); got != c.want {
			t.Errorf("GroupFloat(%v, %d) = %q, want %q", c.v, c.prec, got, c.want)
		}
	}
}

// TestPctGolden pins the rounded-percentage form.
func TestPctGolden(t *testing.T) {
	for f, want := range map[float64]string{
		0: "0%", 0.004: "0%", 0.45: "45%", 0.5: "50%", 0.876: "88%", 1: "100%",
	} {
		if got := Pct(f); got != want {
			t.Errorf("Pct(%v) = %q, want %q", f, got, want)
		}
	}
}

// TestAU pins the one AU constant the call sites convert against.
func TestAU(t *testing.T) {
	if AU != 1.496e11 {
		t.Fatalf("AU = %v, want 1.496e11 m", AU)
	}
}
