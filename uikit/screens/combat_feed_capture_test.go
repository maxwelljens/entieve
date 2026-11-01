package screens

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/internal/capture"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// combatFeedFrames is how many full draws run before the screenshot (the
// anchors and the feed's seat settle on the first frames).
const combatFeedFrames = 4

// TestCombatFeedCaptureFrame renders the combat feed through a real ebiten
// loop: a hit taken, a hit dealt and a scramble dealt, each with its
// important span in its own palette colour (Danger red for what hit you, Info
// blue for what you dealt), sitting above the ship status. It also pins the
// overlay rule: the feed is clear of the bottom pair, which keeps its rect.
//
// Skipped unless ENTIEVE_CAPTURE=1 so the regular suite stays windowless. The
// PNG lands in $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
func TestCombatFeedCaptureFrame(t *testing.T) {
	env, _ := newChromeEnv(t)
	h := NewHud(env)
	c := newChromeUI(env)
	c.AddPanel(h.Container(), h)
	c.root.AddChild(h.Feed().Container()) // the game mounts it as top chrome

	s := env.Sim
	p := s.Player()
	var giver ecs.Entity
	for _, inf := range s.Grid() {
		if inf.Entity != p {
			giver = inf.Entity
			break
		}
	}
	if giver == 0 {
		t.Fatal("fixture: no other entity")
	}
	target, ok := h.cv.Info(giver)
	if !ok {
		t.Fatal("fixture: the target has no Info")
	}
	const foe = "Kuštor"
	h.Push([]sim.Event{
		{Kind: sim.EvCombat, Phase: sim.CombatDamage, Actor: giver, Target: p, Value: 210, Name: foe},
		{Kind: sim.EvCombat, Phase: sim.CombatDamage, Actor: p, Target: giver, Value: 42},
		{Kind: sim.EvCombat, Phase: sim.CombatWeb, Actor: p, Target: giver},
	})
	h.Sync()

	out := capture.Out("combat_feed.png")
	(&capture.Capture{
		Name:   "Entieve combat feed capture",
		Frames: combatFeedFrames,
		Update: func(int) error {
			h.Sync()
			c.ui.Update()
			return nil
		},
		Draw: func(_ int, img *ebiten.Image) error {
			c.ui.Draw(img)
			return nil
		},
		Verify: func(img *ebiten.Image) error {
			var errs []string
			fail := func(format string, args ...any) {
				errs = append(errs, fmt.Sprintf(format, args...))
			}
			feed := h.feed.col.GetWidget().Rect
			strip := h.strip.GetWidget().Rect
			pair := h.pair.GetWidget().Rect
			if feed.Empty() {
				return fmt.Errorf("capture %s: the feed never laid out", out)
			}
			if feed.Max.Y > strip.Min.Y || feed.Min.Y < 0 {
				fail("feed %v is not above the strip %v", feed, strip)
			}
			if feed.Min.X != strip.Min.X {
				fail("feed %v is not aligned on the status column's left edge %v", feed, strip.Min.X)
			}
			if gap := strip.Min.Y - feed.Max.Y; gap != theme.Px(hudFeedGap) {
				fail("feed gap to the strip = %d px, want %d", gap, theme.Px(hudFeedGap))
			}
			if feed.Overlaps(pair) {
				fail("feed %v overlaps the bottom pair %v (it must be an overlay, not a column member)", feed, pair)
			}
			// The three lines, oldest first: taken, dealt, webbed.
			rows := h.feed.rowsAt
			taken, dealt, webbed := rows[hudFeedLines-3], rows[hudFeedLines-2], rows[hudFeedLines-1]
			for name, row := range map[string]*hudFeedRow{"taken": &taken, "dealt": &dealt, "webbed": &webbed} {
				if row.key.Label == "" || uitest.InkIn(img, row.key.GetWidget().Rect) < 8 {
					fail("%s line key %q has no ink in %v", name, row.key.Label, row.key.GetWidget().Rect)
				}
			}
			red := countTint(img, taken.key.GetWidget().Rect, env.Pal().Danger)
			blue := countTint(img, dealt.key.GetWidget().Rect, env.Pal().Info)
			if red < 8 {
				fail("taken key %q carries %d red pixels, want the Danger span", taken.key.Label, red)
			}
			if blue < 8 {
				fail("dealt key %q carries %d blue pixels, want the Info span", dealt.key.Label, blue)
			}
			// The plain runs stay neutral (not the span colour).
			if n := countTint(img, taken.pre.GetWidget().Rect, env.Pal().Danger); n > 4 {
				fail("the taken line's plain run carries %d red pixels (only the key span is coloured)", n)
			}
			if !strings.HasSuffix(taken.key.Label, "210") || dealt.key.Label != "42" || webbed.key.Label != "web "+target.Name {
				fail("lines read %q/%q/%q, want the amount, the amount and the web verb",
					taken.key.Label, dealt.key.Label, webbed.key.Label)
			}
			if len(errs) > 0 {
				return fmt.Errorf("capture %s: %s", out, strings.Join(errs, "; "))
			}
			return nil
		},
		Out: out,
	}).Run(t)
}

// countTint counts the pixels inside box that carry c (within a small
// tolerance, so antialiasing at the glyph edges still reads as the tint).
func countTint(img *ebiten.Image, box image.Rectangle, c color.RGBA) int {
	n := 0
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			if uitest.AbsInt(int(r>>8)-int(c.R)) <= 26 &&
				uitest.AbsInt(int(g>>8)-int(c.G)) <= 26 &&
				uitest.AbsInt(int(b>>8)-int(c.B)) <= 26 {
				n++
			}
		}
	}
	return n
}
