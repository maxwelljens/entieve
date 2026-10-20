package widgets

import (
	"image"
	"math"
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// newPumpedSplit stacks n uniform panes in a split, mounts it in a window
// (the whole widget only ever runs inside one: seam grabs ask the
// topmost input layer) and pumps a frame at 640×480.
func newPumpedSplit(t *testing.T, n int) (*SplitPane, *ebitenui.UI) {
	t.Helper()
	env, _ := newTestEnv(t)
	panes := make([]widget.PreferredSizeLocateableWidget, n)
	for i := range panes {
		panes[i] = widget.NewContainer(
			widget.ContainerOpts.Layout(widget.NewRowLayout()),
			widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(0, theme.Px(SplitMinPane))),
		)
	}
	s := NewSplitPane(env, panes...)
	win := widget.NewWindow(
		widget.WindowOpts.Contents(s.Container()),
		widget.WindowOpts.TitleBar(
			widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout())),
			theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(200), theme.Px(200)),
		widget.WindowOpts.Location(image.Rect(0, 0, 400, 400)),
	)
	ui := newWindowUI(t, env, win)
	uitest.PumpWindow(t, ui, nil, 640, 480)
	return s, ui
}

// paneHeights is the panes' laid-out heights.
func paneHeights(s *SplitPane) []int {
	out := make([]int, len(s.panes))
	for i, p := range s.panes {
		out[i] = p.GetWidget().Rect.Dy()
	}
	return out
}

// seamMid is the grab point on seam i: the seam's centre x, its top y.
func seamMid(s *SplitPane, i int) image.Point {
	r := s.seams[i].GetWidget().Rect
	return image.Pt(r.Min.X+r.Dx()/2, r.Min.Y)
}

// TestSplitPaneEqualSharesByDefault: with no stored ratio the panes split
// the stack equally, a hairline seam sits between each pair, and the
// parts tile the rect exactly.
func TestSplitPaneEqualSharesByDefault(t *testing.T) {
	s, _ := newPumpedSplit(t, 3)
	for i := 0; i < len(s.shares)-1; i++ {
		if want := float64(i+1) / 3; s.Ratio(i) != want {
			t.Fatalf("seam %d default ratio = %v, want %v (equal shares)", i, s.Ratio(i), want)
		}
	}
	h := paneHeights(s)
	if h[0] != h[1] || h[1] != h[2] {
		t.Errorf("pane heights = %v, want equal thirds", h)
	}
	total := 0
	for _, v := range h {
		total += v
	}
	total += s.seamPx * len(s.seams)
	if rect := s.root.GetWidget().Rect; total != rect.Dy() {
		t.Errorf("panes + seams = %d, want the split's %d (no gap, no overflow)", total, rect.Dy())
	}
	if seam := s.seams[0].GetWidget().Rect; seam.Dy() != s.seamPx {
		t.Errorf("seam height = %d, want the hairline %d", seam.Dy(), s.seamPx)
	}
}

// TestSplitPaneDragMovesPairProportionally: dragging seam 0 down grows
// the pane below it by the drag distance, shrinks the pane above it by
// the same, and leaves the third pane untouched — the pair trades space,
// the stack never changes size.
func TestSplitPaneDragMovesPairProportionally(t *testing.T) {
	s, ui := newPumpedSplit(t, 3)
	before := paneHeights(s)
	at := seamMid(s, 0)

	s.Step(at, true, true)
	s.Step(image.Pt(at.X, at.Y+30), false, true)
	s.Step(image.Pt(at.X, at.Y+30), false, false)
	uitest.PumpWindow(t, ui, nil, 640, 480)

	after := paneHeights(s)
	want := before[1] + 30
	if after[1] != want {
		t.Errorf("lower pane = %d, want %d (grew by the 30 px drag)", after[1], want)
	}
	if after[0] != before[0]-30 {
		t.Errorf("upper pane = %d, want %d (shrank by the same 30 px)", after[0], before[0]-30)
	}
	if after[2] != before[2] {
		t.Errorf("third pane = %d, want %d (a seam moves only its pair)", after[2], before[2])
	}
	avail := before[0] + before[1] + before[2] // the stack without the hairlines
	if got, want := s.Ratio(0), float64(after[0])/float64(avail); got != want {
		t.Errorf("seam 0 ratio = %v, want %v (the stack fraction above it)", got, want)
	}
	if got, want := s.Ratio(1), float64(after[0]+after[1])/float64(avail); got != want {
		t.Errorf("seam 1 ratio = %v, want %v (untouched by seam 0's drag)", got, want)
	}
}

