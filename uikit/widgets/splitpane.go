package widgets

import (
	"image"
	"image/color"
	"math"

	eimage "github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// SplitPane is a stack of panes with a draggable seam between each pair
// (CONTEXT.md "Split pane"): the panes divide the leftover height — or,
// for a row, the width — by ratio instead of GridLayout's equal shares.
// Each pane keeps a minimum extent so nothing collapses, and the seams are
// named for the layout sidecar, so a dragged split survives a restart.
//
// A seam's ratio is the fraction of the stack BEFORE it (equal shares put
// seam i at i/n; 0.5 for a two-pane split, the default): the fraction
// above it in a vertical stack, the fraction left of it in a row.
// Dragging a seam trades height (or width) between the two panes it
// bounds and leaves the rest untouched. The seam is a recessed track with
// a centred sash grip (three dots, dim at rest and lit on hover/drag); its
// grab band is 8 px, classified from the cursor every frame like the
// window chrome's edge grips — a body widget that elevates its own input
// layer (a table's scroll viewport) never eats the press in the band. The
// cursor is NS-resize for a stack, EW-resize for a row.
//
// Wiring: the screen builds the pane containers, hands them to
// NewSplitPane (vertical) or NewSplitPaneRow (horizontal), places
// Container() in its grid and calls Poll once per frame from Sync — the
// pane layouts and their ScrollRails are untouched.
type SplitPane struct {
	root   *widget.Container
	panes  []widget.PreferredSizeLocateableWidget
	seams  []*widget.Container
	grips  []*widget.Container // the sash handle inside each seam
	shares []float64           // pane i's share of the stack (sum 1; 1/n each at rest)
	def    []float64           // shares Reset restores (equal unless SetDefaultRatio)
	minPx  int                 // per-pane floor
	seamPx int                 // track thickness
	row    bool                // panes side by side (width split) instead of stacked

	gripDim, gripLit *eimage.NineSlice // the grip's two states (shared by seams)
	lit              int               // seam whose grip is bright (-1 = none)

	seam       int // seam grabbed by the press (-1 = none)
	start      image.Point
	startUpper int // leading pane's extent at the press, px
	pairPx     int // the grabbed pair's combined extent at the press, px
	availPx    int // the stack's split extent (without the seams) at the press
}

// SplitPane metrics in DESIGN units (theme.Px at every measure). The
// floor keeps a section label plus two rows readable; the seam is a 4 px
// recessed track carrying a dotted grip (2 px dots, 2 px apart) with the
// 8 px grab band centred on it (the chrome's edge-grip width; the track
// itself counts, so the margin is 2 px a side). Stored ratios outside
// SplitMinRatio..SplitMaxRatio are clamped: a pane that far collapsed is
// unusable, and the pixel floor then does the rest.
const (
	SplitMinPane  = 72
	SplitSeamH    = 4
	SplitGripDot  = 2
	SplitGripGap  = 2
	SplitGrabPx   = 4
	SplitMinRatio = 0.1
	SplitMaxRatio = 0.9
)

// Seam names one split boundary for the layout sidecar: Split's seam
// Index under Name (the value is the fraction of the stack before it).
type Seam struct {
	Name  string
	Split *SplitPane
	Index int
}

// NewSplitPane stacks the panes (one per hold/table section, each a
// container of its fixed label over its scroll body) with a draggable
// seam between neighbours.
func NewSplitPane(th theme.Theme, panes ...widget.PreferredSizeLocateableWidget) *SplitPane {
	return newSplitPane(th, false, panes)
}

// NewSplitPaneRow places the panes side by side with a draggable vertical
// seam between neighbours: the same pane floors, grab band and sidecar
// ratios as NewSplitPane, measured across the width.
func NewSplitPaneRow(th theme.Theme, panes ...widget.PreferredSizeLocateableWidget) *SplitPane {
	return newSplitPane(th, true, panes)
}

func newSplitPane(th theme.Theme, row bool, panes []widget.PreferredSizeLocateableWidget) *SplitPane {
	pal := th.Pal()
	s := &SplitPane{
		panes:   panes,
		shares:  make([]float64, len(panes)),
		minPx:   theme.Px(SplitMinPane),
		seamPx:  theme.Px(SplitSeamH),
		row:     row,
		seam:    -1,
		lit:     -1,
		gripDim: splitGrip(pal.Head, row),
		gripLit: splitGrip(pal.Text, row),
	}
	// Recessed track: a Border line around the near-black BGHard fill, so
	// the division reads on any surface (the body is the same near-black
	// at window translucency).
	trackImg := theme.Bordered(theme.Solid(pal.BGHard), theme.NRGBA(pal.Border, 200), 1)
	if n := len(panes); n > 0 {
		for i := range s.shares {
			s.shares[i] = 1 / float64(n)
		}
	}
	s.def = append([]float64(nil), s.shares...)
	s.root = widget.NewContainer(widget.ContainerOpts.Layout(s))
	for i, p := range panes {
		s.root.AddChild(p)
		if i < len(panes)-1 {
			grip := widget.NewContainer(
				widget.ContainerOpts.BackgroundImage(s.gripDim),
				widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
					HorizontalPosition: widget.AnchorLayoutPositionCenter,
					VerticalPosition:   widget.AnchorLayoutPositionCenter,
				})),
			)
			seam := widget.NewContainer(
				widget.ContainerOpts.BackgroundImage(trackImg),
				widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
			)
			seam.AddChild(grip)
			s.grips = append(s.grips, grip)
			s.seams = append(s.seams, seam)
			s.root.AddChild(seam)
		}
	}
	return s
}

