package uikit

import (
	"image"
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/entieve/entieve/sim"
)

// Installer behaviour tests for the RowSpec seam (rowspec.go): the menu
// shape is derived once from a spec, the tooltip attaches lazily from
// spec.Tip, and the double-click fires Show Info — strictly, nothing
// else. Surface tests assert their rows' DECLARED DATA against these
// rules instead of re-testing the wiring.

// TestRowSpecMenuShape pins the derivation: the Show Info group first
// (Ref bold, SecRef as "Show Info — <name>"), one MenuSep between the
// groups, the actions after it — grayed when disabled, never hidden —
// plus the degenerate specs: actions-only opens with no leading
// separator (the contract row), ref-only is the single entry (the
// objective label / Show Info link), and an empty spec opens no menu at
// all. An unresolvable Ref keeps its entry, grayed.
func TestRowSpecMenuShape(t *testing.T) {
	env, cmds := newTestEnv(t)
	ref := ItemRef("charge-emp")
	sec := ItemRef("charge-fusion")
	spec := RowSpec{
		Ref:    &ref,
		SecRef: &sec,
		Actions: []RowAction{
			{Label: "Buy", Intent: Buy{Item: "charge-emp", Qty: 1}},
			{Label: "Sell", Disabled: true, Intent: Sell{Item: "charge-emp", Qty: 1}},
		},
	}

	ents := rowMenuEntries(env, env, spec)
	if len(ents) != 5 {
		t.Fatalf("entries = %+v, want Show Info / secondary / sep / Buy / Sell", ents)
	}
	if ents[0].Label != "Show Info" || !ents[0].Bold || ents[0].Disabled {
		t.Errorf("entry 0 = %+v, want a bold, enabled Show Info", ents[0])
	}
	wantSecond := "Show Info — " + chainName(env, "charge-fusion")
	if ents[1].Label != wantSecond || ents[1].Bold {
		t.Errorf("entry 1 = %+v, want the non-bold secondary %q", ents[1], wantSecond)
	}
	if !ents[2].Sep {
		t.Errorf("entry 2 = %+v, want the separator between the info and action groups", ents[2])
	}
	if ents[3].Label != "Buy" || ents[3].Disabled {
		t.Errorf("entry 3 = %+v, want an enabled Buy", ents[3])
	}
	if ents[4].Label != "Sell" || !ents[4].Disabled {
		t.Errorf("entry 4 = %+v, want Sell grayed (visible, not hidden)", ents[4])
	}

	// Dispatch through the seam: a disabled entry closes without emitting,
	// an enabled one emits its intent — here through the session's own
	// sink, which maps it onto the command the trade button fires.
	env.OpenRowMenu(spec, image.Pt(50, 60))
	if !env.Menu().IsOpen() || env.Menu().Rect().Min != (image.Pt(50, 60)) {
		t.Fatalf("menu open=%v origin=%v, want open at the click point",
			env.Menu().IsOpen(), env.Menu().Rect().Min)
	}
	clickMenuEntry(t, env, "Sell")
	if len(*cmds) != 0 {
		t.Fatalf("disabled Sell emitted %v, want nothing", *cmds)
	}
	env.OpenRowMenu(spec, image.Pt(50, 60))
	clickMenuEntry(t, env, "Buy")
	if len(*cmds) != 1 {
		t.Fatalf("enabled Buy emitted %v, want the single CmdBuy", *cmds)
	}
	if c, ok := (*cmds)[0].(sim.CmdBuy); !ok || c.Item != "charge-emp" || c.Qty != 1 {
		t.Fatalf("cmd = %#v, want CmdBuy{charge-emp 1}", (*cmds)[0])
	}

	// Actions only: the single Accept entry, no leading separator.
	actOnly := rowMenuEntries(env, env, RowSpec{Actions: []RowAction{{Label: "Accept"}}})
	if len(actOnly) != 1 || actOnly[0].Label != "Accept" || actOnly[0].Sep {
		t.Errorf("actions-only = %+v, want the lone Accept entry", actOnly)
	}
	// Ref only: the single bold Show Info entry.
	refOnly := rowMenuEntries(env, env, RowSpec{Ref: ptrRef(ModuleRef("mod-emp-turret"))})
	if len(refOnly) != 1 || refOnly[0].Label != "Show Info" || !refOnly[0].Bold {
		t.Errorf("ref-only = %+v, want the lone bold Show Info", refOnly)
	}
	// Empty spec: no popup at all.
	env.Menu().Close()
	env.OpenRowMenu(RowSpec{}, image.Pt(0, 0))
	if env.Menu().IsOpen() {
		t.Error("an empty spec opened a menu")
	}
	// Unresolvable Ref: the entry stays (grayed), and pressing it opens
	// nothing.
	bad := InfoRef{Kind: InfoItem, ID: "item-nowhere"}
	gone := RowSpec{Ref: &bad}
	entries := rowMenuEntries(env, env, gone)
	if len(entries) != 1 || entries[0].Label != "Show Info" || !entries[0].Bold || !entries[0].Disabled {
		t.Fatalf("unresolvable spec = %+v, want one grayed Show Info", entries)
	}
	env.OpenRowMenu(gone, image.Pt(10, 10))
	clickMenuEntry(t, env, "Show Info")
	if env.InfoWindow().IsOpen() {
		t.Error("a grayed Show Info opened the window")
	}
}

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
	if tips := tbl.rowCts[0].GetWidget().ToolTips; len(tips) != 1 {
		t.Fatalf("row carries %d tooltips, want 1", len(tips))
	}
	if got := tbl.tips[0].view.Model().Title; got != "one" {
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
	if got := tbl.tips[0].view.Model().Title; got != "two" {
		t.Errorf("panel title after the key moved = %q, want two", got)
	}

	// No Tip declared: the attachment drops.
	tbl.SetRows([]TableRow{{Cells: []string{"a"}}})
	if tips := tbl.rowCts[0].GetWidget().ToolTips; len(tips) != 0 {
		t.Errorf("row keeps %d tooltips after declaring no Tip, want 0", len(tips))
	}
}

