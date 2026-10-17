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

// TestMarketOrderBookRows pins the order-book view: one row per resting
// order with a side tag, items grouped ascending, and inside every item
// the asks (price ascending) above the bids (price descending) — the
// canonical book order, price-time priority included.
func TestMarketOrderBookRows(t *testing.T) {
	env, _ := newTestEnv(t)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	frame := func() { uitest.PumpWindow(t, ui, m.Sync, 1024, 700) }
	frame()
	frame()

	if len(m.tbl.Rows) == 0 {
		t.Fatal("empty order book")
	}
	var (
		curItem  content.ItemID
		askPhase = true // within an item: asks may not follow bids
		lastAsk  = -1.0
		lastBid  = -1.0
		done     = map[content.ItemID]bool{}
	)
	for i, r := range m.tbl.Rows {
		if len(r.Cells) != 4 {
			t.Fatalf("row %d has %d cells, want 4 (Item/Price/Qty/Side)", i, len(r.Cells))
		}
		id, ok := r.Data.(content.ItemID)
		if !ok {
			t.Fatalf("row %d payload is %T, want content.ItemID", i, r.Data)
		}
		side := r.Cells[3]
		if side != "Ask" && side != "Bid" {
			t.Fatalf("row %d side = %q, want Ask or Bid", i, side)
		}
		price := parseCredits(t, r.Cells[1])
		if i == 0 || id != curItem {
			if done[curItem] {
				t.Fatalf("row %d: item %q left the board and came back (items must group)", i, curItem)
			}
			if i > 0 {
				done[curItem] = true
			}
			curItem, askPhase, lastAsk, lastBid = id, true, -1, -1
		}
		switch side {
		case "Ask":
			if !askPhase {
				t.Fatalf("row %d: ask after bids for %q (book out of order)", i, id)
			}
			if price < lastAsk {
				t.Fatalf("row %d: ask %v below a previous ask %v for %q", i, price, lastAsk, id)
			}
			lastAsk = price
		case "Bid":
			askPhase = false
			if lastBid >= 0 && price > lastBid {
				t.Fatalf("row %d: bid %v above a previous bid %v for %q", i, price, lastBid, id)
			}
			lastBid = price
		}
	}

	// The founding books carry both sides: the showcase station seeds asks
	// (hulls, modules, charges) and standing bids for the loot it buys.
	sides := map[string]bool{}
	for _, r := range m.tbl.Rows {
		sides[r.Cells[3]] = true
	}
	if !sides["Ask"] || !sides["Bid"] {
		t.Fatalf("board shows %v, want both Ask and Bid rows", sides)
	}
	// Rows read back to the sim's own numbers (cell <-> sim consistency).
	sys := env.Sim.System()
	if sys == nil {
		t.Fatal("no current system")
	}
	orders := env.Sim.Market(sys.ID)
	if len(m.tbl.Rows) != len(orders) {
		t.Fatalf("rows = %d, want %d resting orders", len(m.tbl.Rows), len(orders))
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
	stepper := m.qtyBar.GetWidget().Rect
	table := m.tbl.Container().GetWidget().Rect
	if !(title.Max.Y <= stepper.Min.Y && stepper.Max.Y <= table.Min.Y) {
		t.Errorf("stacking changed: title %v, stepper %v, book %v (want stacked top to bottom)",
			title, stepper, table)
	}
	for name, b := range map[string]*widget.Button{"-": m.minus, "+": m.plus, "Buy": m.buy, "Sell": m.sell} {
		if r := b.GetWidget().Rect; !r.In(stepper) {
			t.Errorf("%s button %v outside the stepper row %v", name, r, stepper)
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
	want := "Market — " + itemLabel(env, m.sel) + " " + m.tbl.Rows[0].Cells[1] +
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
// resizes to its current value through the row relayout — "Qty 1,000,000"
// owns a box big enough for itself and never reaches the + button, at
// scale 1 and 1.5 — and the stepped path (up to the ladder's 100) stays
// overlap-free too.
func TestStepperReadoutFitsItsValue(t *testing.T) {
	defer theme.SetScale(1)
	for _, sc := range []float64{1, 1.5} {
		theme.SetScale(sc)
		env, _ := newTestEnv(t)
		inv := NewInventory(env)
		ui := newWindowUI(t, env, inv.Win())
		inv.Sync()
		uitest.PumpWindow(t, ui, inv.Sync, 1280, 720)

		// doSync runs the panel Sync first (the production frame order);
		// the white-box big-value checks skip it because Sync re-derives
		// the readout from the clamped ladder.
		check := func(want string, doSync bool) {
			t.Helper()
			var sync func()
			if doSync {
				sync = inv.Sync
			}
			uitest.PumpWindow(t, ui, sync, 1280, 720)
			lbl := inv.qtyLabel.GetWidget().Rect
			plus := inv.plus.GetWidget().Rect
			minus := inv.minus.GetWidget().Rect
			if inv.qtyLabel.Label != want {
				t.Errorf("scale %v: readout = %q, want %q", sc, inv.qtyLabel.Label, want)
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

		// The stepped path (clamped ladder top), then the wide value the
		// readout must grow for — setStepperValue is exactly what Sync and
		// stepQty call, so this is the production path with a big number.
		inv.qtyIdx = len(qtySteps) - 1
		inv.stepQty(1)
		check("Qty 100", true)
		setStepperValue(inv.qtyBar, inv.qtyLabel, "Qty 1,000,000")
		check("Qty 1,000,000", false)
		inv.qtyIdx = 0
		check("Qty 1", true)
	}
}
