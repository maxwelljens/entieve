package screens

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/internal/capture"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// profCaptureFrames settles a phase before its sample is taken: the kit's
// windows open with the 12-frame fade, and captures pin the settled state.
const profCaptureFrames = 14

// proficiencyCaptureFixture is the capture's model: the shipped fourteen
// families with their authored prerequisite edges and a plausible trained
// state (one family at the cap, one shut branch), so the evidence shows the
// graph the game actually draws now that the sim maps its own families on.
func proficiencyCaptureFixture() []ProfFamily {
	levels := map[string]int{
		"autocannon": 3, "artillery": 2, "mining": 5, "shield": 3, "armor": 1,
		"capacitor": 2, "propulsion": 2, "frigate": 1, "industrial": 1,
		"barge": 1, "rocket": 0, "ewar": 0,
	}
	// The world-start families wear the starter tag, exactly as the sim's
	// starterFamily reads them.
	starters := map[string]bool{}
	if c, err := content.Load(content.FS()); err == nil {
		if w, ok := c.World("world-showcase"); ok {
			if h, ok := c.Ship(w.Start.Ship); ok && h.Family != "" {
				starters[h.Family] = true
			}
			for _, mid := range w.Start.Fit {
				if m, ok := c.Module(mid); ok && m.Family != "" {
					starters[m.Family] = true
				}
			}
		}
	}
	ids := content.ProficiencyFamilyIDs()
	out := make([]ProfFamily, 0, len(ids))
	for _, id := range ids {
		var pre []ProfPrereq
		meets, gate := true, ""
		for _, p := range content.Prerequisites(id) {
			pre = append(pre, ProfPrereq{Family: p.Family, Level: p.Level})
			if levels[p.Family] < p.Level {
				meets = false
				if gate == "" {
					// The sim's sentence for the first unmet edge (the
					// fixture's entry gates are 0).
					gate = content.ProficiencyRequirement(p.Family, p.Level)
				}
			}
		}
		lvl := levels[id]
		next := 0
		xp := 0
		if lvl < content.ProficiencyMax {
			next = 10 * (lvl + 1) * (lvl + 1) // the sim's total-XP ladder
			xp = next * 2 / 3                 // a partly filled bar
		} else {
			xp = 250 // the cap's clamp
		}
		out = append(out, ProfFamily{
			Family: id, Name: content.FamilyLabel(id),
			Level: lvl, XP: xp, XPNext: next, MaxLevel: content.ProficiencyMax,
			Prereqs: pre, Meets: meets, Gate: gate, Starter: starters[id],
		})
	}
	return out
}

