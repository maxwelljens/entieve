package widgets

import (
	"image"
	"math"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
)

// newPumpedSlider mounts a slider in a one-column grid inside a window
// (the only place it runs: the grab asks the widget's input layer) and
// pumps a frame at 640×480 so the rect is laid out.
func newPumpedSlider(t *testing.T, opts SliderOpts) *Slider {
	t.Helper()
	env, _ := newTestEnv(t)
	s := NewSlider(env, opts)
	grid := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false}),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(300), 0)),
	)
	grid.AddChild(s)
	win := widget.NewWindow(
		widget.WindowOpts.Contents(grid),
		widget.WindowOpts.TitleBar(
			widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout())),
			theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(320), theme.Px(120)),
		widget.WindowOpts.Location(image.Rect(0, 0, 340, 140)),
	)
	ui := newWindowUI(t, env, win)
	uitest.PumpWindow(t, ui, nil, 640, 480)
	return s
}

// drag presses at from, moves to each x in path while held, then releases
// at the last point. It returns the OnChange call count.
func drag(t *testing.T, s *Slider, from image.Point, path ...int) {
	t.Helper()
	s.Step(from, true, true)
	last := from.X
	for _, x := range path {
		s.Step(image.Pt(x, from.Y), false, true)
		last = x
	}
	s.Step(image.Pt(last, from.Y), false, false)
}

// sliderMid is the track's centre y (a point inside the hit band).
func sliderMid(s *Slider) image.Point {
	b := s.HitBand()
	return image.Pt(b.Min.X+1, b.Min.Y+b.Dy()/2)
}

// TestSliderFreeflowIsContinuous: the freeflow variant quantises only on
// its own step — neighbouring drag positions give neighbouring values,
// never a detent jump.
func TestSliderFreeflowIsContinuous(t *testing.T) {
	var changes []float64
	s := newPumpedSlider(t, SliderOpts{
		Mode: SliderFreeflow, Min: 0, Max: 100, Step: 1, Value: 0,
		OnChange: func(v float64) { changes = append(changes, v) },
	})
	mid := sliderMid(s)

	s.Step(image.Pt(s.ValueX(50), mid.Y), true, true)
	if got := s.Value(); math.Abs(got-50) > 1 {
		t.Fatalf("click at the 50%% mark = %v, want ≈50", got)
	}
	base := s.Value()
	s.Step(image.Pt(s.ValueX(50)+3, mid.Y), false, true)
	got := s.Value()
	if got == base {
		t.Fatalf("a 3 px drag did not move the freeflow value (%v)", got)
	}
	if math.Abs(got-base) > 2 {
		t.Errorf("3 px of travel jumped %v → %v, want adjacent whole percents (no snapping)", base, got)
	}
	if got != math.Trunc(got) {
		t.Errorf("freeflow value %v is not on the 1%% step", got)
	}
	if len(changes) < 2 {
		t.Errorf("OnChange fired %d times during a click+drag, want the live stream", len(changes))
	}
	s.Step(image.Pt(s.ValueX(50)+3, mid.Y), false, false)
}

// TestSliderNotchSnapsToLadder: the notched variant never holds a value
// between rungs — a drag or a click mid-gap lands on a rung — and the
// track carries one tick per rung, exactly where the handle rests.
func TestSliderNotchSnapsToLadder(t *testing.T) {
	ladder := []float64{0.75, 1.0, 1.25, 1.5, 2.0, 2.5}
	s := newPumpedSlider(t, SliderOpts{
		Mode: SliderNotches, Notches: ladder, Value: 1.0,
	})
	mid := sliderMid(s)

	// Click mid-gap between rungs 2 and 3 (1.25 and 1.5).
	lo, hi := s.ValueX(1.25), s.ValueX(1.5)
	s.Step(image.Pt((lo+hi)/2, mid.Y), true, true)
	got := s.Value()
	if got != 1.25 && got != 1.5 {
		t.Fatalf("click mid-gap = %v, want a rung (1.25 or 1.5), never between", got)
	}
	s.Step(image.Pt((lo+hi)/2, mid.Y), false, false)

	// Drag across the gap: every intermediate value is a rung.
	s.Step(image.Pt(lo, mid.Y), true, true)
	for _, x := range []int{lo + 1, lo + 2, hi - 2, hi - 1, hi} {
		s.Step(image.Pt(x, mid.Y), false, true)
		if v := s.Value(); !onLadder(v, ladder) {
			t.Fatalf("drag x=%d produced %v, which is not on the ladder", x, v)
		}
	}
	s.Step(image.Pt(hi, mid.Y), false, false)

	ticks := s.TickPositions()
	if len(ticks) != len(ladder) {
		t.Fatalf("ticks = %d, want one per rung (%d)", len(ticks), len(ladder))
	}
	for i, n := range ladder {
		if ticks[i] != s.ValueX(n) {
			t.Errorf("tick %d at x=%d, want the handle's x for %v (%d)", i, ticks[i], n, s.ValueX(n))
		}
	}
}

