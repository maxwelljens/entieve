package sim

import (
	"math"
)

// defaultLockSig is the signature radius assumed for targets without motion
// (asteroids, wrecks) when computing lock time.
const defaultLockSig float64 = 100

// systemTargeting advances lock progress for every ship.
//
// Lock time follows EVE's targeting-time curve:
//
//	t_lock = 40000 / (scanRes * asinh(sig)^2)   seconds
//
// with sig in metres. Written as per-tick progress at TickRate:
//
//	progress += DT / t_lock
//	          = scanRes * asinh(sig)^2 / (40000 * 60)
//
// A small signature takes longer to lock and a large one locks faster. Both
// inputs are live: Targeting.EffectiveScanRes (sensor dampeners) and
// Motion.EffectiveSig (microwarpdrives, target painters). Locks drop
// automatically when the target leaves MaxRange or dies; completion emits
// EvtTargetLocked.
func (s *Sim) systemTargeting() {
	for _, e := range sortedEntities(s.Targetings) {
		t, ok := s.Targetings.Get(e)
		if !ok || len(t.Locks) == 0 {
			continue
		}
		selfPos, hasSelf := s.posOf(e)
		kept := t.Locks[:0]
		for _, l := range t.Locks {
			if !s.World.Alive(l.Target) || s.Docked.Has(l.Target) {
				continue // target gone or off-grid
			}
			// A ship in warp cannot be locked or held by a lock: the rule is
			// symmetric (the player's warp drive drops locks the same way),
			// so a warping NPC — and a warping player — is untargetable and
			// unattackable until it lands. Align-phase ships stay lockable:
			// that is where scrams interrupt the warp start.
			if wd, ok := s.Warps.Get(l.Target); ok && wd.Phase == "warp" {
				continue
			}
			tp, ok := s.posOf(l.Target)
			if !ok {
				continue
			}
			if hasSelf && selfPos.Dist(tp) > t.MaxRange {
				continue // drifted out of lock range
			}
			if l.Progress < 1 {
				sig := defaultLockSig
				if m, ok := s.Motions.Get(l.Target); ok {
					if es := m.EffectiveSig(s.time); es > 0 {
						sig = es
					}
				}
				rate := lockProgressPerTick(t.EffectiveScanRes(s.time), sig)
				l.Progress = math.Min(1, l.Progress+rate)
				if l.Progress >= 1 {
					s.emit(Event{Kind: EvTargetLocked, Actor: e, Target: l.Target,
						Text: "Locked " + s.nameOf(l.Target)})
				}
			}
			kept = append(kept, l)
		}
		t.Locks = kept
	}
}

// lockProgressPerTick returns the per-tick lock progress for a scanner and
// target signature (see systemTargeting for the derivation).
func lockProgressPerTick(scanRes, sig float64) float64 {
	if scanRes <= 0 {
		return 0
	}
	if sig <= 0 {
		sig = defaultLockSig
	}
	a := math.Asinh(sig)
	return scanRes * a * a / (40000 * TickRate)
}
