package game

import (
	"image"
	"testing"

	uikit "github.com/entieve/entieve/uikit/screens"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestCursorShapePresentsOnEveryDraw pins the draw/update split ebiten
// really runs: Draw once per display frame, Update 0..N times. A
// stationary pointer must present the claimed shape on EVERY draw, not
// just the first draw after an input pass — the seam claims live on the
// Update side, so a draw-only frame must repeat the last decision
// instead of falling back to the arrow.
func TestCursorShapePresentsOnEveryDraw(t *testing.T) {
	cases := []struct {
		name string
		open func(g *Game)
		seat func(t *testing.T, g *Game) image.Point
		want ebiten.CursorShapeType
	}{
		{"inventory vertical seam", func(g *Game) { g.host.SetOpen(idInventory, true) },
			func(t *testing.T, g *Game) image.Point {
				r := g.host.Slot(string(idInventory)).Splits()[0].Split.Container().Children()[1].GetWidget().Rect
				return image.Pt(r.Min.X+2, r.Min.Y)
			}, ebiten.CursorShapeNSResize},
		{"market horizontal seam", func(g *Game) { g.host.SetOpen(idMarket, true) },
			func(t *testing.T, g *Game) image.Point {
				r := g.host.Slot(string(idMarket)).Splits()[0].Split.Container().Children()[1].GetWidget().Rect
				return image.Pt(r.Min.X, r.Min.Y+r.Dy()/2)
			}, ebiten.CursorShapeEWResize},
		{"window edge grip", func(g *Game) { g.host.SetOpen(idInventory, true) },
			func(t *testing.T, g *Game) image.Point {
				r := bodyOf[*uikit.Inventory](g.host, idInventory).Win().GetContainer().GetWidget().Rect
				return image.Pt(r.Min.X+2, (r.Min.Y+r.Max.Y)/2)
			}, ebiten.CursorShapeEWResize},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			m := useLoopMouse(t)
			g := newShowcaseGame(t, m.pointer())
			g.applySize(1280, 720)
			tc.open(g)
			g.syncMounts()
			img := pump(t, g, 1280, 720)
			pump(t, g, 1280, 720)

			at := tc.seat(t, g)
			m.moveTo(at.X, at.Y)
			m.left = false

			// Every draw must present want, including the draws ebiten runs
			// with zero Update ticks between them (the frame's apply
			// repeats the last decision instead of re-sampling).
			draw := func(label string) {
				m.shape = ebiten.CursorShapeDefault
				g.Draw(img)
				if got := m.shape; got != tc.want {
					t.Errorf("%s: presented %v, want %v (a draw without an Update tick dropped the claim)",
						label, got, tc.want)
				}
			}
			for i := 0; i < 8; i++ {
				if err := g.Update(); err != nil {
					t.Fatalf("Update: %v", err)
				}
				draw("tick+draw")
				draw("draw only")
			}
		})
	}
}