// TestProficiencyCaptureFrame: the proficiency layer open over the world,
// its overview window above it and one ordinary window above both. The
// pins, all inside the loop (pixel readback panics before the game starts):
//
//   - every sampled pixel of the finances window is IDENTICAL with and
//     without the layer mounted ("does not obscure windows"), while a
//     control point on bare layer space moves (the layer is opaque over
//     the world);
//   - two panels painted from the same model are byte-identical (the
//     deterministic layout, at the pixel level);
//   - the overview window drew its totals and skill list (ink in its body).
//
// Skipped unless ENTIEVE_CAPTURE=1 so the regular suite stays windowless.
// The PNG lands in $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
func TestProficiencyCaptureFrame(t *testing.T) {
	if testing.Short() {
		t.Skip("capture")
	}
	uitest.UseScale(t, 1)
	// A still pointer at the layer's corner: neither paint sees a hover,
	// so the twin comparison is the model alone.
	in := &scriptPointer{}
	k, _ := newTestKitWith(t, widgets.NewPointer(in))
	fin := NewFinances(k.Env())
	k.AddWindow(fin.Win())
	fin.Win().SetLocation(image.Rect(900, 380, 1280, 720))
	// Hide the kit's two always-on windows: the evidence is the tree, its
	// overview and one floating window above it (Finances) as the stacking
	// proof.
	k.SetCoreVisible(false)
	p := NewProficiency(k.Env(), proficiencyCaptureFixture, nil)
	ov := NewProficiencyOverview(k.Env(), proficiencyCaptureFixture)
	// The second paint of the same model, never mounted: its canvas is the
	// determinism evidence.
	twin := NewProficiency(k.Env(), proficiencyCaptureFixture, nil)
	pumpKit(t, k, 1280, 720)
	pumpKit(t, k, 1280, 720)
	ov.Sync()

	fr := fin.Win().GetContainer().GetWidget().Rect
	// Opaque window pixels: the title bar (the opaque BGHard fill, text
	// included) and a ledger cell. The body fill is translucent by design,
	// so it composites over whatever lies behind it, the world included;
	// those pixels cannot be expected to match and are not the pin.
	points := []image.Point{
		{fr.Min.X + 120, fr.Min.Y + 6},
		{fr.Min.X + fr.Dx()/2, fr.Min.Y + 6},
		{fr.Max.X - 60, fr.Min.Y + 6},
		{fr.Max.X - 20, fr.Max.Y - 20},
	}
	control := image.Pt(900, 600)

	sample := func(img *ebiten.Image, pts []image.Point) [][4]uint32 {
		out := make([][4]uint32, len(pts))
		for i, pt := range pts {
			r, g, b, a := img.At(pt.X, pt.Y).RGBA()
			out[i] = [4]uint32{r, g, b, a}
		}
		return out
	}

	var before [][4]uint32
	var controlBefore [4]uint32
	var after [][4]uint32
	var controlAfter [4]uint32
	var pixelsA, pixelsB []byte

	out := capture.Out("proficiency_capture.png")
	var cap *capture.Capture
	cap = &capture.Capture{
		Name: "Entieve proficiency capture",
		Update: func(n int) error {
			k.Pointer().Begin()
			k.Sync()
			p.Sync()
			twin.Sync()
			ov.Sync()
			return k.Update()
		},
		Draw: func(n int, img *ebiten.Image) error {
			// The world's place: the game draws it before the interface,
			// so the frame starts opaque (the window bodies are
			// translucent and composite over whatever is behind them).
			img.Fill(color.RGBA{R: 16, G: 24, B: 32, A: 255})
			k.Draw(img)
			switch {
			case n == profCaptureFrames:
				before = sample(img, append(points, control))
				controlBefore = before[len(before)-1]
			case n == profCaptureFrames+1:
				// The layer mounts below every window (no fade: it is a
				// layer, not a window) and paints from the next frame;
				// the overview is an ordinary window, above it.
				k.AddBackdropWindow(p.Win())
				k.AddWindow(ov.Win())
				// The game places the row clear of the rail gutter
				// (anchorRect's floor); the capture shows that default.
				r := ov.Win().GetContainer().GetWidget().Rect
				x := theme.Px(10 + RailInset)
				ov.Win().SetLocation(image.Rect(x, r.Min.Y, x+r.Dx(), r.Max.Y))
			case n == 2*profCaptureFrames+2:
				sampled := sample(img, append(points, control))
				controlAfter = sampled[len(sampled)-1]
				after = sampled[:len(points)]
				pixelsA = readPixels(t, p.Canvas())
				pixelsB = readPixels(t, twin.Canvas())
				if err := capture.Save(out, img); err != nil {
					return err
				}
				cap.Stop()
			}
			return nil
		},
		Verify: func(*ebiten.Image) error {
			if before == nil || after == nil {
				return fmt.Errorf("capture %s: phases did not run (window samples %d)", out, len(after))
			}
			var errs []string
			for i, pt := range points {
				if before[i] != after[i] {
					errs = append(errs, fmt.Sprintf(
						"window pixel %v moved between the frames without and with the layer: %v → %v",
						pt, before[i], after[i]))
				}
			}
			if controlBefore == controlAfter {
				errs = append(errs, fmt.Sprintf(
					"control point %v is identical with and without the layer: the world is not covered", control))
			}
			if !bytes.Equal(pixelsA, pixelsB) {
				errs = append(errs, "two renders of the same model differ (the layout is not deterministic)")
			}
			// The overview's canvas carries its totals and skill list.
			if r, _ := uitest.InkBounds(ov.Canvas(), ov.Canvas().Bounds()); r.Empty() {
				errs = append(errs, "the overview canvas is blank")
			}
			wb := ov.Win().GetContainer().GetWidget().Rect
			if n := uitest.InkIn(ov.Canvas(), ov.Canvas().Bounds()); n < 300 {
				errs = append(errs, fmt.Sprintf("the overview drew %d ink pixels, want its totals and skill list", n))
			}
			if wb.Empty() || !wb.In(image.Rect(0, 0, 1280, 720)) {
				errs = append(errs, fmt.Sprintf("the overview window %v is not on screen", wb))
			}
			if len(errs) == 0 {
				return nil
			}
			for _, e := range errs {
				t.Log(e)
			}
			return fmt.Errorf("capture %s: %d checks failed", out, len(errs))
		},
	}
	cap.Run(t)
}

