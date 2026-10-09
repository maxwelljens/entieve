package sim

import (
	"math"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

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
// (aggro trigger d).
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
//	(b) the NPC took damage;
//	(c) the player fired a weapon or ran a mining cycle within 30 km of the
//	    NPC's Home in the last 20 sim-seconds;
//	(d) the player is within 8 km of the NPC's Home (guardians investigate
//	    interlopers — belt/anomaly guards especially).
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
		aggroOK := playerOnGrid && effAggro > 0 && dist <= effAggro
		if playerOnGrid && (ai.State == NpcIdle || ai.State == "" || ai.State == NpcReturn) {
			if damaged {
				aggroOK = true
			}
			if s.recentPlayerWorkNear(ai.Home) {
				aggroOK = true
			}
			if homeDist <= npcGuardRange {
				aggroOK = true
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
			} else {
				// Guards never sit still: slow patrol around Home.
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
				s.recordRespawn(ai)
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
				s.npcActivate(e, ai)
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

// npcActivate runs the NPC's per-tick module activation policy: weapons and
// ewar once the player is locked and in range, tank when damaged, cap
// booster when low, afterburner while fleeing. Failures are silent (quiet).
func (s *Sim) npcActivate(e ecs.Entity, ai *NpcAI) {
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
				s.activate(e, slot, s.player, true)
			case content.KindStasisWeb, content.KindWarpScram:
				s.activate(e, slot, s.player, true)
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
