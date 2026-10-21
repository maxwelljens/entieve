package screens

import (
	"image"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestMapViewDrawsAndRedraws pins the graph repaint rule: first sync
// paints, an unchanged system does not repaint, a system change does.
func TestMapViewDrawsAndRedraws(t *testing.T) {
	env, _ := newChromeEnv(t)
	m := NewMapView(env)
	if m.Win() == nil {
		t.Fatal("MapView.Win() is nil")
	}
	m.Sync()
	if !m.drawn || m.system != content.SystemID("sys-solace") {
		t.Fatalf("after first Sync drawn=%v system=%q, want sys-solace", m.drawn, m.system)
	}
	if m.redraws != 1 {
		t.Fatalf("first Sync repainted %d times, want 1", m.redraws)
	}
	m.Sync() // same system → no repaint
	if m.redraws != 1 {
		t.Errorf("unchanged sync repainted (%d)", m.redraws)
	}

	if err := env.Sim.LoadSystem("sys-penumbra", ""); err != nil {
		t.Fatalf("LoadSystem: %v", err)
	}
	m.Sync()
	if m.system != content.SystemID("sys-penumbra") || m.redraws != 2 {
		t.Errorf("after jump system=%q redraws=%d, want sys-penumbra/2", m.system, m.redraws)
	}
}

// TestMapViewNodesAndProjection checks the universe graph inputs: every
// system in content (id-sorted, no duplicates) projects inside the image
// with distinct points, every gate edge resolves to a node so the
// connections actually draw, and every node carries its display name (the
// system ids never reach the map's text).
func TestMapViewNodesAndProjection(t *testing.T) {
	env, _ := newChromeEnv(t)
	m := NewMapView(env)
	nodes := m.mapNodes()
	if len(nodes) != len(env.Content.Systems) {
		t.Fatalf("nodes = %d, want %d", len(nodes), len(env.Content.Systems))
	}
	ids := map[content.SystemID]bool{}
	for i, n := range nodes {
		if ids[n.id] {
			t.Errorf("duplicate system node %q", n.id)
		}
		ids[n.id] = true
		if i > 0 && nodes[i-1].id >= n.id {
			t.Errorf("nodes not id-sorted: %q before %q", nodes[i-1].id, n.id)
		}
		if n.name == "" || n.name == string(n.id) {
			t.Errorf("node %q carries no display name (%q)", n.id, n.name)
		}
		if want := env.Content.Systems[n.id].Name; n.name != want {
			t.Errorf("node %q name = %q, want the content name %q", n.id, n.name, want)
		}
	}

	pts := m.project(nodes)
	for i, pt := range pts {
		if pt.x < 0 || pt.x > mapImgW || pt.y < 0 || pt.y > mapImgH {
			t.Errorf("node %q projects to (%v,%v), outside the %dx%d image",
				nodes[i].id, pt.x, pt.y, mapImgW, mapImgH)
		}
		for j := 0; j < i; j++ {
			if pts[j] == pt {
				t.Errorf("nodes %q and %q project to the same point (%v,%v)",
					nodes[j].id, nodes[i].id, pt.x, pt.y)
			}
		}
	}

	// Every gate must resolve, or its edge is silently skipped while drawing.
	for _, n := range nodes {
		def, ok := env.Content.System(n.id)
		if !ok {
			t.Fatalf("system %q missing from content", n.id)
		}
		for i := range def.Celestials {
			g := def.Celestials[i].Gate
			if g != nil && !ids[g.ToSystem] {
				t.Errorf("gate in %q points at unknown system %q", n.id, g.ToSystem)
			}
		}
	}
}

// TestMapViewDrawSmoke draws the starmap at both design sizes.
func TestMapViewDrawSmoke(t *testing.T) {
	env, _ := newChromeEnv(t)
	m := NewMapView(env)
	c := newChromeUI(env)
	c.AddWindow(m)
	c.Smoke(t)
}

// TestZoneLabel pins the in-universe zone vocabulary of the map legend
// (docs/lore/UNIVERSE.md — Zones): the code values never reach display
// text, only the zone word plus its complementary status label.
func TestZoneLabel(t *testing.T) {
	for _, c := range []struct{ sec, want string }{
		{"high", "Charter · secure"},
		{"low", "Fringe · contested"},
		{"null", "Unincorporated · no charter"},
		{"", ""}, {"med", ""}, // unknown codes render nothing
	} {
		if got := zoneLabel(c.sec); got != c.want {
			t.Errorf("zoneLabel(%q) = %q, want %q", c.sec, got, c.want)
		}
	}
}

// mapUnderTest lays the map out at 1280×720 and returns it with a screen
// point for one node's centre and for its label band.
func mapUnderTest(t *testing.T) (*MapView, func(content.SystemID) image.Point, func(content.SystemID) image.Point) {
	t.Helper()
	env, _ := newChromeEnv(t)
	m := NewMapView(env)
	c := newChromeUI(env)
	c.AddWindow(m)
	c.Frame(t, 1280, 720)
	c.Frame(t, 1280, 720) // second frame: geometry settled, nodes laid out
	rect := m.gfx.GetWidget().Rect
	if rect.Empty() {
		t.Fatal("fixture: graphic never laid out")
	}
	at := func(id content.SystemID) image.Point {
		for i, n := range m.nodes {
			if n.id == id {
				return rect.Min.Add(image.Pt(int(m.pts[i].x), int(m.pts[i].y)))
			}
		}
		t.Fatalf("fixture: no node %q", id)
		return image.Point{}
	}
	label := func(id content.SystemID) image.Point {
		for i, n := range m.nodes {
			if n.id == id {
				r := m.labels[i]
				return rect.Min.Add(image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2))
			}
		}
		t.Fatalf("fixture: no node %q", id)
		return image.Point{}
	}
	return m, at, label
}

