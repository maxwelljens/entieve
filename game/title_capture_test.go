package game

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"testing"

	"github.com/entieve/entieve/internal/capture"
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
	g := newTitleGameAt(t, filepath.Join(t.TempDir(), "save.json"))
	g.applySize(1280, 720)

	titleOut := capture.Out("title_capture.png")
	managerOut := capture.Out("title_manager_capture.png")

	var (
		frames       int // Updates: the open fades are counted in frames
		titleSaved   bool
		managerSaved bool

		wordPixels    int
		accent        int
		chromeDiffs   int
		artPixels     int // backdrop samples that carry art colour
		artDiff       int // backdrop samples that differ from the world+dim frame
		distinct      int // distinct backdrop sample colours (not a flat fill)
		fallbackDiffs int
		againDiffs    int
		managerPx     int
	)

	// captureTitle samples the settled title frame against the same frame
	// minus the title chrome (the backdrop layer alone).
	captureTitle := func(img *ebiten.Image) error {
		base := ebiten.NewImage(1280, 720)
		g.drawTitleBackdrop(base)

		// The wordmark: amber tracked text over the backdrop.
		wr := g.titleUI.wordmark.GetWidget().Rect
		if wr.Dx() <= 0 || wr.Dy() <= 0 {
			return fmt.Errorf("wordmark never laid out (%v)", wr)
		}
		for y := wr.Min.Y; y < wr.Max.Y; y++ {
			for x := wr.Min.X; x < wr.Max.X; x++ {
				if img.At(x, y) != base.At(x, y) {
					wordPixels++
				}
				r, _, b, _ := img.At(x, y).RGBA()
				if r>>8 > 100 && int(r>>8) > int(b>>8)+30 {
					accent++
				}
			}
		}
		for _, it := range g.titleUI.items {
			if r := it.Btn.GetWidget().Rect; r.Dx() <= 0 || r.Dy() <= 0 {
				return fmt.Errorf("menu row %q never laid out (%v)", it.Label, r)
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
				chromeDiffs++
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
					artDiff++
				}
				r, gg, b, _ := base.At(x, y).RGBA()
				if r>>8 > 8 || gg>>8 > 8 || b>>8 > 8 {
					artPixels++
				}
				seen[color.RGBAModel.Convert(base.At(x, y)).(color.RGBA)] = true
			}
		}
		distinct = len(seen)

		// The fallback branch is still exactly today's world+dim backdrop.
		plate := titleArtwork
		titleArtwork = func() *ebiten.Image { return nil }
		fb := ebiten.NewImage(1280, 720)
		g.drawTitleBackdrop(fb)
		titleArtwork = plate
		for y := 4; y < 720; y += 16 {
			for x := 4; x < 1280; x += 16 {
				if fb.At(x, y) != world.At(x, y) {
					fallbackDiffs++
				}
			}
		}

		// Drawing the backdrop twice gives the same pixels (determinism).
		again := ebiten.NewImage(1280, 720)
		g.drawTitleBackdrop(again)
		for y := 0; y < 720; y += 8 {
			for x := 0; x < 1280; x += 8 {
				if again.At(x, y) != base.At(x, y) {
					againDiffs++
				}
			}
		}

		var err error
		switch {
		case wordPixels < 50 || accent < 30:
			err = fmt.Errorf("wordmark missing: %d changed pixels, %d accent pixels", wordPixels, accent)
		case chromeDiffs != 0:
			err = fmt.Errorf("gameplay chrome painted at the title: %d probe points differ from the backdrop frame", chromeDiffs)
		case artPixels < 400 || artDiff < 400 || distinct < 30:
			err = fmt.Errorf("artwork backdrop missing: %d lit samples, %d differing from the world, %d distinct colours",
				artPixels, artDiff, distinct)
		case fallbackDiffs != 0:
			err = fmt.Errorf("world fallback drifted: %d samples differ from world+dim", fallbackDiffs)
		case againDiffs != 0:
			err = fmt.Errorf("artwork backdrop is not deterministic: %d samples differ on the second draw", againDiffs)
		}
		if saveErr := capture.Save(titleOut, img); saveErr != nil && err == nil {
			err = saveErr
		}
		return err
	}

	// captureManager pins the save manager window over the title.
	captureManager := func(img *ebiten.Image) error {
		var err error
		switch win := g.titleUI.host.Window(idManager); {
		case win == nil:
			err = fmt.Errorf("manager window did not mount")
		default:
			wr := win.GetContainer().GetWidget().Rect
			if wr.Dx() <= 0 || wr.Dy() <= 0 {
				err = fmt.Errorf("manager window never laid out (%v)", wr)
				break
			}
			base := ebiten.NewImage(1280, 720)
			g.drawTitleBackdrop(base)
			for y := wr.Min.Y; y < wr.Max.Y; y += 2 {
				for x := wr.Min.X; x < wr.Max.X; x += 2 {
					if img.At(x, y) != base.At(x, y) {
						managerPx++
					}
				}
			}
			if managerPx < 100 {
				err = fmt.Errorf("manager window not painted: %d changed pixels (rect %v visible %v mounted %v)",
					managerPx, wr, win.GetContainer().GetWidget().IsVisible(), g.titleUI.host.Mounted(idManager))
			}
		}
		if saveErr := capture.Save(managerOut, img); saveErr != nil && err == nil {
			err = saveErr
		}
		return err
	}

	var cap *capture.Capture
	cap = &capture.Capture{
		Name: "Entieve title capture",
		Update: func(int) error {
			frames++
			return g.Update()
		},
		Draw: func(_ int, img *ebiten.Image) error {
			g.Draw(img)
			if frames < 4 {
				return nil
			}
			if !titleSaved {
				titleSaved = true
				if err := captureTitle(img); err != nil {
					return err
				}
				g.titleUI.openID(idManager) // next frame renders it
				return nil
			}
			if !managerSaved && g.titleUI.host.Mounted(idManager) && frames >= 20 {
				managerSaved = true
				if err := captureManager(img); err != nil {
					return err
				}
				cap.Stop()
			}
			return nil
		},
	}
	cap.Run(t)
	t.Logf("title capture %s (wordmark pixels %d accent %d, chrome diffs %d, artwork pixels %d/%d distinct %d) and %s",
		titleOut, wordPixels, accent, chromeDiffs, artPixels, artDiff, distinct, managerOut)
}
