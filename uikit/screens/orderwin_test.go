package screens

import (
	"fmt"
	"image"
	"sort"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// TestOrderWindowFitsContent is the regression lock for the clipped dialog
// (the qty field and the button row used to fall past the frame): every
// child rect — the content body, the mode toggle, the price field, the live
// preview, the qty field and both buttons — must lie inside the window
// frame at UI scale 1.0 and 1.5, and the frame must be at least the
// content's own preferred size. Content that outgrows orderMinW/orderMinH
// fails here instead of silently overflowing.
func TestOrderWindowFitsContent(t *testing.T) {
	for _, scale := range []float64{1.0, 1.5} {
		t.Run(fmt.Sprintf("scale %.1f", scale), func(t *testing.T) {
			uitest.UseScale(t, scale)
			_, _, env := orderStub([]sim.MarketStock{
				{Item: "item-stub-ore", Price: 100, Qty: 5, Side: sim.OrderSell},
				{Item: "item-stub-ore", Price: 90, Qty: 2, Side: sim.OrderBuy},
			})
			w := NewOrderWin(env)
			ui := newWindowUI(t, env, w.Win())
			w.Show("item-stub-ore", sim.OrderSell, 5)
			uitest.PumpWindow(t, ui, nil, 1024, 700)

			frame := w.win.GetContainer().GetWidget().Rect
			if pw, ph := w.win.GetContainer().PreferredSize(); frame.Dx() < pw || frame.Dy() < ph {
				t.Errorf("frame %v is smaller than its content %dx%d", frame, pw, ph)
			}
			for _, c := range []struct {
				name string
				rect image.Rectangle
			}{
				{"body", w.body.GetWidget().Rect},
				{"toggle", w.modeBtn.GetWidget().Rect},
				{"price", w.price.GetWidget().Rect},
				{"preview", w.preview.GetWidget().Rect},
				{"qty", w.qty.GetWidget().Rect},
				{"accept", w.accept.GetWidget().Rect},
				{"cancel", w.cancel.GetWidget().Rect},
			} {
				if !c.rect.In(frame) {
					t.Errorf("%s rect %v is not inside the window frame %v", c.name, c.rect, frame)
				}
			}
		})
	}
}

// orderStub builds the seam-stub world the order window reads: one item
// with a base price, a docked market and a caller-set book.
func orderStub(orders []sim.MarketStock) (*stubView, *recordingSink, widgets.Seams) {
	// Scale the type at build time like the kit does, so a pin that raises
	// the UI scale exercises the real face metrics (identity at scale 1).
	th := theme.NewSeam(theme.DefaultPalette(), theme.ScaleTypeScale(theme.DefaultTypeScale()))
	cv := &stubView{
		c: &content.Content{Items: map[content.ItemID]*content.ItemDef{
			"item-stub-ore": {Name: "Stub Ore", Category: content.CatOre, BasePrice: 7},
		}},
		orders: orders,
		docked: true,
		wallet: 1000,
	}
	rec := &recordingSink{}
	return cv, rec, widgets.NewSeams(th, cv, rec)
}

// TestOrderWindowDefaults pins the mode defaults: Sell = best bid + 1 €,
// Request = best ask − 1 €, an empty side falls back to the item's base
// price, and the quantity defaults to the clicked stack (Sell) or 1
// (Request) — re-applied on every mode toggle.
func TestOrderWindowDefaults(t *testing.T) {
	cv, _, env := orderStub([]sim.MarketStock{
		{Item: "item-stub-ore", Price: 100, Qty: 5, Side: sim.OrderSell},
		{Item: "item-stub-ore", Price: 90, Qty: 2, Side: sim.OrderBuy},
	})
	w := NewOrderWin(env)
	ui := newWindowUI(t, env, w.Win())
	frame := func() { uitest.PumpWindow(t, ui, nil, 1024, 700) }

	if !w.Show("item-stub-ore", sim.OrderSell, 5) {
		t.Fatal("Show refused a docked launch")
	}
	frame()
	if got, want := w.TitleText(), "Stub Ore — Order"; got != want {
		t.Errorf("title = %q, want %q", got, want)
	}
	if got, want := w.price.GetText(), "91"; got != want { // best bid 90 + 1
		t.Errorf("sell price = %q, want %q", got, want)
	}
	if got, want := w.qty.GetText(), "5"; got != want {
		t.Errorf("sell qty = %q, want the clicked stack %q", got, want)
	}
	if w.accept.GetWidget().Disabled {
		t.Error("Accept disabled on valid sell defaults")
	}

	// Toggle to Request: the mode's defaults re-apply (best ask 100 − 1, qty 1).
	w.toggleMode()
	frame()
	if w.side != sim.OrderBuy {
		t.Fatalf("side = %v, want OrderBuy after one toggle", w.side)
	}
	if got, want := w.modeBtn.Text().Label, "Request"; got != want {
		t.Errorf("mode label = %q, want %q", got, want)
	}
	if got, want := w.price.GetText(), "99"; got != want {
		t.Errorf("request price = %q, want %q", got, want)
	}
	if got, want := w.qty.GetText(), "1"; got != want {
		t.Errorf("request qty = %q, want %q", got, want)
	}

	// Toggling back re-applies Sell's defaults.
	w.toggleMode()
	frame()
	if got, want := w.modeBtn.Text().Label, "Sell"; got != want {
		t.Errorf("mode label = %q, want %q", got, want)
	}
	if got, want := w.price.GetText(), "91"; got != want {
		t.Errorf("sell price after re-toggle = %q, want %q", got, want)
	}
	if got, want := w.qty.GetText(), "5"; got != want {
		t.Errorf("sell qty after re-toggle = %q, want %q", got, want)
	}

	// Empty bid side: Sell falls back to the base price.
	cv.orders = []sim.MarketStock{{Item: "item-stub-ore", Price: 100, Qty: 5, Side: sim.OrderSell}}
	w.Show("item-stub-ore", sim.OrderSell, 3)
	if got, want := w.price.GetText(), "7"; got != want {
		t.Errorf("empty-bid sell price = %q, want the base price %q", got, want)
	}
	// Empty ask side: Request falls back to the base price too.
	cv.orders = []sim.MarketStock{{Item: "item-stub-ore", Price: 90, Qty: 2, Side: sim.OrderBuy}}
	w.Show("item-stub-ore", sim.OrderBuy, 1)
	if got, want := w.price.GetText(), "7"; got != want {
		t.Errorf("empty-ask request price = %q, want the base price %q", got, want)
	}
}

// TestOrderWindowFieldValidation pins the dialog's pre-check only: price
// finite and > 0, qty integer ≥ 1 arm Accept; the live preview shows the
// typed price through theme.FormatCredits; Accept posts PlaceOrder with
// the typed values and closes, Cancel emits nothing.
func TestOrderWindowFieldValidation(t *testing.T) {
	cv, rec, env := orderStub([]sim.MarketStock{
		{Item: "item-stub-ore", Price: 100, Qty: 5, Side: sim.OrderSell},
		{Item: "item-stub-ore", Price: 90, Qty: 2, Side: sim.OrderBuy},
	})
	w := NewOrderWin(env)
	ui := newWindowUI(t, env, w.Win())
	frame := func() { uitest.PumpWindow(t, ui, nil, 1024, 700) }
	w.Show("item-stub-ore", sim.OrderSell, 5)
	frame()

	type set struct {
		price, qty string
		want       bool
	}
	for _, tc := range []set{
		{"91", "5", true},
		{"0", "5", false},   // price must be > 0
		{"-3", "5", false},  // …and positive
		{"abc", "5", false}, // …and a number
		{"1e999", "5", false},
		{"120.5", "0", false}, // qty ≥ 1
		{"120.5", "2.5", false},
		{"120.5", "3", true},
	} {
		w.price.SetText(tc.price)
		w.qty.SetText(tc.qty)
		frame()
		if got := !w.accept.GetWidget().Disabled; got != tc.want {
			t.Errorf("price %q qty %q: accept armed = %v, want %v", tc.price, tc.qty, got, tc.want)
		}
	}
	if got, want := w.preview.Label, theme.FormatCredits(120.5); got != want {
		t.Errorf("preview = %q, want the formatted price %q", got, want)
	}

	// Accept posts the typed values and closes.
	uitest.FireButton(ui, w.accept)
	if len(rec.got) != 1 || rec.got[0] != (widgets.PlaceOrder{
		Item: "item-stub-ore", Side: sim.OrderSell, Price: 120.5, Qty: 3}) {
		t.Fatalf("accept emitted %#v, want the typed PlaceOrder", rec.got)
	}
	if w.IsOpen() {
		t.Error("Accept did not close the window")
	}

	// Cancel emits nothing and closes.
	rec.got = nil
	w.Show("item-stub-ore", sim.OrderSell, 5)
	frame()
	uitest.FireButton(ui, w.cancel)
	if len(rec.got) != 0 {
		t.Fatalf("cancel emitted %#v, want nothing", rec.got)
	}
	if w.IsOpen() {
		t.Error("Cancel did not close the window")
	}

	// Undocked: Show refuses (the menu entry is grayed to match).
	cv.docked = false
	if w.Show("item-stub-ore", sim.OrderSell, 1) {
		t.Error("Show opened while undocked")
	}
}

// TestOrderWindowSingleInstance drives the window through a real Kit:
// a second launch re-targets the one instance (never a second window),
// swaps the mode and its defaults, and reports typing through the same
// seam the game's hotkey gate reads.
func TestOrderWindowSingleInstance(t *testing.T) {
	k, _ := newTestKit(t)
	k.Sim.Docked.Set(k.Sim.Player(), sim.Docked{At: "cel-solace"})
	k.Sync()

	var ids []content.ItemID
	for id := range k.Content.Items {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) < 2 {
		t.Fatal("fixture: need two item ids")
	}
	a, b := ids[0], ids[1]

	k.OpenOrder(a, sim.OrderSell, 4)
	if !k.OrderOpen() {
		t.Fatal("OpenOrder did not open the window")
	}
	if got, want := k.OrderWin().TitleText(), k.Env().TradeableName(a)+" — Order"; got != want {
		t.Errorf("title = %q, want %q", got, want)
	}
	if got, want := k.OrderWin().qty.GetText(), "4"; got != want {
		t.Errorf("sell qty = %q, want %q", got, want)
	}

	// A second launch re-targets the SAME window (one stack entry) and
	// switches to Request with its defaults.
	k.OpenOrder(b, sim.OrderBuy, 1)
	count := 0
	for _, m := range k.stack {
		if m.win == k.OrderWin().Win() {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("order window mounted %d times, want the single instance", count)
	}
	if got, want := k.OrderWin().TitleText(), k.Env().TradeableName(b)+" — Order"; got != want {
		t.Errorf("retargeted title = %q, want %q", got, want)
	}
	if got, want := k.OrderWin().modeBtn.Text().Label, "Request"; got != want {
		t.Errorf("retargeted mode = %q, want %q", got, want)
	}
	if got, want := k.OrderWin().qty.GetText(), "1"; got != want {
		t.Errorf("retargeted qty = %q, want %q", got, want)
	}

	// Typing reports through the seam the game gates on.
	if k.OrderTyping() {
		t.Error("a freshly opened window reads as typing")
	}
	k.OrderWin().FocusPrice()
	if !k.OrderTyping() {
		t.Error("focused price field does not read as typing")
	}
	k.BlurOrder()
	if k.OrderTyping() {
		t.Error("BlurOrder left a field focused")
	}
}
