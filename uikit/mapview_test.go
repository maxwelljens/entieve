package uikit

import (
	"testing"

	"github.com/entieve/entieve/content"
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
// with distinct points, and every gate edge resolves to a node so the
// connections actually draw.
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
	c.addWindow(m)
	c.smoke(t)
}
