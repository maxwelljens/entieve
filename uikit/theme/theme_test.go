package theme

import (
	"image/color"
	"math"
	"testing"
)

// TestPaletteGoldenValues pins the canonical Cyber Terminal palette
// (docs/lore/AESTHETIC.md; kit tokens where the kit has a role, calibrated
// extensions where it does not — uikit is the single source of truth).
func TestPaletteGoldenValues(t *testing.T) {
	p := DefaultPalette()
	want := map[string]string{
		"BG": "#0b1116ee", "BGHard": "#0a0f14", "Border": "#1f3f3a",
		"Accent": "#71f3c4", "Text": "#d9f7ef", "Dim": "#6e8c87",
		"Head": "#86a59f", "Good": "#80efb1", "Warn": "#ffc76b",
		"Danger": "#ff647c", "Info": "#74c7d9", "Shield": "#6fa8ff",
		"Struct": "#b8d7cf", "Cell": "#0e161d", "Cap": "#80efb1",
		"Player": "#8fe9ff", "Hover": "#71f3c4", "HeadBg": "#0f181f",
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
		2000: "2.0 km", 12345: "12.3 km", 9.9999e7: "99,999.0 km",
		1e8: "<0.01 AU", 1.4e9: "<0.01 AU", 1.495999e9: "<0.01 AU",
		1.496e9: "0.01 AU", 1.496e10: "0.10 AU", 6.7e10: "0.45 AU",
		3e11: "2.01 AU", 1.496e13: "100.00 AU",
	}
	for m, want := range distances {
		if got := FormatDistance(m); got != want {
			t.Errorf("formatDistance(%v) = %q, want %q", m, got, want)
		}
	}
	speeds := map[float64]string{
		-1: "0 m/s", 0: "0 m/s", 0.4: "0 m/s",
		127.6: "128 m/s", 1234.5: "1,235 m/s", 98765: "98,765 m/s",
	}
	for v, want := range speeds {
		if got := FormatSpeed(v); got != want {
			t.Errorf("formatSpeed(%v) = %q, want %q", v, got, want)
		}
	}
}

// TestSpacingTokensGolden pins the furniture rhythm and the scrollbar
// gutter derived from it. Data rows keep their own denser scale (the
// table's entryPadV literal), so these are the only air tokens.
func TestSpacingTokensGolden(t *testing.T) {
	if SpSm != 4 || SpMd != 8 || SpLg != 16 {
		t.Errorf("tokens = %d/%d/%d, want 4/8/16", SpSm, SpMd, SpLg)
	}
	if ScrollGutter != 24 {
		t.Errorf("ScrollGutter = %d, want 24 (SpLg + 2×SpSm)", ScrollGutter)
	}
}

// TestFurniturePaddingPins pins the representative furniture insets — the
// button and text-area padding and the slider's track padding (the
// handle's air) — so a padding edit is a deliberate change, not drift.
func TestFurniturePaddingPins(t *testing.T) {
	th := NewTheme(DefaultPalette(), DefaultTypeScale())
	btn := th.ButtonTheme.TextPadding
	if btn.Left != Px(SpMd) || btn.Right != Px(SpMd) || btn.Top != Px(SpSm) || btn.Bottom != Px(SpSm) {
		t.Errorf("button TextPadding = %+v, want L/R SpMd, T/B SpSm", btn)
	}
	tp := th.SliderTheme.TrackPadding
	if tp.Left != Px(SpSm) || tp.Top != Px(SpSm) {
		t.Errorf("slider TrackPadding = %+v, want SpSm", tp)
	}
	if *th.SliderTheme.FixedHandleSize != Px(SpLg) || *th.SliderTheme.MinHandleSize != Px(SpLg) {
		t.Errorf("slider handle = %d/%d, want Px(SpLg)",
			*th.SliderTheme.FixedHandleSize, *th.SliderTheme.MinHandleSize)
	}
	ta := th.TextAreaTheme
	if ta.TextPadding.Right != Px(SpMd) || ta.ScrollContainerPadding.Right != Px(SpSm) {
		t.Errorf("textarea padding right = %d / %d, want SpMd / SpSm",
			ta.TextPadding.Right, ta.ScrollContainerPadding.Right)
	}
}

// TestBorderCompositesToKitHairline pins the Border calibration: the kit's
// hairline is the accent at ~16 %, so the solid Border role must land there
// once a panel's 200-alpha edge composites it over the panel fill. The
// composited output is the contract, not the Border hex.
func TestBorderCompositesToKitHairline(t *testing.T) {
	p := DefaultPalette()
	over := func(src color.RGBA, a uint8, dst color.RGBA) (float64, float64, float64) {
		f := float64(a) / 255
		return f*float64(src.R) + (1-f)*float64(dst.R),
			f*float64(src.G) + (1-f)*float64(dst.G),
			f*float64(src.B) + (1-f)*float64(dst.B)
	}
	cr, cg, cb := over(p.Border, 200, p.BGHard)
	hr, hg, hb := over(p.Accent, 41, p.BGHard) // 0.16 × 255
	if math.Abs(cr-hr) > 3 || math.Abs(cg-hg) > 3 || math.Abs(cb-hb) > 3 {
		t.Errorf("Border@200 over BGHard = %.1f/%.1f/%.1f, want the kit hairline %.1f/%.1f/%.1f",
			cr, cg, cb, hr, hg, hb)
	}
}
