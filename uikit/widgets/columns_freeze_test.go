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

// TestColumnGripDragFollowsThePointer is the edge-tracking contract the
// width arithmetic alone never pinned: the strip sits ON the boundary it
// drags, and that boundary moves with the cursor — the column left of it
// grows, the column right of it gives the width back, and no other
// boundary moves. (The old trade partner was the least important column,
// wherever it was: grabbing Distance's right edge moved Name's edge while
// the grabbed line stood still. The last column's strip sat at the ⋯
// button, made-up boundary and all.)
func TestColumnGripDragFollowsThePointer(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl, ui := newColumnTable(t, env, overviewCols())
	tbl.SetRows(overviewRows("0 m/s"))
	settle(t, tbl, ui)
	before := anchorsOf(tbl)
	avail := tbl.FitWidthForTest()

	// The table's right edge is not a column boundary: the last column
	// keeps no strip (the fit owns that edge; there is nothing to drag).
	if r := tbl.grips[len(tbl.cols)-1].GetWidget().Rect; !r.Empty() {
		t.Errorf("last column's strip = %v, want none (its right edge is the table's)", r)
	}

	grip := tbl.grips[1].GetWidget().Rect
	if grip.Empty() || !grip.In(tbl.header.GetWidget().Rect) {
		t.Fatalf("grip 1 rect = %v, want a strip inside the header %v", grip, tbl.header.GetWidget().Rect)
	}
	// The strip ends exactly on the Distance|Kind boundary it drags.
	if edge := tbl.hdrBtn[2].GetWidget().Rect.Min.X; grip.Max.X != edge {
		t.Fatalf("grip 1 ends at %d, not on the Distance|Kind boundary %d", grip.Max.X, edge)
	}
	// The split rule: a press in the strip belongs to the grip's own
	// (blocking) layer — the grips are added after the sort row, so theirs
	// sits above the button tiling underneath it.
	pt := grip.Min.Add(image.Pt(grip.Dx()/2, grip.Dy()/2))
	gl := tbl.grips[1].GetWidget().EffectiveInputLayer()
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
	if tbl.grip != 1 {
		t.Fatalf("press on grip 1 grabbed %d", tbl.grip)
	}
	// +20 stays inside Kind's floor slack, so the trade is unbounded by it.
	tbl.Step(pt.Add(image.Pt(20, 0)), false, true)
	tbl.Step(pt.Add(image.Pt(20, 0)), false, false)
	settle(t, tbl, ui)
	after := anchorsOf(tbl)

	if tbl.grip != -1 {
		t.Errorf("grip still grabbed after the release")
	}
	if tbl.sortKey != "" {
		t.Errorf("sort key = %q after a grip drag, want none", tbl.sortKey)
	}
	// (a) The grabbed edge follows the pointer — on screen, header and
	// body alike, by exactly the drag distance.
	if got, want := after.hdr[2].Min.X, before.hdr[2].Min.X+20; got != want {
		t.Errorf("Distance|Kind boundary at %d, want %d (the grabbed edge must follow the pointer)", got, want)
	}
	if got, want := after.cells[2].Min.X, before.cells[2].Min.X+20; got != want {
		t.Errorf("row cell boundary at %d, want %d (body must track the header)", got, want)
	}
	if got, want := tbl.grips[1].GetWidget().Rect.Max.X, grip.Max.X+20; got != want {
		t.Errorf("the grabbed strip ends at %d, want %d", got, want)
	}
	// (b) The column left of the edge grew, the one right of it traded the
	// delta away.
	if got, want := after.widths[1], before.widths[1]+20; math.Abs(got-want) > 1 {
		t.Errorf("Distance = %v, want %v (20 px wider)", got, want)
	}
	if got, want := after.widths[2], before.widths[2]-20; math.Abs(got-want) > 1 {
		t.Errorf("Kind = %v, want %v (20 px narrower, the trade partner)", got, want)
	}
	// (c) No other boundary moved.
	if after.hdr[0] != before.hdr[0] {
		t.Errorf("Name's box %v → %v after dragging Distance's edge", before.hdr[0], after.hdr[0])
	}
	if after.hdr[3] != before.hdr[3] {
		t.Errorf("Velocity's box %v → %v after dragging Distance's edge", before.hdr[3], after.hdr[3])
	}
	if after.widths[0] != before.widths[0] || after.widths[3] != before.widths[3] {
		t.Errorf("untouched widths %v/%v → %v/%v",
			before.widths[0], before.widths[3], after.widths[0], after.widths[3])
	}
	if sum := uitest.SumPrefix(tbl.widths); math.Abs(sum-avail) > 1e-6 {
		t.Errorf("column sum = %v, want the column area %v (still tiling)", sum, avail)
	}

	// The floor: the dragged column stops at its header measure, the
	// partner never drops under its own, and the row keeps tiling.
	grip = tbl.grips[1].GetWidget().Rect
	pt = grip.Min.Add(image.Pt(1, 1))
	tbl.Step(pt, true, true)
	tbl.Step(image.Pt(-4000, pt.Y), false, true)
	tbl.Step(image.Pt(-4000, pt.Y), false, false)
	settle(t, tbl, ui)
	if tbl.base[1].Width < tbl.base[1].Min {
		t.Errorf("dragged width %v under the floor %v", tbl.base[1].Width, tbl.base[1].Min)
	}
	if tbl.base[2].Width < tbl.base[2].Min {
		t.Errorf("partner width %v under its floor %v", tbl.base[2].Width, tbl.base[2].Min)
	}
	if sum := uitest.SumPrefix(tbl.widths); math.Abs(sum-tbl.FitWidthForTest()) > 1e-6 {
		t.Errorf("column sum = %v after the floor clamp, want %v", sum, tbl.FitWidthForTest())
	}
}

