package screens

import (
	"image"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// Inventory is the storage window: Cargo / Ore hold / Station hangar as
// Item/Qty/m³ tables with per-section totals, a quantity stepper and
// transfer/sell actions (CmdTransfer / CmdSell).
type Inventory struct {
	th  theme.Theme
	cv  widgets.ContentView
	as  widgets.ActionSink
	win *widget.Window

	cargo, ore, hangar *widgets.Table
	split              *widgets.SplitPane

	cargoLbl *widget.Text
	oreLbl   *widget.Text
	hangLbl  *widget.Text
	stepper  *widgets.QtyStepper

	toCargo  *widget.Button
	toOre    *widget.Button
	toHangar *widget.Button
	sell     *widget.Button

	sel    content.ItemID
	src    string // hold the selected row belongs to
	hasSel bool
}

// NewInventory builds the Inventory window over the shared environment.
// Mount it on a Kit with Kit.AddWindow and call Sync every frame.
func NewInventory(env widgets.Seams) *Inventory {
	inv := &Inventory{th: env, cv: env, as: env}
	p := env.Pal()

	cols := []widgets.Column{
		{Key: "Item", Title: "Item", Priority: 0},
		{Key: "Qty", Title: "Qty", Priority: 1, AlignRight: true},
		{Key: "Vol", Title: "m³", Priority: 2, AlignRight: true},
	}
	// One selection callback per section: the source hold of the next
	// transfer is whichever table the row was clicked in. Contents face:
	// these rows are names, quantities and volumes — IBM Plex Mono
	// lines the columns up (section labels and buttons stay IBM Plex Sans).
	mono := widgets.ContentsFace(env.FaceMono(theme.StepSmall))
	inv.cargo = widgets.NewTable(env, cols, mono, widgets.PrefsID("inventory"))
	inv.cargo.OnSelect(inv.selectFrom("cargo"))
	inv.ore = widgets.NewTable(env, cols, mono, widgets.PrefsID("inventory"))
	inv.ore.OnSelect(inv.selectFrom("ore"))
	inv.hangar = widgets.NewTable(env, cols, mono, widgets.PrefsID("inventory"))
	inv.hangar.OnSelect(inv.selectFrom("hangar"))
	// Inventory rows carry no other action: their RowSpec declares the
	// Show Info ref only (built in rowsOf), so double-click swaps the
	// Show Info window and right-click opens the one-entry menu — through
	// the installer, not hand wiring.

	newLabel := func(s string) *widget.Text {
		return widget.NewText(
			widget.TextOpts.Text(s, env.Face(theme.StepTiny), theme.Solid(p.Accent)),
		)
	}
	inv.cargoLbl = newLabel("Cargo")
	inv.oreLbl = newLabel("Ore hold")
	inv.hangLbl = newLabel("Station hangar")

	inv.stepper = widgets.NewQtyStepper(env, qtySteps[:])

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
	inv.toCargo = newBtn("To Cargo", func() { inv.transfer("cargo") })
	inv.toOre = newBtn("To Ore", func() { inv.transfer("ore") })
	inv.toHangar = newBtn("To Hangar", func() { inv.transfer("hangar") })
	inv.sell = newBtn("Sell", inv.sellClicked)

	toolbar := func(children ...widget.PreferredSizeLocateableWidget) *widget.Container {
		c := widget.NewContainer(
			widget.ContainerOpts.Layout(widget.NewRowLayout(
				widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
				widget.RowLayoutOpts.Spacing(theme.Px(theme.ContentsPad)),
			)),
		)
		c.AddChild(children...)
		return c
	}
	inv.stepper.Row().AddChild(inv.sell)
	xferBar := toolbar(inv.toCargo, inv.toOre, inv.toHangar)

	// Body: the three label/table panes over the two button rows. The
	// panes split the leftover height by seam ratio (equal thirds by
	// default — a drag persists in the layout sidecar).
	inv.split = widgets.NewSplitPane(env,
		paneStack([]bool{false, true}, inv.cargoLbl, inv.cargo.Container()),
		paneStack([]bool{false, true}, inv.oreLbl, inv.ore.Container()),
		paneStack([]bool{false, true}, inv.hangLbl, inv.hangar.Container()),
	)
	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.WindowBodyBox(p, theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200))),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true, false, false}),
			widget.GridLayoutOpts.Spacing(0, theme.Px(4)),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	body.AddChild(inv.split.Container(), inv.stepper.Row(), xferBar)

	title, _ := theme.NewTitleBar(env, "Inventory", "svc_inventory")

	inv.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, theme.Px(theme.TitleBarH)),
		// Content floor: every fixed row plus a full row of each hold
		// section (the sections scroll below that).
		widget.WindowOpts.MinSize(theme.Px(300), theme.Px(420)),
		// The game's Inventory Screen row declares the same rect as its
		// per-type default (game/screens.go), so it opens here on first
		// use, docked and in space alike.
		widget.WindowOpts.Location(image.Rect(theme.Px(396), theme.Px(256), theme.Px(701), theme.Px(690))),
	)
	return inv
}

