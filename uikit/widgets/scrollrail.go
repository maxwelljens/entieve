package widgets

import (
	"math"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
)

// RailSteps is the rail's resolution: a scroll viewport's ScrollTop is a
// 0..1 fraction of the overflow, the slider an integer over it.
const RailSteps = 1000

// ScrollRail is the house scrollbar: a vertical slider in a reserved
// gutter beside a scroll viewport — the market order book's rail,
// generalised into one piece of chrome so no screen copy-pastes it. The
// block (usually the viewport itself, or a header+viewport root) fills
// the row beside a fixed gutter of theme.ScrollGutter design px, and the
// handle can never overhang the content ("the scrollbar is laid over
// content": the old 8 px market rail was narrower than the theme's 16 px
// handle). Sync mirrors the viewport's ScrollTop onto the rail and hides
// the RAIL while the content fits its viewport — the gutter itself stays,
// so the viewport never jumps when a long list starts overflowing.
type ScrollRail struct {
	root    *widget.Container
	bar     *widget.Slider
	scroll  *widget.ScrollContainer
	content widget.PreferredSizeLocateableWidget
}

// NewScrollRail wraps block in the reserved gutter and wires the rail to
// scroll. content is the scrollable content whose preferred height,
// measured against scroll's viewport, decides when the rail shows. The
// caller places Container() wherever block used to go and may set its
// LayoutData (a row layout needs Stretch; an anchor layout needs the
// both-stretch AnchorLayoutData).
func NewScrollRail(env Seams, block widget.PreferredSizeLocateableWidget, scroll *widget.ScrollContainer, content widget.PreferredSizeLocateableWidget) *ScrollRail {
	bar := widget.NewSlider(
		widget.SliderOpts.Orientation(widget.DirectionVertical),
		widget.SliderOpts.MinMax(0, RailSteps),
		widget.SliderOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				StretchHorizontal: true,
				StretchVertical:   true,
			}),
		),
		widget.SliderOpts.ChangedHandler(func(a *widget.SliderChangedEventArgs) {
			scroll.ScrollTop = float64(a.Current) / RailSteps
		}),
	)
	// The slider validates against a theme the moment anything measures
	// it — bind the session's widget theme here (the market rail's
	// convention) so the handle is sized before the first layout.
	bar.GetWidget().SetTheme(env.WidgetTheme())

	// The gutter is its own fixed-width column: hiding the rail (while the
	// content fits) leaves it reserved, so the block never widens or
	// narrows with the overflow state.
	gutter := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(theme.ScrollGutter), 0)),
	)
	gutter.AddChild(bar)

	// A two-column grid — [block][rail], block stretched — so the block
	// fills exactly the wrapper minus ScrollGutter and the wrapper's
	// preferred width still carries the gutter: a parent that sizes by
	// preferred (the rail's column) reserves it, and a parent that
	// stretches the wrapper (a window cell) gives the block the rest.
	root := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewGridLayout(
		widget.GridLayoutOpts.Columns(2),
		widget.GridLayoutOpts.Stretch([]bool{true, false}, []bool{true}),
	)))
	root.AddChild(block, gutter)
	return &ScrollRail{root: root, bar: bar, scroll: scroll, content: content}
}

// Container returns the wrapper to place in the block's slot.
func (r *ScrollRail) Container() widget.Containerer { return r.root }

// Bar returns the slider (drag write-back, and the geometry pins).
func (r *ScrollRail) Bar() *widget.Slider { return r.bar }

// Overflow is the scrollable distance in px: content height minus
// viewport height (≤ 0 exactly while the content fits).
func (r *ScrollRail) Overflow() float64 {
	_, h := r.content.PreferredSize()
	return float64(h) - float64(r.scroll.ViewRect().Dy())
}

// Sync mirrors the viewport offset onto the rail and hides the rail while
// the content fits (no overflow → nothing to drag). State writes only:
// the visibility flip is guarded, so a Sync per frame is cheap.
func (r *ScrollRail) Sync() {
	theme.ShowWidget(r.bar.GetWidget(), r.Overflow() > 0)
	if cur := int(math.Round(r.scroll.ScrollTop * RailSteps)); r.bar.Current != cur {
		r.bar.Current = cur
	}
}
