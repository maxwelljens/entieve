package sim

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// ratNpcPrefix is the id prefix of the pirate rat hulls (npc-rat-*): the NPC
// population security forces clear from high-sec belts.
const ratNpcPrefix = "npc-rat-"

// isRatNpc reports whether def is a pirate rat (an Old Works belt/raid hull).
func isRatNpc(def *content.NpcDef) bool {
	return strings.HasPrefix(string(def.ID), ratNpcPrefix)
}

// npcFleeSeconds is how long a fleeing NPC burns away from its target
// before despawning quietly (Intent.Range stores the flee start time —
// unused by manual intents and serialized with the component).
const npcFleeSeconds = 20

// npcReturnRange is the distance (m) at which a returning NPC settles home.
const npcReturnRange = 500

// npcRetryCooldown is the pause (seconds) before an NPC re-attempts a failed
// module activation (insufficient capacitor, no charges, no lock, ...).
const npcRetryCooldown = 3

// npcGuardRange is the radius (m) around an NPC's Home within which an
// interloper is noticed — guardians investigate anyone this close
// (aggro trigger d; guards themselves never initiate, see systemNpc).
const npcGuardRange = 8_000

// npcWorkRange is the radius (m) around an NPC's Home inside which recent
// work — the player's recorded weapon/mining position, an NPC's live mining
// laser — alerts it (aggro trigger c).
const npcWorkRange = 30_000

// npcWorkWindow is how long (sim seconds) recent player work (the frozen
// lastPlayerWork marker) alerts NPC Homes inside npcWorkRange (trigger c).
const npcWorkWindow = 20

// npcLeashFactor multiplies the effective aggro range into the pursuit
// leash: an aggroed NPC engages while its target is within
// leash = npcLeashFactor * effAggro of its Home, and breaks off beyond it.
const npcLeashFactor = 2.5

// npcPatrolSpeedFrac is the idle patrol speed as a fraction of MaxSpeed.
const npcPatrolSpeedFrac = 0.25

// npcPatrolMaxRadius caps the idle patrol radius (m).
const npcPatrolMaxRadius = 2500

// systemNpc advances every NPC state machine:
//
//	idle (patrolling) -> approach -> orbit -> flee -> despawn
//	     \-> return (target beyond the pursuit leash or gone) -> idle
//
// The machine is candidate-based, not player-based: the player and every
// other on-grid ship (AI hulls, traffic haulers — see npcCandidates) are
// evaluated through the same triggers, gated by the faction relations in
// hostile (sim/hostility.go — FACTIONS.md's Relations table). Aggro
// (idle -> approach) fires when any of these triggers holds for a candidate
// the NPC may act on:
//
//	(a) the candidate is within the NPC's effective aggro range;
//	(b) the NPC took damage from the candidate (source-checked: same-faction
//	    fire never flips it, so a patrol's own side or a rat's own swarm
//	    cannot turn it);
//	(c) the player fired a weapon or ran a mining cycle within 30 km of the
//	    NPC's Home in the last 20 sim-seconds (the frozen work marker), or
//	    an NPC candidate is cutting rock with a live mining laser inside
//	    that radius — a mining op draws the same aggro a pilot does;
//	(d) the candidate is within 8 km of the NPC's Home (guardians investigate
//	    interlopers — belt/anomaly guards especially).
//
// When several candidates trigger, the damage source wins, else the nearest,
// with ties broken by canonical entity order. The chosen handle persists in
// NpcAI.Target (SavedNpcAI.Target), so an engagement survives save/restore.
//
// Guards are the exception: security patrols (npc-guard-*, NpcDef.IsGuard)
// are friendly and never initiate on the player — triggers (a), (c) and
// (d) are skipped for them, so flying near a patrol's anchor (or doing
// work there) can never open fire unprovoked. Only trigger (b),
// player-sourced damage, turns a guard hostile (self-defense); its
// incident response (convoy ambushes) is driven by the traffic system and
// is unaffected. Guards do have one threat of their own: in a HIGH-security
// system a static guard clears pirate rats (npc-rat-*) anchored at its own
// celestial whenever they are not already engaged (guardEngageRat) — the
// ambient security rule, which also covers a rat mid-attack on a miner.
// Traffic patrols are exempt from that rule (their belt sweep lives in
// systemTraffic) and low/null space keeps guards passive. Rats they clear
// still fight back through trigger (b) — the rat's machine now answers any
// other-faction attacker, not just the player.
//
// Civilian miners are the second exception (npc-miner-*, NpcDef.IsMiner):
// they never enter this machine as aggressors — npcMiner runs their
// extraction loop and they never initiate a fight (no trigger, no lock). They
// do flee: damage that drops their structure under the temperament-scaled
// flee threshold ends the extraction loop and they burn for the edge for the
// same 20 s, then despawn (npcMiner). They are ordinary candidate targets for
// everyone else: a rat that finds one working in its territory attacks it,
// and the miner runs — or dies at the rock if it cannot get under the
// threshold in time.
//
// The pursuit leash is leash = npcLeashFactor * effAggro measured from the
// NPC's Home: an engaged NPC chases/engages while its target stays inside
// it and breaks off to return when it is crossed; a returning NPC only
// re-engages while a trigger holds AND the target is back inside the leash
// (symmetric with the break-off, so the state machine can never flip-flop
// across the leash line). A fleeing NPC ignores the leash and keeps the
// 20 s burn-away-then-despawn.
//
// Aggro is announced once, on the idle -> approach transition: the player
// line ("<npc> is attacking you") is unchanged, an NPC target gets one
// SevInfo line naming both (npcAggroLog).
//
// SystemDef.Security tunes temperament through npcTemperament:
//
//	"high" — effAggro = AggroRange * 0.4, flee threshold = max(def, 0.35)
//	         (skittish: shoots late, runs early);
//	"low"  — effAggro = AggroRange, thresholds exactly as authored;
//	"null" — effAggro = AggroRange * 1.3, flee threshold 0 (fights to the
//	         death — never flees).
//
// The temperament applies to miners too: their flee threshold is the same
// structure fraction, so a Dvužek barge runs at max(def, 0.35) in high
// security and at its authored def in low (never in null).
//
// Idle NPCs never sit still: they drift on a slow deterministic patrol
// circle around NpcAI.Home (npcPatrol, phase hashed from the entity handle
// — no RNG draws). Aggroed NPCs lock their target through the normal
// targeting system and activate their weapons/tank/ewar modules through the
// normal module system, with npcRetryCooldown gating failed re-attempts.
func (s *Sim) systemNpc() {
	candidates := s.npcCandidates()
	security := ""
	if sys := s.System(); sys != nil {
		security = s.EffectiveSecurity(sys.ID)
	}

	var t btCtx
	for _, e := range sortedEntities(s.AIs) {
		ai, ok := s.AIs.Get(e)
		if !ok {
			continue
		}
		ship, ok := s.Ships.Get(e)
		if !ok {
			continue
		}
		tr, ok := s.Transforms.Get(e)
		if !ok {
			continue
		}
		def, hasDef := s.Content.Npc(ai.Npc)
		if !hasDef {
			continue
		}
		t.init(s, e, ai, ship, tr, def, security, candidates)
		// Civilian miners run their own extraction loop and never enter the
		// threat machine below: they never initiate, and their flee is the
		// miner loop's own (a damaged barge runs). They are ordinary targets
		// for everyone else (npcCandidates).
		if def.IsMiner() {
			t.role = "miner"
			minerTree.Tick(&t)
			t.done()
			continue
		}
		t.role = "aggressive"
		aggressiveTree.Tick(&t)

		// Combat actions: aggroed NPCs lock their target and run their
		// modules. targetOnGrid is the one resolved before the tree ran
		// (it still describes the PREVIOUS target on the tick a fresh aggro
		// picks one): a pre-existing quirk, kept — the observable tick order
		// is the contract.
		if ai.State == NpcApproach || ai.State == NpcOrbit {
			if t.targetOnGrid {
				s.npcEnsureLock(e, ai.Target)
				s.npcActivate(e, ai, ai.Target)
			}
		}
		t.done()
	}
}

