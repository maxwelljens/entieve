package ui

import "github.com/entieve/entieve/sim"

// presentationGrid returns the grid entities the player may currently
// perceive: sim marks landmark celestials (star/planet/moon/station/gate/
// belt/anomaly) and everything within sim.SensorRange of the player with
// Info.Visible — everything else is off-grid and must not appear in any
// presentation surface (overview, picker, scanners aside).
func (u *UI) presentationGrid() []sim.Info {
	if u.s == nil {
		return nil
	}
	out := make([]sim.Info, 0, 32)
	for _, inf := range u.s.Grid() {
		if inf.Visible {
			out = append(out, inf)
		}
	}
	return out
}
