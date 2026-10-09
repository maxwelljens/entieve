package ui

import (
	"image/color"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// stationTabNames labels the station services tabs.
var stationTabNames = [numStationTabs]string{"Repair", "Market", "Refine", "Fitting + Hangar"}

// hangarVisibleShips is how many hangar ships fit at the minimum window
// size (the rest are reported as "+ N more" on the header line).
const hangarVisibleShips = 3

// ---------------------------------------------------------------------------
// Pure station sub-layout (shared by Draw and click hit-testing)
// ---------------------------------------------------------------------------

// MarketTable returns the table area below the quantity stepper.
func MarketTable(body Rect) Rect {
	off := minHit + padSmall
	return Rect{body.X, body.Y + off, body.W, body.H - off}
}

// StepperRects returns the [-] and [+] quantity stepper buttons (40×40).
func StepperRects(body Rect) (minus, plus Rect) {
	plus = Rect{body.Right() - minHit, body.Y, minHit, minHit}
	minus = Rect{body.Right() - 2*minHit - padSmall, body.Y, minHit, minHit}
	return minus, plus
}

// QtyAnchorX is the fixed left anchor of the "Qty N" stepper label.
func QtyAnchorX(minus Rect) float64 { return minus.X - padLarge - 80 }

// WalletAnchor is the fixed right anchor for the wallet readout (left of
// the qty label): right-aligned so digits never shift it.
func WalletAnchor(minus Rect) float64 {
	return NumberAnchor(Rect{W: QtyAnchorX(minus) - pad})
}

// MarketBuyRect / MarketSellRect are the per-row trade buttons (40 px
// hit height: full minHit even though the row band is slimmer).
func MarketBuyRect(row Rect) Rect {
	return Rect{row.Right() - 2*minHit - padSmall, row.Y, minHit, minHit}
}
func MarketSellRect(row Rect) Rect {
	return Rect{row.Right() - minHit, row.Y, minHit, minHit}
}

// RepairButton is the repair action button at the bottom of its tab.
func RepairButton(body Rect) Rect {
	return Rect{body.X + body.W/2 - 90, body.Bottom() - minHit - padSmall, 180, minHit}
}

// StationSlotRow is one fitted-slot row in the Fitting + Hangar tab.
func StationSlotRow(body Rect, row int) Rect {
	return Rect{body.X, body.Y + float64(row)*fitSlotRowH, body.W, fitSlotRowH - pad}
}

// Hangar row pitch (rows carry a 40 px Assemble button).
const (
	hangarPitch = minHit + padSmall
	itemPitch   = minHit + padSmall
)

// StationHangarHeaderY places the hangar list header.
func StationHangarHeaderY(body Rect) float64 {
	return body.Y + 3*fitSlotRowH + pad
}

// StationHangarRow returns hangar ship row i.
func StationHangarRow(body Rect, i int) Rect {
	return Rect{
		X: body.X,
		Y: StationHangarHeaderY(body) + lineH(FontSmall) + float64(i)*hangarPitch,
		W: body.W,
		H: minHit,
	}
}

// StationItemsHeaderY places the hangar item list.
func StationItemsHeaderY(body Rect) float64 {
	return StationHangarRow(body, hangarVisibleShips).Bottom() + padSmall
}

// StationItemRow returns hangar item row i.
func StationItemRow(body Rect, i int) Rect {
	return Rect{
		X: body.X,
		Y: StationItemsHeaderY(body) + lineH(FontSmall) + float64(i)*itemPitch,
		W: body.W,
		H: minHit,
	}
}

// StationItemMaxRows returns how many hangar item rows fit.
func StationItemMaxRows(body Rect) int {
	itemsY := StationItemsHeaderY(body) + lineH(FontSmall)
	n := int((body.Bottom() - pad - itemsY) / itemPitch)
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
	r := u.WindowRect(WinStation, reg)
	title := "Station Services"
	if name, ok := u.stationName(s); ok {
		title = "Station — " + name
	}
	DrawWindow(dst, r, title, false)

	for i, tr := range TabRects(r, numStationTabs) {
		selected := u.stationTab == i
		hover := tr.Contains(u.mx, u.my)
		bg := ColCell
		border := ColBorder
		fg := ColDim
		if selected {
			bg = withAlpha(ColAccent, 40)
			border = ColAccent
			fg = ColAccent
		} else if hover {
			bg = withAlpha(ColHover, 16) // hover: subtle accent tint
			border = withAlpha(ColAccent, 160)
			fg = ColText
		}
		vector.DrawFilledRect(dst, float32(tr.X), float32(tr.Y), float32(tr.W), float32(tr.H), bg, false)
		vector.StrokeRect(dst, float32(tr.X)+0.5, float32(tr.Y)+0.5, float32(tr.W)-1, float32(tr.H)-1,
			1, border, false)
		TextCentered(dst, stationTabNames[i], tr.X+tr.W/2,
			tr.Y+(tr.H-lineH(FontSmall))/2, FontSmall, fg)
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
	Text(dst, "Repair all damage to shield, armor and structure.", body.X, y, FontSmall, ColText)
	y += lineH(FontSmall) + padSmall
	if sh, ok := s.Ships.Get(player); ok {
		layers := [][4]float64{
			{sh.Shield, sh.MaxShield, 0, 0},
			{sh.Armor, sh.MaxArmor, 0, 0},
			{sh.Structure, sh.MaxStructure, 0, 0},
		}
		names := [3]string{"Shield", "Armor", "Structure"}
		cols := [3]color.RGBA{ColShield, ColWarn, ColStruct}
		labelW := render.Measure("Structure", FontSmall) + padSmall
		rowH := lineH(FontSmall) + padSmall
		for i, l := range layers {
			Text(dst, names[i], body.X, y, FontSmall, cols[i])
			mr := Rect{body.X + labelW, y + (lineH(FontSmall)-13)/2, body.W - labelW - 140, 13}
			f := 1.0
			if l[1] > 0 {
				f = clamp(l[0]/l[1], 0, 1)
			}
			Meter(dst, mr, f, cols[i])
			TextRight(dst, intStr(int64(l[0]))+" / "+intStr(int64(l[1])),
				NumberAnchor(Rect{X: body.X, W: body.W}), y, FontSmall, ColDim)
			y += rowH
		}
	} else {
		Text(dst, "No ship information.", body.X, y, FontSmall, ColDim)
	}
	u.button(dst, RepairButton(body), "Repair (fee applies)", u.needsRepair())
	Text(dst, "Available at stations with the repair service.", body.X,
		RepairButton(body).Bottom()+padSmall, FontSmall, ColDim)
}

// needsRepair reports whether any hull layer is damaged (repair enabled).
func (u *UI) needsRepair() bool {
	if u.s == nil {
		return false
	}
	sh, ok := u.s.Ships.Get(u.s.Player())
	if !ok {
		return false
	}
	return sh.Shield < sh.MaxShield || sh.Armor < sh.MaxArmor ||
		sh.Structure < sh.MaxStructure
}

// drawMarketTab lists station market orders with a quantity stepper.
func (u *UI) drawMarketTab(dst *ebiten.Image, s *sim.Sim, body Rect) {
	minus, plus := StepperRects(body)
	u.button(dst, minus, "-", true)
	u.button(dst, plus, "+", true)
	Text(dst, "Qty "+intStr(int64(u.marketQty())), QtyAnchorX(minus), body.Y+padSmall,
		FontSmall, ColAccent)
	TextRight(dst, FormatCredits(u.walletOrZero(s)), WalletAnchor(minus), body.Y+padSmall,
		FontSmall, ColGood)

	table := MarketTable(body)
	head := StationHeadRect(table)
	vector.DrawFilledRect(dst, float32(head.X), float32(head.Y), float32(head.W), float32(head.H),
		withAlpha(ColBGHard, 180), false)
	headY := head.Y + (head.H-lineH(FontSmall))/2
	priceAnchor := NumberAnchor(Rect{W: head.X + head.W*0.58})
	qtyAnchor := NumberAnchor(Rect{W: head.X + head.W*0.74})
	Text(dst, "ITEM", head.X+padSmall, headY, FontSmall, ColHead)
	TextRight(dst, "PRICE", priceAnchor, headY, FontSmall, ColHead)
	TextRight(dst, "REMAINING", qtyAnchor, headY, FontSmall, ColHead)

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
		rowY := row.Y + (row.H-lineH(FontSmall))/2
		Text(dst, ellipsize(u.itemName(o.Item), priceAnchor-row.X-padSmall, FontSmall),
			row.X+padSmall, rowY, FontSmall, ColText)
		TextRight(dst, FormatCredits(o.Price), priceAnchor, rowY, FontSmall, ColText)
		TextRight(dst, intStr(int64(o.Qty)), qtyAnchor, rowY, FontSmall, ColDim)
		qty := u.marketQty()
		u.button(dst, MarketBuyRect(row), "Buy", o.Qty >= qty)
		u.button(dst, MarketSellRect(row), "Sell", u.cargoCount(o.Item) >= qty)
	}
	if len(sys.Market) == 0 {
		Text(dst, "No market orders in this system.", table.X+padSmall,
			table.Y+stHeadH+padSmall, FontSmall, ColDim)
	}
}

// cargoCount returns how many units of an item the player carries.
func (u *UI) cargoCount(item content.ItemID) int {
	if u.s == nil {
		return 0
	}
	cargo, ok := u.s.Cargos.Get(u.s.Player())
	if !ok {
		return 0
	}
	total := 0
	for _, st := range cargo.Stacks {
		if st.Item == item {
			total += st.Qty
		}
	}
	return total
}

// drawRefineTab lists refineable ore in cargo with a quantity stepper.
func (u *UI) drawRefineTab(dst *ebiten.Image, s *sim.Sim, body Rect) {
	minus, plus := StepperRects(body)
	u.button(dst, minus, "-", true)
	u.button(dst, plus, "+", true)
	Text(dst, "Qty "+intStr(int64(u.marketQty())), QtyAnchorX(minus), body.Y+padSmall,
		FontSmall, ColAccent)

	table := MarketTable(body)
	head := StationHeadRect(table)
	vector.DrawFilledRect(dst, float32(head.X), float32(head.Y), float32(head.W), float32(head.H),
		withAlpha(ColBGHard, 180), false)
	headY := head.Y + (head.H-lineH(FontSmall))/2
	holdAnchor := NumberAnchor(Rect{W: head.X + head.W*0.55})
	Text(dst, "ORE", head.X+padSmall, headY, FontSmall, ColHead)
	TextRight(dst, "IN HOLD", holdAnchor, headY, FontSmall, ColHead)

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
		rowY := row.Y + (row.H-lineH(FontSmall))/2
		Text(dst, ellipsize(def.Name, holdAnchor-row.X-padSmall, FontSmall),
			row.X+padSmall, rowY, FontSmall, ColText)
		TextRight(dst, intStr(int64(st.Qty)), holdAnchor, rowY, FontSmall, ColDim)
		u.button(dst, RightButton(row, 70), "Refine", u.marketQty() <= st.Qty)
	}
	if shown == 0 {
		Text(dst, "No refineable ore in cargo.", table.X+padSmall,
			table.Y+stHeadH+padSmall, FontSmall, ColDim)
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
			Text(dst, labels[row], rr.X, rr.Y+(rr.H-lineH(FontTiny))/2, FontTiny, ColHead)
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
					TextCentered(dst, moduleLetter(def.Name), cell.X+cell.W/2,
						cell.Y+(cell.H-lineH(FontLarge))/2, FontLarge, moduleAccent(def.Kind))
					u.button(dst, FittingUnfitRect(cell), "x", true)
				}
			}
		}
	}

	Text(dst, "Hangar", body.X, StationHangarHeaderY(body), FontSmall, ColAccent)
	if s.Hangar != nil {
		shipCount := len(s.Hangar.Ships)
		if shipCount > hangarVisibleShips {
			TextRight(dst, "+"+intStr(int64(shipCount-hangarVisibleShips))+" more",
				body.Right(), StationHangarHeaderY(body), FontTiny, ColDim)
		}
		if shipCount == 0 {
			Text(dst, "No ships in hangar.", body.X+padSmall,
				StationHangarHeaderY(body)+lineH(FontSmall), FontSmall, ColDim)
		}
		for i, ss := range s.Hangar.Ships {
			if i >= hangarVisibleShips {
				break
			}
			row := StationHangarRow(body, i)
			name := string(ss.Hull)
			if def, ok := u.c.Ship(ss.Hull); ok {
				name = def.Name
			}
			rowY := row.Y + (row.H-lineH(FontSmall))/2
			Text(dst, ellipsize(name, row.W*0.5, FontSmall), row.X+padSmall, rowY, FontSmall, ColText)
			TextRight(dst, intStr(int64(len(ss.Fit)))+" modules",
				row.Right()-2*minHit-3*padSmall, rowY, FontTiny, ColDim)
			u.button(dst, RightButton(row, 84), "Assemble", true)
		}
	}

	Text(dst, "Hangar items", body.X, StationItemsHeaderY(body), FontSmall, ColAccent)
	if s.Hangar != nil {
		if len(s.Hangar.Items) == 0 {
			Text(dst, "empty", body.X+padSmall, StationItemsHeaderY(body)+lineH(FontSmall),
				FontSmall, ColDim)
		}
		for i, st := range s.Hangar.Items {
			if i >= StationItemMaxRows(body) {
				break
			}
			row := StationItemRow(body, i)
			rowY := row.Y + (row.H-lineH(FontSmall))/2
			fitW := 44.0
			Text(dst, ellipsize(u.itemName(st.Item), row.W*0.6, FontSmall),
				row.X+padSmall, rowY, FontSmall, ColText)
			TextRight(dst, "×"+intStr(int64(st.Qty)),
				row.Right()-RightButton(row, fitW).W-2*padSmall, rowY, FontSmall, ColDim)
			// Modules can be fitted into the first free slot of their class.
			if _, ok := u.c.Module(content.ModuleID(st.Item)); ok && st.Qty > 0 {
				fitBtn := RightButton(row, fitW)
				u.button(dst, fitBtn, "Fit", u.firstEmptySlotExists(content.ModuleID(st.Item)))
			}
		}
	}
}

