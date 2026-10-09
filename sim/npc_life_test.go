package sim

import (
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
)

// ---------------------------------------------------------------------------
// Living NPCs: deterministic idle patrol, aggro triggers, security
// temperament and the pursuit leash.
// ---------------------------------------------------------------------------

// patrolRadiusFor returns the documented patrol radius of an NPC def.
func patrolRadiusFor(def *content.NpcDef) float64 {
	r := def.OrbitRange
	if r > npcPatrolMaxRadius {
		r = npcPatrolMaxRadius
	}
	return r
}

// TestNpcIdlePatrolMovesAndStaysNearHome: idle NPCs never sit still — they
// drift on a patrol circle around Home, bounded by the patrol radius.
func TestNpcIdlePatrolMovesAndStaysNearHome(t *testing.T) {
	s := newTestSim(t)
	// Far from the player: no trigger can fire (no damage, no work, and
	// the player is outside effAggro and the 8 km guard range).
	npc := spawnNearPlayerAt(t, s, v(1e7, 60000))
	ai, _ := s.AIs.Get(npc)
	def, _ := s.Content.Npc(ai.Npc)
	radius := patrolRadiusFor(def)
	home := ai.Home

	before, _ := s.posOf(npc)
	stepN(s, 600)
	after, _ := s.posOf(npc)
	if ai.State != NpcIdle {
		t.Fatalf("state=%q want idle (no trigger fired)", ai.State)
	}
	if before == after {
		t.Fatal("idle NPC must patrol: position unchanged over 600 ticks")
	}
	if d := after.Dist(home); d > radius {
		t.Fatalf("patrol drifted to %v m from Home, want <= %v", d, radius)
	}
	// Keep patrolling: several more samples must stay on the circle too.
	for i := 0; i < 5; i++ {
		stepN(s, 1200)
		p, _ := s.posOf(npc)
		if d := p.Dist(home); d > radius {
			t.Fatalf("patrol sample %d at %v m from Home, want <= %v", i, d, radius)
		}
	}
	if ai.State != NpcIdle {
		t.Fatalf("state=%q want idle after long patrol", ai.State)
	}
}

// TestNpcPatrolTwinSimsIdentical: same seed + same spawns => bit-identical
// patrol trajectories (the phase comes from the entity handle, never RNG).
func TestNpcPatrolTwinSimsIdentical(t *testing.T) {
	a := newTestSim(t)
	b := newTestSim(t)
	pos := v(1e7, 60000)
	ea := a.spawnNpc("npc-rat-scuttler", pos, 100)
	eb := b.spawnNpc("npc-rat-scuttler", pos, 100)
	if ea.IsZero() || ea != eb {
		t.Fatalf("twin spawns must hand out identical handles: %v vs %v", ea, eb)
	}
	for i := 0; i < 6000; i++ {
		a.Step()
		b.Step()
		if i%500 == 499 {
			pa, _ := a.posOf(ea)
			pb, _ := b.posOf(eb)
			if pa != pb {
				t.Fatalf("tick %d: patrol diverged: %v vs %v", i+1, pa, pb)
			}
			if a.Hash() != b.Hash() {
				t.Fatalf("tick %d: hash diverged: %016x vs %016x", i+1, a.Hash(), b.Hash())
			}
		}
	}
	// Patrol must actually move in the twins (not just stay identical).
	p0 := v(1e7, 60000)
	p1, _ := a.posOf(ea)
	if p1 == p0 {
		t.Fatal("twin patrol never moved")
	}
}

