package sim

import "math"

// systemCap recharges every capacitor along the EVE curve:
//
//	dC/dt = (2.5 * Capacity / Tau) * sqrt(c) * (1 - sqrt(c)),  c = C/Capacity
//
// applied per tick (Capacitor.RechargeRate is the one copy of the curve).
// The curve peaks at c = 0.25 (sqrt(c)(1-sqrt(c)) is maximal at
// sqrt(c) = 1/2), reproducing EVE's "peak recharge around 25%" behaviour.
func (s *Sim) systemCap() {
	for _, e := range sortedEntities(s.Caps) {
		c, ok := s.Caps.Get(e)
		if !ok || c.Capacity <= 0 || c.Tau <= 0 {
			continue
		}
		if c.Current >= c.Capacity {
			continue
		}
		dC := c.RechargeRate(c.Current) * DT
		c.Current = clamp(c.Current+dC, 0, c.Capacity)
	}
}

// RechargeRate is the recharge curve's rate at a given charge (GJ/s):
// (2.5*Capacity/Tau)*sqrt(c)*(1-sqrt(c)), c = cur/Capacity. The movement of
// the curve, the peak and the Fitting window's readout all come from here.
func (c Capacitor) RechargeRate(cur float64) float64 {
	if c.Capacity <= 0 || c.Tau <= 0 {
		return 0
	}
	r := math.Sqrt(clamp(cur/c.Capacity, 0, 1))
	return 2.5 * c.Capacity / c.Tau * r * (1 - r)
}

// RechargePeak is the curve's maximum rate (GJ/s), at 25% charge.
func (c Capacitor) RechargePeak() float64 { return c.RechargeRate(0.25 * c.Capacity) }

// Endure is how long the capacitor lasts under a constant drain: the
// recharge curve integrated from the current charge at step seconds, capped
// at horizon (an empty cap returns the first step). The step is the caller's
// display resolution; the sim itself recharges per tick.
func (c Capacitor) Endure(drain, step, horizon float64) float64 {
	if drain <= 0 || c.Capacity <= 0 || c.Tau <= 0 || step <= 0 {
		return 0
	}
	cur := clamp(c.Current, 0, c.Capacity)
	for t := 0.0; t < horizon; t += step {
		cur += c.RechargeRate(cur)*step - drain*step
		if cur <= 0 {
			return t + step
		}
		cur = math.Min(cur, c.Capacity)
	}
	return horizon
}
