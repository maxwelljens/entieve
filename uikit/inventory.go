package uikit

import (
	"image"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
)

// Inventory is the storage window: Cargo / Ore hold / Station hangar as
// Item/Qty/m³ tables with per-section totals, a quantity stepper and
// transfer/sell actions (CmdTransfer / CmdSell).
type Inventory struct {
	env Env
	win *widget.Window

	cargo, ore, hangar *Table

	cargoLbl *widget.Text
	oreLbl   *widget.Text
	hangLbl  *widget.Text
	qtyLabel *widget.Text

	minus, plus *widget.Button
	toCargo     *widget.Button
	toOre       *widget.Button
	toHangar    *widget.Button
	sell        *widget.Button

	qtyIdx int
	sel    content.ItemID
	src    string // hold the selected row belongs to
	hasSel bool
}

// NewInventory builds the Inventory window over the shared environment.
// Mount it on a Kit with Kit.AddWindow and call Sync every frame.
func NewInventory(env Env) *Inventory {
	inv := &Inventory{env: env}
	p := env.Pal

	cols := []Column{
		{Key: "Item", Title: "Item", Priority: 0},
		{Key: "Qty", Title: "Qty", Priority: 1, AlignRight: true},
		{Key: "Vol", Title: "m³", Priority: 2, AlignRight: true},
	}
	// One selection callback per section: the source hold of the next
	// transfer is whichever table the row was clicked in.
	inv.cargo = NewTable(env, cols)
	inv.cargo.OnSelect(inv.selectFrom("cargo"))
	inv.ore = NewTable(env, cols)
	inv.ore.OnSelect(inv.selectFrom("ore"))
	inv.hangar = NewTable(env, cols)
	inv.hangar.OnSelect(inv.selectFrom("hangar"))

	newLabel := func(s string) *widget.Text {
		return widget.NewText(
			widget.TextOpts.Text(s, env.Face(StepTiny), solid(p.Accent)),
		)
	}
	inv.cargoLbl = newLabel("Cargo")
	inv.oreLbl = newLabel("Ore hold")
	inv.hangLbl = newLabel("Station hangar")

	inv.qtyLabel = widget.NewText(
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
	inv.minus = newBtn("-", func() { inv.stepQty(-1) })
	inv.plus = newBtn("+", func() { inv.stepQty(1) })
	inv.toCargo = newBtn("To Cargo", func() { inv.transfer("cargo") })
	inv.toOre = newBtn("To Ore", func() { inv.transfer("ore") })
	inv.toHangar = newBtn("To Hangar", func() { inv.transfer("hangar") })
	inv.sell = newBtn("Sell", inv.sellClicked)

	toolbar := func(children ...widget.PreferredSizeLocateableWidget) *widget.Container {
		c := widget.NewContainer(
			widget.ContainerOpts.Layout(widget.NewRowLayout(
				widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
				widget.RowLayoutOpts.Spacing(contentsPad),
			)),
		)
		c.AddChild(children...)
		return c
	}
	qtyBar := toolbar(inv.minus, inv.qtyLabel, inv.plus, inv.sell)
	xferBar := toolbar(inv.toCargo, inv.toOre, inv.toHangar)

	// Body: label/table pairs over the two button rows; the grid gives
	// each table exactly its cell (bounded viewport, equal thirds).
	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BG), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, true, false, true, false, true, false, false}),
			widget.GridLayoutOpts.Spacing(0, 4),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(contentsPad)),
		)),
	)
	body.AddChild(inv.cargoLbl, inv.cargo.Container(),
		inv.oreLbl, inv.ore.Container(),
		inv.hangLbl, inv.hangar.Container(),
		qtyBar, xferBar)

	title := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BGHard), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)
	title.AddChild(widget.NewText(
		widget.TextOpts.Text("Inventory", env.FaceBold(StepBase), solid(p.Accent)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionStart,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
			Padding:            widget.NewInsetsSimple(contentsPad),
		})),
	))

	inv.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, titleBarH),
		widget.WindowOpts.Draggable(),
		// Content floor: every fixed row plus a full row of each hold
		// section (the sections scroll below that).
		widget.WindowOpts.MinSize(300, 420),
		// Fallback only: the composed rect at the 1024×700 minimum.
		// The game recomposes from the real viewport at startup
		// (game/station_layout.go).
		widget.WindowOpts.Location(image.Rect(396, 256, 701, 690)),
	)
	return inv
}

// Win returns the inventory window for Kit.AddWindow.
func (inv *Inventory) Win() *widget.Window { return inv.win }

// selectFrom builds a per-section selection callback recording the
// selected item and the hold it lives in.
func (inv *Inventory) selectFrom(hold string) func(TableRow) {
	return func(r TableRow) {
		if id, ok := r.Data.(content.ItemID); ok {
			inv.sel, inv.src, inv.hasSel = id, hold, true
		}
	}
}

// tableFor returns the section table inv.src names (the default keeps
// the initial state pointed at cargo, which has no selection).
func (inv *Inventory) tableFor(src string) *Table {
	switch src {
	case "ore":
		return inv.ore
	case "hangar":
		return inv.hangar
	default:
		return inv.cargo
	}
}

