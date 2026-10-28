package screens

import (
	"fmt"
	"image"
	"strconv"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/internal/capture"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// logCaptureFrames is how many Update ticks the scenario runs before the
// screenshot (layout and the deferred scroll events settle in the first
// ones).
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
	env, _ := newChromeEnv(t)
	l := NewLog(env)
	c := newChromeUI(env)
	c.AddWindow(l)
	for i := 0; i < 200; i++ {
		l.PushLine(sim.SevInfo, "capture line "+strconv.Itoa(i))
	}

	out := capture.Out("log_capture.png")
	var cap *capture.Capture
	cap = &capture.Capture{
		Name: "Entieve log capture",
		// The scenario counts UPDATE ticks (the fades settle on draws), so it
		// ends the loop itself and the driver saves the frame last drawn.
		Update: func(n int) error {
			// The full-relayout trigger: kit.go does this when windows hide/show.
			if n == 2 {
				l.Container().GetWidget().SetVisibility(widget.Visibility_Hide)
			}
			if n == 3 {
				l.Container().GetWidget().SetVisibility(widget.Visibility_Show)
			}
			if n == 4 {
				l.Win().SetLocation(image.Rect(100, 100, 500, 400))
				for i := 0; i < 40; i++ {
					l.PushLine(sim.SevInfo, "post-resize capture "+strconv.Itoa(i))
				}
			}
			for _, p := range c.panels {
				p.Sync()
			}
			c.ui.Update()
			if n > logCaptureFrames {
				cap.Stop()
			}
			return nil
		},
		Draw: func(_ int, img *ebiten.Image) error {
			c.ui.Draw(img)
			return nil
		},
		Verify: func(img *ebiten.Image) error {
			var errs []string
			fail := func(format string, args ...any) {
				errs = append(errs, fmt.Sprintf(format, args...))
			}

			win := l.Win().GetContainer().GetWidget().Rect
			inner := logInnerRect(win)
			ta := l.ta.GetWidget().Rect

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
				want := env.Pal().Info
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
				return fmt.Errorf("capture %s: %s", out, strings.Join(errs, "; "))
			}
			return nil
		},
		Out: out,
	}
	cap.Run(t)
}
