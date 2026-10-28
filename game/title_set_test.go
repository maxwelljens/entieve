package game

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
)

// The title's one window set (wave 6): ONE Screen host over ONE table —
// the title's own save manager and credits around the game table's InTitle
// rows — so the menu, the live key bindings and every × drive the same
// open flags, and no title window ever reaches the game host.

// TestTitleWindowsAreOneSet: P opens Settings; the Settings window's
// Keybindings button closes it and opens Keybindings (the window has no
// key of its own, so O does nothing); the menu's Credits row closes
// Keybindings and mounts Credits; Esc closes it; a rebind to U moves the
// title's key (P is dead); and the whole story touches exactly one title
// flag and no game-host row.
func TestTitleWindowsAreOneSet(t *testing.T) {
	resetScale(t)
	g := newTitleGameAt(t, filepath.Join(t.TempDir(), "save.json"))
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)

	press := func(k ebiten.Key) {
		t.Helper()
		if err := g.updateTitle(Input{JustPressed: []ebiten.Key{k}}); err != nil {
			t.Fatalf("updateTitle(%v): %v", k, err)
		}
	}

	press(ebiten.KeyP)
	if !g.titleUI.host.IsOpen(idSettings) || !g.titleUI.host.Mounted(idSettings) {
		t.Fatal("P did not open the title's settings window")
	}

	// One open window, ever: the Settings button closes Settings and
	// opens Keybindings. Keybindings has no key, so O is dead.
	press(ebiten.KeyO)
	if !g.titleUI.host.IsOpen(idSettings) || g.titleUI.host.IsOpen(idKeybindings) {
		t.Fatal("O must drive nothing: keybindings has no key")
	}
	click(t, g, settingsPanel(t, g.titleUI.host).KeybindingsButtonForTest())
	if err := g.Update(); err != nil {
		t.Fatalf("Update after the Keybindings button: %v", err)
	}
	if g.titleUI.host.IsOpen(idSettings) || g.titleUI.host.Mounted(idSettings) {
		t.Error("the button left the settings window open beside keybindings")
	}
	if !g.titleUI.host.IsOpen(idKeybindings) || !g.titleUI.host.Mounted(idKeybindings) {
		t.Fatal("the Settings button did not open the title's keybindings window")
	}

	// The menu's Credits label routes through the same lifecycle.
	item := titleItemByLabel(g, "Credits")
	if item == nil {
		t.Fatalf("no Credits row in the title menu (%v)", g.titleUI.labels())
	}
	item.Btn.ClickedEvent.Fire(&widget.ButtonClickedEventArgs{Button: item.Btn})
	if err := g.Update(); err != nil {
		t.Fatalf("Update after the Credits row: %v", err)
	}
	if g.titleUI.host.IsOpen(idKeybindings) || g.titleUI.host.Mounted(idKeybindings) {
		t.Error("the Credits row left the keybindings window open")
	}
	if !g.titleUI.host.Mounted(idCredits) {
		t.Fatal("the Credits row did not mount the credits window")
	}

	if err := g.updateTitle(Input{JustPressed: []ebiten.Key{ebiten.KeyEscape}}); err != nil {
		t.Fatalf("Esc: %v", err)
	}
	if g.titleUI.host.AnyOpen() || g.titleUI.host.Mounted(idCredits) {
		t.Error("Esc left a title window open")
	}
	if !g.titleUI.menuVisible() {
		t.Error("Esc did not bring the title menu back")
	}

	// A rebind moves the title's key with the binding table.
	g.beginCapture("settings")
	press(ebiten.KeyU)
	if k, _ := liveKey(g, "settings"); k != ebiten.KeyU {
		t.Fatalf("fixture: settings did not take U (%v)", k)
	}
	press(ebiten.KeyP)
	if g.titleUI.host.AnyOpen() {
		t.Error("P (no longer settings' key) still opened a title window")
	}
	press(ebiten.KeyU)
	if !g.titleUI.host.Mounted(idSettings) {
		t.Fatal("U (settings' live key) did not open the title's settings window")
	}

	// Exactly one window is up, and the game host never heard of it.
	open := 0
	for _, on := range g.titleUI.host.OpenFlags() {
		if on {
			open++
		}
	}
	if open != 1 {
		t.Errorf("%d title flags set, want exactly one (%v)", open, g.titleUI.host.OpenFlags())
	}
	for _, id := range []ScreenID{idSettings, idKeybindings, idCredits, idManager} {
		if g.host.IsOpen(id) || g.host.Mounted(id) {
			t.Errorf("the game host's %s row was touched by the title", id)
		}
	}
}

// TestTitleRowsStaySceneOwned is a freeze pin, not a bug hunt: the title's
// two own windows (and the InTitle rows) never enter the game table's
// sidecar name list or the KeyMap — the scene keeps its windows.
func TestTitleRowsStaySceneOwned(t *testing.T) {
	rows := titleRows(&titleScreen{})
	if probs := screenSpecErrors(rows); len(probs) > 0 {
		t.Fatalf("the title table is malformed: %v", probs)
	}
	// Menu order: manager, the InTitle rows, credits.
	var ids []ScreenID
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	if !slices.Equal(ids, []ScreenID{idManager, idSettings, idKeybindings, idCredits}) {
		t.Errorf("title rows = %v, want manager, settings, keybindings, credits", ids)
	}
	for _, id := range []ScreenID{idManager, idCredits} {
		if slices.Contains(sidecarNames(screens), string(id)) || slices.Contains(layoutNames, string(id)) {
			t.Errorf("%s entered the layout sidecar", id)
		}
		if slices.Contains(sidecarNames(rows), string(id)) {
			t.Errorf("%s is not transient in the title table", id)
		}
		for _, b := range KeyMap {
			if b.Action == string(id) {
				t.Errorf("%s entered the KeyMap as action %q", id, b.Action)
			}
		}
	}
}
