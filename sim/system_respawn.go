package sim

// systemRespawn fires pending respawn timers for the CURRENT system (only
// the live system's spawn groups tick; timers for other systems are kept
// untouched in the Save and resume when their system is loaded again).
func (s *Sim) systemRespawn() {
	if len(s.respawns) == 0 {
		return
	}
	kept := make([]RespawnTimer, 0, len(s.respawns))
	for _, t := range s.respawns {
		if t.System == s.systemID && s.time >= t.At {
			s.respawnTimer(t)
			continue
		}
		kept = append(kept, t)
	}
	s.respawns = kept
}

// respawnTimer spawns one NPC for an expired timer near its celestial and
// emits exactly one info event when the NPC is actually created, so players
// and tests can observe respawn timers working.
func (s *Sim) respawnTimer(t RespawnTimer) {
	sys := s.System()
	if sys == nil {
		return
	}
	for i := range sys.Celestials {
		cel := &sys.Celestials[i]
		if cel.ID != t.Cel {
			continue
		}
		if e := s.spawnNpc(t.Npc, cel.Position, celestialSpread(cel)); !e.IsZero() {
			s.emit(Event{Kind: EvLog, Severity: SevInfo, Actor: e,
				Text: s.nameOf(e) + " respawned near " + cel.Name})
		}
		return
	}
	// The celestial no longer exists: drop the timer.
}
