package screens

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/uikit/hulls"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// dropHull writes a solid 8×8 marker PNG at assets/hulls/<id>.png under a
// fresh temp root and points the sprite registry at it for the rest of the
// test (an empty load afterwards, so no test inherits the dropped art).
func dropHull(t *testing.T, id string, w, h int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hulls", id+".png")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 64, B: 32, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode: %v", err)
	}
	f.Close()
	render.LoadAssets(filepath.Dir(filepath.Dir(path)))
	t.Cleanup(func() { render.LoadAssets("") })
}

// TestHullArtDropInWins: a dropped assets/hulls/<id>.png is the portrait
// every hull-art surface draws (the HUD, the Show Info hull page and the
// fitting window all go through hullArt); with no file the embedded raster
// returns unchanged, and a hull with neither answers nil.
// Only Bounds is compared on the dropped sprite: ebiten pixel readback needs
// a running game loop.
func TestHullArtDropInWins(t *testing.T) {
	builtin := hulls.Get("ship-vastrek", 120)
	if builtin == nil {
		t.Fatal("fixture: ship-vastrek has no embedded portrait")
	}
	if got := hullArt("ship-vastrek", 120, 120); got.Bounds() != render.InkCrop(builtin).Bounds() {
		t.Fatalf("no assets: hullArt bounds = %v, want the embedded raster's ink %v", got.Bounds(), render.InkCrop(builtin).Bounds())
	}
	if c := color.NRGBAModel.Convert(builtin.At(0, 0)).(color.NRGBA); c.A != 0 {
		t.Errorf("built-in corner = %v, want transparent (the 1 px margin)", c)
	}

	dropHull(t, "ship-vastrek", 8, 8)
	if got := hullArt("ship-vastrek", 120, 120); got.Bounds() != image.Rect(0, 0, 8, 8) {
		t.Errorf("dropped art: hullArt = %v, want the 8×8 drop-in over the %v raster", got.Bounds(), builtin.Bounds())
	}
	if got, want := hullArt("ship-zdanek", 120, 120), render.InkCrop(hulls.Get("ship-zdanek", 120)); got.Bounds() != want.Bounds() {
		t.Errorf("a hull without a file = %v, want the embedded raster's ink %v", got.Bounds(), want.Bounds())
	}
	if got := hullArt("ship-kutrek", 120, 120); got != nil {
		t.Errorf("ship-kutrek (no embedded raster, no file) = %v, want nil", got)
	}
}

// TestHullFitContains: the portrait blocks hold the art whole. A 1376×768
// canvas (the owner's own) in the 2:1 blocks pads above and below; a square
// raster sits centred; nothing is ever drawn past the box, and a degenerate
// input draws nothing.
func TestHullFitContains(t *testing.T) {
	for _, c := range []struct {
		name           string
		sw, sh, bw, bh int
		dw, dh         int
	}{
		{"canvas in the info block", 1376, 768, 200, 100, 179, 100},
		{"canvas in the HUD box", 1376, 768, 120, 60, 108, 60},
		{"square raster in a 2:1 box", 120, 120, 200, 100, 100, 100},
		{"small drop-in blown up", 8, 8, 120, 60, 60, 60},
		{"degenerate", 0, 10, 120, 60, 0, 0},
	} {
		dw, dh := hullFit(c.sw, c.sh, c.bw, c.bh)
		if int(dw+0.5) != c.dw || int(dh+0.5) != c.dh {
			t.Errorf("%s: fit = %.1f×%.1f, want %d×%d", c.name, dw, dh, c.dw, c.dh)
		}
		if dw > float64(c.bw)+0.001 || dh > float64(c.bh)+0.001 {
			t.Errorf("%s: fit %.1f×%.1f overflows the %d×%d box (cover crops)", c.name, dw, dh, c.bw, c.bh)
		}
	}
}

