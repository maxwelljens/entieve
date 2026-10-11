package uikit

import (
	"image"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/sim"
)

// btnLabel reads a button's live label (empty until the widget tree is
// first validated).
func btnLabel(b *widget.Button) string {
	if t := b.Text(); t != nil {
		return t.Label
	}
	return ""
}

// selectedRects returns the body's child rects in child order: the name
// line, the distance line, then every button cell band by band.
func selectedRects(sp *selectedPanel) []image.Rectangle {
	kids := sp.body.Children()
	rects := make([]image.Rectangle, len(kids))
	for i, k := range kids {
		rects[i] = k.GetWidget().Rect
	}
	return rects
}

// assertSelectedLayout checks the design contract of selectedLayout over
// one inner rect: every rect sits inside inner, no two rects overlap,
// each button band tiles its row edge to edge with equal gaps, and the
// stack is top-anchored. strict additionally requires every row and cell
// gap to equal contentsPad — it only holds while the content fits (the
// degenerate cases collapse rows instead of spilling).
func assertSelectedLayout(t *testing.T, tag string, inner image.Rectangle, rects []image.Rectangle, bands []int, strict bool) {
	t.Helper()
	want := 2
	for _, n := range bands {
		want += n
	}
	if len(rects) != want {
		t.Fatalf("%s: %d rects, want %d", tag, len(rects), want)
	}
	for i, r := range rects {
		if !r.In(inner) {
			t.Errorf("%s: rect %d %v escapes inner %v", tag, i, r, inner)
		}
		for j := i + 1; j < len(rects); j++ {
			if r.Overlaps(rects[j]) {
				t.Errorf("%s: rect %d %v overlaps %d %v", tag, i, r, j, rects[j])
			}
		}
	}
	if rects[0].Min.Y != inner.Min.Y {
		t.Errorf("%s: first row starts at %d, want top-anchored %d", tag, rects[0].Min.Y, inner.Min.Y)
	}
	// Bands tile their row with equal gaps; rows stack with the same gap.
	gridGaps := map[int]bool{}
	rowGaps := map[int]bool{}
	idx := 2
	for bi, n := range bands {
		cells := rects[idx : idx+n]
		idx += n
		if n == 0 {
			continue
		}
		if cells[0].Min.X != inner.Min.X {
			t.Errorf("%s: band %d starts at %d, want %d", tag, bi, cells[0].Min.X, inner.Min.X)
		}
		if cells[n-1].Max.X != inner.Max.X {
			t.Errorf("%s: band %d ends at %d, want %d (tiles the row)", tag, bi, cells[n-1].Max.X, inner.Max.X)
		}
		rowTop, rowBot := cells[0].Min.Y, cells[0].Max.Y
		for i, c := range cells {
			if c.Min.Y != rowTop || c.Max.Y != rowBot {
				t.Errorf("%s: band %d cell %d %v not aligned to row %d..%d", tag, bi, i, c, rowTop, rowBot)
			}
			if c.Dx() <= 0 {
				t.Errorf("%s: band %d cell %d has width %d", tag, bi, i, c.Dx())
			}
			if i > 0 {
				gridGaps[c.Min.X-cells[i-1].Max.X] = true
			}
		}
	}
	for i := 1; i < len(rects); i++ {
		if rects[i].Min.Y == rects[i-1].Min.Y {
			continue // same band row, handled above
		}
		rowGaps[rects[i].Min.Y-rects[i-1].Max.Y] = true
	}
	// The button grid's gaps are part of the contract at every size; the
	// row gaps only hold while the content fits (degenerate inner rects
	// collapse rows instead of spilling).
	assertGapSet(t, tag, "cell", gridGaps)
	if strict {
		assertGapSet(t, tag, "row", rowGaps)
	}
}

