package screens

import (
	"image"
	"image/color"
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
	// mktBarSteps is the order-book rail's resolution: the table's ScrollTop
	// is a 0..1 fraction of the overflow, the slider an integer over it. The
	// rail itself is widgets.Table's (ScrollRail) — the reserved-gutter
	// scrollbar every table now carries.
	mktBarSteps = widgets.RailSteps

	// mktSideW is the standing-order column's width in design px at the
	// window's landing rect: the My orders list beside the book (which
	// keeps the height it always had). It is the split's opening share,
	// not a fixed width — the seam trades it with the book.
	mktSideW = 240

	// mktDefaultW is the window's landing width (the 546 px square, see
	// the Location below).
	mktDefaultW = 546

	// mktSideShare is the My-orders pane's fraction of the split at that
	// landing width: today's fixed 240 px of the square, body padding and
	// the seam excluded. The seam stores the fraction BEFORE it, so its
	// default ratio is 1-mktSideShare (the book's share).
	mktSideShare = float64(mktSideW) / float64(mktDefaultW-2*theme.ContentsPad-widgets.SplitSeamH)

	// mineWheelRows is how many My-orders rows one wheel notch scrolls.
	// The list is uncapped (every standing order the player has is
	// listed), so it rides a scroll viewport in the standing-order
	// column with the rail's reserved gutter beside it.
	mineWheelRows = 3
)

// setStepperValue writes a stepper readout that is NOT a quantity stepper
// (the settings scale row) and re-lays its row out. widget.Text.Label
// changes do NOT invalidate layout, so without the relayout the readout
// keeps the box measured for its previous value and a wider number draws
// under the + button. The label carries the tabular contents face, so
// digit widths stay stable while the value steps.
func setStepperValue(bar *widget.Container, lbl *widget.Text, s string) {
	if lbl.Label == s {
		return
	}
	lbl.Label = s
	bar.RequestRelayout()
}

// Market is the station market window: the docked station's order book as
// a per-item tree — one parent row per item of the tradeable catalog
// (canonical order), its resting orders folded out beneath it on a click:
// asks (price ascending) above bids (price descending), each order in the
// book's own [price, qty, side] form. Over that sit the quantity stepper
// and the Buy/Sell actions, which execute as market orders against the
// book. The title carries the selected item's displayed price (last trade,
// best ask, best bid, base) and the fee schedule. Trade is only possible
// while the player is docked, so both buttons are disabled in space.
//
// An item with no orders anywhere still gets its row — dimmed, but live:
// placing the first order is how a market wakes up (Request on Market
// works from an empty book). Placing a hull ask is refused by the sim, so
// hull rows trade the way they always did.
//
// Beside the book sits the standing-order column: My orders — only THIS
// station's player orders, each with a Cancel button. It acts on the
// docked book, exactly like Buy/Sell. The column is the second pane of a
// draggable row split (book | seam | orders) and opens where the old fixed
// 240 px column stood. Posting a new standing order is the typed order
// mini-window's job (OrderWin, launched from a row's context menu), not a
// panel here.
type Market struct {
	th  theme.Theme
	cv  widgets.ContentView
	as  widgets.ActionSink
	win *widget.Window

	tbl     *widgets.Table
	stepper *widgets.QtyStepper
	bar     *widget.Slider // order-book scrollbar rail (mirrors ScrollTop)
	title   *widget.Text
	buy     *widget.Button
	sell    *widget.Button

	// split is the book | My-orders row: the seam trades width between
	// the two panes (the old fixed 240 px column is its opening share).
	split *widgets.SplitPane

	// The tree: items is the catalog the parent rows list (built once —
	// content never changes under a session) and open is the fold state
	// (item → its orders shown). open is session-only: it never persists
	// and a closed window forgets it.
	items []content.ItemID
	open  map[content.ItemID]bool

	// My orders: the station's player orders as rich rows, every one of
	// them listed, in a scroll viewport with the house rail beside it.
	mineLbl    *widget.Text
	mineBox    *widget.Container // the rich rows (the scroll content)
	mineScroll *widget.ScrollContainer
	mineRail   *widgets.ScrollRail
	mineRows   []*widgets.StackRow
	mineKey    string // change signature of the My orders rows

	sel    content.ItemID
	hasSel bool
}

// mktNoSide is an item row's Side cell: an item has no side of its own.
// The Qty column already reads 0 for a dead market, so the cell stays a
// dash rather than an empty box.
const mktNoSide = "—"

