package screens

import (
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// TestMarketStandingOrderPanel drives the market's standing-order half over
// the stub seams: the Place order panel emits PlaceOrder for the selected
// item at its side/price and the toolbar quantity, and the My orders list
// shows ONLY the player's own orders — its Cancel button emitting
// CancelOrder for that row's id.
func TestMarketStandingOrderPanel(t *testing.T) {
	th := theme.NewSeam(theme.DefaultPalette(), theme.DefaultTypeScale())
	cv := &stubView{
		c: &content.Content{Items: map[content.ItemID]*content.ItemDef{
			"item-stub-ore": {Name: "Stub Ore", Category: content.CatOre},
		}},
		orders: []sim.MarketStock{
			{Item: "item-stub-ore", Price: 100, Qty: 5, Side: sim.OrderSell},
			{Item: "item-stub-ore", Price: 90, Qty: 2, Side: sim.OrderBuy, Owner: sim.OwnerPlayer, ID: 7},
		},
		docked: true,
		wallet: 1234.5,
	}
	rec := &recordingSink{}
	env := widgets.NewSeams(th, cv, rec)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	frame := func() { uitest.PumpWindow(t, ui, m.Sync, 1024, 700) }
	frame()
	frame()

	// My orders: the player's own order only (the system ask is excluded),
	// with a Cancel button on the row.
	if len(m.mineRows) != 1 {
		t.Fatalf("mineRows = %d, want the one player order", len(m.mineRows))
	}
	btn := m.mineRows[0].Buttons
	if len(btn) != 1 {
		t.Fatalf("my-orders row has %d buttons, want a single Cancel", len(btn))
	}
	// The row (and its Cancel button) lays out inside the list box.
	if r, box := m.mineRows[0].Root.GetWidget().Rect, m.mineBox.GetWidget().Rect; !r.In(box) {
		t.Errorf("my-orders row %v outside the list box %v", r, box)
	}
	if r, box := btn[0].GetWidget().Rect, m.mineBox.GetWidget().Rect; !r.In(box) {
		t.Errorf("Cancel button %v outside the list box %v", r, box)
	}
	uitest.FireButton(ui, btn[0])
	if len(rec.got) != 1 || rec.got[0] != (widgets.CancelOrder{ID: 7}) {
		t.Fatalf("cancel emitted %#v, want CancelOrder{ID:7}", rec.got)
	}

	// Place: selecting the item seeds the price from the displayed price,
	// one step moves one tick, and Place emits the intent at the toolbar
	// quantity.
	clickRow(ui, m.tbl, 0)
	frame()
	if !m.hasSel || m.price != 42 {
		t.Fatalf("selection/price = %v/%v, want selected at the stub price 42", m.hasSel, m.price)
	}
	m.stepPrice(1)
	if m.price != 42.01 {
		t.Fatalf("price after one step = %v, want 42.01 (one tick at 42)", m.price)
	}
	uitest.FireButton(ui, m.plus) // qty ladder: 1 → 10
	uitest.FireButton(ui, m.placeBtn)
	want := widgets.PlaceOrder{Item: "item-stub-ore", Side: sim.OrderBuy, Price: 42.01, Qty: 10}
	if len(rec.got) != 2 || rec.got[1] != want {
		t.Fatalf("place emitted %#v, want %#v", rec.got, want)
	}

	// The side toggle flips bid → ask.
	m.toggleSide()
	if m.side != sim.OrderSell {
		t.Fatalf("side = %v, want OrderSell after the toggle", m.side)
	}
}
