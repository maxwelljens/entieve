package uikit

import (
	"image"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// EnqueueFunc delivers player intent to the simulation. Chrome panels never
// mutate sim state — every action becomes a sim.Command.
type EnqueueFunc func(sim.Command)

// services is the session's shared interaction state: the context menu
// and the Show Info window (and, when a Kit hosts them, how to mount and
// raise the window). Every Env copied from one source points at the same
// *services, so all panels of a session open the same menu and swap the
// same window.
type services struct {
	menu *Menu
	info *InfoWin
	// show/close are the Kit-hosted Show Info lifecycle (mount + raise /
	// unmount). A bare NewEnv leaves them nil and Show falls back to the
	// window's own show/hide — a test harness mounts it by hand.
	show  func(InfoRef)
	close func()
}

// newServices builds the menu and info window bound to env. The returned
// services is what the environment's svc slot must point at — every Env
// copy made afterwards shares it.
func newServices(env Env) *services {
	s := &services{}
	env.svc = s // the menu and window reach the shared state through env
	s.menu = NewMenu(env)
	s.info = NewInfoWin(env)
	return s
}

// Env is the dependency bundle every chrome panel receives: theme, palette,
// type scale, faces, content and the command seam. Build it once with
// NewEnv and pass it to panel constructors.
type Env struct {
	Content *content.Content
	Sim     *sim.Sim
	Theme   *widget.Theme
	Pal     Palette
	TS      TypeScale
	Enqueue EnqueueFunc

	faces kitFaces
	svc   *services
}

// NewEnv builds the EVE-dark environment (theme + faces) for the given sim.
func NewEnv(c *content.Content, s *sim.Sim, enqueue EnqueueFunc) Env {
	p := DefaultPalette()
	ts := scaleTypeScale(DefaultTypeScale()) // design units → current scale
	env := Env{
		Content: c,
		Sim:     s,
		Theme:   NewTheme(p, ts),
		Pal:     p,
		TS:      ts,
		Enqueue: enqueue,
		faces:   newFaces(ts),
	}
	env.svc = newServices(env)
	return env
}

// Menu returns the environment's context menu (nil in a bare Env literal).
func (e Env) Menu() *Menu {
	if e.svc == nil {
		return nil
	}
	return e.svc.menu
}

// OpenMenu opens the context menu with the given entries at at (the click
// position in screen space). It replaces any menu already up.
func (e Env) OpenMenu(entries []MenuEntry, at image.Point) {
	if m := e.Menu(); m != nil {
		m.Open(entries, at)
	}
}

// ShowInfo opens (or swaps) the session's Show Info window on ref. With a
// Kit hosting the window this mounts it and raises it to the top of the
// window stack; in a bare environment it only fills and reveals the window.
func (e Env) ShowInfo(ref InfoRef) {
	if e.svc == nil {
		return
	}
	if e.svc.show != nil {
		e.svc.show(ref)
		return
	}
	e.svc.info.Show(ref)
}

// InfoWindow returns the session's Show Info window panel.
func (e Env) InfoWindow() *InfoWin {
	if e.svc == nil {
		return nil
	}
	return e.svc.info
}

// Face returns the text face for one type-scale step ("tiny", "small",
// "base", "large"). Unknown steps fall back to base.
func (e Env) Face(step string) *text.Face {
	switch step {
	case StepTiny:
		return facePtr(e.faces.tiny)
	case StepSmall:
		return facePtr(e.faces.small)
	case StepLarge:
		return facePtr(e.faces.large)
	default:
		return facePtr(e.faces.base)
	}
}

// FaceBold returns the emphasis face for one type-scale step: window
// titles, table header sort buttons, buttons and HUD readouts. Unknown
// steps fall back to the base step's Bold.
func (e Env) FaceBold(step string) *text.Face {
	switch step {
	case StepTiny:
		return facePtr(e.faces.baseBold) // no tiny emphasis: titles/readouts are small and up
	case StepSmall:
		return facePtr(e.faces.smallBold)
	case StepLarge:
		return facePtr(e.faces.largeBold)
	default:
		return facePtr(e.faces.baseBold)
	}
}

// FaceMono returns the mono contents face (Source Code Pro Regular, the
// variable font's default instance) for one type-scale step: the row
// cells of tables whose contents are numbers and listed entries
// (inventory, market) — monospaced cells line the columns up. Unknown
// steps fall back to base.
func (e Env) FaceMono(step string) *text.Face {
	switch step {
	case StepTiny:
		return facePtr(render.MonoFace(e.TS.Tiny))
	case StepSmall:
		return facePtr(render.MonoFace(e.TS.Small))
	case StepLarge:
		return facePtr(render.MonoFace(e.TS.Large))
	default:
		return facePtr(render.MonoFace(e.TS.Base))
	}
}

// Type-scale step names accepted by Env.Face.
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
