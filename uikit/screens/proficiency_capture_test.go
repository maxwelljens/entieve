package screens

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/entieve/entieve/internal/capture"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/hajimehoshi/ebiten/v2"
)

// profCaptureFrames settles a phase before its sample is taken: the kit's
// windows open with the 12-frame fade, and captures pin the settled state.
const profCaptureFrames = 14

// proficiencyCaptureFixture is the capture's model: the same twelve
// families, four ranks and two gates as the game's placeholder
// (game/proficiency.go), so the evidence shows the shape the feature
// ships with. Test data only: the sim's model replaces it.
func proficiencyCaptureFixture() []ProfFamily {
	return []ProfFamily{
		{Family: "navigation", Name: "Navigation", Level: 5, MaxLevel: 5, Meets: true, Starter: true},
		{Family: "shield", Name: "Shield Operation", Level: 3, MaxLevel: 5, XP: 100, XPNext: 240, Meets: true,
			Prereqs: []ProfPrereq{{Family: "navigation", Level: 2}}},
		{Family: "armour", Name: "Armour Plating", Level: 1, MaxLevel: 5, XP: 40, XPNext: 120, Meets: true,
			Prereqs: []ProfPrereq{{Family: "shield", Level: 1}}},
		{Family: "autocannon", Name: "Autocannons", Level: 3, MaxLevel: 5, XP: 240, XPNext: 300, Meets: true, Starter: true},
		{Family: "artillery", Name: "Artillery", Level: 2, MaxLevel: 5, XP: 90, XPNext: 160, Meets: true, Starter: true},
		{Family: "salvage", Name: "Salvage", Level: 0, MaxLevel: 3, XP: 0, XPNext: 60, Meets: true,
			Prereqs: []ProfPrereq{{Family: "autocannon", Level: 2}}},
		{Family: "mining", Name: "Mining", Level: 4, MaxLevel: 5, XP: 700, XPNext: 900, Meets: true, Starter: true},
		{Family: "prospecting", Name: "Prospecting", Level: 1, MaxLevel: 3, XP: 30, XPNext: 60, Meets: true,
			Prereqs: []ProfPrereq{{Family: "mining", Level: 1}}},
		{Family: "refining", Name: "Refining", Level: 2, MaxLevel: 4, XP: 20, XPNext: 120, Meets: true,
			Prereqs: []ProfPrereq{{Family: "mining", Level: 2}}},
		{Family: "manufacturing", Name: "Manufacturing", Level: 1, MaxLevel: 4, XP: 0, XPNext: 100, Meets: false,
			Prereqs: []ProfPrereq{{Family: "refining", Level: 3}}},
		{Family: "haulage", Name: "Haulage", Level: 2, MaxLevel: 4, XP: 150, XPNext: 200, Meets: true},
		{Family: "rigging", Name: "Rigging", Level: 0, MaxLevel: 3, XP: 0, XPNext: 80, Meets: false,
			Prereqs: []ProfPrereq{{Family: "manufacturing", Level: 2}, {Family: "haulage", Level: 2}}},
	}
}

// TestProficiencyCaptureFrame: the proficiency layer open over the world
// with one window above it. The pins, all inside the loop (pixel readback
// panics before the game starts):
//
//   - every sampled pixel of the finances window is IDENTICAL with and
//     without the layer mounted ("does not obscure windows"), while a
//     control point on bare layer space moves (the layer is opaque over
//     the world);
//   - two panels painted from the same model are byte-identical (the
//     deterministic layout, at the pixel level).
//
// Skipped unless ENTIEVE_CAPTURE=1 so the regular suite stays windowless.
// The PNG lands in $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
func TestProficiencyCaptureFrame(t *testing.T) {
	if testing.Short() {
		t.Skip("capture")
	}
	uitest.UseScale(t, 1)
	k, _ := newTestKit(t)
	fin := NewFinances(k.Env())
	k.AddWindow(fin.Win())
	fin.Win().SetLocation(image.Rect(900, 380, 1280, 720))
	// Hide the kit's two always-on windows: the evidence is the graph, and
	// one floating window above it (Finances) is the stacking proof.
	k.SetCoreVisible(false)
	p := NewProficiency(k.Env(), proficiencyCaptureFixture, nil)
	// The second paint of the same model, never mounted: its canvas is the
	// determinism evidence.
	twin := NewProficiency(k.Env(), proficiencyCaptureFixture, nil)
	pumpKit(t, k, 1280, 720)
	pumpKit(t, k, 1280, 720)

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
			k.Sync()
			p.Sync()
			twin.Sync()
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
				// layer, not a window) and paints from the next frame.
				k.AddBackdropWindow(p.Win())
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

// readPixels copies an image's pixels for the byte comparison.
func readPixels(t *testing.T, img *ebiten.Image) []byte {
	t.Helper()
	b := img.Bounds()
	buf := make([]byte, 4*b.Dx()*b.Dy())
	img.ReadPixels(buf)
	return buf
}
