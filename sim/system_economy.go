package sim

import (
	"math"
	"slices"
	"sort"
	"strconv"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
)

// repairCostPerHP is the station repair fee in euro per missing hit
// point across shield, armor and structure (documented simplification).
const repairCostPerHP = 2

// sellRatio is the fraction of the listed/base price credited on sale
// (EVE's 1% broker-ish margin simplified to a flat 10% cut).
const sellRatio = 0.9

// systemEconomy executes deferred station-service commands (only while
// docked at a station offering the required service).
func (s *Sim) systemEconomy() {
	for _, cmd := range s.deferEcon {
		switch c := cmd.(type) {
		case CmdBuy:
			s.execBuy(c)
		case CmdSell:
			s.execSell(c)
		case CmdRefine:
			s.execRefine(c)
		case CmdRepair:
			s.execRepair()
		case CmdFitModule:
			s.execFit(c)
		case CmdUnfitModule:
			s.execUnfit(c)
		case CmdAssemble:
			s.execAssemble(c)
		}
	}
	s.deferEcon = nil
}

// stationAccess validates that the player is docked at a station offering
// the given service; it warns and reports false otherwise.
func (s *Sim) stationAccess(svc string) (ecs.Entity, bool) {
	if !s.playerDocked() {
		s.log(SevWarning, s.player, "You must be docked to use station services")
		return 0, false
	}
	st := s.dockedStation()
	if st.IsZero() {
		s.log(SevWarning, s.player, "No station services here")
		return 0, false
	}
	station, ok := s.Stations.Get(st)
	if !ok || !hasService(station, svc) {
		s.log(SevWarning, s.player, "This station does not offer "+svc)
		return 0, false
	}
	return st, true
}

// marketIndex returns the index of the order for item in the current
// system's stock (-1 when unlisted).
func (s *Sim) marketIndex(item content.ItemID) int {
	stock := s.market[s.systemID]
	for i := range stock {
		if stock[i].Item == item {
			return i
		}
	}
	return -1
}

// sellPrice returns the unit sale price of an item/ship and whether a market
// order lists it. Unlisted items fall back to ItemDef.BasePrice, unlisted
// modules to ModuleDef.Price; unlisted ships cannot be sold (the schema has
// no ship base price — documented).
func (s *Sim) sellPrice(item content.ItemID) (float64, bool) {
	if i := s.marketIndex(item); i >= 0 {
		return s.market[s.systemID][i].Price, true
	}
	if def, ok := s.Content.Item(item); ok {
		return def.BasePrice, false
	}
	if def, ok := s.Content.Module(content.ModuleID(item)); ok {
		return def.Price, false
	}
	if _, ok := s.Content.Ship(content.ShipID(item)); ok {
		return 0, false // ships must be listed to be sold
	}
	return 0, false
}

// execBuy purchases from the station market while docked.
func (s *Sim) execBuy(c CmdBuy) {
	if _, ok := s.stationAccess("market"); !ok {
		return
	}
	if c.Qty <= 0 {
		s.log(SevWarning, s.player, "Invalid quantity")
		return
	}
	i := s.marketIndex(c.Item)
	if i < 0 {
		s.log(SevWarning, s.player, s.itemName(c.Item)+" is not sold here")
		return
	}
	stock := &s.market[s.systemID][i]
	if stock.Qty <= 0 {
		s.log(SevWarning, s.player, "Out of stock: "+s.itemName(c.Item))
		return
	}
	qty := c.Qty
	if qty > stock.Qty {
		qty = stock.Qty
	}
	total := stock.Price * float64(qty)
	if s.wallet < total {
		s.emit(Event{Kind: EvMarket, Severity: SevWarning, Actor: s.player, Item: c.Item,
			Text: "Insufficient funds for " + s.itemName(c.Item), Value: total})
		return
	}
	s.wallet -= total
	stock.Qty -= qty

	if hull, ok := s.Content.Ship(content.ShipID(c.Item)); ok {
		// Hulls are not stackable: one StoredShip per unit bought, so the
		// wallet/stock debit (qty units) and the delivered hulls stay in
		// lockstep — exactly how execSell takes them back out.
		for range qty {
			s.Hangar.Ships = append(s.Hangar.Ships, StoredShip{Hull: hull.ID})
		}
	} else {
		s.Hangar.Items = addStacks(s.Hangar.Items, c.Item, qty)
	}
	s.emit(Event{Kind: EvMarket, Severity: SevGood, Actor: s.player, Item: c.Item,
		Text:  "Bought " + itoaN(qty) + " x " + s.itemName(c.Item) + " for " + euro(total),
		Value: total})
}

