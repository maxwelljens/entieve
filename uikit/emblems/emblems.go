// Package emblems ships Entieve's corporation emblems: the original
// full-colour marks behind the `maker` slug every manufactured def carries
// (content.AllCorps). The committed SVG sources live in svg/; this package
// embeds the PNGs rasterized from them, so the game binary carries no
// rasterizer.
//
// The SVG files are the source of truth. Regenerate the PNGs and the
// manifest after editing one:
//
//	cd tools/emblemraster && go run . -svg ../../uikit/emblems/svg -out ../../uikit/emblems
//
// (or `go generate ./uikit/emblems`). The rasterizer is deterministic;
// emblems_test.go pins that the committed PNGs still match their sources
// via the manifest hashes.
//
// Unlike uikit/icons (monochrome silhouettes tinted at draw time), emblems
// are full-colour: Get returns the art as-is, picking the embedded raster
// at the requested size when one exists.
package emblems

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"image"
	_ "image/png" // the embedded art is PNG
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	xdraw "golang.org/x/image/draw"
)

//go:generate sh -c "cd ../../tools/emblemraster && go run . -svg ../../uikit/emblems/svg -out ../../uikit/emblems"

//go:embed manifest.json
var manifestJSON []byte

//go:embed *.png
var pngFS embed.FS

//go:embed svg/*.svg
var svgFS embed.FS

// Entry is one emblem's provenance record in the manifest.
type Entry struct {
	Credit string            `json:"credit"` // "original artwork"
	SVG    string            `json:"svg"`    // source path inside this package
	SVGSHA string            `json:"svg_sha256"`
	PNG    map[string]string `json:"png_sha256"` // raster size -> png sha256
}

// Manifest is manifest.json's shape: the emblem registry (one entry per
// corporation) plus the credit the artwork ships under.
type Manifest struct {
	Credit    string           `json:"credit"`
	Generator string           `json:"generator"`
	Sizes     []int            `json:"sizes"`
	Emblems   map[string]Entry `json:"emblems"`
}

var (
	loadOnce sync.Once
	man      Manifest

	mu    sync.Mutex
	cache map[string]*ebiten.Image // slug|px -> rendered emblem
)

// load parses the embedded manifest once. A malformed manifest is a build
// bug (every file is embedded), so it panics loudly.
func load() {
	loadOnce.Do(func() {
		if err := json.Unmarshal(manifestJSON, &man); err != nil {
			panic("emblems: manifest.json: " + err.Error())
		}
		if len(man.Emblems) == 0 {
			panic("emblems: manifest.json has no emblems")
		}
		cache = map[string]*ebiten.Image{}
	})
}

// Load returns the embedded emblem registry (the regeneration test
// reads its hashes).
func Load() Manifest {
	load()
	return man
}

// Get returns the emblem for a maker slug drawn into a px×px box, or nil
// for an unknown slug (makers are gated upstream, so a nil here is a bug,
// not a fallback case). The embedded raster at px is used verbatim when it
// exists; otherwise the largest embedded raster is scaled down/up.
func Get(slug string, px int) *ebiten.Image {
	load()
	e, ok := man.Emblems[slug]
	if !ok {
		return nil
	}
	if px < 1 {
		px = 1
	}
	key := fmt.Sprintf("%s|%d", slug, px)

	mu.Lock()
	defer mu.Unlock()
	if img, ok := cache[key]; ok {
		return img
	}
	size, file := pick(e, px)
	src := decode(file)
	var out image.Image = src
	if size != px {
		dst := image.NewRGBA(image.Rect(0, 0, px, px))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Src, nil)
		out = dst
	}
	img := ebiten.NewImageFromImage(out)
	cache[key] = img
	return img
}

// pick chooses the embedded raster for a requested size: the exact size
// when present, else the smallest size at or above it, else the largest.
func pick(e Entry, px int) (int, string) {
	best, bestFile := 0, ""
	for _, s := range man.Sizes {
		file := fmt.Sprintf("%s-%d.png", slugOf(e.SVG), s)
		if _, err := pngFS.ReadFile(file); err != nil {
			continue
		}
		if s == px {
			return s, file
		}
		if (best == 0 || s < best) && s > px {
			best, bestFile = s, file
		}
	}
	if bestFile != "" {
		return best, bestFile
	}
	// Fall back to the largest embedded size.
	for _, s := range man.Sizes {
		file := fmt.Sprintf("%s-%d.png", slugOf(e.SVG), s)
		if _, err := pngFS.ReadFile(file); err == nil {
			if s > best {
				best, bestFile = s, file
			}
		}
	}
	return best, bestFile
}

// slugOf derives the slug from a manifest entry's "svg/<slug>.svg" path.
func slugOf(svg string) string {
	const prefix, suffix = "svg/", ".svg"
	if len(svg) > len(prefix)+len(suffix) {
		return svg[len(prefix) : len(svg)-len(suffix)]
	}
	return svg
}

// decode reads one embedded PNG into an image.
func decode(file string) image.Image {
	raw, err := pngFS.ReadFile(file)
	if err != nil {
		panic("emblems: embedded " + file + ": " + err.Error())
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		panic("emblems: decode " + file + ": " + err.Error())
	}
	return src
}
