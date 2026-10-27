package icons

import (
	"encoding/json"
	"image"
	"image/color"
	"os"
	"strings"
	"testing"

	dropin "github.com/entieve/entieve/assets/icons"
)

// readManifest loads manifest.json from disk (the same bytes Get embeds).
func readManifest(t *testing.T) manifest {
	t.Helper()
	raw, err := os.ReadFile("manifest.json")
	if err != nil {
		t.Fatalf("manifest.json: %v", err)
	}
	if string(raw) != string(manifestJSON) {
		t.Fatal("embedded manifest.json differs from the file on disk")
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("manifest.json: %v", err)
	}
	return m
}

// TestManifestCreditsAndFiles pins the credits bookkeeping: every PNG has a
// manifest entry, every manifest entry has a PNG, and every entry is credited
// in CREDITS.md — glyphs with their author and the CC BY 3.0 license, art
// entries with the project source and CC0 (no orphan art, no uncredited art).
func TestManifestCreditsAndFiles(t *testing.T) {
	m := readManifest(t)
	if m.License != "CC BY 3.0" {
		t.Errorf("license = %q, want CC BY 3.0", m.License)
	}
	if m.Source != "https://game-icons.net" {
		t.Errorf("source = %q, want the game-icons.net origin", m.Source)
	}
	credits, err := os.ReadFile("CREDITS.md")
	if err != nil {
		t.Fatalf("CREDITS.md: %v", err)
	}
	lines := strings.Split(string(credits), "\n")

	files := map[string]bool{}
	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".png") {
			files[e.Name()] = true
		}
	}
	if len(files) == 0 {
		t.Fatal("no PNG art in the icon directory")
	}

	art := 0
	used := map[string]bool{}
	for name, e := range m.Icons {
		if e.Slug == "" || e.Author == "" || e.URL == "" || e.File == "" {
			t.Errorf("%s: incomplete record %+v", name, e)
		}
		if !files[e.File] {
			t.Errorf("%s: file %q is not in the icon directory", name, e.File)
		}
		used[e.File] = true

		// Every entry must carry a credits line; what that line says depends
		// on the art's origin.
		var wants []string
		switch e.Mode {
		case ArtMode:
			art++
			if !strings.HasPrefix(e.URL, "https://github.com/maxwelljens/entieve") {
				t.Errorf("%s: url = %q, want the project repository", name, e.URL)
			}
			if e.License == "" || e.LicenseURL == "" {
				t.Errorf("%s: art entry missing license/license_url", name)
			}
			wants = []string{"Public domain", e.LicenseURL}
		case "":
			if !strings.HasPrefix(e.URL, "https://game-icons.net/") {
				t.Errorf("%s: url = %q, want a game-icons.net page", name, e.URL)
			}
			wants = []string{
				e.Slug,
				"Icons made by " + e.Author + ". Available on https://game-icons.net",
				"https://creativecommons.org/licenses/by/3.0/",
			}
		default:
			t.Errorf("%s: unknown mode %q", name, e.Mode)
			continue
		}

		var found bool
		for _, ln := range lines {
			if !strings.Contains(ln, "`"+name+"`") {
				continue
			}
			found = true
			for _, want := range wants {
				if !strings.Contains(ln, want) {
					t.Errorf("%s: credits line missing %q:\n  %s", name, want, ln)
				}
			}
		}
		if !found {
			t.Errorf("%s: no CREDITS.md line", name)
		}
	}
	if art == 0 {
		t.Error("manifest has no art-mode entry")
	}
	for f := range files {
		if !used[f] {
			t.Errorf("%s: PNG has no manifest entry (orphan art)", f)
		}
	}
	if _, ok := m.Icons[Fallback]; !ok {
		t.Errorf("manifest has no %s fallback", Fallback)
	}
}

