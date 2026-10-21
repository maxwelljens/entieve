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
	giveCargo(s, "ammo-ap-s", 5)
	// Canonical order would pick ammo-emp-s first (it is the world's first
	// stack): the preference must flip consumption to ammo-ap-s.

	evs := setAmmo(s, "ammo-ap-s")
	if findEvent(evs, EvModule, "EMP Turret loaded with AP Charge") == nil {
		t.Fatalf("missing EvModule load event: %v", evs)
	}
	if got := s.ammoPref(s.player, ammoSlot()); got != "ammo-ap-s" {
		t.Fatalf("pref=%q want ammo-ap-s", got)
	}

	s.Queue(CmdActivate{Slot: ammoSlot(), Target: npc})
	s.Step()
	drain(s)
	if got := cargoCount(s, "ammo-ap-s"); got != 4 {
		t.Fatalf("preferred charge=%d want 4 (consumed first)", got)
	}
	if got := cargoCount(s, "ammo-emp-s"); got != 20 {
		t.Fatalf("emp=%d want 20 (auto order would have used it)", got)
	}
}

func TestCmdSetAmmoFallsBackWhenPreferredRunsOut(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	forceLock(s, npc)
	giveCargo(s, "ammo-ap-s", 1)

	setAmmo(s, "ammo-ap-s")
	// Shot 1: the single preferred AP charge.
	s.Queue(CmdActivate{Slot: ammoSlot(), Target: npc})
	s.Step()
	drain(s)
	if got := cargoCount(s, "ammo-ap-s"); got != 0 {
		t.Fatalf("ap=%d want 0", got)
	}
	// Shot 2: preference exhausted -> automatic fallback (canonical first
	// stack = ammo-emp-s), no warning required. The cancelled cycle still owns
	// its clock (docs/SIM.md §2), so the next activation waits it out.
	s.Queue(CmdDeactivate{Slot: ammoSlot()})
	s.Step()
	drain(s)
	stepN(s, 120) // the 2 s shot time runs out while the gun is off
	s.Queue(CmdActivate{Slot: ammoSlot(), Target: npc})
	s.Step()
	drain(s)
	if got := cargoCount(s, "ammo-emp-s"); got != 19 {
		t.Fatalf("emp=%d want 19 (fallback charge)", got)
	}
	if got := cargoCount(s, "ammo-ap-s"); got != 0 {
		t.Fatalf("ap=%d want 0", got)
	}
	// The preference itself survives the exhaustion.
	if got := s.ammoPref(s.player, ammoSlot()); got != "ammo-ap-s" {
		t.Fatalf("pref=%q want ammo-ap-s (unchanged)", got)
	}
}

func TestCmdSetAmmoResetRestoresAuto(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	forceLock(s, npc)
	giveCargo(s, "ammo-ap-s", 5)

	setAmmo(s, "ammo-ap-s")
	if s.ammoPref(s.player, ammoSlot()) != "ammo-ap-s" {
		t.Fatal("setup: preference not set")
	}

	evs := setAmmo(s, "")
	if findEvent(evs, EvModule, "auto ammo") == nil {
		t.Fatalf("missing auto-ammo EvModule: %v", evs)
	}
	if got := s.ammoPref(s.player, ammoSlot()); got != "" {
		t.Fatalf("pref=%q want \"\" (automatic)", got)
	}

	// Automatic behaviour: first stack in canonical order = ammo-emp-s.
	s.Queue(CmdActivate{Slot: ammoSlot(), Target: npc})
	s.Step()
	drain(s)
	if got := cargoCount(s, "ammo-emp-s"); got != 19 {
		t.Fatalf("emp=%d want 19 (auto consumed canonical first stack)", got)
	}
	if got := cargoCount(s, "ammo-ap-s"); got != 5 {
		t.Fatalf("ap=%d want 5 (untouched)", got)
	}
}

func TestCmdSetAmmoInvalidItemWarnsAndIsIgnored(t *testing.T) {
	// With a preference already established: every rejection must leave it
	// unchanged.
	base := func(t *testing.T) *Sim {
		t.Helper()
		s := newTestSim(t)
		giveCargo(s, "ammo-ap-s", 5)
		setAmmo(s, "ammo-ap-s")
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
			if got := s.ammoPref(s.player, ammoSlot()); got != "ammo-ap-s" {
				t.Fatalf("pref=%q want ammo-ap-s (state unchanged)", got)
			}
		})
	}

	t.Run("item not in cargo", func(t *testing.T) {
		s := newTestSim(t) // world cargo: emp + cap cells, no AP charges
		evs := setAmmo(s, "ammo-ap-s")
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
		s.Queue(CmdSetAmmo{Slot: web, Item: "ammo-ap-s"})
		s.Step()
		evs := drain(s)
		if findEvent(evs, EvLog, "Cannot load AP Charge into Stasis Webifier") == nil {
			t.Fatalf("missing warning: %v", evs)
		}
	})
}

// TestAmmoSizesDoNotCrossLoad pins the calibre rule at the firing path: a
// gun only ever consumes shells of its own ammo class, so a medium gun stays
// dry while the hold carries small shells only.
func TestAmmoSizesDoNotCrossLoad(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	forceLock(s, npc)

	// A medium-calibre gun and one medium shell alongside the small ones.
	s.Content.Items["ammo-emp-m"] = &content.ItemDef{
		ID: "ammo-emp-m", Name: "Medium EMP Shell", Category: content.CatAmmo,
		Volume: 0.01, BasePrice: 10, AmmoClass: "medium",
		Damage: &content.Damage{EM: 20},
	}
	s.Content.Modules["mod-autocannon-m"] = &content.ModuleDef{
		ID: "mod-autocannon-m", Name: "Medium Autocannon", Slot: content.SlotHigh,
		Kind: content.KindWeapon, ActivationTime: 3,
		Weapon: &content.WeaponAttrs{
			Damage: content.Damage{EM: 20}, Optimal: 10000, Falloff: 9000,
			Tracking: 0.4, AmmoClass: "medium", ShotTime: 3,
		},
	}
	slot := injectModule(t, s, "mod-autocannon-m")
	smallBefore := cargoCount(s, "ammo-emp-s")

	// Small shells aboard: the medium gun refuses to fire.
	setAmmo(s, "ammo-emp-s") // preference does not exist -> ignored, auto stays
	s.Queue(CmdActivate{Slot: slot, Target: npc})
	s.Step()
	evs := drain(s)
	if !hasEvent(evs, EvLog, "no charges") {
		t.Fatalf("medium gun fired on small shells: %v", evs)
	}
	if got := cargoCount(s, "ammo-emp-s"); got != smallBefore {
		t.Fatalf("small shells = %d, want %d (untouched by a medium gun)", got, smallBefore)
	}

	// The matching calibre loads and is consumed.
	giveCargo(s, "ammo-emp-m", 1)
	setAmmo(s, "ammo-emp-m")
	s.Queue(CmdActivate{Slot: slot, Target: npc})
	s.Step()
	drain(s)
	if got := cargoCount(s, "ammo-emp-m"); got != 0 {
		t.Fatalf("medium shell = %d, want 0 (consumed)", got)
	}
	if got := cargoCount(s, "ammo-emp-s"); got != smallBefore {
		t.Fatalf("small shells = %d, want %d (never cross-loaded)", got, smallBefore)
	}
}
