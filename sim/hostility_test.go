package sim

import (
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
)

// ---------------------------------------------------------------------------
// Inter-NPC hostility: the threat machine is candidate-based, so the player
// and NPCs aggro through the same four triggers, gated by FACTIONS.md's
// Relations table (sim/hostility.go). Role passivity stacks on top: guards
// never initiate, miners never initiate and never flee.
// ---------------------------------------------------------------------------

// TestRatAttacksWorkingMiner is the reported gap: an Old Works rat and a
// Dvužek miner cutting rock inside the rat's 30 km work radius, no player —
// the rat's recent-work trigger (c) fires on the mining op exactly as it does
// on the player's frozen work marker.
func TestRatAttacksWorkingMiner(t *testing.T) {
	s := newTestSim(t)
	beltPos := mustPos(s, s.celEntity("cel-belt-a"))
	miner := spawnMiner(t, s, beltPos)
	// 10 km from the miner: beyond the high-sec 6 km aggro range and the
	// 8 km intrusion band, inside the 30 km work radius.
	rat := s.spawnNpc("npc-rat-scuttler", beltPos.Add(v(10000, 0)), 100)
	if rat.IsZero() {
		t.Fatal("rat spawn failed")
	}
	setPlayerPos(s, v(-5e6, -5e6))
	drain(s)

	working := false
	var evs []Event
	for i := 0; i < 600; i++ {
		s.Step()
		evs = append(evs, drain(s)...)
		if s.npcWorking(miner) {
			working = true
		}
	}
	if !working {
		t.Fatal("fixture: the miner never ran a mining cycle")
	}
	if ai, _ := s.AIs.Get(rat); ai.State != NpcApproach || ai.Target != miner {
		t.Fatalf("rat state=%q target=%v want approach on the working miner %v",
			ai.State, ai.Target, miner)
	}
	if findEvent(evs, EvLog, "Scuttler attacks Fixture Miner") == nil {
		t.Fatalf("missing the NPC-vs-NPC aggro line: %v", evs)
	}
}

// TestRatAggrosMinerInProximity pins trigger (a) for an NPC candidate: a
// miner inside the rat's effective aggro range is noticed even though it is
// not working and the player is nowhere near.
func TestRatAggrosMinerInProximity(t *testing.T) {
	s := newTestSim(t)
	s.Content.Systems["sys-a"].Security = "low" // effAggro = 15 km, flee 0.25
	beltPos := mustPos(s, s.celEntity("cel-belt-a"))
	// Emptied belt: the miner has nothing to cut, so only proximity can
	// trigger (no work marker, no live laser).
	emptyBelt(t, s)
	miner := spawnMiner(t, s, beltPos)
	rat := s.spawnNpc("npc-rat-scuttler", beltPos.Add(v(12000, 0)), 100)
	if rat.IsZero() {
		t.Fatal("rat spawn failed")
	}
	// Home pushed out so the 8 km intrusion band cannot fire: only (a).
	if ai, _ := s.AIs.Get(rat); true {
		ai.Home = beltPos.Add(v(30000, 0))
	}
	setPlayerPos(s, v(-5e6, -5e6))
	drain(s)

	s.Step()
	if ai, _ := s.AIs.Get(rat); ai.State != NpcApproach || ai.Target != miner {
		t.Fatalf("rat state=%q target=%v want approach on the miner %v (proximity)",
			ai.State, ai.Target, miner)
	}
}

// TestHostilityTable pins FACTIONS.md's Relations table as code: the Old
// Works raid every non-Old-Works hull, the Charter answers the Old Works
// only, and everything else — corporate houses, independents, unknown
// factions — is neutral (fail closed, never hostile by default). It also
// fails when shipped content carries a faction with no relations row, so the
// table cannot silently lag the data.
func TestHostilityTable(t *testing.T) {
	cases := []struct {
		from, to string
		want     bool
	}{
		{factionOldWorks, factionCharter, true},
		{factionOldWorks, factionDvuzek, true},
		{factionOldWorks, factionIndependent, true},
		{factionOldWorks, factionOldWorks, false},
		{factionCharter, factionOldWorks, true},
		{factionCharter, factionDvuzek, false},
		{factionCharter, factionIndependent, false},
		{factionCharter, factionCharter, false},
		{factionDvuzek, factionOldWorks, false},
		{factionDvuzek, factionCharter, false},
		{factionDvuzek, factionIndependent, false},
		{factionIndependent, factionOldWorks, false},
		{factionIndependent, factionCharter, false},
		{factionIndependent, factionDvuzek, false},
		// Unknown factions fail closed in both directions.
		{"Freeports", factionOldWorks, false},
		{"Freeports", factionIndependent, false},
		{factionOldWorks, "Freeports", false},
		{"", factionOldWorks, false},
		{factionOldWorks, "", false},
	}
	for _, c := range cases {
		if got := hostile(c.from, c.to); got != c.want {
			t.Errorf("hostile(%q, %q) = %v, want %v", c.from, c.to, got, c.want)
		}
	}

	// Every shipped NPC def's faction has a relations row.
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("load content: %v", err)
	}
	if len(c.NPCs) == 0 {
		t.Fatal("no shipped NPC defs loaded")
	}
	for id, def := range c.NPCs {
		if _, ok := hostility[def.Faction]; !ok {
			t.Errorf("%s ships faction %q with no relations row (sim/hostility.go)", id, def.Faction)
		}
	}
}

