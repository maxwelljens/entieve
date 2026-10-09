package sim

import (
	"math"
)

// defaultLockSig is the signature radius assumed for targets without motion
// (asteroids, wrecks) when computing lock time.
const defaultLockSig float64 = 100

// systemTargeting advances lock progress for every ship.
//
// Lock time follows the EVE-classic approximation t_lock = 66 * sig / scanRes
// seconds. Written as per-tick progress with sig in metres:
//
//	progress += DT / t_lock
//	          = (ScanRes/1000) * (60000/sig) / 66 / 3600   (per tick)
//
// so a 40 m frigate locked by a 620 mm/s scanner takes ~4.3 s, "a few
// seconds" as required. Locks drop automatically when the target leaves
// MaxRange or dies; completion emits EvtTargetLocked.
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
			tp, ok := s.posOf(l.Target)
			if !ok {
				continue
			}
			if hasSelf && selfPos.Dist(tp) > t.MaxRange {
				continue // drifted out of lock range
			}
			if l.Progress < 1 {
				sig := defaultLockSig
				if m, ok := s.Motions.Get(l.Target); ok && m.SigRadius > 0 {
					sig = m.SigRadius
				}
				rate := lockProgressPerTick(t.ScanRes, sig)
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
	return (scanRes / 1000) * (60000 / sig) / 66 / 3600
}
