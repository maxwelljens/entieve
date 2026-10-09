package sim

import (
	"encoding/json"
	"testing"

	"github.com/entieve/entieve/core"
)

// scriptTick drives both determinism and save/restore continuation. Every
// command derives from the sim's own state, so identical states produce
// identical inputs. It exercises combat, RNG draws, entity destruction
// (wrecks), entity creation (spawn) and respawn timers.
func scriptTick(s *Sim, i int) {
	switch i {
	case 5:
		setPlayerPos(s, v(5e6+3000, 5e6+500)) // near the anomaly
	case 50:
		if npc := nearestNPC(s); !npc.IsZero() {
			forceLock(s, npc)
		}
	case 100:
		if npc := nearestNPC(s); !npc.IsZero() {
			s.Queue(CmdActivate{Slot: SlotRef{Class: "high", Index: 0}, Target: npc})
		}
	case 400:
		if npc := nearestNPC(s); !npc.IsZero() {
			s.Queue(CmdOrbit{Target: npc, Range: 1500})
		}
	case 700:
		if npc := nearestNPC(s); !npc.IsZero() {
			damageNPC(s, s.player, npc, contentDamage(), false)
		}
	case 900:
		// Post-snapshot entity creation: exercises the free-slot mirror and
		// a fresh NPC RNG fork.
		s.spawnNpc("npc-rat-scuttler", v(5e6+3000, 5e6+500), 200)
	case 950:
		if npc := nearestNPC(s); !npc.IsZero() {
			damageNPC(s, s.player, npc, contentDamage(), false)
		}
	case 1100:
		s.Queue(CmdStop{})
	}
	s.Step()
}

func runScripted(seed int64, ticks int, checkpoints map[int]bool) map[int]uint64 {
	s, err := New(testContent(), testWorld(), seed)
	if err != nil {
		return nil
	}
	out := make(map[int]uint64)
	if checkpoints[0] {
		out[0] = s.Hash()
	}
	for i := 0; i < ticks; i++ {
		scriptTick(s, i)
		if checkpoints[i+1] {
			out[i+1] = s.Hash()
		}
	}
	return out
}

func TestDeterminismTwinRunsIdenticalHash(t *testing.T) {
	checkpoints := map[int]bool{1: true, 100: true, 401: true, 701: true, 951: true, 1200: true}
	a := runScripted(7, 1200, checkpoints)
	b := runScripted(7, 1200, checkpoints)
	if a == nil || b == nil {
		t.Fatal("script failed to run")
	}
	for _, cp := range []int{1, 100, 401, 701, 951, 1200} {
		if a[cp] != b[cp] {
			t.Fatalf("checkpoint %d: hashes differ (a=%016x b=%016x)", cp, a[cp], b[cp])
		}
	}
}

func TestDeterminismDifferentSeedsDiffer(t *testing.T) {
	checkpoints := map[int]bool{1: true, 401: true}
	a := runScripted(7, 500, checkpoints)
	b := runScripted(8, 500, checkpoints)
	if a[1] == b[1] {
		t.Fatal("different seeds must fingerprint differently from the start")
	}
	if a[401] == b[401] {
		t.Fatal("different seeds diverged back to equal state?!")
	}
}

func TestForkStreamsIndependent(t *testing.T) {
	master := core.NewRNG(99)
	f1, f2 := master.Fork(1), master.Fork(2)

	draw := func(r *core.RNG) []uint64 {
		var out []uint64
		for i := 0; i < 8; i++ {
			out = append(out, r.Uint64())
		}
		return out
	}
	m, a, b := draw(master), draw(f1), draw(f2)
	same := func(x, y []uint64) bool {
		for i := range x {
			if x[i] != y[i] {
				return false
			}
		}
		return true
	}
	if same(m, a) || same(m, b) || same(a, b) {
		t.Fatal("forked streams must differ from master and each other")
	}
	// Forking is reproducible: same receiver state + stream id => same stream.
	master2 := core.NewRNG(99)
	f1b := master2.Fork(1)
	if !same(a, draw(f1b)) {
		t.Fatal("fork reproducibility broken")
	}
}