// TestNpcAggroTriggers: each trigger (a)-(d) independently aggros a fresh
// idle NPC; with no trigger at all the NPC keeps patrolling.
func TestNpcAggroTriggers(t *testing.T) {
	cases := []struct {
		name  string
		aggro bool
		setup func(t *testing.T, s *Sim) ecs.Entity
	}{
		{"a player within effective aggro range", true,
			func(t *testing.T, s *Sim) ecs.Entity {
				npc := spawnNearPlayerAt(t, s, v(1e7, 4500))
				// Push Home out of the 8 km guard range so only trigger (a)
				// can fire: the NPC itself stays ~4.5 km from the player.
				ai, _ := s.AIs.Get(npc)
				ai.Home = v(1e7+10000, 500)
				return npc
			}},
		{"b npc took damage", true,
			func(t *testing.T, s *Sim) ecs.Entity {
				// 60 km out: beyond effAggro (6 km high-sec) and the guard.
				npc := spawnNearPlayerAt(t, s, v(1e7, 60000))
				// Chip the shield: damaged, not dead.
				damageNPC(s, s.player, npc, scratchDamage(), false)
				return npc
			}},
		{"c player fired near home recently", true,
			func(t *testing.T, s *Sim) ecs.Entity {
				// 10 km from the player: beyond effAggro and the guard.
				npc := spawnNearPlayerAt(t, s, v(1e7+10000, 500))
				// The player fires a shot at a dummy within lock range;
				// fireWeapon records the work for trigger (c).
				dummy := dummyTarget(t, s, v(1e7+2000, 500), content.Resists{})
				forceLock(s, dummy)
				s.Queue(CmdActivate{Slot: SlotRef{Class: content.SlotHigh, Index: 0}, Target: dummy})
				return npc
			}},
		{"d interloper within 8 km of home", true,
			func(t *testing.T, s *Sim) ecs.Entity {
				// 7.5 km: inside the guard range, beyond effAggro.
				return spawnNearPlayerAt(t, s, v(1e7+7500, 500))
			}},
		{"no trigger out of all ranges", false,
			func(t *testing.T, s *Sim) ecs.Entity {
				return spawnNearPlayerAt(t, s, v(1e7, 60000))
			}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newTestSim(t)
			npc := c.setup(t, s)
			ai, _ := s.AIs.Get(npc)
			s.Step()
			evs := drain(s)
			aggroLog := findEvent(evs, EvLog, "is attacking you")
			if c.aggro {
				if ai.State != NpcApproach {
					t.Fatalf("state=%q want approach", ai.State)
				}
				if aggroLog == nil {
					t.Fatalf("missing aggro log: %v", evs)
				}
				if aggroLog.Severity != SevWarning {
					t.Fatalf("aggro severity=%d want SevWarning", aggroLog.Severity)
				}
				return
			}
			// No trigger: keep patrolling, hostile only when aggroed.
			stepN(s, 300)
			drain(s)
			if ai.State != NpcIdle {
				t.Fatalf("state=%q want idle with no trigger", ai.State)
			}
			if infoHostile(s, npc) {
				t.Fatal("idle NPC must not be hostile")
			}
		})
	}
}

