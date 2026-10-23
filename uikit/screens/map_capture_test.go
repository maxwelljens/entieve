package screens

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestMapCaptureFrame renders the starmap through a real ebiten game loop
// (pixel readback panics before the game starts) and asserts the Cyber
// Terminal grid: 28 px mint lines at ~3.5 % alpha under the gate graph, on
// the map image only.
//
// Skipped unless ENTIEVE_CAPTURE=1 so the regular suite stays windowless.
// The PNG lands in $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
func TestMapCaptureFrame(t *testing.T) {
	if os.Getenv("ENTIEVE_CAPTURE") == "" {
		t.Skip("set ENTIEVE_CAPTURE=1 to render a capture frame")
	}
	uitest.SkipIfLoopRan(t)
	outDir := os.Getenv("ENTIEVE_CAPTURE_DIR")
	if outDir == "" {
		outDir = "/tmp/opencode"
	}

	env, _ := newChromeEnv(t)
	m := NewMapView(env)
	c := newChromeUI(env)
	c.AddWindow(m)

	g := &mapCaptureGame{c: c, m: m, out: filepath.Join(outDir, "map_capture.png")}
	uitest.RunCaptureLoop(t, "Entieve starmap capture", g)
	if g.err != nil {
		t.Fatal(g.err)
	}
	if !g.saved {
		t.Fatal("capture never rendered")
	}
	t.Logf("capture written to %s", g.out)
}

type mapCaptureGame struct {
	c      *chromeUI
	m      *MapView
	out    string
	frames int
	last   *ebiten.Image
	saved  bool
	err    error
}

func (g *mapCaptureGame) Update() error {
	if g.saved || g.err != nil {
		return ebiten.Termination
	}
	for _, p := range g.c.panels {
		p.Sync()
	}
	g.c.ui.Update()
	g.frames++
	if g.frames > 3 && g.last != nil {
		g.verify(g.last)
		return ebiten.Termination
	}
	return nil
}

func (g *mapCaptureGame) Draw(screen *ebiten.Image) {
	img := ebiten.NewImage(1280, 720)
	g.c.ui.Draw(img)
	screen.DrawImage(img, nil)
	g.last = img
}

func (g *mapCaptureGame) Layout(w, h int) (int, int) { return 1280, 720 }

func (g *mapCaptureGame) verify(img *ebiten.Image) {
	var errs []string
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Sprintf(format, args...))
	}
	f, err := os.Create(g.out)
	if err != nil {
		g.err = err
		return
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		g.err = err
		return
	}
	g.saved = true

	// The grid: a scan line near the map image's top crosses the vertical
	// 28 px lines. Each line is the accent at ~3.5 % (alpha 9); anything
	// else on the line (gate edges, node rings) is a different alpha.
	step := float64(theme.Px(28)) * theme.Scale()
	mapImg := g.m.img
	grid := 0
	for x := 0; x < mapImg.Bounds().Dx(); x++ {
		_, _, _, a := mapImg.At(x, 1).RGBA()
		if a > 0 && a < 0x2000 { // 9/255 ≈ 0x0909 in 16-bit
			grid++
		}
	}
	want := int(float64(mapImg.Bounds().Dx()) / step)
	if grid < want-2 {
		fail("map grid: %d lit line samples of %d, want the 28 px lattice", grid, want)
	}

	if len(errs) > 0 {
		g.err = fmt.Errorf("capture %s: %s", g.out, strings.Join(errs, "; "))
	}
}
