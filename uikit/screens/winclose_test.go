package screens

import (
	"image"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// The title-bar close button (CONTEXT.md "Close button"): every
// closable window carries one × through Kit.SetWindowClose, the press
// never starts the chrome-owned drag (the grip keeps corner
// precedence), the registered close runs through its EXISTING semantics,
// the context menu's frame can never fire it, and a Show Info closed by
// it keeps its rect across the reopen.

// settlePump runs frames until no animation is running (bounded — an
// animation is at most openFrames long; the extra frame clears the
// final composite).
func settlePump(t *testing.T, k *Kit, w, h int) {
	t.Helper()
	for i := 0; i < 40; i++ {
		if len(k.anims) == 0 {
			return
		}
		pumpKit(t, k, w, h)
	}
	t.Fatalf("animation still running after 40 frames: %d left", len(k.anims))
}

// closeCenter is the press point for the ×: the button's centre, clear
// of every grip band (the fixture asserts that — a press inside a band
// would test the grip instead).
func closeCenter(t *testing.T, win *widget.Window) image.Point {
	t.Helper()
	b := win.TitleBar.GetWidget().Rect
	r := closeButtonRect(b)
	p := image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	if e := edgesAt(p, win.GetContainer().GetWidget().Rect, theme.Px(edgeGripPx)); e != 0 {
		t.Fatalf("fixture: × press %v sits in a grip band (%b) — pick a bar clear of them", p, e)
	}
	return p
}

// clickAt scripts a press+release at pt through the real pipeline.
func clickAt(t *testing.T, k *Kit, fm *fakeMouse, w, h int, pt image.Point) {
	t.Helper()
	fm.at(pt.X, pt.Y)
	pumpKit(t, k, w, h)
	fm.down()
	pumpKit(t, k, w, h)
	fm.up()
	pumpKit(t, k, w, h)
}

// TestCloseButtonAttachedToEveryClosableWindow: the kit registers the
// two core windows and Show Info itself, a host registers its panels —
// each registration attaches exactly one ×, sized to the ~40 px design
// hit box and sitting inside both the title bar and the chrome's
// exclusion strip (so the widget the player presses and the strip the
// drag excludes are the same region). A window without a title bar
// registers its close but attaches no button (the station services view
// is the production case).
func TestCloseButtonAttachedToEveryClosableWindow(t *testing.T) {
	k, _ := newTestKit(t)
	pumpKit(t, k, 1280, 720)

	for _, win := range []*widget.Window{k.OverviewWindow(), k.SelectedWindow(), k.InfoWindow()} {
		btn := k.CloseButtonOf(win)
		if btn == nil {
			t.Fatalf("window at %v has no × button", win.GetContainer().GetWidget().Rect)
		}
		_, mounted := k.mountOf(win)
		if ch, ok := k.chromes[win]; ok {
			if !ch.closable {
				t.Errorf("chrome closable = false for a registered window")
			}
		} else if mounted {
			t.Errorf("mounted window has no chrome")
		}
		if !mounted {
			continue // never laid out (Show Info before its first Show): existence only
		}
		title := win.TitleBar.GetWidget().Rect
		br := btn.GetWidget().Rect
		if br.Dx() < theme.Px(40) || br.Dy() < theme.Px(40) {
			t.Errorf("× hit box = %v, want ≥ %d×%d design units", br, 40, 40)
		}
		if !br.In(title) {
			t.Errorf("× rect %v outside title bar %v", br, title)
		}
		if !br.In(closeButtonRect(title)) {
			t.Errorf("× rect %v outside the chrome's exclusion strip %v", br, closeButtonRect(title))
		}
	}

	// A host-registered panel.
	log := NewLog(k.Env())
	k.AddWindow(log.Win())
	k.SetWindowClose(log.Win(), func() {})
	if k.CloseButtonOf(log.Win()) == nil {
		t.Error("panel registered after mount has no ×")
	}
	if !k.chromes[log.Win()].closable {
		t.Error("panel chrome did not pick up closable")
	}

	// No title bar: close registered, no button.
	bare := widget.NewWindow(widget.WindowOpts.Contents(
		widget.NewContainer()))
	k.SetWindowClose(bare, func() {})
	if k.CloseButtonOf(bare) != nil {
		t.Error("title-less window attached a ×")
	}
}

// TestCloseButtonStripExcludesDrag pins the pure geometry the chrome
// excludes: the strip is the title bar's right closeBtnW columns, full
// height; an empty title excludes nothing.
func TestCloseButtonStripExcludesDrag(t *testing.T) {
	title := image.Rect(100, 100, 500, 144)
	got := closeButtonRect(title)
	want := image.Rect(500-theme.Px(closeBtnW), 100, 500, 144)
	if got != want {
		t.Errorf("closeButtonRect = %v, want %v", got, want)
	}
	if p := image.Pt(500-theme.Px(closeBtnW)-1, 120); p.In(got) {
		t.Errorf("point %v left of the strip is inside it", p)
	}
	if r := closeButtonRect(image.Rectangle{}); !r.Empty() {
		t.Errorf("empty title: strip = %v, want empty", r)
	}
}

// TestCloseButtonPressNeverStartsADrag: a press on the × arms neither
// the title drag nor the grip (the point is clear of the bands), the
// window never moves, and the release runs the registered close — the
// window starts its close fade. The synthetic half pins the chrome's
// own classification: a strip press outside the grip bands arms nothing
// for a closable window, while the same press on an unclosable bar
// still arms the drag as before.
func TestCloseButtonPressNeverStartsADrag(t *testing.T) {
	k, _ := newTestKit(t)
	fm := useFakeMouse(t)
	log := NewLog(k.Env())
	closes := 0
	k.AddWindow(log.Win())
	k.SetWindowClose(log.Win(), func() { closes++ })
	settlePump(t, k, 1024, 700)

	win := log.Win()
	start := win.GetContainer().GetWidget().Rect
	ch := k.chromes[win]

	// A strip point outside the actual button (its bottom edge row) and
	// clear of every band: synthetic chrome classification.
	title := win.TitleBar.GetWidget().Rect
	strip := image.Pt((closeButtonRect(title).Min.X+title.Max.X)/2, title.Max.Y-1)
	if strip.In(k.CloseButtonOf(win).GetWidget().Rect) {
		t.Fatalf("fixture: %v should sit outside the button", strip)
	}
	ch.step(strip, true)
	if ch.moveArmed || ch.armed {
		t.Fatalf("strip press armed drag state: move=%v grip=%v", ch.moveArmed, ch.armed)
	}
	ch.step(strip, false)
	// The same press on a bar whose window is not closable arms the
	// drag — the exclusion is the button's, not the geometry's.
	plain, plainCh := newChromeTestWindow(t)
	plainCh.closable = false
	pt := image.Pt(plain.GetContainer().GetWidget().Rect.Max.X-theme.Px(closeBtnW)/2,
		plain.GetContainer().GetWidget().Rect.Min.Y+theme.Px(edgeGripPx)+6)
	if !plainCh.moveStart(pt) {
		t.Fatalf("fixture: %v must arm the drag on an unclosable bar", pt)
	}

	// Full pipeline: press the ×, nothing moves, no drag arms.
	p := closeCenter(t, win)
	fm.at(p.X, p.Y)
	pumpKit(t, k, 1024, 700)
	fm.down()
	pumpKit(t, k, 1024, 700)
	if ch.moveArmed || ch.armed {
		t.Fatalf("× press armed drag state: move=%v grip=%v", ch.moveArmed, ch.armed)
	}
	if got := win.GetContainer().GetWidget().Rect; got != start {
		t.Fatalf("× press moved the window: %v -> %v", start, got)
	}
	fm.up()
	pumpKit(t, k, 1024, 700)
	if closes != 1 {
		t.Fatalf("close count = %d, want 1", closes)
	}
	if got := win.GetContainer().GetWidget().Rect; got != start {
		t.Errorf("× close moved the window: %v -> %v", start, got)
	}
	if a, ok := k.anims[win]; !ok || a.open {
		t.Errorf("× did not commit the close fade: ok=%v open=%v", ok, a)
	}
}

// TestCloseButtonGripKeepsCornerPrecedence: where the × strip meets the
// grip bands (the title's top-right corner) a DRAG is still a resize —
// the grip classifies first — and releasing away from the button never
// closes. Releasing on the corner without moving is a click on the ×
// and closes: the strip is the button's, the drag is the grip's.
func TestCloseButtonGripKeepsCornerPrecedence(t *testing.T) {
	k, _ := newTestKit(t)
	fm := useFakeMouse(t)
	log := NewLog(k.Env())
	closes := 0
	k.AddWindow(log.Win())
	k.SetWindowClose(log.Win(), func() { closes++ })
	settlePump(t, k, 1024, 700)
	win := log.Win()
	ch := k.chromes[win]

	rect := win.GetContainer().GetWidget().Rect
	corner := image.Pt(rect.Max.X-3, rect.Min.Y+3)
	if e := edgesAt(corner, rect, theme.Px(edgeGripPx)); e != edgeRight|edgeTop {
		t.Fatalf("fixture: corner %v edges = %b, want right|top", corner, e)
	}
	if !corner.In(closeButtonRect(win.TitleBar.GetWidget().Rect)) {
		t.Fatalf("fixture: corner %v must sit in the × strip", corner)
	}

	// Corner drag: grip arms (move does not — the strip excludes it),
	// the rect follows the resize math, and releasing away closes nothing.
	fm.at(corner.X, corner.Y)
	pumpKit(t, k, 1024, 700)
	fm.down()
	pumpKit(t, k, 1024, 700)
	if !ch.armed || ch.edges != edgeRight|edgeTop {
		t.Fatalf("corner press: grip armed=%v edges=%b, want true/right|top", ch.armed, ch.edges)
	}
	if ch.moveArmed {
		t.Fatal("corner press armed the title drag from the × strip")
	}
	d := image.Pt(-30, 12)
	fm.at(corner.X+d.X, corner.Y+d.Y)
	pumpKit(t, k, 1024, 700)
	fm.up()
	pumpKit(t, k, 1024, 700)
	want := resizeRect(rect, d.X, d.Y, edgeRight|edgeTop, windowMin(win))
	if got := win.GetContainer().GetWidget().Rect; got != want {
		t.Errorf("corner drag: rect = %v, want resizeRect %v", got, want)
	}
	if closes != 0 {
		t.Errorf("corner drag closed the window (%d times) — the grip owns drags here", closes)
	}

	// Corner click, no movement: the × fires (its whole strip is the
	// hit area), the grip's click stays the no-op it always was.
	rect = win.GetContainer().GetWidget().Rect
	corner = image.Pt(rect.Max.X-3, rect.Min.Y+3)
	fm.at(corner.X, corner.Y)
	pumpKit(t, k, 1024, 700)
	fm.down()
	pumpKit(t, k, 1024, 700)
	fm.up()
	pumpKit(t, k, 1024, 700)
	if closes != 1 {
		t.Errorf("corner click closed %d times, want 1", closes)
	}
	if got := win.GetContainer().GetWidget().Rect; got != want {
		t.Errorf("corner click moved the window: %v -> %v", want, got)
	}
}

// TestCloseButtonCannotFireThroughTheMenu: the context menu owns its
// frame — ui.Update is skipped while it is up, so the × widget sees
// neither the press nor the release of a click aimed through the popup.
// (The existing raise contract covers the re-stacking half; this is the
// close half of the same gate.)
func TestCloseButtonCannotFireThroughTheMenu(t *testing.T) {
	k, _ := newTestKit(t)
	fm := useFakeMouse(t)
	log := NewLog(k.Env())
	closes := 0
	k.AddWindow(log.Win())
	k.SetWindowClose(log.Win(), func() { closes++ })
	settlePump(t, k, 1024, 700)

	k.Menu().Open([]widgets.MenuEntry{{Label: "Warp"}, {Label: "Orbit"}},
		closeCenter(t, log.Win()))
	p := closeCenter(t, log.Win())
	clickAt(t, k, fm, 1024, 700, p)
	if closes != 0 {
		t.Errorf("× fired through the context menu's frame (%d closes)", closes)
	}
}

// TestCloseButtonPreservesShowInfoRect: a Show Info closed by its ×
// unmounts at settle (its rect survives — the layout sidecar reads it
// back) and the next Show lands on the exact same rect, fading back in.
func TestCloseButtonPreservesShowInfoRect(t *testing.T) {
	k, _ := newTestKit(t)
	fm := useFakeMouse(t)
	k.ShowInfo(widgets.ItemRef("charge-emp"))
	if !k.InfoOpen() {
		t.Fatal("fixture: Show Info did not open")
	}
	settlePump(t, k, 1280, 720)
	win := k.InfoWindow()
	r0 := win.GetContainer().GetWidget().Rect
	if r0.Empty() {
		t.Fatalf("fixture: info rect %v empty", r0)
	}

	clickAt(t, k, fm, 1280, 720, closeCenter(t, win))
	if k.InfoOpen() {
		t.Fatal("× did not close the Show Info window")
	}
	settlePump(t, k, 1280, 720)
	if _, mounted := k.mountOf(win); mounted {
		t.Fatal("info window still mounted after the close fade settled")
	}

	k.ShowInfo(widgets.ItemRef("charge-emp"))
	if a, ok := k.anims[win]; !ok || !a.open {
		t.Fatalf("reopened Show Info is not fading in: ok=%v open=%v", ok, a)
	}
	settlePump(t, k, 1280, 720)
	if !k.InfoOpen() {
		t.Error("reopened Show Info is not open after the fade")
	}
	if got := win.GetContainer().GetWidget().Rect; got != r0 {
		t.Errorf("rect after close→reopen = %v, want the persisted %v", got, r0)
	}
}
