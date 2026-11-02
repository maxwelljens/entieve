package screens

import (
	"image"
	"reflect"
	"testing"

	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
)

// proficiencyChain is the three-node fixture the layout pins read: A <- B
// <- C, with C's gate unmet (B is level 2, C wants 3). Gate is the sim's
// sentence, so the fixture hands it in exactly as the sim would.
func proficiencyChain() []ProfFamily {
	return []ProfFamily{
		{Family: "fam-a", Name: "Alpha", Level: 2, MaxLevel: 5, XP: 40, XPNext: 160, Meets: true, Starter: true},
		{Family: "fam-b", Name: "Bravo", Level: 2, MaxLevel: 5, XP: 40, XPNext: 160, Meets: true,
			Prereqs: []ProfPrereq{{Family: "fam-a", Level: 2}}},
		{Family: "fam-c", Name: "Charlie", Level: 0, MaxLevel: 5, XP: 0, XPNext: 80, Meets: false,
			Gate: "Requires Bravo III", Prereqs: []ProfPrereq{{Family: "fam-b", Level: 3}}},
	}
}

// TestProficiencyChainLayout: the prerequisite chain draws one arrow per
// edge, the ranks follow the chain (never the input order), a family
// whose prerequisite is unmet is laid out locked with its gate named, and
// the level/XP readouts are exactly the model's numbers.
func TestProficiencyChainLayout(t *testing.T) {
	uitest.UseScale(t, 1)
	canvas := image.Rect(0, 0, 1280, 720)
	nodes, edges := proficiencyLayout(proficiencyChain(), canvas)

	if len(nodes) != 3 || len(edges) != 2 {
		t.Fatalf("layout = %d nodes, %d edges, want 3 and 2", len(nodes), len(edges))
	}
	byID := map[string]profNode{}
	for _, n := range nodes {
		byID[n.Family] = n
	}
	// Ranks: the chain decides them, not the input order.
	if byID["fam-a"].Rank != 0 || byID["fam-b"].Rank != 1 || byID["fam-c"].Rank != 2 {
		t.Fatalf("ranks = a:%d b:%d c:%d, want 0, 1, 2",
			byID["fam-a"].Rank, byID["fam-b"].Rank, byID["fam-c"].Rank)
	}
	// One column per rank, left to right.
	if !(byID["fam-a"].Rect.Min.X < byID["fam-b"].Rect.Min.X &&
		byID["fam-b"].Rect.Max.X <= byID["fam-c"].Rect.Min.X) {
		t.Fatalf("columns: a %v, b %v, c %v are not left to right",
			byID["fam-a"].Rect, byID["fam-b"].Rect, byID["fam-c"].Rect)
	}
	// The two arrows: prerequisite -> dependent, both prerequisite edges
	// in the model.
	idx := map[string]int{}
	for i, n := range nodes {
		idx[n.Family] = i
	}
	want := map[[2]int]bool{
		{idx["fam-a"], idx["fam-b"]}: true,
		{idx["fam-b"], idx["fam-c"]}: true,
	}
	got := map[[2]int]bool{}
	for _, e := range edges {
		got[[2]int{e.From, e.To}] = true
		if e.From == idx["fam-c"] {
			t.Errorf("edge %d -> %d leaves the dependent (arrows point prerequisite -> dependent)", e.From, e.To)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("edges = %v, want %v", got, want)
	}

	// Gates and readouts, straight off the fixture.
	b, c := byID["fam-b"], byID["fam-c"]
	if b.Locked || !c.Locked {
		t.Fatalf("locked = b:%v c:%v, want false and true (C needs B 3, B is 2)", b.Locked, c.Locked)
	}
	if c.GateText != "Requires Bravo III" {
		t.Errorf("C gate = %q, want %q", c.GateText, "Requires Bravo III")
	}
	if b.GateText != "" {
		t.Errorf("B gate = %q, want none", b.GateText)
	}
	if b.LevelText != "2/5" || b.XPText != "40/160 XP" {
		t.Errorf("B readouts = %q, %q, want 2/5 and 40/160 XP", b.LevelText, b.XPText)
	}
	// At the cap the pair is 0 and reads "max".
	capped := profXPText(ProfFamily{Level: 5, MaxLevel: 5})
	if capped != "max" {
		t.Errorf("capped XP readout = %q, want max", capped)
	}
	// Every node stays on the canvas.
	for _, n := range nodes {
		if !n.Rect.In(canvas) || n.Rect.Empty() {
			t.Errorf("%s node %v is not on the canvas %v", n.Family, n.Rect, canvas)
		}
	}
}

// TestProficiencyLayoutDeterministic: the layout is reproducible from the
// model alone. Two calls give identical geometry, and a shuffled input
// order gives the same positions keyed by family (the id is the tiebreak,
// never the slice order).
func TestProficiencyLayoutDeterministic(t *testing.T) {
	uitest.UseScale(t, 1)
	canvas := image.Rect(0, 0, 1280, 720)
	fams := proficiencyChain()

	n1, e1 := proficiencyLayout(fams, canvas)
	n2, e2 := proficiencyLayout(fams, canvas)
	if !reflect.DeepEqual(n1, n2) || !reflect.DeepEqual(e1, e2) {
		t.Fatalf("two layouts of one model differ:\n%v\n%v", n1, n2)
	}

	shuffled := []ProfFamily{fams[2], fams[0], fams[1]}
	n3, e3 := proficiencyLayout(shuffled, canvas)
	pos := func(ns []profNode) map[string]image.Rectangle {
		out := map[string]image.Rectangle{}
		for _, n := range ns {
			out[n.Family] = n.Rect
		}
		return out
	}
	if !reflect.DeepEqual(pos(n1), pos(n3)) || len(e3) != len(e1) {
		t.Fatalf("input order moved the layout:\n%v\n%v", pos(n1), pos(n3))
	}
}

// TestProficiencyLayerMountsBelowWindowsAndOwnsBareSpace: the layer-order
// pin through Kit.Own. With the tree mounted and an ordinary window over
// it, a press on the window stays the window's and a press on bare layer
// space is the layer's (inert, but never the world's), and unmounting
// gives the world point back.
func TestProficiencyLayerMountsBelowWindowsAndOwnsBareSpace(t *testing.T) {
	uitest.UseScale(t, 1)
	k, _ := newTestKit(t)
	fin := NewFinances(k.Env())
	k.AddWindow(fin.Win())
	fin.Win().SetLocation(image.Rect(600, 120, 1000, 460))
	pumpKit(t, k, 1280, 720)

	worldPt := image.Pt(theme.Px(900), theme.Px(600))
	if l := k.Own(worldPt).Layer; l != LayerWorld {
		t.Fatalf("fixture: world point already owned by layer %d", l)
	}

	p := NewProficiency(k.Env(), proficiencyChain, nil, nil)
	p.Sync()
	remove := k.AddBackdropWindow(p.Win())
	pumpKit(t, k, 1280, 720)
	p.Sync()

	winPt := mid(fin.Win().GetContainer().GetWidget().Rect)
	if o := k.Own(winPt); o.Layer != LayerWindow || o.Window != fin.Win() {
		t.Fatalf("press on the window over the tree = {layer %d, window %v}, want the window", o.Layer, o.Window)
	}
	bare := k.Own(worldPt)
	if bare.Layer == LayerWorld || bare.Window != nil || bare.Control {
		t.Fatalf("bare tree space = %+v, want an inert non-world owner", bare)
	}
	if !k.Over(worldPt) {
		t.Error("the tree's bare space does not block the world gate")
	}

	remove()
	pumpKit(t, k, 1280, 720)
	if l := k.Own(worldPt).Layer; l != LayerWorld {
		t.Fatalf("after unmount the world point is layer %d, want world", l)
	}
}

// TestProficiencyReopenRestoresTheSameLayout: closing and reopening the
// layer re-derives the same nodes and edges from the same model (the
// panel holds no layout state of its own).
func TestProficiencyReopenRestoresTheSameLayout(t *testing.T) {
	uitest.UseScale(t, 1)
	k, _ := newTestKit(t)
	p := NewProficiency(k.Env(), proficiencyChain, nil, nil)
	p.Sync()
	pumpKit(t, k, 1280, 720)
	firstNodes, firstEdges := p.nodes, p.edges
	if len(firstNodes) != 3 || len(firstEdges) != 2 {
		t.Fatalf("fixture: %d nodes %d edges", len(firstNodes), len(firstEdges))
	}
	redraws := p.redraw

	remove := k.AddBackdropWindow(p.Win())
	p.Sync()
	remove()
	p.Sync()
	again := k.AddBackdropWindow(p.Win())
	p.Sync()
	again()
	if !reflect.DeepEqual(p.nodes, firstNodes) || !reflect.DeepEqual(p.edges, firstEdges) {
		t.Fatalf("reopen moved the layout:\n%v\n%v", p.nodes, firstNodes)
	}
	if p.redraw != redraws {
		t.Errorf("model unchanged but the layer repainted (%d → %d)", redraws, p.redraw)
	}
}
