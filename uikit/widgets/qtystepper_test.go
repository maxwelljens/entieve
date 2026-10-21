package widgets

import (
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// qtystepperEnv builds the control in its own UI and pumps one frame so
// the row lays out.
func qtystepperEnv(t *testing.T, steps []int) (*QtyStepper, *ebitenui.UI, theme.Theme) {
	t.Helper()
	th := theme.NewSeam(theme.DefaultPalette(), theme.DefaultTypeScale())
	s := NewQtyStepper(th, steps)
	ui := &ebitenui.UI{Container: s.Row(), PrimaryTheme: th.WidgetTheme()}
	pump(ui)
	return s, ui, th
}

func pump(ui *ebitenui.UI) {
	ui.Update()
	ui.Draw(ebiten.NewImage(640, 120))
}

// TestQtyStepperSteps: the value is the rung, ± clamps, and the readout
// tracks it.
func TestQtyStepperSteps(t *testing.T) {
	s, ui, _ := qtystepperEnv(t, []int{1, 10, 100})
	if s.Value() != 1 {
		t.Fatalf("fresh value = %d, want 1", s.Value())
	}
	s.Step(-1)
	if s.Value() != 1 {
		t.Errorf("step below the ladder = %d, want the 1 floor", s.Value())
	}
	s.Step(1)
	if s.Value() != 10 || s.Label().Label != "Qty 10" {
		t.Errorf("after +: value %d readout %q, want 10 / Qty 10", s.Value(), s.Label().Label)
	}
	s.Step(5)
	if s.Value() != 100 {
		t.Errorf("step over the ladder = %d, want the 100 cap", s.Value())
	}
	// The buttons drive the same step.
	uitest.FireButton(ui, s.Minus())
	if s.Value() != 10 {
		t.Errorf("minus button: value %d, want 10", s.Value())
	}
}

// TestQtyStepperSetSteps: re-pointing the ladder clamps the rung (the
// refinery's selected ore shrinks) and an empty ladder reads 0.
func TestQtyStepperSetSteps(t *testing.T) {
	s, _, _ := qtystepperEnv(t, []int{100, 200, 300})
	s.Step(2)
	if s.Value() != 300 {
		t.Fatalf("value = %d, want 300", s.Value())
	}
	s.SetSteps([]int{100, 200})
	if s.Value() != 200 {
		t.Errorf("after shrinking: value %d, want the clamped 200", s.Value())
	}
	s.SetSteps(nil)
	if s.Value() != 0 || s.Label().Label != "Qty 0" {
		t.Errorf("empty ladder: value %d readout %q, want 0 / Qty 0", s.Value(), s.Label().Label)
	}
}

// TestQtyStepperReadoutGrows (Defect B regression): a wider value
// re-measures the readout, so a million never draws under the + button.
func TestQtyStepperReadoutGrows(t *testing.T) {
	s, ui, th := qtystepperEnv(t, []int{1, 1000000})
	before := s.Label().GetWidget().Rect.Dx()
	pump(ui)
	s.Step(1)
	pump(ui)
	lbl := s.Label().GetWidget().Rect
	if got, want := s.Label().Label, "Qty 1,000,000"; got != want {
		t.Fatalf("readout = %q, want %q", got, want)
	}
	need, _ := text.Measure(s.Label().Label, *th.FaceMono(theme.StepSmall), 0)
	if float64(lbl.Dx()) < need {
		t.Errorf("readout box %d wide, need ≥ %v", lbl.Dx(), need)
	}
	if lbl.Dx() <= before {
		t.Errorf("readout box %d did not grow from %d after the relayout", lbl.Dx(), before)
	}
	if plus := s.Plus().GetWidget().Rect; lbl.Max.X > plus.Min.X {
		t.Errorf("readout (%v) overlaps + button (%v)", lbl, plus)
	}
}
