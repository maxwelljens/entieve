package sim

import (
	"math"
	"sort"
	"strconv"

	"github.com/entieve/entieve/content"
)

// systemMission runs the contract loop (docs/CONTENT_AUTHORING.md):
//
//	offered (station board, content) -> accepted (CmdAcceptMission while
//	docked at the offering station) -> progress (kills by rat class, ore
//	mined into the hold) -> ready (objectives met) -> completed (auto-claim
//	when the player docks at the claim station: the offering station for
//	kill/mine, the destination for deliver), or abandoned
//	(CmdAbandonMission).
//
// It executes deferred accept/abandon commands, folds the objective
// observations queued earlier in this Step by systemDamage (player kills)
// and systemMining (ore yields), then auto-claims any contract whose
// objectives are met at a docking event. No expiry in v1; no RNG.
func (s *Sim) systemMission() {
	for _, cmd := range s.deferMission {
		switch c := cmd.(type) {
		case CmdAcceptMission:
			s.execAcceptMission(c)
		case CmdAbandonMission:
			s.execAbandonMission(c)
		}
	}
	s.deferMission = nil

	// Fold this tick's objective observations (parallel to s.missions).
	justReady := make([]bool, len(s.missions))
	for _, npc := range s.pendingKills {
		s.missionNpcKilled(npc, justReady)
	}
	s.pendingKills = nil
	for _, y := range s.pendingOre {
		s.missionOreMined(y.item, y.vol, justReady)
	}
	s.pendingOre = nil

	// Auto-claim on a docking event at the claim station (or when readiness
	// is reached this very tick while already docked there — the only way
	// objectives can complete under a dock, since they advance off-station).
	if s.playerDocked() && (s.dockedThisTick != "" || containsTrue(justReady)) {
		at, _ := s.Docked.Get(s.player)
		for i := 0; i < len(s.missions); {
			def, ok := s.missionDef(&s.missions[i])
			if !ok {
				// Stale save reference (contract removed from content):
				// drop it rather than pay or crash.
				s.missions = append(s.missions[:i], s.missions[i+1:]...)
				if i < len(justReady) {
					justReady = append(justReady[:i], justReady[i+1:]...)
				}
				continue
			}
			claim := claimStation(def)
			triggered := s.dockedThisTick == claim ||
				(justReady[i] && at.At == claim)
			if triggered && s.missionObjectivesMet(def, &s.missions[i]) {
				if def.Kind == content.ContractDeliver {
					s.emitMissionEvent(MissionReady, def, SevInfo,
						"Offer ready: "+def.Title+", delivered at "+s.stationName(claim), 0)
				}
				s.completeMission(i, def)
				justReady = append(justReady[:i], justReady[i+1:]...)
				continue
			}
			i++
		}
	}
	s.dockedThisTick = ""
}

// missionDef resolves an accepted mission's task: its materialized offer when
// the save carries one, else the content lookup (a pre-contacts save). A
// retired id with no def reads as absent — the mission is dropped gracefully,
// never paid and never crashed on.
func (s *Sim) missionDef(m *MissionState) (*content.ContractDef, bool) {
	if m.Def != nil {
		return m.Def, true
	}
	return s.Content.Contract(m.ID)
}

// landStock books delivered cargo as the destination station's requisition
// stock: a resting ask one tick under the ladder, the same posting rule a
// convoy delivery uses, so guard-post requisitions draw from it.
func (s *Sim) landStock(station content.CelID, item content.ItemID, units int) {
	if station == "" || units <= 0 {
		return
	}
	book := s.market[StationID(station)]
	if book == nil {
		return
	}
	price := s.marketBasePrice(item)
	if ask, ok := book.bestAsk(item); ok && ask.Price < price {
		price = ask.Price
	}
	if post := tickDown(price); post > 0 {
		s.PlaceOrder(StationID(station), OrderSell, item, units, post, OwnerSystem)
	}
}

// claimStation returns the station celestial where an offer turns in.
func claimStation(def *content.ContractDef) content.CelID {
	if def.Kind == content.ContractDeliver {
		return def.Objective.Destination
	}
	return def.Station
}

// missionNpcKilled counts one player kill toward matching kill contracts.
func (s *Sim) missionNpcKilled(npc content.NpcID, justReady []bool) {
	for i := range s.missions {
		def, ok := s.missionDef(&s.missions[i])
		if !ok || def.Kind != content.ContractKill || def.Objective.Target != npc {
			continue
		}
		m := &s.missions[i]
		count := float64(def.Objective.Count)
		if m.Progress >= count {
			continue
		}
		m.Progress++
		s.emitMissionEvent(MissionProgress, def, SevInfo,
			"Offer progress: "+def.Title+", "+itoaN(int(m.Progress))+"/"+itoaN(def.Objective.Count), 0)
		if m.Progress >= count {
			justReady[i] = true
			s.emitMissionEvent(MissionReady, def, SevGood,
				"Offer ready: "+def.Title+". Dock at "+s.stationName(claimStation(def))+" to claim", 0)
		}
	}
}

