package sim

import (
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// ---------------------------------------------------------------------------
// Visibility model: landmarks always visible, everything else within
// SensorRange of an undocked player. Grid() still returns every on-grid
// entity — Visible is presentation state, never a filter.
// ---------------------------------------------------------------------------

func TestVisibilityLandmarksAlwaysVisible(t *testing.T) {
	s := newTestSim(t)
	// Park the player absurdly far from every landmark (> SensorRange).
	setPlayerPos(s, v(9e9, 9e9))

	for _, name := range []string{
		"Fixture Star", "Haven Rest", "Gate to Beta", "Fixture Belt", "Fixture Den",
	} {
		e := entityByName(s, name)
		if e.IsZero() {
			t.Fatalf("landmark %q missing from grid", name)
		}
		info, ok := s.Info(e)
		if !ok {
			t.Fatalf("Info(%q) failed", name)
		}
		if !landmarkKind(info.Kind) {
			t.Errorf("%q kind=%q is not a landmark kind", name, info.Kind)
		}
		if !info.Visible {
			t.Errorf("%q at %v m must stay Visible (landmark)", name, info.Distance)
		}
	}
	// A far-away non-landmark is invisible at the same distance.
	rock := firstAsteroid(t, s)
	if info, _ := s.Info(rock); info.Visible {
		t.Fatal("asteroid beyond SensorRange must not be Visible")
	}
}

func TestVisibilityFlipsAtSensorRange(t *testing.T) {
	s := newTestSim(t)

	rock := firstAsteroid(t, s)
	rp, _ := s.posOf(rock)
	wreck := makeWreck(t, s, []content.CargoStack{{Item: "loot-navdata", Qty: 1}}, 300)
	wp, _ := s.posOf(wreck)
	ship := dummyTarget(t, s, v(4e7, 4e7), content.Resists{})
	sp, _ := s.posOf(ship)

	cases := []struct {
		name string
		e    ecs.Entity
		p    core.Vec2
	}{
		{"asteroid", rock, rp},
		{"wreck", wreck, wp},
		{"ship", ship, sp},
	}
	for _, c := range cases {
		// Exactly at the boundary: still visible (dist <= SensorRange).
		setPlayerPos(s, c.p.Add(v(SensorRange, 0)))
		info, ok := s.Info(c.e)
		if !ok {
			t.Fatalf("%s: Info failed", c.name)
		}
		if info.Distance != SensorRange {
			t.Fatalf("%s: boundary distance=%v want %v", c.name, info.Distance, SensorRange)
		}
		if !info.Visible {
			t.Errorf("%s at exactly SensorRange must be Visible", c.name)
		}
		// One metre beyond: invisible.
		setPlayerPos(s, c.p.Add(v(SensorRange+1, 0)))
		info, _ = s.Info(c.e)
		if info.Visible {
			t.Errorf("%s beyond SensorRange must not be Visible", c.name)
		}
	}

	// The player itself is visible while on grid.
	pinfo, ok := s.Info(s.player)
	if !ok || !pinfo.Visible {
		t.Fatalf("player info on grid must be Visible: %+v", pinfo)
	}
}

func TestVisibilityDockedPlayerSeesOnlyLandmarks(t *testing.T) {
	s := newTestSim(t)
	rock := firstAsteroid(t, s)
	npc := spawnNearPlayer(t, s)
	wreck := makeWreck(t, s, []content.CargoStack{{Item: "loot-navdata", Qty: 1}}, 300)
	ship := dummyTarget(t, s, v(4e7, 4e7), content.Resists{})

	s.Docked.Set(s.player, Docked{At: "cel-station-a"})

	for _, e := range []ecs.Entity{rock, npc, wreck, ship} {
		info, ok := s.Info(e)
		if !ok {
			t.Fatal("docked Info failed")
		}
		if info.Visible {
			t.Errorf("%q (kind %q) must not be Visible while docked", info.Name, info.Kind)
		}
	}
	// Landmarks stay visible from inside the station.
	for _, name := range []string{"Haven Rest", "Fixture Star", "Fixture Belt", "Fixture Den"} {
		e := entityByName(s, name)
		if info, _ := s.Info(e); !info.Visible {
			t.Errorf("landmark %q must stay Visible while docked", name)
		}
	}
	// Grid() still lists everything (docked entities themselves excluded):
	// it is the command name-resolution surface, Visibility is UI state.
	listed := map[ecs.Entity]bool{}
	for _, info := range s.Grid() {
		listed[info.Entity] = true
	}
	if listed[s.player] {
		t.Fatal("docked player must be excluded from Grid")
	}
	for _, e := range []ecs.Entity{rock, npc, wreck, ship} {
		if !listed[e] {
			t.Fatal("Grid must keep returning invisible on-grid entities")
		}
	}
}

// TestVisibilityBeltMarkerIsLandmark pins Work 1: the belt marker reports
// KindBelt and behaves as a landmark; its rocks stay plain asteroids.
func TestVisibilityBeltMarkerIsLandmark(t *testing.T) {
	s := newTestSim(t)
	belt := entityByName(s, "Fixture Belt")
	info, ok := s.Info(belt)
	if !ok {
		t.Fatal("belt marker missing")
	}
	if info.Kind != KindBelt {
		t.Fatalf("belt marker kind=%q want %q", info.Kind, KindBelt)
	}
	if s.Asteroids.Has(belt) {
		t.Fatal("belt marker must not be an asteroid entity")
	}
	if !info.Visible {
		t.Fatal("belt marker is a landmark and must be Visible")
	}
	// The belt's rocks remain plain KindAsteroid entities.
	rock := firstAsteroid(t, s)
	rinfo, ok := s.Info(rock)
	if !ok || rinfo.Kind != KindAsteroid {
		t.Fatalf("belt rock kind=%q want %q", rinfo.Kind, KindAsteroid)
	}
}
