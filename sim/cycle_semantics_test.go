package sim

import (
	"math"
	"testing"

	"github.com/entieve/entieve/content"
)

// Cycle semantics pins (docs/SIM.md §2 "Module cycles and docking").
//
// A module's effect lands when its cycle *starts*: activation resolves the
// first cycle immediately (systemModules pays + applies at CycleEnds == now),
// then every ActivationTime. The cycle clock (ModuleRuntime.CycleEnds) is the
// boundary of the cycle in flight. Cancelling a module keeps that clock so
// re-activating within the cancelled cycle resumes at the same boundary
// instead of restarting it — that is what stops the "toggle to replay the
// effect" exploit. Nothing fires while a module is off.

// cycleFixture fits a shield booster (50 HP / 5 s / 20 GJ) on generous HP and
// capacitor so a cycle count is readable straight off the shield pool.
func cycleFixture(t *testing.T) (*Sim, SlotRef) {
	t.Helper()
	s := newTestSim(t)
	slot := injectModule(t, s, "mod-shield-boost")
	ship, _ := s.Ships.Get(s.player)
	ship.Shield, ship.MaxShield = 0, 1e6
	c, _ := s.Caps.Get(s.player)
	c.Current, c.Capacity = 1e6, 1e6
	return s, slot
}

// shieldCycles is the number of 50 HP shield booster effects applied so far.
func shieldCycles(ship *ShipState) float64 { return ship.Shield / 50 }

// shieldCycleTicks is the 5 s shield booster cycle in ticks.
const shieldCycleTicks = 300

func TestCycleToggleSpamCannotExceedCycleRate(t *testing.T) {
	s, slot := cycleFixture(t)
	ship, _ := s.Ships.Get(s.player)

	// One second of deactivate+activate spam: under the old clock reset every
	// tick replayed the effect; a 5 s cycle allows one.
	for i := 0; i < 60; i++ {
		s.Queue(CmdDeactivate{Slot: slot})
		s.Queue(CmdActivate{Slot: slot})
		s.Step()
		drain(s)
	}
	if got := shieldCycles(ship); got > 1 {
		t.Fatalf("%v shield cycles in 60 ticks of toggle spam; a 5 s cycle allows 1", got)
	}
}

func TestCycleDeactivateMidCycleStopsRepeat(t *testing.T) {
	s, slot := cycleFixture(t)
	ship, _ := s.Ships.Get(s.player)
	s.Queue(CmdActivate{Slot: slot})
	s.Step() // first cycle resolves immediately: 50 HP, next boundary at 5 s
	if got := shieldCycles(ship); got != 1 {
		t.Fatalf("first cycle: %v effects, want 1", got)
	}
	stepN(s, 60) // 1 s into the 5 s cycle
	s.Queue(CmdDeactivate{Slot: slot})
	s.Step()
	rt, _ := hudModule(s, slot)
	if rt.Active {
		t.Fatal("cancelled module must be off")
	}
	if rt.CycleEnds <= s.Time() {
		t.Fatalf("cancelled cycle clock = %v, want the in-flight boundary %v",
			rt.CycleEnds, s.Time()+float64(shieldCycleTicks-61)*DT)
	}
	// The cancelled cycle does not auto-repeat, even past its boundary.
	stepN(s, shieldCycleTicks)
	if got := shieldCycles(ship); got != 1 {
		t.Fatalf("%v effects after the cancelled cycle ran out, want 1", got)
	}
	if rt, _ := hudModule(s, slot); rt.Active {
		t.Fatal("module switched itself back on")
	}
}

func TestCycleReactivateMidCycleKeepsClock(t *testing.T) {
	s, slot := cycleFixture(t)
	ship, _ := s.Ships.Get(s.player)
	s.Queue(CmdActivate{Slot: slot})
	s.Step() // effect at t=0; cycle in flight ends at 5 s
	stepN(s, 60)
	s.Queue(CmdDeactivate{Slot: slot})
	s.Queue(CmdActivate{Slot: slot})
	s.Step() // t~1 s: the pending stop is cancelled, the clock is untouched
	rt, _ := hudModule(s, slot)
	if !rt.Active {
		t.Fatal("reactivation must cancel the pending stop")
	}
	if want := shieldCycleTicks * DT; math.Abs(rt.CycleEnds-want) > 1e-9 {
		t.Fatalf("cycle clock = %v after reactivation, want the untouched boundary %v",
			rt.CycleEnds, want)
	}
	// No clock reset: nothing fires before the original boundary...
	stepN(s, 238)
	if got := shieldCycles(ship); got != 1 {
		t.Fatalf("%v effects before the original boundary, want 1", got)
	}
	s.Step() // ...the retained cycle fires exactly there.
	if got := shieldCycles(ship); got != 2 {
		t.Fatalf("%v effects at the original boundary, want 2", got)
	}
	// And the module keeps repeating from that boundary on: two more cycles.
	stepN(s, 600)
	if got := shieldCycles(ship); got != 4 {
		t.Fatalf("%v effects after two further cycles, want 4", got)
	}
}

