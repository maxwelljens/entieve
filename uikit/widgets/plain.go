package widgets

import (
	"image/color"
	"math"
	"reflect"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// Plain-table geometry (design units, scaled by theme.Px). The gutter is a
// furniture token; the row gap is a deliberate data-row literal — the
// table's cellPad/entryPadV precedent (CONTEXT.md: spacing tokens).
const (
	plainRowGap = 2          // vertical gap between rows (dense readout rhythm)
	plainGutter = theme.SpMd // gutter between columns
)

// PlainColumn is one column of a PlainTable. AlignRight marks a value
// column: its cells anchor on the column's right edge and draw in the mono
// tabular face (the house rule for figures). Every other column is a
// left-aligned label in the display face.
type PlainColumn struct {
	AlignRight bool
}

// PlainRow is one PlainTable row: one cell string per column, plus the
// value cells' palette colour (Tint, zero = ColText). Labels stay ColDim.
type PlainRow struct {
	Cells []string
	Tint  color.RGBA
}

// PlainTable is a decorless column list: aligned columns, no header, no row
// tints or zebra, no borders — a dense label/value readout for blocks like
// the Finances holdings. It shares the label/value rows' alignment (Show
// Info's rowWidget): leading columns pack at their measured widths so their
// left edges agree down the table, and the last column stretches to the
// row's right edge, so a value's End anchor lands on one shared x at every
// UI scale.
//
// shortcut: a row narrower than its content (leading columns + value) lets
// the anchored value overlap the label — no shrink or ellipsis. Upgrade to
// a fitted-column layout if a decorless list must survive a narrow window.
type PlainTable struct {
	th   theme.Theme
	cols []PlainColumn
	body *widget.Container
	rows []PlainRow
}

// NewPlainTable builds a decorless table with the given columns over the
// theme seam (faces + palette). Rows arrive via SetRows; embed the
// container in a window and give it the full width of its cell so the
// value column can reach the right edge.
func NewPlainTable(env Seams, cols []PlainColumn) *PlainTable {
	t := &PlainTable{th: env, cols: append([]PlainColumn(nil), cols...)}
	t.body = widget.NewContainer(widget.ContainerOpts.Layout(widget.NewRowLayout(
		widget.RowLayoutOpts.Direction(widget.DirectionVertical),
		widget.RowLayoutOpts.Spacing(theme.Px(plainRowGap)),
	)))
	return t
}

// Container returns the table's widget tree for embedding in a window.
func (t *PlainTable) Container() widget.Containerer { return t.body }

// SetRows replaces the table's rows (a copy) and rebuilds the row widgets.
// Leading columns are packed at their widest cell so their left edges align
// down the table; the last column stretches to the row's right edge. An
// identical call is a no-op (owners Sync every frame — the Table contract).
func (t *PlainTable) SetRows(rows []PlainRow) {
	if reflect.DeepEqual(t.rows, rows) {
		return
	}
	t.body.RemoveChildren()
	t.rows = append([]PlainRow(nil), rows...)
	n := len(t.cols)
	if n == 0 {
		return
	}
	lead := n - 1
	widths := make([]int, lead)
	for j := 0; j < lead; j++ {
		face := t.face(j)
		for _, r := range t.rows {
			if w := int(math.Ceil(TipMeasure(plainCell(r, j), face))); w > widths[j] {
				widths[j] = w
			}
		}
	}
	for _, row := range t.rows {
		t.body.AddChild(t.newRow(row, widths))
	}
}

// face is column j's cell face: mono for value columns, display otherwise.
func (t *PlainTable) face(j int) *text.Face {
	if t.cols[j].AlignRight {
		return t.th.FaceMono(theme.StepSmall)
	}
	return t.th.Face(theme.StepSmall)
}

// newRow builds one row: a single-row grid whose leading columns are pinned
// to their measured widths and whose last column stretches, so a value
// cell's End anchor lands on the row's right edge.
func (t *PlainTable) newRow(row PlainRow, widths []int) *widget.Container {
	p := t.th.Pal()
	n := len(t.cols)
	stretch := make([]bool, n)
	stretch[n-1] = true
	ctr := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(n),
			widget.GridLayoutOpts.Stretch(stretch, []bool{false}),
			widget.GridLayoutOpts.Spacing(theme.Px(plainGutter), 0),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)
	for j := range t.cols {
		align := widget.TextPositionStart
		col := p.Dim
		if t.cols[j].AlignRight {
			align = widget.TextPositionEnd
			col = p.Text
			if row.Tint.A != 0 {
				col = row.Tint
			}
		}
		tx := widget.NewText(
			widget.TextOpts.Text(plainCell(row, j), t.face(j), theme.Solid(col)),
			widget.TextOpts.Position(align, widget.TextPositionCenter),
		)
		if j < len(widths) {
			tx.GetWidget().MinWidth = widths[j]
		}
		ctr.AddChild(tx)
	}
	return ctr
}

// plainCell returns row cell i ("" when the row is short).
func plainCell(row PlainRow, i int) string {
	if i < 0 || i >= len(row.Cells) {
		return ""
	}
	return row.Cells[i]
}

// ---------------------------------------------------------------------------
// Test hooks — the internals a surface test asserts geometry on from another
// package, the Table RowWidgetForTest/RowsForTest precedent.
// ---------------------------------------------------------------------------

// RowWidgetForTest exposes row i's container (the alignment pins read its
// cell rects).
func (t *PlainTable) RowWidgetForTest(i int) *widget.Container {
	if i < 0 || i >= len(t.body.Children()) {
		return nil
	}
	return t.body.Children()[i].(*widget.Container)
}

// RowsForTest is the rows last set.
func (t *PlainTable) RowsForTest() []PlainRow { return t.rows }