// TestSplitPaneMinPaneHolds: a huge drag in either direction stops at the
// pane floor — nothing collapses to zero — and the stack still tiles.
func TestSplitPaneMinPaneHolds(t *testing.T) {
	s, ui := newPumpedSplit(t, 3)
	at := seamMid(s, 1)

	// Drag the seam to the top: the lower pane bottoms out at the floor.
	s.Step(at, true, true)
	s.Step(image.Pt(at.X, at.Y-10000), false, true)
	s.Step(image.Pt(at.X, at.Y-10000), false, false)
	uitest.PumpWindow(t, ui, nil, 640, 480)
	if got := paneHeights(s)[2]; got != s.minPx {
		t.Errorf("lower pane after the hard upward drag = %d, want the floor %d", got, s.minPx)
	}

	// …and to the bottom: the upper pane goes to the floor, the lower grows.
	at = seamMid(s, 1)
	s.Step(at, true, true)
	s.Step(image.Pt(at.X, at.Y+10000), false, true)
	s.Step(image.Pt(at.X, at.Y+10000), false, false)
	uitest.PumpWindow(t, ui, nil, 640, 480)
	h := paneHeights(s)
	if h[1] != s.minPx {
		t.Errorf("upper pane after the hard downward drag = %d, want the floor %d", h[1], s.minPx)
	}
	total := h[0] + h[1] + h[2] + s.seamPx*len(s.seams)
	if rect := s.root.GetWidget().Rect; total != rect.Dy() {
		t.Errorf("panes + seams = %d, want the split's %d", total, rect.Dy())
	}

	// Too little room for three floors (a shrunken window): equal shares
	// win over the ratio and the panes still tile exactly.
	s.root.SetLocation(image.Rect(0, 0, 100, 150))
	uitest.PumpWindow(t, ui, nil, 640, 480)
	h = paneHeights(s)
	if max(h[0], max(h[1], h[2]))-min(h[0], min(h[1], h[2])) > 1 {
		t.Errorf("tiny split heights = %v, want equal shares (±1 px remainder)", h)
	}
	if total := h[0] + h[1] + h[2] + s.seamPx*len(s.seams); total != 150 {
		t.Errorf("tiny split tiles %d px, want the 150 px rect", total)
	}
}

// TestSplitPaneSetRatioClamps: a stored ratio is clamped to the sane
// band, NaN and out-of-range seam indexes are ignored, and a same-value
// write is a no-op (no relayout storm from a per-frame restore).
func TestSplitPaneSetRatioClamps(t *testing.T) {
	s, _ := newPumpedSplit(t, 2)
	for _, c := range []struct{ in, want float64 }{
		{5, SplitMaxRatio}, {-3, SplitMinRatio}, {0.42, 0.42}, {0, SplitMinRatio},
	} {
		s.SetRatio(0, c.in)
		if got := s.Ratio(0); got != c.want {
			t.Errorf("SetRatio(%v) → %v, want %v", c.in, got, c.want)
		}
	}
	s.SetRatio(0, 0.3)
	s.SetRatio(0, math.NaN())
	if got := s.Ratio(0); got != 0.3 {
		t.Errorf("NaN ratio = %v, want the previous 0.3", got)
	}
	s.SetRatio(4, 0.4)
	if got := s.Ratio(4); got != 0.5 {
		t.Errorf("out-of-range seam ratio = %v, want the 0.5 fallback", got)
	}

	// The window-layout reset puts the stack back to equal shares.
	s.SetRatio(0, 0.3)
	s.Reset()
	if got := s.Ratio(0); got != 0.5 {
		t.Errorf("after Reset ratio = %v, want the equal share 0.5", got)
	}
}

