package game

import (
	"image"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// The popup owns the press that lands it — for EVERY consumer, not just for
// the widgets ebitenui gates while Kit.Update skips ui.Update:
//
//   - ebitenui's cached input advances on that frame too (Kit.Update's skip
//     branch), or the unspent press edge fires on the widget under the popup
//     as soon as ui.Update runs again — the row behind "show info";
//   - every raw press poller in the kit reads widgets.PressEdge, which the
//     popup claims (Menu.Click/Esc → Menu.TakeBlocked), so the starmap, the
//     sliders, the split panes and the column grips never see it either.
//
// These pins drive one human click (press, held frames, release — the hold
// is what a real click has, and what makes a parked ebitenui edge surface)
// through the real frame loop.

// popupGesture is the frame rig a popup click needs headless: the scripted
// mouse (ebitenui's cache plus the kit's live seams, exactly as
// menu_click_loop_test's loopMouse describes) and the frame pump.
type popupGesture struct {
	g    *Game
	mm   *loopMouse
	edge bool
	img  *ebiten.Image
}

func newPopupGesture(t *testing.T, g *Game, w, h int) *popupGesture {
	t.Helper()
	p := &popupGesture{g: g, mm: useLoopMouse(t), img: pump(t, g, w, h)}
	t.Cleanup(widgets.SetPollCursorForTest(func() (int, int) { return p.mm.x, p.mm.y }))
	t.Cleanup(widgets.SetPressEdgeForTest(func(b ebiten.MouseButton) bool {
		return p.edge && b == ebiten.MouseButtonLeft
	}))
	return p
}

func (p *popupGesture) frame(t *testing.T) {
	t.Helper()
	if err := p.g.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	p.g.Draw(p.img)
}

// click is one human click at pt: the press frame, two frames still held,
// then the release and the frame after it.
func (p *popupGesture) click(t *testing.T, pt image.Point) {
	t.Helper()
	p.mm.moveTo(pt.X, pt.Y)
	p.mm.left, p.edge = true, true
	p.frame(t)
	p.edge = false
	p.frame(t)
	p.frame(t)
	p.mm.left = false
	p.frame(t)
	p.frame(t)
}

// centre is a rect's middle: where a click lands.
func centre(r image.Rectangle) image.Point {
	return image.Pt(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)
}

// TestMenuEntryClickDoesNotReachTheRowBehind is the user's report, end to
// end: a dropdown over an overview row, "Show Info" pressed. The entry runs
// exactly once (and speaks no second click cue), while the row under the
// popup stays untouched: no press, no click, no selection change.
func TestMenuEntryClickDoesNotReachTheRowBehind(t *testing.T) {
	resetScale(t)
	sfx := &fakeUISound{}
	g := newUISoundGame(t, t.TempDir()+"/save.json", sfx)
	p := newPopupGesture(t, g, 1280, 720)

	tbl := g.kit.OverviewTable()
	row := tbl.RowWidgetForTest(1)
	if row.Rect.Empty() {
		t.Fatal("fixture: overview row never laid out")
	}
	presses, clicks := 0, 0
	row.MouseButtonPressedEvent.AddHandler(func(any) { presses++ })
	row.MouseButtonClickedEvent.AddHandler(func(any) { clicks++ })

	fired := 0
	g.Do(widgets.OpenMenu{Entries: []widgets.MenuEntry{{
		Label:  "Show Info",
		Bold:   true,
		Action: func() { fired++ },
	}}, At: centre(row.Rect)})
	p.frame(t)

	entry := g.kit.Menu().RowRect(0)
	if !centre(entry).In(row.Rect) {
		t.Fatalf("fixture: entry band %v does not sit over row %v", entry, row.Rect)
	}
	sfx.plays = nil
	p.click(t, centre(entry))

	if fired != 1 {
		t.Fatalf("the Show Info entry ran %d times, want exactly 1", fired)
	}
	if g.kit.Menu().IsOpen() {
		t.Error("the entry press left the popup open")
	}
	if presses != 0 || clicks != 0 {
		t.Errorf("the row behind the popup took the entry press: %d presses, %d clicks, want 0/0", presses, clicks)
	}
	if got := tbl.SelIdxForTest(); got != -1 {
		t.Errorf("the row behind the popup was selected: selIdx = %d, want -1", got)
	}
	if got := g.kit.Selected(); got != 0 {
		t.Errorf("the entry press selected an entity: %v, want none", got)
	}
	if n := countCues(sfx.plays, sfxClick); n > 1 {
		t.Errorf("the gesture spoke %d click cues (%v), want at most 1", n, sfx.plays)
	}
}

// TestColumnMenuDismissPressDoesNotReachTheTableRows is the same rule through
// the second dropdown the user named: any press while the ⋯ column menu is up
// belongs to the popup, including the one that only dismisses it. The table
// row under that press — the popup rect never gates it — must stay untouched.
func TestColumnMenuDismissPressDoesNotReachTheTableRows(t *testing.T) {
	resetScale(t)
	g := newShowcaseGame(t)
	p := newPopupGesture(t, g, 1280, 720)

	tbl := g.kit.OverviewTable()
	dots := tbl.MenuBtnForTest()
	row := tbl.RowWidgetForTest(1)
	if dots.GetWidget().Rect.Empty() || row.Rect.Empty() {
		t.Fatal("fixture: the overview's ⋯ button or a row never laid out")
	}
	presses, clicks := 0, 0
	row.MouseButtonPressedEvent.AddHandler(func(any) { presses++ })
	row.MouseButtonClickedEvent.AddHandler(func(any) { clicks++ })

	dots.ClickedEvent.Fire(&widget.ButtonClickedEventArgs{Button: dots, OffsetX: 1, OffsetY: 1})
	p.frame(t)
	if !g.kit.Menu().IsOpen() {
		t.Fatal("the ⋯ button did not open the column menu")
	}
	if centre(row.Rect).In(g.kit.Menu().Rect()) {
		t.Fatalf("fixture: row %v sits inside the popup %v", row.Rect, g.kit.Menu().Rect())
	}

	p.click(t, centre(row.Rect))

	if g.kit.Menu().IsOpen() {
		t.Error("the dismiss press left the column menu open")
	}
	if presses != 0 || clicks != 0 {
		t.Errorf("the row took the popup's dismiss press: %d presses, %d clicks, want 0/0", presses, clicks)
	}
	if got := tbl.SelIdxForTest(); got != -1 {
		t.Errorf("the dismiss press selected the row behind: selIdx = %d, want -1", got)
	}
}

// countCues counts one cue name in a fake seam's play log.
func countCues(plays []string, name string) int {
	n := 0
	for _, p := range plays {
		if p == name {
			n++
		}
	}
	return n
}
