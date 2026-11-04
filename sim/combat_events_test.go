package sim

import (
	"testing"

	"github.com/entieve/entieve/content"
)

// ---------------------------------------------------------------------------
// The combat event set: the structured twin the HUD's combat feed reads.
// The log keeps its own line (EvLog); EvCombat carries no text of its own, so
// one screen can colour what the other prints flat.
// ---------------------------------------------------------------------------

// combatEvents collects every EvCombat of one kind/phase from acc.
func combatEvents(acc []Event, phase string) []Event {
	var out []Event
	for _, ev := range acc {
		if ev.Kind == EvCombat && ev.Phase == phase {
			out = append(out, ev)
		}
	}
	return out
}

// TestCombatDamageCarriesStructure: a landed shot emits the structured twin
// with source, victim, penetrating amount and the source's display name, and
// nothing to print (the log line is the EvLog event beside it).
func TestCombatDamageCarriesStructure(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	forceLock(s, npc)
	drain(s)
	loadBay(t, s, "ammo-emp-s")
	s.Queue(CmdActivate{Slot: SlotRef{Class: content.SlotHigh, Index: 0}, Target: npc})

	var acc []Event
	var hit *Event
	for i := 0; i < 600 && hit == nil; i++ {
		s.Step()
		evs := drain(s)
		acc = append(acc, evs...)
		if got := combatEvents(evs, CombatDamage); len(got) > 0 {
			hit = &got[0]
		}
	}
	if hit == nil {
		t.Fatalf("no EvCombat damage event in 600 ticks (%d events)", len(acc))
	}
	if hit.Actor != s.player || hit.Target != npc {
		t.Errorf("damage event actor/target = %v/%v, want the player %v / the rat %v",
			hit.Actor, hit.Target, s.player, npc)
	}
	if hit.Value <= 0 {
		t.Errorf("damage value = %v, want the penetrating amount", hit.Value)
	}
	if want := s.nameOf(s.player); hit.Name != want {
		t.Errorf("damage source name = %q, want %q", hit.Name, want)
	}
	if hit.Text != "" {
		t.Errorf("damage event carries text %q: the log line is the EvLog twin", hit.Text)
	}
	// The flat log line is still there, unchanged.
	found := false
	for _, ev := range acc {
		if ev.Kind == EvLog && ev.Actor == s.player && ev.Target == npc {
			found = true
			break
		}
	}
	if !found {
		t.Error("the per-hit EvLog line went away (the log would lose damage lines)")
	}
}

// TestCombatEWARCarriesStructure: an applied web and an applied scramble name
// both sides, once per fresh application (a re-application on the next cycle
// is not news).
func TestCombatEWARCarriesStructure(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	web := injectModule(t, s, "mod-stasis-web")
	scram := injectModule(t, s, "mod-warp-scram")
	forceLock(s, npc)
	drain(s)
	s.Queue(CmdActivate{Slot: web, Target: npc})
	s.Queue(CmdActivate{Slot: scram, Target: npc})

	var acc []Event
	for i := 0; i < 60; i++ {
		s.Step()
		acc = append(acc, drain(s)...)
	}
	for _, phase := range []string{CombatWeb, CombatScram} {
		got := combatEvents(acc, phase)
		if len(got) == 0 {
			t.Fatalf("no %q event: %v", phase, acc)
		}
		ev := got[0]
		if ev.Actor != s.player || ev.Target != npc || ev.Name != s.nameOf(s.player) {
			t.Errorf("%s = actor %v target %v name %q, want the player %v / rat %v / %q",
				phase, ev.Actor, ev.Target, ev.Name, s.player, npc, s.nameOf(s.player))
		}
		// Active for a long spell: the cycles refresh the effect without
		// another line (the feed reports the application, not the upkeep).
		if n := len(combatEvents(acc, phase)); n > 1 && phase == CombatWeb {
			t.Errorf("%q emitted %d times in one application, want once", phase, n)
		}
	}
}

// TestCombatDestroyedCarriesName: the kill event names its victim, so the feed
// can print it after the entity is gone (Info no longer answers).
func TestCombatDestroyedCarriesName(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayer(t, s)
	name := s.nameOf(npc)
	drain(s)
	damageNPC(s, s.player, npc, content.Damage{EM: 1e9}, false)
	s.Step()
	for _, ev := range drain(s) {
		if ev.Kind != EvDestroyed || ev.Actor != npc {
			continue
		}
		if ev.Name != name {
			t.Errorf("EvDestroyed name = %q, want the victim's %q", ev.Name, name)
		}
		if ev.Target != s.player {
			t.Errorf("EvDestroyed target = %v, want the killer %v", ev.Target, s.player)
		}
		return
	}
	t.Fatal("no EvDestroyed event for the killed hull")
}