// TestWhiteSilhouette pins the conversion math: RGB becomes white, alpha is
// preserved exactly (8-bit in, 8-bit out, no rounding drift).
func TestWhiteSilhouette(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	cases := []struct {
		x, y int
		c    color.NRGBA
	}{
		{0, 0, color.NRGBA{R: 10, G: 200, B: 30, A: 0}},     // fully transparent
		{1, 0, color.NRGBA{R: 255, G: 255, B: 255, A: 255}}, // fully opaque
		{0, 1, color.NRGBA{R: 0, G: 0, B: 0, A: 1}},         // one unit of alpha
		{1, 1, color.NRGBA{R: 77, G: 88, B: 99, A: 102}},    // partial
	}
	for _, c := range cases {
		src.SetNRGBA(c.x, c.y, c.c)
	}
	got := whiteSilhouette(src)
	if got.Bounds() != image.Rect(0, 0, 2, 2) {
		t.Fatalf("bounds = %v, want 2×2", got.Bounds())
	}
	for _, c := range cases {
		o := got.PixOffset(c.x, c.y)
		if got.Pix[o] != 255 || got.Pix[o+1] != 255 || got.Pix[o+2] != 255 {
			t.Errorf("(%d,%d) rgb = %v, want 255,255,255", c.x, c.y, got.Pix[o:o+3])
		}
		if got.Pix[o+3] != c.c.A {
			t.Errorf("(%d,%d) alpha = %d, want %d preserved exactly",
				c.x, c.y, got.Pix[o+3], c.c.A)
		}
	}
}

