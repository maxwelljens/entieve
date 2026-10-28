package screens

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/entieve/entieve/internal/capture"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// orderCaptureFrames is how many draws the three stages settle over: clear
// the fields, snapshot the blank control, type into both, then sample.
const orderCaptureFrames = 6

// TestOrderWindowCaptureFrame is the visual pin for the typed-field
// rendering bug (price/qty text invisible): the dialog's body carries two
// ebitenui TextInputs, which draw through a screen-sized MaskedRenderBuffer
// — a frame-clipped destination drops their text. The capture mounts the
// dialog the way the kit does (clipWindowBody then AddWindow), records the
// empty (blank) fields, types into both and asserts the glyphs land as
// pixels inside each field's rect that were not there when it was blank.
// Skipped unless ENTIEVE_CAPTURE=1; the PNG lands in $ENTIEVE_CAPTURE_DIR
// (default /tmp/opencode).
func TestOrderWindowCaptureFrame(t *testing.T) {
	_, _, env := orderStub([]sim.MarketStock{
		{Item: "item-stub-ore", Price: 100, Qty: 5, Side: sim.OrderSell},
		{Item: "item-stub-ore", Price: 90, Qty: 2, Side: sim.OrderBuy},
	})
	w := NewOrderWin(env)
	clipWindowBody(w.Win()) // the kit's mount path: decide wrap vs unwrap
	ui := newWindowUI(t, env, w.Win())
	w.Show("item-stub-ore", sim.OrderSell, 5)

	var (
		frames int // completed draws
		last   *ebiten.Image

		stage      int
		stageFrame int

		blankPrice, blankQty []uint32
		priceRect, qtyRect   image.Rectangle

		priceDiff, qtyDiff int
		priceInk, qtyInk   int
		blankPriceInk      int
	)
	out := capture.Out("orderwin_capture.png")
	(&capture.Capture{
		Name:   "Entieve order dialog capture",
		Frames: orderCaptureFrames,
		Size:   image.Pt(1024, 700),
		// The stage gates count DRAWS (ebiten runs Update on its own cadence,
		// several per draw), so every state change is rendered before it is
		// sampled.
		Update: func(int) error {
			ui.Update()
			switch {
			case stage == 0 && frames >= 2:
				// Blank control: the sell defaults are pre-filled at Show, so clear
				// both fields and let the draws settle the empty look.
				w.price.SetText("")
				w.qty.SetText("")
				stage, stageFrame = 1, frames
			case stage == 1 && frames >= stageFrame+2:
				priceRect = w.price.GetWidget().Rect
				qtyRect = w.qty.GetWidget().Rect
				blankPrice = snapshot(last, priceRect)
				blankQty = snapshot(last, qtyRect)
				blankPriceInk = uitest.InkIn(last, priceRect)
				w.price.SetText("12345.67")
				w.qty.SetText("42")
				stage, stageFrame = 2, frames
			}
			return nil
		},
		Draw: func(_ int, img *ebiten.Image) error {
			ui.Draw(img)
			last = img
			frames++
			return nil
		},
		Verify: func(img *ebiten.Image) error {
			priceInk = uitest.InkIn(img, priceRect)
			qtyInk = uitest.InkIn(img, qtyRect)

			var errs []string
			fail := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }
			if r := w.price.GetWidget().Rect; r != priceRect {
				fail("price rect moved %v -> %v between blank and typed frames", priceRect, r)
			}
			if r := w.qty.GetWidget().Rect; r != qtyRect {
				fail("qty rect moved %v -> %v between blank and typed frames", qtyRect, r)
			}
			priceDiff = diffCount(img, priceRect, blankPrice)
			qtyDiff = diffCount(img, qtyRect, blankQty)
			// 8 digits and 2 digits of glyphs; a wrapped buffer drops them all.
			if priceDiff < 8 {
				fail("price field %v: typing changed %d pixels, want the glyphs (invisible text?)", priceRect, priceDiff)
			}
			if qtyDiff < 6 {
				fail("qty field %v: typing changed %d pixels, want the glyphs (invisible text?)", qtyRect, qtyDiff)
			}
			if len(errs) > 0 {
				return fmt.Errorf("capture %s: %s", out, strings.Join(errs, "; "))
			}
			return nil
		},
		Out: out,
	}).Run(t)
	t.Logf("capture %s: blank field ink=%d, typed ink price=%d qty=%d (blank->typed diff price=%d qty=%d)",
		out, blankPriceInk, priceInk, qtyInk, priceDiff, qtyDiff)
}

// snapshot reads a rect's pixels as channel sums, the blank-field control
// the typed frame is diffed against.
func snapshot(img *ebiten.Image, r image.Rectangle) []uint32 {
	out := make([]uint32, 0, r.Dx()*r.Dy())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			rr, gg, bb, aa := img.At(x, y).RGBA()
			out = append(out, rr+gg+bb+aa)
		}
	}
	return out
}

// diffCount counts the pixels in r whose channels differ from the control.
func diffCount(img *ebiten.Image, r image.Rectangle, ref []uint32) int {
	n, i := 0, 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			rr, gg, bb, aa := img.At(x, y).RGBA()
			if rr+gg+bb+aa != ref[i] {
				n++
			}
			i++
		}
	}
	return n
}