// splitGrip builds one sash handle: three dots along the seam's axis
// (across a stack's horizontal seam, down a row's vertical one), drawn at
// their own pixels and centred by the seam's anchor layout.
func splitGrip(c color.RGBA, row bool) *eimage.NineSlice {
	dot, gap := theme.Px(SplitGripDot), theme.Px(SplitGripGap)
	w, h := 3*dot+2*gap, dot
	if row {
		w, h = dot, 3*dot+2*gap
	}
	img := ebiten.NewImage(w, h)
	for i := 0; i < 3; i++ {
		at := i * (dot + gap)
		if row {
			theme.FillRect(img, image.Rect(0, at, dot, at+dot), c)
			continue
		}
		theme.FillRect(img, image.Rect(at, 0, at+dot, dot), c)
	}
	return eimage.NewFixedNineSlice(img)
}

// light brightens seam i's grip (or dims every grip for -1): the handle
// advertises the grab band under the pointer, and stays lit while dragged.
func (s *SplitPane) light(i int) {
	if i == s.lit {
		return
	}
	for n, grip := range s.grips {
		img := s.gripDim
		if n == i {
			img = s.gripLit
		}
		grip.SetBackgroundImage(img)
	}
	s.lit = i
}

// Container returns the wrapper to place where the pane stack goes.
func (s *SplitPane) Container() *widget.Container { return s.root }

// Ratio is seam i's ratio: the fraction of the stack before it (above in
// a stack, left of it in a row), which is also what the layout sidecar
// stores.
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

// setShare writes ratio f for seam i into sh, clamped to the sane band
// (a stored sidecar value can be anything; NaN is ignored): the two panes
// the seam bounds trade extent, so the rest of the stack keeps its shares.
func (s *SplitPane) setShare(sh []float64, i int, f float64) {
	if i < 0 || i >= len(sh)-1 || math.IsNaN(f) {
		return
	}
	f = min(max(f, SplitMinRatio), SplitMaxRatio)
	prefix := 0.0
	for _, v := range sh[:i] {
		prefix += v
	}
	pair := sh[i] + sh[i+1]
	upper := min(max(f-prefix, 0), pair)
	sh[i], sh[i+1] = upper, pair-upper
}

// SetRatio restores seam i's ratio, clamped to the sane band.
// Equal shares stay the default — no SetRatio call, no change.
func (s *SplitPane) SetRatio(i int, f float64) {
	if i < 0 || i >= len(s.shares)-1 {
		return
	}
	was := s.shares[i]
	s.setShare(s.shares, i, f)
	if s.shares[i] != was {
		s.root.RequestRelayout()
	}
}

// SetDefaultRatio sets the ratio Reset restores — and the split's opening
// arrangement — for a panel whose default is not the equal share (the
// market's book / My orders). The other panels never call it: equal
// shares are theirs.
func (s *SplitPane) SetDefaultRatio(i int, f float64) {
	if i < 0 || i >= len(s.def)-1 {
		return
	}
	s.setShare(s.def, i, f)
	s.shares[i], s.shares[i+1] = s.def[i], s.def[i+1]
	s.root.RequestRelayout()
}

// Reset restores the opening shares ("Reset window layout").
func (s *SplitPane) Reset() {
	copy(s.shares, s.def)
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
		} else {
			s.drag(cursor)
			s.setCursor()
			s.light(s.seam)
			return
		}
	}
	at := s.seamAt(cursor)
	s.light(at)
	if rising && at >= 0 {
		s.startUpper = s.axisLen(s.panes[at].GetWidget().Rect)
		s.pairPx = s.startUpper + s.axisLen(s.panes[at+1].GetWidget().Rect)
		s.availPx = s.axisLen(s.root.GetWidget().Rect) - s.seamPx*len(s.seams)
		if s.pairPx <= 0 || s.availPx <= 0 {
			return
		}
		s.seam, s.start = at, cursor
		s.setCursor()
		return
	}
	if at >= 0 {
		s.setCursor()
	}
}