// assertGapSet requires every recorded gap to equal contentsPad.
func assertGapSet(t *testing.T, tag, kind string, gaps map[int]bool) {
	t.Helper()
	if len(gaps) != 1 {
		t.Errorf("%s: %s gaps %v are not all equal", tag, kind, gaps)
		return
	}
	for g := range gaps {
		if g != contentsPad {
			t.Errorf("%s: %s gap = %d, want %d", tag, kind, g, contentsPad)
		}
	}
}

// pumpSelected builds a kit at the given screen size, optionally clamps
// the Selected Item window to its minimum, and selects a live target so
// both text lines carry content. It returns the panel and the inner rect
// the layout works from.
func pumpSelected(t *testing.T, w, h int, minSize bool, selectTarget bool) (*Kit, *selectedPanel, image.Rectangle) {
	t.Helper()
	k, _ := newTestKit(t)
	pumpKit(t, k, w, h)
	if minSize {
		// SetLocation applies the window's MinSize clamp (450×240).
		k.sp.win.SetLocation(image.Rect(0, 0, 10, 10))
		pumpKit(t, k, w, h)
	}
	if selectTarget {
		for _, info := range k.Sim.Grid() {
			if info.CanWarp {
				k.Select(info.Entity)
				break
			}
		}
	}
	pumpKit(t, k, w, h)
	pumpKit(t, k, w, h)
	return k, k.sp, k.sp.body.GetWidget().Rect.Inset(contentsPad)
}

// TestSelectedLayoutMath pins the layout contract at both design sizes
// and at the window minimum: real widget rects must match the pure
// selectedLayout exactly, sit inside the inner rect, never overlap, and
// tile every button band with equal gaps.
func TestSelectedLayoutMath(t *testing.T) {
	cases := []struct {
		tag      string
		w, h     int
		minSize  bool
		selected bool
	}{
		{"1280x720", 1280, 720, false, true},
		{"1024x700", 1024, 700, false, true},
		{"min-450x240", 1280, 720, true, true},
		{"min-450x240-nosel", 1024, 700, true, false},
	}
	for _, tc := range cases {
		_, sp, inner := pumpSelected(t, tc.w, tc.h, tc.minSize, tc.selected)
		rects := selectedRects(sp)
		assertSelectedLayout(t, tc.tag, inner, rects, selectedBands[:], true)

		// The widget tree must be exactly what the pure function returns
		// for the measured inventory — no stale rects.
		l := &selectedLayouter{bands: selectedBands[:]}
		want := selectedLayout(inner, l.inventory(sp.body.Children()))
		if len(want) != len(rects) {
			t.Fatalf("%s: pure layout produced %d rects, widget tree has %d", tc.tag, len(want), len(rects))
		}
		for i := range want {
			if want[i] != rects[i] {
				t.Errorf("%s: rect %d = %v, pure selectedLayout says %v", tc.tag, i, rects[i], want[i])
			}
		}
	}
}

// TestSelectedLayoutDegenerate feeds the pure function inner rects far
// too small for the inventory: it must still return rects inside inner
// with no pairwise overlap (rows collapse instead of spilling).
func TestSelectedLayoutDegenerate(t *testing.T) {
	inv := selectedInventory{
		nameH: 32, distH: 17,
		bands: [][]float64{{120, 170, 70, 60}, {60, 60, 60, 60, 60}, {300}, {300}},
		bandH: 40,
		gap:   contentsPad,
	}
	for _, h := range []int{0, 1, 20, 60, 186, 400} {
		inner := image.Rect(10, 20, 310, 20+h)
		rects := selectedLayout(inner, inv)
		assertSelectedLayout(t, "degenerate", inner, rects, selectedBands[:], false)
	}
}

