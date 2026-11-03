package screens

import (
	"image"
	"math"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/uikit/hulls"
	"github.com/hajimehoshi/ebiten/v2"
)

// hullArt is the hull portrait a screen draws in a w×h box: a file dropped at
// assets/hulls/<id>.png wins over the embedded raster, which is the fallback
// when none is dropped (docs/ART.md). nil means neither exists (a hull
// outside the fleet's nine).
//
// Both sources answer the ART, never its canvas: the load crops a dropped
// file to its ink (render.InkCrop), and the committed raster is cropped here,
// because a hull canvas is mostly transparent margin (the owner's ships sit
// in a 4–5:1 band of a 1376×768 canvas). Fitting the canvas is what made the
// 2:1 blocks read as a small square with wide empty bands.
//
// The dropped file is a photo-sized canvas drawn into a 60–200 px box, so it
// answers with the mip level covering the frame the art actually fills — not
// the box, which would pick a needlessly finer level — and the sampled texel
// is never stretched past 2× (render.MipFor). The committed raster is already
// small enough to draw whole.
func hullArt(hull content.ShipID, w, h int) image.Image {
	full := render.HullSprite(hull)
	if full == nil {
		return render.InkCrop(hulls.Get(hull, max(w, h)))
	}
	b := full.Bounds()
	dw, dh := hullFit(b.Dx(), b.Dy(), w, h)
	return render.HullSpriteFor(hull, int(dw+0.5), int(dh+0.5))
}

// paintHullPortrait paints hull's portrait contained and centred in dst's own
// box, and reports false (leaving dst untouched) when the hull has no art.
// One seam for the HUD's 2:1 status portrait, the Show Info hull page's 2:1
// hero and the fitting window's 2:1 portrait: the ART (never its transparent
// canvas) is contained, never covered, so the frame fills edge to edge on the
// art's own long axis and no mast, antenna or wingtip is ever cropped.
func paintHullPortrait(dst *ebiten.Image, hull content.ShipID) bool {
	b := dst.Bounds()
	src := hullArt(hull, b.Dx(), b.Dy())
	if src == nil {
		return false
	}
	dst.Clear()
	sb := src.Bounds()
	dw, dh := hullFit(sb.Dx(), sb.Dy(), b.Dx(), b.Dy())
	var op ebiten.DrawImageOptions
	op.GeoM.Scale(dw/float64(sb.Dx()), dh/float64(sb.Dy()))
	op.GeoM.Translate(
		float64(b.Min.X)+(float64(b.Dx())-dw)/2,
		float64(b.Min.Y)+(float64(b.Dy())-dh)/2,
	)
	img, ok := src.(*ebiten.Image)
	if !ok {
		img = ebiten.NewImageFromImage(src)
	}
	dst.DrawImage(img, &op)
	return true
}

// hullFit is the contain fit of a sw×sh source into a bw×bh box: the drawn
// size, aspect preserved and never larger than the box. Contain, never
// cover, is the policy for these blocks: the source is the art's own ink, so
// a ship wider than 2:1 fills the width and pads above and below, a squarer
// one fills the height, and no mast or antenna is cropped. Degenerate input
// returns 0,0 so the caller skips the draw.
func hullFit(sw, sh, bw, bh int) (dw, dh float64) {
	if sw <= 0 || sh <= 0 || bw <= 0 || bh <= 0 {
		return 0, 0
	}
	s := math.Min(float64(bw)/float64(sw), float64(bh)/float64(sh))
	return float64(sw) * s, float64(sh) * s
}
