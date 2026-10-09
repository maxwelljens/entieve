package game

import (
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ui"
	"github.com/hajimehoshi/ebiten/v2"
)

// newSmokeGame builds a game session over the embedded content.
func newSmokeGame(t *testing.T) *Game {
	t.Helper()
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	g, err := New(Options{Content: c})
	if err != nil {
		t.Fatalf("game.New: %v", err)
	}
	return g
}

// TestSceneDrawSmoke draws every scene (space world + UI, help overlay,
// starmap overlay, menu, station chrome) at the design and minimum window
// sizes — headless offscreen only; geometry/index bugs surface as panics.
func TestSceneDrawSmoke(t *testing.T) {
	for _, size := range [][2]int{{1280, 720}, {1024, 700}} {
		g := newSmokeGame(t)
		g.w, g.h = size[0], size[1]
		img := ebiten.NewImage(size[0], size[1])

		// Let the sim populate the grid (visible entities, effects).
		for i := 0; i < 300; i++ {
			g.s.Step()
		}
		g.Draw(img)

		// Help overlay: full two-column key map (with the C/scanner and
		// mouse-gesture additions).
		g.u.SetWindowOpen(ui.WinHelp, true)
		hs := &HelpScene{g: g}
		hs.OnEnter()
		hs.Draw(img)

		// Starmap overlay.
		g.u.SetWindowOpen(ui.WinStarmap, true)
		ms := &MapScene{g: g}
		ms.Draw(img)

		// Esc menu + station undock chrome.
		menu := &MenuScene{g: g, mx: -1, my: -1}
		menu.Draw(img)
		st := &StationScene{g: g}
		st.Draw(img)
	}
}

// TestHelpLayoutFits: the help overlay's content (every KeyMap + MouseMap
// line at the derived line pitch) fits its window at the minimum size.
func TestHelpLayoutFits(t *testing.T) {
	g := newSmokeGame(t)
	g.w, g.h = 1024, 700
	panel := g.u.WindowRect(ui.WinHelp, g.u.Layout())
	contentH := panel.H - ui.TitleBarH() - 1
	pitch := ui.LineH(ui.FontTiny)
	keys := (len(KeyMap) + 1) / 2
	needed := 10 + float64(keys)*pitch + 10 + pitch + float64(len(MouseMap))*pitch
	if needed > contentH {
		t.Fatalf("help content needs %v px, window content has %v px",
			needed, contentH)
	}
}