// TestNpcSecurityTemperament pins the security table: effAggro and the
// flee threshold per SystemDef.Security, behaviorally and as pure values.
func TestNpcSecurityTemperament(t *testing.T) {
	def := &content.NpcDef{AggroRange: 15000, FleeThreshold: 0.25}
	cases := []struct {
		security   string
		effAggro   float64
		effFlee    float64
		aggroAt10k bool // 10 km out (beyond the 8 km guard range)
		fleeAt030  bool // structure fraction 0.30
	}{
		{"high", 6000, 0.35, false, true}, // skittish: late to fight, early to run
		{"low", 15000, 0.25, true, false}, // as authored
		{"null", 19500, 0, true, false},   // fights to the death
	}
	for _, c := range cases {
		t.Run(c.security, func(t *testing.T) {
			// Pure table values.
			aggro, flee := npcTemperament(c.security, def)
			if aggro != c.effAggro || flee != c.effFlee {
				t.Fatalf("temperament(%s)=(%v,%v) want (%v,%v)",
					c.security, aggro, flee, c.effAggro, c.effFlee)
			}

			// Behavioral effAggro: a player 10 km out (inside every leash,
			// beyond the guard range) is noticed iff 10 km <= effAggro.
			s := newTestSim(t)
			s.Content.Systems["sys-a"].Security = c.security
			npc := spawnNearPlayerAt(t, s, v(1e7+10000, 500))
			ai, _ := s.AIs.Get(npc)
			s.Step()
			drain(s)
			wantAggro := c.aggroAt10k
			gotAggro := ai.State == NpcApproach || ai.State == NpcOrbit
			if gotAggro != wantAggro {
				t.Fatalf("aggro at 10 km = %v want %v (state=%q)",
					gotAggro, wantAggro, ai.State)
			}

			// Behavioral flee threshold: structure at 0.30.
			s2 := newTestSim(t)
			s2.Content.Systems["sys-a"].Security = c.security
			npc2 := spawnNearPlayerAt(t, s2, v(1e7, 4000))
			ai2, _ := s2.AIs.Get(npc2)
			s2.Step() // aggro -> approach
			drain(s2)
			if ai2.State != NpcApproach {
				t.Fatalf("setup: state=%q want approach", ai2.State)
			}
			// 370 raw EM: shield 200 + armor 100 + 70 -> structure 30/100.
			damageNPC(s2, s2.player, npc2, content.Damage{EM: 370}, false)
			s2.Step()
			evs := drain(s2)
			fled := ai2.State == NpcFlee
			if fled != c.fleeAt030 {
				t.Fatalf("flee at structure 0.30 = %v want %v (state=%q)",
					fled, c.fleeAt030, ai2.State)
			}
			if fled && findEvent(evs, EvLog, "is fleeing") == nil {
				t.Fatalf("missing flee log: %v", evs)
			}
			if !fled && findEvent(evs, EvLog, "is fleeing") != nil {
				t.Fatal("unexpected flee log")
			}
		})
	}
}

// TestNpcLeashBreaksContact: an aggroed NPC keeps hostile while the player
// is within 2.5*effAggro of its Home and breaks off (-> return) beyond it.
func TestNpcLeashBreaksContact(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayerAt(t, s, v(1e7, 4000))
	ai, _ := s.AIs.Get(npc)
	def, _ := s.Content.Npc(ai.Npc)
	aggro, _ := npcTemperament(s.System().Security, def)
	leash := npcLeashFactor * aggro

	stepN(s, 5)
	drain(s)
	if ai.State != NpcApproach && ai.State != NpcOrbit {
		t.Fatalf("setup: state=%q want aggro", ai.State)
	}
	// Player inside the leash: stays engaged and hostile.
	stepN(s, 60)
	drain(s)
	if ai.State != NpcApproach && ai.State != NpcOrbit {
		t.Fatalf("state=%q want continued engagement inside leash %v", ai.State, leash)
	}
	if !infoHostile(s, npc) {
		t.Fatal("engaged NPC must be hostile")
	}

	// Player beyond 2.5*effAggro of Home: contact broken, -> return.
	home := ai.Home
	setPlayerPos(s, home.Add(v(leash+1000, 500)))
	s.Step()
	drain(s)
	if ai.State != NpcReturn {
		t.Fatalf("state=%q want return beyond leash %v (homeDist=%v)",
			ai.State, leash, home.Dist(playerPos(s)))
	}
	if infoHostile(s, npc) {
		t.Fatal("returning NPC must not be hostile")
	}

	// It settles home and patrols again.
	settled := false
	for i := 0; i < 3600; i++ {
		s.Step()
		drain(s)
		p, _ := s.posOf(npc)
		if ai.State == NpcIdle && p.Dist(home) <= npcReturnRange {
			settled = true
			break
		}
	}
	if !settled {
		p, _ := s.posOf(npc)
		t.Fatalf("NPC never settled: state=%q dist=%v", ai.State, p.Dist(home))
	}
}
