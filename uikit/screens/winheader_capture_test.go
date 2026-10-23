package screens

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestHeaderCaptureFrame draws the Refinery and Manufacturing windows through
// a real ebiten loop and pins the pixels the headless suite cannot read back:
// the plate inside each window's band under the title, its alpha falling to
// nothing at the band's bottom edge, the rows painting OVER it, and nothing
// changing outside the band. With no plates under assets/artwork/ it pins the
// fallback instead: both windows byte-identical to the empty-registry frame —
// the plain body, exactly as before.
//
// Skipped unless ENTIEVE_CAPTURE=1; the PNG lands in $ENTIEVE_CAPTURE_DIR
// (default /tmp/opencode). One ebiten loop per process — run it on its own:
//
//	ENTIEVE_CAPTURE=1 go test ./uikit/screens/ -run TestHeaderCaptureFrame
func TestHeaderCaptureFrame(t *testing.T) {
	if os.Getenv("ENTIEVE_CAPTURE") == "" {
		t.Skip("set ENTIEVE_CAPTURE=1 to render a capture frame")
	}
	uitest.SkipIfLoopRan(t)
	outDir := os.Getenv("ENTIEVE_CAPTURE_DIR")
	if outDir == "" {
		outDir = "/tmp/opencode"
	}
	assets, err := filepath.Abs("../../assets")
	if err != nil {
		t.Fatalf("assets path: %v", err)
	}
	render.LoadAssets(assets)
	defer render.LoadAssets("")

	k, _ := newTestKit(t)
	k.SetCoreVisible(false) // the two service windows only: a clean band frame
	ref, man := NewRefinery(k.Env()), NewManufacturing(k.Env())
	k.AddWindow(ref.Win())
	k.AddWindow(man.Win())
	// Side by side, both bodies (and their bands) fully on screen.
	ref.Win().SetLocation(image.Rect(24, 24, 24+theme.Px(430), 24+theme.Px(430)))
	man.Win().SetLocation(image.Rect(500, 24, 500+theme.Px(460), 24+theme.Px(520)))

	g := &headerCaptureGame{k: k, ref: ref, man: man, out: filepath.Join(outDir, "window_headers_capture.png")}
	uitest.RunCaptureLoop(t, "Entieve window headers capture", g)
	if g.err != nil {
		t.Fatal(g.err)
	}
	if !g.saved {
		t.Fatal("capture never rendered")
	}
	t.Logf("capture written to %s (plates present: %v)", g.out, g.plates)
}

// headerCaptureGame drives the kit and, on the settled frame, draws once more
// with an empty sprite registry (no plates) to compare against.
type headerCaptureGame struct {
	k   *Kit
	ref *Refinery
	man *Manufacturing
	out string

	frames int
	last   *ebiten.Image
	plates bool
	saved  bool
	err    error
}

func (g *headerCaptureGame) Update() error {
	if g.saved || g.err != nil {
		return ebiten.Termination
	}
	g.ref.Sync()
	g.man.Sync()
	if err := g.k.Update(); err != nil {
		g.err = err
		return ebiten.Termination
	}
	if g.frames > openFrames+2 && g.last != nil {
		g.err = g.verify(g.last)
		return ebiten.Termination
	}
	return nil
}

func (g *headerCaptureGame) Draw(screen *ebiten.Image) {
	img := ebiten.NewImage(1280, 720)
	img.Fill(color.White)
	g.k.Draw(img)
	g.last = img
	g.frames++
	screen.DrawImage(img, nil)
}

func (g *headerCaptureGame) Layout(w, h int) (int, int) { return 1280, 720 }