// mktItemRow is an item's parent row payload: the item the row names. A
// parent row folds its orders out; its child rows carry mktOrderRow.
type mktItemRow struct{ Item content.ItemID }

// mktOrderRow is one resting order's child row payload: the item it trades
// and the order id (the table's row identity across rebuilds).
type mktOrderRow struct {
	Item content.ItemID
	ID   uint64
}

// mktItemOf resolves a market row payload to its item (false when the row
// is not one of the market's).
func mktItemOf(data any) (content.ItemID, bool) {
	switch r := data.(type) {
	case mktItemRow:
		return r.Item, true
	case mktOrderRow:
		return r.Item, true
	}
	return "", false
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

// mktCell is a row's cell text by column index ("" when the row is short).
func mktCell(row widgets.TableRow, i int) string {
	if i < 0 || i >= len(row.Cells) {
		return ""
	}
	return row.Cells[i]
}

// mktIsParent reports whether a row is an item (parent) row.
func mktIsParent(row widgets.TableRow) bool {
	_, ok := row.Data.(mktItemRow)
	return ok
}

// mktBlocks splits the tree's flat rows into one block per item row: the
// parent followed by its folded-out order rows. Rows before the first
// parent (there are none) would form their own block, so a sort can never
// drop a row.
func mktBlocks(rows []widgets.TableRow) [][]widgets.TableRow {
	var out [][]widgets.TableRow
	for _, r := range rows {
		if mktIsParent(r) || len(out) == 0 {
			out = append(out, nil)
		}
		out[len(out)-1] = append(out[len(out)-1], r)
	}
	return out
}

// mktJoin writes the blocks back into rows, block by block: the tree's
// parent-before-its-orders shape survives any header sort.
func mktJoin(rows []widgets.TableRow, blocks [][]widgets.TableRow) {
	at := 0
	for _, b := range blocks {
		at += copy(rows[at:], b)
	}
}

// NewMarket builds the Market window over the shared environment. Mount
// it on a Kit with Kit.AddWindow and call Sync every frame.
func NewMarket(env widgets.Seams) *Market {
	m := &Market{th: env, cv: env, as: env, items: marketCatalog(env), open: map[content.ItemID]bool{}}
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
	// Sync builds rows in canonical book order (each item's parent row,
	// then its orders when the item is open); the Item sort key keeps that
	// order — reversing the ITEMS on a second click — and any other header
	// falls back to sorting the items by that column's cell, never tearing
	// an item's orders away from their parent.
	m.tbl.SetSortFunc(func(key string, asc bool, rows []widgets.TableRow) {
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
		blocks := mktBlocks(rows)
		if key == "Item" {
			if !asc {
				slices.Reverse(blocks)
			}
		} else {
			sort.SliceStable(blocks, func(a, b int) bool {
				x, y := mktCell(blocks[a][0], idx), mktCell(blocks[b][0], idx)
				if asc {
					return x < y
				}
				return y < x
			})
		}
		mktJoin(rows, blocks)
	})
	// A click selects the row's item (the title quotes it, Buy/Sell trade
	// it) and, on an item row, folds its orders out or away. The row's
	// declared RowSpec already carries its own item, so the menu never
	// depends on this selection; a double-click still opens Show Info.
	m.tbl.OnSelect(func(r widgets.TableRow) {
		item, ok := mktItemOf(r.Data)
		if !ok {
			return
		}
		m.sel, m.hasSel = item, true
		if _, isItem := r.Data.(mktItemRow); isItem {
			m.open[item] = !m.open[item]
		}
	})

	m.stepper = widgets.NewQtyStepper(env, qtySteps[:])

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
	m.buy = newBtn("Buy", m.buyClicked)
	m.sell = newBtn("Sell", m.sellClicked)
	m.stepper.Row().AddChild(m.buy, m.sell)

	// The book row: the order table — widgets.Table owns its scroll
	// viewport, its wheel wiring AND the scrollbar rail beside it (the
	// reserved-gutter chrome, ScrollGutter design px: the handle can never
	// overhang a row, and the rail hides while the book fits). The title
	// line and the stepper row above stay exactly as they are.
	m.bar = m.tbl.Rail()
	book := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout()))
	m.tbl.Container().GetWidget().LayoutData = widget.AnchorLayoutData{
		StretchHorizontal: true,
		StretchVertical:   true,
	}
	book.AddChild(m.tbl.Container())

	// My orders: only THIS station's player orders (station-local, the
	// docked book), each a rich row — the item beside its Cancel button on
	// line 1, side/price/remaining on line 2 — tiled by the table module's
	// StackLayout (the contract board's row form). Every order is listed:
	// the rows ride the house scroll viewport (the reserved-gutter rail
	// hides while they fit) once the list outgrows the column.
	m.mineLbl = widget.NewText(
		widget.TextOpts.Text("My orders", env.Face(theme.StepTiny), theme.Solid(p.Accent)),
	)
	m.mineBox = widget.NewContainer(
		widget.ContainerOpts.Layout(widgets.StackLayout{Gap: theme.Px(theme.ContentsPad)}),
	)
	m.mineScroll = widget.NewScrollContainer(
		widget.ScrollContainerOpts.Content(m.mineBox),
		widget.ScrollContainerOpts.StretchContentWidth(),
		// No chrome of the list's own: an invisible frame, the mask
		// clipping the rows to the viewport (the tables' own mask).
		widget.ScrollContainerOpts.Image(&widget.ScrollContainerImage{
			Idle: theme.NineSlice(color.NRGBA{}),
			Mask: theme.NineSlice(theme.WindowBody(p.BG)),
		}),
	)
	// Wheel scrolls the column; ScrollTop is a 0..1 fraction of the
	// overflow, so the step scales by the measured overflow (the tables'
	// convention). The rail beside it drags the same offset.
	m.mineScroll.GetWidget().ScrolledEvent.AddHandler(func(args any) {
		if ev, ok := args.(*widget.WidgetScrolledEventArgs); ok {
			m.scrollMine(ev.Y)
		}
	})
	// Same stash as the tables: the window chrome's translate walk carries
	// the rows with the viewport instead of re-locating them mid-drag.
	widgets.AttachScrollContent(m.mineScroll, m.mineBox)
	m.mineRail = widgets.NewScrollRail(env, m.mineScroll, m.mineScroll, m.mineBox)
	side := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, true}),
			widget.GridLayoutOpts.Spacing(0, theme.Px(theme.ContentsPad)),
		)),
	)
	side.AddChild(m.mineLbl, m.mineRail.Container())

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
	left.AddChild(m.stepper.Row(), book)

	// Body: book | seam | My orders. The row split replaces the old fixed
	// 240 px column with a draggable seam; it opens at today's proportion
	// (the seam stores the book's share, so the orders pane keeps 240 px
	// of the 546 px landing square) and each pane keeps SplitMinPane.
	m.split = widgets.NewSplitPaneRow(env, left, side)
	m.split.SetDefaultRatio(0, 1-mktSideShare)
	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.Bordered(theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	body.AddChild(m.split.Container())

	title, marketTitle := theme.NewTitleBar(env, "Market", "svc_market")
	m.title = marketTitle

	m.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(300), theme.Px(240)), // content floor
		// The window's landing rect — SQUARE: 546 is the largest side
		// that stays right of the rail gutter, fits the 1024×700 minimum,
		// clears the log pocket and the HUD bar (its top edge is 554
		// there, 546 + the 8 px margin) and still lays the body out — the
		// stepper row plus the opening 240 px My-orders share need a
		// 522 px window. The module rack (bottom-right, top edge
		// 425 at the minimum) is the one chrome the square cannot clear:
		// no side ≥ 522 keeps out of both its x range and the log pocket.
		// The game's Market Screen row declares the same rect as its
		// per-type default (game/screens.go), so it opens here on first
		// use, docked and in space alike.
		widget.WindowOpts.Location(image.Rect(theme.Px(396), theme.Px(8), theme.Px(942), theme.Px(554))),
	)
	return m
}

