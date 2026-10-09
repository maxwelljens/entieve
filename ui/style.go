package ui

import (
	"image/color"

	"github.com/entieve/entieve/render"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// EVE-dark chrome colors.
var (
	ColBG     = mustHex("#0a0e14ee") // near-black translucent panels
	ColBGHard = mustHex("#0a0e14")   // opaque variant for layering
	ColBorder = mustHex("#3a4452")   // thin panel borders
	ColAccent = mustHex("#ffb454")   // amber accents / selection
	ColText   = mustHex("#d8dee9")   // primary text
	ColDim    = mustHex("#7a8494")   // secondary text
	ColHead   = mustHex("#5b677a")   // table headers
	ColGood   = mustHex("#5fcf80")   // positive / shield-ish green
	ColWarn   = mustHex("#e0a33c")   // warnings / armor amber
	ColDanger = mustHex("#e05555")   // hostile red
	ColInfo   = mustHex("#8fb8e0")   // info blue
	ColShield = mustHex("#6fa8ff")   // shield blue
	ColStruct = mustHex("#c9ced6")   // structure grey
	ColCell   = mustHex("#141a24")   // module cell background
	ColCap    = mustHex("#5fcf80")   // capacitor green
	ColPlayer = mustHex("#7fe3ff")   // the player's own ship
)

// mustHex parses a palette color; invalid constants would be a build bug.
func mustHex(s string) color.RGBA {
	c, err := render.ParseHex(s)
	if err != nil {
		panic("ui: bad color constant " + s)
	}
	return c
}

// withAlpha returns c with replaced alpha.
func withAlpha(c color.RGBA, a uint8) color.RGBA {
	c.A = a
	return c
}

// Panel draws a window: translucent fill plus thin border.
func Panel(dst *ebiten.Image, r Rect) {
	vector.DrawFilledRect(dst, float32(r.X), float32(r.Y), float32(r.W), float32(r.H),
		ColBG, false)
	vector.StrokeRect(dst, float32(r.X)+0.5, float32(r.Y)+0.5, float32(r.W)-1, float32(r.H)-1,
		1, ColBorder, false)
}

// PanelTitle draws a window title with an amber underline.
func PanelTitle(dst *ebiten.Image, r Rect, title string) {
	render.Text(dst, title, r.X+7, r.Y+5, 12, ColAccent)
	vector.StrokeLine(dst, float32(r.X+1), float32(r.Y+20), float32(r.Right()-1), float32(r.Y+20),
		1, withAlpha(ColBorder, 200), false)
}

// Button draws a small chrome button.
func Button(dst *ebiten.Image, r Rect, label string, enabled bool) {
	bg, fg := ColCell, ColText
	if !enabled {
		bg = withAlpha(ColCell, 140)
		fg = withAlpha(ColDim, 160)
	}
	vector.DrawFilledRect(dst, float32(r.X), float32(r.Y), float32(r.W), float32(r.H), bg, false)
	vector.StrokeRect(dst, float32(r.X)+0.5, float32(r.Y)+0.5, float32(r.W)-1, float32(r.H)-1,
		1, withAlpha(ColBorder, 220), false)
	render.TextCentered(dst, label, r.X+r.W/2, r.Y+(r.H-11)/2, 12, fg)
}

// Text draws a UI string at (x, y) top-left.
func Text(dst *ebiten.Image, s string, x, y, size float64, col color.RGBA) {
	render.Text(dst, s, x, y, size, col)
}

// TextRight draws a string right-aligned to x.
func TextRight(dst *ebiten.Image, s string, x, y, size float64, col color.RGBA) {
	render.Text(dst, s, x-render.Measure(s, size), y, size, col)
}

// TextCentered draws a string horizontally centered on x.
func TextCentered(dst *ebiten.Image, s string, x, y, size float64, col color.RGBA) {
	render.TextCentered(dst, s, x, y, size, col)
}

// Meter draws a horizontal fraction bar (track + fill).
func Meter(dst *ebiten.Image, r Rect, frac float64, fill color.RGBA) {
	frac = clamp(frac, 0, 1)
	vector.DrawFilledRect(dst, float32(r.X), float32(r.Y), float32(r.W), float32(r.H),
		withAlpha(ColCell, 230), false)
	if frac > 0 {
		vector.DrawFilledRect(dst, float32(r.X), float32(r.Y), float32(r.W*frac), float32(r.H),
			fill, false)
	}
}
