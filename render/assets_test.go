package render

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// stashAssets points the process-wide sprite registry at dir for the rest of
// the test and restores the previous state afterwards (the plate path index
// is process state too, so it stashes with the sprites).
func stashAssets(t *testing.T, dir string) {
	t.Helper()
	prev, prevTried, prevPlates := sprites, spritesTried, platePaths
	t.Cleanup(func() { sprites, spritesTried, platePaths = prev, prevTried, prevPlates })
	LoadAssets(dir)
}

// writePNG drops a w×h PNG at path, creating the parent directories.
func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	img.SetNRGBA(1, 1, color.NRGBA{255, 64, 32, 255})
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode %s: %v", path, err)
	}
}

// TestLoadAssetsLoadsPNGByConvention: a real PNG dropped into a convention
// subdirectory resolves by its content ID (nested-by-convention lookup) and
// keeps its pixel dimensions for the sizing math.
func TestLoadAssetsLoadsPNGByConvention(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "hulls", "ship-sloop.png"), 64, 32)
	writePNG(t, filepath.Join(dir, "stations", "cel-haven-rest.png"), 48, 96)
	stashAssets(t, dir)

	hull := spriteFor("hulls/ship-sloop")
	if hull == nil {
		t.Fatal("hulls/ship-sloop.png did not resolve via convention lookup")
	}
	if b := hull.Bounds(); b.Dx() != 64 || b.Dy() != 32 {
		t.Errorf("hull sprite = %dx%d, want 64x32", b.Dx(), b.Dy())
	}
	st := spriteFor("stations/cel-haven-rest")
	if st == nil {
		t.Fatal("stations/cel-haven-rest.png did not resolve")
	}
	if b := st.Bounds(); b.Dx() != 48 || b.Dy() != 96 {
		t.Errorf("station sprite = %dx%d, want 48x96", b.Dx(), b.Dy())
	}
}

// TestLoadAssetsMissingAndGarbageAreNil: absent file, corrupt file and
// absent root dir all yield nil (procedural fallback) and never panic.
func TestLoadAssetsMissingAndGarbageAreNil(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "hulls", "ship-sloop.png"), 16, 16)
	if err := os.WriteFile(filepath.Join(dir, "hulls", "broken.png"),
		[]byte("this is not a png"), 0o644); err != nil {
		t.Fatalf("write garbage: %v", err)
	}
	stashAssets(t, dir)

	if spriteFor("hulls/ship-sloop") == nil {
		t.Error("valid PNG must load")
	}
	if spriteFor("hulls/broken") != nil {
		t.Error("garbage file must not register a sprite")
	}
	if spriteFor("hulls/never-written") != nil {
		t.Error("missing file must fall back (nil)")
	}

	// A missing root dir is an empty set, not an error.
	stashAssets(t, filepath.Join(t.TempDir(), "no-such-dir"))
	if spriteFor("hulls/ship-sloop") != nil {
		t.Error("missing root dir must yield an empty set")
	}
	// Explicit empty load clears the registry (the test-facing "no art").
	stashAssets(t, "")
	if spriteFor("hulls/ship-sloop") != nil {
		t.Error("LoadAssets(\"\") must clear the registry")
	}
}

// TestSpriteForSelectsProceduralWhenAbsent: selection is pure — a key with a
// loaded PNG picks the sprite, every other key (and every key with no assets
// at all) picks the procedural path.
func TestSpriteForSelectsProceduralWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "hulls", "ship-cutlass.png"), 64, 32)
	stashAssets(t, dir)

	all := []string{
		"hulls/ship-sloop",
		"stations/cel-haven-rest",
		"asteroids/ore-cordite",
		"backgrounds/sys-solace",
	}
	if spriteFor("hulls/ship-cutlass") == nil {
		t.Error("present asset must be selected")
	}
	for _, key := range all {
		if spriteFor(key) != nil {
			t.Errorf("%s: absent asset must select the procedural fallback", key)
		}
	}

	stashAssets(t, "") // no assets at all: every site falls back
	for _, key := range append(all, "hulls/ship-cutlass") {
		if spriteFor(key) != nil {
			t.Errorf("%s: with no assets the procedural path must be chosen", key)
		}
	}
}

// TestAutoLoadProbesWorkingDirOnce: the root probe is lazy and one-shot —
// the first lookup scans ./assets (here: a temp working dir), and a directory
// appearing later is never picked up, so a missing root never re-scans/logs.
func TestAutoLoadProbesWorkingDirOnce(t *testing.T) {
	prev, prevTried := sprites, spritesTried
	t.Cleanup(func() { sprites, spritesTried = prev, prevTried })

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	sprites, spritesTried = nil, false
	if spriteFor("hulls/ship-sloop") != nil {
		t.Error("no asset root must yield nil")
	}
	if !spritesTried {
		t.Fatal("first lookup must trigger the one-shot root probe")
	}

	writePNG(t, filepath.Join(tmp, "assets", "hulls", "ship-sloop.png"), 8, 8)
	if spriteFor("hulls/ship-sloop") != nil {
		t.Error("root probe must run only once; later assets/ must be ignored")
	}
}

