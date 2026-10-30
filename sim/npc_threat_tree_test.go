package sim

import (
	"math"
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
)

// ---------------------------------------------------------------------------
// Prey preference: the selection-time ranking (which candidate is picked).
//
// RED (quoted from the pre-change npcThreat): "the damage source (b) always
// wins, else the nearest, ties by canonical order" — selection had no notion
// of what a candidate was worth:
//
//	if !hit { continue }
//	if damaged && ai.LastHitBy == c.e { return c.e } // (b) beats every ambient trigger
//	if d := selfPos.Dist(c.pos); best.IsZero() || d < bestD { best, bestD = c.e, d }
//
// Now the ambient candidates are ranked by 0.5*(carry/maxCarry) +
// 0.5*(guns/maxGuns) over the eligible set; ties go to the nearer candidate,
// then canonical handle order. Trigger (b) still short-circuits (initial
// aggro), and a candidate outside the triggers is never ranked at all.
// ---------------------------------------------------------------------------

// TestNpcThreatRanksByPreyScore: a guard (small hold, three guns) outscores a
// mining barge (big hold, no guns) even though the barge is nearer.
func TestNpcThreatRanksByPreyScore(t *testing.T) {
	s := newTestSim(t)
	belt := mustPos(s, s.celEntity("cel-belt-a"))
	miner := spawnMiner(t, s, belt.Add(v(500, 0)))
	// The armed candidate: the player's sloop carries one gun and less hold
	// than the barge, and stands farther from the rat.
	setPlayerPos(s, belt.Add(v(4000, 0)))
	rat := s.spawnNpc("npc-rat-scuttler", belt.Add(v(2000, 0)), 0)
	if rat.IsZero() {
		t.Fatal("rat spawn failed")
	}
	ai, _ := s.AIs.Get(rat)
	def, _ := s.Content.Npc(ai.Npc)
	mc, mg := s.preyProfile(miner)
	pc, pg := s.preyProfile(s.player)
	if !(pc < mc && pg > mg) {
		t.Fatalf("fixture: want player hold %.0f < miner hold %.0f and player guns %.0f > miner guns %.0f", pc, mc, pg, mg)
	}
	if d := mustPos(s, rat).Dist(mustPos(s, miner)); d > mustPos(s, rat).Dist(mustPos(s, s.player)) {
		t.Fatalf("fixture: the miner must be the nearer candidate (%.0f m)", d)
	}
	if got := s.npcThreat(rat, mustPos(s, rat), ai, def, false, s.npcCandidates(), 0, math.MaxFloat64, false); got != s.player {
		t.Fatalf("target = %v, want the armed player %v (carry %.0f/%.0f, guns %.0f/%.0f)", got, s.player, pc, mc, pg, mg)
	}
}

// TestNpcThreatTieBreaksToNearestThenCanonical: equal scores go to the nearer
// candidate, and an exact distance tie to the canonical (lower) handle.
func TestNpcThreatTieBreaksToNearestThenCanonical(t *testing.T) {
	s := newTestSim(t)
	belt := mustPos(s, s.celEntity("cel-belt-a"))
	first := spawnMiner(t, s, belt.Add(v(1000, 0)))
	second := spawnMiner(t, s, belt.Add(v(-1000, 0)))
	if first > second {
		t.Fatalf("fixture: want the first spawn to hold the lower handle (%v, %v)", first, second)
	}
	rat := s.spawnNpc("npc-rat-scuttler", belt, 0)
	if rat.IsZero() {
		t.Fatal("rat spawn failed")
	}
	ai, _ := s.AIs.Get(rat)
	def, _ := s.Content.Npc(ai.Npc)
	threat := func() ecs.Entity {
		return s.npcThreat(rat, mustPos(s, rat), ai, def, false, s.npcCandidates(), 0, math.MaxFloat64, false)
	}
	if got := threat(); got != first {
		t.Fatalf("equidistant tie = %v, want the canonical lower handle %v", got, first)
	}
	teleport(s, second, belt.Add(v(500, 0)))
	if got := threat(); got != second {
		t.Fatalf("nearer candidate = %v, want %v", got, second)
	}
}

