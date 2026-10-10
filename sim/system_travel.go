package sim

import (
	"github.com/entieve/entieve/core"
)

// systemTravel executes deferred dock/undock/jump commands.
//
// Docking and jumping require the player within dockRange (2,500 m) of the
// station/gate. Jumping rebuilds the destination system while preserving the
// player entity, ship, cargo, wallet and hangar (see loadSystem).
func (s *Sim) systemTravel() {
	for _, cmd := range s.deferTravel {
		switch c := cmd.(type) {
		case CmdDock:
			s.execDock(c)
		case CmdUndock:
			s.execUndock()
		case CmdJump:
			s.execJump(c)
		}
	}
	s.deferTravel = nil
}

// execDock docks the player at a station entity.
func (s *Sim) execDock(c CmdDock) {
	p := s.player
	if s.playerDocked() {
		s.log(SevWarning, p, "Already docked")
		return
	}
	if !s.Stations.Has(c.Station) || !s.World.Alive(c.Station) {
		s.log(SevWarning, p, "No such station")
		return
	}
	cel, ok := s.Celestials.Get(c.Station)
	if !ok {
		return
	}
	if s.distBetween(p, c.Station) > dockRange {
		s.log(SevWarning, p, "Station is out of range")
		return
	}
	s.Docked.Set(p, Docked{At: cel.Cel})
	if tr, ok := s.Transforms.Get(c.Station); ok {
		if ptr, ok := s.Transforms.Get(p); ok {
			ptr.Pos = tr.Pos // sit at the station while inside
		}
	}
	if m, ok := s.Motions.Get(p); ok {
		m.Vel = core.Vec2{}
	}
	s.Intents.Set(p, Intent{Kind: IntentStop})
	s.Warps.Remove(p)
	s.lastStation = cel.Cel
	s.dockedThisTick = cel.Cel // mission auto-claim trigger (systemMission)
	s.emit(Event{Kind: EvDocked, Actor: p, Target: c.Station,
		Text: "Docked at " + s.nameOf(c.Station)})
}

// execUndock leaves the station: the player appears 5,000 m off along +X.
func (s *Sim) execUndock() {
	p := s.player
	d, ok := s.Docked.Get(p)
	if !ok {
		s.log(SevWarning, p, "You are not docked")
		return
	}
	station := s.stationAt(d.At)
	s.Docked.Remove(p)
	if station.IsZero() {
		s.log(SevWarning, p, "No station to undock from")
		return
	}
	if tr, ok := s.Transforms.Get(station); ok {
		if ptr, ok := s.Transforms.Get(p); ok {
			ptr.Pos = tr.Pos.Add(core.Vec2{X: 5000})
		}
	}
	if m, ok := s.Motions.Get(p); ok {
		m.Vel = core.Vec2{}
	}
	s.Intents.Set(p, Intent{Kind: IntentStop})
	s.lastStation = d.At
	s.emit(Event{Kind: EvUndocked, Actor: p, Target: station,
		Text: "Undocked from " + s.nameOf(station)})
}

// execJump jumps through a stargate into the destination system.
func (s *Sim) execJump(c CmdJump) {
	p := s.player
	if s.playerDocked() {
		s.log(SevWarning, p, "Cannot jump while docked")
		return
	}
	gate, ok := s.Gates.Get(c.Gate)
	if !ok || !s.World.Alive(c.Gate) {
		s.log(SevWarning, p, "No such stargate")
		return
	}
	if s.distBetween(p, c.Gate) > dockRange {
		s.log(SevWarning, p, "Stargate is out of range")
		return
	}
	destName := string(gate.ToSystem)
	if def, ok := s.Content.System(gate.ToSystem); ok {
		destName = def.Name
	}
	if err := s.loadSystem(gate.ToSystem, gate.ToCel); err != nil {
		s.log(SevDanger, p, "Jump failed: "+err.Error())
		return
	}
	s.emit(Event{Kind: EvJumped, Actor: p, Target: c.Gate,
		Text: "Jumped to " + destName})
}
