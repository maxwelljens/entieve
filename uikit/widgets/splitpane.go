package widgets

import (
	"image"
	"math"

	"github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// SplitPane is a vertical stack of panes with a draggable seam between
// each pair (CONTEXT.md "Split pane"): the panes divide the leftover
// height by ratio instead of GridLayout's equal shares. Each pane keeps a
// minimum height so nothing collapses, and the seams are named for the
// layout sidecar, so a dragged split survives a restart.
//
// A seam's ratio is the fraction of the stack ABOVE it (equal shares put
// seam i at i/n; 0.5 for a two-pane split, the default). Dragging a seam
// trades height between the two panes it bounds and leaves the rest
// untouched. The seam itself is a hairline; its grab band is 8 px,
// classified from the cursor every frame like the window chrome's edge
// grips — a body widget that elevates its own input layer (a table's
// scroll viewport) never eats the press in the band.
//
// Wiring: the screen builds the pane containers, hands them to
// NewSplitPane, places Container() in its grid and calls Poll once per
// frame from Sync — the pane layouts and their ScrollRails are untouched.
type SplitPane struct {
	root   *widget.Container
	panes  []widget.PreferredSizeLocateableWidget
	seams  []*widget.Container
	shares []float64 // pane i's share of the stack (sum 1; 1/n each at rest)
	minPx  int       // per-pane floor
	seamPx int       // hairline height

	seam       int // seam grabbed by the press (-1 = none)
	start      image.Point
	startUpper int // upper pane's height at the press, px
	pairPx     int // the grabbed pair's combined height at the press, px
	availPx    int // the stack's split height (without the seams) at the press
}

// SplitPane metrics in DESIGN units (theme.Px at every measure). The
// floor keeps a section label plus two rows readable; the seam is a
// hairline with 4 px of grab on either side (8 px total, the chrome's
// edge-grip width). Stored ratios outside SplitMinRatio..SplitMaxRatio
// are clamped: a pane that far collapsed is unusable, and the pixel floor
// then does the rest.
const (
	SplitMinPane  = 72
	SplitSeamH    = 1
	SplitGrabPx   = 4
	SplitMinRatio = 0.1
	SplitMaxRatio = 0.9
)

// Seam names one split boundary for the layout sidecar: Split's seam
// Index under Name (the value is the fraction of the stack above it).
type Seam struct {
	Name  string
	Split *SplitPane
	Index int
}

// NewSplitPane stacks the panes (one per hold/table section, each a
// container of its fixed label over its scroll body) with a draggable
// seam between neighbours.
func NewSplitPane(th theme.Theme, panes ...widget.PreferredSizeLocateableWidget) *SplitPane {
	s := &SplitPane{
		panes:  panes,
		shares: make([]float64, len(panes)),
		minPx:  theme.Px(SplitMinPane),
		seamPx: theme.Px(SplitSeamH),
		seam:   -1,
	}
	seamImg := theme.NineSlice(theme.NRGBA(th.Pal().Border, 200))
	if n := len(panes); n > 0 {
		for i := range s.shares {
			s.shares[i] = 1 / float64(n)
		}
	}
	s.root = widget.NewContainer(widget.ContainerOpts.Layout(s))
	for i, p := range panes {
		s.root.AddChild(p)
		if i < len(panes)-1 {
			seam := widget.NewContainer(widget.ContainerOpts.BackgroundImage(seamImg))
			s.seams = append(s.seams, seam)
			s.root.AddChild(seam)
		}
	}
	return s
}

// Container returns the wrapper to place where the pane stack goes.
func (s *SplitPane) Container() *widget.Container { return s.root }

// Ratio is seam i's ratio: the fraction of the stack above it, which is
// also what the layout sidecar stores.
func (s *SplitPane) Ratio(i int) float64 {
	if i < 0 || i >= len(s.shares)-1 {
		return 0.5
	}
	sum := 0.0
	for _, sh := range s.shares[:i+1] {
		sum += sh
	}
	return sum
}

// SetRatio restores seam i's ratio, clamped to the sane band (a stored
// sidecar value can be anything; NaN is ignored): the two panes the seam
// bounds trade height, so the rest of the stack keeps its shares.
// Equal shares stay the default — no SetRatio call, no change.
func (s *SplitPane) SetRatio(i int, f float64) {
	if i < 0 || i >= len(s.shares)-1 || math.IsNaN(f) {
		return
	}
	f = min(max(f, SplitMinRatio), SplitMaxRatio)
	prefix := 0.0
	for _, sh := range s.shares[:i] {
		prefix += sh
	}
	pair := s.shares[i] + s.shares[i+1]
	upper := min(max(f-prefix, 0), pair)
	if upper == s.shares[i] {
		return
	}
	s.shares[i], s.shares[i+1] = upper, pair-upper
	s.root.RequestRelayout()
}

// Reset restores the equal-share default ("Reset window layout").
func (s *SplitPane) Reset() {
	n := len(s.shares)
	if n == 0 {
		return
	}
	for i := range s.shares {
		s.shares[i] = 1 / float64(n)
	}
	s.root.RequestRelayout()
}

// Poll advances one frame of seam dragging from the live cursor and the
// left button. The screen calls it from Sync, before Kit.Update.
func (s *SplitPane) Poll() {
	x, y := PollCursor()
	s.Step(image.Pt(x, y), PressEdge(ebiten.MouseButtonLeft), splitHeld())
}

// Step is Poll's testable core: a rising press in a seam's grab band
// grabs it, a held drag moves the seam with the cursor (its two panes
// trade the drag distance), and a release drops the grab.
func (s *SplitPane) Step(cursor image.Point, rising, pressed bool) {
	if s.seam >= 0 {
		if !pressed {
			s.seam = -1
			return
		}
		s.drag(cursor)
		setSplitCursor()
		return
	}
	if rising {
		i := s.seamAt(cursor)
		if i < 0 {
			return
		}
		s.startUpper = s.panes[i].GetWidget().Rect.Dy()
		s.pairPx = s.startUpper + s.panes[i+1].GetWidget().Rect.Dy()
		s.availPx = s.root.GetWidget().Rect.Dy() - s.seamPx*len(s.seams)
		if s.pairPx <= 0 || s.availPx <= 0 {
			return
		}
		s.seam, s.start = i, cursor
		setSplitCursor()
		return
	}
	if s.seamAt(cursor) >= 0 {
		setSplitCursor()
	}
}

// drag moves the grabbed seam: the upper pane's height follows the cursor
// delta, held inside the pane floor, and becomes the seam's new ratio. The
// first movement re-derives the shares from the panes' laid-out heights,
// so the drag trades exact pixels (the shares a resize rounded away must
// not leak a drift into the two panes).
func (s *SplitPane) drag(cursor image.Point) {
	dy := cursor.Y - s.start.Y
	if dy == 0 || s.pairPx <= 0 || s.availPx <= 0 {
		return
	}
	for i, p := range s.panes {
		s.shares[i] = float64(p.GetWidget().Rect.Dy()) / float64(s.availPx)
	}
	upper := float64(s.startUpper - dy)
	if s.pairPx >= 2*s.minPx {
		upper = min(max(upper, float64(s.minPx)), float64(s.pairPx-s.minPx))
	} else {
		upper = float64(s.minPx)
	}
	prefix := 0.0
	for _, sh := range s.shares[:s.seam] {
		prefix += sh
	}
	s.SetRatio(s.seam, prefix+upper/float64(s.availPx))
}

// seamAt is the seam whose grab band holds p on the topmost window, or
// -1: the chrome's gripStart idiom — the point must sit on this pane
// stack's own input layer, so a seam under another window is inert.
func (s *SplitPane) seamAt(p image.Point) int {
	root := s.root.GetWidget()
	if !root.IsVisible() {
		return -1
	}
	if l := root.EffectiveInputLayer(); l == &input.DefaultLayer ||
		!l.ActiveFor(p.X, p.Y, input.LayerEventTypeWheel) {
		return -1
	}
	grab := theme.Px(SplitGrabPx)
	for i, seam := range s.seams {
		r := seam.GetWidget().Rect
		if p.X >= r.Min.X && p.X < r.Max.X &&
			p.Y >= r.Min.Y-grab && p.Y < r.Max.Y+grab {
			return i
		}
	}
	return -1
}

// Layout implements widget.Layouter over the stack's own children
// (pane, seam, pane, …): the panes take heights() and the seams the
// hairline between them.
func (s *SplitPane) Layout(_ []widget.PreferredSizeLocateableWidget, rect image.Rectangle) {
	heights := s.heights(rect.Dy())
	y := rect.Min.Y
	for i, h := range heights {
		s.panes[i].SetLocation(image.Rect(rect.Min.X, y, rect.Max.X, y+h))
		y += h
		if i < len(s.seams) {
			s.seams[i].SetLocation(image.Rect(rect.Min.X, y, rect.Max.X, y+s.seamPx))
			y += s.seamPx
		}
	}
}

// PreferredSize implements widget.Layouter: the widest pane, the panes'
// preferred heights plus the seam hairlines.
func (s *SplitPane) PreferredSize(_ []widget.PreferredSizeLocateableWidget) (int, int) {
	w, h := 0, s.seamPx*len(s.seams)
	for _, p := range s.panes {
		pw, ph := p.PreferredSize()
		w, h = max(w, pw), h+ph
	}
	return w, h
}

// heights splits avail px across the panes: the shares scaled to the
// rect, floored at minPx, with the running budget leaving every later
// pane its floor. Too little room for every floor (a tiny window, a
// just-mounted split) falls back to equal shares, so the panes always
// tile the rect exactly.
func (s *SplitPane) heights(avail int) []int {
	n := len(s.panes)
	out := make([]int, n)
	avail -= s.seamPx * (n - 1)
	if n == 0 {
		return out
	}
	if avail < n*s.minPx {
		avail = max(avail, 0)
		for i := range out {
			out[i] = avail / n
		}
		out[n-1] += avail % n
		return out
	}
	sum := 0.0
	for _, sh := range s.shares {
		sum += sh
	}
	budget := avail
	for i := 0; i < n-1; i++ {
		h := int(math.Round(s.shares[i] / sum * float64(avail)))
		ceil := budget - (n-1-i)*s.minPx
		out[i] = min(max(h, s.minPx), max(ceil, s.minPx))
		budget -= out[i]
	}
	out[n-1] = budget
	return out
}

// splitHeld reads the left button state for the drag. The indirection is
// the test seam: headless tests cannot press a real button (the
// pressEdge/pollCursor precedent).
var splitHeld = func() bool { return ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) }

// SetSplitHeldForTest overrides the seam drag's button read for a
// headless test and returns a restore func. Test-only seam: never call
// it from production code.
func SetSplitHeldForTest(f func() bool) func() {
	prev := splitHeld
	splitHeld = f
	return func() { splitHeld = prev }
}

// setSplitCursor is the seam's cursor write: ebiten's own call (the
// window chrome's setCursorShape idiom — ebitenui's name cache resets the
// OS cursor every frame, so the hover rewrites it every frame too).
var setSplitCursor = func() { ebiten.SetCursorShape(ebiten.CursorShapeNSResize) }

// SetSplitCursorForTest overrides the seam cursor write for a headless
// test and returns a restore func. Test-only seam: never call it from
// production code.
func SetSplitCursorForTest(f func()) func() {
	prev := setSplitCursor
	setSplitCursor = f
	return func() { setSplitCursor = prev }
}