// TestNpcThreatDamageSourceStillWins: preference never delays initial aggro —
// the hull that just hit me is answered ahead of the ranking.
func TestNpcThreatDamageSourceStillWins(t *testing.T) {
	s := newTestSim(t)
	belt := mustPos(s, s.celEntity("cel-belt-a"))
	spawnMiner(t, s, belt) // the fattest candidate, and the nearest
	setPlayerPos(s, belt.Add(v(3000, 0)))
	rat := s.spawnNpc("npc-rat-scuttler", belt.Add(v(2000, 0)), 0)
	if rat.IsZero() {
		t.Fatal("rat spawn failed")
	}
	ai, _ := s.AIs.Get(rat)
	def, _ := s.Content.Npc(ai.Npc)
	ai.LastHitBy = s.player
	if got := s.npcThreat(rat, mustPos(s, rat), ai, def, true, s.npcCandidates(), 0, math.MaxFloat64, false); got != s.player {
		t.Fatalf("target = %v, want the damage source %v", got, s.player)
	}
}

// ---------------------------------------------------------------------------
// The relative-lead switch (engaged NPCs).
//
// RED (quoted from the pre-change dispatch): the approach/orbit case never
// re-ran the threat scan — an engaged hull could not be pulled off its target
// by damage at all:
//
//	case NpcApproach, NpcOrbit:
//		if !targetOnGrid { ... }
//		if structFrac < effFlee { ... }
//		if ai.Home.Dist(tPos) > leash { ... }
//		// ... intent only: no npcThreat call here
// ---------------------------------------------------------------------------

// switchFixture builds a rat engaged with the fat target A and a hostile
// attacker B (the player) standing by — every hit in these tests is injected,
// so B's own weapons never matter. It returns the handles.
func switchFixture(t *testing.T) (s *Sim, rat, a, b ecs.Entity) {
	t.Helper()
	s = newTestSim(t)
	belt := mustPos(s, s.celEntity("cel-belt-a"))
	a = spawnMiner(t, s, belt.Add(v(500, 0)))
	// A target that survives the dwell: the rat's guns would otherwise kill
	// the barge before the rule could be observed.
	if ship, ok := s.Ships.Get(a); ok {
		ship.Shield, ship.Armor, ship.Structure = 1e6, 1e6, 1e6
		ship.MaxShield, ship.MaxArmor, ship.MaxStructure = 1e6, 1e6, 1e6
	}
	b = s.player
	setPlayerPos(s, belt.Add(v(4000, 0)))
	rat = s.spawnNpc("npc-rat-scuttler", belt.Add(v(2000, 0)), 0)
	if rat.IsZero() {
		t.Fatal("rat spawn failed")
	}
	// Both hulls must survive the observation window: the dwell alone is
	// 600 ticks of injected fire.
	if ship, ok := s.Ships.Get(rat); ok {
		ship.Shield, ship.Armor, ship.Structure = 1e6, 1e6, 1e6
		ship.MaxShield, ship.MaxArmor, ship.MaxStructure = 1e6, 1e6, 1e6
	}
	ai, _ := s.AIs.Get(rat)
	ai.State = NpcApproach
	ai.Target = a
	ai.SwitchAt = s.ticks
	drain(s)
	return s, rat, a, b
}

// driveSwitchDamage lands `aDmg` from A and `bDmg` from B every 30 ticks for
// `ticks` ticks, returning the tick at which the rat's target changed (0 if
// it never did).
func driveSwitchDamage(s *Sim, rat, a, b ecs.Entity, aDmg, bDmg content.Damage, ticks int) int64 {
	var switched int64
	for i := 1; i <= ticks; i++ {
		if i%30 == 0 {
			if aDmg != (content.Damage{}) {
				damageNPC(s, a, rat, aDmg, false)
			}
			if bDmg != (content.Damage{}) {
				damageNPC(s, b, rat, bDmg, false)
			}
		}
		s.Step()
		drain(s)
		if ai, ok := s.AIs.Get(rat); ok && ai.Target != a && switched == 0 {
			switched = s.ticks
		}
	}
	return switched
}

