package sim

import (
	"math"
	"testing"
)

// ---------------------------------------------------------------------------
// Shield recharge (EVE curve, step 9)
// ---------------------------------------------------------------------------

// TestShieldRechargePeaksAt25Percent pins the wiki curve against the capacitor
// twin: dC/dt = 10*Cmax/T*(sqrt(c)-c) peaks at 25% at 2.5x the average Cmax/T,
// and is zero at a full shield.
func TestShieldRechargePeaksAt25Percent(t *testing.T) {
	s := newTestSim(t)
	sh, _ := s.Ships.Get(s.player)
	const maxShield, tau = 400.0, 100.0
	sh.MaxShield, sh.ShieldTau = maxShield, tau

	rate := func(frac float64) float64 {
		sh.Shield = frac * maxShield
		before := sh.Shield
		s.systemShield()
		return sh.Shield - before
	}

	// Full shield: nothing to recharge, clamp keeps it put.
	sh.Shield = maxShield
	s.systemShield()
	if sh.Shield != maxShield {
		t.Fatalf("full shield moved to %v", sh.Shield)
	}

	// Peak: at c = 0.25 the bracket is 0.5-0.25 = 0.25, so
	// dC = 2.5*Cmax/T*DT = 2.5x the average rate Cmax/T.
	peak := rate(0.25)
	if want := 2.5 * maxShield / tau * DT; math.Abs(peak-want) > 1e-12 {
		t.Fatalf("peak delta=%v want %v", peak, want)
	}

	// Empty is (numerically) flat — only the nudge off the fixed point.
	if d := rate(0); d > peak*1e-3 {
		t.Fatalf("delta at 0%% = %v, want ~0", d)
	}

	// Monotonic rise up to 25%, monotonic fall after it.
	prev := rate(1e-6)
	for frac := 0.05; frac <= 0.25+1e-9; frac += 0.05 {
		d := rate(frac)
		if d < prev {
			t.Fatalf("rate not rising toward 25%%: at %.2f got %v after %v", frac, d, prev)
		}
		prev = d
	}
	for frac := 0.30; frac < 1.0; frac += 0.05 {
		d := rate(frac)
		if d > prev {
			t.Fatalf("rate not falling past 25%%: at %.2f got %v after %v", frac, d, prev)
		}
		prev = d
	}
}

// TestShieldRechargeTimeIsTo98Percent pins the T semantics: T is the idle time
// from 0% to "roughly 98%" (the analytic curve hits (1-e^-5)^2 = 98.66% at t=T).
func TestShieldRechargeTimeIsTo98Percent(t *testing.T) {
	s := newTestSim(t)
	sh, _ := s.Ships.Get(s.player)
	const maxShield, tau = 400.0, 60.0
	sh.MaxShield, sh.ShieldTau = maxShield, tau

	// Exactly empty: the curve is flat there, and the system nudges it off
	// the fixed point, so a stripped shield rebuilds instead of staying dead.
	sh.Shield = 0
	s.systemShield()
	if sh.Shield <= 0 {
		t.Fatal("empty shield did not start recharging")
	}

	for i := 0; i < int(math.Round(tau/DT)); i++ {
		s.systemShield()
	}
	if frac := sh.Shield / maxShield; frac < 0.98 || frac > 0.99 {
		t.Fatalf("after T=%vs shields at %.4f, want ~98.7%%", tau, frac)
	}
}

// TestShieldExtenderKeepsTauRaisesRegen pins EVE's extender consequence:
// capacity rises, recharge TIME is unchanged, so HP/s rises.
func TestShieldExtenderKeepsTauRaisesRegen(t *testing.T) {
	s := newTestSim(t)
	s.Content.Ships["ship-sloop"].ShieldRecharge = 100
	s.recomputeFit(s.player)
	sh, _ := s.Ships.Get(s.player)
	if sh.ShieldTau != 100 {
		t.Fatalf("ShieldTau=%v want 100 (hull recharge time)", sh.ShieldTau)
	}

	rate := func() float64 {
		sh.Shield = 0.25 * sh.MaxShield
		before := sh.Shield
		s.systemShield()
		return sh.Shield - before
	}
	base, baseMax := rate(), sh.MaxShield
	injectModule(t, s, "mod-shield-extender")
	if sh.MaxShield != baseMax+200 || sh.ShieldTau != 100 {
		t.Fatalf("extender: max=%v tau=%v want %v/100", sh.MaxShield, sh.ShieldTau, baseMax+200)
	}
	if ext := rate(); math.Abs(ext/base-1.4) > 1e-9 {
		t.Fatalf("regen ratio with +200 shield = %v want 1.4 at the same tau", ext/base)
	}
}

// TestNpcShieldRechargesToo covers every ShipState holder, not just the player,
// and pins that armor and structure do NOT regenerate.
func TestNpcShieldRechargesToo(t *testing.T) {
	s := newTestSim(t)
	rat := spawnNearPlayer(t, s)
	sh, _ := s.Ships.Get(rat)
	sh.ShieldTau = 50
	sh.Shield = 50
	sh.Armor, sh.Structure = 40, 30
	stepN(s, 120)
	if sh.Shield <= 50 {
		t.Fatalf("npc shield %v did not recharge over 120 ticks", sh.Shield)
	}
	if sh.Armor != 40 || sh.Structure != 30 {
		t.Fatalf("armor/structure regenerated: %v/%v", sh.Armor, sh.Structure)
	}
}

// TestShieldAndBoosterStack pins that regen and shield boosters simply add,
// both clamped to MaxShield.
func TestShieldAndBoosterStack(t *testing.T) {
	s := newTestSim(t)
	ship, _ := s.Ships.Get(s.player)
	ship.ShieldTau = 100
	ship.Shield = ship.MaxShield - 30
	slot := injectModule(t, s, "mod-shield-boost")
	s.Queue(CmdActivate{Slot: slot, Target: 0})
	s.Step()
	drain(s)
	if got := ship.Shield; got != ship.MaxShield {
		t.Fatalf("shield=%v want clamped to max %v", got, ship.MaxShield)
	}
}

// TestShieldRechargeSurvivesSaveRoundTrip pins that the recharge time is
// carried across snapshot/restore, and re-derived for a pre-field save.
func TestShieldRechargeSurvivesSaveRoundTrip(t *testing.T) {
	c := testContent()
	c.Ships["ship-sloop"].ShieldRecharge = 120
	s, err := New(c, testWorld(), testSeed)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ship, _ := s.Ships.Get(s.player)
	ship.Shield = 111

	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	restored, err := New(c, testWorld(), testSeed)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := restored.Restore(save); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if sh, _ := restored.Ships.Get(restored.player); sh.Shield != 111 || sh.ShieldTau != 120 {
		t.Fatalf("restored shield=%v tau=%v want 111/120", sh.Shield, sh.ShieldTau)
	}

	// A pre-field save carries no ShieldTau: re-derive it from the hull.
	for i := range save.Entities {
		if save.Entities[i].Ship != nil {
			save.Entities[i].Ship.ShieldTau = 0
		}
	}
	old, err := New(c, testWorld(), testSeed)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := old.Restore(save); err != nil {
		t.Fatalf("restore pre-field: %v", err)
	}
	if sh, _ := old.Ships.Get(old.player); sh.ShieldTau != 120 {
		t.Fatalf("pre-field save restored tau=%v want 120", sh.ShieldTau)
	}
}
