package screens

import (
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/hajimehoshi/ebiten/v2"
)

// logBody is a container that records its own Render — the content half of
// the header band's paint-order pin.
type logBody struct {
	*widget.Container
	log *[]string
}

func (b *logBody) Render(screen *ebiten.Image) {
	*b.log = append(*b.log, "body")
	b.Container.Render(screen)
}

// writeHeaderJPEG drops a w×h JPEG at dir/artwork/<name>.jpg.
func writeHeaderJPEG(t *testing.T, dir, name string, w, h int) {
	t.Helper()
	path := filepath.Join(dir, "artwork", name+".jpg")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, image.NewRGBA(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatalf("encode %s: %v", path, err)
	}
}

// TestHeaderBandGeometry pins the band's place: the body's full width, the
// top headerBandPx DESIGN px (so 2× is 240), directly under the title bar —
// never a layout row of its own.
func TestHeaderBandGeometry(t *testing.T) {
	body := image.Rect(100, 60, 500, 460)
	if got, want := headerBandRect(body), image.Rect(100, 60, 500, 180); got != want {
		t.Errorf("band = %v, want %v", got, want)
	}
	short := image.Rect(10, 20, 110, 60) // a body shorter than the band
	if got, want := headerBandRect(short), image.Rect(10, 20, 110, 60); got != want {
		t.Errorf("short body band = %v, want %v (clamped to the body)", got, want)
	}
	if got := headerBandRect(image.Rectangle{}); !got.Empty() {
		t.Errorf("empty body band = %v, want empty", got)
	}

	uitest.UseScale(t, 2)
	if got, want := headerBandRect(body), image.Rect(100, 60, 500, 300); got != want {
		t.Errorf("2× band = %v, want %v (theme.Px scaled)", got, want)
	}
}

// TestHeaderAlphaFadesDownBand pins the gradient: headerTopAlpha on the title
// edge, falling monotonically to nothing at the band's bottom edge, scaled by
// the open fade and held at full after it.
func TestHeaderAlphaFadesDownBand(t *testing.T) {
	const h = headerBandPx
	if got := headerAlpha(0, h, openFrames); got != headerTopAlpha {
		t.Errorf("top alpha = %d, want %d at the settled fade", got, headerTopAlpha)
	}
	prev := uint8(headerTopAlpha)
	for y := 0; y < h; y++ {
		if a := headerAlpha(y, h, openFrames); a > prev {
			t.Fatalf("alpha rises at row %d: %d > %d", y, a, prev)
		} else {
			prev = a
		}
	}
	if got := headerAlpha(h-1, h, openFrames); got > 2 {
		t.Errorf("bottom row alpha = %d, want ≈0 (fully transparent)", got)
	}
	if got := headerAlpha(h, h, openFrames); got != 0 {
		t.Errorf("past the bottom alpha = %d, want 0", got)
	}
	if got := headerAlpha(0, h, 0); got != 0 {
		t.Errorf("fade 0 alpha = %d, want 0 (nothing before the first frame)", got)
	}
	if got := headerAlpha(0, h, openFrames/2); got != headerTopAlpha/2 {
		t.Errorf("half-way fade alpha = %d, want %d", got, headerTopAlpha/2)
	}
	if got := headerAlpha(0, h, openFrames*3); got != headerTopAlpha {
		t.Errorf("settled fade alpha = %d, want it held at %d", got, headerTopAlpha)
	}
}

// TestHeaderBandPaintsPlateThenContent pins the z-order the header needs: the
// band resolves and paints the plate BEFORE the body (content over the band,
// never under it). A nil plate (no asset root) draws nothing and still paints
// the body — the plain-window fallback, no panic.
func TestHeaderBandPaintsPlateThenContent(t *testing.T) {
	for _, c := range []struct {
		name  string
		plate *ebiten.Image
		want  []string
	}{
		{"with plate", ebiten.NewImage(1280, 720), []string{"plate", "body"}},
		// Resolve-then-draw: the resolver runs either way, the nil draw is
		// skipped, and the body still paints over the band.
		{"no plate", nil, []string{"plate", "body"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var log []string
			body := &logBody{Container: widget.NewContainer(), log: &log}
			h := newHeaderBand(body, nil, func() *ebiten.Image {
				log = append(log, "plate")
				return c.plate
			})
			h.SetLocation(image.Rect(0, 0, 400, 300))
			h.Render(ebiten.NewImage(400, 300))
			if len(log) != len(c.want) {
				t.Fatalf("paint order = %v, want %v", log, c.want)
			}
			for i := range c.want {
				if log[i] != c.want[i] {
					t.Fatalf("paint order = %v, want %v", log, c.want)
				}
			}
		})
	}
}

