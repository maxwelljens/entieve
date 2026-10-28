package widgets

import (
	"image"

	"github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Pointer frame — one frame's gesture.
//
// Every widget that reads the pointer (the menu, the pane seams, the slider
// bands, the column grips, the window chrome, the starmap) reads THIS: one
// sample per frame, taken by the host at the top of Update (Begin), so the
// world and the interface hit-test the same point and a frame can never see
// the pointer twice. A widget only says what a gesture MEANS (grab, drag,
// dismiss); the frame says what the pointer IS.
//
// The gesture vocabulary has two halves on purpose:
//
//	Press     the frame's press edge, unless the frame spent it. What every
//	          non-menu poller reads.
//	RawPress  the edge, always. Only the context menu's Poll reads it, so
//	          the popup still acts on the press that dismisses it.
//	Spend     Click/Esc consume the frame's gesture: the popup owns the
//	          press, and nothing underneath sees it that frame.
//	Down      the held state (a drag), never an edge.
//
// A frame is also the pointer's SHAPE: claimers only declare (ClaimShape,
// last claim wins — the chrome steps after the pane polls, so a grip
// outranks a seam), and Apply writes the frame's winner once, at the end of
// Draw. Apply never re-samples and never clears: ebiten runs Draw even on
// frames with no Update, and those draws must present the last frame's
// decision, not fall back to the arrow.
type Pointer struct {
	src Source

	x, y    int
	down    [3]bool // held this frame
	pressed [3]bool // an edge this frame
	spent   [3]bool // the frame's gesture was consumed

	shape   ebiten.CursorShapeType
	claimed bool
}

// Button is a pointer button, indexed into the frame's three states.
type Button int

const (
	Left Button = iota
	Right
	Middle
)

// mouse maps the frame's button onto ebiten's, explicitly (never by
// relying on the two enums agreeing).
func (b Button) mouse() ebiten.MouseButton {
	switch b {
	case Right:
		return ebiten.MouseButtonRight
	case Middle:
		return ebiten.MouseButtonMiddle
	}
	return ebiten.MouseButtonLeft
}

// Source is where a frame's sample comes from: the OS in production
// (OSInput), a script in tests.
type Source interface {
	Cursor() (int, int)
	Down(Button) bool
	JustPressed(Button) bool
	SetShape(ebiten.CursorShapeType)
}

// OSInput is the real pointer: ebiten's live cursor, buttons and shape.
// It is deliberately not ebitenui's input cache, which freezes while the
// kit skips ui.Update (a menu is up), so every hit test would land at the
// open anchor.
type OSInput struct{}

func (OSInput) Cursor() (int, int) { return ebiten.CursorPosition() }
func (OSInput) Down(b Button) bool { return ebiten.IsMouseButtonPressed(b.mouse()) }
func (OSInput) JustPressed(b Button) bool {
	return inpututil.IsMouseButtonJustPressed(b.mouse())
}
func (OSInput) SetShape(s ebiten.CursorShapeType) { ebiten.SetCursorShape(s) }

// NewPointer builds a frame over src; nil means the real OS pointer.
func NewPointer(src Source) *Pointer {
	if src == nil {
		src = OSInput{}
	}
	return &Pointer{src: src}
}

// Begin starts a frame: one sample of the source, the previous frame's
// spent gesture and shape claim gone. The host calls it once, first thing
// in Update.
func (p *Pointer) Begin() {
	if p == nil {
		return
	}
	p.x, p.y = p.src.Cursor()
	for b := Left; b <= Middle; b++ {
		p.down[b], p.pressed[b], p.spent[b] = p.src.Down(b), p.src.JustPressed(b), false
	}
	p.shape, p.claimed = ebiten.CursorShapeDefault, false
}

// At is the frame's pointer position (the one point every hit test uses).
func (p *Pointer) At() image.Point {
	if p == nil {
		return image.Point{}
	}
	return image.Pt(p.x, p.y)
}

// Down reports whether b is held this frame (a drag, not an edge).
func (p *Pointer) Down(b Button) bool { return p != nil && p.down[b] }

// Press reports this frame's press edge for b unless the frame spent it.
func (p *Pointer) Press(b Button) bool { return p != nil && p.pressed[b] && !p.spent[b] }

// RawPress reports the frame's press edge for b, spent or not — the
// context menu's own read, so the popup acts on the press it consumes.
func (p *Pointer) RawPress(b Button) bool { return p != nil && p.pressed[b] }

// Spend consumes the frame's gesture for b: the popup's Click/Esc own the
// press, and no other poller sees it until the next Begin.
func (p *Pointer) Spend(b Button) {
	if p != nil {
		p.spent[b] = true
	}
}

// ClaimShape declares the frame's cursor shape (last claim wins). It does
// not write: Apply is the frame's one OS write.
func (p *Pointer) ClaimShape(s ebiten.CursorShapeType) {
	if p != nil {
		p.shape, p.claimed = s, true
	}
}

// Apply writes the frame's claimed shape — the default arrow when nothing
// claimed. It never re-samples and never clears the claim: a draw-only
// frame repeats the last frame's decision (see Pointer).
func (p *Pointer) Apply() {
	if p == nil {
		return
	}
	shape := ebiten.CursorShapeDefault
	if p.claimed {
		shape = p.shape
	}
	p.src.SetShape(shape)
}

// OwnsAt reports whether pt sits on w's own active input layer for ev —
// the one home of the "on my own layer" test every hover/drag hit test
// asks: a seam, a slider band or a column grip under another window (or
// the modal menu) is inert. A widget with no layer of its own falls back
// to ebitenui's DefaultLayer, which passes ANYWHERE, so that fallback is
// rejected here (a never-mounted or stale widget owns nothing).
func OwnsAt(w *widget.Widget, pt image.Point, ev input.LayerEventType) bool {
	if w == nil || !w.IsVisible() {
		return false
	}
	l := w.EffectiveInputLayer()
	return l != &input.DefaultLayer && l.ActiveFor(pt.X, pt.Y, ev)
}
