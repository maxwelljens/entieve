package screens

import "testing"

// TestStarmapTitleCollapsesOneName pins the window title composition:
// region and constellation usually read as one name (the Wild Fields
// serves both levels — RED: the old composition rendered "the Wild
// Fields / the Wild Fields"), differing names show both, and an empty
// half never renders as a dangling separator.
func TestStarmapTitleCollapsesOneName(t *testing.T) {
	cases := []struct{ region, constellation, want string }{
		{"the Wild Fields", "the Wild Fields", "Starmap — the Wild Fields"},
		{"Far Moravia", "Kresy", "Starmap — Far Moravia / Kresy"},
		{"", "Kresy", "Starmap — Kresy"},
		{"Far Moravia", "", "Starmap — Far Moravia"},
		{"", "", "Starmap"},
	}
	for _, c := range cases {
		if got := starmapTitle(c.region, c.constellation); got != c.want {
			t.Errorf("starmapTitle(%q, %q) = %q, want %q", c.region, c.constellation, got, c.want)
		}
	}
}
