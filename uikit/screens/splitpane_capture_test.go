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
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// splitCaptureFrames is how many draws settle before the sample: the
// open fade is 12 frames, so 13 is the first fully painted frame.
const splitCaptureFrames = 13

// TestSplitPaneSeamCapture draws the three split windows side by side
// through a real ebiten loop and pins the seams' PIXELS, not only their
// rects: each seam is a recessed 4 px track (a Border line around the
// near-black fill) with a dotted sash grip in its middle — the grip's dots
// brighter than the track's fill — and the tracks sit at the split's
// proportions: inventory's two seams at its thirds, fitting's and
// manufacturing's at their halves. Pixel reads need a running loop, so
// this is gated like the other captures; the PNG lands in
// $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
func TestSplitPaneSeamCapture(t *testing.T) {
	if os.Getenv("ENTIEVE_CAPTURE") == "" {
		t.Skip("set ENTIEVE_CAPTURE=1 to render a capture frame")
	}
	uitest.SkipIfLoopRan(t)
	outDir := os.Getenv("ENTIEVE_CAPTURE_DIR")
	if outDir == "" {
		outDir = "/tmp/opencode"
	}

	k, _ := newTestKit(t)
	inv := NewInventory(k.Env())
	fit := NewFitting(k.Env())
	manu := NewManufacturing(k.Env())
	for _, c := range []struct {
		win *widget.Window
		at  image.Rectangle
	}{
		{inv.Win(), image.Rect(60, 8, 440, 448)},
		{fit.Win(), image.Rect(446, 8, 826, 448)},
		{manu.Win(), image.Rect(832, 8, 1252, 448)},
	} {
		k.AddWindow(c.win)
		c.win.SetLocation(c.at)
	}
	// The core Selected Item window would cover the inventory's lower
	// panes: park it below, out of the sweep.
	k.SelectedWindow().SetLocation(image.Rect(60, 460, 510, 716))

	g := &splitCaptureGame{
		k:    k,
		inv:  inv,
		fit:  fit,
		manu: manu,
		out:  filepath.Join(outDir, "splitpanes_capture.png"),
	}
	uitest.RunCaptureLoop(t, "Entieve split pane capture", g)
	if g.err != nil {
		t.Fatal(g.err)
	}
	if !g.saved {
		t.Fatal("capture never rendered")
	}
	t.Logf("capture written to %s", g.out)
}

// splitCaptureGame drives the kit and verifies the settled frame.
type splitCaptureGame struct {
	k      *Kit
	inv    *Inventory
	fit    *Fitting
	manu   *Manufacturing
	out    string
	frames int
	last   *ebiten.Image
	saved  bool
	err    error
}

func (g *splitCaptureGame) Update() error {
	if g.saved || g.err != nil {
		return ebiten.Termination
	}
	g.inv.Sync()
	g.fit.Sync()
	g.manu.Sync()
	if err := g.k.Update(); err != nil {
		g.err = err
		return ebiten.Termination
	}
	if g.frames > splitCaptureFrames && g.last != nil {
		g.err = g.verify(g.last)
		return ebiten.Termination
	}
	return nil
}

func (g *splitCaptureGame) Draw(screen *ebiten.Image) {
	img := ebiten.NewImage(1280, 720)
	img.Fill(color.White)
	g.k.Draw(img)
	g.last = img
	g.frames++
	screen.DrawImage(img, nil)
}

func (g *splitCaptureGame) Layout(w, h int) (int, int) { return 1280, 720 }

func (g *splitCaptureGame) verify(img *ebiten.Image) error {
	var errs []string
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Sprintf(format, args...))
	}
	lum := func(pt image.Point) int {
		r, gg, b, _ := img.At(pt.X, pt.Y).RGBA()
		return int(r>>8) + int(gg>>8) + int(b>>8)
	}

	f, err := os.Create(g.out)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return err
	}
	g.saved = true

	check := func(name string, split *widgets.SplitPane) []int {
		panes := splitPanes(split)
		var rows []int
		for i := 0; i < len(panes)-1; i++ {
			seam := splitSeam(split, i)
			if seam.Dy() != theme.Px(widgets.SplitSeamH) {
				fail("%s: seam %d is %d px, want the %d px track", name, i, seam.Dy(), theme.Px(widgets.SplitSeamH))
				continue
			}
			mid := image.Pt((seam.Min.X+seam.Max.X)/2, (seam.Min.Y+seam.Max.Y)/2)
			fill := lum(image.Pt(seam.Min.X+4, seam.Min.Y+1)) // track fill, clear of the grip
			if edge := lum(image.Pt(seam.Min.X+4, seam.Min.Y)); edge <= fill {
				fail("%s: seam %d's top edge (brightness %d) does not stand out of the track fill (%d)",
					name, i, edge, fill)
			}
			if grip := lum(mid); grip <= fill {
				fail("%s: seam %d's grip (brightness %d) does not stand out of the track fill (%d)",
					name, i, grip, fill)
			}
			rows = append(rows, seam.Min.Y)
		}
		return rows
	}

	// Inventory: two seams on the thirds of the pane stack.
	if rows := check("inventory", g.inv.split); len(rows) == 2 {
		panes := splitPanes(g.inv.split)
		for i, row := range rows {
			want := panes[i].Max.Y
			if d := row - want; d < -2 || d > 2 {
				fail("inventory: seam %d painted at y=%d, want the pane boundary ≈%d", i, row, want)
			}
		}
	} else {
		fail("inventory: %d seams found, want 2", len(rows))
	}
	// Fitting and manufacturing: one seam, at the panes' boundary.
	for _, c := range []struct {
		name  string
		split *widgets.SplitPane
	}{{"fitting", g.fit.split}, {"manufacturing", g.manu.split}} {
		rows := check(c.name, c.split)
		panes := splitPanes(c.split)
		if len(rows) != 1 || len(panes) != 2 {
			fail("%s: %d seams / %d panes, want 1 / 2", c.name, len(rows), len(panes))
			continue
		}
		if want := panes[0].Max.Y; rows[0] < want-2 || rows[0] > want+2 {
			fail("%s: seam painted at y=%d, want the pane boundary ≈%d", c.name, rows[0], want)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("capture %s: %s", g.out, strings.Join(errs, "; "))
	}
	return nil
}
