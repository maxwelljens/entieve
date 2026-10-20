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
