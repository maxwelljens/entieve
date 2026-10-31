package render

import (
	"image"
	"image/color"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/hajimehoshi/ebiten/v2"
)

// flatImage is a w×h RGBA plate of one colour, the box-filter fixture.
func flatImage(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

// TestHalfRGBAIsABoxFilter pins the one pure step of the chain: every
// output pixel is the premultiplied 2×2 average, an odd final row/column
// counted twice, so halving never drops a pixel or a colour.
func TestHalfRGBAIsABoxFilter(t *testing.T) {
	// 4×4 checkerboard of 1 px cells: every 2×2 block is two white and two
	// black, so each output pixel is the half-grey average.
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			c := color.RGBA{0, 0, 0, 255}
			if (x+y)%2 == 0 {
				c = color.RGBA{255, 255, 255, 255}
			}
			src.SetRGBA(x, y, c)
		}
	}
	got := halfRGBA(src)
	if b := got.Bounds(); b.Dx() != 2 || b.Dy() != 2 {
		t.Fatalf("half of 4×4 = %v, want 2×2", b)
	}
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			if c := got.RGBAAt(x, y); c != (color.RGBA{127, 127, 127, 255}) {
				t.Errorf("pixel (%d,%d) = %v, want the 2×2 average (127,127,127,255)", x, y, c)
			}
		}
	}

	// Odd sizes round up, the last column/row counted twice.
	odd := flatImage(3, 5, color.RGBA{40, 80, 120, 255})
	half := halfRGBA(odd)
	if b := half.Bounds(); b.Dx() != 2 || b.Dy() != 3 {
		t.Fatalf("half of 3×5 = %v, want 2×3", b)
	}
	for y := 0; y < 3; y++ {
		for x := 0; x < 2; x++ {
			if c := half.RGBAAt(x, y); c != (color.RGBA{40, 80, 120, 255}) {
				t.Errorf("flat 3×5 half pixel (%d,%d) = %v, want the flat colour", x, y, c)
			}
		}
	}
}

// TestMipChainHalvesToTheFloor pins the chain shape: level 0 is the full
// sprite, every level halves (rounded up), and the walk stops before a
// side would drop under mipFloor.
func TestMipChainHalvesToTheFloor(t *testing.T) {
	cases := []struct {
		w, h int
		want [][2]int
	}{
		{128, 64, [][2]int{{128, 64}, {64, 32}}},                          // one halving: 16 falls under the floor
		{321, 480, [][2]int{{321, 480}, {161, 240}, {81, 120}, {41, 60}}}, // 41×60 is the last: 20 < floor
		{20, 20, [][2]int{{20, 20}}},                                      // below a halving already
	}
	for _, c := range cases {
		src := flatImage(c.w, c.h, color.RGBA{0, 0, 0, 255})
		chain := mipChain(src, ebiten.NewImageFromImage(src))
		if len(chain) != len(c.want) {
			t.Errorf("%dx%d: chain of %d levels, want %d", c.w, c.h, len(chain), len(c.want))
			continue
		}
		for i, want := range c.want {
			b := chain[i].Bounds()
			if b.Dx() != want[0] || b.Dy() != want[1] {
				t.Errorf("%dx%d level %d = %dx%d, want %dx%d",
					c.w, c.h, i, b.Dx(), b.Dy(), want[0], want[1])
			}
		}
	}
}