// firstEmptySlotExists reports whether the module still fits on the hull.
func (u *UI) firstEmptySlotExists(id content.ModuleID) bool {
	def, ok := u.c.Module(id)
	if !ok {
		return false
	}
	_, free := u.firstEmptySlot(def.Slot)
	return free
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
func (u *UI) clickStation(x, y float64, r Rect) {
	if ti := TabAt(r, numStationTabs, x, y); ti >= 0 {
		u.stationTab = ti
		return
	}
	body := StationBody(r)
	switch u.stationTab {
	case tabRepair:
		if RepairButton(body).Contains(x, y) && u.needsRepair() {
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
				if sys.Market[i].Qty >= qty {
					u.enqueue(sim.CmdBuy{Item: item, Qty: qty})
				}
			case MarketSellRect(row).Contains(x, y):
				if u.cargoCount(item) >= qty {
					u.enqueue(sim.CmdSell{Item: item, Qty: qty})
				}
			}
		}
	case tabRefine:
		u.clickStepper(body, x, y)
		table := MarketTable(body)
		if i := StationRowAt(table, y); i >= 0 {
			row := StationRowRect(table, i)
			if RightButton(row, 70).Contains(x, y) {
				if item, ok := u.refineItemAt(i); ok {
					if qty, ok := u.refineQty(item); ok && qty >= u.marketQty() {
						u.enqueue(sim.CmdRefine{Item: item, Qty: u.marketQty()})
					}
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

// refineQty returns how many units of an ore item sit in cargo.
func (u *UI) refineQty(item content.ItemID) (int, bool) {
	cargo, ok := u.s.Cargos.Get(u.s.Player())
	if !ok {
		return 0, false
	}
	for _, st := range cargo.Stacks {
		if st.Item == item {
			return st.Qty, true
		}
	}
	return 0, false
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
			for i := range fit.Defs(classes[row]) {
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
			if i >= hangarVisibleShips {
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
