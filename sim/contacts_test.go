package sim

// Contacts pins (docs/CONTENT_AUTHORING.md, contacts): offers are DERIVED
// from sim state with no RNG and no persistence, expiry is gap-driven,
// accepted work survives its gap closing, and a completed delivery lands its
// cargo as the destination's requisition stock.

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/entieve/entieve/content"
)

// contactContent is missionContent plus contacts:
//
//	contact-test-yard @ cel-station-a — a stock offer (deliver 10 m³ min-ferrite
//	                                    to cel-station-b, target 10 m³)
//	contact-test-front @ cel-station-a — a war-supply offer and a war-clear offer
func contactContent(t *testing.T) *content.Content {
	t.Helper()
	c := missionContent(t)
	// A founding market on the destination so its book exists (no asks:
	// stock starts at zero).
	for i := range c.Systems["sys-a"].Celestials {
		cel := &c.Systems["sys-a"].Celestials[i]
		if cel.ID == "cel-station-b" {
			cel.Station.Market = []content.MarketOrderDef{
				{Item: "min-ferrite", Qty: 50, Price: 9, Side: "buy"},
			}
		}
	}
	c.Contacts = map[content.ContactID]*content.ContactDef{
		"contact-test-yard": {
			ID: "contact-test-yard", Name: "Test Yard Office", Patron: content.PatronCorporation,
			Station: "cel-station-a",
			Offers: []content.ContactOffer{{
				ContractDef: content.ContractDef{
					ID: "offer-test-iron", Title: "Iron run", Kind: content.ContractDeliver, Level: 1,
					Objective:   content.ContractObjective{Item: "min-ferrite", Count: 1, Destination: "cel-station-b"},
					Reward:      content.ContractReward{Credits: 1000, StandingGain: 0.1},
					Description: "Bring {count} m³ of iron to {station} — {urgency}.",
				},
				Source: content.OfferSource{Kind: "stock", Item: "min-ferrite", Target: 10, Base: 1, Max: 10},
			}},
		},
		"contact-test-front": {
			ID: "contact-test-front", Name: "Test Front Liaison", Patron: content.PatronCommission,
			Station: "cel-station-a", MinStanding: 0,
			Offers: []content.ContactOffer{
				{
					ContractDef: content.ContractDef{
						ID: "offer-test-supply", Title: "Hold the line", Kind: content.ContractDeliver, Level: 2,
						Objective:   content.ContractObjective{Item: "ammo-emp-s", Count: 1},
						Reward:      content.ContractReward{Credits: 1000, StandingGain: 0.1},
						Description: "{count} rounds to {station} — the {system} front needs them ({urgency}).",
					},
					Source: content.OfferSource{Kind: "war-supply", Base: 1, Max: 100},
				},
				{
					ContractDef: content.ContractDef{
						ID: "offer-test-clear", Title: "Clear the front", Kind: content.ContractKill, Level: 2,
						Objective:   content.ContractObjective{Target: "npc-rat-scuttler", Count: 1},
						Reward:      content.ContractReward{Credits: 1000, StandingGain: 0.1},
						Description: "{count} {caste} at the {system} front.",
					},
					Source: content.OfferSource{Kind: "war-clear", Base: 1, Max: 4},
				},
			},
		},
	}
	return c
}

