package sim

import (
	"testing"

	"github.com/entieve/entieve/content"
)

// ---------------------------------------------------------------------------
// Clocked loads: every bay load — idle or at an active gun's boundary — takes
// one firing cycle. The rounds move at the start (take-at-start conservation,
// exactly as before), the bay reads Loading until the deadline (CycleEnds)
// passes, and the one load line fires at completion.
// ---------------------------------------------------------------------------

// emptyBay leaves the gun's bay empty with its standing order already set (one
// shell type in cargo), so the idle bay pass starts a load on the next tick.
func emptyBay(t *testing.T, s *Sim) {
	t.Helper()
	_, rt, ok := s.moduleRuntime(s.player, ammoSlot())
	if !ok {
		t.Fatal("fixture has no EMP turret slot")
	}
	s.dumpClip(s.player, rt) // rounds back to cargo
	rt.Selection = "ammo-emp-s"
	setCargo(s, content.CargoStack{Item: "ammo-emp-s", Qty: 20})
	drain(s)
}

// TestIdleLoadTakesItsCycle is the RED-first pin for the rack's reload ring:
// an idle bay's load used to finish in the tick it started, invisible. It now
// takes the gun's cycle, with the rounds conserved and the load line moved to
// completion.
func TestIdleLoadTakesItsCycle(t *testing.T) {
	s := newTestSim(t)
	emptyBay(t, s)
	total := ammoTotal(s, "ammo-emp-s")
	cargo := cargoCount(s, "ammo-emp-s")

	s.Step() // the idle bay starts its load
	if got := bayRounds(s, ammoSlot()); got != 5 {
		t.Fatalf("bay = %d after the start, want 5 (rounds move at the start)", got)
	}
	if got := cargoCount(s, "ammo-emp-s"); got != cargo-5 {
		t.Fatalf("cargo = %d, want %d (rounds left cargo at the start)", got, cargo-5)
	}
	if got := ammoTotal(s, "ammo-emp-s"); got != total {
		t.Fatalf("total = %d, want %d (conserved)", got, total)
	}
	if evs := drain(s); findEvent(evs, EvModule, "loaded with") != nil {
		t.Fatalf("load line fired at the start, want it at completion: %v", evs)
	}
	if cs, _ := s.Clip(s.player, ammoSlot()); !cs.Loading {
		t.Fatalf("clip = %+v, want Loading from the start", cs)
	}

	// One tick short of the EMP turret's 2 s cycle: still loading.
	stepN(s, 119)
	if cs, _ := s.Clip(s.player, ammoSlot()); !cs.Loading {
		t.Fatal("the load completed before its cycle")
	}
	if evs := drain(s); findEvent(evs, EvModule, "loaded with") != nil {
		t.Fatalf("the load completed before its cycle: %v", evs)
	}

	stepN(s, 1) // the deadline
	evs := drain(s)
	if findEvent(evs, EvModule, "EMP Turret loaded with EMP Charge (5 rounds)") == nil {
		t.Fatalf("missing completion line: %v", evs)
	}
	cs, _ := s.Clip(s.player, ammoSlot())
	if cs.Loading {
		t.Fatal("a completed load still reads Loading")
	}
	if cs.Selection != "ammo-emp-s" {
		t.Fatalf("selection = %q, want ammo-emp-s after completion", cs.Selection)
	}

	// Exactly once: another cycle logs nothing new.
	stepN(s, 120)
	if evs := drain(s); findEvent(evs, EvModule, "loaded with") != nil {
		t.Fatalf("load line repeated after completion: %v", evs)
	}
}

// TestIdleLoadHoldsTheGun: a load in flight defers the first shot to its
// deadline — the rounds in the bay are not fireable while the load runs.
func TestIdleLoadHoldsTheGun(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	forceLock(s, npc)
	emptyBay(t, s)

	s.Step() // the idle bay starts its load
	s.Queue(CmdActivate{Slot: ammoSlot(), Target: npc})
	s.Step()
	drain(s)
	stepN(s, 118) // one tick short of the deadline
	if evs := drain(s); hasEvent(evs, EvLog, "hits") || hasEvent(evs, EvLog, "misses") {
		t.Fatalf("the gun fired from a bay that is still loading: %v", evs)
	}
	if got := bayRounds(s, ammoSlot()); got != 5 {
		t.Fatalf("bay = %d before the deadline, want 5", got)
	}
	stepN(s, 1) // the deadline: the load completes and the first shot fires
	if got := bayRounds(s, ammoSlot()); got != 4 {
		t.Fatalf("bay = %d after the deadline, want 4 (the first shot)", got)
	}
}

// TestSaveMidLoadResumes pins the persistence of a load in flight: the bay and
// the not-yet-written selection are both in the Save, so a snapshot taken
// mid-load restores into the same load, completes it once, and needs no schema
// change.
func TestSaveMidLoadResumes(t *testing.T) {
	s := newTestSim(t)
	emptyBay(t, s)
	s.Step() // start the load

	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if save.Schema != saveSchemaV3 {
		t.Fatalf("schema = %d, want %d", save.Schema, saveSchemaV3)
	}
	b, err := New(testContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Restore(save); err != nil {
		t.Fatalf("restore: %v", err)
	}
	drain(b) // whatever the fresh sim logged before the restore
	if got, want := b.Hash(), s.Hash(); got != want {
		t.Fatalf("hash after restore=%016x want %016x", got, want)
	}
	if cs, _ := b.Clip(b.player, ammoSlot()); !cs.Loading {
		t.Fatalf("clip = %+v, want the load in flight after restore", cs)
	}
	stepN(b, 120)
	cs, _ := b.Clip(b.player, ammoSlot())
	if cs.Loading || cs.Selection != "ammo-emp-s" {
		t.Fatalf("clip = %+v, want the load completed after its cycle", cs)
	}
	if findEvent(drain(b), EvModule, "EMP Turret loaded with EMP Charge (5 rounds)") == nil {
		t.Fatal("the restored load never logged its completion")
	}
}

// TestFreshFitHasNoClock is the boot half of the standing-order rule: a
// fresh fit has no order, so it has no rounds and no clock running, and the
// idle bay pass leaves it waiting.
func TestFreshFitHasNoClock(t *testing.T) {
	s := newTestSim(t)
	if got := bayRounds(s, ammoSlot()); got != 0 {
		t.Fatalf("fresh bay = %d, want 0", got)
	}
	cs, ok := s.Clip(s.player, ammoSlot())
	if !ok || cs.Loading || cs.Selection != "" {
		t.Fatalf("clip = %+v at boot, want empty with no order", cs)
	}
	if rt, _ := hudModule(s, ammoSlot()); rt.CycleEnds != 0 {
		t.Fatalf("boot bay left a clock at %v, want 0", rt.CycleEnds)
	}
	if findEvent(drain(s), EvModule, "loaded with") != nil {
		t.Fatal("a fresh fit logged a load")
	}
	stepN(s, 240) // the idle bay pass does not pick a type either
	if got := bayRounds(s, ammoSlot()); got != 0 {
		t.Fatalf("bay = %d after idle ticks, want 0 (no order)", got)
	}
}
