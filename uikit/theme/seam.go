package theme

import (
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/render"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// The Theme seam (CONTEXT.md): the visual interface of every surface —
// faces, scaled metrics, palette — implemented by the kit theme below.
// widgets and screens draw through this interface; theme owns both it
// and its implementation.

// Theme is the visual seam: everything a widget needs to draw itself,
// and nothing that reads state or emits intents. A Theme-only widget
// (the context menu, the title bar, Settings) constructs and runs on a
// Theme alone — it can never touch the view or the sink.
type Theme interface {
	// Face is the text face for one type-scale step ("tiny", "small",
	// "base", "large"). Unknown steps fall back to base.
	Face(step string) *text.Face

	// FaceBold is the emphasis face for one step: window titles, table
	// header sort buttons, buttons and HUD readouts. Unknown steps fall
	// back to the base step's Bold.
	FaceBold(step string) *text.Face

	// FaceMono is the mono contents face (IBM Plex Mono) for one step:
	// the row cells of tables whose contents are numbers and listed
	// entries. Unknown steps fall back to base.
	FaceMono(step string) *text.Face

	// Pal is the palette the kit paints with.
	Pal() Palette

	// TS is the current type scale, already multiplied by the UI scale.
	TS() TypeScale

	// WidgetTheme is the ebitenui widget theme panels ride (window
	// chrome, button/list/text params).
	WidgetTheme() *widget.Theme
}

// kitTheme is the default Theme: one palette, one scaled type scale,
// the faces derived from it and the widget theme built from both.
type kitTheme struct {
	th    *widget.Theme
	pal   Palette
	ts    TypeScale
	faces kitFaces
}

// newTheme builds the Theme for a palette and type scale (the scale is
// already in pixels — see scaleTypeScale).
func NewSeam(p Palette, ts TypeScale) Theme {
	return &kitTheme{th: NewTheme(p, ts), pal: p, ts: ts, faces: NewFaces(ts)}
}

func (t *kitTheme) Face(step string) *text.Face {
	switch step {
	case StepTiny:
		return facePtr(t.faces.Tiny)
	case StepSmall:
		return facePtr(t.faces.Small)
	case StepLarge:
		return facePtr(t.faces.Large)
	default:
		return facePtr(t.faces.Base)
	}
}

func (t *kitTheme) FaceBold(step string) *text.Face {
	switch step {
	case StepTiny:
		return facePtr(t.faces.BaseBold) // no tiny emphasis: titles/readouts are small and up
	case StepSmall:
		return facePtr(t.faces.SmallBold)
	case StepLarge:
		return facePtr(t.faces.LargeBold)
	default:
		return facePtr(t.faces.BaseBold)
	}
}

func (t *kitTheme) FaceMono(step string) *text.Face {
	switch step {
	case StepTiny:
		return facePtr(render.MonoFace(t.ts.Tiny))
	case StepSmall:
		return facePtr(render.MonoFace(t.ts.Small))
	case StepLarge:
		return facePtr(render.MonoFace(t.ts.Large))
	default:
		return facePtr(render.MonoFace(t.ts.Base))
	}
}

func (t *kitTheme) Pal() Palette               { return t.pal }
func (t *kitTheme) TS() TypeScale              { return t.ts }
func (t *kitTheme) WidgetTheme() *widget.Theme { return t.th }
