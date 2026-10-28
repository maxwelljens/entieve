package screens

import (
	"image"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/event"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// moveCounter tallies MoveEvent firings on one window (the MoveEvent
// counterpart of resizeCounter).
type moveCounter struct {
	n    int
	last image.Rectangle
}

func countMoves(win *widget.Window) *moveCounter {
	c := &moveCounter{}
	win.MoveEvent.AddHandler(func(a interface{}) {
		if e, ok := a.(*widget.WindowChangedEventArgs); ok {
			c.n++
			c.last = e.Rect
		}
	})
	return c
}

// titleTestEnv builds the overview + Selected Item kit with the two
// windows parked disjoint, settles two pumps so the mount layout is
// done, and returns the overview (the window under drag).
func titleTestEnv(t *testing.T) (*Kit, *fakeMouse, *widget.Window) {
	t.Helper()
	fm := useFakeMouse(t)
	k, _ := newTestKitWith(t, widgets.NewPointer(fm))
	// Park the Selected Item window clear of the overview so the title
	// press below is the topmost input layer at its own pixel (the same
	// park TestOverviewLogEdgeGripsResize relies on for grips).
	k.sp.win.SetLocation(image.Rect(640, 460, 1004, 690))
	pumpKit(t, k, 1024, 700)
	pumpKit(t, k, 1024, 700)
	return k, fm, k.ov.win
}

// titlePressPoint returns a cursor position inside win's title bar and
// outside every grip band, asserting the fixture (a press outside those
// two constraints would silently test nothing).
func titlePressPoint(t *testing.T, win *widget.Window) image.Point {
	t.Helper()
	rect := win.GetContainer().GetWidget().Rect
	title := win.TitleBar.GetWidget().Rect
	from := image.Point{
		X: (rect.Min.X + rect.Max.X) / 2,
		Y: rect.Min.Y + theme.Px(edgeGripPx) + 12,
	}
	if !from.In(title) {
		t.Fatalf("fixture: press %v must sit in the title bar %v of %v", from, title, rect)
	}
	if e := edgesAt(from, rect, theme.Px(edgeGripPx)); e != 0 {
		t.Fatalf("fixture: press %v must stay outside the grip bands (edges = %b)", from, e)
	}
	return from
}

// TestTitleDragBehavior pins what a title-bar drag DOES, as an
// observable contract: the press alone moves nothing and fires nothing,
// the rect follows the cursor by exactly the press-to-cursor delta, the
// window's contents (the table body and its rows) translate with it,
// release fires exactly one MoveEvent carrying the final rect, a click
// on the title without movement fires one MoveEvent with the unchanged
// rect, a body press fires nothing, and a corner press runs the resize
// AND reports the move (both events, one each).
//
// The assertions are implementation-neutral on purpose: this is the
// same contract ebitenui's built-in WindowOpts.Draggable() and the
// chrome-owned translate drag must both satisfy, so it can be run
// against either to compare them.
func TestTitleDragBehavior(t *testing.T) {
	k, fm, win := titleTestEnv(t)
	res := countResizes(win)
	mvs := countMoves(win)

	start := win.GetContainer().GetWidget().Rect
	from := titlePressPoint(t, win)
	content := widgets.ScrollContentOf(k.ov.tbl.ScrollForTest())
	if content == nil {
		t.Fatal("fixture: overview table scroll content is not stashed (AttachScrollContent)")
	}
	content0 := content.GetWidget().Rect
	row0 := k.ov.tbl.RowCts[0].GetWidget().Rect
	frame := func() { pumpKit(t, k, 1024, 700) }

	// Hover settles, then press: armed, nothing moved, nothing fired.
	fm.at(from.X, from.Y)
	frame()
	fm.down()
	frame()
	if got := win.GetContainer().GetWidget().Rect; got != start {
		t.Fatalf("press alone moved the window: %v -> %v", start, got)
	}
	if mvs.n != 0 || res.n != 0 {
		t.Fatalf("press fired events: moves=%d resizes=%d, want 0/0", mvs.n, res.n)
	}

	// Drag in two steps: the rect tracks press-to-cursor exactly.
	to := from.Add(image.Pt(36, 16))
	fm.at(to.X, to.Y)
	frame()
	extra := image.Pt(8, 6)
	fm.at(to.X+extra.X, to.Y+extra.Y)
	frame()
	want := start.Add(to.Add(extra).Sub(from))
	if got := win.GetContainer().GetWidget().Rect; got != want {
		t.Errorf("after drag: rect = %v, want start+delta %v", got, want)
	}
	if mvs.n != 0 || res.n != 0 {
		t.Errorf("mid-drag events: moves=%d resizes=%d, want 0/0", mvs.n, res.n)
	}

	// The contents translate with the window in the SAME frame — by the
	// translate walk or by the built-in relayout cascade, both must
	// land on the same rects here.
	if got := content.GetWidget().Rect; got != content0.Add(want.Min.Sub(start.Min)) {
		t.Errorf("scroll content = %v, want %v", got, content0.Add(want.Min.Sub(start.Min)))
	}
	if got := k.ov.tbl.RowCts[0].GetWidget().Rect; got != row0.Add(want.Min.Sub(start.Min)) {
		t.Errorf("row 0 = %v, want %v", got, row0.Add(want.Min.Sub(start.Min)))
	}
	if got := win.GetContainer().GetWidget().Rect; got != want {
		t.Errorf("rect drifted after content assert: %v, want %v", got, want)
	}

	// Release: exactly one MoveEvent, final rect, no resize.
	fm.up()
	frame()
	frame()
	if mvs.n != 1 {
		t.Errorf("MoveEvent count = %d, want 1", mvs.n)
	} else if mvs.last != want {
		t.Errorf("MoveEvent rect = %v, want final %v", mvs.last, want)
	}
	if res.n != 0 {
		t.Errorf("ResizeEvent count = %d, want 0 (title drag is not a resize)", res.n)
	}

	// A click on the title with the button never leaving the press point
	// still reports the move once, with the unchanged rect — that is
	// what the built-in handler did on every title release.
	start = win.GetContainer().GetWidget().Rect
	click := titlePressPoint(t, win)
	fm.at(click.X, click.Y)
	frame()
	fm.down()
	frame()
	fm.up()
	frame()
	frame()
	if got := win.GetContainer().GetWidget().Rect; got != start {
		t.Errorf("title click moved the window: %v -> %v", start, got)
	}
	if mvs.n != 2 {
		t.Errorf("MoveEvent count after title click = %d, want 2", mvs.n)
	} else if mvs.last != start {
		t.Errorf("click MoveEvent rect = %v, want unchanged %v", mvs.last, start)
	}
	if res.n != 0 {
		t.Errorf("ResizeEvent count after title click = %d, want 0", res.n)
	}

	// A press in the window body (in the contents grid's padding, over
	// no widget that consumes it) belongs to the content: no move, no
	// resize.
	titleRect := win.TitleBar.GetWidget().Rect
	body := image.Point{
		X: (start.Min.X + start.Max.X) / 2,
		Y: titleRect.Max.Y + 2,
	}
	if !body.In(start) || body.In(titleRect) {
		t.Fatalf("fixture: body press %v must sit in %v and outside the title %v", body, start, titleRect)
	}
	if body.In(k.ov.tbl.ScrollForTest().ViewRect()) {
		t.Fatalf("fixture: body press %v must sit outside the table viewport (would select a row)", body)
	}
	if e := edgesAt(body, start, theme.Px(edgeGripPx)); e != 0 {
		t.Fatalf("fixture: body press %v must stay outside the grip bands (edges = %b)", body, e)
	}
	fm.at(body.X, body.Y)
	frame()
	fm.down()
	frame()
	fm.up()
	frame()
	frame()
	if got := win.GetContainer().GetWidget().Rect; got != start {
		t.Errorf("body press moved the window: %v -> %v", start, got)
	}
	if mvs.n != 2 || res.n != 0 {
		t.Errorf("body press fired events: moves=%d resizes=%d, want 2/0", mvs.n, res.n)
	}

	// A corner press arms BOTH: the grip owns the application (resize
	// math, opposite edge fixed) and the title drag reports the move —
	// the corner was inside the title bar, so the move fires too.
	corner := image.Point{X: start.Min.X + 3, Y: start.Min.Y + 3}
	if e := edgesAt(corner, start, theme.Px(edgeGripPx)); e != edgeLeft|edgeTop {
		t.Fatalf("fixture: corner press %v edges = %b, want left|top", corner, e)
	}
	if !corner.In(win.TitleBar.GetWidget().Rect) {
		t.Fatalf("fixture: corner press %v must sit in the title bar", corner)
	}
	d := image.Pt(14, 9)
	fm.at(corner.X, corner.Y)
	frame()
	fm.down()
	frame()
	fm.at(corner.X+d.X, corner.Y+d.Y)
	frame()
	fm.up()
	frame()
	frame()
	wantResize := resizeRect(start, d.X, d.Y, edgeLeft|edgeTop, windowMin(win, start))
	if got := win.GetContainer().GetWidget().Rect; got != wantResize {
		t.Errorf("corner drag: rect = %v, want resizeRect %v", got, wantResize)
	}
	if res.n != 1 {
		t.Errorf("corner ResizeEvent count = %d, want 1", res.n)
	}
	if mvs.n != 3 {
		t.Errorf("corner MoveEvent count = %d, want 3 (corner is also a title press)", mvs.n)
	} else if mvs.last != wantResize {
		t.Errorf("corner MoveEvent rect = %v, want %v", mvs.last, wantResize)
	}
	event.ExecuteDeferred()
}

// TestTitleDragSkipsRelayout pins the cost this whole change exists for:
// a translate-only title drag must never request a layout pass on the
// window. A layout pass rewrites its children's rects from the parent
// geometry, so a child displaced OUTSIDE that geometry keeps its
// displaced position only if no layout ran — the displaced title bar is
// the canary. (ebitenui's built-in Draggable() path fails this by
// construction: SetLocation → RequestRelayout → doLayout rewrites the
// canary every frame of the drag.)
func TestTitleDragSkipsRelayout(t *testing.T) {
	k, fm, win := titleTestEnv(t)
	res := countResizes(win)
	mvs := countMoves(win)

	start := win.GetContainer().GetWidget().Rect
	from := titlePressPoint(t, win)
	frame := func() { pumpKit(t, k, 1024, 700) }

	// Press FIRST (the arm test reads the title rect), settle the arm,
	// and only then displace the canary.
	fm.at(from.X, from.Y)
	frame()
	fm.down()
	frame()
	if got := win.GetContainer().GetWidget().Rect; got != start {
		t.Fatalf("press alone moved the window: %v -> %v", start, got)
	}
	displaced := win.TitleBar.GetWidget().Rect.Add(image.Pt(999, 999))
	win.TitleBar.GetWidget().Rect = displaced

	// Drag: the rect follows the press-to-cursor delta exactly…
	to := from.Add(image.Pt(30, 14))
	fm.at(to.X, to.Y)
	frame()
	want := start.Add(to.Sub(from))
	if got := win.GetContainer().GetWidget().Rect; got != want {
		t.Fatalf("after drag: rect = %v, want start+delta %v", got, want)
	}
	// …and the displaced title kept its displacement (+ the window's
	// delta): no layout pass rewrote it from parent geometry.
	if got := win.TitleBar.GetWidget().Rect; got != displaced.Add(want.Min.Sub(start.Min)) {
		t.Errorf("title bar = %v, want displaced+delta %v — a layout pass ran during a translate-only drag",
			got, displaced.Add(want.Min.Sub(start.Min)))
	}

	fm.up()
	frame()
	frame()
	if mvs.n != 1 {
		t.Errorf("MoveEvent count = %d, want 1", mvs.n)
	}
	if res.n != 0 {
		t.Errorf("ResizeEvent count = %d, want 0", res.n)
	}
}

// TestTitleNeverOverflowsItsBar is the reported title-bar bug: a long title
// must be ellipsized into the bar's safe width (past the dots and icon, clear
// of the × strip) instead of running under the × and out of the window.
func TestTitleNeverOverflowsItsBar(t *testing.T) {
	k, _ := newTestKit(t)
	env := k.Env()
	long := "Market — Cordite Ore 12.34 € · fee 3%+1% · cheapest Kern Place 11.00 €"
	bar, txt := theme.NewTitleBar(env, long, "svc_market")
	body := widget.NewContainer(widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(200), theme.Px(120))))
	win := widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(bar, theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(200), theme.Px(160)),
		widget.WindowOpts.Location(image.Rect(theme.Px(20), theme.Px(20), theme.Px(20+260), theme.Px(20+180))),
	)
	k.SetWindowClose(win, func() {}) // attaches the × the title must clear
	k.AddWindow(win)
	pumpKit(t, k, 1024, 700)
	pumpKit(t, k, 1024, 700)

	closeBtn := k.CloseButtonOf(win).GetWidget().Rect
	label := txt.GetWidget().Rect
	if closeBtn.Empty() || label.Empty() {
		t.Fatalf("fixture: × %v / label %v never laid out", closeBtn, label)
	}
	face, size := *env.FaceMono(theme.StepBase), env.TS().Base
	avail := float64(closeBtn.Min.X-theme.Px(theme.SpSm)) - float64(label.Min.X)
	if got := theme.MeasureTracked(long, face, size); got <= avail {
		t.Fatalf("fixture: the title measures %v in %v of room — it cannot overflow", got, avail)
	}
	shown := txt.Shown()
	if got := theme.MeasureTracked(shown, face, size); got > avail {
		t.Errorf("the title draws %v wide in %v of room: it runs %v px under the × strip %v",
			got, avail, got-avail, closeBtn)
	}
	if shown == long || !strings.HasSuffix(shown, "…") {
		t.Errorf("the drawn title = %q, want the ellipsized form of %q", shown, long)
	}

	// A title that fits is drawn verbatim (no stray ellipsis on the windows
	// whose base names are short).
	bar2, short := theme.NewTitleBar(env, "Market", "svc_market")
	win2 := widget.NewWindow(
		widget.WindowOpts.Contents(widget.NewContainer(widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(240), theme.Px(120))))),
		widget.WindowOpts.TitleBar(bar2, theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(320), theme.Px(160)),
		widget.WindowOpts.Location(image.Rect(theme.Px(300), theme.Px(20), theme.Px(620), theme.Px(200))),
	)
	k.SetWindowClose(win2, func() {})
	k.AddWindow(win2)
	pumpKit(t, k, 1024, 700)
	pumpKit(t, k, 1024, 700)
	if short.GetWidget().Rect.Empty() {
		t.Fatal("fixture: the short-titled bar never laid out")
	}
	if got := short.Shown(); got != "Market" {
		t.Errorf("a fitting title draws %q, want Market", got)
	}
}