// ---------------------------------------------------------------------------
// Behaviour trees
//
// Each role's per-tick policy is one tree below. The leaves are the rules;
// the combinators give them their precedence, so reading the tree top to
// bottom reads the policy. The trees are stateless: every leaf re-derives its
// answer from Sim state, and the decision it writes lives in NpcAI (the
// persisted memory) and the tick's Intent.
// ---------------------------------------------------------------------------

// aggressiveTree is the per-tick policy of every non-miner NPC — rats,
// cutters, war hulls and guards. Guards differ only in leaves (player-only
// self-defence inside npcThreat, the high-sec rat rule in guardTree), never in
// the state machine itself. Branch order mirrors the old dispatch: idle,
// engaged (approach/orbit), flee, return.
var aggressiveTree = btSel("aggressive",
	btSeq("idle", btLeaf("idle-state", leafIdleState),
		btSel("idle-policy",
			btLeaf("threat", leafThreat),
			guardTree)),
	btSeq("engaged", btLeaf("engaged-state", leafEngagedState),
		btSel("engaged-policy",
			btLeaf("target-lost", leafTargetLost),
			btLeaf("structure-low", leafFleeEntry),
			btLeaf("beyond-leash", leafLeashBreak),
			btLeaf("switch-lead", leafSwitch),
			btLeaf("approach", leafApproach),
			btLeaf("orbit", leafOrbit))),
	btSeq("flee", btLeaf("flee-state", leafFleeState), btLeaf("flee-step", leafFleeStep)),
	btSeq("returning", btLeaf("return-state", leafReturnState), btLeaf("return", leafReturn)),
)

// guardTree is a static guard's idle policy: clear the rats anchored at its
// own celestial in high security, then drift the idle patrol circle. A guard
// fighting here stays NpcIdle on purpose (friendly on the overview; the
// traffic gates and Info.Hostile read the state).
var guardTree = btSel("guard",
	btLeaf("clear-rat", leafGuardRat),
	btLeaf("patrol", leafIdlePatrol),
)

// minerTree is the civilian extraction loop of npc-miner-* (NpcDef.IsMiner):
//
//	flee — a damaged hull under its temperament-scaled flee threshold stops
//	       cutting and runs the shared flee (burn away 20 s, despawn,
//	       respawn timer re-armed)
//	haul — warp to the NEAREST STATION of the same system, and on arrival
//	       post the hold's ore on that station's market as system supply,
//	       then despawn (recordRespawn re-arms the spawn group's timer)
//	mine — cut the nearest rock with volume left, moving on when it
//	       depletes; when the hold is full switch to haul
//
// Miners never initiate: they skip the threat machine entirely (no lock, no
// trigger). They do run, though, so a miner caught by rats leaves the belt
// instead of dying at the rock. They are otherwise normal ships — ordinary
// candidate targets for every non-miner NPC, lockable, damageable, and death
// runs the ordinary wreck path (the wreck carries the hold).
var minerTree = btSel("miner",
	btLeaf("run-or-flee", leafMinerFlee),
	btLeaf("haul", leafMinerHaul),
	btLeaf("mine", leafMinerMine),
)

// patrolTree drives one traffic patrol's dispatched states (system_traffic's
// trafficDrivePatrols). The dispatch/scheduling itself stays in the traffic
// system; this tree is the per-tick policy of a patrol already assigned.
var patrolTree = btSel("patrol",
	btLeaf("aggroed", leafPatrolAggroed),
	btLeaf("respond", leafPatrolRespond),
	btLeaf("return", leafPatrolReturn),
	btLeaf("sweep", leafPatrolSweep),
)

// raiderTree is one ambush raider's per-tick policy (trafficRaidersAct). The
// ambush schedule, hauler and break-off stay in the traffic system.
var raiderTree = btSel("raider",
	btLeaf("engaged", leafRaiderEngaged),
	btLeaf("resolved", leafRaiderResolved),
	btLeaf("fight", leafRaiderFight),
)

// btCtx is the caller-owned evaluation context of one NPC tick: the entity
// being decided, its tree's inputs, and the decision the leaves write back
// (ai.State / ai.Target / the tick's Intent). One context is reused across
// entities and roles, so init clears every field a leaf reads.
type btCtx struct {
	s      *Sim
	role   string // trace label: aggressive | miner | patrol | raider
	branch string // top-level branch that handled the tick (trace)

	e        ecs.Entity
	ai       *NpcAI
	tr       *Transform
	def      *content.NpcDef
	security string
	cands    []npcCandidate
	ship     *ShipState

	damaged    bool
	structFrac float64
	effAggro   float64
	effFlee    float64
	leash      float64
	playerOnly bool

	tPos         core.Vec2
	targetOnGrid bool

	// patrol / raider leaves
	p      *Patrol
	sys    *content.SystemDef
	td     *content.TrafficDef
	atk    *TrafficAttack
	hauler ecs.Entity
}

func (t *btCtx) init(s *Sim, e ecs.Entity, ai *NpcAI, ship *ShipState, tr *Transform, def *content.NpcDef, security string, cands []npcCandidate) {
	*t = btCtx{s: s, e: e, ai: ai, ship: ship, tr: tr, def: def, security: security, cands: cands}
	t.effAggro, t.effFlee = npcTemperament(security, def)
	t.leash = npcLeashFactor * t.effAggro
	t.structFrac = 1
	if ship.MaxStructure > 0 {
		t.structFrac = ship.Structure / ship.MaxStructure
	}
	t.damaged = ship.Shield < ship.MaxShield || ship.Armor < ship.MaxArmor ||
		ship.Structure < ship.MaxStructure
	// A live ambusher's target belongs to the traffic incident
	// (trafficRaidersAct) — candidate resolution would hijack its warp-in
	// onto the hauler it already means to take. Only the player pulls it
	// into the machine, exactly as before. (Miners skip the threat machine
	// entirely, so they skip this per-tick traffic scan.)
	if !def.IsMiner() {
		t.playerOnly = s.isTrafficRaider(e)
	}

	// The persisted target, resolved for this tick: a dead or docked target
	// ends the engagement (-> return).
	t.tPos, t.targetOnGrid = s.posOf(ai.Target)
	if t.targetOnGrid && (!s.World.Alive(ai.Target) || s.Docked.Has(ai.Target)) {
		t.targetOnGrid = false
	}
}

// --- aggressive tree leaves ---

func leafIdleState(c *btCtx) btStatus {
	if c.ai.State != NpcIdle && c.ai.State != "" {
		return btFailure
	}
	return btSuccess
}

func leafEngagedState(c *btCtx) btStatus {
	if c.ai.State != NpcApproach && c.ai.State != NpcOrbit {
		return btFailure
	}
	return btSuccess
}

func leafFleeState(c *btCtx) btStatus {
	if c.ai.State != NpcFlee {
		return btFailure
	}
	return btSuccess
}

func leafReturnState(c *btCtx) btStatus {
	if c.ai.State != NpcReturn {
		return btFailure
	}
	return btSuccess
}

// leafThreat is the idle selection: any trigger aggros a fresh NPC from
// wherever its target is (the leash then governs whether the pursuit
// continues). An NPC already fighting while idle — a guard clearing a rat, a
// raider holding an ambush — keeps that fight unless the new attacker is the
// considerable threat the switch rule asks for.
func leafThreat(c *btCtx) btStatus {
	target := c.s.npcThreat(c.e, c.tr.Pos, c.ai, c.def, c.damaged, c.cands, c.effAggro, math.MaxFloat64, c.playerOnly)
	if target.IsZero() {
		return btFailure
	}
	if c.s.npcFlipBlocked(c.e, c.ai, target) {
		return btFailure
	}
	c.ai.State = NpcApproach
	c.ai.Target = target
	c.ai.SwitchAt = c.s.ticks
	c.s.npcAggroLog(c.e, target)
	return btSuccess
}

