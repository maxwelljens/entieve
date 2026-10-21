package sim

import (
	"math"
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// ---------------------------------------------------------------------------
// Turret math
// ---------------------------------------------------------------------------

func TestHitChanceGoldenValues(t *testing.T) {
	const (
		tracking = 0.4
		sig      = 40.0
		optimal  = 6000.0
		falloff  = 3000.0
	)
	// angular that exactly consumes the tracking term: tracking*sig/40000.
	trackingAng := tracking * sig / 40000

	cases := []struct {
		name    string
		angular float64
		dist    float64
		falloff float64
		want    float64
	}{
		{"in optimal, no transversal", 0, 3000, falloff, 1.0},
		{"at optimal edge", 0, optimal, falloff, 1.0},
		{"optimal+falloff", 0, optimal + falloff, falloff, 0.5},
		{"optimal+2falloff", 0, optimal + 2*falloff, falloff, 0.0625}, // 0.5^(2^2)
		{"tracking term exactly 1", trackingAng, 3000, falloff, 0.5},
		{"both terms 1", trackingAng, optimal + falloff, falloff, 0.25},
		{"no falloff beyond optimal", 0, optimal + 1, 0, 0.0},
	}
	for _, c := range cases {
		got := hitChance(c.angular, tracking, sig, c.dist, optimal, c.falloff)
		if math.Abs(got-c.want) > 1e-12 {
			t.Errorf("%s: chance=%v want %v", c.name, got, c.want)
		}
	}
	// Bigger signature = easier to hit.
	easy := hitChance(trackingAng, tracking, 400, 3000, optimal, falloff)
	if easy <= 0.5 {
		t.Fatalf("large sig should beat tracking term: %v", easy)
	}
}

func TestLosKinematicsTransversal(t *testing.T) {
	// Target flying perpendicular at 100 m/s, 1000 m away.
	dist, angular := losKinematics(v(), v(1000, 0), v(), v(0, 100))
	if dist != 1000 {
		t.Fatalf("dist=%v", dist)
	}
	if math.Abs(angular-0.1) > 1e-12 {
		t.Fatalf("angular=%v want 0.1 (100/1000)", angular)
	}
	// Pure radial motion contributes no transversal.
	_, ang2 := losKinematics(v(), v(1000, 0), v(), v(100, 0))
	if ang2 != 0 {
		t.Fatalf("radial angular=%v want 0", ang2)
	}
}

// dummyTarget creates a static, non-AI ship entity for deterministic shots.
func dummyTarget(t *testing.T, s *Sim, pos core.Vec2, res content.Resists) ecs.Entity {
	t.Helper()
	e := s.create()
	s.Names.Set(e, Name{Value: "Practice Drone"})
	s.Transforms.Set(e, Transform{Pos: pos})
	s.Motions.Set(e, Motion{
		Mass: 1e5, MaxSpeed: 0, Agility: 1, SigRadius: 40,
		SpeedMultiplier: 1, WebFactor: 1,
	})
	s.Ships.Set(e, ShipState{
		Hull: "ship-rat", Shield: 1000, Armor: 1000, Structure: 1000,
		MaxShield: 1000, MaxArmor: 1000, MaxStructure: 1000, Resists: res,
	})
	return e
}

// findWreckingSeed returns an RNG state whose first two draws produce a
// wrecking shot at chance = 1.
func findWreckingSeed() (uint64, bool) {
	for st := uint64(1); st < 100000; st++ {
		r := core.RestoreRNG(st)
		if _, _, wrecking := rollShot(r, 1.0); wrecking {
			return st, true
		}
	}
	return 0, false
}

func TestWreckingShotForcedViaSeed(t *testing.T) {
	s := newTestSim(t)
	dummy := dummyTarget(t, s, v(1e7+2000, 500), content.Resists{})
	lockInstantly(t, s, dummy)
	giveCargo(s, "ammo-emp-s", 10)
	slot := SlotRef{Class: content.SlotHigh, Index: 0} // world-fit turret

	st, ok := findWreckingSeed()
	if !ok {
		t.Fatal("no wrecking seed found")
	}
	ship, _ := s.Ships.Get(dummy)
	ship.Shield = 1000
	s.RNG = core.RestoreRNG(st)

	s.Queue(CmdActivate{Slot: slot, Target: dummy})
	s.Step()
	evs := drain(s)

	if findEvent(evs, EvLog, "Wrecking hit!") == nil {
		t.Fatalf("expected wrecking hit event: %v", evs)
	}
	// Volley = ammo-emp-s {EM10, Thermal2} x3 = 36 total at zero resists.
	want := 1000.0 - (10*3 + 2*3)
	if math.Abs(ship.Shield-want) > 1e-9 {
		t.Fatalf("shield=%v want %v (wrecking x3)", ship.Shield, want)
	}
}

func TestAmmoConsumedPerShot(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	forceLock(s, npc)
	if got := cargoCount(s, "ammo-emp-s"); got != 20 {
		t.Fatalf("start ammo=%d", got)
	}
	slot := SlotRef{Class: content.SlotHigh, Index: 0}
	s.Queue(CmdActivate{Slot: slot, Target: npc})
	s.Step() // first shot resolves on activation
	if got := cargoCount(s, "ammo-emp-s"); got != 19 {
		t.Fatalf("ammo after shot=%d want 19", got)
	}
	stepN(s, 130) // ShotTime = 2s = 120 ticks -> one more shot
	if got := cargoCount(s, "ammo-emp-s"); got != 18 {
		t.Fatalf("ammo after 2nd cycle=%d want 18", got)
	}
}

func TestDryGunWarnsAndDeactivates(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	forceLock(s, npc)
	// Empty the cargo of light charges.
	cargo, _ := s.Cargos.Get(s.player)
	cargo.Stacks = nil

	slot := SlotRef{Class: content.SlotHigh, Index: 0}
	s.Queue(CmdActivate{Slot: slot, Target: npc})
	s.Step()
	evs := drain(s)
	if findEvent(evs, EvLog, "no charges") == nil {
		t.Fatalf("expected dry-gun warning: %v", evs)
	}
	if findEvent(evs, EvModule, "EMP Turret deactivated") == nil {
		t.Fatalf("expected deactivation event: %v", evs)
	}
	rt, ok := hudModule(s, slot)
	if !ok || rt.Active {
		t.Fatal("dry gun should be inactive")
	}
}

func TestRocketPodNeedsNoAmmo(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	forceLock(s, npc)
	cargo, _ := s.Cargos.Get(s.player)
	cargo.Stacks = nil // no ammo at all

	pod := injectModule(t, s, "mod-rocket-pod")
	s.Queue(CmdActivate{Slot: pod, Target: npc})
	s.Step()
	evs := drain(s)
	rt, ok := hudModule(s, pod)
	if !ok || !rt.Active {
		t.Fatalf("rocket pod should fire without ammo; events=%v", evs)
	}
	shot := findEvent(evs, EvLog, "Rat Scuttler")
	if shot == nil {
		t.Fatalf("expected hit/miss event: %v", evs)
	}
}

func TestDamageProfileMatchesAmmo(t *testing.T) {
	s := newTestSim(t)
	// Dummy with EM/thermal-hard, kinetic-soft resists.
	res := content.Resists{EM: 0.9, Thermal: 0.9, Kinetic: 0, Explosive: 0}
	dummy := dummyTarget(t, s, v(1e7+2000, 500), res)
	lockInstantly(t, s, dummy)
	slot := SlotRef{Class: content.SlotHigh, Index: 0}
	base := content.Damage{EM: 10, Thermal: 2}   // turret base volley
	ammoAP := content.Damage{EM: 0, Kinetic: 12} // ammo-ap-s profile

	// Shot 1: EMP charges -> profile {EM10, Thermal2}.
	giveCargo(s, "ammo-emp-s", 5)
	state1 := s.RNG.Snapshot()
	s.Queue(CmdActivate{Slot: slot, Target: dummy})
	s.Step()
	evs := drain(s)
	hit := findEvent(evs, EvLog, "Practice Drone")
	if hit == nil {
		t.Fatalf("no hit event: %v", evs)
	}
	rng := core.RestoreRNG(state1)
	_, mult, _ := rollShot(rng, hitChanceFor(t, s, dummy))
	want := base.Scale(mult).ApplyTo(res).Total()
	if math.Abs(hit.Value-want) > 1e-9 {
		t.Fatalf("EMP shot value=%v want %v (ammo profile not applied?)", hit.Value, want)
	}

	// Shot 2: AP charges -> profile replaces the base volley.
	cargo, _ := s.Cargos.Get(s.player)
	cargo.Stacks = nil
	giveCargo(s, "ammo-ap-s", 5)
	stepN(s, 130) // the 2 s cycle fires once more (AP) en route
	s.Queue(CmdDeactivate{Slot: slot})
	s.Step()
	drain(s)
	stepN(s, 110) // let the cancelled cycle clock run out while the gun is off
	state2 := s.RNG.Snapshot()
	s.Queue(CmdActivate{Slot: slot, Target: dummy})
	s.Step()
	evs = drain(s)
	hit = findEvent(evs, EvLog, "Practice Drone")
	if hit == nil {
		t.Fatalf("no second hit event: %v", evs)
	}
	rng = core.RestoreRNG(state2)
	_, mult, _ = rollShot(rng, hitChanceFor(t, s, dummy))
	want = ammoAP.Scale(mult).ApplyTo(res).Total()
	if math.Abs(hit.Value-want) > 1e-9 {
		t.Fatalf("AP shot value=%v want %v (ammo profile not applied?)", hit.Value, want)
	}
	if cargoCount(s, "ammo-ap-s") != 3 {
		t.Fatalf("ap ammo=%d want 3 (one spare cycle fired en route)", cargoCount(s, "ammo-ap-s"))
	}
}

// hitChanceFor recomputes the chance of the last setup (stationary dummy
// within optimal, no transversal): exactly 1.
func hitChanceFor(t *testing.T, s *Sim, target ecs.Entity) float64 {
	t.Helper()
	tr, _ := s.Transforms.Get(s.player)
	ttr, _ := s.Transforms.Get(target)
	dist := tr.Pos.Dist(ttr.Pos)
	return hitChance(0, 0.4, 40, dist, 6000, 3000)
}

func TestMissEmitsLogEvent(t *testing.T) {
	s := newTestSim(t)
	// Target deep in falloff with huge transversal -> near-certain miss.
	dummy := dummyTarget(t, s, v(1e7+6000, 500), content.Resists{})
	m, _ := s.Motions.Get(dummy)
	m.MaxSpeed = 5000
	s.Intents.Set(dummy, Intent{Kind: IntentManual, Dir: v(0, 1), SpeedFrac: 1})
	forceLock(s, dummy)
	// Several shot cycles; at least one should miss at chance << 1.
	slot := SlotRef{Class: content.SlotHigh, Index: 0}
	s.Queue(CmdActivate{Slot: slot, Target: dummy})
	misses := 0
	for i := 0; i < 400; i++ {
		s.Step()
		for _, ev := range drain(s) {
			if ev.Kind == EvLog && strings.Contains(ev.Text, "misses") {
				misses++
			}
		}
	}
	if misses == 0 {
		t.Fatal("expected at least one miss with extreme transversal")
	}
}

// ---------------------------------------------------------------------------
// Damage
// ---------------------------------------------------------------------------

func TestResistMathTable(t *testing.T) {
	cases := []struct {
		name string
		dmg  content.Damage
		res  content.Resists
		want float64
	}{
		{"pure EM halved", content.Damage{EM: 100}, content.Resists{EM: 0.5}, 50},
		{"mixed types", content.Damage{EM: 100, Kinetic: 100},
			content.Resists{EM: 0.5, Kinetic: 0.25}, 125},
		{"no resists", content.Damage{Thermal: 40, Explosive: 60}, content.Resists{}, 100},
		{"near-immune", content.Damage{EM: 1000}, content.Resists{EM: 0.999}, 1},
		{"zero damage", content.Damage{}, content.Resists{EM: 0.5}, 0},
		{"all four", content.Damage{EM: 10, Thermal: 10, Kinetic: 10, Explosive: 10},
			content.Resists{EM: 0.1, Thermal: 0.2, Kinetic: 0.3, Explosive: 0.4}, 30},
	}
	for _, c := range cases {
		ship := &ShipState{Shield: 1e9, Armor: 1e9, Structure: 1e9, Resists: c.res}
		got := applyDamageToShip(ship, c.dmg)
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: pen=%v want %v", c.name, got, c.want)
		}
	}
}

