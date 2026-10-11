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
		// SetLocation applies the window's MinSize clamp (300×240).
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
		{"min-300x240", 1280, 720, true, true},
		{"min-300x240-nosel", 1024, 700, true, false},
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
		bands: [][]float64{{120, 170, 70}, {60, 60, 60, 60, 60}, {300}, {300}},
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
// design size (default window rect) and at the 300×240 minimum, so any
// drift in the design math shows up as a tuple diff.
func TestSelectedLayoutPinnedRects(t *testing.T) {
	_, sp, _ := pumpSelected(t, 1024, 700, false, true)
	got := selectedRects(sp)
	want := []image.Rectangle{
		image.Rect(13, 485, 383, 517),  // name
		image.Rect(13, 522, 383, 539),  // dist
		image.Rect(13, 544, 133, 577),  // Approach
		image.Rect(138, 544, 308, 577), // Warp to 10 km
		image.Rect(313, 544, 383, 577), // Stop
		image.Rect(13, 582, 81, 615),   // Lock
		image.Rect(86, 582, 157, 615),  // Dock
		image.Rect(162, 582, 238, 615), // Jump
		image.Rect(243, 582, 308, 615), // Loot
		image.Rect(313, 582, 383, 615), // Mine
		image.Rect(13, 620, 383, 653),  // Orbit
		image.Rect(13, 658, 383, 691),  // Warp to within
	}
	assertRectsEqual(t, "1024x700", got, want)

	_, spMin, _ := pumpSelected(t, 1024, 700, true, false)
	gotMin := selectedRects(spMin)
	wantMin := []image.Rectangle{
		image.Rect(5, 49, 295, 81),     // name
		image.Rect(5, 86, 295, 86),     // dist (empty line, zero height)
		image.Rect(5, 91, 98, 123),     // Approach
		image.Rect(103, 91, 236, 123),  // Warp to 10 km
		image.Rect(241, 91, 295, 123),  // Stop
		image.Rect(5, 128, 57, 160),    // Lock
		image.Rect(62, 128, 117, 160),  // Dock
		image.Rect(122, 128, 181, 160), // Jump
		image.Rect(186, 128, 236, 160), // Loot
		image.Rect(241, 128, 295, 160), // Mine
		image.Rect(5, 165, 295, 197),   // Orbit
		image.Rect(5, 202, 295, 234),   // Warp to within
	}
	assertRectsEqual(t, "min-300x240", gotMin, wantMin)
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

// TestSelectedRangeControls walks the two cycling range controls: label
// cycling with wraparound at both ends, the enqueued command carrying the
// chosen range, the CanWarp gate, and the fact that they stay enabled
// while every action button is disabled with no selection. The existing
// action seam (Warp's fixed 10 km, Approach, Stop) is asserted unchanged.
func TestSelectedRangeControls(t *testing.T) {
	k, cmds := newTestKit(t)
	pumpKit(t, k, 1280, 720)
	pumpKit(t, k, 1280, 720)
	sp := k.sp

	if got, want := btnLabel(sp.orbit), "Orbit: 2,500 m »"; got != want {
		t.Errorf("initial orbit label = %q, want %q", got, want)
	}
	if got, want := btnLabel(sp.warpRange), "Warp to within: 0 m »"; got != want {
		t.Errorf("initial warp range label = %q, want %q", got, want)
	}

	// With no selection the actions are disabled but the selectors still
	// cycle, and nothing is enqueued.
	for _, b := range []*widget.Button{sp.approach, sp.warp, sp.stop, sp.lock, sp.dock, sp.jump, sp.loot, sp.mine} {
		if !b.GetWidget().Disabled {
			t.Error("action button enabled with no selection")
		}
	}
	if sp.orbit.GetWidget().Disabled || sp.warpRange.GetWidget().Disabled {
		t.Error("range selector disabled with no selection")
	}
	for i := 0; i < 6; i++ {
		fireButton(k.ui, sp.orbit)
		fireButton(k.ui, sp.warpRange)
	}
	if len(*cmds) != 0 {
		t.Fatalf("cycling with no selection enqueued %d commands, want 0", len(*cmds))
	}
	if got, want := btnLabel(sp.orbit), "Orbit: 5,000 m »"; got != want {
		t.Errorf("after 6 cycles orbit label = %q, want %q", got, want)
	}

	// A warpable target unlocks the action buttons.
	var target sim.Info
	for _, info := range k.Sim.Grid() {
		if info.CanWarp {
			target = info
			break
		}
	}
	k.Select(target.Entity)
	pumpKit(t, k, 1280, 720)
	if sp.warp.GetWidget().Disabled || sp.orbit.GetWidget().Disabled {
		t.Fatal("warp/orbit disabled for a warpable target")
	}

	// Existing seam unchanged: Warp is still a fixed 10 km, Approach and
	// Stop enqueue exactly what they always did.
	before := len(*cmds)
	fireButton(k.ui, sp.warp)
	fireButton(k.ui, sp.approach)
	fireButton(k.ui, sp.stop)
	if got := (*cmds)[before:]; len(got) != 3 {
		t.Fatalf("existing actions enqueued %d commands, want 3", len(got))
	}
	if c, ok := (*cmds)[before].(sim.CmdWarpTo); !ok || c.Range != 10000 || c.Target != target.Entity {
		t.Errorf("Warp cmd = %#v, want CmdWarpTo{Range:10000}", (*cmds)[before])
	}
	if c, ok := (*cmds)[before+1].(sim.CmdApproach); !ok || c.Target != target.Entity {
		t.Errorf("Approach cmd = %#v, want CmdApproach", (*cmds)[before+1])
	}
	if _, ok := (*cmds)[before+2].(sim.CmdStop); !ok {
		t.Errorf("Stop cmd = %#v, want CmdStop", (*cmds)[before+2])
	}
	*cmds = (*cmds)[:before]

	// Orbit: every click picks the next range (wraparound at the end of
	// the cycle) and enqueues CmdOrbit at exactly that range.
	orbitWants := []float64{10000, 20000, 50000, 2500} // started at 5,000 above
	orbitLabels := []string{"Orbit: 10,000 m »", "Orbit: 20,000 m »", "Orbit: 50,000 m »", "Orbit: 2,500 m »"}
	for i, wantRange := range orbitWants {
		fireButton(k.ui, sp.orbit)
		if len(*cmds) != i+1 {
			t.Fatalf("orbit click %d: %d commands, want %d", i, len(*cmds), i+1)
		}
		c, ok := (*cmds)[i].(sim.CmdOrbit)
		if !ok || c.Target != target.Entity || c.Range != wantRange {
			t.Errorf("orbit click %d: cmd = %#v, want CmdOrbit{Target:%v Range:%v}",
				i, (*cmds)[i], target.Entity, wantRange)
		}
		if got := btnLabel(sp.orbit); got != orbitLabels[i] {
			t.Errorf("orbit click %d: label = %q, want %q", i, got, orbitLabels[i])
		}
	}
	*cmds = (*cmds)[:0]

	// Warp to within: same cycling, gated on Info.CanWarp, range carried
	// into CmdWarpTo (0 lands on the target).
	warpWants := []float64{10000, 50000, 0}
	warpLabels := []string{"Warp to within: 10,000 m »", "Warp to within: 50,000 m »", "Warp to within: 0 m »"}
	for i, wantRange := range warpWants {
		fireButton(k.ui, sp.warpRange)
		if len(*cmds) != i+1 {
			t.Fatalf("warp range click %d: %d commands, want %d", i, len(*cmds), i+1)
		}
		c, ok := (*cmds)[i].(sim.CmdWarpTo)
		if !ok || c.Target != target.Entity || c.Range != wantRange {
			t.Errorf("warp range click %d: cmd = %#v, want CmdWarpTo{Target:%v Range:%v}",
				i, (*cmds)[i], target.Entity, wantRange)
		}
		if got := btnLabel(sp.warpRange); got != warpLabels[i] {
			t.Errorf("warp range click %d: label = %q, want %q", i, got, warpLabels[i])
		}
	}
	*cmds = (*cmds)[:0]

	// A non-warpable target (an asteroid): the selector still cycles but
	// CmdWarpTo is gated off, while CmdOrbit still goes through.
	var rock sim.Info
	for _, info := range k.Sim.Grid() {
		if info.Kind == sim.KindAsteroid && !info.CanWarp {
			rock = info
			break
		}
	}
	k.Select(rock.Entity)
	pumpKit(t, k, 1280, 720)
	fireButton(k.ui, sp.warpRange)
	if len(*cmds) != 0 {
		t.Errorf("warp-to-within on a non-warpable target enqueued %#v, want none", *cmds)
	}
	if got, want := btnLabel(sp.warpRange), "Warp to within: 10,000 m »"; got != want {
		t.Errorf("warp range label = %q, want %q (the cycle still advances)", got, want)
	}
	fireButton(k.ui, sp.orbit)
	if len(*cmds) != 1 {
		t.Fatalf("orbit on an asteroid: %d commands, want 1", len(*cmds))
	}
	if c, ok := (*cmds)[0].(sim.CmdOrbit); !ok || c.Target != rock.Entity {
		t.Errorf("cmd = %#v, want CmdOrbit for the asteroid", (*cmds)[0])
	}
}
