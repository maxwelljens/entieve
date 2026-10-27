package game

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

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
	if os.Getenv("ENTIEVE_CAPTURE") == "" {
		t.Skip("set ENTIEVE_CAPTURE=1 to render a capture frame")
	}
	outDir := os.Getenv("ENTIEVE_CAPTURE_DIR")
	if outDir == "" {
		outDir = "/tmp/opencode"
	}
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

	cg := &dockedCaptureGame{
		g: g, assets: assets,
		out:  filepath.Join(outDir, "docked_backdrop_capture.png"),
		bar:  bar,
		grid: grid,
	}
	ebiten.SetWindowTitle("Entieve docked backdrop capture")
	ebiten.SetWindowSize(1280, 720)
	// Capture contract: the loop never shows a window on the desktop.
	ebiten.SetWindowVisible(false)
	if err := ebiten.RunGame(cg); err != nil {
		t.Skipf("cannot run capture game loop: %v", err)
	}
	if cg.err != nil {
		t.Fatal(cg.err)
	}
	if !cg.saved {
		t.Fatal("capture never rendered")
	}
	t.Logf("capture written to %s (title %q, bar samples %d, grid deltas small=%d large=%d)",
		cg.out, dockedTitle(info.Name), len(bar), cg.small, cg.large)
}

// dockedCaptureGame drives the real Game through one ebiten loop and, on
// the settled frame, samples it with and without the artwork plate.
type dockedCaptureGame struct {
	g      *Game
	assets string
	out    string
	bar    []image.Point
	grid   []image.Point

	draws int
	done  bool
	saved bool
	small int // grid points that changed slightly (plate under translucent chrome)
	large int // grid points that changed a lot (bare plate area)
	err   error
}

func (cg *dockedCaptureGame) Update() error {
	if cg.done || cg.err != nil {
		return ebiten.Termination
	}
	return cg.g.Update()
}

func (cg *dockedCaptureGame) Draw(screen *ebiten.Image) {
	img := ebiten.NewImage(1280, 720)
	cg.g.Draw(img)
	screen.DrawImage(img, nil)
	cg.draws++
	if cg.done || cg.err != nil || cg.draws < 16 { // window open fades settle by 12
		return
	}
	// The veil-only frame: same game state, no artwork registered.
	render.LoadAssets("")
	plain := ebiten.NewImage(1280, 720)
	cg.g.Draw(plain)
	render.LoadAssets(cg.assets)

	var errs []string
	for _, p := range cg.bar {
		if img.At(p.X, p.Y) != plain.At(p.X, p.Y) {
			errs = append(errs, fmt.Sprintf("log title bar %v changed with the plate (%v vs %v) — "+
				"the backdrop is painting over the windows", p, img.At(p.X, p.Y), plain.At(p.X, p.Y)))
			break
		}
	}
	for _, p := range cg.grid {
		d := delta(img.At(p.X, p.Y), plain.At(p.X, p.Y))
		switch {
		case d > 60:
			cg.large++
		case d > 4:
			cg.small++
		}
	}
	if cg.large == 0 {
		errs = append(errs, "no bare screen point changed with the plate — the artwork never paints")
	}

	// Screenshot first: evidence is useful even when an assert failed.
	f, err := os.Create(cg.out)
	if err != nil {
		cg.err = err
		return
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		cg.err = err
		return
	}
	if err := f.Close(); err != nil {
		cg.err = err
		return
	}
	cg.saved = true
	cg.done = true
	if len(errs) > 0 {
		cg.err = fmt.Errorf("capture %s: %s", cg.out, errs[0])
	}
}

func (cg *dockedCaptureGame) Layout(w, h int) (int, int) { return 1280, 720 }

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
