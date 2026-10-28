package sim

// The grid half of the squeeze (design decision #6): as the Works lose ground,
// every authored Old Works spawn anywhere shifts toward the tougher castes —
// the same ladder the fallen-system holding force climbs. At the tick-0 share
// the mix is exactly the authored one, so today's belts are unchanged.

import (
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
)

// cornerWorks leaves the Works holding one system (sys-glusza): the squeeze
// floor.
func cornerWorks(s *Sim) {
	for id, side := range s.war.Control {
		if side == controlWorks && id != "sys-glusza" {
			s.war.Control[id] = controlCharter
		}
	}
	s.warRederive()
}

// liveWorks is every live Old Works hull on the grid, by id.
func liveWorks(s *Sim) map[content.NpcID]int {
	out := map[content.NpcID]int{}
	for _, e := range sortedEntities(s.AIs) {
		ai, _ := s.AIs.Get(e)
		if def, ok := s.Content.Npc(ai.Npc); ok && def.Faction == factionOldWorks {
			out[ai.Npc]++
		}
	}
	return out
}

// TestSqueezeStiffensAmbientSpawns pins the ambient half of the squeeze on the
// shipped content: with the Works at their tick-0 share every authored group
// spawns exactly as authored (a charter belt, a works-held belt and the
// Gnazvor anomaly); with the Works at their floor the same groups land as the
// toughest caste.
func TestSqueezeStiffensAmbientSpawns(t *testing.T) {
	c := content.MustLoad()
	mix := func(sys content.SystemID, cornered bool) map[content.NpcID]int {
		t.Helper()
		s, err := New(c, c.Worlds["world-default"], 7)
		if err != nil {
			t.Fatalf("sim: %v", err)
		}
		if cornered {
			cornerWorks(s)
			if sq := s.Squeeze(); !(sq > 1e6) {
				t.Fatalf("cornered squeeze = %v, want the diverging curve", sq)
			}
		} else if sq := s.Squeeze(); sq != 1 {
			t.Fatalf("tick-0 squeeze = %v, want exactly 1", sq)
		}
		if err := s.loadSystem(sys, ""); err != nil {
			t.Fatalf("load %s: %v", sys, err)
		}
		return liveWorks(s)
	}
	sc, ma, hi := content.NpcID("npc-rat-scuttler"), content.NpcID("npc-rat-marauder"), content.NpcID("npc-rat-hierarch")
	for _, tc := range []struct {
		sys    content.SystemID
		why    string
		want   map[content.NpcID]int
		scaled int
	}{
		{"sys-mlynow", "charter belt", map[content.NpcID]int{sc: 3, ma: 1}, 4},
		{"sys-penumbra", "charter belt + anomaly", map[content.NpcID]int{sc: 2, ma: 1, hi: 1}, 4},
		// The works-held belt adds wave 1's holding force (2 hulls at the
		// weakest caste at squeeze 1, 4 at the toughest caste at the floor).
		{"sys-glusza", "works-held belt", map[content.NpcID]int{sc: 5, ma: 2, hi: 1}, 10},
	} {
		if got := mix(tc.sys, false); !sameMix(got, tc.want) {
			t.Fatalf("%s (%s) at the tick-0 share = %v, want the authored %v", tc.sys, tc.why, got, tc.want)
		}
		if got := mix(tc.sys, true); !sameMix(got, map[content.NpcID]int{hi: tc.scaled}) {
			t.Fatalf("%s (%s) at the squeeze floor = %v, want %d Hierarchs", tc.sys, tc.why, got, tc.scaled)
		}
	}

	// A respawn at the floor lands the scaled caste, and re-arms the AUTHORED
	// group, so the squeeze never compounds on itself.
	s, err := New(c, c.Worlds["world-default"], 7)
	if err != nil {
		t.Fatalf("sim: %v", err)
	}
	cornerWorks(s)
	if err := s.loadSystem("sys-mlynow", ""); err != nil {
		t.Fatalf("load sys-mlynow: %v", err)
	}
	s.respawns = []RespawnTimer{{System: "sys-mlynow", Cel: "cel-mlynow-belt", Npc: sc, At: 0}}
	for i := 0; i < 2; i++ {
		s.Step()
	}
	if n := liveWorks(s)[hi]; n != 5 {
		t.Fatalf("hierarchs after the respawn = %d, want 5 (the replacement scaled too)", n)
	}
	var respawned ecs.Entity // the last handle is the replacement just landed
	for _, e := range sortedEntities(s.AIs) {
		if ai, ok := s.AIs.Get(e); ok && ai.Npc == hi {
			respawned = e
		}
	}
	if respawned.IsZero() {
		t.Fatal("no hierarch on the grid")
	}
	s.handleDeath(respawned, s.player)
	var armed bool
	for _, rt := range s.respawns {
		if rt.Npc == sc && rt.Cel == "cel-mlynow-belt" {
			armed = true
		}
	}
	if !armed {
		t.Fatalf("respawn timers = %+v, want the authored scuttler group re-armed", s.respawns)
	}

	// The authored id rides the save: a restored scaled hull still re-arms its
	// authored group after a reload.
	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	restored, err := New(c, c.Worlds["world-default"], 7)
	if err != nil {
		t.Fatalf("restore sim: %v", err)
	}
	if err := restored.Restore(save); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if restored.Hash() != s.Hash() {
		t.Fatal("hash differs across the round trip")
	}
	var carried bool
	for _, e := range sortedEntities(restored.AIs) {
		if ai, ok := restored.AIs.Get(e); ok && ai.Npc == hi && ai.Authored == sc {
			carried = true
		}
	}
	if !carried {
		t.Fatal("a restored scaled hull lost its authored group id")
	}
}

// sameMix compares two id → count mixes exactly.
func sameMix(a, b map[content.NpcID]int) bool {
	if len(a) != len(b) {
		return false
	}
	for id, n := range b {
		if a[id] != n {
			return false
		}
	}
	return true
}

// TestSqueezeSpawnTwinRunDeterminism pins that the scaled spawn makes no
// divergence: two cornered runs of the same belt hash identically.
func TestSqueezeSpawnTwinRunDeterminism(t *testing.T) {
	c := content.MustLoad()
	run := func() map[int64]uint64 {
		s, err := New(c, c.Worlds["world-default"], 7)
		if err != nil {
			t.Fatalf("sim: %v", err)
		}
		cornerWorks(s)
		if err := s.loadSystem("sys-mlynow", ""); err != nil {
			t.Fatalf("load sys-mlynow: %v", err)
		}
		if n := liveWorks(s)["npc-rat-hierarch"]; n != 4 {
			t.Fatalf("cornered belt = %d hierarchs, want the scaled 4", n)
		}
		hashes := map[int64]uint64{}
		for i := 0; i < 600; i++ {
			s.Step()
			if s.ticks%120 == 0 {
				hashes[s.ticks] = s.Hash()
			}
		}
		return hashes
	}
	a, b := run(), run()
	if len(a) != len(b) {
		t.Fatalf("checkpoint count %d vs %d", len(a), len(b))
	}
	for tick, ha := range a {
		if b[tick] != ha {
			t.Fatalf("checkpoint %d: %016x vs %016x", tick, ha, b[tick])
		}
	}
}
