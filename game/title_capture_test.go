package game

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestTitleCaptureFrame is the gated title capture (ENTIEVE_CAPTURE=1,
// one ebiten loop per process). It pins the phase's pixels: the tracked
// amber wordmark and the menu are drawn over the embedded artwork backdrop
// (title_background.jpg, cover-fit under the light veil), and the gameplay
// chrome (rail column, HUD band, Overview corner, log pocket) is ABSENT —
// those sample points are pixel-identical to a backdrop-only frame. A
// second frame opens the save manager and writes it beside the title frame.
//
// Output: $ENTIEVE_CAPTURE_DIR (default /tmp/opencode)/title_capture.png
// and title_manager_capture.png.
func TestTitleCaptureFrame(t *testing.T) {
	if os.Getenv("ENTIEVE_CAPTURE") == "" {
		t.Skip("set ENTIEVE_CAPTURE=1 to render a capture frame")
	}
	outDir := os.Getenv("ENTIEVE_CAPTURE_DIR")
	if outDir == "" {
		outDir = "/tmp/opencode"
	}
	g := newTitleGameAt(t, filepath.Join(t.TempDir(), "save.json"))
	g.applySize(1280, 720)

	cg := &titleCaptureGame{
		g:          g,
		out:        filepath.Join(outDir, "title_capture.png"),
		managerOut: filepath.Join(outDir, "title_manager_capture.png"),
	}
	ebiten.SetWindowTitle("Entieve title capture")
	ebiten.SetWindowSize(1280, 720)
	// Capture contract: the loop never shows a window on the desktop.
	ebiten.SetWindowVisible(false)
	if err := ebiten.RunGame(cg); err != nil {
		t.Skipf("cannot run capture game loop: %v", err)
	}
	if cg.err != nil {
		t.Fatal(cg.err)
	}
	if !cg.titleSaved || !cg.managerSaved {
		t.Fatal("capture never rendered")
	}
	t.Logf("title capture %s (wordmark pixels %d accent %d, chrome diffs %d, artwork pixels %d/%d distinct %d) and %s",
		cg.out, cg.wordPixels, cg.accent, cg.chromeDiffs, cg.artPixels, cg.artDiff, cg.distinct, cg.managerOut)
}

type titleCaptureGame struct {
	g          *Game
	out        string
	managerOut string

	frames       int
	titleSaved   bool
	managerSaved bool
	done         bool
	err          error

	wordPixels    int
	accent        int
	chromeDiffs   int
	artPixels     int // backdrop samples that carry art colour
	artDiff       int // backdrop samples that differ from the world+dim frame
	distinct      int // distinct backdrop sample colours (not a flat fill)
	fallbackDiffs int
	againDiffs    int
	managerPx     int
}

func (cg *titleCaptureGame) Update() error {
	if cg.done || cg.err != nil {
		return ebiten.Termination
	}
	cg.frames++
	return cg.g.Update()
}

func (cg *titleCaptureGame) Draw(screen *ebiten.Image) {
	img := ebiten.NewImage(1280, 720)
	cg.g.Draw(img)
	screen.DrawImage(img, nil)
	if cg.err != nil || cg.done || cg.frames < 4 {
		return
	}
	if !cg.titleSaved {
		cg.captureTitle(img)
		cg.titleSaved = true
		cg.g.titleUI.openID(idManager) // next frame renders it
		return
	}
	if !cg.managerSaved && cg.g.titleUI.host.Mounted(idManager) && cg.frames >= 20 {
		cg.captureManager(img)
		cg.managerSaved = true
		cg.done = true
	}
}

// captureTitle samples the settled title frame against the same frame
// minus the title chrome (the backdrop layer alone).
func (cg *titleCaptureGame) captureTitle(img *ebiten.Image) {
	g := cg.g
	base := ebiten.NewImage(1280, 720)
	g.drawTitleBackdrop(base)

	// The wordmark: amber tracked text over the backdrop.
	wr := g.titleUI.wordmark.GetWidget().Rect
	if wr.Dx() <= 0 || wr.Dy() <= 0 {
		cg.err = fmt.Errorf("wordmark never laid out (%v)", wr)
		return
	}
	for y := wr.Min.Y; y < wr.Max.Y; y++ {
		for x := wr.Min.X; x < wr.Max.X; x++ {
			if img.At(x, y) != base.At(x, y) {
				cg.wordPixels++
			}
			r, _, b, _ := img.At(x, y).RGBA()
			if r>>8 > 100 && int(r>>8) > int(b>>8)+30 {
				cg.accent++
			}
		}
	}
	for _, it := range g.titleUI.items {
		if r := it.Btn.GetWidget().Rect; r.Dx() <= 0 || r.Dy() <= 0 {
			cg.err = fmt.Errorf("menu row %q never laid out (%v)", it.Label, r)
			return
		}
	}

	// Chrome-free probes: rail column, both top corners, the log pocket
	// and the HUD band — all clear of the centred menu.
	probes := []image.Point{
		{X: theme.Px(20), Y: 120}, {X: theme.Px(20), Y: 360}, {X: theme.Px(20), Y: 600},
		{X: 1280 - 40, Y: 30}, {X: theme.Px(97) + 40, Y: 30},
		{X: 140, Y: 660}, {X: 640, Y: 690}, {X: 1000, Y: 690},
	}
	for _, p := range probes {
		if img.At(p.X, p.Y) != base.At(p.X, p.Y) {
			cg.chromeDiffs++
		}
	}

	// The backdrop is the user's artwork, not the old starfield: nearly
	// every sample carries art colour, differs from the world+dim frame,
	// and the samples are not a flat fill.
	world := ebiten.NewImage(1280, 720)
	g.space.Draw(world)
	dimScreen(world, 1280, 720, titleDim)
	seen := map[color.RGBA]bool{}
	for y := 4; y < 720; y += 16 {
		for x := 4; x < 1280; x += 16 {
			if base.At(x, y) != world.At(x, y) {
				cg.artDiff++
			}
			r, gg, b, _ := base.At(x, y).RGBA()
			if r>>8 > 8 || gg>>8 > 8 || b>>8 > 8 {
				cg.artPixels++
			}
			seen[color.RGBAModel.Convert(base.At(x, y)).(color.RGBA)] = true
		}
	}
	cg.distinct = len(seen)

	// The fallback branch is still exactly today's world+dim backdrop.
	plate := titleArtwork
	titleArtwork = func() *ebiten.Image { return nil }
	fb := ebiten.NewImage(1280, 720)
	g.drawTitleBackdrop(fb)
	titleArtwork = plate
	for y := 4; y < 720; y += 16 {
		for x := 4; x < 1280; x += 16 {
			if fb.At(x, y) != world.At(x, y) {
				cg.fallbackDiffs++
			}
		}
	}

	// Drawing the backdrop twice gives the same pixels (determinism).
	again := ebiten.NewImage(1280, 720)
	g.drawTitleBackdrop(again)
	for y := 0; y < 720; y += 8 {
		for x := 0; x < 1280; x += 8 {
			if again.At(x, y) != base.At(x, y) {
				cg.againDiffs++
			}
		}
	}

	if cg.wordPixels < 50 || cg.accent < 30 {
		cg.err = fmt.Errorf("wordmark missing: %d changed pixels, %d accent pixels", cg.wordPixels, cg.accent)
	} else if cg.chromeDiffs != 0 {
		cg.err = fmt.Errorf("gameplay chrome painted at the title: %d probe points differ from the backdrop frame", cg.chromeDiffs)
	} else if cg.artPixels < 400 || cg.artDiff < 400 || cg.distinct < 30 {
		cg.err = fmt.Errorf("artwork backdrop missing: %d lit samples, %d differing from the world, %d distinct colours",
			cg.artPixels, cg.artDiff, cg.distinct)
	} else if cg.fallbackDiffs != 0 {
		cg.err = fmt.Errorf("world fallback drifted: %d samples differ from world+dim", cg.fallbackDiffs)
	} else if cg.againDiffs != 0 {
		cg.err = fmt.Errorf("artwork backdrop is not deterministic: %d samples differ on the second draw", cg.againDiffs)
	}
	cg.write(img, cg.out)
}

// captureManager pins the save manager window over the title.
func (cg *titleCaptureGame) captureManager(img *ebiten.Image) {
	win := cg.g.titleUI.host.Window(idManager)
	if win == nil {
		cg.err = fmt.Errorf("manager window did not mount")
		cg.write(img, cg.managerOut)
		return
	}
	wr := win.GetContainer().GetWidget().Rect
	if wr.Dx() <= 0 || wr.Dy() <= 0 {
		cg.err = fmt.Errorf("manager window never laid out (%v)", wr)
		cg.write(img, cg.managerOut)
		return
	}
	base := ebiten.NewImage(1280, 720)
	cg.g.drawTitleBackdrop(base)
	for y := wr.Min.Y; y < wr.Max.Y; y += 2 {
		for x := wr.Min.X; x < wr.Max.X; x += 2 {
			if img.At(x, y) != base.At(x, y) {
				cg.managerPx++
			}
		}
	}
	if cg.managerPx < 100 {
		cg.err = fmt.Errorf("manager window not painted: %d changed pixels (rect %v visible %v mounted %v)",
			cg.managerPx, wr, win.GetContainer().GetWidget().IsVisible(), cg.g.titleUI.host.Mounted(idManager))
	}
	cg.write(img, cg.managerOut)
}

func (cg *titleCaptureGame) write(img *ebiten.Image, path string) {
	f, err := os.Create(path)
	if err != nil {
		cg.err = err
		return
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		cg.err = err
		return
	}
	if err := f.Close(); err != nil {
		cg.err = err
	}
}

func (cg *titleCaptureGame) Layout(w, h int) (int, int) { return 1280, 720 }
