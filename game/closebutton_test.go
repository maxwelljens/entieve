package game

import (
	"testing"
)

// The Screen spec's close buttons (spec-driven): every row with a title
// bar carries a × (the kit attached it through SetWindowClose in
// buildKit), and pressing it runs THAT ROW'S OWN close semantics — the
// non-modal rows flip their open flag exactly like their key does, the
// modal rows pop their scene — after which the row reopens through the
// same toggle (the rail's and the key's path, unchanged). The one row
// without a title bar (Station Services) attaches no button by design.
//
// rowScene returns the scene a modal row's window is owned by (nil for
// a non-modal row).
func rowScene(g *Game, id panelID) Scene {
	switch id {
	case pMap:
		return &MapScene{g: g}
	case pHelp:
		return &HelpScene{g: g}
	case pMenu:
		return &MenuScene{g: g}
	}
	return nil
}

// sceneUp reports whether that scene type is on the stack.
func sceneUp(g *Game, id panelID) bool {
	return g.findScene(func(sc Scene) bool {
		switch id {
		case pMap:
			_, ok := sc.(*MapScene)
			return ok
		case pHelp:
			_, ok := sc.(*HelpScene)
			return ok
		case pMenu:
			_, ok := sc.(*MenuScene)
			return ok
		}
		return false
	})
}

func TestSpecCloseButtonsFollowTheirRow(t *testing.T) {
	// Two sessions cover the whole table: rows mount in space by
	// default, DockedOnly rows need the station (the gating is the same
	// panelDefs.want rule the key and rail ride).
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

			if s.ID == pServices {
				// The one row with no title bar: no × to attach, and the
				// window keeps its service-row header instead of a bar.
				win := g.screenWin[s.ID]
				if win.TitleBar != nil {
					t.Fatalf("services window grew a title bar (%v)", win.GetContainer().GetWidget().Rect)
				}
				if g.kit.CloseButtonOf(win) != nil {
					t.Error("title-less window has a × button")
				}
				return
			}

			// Open the row the way it opens in play: modals through
			// their scene, everything else through the open flag.
			if sc := rowScene(g, s.ID); sc != nil {
				if sceneUp(g, s.ID) {
					t.Fatal("fixture: scene already up from a previous row")
				}
				g.push(sc)
			} else {
				g.open[s.ID] = true
			}
			pump(t, g, 1280, 720)
			pump(t, g, 1280, 720)
			if g.mounted[s.ID] == nil {
				t.Fatalf("fixture: row did not mount (open=%v docked=%v)", g.open[s.ID], g.s.Docked.Has(g.s.Player()))
			}

			win := g.screenWin[s.ID]
			btn := g.kit.CloseButtonOf(win)
			if btn == nil {
				t.Fatalf("row mounted with no × button")
			}
			if btn.GetWidget().Rect.Empty() {
				t.Fatalf("× never laid out: %v", btn.GetWidget().Rect)
			}

			// Press it — the registered close is this row's own
			// toggleScreen: the flag flips (modal rows pop their scene
			// through it), exactly like the key and the rail entry.
			click(t, g, btn)
			pump(t, g, 1280, 720) // the next reconciliation unmounts
			if g.open[s.ID] {
				t.Errorf("× left the row open — close did not run the row's toggle")
			}
			if s.Modal && sceneUp(g, s.ID) {
				t.Error("× did not pop the row's scene")
			}
			if g.mounted[s.ID] != nil {
				t.Error("row still mounted after its close")
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
			if !g.open[s.ID] {
				t.Error("reopen toggle left the row closed")
			}
			if s.Modal != sceneUp(g, s.ID) {
				t.Errorf("reopen: scene on stack = %v, want modal=%v", sceneUp(g, s.ID), s.Modal)
			}
			if g.mounted[s.ID] == nil {
				t.Error("reopened row did not remount")
			}

			// Leave the row closed again for the rows that follow —
			// the next row's mount settles this close fade (the kit's
			// settle-on-mount rule), so no residue carries over.
			g.toggleScreen(s.Title)
			pump(t, g, 1280, 720)
		})
	}
}
