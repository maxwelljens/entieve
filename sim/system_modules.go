package sim

import (
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
)

// systemModules runs cycle completions for active non-weapon, non-mining
// modules (tank, capacitor, propulsion, ewar). Weapons and mining lasers are
// cycled by their own systems; passive modules never cycle.
//
// Each completed cycle: validate target/capacitor, pay the activation cost,
// apply the effect, and schedule the next cycle. A failed validation
// deactivates the module with a log event.
func (s *Sim) systemModules() {
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
				if !ok {
					rt.Active = false
					continue
				}
				switch def.Kind {
				case content.KindWeapon, content.KindMiningLaser:
					continue // owned by the weapons / mining systems
				case content.KindShieldExtender, content.KindArmorPlate,
					content.KindResistCoat, content.KindCapRecharger:
					continue // passive: never cycles
				}
				slot := SlotRef{Class: class, Index: i}
				if reason, ok := s.checkCycle(e, def, slot); !ok {
					s.deactivate(e, slot, reason, SevWarning)
					continue
				}
				s.payCap(e, def.ActivationCost)
				s.applyActiveEffect(e, def, slot, now)
				period := def.ActivationTime
				if period <= 0 {
					period = 1
				}
				rt.CycleEnds = now + period
			}
		}
	}
}

// targetRequirement describes how a module kind obtains its target.
const (
	targetNone   = ""
	targetLocked = "locked"   // weapon, stasis web, warp scram: locked ship
	targetRock   = "asteroid" // mining laser: live asteroid
)

// reqForKind returns the target requirement of a module kind.
func reqForKind(kind string) string {
	switch kind {
	case content.KindWeapon, content.KindStasisWeb, content.KindWarpScram:
		return targetLocked
	case content.KindMiningLaser:
		return targetRock
	}
	return targetNone
}

// checkCycle validates one cycle of an active module: target presence,
// lock state, range, capacitor, and booster charges. It returns the failure
// reason when invalid.
func (s *Sim) checkCycle(e ecs.Entity, def *content.ModuleDef, slot SlotRef) (string, bool) {
	tg, _ := s.Targetings.Get(e)
	switch reqForKind(def.Kind) {
	case targetLocked:
		tgt, ok := s.modTarget(e, slot)
		if !ok || !s.World.Alive(tgt) {
			return "no target", false
		}
		if def.Kind == content.KindWeapon && !s.Ships.Has(tgt) {
			return "target gone", false
		}
		if tg == nil || !tg.Locked(tgt) {
			return "target not locked", false
		}
		if tg != nil && s.distBetween(e, tgt) > tg.MaxRange {
			return "target out of range", false
		}
	case targetRock:
		tgt, ok := s.modTarget(e, slot)
		if !ok || !s.World.Alive(tgt) {
			return "asteroid gone", false
		}
		ast, ok := s.Asteroids.Get(tgt)
		if !ok || ast.Remaining <= 0 {
			return "asteroid depleted", false
		}
		if tg != nil && s.distBetween(e, tgt) > tg.MaxRange {
			return "asteroid out of range", false
		}
	}
	if def.ActivationCost > 0 {
		if c, ok := s.Caps.Get(e); !ok || c.Current < def.ActivationCost {
			return "insufficient capacitor", false
		}
	}
	if def.Kind == content.KindCapBooster {
		if c, ok := s.Cargos.Get(e); !ok || stacksCount(c.Stacks, capCellID) < 1 {
			return "no cap booster charges", false
		}
	}
	return "", true
}

// payCap deducts an activation cost from the entity's capacitor (floored at 0).
func (s *Sim) payCap(e ecs.Entity, cost float64) {
	if cost <= 0 {
		return
	}
	if c, ok := s.Caps.Get(e); ok {
		c.Current -= cost
		if c.Current < 0 {
			c.Current = 0
		}
	}
}

// applyActiveEffect applies one cycle's effect for tank/cap/prop/ewar kinds.
func (s *Sim) applyActiveEffect(e ecs.Entity, def *content.ModuleDef, slot SlotRef, now float64) {
	b := def.Boost
	switch def.Kind {
	case content.KindShieldBoost:
		if sh, ok := s.Ships.Get(e); ok && b != nil {
			sh.Shield = clamp(sh.Shield+b.ShieldBoostPerCycle, 0, sh.MaxShield)
		}
	case content.KindArmorRep:
		if sh, ok := s.Ships.Get(e); ok && b != nil {
			sh.Armor = clamp(sh.Armor+b.ArmorRepPerCycle, 0, sh.MaxArmor)
		}
	case content.KindCapBooster:
		if c, ok := s.Cargos.Get(e); ok && b != nil {
			if stacks, taken := takeStacks(c.Stacks, capCellID, 1); taken == 1 {
				c.Stacks = stacks
				if cap_, ok := s.Caps.Get(e); ok {
					cap_.Current = clamp(cap_.Current+b.CapRestore, 0, cap_.Capacity)
				}
			}
		}
	case content.KindAfterburner:
		// Cost is paid per cycle; the speed multiplier is maintained by
		// recomputeFit while the module stays Active.
	case content.KindStasisWeb:
		if b == nil {
			return
		}
		if tgt, ok := s.modTarget(e, slot); ok {
			if m, ok := s.Motions.Get(tgt); ok {
				m.WebFactor = 1 - clamp(b.WebStrength, 0, 0.99)
				m.WebbedUntil = now + def.ActivationTime + warpScramGrace
			}
		}
	case content.KindWarpScram:
		if b == nil {
			return
		}
		if tgt, ok := s.modTarget(e, slot); ok {
			if sh, ok := s.Ships.Get(tgt); ok {
				sh.ScrambledUntil = now + def.ActivationTime + warpScramGrace
			}
		}
	}
}

