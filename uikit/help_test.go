package uikit

import (
	"strconv"
	"strings"
	"testing"
)

// TestHelpConstruction renders a small key map: the window builds, Sync is
// a no-op, and the sensor blurb carries the documented numbers.
func TestHelpConstruction(t *testing.T) {
	env, _ := newChromeEnv(t)
	h := NewHelp(env, []KeyRow{
		{"Tab", "cycle-lock nearest target"},
		{"Shift+Tab", "lock selected target"},
		{"F1", "toggle high slot 1"},
		{"Esc", "menu (Resume / Save / Load / Quit)"},
	})
	if h.Win() == nil {
		t.Fatal("HelpPanel.Win() is nil")
	}
	h.Sync()
	for _, want := range []string{
		"250 km sensor radius",
		"3 × sensor range",
		"no names, no distances",
		"Warp-to-within",
	} {
		if !strings.Contains(h.blurb.Label, want) {
			t.Errorf("sensor blurb missing %q:\n%s", want, h.blurb.Label)
		}
	}

	// An empty key map still builds (the table row is skipped).
	empty := NewHelp(env, nil)
	empty.Sync()
	if empty.Win() == nil {
		t.Fatal("empty HelpPanel.Win() is nil")
	}
}

// keyMapRows mimics the shape of game.KeyMap (long descriptions that would
// wrap) so the window height is exercised at its realistic worst case.
func keyMapRows(n int) []KeyRow {
	rows := make([]KeyRow, 0, n)
	for i := 0; i < n; i++ {
		rows = append(rows, KeyRow{
			Key:  "Key" + strconv.Itoa(i),
			Desc: "station services / market tab (while docked)",
		})
	}
	return rows
}

// TestHelpDrawSmoke draws a full-size key map at both design sizes; the
// shared smoke asserts every window stays inside the screen.
func TestHelpDrawSmoke(t *testing.T) {
	env, _ := newChromeEnv(t)
	h := NewHelp(env, keyMapRows(37))
	c := newChromeUI(env)
	c.addWindow(h)
	c.smoke(t)
}