// TestRatDoesNotTurnOnItsOwn pins same-faction neutrality: neither proximity
// nor damage from another Old Works hull flips a rat.
func TestRatDoesNotTurnOnItsOwn(t *testing.T) {
	s := newTestSim(t)
	s.Content.Systems["sys-a"].Security = "low"
	a := s.spawnNpc("npc-rat-scuttler", v(1e7, 60000), 100)
	b := s.spawnNpc("npc-rat-scuttler", v(1e7+5000, 60000), 100)
	if a.IsZero() || b.IsZero() {
		t.Fatal("rat spawn failed")
	}
	setPlayerPos(s, v(-5e6, -5e6))
	drain(s)

	damageNPC(s, a, b, scratchDamage(), false)
	s.Step()
	evs := drain(s)
	aiB, _ := s.AIs.Get(b)
	if aiB.State != NpcIdle {
		t.Fatalf("rat state=%q want idle: same-faction damage flipped it", aiB.State)
	}
	if hasEvent(evs, EvLog, "attacks") {
		t.Fatalf("same-faction aggro line emitted: %v", evs)
	}
	if infoHostile(s, b) {
		t.Fatal("an untouched rat must not read hostile")
	}
}

// TestMinerNeverTurnsOnAnything pins the miner's role passivity beyond the
// player: a miner damaged by the rat already attacking it keeps cutting and
// never enters the machine (no lock, no aggro state).
func TestMinerNeverTurnsOnAnything(t *testing.T) {
	s := newTestSim(t)
	beltPos := mustPos(s, s.celEntity("cel-belt-a"))
	miner := spawnMiner(t, s, beltPos)
	rat := s.spawnNpc("npc-rat-scuttler", beltPos.Add(v(4000, 0)), 100)
	if rat.IsZero() {
		t.Fatal("rat spawn failed")
	}
	setPlayerPos(s, v(-5e6, -5e6))
	drain(s)

	stepN(s, 5)
	drain(s)
	damageNPC(s, rat, miner, scratchDamage(), false)
	for i := 0; i < 600; i++ {
		s.Step()
		drain(s)
		ai, ok := s.AIs.Get(miner)
		if !ok {
			return // killed by the rat: still passive
		}
		if ai.State != NpcMine && ai.State != NpcHaul {
			t.Fatalf("miner state=%q want mine/haul (never a combat state)", ai.State)
		}
		if tg, ok := s.Targetings.Get(miner); ok && len(tg.Locks) != 0 {
			t.Fatalf("miner holds %d lock(s), want none", len(tg.Locks))
		}
		if infoHostile(s, miner) {
			t.Fatal("miner reads hostile")
		}
	}
}

// TestRatFightsBackAgainstGuard pins the generalized damage trigger (b): a
// Charter guard's fire turns the rat onto the guard. Before, the trigger was
// hardcoded to `LastHitBy == player`, so a rat shot by a patrol fought
// nobody.
func TestRatFightsBackAgainstGuard(t *testing.T) {
	s := newTrafficSim(t)
	guard := spawnIdleGuard(t, s, v(1e7+5000, 500))
	rat := s.spawnNpc("npc-rat-scuttler", v(1e7+60000, 500), 100)
	if rat.IsZero() {
		t.Fatal("rat spawn failed")
	}
	drain(s)

	damageNPC(s, guard, rat, scratchDamage(), false)
	s.Step()
	evs := drain(s)
	ai, _ := s.AIs.Get(rat)
	if ai.State != NpcApproach || ai.Target != guard {
		t.Fatalf("rat state=%q target=%v want approach on the guard %v", ai.State, ai.Target, guard)
	}
	if findEvent(evs, EvLog, "Rat Scuttler attacks Guard") == nil {
		t.Fatalf("missing the NPC-vs-NPC aggro line: %v", evs)
	}
	if hasEvent(evs, EvLog, "is attacking you") {
		t.Fatalf("the player was named in a guard fight: %v", evs)
	}
	// Guard neutrality holds: it never initiates, and it stays idle.
	if ga, _ := s.AIs.Get(guard); ga.State != NpcIdle {
		t.Fatalf("guard state=%q want idle", ga.State)
	}
}

