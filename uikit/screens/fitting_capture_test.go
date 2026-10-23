package screens

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/hajimehoshi/ebiten/v2"
)

// fitCaptureFrames is how many draws settle before the sample (the window
// fades in over 12 frames).
const fitCaptureFrames = 13

// TestFittingCaptureFrame draws the fitting window through a real ebiten
// loop and pins the pixels the headless suite cannot read back: the hull
// portrait's ink inside its frame, the maker emblem beside the class ·
// maker line, the module icon centred in a fitted tile, and the dashed
// outline (border ink, hollow centre) of an empty slot.
//
// Skipped unless ENTIEVE_CAPTURE=1; the PNG lands in $ENTIEVE_CAPTURE_DIR
// (default /tmp/opencode).
func TestFittingCaptureFrame(t *testing.T) {
	if os.Getenv("ENTIEVE_CAPTURE") == "" {
		t.Skip("set ENTIEVE_CAPTURE=1 to render a capture frame")
	}
	uitest.SkipIfLoopRan(t)
	outDir := os.Getenv("ENTIEVE_CAPTURE_DIR")
	if outDir == "" {
		outDir = "/tmp/opencode"
	}

	k, _ := newTestKit(t)
	fit := NewFitting(k.Env())
	k.AddWindow(fit.Win())
	fit.Win().SetLocation(image.Rect(370, 8, 1014, 690))

	// One empty high slot so the sweep also shows a placeholder tile.
	if f, ok := k.Sim.Fittings.Get(k.Sim.Player()); ok {
		f.High = f.High[:len(f.High)-1]
	}

	g := &fitCaptureGame{
		k:   k,
		fit: fit,
		out: filepath.Join(outDir, "fitting_capture.png"),
	}
	uitest.RunCaptureLoop(t, "Entieve fitting capture", g)
	if g.err != nil {
		t.Fatal(g.err)
	}
	if !g.saved {
		t.Fatal("capture never rendered")
	}
	t.Logf("capture written to %s", g.out)
}

// fitCaptureGame drives the kit and verifies the settled frame.
type fitCaptureGame struct {
	k      *Kit
	fit    *Fitting
	out    string
	frames int
	last   *ebiten.Image
	saved  bool
	err    error
}

func (g *fitCaptureGame) Update() error {
	if g.saved || g.err != nil {
		return ebiten.Termination
	}
	g.fit.Sync()
	if err := g.k.Update(); err != nil {
		g.err = err
		return ebiten.Termination
	}
	if g.frames > fitCaptureFrames && g.last != nil {
		g.err = g.verify(g.last)
		return ebiten.Termination
	}
	return nil
}

func (g *fitCaptureGame) Draw(screen *ebiten.Image) {
	img := ebiten.NewImage(1280, 720)
	img.Fill(color.White)
	g.k.Draw(img)
	g.last = img
	g.frames++
	screen.DrawImage(img, nil)
}

func (g *fitCaptureGame) Layout(w, h int) (int, int) { return 1280, 720 }

func (g *fitCaptureGame) verify(img *ebiten.Image) error {
	var errs []string
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Sprintf(format, args...))
	}
	// The window body fill is the reference every placeholder read
	// compares against: the gutter between the stat column and the split.
	body := img.At(606, 300)
	same := func(pt image.Point) bool {
		r0, g0, b0, _ := body.RGBA()
		r, g, b, _ := img.At(pt.X, pt.Y).RGBA()
		return uitest.AbsInt(int(r>>8)-int(r0>>8)) <= 6 &&
			uitest.AbsInt(int(g>>8)-int(g0>>8)) <= 6 &&
			uitest.AbsInt(int(b>>8)-int(b0>>8)) <= 6
	}
	differing := func(box image.Rectangle) int {
		n := 0
		for y := box.Min.Y; y < box.Max.Y; y++ {
			for x := box.Min.X; x < box.Max.X; x++ {
				if !same(image.Pt(x, y)) {
					n++
				}
			}
		}
		return n
	}

	f, err := os.Create(g.out)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return err
	}
	g.saved = true

	// The portrait frame carries the hull art.
	frame := g.fit.portraitGfx.GetWidget().Rect
	if got := differing(frame); got < 400 {
		fail("portrait %v has %d non-body pixels, want the hull art", frame, got)
	}

	// The maker emblem leads the class · maker line.
	kids := g.fit.idRow.Children()
	if len(kids) != 2 {
		fail("identity row has %d children, want the emblem + the maker line", len(kids))
	} else if got := differing(kids[0].GetWidget().Rect.Inset(2)); got < 20 {
		fail("emblem box %v has %d non-body pixels, want the maker mark", kids[0].GetWidget().Rect, got)
	}

	// A fitted tile's module icon sits centred in the 50×40 cell (±4 px,
	// the rack's discipline); an empty placeholder is dashed (border ink
	// with gaps) and hollow.
	fitted, empty := 0, 0
	for row := range g.fit.tiles {
		for _, tile := range g.fit.tiles[row] {
			if !fitTileShown(tile) {
				continue
			}
			tileRect := tile.btn.GetWidget().Rect
			if tile.mod == "" {
				empty++
				mid := image.Pt((tileRect.Min.X+tileRect.Max.X)/2, (tileRect.Min.Y+tileRect.Max.Y)/2)
				if !same(mid) {
					fail("empty tile %v is not hollow at its centre", tileRect)
				}
				var dashes int
				for x := tileRect.Min.X; x < tileRect.Max.X; x++ {
					if !same(image.Pt(x, tileRect.Min.Y)) {
						dashes++
					}
				}
				if w := tileRect.Dx(); dashes < w/4 || dashes > 3*w/4 {
					fail("empty tile %v draws %d/%d lit pixels on its top edge, want a dashed outline", tileRect, dashes, w)
				}
				continue
			}
			b, n := uitest.GlyphBounds(img, tileRect)
			if n < 8 {
				fail("tile %v has %d ink pixels for %s, want a drawn glyph", tileRect, n, tile.icon)
				continue
			}
			if dx := (b.Min.X+b.Max.X)/2 - (tileRect.Min.X+tileRect.Max.X)/2; uitest.AbsInt(dx) > 4 {
				fail("tile %v glyph centre is %d px off (icon %s)", tileRect, dx, tile.icon)
			}
			if dy := (b.Min.Y+b.Max.Y)/2 - (tileRect.Min.Y+tileRect.Max.Y)/2; uitest.AbsInt(dy) > 4 {
				fail("tile %v glyph is %d px off vertically (icon %s)", tileRect, dy, tile.icon)
			}
			fitted++
		}
	}
	if fitted == 0 {
		fail("no fitted tiles to check")
	}
	if empty == 0 {
		fail("no empty placeholder tile to check")
	}

	// The summary's capacitor bar paints its track and fill.
	bar := g.fit.capBarGfx.GetWidget().Rect
	if same(image.Pt(bar.Min.X+2, (bar.Min.Y+bar.Max.Y)/2)) {
		fail("cap bar %v is invisible against the window body", bar)
	}

	if len(errs) > 0 {
		return fmt.Errorf("capture %s: %s", g.out, strings.Join(errs, "; "))
	}
	return nil
}
