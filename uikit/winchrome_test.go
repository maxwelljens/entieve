package uikit

import (
	"image"
	"testing"

	"github.com/ebitenui/ebitenui/event"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestEdgesAtGripBands pins the grip hit test: every edge inside its
// 8 px band reports that edge, corners report both axes, the interior
// reports none, and anything outside the rect grips nothing.
func TestEdgesAtGripBands(t *testing.T) {
	const g = edgeGripPx
	rect := image.Rect(100, 200, 500, 400) // 400×200
	cases := []struct {
		name string
		p    image.Point
		want edgeMask
	}{
		{"interior", image.Point{300, 300}, 0},
		{"left band first px", image.Point{100, 300}, edgeLeft},
		{"left band last px", image.Point{100 + g - 1, 300}, edgeLeft},
		{"just past left band", image.Point{100 + g, 300}, 0},
		{"right band first px", image.Point{500 - g, 300}, edgeRight},
		{"right band last px", image.Point{499, 300}, edgeRight},
		{"just past right band", image.Point{500 - g - 1, 300}, 0},
		{"top band", image.Point{300, 200}, edgeTop},
		{"top band last px", image.Point{300, 200 + g - 1}, edgeTop},
		{"bottom band", image.Point{300, 400 - g}, edgeBottom},
		{"bottom band last px", image.Point{300, 399}, edgeBottom},
		{"top-left corner", image.Point{100, 200}, edgeLeft | edgeTop},
		{"top-right corner", image.Point{499, 200}, edgeRight | edgeTop},
		{"bottom-left corner", image.Point{100, 399}, edgeLeft | edgeBottom},
		{"bottom-right corner", image.Point{499, 399}, edgeRight | edgeBottom},
		{"corner band mixes axes", image.Point{105, 205}, edgeLeft | edgeTop},
		{"outside left", image.Point{99, 300}, 0},
		{"outside above", image.Point{300, 199}, 0},
		{"outside right", image.Point{500, 300}, 0}, // Max is exclusive
	}
	for _, c := range cases {
		if got := edgesAt(c.p, rect, g); got != c.want {
			t.Errorf("%s: edgesAt(%v) = %b, want %b", c.name, c.p, got, c.want)
		}
	}

	// A never-laid-out (empty) rect grips nothing even at the origin.
	if got := edgesAt(image.Point{}, image.Rectangle{}, g); got != 0 {
		t.Errorf("empty rect: edgesAt = %b, want 0", got)
	}
}

// TestResizeRectEdgeMath pins the drag math: the dragged edge moves, the
// opposite edge never does, and no axis shrinks below MinSize.
func TestResizeRectEdgeMath(t *testing.T) {
	start := image.Rect(100, 100, 500, 400) // 400×300
	minSize := image.Point{X: 320, Y: 240}
	atMin := image.Rect(180, 160, 500, 400) // exactly MinSize: 320×240

	// Left edge inward by 40 with the window already at MinSize width:
	// Max.X is untouched and the width stays exactly MinSize.
	r := resizeRect(atMin, 40, 0, edgeLeft, minSize)
	if r.Max.X != atMin.Max.X {
		t.Errorf("left@min: Max.X = %d, want %d (opposite edge fixed)", r.Max.X, atMin.Max.X)
	}
	if r.Dx() != minSize.X {
		t.Errorf("left@min: width = %d, want MinSize %d", r.Dx(), minSize.X)
	}
	if r.Min.X != atMin.Min.X {
		t.Errorf("left@min: Min.X = %d, want %d (clamped back)", r.Min.X, atMin.Min.X)
	}

	// Left edge outward: Min.X follows the cursor, Max.X stays put, the
	// window grows.
	r = resizeRect(start, -40, 0, edgeLeft, minSize)
	if r.Min.X != 60 || r.Max.X != start.Max.X || r.Dx() != 440 {
		t.Errorf("left outward: rect = %v, want (60,100)-(500,400)", r)
	}

	// Left edge inward with room to shrink: Min.X moves +25, Max.X fixed.
	r = resizeRect(start, 25, 0, edgeLeft, minSize)
	if r.Min.X != 125 || r.Max.X != 500 || r.Dx() != 375 {
		t.Errorf("left inward: rect = %v, want (125,100)-(500,400)", r)
	}

	// Top edge dragged down 40 past MinSize height: Min.Y stays (height
	// preserved at MinSize), Max.Y is untouched.
	r = resizeRect(atMin, 0, 40, edgeTop, minSize)
	if r.Min.Y != atMin.Min.Y {
		t.Errorf("top@min: Min.Y = %d, want %d (height would drop below MinSize)", r.Min.Y, atMin.Min.Y)
	}
	if r.Dy() != minSize.Y || r.Max.Y != atMin.Max.Y {
		t.Errorf("top@min: height = %d (Max.Y = %d), want %d / %d", r.Dy(), r.Max.Y, minSize.Y, atMin.Max.Y)
	}

	// Top edge upward: Min.Y moves, Max.Y (bottom) fixed, height grows.
	r = resizeRect(start, 0, -30, edgeTop, minSize)
	if r.Min.Y != 70 || r.Max.Y != start.Max.Y || r.Dy() != 330 {
		t.Errorf("top upward: rect = %v, want (100,70)-(500,400)", r)
	}

	// Bottom edge: Max.Y follows the cursor, Min.Y fixed, MinSize clamp.
	r = resizeRect(start, 0, 55, edgeBottom, minSize)
	if r.Max.Y != 455 || r.Min.Y != start.Min.Y {
		t.Errorf("bottom: rect = %v, want (100,100)-(500,455)", r)
	}
	r = resizeRect(start, 0, -500, edgeBottom, minSize)
	if r.Dy() != minSize.Y || r.Min.Y != start.Min.Y {
		t.Errorf("bottom clamp: height = %d at Min.Y %d, want %d at %d",
			r.Dy(), r.Min.Y, minSize.Y, start.Min.Y)
	}

	// Right edge mirrors left: Min.X fixed, Max.X follows, clamped.
	r = resizeRect(start, 60, 0, edgeRight, minSize)
	if r.Min.X != start.Min.X || r.Max.X != 560 {
		t.Errorf("right: rect = %v, want (100,100)-(560,400)", r)
	}
	r = resizeRect(start, -500, 0, edgeRight, minSize)
	if r.Dx() != minSize.X || r.Min.X != start.Min.X {
		t.Errorf("right clamp: width = %d at Min.X %d, want %d at %d",
			r.Dx(), r.Min.X, minSize.X, start.Min.X)
	}

	// Corner: both axes independent (top-left grip moves Min only).
	r = resizeRect(start, -10, -20, edgeLeft|edgeTop, minSize)
	if r.Min.X != 90 || r.Min.Y != 80 || r.Max.X != 500 || r.Max.Y != 400 {
		t.Errorf("top-left corner: rect = %v, want (90,80)-(500,400)", r)
	}

	// No edges: the rect is untouched (a plain click inside a grip of a
	// window that does not move fires ResizeEvent with an equal rect).
	if got := resizeRect(start, 7, 9, 0, minSize); got != start {
		t.Errorf("zero mask: rect = %v, want unchanged %v", got, start)
	}

	// Invariant sweep: for every edge set, the edges not named in the
	// mask stay exactly where they were, and sizes hold MinSize.
	masks := []edgeMask{edgeLeft, edgeRight, edgeTop, edgeBottom,
		edgeLeft | edgeTop, edgeRight | edgeBottom,
		edgeLeft | edgeRight, edgeTop | edgeBottom}
	for _, m := range masks {
		got := resizeRect(start, -700, -700, m, minSize) // maximal drag
		if m&edgeLeft == 0 && got.Min.X != start.Min.X {
			t.Errorf("mask %b: Min.X moved to %d", m, got.Min.X)
		}
		if m&edgeRight == 0 && got.Max.X != start.Max.X {
			t.Errorf("mask %b: Max.X moved to %d", m, got.Max.X)
		}
		if m&edgeTop == 0 && got.Min.Y != start.Min.Y {
			t.Errorf("mask %b: Min.Y moved to %d", m, got.Min.Y)
		}
		if m&edgeBottom == 0 && got.Max.Y != start.Max.Y {
			t.Errorf("mask %b: Max.Y moved to %d", m, got.Max.Y)
		}
		if got.Dx() < minSize.X || got.Dy() < minSize.Y {
			t.Errorf("mask %b: size %dx%d below MinSize %v", m, got.Dx(), got.Dy(), minSize)
		}
	}

	// No MinSize at all: the 1 px floor keeps the rect from inverting.
	got := resizeRect(start, 10000, 10000, edgeLeft|edgeTop, image.Point{})
	if got.Dx() < 1 || got.Dy() < 1 {
		t.Errorf("no MinSize: rect = %v, want at least 1×1", got)
	}
}

// newChromeTestWindow builds a minimal titled window with the resize
// chrome attached, at a known rect.
func newChromeTestWindow(t *testing.T) (*widget.Window, *winChrome) {
	t.Helper()
	env, _ := newTestEnv(t)
	body := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewRowLayout()))
	title := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout()))
	win := widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, 44),
		widget.WindowOpts.Draggable(),
		widget.WindowOpts.MinSize(320, 240),
		widget.WindowOpts.Location(image.Rect(100, 100, 500, 400)),
	)
	ch := makeEdgeResizable(win)
	// One pump on a real UI: layout gives the title bar and contents
	// their rects (press args are widget-relative).
	ui := newWindowUI(t, env, win)
	pumpWindow(t, ui, nil, 1280, 720)
	return win, ch
}

