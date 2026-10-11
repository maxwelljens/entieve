package sim

import (
	"strconv"

	"github.com/entieve/entieve/content"
)

// systemLoot executes queued loot commands and expires old wrecks.
//
// Looting requires the player within dockRange (2,500 m) of the wreck.
// CmdLoot.Item selects one item kind; the empty ItemID moves everything.
// Space-limited partial loots move what fits and emit EvtCargoFull when
// nothing fits; a wreck removed when empty or expired.
func (s *Sim) systemLoot() {
	for _, cmd := range s.deferLoot {
		s.execLoot(cmd)
	}
	s.deferLoot = nil

	for _, e := range sortedEntities(s.Wrecks) {
		if w, ok := s.Wrecks.Get(e); ok && s.time >= w.ExpiresAt {
			s.destroy(e)
		}
	}
}

// execLoot resolves one CmdLoot.
func (s *Sim) execLoot(cmd CmdLoot) {
	p := s.player
	wreck, ok := s.Wrecks.Get(cmd.Wreck)
	if !ok || !s.World.Alive(cmd.Wreck) {
		s.log(SevWarning, p, "No such wreck")
		return
	}
	if s.playerDocked() {
		s.log(SevWarning, p, "Cannot loot while docked")
		return
	}
	if s.time >= wreck.ExpiresAt {
		s.log(SevWarning, p, "Wreck has decayed")
		return
	}
	if s.distBetween(p, cmd.Wreck) > dockRange {
		s.log(SevWarning, p, "Wreck is out of range")
		return
	}
	cargo, ok := s.Cargos.Get(p)
	if !ok {
		return
	}
	space := s.cargoSpace(cargo.Stacks, cargo.Capacity)
	if space <= 0 && len(wreck.Loot) > 0 {
		s.emit(Event{Kind: EvCargoFull, Severity: SevWarning, Actor: p, Target: cmd.Wreck,
			Text: "Cargo hold full"})
		return
	}

	moved := 0
	kept := wreck.Loot[:0]
	for _, st := range wreck.Loot {
		if cmd.Item != "" && st.Item != cmd.Item {
			kept = append(kept, st)
			continue
		}
		qty := st.Qty
		if iv := s.itemVolume(st.Item); iv > 0 {
			if maxQty := int(space / iv); qty > maxQty {
				qty = maxQty
			}
		}
		if qty <= 0 {
			kept = append(kept, st)
			continue
		}
		cargo.Stacks = addStacks(cargo.Stacks, st.Item, qty)
		space -= s.itemVolume(st.Item) * float64(qty)
		kept = append(kept, content.CargoStack{Item: st.Item, Qty: st.Qty - qty})
		moved += qty
		s.emit(Event{Kind: EvLooted, Actor: p, Target: cmd.Wreck, Item: st.Item,
			Text: "Looted " + strconv.Itoa(qty) + " x " + s.itemName(st.Item), Value: float64(qty)})
	}
	// Drop exhausted stacks.
	wreck.Loot = kept[:0]
	for _, st := range kept {
		if st.Qty > 0 {
			wreck.Loot = append(wreck.Loot, st)
		}
	}

	if moved == 0 {
		if cmd.Item != "" {
			s.log(SevWarning, p, "No "+s.itemName(cmd.Item)+" in wreck")
		} else {
			s.emit(Event{Kind: EvCargoFull, Severity: SevWarning, Actor: p, Target: cmd.Wreck,
				Text: "Cargo hold full"})
		}
		return
	}
	if len(wreck.Loot) == 0 {
		s.destroy(cmd.Wreck)
	}
}

// itemName returns the display name of any marketable id: items, modules
// and hulls resolve through their content defs; unknown ids fall back to
// the raw id (which is also what a genuinely unknown id should read as).
func (s *Sim) itemName(id content.ItemID) string {
	if d, ok := s.Content.Item(id); ok {
		return d.Name
	}
	if d, ok := s.Content.Module(content.ModuleID(id)); ok {
		return d.Name
	}
	if d, ok := s.Content.Ship(content.ShipID(id)); ok {
		return d.Name
	}
	return string(id)
}
