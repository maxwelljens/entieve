package sim

import (
	"encoding/json"
	"testing"

	"github.com/entieve/entieve/content"
)

// savedEntityByID returns the SavedEntity with the given raw handle.
func savedEntityByID(save *Save, id uint64) (SavedEntity, bool) {
	for _, se := range save.Entities {
		if se.ID == id {
			return se, true
		}
	}
	return SavedEntity{}, false
}

// TestSaveSchemaRoundTripsNewFields pins the real persistence schema:
// per-entity Intent, ModTargets (sorted), FreeSlots (free-list mirror) and
// LastStation (docked-station handle) all survive Snapshot -> JSON ->
// Restore, with no synthetic meta entity smuggled through Entities.
func TestSaveSchemaRoundTripsNewFields(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s) // sets lastStation = cel-station-a

	// A live NPC to remember as a module target, plus a second binding so
	// the ModTargets sort is exercised; and one to destroy so the free list
	// gains an entry (destroyed last = top of the free list).
	target := spawnNearPlayer(t, s)
	s.setModTarget(s.player, SlotRef{Class: content.SlotHigh, Index: 0}, target)
	s.setModTarget(s.player, SlotRef{Class: content.SlotHigh, Index: 1}, target)
	victim := spawnNearPlayer(t, s)
	s.destroy(victim)

	// A non-trivial movement intent, set after docking (which writes
	// IntentStop).
	station := entityByName(s, "Haven Rest")
	if station.IsZero() {
		t.Fatal("no station entity")
	}
	s.Intents.Set(s.player, Intent{Kind: IntentOrbit, Target: station, Range: 1500})
	wantHash := s.Hash()

	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	// The synthetic meta entity is gone.
	for _, se := range save.Entities {
		if se.ID == ^uint64(0) {
			t.Fatal("synthetic meta entity still present in Save.Entities")
		}
	}

	// Intent is captured on the player's SavedEntity.
	se, ok := savedEntityByID(save, uint64(s.player))
	if !ok {
		t.Fatal("player missing from save")
	}
	if se.Intent == nil || se.Intent.Kind != IntentOrbit ||
		se.Intent.Target != station || se.Intent.Range != 1500 {
		t.Fatalf("saved intent=%+v want orbit/%v/1500", se.Intent, station)
	}

	// ModTargets: both bindings, sorted by owner/class/index.
	if len(save.ModTargets) != 2 {
		t.Fatalf("mod targets=%d want 2: %+v", len(save.ModTargets), save.ModTargets)
	}
	for i := 1; i < len(save.ModTargets); i++ {
		a, b := save.ModTargets[i-1], save.ModTargets[i]
		if a.Owner > b.Owner || (a.Owner == b.Owner && a.Class > b.Class) ||
			(a.Owner == b.Owner && a.Class == b.Class && a.Index >= b.Index) {
			t.Fatalf("mod targets unsorted: %+v", save.ModTargets)
		}
	}
	wantBind := SavedModTarget{Owner: uint64(s.player), Class: content.SlotHigh,
		Index: 0, Target: uint64(target)}
	if save.ModTargets[0] != wantBind {
		t.Fatalf("binding[0]=%+v want %+v", save.ModTargets[0], wantBind)
	}

	// FreeSlots mirrors the destroyed entity's slot.
	found := false
	for _, f := range save.FreeSlots {
		if f.Index == victim.Index() && f.Generation == victim.Generation() {
			found = true
		}
	}
	if !found {
		t.Fatalf("free slots=%+v missing {%d %d}", save.FreeSlots,
			victim.Index(), victim.Generation())
	}

	// LastStation is the live station handle.
	if save.LastStation == 0 || save.LastStation != uint64(station) {
		t.Fatalf("last station=%d want %d", save.LastStation, uint64(station))
	}

	// Restore into a fresh sim: exact hash + state reconstruction.
	b, err := New(testContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Restore(save); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := b.Hash(); got != wantHash {
		t.Fatalf("hash after restore=%016x want %016x", got, wantHash)
	}
	if b.lastStation != "cel-station-a" {
		t.Fatalf("lastStation=%q want cel-station-a", b.lastStation)
	}
	in, ok := b.Intents.Get(b.player)
	if !ok || in.Kind != IntentOrbit || in.Target != station || in.Range != 1500 {
		t.Fatalf("restored intent=%+v want orbit/%v/1500", in, station)
	}
	gotBind, ok := b.modTarget(b.player, SlotRef{Class: content.SlotHigh, Index: 0})
	if !ok || gotBind != target {
		t.Fatalf("restored binding=%v/%v want %v", gotBind, ok, target)
	}
	if len(b.free) != len(save.FreeSlots) {
		t.Fatalf("free list=%+v want %+v", b.free, save.FreeSlots)
	}
	for i, f := range b.free {
		sf := save.FreeSlots[i]
		if f.idx != sf.Index || f.gen != sf.Generation {
			t.Fatalf("free[%d]={%d %d} want {%d %d}", i, f.idx, f.gen, sf.Index, sf.Generation)
		}
	}
	// The next handle handed out matches across the round-trip.
	if hs, hb := s.create(), b.create(); hs != hb {
		t.Fatalf("post-restore Create diverged: %v vs %v", hs, hb)
	}

	// Plain JSON round-trip of the Save itself restores identically.
	raw, err := json.Marshal(save)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Save
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	c, err := New(testContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Restore(&decoded); err != nil {
		t.Fatalf("restore (json): %v", err)
	}
	if got := c.Hash(); got != wantHash {
		t.Fatalf("hash after JSON round-trip=%016x want %016x", got, wantHash)
	}
}
