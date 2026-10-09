package sim

import (
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

// systemNpc advances every NPC state machine:
//
//	idle -> approach -> orbit -> flee (below FleeThreshold structure) -> despawn
//	             \-> return (player beyond AggroRange or gone) -> idle
//
// NPCs aggro when the player is within AggroRange or when they take damage,
// lock the player through the normal targeting system and activate their
// weapons/tank/ewar modules through the normal module system.
func (s *Sim) systemNpc() {
	playerAlive := !s.player.IsZero() && s.World.Alive(s.player)
	playerOnGrid := playerAlive && !s.playerDocked()
	var pPos core.Vec2
	if playerAlive {
		if tr, ok := s.Transforms.Get(s.player); ok {
			pPos = tr.Pos
		}
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

		dist := 0.0
		if playerAlive {
			dist = tr.Pos.Dist(pPos)
		}
		structFrac := 1.0
		if ship.MaxStructure > 0 {
			structFrac = ship.Structure / ship.MaxStructure
		}
		damaged := ship.Shield < ship.MaxShield || ship.Armor < ship.MaxArmor ||
			ship.Structure < ship.MaxStructure

		// Aggro: player inside aggro range, or the NPC has been shot.
		aggroOK := playerOnGrid && def.AggroRange > 0 && dist <= def.AggroRange
		if damaged && playerOnGrid && (ai.State == NpcIdle || ai.State == NpcReturn) {
			aggroOK = true
		}

		switch ai.State {
		case NpcIdle, "":
			if aggroOK {
				ai.State = NpcApproach
				ai.Target = s.player
				s.log(SevWarning, e, s.nameOf(e)+" is attacking you")
			}

		case NpcApproach:
			if !playerOnGrid {
				ai.State = NpcReturn
				ai.Target = 0
				break
			}
			if structFrac < def.FleeThreshold {
				s.npcEnterFlee(e, ai, tr, pPos)
				break
			}
			if !aggroOK {
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
			if structFrac < def.FleeThreshold {
				s.npcEnterFlee(e, ai, tr, pPos)
				break
			}
			if !aggroOK {
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
			if aggroOK {
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
