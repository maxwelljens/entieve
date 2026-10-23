package widgets

import (
	"image"
	"math"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// The column freeze, the drag grips and the ⋯ menu (columns.go). The
// pre-freeze behaviour these pins replace: applyWidths re-measured every
// column over every row, so one cell growing — the overview's Velocity
// mid-warp — widened its column, and fitColumns took the width back from
// Name, shifting every anchor after it. The sort arrow was part of the
// measure too, so a sort click moved columns.

// newColumnTable mounts a column table in a window (the grips' input-layer
// gate needs one, the split seam's harness) and pumps the deferred handlers.
func newColumnTable(t *testing.T, env Seams, cols []Column, opts ...TableOption) (*Table, *ebitenui.UI) {
	t.Helper()
	tbl := NewTable(env, cols, opts...)
	win := widget.NewWindow(
		widget.WindowOpts.Contents(tbl.Container()),
		widget.WindowOpts.TitleBar(
			widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout())),
			theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(200), theme.Px(200)),
		widget.WindowOpts.Location(image.Rect(0, 0, 640, 420)),
	)
	ui := newWindowUI(t, env, win)
	uitest.PumpWindow(t, ui, tbl.Sync, 640, 480)
	return tbl, ui
}

// settle lays the rows out and lets one Sync observe the new table width.
func settle(t *testing.T, tbl *Table, ui *ebitenui.UI) {
	t.Helper()
	tbl.Sync()
	uitest.PumpWindow(t, ui, tbl.Sync, 640, 480)
	tbl.Sync()
}

// colAnchors is what a content change must never move: every header box,
// every fitted width and row 0's cell boxes.
type colAnchors struct {
	hdr    []image.Rectangle
	cells  []image.Rectangle
	widths []float64
}

func anchorsOf(tbl *Table) colAnchors {
	a := colAnchors{widths: append([]float64(nil), tbl.widths...)}
	for i := range tbl.cols {
		a.hdr = append(a.hdr, tbl.hdrBtn[i].GetWidget().Rect)
		a.cells = append(a.cells, tbl.cellRoot(0, i).Rect)
	}
	return a
}

func sameAnchors(t *testing.T, tag string, before, after colAnchors) {
	t.Helper()
	for i := range before.widths {
		if before.widths[i] != after.widths[i] {
			t.Errorf("%s: column %d width %v → %v", tag, i, before.widths[i], after.widths[i])
		}
		if before.hdr[i] != after.hdr[i] {
			t.Errorf("%s: column %d header %v → %v", tag, i, before.hdr[i], after.hdr[i])
		}
		if before.cells[i] != after.cells[i] {
			t.Errorf("%s: column %d cell %v → %v", tag, i, before.cells[i], after.cells[i])
		}
	}
}

// overviewCols is the overview's four columns, the shape the bug hit.
func overviewCols() []Column {
	return []Column{
		{Key: "Name", Title: "Name", Priority: 0},
		{Key: "Distance", Title: "Distance", Priority: 1, AlignRight: true},
		{Key: "Kind", Title: "Kind", Priority: 2},
		{Key: "Velocity", Title: "Velocity", Priority: 3, AlignRight: true},
	}
}

func overviewRows(vel string) []TableRow {
	return []TableRow{
		{Cells: []string{"Veldspar", "1,200 m", "Asteroid", vel}, Data: 0},
		{Cells: []string{"Scordite", "10 m", "Asteroid", "0 m/s"}, Data: 1},
		{Cells: []string{"Rat", "2 m", "Ship", "220 m/s"}, Data: 2},
	}
}

