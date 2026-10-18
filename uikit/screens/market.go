package screens

import (
	"image"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// qtySteps are the trade quantity stepper's steps (1 / 10 / 100).
var qtySteps = [...]int{1, 10, 100}

const (
	// mktBarW is the order book's scrollbar rail: design px reserved at
	// the table's right edge for the house slider chrome (the theme's
	// SliderParams — the same chrome the log's textarea scrollbar uses).
	// The rail column is reserved even while the book fits, so the
	// viewport never jumps when a long book starts overflowing.
	mktBarW = 8

	// mktBarSteps is the rail's resolution: the table's ScrollTop is a
	// 0..1 fraction of the overflow, the slider an integer over it.
	mktBarSteps = 1000

	// mktSideW is the standing-order column's width in design px: the
	// Place order panel over the My orders list, beside the book (which
	// keeps the height it always had). Composed market windows are ≥560 px
	// wide, leaving the book ≥350 px of columns at the 1024 minimum.
	mktSideW = 240

	// mineMaxRows caps the My orders rows drawn before a "+N more" note:
	// the slot cap is 20, but the docked band is short and a scrolled list
	// is not worth the widget churn (the log keeps every fill).
	mineMaxRows = 3
)

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
//
// Beside the book sits the standing-order column: a Place order panel (a
// Bid/Ask side toggle, a price stepper and the Place button, at the
// toolbar's quantity) and My orders — only THIS station's player orders,
// each with a Cancel button. Both act on the docked book, exactly like
// Buy/Sell.
type Market struct {
	th  theme.Theme
	cv  widgets.ContentView
	as  widgets.ActionSink
	win *widget.Window

	tbl      *widgets.Table
	qtyLabel *widget.Text
	qtyBar   *widget.Container // stepper row: relaid when the readout changes
	bar      *widget.Slider    // order-book scrollbar rail (mirrors ScrollTop)
	title    *widget.Text
	buy      *widget.Button
	sell     *widget.Button
	minus    *widget.Button
	plus     *widget.Button

	// Standing orders: the Place order panel over the My orders list.
	placePanel *widget.Container // side toggle + price stepper + Place
	sideBtn    *widget.Button    // flips the standing side (Bid/Ask)
	placeBtn   *widget.Button
	priceRow   *widget.Container // price stepper row: relaid on readout change
	priceLbl   *widget.Text
	pMinus     *widget.Button
	pPlus      *widget.Button
	mineLbl    *widget.Text
	mineBox    *widget.Container // the station's player orders, as rich rows
	mineRows   []*widgets.StackRow
	mineKey    string // change signature of the My orders rows

	side      sim.OrderSide
	price     float64
	priceItem content.ItemID // item the price was last seeded from

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
func NewMarket(env widgets.Seams) *Market {
	m := &Market{th: env, cv: env, as: env}
	p := env.Pal()

	cols := []widgets.Column{
		{Key: "Item", Title: "Item", Priority: 0},
		{Key: "Price", Title: "Price", Priority: 1, AlignRight: true},
		{Key: "Qty", Title: "Qty", Priority: 2, AlignRight: true},
		{Key: "Side", Title: "Side", Priority: 3},
	}
	// Contents face: the order book is names, prices and quantities —
	// IBM Plex Mono lines the columns up (title, toolbar and buttons
	// stay IBM Plex Sans).
	m.tbl = widgets.NewTable(env, cols, widgets.ContentsFace(env.FaceMono(theme.StepSmall)))
	m.tbl.SetSort("Item", true)
	// Sync builds rows in canonical book order; the Item sort key keeps
	// that order (reversing on a second click), any other header click
	// falls back to plain cell sorting.
	m.tbl.SetSortFunc(func(key string, asc bool, rows []widgets.TableRow) {
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
	m.tbl.OnSelect(func(r widgets.TableRow) {
		if id, ok := r.Data.(content.ItemID); ok {
			m.sel, m.hasSel = id, true
		}
	})
	// Double-click (Show Info) and the right-click menu (Show Info +
	// Buy/Sell) derive from each row's declared RowSpec — built in Sync,
	// where the docked gate and the row's own item are known. The menu's
	// actions carry the clicked row's payload, so Buy/Sell always trade
	// the row they were opened on — no click-time selection side effects.
	m.tbl.OnSelect(func(r widgets.TableRow) {
		if id, ok := r.Data.(content.ItemID); ok {
			m.sel, m.hasSel = id, true
		}
	})

	m.qtyLabel = widget.NewText(
		widget.TextOpts.Text("Qty 1", env.FaceMono(theme.StepSmall), theme.Solid(p.Accent)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
			Position: widget.RowLayoutPositionCenter,
		})),
	)

	newBtn := func(label string, clicked func()) *widget.Button {
		return widget.NewButton(
			widget.ButtonOpts.TextLabel(label),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(theme.Px(40), theme.Px(40)),
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
			widget.RowLayoutOpts.Spacing(theme.Px(theme.ContentsPad)),
		)),
	)
	m.qtyBar.AddChild(m.minus, m.qtyLabel, m.plus, m.buy, m.sell)

	// Place-order panel: a side toggle (Bid/Ask), a price stepper and the
	// Place button. The quantity comes from the toolbar stepper above —
	// the same "Qty" value the immediate Buy/Sell use, so the market has
	// one quantity control. Own container, so the price readout's relayout
	// never disturbs the book or the toolbar.
	newSmallBtn := func(label string, clicked func()) *widget.Button {
		return widget.NewButton(
			widget.ButtonOpts.TextLabel(label),
			widget.ButtonOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(40), theme.Px(40))),
			widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) { clicked() }),
		)
	}
	m.side = sim.OrderBuy
	m.sideBtn = newBtn(sideLabel(m.side), m.toggleSide)
	m.placeBtn = newBtn("Place", m.placeClicked)
	m.pMinus = newSmallBtn("-", func() { m.stepPrice(-1) })
	m.pPlus = newSmallBtn("+", func() { m.stepPrice(1) })
	m.priceLbl = widget.NewText(
		widget.TextOpts.Text(theme.FormatCredits(0), env.FaceMono(theme.StepSmall), theme.Solid(p.Accent)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
			Stretch:  true,
			Position: widget.RowLayoutPositionCenter,
		})),
	)
	m.priceRow = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.ContentsPad)),
		)),
	)
	m.priceRow.AddChild(m.pMinus, m.priceLbl, m.pPlus)
	placeTop := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.ContentsPad)),
		)),
	)
	placeTop.AddChild(m.sideBtn, m.placeBtn)
	m.placePanel = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, false}),
			widget.GridLayoutOpts.Spacing(0, theme.Px(theme.ContentsPad)),
		)),
	)
	m.placePanel.AddChild(placeTop, m.priceRow)

	// The book row: the order table's scroll viewport (widgets.Table
	// already owns the ScrollContainer and the wheel wiring) beside a
	// scrollbar rail on its right edge — the long book's overflow has to
	// be VISIBLE and draggable, which the bare viewport never showed
	// (playtest: "the marketplace window does not have a scrollbar").
	// The title line and the stepper row above stay exactly as they are.
	m.bar = widget.NewSlider(
		widget.SliderOpts.Orientation(widget.DirectionVertical),
		widget.SliderOpts.MinMax(0, mktBarSteps),
		widget.SliderOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(theme.Px(mktBarW), 0),
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				HorizontalPosition: widget.AnchorLayoutPositionEnd,
				StretchVertical:    true,
			}),
		),
		widget.SliderOpts.ChangedHandler(func(a *widget.SliderChangedEventArgs) {
			m.tbl.ScrollForTest().ScrollTop = float64(a.Current) / mktBarSteps
		}),
	)
	// The slider validates against a theme the moment anything measures
	// it — bind the session's widget theme here (the same object the
	// window rides), the way NewStackRow binds it for its buttons.
	m.bar.GetWidget().SetTheme(env.WidgetTheme())

	book := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout()))
	// The table fills the book row except the rail column; the rail
	// anchors to the right edge of the same cell.
	tableRoot := m.tbl.Container().GetWidget()
	tableRoot.LayoutData = widget.AnchorLayoutData{
		StretchHorizontal: true,
		StretchVertical:   true,
		Padding:           &widget.Insets{Right: theme.Px(mktBarW)},
	}
	book.AddChild(m.tbl.Container(), m.bar)

	// My orders: only THIS station's player orders (station-local, the
	// docked book), each a rich row — the item beside its Cancel button on
	// line 1, side/price/remaining on line 2 — tiled by the table module's
	// StackLayout (the contract board's row form).
	m.mineLbl = widget.NewText(
		widget.TextOpts.Text("My orders", env.Face(theme.StepTiny), theme.Solid(p.Accent)),
	)
	m.mineBox = widget.NewContainer(
		widget.ContainerOpts.Layout(widgets.StackLayout{Gap: theme.Px(theme.ContentsPad)}),
		// Cap the column's width: a long item name must not widen the
		// standing-order column and squeeze the book.
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.GridLayoutData{
			MaxWidth: theme.Px(mktSideW),
		})),
	)
	side := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, false, true}),
			widget.GridLayoutOpts.Spacing(0, theme.Px(theme.ContentsPad)),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(mktSideW), 0)),
	)
	side.AddChild(m.placePanel, m.mineLbl, m.mineBox)

	// The standing-order column sits beside the toolbar-and-book column,
	// spanning the full body height (so the My orders list gets the room
	// the book's row would otherwise share). The book keeps the full row
	// height it always had, its scrollbar rail included.
	left := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, true}),
			widget.GridLayoutOpts.Spacing(0, theme.Px(4)),
		)),
	)
	left.AddChild(m.qtyBar, book)

	// Body: the toolbar over the book, with the standing-order column
	// across the whole height (the table's own preferred height must never
	// grow the window).
	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.Bordered(theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Stretch([]bool{true, false}, []bool{true}),
			widget.GridLayoutOpts.Spacing(theme.Px(theme.ContentsPad), 0),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	body.AddChild(left, side)

	title, marketTitle := theme.NewTitleBar(env, "Market", "svc_market")
	m.title = marketTitle

	m.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(300), theme.Px(240)), // content floor: mirrored in game/station_layout.go
		// Fallback only: the composed rect at the 1024×700 minimum.
		// The game recomposes from the real viewport at startup
		// (game/station_layout.go).
		widget.WindowOpts.Location(image.Rect(theme.Px(396), theme.Px(8), theme.Px(1014), theme.Px(248))),
	)
	return m
}

