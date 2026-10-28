package game

import (
	"path/filepath"
	"testing"

	uikit "github.com/entieve/entieve/uikit/screens"
	"github.com/hajimehoshi/ebiten/v2"
)

// The key-hint module (game/keyhints.go): one formatter for every chord, and
// the four surfaces that print an action's key reading the live binding
// table instead of a chord baked into a declaration string.

// TestChordOf pins the one chord formatter against the strings the tables
// used to carry baked in — byte-identical defaults are what let the split
// land without wording changes ("Ctrl+S", "Shift+L", "Esc", "F1", "[").
func TestChordOf(t *testing.T) {
	for _, tc := range []struct {
		key         ebiten.Key
		shift, ctrl bool
		want        string
	}{
		{ebiten.KeyTab, false, false, "Tab"},
		{ebiten.KeyTab, true, false, "Shift+Tab"},
		{ebiten.KeyF1, false, false, "F1"},
		{ebiten.KeyF9, false, false, "F9"},
		{ebiten.KeyI, false, false, "I"},
		{ebiten.KeyL, true, false, "Shift+L"},
		{ebiten.KeyS, false, true, "Ctrl+S"},
		{ebiten.KeyF, false, true, "Ctrl+F"},
		{ebiten.KeyEscape, false, false, "Esc"},
		{ebiten.KeyBracketLeft, false, false, "["},
		{ebiten.KeyBracketRight, false, false, "]"},
		{ebiten.KeyMinus, false, false, "-"},
		{ebiten.KeyEqual, false, false, "="},
		{ebiten.KeyArrowUp, false, false, "Up arrow"},
		{ebiten.KeyArrowDown, false, false, "Down arrow"},
		{ebiten.KeyArrowLeft, false, false, "Left arrow"},
		{ebiten.KeyArrowRight, false, false, "Right arrow"},
	} {
		if got := chordOf(tc.key, tc.shift, tc.ctrl); got != tc.want {
			t.Errorf("chordOf(%v, shift=%v, ctrl=%v) = %q, want %q", tc.key, tc.shift, tc.ctrl, got, tc.want)
		}
	}
}

// settingsPanel is the mounted settings panel (the footer's reader).
func settingsPanel(t *testing.T, host *ScreenHost) *uikit.Settings {
	t.Helper()
	s, ok := host.Panel(idSettings).(*uikit.Settings)
	if !ok {
		t.Fatal("the settings row did not build a *uikit.Settings")
	}
	return s
}

// helpRowKey is the help overlay's key column for one action. It reads what
// refreshHelp pushes into the panel (helpRowsFor), the same call the panel
// gets on every rebind.
func helpRowKey(g *Game, action string) string {
	for _, r := range helpRowsFor(g) {
		if r.Desc == descOf(action) {
			return r.Key
		}
	}
	return "<no row>"
}

// TestRebindRefreshesEveryHint is the four-surface pin: the help overlay,
// the keybindings chip, the settings footer and the rail's tooltip hint all
// answer from the live binding table, so one rebind shows on all of them in
// the same frame and one unbind leaves the chip on "—" and drops the
// footer's close clause. Before the seams existed (RailEntry.Hint,
// SettingsActions.CloseHint) the last three were frozen at their defaults.
// (The tip is read on Market: Settings lost its rail entry with the
// Settings/Keybindings navigation pass.)
func TestRebindRefreshesEveryHint(t *testing.T) {
	g := newShowcaseGameAt(t, filepath.Join(t.TempDir(), "save.json"))
	g.applySize(1280, 720)
	for _, id := range []ScreenID{idSettings, idHelp, idKeybindings} {
		g.host.SetOpen(id, true)
	}
	pump(t, g, 1280, 720)

	surfaces := []struct {
		name                  string
		read                  func() string
		def, rebound, unbound string
	}{
		{
			"help row key", func() string { return helpRowKey(g, "settings") },
			"P", "U", "—",
		},
		{
			"keybindings chip", func() string {
				text, _, _ := keybinds(t, g.host).ChipForTest("settings")
				return text
			},
			"P", "U", "—",
		},
		{
			"settings hint", func() string { return settingsPanel(t, g.host).CloseHintForTest() },
			"Applies immediately · P closes", "Applies immediately · U closes", "Applies immediately",
		},
	}
	for _, s := range surfaces {
		if got := s.read(); got != s.def {
			t.Fatalf("%s at the defaults = %q, want %q", s.name, got, s.def)
		}
	}

	// P → U through the real capture path (U is nobody's default).
	g.host.services.Keys.Begin("settings")
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyU}})
	pump(t, g, 1280, 720)
	for _, s := range surfaces {
		if got := s.read(); got != s.rebound {
			t.Errorf("%s after P→U = %q, want %q", s.name, got, s.rebound)
		}
	}

	// Unbind: "—" where a key belongs and no "— closes" clause in the
	// footer.
	g.host.services.Keys.Begin("settings")
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyBackspace}})
	pump(t, g, 1280, 720)
	for _, s := range surfaces {
		if got := s.read(); got != s.unbound {
			t.Errorf("%s unbound = %q, want %q", s.name, got, s.unbound)
		}
	}

	// The rail's tooltip reads live too, on a row the rail still lists
	// (Settings is not a rail entry any more, so its own tip is gone with
	// it): the same rebind moves Market's tip.
	g.host.services.Keys.Begin("market")
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyU}})
	pump(t, g, 1280, 720)
	if got, want := entryByTitle(t, g, "Market").Hint(), "U — toggle market window (trade while docked)"; got != want {
		t.Errorf("rail tooltip hint after G→U = %q, want %q", got, want)
	}
}
