package widgets

import (
	"slices"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
)

// QtyStepper is the house ± quantity control: the − and + hit boxes, the
// tabular readout between them, and the row the caller's action buttons
// join (Row().AddChild). The value is steps[idx]; SetSteps re-points the
// ladder and clamps the index, so a screen whose ladder depends on the
// selection (the refinery's whole ore batches) only supplies the steps.
type QtyStepper struct {
	row   *widget.Container
	lbl   *widget.Text
	minus *widget.Button
	plus  *widget.Button
	steps []int
	idx   int
}

// NewQtyStepper builds the control over an ascending ladder of values.
func NewQtyStepper(th theme.Theme, steps []int) *QtyStepper {
	s := &QtyStepper{steps: steps}
	p := th.Pal()
	s.lbl = widget.NewText(
		widget.TextOpts.Text(s.readout(), th.FaceMono(theme.StepSmall), theme.Solid(p.Accent)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
			Position: widget.RowLayoutPositionCenter,
		})),
	)
	newBtn := func(label string, clicked func()) *widget.Button {
		return widget.NewButton(
			widget.ButtonOpts.TextLabel(label),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(theme.Px(40), theme.Px(40)),
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
			widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) { clicked() }),
		)
	}
	s.minus = newBtn("-", func() { s.Step(-1) })
	s.plus = newBtn("+", func() { s.Step(1) })
	s.row = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.ContentsPad)),
		)),
	)
	s.row.AddChild(s.minus, s.lbl, s.plus)
	return s
}

// Row is the stepper's row, for the caller's action buttons and layout.
func (s *QtyStepper) Row() *widget.Container { return s.row }

// Value is the current rung's quantity (0 for an empty ladder).
func (s *QtyStepper) Value() int {
	if len(s.steps) == 0 {
		return 0
	}
	return s.steps[s.idx]
}

// Index is the current rung (0 for an empty ladder) — a caller that maps
// the rung onto per-row stock (the refinery's batch preview) reads it.
func (s *QtyStepper) Index() int { return s.idx }

// Step moves one rung, clamped to the ladder.
func (s *QtyStepper) Step(delta int) {
	s.idx = min(max(s.idx+delta, 0), max(0, len(s.steps)-1))
	s.relabel()
}

// SetSteps re-points the ladder and clamps the index to it.
func (s *QtyStepper) SetSteps(steps []int) {
	if slices.Equal(s.steps, steps) {
		return
	}
	s.steps = steps
	s.idx = min(s.idx, max(0, len(steps)-1))
	s.relabel()
}

// Minus, Plus and Label expose the parts for layout and hit-box tests.
func (s *QtyStepper) Minus() *widget.Button { return s.minus }
func (s *QtyStepper) Plus() *widget.Button  { return s.plus }
func (s *QtyStepper) Label() *widget.Text   { return s.lbl }

// readout is the label text for the current value.
func (s *QtyStepper) readout() string { return "Qty " + theme.FormatInt(int64(s.Value())) }

// relabel rewrites the readout and relays the row out. widget.Text.Label
// changes do NOT invalidate layout, so without the relayout the readout
// keeps the box measured for its previous value and a wider number draws
// under the + button: the readout must resize to fit its current value,
// so the row re-measures it (the label's box grows and the buttons slide
// right). The label carries the tabular contents face, so digit widths
// stay stable while the value steps.
func (s *QtyStepper) relabel() {
	if text := s.readout(); text != s.lbl.Label {
		s.lbl.Label = text
		s.row.RequestRelayout()
	}
}
