package sim

import (
	"testing"

	"github.com/entieve/entieve/content"
)

// ---------------------------------------------------------------------------
// CmdSetAmmo: per-(owner, slot) preferred charge, consumed first, with
// automatic fallback and strict validation.
// ---------------------------------------------------------------------------

// ammoSlot is the player's world-fit EMP turret slot.
func ammoSlot() SlotRef { return SlotRef{Class: content.SlotHigh, Index: 0} }

// setAmmo queues and applies one CmdSetAmmo, returning the drained events.
func setAmmo(s *Sim, item content.ItemID) []Event {
	s.Queue(CmdSetAmmo{Slot: ammoSlot(), Item: item})
	s.Step()
	return drain(s)
}

func TestCmdSetAmmoPreferredChargeFirst(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	forceLock(s, npc)
	giveCargo(s, "charge-ap", 5)
	// Canonical order would pick charge-emp first (it is the world's first
	// stack): the preference must flip consumption to charge-ap.

	evs := setAmmo(s, "charge-ap")
	if findEvent(evs, EvModule, "EMP Turret loaded with AP Charge") == nil {
		t.Fatalf("missing EvModule load event: %v", evs)
	}
	if got := s.ammoPref(s.player, ammoSlot()); got != "charge-ap" {
		t.Fatalf("pref=%q want charge-ap", got)
	}

	s.Queue(CmdActivate{Slot: ammoSlot(), Target: npc})
	s.Step()
	drain(s)
	if got := cargoCount(s, "charge-ap"); got != 4 {
		t.Fatalf("preferred charge=%d want 4 (consumed first)", got)
	}
	if got := cargoCount(s, "charge-emp"); got != 20 {
		t.Fatalf("emp=%d want 20 (auto order would have used it)", got)
	}
}

func TestCmdSetAmmoFallsBackWhenPreferredRunsOut(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	forceLock(s, npc)
	giveCargo(s, "charge-ap", 1)

	setAmmo(s, "charge-ap")
	// Shot 1: the single preferred AP charge.
	s.Queue(CmdActivate{Slot: ammoSlot(), Target: npc})
	s.Step()
	drain(s)
	if got := cargoCount(s, "charge-ap"); got != 0 {
		t.Fatalf("ap=%d want 0", got)
	}
	// Shot 2: preference exhausted -> automatic fallback (canonical first
	// stack = charge-emp), no warning required.
	s.Queue(CmdDeactivate{Slot: ammoSlot()})
	s.Step()
	drain(s)
	s.Queue(CmdActivate{Slot: ammoSlot(), Target: npc})
	s.Step()
	drain(s)
	if got := cargoCount(s, "charge-emp"); got != 19 {
		t.Fatalf("emp=%d want 19 (fallback charge)", got)
	}
	if got := cargoCount(s, "charge-ap"); got != 0 {
		t.Fatalf("ap=%d want 0", got)
	}
	// The preference itself survives the exhaustion.
	if got := s.ammoPref(s.player, ammoSlot()); got != "charge-ap" {
		t.Fatalf("pref=%q want charge-ap (unchanged)", got)
	}
}

func TestCmdSetAmmoResetRestoresAuto(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	forceLock(s, npc)
	giveCargo(s, "charge-ap", 5)

	setAmmo(s, "charge-ap")
	if s.ammoPref(s.player, ammoSlot()) != "charge-ap" {
		t.Fatal("setup: preference not set")
	}

	evs := setAmmo(s, "")
	if findEvent(evs, EvModule, "auto ammo") == nil {
		t.Fatalf("missing auto-ammo EvModule: %v", evs)
	}
	if got := s.ammoPref(s.player, ammoSlot()); got != "" {
		t.Fatalf("pref=%q want \"\" (automatic)", got)
	}

	// Automatic behaviour: first stack in canonical order = charge-emp.
	s.Queue(CmdActivate{Slot: ammoSlot(), Target: npc})
	s.Step()
	drain(s)
	if got := cargoCount(s, "charge-emp"); got != 19 {
		t.Fatalf("emp=%d want 19 (auto consumed canonical first stack)", got)
	}
	if got := cargoCount(s, "charge-ap"); got != 5 {
		t.Fatalf("ap=%d want 5 (untouched)", got)
	}
}

func TestCmdSetAmmoInvalidItemWarnsAndIsIgnored(t *testing.T) {
	// With a preference already established: every rejection must leave it
	// unchanged.
	base := func(t *testing.T) *Sim {
		t.Helper()
		s := newTestSim(t)
		giveCargo(s, "charge-ap", 5)
		setAmmo(s, "charge-ap")
		drain(s)
		return s
	}

	cases := []struct {
		name     string
		item     content.ItemID
		wantWarn string
	}{
		{"unknown item", "charge-nonexistent",
			"Cannot load charge-nonexistent into EMP Turret"},
		{"mismatched ammo class", "charge-cap-cell",
			"Cannot load Cap Booster Cell into EMP Turret"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := base(t)
			evs := setAmmo(s, c.item)
			warn := findEvent(evs, EvLog, c.wantWarn)
			if warn == nil {
				t.Fatalf("missing warning %q: %v", c.wantWarn, evs)
			}
			if warn.Severity != SevWarning {
				t.Fatalf("severity=%d want SevWarning", warn.Severity)
			}
			if got := s.ammoPref(s.player, ammoSlot()); got != "charge-ap" {
				t.Fatalf("pref=%q want charge-ap (state unchanged)", got)
			}
		})
	}

	t.Run("item not in cargo", func(t *testing.T) {
		s := newTestSim(t) // world cargo: emp + cap cells, no AP charges
		evs := setAmmo(s, "charge-ap")
		if findEvent(evs, EvLog, "Cannot load AP Charge into EMP Turret") == nil {
			t.Fatalf("missing warning: %v", evs)
		}
		if got := s.ammoPref(s.player, ammoSlot()); got != "" {
			t.Fatalf("pref=%q want \"\" (ignored)", got)
		}
	})

	t.Run("non-weapon slot", func(t *testing.T) {
		s := base(t)
		web := injectModule(t, s, "mod-stasis-web")
		s.Queue(CmdSetAmmo{Slot: web, Item: "charge-ap"})
		s.Step()
		evs := drain(s)
		if findEvent(evs, EvLog, "Cannot load AP Charge into Stasis Webifier") == nil {
			t.Fatalf("missing warning: %v", evs)
		}
	})
}
