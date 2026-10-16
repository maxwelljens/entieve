package uikit

import (
	"image"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
)

// EnqueueFunc delivers executed intent to the simulation: the session
// executor queues every command it maps an intent onto here. Chrome
// panels never mutate sim state — every action starts as an Intent.
type EnqueueFunc func(sim.Command)

// services is the session's shared interaction state: the context menu
// and the Show Info window (and, when a Kit hosts them, how to mount and
// raise the window). Every Session copied from one source points at the
// same *services, so all panels of a session open the same menu and swap
// the same window.
type services struct {
	menu *Menu
	info *InfoWin
	// show/close are the Kit-hosted Show Info lifecycle (mount + raise /
	// unmount). A bare NewEnv leaves them nil and Show falls back to the
	// window's own show/hide — a test harness mounts it by hand.
	show  func(InfoRef)
	close func()
}

// Session is one host's environment: the three seams plus the services a
// kit-less session owns (its context menu and Show Info window). Screens
// never hold a Session — they take the seams they need as constructor
// params; hosts and tests reach the shared popup services here.
type Session interface {
	Seams

	// Menu is the session's context menu (nil in a bare seam literal).
	Menu() *Menu

	// InfoWindow is the session's Show Info window panel.
	InfoWindow() *InfoWin

	// ShowInfo opens (or swaps) the session's Show Info window on ref —
	// exactly what Do(ShowInfo{ref}) executes. With a Kit hosting the
	// window this mounts it and raises it to the top of the window
	// stack; in a bare session it only fills and reveals the window.
	ShowInfo(ref InfoRef)

	// OpenMenu opens the context menu with the given entries at at (the
	// click position in screen space). It replaces any menu already up.
	// Exactly what Do(OpenMenu{…}) executes.
	OpenMenu(entries []MenuEntry, at image.Point)

	// OpenRowMenu derives a row's context menu from its RowSpec and
	// opens it at at — through the sink (Do(OpenMenu{…})).
	OpenRowMenu(spec RowSpec, at image.Point)
}

// session is the Session implementation NewEnv and Kit.Env return: the
// three seams plus the shared services.
type session struct {
	Seams
	svc *services
}

func (s *session) Menu() *Menu {
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

func (s *session) ShowInfo(ref InfoRef) {
	if s.svc == nil {
		return
	}
	if s.svc.show != nil {
		s.svc.show(ref)
		return
	}
	s.svc.info.Show(ref)
}

func (s *session) OpenMenu(entries []MenuEntry, at image.Point) {
	if m := s.Menu(); m != nil {
		m.Open(entries, at)
	}
}

func (s *session) OpenRowMenu(spec RowSpec, at image.Point) {
	openRowMenu(s, s, spec, at)
}

// NewEnv builds a standalone session — the default EVE-dark Theme over
// the given content and sim, uikit's built-in ActionSink on enqueue, and
// the session's own context menu and Show Info window. It exists for
// hosts that build several panels from one source without a Kit
// (cmd/enui-spike, the uikit tests); the game builds its session from
// Kit.Env instead, so one theme, view and sink serve the whole session.
func NewEnv(c *content.Content, s *sim.Sim, enqueue EnqueueFunc) Session {
	sess := &session{}
	exec := &exec{
		show:  func(ref InfoRef) { sess.ShowInfo(ref) },
		open:  func(entries []MenuEntry, at image.Point) { sess.OpenMenu(entries, at) },
		queue: enqueue,
	}
	sess.Seams = seams{
		Theme:       newTheme(DefaultPalette(), scaleTypeScale(DefaultTypeScale())),
		ContentView: newView(c, s),
		ActionSink:  exec,
	}
	sess.svc = &services{}
	sess.svc.menu = NewMenu(sess)
	sess.svc.info = NewInfoWin(sess)
	return sess
}

// Type-scale step names accepted by Theme.Face.
const (
	StepTiny  = "tiny"
	StepSmall = "small"
	StepBase  = "base"
	StepLarge = "large"
)

// Panel is the common shape of every chrome panel: a widget tree plus Sync,
// which refreshes displayed state from the sim (called once per frame).
type Panel interface {
	Sync()
}

// WindowPanel is a Panel that owns a draggable/resizable ebitenui window.
type WindowPanel interface {
	Panel
	Win() *widget.Window
}
