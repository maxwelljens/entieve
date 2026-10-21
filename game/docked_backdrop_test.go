package game

import (
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/entieve/entieve/render"
)

// The docked backdrop: the hangar plate fades in over the world while
// docked (12 frames, then held), the caption reads "Docked: <station>" in
// the lower-centre band, the plate is cover-fit, and no artwork in the
// asset root keeps today's veil exactly. The capture
// (TestDockedBackdropCapture) pins the plate under every window.

// writeArtwork drops a w×h JPEG at the artwork convention path under dir.
func writeArtwork(t *testing.T, dir string, w, h int) {
	t.Helper()
	p := filepath.Join(dir, "artwork", "spaceship_hangar.jpg")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.SetRGBA(1, 1, color.RGBA{120, 90, 60, 255})
	f, err := os.Create(p)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatalf("encode: %v", err)
	}
}

// loadArtwork installs a temp artwork root for the test and clears the
// registry afterwards (the package's tests otherwise run art-less: the lazy
// probe looks for assets/ under game/, which does not exist).
func loadArtwork(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	writeArtwork(t, dir, 64, 36)
	render.LoadAssets(dir)
	t.Cleanup(func() { render.LoadAssets("") })
}

// TestStationBackdropFade: the fade counter ramps 0→1 over 12 frames and
// settles at full — never overshooting, never dipping back.
func TestStationBackdropFade(t *testing.T) {
	sc := &StationScene{}
	if got := backdropAlpha(sc.fade); got != 0 {
		t.Fatalf("entry alpha = %v, want 0 (the world shows)", got)
	}
	prev := float32(0)
	for i := 1; i <= stationFadeFrames; i++ {
		if err := sc.Update(Input{}); err != nil {
			t.Fatalf("update %d: %v", i, err)
		}
		a := backdropAlpha(sc.fade)
		if a <= prev {
			t.Errorf("frame %d: alpha %v did not rise over %v", i, a, prev)
		}
		prev = a
	}
	if prev != 1 {
		t.Errorf("fade settles at %v, want 1", prev)
	}
	for i := 0; i < 3; i++ {
		_ = sc.Update(Input{})
	}
	if got := backdropAlpha(sc.fade); got != 1 {
		t.Errorf("after settling, alpha = %v, want 1", got)
	}
}

// TestStationBackdropVeilFallback: without an artwork plate the station
// scene keeps the pre-artwork veil — the same plate/alpha seam Draw reads.
func TestStationBackdropVeilFallback(t *testing.T) {
	render.LoadAssets("")
	art, veil := stationBackdrop()
	if art != nil {
		t.Fatal("artwork plate present with an empty registry")
	}
	if stationVeilAlpha != 150 {
		t.Errorf("stationVeilAlpha = %d, want the legacy veil 150", stationVeilAlpha)
	}
	if veil != stationVeilAlpha {
		t.Errorf("fallback veil alpha = %d, want %d", veil, stationVeilAlpha)
	}
}

// TestStationBackdropArtworkLoaded: with the plate in the asset root the
// scene draws it instead of the veil.
func TestStationBackdropArtworkLoaded(t *testing.T) {
	loadArtwork(t)
	art, veil := stationBackdrop()
	if art == nil {
		t.Fatal("artwork plate did not load from the artwork root")
	}
	if b := art.Bounds(); b.Dx() != 64 || b.Dy() != 36 {
		t.Errorf("plate = %dx%d, want 64x36", b.Dx(), b.Dy())
	}
	if veil != 0 {
		t.Errorf("veil alpha = %d with a plate, want 0 (the plate replaces it)", veil)
	}
}

// TestDockedTitleTextAndPosition pins the caption string (NAMING.md
// interface strings) and its lower-centre anchor.
func TestDockedTitleTextAndPosition(t *testing.T) {
	if got := dockedTitle("Huta"); got != "Docked: Huta" {
		t.Errorf("title = %q, want %q", got, "Docked: Huta")
	}
	if y := dockedTitleY(720); y != 630 {
		t.Errorf("title y at 720 = %v, want 630 (7/8 down)", y)
	}
	if y := dockedTitleY(720); y <= 360 || y >= 720 {
		t.Errorf("title y %v is not in the lower band of 720", y)
	}
	if x := dockedTitleX(1280, 200); x != 540 {
		t.Errorf("title x = %v, want 540 (centred)", x)
	}
}

// TestCoverScale: the plate fills the window on the binding axis and crops
// the other (aspect kept, no letterboxing).
func TestCoverScale(t *testing.T) {
	for _, tc := range []struct {
		name         string
		sw, sh, w, h int
		want         float64
	}{
		{"wide into square", 1000, 500, 800, 800, 1.6},
		{"tall into square", 500, 1000, 800, 800, 1.6},
		{"exact", 1920, 1080, 1280, 720, 1280.0 / 1920},
		{"empty plate", 0, 100, 800, 600, 0},
		{"empty window", 100, 100, 0, 600, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := coverScale(tc.sw, tc.sh, tc.w, tc.h); math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("coverScale(%d,%d,%d,%d) = %v, want %v", tc.sw, tc.sh, tc.w, tc.h, got, tc.want)
			}
		})
	}
	// Cover means both axes of the plate reach past the window.
	s := coverScale(1000, 500, 800, 800)
	if float64(1000)*s < 800 || float64(500)*s < 800 {
		t.Errorf("scale %v does not cover 800×800 with a 1000×500 plate", s)
	}
}