// Sync refreshes the three sections, their totals and the action
// gating. Call it every frame before Kit.Update.
func (inv *Inventory) Sync() {
	s := inv.env.Sim
	player := s.Player()

	var cargo *sim.Cargo
	cargo, hasCargo := s.Cargos.Get(player)
	inv.cargo.SetRows(inv.cargoRows(cargo, hasCargo))
	inv.ore.SetRows(inv.oreRows(cargo, hasCargo))
	inv.hangar.SetRows(inv.rowsOf(inv.hangarRows()))

	inv.cargoLbl.Label = inv.section("Cargo", cargo, hasCargo)
	inv.oreLbl.Label = inv.oreSection(cargo, hasCargo)
	inv.hangLbl.Label = "Station hangar · " + FormatInt(int64(len(inv.hangarRows()))) + " stacks"

	inv.qtyLabel.Label = "Qty " + FormatInt(int64(inv.qty()))
	inv.cargo.Sync()
	inv.ore.Sync()
	inv.hangar.Sync()

	// Revalidate the cached selection against the section it came from:
	// a stack that left that hold must stop gating the actions (the
	// click-time cache alone would keep them armed on a row that no
	// longer exists).
	inv.hasSel = false
	if r, ok := inv.tableFor(inv.src).Selected(); ok {
		if id, isItem := r.Data.(content.ItemID); isItem {
			inv.sel, inv.hasSel = id, true
		}
	}

	// Action gating: a selection in the source section (never its own
	// destination), an ore hold for ore, and a docked player for the
	// hangar side and for selling.
	docked := s.Docked.Has(player)
	oreOK := hasCargo && cargo.OreCapacity > 0
	inv.toCargo.GetWidget().Disabled = !inv.hasSel || inv.src == "cargo"
	inv.toOre.GetWidget().Disabled = !inv.hasSel || inv.src == "ore" || !oreOK
	inv.toHangar.GetWidget().Disabled = !inv.hasSel || inv.src == "hangar" || !docked
	inv.sell.GetWidget().Disabled = !inv.hasSel || !docked
}

// cargoRows renders the general cargo hold (empty when the ship has
// none).
func (inv *Inventory) cargoRows(cargo *sim.Cargo, hasCargo bool) []TableRow {
	if !hasCargo {
		return nil
	}
	return inv.rowsOf(cargo.Stacks)
}

// oreRows renders the dedicated ore hold (empty when the hull has none).
func (inv *Inventory) oreRows(cargo *sim.Cargo, hasCargo bool) []TableRow {
	if !hasCargo || cargo.OreCapacity <= 0 {
		return nil
	}
	return inv.rowsOf(cargo.Ore)
}

// hangarRows is the station hangar's stacks (for the section total).
func (inv *Inventory) hangarRows() []content.CargoStack {
	if inv.env.Sim.Hangar == nil {
		return nil
	}
	return inv.env.Sim.Hangar.Items
}

// rowsOf converts stacks to Item/Qty/m³ table rows.
func (inv *Inventory) rowsOf(stacks []content.CargoStack) []TableRow {
	rows := make([]TableRow, 0, len(stacks))
	for _, st := range stacks {
		vol, name := 0.0, itemLabel(inv.env, st.Item)
		if def, ok := inv.env.Content.Item(st.Item); ok {
			vol = def.Volume * float64(st.Qty)
		}
		rows = append(rows, TableRow{
			Cells: []string{name, "×" + FormatInt(int64(st.Qty)), formatVolume(vol)},
			Data:  st.Item,
		})
	}
	return rows
}

// section renders a section header with used/capacity totals.
func (inv *Inventory) section(title string, cargo *sim.Cargo, hasCargo bool) string {
	if !hasCargo {
		return title + " · no cargo hold"
	}
	used := 0.0
	for _, st := range cargo.Stacks {
		if def, ok := inv.env.Content.Item(st.Item); ok {
			used += def.Volume * float64(st.Qty)
		}
	}
	return title + " · " + formatVolume(used) + " / " + formatVolume(cargo.Capacity)
}

// oreSection renders the ore hold header with used/capacity totals.
func (inv *Inventory) oreSection(cargo *sim.Cargo, hasCargo bool) string {
	if !hasCargo || cargo.OreCapacity <= 0 {
		return "Ore hold · not fitted"
	}
	used := 0.0
	for _, st := range cargo.Ore {
		if def, ok := inv.env.Content.Item(st.Item); ok {
			used += def.Volume * float64(st.Qty)
		}
	}
	return "Ore hold · " + formatVolume(used) + " / " + formatVolume(cargo.OreCapacity)
}

// stepQty moves the quantity stepper one step (clamped).
func (inv *Inventory) stepQty(delta int) {
	inv.qtyIdx += delta
	if inv.qtyIdx < 0 {
		inv.qtyIdx = 0
	}
	if inv.qtyIdx >= len(qtySteps) {
		inv.qtyIdx = len(qtySteps) - 1
	}
	inv.qtyLabel.Label = "Qty " + FormatInt(int64(inv.qty()))
}

// qty is the current transfer/sell quantity.
func (inv *Inventory) qty() int { return qtySteps[inv.qtyIdx] }

// transfer moves the selected stack from its section to hold ("cargo",
// "ore" or "hangar") with the stepper quantity.
func (inv *Inventory) transfer(to string) {
	if !inv.hasSel || inv.src == to {
		return
	}
	// The hangar side needs a docked player (sim validates the rest).
	if (inv.src == "hangar" || to == "hangar") && !inv.env.Sim.Docked.Has(inv.env.Sim.Player()) {
		return
	}
	if to == "ore" {
		cargo, ok := inv.env.Sim.Cargos.Get(inv.env.Sim.Player())
		if !ok || cargo.OreCapacity <= 0 {
			return
		}
	}
	inv.env.Enqueue(sim.CmdTransfer{
		Item: inv.sel,
		Qty:  inv.qty(),
		From: inv.src,
		To:   to,
	})
}

// sellClicked enqueues CmdSell for the selected stack (docked only).
func (inv *Inventory) sellClicked() {
	if !inv.hasSel || !inv.env.Sim.Docked.Has(inv.env.Sim.Player()) {
		return
	}
	inv.env.Enqueue(sim.CmdSell{Item: inv.sel, Qty: inv.qty()})
}