// TestMapViewHitTest pins the click resolution: a node's disc, its label
// band and a point outside the graph resolve as the pointer expects.
func TestMapViewHitTest(t *testing.T) {
	m, at, label := mapUnderTest(t)
	if id, ok := m.nodeAt(at("sys-glusza")); !ok || id != "sys-glusza" {
		t.Errorf("nodeAt(centre) = %q,%v, want sys-glusza,true", id, ok)
	}
	if id, ok := m.nodeAt(label("sys-glusza")); !ok || id != "sys-glusza" {
		t.Errorf("nodeAt(label) = %q,%v, want sys-glusza,true (a name click resolves)", id, ok)
	}
	if id, ok := m.nodeAt(image.Pt(1, 1)); ok {
		t.Errorf("nodeAt(1,1) = %q, want no node", id)
	}
}

// TestMapViewHoverSelectDestination drives the pointer through the kit's
// seams: hover lights a node, a press selects it, a second press sets it
// as the destination, a third clears it — and each change repaints.
func TestMapViewHoverSelectDestination(t *testing.T) {
	m, at, _ := mapUnderTest(t)
	cursor := at("sys-glusza")
	restoreCursor := widgets.SetPollCursorForTest(func() (int, int) { return cursor.X, cursor.Y })
	defer restoreCursor()
	pressed := false
	restorePress := widgets.SetPressEdgeForTest(func(ebiten.MouseButton) bool { return pressed })
	defer restorePress()

	m.Sync()
	if m.hover != "sys-glusza" {
		t.Fatalf("hover = %q, want sys-glusza", m.hover)
	}
	if m.sel != "" {
		t.Fatalf("hover selected %q", m.sel)
	}
	pressed = true
	m.Sync()
	pressed = false
	if m.sel != "sys-glusza" || m.dest != "" {
		t.Fatalf("first press: sel=%q dest=%q, want glusza/none", m.sel, m.dest)
	}
	pressed = true
	m.Sync()
	pressed = false
	if m.dest != "sys-glusza" {
		t.Fatalf("second press: dest=%q, want sys-glusza", m.dest)
	}
	pressed = true
	m.Sync()
	pressed = false
	if m.dest != "" {
		t.Errorf("third press: dest=%q, want cleared", m.dest)
	}

	// The current system never becomes a destination: a press on it just
	// selects it (a route to where you are is no route).
	cursor = at("sys-solace")
	pressed = true
	m.Sync()
	pressed = false
	if m.sel != "sys-solace" || m.dest != "" {
		t.Errorf("current system press: sel=%q dest=%q, want solace/none", m.sel, m.dest)
	}

	// A pointer away from every node clears hover again.
	cursor = image.Pt(1, 1)
	m.Sync()
	if m.hover != "" {
		t.Errorf("hover = %q after leaving the graph, want none", m.hover)
	}
}

// TestMapViewRoute pins the destination route: BFS over the authored gate
// graph, both ends included, every step a real gate — and no route to
// nothing.
func TestMapViewRoute(t *testing.T) {
	m, _, _ := mapUnderTest(t)
	path := m.route("sys-solace", "sys-glusza")
	if len(path) != 4 || path[0] != "sys-solace" || path[3] != "sys-glusza" {
		t.Fatalf("route = %v, want solution → penumbra → gloam → glusza", path)
	}
	gated := func(a, b content.SystemID) bool {
		def, ok := m.cv.Defs().System(a)
		if !ok {
			return false
		}
		for i := range def.Celestials {
			if g := def.Celestials[i].Gate; g != nil && g.ToSystem == b {
				return true
			}
		}
		return false
	}
	for i := 0; i+1 < len(path); i++ {
		if !gated(path[i], path[i+1]) && !gated(path[i+1], path[i]) {
			t.Errorf("route step %q → %q is not a gate", path[i], path[i+1])
		}
	}
	for _, c := range [][2]content.SystemID{{"", "sys-glusza"}, {"sys-solace", ""}, {"sys-solace", "sys-solace"}} {
		if got := m.route(c[0], c[1]); got != nil {
			t.Errorf("route(%q,%q) = %v, want nil", c[0], c[1], got)
		}
	}
}