func leafGuardRat(c *btCtx) btStatus {
	// The ambient security rule: a high-sec guard clears the rats anchored
	// at its own celestial (see the systemNpc doc). Traffic patrols are
	// driven by the belt sweep in systemTraffic instead.
	if !c.def.IsGuard() || c.security != "high" || c.s.Patrols.Has(c.e) {
		return btFailure
	}
	rat := c.s.ratTargetAt(c.ai.Home, c.security)
	if rat.IsZero() {
		return btFailure
	}
	c.s.guardEngageRat(c.e, c.ai, c.tr, rat, c.def)
	return btSuccess
}

func leafIdlePatrol(c *btCtx) btStatus {
	// Guards never sit still: slow patrol around Home — but a patrol working
	// an incident (PatrolRespond) holds station: steering it back to its Home
	// circle mid-fight would drift it out of engagement range of the raiders
	// it is covering (trafficDrivePatrols owns its movement while responding).
	if c.s.patrolResponding(c.e) {
		return btFailure
	}
	c.s.npcPatrol(c.e, c.ai, c.tr, c.def)
	return btSuccess
}

func leafTargetLost(c *btCtx) btStatus {
	if c.targetOnGrid {
		return btFailure
	}
	c.ai.State = NpcReturn
	c.ai.Target = 0
	return btSuccess
}

func leafFleeEntry(c *btCtx) btStatus {
	if c.structFrac >= c.effFlee {
		return btFailure
	}
	c.s.npcEnterFlee(c.e, c.ai, c.tr, c.tPos)
	return btSuccess
}

func leafLeashBreak(c *btCtx) btStatus {
	if c.ai.Home.Dist(c.tPos) <= c.leash {
		return btFailure
	}
	// The target crossed the pursuit leash: break off.
	c.ai.State = NpcReturn
	c.ai.Target = 0
	return btSuccess
}

// leafSwitch is the relative-lead switch: a considerable new attacker takes
// the engagement. It reports Failure after switching on purpose, so the
// approach leaf below steers the tick at the new target (the existing
// "re-enter approach" flow).
func leafSwitch(c *btCtx) btStatus {
	next := c.s.npcSwitchThreat(c)
	if next.IsZero() {
		return btFailure
	}
	c.ai.Target = next
	c.ai.State = NpcApproach
	c.ai.SwitchAt = c.s.ticks
	c.ai.resetHits()
	c.tPos, c.targetOnGrid = c.s.posOf(next)
	return btFailure
}

func leafApproach(c *btCtx) btStatus {
	if c.ai.State != NpcApproach {
		return btFailure
	}
	c.s.Intents.Set(c.e, Intent{Kind: IntentApproach, Target: c.ai.Target})
	if c.tr.Pos.Dist(c.tPos) <= c.def.OrbitRange+50 {
		c.ai.State = NpcOrbit
	}
	return btSuccess
}

func leafOrbit(c *btCtx) btStatus {
	if c.ai.State != NpcOrbit {
		return btFailure
	}
	c.s.Intents.Set(c.e, Intent{Kind: IntentOrbit, Target: c.ai.Target, Range: c.def.OrbitRange})
	return btSuccess
}

func leafFleeStep(c *btCtx) btStatus {
	c.s.npcFleeStep(c.e, c.ai, c.tr, c.tPos)
	return btSuccess
}

func leafReturn(c *btCtx) btStatus {
	// Re-engage only while a candidate is both a trigger and inside the
	// leash — symmetric with the approach break-off, so the state machine can
	// never flip-flop across the leash line.
	if target := c.s.npcThreat(c.e, c.tr.Pos, c.ai, c.def, c.damaged, c.cands, c.effAggro, c.leash, c.playerOnly); !target.IsZero() {
		c.ai.State = NpcApproach
		c.ai.Target = target
		c.ai.SwitchAt = c.s.ticks
		return btSuccess
	}
	home := c.ai.Home.Sub(c.tr.Pos)
	if home.Len() <= npcReturnRange {
		c.ai.State = NpcIdle
		c.s.Intents.Set(c.e, Intent{Kind: IntentStop})
	} else {
		c.s.Intents.Set(c.e, Intent{Kind: IntentManual, Dir: home.Norm(), SpeedFrac: 1})
	}
	return btSuccess
}

// --- miner tree leaves ---

func leafMinerFlee(c *btCtx) btStatus {
	if c.ai.State != NpcFlee && c.s.minerRuns(c.e, c.def, c.security) {
		c.s.stopAllModules(c.e) // the laser cuts out: it is leaving, not working
		c.s.npcEnterFlee(c.e, c.ai, c.tr, c.s.minerAttackerPos(c.ai))
	}
	if c.ai.State != NpcFlee {
		return btFailure
	}
	c.s.npcFleeStep(c.e, c.ai, c.tr, c.s.minerAttackerPos(c.ai))
	return btSuccess
}

func leafMinerHaul(c *btCtx) btStatus {
	if c.ai.State != NpcHaul {
		return btFailure
	}
	c.s.minerHaul(c.e, c.ai, c.tr)
	return btSuccess
}

func leafMinerMine(c *btCtx) btStatus {
	c.ai.State = NpcMine
	c.s.minerMine(c.e, c.ai, c.tr, c.def)
	return btSuccess
}

// --- patrol tree leaves (traffic dispatch owns the assignment) ---

func leafPatrolAggroed(c *btCtx) btStatus {
	if c.ai.State == NpcIdle {
		return btFailure
	}
	if c.ai.Target == c.s.player && (c.ai.State == NpcApproach || c.ai.State == NpcOrbit) {
		c.s.retargetWeapons(c.e, c.s.player)
	}
	return btSuccess
}

func leafPatrolRespond(c *btCtx) btStatus {
	if c.p.State != PatrolRespond {
		return btFailure
	}
	c.s.patrolRespondStep(c.e, c.p, c.ai, c.security)
	return btSuccess
}

func leafPatrolReturn(c *btCtx) btStatus {
	if c.p.State != PatrolReturn {
		return btFailure
	}
	c.s.patrolReturnStep(c.e, c.p, c.ai)
	return btSuccess
}

func leafPatrolSweep(c *btCtx) btStatus {
	if c.p.State != PatrolIdle {
		return btFailure
	}
	c.s.patrolSweepDispatch(c.p, c.sys, c.td, c.security)
	return btSuccess
}

// --- raider tree leaves ---

func leafRaiderEngaged(c *btCtx) btStatus {
	if c.ai.State == NpcIdle {
		return btFailure
	}
	// Engaged with the player: point its combat modules at the player
	// (targets bind at activation, so retarget explicitly).
	if c.ai.Target == c.s.player && (c.ai.State == NpcApproach || c.ai.State == NpcOrbit) {
		c.s.retargetWeapons(c.e, c.s.player)
	}
	return btSuccess
}

func leafRaiderResolved(c *btCtx) btStatus {
	if c.atk.Outcome == "" {
		return btFailure
	}
	return btSuccess // incident resolved: weapons cold until break-off
}

func leafRaiderFight(c *btCtx) btStatus {
	target := c.s.nearbyRespondingPatrol(c.e)
	if target.IsZero() {
		if !c.s.World.Alive(c.hauler) {
			return btFailure
		}
		target = c.hauler
	}
	c.s.retargetWeapons(c.e, target)
	c.s.npcEnsureLock(c.e, target)
	c.s.npcActivate(c.e, c.ai, target)
	return btSuccess
}

