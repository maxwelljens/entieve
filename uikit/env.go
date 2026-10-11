package uikit

import (
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// EnqueueFunc delivers player intent to the simulation. Chrome panels never
// mutate sim state — every action becomes a sim.Command.
type EnqueueFunc func(sim.Command)

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
}

// NewEnv builds the EVE-dark environment (theme + faces) for the given sim.
func NewEnv(c *content.Content, s *sim.Sim, enqueue EnqueueFunc) Env {
	p := DefaultPalette()
	ts := DefaultTypeScale()
	return Env{
		Content: c,
		Sim:     s,
		Theme:   NewTheme(p, ts),
		Pal:     p,
		TS:      ts,
		Enqueue: enqueue,
		faces:   newFaces(ts),
	}
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
