package widgets

import (
	"image"
	"math"

	"github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// Slider geometry (design units, scaled by theme.Px). The row is 40 px to
// sit in the settings window beside its buttons; the GRAB band is the
// 24 px strip centred on the track (the track itself is thinner, the band
// never is), and the handle rides the track inside the band.
const (
	sliderRowH    = 40
	sliderBandH   = 24
	sliderTrackH  = 6
	sliderHandleW = 10
	sliderHandleH = 18
	sliderTickH   = 6
	sliderMinW    = 80
)

// SliderMode selects the value model: freeflow is a continuous range
// quantised only by Step; notches is an explicit ladder the handle snaps
// to (and whose marks the track draws).
type SliderMode int

const (
	// SliderFreeflow is the continuous variant (volumes: 0..100 in 1%
	// steps).
	SliderFreeflow SliderMode = iota
	// SliderNotches is the detented variant (the UI-scale ladder): the
	// handle rests exactly on the Notches values and the track shows a
	// tick for each of them.
	SliderNotches
)

// SliderOpts configures a Slider. The zero Mode is SliderFreeflow; a
// notched slider needs Notches (ascending, at least one value) and takes
// its range from the ladder, a freeflow one needs Min < Max and Step > 0.
//
// The two callbacks split the two jobs a host usually wants apart:
// OnChange fires continuously as the pointer moves (the live effect —
// audio, a preview) and OnRelease fires exactly once when the grab ends
// (the persist point — one sidecar write per drag, not one per frame).
type SliderOpts struct {
	Mode      SliderMode
	Min, Max  float64
	Step      float64
	Notches   []float64
	Value     float64
	OnChange  func(value float64)
	OnRelease func(value float64)
}

// Slider is the house slider, in both variants: a rounded track with an
// accent-filled portion and a handle that is grabbed by a press anywhere
// on its 24 px hit band. The readout is the caller's (the settings row's
// value Text); the widget owns the geometry, the input and the value
// model.
//
// Input is the pane seam's discipline (SplitPane.Step): Poll reads the
// shared raw cursor/press seams each frame while the panel is mounted and
// a press only grabs when the pointer sits on THIS widget's active input
// layer, so a slider under another window is inert and no drag event
// leaks into the window beneath. Once grabbed the handle follows the
// absolute cursor x (never a delta) until the button is released;
// OnRelease fires on that release and nowhere else.
//
// It is not an ebitenui event widget: the panel calls Poll from Sync, the
// same place the split-pane seams are polled, so a window rebuild between
// frames simply drops the old slider (its grab included) and the fresh
// one starts idle.
type Slider struct {
	th     theme.Theme
	widget *widget.Widget

	mode    SliderMode
	min     float64
	max     float64
	step    float64
	notches []float64
	value   float64

	onChange  func(float64)
	onRelease func(float64)
	grab      bool
}

// NewSlider builds a slider over the theme seam.
func NewSlider(th theme.Theme, opts SliderOpts) *Slider {
	s := &Slider{
		th:        th,
		mode:      opts.Mode,
		step:      opts.Step,
		onChange:  opts.OnChange,
		onRelease: opts.OnRelease,
		min:       opts.Min,
		max:       opts.Max,
	}
	if s.mode == SliderNotches && len(opts.Notches) > 0 {
		s.notches = append([]float64(nil), opts.Notches...)
		s.min, s.max = s.notches[0], s.notches[len(s.notches)-1]
	}
	if s.max < s.min {
		s.min, s.max = s.max, s.min
	}
	s.value = s.quantize(opts.Value)
	s.widget = widget.NewWidget(
		widget.WidgetOpts.LayoutData(widget.GridLayoutData{
			VerticalPosition: widget.GridLayoutPositionCenter,
		}),
	)
	return s
}

// GetWidget implements widget.PreferredSizeLocateableWidget.
func (s *Slider) GetWidget() *widget.Widget { return s.widget }

// SetLocation implements widget.PreferredSizeLocateableWidget.
func (s *Slider) SetLocation(r image.Rectangle) { s.widget.Rect = r }

// Validate implements widget.PreferredSizeLocateableWidget (nothing is
// cached: every draw reads the live value and theme scale).
func (s *Slider) Validate() {}

// PreferredSize carries the row height (the hit band is inside it) and a
// usable minimum width for a row that does not stretch it.
func (s *Slider) PreferredSize() (int, int) {
	return theme.Px(sliderMinW), theme.Px(sliderRowH)
}

// Value is the current (quantised) value.
func (s *Slider) Value() float64 { return s.value }

// Grabbed reports whether a press is currently dragging the handle — a
// host that mirrors state into the slider skips the mirror while true, so
// the drag is never snapped back under the pointer.
func (s *Slider) Grabbed() bool { return s.grab }

// SetValue sets the value programmatically: quantised, no callbacks, grab
// untouched. Sync uses it to mirror the host's setting into the handle.
func (s *Slider) SetValue(v float64) { s.value = s.quantize(v) }

// Poll advances one frame of slider input from the frame's pointer and
// the left button. The panel calls it from Sync, before Kit.Update.
func (s *Slider) Poll(p *Pointer) {
	s.Step(p.At(), p.Press(Left), p.Down(Left))
}

// Step is Poll's testable core: a rising press inside the hit band grabs
// the handle and jumps it to the absolute cursor x (click-to-position); a
// held grab keeps following the cursor, even outside the widget; the
// release drops the grab and fires OnRelease exactly once. A press on the
// band that does not change the value does not fire OnChange (a click on
// the current position is still a grab, so its release still persists).
func (s *Slider) Step(cursor image.Point, rising, pressed bool) {
	if s.grab {
		if !pressed {
			s.grab = false
			if s.onRelease != nil {
				s.onRelease(s.value)
			}
			return
		}
		s.setFromX(cursor.X)
		return
	}
	if !rising || !s.hit(cursor) {
		return
	}
	s.grab = true
	s.setFromX(cursor.X)
}

// hit reports whether p lands on the slider's 24 px grab band: inside the
// widget rect AND within the band strip AND on the widget's own active
// input layer (the SplitPane.seamAt idiom — a slider covered by another
// window never grabs).
func (s *Slider) hit(p image.Point) bool {
	w := s.widget
	if !w.IsVisible() || !p.In(w.Rect) {
		return false
	}
	band := s.HitBand()
	if p.Y < band.Min.Y || p.Y >= band.Max.Y {
		return false
	}
	return OwnsAt(w, p, input.LayerEventTypeMouseButton)
}

// HitBand is the widget's 24 design px grab strip, centred on the track.
func (s *Slider) HitBand() image.Rectangle {
	r := s.widget.Rect
	h := min(theme.Px(sliderBandH), r.Dy())
	cy := r.Min.Y + r.Dy()/2
	return image.Rect(r.Min.X, cy-h/2, r.Max.X, cy-h/2+h)
}

// ValueX is the x of v's handle centre — the render and hit math's one
// mapping, exposed for tests and for a host that wants to point at a rung.
// The travel is inset by half a handle so the whole handle stays on the
// track at both ends.
func (s *Slider) ValueX(v float64) int {
	lo, hi := s.trackXs()
	if hi <= lo || s.max <= s.min {
		return lo
	}
	f := (v - s.min) / (s.max - s.min)
	return lo + int(math.Round(f*float64(hi-lo)))
}

// TickPositions is the x of every notch (empty for freeflow) — the drawn
// marks and the tests' "one tick per rung" pin.
func (s *Slider) TickPositions() []int {
	out := make([]int, len(s.notches))
	for i, n := range s.notches {
		out[i] = s.ValueX(n)
	}
	return out
}

// setFromX jumps the value to the absolute x under the cursor.
func (s *Slider) setFromX(x int) {
	v := s.quantize(s.valueAtX(x))
	if v == s.value {
		return
	}
	s.value = v
	if s.onChange != nil {
		s.onChange(v)
	}
}

// valueAtX maps x onto the slider's domain (clamped at both ends).
func (s *Slider) valueAtX(x int) float64 {
	lo, hi := s.trackXs()
	if hi <= lo {
		return s.min
	}
	f := float64(x-lo) / float64(hi-lo)
	f = math.Max(0, math.Min(1, f))
	return s.min + f*(s.max-s.min)
}

// quantize pins a raw value to the mode: the nearest notch, or the
// nearest Step multiple from Min. Both are clamped to the domain.
func (s *Slider) quantize(v float64) float64 {
	if s.mode == SliderNotches && len(s.notches) > 0 {
		best := s.notches[0]
		for _, n := range s.notches[1:] {
			if math.Abs(v-n) < math.Abs(v-best) {
				best = n
			}
		}
		return best
	}
	v = math.Max(s.min, math.Min(s.max, v))
	if s.step > 0 {
		v = s.min + math.Round((v-s.min)/s.step)*s.step
	}
	return math.Max(s.min, math.Min(s.max, v))
}

// trackXs is the handle-centre travel: the track's inner span, inset by
// half a handle at each end.
func (s *Slider) trackXs() (int, int) {
	r := s.widget.Rect
	half := theme.Px(sliderHandleW) / 2
	return r.Min.X + half, r.Max.X - half
}

// Render paints the track, the accent fill up to the handle, the notch
// ticks and the handle — palette roles and theme tokens only.
func (s *Slider) Render(screen *ebiten.Image) {
	r := s.widget.Rect
	if r.Empty() {
		return
	}
	p := s.th.Pal()
	cy := r.Min.Y + r.Dy()/2
	th := theme.Px(sliderTrackH)
	track := image.Rect(r.Min.X, cy-th/2, r.Max.X, cy-th/2+th)
	theme.DrawRoundBox(screen, track,
		theme.Solid(p.Cell), theme.Solid(p.Border), theme.Px(1), theme.AllRadii(theme.RadiusControl))

	hx := s.ValueX(s.value)
	if inner := track.Inset(theme.Px(2)); hx > inner.Min.X {
		fill := image.Rect(inner.Min.X, inner.Min.Y, hx, inner.Max.Y)
		theme.DrawRoundBox(screen, fill,
			theme.Solid(p.Accent), theme.Solid(p.Accent), 0, theme.AllRadii(theme.RadiusControl))
	}
	for _, x := range s.TickPositions() {
		theme.FillRect(screen, image.Rect(x, cy-theme.Px(sliderTickH)/2, x+max(theme.Px(1), 1), cy-theme.Px(sliderTickH)/2+theme.Px(sliderTickH)), theme.Solid(p.Dim))
	}
	hw, hh := theme.Px(sliderHandleW), theme.Px(sliderHandleH)
	handle := image.Rect(hx-hw/2, cy-hh/2, hx-hw/2+hw, cy-hh/2+hh)
	theme.DrawRoundBox(screen, handle,
		theme.Solid(p.Text), theme.Solid(p.Accent), theme.Px(1), theme.AllRadii(theme.RadiusControl))
}
