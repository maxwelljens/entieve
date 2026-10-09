package ui

import (
	"testing"

	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/sim"
)

// TestScanCountsAnonymousAggregation: the scanner reports off-grid contacts
// within 3 × SensorRange as anonymous per-kind counts — no names, no
// distances, no player, no landmarks, no determinism surprises.
func TestScanCountsAnonymousAggregation(t *testing.T) {
	infos := []sim.Info{
		// Off-grid, inside scan range: counted.
		{Kind: sim.KindShip, Distance: 100_000},
		{Kind: sim.KindShip, Distance: 400_000},
		{Kind: sim.KindShip, Distance: 700_000},
		{Kind: sim.KindAsteroid, Distance: 100_000},
		{Kind: sim.KindAsteroid, Distance: 100_001},
		{Kind: sim.KindWreck, Distance: 500_000},
		// On grid (Visible) — already known — and the player: never counted.
		{Kind: sim.KindShip, Distance: 10_000, Visible: true},
		{Kind: sim.KindStar, Distance: 1e9, Visible: true},
		{Kind: sim.KindPlayer, Distance: 0},
		// Beyond 3 × SensorRange: out of scanner reach.
		{Kind: sim.KindShip, Distance: 3*sim.SensorRange + 1},
		{Kind: sim.KindAsteroid, Distance: 1e7},
	}
	got := ScanCounts(infos, sim.SensorRange)
	want := []ScanCount{
		{Kind: "ship", N: 3},
		{Kind: "asteroid", N: 2},
		{Kind: "wreck", N: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("ScanCounts = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ScanCounts[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	// Labels stay anonymous: kind word + count, nothing else.
	if l := ScanKindLabel("ship", 3); l != "Ship ×3" {
		t.Errorf("label = %q, want \"Ship ×3\"", l)
	}
	if l := ScanKindLabel("wreck", 2); l != "Wreck ×2" {
		t.Errorf("label = %q, want \"Wreck ×2\"", l)
	}
}

// TestScanCountsDeterministic: same input, same order — known kinds first,
// unknown kinds alphabetical.
func TestScanCountsDeterministic(t *testing.T) {
	infos := []sim.Info{
		{Kind: "drone", Distance: 1000},
		{Kind: sim.KindWreck, Distance: 1000},
		{Kind: "asteroid", Distance: 1000},
		{Kind: sim.KindShip, Distance: 1000},
		{Kind: "bouy", Distance: 1000},
	}
	once := ScanCounts(infos, sim.SensorRange)
	twice := ScanCounts(infos, sim.SensorRange)
	if len(once) != len(twice) {
		t.Fatalf("nondeterministic lengths: %v vs %v", once, twice)
	}
	for i := range once {
		if once[i] != twice[i] {
			t.Fatalf("nondeterministic order: %v vs %v", once, twice)
		}
	}
	// Known kinds lead (ship, asteroid, wreck), the rest alphabetical.
	wantOrder := []string{"ship", "asteroid", "wreck", "bouy", "drone"}
	for i, k := range wantOrder {
		if once[i].Kind != k {
			t.Fatalf("order[%d] = %q, want %q (full: %v)", i, once[i].Kind, k, once)
		}
	}
}

// TestSweepPhase: the 2-second sweep progress wraps in [0,1).
func TestSweepPhase(t *testing.T) {
	cases := []struct{ t, want float64 }{
		{0, 0},
		{1, 0.5},
		{0.5, 0.25},
		{2, 0},
		{2.5, 0.25},
		{4.5, 0.25},
		{5.999, 0.9995},
	}
	for _, c := range cases {
		got := SweepPhase(c.t, scanCycle)
		if got < 0 || got >= 1 {
			t.Errorf("SweepPhase(%v) = %v out of [0,1)", c.t, got)
		}
		if diff := got - c.want; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("SweepPhase(%v) = %v, want %v", c.t, got, c.want)
		}
	}
	if got := SweepPhase(3, 0); got != 0 {
		t.Errorf("SweepPhase with zero cycle = %v, want 0", got)
	}
}

// TestScanCountsDistanceUsesInfoDistance: contacts are measured from the
// player (Info.Distance), not world position — a pure aggregation over the
// read model.
func TestScanCountsDistanceUsesInfoDistance(t *testing.T) {
	infos := []sim.Info{
		{Kind: sim.KindShip, Distance: 3 * sim.SensorRange, Pos: core.V(1e12, 0)},
		{Kind: sim.KindShip, Distance: 3*sim.SensorRange + 1, Pos: core.V(1, 0)},
	}
	got := ScanCounts(infos, sim.SensorRange)
	if len(got) != 1 || got[0].N != 1 {
		t.Fatalf("boundary contact not counted once: %v", got)
	}
}
