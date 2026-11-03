package screens

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/internal/capture"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// splitCaptureFrames is how many draws settle before the sample: the
// open fade is 12 frames, so 13 is the first fully painted frame.
const splitCaptureFrames = 14

// TestSplitPaneSeamCapture draws the split windows side by side through a
// real ebiten loop and pins the seams' PIXELS, not only their rects: each
// seam is a recessed 4 px track (a Border line around the near-black fill)
// with a dotted sash grip in its middle, the grip's dots brighter than the
// track's fill, and the tracks sit at the split's proportions: inventory's
// two seams at its thirds, the manufacturing board's at the fraction of the
// stack before it (the board over its job strip). Two windows, because at
// their content floors (320 and 640 px wide) a third would have to overlap
// them and the pixel reads would sample the window on top; fitting's own
// window is covered by its capture. Pixel reads need a running loop, so
// this is gated like the other captures; the PNG lands in
// $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
func TestSplitPaneSeamCapture(t *testing.T) {
	k, _ := newTestKit(t)
	inv := NewInventory(k.Env())
	manu := NewManufacturing(k.Env())
	for _, c := range []struct {
		win *widget.Window
		at  image.Rectangle
	}{
		{inv.Win(), image.Rect(0, 8, 320, 428)},
		{manu.Win(), image.Rect(328, 8, 968, 488)},
	} {
		k.AddWindow(c.win)
		c.win.SetLocation(c.at)
	}
	// The core Selected Item window would cover the inventory's lower
	// panes: park it below, out of the sweep.
	k.SelectedWindow().SetLocation(image.Rect(20, 440, 470, 696))

	out := capture.Out("splitpanes_capture.png")
	(&capture.Capture{
		Name:   "Entieve split pane capture",
		Frames: splitCaptureFrames,
		Update: func(int) error {
			inv.Sync()
			manu.Sync()
			return k.Update()
		},
		Draw: func(_ int, img *ebiten.Image) error {
			img.Fill(color.White)
			k.Draw(img)
			return nil
		},
		Verify: func(img *ebiten.Image) error {
			var errs []string
			fail := func(format string, args ...any) {
				errs = append(errs, fmt.Sprintf(format, args...))
			}
			lum := func(pt image.Point) int {
				r, gg, b, _ := img.At(pt.X, pt.Y).RGBA()
				return int(r>>8) + int(gg>>8) + int(b>>8)
			}

			check := func(name string, split *widgets.SplitPane) []int {
				panes := splitPanes(split)
				var rows []int
				for i := 0; i < len(panes)-1; i++ {
					seam := splitSeam(split, i)
					if seam.Dy() != theme.Px(widgets.SplitSeamH) {
						fail("%s: seam %d is %d px, want the %d px track", name, i, seam.Dy(), theme.Px(widgets.SplitSeamH))
						continue
					}
					mid := image.Pt((seam.Min.X+seam.Max.X)/2, (seam.Min.Y+seam.Max.Y)/2)
					fill := lum(image.Pt(seam.Min.X+4, seam.Min.Y+1)) // track fill, clear of the grip
					if edge := lum(image.Pt(seam.Min.X+4, seam.Min.Y)); edge <= fill {
						fail("%s: seam %d's top edge (brightness %d) does not stand out of the track fill (%d)",
							name, i, edge, fill)
					}
					if grip := lum(mid); grip <= fill {
						fail("%s: seam %d's grip (brightness %d) does not stand out of the track fill (%d)",
							name, i, grip, fill)
					}
					rows = append(rows, seam.Min.Y)
				}
				return rows
			}

			// Inventory: two seams on the thirds of the pane stack.
			if rows := check("inventory", inv.split); len(rows) == 2 {
				panes := splitPanes(inv.split)
				for i, row := range rows {
					want := panes[i].Max.Y
					if d := row - want; d < -2 || d > 2 {
						fail("inventory: seam %d painted at y=%d, want the pane boundary ≈%d", i, row, want)
					}
				}
			} else {
				fail("inventory: %d seams found, want 2", len(rows))
			}
			// Manufacturing: one seam, at the panes' boundary.
			for _, c := range []struct {
				name  string
				split *widgets.SplitPane
			}{{"manufacturing", manu.split}} {
				rows := check(c.name, c.split)
				panes := splitPanes(c.split)
				if len(rows) != 1 || len(panes) != 2 {
					fail("%s: %d seams / %d panes, want 1 / 2", c.name, len(rows), len(panes))
					continue
				}
				if want := panes[0].Max.Y; rows[0] < want-2 || rows[0] > want+2 {
					fail("%s: seam painted at y=%d, want the pane boundary ≈%d", c.name, rows[0], want)
				}
			}

			if len(errs) > 0 {
				return fmt.Errorf("capture %s: %s", out, strings.Join(errs, "; "))
			}
			return nil
		},
		Out: out,
	}).Run(t)
}