// missionOreMined counts mined ore volume (m³) toward matching mine
// contracts. Only mining yields count — transfers and market purchases do
// not (see systemMining, which queues them).
func (s *Sim) missionOreMined(item content.ItemID, vol float64, justReady []bool) {
	if vol <= 0 {
		return
	}
	for i := range s.missions {
		def, ok := s.missionDef(&s.missions[i])
		if !ok || def.Kind != content.ContractMine || def.Objective.Item != item {
			continue
		}
		m := &s.missions[i]
		count := float64(def.Objective.Count)
		if m.Progress >= count {
			continue
		}
		m.Progress += vol
		if m.Progress > count {
			m.Progress = count
		}
		s.emitMissionEvent(MissionProgress, def, SevInfo,
			"Offer progress: "+def.Title+", "+formatM3(m.Progress)+"/"+formatM3(count)+" m³", 0)
		if m.Progress >= count {
			justReady[i] = true
			s.emitMissionEvent(MissionReady, def, SevGood,
				"Offer ready: "+def.Title+". Dock at "+s.stationName(claimStation(def))+" to claim", 0)
		}
	}
}

// missionObjectivesMet reports whether a contract can be claimed right now.
func (s *Sim) missionObjectivesMet(def *content.ContractDef, m *MissionState) bool {
	if def.Kind == content.ContractDeliver {
		cargo, ok := s.Cargos.Get(s.player)
		if !ok {
			return false
		}
		units := stacksCount(cargo.Stacks, def.Objective.Item) +
			stacksCount(cargo.Ore, def.Objective.Item)
		return units >= s.deliverUnits(def)
	}
	return m.Progress >= float64(def.Objective.Count)
}

// deliverUnits converts a deliver objective's m³ count into whole item
// units (the validator guarantees count is a whole number of units).
func (s *Sim) deliverUnits(def *content.ContractDef) int {
	vol := s.itemVolume(def.Objective.Item)
	if vol <= 0 {
		return def.Objective.Count
	}
	return int(math.Round(float64(def.Objective.Count) / vol))
}

// completeMission claims offer i: consumes the delivered cargo — which lands
// on the destination's book as requisition stock the guard posts buy from —
// credits the war front the work served, pays the reward (and bonus item to
// the hangar), adds standing, and drops the mission from the active list.
func (s *Sim) completeMission(i int, def *content.ContractDef) {
	m := s.missions[i]
	if def.Kind == content.ContractDeliver {
		units := s.deliverUnits(def)
		if cargo, ok := s.Cargos.Get(s.player); ok {
			need := units
			var taken int
			cargo.Stacks, taken = takeStacks(cargo.Stacks, def.Objective.Item, need)
			if taken < need {
				cargo.Ore, _ = takeStacks(cargo.Ore, def.Objective.Item, need-taken)
			}
		}
		s.landStock(def.Objective.Destination, def.Objective.Item, units)
		// A completed delivery is hull practice, scaled by the offer's
		// authored level (the content state's risk rung).
		if ship, ok := s.Ships.Get(s.player); ok {
			if hull, ok := s.Content.Ship(ship.Hull); ok {
				s.awardProficiency(hull.Family, 4*float64(1+def.Level))
			}
		}
		if m.Front != "" {
			// Delivered materiel is charter push at the front: the goods'
			// value, the same euro-equivalent procurement credits.
			s.warCredit(m.Front, float64(units)*s.marketBasePrice(def.Objective.Item))
		}
	}
	s.wallet += def.Reward.Credits
	s.recordLedger(LedgerEntry{
		What:   "Contact: " + def.Title,
		Where:  s.stationName(s.dockedStationID()),
		Amount: def.Reward.Credits,
	})
	if def.Reward.Item != "" && def.Reward.Qty > 0 {
		s.Hangar.Items = addStacks(s.Hangar.Items, def.Reward.Item, def.Reward.Qty)
	}
	s.standing += def.Reward.StandingGain
	s.missions = append(s.missions[:i], s.missions[i+1:]...)

	text := "Offer complete: " + def.Title + ", " +
		euro(def.Reward.Credits) + " transferred"
	if def.Reward.StandingGain > 0 {
		text += ", standing +" + strconv.FormatFloat(def.Reward.StandingGain, 'f', 2, 64)
	}
	if def.Reward.Item != "" && def.Reward.Qty > 0 {
		text += " + " + itoaN(def.Reward.Qty) + " x " + s.itemName(def.Reward.Item)
	}
	s.emitMissionEvent(MissionCompleted, def, SevGood, text, def.Reward.Credits)
}

