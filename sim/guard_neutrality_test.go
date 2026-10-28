package sim

import (
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// ---------------------------------------------------------------------------
// Guard neutrality: friendly security patrols (npc-guard-*) never initiate
// on the PLAYER. The threat-response triggers — proximity, intrusion near
// Home, recent player work — exist so pirate rats are not passive; they
// must not make friendly security open fire on a pilot who is merely
// flying near the guard's anchor. A guard turns hostile only on
// player-sourced damage (self-defense), and keeps working incidents.
//
// Scope (re-targeted when the belt response landed): guards DO initiate on
// pirate rats anchored at their own celestial in a high-security system
// (the ambient security rule, sim/patrol_belt_test.go) — the rat case is
// the exception this pin deliberately does not cover. Nothing here changes:
// the player is still the one target a guard never opens on unprovoked.
// ---------------------------------------------------------------------------

// spawnIdleGuard places a fixture guard patrol exactly at home, on duty.
// It carries Patrol state, i.e. it is a TRAFFIC patrol: the belt sweep owns
// it and the static-guard rat rule (which this pin is about) never applies.
func spawnIdleGuard(t *testing.T, s *Sim, home core.Vec2) ecs.Entity {
	t.Helper()
	e := s.spawnNpc("npc-guard-test", home, 0)
	if e.IsZero() {
		t.Fatal("spawnNpc(npc-guard-test) failed")
	}
	s.Patrols.Set(e, Patrol{State: PatrolIdle})
	return e
}

// assertGuardNeutral parks the player (already positioned) for 600 ticks
// and fails the moment the guard leaves its idle circle, locks the player
// or emits an attack. The failure text names the trigger that fired
// (proximity a / intrusion d / recent work c) with its measured values.
func assertGuardNeutral(t *testing.T, s *Sim, guard ecs.Entity) {
	t.Helper()
	ai, ok := s.AIs.Get(guard)
	if !ok {
		t.Fatal("guard has no AI state")
	}
	def, _ := s.Content.Npc(ai.Npc)
	effAggro, _ := npcTemperament(s.System().Security, def)
	var evs []Event
	for i := 0; i < 600; i++ {
		s.Step()
		evs = append(evs, drain(s)...)
		st, _ := s.AIs.Get(guard)
		tp, _ := s.posOf(guard)
		pp := playerPos(s)
		dist, homeDist := pp.Dist(tp), ai.Home.Dist(pp)
		if st.State != NpcIdle {
			fired := ""
			if dist <= effAggro {
				fired += " proximity(a)"
			}
			if homeDist <= npcGuardRange {
				fired += " intrusion(d)"
			}
			if s.recentPlayerWorkNear(ai.Home) {
				fired += " work(c)"
			}
			t.Fatalf("tick %d: state=%q want idle — guard initiated on the player;"+
				" fired:%s (dist=%v, homeDist=%v, effAggro=%v)",
				i+1, st.State, fired, dist, homeDist, effAggro)
		}
		if tg, ok := s.Targetings.Get(guard); ok && len(tg.Locks) != 0 {
			t.Fatalf("tick %d: idle guard holds %d lock(s), want none", i+1, len(tg.Locks))
		}
	}
	for _, ev := range evs {
		if ev.Actor == guard && hasText(ev, "is attacking you") {
			t.Fatalf("guard emitted an attack event: %q", ev.Text)
		}
	}
	if infoHostile(s, guard) {
		t.Fatal("idle guard reads hostile")
	}
}

// TestGuardPatrolNeverInitiates: an idle patrol stays idle, lockless and
// quiet for 600 ticks while the player parks inside every threat-response
// band — ~5 km close approach, the ~8 km intrusion edge, low-sec
// proximity (effAggro 6 km) and recent weapon work near the guard's Home.
func TestGuardPatrolNeverInitiates(t *testing.T) {
	cases := []struct {
		name     string
		security string
		home     core.Vec2 // guard Home (spawn point)
		player   core.Vec2 // where the player parks
		work     bool      // player fires a shot during the run (trigger c)
	}{
		// The reported bug: ~5 km from a patrol at its anchor. Inside the
		// 8 km intrusion band in every security; high-sec effAggro is
		// 2400 m so only trigger (d) can fire here.
		{"5 km close approach", "high", v(1e7+5000, 500), v(1e7, 500), false},
		// Edge of the intrusion band (~8 km): only trigger (d) can fire.
		{"7.9 km anchor band edge", "high", v(1e7+7900, 500), v(1e7, 500), false},
		// Low security: effAggro = 6000 m >= 5 km, so proximity (a) also
		// fires alongside the intrusion band.
		{"5 km low security", "low", v(1e7+5000, 500), v(1e7, 500), false},
		// Recent player weapon work within 30 km of Home (trigger c),
		// Home beyond the 8 km band so only (c) can fire.
		{"recent player work near home", "high", v(1e7+20000, 500), v(1e7, 500), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newTrafficSim(t)
			s.Content.Systems["sys-a"].Security = c.security
			guard := spawnIdleGuard(t, s, c.home)
			setPlayerPos(s, c.player)
			if c.work {
				dummy := dummyTarget(t, s, v(1e7+5000, 500), content.Resists{})
				forceLock(s, dummy)
				s.Queue(CmdActivate{Slot: SlotRef{Class: content.SlotHigh, Index: 0}, Target: dummy})
			}
			drain(s)
			assertGuardNeutral(t, s, guard)
		})
	}
}

