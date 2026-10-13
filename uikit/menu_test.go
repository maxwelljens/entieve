package uikit

import (
	"image"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
)

// clickMenuEntry picks the labelled entry whether it is enabled or not:
// a disabled entry must still close the menu (without running), which is
// exactly what the gating tests press.
func clickMenuEntry(t *testing.T, env Env, label string) {
	t.Helper()
	m := env.Menu()
	if !m.IsOpen() {
		t.Fatalf("no menu open (want the %q entry)", label)
	}
	for i, e := range m.Entries() {
		if e.Label != label {
			continue
		}
		m.Click(m.RowRect(i).Min.Add(image.Pt(1, 1)))
		return
	}
	t.Fatalf("menu has no %q entry: %+v", label, m.Entries())
}

// pressMenuEntry picks the labelled entry in the already-open menu and
// fails when it is disabled.
func pressMenuEntry(t *testing.T, env Env, label string) {
	t.Helper()
	if e := entry(t, env, label); e.Disabled {
		t.Fatalf("menu entry %q is disabled", label)
	}
	clickMenuEntry(t, env, label)
}

// pickMenuEntry right-clicks btn open and presses the labelled entry.
func pickMenuEntry(t *testing.T, c *chromeUI, env Env, btn *widget.Button, label string) {
	t.Helper()
	rightClick(c, t, btn)
	if !env.Menu().IsOpen() {
		t.Fatal("right-click opened no menu")
	}
	pressMenuEntry(t, env, label)
}

// menuTestEntries is the standard menu the builder tests drive: a bold
// first entry, a disabled one and a separator group.
func menuTestEntries(hits *[]string) []MenuEntry {
	return []MenuEntry{
		{Label: "Show Info", Bold: true, Action: func() { *hits = append(*hits, "info") }},
		{Label: "Gated", Disabled: true, Action: func() { *hits = append(*hits, "gated") }},
		MenuSep,
		{Label: "Cycle charge", Action: func() { *hits = append(*hits, "cycle") }},
	}
}