// execAcceptMission validates and accepts one live offer at the docked
// station. Offers are derived, never stored: the def accepted is the one the
// derivation returns this tick, and it travels in the MissionState so the
// work survives the gap closing. A fixture's authored contract (Content.Contract)
// still resolves as a static offer for the engine tests. Every rejection
// emits an EvMission "refused" line.
func (s *Sim) execAcceptMission(c CmdAcceptMission) {
	p := s.player
	if !s.playerDocked() {
		s.missionRefused(c.ID, "", "You must be docked to accept an offer")
		return
	}
	at, _ := s.Docked.Get(p)
	var def *content.ContractDef
	var front content.SystemID
	if offer, ok := s.offerByID(at.At, c.ID); ok {
		def, front = offer.Def, offer.Front
	} else {
		d, ok := s.Content.Contract(c.ID)
		if !ok {
			s.missionRefused(c.ID, "", "No such offer "+string(c.ID))
			return
		}
		if d.Station != at.At {
			s.missionRefused(d.ID, d.Title, "Offer not offered here")
			return
		}
		cp := *d
		def = &cp
	}
	// A remnant's charter desk is dark: no new work until a reclaim.
	if s.remnant(at.At) {
		s.missionRefused(def.ID, def.Title, "This station is a remnant: its charter desk is dark")
		return
	}
	if s.standing+1e-9 < def.MinStanding {
		s.missionRefused(def.ID, def.Title,
			"Requires standing "+strconv.FormatFloat(def.MinStanding, 'f', 2, 64))
		return
	}
	for _, m := range s.missions {
		if m.ID == def.ID {
			s.missionRefused(def.ID, def.Title, "Offer already accepted")
			return
		}
	}
	s.missions = append(s.missions, MissionState{ID: def.ID, Def: def, Front: front})
	s.emitMissionEvent(MissionAccepted, def, SevGood, "Offer accepted: "+def.Title, 0)
}

// execAbandonMission drops accepted work — always the player's choice, never
// a failure (accepted work has no timer and no penalty). Like accept it
// requires being docked at the offering station.
func (s *Sim) execAbandonMission(c CmdAbandonMission) {
	p := s.player
	if !s.playerDocked() {
		s.missionRefused(c.ID, "", "You must be docked to abandon an offer")
		return
	}
	idx := -1
	for i, m := range s.missions {
		if m.ID == c.ID {
			idx = i
			break
		}
	}
	if idx < 0 {
		s.missionRefused(c.ID, "", "No active offer "+string(c.ID))
		return
	}
	def, ok := s.missionDef(&s.missions[idx])
	if !ok {
		// Stale save reference: drop it (nothing to announce — the content
		// no longer defines the offer and the save predates materialized
		// offers).
		s.missions = append(s.missions[:idx], s.missions[idx+1:]...)
		return
	}
	at, _ := s.Docked.Get(p)
	if def.Station != at.At {
		s.missionRefused(def.ID, def.Title, "Offer not offered here")
		return
	}
	s.missions = append(s.missions[:idx], s.missions[idx+1:]...)
	s.emitMissionEvent(MissionAbandoned, def, SevWarning, "Offer abandoned: "+def.Title, 0)
}

// emitMissionEvent queues an EvMission with the contract's ID and title.
func (s *Sim) emitMissionEvent(phase string, def *content.ContractDef, sev Severity, text string, value float64) {
	s.emit(Event{Kind: EvMission, Phase: phase, Severity: sev, Actor: s.player,
		ID: string(def.ID), Name: def.Title, Text: text, Value: value})
}

// missionRefused queues the refusal line for a rejected board command.
func (s *Sim) missionRefused(id content.ContractID, name, reason string) {
	s.emit(Event{Kind: EvMission, Phase: MissionRefused, Severity: SevWarning,
		Actor: s.player, ID: string(id), Name: name, Text: "Offer refused: " + reason})
}

// stationName resolves a station celestial ID to its display name (content
// lookup — the claim station may live in another system). Systems are
// scanned in sorted ID order so the result never depends on map order.
func (s *Sim) stationName(cel content.CelID) string {
	systems := make([]content.SystemID, 0, len(s.Content.Systems))
	for sys := range s.Content.Systems {
		systems = append(systems, sys)
	}
	sort.Slice(systems, func(i, j int) bool { return systems[i] < systems[j] })
	for _, sys := range systems {
		for i := range s.Content.Systems[sys].Celestials {
			c := &s.Content.Systems[sys].Celestials[i]
			if c.ID == cel {
				return c.Name
			}
		}
	}
	return string(cel)
}

// containsTrue reports whether any flag is set (claim trigger scan).
func containsTrue(flags []bool) bool {
	for _, f := range flags {
		if f {
			return true
		}
	}
	return false
}
