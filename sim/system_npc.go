package sim

import (
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

// npcFleeSeconds is how long a fleeing NPC burns away from the player
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

// npcWorkRange is the radius (m) around a recorded player weapon/mining
// work position that alerts NPC Homes inside it (aggro trigger c).
const npcWorkRange = 30_000

// npcWorkWindow is how long (sim seconds) recent player weapon/mining work
// keeps guards alert (aggro trigger c).
const npcWorkWindow = 20

// npcLeashFactor multiplies the effective aggro range into the pursuit
// leash: an aggroed NPC engages while the player is within
// leash = npcLeashFactor * effAggro of its Home, and breaks off beyond it.
const npcLeashFactor = 2.5

// npcPatrolSpeedFrac is the idle patrol speed as a fraction of MaxSpeed.
const npcPatrolSpeedFrac = 0.25

// npcPatrolMaxRadius caps the idle patrol radius (m).
const npcPatrolMaxRadius = 2500

// systemNpc advances every NPC state machine:
//
//	idle (patrolling) -> approach -> orbit -> flee -> despawn
//	     \-> return (player beyond the pursuit leash or gone) -> idle
//
// Aggro (idle -> approach) fires when any of these triggers holds for an
// on-grid player:
//
//	(a) the player is within the NPC's effective aggro range;
//	(b) the NPC took damage from the player (source-checked: third-party
//	    fire — a patrol during a convoy ambush — never aggroes an NPC
//	    onto the player);
//	(c) the player fired a weapon or ran a mining cycle within 30 km of the
//	    NPC's Home in the last 20 sim-seconds;
//	(d) the player is within 8 km of the NPC's Home (guardians investigate
//	    interlopers — belt/anomaly guards especially).
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
// ambient security rule. Traffic patrols are exempt from that rule (their
// belt sweep lives in systemTraffic) and low/null space keeps guards
// passive. The threat triggers exist so pirate rats are not passive, and
// apply to every non-guard NPC exactly as before.
//
// Civilian miners are the second exception (npc-miner-*, NpcDef.IsMiner):
// they never enter this machine at all — npcMiner runs their extraction loop
// and they stay passive through it (no trigger, no flee). Their own def is
// what makes them passive; nothing here is weakened for rats.
//
// The pursuit leash is leash = npcLeashFactor * effAggro measured from the
// NPC's Home: an engaged NPC chases/engages while the player stays inside
// it and breaks off to return when it is crossed; a returning NPC only
// re-engages while a trigger holds AND the player is back inside the leash
// (symmetric with the break-off, so the state machine can never flip-flop
// across the leash line). A fleeing NPC ignores the leash and keeps the
// 20 s burn-away-then-despawn.
//
// SystemDef.Security tunes temperament through npcTemperament:
//
//	"high" — effAggro = AggroRange * 0.4, flee threshold = max(def, 0.35)
//	         (skittish: shoots late, runs early);
//	"low"  — effAggro = AggroRange, thresholds exactly as authored;
//	"null" — effAggro = AggroRange * 1.3, flee threshold 0 (fights to the
//	         death — never flees).
//
// Idle NPCs never sit still: they drift on a slow deterministic patrol
// circle around NpcAI.Home (npcPatrol, phase hashed from the entity handle
// — no RNG draws). Aggroed NPCs lock the player through the normal targeting
// system and activate their weapons/tank/ewar modules through the normal
// module system, with npcRetryCooldown gating failed re-attempts.
func (s *Sim) systemNpc() {
	playerAlive := !s.player.IsZero() && s.World.Alive(s.player)
	playerOnGrid := playerAlive && !s.playerDocked()
	var pPos core.Vec2
	if playerAlive {
		if tr, ok := s.Transforms.Get(s.player); ok {
			pPos = tr.Pos
		}
	}
	security := ""
	if sys := s.System(); sys != nil {
		security = sys.Security
	}

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
		// Civilian miners run their own extraction loop and never enter the
		// threat machine below: passive by definition, never aggroed by the
		// player and never fleeing.
		if def.IsMiner() {
			s.npcMiner(e, ai, tr, def)
			continue
		}

		effAggro, effFlee := npcTemperament(security, def)

		dist := 0.0
		homeDist := 0.0
		if playerAlive {
			dist = tr.Pos.Dist(pPos)
			homeDist = ai.Home.Dist(pPos)
		}
		structFrac := 1.0
		if ship.MaxStructure > 0 {
			structFrac = ship.Structure / ship.MaxStructure
		}
		damaged := ship.Shield < ship.MaxShield || ship.Armor < ship.MaxArmor ||
			ship.Structure < ship.MaxStructure

		// Aggro triggers (a)-(d); (b)-(d) only pull a non-pursuing NPC in.
		// Guards never initiate on the player: for npc-guard-* the
		// proximity (a), recent-work (c) and intrusion (d) triggers are
		// skipped — friendly security only defends (b), while pirate rats
		// keep the full threat response ("rats must not be passive").
		isGuard := def.IsGuard()
		aggroOK := !isGuard && playerOnGrid && effAggro > 0 && dist <= effAggro
		if playerOnGrid && (ai.State == NpcIdle || ai.State == "" || ai.State == NpcReturn) {
			// (b) source-blind damage would flip every NPC on the grid onto
			// the player whenever anything else shot it (a patrol fighting
			// raiders, rat-on-rat fire), at any distance: the damage must
			// have come from the player.
			if damaged && ai.LastHitBy == s.player {
				aggroOK = true
			}
			if !isGuard {
				if s.recentPlayerWorkNear(ai.Home) {
					aggroOK = true
				}
				if homeDist <= npcGuardRange {
					aggroOK = true
				}
			}
		}
		// Pursuit leash measured from Home: engagement requires the player
		// inside leash = npcLeashFactor * effAggro.
		leashed := playerOnGrid && homeDist <= npcLeashFactor*effAggro

		switch ai.State {
		case NpcIdle, "":
			// Any trigger aggros a fresh NPC from wherever the player is;
			// the leash then governs whether the pursuit actually continues
			// (approach/orbit break off when it is crossed).
			if aggroOK {
				ai.State = NpcApproach
				ai.Target = s.player
				s.log(SevWarning, e, s.nameOf(e)+" is attacking you")
				break
			}
			// The ambient security rule: a high-sec guard clears the rats
			// anchored at its own celestial (see the systemNpc doc). Traffic
			// patrols are driven by the belt sweep in systemTraffic instead.
			if isGuard && security == "high" && !s.Patrols.Has(e) {
				if rat := s.ratTargetAt(ai.Home, security); !rat.IsZero() {
					s.guardEngageRat(e, ai, tr, rat, def)
					break
				}
			}
			if !s.patrolResponding(e) {
				// Guards never sit still: slow patrol around Home — but a
				// patrol working an incident (PatrolRespond) holds station:
				// steering it back to its Home circle mid-fight would drift
				// it out of engagement range of the raiders it is covering
				// (trafficDrivePatrols owns its movement while responding).
				s.npcPatrol(e, ai, tr, def)
			}

		case NpcApproach:
			if !playerOnGrid {
				ai.State = NpcReturn
				ai.Target = 0
				break
			}
			if structFrac < effFlee {
				s.npcEnterFlee(e, ai, tr, pPos)
				break
			}
			if !leashed {
				// Player crossed the pursuit leash: break off.
				ai.State = NpcReturn
				ai.Target = 0
				break
			}
			s.Intents.Set(e, Intent{Kind: IntentApproach, Target: s.player})
			if dist <= def.OrbitRange+50 {
				ai.State = NpcOrbit
			}

		case NpcOrbit:
			if !playerOnGrid {
				ai.State = NpcReturn
				ai.Target = 0
				break
			}
			if structFrac < effFlee {
				s.npcEnterFlee(e, ai, tr, pPos)
				break
			}
			if !leashed {
				ai.State = NpcReturn
				ai.Target = 0
				break
			}
			s.Intents.Set(e, Intent{Kind: IntentOrbit, Target: s.player, Range: def.OrbitRange})

		case NpcFlee:
			// Burn away from the player; despawn after npcFleeSeconds.
			away := tr.Pos.Sub(pPos)
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
			continue // no combat actions while fleeing

		case NpcReturn:
			// Re-engage only while the player is both a trigger and inside
			// the leash — symmetric with the approach break-off, so the
			// state machine can never flip-flop across the leash line.
			if aggroOK && leashed {
				ai.State = NpcApproach
				ai.Target = s.player
				break
			}
			home := ai.Home.Sub(tr.Pos)
			if home.Len() <= npcReturnRange {
				ai.State = NpcIdle
				s.Intents.Set(e, Intent{Kind: IntentStop})
			} else {
				s.Intents.Set(e, Intent{Kind: IntentManual, Dir: home.Norm(), SpeedFrac: 1})
			}
			continue
		}

		// Combat actions: aggroed NPCs lock the player and run their modules.
		if ai.State == NpcApproach || ai.State == NpcOrbit {
			if playerOnGrid {
				s.npcEnsureLock(e, s.player)
				s.npcActivate(e, ai, s.player)
			}
		}
	}
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

// npcMiner runs the civilian extraction loop of npc-miner-* (NpcDef.IsMiner):
//
//	mine — cut the nearest rock with volume left, moving on when it
//	       depletes; when the hold is full switch to haul
//	haul — warp to the NEAREST STATION of the same system, and on arrival
//	       post the hold's ore on that station's market as system supply,
//	       then despawn (recordRespawn re-arms the spawn group's timer)
//
// The player is never a target and never an aggro trigger: miners skip the
// player-centric threat machine entirely (no lock, no flee). They are
// otherwise normal ships — lockable, damageable, and death runs the ordinary
// wreck path.
func (s *Sim) npcMiner(e ecs.Entity, ai *NpcAI, tr *Transform, def *content.NpcDef) {
	switch ai.State {
	case NpcHaul:
		s.minerHaul(e, ai, tr)
	default:
		ai.State = NpcMine
		s.minerMine(e, ai, tr, def)
	}
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
// forces should engage next, in canonical handle order — the target a
// high-sec guard clears, and the belt occupancy a patrol sweep looks for.
// Free rats come first (a pilot's fight stays the pilot's while other rats
// are free), but when none are free any live rat is returned: security
// finishes the belt rather than leaving a player-touched rat parked there.
// Zero only when the belt is empty. Home is the spawn anchor (the celestial
// position), so the match is an exact comparison: no distance scan, no
// persisted state, and the chase that follows is whatever the belt's own
// scatter gives.
func (s *Sim) ratTargetAt(home core.Vec2, sec string) ecs.Entity {
	var live ecs.Entity
	for _, e := range sortedEntities(s.AIs) {
		ai, ok := s.AIs.Get(e)
		if !ok || ai.Home != home {
			continue
		}
		if def, ok := s.Content.Npc(ai.Npc); !ok || !isRatNpc(def) {
			continue
		}
		if s.ratFree(ai, sec) {
			return e
		}
		if live.IsZero() {
			live = e
		}
	}
	return live
}

// ratFree reports whether a rat is free to engage before the belt's
// player-touched stragglers: idle, or holding a pursuit the player has
// already left — outside the rat's own leash, which systemNpc breaks off on
// the next tick (trigger (b) never expires, so a rat the player damaged and
// flew away from flinches onto the player every few ticks; that flinch is
// not a fight). A rat the player is actually fighting stays the player's
// only while another rat is free; ratTargetAt falls back to it otherwise.
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
		pp, ok := s.posOf(s.player)
		return !ok || ai.Home.Dist(pp) > npcLeashFactor*effAggro
	}
	return false
}