func newContactSim(t *testing.T) *Sim {
	t.Helper()
	s, err := New(contactContent(t), testWorld(), testSeed)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// warContactSim turns the war on and puts a contact at Haven Rest.
func warContactSim(t *testing.T) *Sim {
	t.Helper()
	c := warContent()
	c.Contacts = map[content.ContactID]*content.ContactDef{
		"contact-test-front": {
			ID: "contact-test-front", Name: "Test Front Liaison", Patron: content.PatronCommission,
			Station: "cel-station-a",
			Offers: []content.ContactOffer{{
				ContractDef: content.ContractDef{
					ID: "offer-test-supply", Title: "Hold the line", Kind: content.ContractDeliver, Level: 2,
					Objective:   content.ContractObjective{Item: "ammo-emp-s", Count: 1},
					Reward:      content.ContractReward{Credits: 1000, StandingGain: 0.1},
					Description: "{count} rounds to {station} — the {system} front needs them ({urgency}).",
				},
				Source: content.OfferSource{Kind: "war-supply", Base: 1, Max: 100},
			}},
		},
	}
	s, err := New(c, testWorld(), testSeed)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.newWarState()
	return s
}

func offerByID(t *testing.T, offers []Offer, id content.ContractID) Offer {
	t.Helper()
	for _, o := range offers {
		if o.Def.ID == id {
			return o
		}
	}
	t.Fatalf("no live offer %s in %+v", id, offers)
	return Offer{}
}

// TestConvertedPoolsShowAtTheirStations: every converted pool derives at
// least one live offer at its station at world start — no empty dock screen.
func TestConvertedPoolsShowAtTheirStations(t *testing.T) {
	c := content.MustLoad()
	s, err := New(c, c.Worlds["world-default"], 42)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, cel := range []content.CelID{
		"cel-doki", "cel-huta", "cel-haven-rest", "cel-penumbra-port",
	} {
		offers := s.ContactOffers(cel)
		if len(offers) == 0 {
			t.Errorf("%s: no live offers (empty dock screen)", cel)
			continue
		}
		for _, o := range offers {
			if _, ok := c.Contract(o.Def.ID); !ok {
				t.Errorf("%s: offer %s is not a content template", cel, o.Def.ID)
			}
		}
	}
}

// TestContactOfferGeneration: a stock shortfall produces the offer with the
// derived count and reward; filling the station's stock closes it (expiry is
// gap-driven, not a timer).
func TestContactOfferGeneration(t *testing.T) {
	s := newContactSim(t)

	offers := s.ContactOffers("cel-station-a")
	if len(offers) != 1 {
		t.Fatalf("offers = %+v, want the one stock offer", offers)
	}
	o := offers[0]
	if o.Def.ID != "offer-test-iron" {
		t.Fatalf("offer = %s, want offer-test-iron", o.Def.ID)
	}
	if o.Def.Objective.Count != 10 || o.Def.Objective.Destination != "cel-station-b" {
		t.Errorf("derived objective = %+v, want 10 m³ to cel-station-b", o.Def.Objective)
	}
	if want := 1000.0 * 10; o.Def.Reward.Credits != want {
		t.Errorf("credits = %v, want %v (scaled with the count)", o.Def.Reward.Credits, want)
	}
	if o.Stock != 0 {
		t.Errorf("stock = %d, want 0", o.Stock)
	}
	if o.ContactName != "Test Yard Office" || o.Patron != content.PatronCorporation {
		t.Errorf("contact = %q/%q", o.ContactName, o.Patron)
	}

	// The stock rises to the target: the gap is closed, the offer is gone.
	if _, ok := s.PlaceOrder("cel-station-b", OrderSell, "min-ferrite", 100, 12, OwnerSystem); !ok {
		t.Fatal("placing the stock ask failed")
	}
	if got := s.ContactOffers("cel-station-a"); len(got) != 0 {
		t.Fatalf("offers after the stock filled = %+v, want none", got)
	}
}

// TestContactOffersNameTheFront: a war front's supply gap derives a materiel
// offer that names that front's system and station; no war, or a closed gap,
// derives nothing.
func TestContactOffersNameTheFront(t *testing.T) {
	s := warContactSim(t)
	offers := s.ContactOffers("cel-station-a")
	if len(offers) != 1 {
		t.Fatalf("offers = %+v, want the front's materiel offer", offers)
	}
	o := offers[0]
	// sys-a is charter-held and idle; the works-held sys-w needs the push, so
	// the nearest gap is sys-a's own front (meter 0) — close it to zero room?
	// The works side always carries room, so the chosen front is sys-w.
	if o.Front != "sys-w" {
		t.Fatalf("front = %q, want sys-w (the works-held front)", o.Front)
	}
	if o.Def.Objective.Destination != "cel-station-a" {
		t.Errorf("destination = %q, want the contact's station for a works-held front", o.Def.Objective.Destination)
	}
	if o.Def.Objective.Count < 1 {
		t.Errorf("count = %d, want a positive gap count", o.Def.Objective.Count)
	}

	// Closing the chosen gap (works held, meter at the threshold) drops it.
	s.war.Contest["sys-w"].Meter = s.war.Contest["sys-w"].Threshold
	if got := s.ContactOffers("cel-station-a"); len(got) != 0 {
		t.Fatalf("offers with the gap closed = %+v, want none", got)
	}

	// No war at all: no derived offers.
	plain := newContactSim(t)
	if got := plain.ContactOffers("cel-station-a"); len(got) != 1 {
		t.Fatalf("stock-only fixture offers = %+v, want just the stock offer", got)
	}
}

// TestContactOffersDeterministic: two evaluations over the same state are
// byte-identical, and derivation never touches the RNG or the state hash.
func TestContactOffersDeterministic(t *testing.T) {
	s := warContactSim(t)
	before := s.Hash()
	a, _ := json.Marshal(s.ContactOffers("cel-station-a"))
	b, _ := json.Marshal(s.ContactOffers("cel-station-a"))
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("two derivations differ:\n%s\n%s", a, b)
	}
	if after := s.Hash(); after != before {
		t.Fatalf("derivation moved the state hash: %x -> %x", before, after)
	}
}

