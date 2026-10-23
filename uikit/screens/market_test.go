package screens

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// parseCredits reads back a FormatCredits cell ("1,234.56 €", "50¢").
func parseCredits(t *testing.T, s string) float64 {
	t.Helper()
	if strings.HasSuffix(s, "¢") {
		cents, err := strconv.ParseFloat(strings.TrimSuffix(s, "¢"), 64)
		if err != nil {
			t.Fatalf("bad cents cell %q: %v", s, err)
		}
		return cents / 100
	}
	s = strings.TrimSuffix(strings.TrimSpace(s), " €")
	s = strings.ReplaceAll(s, ",", "")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("bad credits cell %q: %v", s, err)
	}
	return v
}

// TestMarketRowIcons pins the market's row glyphs: every item row (catalog
// parent, expanded order, search hit) carries a 20 px icon — a module's own
// art, else its group's glyph — while the browse-tree branch rows stay
// text-only.
func TestMarketRowIcons(t *testing.T) {
	env, _ := newTestEnv(t)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	uitest.PumpWindow(t, ui, m.Sync, 1024, 700)

	seen := map[string]bool{}
	for _, r := range m.tbl.Rows {
		it, ok := r.Data.(mktItemRow)
		if !ok {
			continue
		}
		if r.IconSize != mktRowIcon {
			t.Errorf("%s: icon size = %d, want %d", it.Item, r.IconSize, mktRowIcon)
		}
		if len(r.Icons) != 1 || r.Icons[0] != m.itemIcon(it.Item) {
			t.Errorf("%s: icon = %v, want %q", it.Item, r.Icons, m.itemIcon(it.Item))
		}
		seen[r.Icons[0]] = true
	}
	for _, want := range []string{"mod_autocannon_s", "ammo", "act_loot", "kind_belt", "kind_asteroid", "row_hull"} {
		if !seen[want] {
			t.Errorf("no catalog row carries %q (saw %v)", want, seen)
		}
	}

	// Branch rows (All, Ammunition, …) are text-only.
	for _, r := range m.tree.Rows {
		if len(r.Icons) != 0 || r.IconSize != 0 {
			t.Errorf("tree row %q carries an icon (%v)", r.Cells, r.Icons)
		}
	}

	// An expanded item's order rows inherit its glyph (same 20 px box).
	expanded := false
	for _, r := range m.tbl.Rows {
		it, ok := r.Data.(mktItemRow)
		if !ok || expanded {
			continue
		}
		if r.Cells[2] == "0" {
			continue // no resting orders to fold out
		}
		m.open[it.Item] = true
		expanded = true
	}
	if !expanded {
		t.Fatal("fixture: no item with resting orders")
	}
	uitest.PumpWindow(t, ui, m.Sync, 1024, 700)
	books := 0
	for _, r := range m.tbl.Rows {
		if o, ok := r.Data.(mktOrderRow); ok {
			books++
			if r.IconSize != mktRowIcon || len(r.Icons) != 1 || r.Icons[0] != m.itemIcon(o.Item) {
				t.Errorf("order row icon = %v/%d, want the item glyph at %d", r.Icons, r.IconSize, mktRowIcon)
			}
		}
	}
	if books == 0 {
		t.Error("fixture: no expanded order rows")
	}
}

