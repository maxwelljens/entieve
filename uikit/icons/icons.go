// Package icons ships Entieve's icon set: the monochrome game-icons.net
// silhouettes embedded in this directory, loaded as white-alpha bitmaps and
// tinted per palette role at draw time, plus the per-module full-colour art
// (CC0, `"mode":"art"`) drawn as-is.
//
// Every glyph is a 512×512 transparent PNG render fetched from
// https://game-icons.net (CC BY 3.0 — authors are credited in CREDITS.md and
// machine-readably in manifest.json, which this package also embeds). The art
// entries are 256×256 downscales of the local module renders, public domain.
//
// Get caches per name+size+tint (name+size for art, which ignores the tint),
// so a palette role drawn every frame costs one map lookup after the first
// call. Unknown names never miss: they resolve to the neutral gear silhouette
// (Fallback), so a caller can never paint a missing-image artifact.
//
// The player's drop-in icon set (assets/icons, one file per content id)
// registers itself here as full-colour art keyed by the content id, so a
// module or item id draws its dropped art through the very same Get.
package icons

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/png" // the embedded art is PNG
	"math"
	"sync"

	dropin "github.com/entieve/entieve/assets/icons"
	"github.com/hajimehoshi/ebiten/v2"
	xdraw "golang.org/x/image/draw"
)

// The three sizes of the UI type grid. Callers pick one; Get renders into an
// exactly size×size box, so an icon can never stretch.
const (
	Inline = 16 // table rows, target cards, compact readouts
	Button = 20 // buttons
	Title  = 24 // title bars and headers
)

// Fallback is the semantic name Get resolves unknown names to: a neutral gear
// silhouette, never a missing-image artifact.
const Fallback = "kind_unknown"

//go:embed manifest.json
var manifestJSON []byte

//go:embed *.png
var pngFS embed.FS

// Nudge is a per-icon optical correction in fractions of the icon box: the
// glyph is drawn dx·size pixels right and dy·size pixels down of where a
// purely geometric centering would put it. Centered-looking beats
// mathematically centered.
type Nudge struct {
	DX float64 `json:"dx"`
	DY float64 `json:"dy"`
}

// ArtMode is the manifest `mode` of a full-colour render: it is drawn as-is
// (no silhouette flattening, no tint), unlike the "" mode glyphs.
const ArtMode = "art"

// Entry is one manifest record: how to find the art and credit its author.
type Entry struct {
	Slug   string `json:"slug"`
	Author string `json:"author"`
	URL    string `json:"url"`
	File   string `json:"file"`
	// Mode "" = a tinted white silhouette; "art" = the full-colour render.
	Mode string `json:"mode,omitempty"`
	// License/LicenseURL override the set-level license for one entry (the
	// art entries are CC0 while the glyph set is CC BY 3.0).
	License    string `json:"license,omitempty"`
	LicenseURL string `json:"license_url,omitempty"`
	Nudge      *Nudge `json:"nudge,omitempty"`
}

// manifest is manifest.json's shape.
type manifest struct {
	License    string           `json:"license"`
	LicenseURL string           `json:"license_url"`
	Source     string           `json:"source"`
	Icons      map[string]Entry `json:"icons"`
}

var (
	loadOnce sync.Once
	registry map[string]Entry              // semantic name -> record
	silhou   map[string]silShape           // file -> decoded silhouette (alpha only)
	arts     map[string]*image.NRGBA       // file -> decoded full-colour art
	dropins  map[string]func() image.Image // content id -> lazy drop-in art

	mu    sync.Mutex
	cache map[string]*ebiten.Image // name|size|tint (name|size for art) -> rendered icon
)

