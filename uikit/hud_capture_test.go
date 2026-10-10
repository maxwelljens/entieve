package uikit

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// hudCaptureFrames is how many full draws run before the screenshot is
// taken (anchors and layout settle on the first frames).
const hudCaptureFrames = 3

// TestHudCaptureFrame renders the HUD + rack through a real ebiten game
// loop — pixel readback (Image.At) panics before the game starts — then
// screenshots the frame and asserts the drawn geometry:
//   - each ship ring must DEPLETE: full-strength color inside the sweep,
//     the dim RGB track outside it (an alpha-dimmed track composites to
//     nearly the fill pixel against the opaque panel — the exact defect);
//   - each target card must show glyph pixels on its text line, a
//     fraction-filled layer bar and an empty tail beyond the fraction.
//
// Skipped unless ENTIEVE_CAPTURE=1 so the regular suite stays windowless.
// The PNG lands in $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
func TestHudCaptureFrame(t *testing.T) {
	if os.Getenv("ENTIEVE_CAPTURE") == "" {
		t.Skip("set ENTIEVE_CAPTURE=1 to render a capture frame")
	}
	outDir := os.Getenv("ENTIEVE_CAPTURE_DIR")
	if outDir == "" {
		outDir = "/tmp/opencode"
	}

	env, _ := newChromeEnv(t)
	h := NewHud(env)
	r := NewRack(env)
	c := newChromeUI(env)
	c.addPanel(h.Container(), h)
	c.addPanel(r.Container(), r)

	// --- Scenario: a damaged player, two locks (damaged ship + rock), one
	// active module. Everything mutates the sim's read surface directly,
	// exactly like the other chrome tests do.
	s := env.Sim
	player := s.Player()
	sh, ok := s.Ships.Get(player)
	if !ok {
		t.Fatal("player has no ship")
	}
	sh.Shield = sh.MaxShield * 0.5       // ring sweep 180°
	sh.Armor = sh.MaxArmor * 0.75        // ring sweep 270°
	sh.Structure = sh.MaxStructure * 0.9 // ring sweep 324°

	var shipEnt, rockEnt ecs.Entity
	for _, inf := range s.Grid() {
		if inf.Entity == player {
			continue
		}
		if inf.Kind == sim.KindShip && shipEnt == 0 {
			shipEnt = inf.Entity
		} else if inf.Kind == sim.KindAsteroid && rockEnt == 0 {
			rockEnt = inf.Entity
		}
	}
	if shipEnt == 0 || rockEnt == 0 {
		t.Fatalf("showcase grid missing ship/rock (ship=%v rock=%v)", shipEnt, rockEnt)
	}
	if tgt, ok := s.Ships.Get(shipEnt); ok {
		tgt.Shield = tgt.MaxShield * 0.6
		tgt.Armor = tgt.MaxArmor * 1.0
		tgt.Structure = tgt.MaxStructure * 0.3
	}
	tg, _ := s.Targetings.Get(player)
	tg.Locks = []sim.Lock{
		{Target: shipEnt, Progress: 1},
		{Target: rockEnt, Progress: 1},
	}
	if fit, ok := s.Fittings.Get(player); ok && len(fit.HighRT) > 0 {
		fit.HighRT[0].Active = true
		fit.HighRT[0].CycleEnds = s.Time() + 1.5
	}

	g := &hudCaptureGame{c: c, h: h, out: filepath.Join(outDir, "hud_capture.png")}
	ebiten.SetWindowTitle("Entieve HUD capture")
	ebiten.SetWindowSize(1280, 720)
	if err := ebiten.RunGame(g); err != nil {
		t.Skipf("cannot run capture game loop: %v", err)
	}
	if g.err != nil {
		t.Fatal(g.err)
	}
	if !g.saved {
		t.Fatal("capture never rendered")
	}
	t.Logf("capture written to %s", g.out)
}

// hudCaptureGame drives the chrome for a few frames, then screenshots and
// verifies the drawn pixels.
type hudCaptureGame struct {
	c      *chromeUI
	h      *Hud
	out    string
	frames int
	last   *ebiten.Image
	saved  bool
	err    error
}

func (g *hudCaptureGame) Update() error {
	if g.saved || g.err != nil {
		return ebiten.Termination
	}
	for _, p := range g.c.panels {
		p.Sync()
	}
	g.c.ui.Update()
	g.frames++
	if g.frames > hudCaptureFrames && g.last != nil {
		g.verify(g.last)
		return ebiten.Termination
	}
	return nil
}