// Win returns the market window for Kit.AddWindow.
func (m *Market) Win() *widget.Window { return m.win }

// Splits names the window's draggable seam for the layout sidecar: the
// My-orders pane right of it.
func (m *Market) Splits() []widgets.Seam {
	return []widgets.Seam{{Name: "market-orders", Split: m.split, Index: 0}}
}

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

// marketCatalog is the market's item universe in canonical order: every
// item (ore, mineral, loot, ammo, charge), module and hull the book can
// list, ascending by id — the order the book always used. Built once; the
// catalog never changes under a session.
func marketCatalog(cv widgets.ContentView) []content.ItemID {
	d := cv.Defs()
	if d == nil {
		return nil
	}
	out := make([]content.ItemID, 0, len(d.Items)+len(d.Modules)+len(d.Ships))
	for id := range d.Items {
		out = append(out, id)
	}
	for id := range d.Modules {
		out = append(out, content.ItemID(id))
	}
	for id := range d.Ships {
		out = append(out, content.ItemID(id))
	}
	slices.Sort(out)
	return out
}

// Sync refreshes the rows, the wallet readout and the docked gating
// from the simulation. Call it every frame before Kit.Update.
func (m *Market) Sync() {
	m.split.Poll()
	orders := m.orders()
	sortBookOrders(orders)
	// Folds are session-only: a window that closed (the kit hides it, see
	// winanim.startClose) forgets what was open and reopens collapsed.
	// shortcut: a hotkey close unmounts without hiding, so that path keeps
	// the folds until the next × close; add a close hook to the Screen seam
	// if a reopened window must always start collapsed.
	if !m.win.GetContainer().GetWidget().IsVisible() {
		clear(m.open)
	}
	// Trades only happen docked at a station — the same gate feeds the
	// buttons below and every row's declared Buy/Sell actions.
	gated := !m.docked()
	// One quote per item for the parent rows, through the seam's ladder:
	// the station's book docked, the shown system's orders in space.
	docked := m.cv.DockedMarket()
	quote := func(item content.ItemID) float64 { return m.cv.PriceLadder(docked, item) }
	by := make(map[content.ItemID][]sim.MarketStock, len(m.items))
	for _, o := range orders {
		by[o.Item] = append(by[o.Item], o)
	}
	rows := make([]widgets.TableRow, 0, len(m.items)+len(orders))
	for _, item := range m.items {
		ords := by[item]
		rows = append(rows, m.itemRow(item, ords, quote(item), gated))
		if !m.open[item] {
			continue
		}
		// sortBookOrders put the item's orders in book order already:
		// asks ascending, then bids descending.
		for _, o := range ords {
			rows = append(rows, m.bookRow(o, gated))
		}
	}
	m.tbl.SetRows(rows)
	m.tbl.Sync()

	// Revalidate the cached selection against the table itself: a row
	// that left the board (sold out, station swapped) must stop reading
	// as selected, or Buy/Sell would keep acting on an item the market
	// no longer offers.
	m.hasSel = false
	if r, ok := m.tbl.Selected(); ok {
		if item, isRow := mktItemOf(r.Data); isRow {
			m.sel, m.hasSel = item, true
		}
	}

	// The station's own standing orders (rebuilt only when they move).
	m.syncMine(gated)

	// The list rides the house rail beside it: mirror the offset onto the
	// handle, and drop a stale offset once the list fits again — the
	// tables' clamp rule (ebitenui would otherwise bottom-anchor it).
	m.mineRail.Sync()
	if m.mineRail.Overflow() <= 0 {
		m.mineScroll.ScrollTop = 0
	}

	sys := m.cv.System()
	if sys != nil {
		label := "Market — " + theme.FormatCredits(m.cv.Wallet())
		if m.hasSel {
			// Selected: the book's price voice for this item (last trade,
			// best ask, best bid, then base) and the fee schedule.
			label = "Market — " + m.cv.TradeableName(m.sel) + " " +
				theme.FormatCredits(m.cv.PriceLadder(docked, m.sel)) +
				" · fee " + feePct(sim.MarketBrokerFee) + "+" + feePct(sim.MarketSalesTax)
		}
		m.title.Label = label
	}
	// Trades only happen docked at a station (the gate declared above).
	m.buy.GetWidget().Disabled = gated
	m.sell.GetWidget().Disabled = gated
	// The scrollbar rail (the table's own) mirrors the table's scroll in
	// m.tbl.Sync above: hidden while the book fits, thumb at ScrollTop
	// otherwise; a drag writes the offset back.
}

