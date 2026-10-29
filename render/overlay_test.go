package render

import (
	"math"
	"os"
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestShotAgeFollowsCycleClock pins the shot-timing seam: the renderer reads
// the shot moment off the runtime's cycle clock, so a gun whose CycleEnds is
// one period ahead of the shot reports age 0 at the shot and one period just
// before the next one.
func TestShotAgeFollowsCycleClock(t *testing.T) {
	const period, shot = 3.0, 100.0
	cycleEnds := shot + period // what fireWeapon writes at the shot
	cases := []struct {
		now  float64
		want float64
		ok   bool
	}{
		{shot, 0, true},
		{shot + 0.01, 0.01, true},
		{shot + period - 1e-9, period, true}, // last frame before the next shot
		{shot - 0.01, 0, false},              // before the shot
		{shot + period, 0, false},            // cycle run out, not yet re-fired
	}
	for _, c := range cases {
		age, ok := shotAge(true, cycleEnds, period, c.now)
		if ok != c.ok || (ok && math.Abs(age-c.want) > 1e-6) {
			t.Errorf("shotAge(now=%v) = %v,%v want %v,%v", c.now, age, ok, c.want, c.ok)
		}
	}
	if _, ok := shotAge(false, cycleEnds, period, shot); ok {
		t.Error("an inactive module must report no shot")
	}
	if _, ok := shotAge(true, 0, period, shot); ok {
		t.Error("a never-fired module (CycleEnds 0) must report no shot")
	}
}

// TestWeaponFireIsPerShot pins the end state: an active locked gun is drawn
// as discrete shots — flash, short bolts, impact — and is silent for the rest
// of its cycle. No stroke may reach the target: that full-length line was the
// laser look.
func TestWeaponFireIsPerShot(t *testing.T) {
	nose := core.V(100, 100)
	tgt := core.V(500, 100) // 400 px away
	const sizePx = 20.0

	for _, tc := range []struct {
		name   string
		heavy  bool
		period float64
	}{
		{"autocannon", false, 3},
		{"artillery", true, 9},
	} {
		fired, quiet := false, true
		for age := 0.0; age < tc.period; age += 0.01 {
			r := New(nil)
			dst := ebiten.NewImage(600, 200)
			r.weaponFire(dst, nose, tgt, true, sizePx, age, tc.heavy)

			if len(r.shotStrokes) > 0 {
				fired = true
			}
			for _, l := range r.shotStrokes {
				if l > boltTail+0.5 {
					t.Fatalf("%s age %.2f: stroke %.1f px spans toward the target (max bolt %.1f)",
						tc.name, age, l, float32(boltTail))
				}
			}
			// Mid-cycle: every round has arrived and faded, so nothing at all.
			if age > 2 && age < tc.period-1 {
				quiet = r.shotPrims == 0 && quiet
			}
		}
		if !fired {
			t.Errorf("%s: no bolt stroke drawn anywhere in the cycle", tc.name)
		}
		if !quiet {
			t.Errorf("%s: effects persist mid-cycle (fire must be an event)", tc.name)
		}
	}

	// The families stay split: a burst of thin bolts vs one heavy round.
	if autoShot.bolts < 2 || artyShot.bolts != 1 || artyShot.flight <= autoShot.flight {
		t.Errorf("family timelines lost their split: auto %+v arty %+v", autoShot, artyShot)
	}
}

// TestWeaponFireWithoutLock pins that a firing gun with no locked target
// still flashes at the muzzle and never strokes a line into empty space.
func TestWeaponFireWithoutLock(t *testing.T) {
	r := New(nil)
	r.weaponFire(ebiten.NewImage(200, 200), core.V(100, 100), core.Vec2{}, false, 20, 0, false)
	if r.shotPrims == 0 {
		t.Error("no muzzle flash without a lock")
	}
	if len(r.shotStrokes) != 0 {
		t.Errorf("stroked %d bolts with no target", len(r.shotStrokes))
	}
}

// TestShotSeamLandsOnRealFire pins the seam against the simulation: park the
// player beside a hostile (through the save snapshot the game itself uses),
// lock it, fire the start autocannon, and read the shot back through the
// cycle clock the renderer uses. The derived age is under one tick on the
// tick the sim fires, so the flash cannot drift from the shot.
func TestShotSeamLandsOnRealFire(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	wd, ok := c.World("world-showcase")
	if !ok {
		t.Fatal("world-showcase missing")
	}
	s, err := sim.New(c, wd, 42)
	if err != nil {
		t.Fatalf("sim.New: %v", err)
	}

	// Find an NPC (a Scuttler) and park the player 5 km off it.
	var npc ecs.Entity
	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	for i := range save.Entities {
		if e := &save.Entities[i]; e.NPC != nil {
			npc = ecs.Entity(e.ID)
			for j := range save.Entities {
				if save.Entities[j].ID == uint64(s.Player()) && e.Transform != nil {
					save.Entities[j].Transform.Pos = e.Transform.Pos.Add(core.V(5000, 0))
				}
			}
			break
		}
	}
	if npc.IsZero() {
		t.Fatal("no NPC in the showcase save")
	}
	if err := s.Restore(save); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	s.Queue(sim.CmdLock{Target: npc})
	locked := false
	for i := 0; i < 30*sim.TickRate && !locked; i++ {
		s.Step()
		if tg, ok := s.Targetings.Get(s.Player()); ok {
			for _, l := range tg.Locks {
				locked = locked || (l.Target == npc && l.Progress >= 1)
			}
		}
	}
	if !locked {
		t.Fatal("lock never completed")
	}

	fit, _ := s.Fittings.Get(s.Player())
	idx, def := -1, (*content.ModuleDef)(nil)
	for i, id := range fit.High {
		if d, ok := c.Module(id); ok && d.Kind == content.KindWeapon {
			idx, def = i, d
			break
		}
	}
	if idx < 0 {
		t.Fatal("start fit has no high-slot weapon")
	}
	// The showcase fit carries four shell types, so its bay starts in the
	// waiting state: select one before firing (flag: bay tranche).
	s.Queue(sim.CmdSetAmmo{Slot: sim.SlotRef{Class: content.SlotHigh, Index: idx}, Item: "ammo-emp-s"})
	s.Queue(sim.CmdActivate{Slot: sim.SlotRef{Class: content.SlotHigh, Index: idx}, Target: npc})
	for i := 0; i < 10*sim.TickRate; i++ {
		s.Step()
		fit, _ := s.Fittings.Get(s.Player())
		rt := fit.HighRT[idx]
		if age, fired := shotAge(rt.Active, rt.CycleEnds, shotCycle(def), s.Time()); fired {
			if age > sim.DT+1e-9 {
				t.Fatalf("first visible shot has age %v s, want one tick at most", age)
			}
			// And the draw path shows that shot: a flash/bolt, no beam.
			pp, _ := s.Transforms.Get(s.Player())
			r := New(c)
			cam := Camera{Center: pp.Pos, Zoom: 0.02, ScreenW: 640, ScreenH: 360}
			r.Draw(ebiten.NewImage(640, 360), cam, s)
			if r.shotPrims == 0 || len(r.shotStrokes) == 0 {
				t.Fatalf("firing frame drew %d primitives, %d strokes", r.shotPrims, len(r.shotStrokes))
			}
			for _, l := range r.shotStrokes {
				if l > boltTail+0.5 {
					t.Fatalf("firing frame stroked a %.1f px line to the target", l)
				}
			}
			return
		}
	}
	t.Fatal("the gun never fired")
}

// TestWeaponFireStrokesOnlyBolts is the guard the geometry test cannot be: a
// headless run cannot read pixels back, so a stray full-length stroke would
// slip past the recorder. Pin the source: every stroke weaponFire issues goes
// through shotLine (which records its length), never vector.StrokeLine.
func TestWeaponFireStrokesOnlyBolts(t *testing.T) {
	src, err := os.ReadFile("overlay.go")
	if err != nil {
		t.Skipf("source not available: %v", err)
	}
	body := string(src)
	i := strings.Index(body, "func (r *Renderer) weaponFire")
	if i < 0 {
		t.Fatal("weaponFire not found in overlay.go")
	}
	body = body[i:]
	if j := strings.Index(body, "// shotLine strokes"); j > 0 {
		body = body[:j]
	}
	if strings.Contains(body, "vector.StrokeLine") {
		t.Error("weaponFire strokes a line directly — weapon fire must go through shotLine (bolts only)")
	}
}

// TestFireFXAimsAtTheGunsOwnTarget: with two targets locked, a gun
// activated on the SECOND lock must draw its shot at that ship — not at
// the first lock (the pre-fix targetPoint aimed every gun at the first
// fully locked target, so a multi-lock ship showed its fire going to the
// wrong hull while the sim damaged the right one).
func TestFireFXAimsAtTheGunsOwnTarget(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	wd, ok := c.World("world-showcase")
	if !ok {
		t.Fatal("world-showcase missing")
	}
	s, err := sim.New(c, wd, 42)
	if err != nil {
		t.Fatalf("sim.New: %v", err)
	}
	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	// Two NPC hulls to lock, in save order (the lock order follows it).
	var first, second ecs.Entity
	var firstPos core.Vec2
	for i := range save.Entities {
		e := &save.Entities[i]
		if e.NPC == nil || e.Transform == nil {
			continue
		}
		if first.IsZero() {
			first, firstPos = ecs.Entity(e.ID), e.Transform.Pos
		} else if second.IsZero() {
			second = ecs.Entity(e.ID)
		}
	}
	if first.IsZero() || second.IsZero() {
		t.Skip("showcase save carries fewer than two NPCs")
	}
	for i := range save.Entities {
		if save.Entities[i].ID == uint64(s.Player()) && save.Entities[i].Transform != nil {
			save.Entities[i].Transform.Pos = firstPos.Add(core.V(-20000, 0))
		}
	}
	if err := s.Restore(save); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	lock := func(target ecs.Entity) {
		s.Queue(sim.CmdLock{Target: target})
		for i := 0; i < 60*sim.TickRate; i++ {
			s.Step()
			if tg, ok := s.Targetings.Get(s.Player()); ok && tg.Locked(target) {
				return
			}
		}
		t.Fatalf("lock on %v never completed", target)
	}
	lock(first)
	lock(second)

	fit, _ := s.Fittings.Get(s.Player())
	idx := -1
	for i, id := range fit.High {
		if d, ok := c.Module(id); ok && d.Kind == content.KindWeapon {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("start fit has no high-slot weapon")
	}
	s.Queue(sim.CmdSetAmmo{Slot: sim.SlotRef{Class: content.SlotHigh, Index: idx}, Item: "ammo-emp-s"})
	s.Queue(sim.CmdActivate{Slot: sim.SlotRef{Class: content.SlotHigh, Index: idx}, Target: second})
	for i := 0; i < 10*sim.TickRate; i++ {
		s.Step()
	}

	pp, _ := s.Transforms.Get(s.Player())
	cam := Camera{Center: pp.Pos, Zoom: 0.02, ScreenW: 1280, ScreenH: 720}
	aim, ok := targetPoint(cam, s, s.Player(), content.SlotHigh, idx)
	if !ok {
		t.Fatal("a firing gun has no aim point")
	}
	tf, _ := s.Transforms.Get(first)
	ts, _ := s.Transforms.Get(second)
	fx, fy := cam.WorldToScreen(tf.Pos)
	sx, sy := cam.WorldToScreen(ts.Pos)
	if aim != core.V(sx, sy) {
		t.Fatalf("shot aims at (%.1f,%.1f): the first lock sits at (%.1f,%.1f), "+
			"the gun's own target at (%.1f,%.1f)", aim.X, aim.Y, fx, fy, sx, sy)
	}
}
