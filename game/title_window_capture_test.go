package game

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/internal/capture"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// The two title-window captures (ENTIEVE_CAPTURE=1, one ebiten loop per
// process): the redesigned Keybindings and Credits windows as a player
// sees them over the artwork title backdrop. Both settle the open fade
// before the
// capture — the fade is 12 frames, the first window frame lands at 4, so
// frame 24 is fully settled (the known fade-phase flake).
//
// Output: $ENTIEVE_CAPTURE_DIR (default /tmp/opencode)/
// keybinds_capture.png and credits_capture.png.
func TestTitleKeybindingsCapture(t *testing.T) {
	runTitleWindowCapture(t, "keybindings", "keybinds_capture.png")
}

func TestTitleCreditsCapture(t *testing.T) {
	runTitleWindowCapture(t, "credits", "credits_capture.png")
}

// runTitleWindowCapture drives one title window to its settled frame,
// checks it is really on screen with its tracked chrome, and writes it.
func runTitleWindowCapture(t *testing.T, which, name string) {
	g := newTitleGameAt(t, filepath.Join(t.TempDir(), "save.json"))
	g.applySize(1280, 720)

	out := capture.Out(name)
	var (
		frames     int // Updates
		draws      int
		opened     bool
		openedDraw int
		winPx      int
		headers    int
	)
	var cap *capture.Capture
	cap = &capture.Capture{
		Name: "Entieve " + which + " capture",
		Update: func(int) error {
			frames++
			if !opened && frames == 4 {
				g.titleUI.openID(ScreenID(which))
				opened, openedDraw = true, draws // the fade counts DRAWN frames
			}
			return g.Update()
		},
		Draw: func(_ int, img *ebiten.Image) error {
			g.Draw(img)
			// The fade is 12 DRAWN frames (kit openFrames) and Draw is not
			// called on every Update: wait 20 draws after the open, so the
			// window is always settled before the capture and the bytes stay
			// deterministic.
			draws++
			if !opened || draws < openedDraw+20 {
				return nil
			}
			px, hdrs, err := captureTitleWindow(g, which, img)
			winPx, headers = px, hdrs
			cap.Stop()
			return err
		},
		Out: out,
	}
	cap.Run(t)
	t.Logf("%s capture %s (%d window pixels, %d headers)", which, out, winPx, headers)
}

// captureTitleWindow pins the window: it paints over the title backdrop
// frame, its tracked headers are there, and (keybindings) its sections are
// the three the action tables derive.
func captureTitleWindow(g *Game, which string, img *ebiten.Image) (winPx, headers int, err error) {
	var win *widget.Window
	var page widget.PreferredSizeLocateableWidget
	switch which {
	case "keybindings":
		win = g.titleUI.host.Window(idKeybindings)
		if kb := bodyOf[keybindPanel](g.titleUI.host, idKeybindings); kb.Keybindings != nil {
			page = widgets.ScrollContentOf(kb.ScrollForTest())
		}

	case "credits":
		cp := bodyOf[creditPanel](g.titleUI.host, idCredits)
		win = cp.Win()
		page = widgets.ScrollContentOf(cp.scroll)
	}
	if win == nil || page == nil {
		return 0, 0, fmt.Errorf("%s window did not mount (title=%v open=%v mounted=%v)",
			which, g.title, g.titleUI.host.IsOpen(ScreenID(which)),
			g.titleUI.host.Mounted(ScreenID(which)))
	}
	r := win.GetContainer().GetWidget().Rect
	if r.Dx() <= 0 || r.Dy() <= 0 || r.Max.X > 1280 || r.Max.Y > 720 {
		return 0, 0, fmt.Errorf("%s window rect = %v, want an on-screen size", which, r)
	}
	base := ebiten.NewImage(1280, 720)
	g.drawTitleBackdrop(base)
	for y := r.Min.Y; y < r.Max.Y; y += 2 {
		for x := r.Min.X; x < r.Max.X; x += 2 {
			if img.At(x, y) != base.At(x, y) {
				winPx++
			}
		}
	}
	if winPx < 500 {
		return winPx, 0, fmt.Errorf("%s window not painted: %d changed pixels (rect %v)", which, winPx, r)
	}

	// The tracked headers: the window's own section set.
	got := []string{}
	if c, ok := page.(widget.Containerer); ok {
		for _, kid := range c.Children() {
			if ct, ok := kid.(*theme.ChromeText); ok {
				got = append(got, ct.Label)
			}
		}
	}
	headers = len(got)
	want := keybindHeaders(g)
	if which == "credits" {
		want = creditHeaders()
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		return winPx, headers, fmt.Errorf("%s headers = %v, want %v", which, got, want)
	}
	return winPx, headers, nil
}

// keybindHeaders is the keybindings window's tracked chrome, in draw
// order, derived from the same rows the panel draws from.
func keybindHeaders(g *Game) []string {
	var out []string
	seen := map[string]bool{}
	for _, r := range g.host.services.Keys.Rows() {
		if !seen[r.Group] {
			seen[r.Group] = true
			out = append(out, r.Group)
		}
	}
	return out
}

// creditHeaders is the credits window's tracked chrome: the wordmark, the
// subtitle and one header per section.
func creditHeaders() []string {
	out := []string{"ENTIEVE", "CREDITS"}
	for _, s := range creditSections {
		out = append(out, strings.ToUpper(s.Title))
	}
	return out
}
