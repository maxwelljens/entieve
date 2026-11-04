package screens

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/internal/capture"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// inventoryCaptureFrames is the settled frame: the open fade is 12 frames,
// and the drag's popup opens on frame 7, so 26 holds the fully painted
// popup over the thumbnail view.
const inventoryCaptureFrames = 38

// TestInventoryCaptureFrame renders the inventory in its thumbnail view
// with the fullness bars, a hull fitted with an ore hold and the player in
// space, then scripted-drags a cargo tile onto the ore section: the PNG
// shows the tile grid, both bars, the hangar's undocked hatch ("Not
// docked") and the transfer popup open on the whole stack.
//
// The pixels the headless suite cannot read back are pinned here: the hatch
// really draws its diagonals (and is gone once docked), the bar's fill is
// the used/capacity ratio and its numbers sit on it, and the popup leads
// with the item's icon. The PNG lands in $ENTIEVE_CAPTURE_DIR (default
// /tmp/opencode).
//
// Skipped unless ENTIEVE_CAPTURE=1; one ebiten loop per process — run it on
// its own:
//
//	ENTIEVE_CAPTURE=1 go test ./uikit/screens/ -run TestInventoryCaptureFrame
func TestInventoryCaptureFrame(t *testing.T) {
	env, _ := newTestEnv(t)
	withOreHold(env, 40)
	inv := NewInventory(env)
	inv.Win().SetLocation(image.Rect(40, 20, 680, 700))
	ui := newQtyUI(t, env, inv.Win())
	q := env.QtyWindow()

	inv.Sync() // rows exist before the loop reads the dragged stack
	cargoUsed, cargoCap, _ := env.Holds().Used("cargo")
	dragItem, _ := inv.cargo.Rows[0].Data.(content.ItemID)
	wantQty := 0
	for _, st := range env.Holds().Cargo() {
		if st.Item == dragItem {
			wantQty += st.Qty
		}
	}
	out := capture.Out("inventory_capture.png")

	(&capture.Capture{
		Name:   "Entieve inventory capture",
		Frames: inventoryCaptureFrames,
		Update: func(n int) error {
			switch n {
			case 4:
				uitest.FireButton(ui, inv.view) // the thumbnail view
			case 12:
				if len(inv.cargoTiles.tiles) == 0 {
					return fmt.Errorf("fixture: no cargo tiles (tiles=%v)", inv.ViewTiles())
				}
				r := inv.cargoTiles.tiles[0].GetWidget().Rect
				env.in.x, env.in.y = r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
				env.in.edge, env.in.down = [3]bool{true}, [3]bool{true}
			case 13:
				r := inv.oreBody.GetWidget().Rect
				env.in.x, env.in.y = r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
				env.in.edge = [3]bool{}
			case 14:
				env.in.down = [3]bool{}
			case 16:
				// Out from under the inventory (the bare session's picker
				// opens at its fallback rect, not centred like the kit's).
				q.Win().SetLocation(image.Rect(700, 260, 1160, 492))
			}
			env.Pointer().Begin()
			inv.Sync()
			q.Sync()
			ui.Update()
			return nil
		},
		Draw: func(_ int, img *ebiten.Image) error {
			img.Fill(color.White)
			ui.Draw(img)
			return nil
		},
		Verify: func(img *ebiten.Image) error {
			var errs []string
			fail := func(format string, args ...any) {
				errs = append(errs, fmt.Sprintf(format, args...))
			}
			if !inv.ViewTiles() {
				fail("the inventory is not in its thumbnail view")
			}
			if len(inv.cargoTiles.tiles) != len(env.Holds().Cargo()) {
				fail("cargo tiles = %d, want one per stack (%d)", len(inv.cargoTiles.tiles), len(env.Holds().Cargo()))
			}
			// The tile view draws: icon and name pixels inside the first
			// tile (an empty tile grid would be a silent blank area).
			if n := uitest.InkIn(img, inv.cargoTiles.tiles[0].GetWidget().Rect); n == 0 {
				fail("first cargo tile drew no pixels")
			}

			// The bars: fill width is used/capacity and the figures sit on
			// the bar (pixel probes at the fill's two ends, clear of the
			// centred text).
			bar := inv.cargoBar
			inner := bar.Bar.GetWidget().Rect.Inset(theme.Px(2))
			frac := cargoUsed / cargoCap
			if want := int(float64(inner.Dx()) * frac); bar.FillRect().Dx() != want {
				fail("cargo fill = %d px, want %d (%.3f of %d)", bar.FillRect().Dx(), want, frac, inner.Dx())
			}
			if got, want := bar.Value.Label, theme.FormatVolumeShort(cargoUsed)+" / "+theme.FormatVolumeShort(cargoCap); got != want {
				fail("cargo bar text = %q, want %q", got, want)
			}
			at := func(x int) color.RGBA {
				r, g, b, _ := img.At(x, inner.Min.Y+inner.Dy()/2).RGBA()
				return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255}
			}
			accent := env.Pal().Accent
			if got := at(inner.Min.X + 1); !near(got, accent) {
				fail("cargo bar's fill pixel = %v, want the accent %v", got, accent)
			}
			if fw := bar.FillRect().Dx(); fw > 0 && fw < inner.Dx()-4 {
				if got := at(inner.Max.X - 2); near(got, accent) {
					fail("cargo bar's track end = %v, want no accent past the fill", got)
				}
			}

			// The undocked hangar: hatch lines in the border role and the
			// "Not docked" plate's bright text over them.
			band := inv.hangBody.GetWidget().Rect.Inset(theme.Px(6))
			if !inv.hatchShown() {
				fail("undocked hangar is not the hatch")
			}
			undocked := hatchPixels(img, band)
			if undocked < 200 {
				fail("hangar band has %d hatch pixels, want the diagonals", undocked)
			}
			if n := uitest.InkIn(img, inv.hatch.text.GetWidget().Rect); n == 0 {
				fail("the hatch's %q text drew no pixels", hatchLabel)
			}

			// The popup: open on the whole stack, the item's mark and name,
			// the destination named.
			if !q.IsOpen() {
				fail("the drag opened no transfer popup (dragging=%v, src=%s)", inv.dragging, inv.src)
			} else if q.Value() != wantQty {
				fail("popup quantity = %d, want the stack (%d)", q.Value(), wantQty)
			}
			if q.mark.Widget().GetWidget().GetVisibility() != widget.Visibility_Show {
				fail("popup shows no item icon")
			}
			if line := q.infoLine(); !strings.Contains(line, env.TradeableName(dragItem)) || !strings.Contains(line, "Ore") {
				fail("popup line = %q, want the item and destination", line)
			}

			// The drag chip follows the pointer PIXEL for pixel: two move
			// frames inside one target (no highlight flip to re-lay the
			// overlay), and the drawn chip's accent-border centroid must
			// move by exactly the pointer delta. Rendered into scratch
			// frames; the saved PNG stays the earlier shot.
			tile := inv.cargoTiles.tiles[0].GetWidget().Rect
			env.in.x, env.in.y = tile.Min.X+tile.Dx()/2, tile.Min.Y+tile.Dy()/2
			env.in.edge, env.in.down = [3]bool{true}, [3]bool{true}
			env.Pointer().Begin()
			inv.Sync()
			ui.Update()
			ore := inv.oreBody.GetWidget().Rect
			p1 := image.Pt(ore.Min.X+theme.Px(20), ore.Max.Y-theme.Px(20))
			delta := image.Pt(theme.Px(120), 0)
			chip := func(p image.Point) (int, image.Point) {
				env.in.x, env.in.y = p.X, p.Y
				env.in.edge = [3]bool{}
				env.Pointer().Begin()
				inv.Sync()
				q.Sync()
				ui.Update()
				shot := ebiten.NewImage(1280, 720)
				ui.Draw(shot)
				return accentInk(shot, ghostBox(inv, p), env.Pal().Accent)
			}
			n1, c1 := chip(p1)
			n2, c2 := chip(p1.Add(delta))
			env.in.down = [3]bool{}
			if n1 < 100 || n2 < 100 {
				fail("the drag chip's border is not drawn where the pointer is (%d/%d accent px)", n1, n2)
			} else if got := c2.Sub(c1); got != delta {
				fail("the drag chip's ink moved %v, want the pointer delta %v", got, delta)
			}

			// Docking swaps the hatch for the table: the same band's
			// diagonal lines are gone. Rendered into a scratch frame so the
			// saved PNG stays the undocked shot.
			dockAtStation(t, env.Sim)
			scratch := ebiten.NewImage(1280, 720)
			for i := 0; i < 3; i++ {
				inv.Sync()
				ui.Update()
				scratch.Fill(color.White)
				ui.Draw(scratch)
			}
			if inv.hatchShown() {
				fail("docked hangar still shows the hatch")
			}
			if docked := hatchPixels(scratch, band); docked > undocked/3 {
				fail("docked hangar band still has %d hatch pixels (undocked %d)", docked, undocked)
			}

			if len(errs) > 0 {
				return fmt.Errorf("capture %s: %s", out, strings.Join(errs, "; "))
			}
			return nil
		},
		Out: out,
	}).Run(t)
}