// TestProficiencyFamilyCardCapture shows the node click's card: the tree
// under it, the card beside the clicked family with its level line, what
// the family is for and the practice list. Pins: the card is fully on
// screen over the tree, its title bar and body bands carry ink, and the
// practice block spans several inked scanlines (the page kept its list).
func TestProficiencyFamilyCardCapture(t *testing.T) {
	if testing.Short() {
		t.Skip("capture")
	}
	uitest.UseScale(t, 1)
	in := &scriptPointer{}
	k, _ := newTestKitWith(t, widgets.NewPointer(in))
	k.SetCoreVisible(false)
	p := NewProficiency(k.Env(), proficiencyCaptureFixture, nil)
	p.Sync()
	k.AddBackdropWindow(p.Win())
	pumpKit(t, k, 1280, 720)

	// The click that opens the card: a point inside the mining node.
	var node profNode
	for _, n := range p.nodes {
		if n.Family == "mining" {
			node = n
			break
		}
	}
	if node.Family == "" {
		t.Fatal("fixture: no mining node")
	}
	at := image.Pt(node.Rect.Min.X+node.Rect.Dx()/2, node.Rect.Min.Y+node.Rect.Dy()/2)
	k.OpenProfFamily(widgets.ShowProfFamily{Family: "mining", At: at, Level: 5})

	out := capture.Out("proficiency_family_card.png")
	var cap *capture.Capture
	cap = &capture.Capture{
		Name:   "Entieve proficiency family card",
		Frames: profCaptureFrames,
		Update: func(int) error {
			k.Pointer().Begin()
			k.Sync()
			p.Sync()
			return k.Update()
		},
		Draw: func(_ int, img *ebiten.Image) error {
			img.Fill(color.RGBA{R: 16, G: 24, B: 32, A: 255})
			k.Draw(img)
			return nil
		},
		Verify: func(img *ebiten.Image) error {
			var errs []string
			win := k.profCardWin
			if win == nil || !win.IsOpen() {
				return fmt.Errorf("capture %s: the family card did not open", out)
			}
			r := win.Win().GetContainer().GetWidget().Rect
			if !r.In(image.Rect(0, 0, 1280, 720)) {
				errs = append(errs, fmt.Sprintf("card %v is not fully on screen", r))
			}
			if win.Family() != "mining" {
				errs = append(errs, fmt.Sprintf("card names %q, want mining", win.Family()))
			}
			if win.title.Label != "Mining" {
				errs = append(errs, fmt.Sprintf("title bar reads %q", win.title.Label))
			}
			// The body bands: the description and the practice list.
			for name, w := range map[string]*widget.Text{
				"description": win.desc,
				"practice":    win.practice,
			} {
				if n := uitest.InkIn(img, w.GetWidget().Rect); n < 40 {
					errs = append(errs, fmt.Sprintf("%s drew %d ink pixels, want a readable block", name, n))
				}
			}
			// The practice list is multi-line: several inked scanlines.
			pb := win.practice.GetWidget().Rect
			rows := 0
			for y := pb.Min.Y; y < pb.Max.Y; y++ {
				if uitest.InkIn(img, image.Rect(pb.Min.X, y, pb.Max.X, y+1)) > 0 {
					rows++
				}
			}
			if rows < 20 {
				errs = append(errs, fmt.Sprintf("practice block spans %d inked scanlines, want its bulleted lines", rows))
			}
			if len(errs) == 0 {
				return nil
			}
			return fmt.Errorf("capture %s: %s", out, strings.Join(errs, "; "))
		},
		Out: out,
	}
	cap.Run(t)
}

// readPixels copies an image's pixels for the byte comparison.
func readPixels(t *testing.T, img *ebiten.Image) []byte {
	t.Helper()
	b := img.Bounds()
	buf := make([]byte, 4*b.Dx()*b.Dy())
	img.ReadPixels(buf)
	return buf
}
