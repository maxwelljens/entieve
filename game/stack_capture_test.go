package game

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/internal/capture"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestStackPickerCapture: the stack picker's column over a stacked fixture —
// three static entities (a gate, a station and an asteroid: three kinds) parked
// on one point, the cursor resting on them through the dwell. Pixel readback
// needs a running ebiten loop, so this is the gated capture (ENTIEVE_CAPTURE=1,
// one loop per process — run it on its own); the PNG lands in
// $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
//
// Method: park the fixture, take one reference frame before the dwell opens
// the column, pump the dwell, then verify the last frame: every entry's art
// region is byte-identical to the grid's own body art at the same zoom (no
// frame, no scaled miniature), the name sits right of the art, nothing paints
// behind the column (the world shows between the entries), and the three
// listed entities have stopped drawing in space — the column replaces them.
func TestStackPickerCapture(t *testing.T) {
	mm := useLoopMouse(t)
	g := newShowcaseGame(t, mm.pointer())
	pump(t, g, 1280, 720) // warm the layout up before the capture frames
	ply, _ := g.s.Transforms.Get(g.s.Player())

	var gate, station, rock ecs.Entity
	for _, inf := range g.s.Grid() {
		switch inf.Kind {
		case sim.KindGate:
			gate = inf.Entity
		case sim.KindStation:
			station = inf.Entity
		case sim.KindAsteroid:
			rock = inf.Entity
		}
	}
	if gate.IsZero() || station.IsZero() || rock.IsZero() {
		t.Fatalf("fixture: gate %v, station %v, rock %v", gate, station, rock)
	}
	pos := ply.Pos.Add(core.V(0, 24/zoomBase))
	for _, e := range []ecs.Entity{gate, station, rock} {
		parkOn(g, e, pos)
	}
	atX, atY := panUnderCursor(g, pos)
	if g.overUI(atX, atY) {
		t.Fatalf("fixture: the cursor (%v, %v) sits on a panel", atX, atY)
	}
	mm.moveTo(int(atX), int(atY))

	var plain *ebiten.Image
	var band image.Rectangle
	out := capture.Out("stack_picker_capture.png")
	frames := dwellFrames + 4 // the dwell, then a few frames of the open column
	(&capture.Capture{
		Name:   "Entieve stack picker capture",
		Frames: frames,
		Update: func(int) error { return g.Update() },
		Draw: func(n int, dst *ebiten.Image) error {
			g.Draw(dst)
			if n == 1 { // before the dwell: the world with no column
				plain = ebiten.NewImage(1280, 720)
				plain.DrawImage(dst, nil)
			}
			// The probe walks the screen, so it runs once: a per-frame scan
			// would spend the frame budget this capture is evidence for.
			if n == frames {
				band = stackBand(g, atX, atY)
			}
			return nil
		},
		Verify: func(dst *ebiten.Image) error {
			if !g.kit.StackShown() {
				return fmt.Errorf("capture %s: the column never opened over the fixture", out)
			}
			if plain == nil {
				return fmt.Errorf("capture %s: no reference frame", out)
			}
			if band.Empty() {
				return fmt.Errorf("capture %s: the open column owns no band around (%v, %v)", out, atX, atY)
			}
			cam := g.space.Camera(1280, 720)
			cells := g.space.pickAll(cam, atX, atY)
			if len(cells) < 3 {
				return fmt.Errorf("capture %s: cluster resolved %d entries, want 3", out, len(cells))
			}

			// The band is exactly the entries: no padding, no panel.
			gap := theme.Px(theme.SpSm)
			w, h := 0, -gap
			imgs := make([]*ebiten.Image, 3)
			for i := 0; i < 3; i++ {
				imgs[i] = g.kit.StackCellImageForTest(i)
				if imgs[i] == nil {
					return fmt.Errorf("capture %s: entry %d has no image", out, i)
				}
				b := imgs[i].Bounds()
				w = max(w, b.Dx())
				h += b.Dy() + gap
			}
			if band.Dx() != w || band.Dy() != h {
				return fmt.Errorf("capture %s: column band %v for entries %dx%d, want the entries' own size",
					out, band, w, h)
			}

			for i := 0; i < 3; i++ {
				img := imgs[i]
				art := g.ren.EntityArtSize(g.s, cells[i], cam.Zoom, math.Max(1280, 720))
				b := img.Bounds()
				if b.Dx() <= art {
					return fmt.Errorf("capture %s: entry %d is %d px wide for %d px of art: no name beside it",
						out, i, b.Dx(), art)
				}
				// The art is the grid's own, byte for byte: the same body
				// pass drawn at the entry's own camera into a same-size
				// image, so any frame, panel or glyph ink would show.
				ref := ebiten.NewImage(art, b.Dy())
				g.ren.DrawEntityBody(ref, g.s, cells[i], render.Camera{
					Center: cells[i].Pos, Zoom: cam.Zoom,
					ScreenW: float64(art), ScreenH: float64(b.Dy()), CapDim: math.Max(1280, 720),
				})
				artBox := image.Rect(0, 0, art, b.Dy())
				if n, at := diffPixels(img, artBox, ref, image.Pt(0, 0), 0); n != 0 {
					return fmt.Errorf("capture %s: entry %d (%s) paints more than the body art at its own camera "+
						"(%d px differ, first %v)", out, i, cells[i].Kind, n, at)
				}

				// And that box is the grid's size, at the grid's zoom: the same
				// body drawn with the world camera (its screen, its caps, the
				// entity at the screen centre) is the same picture. ebiten's
				// vector antialiasing is not exactly translation-invariant, so
				// a handful of boundary pixels may differ.
				world := ebiten.NewImage(1280, 720)
				g.ren.DrawEntityBody(world, g.s, cells[i], render.Camera{
					Center: cells[i].Pos, Zoom: cam.Zoom, ScreenW: 1280, ScreenH: 720,
				})
				// ebiten samples a scaled additive sprite with ties broken by
				// the destination, so a few boundary pixels bend: 5 in this
				// fixture, never a frame's worth.
				if n, at := diffPixels(img, artBox, world, image.Pt(640-art/2, 360-b.Dy()/2), 64); n > artTiePx {
					return fmt.Errorf("capture %s: entry %d (%s) is not the grid's art at zoom %v "+
						"(art box %d px, %d px differ, first %v)", out, i, cells[i].Kind, cam.Zoom, art, n, at)
				}
				if n := inkOf(img, image.Rect(art, 0, b.Dx(), b.Dy()), theme.DefaultPalette().Text, 24); n == 0 {
					return fmt.Errorf("capture %s: entry %d draws no name right of its %d px of art", out, i, art)
				}
			}

			// No panel behind the column: the band is exactly the entries
			// (the check above), and entry 0's art box corner is empty, so
			// whatever the world draws there shows through untouched.
			for y := 0; y < 8; y++ {
				for x := 0; x < 8; x++ {
					if _, _, _, a := imgs[0].At(x, y).RGBA(); a != 0 {
						return fmt.Errorf("capture %s: entry 0's corner (%d,%d) is not empty", out, x, y)
					}
				}
			}

			// The listed entities left the grid: their bracket ink is in the
			// reference frame and gone from the last one.
			left := image.Rect(int(atX)-70, int(atY)-70, int(atX)+12, int(atY)+70)
			for i := 0; i < 3; i++ {
				col := render.BracketColor(cells[i].Kind, cells[i].Hostile)
				// A bracket's corner arms are dozens of pixels; the grid may
				// keep a stray background pixel within the tolerance.
				if n := inkOf(plain, left, col, 16); n < bracketInkMin {
					return fmt.Errorf("capture %s: the reference frame draws no %v bracket for entry %d (%d px)",
						out, col, i, n)
				}
				if n := inkOf(dst, left, col, 16); n >= bracketInkMin {
					return fmt.Errorf("capture %s: entry %d (%s) still draws %d px of its %v bracket in the world",
						out, i, cells[i].Kind, n, col)
				}
			}
			return nil
		},
		Out: out,
	}).Run(t)
	t.Logf("capture written to %s (band %v at cursor %v,%v)", out, band, atX, atY)
}

