package screens

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/entieve/entieve/internal/capture"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// mfgCaptureFrames is how many draws settle before the sample (the window
// fades in over 12 frames).
const mfgCaptureFrames = 14

// TestManufacturingCaptureFrame draws the revamped manufacturing window
// through a real ebiten loop and pins the pixels the headless suite cannot
// read back: the three panes (browse tree, recipe list with its value-share
// strips, the selected recipe's inspector bill) and the job strip below.
//
// Skipped unless ENTIEVE_CAPTURE=1; one ebiten loop per process — run it on
// its own:
//
//	ENTIEVE_CAPTURE=1 go test ./uikit/screens/ -run TestManufacturingCaptureFrame
func TestManufacturingCaptureFrame(t *testing.T) {
	k, _ := newTestKit(t)
	k.SetCoreVisible(false) // the manufacturing window only
	f := NewManufacturing(k.Env())
	k.AddWindow(f.Win())
	// The landing width, grown to the height the whole bill needs (the
	// window is resizable; the capture is the design, not the floor).
	f.Win().SetLocation(image.Rect(theme.Px(140), theme.Px(16), theme.Px(140+mfgDefaultW), theme.Px(16+mfgDefaultH+124)))

	// Docked at the start system's industry station, with one running job for
	// the bottom strip and a partly stocked artillery bill (two minerals
	// short, so the strip shows both states and the gate names the block).
	const station = sim.StationID("cel-haven-rest")
	k.Sim.Docked.Set(k.Sim.Player(), sim.Docked{At: station})
	for _, m := range k.Sim.JobMaterials(station, "rec-ammo-emp-s", 1) {
		giveItem(k.Sim, m.Item, m.Qty)
	}
	if _, ok := k.Sim.StartJob(station, "rec-ammo-emp-s", 1, sim.OwnerPlayer); !ok {
		t.Fatal("fixture: the charge job was refused")
	}
	for _, n := range f.needs("rec-mod-artillery-l", 1) {
		switch n.Item {
		case "min-argon", "min-silex":
			// left empty: the two short minerals the gate must name
		default:
			giveItem(k.Sim, n.Item, n.Need)
		}
	}
	// Select the heaviest recipe mid-loop (the list needs one Sync to have
	// its rows): its bill is the inspector's five rows.
	var frames, row int
	row = -1
	out := capture.Out("manufacturing_capture.png")
	(&capture.Capture{
		Name:   "Entieve manufacturing capture",
		Frames: mfgCaptureFrames,
		Update: func(int) error {
			if frames == 2 && row < 0 {
				row = recipeRowFor(f, "mod-artillery-l")
				if row < 0 {
					return fmt.Errorf("fixture: no artillery recipe row in %+v", f.recipes.Rows)
				}
				clickRow(k.ui, f.recipes, row) // the selection gesture (inspector)
			}
			return k.Update()
		},
		Draw: func(_ int, img *ebiten.Image) error {
			img.Fill(color.White)
			f.Sync()
			k.Draw(img)
			frames++
			return nil
		},
		Verify: func(img *ebiten.Image) error {
			var errs []string
			fail := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }

			win := f.Win().GetContainer().GetWidget().Rect
			if win.Empty() || win.Max.X > 1280 || win.Max.Y > 720 {
				fail("manufacturing window %v does not sit in the 1280×720 frame", win)
			}
			// The three panes laid out: the tree's rows, the list's rich
			// rows and the inspector's material rows all have rects.
			if len(f.tree.Rows) < 4 {
				fail("browse tree shows %d rows, want the root branches", len(f.tree.Rows))
			}
			if len(f.recipes.Rows) < 5 {
				fail("recipe list shows %d rows, want the catalog", len(f.recipes.Rows))
			}
			if got := len(f.matsRows); got != 5 {
				fail("inspector material rows = %d, want the artillery bill's 5", got)
			}
			for i, r := range f.matsRows {
				if r.Root.GetWidget().Rect.Empty() {
					fail("inspector material row %d never laid out", i)
				}
			}
			if !f.hasSel || f.sel.recipe != "rec-mod-artillery-l" {
				fail("selection = %+v (hasSel=%v), want the artillery recipe", f.sel, f.hasSel)
			}
			// The row's one-glance verdict and the inspector's gate name the
			// same block (the two short minerals), and the job strip lists
			// the running job.
			if got := recipeVerdict(f.recipes.Rows[row]); got != "× short 2" {
				fail("artillery row verdict = %q, want × short 2", got)
			}
			if got := f.gateLbl.Label; !strings.Contains(got, "Vetril") || !strings.Contains(got, "Helion") {
				fail("inspector gate = %q, want the short minerals named", got)
			}
			if len(f.jobs.Rows) != 1 || f.jobs.Rows[0].Cells[4] != "Running" {
				fail("job strip rows = %+v, want the one running job", f.jobs.Rows)
			}

			// Ink where the new visual language draws: the first recipe
			// row's strip line and the inspector's per-material bars are not
			// bare panel fill. The thresholds sit well under the measured
			// counts, so the pin is content, not antialiasing.
			body := rgb(img.At(win.Min.X+theme.Px(theme.RadiusWindow)+6, win.Max.Y-6))
			first := f.recipes.RowWidget(0).GetWidget().Rect
			if first.Empty() || !first.In(win) {
				fail("first recipe row %v is not inside the window %v", first, win)
			}
			lower := image.Rect(first.Min.X, first.Min.Y+first.Dy()/2, first.Max.X, first.Max.Y)
			if n := captureInk(img, lower, body); n < 30 {
				fail("recipe row %v strip band carries %d ink pixels, want the value-share strip", lower, n)
			}
			bar := f.matsRows[0].Root.GetWidget().Rect
			if n := captureInk(img, bar, body); n < 30 {
				fail("inspector material row %v carries %d ink pixels, want its icon, name and bar", bar, n)
			}

			if len(errs) > 0 {
				return fmt.Errorf("capture %s: %s", out, strings.Join(errs, "; "))
			}
			return nil
		},
		Out: out,
	}).Run(t)
}

// captureInk counts the pixels of a rect that differ from the body colour by
// more than a hair (the window's own fill): text, art and bars all count.
func captureInk(img *ebiten.Image, r image.Rectangle, body color.RGBA) int {
	n := 0
	for y := r.Min.Y; y < r.Max.Y && y < img.Bounds().Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X && x < img.Bounds().Max.X; x++ {
			if x < 0 || y < 0 {
				continue
			}
			p := rgb(img.At(x, y))
			if channelDelta(p, body) > 12 {
				n++
			}
		}
	}
	return n
}
