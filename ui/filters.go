package ui

import "github.com/entieve/entieve/sim"

// FilterGroup is one overview visibility toggle group.
type FilterGroup int

// The overview filter groups (draw order in the filter popup).
const (
	FilterShips FilterGroup = iota
	FilterAsteroids
	FilterWrecks
	FilterStations
	FilterCelestials
	numFilterGroups
)

// KindFilters is the per-group on/off state, remembered per session. It is
// a pure presentation filter layered on top of Info.Visible.
type KindFilters [numFilterGroups]bool

// AllFilters returns every group enabled (session default).
func AllFilters() KindFilters { return KindFilters{true, true, true, true, true} }

// FilterGroupOf maps an entity kind to its overview filter group.
// Belts and other landmark celestials fall in with celestials.
func FilterGroupOf(kind string) FilterGroup {
	switch kind {
	case sim.KindShip, sim.KindPlayer:
		return FilterShips
	case sim.KindAsteroid:
		return FilterAsteroids
	case sim.KindWreck:
		return FilterWrecks
	case sim.KindStation, sim.KindGate:
		return FilterStations
	default: // star, planet, moon, belt, anomaly, unknown celestials
		return FilterCelestials
	}
}

// Passes reports whether an entity of the given kind survives the filters.
// The player's own ship is never hidden.
func (f KindFilters) Passes(kind string) bool {
	if kind == sim.KindPlayer {
		return true
	}
	return f[FilterGroupOf(kind)]
}

// Toggle flips one group.
func (f *KindFilters) Toggle(g FilterGroup) {
	if g < 0 || g >= numFilterGroups {
		return
	}
	f[g] = !f[g]
}
