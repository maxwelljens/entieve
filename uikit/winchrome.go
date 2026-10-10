package uikit

import (
	"image"

	"github.com/ebitenui/ebitenui/event"
	"github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
)

// Window chrome: edge-grip resize for every window uikit builds.
//
// ebitenui's widget.WindowOpts.Resizeable() only extends the container
// rect's Max corner, so it resizes from the bottom/right edge alone.
// makeEdgeResizable replaces it with an N/E/S/W grip (corners use both
// axes) driven from the window's own title-bar/contents press events, so
// the opposite edge always stays fixed and MinSize is honored. Title-bar
// dragging keeps the built-in Draggable() handling; where a grip press
// lands inside the title bar both run, and the chrome's absolute
// recompute (step) wins every frame.

// edgeGripPx is the grip band width per edge; a corner engages both axes.
const edgeGripPx = 8

// edgeMask is a bit set of window edges.
type edgeMask uint8

const (
	edgeLeft edgeMask = 1 << iota
	edgeRight
	edgeTop
	edgeBottom
)

// edgesAt classifies which edges of rect grip p (within edgeGripPx of an
// edge, counted from the last pixel row/column so both bands are exactly
// grip wide). A point outside rect grips nothing; corners set both axes.
// Pure — unit-tested by TestEdgesAtGripBands.
func edgesAt(p image.Point, rect image.Rectangle, grip int) edgeMask {
	if !p.In(rect) {
		return 0
	}
	var m edgeMask
	if p.X-rect.Min.X < grip {
		m |= edgeLeft
	}
	if rect.Max.X-1-p.X < grip {
		m |= edgeRight
	}
	if p.Y-rect.Min.Y < grip {
		m |= edgeTop
	}
	if rect.Max.Y-1-p.Y < grip {
		m |= edgeBottom
	}
	return m
}

// resizeRect computes the window rect after a pointer drag of (dx,dy)
// from start on the given edges. The dragged edge follows the cursor,
// the opposite edge never moves, and the size never drops below minSize
// (floored at 1 px so the rect cannot invert even with no MinSize).
// MaxSize is not modeled: no chromed window sets one. Pure —
// unit-tested by TestResizeRectEdgeMath.
func resizeRect(start image.Rectangle, dx, dy int, edges edgeMask, minSize image.Point) image.Rectangle {
	r := start
	minW, minH := max(minSize.X, 1), max(minSize.Y, 1)
	if edges&edgeLeft != 0 {
		// Left edge moves; Max.X is the anchor (clamp, never overshoot).
		r.Min.X = min(start.Min.X+dx, start.Max.X-minW)
	}
	if edges&edgeRight != 0 {
		r.Max.X = max(start.Max.X+dx, start.Min.X+minW)
	}
	if edges&edgeTop != 0 {
		r.Min.Y = min(start.Min.Y+dy, start.Max.Y-minH)
	}
	if edges&edgeBottom != 0 {
		r.Max.Y = max(start.Max.Y+dy, start.Min.Y+minH)
	}
	return r
}

// winChrome is one window's edge-resize state: attached by
// makeEdgeResizable, pumped every frame by Kit.stepChromes.
type winChrome struct {
	win    *widget.Window
	active bool            // a grip drag is in flight
	edges  edgeMask        // gripped edges
	start  image.Rectangle // window rect at press
	origin image.Point     // cursor at press
}

// chromeByWindow indexes chrome by window so makeEdgeResizable is
// idempotent and Kit.AddWindow can pick a mounted window's chrome up.
// NewKit clears it: one kit owns the live windows, so discarded kits'
// windows do not pile up across save/load rebuilds.
var chromeByWindow = map[*widget.Window]*winChrome{}

