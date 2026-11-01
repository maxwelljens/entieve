package render

import (
	"errors"
	"fmt"
	"image"
	"math"
	"os"
	"strings"
	"testing"

	"image/color"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/internal/capture"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestLabelsFollowTheHover pins the owner's rule at the source: a name and
// distance label belongs to the one entity under the cursor, at every zoom
// (zoom is not part of the decision), and to nothing else.
func TestLabelsFollowTheHover(t *testing.T) {
	r := New(nil)
	ship := gridObj{info: sim.Info{Entity: 7, Kind: sim.KindShip}}
	rock := gridObj{info: sim.Info{Entity: 8, Kind: sim.KindAsteroid}}
	if r.labelsOn(ship) {
		t.Error("no hover: a label drew anyway")
	}
	r.SetHover(ship.info.Entity)
	if !r.labelsOn(ship) {
		t.Error("the hovered entity drew no label")
	}
	if r.labelsOn(rock) {
		t.Error("an entity the cursor is not on drew a label")
	}
	r.SetHover(0)
	if r.labelsOn(ship) {
		t.Error("the label survived the hover leaving the entity")
	}
}

// TestStackLayerFollowsPaintOrder pins the stack picker's depth key to the
// overlay's bucket order: a ship paints over a wreck, a wreck over a rock, a
// rock over a station, a station over a celestial.
func TestStackLayerFollowsPaintOrder(t *testing.T) {
	order := []string{sim.KindStar, sim.KindPlanet, sim.KindAnomaly,
		sim.KindStation, sim.KindAsteroid, sim.KindWreck, sim.KindShip}
	for i := 1; i < len(order); i++ {
		if StackLayer(order[i]) <= StackLayer(order[i-1]) {
			t.Errorf("StackLayer(%s) = %d does not paint over StackLayer(%s) = %d",
				order[i], StackLayer(order[i]), order[i-1], StackLayer(order[i-1]))
		}
	}
	// Kinds sharing a bucket tie: the picker breaks those canonically.
	for _, pair := range [][2]string{
		{sim.KindPlanet, sim.KindMoon}, {sim.KindStation, sim.KindGate}, {sim.KindShip, sim.KindPlayer},
	} {
		if StackLayer(pair[0]) != StackLayer(pair[1]) {
			t.Errorf("StackLayer(%s)=%d and StackLayer(%s)=%d share a paint bucket, want one layer",
				pair[0], StackLayer(pair[0]), pair[1], StackLayer(pair[1]))
		}
	}
}

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
	s.Queue(sim.CmdActivate{Slot: sim.SlotRef{Class: content.SlotHigh, Index: idx}, Target: npc, Confirm: true})
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
	s.Queue(sim.CmdActivate{Slot: sim.SlotRef{Class: content.SlotHigh, Index: idx}, Target: second, Confirm: true})
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

// TestCellDrawsTheWorldsBodyArt pins the stack picker's cells to the world's
// own drawing: a cell submits the entity through drawBody, the same pass Draw
// runs over its buckets, for every kind the picker can list — and the box
// EntityArtSize reserves holds the whole art, no pixel cut at the border. An
// icon cell would submit no body primitive at all.
func TestCellDrawsTheWorldsBodyArt(t *testing.T) {
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
	r := New(c)
	// The default framing: 1280×720 at zoom 1 (zoomBase px/m).
	const zoom, capDim = 0.02, 720.0
	kinds := map[string]bool{}
	for _, inf := range s.Grid() {
		if !inf.Visible || kinds[inf.Kind] {
			continue
		}
		kinds[inf.Kind] = true
		size := r.EntityArtSize(s, inf, zoom, capDim)
		if size < 2 {
			t.Fatalf("cell for %s (%v) is %d px", inf.Kind, inf.Name, size)
		}
		dst := ebiten.NewImage(size, size)
		r.bodyPrims = 0
		r.DrawEntityBody(dst, s, inf, Camera{
			Center: inf.Pos, Zoom: zoom,
			ScreenW: float64(size), ScreenH: float64(size), CapDim: capDim,
		})
		if r.bodyPrims == 0 {
			t.Errorf("cell for %s (%v) drew no body art: the cell must be the entity as the world draws it",
				inf.Kind, inf.Name)
		}
	}
	if len(kinds) < 3 {
		t.Fatalf("fixture covered %d kinds, want the showcase's spread", len(kinds))
	}

	// A synthetic entry (an entity the sim does not know) still gets art: the
	// picker's own tests feed hand-built infos.
	inf := sim.Info{Entity: ecs.Entity(9999), Kind: sim.KindShip, Visible: true}
	size := r.EntityArtSize(s, inf, zoom, capDim)
	r.bodyPrims = 0
	r.DrawEntityBody(ebiten.NewImage(size, size), s, inf, Camera{
		Center: inf.Pos, Zoom: zoom,
		ScreenW: float64(size), ScreenH: float64(size), CapDim: capDim,
	})
	if r.bodyPrims == 0 {
		t.Error("a cell drew nothing for an entity outside the sim")
	}
}

// TestEntityArtSizeFollowsZoom pins the cell to the world's size: the art box
// grows with the zoom (a station is 102 px at the default framing, 202 px one
// notch in), and a star's box stops at the world's own screen cap rather than
// at whatever cell it fills.
func TestEntityArtSizeFollowsZoom(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	wd, _ := c.World("world-showcase")
	s, err := sim.New(c, wd, 42)
	if err != nil {
		t.Fatalf("sim.New: %v", err)
	}
	r := New(c)
	var station, star sim.Info
	for _, inf := range s.Grid() {
		switch inf.Kind {
		case sim.KindStation:
			station = inf
		case sim.KindStar:
			star = inf
		}
	}
	if station.Entity.IsZero() || star.Entity.IsZero() {
		t.Fatal("fixture: the showcase needs a station and a star")
	}
	// The cell takes the art's own pixels: one zoom notch in doubles the
	// station's box (its body is uncapped at this framing).
	small := r.EntityArtSize(s, station, 0.02, 720)
	big := r.EntityArtSize(s, station, 0.04, 720)
	if ratio := float64(big) / float64(small); math.Abs(ratio-2) > 0.1 {
		t.Errorf("station art %d px at zoom 0.02, %d px at 0.04 (ratio %.2f), want the art to double with the zoom",
			small, big, ratio)
	}
	// The star's glow caps at 45% of the world's long screen edge (324 px),
	// not at whatever cell it fills: the box is the grid's at every zoom.
	if got, want := r.EntityArtSize(s, star, 0.02, 720), 650; got != want {
		t.Errorf("star art = %d px, want the world cap %d", got, want)
	}
	if got, want := r.EntityArtSize(s, star, 0.04, 720), 650; got != want {
		t.Errorf("star art = %d px one zoom notch in, want the same world cap %d", got, want)
	}
	if got := r.EntityArtSize(s, star, 0.02, 320); got == 650 {
		t.Errorf("star art ignored the reference viewport: %d px at a 320 px cap", got)
	}
}

// TestHiddenEntitiesLeaveTheGrid pins the stack picker's disappearance to the
// body pass: while the column is up the entities it displays submit no body
// primitive, every entity it does not list keeps drawing, the player is hidden
// like anything else, and a dismissed column (SetHidden(nil)) restores them.
// What the world would still paint around a hidden entity (bracket, hover
// label, selection ring, the lock ring that stays) is pinned by the stack
// picker capture, where pixel readback is available.
func TestHiddenEntitiesLeaveTheGrid(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	wd, _ := c.World("world-showcase")
	s, err := sim.New(c, wd, 42)
	if err != nil {
		t.Fatalf("sim.New: %v", err)
	}
	var station ecs.Entity
	for _, inf := range s.Grid() {
		if inf.Kind == sim.KindStation && inf.Visible {
			station = inf.Entity
			break
		}
	}
	if station.IsZero() {
		t.Fatal("fixture: no visible station")
	}
	tr, _ := s.Transforms.Get(station)
	cam := Camera{Center: tr.Pos, Zoom: 0.02, ScreenW: 1280, ScreenH: 720}
	r := New(c)
	count := func(hide []ecs.Entity, cam Camera) int {
		r.SetHidden(hide)
		r.Draw(ebiten.NewImage(1280, 720), cam, s)
		return r.bodyPrims
	}
	full := count(nil, cam)
	if full == 0 {
		t.Fatal("fixture: nothing draws")
	}
	if got := count([]ecs.Entity{station}, cam); got != full-1 {
		t.Errorf("hidden station: %d bodies drawn, want %d (the station draws in the column, nowhere else)",
			got, full-1)
	}
	if got := count([]ecs.Entity{s.Player()}, cam); got != full {
		t.Errorf("hiding an entity outside the column dropped a body: %d, want %d", got, full)
	}
	if got := count(nil, cam); got != full {
		t.Errorf("a dismissed column left %d of %d bodies hidden", full-got, full)
	}

	// The player is an entity like any other: listed, it leaves the grid.
	ptr, _ := s.Transforms.Get(s.Player())
	pcam := Camera{Center: ptr.Pos, Zoom: 0.02, ScreenW: 1280, ScreenH: 720}
	withPlayer := count(nil, pcam)
	if got := count([]ecs.Entity{s.Player()}, pcam); got != withPlayer-1 {
		t.Errorf("listed player: %d bodies drawn, want %d", got, withPlayer-1)
	}
}

// TestHiddenEntitiesKeepTheirLockRing pins what the stack column's
// disappearance leaves alone: a hidden target still draws the player's lock
// ring, because the ring marks your lock, not the target's presence. Pixel
// readback needs a running ebiten loop, so this is the gated probe (no PNG);
// the reads happen inside the loop, where ebiten's vector paths are resolved.
func TestHiddenEntitiesKeepTheirLockRing(t *testing.T) {
	var got []string
	ran := false
	(&capture.Capture{
		Name:   "Entieve hidden lock ring probe",
		Frames: 1,
		Draw: func(_ int, _ *ebiten.Image) error {
			ran = true
			c, err := content.Load(content.FS())
			if err != nil {
				return err
			}
			wd, ok := c.World("world-showcase")
			if !ok {
				return errors.New("world-showcase missing")
			}
			s, err := sim.New(c, wd, 42)
			if err != nil {
				return err
			}
			var station ecs.Entity
			for _, inf := range s.Grid() {
				if inf.Kind == sim.KindStation && inf.Visible {
					station = inf.Entity
					break
				}
			}
			if station.IsZero() {
				return errors.New("fixture: no visible station")
			}
			tg, ok := s.Targetings.Get(s.Player())
			if !ok {
				return errors.New("fixture: the player cannot lock")
			}
			tr, _ := s.Transforms.Get(station)
			cam := Camera{Center: tr.Pos, Zoom: 0.02, ScreenW: 1280, ScreenH: 720}
			r := New(c)
			r.SetSelected(station)
			r.SetHover(station)
			r.SetHidden([]ecs.Entity{station})
			shoot := func() *ebiten.Image {
				img := ebiten.NewImage(1280, 720)
				r.Draw(img, cam, s)
				return img
			}
			box := image.Rect(600, 320, 680, 400)
			tg.Locks = []sim.Lock{{Target: station, Progress: 1}}
			s.Targetings.Set(s.Player(), *tg)
			withLock := inkNear(shoot(), box, colLock)
			tg.Locks = nil
			s.Targetings.Set(s.Player(), *tg)
			without := inkNear(shoot(), box, colLock)
			if withLock == 0 {
				got = append(got, "a hidden, locked target lost its lock ring: the ring marks the lock, not the presence")
			}
			if without != 0 {
				got = append(got, fmt.Sprintf("%d lock-coloured pixels drew with no lock", without))
			}
			return nil
		},
	}).Run(t)
	if !ran {
		t.Skip("the capture loop did not run")
	}
	for _, g := range got {
		t.Error(g)
	}
}

// inkNear counts the pixels of r within 20 of col on every colour channel.
func inkNear(img *ebiten.Image, r image.Rectangle, col color.RGBA) int {
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			cr, cg, cb, _ := img.At(x, y).RGBA()
			if absDiff8(cr>>8, uint32(col.R)) <= 20 && absDiff8(cg>>8, uint32(col.G)) <= 20 &&
				absDiff8(cb>>8, uint32(col.B)) <= 20 {
				n++
			}
		}
	}
	return n
}

// absDiff8 is |a-b| on 8-bit channels.
func absDiff8(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}
