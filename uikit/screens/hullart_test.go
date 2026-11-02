package screens

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/uikit/hulls"
)

// dropHull writes a solid 8×8 marker PNG at assets/hulls/<id>.png under a
// fresh temp root and points the sprite registry at it for the rest of the
// test (an empty load afterwards, so no test inherits the dropped art).
func dropHull(t *testing.T, id string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hulls", id+".png")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 64, B: 32, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode: %v", err)
	}
	f.Close()
	render.LoadAssets(filepath.Dir(filepath.Dir(path)))
	t.Cleanup(func() { render.LoadAssets("") })
}

// TestHullArtDropInWins: a dropped assets/hulls/<id>.png is the portrait
// both hull-art surfaces draw (paintPortrait calls hullArt); with no file the
// embedded raster returns unchanged, and a hull with neither answers nil.
// Only Bounds is compared on the dropped sprite: ebiten pixel readback needs
// a running game loop.
func TestHullArtDropInWins(t *testing.T) {
	builtin := hulls.Get("ship-vastrek", 120)
	if builtin == nil {
		t.Fatal("fixture: ship-vastrek has no embedded portrait")
	}
	if got := hullArt("ship-vastrek", 120); got.Bounds() != builtin.Bounds() {
		t.Fatalf("no assets: hullArt bounds = %v, want the embedded raster's %v", got.Bounds(), builtin.Bounds())
	}
	if c := color.NRGBAModel.Convert(builtin.At(0, 0)).(color.NRGBA); c.A != 0 {
		t.Errorf("built-in corner = %v, want transparent (the 1 px margin)", c)
	}

	dropHull(t, "ship-vastrek")
	if got := hullArt("ship-vastrek", 120); got.Bounds() != image.Rect(0, 0, 8, 8) {
		t.Errorf("dropped art: hullArt = %v, want the 8×8 drop-in over the %v raster", got.Bounds(), builtin.Bounds())
	}
	if got := hullArt("ship-zdanek", 120); got.Bounds() != builtin.Bounds() {
		t.Errorf("a hull without a file = %v, want the embedded raster", got.Bounds())
	}
	if got := hullArt("ship-kutrek", 120); got != nil {
		t.Errorf("ship-kutrek (no embedded raster, no file) = %v, want nil", got)
	}
}
