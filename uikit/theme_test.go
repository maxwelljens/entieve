package uikit

import (
	"image/color"
	"math"
	"testing"
)

// TestPaletteGoldenValues pins the canonical EVE-dark palette (ported from
// the retired legacy ui/style.go — uikit is now the single source of truth).
func TestPaletteGoldenValues(t *testing.T) {
	p := DefaultPalette()
	want := map[string]string{
		"BG": "#0a0e14ee", "BGHard": "#0a0e14", "Border": "#3a4452",
		"Accent": "#ffb454", "Text": "#d8dee9", "Dim": "#7a8494",
		"Head": "#5b677a", "Good": "#5fcf80", "Warn": "#e0a33c",
		"Danger": "#e05555", "Info": "#8fb8e0", "Shield": "#6fa8ff",
		"Struct": "#c9ced6", "Cell": "#141a24", "Cap": "#5fcf80",
		"Player": "#7fe3ff", "Hover": "#ffb454", "HeadBg": "#131926",
	}
	got := map[string]string{
		"BG": hexRGBA(p.BG), "BGHard": hexRGBA(p.BGHard), "Border": hexRGBA(p.Border),
		"Accent": hexRGBA(p.Accent), "Text": hexRGBA(p.Text), "Dim": hexRGBA(p.Dim),
		"Head": hexRGBA(p.Head), "Good": hexRGBA(p.Good), "Warn": hexRGBA(p.Warn),
		"Danger": hexRGBA(p.Danger), "Info": hexRGBA(p.Info), "Shield": hexRGBA(p.Shield),
		"Struct": hexRGBA(p.Struct), "Cell": hexRGBA(p.Cell), "Cap": hexRGBA(p.Cap),
		"Player": hexRGBA(p.Player), "Hover": hexRGBA(p.Hover), "HeadBg": hexRGBA(p.HeadBg),
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s = %s, want %s", name, got[name], w)
		}
	}
}

// hexRGBA formats a color as #rrggbb or #rrggbbaa (alpha != 255).
func hexRGBA(c color.RGBA) string {
	if c.A == 255 {
		return "#" + hexByte(uint32(c.R)) + hexByte(uint32(c.G)) + hexByte(uint32(c.B))
	}
	return "#" + hexByte(uint32(c.R)) + hexByte(uint32(c.G)) + hexByte(uint32(c.B)) + hexByte(uint32(c.A))
}

func hexByte(v uint32) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[(v>>4)&0xf], digits[v&0xf]})
}

// TestTypeScaleGolden pins the type scale (20px base with 0.8/1.3/0.7 steps).
func TestTypeScaleGolden(t *testing.T) {
	ts := DefaultTypeScale()
	for _, c := range []struct {
		name string
		got  float64
		want float64
	}{
		{"Base", ts.Base, 20}, {"Small", ts.Small, 20 * 0.8},
		{"Large", ts.Large, 20 * 1.3}, {"Tiny", ts.Tiny, 20 * 0.7},
	} {
		if math.Abs(c.got-c.want) > 1e-9 {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

// TestFormattersGolden pins the number formatters to golden outputs (the
// legacy ui package they were ported from has been retired).
func TestFormattersGolden(t *testing.T) {
	distances := map[float64]string{
		-5: "0 m", 0: "0 m", 42: "42 m", 999: "999 m", 1999: "1,999 m",
		2000: "2.0 km", 12345: "12.3 km", 1.495999e9: "1,495,999.0 km",
		1.496e10: "0.1 AU", 3e11: "2.0 AU",
	}
	for m, want := range distances {
		if got := formatDistance(m); got != want {
			t.Errorf("formatDistance(%v) = %q, want %q", m, got, want)
		}
	}
	speeds := map[float64]string{
		-1: "0 m/s", 0: "0 m/s", 0.4: "0 m/s",
		127.6: "128 m/s", 1234.5: "1,235 m/s", 98765: "98,765 m/s",
	}
	for v, want := range speeds {
		if got := formatSpeed(v); got != want {
			t.Errorf("formatSpeed(%v) = %q, want %q", v, got, want)
		}
	}
}
