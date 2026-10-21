// Package hulls resolves a hull's portrait raster.
//
// STUB — the coordinator drops this file at integration in favor of the art
// stream's package; the API below must stay identical. The placeholder is a
// single committed silhouette, returned for every hull id: the HUD portrait
// then renders real ink in tests without shipping the final 120/240 px art.
package hulls

import (
	"bytes"
	_ "embed"
	"image"
	_ "image/png"

	"github.com/entieve/entieve/content"
)

//go:embed placeholder_120.png
var placeholder []byte

// rasters is every committed portrait size, ascending: Get picks the smallest
// raster >= size, else the largest (the API contract the real package keeps).
var rasters = []image.Image{decode()}

func decode() image.Image {
	img, _, err := image.Decode(bytes.NewReader(placeholder))
	if err != nil {
		panic("hulls: embedded placeholder: " + err.Error())
	}
	return img
}

// Get returns the portrait raster for id at the smallest committed size >=
// size (the largest when every raster is smaller). The stub ignores the id and
// never returns nil.
func Get(id content.ShipID, size int) image.Image {
	for _, img := range rasters {
		if img.Bounds().Dx() >= size {
			return img
		}
	}
	return rasters[len(rasters)-1]
}

// Has reports whether id has a portrait. The stub covers every id.
func Has(id content.ShipID) bool { return true }
