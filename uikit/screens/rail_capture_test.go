package screens

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/hajimehoshi/ebiten/v2"
)

// railCaptureFrames is how many drawn frames settle each phase before
// its sample is taken (anchors and layout settle on the first frames).
// 13 draws per phase (on top of the two fixture pumps): the kit
// windows open with the 12-frame fade — captures pin the settled
// state, never a mid-fade one (winanim.go).
const railCaptureFrames = 13

// TestRailPixelsDrawAboveWindows: a window dragged onto the rail's
// gutter must never paint over the rail — the rail's column is topmost
// among window pixels. Pixel readback panics before the game starts, so
// this rides the gated real-loop harness (ENTIEVE_CAPTURE=1).
//
// Method: the sample points sit on the entry cells' 1 px borders, over
// the dragged window (and one tab-border point over the bare backdrop as
// the harness control). The expanded-rail frame must differ at every
// point from the SAME window frame with the rail COLLAPSED (cells gone,
// window unchanged). If the window painted over the rail (the defect),
// both frames would carry identical window pixels and every comparison
// would come back equal. Samples run INSIDE Draw, on the frame just
// rendered — ebiten runs Update and Draw at their own cadences, so a
// sample staged from Update can read the previous phase's image.
//
// Skipped unless ENTIEVE_CAPTURE=1 so the regular suite stays windowless.
// The PNG lands in $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
func TestRailPixelsDrawAboveWindows(t *testing.T) {
	if os.Getenv("ENTIEVE_CAPTURE") == "" {
		t.Skip("set ENTIEVE_CAPTURE=1 to render a capture frame")
	}
	uitest.SkipIfLoopRan(t)
	outDir := os.Getenv("ENTIEVE_CAPTURE_DIR")
	if outDir == "" {
		outDir = "/tmp/opencode"
	}

	k, _ := newTestKit(t)
	open := false
	r := NewRail(k.Env(), railEntries3(&open))
	k.AddTopContainer(r, widget.AnchorLayoutData{
		HorizontalPosition: widget.AnchorLayoutPositionStart,
		VerticalPosition:   widget.AnchorLayoutPositionStart,
	})
	// The window under test, dragged onto the gutter so it sits beneath
	// the entry cells.
	k.OverviewWindow().SetLocation(image.Rect(0, 8, 400, 400))
	pumpKit(t, k, 1280, 720)
	pumpKit(t, k, 1280, 720)

	ovRect := k.OverviewWindow().GetContainer().GetWidget().Rect
	cell := r.Button("Overview").GetWidget().Rect
	if cell.Dx() <= 0 || !cell.In(ovRect) {
		t.Fatalf("fixture: Overview cell %v not under the dragged overview %v (or never laid out)", cell, ovRect)
	}
	// Border pixels: the entry cell's four edges over the dragged
	// window — the collapse removes exactly these (cells gone, window
	// unchanged), so each must move between the two phases.
	points := []image.Point{
		{cell.Min.X, (cell.Min.Y + cell.Max.Y) / 2},
		{(cell.Min.X + cell.Max.X) / 2, cell.Min.Y},
		{cell.Max.X - 1, (cell.Min.Y + cell.Max.Y) / 2},
		{(cell.Min.X + cell.Max.X) / 2, cell.Max.Y - 1},
	}

	g := &railCaptureGame{
		k: k, rail: r, points: points,
		out: filepath.Join(outDir, "rail_capture.png"),
	}
	uitest.RunCaptureLoop(t, "Entieve rail capture", g)
	if g.err != nil {
		for i, p := range g.points {
			t.Logf("point %v: rail above = %v, after collapse = %v",
				p, formatRGBA(g.above[i]), formatRGBA(g.collapsedPx[i]))
		}
		t.Fatal(g.err)
	}
	if !g.saved {
		t.Fatal("capture never rendered")
	}
	t.Logf("capture written to %s", g.out)
}

// railCaptureGame pumps the kit, and samples from inside Draw — the
// frame just rendered is the only honest sample: ebiten runs Update and
// Draw on separate cadences, so a frame staged in Update can be the
// previous phase's image. Phase 0 samples the cell borders with the rail
// expanded (screenshot kept), collapses the rail (cells gone, window
// unchanged), phase 1 samples the same points again: every point must
// have moved.
type railCaptureGame struct {
	k      *Kit
	rail   *Rail
	points []image.Point
	out    string

	draws          int
	collapsed      bool
	collapsedDraws int
	done           bool
	expanded       *ebiten.Image // screenshot evidence: the rail-over-window frame
	above          []color.RGBA
	collapsedPx    []color.RGBA
	saved          bool
	err            error
}

func (g *railCaptureGame) Update() error {
	if g.saved || g.err != nil {
		return ebiten.Termination
	}
	g.k.Sync()
	if err := g.k.Update(); err != nil {
		g.err = err
		return ebiten.Termination
	}
	return nil
}

func (g *railCaptureGame) Draw(screen *ebiten.Image) {
	img := ebiten.NewImage(1280, 720)
	g.k.Draw(img)
	screen.DrawImage(img, nil)
	g.draws++

	switch {
	case g.done || g.err != nil:
		return
	case !g.collapsed:
		if g.draws <= railCaptureFrames {
			return
		}
		g.above = g.sample(img)
		g.expanded = img
		g.rail.SetCollapsed(true) // the cells vanish from the next frame on
		g.collapsed = true
		g.collapsedDraws = 0
	case g.collapsedDraws <= railCaptureFrames:
		g.collapsedDraws++
	default:
		g.collapsedPx = g.sample(img)
		g.verify()
		g.done = true
	}
}

func (g *railCaptureGame) Layout(w, h int) (int, int) { return 1280, 720 }

// sample reads one RGBA per point.
func (g *railCaptureGame) sample(img *ebiten.Image) []color.RGBA {
	out := make([]color.RGBA, len(g.points))
	for i, p := range g.points {
		r, gr, b, a := img.At(p.X, p.Y).RGBA()
		out[i] = color.RGBA{R: uint8(r >> 8), G: uint8(gr >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
	}
	return out
}

// verify asserts every point changed once the cells were drawn over the
// window, screenshots the expanded frame and reports.
func (g *railCaptureGame) verify() {
	var errs []string
	for i, p := range g.points {
		if g.above[i] == g.collapsedPx[i] {
			errs = append(errs, fmt.Sprintf("point %v identical with and without the cells (%s) — "+
				"the rail is not painting over the window", p, formatRGBA(g.above[i])))
		}
	}

	// Screenshot first: evidence is useful even when an assert failed.
	f, err := os.Create(g.out)
	if err != nil {
		g.err = err
		return
	}
	defer f.Close()
	if err := png.Encode(f, g.expanded); err != nil {
		g.err = err
		return
	}
	g.saved = true
	if len(errs) > 0 {
		g.err = fmt.Errorf("capture %s: %s", g.out, strings.Join(errs, "; "))
	}
}

// formatRGBA renders a sampled pixel for failure messages.
func formatRGBA(c color.RGBA) string {
	return fmt.Sprintf("(%d,%d,%d,%d)", c.R, c.G, c.B, c.A)
}