// execSell sells from cargo (general hold first, then ore hold), then the
// hangar; stored hulls sell one at a time.
func (s *Sim) execSell(c CmdSell) {
	if _, ok := s.stationAccess("market"); !ok {
		return
	}
	if c.Qty <= 0 {
		s.log(SevWarning, s.player, "Invalid quantity")
		return
	}
	price, listed := s.sellPrice(c.Item)
	if _, isShip := s.Content.Ship(content.ShipID(c.Item)); isShip && !listed {
		s.log(SevWarning, s.player, "No market for "+s.itemName(c.Item))
		return
	}
	if price <= 0 {
		s.log(SevWarning, s.player, "Cannot sell "+s.itemName(c.Item))
		return
	}

	remaining := c.Qty
	sold := 0
	if cargo, ok := s.Cargos.Get(s.player); ok && remaining > 0 {
		var n int
		cargo.Stacks, n = takeStacks(cargo.Stacks, c.Item, remaining)
		remaining -= n
		sold += n
	}
	if cargo, ok := s.Cargos.Get(s.player); ok && remaining > 0 {
		var n int
		cargo.Ore, n = takeStacks(cargo.Ore, c.Item, remaining)
		remaining -= n
		sold += n
	}
	if remaining > 0 {
		var n int
		s.Hangar.Items, n = takeStacks(s.Hangar.Items, c.Item, remaining)
		remaining -= n
		sold += n
	}
	// Stored hulls: one hull per unit.
	if remaining > 0 {
		kept := s.Hangar.Ships[:0]
		for _, sh := range s.Hangar.Ships {
			if remaining > 0 && sh.Hull == content.ShipID(c.Item) {
				remaining--
				sold++
				continue
			}
			kept = append(kept, sh)
		}
		s.Hangar.Ships = kept
	}
	if sold == 0 {
		s.log(SevWarning, s.player, "Nothing to sell")
		return
	}

	proceeds := sellRatio * price * float64(sold)
	s.wallet += proceeds
	if i := s.marketIndex(c.Item); i >= 0 {
		s.market[s.systemID][i].Qty += sold
	}
	s.emit(Event{Kind: EvMarket, Severity: SevGood, Actor: s.player, Item: c.Item,
		Text:  "Sold " + itoaN(sold) + " x " + s.itemName(c.Item) + " for " + euro(proceeds),
		Value: proceeds})
}

