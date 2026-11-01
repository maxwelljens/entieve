package sim

import (
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
)

// ---------------------------------------------------------------------------
// Friendly fire: the confirm gate, the escalation to outlaw, and the two
// places the record changes behaviour (guard acquisition, distress wings).
// ---------------------------------------------------------------------------

// friendlyFixture places a civilian miner in weapon range of the player and
// grants the lock the gun needs, then returns the miner and the world-fit
// weapon slot.
func friendlyFixture(t *testing.T, s *Sim) (ecs.Entity, SlotRef) {
	t.Helper()
	miner := spawnMiner(t, s, playerPos(s).Add(v(2000, 0)))
	lockInstantly(t, s, miner)
	return miner, SlotRef{Class: content.SlotHigh, Index: 0}
}

// TestFriendlyFireRefusedWithoutConfirm is the red-first pin: an aggressive
// activation on a friendly hull without Confirm emits the prompt, activates
// nothing, damages nothing and stamps no aggression.
func TestFriendlyFireRefusedWithoutConfirm(t *testing.T) {
	s := newTestSim(t)
	miner, slot := friendlyFixture(t, s)
	def, _, ok := s.moduleDef(s.player, slot)
	if !ok {
		t.Fatal("fixture: no weapon in high slot 0")
	}
	shield := mustShield(t, s, miner)
	drain(s)

	s.Queue(CmdActivate{Slot: slot, Target: miner})
	s.Step()
	evs := drain(s)

	prompt := findEvent(evs, EvFriendlyFire, "Friendly fire")
	if prompt == nil {
		t.Fatalf("no friendly-fire prompt: %v", evs)
	}
	if prompt.Slot != slot || prompt.Target != miner || prompt.Name != def.Name {
		t.Fatalf("prompt={slot:%+v target:%v name:%q}, want {%+v %v %q}",
			prompt.Slot, prompt.Target, prompt.Name, slot, miner, def.Name)
	}
	if rt, ok := hudModule(s, slot); !ok || rt.Active {
		t.Fatal("the module activated without confirmation")
	}
	if hasEvent(evs, EvModule, "activated") {
		t.Fatalf("refused activation still emitted EvModule: %v", evs)
	}
	if got := mustShield(t, s, miner); got != shield {
		t.Fatalf("miner shield=%v want %v (nothing fired)", got, shield)
	}
	if (s.aggression != playerAggression{}) {
		t.Fatalf("refusal stamped aggression: %+v", s.aggression)
	}
}

// TestFriendlyFireConfirmedActivates: Confirm=true fires the same command
// and starts the record.
func TestFriendlyFireConfirmedActivates(t *testing.T) {
	s := newTestSim(t)
	miner, slot := friendlyFixture(t, s)
	drain(s)

	s.Queue(CmdActivate{Slot: slot, Target: miner, Confirm: true})
	s.Step()
	evs := drain(s)

	if !hasEvent(evs, EvModule, "activated") {
		t.Fatalf("confirmed activation did not fire: %v", evs)
	}
	if hasEvent(evs, EvFriendlyFire, "") {
		t.Fatalf("confirmed activation still prompted: %v", evs)
	}
	if s.aggression.until == 0 || s.aggression.first != s.aggression.until-aggressionWindowSec {
		t.Fatalf("aggression=%+v, want a fresh run stamped now", s.aggression)
	}
	if s.playerOutlaw() {
		t.Fatal("one confirmed act already reads outlaw")
	}
}

// TestFriendlyFireNoPromptOnHostile: a rat is a legitimate target, no
// confirmation, no record.
func TestFriendlyFireNoPromptOnHostile(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	forceLock(s, npc)
	slot := SlotRef{Class: content.SlotHigh, Index: 0}
	drain(s)

	s.Queue(CmdActivate{Slot: slot, Target: npc})
	s.Step()
	evs := drain(s)

	if hasEvent(evs, EvFriendlyFire, "") {
		t.Fatalf("a hostile rat prompted: %v", evs)
	}
	if !hasEvent(evs, EvModule, "activated") {
		t.Fatalf("the rat activation did not fire: %v", evs)
	}
	if (s.aggression != playerAggression{}) {
		t.Fatalf("attacking a rat stamped aggression: %+v", s.aggression)
	}
}