// TestGuardPatrolDefendsWhenAttacked: same-faction damage (a sister patrol)
// leaves the guard on duty — trigger (b) is source-checked, so an ally's
// fire never flips it — while player-sourced damage flips it onto the player
// (self-defense, LastHitBy attribution) and it locks through the normal
// targeting system.
func TestGuardPatrolDefendsWhenAttacked(t *testing.T) {
	s := newTrafficSim(t)
	guard := spawnIdleGuard(t, s, v(1e7+5000, 500))
	setPlayerPos(s, v(1e7, 500))
	drain(s)
	ai, ok := s.AIs.Get(guard)
	if !ok {
		t.Fatal("guard has no AI state")
	}

	// Same-faction hit: a sister patrol's fire is not the player's fight.
	src := spawnIdleGuard(t, s, v(1e7+40000, 500))
	damageNPC(s, src, guard, scratchDamage(), false)
	s.Step()
	drain(s)
	if ai.State != NpcIdle {
		t.Fatalf("state=%q want idle after same-faction damage", ai.State)
	}
	if ai.LastHitBy != src {
		t.Fatalf("LastHitBy=%v want the third party %v", ai.LastHitBy, src)
	}

	// Player hit: self-defense.
	damageNPC(s, s.player, guard, scratchDamage(), false)
	s.Step()
	evs := drain(s)
	if ai.State != NpcApproach {
		t.Fatalf("state=%q want approach after player-sourced damage", ai.State)
	}
	if ai.LastHitBy != s.player {
		t.Fatalf("LastHitBy=%v want the player %v", ai.LastHitBy, s.player)
	}
	if findEvent(evs, EvLog, "is attacking you") == nil {
		t.Fatalf("missing aggro warning: %v", evs)
	}
	locked := false
	for i := 0; i < 60 && !locked; i++ {
		s.Step()
		drain(s)
		if tg, ok := s.Targetings.Get(guard); ok {
			for _, l := range tg.Locks {
				if l.Target == s.player {
					locked = true
				}
			}
		}
	}
	if !locked {
		t.Fatal("defending guard never locked the player")
	}
}

// guardEncounterTick flies the player to the guard's anchor, chips the
// guard's shield (it defends) and closes in. Every action derives from the
// tick index and sim state, so twin runs are bit-identical.
func guardEncounterTick(s *Sim, guard ecs.Entity, i int) {
	switch i {
	case 5:
		setPlayerPos(s, v(1e7, 500)) // 5 km from the guard's Home
	case 50:
		damageNPC(s, s.player, guard, scratchDamage(), false) // provoke self-defense
	case 400:
		setPlayerPos(s, v(1e7+3000, 500)) // close in, still leashed
	}
	s.Step()
}

// TestGuardPatrolTwinRunAndSaveRestoreIdentical: the patrol-encounter
// script produces bit-identical hashes across twin sims, and a snapshot
// taken mid-encounter (guard damaged and engaged) restores into a fresh
// sim that then tracks the uninterrupted run to the last tick.
func TestGuardPatrolTwinRunAndSaveRestoreIdentical(t *testing.T) {
	const ticks, restoreAt = 700, 350
	newRun := func() (*Sim, ecs.Entity) {
		s := newTrafficSimSeed(t, 11)
		guard := spawnIdleGuard(t, s, v(1e7+5000, 500))
		drain(s)
		return s, guard
	}
	a, guardA := newRun()
	b, guardB := newRun()
	if guardA != guardB {
		t.Fatalf("twin spawns handed out different handles: %v vs %v", guardA, guardB)
	}

	var c *Sim
	for i := 0; i < ticks; i++ {
		guardEncounterTick(a, guardA, i)
		guardEncounterTick(b, guardB, i)
		if i == restoreAt-1 {
			save, err := b.Snapshot()
			if err != nil {
				t.Fatalf("snapshot: %v", err)
			}
			// Mid-encounter: the guard is damaged and engaged with the
			// player (LastHitBy, state and locks all inside Save).
			ai, _ := b.AIs.Get(guardB)
			if ai.State != NpcApproach && ai.State != NpcOrbit {
				t.Fatalf("fixture: guard state=%q want an engagement at the save point", ai.State)
			}
			c, err = New(trafficContent(), testWorld(), 11)
			if err != nil {
				t.Fatalf("fresh sim: %v", err)
			}
			if err := c.Restore(save); err != nil {
				t.Fatalf("restore: %v", err)
			}
			if c.Hash() != b.Hash() {
				t.Fatalf("restored hash=%016x want %016x (mid-encounter round-trip)",
					c.Hash(), b.Hash())
			}
		}
		if c != nil && i != restoreAt-1 {
			// Handles survive the restore; c starts at the tick AFTER the
			// snapshot (the snapshot already contains this tick's step).
			guardEncounterTick(c, guardB, i)
		}
		if i%100 == 99 {
			if a.Hash() != b.Hash() {
				t.Fatalf("tick %d: twin hashes differ: a=%016x b=%016x", i+1, a.Hash(), b.Hash())
			}
			if c != nil && a.Hash() != c.Hash() {
				t.Fatalf("tick %d: restored run diverged: a=%016x c=%016x", i+1, a.Hash(), c.Hash())
			}
		}
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