// TestFitSpritePinsSizing pins the design math: a sprite fits inside the
// procedural art's logical footprint (square of side 2·radius) with its
// aspect preserved, and degenerate input never draws.
func TestFitSpritePinsSizing(t *testing.T) {
	cases := []struct {
		name   string
		w, h   int
		radius float64
		dw, dh float64
	}{
		{"wide into r32", 100, 50, 32, 64, 32},
		{"tall into r32", 50, 100, 32, 32, 64},
		{"square into r32", 64, 64, 32, 64, 64},
		{"wide into r16", 100, 50, 16, 32, 16},
		{"zero width", 0, 50, 32, 0, 0},
		{"zero height", 100, 0, 32, 0, 0},
		{"zero radius", 100, 50, 0, 0, 0},
	}
	for _, c := range cases {
		dw, dh := fitSprite(c.w, c.h, c.radius)
		if math.Abs(dw-c.dw) > 1e-9 || math.Abs(dh-c.dh) > 1e-9 {
			t.Errorf("%s: fitSprite(%d, %d, %v) = (%v, %v), want (%v, %v)",
				c.name, c.w, c.h, c.radius, dw, dh, c.dw, c.dh)
		}
	}
}

// TestSpriteOptionsMatchesVecRotate pins the ship-sprite transform: the
// sprite centre lands on the entity's screen position, and the nose (+X,
// where the procedural hulls point) rotates exactly like core.Vec2.Rotate —
// so sprite ships and vector ships share a heading convention.
func TestSpriteOptionsMatchesVecRotate(t *testing.T) {
	const ang = 0.7
	op := spriteOptions(100, 50, 400, 300, 64, 32, ang)
	mx, my := op.GeoM.Apply(50, 25)
	if math.Abs(mx-400) > 1e-9 || math.Abs(my-300) > 1e-9 {
		t.Errorf("sprite centre = (%v, %v), want (400, 300)", mx, my)
	}
	ax, ay := op.GeoM.Apply(100, 0)
	bx, by := op.GeoM.Apply(0, 0)
	want := core.Vec2{X: 64, Y: 0}.Rotate(ang) // 100 src px → 64 dest px
	if math.Abs((ax-bx)-want.X) > 1e-9 || math.Abs((ay-by)-want.Y) > 1e-9 {
		t.Errorf("nose delta = (%v, %v), Vec2.Rotate says (%v, %v)",
			ax-bx, ay-by, want.X, want.Y)
	}
	// Unrotated sprites point along +X, as celestials expect.
	op0 := spriteOptions(100, 50, 400, 300, 64, 32, 0)
	ax, ay = op0.GeoM.Apply(100, 0)
	bx, by = op0.GeoM.Apply(0, 0)
	if math.Abs((ax-bx)-64) > 1e-9 || math.Abs(ay-by) > 1e-9 {
		t.Errorf("unrotated nose delta = (%v, %v), want (64, 0)", ax-bx, ay-by)
	}
}

// TestDrawSmokeWithAndWithoutAssets drives a full world frame offscreen both
// ways — empty registry (stock procedural path) and dropped-in sprites for a
// hull, a celestial and a background. Same call path, no panics. Pixel
// readback needs a running ebiten loop, so this asserts no-crash only.
func TestDrawSmokeWithAndWithoutAssets(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("load content: %v", err)
	}
	w, ok := c.Worlds["world-default"]
	if !ok {
		t.Fatal("world-default missing from embedded content")
	}
	s, err := sim.New(c, w, 1)
	if err != nil {
		t.Fatalf("sim.New: %v", err)
	}
	r := New(c)
	dst := ebiten.NewImage(640, 360)
	cam := Camera{Center: core.V(0, 0), Zoom: 0.02, ScreenW: 640, ScreenH: 360}

	t.Run("no assets", func(t *testing.T) {
		stashAssets(t, "")
		r.Draw(dst, cam, s) // must run the untouched procedural path
	})

	t.Run("with assets", func(t *testing.T) {
		dir := t.TempDir()
		writePNG(t, filepath.Join(dir, "hulls", "ship-sloop.png"), 64, 32)
		writePNG(t, filepath.Join(dir, "stations", "cel-haven-rest.png"), 48, 48)
		writePNG(t, filepath.Join(dir, "backgrounds", "sys-solace.png"), 320, 180)
		stashAssets(t, dir)
		r.Draw(dst, cam, s)
		// No rock is on grid at spawn, so drive the asteroid site directly
		// to exercise its sprite branch too.
		r.drawRock(dst, cam, gridObj{
			info: sim.Info{Kind: sim.KindAsteroid}, ore: "ore-cordite",
		})
	})
}
