// Package icons ships Entieve's icon set: the monochrome game-icons.net
// silhouettes embedded in this directory, loaded as white-alpha bitmaps and
// tinted per palette role at draw time.
//
// Every icon is a 512×512 transparent PNG render fetched from
// https://game-icons.net (CC BY 3.0 — authors are credited in CREDITS.md and
// machine-readably in manifest.json, which this package also embeds).
//
// Get caches per name+size+tint, so a palette role drawn every frame costs one
// map lookup after the first call. Unknown names never miss: they resolve to
// the neutral gear silhouette (Fallback), so a caller can never paint a
// missing-image artifact.
package icons

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/png" // the embedded art is PNG
	"math"
	"sync"

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

// Entry is one manifest record: how to find the art and credit its author.
type Entry struct {
	Slug   string `json:"slug"`
	Author string `json:"author"`
	URL    string `json:"url"`
	File   string `json:"file"`
	Nudge  *Nudge `json:"nudge,omitempty"`
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
	registry map[string]Entry        // semantic name -> record
	silhou   map[string]*image.NRGBA // file -> white silhouette (decoded once)

	mu    sync.Mutex
	cache map[string]*ebiten.Image // name|size|tint -> rendered icon
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
		silhou = map[string]*image.NRGBA{}
		cache = map[string]*ebiten.Image{}
	})
}

// Has reports whether name is a known semantic icon name.
func Has(name string) bool {
	load()
	_, ok := registry[name]
	return ok
}

// names returns every semantic icon name in the registry (tests only).
func names() []string {
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
func Get(name string, size int, tint color.RGBA) *ebiten.Image {
	load()
	if size < 1 {
		size = 1
	}
	if _, ok := registry[name]; !ok {
		name = Fallback
	}
	key := fmt.Sprintf("%s|%d|%d,%d,%d,%d", name, size, tint.R, tint.G, tint.B, tint.A)

	mu.Lock()
	defer mu.Unlock()
	if img, ok := cache[key]; ok {
		return img
	}
	e := registry[name]
	img := ebiten.NewImageFromImage(tintBox(silhouette(e.File), size, e.Nudge, tint))
	cache[key] = img
	return img
}

// silhouette returns the white silhouette of one manifest file, decoding it
// on first use (Get holds mu).
func silhouette(file string) *image.NRGBA {
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
	s := whiteSilhouette(src)
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
