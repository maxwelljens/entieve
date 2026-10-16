package uikit

import (
	"image"
	"slices"
	"sort"
	"strconv"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
)

// qtySteps are the trade quantity stepper's steps (1 / 10 / 100).
var qtySteps = [...]int{1, 10, 100}

// setStepperValue writes a stepper readout (market/inventory/factory qty)
// and re-lays its row out. widget.Text.Label changes do NOT invalidate
// layout, so without the relayout the readout keeps the box measured for
// its previous value and a wider number draws under the + button
// (Defect B: the readout must resize to fit its current value, so the
// row re-measures it — the label's box grows and the buttons slide
// right). The label carries the tabular contents face, so digit widths
// stay stable while the value steps.
func setStepperValue(bar *widget.Container, lbl *widget.Text, s string) {
	if lbl.Label == s {
		return
	}
	lbl.Label = s
	bar.RequestRelayout()
}

// Market is the station market window: the docked station's order book as
// rows — items ascending, asks (price ascending) above bids (price
// descending) — over a quantity stepper and the Buy/Sell actions, which
// execute as market orders against that book. The title carries the
// selected item's displayed price (last trade, best ask, best bid, base)
// and the fee schedule. Trade is only possible while the player is docked,
// so both buttons are disabled in space.
type Market struct {
	env Env
	win *widget.Window

	tbl      *Table
	qtyLabel *widget.Text
	qtyBar   *widget.Container // stepper row: relaid when the readout changes
	title    *widget.Text
	buy      *widget.Button
	sell     *widget.Button
	minus    *widget.Button
	plus     *widget.Button

	qtyIdx int
	sel    content.ItemID
	hasSel bool
}

// feePct renders a fee rate as a percentage ("3%", "2.5%").
func feePct(rate float64) string {
	return strconv.FormatFloat(rate*100, 'g', -1, 64) + "%"
}

// sortBookOrders puts mixed orders into canonical display order: item
// ascending, asks before bids, asks by price ascending, bids by price
// descending. Stable, so equal prices keep arrival (price-time) order.
func sortBookOrders(orders []sim.MarketStock) {
	sort.SliceStable(orders, func(i, j int) bool {
		a, b := orders[i], orders[j]
		if a.Item != b.Item {
			return a.Item < b.Item
		}
		if a.Side != b.Side {
			return a.Side == sim.OrderSell
		}
		if a.Price != b.Price {
			if a.Side == sim.OrderSell {
				return a.Price < b.Price
			}
			return a.Price > b.Price
		}
		return false
	})
}

