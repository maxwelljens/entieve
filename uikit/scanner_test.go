package uikit

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/sim"
)

// TestScanCountsRule pins the aggregation: visible contacts, the player
// and anything past 3× sensor range never count; known kinds keep their
// display order and the rest sort alphabetically.
func TestScanCountsRule(t *testing.T) {
	const sensor = 1000.0
	infos := []sim.Info{
		{Kind: sim.KindShip, Distance: 100},              // off-grid, in range → counted
		{Kind: sim.KindShip, Distance: 4 * sensor},       // past 3× → out
		{Kind: sim.KindShip, Distance: 0, Visible: true}, // visible → out
		{Kind: sim.KindPlayer, Distance: 0},              // the player → out
		{Kind: sim.KindWreck, Distance: 3 * sensor},      // boundary (≤) → counted
		{Kind: sim.KindAsteroid, Distance: sensor},       // counted
		{Kind: "moon", Distance: sensor},                 // unknown order: alphabetical tail
		{Kind: "belt", Distance: sensor},                 // …before "moon"
	}
	got := scanCounts(infos, sensor)
	want := []scanCount{
		{Kind: sim.KindShip, N: 1},
		{Kind: sim.KindAsteroid, N: 1},
		{Kind: sim.KindWreck, N: 1},
		{Kind: "belt", N: 1},
		{Kind: "moon", N: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("counts = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("counts[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	// Nothing off-grid in range → no groups at all.
	if got := scanCounts([]sim.Info{{Kind: sim.KindPlayer}, {Kind: sim.KindShip, Visible: true}}, sensor); len(got) != 0 {
		t.Errorf("empty grid produced %+v", got)
	}
}

// TestScannerSweepCounts drives the widget: the showcase grid starts with
// every contact beyond the sweep, then the player is moved next to the belt
// so an asteroid sits off-grid but inside 3× range — the next completed
// sweep must report it anonymously.
func TestScannerSweepCounts(t *testing.T) {
	env, _ := newChromeEnv(t)
	s := env.Sim
	sc := NewScanner(env)
	c := newChromeUI(env)
	c.addWindow(sc)
	c.frame(t, 1024, 700)

	// Showcase start: off-grid contacts are all past 750 km.
	if len(sc.counts) != 0 {
		t.Fatalf("start counts = %+v, want none", sc.counts)
	}
	if vis := sc.empty.GetWidget().GetVisibility(); vis != widget.Visibility_Show {
		t.Errorf("empty hint hidden with no contacts (visibility %d)", vis)
	}
	for i, lbl := range sc.labels {
		if vis := lbl.GetWidget().GetVisibility(); vis != widget.Visibility_Hide {
			t.Errorf("label %d shown with no contacts (visibility %d)", i, vis)
		}
	}
	if !strings.HasPrefix(sc.status.Label, "Sweep ") {
		t.Errorf("status = %q", sc.status.Label)
	}

	// Teleport the player 400 km off an asteroid: off-grid (> 250 km) but
	// inside the 750 km sweep.
	var rock sim.Info
	found := false
	for _, inf := range s.Grid() {
		if inf.Kind == sim.KindAsteroid {
			rock, found = inf, true
			break
		}
	}
	if !found {
		t.Fatal("showcase grid has no asteroid")
	}
	tr, ok := s.Transforms.Get(s.Player())
	if !ok {
		t.Fatal("player has no transform")
	}
	tr.Pos = core.Vec2{X: rock.Pos.X + 400_000, Y: rock.Pos.Y}

	// Force a sweep completion: phase 0.95 → 0.05 across the wrap.
	sc.base = time.Now().Add(-1900 * time.Millisecond)
	sc.Sync()
	sc.base = time.Now().Add(-2100 * time.Millisecond)
	sc.Sync()

	// The label pool mirrors the counts (index-for-index).
	idx := -1
	for i := range sc.counts {
		if sc.counts[i].Kind == sim.KindAsteroid {
			idx = i
		}
	}
	if idx < 0 || sc.counts[idx].N < 1 {
		t.Fatalf("moved player sees %+v, want at least one asteroid", sc.counts)
	}
	if lbl, want := sc.labels[idx].Label, scanKindLabel(sc.counts[idx].Kind, sc.counts[idx].N); lbl != want {
		t.Errorf("label %d = %q, want %q", idx, lbl, want)
	}
	// Everything reported is an anonymous off-grid kind: the player never
	// appears (the range/visibility rules are pinned by TestScanCountsRule).
	for _, cnt := range sc.counts {
		if cnt.Kind == sim.KindPlayer {
			t.Errorf("player leaked into counts %+v", sc.counts)
		}
	}
	if sc.bar.GetCurrent() < 0 || sc.bar.GetCurrent() > scanMax {
		t.Errorf("sweep bar = %d, want 0..%d", sc.bar.GetCurrent(), scanMax)
	}
}

// TestScanPhase checks the sweep phase math (including the wrap).
func TestScanPhase(t *testing.T) {
	for _, c := range []struct{ t, want float64 }{
		{0, 0}, {0.5, 0.25}, {1.9, 0.95}, {2, 0}, {2.1, 0.05}, {5.5, 0.75},
	} {
		got := scanPhase(c.t, scanCycle)
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("scanPhase(%v) = %v, want %v", c.t, got, c.want)
		}
	}
	if got := scanPhase(1, 0); got != 0 {
		t.Errorf("scanPhase with zero cycle = %v, want 0", got)
	}
}

// TestScannerDrawSmoke draws the scanner at both design sizes.
func TestScannerDrawSmoke(t *testing.T) {
	env, _ := newChromeEnv(t)
	sc := NewScanner(env)
	c := newChromeUI(env)
	c.addWindow(sc)
	c.smoke(t)
}
