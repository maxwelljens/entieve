package game

import (
	"image"
	"testing"

	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestUISoundMenuOpen is the context-menu phase: the popup APPEARING speaks,
// through the real frame loop — the ⋯ header button is clicked through
// ebitenui's own input path. Dismissal stays silent on purpose: the press
// that dismisses the popup is the same gesture whose landing the click cue
// already reports.
//
// The open cue lands one frame after the popup appears (the menu's state is
// read at the frame's start, the same edge the host's gate reads).
func TestUISoundMenuOpen(t *testing.T) {
	mm := useLoopMouse(t)
	g := newUISoundGame(t, t.TempDir()+"/save.json", &fakeUISound{}, mm.pointer())
	f := g.ui.(*fakeUISound)
	pump(t, g, 1280, 720)
	img := ebiten.NewImage(1280, 720)

	frame := func() {
		t.Helper()
		if err := g.Update(); err != nil {
			t.Fatalf("Update: %v", err)
		}
		g.Draw(img)
	}
	// press clicks the left button at pt through the loop and settles the
	// frame the popup's open edge rides.
	press := func(pt image.Point) {
		mm.moveTo(pt.X, pt.Y)
		mm.left = true
		frame()
		mm.left = false
		frame()
		frame()
	}

	// 1. The ⋯ column menu: its appearance is one open.
	btn := g.kit.OverviewTable().MenuBtnForTest().GetWidget().Rect
	if btn.Empty() {
		t.Fatal("fixture: the overview ⋯ button never laid out")
	}
	dots := image.Pt(btn.Min.X+btn.Dx()/2, btn.Min.Y+btn.Dy()/2)
	f.plays = nil
	press(dots)
	if !g.kit.Menu().IsOpen() {
		t.Fatalf("the ⋯ press did not open the menu (dots %v)", dots)
	}
	// The press itself lands on the ⋯ control, so it is also the ordinary
	// click cue — then the popup's own open edge on the next frame.
	if len(f.plays) != 2 || f.plays[0] != sfxClick || f.plays[1] != sfxOpen {
		t.Fatalf("the ⋯ menu appearing = %v, want %q then %q", f.plays, sfxClick, sfxOpen)
	}

	// 2. Dismissal without a selection is silent: no close cue (the press
	//    itself lands on no control here).
	f.plays = nil
	press(image.Pt(theme.Px(600), theme.Px(60)))
	if g.kit.Menu().IsOpen() {
		t.Fatal("the dismiss press did not close the menu")
	}
	if len(f.plays) != 0 {
		t.Fatalf("a dismiss press played %v, want silence", f.plays)
	}

	// 3. A second menu in the same session is another open, and a press on
	//    its entry is the existing click (never a second open).
	f.plays = nil
	press(dots)
	if len(f.plays) != 2 || f.plays[0] != sfxClick || f.plays[1] != sfxOpen {
		t.Fatalf("a second menu = %v, want %q then %q", f.plays, sfxClick, sfxOpen)
	}
	entry := g.kit.Menu().RowRect(0)
	if entry.Empty() {
		t.Fatal("the open menu exposes no row rect")
	}
	f.plays = nil
	g.uiClick(Input{
		MouseX:      float64(entry.Min.X + entry.Dx()/2),
		MouseY:      float64(entry.Min.Y + entry.Dy()/2),
		JustClicked: [2]bool{true, false},
	})
	if len(f.plays) != 1 || f.plays[0] != sfxClick {
		t.Fatalf("press on a menu entry = %v, want one %q", f.plays, sfxClick)
	}
}