// TestTitleBarStillRetitles pins the retitle capability the static titles
// keep (the user's "preserve the underlying functionality"): production
// passes base names, but setting the label still reaches the drawn title —
// verbatim while it fits.
func TestTitleBarStillRetitles(t *testing.T) {
	k, _ := newTestKit(t)
	env := k.Env()
	bar, txt := theme.NewTitleBar(env, "Market", "")
	win := widget.NewWindow(
		widget.WindowOpts.Contents(widget.NewContainer(widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(240), theme.Px(120))))),
		widget.WindowOpts.TitleBar(bar, theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(320), theme.Px(160)),
		widget.WindowOpts.Location(image.Rect(theme.Px(20), theme.Px(20), theme.Px(340), theme.Px(200))),
	)
	k.SetWindowClose(win, func() {})
	k.AddWindow(win)
	pumpKit(t, k, 1024, 700)
	pumpKit(t, k, 1024, 700)

	txt.Label = "Market — Ore"
	pumpKit(t, k, 1024, 700)
	if got := txt.Shown(); got != "Market — Ore" {
		t.Errorf("the drawn title = %q, want the retitled %q", got, txt.Label)
	}
}

// TestTitleBarSliverDrawsTheBareEllipsis: a bar narrower than the dots +
// icon lead-in leaves the label no room at all — it draws the bare "…", never
// the full title over the ×.
func TestTitleBarSliverDrawsTheBareEllipsis(t *testing.T) {
	k, _ := newTestKit(t)
	env := k.Env()
	long := "Market — Cordite Ore 12.34 € · fee 3%+1% · cheapest Kern Place 11.00 €"
	bar, txt := theme.NewTitleBar(env, long, "svc_market")
	win := widget.NewWindow(
		widget.WindowOpts.Contents(widget.NewContainer(widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(60), theme.Px(60))))),
		widget.WindowOpts.TitleBar(bar, theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(100), theme.Px(100)),
		widget.WindowOpts.Location(image.Rect(theme.Px(20), theme.Px(20), theme.Px(120), theme.Px(140))),
	)
	k.SetWindowClose(win, func() {})
	k.AddWindow(win)
	pumpKit(t, k, 1024, 700)
	pumpKit(t, k, 1024, 700)

	closeBtn := k.CloseButtonOf(win).GetWidget().Rect
	label := txt.GetWidget().Rect
	if closeBtn.Empty() || label.Empty() {
		t.Fatalf("fixture: × %v / label %v never laid out", closeBtn, label)
	}
	face, size := *env.FaceMono(theme.StepBase), env.TS().Base
	room := float64(closeBtn.Min.X-theme.Px(theme.SpSm)) - float64(label.Min.X)
	if room >= theme.MeasureTracked("M", face, size) {
		t.Fatalf("fixture: %v px of room still fits a glyph — the sliver case is untested", room)
	}
	// No room for a glyph: the bare "…" when even that fits, nothing when it
	// does not — never the full title over the ×.
	if got := txt.Shown(); got != "…" && got != "" {
		t.Errorf("a title with %v px of room draws %q, want the bare ellipsis or nothing", room, got)
	}
	if got := theme.MeasureTracked(txt.Shown(), face, size); got > max(room, 0) {
		t.Errorf("the drawn title measures %v in %v of room", got, room)
	}
}
