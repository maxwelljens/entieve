package screens

import (
	"fmt"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
)

// iconsCaptureFrames is how many full draws run before the screenshot is
// taken (anchors and layout settle on the first frames).
// 13 draws: the kit windows open with the 12-frame fade — captures
// pin the settled state, never a mid-fade one (winanim.go).
const iconsCaptureFrames = 13

// TestIconsCaptureFrame renders the three icon-bearing core surfaces —
// overview (kind column), Selected Item (kind glyph + action buttons) and
// the module rack (class tiles) — through a real ebiten game loop, checks
// that the glyphs actually landed on pixels (ink in each box, the tile
// glyph centred in its tile) and screenshots the frame.
//
// Skipped unless ENTIEVE_CAPTURE=1 so the regular suite stays windowless.
// The PNG lands in $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
func TestIconsCaptureFrame(t *testing.T) {
	if os.Getenv("ENTIEVE_CAPTURE") == "" {
		t.Skip("set ENTIEVE_CAPTURE=1 to render a capture frame")
	}
	uitest.SkipIfLoopRan(t)
	outDir := os.Getenv("ENTIEVE_CAPTURE_DIR")
	if outDir == "" {
		outDir = "/tmp/opencode"
	}

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

	g := &iconsCaptureGame{k: k, rack: r, out: filepath.Join(outDir, "icons_capture.png")}
	uitest.RunCaptureLoop(t, "Entieve icons capture", g)
	if g.err != nil {
		t.Fatal(g.err)
	}
	if !g.saved {
		t.Fatal("capture never rendered")
	}
	t.Logf("capture written to %s", g.out)
}

// iconsCaptureGame drives kit + rack for a few frames, then screenshots
// and verifies the drawn glyphs.
type iconsCaptureGame struct {
	k      *Kit
	rack   *Rack
	out    string
	frames int
	last   *ebiten.Image
	saved  bool
	err    error
}

func (g *iconsCaptureGame) Update() error {
	if g.saved || g.err != nil {
		return ebiten.Termination
	}
	g.k.Sync()
	g.rack.Sync()
	if err := g.k.Update(); err != nil {
		g.err = err
		return ebiten.Termination
	}
	if g.frames > iconsCaptureFrames && g.last != nil {
		g.verify(g.last)
		return ebiten.Termination
	}
	return nil
}

func (g *iconsCaptureGame) Draw(screen *ebiten.Image) {
	img := ebiten.NewImage(1280, 720)
	g.k.Draw(img)
	screen.DrawImage(img, nil)
	g.last = img
	g.frames++ // frames count DRAWS: ebiten runs Updates on their own cadence (see rail_capture_test.go)
}

func (g *iconsCaptureGame) Layout(w, h int) (int, int) { return 1280, 720 }

// verify screenshots the frame and asserts the drawn glyphs. Every failure
// is collected so one run reports all of them.
func (g *iconsCaptureGame) verify(img *ebiten.Image) {
	var errs []string
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Sprintf(format, args...))
	}

	// --- 1. Overview kind column: the first row's icon box has ink.
	tbl := g.k.ov.tbl
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
	if g.k.sp.kindIcon.Name() == "" {
		fail("selected header shows no kind glyph (selected=%q)", g.k.sp.name.Label)
	} else if box := g.k.sp.kindIcon.Widget().GetWidget().Rect; uitest.InkIn(img, box) < 8 {
		fail("selected header glyph box %v has too little ink", box)
	}

	// --- 3. Action buttons: every enabled action button carries its 20 px
	// glyph (ink over the button face, inside the button rect).
	for _, b := range []*widget.Button{
		g.k.sp.approach, g.k.sp.warp, g.k.sp.orbit, g.k.sp.stop,
		g.k.sp.lock, g.k.sp.dock, g.k.sp.jump, g.k.sp.loot, g.k.sp.mine,
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

	// --- 4. Rack tiles: a fitted cell's glyph sits centred in its tile.
	// The tile body is flat, so the glyph is whatever differs from the
	// body colour sampled in a corner the glyph can never reach (the
	// glyph is 20 px centred in a 50×40 tile): disabled tiles draw at
	// 40 % and would fail an absolute brightness threshold.
	// The tile's inner ink box must be centred on the tile to within a
	// couple of pixels — a glyph hung off one side (the graphic-row
	// centering defect) reads 5 px off and fails here.
	checked := 0
	for row := range g.rack.cells {
		for _, cell := range g.rack.cells[row] {
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

	// Screenshot first: evidence is useful even when an assert failed.
	f, err := os.Create(g.out)
	if err != nil {
		g.err = err
		return
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		g.err = err
		return
	}
	g.saved = true
	if len(errs) > 0 {
		g.err = fmt.Errorf("capture %s: %s", g.out, strings.Join(errs, "; "))
	}
}
