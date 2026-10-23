package widgets

import (
	"image"
	"math"
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/hajimehoshi/ebiten/v2"
)

// The column MODEL, reported from play: the frozen bases and the widths on
// screen used to diverge after a freeze (the fit fed the bases back through
// the first-fit policy: all surplus to the grow column, all shortage taken
// from the priority order), so a grip dragged a number the player could not
// see. In the market the Item column kept the whole surplus above its own
// floor — "cannot make Item smaller than a very large size" — and a hidden
// column freed width that no grip could give to the others. These pins hold
// the two policies apart: the first fit keeps the old grow-column measure
// (byte-identical default frames), every later layout rescales the frozen
// bases proportionally, and a grip drags the widths the player is looking
// at (a press adopts the laid-out widths as the base).

// bookCols/bookRows are the market book's shape at its book pane width.
func bookCols() []Column {
	return []Column{
		{Key: "Item", Title: "Item", Priority: 0},
		{Key: "Price", Title: "Price", Priority: 1, AlignRight: true},
		{Key: "Qty", Title: "Qty", Priority: 2, AlignRight: true},
		{Key: "Side", Title: "Side", Priority: 3},
	}
}

func bookRows() []TableRow {
	return []TableRow{
		{Cells: []string{"Tritanium", "9,999.99 €", "12,345", "Ask"}, Data: 0},
		{Cells: []string{"Scordite", "12.34 €", "1,000", "Bid"}, Data: 1},
	}
}

// newSizedTable builds a standalone table in its own UI and lays it out at
// w px, so its first fit freezes at exactly that width (newPumpedTable would
// freeze at its 640 px draw first).
func newSizedTable(t *testing.T, env Seams, cols []Column, w int) (*Table, *ebitenui.UI) {
	t.Helper()
	tbl := NewTable(env, cols)
	ui := &ebitenui.UI{Container: tbl.Container(), PrimaryTheme: env.WidgetTheme()}
	ui.Update()
	ui.Draw(ebiten.NewImage(w, 400))
	return tbl, ui
}

// pumpAt runs the real frame the layout lives in — Sync, the deferred event
// pump, the widget layout at w — and one more Sync, which is where a new
// table width reaches the columns.
func pumpAt(t *testing.T, tbl *Table, ui *ebitenui.UI, w int) {
	t.Helper()
	tbl.Sync()
	ui.Update()
	ui.Draw(ebiten.NewImage(w, 400))
	tbl.Sync()
}

// dragGrip drags column i's strip dx pixels through its Step core.
func dragGrip(t *testing.T, tbl *Table, i, dx int) {
	t.Helper()
	if i < 0 || i >= len(tbl.grips) {
		t.Fatalf("no grip %d", i)
	}
	r := tbl.grips[i].GetWidget().Rect
	if r.Empty() {
		t.Fatalf("column %d has no grip strip", i)
	}
	pt := r.Min.Add(image.Pt(r.Dx()/2, r.Dy()/2))
	tbl.Step(pt, true, true)
	tbl.Step(pt.Add(image.Pt(dx, 0)), false, true)
	tbl.Step(pt.Add(image.Pt(dx, 0)), false, false)
}

// TestColumnGripResizesTheWidthOnScreen is bug 1: the market book frozen at
// its book pane, then opened larger. The Item column showed its frozen width
// plus the whole surplus, while the grip moved only the frozen width under
// it — the column bottomed out at floor+surplus, a "very large size" the
// player could never get past, and Price/Qty could not take the room.
func TestColumnGripResizesTheWidthOnScreen(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl, ui := newSizedTable(t, env, bookCols(), 360) // the book pane width
	tbl.SetRows(bookRows())
	pumpAt(t, tbl, ui, 360)
	if !tbl.frozen {
		t.Fatal("table never froze")
	}
	pumpAt(t, tbl, ui, 700) // the market window opened/resized wider: surplus

	before := append([]float64(nil), tbl.widths...)
	floor := tbl.base[0].Min
	if before[0] < floor+40 {
		t.Fatalf("Item shows %v over a %v floor: the surplus setup is wrong", before[0], floor)
	}
	dragGrip(t, tbl, 0, -1000)
	pumpAt(t, tbl, ui, 700)
	after := tbl.widths

	if after[0] > floor+0.5 {
		t.Errorf("Item = %v after dragging it to the left edge, want its %v floor: the drag moved a width the player cannot see", after[0], floor)
	}
	if want := before[1] + (before[0] - after[0]); math.Abs(after[1]-want) > 1 {
		t.Errorf("Price = %v, want %v (the width Item gave up)", after[1], want)
	}
	if sum := uitest.SumPrefix(after); math.Abs(sum-tbl.FitWidthForTest()) > 1e-6 {
		t.Errorf("column sum = %v, want %v (still tiling)", sum, tbl.FitWidthForTest())
	}
	// Durable: another frame lays the same widths out again.
	pumpAt(t, tbl, ui, 700)
	if tbl.widths[0] != after[0] || tbl.widths[1] != after[1] {
		t.Errorf("widths %v/%v did not survive the next frame: %v/%v",
			after[0], after[1], tbl.widths[0], tbl.widths[1])
	}
}

