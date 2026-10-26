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
//
// Dynamic window titles are deliberately unused as of the static-title
// change — every window passes its base display name — but the mechanism
// stays: a caller may still set Label and the label follows. The drawn
// label is ellipsized to the title bar's safe width (Shown), so a long title
// can never run under the × or out of the bar.
type ChromeText struct {
	Label string

	w    *widget.Widget
	face text.Face
	size float64
	col  color.RGBA
	laid string // the label the parent laid out last

	// bar is the title bar this label sits in (nil for a panel head): the
	// bound Shown ellipsizes against.
	bar *widget.Container
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

// Ellipsize shortens s with a trailing "…" so the result measures at most
// w, using measure (which must be monotonic in string length). An empty
// string is returned when even the ellipsis does not fit. The one
// implementation: widgets.Ellipsize delegates here (theme <- widgets).
func Ellipsize(s string, w float64, measure func(string) float64) string {
	if measure(s) <= w {
		return s
	}
	const dots = "…"
	if measure(dots) > w {
		return ""
	}
	runes := []rune(s)
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if measure(string(runes[:mid])+dots) <= w {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo == 0 {
		return dots
	}
	return string(runes[:lo]) + dots
}

// Shown is the label as drawn: the whole Label while it fits, its ellipsized
// form when the title bar is too narrow (a dynamic title must never overflow
// the bar or run under the ×).
func (t *ChromeText) Shown() string {
	w, bound := t.room()
	if !bound {
		return t.Label
	}
	return Ellipsize(t.Label, max(w, 0), func(s string) float64 { return MeasureTracked(s, t.face, t.size) })
}

// room is the width the label may use, from its own left edge: the bar's
// right edge, or the left edge of the bar's end-anchored chrome (the ×)
// when that comes first, less one hairline gap. bound is false when there is
// nothing to bound against (a panel head outside a title bar, or a bar that
// has not been laid out yet); the width itself may be negative when the bar
// is narrower than the label's own lead-in, which draws the bare ellipsis.
func (t *ChromeText) room() (float64, bool) {
	if t.bar == nil || t.w.Rect.Empty() || t.bar.GetWidget().Rect.Empty() {
		return 0, false
	}
	right := t.bar.GetWidget().Rect.Max.X
	for _, ch := range t.bar.Children() {
		cw := ch.GetWidget()
		if cw.Rect.Empty() {
			continue
		}
		if a, ok := cw.LayoutData.(widget.AnchorLayoutData); ok &&
			a.HorizontalPosition == widget.AnchorLayoutPositionEnd {
			right = min(right, cw.Rect.Min.X)
		}
	}
	return float64(right - t.w.Rect.Min.X - Px(SpSm)), true
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
	DrawTracked(dst, t.Shown(), t.face, t.size,
		float64(t.w.Rect.Min.X), float64(t.w.Rect.Min.Y), t.col)
}

func (t *ChromeText) Update(upd *widget.UpdateObject) {
	t.w.Update(upd)
	if t.Label != t.laid {
		t.laid = t.Label
		upd.RelayoutRequested = true
	}
}
