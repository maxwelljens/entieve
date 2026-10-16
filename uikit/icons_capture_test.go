package uikit

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
)

// ebitenLoopRan records that this test process already drove an ebiten
// game loop: ebiten refuses to build images once RunGame has finished
// ("NewImage cannot be called after RunGame finishes"), so only one
// capture can construct widgets per process. Each capture test calls
// skipIfLoopRan before building anything, so running the whole capture
// set skips the extras instead of panicking.
var ebitenLoopRan bool

// skipIfLoopRan skips the test when this process already ran a loop.
func skipIfLoopRan(t *testing.T) {
	t.Helper()
	if ebitenLoopRan {
		t.Skip("one ebiten game loop per test process: run this capture on its own")
	}
}

// runCaptureLoop drives one capture game through ebiten, records that the
// loop ran and skips when the window system is unavailable.
func runCaptureLoop(t *testing.T, title string, g ebiten.Game) {
	t.Helper()
	ebiten.SetWindowTitle(title)
	ebiten.SetWindowSize(1280, 720)
	err := ebiten.RunGame(g)
	ebitenLoopRan = true
	if err != nil {
		t.Skipf("cannot run capture game loop: %v", err)
	}
}

// iconsCaptureFrames is how many full draws run before the screenshot is
// taken (anchors and layout settle on the first frames).
const iconsCaptureFrames = 3

// TestIconsCaptureFrame renders the three icon-bearing core surfaces —
// overview (kind column), Selected Item (kind glyph + action buttons) and
// the module rack (class tiles) — through a real ebiten game loop, checks
// that the glyphs actually landed on pixels (ink in each box, the tile
// glyph centred in its tile) and screenshots the frame.
//
// Skipped unless ENTIEVE_CAPTURE=1 so the regular suite stays windowless.
// The PNG lands in $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
func TestIconsCaptureFrame(t *testing.T) {
	if os.Getenv("ENTIEVE_CAPTURE") == "" {
		t.Skip("set ENTIEVE_CAPTURE=1 to render a capture frame")
	}
	skipIfLoopRan(t)
	outDir := os.Getenv("ENTIEVE_CAPTURE_DIR")
	if outDir == "" {
		outDir = "/tmp/opencode"
	}

	k, _ := newTestKit(t)
	// Select a warpable landmark so the Selected Item header shows its
	// kind glyph and the action buttons are enabled.
	for _, info := range k.Sim.Grid() {
		if info.CanWarp {
			k.Select(info.Entity)
			break
		}
	}
	r := NewRack(k.Env())
	k.AddContainer(r.Container(), widget.AnchorLayoutData{
		HorizontalPosition: widget.AnchorLayoutPositionEnd,
		VerticalPosition:   widget.AnchorLayoutPositionEnd,
		Padding:            &widget.Insets{Right: 10, Bottom: 10},
	})

	g := &iconsCaptureGame{k: k, rack: r, out: filepath.Join(outDir, "icons_capture.png")}
	runCaptureLoop(t, "Entieve icons capture", g)
	if g.err != nil {
		t.Fatal(g.err)
	}
	if !g.saved {
		t.Fatal("capture never rendered")
	}
	t.Logf("capture written to %s", g.out)
}

// iconsCaptureGame drives kit + rack for a few frames, then screenshots
// and verifies the drawn glyphs.
type iconsCaptureGame struct {
	k      *Kit
	rack   *Rack
	out    string
	frames int
	last   *ebiten.Image
	saved  bool
	err    error
}

func (g *iconsCaptureGame) Update() error {
	if g.saved || g.err != nil {
		return ebiten.Termination
	}
	g.k.Sync()
	g.rack.Sync()
	if err := g.k.Update(); err != nil {
		g.err = err
		return ebiten.Termination
	}
	g.frames++
	if g.frames > iconsCaptureFrames && g.last != nil {
		g.verify(g.last)
		return ebiten.Termination
	}
	return nil
}

func (g *iconsCaptureGame) Draw(screen *ebiten.Image) {
	img := ebiten.NewImage(1280, 720)
	g.k.Draw(img)
	screen.DrawImage(img, nil)
	g.last = img
}

func (g *iconsCaptureGame) Layout(w, h int) (int, int) { return 1280, 720 }

