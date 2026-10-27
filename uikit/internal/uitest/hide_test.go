package uitest

import (
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestCaptureLoopStaysHidden pins the capture contract the user asked
// for: a gated capture loop must never show a window on the desktop.
// The game records IsWindowVisible on its first frame; RED shows the
// window visible (the loop runs without SetWindowVisible(false)).
// Gated like every other capture: the regular suite stays windowless
// and never touches a display.
func TestCaptureLoopStaysHidden(t *testing.T) {
	if os.Getenv("ENTIEVE_CAPTURE") == "" {
		t.Skip("set ENTIEVE_CAPTURE=1 to run a capture loop")
	}
	framed, visible := false, true
	RunCaptureLoop(t, "Entieve hidden-loop pin", &hidePinGame{
		onFrame: func() { framed, visible = true, ebiten.IsWindowVisible() },
	})
	if !framed {
		t.Fatal("the pin game never framed")
	}
	if visible {
		t.Error("the capture loop showed a window on the desktop")
	}
}

// hidePinGame frames once, then terminates on the next Update.
type hidePinGame struct {
	framed  bool
	done    bool
	onFrame func()
}

func (g *hidePinGame) Layout(ow, oh int) (int, int) { return ow, oh }

func (g *hidePinGame) Update() error {
	if g.done {
		return ebiten.Termination
	}
	return nil
}

func (g *hidePinGame) Draw(*ebiten.Image) {
	if g.done {
		return
	}
	if g.onFrame != nil {
		g.onFrame()
	}
	g.done = true
}
