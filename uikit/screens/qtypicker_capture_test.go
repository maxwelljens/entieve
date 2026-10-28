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

// TestRefineryPickerCapture renders the refinery with an ore selected and
// the quantity picker up, through a real ebiten loop, and pins what the
// headless suite cannot read back: the modal little window over the
// panel, its notched ladder (one tick per whole 100-ore batch, the handle
// on the first), the live readout and the preview line. The PNG lands in
// $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
//
// Skipped unless ENTIEVE_CAPTURE=1; one ebiten loop per process — run it
// on its own:
//
//	ENTIEVE_CAPTURE=1 go test ./uikit/screens/ -run TestRefineryPickerCapture
func TestRefineryPickerCapture(t *testing.T) {
	k, _ := newTestKit(t)
	k.SetCoreVisible(false) // the refinery and its picker only
	ref := NewRefinery(k.Env())
	k.AddWindow(ref.Win())
	ref.Win().SetLocation(image.Rect(24, 24, 24+theme.Px(430), 24+theme.Px(430)))
	// 250 ore held: a two-rung ladder (100, 200) and the 200-ore preview.
	giveItem(k.Sim, "ore-cordite", 150)
	k.Sim.Docked.Set(k.Sim.Player(), sim.Docked{At: "cel-haven-rest"})

	var (
		frames int // completed draws
		last   *ebiten.Image
		openAt int // the draw the selection gesture ran (0 = not yet)
	)
	out := capture.Out("picker_capture.png")
	var cap *capture.Capture
	cap = &capture.Capture{
		Name: "Entieve refinery quantity picker capture",
		// The picker opens mid-loop, so the scenario ends the loop itself.
		Update: func(int) error {
			if openAt == 0 && frames >= openFrames+1 {
				row := -1
				for i, tr := range ref.ores.Rows {
					if d, ok := tr.Data.(refineryRow); ok && d.ore == "ore-cordite" {
						row = i
					}
				}
				if row < 0 {
					return fmt.Errorf("no cordite row in %+v", ref.ores.Rows)
				}
				clickRow(k.ui, ref.ores, row) // the selection gesture
				openAt = frames
				// The kit centres the picker on the mount (inside the click), so
				// move it after the gesture: beside the panel rather than over its
				// status line, so the frame shows the whole body next to the list
				// it came from.
				q := k.QtyWin().Win()
				q.SetLocation(image.Rect(470, 300, 470+theme.Px(qtyMinW), 300+theme.Px(qtyMinH)))
			}
			if err := k.Update(); err != nil {
				return err
			}
			if openAt > 0 && frames > openAt+openFrames+2 && last != nil {
				cap.Stop()
			}
			return nil
		},
		Draw: func(_ int, img *ebiten.Image) error {
			img.Fill(color.White)
			ref.Sync()
			k.Draw(img)
			last = img
			frames++
			return nil
		},
		// verify pins the picker's pixels: the window is up
		// and modal, the ladder drew one tick per whole batch with the handle on
		// the first rung, and the readout/preview agree with the picked value.
		Verify: func(img *ebiten.Image) error {
			var errs []string
			fail := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }

			q := k.QtyWin()
			if q == nil || !q.IsOpen() {
				return fmt.Errorf("capture %s: the picker is not up", out)
			}
			win := q.Win().GetContainer().GetWidget().Rect
			sl := q.Slider()
			ticks := sl.TickPositions()
			if len(ticks) != 2 {
				fail("ladder ticks = %d, want 2 (100 and 200 held)", len(ticks))
			}
			if got := sl.Value(); got != 100 {
				fail("picker value = %v, want the seeded 100", got)
			}
			if got := q.read.Label; got != "100" {
				fail("readout = %q, want 100", got)
			}
			if got := q.DetailText(); !strings.Contains(got, "Refine 100 × ") {
				fail("preview = %q, want the one-batch conversion", got)
			}
			if got := q.NoteText(); !strings.Contains(got, "hangar") {
				fail("note = %q, want the hangar destination", got)
			}

			// The picker paints over the panel: its body colour sits inside its
			// rect, and the slider's band carries the track/handle ink (not the
			// bare body colour) at the handle's position.
			body := img.At(win.Min.X+theme.Px(theme.RadiusWindow)+4, win.Max.Y-6)
			if lum(body) == 0 {
				fail("picker body sample at %v reads black (the window did not paint)", win)
			}
			band := sl.HitBand()
			handle := image.Pt(sl.ValueX(sl.Value()), (band.Min.Y+band.Max.Y)/2)
			if !handle.In(win) {
				fail("slider handle %v outside the picker %v", handle, win)
			}
			if d := channelDelta(rgb(img.At(handle.X, handle.Y)), rgb(body)); d < 20 {
				fail("handle pixel at %v reads the body colour (delta %d): no handle drawn", handle, d)
			}
			// Both rungs are marked: a tick column differs from the track fill.
			fill := rgb(img.At(band.Min.X+4, band.Min.Y+2))
			for i, x := range ticks {
				if d := channelDelta(rgb(img.At(x, band.Min.Y+band.Dy()/2)), fill); d < 8 {
					fail("ladder tick %d at x=%d does not stand out of the track (delta %d)", i, x, d)
				}
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

// rgb converts a colour to the byte triple the probes compare.
func rgb(c color.Color) color.RGBA {
	r, gg, b, a := c.RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(gg >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

// lum is a colour's summed brightness (0 = black).
func lum(c color.Color) int {
	p := rgb(c)
	return int(p.R) + int(p.G) + int(p.B)
}