// TestColumnGripClaimsTheResizeCursor: hovering a strip advertises the
// split row's EW shape through the frame (one claim per frame; the host's
// Apply writes it once).
func TestColumnGripClaimsTheResizeCursor(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl, ui := newColumnTable(t, env, overviewCols())
	tbl.SetRows(overviewRows("0 m/s"))
	settle(t, tbl, ui)

	grip := tbl.grips[1].GetWidget().Rect
	env.in.x, env.in.y = grip.Min.X+1, grip.Min.Y+1
	env.Pointer().Begin()
	tbl.Poll() // idle hover: the grip claims the frame
	env.Pointer().Apply()
	if env.in.shape != ebiten.CursorShapeEWResize {
		t.Errorf("grip cursor = %v, want EW-resize", env.in.shape)
	}
}

// TestColumnGripIgnoresLayersItDoesNotOwn pins the owner test the columns
// copy was missing: a widget whose layer is gone falls back to ebitenui's
// DefaultLayer, which is active ANYWHERE, so without the guard a press on
// any pixel would grab a column. OwnsAt rejects that fallback, so a stale
// strip is inert — and so is a strip under a covering layer.
func TestColumnGripIgnoresLayersItDoesNotOwn(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl, ui := newColumnTable(t, env, overviewCols())
	tbl.SetRows(overviewRows("0 m/s"))
	settle(t, tbl, ui)
	pt := tbl.grips[1].GetWidget().Rect.Min.Add(image.Pt(1, 1))
	if i := tbl.gripAt(pt); i != 1 {
		t.Fatalf("fixture: grip at %v = %d, want 1", pt, i)
	}

	// The frame's layers are gone (a widget pumped by a frame that no
	// longer exists): DefaultLayer passes anywhere, so only the guard can
	// keep the strip inert.
	input.SetupInputLayersWithDeferred(nil)
	if i := tbl.gripAt(pt); i != -1 {
		t.Errorf("a grip with no layer of its own owned %v (grip %d)", pt, i)
	}
	tbl.Step(pt, true, true)
	if tbl.grip != -1 {
		t.Errorf("a press on a stale grip grabbed column %d", tbl.grip)
	}

	// A covering layer in front of it (another window, the modal menu):
	// the strip is buried, so the press belongs to whatever covers it.
	settle(t, tbl, ui) // this frame's layers
	cover := widget.NewContainer().GetWidget()
	cover.ElevateToNewInputLayer(&input.Layer{
		EventTypes: input.LayerEventTypeAll,
		FullScreen: true,
		BlockLower: true,
		RectFunc:   func() image.Rectangle { return image.Rect(0, 0, 640, 480) },
	})
	tbl.Step(pt, true, true)
	if tbl.grip != -1 {
		t.Errorf("a press on a covered grip grabbed column %d", tbl.grip)
	}

	// Leave no layer behind for the next test's frame.
	input.SetupInputLayersWithDeferred(nil)
}

