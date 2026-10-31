package game

import (
	"fmt"
	"image"
	"testing"

	"github.com/entieve/entieve/internal/capture"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestContactDetailsCapture: the Details dialog is the dock's headline
// screen — the contact's face, their words and the contract at a glance —
// and it is drawn by the real game over the docked backdrop. Pixel readback
// needs a running ebiten loop, so this is the gated capture
// (ENTIEVE_CAPTURE=1, one loop per process — scripts/captures.sh drives it);
// the PNG lands in $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
//
// Method: dock at the first station, snapshot a dialog-free frame, press the
// offer row's Details button through the kit, then sample a grid inside the
// dialog's rect: most of it must differ from the bare dock (the page really
// paints) and the window must sit inside the viewport.
func TestContactDetailsCapture(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	dockAt(t, g)
	g.syncScenes()
	g.syncMounts()
	for i := 0; i < stationFadeFrames+3; i++ {
		if err := g.Update(); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
	}
	rows := stationBoard(t, g).Rows(false)
	if len(rows) == 0 || rows[0].ID == "" {
		t.Fatal("fixture: no live offer at the dock")
	}
	row := findRow(t, rows, rows[0].ID)
	btn := row.Btn

	var (
		bare    *ebiten.Image // the same dock, dialog closed
		dialog  image.Rectangle
		opened  bool
		changed int
		sampled int
	)

	out := capture.Out("contact_details_capture.png")
	(&capture.Capture{
		Name:   "Entieve contact details capture",
		Frames: 12,
		Update: func(n int) error {
			if n == 3 && !opened {
				opened = true
				click(t, g, btn) // the row's Details button → the dialog
			}
			return g.Update()
		},
		Draw: func(n int, img *ebiten.Image) error {
			g.Draw(img)
			switch {
			case n == 2:
				bare = ebiten.NewImage(1280, 720)
				bare.DrawImage(img, nil)
			case n == 12:
				if w := g.kit.ContactWindow(); w != nil {
					dialog = w.GetContainer().GetWidget().Rect
				}
				for y := dialog.Min.Y + 8; y < dialog.Max.Y-8; y += 12 {
					for x := dialog.Min.X + 8; x < dialog.Max.X-8; x += 12 {
						sampled++
						if delta(img.At(x, y), bare.At(x, y)) > 20 {
							changed++
						}
					}
				}
			}
			return nil
		},
		Verify: func(*ebiten.Image) error {
			if !g.kit.ContactOpen() {
				return fmt.Errorf("capture %s: Details never opened", out)
			}
			screen := image.Rect(0, 0, 1280, 720)
			if dialog.Empty() || !dialog.In(screen) {
				return fmt.Errorf("capture %s: dialog rect %v is not on screen", out, dialog)
			}
			if sampled == 0 || changed*3 < sampled*2 {
				return fmt.Errorf("capture %s: only %d/%d sampled dialog pixels changed — the page did not paint",
					out, changed, sampled)
			}
			return nil
		},
		Out: out,
	}).Run(t)
	t.Logf("capture written to %s (dialog %v, %d/%d sampled pixels changed)",
		out, dialog, changed, sampled)
}
