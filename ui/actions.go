package ui

import "github.com/entieve/entieve/sim"

// Action is a contextual selected-item button. Each maps 1:1 to a sim
// command (see UI.doAction).
type Action string

// The contextual actions, in draw order.
const (
	ActionApproach Action = "Approach"
	ActionOrbit    Action = "Orbit"
	ActionAlign    Action = "Align"
	ActionWarp     Action = "Warp To"
	ActionLock     Action = "Lock"
	ActionMine     Action = "Mine"
	ActionLoot     Action = "Loot"
	ActionDock     Action = "Dock"
	ActionJump     Action = "Jump"
)

// OrbitRanges are the fixed orbit ranges the Orbit button cycles through.
var OrbitRanges = [...]float64{1000, 2500, 5000, 10000}

// WarpRanges are the warp landing-range choices in meters (the segmented
// control `0 | 10 | 50` km). 0 lands on the target (sim's default
// tolerance); Range > 0 lands that many meters short.
var WarpRanges = [...]float64{0, 10000, 50000}

// CycleWarpRange advances a warp-range choice index with wraparound.
func CycleWarpRange(idx int) int {
	n := len(WarpRanges)
	return ((idx % n) + n) % n
}

// WarpRangeMeters returns the landing range (m) for a choice index.
func WarpRangeMeters(idx int) float64 { return WarpRanges[CycleWarpRange(idx)] }

// WarpRangeLabel renders the warp button label: the current choice is
// always visible ("Warp to 0 km" / "Warp to 10 km" / "Warp to 50 km").
func WarpRangeLabel(idx int) string {
	m := WarpRangeMeters(idx)
	if m == 0 {
		return "Warp to 0 km"
	}
	return "Warp to " + itoa(int(m/1000)) + " km"
}

// ActionsFor derives the contextual action list for a selected entity from
// its Info flags. The player itself has no actions. Pure and tested.
func ActionsFor(info sim.Info) []Action {
	if info.Kind == sim.KindPlayer {
		return nil
	}
	// Movement actions make sense for anything on grid.
	out := []Action{ActionApproach, ActionOrbit, ActionAlign}
	if info.CanWarp {
		out = append(out, ActionWarp)
	}
	if canLock(info.Kind) {
		out = append(out, ActionLock)
	}
	if info.CanMine {
		out = append(out, ActionMine)
	}
	if info.CanLoot {
		out = append(out, ActionLoot)
	}
	if info.CanDock {
		out = append(out, ActionDock)
	}
	if info.CanJump {
		out = append(out, ActionJump)
	}
	return out
}

// canLock reports whether entities of this kind can be target-locked.
func canLock(kind string) bool {
	switch kind {
	case sim.KindShip, sim.KindAsteroid, sim.KindWreck:
		return true
	}
	return false
}