// activate validates and switches on one module. It reports whether the
// module is (now) active; quiet suppresses warning logs (NPC AI retries).
// On success the module's first cycle resolves immediately this tick unless a
// cancelled cycle is still running: then the module resumes that cycle.
func (s *Sim) activate(e ecs.Entity, slot SlotRef, target ecs.Entity, quiet bool) bool {
	_, rt, ok := s.moduleRuntime(e, slot)
	if !ok {
		if !quiet {
			s.log(SevWarning, e, "No such module slot")
		}
		return false
	}
	if rt.Active {
		return true
	}
	def, ok := s.Content.Module(rt.Def)
	if !ok {
		return false
	}
	if def.ActivationTime <= 0 && def.Weapon == nil && def.Mining == nil {
		s.warnOnce(e, rt, def.Name+" is passive and cannot be activated", quiet)
		return false
	}

	// Target requirements at activation time.
	tg, _ := s.Targetings.Get(e)
	switch reqForKind(def.Kind) {
	case targetLocked:
		switch {
		case target.IsZero() || !s.World.Alive(target):
			s.warnOnce(e, rt, def.Name+" requires a target", quiet)
			return false
		case def.Kind == content.KindWeapon && !s.Ships.Has(target):
			s.warnOnce(e, rt, def.Name+" cannot target that", quiet)
			return false
		case tg == nil || !tg.Locked(target):
			s.warnOnce(e, rt, def.Name+" requires a locked target", quiet)
			return false
		case tg != nil && s.distBetween(e, target) > tg.MaxRange:
			s.warnOnce(e, rt, "Target out of range for "+def.Name, quiet)
			return false
		}
	case targetRock:
		switch {
		case target.IsZero() || !s.World.Alive(target):
			s.warnOnce(e, rt, def.Name+" requires an asteroid", quiet)
			return false
		case !s.Asteroids.Has(target):
			s.warnOnce(e, rt, def.Name+" cannot mine that", quiet)
			return false
		case tg != nil && s.distBetween(e, target) > tg.MaxRange:
			s.warnOnce(e, rt, "Asteroid out of range for "+def.Name, quiet)
			return false
		}
	}

	if def.ActivationCost > 0 {
		if c, ok := s.Caps.Get(e); !ok || c.Current < def.ActivationCost {
			s.warnOnce(e, rt, "Insufficient capacitor for "+def.Name, quiet)
			return false
		}
	}
	if def.Kind == content.KindCapBooster {
		if c, ok := s.Cargos.Get(e); !ok || stacksCount(c.Stacks, capCellID) < 1 {
			s.warnOnce(e, rt, "No cap booster charges in cargo", quiet)
			return false
		}
	}

	rt.Active = true
	// A cancelled cycle keeps its clock: switching a module back on inside the
	// cycle it was cancelled in resumes at that boundary instead of restarting
	// it, so toggling can never buy more than one effect per cycle time.
	if rt.CycleEnds <= s.time {
		rt.CycleEnds = s.time
	}
	s.setModTarget(e, slot, target)
	if def.Kind == content.KindAfterburner {
		s.recomputeFit(e) // raises Motion.SpeedMultiplier while active
	}

	text := def.Name + " activated"
	if reqForKind(def.Kind) != targetNone && !target.IsZero() {
		text += " on " + s.nameOf(target)
	}
	s.emit(Event{Kind: EvModule, Actor: e, Target: target, Text: text})
	return true
}

// deactivate switches a module off, optionally logging the reason, and
// recomputes fit-derived stats when the module carried a passive-style
// effect (the afterburner speed multiplier ends with activation).
//
// The cycle clock is kept: the cycle in flight runs out untouched (an
// activation inside it resumes at the same boundary — no clock reset, no
// replay), and nothing resolves while the module is off.
func (s *Sim) deactivate(e ecs.Entity, slot SlotRef, reason string, sev Severity) {
	_, rt, ok := s.moduleRuntime(e, slot)
	if !ok || !rt.Active {
		return
	}
	name := string(rt.Def)
	isAB := false
	if def, ok := s.Content.Module(rt.Def); ok {
		name = def.Name
		isAB = def.Kind == content.KindAfterburner
	}
	rt.Active = false
	if reason != "" {
		s.log(sev, e, name+" deactivated ("+reason+")")
	}
	if isAB {
		s.recomputeFit(e)
	}
	s.emit(Event{Kind: EvModule, Actor: e, Text: name + " deactivated"})
}

// stopAllModules clears every fitted module's active state and cycle clock.
// Docking and undocking use it so the ship leaves the station with nothing
// running and no cycle already in flight.
func (s *Sim) stopAllModules(e ecs.Entity) {
	if fit, ok := s.Fittings.Get(e); ok {
		for _, class := range slotClasses {
			rts := fit.Runtimes(class)
			for i := range rts {
				rts[i].Active = false
				rts[i].CycleEnds = 0
			}
		}
	}
	s.recomputeFit(e) // drops the afterburner speed multiplier
}

// warnOnce emits a rate-limited activation warning for one module.
func (s *Sim) warnOnce(e ecs.Entity, rt *ModuleRuntime, text string, quiet bool) {
	if rt.LastWarn > 0 && s.time-rt.LastWarn < warnInterval {
		return
	}
	rt.LastWarn = s.time
	if !quiet {
		s.log(SevWarning, e, text)
	}
}
