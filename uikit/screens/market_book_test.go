package screens

import (
	"math"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// TestMarketSelectionShowsTheBook pins the market's right pane: selecting
// an item puts that item's full order book there — asks (price ascending)
// above bids (price descending), the fold's own [price, qty, side] cells —
// while the catalog stays a plain item list with no order rows and no
// per-item expander. No selection means a hint line in the book section.
func TestMarketSelectionShowsTheBook(t *testing.T) {
	m, _, _, frame, click := mktTreeEnv(t)
	frame()
	frame()

	// Empty state: the section bar names the gesture, no rows.
	if got, want := m.bookLbl.Label, "Order book: select an item"; got != want {
		t.Errorf("book bar = %q, want %q", got, want)
	}
	if len(m.book) != 0 || len(m.bookRows) != 0 {
		t.Fatalf("book rows before a selection = %d/%d, want none", len(m.book), len(m.bookRows))
	}

	click(mktParentRow(t, m, "item-stub-ore"))
	if !m.hasSel || m.sel != "item-stub-ore" {
		t.Fatalf("selection = %q (hasSel=%v), want the clicked item", m.sel, m.hasSel)
	}
	// The catalog is items only: one row per catalog item, before and after
	// the click (the fold is gone), and no row carries an order payload.
	if got, want := len(m.tbl.Rows), 2; got != want {
		t.Fatalf("catalog rows after selecting = %d, want the plain %d item rows", got, want)
	}
	for i, r := range m.tbl.Rows {
		if _, ok := r.Data.(mktItemRow); !ok {
			t.Fatalf("catalog row %d payload = %#v, want an item row", i, r.Data)
		}
	}

	// The book itself: the fixture's four orders in book order.
	want := []struct {
		price float64
		qty   string
		side  string
	}{
		{100, "5", "Ask"},
		{120, "7", "Ask"},
		{90, "2", "Bid"},
		{80, "3", "Bid"},
	}
	if len(m.book) != len(want) || len(m.bookRows) != len(want) {
		t.Fatalf("book rows = %d/%d, want %d", len(m.book), len(m.bookRows), len(want))
	}
	if got, want := m.bookLbl.Label, "Order book: Stub Ore"; got != want {
		t.Errorf("book bar = %q, want %q", got, want)
	}
	for i, w := range want {
		r := m.book[i]
		if r.Cells[0] != "" {
			t.Errorf("book row %d item cell = %q, want blank (the bar names it)", i, r.Cells[0])
		}
		if got := parseCredits(t, r.Cells[1]); got != w.price {
			t.Errorf("book row %d price = %v, want %v", i, got, w.price)
		}
		if r.Cells[2] != w.qty || r.Cells[3] != w.side {
			t.Errorf("book row %d = %q/%q, want %q/%q", i, r.Cells[2], r.Cells[3], w.qty, w.side)
		}
		if len(r.Stack) != 1 || len(r.Stack[0].Segs) == 0 {
			t.Errorf("book row %d carries no line stack", i)
		}
	}

	// Rendered inside the pane's book section, and above My orders.
	box := m.bookBox.GetWidget().Rect
	for i, r := range m.bookRows {
		if got := r.Root.GetWidget().Rect; !got.In(box) {
			t.Errorf("book row %d %v lies outside the book section %v", i, got, box)
		}
	}
	if m.mineLbl.GetWidget().Rect.Min.Y < box.Max.Y {
		t.Errorf("My orders bar %v sits above the book section %v", m.mineLbl.GetWidget().Rect, box)
	}

	// A second click on the same item selects it again; it never folds
	// anything anywhere.
	click(mktParentRow(t, m, "item-stub-ore"))
	if got := len(m.tbl.Rows); got != 2 {
		t.Errorf("catalog rows after a second click = %d, want the plain 2", got)
	}
	if got := len(m.book); got != 4 {
		t.Errorf("book rows after a second click = %d, want the item's 4", got)
	}

	// Selecting another (orderless) item empties the book, not the section.
	click(mktParentRow(t, m, "item-stub-empty"))
	if got := len(m.book); got != 0 {
		t.Errorf("book rows for an orderless item = %d, want none", got)
	}
	if got, want := m.bookLbl.Label, "Order book: Stub Mineral"; got != want {
		t.Errorf("book bar = %q, want %q", got, want)
	}
}

// TestMarketSideRailCoversBothSections: the right pane's one scroll column
// carries the order book over My orders, and the house rail beside it
// scrolls both — one wheel notch moves both sections together and the rail
// mirrors the offset.
func TestMarketSideRailCoversBothSections(t *testing.T) {
	th := theme.NewSeam(theme.DefaultPalette(), theme.DefaultTypeScale())
	orders := []sim.MarketStock{
		{Item: "item-stub-ore", Price: 100, Qty: 5, Side: sim.OrderSell},
		{Item: "item-stub-ore", Price: 90, Qty: 2, Side: sim.OrderBuy},
	}
	for i := 1; i <= 20; i++ {
		orders = append(orders, sim.MarketStock{
			Item: "item-stub-ore", Price: float64(100 + i), Qty: i, Side: sim.OrderBuy,
			Owner: sim.OwnerPlayer, ID: uint64(i),
		})
	}
	cv := &stubView{
		c: &content.Content{Items: map[content.ItemID]*content.ItemDef{
			"item-stub-ore": {Name: "Stub Ore", Category: content.CatOre},
		}},
		orders: orders,
		docked: true,
		wallet: 1,
	}
	env := widgets.NewSeams(th, cv, &recordingSink{}, nil)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	frame := func() { uitest.PumpWindow(t, ui, m.Sync, 1024, 700) }
	frame()
	frame()
	clickRow(ui, m.tbl, 0) // select the item: its 21 resting orders fill the book
	frame()

	if len(m.book) != 22 || len(m.mineRows) != 20 {
		t.Fatalf("book/My orders rows = %d/%d, want the item's 22 orders and 20 My orders", len(m.book), len(m.mineRows))
	}
	// Both sections sit in the same scroll column: the book's box ends
	// before My orders' box starts, both inside the content.
	bookBox, mineBox := m.bookBox.GetWidget().Rect, m.mineBox.GetWidget().Rect
	if bookBox.Max.Y > mineBox.Min.Y {
		t.Errorf("book section %v overlaps My orders %v (want one column, book first)", bookBox, mineBox)
	}
	content := m.sideScroll.ContentRect()
	if !bookBox.In(content) || !mineBox.In(content) {
		t.Errorf("sections %v / %v outside the scroll content %v", bookBox, mineBox, content)
	}
	if !m.sideRail.Bar().GetWidget().IsVisible() {
		t.Fatal("rail hidden over an overflowing column")
	}

	// One wheel notch moves both sections' first rows and the offset.
	book0 := m.bookRows[0].Root.GetWidget().Rect
	mine0 := m.mineRows[0].Root.GetWidget().Rect
	m.sideScroll.GetWidget().ScrolledEvent.Fire(&widget.WidgetScrolledEventArgs{Y: -3})
	frame()
	frame()
	if m.sideScroll.ScrollTop <= 0 {
		t.Fatalf("wheel: ScrollTop = %v, want > 0", m.sideScroll.ScrollTop)
	}
	if got := m.bookRows[0].Root.GetWidget().Rect; got.Min.Y >= book0.Min.Y {
		t.Errorf("book row 0 at %d, want above %d (section moved)", got.Min.Y, book0.Min.Y)
	}
	if got := m.mineRows[0].Root.GetWidget().Rect; got.Min.Y >= mine0.Min.Y {
		t.Errorf("My orders row 0 at %d, want above %d (section moved)", got.Min.Y, mine0.Min.Y)
	}
	if want := int(math.Round(m.sideScroll.ScrollTop * widgets.RailSteps)); absInt(m.sideRail.Bar().Current-want) > 1 {
		t.Errorf("rail position = %d, want the mirrored ScrollTop %d", m.sideRail.Bar().Current, want)
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
