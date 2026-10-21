package hulls

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"strconv"
	"testing"

	"github.com/entieve/entieve/content"
)

// fleet is the portrait set: exactly the nine hulls in content/data/ships.
var fleet = []content.ShipID{
	"ship-sloop",
	"ship-cutlass",
	"ship-barge",
	"ship-hauler",
	"ship-npc-patrol",
	"ship-npc-hauler",
	"ship-rat-scuttler",
	"ship-rat-marauder",
	"ship-rat-hierarch",
}

// TestManifestPinsRegeneration is the SVG↔PNG pin: the committed PNGs must
// still be what the committed SVG sources rasterize to. The manifest
// (written by tools/hullraster on every regeneration) records each source's
// and each raster's sha256; if an SVG is edited without regenerating — or a
// PNG is touched by hand — the recorded hash stops matching the embedded
// bytes and this fails.
func TestManifestPinsRegeneration(t *testing.T) {
	m := Load()
	if m.Credit != "original artwork" {
		t.Errorf("manifest credit = %q, want %q", m.Credit, "original artwork")
	}
	if m.Generator != "tools/hullraster" {
		t.Errorf("manifest generator = %q, want %q", m.Generator, "tools/hullraster")
	}
	if len(m.Sizes) != 2 || m.Sizes[0] != 120 || m.Sizes[1] != 240 {
		t.Errorf("manifest sizes = %v, want [120 240] ascending", m.Sizes)
	}
	for _, id := range fleet {
		e := m.Hulls[string(id)]
		if e.Credit != "original artwork" {
			t.Errorf("%s: entry credit = %q, want %q", id, e.Credit, "original artwork")
		}
		raw, err := svgFS.ReadFile(e.SVG)
		if err != nil {
			t.Errorf("%s: read %s: %v", id, e.SVG, err)
			continue
		}
		if got := hash(raw); got != e.SVGSHA {
			t.Errorf("%s: source hash = %s, manifest records %s — regenerate the portraits", id, got, e.SVGSHA)
		}
		for _, size := range m.Sizes {
			want, ok := e.PNG[strconv.Itoa(size)]
			if !ok {
				t.Errorf("%s: manifest has no %d px raster", id, size)
				continue
			}
			raw, err := pngFS.ReadFile(string(id) + "-" + strconv.Itoa(size) + ".png")
			if err != nil {
				t.Errorf("%s: read %d px png: %v", id, size, err)
				continue
			}
			if got := hash(raw); got != want {
				t.Errorf("%s %d px: png hash = %s, manifest records %s — regenerate the portraits", id, size, got, want)
			}
		}
	}
}

// TestPortraitForEveryHull pins the registry seam: exactly the fleet's nine
// hulls have a portrait, no more and no fewer.
func TestPortraitForEveryHull(t *testing.T) {
	m := Load()
	want := map[content.ShipID]bool{}
	for _, id := range fleet {
		want[id] = true
		if !Has(id) {
			t.Errorf("no portrait for %s", id)
		}
	}
	for id := range m.Hulls {
		if !want[content.ShipID(id)] {
			t.Errorf("portrait %s is not one of the fleet's nine hulls", id)
		}
	}
}

// TestGetPicksRaster pins the size contract: the smallest committed raster at
// or above the request, else the largest.
func TestGetPicksRaster(t *testing.T) {
	for _, c := range []struct{ ask, want int }{
		{72, 120}, {120, 120}, {121, 240}, {240, 240}, {2000, 240},
	} {
		for _, id := range fleet {
			img := Get(id, c.ask)
			if img == nil {
				t.Fatalf("%s: Get(%d) returned nil", id, c.ask)
			}
			if b := img.Bounds(); b.Dx() != c.want || b.Dy() != c.want {
				t.Errorf("%s: Get(%d) box = %v, want %d×%d", id, c.ask, b, c.want, c.want)
			}
		}
	}
	if Get("ship-does-not-exist", 120) != nil {
		t.Error("unknown hull resolved to a portrait")
	}
	if Has("ship-does-not-exist") {
		t.Error("Has said an unknown hull has a portrait")
	}
}

// TestRastersAreSquareAlphaPNGs pins the file contract: straight-alpha PNGs,
// square and exactly the manifest size, transparent at the corners, and the
// hull filling most of the frame width with a slight margin — the portraits
// have to read at 72 px.
func TestRastersAreSquareAlphaPNGs(t *testing.T) {
	for _, id := range fleet {
		for _, size := range Load().Sizes {
			file := string(id) + "-" + strconv.Itoa(size) + ".png"
			raw, err := pngFS.ReadFile(file)
			if err != nil {
				t.Errorf("%s: %v", file, err)
				continue
			}
			cfg, err := png.DecodeConfig(bytes.NewReader(raw))
			if err != nil {
				t.Errorf("%s: %v", file, err)
				continue
			}
			if cfg.ColorModel != color.NRGBAModel {
				t.Errorf("%s: colour model = %v, want NRGBA (8-bit straight alpha)", file, cfg.ColorModel)
			}
			if cfg.Width != size || cfg.Height != size {
				t.Errorf("%s: %dx%d, want %d×%d", file, cfg.Width, cfg.Height, size, size)
				continue
			}
			img, err := png.Decode(bytes.NewReader(raw))
			if err != nil {
				t.Errorf("%s: %v", file, err)
				continue
			}
			minX, minY, maxX, maxY := bounds(img)
			if minX < 1 || minY < 1 || maxX > size-2 || maxY > size-2 {
				t.Errorf("%s: ink bbox %d,%d–%d,%d escapes the 1 px margin", file, minX, minY, maxX, maxY)
			}
			if w := maxX - minX; w*4 < size*3 {
				t.Errorf("%s: hull is %d px wide of %d, want at least 75%%", file, w, size)
			}
		}
	}
}

// bounds returns the ink bounding box of a transparent-background raster.
func bounds(img image.Image) (minX, minY, maxX, maxY int) {
	b := img.Bounds()
	minX, minY, maxX, maxY = b.Max.X, b.Max.Y, b.Min.X-1, b.Min.Y-1
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA).A == 0 {
				continue
			}
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x > maxX {
				maxX = x
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	return minX, minY, maxX, maxY
}

func hash(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
