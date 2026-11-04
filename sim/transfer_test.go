package sim

import (
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
)

// ---------------------------------------------------------------------------
// CmdTransfer: hold-to-hold and station-hangar moves
// ---------------------------------------------------------------------------

// queueTransfer issues one CmdTransfer and returns the resulting events.
func queueTransfer(s *Sim, item content.ItemID, qty int, from, to string) []Event {
	s.Queue(CmdTransfer{Item: item, Qty: qty, From: from, To: to})
	s.Step()
	return drain(s)
}

// warnEvent returns the first SevWarning log line containing substr.
func warnEvent(evs []Event, substr string) *Event {
	for i := range evs {
		if evs[i].Kind == EvLog && evs[i].Severity == SevWarning &&
			strings.Contains(evs[i].Text, substr) {
			return &evs[i]
		}
	}
	return nil
}

// oreHoldEnabled gives the sloop a dedicated ore hold for the test (the
// fixture sloop ships with OreHold 0).
func oreHoldEnabled(s *Sim, capacity float64) *Cargo {
	c, _ := s.Cargos.Get(s.player)
	c.OreCapacity = capacity
	return c
}

func TestTransferCargoToOreHoldMergesStacks(t *testing.T) {
	s := newTestSim(t)
	c := oreHoldEnabled(s, 1000)
	c.Ore = addStacks(c.Ore, "min-ferrite", 2)
	giveCargo(s, "min-ferrite", 7)

	evs := queueTransfer(s, "min-ferrite", 4, "cargo", "ore")

	if got := stacksCount(c.Stacks, "min-ferrite"); got != 3 {
		t.Fatalf("cargo ferrite=%d want 3", got)
	}
	// Stack merge: one combined stack, not a second entry.
	if len(c.Ore) != 1 || c.Ore[0].Qty != 6 {
		t.Fatalf("ore stacks=%+v want single stack of 6", c.Ore)
	}
	ev := findEvent(evs, EvMarket, "Transferred 4 x Ferrite to ore hold")
	if ev == nil {
		t.Fatalf("missing transfer event: %v", evs)
	}
	if ev.Severity != SevInfo || ev.Item != "min-ferrite" || ev.Value != 4 {
		t.Fatalf("event=%+v want SevInfo/min-ferrite/4", ev)
	}
}

func TestTransferToHangarAndBackWhileDocked(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	before := cargoCount(s, "ammo-emp-s")

	evs := queueTransfer(s, "ammo-emp-s", 8, "cargo", "hangar")
	if got := cargoCount(s, "ammo-emp-s"); got != before-8 {
		t.Fatalf("cargo=%d want %d", got, before-8)
	}
	if hangarCount(s, "ammo-emp-s") != 8 {
		t.Fatalf("hangar=%d want 8", hangarCount(s, "ammo-emp-s"))
	}
	if findEvent(evs, EvMarket, "Transferred 8 x EMP Charge to station hangar") == nil {
		t.Fatalf("missing transfer event: %v", evs)
	}

	evs = queueTransfer(s, "ammo-emp-s", 5, "hangar", "cargo")
	if got := cargoCount(s, "ammo-emp-s"); got != before-3 {
		t.Fatalf("cargo after return=%d want %d", got, before-3)
	}
	if hangarCount(s, "ammo-emp-s") != 3 {
		t.Fatalf("hangar after return=%d want 3", hangarCount(s, "ammo-emp-s"))
	}
	if findEvent(evs, EvMarket, "Transferred 5 x EMP Charge to cargo hold") == nil {
		t.Fatalf("missing transfer-back event: %v", evs)
	}
}

func TestTransferHangarRequiresDocked(t *testing.T) {
	s := newTestSim(t) // not docked
	s.Hangar.Items = addStacks(s.Hangar.Items, "min-nickel", 4)
	before := cargoCount(s, "min-nickel")

	// Into the hangar while undocked.
	evs := queueTransfer(s, "min-nickel", 2, "cargo", "hangar")
	if warnEvent(evs, "Must be docked to access the hangar") == nil {
		t.Fatalf("expected not-docked warning: %v", evs)
	}
	// Out of the hangar while undocked.
	evs = queueTransfer(s, "min-nickel", 2, "hangar", "cargo")
	if warnEvent(evs, "Must be docked to access the hangar") == nil {
		t.Fatalf("expected not-docked warning: %v", evs)
	}
	if cargoCount(s, "min-nickel") != before || hangarCount(s, "min-nickel") != 4 {
		t.Fatalf("cargo=%d hangar=%d: nothing may move while undocked",
			cargoCount(s, "min-nickel"), hangarCount(s, "min-nickel"))
	}
}