// TestColumnAnchorsSurviveContentChanges is the freeze's contract: a cell
// widening (the old trigger — it grew Velocity's natural width and
// fitColumns took it from Name) changes nothing but the ellipsis, and
// neither does a sort click (the arrow used to be part of the measure).
func TestColumnAnchorsSurviveContentChanges(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl, ui := newColumnTable(t, env, overviewCols())
	tbl.SetRows(overviewRows("0 m/s"))
	settle(t, tbl, ui)
	before := anchorsOf(tbl)

	// The trigger is real: the widest cell now measures past the frozen
	// column, which is exactly what used to move it.
	wide := strings.Repeat("9", 40) + " m/s"
	if got, was := tbl.measure(wide), tbl.base[3].Width; got+2*float64(theme.Px(cellPad)) <= was {
		t.Fatalf("the wide cell measures %v, not past the frozen %v — the pin proves nothing", got, was)
	}

	tbl.SetRows(overviewRows(wide))
	settle(t, tbl, ui)
	sameAnchors(t, "cell grew", before, anchorsOf(tbl))
	if lbl := tbl.Texts[0][3].Label; !strings.HasSuffix(lbl, "…") {
		t.Errorf("wide velocity cell = %q, want an ellipsized label", lbl)
	}

	// A sort click moves the arrow only: the frozen base never re-measures.
	tbl.SetSort("Velocity", false)
	settle(t, tbl, ui)
	sameAnchors(t, "sort click", before, anchorsOf(tbl))
	if lbl := tbl.hdrBtn[3].Text().Label; !strings.HasSuffix(lbl, "↓") {
		t.Errorf("sorted header = %q, want the ↓ indicator", lbl)
	}
}

// TestFirstFitIsTheOldMeasurement pins the other half of the contract: the
// frozen base IS today's fit — every new table opens byte-identical to the
// layout the elastic fit produced. naturalOf restates the pre-freeze
// measurement (header + widest cell, floors at the header) and the fit is
// the shipped policy.
func TestFirstFitIsTheOldMeasurement(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl, ui := newColumnTable(t, env, overviewCols())
	tbl.SetRows(overviewRows("0 m/s"))
	settle(t, tbl, ui)
	if !tbl.frozen {
		t.Fatal("table never froze")
	}

	natural := make([]ColWidth, len(tbl.cols))
	prio := make([]int, len(tbl.cols))
	for i, c := range tbl.cols {
		floor := tbl.headMeasure(c.Title+tbl.indicator(i)) + 2*float64(theme.Px(cellPad))
		w := floor
		for _, row := range tbl.Rows {
			if m := tbl.measure(cellOf(row, i)) + 2*float64(theme.Px(cellPad)); m > w {
				w = m
			}
		}
		natural[i] = ColWidth{Label: c.Title, Width: w, Min: floor}
		prio[i] = c.Priority
	}
	want := fitColumns(natural, prio, growColumn(prio), tbl.FitWidthForTest())
	for i := range want {
		if math.Abs(tbl.widths[i]-want[i].Width) > 1e-9 {
			t.Errorf("column %s = %v, want today's %v", tbl.cols[i].Title, tbl.widths[i], want[i].Width)
		}
	}
}

// TestColumnGripDragResizesOnlyItsColumn drives the real drag: the strip
// owns the press (its own input layer covers the sort button), the delta
// trades with the next column so the row keeps tiling, and no sort fires.
func TestColumnGripDragResizesOnlyItsColumn(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl, ui := newColumnTable(t, env, overviewCols())
	tbl.SetRows(overviewRows("0 m/s"))
	settle(t, tbl, ui)
	before := anchorsOf(tbl)
	avail := tbl.FitWidthForTest()

	// The last column's strip stops at the ⋯ button, which owns the
	// strip's right end.
	grip := tbl.grips[3].GetWidget().Rect
	if grip.Empty() || !grip.In(tbl.header.GetWidget().Rect) {
		t.Fatalf("grip 3 rect = %v, want a strip inside the header %v", grip, tbl.header.GetWidget().Rect)
	}
	if menu := tbl.menuBtn.GetWidget().Rect; grip.Max.X > menu.Min.X {
		t.Errorf("last grip %v runs under the column menu %v", grip, menu)
	}
	// The split rule: a press in the strip belongs to the grip's own
	// (blocking) layer — the grips are added after the sort row, so theirs
	// sits above the button tiling underneath it.
	pt := grip.Min.Add(image.Pt(grip.Dx()/2, grip.Dy()/2))
	gl := tbl.grips[3].GetWidget().EffectiveInputLayer()
	if !gl.ActiveFor(pt.X, pt.Y, input.LayerEventTypeMouseButton) || !gl.BlockLower {
		t.Fatal("the grip's input layer does not block its strip")
	}
	if gl == tbl.header.GetWidget().EffectiveInputLayer() {
		t.Fatal("the grip shares the header's layer instead of owning its strip")
	}
	if kids := tbl.header.Children(); len(kids) != len(tbl.cols)+2 || kids[1] != tbl.grips[0] {
		t.Fatalf("header children = %d, want the sort row then one grip per column then the menu", len(kids))
	}

	tbl.Step(pt, true, true)
	if tbl.grip != 3 {
		t.Fatalf("press on grip 3 grabbed %d", tbl.grip)
	}
	tbl.Step(pt.Add(image.Pt(40, 0)), false, true)
	tbl.Step(pt.Add(image.Pt(40, 0)), false, false)
	settle(t, tbl, ui)

	if tbl.grip != -1 {
		t.Errorf("grip still grabbed after the release")
	}
	if tbl.sortKey != "" {
		t.Errorf("sort key = %q after a grip drag, want none", tbl.sortKey)
	}
	if got, want := tbl.widths[3], before.widths[3]+40; math.Abs(got-want) > 1 {
		t.Errorf("dragged column = %v, want %v (40 px wider)", got, want)
	}
	if got := tbl.widths[0]; math.Abs(got-(before.widths[0]-40)) > 1 {
		t.Errorf("traded column = %v, want %v (40 px narrower)", got, before.widths[0]-40)
	}
	if sum := uitest.SumPrefix(tbl.widths[:len(tbl.cols)]); math.Abs(sum-avail) > 1e-6 {
		t.Errorf("column sum = %v, want the table width %v (still tiling)", sum, avail)
	}

	// The strip never drops below the header floor (the fit's own Min).
	grip = tbl.grips[3].GetWidget().Rect
	tbl.Step(grip.Min.Add(image.Pt(1, 1)), true, true)
	tbl.Step(image.Pt(-1000, grip.Min.Y), false, true)
	tbl.Step(image.Pt(-1000, grip.Min.Y), false, false)
	if tbl.base[3].Width < tbl.base[3].Min {
		t.Errorf("dragged width %v under the floor %v", tbl.base[3].Width, tbl.base[3].Min)
	}
}

