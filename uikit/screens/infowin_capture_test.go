package screens

import (
	"fmt"
	"image/color"
	"math"

	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/hajimehoshi/ebiten/v2"
)

// infoCaptureFrames is how many full draws run before the screenshot is
// taken (anchors and layout settle on the first frames).
// 13 draws: Show Info opens with the 12-frame fade — captures pin
// the settled state, never a mid-fade one (winanim.go).
const infoCaptureFrames = 13

// TestInfoWindowCaptureFrame renders the two Show Info surfaces — a
// floating details window (hull page: header, prose, section rows) and a
// context menu open beside it (bold first entry, disabled entry grayed,
// separator hairline, palette hover band) — through a real ebiten game
// loop, checks the drawn pixels and screenshots the frame.
//
// Skipped unless ENTIEVE_CAPTURE=1 so the regular suite stays windowless.
// The PNG lands in $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
func TestInfoWindowCaptureFrame(t *testing.T) {
	if os.Getenv("ENTIEVE_CAPTURE") == "" {
		t.Skip("set ENTIEVE_CAPTURE=1 to render a capture frame")
	}
	uitest.SkipIfLoopRan(t)
	outDir := os.Getenv("ENTIEVE_CAPTURE_DIR")
	if outDir == "" {
		outDir = "/tmp/opencode"
	}

	k, _ := newTestKit(t)
	env := &testEnv{Session: k.Env(), Sim: k.Sim, Content: k.Content}
	fm := useFakeMouse(t)

	// A hull page with every block (the content stream authors the
	// description/origin fields — the fixture carries them).
	fixtureShip(t, env, "ship-capture-hull", &content.ShipDef{
		Name:        "Kestrel Prime",
		Class:       "frigate",
		Origin:      "moravian",
		Description: "A heritage frigate restored from the old works: light, fast and stubbornly analogue where it counts.",
		Slots:       content.Slots{High: 4, Mid: 4, Low: 2},
		Shield:      700, Armor: 600, Structure: 800,
		Resists:   content.Resists{EM: 0.25, Thermal: 0.4, Kinetic: 0.5, Explosive: 0.1},
		Capacitor: 1200, Recharge: 320,
		MaxSpeed: 340, Agility: 3.2, Mass: 1.1e6, WarpSpeed: 4.5,
		SigRadius: 42, ScanResolution: 450, MaxTargetRange: 60000, MaxTargets: 7,
		Cargo: 450, OreHold: 150,
	})
	k.ShowInfo(widgets.ShipRef("ship-capture-hull"))

	// A rack-shaped menu open beside the window.
	m := k.Menu()
	m.Open([]widgets.MenuEntry{
		{Label: "Show Info", Bold: true, Action: func() {}},
		{Label: "Show Info — Fusion Charge", Action: func() {}},
		widgets.MenuSep,
		{Label: "Cycle charge", Disabled: true, Action: func() {}},
	}, image.Pt(760, 140))
	// Park the (fake) cursor on the second row: Kit.Update's menu poll
	// reads the live cursor for the hover band, exactly as in game — and
	// the menu's live poll is the RAW ebiten cursor (menu.go's
	// pollCursor), not the ebitenui seam above, so pin it to the parked
	// point through its test seam or the band would follow the operator's
	// real pointer.
	hov := m.RowRect(1)
	cx, cy := (hov.Min.X+hov.Max.X)/2, (hov.Min.Y+hov.Max.Y)/2
	fm.at(cx, cy)
	t.Cleanup(widgets.SetPollCursorForTest(func() (int, int) { return cx, cy }))
	m.Hover(m.RowRect(1).Min.Add(image.Pt(6, 6)))

	g := &infoCaptureGame{k: k, out: filepath.Join(outDir, "infowin_capture.png")}
	uitest.RunCaptureLoop(t, "Entieve Show Info capture", g)
	if g.err != nil {
		t.Fatal(g.err)
	}
	if !g.saved {
		t.Fatal("capture never rendered")
	}
	t.Logf("capture written to %s", g.out)
}

// infoCaptureGame drives the kit for a few frames, then screenshots and
// verifies the drawn menu and details window.
type infoCaptureGame struct {
	k      *Kit
	out    string
	frames int
	last   *ebiten.Image
	saved  bool
	err    error
}

func (g *infoCaptureGame) Update() error {
	if g.saved || g.err != nil {
		return ebiten.Termination
	}
	g.k.Sync()
	if err := g.k.Update(); err != nil {
		g.err = err
		return ebiten.Termination
	}
	if g.frames > infoCaptureFrames && g.last != nil {
		g.verify(g.last)
		return ebiten.Termination
	}
	return nil
}

func (g *infoCaptureGame) Draw(screen *ebiten.Image) {
	img := ebiten.NewImage(1280, 720)
	g.k.Draw(img)
	screen.DrawImage(img, nil)
	g.last = img
	g.frames++ // frames count DRAWS: ebiten runs Updates on their own cadence (see rail_capture_test.go)
}

func (g *infoCaptureGame) Layout(w, h int) (int, int) { return 1280, 720 }