func TestLayerOrderAndOverflow(t *testing.T) {
	// Shield absorbs first; overflow carries to armor at full value.
	ship := &ShipState{Shield: 10, Armor: 500, Structure: 400}
	applyDamageToShip(ship, content.Damage{EM: 100})
	if ship.Shield != 0 || ship.Armor != 410 || ship.Structure != 400 {
		t.Fatalf("after 100 into 10/500/400: S=%v A=%v St=%v", ship.Shield, ship.Armor, ship.Structure)
	}
	// Armor overflows into structure at full value.
	ship = &ShipState{Shield: 0, Armor: 20, Structure: 400}
	applyDamageToShip(ship, content.Damage{EM: 100})
	if ship.Armor != 0 || ship.Structure != 320 {
		t.Fatalf("overflow: A=%v St=%v want 0/320", ship.Armor, ship.Structure)
	}
	// Overkill exactly depletes structure (clamped at the pool boundary);
	// Structure <= 0 is the death signal the damage system resolves on.
	ship = &ShipState{Shield: 10, Armor: 20, Structure: 400}
	applyDamageToShip(ship, content.Damage{EM: 5000})
	if ship.Structure != 0 {
		t.Fatalf("structure=%v want 0 (lethal)", ship.Structure)
	}
	// Resists apply once at impact, then layers share the pooled remainder.
	ship = &ShipState{Shield: 100, Armor: 100, Structure: 100,
		Resists: content.Resists{EM: 0.5}}
	pen := applyDamageToShip(ship, content.Damage{EM: 100})
	if pen != 50 || ship.Shield != 50 || ship.Armor != 100 {
		t.Fatalf("pen=%v S=%v A=%v", pen, ship.Shield, ship.Armor)
	}
}

