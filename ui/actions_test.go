package ui

import (
	"reflect"
	"testing"

	"github.com/entieve/entieve/sim"
)

// TestActionsFor derives contextual actions from Info flags — the Selected
// Item window's button set (1:1 with sim commands).
func TestActionsFor(t *testing.T) {
	cases := []struct {
		name string
		info sim.Info
		want []Action
	}{
		{
			name: "player has no self-actions",
			info: sim.Info{Kind: sim.KindPlayer},
			want: nil,
		},
		{
			name: "plain hostile ship",
			info: sim.Info{Kind: sim.KindShip, Hostile: true},
			want: []Action{ActionApproach, ActionOrbit, ActionAlign, ActionLock},
		},
		{
			name: "warpable ship",
			info: sim.Info{Kind: sim.KindShip, CanWarp: true},
			want: []Action{ActionApproach, ActionOrbit, ActionAlign, ActionWarp, ActionLock},
		},
		{
			name: "station with dock service",
			info: sim.Info{Kind: sim.KindStation, CanDock: true},
			want: []Action{ActionApproach, ActionOrbit, ActionAlign, ActionDock},
		},
		{
			name: "gate with jump",
			info: sim.Info{Kind: sim.KindGate, CanJump: true},
			want: []Action{ActionApproach, ActionOrbit, ActionAlign, ActionJump},
		},
		{
			name: "mineable asteroid",
			info: sim.Info{Kind: sim.KindAsteroid, CanMine: true},
			want: []Action{ActionApproach, ActionOrbit, ActionAlign, ActionLock, ActionMine},
		},
		{
			name: "lootable wreck",
			info: sim.Info{Kind: sim.KindWreck, CanLoot: true},
			want: []Action{ActionApproach, ActionOrbit, ActionAlign, ActionLock, ActionLoot},
		},
		{
			name: "planet only moves",
			info: sim.Info{Kind: sim.KindPlanet},
			want: []Action{ActionApproach, ActionOrbit, ActionAlign},
		},
		{
			name: "ship with every flag",
			info: sim.Info{
				Kind: sim.KindShip, CanWarp: true, CanMine: true,
				CanLoot: true, CanDock: true, CanJump: true,
			},
			want: []Action{
				ActionApproach, ActionOrbit, ActionAlign, ActionWarp,
				ActionLock, ActionMine, ActionLoot, ActionDock, ActionJump,
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ActionsFor(c.info)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("ActionsFor(%+v) = %v, want %v", c.info, got, c.want)
			}
		})
	}
}

// TestActionsForMatchesButtons: the Selected Item panel's button grid must
// accommodate the largest action set without escaping the panel — covered by
// layout tests; here we just pin the max set size (9 <= 3×3).
func TestActionsForMaxSize(t *testing.T) {
	info := sim.Info{
		Kind: sim.KindShip, CanWarp: true, CanMine: true,
		CanLoot: true, CanDock: true, CanJump: true,
	}
	if n := len(ActionsFor(info)); n > actCols*3 {
		t.Fatalf("action set %d exceeds the 3x3 button grid", n)
	}
}
