package screens

import (
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"strconv"
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
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