func TestNpcDeathCreatesWreckWithLootBounds(t *testing.T) {
	s := newTestSim(t)
	for i := 0; i < 5; i++ {
		npc := spawnNearPlayer(t, s)
		damageNPC(s, s.player, npc, content.Damage{EM: 10000}, false)
		s.Step()
		drain(s)
		if s.World.Alive(npc) {
			t.Fatalf("NPC %d survived lethal damage", i)
		}
	}
	// Five wrecks with loot within [MinQty, MaxQty] = [1, 2].
	wrecks := sortedEntities(s.Wrecks)
	if len(wrecks) != 5 {
		t.Fatalf("wrecks=%d want 5", len(wrecks))
	}
	for _, w := range wrecks {
		wr, _ := s.Wrecks.Get(w)
		if len(wr.Loot) != 1 || wr.Loot[0].Item != "loot-alloy-scrap" {
			t.Fatalf("loot=%+v", wr.Loot)
		}
		if wr.Loot[0].Qty < 1 || wr.Loot[0].Qty > 2 {
			t.Fatalf("loot qty %d out of bounds [1,2]", wr.Loot[0].Qty)
		}
		// Wrecks expire 300s after creation, which happened within the last
		// few ticks of this test (time advanced by DT per Step).
		if wr.ExpiresAt < wreckExpiry || wr.ExpiresAt > wreckExpiry+1 {
			t.Fatalf("expires=%v want ~%v", wr.ExpiresAt, wreckExpiry)
		}
	}
}

