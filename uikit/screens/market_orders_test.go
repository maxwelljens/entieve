package screens

import (
	"image"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// TestMarketStandingOrderPanel drives the market's standing-order half over
// the stub seams: the My orders list shows ONLY the player's own orders —
// its Cancel button emitting CancelOrder for that row's id — and every book
// row declares the two order-window launch verbs ("Sell on Market" /
// "Request on Market"), which emit OpenOrder through the row's ActionSink
// (the typed order mini-window is the posting surface now; the old Place
// panel is gone).
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

	// The book row declares the two order-window launch verbs: Sell on
	// Market carries the player's holdings as the window's sell default,
	// Request on Market defaults to 1. Pressing each through a real Menu
	// routes OpenOrder into the sink.
	spec := m.tbl.Rows[0].Spec
	wantActions := []widgets.RowAction{
		{Label: "Buy", Intent: widgets.Buy{Item: "item-stub-ore", Qty: 1}},
		{Label: "Sell", Intent: widgets.Sell{Item: "item-stub-ore", Qty: 1}},
		{Label: "Sell on Market", Intent: widgets.OpenOrder{Item: "item-stub-ore", Side: sim.OrderSell, Qty: 5}},
		{Label: "Request on Market", Intent: widgets.OpenOrder{Item: "item-stub-ore", Side: sim.OrderBuy, Qty: 1}},
	}
	if len(spec.Actions) != len(wantActions) {
		t.Fatalf("row actions = %+v, want %+v", spec.Actions, wantActions)
	}
	for i, want := range wantActions {
		if spec.Actions[i].Label != want.Label || spec.Actions[i].Intent != want.Intent {
			t.Errorf("action %d = %+v, want %+v", i, spec.Actions[i], want)
		}
	}

	entries := widgets.RowMenuEntries(cv, rec, spec)
	menu := widgets.NewMenu(th)
	press := func(label string) {
		t.Helper()
		menu.Open(entries, image.Pt(10, 10))
		for i, e := range menu.Entries() {
			if e.Label == label {
				menu.Click(menu.RowRect(i).Min.Add(image.Pt(1, 1)))
				return
			}
		}
		t.Fatalf("menu has no %q entry: %+v", label, menu.Entries())
	}
	press("Sell on Market")
	press("Request on Market")
	if len(rec.got) != 3 {
		t.Fatalf("order-window launches emitted %#v, want two OpenOrder intents", rec.got)
	}
	if got := rec.got[1]; got != (widgets.OpenOrder{Item: "item-stub-ore", Side: sim.OrderSell, Qty: 5}) {
		t.Errorf("Sell on Market emitted %#v, want OpenOrder{sell, qty 5}", got)
	}
	if got := rec.got[2]; got != (widgets.OpenOrder{Item: "item-stub-ore", Side: sim.OrderBuy, Qty: 1}) {
		t.Errorf("Request on Market emitted %#v, want OpenOrder{buy, qty 1}", got)
	}
}
