package screens

import (
	"fmt"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// logCaptureFrames is how many full draws run before the screenshot is
// taken (layout and the deferred scroll events settle in the first ones).
const logCaptureFrames = 4

// TestLogWindowCapture renders the log window through a real ebiten game
// loop — pixel readback (Image.At) panics before the game starts, so the
// rect pins in logconform_test.go cannot do this — and asserts at the
// pixel level that the log conforms to its frame with 200 lines in it:
//   - zero non-transparent pixels OUTSIDE the window rect (the symptom:
//     appended lines drawing past the frame after any relayout);
//   - glyph pixels INSIDE the body (so the zero-leak check is not vacuous);
//   - glyph pixels on the newest line's band at the bottom of the text
//     area (the tail: the last pushed line is in view).
//
// The scenario walks the exact paths that used to size the content from
// its preferred height: 200 pushes, a full relayout (visibility toggle),
// then a resize to 400×300.
//
// Skipped unless ENTIEVE_CAPTURE=1 so the regular suite stays windowless.
// The PNG lands in $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
func TestLogWindowCapture(t *testing.T) {
	if os.Getenv("ENTIEVE_CAPTURE") == "" {
		t.Skip("set ENTIEVE_CAPTURE=1 to render a capture frame")
	}
	uitest.SkipIfLoopRan(t)
	outDir := os.Getenv("ENTIEVE_CAPTURE_DIR")
	if outDir == "" {
		outDir = "/tmp/opencode"
	}

	env, _ := newChromeEnv(t)
	l := NewLog(env)
	c := newChromeUI(env)
	c.AddWindow(l)
	for i := 0; i < 200; i++ {
		l.PushLine(sim.SevInfo, "capture line "+strconv.Itoa(i))
	}

	g := &logCaptureGame{c: c, l: l, env: env, out: filepath.Join(outDir, "log_capture.png")}
	uitest.RunCaptureLoop(t, "Entieve log capture", g)
	if g.err != nil {
		t.Fatal(g.err)
	}
	if !g.saved {
		t.Fatal("capture never rendered")
	}
	t.Logf("capture written to %s", g.out)
}

// logCaptureGame drives the log for a few frames — including the
// relayout (visibility toggle) and the resize that used to leak the
// preferred height into the frame — then screenshots and verifies.
type logCaptureGame struct {
	c      *chromeUI
	l      *LogPanel
	env    widgets.Env
	out    string
	frames int
	last   *ebiten.Image
	saved  bool
	err    error
}

func (g *logCaptureGame) Update() error {
	if g.saved || g.err != nil {
		return ebiten.Termination
	}
	// The full-relayout trigger: kit.go does this when windows hide/show.
	if g.frames == 1 {
		g.l.Container().GetWidget().SetVisibility(widget.Visibility_Hide)
	}
	if g.frames == 2 {
		g.l.Container().GetWidget().SetVisibility(widget.Visibility_Show)
	}
	if g.frames == 3 {
		g.l.Win().SetLocation(image.Rect(100, 100, 500, 400))
		for i := 0; i < 40; i++ {
			g.l.PushLine(sim.SevInfo, "post-resize capture "+strconv.Itoa(i))
		}
	}
	for _, p := range g.c.panels {
		p.Sync()
	}
	g.c.ui.Update()
	g.frames++
	if g.frames > logCaptureFrames && g.last != nil {
		g.verify(g.last)
		return ebiten.Termination
	}
	return nil
}

func (g *logCaptureGame) Draw(screen *ebiten.Image) {
	img := ebiten.NewImage(1280, 720)
	g.c.ui.Draw(img)
	screen.DrawImage(img, nil)
	g.last = img
}

func (g *logCaptureGame) Layout(w, h int) (int, int) { return 1280, 720 }

// verify asserts the pixel-level conformance and saves the frame.
func (g *logCaptureGame) verify(img *ebiten.Image) {
	var errs []string
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Sprintf(format, args...))
	}

	// Screenshot first: evidence is useful even when an assert failed.
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

	win := g.l.Win().GetContainer().GetWidget().Rect
	inner := logInnerRect(win)
	ta := g.l.ta.GetWidget().Rect

	// 1. Nothing drawn outside the window frame.
	leak := 0
	leakBox := image.Rectangle{}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if x >= win.Min.X && x < win.Max.X && y >= win.Min.Y && y < win.Max.Y {
				continue
			}
			if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
				px := image.Rect(x, y, x+1, y+1)
				if leak == 0 {
					leakBox = px
				} else {
					leakBox = leakBox.Union(px)
				}
				leak++
			}
		}
	}
	if leak != 0 {
		fail("window %v: %d pixels drawn outside the frame (leak box %v)", win, leak, leakBox)
	}

	// 2. Glyph pixels inside the body: the pushed lines are SevInfo, so
	// their glyphs carry the palette's info color (non-vacuous: the text
	// really drew inside the frame).
	iabs := func(v int) int {
		if v < 0 {
			return -v
		}
		return v
	}
	glyphs := func(r image.Rectangle) int {
		want := g.env.Pal().Info
		n := 0
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				rr, gg, bb, _ := img.At(x, y).RGBA()
				if iabs(int(rr>>8)-int(want.R)) <= 32 &&
					iabs(int(gg>>8)-int(want.G)) <= 32 &&
					iabs(int(bb>>8)-int(want.B)) <= 32 {
					n++
				}
			}
		}
		return n
	}
	bodyGlyphs := glyphs(inner.Inset(theme.ContentsPad))
	if bodyGlyphs == 0 {
		fail("inner rect %v: no glyph pixels — the log drew nothing", inner)
	}

	// 3. Tail: glyph pixels on the newest line's band at the bottom of
	// the text area (the text area's last 24 px, minus the 4 px scroll
	// padding; the vertical scrollbar column starts further right).
	band := image.Rect(ta.Min.X+4, ta.Max.Y-4-24, ta.Max.X-24, ta.Max.Y-4)
	if n := glyphs(band); n == 0 {
		fail("tail band %v: no glyph pixels — the newest line is not in view", band)
	}

	if len(errs) > 0 {
		g.err = fmt.Errorf("capture %s: %s", g.out, strings.Join(errs, "; "))
	}
}
