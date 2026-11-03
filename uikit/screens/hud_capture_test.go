package screens

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/internal/capture"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// hudCaptureFrames is how many full draws run before the screenshot is
// taken (anchors and layout settle on the first frames).
const hudCaptureFrames = 4

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
	env, _ := newChromeEnv(t)
	h := NewHud(env)
	r := NewRack(env)
	h.MountRack(r.Container())
	c := newChromeUI(env)
	c.AddPanel(h.Container(), h)
	c.root.AddChild(h.Cards().Container()) // the game mounts it as top chrome
	c.panels = append(c.panels, r)         // the rack rides the HUD pair; Sync it as the game does

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
	tg.Locks = []sim.Lock{{Target: shipEnt, Progress: 1}}
	if fit, ok := s.Fittings.Get(player); ok && len(fit.HighRT) > 0 {
		// A real activation on the locked ship target: the card's module
		// mini-cell (glyph + cycle ring) reads the sim's own binding, so the
		// fixture fires the high gun at it rather than faking a runtime.
		if pt, ok := s.Transforms.Get(player); ok {
			if tt, ok := s.Transforms.Get(shipEnt); ok {
				tt.Pos = pt.Pos.Add(core.V(2000, 0))
			}
		}
		s.Queue(sim.CmdActivate{Slot: sim.SlotRef{Class: content.SlotHigh, Index: 0}, Target: shipEnt, Confirm: true})
		s.Step()
		// The binding is the sim's own (above); the cycle clock is staged so
		// the frame catches the ring mid-cycle (the live bay is empty here).
		if rt := &fit.HighRT[0]; rt.Active || rt.CycleEnds == 0 {
			rt.Active = true
			if def, ok := s.Content.Module(rt.Def); ok && def.ActivationTime > 0 {
				rt.CycleEnds = s.Time() + 0.75*def.ActivationTime
			}
		}
		// The tick may have pruned the far rock's lock or landed the shot on
		// the target: re-stage the frame's numbers.
		if tgt, ok := s.Ships.Get(shipEnt); ok {
			tgt.Shield = tgt.MaxShield * 0.6
			tgt.Armor = tgt.MaxArmor * 1.0
			tgt.Structure = tgt.MaxStructure * 0.3
		}
	}
	tg.Locks = []sim.Lock{
		{Target: shipEnt, Progress: 1},
		{Target: rockEnt, Progress: 1},
	}

	out := capture.Out("hud_capture.png")
	(&capture.Capture{
		Name:   "Entieve HUD capture",
		Frames: hudCaptureFrames,
		Update: func(int) error {
			for _, p := range c.panels {
				p.Sync()
			}
			c.ui.Update()
			return nil
		},
		Draw: func(_ int, img *ebiten.Image) error {
			c.ui.Draw(img)
			return nil
		},
		Verify: func(img *ebiten.Image) error {
			// Every failure is collected so one run reports all of them.
			var errs []string
			fail := func(format string, args ...any) {
				errs = append(errs, fmt.Sprintf(format, args...))
			}

			// --- 1. Hull portrait: the box carries the placeholder hull's ink.
			if box := h.portraitGfx.GetWidget().Rect; uitest.InkIn(img, box) < 200 {
				fail("hull portrait box %v has %d ink pixels, want a drawn portrait",
					box, uitest.InkIn(img, box))
			}

			// --- 2. The hull name under the portrait: inked in its box.
			if nb := h.hullName.GetWidget().Rect; uitest.InkIn(img, nb) < 40 {
				fail("hull name box %v has %d ink pixels, want the hull's name",
					nb, uitest.InkIn(img, nb))
			}

			// --- 3. Health bars deplete at 50/75/90 %: the layer colour inside the
			// fill (sampled left of the printed value), the dark panel in the tail.
			rgb := func(x, y int) (int, int, int) {
				r, gch, b, _ := img.At(x, y).RGBA()
				return int(r >> 8), int(gch >> 8), int(b >> 8)
			}
			hpBox := h.healthGfx.GetWidget().Rect
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
				// Sample just inside the fill's left edge: the doubled layer
				// glyph and its outline now reach x+18, so x+12 is glyph ink.
				r, gch, b := rgb(bar.rect.Min.X+1, y)
				if !bar.check(r, gch, b) {
					fail("%s: fill pixel = (%d,%d,%d), want the layer colour", bar.name, r, gch, b)
				}
				tr, tg, tb := rgb(bar.rect.Max.X-4, y)
				if tr > 80 || tg > 80 || tb > 80 {
					fail("%s: tail pixel = (%d,%d,%d), want the dark panel past the fraction",
						bar.name, tr, tg, tb)
				}
			}

			// --- 4. Strip: above the status panel, clock icon inked, wallet at the
			// right edge.
			strip := h.strip.GetWidget().Rect
			status := h.status.GetWidget().Rect
			if strip.Max.Y > status.Min.Y {
				fail("strip %v is not above the status %v", strip, status)
			}
			if clock := h.strip.Children()[0].GetWidget().Rect; uitest.InkIn(img, clock) < 8 {
				fail("clock icon box %v has too little ink", clock)
			}
			if wallet := h.walletLbl.GetWidget().Rect; wallet.Max.X < strip.Max.X-10 || wallet.Max.X > strip.Max.X {
				fail("wallet %v is not right-anchored in the strip %v", wallet, strip)
			}

			// --- 2. Target card: glyph pixels on the text line, a 60 % shield
			// fill in bar 0 and a dim track in the empty tail of bar 2.

			boxRect := h.boxes[0].root.GetWidget().Rect
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

			// --- 3. The card's module mini-cell: the active gun's glyph and
			// its cycle ring (palette Accent) in the first cell box.
			if n := h.boxes[0].card.nmods; n != 1 {
				fail("card module cells = %d, want the one gun cycling on it", n)
			}
			cell := targetCardMod(card, 0)
			ring := countTint(img, image.Rect(
				boxRect.Min.X+cell.Min.X, boxRect.Min.Y+cell.Min.Y,
				boxRect.Min.X+cell.Max.X, boxRect.Min.Y+cell.Max.Y), theme.DefaultPalette().Accent)
			if ring < 10 {
				fail("module cell %v carries %d ring pixels, want a drawn cycle ring", cell, ring)
			}

			if len(errs) > 0 {
				return fmt.Errorf("capture %s: %s", out, strings.Join(errs, "; "))
			}
			return nil
		},
		Out: out,
	}).Run(t)
}

