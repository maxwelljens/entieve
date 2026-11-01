package sim

import "github.com/entieve/entieve/ecs"

// Friendly fire is the player's own choice, recorded rather than forbidden:
// the first aggressive activation on a peaceful hull opens the client's
// confirmation dialog (EvFriendlyFire), the confirmed act fires the module
// and stamps the record, and every later friendly-fire hit slides the
// window. A run of confirmed acts spanning aggressionPersistSec puts the
// player OUTLAW: security then treats the pilot as a target on sight, through
// the same threat machine a rat walks into. The record is transient (like
// lastPlayerWork): it never enters the Save, so a restore starts clean.
const (
	// aggressionPersistSec is how long a run of confirmed acts must span
	// before the warrant is issued: one mistaken volley stays a mistake.
	aggressionPersistSec = 10.0
	// aggressionWindowSec is the quiet time that clears the record, and with
	// it the outlaw warrant.
	aggressionWindowSec = 60.0
)

// playerAggression is the player's friendly-fire run: first is the sim time
// of the run's first confirmed act, until the expiry of its live window
// (until-windowSec is the last act).
type playerAggression struct {
	first float64
	until float64
}

// execActivate applies CmdActivate through the friendly-fire gate: an
// aggressive module aimed at a peaceful hull does nothing until the player
// confirms it. The client opens the dialog from EvFriendlyFire; headless
// scripts send the same command with Confirm set.
func (s *Sim) execActivate(c CmdActivate) {
	if !c.Confirm {
		if def, _, ok := s.moduleDef(s.player, c.Slot); ok && def.IsAggressive() && s.friendlyTarget(c.Target) {
			s.emit(Event{Kind: EvFriendlyFire, Severity: SevWarning, Actor: s.player,
				Target: c.Target, Slot: c.Slot,
				Name: def.Name, Text: "Friendly fire: " + def.Name + " on " + s.nameOf(c.Target)})
			return
		}
	}
	if s.activate(s.player, c.Slot, c.Target, false) && s.peacefulShip(c.Target) {
		s.stampAggression()
	}
}

// peacefulShip reports whether e is a crewed, live, non-player ship that is
// not at war with independents: the hulls a confirmed hit is friendly fire
// against. Rats and raiders fail (they are the war); miners, haulers,
// patrols and guards pass. An unpiloted hull (no AI, no traffic) is not a
// crew and never prompts.
func (s *Sim) peacefulShip(e ecs.Entity) bool {
	if e.IsZero() || e == s.player || !s.World.Alive(e) || s.Docked.Has(e) || !s.Ships.Has(e) {
		return false
	}
	if s.isTrafficRaider(e) {
		return false
	}
	if !s.AIs.Has(e) && !s.Traffic.Has(e) {
		return false
	}
	return !hostile(s.factionOf(e), factionIndependent)
}

// friendlyTarget is the confirm gate's predicate: a peaceful hull that is not
// already fighting the player. A self-defending guard, and any security hull
// once the player is outlaw, is fair game and prompts nothing.
func (s *Sim) friendlyTarget(e ecs.Entity) bool {
	if !s.peacefulShip(e) {
		return false
	}
	if ai, ok := s.AIs.Get(e); ok &&
		(ai.State == NpcApproach || ai.State == NpcOrbit) && ai.Target == s.player {
		return false
	}
	return true
}

// playerOutlaw reports whether the player's confirmed acts span the
// persistence window while it is still live: all security answers the
// warrant. A lapsed window reads clean without erasing the record, so the
// next act starts a fresh run.
func (s *Sim) playerOutlaw() bool {
	a := s.aggression
	return a.until > 0 && s.time < a.until &&
		(a.until-aggressionWindowSec)-a.first >= aggressionPersistSec
}

// stampAggression records one confirmed friendly-fire act, starting a run or
// sliding the live window, and announces the warrant the first time the run
// spans aggressionPersistSec.
func (s *Sim) stampAggression() {
	was := s.playerOutlaw()
	if s.aggression.until == 0 || s.time >= s.aggression.until {
		s.aggression.first = s.time // a lapsed window: a fresh run starts here
	}
	s.aggression.until = s.time + aggressionWindowSec
	if !was && s.playerOutlaw() {
		s.emit(Event{Kind: EvOutlaw, Severity: SevDanger, Actor: s.player,
			Text: "Outlaw: security fires on sight"})
	}
}
