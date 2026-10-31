package sim

// Contacts — dock-side mission-givers (CONTEXT.md "Contact", "Contact
// offer", "Requisition stock"). A contact represents a patron (the Charter,
// the Commission or a corporation) at one station, gated by the player's
// one career standing. Its offers are AUTHORED tasks whose parameters and
// urgency the sim derives from its own state on every evaluation — no RNG,
// no persistence, no wall-clock:
//
//	stock       a station's requisition stock of the objective item is below
//	            the offer's target; the count is the shortfall, so a completed
//	            delivery (which lands its cargo as stock) closes the offer.
//	rats        live Old Works hulls of the objective's class work the system.
//	war-supply  a front carries a supply gap; the offer names that front and
//	            its deficit, and completing it credits the front (warCredit).
//	war-clear   a charter-held front is overrun (meter at or past half); the
//	            offer asks for the works holding force at that front.
//
// Derivation is a pure function of sim state, so twin runs derive identical
// offers and two evaluations over one state are byte-identical.
// docs/CONTENT_AUTHORING.md (contacts) is the authoring contract.

import (
	"math"
	"sort"
	"strings"

	"github.com/entieve/entieve/content"
)

// Offer is one live contact offer: the materialized def (parameters filled),
// the contact it belongs to and the prose the offer speaks.
type Offer struct {
	Def         *content.ContractDef
	ContactName string
	Patron      string
	MinStanding float64
	Prose       string
	// Stock is the offer's requisition stock — the destination station's
	// resting asks for the objective item, in whole units (deliver offers).
	Stock int
	// Front is the war front a completed delivery credits ("" = none).
	Front content.SystemID
}

// ContactOffers derives every live offer at one station, contacts in id order
// and each contact's pool in authored order (canonical, deterministic).
func (s *Sim) ContactOffers(cel content.CelID) []Offer {
	var out []Offer
	for _, ct := range s.Content.ContactsAt(cel) {
		for i := range ct.Offers {
			if o, ok := s.deriveOffer(ct, &ct.Offers[i]); ok {
				out = append(out, o)
			}
		}
	}
	return out
}

// StationStock is the requisition stock of one item at a station: the units
// resting as asks on its book (what guard posts buy replacements from).
func (s *Sim) StationStock(station StationID, item content.ItemID) int {
	return s.marketAskQty(station, item)
}

// offerByID finds the live offer with id at the given station (the accept
// lookup: offers are re-derived, never stored before acceptance).
func (s *Sim) offerByID(cel content.CelID, id content.ContractID) (Offer, bool) {
	for _, o := range s.ContactOffers(cel) {
		if o.Def.ID == id {
			return o, true
		}
	}
	return Offer{}, false
}

// deriveOffer derives one authored offer against the current state; ok is
// false when its gap has closed (the offer's expiry rule).
func (s *Sim) deriveOffer(ct *content.ContactDef, of *content.ContactOffer) (Offer, bool) {
	switch of.Source.Kind {
	case "stock":
		return s.deriveStockOffer(ct, of)
	case "rats":
		return s.deriveRatsOffer(ct, of)
	case "war-supply":
		return s.deriveWarSupplyOffer(ct, of)
	case "war-clear":
		return s.deriveWarClearOffer(ct, of)
	}
	return Offer{}, false // validator rejects unknown kinds; fail closed
}

// deriveStockOffer derives a requisition-shortfall offer: the station's stock
// of the watched item (units × volume) against the authored target, in the
// objective's own unit. The delivery destination is the signal for deliver
// offers, the contact's own station for mine offers.
func (s *Sim) deriveStockOffer(ct *content.ContactDef, of *content.ContactOffer) (Offer, bool) {
	src := &of.Source
	station := ct.Station
	if of.Kind == content.ContractDeliver {
		station = of.Objective.Destination
	}
	volume := s.itemVolume(src.Item)
	have := float64(s.marketAskQty(station, src.Item))
	if volume > 0 {
		have *= volume
	}
	short := float64(src.Target) - have
	if short < 1 {
		return Offer{}, false // the gap is closed: no offer
	}
	count := int(math.Ceil(short))
	count = clampCount(count, src.Base, src.Max)
	def := s.materialize(ct, of, count, content.CelID(station))
	params := s.proseParams(ct, def, "", liveParams{count: count, urgency: gapWord(short, float64(src.Target))})
	return s.finishOfferWith(ct, def, station, params, "")
}