// Win returns the market window for Kit.AddWindow.
func (m *Market) Win() *widget.Window { return m.win }

// orders returns the orders the window shows: the docked station's own
// book when it offers the market, else every station's book in the
// current system (the undocked fallback view).
func (m *Market) orders() []sim.MarketStock {
	if st := m.cv.DockedMarket(); st != "" {
		return m.cv.BookOrders(st)
	}
	sys := m.cv.System()
	if sys == nil {
		return nil
	}
	return m.cv.Market(sys.ID)
}

// priceOf quotes an item's displayed price for the shown view: the
// docked station's full ladder (last trade, best ask, best bid, base)
// when docked at a market station, else the shown orders' own ladder
// (best ask, then best bid, then the def base price) — the undocked
// fallback view has no station ladder to read.
func (m *Market) priceOf(item content.ItemID) float64 {
	station := m.cv.DockedMarket()
	if station != "" {
		return m.cv.MarketPrice(station, item)
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
	return m.cv.MarketPrice(station, item) // def base price
}

// Sync refreshes the rows, the wallet readout and the docked gating
// from the simulation. Call it every frame before Kit.Update.
func (m *Market) Sync() {
	orders := m.orders()
	sortBookOrders(orders)
	// Trades only happen docked at a station — the same gate feeds the
	// buttons below and every row's declared Buy/Sell actions.
	gated := !m.docked()
	rows := make([]widgets.TableRow, 0, len(orders))
	for _, o := range orders {
		side := "Ask"
		if o.Side == sim.OrderBuy {
			side = "Bid"
		}
		item := o.Item
		rows = append(rows, widgets.TableRow{
			Cells: []string{
				itemLabel(m.cv, o.Item),
				theme.FormatCredits(o.Price),
				theme.FormatInt(int64(o.Qty)),
				side,
			},
			Data: o.Item,
			Spec: widgets.RowSpec{
				Ref: widgets.RowRef(m.cv, o.Item),
				Actions: []widgets.RowAction{
					{Label: "Buy", Disabled: gated, Intent: widgets.Buy{Item: item, Qty: m.qty()}},
					{Label: "Sell", Disabled: gated, Intent: widgets.Sell{Item: item, Qty: m.qty()}},
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

	setStepperValue(m.qtyBar, m.qtyLabel, "Qty "+theme.FormatInt(int64(m.qty())))

	// Place-order panel: seed the price from the selected item's displayed
	// price when the selection changes (a fresh quote to undercut or
	// overcut), then refresh the readouts and the gate.
	if m.hasSel && m.sel != m.priceItem {
		m.priceItem = m.sel
		m.price = m.cv.MarketPrice(m.cv.DockedMarket(), m.sel)
	}
	setStepperValue(m.priceRow, m.priceLbl, theme.FormatCredits(m.price))
	m.placeBtn.GetWidget().Disabled = gated || !m.hasSel || m.price <= 0

	// The station's own standing orders (rebuilt only when they move).
	m.syncMine(gated)

	sys := m.cv.System()
	if sys != nil {
		label := "Market — " + theme.FormatCredits(m.cv.Wallet())
		if m.hasSel {
			// Selected: the book's price voice for this item (last trade,
			// best ask, best bid, then base) and the fee schedule.
			label = "Market — " + itemLabel(m.cv, m.sel) + " " + theme.FormatCredits(m.priceOf(m.sel)) +
				" · fee " + feePct(sim.MarketBrokerFee) + "+" + feePct(sim.MarketSalesTax)
		}
		m.title.Label = label
	}
	// Trades only happen docked at a station (the gate declared above).
	m.buy.GetWidget().Disabled = gated
	m.sell.GetWidget().Disabled = gated

	// The scrollbar rail mirrors the table's scroll: hidden while the
	// book fits its viewport (no overflow → ScrollTop pinned 0), thumb
	// at ScrollTop otherwise. Wheel input reaches the table's own
	// scroll container; dragging the rail writes the offset back.
	theme.ShowWidget(m.bar.GetWidget(), m.bookOverflow() > 0)
	if cur := int(math.Round(m.tbl.ScrollForTest().ScrollTop * mktBarSteps)); m.bar.Current != cur {
		m.bar.Current = cur
	}
}

// bookOverflow is the order book's scrollable distance in px: the laid
// out rows' union minus the scroll viewport (≤ 0 while the book fits).
// Row rects come from the last layout pass, so a first Sync before any
// draw reports no overflow and the rail stays hidden.
func (m *Market) bookOverflow() float64 {
	rows := m.tbl.RowCts
	if len(rows) == 0 {
		return 0
	}
	union := rows[0].GetWidget().Rect
	for _, r := range rows[1:] {
		union = union.Union(r.GetWidget().Rect)
	}
	return float64(union.Dy() - m.tbl.ScrollForTest().ViewRect().Dy())
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
	setStepperValue(m.qtyBar, m.qtyLabel, "Qty "+theme.FormatInt(int64(m.qty())))
}

// qty is the current trade quantity.
func (m *Market) qty() int { return qtySteps[m.qtyIdx] }

// docked reports whether the player is docked (the trade precondition).
func (m *Market) docked() bool {
	return m.cv.PlayerDocked()
}

// buyClicked emits Buy for the selected order and quantity (the
// toolbar's own button acts on the left-click selection). The row menu's
// trade verbs ride each row's RowSpec intents — same gate, same payload,
// on the row they were opened on.
func (m *Market) buyClicked() {
	if !m.docked() || !m.hasSel {
		return
	}
	m.as.Do(widgets.Buy{Item: m.sel, Qty: m.qty()})
}

// sellClicked emits Sell for the selected order and quantity.
func (m *Market) sellClicked() {
	if !m.docked() || !m.hasSel {
		return
	}
	m.as.Do(widgets.Sell{Item: m.sel, Qty: m.qty()})
}

// mineOrders returns the player's resting orders at the docked station —
// the one book the market window shows (station-local, exactly like the
// trade buttons).
func (m *Market) mineOrders() []sim.MarketStock {
	st := m.cv.DockedMarket()
	if st == "" {
		return nil
	}
	var out []sim.MarketStock
	for _, o := range m.cv.BookOrders(st) {
		if o.Owner == sim.OwnerPlayer {
			out = append(out, o)
		}
	}
	return out
}

// syncMine rebuilds the My orders rows only when their content (or the
// docked gate) moved: one rich row per player order at the docked station,
// its Cancel button the row's trailing action — the same gate the book's
// trade actions carry.
func (m *Market) syncMine(gated bool) {
	orders := m.mineOrders()
	key := mineKey(orders, gated)
	if key == m.mineKey {
		return
	}
	m.mineKey = key
	m.mineBox.RemoveChildren()
	m.mineRows = m.mineRows[:0]
	show := len(orders)
	if show > mineMaxRows {
		show = mineMaxRows
	}
	for _, o := range orders[:show] {
		m.mineRows = append(m.mineRows, m.orderRow(o, gated))
	}
	if hidden := len(orders) - show; hidden > 0 {
		m.mineRows = append(m.mineRows, m.noteRow("+"+strconv.Itoa(hidden)+" more"))
	}
	if len(orders) == 0 {
		m.mineLbl.Label = "My orders — none"
	} else {
		m.mineLbl.Label = "My orders — " + theme.FormatInt(int64(len(orders)))
	}
}

// orderRow is one of the player's orders as a rich row: the item beside
// the Cancel button on line 1, the side, price and remaining quantity on
// line 2.
func (m *Market) orderRow(o sim.MarketStock, gated bool) *widgets.StackRow {
	p := m.th.Pal()
	side, tint := "Ask", p.Warn
	if o.Side == sim.OrderBuy {
		side, tint = "Bid", p.Info
	}
	row := widgets.TableRow{
		Cells: []string{itemLabel(m.cv, o.Item), side, theme.FormatCredits(o.Price), theme.FormatInt(int64(o.Qty))},
		Data:  o.ID,
		Stack: []widgets.RowLine{
			{Segs: []widgets.RowSeg{{Text: itemLabel(m.cv, o.Item), Step: theme.StepSmall, Tint: p.Accent}}},
			{Segs: []widgets.RowSeg{
				{Text: side, Tint: tint, Step: theme.StepTiny},
				{Text: theme.FormatCredits(o.Price), Step: theme.StepTiny},
				{Text: "×" + theme.FormatInt(int64(o.Qty)), Step: theme.StepTiny},
			}},
		},
		Spec: widgets.RowSpec{Actions: []widgets.RowAction{
			{Label: "Cancel", Disabled: gated, Intent: widgets.CancelOrder{ID: o.ID}},
		}},
	}
	return widgets.NewStackRow(widgets.NewSeams(m.th, m.cv, m.as), row, m.th.Face(theme.StepSmall))
}

// noteRow is a dim rich row with no verbs (the "+N more" overflow note).
func (m *Market) noteRow(text string) *widgets.StackRow {
	p := m.th.Pal()
	row := widgets.TableRow{Stack: []widgets.RowLine{{Segs: []widgets.RowSeg{{
		Text: text, Tint: p.Dim, Step: theme.StepTiny,
	}}}}}
	return widgets.NewStackRow(widgets.NewSeams(m.th, m.cv, m.as), row, m.th.Face(theme.StepSmall))
}

// mineKey is the My orders section's change signature: the docked gate
// plus every order's id/side/price/remaining (the book is canonical, so
// the signature is stable across equal states).
func mineKey(orders []sim.MarketStock, gated bool) string {
	var b strings.Builder
	// Non-empty even with no orders, so the first Sync always builds (the
	// zero-value key must never read as "unchanged").
	b.WriteString("m;")
	if gated {
		b.WriteString("g;")
	}
	for _, o := range orders {
		b.WriteString(strconv.FormatUint(o.ID, 10))
		b.WriteByte(':')
		b.WriteString(string(o.Side))
		b.WriteByte(':')
		b.WriteString(strconv.FormatFloat(o.Price, 'g', -1, 64))
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(o.Qty))
		b.WriteByte(';')
	}
	return b.String()
}

// stepPrice moves the standing order's price one tick at its current
// magnitude (never to zero or below). The sim quantizes the price onto the
// same tick grid at placement, so what the readout shows is what rests.
func (m *Market) stepPrice(delta int) {
	step := sim.TickStep(m.price)
	if step <= 0 {
		step = 0.01 // no seed price yet: the smallest tick
	}
	p := m.price + float64(delta)*step
	if p < step {
		p = step
	}
	m.price = p
	setStepperValue(m.priceRow, m.priceLbl, theme.FormatCredits(m.price))
}

// toggleSide flips the standing order's side between bid and ask.
func (m *Market) toggleSide() {
	if m.side == sim.OrderBuy {
		m.side = sim.OrderSell
	} else {
		m.side = sim.OrderBuy
	}
	m.sideBtn.SetText(sideLabel(m.side))
}

// sideLabel is a standing-order side's display name: EVE's bid (buy) and
// ask (sell).
func sideLabel(s sim.OrderSide) string {
	if s == sim.OrderSell {
		return "Ask"
	}
	return "Bid"
}

// placeClicked posts the standing order the panel describes: the selected
// item, the panel's side and price, the toolbar's quantity. Guarded even
// when called directly (the market buttons' rule).
func (m *Market) placeClicked() {
	if !m.docked() || !m.hasSel || m.price <= 0 {
		return
	}
	m.as.Do(widgets.PlaceOrder{Item: m.sel, Side: m.side, Price: m.price, Qty: m.qty()})
}

// itemLabel resolves a market/hangar ID to a display name (items, ships
// and modules are all addressable as IDs; mirrors ui.UI.itemName).
func itemLabel(cv widgets.ContentView, id content.ItemID) string {
	if def, ok := cv.Defs().Item(id); ok {
		return def.Name
	}
	if def, ok := cv.Defs().Ship(content.ShipID(id)); ok {
		return def.Name
	}
	if def, ok := cv.Defs().Module(content.ModuleID(id)); ok {
		return def.Name
	}
	return string(id)
}
