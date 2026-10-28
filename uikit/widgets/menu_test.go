package widgets

import (
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

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

// TestPointerSpendsTheFramePress pins the popup's claim on the frame's
// press: the gesture that dismisses the popup (or lands an entry) belongs
// to the popup, so Pointer.Press — what every non-menu poller reads (the
// starmap's nodes, the slider bands, the split panes, the column grips) —
// reports nothing for that frame, while the popup's own raw edge (what
// Menu.Poll read) still did. The next Begin hands the next frame back.
func TestPointerSpendsTheFramePress(t *testing.T) {
	env, _ := newChromeEnv(t)
	m := env.Menu()
	var hits []string

	env.in.x, env.in.y = 10, 10
	env.in.edge = [3]bool{true, true, false}
	env.Pointer().Begin()

	if !env.Pointer().Press(Left) {
		t.Fatal("fixture: the free press edge is already spent")
	}
	m.Open(menuTestEntries(&hits), image.Pt(10, 10))
	if !m.Poll() {
		t.Fatal("the popup did not take the frame's press")
	}
	if env.Pointer().Press(Left) || env.Pointer().Press(Right) {
		t.Error("the popup's own press reached the gated edge too")
	}
	if !env.Pointer().RawPress(Left) {
		t.Error("Spend took the raw edge the popup reads")
	}
	if !m.TakeBlocked() {
		t.Fatal("the press set no consumed-gesture latch")
	}
	env.Pointer().Begin() // the next frame
	if !env.Pointer().Press(Left) {
		t.Error("the frame's Spend outlived its frame: the next one is silent")
	}
	if len(hits) != 0 {
		t.Errorf("a dismiss outside every band ran %v, want nothing", hits)
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

// TestMenuHitTestMatchesScaledLayout: the popup lays out its row bands
// and label offsets from the same scaled constants, so at 1.5× the point
// the label draws at hits its own row and a point above the popup hits
// nothing — hit test and paint agree in Layout units at every scale.
func TestMenuHitTestMatchesScaledLayout(t *testing.T) {
	uitest.UseScale(t, 1.5)
	env, _ := newTestEnv(t)
	m := env.Menu()
	if m == nil {
		t.Fatal("env has no menu")
	}
	at := image.Point{X: 400, Y: 300}
	m.Open([]MenuEntry{{Label: "Show Info", Bold: true}, MenuSep, {Label: "Warp"}}, at)
	defer m.Close()

	pop := m.Rect()
	if pop.Dx() < theme.Px(menuMinW) {
		t.Errorf("popup width %d, want ≥ scaled menuMinW %d", pop.Dx(), theme.Px(menuMinW))
	}
	row0 := m.RowRect(0)
	if want := TipLineH(env.FaceBold(theme.StepSmall)) + 2*theme.Px(menuRowPadY); row0.Dy() != want {
		t.Errorf("row0 height %d, want the scaled bold line box + 2×%d = %d",
			row0.Dy(), theme.Px(menuRowPadY), want)
	}
	// The label draw anchor lands inside its own row band...
	labelPt := image.Point{X: row0.Min.X + theme.Px(menuRowPadX), Y: row0.Min.Y + theme.Px(menuRowPadY)}
	if i, ok := m.at(labelPt); !ok || i != 0 {
		t.Errorf("hit at the row-0 label anchor %v = (%d,%v), want (0,true)", labelPt, i, ok)
	}
	// ...and one pixel above the popup hits nothing (scaled boundary).
	if _, ok := m.at(image.Point{X: labelPt.X, Y: pop.Min.Y - 1}); ok {
		t.Errorf("hit one px above the popup (y=%d) succeeded, want none", pop.Min.Y-1)
	}
	// The last band still ends exactly one scaled bottom pad above the popup.
	last := m.RowRect(len(m.Entries()) - 1)
	if last.Max.Y != pop.Max.Y-theme.Px(menuPadY) {
		t.Errorf("last row ends %d, popup ends %d (bottom pad %d)",
			last.Max.Y, pop.Max.Y, theme.Px(menuPadY))
	}
}
