package theme

import (
	"image"
	"image/color"
	"testing"
)

// Rounded chrome pins: the token radii, the per-corner masks the window
// pieces need, and the panel-depth gradient's size. Pixel evidence comes
// from the rasteriser directly — ebiten image readback needs a game loop,
// which the capture gate owns.

func TestRadiusTokens(t *testing.T) {
	if RadiusWindow != 14 || RadiusPanel != 8 || RadiusControl != 5 {
		t.Errorf("radius tokens = %d/%d/%d, want 14/8/5",
			RadiusWindow, RadiusPanel, RadiusControl)
	}
	if TopRadii(14) != (Radii{14, 14, 0, 0}) {
		t.Errorf("TopRadii = %v, want top-only", TopRadii(14))
	}
	if BottomRadii(14) != (Radii{0, 0, 14, 14}) {
		t.Errorf("BottomRadii = %v, want bottom-only", BottomRadii(14))
	}
}

// boxRaster renders the box every chrome class generates, so the corner
// and gradient pins read the same pixels the nine-slice carries.
func boxRaster(w, h int, body, edge color.NRGBA, radii Radii) *image.NRGBA {
	return roundRectImage(w, h, body, body, edge, 1, scaleRadii(radii), 0, 0)
}

func TestWindowPiecesArePerCornerRounded(t *testing.T) {
	p := DefaultPalette()
	body := WindowBody(p.BG)
	edge := NRGBA(p.Border, 200)

	// Title bar: TOP corners rounded, bottom square (the seam).
	top := boxRaster(80, 44, Solid(p.BGHard), edge, TopRadii(RadiusWindow))
	if a := top.NRGBAAt(0, 0).A; a != 0 {
		t.Errorf("title bar top-left pixel alpha = %d, want 0 (rounded away)", a)
	}
	if a := top.NRGBAAt(1, 1).A; a != 0 {
		t.Errorf("title bar 1,1 alpha = %d, want 0 (inside the 14 px arc)", a)
	}
	if c := top.NRGBAAt(0, 43); c.A < 200 {
		t.Errorf("title bar bottom-left = %v, want the square border hairline", c)
	}

	// Window body: BOTTOM corners rounded, top square.
	bot := boxRaster(80, 120, body, edge, BottomRadii(RadiusWindow))
	if a := bot.NRGBAAt(0, 0).A; a < 200 {
		t.Errorf("body top-left alpha = %d, want the square hairline (seam)", a)
	}
	if a := bot.NRGBAAt(0, 119).A; a != 0 {
		t.Errorf("body bottom-left alpha = %d, want 0 (rounded away)", a)
	}

	// Panel/card: all four corners at 8.
	panel := boxRaster(80, 60, body, edge, AllRadii(RadiusPanel))
	for _, pt := range []image.Point{{0, 0}, {79, 0}, {0, 59}, {79, 59}} {
		if a := panel.NRGBAAt(pt.X, pt.Y).A; a != 0 {
			t.Errorf("panel corner %v alpha = %d, want 0", pt, a)
		}
	}
	// The fill inside the arc is the body, not the border.
	if c := panel.NRGBAAt(20, 30); c.A != body.A {
		t.Errorf("panel centre fill = %v, want the body at alpha %d", c, body.A)
	}
}

// TestChromeClassMinSizes pins the padding each class reserves: the
// nine-slice's corner cells (2·radius) and the 1 px window seam strip.
func TestChromeClassMinSizes(t *testing.T) {
	p := DefaultPalette()
	edge := NRGBA(p.Border, 200)
	cases := []struct {
		name string
		img  interface{ MinSize() (int, int) }
		w, h int
	}{
		{"control", Bordered(Solid(p.Cell), edge, 1), 2 * RadiusControl, 2 * RadiusControl},
		{"panel", PanelBox(p, WindowBody(p.BG), edge), 2 * RadiusPanel, 2 * RadiusPanel},
		{"window title", WindowTitleBox(Solid(p.BGHard), edge), 2 * RadiusWindow, RadiusWindow + 1},
		{"window body", WindowBodyBox(p, WindowBody(p.BG), edge), 2 * RadiusWindow, RadiusWindow + 1},
	}
	for _, c := range cases {
		w, h := c.img.MinSize()
		if w != c.w || h != c.h {
			t.Errorf("%s min size = %d×%d, want %d×%d", c.name, w, h, c.w, c.h)
		}
	}
}

// TestPanelDepthToken pins the panel gradient's size: a depth cue inside
// the 230 WindowAlpha token, not a colour change.
func TestPanelDepthToken(t *testing.T) {
	p := DefaultPalette()
	body := WindowBody(p.BG)
	top, bot := PanelTop(p, body), PanelBottom(p, body)
	if top.R == body.R && top.G == body.G && top.B == body.B {
		t.Errorf("PanelTop = %v, want the body lifted toward HeadBg", top)
	}
	for _, d := range []int{int(top.R) - int(body.R), int(top.G) - int(body.G), int(top.B) - int(body.B)} {
		if d < 0 || d > 6 {
			t.Errorf("PanelTop delta = %d, want 0..6 (subtle)", d)
		}
	}
	if top.B > p.HeadBg.B || top.R > p.HeadBg.R || top.G > p.HeadBg.G {
		t.Errorf("PanelTop %v overshoots HeadBg %v", top, p.HeadBg)
	}
	if bot.A < body.A || bot.A-body.A > 4 || body.A-top.A > 4 {
		t.Errorf("depth alpha = %d→%d around %d, want ±4", top.A, bot.A, body.A)
	}
	// The token stays the ramp's midpoint.
	if (int(top.A)+int(bot.A)+1)/2 != WindowAlpha {
		t.Errorf("depth midpoint = %d, want WindowAlpha %d", (int(top.A)+int(bot.A)+1)/2, WindowAlpha)
	}
}

// TestDepthRampDrawsTopToBottom proves the nine-slice's centre row really
// carries the ramp (not a flat fill): the raster's first body row is the
// top end and its last is the bottom end.
func TestDepthRampDrawsTopToBottom(t *testing.T) {
	p := DefaultPalette()
	body := WindowBody(p.BG)
	top, bot := PanelTop(p, body), PanelBottom(p, body)
	img := roundRectImage(40, 300, top, bot, NRGBA(p.Border, 200), 1, scaleRadii(AllRadii(RadiusPanel)), RadiusPanel, RadiusPanel)
	first := img.NRGBAAt(20, RadiusPanel+1)
	last := img.NRGBAAt(20, 300-RadiusPanel-2)
	if first.R != top.R || first.A != top.A {
		t.Errorf("ramp top row = %v, want %v", first, top)
	}
	if last.R != bot.R || last.A != bot.A {
		t.Errorf("ramp bottom row = %v, want %v", last, bot)
	}
}
