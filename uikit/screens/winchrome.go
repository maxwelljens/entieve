package screens

import (
	"image"

	"github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// Window chrome: edge-grip resize AND title-bar dragging for every
// window a Kit mounts.
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
// The grip never swallows the press: a click without a drag is a no-op
// here, so clicking a table row inside the band still selects the row.
//
// Title-bar dragging is chrome-owned the same way (no WindowOpts
// Draggable()): ebitenui's built-in drag moved the rect through
// SetLocation from Render, whose RequestRelayout re-laid out the whole
// window subtree every frame of the drag — the cost this replaced. The
// chrome's title drag is translate-only: translate walks the widget tree
// rewriting rects in place (containers recurse, a TextArea re-lays its
// private interior, a ScrollContainer carries its stashed content so
// renderContent sees an equal rect and skips its own re-location), so no
// layout pass runs. It reports the release through Window.MoveEvent
// exactly as the built-in handler did — including a plain click on the
// title — and the grip keeps precedence where a press is in both bands
// (the corner), with both events firing on release.

// edgeGripPx is the grip band width per edge in DESIGN units (scaled by
// Px where the band is measured — the cursor runs in the same Layout
// units every other metric is scaled into, so grips stay consistent);
// a corner engages both axes.
const edgeGripPx = 8

// closeBtnW/closeBtnH are the title-bar close button's hit box in DESIGN
// units (scaled through Px everywhere it is measured): the same values
// the × button sets as its MinSize, so the chrome's exclusion strip and
// the widget's own rect agree.
const (
	closeBtnW = 46
	closeBtnH = 40
)

// closeButtonRect is the × strip at a title bar's right edge: full bar
// height, closeBtnW wide. moveStart excludes it for a closable window,
// so a press that closes never arms a drag — while the window's grip
// bands still classify first (edgesAt), which is what keeps a corner
// press a resize. Pure — unit-tested by TestCloseButtonStripExcludesDrag.
func closeButtonRect(title image.Rectangle) image.Rectangle {
	if title.Empty() {
		return image.Rectangle{}
	}
	return image.Rect(title.Max.X-theme.Px(closeBtnW), title.Min.Y, title.Max.X, title.Max.Y)
}

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

// winChrome is one window's drag state: attached by Kit.AddWindow,
// pumped every frame by Kit.stepChromes while that window stays mounted.
type winChrome struct {
	win        *widget.Window
	held       bool            // left button state at the previous step
	armed      bool            // press started in a grip band, waiting for a drag
	active     bool            // a grip drag is in flight
	edges      edgeMask        // gripped edges
	moveArmed  bool            // press started in the title bar, waiting for a drag
	moveActive bool            // a title drag is in flight
	start      image.Rectangle // window rect at press
	origin     image.Point     // cursor at press

	// closable marks the × strip at the title bar's right edge: a press
	// there never arms the title drag (the button owns that strip).
	// settle drops the window's open animation when a drag arms — the
	// fade must not relayout the subtree a drag is translating (nil for
	// a chrome a test builds on its own).
	closable bool
	settle   func()
}

// cancel drops any drag state without touching the rect: the window is
// leaving the pump (unmounted, or hidden this frame) and must leave no
// drag or cursor side effects behind.
func (c *winChrome) cancel() {
	c.armed, c.active = false, false
	c.moveArmed, c.moveActive = false, false
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
	e := edgesAt(p, wgt.Rect, theme.Px(edgeGripPx))
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

// moveStart reports whether a press on p starts a title-bar drag: the
// point must sit in this window's own title bar (the same rect ebitenui's
// built-in Draggable() armed from — a window without a TitleBar never
// drags) and on top of every other input layer, so only the topmost
// window under the cursor ever moves. The layer test asks for mouse
// BUTTON events here (the grip asks for wheel only — that is a body
// viewport property: a scroll layer claims every event except wheel and
// covers the side/bottom grip bands, while the title band is never
// covered by the window's own body, so the plain press is the right
// question for it).
func (c *winChrome) moveStart(p image.Point) bool {
	wgt := c.win.GetContainer().GetWidget()
	if !wgt.IsVisible() || c.win.TitleBar == nil {
		return false
	}
	if !p.In(c.win.TitleBar.GetWidget().Rect) {
		return false
	}
	// The × strip belongs to the close button: a press there never arms
	// the drag (the grip still classifies first in its own test, so a
	// corner press keeps resizing).
	if c.closable && p.In(closeButtonRect(c.win.TitleBar.GetWidget().Rect)) {
		return false
	}
	// A never-mounted window has no layer of its own — same reject as
	// gripStart (DefaultLayer would pass anywhere).
	layer := wgt.EffectiveInputLayer()
	if layer == &input.DefaultLayer || !layer.ActiveFor(p.X, p.Y, input.LayerEventTypeMouseButton) {
		return false
	}
	return true
}

// step pumps one frame from the cursor and the left-button state.
//
// A press that begins in a grip band arms the resize; one that begins in
// the title bar arms the move; a corner press arms both, and the grip
// keeps precedence while it is armed or active (the rect is recomputed
// in absolute terms from the press every step), so a corner drag resizes
// exactly like a plain grip drag — and still reports the move on
// release, because the press was in the title bar too. Each drag becomes
// active as soon as the cursor moves. Releasing without ever moving is a
// click, not a drag: the grip keeps its no-op contract (nothing resizes,
// no ResizeEvent, the child widget keeps the press) while the title drag
// still reports it — one MoveEvent with the unchanged rect, which is
// what the built-in handler fired on every title release. Releasing after
// a drag fires ResizeEvent (and MoveEvent, if the press was in the title
// bar) once with the final rect. cursor/pressed are parameters so tests
// drive the math without ebiten input.
//
// A hidden window is never stepped into: its drag state drops instead of
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
	case c.active: // grip drag in flight (grip precedence over the move)
		if !pressed {
			c.active = false
			// Move first, then resize: the built-in path queued
			// MoveEvent during ui.Update, ahead of the chrome's own
			// ResizeEvent, so handlers see them in the same order.
			c.fireMove(wgt)
			c.win.ResizeEvent.Fire(&widget.WindowChangedEventArgs{
				Window: c.win,
				Rect:   wgt.Rect,
			})
			return
		}
		c.apply(cursor, wgt)
		return

	case c.armed: // grip press waiting for the cursor to move
		if !pressed {
			c.armed = false // click without drag: no-op, child keeps it
			c.fireMove(wgt) // …unless it was also a title press
			return
		}
		if cursor != c.origin {
			c.armed, c.active = false, true
			c.apply(cursor, wgt)
		}
		return

	case c.moveActive: // title drag in flight
		if !pressed {
			c.fireMove(wgt)
			return
		}
		c.applyMove(cursor)
		return

	case c.moveArmed: // title press waiting for the cursor to move
		if !pressed {
			c.fireMove(wgt) // click on the title: report it, rect unchanged
			return
		}
		if cursor != c.origin {
			c.moveArmed, c.moveActive = false, true
			c.applyMove(cursor)
		}
		return

	case rising:
		e := c.gripStart(cursor)
		m := c.moveStart(cursor)
		if e != 0 || m {
			c.start, c.origin = wgt.Rect, cursor
			// Grabbing the window settles its open animation: the fade's
			// per-frame park flips would relayout the subtree the drag
			// translates (CONTEXT.md "Settle").
			if c.settle != nil {
				c.settle()
			}
		}
		c.armed, c.edges, c.moveArmed = e != 0, e, m
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

// fireMove reports a completed title press: exactly one MoveEvent when
// the press started in the title bar (drag or click — the built-in
// handler fired on any title release, carrying the rect as it stands),
// nothing otherwise. Clearing both move flags is part of the fire.
func (c *winChrome) fireMove(wgt *widget.Widget) {
	if !c.moveArmed && !c.moveActive {
		return
	}
	c.moveArmed, c.moveActive = false, false
	c.win.MoveEvent.Fire(&widget.WindowChangedEventArgs{
		Window: c.win,
		Rect:   wgt.Rect,
	})
}

// applyMove translates the window to start + press-to-cursor delta —
// absolute from the press, like the grip's recompute, so the step's
// second per-frame pass (after ui.Draw) derives the same target and is
// a no-op instead of double-moving.
func (c *winChrome) applyMove(cursor image.Point) {
	target := c.start.Add(cursor.Sub(c.origin))
	wgt := c.win.GetContainer().GetWidget()
	if target != wgt.Rect {
		translate(c.win, target.Min.Sub(wgt.Rect.Min))
	}
}

// translate moves win's whole widget subtree rigidly by d WITHOUT a
// relayout: Container.SetLocation would RequestRelayout the subtree —
// the per-frame cascade (grid → title/contents → table body rows) that
// made a window drag cost a full layout pass every frame. Rects are
// rewritten in place instead; see translateNode for the per-type rules.
func translate(win *widget.Window, d image.Point) {
	if d == (image.Point{}) {
		return
	}
	root := win.GetContainer()
	root.GetWidget().Rect = root.GetWidget().Rect.Add(d)
	for _, ch := range root.Children() {
		translateNode(ch, d)
	}
}

// translateNode is translate's per-child step: the rect moves first,
// then the type-specific follow-up. Containers recurse through their
// children; a TextArea's children are private (no Children()), so its
// tiny interior layout re-runs from the new rect (cheap: the width —
// hence the text measure — is unchanged); a ScrollContainer carries the
// content subtree stashed by widgets.AttachScrollContent, so
// renderContent derives a rect equal to the content's already-translated
// one and skips its own SetLocation + RequestRelayout of it. Everything
// else (Button, Slider, Text, Graphic, ProgressBar, …) is rect-only —
// which is all SetLocation did for them: Slider re-derives its handle
// from the rect at Render, Button re-syncs its inner container at Render.
func translateNode(n widget.PreferredSizeLocateableWidget, d image.Point) {
	n.GetWidget().Rect = n.GetWidget().Rect.Add(d)
	switch t := n.(type) {
	case widget.Containerer:
		for _, ch := range t.Children() {
			translateNode(ch, d)
		}
	case *widget.TextArea:
		t.RequestRelayout()
	case *widget.ScrollContainer:
		if content := widgets.ScrollContentOf(t); content != nil {
			translateNode(content, d)
		}
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

// stepChromes pumps the chrome of every window currently mounted on this
// kit (Kit.chromes — an unmounted window's entry is gone, so it is never
// stepped). The Kit calls it right after ui.Update (press events just
// fired, so the new rect applies to this frame's render) and again after
// ui.Draw (input may have changed while drawing; both drags recompute
// their rect absolutely from the press, so this pass applies the fresh
// cursor or derives an identical rect and does nothing).
func (k *Kit) stepChromes() {
	cursor := image.Point{}
	cursor.X, cursor.Y = input.CursorPosition()
	pressed := input.MouseButtonPressed(ebiten.MouseButtonLeft)
	for _, ch := range k.chromes {
		ch.step(cursor, pressed)
	}
}
