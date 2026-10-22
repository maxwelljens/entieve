package sim

import (
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// ---------------------------------------------------------------------------
// The high-sec belt response: security forces clear pirate rats.
//
// OLD behaviour (the reported bug): guards never initiated on anything but
// player-sourced damage, so rats parked on a high-sec belt — Rudnik's
// static guards, Starogród's patrols — sat there forever.
//
// NEW behaviour, two forces:
//
//   - a static guard (npc-guard-*, not in the Patrols store) engages the rats
//     anchored at its own celestial in a HIGH-security system, and the rats
//     shoot back (guardEngageRat / ratDefend, system_npc.go);
//   - a traffic patrol sweeps an occupied high-sec belt once per patrol beat,
//     then flies home (patrolSweepDispatch / patrolSweepStep,
//     system_traffic.go);
//   - the last rat of a high-sec belt dying logs one line (checkBeltCleared,
//     system_damage.go).
//
// Both forces leave rats that are already engaged alone (a pilot's fight is
// the pilot's), and neither exists in low/null space.
// ---------------------------------------------------------------------------

// beltPosOf returns the grid position of a belt celestial.
func beltPosOf(t *testing.T, s *Sim, cel content.CelID) core.Vec2 {
	t.Helper()
	e := s.celEntity(cel)
	if e.IsZero() {
		t.Fatalf("no celestial entity for %s", cel)
	}
	p, ok := s.posOf(e)
	if !ok {
		t.Fatalf("celestial %s has no position", cel)
	}
	return p
}

// newBeltSim builds a sim on the traffic fixture with the player parked far
// off the lanes (so rats stay unmolested) and the traffic stream drained.
func newBeltSim(t *testing.T, c *content.Content, seed int64) *Sim {
	t.Helper()
	s, err := New(c, testWorld(), seed)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	setPlayerPos(s, core.Vec2{X: -5e6, Y: -5e6})
	drain(s)
	return s
}

// spawnStaticGuard places a guard anchored at home that is NOT in the
// Patrols store — the Rudnik static belt guard.
func spawnStaticGuard(t *testing.T, s *Sim, home core.Vec2) ecs.Entity {
	t.Helper()
	e := s.spawnNpc("npc-guard-test", home, 0)
	if e.IsZero() {
		t.Fatal("spawnNpc(npc-guard-test) failed")
	}
	if s.Patrols.Has(e) {
		t.Fatal("fixture: a static guard must not carry Patrol state")
	}
	return e
}

// sweepCount returns how many live rats the belt anchor holds.
func ratsAt(s *Sim, home core.Vec2) int {
	n := 0
	for _, e := range sortedEntities(s.AIs) {
		ai, ok := s.AIs.Get(e)
		if !ok || ai.Home != home {
			continue
		}
		if def, ok := s.Content.Npc(ai.Npc); ok && isRatNpc(def) {
			n++
		}
	}
	return n
}

// TestHighSecGuardEngagesBeltRats pins the ambient security rule: a static
// high-sec guard clears the rats anchored at its celestial with no player
// involved, the rats return fire, and nothing is ever pointed at the player.
func TestHighSecGuardEngagesBeltRats(t *testing.T) {
	c := trafficContent()
	c.Traffic["sys-a"].Patrol = nil // isolate: no scheduled patrols in this pin
	// A rat that can actually hit (the shared fixture's autocannon tracks at
	// 0.4 and never lands on a drifting hull — same reason trafficContent
	// gives its guards mod-test-turret).
	c.NPCs["npc-rat-scuttler"].Fit = []content.ModuleID{"mod-test-turret"}
	s := newBeltSim(t, c, 7)
	belt := beltPosOf(t, s, "cel-belt-a")
	guard := spawnStaticGuard(t, s, belt)
	rat := s.spawnNpc("npc-rat-scuttler", belt, 100)
	if rat.IsZero() {
		t.Fatal("spawnNpc(npc-rat-scuttler) failed")
	}
	drain(s)

	var evs []Event
	stepCollect(s, &evs, 8000)

	if !hasEvent(evs, EvDestroyed, "Rat Scuttler destroyed") {
		t.Fatalf("guard never cleared the belt rat (state=%s)", aiState(s, rat))
	}
	if !hasEvent(evs, EvLog, "Guard hits Rat Scuttler") {
		t.Fatal("the guard never fired on the rat")
	}
	if !hasEvent(evs, EvLog, "Rat Scuttler hits Guard") {
		t.Fatal("the rat never returned fire (one-sided execution)")
	}
	if hasEvent(evs, EvLog, "is attacking you") {
		t.Fatal("a friendly guard aggroed the player")
	}
	if infoHostile(s, guard) {
		t.Fatal("an engaged guard must stay friendly on the overview")
	}
	if tg, ok := s.Targetings.Get(guard); ok && len(tg.Locks) != 0 {
		t.Fatalf("guard holds %d lock(s), want none", len(tg.Locks))
	}
}

// TestPatrolSweepsOccupiedHighSecBelt pins the traffic half: the system's
// scheduled patrols put an occupied belt on the board, one of them dispatches
// to it, kills the rats and returns to its anchor.
func TestPatrolSweepsOccupiedHighSecBelt(t *testing.T) {
	s := newBeltSim(t, trafficContent(), 7) // sys-a: high, patrol beat 600 ticks
	belt := s.celEntity("cel-belt-a")
	beltPos := beltPosOf(t, s, "cel-belt-a")
	rat := s.spawnNpc("npc-rat-scuttler", beltPos, 100)
	if rat.IsZero() {
		t.Fatal("spawnNpc(npc-rat-scuttler) failed")
	}
	drain(s)

	var evs []Event
	dispatched := false
	cleared := false
	for i := 0; i < 12000; i++ {
		s.Step()
		evs = append(evs, drain(s)...)
		for _, e := range sortedEntities(s.Patrols) {
			if p, ok := s.Patrols.Get(e); ok && p.State == PatrolRespond {
				dispatched = true
				if p.Incident != belt {
					t.Fatalf("patrol responding to %v, want the belt %v", p.Incident, belt)
				}
			}
		}
		if !s.World.Alive(rat) {
			cleared = true
			break
		}
	}
	if !dispatched {
		t.Fatal("no patrol ever dispatched to the occupied belt")
	}
	if !cleared {
		t.Fatalf("the belt rat survived the sweep (state=%s)", aiState(s, rat))
	}
	if !hasEvent(evs, EvLog, "Fixture Belt cleared") {
		t.Fatal("no cleared line for the emptied belt")
	}

	// Returns home once the belt is clear.
	var back []Event
	stepCollect(s, &back, 6000)
	for _, e := range sortedEntities(s.Patrols) {
		if p, ok := s.Patrols.Get(e); ok && p.State != PatrolIdle {
			t.Fatalf("patrol %v still %s after the sweep", e, p.State)
		}
	}
	if ratsAt(s, beltPos) != 0 {
		t.Fatal("the belt was not emptied")
	}
}

// TestLowSecBeltGetsNoSecurityResponse is the high-sec gate: the same belt
// occupancy in a low-sec system with the same patrol schedule gets no sweep —
// the positive control is TestPatrolSweepsOccupiedHighSecBelt.
func TestLowSecBeltGetsNoSecurityResponse(t *testing.T) {
	c := trafficContent()
	// Give the lawless lane the same patrol schedule the high-sec system has.
	c.Traffic["sys-b"].Patrol = &content.PatrolDef{
		Hulls: []content.ShipID{"ship-guard"}, IntervalTicks: 600,
	}
	s := newBeltSim(t, c, 7)
	if err := s.LoadSystem("sys-b", ""); err != nil {
		t.Fatalf("LoadSystem: %v", err)
	}
	setPlayerPos(s, core.Vec2{X: -5e6, Y: -5e6})
	drain(s)
	beltPos := beltPosOf(t, s, "cel-belt-b")
	if n := ratsAt(s, beltPos); n != 1 {
		t.Fatalf("fixture: sys-b belt holds %d rats, want 1", n)
	}

	var evs []Event
	stepCollect(s, &evs, 12000)

	for _, e := range sortedEntities(s.Patrols) {
		if p, ok := s.Patrols.Get(e); ok && p.State != PatrolIdle {
			t.Fatalf("low-sec patrol %v left its anchor (%s)", e, p.State)
		}
	}
	if n := ratsAt(s, beltPos); n != 1 {
		t.Fatalf("low-sec belt rats=%d, want 1 (security must not respond)", n)
	}
	if hasEvent(evs, EvLog, "cleared") {
		t.Fatal("a low-sec belt must never log a cleared line")
	}
}

// TestBeltClearedLogFiresOncePerEmptying: the cleared line names the belt,
// fires exactly once when the last rat dies (whoever killed it), and never for
// a survivor, a guard, or a low-sec belt.
func TestBeltClearedLogFiresOncePerEmptying(t *testing.T) {
	count := func(evs []Event) int {
		n := 0
		for _, ev := range evs {
			if ev.Kind == EvLog && ev.Text == "Fixture Belt cleared" {
				n++
			}
		}
		return n
	}

	// Two rats: the first death says nothing, the second says it once.
	c := trafficContent()
	c.Traffic["sys-a"].Patrol = nil
	s := newBeltSim(t, c, 7)
	belt := beltPosOf(t, s, "cel-belt-a")
	a := s.spawnNpc("npc-rat-scuttler", belt, 100)
	b := s.spawnNpc("npc-rat-scuttler", belt, 100)
	drain(s)
	var evs []Event
	damageNPC(s, s.player, a, contentDamage(), false)
	stepCollect(s, &evs, 1)
	if got := count(evs); got != 0 {
		t.Fatalf("cleared lines=%d with a survivor, want 0", got)
	}
	damageNPC(s, s.player, b, contentDamage(), false)
	stepCollect(s, &evs, 1)
	if got := count(evs); got != 1 {
		t.Fatalf("cleared lines=%d after emptying, want 1", got)
	}
	// The belt re-arms on its own: a fresh rat cleared again speaks again.
	d := s.spawnNpc("npc-rat-scuttler", belt, 100)
	drain(s)
	damageNPC(s, s.player, d, contentDamage(), false)
	stepCollect(s, &evs, 1)
	if got := count(evs); got != 2 {
		t.Fatalf("cleared lines=%d after the second emptying, want 2", got)
	}

	// Both rats dying in the same tick still speak exactly once.
	tc := trafficContent()
	tc.Traffic["sys-a"].Patrol = nil
	ts := newBeltSim(t, tc, 7)
	tbelt := beltPosOf(t, ts, "cel-belt-a")
	ta := ts.spawnNpc("npc-rat-scuttler", tbelt, 100)
	tb := ts.spawnNpc("npc-rat-scuttler", tbelt, 100)
	drain(ts)
	damageNPC(ts, ts.player, ta, contentDamage(), false)
	damageNPC(ts, ts.player, tb, contentDamage(), false)
	var tevs []Event
	stepCollect(ts, &tevs, 1)
	if got := count(tevs); got != 1 {
		t.Fatalf("cleared lines=%d for a simultaneous emptying, want 1", got)
	}

	// A guard dying at a belt is not a cleared belt.
	g := newBeltSim(t, trafficContent(), 7)
	guard := spawnStaticGuard(t, g, beltPosOf(t, g, "cel-belt-a"))
	drain(g)
	var gevs []Event
	damageNPC(g, g.player, guard, contentDamage(), false)
	stepCollect(g, &gevs, 1)
	if got := count(gevs); got != 0 {
		t.Fatalf("cleared lines=%d for a dying guard, want 0", got)
	}

	// Low-sec belts are out of scope (the high-sec gate).
	l := trafficContent()
	l.Systems["sys-a"].Security = "low"
	ls := newBeltSim(t, l, 7)
	rat := ls.spawnNpc("npc-rat-scuttler", beltPosOf(t, ls, "cel-belt-a"), 100)
	drain(ls)
	var levs []Event
	damageNPC(ls, ls.player, rat, contentDamage(), false)
	stepCollect(ls, &levs, 1)
	if got := count(levs); got != 0 {
		t.Fatalf("cleared lines=%d for a low-sec belt, want 0", got)
	}
}

// TestPatrolBeltSweepTwinRunAndSaveRestoreIdentical: the belt response is
// deterministic — twin runs hash identically, and a snapshot taken mid-sweep
// restores into a sim that tracks the uninterrupted run to the last tick.
func TestPatrolBeltSweepTwinRunAndSaveRestoreIdentical(t *testing.T) {
	const ticks, restoreAt = 6000, 2500
	newRun := func() (*Sim, ecs.Entity) {
		s := newBeltSim(t, trafficContent(), 11)
		rat := s.spawnNpc("npc-rat-scuttler", beltPosOf(t, s, "cel-belt-a"), 100)
		if rat.IsZero() {
			t.Fatal("spawnNpc(npc-rat-scuttler) failed")
		}
		return s, rat
	}
	a, ratA := newRun()
	b, ratB := newRun()
	if ratA != ratB {
		t.Fatalf("twin spawns handed out different handles: %v vs %v", ratA, ratB)
	}

	var c *Sim
	swept := false
	for i := 0; i < ticks; i++ {
		a.Step()
		b.Step()
		if !a.World.Alive(ratA) {
			swept = true
		}
		if i == restoreAt-1 {
			save, err := b.Snapshot()
			if err != nil {
				t.Fatalf("snapshot: %v", err)
			}
			// Mid-response: at least one patrol is working the belt.
			working := false
			for _, e := range sortedEntities(b.Patrols) {
				if p, ok := b.Patrols.Get(e); ok && p.State == PatrolRespond {
					working = true
				}
			}
			if !working {
				t.Fatal("fixture: no patrol responding at the save point")
			}
			c, err = New(trafficContent(), testWorld(), 11)
			if err != nil {
				t.Fatalf("fresh sim: %v", err)
			}
			if err := c.Restore(save); err != nil {
				t.Fatalf("restore: %v", err)
			}
			if c.Hash() != b.Hash() {
				t.Fatalf("restored hash=%016x want %016x", c.Hash(), b.Hash())
			}
		}
		if c != nil && i != restoreAt-1 {
			c.Step()
		}
		if i%500 == 499 {
			if a.Hash() != b.Hash() {
				t.Fatalf("tick %d: twin hashes differ: a=%016x b=%016x", i+1, a.Hash(), b.Hash())
			}
			if c != nil && a.Hash() != c.Hash() {
				t.Fatalf("tick %d: restored run diverged: a=%016x c=%016x", i+1, a.Hash(), c.Hash())
			}
		}
	}
	if !swept {
		t.Fatal("fixture: the belt was never swept in the window")
	}
	if a.Hash() != b.Hash() {
		t.Fatalf("final twin hashes differ: a=%016x b=%016x", a.Hash(), b.Hash())
	}
	if c == nil {
		t.Fatal("restore never happened")
	}
	if a.Hash() != c.Hash() {
		t.Fatalf("final restored hash=%016x want %016x", c.Hash(), a.Hash())
	}
}

// beltClearedLines counts the "<belt> cleared" lines in evs.
func beltClearedLines(evs []Event) int {
	n := 0
	for _, ev := range evs {
		if ev.Kind == EvLog && ev.Text == "Fixture Belt cleared" {
			n++
		}
	}
	return n
}

// TestPatrolSweepFinishesTheBelt: the sweep stays until the belt is empty.
// The two rats the old rule abandoned are still there — one flinching at a
// nearby player (trigger (b) never expires, so ratFree treated it as engaged
// forever), one far off its Home anchor — and the patrol must clear both
// before it logs and flies home.
func TestPatrolSweepFinishesTheBelt(t *testing.T) {
	s := newBeltSim(t, trafficContent(), 7) // sys-a: high, patrol beat 600
	belt := beltPosOf(t, s, "cel-belt-a")
	// The player sits inside the fixture scuttler's high-sec leash (2.5 ×
	// 6 km) but outside every trigger radius, chips one rat, then stops
	// fighting: that rat keeps flinching onto the player and is never "free".
	setPlayerPos(s, belt.Add(core.Vec2{X: 12000}))
	flinch := s.spawnNpc("npc-rat-scuttler", belt, 100)
	damageNPC(s, s.player, flinch, scratchDamage(), false)
	straggler := s.spawnNpc("npc-rat-scuttler", belt, 100)
	teleport(s, straggler, belt.Add(core.Vec2{X: -300000}))
	drain(s)

	var evs []Event
	for i := 0; i < 30000 && (s.World.Alive(flinch) || s.World.Alive(straggler)); i++ {
		s.Step()
		evs = append(evs, drain(s)...)
	}
	if s.World.Alive(flinch) {
		t.Fatalf("the sweep left the flinching rat alive (state=%s)", aiState(s, flinch))
	}
	if s.World.Alive(straggler) {
		t.Fatalf("the sweep left the straggler alive (state=%s)", aiState(s, straggler))
	}
	if n := beltClearedLines(evs); n != 1 {
		t.Fatalf("cleared lines=%d after emptying the belt, want exactly 1", n)
	}
	var home []Event
	stepCollect(s, &home, 6000)
	for _, e := range sortedEntities(s.Patrols) {
		if p, ok := s.Patrols.Get(e); ok && p.State != PatrolIdle {
			t.Fatalf("patrol %v still %s after the belt emptied", e, p.State)
		}
	}
}

// TestHighSecGuardFinishesPlayerTouchedRat: the static guard's completion
// rule — it fights the belt to empty even when the only rat left is
// flinching at a nearby player, and it still never initiates on the player.
func TestHighSecGuardFinishesPlayerTouchedRat(t *testing.T) {
	c := trafficContent()
	c.Traffic["sys-a"].Patrol = nil // isolate: no scheduled patrols in this pin
	c.NPCs["npc-rat-scuttler"].Fit = []content.ModuleID{"mod-test-turret"}
	s := newBeltSim(t, c, 7)
	belt := beltPosOf(t, s, "cel-belt-a")
	setPlayerPos(s, belt.Add(core.Vec2{X: 12000}))
	guard := spawnStaticGuard(t, s, belt)
	rat := s.spawnNpc("npc-rat-scuttler", belt, 100)
	if rat.IsZero() {
		t.Fatal("spawnNpc(npc-rat-scuttler) failed")
	}
	damageNPC(s, s.player, rat, scratchDamage(), false)
	drain(s)

	var evs []Event
	for i := 0; i < 40000 && s.World.Alive(rat); i++ {
		s.Step()
		evs = append(evs, drain(s)...)
	}
	if s.World.Alive(rat) {
		t.Fatalf("the guard left a player-touched rat alive (state=%s)", aiState(s, rat))
	}
	if hasEvent(evs, EvLog, "Guard is attacking you") {
		t.Fatal("a friendly guard aggroed the player")
	}
	if infoHostile(s, guard) {
		t.Fatal("an engaged guard must stay friendly on the overview")
	}
}

// aiState is a small readback for failure messages.
func aiState(s *Sim, e ecs.Entity) string {
	if ai, ok := s.AIs.Get(e); ok {
		return string(ai.State)
	}
	return "<gone>"
}
