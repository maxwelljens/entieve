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