// TestColumnGripClaimsTheResizeCursor: hovering a strip advertises the
// split row's EW shape (one ClaimCursor; the kit writes it once per frame).
func TestColumnGripClaimsTheResizeCursor(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl, ui := newColumnTable(t, env, overviewCols())
	tbl.SetRows(overviewRows("0 m/s"))
	settle(t, tbl, ui)

	var shape ebiten.CursorShapeType
	defer SetCursorShapeForTest(func(c ebiten.CursorShapeType) { shape = c })()
	defer ApplyCursor()
	grip := tbl.grips[1].GetWidget().Rect
	tbl.Step(grip.Min.Add(image.Pt(1, 1)), false, false)
	ApplyCursor()
	if shape != ebiten.CursorShapeEWResize {
		t.Errorf("grip cursor = %v, want EW-resize", shape)
	}
}

// TestColumnMenuTogglesVisibility: the ⋯ menu lists one row per column
// with its checked state, a row hides that column by display filter (the
// positional data never changes), reopens the menu at the same point, and
// the entry that would hide the last visible column is disabled.
func TestColumnMenuTogglesVisibility(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl, ui := newColumnTable(t, env, overviewCols())
	tbl.SetRows(overviewRows("0 m/s"))
	settle(t, tbl, ui)
	before := anchorsOf(tbl)

	at := image.Pt(120, 60)
	tbl.openColumnMenu(at)
	entries := env.Menu().Entries()
	if len(entries) != len(tbl.cols) {
		t.Fatalf("menu rows = %d, want one per column (%d)", len(entries), len(tbl.cols))
	}
	for i, e := range entries {
		if !e.Checked || e.Disabled || e.Label != tbl.cols[i].Title {
			t.Errorf("row %d = %+v, want the checked, enabled %q", i, e, tbl.cols[i].Title)
		}
	}
	if env.Menu().tick == 0 {
		t.Error("a checked menu reserved no tick gutter")
	}

	// Hide Kind (column 2): filtered out, indices intact, menu reopened.
	entries[2].Action()
	if !env.Menu().IsOpen() || env.Menu().Rect().Min != at {
		t.Fatalf("menu did not reopen at %v (open=%v rect=%v)", at, env.Menu().IsOpen(), env.Menu().Rect())
	}
	if !tbl.hidden[2] || tbl.widths[2] != 0 {
		t.Errorf("hidden column: hidden=%v width=%v", tbl.hidden[2], tbl.widths[2])
	}
	if tbl.hdrBtn[2].GetWidget().GetVisibility() != widget.Visibility_Hide {
		t.Error("hidden column's header button still draws")
	}
	if r := tbl.cellRoot(0, 2).Rect; !r.Empty() && tbl.cellRoot(0, 2).GetVisibility() != widget.Visibility_Hide {
		t.Errorf("hidden column's cell still draws at %v", r)
	}
	for _, row := range tbl.Rows {
		if len(row.Cells) != len(tbl.cols) {
			t.Fatalf("row cells = %d, want the positional %d", len(row.Cells), len(tbl.cols))
		}
	}
	if got := tbl.Texts[0][2].Label; got == "" {
		t.Error("hidden column lost its cell text (display filter must not touch the data)")
	}
	if sum := uitest.SumPrefix(tbl.widths); math.Abs(sum-(tbl.FitWidthForTest())) > 1e-6 {
		t.Errorf("column sum = %v, want the table width %v (visible columns retile)", sum, tbl.FitWidthForTest())
	}

	// Re-showing it restores the exact frozen widths.
	if e := env.Menu().Entries()[2]; e.Checked {
		t.Errorf("reopened menu still reports Kind checked")
	} else {
		e.Action()
	}
	settle(t, tbl, ui)
	sameAnchors(t, "column shown again", before, anchorsOf(tbl))
	if tbl.hidden[2] {
		t.Error("Kind stayed hidden after the second toggle")
	}

	// Never the last one: with one column left, that row only reports.
	tbl.setColumnVisible(0, false)
	tbl.setColumnVisible(1, false)
	tbl.setColumnVisible(3, false)
	if n := tbl.visibleCols(); n != 1 {
		t.Fatalf("visible columns = %d, want the last one kept", n)
	}
	last := 2
	tbl.openColumnMenu(at)
	e := env.Menu().Entries()[last]
	if !e.Checked || !e.Disabled {
		t.Errorf("last visible column's row = %+v, want checked and disabled", e)
	}
	e.Action()
	if tbl.hidden[last] {
		t.Error("the last visible column was hidden")
	}
}