func TestSnapshotRestoreHashEqual(t *testing.T) {
	s, err := New(testContent(), testWorld(), 1234)
	if err != nil {
		t.Fatal(err)
	}
	// Build up state: combat, module activity, movement.
	setPlayerPos(s, v(5e6+3000, 5e6+500))
	for i := 0; i < 300; i++ {
		scriptTick(s, i)
	}
	before := s.Hash()

	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	// Snapshot must not mutate the live sim.
	if after := s.Hash(); after != before {
		t.Fatalf("Hash changed by Snapshot: %016x -> %016x", before, after)
	}

	b, err := New(testContent(), testWorld(), 1234)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Restore(save); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := b.Hash(); got != before {
		t.Fatalf("restored hash=%016x want %016x", got, before)
	}
	// Scalars and handles round-trip.
	if b.Ticks() != s.Ticks() || b.Wallet() != s.Wallet() || b.Player() != s.Player() {
		t.Fatal("scalar/handle state mismatch after restore")
	}
	if b.Time() != s.Time() {
		t.Fatalf("time mismatch: %v vs %v", b.Time(), s.Time())
	}
}

func TestRestoreContinuesIdentically(t *testing.T) {
	checkpoints := map[int]bool{401: true, 701: true, 951: true, 1200: true}

	// Run A to the snapshot point (mid-combat, active module, clean queue).
	a, err := New(testContent(), testWorld(), 7)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 400; i++ {
		scriptTick(a, i)
	}
	save, err := a.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	// Run B: fresh sim restored from A's snapshot.
	b, err := New(testContent(), testWorld(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Restore(save); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if a.Hash() != b.Hash() {
		t.Fatal("hashes differ immediately after restore")
	}

	// Continue both with the identical script.
	var aHashes, bHashes []uint64
	for i := 400; i < 1200; i++ {
		scriptTick(a, i)
		scriptTick(b, i)
		if checkpoints[i+1] {
			aHashes = append(aHashes, a.Hash())
			bHashes = append(bHashes, b.Hash())
		}
	}
	for i := range aHashes {
		if aHashes[i] != bHashes[i] {
			t.Fatalf("continuation diverged at checkpoint %d: %016x vs %016x",
				i, aHashes[i], bHashes[i])
		}
	}
	// The continuation also created entities (spawn at i=900, wrecks), which
	// proves the free-slot mirror reproduced exact handles.
	if a.World.Len() != b.World.Len() {
		t.Fatalf("entity counts differ: %d vs %d", a.World.Len(), b.World.Len())
	}
}

func TestSaveJSONRoundTrip(t *testing.T) {
	s, err := New(testContent(), testWorld(), 555)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 200; i++ {
		scriptTick(s, i)
	}
	before := s.Hash()
	save, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(save)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Save
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	b, err := New(testContent(), testWorld(), 555)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Restore(&decoded); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := b.Hash(); got != before {
		t.Fatalf("hash after JSON round-trip=%016x want %016x", got, before)
	}
	// And it keeps running identically for a while.
	for i := 200; i < 300; i++ {
		scriptTick(s, i)
		scriptTick(b, i)
	}
	if s.Hash() != b.Hash() {
		t.Fatal("diverged after JSON round-trip continuation")
	}
}

func TestHashTracksStateChanges(t *testing.T) {
	s := newTestSim(t)
	h0 := s.Hash()
	s.wallet += 1
	if s.Hash() == h0 {
		t.Fatal("Hash must react to wallet changes")
	}
	h1 := s.Hash()
	s.Queue(CmdStop{}) // queued commands are inputs, not state (documented)
	if s.Hash() != h1 {
		t.Fatal("Hash must not depend on the pending command queue")
	}
	s.Step()
	if s.Hash() == h1 {
		t.Fatal("Hash must react to simulation steps")
	}
}

func TestRestoreRejectsInvalidSaves(t *testing.T) {
	s := newTestSim(t)
	if err := s.Restore(nil); err == nil {
		t.Fatal("nil save should error")
	}
	bad := &Save{System: "sys-missing"}
	if err := s.Restore(bad); err == nil {
		t.Fatal("unknown system should error")
	}
	// A save without a player entity.
	bad2 := &Save{System: "sys-a"}
	if err := s.Restore(bad2); err == nil {
		t.Fatal("save without player should error")
	}
}