// TestSelectedLayoutPinnedRects pins the exact geometry at the 1024×700
// design size (default window rect) and at the 450×240 minimum, so any
// drift in the design math shows up as a tuple diff.
func TestSelectedLayoutPinnedRects(t *testing.T) {
	_, sp, _ := pumpSelected(t, 1024, 700, false, true)
	got := selectedRects(sp)
	want := []image.Rectangle{
		image.Rect(13, 485, 453, 515),  // name (icon + label row)
		image.Rect(13, 520, 453, 537),  // dist
		image.Rect(13, 542, 149, 575),  // Approach
		image.Rect(154, 542, 255, 575), // Warp
		image.Rect(260, 542, 355, 575), // Orbit
		image.Rect(360, 542, 453, 575), // Stop
		image.Rect(13, 580, 95, 613),   // Lock
		image.Rect(100, 580, 185, 613), // Dock
		image.Rect(190, 580, 277, 613), // Jump
		image.Rect(282, 580, 362, 613), // Loot
		image.Rect(367, 580, 453, 613), // Mine
		image.Rect(13, 618, 453, 651),  // Orbit range selector
		image.Rect(13, 656, 453, 689),  // Warp to within selector
	}
	assertRectsEqual(t, "1024x700", got, want)

	_, spMin, _ := pumpSelected(t, 1024, 700, true, false)
	gotMin := selectedRects(spMin)
	wantMin := []image.Rectangle{
		image.Rect(5, 49, 445, 79),     // name (icon + label row)
		image.Rect(5, 84, 445, 84),     // dist (empty line, zero height)
		image.Rect(5, 89, 141, 121),    // Approach
		image.Rect(146, 89, 247, 121),  // Warp
		image.Rect(252, 89, 347, 121),  // Orbit
		image.Rect(352, 89, 445, 121),  // Stop
		image.Rect(5, 126, 87, 158),    // Lock
		image.Rect(92, 126, 177, 158),  // Dock
		image.Rect(182, 126, 269, 158), // Jump
		image.Rect(274, 126, 354, 158), // Loot
		image.Rect(359, 126, 445, 158), // Mine
		image.Rect(5, 163, 445, 195),   // Orbit range selector
		image.Rect(5, 200, 445, 232),   // Warp to within selector
	}
	assertRectsEqual(t, "min-450x240", gotMin, wantMin)
}

// assertRectsEqual diffs two rect tuples.
func assertRectsEqual(t *testing.T, tag string, got, want []image.Rectangle) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d rects, want %d", tag, len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s: rect %d = %v, want %v", tag, i, got[i], want[i])
		}
	}
}

// TestSelectedDistLineRelayout covers the stale-layout defect: the dist
// line starts with an empty label (zero-height rect) and must be given a
// fresh rect — not left sitting over the first button row — the moment
// the selection puts text in it.
func TestSelectedDistLineRelayout(t *testing.T) {
	k, _ := newTestKit(t)
	pumpKit(t, k, 1024, 700)
	pumpKit(t, k, 1024, 700)
	sp := k.sp

	if d := sp.dist.GetWidget().Rect; d.Dy() != 0 {
		t.Fatalf("empty dist rect = %v, want zero height", d)
	}

	var target sim.Info
	for _, info := range k.Sim.Grid() {
		if info.CanWarp {
			target = info
			break
		}
	}
	k.Select(target.Entity)
	pumpKit(t, k, 1024, 700)
	pumpKit(t, k, 1024, 700)

	dist, approach := sp.dist.GetWidget().Rect, sp.approach.GetWidget().Rect
	if dist.Dy() <= 0 {
		t.Fatalf("dist rect = %v after selecting %q, want a measured line", dist, sp.name.Label)
	}
	if dist.Overlaps(approach) {
		t.Errorf("dist %v overlaps the first button row %v", dist, approach)
	}
	if dist.Max.Y > approach.Min.Y {
		t.Errorf("dist %v ends below the first button row %v", dist, approach)
	}
	// The full stack still satisfies the layout contract after the swap.
	_, _, inner := pumpSelected(t, 1024, 700, false, true)
	_ = inner
	assertSelectedLayout(t, "relayout",
		sp.body.GetWidget().Rect.Inset(contentsPad), selectedRects(sp), selectedBands[:], true)
}

