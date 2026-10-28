package widgets

import (
	"image"
	"testing"

	"github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
)

// scriptInput is a Source a test drives by hand: the OS never sees it.
type scriptInput struct {
	x, y   int
	down   [3]bool
	edge   [3]bool
	shape  ebiten.CursorShapeType
	writes int
}

func (s *scriptInput) Cursor() (int, int)        { return s.x, s.y }
func (s *scriptInput) Down(b Button) bool        { return s.down[b] }
func (s *scriptInput) JustPressed(b Button) bool { return s.edge[b] }
func (s *scriptInput) SetShape(sh ebiten.CursorShapeType) {
	s.shape, s.writes = sh, s.writes+1
}

// TestPointerSpendHidesThePressNotTheEdge pins the gesture vocabulary:
// Begin takes the frame's one sample (a later OS read changes nothing),
// Press is the edge unless the frame spent it, RawPress is always the
// edge, Down is the held state (not an edge), and the next Begin is
// live again.
func TestPointerSpendHidesThePressNotTheEdge(t *testing.T) {
	src := &scriptInput{x: 1, y: 2, edge: [3]bool{true, true, false}}
	p := NewPointer(src)
	p.Begin()
	if got := p.At(); got != image.Pt(1, 2) {
		t.Errorf("At() = %v, want the frame's one sample (1,2)", got)
	}
	if p.Down(Left) {
		t.Error("Down reports a held button; this frame has an edge, not a hold")
	}
	if !p.Press(Left) || !p.RawPress(Left) {
		t.Fatal("fixture: the frame's left edge is unreadable")
	}

	// A later OS read must not reach this frame: one point, one gesture.
	src.x, src.y, src.edge = 50, 60, [3]bool{}
	if p.At() != image.Pt(1, 2) || !p.Press(Left) {
		t.Error("the frame re-sampled the source: At/Press changed under it")
	}

	p.Spend(Left)
	if p.Press(Left) {
		t.Error("a spent press still reads as this frame's edge")
	}
	if !p.RawPress(Left) {
		t.Error("Spend took the raw edge too: the popup's own first read would see nothing")
	}
	if p.Press(Right) != true {
		t.Error("Spend(Left) spent another button")
	}

	// The next frame begins live: the OS still holds the left button (no
	// new edge, so Press is false while Down is true) and the right one
	// edges — the previous frame's Spend(Left) must not leak into it.
	src.down, src.edge = [3]bool{true, true, false}, [3]bool{false, true, false}
	p.Begin()
	if p.Press(Left) || p.RawPress(Left) {
		t.Error("a held button (no edge) reported a press")
	}
	if !p.Down(Left) {
		t.Error("a held button did not report Down")
	}
	if !p.Press(Right) {
		t.Error("the previous frame's Spend leaked into this one")
	}
}

// TestPointerClaimLastWins pins the claim/apply contract: within one
// frame the last claim wins (the chrome steps after the pane polls, so a
// grip outranks a seam); a second Apply without a new Begin repeats the
// same shape (ebiten's draw-only frames); Begin starts the next frame,
// and a frame with no claim writes the default arrow.
func TestPointerClaimLastWins(t *testing.T) {
	src := &scriptInput{}
	p := NewPointer(src)

	p.Begin()
	p.ClaimShape(ebiten.CursorShapeNSResize) // a pane seam polls first
	p.ClaimShape(ebiten.CursorShapeEWResize) // a grip steps after it
	p.Apply()
	if src.shape != ebiten.CursorShapeEWResize || src.writes != 1 {
		t.Errorf("two claims wrote %v (%d writes), want the last claim EW-resize, once", src.shape, src.writes)
	}

	src.shape = ebiten.CursorShapeDefault
	p.Apply() // no new frame: the draw repeats the frame's decision
	if src.shape != ebiten.CursorShapeEWResize || src.writes != 2 {
		t.Errorf("draw-only frame wrote %v (%d writes), want the last frame's EW-resize repeated", src.shape, src.writes)
	}

	p.Begin()
	p.Apply() // new frame, nothing claimed
	if src.shape != ebiten.CursorShapeDefault {
		t.Errorf("unclaimed frame wrote %v, want the default arrow", src.shape)
	}
}

// TestOwnsAtOnlyOnItsOwnLayer pins the one home of the "on my own input
// layer" test: a point on the widget's own active layer is owned, a point
// outside it is not, a covering layer blocks it, and a widget with no
// layer of its own — EffectiveInputLayer falls back to DefaultLayer,
// which passes ANYWHERE — owns nothing (the guard the columns copy was
// missing).
func TestOwnsAtOnlyOnItsOwnLayer(t *testing.T) {
	// Layers live one frame and are process-global: leave none behind.
	input.SetupInputLayersWithDeferred(nil)
	t.Cleanup(func() { input.SetupInputLayersWithDeferred(nil) })

	pt := image.Pt(10, 10)
	w := widget.NewContainer().GetWidget()
	w.Rect = image.Rect(0, 0, 40, 40)
	if OwnsAt(w, pt, input.LayerEventTypeMouseButton) {
		t.Error("a widget with no layer of its own owned the point (DefaultLayer passes anywhere)")
	}

	w.ElevateToNewInputLayer(&input.Layer{
		EventTypes: input.LayerEventTypeMouseButton,
		RectFunc:   func() image.Rectangle { return w.Rect },
	})
	if !OwnsAt(w, pt, input.LayerEventTypeMouseButton) {
		t.Error("the widget's own layer did not own a point inside its rect")
	}
	if OwnsAt(w, image.Pt(100, 100), input.LayerEventTypeMouseButton) {
		t.Error("the widget owned a point outside its own layer's rect")
	}
	if OwnsAt(w, pt, input.LayerEventTypeWheel) {
		t.Error("the widget owned an event type its layer does not claim")
	}

	// A cover in front of it: fullscreen and blocking (a modal window).
	cover := widget.NewContainer().GetWidget()
	cover.ElevateToNewInputLayer(&input.Layer{
		EventTypes: input.LayerEventTypeAll,
		FullScreen: true,
		BlockLower: true,
		RectFunc:   func() image.Rectangle { return image.Rect(0, 0, 1000, 1000) },
	})
	if OwnsAt(w, pt, input.LayerEventTypeMouseButton) {
		t.Error("the widget owned a point under a covering layer")
	}
}
