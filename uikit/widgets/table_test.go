package widgets

import (
	"fmt"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"image"
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
	// The columns fit the cell MINUS the scrollbar gutter reserved at the
	// table's right edge (ScrollRail).
	gutter := float64(theme.Px(theme.ScrollGutter))
	if want := 640 - gutter; tbl.FitWidthForTest() != want {
		t.Fatalf("fittedFor = %v, want %v (640 − the %v px gutter)", tbl.FitWidthForTest(), want, gutter)
	}
	if sum := uitest.SumPrefix(tbl.widths); math.Abs(sum-(640-gutter)) > 1e-6 {
		t.Fatalf("column sum = %v, want %v", sum, 640-gutter)
	}

	// A narrower draw resizes the root; Sync refits to it. All the
	// surplus lives in the lowest-priority column, so that is the one
	// that gives it back.
	oldName := tbl.widths[0]
	ui.Draw(ebiten.NewImage(400, 300))
	tbl.Sync()
	if want := 400 - gutter; tbl.FitWidthForTest() != want {
		t.Fatalf("after resize fittedFor = %v, want %v", tbl.FitWidthForTest(), want)
	}
	if sum := uitest.SumPrefix(tbl.widths); math.Abs(sum-(400-gutter)) > 1e-6 {
		t.Fatalf("after resize column sum = %v, want %v", sum, 400-gutter)
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

// ---------------------------------------------------------------------------
// Rich rows: the line stack (the row form the contract board renders as)
// ---------------------------------------------------------------------------

// pumpRich flushes ebitenui's deferred event queue and draws one frame at
// 640×400 (row layout runs inside the draw).
func pumpRich(ui *ebitenui.UI) {
	ui.Update()
	ui.Draw(ebiten.NewImage(640, 400))
}

// newRichTable builds a one-column table over the given rows and pumps a
// frame, so the rich row widgets are built, validated and laid out.
func newRichTable(t *testing.T, env Seams, rows []TableRow) (*Table, *ebitenui.UI) {
	t.Helper()
	tbl := NewTable(env, []Column{{Key: "A", Title: "A"}})
	ui := &ebitenui.UI{Container: tbl.Container(), PrimaryTheme: env.WidgetTheme()}
	ui.Update()
	tbl.SetRows(rows)
	pumpRich(ui)
	return tbl, ui
}

// TestTableRichRowStackTiles pins the rich row's own layout: its lines
// stack vertically inside the row's padding, the cells of a line run left
// to right at the row's gap, the leading icon boxes with line 1's first
// cell at the icon gap over that cell's minimum width, the trailing action
// button rides line 1 at the rich-row box, a single-cell line renders as
// the bare text (what the row-surface goldens read), and a lone line
// centres itself in the strip (the stretched "+N more" row).
func TestTableRichRowStackTiles(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl, ui := newRichTable(t, env, []TableRow{
		{
			Cells: []string{"cleanup"}, // Cells stay the fit/sort source
			Icons: []string{"svc_contracts"},
			Stack: []RowLine{
				{Segs: []RowSeg{{Text: "Belt cleanup", MinW: 120}}},
				{Segs: []RowSeg{{Text: "Destroy 1× Scuttler"}, {Text: "Requires 0.20"}}},
				{Segs: []RowSeg{{Text: "40,000.00 € · +0.10 standing"}}},
			},
			Spec: RowSpec{Actions: []RowAction{{Label: "Accept"}}},
		},
		{Cells: []string{"note"}, Stack: []RowLine{{Segs: []RowSeg{{Text: "+4 more"}}}}},
	})
	pad, gap := theme.Px(RichPad), theme.Px(RichGap)

	if len(tbl.RowCts) != 2 {
		t.Fatalf("rows = %d, want 2", len(tbl.RowCts))
	}
	row := tbl.RowCts[0]
	lines := row.Children()
	if len(lines) != 3 {
		t.Fatalf("rich row lines = %d, want 3", len(lines))
	}
	rr := row.GetWidget().Rect
	y := rr.Min.Y
	for i, ln := range lines {
		got := ln.GetWidget().Rect
		if got.Min.X != rr.Min.X+pad || got.Min.Y != y {
			t.Errorf("line %d at %v, want x=%d y=%d (padded top-flush stack)",
				i, got, rr.Min.X+pad, y)
		}
		y = got.Max.Y
	}

	// Line 1: [icon + title] cell, then the trailing action button.
	line1 := lines[0].(*widget.Container)
	if len(line1.Children()) != 2 {
		t.Fatalf("line 1 children = %d, want the lead cell and the button", len(line1.Children()))
	}
	btn, ok := line1.Children()[1].(*widget.Button)
	if !ok {
		t.Fatalf("line 1's trailing child is %T, want *widget.Button", line1.Children()[1])
	}
	if got, want := btn.GetWidget().Rect.Min.X, line1.Children()[0].GetWidget().Rect.Max.X+gap; got != want {
		t.Errorf("button at x=%d, want %d (RichGap after the lead cell)", got, want)
	}
	if got := btn.GetWidget().Rect.Dx(); got < theme.Px(RichBtnW) {
		t.Errorf("button width %d, want ≥ RichBtnW %d", got, theme.Px(RichBtnW))
	}
	lead := line1.Children()[0].(*widget.Container)
	if len(lead.Children()) != 2 {
		t.Fatalf("lead cell children = %d, want icon + title", len(lead.Children()))
	}
	icon, title := lead.Children()[0].GetWidget(), lead.Children()[1].GetWidget()
	if got, want := title.Rect.Min.X, icon.Rect.Max.X+theme.Px(theme.IconGap); got != want {
		t.Errorf("title at x=%d, want %d (icon gap)", got, want)
	}
	if got := title.Rect.Dx(); got < 120 {
		t.Errorf("title width %d, want its MinW 120", got)
	}
	// The icon centres in the line (the row is as tall as its button).
	if title.Rect.Dy() >= line1.GetWidget().Rect.Dy() && icon.Rect.Min.Y < title.Rect.Min.Y {
		t.Errorf("icon %v is not centred against the title line", icon.Rect)
	}

	// Line 2: two cells at the row's gap; line 3: the bare reward text.
	line2 := lines[1].(*widget.Container)
	if len(line2.Children()) != 2 {
		t.Fatalf("line 2 cells = %d, want summary + requirement", len(line2.Children()))
	}
	if got, want := line2.Children()[1].GetWidget().Rect.Min.X,
		line2.Children()[0].GetWidget().Rect.Max.X+gap; got != want {
		t.Errorf("requirement at x=%d, want %d (RichGap)", got, want)
	}
	if _, ok := lines[2].(*widget.Text); !ok {
		t.Errorf("single-cell line rendered as %T, want the bare *widget.Text", lines[2])
	}

	// The lone-line row centres its line (a stretched strip's note row).
	note := tbl.RowCts[1]
	if len(note.Children()) != 1 {
		t.Fatalf("note row lines = %d, want 1", len(note.Children()))
	}
	ld, ok := note.Children()[0].GetWidget().LayoutData.(widget.RowLayoutData)
	if !ok || ld.Position != widget.RowLayoutPositionCenter {
		t.Errorf("lone line layout data = %#v, want centred", note.Children()[0].GetWidget().LayoutData)
	}
	ui.Draw(ebiten.NewImage(640, 400)) // keep the frame honest
}

// TestStackRowIconInsideRow pins the rich-row icon anchoring class the
// playtest flagged (the station contract board's rows): the icon box is
// exactly its Px-sized square, sits STRICTLY inside the row bounds,
// starts at the row's leading pad (its leading column slot) and centres
// vertically in line 1 with equal margins ±1 px — at scale 1 and 1.5.
func TestStackRowIconInsideRow(t *testing.T) {
	for _, scale := range []float64{1, 1.5} {
		uitest.UseScale(t, scale)
		env, _ := newTestEnv(t)
		tbl, _ := newRichTable(t, env, []TableRow{
			{
				Cells: []string{"cleanup"},
				Icons: []string{"svc_contracts"},
				Stack: []RowLine{
					{Segs: []RowSeg{{Text: "Belt cleanup", MinW: 120}}},
					{Segs: []RowSeg{{Text: "Destroy 1× Scuttler"}, {Text: "Requires 0.20"}}},
				},
				Spec: RowSpec{Actions: []RowAction{{Label: "Accept"}}},
			},
		})
		if tbl.rich == nil || tbl.rich[0] == nil || tbl.rich[0].Icon == nil {
			t.Fatalf("scale %v: rich row has no icon box", scale)
		}
		row := tbl.RowCts[0]
		rr := row.GetWidget().Rect
		icon := tbl.rich[0].Icon.Widget().GetWidget().Rect

		if icon.Dx() != theme.Px(theme.IconInline) || icon.Dy() != theme.Px(theme.IconInline) {
			t.Errorf("scale %v: icon box = %v, want %d² design (the Px funnel)", scale, icon, theme.IconInline)
		}
		if !icon.In(rr) {
			t.Errorf("scale %v: icon %v escapes the row %v", scale, icon, rr)
			continue
		}
		// Leading column: the box starts at the row's own pad.
		if l := icon.Min.X - rr.Min.X; math.Abs(float64(l-theme.Px(RichPad))) > 1 {
			t.Errorf("scale %v: icon %v sits %d px into row %v, want the %d px leading pad ±1",
				scale, icon, l, rr, theme.Px(RichPad))
		}
		// ...and centres in line 1 with equal margins (±2: the box
		// rounds twice — inside its lead cell, the cell inside the line).
		line1 := row.Children()[0].GetWidget().Rect
		top, bot := icon.Min.Y-line1.Min.Y, line1.Max.Y-icon.Max.Y
		if math.Abs(float64(top-bot)) > 2 {
			t.Errorf("scale %v: icon %v margins in line 1 %v: top %d bottom %d, want equal ±2",
				scale, icon, line1, top, bot)
		}
		// The title follows at the icon gap, inside the same lead cell.
		lead := row.Children()[0].(*widget.Container).Children()[0].(*widget.Container)
		title := lead.Children()[1].GetWidget().Rect
		if got, want := title.Min.X, icon.Max.X+theme.Px(theme.IconGap); math.Abs(float64(got-want)) > 1 {
			t.Errorf("scale %v: title at %d, want the icon gap %d", scale, got, want)
		}
		if !title.In(line1) {
			t.Errorf("scale %v: title %v escapes line 1 %v", scale, title, line1)
		}
		if !lead.GetWidget().Rect.In(rr) {
			t.Errorf("scale %v: lead cell %v escapes the row %v", scale, lead.GetWidget().Rect, rr)
		}
	}
}

// TestTableRichRowLineRefRoutes: a line that carries a Ref is its own
// gesture target through the shared installer — right-click opens ITS
// menu (bold-first Show Info, no row verbs) and a double-click emits
// Show Info for that ref and nothing else — while the row's own
// right-click still derives its menu from the row's spec.
func TestTableRichRowLineRefRoutes(t *testing.T) {
	env, _ := newTestEnv(t)
	var intents []Intent
	seams := NewSeams(env, env, SinkFunc(func(i Intent) {
		intents = append(intents, i)
		if om, ok := i.(OpenMenu); ok {
			env.Menu().Open(om.Entries, om.At) // what the host's sink does
		}
	}))
	ref := ItemRef("charge-emp")
	tbl, ui := newRichTable(t, seams, []TableRow{{
		Cells: []string{"emp"},
		Stack: []RowLine{
			{Segs: []RowSeg{{Text: "Deliver 2 m³ EMP Charge"}}},
			{Segs: []RowSeg{{Text: "EMP Charge"}}, Ref: &ref},
		},
		Spec: RowSpec{Actions: []RowAction{{Label: "Accept"}}},
	}})
	if tbl.RowCts == nil {
		t.Fatal("no rows")
	}
	line := tbl.RowCts[0].Children()[1].GetWidget()

	// Right-click on the line: its own menu, alone and leading with the
	// bold Show Info.
	line.MouseButtonClickedEvent.Fire(&widget.WidgetMouseButtonClickedEventArgs{
		Widget: line, Button: ebiten.MouseButtonRight, OffsetX: 4, OffsetY: 3,
	})
	pumpRich(ui)
	m := env.Menu()
	if !m.IsOpen() {
		t.Fatal("line right-click opened no menu")
	}
	entries := m.Entries()
	if len(entries) != 1 || entries[0].Label != "Show Info" || !entries[0].Bold {
		t.Fatalf("line menu = %+v, want the single bold Show Info", entries)
	}
	intents = nil // the OpenMenu the right-click rode on is already asserted
	m.Click(m.RowRect(0).Min.Add(image.Pt(1, 1)))
	if len(intents) != 1 {
		t.Fatalf("menu press enqueued %d intents, want 1", len(intents))
	}
	if si, ok := intents[0].(ShowInfo); !ok || si.Ref != ref {
		t.Fatalf("intent = %#v, want ShowInfo{%v}", intents[0], ref)
	}

	// Double-click on the line: Show Info, strictly (no menu, twice fast).
	intents = nil
	for i := 0; i < 2; i++ {
		line.MouseButtonClickedEvent.Fire(&widget.WidgetMouseButtonClickedEventArgs{
			Widget: line, Button: ebiten.MouseButtonLeft, OffsetX: 4, OffsetY: 3,
		})
		pumpRich(ui)
	}
	if len(intents) != 1 {
		t.Fatalf("double-click emitted %d intents, want 1: %#v", len(intents), intents)
	}
	if si, ok := intents[0].(ShowInfo); !ok || si.Ref != ref {
		t.Fatalf("double-click intent = %#v, want ShowInfo{%v}", intents[0], ref)
	}

	// The row's own right-click still opens the ROW's menu (its action,
	// its gate — no Show Info: a contract-like row declares no ref).
	intents = nil
	rowW := tbl.RowWidgetForTest(0)
	rowW.MouseButtonClickedEvent.Fire(&widget.WidgetMouseButtonClickedEventArgs{
		Widget: rowW, Button: ebiten.MouseButtonRight, OffsetX: 4, OffsetY: 3,
	})
	pumpRich(ui)
	entries = m.Entries()
	if len(entries) != 1 || entries[0].Label != "Accept" {
		t.Fatalf("row menu = %+v, want the single Accept entry", entries)
	}
}

// TestTableRichRowActionButtons: a rich row renders its Spec.Actions as
// trailing buttons (at most two), the disabled one is grayed — widget
// Disabled set — and emits nothing even when its click event is forced
// straight past that gate, the enabled one emits its intent through the
// sink.
func TestTableRichRowActionButtons(t *testing.T) {
	env, _ := newTestEnv(t)
	var intents []Intent
	seams := NewSeams(env, env, SinkFunc(func(i Intent) { intents = append(intents, i) }))
	tbl, ui := newRichTable(t, seams, []TableRow{{
		Cells: []string{"cleanup"},
		Stack: []RowLine{{Segs: []RowSeg{{Text: "Belt cleanup"}}}},
		Spec: RowSpec{Actions: []RowAction{
			{Label: "Accept", Intent: Accept{Contract: "contract-solace-cleanup-1"}},
			{Label: "Accepted", Disabled: true, Intent: Accept{Contract: "contract-solace-cordite-1"}},
			{Label: "Dropped", Intent: Accept{Contract: "contract-penumbra-den-hunt-1"}},
		}},
	}})
	line1, ok := tbl.RowCts[0].Children()[0].(*widget.Container)
	if !ok {
		t.Fatalf("line 1 = %T, want a container holding the buttons", tbl.RowCts[0].Children()[0])
	}
	var buttons []*widget.Button
	for _, c := range line1.Children() {
		if b, ok := c.(*widget.Button); ok {
			buttons = append(buttons, b)
		}
	}
	if len(buttons) != 2 {
		t.Fatalf("trailing buttons = %d, want 2 (the row offers at most two)", len(buttons))
	}
	if !buttons[1].GetWidget().Disabled {
		t.Error("grayed action renders enabled")
	}
	if buttons[0].GetWidget().Disabled {
		t.Error("enabled action renders disabled")
	}
	if got := buttons[0].Text().Label; got != "Accept" {
		t.Errorf("button label = %q, want Accept", got)
	}

	buttons[0].ClickedEvent.Fire(&widget.ButtonClickedEventArgs{Button: buttons[0]})
	pumpRich(ui)
	if len(intents) != 1 {
		t.Fatalf("enabled button emitted %d intents, want 1", len(intents))
	}
	if a, ok := intents[0].(Accept); !ok || a.Contract != "contract-solace-cleanup-1" {
		t.Fatalf("intent = %#v, want Accept{contract-solace-cleanup-1}", intents[0])
	}
	// The grayed one, clicked anyway (a harness can fire past the gate).
	buttons[1].ClickedEvent.Fire(&widget.ButtonClickedEventArgs{Button: buttons[1]})
	pumpRich(ui)
	if len(intents) != 1 {
		t.Fatalf("grayed button emitted %#v, want nothing further", intents)
	}
}

// TestTableRichRowChangeGatedRebuild pins the change gate for stacks: an
// identical SetRows reuses the row's widgets, while any move in what the
// stack renders — a line's text, the leading icon, a trailing button's
// gate — rebuilds it with the new content, and the fresh snapshot gates
// the next identical SetRows again.
func TestTableRichRowChangeGatedRebuild(t *testing.T) {
	env, _ := newTestEnv(t)
	rows := func(title, icon string, disabled bool) []TableRow {
		return []TableRow{{
			Cells: []string{title},
			Icons: []string{icon},
			Stack: []RowLine{
				{Segs: []RowSeg{{Text: title}}},
				{Segs: []RowSeg{{Text: "Destroy 1× Scuttler"}}},
			},
			Spec: RowSpec{Actions: []RowAction{{Label: "Accept", Disabled: disabled,
				Intent: Accept{Contract: "contract-solace-cleanup-1"}}}},
		}}
	}
	tbl, _ := newRichTable(t, env, rows("Belt cleanup", "", false))
	built := tbl.RowCts[0]
	lineTitle := func() string {
		return tbl.RowCts[0].Children()[0].(*widget.Container).Children()[0].(*widget.Text).Label
	}
	if got := lineTitle(); got != "Belt cleanup" {
		t.Fatalf("line 1 = %q, want Belt cleanup", got)
	}

	// Identical content: the widgets are reused (the diff-friendly rule).
	tbl.SetRows(rows("Belt cleanup", "", false))
	if tbl.RowCts[0] != built {
		t.Error("identical SetRows rebuilt the rich row")
	}

	// A line's text moved: rebuilt, carrying the new line.
	tbl.SetRows(rows("Cordite order", "", false))
	if tbl.RowCts[0] == built {
		t.Fatal("moved stack text did not rebuild the row")
	}
	if got := lineTitle(); got != "Cordite order" {
		t.Errorf("rebuilt line 1 = %q, want Cordite order", got)
	}

	// The leading icon moved: rebuilt (the icon is stack-rendered too).
	iconRow := tbl.RowCts[0]
	tbl.SetRows(rows("Cordite order", "svc_contracts", false))
	if tbl.RowCts[0] == iconRow {
		t.Error("moved leading icon did not rebuild the row")
	}

	// A trailing button's gate moved: rebuilt, grayed — and the fresh
	// snapshot gates the next identical SetRows.
	gated := tbl.RowCts[0]
	tbl.SetRows(rows("Cordite order", "svc_contracts", true))
	if tbl.RowCts[0] == gated {
		t.Fatal("moved action gate did not rebuild the row")
	}
	line1 := tbl.RowCts[0].Children()[0].(*widget.Container)
	btn, ok := line1.Children()[len(line1.Children())-1].(*widget.Button)
	if !ok || !btn.GetWidget().Disabled {
		t.Fatalf("rebuilt button disabled = %v, want the grayed gate", !ok || btn.GetWidget().Disabled)
	}
	stable := tbl.RowCts[0]
	tbl.SetRows(rows("Cordite order", "svc_contracts", true))
	if tbl.RowCts[0] != stable {
		t.Error("unchanged gate rebuilt the row again (snapshot not refreshed)")
	}

	// Dropping the stack flips the row back to plain form: rebuilt as
	// per-column text widgets (the reverse move rebuilds too).
	tbl.SetRows([]TableRow{{Cells: []string{"Cordite order"}}})
	if tbl.rich[0] != nil {
		t.Fatal("row kept its stack widgets after dropping Stack")
	}
	if got := tbl.Texts[0][0].Label; got != "Cordite order" {
		t.Errorf("plain cell label = %q, want the cell text", got)
	}
	if len(tbl.RowCts[0].Children()) != 1 {
		t.Errorf("plain row children = %d, want the single column cell", len(tbl.RowCts[0].Children()))
	}
}

// TestTableScrollRailGutter: the table's scrollbar rides a reserved
// gutter beside the viewport — the gutter is ScrollGutter wide, reserved
// even while the rows fit (the rail hides but the viewport stays inset,
// so it never jumps), and once the rows overflow the handle shows, never
// intersects the viewport, and a wheel notch still scrolls — at scale 1
// and 1.5.
func TestTableScrollRailGutter(t *testing.T) {
	for _, tc := range []struct {
		name  string
		scale float64
	}{{"scale 1", 1}, {"scale 1.5", 1.5}} {
		t.Run(tc.name, func(t *testing.T) {
			uitest.UseScale(t, tc.scale)
			env, _ := newTestEnv(t)
			tbl, ui := newPumpedTable(t, env, []Column{{Key: "A", Title: "A"}})
			tbl.SetRows([]TableRow{{Cells: []string{"one"}, Data: 0}})
			tbl.Sync()
			ui.Draw(ebiten.NewImage(640, 400))

			// The table root stops ScrollGutter short of the wrapper.
			if got := tbl.Container().GetWidget().Rect.Dx() - tbl.root.GetWidget().Rect.Dx(); got != theme.Px(theme.ScrollGutter) {
				t.Errorf("reserved gutter = %d, want ScrollGutter %d", got, theme.Px(theme.ScrollGutter))
			}
			if tbl.Rail().GetWidget().IsVisible() {
				t.Error("rail visible while one row fits")
			}

			// Overflow: the rail shows and the handle clears the viewport.
			rows := make([]TableRow, 60)
			for i := range rows {
				rows[i] = TableRow{Cells: []string{"row"}, Data: i}
			}
			tbl.SetRows(rows)
			tbl.Sync()
			ui.Draw(ebiten.NewImage(640, 400))
			tbl.Sync()
			if !tbl.Rail().GetWidget().IsVisible() {
				t.Fatalf("rail hidden over an overflowing table (overflow %v)", tbl.scrollOverflow())
			}
			bar := tbl.Rail().GetWidget().Rect
			if bar.Overlaps(tbl.ScrollForTest().ViewRect()) {
				t.Errorf("scrollbar %v intersects the viewport %v", bar, tbl.ScrollForTest().ViewRect())
			}
			if !bar.In(tbl.Container().GetWidget().Rect) {
				t.Errorf("scrollbar %v leaves the table %v", bar, tbl.Container().GetWidget().Rect)
			}
			// Wheel still scrolls the viewport.
			tbl.ScrollForTest().GetWidget().ScrolledEvent.Fire(&widget.WidgetScrolledEventArgs{Y: -3})
			ui.Update()
			ui.Draw(ebiten.NewImage(640, 400))
			if tbl.ScrollForTest().ScrollTop <= 0 {
				t.Fatalf("wheel: ScrollTop = %v, want > 0", tbl.ScrollForTest().ScrollTop)
			}
		})
	}
}

// TestTableRowDensityPins: information density in tables is deliberate
// (EVE-like), so the furniture tokens never reach a row's metrics — the
// cell and row padding stay their literal values. The header gap is
// furniture and does take the token.
func TestTableRowDensityPins(t *testing.T) {
	if entryPadV != 5 || cellPad != 5 {
		t.Errorf("row density = entryPadV %d / cellPad %d, want 5/5 (deliberate)", entryPadV, cellPad)
	}
	if headerSpace != theme.SpSm {
		t.Errorf("headerSpace = %d, want the SpSm furniture token %d", headerSpace, theme.SpSm)
	}
}
