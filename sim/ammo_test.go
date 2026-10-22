package sim

import (
	"testing"

	"github.com/entieve/entieve/content"
)

// ---------------------------------------------------------------------------
// Shell bays: a charge-fed gun loads a full bay from cargo by a load cycle one
// firing cycle long, fires rounds out of the bay without touching cargo, and
// re-resolves its shell type by the selection rule ("exactly one type in
// cargo" auto-loads; 2+ waits for CmdSetAmmo). CmdSetAmmo dumps the bay back
// to cargo and starts the new load — rounds are never destroyed.
// ---------------------------------------------------------------------------

// ammoSlot is the player's world-fit EMP turret slot (bay: 5 rounds).
func ammoSlot() SlotRef { return SlotRef{Class: content.SlotHigh, Index: 0} }

// setAmmo queues and applies one CmdSetAmmo, returning the drained events.
func setAmmo(s *Sim, item content.ItemID) []Event {
	s.Queue(CmdSetAmmo{Slot: ammoSlot(), Item: item})
	s.Step()
	return drain(s)
}

// bayRounds is the rounds loaded in one slot.
func bayRounds(s *Sim, slot SlotRef) int {
	_, rt, ok := s.moduleRuntime(s.player, slot)
	if !ok {
		return 0
	}
	return rt.Clip.Qty
}

// setCargo replaces the player's general cargo (fixture setup).
func setCargo(s *Sim, stacks ...content.CargoStack) {
	cargo, _ := s.Cargos.Get(s.player)
	cargo.Stacks = stacks
}

// TestFreshFitPreloadsTheBay: a fresh fit loads a full bay from cargo before
// the first shot — one quiet "<module> loaded with <shell> (N rounds)" line,
// cargo down by exactly the loaded rounds.
func TestFreshFitPreloadsTheBay(t *testing.T) {
	s := newTestSim(t)
	if got := bayRounds(s, ammoSlot()); got != 5 {
		t.Fatalf("fresh bay = %d rounds, want 5", got)
	}
	if got := cargoCount(s, "ammo-emp-s"); got != 15 {
		t.Fatalf("cargo emp = %d, want 15 (5 loaded)", got)
	}
	if got := ammoTotal(s, "ammo-emp-s"); got != 20 {
		t.Fatalf("total emp = %d, want 20 (conserved)", got)
	}
	if findEvent(drain(s), EvModule, "EMP Turret loaded with EMP Charge (5 rounds)") == nil {
		t.Fatal("missing load line")
	}
}

// TestClipFiresWithoutTouchingCargo: shots come out of the bay; cargo only
// moves when the bay reloads.
func TestClipFiresWithoutTouchingCargo(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	forceLock(s, npc)
	s.Queue(CmdActivate{Slot: ammoSlot(), Target: npc})
	s.Step()
	if got := bayRounds(s, ammoSlot()); got != 4 {
		t.Fatalf("bay after shot = %d, want 4", got)
	}
	if got := cargoCount(s, "ammo-emp-s"); got != 15 {
		t.Fatalf("cargo after shot = %d, want 15 (untouched)", got)
	}
}

// TestBayReloadsWhenEmpty: the sixth shot finds the bay empty, loads a full
// bay from cargo (one firing cycle), and fires from it on the next cycle.
func TestBayReloadsWhenEmpty(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	forceLock(s, npc)
	s.Queue(CmdActivate{Slot: ammoSlot(), Target: npc})
	s.Step()
	drain(s)

	stepN(s, 480) // t=8 s: shots at 2/4/6/8 empty the 5-round bay
	drain(s)
	if got := bayRounds(s, ammoSlot()); got != 0 {
		t.Fatalf("bay = %d rounds, want 0", got)
	}
	if got := cargoCount(s, "ammo-emp-s"); got != 15 {
		t.Fatalf("cargo = %d, want 15 (no reload yet)", got)
	}

	stepN(s, 120) // t=10 s: the cycle reloads instead of firing
	evs := drain(s)
	if got := bayRounds(s, ammoSlot()); got != 5 {
		t.Fatalf("bay after reload = %d, want 5", got)
	}
	if got := cargoCount(s, "ammo-emp-s"); got != 10 {
		t.Fatalf("cargo after reload = %d, want 10", got)
	}
	if hasEvent(evs, EvLog, "Sloop hits") || hasEvent(evs, EvLog, "Sloop misses") {
		t.Fatalf("a load cycle fired a shot: %v", evs)
	}
	if findEvent(evs, EvModule, "EMP Turret loaded with EMP Charge (5 rounds)") == nil {
		t.Fatalf("missing reload line: %v", evs)
	}

	stepN(s, 120) // t=12 s: the deferred shot fires from the new bay
	if got := bayRounds(s, ammoSlot()); got != 4 {
		t.Fatalf("bay after the deferred shot = %d, want 4", got)
	}
	if got := cargoCount(s, "ammo-emp-s"); got != 10 {
		t.Fatalf("cargo after the deferred shot = %d, want 10", got)
	}
}

