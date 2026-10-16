package screens

import (
	"fmt"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/widgets"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// tooltipCaptureFrames is how many full draws run before the screenshot is
// taken (anchors and layout settle on the first frames).
const tooltipCaptureFrames = 3

// TestTooltipCaptureFrame renders a rack tooltip — the real structured
// panel the EMP Turret cell shows, mounted where a cursor-sticky hover
// would put it — over a live rack through a real ebiten game loop, checks
// the drawn pixels (icon ink, bold title glyphs, the accent hairline,
// values ending on their shared anchor) and screenshots the frame.
//
// Skipped unless ENTIEVE_CAPTURE=1 so the regular suite stays windowless.
// The PNG lands in $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
func TestTooltipCaptureFrame(t *testing.T) {
	if os.Getenv("ENTIEVE_CAPTURE") == "" {
		t.Skip("set ENTIEVE_CAPTURE=1 to render a capture frame")
	}
	uitest.SkipIfLoopRan(t)
	outDir := os.Getenv("ENTIEVE_CAPTURE_DIR")
	if outDir == "" {
		outDir = "/tmp/opencode"
	}

	env, _ := newChromeEnv(t)
	r := NewRack(env)
	c := newChromeUI(env)
	c.AddPanel(r.Container(), r)
	c.Frame(t, 1280, 720) // Sync fills the cells, so the tile has a rect

	def, ok := env.Content.Module("mod-emp-turret")
	if !ok {
		t.Fatal("fixture: mod-emp-turret missing")
	}
	_, build := r.cellTip(def, sim.SlotRef{Class: content.SlotHigh, Index: 0})
	model := build()
	body, view := widgets.NewTipContent(env, model)

	// Where the hover would land it: the cursor-sticky anchor is 12 px
	// right/below the cell, the panel trailing left, clamped into the
	// window (the same rule tipView draws itself by).
	cell := r.cells[0][0].btn.GetWidget().Rect
	size := widgets.TipSize(env, model)
	raw := image.Rect(cell.Min.X+12-size.X, cell.Min.Y+12, cell.Min.X+12, cell.Min.Y+12+size.Y)
	at := widgets.ClampTipRect(raw, image.Rect(0, 0, 1280, 720)).Min
	body.GetWidget().LayoutData = widget.AnchorLayoutData{
		HorizontalPosition: widget.AnchorLayoutPositionStart,
		VerticalPosition:   widget.AnchorLayoutPositionStart,
		Padding:            &widget.Insets{Left: at.X, Top: at.Y},
	}
	c.root.AddChild(body)

	g := &tooltipCaptureGame{c: c, View: view, out: filepath.Join(outDir, "tooltip_capture.png")}
	uitest.RunCaptureLoop(t, "Entieve tooltip capture", g)
	if g.err != nil {
		t.Fatal(g.err)
	}
	if !g.saved {
		t.Fatal("capture never rendered")
	}
	t.Logf("capture written to %s", g.out)
}

// tooltipCaptureGame drives the rack for a few frames, then screenshots and
// verifies the drawn tooltip pixels.
type tooltipCaptureGame struct {
	c      *chromeUI
	View   *widgets.TipView
	out    string
	frames int
	last   *ebiten.Image
	saved  bool
	err    error
}

func (g *tooltipCaptureGame) Update() error {
	if g.saved || g.err != nil {
		return ebiten.Termination
	}
	for _, p := range g.c.panels {
		p.Sync()
	}
	g.c.ui.Update()
	g.frames++
	if g.frames > tooltipCaptureFrames && g.last != nil {
		g.verify(g.last)
		return ebiten.Termination
	}
	return nil
}

func (g *tooltipCaptureGame) Draw(screen *ebiten.Image) {
	img := ebiten.NewImage(1280, 720)
	g.c.ui.Draw(img)
	screen.DrawImage(img, nil)
	g.last = img
}

func (g *tooltipCaptureGame) Layout(w, h int) (int, int) { return 1280, 720 }

// verify screenshots the frame and asserts the drawn tooltip: the panel is
// on screen, its icon and title carry ink, the hairline is accent-tinted
// against the fill, and the section's values end on the shared anchor.
// Every failure is collected so one run reports all of them.
func (g *tooltipCaptureGame) verify(img *ebiten.Image) {
	var errs []string
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Sprintf(format, args...))
	}

	panel := g.View.GetWidget().Rect
	screen := image.Rect(0, 0, 1280, 720)
	if !panel.In(screen) {
		fail("panel %v is not fully on screen", panel)
	}
	// The layout is recomputed for the drawn width — the same call
	// Render makes — and its rects are offset by the panel's origin.
	layout := g.View.Panel(g.View.GetWidget().Rect.Dx() - 2*widgets.TipPadX)
	at := func(r image.Rectangle) image.Rectangle { return r.Add(panel.Min) }

	// --- 1. The 20 px class glyph in the header has ink.
	if ic := at(layout.Icon); !ic.Empty() {
		if n := uitest.InkIn(img, ic); n < 8 {
			fail("icon box %v drew %d ink pixels, want ≥ 8", ic, n)
		}
	}
	// --- 2. The bold title carries bright glyphs on the fill.
	title := at(layout.Title)
	if n := uitest.InkIn(img, title); n < 20 {
		fail("title box %v drew %d bright pixels, want ≥ 20", title, n)
	}
	// --- 3. The first hairline reads accent over the panel fill (the fill//     itself is near-black, so any real divider is far brighter).
	if len(layout.Rules) == 0 {
		fail("layout has no hairline")
	} else {
		rule := at(layout.Rules[0])
		lit := 0
		for x := rule.Min.X + 2; x < rule.Max.X-2; x += 8 {
			rr, _, _, _ := img.At(x, (rule.Min.Y+rule.Max.Y)/2).RGBA()
			if rr>>8 > 50 {
				lit++
			}
		}
		if lit < 3 {
			fail("hairline %v lit only %d samples, want the accent tint everywhere", rule, lit)
		}
	}
	// --- 4. Values end on the shared anchor: the rightmost glyph pixel of
	//     the first row's value stops at ValueX and never crosses it.
	if len(layout.Sections) > 0 && len(layout.Sections[0].Rows) > 0 {
		row := layout.Sections[0].Rows[0]
		vr := at(row.Value)
		anchor := panel.Min.X + layout.ValueX
		right, n := uitest.InkBounds(img, vr)
		if n == 0 {
			fail("value box %v drew no ink", vr)
		} else if right.Max.X > anchor+1 || right.Max.X < anchor-8 {
			fail("value ink ends at x=%d, want within a pixel of the anchor %d", right.Max.X, anchor)
		}
		// The label starts at the content edge (left-anchored).
		lr := at(row.Label)
		if left, n := uitest.InkBounds(img, lr); n > 0 && left.Min.X > panel.Min.X+widgets.TipPadX+4 {
			fail("label ink starts at x=%d, want near the content edge %d",
				left.Min.X, panel.Min.X+widgets.TipPadX)
		}
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