// Win returns the inventory window for Kit.AddWindow.
func (inv *Inventory) Win() *widget.Window { return inv.win }

// Splits names the window's draggable seams for the layout sidecar: each
// key names the hold pane below its seam.
func (inv *Inventory) Splits() []widgets.Seam {
	return []widgets.Seam{
		{Name: "inventory-ore", Split: inv.split, Index: 0},
		{Name: "inventory-hangar", Split: inv.split, Index: 1},
	}
}

// Tables are the window's tables for the layout sidecar. The three holds
// share one column set (they are the same three columns over the same
// holds), so a width dragged in one of them applies to all three.
func (inv *Inventory) Tables() []*widgets.Table {
	return []*widgets.Table{inv.cargo, inv.ore, inv.hangar}
}

// paneStack stacks one split pane's rows in a single column: the rows
// marked stretch take the pane's leftover height (its scroll body), the
// rest keep their preferred height (labels, toolbars). Each body keeps
// its own ScrollRail gutter.
func paneStack(stretch []bool, rows ...widget.PreferredSizeLocateableWidget) *widget.Container {
	c := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewGridLayout(
		widget.GridLayoutOpts.Columns(1),
		widget.GridLayoutOpts.Stretch([]bool{true}, stretch),
		widget.GridLayoutOpts.Spacing(0, theme.Px(4)),
	)))
	c.AddChild(rows...)
	return c
}

// selectFrom builds a per-section selection callback recording the
// selected item and the hold it lives in.
func (inv *Inventory) selectFrom(hold string) func(widgets.TableRow) {
	return func(r widgets.TableRow) {
		if id, ok := r.Data.(content.ItemID); ok {
			inv.sel, inv.src, inv.hasSel = id, hold, true
		}
	}
}

// tableFor returns the section table inv.src names (the default keeps
// the initial state pointed at cargo, which has no selection).
func (inv *Inventory) tableFor(src string) *widgets.Table {
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
	inv.split.Poll()
	s := inv.cv
	player := s.Player()

	var cargo sim.Cargo
	cargo, hasCargo := s.Cargo(player)
	inv.cargo.SetRows(inv.cargoRows(cargo, hasCargo))
	inv.ore.SetRows(inv.oreRows(cargo, hasCargo))
	inv.hangar.SetRows(inv.rowsOf(inv.hangarRows()))

	inv.cargoLbl.Label = inv.section("Cargo")
	inv.oreLbl.Label = inv.oreSection()
	inv.hangLbl.Label = "Station hangar · " + theme.FormatInt(int64(len(inv.hangarRows()))) + " stacks"

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
	docked := s.PlayerDocked()
	oreOK := hasCargo && cargo.OreCapacity > 0
	inv.toCargo.GetWidget().Disabled = !inv.hasSel || inv.src == "cargo"
	inv.toOre.GetWidget().Disabled = !inv.hasSel || inv.src == "ore" || !oreOK
	inv.toHangar.GetWidget().Disabled = !inv.hasSel || inv.src == "hangar" || !docked
	inv.sell.GetWidget().Disabled = !inv.hasSel || !docked
}