// TestEngagedNpcSwitchesOnConsiderableThreat: B leads A by 3x inside the
// window, and once the dwell passes the rat takes B.
func TestEngagedNpcSwitchesOnConsiderableThreat(t *testing.T) {
	s, rat, a, b := switchFixture(t)
	switched := driveSwitchDamage(s, rat, a, b, scratchDamage(), content.Damage{EM: 150}, 700)
	if switched == 0 || switched < npcSwitchDwell {
		t.Fatalf("switch at tick %d, want it after the %d-tick dwell", switched, npcSwitchDwell)
	}
	ai, _ := s.AIs.Get(rat)
	if ai.Target != b || ai.State != NpcApproach {
		t.Fatalf("state=%q target=%v, want approach on the leading attacker %v", ai.State, ai.Target, b)
	}
}

// TestEngagedNpcHoldsThroughLightPoke: a light poke never pulls an engaged
// hull off the target that is hurting it more.
func TestEngagedNpcHoldsThroughLightPoke(t *testing.T) {
	s, rat, a, b := switchFixture(t)
	switched := driveSwitchDamage(s, rat, a, b, content.Damage{EM: 150}, scratchDamage(), 900)
	if switched != 0 {
		t.Fatalf("switched at tick %d on a light poke; want the engagement held", switched)
	}
	ai, ok := s.AIs.Get(rat)
	if !ok || ai.Target != a {
		t.Fatalf("target = %v, want the original target %v", ai, a)
	}
}

// TestSwitchLedgerIsTransient pins the documented restore consequence: the
// damage window and dwell marker are not saved, so a restored hull needs
// fresh damage before it can switch (lastPlayerWork's class of reset).
func TestSwitchLedgerIsTransient(t *testing.T) {
	s, rat, _, _ := switchFixture(t)
	damageNPC(s, 0, rat, content.Damage{EM: 50}, false)
	s.Step()
	ai, _ := s.AIs.Get(rat)
	if ai.hitSum(s.ticks, 0) != 0 && len(ai.HitLog) == 0 {
		t.Fatal("fixture: no damage landed")
	}
	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	c, err := New(testContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatalf("fresh sim: %v", err)
	}
	if err := c.Restore(save); err != nil {
		t.Fatalf("restore: %v", err)
	}
	ra, _ := c.AIs.Get(rat)
	if ra.SwitchAt != 0 || ra.HitHead != 0 {
		t.Fatalf("restored switch state = at %d head %d, want the transient ledger cleared", ra.SwitchAt, ra.HitHead)
	}
	for i := range ra.HitLog {
		if ra.HitLog[i] != (npcHitEvent{}) {
			t.Fatalf("restored hit log entry %d = %+v, want empty", i, ra.HitLog[i])
		}
	}
}

// ---------------------------------------------------------------------------
// The engaged-idle guard (the old last-hit flip, replaced).
//
// RED (quoted from the pre-change npcThreat): trigger (b) returned the
// attacker unconditionally, so any fresh hit flipped an engaged guard the
// same tick it landed:
//
//	hit := damaged && ai.LastHitBy == c.e && c.faction != mine
//	...
//	if damaged && ai.LastHitBy == c.e { return c.e } // (b) beats every ambient trigger
// ---------------------------------------------------------------------------

// TestEngagedGuardIgnoresLightPoke: a guard clearing a rat stays on the rat
// while the rat is hurting it more than a light player poke.
func TestEngagedGuardIgnoresLightPoke(t *testing.T) {
	s := newTrafficSim(t)
	belt := beltPosOf(t, s, "cel-belt-a")
	guard := spawnStaticGuard(t, s, belt)
	rat := s.spawnNpc("npc-rat-scuttler", belt, 100)
	if rat.IsZero() {
		t.Fatal("rat spawn failed")
	}
	setPlayerPos(s, v(-5e6, -5e6))
	drain(s)
	for i := 0; i < 5; i++ {
		s.Step()
	}
	drain(s)
	// The guard has held the rat long enough for the dwell to pass; the rat
	// has been shooting back, the player's poke is small.
	ga, _ := s.AIs.Get(guard)
	ga.SwitchAt = s.ticks - npcSwitchDwell - 1
	damageNPC(s, rat, guard, content.Damage{EM: 150}, false)
	damageNPC(s, s.player, guard, scratchDamage(), false)
	s.Step()
	drain(s)
	ga, _ = s.AIs.Get(guard)
	if ga.State != NpcIdle || ga.Target == s.player {
		t.Fatalf("guard state=%q target=%v after a light poke; want it idle on the rat", ga.State, ga.Target)
	}
}

