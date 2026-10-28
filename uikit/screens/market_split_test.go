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

// TestMarketSplitOpensAtTheSideColumn: the browse tree, the catalog and My
// orders share one row split whose opening shares reproduce the 176 px
// browse pane and the old fixed 240 px orders column at the 760 px landing
// rect, with two seams between the three panes.
func TestMarketSplitOpensAtTheSideColumn(t *testing.T) {
	env, _ := newTestEnv(t)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	frame := func() { uitest.PumpWindow(t, ui, m.Sync, 1024, 700) }
	frame()
	frame()

	if got, want := m.Win().GetContainer().GetWidget().Rect.Dx(), theme.Px(mktDefaultW); got != want {
		t.Fatalf("market window = %d px wide, want its %d px landing rect", got, want)
	}
	panes := splitPanes(m.split)
	if len(panes) != 3 {
		t.Fatalf("market split has %d panes, want browse | catalog | orders", len(panes))
	}
	if got, want := panes[0].Dx(), theme.Px(mktTreeW); got < want-1 || got > want+1 {
		t.Errorf("browse pane = %d px, want the %d px opening share", got, want)
	}
	if got, want := panes[2].Dx(), theme.Px(mktSideW); got < want-1 || got > want+1 {
		t.Errorf("My orders pane = %d px, want the old side column's %d px", got, want)
	}
	if got, want := m.split.Ratio(0), mktTreeShare; math.Abs(got-want) > 1e-9 {
		t.Errorf("tree seam ratio = %v, want the browse opening share %v", got, want)
	}
	if got, want := m.split.Ratio(1), 1-mktSideShare; math.Abs(got-want) > 1e-9 {
		t.Errorf("orders seam ratio = %v, want the catalog's opening share %v", got, want)
	}
	root := m.split.Container().GetWidget().Rect
	if panes[0].Min.X != root.Min.X || panes[2].Max.X != root.Max.X {
		t.Errorf("panes %v do not tile the split %v", panes, root)
	}
	if !m.tree.Container().GetWidget().Rect.In(panes[0]) {
		t.Errorf("browse tree %v is outside the browse pane %v", m.tree.Container().GetWidget().Rect, panes[0])
	}
	// The search field sits above the tree, inside the same pane.
	search := m.search.GetWidget().Rect
	if !search.In(panes[0]) {
		t.Errorf("search field %v is outside the browse pane %v", search, panes[0])
	}
	if search.Max.Y > m.tree.Container().GetWidget().Rect.Min.Y {
		t.Errorf("search field %v overlaps the tree %v", search, m.tree.Container().GetWidget().Rect)
	}
	if !m.tbl.Container().GetWidget().Rect.In(panes[1]) {
		t.Errorf("catalog table %v is outside the catalog pane %v", m.tbl.Container().GetWidget().Rect, panes[1])
	}
	if !m.mineRail.Container().GetWidget().Rect.In(panes[2]) {
		t.Errorf("My orders rail %v is outside the orders pane %v", m.mineRail.Container().GetWidget().Rect, panes[2])
	}
}

