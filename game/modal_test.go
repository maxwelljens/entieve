package game

import (
	"image"
	"slices"
	"testing"

	"github.com/entieve/entieve/uikit/widgets"
)

// Modality is a row property (the Modal facet): the Screen host owns a
// modal row's open and close step, the veil it paints and its place in
// the Esc ladder. These are the pins for that one host path — the ladder
// as a whole chain, the veils as data, the single toggle every door runs,
// and the help window restoring from the sidecar like any other row. The
// settings session's return to the pause menu is pinned next to the rest
// of the pause menu (title_test.go), which is where its doors live.

// TestModalEscLadder: Esc is one ladder — the proficiency card, then the
// tree layer, then the open modal rows in reverse declaration order (the
// menu outranks help), and with none up it opens the menu.
func TestModalEscLadder(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)

	// Nothing up: the ladder's last step opens the menu.
	g.onEscape()
	if !g.host.IsOpen(idMenu) {
		t.Fatal("Esc with no modal up did not open the menu")
	}

	// Help sits under the menu: the menu closes first (declared last).
	g.host.SetOpen(idHelp, true)
	g.onEscape()
	if g.host.IsOpen(idMenu) {
		t.Error("Esc closed help while the menu was above it")
	}
	if !g.host.IsOpen(idHelp) {
		t.Error("Esc took help down with the menu")
	}

	// Help alone: Esc closes it, and the menu does not open behind it.
	g.onEscape()
	if g.host.IsOpen(idHelp) || g.host.IsOpen(idMenu) {
		t.Errorf("Esc on help = {help %v, menu %v}, want both closed",
			g.host.IsOpen(idHelp), g.host.IsOpen(idMenu))
	}

	// The session hands back to the menu it belongs to, and the next Esc
	// closes the menu.
	g.host.SetOpen(idMenu, true)
	g.toggleScreen("Settings")
	if !g.host.Session(idSettings) {
		t.Fatal("the Settings toggle did not open the menu's session")
	}
	g.onEscape()
	if !g.host.IsOpen(idMenu) || g.host.IsOpen(idSettings) {
		t.Errorf("Esc from the session = {menu %v, settings %v}, want the menu back",
			g.host.IsOpen(idMenu), g.host.IsOpen(idSettings))
	}
	g.onEscape()
	if g.host.IsOpen(idMenu) {
		t.Error("Esc from the returned menu did not close it")
	}

	// The tree is the layer above every modal: Esc closes it first and the
	// menu behind it stays.
	g.host.SetOpen(idMenu, true)
	g.host.SetOpen(idProficiency, true)
	g.onEscape()
	if g.host.IsOpen(idProficiency) || !g.host.IsOpen(idMenu) {
		t.Errorf("Esc with the tree over the menu = {tree %v, menu %v}, want the tree closed",
			g.host.IsOpen(idProficiency), g.host.IsOpen(idMenu))
	}
	g.onEscape()
	if g.host.IsOpen(idMenu) {
		t.Error("Esc after the tree did not close the menu")
	}

	// The proficiency card is one step above the tree: Esc backs out of the
	// card, then the tree, and only then does Esc reach the menu.
	g.host.SetOpen(idProficiency, true)
	g.kit.OpenProfFamily(widgets.ShowProfFamily{Family: "mining", At: image.Pt(640, 360), Level: 1})
	g.onEscape()
	if g.kit.ProfFamilyOpen() || !g.host.IsOpen(idProficiency) {
		t.Error("Esc did not back out of the card first")
	}
	g.onEscape()
	if g.host.IsOpen(idProficiency) {
		t.Error("Esc did not close the tree after the card")
	}
	g.onEscape()
	if !g.host.IsOpen(idMenu) {
		t.Error("Esc with nothing up did not open the menu")
	}
}

// TestModalMenuClosesUnderTheStation: the menu can sit UNDER the station
// scene (a dock lands above it). Esc closes that one menu wherever it
// sits; the top-only check this replaced pushed a second menu over the
// station and left the modal window (and its veil) up.
func TestModalMenuClosesUnderTheStation(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	g.host.SetOpen(idMenu, true)
	dockAt(t, g)
	g.syncScenes()
	if _, top := g.top().(*StationScene); !top {
		t.Fatalf("fixture: top scene is %T, want the station above the menu", g.top())
	}
	g.onEscape()
	if g.host.IsOpen(idMenu) {
		t.Fatal("Esc left a menu open under the station")
	}
	pump(t, g, 1280, 720)
	if g.host.Mounted(idMenu) {
		t.Error("the menu window is still mounted after Esc")
	}
	if v := g.host.Veils(); len(v) != 0 {
		t.Errorf("veils = %v, want none with the menu closed", v)
	}
}

