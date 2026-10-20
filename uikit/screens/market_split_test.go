package screens

import (
	"image"
	"math"
	"testing"

	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestMarketSplitOpensAtTheSideColumn: the book and My orders share one
// row split whose opening ratio reproduces the old fixed 240 px column at
// the window's 546 px landing square, with the table in the book pane and
// the orders rail in the side pane.
func TestMarketSplitOpensAtTheSideColumn(t *testing.T) {
	env, _ := newTestEnv(t)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	frame := func() { uitest.PumpWindow(t, ui, m.Sync, 1024, 700) }
	frame()
	frame()

	if got, want := m.Win().GetContainer().GetWidget().Rect.Dx(), theme.Px(mktDefaultW); got != want {
		t.Fatalf("market window = %d px wide, want its %d px landing square", got, want)
	}
	panes := splitPanes(m.split)
	if len(panes) != 2 {
		t.Fatalf("market split has %d panes, want book | orders", len(panes))
	}
	if got, want := panes[1].Dx(), theme.Px(mktSideW); got < want-1 || got > want+1 {
		t.Errorf("My orders pane = %d px, want the old side column's %d px", got, want)
	}
	if got, want := m.split.Ratio(0), 1-mktSideShare; math.Abs(got-want) > 1e-9 {
		t.Errorf("market seam ratio = %v, want the book's opening share %v", got, want)
	}
	root := m.split.Container().GetWidget().Rect
	if panes[0].Min.X != root.Min.X || panes[1].Max.X != root.Max.X {
		t.Errorf("panes %v do not tile the split %v", panes, root)
	}
	if !m.tbl.Container().GetWidget().Rect.In(panes[0]) {
		t.Errorf("order book %v is outside the book pane %v", m.tbl.Container().GetWidget().Rect, panes[0])
	}
	if !m.mineRail.Container().GetWidget().Rect.In(panes[1]) {
		t.Errorf("My orders rail %v is outside the orders pane %v", m.mineRail.Container().GetWidget().Rect, panes[1])
	}
}

// TestMarketSplitDragTradesWidths drives the real window input: a press
// on the seam's grab band, a 20 px drag right (the book grows, the orders
// pane shrinks by the same), a release. The seam advertises the EW cursor
// and the ratio follows.
func TestMarketSplitDragTradesWidths(t *testing.T) {
	env, _ := newTestEnv(t)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	// A bare ebitenui.UI has no Kit.Draw, so the frame's seam-cursor apply
	// (the kit's one per-frame write) is explicit in this frame model.
	pump := func() {
		uitest.PumpWindow(t, ui, m.Sync, 1024, 700)
		widgets.ApplyCursor()
	}
	pump()

	var shape ebiten.CursorShapeType
	defer widgets.SetCursorShapeForTest(func(c ebiten.CursorShapeType) { shape = c })()
	before := splitPanes(m.split)
	seam := splitSeam(m.split, 0)
	cur := image.Pt(seam.Min.X, (seam.Min.Y+seam.Max.Y)/2)

	var down, held bool
	defer widgets.SetPollCursorForTest(func() (int, int) { return cur.X, cur.Y })()
	defer widgets.SetPressEdgeForTest(func(ebiten.MouseButton) bool {
		d := down
		down = false
		return d
	})()
	defer widgets.SetSplitHeldForTest(func() bool { return held })()

	pump() // hover the band
	if shape != ebiten.CursorShapeEWResize {
		t.Errorf("seam cursor = %v, want EW-resize", shape)
	}
	down, held = true, true
	pump() // arm the grab
	cur.X += 20
	pump() // drag right
	held = false
	pump() // release

	after := splitPanes(m.split)
	if d := after[0].Dx() - (before[0].Dx() + 20); d < -1 || d > 1 {
		t.Errorf("book pane = %d px, want %d (20 px wider)", after[0].Dx(), before[0].Dx()+20)
	}
	if d := after[1].Dx() - (before[1].Dx() - 20); d < -1 || d > 1 {
		t.Errorf("orders pane = %d px, want %d (20 px narrower)", after[1].Dx(), before[1].Dx()-20)
	}
	if after[0].Dx()+after[1].Dx() != before[0].Dx()+before[1].Dx() {
		t.Errorf("pair width = %d px, want the unchanged %d (the pair trades space)",
			after[0].Dx()+after[1].Dx(), before[0].Dx()+before[1].Dx())
	}
	if got := m.Splits()[0].Split.Ratio(0); math.Abs(got-(1-mktSideShare)) < 1e-9 {
		t.Error("the drag left the seam ratio at its opening share")
	}
	root := m.split.Container().GetWidget().Rect
	if after[0].Min.X != root.Min.X || after[1].Max.X != root.Max.X {
		t.Errorf("panes %v do not tile the split %v after the drag", after, root)
	}
}

// TestMarketSplitClampsAndFloors: a stored ratio outside the sane band
// restores clamped, and even a hard drag against the window's minimum
// width leaves the orders pane its SplitMinPane floor.
func TestMarketSplitClampsAndFloors(t *testing.T) {
	env, _ := newTestEnv(t)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	pump := func() { uitest.PumpWindow(t, ui, m.Sync, 1024, 700) }
	pump()

	m.split.SetRatio(0, 2)
	if got := m.split.Ratio(0); got != widgets.SplitMaxRatio {
		t.Errorf("ratio 2 = %v, want the clamped max %v", got, widgets.SplitMaxRatio)
	}
	m.split.SetRatio(0, -1)
	if got := m.split.Ratio(0); got != widgets.SplitMinRatio {
		t.Errorf("ratio -1 = %v, want the clamped min %v", got, widgets.SplitMinRatio)
	}

	// The window floor (300 px wide): push the seam to its extreme and
	// the orders pane stops at the pane floor, the split still tiling.
	m.Win().SetLocation(image.Rect(0, 0, theme.Px(300), theme.Px(400)))
	m.split.SetRatio(0, widgets.SplitMaxRatio)
	pump()
	panes := splitPanes(m.split)
	if got, want := panes[1].Dx(), theme.Px(widgets.SplitMinPane); got != want {
		t.Errorf("orders pane after the hard drag = %d px, want the %d px floor", got, want)
	}
	if total := panes[0].Dx() + panes[1].Dx() + theme.Px(widgets.SplitSeamH); total != m.split.Container().GetWidget().Rect.Dx() {
		t.Errorf("panes + seam = %d px, want the split's %d", total, m.split.Container().GetWidget().Rect.Dx())
	}
}