// onLadder reports whether v is one of the rungs.
func onLadder(v float64, ladder []float64) bool {
	for _, n := range ladder {
		if v == n {
			return true
		}
	}
	return false
}

// TestSliderClampsAtBothEnds: travel past either end pins the domain's
// ends in both modes.
func TestSliderClampsAtBothEnds(t *testing.T) {
	s := newPumpedSlider(t, SliderOpts{Mode: SliderFreeflow, Min: 0, Max: 100, Step: 1})
	mid := sliderMid(s)
	r := s.GetWidget().Rect
	s.Step(image.Pt(r.Max.X-1, mid.Y), true, true) // grab at the right end
	s.Step(image.Pt(r.Max.X+500, mid.Y), false, true)
	if got := s.Value(); got != 100 {
		t.Errorf("drag far right = %v, want 100", got)
	}
	s.Step(image.Pt(r.Min.X-500, mid.Y), false, true)
	if got := s.Value(); got != 0 {
		t.Errorf("drag far left = %v, want 0", got)
	}
	s.Step(image.Pt(r.Min.X-500, mid.Y), false, false)

	n := newPumpedSlider(t, SliderOpts{Mode: SliderNotches, Notches: []float64{0.75, 1, 2.5}})
	nmid := sliderMid(n)
	nr := n.GetWidget().Rect
	n.Step(image.Pt(nr.Max.X-1, nmid.Y), true, true)
	n.Step(image.Pt(nr.Max.X+500, nmid.Y), false, true)
	if got := n.Value(); got != 2.5 {
		t.Errorf("notched drag far right = %v, want 2.5", got)
	}
	n.Step(image.Pt(nr.Min.X-500, nmid.Y), false, true)
	if got := n.Value(); got != 0.75 {
		t.Errorf("notched drag far left = %v, want 0.75", got)
	}
	n.Step(image.Pt(nr.Min.X-500, nmid.Y), false, false)
}

// TestSliderReleaseOncePerGrab: OnChange streams while the pointer moves,
// OnRelease fires exactly once when the grab ends — and a click without a
// drag still counts as one grab (one release, possibly zero changes).
func TestSliderReleaseOncePerGrab(t *testing.T) {
	var changes, releases int
	s := newPumpedSlider(t, SliderOpts{
		Mode: SliderFreeflow, Min: 0, Max: 100, Step: 1,
		OnChange:  func(float64) { changes++ },
		OnRelease: func(float64) { releases++ },
	})
	mid := sliderMid(s)
	drag(t, s, image.Pt(s.ValueX(10), mid.Y), s.ValueX(20), s.ValueX(40), s.ValueX(60))
	if changes != 4 {
		t.Errorf("OnChange calls = %d, want 4 (the press plus one per moved position)", changes)
	}
	if releases != 1 {
		t.Errorf("OnRelease calls = %d, want exactly 1", releases)
	}
	s.Step(image.Pt(s.ValueX(60), mid.Y), false, false) // a stray release
	if releases != 1 {
		t.Errorf("a release without a grab fired OnRelease (%d)", releases)
	}

	// Click on the current position: no change, still one grab/release.
	before := changes
	drag(t, s, image.Pt(s.ValueX(s.Value()), mid.Y))
	if changes != before {
		t.Errorf("a click on the current value fired OnChange")
	}
	if releases != 2 {
		t.Errorf("click-without-drag releases = %d, want 2", releases)
	}
}

// TestSliderHitBandAtMinScale: the grab band is 24 design px (18 px at
// the 0.75 rung) and a click just inside it grabs, regardless of where
// the thin track sits; a click just outside it is inert. The press need
// not land on the handle.
func TestSliderHitBandAtMinScale(t *testing.T) {
	uitest.UseScale(t, 0.75)
	s := newPumpedSlider(t, SliderOpts{Mode: SliderFreeflow, Min: 0, Max: 100, Step: 1, Value: 0})
	band := s.HitBand()
	if got, want := band.Dy(), theme.Px(24); got != want {
		t.Fatalf("hit band = %d px tall, want the 24 design px band (%d)", got, want)
	}
	x := s.ValueX(50)

	s.Step(image.Pt(x, band.Min.Y-1), true, true)
	if s.Grabbed() {
		t.Error("a press above the band grabbed the slider")
	}
	s.Step(image.Pt(x, band.Min.Y), true, true)
	if !s.Grabbed() {
		t.Fatal("a press on the band's top edge did not grab")
	}
	if got := s.Value(); math.Abs(got-50) > 1 {
		t.Errorf("click on the band = %v, want ≈50", got)
	}
	s.Step(image.Pt(x, band.Max.Y-1), false, false)
	if s.Grabbed() {
		t.Error("the release left the slider grabbed")
	}
	s.Step(image.Pt(x, band.Max.Y), true, true)
	if s.Grabbed() {
		t.Error("a press one px below the band grabbed the slider")
	}
}