// TestModalVeils: the dim is the row's declaration, painted in declaration
// order — help's 180 under the menu's 170, the session's 170, and none for
// the settings window its own key opens.
func TestModalVeils(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	if v := g.host.Veils(); len(v) != 0 {
		t.Fatalf("boot veils = %v, want none", v)
	}

	g.host.SetOpen(idHelp, true)
	if v := g.host.Veils(); !slices.Equal(v, []uint8{180}) {
		t.Errorf("help veil = %v, want [180]", v)
	}
	g.host.SetOpen(idMenu, true)
	if v := g.host.Veils(); !slices.Equal(v, []uint8{180, 170}) {
		t.Errorf("help + menu veils = %v, want [180 170] (declaration order)", v)
	}
	g.host.SetOpen(idHelp, false)
	g.host.SetOpen(idMenu, false)

	// The session dims; the plain window does not.
	g.host.Toggle(idSettings)
	if v := g.host.Veils(); len(v) != 0 {
		t.Errorf("plain settings veils = %v, want none", v)
	}
	g.host.Toggle(idSettings)
	g.host.SetOpen(idMenu, true)
	g.toggleScreen("Settings")
	if v := g.host.Veils(); !slices.Equal(v, []uint8{170}) {
		t.Errorf("session veil = %v, want [170]", v)
	}
}

// TestModalRowsOpenAndCloseThroughOnePath: every modal row opens and
// closes through the same host toggle its key, the rail and the × run —
// flag, mount and the window's modal layer together.
func TestModalRowsOpenAndCloseThroughOnePath(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	for _, s := range screens {
		if s.Modal.Dim == 0 {
			continue
		}
		g.host.Toggle(s.ID)
		pump(t, g, 1280, 720)
		if !g.host.IsOpen(s.ID) || !g.host.Mounted(s.ID) {
			t.Errorf("%s: toggle did not open and mount the row", screenLabel(s))
		}
		// A session row opened outside its session is the plain window
		// its own key has always opened.
		wantModal := s.Modal.Session == ""
		if w := g.host.Window(s.ID); w == nil || w.Modal != wantModal {
			t.Errorf("%s: window modal = %v after the toggle, want %v", screenLabel(s), w.Modal, wantModal)
		}
		g.host.Toggle(s.ID)
		pump(t, g, 1280, 720)
		if g.host.IsOpen(s.ID) || g.host.Mounted(s.ID) {
			t.Errorf("%s: toggle did not close and unmount the row", screenLabel(s))
		}
	}
}

// TestModalHelpRestoresFromTheSidecar: the help window is a row, so the
// layout sidecar's open flag mounts it like every other row — modal, with
// its veil — and no scene has to be rebuilt for it.
func TestModalHelpRestoresFromTheSidecar(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)
	g.host.SetOpen(idHelp, true)
	g.syncMounts()
	pump(t, g, 1280, 720)
	if err := g.writeLayout(); err != nil {
		t.Fatalf("writeLayout: %v", err)
	}
	if e := readDoc(t, g.layoutPath()).Windows["help"]; !e.Open {
		t.Fatal("the open help row did not persist its flag")
	}

	// Close it, then restore the file over a rebuilt interface.
	g.host.SetOpen(idHelp, false)
	g.syncMounts()
	pump(t, g, 1280, 720)
	if g.host.IsOpen(idHelp) || g.host.Mounted(idHelp) {
		t.Fatal("fixture: help still open")
	}
	g.applyLayout(1280, 720)
	pump(t, g, 1280, 720)
	if !g.host.IsOpen(idHelp) || !g.host.Mounted(idHelp) {
		t.Error("the restored flag did not open and mount the help row")
	}
	if w := g.host.Window(idHelp); w == nil || !w.Modal {
		t.Error("the restored help window is not modal")
	}
	if v := g.host.Veils(); !slices.Equal(v, []uint8{180}) {
		t.Errorf("restored help veil = %v, want [180]", v)
	}
}

// TestModalSessionEndsWithItsRow: a session flag cannot outlive its open
// row. A close by flag (the sidecar restore path, a programmatic close)
// ends the session, so the next plain open is the floating window its key
// has always opened and Esc no longer hands back to the menu.
func TestModalSessionEndsWithItsRow(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	g.host.SetOpen(idMenu, true)
	g.toggleScreen("Settings")
	if !g.host.Session(idSettings) {
		t.Fatal("fixture: no session")
	}
	g.host.SetOpen(idSettings, false) // the close a restore (not the ×) takes
	g.host.SetOpen(idMenu, false)
	g.host.Toggle(idSettings) // the P path
	if g.host.Session(idSettings) {
		t.Error("the closed session came back with the plain open")
	}
	if w := g.host.Window(idSettings); w == nil || w.Modal {
		t.Error("the plain open is modal: a stale session survived its row")
	}
	if v := g.host.Veils(); len(v) != 0 {
		t.Errorf("veils = %v, want none behind the plain settings window", v)
	}
}

// TestModalMenuNeverPersists: the pause menu is transient — opening it
// leaves the sidecar untouched, so a restart cannot reopen it (the quit
// and reboot paths read the same file).
func TestModalMenuNeverPersists(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)
	g.host.SetOpen(idMenu, true)
	pump(t, g, 1280, 720)
	if err := g.writeLayout(); err != nil {
		t.Fatalf("writeLayout: %v", err)
	}
	if _, ok := readDoc(t, g.layoutPath()).Windows["menu"]; ok {
		t.Error("the Esc menu was persisted: a restart could reopen it")
	}
	if !g.host.IsOpen(idMenu) {
		t.Error("writing the sidecar closed the menu")
	}
}
