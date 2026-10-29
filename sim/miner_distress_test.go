package sim

import (
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
)

// ---------------------------------------------------------------------------
// Miner distress: a Dvužek barge that takes hostile damage in charter space
// calls the Charter Authority, which answers with the same patrol wing a
// convoy ambush gets — held on the miner, one live response per miner, none
// inside Works control.
// ---------------------------------------------------------------------------

// respondingTo count of live patrols working miner.
func wingOn(s *Sim, miner ecs.Entity) int {
	n := 0
	s.Patrols.Each(func(_ ecs.Entity, p *Patrol) {
		if p.Incident == miner && p.State == PatrolRespond {
			n++
		}
	})
	return n
}

// TestMinerDistressCallsTheWing pins the trigger, the wing size and the
// rate limit: one dispatch on the first hostile hit, no second wing while the
// first is still on the incident.
func TestMinerDistressCallsTheWing(t *testing.T) {
	s := newTrafficSim(t)
	beltPos := mustPos(s, s.celEntity("cel-belt-a"))
	miner := spawnMiner(t, s, beltPos)
	rat := s.spawnNpc("npc-rat-scuttler", beltPos.Add(v(3000, 0)), 0)
	drain(s)

	damageNPC(s, rat, miner, content.Damage{EM: 5}, false)
	s.Step()
	evs := drain(s)
	resp := firstPhase(evs, EvTraffic, TrafficPatrolResponse)
	if resp == nil {
		t.Fatalf("no patrol response to the miner under fire: %v", evs)
	}
	if want := float64(trafficPatrolWing("high")); resp.Value != want {
		t.Fatalf("wing = %v, want the high-sec wing %v", resp.Value, want)
	}
	if got := wingOn(s, miner); got != trafficPatrolWing("high") {
		t.Fatalf("%d patrols hold the miner, want %d", got, trafficPatrolWing("high"))
	}

	// Rate limit: more damage while the wing is live dispatches nothing.
	damageNPC(s, rat, miner, content.Damage{EM: 5}, false)
	s.Step()
	if n := countPhase(drain(s), EvTraffic, TrafficPatrolResponse); n != 0 {
		t.Fatalf("%d extra responses while a wing was live, want 0", n)
	}
	if got := wingOn(s, miner); got != trafficPatrolWing("high") {
		t.Fatalf("wing grew to %d, want %d (one live response per miner)", got, trafficPatrolWing("high"))
	}
}

// TestMinerDistressWingClearsAndLeaves pins the miner branch of the patrol
// response: the wing warps in, fires on the rat around the miner, and goes
// home once no rat is left.
func TestMinerDistressWingClearsAndLeaves(t *testing.T) {
	s := newTrafficSim(t)
	beltPos := mustPos(s, s.celEntity("cel-belt-a"))
	miner := spawnMiner(t, s, beltPos)
	rat := s.spawnNpc("npc-rat-scuttler", beltPos.Add(v(2000, 0)), 0)
	drain(s)

	damageNPC(s, rat, miner, content.Damage{EM: 5}, false)
	s.Step()
	drain(s)

	engaged := false
	for i := 0; i < 3000 && !engaged; i++ {
		s.Step()
		drain(s)
		s.Patrols.Each(func(pe ecs.Entity, p *Patrol) {
			if p.Incident != miner || p.State != PatrolRespond {
				return
			}
			if tgt, ok := s.modTarget(pe, SlotRef{Class: content.SlotHigh, Index: 0}); ok && tgt == rat {
				engaged = true
			}
		})
	}
	if !engaged {
		t.Fatal("the wing never fired on the rat attacking the miner")
	}

	// Kill the rat: the wing has nothing left to hold and returns home.
	s.destroy(rat)
	drain(s)
	for i := 0; i < 1200; i++ {
		s.Step()
		drain(s)
	}
	s.Patrols.Each(func(_ ecs.Entity, p *Patrol) {
		if p.State == PatrolRespond && p.Incident == miner {
			t.Fatalf("patrol still holding the miner after the rats died")
		}
	})
}

// TestMinerDistressSkipsWorksControl: charter security does not answer inside
// Works control (the war's own rule).
func TestMinerDistressSkipsWorksControl(t *testing.T) {
	s := newTrafficSim(t)
	beltPos := mustPos(s, s.celEntity("cel-belt-a"))
	miner := spawnMiner(t, s, beltPos)
	rat := s.spawnNpc("npc-rat-scuttler", beltPos.Add(v(3000, 0)), 0)
	s.newWarState()
	s.war.Control["sys-a"] = controlWorks
	drain(s)

	damageNPC(s, rat, miner, content.Damage{EM: 5}, false)
	s.Step()
	if n := countPhase(drain(s), EvTraffic, TrafficPatrolResponse); n != 0 {
		t.Fatalf("%d responses inside Works control, want 0", n)
	}
	if got := wingOn(s, miner); got != 0 {
		t.Fatalf("%d patrols hold the miner in Works space, want 0", got)
	}
}

// TestMinerDistressIgnoresSisterFire: damage from the miner's own faction is
// not distress (the same-faction rule every NPC keeps).
func TestMinerDistressIgnoresSisterFire(t *testing.T) {
	s := newTrafficSim(t)
	beltPos := mustPos(s, s.celEntity("cel-belt-a"))
	miner := spawnMiner(t, s, beltPos)
	sister := spawnMiner(t, s, beltPos.Add(v(2000, 0)))
	drain(s)

	damageNPC(s, sister, miner, content.Damage{EM: 5}, false)
	s.Step()
	if n := countPhase(drain(s), EvTraffic, TrafficPatrolResponse); n != 0 {
		t.Fatalf("%d responses to same-faction fire, want 0", n)
	}
}