// TestHullArtSamplesTheFrame: the owner's hull canvases are 1376×768 and every
// portrait block is 60–200 px, so hullArt answers the mip level covering the
// block instead of the full file — the chain floors at 32 px (render/mip.go).
// A hull with no drop-in keeps the embedded raster whole: both sources work.
func TestHullArtSamplesTheFrame(t *testing.T) {
	const id = content.ShipID("ship-owner-shot")
	dropHull(t, string(id), 1376, 768)
	full := render.HullSprite(id)
	if b := full.Bounds(); b.Dx() != 1376 || b.Dy() != 768 {
		t.Fatalf("dropped hull = %v, want the 1376×768 canvas", b)
	}
	for _, box := range [][2]int{
		{hudPortraitW, hudPortraitH},
		{infoHullW, infoHullH},
		{fitPortraitW, fitPortraitH},
	} {
		got := hullArt(id, box[0], box[1])
		if got == nil {
			t.Fatalf("no art for %s in a %d×%d box", id, box[0], box[1])
		}
		b := got.Bounds()
		// The frame the art fills is the contained fit of the box.
		fw, fh := hullFit(1376, 768, box[0], box[1])
		if b.Dx() >= 1376 && b.Dy() >= 768 {
			t.Errorf("%d×%d box sampled the full canvas %v, want a mip level", box[0], box[1], b)
		}
		if b.Dx() < int(fw) || b.Dy() < int(fh) {
			t.Errorf("%d×%d box sampled %v, under-covering the %.0f×%.0f frame", box[0], box[1], b, fw, fh)
		}
		if b.Dx() > 2*int(fw) || b.Dy() > 2*int(fh) {
			t.Errorf("%d×%d box sampled %v, more than 2× the %.0f×%.0f frame (texels stretched past 2×)",
				box[0], box[1], b, fw, fh)
		}
	}
	// The built-in source: no file for ship-zdanek, so the committed raster
	// answers whole (it is already small; nothing to sample), cropped to its
	// ink like the dropped art — at the box's longest side.
	builtin := render.InkCrop(hulls.Get("ship-zdanek", max(hudPortraitW, hudPortraitH)))
	if got := hullArt("ship-zdanek", hudPortraitW, hudPortraitH); got.Bounds() != builtin.Bounds() {
		t.Errorf("built-in hull in the HUD box = %v, want the raster's ink %v", got.Bounds(), builtin.Bounds())
	}
}

// TestInfoWindowHullPageLeadsWithTheShip: the Show Info hull page opens on
// the hull ITSELF, a 2:1 wide block (infoHullW×infoHullH) on its dark plate
// beside the title — the one page whose hero is a picture of the thing, not
// its icon. Pinned both ways: a module page keeps the square hero token, so
// "the ship window reads differently" cannot drift in either direction. The
// block is drawn through hullArt (drop-in or raster), so its box is the same
// shape either way.
func TestInfoWindowHullPageLeadsWithTheShip(t *testing.T) {
	env, _ := newChromeEnv(t)
	w := env.InfoWindow()
	ui := newWindowUI(t, env, w.Win())

	check := func(name string) {
		t.Helper()
		hero, plate := w.Hero(), w.HeroPlate()
		if hero == nil || plate == nil {
			t.Fatalf("%s: hull page has no hero block", name)
		}
		if b := hero.Image.Bounds(); b.Dx() != theme.Px(infoHullW) || b.Dy() != theme.Px(infoHullH) {
			t.Errorf("%s: hero = %v, want the %d×%d ship block", name, b, theme.Px(infoHullW), theme.Px(infoHullH))
		} else if b.Dx() != 2*b.Dy() {
			t.Errorf("%s: hero %v is not 2:1 wide", name, b)
		}
		box := plate.GetWidget().Rect
		if wantW, wantH := theme.Px(infoHullW)+2*theme.Px(infoPlatePad), theme.Px(infoHullH)+2*theme.Px(infoPlatePad); box.Dx() != wantW || box.Dy() != wantH {
			t.Errorf("%s: plate = %v, want %d×%d (the block plus its padding)", name, box, wantW, wantH)
		}
		_, col := headerRow(t, w)
		if col.GetWidget().Rect.Min.X < box.Max.X {
			t.Errorf("%s: title column %v starts over the ship block %v", name, col.GetWidget().Rect, box)
		}
	}

	// No drop-in: the committed raster fills the same 2:1 block.
	if !w.Show(widgets.ShipRef("ship-vastrek")) {
		t.Fatal("ship-vastrek did not resolve")
	}
	uitest.PumpWindow(t, ui, w.Sync, 1024, 700)
	check("embedded raster")

	// A dropped 1376×768 canvas: the block does not change shape, and the
	// portrait seam now answers a mip level (hullArt, pinned above).
	dropHull(t, "ship-vastrek", 1376, 768)
	if !w.Show(widgets.ShipRef("ship-vastrek")) {
		t.Fatal("ship-vastrek did not resolve after the drop")
	}
	uitest.PumpWindow(t, ui, w.Sync, 1024, 700)
	check("dropped canvas")

	// A hull with neither has no picture to show: the square icon plate
	// stays (never an empty wide box).
	if !w.Show(widgets.ShipRef("ship-kutrek")) {
		t.Fatal("ship-kutrek did not resolve")
	}
	uitest.PumpWindow(t, ui, w.Sync, 1024, 700)
	if hero, plate := w.Hero(), w.HeroPlate(); hero == nil || plate == nil {
		t.Fatal("a hull with no art lost its hero plate")
	} else if b := hero.Image.Bounds(); b.Dx() != theme.Px(theme.IconHero) || b.Dy() != theme.Px(theme.IconHero) {
		t.Errorf("hull with no art: hero = %v, want the square %d icon", b, theme.Px(theme.IconHero))
	} else if got := plate.GetWidget().Rect.Dx(); got != theme.Px(infoPlateW()) {
		t.Errorf("hull with no art: plate = %d wide, want the icon plate %d", got, theme.Px(infoPlateW()))
	}

	// The other way: a module page keeps the square hero token on the icon
	// plate, never the wide ship block.
	if !w.Show(widgets.ModuleRef("mod-autocannon-s")) {
		t.Fatal("mod-autocannon-s did not resolve")
	}
	uitest.PumpWindow(t, ui, w.Sync, 1024, 700)
	hero, plate := w.Hero(), w.HeroPlate()
	if hero == nil || plate == nil {
		t.Fatal("module page lost its hero")
	}
	if b := hero.Image.Bounds(); b.Dx() != theme.Px(theme.IconHero) || b.Dy() != theme.Px(theme.IconHero) {
		t.Errorf("module hero = %v, want the square %d icon", b, theme.Px(theme.IconHero))
	}
	if got, want := plate.GetWidget().Rect.Dx(), theme.Px(infoPlateW()); got != want {
		t.Errorf("module plate = %d wide, want the icon plate %d", got, want)
	}
}

