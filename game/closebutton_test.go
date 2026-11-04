package game

import (
	"testing"
)

// The Screen spec's close buttons (spec-driven): every row with a title
// bar carries a × (the kit attached it through SetWindowClose in
// buildKit), and pressing it runs THAT ROW'S OWN close semantics — the
// row's one toggle, the same path as its key, the rail and the menu
// button (the modal rows included: the host owns their modality, so a
// scene is never involved). The one row without a title bar (Station
// Services) attaches no button by design.

func TestSpecCloseButtonsFollowTheirRow(t *testing.T) {
	// Two sessions cover the whole table: rows mount in space by
	// default, DockedOnly rows need the station (the gating is the same
	// host want rule the key and rail ride).
	space := newShowcaseGame(t)
	space.applySize(1280, 720)
	docked := dockedShowcaseGame(t, 1280, 720)

	for _, s := range screens {
		s := s
		t.Run(s.Title, func(t *testing.T) {
			g := space
			if s.DockedOnly {
				g = docked
			}

			if s.ID == idServices || s.ID == idProficiency {
				// The two rows with no title bar: no × to attach, and the
				// window keeps its own header instead of a bar (the
				// services board's strip; the proficiency layer is a
				// full-viewport sheet, closed by Esc, its key and the rail
				// entry).
				win := g.host.Window(s.ID)
				if win.TitleBar != nil {
					t.Fatalf("%s window grew a title bar (%v)", s.ID, win.GetContainer().GetWidget().Rect)
				}
				if g.kit.CloseButtonOf(win) != nil {
					t.Error("title-less window has a × button")
				}
				return
			}

			// Open the row the way it opens in play: its flag (the toggle
			// behind the key, the rail and the menu button).
			g.host.SetOpen(s.ID, true)
			pump(t, g, 1280, 720)
			pump(t, g, 1280, 720)
			if !g.host.Mounted(s.ID) {
				t.Fatalf("fixture: row did not mount (open=%v docked=%v)", g.host.IsOpen(s.ID), g.s.Docked.Has(g.s.Player()))
			}

			win := g.host.Window(s.ID)
			btn := g.kit.CloseButtonOf(win)
			if btn == nil {
				t.Fatalf("row mounted with no × button")
			}
			if btn.GetWidget().Rect.Empty() {
				t.Fatalf("× never laid out: %v", btn.GetWidget().Rect)
			}

			// Press it — the registered close is this row's own toggle:
			// the flag flips, exactly like the key and the rail entry.
			click(t, g, btn)
			pump(t, g, 1280, 720) // the next reconciliation unmounts
			if g.host.IsOpen(s.ID) {
				t.Errorf("× left the row open — close did not run the row's toggle")
			}
			// A Persistent row hides on its close and stays mounted (the
			// two always-on windows); every other row unmounts.
			if g.host.Mounted(s.ID) != s.Persistent {
				t.Errorf("row mounted after its close = %v, want %v (Persistent)", g.host.Mounted(s.ID), s.Persistent)
			}
			if win.GetContainer().GetWidget().IsVisible() {
				t.Error("closed row is still visible")
			}

			// Reopen MID-FADE — the same toggle the rail and the key
			// use. The kit's reopen path cancels the running close fade
			// (addWindow drops the pending unmount) and the window
			// fades back in; the fade and that cancel are pinned
			// kit-side, this asserts the row semantics survive it.
			g.toggleScreen(s.Title)
			pump(t, g, 1280, 720)
			if !g.host.IsOpen(s.ID) {
				t.Error("reopen toggle left the row closed")
			}
			if !g.host.Mounted(s.ID) || !win.GetContainer().GetWidget().IsVisible() {
				t.Error("reopened row is not mounted and visible")
			}

			// Leave the row closed again for the rows that follow —
			// the next row's mount settles this close fade (the kit's
			// settle-on-mount rule), so no residue carries over.
			g.toggleScreen(s.Title)
			pump(t, g, 1280, 720)
		})
	}
}