// TestAggroTreatsPlayerAndNpcTheSame is the core promise: the same fixture,
// the same trigger, whoever the candidate is. A rat notices a pilot 10 km out
// exactly as it notices a miner 10 km out — same transition, same chosen
// target — for the proximity band and for the work band, and only the
// announcement differs (the player's warning vs one line naming the hulls).
func TestAggroTreatsPlayerAndNpcTheSame(t *testing.T) {
	type run struct {
		trigger   string
		player    bool
		logSubstr string
		severity  Severity
	}
	for _, tr := range []string{"proximity", "work"} {
		for _, c := range []run{
			{tr, true, "is attacking you", SevWarning},
			{tr, false, "attacks Fixture Miner", SevInfo},
		} {
			name := tr + "/player"
			if !c.player {
				name = tr + "/miner"
			}
			t.Run(name, func(t *testing.T) {
				s := newTestSim(t)
				beltPos := mustPos(s, s.celEntity("cel-belt-a"))
				// Candidate and Home 10 km apart: inside the low-sec 15 km
				// proximity band (trigger a), outside the 8 km intrusion band
				// (d). High security shrinks proximity to 6 km, leaving only
				// the work trigger (c).
				home := beltPos.Add(v(10000, 0))
				at := beltPos
				s.Content.Systems["sys-a"].Security = "low"
				if tr == "work" {
					s.Content.Systems["sys-a"].Security = "high"
				}
				rat := s.spawnNpc("npc-rat-scuttler", home, 100)
				if rat.IsZero() {
					t.Fatal("rat spawn failed")
				}
				var want ecs.Entity
				if c.player {
					setPlayerPos(s, at)
					want = s.player
					if tr == "work" {
						// Fire a real cycle: the frozen player work marker.
						dummy := dummyTarget(t, s, at.Add(v(3000, 0)), content.Resists{})
						forceLock(s, dummy)
						s.Queue(CmdActivate{Slot: SlotRef{Class: content.SlotHigh, Index: 0}, Target: dummy})
					}
				} else {
					setPlayerPos(s, v(-5e6, -5e6))
					want = spawnMiner(t, s, at)
				}
				drain(s)

				// Step until the trigger's own signal exists (the shot fired /
				// the laser cut), then one more tick for the scan after it.
				fired := func() bool {
					if tr == "proximity" {
						return true
					}
					if c.player {
						return s.lastPlayerWork.at >= 0
					}
					return s.npcWorking(want)
				}
				var evs []Event
				for i := 0; i < 600; i++ {
					s.Step()
					evs = append(evs, drain(s)...)
					if fired() {
						break
					}
				}
				if !fired() {
					t.Fatalf("fixture: the %s trigger never fired for %s", tr, name)
				}
				// One more tick: an NPC's laser can switch on after the
				// rat's own turn in the same systemNpc pass.
				s.Step()
				evs = append(evs, drain(s)...)
				if ai, _ := s.AIs.Get(rat); ai.State != NpcApproach {
					t.Fatalf("state=%q want approach (the same trigger fired for %s)", ai.State, name)
				}
				if ai, _ := s.AIs.Get(rat); ai.Target != want {
					t.Fatalf("target=%v want %v", ai.Target, want)
				}
				ev := findEvent(evs, EvLog, c.logSubstr)
				if ev == nil {
					t.Fatalf("missing aggro line %q: %v", c.logSubstr, evs)
				}
				if ev.Severity != c.severity {
					t.Fatalf("aggro severity=%d want %d", ev.Severity, c.severity)
				}
			})
		}
	}
}