// TestColumnGripAfterHidingAColumn is bug 2: hide Velocity in the overview
// (the ⋯ menu path), then resize. The freed width was parked on the grow
// column (Name) and Name's grip could not pass it on — its trade partner
// Distance sat on its header floor — so the drag died at once.
func TestColumnGripAfterHidingAColumn(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl, ui := newSizedTable(t, env, overviewCols(), 364)
	tbl.SetRows(overviewRows("0 m/s"))
	// The overview's default 380 px window less its padding, the scrollbar
	// gutter and the ⋯ strip: the fit squeezes every column but Name.
	pumpAt(t, tbl, ui, 364)
	at := image.Pt(10, 10)
	tbl.openColumnMenu(at)
	env.Menu().Entries()[3].Action() // hide Velocity, the real entry path
	pumpAt(t, tbl, ui, 364)
	if !tbl.hidden[3] {
		t.Fatal("the menu entry hid nothing")
	}

	// The freed width spreads over the visible columns, ratios kept: the
	// grow column no longer parks it.
	if got, want := tbl.widths[1]/tbl.widths[2], tbl.base[1].Width/tbl.base[2].Width; math.Abs(got-want) > 1e-6 {
		t.Errorf("Distance:Kind ratio = %v, want the frozen %v", got, want)
	}
	if sum := uitest.SumPrefix(tbl.widths); math.Abs(sum-tbl.FitWidthForTest()) > 1e-6 {
		t.Errorf("column sum = %v, want %v (the freed width still tiles)", sum, tbl.FitWidthForTest())
	}

	// Name's strip: +20 onto Distance, which now has room to give.
	nameBefore := tbl.widths[0]
	dragGrip(t, tbl, 0, 20)
	pumpAt(t, tbl, ui, 364)
	if got, want := tbl.widths[0], nameBefore+20; math.Abs(got-want) > 1 {
		t.Errorf("Name = %v after a +20 drag, want %v (the resize must survive)", got, want)
	}

	// And the other boundary: Distance grows by the slack Kind can give.
	distBefore := tbl.widths[1]
	dragGrip(t, tbl, 1, 20)
	pumpAt(t, tbl, ui, 364)
	if tbl.widths[1] <= distBefore+1 {
		t.Errorf("Distance = %v after a +20 drag, want wider than %v", tbl.widths[1], distBefore)
	}
	if sum := uitest.SumPrefix(tbl.widths); math.Abs(sum-tbl.FitWidthForTest()) > 1e-6 {
		t.Errorf("column sum = %v after the drags, want %v", sum, tbl.FitWidthForTest())
	}
}

// TestColumnLayoutPolicy is the pure split: the first fit is the old elastic
// measure, every later layout preserves the frozen ratios, and a short table
// still gives width back in Priority order toward the floors.
func TestColumnLayoutPolicy(t *testing.T) {
	bases := func() []ColWidth {
		return []ColWidth{
			col("Name", 100, 40),
			col("Distance", 50, 30),
			col("Kind", 30, 20),
			col("Velocity", 40, 25),
		}
	}
	prio := []int{0, 1, 2, 3}

	// Wider: every column scales by the same factor, the row keeps tiling.
	got := layoutColumns(bases(), prio, make([]bool, 4), 440)
	for i, want := range []float64{200, 100, 60, 80} {
		if math.Abs(got[i]-want) > 1e-9 {
			t.Errorf("wide: column %d = %v, want %v", i, got[i], want)
		}
	}
	if sum := uitest.SumPrefix(got); math.Abs(sum-440) > 1e-9 {
		t.Errorf("wide: sum = %v, want 440", sum)
	}

	// Exact: the bases are the layout, byte for byte.
	got = layoutColumns(bases(), prio, make([]bool, 4), 220)
	for i, want := range []float64{100, 50, 30, 40} {
		if got[i] != want {
			t.Errorf("exact: column %d = %v, want %v", i, got[i], want)
		}
	}

	// Short: the shipped Priority order toward the floors, unchanged.
	got = layoutColumns(bases(), prio, make([]bool, 4), 180)
	want := fitColumns(bases(), prio, -1, 180)
	for i := range want {
		if math.Abs(got[i]-want[i].Width) > 1e-9 {
			t.Errorf("short: column %d = %v, want the priority fit %v", i, got[i], want[i].Width)
		}
	}

	// Hidden: filtered out (width 0), the visible set rescaled to tile.
	hidden := []bool{false, true, false, false}
	got = layoutColumns(bases(), prio, hidden, 300)
	if got[1] != 0 {
		t.Errorf("hidden column width = %v, want 0", got[1])
	}
	if sum := uitest.SumPrefix(got); math.Abs(sum-300) > 1e-9 {
		t.Errorf("hidden: sum = %v, want 300", sum)
	}
	if r := got[0] / got[2]; math.Abs(r-100.0/30.0) > 1e-9 {
		t.Errorf("hidden: Name:Kind ratio = %v, want the frozen %v", r, 100.0/30.0)
	}
}