// verify screenshots the frame and asserts the pixels: the menu popup is
// on screen with label ink, its hover band reads the palette's hover tint
// and its separator draws; the details window is on screen with title-bar and body ink.
// Every failure is collected so one run reports all of them.
func (g *infoCaptureGame) verify(img *ebiten.Image) {
	var errs []string
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Sprintf(format, args...))
	}
	screen := img.Bounds()

	// --- The context menu.
	m := g.k.Menu()
	if !m.IsOpen() {
		fail("menu is not open")
	} else {
		r := m.Rect()
		if !r.In(screen) {
			fail("menu rect %v not on screen", r)
		}
		if n := uitest.InkIn(img, r); n < 40 {
			fail("menu %v drew %d ink pixels, want ≥ 40", r, n)
		}
		// The hovered row's right half carries only the hover band fill:
		// the palette's Hover role over the popup's fill (hue-agnostic —
		// whatever the injected palette says, the band must be its tint).
		pal := g.k.Env().Pal()
		hov := m.RowRect(1)
		band := image.Rect(hov.Min.X+hov.Dx()/2, hov.Min.Y, hov.Max.X-2, hov.Max.Y-2)
		tinted := 0
		for y := band.Min.Y; y < band.Max.Y; y++ {
			for x := band.Min.X; x < band.Max.X; x++ {
				if f, ok := tintFrac(img.At(x, y), pal.BGHard, pal.Hover); ok && f > 0.02 && f < 0.5 {
					tinted++
				}
			}
		}
		if tinted < 20 {
			fail("hover band %v drew %d tinted pixels, want ≥ 20", band, tinted)
		}
		// The separator hairline: the Border role over the fill, centered
		// in its band, spanning most of the popup's inset width.
		sep := m.RowRect(2)
		y := (sep.Min.Y + sep.Max.Y) / 2
		lit := 0
		for x := sep.Min.X + 14; x < sep.Max.X-14; x += 4 {
			if f, ok := tintFrac(img.At(x, y), pal.BGHard, pal.Border); ok && f > 0.3 {
				lit++
			}
		}
		if want := (sep.Dx() - 28) / 4; lit < want-2 {
			fail("separator row drew %d lit samples of %d, want the hairline", lit, want)
		}
		// Disabled entry: its label draws, but markedly fainter than
		// the enabled first entry (the grayed look, never hidden). Ink is
		// measured against the popup fill, not an absolute brightness.
		enabled := inkMass(img, m.RowRect(0), pal.BGHard)
		disabled := inkMass(img, m.RowRect(3), pal.BGHard)
		if disabled < 20 {
			fail("disabled entry drew %d ink mass, want its grayed label", disabled)
		}
		if enabled > 0 && disabled*2 > enabled {
			fail("disabled entry is not grayed: %d ink mass vs %d for the enabled entry", disabled, enabled)
		}
	}

	// --- The details window.
	w := g.k.InfoWindow()
	r := w.GetContainer().GetWidget().Rect
	if !w.GetContainer().GetWidget().IsVisible() {
		fail("info window is not visible")
	}
	if !r.In(screen) {
		fail("info window %v not on screen", r)
	} else {
		title := image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+theme.TitleBarH)
		if n := uitest.InkIn(img, title); n < 60 {
			fail("title bar %v drew %d ink pixels, want ≥ 60", title, n)
		}
		body := image.Rect(r.Min.X+4, r.Min.Y+theme.TitleBarH+4, r.Max.X-4, r.Max.Y-4)
		if n := uitest.InkIn(img, body); n < 200 {
			fail("body %v drew %d ink pixels, want ≥ 200", body, n)
		}
		// The header line carries the hull name in the bold title face.
		head := image.Rect(r.Min.X+8, r.Min.Y+theme.TitleBarH+6, r.Max.X-8, r.Min.Y+theme.TitleBarH+34)
		if n := uitest.InkIn(img, head); n < 40 {
			fail("header line %v drew %d ink pixels, want the hull name", n, n)
		}
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

// tintFrac reports how far c has moved from base toward role (0 = base,
// 1 = role), using only the channels where the role is a real move; ok is
// false when the channels disagree, i.e. the pixel is not that tint.
func tintFrac(c color.Color, base, role color.RGBA) (f float64, ok bool) {
	cr, cg, cb, _ := c.RGBA()
	got := [3]float64{float64(cr >> 8), float64(cg >> 8), float64(cb >> 8)}
	from := [3]float64{float64(base.R), float64(base.G), float64(base.B)}
	ref := [3]float64{float64(role.R) - from[0], float64(role.G) - from[1], float64(role.B) - from[2]}
	lo, hi, n := math.Inf(1), math.Inf(-1), 0
	for i := range got {
		if math.Abs(ref[i]) < 12 {
			continue // this channel barely moves: not evidence
		}
		r := (got[i] - from[i]) / ref[i]
		lo, hi = math.Min(lo, r), math.Max(hi, r)
		n++
	}
	if n < 2 || hi-lo > 0.2 {
		return 0, false
	}
	return (lo + hi) / 2, true
}

// inkMass sums how far every pixel's brightest channel rises above the
// popup fill — the palette-relative ink measure (InkIn hardcodes 90, which
// the muted roles no longer clear). It weighs brightness, so a grayed
// label reads as a fraction of the enabled one instead of a pixel count.
func inkMass(img *ebiten.Image, box image.Rectangle, base color.RGBA) int {
	bm := int(max(base.R, max(base.G, base.B)))
	sum := 0
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			r, gc, b, a := img.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			if d := int(max(r>>8, max(gc>>8, b>>8))) - bm; d > 0 {
				sum += d
			}
		}
	}
	return sum
}