// TestHighSecGuardClearsRatAttackingMiner pins the security chain: a rat
// attacking a mining op in a high-sec belt is just a rat at the static
// guard's celestial, so the ambient rule engages it — no new guard behavior,
// just observable coverage of the miner's fight.
func TestHighSecGuardClearsRatAttackingMiner(t *testing.T) {
	c := trafficContent()
	c.Traffic["sys-a"].Patrol = nil // isolate the static guard rule
	c.NPCs["npc-rat-scuttler"].Fit = []content.ModuleID{"mod-test-turret"}
	s := newBeltSim(t, c, 7)
	belt := beltPosOf(t, s, "cel-belt-a")
	guard := spawnStaticGuard(t, s, belt)
	// The guard stands off its anchor (Home unchanged): the rat picks the
	// miner working at the belt, and the guard answers the rat. The rat is
	// anchored AT the celestial (its Home must equal the guard's, the belt
	// rule's exact match) while its hull scatters. 12 km also keeps the
	// guard outside the rat's 6 km high-sec proximity trigger: prey
	// preference ranks cargo against guns, so a guard inside that band would
	// out-score the unarmed barge (the rat would answer the warship first,
	// which is the policy, not the chain this test proves).
	teleport(s, guard, belt.Add(v(12000, 0)))
	spawnMiner(t, s, belt)
	rat := s.spawnNpc("npc-rat-scuttler", belt, 1000)
	if rat.IsZero() {
		t.Fatal("rat spawn failed")
	}
	setPlayerPos(s, v(-5e6, -5e6))
	drain(s)

	var evs []Event
	minerAttacked := false
	for i := 0; i < 6000; i++ {
		s.Step()
		for _, ev := range drain(s) {
			evs = append(evs, ev)
			if ev.Kind == EvLog && hasText(ev, "attacks Fixture Miner") && ev.Actor == rat {
				minerAttacked = true
			}
		}
		if minerAttacked && hasEvent(evs, EvLog, "hits Rat Scuttler") {
			break
		}
	}
	if !minerAttacked {
		t.Fatal("the rat never attacked the mining op")
	}
	if !hasEvent(evs, EvLog, "hits Rat Scuttler") {
		t.Fatalf("the high-sec guard never engaged the rat attacking the miner: %v", evs)
	}
}

// TestNpcNpcEngagementSaveRoundTrip pins the persisted target handle: an
// in-flight rat-vs-miner engagement survives Snapshot/Restore bit-identically
// and both runs stay in lockstep afterwards.
func TestNpcNpcEngagementSaveRoundTrip(t *testing.T) {
	newRun := func() (*Sim, [2]ecs.Entity) {
		s := newTestSim(t)
		beltPos := mustPos(s, s.celEntity("cel-belt-a"))
		miner := spawnMiner(t, s, beltPos)
		rat := s.spawnNpc("npc-rat-scuttler", beltPos.Add(v(4000, 0)), 100)
		if rat.IsZero() {
			t.Fatalf("rat spawn failed")
		}
		setPlayerPos(s, v(-5e6, -5e6))
		drain(s)
		return s, [2]ecs.Entity{rat, miner}
	}
	a, pair := newRun()
	b, _ := newRun()
	rat, miner := pair[0], pair[1]

	// Drive the engagement into flight, then snapshot.
	for i := 0; i < 300; i++ {
		a.Step()
		b.Step()
		drain(a)
		drain(b)
	}
	aiA, _ := a.AIs.Get(rat)
	if aiA.State != NpcApproach && aiA.State != NpcOrbit {
		t.Fatalf("fixture: rat state=%q want an engagement at the save point", aiA.State)
	}
	if aiA.Target != miner {
		t.Fatalf("fixture: rat target=%v want the miner %v", aiA.Target, miner)
	}
	save, err := a.Snapshot()
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
	aiC, ok := c.AIs.Get(rat)
	if !ok || aiC.State != aiA.State || aiC.Target != aiA.Target {
		t.Fatalf("restored AI=%+v want state=%q target=%v", aiC, aiA.State, aiA.Target)
	}
	if c.Hash() != a.Hash() {
		t.Fatalf("restored hash=%016x want %016x", c.Hash(), a.Hash())
	}
	for i := 0; i < 600; i++ {
		a.Step()
		b.Step()
		c.Step()
		drain(a)
		drain(b)
		drain(c)
	}
	if a.Hash() != b.Hash() || a.Hash() != c.Hash() {
		t.Fatalf("post-restore divergence: live=%016x twin=%016x restored=%016x",
			a.Hash(), b.Hash(), c.Hash())
	}
}
