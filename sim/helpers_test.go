package sim

import "testing"

// TestKmLabelBands pins the shared distance label's unit switch with the
// golden table the panels read (sim.KmLabel, exported so screens quote the
// refusal log line verbatim). The money format's one golden table lives in
// the format package.
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
		if got := KmLabel(c.in); got != c.want {
			t.Errorf("KmLabel(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
