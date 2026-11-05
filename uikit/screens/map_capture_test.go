package screens

import (
	"fmt"
	"testing"

	"github.com/entieve/entieve/internal/capture"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// mapCaptureFrames is how many Update ticks run before the screenshot (the
// anchors and layout settle on the first frames).
const mapCaptureFrames = 3

// TestMapCaptureFrame renders the starmap through a real ebiten game loop
// (pixel readback panics before the game starts) and asserts the kit's
// 28 px lattice: accent lines at ~3.5 % alpha under the gate graph, on the
// map image only. The frame is the war's starmap — world-default (the one
// world that carries the war) with a front selected, so the capture pins
// the control markers, the contested ring, the war key and the card's war
// readout as well as the lattice; world-showcase carries no war.
//
// Skipped unless ENTIEVE_CAPTURE=1 so the regular suite stays windowless.
// The PNG lands in $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
func TestMapCaptureFrame(t *testing.T) {
	env := newWarEnv(t)
	m := NewMapView(env, nil, nil)
	m.sel = "sys-penumbra" // Kvonice: a charter-held front
	c := newChromeUI(env)
	c.AddWindow(m)

	out := capture.Out("map_capture.png")
	var cap *capture.Capture
	cap = &capture.Capture{
		Name: "Entieve starmap capture",
		Update: func(n int) error {
			for _, p := range c.panels {
				p.Sync()
			}
			c.ui.Update()
			if n > mapCaptureFrames {
				cap.Stop()
			}
			return nil
		},
		Draw: func(_ int, img *ebiten.Image) error {
			c.ui.Draw(img)
			return nil
		},
		Verify: func(*ebiten.Image) error {
			// The grid: a scan line inside the graph column crosses the
			// vertical 28 px lines. Each line is the accent at ~3.5 %
			// (alpha 9); anything else on the line (gate edges, node
			// rings) is a different alpha. The graph column is the one
			// zone the lattice covers (the header and the side columns
			// are panel chrome).
			g := m.graph
			if g.Empty() {
				return fmt.Errorf("capture %s: the graph column was never laid out", out)
			}
			// The layer is opaque, so the 3.5 % accent lattice composites
			// onto the BGHard backdrop instead of carrying alpha 9. A grid
			// pixel therefore reads a little brighter than the backdrop
			// (r 10 → ~19) while a gate edge (r ~56) or a node ring (r
			// ~255) stays outside the band.
			step := float64(theme.Px(mapGridStep))
			grid := 0
			y := g.Min.Y + 5
			for x := g.Min.X; x < g.Max.X; x++ {
				r, _, _, _ := m.img.At(x, y).RGBA()
				if r >= 0x0c00 && r <= 0x2800 { // 12..40 in 16-bit channels
					grid++
				}
			}
			want := int(float64(g.Dx()) / step)
			if grid < want-2 {
				return fmt.Errorf("capture %s: map grid: %d lit line samples of %d, want the 28 px lattice", out, grid, want)
			}
			return nil
		},
		Out: out,
	}
	cap.Run(t)
}