// TestColumnPrefsRoundTrip: the frozen base and the hidden flags are
// persisted in DESIGN units, so one stored set restores at another scale.
func TestColumnPrefsRoundTrip(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl, ui := newColumnTable(t, env, overviewCols(), PrefsID("t"))
	tbl.SetPrefs(map[string]ColumnPref{
		"Name":     {Width: 120},
		"Velocity": {Off: true},
	})
	tbl.SetRows(overviewRows("0 m/s"))
	settle(t, tbl, ui)

	prefs := tbl.Prefs()
	if got := prefs["Name"].Width; math.Abs(got-120) > 1e-6 {
		t.Errorf("stored Name width = %v, want the 120 design px", got)
	}
	if !prefs["Velocity"].Off {
		t.Error("stored prefs lost the hidden column")
	}
	if !tbl.hidden[3] {
		t.Error("Velocity hidden flag not applied")
	}
	if got := tbl.base[0].Width; math.Abs(got-120) > 1e-6 {
		t.Errorf("frozen Name = %v, want the stored 120", got)
	}

	// Scale 1.5: the same design width lands at 1.5× px and reads back as
	// the stored design number.
	uitest.UseScale(t, 1.5)
	env2, _ := newTestEnv(t)
	tbl2, ui2 := newColumnTable(t, env2, overviewCols(), PrefsID("t"))
	tbl2.SetPrefs(map[string]ColumnPref{"Name": {Width: 120}})
	tbl2.SetRows(overviewRows("0 m/s"))
	settle(t, tbl2, ui2)
	if got := tbl2.base[0].Width; math.Abs(got-180) > 1 {
		t.Errorf("scaled frozen Name = %v, want 180 px (120 design × 1.5)", got)
	}
	if got := tbl2.Prefs()["Name"].Width; math.Abs(got-120) > 1 {
		t.Errorf("scaled round-trip = %v, want the 120 design px back", got)
	}

	// ResetPrefs drops both: the next fit measures the content again and
	// shows every column.
	tbl2.ResetPrefs()
	if got := tbl2.Prefs()["Name"].Width; math.Abs(got-180) < 1 {
		t.Errorf("ResetPrefs kept the stored width (%v)", got)
	}
	for i, h := range tbl2.hidden {
		if h {
			t.Errorf("ResetPrefs left column %d hidden", i)
		}
	}
}