// TestOutlawEscalationAndExpiry pins the escalation curve: a confirmed act
// starts the run, a later confirmed hit spans the persistence window and
// raises the warrant once, and a lapsed window reads clean again.
func TestOutlawEscalationAndExpiry(t *testing.T) {
	s := newTestSim(t)
	miner, slot := friendlyFixture(t, s)

	s.Queue(CmdActivate{Slot: slot, Target: miner, Confirm: true})
	s.Step()
	drain(s)
	if s.playerOutlaw() {
		t.Fatal("outlaw on the first confirmed act")
	}
	first := s.aggression.first

	// The same run, one persistence window later: a hit on a friendly hull
	// (no second dialog) slides the record and spans the threshold.
	stepN(s, int(aggressionPersistSec/DT)+60)
	next := spawnMiner(t, s, playerPos(s).Add(v(2500, 0)))
	damageNPC(s, s.player, next, content.Damage{EM: 5}, false)
	s.Step()
	evs := drain(s)
	if !s.playerOutlaw() {
		t.Fatalf("no warrant after the acts spanned %.0f s (aggression=%+v)",
			aggressionPersistSec, s.aggression)
	}
	if s.aggression.first != first {
		t.Fatalf("the run restarted mid-window: first=%v want %v", s.aggression.first, first)
	}
	if n := countKind(evs, EvOutlaw); n != 1 {
		t.Fatalf("EvOutlaw emitted %d times, want exactly 1", n)
	}

	// The window lapses: the warrant clears without erasing the record, so
	// the next act starts a fresh run (span 0).
	s.aggression.until = s.time - 1 // as a quiet window would leave it
	if s.playerOutlaw() {
		t.Fatal("a lapsed window still reads outlaw")
	}
	s.stampAggression()
	if s.aggression.first != s.time || s.playerOutlaw() {
		t.Fatalf("lapsed run restarted wrong: %+v (time %v)", s.aggression, s.time)
	}
}

