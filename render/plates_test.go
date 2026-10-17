package render

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
)

// writeSkyPNG drops a fully opaque w×h PNG at path — the fixture for an
// opaque-sky plate (writePNG in assets_test.go is the transparent star one).
func writeSkyPNG(t *testing.T, path string, w, h int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{0, 0, 0, 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode %s: %v", path, err)
	}
}

// TestPlateKeyLayerMapping pins the naming convention: layer 0 is the plain
// system id, layer 1 the -01 suffix, layer 2 the optional -02 suffix.
func TestPlateKeyLayerMapping(t *testing.T) {
	cases := map[int]string{
		-3: "backgrounds/sys-solace",
		0:  "backgrounds/sys-solace",
		1:  "backgrounds/sys-solace-01",
		2:  "backgrounds/sys-solace-02",
	}
	for layer, want := range cases {
		if got := plateKey("sys-solace", layer); got != want {
			t.Errorf("plateKey(sys-solace, %d) = %q, want %q", layer, got, want)
		}
	}
}

// TestPlateLoadingViaRegistry: fixture plates load through the drop-in asset
// registry by convention path, are indexed without being decoded into the
// sprite map (memory invariant), and decode on demand at their pixel size.
func TestPlateLoadingViaRegistry(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "backgrounds", "sys-fixture.png"), 64, 32)
	writePNG(t, filepath.Join(dir, "backgrounds", "sys-fixture-01.png"), 32, 64)
	stashAssets(t, dir)

	if sprites["backgrounds/sys-fixture"] != nil {
		t.Error("plates must not be decoded into the sprite registry")
	}
	p0 := platePath(plateKey("sys-fixture", 0))
	if p0 == "" || !strings.HasSuffix(p0, "sys-fixture.png") {
		t.Errorf("layer 0 path = %q, want …/sys-fixture.png", p0)
	}
	p1 := platePath(plateKey("sys-fixture", 1))
	if p1 == "" || !strings.HasSuffix(p1, "sys-fixture-01.png") {
		t.Errorf("layer 1 path = %q, want …/sys-fixture-01.png", p1)
	}
	if platePath(plateKey("sys-none", 0)) != "" {
		t.Error("a system without plates must fall back (empty path)")
	}

	img, sky := loadPlate(p0)
	if img == nil {
		t.Fatal("registered plate must decode")
	}
	if b := img.Bounds(); b.Dx() != 64 || b.Dy() != 32 {
		t.Errorf("plate = %dx%d, want 64x32", b.Dx(), b.Dy())
	}
	if sky {
		t.Error("transparent star fixture must commit to the starfield path")
	}
}

// TestPlateProbeCommitsDrawPath pins the alpha-path decision both ways: the
// one-shot pixel probe sends dots-on-transparent plates to the starfield
// layers (over the nebula washes) and opaque-black-sky plates to the bottom
// layer (under them).
func TestPlateProbeCommitsDrawPath(t *testing.T) {
	const n = 16

	star := image.NewNRGBA(image.Rect(0, 0, n, n)) // transparent sky, opaque dot
	star.SetNRGBA(8, 8, color.NRGBA{255, 255, 255, 255})
	if plateSky(star) {
		t.Error("dots on a transparent sky must take the starfield path")
	}

	solid := image.NewNRGBA(image.Rect(0, 0, n, n)) // opaque black sky + two stars
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			solid.SetNRGBA(x, y, color.NRGBA{0, 0, 0, 255})
		}
	}
	solid.SetNRGBA(3, 3, color.NRGBA{255, 255, 255, 255})
	solid.SetNRGBA(12, 5, color.NRGBA{200, 210, 255, 255})
	if !plateSky(solid) {
		t.Error("opaque black sky must take the bottom-layer path")
	}
}