// TestWinChromeGripDrag drives the chrome end to end without ebiten
// input: a deferred press event starts the grip, step() applies the drag
// math every frame (the win rect only moves through SetLocation), and
// release fires ResizeEvent exactly once with the final rect.
func TestWinChromeGripDrag(t *testing.T) {
	win, ch := newChromeTestWindow(t)
	event.ExecuteDeferred() // flush the deferred handler registrations

	// Count ResizeEvents.
	resizes := 0
	var last image.Rectangle
	win.ResizeEvent.AddHandler(func(a interface{}) {
		if e, ok := a.(*widget.WindowChangedEventArgs); ok {
			resizes++
			last = e.Rect
		}
	})
	event.ExecuteDeferred()

	// Press on the left edge inside the title bar (title-relative offset
	// 3 px in, 20 px down): this starts a left-grip resize, and the
	// built-in Draggable() handler starts its title drag too — the
	// chrome's absolute recompute must win.
	win.TitleBar.GetWidget().MouseButtonPressedEvent.Fire(&widget.WidgetMouseButtonPressedEventArgs{
		Widget:  win.TitleBar.GetWidget(),
		Button:  ebiten.MouseButtonLeft,
		OffsetX: 3,
		OffsetY: 5,
	})
	event.ExecuteDeferred()
	if !ch.active || ch.edges != edgeLeft|edgeTop {
		t.Fatalf("press in top-left grip: active = %v edges = %b, want true / left|top", ch.active, ch.edges)
	}

	rect := win.GetContainer().GetWidget().Rect
	if rect != image.Rect(100, 100, 500, 400) {
		t.Fatalf("rect at press = %v", rect)
	}

	// Drag inward/down by (+40,+10) from the press point (103,105):
	// left edge moves, right edge fixed.
	ch.step(image.Point{X: 143, Y: 115}, true)
	rect = win.GetContainer().GetWidget().Rect
	if rect.Max.X != 500 || rect.Min.X != 140 || rect.Min.Y != 110 {
		t.Errorf("drag (+40,+10): rect = %v, want (140,110)-(500,400)", rect)
	}
	if resizes != 0 {
		t.Errorf("ResizeEvent fired mid-drag (%d times)", resizes)
	}

	// Shrink below MinSize: clamped, right/bottom edges still fixed.
	ch.step(image.Point{X: 400, Y: 400}, true)
	rect = win.GetContainer().GetWidget().Rect
	if rect.Max.X != 500 || rect.Max.Y != 400 || rect.Dx() != 320 || rect.Dy() != 240 {
		t.Errorf("drag past MinSize: rect = %v, want 320×240 ending at (500,400)", rect)
	}

	// Release: one ResizeEvent, final rect, chrome idle again. The
	// event queue is deferred (ebitenui semantics), so flush it before
	// counting.
	ch.step(image.Point{X: 400, Y: 400}, false)
	event.ExecuteDeferred()
	if ch.active {
		t.Error("still active after release")
	}
	if resizes != 1 {
		t.Fatalf("ResizeEvent count = %d, want 1", resizes)
	}
	if last != win.GetContainer().GetWidget().Rect {
		t.Errorf("ResizeEvent rect = %v, want final %v", last, win.GetContainer().GetWidget().Rect)
	}

	// A press in the window interior does not start a resize (the title
	// drag / content widgets own it).
	win.Contents.GetWidget().MouseButtonPressedEvent.Fire(&widget.WidgetMouseButtonPressedEventArgs{
		Widget:  win.Contents.GetWidget(),
		Button:  ebiten.MouseButtonLeft,
		OffsetX: 200,
		OffsetY: 100,
	})
	event.ExecuteDeferred()
	if ch.active {
		t.Error("interior press started a resize")
	}
	ch.step(image.Point{X: 999, Y: 999}, true) // no-op: nothing active
	if resizes != 1 {
		t.Errorf("ResizeEvent count after interior press = %d, want 1", resizes)
	}
}

// TestWinChromeIsIdempotent: attaching the chrome twice keeps one state
// and one set of handlers.
func TestWinChromeIsIdempotent(t *testing.T) {
	win, first := newChromeTestWindow(t)
	again := makeEdgeResizable(win)
	if again != first {
		t.Fatal("makeEdgeResizable built a second chrome for the same window")
	}
	if _, dup := chromeByWindow[win]; !dup {
		t.Fatal("window not indexed in chromeByWindow")
	}
}