func TestDockRestoresShipAndStopsModules(t *testing.T) {
	s := newTestSim(t)
	boost := injectModule(t, s, "mod-shield-boost")
	rep := injectModule(t, s, "mod-armor-rep")
	ab := injectModule(t, s, "mod-afterburner")
	ship, _ := s.Ships.Get(s.player)
	c, _ := s.Caps.Get(s.player)
	for _, slot := range []SlotRef{boost, rep, ab} {
		if !s.activate(s.player, slot, 0, false) {
			t.Fatalf("activate %v failed", slot)
		}
	}
	s.Step() // first cycles resolve; all three stay active with a cycle in flight
	drain(s)
	for _, slot := range []SlotRef{boost, rep, ab} {
		if rt, _ := hudModule(s, slot); !rt.Active {
			t.Fatalf("module %v should be cycling before the dock", slot)
		}
	}
	if m, _ := s.Motions.Get(s.player); m.SpeedMultiplier != 1.5 {
		t.Fatalf("afterburner multiplier=%v, want 1.5", m.SpeedMultiplier)
	}

	// Wreck the ship and drain the cap after the cycles started.
	ship.Shield, ship.Armor, ship.Structure = 100, 200, 300
	c.Current = 10
	wallet, entries := s.Wallet(), len(s.ledger)

	s.Queue(CmdDock{Station: entityByName(s, "Haven Rest")})
	s.Step()
	drain(s)

	if ship.Shield != ship.MaxShield || ship.Armor != ship.MaxArmor || ship.Structure != ship.MaxStructure {
		t.Fatalf("docked layers %v/%v/%v, want full %v/%v/%v",
			ship.Shield, ship.Armor, ship.Structure, ship.MaxShield, ship.MaxArmor, ship.MaxStructure)
	}
	if c.Current != c.Capacity {
		t.Fatalf("docked capacitor %v, want %v", c.Current, c.Capacity)
	}
	for _, slot := range []SlotRef{boost, rep, ab} {
		rt, _ := hudModule(s, slot)
		if rt.Active || rt.CycleEnds != 0 {
			t.Fatalf("module %v survived the dock: active=%v cycleEnds=%v", slot, rt.Active, rt.CycleEnds)
		}
	}
	if m, _ := s.Motions.Get(s.player); m.SpeedMultiplier != 1 {
		t.Fatalf("afterburner multiplier after dock=%v, want 1", m.SpeedMultiplier)
	}
	if s.Wallet() != wallet || len(s.ledger) != entries {
		t.Fatalf("docking cost money: wallet %v -> %v, ledger %v -> %v",
			wallet, s.Wallet(), entries, len(s.ledger))
	}
}

func TestUndockLeavesModulesOff(t *testing.T) {
	s := newTestSim(t)
	boost := injectModule(t, s, "mod-shield-boost")
	dockAtHaven(t, s)
	// Even a module switched on inside the station is off when undocking.
	s.Queue(CmdActivate{Slot: boost})
	s.Step()
	drain(s)
	if rt, _ := hudModule(s, boost); !rt.Active {
		t.Fatal("activation while docked failed")
	}
	s.Queue(CmdUndock{})
	s.Step()
	drain(s)
	rt, _ := hudModule(s, boost)
	if rt.Active || rt.CycleEnds != 0 {
		t.Fatalf("undocked with a running module: active=%v cycleEnds=%v", rt.Active, rt.CycleEnds)
	}
}

func TestNpcActivationIsCycleBounded(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	tg, _ := s.Targetings.Get(npc)
	tg.Locks = append(tg.Locks, Lock{Target: s.player, Progress: 1})
	ai, _ := s.AIs.Get(npc)
	// A stationary shooter hits: motion would put the target in transversal.
	if m, ok := s.Motions.Get(npc); ok {
		m.MaxSpeed = 0
	}
	s.Intents.Set(npc, Intent{Kind: IntentStop})
	// Capacitor is a separate economy: isolate the cycle bound from it.
	if c, ok := s.Caps.Get(npc); ok {
		c.Current, c.Capacity = 1e6, 1e6
	}
	turret := SlotRef{Class: content.SlotHigh, Index: 0}
	ship, _ := s.Ships.Get(s.player)

	// The old exploit on the NPC path: cancel the turret every tick, let the
	// AI re-activate it. A 2 s shot time allows 6 shots in 10 s.
	const ticks = 600
	for i := 0; i < ticks; i++ {
		s.deactivate(npc, turret, "", SevInfo)
		s.npcActivate(npc, ai, s.player)
		s.Step()
		drain(s)
	}
	loss := (ship.MaxShield - ship.Shield) + (ship.MaxArmor - ship.Armor) +
		(ship.MaxStructure - ship.Structure)
	if loss > 6*12 { // 6 shots x at most a full 12-point volley
		t.Fatalf("NPC dealt %v damage in %v ticks; a 2 s cycle allows 6 shots", loss, ticks)
	}
	t.Logf("NPC dealt %v damage over %v ticks", loss, ticks)
	if loss <= 0 {
		t.Fatal("NPC never fired")
	}
}

// TestCycleRateHoldsForEveryTogglePattern sweeps on/off periods across the
// cycle boundary: 50 s of toggling may never yield more than the 10 cycles a
// 5 s module owns (plus the instant first cycle of the first activation).
func TestCycleRateHoldsForEveryTogglePattern(t *testing.T) {
	const window = 3000
	for _, period := range []int{1, 2, 3, 7, 50, 149, 150, 151, 299, 300, 301, 600} {
		s, slot := cycleFixture(t)
		ship, _ := s.Ships.Get(s.player)
		active := false
		for i := 0; i < window; i++ {
			if i%period == 0 {
				if active {
					s.Queue(CmdDeactivate{Slot: slot})
				} else {
					s.Queue(CmdActivate{Slot: slot})
				}
				active = !active
			}
			s.Step()
			drain(s)
		}
		if got, max := shieldCycles(ship), float64(window/shieldCycleTicks+1); got > max {
			t.Fatalf("toggling every %d ticks: %v effects in %v ticks, max %v", period, got, window, max)
		}
	}
}
