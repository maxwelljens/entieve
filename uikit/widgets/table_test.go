package widgets

import (
	"fmt"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"image/color"
	"math"
	"sort"
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
)

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
	if got, want := dataIDs(tbl.Rows), []int{1, 3, 0, 2}; !equalInts(got, want) {
		t.Errorf("asc order = %v, want %v", got, want)
	}
	tbl.SetSort("A", false)
	if got, want := dataIDs(tbl.Rows), []int{2, 0, 1, 3}; !equalInts(got, want) {
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
	if got, want := dataIDs(tbl.Rows), []int{10, 8, 9}; !equalInts(got, want) {
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
	if got, want := dataIDs(tbl.Rows), []int{8, 9, 10}; !equalInts(got, want) {
		t.Errorf("numeric order = %v, want %v", got, want)
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
	if tbl.SelIdxForTest() != 1 {
		t.Fatalf("selIdx = %d, want 1", tbl.SelIdxForTest())
	}
	ctr0 := tbl.RowCts[0]

	// Same count: widgets are reused (diff-friendly) and labels refresh.
	updated := rows()
	updated[0].Cells[0] = "xx"
	tbl.SetRows(updated)
	if tbl.RowCts[0] != ctr0 {
		t.Error("row widgets rebuilt despite an unchanged row count")
	}
	if got := tbl.Texts[0][0].Label; got != "xx" {
		t.Errorf("updated cell label = %q, want %q", got, "xx")
	}
	if tbl.SelIdxForTest() != 1 || tbl.Rows[tbl.SelIdxForTest()].Data != "y" {
		t.Errorf("selection moved to %d (%v), want the y row", tbl.SelIdxForTest(), tbl.Rows[tbl.SelIdxForTest()].Data)
	}

	// Reordered: the selection follows its Data to the new index.
	tbl.SetRows([]TableRow{rows()[2], rows()[0], rows()[1]})
	if tbl.SelIdxForTest() != 2 || tbl.Rows[2].Data != "y" {
		t.Errorf("after reorder selIdx = %d (%v), want 2 (y)", tbl.SelIdxForTest(), tbl.Rows[tbl.SelIdxForTest()].Data)
	}

	// Gone: the selection clears.
	tbl.SetRows([]TableRow{rows()[0], rows()[2]})
	if tbl.SelIdxForTest() != -1 {
		t.Errorf("removed row left selIdx = %d, want -1", tbl.SelIdxForTest())
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
	if tbl.FitWidthForTest() != 640 {
		t.Fatalf("fittedFor = %v, want 640", tbl.FitWidthForTest())
	}
	if sum := uitest.SumPrefix(tbl.widths); math.Abs(sum-640) > 1e-6 {
		t.Fatalf("column sum = %v, want 640", sum)
	}

	// A narrower draw resizes the root; Sync refits to it. All the
	// surplus lives in the lowest-priority column, so that is the one
	// that gives it back.
	oldName := tbl.widths[0]
	ui.Draw(ebiten.NewImage(400, 300))
	tbl.Sync()
	if tbl.FitWidthForTest() != 400 {
		t.Fatalf("after resize fittedFor = %v, want 400", tbl.FitWidthForTest())
	}
	if sum := uitest.SumPrefix(tbl.widths); math.Abs(sum-400) > 1e-6 {
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
		for r := 1; r < len(tbl.Rows); r++ {
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
	accent := WithA(env.Pal().Accent, selectionAlpha)
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
	if tbl.SelIdxForTest() != 1 || tbl.selData != "beta" {
		t.Fatalf("SelectData(beta): idx=%d data=%v, want 1/beta", tbl.SelIdxForTest(), tbl.selData)
	}
	tbl.SelectData(nil)
	if tbl.SelIdxForTest() != -1 || tbl.selData != nil {
		t.Fatalf("SelectData(nil): idx=%d data=%v, want -1/nil", tbl.SelIdxForTest(), tbl.selData)
	}
}

// fireWheel sends one wheel notch through the deferred event queue
// (positive dy = wheel rolled towards the user = wheel up).
func fireWheel(ui *ebitenui.UI, tbl *Table, dy float64) {
	tbl.ScrollForTest().GetWidget().ScrolledEvent.Fire(&widget.WidgetScrolledEventArgs{Y: dy})
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
	tbl.ScrollForTest().ScrollTop = 0.5
	fireWheel(ui, tbl, 1)
	if got := tbl.ScrollForTest().ScrollTop; math.Abs(got-(0.5-step)) > 1e-9 {
		t.Errorf("wheel up: ScrollTop = %v, want %v (0.5 - %v)", got, 0.5-step, step)
	}
	// Repeated wheel up stays monotonic (never climbs back).
	for i := 0; i < 3; i++ {
		before := tbl.ScrollForTest().ScrollTop
		fireWheel(ui, tbl, 1)
		if after := tbl.ScrollForTest().ScrollTop; after >= before {
			t.Fatalf("wheel up step %d: ScrollTop %v -> %v, want strictly less", i, before, after)
		}
	}
	// Wheel down reverses it: ScrollTop increases by the same step.
	before := tbl.ScrollForTest().ScrollTop
	fireWheel(ui, tbl, -1)
	if got := tbl.ScrollForTest().ScrollTop; math.Abs(got-(before+step)) > 1e-9 {
		t.Errorf("wheel down: ScrollTop = %v, want %v (%v + %v)", got, before+step, before, step)
	}

	// Clamping: never below 0, never above 1.
	tbl.ScrollForTest().ScrollTop = 0
	fireWheel(ui, tbl, 1)
	if got := tbl.ScrollForTest().ScrollTop; got != 0 {
		t.Errorf("wheel up at top: ScrollTop = %v, want 0", got)
	}
	tbl.ScrollForTest().ScrollTop = 1
	fireWheel(ui, tbl, -1)
	if got := tbl.ScrollForTest().ScrollTop; got != 1 {
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
	view := tbl.ScrollForTest().ViewRect()

	// Scrolled to the bottom: row 0 is pushed up out of the viewport.
	tbl.ScrollForTest().ScrollTop = 1
	pump()
	if r0 := tbl.RowCts[0].GetWidget().Rect; r0.Max.Y > view.Min.Y {
		t.Fatalf("bottom-scrolled fixture: row 0 = %v, want above view %v", r0, view)
	}

	// Rows shrink to two (they fit): the offset clears immediately and
	// the rows re-anchor to the top of the viewport.
	tbl.SetRows(rows(2))
	pump()
	if tbl.ScrollForTest().ScrollTop != 0 {
		t.Errorf("ScrollTop after shrink = %v, want 0", tbl.ScrollForTest().ScrollTop)
	}
	if r0 := tbl.RowCts[0].GetWidget().Rect; r0.Min.Y != view.Min.Y {
		t.Errorf("row 0 = %v, want top-anchored at view %v", r0, view)
	}
	r1 := tbl.RowCts[1].GetWidget().Rect
	if r1.Min.Y < tbl.RowCts[0].GetWidget().Rect.Max.Y {
		t.Errorf("row 1 %v overlaps row 0 %v", r1, tbl.RowCts[0].GetWidget().Rect)
	}
	if r1.Max.Y > view.Max.Y {
		t.Errorf("row 1 = %v, exceeds view %v", r1, view)
	}

	// Rows that fit ignore the wheel: the offset can never go stale.
	fireWheel(ui, tbl, -3)
	fireWheel(ui, tbl, -3)
	if tbl.ScrollForTest().ScrollTop != 0 {
		t.Errorf("ScrollTop after wheel on a fitted list = %v, want 0", tbl.ScrollForTest().ScrollTop)
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
		for r := range tbl.Rows {
			cell := tbl.cellRoot(r, 1)
			icon := tbl.Cells[r][1].Box.Widget().GetWidget().Rect
			label := tbl.Texts[r][1].GetWidget().Rect

			if icon.Dx() != theme.IconInline || icon.Dy() != theme.IconInline {
				t.Errorf("%s: row %d icon box = %v, want %d×%d",
					sz.tag, r, icon, theme.IconInline, theme.IconInline)
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
			if tbl.Cells[r][2] != nil {
				t.Errorf("%s: row %d right-aligned column grew an icon cell", sz.tag, r)
			}
		}
	}
}

// TestHeaderCellAnchorAgreement (Defect A regression): every header
// label sits at the SAME x its column's cell text uses — left anchors at
// the column's left edge, right anchors at the column's right edge minus
// cellPad — at scale 1 and at 1.5, for both alignment sides. The
// right-cell anchor accounts for ebitenui's End-positioning math
// (glyphs land Padding.Left+2×Padding.Right left of the rect edge;
// Padding.Right is 0 on purpose — see the Defect A note in newRow).
func TestHeaderCellAnchorAgreement(t *testing.T) {
	defer theme.SetScale(1)
	for _, sc := range []float64{1, 1.5} {
		theme.SetScale(sc)
		env, _ := newTestEnv(t)
		cols := []Column{
			{Key: "Item", Title: "Item", Priority: 0},
			{Key: "Qty", Title: "Qty", Priority: 1, AlignRight: true},
			{Key: "Vol", Title: "m³", Priority: 2, AlignRight: true},
		}
		tbl, ui := newPumpedTable(t, env, cols, ContentsFace(env.FaceMono(theme.StepSmall)))
		tbl.SetRows([]TableRow{{Cells: []string{"Tritanium", "×1,000", "100.0"}, Data: 0}})
		// SetRows refits; the cells get their rects on the next frame.
		ui.Update()
		ui.Draw(ebiten.NewImage(640, 400))

		for i, c := range cols {
			// Header label glyph box: the button's anchor padding put it
			// there and its own text padding is zero.
			lbl := tbl.hdrBtn[i].Text().GetWidget().Rect
			cell := tbl.Texts[0][i].GetWidget().Rect
			if c.AlignRight {
				cellEnd := cell.Max.X - theme.Px(cellPad)
				if lbl.Max.X != cellEnd {
					t.Errorf("scale %v col %q: header label ends %d, cell text ends %d (want equal)",
						sc, c.Title, lbl.Max.X, cellEnd)
				}
			} else {
				if lbl.Min.X != cell.Min.X {
					t.Errorf("scale %v col %q: header label starts %d, cell text starts %d (want equal)",
						sc, c.Title, lbl.Min.X, cell.Min.X)
				}
			}
		}
	}
}

// TestTableRowIconTints pins the icon palette roles: ColText by default,
// the row's own tint (own ship / hostiles) when it carries one, and the
// accent once the row is selected.
func TestTableRowIconTints(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl := NewTable(env, []Column{
		{Key: "Name", Title: "Name", Priority: 0},
		{Key: "Kind", Title: "Kind", Priority: 1},
	})
	tbl.SetRows([]TableRow{
		{Cells: []string{"Rock 1", "Asteroid"}, Icons: []string{"", "kind_asteroid"}, Data: 0},
		{Cells: []string{"Rat 1", "Ship"}, Icons: []string{"", "kind_ship"},
			IconTint: env.Pal().Danger, Data: 1},
	})
	if got := tbl.iconTint(0, tbl.Rows[0]); got != env.Pal().Text {
		t.Errorf("plain icon tint = %v, want ColText %v", got, env.Pal().Text)
	}
	if got := tbl.iconTint(1, tbl.Rows[1]); got != env.Pal().Danger {
		t.Errorf("hostile icon tint = %v, want ColDanger %v", got, env.Pal().Danger)
	}
	tbl.SelectData(1)
	if got := tbl.iconTint(1, tbl.Rows[1]); got != env.Pal().Accent {
		t.Errorf("selected icon tint = %v, want ColAccent %v", got, env.Pal().Accent)
	}
}
