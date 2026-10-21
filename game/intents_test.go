package game

import (
	"image"
	"os"
	"testing"

	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/widgets"
)

// Intent → handler dispatch (uikit/widgets/intents.go + Game.Do): the intents a
// screen emits reach the very handlers the old buttons and keys drove,
// through the sink buildKit installs (Kit.SetSink).

// TestDoHandlesHostIntents: the game-level intents dispatch to the
// handlers only the game has — the Screen-spec toggle (whose flag is
// what setPanel mounts from) and the Esc menu's Save/Load/Quit — using
// the docked-showcase setup the behaviour tests share.
func TestDoHandlesHostIntents(t *testing.T) {
	g := dockedShowcaseGame(t, 1280, 720)

	// Toggle: docking opened ONLY the services view (nothing force-
	// opens any more), so the market starts closed and the intent flips
	// exactly what the screen's key flips.
	if g.open[pMarket] {
		t.Fatal("fixture: market opened by docking (only the services view opens)")
	}
	g.Do(widgets.Toggle{Screen: "Market"})
	if !g.open[pMarket] {
		t.Error("Toggle did not open the Market screen")
	}
	g.syncMounts()
	if g.mounted[pMarket] == nil {
		t.Error("opened Market screen did not mount")
	}
	g.Do(widgets.Toggle{Screen: "market"}) // the title matches case-insensitively
	if g.open[pMarket] {
		t.Error("second Toggle did not close the Market screen")
	}
	g.syncMounts()
	if g.mounted[pMarket] != nil {
		t.Error("closed Market screen stayed mounted (setPanel handler not reached)")
	}

	// A non-modal row flips its open flag, the way its key does.
	g.Do(widgets.Toggle{Screen: "Starmap"})
	if !g.open[pMap] {
		t.Error("Starmap toggle did not open the window")
	}
	g.Do(widgets.Toggle{Screen: "Starmap"})
	if g.open[pMap] {
		t.Error("second Starmap toggle did not close the window")
	}

	// The Esc menu's verbs: Save writes the session's save file (the
	// same handler Ctrl+S runs), Load defers to the post-pump rebuild,
	// Quit latches the loop's termination flag.
	g.Do(widgets.Save{})
	if _, err := os.Stat(g.SavePath()); err != nil {
		t.Errorf("Save intent did not write %s: %v", g.SavePath(), err)
	}
	g.Do(widgets.Load{})
	if !g.pendingLoad {
		t.Error("Load intent did not arm the deferred load")
	}
	g.pendingLoad = false
	g.Do(widgets.Quit{})
	if !g.quit {
		t.Error("Quit intent did not latch the quit flag")
	}
}

// TestDoDelegatesToTheKitExecutor: everything the game does not handle
// itself runs on the kit's executor — the session intents (Show Info,
// the context menu) land on the shared services, and commands queue on
// the game's enqueue tail.
func TestDoDelegatesToTheKitExecutor(t *testing.T) {
	g := dockedShowcaseGame(t, 1280, 720)
	var got []sim.Command
	g.enqueue = func(c sim.Command) { got = append(got, c) }

	// Undock: exactly what the services view's Undock button queues.
	g.Do(widgets.Undock{})
	if len(got) != 1 {
		t.Fatalf("Undock queued %#v, want the single CmdUndock", got)
	}
	if _, ok := got[0].(sim.CmdUndock); !ok {
		t.Errorf("cmd = %#v, want CmdUndock", got[0])
	}

	// Show Info: exactly what the menu's Show Info entries open.
	g.Do(widgets.ShowInfo{Ref: widgets.ItemRef("ammo-emp-s")})
	if !g.kit.InfoOpen() {
		t.Fatal("ShowInfo intent did not open the window")
	}
	if ref, _ := g.env.InfoWindow().Ref(); ref != widgets.ItemRef("ammo-emp-s") {
		t.Errorf("info ref = %v, want ammo-emp-s", ref)
	}

	// OpenMenu: exactly what the row installer's popup mounts.
	g.Do(widgets.OpenMenu{Entries: []widgets.MenuEntry{{Label: "Stub"}}, At: image.Pt(5, 6)})
	m := g.kit.Menu()
	if !m.IsOpen() || m.Rect().Min != (image.Pt(5, 6)) {
		t.Errorf("menu open=%v origin=%v, want open at the intent's point",
			m.IsOpen(), m.Rect().Min)
	}
	m.Close()
}
