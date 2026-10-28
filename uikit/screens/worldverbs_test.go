package screens

import (
	"strconv"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/widgets"
)

// verbView is the fixture ContentView the resolver runs against: one
// entity per target kind plus a full high rack (slot 2 is the mining
// laser). No sim is involved — the resolver reads only the view.
type verbView struct {
	widgets.ContentView
	player ecs.Entity
	grid   []sim.Info
	fit    sim.Fitting
	defs   *content.Content
}

func (v *verbView) Player() ecs.Entity                     { return v.player }
func (v *verbView) Grid() []sim.Info                       { return v.grid }
func (v *verbView) Defs() *content.Content                 { return v.defs }
func (v *verbView) Fitting(ecs.Entity) (sim.Fitting, bool) { return v.fit, true }

func (v *verbView) Info(e ecs.Entity) (sim.Info, bool) {
	for _, info := range v.grid {
		if info.Entity == e {
			return info, true
		}
	}
	return sim.Info{}, false
}

// verbFixture builds the view: the flag each verb reads is set on exactly
// the entity that offers it, and the distances make the nearest picks
// unambiguous (wreckNear 2 km, rock 3 km, station 4 km, gate 6 km,
// wreckFar 9 km).
type verbFixture struct {
	*verbView
	station, gate, wreckNear, wreckFar, rock, planetFar, planetNear ecs.Entity
}

func newVerbFixture(t *testing.T) *verbFixture {
	t.Helper()
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	f := &verbFixture{
		station: 2, gate: 3, wreckNear: 4, wreckFar: 5, rock: 6,
		planetFar: 7, planetNear: 8,
	}
	v := &verbView{player: 1, defs: c}
	// A full high rack: every F-key row must resolve, and slot 2 is the
	// mining laser firstMiningSlot finds.
	high := []content.ModuleID{"mod-autocannon-s", "mod-mining-laser"}
	for len(high) < 8 {
		high = append(high, "mod-autocannon-s")
	}
	rt := make([]sim.ModuleRuntime, len(high))
	for i, id := range high {
		rt[i] = sim.ModuleRuntime{Def: id}
	}
	v.fit = sim.Fitting{High: high, HighRT: rt}
	v.grid = []sim.Info{
		{Entity: f.station, Name: "Station", Kind: sim.KindStation, Distance: 4000, Visible: true, CanDock: true},
		{Entity: f.gate, Name: "Gate", Kind: sim.KindGate, Distance: 6000, Visible: true, CanJump: true},
		{Entity: f.wreckNear, Name: "Wreck A", Kind: sim.KindWreck, Distance: 2000, Visible: true, CanLoot: true},
		{Entity: f.wreckFar, Name: "Wreck B", Kind: sim.KindWreck, Distance: 9000, Visible: true, CanLoot: true},
		{Entity: f.rock, Name: "Rock", Kind: sim.KindAsteroid, Distance: 3000, Visible: true, CanMine: true},
		{Entity: f.planetFar, Name: "Planet", Kind: sim.KindPlanet, Distance: 1e6, Visible: true, CanWarp: true},
		{Entity: f.planetNear, Name: "Moon", Kind: sim.KindMoon, Distance: 10000, Visible: true, CanWarp: true},
		{Entity: v.player, Name: "Player", Kind: sim.KindPlayer, Visible: true},
	}
	f.verbView = v
	return f
}

// TestWorldVerbTableResolvesAndEmits pins every row: what it targets, the
// intent it builds and the entity that intent carries. The fallbacks are
// the shared surface's (dock on a gate → the nearest station, loot ignores
// the selected wreck, cycle starts at the nearest).
func TestWorldVerbTableResolvesAndEmits(t *testing.T) {
	f := newVerbFixture(t)
	highSlot := func(i int) sim.SlotRef { return sim.SlotRef{Class: content.SlotHigh, Index: i} }

	type verbCase struct {
		name, action string
		sel          ecs.Entity
		want         widgets.Intent
	}
	cases := []verbCase{
		{"lock-nearest from nothing starts at the nearest", "lock-nearest", 0,
			widgets.Lock{Entity: f.wreckNear}},
		{"lock-nearest advances past the selection", "lock-nearest", f.wreckNear,
			widgets.Lock{Entity: f.rock}},
		{"lock-selected locks the selection", "lock-selected", f.rock,
			widgets.Lock{Entity: f.rock}},
		{"high-1 activates slot 1 at the selection", "high-1", f.wreckFar,
			widgets.Activate{Slot: highSlot(0), Target: f.wreckFar}},
		{"high-8 deactivates an active slot", "high-8", f.wreckFar,
			widgets.Deactivate{Slot: highSlot(7)}},
		{"approach", "approach", f.rock, widgets.Approach{Entity: f.rock}},
		{"orbit", "orbit", f.rock, widgets.Orbit{Entity: f.rock}},
		{"stop", "stop", f.rock, widgets.Stop{}},
		{"align", "align", f.rock, widgets.Align{Entity: f.rock}},
		{"warp", "warp", f.planetFar, widgets.WarpTo{Entity: f.planetFar}},
		{"dock at the selected station", "dock", f.station,
			widgets.Dock{Entity: f.station}},
		{"dock on a non-station falls back to the nearest station", "dock", f.gate,
			widgets.Dock{Entity: f.station}},
		{"jump at the selected gate", "jump", f.gate,
			widgets.Jump{Entity: f.gate}},
		{"jump on a non-gate falls back to the nearest gate", "jump", f.station,
			widgets.Jump{Entity: f.gate}},
		{"loot ignores the selected wreck", "loot", f.wreckFar,
			widgets.Loot{Entity: f.wreckNear}},
		{"mine fires the mining laser at the selection", "mine", f.rock,
			widgets.Mine{Entity: f.rock, Slot: highSlot(1)}},
	}
	// High slots 2..8 map onto their own index; slot 8 is the active one.
	f.fit.HighRT[7].Active = true
	for i := 1; i < 8; i++ {
		if i == 7 {
			continue
		}
		action := "high-" + strconv.Itoa(i+1)
		cases = append(cases, verbCase{action + " toggles its own slot",
			action, f.rock, widgets.Activate{Slot: highSlot(i), Target: f.rock}})
	}

	for _, c := range cases {
		v, ok := (&Kit{}).Verb(c.action)
		if !ok {
			t.Errorf("%s: no verb row for %q", c.name, c.action)
			continue
		}
		res, ok := resolveVerb(v, f.verbView, c.sel, nil)
		if !ok {
			t.Errorf("%s: resolveVerb(%q, sel=%v) failed", c.name, c.action, c.sel)
			continue
		}
		if got := v.Emit(res); got != c.want {
			t.Errorf("%s: %q emitted %#v, want %#v", c.name, c.action, got, c.want)
		}
	}
	// Every row has a case: a new verb without one fails here.
	covered := map[string]bool{}
	for _, c := range cases {
		covered[c.action] = true
	}
	for _, v := range WorldVerbs {
		if !covered[v.Action] {
			t.Errorf("verb %q has no resolution case", v.Action)
		}
	}
}