// TestHeaderBandFadeResetsOnOpen: the band fades in over the house 12 frames,
// holds, and restarts when the window opens again (Kit.AddWindow is the open).
func TestHeaderBandFadeResetsOnOpen(t *testing.T) {
	k, _ := newTestKit(t)
	ref := NewRefinery(k.Env())
	k.AddWindow(ref.Win())
	band := ref.Win().Contents.(*headerBand)
	if band.fade != 0 {
		t.Fatalf("fade %d at mount, want 0", band.fade)
	}
	for i := 0; i < openFrames; i++ {
		pumpKit(t, k, 1280, 720)
	}
	if band.fade != openFrames {
		t.Fatalf("fade %d after %d frames, want %d", band.fade, openFrames, openFrames)
	}
	pumpKit(t, k, 1280, 720)
	if band.fade != openFrames {
		t.Fatalf("fade %d after settling, want it held at %d", band.fade, openFrames)
	}

	// The band is the body: full width, its top edge on the title bar's
	// bottom edge (the plate never covers the title).
	bandRect := band.GetWidget().Rect
	titleRect := ref.Win().TitleBar.GetWidget().Rect
	if bandRect.Min.Y != titleRect.Max.Y {
		t.Errorf("band top %d, want it on the title bar's bottom %d", bandRect.Min.Y, titleRect.Max.Y)
	}
	if bandRect.Dx() != ref.Win().GetContainer().GetWidget().Rect.Dx() {
		t.Errorf("band width %d, want the window width %d", bandRect.Dx(),
			ref.Win().GetContainer().GetWidget().Rect.Dx())
	}

	k.AddWindow(ref.Win()) // reopen while still mounted
	if band.fade != 0 {
		t.Fatalf("fade %d on reopen, want 0", band.fade)
	}
}

// TestHeaderPlatesPerWindow pins the wiring: each window resolves its OWN
// plate (the refinery never shows the manufacturing art), and no plates at all
// leaves both bands on the plain panel.
func TestHeaderPlatesPerWindow(t *testing.T) {
	dir := t.TempDir()
	writeHeaderJPEG(t, dir, "refining_header", 32, 18)
	writeHeaderJPEG(t, dir, "manufacturing_header", 64, 36)
	render.LoadAssets(dir)
	t.Cleanup(func() { render.LoadAssets("") })

	env, _ := newChromeEnv(t)
	ref, manu := NewRefinery(env), NewManufacturing(env)
	plate := func(w *widget.Window) *ebiten.Image {
		band, ok := w.Contents.(*headerBand)
		if !ok {
			t.Fatalf("window body is %T, want *headerBand", w.Contents)
		}
		return band.plate()
	}
	if b := plate(ref.Win()); b == nil || b.Bounds().Dx() != 32 {
		t.Errorf("refinery plate = %v, want the 32×18 refined header", b)
	}
	if b := plate(manu.Win()); b == nil || b.Bounds().Dx() != 64 {
		t.Errorf("manufacturing plate = %v, want the 64×36 header", b)
	}

	render.LoadAssets("") // the release build: no assets/ at all
	for _, w := range []*widget.Window{ref.Win(), manu.Win()} {
		if b := plate(w); b != nil {
			t.Errorf("band resolves %v with an empty registry, want nil (plain panel)", b)
		}
	}
}

// TestHeaderBandDrawSmoke drives the draw path headless both ways (plate and
// none) through a real window frame: same call path, no panic. Pixel readback
// needs a running ebiten loop, so the capture pins the pixels.
func TestHeaderBandDrawSmoke(t *testing.T) {
	dir := t.TempDir()
	writeHeaderJPEG(t, dir, "refining_header", 1280, 720)
	env, _ := newChromeEnv(t)
	ref := NewRefinery(env)
	ui := newWindowUI(t, env, ref.Win())

	render.LoadAssets(dir)
	t.Cleanup(func() { render.LoadAssets("") })
	uitest.PumpWindow(t, ui, ref.Sync, 1024, 700)
	uitest.PumpWindow(t, ui, ref.Sync, 1024, 700)

	render.LoadAssets("")
	uitest.PumpWindow(t, ui, ref.Sync, 1024, 700)
}
