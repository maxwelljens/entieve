package render

import (
	"image/color"
	"math"
	"testing"

	"github.com/entieve/entieve/internal/capture"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// TestFmtDistBands pins the world-label distance formatter — the duplicated
// twin of theme.FormatDistance: meters under 2 km, 1-decimal km through
// 99,999 km, "<0.01 AU" below 0.01 AU, 2-decimal AU beyond.
func TestFmtDistBands(t *testing.T) {
	for m, want := range map[float64]string{
		42: "42 m", 2000: "2.0 km", 9.9999e7: "99,999.0 km",
		1e8: "<0.01 AU", 1.496e9: "0.01 AU", 3e11: "2.01 AU",
	} {
		if got := fmtDist(m); got != want {
			t.Errorf("fmtDist(%v) = %q, want %q", m, got, want)
		}
	}
}

// TestColorScalePremultiplies pins the straight-alpha → premultiplied
// conversion at the seam: a straight (255,0,0,55) must reach the compositor
// as (55,0,0,55), not (255,0,0,55). Opaque colors are unchanged. Pure
// arithmetic — the headless suite cannot read pixels back.
func TestColorScalePremultiplies(t *testing.T) {
	cases := []struct {
		name string
		in   color.RGBA // straight alpha (house convention)
		want color.RGBA // premultiplied, what the GPU must receive
	}{
		{"dim red", color.RGBA{255, 0, 0, 55}, color.RGBA{55, 0, 0, 55}},
		{"dim shield", color.RGBA{111, 168, 255, 55}, color.RGBA{24, 36, 55, 55}},
		{"half grey", color.RGBA{200, 200, 200, 128}, color.RGBA{100, 100, 100, 128}},
		{"opaque unchanged", color.RGBA{255, 180, 84, 255}, color.RGBA{255, 180, 84, 255}},
	}
	for _, c := range cases {
		r, g, b, a := premul(c.in)
		got := color.RGBA{unorm(r), unorm(g), unorm(b), unorm(a)}
		if !near(got, c.want, 1) {
			t.Errorf("%s: compositor receives %v, want %v", c.name, got, c.want)
		}
	}
}

// TestAlphaCompositeGroundTruth renders a real offscreen frame and checks
// exact src-over premultiplied math: straight red at α=55 over opaque blue
// must land on (55, 0, 200, 255) — out = src·α + dst·(1−α), α = 55/255.
// Before the premultiply fix this measured ~(255, 0, 55, 255) — nearly
// opaque red — the exact "dimming does nothing" bug.
//
// Pixel readback requires a running ebiten loop, so this is gated behind
// ENTIEVE_CAPTURE=1 like the frame captures (needs a display).
func TestAlphaCompositeGroundTruth(t *testing.T) {
	got := color.RGBA{}
	(&capture.Capture{
		Name:   "Entieve alpha composite probe",
		Frames: 1,
		Draw: func(_ int, _ *ebiten.Image) error {
			dst := ebiten.NewImage(40, 40)
			dst.Fill(color.RGBA{0, 0, 255, 255})

			var path vector.Path
			path.MoveTo(10, 10)
			path.LineTo(30, 10)
			path.LineTo(30, 30)
			path.LineTo(10, 30)
			path.Close()
			vector.FillPath(dst, &path, nil, &vector.DrawPathOptions{
				AntiAlias:  true,
				ColorScale: colorScale(color.RGBA{255, 0, 0, 55}),
			})

			got = dst.At(20, 20).(color.RGBA)
			return nil
		},
	}).Run(t)
	a := 55.0 / 255.0
	want := color.RGBA{
		R: uint8(math.Round(255*a + 0*(1-a))),
		G: 0,
		B: uint8(math.Round(0 + 255*(1-a))),
		A: 255,
	}
	if !near(got, want, 2) {
		t.Fatalf("composite = %v, want %v (out = src·a + dst·(1−a), a=55/255)", got, want)
	}
}

// unorm maps a 0..1 component back to 8-bit.
func unorm(v float32) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 255
	}
	return uint8(math.Round(float64(v) * 255))
}

// near reports whether two premultiplied pixels agree within tol per channel.
func near(got, want color.RGBA, tol uint8) bool {
	d := func(a, b uint8) uint8 {
		if a > b {
			return a - b
		}
		return b - a
	}
	return d(got.R, want.R) <= tol && d(got.G, want.G) <= tol &&
		d(got.B, want.B) <= tol && d(got.A, want.A) <= tol
}
