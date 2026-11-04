package sim

import (
	"testing"

	"github.com/entieve/entieve/content"
)

// ---------------------------------------------------------------------------
// Shell bays: a charge-fed gun loads a full bay from cargo by a load cycle one
// firing cycle long, fires rounds out of the bay without touching cargo, and
// loads ONLY the standing order (CmdSetAmmo). With no order a bay loads
// nothing, whatever cargo holds; the order survives an empty bay and a
// stock-out. CmdSetAmmo dumps the bay back to cargo and starts the new load —
// rounds are never destroyed.
// ---------------------------------------------------------------------------

// ammoSlot is the player's world-fit EMP turret slot (bay: 5 rounds).
func ammoSlot() SlotRef { return SlotRef{Class: content.SlotHigh, Index: 0} }

// setAmmo queues and applies one CmdSetAmmo, returning the drained events.
func setAmmo(s *Sim, item content.ItemID) []Event {
	s.Queue(CmdSetAmmo{Slot: ammoSlot(), Item: item})
	s.Step()
	return drain(s)
}

// loadBay settles the player's world-fit bay the way a completed CmdSetAmmo
// leaves it — the picked type in the bay, the order written — without
// spending the load cycle. For tests about something other than the load; the
// pick and its cycle are pinned here.
func loadBay(t *testing.T, s *Sim, item content.ItemID) {
	t.Helper()
	def, rt, ok := s.moduleDef(s.player, ammoSlot())
	if !ok {
		t.Fatal("fixture has no EMP turret slot")
	}
	cargo, ok := s.Cargos.Get(s.player)
	if !ok {
		t.Fatal("fixture has no cargo")
	}
	if n := s.loadClip(cargo, def, rt, item); n <= 0 {
		t.Fatalf("fixture could not load %s", item)
	}
	rt.CycleEnds = 0
	s.finishLoad(s.player, def, rt)
	drain(s)
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

// TestFreshFitLoadsNothing: a fresh fit starts with empty bays. One round type
// in cargo is not an order, so nothing loads, no load line fires and the rack
// reads waiting (0 rounds, no selection, the type offered).
func TestFreshFitLoadsNothing(t *testing.T) {
	s := newTestSim(t)
	if got := bayRounds(s, ammoSlot()); got != 0 {
		t.Fatalf("fresh bay = %d rounds, want 0 (no pre-load)", got)
	}
	if got := cargoCount(s, "ammo-emp-s"); got != 20 {
		t.Fatalf("cargo emp = %d, want 20 (nothing moved)", got)
	}
	cs, ok := s.Clip(s.player, ammoSlot())
	if !ok || cs.Rounds != 0 || cs.Selection != "" || len(cs.Options) != 1 {
		t.Fatalf("clip at boot = %+v, want empty and waiting on the one type", cs)
	}
	if findEvent(drain(s), EvModule, "loaded with") != nil {
		t.Fatal("a fresh fit logged a load")
	}
	// A tick of idle time changes nothing: the bay pass never picks a type.
	stepN(s, 240)
	if got := bayRounds(s, ammoSlot()); got != 0 {
		t.Fatalf("bay after idle ticks = %d, want 0", got)
	}
	if got := cargoCount(s, "ammo-emp-s"); got != 20 {
		t.Fatalf("cargo emp after idle ticks = %d, want 20", got)
	}
}

// TestClipFiresWithoutTouchingCargo: shots come out of the bay; cargo only
// moves when the bay reloads.
func TestClipFiresWithoutTouchingCargo(t *testing.T) {
	s := newTestSim(t)
	loadBay(t, s, "ammo-emp-s")
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
	loadBay(t, s, "ammo-emp-s")
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

	stepN(s, 120) // t=10 s: the cycle loads instead of firing (rounds move in)
	evs := drain(s)
	if got := bayRounds(s, ammoSlot()); got != 5 {
		t.Fatalf("bay after reload start = %d, want 5", got)
	}
	if got := cargoCount(s, "ammo-emp-s"); got != 10 {
		t.Fatalf("cargo after reload start = %d, want 10", got)
	}
	if hasEvent(evs, EvLog, "Sloop hits") || hasEvent(evs, EvLog, "Sloop misses") {
		t.Fatalf("a load cycle fired a shot: %v", evs)
	}
	if findEvent(evs, EvModule, "loaded with") != nil {
		t.Fatalf("the load line fired at the start, want it at completion: %v", evs)
	}

	stepN(s, 120) // t=12 s: the load completes and the deferred shot fires
	if findEvent(drain(s), EvModule, "EMP Turret loaded with EMP Charge (5 rounds)") == nil {
		t.Fatal("missing completion line")
	}
	if got := bayRounds(s, ammoSlot()); got != 4 {
		t.Fatalf("bay after the deferred shot = %d, want 4", got)
	}
	if got := cargoCount(s, "ammo-emp-s"); got != 10 {
		t.Fatalf("cargo after the deferred shot = %d, want 10", got)
	}
}

// TestBayWithoutAnOrderWaits: an empty bay with no standing order loads
// nothing, however much cargo fits it — it waits for the player's pick with
// one quiet "waiting for ammo selection" line, never the dry-gun warning.
func TestBayWithoutAnOrderWaits(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	forceLock(s, npc)
	setCargo(s,
		content.CargoStack{Item: "ammo-emp-s", Qty: 20},
		content.CargoStack{Item: "ammo-fusion-s", Qty: 5},
		content.CargoStack{Item: "ammo-ap-s", Qty: 3},
	)

	s.Queue(CmdActivate{Slot: ammoSlot(), Target: npc})
	s.Step()
	evs := drain(s)
	if got := bayRounds(s, ammoSlot()); got != 0 {
		t.Fatalf("bay = %d, want 0 (no order, no load)", got)
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
	// No stack moved: the bay guessed nothing.
	if got := ammoTotal(s, "ammo-emp-s"); got != 20 {
		t.Fatalf("EMP rounds = %d, want 20 (never touched)", got)
	}
	if got := ammoTotal(s, "ammo-ap-s"); got != 3 {
		t.Fatalf("AP rounds = %d, want 3 (never touched)", got)
	}
	// Waiting is readable: empty bay, no selection, every type offered.
	cs, ok := s.Clip(s.player, ammoSlot())
	if !ok || cs.Rounds != 0 || cs.Selection != "" || len(cs.Options) != 3 {
		t.Fatalf("clip state = %+v, want empty/waiting with 3 options", cs)
	}
}

// TestBayKeepsTheOrderThroughAStockOut: the selected type running out does not
// re-aim the bay, not even when exactly one other type remains in cargo — the
// old auto-switch. The order survives, the bay stays empty while the type is
// gone, and the load resumes by itself when the type is back.
func TestBayKeepsTheOrderThroughAStockOut(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	forceLock(s, npc)
	giveCargo(s, "ammo-ap-s", 2)
	setAmmo(s, "ammo-ap-s")
	// Only fusion is left in cargo: under the retired rule it would load.
	setCargo(s, content.CargoStack{Item: "ammo-fusion-s", Qty: 5})

	s.Queue(CmdActivate{Slot: ammoSlot(), Target: npc})
	s.Step()
	drain(s)
	// The two AP rounds fire at t=2 and t=4; at t=6 the bay is empty and the
	// order's type is out of cargo: the gun goes dry, fusion is never touched.
	stepN(s, 360)
	evs := drain(s)
	if got := bayRounds(s, ammoSlot()); got != 0 {
		t.Fatalf("bay = %d, want 0 (no AP left)", got)
	}
	if findEvent(evs, EvLog, "no charges") == nil {
		t.Fatalf("missing dry-gun warning for the order's type: %v", evs)
	}
	if findEvent(evs, EvLog, "waiting for ammo selection") != nil {
		t.Fatalf("an order is set; the bay must not ask for a selection: %v", evs)
	}
	if got := s.ammoPref(s.player, ammoSlot()); got != "ammo-ap-s" {
		t.Fatalf("order = %q, want ammo-ap-s (kept through the stock-out)", got)
	}
	if got := ammoTotal(s, "ammo-fusion-s"); got != 5 {
		t.Fatalf("fusion = %d, want 5 (never loaded)", got)
	}

	// The type comes back: the idle bay resumes the order without a new pick.
	giveCargo(s, "ammo-ap-s", 2)
	s.Step()
	if got := bayRounds(s, ammoSlot()); got != 2 {
		t.Fatalf("bay after the restock = %d, want 2 (the order resumed)", got)
	}
	stepN(s, 120) // the load cycle
	if findEvent(drain(s), EvModule, "EMP Turret loaded with AP Charge (2 rounds)") == nil {
		t.Fatal("missing the resumed load's completion line")
	}
	if got := s.ammoPref(s.player, ammoSlot()); got != "ammo-ap-s" {
		t.Fatalf("selection = %q, want ammo-ap-s", got)
	}
}

// TestCmdSetAmmoDumpsAndReloads: switching shells dumps the loaded rounds
// back to cargo, loads the new type immediately and keeps the module active.
func TestCmdSetAmmoDumpsAndReloads(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	forceLock(s, npc)
	loadBay(t, s, "ammo-emp-s")
	giveCargo(s, "ammo-ap-s", 4)

	s.Queue(CmdActivate{Slot: ammoSlot(), Target: npc})
	s.Step()
	drain(s)

	before := ammoTotal(s, "ammo-emp-s")
	evs := setAmmo(s, "ammo-ap-s")
	// The switch is a load like any other: the AP rounds are in at once and
	// the bay is not ready until the load's cycle completes.
	if findEvent(evs, EvModule, "loaded with") != nil {
		t.Fatalf("the load line fired at the start, want it at completion: %v", evs)
	}
	if cs, _ := s.Clip(s.player, ammoSlot()); !cs.Loading {
		t.Fatalf("clip = %+v, want a load in flight", cs)
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
	if rt, _ := hudModule(s, ammoSlot()); !rt.Active {
		t.Fatal("module must stay Active through the switch")
	}
	// The load cycle completes and the deferred shot fires from the new bay.
	stepN(s, 120)
	if findEvent(drain(s), EvModule, "EMP Turret loaded with AP Charge (4 rounds)") == nil {
		t.Fatal("missing completion line")
	}
	if got := s.ammoPref(s.player, ammoSlot()); got != "ammo-ap-s" {
		t.Fatalf("selection = %q", got)
	}
	if got := bayRounds(s, ammoSlot()); got != 3 {
		t.Fatalf("bay = %d after the deferred shot, want 3", got)
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

	// One type only: clearing still loads nothing — one type is not an order.
	// The bay stays empty and waiting until an explicit pick.
	s2 := newTestSim(t)
	_, rt, _ := s2.moduleRuntime(s2.player, ammoSlot())
	s2.dumpClip(s2.player, rt) // empty the bay first so cargo holds one type
	setCargo(s2, content.CargoStack{Item: "ammo-ap-s", Qty: 7})
	evs = setAmmo(s2, "")
	if got := bayRounds(s2, ammoSlot()); got != 0 {
		t.Fatalf("bay = %d AP rounds, want 0 (one type is not an order)", got)
	}
	if findEvent(evs, EvLog, "EMP Turret waiting for ammo selection") == nil {
		t.Fatalf("missing waiting line for the lone type: %v", evs)
	}
	setAmmo(s2, "ammo-ap-s") // the explicit pick
	if got := bayRounds(s2, ammoSlot()); got != 5 {
		t.Fatalf("bay = %d AP rounds, want 5 (loading after the pick)", got)
	}
	if cs, _ := s2.Clip(s2.player, ammoSlot()); !cs.Loading {
		t.Fatalf("clip = %+v, want a load in flight", cs)
	}
	stepN(s2, 120)
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
		stepN(s, 120) // let the switch's load complete: a settled bay
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
		if got := s.ammoPref(s.player, ammoSlot()); got != "" {
			t.Fatalf("selection=%q want none (unchanged: a fresh fit has no order)", got)
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

	// The matching family loads and is consumed from its bay. The load takes
	// the gun's cycle, so the one round is spent after it completes.
	giveCargo(s, "ammo-emp-m", 1)
	s.Queue(CmdSetAmmo{Slot: slot, Item: "ammo-emp-m"})
	s.Step()
	s.Queue(CmdActivate{Slot: slot, Target: npc})
	s.Step()
	drain(s)
	if got := cargoCount(s, "ammo-emp-m"); got != 0 {
		t.Fatalf("medium shell = %d, want 0 (loaded into the bay)", got)
	}
	if got := bayRounds(s, slot); got != 1 {
		t.Fatalf("medium bay = %d, want 1 (in at the load's start)", got)
	}
	stepN(s, 180) // the 3 s load cycle completes and the shot fires
	if got := bayRounds(s, slot); got != 0 {
		t.Fatalf("medium bay = %d, want 0 (one round fired)", got)
	}
	if got := ammoTotal(s, "ammo-emp-s"); got != smallBefore {
		t.Fatalf("small shells = %d, want %d (never cross-loaded)", got, smallBefore)
	}
}

// TestNpcBayAutoLoads: NPC charges resolve to auto with no interaction — the
// single granted type's load starts on the spawn tick and completes one cycle
// later, and the rat fires from it.
func TestNpcBayAutoLoads(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	s.Step() // the bay pass starts the rat's single shell type

	fit, _ := s.Fittings.Get(npc)
	if len(fit.HighRT) == 0 {
		t.Fatal("rat has no high module")
	}
	// Take at start: the rounds are in, the selection lands at completion.
	if fit.HighRT[0].Clip.Qty == 0 || fit.HighRT[0].Clip.Item == "" {
		t.Fatalf("rat bay = %+v, want rounds in at the load's start", fit.HighRT[0].Clip)
	}
	if cs, ok := s.Clip(npc, SlotRef{Class: content.SlotHigh, Index: 0}); !ok || !cs.Loading {
		t.Fatalf("rat clip = %+v, want a load in flight", cs)
	}
	stepN(s, 130) // the 2 s load cycle
	if got := fit.HighRT[0].Selection; got == "" {
		t.Fatal("rat bay has no selection after its load completed")
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
	loadBay(t, s, "ammo-emp-s")
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

// TestClipStateCarriesTheBaySpec pins the read-only spec fields the UI reads
// instead of re-deriving the bay rule (the rack tooltip's bay row): the bay's
// m³ and the load cycle are the ones the load itself runs on, and the capacity
// is the bay in whole rounds of the round the bay names.
func TestClipStateCarriesTheBaySpec(t *testing.T) {
	s := newTestSim(t)
	cs, ok := s.Clip(s.player, ammoSlot())
	if !ok {
		t.Fatal("fixture has no EMP turret bay")
	}
	if cs.BayM3 != 0.05 {
		t.Errorf("BayM3 = %v, want 0.05 (the autocannon's fixture bay)", cs.BayM3)
	}
	if cs.CycleSeconds != 2 {
		t.Errorf("CycleSeconds = %v, want 2 (the gun's firing cycle)", cs.CycleSeconds)
	}
	// Empty with no order: no round to count, so no capacity either.
	if cs.Capacity != 0 || cs.Rounds != 0 {
		t.Errorf("capacity %d / loaded %d, want 0/0 (no round named)", cs.Capacity, cs.Rounds)
	}
	// With the order set the capacity and the live count agree by construction.
	loadBay(t, s, "ammo-emp-s")
	cs, _ = s.Clip(s.player, ammoSlot())
	if cs.Capacity != 5 || cs.Rounds != cs.Capacity {
		t.Errorf("capacity %d / loaded %d, want the full 5-round bay", cs.Capacity, cs.Rounds)
	}
}

// TestNoPickNoLoad is the owner's regression sequence at the bay level: three
// starting ammo stacks, no CmdSetAmmo, ticks of idle time and a locked target —
// nothing auto-loads, and the third stack stays in cargo.
func TestNoPickNoLoad(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	forceLock(s, npc)
	setCargo(s,
		content.CargoStack{Item: "ammo-emp-s", Qty: 80},
		content.CargoStack{Item: "ammo-fusion-s", Qty: 40},
		content.CargoStack{Item: "ammo-ap-s", Qty: 20},
	)
	stepN(s, 600)
	if got := bayRounds(s, ammoSlot()); got != 0 {
		t.Fatalf("bay = %d after idle ticks, want 0", got)
	}
	for item, want := range map[content.ItemID]int{
		"ammo-emp-s": 80, "ammo-fusion-s": 40, "ammo-ap-s": 20,
	} {
		if got := cargoCount(s, item); got != want {
			t.Fatalf("%s cargo = %d, want %d (nothing leaves cargo without a pick)", item, got, want)
		}
	}
	if got := s.ammoPref(s.player, ammoSlot()); got != "" {
		t.Fatalf("selection = %q, want none", got)
	}
}

// TestNpcBayScope pins where the two rules part. The bays share fillClip, but
// the one-type guess and the stock-out re-aim belong to NPCs alone: a rat loads
// its single granted type and switches to the one type left when its own is
// gone, while the player's bay with the same cargo waits and keeps its order.
func TestNpcBayScope(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	_, prt, ok := s.moduleRuntime(s.player, ammoSlot())
	if !ok {
		t.Fatal("fixture has no EMP turret slot")
	}
	// Same cargo, same empty bays: only fusion is aboard, the player's order
	// names EMP (out of stock).
	setCargo(s, content.CargoStack{Item: "ammo-fusion-s", Qty: 5})
	prt.Clip = content.CargoStack{}
	prt.Selection = "ammo-emp-s"
	fit, _ := s.Fittings.Get(npc)
	if len(fit.HighRT) == 0 {
		t.Fatal("rat has no high module")
	}
	ratCargo, _ := s.Cargos.Get(npc)
	ratCargo.Stacks = []content.CargoStack{{Item: "ammo-fusion-s", Qty: 5}}
	fit.HighRT[0].Clip = content.CargoStack{}
	fit.HighRT[0].Selection = "" // never picked: the spawn grant is its order

	s.Step()
	if got := prt.Clip; got.Qty != 0 {
		t.Fatalf("player bay = %+v, want empty (the order's type is gone)", got)
	}
	if got := prt.Selection; got != "ammo-emp-s" {
		t.Fatalf("player order = %q, want ammo-emp-s (kept)", got)
	}
	if got := fit.HighRT[0].Clip; got.Qty == 0 || got.Item != "ammo-fusion-s" {
		t.Fatalf("rat bay = %+v, want the fusion type loaded (no rack to ask)", got)
	}
	// The rat's own type out of stock re-aims it at the one type left.
	stepN(s, 130) // its 2 s load cycle, so the bay settles first
	ratCargo.Stacks = []content.CargoStack{{Item: "ammo-fusion-s", Qty: 5}}
	fit.HighRT[0].Clip = content.CargoStack{}
	fit.HighRT[0].Selection = "ammo-emp-s" // its old type, out of stock
	s.Step()
	if got := fit.HighRT[0].Clip; got.Qty == 0 || got.Item != "ammo-fusion-s" {
		t.Fatalf("rat bay after a stock-out = %+v, want the one remaining type", got)
	}
	if got := prt.Clip.Qty; got != 0 {
		t.Fatalf("player bay = %d after the rat re-aimed, want 0", got)
	}
}

// TestCmdSetAmmoLoadsCapCells: the cap booster is a charge bay too, and its
// cell is picked the same explicit way (the rack offers the same load); with
// no pick the booster refuses to activate instead of loading a cell itself.
func TestCmdSetAmmoLoadsCapCells(t *testing.T) {
	s := newTestSim(t)
	slot := injectModule(t, s, "mod-cap-booster")
	s.Queue(CmdActivate{Slot: slot})
	s.Step()
	evs := drain(s)
	if findEvent(evs, EvLog, "Cap Booster waiting for ammo selection") == nil {
		t.Fatalf("missing waiting warning: %v", evs)
	}
	if got := bayRounds(s, slot); got != 0 {
		t.Fatalf("booster bay = %d before a pick, want 0", got)
	}

	evs = func() []Event {
		s.Queue(CmdSetAmmo{Slot: slot, Item: capCellID})
		s.Step()
		return drain(s)
	}()
	if findEvent(evs, EvLog, "Cannot load") != nil {
		t.Fatalf("the booster refused its own cell: %v", evs)
	}
	// The fixture holds 5 cells; the pick moves them all into the 25-round bay.
	if got := bayRounds(s, slot); got != 5 {
		t.Fatalf("booster bay = %d after the pick, want 5 (every cell loaded)", got)
	}
	if got := cargoCount(s, capCellID); got != 0 {
		t.Fatalf("cells in cargo = %d, want 0 (all loaded)", got)
	}
}