// itemRow is one item's parent row: the item, its displayed price (the
// station's ladder: last trade, best ask, best bid, base), the total
// quantity resting on both sides and the dash an item's side cell reads.
// An item with no orders anywhere is dimmed but stays fully actionable —
// placing its first order is how a market wakes up.
func (m *Market) itemRow(item content.ItemID, ords []sim.MarketStock, price float64, gated bool) widgets.TableRow {
	total := 0
	for _, o := range ords {
		total += o.Qty
	}
	row := widgets.TableRow{
		Cells: []string{
			m.cv.TradeableName(item),
			theme.FormatCredits(price),
			theme.FormatInt(int64(total)),
			mktNoSide,
		},
		Data: mktItemRow{Item: item},
		Spec: m.rowSpec(item, gated),
	}
	if len(ords) == 0 {
		row.TextTint = m.th.Pal().Dim
	}
	return row
}

// bookRow is one resting order as its item's child row: the book's own
// [price, qty, side] cells (the item is the parent above it), the
// player's own orders in the owned-row green — one glance separates them
// from the NPC/agent rows (the overview's own-ship/hostile tint
// convention). Selection still wins.
func (m *Market) bookRow(o sim.MarketStock, gated bool) widgets.TableRow {
	side := "Ask"
	if o.Side == sim.OrderBuy {
		side = "Bid"
	}
	var tint color.RGBA
	if o.Owner == sim.OwnerPlayer {
		tint = theme.OwnedRowTint(m.th.Pal())
	}
	return widgets.TableRow{
		Cells: []string{"", theme.FormatCredits(o.Price), theme.FormatInt(int64(o.Qty)), side},
		Tint:  tint,
		Data:  mktOrderRow{Item: o.Item, ID: o.ID},
		Spec:  m.rowSpec(o.Item, gated),
	}
}

