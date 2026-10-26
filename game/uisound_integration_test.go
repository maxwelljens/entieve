package game

import (
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestUISoundIntegration drives all five cues through the REAL Game wiring
// (New with Options.UISound, then the exact functions Update runs) — the
// arbiter the host-level pins are not: it catches a hook never wired, a
// cue that never reaches the seam, and a click gate that fires from
// anywhere on a window.
func TestUISoundIntegration(t *testing.T) {
	resetScale(t)
	f := &fakeUISound{}
	g := newUISoundGame(t, t.TempDir()+"/save.json", f)
	pump(t, g, 1280, 720)
	// The boot composition (the log's default mount) is not an open.
	if len(f.plays) != 0 {
		t.Fatalf("boot played %v, want silence", f.plays)
	}

	// open / close: Esc mounts the menu scene's window and unmounts it.
	g.pressKeys(ebiten.KeyEscape)
	if len(f.plays) != 1 || f.plays[0] != sfxOpen {
		t.Fatalf("Esc menu open = %v, want one %q", f.plays, sfxOpen)
	}
	g.pressKeys(ebiten.KeyEscape)
	if len(f.plays) != 2 || f.plays[1] != sfxClose {
		t.Fatalf("Esc menu close = %v, want a following %q", f.plays, sfxClose)
	}

	// A live scale change rebuilds the interface and remounts the windows
	// that were already open: those are not opens, so no cue.
	f.plays = nil
	g.setUIScale(1.25)
	pump(t, g, 1280, 720)
	if len(f.plays) != 0 {
		t.Errorf("a scale rebuild replayed %v, want silence", f.plays)
	}

	// confirm: a quantity verb through the game's own sink.
	f.plays = nil
	g.Do(widgets.Refine{Ore: "ore-cordite", Qty: 10})
	if len(f.plays) != 1 || f.plays[0] != sfxConfirm {
		t.Errorf("Refine through Game.Do = %v, want one confirm", f.plays)
	}

	// refuse: a fixed chord while a keybindings row is armed.
	f.plays = nil
	g.beginCapture("settings")
	g.pressKeys(ebiten.KeyF9)
	if len(f.plays) != 1 || f.plays[0] != sfxRefuse {
		t.Errorf("fixed chord = %v, want one refuse", f.plays)
	}
	g.cancelCapture()

	// click: a press on the settings window's own × (a real button), after
	// that window's mount spoke its open.
	f.plays = nil
	g.host.SetOpen(idSettings, true)
	pump(t, g, 1280, 720)
	if len(f.plays) != 1 || f.plays[0] != sfxOpen {
		t.Fatalf("settings mount = %v, want one open", f.plays)
	}
	f.plays = nil
	btn := g.kit.CloseButtonOf(g.host.Window(idSettings)).GetWidget().Rect
	g.uiClick(Input{
		MouseX:      float64(btn.Min.X + btn.Dx()/2),
		MouseY:      float64(btn.Min.Y + btn.Dy()/2),
		JustClicked: [2]bool{true, false},
	})
	if len(f.plays) != 1 || f.plays[0] != sfxClick {
		t.Errorf("press on the settings × = %v, want one click", f.plays)
	}
}

// TestUISoundIntegrationTitle is the title-phase half: a title window
// (Settings, mounted through the title host) opens and closes with its
// cues too.
func TestUISoundIntegrationTitle(t *testing.T) {
	resetScale(t)
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	wd, ok := c.World("world-showcase")
	if !ok {
		t.Fatal("world-showcase missing")
	}
	f := &fakeUISound{}
	g, err := New(Options{
		Content: c, World: wd, Seed: 42, SavePath: t.TempDir() + "/save.json",
		StartInTitle: true, UISound: f,
	})
	if err != nil {
		t.Fatalf("game.New: %v", err)
	}
	g.applySize(1280, 720)
	g.titleUI = g.ensureTitleUI()
	f.plays = nil

	if err := g.updateTitle(Input{JustPressed: []ebiten.Key{ebiten.KeyP}}); err != nil {
		t.Fatalf("updateTitle: %v", err)
	}
	if len(f.plays) != 1 || f.plays[0] != sfxOpen {
		t.Fatalf("title settings open = %v, want one %q", f.plays, sfxOpen)
	}
	if err := g.updateTitle(Input{JustPressed: []ebiten.Key{ebiten.KeyEscape}}); err != nil {
		t.Fatalf("updateTitle: %v", err)
	}
	if len(f.plays) != 2 || f.plays[1] != sfxClose {
		t.Fatalf("title settings close = %v, want a following %q", f.plays, sfxClose)
	}
}

// pressKeys feeds one frame's key press through the production key pass
// and pumps the frame that follows it — what a real keyboard press does
// across one Update (the input snapshot itself is the only part a headless
// test cannot fake).
func (g *Game) pressKeys(keys ...ebiten.Key) {
	_ = g.handleKeys(Input{JustPressed: keys, KeysDown: keys})
	_ = g.Update()
}
