package sim

import (
	"math"
	"testing"

	"github.com/entieve/entieve/ecs"
)

func TestLockCompletesInExpectedTime(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s) // ~1500 m away, within lock range
	s.Queue(CmdLock{Target: npc})

	stepN(s, 180)
	tg, _ := s.Targetings.Get(s.player)
	if len(tg.Locks) != 1 {
		t.Fatalf("locks=%d want 1", len(tg.Locks))
	}
	if tg.Locks[0].Progress >= 1 {
		t.Fatal("locked suspiciously fast (180 ticks)")
	}
	stepN(s, 140)
	evs := drain(s)
	if !hasEvent(evs, EvTargetLocked, "Locked Rat Scuttler") {
		t.Fatalf("no lock event: %v", evs)
	}
	tg, _ = s.Targetings.Get(s.player)
	if len(tg.Locks) != 1 || tg.Locks[0].Progress < 1 {
		t.Fatalf("lock not complete: %+v", tg.Locks)
	}
	if !tg.Locked(npc) {
		t.Fatal("Locked() false after completion")
	}
	// EVE's curve: t = 40000/(500 * asinh(50)^2) = 3.77 s, about 226
	// ticks. The lock must still be partial at 180 ticks (checked above)
	// and must complete by 320.
}

func TestMaxTargetsEnforced(t *testing.T) {
	s := newTestSim(t)
	var targets []ecs.Entity
	for i := 0; i < 3; i++ {
		e := s.create()
		s.Names.Set(e, Name{Value: "Dummy"})
		s.Transforms.Set(e, Transform{Pos: v(1e7+1000+float64(i)*100, 500)})
		targets = append(targets, e)
	}
	s.Queue(CmdLock{Target: targets[0]})
	s.Queue(CmdLock{Target: targets[1]})
	s.Queue(CmdLock{Target: targets[2]})
	s.Step()
	evs := drain(s)
	tg, _ := s.Targetings.Get(s.player)
	if len(tg.Locks) != 2 {
		t.Fatalf("locks=%d want 2 (MaxTargets)", len(tg.Locks))
	}
	if findEvent(evs, EvLog, "Maximum number of targets") == nil {
		t.Fatalf("expected max-targets warning: %v", evs)
	}
}

func TestOutOfRangeLockDrops(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	s.Queue(CmdLock{Target: npc})
	stepN(s, 100) // partial progress
	tg, _ := s.Targetings.Get(s.player)
	if len(tg.Locks) != 1 || tg.Locks[0].Progress >= 1 {
		t.Fatalf("expected partial lock: %+v", tg.Locks)
	}
	// Fly far beyond MaxRange (12000 m).
	setPlayerPos(s, v(1e7+30000, 500))
	s.Step()
	tg, _ = s.Targetings.Get(s.player)
	if len(tg.Locks) != 0 {
		t.Fatalf("out-of-range lock not dropped: %+v", tg.Locks)
	}
}

func TestUnlockRemovesLock(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	s.Queue(CmdLock{Target: npc})
	stepN(s, 50)
	s.Queue(CmdUnlock{Target: npc})
	s.Step()
	tg, _ := s.Targetings.Get(s.player)
	if len(tg.Locks) != 0 {
		t.Fatalf("unlock failed: %+v", tg.Locks)
	}
}

func TestLockRangeCheckedAtStart(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	setPlayerPos(s, v(1e7+50000, 500)) // 50 km away, MaxRange 12 km
	s.Queue(CmdLock{Target: npc})
	s.Step()
	evs := drain(s)
	tg, _ := s.Targetings.Get(s.player)
	if len(tg.Locks) != 0 {
		t.Fatal("locked beyond MaxRange")
	}
	if findEvent(evs, EvLog, "out of lock range") == nil {
		t.Fatalf("expected range warning: %v", evs)
	}
}

func TestLockProgressFormulaValues(t *testing.T) {
	// EVE's curve: t_lock = 40000/(scanRes * asinh(sig)^2) seconds;
	// per-tick progress = 1/(60*t_lock).
	cases := []struct {
		scanRes, sig float64
	}{
		{500, 40},
		{400, 50},
		{620, 100},
	}
	for _, c := range cases {
		a := math.Asinh(c.sig)
		want := c.scanRes * a * a / (40000 * 60)
		got := lockProgressPerTick(c.scanRes, c.sig)
		if math.Abs(got-want) > 1e-15 {
			t.Fatalf("rate(%v,%v)=%v want %v", c.scanRes, c.sig, got, want)
		}
	}
	if lockProgressPerTick(0, 40) != 0 {
		t.Fatal("zero scan res must not progress")
	}
	// EVE's worked example: 600 mm/s and a 400 m signature lock in about
	// 1.5 s, so the rate is about 1/(60 * 1.5).
	if got, want := lockProgressPerTick(600, 400), 1/(60*1.5); math.Abs(got-want) > 1e-4 {
		t.Fatalf("worked-example rate = %v, want about %v", got, want)
	}
	// The shape EVE documents: a high scan resolution locks a large
	// signature quickly and a small signature slowly.
	if lockProgressPerTick(600, 400) <= lockProgressPerTick(600, 40) {
		t.Fatal("a large signature must lock faster than a small one")
	}
}
