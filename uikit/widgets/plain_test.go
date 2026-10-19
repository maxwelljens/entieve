package widgets

import (
	"image"
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// newPlain builds a PlainTable with its rows in its own UI and pumps one
// frame so the grid layout has run at w×h.
func newPlain(t *testing.T, env Seams, cols []PlainColumn, rows []PlainRow, w, h int) (*PlainTable, *ebiten.Image) {
	t.Helper()
	pt := NewPlainTable(env, cols)
	pt.SetRows(rows)
	ui := &ebitenui.UI{Container: pt.Container(), PrimaryTheme: env.WidgetTheme()}
	img := ebiten.NewImage(w, h)
	ui.Update()
	ui.Draw(img)
	return pt, img
}

// plainCellRect is row r column c's laid-out cell rect.
func plainCellRect(pt *PlainTable, r, c int) image.Rectangle {
	return pt.RowWidgetForTest(r).Children()[c].(*widget.Text).GetWidget().Rect
}

// TestPlainTableColumnsAlign pins the decorless alignment: every row's
// label starts on the body's left edge, every value ends on the row's right
// edge, and all value right edges agree (within ±1) — at UI scales 1 and
// 1.5, so the anchor holds when the mono face and the design units scale.
func TestPlainTableColumnsAlign(t *testing.T) {
	defer theme.SetScale(1)
	rows := []PlainRow{
		{Cells: []string{"Cash", "1,234.50 €"}},
		{Cells: []string{"Deployed in orders", "90.00 €"}},
		{Cells: []string{"Receivables", "30,000.00 €"}},
		{Cells: []string{"Total", "31,324.50 €"}},
	}
	for _, sc := range []float64{1, 1.5} {
		theme.SetScale(sc)
		env, _ := newTestEnv(t)
		pt, _ := newPlain(t, env, []PlainColumn{{}, {AlignRight: true}}, rows, 480, 200)

		body := pt.RowWidgetForTest(0).GetWidget().Rect
		valueX := 0
		for r := range rows {
			row := pt.RowWidgetForTest(r).GetWidget().Rect
			if row.Dx() != body.Dx() || row.Min.X != body.Min.X {
				t.Errorf("scale %v: row %d spans %v, want the body width %v", sc, r, row, body)
			}
			if got := plainCellRect(pt, r, 0).Min.X; got != row.Min.X {
				t.Errorf("scale %v: row %d label starts %d, want the row edge %d", sc, r, got, row.Min.X)
			}
			val := plainCellRect(pt, r, 1).Max.X
			if val != row.Max.X {
				t.Errorf("scale %v: row %d value ends %d, want the row edge %d", sc, r, val, row.Max.X)
			}
			if r == 0 {
				valueX = val
			} else if d := val - valueX; d > 1 || d < -1 {
				t.Errorf("scale %v: row %d value x = %d, want %d ±1 (one column)", sc, r, val, valueX)
			}
		}
	}
}

// plainTree is every widget in n's subtree (depth-first).
func plainTree(n widget.PreferredSizeLocateableWidget) []widget.PreferredSizeLocateableWidget {
	out := []widget.PreferredSizeLocateableWidget{n}
	if c, ok := n.(widget.Containerer); ok {
		for _, ch := range c.Children() {
			out = append(out, plainTree(ch)...)
		}
	}
	return out
}

// TestPlainTableDecorless pins the "no decoration" contract: the root tiles
// exactly the rows (no header strip), and the whole tree is containers and
// texts — no header sort buttons, no scroll viewport/rail, nothing that
// could paint a tint, zebra or border. (Pixel readback is a capture-loop
// concern; this is the windowless structural pin.)
func TestPlainTableDecorless(t *testing.T) {
	env, _ := newTestEnv(t)
	rows := []PlainRow{
		{Cells: []string{"Cash", "1,234.50 €"}},
		{Cells: []string{"Total", "1,234.50 €"}},
	}
	pt, _ := newPlain(t, env, []PlainColumn{{}, {AlignRight: true}}, rows, 320, 80)

	if got := len(pt.Container().Children()); got != len(rows) {
		t.Errorf("root children = %d, want %d rows and no header", got, len(rows))
	}
	for _, n := range plainTree(pt.Container()) {
		switch n.(type) {
		case *widget.Container, *widget.Text:
		default:
			t.Errorf("plain table tree carries a %T — decoration or chrome", n)
		}
	}
}

// TestPlainTableReuseArbitraryColumns is the reuse surface: any number of
// columns of arbitrary content align — every value column's right edges
// agree down the table and the last column lands on the row's right edge.
func TestPlainTableReuseArbitraryColumns(t *testing.T) {
	env, _ := newTestEnv(t)
	rows := []PlainRow{
		{Cells: []string{"Tritanium", "1,000", "100.0"}},
		{Cells: []string{"Pyerite", "25", "9.5"}},
		{Cells: []string{"Mexallon", "7,500", "1,234.5"}},
	}
	pt, _ := newPlain(t, env, []PlainColumn{{}, {AlignRight: true}, {AlignRight: true}}, rows, 480, 200)

	row0 := pt.RowWidgetForTest(0).GetWidget().Rect
	for _, c := range []int{1, 2} {
		want := plainCellRect(pt, 0, c).Max.X
		for r := range rows {
			if got := plainCellRect(pt, r, c).Max.X; got != want {
				t.Errorf("col %d row %d right edge = %d, want %d", c, r, got, want)
			}
		}
	}
	if got := plainCellRect(pt, 0, 2).Max.X; got != row0.Max.X {
		t.Errorf("value column ends %d, want the row edge %d", got, row0.Max.X)
	}
	if got := pt.RowsForTest()[0].Cells[0]; got != "Tritanium" {
		t.Errorf("row 0 col 0 = %q, want Tritanium", got)
	}
}