// TestMipForPicksTheCoveringLevel pins the selection rule the draw sites
// rely on: the smallest level that still covers the frame (so the stretch
// stays under 2×), the full image for a frame bigger than the art, nil
// when no art is dropped.
func TestMipForPicksTheCoveringLevel(t *testing.T) {
	dir := t.TempDir()
	writeJPEG(t, dir+"/portraits/contact-big.jpg", 320, 480) // chain 320,160,80,40
	stashAssets(t, dir)

	cases := []struct {
		name  string
		w, h  int
		wantW int
		wantH int
	}{
		{"row thumb", 56, 84, 80, 120}, // 40×60 is too short, 80×120 covers
		{"details frame", 144, 216, 160, 240},
		{"tiny", 10, 10, 40, 60}, // the floor level
		{"bigger than the art", 900, 900, 320, 480},
	}
	for _, c := range cases {
		img := MipFor("portraits/contact-big", c.w, c.h)
		if img == nil {
			t.Fatalf("%s: no level", c.name)
		}
		b := img.Bounds()
		if b.Dx() != c.wantW || b.Dy() != c.wantH {
			t.Errorf("%s (%d×%d): level = %dx%d, want %dx%d",
				c.name, c.w, c.h, b.Dx(), b.Dy(), c.wantW, c.wantH)
		}
	}
	if MipFor("portraits/nobody", 56, 84) != nil {
		t.Error("a key with no art must answer nil (the caller's fallback)")
	}
	if MipFor("portraits/contact-big", 0, 0) == nil {
		t.Error("a non-positive frame must still answer the full image")
	}
}

// TestMipChainsSkipPlatesAndIcons pins what deliberately carries no chain:
// the background plates are only indexed (never decoded here), the drop-in
// icon set never enters the sprite registry, and the artwork plates are
// window-sized — MipFor answers them with the full image, not a chain.
func TestMipChainsSkipPlatesAndIcons(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir+"/backgrounds/sys-fixture.png", 64, 64)
	writePNG(t, dir+"/icons/mod-autocannon-s.png", 64, 64)
	writeJPEG(t, dir+"/artwork/spaceship_hangar.jpg", 200, 100)
	writeJPEG(t, dir+"/portraits/contact-fixture.jpg", 200, 100)
	stashAssets(t, dir)

	if mips["backgrounds/sys-fixture"] != nil {
		t.Error("a background plate got a chain (it is only indexed, never decoded)")
	}
	if mips["icons/mod-autocannon-s"] != nil {
		t.Error("the drop-in icon set got a chain")
	}
	if mips["artwork/spaceship_hangar"] != nil {
		t.Error("an artwork plate got a chain (window-sized, no sampled draw site)")
	}
	if art := MipFor("artwork/spaceship_hangar", 32, 32); art == nil || !art.Bounds().Eq(image.Rect(0, 0, 200, 100)) {
		t.Errorf("MipFor of a plate = %v, want the full 200×100 image", art)
	}
	if chain := mips["portraits/contact-fixture"]; len(chain) != 2 {
		t.Errorf("portrait chain = %d levels, want 2 (200×100 → 100×50)", len(chain))
	}
	if p := ContactPortraitFor("contact-fixture", 40, 40); p == nil || p.Bounds().Dx() != 100 {
		t.Errorf("ContactPortraitFor = %v, want the 100×50 level", p)
	}
	if ContactPortraitFor("", 40, 40) != nil {
		t.Error("an empty contact id must answer nil")
	}
}

// TestContactPortraitSampledForTheFrame is the row-thumb pin: the dock
// board's portrait box no longer pulls a full-size photo for a 56×84
// frame — the sampled level covers the frame within 2×, while the plain
// accessor still answers the full image.
func TestContactPortraitSampledForTheFrame(t *testing.T) {
	dir := t.TempDir()
	const id = content.ContactID("contact-owner-shot")
	writeJPEG(t, dir+"/portraits/"+string(id)+".jpg", 592, 880) // the owner's real size
	stashAssets(t, dir)

	full := ContactPortrait(id)
	if full == nil {
		t.Fatal("no portrait registered")
	}
	if b := full.Bounds(); b.Dx() != 592 || b.Dy() != 880 {
		t.Fatalf("full portrait = %v, want 592×880", b)
	}
	thumb := ContactPortraitFor(id, 56, 84)
	if thumb == nil {
		t.Fatal("no thumb level")
	}
	if b := thumb.Bounds(); b.Dx() > 2*56 || b.Dy() > 2*84 {
		t.Errorf("56×84 thumb samples a %dx%d level, want ≤ 112×168", b.Dx(), b.Dy())
	}
	if thumb == full {
		t.Error("the thumb still samples the full-resolution portrait")
	}
}
