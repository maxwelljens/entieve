package screens

import (
	"github.com/ebitenui/ebitenui/widget"
)

// One lifecycle for the kit's on-demand windows (CONTEXT.md "Dialog"):
// Show Info, the order mini-window and the quantity picker are all
// single-instance dialogs — a launch re-targets the one window in place
// and raises it to the top of the stack, a close unmounts it, and a
// launch landing while the × close still fades re-opens it. The rules
// live here once; each panel keeps its own Show/hide/IsOpen and the Kit
// keeps addWindow's fade/settle/stack work, raise() and Over untouched.

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
