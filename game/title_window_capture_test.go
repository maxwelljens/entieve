package game

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
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
	if os.Getenv("ENTIEVE_CAPTURE") == "" {
		t.Skip("set ENTIEVE_CAPTURE=1 to render a capture frame")
	}
	outDir := os.Getenv("ENTIEVE_CAPTURE_DIR")
	if outDir == "" {
		outDir = "/tmp/opencode"
	}
	g := newTitleGameAt(t, filepath.Join(t.TempDir(), "save.json"))
	g.applySize(1280, 720)

	cg := &titleWindowCapture{t: t, g: g, which: which, out: filepath.Join(outDir, name)}
	ebiten.SetWindowTitle("Entieve " + which + " capture")
	ebiten.SetWindowSize(1280, 720)
	if err := ebiten.RunGame(cg); err != nil {
		t.Skipf("cannot run capture game loop: %v", err)
	}
	if cg.err != nil {
		t.Fatal(cg.err)
	}
	if !cg.saved {
		t.Fatal("capture never rendered")
	}
	t.Logf("%s capture %s (%d window pixels, %d headers)", which, cg.out, cg.winPx, cg.headers)
}

type titleWindowCapture struct {
	t     *testing.T
	g     *Game
	which string
	out   string

	frames     int
	draws      int
	opened     bool
	openedDraw int
	saved      bool
	err        error

	winPx   int
	headers int
}

func (cg *titleWindowCapture) Update() error {
	if cg.saved || cg.err != nil {
		return ebiten.Termination
	}
	cg.frames++
	if !cg.opened && cg.frames == 4 {
		cg.g.titleUI.open(cg.which)
		cg.opened, cg.openedDraw = true, cg.draws // the fade counts DRAWN frames
	}
	return cg.g.Update()
}

func (cg *titleWindowCapture) Draw(screen *ebiten.Image) {
	img := ebiten.NewImage(1280, 720)
	cg.g.Draw(img)
	screen.DrawImage(img, nil)
	// The fade is 12 DRAWN frames (kit openFrames) and Draw is not called
	// on every Update: wait 20 draws after the open, so the window is
	// always settled before the capture and the bytes stay deterministic.
	cg.draws++
	if cg.saved || cg.err != nil || !cg.opened || cg.draws < cg.openedDraw+20 {
		return
	}
	cg.capture(img)
}

// capture pins the window: it paints over the title backdrop frame, its
// tracked headers are there, and (keybindings) its sections are the three
// the action tables derive.
func (cg *titleWindowCapture) capture(img *ebiten.Image) {
	g := cg.g
	var win *widget.Window
	var page widget.PreferredSizeLocateableWidget
	switch cg.which {
	case "keybindings":
		win = g.titleUI.host.Window(idKeybindings)
		if kb := bodyOf[keybindPanel](g.titleUI.host, idKeybindings); kb.Keybindings != nil {
			page = widgets.ScrollContentOf(kb.ScrollForTest())
		}

	case "credits":
		win = g.titleUI.credits
		page = widgets.ScrollContentOf(g.titleUI.creditScroll)
	}
	if win == nil || page == nil {
		cg.err = fmt.Errorf("%s window did not mount (title=%v open=%v mounted=%v)",
			cg.which, g.title, g.titleUI.host.IsOpen(ScreenID(cg.which)),
			g.titleUI.host.Mounted(ScreenID(cg.which)))
		return
	}
	r := win.GetContainer().GetWidget().Rect
	if r.Dx() <= 0 || r.Dy() <= 0 || r.Max.X > 1280 || r.Max.Y > 720 {
		cg.err = fmt.Errorf("%s window rect = %v, want an on-screen size", cg.which, r)
		return
	}
	base := ebiten.NewImage(1280, 720)
	g.drawTitleBackdrop(base)
	for y := r.Min.Y; y < r.Max.Y; y += 2 {
		for x := r.Min.X; x < r.Max.X; x += 2 {
			if img.At(x, y) != base.At(x, y) {
				cg.winPx++
			}
		}
	}
	if cg.winPx < 500 {
		cg.err = fmt.Errorf("%s window not painted: %d changed pixels (rect %v)", cg.which, cg.winPx, r)
		cg.write(img)
		return
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
	want := keybindHeaders(g)
	if cg.which == "credits" {
		want = creditHeaders()
	}
	cg.headers = len(got)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		cg.err = fmt.Errorf("%s headers = %v, want %v", cg.which, got, want)
		cg.write(img)
		return
	}
	cg.saved = true
	cg.write(img)
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

func (cg *titleWindowCapture) write(img *ebiten.Image) {
	f, err := os.Create(cg.out)
	if err != nil {
		cg.err = err
		return
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		cg.err = err
		return
	}
	if err := f.Close(); err != nil {
		cg.err = err
	}
}

func (cg *titleWindowCapture) Layout(w, h int) (int, int) { return 1280, 720 }
