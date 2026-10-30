package widgets

import (
	"image"
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// The widget roster had no tick box (columns, table, slider, menu, split
// pane, tooltip, plain table, rowspec, scroll rail — no checkbox): the
// overview's per-kind filters are the first caller, so the element is new
// here and these tests pin the behaviour it was built for.

// newPumpedCheckBox mounts a checkbox in a one-cell grid inside a window
// (the only place it runs: the press asks the widget's input layer) and
// pumps a frame at 640×480 so the rect is laid out.
func newPumpedCheckBox(t *testing.T, opts CheckBoxOpts) (*CheckBox, *ebitenui.UI) {
	t.Helper()
	env, _ := newTestEnv(t)
	cb := NewCheckBox(env, opts)
	grid := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{false}, []bool{false}),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(240), 0)),
	)
	grid.AddChild(cb)
	win := widget.NewWindow(
		widget.WindowOpts.Contents(grid),
		widget.WindowOpts.TitleBar(
			widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout())),
			theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(260), theme.Px(80)),
		widget.WindowOpts.Location(image.Rect(0, 0, 280, 100)),
	)
	ui := newWindowUI(t, env, win)
	uitest.PumpWindow(t, ui, nil, 640, 480)
	return cb, ui
}

// rowEnd is a point on the row's far right — past the marker, over the
// label — so a press there proves the WHOLE row is the click target.
func rowEnd(cb *CheckBox) image.Point {
	r := cb.GetWidget().Rect
	return image.Pt(r.Max.X-2, r.Min.Y+r.Dy()/2)
}

// clickCheckBox runs one press-and-release gesture at at.
func clickCheckBox(cb *CheckBox, at image.Point) {
	cb.Step(at, true)
	cb.Step(at, false)
}

// TestCheckBoxTogglesOnRowClick: a click anywhere on the row flips the
// state exactly once and reports it; a held button does not repeat, and a
// press below the row is inert.
func TestCheckBoxTogglesOnRowClick(t *testing.T) {
	var changes []bool
	cb, _ := newPumpedCheckBox(t, CheckBoxOpts{
		Label:    "Visible",
		OnChange: func(v bool) { changes = append(changes, v) },
	})
	if cb.Checked() {
		t.Fatal("a checkbox built unchecked reports checked")
	}

	clickCheckBox(cb, rowEnd(cb))
	if !cb.Checked() {
		t.Fatal("a click on the row's label did not check the box")
	}
	if len(changes) != 1 || changes[0] != true {
		t.Fatalf("OnChange = %v, want exactly [true]", changes)
	}

	// A held button (no fresh edge) never re-fires.
	cb.Step(rowEnd(cb), false)
	if len(changes) != 1 {
		t.Errorf("a held press fired OnChange again (%v)", changes)
	}

	clickCheckBox(cb, rowEnd(cb))
	if cb.Checked() {
		t.Fatal("a second click did not uncheck the box")
	}
	if len(changes) != 2 || changes[1] != false {
		t.Fatalf("OnChange = %v, want [true false]", changes)
	}

	r := cb.GetWidget().Rect
	clickCheckBox(cb, image.Pt(r.Min.X+1, r.Max.Y+theme.Px(4)))
	if cb.Checked() {
		t.Error("a press below the row toggled the box")
	}
	if len(changes) != 2 {
		t.Errorf("a press outside the row fired OnChange (%v)", changes)
	}
}

// TestCheckBoxSetCheckedIsSilent: SetChecked is the host's mirror — it
// round-trips the state and never calls OnChange (a stored value must not
// echo back as an edit).
func TestCheckBoxSetCheckedIsSilent(t *testing.T) {
	fired := 0
	cb, _ := newPumpedCheckBox(t, CheckBoxOpts{
		Label: "Colour", Checked: true,
		OnChange: func(bool) { fired++ },
	})
	if !cb.Checked() {
		t.Fatal("a checkbox built checked reports unchecked")
	}
	cb.SetChecked(false)
	if cb.Checked() {
		t.Error("SetChecked(false) left the box checked")
	}
	cb.SetChecked(true)
	if !cb.Checked() {
		t.Error("SetChecked(true) left the box unchecked")
	}
	if fired != 0 {
		t.Errorf("SetChecked fired OnChange %d times, want 0", fired)
	}
}

// TestCheckBoxRendersBothStates: the marker is on the row's render path at
// both states — laid out as a 16 design-px square inside the row and clear
// of the label — and Render paints each state (pixel readback needs a live
// ebiten loop, which is the capture suite's job, not this one's).
func TestCheckBoxRendersBothStates(t *testing.T) {
	cb, ui := newPumpedCheckBox(t, CheckBoxOpts{Label: "Star"})
	row, box := cb.GetWidget().Rect, cb.mark.GetWidget().Rect
	if !box.In(row) {
		t.Fatalf("marker %v is not inside its row %v", box, row)
	}
	if box.Dx() != theme.Px(checkBoxSize) || box.Dy() != theme.Px(checkBoxSize) {
		t.Errorf("marker = %v, want a %d² design box", box, checkBoxSize)
	}
	if label := cb.label.GetWidget().Rect; box.Overlaps(label) {
		t.Errorf("marker %v overlaps its label %v", box, label)
	}
	for _, checked := range []bool{false, true} {
		cb.SetChecked(checked)
		pumpFrame(ui) // the container's Render walks the mark at this state
	}
}

// pumpFrame runs one frame and returns the drawn image.
func pumpFrame(ui *ebitenui.UI) *ebiten.Image {
	img := ebiten.NewImage(640, 480)
	ui.Update()
	ui.Draw(img)
	return img
}