// TestStarLayerTilePlateOrProcedural: layer mapping through the draw path —
// the -01 plate serves layer 1, and both the layers and the systems without
// a plate keep the untouched procedural tile (pixel-identical fallback).
func TestStarLayerTilePlateOrProcedural(t *testing.T) {
	dir := t.TempDir()
	// Only the layer-1 plate exists for this system.
	writePNG(t, filepath.Join(dir, "backgrounds", "sys-plate-01.png"), 48, 24)
	stashAssets(t, dir)

	r := New(nil)
	sys := &content.SystemDef{ID: "sys-plate"}
	seed := HashID("sys-plate")

	img, tw, th := r.starLayerTile(seed, 1, sys)
	if img == nil || img == r.starTile(seed, 1) {
		t.Fatal("layer 1 must draw its -01 plate, not the procedural tile")
	}
	if tw != 48 || th != 24 {
		t.Errorf("plate tile = %vx%v, want 48x24", tw, th)
	}
	for _, layer := range []int{0, 2} {
		img, tw, th := r.starLayerTile(seed, layer, sys)
		if img != r.starTile(seed, layer) {
			t.Errorf("layer %d without a plate must keep the procedural tile", layer)
		}
		if tw != starTileSize || th != starTileSize {
			t.Errorf("procedural tile = %vx%v, want %dx%d", tw, th, starTileSize, starTileSize)
		}
	}

	// A system with no plates at all: every layer procedural.
	plain := &content.SystemDef{ID: "sys-plain"}
	for layer := 0; layer < 3; layer++ {
		if img, _, _ := r.starLayerTile(HashID("sys-plain"), layer, plain); img != r.starTile(HashID("sys-plain"), layer) {
			t.Errorf("system without plates: layer %d must stay procedural", layer)
		}
	}
}

// TestPlateEvictedOnSystemSwitch: the plate cache holds one system's plates
// only — switching systems drops the previous decode (a 2560² plate is
// ~26 MB), and the footprint stays bounded to one system × three layers.
func TestPlateEvictedOnSystemSwitch(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "backgrounds", "sys-a.png"), 40, 20)
	writePNG(t, filepath.Join(dir, "backgrounds", "sys-a-01.png"), 20, 40)
	writePNG(t, filepath.Join(dir, "backgrounds", "sys-b.png"), 12, 12)
	stashAssets(t, dir)

	r := New(nil)
	sysA := &content.SystemDef{ID: "sys-a"}
	sysB := &content.SystemDef{ID: "sys-b"}

	p0, p1 := r.plate(0, sysA), r.plate(1, sysA)
	if p0 == nil || p1 == nil {
		t.Fatal("sys-a plates must load")
	}
	if b := p0.img.Bounds(); b.Dx() != 40 || b.Dy() != 20 {
		t.Errorf("sys-a layer 0 = %dx%d, want 40x20", b.Dx(), b.Dy())
	}
	if len(r.plates) != 2 {
		t.Fatalf("cache holds %d plates, want 2", len(r.plates))
	}

	pb := r.plate(0, sysB)
	if pb == nil {
		t.Fatal("sys-b plate must load")
	}
	if len(r.plates) != 1 {
		t.Errorf("switching systems must evict sys-a's plates, cache holds %d", len(r.plates))
	}
	if b := pb.img.Bounds(); b.Dx() != 12 || b.Dy() != 12 {
		t.Errorf("sys-b plate = %dx%d, want 12x12", b.Dx(), b.Dy())
	}

	// Returning to sys-a re-decodes: the evicted images are gone, not kept.
	again0 := r.plate(0, sysA)
	again1 := r.plate(1, sysA)
	if again0 == nil || again1 == nil {
		t.Fatal("sys-a plates must reload after the switch back")
	}
	if again0.img == p0.img || again1.img == p1.img {
		t.Error("evicted plates must not survive a system switch")
	}
	if len(r.plates) != 2 {
		t.Errorf("back on sys-a the cache holds %d plates, want 2", len(r.plates))
	}
	if len(r.plates) > 3 {
		t.Errorf("footprint unbounded: %d plates cached", len(r.plates))
	}

	// A missing layer probes once per system and caches nothing.
	if r.plate(2, sysA) != nil {
		t.Error("sys-a has no -02 plate")
	}
	if r.plate(0, sysB) == nil {
		t.Error("switching back to sys-b must reload its plate")
	}
	if len(r.plates) != 1 {
		t.Errorf("on sys-b the cache holds %d plates, want 1", len(r.plates))
	}
}
