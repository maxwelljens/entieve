package widgets

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestCursorClaimLastWins pins the pass/claim/draw contract: within one
// input pass the last claim wins (the chrome steps after the pane polls,
// so a grip outranks a seam); a second Apply without a new pass repeats
// the same shape (ebiten's draw-only frames); BeginCursorFrame starts the
// next pass, and a pass with no claim writes the default arrow.
func TestCursorClaimLastWins(t *testing.T) {
	var got ebiten.CursorShapeType
	defer SetCursorShapeForTest(func(s ebiten.CursorShapeType) { got = s })()

	BeginCursorFrame()
	ClaimCursor(ebiten.CursorShapeNSResize) // a pane seam polls first
	ClaimCursor(ebiten.CursorShapeEWResize) // a grip steps after it
	ApplyCursor()
	if got != ebiten.CursorShapeEWResize {
		t.Errorf("two claims wrote %v, want the last claim EW-resize", got)
	}

	got = ebiten.CursorShapeDefault
	ApplyCursor() // no new pass: the draw repeats the pass's decision
	if got != ebiten.CursorShapeEWResize {
		t.Errorf("draw-only frame wrote %v, want the last pass's EW-resize", got)
	}

	BeginCursorFrame()
	ApplyCursor() // new pass, nothing claimed
	if got != ebiten.CursorShapeDefault {
		t.Errorf("unclaimed pass wrote %v, want the default arrow", got)
	}
}