// ---------------------------------------------------------------------------
// Prey preference, the damage ledger and the relative-lead switch
// ---------------------------------------------------------------------------

const (
	// npcSwitchLead is how far a new attacker's damage to me must lead the
	// current target's (over the same window) before it can take the
	// engagement: 2x the current target is a considerable threat.
	npcSwitchLead = 2.0
	// npcSwitchWindow is the trailing window (ticks) both damage sums cover.
	npcSwitchWindow = 120
	// npcSwitchDwell is how long (ticks) a hull keeps a target it acquired or
	// switched to before the lead rule may move it again.
	npcSwitchDwell = 600
	// npcHitRingLen bounds the per-hull damage ledger.
	// shortcut: a 64-event ring; a long, many-source brawl can evict events
	// still inside the 120-tick window and under-report a source's lead —
	// enlarge the ring if fights ever get denser than a few shooters.
	npcHitRingLen = 64
)

// npcHitEvent is one damage event in NpcAI.HitLog.
type npcHitEvent struct {
	at  int64      // sim tick the damage landed
	src ecs.Entity // attacker
	dmg float64    // penetrating damage
}

// noteHit records damage this hull just took (systemDamage). The ledger is
// transient — see SavedNpcAI.
func (ai *NpcAI) noteHit(tick int64, src ecs.Entity, dmg float64) {
	if src == 0 || dmg <= 0 {
		return
	}
	ai.HitLog[ai.HitHead] = npcHitEvent{at: tick, src: src, dmg: dmg}
	ai.HitHead = (ai.HitHead + 1) % npcHitRingLen
}

// hitSum is the damage src dealt this hull inside the trailing switch window
// (0 for a source with no live hits). Pure scan: fixed order, no alloc.
func (ai *NpcAI) hitSum(tick int64, src ecs.Entity) float64 {
	var sum float64
	for i := range ai.HitLog {
		if e := &ai.HitLog[i]; e.src == src && tick-e.at <= npcSwitchWindow {
			sum += e.dmg
		}
	}
	return sum
}

// resetHits clears the damage window (on a target switch).
func (ai *NpcAI) resetHits() {
	ai.HitLog = [npcHitRingLen]npcHitEvent{}
	ai.HitHead = 0
}

// preyProfile is one candidate's prey value: cargo capacity (general hold plus
// ore hold, the hull def's numbers when the entity carries no cargo
// component) and fitted weapon count. Pure reads, no allocation.
func (s *Sim) preyProfile(e ecs.Entity) (carry, guns float64) {
	if c, ok := s.Cargos.Get(e); ok {
		carry = c.Capacity + c.OreCapacity
	} else if sh, ok := s.Ships.Get(e); ok {
		if def, ok := s.Content.Ship(sh.Hull); ok {
			carry = def.Cargo + def.OreHold
		}
	}
	if fit, ok := s.Fittings.Get(e); ok {
		for _, class := range slotClasses {
			for _, id := range fit.Defs(class) {
				if def, ok := s.Content.Module(id); ok && def.Kind == content.KindWeapon {
					guns++
				}
			}
		}
	}
	return carry, guns
}

// preyScore ranks one candidate among the eligible set: half its cargo share
// and half its gun share, each normalised against the set's maximum (equal
// weights; a term whose maximum is 0 contributes 0).
func preyScore(carry, guns, maxCarry, maxGuns float64) float64 {
	var score float64
	if maxCarry > 0 {
		score += 0.5 * carry / maxCarry
	}
	if maxGuns > 0 {
		score += 0.5 * guns / maxGuns
	}
	return score
}

// npcTrigger classifies one candidate against the threat machine's triggers.
type npcTrigger uint8

const (
	npcTriggerNone npcTrigger = iota
	npcTriggerAmbient
	npcTriggerHit // trigger (b): the candidate is the damage source
)

// npcTriggered evaluates triggers (a)-(d) for one candidate, unchanged: the
// damage source of another faction (b), else for non-guards an ambient
// trigger — proximity (a), the player's frozen work marker or an NPC mining
// op (c), intrusion (d) — inside maxHomeDist of Home (the leash; MaxFloat64
// for a fresh aggro). Guards only ever self-defend against the player.
func (s *Sim) npcTriggered(ai *NpcAI, def *content.NpcDef, damaged bool, c npcCandidate, selfPos core.Vec2, mine string, effAggro, maxHomeDist float64, playerOnly bool) npcTrigger {
	if playerOnly && c.e != s.player {
		return npcTriggerNone
	}
	homeDist := ai.Home.Dist(c.pos)
	if homeDist > maxHomeDist {
		return npcTriggerNone
	}
	// (b) source-checked damage: same-faction fire never flips an NPC.
	hit := damaged && ai.LastHitBy == c.e && c.faction != mine
	if hit && def.IsGuard() && c.e != s.player {
		hit = false // guards keep their player-only self-defense rule
	}
	if hit {
		return npcTriggerHit
	}
	if def.IsGuard() || !hostile(mine, c.faction) {
		return npcTriggerNone
	}
	switch {
	case effAggro > 0 && selfPos.Dist(c.pos) <= effAggro:
		return npcTriggerAmbient // (a) proximity
	case c.e == s.player && s.recentPlayerWorkNear(ai.Home):
		return npcTriggerAmbient // (c) frozen player work marker
	case c.working && homeDist <= npcWorkRange:
		return npcTriggerAmbient // (c) an NPC mining op inside the work radius
	case homeDist <= npcGuardRange:
		return npcTriggerAmbient // (d) intrusion
	}
	return npcTriggerNone
}

// npcThreat returns the candidate e should engage this tick, or zero.
//
// The damage source (b) keeps initial aggro unchanged: the hull that just hit
// me is answered immediately. Every other candidate that passes today's
// triggers inside the leash is ranked by prey score (cargo capacity and guns
// over the eligible set) — ties go to the nearer candidate, then canonical
// handle order. Guards only ever self-defend against the player (b) — they
// never initiate, and their answer to Old Works hulls is guardEngageRat.
// playerOnly restricts the scan to the player (a live traffic ambusher).
func (s *Sim) npcThreat(e ecs.Entity, selfPos core.Vec2, ai *NpcAI, def *content.NpcDef, damaged bool, candidates []npcCandidate, effAggro, maxHomeDist float64, playerOnly bool) ecs.Entity {
	mine := s.factionOf(e)
	tr := s.traceFor(e)
	for _, c := range candidates {
		if c.e == e {
			continue
		}
		if s.npcTriggered(ai, def, damaged, c, selfPos, mine, effAggro, maxHomeDist, playerOnly) == npcTriggerHit {
			if tr != nil {
				tr.out(fmt.Sprintf("npc-trace tick=%d npc=%q select winner=%q rule=damage-source",
					s.ticks, s.nameOf(e), s.nameOf(c.e)))
			}
			return c.e
		}
	}

	var maxCarry, maxGuns float64
	for _, c := range candidates {
		if c.e == e || s.npcTriggered(ai, def, damaged, c, selfPos, mine, effAggro, maxHomeDist, playerOnly) != npcTriggerAmbient {
			continue
		}
		carry, guns := s.preyProfile(c.e)
		maxCarry = math.Max(maxCarry, carry)
		maxGuns = math.Max(maxGuns, guns)
	}
	var best ecs.Entity
	var bestScore, bestD float64
	for _, c := range candidates {
		if c.e == e || s.npcTriggered(ai, def, damaged, c, selfPos, mine, effAggro, maxHomeDist, playerOnly) != npcTriggerAmbient {
			continue
		}
		carry, guns := s.preyProfile(c.e)
		score := preyScore(carry, guns, maxCarry, maxGuns)
		d := selfPos.Dist(c.pos)
		if best.IsZero() || score > bestScore || (score == bestScore && d < bestD) {
			best, bestScore, bestD = c.e, score, d
		}
		if tr != nil {
			tr.out(fmt.Sprintf("npc-trace tick=%d npc=%q select cand=%q carry=%.0f/%.0f guns=%.0f/%.0f score=%.3f dist=%.0f",
				s.ticks, s.nameOf(e), s.nameOf(c.e), carry, maxCarry, guns, maxGuns, score, d))
		}
	}
	if tr != nil && !best.IsZero() {
		tr.out(fmt.Sprintf("npc-trace tick=%d npc=%q select winner=%q rule=prey-score score=%.3f",
			s.ticks, s.nameOf(e), s.nameOf(best), bestScore))
	}
	return best
}