// TestBayWaitsWithTwoTypes: when the selected type runs out and two other
// types compete, the gun sits dry and waits — one quiet "waiting for ammo
// selection" line, never the dry-gun warning.
func TestBayWaitsWithTwoTypes(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	forceLock(s, npc)
	setCargo(s,
		content.CargoStack{Item: "ammo-emp-s", Qty: 15},
		content.CargoStack{Item: "ammo-fusion-s", Qty: 5},
		content.CargoStack{Item: "ammo-ap-s", Qty: 3},
	)
	setAmmo(s, "ammo-ap-s")
	if got := bayRounds(s, ammoSlot()); got != 3 {
		t.Fatalf("bay = %d AP rounds, want 3", got)
	}

	s.Queue(CmdActivate{Slot: ammoSlot(), Target: npc})
	s.Step()
	drain(s)
	// The three AP rounds fire at t=0/2/4; at t=6 the empty bay finds two
	// types competing.
	stepN(s, 360)
	evs := drain(s)
	if got := bayRounds(s, ammoSlot()); got != 0 {
		t.Fatalf("bay = %d, want 0", got)
	}
	if findEvent(evs, EvLog, "EMP Turret waiting for ammo selection") == nil {
		t.Fatalf("missing waiting line: %v", evs)
	}
	if hasEvent(evs, EvLog, "no charges") {
		t.Fatalf("the waiting state must not use the dry-gun warning: %v", evs)
	}
	if rt, _ := hudModule(s, ammoSlot()); rt.Active {
		t.Fatal("a waiting gun deactivates (quietly)")
	}
	if got := ammoTotal(s, "ammo-emp-s"); got != 20 {
		t.Fatalf("EMP rounds = %d, want 20 (dumped back, never lost)", got)
	}
	// Waiting is readable: empty bay, no selection, both types offered.
	cs, ok := s.Clip(s.player, ammoSlot())
	if !ok || cs.Rounds != 0 || cs.Selection != "" || len(cs.Options) != 2 {
		t.Fatalf("clip state = %+v, want empty/waiting with 2 options", cs)
	}
}

// TestBayAutoSwitchesToTheLastType: the selected type running out with
// exactly one other type in cargo switches automatically.
func TestBayAutoSwitchesToTheLastType(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	forceLock(s, npc)
	giveCargo(s, "ammo-ap-s", 2)
	setAmmo(s, "ammo-ap-s")
	// Only fusion is left in cargo: once AP runs out it is the single
	// remaining type, so the bay switches to it by itself.
	setCargo(s, content.CargoStack{Item: "ammo-fusion-s", Qty: 5})

	s.Queue(CmdActivate{Slot: ammoSlot(), Target: npc})
	s.Step()
	drain(s)
	// The two AP rounds fire at t=0 and t=2; at t=4 the empty bay finds
	// fusion as the only remaining type and loads it.
	stepN(s, 240)
	evs := drain(s)
	if findEvent(evs, EvModule, "EMP Turret loaded with Fusion Charge (5 rounds)") == nil {
		t.Fatalf("missing auto-switch load: %v", evs)
	}
	if got := ammoTotal(s, "ammo-fusion-s"); got != 5 {
		t.Fatalf("fusion = %d, want 5 (conserved)", got)
	}
	if got := s.ammoPref(s.player, ammoSlot()); got != "ammo-fusion-s" {
		t.Fatalf("selection = %q, want ammo-fusion-s", got)
	}
	// And the gun is loaded and firing again.
	if got := bayRounds(s, ammoSlot()); got != 5 {
		t.Fatalf("auto-switch loaded %d rounds, want 5", got)
	}
}

