package screens

import (
	"image"

	"github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/widget"
)

// Hit test — one walk, one answer (CONTEXT.md "Hit test").
//
// Every "does the interface own this point?" question in the app is Own:
// the world gate (game.overUI), the click cue (PressTarget), the window
// raise (raisePressed) and Kit.Over read the same precedence, so a press is
// owned by exactly one layer and paint order, input order and this answer
// cannot disagree.
//
// The walk's order is the precedence, top of the interface first: the
// popup latch, the popup's own rect, a modal window's fullscreen layer, the
// top chrome, the window stack (topmost first), the root chrome's
// descendants, and last ebitenui's hover flag — which gates but never
// raises.

// Layer is where a point lands in the interface's stacking order. The
// values run in the hit test's precedence order, not in paint order.
type Layer uint8

const (
	LayerWorld      Layer = iota // the simulation: nothing of the interface
	LayerPopup                   // the context menu and the quantity picker
	LayerTopChrome               // the navigation rail, above every window
	LayerWindow                  // one mounted window (the topmost under pt)
	LayerRootChrome              // HUD and rack — descendants only
	LayerHovered                 // ebitenui's hover flag: the backstop
)

// Owner is one hit test's answer: the layer that owns pt, the window when
// that layer is a window (nil otherwise), and whether pt lands on an
// interactive control there — the click cue's gate.
type Owner struct {
	Layer   Layer
	Window  *widget.Window
	Control bool
}

// Own runs the one press-ownership walk over pt, in the order documented on
// the package's hit test comment.
func (k *Kit) Own(pt image.Point) Owner {
	// 1. The popup latch: while a popup is up — or consumed this frame's
	// gesture — it owns every point, the same frame gate MenuInput returns.
	// The control answer is the popup's own; at the frame's press point it
	// also carries the sample taken before the poll (the press that runs an
	// entry closes the popup and runs its action).
	if k.held {
		c := k.popupHit(pt)
		if pt == k.popupAt {
			c = c || k.popupControl
		}
		return Owner{Layer: LayerPopup, Control: c}
	}
	// 2. The popup's own rect, before its first poll of a frame (and for a
	// host that never polls): the deterministic world gate MenuRect feeds.
	if r, open := k.MenuRect(); open && pt.In(r) {
		return Owner{Layer: LayerPopup, Control: k.menuEntry(pt)}
	}
	// 3. A visible modal window: its ebitenui input layer is fullscreen, so
	// nothing below it — rail, another window or the world — is pressed.
	if w := k.modalWindow(); w != nil {
		return Owner{Layer: LayerWindow, Window: w, Control: k.windowControl(w, pt)}
	}
	// 4. Top chrome (the rail) owns its rect outright, above every window.
	for i := len(k.tops) - 1; i >= 0; i-- {
		if c := k.tops[i].Container(); hitRect(c.GetWidget(), pt) {
			return Owner{Layer: LayerTopChrome, Control: pressLoud(c, pt)}
		}
	}
	// 5. The window stack, topmost first: the first visible window holding
	// pt owns it — its widgets, then its registered row surface.
	for i := len(k.stack) - 1; i >= 0; i-- {
		win := k.stack[i].win
		if hitRect(win.GetContainer().GetWidget(), pt) {
			return Owner{Layer: LayerWindow, Window: win, Control: k.windowControl(win, pt)}
		}
	}
	// 6. Root chrome (HUD, rack): its descendants only — the HUD container
	// stretches over the whole screen and owns nothing by itself.
	for _, c := range k.root.Children() {
		if rootChromeHit(c, pt) {
			return Owner{Layer: LayerRootChrome, Control: pressLoud(c, pt)}
		}
	}
	// 7. ebitenui's hover flag, last: it covers what the rects cannot
	// measure (tooltips, ephemeral widgets) and lags a frame behind.
	if input.UIHovered {
		return Owner{Layer: LayerHovered}
	}
	return Owner{}
}

// SetWindowRows registers a mounted window's table-row surface: the hit
// test consults it after the widget walk fails, because a table's rows sit
// inside a scroll viewport the walk cannot descend into. Hosts register one
// live closure per mount, so a table swapped inside the panel is seen; nil
// drops the registration.
func (k *Kit) SetWindowRows(w *widget.Window, hit func(image.Point) bool) {
	if hit == nil {
		delete(k.rows, w)
		return
	}
	k.rows[w] = hit
}

// Held reports the popup latch: the context menu or the quantity picker
// owned this frame's input (or consumed its press/Esc). Hosts read it to
// gate world input and to leave the frame's keys alone.
func (k *Kit) Held() bool { return k.held }

// windowControl reports whether pt lands on a control inside win: its
// widget tree first, then its registered row surface (topmost window only —
// a window below is never consulted for a point the walk already gave away).
func (k *Kit) windowControl(win *widget.Window, pt image.Point) bool {
	if pressLoud(win.GetContainer(), pt) {
		return true
	}
	if rows := k.rows[win]; rows != nil {
		return rows(pt)
	}
	return false
}

// popupHit reports whether pt lands on a control of the open popup: a menu
// entry, or a control of the open quantity picker.
func (k *Kit) popupHit(pt image.Point) bool {
	if _, open := k.MenuRect(); open {
		return k.menuEntry(pt)
	}
	if k.qtyWin != nil && k.qtyWin.IsOpen() {
		if w := k.QtyWindow(); w != nil {
			return k.windowControl(w, pt)
		}
	}
	return false
}

// menuEntry reports whether pt lands on a menu entry that acts: the popup's
// own bands, separators and disabled entries excluded (the popup is drawn,
// not a widget, so its bands are the hit test).
func (k *Kit) menuEntry(pt image.Point) bool {
	m := k.svc.menu
	if m == nil || !m.IsOpen() {
		return false
	}
	for i, e := range m.Entries() {
		if pt.In(m.RowRect(i)) {
			return !e.Sep && !e.Disabled
		}
	}
	return false
}

// modalWindow returns the visible modal window that owns the frame, nil
// when none is up.
func (k *Kit) modalWindow() *widget.Window {
	for _, m := range k.stack {
		if m.win.Modal && m.win.GetContainer().GetWidget().IsVisible() {
			return m.win
		}
	}
	return nil
}

// stackTop is the topmost mounted window (nil for an empty stack).
func (k *Kit) stackTop() *widget.Window {
	if len(k.stack) == 0 {
		return nil
	}
	return k.stack[len(k.stack)-1].win
}

// rootChromeHit reports whether pt sits on a descendant of a root child —
// never on the child's own rect: the HUD stretches over the whole screen
// and must claim only what it paints.
func rootChromeHit(n widget.PreferredSizeLocateableWidget, pt image.Point) bool {
	sub, ok := n.(widget.Containerer)
	if !ok {
		return false
	}
	for _, kid := range sub.Children() {
		if hitRect(kid.GetWidget(), pt) || rootChromeHit(kid, pt) {
			return true
		}
	}
	return false
}