// TestMarketSplitDragTradesWidths drives the real window input: a press
// on the tree seam's grab band, a 20 px drag right (browse grows, the
// catalog shrinks by the same), a release; then the orders seam moves the
// same way between the catalog and My orders. The seam advertises the EW
// cursor and each ratio follows.
func TestMarketSplitDragTradesWidths(t *testing.T) {
	env, _ := newTestEnv(t)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	// A bare ebitenui.UI has no Kit.Draw, so the frame's seam-cursor apply
	// (the kit's one per-frame write) is explicit in this frame model —
	// after a fresh sample, as the host's Update does.
	pump := func() {
		env.Pointer().Begin()
		uitest.PumpWindow(t, ui, m.Sync, 1024, 700)
		env.Pointer().Apply()
	}
	pump()

	before := splitPanes(m.split)
	seam := splitSeam(m.split, 0)
	cur := image.Pt(seam.Min.X, (seam.Min.Y+seam.Max.Y)/2)
	env.in.x, env.in.y = cur.X, cur.Y

	pump() // hover the band
	if env.in.shape != ebiten.CursorShapeEWResize {
		t.Errorf("seam cursor = %v, want EW-resize", env.in.shape)
	}
	env.in.edge, env.in.down = [3]bool{true, false, false}, [3]bool{true, false, false}
	pump() // arm the grab
	cur.X += 20
	env.in.x = cur.X
	env.in.edge = [3]bool{}
	pump() // drag right
	env.in.down = [3]bool{}
	pump() // release

	after := splitPanes(m.split)
	if d := after[0].Dx() - (before[0].Dx() + 20); d < -1 || d > 1 {
		t.Errorf("browse pane = %d px, want %d (20 px wider)", after[0].Dx(), before[0].Dx()+20)
	}
	if d := after[1].Dx() - (before[1].Dx() - 20); d < -1 || d > 1 {
		t.Errorf("catalog pane = %d px, want %d (20 px narrower)", after[1].Dx(), before[1].Dx()-20)
	}
	if after[0].Dx()+after[1].Dx() != before[0].Dx()+before[1].Dx() {
		t.Errorf("pair width = %d px, want the unchanged %d (the pair trades space)",
			after[0].Dx()+after[1].Dx(), before[0].Dx()+before[1].Dx())
	}
	if got := m.split.Ratio(0); math.Abs(got-mktTreeShare) < 1e-6 {
		t.Error("the drag left the tree seam at its opening share")
	}

	// The orders seam (index 1) trades the catalog with My orders.
	moved := splitPanes(m.split)
	seam = splitSeam(m.split, 1)
	cur = image.Pt(seam.Min.X, (seam.Min.Y+seam.Max.Y)/2)
	env.in.x, env.in.y = cur.X, cur.Y
	env.in.edge, env.in.down = [3]bool{true, false, false}, [3]bool{true, false, false}
	pump()
	cur.X -= 20
	env.in.x = cur.X
	env.in.edge = [3]bool{}
	pump()
	env.in.down = [3]bool{}
	pump()
	end := splitPanes(m.split)
	if d := end[1].Dx() - (moved[1].Dx() - 20); d < -1 || d > 1 {
		t.Errorf("catalog pane = %d px, want %d after the orders-seam drag", end[1].Dx(), moved[1].Dx()-20)
	}
	if d := end[2].Dx() - (moved[2].Dx() + 20); d < -1 || d > 1 {
		t.Errorf("orders pane = %d px, want %d after the orders-seam drag", end[2].Dx(), moved[2].Dx()+20)
	}
	root := m.split.Container().GetWidget().Rect
	if end[0].Min.X != root.Min.X || end[2].Max.X != root.Max.X {
		t.Errorf("panes %v do not tile the split %v after the drags", end, root)
	}
}

// TestMarketSplitClampsAndFloors: a stored ratio outside the sane band
// restores clamped, and even a hard drag against the window's minimum
// width leaves My orders its SplitMinPane floor.
func TestMarketSplitClampsAndFloors(t *testing.T) {
	env, _ := newTestEnv(t)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	pump := func() { uitest.PumpWindow(t, ui, m.Sync, 1024, 700) }
	pump()

	// The orders seam's ratio is the catalog's share: clamp to the band.
	m.split.SetRatio(1, 2)
	if got := m.split.Ratio(1); math.Abs(got-widgets.SplitMaxRatio) > 1e-9 {
		t.Errorf("ratio 2 = %v, want the clamped max %v", got, widgets.SplitMaxRatio)
	}
	// It can never cross the browse pane: the pair it bounds only trades
	// its own width, so a stored 0 lands on the tree's share.
	m.split.SetRatio(1, -1)
	if got := m.split.Ratio(1); math.Abs(got-mktTreeShare) > 1e-9 {
		t.Errorf("ratio -1 = %v, want the browse share %v (the pair trades itself)", got, mktTreeShare)
	}

	// The window floor (300 px wide): push the orders seam to its extreme
	// and My orders stops at the pane floor, the split still tiling.
	m.Win().SetLocation(image.Rect(0, 0, theme.Px(300), theme.Px(400)))
	m.split.SetRatio(1, widgets.SplitMaxRatio)
	pump()
	panes := splitPanes(m.split)
	if got, want := panes[2].Dx(), theme.Px(widgets.SplitMinPane); got != want {
		t.Errorf("orders pane after the hard drag = %d px, want the %d px floor", got, want)
	}
	if total := panes[0].Dx() + panes[1].Dx() + panes[2].Dx() + 2*theme.Px(widgets.SplitSeamH); total != m.split.Container().GetWidget().Rect.Dx() {
		t.Errorf("panes + seams = %d px, want the split's %d", total, m.split.Container().GetWidget().Rect.Dx())
	}
}
