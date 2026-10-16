package uikit

import (
	"image"

	"github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
)

// Window chrome: edge-grip resize for every window a Kit mounts.
//
// ebitenui's windowOpts.Resizeable() only extends the container rect's
// Max corner, so it resizes from the bottom/right edge alone. The grip
// replaces it with an N/E/S/W grip (corners use both axes) so the
// opposite edge always stays fixed and MinSize is honored. The kit's
// AddWindow attaches one chrome per mounted window (Kit.chromes) and
// detaches it again through the remove func it returns, so only mounted
// windows are ever pumped.
//
// The grip is classified from the WINDOW RECT in the chrome's per-frame
// step, never from widget press events: a body widget that fills the
// window (the overview's table, the log's text area) elevates its own
// input layer over the 8 px grip band, so press events routed to
// TitleBar/Contents never fire there and those windows could not be
// resized from their own edges. step only needs the cursor and the button
// state, which is also what makes the drag testable without ebiten input.
// Title-bar dragging keeps the built-in Draggable() handling; while a grip
// drag is active the chrome's absolute recompute wins every frame. The
// grip never swallows the press: a click without a drag is a no-op here,
// so clicking a table row inside the band still selects the row.

// edgeGripPx is the grip band width per edge in DESIGN units (scaled by
// Px where the band is measured — the cursor runs in the same Layout
// units every other metric is scaled into, so grips stay consistent);
// a corner engages both axes.
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

// winChrome is one window's edge-resize state: attached by Kit.AddWindow,
// pumped every frame by Kit.stepChromes while that window stays mounted.
type winChrome struct {
	win    *widget.Window
	held   bool            // left button state at the previous step
	armed  bool            // press started in a grip band, waiting for a drag
	active bool            // a grip drag is in flight
	edges  edgeMask        // gripped edges
	start  image.Rectangle // window rect at press
	origin image.Point     // cursor at press
}

// cancel drops any grip state without touching the rect: the window is
// leaving the pump (unmounted, or hidden this frame) and must leave no
// drag or cursor side effects behind.
func (c *winChrome) cancel() {
	c.armed, c.active = false, false
}

// gripStart reports the edges gripped when a press lands on p: the point
// must sit in this window's own grip band AND on top of every other input
// layer, so only the topmost window under the cursor ever starts a resize.
// The layer test asks for LayerEventTypeWheel on purpose: a body's scroll
// viewport elevates a layer that claims every event EXCEPT wheel
// (ebitenui's ScrollContainer), so a mouse-eligible test would report
// "covered" for a window's own body — exactly the defect this replaced —
// while another window's layer (which claims wheel too) still blocks.
func (c *winChrome) gripStart(p image.Point) edgeMask {
	wgt := c.win.GetContainer().GetWidget()
	if !wgt.IsVisible() {
		return 0
	}
	e := edgesAt(p, wgt.Rect, Px(edgeGripPx))
	if e == 0 {
		return 0
	}
	// A never-mounted window has no layer of its own — EffectiveInputLayer
	// falls back to DefaultLayer, which passes anywhere, so reject that
	// explicitly (its stale rect would fake grips across the screen).
	layer := wgt.EffectiveInputLayer()
	if layer == &input.DefaultLayer || !layer.ActiveFor(p.X, p.Y, input.LayerEventTypeWheel) {
		return 0
	}
	return e
}

// step pumps one frame from the cursor and the left-button state.
//
// A press that begins in a grip band arms the drag; it becomes active as
// soon as the cursor moves, and the rect is then recomputed in absolute
// terms from the press every step — so the built-in title-bar drag (which
// also runs for presses inside the title bar's 44 px) cannot double-move
// the window: its Render-time offset is wiped on the next step. Releasing
// without ever moving is a click, not a drag: nothing resizes and no
// ResizeEvent fires (the child widget keeps the press). Releasing after a
// drag fires ResizeEvent once with the final rect. cursor/pressed are
// parameters so tests drive the math without ebiten input.
//
// A hidden window is never stepped into: its grip state drops instead of
// applying rects (the idle branch already refused the cursor; an active
// drag used to keep moving the invisible rect, which then read back as a
// player-moved window).
func (c *winChrome) step(cursor image.Point, pressed bool) {
	wgt := c.win.GetContainer().GetWidget()
	rising := pressed && !c.held
	c.held = pressed

	if !wgt.IsVisible() {
		c.cancel()
		return
	}

	switch {
	case c.active:
		if !pressed {
			c.active = false
			c.win.ResizeEvent.Fire(&widget.WindowChangedEventArgs{
				Window: c.win,
				Rect:   wgt.Rect,
			})
			return
		}
		c.apply(cursor, wgt)
		return

	case c.armed:
		if !pressed {
			c.armed = false // click without drag: no-op, child keeps it
			return
		}
		if cursor != c.origin {
			c.armed, c.active = false, true
			c.apply(cursor, wgt)
		}
		return

	case rising:
		if e := c.gripStart(cursor); e != 0 {
			c.armed, c.edges, c.start, c.origin = true, e, wgt.Rect, cursor
		}
		return
	}

	if pressed {
		return
	}
	// Idle hover: advertise the resize cursor over an idle grip, using the
	// same topmost-layer test as gripStart, so a grip covered by another
	// window does not advertise resizing.
	if e := c.gripStart(cursor); e != 0 {
		setResizeCursor(e)
	}
}

// apply moves the window to the rect the drag math asks for.
func (c *winChrome) apply(cursor image.Point, wgt *widget.Widget) {
	r := resizeRect(c.start, cursor.X-c.origin.X, cursor.Y-c.origin.Y,
		c.edges, windowMin(c.win))
	if r != wgt.Rect {
		c.win.SetLocation(r)
	}
	setResizeCursor(c.edges)
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

// stepChromes pumps the chrome of every window currently mounted on this
// kit (Kit.chromes — an unmounted window's entry is gone, so it is never
// stepped). The Kit calls it right after ui.Update (press events just
// fired, so the new rect applies to this frame's render) and again after
// ui.Draw (the built-in title drag moves the rect during Render; the
// recompute wipes that drift before the next frame's hit tests).
func (k *Kit) stepChromes() {
	cursor := image.Point{}
	cursor.X, cursor.Y = input.CursorPosition()
	pressed := input.MouseButtonPressed(ebiten.MouseButtonLeft)
	for _, ch := range k.chromes {
		ch.step(cursor, pressed)
	}
}