// TestRowSpecDoubleClickStrict pins the double-click half: one click is
// bookkeeping, a repeat inside the window opens Show Info on the row's
// Ref and fires NOTHING else (no commands — Show Info is the whole
// gesture), and a row whose Ref does not resolve never fires.
func TestRowSpecDoubleClickStrict(t *testing.T) {
	env, cmds := newTestEnv(t)
	ref := ModuleRef("mod-emp-turret")
	bad := InfoRef{Kind: InfoItem, ID: "item-nowhere"}
	tbl := NewTable(env, []Column{{Key: "A", Title: "A"}})
	tbl.SetRows([]TableRow{
		{Cells: []string{"gun"}, Data: 0, Spec: RowSpec{Ref: &ref}},
		{Cells: []string{"ghost"}, Data: 1, Spec: RowSpec{Ref: &bad}},
	})
	ui := &ebitenui.UI{Container: tbl.Container(), PrimaryTheme: env.WidgetTheme()}
	ui.Update()

	clickRow(ui, tbl, 0)
	if env.InfoWindow().IsOpen() {
		t.Fatal("a single click opened Show Info")
	}
	clickRow(ui, tbl, 0)
	if !env.InfoWindow().IsOpen() {
		t.Fatal("repeat click did not open Show Info")
	}
	if got, _ := env.InfoWindow().Ref(); got != ref {
		t.Errorf("Show Info ref = %v, want the row's %v", got, ref)
	}
	if len(*cmds) != 0 {
		t.Fatalf("double-click fired %d commands, want Show Info only", len(*cmds))
	}

	env.InfoWindow().Close()
	clickRow(ui, tbl, 1)
	clickRow(ui, tbl, 1)
	if env.InfoWindow().IsOpen() {
		t.Error("a row with an unresolvable Ref fired Show Info")
	}
	if len(*cmds) != 0 {
		t.Errorf("unresolvable double-click fired %d commands, want 0", len(*cmds))
	}
}

// TestRowSpecMenuAtScale drives the installer's menu at scale 1.5: the
// derived entries lay their bands out in scaled units and the hit test
// agrees with the paint — the same contract TestMenuHitTestMatchesScaledLayout
// pins for a hand-built menu, now through OpenRowMenu.
func TestRowSpecMenuAtScale(t *testing.T) {
	useScale(t, 1.5)
	env, _ := newTestEnv(t)
	ref := ItemRef("charge-emp")
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
	if pop.Dx() < Px(menuMinW) {
		t.Errorf("popup width %d, want ≥ scaled menuMinW %d", pop.Dx(), Px(menuMinW))
	}
	row0 := m.RowRect(0)
	if want := tipLineH(env.FaceBold(StepSmall)) + 2*Px(menuRowPadY); row0.Dy() != want {
		t.Errorf("row0 height %d, want the scaled bold line box + 2×%d = %d",
			row0.Dy(), Px(menuRowPadY), want)
	}
	labelPt := image.Point{X: row0.Min.X + Px(menuRowPadX), Y: row0.Min.Y + Px(menuRowPadY)}
	if i, ok := m.at(labelPt); !ok || i != 0 {
		t.Errorf("hit at the row-0 label anchor %v = (%d,%v), want (0,true)", labelPt, i, ok)
	}
	if _, ok := m.at(image.Point{X: labelPt.X, Y: pop.Min.Y - 1}); ok {
		t.Errorf("hit one px above the popup (y=%d) succeeded, want none", pop.Min.Y-1)
	}
	last := m.RowRect(len(m.Entries()) - 1)
	if last.Max.Y != pop.Max.Y-Px(menuPadY) {
		t.Errorf("last row ends %d, popup ends %d (bottom pad %d)",
			last.Max.Y, pop.Max.Y, Px(menuPadY))
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