// TestCmdSetAmmoDumpsAndReloads: switching shells dumps the loaded rounds
// back to cargo, loads the new type immediately and keeps the module active.
func TestCmdSetAmmoDumpsAndReloads(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	forceLock(s, npc)
	giveCargo(s, "ammo-ap-s", 4)

	s.Queue(CmdActivate{Slot: ammoSlot(), Target: npc})
	s.Step()
	drain(s)

	before := ammoTotal(s, "ammo-emp-s")
	evs := setAmmo(s, "ammo-ap-s")
	if findEvent(evs, EvModule, "EMP Turret loaded with AP Charge (4 rounds)") == nil {
		t.Fatalf("missing load line: %v", evs)
	}
	// The five EMP rounds went back to cargo; the AP rounds loaded.
	if got := ammoTotal(s, "ammo-emp-s"); got != before {
		t.Fatalf("emp total = %d, want %d (dumped, not lost)", got, before)
	}
	if got := cargoCount(s, "ammo-emp-s"); got != 19 {
		t.Fatalf("emp cargo = %d, want 19 (4 loaded rounds dumped back)", got)
	}
	if got := bayRounds(s, ammoSlot()); got != 4 {
		t.Fatalf("bay = %d AP rounds, want 4", got)
	}
	if got := s.ammoPref(s.player, ammoSlot()); got != "ammo-ap-s" {
		t.Fatalf("selection = %q", got)
	}
	if rt, _ := hudModule(s, ammoSlot()); !rt.Active {
		t.Fatal("module must stay Active through the switch")
	}
	// The load cycle defers the next shot by one firing cycle, then it fires
	// from the new bay.
	if got := bayRounds(s, ammoSlot()); got != 4 {
		t.Fatalf("bay = %d, want 4 until the deferred shot", got)
	}
}

// TestCmdSetAmmoClearDumpsAndWaits: "" clears the selection, dumps the bay to
// cargo and — with two types competing — waits for a choice instead of
// following cargo order.
func TestCmdSetAmmoClearDumpsAndWaits(t *testing.T) {
	s := newTestSim(t)
	giveCargo(s, "ammo-fusion-s", 5)
	evs := setAmmo(s, "")
	if s.ammoPref(s.player, ammoSlot()) != "" {
		t.Fatalf("selection = %q, want none", s.ammoPref(s.player, ammoSlot()))
	}
	if got := bayRounds(s, ammoSlot()); got != 0 {
		t.Fatalf("bay = %d, want 0 (dumped)", got)
	}
	// EMP (dumped, 20) + fusion (5): two types, so no auto-load.
	if got := cargoCount(s, "ammo-emp-s"); got != 20 {
		t.Fatalf("emp cargo = %d, want 20", got)
	}
	if findEvent(evs, EvLog, "EMP Turret waiting for ammo selection") == nil {
		t.Fatalf("missing waiting line: %v", evs)
	}
	if findEvent(evs, EvModule, "EMP Turret ammo selection cleared") == nil {
		t.Fatalf("missing clear event: %v", evs)
	}

	// One type only: clearing auto-loads it again (the bay rule).
	s2 := newTestSim(t)
	_, rt, _ := s2.moduleRuntime(s2.player, ammoSlot())
	s2.dumpClip(s2.player, rt) // empty the bay first so cargo holds one type
	setCargo(s2, content.CargoStack{Item: "ammo-ap-s", Qty: 7})
	setAmmo(s2, "")
	if got := bayRounds(s2, ammoSlot()); got != 5 {
		t.Fatalf("bay = %d AP rounds, want 5 (auto after clear)", got)
	}
	if got := s2.ammoPref(s2.player, ammoSlot()); got != "ammo-ap-s" {
		t.Fatalf("selection = %q, want ammo-ap-s", got)
	}
}