// inkBounds returns the bounding box of bright pixels inside box (and how
// many there are) — the drawn silhouette's on-screen footprint.
func inkBounds(img *ebiten.Image, box image.Rectangle) (image.Rectangle, int) {
	return inkBoundsFunc(img, box, func(x, y int) bool {
		r, gch, b, a := img.At(x, y).RGBA()
		return a != 0 && (r>>8 > 90 || gch>>8 > 90 || b>>8 > 90)
	})
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
func inkIn(img *ebiten.Image, box image.Rectangle) int {
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

// glyphBounds returns the on-screen footprint of a rack tile's glyph: the
// bounds of every pixel inside the tile that differs from its flat body
// colour (sampled at an inner corner the centred glyph never reaches).
func glyphBounds(img *ebiten.Image, tile image.Rectangle) (image.Rectangle, int) {
	refR, refG, refB, _ := img.At(tile.Min.X+4, tile.Min.Y+4).RGBA()
	close := func(c interface {
		RGBA() (uint32, uint32, uint32, uint32)
	}) bool {
		r, g, b, _ := c.RGBA()
		return absInt(int(r>>8)-int(refR>>8)) <= 14 &&
			absInt(int(g>>8)-int(refG>>8)) <= 14 &&
			absInt(int(b>>8)-int(refB>>8)) <= 14
	}
	return inkBoundsFunc(img, tile.Inset(3), func(x, y int) bool {
		return !close(img.At(x, y))
	})
}

// verify screenshots the frame and asserts the drawn glyphs. Every failure
// is collected so one run reports all of them.
func (g *iconsCaptureGame) verify(img *ebiten.Image) {
	var errs []string
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Sprintf(format, args...))
	}

	// --- 1. Overview kind column: the first row's icon box has ink.
	tbl := g.k.ov.tbl
	if len(tbl.cells) == 0 || tbl.cells[0][2] == nil {
		fail("overview has no kind icon cells (rows=%d)", len(tbl.cells))
	} else {
		box := tbl.cells[0][2].box.Widget().GetWidget().Rect
		if box.Dx() != IconInline {
			fail("overview kind icon box = %v, want %d px square", box, IconInline)
		}
		if ink := inkIn(img, box); ink < 8 {
			fail("overview kind icon box %v has %d ink pixels, want ≥ 8", box, ink)
		}
		label := tbl.texts[0][2].GetWidget().Rect
		if box.Overlaps(label) {
			fail("overview icon %v overlaps its label %v", box, label)
		}
	}

	// --- 2. Selected Item header: the kind glyph beside the name.
	if g.k.sp.kindIcon.Name() == "" {
		fail("selected header shows no kind glyph (selected=%q)", g.k.sp.name.Label)
	} else if box := g.k.sp.kindIcon.Widget().GetWidget().Rect; inkIn(img, box) < 8 {
		fail("selected header glyph box %v has too little ink", box)
	}

	// --- 3. Action buttons: every enabled action button carries its 20 px
	// glyph (ink over the button face, inside the button rect).
	for _, b := range []*widget.Button{
		g.k.sp.approach, g.k.sp.warp, g.k.sp.orbit, g.k.sp.stop,
		g.k.sp.lock, g.k.sp.dock, g.k.sp.jump, g.k.sp.loot, g.k.sp.mine,
	} {
		if b.GraphicImage() == nil || b.GraphicImage().Idle == nil {
			fail("%q button has no glyph", b.Text().Label)
			continue
		}
		r := b.GetWidget().Rect
		if ink := inkIn(img, r); ink < 20 {
			fail("%q button %v drew only %d bright pixels", b.Text().Label, r, ink)
		}
	}

	// --- 4. Rack tiles: a fitted cell's glyph sits centred in its tile.
	// The tile body is flat, so the glyph is whatever differs from the
	// body colour sampled in a corner the glyph can never reach (the
	// glyph is 20 px centred in a 50×40 tile): disabled tiles draw at
	// 40 % and would fail an absolute brightness threshold.
	// The tile's inner ink box must be centred on the tile to within a
	// couple of pixels — a glyph hung off one side (the graphic-row
	// centering defect) reads 5 px off and fails here.
	checked := 0
	for row := range g.rack.cells {
		for _, cell := range g.rack.cells[row] {
			if cell.icon == "" || cell.root.GetWidget().GetVisibility() != widget.Visibility_Show {
				continue
			}
			tile := cell.btn.GetWidget().Rect
			bx, n := glyphBounds(img, tile)
			if n < 8 {
				fail("rack tile %v has %d ink pixels for %s, want a drawn glyph",
					tile, n, cell.icon)
				checked++
				continue
			}
			if dx := (bx.Min.X+bx.Max.X)/2 - (tile.Min.X+tile.Max.X)/2; absInt(dx) > 4 {
				fail("rack tile %v glyph centre is %d px off (icon %s)", tile, dx, cell.icon)
			}
			if dy := (bx.Min.Y+bx.Max.Y)/2 - (tile.Min.Y+tile.Max.Y)/2; absInt(dy) > 4 {
				fail("rack tile %v glyph is %d px off vertically (icon %s)", tile, dy, cell.icon)
			}
			checked++
		}
	}
	if checked == 0 {
		fail("no fitted rack cells to check")
	}

	// Screenshot first: evidence is useful even when an assert failed.
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
	if len(errs) > 0 {
		g.err = fmt.Errorf("capture %s: %s", g.out, strings.Join(errs, "; "))
	}
}

// absInt is |v| (the capture checks only).
func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
