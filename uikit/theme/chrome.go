package theme

import (
	"image"
	"image/color"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Window-chrome constants and the shared draw plumbing every surface
// paints with (the title-bar geometry the title builders and panels
// share, the visibility flip, the raw fill/text helpers).

// Layout constants for the overview window chrome (derived from the
// legacy ui padding scale).
const (
	TitleBarH   = 44 // title line + breathing room (ui.TitleBarH at 21 px)
	ContentsPad = 8  // panel margins inside a window (the SpMd air: a real, visible margin on all four sides)
)

// WindowAlpha is the ONE translucency token: the alpha every window
// body fill draws at (~0.90), so windows read the world through a
// slight translucency. The title bar stays on the opaque BGHard, and
// non-window chrome (HUD, rail, rack) keeps the palette alpha — every
// window body funnels through WindowBody so the value stays uniform.
const WindowAlpha = 230 // 0.90 × 255

// WindowBody returns a window body fill color: the palette color at the
// WindowAlpha translucency token (pinned by
// TestWindowBodyTranslucencyToken; the drawn uniformity across windows
// by TestWindowFadeAndTranslucencyCapture).
func WindowBody(c color.RGBA) color.NRGBA { return NRGBA(c, WindowAlpha) }

// showWidget shows or hides w only on a state change (a visibility flip
// requests a parent relayout, so Sync must not spam it).
func ShowWidget(w *widget.Widget, show bool) {
	want := widget.Visibility_Show
	if !show {
		want = widget.Visibility_Hide
	}
	if w.GetVisibility() != want {
		w.SetVisibility(want)
	}
}

// fillRect fills r with a solid color.
func FillRect(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	vector.DrawFilledRect(dst,
		float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()),
		col, false)
}

// drawCardText draws s with its rendering region's top-left at (x, y) —
// text/v2's default alignment for left-to-right faces.
func DrawCardText(dst *ebiten.Image, s string, face *text.Face, x, y int, col color.Color) {
	if s == "" || face == nil {
		return
	}
	opts := &text.DrawOptions{}
	opts.GeoM.Translate(float64(x), float64(y))
	opts.ColorScale.ScaleWithColor(col)
	text.Draw(dst, s, *face, opts)
}