// deriveRatsOffer derives a kill offer from live Old Works hulls of the
// objective's class in the watched system. The count stays authored (the
// authored job is the job); the live hulls are what keeps it on the board.
func (s *Sim) deriveRatsOffer(ct *content.ContactDef, of *content.ContactOffer) (Offer, bool) {
	if s.systemID != of.Source.System {
		return Offer{}, false // the live grid is another system: nothing to watch
	}
	live := 0
	for _, e := range sortedEntities(s.AIs) {
		ai, ok := s.AIs.Get(e)
		if ok && ai.Npc == of.Objective.Target {
			live++
		}
	}
	if live == 0 {
		return Offer{}, false
	}
	def := s.materialize(ct, of, of.Objective.Count, of.Objective.Destination)
	params := s.proseParams(ct, def, of.Source.System, liveParams{count: of.Objective.Count, urgency: "standing"})
	return s.finishOfferWith(ct, def, ct.Station, params, "")
}

// warFrontGap is one front's supply gap: the euro-equivalent of materiel the
// charter still needs to move the meter (charter-held: to hold it back to
// zero; works-held: to push it to the threshold).
func warFrontGap(c *Contest, side string) float64 {
	if c == nil {
		return 0
	}
	if side == controlCharter {
		return c.Meter
	}
	return c.Threshold - c.Meter
}

// frontsByDistance orders fronts by distance from the contact's system, then
// by id — the canonical tie-break every derived geography read uses.
func (s *Sim) frontsByDistance(near content.SystemID) []content.SystemID {
	here := s.Content.Systems[near]
	ids := s.warContestIDs()
	sort.SliceStable(ids, func(i, j int) bool {
		di, dj := math.MaxFloat64, math.MaxFloat64
		if here != nil {
			if o := s.Content.Systems[ids[i]]; o != nil {
				di = here.Position.Dist(o.Position)
			}
			if o := s.Content.Systems[ids[j]]; o != nil {
				dj = here.Position.Dist(o.Position)
			}
		}
		if di != dj {
			return di < dj
		}
		return ids[i] < ids[j]
	})
	return ids
}

// deriveWarSupplyOffer derives a materiel offer from the nearest front with a
// supply gap: the count is the gap's worth of the objective item, the
// destination the front's station (the contact's own when the front is
// works-held), and completion credits that front.
func (s *Sim) deriveWarSupplyOffer(ct *content.ContactDef, of *content.ContactOffer) (Offer, bool) {
	if s.war == nil {
		return Offer{}, false
	}
	stationSys, _ := s.stationSystem(ct.Station)
	price := s.marketBasePrice(of.Objective.Item)
	if price <= 0 {
		return Offer{}, false
	}
	for _, id := range s.frontsByDistance(stationSys) {
		c := s.war.Contest[id]
		side := s.war.Control[id]
		gap := warFrontGap(c, side)
		if gap <= 1e-6 {
			continue
		}
		value := gap * supplyPerFlip * s.Squeeze()
		count := clampCount(int(math.Ceil(value/price)), of.Source.Base, of.Source.Max)
		dest := ct.Station
		if side == controlCharter {
			dest = s.firstStation(id)
		}
		def := s.materialize(ct, of, count, dest)
		params := s.proseParams(ct, def, id, liveParams{count: count, urgency: gapWord(gap, c.Threshold)})
		return s.finishOfferWith(ct, def, dest, params, id)
	}
	return Offer{}, false
}

// deriveWarClearOffer derives a clear-the-front offer from a charter-held
// front that is being overrun: the squeeze re-draws the objective's caste,
// the works' holding profile sets the count, and the prose names the front.
func (s *Sim) deriveWarClearOffer(ct *content.ContactDef, of *content.ContactOffer) (Offer, bool) {
	if s.war == nil {
		return Offer{}, false
	}
	stationSys, _ := s.stationSystem(ct.Station)
	sq := s.Squeeze()
	for _, id := range s.frontsByDistance(stationSys) {
		c := s.war.Contest[id]
		if s.war.Control[id] != controlCharter || c == nil || c.Meter < c.Threshold*0.5 {
			continue
		}
		count := clampCount(warProfileCount(sq), of.Source.Base, of.Source.Max)
		def := s.materialize(ct, of, count, "")
		def.Objective.Target = s.squeezeCaste(of.Objective.Target)
		params := s.proseParams(ct, def, id, liveParams{count: count, urgency: "critical"})
		return s.finishOfferWith(ct, def, ct.Station, params, "")
	}
	return Offer{}, false
}