// hudCardsCaptureFrames is how many drawn frames settle each phase of
// TestHudCardsPixelsDrawAboveWindows (anchors and layout settle on the
// first frames; the overview's fade is 12).
const hudCardsCaptureFrames = 13

// TestHudCardsPixelsDrawAboveWindows is the target-card twin of
// TestRailPixelsDrawAboveWindows: the Overview's default body covers the
// card band, and the cards must be the pixels there. Pixel readback panics
// before the game starts, so this rides the gated real-loop harness
// (ENTIEVE_CAPTURE=1).
//
// Method: sample four card-interior points over the window, then clear the
// locks (cards gone, window unchanged) and sample the same points: the
// card frame must differ at every point. If the window painted over the
// row, both frames would carry identical window pixels and every comparison
// would come back equal. Samples run INSIDE Draw, on the frame just
// rendered.
func TestHudCardsPixelsDrawAboveWindows(t *testing.T) {
	k, _ := newTestKit(t)
	h := NewHud(k.Env())
	k.AddContainer(h.Container(), widget.AnchorLayoutData{
		StretchHorizontal: true, StretchVertical: true,
	})
	k.AddTopContainer(h.Cards(), widget.AnchorLayoutData{
		HorizontalPosition: widget.AnchorLayoutPositionCenter,
		VerticalPosition:   widget.AnchorLayoutPositionStart,
		Padding:            &widget.Insets{Top: theme.Px(10)},
	})
	s := k.Sim
	var target ecs.Entity
	for _, inf := range s.Grid() {
		if inf.Entity != s.Player() {
			target = inf.Entity
			break
		}
	}
	tg, ok := s.Targetings.Get(s.Player())
	if !ok {
		t.Fatal("player has no targeting component")
	}
	tg.Locks = []sim.Lock{{Target: target, Progress: 1}}
	// The player's selection sits on the card too: the selected card's
	// border takes the palette Accent (2 px) over the plain hairline the
	// unselected row keeps. The pixel pin below reads that ring back. (The
	// picker returns the locked target directly so the frame shows the
	// border alone; the game's picker returns Kit.Selected, pinned in
	// TestHudTargetCardSelectionBorder.)
	h.SetTargetPicker(func() ecs.Entity { return target })
	h.Sync()
	pumpKit(t, k, 1024, 700)
	pumpKit(t, k, 1024, 700)

	box := h.boxes[0].root.GetWidget().Rect
	ovRect := k.OverviewWindow().GetContainer().GetWidget().Rect
	if box.Empty() || !box.In(ovRect) {
		t.Fatalf("fixture: card %v not under the overview %v", box, ovRect)
	}
	w, hgt := theme.Px(hudBoxW), theme.Px(hudBoxH)
	border := image.Point{X: box.Min.X + 1, Y: box.Min.Y + hgt/2}
	points := []image.Point{
		{box.Min.X + w/2, box.Min.Y + 4},
		{box.Min.X + 4, box.Min.Y + hgt/2},
		{box.Min.X + w/2, box.Min.Y + hgt - 5},
		{box.Min.X + w - 5, box.Min.Y + hgt/2},
	}
	sample := func(img *ebiten.Image) []color.RGBA {
		out := make([]color.RGBA, len(points))
		for i, p := range points {
			r, gr, b, a := img.At(p.X, p.Y).RGBA()
			out[i] = color.RGBA{R: uint8(r >> 8), G: uint8(gr >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
		}
		return out
	}

	var withCards *ebiten.Image // screenshot evidence: the cards-over-window frame
	var above, without []color.RGBA
	var borderRGB [3]int // the selected card's border pixel
	out := capture.Out("hud_cards_top.png")
	var cap *capture.Capture
	cap = &capture.Capture{
		Name: "Entieve target cards over window capture",
		Size: image.Pt(1024, 700),
		Update: func(int) error {
			h.Sync()
			k.Sync()
			return k.Update()
		},
		Draw: func(n int, img *ebiten.Image) error {
			k.Draw(img)
			switch {
			case n <= hudCardsCaptureFrames:
			case n == hudCardsCaptureFrames+1:
				above = sample(img)
				withCards = img
				br, bg, bb, _ := img.At(border.X, border.Y).RGBA()
				borderRGB = [3]int{int(br >> 8), int(bg >> 8), int(bb >> 8)}
				tg.Locks = nil // the cards vanish from the next frame on
			case n <= hudCardsCaptureFrames+2+hudGhostFrames:
				// The blink window: a cleared lock keeps its card as a ghost
				// for hudGhostFrames before it disappears, so the "without"
				// sample waits for the blink to finish.
			default:
				without = sample(img)
				if err := capture.Save(out, withCards); err != nil {
					return err
				}
				cap.Stop()
			}
			return nil
		},
		Verify: func(*ebiten.Image) error {
			var errs []string
			for i, p := range points {
				if above[i] == without[i] {
					errs = append(errs, fmt.Sprintf("point %v identical with and without the cards (%s) — "+
						"the cards are not painting over the window", p, formatRGBA(above[i])))
				}
			}
			// The selected card's border is the palette Accent (amber), not
			// the faint grey hairline: red leads blue by a wide margin.
			if borderRGB[0] < 150 || borderRGB[0] < borderRGB[2]+40 {
				errs = append(errs, fmt.Sprintf("selected-card border at %v = %v, want the Accent amber ring", border, borderRGB))
			}
			if len(errs) == 0 {
				return nil
			}
			for i, p := range points {
				t.Logf("point %v: cards above = %v, without = %v",
					p, formatRGBA(above[i]), formatRGBA(without[i]))
			}
			return fmt.Errorf("capture %s: %s", out, strings.Join(errs, "; "))
		},
	}
	cap.Run(t)
}