func TestBountyPaidOnPlayerKillOnly(t *testing.T) {
	s := newTestSim(t)
	walletBefore := s.Wallet()

	npc := spawnNearPlayer(t, s)
	damageNPC(s, s.player, npc, content.Damage{EM: 10000}, false)
	s.Step()
	evs := drain(s)
	if s.Wallet() != walletBefore+15000 {
		t.Fatalf("wallet=%v want %v", s.Wallet(), walletBefore+15000)
	}
	if findEvent(evs, EvLog, "Bounty paid: 15,000.00 €") == nil {
		t.Fatalf("missing bounty event: %v", evs)
	}

	// A kill attributed to something else pays nothing.
	other := spawnNearPlayer(t, s)
	attacker := s.create()
	s.Names.Set(attacker, Name{Value: "Concord Drone"})
	damageNPC(s, attacker, other, content.Damage{EM: 10000}, false)
	s.Step()
	drain(s)
	if s.Wallet() != walletBefore+15000 {
		t.Fatalf("bounty paid for non-player kill: %v", s.Wallet())
	}
}

func TestPlayerDeathRespawnsInSloop(t *testing.T) {
	s := newTestSim(t)
	giveCargo(s, "ammo-emp-s", 30)
	walletBefore := s.Wallet()
	// Park the player far from any station so the respawn move is obvious.
	setPlayerPos(s, v(2e7, 2e7))

	damageNPC(s, spawnNearPlayer(t, s), s.player, content.Damage{EM: 100000}, false)
	s.Step()
	evs := drain(s)

	if findEvent(evs, EvDestroyed, "was destroyed") == nil {
		t.Fatalf("missing EvDestroyed: %v", evs)
	}
	if !hasEvent(evs, EvLog, "Respawned at Haven Rest in a Sloop") {
		t.Fatalf("missing respawn log: %v", evs)
	}
	ship, _ := s.Ships.Get(s.player)
	if ship.Hull != "ship-sloop" {
		t.Fatalf("hull=%s", ship.Hull)
	}
	if ship.Shield != ship.MaxShield || ship.Armor != ship.MaxArmor ||
		ship.Structure != ship.MaxStructure {
		t.Fatal("respawn ship should be at full layers")
	}
	if cargoCount(s, "ammo-emp-s") != 0 {
		t.Fatal("cargo should be lost on death (EVE spirit)")
	}
	if s.Wallet() != walletBefore {
		t.Fatal("wallet must survive death")
	}
	// At the system's first station + 5000 m along +X.
	pos := playerPos(s)
	if math.Abs(pos.X-(1e7+5000)) > 1 || math.Abs(pos.Y) > 1 {
		t.Fatalf("respawn pos=%v want (10005000, 0)", pos)
	}
	fit, _ := s.Fittings.Get(s.player)
	if len(fit.High) != 1 || fit.High[0] != "mod-autocannon-s" {
		t.Fatalf("respawn fit=%v", fit.High)
	}
}

