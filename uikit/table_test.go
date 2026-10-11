package uikit

import (
	"fmt"
	"image/color"
	"math"
	"sort"
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// newTestEnv builds an Env over the embedded content and world-showcase,
// capturing every enqueued command.
func newTestEnv(t *testing.T) (Env, *[]sim.Command) {
	t.Helper()
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	wd, ok := c.World("world-showcase")
	if !ok {
		t.Fatal("world-showcase missing")
	}
	s, err := sim.New(c, wd, 42)
	if err != nil {
		t.Fatalf("sim.New: %v", err)
	}
	var cmds []sim.Command
	return NewEnv(c, s, func(cmd sim.Command) { cmds = append(cmds, cmd) }), &cmds
}

// newPumpedTable builds a standalone table in its own UI and pumps one
// frame so the deferred constructor handlers are registered and the
// widget layout has run at 640×400.
func newPumpedTable(t *testing.T, env Env, cols []Column) (*Table, *ebitenui.UI) {
	t.Helper()
	tbl := NewTable(env, cols)
	ui := &ebitenui.UI{Container: tbl.Container(), PrimaryTheme: env.Theme}
	ui.Update()
	ui.Draw(ebiten.NewImage(640, 400))
	return tbl, ui
}

// clickRow fires a real deferred click on row i and dispatches it on the
// next UI pump (ebitenui queues both handlers and events until Update).
func clickRow(ui *ebitenui.UI, tbl *Table, i int) {
	tbl.rowCts[i].GetWidget().MouseButtonClickedEvent.Fire(&widget.WidgetMouseButtonClickedEventArgs{
		Button: ebiten.MouseButtonLeft,
	})
	ui.Update()
}

// dataIDs extracts the int payloads of rows in display order.
func dataIDs(rows []TableRow) []int {
	out := make([]int, len(rows))
	for i, r := range rows {
		out[i], _ = r.Data.(int)
	}
	return out
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestTableSortAscDescStable covers the default lexicographic sort in
// both directions, including the stability guarantee for equal keys.
func TestTableSortAscDescStable(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl := NewTable(env, []Column{{Key: "A", Title: "A"}, {Key: "B", Title: "B"}})
	base := []TableRow{
		{Cells: []string{"b", "1"}, Data: 0},
		{Cells: []string{"a", "2"}, Data: 1},
		{Cells: []string{"c", "3"}, Data: 2},
		{Cells: []string{"a", "4"}, Data: 3},
	}
	tbl.SetRows(base)
	tbl.SetSort("A", true)
	if got, want := dataIDs(tbl.rows), []int{1, 3, 0, 2}; !equalInts(got, want) {
		t.Errorf("asc order = %v, want %v", got, want)
	}
	tbl.SetSort("A", false)
	if got, want := dataIDs(tbl.rows), []int{2, 0, 1, 3}; !equalInts(got, want) {
		t.Errorf("desc order = %v, want %v (ties keep their input order)", got, want)
	}
	// The indicator suffix follows the active sort.
	if got := tbl.indicator(0); got != " ↓" {
		t.Errorf("active header indicator = %q, want %q", got, " ↓")
	}
	if got := tbl.indicator(1); got != "" {
		t.Errorf("inactive header indicator = %q, want empty", got)
	}
}

// TestTableSortFunc: a custom sort function overrides the default
// lexicographic order (numeric instead of string comparison).
func TestTableSortFunc(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl := NewTable(env, []Column{{Key: "N", Title: "N"}})
	tbl.SetRows([]TableRow{
		{Cells: []string{"10"}, Data: 10},
		{Cells: []string{"9"}, Data: 9},
		{Cells: []string{"8"}, Data: 8},
	})

	tbl.SetSort("N", true)
	if got, want := dataIDs(tbl.rows), []int{10, 8, 9}; !equalInts(got, want) {
		t.Errorf("string order = %v, want %v", got, want)
	}

	tbl.SetSortFunc(func(key string, asc bool, rows []TableRow) {
		sort.SliceStable(rows, func(i, j int) bool {
			if asc {
				return rows[i].Data.(int) < rows[j].Data.(int)
			}
			return rows[j].Data.(int) < rows[i].Data.(int)
		})
	})
	tbl.SetSort("N", true)
	if got, want := dataIDs(tbl.rows), []int{8, 9, 10}; !equalInts(got, want) {
		t.Errorf("numeric order = %v, want %v", got, want)
	}
}

// TestTableSelectVsActivate drives the real deferred-event path: a
// click selects, a repeat click on the same row inside the window also
// activates, a different row does not.
func TestTableSelectVsActivate(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl := NewTable(env, []Column{{Key: "A", Title: "A"}})
	var selected, activated []int
	tbl.OnSelect(func(r TableRow) { selected = append(selected, r.Data.(int)) })
	tbl.OnActivate(func(r TableRow) { activated = append(activated, r.Data.(int)) })
	tbl.SetRows([]TableRow{
		{Cells: []string{"alpha"}, Data: 0},
		{Cells: []string{"bravo"}, Data: 1},
		{Cells: []string{"charlie"}, Data: 2},
	})

	ui := &ebitenui.UI{Container: tbl.Container(), PrimaryTheme: env.Theme}
	ui.Update() // registers the deferred constructor handlers

	clickRow(ui, tbl, 1)
	if !equalInts(selected, []int{1}) || len(activated) != 0 {
		t.Fatalf("first click: selected=%v activated=%v, want [1] []", selected, activated)
	}
	if tbl.selIdx != 1 {
		t.Errorf("selIdx = %d, want 1", tbl.selIdx)
	}

	clickRow(ui, tbl, 1)
	if !equalInts(selected, []int{1, 1}) || !equalInts(activated, []int{1}) {
		t.Fatalf("repeat click: selected=%v activated=%v, want [1 1] [1]", selected, activated)
	}

	clickRow(ui, tbl, 2)
	if !equalInts(selected, []int{1, 1, 2}) || !equalInts(activated, []int{1}) {
		t.Fatalf("other row: selected=%v activated=%v, want [1 1 2] [1]", selected, activated)
	}
}

// TestTableSetRowsKeepsSelection: SetRows updates widgets in place when
// the row count is unchanged, keeps the selection by Data identity
// across reorders, and clears it when the row is gone.
func TestTableSetRowsKeepsSelection(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl := NewTable(env, []Column{{Key: "A", Title: "A"}, {Key: "B", Title: "B"}})
	rows := func() []TableRow {
		return []TableRow{
			{Cells: []string{"x", "1"}, Data: "x"},
			{Cells: []string{"y", "2"}, Data: "y"},
			{Cells: []string{"z", "3"}, Data: "z"},
		}
	}
	tbl.SetRows(rows())
	tbl.rowClicked(1) // select "y"
	if tbl.selIdx != 1 {
		t.Fatalf("selIdx = %d, want 1", tbl.selIdx)
	}
	ctr0 := tbl.rowCts[0]

	// Same count: widgets are reused (diff-friendly) and labels refresh.
	updated := rows()
	updated[0].Cells[0] = "xx"
	tbl.SetRows(updated)
	if tbl.rowCts[0] != ctr0 {
		t.Error("row widgets rebuilt despite an unchanged row count")
	}
	if got := tbl.texts[0][0].Label; got != "xx" {
		t.Errorf("updated cell label = %q, want %q", got, "xx")
	}
	if tbl.selIdx != 1 || tbl.rows[tbl.selIdx].Data != "y" {
		t.Errorf("selection moved to %d (%v), want the y row", tbl.selIdx, tbl.rows[tbl.selIdx].Data)
	}

	// Reordered: the selection follows its Data to the new index.
	tbl.SetRows([]TableRow{rows()[2], rows()[0], rows()[1]})
	if tbl.selIdx != 2 || tbl.rows[2].Data != "y" {
		t.Errorf("after reorder selIdx = %d (%v), want 2 (y)", tbl.selIdx, tbl.rows[tbl.selIdx].Data)
	}

	// Gone: the selection clears.
	tbl.SetRows([]TableRow{rows()[0], rows()[2]})
	if tbl.selIdx != -1 {
		t.Errorf("removed row left selIdx = %d, want -1", tbl.selIdx)
	}
}

// TestTableRefitOnResize: the table refits its columns whenever its own
// width changes (window resize/drag), keeping the fit exact.
func TestTableRefitOnResize(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl, ui := newPumpedTable(t, env, []Column{
		{Key: "A", Title: "Name", Priority: 0},
		{Key: "B", Title: "Value", Priority: 1, AlignRight: true},
	})
	tbl.SetRows([]TableRow{{Cells: []string{"alpha", "1"}, Data: 0}})
	tbl.Sync()
	if tbl.fittedFor != 640 {
		t.Fatalf("fittedFor = %v, want 640", tbl.fittedFor)
	}
	if sum := sumPrefix(tbl.widths); math.Abs(sum-640) > 1e-6 {
		t.Fatalf("column sum = %v, want 640", sum)
	}

	// A narrower draw resizes the root; Sync refits to it. All the
	// surplus lives in the lowest-priority column, so that is the one
	// that gives it back.
	oldName := tbl.widths[0]
	ui.Draw(ebiten.NewImage(400, 300))
	tbl.Sync()
	if tbl.fittedFor != 400 {
		t.Fatalf("after resize fittedFor = %v, want 400", tbl.fittedFor)
	}
	if sum := sumPrefix(tbl.widths); math.Abs(sum-400) > 1e-6 {
		t.Fatalf("after resize column sum = %v, want 400", sum)
	}
	if tbl.widths[0] == oldName {
		t.Errorf("Name column width unchanged (%v) after a 240 px shrink", oldName)
	}
}

// TestTableRightAlignAnchors: headers, right-aligned cells and left
// cells share exact column boxes — every row's cell rect for a column
// ends at the same x (fixed anchors, no per-row jitter), and the header
// button tiles the same box.
func TestTableRightAlignAnchors(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl, ui := newPumpedTable(t, env, []Column{
		{Key: "Name", Title: "Name", Priority: 0},
		{Key: "Dist", Title: "Dist", Priority: 1, AlignRight: true},
		{Key: "Vel", Title: "Vel", Priority: 2, AlignRight: true},
	})
	tbl.SetRows([]TableRow{
		{Cells: []string{"a", "1,000 m", "100 m/s"}, Data: 0},
		{Cells: []string{"a very long entity name", "10 m", "1 m/s"}, Data: 1},
		{Cells: []string{"b", "2 m", "2 m/s"}, Data: 2},
	})
	tbl.Sync()
	ui.Draw(ebiten.NewImage(640, 400)) // relayout with the new cells

	for i, col := range tbl.cols {
		// Header button and every cell occupy the same column box (the
		// cell root: an icon cell's container, else the text itself).
		hdr := tbl.hdrBtn[i].GetWidget().Rect
		first := tbl.cellRoot(0, i).Rect
		if hdr.Dx() != first.Dx() || hdr.Max.X != first.Max.X {
			t.Errorf("column %s: header box %v != cell box %v", col.Title, hdr, first)
		}
		for r := 1; r < len(tbl.rows); r++ {
			rect := tbl.cellRoot(r, i).Rect
			if rect.Dx() != first.Dx() || rect.Max.X != first.Max.X {
				t.Errorf("column %s: row %d box %v, want anchor %v",
					col.Title, r, rect, first)
			}
		}
		// The fitted width is what the widget was pinned to.
		want := tbl.hdrBtn[i].GetWidget().MinWidth
		if tbl.cellRoot(0, i).MinWidth != want {
			t.Errorf("column %s: cell MinWidth %d != header %d",
				col.Title, tbl.cellRoot(0, i).MinWidth, want)
		}
	}
}

// TestTableTintAssignment: row backgrounds follow TableRow.Tint and the
// selected row switches to the accent tint.
func TestTableTintAssignment(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl := NewTable(env, []Column{{Key: "A", Title: "A"}})
	red := color.RGBA{255, 0, 0, 60}
	blue := color.RGBA{0, 0, 255, 80}
	tbl.SetRows([]TableRow{
		{Cells: []string{"a"}, Tint: red, Data: 0},
		{Cells: []string{"b"}, Data: 1},
		{Cells: []string{"c"}, Tint: blue, Data: 2},
	})
	want := []color.RGBA{red, {}, blue}
	for i, w := range want {
		if tbl.applied[i] != w {
			t.Errorf("row %d background = %v, want %v", i, tbl.applied[i], w)
		}
	}

	tbl.rowClicked(0)
	accent := withA(env.Pal.Accent, selectionAlpha)
	if tbl.applied[0] != accent {
		t.Errorf("selected row background = %v, want accent %v", tbl.applied[0], accent)
	}
	if tbl.applied[1] != want[1] || tbl.applied[2] != want[2] {
		t.Errorf("selection repainted neighbours: %v %v", tbl.applied[1], tbl.applied[2])
	}
}

// TestSelectDataExternal pins external selection (world clicks): the row
// highlight follows SelectData and clears on nil.
func TestSelectDataExternal(t *testing.T) {
	env, _ := newChromeEnv(t)
	tbl := NewTable(env, []Column{{Key: "Name", Title: "Name", Width: 100}})
	tbl.SetRows([]TableRow{
		{Cells: []string{"Alpha"}, Data: "alpha"},
		{Cells: []string{"Beta"}, Data: "beta"},
	})
	tbl.SelectData("beta")
	if tbl.selIdx != 1 || tbl.selData != "beta" {
		t.Fatalf("SelectData(beta): idx=%d data=%v, want 1/beta", tbl.selIdx, tbl.selData)
	}
	tbl.SelectData(nil)
	if tbl.selIdx != -1 || tbl.selData != nil {
		t.Fatalf("SelectData(nil): idx=%d data=%v, want -1/nil", tbl.selIdx, tbl.selData)
	}
}

// fireWheel sends one wheel notch through the deferred event queue
// (positive dy = wheel rolled towards the user = wheel up).
func fireWheel(ui *ebitenui.UI, tbl *Table, dy float64) {
	tbl.scroll.GetWidget().ScrolledEvent.Fire(&widget.WidgetScrolledEventArgs{Y: dy})
	ui.Update()
}

// TestTableWheelScrollDirection pins the wheel convention: a positive
// notch (wheel up) decreases ScrollTop towards the top of the list, a
// negative notch increases it, each step is exactly wheelRows rows of the
// measured overflow, and the offset stays clamped to 0..1.
func TestTableWheelScrollDirection(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl, ui := newPumpedTable(t, env, []Column{{Key: "A", Title: "A"}})
	rows := make([]TableRow, 40)
	for i := range rows {
		rows[i] = TableRow{Cells: []string{fmt.Sprintf("row %d", i)}, Data: i}
	}
	tbl.SetRows(rows)
	ui.Update()
	ui.Draw(ebiten.NewImage(640, 400))

	over := tbl.scrollOverflow()
	if over <= 0 {
		t.Fatalf("overflow = %v, want > 0 (40 rows must overflow the viewport)", over)
	}
	step := wheelRows * tbl.rowH / over

	// Wheel up from mid-scroll: ScrollTop decreases by exactly one step.
	tbl.scroll.ScrollTop = 0.5
	fireWheel(ui, tbl, 1)
	if got := tbl.scroll.ScrollTop; math.Abs(got-(0.5-step)) > 1e-9 {
		t.Errorf("wheel up: ScrollTop = %v, want %v (0.5 - %v)", got, 0.5-step, step)
	}
	// Repeated wheel up stays monotonic (never climbs back).
	for i := 0; i < 3; i++ {
		before := tbl.scroll.ScrollTop
		fireWheel(ui, tbl, 1)
		if after := tbl.scroll.ScrollTop; after >= before {
			t.Fatalf("wheel up step %d: ScrollTop %v -> %v, want strictly less", i, before, after)
		}
	}
	// Wheel down reverses it: ScrollTop increases by the same step.
	before := tbl.scroll.ScrollTop
	fireWheel(ui, tbl, -1)
	if got := tbl.scroll.ScrollTop; math.Abs(got-(before+step)) > 1e-9 {
		t.Errorf("wheel down: ScrollTop = %v, want %v (%v + %v)", got, before+step, before, step)
	}

	// Clamping: never below 0, never above 1.
	tbl.scroll.ScrollTop = 0
	fireWheel(ui, tbl, 1)
	if got := tbl.scroll.ScrollTop; got != 0 {
		t.Errorf("wheel up at top: ScrollTop = %v, want 0", got)
	}
	tbl.scroll.ScrollTop = 1
	fireWheel(ui, tbl, -1)
	if got := tbl.scroll.ScrollTop; got != 1 {
		t.Errorf("wheel down at bottom: ScrollTop = %v, want 1", got)
	}
}

// TestTableRowsTopAnchored: rows start at the top of the viewport and a
// leftover offset is dropped when the rows shrink back into view — the
// list must never stay bottom-anchored.
func TestTableRowsTopAnchored(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl, ui := newPumpedTable(t, env, []Column{{Key: "A", Title: "A"}})
	rows := func(n int) []TableRow {
		out := make([]TableRow, n)
		for i := range out {
			out[i] = TableRow{Cells: []string{fmt.Sprintf("row %d", i)}, Data: i}
		}
		return out
	}
	pump := func() {
		ui.Update()
		ui.Draw(ebiten.NewImage(640, 400))
	}

	tbl.SetRows(rows(40))
	pump()
	if over := tbl.scrollOverflow(); over <= 0 {
		t.Fatalf("overflow = %v, want > 0 for 40 rows", over)
	}
	view := tbl.scroll.ViewRect()

	// Scrolled to the bottom: row 0 is pushed up out of the viewport.
	tbl.scroll.ScrollTop = 1
	pump()
	if r0 := tbl.rowCts[0].GetWidget().Rect; r0.Max.Y > view.Min.Y {
		t.Fatalf("bottom-scrolled fixture: row 0 = %v, want above view %v", r0, view)
	}

	// Rows shrink to two (they fit): the offset clears immediately and
	// the rows re-anchor to the top of the viewport.
	tbl.SetRows(rows(2))
	pump()
	if tbl.scroll.ScrollTop != 0 {
		t.Errorf("ScrollTop after shrink = %v, want 0", tbl.scroll.ScrollTop)
	}
	if r0 := tbl.rowCts[0].GetWidget().Rect; r0.Min.Y != view.Min.Y {
		t.Errorf("row 0 = %v, want top-anchored at view %v", r0, view)
	}
	r1 := tbl.rowCts[1].GetWidget().Rect
	if r1.Min.Y < tbl.rowCts[0].GetWidget().Rect.Max.Y {
		t.Errorf("row 1 %v overlaps row 0 %v", r1, tbl.rowCts[0].GetWidget().Rect)
	}
	if r1.Max.Y > view.Max.Y {
		t.Errorf("row 1 = %v, exceeds view %v", r1, view)
	}

	// Rows that fit ignore the wheel: the offset can never go stale.
	fireWheel(ui, tbl, -3)
	fireWheel(ui, tbl, -3)
	if tbl.scroll.ScrollTop != 0 {
		t.Errorf("ScrollTop after wheel on a fitted list = %v, want 0", tbl.scroll.ScrollTop)
	}
}

// TestIconCellLayoutAtDesignSizes pins the icon-cell geometry at both
// design sizes and at the minimum window: the glyph box is the inline
// square, the box and the label never overlap, both sit inside the column
// box, the cell keeps the header's anchor, and right-aligned columns never
// grow an icon box (their numeric anchors stay exact).
func TestIconCellLayoutAtDesignSizes(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl, ui := newPumpedTable(t, env, []Column{
		{Key: "Name", Title: "Name", Priority: 0},
		{Key: "Kind", Title: "Kind", Priority: 1},
		{Key: "Dist", Title: "Dist", Priority: 2, AlignRight: true},
	})
	rows := []TableRow{
		{Cells: []string{"Rock 1", "Asteroid", "1,200 m"},
			Icons: []string{"", "kind_asteroid", ""}, Data: 0},
		{Cells: []string{"A very long entity name indeed", "Ship", "10 m"},
			Icons: []string{"", "kind_ship", ""}, Data: 1},
		{Cells: []string{"Port", "Station", "2 m"},
			Icons: []string{"", "kind_station", ""}, Data: 2},
	}
	for _, sz := range []struct {
		tag  string
		w, h int
	}{
		{"1280x720", 1280, 720},
		{"1024x700", 1024, 700},
		{"min-window", 320, 240},
	} {
		tbl.SetRows(rows)
		tbl.Sync()
		ui.Draw(ebiten.NewImage(sz.w, sz.h))
		tbl.Sync()
		ui.Draw(ebiten.NewImage(sz.w, sz.h))

		hdr := tbl.hdrBtn[1].GetWidget().Rect
		for r := range tbl.rows {
			cell := tbl.cellRoot(r, 1)
			icon := tbl.cells[r][1].box.Widget().GetWidget().Rect
			label := tbl.texts[r][1].GetWidget().Rect

			if icon.Dx() != IconInline || icon.Dy() != IconInline {
				t.Errorf("%s: row %d icon box = %v, want %d×%d",
					sz.tag, r, icon, IconInline, IconInline)
			}
			if icon.Overlaps(label) {
				t.Errorf("%s: row %d icon %v overlaps the label %v", sz.tag, r, icon, label)
			}
			if icon.Max.X > label.Min.X {
				t.Errorf("%s: row %d icon ends at x=%d, label starts at %d",
					sz.tag, r, icon.Max.X, label.Min.X)
			}
			if !icon.In(cell.Rect) || !label.In(cell.Rect) {
				t.Errorf("%s: row %d icon %v / label %v escape the cell %v",
					sz.tag, r, icon, label, cell.Rect)
			}
			// The cell keeps the header's column anchor.
			if cell.Rect.Dx() != hdr.Dx() || cell.Rect.Max.X != hdr.Max.X {
				t.Errorf("%s: row %d cell %v, want the header box %v",
					sz.tag, r, cell.Rect, hdr)
			}
			// Right-aligned columns carry no icon cell at all.
			if tbl.cells[r][2] != nil {
				t.Errorf("%s: row %d right-aligned column grew an icon cell", sz.tag, r)
			}
		}
	}
}
