package emblems

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"testing"

	"github.com/entieve/entieve/content"
)

// TestManifestPinsRegeneration is the SVG↔PNG pin: the committed PNGs must
// still be what the committed SVG sources rasterize to. The manifest
// (written by tools/emblemraster on every regeneration) records each
// source's and each raster's sha256; if an SVG is edited without
// regenerating — or a PNG is touched by hand — the recorded hash stops
// matching the embedded bytes and this fails.
func TestManifestPinsRegeneration(t *testing.T) {
	m := Load()
	if m.Credit != "original artwork" {
		t.Errorf("manifest credit = %q, want %q", m.Credit, "original artwork")
	}
	if len(m.Sizes) != 3 || m.Sizes[0] != 24 || m.Sizes[1] != 32 || m.Sizes[2] != 48 {
		t.Errorf("manifest sizes = %v, want [24 32 48]", m.Sizes)
	}
	for _, slug := range slugsOf(t) {
		e := m.Emblems[slug]
		if e.Credit != "original artwork" {
			t.Errorf("%s: entry credit = %q, want %q", slug, e.Credit, "original artwork")
		}
		raw, err := svgFS.ReadFile(e.SVG)
		if err != nil {
			t.Errorf("%s: read %s: %v", slug, e.SVG, err)
			continue
		}
		if got := hash(raw); got != e.SVGSHA {
			t.Errorf("%s: source hash = %s, manifest records %s — regenerate the emblems", slug, got, e.SVGSHA)
		}
		for _, size := range m.Sizes {
			want, ok := e.PNG[strconv.Itoa(size)]
			if !ok {
				t.Errorf("%s: manifest has no %d px raster", slug, size)
				continue
			}
			png, err := pngFS.ReadFile(slug + "-" + strconv.Itoa(size) + ".png")
			if err != nil {
				t.Errorf("%s: read %d px png: %v", slug, size, err)
				continue
			}
			if got := hash(png); got != want {
				t.Errorf("%s %d px: png hash = %s, manifest records %s — regenerate the emblems", slug, size, got, want)
			}
		}
	}
}

// TestEmblemForEveryCorp pins the registry seam: exactly one emblem per
// manufacturer in content.AllCorps, no more and no fewer.
func TestEmblemForEveryCorp(t *testing.T) {
	m := Load()
	want := map[string]bool{}
	for _, c := range content.AllCorps {
		want[c.ID] = true
		if _, ok := m.Emblems[c.ID]; !ok {
			t.Errorf("no emblem for %s (%s)", c.ID, c.Name)
		}
	}
	for slug := range m.Emblems {
		if !want[slug] {
			t.Errorf("emblem %s has no corporation in content.AllCorps", slug)
		}
	}
}

// TestGetResolves pins the draw seam: every corp slug resolves to art at
// the UI block size, an unknown slug resolves to nil (never a fallback
// mark).
func TestGetResolves(t *testing.T) {
	for _, c := range content.AllCorps {
		img := Get(c.ID, 40)
		if img == nil {
			t.Fatalf("%s: Get returned nil", c.ID)
		}
		if b := img.Bounds(); b.Dx() != 40 || b.Dy() != 40 {
			t.Errorf("%s: emblem box = %v, want 40×40", c.ID, b)
		}
	}
	if Get("corp-does-not-exist", 40) != nil {
		t.Error("unknown slug resolved to an emblem")
	}
}

// TestGetPicksEmbeddedRaster pins the size selection: a requested 24/32/48
// uses the embedded raster at that exact size.
func TestGetPicksEmbeddedRaster(t *testing.T) {
	m := Load()
	for _, size := range m.Sizes {
		e := m.Emblems["corp-praetor"]
		gotSize, file := pick(e, size)
		if gotSize != size {
			t.Errorf("pick(%d) = %d, want the exact embedded size", size, gotSize)
		}
		if _, err := pngFS.ReadFile(file); err != nil {
			t.Errorf("pick(%d) file %s unreadable: %v", size, file, err)
		}
	}
}

func slugsOf(t *testing.T) []string {
	t.Helper()
	out := make([]string, 0, len(Load().Emblems))
	for slug := range Load().Emblems {
		out = append(out, slug)
	}
	sort.Strings(out)
	return out
}

func hash(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