// TestEngagedGuardSwitchesOnConsiderableThreat: the same guard flips to the
// player once the player is the considerable threat (2x the rat's damage).
func TestEngagedGuardSwitchesOnConsiderableThreat(t *testing.T) {
	s := newTrafficSim(t)
	belt := beltPosOf(t, s, "cel-belt-a")
	guard := spawnStaticGuard(t, s, belt)
	rat := s.spawnNpc("npc-rat-scuttler", belt, 100)
	if rat.IsZero() {
		t.Fatal("rat spawn failed")
	}
	setPlayerPos(s, v(-5e6, -5e6))
	drain(s)
	for i := 0; i < 5; i++ {
		s.Step()
	}
	drain(s)
	ga, _ := s.AIs.Get(guard)
	ga.SwitchAt = s.ticks - npcSwitchDwell - 1
	damageNPC(s, rat, guard, scratchDamage(), false)
	damageNPC(s, s.player, guard, content.Damage{EM: 500}, false)
	s.Step()
	drain(s)
	ga, _ = s.AIs.Get(guard)
	if ga.State != NpcApproach || ga.Target != s.player {
		t.Fatalf("guard state=%q target=%v, want approach on the player", ga.State, ga.Target)
	}
}

// ---------------------------------------------------------------------------
// Trace: read-only, and a traced run must hash identically to its twin.
// ---------------------------------------------------------------------------

// TestNpcTraceDoesNotChangeTheSim: the observer prints, never decides.
func TestNpcTraceDoesNotChangeTheSim(t *testing.T) {
	newRun := func() *Sim {
		s := newTestSim(t)
		belt := mustPos(s, s.celEntity("cel-belt-a"))
		spawnMiner(t, s, belt)
		rat := s.spawnNpc("npc-rat-scuttler", belt.Add(v(1000, 0)), 100)
		if rat.IsZero() {
			t.Fatal("rat spawn failed")
		}
		setPlayerPos(s, v(-5e6, -5e6))
		drain(s)
		return s
	}
	a := newRun()
	b := newRun()
	var lines []string
	b.TraceNpc("Rat Scuttler", func(line string) { lines = append(lines, line) })
	for i := 0; i < 300; i++ {
		a.Step()
		b.Step()
		drain(a)
		drain(b)
	}
	if a.Hash() != b.Hash() {
		t.Fatalf("traced run diverged: traced=%016x plain=%016x", b.Hash(), a.Hash())
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"npc-trace tick=", "role=aggressive", "branch=", " select ", " switch "} {
		if !strings.Contains(joined, want) {
			t.Fatalf("trace missing %q:\n%s", want, joined)
		}
	}
}

// ---------------------------------------------------------------------------
// Structural: each role's tree handles the standard fixture on its documented
// branch (the shape test the wave asks for, per role).
// ---------------------------------------------------------------------------