// TestContactOfferExpiryKeepsAcceptedWork: an accepted derived offer survives
// its gap closing (nothing fails, no timer), and still completes.
func TestContactOfferExpiryKeepsAcceptedWork(t *testing.T) {
	s := newContactSim(t)
	dockNamed(t, s, "Haven Rest")

	if evs := acceptMission(t, s, "offer-test-iron"); findMission(evs, MissionAccepted) == nil {
		t.Fatalf("accept refused: %v", evs)
	}
	// Pile in the cargo the delivery needs, then close the offer's gap.
	if c, ok := s.Cargos.Get(s.player); ok {
		c.Stacks = addStacks(c.Stacks, "min-ferrite", 100)
	}
	if _, ok := s.PlaceOrder("cel-station-b", OrderSell, "min-ferrite", 100, 12, OwnerSystem); !ok {
		t.Fatal("placing the stock ask failed")
	}
	if got := s.ContactOffers("cel-station-a"); len(got) != 0 {
		t.Fatalf("offers = %+v, want the gap's offer gone", got)
	}
	if len(s.Missions()) != 1 {
		t.Fatalf("accepted work dropped with the gap: %+v", s.Missions())
	}
	// The materialized offer rides the save: a restore keeps the task.
	sv, err := s.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(sv.Missions) != 1 || sv.Missions[0].Def == nil {
		t.Fatalf("save carries no materialized offer: %+v", sv.Missions)
	}
	if err := s.Restore(sv); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	// Still claimable at the destination.
	undock(t, s)
	evs := dockNamed(t, s, "Waypoint Two")
	if findMission(evs, MissionCompleted) == nil {
		t.Fatalf("accepted work never completed after the gap closed: %v", evs)
	}
}

// TestDeliveredCargoLandsAsRequisitionStock: completing a delivery raises the
// destination's requisition stock (the new world-effect hook) and credits the
// war front the work served.
func TestDeliveredCargoLandsAsRequisitionStock(t *testing.T) {
	s := newContactSim(t)
	dockNamed(t, s, "Haven Rest")
	if evs := acceptMission(t, s, "offer-test-iron"); findMission(evs, MissionAccepted) == nil {
		t.Fatalf("accept refused: %v", evs)
	}
	if c, ok := s.Cargos.Get(s.player); ok {
		c.Stacks = addStacks(c.Stacks, "min-ferrite", 100)
	}
	before := s.StationStock("cel-station-b", "min-ferrite")
	undock(t, s)
	evs := dockNamed(t, s, "Waypoint Two")
	if findMission(evs, MissionCompleted) == nil {
		t.Fatalf("delivery never completed: %v", evs)
	}
	after := s.StationStock("cel-station-b", "min-ferrite")
	if after != before+100 {
		t.Fatalf("requisition stock = %d, want %d (+100 delivered units)", after, before+100)
	}
}

// TestContactWarDeliveryCreditsTheFront: a war-supply completion credits the
// front it was derived from (the warCredit rail) and lands stock.
func TestContactWarDeliveryCreditsTheFront(t *testing.T) {
	s := warContactSim(t)
	s.war.Treasury = 1 // keep procurement quiet so only the delivery moves supply
	dockNamed(t, s, "Haven Rest")
	var id content.ContractID
	for _, o := range s.ContactOffers("cel-station-a") {
		if o.Front != "" {
			id = o.Def.ID
		}
	}
	if id == "" {
		t.Fatal("no war-supply offer live")
	}
	if evs := acceptMission(t, s, id); findMission(evs, MissionAccepted) == nil {
		t.Fatalf("accept refused: %v", evs)
	}
	m := s.Missions()[0]
	front := m.Front
	if front == "" {
		t.Fatal("accepted mission carries no front")
	}
	if c, ok := s.Cargos.Get(s.player); ok {
		c.Stacks = addStacks(c.Stacks, m.Def.Objective.Item, s.deliverUnits(m.Def))
	}
	// Dock at the claim station: the contact's own for a works-held front.
	undock(t, s)
	evs := dockNamed(t, s, "Haven Rest")
	if findMission(evs, MissionCompleted) == nil {
		t.Fatalf("war delivery never completed: %v", evs)
	}
	if got := s.war.Contest[front].SupplyPending; got <= 0 {
		t.Fatalf("front %s supply pending = %v, want the delivery credited", front, got)
	}
}

// TestStaleMissionIdDegrades: a pre-contacts save carrying a retired offer id
// (no materialized def) is dropped at the claim scan — never paid, never a
// crash.
func TestStaleMissionIdDegrades(t *testing.T) {
	s := newContactSim(t)
	dockNamed(t, s, "Haven Rest")
	sv, err := s.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	sv.Missions = []MissionState{{ID: "contract-retired"}}
	if err := s.Restore(sv); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	s.dockedThisTick = "cel-station-a"
	s.Step()
	if len(s.Missions()) != 0 {
		t.Fatalf("stale mission survived: %+v", s.Missions())
	}
	for _, ev := range drain(s) {
		if ev.Kind == EvMission && ev.Phase == MissionCompleted {
			t.Fatalf("stale mission paid out: %+v", ev)
		}
	}
}
