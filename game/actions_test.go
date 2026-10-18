package game

import (
	"math"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestWorldKeysActAtKitRanges pins the keybind half of the "one source
// of truth" contract: Q orbits and Z warps at the values the Selected
// Item selectors hold (Kit.OrbitRange / Kit.WarpRange), the same values
// the panel's Orbit/Warp action buttons enqueue (uikit pins that half).
// The selectors are driven through the real widget tree first, so a
// keybind that keeps its own range (the old Q cycle, the old fixed
// 10 km warp) fails here.
func TestWorldKeysActAtKitRanges(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	for i := 0; i < 3; i++ {
		if err := g.Update(); err != nil {
			t.Fatalf("Update frame %d: %v", i, err)
		}
	}

	// A warpable target beyond sim.WarpMinRange (the drive refuses
	// anything closer), selected for both keys.
	var target sim.Info
	for _, info := range g.s.Grid() {
		if info.CanWarp && info.Distance >= sim.WarpMinRange {
			target = info
			break
		}
	}
	if target.Entity.IsZero() {
		t.Fatal("fixture: no warpable target at least WarpMinRange away")
	}
	g.setSelected(target.Entity)

	// Drive both selectors off their defaults through their buttons:
	// orbit one click (2,500 -> 5,000), warp two clicks (0 -> 10,000 ->
	// 50,000). Nothing may enqueue while cycling.
	fireSelector(t, g, "Orbit range:", 1)
	fireSelector(t, g, "Warp to within:", 2)
	if got, want := g.kit.OrbitRange(), 5000.0; got != want {
		t.Fatalf("orbit selector = %v, want %v", got, want)
	}
	if got, want := g.kit.WarpRange(), 50000.0; got != want {
		t.Fatalf("warp selector = %v, want %v", got, want)
	}

	// Q: the orbit intent lands on the target at the selector's range.
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyQ}})
	g.s.Step()
	intent, ok := g.s.Intents.Get(g.s.Player())
	if !ok || intent.Kind != sim.IntentOrbit {
		t.Fatalf("Q intent = %+v (ok=%v), want IntentOrbit", intent, ok)
	}
	if intent.Target != target.Entity || intent.Range != g.kit.OrbitRange() {
		t.Errorf("Q orbit = {Target:%v Range:%v}, want {Target:%v Range:%v}",
			intent.Target, intent.Range, target.Entity, g.kit.OrbitRange())
	}

	// Z: the warp drive engages, landing WarpRange() short of the target.
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyZ}})
	g.s.Step()
	wd, ok := g.s.Warps.Get(g.s.Player())
	if !ok {
		t.Fatal("Z did not start a warp")
	}
	pp, _ := g.s.Transforms.Get(g.s.Player())
	tp, _ := g.s.Transforms.Get(target.Entity)
	want := tp.Pos
	if rng := g.kit.WarpRange(); rng > 0 {
		if dir := tp.Pos.Sub(pp.Pos); dir.Len() > rng && !dir.IsZero() {
			want = tp.Pos.Sub(dir.Norm().Scale(rng))
		}
	}
	if math.Abs(wd.To.X-want.X) > 1e-6 || math.Abs(wd.To.Y-want.Y) > 1e-6 {
		t.Errorf("warp landing = %v, want %v (Kit.WarpRange %v)",
			wd.To, want, g.kit.WarpRange())
	}
}

// fireSelector clicks the Selected Item window's range selector whose
// label starts with prefix, n times, through the real widget tree (the
// panel exposes no selector handle: the widget IS the interface).
func fireSelector(t *testing.T, g *Game, prefix string, n int) {
	t.Helper()
	var btn *widget.Button
	// Recurse through Containerer: the window body wears the kit's
	// frame-clip wrapper (uikit/screens/winclip.go).
	var walk func(c widget.Containerer)
	walk = func(c widget.Containerer) {
		for _, ch := range c.Children() {
			switch v := ch.(type) {
			case *widget.Button:
				if v.Text() != nil && strings.HasPrefix(v.Text().Label, prefix) {
					btn = v
				}
			case widget.Containerer:
				walk(v)
			}
		}
	}
	ctr, ok := g.kit.SelectedWindow().GetContainer().(widget.Containerer)
	if !ok {
		t.Fatalf("Selected Item window container is %T", g.kit.SelectedWindow().GetContainer())
	}
	walk(ctr)
	if btn == nil {
		t.Fatalf("no selector labelled %q in the Selected Item window", prefix)
	}
	for i := 0; i < n; i++ {
		btn.ClickedEvent.Fire(&widget.ButtonClickedEventArgs{Button: btn})
		if err := g.kit.Update(); err != nil {
			t.Fatalf("kit.Update: %v", err)
		}
	}
}
