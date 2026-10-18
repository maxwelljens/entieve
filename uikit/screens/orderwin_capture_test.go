package screens

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/hajimehoshi/ebiten/v2"
)

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
	if os.Getenv("ENTIEVE_CAPTURE") == "" {
		t.Skip("set ENTIEVE_CAPTURE=1 to render a capture frame")
	}
	uitest.SkipIfLoopRan(t)
	outDir := os.Getenv("ENTIEVE_CAPTURE_DIR")
	if outDir == "" {
		outDir = "/tmp/opencode"
	}

	_, _, env := orderStub([]sim.MarketStock{
		{Item: "item-stub-ore", Price: 100, Qty: 5, Side: sim.OrderSell},
		{Item: "item-stub-ore", Price: 90, Qty: 2, Side: sim.OrderBuy},
	})
	w := NewOrderWin(env)
	clipWindowBody(w.Win()) // the kit's mount path: decide wrap vs unwrap
	ui := newWindowUI(t, env, w.Win())
	w.Show("item-stub-ore", sim.OrderSell, 5)

	g := &orderCaptureGame{ui: ui, w: w, out: filepath.Join(outDir, "orderwin_capture.png")}
	uitest.RunCaptureLoop(t, "Entieve order dialog capture", g)
	if g.err != nil {
		t.Fatal(g.err)
	}
	if !g.saved {
		t.Fatal("capture never rendered")
	}
	t.Logf("capture %s: blank field ink=%d, typed ink price=%d qty=%d (blank->typed diff price=%d qty=%d)",
		g.out, g.blankPriceInk, g.priceInk, g.qtyInk, g.priceDiff, g.qtyDiff)
}

// orderCaptureGame drives the dialog: blank the fields, snapshot those
// rects, type into both, then compare the drawn field pixels against the
// blank control. The stage gates count DRAWS (ebiten runs Update on its
// own cadence, several per draw), so every state change is rendered
// before it is sampled.
type orderCaptureGame struct {
	ui interface {
		Update()
		Draw(*ebiten.Image)
	}
	w   *OrderWin
	out string

	frames int // completed draws
	last   *ebiten.Image

	stage      int
	stageFrame int

	blankPrice, blankQty []uint32
	priceRect, qtyRect   image.Rectangle

	priceDiff, qtyDiff int
	priceInk, qtyInk   int
	blankPriceInk      int
	saved              bool
	err                error
}

func (g *orderCaptureGame) Update() error {
	if g.saved || g.err != nil {
		return ebiten.Termination
	}
	g.ui.Update()
	switch {
	case g.stage == 0 && g.frames >= 2:
		// Blank control: the sell defaults are pre-filled at Show, so clear
		// both fields and let the draws settle the empty look.
		g.w.price.SetText("")
		g.w.qty.SetText("")
		g.stage, g.stageFrame = 1, g.frames
	case g.stage == 1 && g.frames >= g.stageFrame+2:
		g.priceRect = g.w.price.GetWidget().Rect
		g.qtyRect = g.w.qty.GetWidget().Rect
		g.blankPrice = snapshot(g.last, g.priceRect)
		g.blankQty = snapshot(g.last, g.qtyRect)
		g.blankPriceInk = uitest.InkIn(g.last, g.priceRect)
		g.w.price.SetText("12345.67")
		g.w.qty.SetText("42")
		g.stage, g.stageFrame = 2, g.frames
	case g.stage == 2 && g.frames >= g.stageFrame+2:
		g.verify(g.last)
		return ebiten.Termination
	}
	return nil
}

func (g *orderCaptureGame) Draw(screen *ebiten.Image) {
	img := ebiten.NewImage(1024, 700)
	g.ui.Draw(img)
	screen.DrawImage(img, nil)
	g.last = img
	g.frames++
}

func (g *orderCaptureGame) Layout(w, h int) (int, int) { return 1024, 700 }

func (g *orderCaptureGame) verify(img *ebiten.Image) {
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

	g.priceInk = uitest.InkIn(img, g.priceRect)
	g.qtyInk = uitest.InkIn(img, g.qtyRect)

	var errs []string
	fail := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }
	if r := g.w.price.GetWidget().Rect; r != g.priceRect {
		fail("price rect moved %v -> %v between blank and typed frames", g.priceRect, r)
	}
	if r := g.w.qty.GetWidget().Rect; r != g.qtyRect {
		fail("qty rect moved %v -> %v between blank and typed frames", g.qtyRect, r)
	}
	g.priceDiff = diffCount(img, g.priceRect, g.blankPrice)
	g.qtyDiff = diffCount(img, g.qtyRect, g.blankQty)
	// 8 digits and 2 digits of glyphs; a wrapped buffer drops them all.
	if g.priceDiff < 8 {
		fail("price field %v: typing changed %d pixels, want the glyphs (invisible text?)", g.priceRect, g.priceDiff)
	}
	if g.qtyDiff < 6 {
		fail("qty field %v: typing changed %d pixels, want the glyphs (invisible text?)", g.qtyRect, g.qtyDiff)
	}
	if len(errs) > 0 {
		g.err = fmt.Errorf("capture %s: %s", g.out, strings.Join(errs, "; "))
	}
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
