package sim

import "math"

// systemShield recharges every shield along EVE's curve:
//
//	dC/dt = (10 * Cmax / T) * (sqrt(c) - c),  c = C/Cmax
//
// applied per tick, where T is the hull's shield recharge time (0% to ~98%
// when idle). The bracket peaks at c = 0.25 (2.5x the average rate Cmax/T),
// reproducing EVE's peak recharge at 25% shield. Armor and structure do not
// regenerate. Shield boosters stack on top; both clamp to MaxShield.
func (s *Sim) systemShield() {
	for _, e := range sortedEntities(s.Ships) {
		sh, ok := s.Ships.Get(e)
		if !ok || sh.MaxShield <= 0 || sh.ShieldTau <= 0 {
			continue
		}
		if sh.Shield >= sh.MaxShield {
			continue
		}
		if sh.Shield <= 0 {
			// dC/dt is exactly zero at 0: nudge off the fixed point so a
			// stripped shield rebuilds, as EVE's measured curve does.
			sh.Shield = 1e-9
		}
		c := sh.Shield / sh.MaxShield
		dC := (10 * sh.MaxShield / sh.ShieldTau) * (math.Sqrt(c) - c) * DT
		sh.Shield = clamp(sh.Shield+dC, 0, sh.MaxShield)
	}
}