// TestHullArtFitsTheInkNotTheCanvas: the portrait blocks fit the ART, not the
// transparent canvas it sits in. A 160×64 canvas with a 120×25 ship band (the
// owner's ~4.8:1 drop-ins) draws the ship the HUD box's full 240 px wide by
// 50 tall, and the committed rasters do the same on the axis they bind: their
// ink fills the frame's short side (a squarer ship is height-bound).
func TestHullArtFitsTheInkNotTheCanvas(t *testing.T) {
	const id = content.ShipID("ship-owner-margin")
	path := filepath.Join(t.TempDir(), "hulls", string(id)+".png")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 160, 64))
	for y := 20; y < 45; y++ {
		for x := 20; x < 140; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()
	render.LoadAssets(filepath.Dir(filepath.Dir(path)))
	t.Cleanup(func() { render.LoadAssets("") })

	got := hullArt(id, hudPortraitW, hudPortraitH)
	if got == nil {
		t.Fatal("no art for the dropped canvas")
	}
	if b := got.Bounds(); b.Dx() != 120 || b.Dy() != 25 {
		t.Errorf("dropped art = %v, want its 120×25 ink (the load crops the canvas)", b)
	}
	dw, dh := hullFit(120, 25, hudPortraitW, hudPortraitH)
	if int(dw+0.5) != hudPortraitW {
		t.Errorf("ink drawn %.1f wide, want the box's %d (a ship wider than the box's 4:1 fills the width)", dw, hudPortraitW)
	}
	if int(dh+0.5) != 50 {
		t.Errorf("ink drawn %.1f tall, want 50 (aspect preserved, nothing cropped)", dh)
	}

	// The committed raster: ink ~1.78:1 in the 240×60 box -> ~107×60, still
	// filling the height (was 109×60 in the old 120×60 box). A squarer ship
	// is height-bound, so only the drop-in art grew with the frame.
	in := render.InkCrop(hulls.Get("ship-vastrek", max(hudPortraitW, hudPortraitH))).Bounds()
	bw, bh := hullFit(in.Dx(), in.Dy(), hudPortraitW, hudPortraitH)
	if int(bh+0.5) != hudPortraitH {
		t.Errorf("raster ink drawn %.1f tall, want the box's %d", bh, hudPortraitH)
	}
	if int(bw+0.5) >= hudPortraitW {
		t.Errorf("raster ink drawn %.1f wide, want the height-bound fit under the box's %d", bw, hudPortraitW)
	}
}