// TestOutlawResetsOnRestore: the record is transient, like lastPlayerWork.
func TestOutlawResetsOnRestore(t *testing.T) {
	s := newTestSim(t)
	s.aggression = playerAggression{first: s.time - aggressionPersistSec - 1, until: s.time + aggressionWindowSec}
	if !s.playerOutlaw() {
		t.Fatal("fixture: not outlaw")
	}
	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	b, err := New(testContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Restore(save); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if (b.aggression != playerAggression{}) || b.playerOutlaw() {
		t.Fatalf("restored aggression=%+v, want cleared", b.aggression)
	}
}

// TestOutlawSecurityEngagesOnSight: with the warrant live, an idle guard
// acquires the player inside its authored aggro range without being hit.
// The same fixture without the warrant stays neutral (TestGuardPatrolNeverInitiates).
func TestOutlawSecurityEngagesOnSight(t *testing.T) {
	s := newTrafficSim(t)
	guard := spawnIdleGuard(t, s, v(1e7+5000, 500))
	setPlayerPos(s, v(1e7, 500))
	drain(s)

	s.aggression = playerAggression{first: s.time - aggressionPersistSec - 1, until: s.time + aggressionWindowSec}
	for i := 0; i < 5; i++ {
		s.Step()
	}
	ai, _ := s.AIs.Get(guard)
	if ai.State != NpcApproach || ai.Target != s.player {
		t.Fatalf("guard state=%q target=%v, want approach on the outlaw player %v",
			ai.State, ai.Target, s.player)
	}
}

// TestWingEngagesPlayerAggressor is the reported bug: the player shoots a
// miner, the distress wing arrives, and it must fight the player, not warp
// off once no rats are left.
func TestWingEngagesPlayerAggressor(t *testing.T) {
	s := newTrafficSim(t)
	beltPos := mustPos(s, s.celEntity("cel-belt-a"))
	miner := spawnMiner(t, s, beltPos)
	setPlayerPos(s, beltPos.Add(v(2000, 0)))
	// A tanky pilot so the ten-cutter wing cannot end the fixture early.
	if sh, ok := s.Ships.Get(s.player); ok {
		sh.MaxShield, sh.Shield = 1e9, 1e9
		sh.MaxArmor, sh.Armor = 1e9, 1e9
		sh.MaxStructure, sh.Structure = 1e9, 1e9
	}
	drain(s)

	damageNPC(s, s.player, miner, content.Damage{EM: 5}, false)
	s.Step()
	drain(s)
	if s.playerOutlaw() {
		t.Fatal("fixture: one hit is not a persistent aggression")
	}
	// The dispatch records the attacker on every responder (the new save
	// field): that record is what the wing fights.
	recorded := 0
	s.Patrols.Each(func(_ ecs.Entity, p *Patrol) {
		if p.Incident == miner && p.State == PatrolRespond && p.Aggressor == s.player {
			recorded++
		}
	})
	if recorded == 0 {
		t.Fatal("no responder recorded the player as the miner's aggressor")
	}
	// The field rides the save schema (omitempty: an ordinary patrol writes
	// exactly the bytes it did before it existed).
	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	b, err := New(testContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Restore(save); err != nil {
		t.Fatalf("restore: %v", err)
	}
	restored := 0
	b.Patrols.Each(func(_ ecs.Entity, p *Patrol) {
		if p.Aggressor == s.player {
			restored++
		}
	})
	if restored != recorded {
		t.Fatalf("restored aggressors = %d, want %d", restored, recorded)
	}

	engaged := false
	for i := 0; i < 3600 && !engaged; i++ {
		s.Step()
		drain(s)
		s.Patrols.Each(func(pe ecs.Entity, p *Patrol) {
			if p.Incident != miner || p.State != PatrolRespond {
				return
			}
			if tgt, ok := s.modTarget(pe, SlotRef{Class: content.SlotHigh, Index: 0}); ok && tgt == s.player {
				engaged = true
			}
		})
	}
	if !engaged {
		t.Fatal("the distress wing never engaged the player who shot the miner")
	}
}

// TestWingFiresOnLandedOutlaw pins the reported bug: security forces warp
// in, lock the pilot and then turn for the station instead of shooting. With
// the warrant live the landed wing aggros the outlaw through the ordinary
// threat machine, whose leash is measured from the wing's POST (14,000 km
// away here): the very next tick breaks the engagement off into NpcReturn,
// and the patrol tree's "aggroed" leaf then claimed every tick because the
// AI was not idle — so patrolRespondStep never ran again. The hull burned
// home at sublight speed with its lock on the pilot and its guns cold.
// Landed security must hold and fire at the outlaw (the hold-and-fire shape
// patrolMinerStep already uses for its aggressor).
func TestWingFiresOnLandedOutlaw(t *testing.T) {
	s := newTrafficSim(t)
	beltPos := mustPos(s, s.celEntity("cel-belt-a"))
	miner := spawnMiner(t, s, beltPos)
	setPlayerPos(s, beltPos.Add(v(2000, 0)))
	// A tanky pilot: the wing must land hits, not end the fixture early.
	if sh, ok := s.Ships.Get(s.player); ok {
		sh.MaxShield, sh.Shield = 1e9, 1e9
		sh.MaxArmor, sh.Armor = 1e9, 1e9
		sh.MaxStructure, sh.Structure = 1e9, 1e9
	}
	drain(s)
	damageNPC(s, s.player, miner, content.Damage{EM: 5}, false)
	s.Step()
	drain(s)
	// The run of confirmed acts has persisted: the pilot is an outlaw.
	s.aggression = playerAggression{first: s.time - aggressionPersistSec - 1, until: s.time + aggressionWindowSec}
	shield := mustShield(t, s, s.player)

	landed, fired := false, false
	var stalled ecs.Entity // a responder that has landed and is not firing
	for i := 0; i < 3600 && !(landed && fired); i++ {
		s.Step()
		drain(s)
		s.Patrols.Each(func(pe ecs.Entity, p *Patrol) {
			if p.State != PatrolRespond || p.Incident != miner {
				return
			}
			tr, ok := s.Transforms.Get(pe)
			if !ok || tr.Pos.Dist(playerPos(s)) > trafficPatrolEngageRange {
				return
			}
			landed = true
			if rt, ok := responderModule(s, pe); ok && rt.Active {
				fired = true
			}
			if stalled.IsZero() {
				stalled = pe
			}
		})
		if mustShield(t, s, s.player) < shield {
			fired = true
		}
	}
	if !landed {
		t.Fatal("fixture: no responder reached the belt")
	}
	if !fired {
		p, _ := s.Patrols.Get(stalled)
		ai, _ := s.AIs.Get(stalled)
		t.Fatalf("the landed wing never fired on the outlaw (patrol state=%q incident=%v ai=%q target=%v): "+
			"security holds a lock and turns for home instead of shooting", p.State, p.Incident, ai.State, ai.Target)
	}
}

// responderModule is one patrol hull's first high-slot runtime (the cutter's
// turret): the module a landed wing fires the outlaw with.
func responderModule(s *Sim, e ecs.Entity) (ModuleRuntime, bool) {
	fit, ok := s.Fittings.Get(e)
	if !ok {
		return ModuleRuntime{}, false
	}
	if rts := fit.Runtimes(content.SlotHigh); len(rts) > 0 {
		return rts[0], true
	}
	return ModuleRuntime{}, false
}

// countKind counts events of one kind (event phases are not events here).
func countKind(evs []Event, kind EventKind) int {
	n := 0
	for _, ev := range evs {
		if ev.Kind == kind {
			n++
		}
	}
	return n
}

// mustShield returns one ship's shield (fails on a non-ship).
func mustShield(t *testing.T, s *Sim, e ecs.Entity) float64 {
	t.Helper()
	sh, ok := s.Ships.Get(e)
	if !ok {
		t.Fatalf("%v has no ship state", e)
	}
	return sh.Shield
}
