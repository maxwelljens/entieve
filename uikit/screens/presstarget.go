package screens

import (
	"image"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/widgets"
)

// PressTarget reports whether a press at pt lands on an interactive
// control — the UI click cue's gate (game.Game.uiClick). It is the widget
// tree's own layering: the context menu's popup, then top chrome, then the
// window stack top-down, then the root chrome (HUD, rack), and the first
// layer holding pt decides. Inside that layer the deepest widget under pt
// and its ancestors are walked: a button, a text input, a uikit slider's
// grab band, or rows(win) — the host's tables, whose rows sit inside a
// scroll viewport the walk cannot descend into — makes the press loud;
// containers, window bodies, title bars, labels and dead space stay silent.
func (k *Kit) PressTarget(pt image.Point, rows func(*widget.Window) bool) bool {
	if r, open := k.MenuRect(); open && pt.In(r) {
		return true
	}
	rowsAt := func(win *widget.Window) bool {
		if win == k.ov.win && k.ov.tbl != nil && k.ov.tbl.RowAt(pt) {
			return true
		}
		return rows != nil && rows(win)
	}
	for i := len(k.tops) - 1; i >= 0; i-- {
		if c := k.tops[i].Container(); hitRect(c.GetWidget(), pt) {
			return pressLoud(c, pt) // top chrome occludes the stack below
		}
	}
	for i := len(k.stack) - 1; i >= 0; i-- {
		win := k.stack[i].win
		if c := win.GetContainer(); hitRect(c.GetWidget(), pt) {
			// The topmost window under pt owns it: its widgets and rows.
			return pressLoud(c, pt) || rowsAt(win)
		}
	}
	for _, c := range k.root.Children() {
		if pressLoud(c, pt) {
			return true
		}
	}
	return false
}

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
