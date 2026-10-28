package game

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"testing"

	"github.com/entieve/entieve/internal/capture"
	"github.com/entieve/entieve/render"
	uikit "github.com/entieve/entieve/uikit/screens"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestDockedBackdropCapture: the docked hangar plate draws UNDER every
// window (scene order: StationScene, then kit.Draw) and the caption sits
// over it. Pixel readback needs a running ebiten loop, so this is the gated
// capture (ENTIEVE_CAPTURE=1, one loop per process — run it on its own);
// the PNG lands in $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
//
// Method: dock at the first station, settle the 12-frame fade, then on one
// frame draw twice — once with the real asset root, once with an empty
// registry (the veil fallback) — into two images:
//   - the log window's opaque title bar must be pixel-identical between the
//     two: the window covers the backdrop (were the plate painted above the
//     chrome, the bar would change);
//   - bare screen points must differ: the plate really paints;
//   - large deltas count the bare plate area, small ones the tinted windows.
func TestDockedBackdropCapture(t *testing.T) {
	assets, err := filepath.Abs("../assets")
	if err != nil {
		t.Fatalf("assets path: %v", err)
	}
	render.LoadAssets(assets)
	defer render.LoadAssets("")
	if render.DockedArtwork() == nil {
		t.Fatalf("%s carries no artwork/spaceship_hangar plate", assets)
	}

	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	dockAt(t, g)
	g.host.SetOpen(idLog, true)
	g.syncScenes()
	g.syncMounts()
	// Settle layout and the fade through the production frame path.
	for i := 0; i < stationFadeFrames+3; i++ {
		if err := g.Update(); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
	}
	info, ok := g.env.DockedStation()
	if !ok || info.Name == "" {
		t.Fatalf("fixture: not docked (%+v, %v)", info, ok)
	}
	logRect := bodyOf[*uikit.LogPanel](g.host, idLog).Win().GetContainer().GetWidget().Rect
	if dirs := logRect.Dx(); dirs <= 0 {
		t.Fatalf("fixture: log window never laid out (%v)", logRect)
	}

	// Sample points: the opaque log title bar (the under-window pin) and a
	// coarse screen grid (the plate-is-visible pin).
	barY := logRect.Min.Y + theme.Px(theme.TitleBarH)/2
	var bar []image.Point
	for x := logRect.Min.X + theme.Px(24) + 4; x < logRect.Max.X-6; x += 7 {
		if x < theme.Px(uikit.RailInset)+2 {
			continue // the rail is top chrome, not a window
		}
		bar = append(bar, image.Pt(x, barY))
	}
	var grid []image.Point
	for y := 4; y < 720; y += 16 {
		for x := 4; x < 1280; x += 16 {
			grid = append(grid, image.Pt(x, y))
		}
	}
	if len(bar) < 8 {
		t.Fatalf("fixture: only %d title-bar samples in %v", len(bar), logRect)
	}

	var plain *ebiten.Image // the veil-only frame: same state, no artwork registered
	var small, large int
	out := capture.Out("docked_backdrop_capture.png")
	(&capture.Capture{
		Name:   "Entieve docked backdrop capture",
		Frames: 16, // the window open fades settle by 12
		Update: func(int) error { return g.Update() },
		Draw: func(n int, img *ebiten.Image) error {
			g.Draw(img)
			if n == 16 {
				render.LoadAssets("")
				plain = ebiten.NewImage(1280, 720)
				g.Draw(plain)
				render.LoadAssets(assets)
			}
			return nil
		},
		Verify: func(img *ebiten.Image) error {
			var errs []string
			for _, p := range bar {
				if img.At(p.X, p.Y) != plain.At(p.X, p.Y) {
					errs = append(errs, fmt.Sprintf("log title bar %v changed with the plate (%v vs %v) — "+
						"the backdrop is painting over the windows", p, img.At(p.X, p.Y), plain.At(p.X, p.Y)))
					break
				}
			}
			for _, p := range grid {
				d := delta(img.At(p.X, p.Y), plain.At(p.X, p.Y))
				switch {
				case d > 60:
					large++
				case d > 4:
					small++
				}
			}
			if large == 0 {
				errs = append(errs, "no bare screen point changed with the plate — the artwork never paints")
			}
			if len(errs) > 0 {
				return fmt.Errorf("capture %s: %s", out, errs[0])
			}
			return nil
		},
		Out: out,
	}).Run(t)
	t.Logf("capture written to %s (title %q, bar samples %d, grid deltas small=%d large=%d)",
		out, dockedTitle(info.Name), len(bar), small, large)
}

// delta is the largest per-channel difference between two sampled pixels.
func delta(a, b color.Color) int {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	d := func(x, y uint32) int {
		if x > y {
			return int((x - y) >> 8)
		}
		return int((y - x) >> 8)
	}
	return max(d(ar, br), max(d(ag, bg), d(ab, bb)))
}
