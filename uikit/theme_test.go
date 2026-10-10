package uikit

import (
	"image/color"
	"testing"

	"github.com/entieve/entieve/ui"
)

// TestThemeMatchesLegacyPalette pins the uikit theme to the legacy ui
// package while both layers coexist: any palette or type-scale drift in
// either direction fails here. This is the only test file allowed to
// import ui.
func TestThemeMatchesLegacyPalette(t *testing.T) {
	p := DefaultPalette()
	cases := []struct {
		name string
		got  color.RGBA
		want color.RGBA
	}{
		{"ColBG", p.BG, ui.ColBG},
		{"ColBGHard", p.BGHard, ui.ColBGHard},
		{"ColBorder", p.Border, ui.ColBorder},
		{"ColAccent", p.Accent, ui.ColAccent},
		{"ColText", p.Text, ui.ColText},
		{"ColDim", p.Dim, ui.ColDim},
		{"ColHead", p.Head, ui.ColHead},
		{"ColGood", p.Good, ui.ColGood},
		{"ColWarn", p.Warn, ui.ColWarn},
		{"ColDanger", p.Danger, ui.ColDanger},
		{"ColInfo", p.Info, ui.ColInfo},
		{"ColShield", p.Shield, ui.ColShield},
		{"ColStruct", p.Struct, ui.ColStruct},
		{"ColCell", p.Cell, ui.ColCell},
		{"ColCap", p.Cap, ui.ColCap},
		{"ColPlayer", p.Player, ui.ColPlayer},
		{"ColHover", p.Hover, ui.ColHover},
		{"ColHeadBg", p.HeadBg, ui.ColHeadBg},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}

	ts := DefaultTypeScale()
	if ts.Base != float64(ui.FontBase) {
		t.Errorf("TypeScale.Base = %v, want ui.FontBase = %v", ts.Base, ui.FontBase)
	}
	if ts.Small != float64(ui.FontSmall) {
		t.Errorf("TypeScale.Small = %v, want ui.FontSmall = %v", ts.Small, ui.FontSmall)
	}
	if ts.Large != float64(ui.FontLarge) {
		t.Errorf("TypeScale.Large = %v, want ui.FontLarge = %v", ts.Large, ui.FontLarge)
	}
	if ts.Tiny != float64(ui.FontTiny) {
		t.Errorf("TypeScale.Tiny = %v, want ui.FontTiny = %v", ts.Tiny, ui.FontTiny)
	}
}

// TestFormattersMatchLegacy pins the duplicated distance/speed formatters
// to ui's, so overview cells cannot drift from the old HUD.
func TestFormattersMatchLegacy(t *testing.T) {
	distances := []float64{-5, 0, 42, 999, 1999, 2000, 12345, 1495999000, 1.496e10, 3e11}
	for _, m := range distances {
		if got, want := formatDistance(m), ui.FormatDistance(m); got != want {
			t.Errorf("formatDistance(%v) = %q, want %q", m, got, want)
		}
	}
	speeds := []float64{-1, 0, 0.4, 127.6, 1234.5, 98765}
	for _, v := range speeds {
		if got, want := formatSpeed(v), ui.FormatSpeed(v); got != want {
			t.Errorf("formatSpeed(%v) = %q, want %q", v, got, want)
		}
	}
}
