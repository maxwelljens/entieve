package screens

import (
	"image"
	"math"
	"strconv"
	"strings"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// orderMinW/orderMinH are the mini-window's design-px floor (and its
// fallback rect, centred by the host at startup — see game.placeDefaults).
// orderMinH is the measured content requirement (title bar 44 + the padded
// body's 219: the five rows and their ContentsPad gaps) rounded up by one
// ContentsPad of cushion, so device-pixel rounding at higher type scales
// can't clip the last row. Content growth past this fails loudly in
// TestOrderWindowFitsContent / TestOrderWindowOpensCentered.
const (
	orderMinW = 300
	orderMinH = 272
)

// OrderWin is the player market-order mini-window: a small, draggable,
// single-instance dialog that posts ONE standing order on the ship's
// docked station book. It is launched from item context menus ("Sell on
// Market" / "Request on Market") and re-targeted in place by a second
// launch — Show-Info-style; its rect is never persisted (transient) and
// the host closes it when the player undocks.
//
// Two typed fields (ebitenui TextInput — no steppers): PRICE in euro
// (plain decimal typing, e.g. "12345.67") with a live theme.FormatCredits
// preview, and QUANTITY. Accept is armed only while the two fields parse
// (price finite and > 0, qty integer ≥ 1); every other authority (escrow,
// the 3% broker fee, oversell, funds) stays the engine's — it answers with
// its own SevWarning lines. Accept emits the existing PlaceOrder intent.
type OrderWin struct {
	th theme.Theme
	cv widgets.ContentView
	as widgets.ActionSink

	win   *widget.Window
	title *theme.ChromeText
	body  *widget.Container

	modeBtn *widget.Button
	price   *widget.TextInput
	qty     *widget.TextInput
	preview *widget.Text
	accept  *widget.Button
	cancel  *widget.Button

	side    sim.OrderSide
	item    content.ItemID
	sellQty int // the sell mode's default quantity (the clicked stack / holdings)

	priceVal float64
	qtyVal   int
	valid    bool

	onClose func() // Kit-hosted: unmount (a bare env only hides)
}

// NewOrderWin builds the closed mini-window over the given environment.
// It starts hidden and centred by the host; it appears only on demand.
func NewOrderWin(env widgets.Seams) *OrderWin {
	p := env.Pal()
	o := &OrderWin{th: env, cv: env, as: env}

	field := func() *widget.TextInput {
		return widget.NewTextInput(
			widget.TextInputOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(theme.Px(120), theme.Px(28)),
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
		)
	}
	o.price = field()
	o.qty = field()
	o.price.ChangedEvent.AddHandler(func(any) { o.refresh() })
	o.qty.ChangedEvent.AddHandler(func(any) { o.refresh() })

	label := func(s string) *widget.Text {
		return widget.NewText(
			widget.TextOpts.Text(s, env.Face(theme.StepTiny), theme.Solid(p.Accent)),
			widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Position: widget.RowLayoutPositionCenter,
			})),
		)
	}
	o.preview = widget.NewText(
		widget.TextOpts.Text("", env.FaceMono(theme.StepTiny), theme.Solid(p.Dim)),
	)

	newBtn := func(text string, clicked func()) *widget.Button {
		return widget.NewButton(
			widget.ButtonOpts.TextLabel(text),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(theme.Px(40), theme.Px(40)),
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
			widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) { clicked() }),
		)
	}
	o.modeBtn = newBtn("Sell", o.toggleMode)
	o.accept = newBtn("Accept", o.submit)
	o.cancel = newBtn("Cancel", o.Close)

	row := func(children ...widget.PreferredSizeLocateableWidget) *widget.Container {
		c := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.ContentsPad)),
		)))
		c.AddChild(children...)
		return c
	}

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.WindowBodyBox(p, theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200))),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.ContentsPad)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	body.AddChild(o.modeBtn, row(label("Price"), o.price), o.preview, row(label("Qty"), o.qty), row(o.accept, o.cancel))
	o.body = body

	// The title bar carries the base display name; the × close is attached
	// by the kit's close seam (SetWindowClose), like Show Info.
	bar, title := theme.NewTitleBar(env, "Order", "")
	o.title = title

	o.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(bar, theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(orderMinW), theme.Px(orderMinH)),
		// Fallback only: the host centres it for the real viewport. Kept at
		// the floor: Location bypasses SetLocation's MinSize clamp, so a
		// smaller fallback would let the host centre the wrong height.
		widget.WindowOpts.Location(image.Rect(theme.Px(362), theme.Px(240), theme.Px(362+orderMinW), theme.Px(240+orderMinH))),
	)
	theme.ShowWidget(o.win.GetContainer().GetWidget(), false)
	return o
}

// Win returns the window for Kit.AddWindow / the host's hit test.
func (o *OrderWin) Win() *widget.Window { return o.win }

// Show re-targets the window on item, pre-set to side and its sell
// default quantity (qty ≤ 0 falls back to 1), re-applies that mode's
// default price and reveals the window. It reports whether the window can
// open at all: orders rest on the docked station's book, so an undocked
// player opens nothing (the menu entry is grayed too).
func (o *OrderWin) Show(item content.ItemID, side sim.OrderSide, qty int) bool {
	if o.cv == nil || !o.cv.PlayerDocked() {
		return false
	}
	o.item = item
	if qty < 1 {
		qty = 1
	}
	o.sellQty = qty
	// The title is the base display name only (a window title is
	// furniture); the item is not repeated there.
	o.title.Label = "Order"
	o.setMode(side)
	// Open unfocused: the player clicks a field to type, and until then the
	// world hotkeys stay live (the game gates on OrderTyping).
	o.Blur()
	theme.ShowWidget(o.win.GetContainer().GetWidget(), true)
	return true
}