// near reports whether two colours are within a few levels per channel
// (the drawn nine-slice's edges antialias).
func near(a, b color.RGBA) bool {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(a.R, b.R) < 12 && d(a.G, b.G) < 12 && d(a.B, b.B) < 12
}

// hatchPixels counts the band's border-role pixels: the hatch's hairlines
// (the palette's border over the window body), clear of glyph inks.
func hatchPixels(img *ebiten.Image, band image.Rectangle) int {
	n := 0
	for y := band.Min.Y; y < band.Max.Y; y++ {
		for x := band.Min.X; x < band.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			sum := int(r>>8) + int(g>>8) + int(b>>8)
			if sum > 90 && sum < 320 {
				n++
			}
		}
	}
	return n
}

// accentInk counts the accent pixels inside box and returns their centroid:
// the drag chip's border is the only accent ink drawn there, so its centroid
// is the chip's drawn centre (a stale chip cannot pass the tracking pin).
func accentInk(img *ebiten.Image, box image.Rectangle, accent color.RGBA) (int, image.Point) {
	near := func(v uint32, want uint8) bool {
		x := int(v>>8) - int(want)
		return x > -24 && x < 24
	}
	sx, sy, n := 0, 0, 0
	for y := max(box.Min.Y, 0); y < min(box.Max.Y, img.Bounds().Dy()); y++ {
		for x := max(box.Min.X, 0); x < min(box.Max.X, img.Bounds().Dx()); x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			if near(r, accent.R) && near(g, accent.G) && near(b, accent.B) {
				sx += x
				sy += y
				n++
			}
		}
	}
	if n == 0 {
		return 0, image.Point{}
	}
	return n, image.Pt(sx/n, sy/n)
}
