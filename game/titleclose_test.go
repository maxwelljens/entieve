package game

import (
	"path/filepath"
	"testing"
)

// TestTitleCloseButtonReturnsToMenu: the × on a title-scoped window (the
// lazily built Settings / Keybindings rows) runs THAT host's close, so the
// window unmounts, the title menu comes back and the title phase stays put
// (sim frozen, no gameplay chrome). The × must never reach the game host.
func TestTitleCloseButtonReturnsToMenu(t *testing.T) {
	for _, id := range []ScreenID{idKeybindings, idSettings} {
		id := id
		t.Run(string(id), func(t *testing.T) {
			g := newTitleGameAt(t, filepath.Join(t.TempDir(), "save.json"))
			g.applySize(1280, 720)
			pump(t, g, 1280, 720)

			g.titleUI.open(string(id))
			pump(t, g, 1280, 720)
			if !g.titleUI.host.Mounted(id) {
				t.Fatalf("fixture: %s did not mount at the title", id)
			}
			if g.titleUI.menu.GetWidget().IsVisible() {
				t.Fatal("fixture: menu still visible with the window up")
			}
			win := g.titleUI.host.Window(id)
			btn := g.kit.CloseButtonOf(win)
			if btn == nil {
				t.Fatalf("%s mounted with no × button", id)
			}
			if btn.GetWidget().Rect.Empty() {
				t.Fatalf("%s × never laid out: %v", id, btn.GetWidget().Rect)
			}

			h0 := g.s.Hash()
			click(t, g, btn)
			pump(t, g, 1280, 720)

			if g.titleUI.host.IsOpen(id) {
				t.Errorf("× left the title %s row open", id)
			}
			if g.titleUI.host.Mounted(id) {
				t.Errorf("title %s still mounted after its ×", id)
			}
			if !g.titleUI.menu.GetWidget().IsVisible() {
				t.Error("× did not bring the title menu back")
			}
			if g.host.IsOpen(id) {
				t.Errorf("× toggled the GAME host's %s row instead of the title's", id)
			}
			if !g.title {
				t.Fatal("× left the title phase")
			}
			if g.s.Hash() != h0 {
				t.Error("× at the title stepped the sim")
			}
		})
	}
}

// TestTitleOwnWindowsCloseButtons: the title's own windows (Credits, the
// save manager) keep their × → closeTop path — the window unmounts, the
// menu returns and the phase stays the title.
func TestTitleOwnWindowsCloseButtons(t *testing.T) {
	for _, which := range []string{"credits", "manager"} {
		which := which
		t.Run(which, func(t *testing.T) {
			g := newTitleGameAt(t, filepath.Join(t.TempDir(), "save.json"))
			g.applySize(1280, 720)
			pump(t, g, 1280, 720)
			g.titleUI.open(which)
			pump(t, g, 1280, 720)

			win := g.titleUI.credits
			if which == "manager" {
				win = g.titleUI.manager
			}
			if win == nil {
				t.Fatalf("fixture: %s did not mount", which)
			}
			btn := g.kit.CloseButtonOf(win)
			if btn == nil {
				t.Fatalf("%s has no × button", which)
			}
			click(t, g, btn)
			pump(t, g, 1280, 720)

			if g.titleUI.windowOpen() {
				t.Errorf("%s × left a title window open", which)
			}
			if !g.titleUI.menu.GetWidget().IsVisible() {
				t.Errorf("%s × did not bring the title menu back", which)
			}
			if !g.title {
				t.Errorf("%s × left the title phase", which)
			}
		})
	}
}