func (g *hudCaptureGame) Draw(screen *ebiten.Image) {
	img := ebiten.NewImage(1280, 720)
	g.c.ui.Draw(img)
	screen.DrawImage(img, nil)
	g.last = img
}

func (g *hudCaptureGame) Layout(w, h int) (int, int) { return 1280, 720 }

// verify screenshots the frame and asserts the drawn geometry. Every
// failure is collected so one run reports all of them.
func (g *hudCaptureGame) verify(img *ebiten.Image) {
	var errs []string
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Sprintf(format, args...))
	}

	// --- 1. Ring arcs deplete: brightest green channel in an 8 px radial
	// band at each sample angle. Filled sweep = full layer color; the
	// depleted remainder = ringTrackColor (25 % RGB). An alpha-only dim
	// track would read ~168 here and fail.
	ringRect := g.h.bar.Children()[0].GetWidget().Rect
	cx := float64(ringRect.Min.X) + float64(ringRect.Dx())/2
	cy := float64(ringRect.Min.Y) + float64(ringRect.Dy())/2

	bandG := func(rad, thetaDeg float64) int {
		th := thetaDeg * math.Pi / 180
		best := 0
		for dr := -4.0; dr <= 4; dr++ {
			x := int(cx + math.Sin(th)*(rad+dr) + 0.5)
			y := int(cy - math.Cos(th)*(rad+dr) + 0.5)
			_, gch, _, _ := img.At(x, y).RGBA()
			if v := int(gch >> 8); v > best {
				best = v
			}
		}
		return best
	}

	ringCases := []struct {
		name    string
		rad     float64
		inside  []float64
		outside []float64
	}{
		{"shield@50%", hudRingR, []float64{45, 135}, []float64{225, 315}},
		{"armor@75%", hudRingR - hudRingStep, []float64{45, 135, 225}, []float64{315, 340}},
		{"structure@90%", hudRingR - 2*hudRingStep, []float64{45, 135, 225}, []float64{340, 350}},
	}
	for _, rc := range ringCases {
		inMin, outMax := 255, 0
		for _, a := range rc.inside {
			if v := bandG(rc.rad, a); v < inMin {
				inMin = v
			}
		}
		for _, a := range rc.outside {
			if v := bandG(rc.rad, a); v > outMax {
				outMax = v
			}
		}
		switch {
		case inMin < 150:
			fail("%s: filled arc green %d, want >= 150 (full layer color)", rc.name, inMin)
		case outMax > 90:
			fail("%s: depleted arc green %d vs filled %d — track is not dim", rc.name, outMax, inMin)
		}
	}

	// --- 2. Target card: glyph pixels on the text line, a 60 % shield
	// fill in bar 0 and a dim track in the empty tail of bar 2.
	boxRect := g.h.boxes[0].root.GetWidget().Rect
	card := image.Rect(0, 0, hudBoxW, hudBoxH)
	textRow := targetCardText(card)
	foundText := false
	for y := textRow.Min.Y; y < textRow.Max.Y && !foundText; y++ {
		for x := textRow.Min.X; x < textRow.Max.X; x++ {
			r, gr, b, _ := img.At(boxRect.Min.X+x, boxRect.Min.Y+y).RGBA()
			if r>>8 > 150 && gr>>8 > 150 && b>>8 > 150 {
				foundText = true
				break
			}
		}
	}
	if !foundText {
		fail("target card text row has no glyph pixels")
	}
	bars := layerBars(targetCardBars(card))
	in := img.At(boxRect.Min.X+bars[0].Min.X+10, boxRect.Min.Y+bars[0].Min.Y+4)
	tail := img.At(boxRect.Min.X+bars[2].Max.X-6, boxRect.Min.Y+bars[2].Min.Y+4)
	ir, _, ib, _ := in.RGBA()
	tr, _, _, _ := tail.RGBA()
	if ir>>8 < 80 || ib>>8 < 200 {
		fail("bar0 fill pixel = (%d,%d), want shield blue (111,255)", ir>>8, ib>>8)
	}
	if tr>>8 > 80 {
		fail("bar2 empty tail red = %d, want dim track (<= 80)", tr>>8)
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
