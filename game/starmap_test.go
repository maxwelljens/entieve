package game

import (
	"image"
	"testing"

	uikit "github.com/entieve/entieve/uikit/screens"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestStarmapRowIsTheFullViewportLayer: the starmap is the house's
// over-chrome layer (the Screen row's OverChrome facet). N opens it (the
// Screen spec's own toggle), it mounts as a viewport-sized layer with no
// title bar, and it owns the whole screen through the hit test: the window
// under it, the rail column and bare canvas all answer the front layer, so
// no press leaks through to the chrome.
//
// The contrast is the proficiency tree, the backdrop layer: it declares no
// OverChrome, so it keeps its presses under every window and the rail
// (TestProficiencyRowIsTheFullViewportLayer). The two layers differ by
// data, not by panel.
func TestStarmapRowIsTheFullViewportLayer(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	if !screenByID(idMap).OverChrome {
		t.Fatal("the starmap row does not declare OverChrome")
	}
	if screenByID(idProficiency).OverChrome {
		t.Fatal("the proficiency tree declares OverChrome: its layer must stay under the rail")
	}

	// A window under the layer, so the order has something to cover.
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

	// Paint order, read back as press order: the layer covers the window
	// below it and the rail column alike.
	fin := g.host.Window(idFinances)
	fr := fin.GetContainer().GetWidget().Rect
	overWindow := image.Pt(fr.Min.X+fr.Dx()/2, fr.Min.Y+fr.Dy()/2)
	if o := g.kit.Own(overWindow); o.Layer != uikit.LayerFront || o.Window != nil || o.Control {
		t.Errorf("press over the finances window = {layer %d, window %v, control %v}, want the front layer, inert",
			o.Layer, o.Window, o.Control)
	}
	// The rail is chrome, but the layer paints over it, so the rail entry
	// under it must not take the press.
	if o := g.kit.Own(image.Pt(10, 300)); o.Layer != uikit.LayerFront {
		t.Errorf("rail point = layer %d, want the front layer (no click-through to the rail)", o.Layer)
	}
	// Bare canvas: inert (no control, no window) but never the world.
	bare := image.Pt(g.w-80, g.h-120)
	if o := g.kit.Own(bare); o.Layer != uikit.LayerFront || o.Control || o.Window != nil {
		t.Errorf("bare layer space = %+v, want an inert front-layer owner", o)
	}

	// Esc closes the layer (the ladder's step), and the key reopens it.
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
	// With the layer gone the rail is the top chrome again: the door it
	// covers while the map is up still toggles the row.
	if o := g.kit.Own(image.Pt(10, 300)); o.Layer != uikit.LayerTopChrome {
		t.Errorf("rail point after the close = layer %d, want the top chrome", o.Layer)
	}
	if err := g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyN}}); err != nil {
		t.Fatalf("handleKeys(N) again: %v", err)
	}
	pump(t, g, 1280, 720)
	if !g.host.IsOpen(idMap) {
		t.Error("N did not reopen the layer")
	}
}

// TestStarmapCanvasCrossClosesTheLayer: the layer has no title bar, so its
// × is drawn in the canvas's top-right corner. A press on that rect runs
// the row's one toggle, exactly like Esc and N: the front layer being
// above the chrome must not cost it a mouse door.
func TestStarmapCanvasCrossClosesTheLayer(t *testing.T) {
	mm := useLoopMouse(t)
	g := newShowcaseGame(t, mm.pointer())
	g.applySize(1280, 720)
	g.host.SetOpen(idMap, true)
	pump(t, g, 1280, 720)

	m := bodyOf[*uikit.MapView](g.host, idMap)
	if m == nil {
		t.Fatal("fixture: the starmap panel was not built")
	}
	r := m.CloseRect()
	if r.Empty() || !r.In(image.Rect(0, 0, g.w, g.h)) {
		t.Fatalf("× rect = %v, want a box inside the layer", r)
	}
	at := image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)

	mm.moveTo(at.X, at.Y)
	if err := g.Update(); err != nil { // move: hover only
		t.Fatalf("Update: %v", err)
	}
	if !g.host.IsOpen(idMap) {
		t.Fatal("a hover closed the layer")
	}
	mm.left = true
	if err := g.Update(); err != nil { // press
		t.Fatalf("Update: %v", err)
	}
	mm.left = false
	if err := g.Update(); err != nil { // release
		t.Fatalf("Update: %v", err)
	}
	pump(t, g, 1280, 720)
	if g.host.IsOpen(idMap) || g.host.Mounted(idMap) {
		t.Fatal("the canvas × left the starmap layer open")
	}
}
