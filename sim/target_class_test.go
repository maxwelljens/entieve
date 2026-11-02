package sim

import (
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
)

// TestModuleTargetClassGate is the matrix pin for the one target-class rule:
// every module that takes a target is accepted by the activation gate exactly
// when its TargetClass names the entity's class, which is the same Suits
// answer the rack's pick reads. A UI copy that disagrees with the sim fails
// here. TargetNone modules are skipped: they take no target at all, so the
// sim ignores whatever entity a caller hands them and the UI always picks the
// zero entity.
func TestModuleTargetClassGate(t *testing.T) {
	cols := []struct {
		name string
		ship bool
		rock bool
		find func(*testing.T, *Sim) ecs.Entity
	}{
		{"ship", true, false, func(t *testing.T, s *Sim) ecs.Entity { return spawnNearPlayer(t, s) }},
		{"asteroid", false, true, func(t *testing.T, s *Sim) ecs.Entity {
			rocks := sortedEntities(s.Asteroids)
			if len(rocks) == 0 {
				t.Fatal("fixture belt has no asteroid")
			}
			teleport(s, rocks[0], v(1e7+1000, 500)) // inside lock range of the player
			return rocks[0]
		}},
		{"station", false, false, func(t *testing.T, s *Sim) ecs.Entity { return s.stationAt("cel-station-a") }},
		{"none", false, false, func(*testing.T, *Sim) ecs.Entity { return 0 }},
	}
	pairs := 0
	for id, def := range testContent().Modules {
		cls := def.TargetClass()
		if cls == content.TargetNone {
			continue
		}
		for _, col := range cols {
			s := newTestSim(t)
			slot := injectModule(t, s, id)
			target := col.find(t, s)
			if (col.name == "none") != target.IsZero() {
				t.Fatalf("%s/%s: target %v does not match the column", id, col.name, target)
			}
			forceLock(s, target) // the lock dimension held constant across the matrix
			got := s.activate(s.player, slot, target, true)
			want := !target.IsZero() && cls.Suits(col.ship, col.rock)
			if got != want {
				t.Errorf("%s (%s) on %s: sim accepted = %v, target class says %v", id, cls, col.name, got, want)
			}
			pairs++
		}
	}
	if pairs == 0 {
		t.Fatal("fixture has no targeting module: the matrix checked nothing")
	}
}
