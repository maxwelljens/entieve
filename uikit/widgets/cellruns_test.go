package widgets

import (
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// TestTableCellRuns pins the styled-run cell: a cell with CellRuns draws
// its segments as separate texts in one line (the dim availability
// suffix), keeps its plain string as the fit/sort source, and a moved run
// rebuilds the row instead of being dropped.
func TestTableCellRuns(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl := NewTable(env, []Column{
		{Key: "Item", Title: "Item"},
		{Key: "Qty", Title: "Qty", AlignRight: true},
	})
	row := TableRow{
		Cells:    []string{"Ore", "12"},
		CellRuns: make([][]RowSeg, 2),
	}
	row.CellRuns[1] = []RowSeg{
		{Text: "12"},
		{Text: " · 3 more", Tint: env.Pal().Dim, Step: theme.StepTiny},
	}
	tbl.SetRows([]TableRow{row})

	if tbl.Texts[0][0] == nil {
		t.Fatal("the plain cell lost its label")
	}
	if tbl.Texts[0][1] != nil {
		t.Fatal("the run cell built a single label — the suffix would share its tint")
	}
	cell := tbl.Cells[0][1]
	if cell == nil || len(cell.ctr.Children()) != 1 {
		t.Fatalf("run cell = %+v, want one line container", cell)
	}
	runs := cell.ctr.Children()[0].(*widget.Container).Children()
	if len(runs) != 2 {
		t.Fatalf("run cell draws %d texts, want 2", len(runs))
	}
	if got := runs[1].(*widget.Text).Label; got != " · 3 more" {
		t.Errorf("suffix text = %q, want the dim run", got)
	}
	// The plain string stays the cell's fit/sort source.
	if got := cellOf(tbl.Rows[0], 1); got != "12" {
		t.Errorf("cell source = %q, want 12", got)
	}
	// The column is measured from the drawn runs, not the two-char cell:
	// the suffix must fit without spilling into the Side column.
	want := 2 * float64(theme.Px(cellPad))
	for _, r := range row.CellRuns[1] {
		face := tbl.Face
		if r.Step != "" {
			face = tbl.th.Face(r.Step)
		}
		w, _ := text.Measure(r.Text, *face, 0)
		want += w
	}
	if got := tbl.widths[1]; got < want {
		t.Errorf("Qty column = %v, want ≥ %v (the suffix's measured width)", got, want)
	}

	// A moved run rebuilds the row's widgets.
	row.CellRuns[1][1].Text = " · 4 more"
	tbl.SetRows([]TableRow{row})
	runs = tbl.Cells[0][1].ctr.Children()[0].(*widget.Container).Children()
	if got := runs[1].(*widget.Text).Label; got != " · 4 more" {
		t.Errorf("moved suffix = %q, want the rebuilt row", got)
	}
}