// ---------------------------------------------------------------------------
// Capacitor
// ---------------------------------------------------------------------------

func TestCapRechargePeaksAt25Percent(t *testing.T) {
	s := newTestSim(t)
	c, _ := s.Caps.Get(s.player)
	bestFrac, bestDelta := 0.0, -1.0
	for frac := 0.05; frac < 0.96; frac += 0.05 {
		c.Current = c.Capacity * frac
		before := c.Current
		s.systemCap()
		delta := c.Current - before
		if delta > bestDelta {
			bestDelta, bestFrac = delta, frac
		}
	}
	if math.Abs(bestFrac-0.25) > 0.05 {
		t.Fatalf("peak recharge at %.2f want ~0.25", bestFrac)
	}
	// Curve sanity: full capacitor does not recharge.
	c.Current = c.Capacity
	s.systemCap()
	if c.Current < c.Capacity {
		t.Fatal("full cap should not gain/lose")
	}
	// Numeric peak value: 2.5*Cap/Tau*0.25*DT for Cap=400, Tau=200.
	c.Current = 0.25 * c.Capacity
	before := c.Current
	s.systemCap()
	want := 2.5 * 400 / 200 * 0.25 * DT
	if math.Abs((c.Current-before)-want) > 1e-12 {
		t.Fatalf("peak delta=%v want %v", c.Current-before, want)
	}
}

