package sim

import "math"

// systemCap recharges every capacitor along the EVE curve:
//
//	dC/dt = (2.5 * Capacity / Tau) * sqrt(c) * (1 - sqrt(c)),  c = C/Capacity
//
// applied per tick. The curve peaks at c = 0.25 (sqrt(c)(1-sqrt(c)) is
// maximal at sqrt(c) = 1/2), reproducing EVE's "peak recharge around 25%"
// behaviour.
func (s *Sim) systemCap() {
	for _, e := range sortedEntities(s.Caps) {
		c, ok := s.Caps.Get(e)
		if !ok || c.Capacity <= 0 || c.Tau <= 0 {
			continue
		}
		if c.Current >= c.Capacity {
			continue
		}
		r := math.Sqrt(clamp(c.Current/c.Capacity, 0, 1))
		dC := (2.5 * c.Capacity / c.Tau) * r * (1 - r) * DT
		c.Current = clamp(c.Current+dC, 0, c.Capacity)
	}
}