// TestWorldVerbReadyIsThePanelGate: Eligible says "may act" (the key
// path); Ready is the panel's disabled state and why text. Warp inside
// sim.WarpMinRange is the one verb with a reason.
func TestWorldVerbReadyIsThePanelGate(t *testing.T) {
	f := newVerbFixture(t)
	if _, ok := resolveVerb(mustVerb(t, "warp"), f.verbView, f.planetNear, nil); !ok {
		t.Fatal("warp inside WarpMinRange is not eligible (the key must still ask the sim)")
	}
	k := &Kit{cv: f.verbView, selected: f.planetNear}
	if ok, _ := k.VerbReady("warp"); ok {
		t.Error("warp PanelReady inside WarpMinRange = true, want false")
	}
	if _, why := k.VerbReady("warp"); why != "Target within warp minimum range ("+sim.KmLabel(sim.WarpMinRange)+")" {
		t.Errorf("warp why = %q, want the quoted minimum range", why)
	}

	// Stop needs no target to act, but the panel's button greys out with
	// nothing selected — the key path still stops the ship.
	k.selected = 0
	if ok, _ := k.VerbReady("stop"); ok {
		t.Error("stop PanelReady without a selection = true, want false")
	}
	res, ok := resolveVerb(mustVerb(t, "stop"), f.verbView, 0, nil)
	if !ok || res.Entity != f.verbView.player {
		t.Errorf("stop without a selection did not resolve to the player: %+v (ok=%v)", res, ok)
	}

	// A docked player owns no dockable neighbour: the fallback must stay
	// empty, never fall back past eligibility.
	f.grid[0].CanDock = false
	k.selected = 0
	if ok, _ := k.VerbReady("dock"); ok {
		t.Error("dock PanelReady with no dockable entity = true, want false")
	}
	if _, ok := resolveVerb(mustVerb(t, "dock"), f.verbView, 0, nil); ok {
		t.Error("dock resolved with no dockable entity")
	}
}

// TestLockSelectedRefusesStation pins the disclosed behavior change: the
// panel already greyed Lock on a station, and the unified verb now makes
// the Shift+Tab key say the same (the old key stood a CmdLock at any
// selection).
func TestLockSelectedRefusesStation(t *testing.T) {
	f := newVerbFixture(t)
	if _, ok := resolveVerb(mustVerb(t, "lock-selected"), f.verbView, f.station, nil); ok {
		t.Error("lock-selected resolved a station, want ineligible")
	}
	if ok, _ := (&Kit{cv: f.verbView, selected: f.station}).VerbReady("lock-selected"); ok {
		t.Error("lock-selected PanelReady on a station = true, want false")
	}
}

// mustVerb resolves an action id or fails.
func mustVerb(t *testing.T, action string) WorldVerb {
	t.Helper()
	v, ok := (&Kit{}).Verb(action)
	if !ok {
		t.Fatalf("no verb row for %q", action)
	}
	return v
}

// TestNextLockIdx pins the Tab cycle's anchoring semantics: the step is
// the position after the SELECTED candidate in the freshly sorted list
// (wrapping), and a selection outside the list restarts at the nearest —
// a remembered index would skip or repeat entries once the distances
// re-sort. (Moved here with the cycle itself.)
func TestNextLockIdx(t *testing.T) {
	cands := []sim.Info{{Entity: 10}, {Entity: 11}, {Entity: 12}}
	cases := []struct {
		name string
		sel  ecs.Entity
		want int
	}{
		{"no selection starts at the nearest", 0, 0},
		{"advances from the first candidate", 10, 1},
		{"advances from the middle", 11, 2},
		{"wraps after the farthest", 12, 0},
		{"non-candidate selection restarts", 99, 0},
	}
	for _, c := range cases {
		if got := nextLockIdx(cands, c.sel); got != c.want {
			t.Errorf("%s: nextLockIdx(_, %v) = %d, want %d", c.name, c.sel, got, c.want)
		}
	}
	// A single candidate always wraps to itself.
	if got := nextLockIdx(cands[:1], 10); got != 0 {
		t.Errorf("single candidate: nextLockIdx = %d, want 0", got)
	}
}
