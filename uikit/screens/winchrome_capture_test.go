package screens

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// winChromeCaptureFrames is how many draws run before the screenshot:
// the open fade is 12 frames, so 13 lands on the first fully settled
// frame — captures always pin the settled state, never a mid-fade one.
const winChromeCaptureFrames = 13

// TestWindowFadeAndTranslucencyCapture renders five kit windows (the
// two core ones, Show Info, the log and the market) over a white
// backdrop through a real ebiten game loop and asserts the render-only
// animation's visual endpoints plus the translucency token, uniformly:
//
//   - draw 1 is open frame 0: the just-mounted windows reach NO pixels
//     (the sample sits on the raw backdrop);
//   - at settle every window's body background reads exactly
//     theme.WindowBody(BG) composited over the backdrop — one alpha
//     across all windows (the uniformity half of the translucency);
//   - the × drew its glyph inside the chrome's exclusion strip (the
//     close affordance's pixels).
//
// Skipped unless ENTIEVE_CAPTURE=1 so the regular suite stays
// windowless (pixel reads need a running game loop). The PNG lands in
// $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
func TestWindowFadeAndTranslucencyCapture(t *testing.T) {
	if os.Getenv("ENTIEVE_CAPTURE") == "" {
		t.Skip("set ENTIEVE_CAPTURE=1 to render a capture frame")
	}
	uitest.SkipIfLoopRan(t)
	outDir := os.Getenv("ENTIEVE_CAPTURE_DIR")
	if outDir == "" {
		outDir = "/tmp/opencode"
	}

	k, _ := newTestKit(t)
	// Five windows parked DISJOINT (1280×720), so every body sample
	// belongs to the window it names — no other window paints over it.
	k.OverviewWindow().SetLocation(image.Rect(64, 8, 444, 428))
	k.SelectedWindow().SetLocation(image.Rect(64, 436, 514, 696))
	k.ShowInfo(widgets.ItemRef("charge-emp"))
	k.InfoWindow().SetLocation(image.Rect(460, 8, 880, 468))
	log := NewLog(k.Env())
	k.AddWindow(log.Win())
	log.Win().SetLocation(image.Rect(900, 8, 1188, 248))
	market := NewMarket(k.Env())
	k.AddWindow(market.Win())
	market.Win().SetLocation(image.Rect(900, 260, 1200, 500))

	g := &winChromeCaptureGame{
		k:   k,
		out: filepath.Join(outDir, "winchrome_capture.png"),
		wins: []captureWin{
			{"overview", k.OverviewWindow()},
			{"selected", k.SelectedWindow()},
			{"info", k.InfoWindow()},
			{"log", log.Win()},
			{"market", market.Win()},
		},
	}
	uitest.RunCaptureLoop(t, "Entieve window chrome capture", g)
	if g.err != nil {
		t.Fatal(g.err)
	}
	if !g.saved {
		t.Fatal("capture never rendered")
	}
	t.Logf("capture written to %s", g.out)
}

// captureWin is one window the uniformity sweep samples.
type captureWin struct {
	name string
	win  *widget.Window
}

// winChromeCaptureGame drives the kit, keeps the first draw (open
// frame 0) and verifies the settled frame once the fade has run out.
type winChromeCaptureGame struct {
	k      *Kit
	out    string
	wins   []captureWin
	frames int
	start  *ebiten.Image // the first draw (open frame 0)
	last   *ebiten.Image
	saved  bool
	err    error
}

func (g *winChromeCaptureGame) Update() error {
	if g.saved || g.err != nil {
		return ebiten.Termination
	}
	g.k.Sync()
	if err := g.k.Update(); err != nil {
		g.err = err
		return ebiten.Termination
	}
	if g.frames > winChromeCaptureFrames && g.last != nil {
		g.err = g.verify(g.last)
		return ebiten.Termination
	}
	return nil
}

func (g *winChromeCaptureGame) Draw(screen *ebiten.Image) {
	img := ebiten.NewImage(1280, 720)
	img.Fill(color.White) // the backdrop every body blend is measured against
	g.k.Draw(img)
	if g.frames == 0 {
		g.start = img // draw 1 = open frame 0 of every freshly mounted window
	}
	g.last = img
	g.frames++ // frames count DRAWS: ebiten runs Updates on their own cadence (see rail_capture_test.go)
	screen.DrawImage(img, nil)
}

func (g *winChromeCaptureGame) Layout(w, h int) (int, int) { return 1280, 720 }

// verify asserts the settled frame and screenshots it. Every failure is
// collected so one run reports all of them. Screenshot first: evidence
// is useful even when an assert failed.
func (g *winChromeCaptureGame) verify(img *ebiten.Image) error {
	var errs []string
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Sprintf(format, args...))
	}
	sample := func(pt image.Point) (int, int, int) {
		r, gg, b, _ := img.At(pt.X, pt.Y).RGBA()
		return int(r >> 8), int(gg >> 8), int(b >> 8)
	}

	f, err := os.Create(g.out)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return err
	}
	g.saved = true

	// Endpoint 1 — open frame 0 reaches no pixels: every freshly
	// mounted window is still transparent, so the backdrop shows
	// through untouched.
	if g.start == nil {
		fail("the first draw was never captured")
	} else {
		pt := bodySample(g.k.OverviewWindow().GetContainer().GetWidget().Rect)
		if r, gg, b := sampleAt(g.start, pt); r != 255 || gg != 255 || b != 255 {
			fail("open frame 0 painted (%d,%d,%d) at %v, want the raw backdrop (255,255,255)", r, gg, b, pt)
		}
	}

	// Endpoint 2 (uniformity) — at settle every body reads the ONE
	// translucency token over the same backdrop.
	want := theme.WindowBody(g.k.th.Pal().BG)
	// out = src·a + dst·(1−a) in 8-bit over the opaque white backdrop.
	blend := func(c uint8) int {
		return (int(c)*int(want.A) + 255*(255-int(want.A)) + 127) / 255
	}
	wantR, wantG, wantB := blend(want.R), blend(want.G), blend(want.B)
	for _, w := range g.wins {
		if w.win == nil {
			fail("%s window missing", w.name)
			continue
		}
		pt := bodySample(w.win.GetContainer().GetWidget().Rect)
		r, gg, b := sample(pt)
		if abs(r-wantR) > 2 || abs(gg-wantG) > 2 || abs(b-wantB) > 2 {
			fail("%s body at %v = (%d,%d,%d), want the WindowBody blend (%d,%d,%d) — every window must share the one translucency token",
				w.name, pt, r, gg, b, wantR, wantG, wantB)
		}
	}

	// The × glyph drew inside the overview's exclusion strip (its own
	// text ink over the near-black bar).
	strip := closeButtonRect(g.k.OverviewWindow().TitleBar.GetWidget().Rect)
	if n := uitest.InkIn(img, strip); n < 8 {
		fail("× strip %v drew %d ink pixels, want the glyph (≥ 8)", strip, n)
	}

	if len(errs) > 0 {
		return fmt.Errorf("capture %s: %s", g.out, strings.Join(errs, "; "))
	}
	return nil
}

// sampleAt reads a pixel from a specific frame image.
func sampleAt(img *ebiten.Image, pt image.Point) (int, int, int) {
	r, g, b, _ := img.At(pt.X, pt.Y).RGBA()
	return int(r >> 8), int(g >> 8), int(b >> 8)
}