// TestMarketOrderBookRows pins the order-book tree: one parent row per
// item of the tradeable catalog (canonical order, always listed), its
// resting orders folded out beneath it on a click — asks (price
// ascending) above the bids (price descending) — and every order of the
// station's book present exactly once, expanded.
func TestMarketOrderBookRows(t *testing.T) {
	env, _ := newTestEnv(t)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	frame := func() { uitest.PumpWindow(t, ui, m.Sync, 1024, 700) }
	frame()
	frame()

	want := marketCatalog(env)
	if len(m.tbl.Rows) != len(want) {
		t.Fatalf("rows = %d, want one parent per catalog item (%d)", len(m.tbl.Rows), len(want))
	}
	for i, r := range m.tbl.Rows {
		if len(r.Cells) != 4 {
			t.Fatalf("row %d has %d cells, want 4 (Item/Price/Qty/Side)", i, len(r.Cells))
		}
		row, ok := r.Data.(mktItemRow)
		if !ok {
			t.Fatalf("row %d payload is %#v, want an item (parent) row", i, r.Data)
		}
		if row.Item != want[i] {
			t.Fatalf("row %d item = %q, want %q (canonical catalog order)", i, row.Item, want[i])
		}
		if r.Cells[3] != mktNoSide {
			t.Errorf("row %d side = %q, want %q", i, r.Cells[3], mktNoSide)
		}
	}

	// Expanding every item shows the station's whole book, in book order
	// inside each item: asks ascending, then bids descending.
	sys := env.Sim.System()
	if sys == nil {
		t.Fatal("no current system")
	}
	orders := env.Sim.Market(sys.ID)
	if len(orders) == 0 {
		t.Fatal("fixture: empty book")
	}
	for _, item := range want {
		m.open[item] = true
	}
	frame()
	if got, wantN := len(m.tbl.Rows), len(want)+len(orders); got != wantN {
		t.Fatalf("expanded rows = %d, want %d (catalog + every order)", got, wantN)
	}
	seen := 0
	var (
		curItem  content.ItemID
		askPhase = true // within an item: asks may not follow bids
		lastAsk  = -1.0
		lastBid  = -1.0
		done     = map[content.ItemID]bool{}
	)
	for i, r := range m.tbl.Rows {
		if parent, isParent := r.Data.(mktItemRow); isParent {
			if done[curItem] && i > 0 {
				t.Fatalf("row %d: item %q left the board and came back (items must group)", i, curItem)
			}
			if i > 0 {
				done[curItem] = true
			}
			curItem, askPhase, lastAsk, lastBid = parent.Item, true, -1, -1
			continue
		}
		row, isOrder := r.Data.(mktOrderRow)
		if !isOrder {
			t.Fatalf("row %d payload = %#v, want an item or an order row", i, r.Data)
		}
		seen++
		if row.Item != curItem {
			t.Fatalf("row %d: order row for %q under parent %q", i, row.Item, curItem)
		}
		if r.Cells[0] != "" {
			t.Errorf("row %d: child item cell = %q, want blank (the parent names it)", i, r.Cells[0])
		}
		side := r.Cells[3]
		price := parseCredits(t, r.Cells[1])
		switch side {
		case "Ask":
			if !askPhase {
				t.Fatalf("row %d: ask after bids for %q (book out of order)", i, row.Item)
			}
			if price < lastAsk {
				t.Fatalf("row %d: ask %v below a previous ask %v for %q", i, price, lastAsk, row.Item)
			}
			lastAsk = price
		case "Bid":
			askPhase = false
			if lastBid >= 0 && price > lastBid {
				t.Fatalf("row %d: bid %v above a previous bid %v for %q", i, price, lastBid, row.Item)
			}
			lastBid = price
		default:
			t.Fatalf("row %d side = %q, want Ask or Bid", i, side)
		}
	}
	if seen != len(orders) {
		t.Fatalf("child rows = %d, want every resting order (%d) listed once", seen, len(orders))
	}

	// The founding books carry both sides: the showcase station seeds asks
	// (hulls, modules, charges, materials) and standing bids (loot and the
	// raw materials its industry buys).
	sides := map[string]bool{}
	for _, r := range m.tbl.Rows {
		if r.Cells[3] != mktNoSide {
			sides[r.Cells[3]] = true
		}
	}
	if !sides["Ask"] || !sides["Bid"] {
		t.Fatalf("board shows %v, want both Ask and Bid rows", sides)
	}
}

