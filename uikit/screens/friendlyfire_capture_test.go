package screens

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/internal/capture"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestFriendlyFireConfirmCapture renders the friendly-fire confirmation
// through a real ebiten loop and pins what the headless suite cannot read
// back: the single dialog over the grid, its title, the named target and
// module rows, and both buttons with ink (the primary reading the owner's
// words). The PNG lands in $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
//
// Skipped unless ENTIEVE_CAPTURE=1; one ebiten loop per process — run it on
// its own:
//
//	ENTIEVE_CAPTURE=1 go test ./uikit/screens/ -run TestFriendlyFireConfirmCapture
func TestFriendlyFireConfirmCapture(t *testing.T) {
	k, _ := newTestKit(t)
	hideCore(k) // the grid and the dialog only

	var target ecs.Entity
	for _, inf := range k.Sim.Grid() {
		if inf.Kind == sim.KindShip && inf.Entity != k.Sim.Player() {
			target = inf.Entity
			break
		}
	}
	if target.IsZero() {
		t.Fatal("fixture: no NPC ship on the grid")
	}
	info, _ := k.Sim.Info(target)
	const module = "250mm Auto Cannon"
	k.OpenFriendlyFire(sim.SlotRef{Class: "high", Index: 2}, target, module)

	out := capture.Out("friendlyfire_confirm.png")
	(&capture.Capture{
		Name:   "Entieve friendly-fire confirmation capture",
		Frames: 20, // the dialog fade settles by frame 12
		Update: func(int) error {
			k.Pointer().Begin()
			k.Sync()
			return k.Update()
		},
		Draw: func(_ int, img *ebiten.Image) error {
			k.Draw(img)
			return nil
		},
		Verify: func(img *ebiten.Image) error {
			var errs []string
			fail := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }

			f := k.friendlyFireWin
			if f == nil || !f.IsOpen() {
				return fmt.Errorf("capture %s: the confirm dialog is not up", out)
			}
			win := f.Win().GetContainer().GetWidget().Rect
			if win.Empty() || win.Max.X > 1280 || win.Max.Y > 720 {
				fail("dialog %v is not on screen", win)
			}
			if f.title.Label != "Friendly fire" {
				fail("title = %q, want Friendly fire", f.title.Label)
			}
			if f.target.Label != info.Name {
				fail("target row = %q, want %q", f.target.Label, info.Name)
			}
			if f.module.Label != module {
				fail("module row = %q, want %q", f.module.Label, module)
			}
			if got := f.Confirm().Text().Label; got != "Confirm foolishness" {
				fail("primary button = %q, want the owner's words", got)
			}
			if got := f.Cancel().Text().Label; got != "Cancel" {
				fail("cancel button = %q, want Cancel", got)
			}
			// The window paints its body, and both buttons carry label ink.
			body := img.At(win.Min.X+theme.Px(theme.RadiusWindow)+6, win.Min.Y+theme.Px(theme.TitleBarH)+theme.Px(theme.ContentsPad)+4)
			if lum(body) == 0 {
				fail("dialog body sample at %v reads black (the window did not paint)", win)
			}
			for name, w := range map[string]*widget.Widget{
				"confirm": f.Confirm().GetWidget(),
				"cancel":  f.Cancel().GetWidget(),
			} {
				if !w.Rect.In(win) {
					fail("%s button %v escapes the dialog %v", name, w.Rect, win)
					continue
				}
				if n := uitest.InkIn(img, w.Rect); n < 12 {
					fail("%s button %v carries %d ink pixels, want its label", name, w.Rect, n)
				}
			}
			if len(errs) > 0 {
				return fmt.Errorf("capture %s: %s", out, strings.Join(errs, "; "))
			}
			return nil
		},
		Out: out,
	}).Run(t)
}
