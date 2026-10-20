package widgets

import "github.com/hajimehoshi/ebiten/v2"

// setCursorShape is the repo's one OS cursor write. Window grips and pane
// seams only CLAIM the frame's shape (ClaimCursor); ApplyCursor writes
// the winner once, after every input pass — ebitenui's own cursor
// management is off (Kit disables it), so no other writer can reset the
// shape behind it. Two writers alternating arrow/resize in one frame is
// what made the cursor flicker.
var setCursorShape = ebiten.SetCursorShape

// The frame's claimed shape. Last claim wins: the window chrome steps
// after the pane polls (both in Kit.Update and after ui.Draw), so an edge
// or corner grip outranks a seam; no claim at all is the default arrow.
var (
	claimShape ebiten.CursorShapeType
	claimed    bool
)

// ClaimCursor declares shape for the current frame. It does not write:
// ApplyCursor is the frame's only OS cursor write.
func ClaimCursor(shape ebiten.CursorShapeType) {
	claimShape, claimed = shape, true
}

// ApplyCursor writes the frame's claimed shape — the default arrow when
// nothing claimed — and clears the claim. The kit calls it once, at the
// end of Draw, after ui.Draw's input pass and the chrome's own step.
func ApplyCursor() {
	shape := ebiten.CursorShapeDefault
	if claimed {
		shape = claimShape
	}
	claimShape, claimed = ebiten.CursorShapeDefault, false
	setCursorShape(shape)
}

// SetCursorShapeForTest overrides the shared cursor write for a headless
// test and returns a restore func. Test-only seam: never call it from
// production code.
func SetCursorShapeForTest(f func(ebiten.CursorShapeType)) func() {
	prev := setCursorShape
	setCursorShape = f
	return func() { setCursorShape = prev }
}
