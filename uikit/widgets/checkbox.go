package widgets

import (
	"image"

	"github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// CheckBox geometry (design units, scaled by Px): a 16 px marker beside its
// label on one row. The row height is the label's, so a checkbox composes in
// a grid row beside any text.
const (
	checkBoxSize = 16
	checkBoxGap  = 6
)

// CheckBoxOpts configures a CheckBox. Checked is the initial state; OnChange
// fires on every click that flips it (never on SetChecked).
type CheckBoxOpts struct {
	Label    string
	Checked  bool
	OnChange func(checked bool)
}

// CheckBox is the house tick box — the owner's "ticker box": a square marker
// and a label on one row, toggled by a click anywhere on the row. The
// palette carries both states (Border edge, Cell fill, the Accent tick the
// context menu's checked rows already draw), so a theme-only caller needs
// nothing else.
//
// It embeds its row container, so any layout takes it as a widget child
// directly (a wrapper struct would hide the Renderer and the layout would
// never descend into the row).
//
// Input is the Slider's discipline: the panel calls Poll from Sync and Step
// is the testable core. A press only counts on this widget's own active
// input layer, so a checkbox under another window is inert.
type CheckBox struct {
	*widget.Container
	th    theme.Theme
	mark  *checkMark
	label *widget.Text

	checked  bool
	hover    bool
	onChange func(bool)
}

// NewCheckBox builds a checkbox over the theme seam.
func NewCheckBox(th theme.Theme, opts CheckBoxOpts) *CheckBox {
	c := &CheckBox{th: th, checked: opts.Checked, onChange: opts.OnChange}
	c.mark = newCheckMark(th, c.checked)
	c.label = widget.NewText(
		widget.TextOpts.Text(opts.Label, th.Face(theme.StepSmall), theme.Solid(th.Pal().Text)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
			Position: widget.RowLayoutPositionCenter,
		})),
	)
	c.Container = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(checkBoxGap)),
		)),
		// The default slot: centred in a grid cell's row; a caller with its
		// own placement overrides GetWidget().LayoutData afterwards.
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.GridLayoutData{
			VerticalPosition: widget.GridLayoutPositionCenter,
		})),
	)
	c.AddChild(c.mark, c.label)
	return c
}

// Checked is the current state.
func (c *CheckBox) Checked() bool { return c.checked }

// SetChecked sets the state programmatically — no callback (the host's own
// value never echoes back as an edit).
func (c *CheckBox) SetChecked(v bool) {
	c.checked = v
	c.mark.checked = v
}

// Poll advances one frame of input from the frame's pointer and the left
// button. The panel calls it from Sync, before Kit.Update.
func (c *CheckBox) Poll(p *Pointer) { c.Step(p.At(), p.Press(Left)) }

// Step is Poll's testable core: hover tracks the cursor, and the rising edge
// of a press anywhere on the row flips the state once and fires OnChange. A
// press outside the row, or a held button, does nothing.
func (c *CheckBox) Step(cursor image.Point, rising bool) {
	c.hover = c.hit(cursor)
	c.mark.hover = c.hover
	if !rising || !c.hover {
		return
	}
	c.SetChecked(!c.checked)
	if c.onChange != nil {
		c.onChange(c.checked)
	}
}

// hit reports whether p lands on the checkbox row: inside its rect and on
// the widget's own active input layer (the Slider.hit idiom).
func (c *CheckBox) hit(p image.Point) bool {
	w := c.GetWidget()
	if !w.IsVisible() || !p.In(w.Rect) {
		return false
	}
	return OwnsAt(w, p, input.LayerEventTypeMouseButton)
}

// checkMark is the square marker: a Renderer child of the row, so the row
// lays it out and the container paints it after the layout pass.
type checkMark struct {
	th      theme.Theme
	w       *widget.Widget
	checked bool
	hover   bool
}

func newCheckMark(th theme.Theme, checked bool) *checkMark {
	m := &checkMark{th: th, checked: checked}
	m.w = widget.NewWidget(
		widget.WidgetOpts.MinSize(theme.Px(checkBoxSize), theme.Px(checkBoxSize)),
		widget.WidgetOpts.LayoutData(widget.RowLayoutData{Position: widget.RowLayoutPositionCenter}),
	)
	return m
}

func (m *checkMark) GetWidget() *widget.Widget     { return m.w }
func (m *checkMark) SetLocation(r image.Rectangle) { m.w.Rect = r }
func (m *checkMark) PreferredSize() (int, int) {
	return theme.Px(checkBoxSize), theme.Px(checkBoxSize)
}
func (m *checkMark) Validate() {}

// Render paints the marker: a Cell box edged in Border (Accent while
// hovered) and, when checked, the context menu's two-stroke Accent tick.
func (m *checkMark) Render(dst *ebiten.Image) {
	r := m.w.Rect
	if r.Empty() {
		return
	}
	p := m.th.Pal()
	edge := p.Border
	if m.hover {
		edge = p.Accent
	}
	theme.DrawRoundBox(dst, r, theme.Solid(p.Cell), theme.Solid(edge), theme.Px(1), theme.AllRadii(theme.RadiusControl))
	if !m.checked {
		return
	}
	s := float32(theme.Scale())
	x := float32(r.Min.X) + 4*s
	y := float32(r.Min.Y+r.Dy()/2) - s
	tick := theme.Solid(p.Accent)
	vector.StrokeLine(dst, x, y, x+3*s, y+3*s, s, tick, true)
	vector.StrokeLine(dst, x+3*s, y+3*s, x+10*s, y-4*s, s, tick, true)
}
