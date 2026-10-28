package screens

import (
	"image"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/widgets"
)

// PressTarget reports whether a press at pt lands on an interactive
// control — the UI click cue's gate (game.Game.uiClick): Own's Control
// flag. Inside the owning layer the deepest widget under pt and its
// ancestors are walked: a button, a text input, a uikit slider's grab band,
// a window's registered rows (SetWindowRows — a table's rows sit in a
// scroll viewport the walk cannot descend into) or a context-menu entry
// makes the press loud; containers, window bodies, title bars, labels,
// disabled controls and dead space stay silent.
func (k *Kit) PressTarget(pt image.Point) bool { return k.Own(pt).Control }

// pressLoud walks n's subtree under pt: the node itself first (a control
// wrapping its own label), then its children, newest first (the tree's
// paint order), so the deepest control under the pointer wins.
func pressLoud(n widget.PreferredSizeLocateableWidget, pt image.Point) bool {
	w := n.GetWidget()
	if !hitRect(w, pt) {
		return false
	}
	switch c := n.(type) {
	case *widget.Button:
		return !w.Disabled
	case *widget.TextInput:
		return !w.Disabled
	case *widgets.Slider:
		return pt.In(c.HitBand())
	}
	if sub, ok := n.(widget.Containerer); ok {
		kids := sub.Children()
		for i := len(kids) - 1; i >= 0; i-- {
			if pressLoud(kids[i], pt) {
				return true
			}
		}
	}
	return false
}

// hitRect reports whether pt sits on a visible widget (a hidden widget
// keeps its last rect but claims nothing).
func hitRect(w *widget.Widget, pt image.Point) bool {
	return w.IsVisible() && pt.In(w.Rect)
}
