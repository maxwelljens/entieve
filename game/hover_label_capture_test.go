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

// TestHoverLabelCapture: the world's only entity label follows the cursor —
// drawn for the entity it rests on, absent otherwise, at the near framing and
// at the far system view alike (zoom no longer gates it). Pixel readback needs
// a running ebiten loop, so this is the gated capture (ENTIEVE_CAPTURE=1, one
// loop per process — run it on its own); the PNG lands in
// $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
//
// Method: park a station at a clear world point, then take the band below it
// twice per framing — cursor away from every entity, then on the station. The
// label's ink (the name and distance colours) may only appear in the on frame,
// and that must hold at both zooms.
func TestHoverLabelCapture(t *testing.T) {
	mm := useLoopMouse(t)
	g := newShowcaseGame(t, mm.pointer())
	pump(t, g, 1280, 720) // warm the layout up before the capture frames
	sc := g.space
	ply, _ := g.s.Transforms.Get(g.s.Player())

	var station ecs.Entity
	for _, inf := range g.s.Grid() {
		if inf.Kind == sim.KindStation {
			station = inf.Entity
			break
		}
	}
	if station.IsZero() {
		t.Fatal("fixture: no station in the showcase grid")
	}
	pos := ply.Pos.Add(core.V(0, 60/zoomBase)) // 60 px below the player: clear of its pick radius
	parkOn(g, station, pos)
	onX, onY := panUnderCursor(g, pos)
	offX, offY := clearPoint(g, onX, onY+320)
	if g.overUI(onX, onY) {
		t.Fatalf("fixture: the cursor point (%v,%v) sits on a panel", onX, onY)
	}

	band := image.Rect(int(onX)-200, int(onY)+12, int(onX)+200, int(onY)+130)
	// One frame's Draw sets up the next frame's Update (the pointer and the
	// camera are read there), so each state is set the frame before it is
	// snapshotted.
	var shots [6]*ebiten.Image
	var hovered [6]ecs.Entity
	out := capture.Out("hover_label_capture.png")
	frame := 0
	(&capture.Capture{
		Name:   "Entieve hover label capture",
		Frames: len(shots),
		Update: func(int) error { return g.Update() },
		Draw: func(n int, dst *ebiten.Image) error {
			switch n {
			case 1: // near framing, cursor off
				mm.moveTo(int(offX), int(offY))
			case 2: // near framing, cursor on: the label draws (and is the PNG)
				mm.moveTo(int(onX), int(onY))
			case 3: // the far system view, cursor off
				sc.zoom = zoomMin
				onX, onY = panUnderCursor(g, pos)
				offX, offY = clearPoint(g, onX, onY+320)
				mm.moveTo(int(offX), int(offY))
			case 4: // far framing, cursor on
				mm.moveTo(int(onX), int(onY))
			case 5: // back to the near framing, cursor on: the evidence PNG
				sc.zoom = 1
				panUnderCursor(g, pos)
				mm.moveTo(int(onX), int(onY))
			}
			g.Draw(dst)
			shots[n-1] = ebiten.NewImage(1280, 720)
			shots[n-1].DrawImage(dst, nil)
			hovered[n-1] = sc.hover
			frame = n
			return nil
		},
		Verify: func(dst *ebiten.Image) error {
			if frame != len(shots) || shots[2] == nil || shots[4] == nil {
				return fmt.Errorf("capture %s: only %d of %d frames ran", out, frame, len(shots))
			}
			if hovered[2] != station || hovered[5] != station {
				return fmt.Errorf("capture %s: the cursor on the station resolved %v (near) and %v (again), want %v",
					out, hovered[2], hovered[5], station)
			}
			if hovered[4].IsZero() {
				return fmt.Errorf("capture %s: the far system view resolved no entity under the cursor", out)
			}
			for _, c := range []struct {
				name   string
				away   *ebiten.Image
				on     *ebiten.Image
				zoomed string
			}{
				{"near", shots[1], shots[2], "the near framing"},
				{"far", shots[3], shots[4], "the far system view"},
			} {
				away, on := labelInk(c.away, band), labelInk(c.on, band)
				if on < hoverLabelInkMin {
					return fmt.Errorf("capture %s: the hovered entity drew %d label pixels in %v at %s, want %d",
						out, on, band, c.zoomed, hoverLabelInkMin)
				}
				if away >= hoverLabelInkMin {
					return fmt.Errorf("capture %s: %d label pixels drew with the cursor off the entity at %s",
						out, away, c.zoomed)
				}
				if on == away {
					return fmt.Errorf("capture %s: the label band did not change at %s", out, c.zoomed)
				}
			}
			return nil
		},
		Out: out,
	}).Run(t)
	t.Logf("capture written to %s (label band %v at %v,%v)", out, band, onX, onY)
}

// hoverLabelInkMin is how many label-coloured pixels a drawn name and distance
// pair leaves in the band: two text lines make hundreds, a stray star a handful.
const hoverLabelInkMin = 25

// labelInk counts the pixels in b close to the two colours the overlay's name
// and distance labels use.
func labelInk(img *ebiten.Image, b image.Rectangle) int {
	b = b.Intersect(img.Bounds())
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if nearLabelInk(img.At(x, y)) {
				n++
			}
		}
	}
	return n
}

// nearLabelInk reports whether c is one of the label inks, within the
// anti-aliasing tolerance the text needs.
func nearLabelInk(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	for _, want := range [...][3]uint32{{216, 222, 233}, {135, 146, 163}} {
		if absDiff(r>>8, want[0]) <= 10 && absDiff(g>>8, want[1]) <= 10 && absDiff(b>>8, want[2]) <= 10 {
			return true
		}
	}
	return false
}

// absDiff is |a - b| on 8-bit channels.
func absDiff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}

// clearPoint walks a short deterministic cross around (x, y) until it finds a
// point off every panel where no perceivable entity sits within the pick
// radius, so the capture's "cursor off the entity" frame really resolves
// nothing.
func clearPoint(g *Game, x, y float64) (float64, float64) {
	sc := g.space
	cam := sc.Camera(1280, 720)
	for d := 0.0; d < 600; d += 20 {
		for _, p := range [][2]float64{{x + d, y}, {x - d, y}, {x, y + d}, {x, y - d}} {
			if g.overUI(p[0], p[1]) {
				continue
			}
			if _, hit := sc.pick(cam, p[0], p[1]); !hit {
				return p[0], p[1]
			}
		}
	}
	return x, y
}