// execRefine converts ore units into minerals via RefineYield at a refine
// service station. Output is floor(yield * qty) per mineral (documented);
// when the outputs do not fit in the general hold, nothing is refined and
// EvtCargoFull fires.
func (s *Sim) execRefine(c CmdRefine) {
	if _, ok := s.stationAccess("refine"); !ok {
		return
	}
	def, ok := s.Content.Item(c.Item)
	if !ok || def.Category != content.CatOre || len(def.RefineYield) == 0 {
		s.log(SevWarning, s.player, "Cannot refine "+s.itemName(c.Item))
		return
	}
	cargo, ok := s.Cargos.Get(s.player)
	if !ok || c.Qty <= 0 {
		s.log(SevWarning, s.player, "Invalid quantity")
		return
	}
	avail := stacksCount(cargo.Stacks, c.Item) + stacksCount(cargo.Ore, c.Item)
	qty := c.Qty
	if qty > avail {
		qty = avail
	}
	if qty <= 0 {
		s.log(SevWarning, s.player, "No "+def.Name+" to refine")
		return
	}

	// Deterministic output order: sorted mineral IDs.
	outs := make([]content.ItemID, 0, len(def.RefineYield))
	for id := range def.RefineYield {
		outs = append(outs, id)
	}
	sort.Slice(outs, func(i, j int) bool { return outs[i] < outs[j] })

	type outStack struct {
		item content.ItemID
		qty  int
	}
	var results []outStack
	var outVol float64
	for _, id := range outs {
		n := int(math.Floor(def.RefineYield[id] * float64(qty)))
		if n > 0 {
			results = append(results, outStack{id, n})
			outVol += s.itemVolume(id) * float64(n)
		}
	}
	if space := s.cargoSpace(cargo.Stacks, cargo.Capacity); outVol > space+1e-9 {
		s.emit(Event{Kind: EvCargoFull, Severity: SevWarning, Actor: s.player,
			Text: "Cargo hold full: cannot store refined minerals"})
		return
	}

	// Consume ore (general hold first, then ore hold).
	var t1, t2 int
	cargo.Stacks, t1 = takeStacks(cargo.Stacks, c.Item, qty)
	cargo.Ore, t2 = takeStacks(cargo.Ore, c.Item, qty-t1)
	if t1+t2 != qty {
		// Shouldn't happen (avail checked); restore nothing, just report.
		s.log(SevWarning, s.player, "Refine count mismatch")
		return
	}
	for _, r := range results {
		cargo.Stacks = addStacks(cargo.Stacks, r.item, r.qty)
	}

	parts := ""
	for i, r := range results {
		if i > 0 {
			parts += ", "
		}
		parts += itoaN(r.qty) + " x " + s.itemName(r.item)
	}
	if parts == "" {
		parts = "nothing"
	}
	s.emit(Event{Kind: EvLog, Severity: SevGood, Actor: s.player, Item: c.Item,
		Text:  "Refined " + itoaN(qty) + " x " + def.Name + " into " + parts,
		Value: float64(qty)})
}

// execRepair restores all three layers to maximum for 2 euro per missing HP.
func (s *Sim) execRepair() {
	if _, ok := s.stationAccess("repair"); !ok {
		return
	}
	ship, ok := s.Ships.Get(s.player)
	if !ok {
		return
	}
	missing := math.Max(0, ship.MaxShield-ship.Shield) +
		math.Max(0, ship.MaxArmor-ship.Armor) +
		math.Max(0, ship.MaxStructure-ship.Structure)
	if missing <= 0 {
		s.log(SevInfo, s.player, "Ship is already at full strength")
		return
	}
	cost := repairCostPerHP * missing
	if s.wallet < cost {
		s.emit(Event{Kind: EvLog, Severity: SevWarning, Actor: s.player,
			Text:  "Insufficient funds for repairs (" + euro(cost) + ")",
			Value: cost})
		return
	}
	s.wallet -= cost
	ship.Shield = ship.MaxShield
	ship.Armor = ship.MaxArmor
	ship.Structure = ship.MaxStructure
	s.emit(Event{Kind: EvLog, Severity: SevGood, Actor: s.player,
		Text:  "Repaired " + num(missing) + " HP for " + euro(cost),
		Value: cost})
}

