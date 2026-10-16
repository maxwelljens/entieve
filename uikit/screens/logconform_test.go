package screens

import (
	"fmt"
	"github.com/entieve/entieve/uikit/theme"
	"image"
	"reflect"
	"strconv"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/sim"
)

// Log window conformance pins: the body is laid out from the WINDOW, not
// from the text — the old RowLayout body stretched only in the cross
// axis, so the text area kept its preferred height (the whole history)
// and drew thousands of pixels past the frame on every relayout. These
// pins hold the window/body/text-area rect chain at both design sizes
// while 200+ lines flow through it.

// logInnerRect is the window's inner rect: the contents row under the
// title bar (the window grid pins the title row to exactly titleBarH —
// MinHeight floors it and GridLayoutData.MaxHeight caps it).
func logInnerRect(win image.Rectangle) image.Rectangle {
	return image.Rect(win.Min.X, win.Min.Y+theme.TitleBarH, win.Max.X, win.Max.Y)
}

// logHandleRect reads the scrollbar handle's rect. Slider.handle is
// private, and the ebitenui size/position mismatch (size floored at
// MinHandleSize, position computed from FixedHandleSize) shows up only
// here and in the pixel capture: the handle at the bottom of its travel
// poked through the window edge. Read-only reflect, same contract as
// logScrollTop.
func logHandleRect(t *testing.T, s *widget.Slider) image.Rectangle {
	t.Helper()
	field := func(v reflect.Value, name string) reflect.Value {
		if !v.IsValid() {
			t.Fatalf("ebitenui internals moved while resolving %q — re-pin logHandleRect", name)
		}
		f := v.FieldByName(name)
		if !f.IsValid() {
			t.Fatalf("ebitenui internals moved: no field %q — re-pin logHandleRect", name)
		}
		return f
	}
	h := field(reflect.ValueOf(s).Elem(), "handle")
	w := field(h.Elem(), "widget")
	r := field(w.Elem(), "Rect")
	min := field(r, "Min")
	max := field(r, "Max")
	return image.Rect(int(field(min, "X").Int()), int(field(min, "Y").Int()),
		int(field(max, "X").Int()), int(field(max, "Y").Int()))
}

// assertLogConform checks the rect chain at one step: the window sits at
// wantWin byte-identical, the body IS the inner rect, and the text area
// IS the body's padded inner rect (hence inside the frame), with the
// scrollbar widgets it hosts inside it too.
func assertLogConform(t *testing.T, l *LogPanel, step string, wantWin image.Rectangle) {
	t.Helper()
	win := l.Win().GetContainer().GetWidget().Rect
	if win != wantWin {
		t.Fatalf("%s: window rect = %v, want %v (appends/resizes must not drift the window)", step, win, wantWin)
	}
	inner := logInnerRect(win)
	body := l.win.Contents.GetWidget().Rect
	if body != inner {
		t.Errorf("%s: body rect = %v, want the window's inner rect %v", step, body, inner)
	}
	ta := l.ta.GetWidget().Rect
	if want := body.Inset(theme.ContentsPad); ta != want {
		t.Errorf("%s: text area rect = %v, want the body's inner rect %v", step, ta, want)
	}
	if !ta.In(inner) {
		t.Errorf("%s: text area %v escapes the window's inner rect %v", step, ta, inner)
	}
	// Every content widget the text area hosts stays inside it: the
	// scrollbar track and the handle at both ends of its travel.
	s := logVSlider(t, l)
	if r := s.GetWidget().Rect; !r.In(ta) {
		t.Errorf("%s: scrollbar rect %v escapes the text area %v", step, r, ta)
	}
	if h := logHandleRect(t, s); !h.In(ta) {
		t.Errorf("%s: scrollbar handle %v escapes the text area %v (inner %v)", step, h, ta, inner)
	}
}

