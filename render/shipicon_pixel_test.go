package render

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/internal/capture"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// iconTileSize is the square one probe icon is drawn into: room for the
// biggest class at zoom 1 (barge/industrial: 42 px radius) plus its bracket.
const iconTileSize = 120

// iconTile draws one ship's icon and reads the frame back as raw RGBA.
// Pixel readback needs a running ebiten loop, so callers run inside the
// capture probe below.
func iconTile(hull, kind string, hostile bool, facing float64) []byte {
	dst := ebiten.NewImage(iconTileSize, iconTileSize)
	cam := Camera{Center: core.Vec2{}, Zoom: 1, ScreenW: iconTileSize, ScreenH: iconTileSize}
	New(nil).drawShip(dst, cam, gridObj{
		info:   sim.Info{Kind: kind, Hostile: hostile},
		hull:   hull,
		facing: facing,
	}, 0)
	buf := make([]byte, iconTileSize*iconTileSize*4)
	dst.ReadPixels(buf)
	return buf
}

// TestShipIconPixelProbe is the pixel truth under the geometry pins: each
// class draws its own glyph, two headings draw byte-identical pixels, the
// tint follows the bracket rule, a dropped hull portrait changes nothing,
// and two draws are identical. Readback needs a display, so it runs inside
// the gated capture loop (ENTIEVE_CAPTURE=1) and writes no PNG: the pins
// itself must hold outside the gate (the geometry and source pins do that).
func TestShipIconPixelProbe(t *testing.T) {
	// A hull drop-in, the art the world must now ignore.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "hulls"), 0o755); err != nil {
		t.Fatal(err)
	}
	marker := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			marker.SetRGBA(x, y, color.RGBA{255, 0, 255, 255})
		}
	}
	f, err := os.Create(filepath.Join(dir, "hulls", "ship-vastrek.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, marker); err != nil {
		t.Fatal(err)
	}
	f.Close()

	(&capture.Capture{
		Name:   "Entieve ship icon probe",
		Frames: 1,
		Draw: func(_ int, _ *ebiten.Image) error {
			classes := []string{"frigate", "destroyer", "industrial", "barge", "rat"}
			tiles := map[string][]byte{}
			for _, class := range classes {
				tiles[class] = iconTile(class, sim.KindShip, true, 0)
			}
			for i, a := range classes {
				for _, b := range classes[i+1:] {
					if bytes.Equal(tiles[a], tiles[b]) {
						t.Errorf("classes %q and %q draw identical pixels", a, b)
					}
				}
			}

			// Non-rotating: a quarter turn of heading is the same picture.
			for _, class := range classes {
				if got := iconTile(class, sim.KindShip, true, 1.5708); !bytes.Equal(got, tiles[class]) {
					t.Errorf("%s: the icon moved with the heading", class)
				}
			}
			// Determinism: same inputs, same bytes.
			if !bytes.Equal(iconTile("barge", sim.KindShip, true, 0), tiles["barge"]) {
				t.Error("two identical draws differ")
			}

			// Tint: affiliation changes the colour, never the silhouette.
			hostile := tiles["destroyer"]
			neutral := iconTile("destroyer", sim.KindShip, false, 0)
			player := iconTile("destroyer", sim.KindPlayer, false, 0)
			if bytes.Equal(hostile, neutral) || bytes.Equal(hostile, player) {
				t.Error("the affiliation tint did not change the icon")
			}
			if !sameMask(hostile, neutral) || !sameMask(hostile, player) {
				t.Error("the tint changed the silhouette, not just the colour")
			}

			// A dropped hull portrait is portrait art: world pixels ignore it.
			before := iconTile("frigate", sim.KindPlayer, false, 0.4)
			LoadAssets(dir)
			defer LoadAssets("")
			if got := iconTile("frigate", sim.KindPlayer, false, 0.4); !bytes.Equal(got, before) {
				t.Error("a hull drop-in changed the world icon")
			}
			return nil
		},
	}).Run(t)
}

// sameMask reports whether two RGBA buffers cover the same pixels (alpha,
// since the icon is filled at full alpha).
func sameMask(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 3; i < len(a); i += 4 {
		if (a[i] >= 128) != (b[i] >= 128) {
			return false
		}
	}
	return true
}
