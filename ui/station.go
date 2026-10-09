package ui

import (
	"image/color"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// stationTabNames labels the station services tabs.
var stationTabNames = [numStationTabs]string{"Repair", "Market", "Refine", "Fitting + Hangar"}

// ---------------------------------------------------------------------------
// Pure station sub-layout (shared by Draw and click hit-testing)
// ---------------------------------------------------------------------------

// MarketTable returns the table area below the quantity stepper.
func MarketTable(body Rect) Rect { return Rect{body.X, body.Y + 26, body.W, body.H - 26} }

// StepperRects returns the [-] and [+] quantity stepper buttons.
func StepperRects(body Rect) (minus, plus Rect) {
	return Rect{body.Right() - 116, body.Y, 26, 22},
		Rect{body.Right() - 26, body.Y, 26, 22}
}

// MarketBuyRect / MarketSellRect are the per-row trade buttons.
func MarketBuyRect(row Rect) Rect  { return Rect{row.Right() - 94, row.Y + 3, 44, row.H - 6} }
func MarketSellRect(row Rect) Rect { return Rect{row.Right() - 46, row.Y + 3, 44, row.H - 6} }

// RepairButton is the repair action button at the bottom of its tab.
func RepairButton(body Rect) Rect {
	return Rect{body.X + body.W/2 - 90, body.Bottom() - 32, 180, 28}
}

// StationSlotRow is one fitted-slot row in the Fitting + Hangar tab.
func StationSlotRow(body Rect, row int) Rect {
	return Rect{body.X, body.Y + float64(row)*fitSlotRowH, body.W, fitSlotRowH - 4}
}

// StationHangarHeaderY / StationHangarRow place the hangar list.
func StationHangarHeaderY(body Rect) float64 {
	return body.Y + 3*fitSlotRowH + 6
}

// StationHangarRow returns hangar ship row i.
func StationHangarRow(body Rect, i int) Rect {
	return Rect{body.X, StationHangarHeaderY(body) + 18 + float64(i)*26, body.W, 24}
}

// StationItemsHeaderY / StationItemRow place the hangar item list.
func StationItemsHeaderY(body Rect) float64 {
	return StationHangarRow(body, 5).Bottom() + 6
}

// StationItemRow returns hangar item row i.
func StationItemRow(body Rect, i int) Rect {
	return Rect{body.X, StationItemsHeaderY(body) + 16 + float64(i)*18, body.W, 17}
}

// StationItemMaxRows returns how many hangar item rows fit.
func StationItemMaxRows(body Rect) int {
	n := int((body.Bottom() - StationItemsHeaderY(body) - 20) / 18)
	if n < 0 {
		return 0
	}
	return n
}

// stationName resolves the docked station's display name.
func (u *UI) stationName(s *sim.Sim) (string, bool) {
	d, ok := s.Docked.Get(s.Player())
	if !ok {
		return "", false
	}
	if sys := s.System(); sys != nil {
		for i := range sys.Celestials {
			if sys.Celestials[i].ID == d.At {
				return sys.Celestials[i].Name, true
			}
		}
	}
	return string(d.At), true
}

// itemName resolves a market/hangar ID to a display name (items, ships and
// modules all addressable as IDs).
func (u *UI) itemName(id content.ItemID) string {
	if def, ok := u.c.Item(id); ok {
		return def.Name
	}
	if def, ok := u.c.Ship(content.ShipID(id)); ok {
		return def.Name
	}
	if def, ok := u.c.Module(content.ModuleID(id)); ok {
		return def.Name
	}
	return string(id)
}

// ---------------------------------------------------------------------------
// Station services panel
// ---------------------------------------------------------------------------

// drawStation draws the docked station services window (tabbed).
func (u *UI) drawStation(dst *ebiten.Image, s *sim.Sim, reg Regions) {
	r := reg.Station
	Panel(dst, r)
	title := "Station Services"
	if name, ok := u.stationName(s); ok {
		title = "Station — " + name
	}
	PanelTitle(dst, r, title)

	for i, tr := range TabRects(r, numStationTabs) {
		selected := u.stationTab == i
		bg := ColCell
		border := ColBorder
		fg := ColDim
		if selected {
			bg = withAlpha(ColAccent, 40)
			border = ColAccent
			fg = ColAccent
		}
		vector.DrawFilledRect(dst, float32(tr.X), float32(tr.Y), float32(tr.W), float32(tr.H), bg, false)
		vector.StrokeRect(dst, float32(tr.X)+0.5, float32(tr.Y)+0.5, float32(tr.W)-1, float32(tr.H)-1,
			1, border, false)
		TextCentered(dst, stationTabNames[i], tr.X+tr.W/2, tr.Y+6, 11, fg)
	}

	body := StationBody(r)
	switch u.stationTab {
	case tabRepair:
		u.drawRepairTab(dst, s, body)
	case tabMarket:
		u.drawMarketTab(dst, s, body)
	case tabRefine:
		u.drawRefineTab(dst, s, body)
	default:
		u.drawHangarTab(dst, s, body)
	}
}

// drawRepairTab shows layer HP and the repair action.
func (u *UI) drawRepairTab(dst *ebiten.Image, s *sim.Sim, body Rect) {
	player := s.Player()
	y := body.Y
	Text(dst, "Repair all damage to shield, armor and structure.", body.X, y, 11, ColText)
	y += 24
	if sh, ok := s.Ships.Get(player); ok {
		layers := [][4]float64{
			{sh.Shield, sh.MaxShield, 0, 0},
			{sh.Armor, sh.MaxArmor, 0, 0},
			{sh.Structure, sh.MaxStructure, 0, 0},
		}
		names := [3]string{"Shield", "Armor", "Structure"}
		cols := [3]color.RGBA{ColShield, ColWarn, ColStruct}
		for i, l := range layers {
			Text(dst, names[i], body.X, y+2, 11, cols[i])
			mr := Rect{body.X + 70, y + 3, body.W - 240, 12}
			f := 1.0
			if l[1] > 0 {
				f = clamp(l[0]/l[1], 0, 1)
			}
			Meter(dst, mr, f, cols[i])
			TextRight(dst, intStr(int64(l[0]))+" / "+intStr(int64(l[1])), body.Right(), y+2, 11, ColDim)
			y += 24
		}
	} else {
		Text(dst, "No ship information.", body.X, y, 11, ColDim)
	}
	Button(dst, RepairButton(body), "Repair (fee applies)", true)
	Text(dst, "Available at stations with the repair service.", body.X, RepairButton(body).Bottom()+8, 10, ColDim)
}

// drawMarketTab lists station market orders with a quantity stepper.
func (u *UI) drawMarketTab(dst *ebiten.Image, s *sim.Sim, body Rect) {
	minus, plus := StepperRects(body)
	Button(dst, minus, "-", true)
	Button(dst, plus, "+", true)
	Text(dst, "Qty "+intStr(int64(u.marketQty())), plus.X-76, body.Y+5, 11, ColAccent)
	TextRight(dst, FormatCredits(u.walletOrZero(s)), body.X+body.W-126, body.Y+5, 11, ColGood)

	table := MarketTable(body)
	head := StationHeadRect(table)
	vector.DrawFilledRect(dst, float32(head.X), float32(head.Y), float32(head.W), float32(head.H),
		withAlpha(ColBGHard, 180), false)
	Text(dst, "ITEM", head.X+4, head.Y+5, 10, ColHead)
	TextRight(dst, "PRICE", head.X+head.W*0.58, head.Y+5, 10, ColHead)
	TextRight(dst, "REMAINING", head.X+head.W*0.74, head.Y+5, 10, ColHead)

	sys := s.System()
	if sys == nil {
		return
	}
	max := StationMaxRows(table)
	for i, o := range sys.Market {
		if i >= max {
			break
		}
		row := StationRowRect(table, i)
		Text(dst, ellipsize(u.itemName(o.Item), head.W*0.42, 11), row.X+4, row.Y+5, 11, ColText)
		TextRight(dst, FormatCredits(o.Price), row.X+head.W*0.58, row.Y+5, 11, ColText)
		TextRight(dst, intStr(int64(o.Qty)), row.X+head.W*0.74, row.Y+5, 11, ColDim)
		Button(dst, MarketBuyRect(row), "Buy", true)
		Button(dst, MarketSellRect(row), "Sell", true)
	}
	if len(sys.Market) == 0 {
		Text(dst, "No market orders in this system.", table.X+4, table.Y+stHeadH+4, 11, ColDim)
	}
}

// drawRefineTab lists refineable ore in cargo with a quantity stepper.
func (u *UI) drawRefineTab(dst *ebiten.Image, s *sim.Sim, body Rect) {
	minus, plus := StepperRects(body)
	Button(dst, minus, "-", true)
	Button(dst, plus, "+", true)
	Text(dst, "Qty "+intStr(int64(u.marketQty())), plus.X-76, body.Y+5, 11, ColAccent)

	table := MarketTable(body)
	head := StationHeadRect(table)
	vector.DrawFilledRect(dst, float32(head.X), float32(head.Y), float32(head.W), float32(head.H),
		withAlpha(ColBGHard, 180), false)
	Text(dst, "ORE", head.X+4, head.Y+5, 10, ColHead)
	TextRight(dst, "IN HOLD", head.X+head.W*0.55, head.Y+5, 10, ColHead)

	cargo, ok := s.Cargos.Get(s.Player())
	if !ok {
		return
	}
	max := StationMaxRows(table)
	shown := 0
	for _, st := range cargo.Stacks {
		def, ok := u.c.Item(st.Item)
		if !ok || len(def.RefineYield) == 0 {
			continue
		}
		if shown >= max {
			break
		}
		row := StationRowRect(table, shown)
		shown++
		Text(dst, def.Name, row.X+4, row.Y+5, 11, ColText)
		TextRight(dst, intStr(int64(st.Qty)), row.X+head.W*0.55, row.Y+5, 11, ColDim)
		Button(dst, RightButton(row, 70), "Refine", true)
	}
	if shown == 0 {
		Text(dst, "No refineable ore in cargo.", table.X+4, table.Y+stHeadH+4, 11, ColDim)
	}
}

// drawHangarTab shows fitted slots, hangar ships and stored items.
func (u *UI) drawHangarTab(dst *ebiten.Image, s *sim.Sim, body Rect) {
	player := s.Player()
	classes := [modRows]string{content.SlotHigh, content.SlotMid, content.SlotLow}
	labels := [modRows]string{"High", "Mid", "Low"}
	maxCells := FittingMaxCells(body)

	if fit, ok := s.Fittings.Get(player); ok {
		for row := 0; row < modRows; row++ {
			rr := StationSlotRow(body, row)
			Text(dst, labels[row], rr.X, rr.Y+9, 10, ColHead)
			defs := fit.Defs(classes[row])
			for i := 0; i < len(defs) && i < maxCells; i++ {
				cell := FittingCellRect(rr, i)
				def, _ := u.c.Module(defs[i])
				bg, border := ColCell, ColAccent
				if def == nil {
					border = ColBorder
				}
				vector.DrawFilledRect(dst, float32(cell.X), float32(cell.Y), float32(cell.W), float32(cell.H),
					bg, false)
				vector.StrokeRect(dst, float32(cell.X)+0.5, float32(cell.Y)+0.5,
					float32(cell.W)-1, float32(cell.H)-1, 1, border, false)
				if def != nil {
					TextCentered(dst, moduleLetter(def.Name), cell.X+cell.W/2, cell.Y+7, 13, moduleAccent(def.Kind))
					Button(dst, FittingUnfitRect(cell), "x", true)
				}
			}
		}
	}

	Text(dst, "Hangar", body.X, StationHangarHeaderY(body), 11, ColAccent)
	if s.Hangar != nil {
		if len(s.Hangar.Ships) == 0 {
			Text(dst, "No ships in hangar.", body.X+4, StationHangarHeaderY(body)+18, 10, ColDim)
		}
		for i, ss := range s.Hangar.Ships {
			if i >= 5 {
				break
			}
			row := StationHangarRow(body, i)
			name := string(ss.Hull)
			if def, ok := u.c.Ship(ss.Hull); ok {
				name = def.Name
			}
			Text(dst, name, row.X+4, row.Y+5, 11, ColText)
			Text(dst, intStr(int64(len(ss.Fit)))+" modules", row.X+150, row.Y+6, 9, ColDim)
			Button(dst, RightButton(row, 84), "Assemble", true)
		}
	}

	Text(dst, "Hangar items", body.X, StationItemsHeaderY(body), 11, ColAccent)
	if s.Hangar != nil {
		if len(s.Hangar.Items) == 0 {
			Text(dst, "empty", body.X+4, StationItemsHeaderY(body)+18, 10, ColDim)
		}
		for i, st := range s.Hangar.Items {
			if i >= StationItemMaxRows(body) {
				break
			}
			row := StationItemRow(body, i)
			Text(dst, u.itemName(st.Item), row.X+4, row.Y+3, 10, ColText)
			TextRight(dst, "×"+intStr(int64(st.Qty)), row.Right()-48, row.Y+3, 10, ColDim)
			// Modules can be fitted into the first free slot of their class.
			if _, ok := u.c.Module(content.ModuleID(st.Item)); ok && st.Qty > 0 {
				Button(dst, RightButton(row, 44), "Fit", true)
			}
		}
	}
}

// walletOrZero returns the player's balance.
func (u *UI) walletOrZero(s *sim.Sim) float64 {
	if s == nil {
		return 0
	}
	return s.Wallet()
}

// ---------------------------------------------------------------------------
// Station clicks
// ---------------------------------------------------------------------------

// clickStation dispatches clicks inside the station services panel.
func (u *UI) clickStation(x, y float64, reg Regions) {
	r := reg.Station
	if ti := TabAt(r, numStationTabs, x, y); ti >= 0 {
		u.stationTab = ti
		return
	}
	body := StationBody(r)
	switch u.stationTab {
	case tabRepair:
		if RepairButton(body).Contains(x, y) {
			u.enqueue(sim.CmdRepair{})
		}
	case tabMarket:
		u.clickStepper(body, x, y)
		table := MarketTable(body)
		if i := StationRowAt(table, y); i >= 0 {
			sys := u.s.System()
			if sys == nil || i >= len(sys.Market) {
				return
			}
			row := StationRowRect(table, i)
			item := sys.Market[i].Item
			qty := u.marketQty()
			switch {
			case MarketBuyRect(row).Contains(x, y):
				u.enqueue(sim.CmdBuy{Item: item, Qty: qty})
			case MarketSellRect(row).Contains(x, y):
				u.enqueue(sim.CmdSell{Item: item, Qty: qty})
			}
		}
	case tabRefine:
		u.clickStepper(body, x, y)
		table := MarketTable(body)
		if i := StationRowAt(table, y); i >= 0 {
			row := StationRowRect(table, i)
			if RightButton(row, 70).Contains(x, y) {
				if item, ok := u.refineItemAt(i); ok {
					u.enqueue(sim.CmdRefine{Item: item, Qty: u.marketQty()})
				}
			}
		}
	case tabFitting:
		u.clickHangarTab(body, x, y)
	}
}

// clickStepper handles the [-]/[+] quantity buttons.
func (u *UI) clickStepper(body Rect, x, y float64) {
	minus, plus := StepperRects(body)
	if minus.Contains(x, y) && u.marketQtyIdx > 0 {
		u.marketQtyIdx--
	}
	if plus.Contains(x, y) && u.marketQtyIdx < len(marketQtySteps)-1 {
		u.marketQtyIdx++
	}
}

// refineItemAt maps a visible refine row to its ore item ID.
func (u *UI) refineItemAt(row int) (content.ItemID, bool) {
	cargo, ok := u.s.Cargos.Get(u.s.Player())
	if !ok {
		return "", false
	}
	shown := 0
	for _, st := range cargo.Stacks {
		def, ok := u.c.Item(st.Item)
		if !ok || len(def.RefineYield) == 0 {
			continue
		}
		if shown == row {
			return st.Item, true
		}
		shown++
	}
	return "", false
}

// clickHangarTab handles unfit and assemble clicks.
func (u *UI) clickHangarTab(body Rect, x, y float64) {
	player := u.s.Player()
	classes := [modRows]string{content.SlotHigh, content.SlotMid, content.SlotLow}
	if fit, ok := u.s.Fittings.Get(player); ok {
		for row := 0; row < modRows; row++ {
			rr := StationSlotRow(body, row)
			if y < rr.Y || y >= rr.Bottom() {
				continue
			}
			for i, id := range fit.Defs(classes[row]) {
				_ = id
				cell := FittingCellRect(rr, i)
				if FittingUnfitRect(cell).Contains(x, y) {
					u.enqueue(sim.CmdUnfitModule{Slot: sim.SlotRef{Class: classes[row], Index: i}})
					return
				}
			}
		}
	}
	if u.s.Hangar != nil {
		for i, ss := range u.s.Hangar.Ships {
			if i >= 5 {
				break
			}
			if StationHangarRow(body, i).Contains(x, y) {
				u.enqueue(sim.CmdAssemble{Ship: ss.Hull})
				return
			}
		}
		for i, st := range u.s.Hangar.Items {
			if i >= StationItemMaxRows(body) {
				break
			}
			row := StationItemRow(body, i)
			if !RightButton(row, 44).Contains(x, y) {
				continue
			}
			def, ok := u.c.Module(content.ModuleID(st.Item))
			if !ok || st.Qty <= 0 {
				continue
			}
			if slot, ok := u.firstEmptySlot(def.Slot); ok {
				u.enqueue(sim.CmdFitModule{Item: def.ID, Slot: slot})
			}
			return
		}
	}
}

// firstEmptySlot finds the lowest free slot index of the given class on the
// player's hull (nil when the hull is full).
func (u *UI) firstEmptySlot(class string) (sim.SlotRef, bool) {
	player := u.s.Player()
	ship, ok := u.s.Ships.Get(player)
	if !ok {
		return sim.SlotRef{}, false
	}
	fit, ok := u.s.Fittings.Get(player)
	if !ok {
		return sim.SlotRef{}, false
	}
	hull, ok := u.c.Ship(ship.Hull)
	if !ok {
		return sim.SlotRef{}, false
	}
	capacity := hull.Slots.High
	switch class {
	case content.SlotMid:
		capacity = hull.Slots.Mid
	case content.SlotLow:
		capacity = hull.Slots.Low
	}
	used := fit.Defs(class)
	if len(used) >= capacity {
		return sim.SlotRef{}, false
	}
	return sim.SlotRef{Class: class, Index: len(used)}, true
}