// guardEngageRat drives a static high-sec guard onto a belt rat: close to
// orbit range, lock, and fire through the normal module pipeline. The
// guard's AI state stays idle — guards remain friendly on the overview and
// the player-centric state machine above is untouched — and the target is
// recomputed from world state every tick, so a killed rat needs no cleanup:
// the next tick finds the next rat or resumes the idle patrol.
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
// explicit target without touching its AI state). The rat's own state
// machine stays player-centric, so its return fire can never pull it onto
// the player; it fights the patrol to the death while it lives.
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
func (s *Sim) npcEnterFlee(e ecs.Entity, ai *NpcAI, tr *Transform, pPos core.Vec2) {
	ai.State = NpcFlee
	away := tr.Pos.Sub(pPos)
	if away.IsZero() {
		away = core.Vec2{X: 1}
	}
	s.Intents.Set(e, Intent{Kind: IntentManual, Dir: away.Norm(), SpeedFrac: 1, Range: s.time})
	s.log(SevInfo, e, s.nameOf(e)+" is fleeing")
}

// npcEnsureLock starts locking the player if the NPC is not already.
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
// Failures are silent (quiet). systemNpc passes the player; the traffic
// system passes the ambushed hauler or an attacking patrol, which is what
// makes NPC-vs-NPC combat run through the normal lock/activate/turret/
// damage pipeline.
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