// logVSlider returns the vertical scrollbar the TextArea built. It is
// TextArea's only exported reach into its scroll state: Current mirrors
// the tail (Max = view at the newest line).
func logVSlider(t *testing.T, l *LogPanel) *widget.Slider {
	t.Helper()
	for _, f := range l.ta.GetFocusers() {
		if s, ok := f.(*widget.Slider); ok {
			return s
		}
	}
	t.Fatal("the log's TextArea built no vertical scrollbar")
	return nil
}

// logScrollTop reads the TextArea's internal ScrollTop. ebitenui exposes
// no accessor on TextArea, and the tail/wheel pins must observe the
// actual scroll position — not just the slider it is driven from (a
// dropped VerticalScrollMode(ScrollEnd) or ChangedHandler would move the
// slider and never the content). Read-only reflect against the pinned
// ebitenui v0.7.3; the panic message names the field if it ever moves.
func logScrollTop(t *testing.T, l *LogPanel) float64 {
	t.Helper()
	sc := reflect.ValueOf(l.ta).Elem().FieldByName("scrollContainer")
	if !sc.IsValid() {
		t.Fatal("TextArea.scrollContainer field missing — ebitenui internals moved, re-pin logScrollTop")
	}
	st := sc.Elem().FieldByName("ScrollTop")
	if !st.IsValid() {
		t.Fatal("ScrollContainer.ScrollTop field missing — ebitenui internals moved, re-pin logScrollTop")
	}
	return st.Float()
}

// TestLogBodyConformsToWindow is the design-math pin: at the default
// 288×240 and a resized 400×300, appending 200+ lines leaves the window
// rect byte-identical, the body rect equal to the window's inner rect,
// the text area equal to the body's padded inner rect (every reachable
// content widget inside the frame), and it stays that way through the
// full relayout (visibility toggle) and resize paths that used to size
// the content from its preferred height.
func TestLogBodyConformsToWindow(t *testing.T) {
	env, _ := newChromeEnv(t)
	l := NewLog(env)
	c := newChromeUI(env)
	c.AddWindow(l)
	c.Frame(t, 1280, 720)

	def := logDefaultRect()
	assertLogConform(t, l, "mounted", def)

	// Appends: a frame every 55 pushes, conformance after each.
	for i := 0; i < 220; i++ {
		l.PushLine(sim.SevInfo, "conform line "+strconv.Itoa(i))
		if i%55 == 54 {
			c.Frame(t, 1280, 720)
			assertLogConform(t, l, fmt.Sprintf("%d pushes", i+1), def)
		}
	}
	// The leak we pin against: the text's preferred height is the whole
	// history, far past the viewport, while the rect stays window-sized.
	ta := l.ta.GetWidget().Rect
	if _, ph := l.ta.PreferredSize(); ph <= ta.Dy() {
		t.Fatalf("fixture: preferred height %d does not overflow the viewport %d — "+
			"the pin below would not catch a preferred-size leak", ph, ta.Dy())
	}

	// Full relayout (the trigger behind "does not resize correctly when
	// new entries are added"): kit.go toggles window visibility, which
	// sets RelayoutRequested and re-runs every layout in the window.
	// Pre-fix this sized the text area from its preferred height.
	l.Container().GetWidget().SetVisibility(widget.Visibility_Hide)
	l.Container().GetWidget().SetVisibility(widget.Visibility_Show)
	c.Frame(t, 1280, 720)
	assertLogConform(t, l, "after visibility relayout", def)

	// SetLines (save/load rebuild) changes content wholesale: same chain.
	lines := make([]string, 0, 210)
	for i := 0; i < 210; i++ {
		lines = append(lines, "[color=#5fcf80]reload line "+strconv.Itoa(i)+"[/color]")
	}
	l.SetLines(lines)
	c.Frame(t, 1280, 720)
	assertLogConform(t, l, "after SetLines", def)

	// Resize to 400×300: the body conforms to the new inner rect and the
	// content reflows into it without touching the window again.
	resized := image.Rect(100, 100, 500, 400)
	l.Win().SetLocation(resized)
	c.Frame(t, 1280, 720)
	assertLogConform(t, l, "resized 400x300", resized)

	// More appends at the new size: window byte-identical, body still the
	// inner rect, text area still the body's inner rect.
	for i := 0; i < 80; i++ {
		l.PushLine(sim.SevInfo, "post-resize "+strconv.Itoa(i))
	}
	c.Frame(t, 1280, 720)
	c.Frame(t, 1280, 720)
	assertLogConform(t, l, "80 pushes after resize", resized)
}

