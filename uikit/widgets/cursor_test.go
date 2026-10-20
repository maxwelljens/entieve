package widgets

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestCursorClaimLastWins pins the frame's single writer: one claim per
// frame is applied, the last claimer wins (the chrome steps after the
// pane polls, so a grip outranks a seam), and a frame with no claim
// writes the default arrow.
func TestCursorClaimLastWins(t *testing.T) {
	var got ebiten.CursorShapeType
	defer SetCursorShapeForTest(func(s ebiten.CursorShapeType) { got = s })()

	ClaimCursor(ebiten.CursorShapeNSResize) // a pane seam polls first
	ClaimCursor(ebiten.CursorShapeEWResize) // a grip steps after it
	ApplyCursor()
	if got != ebiten.CursorShapeEWResize {
		t.Errorf("two claims wrote %v, want the last claim EW-resize", got)
	}

	got = ebiten.CursorShapeEWResize
	ApplyCursor() // nothing claimed this frame
	if got != ebiten.CursorShapeDefault {
		t.Errorf("unclaimed frame wrote %v, want the default arrow", got)
	}
}
