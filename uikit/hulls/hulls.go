// Package hulls ships Entieve's hull portraits: the top-down, full-colour
// hull art the HUD and Show Info draw beside a ship's name. The committed
// SVG sources live in svg/; this package embeds the PNGs rasterized from
// them, so the game binary carries no rasterizer.
//
// The SVG files are the source of truth. Regenerate the PNGs and the
// manifest after editing one:
//
//	cd tools/hullraster && go run . -svg ../../uikit/hulls/svg -out ../../uikit/hulls
//
// (or `go generate ./uikit/hulls`). The rasterizer is deterministic;
// hulls_test.go pins that the committed PNGs still match their sources via
// the manifest hashes.
//
// Like uikit/emblems, portraits are full-colour: Get returns the embedded
// raster as-is and never tints it. Sizes are picked, not scaled — the
// caller draws the returned image into its own box.
package hulls

import (
	"bytes"
	"embed"
	"encoding/json"
	"image"
	_ "image/png" // the embedded art is PNG
	"strconv"
	"sync"

	"github.com/entieve/entieve/content"
)

//go:generate sh -c "cd ../../tools/hullraster && go run . -svg ../../uikit/hulls/svg -out ../../uikit/hulls"

//go:embed manifest.json
var manifestJSON []byte

//go:embed *.png
var pngFS embed.FS

//go:embed svg/*.svg
var svgFS embed.FS

// Entry is one portrait's provenance record in the manifest.
type Entry struct {
	Credit string            `json:"credit"` // "original artwork"
	SVG    string            `json:"svg"`    // source path inside this package
	SVGSHA string            `json:"svg_sha256"`
	PNG    map[string]string `json:"png_sha256"` // raster size -> png sha256
}

// Manifest is manifest.json's shape: one entry per hull plus the sizes the
// rasterizer emits, ascending.
type Manifest struct {
	Credit    string           `json:"credit"`
	Generator string           `json:"generator"`
	Sizes     []int            `json:"sizes"`
	Hulls     map[string]Entry `json:"hulls"`
}

var (
	loadOnce sync.Once
	man      Manifest

	mu    sync.Mutex
	cache map[string]image.Image // embedded file -> decoded raster
)

// load parses the embedded manifest once. A malformed manifest is a build
// bug (every file is embedded), so it panics loudly.
func load() {
	loadOnce.Do(func() {
		if err := json.Unmarshal(manifestJSON, &man); err != nil {
			panic("hulls: manifest.json: " + err.Error())
		}
		if len(man.Hulls) == 0 {
			panic("hulls: manifest.json has no hulls")
		}
		cache = map[string]image.Image{}
	})
}

// Load returns the embedded portrait registry (the regeneration test reads
// its hashes).
func Load() Manifest {
	load()
	return man
}

// Has reports whether the hull has a portrait.
func Has(id content.ShipID) bool {
	load()
	_, ok := man.Hulls[string(id)]
	return ok
}

// Get returns the portrait for a hull: the smallest committed raster at least
// size px, else the largest one. It is nil for an unknown hull, so a caller
// can fall back to its own silhouette.
func Get(id content.ShipID, size int) image.Image {
	load()
	if _, ok := man.Hulls[string(id)]; !ok {
		return nil
	}
	_, file := pick(id, size)

	mu.Lock()
	defer mu.Unlock()
	if img, ok := cache[file]; ok {
		return img
	}
	img := decode(file)
	cache[file] = img
	return img
}

// pick chooses the embedded raster for a requested size: the smallest size at
// or above it, else the largest. man.Sizes is ascending (the test pins it).
func pick(id content.ShipID, size int) (int, string) {
	best, bestFile := 0, ""
	for _, s := range man.Sizes {
		file := string(id) + "-" + strconv.Itoa(s) + ".png"
		if _, err := pngFS.ReadFile(file); err != nil {
			continue
		}
		if s >= size {
			return s, file
		}
		best, bestFile = s, file
	}
	return best, bestFile
}

// decode reads one embedded PNG into an image.
func decode(file string) image.Image {
	raw, err := pngFS.ReadFile(file)
	if err != nil {
		panic("hulls: embedded " + file + ": " + err.Error())
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		panic("hulls: decode " + file + ": " + err.Error())
	}
	return src
}
