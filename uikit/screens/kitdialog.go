package screens

import (
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
)

// One lifecycle for the kit's on-demand windows (CONTEXT.md "Dialog"):
// Show Info, the order mini-window and the quantity picker are all
// single-instance dialogs — a launch re-targets the one window in place
// and raises it to the top of the stack, a close unmounts it, and a
// launch landing while the × close still fades re-opens it. The rules
// live here once; each panel embeds dialogShell for the Close/hide/IsOpen
// trio and the Kit keeps addWindow's fade/settle/stack work, raise() and
// Over untouched.

// dialogShell is the lifecycle every on-demand dialog shares: its window,
// the Kit's close hook, and an optional extra hide hook for the panel's own
// state. Embedding it gives the panel the trio and keeps its win field
// reachable; content, placement and semantics stay in the panel.
type dialogShell struct {
	win     *widget.Window
	onClose func()
	onHide  func()
}

// Close closes the dialog: through the Kit it also unmounts (so a closed
// dialog stops gating world input); in a bare environment it just hides.
func (d *dialogShell) Close() {
	if d.onClose != nil {
		d.onClose()
		return
	}
	d.hide()
}

// hide flips the container invisible, after the panel's own hide hook (the
// kit's unmount path).
func (d *dialogShell) hide() {
	if d.onHide != nil {
		d.onHide()
	}
	theme.ShowWidget(d.win.GetContainer().GetWidget(), false)
}

// IsOpen reports whether the dialog is revealed (shown, and mounted when a
// Kit hosts it).
func (d *dialogShell) IsOpen() bool {
	return d.win != nil && d.win.GetContainer().GetWidget().IsVisible()
}

// dialog is one kit-hosted on-demand window's lifecycle state. remove is
// the live mount handle returned by addWindow — nil while closed, and
// still non-nil through a raise (showDialog replaces it). place runs on
// every show, before the mount: only the picker has one (it centres on
// the viewport); Show Info and the order window are never re-placed, so
// a re-target cannot make them jump.
type dialog struct {
	win    *widget.Window
	hide   func()
	isOpen func() bool
	place  func(*widget.Window)
	remove widget.RemoveWindowFunc // nil = closed
}

// newDialog wraps one built panel's window: it registers the window's ×
// close (one site for every dialog — SetWindowClose attaches the button
// and wires it to close) and returns the lifecycle handle the exported
// Show/Close/Open methods drive.
func (k *Kit) newDialog(win *widget.Window, hide func(), isOpen func() bool, place func(*widget.Window), close func()) *dialog {
	k.SetWindowClose(win, close)
	return &dialog{win: win, hide: hide, isOpen: isOpen, place: place}
}

// showDialog opens or re-targets a dialog: retarget fills (or swaps) the
// panel and reports whether it can open at all (false = refuse, nothing
// mounts), place recentres where the panel declares one, then the mount.
//
// A launch on the already-open window is a raise, not an open: the mount
// is removed and re-added on top of the stack with animate=false — never
// a plain AddWindow, which would restart the fade. A launch landing while
// the × close still fades re-opens it: addWindow drops that pending
// unmount and fades the window back in from zero.
//
// The remount rides the documented ebitenui window-remount leak (a few KB
// per raise) — the price of a deterministic top-of-stack without
// ebitenui's own relayering.
func (k *Kit) showDialog(d *dialog, retarget func() bool) {
	if !retarget() {
		return
	}
	if d.place != nil {
		d.place(d.win)
	}
	raised := d.remove != nil
	if raised {
		d.remove()
	}
	d.remove = k.addWindow(d.win, !raised)
}

// closeDialog closes a dialog: unmount (a visible window's rect survives;
// the transient dialogs never persist it) and hide. Idempotent — a
// second close finds no mount and only hides again.
func (k *Kit) closeDialog(d *dialog) {
	if d.remove != nil {
		d.remove()
		d.remove = nil
	}
	d.hide()
}
