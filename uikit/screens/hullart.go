package screens

import (
	"image"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/uikit/hulls"
)

// hullArt is the hull portrait a screen draws beside a ship's name: a file
// dropped at assets/hulls/<id>.png (render.HullSprite) wins over the
// embedded raster, which is the fallback when none is dropped (docs/ART.md).
// nil means neither exists (a hull outside the fleet's nine).
func hullArt(hull content.ShipID, size int) image.Image {
	if dropped := render.HullSprite(hull); dropped != nil {
		return dropped
	}
	return hulls.Get(hull, size)
}