// TestCmdSetAmmoInvalidItemWarnsAndIsIgnored: every rejection leaves the bay,
// the selection and cargo untouched.
func TestCmdSetAmmoInvalidItemWarnsAndIsIgnored(t *testing.T) {
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
				t.Fatalf("selection=%q want ammo-ap-s (state unchanged)", got)
			}
			if got := bayRounds(s, ammoSlot()); got != 5 {
				t.Fatalf("bay=%d want 5 (state unchanged)", got)
			}
		})
	}

	t.Run("item not in cargo", func(t *testing.T) {
		s := newTestSim(t) // world cargo: emp + cap cells, no AP charges
		evs := setAmmo(s, "ammo-ap-s")
		if findEvent(evs, EvLog, "Cannot load AP Charge into EMP Turret") == nil {
			t.Fatalf("missing warning: %v", evs)
		}
		if got := s.ammoPref(s.player, ammoSlot()); got != "ammo-emp-s" {
			t.Fatalf("selection=%q want ammo-emp-s (unchanged)", got)
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

// TestAmmoFamiliesDoNotCrossLoad pins the family rule at the firing path: a
// medium-calibre gun stays dry while the hold carries small-family shells.
func TestAmmoFamiliesDoNotCrossLoad(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	forceLock(s, npc)

	// A medium-family gun and one of its shells alongside the small ones.
	s.Content.Items["ammo-emp-m"] = &content.ItemDef{
		ID: "ammo-emp-m", Name: "Medium EMP Shell", Category: content.CatAmmo,
		Volume: 0.01, BasePrice: 10, AmmoClass: "medium-autocannon",
		Damage: &content.Damage{EM: 20},
	}
	s.Content.Modules["mod-autocannon-m"] = &content.ModuleDef{
		ID: "mod-autocannon-m", Name: "Medium Autocannon", Slot: content.SlotHigh,
		Kind: content.KindWeapon, ActivationTime: 3,
		Weapon: &content.WeaponAttrs{
			Damage: content.Damage{EM: 20}, Optimal: 10000, Falloff: 9000,
			Tracking: 0.4, AmmoClass: "medium-autocannon", ShotTime: 3, BayM3: 0.1,
		},
	}
	slot := injectModule(t, s, "mod-autocannon-m")
	smallBefore := ammoTotal(s, "ammo-emp-s")

	// Small shells aboard: the medium gun refuses to fire.
	setAmmo(s, "ammo-emp-s") // wrong family -> ignored, bay stays empty
	s.Queue(CmdActivate{Slot: slot, Target: npc})
	s.Step()
	evs := drain(s)
	if !hasEvent(evs, EvLog, "no charges") {
		t.Fatalf("medium gun fired on small shells: %v", evs)
	}
	if got := ammoTotal(s, "ammo-emp-s"); got != smallBefore {
		t.Fatalf("small shells = %d, want %d (untouched by a medium gun)", got, smallBefore)
	}

	// The matching family loads and is consumed from its bay.
	giveCargo(s, "ammo-emp-m", 1)
	setAmmo(s, "ammo-emp-m")
	s.Queue(CmdActivate{Slot: slot, Target: npc})
	s.Step()
	drain(s)
	if got := cargoCount(s, "ammo-emp-m"); got != 0 {
		t.Fatalf("medium shell = %d, want 0 (loaded into the bay)", got)
	}
	if got := bayRounds(s, slot); got != 0 {
		t.Fatalf("medium bay = %d, want 0 (one round fired)", got)
	}
	if got := ammoTotal(s, "ammo-emp-s"); got != smallBefore {
		t.Fatalf("small shells = %d, want %d (never cross-loaded)", got, smallBefore)
	}
}

// TestNpcBayAutoLoads: NPC charges resolve to auto with no interaction — the
// single granted type loads its bay and the rat fires.
func TestNpcBayAutoLoads(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	s.Step() // the bay pass resolves the rat's single shell type

	fit, _ := s.Fittings.Get(npc)
	if len(fit.HighRT) == 0 {
		t.Fatal("rat has no high module")
	}
	if fit.HighRT[0].Clip.Qty == 0 || fit.HighRT[0].Clip.Item == "" {
		t.Fatalf("rat bay = %+v, want loaded", fit.HighRT[0].Clip)
	}
	if got := fit.HighRT[0].Selection; got == "" {
		t.Fatal("rat bay has no selection after auto-load")
	}
}

// TestWreckCarriesClipRounds: a destroyed ship's loaded rounds are part of
// its wreck; the rest of its cargo is not (unchanged behaviour).
func TestWreckCarriesClipRounds(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	s.Step() // load the rat's bay
	fit, _ := s.Fittings.Get(npc)
	fit.HighRT[0].Active = false // freeze its gun: the bay at death is known
	fit.HighRT[0].CycleEnds = s.Time() + 1e6
	loaded := fit.HighRT[0].Clip
	if loaded.Qty == 0 {
		t.Fatal("fixture: rat bay never loaded")
	}

	damageNPC(s, s.player, npc, content.Damage{EM: 1e6}, false)
	s.Step()

	var wreck Wreck
	found := false
	for _, e := range sortedEntities(s.Wrecks) {
		w, ok := s.Wrecks.Get(e)
		if ok {
			wreck, found = *w, true
			break
		}
	}
	if !found {
		t.Fatal("no wreck")
	}
	if got := stacksCount(wreck.Loot, loaded.Item); got != loaded.Qty {
		t.Fatalf("wreck carries %d x %s, want %d (the loaded bay)", got, loaded.Item, loaded.Qty)
	}
}

// TestUnfitReturnsClipToCargo: unfitting a gun dumps its loaded rounds back
// into cargo; nothing is destroyed.
func TestUnfitReturnsClipToCargo(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s) // the fitting service
	if got := bayRounds(s, ammoSlot()); got != 5 {
		t.Fatalf("fixture: bay = %d, want 5", got)
	}
	before := ammoTotal(s, "ammo-emp-s")

	s.Queue(CmdUnfitModule{Slot: ammoSlot()})
	s.Step()
	drain(s)
	if got := cargoCount(s, "ammo-emp-s"); got != before {
		t.Fatalf("cargo emp = %d, want %d (bay returned)", got, before)
	}
	fit, _ := s.Fittings.Get(s.player)
	for _, rt := range fit.HighRT {
		if rt.Clip.Qty != 0 {
			t.Fatalf("unfitted bay still holds %+v", rt.Clip)
		}
	}
}

// TestNpcChargeGrantPerFamily: an NPC spawn grant is one stack per distinct
// shell family in its fit. The shipped Hierarch carries large artillery and
// large autocannons, so it gets two grants (one per family), and the cap
// booster it also fits still gets no cells — a pre-existing gap, unchanged.
func TestNpcChargeGrantPerFamily(t *testing.T) {
	s := &Sim{Content: content.MustLoad()}
	hierarch, ok := s.Content.Npc("npc-rat-hierarch")
	if !ok {
		t.Fatal("missing npc-rat-hierarch")
	}
	got := s.npcCharges(hierarch.Fit)
	if len(got) != 2 {
		t.Fatalf("hierarch grants = %+v, want 2 (large artillery + large autocannon)", got)
	}
	families := map[string]bool{}
	for _, st := range got {
		def, ok := s.Content.Item(st.Item)
		if !ok {
			t.Fatalf("grant names unknown item %s", st.Item)
		}
		if st.Qty != 1000 {
			t.Errorf("grant %s = %d rounds, want 1000", st.Item, st.Qty)
		}
		families[def.AmmoClass] = true
		if def.ID == capCellID {
			t.Error("cap booster granted cells (pre-existing gap must stay unchanged)")
		}
	}
	if !families["large-artillery"] || !families["large-autocannon"] {
		t.Fatalf("granted families = %v", families)
	}
}
