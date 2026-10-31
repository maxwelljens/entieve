package render

// Mip chains: every sprite the drop-in walk registers also carries a
// halving chain of its own pixels — level 0 is the sprite, level 1 half its
// size, and so on down to mipFloor. A draw site showing a sprite far below
// its native size (a 592×880 portrait photo in a thumb) then samples a
// level built for that size instead of asking the GPU to skip ~20 texels
// per output pixel, which is the shimmer the dock view showed.
//
// The chain is a CPU 2×2 box filter over the decoded pixels, not a GPU
// downscale: captures, twin runs and every machine build bit-identical
// levels. Building it is one walk per sprite at load (game goroutine only,
// like the sprite registry itself — no locking).
//
// The artwork kind is the one exclusion: a window-sized plate
// (spaceship_hangar.jpg is 2512×1680) has no draw site that samples it
// through a chain yet, so a chain would be 5.6 MB of held texture for
// nothing. MipFor still answers for those keys — with the full plate.

import (
	"image"
	"image/color"
	"image/draw"

	"github.com/entieve/entieve/content"
	"github.com/hajimehoshi/ebiten/v2"
)

// mipFloor is the smallest side a chain level may have: a level below it
// is built on demand by nobody (the smallest draw site here is a 20 px
// icon box), and every level costs GPU memory for the sprite's whole life.
const mipFloor = 32

// mips holds each sprite's chain, level 0 first (built by mipChain, reset
// by LoadAssets). Same lifetime and goroutine rules as sprites.

// mipChain builds the halving chain of one decoded sprite: full is level 0
// (the sprite's own image — the chain never duplicates it), then 2×2 box
// averages, rounded down, until the next level would drop a side below
// mipFloor. A sprite already smaller than two floors is a chain of one.
func mipChain(src image.Image, full *ebiten.Image) []*ebiten.Image {
	base := toRGBA(src)
	chain := []*ebiten.Image{full}
	for {
		b := base.Bounds()
		if b.Dx()/2 < mipFloor || b.Dy()/2 < mipFloor {
			return chain
		}
		base = halfRGBA(base)
		chain = append(chain, ebiten.NewImageFromImage(base))
	}
}

// toRGBA copies any decoded image into premultiplied 8-bit RGBA — the
// format the box filter and the GPU both want (a JPEG decodes as YCbCr,
// whose At is a conversion per pixel).
func toRGBA(src image.Image) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

// halfRGBA is one chain step: every output pixel the premultiplied average
// of its 2×2 source block, an odd last row/column counted twice so no
// pixel is dropped. Pure, so the filter is pinned without a GPU.
func halfRGBA(src *image.RGBA) *image.RGBA {
	b := src.Bounds()
	w, h := (b.Dx()+1)/2, (b.Dy()+1)/2
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		y0 := min(b.Min.Y+2*y, b.Max.Y-1)
		y1 := min(b.Min.Y+2*y+1, b.Max.Y-1)
		for x := 0; x < w; x++ {
			x0 := min(b.Min.X+2*x, b.Max.X-1)
			x1 := min(b.Min.X+2*x+1, b.Max.X-1)
			a, c := src.RGBAAt(x0, y0), src.RGBAAt(x1, y0)
			d, e := src.RGBAAt(x0, y1), src.RGBAAt(x1, y1)
			dst.SetRGBA(x, y, color.RGBA{
				R: uint8((uint16(a.R) + uint16(c.R) + uint16(d.R) + uint16(e.R)) / 4),
				G: uint8((uint16(a.G) + uint16(c.G) + uint16(d.G) + uint16(e.G)) / 4),
				B: uint8((uint16(a.B) + uint16(c.B) + uint16(d.B) + uint16(e.B)) / 4),
				A: uint8((uint16(a.A) + uint16(c.A) + uint16(d.A) + uint16(e.A)) / 4),
			})
		}
	}
	return dst
}

// MipFor returns the sprite registered under key sampled for a w×h frame:
// the smallest chain level that still covers it, so a cover/fit draw
// stretches the sampled texels at most 2× and filtering stays sharp. The
// full image answers for a frame bigger than the art (or a non-positive
// one) and for a kind that carries no chain (the artwork plates). nil when
// no art is dropped for the key — the caller's fallback signal.
func MipFor(key string, w, h int) *ebiten.Image {
	ensureSprites()
	chain := mips[key]
	if len(chain) == 0 {
		return sprites[key]
	}
	if w > 0 && h > 0 {
		for i := len(chain) - 1; i > 0; i-- {
			if b := chain[i].Bounds(); b.Dx() >= w && b.Dy() >= h {
				return chain[i]
			}
		}
	}
	return chain[0]
}

// ContactPortraitFor is MipFor on the contact-portrait convention key: the
// portrait level that covers a w×h frame. nil when no portrait is dropped.
func ContactPortraitFor(id content.ContactID, w, h int) *ebiten.Image {
	if id == "" {
		return nil
	}
	return MipFor(contactPortraitRoot+string(id), w, h)
}