// verify compares the settled frame (plates loaded) with the plain one (empty
// registry, same widgets) and with the release frame (empty registry, the
// tables pumped so their viewports are today's filled ones).
func (g *headerCaptureGame) verify(img *ebiten.Image) error {
	plates := render.RefiningHeader() != nil && render.ManufacturingHeader() != nil
	render.LoadAssets("")
	// The plain frame: the same widget state (the banded viewport still in
	// place) with the plate gone — what the band contributes.
	plain := ebiten.NewImage(1280, 720)
	plain.Fill(color.White)
	g.k.Draw(plain)
	// The release frame: no plate, so the tables pump the empty band and the
	// viewport is today's again (the fill covers it) — the old look.
	g.ref.Sync()
	g.man.Sync()
	today := ebiten.NewImage(1280, 720)
	today.Fill(color.White)
	g.k.Draw(today)
	g.plates = plates

	f, err := os.Create(g.out)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	g.saved = true

	shot, was, now := readFrame(img), readFrame(plain), readFrame(today)

	refBody := g.ref.Win().Contents.(*headerBand).GetWidget().Rect
	manBody := g.man.Win().Contents.(*headerBand).GetWidget().Rect
	bands := []image.Rectangle{headerBandRect(refBody), headerBandRect(manBody)}

	var errs []string
	fail := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }

	// Anything outside the two bands must be identical to the no-plate frame:
	// the plate is confined to the band and no layout moved, plates or not.
	if n := diffsOutside(shot, was, bands, 0); n > 0 {
		fail("%d sampled pixels outside the bands changed against the plain frame "+
			"(the plate must stay in the band)", n)
	}

	if !plates {
		if n := diffsOutside(shot, was, nil, 0); n > 0 {
			fail("no plates, yet %d pixels differ from the plain frame "+
				"(the bands must be invisible without artwork)", n)
		}
		return joinErrs(g.out, errs)
	}

	for _, w := range []struct {
		name       string
		band, body image.Rectangle
	}{
		{"refinery", bands[0], refBody},
		{"manufacturing", bands[1], manBody},
	} {
		// The panel, well inside the body's padding: the reference every
		// "content pixel" probe compares against.
		// Clear of the body's rounded bottom corner: the radius is the
		// window token, so the reference sits inside the straight edge.
		panel := shot.px(w.body.Min.X+theme.Px(theme.RadiusWindow)+6, w.body.Max.Y-4)
		checkBand(w.name, w.band, w.body, panel, shot, was, fail)
	}
	// The rows on the band let it through: their background moves far from
	// the no-plate frame, while a row below the band is byte-identical (the
	// viewport's fill starts at the band's bottom edge only).
	for _, w := range []struct {
		name string
		tbl  *widgets.Table
		band image.Rectangle
	}{
		{"refinery", g.ref.ores, bands[0]},
		{"manufacturing", g.man.recipes, bands[1]},
	} {
		checkHeaderRows(w.name, w.tbl, w.band, shot, now, fail)
	}
	return joinErrs(g.out, errs)
}

// checkHeaderRows pins the plate reading through the table rows on the band:
// a row's background inside the band changes substantially against the
// no-plate frame (the art shows where the viewport used to paint its fill),
// and a row below the band is byte-identical (the fill still covers it
// exactly as before).
func checkHeaderRows(name string, tbl *widgets.Table, band image.Rectangle, shot, was *frame, fail func(string, ...any)) {
	inBand := -1
	for i := range tbl.Rows {
		rr := tbl.RowWidget(i).GetWidget().Rect
		if rr.Empty() {
			continue
		}
		if inBand < 0 && rr.Max.Y <= band.Max.Y {
			inBand = i
		}
	}
	if inBand < 0 {
		fail("%s: no table row sits fully inside the band %v — the band misses the rows", name, band)
		return
	}
	rr := tbl.RowWidget(inBand).GetWidget().Rect
	// The row's top/bottom padding scanlines: background under and over the
	// text, never a glyph. Measured against the release frame: 106/255
	// (refinery) and 37/255 (manufacturing) now; with the fill still painted
	// over the art the same probe moved 9 and 2.
	strongest := 0
	for _, y := range []int{rr.Min.Y + theme.Px(3), rr.Max.Y - theme.Px(2)} {
		for x := rr.Min.X + theme.Px(2); x < rr.Max.X-theme.Px(2); x++ {
			if d := shot.diff(was, x, y); d > strongest {
				strongest = d
			}
		}
	}
	if strongest < 12 {
		fail("%s: row %d background inside the band moved %d/255 from the no-plate frame — the plate is buried again",
			name, inBand, strongest)
	}
	// From the band's bottom edge down the viewport paints the no-plate
	// frame: rows, gaps, border and fill all unchanged. One unit of slack:
	// the viewport's rounded corner (and its 1 px border) is anti-aliased,
	// so where the plate's own last fringe is a fraction of a level the
	// blend can move a single step.
	view := tbl.ScrollForTest().ViewRect()
	// The first rows under the band are the banded fill's own ramp start
	// (the depth ramp begins at the fill line), so they differ from the
	// plain frame's mid-ramp value by a few levels; from there down it is
	// today's fill again.
	for y := band.Max.Y + theme.Px(2); y < view.Max.Y; y++ {
		for x := view.Min.X; x < view.Max.X; x++ {
			if d := shot.diff(was, x, y); d > 1 {
				fail("%s: viewport pixel (%d,%d) below the band moved %d/255 — the banded viewport must keep today's fill there",
					name, x, y, d)
				return
			}
		}
	}
}