// TestColumnHeaderLabelStopsAtTheMenu: the ⋯ button owns the header's
// right end and no column's header ink lands in its strip. The label is
// ellipsized into its own column (the sort arrow survives), and the
// columns tile the header up to the strip, so the dots are never
// obscured — the last column's label used to run straight across them.
func TestColumnHeaderLabelStopsAtTheMenu(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl, ui := newColumnTable(t, env, overviewCols())
	// The narrowest the fit allows, then the sort arrow pushes the label
	// past the frozen floor: the label is wider than its column can show.
	tbl.SetPrefs(map[string]ColumnPref{"Velocity": {Width: 1}})
	tbl.SetRows(overviewRows("0 m/s"))
	tbl.SetSort("Velocity", true)
	settle(t, tbl, ui)

	last := len(tbl.cols) - 1
	label := tbl.hdrBtn[last].Text().Label
	budget := tbl.widths[last] - 2*float64(theme.Px(cellPad))
	if raw := tbl.cols[last].Title + tbl.indicator(last); tbl.headMeasure(raw) <= budget {
		t.Fatalf("the %q label (%v) fits its %v column: the pin proves nothing",
			raw, tbl.headMeasure(raw), tbl.widths[last])
	}
	if !strings.Contains(label, "…") || !strings.HasSuffix(label, tbl.indicator(last)) {
		t.Errorf("header label = %q, want an ellipsis that keeps the sort arrow %q", label, tbl.indicator(last))
	}
	if got := tbl.headMeasure(label); got > budget {
		t.Errorf("header label %q measures %v, past its %v budget", label, got, budget)
	}

	// The dots strip is the header's right end; the last column stops
	// before it (header and body alike).
	ui.Update()
	menu := tbl.menuBtn.GetWidget().Rect
	if menu.Empty() {
		t.Fatal("no ⋯ button rect")
	}
	if got := tbl.hdrBtn[last].GetWidget().Rect.Max.X; got > menu.Min.X {
		t.Errorf("the last column's header ends at %d, inside the ⋯ strip %v", got, menu)
	}
	if got := tbl.cellRoot(0, last).Rect.Max.X; got > menu.Min.X {
		t.Errorf("the last column's cells end at %d, inside the ⋯ strip %v", got, menu)
	}
	// Ink scan (geometry: a headless process cannot read pixels back): the
	// label's anchor box is the glyph's own extent — the same box
	// TestHeaderCellAnchorAgreement pins — so a box clear of the strip
	// means no label ink can land in it.
	lbl := tbl.hdrBtn[last].Text().GetWidget().Rect
	if lbl.Empty() {
		t.Fatal("the header label has no box")
	}
	if lbl.Max.X > menu.Min.X {
		t.Errorf("the header label's ink box %v runs into the ⋯ strip %v", lbl, menu)
	}
	// The ellipsize budget is a float and the widget box rounds to the
	// nearest pixel (layoutCells' ellipsize note), so compare against the
	// rounded budget, not its truncation.
	if lbl.Max.X-lbl.Min.X > int(math.Round(budget)) {
		t.Errorf("the header label's box is %d wide, past its %v budget", lbl.Dx(), budget)
	}
	// The grip strips stay clear too: the label is inset by cellPad, the
	// strips are narrower than that inset (a one-line invariant).
	for i := range tbl.cols {
		if g := tbl.grips[i].GetWidget().Rect; !g.Empty() && tbl.hdrBtn[i].Text().GetWidget().Rect.Overlaps(g) {
			t.Errorf("column %d's label %v runs into its grip %v",
				i, tbl.hdrBtn[i].Text().GetWidget().Rect, g)
		}
	}
	// Row cells live in the viewport below the header: no cell can reach a
	// header control.
	hdr := tbl.header.GetWidget().Rect
	for r := range tbl.Rows {
		for i := range tbl.cols {
			if c := tbl.cellRoot(r, i).Rect; !c.Empty() && c.Min.Y < hdr.Max.Y {
				t.Errorf("row %d column %d cell %v rises into the header %v", r, i, c, hdr)
			}
		}
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

// TestColumnMenuToggleAppliesFromTheDotsClick drives the real path: the
// ⋯ button's click opens the menu, the entry runs through the menu's own
// gesture handling and the column is hidden by the end of that frame — no
// window resize, no row change. The entry reopens the menu in place, so
// the same press edge must not run an entry a second time when the frame
// polls again (the host's MenuInput pass, then Kit.Update's): that second
// toggle used to undo the first, which is why the menu "did nothing".
func TestColumnMenuToggleAppliesFromTheDotsClick(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl, ui := newColumnTable(t, env, overviewCols())
	tbl.SetRows(overviewRows("0 m/s"))
	settle(t, tbl, ui)
	m := env.Menu()

	// The ⋯ click, through ebitenui's deferred event (the button's own
	// clicked handler derives the popup anchor from itself).
	btn := tbl.menuBtn.GetWidget()
	btn.MouseButtonClickedEvent.Fire(&widget.WidgetMouseButtonClickedEventArgs{Button: ebiten.MouseButtonLeft})
	ui.Update()
	if !m.IsOpen() {
		t.Fatal("the ⋯ click opened no menu")
	}
	at := m.Rect().Min

	const kind = 2
	row := m.RowRect(kind)
	if row.Empty() {
		t.Fatalf("menu has no row %d (rows: %d)", kind, len(m.Entries()))
	}
	pt := row.Min.Add(image.Pt(row.Dx()/2, row.Dy()/2))
	env.in.x, env.in.y, env.in.edge = pt.X, pt.Y, [3]bool{true, false, false}
	env.Pointer().Begin()

	// One frame, two polls: the host's MenuInput pass and Kit.Update's
	// fallback poll read the same press edge.
	m.Poll()
	m.Poll()
	m.TakeBlocked()

	if !tbl.hidden[kind] {
		t.Fatal("clicking Kind hid nothing: the column is still shown")
	}
	if tbl.widths[kind] != 0 {
		t.Errorf("hidden Kind width = %v, want the display filter's 0", tbl.widths[kind])
	}
	if tbl.hdrBtn[kind].GetWidget().GetVisibility() != widget.Visibility_Hide {
		t.Error("hidden column's header button still draws")
	}
	if r := tbl.cellRoot(0, kind).Rect; !r.Empty() && tbl.cellRoot(0, kind).GetVisibility() != widget.Visibility_Hide {
		t.Errorf("hidden column's cell still draws at %v", r)
	}
	if !m.IsOpen() || m.Rect().Min != at {
		t.Errorf("menu did not reopen at %v (open=%v rect=%v)", at, m.IsOpen(), m.Rect())
	}
	if m.Entries()[kind].Checked {
		t.Error("reopened menu still reports Kind checked")
	}
	if sum := uitest.SumPrefix(tbl.widths); math.Abs(sum-tbl.FitWidthForTest()) > 1e-6 {
		t.Errorf("column sum = %v, want %v (the visible columns retile)", sum, tbl.FitWidthForTest())
	}

	// The latch is one gesture, not a freeze: the next click on the same
	// row shows Kind again.
	env.in.edge = [3]bool{}
	env.Pointer().Begin()
	m.Poll()
	env.in.edge = [3]bool{true, false, false}
	env.Pointer().Begin()
	m.Poll()
	m.TakeBlocked()
	if tbl.hidden[kind] {
		t.Error("the next click did not show Kind again (menu input stuck)")
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
