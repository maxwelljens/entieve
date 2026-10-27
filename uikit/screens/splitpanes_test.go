package screens

import (
	"image"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// splitPanes is a split's pane rects, top to bottom (the children are
// pane, seam, pane, …).
func splitPanes(s *widgets.SplitPane) []image.Rectangle {
	var out []image.Rectangle
	for i, ch := range s.Container().Children() {
		if i%2 == 0 {
			out = append(out, ch.GetWidget().Rect)
		}
	}
	return out
}

// splitSeam is the seam container's rect between panes i and i+1.
func splitSeam(s *widgets.SplitPane, i int) image.Rectangle {
	return s.Container().Children()[2*i+1].GetWidget().Rect
}

// TestWindowSplitsEqualByDefault pins the multi-pane windows' opening
// shares with nothing stored: Inventory thirds, Manufacturing halves, and
// every split tiling its body exactly (no gap, no overflow). Fitting is the
// one exception — its tile grid needs the taller slots pane at the window's
// frozen height, so it opens at fitSlotShare.
func TestWindowSplitsEqualByDefault(t *testing.T) {
	env, _ := newTestEnv(t)

	cases := []struct {
		name  string
		build func() (*widget.Window, *widgets.SplitPane)
		panes int
		share float64 // seam 0's opening share; 0 = the equal-share default
	}{
		{"inventory", func() (*widget.Window, *widgets.SplitPane) {
			p := NewInventory(env)
			return p.Win(), p.split
		}, 3, 0},
		{"fitting", func() (*widget.Window, *widgets.SplitPane) {
			p := NewFitting(env)
			return p.Win(), p.split
		}, 2, fitSlotShare},
		{"manufacturing", func() (*widget.Window, *widgets.SplitPane) {
			p := NewManufacturing(env)
			return p.Win(), p.split
		}, 2, 0},
	}
	for _, c := range cases {
		win, split := c.build()
		uitest.PumpWindow(t, newWindowUI(t, env, win), nil, 1280, 720)

		for i := 0; i < c.panes-1; i++ {
			want := float64(i+1) / float64(c.panes)
			if i == 0 && c.share != 0 {
				want = c.share
			}
			if got := split.Ratio(i); got != want {
				t.Errorf("%s: seam %d ratio = %v, want %v", c.name, i, got, want)
			}
		}
		panes := splitPanes(split)
		if len(panes) != c.panes {
			t.Fatalf("%s: %d panes, want %d", c.name, len(panes), c.panes)
		}
		for i := 1; i < len(panes) && c.share == 0; i++ {
			if d := panes[i].Dy() - panes[i-1].Dy(); d < -1 || d > 1 {
				t.Errorf("%s: pane heights %v, want equal shares", c.name, panes)
			}
		}
		root := split.Container().GetWidget().Rect
		if panes[0].Min.Y != root.Min.Y || panes[len(panes)-1].Max.Y != root.Max.Y {
			t.Errorf("%s: panes %v do not tile the split %v", c.name, panes, root)
		}
	}
}

// TestInventorySplitDragTradesTwoPanes drives the real window input:
// a press in the cargo/ore seam's grab band, a 15 px downward drag, a
// release. The seam follows the cursor down: cargo's table grows by 15,
// ore's shrinks by the same, the hangar above/below them is untouched,
// and the seam's ratio follows.
func TestInventorySplitDragTradesTwoPanes(t *testing.T) {
	env, _ := newTestEnv(t)
	inv := NewInventory(env)
	ui := newWindowUI(t, env, inv.Win())
	pump := func() { uitest.PumpWindow(t, ui, inv.Sync, 1280, 720) }
	pump()

	before := splitPanes(inv.split)
	cargoBefore := inv.cargo.Container().GetWidget().Rect.Dy()
	seam := splitSeam(inv.split, 0)
	cur := image.Pt(seam.Min.X+5, seam.Min.Y)

	var down, held bool
	defer widgets.SetPollCursorForTest(func() (int, int) { return cur.X, cur.Y })()
	defer widgets.SetPressEdgeForTest(func(ebiten.MouseButton) bool {
		d := down
		down = false
		return d
	})()
	defer widgets.SetSplitHeldForTest(func() bool { return held })()

	pump() // hover the band
	down, held = true, true
	pump() // arm the grab
	cur.Y += 15
	pump() // drag
	held = false
	pump() // release

	after := splitPanes(inv.split)
	if d := after[0].Dy() - (before[0].Dy() + 15); d < -1 || d > 1 {
		t.Errorf("cargo pane = %d px, want %d (15 px taller: the seam moved down with the cursor)", after[0].Dy(), before[0].Dy()+15)
	}
	if d := after[1].Dy() - (before[1].Dy() - 15); d < -1 || d > 1 {
		t.Errorf("ore pane = %d px, want %d (15 px shorter)", after[1].Dy(), before[1].Dy()-15)
	}
	if after[0].Dy()+after[1].Dy() != before[0].Dy()+before[1].Dy() {
		t.Errorf("cargo+ore = %d px, want the unchanged pair %d (the pair trades space)",
			after[0].Dy()+after[1].Dy(), before[0].Dy()+before[1].Dy())
	}
	if d := after[2].Dy() - before[2].Dy(); d < -1 || d > 1 {
		t.Errorf("hangar pane = %d px, want %d (a seam moves only its pair)", after[2].Dy(), before[2].Dy())
	}
	// Adjacency, not just sizes: the stack still tiles without gaps.
	root := inv.split.Container().GetWidget().Rect
	if after[0].Min.Y != root.Min.Y || after[2].Max.Y != root.Max.Y {
		t.Errorf("panes %v do not tile the split %v", after, root)
	}
	if got := inv.Splits()[0].Split.Ratio(0); got == float64(1)/3 {
		t.Error("the drag left the seam ratio at its equal-share default")
	}
	// The tables (not only the panes) follow: the drag reaches layout.
	if got, want := inv.cargo.Container().GetWidget().Rect.Dy(), cargoBefore+15; got < want-1 || got > want+1 {
		t.Errorf("cargo table height = %d, want %d (the pane's 15 px)", got, want)
	}
}
