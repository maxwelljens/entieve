package game

import (
	"image"
	"testing"

	uikit "github.com/entieve/entieve/uikit/screens"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestStarmapRowIsTheFullViewportLayer: the starmap row is the house's
// second full-viewport layer. N opens it (the Screen spec's own toggle),
// it mounts as the viewport-sized layer below every window, it has no
// title bar, and Esc closes it before the Esc menu. The hit test is the
// observable contract: a window opened over it keeps its own presses,
// bare layer space belongs to the layer (never the world), and the rail
// keeps its top-chrome precedence.
func TestStarmapRowIsTheFullViewportLayer(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	// A window over the layer, so the layer order has something to sit
	// under.
	g.host.SetOpen(idFinances, true)
	pump(t, g, 1280, 720)
	if g.host.IsOpen(idMap) {
		t.Fatal("fixture: the layer starts open")
	}

	if err := g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyN}}); err != nil {
		t.Fatalf("handleKeys(N): %v", err)
	}
	if !g.host.IsOpen(idMap) {
		t.Fatal("N did not open the starmap row")
	}
	pump(t, g, 1280, 720)
	if !g.host.Mounted(idMap) {
		t.Fatal("the open row did not mount")
	}
	layer := g.host.Window(idMap)
	if got, want := layer.GetContainer().GetWidget().Rect, image.Rect(0, 0, g.w, g.h); got != want {
		t.Fatalf("layer rect = %v, want the full viewport %v", got, want)
	}
	if layer.TitleBar != nil {
		t.Error("the layer grew a title bar")
	}

	// The window above owns its own presses, not the layer.
	fin := g.host.Window(idFinances)
	fr := fin.GetContainer().GetWidget().Rect
	pt := image.Pt(fr.Min.X+fr.Dx()/2, fr.Min.Y+fr.Dy()/2)
	if o := g.kit.Own(pt); o.Layer != uikit.LayerWindow || o.Window != fin {
		t.Errorf("press over the finances window = {layer %d, window %v}, want that window", o.Layer, o.Window)
	}
	// Bare layer space: inert (no control, no window) but never the world.
	bare := image.Pt(g.w-80, g.h-120)
	if o := g.kit.Own(bare); o.Layer == uikit.LayerWorld || o.Control || o.Window != nil {
		t.Errorf("bare layer space = %+v, want an inert non-world owner", o)
	}
	// The rail is top chrome: above the layer, as it paints.
	if o := g.kit.Own(image.Pt(10, 300)); o.Layer != uikit.LayerTopChrome {
		t.Errorf("rail point = layer %d, want the top chrome", o.Layer)
	}

	// Esc closes the layer (the modal idiom), and the key reopens it.
	if err := g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyEscape}}); err != nil {
		t.Fatalf("handleKeys(Esc): %v", err)
	}
	pump(t, g, 1280, 720)
	if g.host.IsOpen(idMap) || g.host.Mounted(idMap) {
		t.Fatal("Esc left the starmap layer open")
	}
	if g.host.IsOpen(idMenu) {
		t.Fatal("Esc opened the menu behind the layer instead of closing the layer")
	}
	if o := g.kit.Own(bare); o.Layer != uikit.LayerWorld {
		t.Errorf("after the close the world point is layer %d, want world", o.Layer)
	}
}