// npcEngagement is the hull this NPC is already fighting: its persisted
// target while that is live, else whatever its guns are locked on — an NPC
// fighting while its AI state stays idle (a static guard clearing rats, a
// raider holding an ambush). Zero when it has no live engagement.
func (s *Sim) npcEngagement(e ecs.Entity, ai *NpcAI) ecs.Entity {
	if ai.Target != 0 && s.World.Alive(ai.Target) && !s.Docked.Has(ai.Target) {
		return ai.Target
	}
	if tg, ok := s.Targetings.Get(e); ok {
		for _, l := range tg.Locks {
			if l.Target != 0 && s.World.Alive(l.Target) {
				return l.Target
			}
		}
	}
	return 0
}

// npcFlipBlocked reports whether a fresh attacker must NOT flip this NPC: it
// is a role hull already fighting while idle (a static guard clearing rats, a
// raider holding an ambush) and the attacker neither leads the damage window
// by npcSwitchLead nor arrives after the dwell. With no live target any
// attacker leads, and an ordinary hull's idle flip is plain initial aggro —
// the gate exists only where the old code flipped unconditionally mid-fight.
func (s *Sim) npcFlipBlocked(e ecs.Entity, ai *NpcAI, want ecs.Entity) bool {
	def, ok := s.Content.Npc(ai.Npc)
	if !ok || (!def.IsGuard() && !s.isTrafficRaider(e)) {
		return false
	}
	cur := s.npcEngagement(e, ai)
	blocked := false
	if !cur.IsZero() && cur != want {
		newDmg := ai.hitSum(s.ticks, want)
		lead := newDmg >= npcSwitchLead*ai.hitSum(s.ticks, cur)
		blocked = !(lead && newDmg > 0 && s.ticks-ai.SwitchAt >= npcSwitchDwell)
	}
	if tr := s.traceFor(e); tr != nil && blocked {
		tr.out(fmt.Sprintf("npc-trace tick=%d npc=%q switch verdict=hold cur=%q cur-dmg=%.1f new=%q new-dmg=%.1f dwell=%d",
			s.ticks, s.nameOf(e), s.nameOf(cur), ai.hitSum(s.ticks, cur), s.nameOf(want),
			ai.hitSum(s.ticks, want), npcSwitchDwell-(s.ticks-ai.SwitchAt)))
	}
	return blocked
}

// npcSwitchThreat returns the attacker that takes an engaged NPC's target
// under the relative-lead rule, or zero: another eligible hostile whose
// damage over the trailing window leads the current target's by npcSwitchLead,
// once the dwell since the last acquisition has passed (a switch clears the
// window). Ties go to canonical handle order.
func (s *Sim) npcSwitchThreat(c *btCtx) ecs.Entity {
	cur := c.ai.Target
	curDmg := c.ai.hitSum(c.s.ticks, cur)
	curFaction := ""
	if !cur.IsZero() {
		curFaction = c.s.factionOf(cur)
	}
	mine := c.s.factionOf(c.e)
	dwell := c.s.ticks - c.ai.SwitchAt
	var best ecs.Entity
	var bestDmg float64
	if dwell >= npcSwitchDwell {
		for _, cand := range c.cands {
			if cand.e == c.e || cand.e == cur {
				continue
			}
			// A wing-mate of the current target is the same threat already
			// engaged: switching between identical hulls changes nothing.
			if !cur.IsZero() && cand.faction == curFaction {
				continue
			}
			if c.playerOnly && cand.e != c.s.player {
				continue
			}
			if c.def.IsGuard() && cand.e != c.s.player {
				continue
			}
			if !hostile(mine, cand.faction) || c.ai.Home.Dist(cand.pos) > c.leash {
				continue
			}
			dmg := c.ai.hitSum(c.s.ticks, cand.e)
			if dmg <= 0 || dmg < npcSwitchLead*curDmg {
				continue
			}
			if best.IsZero() || dmg > bestDmg {
				best, bestDmg = cand.e, dmg
			}
		}
	}
	if tr := c.s.traceFor(c.e); tr != nil {
		for _, cand := range c.cands {
			if cand.e == c.e {
				continue
			}
			if d := c.ai.hitSum(c.s.ticks, cand.e); d > 0 {
				tr.out(fmt.Sprintf("npc-trace tick=%d npc=%q switch src=%q dmg=%.1f window=%d",
					c.s.ticks, c.s.nameOf(c.e), c.s.nameOf(cand.e), d, npcSwitchWindow))
			}
		}
		verdict := "hold"
		if !best.IsZero() {
			verdict = "switch"
		} else if dwell < npcSwitchDwell {
			verdict = "dwell"
		}
		top, topDmg := "none", 0.0
		if !best.IsZero() {
			top, topDmg = c.s.nameOf(best), bestDmg
		}
		tr.out(fmt.Sprintf("npc-trace tick=%d npc=%q switch verdict=%s cur=%q cur-dmg=%.1f top=%q top-dmg=%.1f lead=%.2f dwell=%d",
			c.s.ticks, c.s.nameOf(c.e), verdict, c.s.nameOf(cur), curDmg, top, topDmg,
			leadRatio(topDmg, curDmg), npcSwitchDwell-dwell))
	}
	return best
}

// leadRatio is the trace's lead figure: the challenger's share of the current
// target's damage (infinite when the current target has taken none).
func leadRatio(top, cur float64) float64 {
	if cur <= 0 {
		return math.Inf(1)
	}
	return top / cur
}

// npcTraceCfg is the harness observer behind `entieve simulate --trace-npc`
// (Sim.npcTrace). Read-only: it formats lines and never touches sim state.
type npcTraceCfg struct {
	name string
	out  func(line string)
}

// TraceNpc attaches a read-only, per-tick decision trace to the NPC with this
// display name ("" detaches). Zero cost when off: every trace site is guarded
// by a nil check, and a traced run must hash identically to an untraced twin.
func (s *Sim) TraceNpc(name string, out func(line string)) {
	if name == "" || out == nil {
		s.npcTrace = nil
		return
	}
	s.npcTrace = &npcTraceCfg{name: name, out: out}
}

func (s *Sim) traceFor(e ecs.Entity) *npcTraceCfg {
	if s.npcTrace == nil || s.nameOf(e) != s.npcTrace.name {
		return nil
	}
	return s.npcTrace
}

// done writes the tick's decision line: which top-level branch handled it and
// the state/target it produced.
func (t *btCtx) done() {
	tr := t.s.traceFor(t.e)
	if tr == nil {
		return
	}
	target := "none"
	if t.ai.Target != 0 {
		target = t.s.nameOf(t.ai.Target)
	}
	tr.out(fmt.Sprintf("npc-trace tick=%d npc=%q role=%s branch=%s state=%s target=%q",
		t.s.ticks, t.s.nameOf(t.e), t.role, t.branch, t.ai.State, target))
}

// npcCandidate is one ship the threat machine may consider: who it is, where
// it is, what it fights for and whether it is working a rock.
type npcCandidate struct {
	e       ecs.Entity
	pos     core.Vec2
	faction string
	working bool
}