// execFit moves a module from the hangar into a hull slot (fitting service).
func (s *Sim) execFit(c CmdFitModule) {
	if _, ok := s.stationAccess("fitting"); !ok {
		return
	}
	def, ok := s.Content.Module(c.Item)
	if !ok {
		s.log(SevWarning, s.player, "Unknown module "+string(c.Item))
		return
	}
	if def.Slot != c.Slot.Class {
		s.log(SevWarning, s.player, def.Name+" cannot fit a "+c.Slot.Class+" slot")
		return
	}
	fit, ok := s.Fittings.Get(s.player)
	if !ok {
		return
	}
	ship, ok := s.Ships.Get(s.player)
	if !ok {
		return
	}
	hull, ok := s.Content.Ship(ship.Hull)
	if !ok {
		return
	}
	defs := fit.Defs(c.Slot.Class)
	cap := slotCapacity(hull, c.Slot.Class)
	if len(defs) >= cap {
		s.log(SevWarning, s.player, "No free "+c.Slot.Class+" slot")
		return
	}
	if c.Slot.Index < 0 || c.Slot.Index > len(defs) {
		s.log(SevWarning, s.player, "Invalid slot index")
		return
	}
	hangar, n := takeStacks(s.Hangar.Items, content.ItemID(c.Item), 1)
	if n < 1 {
		s.log(SevWarning, s.player, def.Name+" is not in the hangar")
		return
	}
	s.Hangar.Items = hangar

	rt := ModuleRuntime{Def: c.Item}
	switch c.Slot.Class {
	case content.SlotHigh:
		fit.High = slices.Insert(fit.High, c.Slot.Index, c.Item)
		fit.HighRT = slices.Insert(fit.HighRT, c.Slot.Index, rt)
	case content.SlotMid:
		fit.Mid = slices.Insert(fit.Mid, c.Slot.Index, c.Item)
		fit.MidRT = slices.Insert(fit.MidRT, c.Slot.Index, rt)
	case content.SlotLow:
		fit.Low = slices.Insert(fit.Low, c.Slot.Index, c.Item)
		fit.LowRT = slices.Insert(fit.LowRT, c.Slot.Index, rt)
	}
	s.recomputeFit(s.player)
	s.emit(Event{Kind: EvModule, Actor: s.player,
		Text: def.Name + " fitted to " + c.Slot.Class + " slot " + itoaN(c.Slot.Index+1)})
}

// execUnfit moves a fitted module back into the hangar (fitting service).
func (s *Sim) execUnfit(c CmdUnfitModule) {
	if _, ok := s.stationAccess("fitting"); !ok {
		return
	}
	fit, ok := s.Fittings.Get(s.player)
	if !ok {
		return
	}
	i := c.Slot.Index
	if i < 0 {
		s.log(SevWarning, s.player, "Invalid slot index")
		return
	}
	var id content.ModuleID
	switch c.Slot.Class {
	case content.SlotHigh:
		if i >= len(fit.High) {
			s.log(SevWarning, s.player, "No module in that slot")
			return
		}
		id = fit.High[i]
		fit.High = slices.Delete(fit.High, i, i+1)
		fit.HighRT = slices.Delete(fit.HighRT, i, i+1)
	case content.SlotMid:
		if i >= len(fit.Mid) {
			s.log(SevWarning, s.player, "No module in that slot")
			return
		}
		id = fit.Mid[i]
		fit.Mid = slices.Delete(fit.Mid, i, i+1)
		fit.MidRT = slices.Delete(fit.MidRT, i, i+1)
	case content.SlotLow:
		if i >= len(fit.Low) {
			s.log(SevWarning, s.player, "No module in that slot")
			return
		}
		id = fit.Low[i]
		fit.Low = slices.Delete(fit.Low, i, i+1)
		fit.LowRT = slices.Delete(fit.LowRT, i, i+1)
	default:
		s.log(SevWarning, s.player, "Invalid slot class")
		return
	}
	// Drop and re-key remembered activation targets and ammo preferences
	// around the removed slot.
	s.shiftModTargets(s.player, c.Slot.Class, c.Slot.Index)
	s.shiftAmmoPrefs(s.player, c.Slot.Class, c.Slot.Index)
	s.Hangar.Items = addStacks(s.Hangar.Items, content.ItemID(id), 1)
	s.recomputeFit(s.player)
	name := string(id)
	if def, ok := s.Content.Module(id); ok {
		name = def.Name
	}
	s.emit(Event{Kind: EvModule, Actor: s.player, Text: name + " unfit"})
}

