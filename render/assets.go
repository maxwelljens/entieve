package render

import (
	"image/png"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
)

// Zero-config art pipeline: a PNG dropped under assets/<kind>/<id>.png
// replaces the procedural vector art for that content ID (convention table
// in docs/ART.md). Nothing is configured — a missing file simply means
// "draw the vector art", so an empty assets/ directory plays identically.

// sprites holds the loaded art keyed by convention path relative to the
// asset root ("hulls/ship-sloop"). spritesTried records that the lazy root
// probe happened, so a missing assets/ dir is never re-scanned or logged.
// Like faceCache, access is game-goroutine only — no locking.
var (
	sprites      map[string]*ebiten.Image
	spritesTried bool
)

// LoadAssets loads every PNG under dir into the sprite registry, replacing
// whatever was loaded before. dir may be "" or absent: the registry simply
// ends up empty, never an error. An explicit load also marks the registry
// probed, so the lazy ./assets auto-load stays off afterwards. Additive
// entry point for tests and callers that keep art somewhere specific.
func LoadAssets(dir string) {
	sprites = map[string]*ebiten.Image{}
	spritesTried = true
	if dir == "" {
		return
	}
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil // unreadable entries are skipped, never fatal
		}
		ext := filepath.Ext(path)
		if !strings.EqualFold(ext, ".png") {
			return nil
		}
		rel, rerr := filepath.Rel(dir, path)
		if rerr != nil {
			return nil
		}
		if img := loadPNG(path); img != nil {
			sprites[strings.TrimSuffix(filepath.ToSlash(rel), ext)] = img
		}
		return nil
	})
}

// spriteFor returns the art sprite registered under key — a convention path
// such as "hulls/ship-sloop" (i.e. assets/hulls/ship-sloop.png) — or nil
// when no file exists. Nil is the fallback signal: every draw site then
// takes the untouched procedural path.
func spriteFor(key string) *ebiten.Image {
	ensureSprites()
	return sprites[key]
}

// ensureSprites probes the convention asset roots exactly once, on the
// first lookup: ./assets (working dir) first, then assets/ next to the
// executable. No root found = empty registry, no error, no log.
func ensureSprites() {
	if spritesTried {
		return
	}
	spritesTried = true
	for _, root := range assetRoots() {
		if fi, err := os.Stat(root); err == nil && fi.IsDir() {
			LoadAssets(root)
			return
		}
	}
}

// assetRoots lists the drop-in directory search order.
func assetRoots() []string {
	roots := []string{"assets"} // relative to the working dir
	if exe, err := os.Executable(); err == nil {
		roots = append(roots, filepath.Join(filepath.Dir(exe), "assets"))
	}
	return roots
}

// loadPNG decodes one PNG into a GPU image. nil for anything unreadable,
// empty or not a PNG — a corrupt drop-in must never crash the game.
// image/png yields straight alpha, the same convention as colorScale, so
// DrawImage composites it exactly like the procedural sprites.
func loadPNG(path string) *ebiten.Image {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil
	}
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return nil
	}
	return ebiten.NewImageFromImage(img)
}

// fitSprite scales a w×h source to fit the square of side 2·logicalRadius —
// the same footprint the procedural art draws for that object — preserving
// aspect ratio. logicalRadius is in draw pixels (already zoom-clamped by the
// caller), so world spacing and overview scale stay exactly as they are.
// Degenerate input returns 0,0; callers skip the draw.
func fitSprite(w, h int, logicalRadius float64) (dw, dh float64) {
	if w <= 0 || h <= 0 || logicalRadius <= 0 {
		return 0, 0
	}
	box := 2 * logicalRadius
	s := math.Min(box/float64(w), box/float64(h))
	return float64(w) * s, float64(h) * s
}

// spriteOptions builds the transform for a w×h sprite drawn at dw×dh px,
// centred on (cx, cy) and rotated by ang radians around its own centre
// (ang = 0 for celestials; ships pass their facing). The rotation matrix
// must match core.Vec2.Rotate — the transform the procedural silhouettes
// use — so a hull sprite points exactly where the vector nose would
// (pinned by TestSpriteOptionsMatchesVecRotate).
func spriteOptions(w, h int, cx, cy, dw, dh, ang float64) *ebiten.DrawImageOptions {
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(dw/float64(w), dh/float64(h))
	if ang != 0 {
		op.GeoM.Rotate(ang)
	}
	// Re-centre after the rotation moved the sprite's middle off (cx, cy).
	mx, my := op.GeoM.Apply(float64(w)/2, float64(h)/2)
	op.GeoM.Translate(cx-mx, cy-my)
	return op
}

// drawSprite draws img centred at (cx, cy) at size dw×dh px, rotated by ang.
// A nil image or a zero-size fit is a no-op — the procedural fallback.
func drawSprite(dst *ebiten.Image, img *ebiten.Image, cx, cy, dw, dh, ang float64) {
	if img == nil || dw <= 0 || dh <= 0 {
		return
	}
	b := img.Bounds()
	dst.DrawImage(img, spriteOptions(b.Dx(), b.Dy(), cx, cy, dw, dh, ang))
}
