package sim

import (
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// ---------------------------------------------------------------------------
// F3 — NPC engagement ownership pins (behavior only, no resolver refactor):
// (a) aggro trigger (b) is source-checked — third-party damage never flips
// an NPC onto the player; (b) a patrol responding to a convoy ambush holds
// station on the incident instead of being steered back to its Home circle
// by the idle-patrol loop.
// ---------------------------------------------------------------------------

// TestThirdPartyDamageDoesNotAggroOntoPlayer: a raider shot by a patrol (as
// happens when the response joins a convoy ambush) stays idle — no aggro
// onto a distant player — while the identical damage from the player still
// aggros (positive control, so the trigger cannot silently die).
func TestThirdPartyDamageDoesNotAggroOntoPlayer(t *testing.T) {
	s := newTrafficSim(t) // player parked at -5e6,-5e6: outside every trigger
	patrol := s.spawnPatrolShip(s.System(), "ship-guard")
	if patrol.IsZero() {
		t.Fatal("spawnPatrolShip failed")
	}
	raider := s.spawnNpcStream("npc-rat-raider", v(1e7, 60000), 100, s.trafficRNG)
	if raider.IsZero() {
		t.Fatal("raider spawn failed")
	}

	damageNPC(s, patrol, raider, content.Damage{EM: 50}, false)
	s.Step()
	evs := drain(s)
	ai, ok := s.AIs.Get(raider)
	if !ok {
		t.Fatal("raider has no AI")
	}
	if ai.LastHitBy != patrol {
		t.Fatalf("LastHitBy=%v want the patrol %v", ai.LastHitBy, patrol)
	}
	if ai.State != NpcIdle {
		t.Fatalf("state=%q want idle: patrol damage aggroed the raider onto the player", ai.State)
	}
	if hasEvent(evs, EvLog, "is attacking you") {
		t.Fatalf("aggro warning emitted for third-party damage: %v", evs)
	}
	// It must stay that way — the HP-based "damaged" flag persists.
	stepN(s, 300)
	drain(s)
	if ai, _ = s.AIs.Get(raider); ai.State != NpcIdle {
		t.Fatalf("state=%q after 300 ticks, want idle", ai.State)
	}
	if infoHostile(s, raider) {
		t.Fatal("idle NPC must not be hostile")
	}

	// Positive control: the same damage from the player aggros.
	victim := s.spawnNpcStream("npc-rat-raider", v(1e7, 70000), 100, s.trafficRNG)
	damageNPC(s, s.player, victim, content.Damage{EM: 50}, false)
	s.Step()
	evs = drain(s)
	if ai, _ = s.AIs.Get(victim); ai.State != NpcApproach {
		t.Fatalf("state=%q want approach for player-sourced damage", ai.State)
	}
	if ai.LastHitBy != s.player {
		t.Fatalf("LastHitBy=%v want the player", ai.LastHitBy)
	}
	if !hasEvent(evs, EvLog, "is attacking you") {
		t.Fatalf("missing aggro warning for player-sourced damage: %v", evs)
	}
}

// TestRespondingPatrolHoldsStation: a patrol in PatrolRespond with a live
// raid to cover keeps its position on the incident — no idle-patrol drift
// back to its Home anchor — and never switches its weapons off the fight.
func TestRespondingPatrolHoldsStation(t *testing.T) {
	s := newTrafficSim(t)

	// Incident: an ambushed hauler with one live raider, outcome already
	// recorded so the raiders stay quiet (this pin is about patrol
	// movement). The patrol response keys off the live raid either way.
	hauler := s.create()
	s.Names.Set(hauler, Name{Value: "Test Hauler"})
	s.Transforms.Set(hauler, Transform{Pos: v(0, 0)})
	raider := s.spawnNpcStream("npc-rat-raider", v(3000, 0), 100, s.trafficRNG)
	if raider.IsZero() {
		t.Fatal("raider spawn failed")
	}
	s.Traffic.Set(hauler, Traffic{
		Phase: TrafficTransit, Dest: "cel-gate-b", Next: 1e18,
		Attack: &TrafficAttack{
			Raiders: []ecs.Entity{raider}, Started: s.time,
			RespondAt: -1, Responded: true, Outcome: TrafficOutcomeSaved,
		},
	})

	patrol := s.spawnPatrolShip(s.System(), "ship-guard")
	if patrol.IsZero() {
		t.Fatal("spawnPatrolShip failed")
	}
	p, ok := s.Patrols.Get(patrol)
	if !ok {
		t.Fatal("patrol component missing")
	}
	p.State = PatrolRespond
	p.Incident = hauler
	p.Next = 0
	// On the incident but nowhere near its Home anchor (25 km east of the
	// station): any idle-patrol steering pulls it away from the fight.
	teleport(s, patrol, v(0, 6000))
	if m, ok := s.Motions.Get(patrol); ok {
		m.Vel = core.Vec2{} // measure drift, not carried momentum
	}
	start, _ := s.posOf(patrol)

	stepN(s, 600)

	if p, ok = s.Patrols.Get(patrol); !ok || p.State != PatrolRespond {
		t.Fatalf("patrol state=%v want respond", p)
	}
	ai, ok := s.AIs.Get(patrol)
	if !ok || ai.State != NpcIdle {
		t.Fatalf("patrol AI state=%v want idle (must not aggro off the fight)", ai)
	}
	if in, ok := s.Intents.Get(patrol); !ok || in.Kind != IntentStop {
		t.Fatalf("patrol intent=%+v want a hold (IntentStop)", in)
	}
	got, _ := s.posOf(patrol)
	if d := got.Dist(start); d > 100 {
		t.Fatalf("patrol drifted %v m from its station point (idle-patrol steering)", d)
	}
	if d, _ := s.posOf(raider); got.Dist(d) > trafficPatrolEngageRange {
		t.Fatalf("patrol is %v m from the raid, beyond the %v m engage range",
			got.Dist(d), trafficPatrolEngageRange)
	}
}