// rowSpec is every market row's declared interactions — an item row and
// its order rows offer the same verbs on the same item (Buy/Sell trade
// the book, the two order-window launches post to it); only the payload
// tells the row kinds apart. The menu's actions carry the row's own item,
// so Buy/Sell always trade the row they were opened on — no click-time
// selection side effects.
func (m *Market) rowSpec(item content.ItemID, gated bool) widgets.RowSpec {
	return widgets.RowSpec{
		Ref: widgets.RowRef(m.cv, item),
		Actions: []widgets.RowAction{
			{Label: "Buy", Disabled: gated, Intent: widgets.Buy{Item: item, Qty: m.stepper.Value()}},
			{Label: "Sell", Disabled: gated, Intent: widgets.Sell{Item: item, Qty: m.stepper.Value()}},
			{Label: "Sell on Market", Disabled: gated,
				Intent: widgets.OpenOrder{Item: item, Side: sim.OrderSell, Qty: m.cv.PlayerStock(item)}},
			{Label: "Request on Market", Disabled: gated,
				Intent: widgets.OpenOrder{Item: item, Side: sim.OrderBuy, Qty: 1}},
		},
	}
}

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
	m.as.Do(widgets.Buy{Item: m.sel, Qty: m.stepper.Value()})
}

// sellClicked emits Sell for the selected order and quantity.
func (m *Market) sellClicked() {
	if !m.docked() || !m.hasSel {
		return
	}
	m.as.Do(widgets.Sell{Item: m.sel, Qty: m.stepper.Value()})
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
// docked gate) moved: one rich row per player order at the docked
// station — every order, no cap — its Cancel button the row's trailing
// action, the same gate the book's trade actions carry.
func (m *Market) syncMine(gated bool) {
	orders := m.mineOrders()
	key := mineKey(orders, gated)
	if key == m.mineKey {
		return
	}
	m.mineKey = key
	m.mineBox.RemoveChildren()
	m.mineRows = m.mineRows[:0]
	for _, o := range orders {
		r := m.orderRow(o, gated)
		m.mineRows = append(m.mineRows, r)
		m.mineBox.AddChild(r.Root)
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
		Cells: []string{m.cv.TradeableName(o.Item), side, theme.FormatCredits(o.Price), theme.FormatInt(int64(o.Qty))},
		Data:  o.ID,
		Stack: []widgets.RowLine{
			{Segs: []widgets.RowSeg{{Text: m.cv.TradeableName(o.Item), Step: theme.StepSmall, Tint: p.Accent}}},
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

// scrollMine turns wheel notches into the My orders viewport's offset:
// mineWheelRows rows of the measured row height per notch, the same
// 0..1-fraction-of-overflow convention the tables use (a positive notch
// is wheel up, scrolling towards the top of the list).
func (m *Market) scrollMine(notches float64) {
	over := m.mineRail.Overflow()
	if over <= 0 {
		m.mineScroll.ScrollTop = 0
		return
	}
	rowH := 0
	if len(m.mineRows) > 0 {
		_, rowH = m.mineRows[0].Root.PreferredSize()
	}
	if rowH <= 0 {
		return
	}
	top := m.mineScroll.ScrollTop - notches*mineWheelRows*float64(rowH)/over
	m.mineScroll.ScrollTop = math.Max(0, math.Min(1, top))
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
