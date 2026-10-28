package screens

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/internal/capture"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// iconsCaptureFrames is how many full draws run before the screenshot is
// taken (anchors and layout settle on the first frames).
// 14 draws: the kit windows open with the 12-frame fade — captures
// pin the settled state, never a mid-fade one (winanim.go).
const iconsCaptureFrames = 14

// TestIconsCaptureFrame renders the three icon-bearing core surfaces —
// overview (kind column), Selected Item (kind glyph + action buttons) and
// the module rack (module icons) — through a real ebiten game loop, checks
// that the glyphs actually landed on pixels (ink in each box, the tile
// glyph centred in its tile) and screenshots the frame.
//
// Skipped unless ENTIEVE_CAPTURE=1 so the regular suite stays windowless.
// The PNG lands in $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
func TestIconsCaptureFrame(t *testing.T) {
	k, _ := newTestKit(t)
	// Select a warpable landmark so the Selected Item header shows its
	// kind glyph and the action buttons are enabled.
	for _, info := range k.Sim.Grid() {
		if info.CanWarp {
			k.Select(info.Entity)
			break
		}
	}
	r := NewRack(k.Env())
	k.AddContainer(r.Container(), widget.AnchorLayoutData{
		HorizontalPosition: widget.AnchorLayoutPositionEnd,
		VerticalPosition:   widget.AnchorLayoutPositionEnd,
		Padding:            &widget.Insets{Right: 10, Bottom: 10},
	})

	out := capture.Out("icons_capture.png")
	(&capture.Capture{
		Name:   "Entieve icons capture",
		Frames: iconsCaptureFrames,
		Update: func(int) error {
			k.Sync()
			r.Sync()
			return k.Update()
		},
		Draw: func(_ int, img *ebiten.Image) error {
			k.Draw(img)
			return nil
		},
		Verify: func(img *ebiten.Image) error {
			// Every failure is collected so one run reports all of them.
			var errs []string
			fail := func(format string, args ...any) {
				errs = append(errs, fmt.Sprintf(format, args...))
			}

			// --- 1. Overview kind column: the first row's icon box has ink.
			tbl := k.ov.tbl
			if len(tbl.Cells) == 0 || tbl.Cells[0][2] == nil {
				fail("overview has no kind icon cells (rows=%d)", len(tbl.Cells))
			} else {
				box := tbl.Cells[0][2].Box.Widget().GetWidget().Rect
				if box.Dx() != theme.IconInline {
					fail("overview kind icon box = %v, want %d px square", box, theme.IconInline)
				}
				if ink := uitest.InkIn(img, box); ink < 8 {
					fail("overview kind icon box %v has %d ink pixels, want ≥ 8", box, ink)
				}
				label := tbl.Texts[0][2].GetWidget().Rect
				if box.Overlaps(label) {
					fail("overview icon %v overlaps its label %v", box, label)
				}
			}

			// --- 2. Selected Item header: the kind glyph beside the name.
			if k.sp.kindIcon.Name() == "" {
				fail("selected header shows no kind glyph (selected=%q)", k.sp.name.Label)
			} else if box := k.sp.kindIcon.Widget().GetWidget().Rect; uitest.InkIn(img, box) < 8 {
				fail("selected header glyph box %v has too little ink", box)
			}

			// --- 3. Action buttons: every enabled action button carries its 20 px
			// glyph (ink over the button face, inside the button rect).
			for _, b := range []*widget.Button{
				k.sp.approach, k.sp.warp, k.sp.orbit, k.sp.stop,
				k.sp.lock, k.sp.dock, k.sp.jump, k.sp.loot, k.sp.mine,
			} {
				if b.GraphicImage() == nil || b.GraphicImage().Idle == nil {
					fail("%q button has no glyph", b.Text().Label)
					continue
				}
				r := b.GetWidget().Rect
				if ink := uitest.InkIn(img, r); ink < 20 {
					fail("%q button %v drew only %d bright pixels", b.Text().Label, r, ink)
				}
			}

			// --- 4. Rack tiles: a fitted cell's icon (art or class glyph) sits
			// centred in its tile. The tile body is flat, so the icon is whatever
			// differs from the body colour sampled in a corner the icon can never
			// reach (it is 56 px centred in a 62×62 tile): disabled tiles draw at
			// 40 % and would fail an absolute brightness threshold.
			// The tile's inner ink box must be centred on the tile to within a
			// couple of pixels — a glyph hung off one side (the graphic-row
			// centering defect) reads 5 px off and fails here.
			checked := 0
			for row := range r.cells {
				for _, cell := range r.cells[row] {
					if cell.icon == "" || cell.root.GetWidget().GetVisibility() != widget.Visibility_Show {
						continue
					}
					tile := cell.btn.GetWidget().Rect
					// The waiting badge (top-right dot) and ready marker
					// (bottom-right shell) are designed corner ink — exclude
					// their footprints so only the glyph moves the centroid.
					var excl []image.Rectangle
					if cell.ammoGfx.GetWidget().GetVisibility() == widget.Visibility_Show {
						ox, oy := rackAmmoOrigin()
						s := theme.Px(rackAmmoIcon)
						excl = append(excl, image.Rect(tile.Min.X+ox, tile.Min.Y+oy,
							tile.Min.X+ox+s, tile.Min.Y+oy+s))
					}
					if cell.badgeGfx.GetWidget().GetVisibility() == widget.Visibility_Show {
						r := theme.Px(rackBadgeR)
						cx := tile.Max.X - theme.Px(rackBadgeInset)
						cy := tile.Min.Y + theme.Px(rackBadgeInset)
						excl = append(excl, image.Rect(cx-r, cy-r, cx+r, cy+r))
					}
					bx, n := uitest.GlyphBounds(img, tile, excl...)
					if n < 8 {
						fail("rack tile %v has %d ink pixels for %s, want a drawn glyph",
							tile, n, cell.icon)
						checked++
						continue
					}
					if dx := (bx.Min.X+bx.Max.X)/2 - (tile.Min.X+tile.Max.X)/2; uitest.AbsInt(dx) > 4 {
						fail("rack tile %v glyph centre is %d px off (icon %s)", tile, dx, cell.icon)
					}
					if dy := (bx.Min.Y+bx.Max.Y)/2 - (tile.Min.Y+tile.Max.Y)/2; uitest.AbsInt(dy) > 4 {
						fail("rack tile %v glyph is %d px off vertically (icon %s)", tile, dy, cell.icon)
					}
					checked++
				}
			}
			if checked == 0 {
				fail("no fitted rack cells to check")
			}

			if len(errs) > 0 {
				return fmt.Errorf("capture %s: %s", out, strings.Join(errs, "; "))
			}
			return nil
		},
		Out: out,
	}).Run(t)
}
