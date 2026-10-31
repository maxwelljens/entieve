package screens

import (
	"image"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// EnqueueFunc delivers executed intent to the simulation: the session
// executor queues every command it maps an intent onto here. Chrome
// panels never mutate sim state — every action starts as an Intent.
type EnqueueFunc func(sim.Command)

// services is the session's shared interaction state: the context menu,
// the Show Info window and the quantity picker (and, when a Kit hosts
// them, how to mount and raise the windows). Every Session copied from one
// source points at the same *services, so all panels of a session open the
// same menu, swap the same window and share the one picker.
type services struct {
	menu *widgets.Menu
	info *InfoWin
	// qty is the quantity picker every ± ticker was replaced by: one
	// instance, re-targeted on every open (its per-mode memory lives in it).
	qty *QtyWin
	// contact is the Details dialog of a contact offer: one instance,
	// re-targeted on every open (CONTEXT.md "Dialog").
	contact *ContactWin
	// show/close are the Kit-hosted Show Info lifecycle (mount + raise /
	// unmount). A bare NewEnv leaves them nil and Show falls back to the
	// window's own show/hide — a test harness mounts it by hand.
	show  func(widgets.InfoRef)
	close func()
	// qtyShow is the picker's Kit-hosted open (mount + centre + raise); a
	// bare NewEnv leaves it nil and the picker only shows.
	qtyShow func(widgets.QtyPick)
	// contactShow is the Details dialog's Kit-hosted open (mount + raise);
	// nil falls back to the window's own show.
	contactShow func(content.ContractID)
}

// Session is one host's environment: the three seams plus the services a
// kit-less session owns (its context menu and Show Info window). Screens
// never hold a Session — they take the seams they need as constructor
// params; hosts and tests reach the shared popup services here.
type Session interface {
	widgets.Seams

	// Menu is the session's context menu (nil in a bare seam literal).
	Menu() *widgets.Menu

	// InfoWindow is the session's Show Info window panel.
	InfoWindow() *InfoWin

	// QtyWindow is the session's one quantity picker (CONTEXT.md "Quantity
	// picker"). Panels never open it directly: they emit QtyPick through
	// the sink, which lands here. A host mounts the window (a Kit does it
	// on first open).
	QtyWindow() *QtyWin

	// OpenQty opens (or re-targets) the session's quantity picker on req —
	// exactly what Do(QtyPick{…}) executes. With a Kit hosting the window
	// this mounts it; in a bare session it only fills and reveals it.
	OpenQty(req widgets.QtyPick)

	// ContactWindow is the session's one contact Details dialog
	// (CONTEXT.md "Dialog"). Like the picker it is opened through the sink.
	ContactWindow() *ContactWin

	// ShowContact opens (or re-targets) the Details dialog on one live
	// offer — exactly what Do(ShowContact{…}) executes.
	ShowContact(offer content.ContractID)

	// ShowInfo opens (or swaps) the session's Show Info window on ref —
	// exactly what Do(ShowInfo{ref}) executes. With a Kit hosting the
	// window this mounts it and raises it to the top of the window
	// stack; in a bare session it only fills and reveals the window.
	ShowInfo(ref widgets.InfoRef)

	// OpenMenu opens the context menu with the given entries at at (the
	// click position in screen space). It replaces any menu already up.
	// Exactly what Do(OpenMenu{…}) executes.
	OpenMenu(entries []widgets.MenuEntry, at image.Point)
}

// session is the Session implementation NewEnv and Kit.Env return: the
// three seams plus the shared services.
type session struct {
	widgets.Seams
	svc *services
}

func (s *session) Menu() *widgets.Menu {
	if s.svc == nil {
		return nil
	}
	return s.svc.menu
}

func (s *session) InfoWindow() *InfoWin {
	if s.svc == nil {
		return nil
	}
	return s.svc.info
}

func (s *session) QtyWindow() *QtyWin {
	if s.svc == nil {
		return nil
	}
	return s.svc.qty
}

// OpenQty runs the picker's open: the Kit-hosted mount when there is one
// (it centres and raises the window), else the window's own show.
func (s *session) OpenQty(req widgets.QtyPick) {
	if s.svc == nil || s.svc.qty == nil {
		return
	}
	if s.svc.qtyShow != nil {
		s.svc.qtyShow(req)
		return
	}
	s.svc.qty.Show(req)
}

// ContactWindow is the session's Details dialog (nil before NewEnv finishes).
func (s *session) ContactWindow() *ContactWin {
	if s.svc == nil {
		return nil
	}
	return s.svc.contact
}

// ShowContact runs the Details dialog's open: the Kit-hosted mount when
// there is one, else the window's own show.
func (s *session) ShowContact(offer content.ContractID) {
	if s.svc == nil || s.svc.contact == nil {
		return
	}
	if s.svc.contactShow != nil {
		s.svc.contactShow(offer)
		return
	}
	s.svc.contact.Show(offer)
}

func (s *session) ShowInfo(ref widgets.InfoRef) {
	if s.svc == nil {
		return
	}
	if s.svc.show != nil {
		s.svc.show(ref)
		return
	}
	s.svc.info.Show(ref)
}

func (s *session) OpenMenu(entries []widgets.MenuEntry, at image.Point) {
	if m := s.Menu(); m != nil {
		m.Open(entries, at)
	}
}

// NewEnv builds a standalone session — the default EVE-dark Theme over
// the given content and sim, uikit's built-in ActionSink on enqueue, and
// the session's own context menu and Show Info window. It exists for
// hosts that build several panels from one source without a Kit
// (cmd/enui-spike, the uikit tests); the game builds its session from
// Kit.Env instead, so one theme, view and sink serve the whole session.
func NewEnv(c *content.Content, s *sim.Sim, enqueue EnqueueFunc, p *widgets.Pointer) Session {
	sess := &session{}
	exec := widgets.NewExec(
		func(ref widgets.InfoRef) { sess.ShowInfo(ref) },
		func(entries []widgets.MenuEntry, at image.Point) { sess.OpenMenu(entries, at) },
		enqueue,
		nil,
	)
	sess.Seams = widgets.NewSeams(
		theme.NewSeam(theme.DefaultPalette(), theme.ScaleTypeScale(theme.DefaultTypeScale())),
		widgets.NewView(c, s),
		// The picker's and the Details dialog's opens are session intents
		// the built-in executor does not know: the sink intercepts them
		// here, everything else rides exec.
		widgets.SinkFunc(func(i widgets.Intent) {
			switch v := i.(type) {
			case widgets.QtyPick:
				sess.OpenQty(v)
				return
			case widgets.ShowContact:
				sess.ShowContact(v.Offer)
				return
			}
			exec.Do(i)
		}),
		// A bare session may be given the host's frame (the kit's session
		// always is); nil builds its own OS one.
		p,
	)
	sess.svc = &services{}
	sess.svc.menu = widgets.NewMenu(sess, sess.Pointer())
	sess.svc.info = NewInfoWin(sess)
	sess.svc.qty = NewQtyWin(sess)
	sess.svc.contact = NewContactWin(sess)
	return sess
}
