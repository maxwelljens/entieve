package render

import (
	"strings"
	"testing"

	"golang.org/x/image/font/sfnt"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// TestEmbeddedFontFamily pins the font payload: IBM Plex Sans ships as
// four weights plus its SIL OFL notice, and IBM Plex Mono (the mono
// contents face) as its single Regular TTF plus IBMPlexMono_LICENCE.txt
// — all inside the embedded directory, so the licenses travel with the
// fonts (the binary carries them too), and every glyph the interface
// actually draws is covered.
//
// IBM Plex Sans has no U+25B2/U+25BC triangles — the sort indicators are
// the ↑/↓ arrows it does carry (uikit/table.go indicator()).
func TestEmbeddedFontFamily(t *testing.T) {
	entries, err := fontFS.ReadDir("fonts")
	if err != nil {
		t.Fatalf("embedded fonts dir: %v", err)
	}
	var ttfs, others []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".ttf") {
			ttfs = append(ttfs, e.Name())
		} else {
			others = append(others, e.Name())
		}
	}
	want := []string{
		"IBMPlexSans-Bold.ttf", "IBMPlexSans-Light.ttf",
		"IBMPlexSans-Medium.ttf", "IBMPlexSans-Regular.ttf",
		"IBMPlexMono-Regular.ttf",
	}
	if len(ttfs) != len(want) {
		t.Fatalf("embedded TTFs = %v, want %v", ttfs, want)
	}
	for _, w := range want {
		found := false
		for _, g := range ttfs {
			if g == w {
				found = true
			}
		}
		if !found {
			t.Errorf("embedded font %s missing (have %v)", w, ttfs)
		}
	}
	// Each family's license travels in the same directory as its TTFs
	// (and with the binary), under its canonical per-family name.
	for _, name := range []string{"IBMPlexSans_LICENCE.txt", "IBMPlexMono_LICENCE.txt"} {
		found := false
		for _, o := range others {
			if o == name {
				found = true
			}
		}
		if !found {
			t.Errorf("license file %s missing beside the TTFs (have %v)", name, others)
			continue
		}
		// The OFL notice itself: license name and copyright line.
		lic, err := fontFS.ReadFile("fonts/" + name)
		if err != nil {
			t.Fatalf("embedded %s: %v", name, err)
		}
		for _, want := range []string{
			"SIL Open Font License, Version 1.1",
			"Copyright © 2017 IBM Corp. with Reserved Font Name \"Plex\"",
		} {
			if !strings.Contains(string(lic), want) {
				t.Errorf("%s missing %q", name, want)
			}
		}
	}
}

// TestFaceWeightsParseAndDiffer: the body and emphasis faces resolve to
// different sources at the requested size (FaceBold must not silently
// fall back to Regular), and the glyph set the interface draws —
// ASCII plus the punctuation and arrows in the labels — exists in the
// body weight.
func TestFaceWeightsParseAndDiffer(t *testing.T) {
	reg, bold := Face(21), FaceBold(21)
	if reg.Size != 21 || bold.Size != 21 {
		t.Errorf("face sizes = %v/%v, want 21/21", reg.Size, bold.Size)
	}
	if reg.Source == bold.Source {
		t.Fatal("Face and FaceBold share a source: the emphasis weight did not load")
	}
	if Face(21) != reg {
		t.Error("Face is not cached per size")
	}
	if FaceBold(16) == bold {
		t.Error("FaceBold is not cached per size")
	}

	var buf sfnt.Buffer
	raw, err := fontFS.ReadFile(regularFile)
	if err != nil {
		t.Fatal(err)
	}
	f, err := sfnt.Parse(raw)
	if err != nil {
		t.Fatalf("parse %s: %v", regularFile, err)
	}
	runes := []rune("↑↓") // the sort indicators, and nothing above them
	for c := rune(0x20); c < 0x7f; c++ {
		runes = append(runes, c) // ASCII: every label, number and key
	}
	for _, r := range []rune("·»×³≈—–…↑↓“”") { // label punctuation the UI draws
		runes = append(runes, r)
	}
	for _, r := range []rune("ąćęłńóśźżĄĆĘŁŃÓŚŹŻčďěňřšťůžČĎĚŇŘŠŤŮŽíÍ") { // the canon name alphabet: captions and tooltips draw names too
		runes = append(runes, r)
	}
	for _, r := range runes {
		k, err := f.GlyphIndex(&buf, r)
		if err != nil {
			t.Fatalf("glyph lookup %q: %v", r, err)
		}
		if k == 0 {
			t.Errorf("body face has no glyph for %q", r)
		}
	}
}