// makeEdgeResizable gives win four-edge plus corner grip resizing
// (edgeGripPx bands, MinSize honored, ResizeEvent fired on release).
// Call it once per window after widget.NewWindow; title-bar dragging
// stays with the built-in Draggable() handling this coexists with.
func makeEdgeResizable(win *widget.Window) *winChrome {
	if ch, ok := chromeByWindow[win]; ok {
		return ch
	}
	ch := &winChrome{win: win}
	// Press points come from the event args (widget-relative), not the
	// global cursor, so the seam stays testable without ebiten input.
	// ebitenui only delivers these to the top-most input layer, which
	// is what keeps a covered window from starting its own resize.
	press := func(a *widget.WidgetMouseButtonPressedEventArgs) {
		if a.Button != ebiten.MouseButtonLeft {
			return
		}
		wgt := win.GetContainer().GetWidget()
		if !wgt.IsVisible() {
			return
		}
		p := image.Point{
			X: a.Widget.Rect.Min.X + a.OffsetX,
			Y: a.Widget.Rect.Min.Y + a.OffsetY,
		}
		e := edgesAt(p, wgt.Rect, edgeGripPx)
		if e == 0 {
			return // not a grip press: the title-bar drag owns it
		}
		ch.begin(wgt.Rect, p, e)
	}
	if tb := win.TitleBar; tb != nil {
		tb.GetWidget().MouseButtonPressedEvent.AddHandler(event.WrapHandler(press))
	}
	win.Contents.GetWidget().MouseButtonPressedEvent.AddHandler(event.WrapHandler(press))
	chromeByWindow[win] = ch
	return ch
}

// begin starts a grip drag from the given window rect and cursor point.
func (c *winChrome) begin(rect image.Rectangle, cursor image.Point, e edgeMask) {
	c.active, c.edges, c.start, c.origin = true, e, rect, cursor
}

// step pumps one frame: an active grip drag applies its rect, fires
// ResizeEvent on release, and — when idle — advertises the resize cursor
// over an idle grip. The rect is recomputed in absolute terms from the
// press every step, so the built-in title-bar drag (which also runs for
// presses inside the title bar's 44 px) cannot double-move the window:
// its Render-time offset is wiped on the next step. cursor/pressed are
// parameters so tests drive the math without ebiten input.
func (c *winChrome) step(cursor image.Point, pressed bool) {
	wgt := c.win.GetContainer().GetWidget()
	if c.active {
		r := resizeRect(c.start, cursor.X-c.origin.X, cursor.Y-c.origin.Y,
			c.edges, windowMin(c.win))
		if r != wgt.Rect {
			c.win.SetLocation(r)
		}
		if !pressed {
			c.active = false
			c.win.ResizeEvent.Fire(&widget.WindowChangedEventArgs{
				Window: c.win,
				Rect:   wgt.Rect,
			})
		}
		setResizeCursor(c.edges)
		return
	}
	if pressed || !wgt.IsVisible() {
		return
	}
	// Idle hover: the same layer test ebitenui's own resize cursor used,
	// so a grip covered by another window does not advertise resizing.
	// A never-mounted window has no layer of its own — EffectiveInputLayer
	// falls back to DefaultLayer, which passes anywhere, so reject that
	// explicitly (its stale rect would fake grips across the screen).
	layer := wgt.EffectiveInputLayer()
	if layer == &input.DefaultLayer || !layer.ActiveFor(cursor.X, cursor.Y, input.LayerEventTypeAll) {
		return
	}
	if e := edgesAt(cursor, wgt.Rect, edgeGripPx); e != 0 {
		setResizeCursor(e)
	}
}

// windowMin is the window's MinSize as a point (no MinSize → 1×1 floor
// lives inside resizeRect).
func windowMin(win *widget.Window) image.Point {
	if win.MinSize != nil {
		return *win.MinSize
	}
	return image.Point{}
}

// setResizeCursor picks the cursor for the gripped axes. ebitenui ships
// only the EW/NS resize cursors (no diagonals), so any horizontal edge —
// corners included — uses EW.
func setResizeCursor(e edgeMask) {
	if e&(edgeLeft|edgeRight) != 0 {
		input.SetCursorShape(input.CURSOR_EWRESIZE)
		return
	}
	input.SetCursorShape(input.CURSOR_NSRESIZE)
}

// stepChromes pumps every window chrome registered with this kit. The
// Kit calls it right after ui.Update (press events just fired, so the
// new rect applies to this frame's render) and again after ui.Draw (the
// built-in title drag moves the rect during Render; the recompute wipes
// that drift before the next frame's hit tests).
func (k *Kit) stepChromes() {
	cursor, _ := input.CursorPosition()
	pressed := input.MouseButtonPressed(ebiten.MouseButtonLeft)
	pt := image.Point{X: cursor}
	for _, ch := range k.chromes {
		ch.step(pt, pressed)
	}
}
