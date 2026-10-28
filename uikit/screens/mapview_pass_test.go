package screens

import (
	"image"
	"math"
	"testing"

	"github.com/entieve/entieve/uikit/widgets"
)

// TestMapViewIgnoresThePopupsPress pins the starmap's half of the popup
// rule: the node poll reads the frame's press (Pointer.Press, the
// SplitPane idiom), so the press that lands a context-menu entry — or
// dismisses the menu — is spent on the popup, never on the node under it,
// or the map re-selects a system behind the popup.
func TestMapViewIgnoresThePopupsPress(t *testing.T) {
	env, _ := newChromeEnv(t)
	m := NewMapView(env)
	newChromeUI(env).AddWindow(m).Frame(t, 1280, 720)

	// A node point that really hit-tests as its own node.
	r := m.gfx.GetWidget().Rect
	var pt image.Point
	found := false
	for i := range m.pts {
		pt = image.Pt(int(math.Round(float64(r.Min.X)+m.pts[i].x)), int(math.Round(float64(r.Min.Y)+m.pts[i].y)))
		if id, ok := m.nodeAt(pt); ok && id == m.nodes[i].id {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("fixture: no starmap node is hit-testable")
	}

	env.in.x, env.in.y = pt.X, pt.Y
	env.in.edge = [3]bool{true, false, false}
	env.Pointer().Begin()

	menu := env.Menu()
	menu.Open([]widgets.MenuEntry{{Label: "Show Info"}}, pt)
	if !menu.Poll() {
		t.Fatal("the popup did not take the frame's press")
	}
	m.Sync() // the map polls the same frame, as every panel Sync does

	if m.sel != "" || m.dest != "" {
		t.Errorf("the starmap took the popup's press: sel = %q, dest = %q", m.sel, m.dest)
	}
}

// TestMapViewPollsWhenNoPopupClaimedThePress is the same poll without a
// popup: a press on a node selects it, so the pin above is not vacuous.
func TestMapViewPollsWhenNoPopupClaimedThePress(t *testing.T) {
	env, _ := newChromeEnv(t)
	m := NewMapView(env)
	newChromeUI(env).AddWindow(m).Frame(t, 1280, 720)

	r := m.gfx.GetWidget().Rect
	pt := image.Pt(int(math.Round(float64(r.Min.X)+m.pts[0].x)), int(math.Round(float64(r.Min.Y)+m.pts[0].y)))
	if _, ok := m.nodeAt(pt); !ok {
		t.Skip("fixture: node 0 is not hit-testable at this scale")
	}
	env.in.x, env.in.y = pt.X, pt.Y
	env.in.edge = [3]bool{true, false, false}
	env.Pointer().Begin()

	m.Sync()

	if m.sel != m.nodes[0].id {
		t.Errorf("a free press did not select node 0: sel = %q, want %q", m.sel, m.nodes[0].id)
	}
}