// TestSplitPaneGrabBand: the press must land within 4 px of the hairline
// and on the stack's own input layer; hovering the band advertises the
// NS-resize cursor.
func TestSplitPaneGrabBand(t *testing.T) {
	s, _ := newPumpedSplit(t, 2)
	at := seamMid(s, 0)
	grab := theme.Px(SplitGrabPx)

	if got := s.seamAt(image.Pt(at.X, at.Y+grab+2)); got != -1 {
		t.Errorf("press %d px below the seam grabbed seam %d, want none", grab+2, got)
	}
	if got := s.seamAt(image.Pt(at.X, at.Y+grab-1)); got != 0 {
		t.Errorf("press inside the grab band = seam %d, want 0", got)
	}

	cursors := 0
	defer SetSplitCursorForTest(func(ebiten.CursorShapeType) { cursors++ })()
	s.Step(at, false, false) // idle hover
	if cursors == 0 {
		t.Error("hovering the seam advertised no resize cursor")
	}
}

// newPumpedRow places n uniform panes side by side in a row split and
// pumps a frame inside a 400×400 window (the horizontal twin of
// newPumpedSplit).
func newPumpedRow(t *testing.T, n int) (*SplitPane, *ebitenui.UI) {
	t.Helper()
	env, _ := newTestEnv(t)
	panes := make([]widget.PreferredSizeLocateableWidget, n)
	for i := range panes {
		panes[i] = widget.NewContainer(
			widget.ContainerOpts.Layout(widget.NewRowLayout()),
			widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(SplitMinPane), 0)),
		)
	}
	s := NewSplitPaneRow(env, panes...)
	win := widget.NewWindow(
		widget.WindowOpts.Contents(s.Container()),
		widget.WindowOpts.TitleBar(
			widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout())),
			theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(200), theme.Px(200)),
		widget.WindowOpts.Location(image.Rect(0, 0, 400, 400)),
	)
	ui := newWindowUI(t, env, win)
	uitest.PumpWindow(t, ui, nil, 640, 480)
	return s, ui
}

// rowWidths is the panes' laid-out widths, left to right.
func rowWidths(s *SplitPane) []int {
	out := make([]int, len(s.panes))
	for i, p := range s.panes {
		out[i] = p.GetWidget().Rect.Dx()
	}
	return out
}

// rowSeamMid is the grab point on seam i in a row: the seam's left x and
// its vertical centre.
func rowSeamMid(s *SplitPane, i int) image.Point {
	r := s.seams[i].GetWidget().Rect
	return image.Pt(r.Min.X, r.Min.Y+r.Dy()/2)
}