// NewMarket builds the Market window over the shared environment. Mount
// it on a Kit with Kit.AddWindow and call Sync every frame.
func NewMarket(env Env) *Market {
	m := &Market{env: env}
	p := env.Pal

	cols := []Column{
		{Key: "Item", Title: "Item", Priority: 0},
		{Key: "Price", Title: "Price", Priority: 1, AlignRight: true},
		{Key: "Qty", Title: "Qty", Priority: 2, AlignRight: true},
		{Key: "Side", Title: "Side", Priority: 3},
	}
	// Contents face: the order book is names, prices and quantities —
	// Source Code Pro lines the columns up (title, toolbar and buttons
	// stay Turret Road).
	m.tbl = NewTable(env, cols, ContentsFace(env.FaceMono(StepSmall)))
	m.tbl.SetSort("Item", true)
	// Sync builds rows in canonical book order; the Item sort key keeps
	// that order (reversing on a second click), any other header click
	// falls back to plain cell sorting.
	m.tbl.SetSortFunc(func(key string, asc bool, rows []TableRow) {
		if key == "Item" {
			if !asc {
				slices.Reverse(rows)
			}
			return
		}
		idx := -1
		for i, c := range cols {
			if c.Key == key {
				idx = i
				break
			}
		}
		if idx < 0 {
			return
		}
		sort.SliceStable(rows, func(a, b int) bool {
			x, y := rows[a].Cells[idx], rows[b].Cells[idx]
			if asc {
				return x < y
			}
			return y < x
		})
	})
	m.tbl.OnSelect(func(r TableRow) {
		if id, ok := r.Data.(content.ItemID); ok {
			m.sel, m.hasSel = id, true
		}
	})
	// Double-click (Show Info) and the right-click menu (Show Info +
	// Buy/Sell) derive from each row's declared RowSpec — built in Sync,
	// where the docked gate and the row's own item are known. The menu's
	// actions carry the clicked row's payload, so Buy/Sell always trade
	// the row they were opened on — no click-time selection side effects.
	m.tbl.OnSelect(func(r TableRow) {
		if id, ok := r.Data.(content.ItemID); ok {
			m.sel, m.hasSel = id, true
		}
	})

	m.qtyLabel = widget.NewText(
		widget.TextOpts.Text("Qty 1", env.FaceMono(StepSmall), solid(p.Accent)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
			Position: widget.RowLayoutPositionCenter,
		})),
	)

	newBtn := func(label string, clicked func()) *widget.Button {
		return widget.NewButton(
			widget.ButtonOpts.TextLabel(label),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(Px(40), Px(40)),
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
			widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) { clicked() }),
		)
	}
	m.minus = newBtn("-", func() { m.stepQty(-1) })
	m.plus = newBtn("+", func() { m.stepQty(1) })
	m.buy = newBtn("Buy", m.buyClicked)
	m.sell = newBtn("Sell", m.sellClicked)

	m.qtyBar = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(Px(contentsPad)),
		)),
	)
	m.qtyBar.AddChild(m.minus, m.qtyLabel, m.plus, m.buy, m.sell)

	// Body: fixed toolbar row over the table row (the table's own
	// preferred height must never grow the window).
	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BG), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, true}),
			widget.GridLayoutOpts.Spacing(0, Px(4)),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(Px(contentsPad))),
		)),
	)
	body.AddChild(m.qtyBar)
	body.AddChild(m.tbl.Container())

	title, marketTitle := NewTitleBar(env, "Market", "svc_market")
	m.title = marketTitle

	m.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, Px(titleBarH)),
		widget.WindowOpts.Draggable(),
		widget.WindowOpts.MinSize(Px(300), Px(240)), // content floor: mirrored in game/station_layout.go
		// Fallback only: the composed rect at the 1024×700 minimum.
		// The game recomposes from the real viewport at startup
		// (game/station_layout.go).
		widget.WindowOpts.Location(image.Rect(Px(396), Px(8), Px(1014), Px(248))),
	)
	return m
}

// Win returns the market window for Kit.AddWindow.
func (m *Market) Win() *widget.Window { return m.win }

// orders returns the orders the window shows: the docked station's own
// book when it offers the market, else every station's book in the
// current system (the undocked fallback view).
func (m *Market) orders() []sim.MarketStock {
	if st := m.env.Sim.DockedMarket(); st != "" {
		return m.env.Sim.BookOrders(st)
	}
	sys := m.env.Sim.System()
	if sys == nil {
		return nil
	}
	return m.env.Sim.Market(sys.ID)
}

// priceOf quotes an item's displayed price for the shown view: the
// docked station's full ladder (last trade, best ask, best bid, base)
// when docked at a market station, else the shown orders' own ladder
// (best ask, then best bid, then the def base price) — the undocked
// fallback view has no station ladder to read.
func (m *Market) priceOf(item content.ItemID) float64 {
	station := m.env.Sim.DockedMarket()
	if station != "" {
		return m.env.Sim.MarketPrice(station, item)
	}
	ask, bid := -1.0, -1.0
	for _, o := range m.orders() {
		if o.Item != item {
			continue
		}
		if o.Side == sim.OrderSell {
			if ask < 0 || o.Price < ask {
				ask = o.Price
			}
		} else if o.Price > bid {
			bid = o.Price
		}
	}
	if ask >= 0 {
		return ask
	}
	if bid >= 0 {
		return bid
	}
	return m.env.Sim.MarketPrice(station, item) // def base price
}

