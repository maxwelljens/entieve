package screens

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/hajimehoshi/ebiten/v2"
)

// hudCaptureFrames is how many full draws run before the screenshot is
// taken (anchors and layout settle on the first frames).
const hudCaptureFrames = 3

// TestHudCaptureFrame renders the HUD + rack through a real ebiten game
// loop — pixel readback (Image.At) panics before the game starts — then
// screenshots the frame and asserts the drawn geometry:
//   - the hull portrait carries ink;
//   - each health bar must DEPLETE: the layer color up to the fraction and
//     the dark panel beyond it (50/75/90 % damage);
//   - the time/wallet strip sits above the status and its clock icon has ink;
//   - each target card must show glyph pixels on its text line, a
//     fraction-filled layer bar and an empty tail beyond the fraction.
//
// Skipped unless ENTIEVE_CAPTURE=1 so the regular suite stays windowless.
// The PNG lands in $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
func TestHudCaptureFrame(t *testing.T) {
	if os.Getenv("ENTIEVE_CAPTURE") == "" {
		t.Skip("set ENTIEVE_CAPTURE=1 to render a capture frame")
	}
	uitest.SkipIfLoopRan(t)
	outDir := os.Getenv("ENTIEVE_CAPTURE_DIR")
	if outDir == "" {
		outDir = "/tmp/opencode"
	}

	env, _ := newChromeEnv(t)
	h := NewHud(env)
	r := NewRack(env)
	h.MountRack(r.Container())
	c := newChromeUI(env)
	c.AddPanel(h.Container(), h)
	c.panels = append(c.panels, r) // the rack rides the HUD pair; Sync it as the game does

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
	uitest.RunCaptureLoop(t, "Entieve HUD capture", g)
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
	g.frames++ // frames count DRAWS: ebiten runs Updates on their own cadence (see rail_capture_test.go)
}

func (g *hudCaptureGame) Layout(w, h int) (int, int) { return 1280, 720 }

// verify screenshots the frame and asserts the drawn geometry. Every
// failure is collected so one run reports all of them.
func (g *hudCaptureGame) verify(img *ebiten.Image) {
	var errs []string
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Sprintf(format, args...))
	}

	// --- 1. Hull portrait: the box carries the placeholder hull's ink.
	if box := g.h.portraitGfx.GetWidget().Rect; uitest.InkIn(img, box) < 200 {
		fail("hull portrait box %v has %d ink pixels, want a drawn portrait",
			box, uitest.InkIn(img, box))
	}

	// --- 2. Health bars deplete at 50/75/90 %: the layer colour inside the
	// fill (sampled left of the printed value), the dark panel in the tail.
	rgb := func(x, y int) (int, int, int) {
		r, gch, b, _ := img.At(x, y).RGBA()
		return int(r >> 8), int(gch >> 8), int(b >> 8)
	}
	hpBox := g.h.healthGfx.GetWidget().Rect
	for _, bar := range []struct {
		name  string
		rect  image.Rectangle
		check func(r, gch, b int) bool
	}{
		{"shield@50%", image.Rect(hpBox.Min.X, hpBox.Min.Y, hpBox.Max.X, hpBox.Min.Y+hudBarH), func(r, gch, b int) bool { return b > 200 && b > r+40 }},
		{"armor@75%", image.Rect(hpBox.Min.X, hpBox.Min.Y+hudBarH+hudHpGap, hpBox.Max.X, hpBox.Min.Y+2*hudBarH+hudHpGap), func(r, gch, b int) bool { return r > 180 && b < 120 }},
		{"structure@90%", image.Rect(hpBox.Min.X, hpBox.Min.Y+2*(hudBarH+hudHpGap), hpBox.Max.X, hpBox.Min.Y+3*hudBarH+2*hudHpGap), func(r, gch, b int) bool { return r > 170 && gch > 170 && b > 170 }},
	} {
		y := (bar.rect.Min.Y + bar.rect.Max.Y) / 2
		r, gch, b := rgb(bar.rect.Min.X+12, y)
		if !bar.check(r, gch, b) {
			fail("%s: fill pixel = (%d,%d,%d), want the layer colour", bar.name, r, gch, b)
		}
		tr, tg, tb := rgb(bar.rect.Max.X-4, y)
		if tr > 80 || tg > 80 || tb > 80 {
			fail("%s: tail pixel = (%d,%d,%d), want the dark panel past the fraction",
				bar.name, tr, tg, tb)
		}
	}

	// --- 3. Strip: above the status panel, clock icon inked, wallet at the
	// right edge.
	strip := g.h.strip.GetWidget().Rect
	status := g.h.status.GetWidget().Rect
	if strip.Max.Y > status.Min.Y {
		fail("strip %v is not above the status %v", strip, status)
	}
	if clock := g.h.strip.Children()[0].GetWidget().Rect; uitest.InkIn(img, clock) < 8 {
		fail("clock icon box %v has too little ink", clock)
	}
	if wallet := g.h.walletLbl.GetWidget().Rect; wallet.Max.X < strip.Max.X-10 || wallet.Max.X > strip.Max.X {
		fail("wallet %v is not right-anchored in the strip %v", wallet, strip)
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
