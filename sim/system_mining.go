package sim

import (
	"math"
	"strconv"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
)

// systemMining runs completed cycles of active mining lasers.
//
// Each cycle moves min(VolumePerCycle, Remaining) m³ of asteroid volume into
// the ore hold (or the general cargo when the hull has no ore hold),
// converting m³ to whole units via ItemDef.Volume. Fractional m³ accumulates
// in Cargo.Remainder until it materializes as a unit, so no ore is lost to
// rounding. A hold with no room for even one whole unit emits EvtCargoFull
// and deactivates the laser; a depleted asteroid is destroyed with a log.
func (s *Sim) systemMining() {
	now := s.time
	for _, e := range sortedEntities(s.Fittings) {
		fit, ok := s.Fittings.Get(e)
		if !ok {
			continue
		}
		for _, class := range slotClasses {
			defs, rts := fit.Defs(class), fit.Runtimes(class)
			for i := range rts {
				rt := &rts[i]
				if !rt.Active || now < rt.CycleEnds {
					continue
				}
				def, ok := s.Content.Module(defs[i])
				if !ok || def.Kind != content.KindMiningLaser || def.Mining == nil {
					continue
				}
				slot := SlotRef{Class: class, Index: i}
				if reason, ok := s.checkCycle(e, def, slot); !ok {
					s.deactivate(e, slot, reason, SevWarning)
					continue
				}
				s.payCap(e, def.ActivationCost)
				s.mineCycle(e, def, slot)
				period := def.Mining.CycleTime
				if period <= 0 {
					period = def.ActivationTime
				}
				if period <= 0 {
					period = 1
				}
				rt.CycleEnds = now + period
			}
		}
	}
}

// mineCycle resolves one mining cycle: volume accounting, unit conversion,
// cargo-full handling and asteroid depletion.
func (s *Sim) mineCycle(e ecs.Entity, def *content.ModuleDef, slot SlotRef) {
	tgt, ok := s.modTarget(e, slot)
	if !ok {
		return
	}
	ast, ok := s.Asteroids.Get(tgt)
	if !ok {
		return
	}
	cargo, ok := s.Cargos.Get(e)
	if !ok {
		return
	}
	m := def.Mining

	container, capM3 := oreContainer(cargo)
	space := capM3 - s.stacksVolume(*container)
	iv := s.mineralUnitVolume(ast.Ore)
	unitsCap := math.Floor(space / iv)
	if unitsCap <= 0 {
		// Not even one whole unit fits.
		hold := "Cargo hold"
		if cargo.OreCapacity > 0 {
			hold = "Ore hold"
		}
		s.emit(Event{Kind: EvCargoFull, Severity: SevWarning, Actor: e, Target: tgt,
			Text: hold + " full: cannot store " + s.oreName(ast.Ore)})
		s.deactivate(e, slot, "hold full", SevWarning)
		return
	}

	volWant := math.Min(m.VolumePerCycle, ast.Remaining)
	if volWant <= 0 {
		s.deactivate(e, slot, "asteroid depleted", SevWarning)
		return
	}
	unitsNat := math.Floor((cargo.Remainder + volWant) / iv)
	units := math.Min(unitsNat, unitsCap)
	vol := volWant
	if units < unitsNat {
		// Only mine what fits as whole units this cycle.
		vol = units*iv - cargo.Remainder
		if vol < 0 {
			vol = 0
		}
	}

	// Apply: asteroid volume -> remainder/units in the ore container.
	ast.Remaining -= vol
	total := cargo.Remainder + vol
	units = math.Floor(total / iv)
	cargo.Remainder = total - units*iv
	if units > 0 {
		*container = addStacks(*container, ast.Ore, int(units))
	}

	// A completed mining cycle counts as recent player activity for the
	// NPC aggro trigger (see systemNpc).
	if e == s.player {
		if tr, ok := s.Transforms.Get(e); ok {
			s.recordPlayerWork(tr.Pos)
		}
	}

	s.emit(Event{Kind: EvLog, Severity: SevInfo, Actor: e, Target: tgt,
		Text:  "Mined " + formatM3(vol) + " m³ " + s.oreName(ast.Ore),
		Value: vol, Item: ast.Ore})

	// Depletion.
	if ast.Remaining <= 1e-9 {
		s.emit(Event{Kind: EvLog, Severity: SevInfo, Actor: e, Target: tgt,
			Text: s.nameOf(tgt) + " depleted", Item: ast.Ore})
		s.destroy(tgt)
	}
}

// mineralUnitVolume returns the m³ per unit of an ore (fallback 1 for
// content without a volume).
func (s *Sim) mineralUnitVolume(ore content.ItemID) float64 {
	if d, ok := s.Content.Item(ore); ok && d.Volume > 0 {
		return d.Volume
	}
	return 1
}

// oreName returns the display name of an ore item.
func (s *Sim) oreName(ore content.ItemID) string {
	if d, ok := s.Content.Item(ore); ok {
		return d.Name
	}
	return string(ore)
}

// formatM3 renders a volume for event text: whole numbers without decimals.
func formatM3(v float64) string {
	if v == math.Floor(v) {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	return strconv.FormatFloat(v, 'f', 2, 64)
}