// axisLen is a rect's extent along the split axis (height in a stack,
// width in a row).
func (s *SplitPane) axisLen(r image.Rectangle) int {
	if s.row {
		return r.Dx()
	}
	return r.Dy()
}

// drag moves the grabbed seam: the leading pane's extent follows the
// cursor delta, held inside the pane floor, and becomes the seam's new
// ratio. The first movement re-derives the shares from the panes'
// laid-out extents, so the drag trades exact pixels (the shares a resize
// rounded away must not leak a drift into the two panes).
func (s *SplitPane) drag(cursor image.Point) {
	// lead is the leading pane's growth: the seam follows the cursor, so
	// dragging down the stack (or right in a row) grows the pane before
	// it. Up/down was inverted once; the tests pin the cursor-aligned sign.
	lead := cursor.Y - s.start.Y
	if s.row {
		lead = cursor.X - s.start.X
	}
	if lead == 0 || s.pairPx <= 0 || s.availPx <= 0 {
		return
	}
	for i, p := range s.panes {
		s.shares[i] = float64(s.axisLen(p.GetWidget().Rect)) / float64(s.availPx)
	}
	upper := float64(s.startUpper + lead)
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
	// The grab band is 8 px total, centred on the track (the track counts
	// inside it), like the old hairline's 4 px on either side.
	grab := theme.Px(SplitGrabPx)
	pad := max(grab-s.seamPx/2, 0)
	for i, seam := range s.seams {
		r := seam.GetWidget().Rect
		if s.row {
			if p.Y >= r.Min.Y && p.Y < r.Max.Y &&
				p.X >= r.Min.X-pad && p.X < r.Max.X+pad {
				return i
			}
			continue
		}
		if p.X >= r.Min.X && p.X < r.Max.X &&
			p.Y >= r.Min.Y-pad && p.Y < r.Max.Y+pad {
			return i
		}
	}
	return -1
}

// Layout implements widget.Layouter over the stack's own children
// (pane, seam, pane, …): the panes take sizes() and the seams the
// track between them.
func (s *SplitPane) Layout(_ []widget.PreferredSizeLocateableWidget, rect image.Rectangle) {
	sizes := s.sizes(s.axisLen(rect))
	at := rect.Min
	for i, n := range sizes {
		if s.row {
			s.panes[i].SetLocation(image.Rect(at.X, rect.Min.Y, at.X+n, rect.Max.Y))
			at.X += n
			if i < len(s.seams) {
				s.seams[i].SetLocation(image.Rect(at.X, rect.Min.Y, at.X+s.seamPx, rect.Max.Y))
				at.X += s.seamPx
			}
			continue
		}
		s.panes[i].SetLocation(image.Rect(rect.Min.X, at.Y, rect.Max.X, at.Y+n))
		at.Y += n
		if i < len(s.seams) {
			s.seams[i].SetLocation(image.Rect(rect.Min.X, at.Y, rect.Max.X, at.Y+s.seamPx))
			at.Y += s.seamPx
		}
	}
}

// PreferredSize implements widget.Layouter: the panes' preferred extents
// along the axis plus the seam tracks, the largest across it.
func (s *SplitPane) PreferredSize(_ []widget.PreferredSizeLocateableWidget) (int, int) {
	w, h := 0, 0
	if s.row {
		w = s.seamPx * len(s.seams)
		for _, p := range s.panes {
			pw, ph := p.PreferredSize()
			w, h = w+pw, max(h, ph)
		}
		return w, h
	}
	h = s.seamPx * len(s.seams)
	for _, p := range s.panes {
		pw, ph := p.PreferredSize()
		w, h = max(w, pw), h+ph
	}
	return w, h
}

// sizes splits avail px across the panes: the shares scaled to the rect,
// floored at minPx, with the running budget leaving every later pane its
// floor. Too little room for every floor (a tiny window, a just-mounted
// split) falls back to equal shares, so the panes always tile the rect
// exactly.
func (s *SplitPane) sizes(avail int) []int {
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

// setCursor claims the seam's cursor: NS over a stack's horizontal seam,
// EW over a row's vertical one. It only declares the shape — the kit's
// single ApplyCursor writes it after the chrome pass, so a window grip
// still wins in its own band.
func (s *SplitPane) setCursor() {
	shape := ebiten.CursorShapeNSResize
	if s.row {
		shape = ebiten.CursorShapeEWResize
	}
	ClaimCursor(shape)
}