// checkBand pins one window's band: full body width, the plate painting at
// the title edge and transparent at the band's bottom edge, with the content
// ink still painted over it.
func checkBand(name string, band, body image.Rectangle, panel color.RGBA, shot, was *frame, fail func(string, ...any)) {
	if band.Dy() != theme.Px(headerBandPx) || band.Dx() != body.Dx() {
		fail("%s band %v is not a %d px strip across its body %v", name, band, theme.Px(headerBandPx), body)
		return
	}
	// The plate's own contribution: only the pixels the plain frame leaves at
	// the panel colour show it undiluted (the body's padding and the gaps
	// between rows — the rows themselves paint 230-alpha fills over it).
	profile := func(from, to int) int {
		strongest := 0
		for y := band.Min.Y + from; y < band.Min.Y+to; y++ {
			for x := band.Min.X; x < band.Max.X; x++ {
				// 12, not 4: the panel-depth ramp puts the band's top fill
				// ~10 above the body's bottom reference pixel, so a tight
				// match would read the whole bare band as content.
				if channelDelta(was.px(x, y), panel) > 12 {
					continue // content ink or a row fill: not the bare band
				}
				if d := shot.diff(was, x, y); d > strongest {
					strongest = d
				}
			}
		}
		return strongest
	}
	// The plate paints in the band but is gone at its bottom edge. The
	// strongest bare-band delta is art-dependent, so the pin is the two
	// endpoints (the fade's linearity is pinned by TestHeaderAlphaFadesDownBand).
	if got := profile(0, band.Dy()); got <= 20 {
		fail("%s band %v: the plate does not paint (strongest bare-band delta %d)", name, band, got)
	}
	// A few units of slack: near-panel antialiased chrome (a row hairline)
	// blends the plate's last 1% at its edge.
	if got := profile(band.Dy()-8, band.Dy()); got > 10 {
		fail("%s band %v: the bottom edge is not transparent (strongest bare-band delta %d)",
			name, band, got)
	}

	// The rows sit ON the band: the plain frame's ink pixels (everything well
	// off the panel colour) survive the plate. A plate painted over the
	// content would recolour nearly all of them.
	ink, buried := 0, 0
	for y := band.Min.Y; y < band.Max.Y; y++ {
		for x := band.Min.X; x < band.Max.X; x++ {
			if channelDelta(was.px(x, y), panel) <= 100 {
				continue // panel, or a fringe that blends the backdrop by nature
			}
			ink++ // a content pixel: an opaque glyph or icon interior
			if shot.diff(was, x, y) > 40 {
				buried++
			}
		}
	}
	if ink < 40 {
		fail("%s band %v has %d content pixels, want the first rows on the plate", name, band, ink)
	} else if buried*100 > ink { // 1% slack for a stray fringe pixel
		fail("%s band %v: %d/%d content pixels recoloured — the plate paints over the rows",
			name, band, buried, ink)
	}
}

// joinErrs folds the collected failures into one error (nil when clean).
func joinErrs(out string, errs []string) error {
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("capture %s: %s", out, errs[0])
}

// frame is one frame's pixels, read back once (per-pixel At needs a GPU
// round-trip each).
type frame struct {
	rgba   []byte
	w, h   int
	stride int
}

// readFrame dumps the image's pixels (one GPU readback for the whole frame).
func readFrame(img *ebiten.Image) *frame {
	b := img.Bounds()
	f := &frame{w: b.Dx(), h: b.Dy(), stride: b.Dx() * 4}
	f.rgba = make([]byte, f.stride*f.h)
	img.ReadPixels(f.rgba)
	return f
}

// px is one pixel.
func (f *frame) px(x, y int) color.RGBA {
	i := y*f.stride + x*4
	return color.RGBA{R: f.rgba[i], G: f.rgba[i+1], B: f.rgba[i+2], A: f.rgba[i+3]}
}

// diff is the largest per-channel difference between the two frames at x, y.
func (f *frame) diff(g *frame, x, y int) int {
	return channelDelta(f.px(x, y), g.px(x, y))
}

// channelDelta is the largest per-channel difference between two colours.
func channelDelta(a, b color.RGBA) int {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return max(d(a.R, b.R), max(d(a.G, b.G), d(a.B, b.B)))
}

// diffsOutside counts sampled pixels outside every rect in keep that changed.
func diffsOutside(a, b *frame, keep []image.Rectangle, tol int) int {
	n := 0
	for y := 0; y < a.h; y += 4 {
		for x := 0; x < a.w; x += 4 {
			p := image.Pt(x, y)
			inside := false
			for _, r := range keep {
				if p.In(r) {
					inside = true
					break
				}
			}
			if !inside && a.diff(b, x, y) > tol {
				n++
			}
		}
	}
	return n
}