// npcCandidates lists the ships every NPC evaluates the same way — the
// player, AI hulls and traffic haulers — in canonical handle order. Wrecks,
// rocks, drones and other props are not candidates.
func (s *Sim) npcCandidates() []npcCandidate {
	var out []npcCandidate
	for _, e := range sortedEntities(s.Ships) {
		if !s.World.Alive(e) || s.Docked.Has(e) {
			continue
		}
		if e != s.player && !s.AIs.Has(e) && !s.Traffic.Has(e) {
			continue
		}
		tr, ok := s.Transforms.Get(e)
		if !ok {
			continue
		}
		out = append(out, npcCandidate{
			e: e, pos: tr.Pos, faction: s.factionOf(e), working: s.npcWorking(e),
		})
	}
	return out
}

// factionOf returns the faction a ship fights for: the entity's own override
// (a Corporate Fleets hull flying an authored machine), else its NPC def's,
// else the independents' flag (the player and traffic haulers carry no
// colours).
func (s *Sim) factionOf(e ecs.Entity) string {
	if ai, ok := s.AIs.Get(e); ok {
		if ai.Faction != "" {
			return ai.Faction
		}
		if def, ok := s.Content.Npc(ai.Npc); ok {
			return def.Faction
		}
	}
	return factionIndependent
}

// npcWorking reports whether e is running a mining cycle — the live form of
// the player's frozen work marker for NPC candidates (aggro trigger c).
func (s *Sim) npcWorking(e ecs.Entity) bool {
	fit, ok := s.Fittings.Get(e)
	if !ok {
		return false
	}
	for _, class := range slotClasses {
		rts := fit.Runtimes(class)
		for i := range rts {
			if !rts[i].Active {
				continue
			}
			if def, ok := s.Content.Module(rts[i].Def); ok && def.Kind == content.KindMiningLaser {
				return true
			}
		}
	}
	return false
}

// npcAggroLog announces an engagement once, on the idle -> approach
// transition. The player line is byte-identical to the old machine's; an NPC
// target gets one SevInfo line naming both hulls.
func (s *Sim) npcAggroLog(e, target ecs.Entity) {
	if target == s.player {
		s.log(SevWarning, e, s.nameOf(e)+" is attacking you")
		return
	}
	s.log(SevInfo, e, s.nameOf(e)+" attacks "+s.nameOf(target))
}

// npcTemperament maps a system's Security rating onto an NPC's effective
// aggro range and flee threshold (see systemNpc for the table). Unknown
// ratings behave like "low" (content validation pins the domain to
// high|low|null anyway).
func npcTemperament(security string, def *content.NpcDef) (effAggro, effFlee float64) {
	switch security {
	case "high":
		return def.AggroRange * 0.4, math.Max(def.FleeThreshold, 0.35)
	case "null":
		return def.AggroRange * 1.3, 0
	default: // "low" (and anything unvalidated)
		return def.AggroRange, def.FleeThreshold
	}
}

// npcPatrol steers an idle NPC around a slow circle centered on its Home so
// guards never sit still. The patrol radius is min(OrbitRange, 2500) m at
// npcPatrolSpeedFrac of MaxSpeed. Everything derives from deterministic
// state: the start phase hashes the entity handle (patrolPhase), the
// angular speed is speed/radius, the target point is a function of sim
// time. No Rand draws — patrol can never shift an RNG stream or break
// twin-run determinism.
func (s *Sim) npcPatrol(e ecs.Entity, ai *NpcAI, tr *Transform, def *content.NpcDef) {
	r := def.OrbitRange
	if r > npcPatrolMaxRadius {
		r = npcPatrolMaxRadius
	}
	m, hasMotion := s.Motions.Get(e)
	if r <= 0 || !hasMotion || m.MaxSpeed <= 0 {
		s.Intents.Set(e, Intent{Kind: IntentStop})
		return
	}
	omega := npcPatrolSpeedFrac * m.MaxSpeed / r
	theta := patrolPhase(e) + omega*s.time
	target := ai.Home.Add(core.Vec2{X: math.Cos(theta) * r, Y: math.Sin(theta) * r})
	dir := target.Sub(tr.Pos)
	if dir.IsZero() {
		// Exactly on the patrol point: slide along the tangent.
		dir = core.Vec2{X: -math.Sin(theta), Y: math.Cos(theta)}
	}
	s.Intents.Set(e, Intent{Kind: IntentManual, Dir: dir.Norm(), SpeedFrac: npcPatrolSpeedFrac})
}

// npcMinerWarpRange is the distance (m) past which a miner warps to its
// work: frontier belts scatter wider than the sensor grid, and sublight
// would never cross them.
const npcMinerWarpRange = 100_000

// minerRuns reports whether a damaged miner's structure has fallen under its
// temperament-scaled flee threshold (the same fraction the threat machine's
// flee trigger reads). A 0 threshold (null space) keeps even a miner at the
// rock.
func (s *Sim) minerRuns(e ecs.Entity, def *content.NpcDef, security string) bool {
	ship, ok := s.Ships.Get(e)
	if !ok || ship.MaxStructure <= 0 {
		return false
	}
	_, effFlee := npcTemperament(security, def)
	return effFlee > 0 && ship.Structure/ship.MaxStructure < effFlee
}

// minerAttackerPos is where a miner runs from: its last attacker while that
// hull is still on the grid, else its Home — it leaves the belt either way.
func (s *Sim) minerAttackerPos(ai *NpcAI) core.Vec2 {
	if p, ok := s.posOf(ai.LastHitBy); ok {
		return p
	}
	return ai.Home
}

// minerMine cuts the nearest rock with volume left. The mining laser needs no
// lock (targetRock, see checkCycle): the miner binds it straight to the rock.
func (s *Sim) minerMine(e ecs.Entity, ai *NpcAI, tr *Transform, def *content.NpcDef) {
	if s.minerHoldFull(e) {
		st := s.nearestStation(tr.Pos)
		if st.IsZero() {
			return // no market in this system: the hold stays put
		}
		ai.State = NpcHaul
		ai.Target = st
		s.stopAllModules(e) // nothing cycles while the hold is full
		return
	}
	rock := s.nearestBeltRock(tr.Pos)
	if rock.IsZero() {
		s.npcPatrol(e, ai, tr, def) // empty belt: keep drifting, no RNG
		return
	}
	ai.Target = rock
	rp, ok := s.posOf(rock)
	if !ok {
		return
	}
	dist := tr.Pos.Dist(rp)
	if dist > npcMinerWarpRange {
		if !s.Warps.Has(e) {
			s.warpToPoint(e, rp)
		}
		return
	}
	if dist > def.OrbitRange {
		s.Intents.Set(e, Intent{Kind: IntentApproach, Target: rock})
		return
	}
	s.Intents.Set(e, Intent{Kind: IntentStop})
	s.npcActivate(e, ai, rock)
}

// minerHaul carries a full hold to the station picked when the hold filled.
func (s *Sim) minerHaul(e ecs.Entity, ai *NpcAI, tr *Transform) {
	sp, ok := s.posOf(ai.Target)
	if ai.Target.IsZero() || !ok {
		ai.State = NpcMine
		ai.Target = 0
		return
	}
	if tr.Pos.Dist(sp) > warpLandRange {
		if !s.Warps.Has(e) {
			s.warpToPoint(e, sp)
		}
		return
	}
	if !s.minerDeliver(e, ai.Target) {
		ai.State = NpcMine // an unsellable load stays aboard: back to cutting
		return
	}
	s.recordRespawn(e, ai)
	s.destroy(e)
}