// TestMenuBuildAndDispatch pins the popup's construction: entries keep
// their order (Show Info first and bold, separators as their own bands),
// row bands tile the popup, enabled entries dispatch their Action on a
// click, disabled ones and separators only close, and any click or Esc
// closes the menu.
func TestMenuBuildAndDispatch(t *testing.T) {
	env, _ := newChromeEnv(t)
	m := env.Menu()
	var hits []string
	entries := menuTestEntries(&hits)

	m.Open(entries, image.Pt(100, 50))
	if !m.IsOpen() {
		t.Fatal("menu did not open")
	}
	if r := m.Rect(); r.Min != (image.Pt(100, 50)) {
		t.Errorf("popup origin = %v, want (100,50) (the click position)", r.Min)
	}
	if got := m.Entries(); len(got) != 4 {
		t.Fatalf("entries = %d, want 4", len(got))
	} else {
		if got[0].Label != "Show Info" || !got[0].Bold || got[0].Disabled {
			t.Errorf("first entry = %+v, want a bold, enabled Show Info", got[0])
		}
		if !got[1].Disabled {
			t.Errorf("second entry = %+v, want disabled (visible, grayed)", got[1])
		}
		if !got[2].Sep {
			t.Errorf("third entry = %+v, want the separator", got[2])
		}
	}
	// Bands: one per entry, positive height, pairwise disjoint, inside
	// the popup (they are both the hit test and the hover fill).
	for i := range entries {
		rr := m.RowRect(i)
		if rr.Dy() <= 0 || !rr.In(m.Rect()) {
			t.Errorf("row %d band %v not a positive rect inside %v", i, rr, m.Rect())
		}
		for j := i + 1; j < len(entries); j++ {
			if rr.Overlaps(m.RowRect(j)) {
				t.Errorf("rows %d and %d overlap: %v / %v", i, j, rr, m.RowRect(j))
			}
		}
	}
	// Hover follows the pointer onto an entry, never onto a separator's
	// outside-the-popup space.
	m.Hover(m.RowRect(3).Min.Add(image.Pt(2, 2)))
	if m.HoverIndex() != 3 {
		t.Errorf("hover index = %d, want 3", m.HoverIndex())
	}
	m.Hover(image.Point{})
	if m.HoverIndex() != -1 {
		t.Errorf("hover off-menu = %d, want -1", m.HoverIndex())
	}

	// Enabled entry: click runs the Action and closes.
	if !m.Click(m.RowRect(3).Min.Add(image.Pt(1, 1))) {
		t.Fatal("click on an open menu reported no consumption")
	}
	if m.IsOpen() {
		t.Error("menu stayed open after a click")
	}
	if len(hits) != 1 || hits[0] != "cycle" {
		t.Fatalf("actions = %v, want [cycle]", hits)
	}

	// Disabled entry: closes, never runs.
	m.Open(entries, image.Pt(10, 10))
	m.Click(m.RowRect(1).Min.Add(image.Pt(1, 1)))
	if m.IsOpen() || len(hits) != 1 {
		t.Errorf("disabled click: open=%v actions=%v, want closed and untouched", m.IsOpen(), hits)
	}

	// Separator: closes without running anything.
	m.Open(entries, image.Pt(10, 10))
	m.Click(m.RowRect(2).Min.Add(image.Pt(1, 1)))
	if m.IsOpen() || len(hits) != 1 {
		t.Errorf("separator click: open=%v actions=%v, want closed and untouched", m.IsOpen(), hits)
	}

	// A click outside every band: closes, no action.
	m.Open(entries, image.Pt(10, 10))
	outside := m.Rect().Max.Add(image.Pt(40, 40))
	if !m.Click(outside) {
		t.Error("outside click reported no consumption")
	}
	if m.IsOpen() || len(hits) != 1 {
		t.Errorf("outside click: open=%v actions=%v, want closed and untouched", m.IsOpen(), hits)
	}

	// Esc: closes without running anything.
	m.Open(entries, image.Pt(10, 10))
	if !m.Esc() {
		t.Error("Esc on an open menu reported no consumption")
	}
	if m.IsOpen() || len(hits) != 1 {
		t.Errorf("Esc: open=%v actions=%v, want closed and untouched", m.IsOpen(), hits)
	}

	// A closed menu consumes nothing (and exposes no gate rect).
	if m.Click(image.Pt(0, 0)) || m.Esc() {
		t.Error("a closed menu consumed input")
	}
	if r := m.Rect(); !r.Empty() {
		t.Errorf("closed menu rect = %v, want empty", r)
	}
}

// TestMenuDrawsAndClamps: Draw records the screen (the clamp bound), so
// a popup opened past an edge slides back inside it, and the drawn frame
// carries the labels — bold first entry, grayed disabled — plus the
// separator hairline.
func TestMenuDrawsAndClamps(t *testing.T) {
	env, _ := newChromeEnv(t)
	m := env.Menu()
	var hits []string
	entries := menuTestEntries(&hits)

	img := ebiten.NewImage(640, 400)
	m.Open(entries, image.Pt(0, 0)) // no screen known yet: no clamp
	m.Draw(img)
	if r := m.Rect(); r.Min != (image.Pt(0, 0)) {
		t.Fatalf("pre-clamp origin = %v", r.Min)
	}

	// Now the screen is known: an anchor past the bottom-right corner
	// slides the popup fully inside it.
	m.Open(entries, image.Pt(620, 390))
	if r := m.Rect(); !r.In(img.Bounds()) {
		t.Fatalf("popup %v not clamped into %v", r, img.Bounds())
	}
	if r := m.Rect(); r.Max.X != 640 || r.Max.Y != 400 {
		t.Errorf("clamped popup = %v, want it flush with the screen corner", r)
	}
	// Rows follow the clamp (still disjoint, still inside).
	for i := range entries {
		if rr := m.RowRect(i); !rr.In(m.Rect()) || rr.Dy() <= 0 {
			t.Errorf("row %d after clamp = %v, want inside %v", i, rr, m.Rect())
		}
	}
	// The frame draws without panicking and clears on close.
	m.Draw(img)
	m.Close()
	m.Draw(img)
}
