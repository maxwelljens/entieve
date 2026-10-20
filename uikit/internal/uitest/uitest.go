// Package uitest holds the uikit test support with no widgets/screens
// dependency of its own — the ebiten capture loop, the pixel-ink probes,
// the frame pumps and the scale fixture — shared by the widgets and
// screens test suites. The theme tests never import it: a theme-internal
// test may not import a package that imports theme, so scale_test keeps
// its own local useScale.
package uitest

import (
	"image"
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// ebitenLoopRan records whether this process already drove a window
// loop, so running the whole capture set skips the extras instead of
// panicking.
var ebitenLoopRan bool

// SkipIfLoopRan skips the test when this process already ran a loop.
func SkipIfLoopRan(t *testing.T) {
	t.Helper()
	if ebitenLoopRan {
		t.Skip("one ebiten game loop per test process: run this capture on its own")
	}
}

// RunCaptureLoop drives one capture game through ebiten, records that the
// loop ran and skips when the window system is unavailable.
func RunCaptureLoop(t *testing.T, title string, g ebiten.Game) {
	t.Helper()
	ebiten.SetWindowTitle(title)
	ebiten.SetWindowSize(1280, 720)
	err := ebiten.RunGame(g)
	ebitenLoopRan = true
	if err != nil {
		t.Skipf("cannot run capture game loop: %v", err)
	}
}

// PumpWindow runs one frame: Sync (rows + gating), Update (dispatch
// deferred events) and Draw (layout). It does NOT announce a cursor input
// pass (widgets.BeginCursorFrame): a caller that asserts the unclaimed
// arrow announces one itself, and a caller that never does keeps the last
// claim — the same "draw-only frame repeats the decision" semantics the
// game relies on.
func PumpWindow(t *testing.T, ui *ebitenui.UI, sync func(), w, h int) {
	t.Helper()
	if sync != nil {
		sync()
	}
	ui.Update()
	ui.Draw(ebiten.NewImage(w, h))
}

// FireButton clicks a button through the deferred event queue.
func FireButton(ui *ebitenui.UI, b *widget.Button) {
	b.ClickedEvent.Fire(&widget.ButtonClickedEventArgs{Button: b})
	ui.Update()
}

// SumPrefix adds the first n fitted column widths.
func SumPrefix(cols []float64) float64 {
	s := 0.0
	for _, c := range cols {
		s += c
	}
	return s
}

// UseScale installs s for the test's duration and restores the identity.
func UseScale(t *testing.T, s float64) {
	t.Helper()
	theme.SetScale(s)
	t.Cleanup(func() { theme.SetScale(1) })
}

// inkBoundsFunc is inkBounds with a caller-supplied pixel test.
func inkBoundsFunc(img *ebiten.Image, box image.Rectangle, hit func(x, y int) bool) (image.Rectangle, int) {
	minX, minY, maxX, maxY := 0, 0, 0, 0
	n := 0
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			if !hit(x, y) {
				continue
			}
			if n == 0 {
				minX, minY, maxX, maxY = x, y, x, y
			} else {
				if x < minX {
					minX = x
				}
				if x >= maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y >= maxY {
					maxY = y
				}
			}
			n++
		}
	}
	if n == 0 {
		return image.Rectangle{}, 0
	}
	return image.Rect(minX, minY, maxX+1, maxY+1), n
}

// InkBounds returns the bounding box of bright pixels inside box (and how
// many there are) — the drawn silhouette's on-screen footprint.
func InkBounds(img *ebiten.Image, box image.Rectangle) (image.Rectangle, int) {
	return inkBoundsFunc(img, box, func(x, y int) bool {
		r, gch, b, a := img.At(x, y).RGBA()
		return a != 0 && (r>>8 > 90 || gch>>8 > 90 || b>>8 > 90)
	})
}

func InkIn(img *ebiten.Image, box image.Rectangle) int {
	n := 0
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			r, gch, b, a := img.At(x, y).RGBA()
			if a > 0 && (r>>8 > 90 || gch>>8 > 90 || b>>8 > 90) {
				n++
			}
		}
	}
	return n
}

// AbsInt is |v| (the capture checks only).
func AbsInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// GlyphBounds returns the on-screen footprint of a rack tile's glyph: the
// bounds of every pixel inside the tile that differs from its flat body
// colour (sampled at an inner corner the centred glyph never reaches).
func GlyphBounds(img *ebiten.Image, tile image.Rectangle) (image.Rectangle, int) {
	refR, refG, refB, _ := img.At(tile.Min.X+4, tile.Min.Y+4).RGBA()
	close := func(c interface {
		RGBA() (uint32, uint32, uint32, uint32)
	}) bool {
		r, g, b, _ := c.RGBA()
		return AbsInt(int(r>>8)-int(refR>>8)) <= 14 &&
			AbsInt(int(g>>8)-int(refG>>8)) <= 14 &&
			AbsInt(int(b>>8)-int(refB>>8)) <= 14
	}
	return inkBoundsFunc(img, tile.Inset(3), func(x, y int) bool {
		return !close(img.At(x, y))
	})
}