// Sync refreshes the rows, the wallet readout and the docked gating
// from the simulation. Call it every frame before Kit.Update.
func (m *Market) Sync() {
	orders := m.orders()
	sortBookOrders(orders)
	// Trades only happen docked at a station — the same gate feeds the
	// buttons below and every row's declared Buy/Sell actions.
	gated := !m.docked()
	rows := make([]TableRow, 0, len(orders))
	for _, o := range orders {
		side := "Ask"
		if o.Side == sim.OrderBuy {
			side = "Bid"
		}
		item := o.Item
		rows = append(rows, TableRow{
			Cells: []string{
				itemLabel(m.env, o.Item),
				FormatCredits(o.Price),
				FormatInt(int64(o.Qty)),
				side,
			},
			Data: o.Item,
			Spec: RowSpec{
				Ref: m.env.rowRef(o.Item),
				Actions: []RowAction{
					{Label: "Buy", Disabled: gated, Do: func() { m.buyRow(item) }},
					{Label: "Sell", Disabled: gated, Do: func() { m.sellRow(item) }},
				},
			},
		})
	}
	m.tbl.SetRows(rows)
	m.tbl.Sync()

	// Revalidate the cached selection against the table itself: a row
	// that left the board (sold out, station swapped) must stop reading
	// as selected, or Buy/Sell would keep acting on an item the market
	// no longer offers.
	m.hasSel = false
	if r, ok := m.tbl.Selected(); ok {
		if id, isItem := r.Data.(content.ItemID); isItem {
			m.sel, m.hasSel = id, true
		}
	}

	setStepperValue(m.qtyBar, m.qtyLabel, "Qty "+FormatInt(int64(m.qty())))
	sys := m.env.Sim.System()
	if sys != nil {
		label := "Market — " + FormatCredits(m.env.Sim.Wallet())
		if m.hasSel {
			// Selected: the book's price voice for this item (last trade,
			// best ask, best bid, then base) and the fee schedule.
			label = "Market — " + itemLabel(m.env, m.sel) + " " + FormatCredits(m.priceOf(m.sel)) +
				" · fee " + feePct(sim.MarketBrokerFee) + "+" + feePct(sim.MarketSalesTax)
		}
		m.title.Label = label
	}
	// Trades only happen docked at a station (the gate declared above).
	m.buy.GetWidget().Disabled = gated
	m.sell.GetWidget().Disabled = gated
}

// stepQty moves the quantity stepper one step (clamped).
func (m *Market) stepQty(delta int) {
	m.qtyIdx += delta
	if m.qtyIdx < 0 {
		m.qtyIdx = 0
	}
	if m.qtyIdx >= len(qtySteps) {
		m.qtyIdx = len(qtySteps) - 1
	}
	setStepperValue(m.qtyBar, m.qtyLabel, "Qty "+FormatInt(int64(m.qty())))
}

// qty is the current trade quantity.
func (m *Market) qty() int { return qtySteps[m.qtyIdx] }

// docked reports whether the player is docked (the trade precondition).
func (m *Market) docked() bool {
	return m.env.Sim.Docked.Has(m.env.Sim.Player())
}

// buyClicked enqueues CmdBuy for the selected order and quantity (the
// toolbar's own button acts on the left-click selection).
func (m *Market) buyClicked() {
	if !m.docked() || !m.hasSel {
		return
	}
	m.env.Enqueue(sim.CmdBuy{Item: m.sel, Qty: m.qty()})
}

// sellClicked enqueues CmdSell for the selected order and quantity.
func (m *Market) sellClicked() {
	if !m.docked() || !m.hasSel {
		return
	}
	m.env.Enqueue(sim.CmdSell{Item: m.sel, Qty: m.qty()})
}

// buyRow/sellRow are the row menu's trade actions: they act on the row
// the menu was opened on (its item, captured in the row's RowSpec) with
// the toolbar's own docked gate, so a right-clicked row trades itself
// without ever becoming the window's selection.
func (m *Market) buyRow(item content.ItemID) {
	if !m.docked() {
		return
	}
	m.env.Enqueue(sim.CmdBuy{Item: item, Qty: m.qty()})
}

func (m *Market) sellRow(item content.ItemID) {
	if !m.docked() {
		return
	}
	m.env.Enqueue(sim.CmdSell{Item: item, Qty: m.qty()})
}

// itemLabel resolves a market/hangar ID to a display name (items, ships
// and modules are all addressable as IDs; mirrors ui.UI.itemName).
func itemLabel(env Env, id content.ItemID) string {
	if def, ok := env.Content.Item(id); ok {
		return def.Name
	}
	if def, ok := env.Content.Ship(content.ShipID(id)); ok {
		return def.Name
	}
	if def, ok := env.Content.Module(content.ModuleID(id)); ok {
		return def.Name
	}
	return string(id)
}
