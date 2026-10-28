package screens

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/internal/capture"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// winChromeCaptureFrames is how many draws run before the screenshot:
// the open fade is 12 frames, so 13 lands on the first fully settled
// frame — captures always pin the settled state, never a mid-fade one.
const winChromeCaptureFrames = 14

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
	k, _ := newTestKit(t)
	// Five windows parked DISJOINT (1280×720), so every body sample
	// belongs to the window it names — no other window paints over it.
	k.OverviewWindow().SetLocation(image.Rect(64, 8, 444, 428))
	k.SelectedWindow().SetLocation(image.Rect(64, 436, 514, 696))
	k.ShowInfo(widgets.ItemRef("ammo-emp-s"))
	k.InfoWindow().SetLocation(image.Rect(460, 8, 880, 468))
	log := NewLog(k.Env())
	k.AddWindow(log.Win())
	log.Win().SetLocation(image.Rect(900, 8, 1188, 248))
	market := NewMarket(k.Env())
	k.AddWindow(market.Win())
	market.Win().SetLocation(image.Rect(900, 260, 1200, 500))

	wins := []captureWin{
		{"overview", k.OverviewWindow()},
		{"selected", k.SelectedWindow()},
		{"info", k.InfoWindow()},
		{"log", log.Win()},
		{"market", market.Win()},
	}
	var start *ebiten.Image // the first draw (open frame 0)
	out := capture.Out("winchrome_capture.png")
	(&capture.Capture{
		Name:   "Entieve window chrome capture",
		Frames: winChromeCaptureFrames,
		Update: func(int) error {
			k.Sync()
			return k.Update()
		},
		Draw: func(n int, img *ebiten.Image) error {
			img.Fill(color.White) // the backdrop every body blend is measured against
			k.Draw(img)
			if n == 1 {
				start = img // draw 1 = open frame 0 of every freshly mounted window
			}
			return nil
		},
		// verify asserts the settled frame. Every failure is collected so one
		// run reports all of them.
		Verify: func(img *ebiten.Image) error {
			var errs []string
			fail := func(format string, args ...any) {
				errs = append(errs, fmt.Sprintf(format, args...))
			}
			sample := func(pt image.Point) (int, int, int) {
				r, gg, b, _ := img.At(pt.X, pt.Y).RGBA()
				return int(r >> 8), int(gg >> 8), int(b >> 8)
			}

			// Endpoint 1 — open frame 0 reaches no pixels: every freshly
			// mounted window is still transparent, so the backdrop shows
			// through untouched.
			if start == nil {
				fail("the first draw was never captured")
			} else {
				pt := bodySample(k.OverviewWindow().GetContainer().GetWidget().Rect)
				if r, gg, b := sampleAt(start, pt); r != 255 || gg != 255 || b != 255 {
					fail("open frame 0 painted (%d,%d,%d) at %v, want the raw backdrop (255,255,255)", r, gg, b, pt)
				}
			}

			// Endpoint 2 (uniformity) — at settle every body reads the ONE
			// translucency token over the same backdrop. The body's top rows are
			// the panel-depth ramp's top end (PanelTop), by construction: the
			// sample sits two px under the title seam.
			pal := k.th.Pal()
			want := theme.PanelTop(pal, theme.WindowBody(pal.BG))
			// out = src·a + dst·(1−a) in 8-bit over the opaque white backdrop.
			blend := func(c uint8) int {
				return (int(c)*int(want.A) + 255*(255-int(want.A)) + 127) / 255
			}
			wantR, wantG, wantB := blend(want.R), blend(want.G), blend(want.B)
			for _, w := range wins {
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

			// Rounded chrome: every window's bottom-left corner is cut away (the
			// backdrop shows through) while the same row ten px in is the frame.
			// Title bars are top-rounded only, bodies bottom-rounded only.
			// A window whose corner another window covers is skipped for the
			// corner probes (the info window overlaps the selected item's title
			// bar here); the body-uniformity arm above still covers every window.
			covered := func(pt image.Point, self *widget.Window) bool {
				for _, o := range wins {
					if o.win == nil || o.win == self {
						continue
					}
					if pt.In(o.win.GetContainer().GetWidget().Rect) {
						return true
					}
				}
				return false
			}
			for _, w := range wins {
				if w.win == nil {
					continue
				}
				r := w.win.GetContainer().GetWidget().Rect
				corner := image.Pt(r.Min.X, r.Max.Y-1)
				if covered(corner, w.win) {
					continue
				}
				if cr, cg, cb := sample(corner); cr != 255 || cg != 255 || cb != 255 {
					fail("%s bottom-left corner %v = (%d,%d,%d), want the rounded-away backdrop (255,255,255)",
						w.name, corner, cr, cg, cb)
				}
				edge := image.Pt(r.Min.X+theme.Px(theme.RadiusWindow)+2, r.Max.Y-1)
				if cr, cg, cb := sample(edge); cr == 255 && cg == 255 && cb == 255 {
					fail("%s bottom edge %v reads the backdrop: the body did not fill its frame", w.name, edge)
				}
				top := image.Pt(r.Min.X, r.Min.Y)
				if cr, cg, cb := sample(top); cr != 255 || cg != 255 || cb != 255 {
					fail("%s top-left corner %v = (%d,%d,%d), want the title bar's rounded-away corner",
						w.name, top, cr, cg, cb)
				}
			}

			// Box contract — the generated shape's bounding box IS the piece's
			// rect: the top and bottom bands carry the piece's fill edge to edge
			// (only the corner cutouts are transparent), so no backdrop band shows
			// where the arcs begin. The title bar's band is the flat BGHard fill,
			// the body's bottom band the depth ramp's bottom end (PanelBottom).
			blendOver := func(c color.NRGBA) (int, int, int) {
				f := func(v uint8) int { return (int(v)*int(c.A) + 255*(255-int(c.A)) + 127) / 255 }
				return f(c.R), f(c.G), f(c.B)
			}
			titleWant := theme.Solid(pal.BGHard)
			bodyWant := theme.PanelBottom(pal, theme.WindowBody(pal.BG))
			for _, w := range wins {
				if w.win == nil {
					continue
				}
				tb := w.win.TitleBar.GetWidget().Rect
				tx := tb.Min.X + tb.Dx()/2
				// Rows 1 and RadiusWindow/2 sit inside the top slice (the arcs'
				// band): the fill runs right up to the hairline, no gap above it.
				for _, pt := range []image.Point{
					{X: tx, Y: tb.Min.Y + theme.Px(1)},
					{X: tx, Y: tb.Min.Y + theme.Px(theme.RadiusWindow)/2},
				} {
					r, gg, b := sample(pt)
					if abs(r-int(titleWant.R)) > 1 || abs(gg-int(titleWant.G)) > 1 || abs(b-int(titleWant.B)) > 1 {
						fail("%s title top band %v = (%d,%d,%d), want the title-bar fill (%d,%d,%d) — the box fills its rect",
							w.name, pt, r, gg, b, titleWant.R, titleWant.G, titleWant.B)
					}
				}
				// The rect's very first row is the frame hairline, never the backdrop.
				if r, gg, b := sample(image.Pt(tx, tb.Min.Y)); r == 255 && gg == 255 && b == 255 {
					fail("%s title top row %v reads the backdrop: the box does not reach its rect's top edge", w.name, image.Pt(tx, tb.Min.Y))
				}
				r := w.win.GetContainer().GetWidget().Rect
				bx := r.Min.X + r.Dx()/2
				wantR, wantG, wantB := blendOver(bodyWant)
				for _, dy := range []int{1, 2} {
					pt := image.Pt(bx, r.Max.Y-1-dy)
					cr, cg, cb := sample(pt)
					if abs(cr-wantR) > 2 || abs(cg-wantG) > 2 || abs(cb-wantB) > 2 {
						fail("%s body bottom band %v = (%d,%d,%d), want the ramp's bottom end (%d,%d,%d)",
							w.name, pt, cr, cg, cb, wantR, wantG, wantB)
					}
				}
			}

			// The × glyph drew inside the overview's exclusion strip (its own
			// text ink over the near-black bar).
			strip := closeButtonRect(k.OverviewWindow().TitleBar.GetWidget().Rect)
			if n := uitest.InkIn(img, strip); n < 8 {
				fail("× strip %v drew %d ink pixels, want the glyph (≥ 8)", strip, n)
			}

			if len(errs) > 0 {
				return fmt.Errorf("capture %s: %s", out, strings.Join(errs, "; "))
			}
			return nil
		},
		Out: out,
	}).Run(t)
}

// captureWin is one window the uniformity sweep samples.
type captureWin struct {
	name string
	win  *widget.Window
}

// sampleAt reads a pixel from a specific frame image.
func sampleAt(img *ebiten.Image, pt image.Point) (int, int, int) {
	r, g, b, _ := img.At(pt.X, pt.Y).RGBA()
	return int(r >> 8), int(g >> 8), int(b >> 8)
}
