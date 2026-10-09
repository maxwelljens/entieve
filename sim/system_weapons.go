package sim

import (
	"math"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// systemWeapons fires active weapon modules whose cycle has completed.
//
// Turret hit chance (EVE-faithful, docs/ARCHITECTURE.md):
//
//	chance = 0.5 ^ ( (angular / (tracking * sig/40000))^2
//	               + (max(0, dist - optimal) / falloff)^2 )
//
// where angular = transversal speed / distance (rad/s), sig is the target's
// signature radius in metres, tracking the module's rad/s figure. Two rolls
// come from the SHOOTER's RNG stream (the player's master stream or the
// NPC's forked stream): a hit roll r1 < chance, then a damage roll r2 in
// [0,1) giving multiplier 0.01 + r2*0.99, with a wrecking shot at x3 when
// r2 < 0.01. Ammo with a damage profile replaces the base volley; guns
// without a matching charge in cargo are dry: they log one warning per dry
// cycle and deactivate.
func (s *Sim) systemWeapons() {
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
				if !ok || def.Kind != content.KindWeapon || def.Weapon == nil {
					continue
				}
				slot := SlotRef{Class: class, Index: i}
				s.fireWeapon(e, def, slot, rt)
			}
		}
	}
}

// fireWeapon resolves one weapon cycle: validate, pay, load ammo, roll,
// and queue damage for the damage system.
func (s *Sim) fireWeapon(e ecs.Entity, def *content.ModuleDef, slot SlotRef, rt *ModuleRuntime) {
	w := def.Weapon
	if reason, ok := s.checkCycle(e, def, slot); !ok {
		s.deactivate(e, slot, reason, SevWarning)
		return
	}
	s.payCap(e, def.ActivationCost)

	// Ammunition: consume one charge per shot when the weapon needs ammo.
	volley := w.Damage
	if w.AmmoClass != "" {
		cargo, ok := s.Cargos.Get(e)
		if !ok {
			s.deactivate(e, slot, "no cargo", SevWarning)
			return
		}
		ammoItem, ammoDef, ok := findCharge(s, e, slot, cargo.Stacks, w.AmmoClass)
		if !ok {
			s.deactivate(e, slot, "no charges", SevWarning)
			return
		}
		stacks, taken := takeStacks(cargo.Stacks, ammoItem, 1)
		if taken != 1 {
			s.deactivate(e, slot, "no charges", SevWarning)
			return
		}
		cargo.Stacks = stacks
		if ammoDef.Damage != nil {
			volley = *ammoDef.Damage
		}
	}

	tgt, _ := s.modTarget(e, slot)
	tr, _ := s.Transforms.Get(e)
	ttr, ok := s.Transforms.Get(tgt)
	if tr == nil || !ok {
		s.deactivate(e, slot, "target gone", SevWarning)
		return
	}
	// The player firing counts as recent activity on the grid: NPC guards
	// treat it as an aggro trigger around this position (see systemNpc).
	if e == s.player {
		s.recordPlayerWork(tr.Pos)
	}
	var sm, tm *Motion
	if m, ok := s.Motions.Get(e); ok {
		sm = m
	}
	if m, ok := s.Motions.Get(tgt); ok {
		tm = m
	}
	var shooterVel, targetVel core.Vec2
	if sm != nil {
		shooterVel = sm.Vel
	}
	if tm != nil {
		targetVel = tm.Vel
	}
	sig := defaultLockSig
	if tm != nil && tm.SigRadius > 0 {
		sig = tm.SigRadius
	}
	dist, angular := losKinematics(tr.Pos, ttr.Pos, shooterVel, targetVel)
	chance := hitChance(angular, w.Tracking, sig, dist, w.Optimal, w.Falloff)

	rng := s.rngFor(e)
	hit, mult, wrecking := rollShot(rng, chance)
	if !hit {
		s.emit(Event{Kind: EvLog, Actor: e, Target: tgt,
			Text: s.nameOf(e) + " misses " + s.nounOf(tgt)})
	} else {
		s.damage = append(s.damage, pendingDamage{
			src: e, dst: tgt, dmg: volley.Scale(mult), wrecking: wrecking,
		})
	}

	period := w.ShotTime
	if period <= 0 {
		period = def.ActivationTime
	}
	if period <= 0 {
		period = 1
	}
	rt.CycleEnds = s.time + period
}

// findCharge returns the charge the weapon should consume: the owner's
// CmdSetAmmo preference when it is still in cargo and still matches the
// weapon's ammo class, else the first cargo stack carrying any item of the
// given ammo class (canonical stack order). A preference whose item ran
// out or no longer matches silently falls back to automatic selection.
func findCharge(s *Sim, e ecs.Entity, slot SlotRef, stacks []content.CargoStack, ammoClass string) (content.ItemID, *content.ItemDef, bool) {
	if pref := s.ammoPref(e, slot); pref != "" {
		if stacksCount(stacks, pref) > 0 {
			if def, ok := s.Content.Item(pref); ok && def.AmmoClass == ammoClass {
				return pref, def, true
			}
		}
	}
	for _, st := range stacks {
		if st.Qty <= 0 {
			continue
		}
		def, ok := s.Content.Item(st.Item)
		if !ok {
			continue
		}
		if def.AmmoClass != "" && def.AmmoClass == ammoClass {
			return st.Item, def, true
		}
	}
	return "", nil, false
}

// losKinematics returns the distance between shooter and target and the
// angular velocity of the target relative to the shooter: the transversal
// component of the relative velocity divided by the distance (rad/s).
func losKinematics(shooter, target core.Vec2, shooterVel, targetVel core.Vec2) (dist, angular float64) {
	d := target.Sub(shooter)
	dist = d.Len()
	if dist <= 0 {
		return 0, 0
	}
	rel := targetVel.Sub(shooterVel)
	// |d x rel| / |d| is the transversal speed; divide by distance again.
	transversal := math.Abs(d.X*rel.Y-d.Y*rel.X) / dist
	return dist, transversal / dist
}

// hitChance evaluates the EVE turret chance formula. See systemWeapons.
func hitChance(angular, tracking, sig, dist, optimal, falloff float64) float64 {
	// Tracking term.
	var trackingExp float64
	if angular > 0 {
		denom := tracking * (sig / 40000)
		if denom <= 0 {
			return 0
		}
		r := angular / denom
		trackingExp = r * r
	}
	// Falloff term: only distance beyond optimal matters.
	var falloffExp float64
	if over := dist - optimal; over > 0 {
		if falloff <= 0 {
			return 0
		}
		r := over / falloff
		falloffExp = r * r
	}
	return math.Pow(0.5, trackingExp+falloffExp)
}

// rollShot performs the two RNG rolls of one turret shot. It reports the
// hit, the damage multiplier, and whether the shot was a wrecking hit (x3).
func rollShot(rng *core.RNG, chance float64) (hit bool, mult float64, wrecking bool) {
	r1 := rng.Float64()
	if r1 >= chance {
		return false, 0, false
	}
	r2 := rng.Float64()
	if r2 < 0.01 {
		return true, 3, true
	}
	return true, 0.01 + r2*0.99, false
}

// pendingDamage is queued damage awaiting the damage system.
type pendingDamage struct {
	src, dst ecs.Entity
	dmg      content.Damage
	wrecking bool
}
