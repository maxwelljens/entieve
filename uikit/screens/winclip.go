package screens

import (
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
)

// Window frame clip (CONTEXT.md: window resize chrome).
//
// ebitenui lays a container's children out at their preferred sizes and
// never clips a widget tree: a body shrunk below its content — or content
// that grew after the last resize — paints rows, text and borders over
// whatever sits under the window, "the contents spill from beyond the
// frame". The resize clamp (winchrome.go's windowMin, which raises a grip
// drag's floor to the body's content minimum) is the primary guard; this
// wrapper is the belt-and-braces layer that guarantees no body pixel
// escapes the frame even when a layout overflows.
//
// It renders the body into screen.SubImage(bodyRect) instead of screen.
// That is a clipped draw, not the full-screen offscreen composite
// ebitenui's ScrollContainer needs for its mask: ebiten's driver clips a
// draw to the destination sub-image's bounds while keeping the parent's
// absolute coordinates (SubImage.adjustPosition subtracts the ORIGINAL
// image's origin, never the sub-rect's), and it reuses one cached
// sub-image per rect. Layout, hit-testing and input layers are untouched;
// the per-frame cost is one cached sub-image lookup and a clipped draw.
type winClip struct{ *widget.Container }

// Render draws the wrapped body clipped to the wrapper's rect.
func (c *winClip) Render(screen *ebiten.Image) {
	if r := c.GetWidget().Rect; !r.Empty() {
		if sub, ok := screen.SubImage(r).(*ebiten.Image); ok {
			screen = sub
		}
	}
	c.Container.Render(screen)
}

// clipWindowBody wraps w's body in a winClip unless the body carries a
// widget that renders through a screen-sized MaskedRenderBuffer. A
// ScrollContainer, TextInput, TextArea or List draws its content into a
// buffer sized to the destination, so a sub-image destination misplaces
// or drops it — a body holding one of those renders unwrapped; the
// scroll viewports clip their own content, and a flat body with a field
// (the order dialog) fits its frame by MinSize. Idempotent: a raise
// re-mounts through AddWindow and finds the wrapper already there.
func clipWindowBody(w *widget.Window) {
	if w == nil || w.Contents == nil {
		return
	}
	if _, done := w.Contents.(*winClip); done {
		return
	}
	if hasMaskedBuffer(w.Contents) {
		return
	}
	body := w.Contents
	c := w.GetContainer()
	wrap := &winClip{Container: widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
		)),
	)}
	// Re-home the body: RemoveChild clears its parent (and the window
	// container's grid keeps its rows — [TitleBar, Contents] — because the
	// wrapper is appended after the removal).
	c.RemoveChild(body)
	wrap.AddChild(body)
	c.AddChild(wrap)
	w.Contents = wrap
}

// hasScrollContainer reports whether n's subtree renders through a scroll
// viewport: the one widget family that clips itself (and whose preferred
// size is its content's, not a frame minimum).
func hasScrollContainer(n widget.PreferredSizeLocateableWidget) bool {
	switch n.(type) {
	case *widget.ScrollContainer, *widget.TextArea:
		return true
	}
	if c, ok := n.(widget.Containerer); ok {
		for _, ch := range c.Children() {
			if hasScrollContainer(ch) {
				return true
			}
		}
	}
	return false
}

// hasMaskedBuffer reports whether n's subtree renders through ebitenui's
// MaskedRenderBuffer, which sizes its scratch images to the draw
// destination: TextInput and the scroll viewport family. TextArea and
// List each own a ScrollContainer internally (as a child of an outer
// container, or as their very widget), so the plain child walk cannot see
// through them — they are matched by type.
func hasMaskedBuffer(n widget.PreferredSizeLocateableWidget) bool {
	switch n.(type) {
	case *widget.TextInput, *widget.ScrollContainer, *widget.TextArea, *widget.List:
		return true
	}
	if c, ok := n.(widget.Containerer); ok {
		for _, ch := range c.Children() {
			if hasMaskedBuffer(ch) {
				return true
			}
		}
	}
	return false
}