// TestMonoFaceResolvesAndCovers pins the tabular contents face: IBM
// Plex Mono parses through text/v2 at the sizes the tables draw at,
// shapes headless as a real monospace (uniform digit advances — the
// column-alignment contract; the static Regular weight is what text/v2
// draws), is a different source than the display face, and covers every
// glyph the inventory and market rows draw: row punctuation, the €/¢
// currency marks, and the Polish/Czech diacritics the system and item
// names carry (Młondan, Kvonice…).
//
// The sort indicators (↑/↓) are deliberately absent from the probe:
// they live in the header buttons, which keep the display face.
func TestMonoFaceResolvesAndCovers(t *testing.T) {
	const small = 16 // the table contents step (0.8 × the 20 px base)
	f := MonoFace(small)
	if f.Size != small {
		t.Errorf("MonoFace(%v).Size = %v, want %v", small, f.Size, small)
	}
	if MonoFace(small) != f {
		t.Error("MonoFace is not cached per size")
	}
	if MonoFace(21) == f {
		t.Error("MonoFace(21) returned the 16 px face: the cache ignores size")
	}
	if f.Source == Face(small).Source {
		t.Fatal("MonoFace shares its source with the display face: IBM Plex Mono did not load")
	}

	// text/v2 shapes the static Regular weight: every digit has the same
	// advance, and a name with diacritics measures to a real width.
	d1, _ := text.Measure("1", f, 0)
	d4, _ := text.Measure("1111", f, 0)
	d0, _ := text.Measure("0000", f, 0)
	if d1 <= 0 {
		t.Fatalf("mono face measures %q to width %v, want > 0", "1", d1)
	}
	if d4 != 4*d1 || d0 != d4 {
		t.Errorf("digit advances not uniform: %q=%v %q=%v %q=%v — not a monospace default instance",
			"1", d1, "1111", d4, "0000", d0)
	}
	if w, _ := text.Measure("Młondan 12,345.67 €", f, 0); w <= d4 {
		t.Errorf("diacritic name measures %v, want more than %q (%v)", w, "1111", d4)
	}

	var buf sfnt.Buffer
	raw, err := fontFS.ReadFile(monoFile)
	if err != nil {
		t.Fatal(err)
	}
	mf, err := sfnt.Parse(raw)
	if err != nil {
		t.Fatalf("parse %s: %v", monoFile, err)
	}
	runes := []rune{}
	for c := rune(0x20); c < 0x7f; c++ {
		runes = append(runes, c) // ASCII: every label, number and key
	}
	// Row punctuation beyond ASCII: prices ("12,345.67 €", "50¢"), the
	// volume unit ("123.4 m³"), the qty prefix ("×1,234") and the
	// ellipsis/column dash the row text can draw.
	for _, r := range []rune("€¢×³·—–…") {
		runes = append(runes, r)
	}
	// Polish and Czech diacritics in the names rows list, both cases.
	for _, r := range []rune("ąćęłńóśźżĄĆĘŁŃÓŚŹŻčďěňřšťůžČĎĚŇŘŠŤŮŽíÍ") {
		runes = append(runes, r)
	}
	for _, r := range runes {
		k, err := mf.GlyphIndex(&buf, r)
		if err != nil {
			t.Fatalf("glyph lookup %q: %v", r, err)
		}
		if k == 0 {
			t.Errorf("mono face has no glyph for %q", r)
		}
	}
}