func TestTransferRejectsBadHoldsAndQuantity(t *testing.T) {
	s := newTestSim(t)
	giveCargo(s, "min-ferrite", 5)

	// From == To.
	evs := queueTransfer(s, "min-ferrite", 1, "cargo", "cargo")
	if warnEvent(evs, "Invalid transfer holds") == nil {
		t.Fatalf("expected same-hold warning: %v", evs)
	}
	// Unknown hold name.
	evs = queueTransfer(s, "min-ferrite", 1, "cargo", "banks")
	if warnEvent(evs, "Invalid transfer holds") == nil {
		t.Fatalf("expected unknown-hold warning: %v", evs)
	}
	// Non-positive quantities.
	for _, qty := range []int{0, -3} {
		evs = queueTransfer(s, "min-ferrite", qty, "cargo", "hangar")
		if warnEvent(evs, "Invalid quantity") == nil {
			t.Fatalf("qty=%d: expected invalid-quantity warning: %v", qty, evs)
		}
	}
	// Unknown item.
	evs = queueTransfer(s, "item-does-not-exist", 1, "cargo", "hangar")
	if warnEvent(evs, "Unknown item item-does-not-exist") == nil {
		t.Fatalf("expected unknown-item warning: %v", evs)
	}
	// Nothing moved by any rejected command.
	if cargoCount(s, "min-ferrite") != 5 {
		t.Fatalf("cargo=%d want 5 (unchanged)", cargoCount(s, "min-ferrite"))
	}
	if len(s.Hangar.Items) != 0 {
		t.Fatalf("hangar=%+v want empty", s.Hangar.Items)
	}
}

func TestTransferRejectsInsufficientQuantity(t *testing.T) {
	s := newTestSim(t)
	oreHoldEnabled(s, 1000)
	giveCargo(s, "min-ferrite", 3)

	evs := queueTransfer(s, "min-ferrite", 4, "cargo", "ore")
	if warnEvent(evs, "Not enough Ferrite in cargo hold") == nil {
		t.Fatalf("expected insufficient warning: %v", evs)
	}
	// Strict rejection: no partial move.
	c, _ := s.Cargos.Get(s.player)
	if stacksCount(c.Stacks, "min-ferrite") != 3 {
		t.Fatalf("cargo ferrite=%d want 3 (unchanged)", stacksCount(c.Stacks, "min-ferrite"))
	}
	if len(c.Ore) != 0 {
		t.Fatalf("ore stacks=%+v want empty", c.Ore)
	}
	if findEvent(evs, EvMarket, "Transferred") != nil {
		t.Fatalf("no success event allowed on rejection: %v", evs)
	}
}

func TestTransferRejectsCapacityOverflow(t *testing.T) {
	s := newTestSim(t)
	c := oreHoldEnabled(s, 2) // tiny ore hold
	giveCargo(s, "ore-sterngrit", 5)

	// Overflow into the ore hold (5 m³ requested > 2 m³ free).
	evs := queueTransfer(s, "ore-sterngrit", 3, "cargo", "ore")
	if warnEvent(evs, "Not enough space in ore hold") == nil {
		t.Fatalf("expected ore-hold overflow warning: %v", evs)
	}
	if stacksCount(c.Stacks, "ore-sterngrit") != 5 {
		t.Fatalf("cargo ore=%d want 5 (unchanged)", stacksCount(c.Stacks, "ore-sterngrit"))
	}
	// An exact fit still succeeds (2 units x 1 m³ = 2 m³ capacity).
	evs = queueTransfer(s, "ore-sterngrit", 2, "cargo", "ore")
	if findEvent(evs, EvMarket, "Transferred 2 x Sterngrit to ore hold") == nil {
		t.Fatalf("exact fit should succeed: %v", evs)
	}

	// Overflow into the general hold: 450 m³ > ~396 m³ free on the sloop.
	oreHoldEnabled(s, 1000)
	c.Ore = addStacks(c.Ore, "ore-sterngrit", 600)
	evs = queueTransfer(s, "ore-sterngrit", 450, "ore", "cargo")
	if warnEvent(evs, "Not enough space in cargo hold") == nil {
		t.Fatalf("expected cargo overflow warning: %v", evs)
	}
	if stacksCount(c.Ore, "ore-sterngrit") != 602 {
		t.Fatalf("ore=%d want 602 (unchanged)", stacksCount(c.Ore, "ore-sterngrit"))
	}
}

// playerHeldByItem counts every unit of every item the player holds: the
// three holds AND the fitted modules' bays. A bay takes its rounds from cargo
// at the start of a load (clips.go), so a hold-only count reads the owner's
// "the third stack disappeared" as a loss when the rounds are in the gun.
func playerHeldByItem(s *Sim) map[content.ItemID]int {
	out := map[content.ItemID]int{}
	add := func(st []content.CargoStack) {
		for _, st := range st {
			out[st.Item] += st.Qty
		}
	}
	if c, ok := s.Cargos.Get(s.player); ok {
		add(c.Stacks)
		add(c.Ore)
	}
	add(s.Hangar.Items)
	if fit, ok := s.Fittings.Get(s.player); ok {
		for _, class := range slotClasses {
			rts := fit.Runtimes(class)
			for i := range rts {
				if rts[i].Clip.Item != "" {
					out[rts[i].Clip.Item] += rts[i].Clip.Qty
				}
			}
		}
	}
	return out
}