// minerDeliver posts a miner's whole ore hold on the station's book as
// system-owned asks, one tick under the station's own best ask — the convoy
// delivery rule (see the traffic delivery in system_traffic.go), which is
// also why a delivery mints no money: settle() only moves currency for
// player-owned orders, so the system-side ask simply adds supply at a real
// price. Deterministic: items are posted in sorted order, one order each.
// Reports whether the hold came out empty (an ore with no tradable price
// stays aboard, so nothing is destroyed by a bad quote).
func (s *Sim) minerDeliver(e ecs.Entity, station ecs.Entity) bool {
	c, ok := s.Cargos.Get(e)
	cel, celOK := s.Celestials.Get(station)
	if !ok || !celOK || len(c.Ore) == 0 {
		return true
	}
	items := make([]content.ItemID, 0, len(c.Ore))
	for _, st := range c.Ore {
		items = append(items, st.Item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i] < items[j] })
	book := s.market[cel.Cel]
	var left []content.CargoStack // stacks with no tradable price stay aboard
	for _, item := range items {
		qty := stacksCount(c.Ore, item)
		price := s.marketBasePrice(item)
		if book != nil {
			if ask, ok := book.bestAsk(item); ok && ask.Price < price {
				price = ask.Price
			}
		}
		post := tickDown(price)
		if qty <= 0 || post <= 0 {
			left = addStacks(left, item, qty) // never lose a unit to a bad price
			continue
		}
		s.PlaceOrder(cel.Cel, OrderSell, item, qty, post, OwnerSystem)
		s.emit(Event{Kind: EvLog, Severity: SevInfo, Actor: e, System: s.systemID, Item: item,
			Value: float64(qty),
			Text: s.nameOf(e) + " delivered " + formatM3(s.itemVolume(item)*float64(qty)) +
				" m³ " + s.itemName(item) + " to " + s.nameOf(station)})
	}
	c.Ore = left
	s.Cargos.Set(e, *c)
	return len(left) == 0
}

// minerHoldFull reports whether a miner's ore container has less than one m³
// left.
// shortcut: full = under one m³ free (every shipped ore is 1 m³ per unit);
// split the check per ore if sub-m³ ores ever ship.
func (s *Sim) minerHoldFull(e ecs.Entity) bool {
	c, ok := s.Cargos.Get(e)
	if !ok {
		return true
	}
	container, capM3 := oreContainer(c)
	return capM3 <= 0 || capM3-s.stacksVolume(*container) < 1
}

// nearestBeltRock returns the nearest live asteroid with volume left (zero
// when the grid holds none) — a miner's next cut. Canonical handle order and
// a strict comparison make ties deterministic.
func (s *Sim) nearestBeltRock(pos core.Vec2) ecs.Entity {
	var best ecs.Entity
	bestD := math.MaxFloat64
	for _, e := range sortedEntities(s.Asteroids) {
		a, ok := s.Asteroids.Get(e)
		if !ok || a.Remaining <= beltDepletedEpsilon {
			continue
		}
		p, ok := s.posOf(e)
		if !ok {
			continue
		}
		if d := pos.Dist(p); d < bestD {
			bestD, best = d, e
		}
	}
	return best
}

// nearestStation returns the nearest station celestial on the grid (zero when
// the system has none).
func (s *Sim) nearestStation(pos core.Vec2) ecs.Entity {
	var best ecs.Entity
	bestD := math.MaxFloat64
	for _, e := range sortedEntities(s.Celestials) {
		c, ok := s.Celestials.Get(e)
		if !ok || c.Kind != content.CelStation {
			continue
		}
		p, ok := s.posOf(e)
		if !ok {
			continue
		}
		if d := pos.Dist(p); d < bestD {
			bestD, best = d, e
		}
	}
	return best
}

// ratTargetAt returns the rat (npc-rat-*) anchored at home the security
// forces should engage next — the target a high-sec guard clears, and the
// belt occupancy a patrol sweep looks for. Free rats come first (an NPC's
// fight stays its own while other rats are free); within each set the rats
// are ranked by the same prey score as the threat machine (cargo capacity and
// guns, ties to the nearer rat, then canonical handle order), because a guard
// clearing a belt wants the fat hull, not the first handle. When no rat is
// free any live rat ranks the same way: security finishes the belt rather
// than leaving a player-touched rat parked there. Zero only when the belt is
// empty. Home is the spawn anchor (the celestial position), so the match is
// an exact comparison: no distance scan beyond the ranking, no persisted
// state, and the chase that follows is whatever the belt's own scatter gives.
func (s *Sim) ratTargetAt(home core.Vec2, sec string) ecs.Entity {
	var maxCarry, maxGuns float64
	for _, e := range sortedEntities(s.AIs) {
		if _, ok := s.beltRatAt(e, home); !ok {
			continue
		}
		carry, guns := s.preyProfile(e)
		maxCarry = math.Max(maxCarry, carry)
		maxGuns = math.Max(maxGuns, guns)
	}
	var free, live ecs.Entity
	var freeScore, liveScore, freeD, liveD float64
	for _, e := range sortedEntities(s.AIs) {
		ai, ok := s.beltRatAt(e, home)
		if !ok {
			continue
		}
		carry, guns := s.preyProfile(e)
		score := preyScore(carry, guns, maxCarry, maxGuns)
		d := math.MaxFloat64
		if p, ok := s.posOf(e); ok {
			d = home.Dist(p)
		}
		if s.ratFree(ai, sec) {
			if free.IsZero() || score > freeScore || (score == freeScore && d < freeD) {
				free, freeScore, freeD = e, score, d
			}
			continue
		}
		if live.IsZero() || score > liveScore || (score == liveScore && d < liveD) {
			live, liveScore, liveD = e, score, d
		}
	}
	if !free.IsZero() {
		return free
	}
	return live
}

// beltRatAt reports whether e is a live belt rat anchored at home: the rat
// hulls security clears, never a war hull flying one (the Corporate Fleets'
// faction override is a front action, not a belt sweep).
func (s *Sim) beltRatAt(e ecs.Entity, home core.Vec2) (*NpcAI, bool) {
	ai, ok := s.AIs.Get(e)
	if !ok || ai.Home != home {
		return nil, false
	}
	if def, ok := s.Content.Npc(ai.Npc); !ok || !isRatNpc(def) || ai.Faction != "" {
		return nil, false
	}
	return ai, true
}

// ratNear returns the closest live rat (npc-rat-*) within r of pos, ties in
// canonical handle order — the hostiles a distress wing clears around the
// miner it was sent to (no Home comparison: the rats it hunts are wherever
// the attack is).
func (s *Sim) ratNear(pos core.Vec2, r float64) ecs.Entity {
	var best ecs.Entity
	bestD := math.MaxFloat64
	for _, e := range sortedEntities(s.AIs) {
		ai, ok := s.AIs.Get(e)
		if !ok || ai.Faction != "" {
			continue
		}
		if def, ok := s.Content.Npc(ai.Npc); !ok || !isRatNpc(def) {
			continue
		}
		if p, ok := s.posOf(e); ok {
			if d := p.Dist(pos); d <= r && d < bestD {
				best, bestD = e, d
			}
		}
	}
	return best
}

// ratFree reports whether a rat is free to engage before the belt's
// player-touched stragglers: idle, or holding a pursuit its target has
// already left — outside the rat's own leash, which systemNpc breaks off on
// the next tick (trigger (b) never expires, so a rat the player damaged and
// flew away from flinches onto the player every few ticks; that flinch is
// not a fight). A rat actually in a fight stays that target's only while
// another rat is free; ratTargetAt falls back to it otherwise.
func (s *Sim) ratFree(ai *NpcAI, sec string) bool {
	switch ai.State {
	case NpcIdle, "":
		return true
	case NpcApproach, NpcOrbit, NpcReturn:
		def, ok := s.Content.Npc(ai.Npc)
		if !ok {
			return false
		}
		effAggro, _ := npcTemperament(sec, def)
		tp, ok := s.posOf(ai.Target)
		return !ok || ai.Home.Dist(tp) > npcLeashFactor*effAggro
	}
	return false
}

