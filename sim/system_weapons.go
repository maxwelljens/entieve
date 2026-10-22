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
// r2 < 0.01. Ammo with a damage profile replaces the base volley, scaled by
// the gun's damage multiplier; a gun whose bay cannot be filled (no matching
// shell in cargo) is dry: it logs one warning per dry cycle and deactivates.
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

// fireWeapon resolves one weapon cycle: validate, pay, draw a round from the
// shell bay, roll, and queue damage for the damage system. An empty bay is
// refilled from cargo by a full-bay load cycle that defers this shot by one
// firing cycle; 2+ competing shell types leave the gun waiting for a choice
// (quiet, one line), and a bay with no matching rounds at all is dry: one
// "no charges" warning per dry cycle, then the module deactivates.
func (s *Sim) fireWeapon(e ecs.Entity, def *content.ModuleDef, slot SlotRef, rt *ModuleRuntime) {
	w := def.Weapon
	if reason, ok := s.checkCycle(e, def, slot); !ok {
		s.deactivate(e, slot, reason, SevWarning)
		return
	}
	period := cyclePeriod(def)

	// Ammunition: draw one round from the bay, loading it first when empty.
	// A load cycle costs no capacitor (loading is not a firing cycle's work).
	volley := w.Damage
	if w.AmmoClass != "" {
		switch s.fillClip(e, def, rt) {
		case clipFilled:
			// The load cycle replaces this cycle: the shot waits it out.
			rt.CycleEnds = s.time + period
			return
		case clipAmbiguous:
			// Two or more shell types compete and none is selected: sit dry
			// and wait for the player's choice, without the dry-gun warning.
			s.emit(Event{Kind: EvLog, Severity: SevInfo, Actor: e,
				Text: def.Name + " waiting for ammo selection"})
			s.deactivate(e, slot, "", SevInfo)
			return
		}
		if rt.Clip.Qty < 1 {
			s.deactivate(e, slot, "no charges", SevWarning)
			return
		}
		rt.Clip.Qty--
		if ammoDef, ok := s.Content.Item(rt.Clip.Item); ok && ammoDef.Damage != nil {
			// The shell carries the damage profile; the gun's multiplier is
			// its share of the shot (artillery throws a heavier volley).
			volley = ammoDef.Damage.Scale(w.Mult())
		}
	}

	s.payCap(e, def.ActivationCost)

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

	rt.CycleEnds = s.time + period
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
