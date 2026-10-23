package theme

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// The kit's signature mark: three hollow ring dots at a title bar's left,
// then the icon and title. Rings only — nothing is filled, and the ink is
// the bar's own hairline role, so the mark is pure form.
const (
	TitleDot     = 9 // ring outer diameter (design px)
	TitleDotGap  = 7 // air between rings
	TitleDotsGap = SpMd
	// TitleDotsW is the dot block's width: three rings and the two gaps.
	TitleDotsW = 3*TitleDot + 2*TitleDotGap
)

// TitleDots renders the three-ring block at the current scale.
func TitleDots(p Palette) *ebiten.Image {
	return ebiten.NewImageFromImage(titleDotsImage(p))
}

// titleDotsImage is the ring block's raster (split out so tests can read
// its pixels without a game loop).
func titleDotsImage(p Palette) *image.NRGBA {
	d := Px(TitleDot)
	gap := Px(TitleDotGap)
	stroke := max(Px(1), 1)
	w, h := Px(TitleDotsW), d
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	ink := NRGBA(p.Border, 200)
	outer := float64(d) / 2
	band := float64(stroke) / 2
	for i := 0; i < 3; i++ {
		cx := float64(i*(d+gap)) + outer
		cy := float64(h) / 2
		for y := 0; y < h; y++ {
			for x := i * (d + gap); x < i*(d+gap)+d && x < w; x++ {
				dist := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
				// 1-px ring just inside the outer edge.
				cov := clamp01(band + 0.5 - math.Abs(dist-(outer-band)))
				if cov <= 0 {
					continue
				}
				img.SetNRGBA(x, y, color.NRGBA{
					R: ink.R, G: ink.G, B: ink.B,
					A: uint8(math.Round(float64(ink.A) * cov)),
				})
			}
		}
	}
	return img
}
