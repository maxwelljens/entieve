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

// TestWellBoxIsTheDarkPlate pins the recessed plate (the Show Info icon
// well): the opaque BGHard fill — distinct from the translucent window body
// it is laid on — flat (no depth ramp) at the panel radius.
func TestWellBoxIsTheDarkPlate(t *testing.T) {
	p := DefaultPalette()
	fill, body := WellFill(p), WindowBody(p.BG)
	if fill != Solid(p.BGHard) {
		t.Errorf("WellFill = %v, want the opaque BGHard role %v", fill, Solid(p.BGHard))
	}
	if fill == body {
		t.Errorf("WellFill %v is the window-body fill: the plate would not read apart", fill)
	}
	if fill.A != 255 {
		t.Errorf("WellFill alpha = %d, want the opaque role", fill.A)
	}
	edge := NRGBA(p.Border, 200)
	if w, h := WellBox(p, edge).MinSize(); w != 2*RadiusPanel || h != 2*RadiusPanel {
		t.Errorf("well box min size = %d×%d, want the panel radius box %d×%d", w, h, 2*RadiusPanel, 2*RadiusPanel)
	}
	img := boxRaster(80, 80, fill, edge, AllRadii(RadiusPanel))
	if c := img.NRGBAAt(40, 40); c != fill {
		t.Errorf("well centre fill = %v, want the flat %v (no depth ramp)", c, fill)
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

// TestChromeBoxFillsItsRect pins the box contract every chrome class
// shares: the generated raster IS the piece's rect — the nine-slice cuts
// are cell sizes, not padding bands, so the top and bottom bands read the
// piece's fill edge to edge and only the corner cutouts are transparent.
// A regression here shows as the backdrop through a band where the arcs
// begin (the winchrome capture pins the drawn pixels too).
func TestChromeBoxFillsItsRect(t *testing.T) {
	p := DefaultPalette()
	edge := NRGBA(p.Border, 200)
	body := WindowBody(p.BG)
	panelTop, panelBot := PanelTop(p, body), PanelBottom(p, body)
	cases := []struct {
		name        string
		top, bottom color.NRGBA
		radii       Radii
		fillTop     int
	}{
		{"window title", Solid(p.BGHard), Solid(p.BGHard), TopRadii(RadiusWindow), 0},
		{"window body", panelTop, panelBot, BottomRadii(RadiusWindow), 0},
		{"panel", panelTop, panelBot, AllRadii(RadiusPanel), 0},
		{"control", Solid(p.Cell), Solid(p.Cell), AllRadii(RadiusControl), 0},
		{"banded viewport", body, body, AllRadii(RadiusControl), Px(21)},
	}
	for _, c := range cases {
		img, padX, padY := roundedBoxRaster(c.top, c.bottom, edge, 1, c.radii, c.fillTop)
		b := img.Bounds()
		// The cuts tile the raster exactly: no row or column of padding
		// sits outside the nine cells.
		if padX[0]+padX[1]+padX[2] != b.Dx() || padY[0]+padY[1]+padY[2] != b.Dy() {
			t.Errorf("%s cuts %v×%v do not tile the %d×%d raster", c.name, padX, padY, b.Dx(), b.Dy())
		}
		mid := b.Dx() / 2
		// Corner cutouts: transparent on a rounded corner only, and the
		// shape reaches its rect's edge just past the arc.
		corners := []struct {
			pt image.Point
			r  int
		}{
			{image.Pt(0, 0), c.radii.TL},
			{image.Pt(b.Dx()-1, 0), c.radii.TR},
			{image.Pt(b.Dx()-1, b.Dy()-1), c.radii.BR},
			{image.Pt(0, b.Dy()-1), c.radii.BL},
		}
		for _, k := range corners {
			if a := img.NRGBAAt(k.pt.X, k.pt.Y).A; (k.r > 0) != (a == 0) {
				t.Errorf("%s corner %v alpha = %d, want transparent only where rounded (r=%d)", c.name, k.pt, a, k.r)
			}
		}
		if a := img.NRGBAAt(padX[0], 0).A; a == 0 {
			t.Errorf("%s top edge at x=%d is transparent: the box does not reach its rect's top", c.name, padX[0])
		}
		// The centre column: filled from the first row under the hairline to
		// the last, except the banded viewport's requested blank band.
		floor := min(c.top.A, c.bottom.A) // the fill never dips below its ends
		for y := Px(1); y < b.Dy()-Px(1); y++ {
			got := img.NRGBAAt(mid, y)
			if y < c.fillTop {
				if got.A != 0 {
					t.Errorf("%s band row %d alpha = %d, want 0 (artwork shows through)", c.name, y, got.A)
				}
				continue
			}
			if got.A < floor {
				t.Errorf("%s centre column row %d alpha = %d, want the fill (≥ %d)", c.name, y, got.A, floor)
			}
		}
		// The top band carries the ramp's top colour, the bottom band its
		// bottom end (the hairline keeps the last row, so skip it).
		if c.fillTop == 0 {
			y := max(Px(1), padY[0]-1)
			if got := img.NRGBAAt(mid, y); got != c.top {
				t.Errorf("%s top band row %d = %v, want the top colour %v", c.name, y, got, c.top)
			}
		}
		for y := b.Dy() - padY[2]; y < b.Dy()-Px(1); y++ {
			if got := img.NRGBAAt(mid, y); got != c.bottom {
				t.Errorf("%s bottom band row %d = %v, want the bottom colour %v", c.name, y, got, c.bottom)
			}
		}
		if y := b.Dy() - Px(1) - 1; y >= c.fillTop {
			if got := img.NRGBAAt(mid, y); got != c.bottom {
				t.Errorf("%s last fill row %d = %v, want the bottom colour %v", c.name, y, got, c.bottom)
			}
		}
	}
}

// TestDepthRampDrawsTopToBottom proves the nine-slice's centre row really
// carries the ramp (not a flat fill): the raster's first body row is the
// top end and its last is the bottom end.
func TestDepthRampDrawsTopToBottom(t *testing.T) {
	p := DefaultPalette()
	body := WindowBody(p.BG)
	top, bot := PanelTop(p, body), PanelBottom(p, body)
	img := roundRectImage(40, 300, top, bot, NRGBA(p.Border, 200), 1, scaleRadii(AllRadii(RadiusPanel)), RadiusPanel, 0)
	first := img.NRGBAAt(20, RadiusPanel+1)
	last := img.NRGBAAt(20, 300-RadiusPanel-2)
	if first.R != top.R || first.A != top.A {
		t.Errorf("ramp top row = %v, want %v", first, top)
	}
	if last.R != bot.R || last.A != bot.A {
		t.Errorf("ramp bottom row = %v, want %v", last, bot)
	}
}