// Close closes the window: through the Kit it also unmounts (so a closed
// window stops gating world input); in a bare environment it just hides.
func (o *OrderWin) Close() {
	if o.onClose != nil {
		o.onClose()
		return
	}
	o.hide()
}

// hide flips the container invisible (the kit's unmount path).
func (o *OrderWin) hide() {
	o.Blur()
	theme.ShowWidget(o.win.GetContainer().GetWidget(), false)
}

// IsOpen reports whether the window is revealed (shown, and mounted when
// a Kit hosts it).
func (o *OrderWin) IsOpen() bool {
	return o.win != nil && o.win.GetContainer().GetWidget().IsVisible()
}

// TitleText is the title-bar label (the base display name).
func (o *OrderWin) TitleText() string {
	if o.title == nil {
		return ""
	}
	return o.title.Label
}

// Typing reports whether a price/qty field holds focus (a visible caret):
// the game yields its world hotkeys to the field while it is true. A
// closed window never reads as typing (a hidden field keeps its focus).
func (o *OrderWin) Typing() bool {
	if !o.IsOpen() {
		return false
	}
	return o.price.IsFocused() || o.qty.IsFocused()
}

// Blur clears focus from both fields (the game's Esc-while-typing: focus
// goes first, closing needs Cancel / the chrome ×).
func (o *OrderWin) Blur() {
	if o.price.IsFocused() {
		o.price.Focus(false)
	}
	if o.qty.IsFocused() {
		o.qty.Focus(false)
	}
}

// FocusPrice focuses the price field (the game's key-gate tests).
func (o *OrderWin) FocusPrice() { o.price.Focus(true) }

// toggleMode flips Sell ↔ Request and re-applies that mode's defaults.
func (o *OrderWin) toggleMode() {
	if o.side == sim.OrderSell {
		o.setMode(sim.OrderBuy)
	} else {
		o.setMode(sim.OrderSell)
	}
}

// setMode applies a mode: the exact button label, the mode's default price
// (Sell = best bid + 1 €, Request = best ask − 1 €, empty side = base
// price) and its default quantity (Sell = the clicked stack, Request = 1).
func (o *OrderWin) setMode(side sim.OrderSide) {
	o.side = side
	if side == sim.OrderSell {
		o.modeBtn.SetText("Sell")
	} else {
		o.modeBtn.SetText("Request")
	}
	o.price.SetText(strconv.FormatFloat(o.defaultPrice(), 'f', -1, 64))
	q := 1
	if side == sim.OrderSell {
		q = o.sellQty
	}
	o.qty.SetText(strconv.Itoa(q))
	o.refresh()
}

// defaultPrice quotes the mode's resting default on the docked book: Sell
// sits at the best bid + 1 €, Request at the best ask − 1 €; a side with
// no order falls back to the item's base/registry price.
func (o *OrderWin) defaultPrice() float64 {
	ask, bid := -1.0, -1.0
	for _, ord := range o.cv.BookOrders(o.cv.DockedMarket()) {
		if ord.Item != o.item {
			continue
		}
		switch ord.Side {
		case sim.OrderSell:
			if ask < 0 || ord.Price < ask {
				ask = ord.Price
			}
		case sim.OrderBuy:
			if ord.Price > bid {
				bid = ord.Price
			}
		}
	}
	if o.side == sim.OrderSell {
		if bid >= 0 {
			return bid + 1
		}
	} else if ask >= 0 {
		return ask - 1
	}
	return orderBasePrice(o.cv, o.item)
}

// orderBasePrice is the item's registry price — the def base price for an
// item, the price for a module (mirrors sim.marketBasePrice; a hull has
// none and reads 0, which the field gate then refuses).
func orderBasePrice(cv widgets.ContentView, item content.ItemID) float64 {
	if cv.Defs() == nil {
		return 0
	}
	if def, ok := cv.Defs().Item(item); ok {
		return def.BasePrice
	}
	if def, ok := cv.Defs().Module(content.ModuleID(item)); ok {
		return def.Price
	}
	return 0
}

// refresh re-parses the typed fields, updates the price preview and arms
// or disarms Accept. It is the dialog's pre-check only — a shape gate, not
// the engine's authority over funds, escrow or holdings.
func (o *OrderWin) refresh() {
	price, okP := parseOrderPrice(o.price.GetText())
	qty, okQ := parseOrderQty(o.qty.GetText())
	o.priceVal, o.qtyVal, o.valid = price, qty, okP && okQ
	if okP {
		o.preview.Label = theme.FormatCredits(price)
	} else {
		o.preview.Label = ""
	}
	o.accept.GetWidget().Disabled = !o.valid
}

// submit emits the PlaceOrder intent the fields describe and closes the
// window (the engine answers a refusal with its own SevWarning).
func (o *OrderWin) submit() {
	o.refresh()
	if !o.valid {
		return
	}
	o.as.Do(widgets.PlaceOrder{Item: o.item, Side: o.side, Price: o.priceVal, Qty: o.qtyVal})
	o.Close()
}

// parseOrderPrice reads a typed euro amount: a finite value > 0.
func parseOrderPrice(s string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
		return 0, false
	}
	return v, true
}

// parseOrderQty reads a typed quantity: an integer ≥ 1.
func parseOrderQty(s string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}