// load parses the embedded manifest once. A malformed manifest is a build
// bug (both files are embedded), so it panics loudly.
func load() {
	loadOnce.Do(func() {
		var m manifest
		if err := json.Unmarshal(manifestJSON, &m); err != nil {
			panic("icons: manifest.json: " + err.Error())
		}
		if len(m.Icons) == 0 {
			panic("icons: manifest.json has no icons")
		}
		if _, ok := m.Icons[Fallback]; !ok {
			panic("icons: manifest.json is missing the fallback icon " + Fallback)
		}
		registry = m.Icons
		silhou = map[string]silShape{}
		arts = map[string]*image.NRGBA{}
		dropins = map[string]func() image.Image{}
		cache = map[string]*ebiten.Image{}
		// The player's drop-in set: one art entry per embedded file, decoded
		// on its first draw (a 720² render is never decoded unseen).
		for _, id := range dropin.IDs() {
			dropins[id] = func() image.Image {
				img, _ := dropin.Load(id)
				return img
			}
		}
	})
}

// RegisterArt adds a runtime full-colour art entry under id, drawn exactly
// like a manifest "art" entry (as-is, no tint) — the drop-in icon path, for
// art that is not in manifest.json. It replaces any earlier entry for id.
func RegisterArt(id string, img image.Image) {
	load()
	registerArt(id, func() image.Image { return img })
}

// registerArt fills the drop-in map (Get and RegisterArt hold mu; load fills
// it before anything can read it).
func registerArt(id string, dec func() image.Image) {
	mu.Lock()
	defer mu.Unlock()
	dropins[id] = dec
}

// Has reports whether name is a known semantic icon name — a manifest glyph
// or a registered drop-in art id.
func Has(name string) bool {
	load()
	if _, ok := registry[name]; ok {
		return true
	}
	_, ok := dropins[name]
	return ok
}

// Names returns every semantic icon name in the registry (the content
// validator's icon-key set, via the CLI).
func Names() []string {
	load()
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	return out
}

// Get returns the icon drawn into a size×size box and tinted with tint
// (RGB picks the color, A the opacity — 255 is fully opaque). Repeated calls
// with equal arguments return the same image. Unknown names return the
// Fallback icon.
//
// An art-mode entry ("mode":"art" or a drop-in content id) is drawn
// full-colour as-is: the tint is ignored, so its cache key drops the tint
// (one entry per name+size — two tints share it instead of thrashing the
// cache).
func Get(name string, size int, tint color.RGBA) *ebiten.Image {
	load()
	if size < 1 {
		size = 1
	}
	e, known := registry[name]
	dropinArt, dropped := dropins[name]
	if !known && !dropped {
		name = Fallback
		e = registry[Fallback]
	}
	art := dropped || e.Mode == ArtMode
	key := fmt.Sprintf("%s|%d|art", name, size)
	if !art {
		key = fmt.Sprintf("%s|%d|%d,%d,%d,%d", name, size, tint.R, tint.G, tint.B, tint.A)
	}

	mu.Lock()
	defer mu.Unlock()
	if img, ok := cache[key]; ok {
		return img
	}
	var img *ebiten.Image
	switch {
	case dropped:
		img = ebiten.NewImageFromImage(scaleBox(straightAlpha(dropinArt()), size, nil))
	case art:
		img = ebiten.NewImageFromImage(scaleBox(artImage(e.File), size, e.Nudge))
	default:
		img = ebiten.NewImageFromImage(tintBox(silhouette(e.File), size, e.Nudge, tint))
	}
	cache[key] = img
	return img
}

// straightAlpha converts a decoded image to the straight-alpha NRGBA form
// scaleBox scales (drop-in art decodes as RGBA or paletted, not NRGBA).
func straightAlpha(src image.Image) *image.NRGBA {
	if s, ok := src.(*image.NRGBA); ok {
		return s
	}
	dst := image.NewNRGBA(src.Bounds())
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
	return dst
}

// artImage decodes one embedded full-colour PNG into straight-alpha NRGBA
// once (Get holds mu).
func artImage(file string) *image.NRGBA {
	if s, ok := arts[file]; ok {
		return s
	}
	raw, err := pngFS.ReadFile(file)
	if err != nil {
		panic("icons: embedded " + file + ": " + err.Error())
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		panic("icons: decode " + file + ": " + err.Error())
	}
	dst := image.NewNRGBA(src.Bounds())
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
	arts[file] = dst
	return dst
}

