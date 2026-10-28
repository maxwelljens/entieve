package screens

import (
	"image"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// Click-anywhere-to-raise, on the kit's own seam: any mouse press (left
// or right) inside a window — body, title bar or resize chrome — re-tops
// that window through remove + re-mount, the topmost of two overlapping
// windows wins the click, and neither the modal context menu, top chrome
// (the rail) nor a modal window ever triggers a raise it does not own.
//
// The raise reads the raw press edge and cursor through widgets' shared
// test seams (the same ones Menu.Poll reads). This pump installs no
// ebitenui cursor updater, so widget-side presses fire nowhere — the
// only consumer of the scripted edge is the raise, which is exactly the
// unit under test here; the real input path (rail over a window) is
// pinned game-side in TestRailClicksWinOverAWindowUnderneath.

// raiseMouse scripts the pointer frame the raise polls. btn narrows the
// edge to one button (widgets.Left = both, unless only is set).
type raiseMouse struct {
	x, y int
	edge bool
	btn  widgets.Button // widgets.Left reports the edge for every button unless only
	only bool
}

// raiseMouse is a widgets.Source: the kit's frame reads it.
func (m *raiseMouse) Cursor() (int, int)       { return m.x, m.y }
func (m *raiseMouse) Down(widgets.Button) bool { return false } // the raise reads edges only
func (m *raiseMouse) JustPressed(b widgets.Button) bool {
	return m.edge && (!m.only || b == m.btn)
}
func (m *raiseMouse) SetShape(ebiten.CursorShapeType) {}

func useRaiseMouse(t *testing.T) *raiseMouse {
	t.Helper()
	return &raiseMouse{}
}

// press fires one scripted press edge at pt and runs one kit frame.
func press(t *testing.T, k *Kit, m *raiseMouse, pt image.Point) {
	t.Helper()
	m.x, m.y = pt.X, pt.Y
	m.edge = true
	defer func() { m.edge = false }()
	k.Pointer().Begin() // the host's sample for this frame
	if err := k.Update(); err != nil {
		t.Fatalf("Update at %v: %v", pt, err)
	}
}

// raiseFixture builds a kit whose two core windows overlap: the overview
// at its default rect, Selected Item re-parked over its right half —
// then pumped so every rect and input layer has settled. Returns the kit
// and the overview-only point, overlap point and title/grip points the
// cases drive (derived from the live rects, so they stay inside at any
// scale).
func raiseFixture(t *testing.T, m *raiseMouse) (k *Kit, ov, overlap, title, grip image.Point) {
	t.Helper()
	k, _ = newTestKitWith(t, widgets.NewPointer(m))
	ovW, spW := k.OverviewWindow(), k.SelectedWindow()
	ovr := ovW.GetContainer().GetWidget().Rect
	spr := spW.GetContainer().GetWidget().Rect
	mid := ovr.Min.X + ovr.Dx()/2
	spW.SetLocation(image.Rect(mid, ovr.Min.Y+40, mid+spr.Dx(), ovr.Min.Y+40+spr.Dy()))
	pumpKit(t, k, 1280, 720)
	pumpKit(t, k, 1280, 720)

	ov = image.Pt(ovr.Min.X+20, ovr.Min.Y+ovr.Dy()/2)  // overview body, clear of the overlap
	overlap = image.Pt(mid+10, ovr.Min.Y+60)           // inside BOTH windows
	title = image.Pt(ovr.Min.X+20, ovr.Min.Y+6)        // the title bar band
	grip = image.Pt(ovr.Min.X+2, ovr.Min.Y+ovr.Dy()/2) // the left resize grip band
	return k, ov, overlap, title, grip
}

// topWindow returns the kit's topmost mounted window.
func topWindow(k *Kit) *widget.Window { return k.stack[len(k.stack)-1].win }

// assertTop pins the top of the stack (failures name the window by its
// rect, since the kit windows carry no title).
func assertTop(t *testing.T, k *Kit, want *widget.Window, what string) {
	t.Helper()
	if got := topWindow(k); got != want {
		t.Errorf("%s: top of stack = the window at %v, want the one at %v",
			what, got.GetContainer().GetWidget().Rect, want.GetContainer().GetWidget().Rect)
	}
}

// TestPressInsideWindowRaisesIt walks the raise through every surface a
// press can land on: the body, the overlap, the title bar and the resize
// grip. The overlap runs both ways — whichever of the two windows is on
// TOP is the topmost under the cursor and holds the stack (a bottom-up
// hit test would let the buried one steal), and a buried window rises
// from its own visible body. A raise never moves the window.
func TestPressInsideWindowRaisesIt(t *testing.T) {
	m := useRaiseMouse(t)
	k, bodyPt, overlap, titlePt, gripPt := raiseFixture(t, m)
	ov, sp := k.OverviewWindow(), k.SelectedWindow()
	// Mount order: NewKit mounts the overview first, Selected Item on top.
	assertTop(t, k, sp, "fixture")

	ovRect := ov.GetContainer().GetWidget().Rect
	spOnly := overlap.Add(image.Pt(240, 0)) // inside Selected Item only (past the overview's right edge)

	// The body: this is the defect — ebitenui's own relayering never saw
	// presses here (the body's scroll layer covers the window's).
	press(t, k, m, bodyPt)
	assertTop(t, k, ov, "press on the body")

	// Both windows contain this point and the overview is on top: it is
	// the topmost under the cursor, so it holds — the buried Selected
	// Item must not steal the raise.
	press(t, k, m, overlap)
	assertTop(t, k, ov, "overlap press keeps the topmost window")

	// The buried Selected Item rises from ITS own body (outside the
	// overlap, where it alone sits under the cursor).
	press(t, k, m, spOnly)
	assertTop(t, k, sp, "press on the buried window's body")

	// Same overlap, now with Selected Item on top: it holds.
	press(t, k, m, overlap)
	assertTop(t, k, sp, "overlap press keeps the new topmost window")

	// The title bar keeps raising (the surface ebitenui's built-in path
	// already handled).
	press(t, k, m, titlePt)
	assertTop(t, k, ov, "press on the title bar")

	// Reset, then press the overview's resize grip band — the chrome is
	// part of the window, so it raises too.
	press(t, k, m, spOnly)
	assertTop(t, k, sp, "reset: Selected Item on top")
	press(t, k, m, gripPt)
	assertTop(t, k, ov, "press on the resize chrome")

	// A raise never moves the window: the rect is exactly what the
	// fixture laid out.
	if got := ov.GetContainer().GetWidget().Rect; got != ovRect {
		t.Errorf("raise moved the overview: rect = %v, want %v", got, ovRect)
	}
	// A press clear of every window raises nothing (the top stays put).
	press(t, k, m, image.Pt(1200, 650))
	assertTop(t, k, ov, "press clear of every window")
}

// TestRightPressRaises: the right button raises exactly like the left —
// the context-menu gesture re-stacks the window it landed on.
func TestRightPressRaises(t *testing.T) {
	m := useRaiseMouse(t)
	m.only, m.btn = true, widgets.Right
	k, bodyPt, _, _, _ := raiseFixture(t, m)
	ov, sp := k.OverviewWindow(), k.SelectedWindow()
	assertTop(t, k, sp, "fixture")

	press(t, k, m, bodyPt)
	assertTop(t, k, ov, "right press on the body")
}

// TestRaiseNeverFiresThroughTheMenu: while the context menu is up it
// owns the frame — the dismiss click is consumed by Menu.Poll's blocked
// latch (or the open guard when no edge fired), and the raise must skip
// with ui.Update. The dismiss click can never re-stack what lies beneath
// the popup.
func TestRaiseNeverFiresThroughTheMenu(t *testing.T) {
	m := useRaiseMouse(t)
	k, bodyPt, _, _, _ := raiseFixture(t, m)
	sp := k.SelectedWindow()
	assertTop(t, k, sp, "fixture")

	// Menu up at the body point: the press is consumed as the dismiss,
	// blocked-latch first.
	k.Menu().Open([]widgets.MenuEntry{{Label: "Show Info"}, widgets.MenuSep, {Label: "Warp"}}, bodyPt)
	press(t, k, m, bodyPt)
	if k.Menu().IsOpen() {
		t.Fatal("fixture: the press did not dismiss the menu (latch path not exercised)")
	}
	assertTop(t, k, sp, "dismiss click through an open menu")

	// Menu reopened over a DIFFERENT window point: same result.
	k.Menu().Open([]widgets.MenuEntry{{Label: "Show Info"}}, bodyPt)
	press(t, k, m, k.Menu().RowRect(0).Min)
	assertTop(t, k, sp, "menu entry click")
	if k.Menu().IsOpen() {
		t.Error("menu stayed open after its entry click")
	}
}

// TestTopChromeOwnsItsRect: a press inside the rail's column (top
// chrome) never raises the window parked underneath it — the rail owns
// its rect outright, paint AND input — while a press on the same window
// clear of the rail still raises it.
func TestTopChromeOwnsItsRect(t *testing.T) {
	m := useRaiseMouse(t)
	k, _, _, _, _ := raiseFixture(t, m)
	ov, sp := k.OverviewWindow(), k.SelectedWindow()

	open := false
	r := NewRail(k.Env(), railEntries3(&open))
	k.AddTopContainer(r, widget.AnchorLayoutData{
		HorizontalPosition: widget.AnchorLayoutPositionStart,
		VerticalPosition:   widget.AnchorLayoutPositionStart,
	})
	pumpKit(t, k, 1280, 720)
	rr := r.Container().GetWidget().Rect
	if rr.Dx() != theme.Px(RailInset) || rr.Dy() <= 0 {
		t.Fatalf("fixture: rail column = %v, want the RailInset-wide gutter %d px", rr, theme.Px(RailInset))
	}

	// Drag the overview onto the gutter: it now sits under the rail AND
	// under a point inside both.
	ov.SetLocation(image.Rect(0, 0, 400, 400))
	pumpKit(t, k, 1280, 720)
	underRail := rr.Min.Add(image.Pt(10, 16))
	if !underRail.In(ov.GetContainer().GetWidget().Rect) {
		t.Fatalf("fixture: press point %v not inside the dragged overview %v",
			underRail, ov.GetContainer().GetWidget().Rect)
	}

	press(t, k, m, underRail)
	assertTop(t, k, sp, "press inside the rail column")

	// Control: the same window raises from a point clear of the rail.
	press(t, k, m, image.Pt(200, 200))
	assertTop(t, k, ov, "press on the overview clear of the rail")
}

// TestRaiseStopsAtAModalWindow: a modal window mounted above the rest
// owns every gesture — a press on a window beneath it must not rise
// through it. Drop the modal flag and the same press raises (the gate,
// not the geometry, was holding it back).
func TestRaiseStopsAtAModalWindow(t *testing.T) {
	m := useRaiseMouse(t)
	k, bodyPt, _, _, _ := raiseFixture(t, m)
	ov := k.OverviewWindow()

	log := NewLog(k.Env())
	log.Win().Modal = true
	k.AddWindow(log.Win())
	log.Win().SetLocation(image.Rect(800, 400, 1088, 640))
	pumpKit(t, k, 1280, 720)
	if got := topWindow(k); got != log.Win() {
		t.Fatalf("fixture: top of stack = %v, want the modal log", got.GetContainer().GetWidget().Rect)
	}

	press(t, k, m, bodyPt)
	assertTop(t, k, log.Win(), "press below a modal window")

	log.Win().Modal = false
	press(t, k, m, bodyPt)
	assertTop(t, k, ov, "same press once the modal flag is gone")
}
