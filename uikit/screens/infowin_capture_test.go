package screens

import (
	"fmt"
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
// separator hairline, amber hover band) — through a real ebiten game
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
		Maker:       "corp-borealis",
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
// on screen with label ink, its hover band reads amber and its separator
// draws; the details window is on screen with title-bar and body ink.
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
		// The hovered row's right half carries only the amber band fill.
		hov := m.RowRect(1)
		band := image.Rect(hov.Min.X+hov.Dx()/2, hov.Min.Y, hov.Max.X-2, hov.Max.Y-2)
		amber := 0
		for y := band.Min.Y; y < band.Max.Y; y++ {
			for x := band.Min.X; x < band.Max.X; x++ {
				rr, gg, bb, _ := img.At(x, y).RGBA()
				r8, g8, b8 := int(rr>>8), int(gg>>8), int(bb>>8)
				// The 40-alpha amber band over the near-black fill: red
				// lifted above both channels, green still over blue.
				if r8 > 35 && r8 > g8+5 && g8 > b8 {
					amber++
				}
			}
		}
		if amber < 20 {
			fail("hover band %v drew %d amber pixels, want ≥ 20", band, amber)
		}
		// The separator hairline: brighter than the fill, centered in
		// its band, spanning most of the popup's inset width.
		sep := m.RowRect(2)
		y := (sep.Min.Y + sep.Max.Y) / 2
		lit := 0
		for x := sep.Min.X + 14; x < sep.Max.X-14; x += 4 {
			rr, _, _, _ := img.At(x, y).RGBA()
			if int(rr>>8) > 24 {
				lit++
			}
		}
		if want := (sep.Dx() - 28) / 4; lit < want-2 {
			fail("separator row drew %d lit samples of %d, want the hairline", lit, want)
		}
		// Disabled entry: its label draws, but markedly fainter than
		// the enabled first entry (the grayed look, never hidden).
		enabled := uitest.InkIn(img, m.RowRect(0))
		disabled := uitest.InkIn(img, m.RowRect(3))
		if disabled < 3 {
			fail("disabled entry drew %d ink pixels, want its grayed label", disabled)
		}
		if enabled > 0 && disabled*3 > enabled {
			fail("disabled entry is not grayed: %d ink vs %d for the enabled entry", disabled, enabled)
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
		// The header row draws: the entity icon on its dark plate, the hull
		// name on the column beside it. The plate is opaque where the body
		// is translucent, with a Border hairline: sample its top edge
		// against its own fill (that stroke is what makes it read as a box
		// over the dark world).
		if pl := g.k.svc.info.HeroPlate(); pl == nil || g.k.svc.info.Hero() == nil {
			fail("header has no icon plate")
		} else if n := uitest.InkIn(img, pl.GetWidget().Rect); n < 40 {
			fail("icon plate %v drew %d ink pixels, want the entity icon", pl.GetWidget().Rect, n)
		} else {
			box := pl.GetWidget().Rect
			mid := box.Min.X + box.Dx()/2
			er, eg, eb, _ := img.At(mid, box.Min.Y).RGBA()
			fr, fg, fb, _ := img.At(mid, box.Min.Y+theme.Px(infoPlatePad/2)).RGBA()
			if uitest.AbsInt(int(er>>8)-int(fr>>8)) < 10 &&
				uitest.AbsInt(int(eg>>8)-int(fg>>8)) < 10 &&
				uitest.AbsInt(int(eb>>8)-int(fb>>8)) < 10 {
				fail("icon plate %v: its top edge = its fill (%d,%d,%d) — no hairline, the box does not read",
					box, fr>>8, fg>>8, fb>>8)
			}
		}
		// The maker line leads with the dropped corporation art at the corp
		// token (the corp icons replace the vector emblems here).
		if e := g.k.svc.info.Emblem(); e == nil || e.Image == nil {
			fail("maker line has no corporation mark")
		} else if b := e.Image.Bounds(); b.Dx() != theme.Px(theme.IconCorpInfo) {
			fail("maker mark box = %v, want %d px", b, theme.Px(theme.IconCorpInfo))
		} else {
			box := e.GetWidget().Rect
			// The hull name sits in the header column directly above the
			// mark: the column's rows down to the mark must carry the bold
			// title's ink.
			top := box.Min.Y
			if pl := g.k.svc.info.HeroPlate(); pl != nil {
				top = pl.GetWidget().Rect.Min.Y
			}
			name := image.Rect(box.Min.X, top, r.Max.X-theme.Px(theme.ContentsPad), box.Min.Y)
			if n := uitest.InkIn(img, name); n < 20 {
				fail("header column %v drew %d ink pixels, want the hull name", name, n)
			}
			br, bg, bb, _ := img.At(box.Min.X-3, box.Min.Y+box.Dy()/2).RGBA()
			diff := 0
			for y := box.Min.Y; y < box.Max.Y; y++ {
				for x := box.Min.X; x < box.Max.X; x++ {
					pr, pg, pb, _ := img.At(x, y).RGBA()
					if uitest.AbsInt(int(pr>>8)-int(br>>8)) > 6 ||
						uitest.AbsInt(int(pg>>8)-int(bg>>8)) > 6 ||
						uitest.AbsInt(int(pb>>8)-int(bb>>8)) > 6 {
						diff++
					}
				}
			}
			if diff < 20 {
				fail("maker mark %v drew %d non-body pixels, want the corp art", box, diff)
			}
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