// TestMarketOrderBookScrolls pins the order book's scrollability
// (playtest: "the marketplace window does not have a scrollbar"), in the
// pattern the table scroll tests use: the book rides the standard
// ScrollContainer with real overflow at the window's own size, a wheel
// notch moves the offset and the rows with it, and the scrollbar rail
// beside the viewport mirrors the offset — and writes drags back — while
// the title line and the stepper/button row keep their place above the
// book.
func TestMarketOrderBookScrolls(t *testing.T) {
	env, _ := newTestEnv(t)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	frame := func() { uitest.PumpWindow(t, ui, m.Sync, 1280, 720) }
	frame()
	frame()

	sc := m.tbl.ScrollForTest()
	if sc == nil {
		t.Fatal("market table has no scroll container")
	}
	view := sc.ViewRect()
	if !view.In(m.Win().GetContainer().GetWidget().Rect) {
		t.Fatalf("scroll viewport %v outside the window %v", view, m.Win().GetContainer().GetWidget().Rect)
	}
	rows := m.tbl.RowCts
	if len(rows) < 3 {
		t.Fatalf("rows = %d, want a book long enough to overflow", len(rows))
	}
	union := rows[0].GetWidget().Rect
	for _, r := range rows[1:] {
		union = union.Union(r.GetWidget().Rect)
	}
	if union.Dy() <= view.Dy() {
		t.Fatalf("book content %d px does not overflow the %d px viewport", union.Dy(), view.Dy())
	}
	if !m.bar.GetWidget().IsVisible() {
		t.Error("scrollbar hidden over an overflowing book")
	}

	// Wheel: the table's scroll handler moves the offset and the rows.
	row0 := rows[0].GetWidget().Rect
	sc.GetWidget().ScrolledEvent.Fire(&widget.WidgetScrolledEventArgs{Y: -3})
	frame()
	frame()
	if sc.ScrollTop <= 0 {
		t.Fatalf("wheel: ScrollTop = %v, want > 0", sc.ScrollTop)
	}
	if got := rows[0].GetWidget().Rect; got.Min.Y >= row0.Min.Y {
		t.Errorf("wheel: row 0 at %d, want above %d (content moved)", got.Min.Y, row0.Min.Y)
	}
	// The rail mirrors the offset the wheel set.
	if want := int(math.Round(sc.ScrollTop * mktBarSteps)); m.bar.Current != want {
		t.Errorf("rail position = %d, want the mirrored ScrollTop %d", m.bar.Current, want)
	}

	// Drag: the rail writes the offset back through its changed event
	// (what a handle drag fires mid-frame), and the mirror re-converges.
	sc.ScrollTop = 0
	m.bar.ChangedEvent.Fire(&widget.SliderChangedEventArgs{Slider: m.bar, Current: 500, Dragging: true})
	ui.Update() // dispatch the deferred event before the next frame's mirror
	frame()
	frame()
	if got := sc.ScrollTop; math.Abs(got-0.5) > 1e-9 {
		t.Errorf("rail drag: ScrollTop = %v, want 0.5", got)
	}
	if m.bar.Current != 500 {
		t.Errorf("rail position = %d, want 500 (mirror of ScrollTop 0.5)", m.bar.Current)
	}

	// The window's own rows are untouched: title line, stepper row
	// (minus/readout/plus/buy/sell), then the book under them.
	title := m.title.GetWidget().Rect
	stepper := m.stepper.Row().GetWidget().Rect
	table := m.tbl.Container().GetWidget().Rect
	if !(title.Max.Y <= stepper.Min.Y && stepper.Max.Y <= table.Min.Y) {
		t.Errorf("stacking changed: title %v, stepper %v, book %v (want stacked top to bottom)",
			title, stepper, table)
	}
	for name, b := range map[string]*widget.Button{"-": m.stepper.Minus(), "+": m.stepper.Plus(), "Buy": m.buy, "Sell": m.sell} {
		if r := b.GetWidget().Rect; !r.In(stepper) {
			t.Errorf("%s button %v outside the stepper row %v", name, r, stepper)
		}
	}
}

// TestMarketBookWheelSettles: a fast flick on the order book lands and
// stays. Two notches on consecutive frames, then silence: the offset may
// not reverse and the rows may not jump — the rail's mirror and its
// Changed write-back locked two frames apart and the book oscillated one
// wheel step every frame.
func TestMarketBookWheelSettles(t *testing.T) {
	env, _ := newTestEnv(t)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	frame := func() { uitest.PumpWindow(t, ui, m.Sync, 1024, 700) }
	frame()
	frame()
	clickRow(ui, m.tbl, 0) // fold the first item open: a book long enough to scroll
	frame()

	sc := m.tbl.ScrollForTest()
	for i := 0; i < 2; i++ {
		sc.GetWidget().ScrolledEvent.Fire(&widget.WidgetScrolledEventArgs{Y: -1})
		frame()
	}
	var tops []float64
	for f := 0; f < 30; f++ {
		frame()
		tops = append(tops, sc.ScrollTop)
	}
	if tops[len(tops)-1] <= 0 {
		t.Fatalf("the flick scrolled nothing (ScrollTop %.5f)", tops[len(tops)-1])
	}
	for i := 2; i < len(tops); i++ {
		if (tops[i]-tops[i-1])*(tops[i-1]-tops[i-2]) < 0 {
			t.Fatalf("offset reversed with no wheel input at frame %d: %.5f -> %.5f -> %.5f",
				i, tops[i-2], tops[i-1], tops[i])
		}
	}
	// ebitenui re-anchors the rows from ScrollTop every frame: a still
	// frame with a moving content top is the jump the user sees.
	last := sc.ContentRect().Min.Y
	for f := 0; f < 10; f++ {
		frame()
		y := sc.ContentRect().Min.Y
		if y != last {
			t.Fatalf("book content jumped from y=%d to y=%d with no input", last, y)
		}
	}
}