// requireHeld fails when any item's held total moved from want.
func requireHeld(t *testing.T, s *Sim, want map[content.ItemID]int, when string) {
	t.Helper()
	got := playerHeldByItem(s)
	for item, n := range want {
		if got[item] != n {
			t.Errorf("%s: %s held=%d want %d (every unit must still exist)", when, item, got[item], n)
		}
	}
	for item, n := range got {
		if n != want[item] {
			t.Errorf("%s: %s held=%d want %d (every unit must still exist)", when, item, n, want[item])
		}
	}
}

// TestTransferThreeStacksConserveEveryUnit is the owner's inventory repro at
// the sim level: the three starting autocannon rounds sit in cargo, two
// CmdTransfer commands move the first two to the hangar, and the third stack
// STAYS in cargo — no bay loads a round nobody picked (clips.go), which is
// exactly the fix for the stack that used to vanish into the autocannon's
// clip. The pin: cargo + ore + hangar + bays carries the same units per item
// after every command.
func TestTransferThreeStacksConserveEveryUnit(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	wd, ok := c.World("world-showcase")
	if !ok {
		t.Fatal("world-showcase missing")
	}
	s, err := New(c, wd, 42)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Dock the way the UI tests do (the station's celestial id): the grid
	// dock command needs range, and range is not what this pin is about.
	docked := false
	for _, cel := range s.System().Celestials {
		if cel.Station != nil {
			s.Docked.Set(s.player, Docked{At: cel.ID})
			docked = true
			break
		}
	}
	if !docked {
		t.Fatal("showcase start system has no station")
	}

	rounds := []content.ItemID{"ammo-emp-s", "ammo-fusion-s", "ammo-ap-s"}
	cargo, _ := s.Cargos.Get(s.player)
	cargo.Stacks = []content.CargoStack{
		{Item: rounds[0], Qty: 80},
		{Item: rounds[1], Qty: 40},
		{Item: rounds[2], Qty: 20},
	}
	want := playerHeldByItem(s)

	queueTransfer(s, rounds[0], 80, "cargo", "hangar")
	requireHeld(t, s, want, "after transfer #1")
	queueTransfer(s, rounds[1], 40, "cargo", "hangar")
	requireHeld(t, s, want, "after transfer #2")

	// The two moved stacks are in the hangar, whole.
	if stacksCount(s.Hangar.Items, rounds[0]) != 80 || stacksCount(s.Hangar.Items, rounds[1]) != 40 {
		t.Fatalf("hangar=%+v want 80 %s + 40 %s", s.Hangar.Items, rounds[0], rounds[1])
	}
	// Where the third stack went: nowhere. The idle autocannon's bay has no
	// standing order, so it loads nothing and the stack stays in cargo whole.
	if got := stacksCount(cargo.Stacks, rounds[2]); got != 20 {
		t.Fatalf("third stack = %d %s in cargo, want 20 (no bay may take it)", got, rounds[2])
	}
	fit, ok := s.Fittings.Get(s.player)
	if !ok {
		t.Fatal("fixture: player has no fitting")
	}
	for _, class := range slotClasses {
		rts := fit.Runtimes(class)
		for i := range rts {
			if rts[i].Clip.Item == rounds[2] {
				t.Fatalf("a bay auto-loaded %+v without a pick", rts[i].Clip)
			}
		}
	}
	t.Logf("%s: cargo=%d bays=0 (the third stack stayed in cargo)",
		rounds[2], stacksCount(cargo.Stacks, rounds[2]))
}

func TestTransferRejectsMissingOreHold(t *testing.T) {
	s := newTestSim(t) // fixture sloop has OreHold 0
	giveCargo(s, "ore-sterngrit", 3)

	evs := queueTransfer(s, "ore-sterngrit", 1, "cargo", "ore")
	if warnEvent(evs, "This ship has no ore hold") == nil {
		t.Fatalf("expected no-ore-hold warning: %v", evs)
	}
	evs = queueTransfer(s, "ore-sterngrit", 1, "ore", "cargo")
	if warnEvent(evs, "This ship has no ore hold") == nil {
		t.Fatalf("expected no-ore-hold warning (reverse): %v", evs)
	}
	if cargoCount(s, "ore-sterngrit") != 3 {
		t.Fatalf("cargo ore=%d want 3 (unchanged)", cargoCount(s, "ore-sterngrit"))
	}
}
