package ui

import (
	"testing"

	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/sim"
)

// byName indexes rows by name for order assertions.
func names(rows []sim.Info) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Name
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestSortOverviewByName: ascending and descending name sorts.
func TestSortOverviewByName(t *testing.T) {
	rows := []sim.Info{
		{Name: "Zeta", Distance: 1},
		{Name: "alpha", Distance: 2},
		{Name: "Mid", Distance: 3},
	}
	SortOverview(rows, SortName, true)
	// Byte order: "Mid" < "Zeta" < "alpha" (uppercase first).
	if want := []string{"Mid", "Zeta", "alpha"}; !eq(names(rows), want) {
		t.Fatalf("asc = %v, want %v", names(rows), want)
	}
	SortOverview(rows, SortName, false)
	if want := []string{"alpha", "Zeta", "Mid"}; !eq(names(rows), want) {
		t.Fatalf("desc = %v, want %v", names(rows), want)
	}
}

// TestSortOverviewByDistanceAndVelocity: numeric columns.
func TestSortOverviewByDistanceAndVelocity(t *testing.T) {
	rows := []sim.Info{
		{Name: "far", Distance: 9000, Vel: core.V(10, 0)},
		{Name: "near", Distance: 100, Vel: core.V(500, 0)},
		{Name: "mid", Distance: 500, Vel: core.V(90, 0)},
	}
	SortOverview(rows, SortDistance, true)
	if want := []string{"near", "mid", "far"}; !eq(names(rows), want) {
		t.Fatalf("distance asc = %v, want %v", names(rows), want)
	}
	SortOverview(rows, SortDistance, false)
	if want := []string{"far", "mid", "near"}; !eq(names(rows), want) {
		t.Fatalf("distance desc = %v, want %v", names(rows), want)
	}
	SortOverview(rows, SortVelocity, true)
	if want := []string{"far", "mid", "near"}; !eq(names(rows), want) {
		t.Fatalf("velocity asc = %v, want %v", names(rows), want)
	}
}

// TestSortOverviewStable: rows with equal keys keep their input order (the
// overview re-sorts every frame; stability prevents row jitter).
func TestSortOverviewStable(t *testing.T) {
	mk := func() []sim.Info {
		return []sim.Info{
			{Name: "a1", Distance: 100},
			{Name: "b1", Distance: 100},
			{Name: "c1", Distance: 50},
			{Name: "d1", Distance: 100},
			{Name: "e1", Distance: 50},
		}
	}
	// Ascending: ties (100) must keep a1,b1,d1 order; ties (50) keep c1,e1.
	rows := mk()
	SortOverview(rows, SortDistance, true)
	if want := []string{"c1", "e1", "a1", "b1", "d1"}; !eq(names(rows), want) {
		t.Fatalf("stable asc = %v, want %v", names(rows), want)
	}
	// Descending: ties must still be stable in input order.
	rows = mk()
	SortOverview(rows, SortDistance, false)
	if want := []string{"a1", "b1", "d1", "c1", "e1"}; !eq(names(rows), want) {
		t.Fatalf("stable desc = %v, want %v", names(rows), want)
	}
}

// TestSortOverviewByKind: the icon column sorts by kind.
func TestSortOverviewByKind(t *testing.T) {
	rows := []sim.Info{
		{Name: "s", Kind: sim.KindShip},
		{Name: "g", Kind: sim.KindGate},
		{Name: "a", Kind: sim.KindAsteroid},
	}
	SortOverview(rows, SortKind, true)
	if want := []string{"a", "g", "s"}; !eq(names(rows), want) {
		t.Fatalf("kind asc = %v, want %v", names(rows), want)
	}
}