// TestMarketTitleShowsPriceAndFee: unselected, the title carries the
// wallet; selected, it switches to the item's displayed price (the price
// ladder) and the fee schedule — the view's last-price + fee display.
func TestMarketTitleShowsPriceAndFee(t *testing.T) {
	env, _ := newTestEnv(t)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	frame := func() { uitest.PumpWindow(t, ui, m.Sync, 1024, 700) }
	frame()
	frame()

	if want := "Market — " + theme.FormatCredits(env.Sim.Wallet()); m.title.Label != want {
		t.Fatalf("title = %q, want %q", m.title.Label, want)
	}

	clickRow(ui, m.tbl, 0)
	frame()
	if !m.hasSel {
		t.Fatal("selection did not take")
	}
	// The title quotes the shown book: the selected row's own price for the
	// item, plus the fee schedule.
	want := "Market — " + env.TradeableName(m.sel) + " " + m.tbl.Rows[0].Cells[1] +
		" · fee " + feePct(sim.MarketBrokerFee) + "+" + feePct(sim.MarketSalesTax)
	if m.title.Label != want {
		t.Fatalf("title = %q, want %q", m.title.Label, want)
	}
	if !strings.Contains(m.title.Label, feePct(sim.MarketBrokerFee)) ||
		!strings.Contains(m.title.Label, feePct(sim.MarketSalesTax)) {
		t.Fatalf("title %q does not quote both fees", m.title.Label)
	}
}

// TestStepperReadoutFitsItsValue (Defect B regression): the qty readout
// resizes to its current value as the stepper steps — "Qty 1" grows to
// "Qty 100" and never reaches the + button, at scale 1 and 1.5. (The
// million-digit case lives beside the widget: widgets
// TestQtyStepperReadoutGrows.)
func TestStepperReadoutFitsItsValue(t *testing.T) {
	defer theme.SetScale(1)
	for _, sc := range []float64{1, 1.5} {
		theme.SetScale(sc)
		env, _ := newTestEnv(t)
		inv := NewInventory(env)
		ui := newWindowUI(t, env, inv.Win())
		inv.Sync()
		uitest.PumpWindow(t, ui, inv.Sync, 1280, 720)

		check := func(want string) {
			t.Helper()
			uitest.PumpWindow(t, ui, inv.Sync, 1280, 720)
			lbl := inv.stepper.Label().GetWidget().Rect
			plus := inv.stepper.Plus().GetWidget().Rect
			minus := inv.stepper.Minus().GetWidget().Rect
			if got := inv.stepper.Label().Label; got != want {
				t.Errorf("scale %v: readout = %q, want %q", sc, got, want)
			}
			// The box measures the value itself (same face, no h-padding).
			need, _ := text.Measure(want, *env.FaceMono(theme.StepSmall), 0)
			if float64(lbl.Dx()) < need {
				t.Errorf("scale %v: readout box %d wide, need ≥ %v for %q",
					sc, lbl.Dx(), need, want)
			}
			if lbl.Max.X > plus.Min.X {
				t.Errorf("scale %v: readout (%v) overlaps + button (%v)", sc, lbl, plus)
			}
			if lbl.Min.X < minus.Max.X {
				t.Errorf("scale %v: readout (%v) overlaps − button (%v)", sc, lbl, minus)
			}
		}

		check("Qty 1")
		inv.stepper.Step(2) // the ladder's top, 100
		check("Qty 100")
		inv.stepper.Step(-2)
		check("Qty 1")
	}
}
