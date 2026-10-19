package screens

import (
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"image"
	"testing"

	"github.com/ebitenui/ebitenui/event"
	"github.com/ebitenui/ebitenui/input"
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

// fakeMouse is ebitenui's CursorUpdater seam: a scripted cursor and left
// button. Install it so the REAL pipeline (widget press/click events,
// input layers, the chrome's step) runs off test input instead of ebiten.
type fakeMouse struct {
	x, y     int
	pressed  bool
	justDown bool
	justUp   bool
}

func (f *fakeMouse) Update()                             {}
func (f *fakeMouse) AfterUpdate()                        { f.justDown, f.justUp = false, false }
func (f *fakeMouse) Draw(*ebiten.Image)                  {}
func (f *fakeMouse) AfterDraw(*ebiten.Image)             {}
func (f *fakeMouse) CursorPosition() (int, int)          { return f.x, f.y }
func (f *fakeMouse) GetCursorImage(string) *ebiten.Image { return nil }
func (f *fakeMouse) GetCursorOffset(string) image.Point  { return image.Point{} }
func (f *fakeMouse) MouseButtonPressed(b ebiten.MouseButton) bool {
	return b == ebiten.MouseButtonLeft && f.pressed
}
func (f *fakeMouse) MouseButtonJustPressed(b ebiten.MouseButton) bool {
	return b == ebiten.MouseButtonLeft && f.justDown
}
func (f *fakeMouse) MouseButtonJustReleased(b ebiten.MouseButton) bool {
	return b == ebiten.MouseButtonLeft && f.justUp
}

// at moves the scripted cursor (frame N-1 state must be settled first).
func (f *fakeMouse) at(x, y int) { f.x, f.y = x, y }

// down/up press and release the left button with one-frame edge pulses.
func (f *fakeMouse) down() { f.pressed, f.justDown = true, true }
func (f *fakeMouse) up()   { f.pressed, f.justUp = false, true }

// useFakeMouse installs a scripted mouse for the test's duration.
func useFakeMouse(t *testing.T) *fakeMouse {
	t.Helper()
	f := &fakeMouse{}
	input.SetCursorUpdater(f)
	// The cursor renderer would reach for ebiten's window cursor; there
	// is no window in a test.
	prevMgmt := input.CursorManagementEnabled
	input.CursorManagementEnabled = false
	t.Cleanup(func() {
		input.SetCursorUpdater(nil)
		input.CursorManagementEnabled = prevMgmt
	})
	return f
}

// newChromeTestWindow builds a minimal titled window with a resize
// chrome, at a known rect. No WindowOpts.Draggable(): the title drag is
// chrome-owned like the grip (see winchrome.go), and this fixture pumps
// the chrome directly.
func newChromeTestWindow(t *testing.T) (*widget.Window, *winChrome) {
	t.Helper()
	env, _ := newTestEnv(t)
	body := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewRowLayout()))
	title := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout()))
	win := widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, 44),
		widget.WindowOpts.MinSize(320, 240),
		widget.WindowOpts.Location(image.Rect(100, 100, 500, 400)),
	)
	// The kit attaches the chrome on AddWindow; this window is pumped by
	// a bare UI instead, so the test holds the chrome directly.
	ch := &winChrome{win: win}
	// One pump on a real UI: layout gives the title bar and contents
	// their rects and installs the window's input layer (the grip needs
	// a topmost layer to test against).
	ui := newWindowUI(t, env, win)
	uitest.PumpWindow(t, ui, nil, 1280, 720)
	return win, ch
}

