package screens

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/entieve/entieve/internal/capture"
	"github.com/hajimehoshi/ebiten/v2"
)

// settingsCaptureFrames is how many draws settle before the sample (the
// window fades in over 12 frames).
const settingsCaptureFrames = 14

// TestSettingsCaptureFrame draws the settings window through a real
// ebiten loop and pins the slider family's pixels the headless suite
// cannot read back: the accent fill reaches the master volume's 100%
// handle, the notched scale slider carries one dim tick per rung at the
// handle's own resting positions, and every handle is on its track.
// Skipped unless ENTIEVE_CAPTURE=1; the PNG lands in $ENTIEVE_CAPTURE_DIR
// (default /tmp/opencode).
func TestSettingsCaptureFrame(t *testing.T) {
	env, _ := newTestEnv(t)
	act := staticSettingsActions()
	s := NewSettings(env, act)
	ui := newWindowUI(t, env, s.Win())
	s.Win().SetLocation(image.Rect(490, 130, 790, 590))

	out := capture.Out("settings_capture.png")
	(&capture.Capture{
		Name:   "Entieve settings capture",
		Frames: settingsCaptureFrames,
		Update: func(int) error {
			s.Sync()
			ui.Update()
			return nil
		},
		Draw: func(_ int, img *ebiten.Image) error {
			img.Fill(color.White)
			ui.Draw(img)
			return nil
		},
		// verify samples the sliders on the settled frame.
		Verify: func(img *ebiten.Image) error {
			p := s.th.Pal()
			if v := s.masterSlide.Value(); v != 100 {
				return fmt.Errorf("master slider = %v, want the 100%% default", v)
			}
			// The 100% handle sits at the track's right end, so the fill spans
			// the whole track: its centre is accent.
			mr := s.masterSlide.GetWidget().Rect
			mcy := mr.Min.Y + mr.Dy()/2
			if got := pixel(img, mr.Min.X+mr.Dx()/2, mcy); got != p.Accent {
				return fmt.Errorf("master track centre = %v, want the accent fill %v", got, p.Accent)
			}
			if got := pixel(img, s.masterSlide.ValueX(100), mcy); got != p.Text {
				return fmt.Errorf("master handle = %v, want the handle body %v", got, p.Text)
			}

			// One dim tick per rung, at the handle's resting x; only the rung the
			// handle sits on is covered by the handle itself.
			sr := s.scaleSlider.GetWidget().Rect
			scy := sr.Min.Y + sr.Dy()/2
			hx := s.scaleSlider.ValueX(s.scaleSlider.Value())
			ticks := s.scaleSlider.TickPositions()
			if len(ticks) != 6 {
				return fmt.Errorf("scale ticks = %d, want 6", len(ticks))
			}
			for _, x := range ticks {
				if x == hx {
					continue // the resting handle paints over its own tick
				}
				if got := pixel(img, x, scy); got != p.Dim {
					return fmt.Errorf("scale tick at x=%d = %v, want the dim mark %v", x, got, p.Dim)
				}
			}
			if got := pixel(img, hx, scy); got != p.Text {
				return fmt.Errorf("scale handle = %v, want the handle body %v", got, p.Text)
			}
			return nil
		},
		Out: out,
	}).Run(t)
}

// pixel is the image's colour at (x, y) as the palette's own type.
func pixel(img *ebiten.Image, x, y int) color.RGBA {
	return color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
}
