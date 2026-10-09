package sim

import (
	"math"
	"testing"

	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

func TestStepAdvancesTimeAndTicks(t *testing.T) {
	s := newTestSim(t)
	if s.Ticks() != 0 || s.Time() != 0 {
		t.Fatalf("fresh sim: ticks=%d time=%v", s.Ticks(), s.Time())
	}
	s.Step()
	if s.Ticks() != 1 {
		t.Fatalf("after 1 step: ticks=%d", s.Ticks())
	}
	if math.Abs(s.Time()-DT) > 1e-12 {
		t.Fatalf("after 1 step: time=%v want %v", s.Time(), DT)
	}
	stepN(s, 59)
	if s.Ticks() != 60 {
		t.Fatalf("after 60 steps: ticks=%d", s.Ticks())
	}
	if math.Abs(s.Time()-1.0) > 1e-9 {
		t.Fatalf("after 60 steps: time=%v want ~1", s.Time())
	}
}

func TestQueueAppliedNextStep(t *testing.T) {
	s := newTestSim(t)
	if s.Intents.Has(s.player) {
		t.Fatal("fresh player should have no intent")
	}
	s.Queue(CmdManualVel{Dir: v(1, 0), SpeedFrac: 1})
	if s.Intents.Has(s.player) {
		t.Fatal("command must not apply before Step")
	}
	s.Step()
	in, ok := s.Intents.Get(s.player)
	if !ok || in.Kind != IntentManual {
		t.Fatalf("intent after step: ok=%v kind=%q", ok, in.Kind)
	}
}

func TestApproachReachesTarget(t *testing.T) {
	s := newTestSim(t)
	target := s.create()
	s.Names.Set(target, Name{Value: "Beacon"})
	s.Transforms.Set(target, Transform{Pos: v(5000, 0)})
	setPlayerPos(s, v())

	s.Queue(CmdApproach{Target: target})
	reached := false
	for i := 0; i < 3000; i++ {
		s.Step()
		d := s.distBetween(s.player, target)
		if d <= approachStopRange+100 {
			reached = true
			break
		}
	}
	if !reached {
		t.Fatalf("approach never reached target, dist=%v", s.distBetween(s.player, target))
	}
	// It must settle, not fly past at full speed.
	stepN(s, 300)
	d := s.distBetween(s.player, target)
	if d > approachStopRange+250 {
		t.Fatalf("overshot: dist=%v", d)
	}
	m, _ := s.Motions.Get(s.player)
	if m.Vel.Len() > 1 {
		t.Fatalf("should have stopped, speed=%v", m.Vel.Len())
	}
}

func TestOrbitHoldsRange(t *testing.T) {
	s := newTestSim(t)
	target := s.create()
	s.Names.Set(target, Name{Value: "Orbital Marker"})
	s.Transforms.Set(target, Transform{Pos: v(100000, 100000)})
	setPlayerPos(s, v(100000+1500, 100000))

	const want = 1500.0
	s.Queue(CmdOrbit{Target: target, Range: want})
	stepN(s, 120) // settle
	minD, maxD := math.MaxFloat64, 0.0
	for i := 0; i < 900; i++ {
		s.Step()
		d := s.distBetween(s.player, target)
		minD = math.Min(minD, d)
		maxD = math.Max(maxD, d)
	}
	if minD < want-250 || maxD > want+250 {
		t.Fatalf("orbit range drifted: min=%v max=%v want=%v", minD, maxD, want)
	}
}

func TestManualVelocityDirectionAndThrottle(t *testing.T) {
	s := newTestSim(t)
	s.Queue(CmdManualVel{Dir: v(0, 3), SpeedFrac: 0.5})
	stepN(s, 300)
	m, _ := s.Motions.Get(s.player)
	if math.Abs(m.Vel.X) > 1e-6 {
		t.Fatalf("vel.X=%v want 0", m.Vel.X)
	}
	if math.Abs(m.Vel.Y-150) > 1e-3 {
		t.Fatalf("vel.Y=%v want 150", m.Vel.Y)
	}
	// Zero direction falls back to stop.
	s.Queue(CmdManualVel{Dir: v(), SpeedFrac: 1})
	stepN(s, 600)
	m, _ = s.Motions.Get(s.player)
	if m.Vel.Len() > 1e-6 {
		t.Fatalf("stop did not decelerate: %v", m.Vel.Len())
	}
}

func TestAccelerationLimitAlignTime(t *testing.T) {
	s := newTestSim(t)
	// Sloop: MaxSpeed 300, Agility 0.4 -> alignTime 4s -> 75 m/s per second.
	s.Queue(CmdManualVel{Dir: v(1, 0), SpeedFrac: 1})
	s.Step()
	m, _ := s.Motions.Get(s.player)
	if m.Vel.Len() > 300*DT/4+1e-9 {
		t.Fatalf("first tick exceeded maxDeltaV: %v", m.Vel.Len())
	}
	stepN(s, 59) // 60 ticks = 1 s total
	if math.Abs(m.Vel.Len()-75) > 1e-6 {
		t.Fatalf("speed after 1s = %v want ~75 (EVE align ramp)", m.Vel.Len())
	}
	if m.Vel.Len() >= 300 {
		t.Fatal("ship must not reach max speed in 1s")
	}
	stepN(s, 600)
	if m.Vel.Len() < 299.999 {
		t.Fatalf("speed should saturate at max: %v", m.Vel.Len())
	}
}

func TestAfterburnerSpeedMultiplier(t *testing.T) {
	s := newTestSim(t)
	slot := injectModule(t, s, "mod-afterburner")
	if !s.activate(s.player, slot, 0, false) {
		t.Fatal("afterburner failed to activate")
	}
	m, _ := s.Motions.Get(s.player)
	if m.SpeedMultiplier != 1.5 {
		t.Fatalf("SpeedMultiplier=%v want 1.5", m.SpeedMultiplier)
	}
	s.Queue(CmdManualVel{Dir: v(1, 0), SpeedFrac: 1})
	stepN(s, 600) // long enough to accelerate from 300 to 450
	if m.Vel.Len() < 449 {
		t.Fatalf("AB top speed not reached: %v", m.Vel.Len())
	}
	s.Queue(CmdDeactivate{Slot: slot})
	s.Step()
	if m.SpeedMultiplier != 1 {
		t.Fatalf("SpeedMultiplier after deactivate=%v", m.SpeedMultiplier)
	}
	stepN(s, 120)
	if m.Vel.Len() > 300.001 {
		t.Fatalf("speed should clamp back to 300: %v", m.Vel.Len())
	}
}

func TestStasisWebSlowsAndExpires(t *testing.T) {
	s := newTestSim(t)
	// A moving victim ship.
	victim := s.create()
	s.Names.Set(victim, Name{Value: "Victim"})
	s.Transforms.Set(victim, Transform{Pos: v(1e7, 5000)})
	s.Motions.Set(victim, Motion{
		Mass: 1e5, MaxSpeed: 200, Agility: 0.5, SigRadius: 30,
		SpeedMultiplier: 1, WebFactor: 1,
	})
	s.Intents.Set(victim, Intent{Kind: IntentManual, Dir: v(1, 0), SpeedFrac: 1})
	stepN(s, 600)
	vm, _ := s.Motions.Get(victim)
	if math.Abs(vm.Vel.Len()-200) > 0.01 {
		t.Fatalf("victim pre-web speed=%v want 200", vm.Vel.Len())
	}

	web := injectModule(t, s, "mod-stasis-web")
	lockInstantly(t, s, victim)
	s.Queue(CmdActivate{Slot: web, Target: victim})
	s.Step() // activation; the first module cycle applies the web this tick
	s.Step() // movement now clamps the victim to its webbed max speed
	if math.Abs(vm.WebFactor-0.5) > 1e-9 {
		t.Fatalf("WebFactor=%v want 0.5", vm.WebFactor)
	}
	if vm.Vel.Len() > 100.001 {
		t.Fatalf("webbed speed=%v want <=100", vm.Vel.Len())
	}
	// Deactivating stops refreshing the web; it expires ~5s after the last
	// cycle and the victim accelerates back to full speed.
	s.Queue(CmdDeactivate{Slot: web})
	s.Step()
	stepN(s, 1500)
	if vm.Vel.Len() < 199 {
		t.Fatalf("web expired but speed=%v", vm.Vel.Len())
	}
}

func TestWebbedShipCannotExceedWebbedMax(t *testing.T) {
	// Direct-component check of EffectiveMaxSpeed semantics.
	m := Motion{MaxSpeed: 300, SpeedMultiplier: 1, WebFactor: 0.5, WebbedUntil: 10}
	if got := m.EffectiveMaxSpeed(5); math.Abs(got-150) > 1e-9 {
		t.Fatalf("webbed effective=%v want 150", got)
	}
	if got := m.EffectiveMaxSpeed(11); got != 300 {
		t.Fatalf("expired web effective=%v want 300", got)
	}
	m.SpeedMultiplier = 1.5
	if got := m.EffectiveMaxSpeed(5); math.Abs(got-225) > 1e-9 {
		t.Fatalf("web+AB effective=%v want 225", got)
	}
}

// ---------------------------------------------------------------------------
// Warp
// ---------------------------------------------------------------------------

func TestWarpSequenceEvents(t *testing.T) {
	s := newTestSim(t)
	gate := entityByName(s, "Gate to Beta")
	if gate.IsZero() {
		t.Fatal("no gate")
	}
	setPlayerPos(s, v(1e7, 1e7))
	s.Queue(CmdWarpTo{Target: gate})

	var all []Event
	landed := false
	for i := 0; i < 1200 && !landed; i++ {
		s.Step()
		all = append(all, drain(s)...)
		if hasEvent(all, EvWarp, "Warp complete") {
			landed = true
		}
	}
	start := findEvent(all, EvWarp, "Warp drive active")
	land := findEvent(all, EvWarp, "Warp complete")
	if start == nil {
		t.Fatalf("no warp start event; events=%v", all)
	}
	if land == nil {
		t.Fatal("no warp land event")
	}
	if land.Time < start.Time {
		t.Fatalf("land before start: %v < %v", land.Time, start.Time)
	}
	if s.Warps.Has(s.player) {
		t.Fatal("warp drive still engaged after landing")
	}
	if d := s.distBetween(s.player, gate); d > 2500 {
		t.Fatalf("landed %v m away (want <=2500)", d)
	}
}

func TestWarpRequiresAlign75Percent(t *testing.T) {
	s := newTestSim(t)
	gate := entityByName(s, "Gate to Beta")
	setPlayerPos(s, v())
	// Make aligning take forever.
	m, _ := s.Motions.Get(s.player)
	m.Agility = 100 // alignTime = 1000s
	s.Queue(CmdWarpTo{Target: gate})

	var all []Event
	for i := 0; i < 600; i++ {
		s.Step()
		all = append(all, drain(s)...)
	}
	if !s.Warps.Has(s.player) {
		t.Fatal("warp drive missing")
	}
	wd, _ := s.Warps.Get(s.player)
	if wd.Phase != "align" {
		t.Fatalf("phase=%q want align (speed never reached 75%%)", wd.Phase)
	}
	if m.Vel.Len() >= 0.75*m.MaxSpeed {
		t.Fatalf("speed %v should be below 75%% of %v", m.Vel.Len(), m.MaxSpeed)
	}
	if hasEvent(all, EvWarp, "Warp drive active") {
		t.Fatal("warp started below 75% speed")
	}
}

func TestWarpBlockedWhenScrambled(t *testing.T) {
	s := newTestSim(t)
	gate := entityByName(s, "Gate to Beta")
	ship, _ := s.Ships.Get(s.player)
	ship.ScrambledUntil = s.time + 100
	s.Queue(CmdWarpTo{Target: gate})
	s.Step()
	evs := drain(s)
	if s.Warps.Has(s.player) {
		t.Fatal("warp started while scrambled")
	}
	ev := findEvent(evs, EvWarp, "Warp scrambled")
	if ev == nil {
		t.Fatalf("no scram event: %v", evs)
	}
	if ev.Severity != SevWarning {
		t.Fatalf("scram severity=%v want warning", ev.Severity)
	}
	if ev.Phase != "scrammed" {
		t.Fatalf("phase=%q", ev.Phase)
	}
}

func TestWarpCancelledByScram(t *testing.T) {
	s := newTestSim(t)
	gate := entityByName(s, "Gate to Beta")
	setPlayerPos(s, v(1e7, 1e7))
	s.Queue(CmdWarpTo{Target: gate})
	s.Step()
	drain(s)
	wd, ok := s.Warps.Get(s.player)
	if !ok || wd.Phase != "align" {
		t.Fatalf("expected align phase, ok=%v phase=%q", ok, wd.Phase)
	}
	// Mid-align scramble cancels the warp.
	ship, _ := s.Ships.Get(s.player)
	ship.ScrambledUntil = s.time + 100
	s.Step()
	evs := drain(s)
	if s.Warps.Has(s.player) {
		t.Fatal("warp not cancelled by scram")
	}
	if findEvent(evs, EvWarp, "Warp scrambled") == nil {
		t.Fatalf("no scram event: %v", evs)
	}
}

func TestWarpLandsWithin2500m(t *testing.T) {
	s := newTestSim(t)
	station := entityByName(s, "Haven Rest")
	setPlayerPos(s, v(3e7, 3e7)) // far corner
	s.Queue(CmdWarpTo{Target: station})
	found := false
	for i := 0; i < 1800; i++ {
		s.Step()
		if hasEvent(drain(s), EvWarp, "Warp complete") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("warp never landed")
	}
	if d := s.distBetween(s.player, station); d > 2500 {
		t.Fatalf("land distance %v > 2500", d)
	}
}

func TestWarpOnlyToCelestials(t *testing.T) {
	s := newTestSim(t)
	rover := s.create()
	s.Names.Set(rover, Name{Value: "Rover"})
	s.Transforms.Set(rover, Transform{Pos: v(1e7, 600)})
	s.Queue(CmdWarpTo{Target: rover})
	s.Step()
	evs := drain(s)
	if s.Warps.Has(s.player) {
		t.Fatal("warped to a non-celestial")
	}
	if findEvent(evs, EvLog, "Cannot warp there") == nil {
		t.Fatalf("expected warning: %v", evs)
	}
}

func TestAlignToPointsAndAccelerates(t *testing.T) {
	s := newTestSim(t)
	gate := entityByName(s, "Gate to Beta")
	setPlayerPos(s, v())
	s.Queue(CmdAlignTo{Target: gate})
	stepN(s, 300)
	m, _ := s.Motions.Get(s.player)
	if m.Vel.Len() < 50 {
		t.Fatalf("align did not accelerate: %v", m.Vel.Len())
	}
	tr, _ := s.Transforms.Get(s.player)
	want := math.Atan2(gatePos(s, gate).Y-tr.Pos.Y, gatePos(s, gate).X-tr.Pos.X)
	if math.Abs(wrapPi(tr.Facing-want)) > 0.05 {
		t.Fatalf("facing=%v want %v", tr.Facing, want)
	}
	if s.Warps.Has(s.player) {
		t.Fatal("align must not warp")
	}
}

func gatePos(s *Sim, e ecs.Entity) core.Vec2 {
	p, _ := s.posOf(e)
	return p
}

func TestLoadSystemUnknownErrors(t *testing.T) {
	s := newTestSim(t)
	if err := s.LoadSystem("sys-nope", ""); err == nil {
		t.Fatal("expected error for unknown system")
	}
	if err := s.LoadSystem("sys-a", "cel-nope"); err == nil {
		t.Fatal("expected error for unknown entry celestial")
	}
}

func TestNewValidationErrors(t *testing.T) {
	c, w := testContent(), testWorld()
	if _, err := New(nil, w, 1); err == nil {
		t.Fatal("nil content should error")
	}
	if _, err := New(c, nil, 1); err == nil {
		t.Fatal("nil world should error")
	}
	bad := *w
	bad.Start.System = "sys-missing"
	if _, err := New(c, &bad, 1); err == nil {
		t.Fatal("unknown system should error")
	}
	bad2 := *w
	bad2.Start.Ship = "ship-missing"
	if _, err := New(c, &bad2, 1); err == nil {
		t.Fatal("unknown ship should error")
	}
}