func TestActivationCostDrainsCapacitor(t *testing.T) {
	s := newTestSim(t)
	slot := injectModule(t, s, "mod-shield-boost")
	c, _ := s.Caps.Get(s.player)
	before := c.Current
	s.Queue(CmdActivate{Slot: slot, Target: 0})
	s.Step()
	drain(s)
	// One tick of recharge (at most 1.25*DT) happens after the deduction.
	if math.Abs(c.Current-(before-20)) > 1.25*DT+1e-12 {
		t.Fatalf("cap=%v want ~%v (activation cost 20)", c.Current, before-20)
	}
	// Second cycle 5s later drains again (allowing for capacitor recharge,
	// which adds at most ~6.3 GJ over the 5s window).
	stepN(s, 301)
	drainAmt := before - c.Current
	if drainAmt < 39 || drainAmt > 47 {
		t.Fatalf("net drain after 2 cycles=%v want ~40", drainAmt)
	}
}

func TestCapBoosterConsumesCellAndRestores(t *testing.T) {
	s := newTestSim(t)
	slot := injectModule(t, s, "mod-cap-booster")
	c, _ := s.Caps.Get(s.player)
	c.Current = 50
	if got := cargoCount(s, "charge-cap-cell"); got != 5 {
		t.Fatalf("cells=%d", got)
	}
	s.Queue(CmdActivate{Slot: slot, Target: 0})
	s.Step()
	drain(s)
	if math.Abs(c.Current-150) > 1.25*DT+1e-12 {
		t.Fatalf("cap=%v want ~150 (50 + CapRestore 100)", c.Current)
	}
	if got := cargoCount(s, "charge-cap-cell"); got != 4 {
		t.Fatalf("cells after boost=%d want 4", got)
	}
}

func TestCapBoosterFailsWithoutCell(t *testing.T) {
	s := newTestSim(t)
	slot := injectModule(t, s, "mod-cap-booster")
	c, _ := s.Caps.Get(s.player)
	c.Current = 50
	cargo, _ := s.Cargos.Get(s.player)
	cargo.Stacks = nil

	s.Queue(CmdActivate{Slot: slot, Target: 0})
	s.Step()
	evs := drain(s)
	if findEvent(evs, EvLog, "No cap booster charges") == nil {
		t.Fatalf("expected warning: %v", evs)
	}
	rt, _ := hudModule(s, slot)
	if rt.Active {
		t.Fatal("should not activate without cells")
	}
	if math.Abs(c.Current-50) > 1.25*DT+1e-12 {
		t.Fatalf("cap changed beyond one tick of recharge: %v", c.Current)
	}
}

// ---------------------------------------------------------------------------
// Tank
// ---------------------------------------------------------------------------

func TestShieldBoosterHealsPerCycle(t *testing.T) {
	s := newTestSim(t)
	slot := injectModule(t, s, "mod-shield-boost")
	ship, _ := s.Ships.Get(s.player)
	ship.Shield = 400
	s.Queue(CmdActivate{Slot: slot, Target: 0})
	s.Step()
	drain(s)
	if ship.Shield != 450 {
		t.Fatalf("shield=%v want 450 (first cycle)", ship.Shield)
	}
	stepN(s, 301) // next 5s cycle
	if ship.Shield != 500 {
		t.Fatalf("shield=%v want 500 (second cycle)", ship.Shield)
	}
}

func TestArmorRepHealsPerCycle(t *testing.T) {
	s := newTestSim(t)
	slot := injectModule(t, s, "mod-armor-rep")
	ship, _ := s.Ships.Get(s.player)
	ship.Armor = 400
	c, _ := s.Caps.Get(s.player)
	before := c.Current
	s.Queue(CmdActivate{Slot: slot, Target: 0})
	s.Step()
	drain(s)
	if ship.Armor != 440 {
		t.Fatalf("armor=%v want 440", ship.Armor)
	}
	if math.Abs(c.Current-(before-15)) > 1.25*DT+1e-12 {
		t.Fatalf("cap=%v want ~%v", c.Current, before-15)
	}
}

