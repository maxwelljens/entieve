package ui

import (
	"image/color"
	"math"

	"github.com/entieve/entieve/render"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// ---------------------------------------------------------------------------
// Typography — the single source of truth for every text size in the UI.
// FontBase is 1.75× the legacy 12 px base (text is ~75% larger everywhere);
// everything else derives from it. render mirrors the same scale for its
// in-world name/distance labels (render.FontScale) — pinned together by
// TestFontScaleMatchesRender (render cannot import ui).
// ---------------------------------------------------------------------------
const (
	FontBase  = 21.0           // default UI text (≈ 1.75× the old 12)
	FontSmall = 0.8 * FontBase // 16.8 — table/body text (old 10/11)
	FontLarge = 1.3 * FontBase // 27.3 — headings/emphasis (old 13/14)
	FontTiny  = 0.7 * FontBase // 14.7 — captions, F-key labels (old 9)
)

// ---------------------------------------------------------------------------
// Padding — every spacing value derives from these (≈1.2× the legacy
// hand-tuned 8/4/20 px values).
// ---------------------------------------------------------------------------
const (
	padSmall = 5.0  // inner gaps (≈1.2 × old 4)
	pad      = 10.0 // panel margins (≈1.2 × old 8)
	padLarge = 24.0 // section gaps (≈1.2 × old 20)
)

// minHit is the minimum hit area of any full-size interactive control (px).
const minHit = 40.0

// LineH returns the line height (text rhythm) for a font size.
func LineH(size float64) float64 { return math.Ceil(size * 1.35) }

// lineH is the package-internal shorthand.
func lineH(size float64) float64 { return LineH(size) }

// titleBarH is the height of every window title bar: the title line plus
// breathing room; also the minimum drag strip and close-button host.
var titleBarH = LineH(FontBase) + 3*padSmall // 44

// TitleBarH is the exported title-bar height for game-side overlays that
// draw their own chrome through DrawWindow.
func TitleBarH() float64 { return titleBarH }

// ---------------------------------------------------------------------------
// EVE-dark chrome colors.
// ---------------------------------------------------------------------------
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
	ColHover  = mustHex("#ffb454")   // hover tint (drawn translucent)
	ColHeadBg = mustHex("#131926")   // table header strip
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

// ---------------------------------------------------------------------------
// Primitives
// ---------------------------------------------------------------------------

// shadowRects returns the layered drop-shadow rects beneath a window:
// 1–3 px translucent black offsets for depth instead of a hard border.
func shadowRects(r Rect) []Rect {
	return []Rect{
		{X: r.X + 3, Y: r.Y + 3, W: r.W, H: r.H},
		{X: r.X + 2, Y: r.Y + 2, W: r.W, H: r.H},
		{X: r.X + 1, Y: r.Y + 1, W: r.W, H: r.H},
	}
}

// Shadow draws the soft layered drop shadow beneath a panel rect.
func Shadow(dst *ebiten.Image, r Rect) {
	for i, sr := range shadowRects(r) {
		a := uint8(28 + 22*i) // 28/50/72, softest furthest out
		vector.DrawFilledRect(dst, float32(sr.X), float32(sr.Y), float32(sr.W), float32(sr.H),
			withAlpha(color.RGBA{0, 0, 0, 255}, a), false)
	}
}

// Panel draws a window: layered drop shadow, translucent fill and the thin
// border.
func Panel(dst *ebiten.Image, r Rect) {
	Shadow(dst, r)
	vector.DrawFilledRect(dst, float32(r.X), float32(r.Y), float32(r.W), float32(r.H),
		ColBG, false)
	vector.StrokeRect(dst, float32(r.X)+0.5, float32(r.Y)+0.5, float32(r.W)-1, float32(r.H)-1,
		1, ColBorder, false)
}

// CloseButtonRect returns the [×] hit rect in a window's title bar.
// The hit area is minHit×minHit (40×40).
func CloseButtonRect(r Rect) Rect {
	return Rect{X: r.Right() - minHit - padSmall, Y: r.Y + (titleBarH-minHit)/2, W: minHit, H: minHit}
}

// TitleBarRect returns the draggable title-bar strip of a window.
func TitleBarRect(r Rect) Rect { return Rect{X: r.X, Y: r.Y, W: r.W, H: titleBarH} }

// DrawWindow renders full window chrome — layered shadow, panel fill,
// title bar (title + underline) and, when closable, the [×] button — and
// returns the content rect below the title bar.
func DrawWindow(dst *ebiten.Image, r Rect, title string, closable bool) Rect {
	Panel(dst, r)
	ty := r.Y + (titleBarH-LineH(FontBase))/2
	Text(dst, title, r.X+pad, ty, FontBase, ColAccent)
	vector.StrokeLine(dst, float32(r.X+1), float32(r.Y+titleBarH-1), float32(r.Right()-1),
		float32(r.Y+titleBarH-1), 1, withAlpha(ColBorder, 200), false)
	if closable {
		CloseButton(dst, r, false, false)
	}
	return Rect{X: r.X + 1, Y: r.Y + titleBarH, W: r.W - 2, H: r.H - titleBarH - 1}
}

// CloseButton draws the [×] close affordance of a window title bar.
func CloseButton(dst *ebiten.Image, r Rect, hover, pressed bool) {
	cr := CloseButtonRect(r)
	bg := withAlpha(ColCell, 0)
	fg := ColDim
	switch {
	case pressed:
		bg = withAlpha(ColDanger, 70)
		fg = ColText
	case hover:
		bg = withAlpha(ColDanger, 40)
		fg = ColText
	}
	if bg.A > 0 {
		vector.DrawFilledRect(dst, float32(cr.X), float32(cr.Y), float32(cr.W), float32(cr.H), bg, false)
	}
	TextCentered(dst, "×", cr.X+cr.W/2, cr.Y+(cr.H-LineH(FontBase))/2, FontBase, fg)
}

// PanelTitle draws a bare window title with an amber underline (chrome-less
// legacy panels; prefer DrawWindow).
func PanelTitle(dst *ebiten.Image, r Rect, title string) {
	ty := r.Y + (titleBarH-LineH(FontBase))/2
	Text(dst, title, r.X+pad, ty, FontBase, ColAccent)
	vector.StrokeLine(dst, float32(r.X+1), float32(r.Y+titleBarH-1), float32(r.Right()-1),
		float32(r.Y+titleBarH-1), 1, withAlpha(ColBorder, 200), false)
}

// Button draws a chrome button (neutral state).
func Button(dst *ebiten.Image, r Rect, label string, enabled bool) {
	ButtonState(dst, r, label, enabled, false, false)
}

// ButtonState draws a chrome button with hover tint and pressed inset
// feedback; disabled buttons dim and ignore interaction visually.
func ButtonState(dst *ebiten.Image, r Rect, label string, enabled, hover, pressed bool) {
	bg, fg := ColCell, ColText
	switch {
	case !enabled:
		bg = withAlpha(ColCell, 140)
		fg = withAlpha(ColDim, 160)
	case pressed:
		bg = withAlpha(ColHover, 30) // dimmed fill while held
		fg = ColText
	case hover:
		bg = withAlpha(ColHover, 18) // subtle accent tint
	}
	vector.DrawFilledRect(dst, float32(r.X), float32(r.Y), float32(r.W), float32(r.H), bg, false)
	if pressed && enabled {
		// Inset ring: pressed feel without scaling layout.
		vector.StrokeRect(dst, float32(r.X)+1.5, float32(r.Y)+1.5, float32(r.W)-3, float32(r.H)-3,
			1, withAlpha(ColBGHard, 160), false)
	}
	border := withAlpha(ColBorder, 220)
	if enabled && hover {
		border = withAlpha(ColAccent, 200)
	}
	if !enabled {
		border = withAlpha(ColBorder, 110)
	}
	vector.StrokeRect(dst, float32(r.X)+0.5, float32(r.Y)+0.5, float32(r.W)-1, float32(r.H)-1,
		1, border, false)
	// Button labels use FontSmall: at the 40 px minimum hit area they stay
	// legible, and the longest action label ("Approach") still fits the
	// narrowest column without ellipsizing.
	TextCentered(dst, label, r.X+r.W/2, r.Y+(r.H-LineH(FontSmall))/2, FontSmall, fg)
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

// NumberAnchor returns the fixed right-alignment anchor for a numeric
// column. All number columns in the overview/HUD/market draw through this,
// so digits change width but the column edge never shifts horizontally.
func NumberAnchor(col Rect) float64 { return col.Right() - padSmall }

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

// hoverFill returns the subtle accent tint used for hovered rows/cells.
func hoverFill(alpha uint8) color.RGBA { return withAlpha(ColHover, alpha) }

// drawSortArrow draws the ▲/▼ active-sort indicator (and other carets)
// as a small triangle — gofont has no reliable glyph for it.
func drawSortArrow(dst *ebiten.Image, cx, cy, size float64, up bool, col color.RGBA) {
	var p vector.Path
	if up {
		p.MoveTo(float32(cx), float32(cy-size/2))
		p.LineTo(float32(cx+size/2), float32(cy+size/2))
		p.LineTo(float32(cx-size/2), float32(cy+size/2))
	} else {
		p.MoveTo(float32(cx), float32(cy+size/2))
		p.LineTo(float32(cx+size/2), float32(cy-size/2))
		p.LineTo(float32(cx-size/2), float32(cy-size/2))
	}
	p.Close()
	var cs ebiten.ColorScale
	cs.Scale(float32(col.R)/255, float32(col.G)/255,
		float32(col.B)/255, float32(col.A)/255)
	vector.FillPath(dst, &p, nil, &vector.DrawPathOptions{AntiAlias: true, ColorScale: cs})
}
