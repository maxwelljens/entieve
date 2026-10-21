package widgets

import (
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"image"
	"testing"

	"github.com/ebitenui/ebitenui"
)

// Installer behaviour tests for the RowSpec seam (rowspec.go): the menu
// shape is derived once from a spec, the tooltip attaches lazily from
// spec.Tip, and the double-click fires Show Info — strictly, nothing
// else. Surface tests assert their rows' DECLARED DATA against these
// rules instead of re-testing the wiring.

// TestRowSpecTooltipLazy pins the tooltip half: spec.Tip attaches to the
// row widget on the first SetRows (one build), survives an identical
// SetRows without rebuilding, rebuilds only when its change key moves,
// and detaches when the row declares no Tip.
func TestRowSpecTooltipLazy(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl := NewTable(env, []Column{{Key: "A", Title: "A"}})
	ui := &ebitenui.UI{Container: tbl.Container(), PrimaryTheme: env.WidgetTheme()}
	ui.Update()

	builds := 0
	build := func(title string) func() TooltipModel {
		return func() TooltipModel {
			builds++
			return TooltipModel{Title: title}
		}
	}
	rows := func(key, title string) []TableRow {
		return []TableRow{{
			Cells: []string{"a"},
			Spec:  RowSpec{Tip: TipData{Key: key, Build: build(title)}},
		}}
	}

	tbl.SetRows(rows("k1", "one"))
	if builds != 1 {
		t.Fatalf("first SetRows built the panel %d times, want 1 (lazy attach)", builds)
	}
	if tips := tbl.RowCts[0].GetWidget().ToolTips; len(tips) != 1 {
		t.Fatalf("row carries %d tooltips, want 1", len(tips))
	}
	if got := tbl.Tips[0].View.Model().Title; got != "one" {
		t.Fatalf("panel title = %q, want one", got)
	}

	// Identical spec: no rebuild.
	tbl.SetRows(rows("k1", "one"))
	if builds != 1 {
		t.Errorf("unchanged key rebuilt the panel (%d builds), want 1", builds)
	}

	// Key moved: exactly one rebuild, current content.
	tbl.SetRows(rows("k2", "two"))
	if builds != 2 {
		t.Errorf("moved key built %d panels, want 2", builds)
	}
	if got := tbl.Tips[0].View.Model().Title; got != "two" {
		t.Errorf("panel title after the key moved = %q, want two", got)
	}

	// No Tip declared: the attachment drops.
	tbl.SetRows([]TableRow{{Cells: []string{"a"}}})
	if tips := tbl.RowCts[0].GetWidget().ToolTips; len(tips) != 0 {
		t.Errorf("row keeps %d tooltips after declaring no Tip, want 0", len(tips))
	}
}

// TestRowSpecMenuAtScale drives the installer's menu at scale 1.5: the
// derived entries lay their bands out in scaled units and the hit test
// agrees with the paint — the same contract TestMenuHitTestMatchesScaledLayout
// pins for a hand-built menu, now through OpenRowMenu.
func TestRowSpecMenuAtScale(t *testing.T) {
	uitest.UseScale(t, 1.5)
	env, _ := newTestEnv(t)
	ref := ItemRef("ammo-emp-s")
	spec := RowSpec{
		Ref:     &ref,
		Actions: []RowAction{{Label: "Buy", Disabled: true}, {Label: "Sell"}},
	}
	at := image.Point{X: 400, Y: 300}
	env.OpenRowMenu(spec, at)
	defer env.Menu().Close()

	m := env.Menu()
	if !m.IsOpen() {
		t.Fatal("installer opened no menu")
	}
	if got := len(m.Entries()); got != 4 {
		t.Fatalf("entries at 1.5 = %d, want 4 (Show Info / sep / Buy / Sell)", got)
	}
	pop := m.Rect()
	if pop.Min != at {
		t.Errorf("popup origin = %v, want the click point %v", pop.Min, at)
	}
	if pop.Dx() < theme.Px(menuMinW) {
		t.Errorf("popup width %d, want ≥ scaled menuMinW %d", pop.Dx(), theme.Px(menuMinW))
	}
	row0 := m.RowRect(0)
	if want := TipLineH(env.FaceBold(theme.StepSmall)) + 2*theme.Px(menuRowPadY); row0.Dy() != want {
		t.Errorf("row0 height %d, want the scaled bold line box + 2×%d = %d",
			row0.Dy(), theme.Px(menuRowPadY), want)
	}
	labelPt := image.Point{X: row0.Min.X + theme.Px(menuRowPadX), Y: row0.Min.Y + theme.Px(menuRowPadY)}
	if i, ok := m.at(labelPt); !ok || i != 0 {
		t.Errorf("hit at the row-0 label anchor %v = (%d,%v), want (0,true)", labelPt, i, ok)
	}
	if _, ok := m.at(image.Point{X: labelPt.X, Y: pop.Min.Y - 1}); ok {
		t.Errorf("hit one px above the popup (y=%d) succeeded, want none", pop.Min.Y-1)
	}
	last := m.RowRect(len(m.Entries()) - 1)
	if last.Max.Y != pop.Max.Y-theme.Px(menuPadY) {
		t.Errorf("last row ends %d, popup ends %d (bottom pad %d)",
			last.Max.Y, pop.Max.Y, theme.Px(menuPadY))
	}
	// The rows still tile (disjoint, inside) at the scaled size.
	for i := range m.Entries() {
		rr := m.RowRect(i)
		if rr.Dy() <= 0 || !rr.In(pop) {
			t.Errorf("row %d band %v not a positive rect inside %v", i, rr, pop)
		}
		if i > 0 && rr.Overlaps(m.RowRect(i-1)) {
			t.Errorf("rows %d and %d overlap: %v / %v", i-1, i, m.RowRect(i-1), rr)
		}
	}
}

// TestRowDoubleGesture pins the override half of the double-click rule: a
// surface whose repeat click means its own verb (the fitting hangar's Fit)
// fires that verb, never Show Info, and a click on a different ref only
// re-arms the timer.
func TestRowDoubleGesture(t *testing.T) {
	env, _ := newTestEnv(t)
	var intents []Intent
	var d RowDouble
	fired := 0
	mod, ore := ItemRef("ammo-emp-s"), ItemRef("ore-sterngrit")
	if !InfoRefExists(env, mod) || !InfoRefExists(env, ore) {
		t.Fatal("fixture: refs missing from the showcase content")
	}

	// First click: arms only.
	if d.Gesture(env, RowSpec{Ref: &mod}, func() { fired++ }) {
		t.Fatal("a lone click completed the gesture")
	}
	if fired != 0 || len(intents) != 0 {
		t.Fatalf("first click fired %d verbs / %d intents, want 0", fired, len(intents))
	}
	// A different ref re-arms instead of firing.
	if d.Gesture(env, RowSpec{Ref: &ore}, func() { fired++ }) {
		t.Fatal("a click on another row completed the gesture")
	}
	if fired != 0 {
		t.Fatalf("a different ref fired %d verbs, want 0", fired)
	}
	// Back on the first ref, twice: the repeat fires the surface's verb.
	d.Gesture(env, RowSpec{Ref: &mod}, func() { fired++ })
	if !d.Gesture(env, RowSpec{Ref: &mod}, func() { fired++ }) {
		t.Fatal("the repeat click did not complete the gesture")
	}
	if fired != 1 {
		t.Errorf("gesture fired %d verbs, want 1", fired)
	}
	if len(intents) != 0 {
		t.Errorf("gesture emitted %d intents, want none (Show Info is the menu's)", len(intents))
	}
}