// TestSplitPaneRowEqualShares: a row split places the panes side by side
// in equal shares, a hairline seam between each pair (full height, one
// pixel wide), the parts tiling the rect; the grab band runs across the
// seam and advertises the EW cursor.
func TestSplitPaneRowEqualShares(t *testing.T) {
	s, _ := newPumpedRow(t, 3)
	w := rowWidths(s)
	if max(w[0], max(w[1], w[2]))-min(w[0], min(w[1], w[2])) > 1 {
		t.Errorf("pane widths = %v, want equal thirds (±1 px remainder)", w)
	}
	total := w[0] + w[1] + w[2] + s.seamPx*len(s.seams)
	if rect := s.root.GetWidget().Rect; total != rect.Dx() {
		t.Errorf("panes + seams = %d, want the split's %d (no gap, no overflow)", total, rect.Dx())
	}
	if seam := s.seams[0].GetWidget().Rect; seam.Dx() != s.seamPx || seam.Dy() != s.root.GetWidget().Rect.Dy() {
		t.Errorf("seam = %v, want a %d px hairline over the full height", seam, s.seamPx)
	}

	at := rowSeamMid(s, 0)
	grab := theme.Px(SplitGrabPx)
	if got := s.seamAt(image.Pt(at.X+grab+2, at.Y)); got != -1 {
		t.Errorf("press %d px right of the seam grabbed seam %d, want none", grab+2, got)
	}
	if got := s.seamAt(image.Pt(at.X+grab-1, at.Y)); got != 0 {
		t.Errorf("press inside the grab band = seam %d, want 0", got)
	}

	var shape ebiten.CursorShapeType
	defer SetSplitCursorForTest(func(c ebiten.CursorShapeType) { shape = c })()
	s.Step(at, false, false) // idle hover
	if shape != ebiten.CursorShapeEWResize {
		t.Errorf("row seam cursor = %v, want EW-resize", shape)
	}
}

// TestSplitPaneRowDragTradesWidths: dragging the seam right widens the
// pane before it by exactly the drag distance, shrinks the pane after it
// by the same, and leaves the third untouched; a hard drag stops at the
// pane floor and the row still tiles.
func TestSplitPaneRowDragTradesWidths(t *testing.T) {
	s, ui := newPumpedRow(t, 3)
	before := rowWidths(s)
	at := rowSeamMid(s, 0)

	s.Step(at, true, true)
	s.Step(image.Pt(at.X+30, at.Y), false, true)
	s.Step(image.Pt(at.X+30, at.Y), false, false)
	uitest.PumpWindow(t, ui, nil, 640, 480)

	after := rowWidths(s)
	if after[0] != before[0]+30 {
		t.Errorf("leading pane = %d, want %d (grew by the 30 px drag)", after[0], before[0]+30)
	}
	if after[1] != before[1]-30 {
		t.Errorf("trailing pane = %d, want %d (shrank by the same 30 px)", after[1], before[1]-30)
	}
	if after[2] != before[2] {
		t.Errorf("third pane = %d, want %d (a seam moves only its pair)", after[2], before[2])
	}
	if got, want := s.Ratio(0), float64(after[0])/float64(before[0]+before[1]+before[2]); got != want {
		t.Errorf("seam ratio = %v, want %v (the fraction left of it)", got, want)
	}

	// Hard drag left: the leading pane bottoms out at the floor.
	at = rowSeamMid(s, 0)
	s.Step(at, true, true)
	s.Step(image.Pt(at.X-10000, at.Y), false, true)
	s.Step(image.Pt(at.X-10000, at.Y), false, false)
	uitest.PumpWindow(t, ui, nil, 640, 480)
	w := rowWidths(s)
	if w[0] != s.minPx {
		t.Errorf("leading pane after the hard drag = %d, want the floor %d", w[0], s.minPx)
	}
	if total := w[0] + w[1] + w[2] + s.seamPx*len(s.seams); total != s.root.GetWidget().Rect.Dx() {
		t.Errorf("panes + seams = %d, want the split's %d", total, s.root.GetWidget().Rect.Dx())
	}
}

// TestSplitPaneDefaultRatio: a panel whose opening arrangement is not the
// equal share sets its own default; Reset returns to it, not to 1/n.
func TestSplitPaneDefaultRatio(t *testing.T) {
	s, _ := newPumpedRow(t, 2)
	s.SetDefaultRatio(0, 0.6)
	if got := s.Ratio(0); got != 0.6 {
		t.Errorf("default ratio = %v, want the 0.6 default applied", got)
	}
	s.SetRatio(0, 0.3)
	s.Reset()
	if got := s.Ratio(0); got != 0.6 {
		t.Errorf("after Reset ratio = %v, want the 0.6 default", got)
	}
}