// TestSelectedRangeControls walks the pure selectors and the action
// buttons: a selector click only cycles its own value — a full cycle
// wraps exactly back to the start and enqueues nothing at any step, with
// or without a target — while the Warp/Orbit ACTION buttons enqueue
// exactly CmdWarpTo/CmdOrbit carrying the value the selector shows at
// click time. Kit.WarpRange/Kit.OrbitRange read that same value back:
// it is what the Z/Q keybinds act at (game pins that half).
func TestSelectedRangeControls(t *testing.T) {
	k, cmds := newTestKit(t)
	pumpKit(t, k, 1280, 720)
	pumpKit(t, k, 1280, 720)
	sp := k.sp

	if got, want := btnLabel(sp.orbitRange), "Orbit range: 2,500 m »"; got != want {
		t.Errorf("initial orbit selector = %q, want %q", got, want)
	}
	if got, want := btnLabel(sp.warpRange), "Warp to within: 0 m »"; got != want {
		t.Errorf("initial warp selector = %q, want %q", got, want)
	}

	// No selection: every ACTION is disabled, both selectors stay enabled
	// (they are state, not actions).
	for name, b := range map[string]*widget.Button{
		"approach": sp.approach, "warp": sp.warp, "orbit": sp.orbit, "stop": sp.stop,
		"lock": sp.lock, "dock": sp.dock, "jump": sp.jump, "loot": sp.loot, "mine": sp.mine,
	} {
		if !b.GetWidget().Disabled {
			t.Errorf("%s enabled with no selection", name)
		}
	}
	if sp.orbitRange.GetWidget().Disabled || sp.warpRange.GetWidget().Disabled {
		t.Error("range selector disabled with no selection")
	}

	// A full orbit cycle: five clicks walk every value and wrap exactly
	// to the start. The selectors enqueue nothing, ever.
	orbitWalk := []struct {
		label string
		rng   float64
	}{
		{"Orbit range: 5,000 m »", 5000},
		{"Orbit range: 10,000 m »", 10000},
		{"Orbit range: 20,000 m »", 20000},
		{"Orbit range: 50,000 m »", 50000},
		{"Orbit range: 2,500 m »", 2500}, // wraparound lands on the start
	}
	for i, want := range orbitWalk {
		fireButton(k.ui, sp.orbitRange)
		if got := btnLabel(sp.orbitRange); got != want.label {
			t.Errorf("orbit selector click %d: label = %q, want %q", i+1, got, want.label)
		}
		if got := k.OrbitRange(); got != want.rng {
			t.Errorf("orbit selector click %d: Kit.OrbitRange() = %v, want %v", i+1, got, want.rng)
		}
	}
	warpWalk := []struct {
		label string
		rng   float64
	}{
		{"Warp to within: 10,000 m »", 10000},
		{"Warp to within: 50,000 m »", 50000},
		{"Warp to within: 0 m »", 0}, // wraparound lands on the start
	}
	for i, want := range warpWalk {
		fireButton(k.ui, sp.warpRange)
		if got := btnLabel(sp.warpRange); got != want.label {
			t.Errorf("warp selector click %d: label = %q, want %q", i+1, got, want.label)
		}
		if got := k.WarpRange(); got != want.rng {
			t.Errorf("warp selector click %d: Kit.WarpRange() = %v, want %v", i+1, got, want.rng)
		}
	}
	if len(*cmds) != 0 {
		t.Fatalf("cycling the selectors enqueued %d commands, want 0", len(*cmds))
	}

	// A warpable target far enough to engage unlocks the actions.
	target := warpableTarget(t, k, sim.WarpMinRange)
	k.Select(target.Entity)
	pumpKit(t, k, 1280, 720)
	if sp.warp.GetWidget().Disabled || sp.orbit.GetWidget().Disabled {
		t.Fatal("warp/orbit disabled for a warpable target")
	}

	// Selectors still cycle silently WITH a target: the old
	// advance-then-enqueue behaviour is gone.
	before := len(*cmds)
	fireButton(k.ui, sp.warpRange)  // 0 -> 10,000
	fireButton(k.ui, sp.orbitRange) // 2,500 -> 5,000
	if len(*cmds) != before {
		t.Fatalf("cycling with a target enqueued %d commands, want 0", len(*cmds))
	}

	// The action buttons carry the selector value AT CLICK TIME.
	fireButton(k.ui, sp.warp)
	fireButton(k.ui, sp.orbit)
	if len(*cmds) != before+2 {
		t.Fatalf("actions enqueued %d commands, want 2", len(*cmds)-before)
	}
	wcmd, ok := (*cmds)[before].(sim.CmdWarpTo)
	if !ok || wcmd.Target != target.Entity || wcmd.Range != k.WarpRange() {
		t.Errorf("Warp action = %#v, want CmdWarpTo{Target:%v Range:%v}",
			(*cmds)[before], target.Entity, k.WarpRange())
	}
	if wcmd.Range != 10000 {
		t.Errorf("Warp action range = %v, want the selector's 10000", wcmd.Range)
	}
	ocmd, ok := (*cmds)[before+1].(sim.CmdOrbit)
	if !ok || ocmd.Target != target.Entity || ocmd.Range != k.OrbitRange() {
		t.Errorf("Orbit action = %#v, want CmdOrbit{Target:%v Range:%v}",
			(*cmds)[before+1], target.Entity, k.OrbitRange())
	}
	if ocmd.Range != 5000 {
		t.Errorf("Orbit action range = %v, want the selector's 5000", ocmd.Range)
	}
	*cmds = (*cmds)[:before]

	// The existing seam is unchanged: Approach and Stop enqueue exactly
	// what they always did.
	fireButton(k.ui, sp.approach)
	fireButton(k.ui, sp.stop)
	if len(*cmds) != 2 {
		t.Fatalf("Approach/Stop enqueued %d commands, want 2", len(*cmds))
	}
	if c, ok := (*cmds)[0].(sim.CmdApproach); !ok || c.Target != target.Entity {
		t.Errorf("cmd = %#v, want CmdApproach", (*cmds)[0])
	}
	if _, ok := (*cmds)[1].(sim.CmdStop); !ok {
		t.Errorf("cmd = %#v, want CmdStop", (*cmds)[1])
	}
	*cmds = (*cmds)[:before]

	// A non-warpable target (an asteroid): the Warp action is gated off,
	// the Orbit action still acts, and the selectors still cycle.
	var rock sim.Info
	for _, info := range k.Sim.Grid() {
		if info.Kind == sim.KindAsteroid && !info.CanWarp {
			rock = info
			break
		}
	}
	if rock.Entity.IsZero() {
		t.Fatal("fixture: no asteroid on the grid")
	}
	k.Select(rock.Entity)
	pumpKit(t, k, 1280, 720)
	if !sp.warp.GetWidget().Disabled {
		t.Error("Warp action enabled for a non-warpable target")
	}
	fireButton(k.ui, sp.warpRange)
	fireButton(k.ui, sp.orbit)
	if len(*cmds) != 1 {
		t.Fatalf("on an asteroid: %d commands, want 1 (the orbit action)", len(*cmds))
	}
	if c, ok := (*cmds)[0].(sim.CmdOrbit); !ok || c.Target != rock.Entity || c.Range != k.OrbitRange() {
		t.Errorf("cmd = %#v, want CmdOrbit for the asteroid at the selector value", (*cmds)[0])
	}
	*cmds = (*cmds)[:before]
}