// cargoRows renders the general cargo hold (empty when the ship has
// none).
func (inv *Inventory) cargoRows(cargo sim.Cargo, hasCargo bool) []widgets.TableRow {
	if !hasCargo {
		return nil
	}
	return inv.rowsOf(cargo.Stacks)
}

// oreRows renders the dedicated ore hold (empty when the hull has none).
func (inv *Inventory) oreRows(cargo sim.Cargo, hasCargo bool) []widgets.TableRow {
	if !hasCargo || cargo.OreCapacity <= 0 {
		return nil
	}
	return inv.rowsOf(cargo.Ore)
}

// hangarRows is the station hangar's stacks (for the section total).
func (inv *Inventory) hangarRows() []content.CargoStack {
	return inv.cv.HangarItems()
}

// rowsOf converts stacks to Item/Qty/m³ table rows, each declaring the
// full-stack "Sell on Market" verb (grayed, never hidden, while undocked —
// orders rest on the docked station's book). The action carries the
// clicked stack's own quantity as the order window's sell default.
func (inv *Inventory) rowsOf(stacks []content.CargoStack) []widgets.TableRow {
	rows := make([]widgets.TableRow, 0, len(stacks))
	docked := inv.cv.PlayerDocked()
	for _, st := range stacks {
		vol, name := 0.0, inv.cv.TradeableName(st.Item)
		if def, ok := inv.cv.Defs().Item(st.Item); ok {
			vol = def.Volume * float64(st.Qty)
		}
		rows = append(rows, widgets.TableRow{
			Cells: []string{name, "×" + theme.FormatInt(int64(st.Qty)), theme.FormatVolumeShort(vol)},
			Data:  st.Item,
			// The whole interaction surface: Show Info, plus the sell verb.
			Spec: widgets.RowSpec{
				Ref: widgets.RowRef(inv.cv, st.Item),
				Actions: []widgets.RowAction{
					{Label: "Sell on Market", Disabled: !docked,
						Intent: widgets.OpenOrder{Item: st.Item, Side: sim.OrderSell, Qty: st.Qty}},
				},
			},
		})
	}
	return rows
}

// section renders the general hold's header with used/capacity totals.
func (inv *Inventory) section(title string) string {
	used, cap, ok := inv.cv.Hold("cargo")
	if !ok {
		return title + " · no cargo hold"
	}
	return title + " · " + theme.FormatVolumeShort(used) + " / " + theme.FormatVolumeShort(cap)
}

// oreSection renders the ore hold header with used/capacity totals.
func (inv *Inventory) oreSection() string {
	used, cap, ok := inv.cv.Hold("ore")
	if !ok {
		return "Ore hold · not fitted"
	}
	return "Ore hold · " + theme.FormatVolumeShort(used) + " / " + theme.FormatVolumeShort(cap)
}

// transfer moves the selected stack from its section to hold ("cargo",
// "ore" or "hangar") with the stepper quantity.
func (inv *Inventory) transfer(to string) {
	if !inv.hasSel || inv.src == to {
		return
	}
	// The hangar side needs a docked player (sim validates the rest).
	if (inv.src == "hangar" || to == "hangar") && !inv.cv.PlayerDocked() {
		return
	}
	if to == "ore" {
		cargo, ok := inv.cv.Cargo(inv.cv.Player())
		if !ok || cargo.OreCapacity <= 0 {
			return
		}
	}
	inv.as.Do(widgets.Transfer{
		Item: inv.sel,
		Qty:  inv.stepper.Value(),
		From: inv.src,
		To:   to,
	})
}

// sellClicked emits Sell for the selected stack (docked only).
func (inv *Inventory) sellClicked() {
	if !inv.hasSel || !inv.cv.PlayerDocked() {
		return
	}
	inv.as.Do(widgets.Sell{Item: inv.sel, Qty: inv.stepper.Value()})
}