func TestPassiveModulesRaiseLayerMaxes(t *testing.T) {
	s := newTestSim(t)
	ship, _ := s.Ships.Get(s.player)
	if ship.MaxShield != 500 || ship.MaxArmor != 500 {
		t.Fatalf("base maxes: %v/%v", ship.MaxShield, ship.MaxArmor)
	}
	ext := injectModule(t, s, "mod-shield-extender")
	plate := injectModule(t, s, "mod-armor-plate")
	if ship.MaxShield != 700 {
		t.Fatalf("MaxShield=%v want 700", ship.MaxShield)
	}
	if ship.MaxArmor != 750 {
		t.Fatalf("MaxArmor=%v want 750", ship.MaxArmor)
	}
	// Un-fitting via the direct path restores the old maxes (clamped HP).
	ship.Shield = 690
	// Remove the higher index first so the earlier slot ref stays valid.
	removeInjected(t, s, plate)
	removeInjected(t, s, ext)
	if ship.MaxShield != 500 || ship.MaxArmor != 500 {
		t.Fatalf("after unfit: %v/%v", ship.MaxShield, ship.MaxArmor)
	}
	if ship.Shield != 500 {
		t.Fatalf("shield clamped=%v want 500", ship.Shield)
	}
}

// removeInjected removes a directly-injected module and recomputes fit.
func removeInjected(t *testing.T, s *Sim, slot SlotRef) {
	t.Helper()
	fit, _ := s.Fittings.Get(s.player)
	switch slot.Class {
	case content.SlotHigh:
		fit.High = append(fit.High[:slot.Index], fit.High[slot.Index+1:]...)
		fit.HighRT = append(fit.HighRT[:slot.Index], fit.HighRT[slot.Index+1:]...)
	case content.SlotMid:
		fit.Mid = append(fit.Mid[:slot.Index], fit.Mid[slot.Index+1:]...)
		fit.MidRT = append(fit.MidRT[:slot.Index], fit.MidRT[slot.Index+1:]...)
	case content.SlotLow:
		fit.Low = append(fit.Low[:slot.Index], fit.Low[slot.Index+1:]...)
		fit.LowRT = append(fit.LowRT[:slot.Index], fit.LowRT[slot.Index+1:]...)
	}
	// Shift module-target bindings after the removed index.
	s.shiftModTargets(s.player, slot.Class, slot.Index)
	s.recomputeFit(s.player)
}

func TestResistCoatRecomputeFit(t *testing.T) {
	s := newTestSim(t)
	ship, _ := s.Ships.Get(s.player)
	if ship.Resists.EM != 0.1 {
		t.Fatalf("base resist EM=%v", ship.Resists.EM)
	}
	coat := injectModule(t, s, "mod-resist-coat")
	// 0.1 + 0.2 carries float representation error; compare with epsilon.
	if math.Abs(ship.Resists.EM-0.3) > 1e-12 || math.Abs(ship.Resists.Explosive-0.3) > 1e-12 {
		t.Fatalf("coated resists=%+v", ship.Resists)
	}
	removeInjected(t, s, coat)
	if ship.Resists.EM != 0.1 {
		t.Fatalf("after unfit EM=%v", ship.Resists.EM)
	}
	// Resists clamp strictly below 1 even with an absurd bonus (fresh
	// content per test, so mutating the def here is safe).
	s2 := newTestSim(t)
	s2.Content.Modules["mod-resist-coat"].Passive.ResistBonus = content.Resists{EM: 0.99}
	ship2, _ := s2.Ships.Get(s2.player)
	injectModule(t, s2, "mod-resist-coat")
	if ship2.Resists.EM >= 1 {
		t.Fatalf("resist must stay < 1: %v", ship2.Resists.EM)
	}
}

func TestCapRechargerRaisesRechargeStat(t *testing.T) {
	s := newTestSim(t)
	c, _ := s.Caps.Get(s.player)
	if c.Tau != 200 {
		t.Fatalf("base tau=%v", c.Tau)
	}
	injectModule(t, s, "mod-cap-recharger")
	if c.Tau != 150 {
		t.Fatalf("tau with recharger=%v want 150", c.Tau)
	}
}
