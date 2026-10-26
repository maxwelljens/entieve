package screens

import (
	"image"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
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

	// The book row declares the two trade verbs (both opening the quantity
	// picker on the row's item) and the two order-window launch verbs: Sell
	// on Market carries the player's holdings as the window's sell default,
	// Request on Market defaults to 1. Pressing each through a real Menu
	// routes its intent into the sink.
	spec := m.tbl.Rows[0].Spec
	wantActions := []widgets.RowAction{
		{Label: "Buy", Intent: widgets.QtyPick{Mode: widgets.QtyBuy, Item: "item-stub-ore"}},
		{Label: "Sell", Intent: widgets.QtyPick{Mode: widgets.QtySell, Item: "item-stub-ore"}},
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

// TestMarketMyOrdersListsEveryOrder pins the uncapped standing-order
// column: every player order the 20-slot cap allows is listed (no "+N
// more" truncation), each with its own Cancel; the list scrolls through
// them on the house rail — a wheel notch moves the offset and the rows
// with it; and the player's own book rows wear the owned-row green while
// the NPC rows keep the plain background.
func TestMarketMyOrdersListsEveryOrder(t *testing.T) {
	th := theme.NewSeam(theme.DefaultPalette(), theme.DefaultTypeScale())
	orders := []sim.MarketStock{
		{Item: "item-stub-ore", Price: 50, Qty: 1, Side: sim.OrderSell}, // an NPC ask
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
	rec := &recordingSink{}
	env := widgets.NewSeams(th, cv, rec)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	frame := func() { uitest.PumpWindow(t, ui, m.Sync, 1024, 700) }
	frame()
	frame()

	if len(m.mineRows) != 20 {
		t.Fatalf("mineRows = %d, want all 20 player orders listed", len(m.mineRows))
	}
	if want := "My orders — " + theme.FormatInt(20); m.mineLbl.Label != want {
		t.Errorf("label = %q, want %q", m.mineLbl.Label, want)
	}
	// The uncapped list must not grow the window: the default rect stands
	// (the scroll viewport bounds it, exactly like the order book). The
	// default is the three-pane landing rect — 760 × 560, on the rail
	// gutter's clear line and inside the 1024×700 minimum (see NewMarket).
	wantRect := image.Rect(theme.Px(97), theme.Px(8), theme.Px(857), theme.Px(568))
	if got := m.Win().GetContainer().GetWidget().Rect; got != wantRect {
		t.Errorf("window rect = %v, want the fixed rect %v (the list must not resize the window)", got, wantRect)
	}
	// Every order keeps its own Cancel verb: pressing two rows emits two
	// distinct order ids in the placed range (the per-row Cancel is intact
	// past the old three-row cap).
	for i, r := range m.mineRows {
		if r == nil || len(r.Buttons) != 1 {
			t.Fatalf("row %d has %d Cancel buttons, want one", i, len(r.Buttons))
		}
	}
	uitest.FireButton(ui, m.mineRows[0].Buttons[0])
	uitest.FireButton(ui, m.mineRows[19].Buttons[0])
	if len(rec.got) != 2 {
		t.Fatalf("Cancel presses emitted %#v, want two CancelOrder intents", rec.got)
	}
	first, ok1 := rec.got[0].(widgets.CancelOrder)
	last, ok2 := rec.got[1].(widgets.CancelOrder)
	if !ok1 || !ok2 || first.ID == last.ID || first.ID < 1 || last.ID < 1 || first.ID > 20 || last.ID > 20 {
		t.Fatalf("Cancel presses emitted %#v, want two distinct order ids in 1..20", rec.got)
	}

	// The list outgrows the column: the rail is up and a wheel notch
	// scrolls the rows inside the viewport (the list stays usable).
	view := m.mineScroll.ViewRect()
	union := m.mineRows[0].Root.GetWidget().Rect
	for _, r := range m.mineRows[1:] {
		union = union.Union(r.Root.GetWidget().Rect)
	}
	if union.Dy() <= view.Dy() {
		t.Fatalf("list content %d px does not overflow the %d px viewport", union.Dy(), view.Dy())
	}
	if !m.mineRail.Bar().GetWidget().IsVisible() {
		t.Error("rail hidden over an overflowing list")
	}
	row0 := m.mineRows[0].Root.GetWidget().Rect
	m.mineScroll.GetWidget().ScrolledEvent.Fire(&widget.WidgetScrolledEventArgs{Y: -3})
	frame()
	frame()
	if m.mineScroll.ScrollTop <= 0 {
		t.Fatalf("wheel: ScrollTop = %v, want > 0", m.mineScroll.ScrollTop)
	}
	if got := m.mineRows[0].Root.GetWidget().Rect; got.Min.Y >= row0.Min.Y {
		t.Errorf("wheel: row 0 at %d, want above %d (list moved)", got.Min.Y, row0.Min.Y)
	}

	// Ownership highlight inside the fold: the 20 player orders wear the
	// owned green, the one NPC ask keeps the plain background (and the
	// item's own parent row carries no tint at all).
	clickRow(ui, m.tbl, 0)
	frame()
	if got, want := len(m.tbl.Rows), 22; got != want {
		t.Fatalf("expanded rows = %d, want the parent + 21 orders (%d)", got, want)
	}
	want := theme.OwnedRowTint(theme.DefaultPalette())
	owned, plain := 0, 0
	for _, r := range m.tbl.Rows {
		switch {
		case r.Tint == want:
			owned++
		case r.Tint.A == 0:
			plain++
		default:
			t.Errorf("book row tint = %#v, want the owned green or none", r.Tint)
		}
	}
	if owned != 20 || plain != 2 {
		t.Errorf("book tints: %d owned, %d plain, want 20/2 (parent + NPC ask)", owned, plain)
	}
}

// TestMarketMyOrdersAfterOrderDialog proves the order-dialog path end to
// end over a real Kit and sim: the market book row's own launch intent
// (OpenOrder, emitted into the kit's sink exactly as the row menu emits
// it) opens the order mini-window on the docked station's book; the
// window's Accept emits PlaceOrder, which the kit maps onto the enqueued
// CmdPlaceOrder; the sim posts the order for OwnerPlayer; and the
// market's My orders column then lists it, its Cancel verb present and
// the matching book row wearing the owned green.
func TestMarketMyOrdersAfterOrderDialog(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	wd, ok := c.World("world-showcase")
	if !ok {
		t.Fatal("world-showcase missing")
	}
	s, err := sim.New(c, wd, 42)
	if err != nil {
		t.Fatalf("sim.New: %v", err)
	}
	// The real path: intents ride onto the sim's own command queue.
	k, err := NewKit(c, s, s.Queue)
	if err != nil {
		t.Fatalf("NewKit: %v", err)
	}
	// Jalvrek is sys-solace's market station: the order dialog and the
	// market column both read the docked book.
	s.Docked.Set(s.Player(), sim.Docked{At: "cel-haven-rest"})

	m := NewMarket(k.Env())
	mui := newWindowUI(t, k.Env(), m.Win())
	frame := func() { uitest.PumpWindow(t, mui, m.Sync, 1024, 700) }
	frame()
	frame()
	if len(m.mineRows) != 0 {
		t.Fatalf("fixture: %d player orders before placing one", len(m.mineRows))
	}

	const item = content.ItemID("loot-navdata")
	k.Env().Do(widgets.OpenOrder{Item: item, Side: sim.OrderBuy, Qty: 1})
	if !k.OrderOpen() {
		t.Fatal("the market's OpenOrder did not open the order dialog")
	}
	w := k.OrderWin()
	w.price.SetText("1") // a resting bid nothing on the book can cross
	w.qty.SetText("2")
	pumpKit(t, k, 1024, 700) // arm Accept through the field handlers
	if w.accept.GetWidget().Disabled {
		t.Fatal("Accept disarmed on valid typed fields")
	}
	uitest.FireButton(k.ui, w.accept) // Accept → PlaceOrder → CmdPlaceOrder → sim queue
	if k.OrderOpen() {
		t.Error("Accept did not close the dialog")
	}

	s.Step() // the queued CmdPlaceOrder posts the OwnerPlayer order
	frame()

	if len(m.mineRows) != 1 {
		t.Fatalf("My orders lists %d rows after the dialog, want 1", len(m.mineRows))
	}
	if btn := m.mineRows[0].Buttons; len(btn) != 1 {
		t.Fatalf("my-orders row has %d verbs, want one Cancel", len(btn))
	}
	if want := "My orders — 1"; m.mineLbl.Label != want {
		t.Errorf("label = %q, want %q", m.mineLbl.Label, want)
	}

	// Every OwnerPlayer book row is green, and there is at least one —
	// the order the dialog placed. It sits in its item's fold.
	clickRow(mui, m.tbl, mktParentRow(t, m, item))
	frame()
	ownedOrders := 0
	for _, o := range k.Env().BookOrders("cel-haven-rest") {
		if o.Owner == sim.OwnerPlayer {
			ownedOrders++
		}
	}
	want := theme.OwnedRowTint(theme.DefaultPalette())
	green := 0
	for _, r := range m.tbl.Rows {
		if r.Tint == want {
			green++
		}
	}
	if ownedOrders < 1 || green != ownedOrders {
		t.Fatalf("book: %d OwnerPlayer orders, %d green rows — want the placed order green", ownedOrders, green)
	}
}
