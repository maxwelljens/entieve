package uikit

import (
	"image"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
)

// qtySteps are the trade quantity stepper's steps (1 / 10 / 100).
var qtySteps = [...]int{1, 10, 100}

// Market is the station market window: an Item/Price/Qty table over a
// quantity stepper and the Buy/Sell actions. Trade is only possible
// while the player is docked, so both buttons are disabled in space.
type Market struct {
	env Env
	win *widget.Window

	tbl      *Table
	qtyLabel *widget.Text
	title    *widget.Text
	buy      *widget.Button
	sell     *widget.Button
	minus    *widget.Button
	plus     *widget.Button

	qtyIdx int
	sel    content.ItemID
	hasSel bool
}

// NewMarket builds the Market window over the shared environment. Mount
// it on a Kit with Kit.AddWindow and call Sync every frame.
func NewMarket(env Env) *Market {
	m := &Market{env: env}
	p := env.Pal

	m.tbl = NewTable(env, []Column{
		{Key: "Item", Title: "Item", Priority: 0},
		{Key: "Price", Title: "Price", Priority: 1, AlignRight: true},
		{Key: "Qty", Title: "Qty", Priority: 2, AlignRight: true},
	})
	m.tbl.SetSort("Item", true)
	m.tbl.OnSelect(func(r TableRow) {
		if id, ok := r.Data.(content.ItemID); ok {
			m.sel, m.hasSel = id, true
		}
	})

	m.qtyLabel = widget.NewText(
		widget.TextOpts.Text("Qty 1", env.Face(StepSmall), solid(p.Accent)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
			Position: widget.RowLayoutPositionCenter,
		})),
	)

	newBtn := func(label string, clicked func()) *widget.Button {
		return widget.NewButton(
			widget.ButtonOpts.TextLabel(label),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(0, 40),
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
			widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) { clicked() }),
		)
	}
	m.minus = newBtn("-", func() { m.stepQty(-1) })
	m.plus = newBtn("+", func() { m.stepQty(1) })
	m.buy = newBtn("Buy", m.buyClicked)
	m.sell = newBtn("Sell", m.sellClicked)

	toolbar := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(contentsPad),
		)),
	)
	toolbar.AddChild(m.minus, m.qtyLabel, m.plus, m.buy, m.sell)

	// Body: fixed toolbar row over the table row (the table's own
	// preferred height must never grow the window).
	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BG), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, true}),
			widget.GridLayoutOpts.Spacing(0, contentsPad),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(contentsPad)),
		)),
	)
	body.AddChild(toolbar)
	body.AddChild(m.tbl.Container())

	title := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BGHard), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)
	m.title = widget.NewText(
		widget.TextOpts.Text("Market", env.Face(StepBase), solid(p.Accent)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionStart,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
			Padding:            widget.NewInsetsSimple(contentsPad),
		})),
	)
	title.AddChild(m.title)

	m.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, titleBarH),
		widget.WindowOpts.Draggable(),
		widget.WindowOpts.MinSize(300, 240),
		widget.WindowOpts.Location(image.Rect(396, 8, 696, 248)),
	)
	makeEdgeResizable(m.win) // four-edge resize (replaces WindowOpts.Resizeable)
	return m
}

// Win returns the market window for Kit.AddWindow.
func (m *Market) Win() *widget.Window { return m.win }

// Sync refreshes the rows, the wallet readout and the docked gating
// from the simulation. Call it every frame before Kit.Update.
func (m *Market) Sync() {
	sys := m.env.Sim.System()
	var stock []sim.MarketStock
	if sys != nil {
		stock = m.env.Sim.Market(sys.ID)
	}
	rows := make([]TableRow, 0, len(stock))
	for _, st := range stock {
		rows = append(rows, TableRow{
			Cells: []string{
				itemLabel(m.env, st.Item),
				formatCredits(st.Price),
				groupInt(int64(st.Qty)),
			},
			Data: st.Item,
		})
	}
	m.tbl.SetRows(rows)
	m.tbl.Sync()

	m.qtyLabel.Label = "Qty " + groupInt(int64(m.qty()))
	if sys != nil {
		m.title.Label = "Market — " + formatCredits(m.env.Sim.Wallet())
	}
	// Trades only happen docked at a station.
	gated := !m.docked()
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
	m.qtyLabel.Label = "Qty " + groupInt(int64(m.qty()))
}

// qty is the current trade quantity.
func (m *Market) qty() int { return qtySteps[m.qtyIdx] }

// docked reports whether the player is docked (the trade precondition).
func (m *Market) docked() bool {
	return m.env.Sim.Docked.Has(m.env.Sim.Player())
}

// buyClicked enqueues CmdBuy for the selected order and quantity.
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
