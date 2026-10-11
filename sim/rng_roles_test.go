package sim

import (
	"math"
	"testing"

	"github.com/entieve/entieve/core"
)

// ---------------------------------------------------------------------------
// F4 — the master stream carries TWO roles: player combat rolls and spawn
// entropy (asteroid/NPC scatter in build.go, plus the per-NPC Fork seeds).
// trafficRNG is the isolated fork. This file mirrors
// TestTrafficNeverDrawsMasterStream for the master stream: it pins exactly
// what the master may consume — nothing while idle, and precisely one
// (angle, radius) scatter pair per asteroid/NPC created by a respawn or a
// gate jump — so a future subsystem reaching for s.RNG fails a test.
// ---------------------------------------------------------------------------

// replaySpawnScatter rebuilds the master-stream consumption of spawn
// creation: spawnAsteroid and spawnNpcStream each draw Range(0, 2π) then
// Range(0, spread) — two master draws per asteroid or NPC. The per-NPC
// Fork seed only reads the stream; it never advances it.
func replaySpawnScatter(state uint64, spreads ...float64) uint64 {
	rng := core.RestoreRNG(state)
	for _, spread := range spreads {
		rng.Range(0, 2*math.Pi)
		rng.Range(0, spread)
	}
	return rng.Snapshot()
}

func TestMasterStreamRolesPinned(t *testing.T) {
	t.Run("idle ticks consume no master rolls", func(t *testing.T) {
		s := newTestSim(t)
		before := s.RNG.Snapshot()
		stepN(s, 900)
		if got := s.RNG.Snapshot(); got != before {
			t.Fatalf("idle simulation consumed master rolls: %016x -> %016x", before, got)
		}
	})

	t.Run("respawn spawn draws exactly its scatter", func(t *testing.T) {
		s := newTestSim(t)
		npcsBefore := s.AIs.Len()
		before := s.RNG.Snapshot()
		// The anomaly respawn group: spread = the celestial's radius (1500).
		s.respawns = append(s.respawns, RespawnTimer{
			System: "sys-a", Cel: "cel-anomaly-a", Npc: "npc-rat-scuttler", At: s.time,
		})
		s.Step()
		drain(s)
		if s.AIs.Len() != npcsBefore+1 {
			t.Fatalf("NPCs=%d want %d (respawn never fired)", s.AIs.Len(), npcsBefore+1)
		}
		if got, want := s.RNG.Snapshot(), replaySpawnScatter(before, 1500); got != want {
			t.Fatalf("respawn drew %016x, want exactly the 2 scatter rolls %016x", got, want)
		}
	})

	t.Run("gate jump draws exactly its scatter", func(t *testing.T) {
		s := newTestSim(t)
		gate := entityByName(s, "Gate to Beta")
		if gate.IsZero() {
			t.Fatal("no gate")
		}
		setPlayerPos(s, gatePos(s, gate))
		before := s.RNG.Snapshot()
		s.Queue(CmdJump{Gate: gate})
		s.Step()
		drain(s)
		if s.systemID != "sys-b" {
			t.Fatalf("system=%q want sys-b (jump did not happen)", s.systemID)
		}
		// sys-b loads one belt asteroid plus one belt NPC, both scattered
		// around the belt (radius 1000) — four master rolls, nothing else.
		if got, want := s.RNG.Snapshot(), replaySpawnScatter(before, 1000, 1000); got != want {
			t.Fatalf("gate jump drew %016x, want exactly the 4 scatter rolls %016x", got, want)
		}
		after := s.RNG.Snapshot()
		stepN(s, 600)
		if got := s.RNG.Snapshot(); got != after {
			t.Fatalf("post-jump idle ticks consumed master rolls: %016x -> %016x", after, got)
		}
	})
}