// liveParams are the derivation's own prose facts that are not on the def.
type liveParams struct {
	count   int
	urgency string
}

// materialize copies an authored offer into the def the engine accepts:
// parameters filled, the contact's station and standing gate applied, and the
// reward scaled with the derived count (the authored count/reward calibration
// holds per unit).
func (s *Sim) materialize(ct *content.ContactDef, of *content.ContactOffer, count int, dest content.CelID) *content.ContractDef {
	def := of.ContractDef
	def.Station = ct.Station
	def.MinStanding = ct.MinStanding
	def.Objective.Count = count
	if dest != "" {
		def.Objective.Destination = dest
	}
	if of.Objective.Count > 0 && count != of.Objective.Count {
		def.Reward.Credits = of.Reward.Credits * float64(count) / float64(of.Objective.Count)
	}
	return &def
}

// finishOfferWith is the one offer constructor: prose fill, prose stored on
// the def (an accepted mission keeps the words it was offered with), and the
// destination's requisition stock read.
func (s *Sim) finishOfferWith(ct *content.ContactDef, def *content.ContractDef, station content.CelID, params map[string]string, front content.SystemID) (Offer, bool) {
	def.Description = fillProse(def.Description, params)
	o := Offer{
		Def:         def,
		ContactName: ct.Name,
		Patron:      ct.Patron,
		MinStanding: ct.MinStanding,
		Prose:       def.Description,
		Front:       front,
	}
	if def.Kind == content.ContractDeliver {
		o.Stock = s.marketAskQty(StationID(def.Objective.Destination), def.Objective.Item)
	}
	return o, true
}

// proseParams builds the slot values the authored prose reads. station is the
// claim station (deliver destination, else the contact's), system the
// derived/watched system name, caste/item the objective's display name.
func (s *Sim) proseParams(ct *content.ContactDef, def *content.ContractDef, system content.SystemID, live liveParams) map[string]string {
	if system == "" {
		system, _ = s.stationSystem(ct.Station)
	}
	station := ct.Station
	if def.Kind == content.ContractDeliver {
		station = def.Objective.Destination
	}
	item := ""
	if def.Objective.Item != "" {
		item = s.itemName(def.Objective.Item)
	}
	caste := ""
	if def.Objective.Target != "" {
		caste = string(def.Objective.Target)
		if npc, ok := s.Content.Npc(def.Objective.Target); ok {
			caste = npc.Name
		}
	}
	count := live.count
	if count == 0 {
		count = def.Objective.Count
	}
	urgency := live.urgency
	if urgency == "" {
		urgency = "standing"
	}
	return map[string]string{
		"{count}":   itoaN(count),
		"{system}":  s.warSystemName(system),
		"{station}": s.stationName(station),
		"{item}":    item,
		"{caste}":   caste,
		"{urgency}": urgency,
	}
}

// fillProse replaces the authored slots; an unknown slot is left verbatim
// (the validator's job to keep the authored set closed).
func fillProse(desc string, params map[string]string) string {
	pairs := make([]string, 0, len(params)*2)
	// Fixed order keeps the replacement deterministic regardless of map order.
	for _, k := range []string{"{count}", "{system}", "{station}", "{item}", "{caste}", "{urgency}"} {
		if v, ok := params[k]; ok {
			pairs = append(pairs, k, v)
		}
	}
	return strings.NewReplacer(pairs...).Replace(desc)
}

// gapWord names a gap's urgency in prose: the fraction of the target still
// missing, in three authored bands.
func gapWord(missing, target float64) string {
	if target <= 0 {
		return "standing"
	}
	f := missing / target
	switch {
	case f >= 2.0/3.0:
		return "critical"
	case f >= 1.0/3.0:
		return "urgent"
	}
	return "standing"
}

// clampCount keeps a derived count inside the authored window.
func clampCount(n, base, max int) int {
	if n < base {
		n = base
	}
	if max > 0 && n > max {
		n = max
	}
	if n < 1 {
		n = 1
	}
	return n
}

// firstStation is a system's first station in celestial order ("" when it has
// none — a fallen system's station is a remnant, still a station).
func (s *Sim) firstStation(sys content.SystemID) content.CelID {
	def := s.Content.Systems[sys]
	if def == nil {
		return ""
	}
	for i := range def.Celestials {
		if def.Celestials[i].Kind == content.CelStation {
			return def.Celestials[i].ID
		}
	}
	return ""
}
