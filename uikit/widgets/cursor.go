package widgets

import "github.com/hajimehoshi/ebiten/v2"

// setCursorShape is the repo's one OS cursor write. Window grips and pane
// seams only CLAIM the frame's shape (ClaimCursor); ApplyCursor writes
// the winner once, after every input pass — ebitenui's own cursor
// management is off (Kit disables it), so no other writer can reset the
// shape behind it. Two writers alternating arrow/resize in one frame is
// what made the cursor flicker.
var setCursorShape = ebiten.SetCursorShape

// The current input pass's claimed shape. Last claim wins: the window
// chrome steps after the pane polls (both in Kit.Update and after
// ui.Draw), so an edge or corner grip outranks a seam; no claim in the
// pass is the default arrow.
var (
	claimShape ebiten.CursorShapeType
	claimed    bool
)

// ClaimCursor declares shape for the current input pass. It does not
// write: ApplyCursor is the frame's only OS cursor write.
func ClaimCursor(shape ebiten.CursorShapeType) {
	claimShape, claimed = shape, true
}

// BeginCursorFrame starts a new input pass: the previous pass's claim no
// longer applies, so a pointer that left a seam falls back to the default.
// The game calls it on the first line of Update. A host that never calls
// it (the uitest pump) keeps the last claim, i.e. its draws all repeat the
// same decision.
func BeginCursorFrame() {
	claimShape, claimed = ebiten.CursorShapeDefault, false
}

// ApplyCursor writes the current pass's claimed shape — the default arrow
// when nothing claimed. It does NOT clear the claim: ebiten runs Draw even
// on frames with zero Update ticks, and those draws must present the last
// pass's decision instead of falling back to the arrow. The kit calls it
// once, at the end of Draw, after ui.Draw's input pass and the chrome's
// own step.
func ApplyCursor() {
	shape := ebiten.CursorShapeDefault
	if claimed {
		shape = claimShape
	}
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