// warpableTarget picks a CanWarp grid entity at least min metres from the
// player (the warp minimum gate), or fails the fixture.
func warpableTarget(t *testing.T, k *Kit, min float64) sim.Info {
	t.Helper()
	for _, info := range k.Sim.Grid() {
		if info.CanWarp && info.Distance >= min {
			return info
		}
	}
	t.Fatalf("fixture: no warpable target at least %v m away", min)
	return sim.Info{}
}

// TestWarpMinimumRangeGate pins the UI half of EVE's 150 km rule: the
// Warp action is disabled while the selected target sits inside
// sim.WarpMinRange and says so in a hover hint, and unlocks again the
// moment the target is far enough — the constant in sim is the only
// source of the number.
func TestWarpMinimumRangeGate(t *testing.T) {
	k, _ := newTestKit(t)
	pumpKit(t, k, 1280, 720)
	pumpKit(t, k, 1280, 720)
	sp := k.sp

	target := warpableTarget(t, k, sim.WarpMinRange)
	k.Select(target.Entity)
	pumpKit(t, k, 1280, 720)
	if sp.warp.GetWidget().Disabled {
		t.Fatal("Warp disabled outside the minimum range")
	}
	if tips := sp.warp.GetWidget().ToolTips; len(tips) != 0 {
		t.Errorf("Warp hint attached outside the minimum range: %d tooltips", len(tips))
	}

	// Park the target WarpMinRange-1000 m from the player: inside the
	// gate. Info.Distance is computed live, so no step is needed.
	tr, ok := k.Sim.Transforms.Get(target.Entity)
	if !ok {
		t.Fatal("target has no transform")
	}
	ply, _ := k.Sim.Transforms.Get(k.Sim.Player())
	near := ply.Pos
	near.X += sim.WarpMinRange - 1000
	k.Sim.Transforms.Set(target.Entity, sim.Transform{Pos: near})
	k.Sync()
	if !sp.warp.GetWidget().Disabled {
		t.Fatal("Warp enabled inside the minimum range")
	}
	if tips := sp.warp.GetWidget().ToolTips; len(tips) != 1 {
		t.Fatalf("Warp hint not attached inside the minimum range: %d tooltips", len(tips))
	}
	if m := sp.warpTipView.Model(); m.Icon != "act_warp" || m.Title != "Warp unavailable" ||
		m.Sub != "Target within warp minimum range (150 km)" {
		t.Errorf("Warp hint = %q/%q/%q, want act_warp / Warp unavailable / the quoted reason",
			m.Icon, m.Title, m.Sub)
	}
	// The hint quotes sim.WarpMinRange, not a literal.
	if kmLabel(sim.WarpMinRange) != "150 km" {
		t.Fatalf("kmLabel(%v) = %q, want 150 km", sim.WarpMinRange, kmLabel(sim.WarpMinRange))
	}

	// Exactly at the minimum the drive may engage (== is allowed).
	at := ply.Pos
	at.X += sim.WarpMinRange
	k.Sim.Transforms.Set(target.Entity, sim.Transform{Pos: at})
	k.Sync()
	if sp.warp.GetWidget().Disabled {
		t.Error("Warp disabled at exactly WarpMinRange")
	}
	// Selectors are state: they stay enabled through the gate.
	if sp.warpRange.GetWidget().Disabled {
		t.Error("warp selector disabled inside the minimum range")
	}
	_ = tr
}

// TestSelectedButtonContentFitsItsCell: an icon+label action button never
// asks for more width than its cell hands out, so the glyph+label group can
// never bleed into its neighbour — at both design sizes and at the window
// minimum (the whole reason the minimum grew with the icons).
func TestSelectedButtonContentFitsItsCell(t *testing.T) {
	cases := []struct {
		tag     string
		w, h    int
		minSize bool
	}{
		{"1280x720", 1280, 720, false},
		{"1024x700", 1024, 700, false},
		{"min-450x240", 1024, 700, true},
	}
	for _, tc := range cases {
		_, sp, _ := pumpSelected(t, tc.w, tc.h, tc.minSize, true)
		for _, b := range []*widget.Button{
			sp.approach, sp.warp, sp.orbit, sp.stop,
			sp.lock, sp.dock, sp.jump, sp.loot, sp.mine,
		} {
			nat, _ := b.PreferredSize()
			if cell := b.GetWidget().Rect.Dx(); nat > cell {
				label := ""
				if txt := b.Text(); txt != nil {
					label = txt.Label
				}
				t.Errorf("%s: %q button content %d px > its %d px cell",
					tc.tag, label, nat, cell)
			}
		}
	}
}
