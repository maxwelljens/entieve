package theme

import (
	"image"
	"image/color"
	"math"
	"unicode/utf8"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// Chrome labels — window titles, panel heads and section heads — are set in
// Plex Mono with a letter-tracking step (the Cyber Terminal kit's terminal
// register). Body, row and list text stays Plex Sans without tracking.
//
// text/v2's Face interface is closed (unexported methods), so tracking
// cannot ride a face wrapper: these helpers draw and measure glyph by
// glyph.

// trackEm is the tracking step as a fraction of the type size — the kit's
// 0.10–0.18 em label range at its tighter end, so the dense chrome and the
// geometry tests pinned to the sans layout stay close.
const trackEm = 0.12

// TrackStep is the tracking gap in pixels for a face at size px.
func TrackStep(size float64) float64 { return math.Round(size*trackEm*100) / 100 }

// MeasureTracked measures s with a tracking step of TrackStep(size) between
// glyphs.
func MeasureTracked(s string, face text.Face, size float64) float64 {
	if s == "" {
		return 0
	}
	w := text.Advance(s, face)
	if n := utf8.RuneCountInString(s); n > 1 {
		w += float64(n-1) * TrackStep(size)
	}
	return w
}

// DrawTracked draws s with its region's top-left at (x, y), one glyph at a
// time with the tracking step between advances — the same origin convention
// as DrawCardText.
func DrawTracked(dst *ebiten.Image, s string, face text.Face, size float64, x, y float64, col color.Color) {
	if s == "" || face == nil {
		return
	}
	step := TrackStep(size)
	opts := &text.DrawOptions{}
	opts.GeoM.Translate(x, y)
	opts.ColorScale.ScaleWithColor(col)
	for _, r := range s {
		text.Draw(dst, string(r), face, opts)
		opts.GeoM.Translate(text.Advance(string(r), face)+step, 0)
	}
}

// ChromeText is a tracked mono label widget (the chrome register). It
// mirrors widget.Text's Label field so retitling call sites keep working;
// a changed Label asks for a relayout on the next Update.
type ChromeText struct {
	Label string

	w    *widget.Widget
	face text.Face
	size float64
	col  color.RGBA
	laid string // the label the parent laid out last
}

// NewChromeText builds a tracked chrome label. step names a type-scale step
// (base for window titles and panel heads, small for section heads).
func NewChromeText(d Theme, step, label string, col color.RGBA) *ChromeText {
	t := &ChromeText{Label: label, laid: label, face: *d.FaceMono(step), size: d.TS().Base, col: col}
	switch step {
	case StepTiny:
		t.size = d.TS().Tiny
	case StepSmall:
		t.size = d.TS().Small
	case StepLarge:
		t.size = d.TS().Large
	}
	t.w = widget.NewWidget()
	return t
}

// SetColor recolours the label.
func (t *ChromeText) SetColor(c color.RGBA) { t.col = c }

func (t *ChromeText) GetWidget() *widget.Widget { return t.w }

func (t *ChromeText) Validate() {}

func (t *ChromeText) SetLocation(r image.Rectangle) { t.w.Rect = r }

func (t *ChromeText) PreferredSize() (int, int) {
	m := t.face.Metrics()
	return int(math.Ceil(MeasureTracked(t.Label, t.face, t.size))),
		int(math.Ceil(m.HAscent + m.HDescent))
}

func (t *ChromeText) Render(dst *ebiten.Image) {
	if !t.w.IsVisible() {
		return
	}
	DrawTracked(dst, t.Label, t.face, t.size,
		float64(t.w.Rect.Min.X), float64(t.w.Rect.Min.Y), t.col)
}

func (t *ChromeText) Update(upd *widget.UpdateObject) {
	t.w.Update(upd)
	if t.Label != t.laid {
		t.laid = t.Label
		upd.RelayoutRequested = true
	}
}
