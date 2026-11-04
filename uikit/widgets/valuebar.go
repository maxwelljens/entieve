package widgets

import (
	"image"
	"image/color"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
)

// BarH is the house value bar's height (design px): the HUD capacitor
// bar's band, so every "figure inside a bar" readout is the same strip.
const BarH = 18

// ValueBarMax is the bar's value resolution (0.1 % steps, the capacitor
// bar's 1000 steps).
const ValueBarMax = 1000

// ValueBar is the house bar-with-the-figure-inside: an ebitenui progress
// bar under a centred value label, both stacked in one widget. The HUD's
// capacitor and the inventory's hold fullness are this one construction
// (the capacitor-bar look); the fill is the caller's palette role.
type ValueBar struct {
	Bar       *widget.ProgressBar
	Value     *widget.Text
	Container *widget.Container
}

// NewValueBar builds the bar over the theme seam: fill is the palette
// role painted up to the value, minW its design-px floor (0 = the host's
// layout width). The HUD adds its glyph to Container after the pair.
func NewValueBar(th theme.Theme, fill color.RGBA, minW int) *ValueBar {
	p := th.Pal()
	b := &ValueBar{}
	b.Bar = widget.NewProgressBar(
		widget.ProgressBarOpts.Values(0, ValueBarMax, 0),
		widget.ProgressBarOpts.FillImage(&widget.ProgressBarImage{Idle: theme.NineSlice(theme.Solid(fill))}),
		widget.ProgressBarOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(minW), theme.Px(BarH))),
	)
	b.Value = widget.NewText(widget.TextOpts.Text("", th.Face(theme.StepTiny), theme.Solid(p.BGHard)),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter))
	b.Container = widget.NewContainer(widget.ContainerOpts.Layout(widget.NewStackedLayout()))
	b.Container.AddChild(b.Bar, b.Value)
	return b
}

// AddMark puts a widget between the bar and the value (the HUD's glyph),
// so the value text keeps drawing over it. The stack draws its children in
// order, and the value must stay last.
func (b *ValueBar) AddMark(w widget.PreferredSizeLocateableWidget) {
	b.Container.RemoveChild(b.Value)
	b.Container.AddChild(w, b.Value)
}

// Set paints frac (clamped 0..1) and its overlaid label.
func (b *ValueBar) Set(frac float64, label string) {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	b.Bar.SetCurrent(int(frac * ValueBarMax))
	b.Value.Label = label
}

// FillRect is the painted fill's rect: the bar inset by the theme's track
// padding, filled to the value (the progress bar's own math, truncated to
// whole pixels). Exposed for pins and hosts that need the value's extent,
// like Slider.ValueX.
func (b *ValueBar) FillRect() image.Rectangle {
	r := b.Bar.GetWidget().Rect
	pad := theme.Px(2) // theme.ProgressBarTheme.TrackPadding
	inner := image.Rect(r.Min.X+pad, r.Min.Y+pad, r.Max.X-pad, r.Max.Y-pad)
	if inner.Empty() || b.Bar.Max <= b.Bar.Min {
		return image.Rectangle{}
	}
	frac := float64(b.Bar.GetCurrent()-b.Bar.Min) / float64(b.Bar.Max-b.Bar.Min)
	return image.Rect(inner.Min.X, inner.Min.Y, inner.Min.X+int(float64(inner.Dx())*frac), inner.Max.Y)
}
