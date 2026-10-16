package game

import (
	"image"
	"testing"

	"github.com/ebitenui/ebitenui/input"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// This test closes the gap the panel-level wiring tests leave open: they
// call Menu.Click / the handlers DIRECTLY, while a player drives the menu
// through the real Game.Update frame loop with real input edges.
//
// Seam choices (headless runs cannot generate OS input, and the menu is
// opened by ebitenui widget events, NOT by the scene Input snapshot):
//
//   - Cursor + button state for the WIDGETS (the right-click that opens
//     the menu) go through ebitenui's CursorUpdater seam
//     (input.SetCursorUpdater) — the same seam uikit/winchrome_test
//     uses. loopMouse below MIRRORS ebitenui's internal handler contract
//     exactly: raw scripted state is cached on Update(), which ebitenui
//     calls only from UI.Update (input.Update). CursorPosition/Pressed/
//     JustPressed all read that cache — and while a menu is open
//     Kit.Update skips ui.Update, so the cache freezes at the open
//     anchor. That frozen-cache dynamic is what the production bug rode
//     on; the test keeps reproducing it faithfully for the widgets.
//
//   - Menu.poll's cursor read is NOT that cache: production reads the
//     cursor live (ebiten.CursorPosition), so the same scripted raw
//     position feeds uikit.SetPollCursorForTest. loopMouse's raw x,y is
//     that position — one scripted "OS", two reads, as in production.
//
//   - The press EDGE that Menu.poll dispatches on comes from raw
//     ebiten inpututil (no injection path exists headless), so it is
//     delivered through the test-only setter uikit.SetPressEdgeForTest.
//     The edge is non-consuming per tick, like inpututil's: it stays
//     set for the whole click frame, covering both the Game.Update and
//     the Kit.Update poll passes.
//
// loopMouse is ebitenui's CursorUpdater seam with the DEFAULT internal
// handler's caching semantics: raw scripted state becomes visible only
// when Update() runs (ebitenui calls it from UI.Update via input.Update),
// and every reader (CursorPosition, MouseButtonPressed, JustPressed)
// reads the cache.
type loopMouse struct {
	// raw scripted state (the test's "OS")
	x, y  int
	left  bool
	right bool

	// cache — refreshed only in Update()
	cx, cy              int
	leftNow, rightNow   bool
	leftPrev, rightPrev bool
	leftJust, rightJust bool
	leftRel, rightRel   bool
}

func (m *loopMouse) Update() {
	m.cx, m.cy = m.x, m.y
	m.leftJust = m.left && m.left != m.leftPrev
	m.rightJust = m.right && m.right != m.rightPrev
	m.leftRel = !m.left && m.left != m.leftPrev
	m.rightRel = !m.right && m.right != m.rightPrev
	m.leftNow, m.rightNow = m.left, m.right
	m.leftPrev, m.rightPrev = m.left, m.right
}

// AfterUpdate mirrors the internal handler: it clears only chars/wheel,
// never the pressed/just state (that persists until the next Update).
func (m *loopMouse) AfterUpdate()               {}
func (m *loopMouse) Draw(*ebiten.Image)         {}
func (m *loopMouse) AfterDraw(*ebiten.Image)    {}
func (m *loopMouse) CursorPosition() (int, int) { return m.cx, m.cy }

func (m *loopMouse) MouseButtonPressed(b ebiten.MouseButton) bool {
	switch b {
	case ebiten.MouseButtonLeft:
		return m.leftNow
	case ebiten.MouseButtonRight:
		return m.rightNow
	}
	return false
}

func (m *loopMouse) MouseButtonJustPressed(b ebiten.MouseButton) bool {
	switch b {
	case ebiten.MouseButtonLeft:
		return m.leftJust
	case ebiten.MouseButtonRight:
		return m.rightJust
	}
	return false
}

func (m *loopMouse) MouseButtonJustReleased(b ebiten.MouseButton) bool {
	switch b {
	case ebiten.MouseButtonLeft:
		return m.leftRel
	case ebiten.MouseButtonRight:
		return m.rightRel
	}
	return false
}

func (m *loopMouse) GetCursorImage(string) *ebiten.Image { return nil }
func (m *loopMouse) GetCursorOffset(string) image.Point  { return image.Point{} }

// moveTo scripts the cursor (raw); the ebitenui cache picks it up on the
// next UI.Update — i.e. never while a menu is up — while Menu.poll's
// live seam (SetPollCursorForTest) sees it immediately, as in production.
func (m *loopMouse) moveTo(x, y int) { m.x, m.y = x, y }

// useLoopMouse installs the scripted mouse for the test's duration.
func useLoopMouse(t *testing.T) *loopMouse {
	t.Helper()
	m := &loopMouse{}
	input.SetCursorUpdater(m)
	// No window in a test: the cursor renderer must not reach for ebiten.
	prevMgmt := input.CursorManagementEnabled
	input.CursorManagementEnabled = false
	t.Cleanup(func() {
		input.SetCursorUpdater(nil)
		input.CursorManagementEnabled = prevMgmt
	})
	return m
}

// TestMenuClickDispatchesThroughUpdateLoop drives the context menu the
// way a player does: right-click a wired row through the real widget
// input path inside Game.Update (menu opens), move onto an entry,
// left-click it — and the entry's action must fire. The action surface
// is the contract board's objective label: its menu leads with a bold
// Show Info whose Action opens the item-details window (Kit.InfoOpen).
//
// Red condition this pins: the menu is open, a left-press edge is
// delivered on the entry's row rect through the production poll, and the
// action still does not run.
func TestMenuClickDispatchesThroughUpdateLoop(t *testing.T) {
	g := dockedShowcaseGame(t, 1280, 720)
	mm := useLoopMouse(t)
	img := ebiten.NewImage(1280, 720)

	// The press-edge seam: armed only for the click frame below.
	clickEdge := false
	defer widgets.SetPressEdgeForTest(func(b ebiten.MouseButton) bool {
		return b == ebiten.MouseButtonLeft && clickEdge
	})()

	// The menu's live-cursor seam: Menu.poll reads the scripted raw
	// position, exactly as production reads ebiten's cursor.
	defer widgets.SetPollCursorForTest(func() (int, int) { return mm.x, mm.y })()

	frame := func() {
		t.Helper()
		if err := g.Update(); err != nil {
			t.Fatalf("Update: %v", err)
		}
		g.Draw(img)
	}

	// 1. Cursor onto the objective label of a wired row.
	row := findRow(t, g.stAvailRows, "contract-solace-cleanup-1")
	objective := row.root.Children()[1].GetWidget()
	if objective.Rect.Empty() {
		t.Fatal("fixture: objective label never laid out")
	}
	mm.moveTo(objective.Rect.Min.X+objective.Rect.Dx()/2,
		objective.Rect.Min.Y+objective.Rect.Dy()/2)

	// 2. Right-press, right-release: the production open path
	//    (widget fireEvents inside Kit.Update → OpenMenu). The press
	//    frame also refreshes the cursor cache onto the label.
	mm.right = true
	frame()
	mm.right = false
	frame()

	if !g.kit.Menu().IsOpen() {
		t.Fatal("right-click did not open the context menu through Game.Update")
	}
	entries := g.kit.Menu().Entries()
	if len(entries) != 1 || entries[0].Label != "Show Info" || entries[0].Disabled {
		t.Fatalf("menu after right-click = %+v, want the single enabled Show Info", entries)
	}

	// 3. Move onto entry 0 (the scripted raw cursor — Menu.poll reads
	//    it live) and click it through the production poll
	//    (Game.Update's Kit.MenuInput pass).
	rr := g.kit.Menu().RowRect(0)
	openRect := g.kit.Menu().Rect()
	if rr.Empty() {
		t.Fatal("open menu exposes no row rect")
	}
	mm.moveTo(rr.Min.X+rr.Dx()/2, rr.Min.Y+rr.Dy()/2)

	// 3b. Hover — the second symptom of the same stale-cursor root: the
	//     highlight must follow the live cursor onto row 0, not stay
	//     frozen at the open anchor.
	frame()
	if h := g.kit.Menu().HoverIndex(); h != 0 {
		t.Errorf("hover index = %d after moving onto RowRect(0), want 0 "+
			"(stuck at the open anchor %v?)", h, openRect.Min)
	}

	// The click: one left-press edge on entry 0's row rect.
	clickEdge = true
	frame()
	clickEdge = false
	frame()

	// 4. The entry's action must have run.
	if !g.kit.InfoOpen() {
		t.Errorf("menu entry click did not dispatch: menu was open with %q at %v, "+
			"left-press edge delivered at RowRect(0)=%v, but Kit.InfoOpen()=false "+
			"(menu open after click: %v)",
			entries[0].Label, openRect, rr, g.kit.Menu().IsOpen())
	}
}