// TestTintAlpha pins the tint math: RGB is the tint, alpha is the silhouette
// alpha scaled by the tint's opacity (the disabled look is tint.A == 102 =
// 40%).
func TestTintAlpha(t *testing.T) {
	sil := image.NewNRGBA(image.Rect(0, 0, 1, 2))
	sil.SetNRGBA(0, 0, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	sil.SetNRGBA(0, 1, color.NRGBA{R: 255, G: 255, B: 255, A: 200})

	amber := color.RGBA{R: 0xff, G: 0xb4, B: 0x54, A: 255}
	got := tintNRGBA(sil, amber)
	p := got.PixOffset(0, 0)
	if got.Pix[p] != 0xff || got.Pix[p+1] != 0xb4 || got.Pix[p+2] != 0x54 || got.Pix[p+3] != 255 {
		t.Errorf("opaque tint = %v, want ff b4 54 ff", got.Pix[p:p+4])
	}

	disabled := color.RGBA{R: 0x7a, G: 0x84, B: 0x94, A: 102} // 40%
	got = tintNRGBA(sil, disabled)
	p = got.PixOffset(0, 0)
	if got.Pix[p+3] != 102 {
		t.Errorf("disabled alpha = %d, want 102 (40%% of 255)", got.Pix[p+3])
	}
	p = got.PixOffset(0, 1)
	if got.Pix[p+3] != 80 {
		t.Errorf("200 alpha at 40%% = %d, want 80", got.Pix[p+3])
	}
	// Premultiplied: no channel may exceed alpha.
	for y := 0; y < 2; y++ {
		o := got.PixOffset(0, y)
		for ch := 0; ch < 3; ch++ {
			if got.Pix[o+ch] > got.Pix[o+3] {
				t.Errorf("channel %d = %d exceeds alpha %d (not premultiplied)",
					ch, got.Pix[o+ch], got.Pix[o+3])
			}
		}
	}
}

// TestBoxSizesAreSquareAndOnTheGrid: every size of the type grid renders into
// an exactly square box (aspect can never stretch) and non-grid sizes work
// too.
func TestBoxSizesAreSquareAndOnTheGrid(t *testing.T) {
	white := color.RGBA{A: 255}
	for _, size := range []int{Inline, Button, Title, 1, 8, 33, 128} {
		for _, name := range []string{"kind_ship", "act_orbit", Fallback, "not-a-real-icon"} {
			img := Get(name, size, white)
			b := img.Bounds()
			if b.Dx() != size || b.Dy() != size {
				t.Errorf("Get(%q, %d) bounds = %v, want %d×%d",
					name, size, b, size, size)
			}
		}
	}
	if Inline != 16 || Button != 20 || Title != 24 {
		t.Errorf("size grid = %d/%d/%d, want 16/20/24", Inline, Button, Title)
	}
}

// TestFallbackNeverMisses: unknown names resolve to the neutral gear and
// share its cached image, and every manifest name is reportable by Has.
func TestFallbackNeverMisses(t *testing.T) {
	white := color.RGBA{A: 255}
	if !Has(Fallback) {
		t.Fatalf("Has(%q) = false", Fallback)
	}
	unknown := Get("kind_definitely_not_here", Button, white)
	fallback := Get(Fallback, Button, white)
	if unknown != fallback {
		t.Errorf("unknown name rendered its own image, want the %s image", Fallback)
	}
	for _, n := range Names() {
		if !Has(n) {
			t.Errorf("Has(%q) = false for a manifest icon", n)
		}
	}
}

// TestGetCachesByArguments: identical arguments return the identical image,
// any argument change returns a different one.
func TestGetCachesByArguments(t *testing.T) {
	a := Get("kind_ship", Inline, color.RGBA{R: 216, G: 222, B: 233, A: 255})
	b := Get("kind_ship", Inline, color.RGBA{R: 216, G: 222, B: 233, A: 255})
	if a != b {
		t.Error("same name+size+tint returned different images")
	}
	tinted := Get("kind_ship", Inline, color.RGBA{R: 224, G: 85, B: 85, A: 255})
	if a == tinted {
		t.Error("different tint shared the cached image")
	}
	resized := Get("kind_ship", Button, color.RGBA{R: 216, G: 222, B: 233, A: 255})
	if a == resized {
		t.Error("different size shared the cached image")
	}
	other := Get("act_orbit", Inline, color.RGBA{R: 216, G: 222, B: 233, A: 255})
	if a == other {
		t.Error("different icon shared the cached image")
	}
}

// filled reports whether (x,y) is solidly inked in a CPU-side box.
func filled(img *image.NRGBA, x, y int) bool { return img.Pix[img.PixOffset(x, y)+3] > 200 }

// empty reports whether (x,y) is solidly transparent in a CPU-side box.
func empty(img *image.NRGBA, x, y int) bool { return img.Pix[img.PixOffset(x, y)+3] < 32 }

// TestNudgeShiftsGlyph pins the optical-nudge math: a nudge is a fraction of
// the box, applied by drawing the scaled glyph off the geometric center.
func TestNudgeShiftsGlyph(t *testing.T) {
	// A centered 32 px block inside a 64 px source.
	sil := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 16; y < 48; y++ {
		for x := 16; x < 48; x++ {
			sil.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	// No nudge: the block lands centered in the 32 px box (8..24).
	flat := scaleBox(sil, 32, nil)
	if !filled(flat, 10, 10) || !empty(flat, 4, 4) {
		t.Error("nudge-free scale is not centered")
	}
	// dx 0.25 shifts the glyph right by 8 px (0.25 × 32).
	shifted := scaleBox(sil, 32, &Nudge{DX: 0.25, DY: 0})
	if !empty(shifted, 10, 10) {
		t.Error("dx nudge did not move the glyph right (left edge still filled)")
	}
	if !filled(shifted, 20, 10) {
		t.Error("dx nudge did not move the glyph right (right side empty)")
	}
	// dy 0.125 shifts it down by 4 px (0.125 × 32), rounded.
	down := scaleBox(sil, 32, &Nudge{DX: 0, DY: 0.125})
	if !empty(down, 10, 6) {
		t.Error("dy nudge did not move the glyph down (top still filled)")
	}
	if !filled(down, 10, 14) {
		t.Error("dy nudge did not move the glyph down (bottom empty)")
	}
}

// TestNudgeValuesAreSane pins the shipped optical corrections: each one is a// small fraction of the box (a large value would read as a misplaced icon).
func TestNudgeValuesAreSane(t *testing.T) {
	for name, e := range readManifest(t).Icons {
		if e.Nudge == nil {
			continue
		}
		if e.Nudge.DX < -0.25 || e.Nudge.DX > 0.25 || e.Nudge.DY < -0.25 || e.Nudge.DY > 0.25 {
			t.Errorf("%s: nudge %+v exceeds ±0.25 of the box", name, e.Nudge)
		}
		if e.Nudge.DX == 0 && e.Nudge.DY == 0 {
			t.Errorf("%s: nudge is all zero — drop the field instead", name)
		}
	}
}

// TestEveryIconRendersAtEverySize is the draw smoke: every manifest icon, at
// every grid size, in the primary and disabled tints, produces pixels.
func TestEveryIconRendersAtEverySize(t *testing.T) {
	tints := map[string]color.RGBA{
		"text":     {R: 0xd8, G: 0xde, B: 0xe9, A: 255},
		"disabled": {R: 0x7a, G: 0x84, B: 0x94, A: 102},
	}
	for _, name := range Names() {
		e := registry[name]
		for _, size := range []int{Inline, Button, Title} {
			for tn, tint := range tints {
				img := Get(name, size, tint)
				if img.Bounds().Dx() != size || img.Bounds().Dy() != size {
					t.Fatalf("%s@%d: bounds %v", name, size, img.Bounds())
				}
				if tn != "text" {
					continue
				}
				// Art is drawn full-colour (the silhouette path would flatten
				// it white); glyphs go through the tinted silhouette.
				if e.Mode == ArtMode {
					if !nrgbaInk(artBox(e, size)) {
						t.Errorf("%s@%d: art rendered no ink", name, size)
					}
				} else if !hasInk(tintBox(silhouette(e.File), size, e.Nudge, tint)) {
					t.Errorf("%s@%d: rendered no ink", name, size)
				}
			}
		}
	}
}

// TestArtModeIsFullColourAndTintFree pins the art draw path: an art entry
// ignores the requested tint (two tints share one cached image) and keeps the
// render's own colours, where the glyph path would flatten everything to
// white.
func TestArtModeIsFullColourAndTintFree(t *testing.T) {
	red := color.RGBA{R: 255, A: 255}
	blue := color.RGBA{B: 255, A: 255}
	checked := 0
	for name, e := range readManifest(t).Icons {
		if e.Mode != ArtMode {
			continue
		}
		checked++
		a, b := Get(name, Title, red), Get(name, Title, blue)
		if a != b {
			t.Errorf("%s: two tints rendered different art images (tint must be ignored)", name)
		}
		if !nrgbaColour(artBox(e, Title)) {
			t.Errorf("%s: art rendered without a coloured pixel (flattened?)", name)
		}
	}
	if checked == 0 {
		t.Fatal("no art-mode entries to check")
	}
}

// TestDropinArtIsRegisteredAsArt pins the player's drop-in set on the same
// draw path as the manifest art: every embedded content id answers Has, draws
// full-colour (two tints share one image — the tint is ignored) and stays out
// of the manifest registry.
func TestDropinArtIsRegisteredAsArt(t *testing.T) {
	ids := dropin.IDs()
	if len(ids) == 0 {
		t.Fatal("no drop-in icons registered")
	}
	red := color.RGBA{R: 255, A: 255}
	blue := color.RGBA{B: 255, A: 255}
	for _, id := range ids {
		if !Has(id) {
			t.Errorf("%s: Has = false for a registered drop-in", id)
		}
		if _, in := registry[id]; in {
			t.Errorf("%s: drop-in id shadows a manifest entry", id)
		}
		if Get(id, Title, red) != Get(id, Title, blue) {
			t.Errorf("%s: two tints rendered different drop-in images (tint must be ignored)", id)
		}
		img, ok := dropin.Load(id)
		if !ok || !nrgbaInk(scaleBox(straightAlpha(img), Title, nil)) {
			t.Errorf("%s: drop-in art rendered no ink", id)
		}
	}
}

// artBox is the CPU-side art render Get hands to ebiten (the test's only
// window into its pixels: a live ebiten image cannot be read headless).
func artBox(e Entry, size int) *image.NRGBA {
	return scaleBox(artImage(e.File), size, e.Nudge)
}

// nrgbaInk reports whether a CPU-side box has any visible pixel.
func nrgbaInk(img *image.NRGBA) bool {
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] > 8 {
			return true
		}
	}
	return false
}

// nrgbaColour reports whether a CPU-side box carries a pixel whose channels
// are clearly unequal (≈24/255): a flattened glyph would be white/grey only.
func nrgbaColour(img *image.NRGBA) bool {
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i+3] < 64 {
			continue
		}
		hi := max(img.Pix[i], img.Pix[i+1], img.Pix[i+2])
		lo := min(img.Pix[i], img.Pix[i+1], img.Pix[i+2])
		if int(hi)-int(lo) > 24 {
			return true
		}
	}
	return false
}

// hasInk reports whether a CPU-side tinted box has any visible pixel.
func hasInk(img *image.RGBA) bool {
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] > 8 {
			return true
		}
	}
	return false
}
