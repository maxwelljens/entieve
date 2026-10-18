package sim

import "testing"

// TestEuroFormat pins the money-string convention of docs/lore/NAMING.md:
// € suffix after the amount with thousands separators and two decimals, bare
// ¢ for amounts under one euro, the minus sign out front, and zero staying
// "0.00 €".
func TestEuroFormat(t *testing.T) {
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
		{-1234.5, "-1,234.50 €"},
		{-0.5, "-50¢"},
	} {
		if got := euro(c.in); got != c.want {
			t.Errorf("euro(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestKmLabelBands pins the refusal-line kmLabel unit switch with the same
// golden table as uikit/screens.TestKmLabelBands — sim.kmLabel is
// unexported and unreachable from that package, so the two tables must stay
// identical to keep the panel hint and the refusal log line wording alike.
func TestKmLabelBands(t *testing.T) {
	for _, c := range []struct {
		in   float64
		want string
	}{
		{WarpMinRange, "150 km"},
		{9.9999e7, "99999 km"},
		{1e8, "<0.01 AU"},
		{1.4e9, "<0.01 AU"},
		{1.496e9, "0.01 AU"},
		{3e11, "2.01 AU"},
	} {
		if got := kmLabel(c.in); got != c.want {
			t.Errorf("kmLabel(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