// execAssemble swaps the piloted hull with a stored hull in the hangar
// (fitting service). Cargo travels with each hull; stored hulls always carry
// full HP (StoredShip has no HP field — documented simplification).
func (s *Sim) execAssemble(c CmdAssemble) {
	if _, ok := s.stationAccess("fitting"); !ok {
		return
	}
	storedIdx := -1
	for i, sh := range s.Hangar.Ships {
		if sh.Hull == c.Ship {
			storedIdx = i
			break
		}
	}
	if storedIdx < 0 {
		s.log(SevWarning, s.player, "No "+s.itemName(content.ItemID(c.Ship))+" in the hangar")
		return
	}
	newHull, ok := s.Content.Ship(c.Ship)
	if !ok {
		s.log(SevWarning, s.player, "Unknown ship "+string(c.Ship))
		return
	}
	ship, ok := s.Ships.Get(s.player)
	if !ok {
		return
	}
	curHull, ok := s.Content.Ship(ship.Hull)
	if !ok {
		return
	}
	incoming := s.Hangar.Ships[storedIdx]
	newFit, err := s.buildFitting(incoming.Fit, newHull)
	if err != nil {
		s.log(SevWarning, s.player, "Stored fit is invalid: "+err.Error())
		return
	}

	// Snapshot the current hull + fit + cargo for the hangar.
	var curStored StoredShip
	if fit, ok := s.Fittings.Get(s.player); ok {
		curStored.Fit = flatFit(fit)
	}
	if cargo, ok := s.Cargos.Get(s.player); ok {
		merged := append([]content.CargoStack(nil), cargo.Stacks...)
		merged = append(merged, cargo.Ore...)
		curStored.Cargo = merged
	}
	curStored.Hull = curHull.ID
	s.Hangar.Ships[storedIdx] = curStored

	// Install the incoming hull: full layers, full capacitor, its cargo
	// (ore into the ore hold when the hull has one, else general cargo).
	newCargo := Cargo{Capacity: newHull.Cargo, OreCapacity: newHull.OreHold}
	for _, st := range incoming.Cargo {
		if st.Qty <= 0 {
			continue
		}
		to := &newCargo.Stacks
		if def, ok := s.Content.Item(st.Item); ok && def.Category == content.CatOre && newHull.OreHold > 0 {
			to = &newCargo.Ore
		}
		*to = addStacks(*to, st.Item, st.Qty)
	}
	s.Ships.Set(s.player, ShipState{
		Hull: newHull.ID, Shield: newHull.Shield, Armor: newHull.Armor, Structure: newHull.Structure,
		MaxShield: newHull.Shield, MaxArmor: newHull.Armor, MaxStructure: newHull.Structure,
		Resists: newHull.Resists,
	})
	s.Caps.Set(s.player, Capacitor{Current: newHull.Capacitor, Capacity: newHull.Capacitor, Tau: newHull.Recharge})
	s.Cargos.Set(s.player, newCargo)
	s.Fittings.Set(s.player, newFit)
	if t, ok := s.Targetings.Get(s.player); ok {
		t.Locks = nil
	}
	s.recomputeFit(s.player)
	s.emit(Event{Kind: EvLog, Severity: SevGood, Actor: s.player,
		Text: "Assembled " + newHull.Name})
}

// slotCapacity returns the hull's slot count for a class.
func slotCapacity(hull *content.ShipDef, class string) int {
	switch class {
	case content.SlotHigh:
		return hull.Slots.High
	case content.SlotMid:
		return hull.Slots.Mid
	case content.SlotLow:
		return hull.Slots.Low
	}
	return 0
}

// itoaN formats an int for event text.
func itoaN(n int) string { return strconv.Itoa(n) }
