package screens

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/internal/capture"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// rfCaptureFrames is how many draws settle before the sample (the window
// fades in over 12 frames).
const rfCaptureFrames = 14

// TestRefineryCaptureFrame draws the revamped refinery window through a real
// ebiten loop and pins the pixels the headless suite cannot read back: the
// three panes (browse tree, ore list with its verdict glyphs and value strips,
// the selected ore's inspector) with the head, the money table and the yield
// bill laid out.
//
// Skipped unless ENTIEVE_CAPTURE=1; one ebiten loop per process — run it on
// its own:
//
//	ENTIEVE_CAPTURE=1 go test ./uikit/screens/ -run TestRefineryCaptureFrame
func TestRefineryCaptureFrame(t *testing.T) {
	k, _ := newTestKit(t)
	k.SetCoreVisible(false) // the refinery window only
	f := NewRefinery(k.Env())
	k.AddWindow(f.Win())
	// The landing width, grown to the height the whole inspector page needs
	// (the window is resizable; the capture is the design, not the floor).
	f.Win().SetLocation(image.Rect(theme.Px(120), theme.Px(16), theme.Px(120+rfDefaultW), theme.Px(16+rfDefaultH+140)))

	// Docked at the start system's refine station (it runs a market too, so
	// the money table prices both sides off its own book), with three ores
	// held: the list shows a one, a two and a three-segment strip, and the
	// inspector the selected ore's two-mineral batch.
	const station = sim.StationID("cel-haven-rest")
	k.Sim.Docked.Set(k.Sim.Player(), sim.Docked{At: station})
	for _, ore := range []content.ItemID{"ore-cordite", "ore-lucidite", "ore-sterngrit"} {
		giveItem(k.Sim, ore, 100)
	}
	// Select Kerngrit mid-loop (the list needs one Sync to have its rows):
	// its bill is the inspector's two rows.
	var frames, row int
	row = -1
	out := capture.Out("refinery_capture.png")
	(&capture.Capture{
		Name:   "Entieve refinery capture",
		Frames: rfCaptureFrames,
		Update: func(int) error {
			if frames == 2 && row < 0 {
				row = oreRowFor(f, "ore-cordite")
				if row < 0 {
					return fmt.Errorf("fixture: no cordite row in %+v", f.ores.Rows)
				}
				clickRow(k.ui, f.ores, row) // the selection gesture (inspector)
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
				fail("refinery window %v does not sit in the 1280×720 frame", win)
			}
			// The three panes laid out: the tree's branches, the list's rich
			// rows and the inspector's bill rows all have rects.
			if len(f.tree.Rows) < 2 {
				fail("browse tree shows %d rows, want the root branch", len(f.tree.Rows))
			}
			if len(oreNames(f)) < 3 {
				fail("ore list shows %v, want the three held ores", oreNames(f))
			}
			if got := len(f.billRows); got != 2 {
				fail("inspector bill rows = %d, want the cordite batch's 2", got)
			}
			view := f.billScroll.ViewRect()
			for i, r := range f.billRows {
				if r.Root.GetWidget().Rect.Empty() {
					fail("bill row %d never laid out", i)
				} else if !r.Root.GetWidget().Rect.In(view) {
					fail("bill row %d %v falls outside the %v page viewport", i, r.Root.GetWidget().Rect, view)
				}
			}
			if !f.hasSel || f.sel.ore != "ore-cordite" {
				fail("selection = %+v (hasSel=%v), want cordite", f.sel, f.hasSel)
			}
			// The row's one-glance verdict and the inspector's glyph agree
			// (the ore refines here), and the money table priced both sides
			// off the local book: 100 ore at 41 €, its minerals at 535 €.
			if got := oreVerdictText(oreRowOf(t, f, "ore-cordite")); got != "✓" {
				fail("lucidite row verdict = %q, want the tick", got)
			}
			if got := rfVerdict(f); got != "✓" {
				fail("inspector verdict = %q, want the tick", got)
			}
			if f.refineBtn.GetWidget().Disabled {
				fail("Refine grayed at a refine station with a whole batch")
			}
			for _, c := range []struct{ label, want string }{
				{"Ore value", "4,100 €"},
				{"Minerals value", "535 €"},
				{"Refine delta", "−3,565 €"},
				{"Margin", "-666%"},
			} {
				if got := mathValue(f, c.label); got != c.want {
					fail("math row %q = %q, want %q", c.label, got, c.want)
				}
			}
			if got := f.head.headIcon.Widget().Image.Bounds().Dx(); got != theme.Px(theme.IconHero) {
				fail("inspector head icon = %d px, want the %d px hero", got, theme.Px(theme.IconHero))
			}
			if f.head.headProse.Label == "" || f.head.headProse.GetWidget().Rect.Empty() {
				fail("inspector description missing at %v", f.head.headProse.GetWidget().Rect)
			}
			if got := f.billRows[0].Icon.Widget().GetWidget().Rect.Dx(); got != theme.Px(mfgMatIcon) {
				fail("bill icon = %d px, want the %d px box", got, theme.Px(mfgMatIcon))
			}

			// Ink where the new visual language draws: the first ore row's
			// strip line and the money table's first row are not bare panel
			// fill. The thresholds sit well under the measured counts, so the
			// pin is content, not antialiasing.
			body := rgb(img.At(win.Min.X+theme.Px(theme.RadiusWindow)+6, win.Max.Y-6))
			first := f.ores.RowWidget(0).GetWidget().Rect
			if first.Empty() || !first.In(win) {
				fail("first ore row %v is not inside the window %v", first, win)
			}
			lower := image.Rect(first.Min.X, first.Min.Y+first.Dy()/2, first.Max.X, first.Max.Y)
			if n := captureInk(img, lower, body); n < 30 {
				fail("ore row %v strip band carries %d ink pixels, want the value-share strip", lower, n)
			}
			bar := f.math.RowWidgetForTest(0).GetWidget().Rect
			if n := captureInk(img, bar, body); n < 30 {
				fail("math row %v carries %d ink pixels, want its label and figure", bar, n)
			}

			if len(errs) > 0 {
				return fmt.Errorf("capture %s: %s", out, strings.Join(errs, "; "))
			}
			return nil
		},
		Out: out,
	}).Run(t)
}
