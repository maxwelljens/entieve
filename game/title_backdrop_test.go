package game

import (
	"bytes"
	"image/jpeg"
	"path/filepath"
	"testing"

	"github.com/entieve/entieve/assets/artwork"
	"github.com/entieve/entieve/render"
	"github.com/hajimehoshi/ebiten/v2"
)

// The title backdrop is the embedded title_background.jpg. These pin the
// embed, the cover fit and the two draw branches; the drawn pixels are the
// gated captures' job (nothing reads pixels before ebiten.RunGame starts).

// TestTitleArtworkEmbedded: the shim carries the plate and it decodes to a
// real image.
func TestTitleArtworkEmbedded(t *testing.T) {
	if len(artwork.Title) == 0 {
		t.Fatal("artwork.Title is empty: title_background.jpg not embedded")
	}
	img, err := jpeg.Decode(bytes.NewReader(artwork.Title))
	if err != nil {
		t.Fatalf("embedded title plate does not decode: %v", err)
	}
	if b := img.Bounds(); b.Dx() <= 0 || b.Dy() <= 0 {
		t.Fatalf("embedded title plate is empty: %v", b)
	}
}

// TestTitleArtworkCoverFits: the plate covers the design frame and odd
// sizes (CoverScale's contract: the larger axis scale, both axes ≥ frame).
func TestTitleArtworkCoverFits(t *testing.T) {
	img, err := jpeg.Decode(bytes.NewReader(artwork.Title))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	b := img.Bounds()
	for _, f := range []struct{ w, h int }{{1280, 720}, {1024, 700}, {1919, 1081}} {
		s := render.CoverScale(b.Dx(), b.Dy(), f.w, f.h)
		dw, dh := float64(b.Dx())*s, float64(b.Dy())*s
		if dw < float64(f.w) || dh < float64(f.h) {
			t.Errorf("cover %dx%d → %.1f×%.1f does not fill %dx%d", b.Dx(), b.Dy(), dw, dh, f.w, f.h)
		}
	}
}

// TestTitleBackdropBranches: the phase draws the embedded plate when the
// seam yields one, and takes the static-world fallback when it yields nil.
func TestTitleBackdropBranches(t *testing.T) {
	g := newTitleGameAt(t, filepath.Join(t.TempDir(), "save.json"))
	g.applySize(1280, 720)
	plate := titleArtwork
	defer func() { titleArtwork = plate }()
	img := ebiten.NewImage(1280, 720)

	if got := g.drawTitleBackdrop(img); got == nil {
		t.Fatal("shipped plate not drawn: the title took the world fallback")
	}
	titleArtwork = func() *ebiten.Image { return nil }
	if got := g.drawTitleBackdrop(img); got != nil {
		t.Fatal("nil plate: the title did not take the world fallback")
	}
}