func TestRoleTreeBranches(t *testing.T) {
	// tickRole evaluates one role tree for e and returns the context.
	tickRole := func(s *Sim, tree *btNode, e ecs.Entity, role string) btCtx {
		t.Helper()
		ai, _ := s.AIs.Get(e)
		ship, _ := s.Ships.Get(e)
		tr, _ := s.Transforms.Get(e)
		def, _ := s.Content.Npc(ai.Npc)
		sec := s.EffectiveSecurity(s.System().ID)
		var c btCtx
		c.init(s, e, ai, ship, tr, def, sec, s.npcCandidates())
		c.role = role
		tree.Tick(&c)
		return c
	}

	t.Run("aggressive idle engages the intruder", func(t *testing.T) {
		s := newTestSim(t)
		belt := mustPos(s, s.celEntity("cel-belt-a"))
		miner := spawnMiner(t, s, belt)
		rat := s.spawnNpc("npc-rat-scuttler", belt.Add(v(1000, 0)), 0)
		setPlayerPos(s, v(-5e6, -5e6))
		c := tickRole(s, &aggressiveTree, rat, "aggressive")
		if c.branch != "idle" || c.ai.State != NpcApproach || c.ai.Target != miner {
			t.Fatalf("branch=%q state=%q target=%v, want idle -> approach on the miner", c.branch, c.ai.State, c.ai.Target)
		}
	})

	t.Run("aggressive engaged breaks off a lost target", func(t *testing.T) {
		s := newTestSim(t)
		belt := mustPos(s, s.celEntity("cel-belt-a"))
		miner := spawnMiner(t, s, belt)
		rat := s.spawnNpc("npc-rat-scuttler", belt.Add(v(1000, 0)), 0)
		ai, _ := s.AIs.Get(rat)
		ai.State, ai.Target = NpcOrbit, miner
		s.destroy(miner)
		setPlayerPos(s, v(-5e6, -5e6))
		c := tickRole(s, &aggressiveTree, rat, "aggressive")
		if c.branch != "engaged" || c.ai.State != NpcReturn {
			t.Fatalf("branch=%q state=%q, want engaged -> return", c.branch, c.ai.State)
		}
	})

	t.Run("miner mines", func(t *testing.T) {
		s := newTestSim(t)
		miner := spawnMiner(t, s, mustPos(s, s.celEntity("cel-belt-a")))
		c := tickRole(s, &minerTree, miner, "miner")
		if c.branch != "mine" || c.ai.State != NpcMine {
			t.Fatalf("branch=%q state=%q, want mine", c.branch, c.ai.State)
		}
	})

	t.Run("guard clears the anchored rat", func(t *testing.T) {
		c := trafficContent()
		c.Traffic["sys-a"].Patrol = nil
		s := newBeltSim(t, c, 7)
		belt := beltPosOf(t, s, "cel-belt-a")
		guard := spawnStaticGuard(t, s, belt)
		rat := s.spawnNpc("npc-rat-scuttler", belt, 100)
		setPlayerPos(s, v(-5e6, -5e6))
		ctx := tickRole(s, &aggressiveTree, guard, "aggressive")
		if ctx.branch != "idle" || ctx.ai.State != NpcIdle {
			t.Fatalf("branch=%q state=%q, want the idle guard branch", ctx.branch, ctx.ai.State)
		}
		if tg, ok := s.Targetings.Get(guard); !ok || len(tg.Locks) == 0 || tg.Locks[0].Target != rat {
			t.Fatalf("guard locks = %+v, want the anchored rat %v", tg, rat)
		}
	})

	t.Run("patrol responds", func(t *testing.T) {
		s := newBeltSim(t, trafficContent(), 7)
		var e ecs.Entity
		for i := 0; i < 600 && e.IsZero(); i++ {
			s.Step()
			drain(s)
			for _, id := range sortedEntities(s.Patrols) {
				e = id
				break
			}
		}
		if e.IsZero() {
			t.Fatal("fixture: no patrol")
		}
		p, _ := s.Patrols.Get(e)
		p.State, p.Incident = PatrolRespond, 0
		ai, _ := s.AIs.Get(e)
		ai.State = NpcIdle
		var c btCtx
		c = btCtx{s: s, e: e, ai: ai, p: p, security: "high"}
		patrolTree.Tick(&c)
		if c.branch != "respond" || p.State != PatrolReturn {
			t.Fatalf("branch=%q patrol=%q, want respond -> return (no live incident)", c.branch, p.State)
		}
	})

	t.Run("raider engaged with the player", func(t *testing.T) {
		s := newTrafficSim(t)
		rat := s.spawnNpc("npc-rat-scuttler", v(1e7, 1e7), 0)
		ai, _ := s.AIs.Get(rat)
		ai.State, ai.Target = NpcApproach, s.player
		var c btCtx
		c = btCtx{s: s, e: rat, ai: ai, atk: &TrafficAttack{}, hauler: 0}
		raiderTree.Tick(&c)
		if c.branch != "engaged" {
			t.Fatalf("branch=%q, want the engaged branch", c.branch)
		}
	})
}