// guardEngageRat drives a static high-sec guard onto a belt rat: close to
// orbit range, lock, and fire through the normal module pipeline. The
// guard's AI state stays idle — guards remain friendly on the overview and
// only ever enter the threat machine to self-defend against the player —
// and the target is recomputed from world state every tick, so a killed rat
// needs no cleanup: the next tick finds the next rat or resumes the idle
// patrol. This is also the coverage a mining op gets: a rat mid-attack on a
// miner anchored here is just a rat at the guard's celestial.
func (s *Sim) guardEngageRat(e ecs.Entity, ai *NpcAI, tr *Transform, rat ecs.Entity, def *content.NpcDef) {
	rp, ok := s.posOf(rat)
	if !ok {
		return
	}
	if tr.Pos.Dist(rp) <= def.OrbitRange+50 {
		s.Intents.Set(e, Intent{Kind: IntentOrbit, Target: rat, Range: def.OrbitRange})
	} else {
		s.Intents.Set(e, Intent{Kind: IntentApproach, Target: rat})
	}
	s.npcEnsureLock(e, rat)
	s.npcActivate(e, ai, rat)
	s.ratDefend(rat, e)
}

// ratDefend points an idle rat's weapons at the security ship clearing it —
// the trafficRaidersAct pattern (an idle NPC's weapons are driven at an
// explicit target without touching its AI state). A rat that takes the
// guard's fire also aggros it through the machine's damage trigger (b); this
// gives it return fire on the first lock, before that damage lands.
//
// No retargetWeapons here: several security ships can work one rat, and
// re-pointing a running gun between them every tick would churn
// activate/deactivate forever. The gun binds to the first attacker it locks
// and the module system re-arms it when that target dies.
func (s *Sim) ratDefend(rat, attacker ecs.Entity) {
	ai, ok := s.AIs.Get(rat)
	if !ok || ai.State != NpcIdle {
		return
	}
	s.npcEnsureLock(rat, attacker)
	s.npcActivate(rat, ai, attacker)
}

// patrolPhase maps an entity handle to its patrol start angle in [0, 2π)
// through the SplitMix64 finalizer — a pure hash of the handle, kept
// deliberately separate from any *core.RNG so patrolling draws nothing.
func patrolPhase(e ecs.Entity) float64 {
	h := uint64(e)
	h ^= h >> 30
	h *= 0xbf58476d1ce4e5b9
	h ^= h >> 27
	h *= 0x94d049bb133111eb
	h ^= h >> 31
	return float64(h>>11) * (2 * math.Pi / float64(uint64(1)<<53))
}

// playerWorkNever marks lastPlayerWork.at as "no work recorded this run".
// A plain 0 would be ambiguous: tick 0 is a legitimate moment to fire.
const playerWorkNever = -1

// recordPlayerWork notes that the player just fired a weapon or completed a
// mining cycle at pos (NPC aggro trigger c, see systemNpc).
func (s *Sim) recordPlayerWork(pos core.Vec2) {
	s.lastPlayerWork.pos = pos
	s.lastPlayerWork.at = s.time
}

// recentPlayerWorkNear reports whether the player fired a weapon or ran a
// mining cycle within npcWorkRange of home in the last npcWorkWindow sim
// seconds (NPC aggro trigger c).
func (s *Sim) recentPlayerWorkNear(home core.Vec2) bool {
	w := s.lastPlayerWork
	if w.at < 0 || s.time-w.at > npcWorkWindow {
		return false
	}
	return w.pos.Dist(home) <= npcWorkRange
}

// npcEnterFlee switches an NPC into the flee state, recording the flee
// start time in the (otherwise unused) manual-intent Range field.
func (s *Sim) npcEnterFlee(e ecs.Entity, ai *NpcAI, tr *Transform, awayFrom core.Vec2) {
	ai.State = NpcFlee
	away := tr.Pos.Sub(awayFrom)
	if away.IsZero() {
		away = core.Vec2{X: 1}
	}
	s.Intents.Set(e, Intent{Kind: IntentManual, Dir: away.Norm(), SpeedFrac: 1, Range: s.time})
	s.log(SevInfo, e, s.nameOf(e)+" is fleeing")
}

// npcFleeStep is the NpcFlee state: burn away from awayFrom (the target for
// an engaged NPC, the attacker for a damaged miner) and despawn quietly
// after npcFleeSeconds. The flee start time rides Intent.Range.
func (s *Sim) npcFleeStep(e ecs.Entity, ai *NpcAI, tr *Transform, awayFrom core.Vec2) {
	away := tr.Pos.Sub(awayFrom)
	if away.IsZero() {
		away = core.Vec2{X: 1}
	}
	start := s.time
	if in, ok := s.Intents.Get(e); ok && in.Kind == IntentManual && in.Range > 0 {
		start = in.Range
	}
	s.Intents.Set(e, Intent{Kind: IntentManual, Dir: away.Norm(), SpeedFrac: 1, Range: start})
	if s.time-start >= npcFleeSeconds {
		s.emit(Event{Kind: EvLog, Severity: SevInfo, Actor: e,
			Text: s.nameOf(e) + " retreats and escapes"})
		s.recordRespawn(e, ai)
		s.destroy(e)
	}
}

// npcEnsureLock starts locking target if the NPC is not already.
func (s *Sim) npcEnsureLock(e, target ecs.Entity) {
	tg, ok := s.Targetings.Get(e)
	if !ok {
		return
	}
	for _, l := range tg.Locks {
		if l.Target == target {
			return
		}
	}
	if len(tg.Locks) >= tg.MaxTargets {
		return
	}
	tg.Locks = append(tg.Locks, Lock{Target: target})
}

// npcActivate runs the NPC's per-tick module activation policy against an
// explicit target: weapons and ewar once the target is locked and in range,
// mining lasers on a rock (no lock needed), tank when damaged, cap booster
// when low, afterburner while fleeing.
// Failures are silent (quiet). systemNpc passes whatever its threat machine
// resolved (the player or another NPC); the traffic system passes the
// ambushed hauler or an attacking patrol — both run NPC-vs-NPC combat
// through the normal lock/activate/turret/damage pipeline.
func (s *Sim) npcActivate(e ecs.Entity, ai *NpcAI, target ecs.Entity) {
	fit, ok := s.Fittings.Get(e)
	if !ok {
		return
	}
	ship, _ := s.Ships.Get(e)
	cap, _ := s.Caps.Get(e)

	for _, class := range slotClasses {
		defs, rts := fit.Defs(class), fit.Runtimes(class)
		for i := range rts {
			// Retry gating: a module that just failed (cap, ammo, locks) is
			// left alone for npcRetryCooldown seconds so NPCs do not chatter
			// a re-activation attempt every tick.
			if rts[i].Active || s.time < rts[i].LastWarn+npcRetryCooldown {
				continue
			}
			def, ok := s.Content.Module(defs[i])
			if !ok {
				continue
			}
			slot := SlotRef{Class: class, Index: i}
			switch def.Kind {
			case content.KindWeapon:
				s.activate(e, slot, target, true)
			case content.KindMiningLaser:
				s.activate(e, slot, target, true)
			case content.KindStasisWeb, content.KindWarpScram:
				s.activate(e, slot, target, true)
			case content.KindShieldBoost:
				if ship != nil && ship.Shield < ship.MaxShield*0.9 {
					s.activate(e, slot, 0, true)
				}
			case content.KindArmorRep:
				if ship != nil && ship.Armor < ship.MaxArmor*0.9 {
					s.activate(e, slot, 0, true)
				}
			case content.KindCapBooster:
				if cap != nil && cap.Capacity > 0 && cap.Current < cap.Capacity*0.3 {
					s.activate(e, slot, 0, true)
				}
			case content.KindAfterburner:
				if ai.State == NpcFlee {
					s.activate(e, slot, 0, true)
				}
			}
		}
	}
}