// TestWinChromeGripDrag drives the chrome end to end from the cursor and
// button state alone: a press in the grip band arms the drag, step()
// applies the drag math every frame (the win rect only moves through
// SetLocation), and release fires ResizeEvent exactly once with the
// final rect. The press point is inside the title bar too (the top-left
// corner always is), so the release also reports the move — both
// handlers fire, the grip owning the rect.
func TestWinChromeGripDrag(t *testing.T) {
	win, ch := newChromeTestWindow(t)

	// Count ResizeEvents and MoveEvents.
	resizes := 0
	var last image.Rectangle
	win.ResizeEvent.AddHandler(func(a interface{}) {
		if e, ok := a.(*widget.WindowChangedEventArgs); ok {
			resizes++
			last = e.Rect
		}
	})
	moves := 0
	var lastMove image.Rectangle
	win.MoveEvent.AddHandler(func(a interface{}) {
		if e, ok := a.(*widget.WindowChangedEventArgs); ok {
			moves++
			lastMove = e.Rect
		}
	})
	event.ExecuteDeferred()

	// Press on the left edge inside the title bar (3 px in, 5 px down):
	// this arms a left|top grip AND the title move — the grip wins while
	// it is armed or active, so the rect follows the resize math.
	ch.step(image.Point{X: 103, Y: 105}, true)
	if !ch.armed || ch.edges != edgeLeft|edgeTop {
		t.Fatalf("press in top-left grip: armed = %v edges = %b, want true / left|top", ch.armed, ch.edges)
	}
	if ch.active {
		t.Fatal("grip became active before the cursor moved")
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
	if moves != 0 {
		t.Errorf("MoveEvent fired mid-drag (%d times)", moves)
	}

	// Shrink below MinSize: clamped, right/bottom edges still fixed.
	ch.step(image.Point{X: 400, Y: 400}, true)
	rect = win.GetContainer().GetWidget().Rect
	if rect.Max.X != 500 || rect.Max.Y != 400 || rect.Dx() != 320 || rect.Dy() != 240 {
		t.Errorf("drag past MinSize: rect = %v, want 320×240 ending at (500,400)", rect)
	}

	// Release: one ResizeEvent with the final rect, and — because the
	// press was in the title bar as well — one MoveEvent (the corner
	// reports both, chrome idle again). The event queue is deferred
	// (ebitenui semantics), so flush it before counting.
	ch.step(image.Point{X: 400, Y: 400}, false)
	event.ExecuteDeferred()
	if ch.active || ch.armed || ch.moveArmed || ch.moveActive {
		t.Error("still dragging after release")
	}
	if resizes != 1 {
		t.Fatalf("ResizeEvent count = %d, want 1", resizes)
	}
	if last != win.GetContainer().GetWidget().Rect {
		t.Errorf("ResizeEvent rect = %v, want final %v", last, win.GetContainer().GetWidget().Rect)
	}
	if moves != 1 {
		t.Errorf("MoveEvent count = %d, want 1 (the press was inside the title bar)", moves)
	} else if lastMove != win.GetContainer().GetWidget().Rect {
		t.Errorf("MoveEvent rect = %v, want final %v", lastMove, win.GetContainer().GetWidget().Rect)
	}

	// A press in the window interior does not arm a resize or a move (the
	// title drag / content widgets own it).
	ch.step(image.Point{X: 300, Y: 300}, true)
	if ch.armed || ch.active {
		t.Error("interior press armed a resize")
	}
	if ch.moveArmed || ch.moveActive {
		t.Error("interior press armed a move")
	}
	ch.step(image.Point{X: 999, Y: 999}, false)
	if resizes != 1 {
		t.Errorf("ResizeEvent count after interior press = %d, want 1", resizes)
	}
	if moves != 1 {
		t.Errorf("MoveEvent count after interior press = %d, want 1", moves)
	}
}

// TestWinChromePressWithoutDragIsNoOp: a press that starts in the grip
// band and releases without moving never resizes and never fires
// ResizeEvent — the press belongs to the child widget. The bottom-right
// corner is not inside the title bar, so it reports no move either.
func TestWinChromePressWithoutDragIsNoOp(t *testing.T) {
	win, ch := newChromeTestWindow(t)
	resizes := 0
	win.ResizeEvent.AddHandler(func(interface{}) { resizes++ })
	moves := 0
	win.MoveEvent.AddHandler(func(interface{}) { moves++ })
	event.ExecuteDeferred()

	start := win.GetContainer().GetWidget().Rect
	ch.step(image.Point{X: start.Max.X - 3, Y: start.Max.Y - 3}, true) // bottom-right corner
	if !ch.armed {
		t.Fatal("corner press did not arm")
	}
	// Small jitter inside the press, then release: still no drag.
	ch.step(image.Point{X: start.Max.X - 3, Y: start.Max.Y - 3}, true)
	ch.step(image.Point{X: start.Max.X - 3, Y: start.Max.Y - 3}, false)
	event.ExecuteDeferred()
	if ch.armed || ch.active {
		t.Error("chrome still dragging after a click")
	}
	if got := win.GetContainer().GetWidget().Rect; got != start {
		t.Errorf("rect = %v, want unchanged %v", got, start)
	}
	if resizes != 0 {
		t.Errorf("ResizeEvent count = %d, want 0 for a click", resizes)
	}
	if moves != 0 {
		t.Errorf("MoveEvent count = %d, want 0 for a corner click outside the title bar", moves)
	}
}

// TestAddWindowChromeIsIdempotent: mounting a window twice keeps one
// chrome (and one registry entry) — the second AddWindow reuses the
// first one's attachment instead of stacking a second pump entry.
func TestAddWindowChromeIsIdempotent(t *testing.T) {
	k, _ := newTestKit(t) // NewKit mounts the overview and Selected Item
	if len(k.chromes) != 2 {
		t.Fatalf("kit starts with %d chromes, want 2 (overview + selected)", len(k.chromes))
	}
	log := NewLog(k.Env())
	first := k.AddWindow(log.Win())
	if len(k.chromes) != 3 {
		t.Fatalf("after one AddWindow: %d chromes, want 3", len(k.chromes))
	}
	again := k.AddWindow(log.Win())
	if len(k.chromes) != 3 {
		t.Fatalf("duplicate AddWindow grew the registry to %d chromes, want 3", len(k.chromes))
	}
	if ch := k.chromes[log.Win()]; ch == nil || ch.win != log.Win() {
		t.Fatalf("log chrome = %v, want the window's own chrome", ch)
	}
	first()
	again() // both handles are the same close path: still one detach
	if _, ok := k.chromes[log.Win()]; ok {
		t.Fatal("remove func left the log in the chrome registry")
	}
}

// TestTwoKitsKeepTheirOwnGrips: the chrome registry lives on the kit,
// so building a second kit (the save/load rebuild path) must not disturb
// the first one's grips — the old package-global registry was cleared by
// every NewKit, silently killing the first kit's windows.
func TestTwoKitsKeepTheirOwnGrips(t *testing.T) {
	k1, _ := newTestKit(t)
	fm := useFakeMouse(t)
	log1 := NewLog(k1.Env())
	k1.AddWindow(log1.Win())
	log1.Win().SetLocation(image.Rect(10, 430, 10+logDefaultW, 430+logDefaultH))
	pumpKit(t, k1, 1024, 700)
	pumpKit(t, k1, 1024, 700)
	before := len(k1.chromes)

	k2, _ := newTestKit(t)
	if len(k1.chromes) != before {
		t.Fatalf("second NewKit changed the first kit's registry: %d → %d", before, len(k1.chromes))
	}
	if ch := k1.chromes[log1.Win()]; ch == nil {
		t.Fatal("first kit lost its log chrome to the second kit")
	}
	if len(k2.chromes) != 2 {
		t.Errorf("second kit has %d chromes, want 2 (its own overview + selected)", len(k2.chromes))
	}

	// The first kit still pumps it: a press in the log's grip band arms
	// that kit's chrome (the second kit has no such window).
	r := log1.Win().GetContainer().GetWidget().Rect
	fm.at(r.Min.X+3, r.Min.Y+60)
	fm.down()
	pumpKit(t, k1, 1024, 700)
	if ch := k1.chromes[log1.Win()]; ch == nil || !ch.armed {
		t.Fatal("first kit no longer pumps the log's grip after a second kit was built")
	}
	fm.up()
	pumpKit(t, k1, 1024, 700)
}

// TestUnmountMidDragLeavesNoSideEffects: a grip drag interrupted by a
// hide (docked-style) or an unmount stops dead — the rect stops moving
// under a still-held button and no ResizeEvent ever fires. The registry
// entry goes with the unmount, so the window is not pumped any more.
func TestUnmountMidDragLeavesNoSideEffects(t *testing.T) {
	k, _ := newTestKit(t)
	fm := useFakeMouse(t)
	log := NewLog(k.Env())
	win := log.Win()
	remove := k.AddWindow(win)
	win.SetLocation(image.Rect(10, 430, 10+logDefaultW, 430+logDefaultH))
	pumpKit(t, k, 1024, 700)
	pumpKit(t, k, 1024, 700)
	resizes := countResizes(win)

	// Press in the left grip band and drag: the rect follows the cursor.
	r := win.GetContainer().GetWidget().Rect
	fm.at(r.Min.X+3, r.Min.Y+60)
	fm.down()
	pumpKit(t, k, 1024, 700) // rising edge: armed, no move yet
	fm.at(r.Min.X+43, r.Min.Y+60)
	pumpKit(t, k, 1024, 700) // active: the drag applies
	dragged := win.GetContainer().GetWidget().Rect
	if dragged == r {
		t.Fatalf("fixture: the grip drag never moved the window (rect %v)", r)
	}
	if resizes.n != 0 {
		t.Fatalf("ResizeEvent fired mid-drag (%d)", resizes.n)
	}

	// Hidden mid-drag: step refuses to move an invisible rect.
	win.GetContainer().GetWidget().SetVisibility(widget.Visibility_Hide)
	for i := 0; i < 3; i++ {
		pumpKit(t, k, 1024, 700)
	}
	if got := win.GetContainer().GetWidget().Rect; got != dragged {
		t.Errorf("hidden window moved mid-drag: %v → %v", dragged, got)
	}

	// Unmounted mid-drag (button still held): the chrome leaves the
	// registry, so nothing pumps the window any more.
	remove()
	if _, ok := k.chromes[win]; ok {
		t.Fatal("remove func left the window in the chrome registry")
	}
	for i := 0; i < 3; i++ {
		pumpKit(t, k, 1024, 700)
	}
	if got := win.GetContainer().GetWidget().Rect; got != dragged {
		t.Errorf("unmounted window moved while unmounted: %v → %v", dragged, got)
	}

	// Release after the unmount: still no ResizeEvent, still no move.
	fm.up()
	for i := 0; i < 2; i++ {
		pumpKit(t, k, 1024, 700)
	}
	if got := win.GetContainer().GetWidget().Rect; got != dragged {
		t.Errorf("rect moved on release: %v → %v", dragged, got)
	}
	if resizes.n != 0 {
		t.Errorf("ResizeEvent fired for the interrupted drag (%d), want 0", resizes.n)
	}
}

// resizeCounter tallies ResizeEvent firings on one window.
type resizeCounter struct {
	n    int
	last image.Rectangle
}

func countResizes(win *widget.Window) *resizeCounter {
	c := &resizeCounter{}
	win.ResizeEvent.AddHandler(func(a interface{}) {
		if e, ok := a.(*widget.WindowChangedEventArgs); ok {
			c.n++
			c.last = e.Rect
		}
	})
	return c
}

// dragAt runs a full press-drag-release through the REAL frame loop
// (kit Sync/Update/Draw with a scripted mouse) starting at from and
// ending at from+delta. It returns the window rect before the press.
func dragAt(t *testing.T, k *Kit, fm *fakeMouse, win *widget.Window, from image.Point, dx, dy int) image.Rectangle {
	t.Helper()
	start := win.GetContainer().GetWidget().Rect
	frame := func() { pumpKit(t, k, 1024, 700) }
	fm.at(from.X, from.Y)
	frame() // hover settles before the press
	fm.down()
	frame() // rising edge: the grip arms (no rect change yet)
	fm.at(from.X+dx, from.Y+dy)
	frame() // cursor moved: the drag is active and the rect applies
	fm.up()
	frame() // release: ResizeEvent fires (deferred)
	frame() // dispatch the deferred ResizeEvent
	return start
}

// TestOverviewLogEdgeGripsResize is the regression test for the two
// windows whose bodies used to swallow grip presses: the overview's
// table and the log's text area are scroll containers that elevate an
// input layer over the grip band. Now the body padding (ContentsPad = 8,
// exactly the grip band) insets both scroll layers clear of the band, so
// the press is no longer swallowed; the cases below still pin that every
// edge and corner resizes exactly like the pure resizeRect math.
func TestOverviewLogEdgeGripsResize(t *testing.T) {
	k, _ := newTestKit(t)
	fm := useFakeMouse(t)
	log := NewLog(k.Env())
	k.AddWindow(log.Win())
	// Park the Selected Item window clear of both test windows so the
	// grip under test is the topmost layer at its own press point, and
	// size the log to its minimum so the text area fills the body all
	// the way to the bottom grip band.
	k.sp.win.SetLocation(image.Rect(600, 436, 980, 696))
	logTestRect := image.Rect(10, 690-logMinH, 10+logDefaultW, 690)
	log.Win().SetLocation(logTestRect)
	pumpKit(t, k, 1024, 700)
	pumpKit(t, k, 1024, 700)

	ovRes := countResizes(k.ov.win)
	logRes := countResizes(log.Win())

	ovView := k.ov.tbl.ScrollForTest().ViewRect()
	taView := log.ta.GetWidget().Rect
	ovRect := k.ov.win.GetContainer().GetWidget().Rect
	logRect := log.Win().GetContainer().GetWidget().Rect
	if !ovView.In(ovRect) || !taView.In(logRect) {
		t.Fatalf("fixture: overview viewport %v not inside %v / log textarea %v not inside %v",
			ovView, ovRect, taView, logRect)
	}

	cases := []struct {
		name    string
		win     *widget.Window
		res     *resizeCounter
		from    image.Point
		dx, dy  int
		edges   edgeMask
		inBody  image.Rectangle // the press must sit in this full-bleed body
		swallow bool            // the body's layer must swallow that press
	}{
		{"overview left edge (table inset clear of the grip band)", k.ov.win, ovRes,
			image.Point{ovRect.Min.X + 6, ovView.Min.Y + 12}, -40, 0, edgeLeft, image.Rectangle{}, false},
		{"overview bottom edge (table inset clear of the grip band)", k.ov.win, ovRes,
			image.Point{(ovView.Min.X + ovView.Max.X) / 2, ovRect.Max.Y - 7}, 0, 30, edgeBottom, image.Rectangle{}, false},
		// The table now stops short of its own scrollbar gutter, so the
		// right-edge band over it is the (empty while the rows fit)
		// gutter, not the elevated scroll viewport — the press is not
		// swallowed and the grip still classifies.
		{"overview right edge through the scrollbar gutter", k.ov.win, ovRes,
			image.Point{ovRect.Max.X - 7, ovView.Min.Y + 12}, 25, 0, edgeRight, ovRect, false},
		{"overview bottom-right corner through the scrollbar gutter", k.ov.win, ovRes,
			image.Point{ovRect.Max.X - 7, ovRect.Max.Y - 7}, 20, 18, edgeRight | edgeBottom, ovRect, false},
		{"overview top edge (title bar)", k.ov.win, ovRes,
			image.Point{(ovRect.Min.X + ovRect.Max.X) / 2, ovRect.Min.Y + 4}, 0, -20, edgeTop, image.Rectangle{}, false},
		// The log's body nests two paddings (8 px body + 4 px text-area
		// scroll), so its inner scroll layer stops 12 px short of the
		// window edge and never covers the grip band — these cases pin
		// the resize math over a text-area-filled body.
		{"log left edge through the text area", log.Win(), logRes,
			image.Point{logRect.Min.X + 6, taView.Min.Y + 12}, -35, 0, edgeLeft, image.Rectangle{}, false},
		{"log right edge through the text area", log.Win(), logRes,
			image.Point{logRect.Max.X - 7, taView.Min.Y + 12}, 30, 0, edgeRight, image.Rectangle{}, false},
		{"log bottom edge through the text area", log.Win(), logRes,
			image.Point{(taView.Min.X + taView.Max.X) / 2, logRect.Max.Y - 7}, 0, 40, edgeBottom, image.Rectangle{}, false},
		{"log top-left corner (title bar)", log.Win(), logRes,
			image.Point{logRect.Min.X + 4, logRect.Min.Y + 4}, -12, -10, edgeLeft | edgeTop, image.Rectangle{}, false},
	}

	for _, tc := range cases {
		before := tc.res.n
		// Park the window under test back at its default rect so each
		// case starts from a known geometry.
		if tc.win == k.ov.win {
			k.ov.win.SetLocation(image.Rect(8+RailInset, 8, 388+RailInset, 417)) // the constructor default (380×409)
		} else {
			log.Win().SetLocation(logTestRect)
		}
		// These cases press and release inside table rows, and a repeat
		// click opens Show Info — a floating window that would block the
		// next grip (a window's input layer does not care about
		// visibility). Park it closed alongside the windows above.
		k.CloseInfo()
		pumpKit(t, k, 1024, 700)
		rect := tc.win.GetContainer().GetWidget().Rect
		if tc.inBody != (image.Rectangle{}) {
			if !tc.from.In(rect) || !tc.from.In(tc.inBody) {
				t.Fatalf("%s: press point %v not in window %v / body %v — "+
					"the test would miss the full-bleed case", tc.name, tc.from, rect, tc.inBody)
			}
		}
		// Root cause, asserted: where the body's elevated scroll layer
		// covers the pixel, left-button presses never reach the
		// TitleBar/Contents widgets the old route listened on.
		if tc.swallow {
			if l := tc.win.GetContainer().GetWidget().EffectiveInputLayer(); l.ActiveFor(tc.from.X, tc.from.Y, input.LayerEventTypeMouseButton) {
				t.Fatalf("%s: press %v is NOT swallowed for mouse events — fixture no "+
					"longer covers the defect", tc.name, tc.from)
			}
		}
		start := dragAt(t, k, fm, tc.win, tc.from, tc.dx, tc.dy)
		got := tc.win.GetContainer().GetWidget().Rect
		want := resizeRect(start, tc.dx, tc.dy, tc.edges, windowMin(tc.win, start))
		if got != want {
			t.Errorf("%s: rect = %v, want resizeRect math %v", tc.name, got, want)
		}
		if tc.res.n != before+1 {
			t.Errorf("%s: ResizeEvent count = %d, want %d", tc.name, tc.res.n, before+1)
		} else if tc.res.last != want {
			t.Errorf("%s: ResizeEvent rect = %v, want %v", tc.name, tc.res.last, want)
		}
	}
}

// TestRowClickNearGripEdgeDoesNotResize: a click that starts inside the
// grip band but never drags is a no-op for the chrome; a click on the
// closest row pixel to that band still reaches the row and selects.
func TestRowClickNearGripEdgeDoesNotResize(t *testing.T) {
	k, _ := newTestKit(t)
	fm := useFakeMouse(t)
	pumpKit(t, k, 1024, 700)
	pumpKit(t, k, 1024, 700)

	ovRes := countResizes(k.ov.win)
	ovRect := k.ov.win.GetContainer().GetWidget().Rect
	if len(k.ov.tbl.RowCts) == 0 {
		t.Fatal("overview has no rows")
	}
	row := k.ov.tbl.RowCts[0].GetWidget().Rect

	click := func(from image.Point) {
		fm.at(from.X, from.Y)
		pumpKit(t, k, 1024, 700)
		fm.down()
		pumpKit(t, k, 1024, 700)
		if got := k.ov.win.GetContainer().GetWidget().Rect; got != ovRect {
			t.Fatalf("press alone moved the window: %v -> %v", ovRect, got)
		}
		fm.up()
		pumpKit(t, k, 1024, 700)
		pumpKit(t, k, 1024, 700)
	}

	// The grip band (8 px) is CONTENTS padding now — the table is inset
	// clear of it, so a press in the band lands on the chrome, not a
	// row: no resize and no selection.
	bandFrom := image.Point{X: ovRect.Min.X + 6, Y: (row.Min.Y + row.Max.Y) / 2}
	if edgesAt(bandFrom, ovRect, edgeGripPx)&edgeLeft == 0 {
		t.Fatalf("fixture: press %v must sit in the left grip band of %v", bandFrom, ovRect)
	}
	click(bandFrom)
	if got := k.ov.win.GetContainer().GetWidget().Rect; got != ovRect {
		t.Errorf("click without drag resized the window: %v -> %v", ovRect, got)
	}
	if ovRes.n != 0 {
		t.Errorf("ResizeEvent count = %d, want 0 for a click", ovRes.n)
	}
	if got := k.Selected(); got != 0 {
		t.Errorf("selected = %v, want none (the grip band is chrome, not a row)", got)
	}

	// The closest row pixel to the band still gets the click and selects.
	rowFrom := image.Point{X: row.Min.X, Y: (row.Min.Y + row.Max.Y) / 2}
	if edgesAt(rowFrom, ovRect, edgeGripPx) != 0 {
		t.Fatalf("fixture: press %v must sit outside the grip band of %v", rowFrom, ovRect)
	}
	click(rowFrom)
	if got := k.ov.win.GetContainer().GetWidget().Rect; got != ovRect {
		t.Errorf("row click resized the window: %v -> %v", ovRect, got)
	}
	if ovRes.n != 0 {
		t.Errorf("ResizeEvent count = %d, want 0 for a row click", ovRes.n)
	}
	wantSel := k.ov.tbl.Rows[0].Data.(*ovRow).info.Entity
	if got := k.Selected(); got != wantSel {
		t.Errorf("selected = %v, want row 0 %v (the click reached the child)", got, wantSel)
	}
}

// TestTopmostWindowWinsGrip: two windows' grip bands overlap at the same
// pixel; only the topmost one (the later AddWindow) may start a resize.
func TestTopmostWindowWinsGrip(t *testing.T) {
	k, _ := newTestKit(t)
	fm := useFakeMouse(t)
	log := NewLog(k.Env())
	k.AddWindow(log.Win())
	pumpKit(t, k, 1024, 700)
	pumpKit(t, k, 1024, 700)

	// Stack the log so its left band and the overview's right band
	// share a pixel column.
	ovRect := k.ov.win.GetContainer().GetWidget().Rect
	logRect := image.Rect(ovRect.Max.X-4, 60, ovRect.Max.X-4+logDefaultW, 60+logDefaultH)
	log.Win().SetLocation(logRect)
	pumpKit(t, k, 1024, 700)

	ovRes := countResizes(k.ov.win)
	logRes := countResizes(log.Win())

	ovView := k.ov.tbl.ScrollForTest().ViewRect()
	from := image.Point{X: ovRect.Max.X - 2, Y: ovView.Min.Y + 12}
	if edgesAt(from, ovRect, edgeGripPx)&edgeRight == 0 ||
		edgesAt(from, log.Win().GetContainer().GetWidget().Rect, edgeGripPx)&edgeLeft == 0 {
		t.Fatalf("fixture: %v must be in both windows' grip bands (ov %v, log %v)",
			from, ovRect, log.Win().GetContainer().GetWidget().Rect)
	}

	start := dragAt(t, k, fm, k.ov.win, from, -30, 0)
	ovGot := k.ov.win.GetContainer().GetWidget().Rect
	logGot := log.Win().GetContainer().GetWidget().Rect
	if ovGot != start {
		t.Errorf("covered window resized: %v -> %v", start, ovGot)
	}
	if ovRes.n != 0 {
		t.Errorf("covered window fired %d ResizeEvents, want 0", ovRes.n)
	}
	if want := resizeRect(logRect, -30, 0, edgeLeft, windowMin(log.Win(), logRect)); logGot != want {
		t.Errorf("topmost window rect = %v, want %v", logGot, want)
	}
	if logRes.n != 1 {
		t.Errorf("topmost window ResizeEvent count = %d, want 1", logRes.n)
	}
}

// TestGripBandScalesWithUI: the resize grip measures its band with
// Px(edgeGripPx) in the same units the cursor arrives in, so at 1.5× a
// press 10 px inside the left edge still grips and one exactly 12 px in
// does not (the band grew with everything else).
func TestGripBandScalesWithUI(t *testing.T) {
	uitest.UseScale(t, 1.5)
	if theme.Px(edgeGripPx) != 12 {
		t.Fatalf("Px(edgeGripPx) at 1.5 = %d, want 12", theme.Px(edgeGripPx))
	}
	win, ch := newChromeTestWindow(t)
	rect := win.GetContainer().GetWidget().Rect

	inside := image.Point{X: rect.Min.X + 10, Y: rect.Min.Y + 50}
	if e := ch.gripStart(inside); e&edgeLeft == 0 {
		t.Errorf("press %v (10 px in, band %d): edges = %b, want edgeLeft", inside, theme.Px(edgeGripPx), e)
	}
	outside := image.Point{X: rect.Min.X + theme.Px(edgeGripPx), Y: rect.Min.Y + 50}
	if e := ch.gripStart(outside); e != 0 {
		t.Errorf("press %v (exactly band width in): edges = %b, want 0 (band is %d)",
			outside, e, theme.Px(edgeGripPx))
	}
}

// TestResizeGripCursorShapes: the chrome advertises the matching cursor
// on every grip — the EW/NS arrows on the four edges, the matching
// diagonal on the four corners (left+top / right+bottom NWSE,
// right+top / left+bottom NESW) — while hovering, while the press is
// held (armed) and while the drag is in flight (active). The write goes
// through the setCursorShape seam, so the assertion needs no OS cursor;
// the chrome re-writes the shape every frame it classifies a grip
// because ebitenui's AfterDraw resets the OS cursor from its own name
// cache each frame (which knows no diagonal names — see setResizeCursor).
func TestResizeGripCursorShapes(t *testing.T) {
	win, ch := newChromeTestWindow(t)
	rect := win.GetContainer().GetWidget().Rect
	if rect.Dx() == 0 || rect.Dy() == 0 {
		t.Fatalf("fixture: window never laid out (%v)", rect)
	}

	var got ebiten.CursorShapeType
	writes := 0
	restore := SetCursorShapeForTest(func(s ebiten.CursorShapeType) { got, writes = s, writes+1 })
	defer restore()

	midY := (rect.Min.Y + rect.Max.Y) / 2
	midX := (rect.Min.X + rect.Max.X) / 2
	cases := []struct {
		name string
		p    image.Point
		want ebiten.CursorShapeType
	}{
		{"left edge", image.Point{rect.Min.X + 2, midY}, ebiten.CursorShapeEWResize},
		{"right edge", image.Point{rect.Max.X - 2, midY}, ebiten.CursorShapeEWResize},
		{"top edge", image.Point{midX, rect.Min.Y + 2}, ebiten.CursorShapeNSResize},
		{"bottom edge", image.Point{midX, rect.Max.Y - 2}, ebiten.CursorShapeNSResize},
		{"top-left corner", image.Point{rect.Min.X + 2, rect.Min.Y + 2}, ebiten.CursorShapeNWSEResize},
		{"bottom-right corner", image.Point{rect.Max.X - 2, rect.Max.Y - 2}, ebiten.CursorShapeNWSEResize},
		{"top-right corner", image.Point{rect.Max.X - 2, rect.Min.Y + 2}, ebiten.CursorShapeNESWResize},
		{"bottom-left corner", image.Point{rect.Min.X + 2, rect.Max.Y - 2}, ebiten.CursorShapeNESWResize},
	}
	for _, c := range cases {
		if e := ch.gripStart(c.p); e == 0 {
			t.Fatalf("%s: fixture: %v is not in a grip band of %v", c.name, c.p, rect)
		}

		// Idle hover: the band advertises its own shape.
		got, writes = 0, 0
		ch.step(c.p, false)
		if writes == 0 || got != c.want {
			t.Errorf("%s: hover cursor = %v (%d writes), want %v", c.name, got, writes, c.want)
		}

		// Pressed and held without moving (armed): the shape stays up.
		got, writes = 0, 0
		ch.step(c.p, true) // rising edge: arms the grip
		ch.step(c.p, true) // held, cursor unmoved
		if writes == 0 || got != c.want {
			t.Errorf("%s: held cursor = %v (%d writes), want %v", c.name, got, writes, c.want)
		}
		ch.step(c.p, false) // release without a drag: a click, state resets
	}

	// Dragging: the active branch keeps the shape while the rect follows.
	corner := image.Point{rect.Max.X - 2, rect.Max.Y - 2}
	ch.step(corner, true)
	got, writes = 0, 0
	ch.step(corner.Add(image.Point{20, 20}), true)
	if writes == 0 || got != ebiten.CursorShapeNWSEResize {
		t.Errorf("dragging the bottom-right corner: cursor = %v (%d writes), want %v",
			got, writes, ebiten.CursorShapeNWSEResize)
	}
	ch.step(corner.Add(image.Point{20, 20}), false)

	// Clear of every band the chrome writes nothing: it must not claim
	// the cursor over the body.
	got, writes = 0, 0
	ch.step(image.Point{midX, midY}, false)
	if writes != 0 {
		t.Errorf("body hover wrote the cursor %d times (%v), want none", writes, got)
	}
}