// bracketInkMin is the ink a bracket's corner arms leave in the probe box;
// a few stray background pixels stay well under it.
const bracketInkMin = 8

// artTiePx is how many pixels a scaled additive sprite may lose to ebiten's
// destination-dependent sampling ties; a wrong zoom or cap moves thousands.
const artTiePx = 32

// stackBand is the rect the open column owns, found by probing StackOwns.
func stackBand(g *Game, x, y float64) image.Rectangle {
	var band image.Rectangle
	for py := int(y) - 400; py <= int(y)+400; py++ {
		for px := int(x) - 100; px <= int(x)+1000; px++ {
			if !g.kit.StackOwns(float64(px), float64(py)) {
				continue
			}
			p := image.Pt(px, py)
			if band.Empty() {
				band = image.Rect(px, py, px+1, py+1)
				continue
			}
			band = band.Union(image.Rect(p.X, p.Y, p.X+1, p.Y+1))
		}
	}
	return band
}

// diffPixels counts the pixels where a's rect r differs from b's rect at
// origin o by more than slack on any channel, and returns the first one (in
// a's coordinates).
func diffPixels(a *ebiten.Image, r image.Rectangle, b *ebiten.Image, o image.Point, slack uint32) (int, image.Point) {
	n, first := 0, image.Point{}
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			if !nearColor(a.At(r.Min.X+x, r.Min.Y+y), b.At(o.X+x, o.Y+y), slack) {
				if n == 0 {
					first = image.Pt(r.Min.X+x, r.Min.Y+y)
				}
				n++
			}
		}
	}
	return n, first
}

// nearColor reports whether two sampled pixels agree on every channel within
// slack (0 = exactly equal).
func nearColor(a, b color.Color, slack uint32) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return absDiff(ar>>8, br>>8) <= slack && absDiff(ag>>8, bg>>8) <= slack &&
		absDiff(ab>>8, bb>>8) <= slack && absDiff(aa>>8, ba>>8) <= slack
}

// inkOf counts the pixels of r within tol of col on every channel.
func inkOf(img *ebiten.Image, r image.Rectangle, col color.RGBA, tol uint32) int {
	r = r.Intersect(img.Bounds())
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			cr, cg, cb, ca := img.At(x, y).RGBA()
			if absDiff(cr>>8, uint32(col.R)) <= tol && absDiff(cg>>8, uint32(col.G)) <= tol &&
				absDiff(cb>>8, uint32(col.B)) <= tol && absDiff(ca>>8, uint32(col.A)) <= tol {
				n++
			}
		}
	}
	return n
}
