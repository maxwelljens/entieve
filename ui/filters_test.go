package ui

import (
	"testing"

	"github.com/entieve/entieve/sim"
)

// TestFilterGroupMapping: every entity kind maps to exactly one overview
// filter group; belts group with celestials.
func TestFilterGroupMapping(t *testing.T) {
	cases := []struct {
		kind string
		want FilterGroup
	}{
		{sim.KindShip, FilterShips},
		{sim.KindPlayer, FilterShips},
		{sim.KindAsteroid, FilterAsteroids},
		{sim.KindWreck, FilterWrecks},
		{sim.KindStation, FilterStations},
		{sim.KindGate, FilterStations},
		{sim.KindStar, FilterCelestials},
		{sim.KindPlanet, FilterCelestials},
		{sim.KindMoon, FilterCelestials},
		{sim.KindBelt, FilterCelestials},
		{sim.KindAnomaly, FilterCelestials},
	}
	for _, c := range cases {
		if got := FilterGroupOf(c.kind); got != c.want {
			t.Errorf("FilterGroupOf(%q) = %v, want %v", c.kind, got, c.want)
		}
	}
}

// TestKindFiltersPasses: defaults pass everything; toggling a group hides
// only its kinds — and never the player's own ship.
func TestKindFiltersPasses(t *testing.T) {
	f := AllFilters()
	for _, kind := range []string{sim.KindShip, sim.KindAsteroid, sim.KindWreck,
		sim.KindStation, sim.KindGate, sim.KindStar, sim.KindBelt} {
		if !f.Passes(kind) {
			t.Errorf("default filters hide %q", kind)
		}
	}
	f.Toggle(FilterShips)
	if f.Passes(sim.KindShip) {
		t.Error("ships filter off must hide ships")
	}
	if !f.Passes(sim.KindPlayer) {
		t.Error("the player's own ship must never be hidden")
	}
	if !f.Passes(sim.KindWreck) || !f.Passes(sim.KindStation) {
		t.Error("toggling ships must not affect other groups")
	}
	f.Toggle(FilterShips)
	if !f.Passes(sim.KindShip) {
		t.Error("re-enabling ships must show ships again")
	}
	f.Toggle(FilterCelestials)
	if f.Passes(sim.KindBelt) || f.Passes(sim.KindStar) {
		t.Error("celestial filter off must hide belts and stars")
	}
}