// silShape is one decoded silhouette's alpha channel at its source size.
// The white silhouette is RGB=255 + alpha, so the two RGB channels are a
// constant that never carries data: caching alpha alone keeps the exact
// same pixels for 1/4 the memory (512×512: 1 MiB → 256 KiB per file).
type silShape struct {
	w, h  int
	alpha []byte
}

// silhouette returns the white silhouette of one manifest file: the alpha
// channel is decoded on first use (Get holds mu) and the RGB=255 NRGBA is
// rebuilt per call — callers scale it away immediately, so only the
// compact alpha form lives for the life of the process.
func silhouette(file string) *image.NRGBA {
	s := silhouetteAlpha(file)
	dst := image.NewNRGBA(image.Rect(0, 0, s.w, s.h))
	for i, a := range s.alpha {
		o := i * 4
		dst.Pix[o+0] = 255
		dst.Pix[o+1] = 255
		dst.Pix[o+2] = 255
		dst.Pix[o+3] = a
	}
	return dst
}

// silhouetteAlpha decodes one embedded PNG into its alpha channel once
// (Get holds mu).
func silhouetteAlpha(file string) silShape {
	if s, ok := silhou[file]; ok {
		return s
	}
	raw, err := pngFS.ReadFile(file)
	if err != nil {
		panic("icons: embedded " + file + ": " + err.Error())
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		panic("icons: decode " + file + ": " + err.Error())
	}
	white := whiteSilhouette(src)
	s := silShape{w: white.Bounds().Dx(), h: white.Bounds().Dy()}
	s.alpha = make([]byte, s.w*s.h)
	for i := range s.alpha {
		s.alpha[i] = white.Pix[i*4+3]
	}
	silhou[file] = s
	return s
}

// whiteSilhouette converts a decoded icon into a white-on-transparent
// silhouette: every RGB channel becomes 255 and the source alpha is
// preserved exactly.
func whiteSilhouette(src image.Image) *image.NRGBA {
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			_, _, _, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			o := dst.PixOffset(x, y)
			dst.Pix[o+0] = 255
			dst.Pix[o+1] = 255
			dst.Pix[o+2] = 255
			dst.Pix[o+3] = uint8(a >> 8)
		}
	}
	return dst
}

// scaleBox scales a silhouette into a size×size box with the icon's optical
// nudge applied (fractions of the box, so it tracks the size grid).
func scaleBox(sil *image.NRGBA, size int, n *Nudge) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	ox, oy := 0, 0
	if n != nil {
		ox = int(math.Round(n.DX * float64(size)))
		oy = int(math.Round(n.DY * float64(size)))
	}
	xdraw.CatmullRom.Scale(dst, image.Rect(ox, oy, ox+size, oy+size),
		sil, sil.Bounds(), xdraw.Src, nil)
	return dst
}

// tintBox scales the silhouette into its box and applies the tint, returning
// a premultiplied RGBA ready for ebiten: RGB is the tint color, alpha is the
// silhouette alpha scaled by the tint's opacity.
func tintBox(sil *image.NRGBA, size int, n *Nudge, tint color.RGBA) *image.RGBA {
	return tintNRGBA(scaleBox(sil, size, n), tint)
}

// tintNRGBA multiplies a white silhouette by tint. The result is premultiplied
// (the form ebiten copies verbatim); the disabled 40% look is tint.A == 102.
func tintNRGBA(src *image.NRGBA, tint color.RGBA) *image.RGBA {
	dst := image.NewRGBA(src.Bounds())
	for i := 0; i < len(src.Pix); i += 4 {
		a := uint16(src.Pix[i+3]) * uint16(tint.A) / 255
		dst.Pix[i+0] = uint8(uint16(tint.R) * a / 255)
		dst.Pix[i+1] = uint8(uint16(tint.G) * a / 255)
		dst.Pix[i+2] = uint8(uint16(tint.B) * a / 255)
		dst.Pix[i+3] = uint8(a)
	}
	return dst
}
