package game

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/internal/capture"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestStackPickerCapture: the stack picker's column over a stacked fixture —
// three static entities (a gate, a station and an asteroid: three bracket
// tints) parked on one point, the cursor resting on them through the dwell.
// Pixel readback needs a running ebiten loop, so this is the gated capture
// (ENTIEVE_CAPTURE=1, one loop per process — run it on its own); the PNG
// lands in $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
//
// Method: park the fixture, take one reference frame before the dwell opens
// the column, pump the dwell, then verify the last frame: the column's band
// (found through Kit.StackOwns, not a hardcoded rect) paints over the world
// and every icon cell changed against the reference.
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
	(&capture.Capture{
		Name:   "Entieve stack picker capture",
		Frames: dwellFrames + 4, // the dwell, then a few frames of the open column
		Update: func(int) error { return g.Update() },
		Draw: func(n int, dst *ebiten.Image) error {
			g.Draw(dst)
			if n == 1 { // before the dwell: the world with no column
				plain = ebiten.NewImage(1280, 720)
				plain.DrawImage(dst, nil)
			}
			band = stackBand(g, atX, atY)
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
			// Three icons: 20 px cells, 4 px gaps and 4 px padding
			// (uikit/screens stack.go) at the design scale.
			want := image.Pt(28, 76)
			if band.Dx() != want.X || band.Dy() != want.Y {
				return fmt.Errorf("capture %s: column band %v, want %dx%d", out, band, want.X, want.Y)
			}
			for i := 0; i < 3; i++ {
				if !cellChanged(dst, plain, band, i) {
					return fmt.Errorf("capture %s: icon %d painted nothing in %v", out, i, band)
				}
			}
			return nil
		},
		Out: out,
	}).Run(t)
	t.Logf("capture written to %s (band %v at cursor %v,%v)", out, band, atX, atY)
}

// stackBand is the rect the open column owns, found by probing StackOwns.
func stackBand(g *Game, x, y float64) image.Rectangle {
	var band image.Rectangle
	for py := int(y) - 120; py <= int(y)+120; py++ {
		for px := int(x); px <= int(x)+120; px++ {
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

// cellChanged reports whether icon i's cell paints anything new: a small
// probe grid inside the cell must differ from the reference frame.
func cellChanged(dst, plain *ebiten.Image, band image.Rectangle, i int) bool {
	top := band.Min.Y + band.Dy()*i/3
	bot := band.Min.Y + band.Dy()*(i+1)/3
	for py := top + 3; py < bot-3; py++ {
		for px := band.Min.X + 3; px < band.Max.X-3; px++ {
			if !sameColor(dst.At(px, py), plain.At(px, py)) {
				return true
			}
		}
	}
	return false
}

// sameColor reports two sampled pixels equal on every channel.
func sameColor(a, b color.Color) bool {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	return ar == br && ag == bg && ab == bb
}