// TestLogTailAndWheelScroll pins the scroll semantics: an append tails
// to the newest line (slider at Max, ScrollTop at 1 — always-tail: the
// TextArea re-pins on every SetText because it exposes no "user scrolled
// up" query), wheel moves through the scrollback with the table's
// convention (positive notch = wheel up = towards the top) and clamps at
// both ends, and a tail again after being scrolled away.
func TestLogTailAndWheelScroll(t *testing.T) {
	env, _ := newChromeEnv(t)
	l := NewLog(env)
	c := newChromeUI(env)
	c.AddWindow(l)
	for i := 0; i < 200; i++ {
		l.PushLine(sim.SevInfo, "scroll line "+strconv.Itoa(i))
	}
	c.Frame(t, 1280, 720)
	c.Frame(t, 1280, 720)

	s := logVSlider(t, l)
	// Tail after mount: newest line in view.
	if s.Current != s.Max || logScrollTop(t, l) != 1 {
		t.Fatalf("tail: Current=%d/%d ScrollTop=%v, want Max and 1 (newest line visible)",
			s.Current, s.Max, logScrollTop(t, l))
	}

	// fireLogWheel fires one notch on the log's vertical scrollbar — the
	// wheel seam inside the log's own widget tree (ebitenui's Slider
	// handler: vertical, Current -= PageSize·Y, the same −Y convention
	// TextArea's internal text-wheel handler uses) — and pumps the
	// deferred event queue. The Changed → ScrollTop hop lands one frame
	// later (Slider fires ChangedEvent from Render).
	fireLogWheel := func(notches float64) {
		s.GetWidget().ScrolledEvent.Fire(&widget.WidgetScrolledEventArgs{Y: notches})
		c.Frame(t, 1280, 720) // dispatch the scroll → move the slider
		c.Frame(t, 1280, 720) // Slider.Render fires Changed; dispatch it
	}

	// Wheel up (positive notch): strictly towards the top of the history.
	before := logScrollTop(t, l)
	fireLogWheel(1)
	mid := logScrollTop(t, l)
	if mid >= before {
		t.Fatalf("wheel up: ScrollTop = %v, want strictly less than %v (towards the top)", mid, before)
	}
	if s.Current >= s.Max {
		t.Errorf("wheel up: slider Current = %d, want < Max %d", s.Current, s.Max)
	}
	// Scrollback is reachable: hammer wheel-up, clamp at the top.
	for i := 0; i < 40; i++ {
		fireLogWheel(1)
	}
	if top := logScrollTop(t, l); top != 0 || s.Current != s.Min {
		t.Errorf("wheel up clamp: ScrollTop = %v Current = %d, want 0 and Min %d", top, s.Current, s.Min)
	}

	// Wheel down reverses and clamps at the tail.
	for i := 0; i < 60; i++ {
		fireLogWheel(-1)
	}
	if bot := logScrollTop(t, l); bot != 1 || s.Current != s.Max {
		t.Errorf("wheel down clamp: ScrollTop = %v Current = %d, want 1 and Max %d", bot, s.Current, s.Max)
	}

	// Mid-history, an append re-tails (always-tail): newest line visible.
	fireLogWheel(4)
	if logScrollTop(t, l) >= 1 {
		t.Fatalf("fixture: ScrollTop = %v, want < 1 before the re-tail append", logScrollTop(t, l))
	}
	l.PushLine(sim.SevInfo, "after wheel")
	c.Frame(t, 1280, 720)
	c.Frame(t, 1280, 720)
	if s.Current != s.Max || logScrollTop(t, l) != 1 {
		t.Errorf("re-tail after append: Current=%d/%d ScrollTop=%v, want Max and 1",
			s.Current, s.Max, logScrollTop(t, l))
	}
}
