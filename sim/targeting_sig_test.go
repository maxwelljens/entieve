package sim

import (
	"math"
	"testing"
)

// TestMicrowarpdriveBloomsTheSignature pins the active microwarpdrive's
// signature bloom: six times the hull's radius while the module runs, back to
// the base the moment it is switched off (recomputeFit owns both).
func TestMicrowarpdriveBloomsTheSignature(t *testing.T) {
	s := newTestSim(t)
	const base = 40.0 // ship-vastrek
	slot := injectModule(t, s, "mod-microwarpdrive")
	if !s.activate(s.player, slot, 0, true) {
		t.Fatal("microwarpdrive did not activate")
	}
	m, ok := s.Motions.Get(s.player)
	if !ok {
		t.Fatal("player has no motion")
	}
	if m.SigMultiplier != 6 {
		t.Fatalf("SigMultiplier = %v, want 6", m.SigMultiplier)
	}
	if got := m.EffectiveSig(s.time); got != base*6 {
		t.Fatalf("EffectiveSig = %v, want %v", got, base*6)
	}
	// The bloom feeds lock progress: the same scanner locks the bloomed hull
	// faster than the bare one.
	bloomed := lockProgressPerTick(500, m.EffectiveSig(s.time))
	if bloomed <= lockProgressPerTick(500, base) {
		t.Fatalf("bloom did not speed the lock: %v vs %v", bloomed, lockProgressPerTick(500, base))
	}
	if want := lockProgressPerTick(500, 240); math.Abs(bloomed-want) > 1e-15 {
		t.Fatalf("bloomed rate = %v, want %v", bloomed, want)
	}

	s.deactivate(s.player, slot, "", SevInfo)
	m, _ = s.Motions.Get(s.player)
	if m.SigMultiplier != 1 {
		t.Fatalf("SigMultiplier after deactivation = %v, want 1", m.SigMultiplier)
	}
	if got := m.EffectiveSig(s.time); got != base {
		t.Fatalf("EffectiveSig after deactivation = %v, want %v", got, base)
	}
}

// TestTargetPainterRaisesTheTargetSignature pins the painter: +25% on the
// locked target's signature, one EvCombat paint on the first cycle, nothing
// on a refresh inside the window, and the base value back after expiry.
func TestTargetPainterRaisesTheTargetSignature(t *testing.T) {
	s := newTestSim(t)
	tgt := spawnNearPlayer(t, s) // ship-rat: signature 50 m
	lockInstantly(t, s, tgt)
	slot := injectModule(t, s, "mod-target-painter")
	if !s.activate(s.player, slot, tgt, true) {
		t.Fatal("target painter did not activate")
	}

	s.Step() // the first cycle resolves this tick
	evs := drain(s)
	if n := countPhase(evs, EvCombat, CombatPaint); n != 1 {
		t.Fatalf("paint events = %d, want 1: %v", n, evs)
	}
	m, _ := s.Motions.Get(tgt)
	if got, want := m.EffectiveSig(s.time), 50*1.25; got != want {
		t.Fatalf("painted signature = %v, want %v", got, want)
	}

	// A second cycle inside the effect window refreshes the value silently.
	stepN(s, 330)
	if n := countPhase(drain(s), EvCombat, CombatPaint); n != 0 {
		t.Fatalf("refresh emitted %d paint events, want 0", n)
	}
	if got, want := m.EffectiveSig(s.time), 50*1.25; got != want {
		t.Fatalf("refreshed signature = %v, want %v", got, want)
	}

	// The effect is expiry-driven: switching the module off lets it lapse.
	s.deactivate(s.player, slot, "", SevInfo)
	stepN(s, 700)
	if got := m.EffectiveSig(s.time); got != 50 {
		t.Fatalf("signature after expiry = %v, want 50", got)
	}
}

// TestSensorDampCutsTheTargetScanResolution pins the dampener: -45% on the
// locked target's scan resolution, one EvCombat damp on the first cycle, and
// the base value back after expiry.
func TestSensorDampCutsTheTargetScanResolution(t *testing.T) {
	s := newTestSim(t)
	tgt := spawnNearPlayer(t, s) // ship-rat: scan resolution 400
	lockInstantly(t, s, tgt)
	slot := injectModule(t, s, "mod-sensor-damp")
	if !s.activate(s.player, slot, tgt, true) {
		t.Fatal("sensor dampener did not activate")
	}

	s.Step()
	evs := drain(s)
	if n := countPhase(evs, EvCombat, CombatDamp); n != 1 {
		t.Fatalf("damp events = %d, want 1: %v", n, evs)
	}
	tg, _ := s.Targetings.Get(tgt)
	if got, want := tg.EffectiveScanRes(s.time), 400*0.55; math.Abs(got-want) > 1e-9 {
		t.Fatalf("damped scan resolution = %v, want %v", got, want)
	}

	s.deactivate(s.player, slot, "", SevInfo)
	stepN(s, 400)
	if got := tg.EffectiveScanRes(s.time); got != 400 {
		t.Fatalf("scan resolution after expiry = %v, want 400", got)
	}
}

// TestShieldExtenderAddsSignatureRadius pins the extender's flat seven metre
// signature cost: fitted raises the hull's radius, un-fitted drops it back.
func TestShieldExtenderAddsSignatureRadius(t *testing.T) {
	s := newTestSim(t)
	injectModule(t, s, "mod-shield-extender")
	m, _ := s.Motions.Get(s.player)
	if got, want := m.SigRadius, 40.0+7; got != want {
		t.Fatalf("fitted signature = %v, want %v", got, want)
	}

	fit, _ := s.Fittings.Get(s.player)
	fit.Low = fit.Low[:len(fit.Low)-1]
	fit.LowRT = fit.LowRT[:len(fit.LowRT)-1]
	s.recomputeFit(s.player)
	if got := m.SigRadius; got != 40 {
		t.Fatalf("un-fitted signature = %v, want 40", got)
	}
}

// TestLockTimeFormulaSeconds pins t_lock in seconds against the EVE curve,
// t = 40000 / (scanRes * asinh(sig)^2), including the wiki's worked example
// (600 mm/s locks a 400 m signature in about 1.5 s).
func TestLockTimeFormulaSeconds(t *testing.T) {
	cases := []struct {
		scanRes, sig, want float64
	}{
		{600, 400, 1.5}, // EVE's worked example
		{500, 50, 3.77},
	}
	for _, c := range cases {
		got := 1 / (60 * lockProgressPerTick(c.scanRes, c.sig))
		if math.Abs(got-c.want) > 0.02 {
			t.Errorf("t_lock(%v, %v) = %v s, want about %v s", c.scanRes, c.sig, got, c.want)
		}
	}
}
