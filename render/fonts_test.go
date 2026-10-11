package render

import (
	"strings"
	"testing"

	"golang.org/x/image/font/sfnt"
)

// TestEmbeddedFontFamily pins the font payload: Turret Road ships as six
// weights plus its OFL.txt inside the embedded directory (the license
// travels with the fonts), both faces the UI builds parse, and every
// glyph the interface actually draws is covered.
//
// Turret Road has no U+25B2/U+25BC triangles — the sort indicators are
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
		"TurretRoad-Bold.ttf", "TurretRoad-ExtraBold.ttf", "TurretRoad-ExtraLight.ttf",
		"TurretRoad-Light.ttf", "TurretRoad-Medium.ttf", "TurretRoad-Regular.ttf",
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
	if len(others) != 1 || others[0] != "OFL.txt" {
		t.Errorf("files beside the TTFs = %v, want [OFL.txt] (the license travels with the fonts)", others)
	}

	// The OFL notice itself: license name and copyright line.
	lic, err := fontFS.ReadFile("fonts/OFL.txt")
	if err != nil {
		t.Fatalf("embedded OFL.txt: %v", err)
	}
	for _, want := range []string{
		"SIL Open Font License, Version 1.1",
		"Copyright 2018 The Turret Road Project Authors",
	} {
		if !strings.Contains(string(lic), want) {
			t.Errorf("OFL.txt missing %q", want)
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
	if FaceBold(16.8) == bold {
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
