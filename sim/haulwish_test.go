package sim

import (
	"math"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// ---------------------------------------------------------------------------
// Haul-wish acceptance: wish-driven shipments (system_traffic.go's
// wish-consumption seam). An agent's cross-node arbitrage wish becomes a
// real convoy — goods sourced out of the agent's own stock at spawn, the
// cross-system journey through the shared ledger, delivery as agent-owned
// asks at the agent's reservation price, the haul fee settled through
// FulfillHaul — plus the stale-wish skip, the unpaid loss, cap sharing
// with the authored lanes, coexistence, and determinism with wishes
// active (twin runs + a post-restore continuation).
// ---------------------------------------------------------------------------

// wishContent is the end-to-end fixture: the cross-system delivery station
// (Dock Beta) carries a founding min-ferrite ask — the expensive end of
// the spread — sys-a's scheduled lanes are silent (the tests observe
// exactly their own shipments; the coexistence test switches one back on)
// and sys-b's raider is disarmed (ambushes have their own pins).
func wishContent() *content.Content {
	c := crossContent()
	c.Traffic["sys-a"].Routes = nil
	c.Traffic["sys-b"].Raider = ""
	sysB := c.Systems["sys-b"]
	for i := range sysB.Celestials {
		if sysB.Celestials[i].ID == "cel-b-dock" {
			sysB.Celestials[i].Station.Market = []content.MarketOrderDef{
				{Item: "min-ferrite", Qty: 500, Price: 50},
			}
		}
	}
	return c
}

// newWishSim builds the fixture with the player parked away from the lanes.
func newWishSim(t *testing.T, seed int64) *Sim {
	t.Helper()
	s, err := New(wishContent(), testWorld(), seed)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	setPlayerPos(s, core.Vec2{X: -5e6, Y: -5e6})
	drain(s)
	return s
}

// wishHaulers counts live convoys carrying an agent's wish shipment
// (Traffic.Owner set); authored lanes carry no owner tag.
func wishHaulers(s *Sim) int {
	n := 0
	s.Traffic.Each(func(_ ecs.Entity, tc *Traffic) {
		if tc.Owner != "" {
			n++
		}
	})
	return n
}

// seedWishAgent appends an industrialist holding qty of item (a
// market-only home keeps its behavior pass quoting-only: no jobs, no ore,
// no upkeep) and returns it after one behavior pass — the pass that turns
// its surplus into a resting ask and the station spread into a wish.
// The wish's size follows the documented rule (traderBaseQty scaled by
// the capital selector), which the pass's placement fee nudges — callers
// read it back from the wish rather than assuming the seeded stock.
func seedWishAgent(t *testing.T, s *Sim, id string, home StationID, item content.ItemID, qty int) *Agent {
	t.Helper()
	addTestAgent(s, id, RoleIndustrialist, home, 100_000)
	a := agentOf(s, id)
	addInv(a, item, qty)
	stepPasses(s, 1)
	return a
}

// wishQty is the size the pass authored into the wish (see seedWishAgent).
func wishQty(t *testing.T, s *Sim) int {
	t.Helper()
	ws := s.HaulWishes()
	if len(ws) != 1 {
		t.Fatalf("wishes = %+v, want exactly one", ws)
	}
	return ws[0].Qty
}

// ---------------------------------------------------------------------------
// End to end: wish → convoy → gate hop → delivery at the agent's ask
// ---------------------------------------------------------------------------

// TestHaulWishSpawnsConvoyAndDelivers pins the whole seam: the wish lists
// the agent's surplus at the cheap station, the poll launches a convoy
// that debits the agent's own ask at spawn (no conjuring), the shipment
// parks as an OWNER-TAGGED journey and resumes in the far system, the
// delivery posts agent-owned asks at the agent's reservation price with
// the market voice, and FulfillHaul charges exactly the pinned haul fee
// on the delivery tick (and nothing at spawn or in flight).
func TestHaulWishSpawnsConvoyAndDelivers(t *testing.T) {
	s := newWishSim(t, 9)
	wantOwner := agentOwnerTag("ag-haul")
	a := seedWishAgent(t, s, "ag-haul", "cel-a-dock", "min-ferrite", 20)

	// The pass quoted its surplus at home and the cross-system spread
	// became exactly one wish.
	wishes := s.HaulWishes()
	if len(wishes) != 1 {
		t.Fatalf("wishes = %+v, want exactly one min-ferrite spread", wishes)
	}
	w := wishes[0]
	// The pass sizes the wish with the documented rule (traderBaseQty ×
	// the capital selector) — the placement fee nudges the scale, so the
	// expected size comes from the formula, not a literal.
	wantQty := max(1, int(traderBaseQty*agentQtyScale(a)))
	if w.Owner != wantOwner || w.From != "cel-a-dock" || w.To != "cel-b-dock" ||
		w.Item != "min-ferrite" || w.Qty != wantQty || wantQty > 20 {
		t.Fatalf("wish = %+v, want %s cel-a-dock→cel-b-dock min-ferrite ×%d (stock 20): %+v",
			w, wantOwner, wantQty, w)
	}
	if q, p := askAt(s, "cel-a-dock", "min-ferrite"); q != 20 || p != 8.25 {
		t.Fatalf("origin ask = %d @ %v, want 20 @ 8.25 (reservation × margin)", q, p)
	}

	// The poll launches the convoy: the goods leave the agent's own ask at
	// the origin and its free stock, nothing moves on its capital.
	var evs []Event
	capAtSpawn := a.Capital
	if !runUntil(t, s, &evs, 4*trafficWishInterval, func() bool { return wishHaulers(s) == 1 }) {
		t.Fatalf("wish convoy never spawned; %v", trafficTexts(evs))
	}
	e, tc, ok := findHauler(s, "")
	if !ok {
		t.Fatal("no hauler on the grid")
	}
	if tc.Owner != wantOwner {
		t.Fatalf("hauler owner = %q, want %q", tc.Owner, wantOwner)
	}
	if tc.Dest != "cel-gate-b" || tc.DestSys != "sys-b" || tc.DestCel != "cel-b-dock" {
		t.Fatalf("leg1 = dest %q sys %q cel %q, want cel-gate-b/sys-b/cel-b-dock",
			tc.Dest, tc.DestSys, tc.DestCel)
	}
	if cargo, _ := s.Cargos.Get(e); stacksCount(cargo.Stacks, "min-ferrite") != wantQty {
		t.Fatalf("hauler cargo = %d, want %d (the wished units)", stacksCount(cargo.Stacks, "min-ferrite"), wantQty)
	}
	if q, _ := askAt(s, "cel-a-dock", "min-ferrite"); q != 0 {
		t.Fatalf("origin ask after spawn = %d, want 0 (sourced from the agent's own ask)", q)
	}
	if n := a.Inventory["min-ferrite"]; n != 20-wantQty {
		t.Fatalf("agent stock after spawn = %d, want %d (the wished units ride the hauler)",
			n, 20-wantQty)
	}
	if len(a.InFlight) != 1 || len(a.Wishes) != 1 {
		t.Fatalf("commitment = %d in flight / %d pending, want 1/1", len(a.InFlight), len(a.Wishes))
	}
	if a.Capital != capAtSpawn {
		t.Fatalf("capital at spawn = %v, want %v (spawn charges nothing)", a.Capital, capAtSpawn)
	}
	// The leg math the fee is pinned to: two systems, two warps.
	if legs := s.trafficWishLegs(w.From, w.To); legs != 2 {
		t.Fatalf("legs = %d, want 2 (sys-a → sys-b)", legs)
	}

	// Gate hop: the shipment parks as an owner-tagged journey…
	if !runUntil(t, s, &evs, 3000, func() bool { return len(s.trafficJourneys) == 1 }) {
		t.Fatalf("shipment never hopped; %v", trafficTexts(evs))
	}
	j := s.trafficJourneys[0]
	if j.Owner != wantOwner || j.Item != "min-ferrite" || j.Qty != wantQty ||
		j.System != "sys-b" || j.DestCel != "cel-b-dock" {
		t.Fatalf("parked journey = %+v, want owner %q to cel-b-dock carrying %d", j, wantOwner, wantQty)
	}
	// …the player follows it through the gate…
	gate := entityByName(s, "Gate to Beta")
	if gate.IsZero() {
		t.Fatal("no sys-a gate on grid")
	}
	setPlayerPos(s, gatePos(s, gate))
	s.Queue(CmdJump{Gate: gate})
	s.Step()
	evs = append(evs, drain(s)...)
	if s.systemID != "sys-b" {
		t.Fatalf("system = %q, want sys-b", s.systemID)
	}
	// Resume is capped and staggered (trafficResumeInterval): the parked
	// shipment flies within a few schedule windows — not necessarily on
	// the jump tick itself. No sys-b journey may stay parked once it has
	// resumed; journeys for the system the player left stay parked.
	if !runUntil(t, s, &evs, 600, func() bool {
		for _, j := range s.trafficJourneys {
			if j.System == "sys-b" {
				return false
			}
		}
		return true
	}) {
		t.Fatalf("sys-b journey never resumed: %+v", s.trafficJourneys)
	}
	_, tc2, ok := findHauler(s, "cel-b-dock")
	if !ok {
		t.Fatalf("resumed hauler missing; %v", trafficTexts(evs))
	}
	if tc2.Owner != wantOwner {
		t.Fatalf("resumed owner = %q, want %q (the tag rides the journey)", tc2.Owner, wantOwner)
	}

	// …and delivers. The agent quotes nothing while its goods are in
	// flight (no stock, no resting orders), so its capital is frozen: the
	// only movement on the delivery tick is the haul fee.
	wantAsk := quantizeTick(a.Beliefs["min-ferrite"] * (1 + a.Margin["min-ferrite"]))
	if wantAsk <= 0 {
		t.Fatal("fixture: agent has no reservation ask for min-ferrite")
	}
	var delivered *Event
	capBefore := a.Capital
	for i := 0; i < 6000 && delivered == nil; i++ {
		capBefore = a.Capital
		s.Step()
		evs = append(evs, drain(s)...)
		delivered = firstPhase(evs, EvTraffic, TrafficDelivered)
	}
	if delivered == nil {
		t.Fatalf("wish shipment never delivered; %v", trafficTexts(evs))
	}
	if delivered.System != "sys-b" || delivered.Cel != "cel-b-dock" {
		t.Fatalf("delivered = %q/%q, want sys-b/cel-b-dock", delivered.System, delivered.Cel)
	}
	// cost = 2.5 €/m³/leg × 2 legs × (0.1 m³ × qty), paid as a sink.
	wantCost := trafficHaulFee * 2 * s.itemVolume("min-ferrite") * float64(wantQty)
	if got := capBefore - a.Capital; math.Abs(got-wantCost) > 1e-9 {
		t.Fatalf("capital moved %v on the delivery tick, want %v (haul fee)", got, wantCost)
	}
	if len(a.InFlight) != 0 {
		t.Fatalf("in-flight = %+v, want none (settled)", a.InFlight)
	}
	for _, pending := range s.HaulWishes() {
		if sameWishShipment(pending, w) {
			t.Fatalf("settled wish still pending: %+v", pending)
		}
	}
	// The cargo lists at the destination under the agent's own ask.
	if q, p := askAt(s, "cel-b-dock", "min-ferrite"); q != wantQty || p != wantAsk {
		t.Fatalf("destination ask = %d @ %v, want %d @ %v (agent reservation)", q, p, wantQty, wantAsk)
	}
	o, ok := restOrder(s, "cel-b-dock", OrderSell, "min-ferrite", wantOwner)
	if !ok || o.Qty != wantQty || o.Price != wantAsk {
		t.Fatalf("delivered order = %+v (found %v), want %s owning %d @ %v", o, ok, wantOwner, wantQty, wantAsk)
	}
	if n := a.Inventory["min-ferrite"]; n != 20 {
		t.Fatalf("agent stock after delivery = %d, want 20 (escrowed in its ask)", n)
	}
	// The origin books were touched exactly once: the agent's ask at Dock
	// Two is gone, Haven Rest's founding stock was never touched.
	if q, p := askAt(s, "cel-station-a", "min-ferrite"); q != 500 || p != 10 {
		t.Fatalf("founding origin ask = %d @ %v, want 500 @ 10 (never touched)", q, p)
	}
	// The existing market voice reports the arriving supply.
	if !hasEvent(evs, EvMarket, "price falls to") {
		t.Fatalf("no market voice on the agent listing; %v", trafficTexts(evs))
	}
}

// ---------------------------------------------------------------------------
// Stale wish: the spread collapsed while waiting for a slot
// ---------------------------------------------------------------------------

// TestHaulWishStaleSpreadSkipsSpawn pins the re-validation: when the
// destination's price data is gone the poll leaves the wish pending (no
// hauler, no debit), and the next behavior pass prunes it.
func TestHaulWishStaleSpreadSkipsSpawn(t *testing.T) {
	s := newWishSim(t, 9)
	a := seedWishAgent(t, s, "ag-haul", "cel-a-dock", "min-ferrite", 20)
	if len(s.HaulWishes()) != 1 {
		t.Fatalf("fixture wishes = %+v, want one", s.HaulWishes())
	}

	// The spread collapses: Dock Beta's founding ask leaves the book, so
	// the net spread no longer covers the pinned haul fee.
	var askID uint64
	for _, o := range s.BookOrders("cel-b-dock") {
		if o.Item == "min-ferrite" && o.Side == OrderSell {
			askID = o.ID
		}
	}
	if askID == 0 || !s.CancelOrder("cel-b-dock", askID, OwnerSystem) {
		t.Fatal("fixture: destination ask not found")
	}
	drain(s)

	var evs []Event
	if runUntil(t, s, &evs, 4*trafficWishInterval, func() bool { return wishHaulers(s) > 0 }) {
		t.Fatal("stale wish spawned a convoy")
	}
	if len(s.HaulWishes()) != 1 {
		t.Fatalf("wishes after the skip = %+v, want the stale one left pending", s.HaulWishes())
	}
	// Nothing was sourced: the agent still holds its stock and its ask.
	if n := a.Inventory["min-ferrite"]; n != 20 {
		t.Fatalf("agent stock = %d, want 20 (no spawn, no debit)", n)
	}
	if q, p := askAt(s, "cel-a-dock", "min-ferrite"); q != 20 || p != 8.25 {
		t.Fatalf("origin ask = %d @ %v, want 20 @ 8.25 (untouched)", q, p)
	}
	if len(a.InFlight) != 0 {
		t.Fatalf("in-flight = %+v, want none", a.InFlight)
	}

	// The next behavior pass rebuilds from live spreads: the stale wish is
	// pruned (any survivor of the rebuild still cannot pay the fee).
	stepPasses(s, 1)
	for _, w := range s.HaulWishes() {
		if w.To == "cel-b-dock" {
			t.Fatalf("stale wish survived the pass: %+v", w)
		}
	}
	if runUntil(t, s, &evs, 3*trafficWishInterval, func() bool { return wishHaulers(s) > 0 }) {
		t.Fatal("a rebuilt wish spawned although its spread cannot pay the fee")
	}
	if n := a.Inventory["min-ferrite"]; n != 20 {
		t.Fatalf("agent stock after the second poll = %d, want 20", n)
	}
}

// TestHaulWishUnfundedWishSkippedAndPruned pins the funding rule: a wish
// whose own haul budget exceeds the agent's free capital is skipped by the
// poll (no convoy, nothing sourced) and pruned by the next behavior pass.
func TestHaulWishUnfundedWishSkippedAndPruned(t *testing.T) {
	s := newWishSim(t, 9)
	a := seedWishAgent(t, s, "ag-haul", "cel-a-dock", "min-ferrite", 20)
	if len(s.HaulWishes()) != 1 {
		t.Fatalf("fixture wishes = %+v, want one", s.HaulWishes())
	}
	// Free capital below the wish's own MaxCost (and below the pinned fee).
	a.Capital = dormantCapital + 1

	var evs []Event
	if runUntil(t, s, &evs, 4*trafficWishInterval, func() bool { return wishHaulers(s) > 0 }) {
		t.Fatal("unfunded wish spawned a convoy")
	}
	if len(s.HaulWishes()) != 1 {
		t.Fatalf("wishes after the skip = %+v, want it left pending", s.HaulWishes())
	}
	if n := a.Inventory["min-ferrite"]; n != 20 {
		t.Fatalf("agent stock = %d, want 20 (nothing sourced)", n)
	}
	stepPasses(s, 1)
	if n := len(s.HaulWishes()); n != 0 {
		t.Fatalf("wishes after the pass = %d, want 0 (pruned: unfundable)", n)
	}
}

// ---------------------------------------------------------------------------
// Loss: the goods die with the hauler, the agent is never paid
// ---------------------------------------------------------------------------

// TestHaulWishLostShipmentNoPayment pins the loss path: destroying the
// convoy emits the existing loss events, drops the wish WITHOUT
// FulfillHaul (capital untouched), keeps the inventory debit (the wreck
// carries the goods) and never lets the destination book see them.
func TestHaulWishLostShipmentNoPayment(t *testing.T) {
	s := newWishSim(t, 9)
	a := seedWishAgent(t, s, "ag-haul", "cel-a-dock", "min-ferrite", 20)
	wantQty := wishQty(t, s)
	var evs []Event
	if !runUntil(t, s, &evs, 4*trafficWishInterval, func() bool { return wishHaulers(s) == 1 }) {
		t.Fatalf("wish convoy never spawned; %v", trafficTexts(evs))
	}
	e, _, ok := findHauler(s, "")
	if !ok {
		t.Fatal("no hauler")
	}
	capBefore := a.Capital

	damageNPC(s, s.player, e, content.Damage{EM: 1e6}, false)
	s.Step()
	evs = drain(s)
	if firstPhase(evs, EvTraffic, TrafficDestroyed) == nil {
		t.Fatalf("no destroyed event; %v", trafficTexts(evs))
	}
	if !hasEvent(evs, EvMarket, "shipment lost en route to") {
		t.Fatalf("no supply-risk event; %v", trafficTexts(evs))
	}
	if a.Capital != capBefore {
		t.Fatalf("capital = %v, want %v (a lost shipment is never paid)", a.Capital, capBefore)
	}
	if len(s.HaulWishes()) != 0 || len(a.InFlight) != 0 {
		t.Fatalf("wishes = %+v / in-flight = %+v, want both empty (dropped unpaid)",
			s.HaulWishes(), a.InFlight)
	}
	if n := a.Inventory["min-ferrite"]; n != 20-wantQty {
		t.Fatalf("agent stock = %d, want %d (the shipped units went down with the hauler)",
			n, 20-wantQty)
	}
	wreck := wreckWithCargo(s, "min-ferrite")
	if wreck.IsZero() {
		t.Fatal("lost shipment left no wreck carrying the cargo")
	}
	loot, hasWreck := s.Wrecks.Get(wreck)
	got := -1
	if hasWreck {
		got = stacksCount(loot.Loot, "min-ferrite")
	}
	if got != wantQty {
		t.Fatalf("wreck cargo = %d, want %d (the wished units)", got, wantQty)
	}
	if q, p := askAt(s, "cel-b-dock", "min-ferrite"); q != 500 || p != 50 {
		t.Fatalf("destination ask = %d @ %v, want 500 @ 50 (nothing arrived)", q, p)
	}
	if q, _ := askAt(s, "cel-a-dock", "min-ferrite"); q != 0 {
		t.Fatalf("origin ask = %d, want 0 (no reversal of the spawn-time debit)", q)
	}
}

// ---------------------------------------------------------------------------
// Cap sharing and coexistence with the authored lanes
// ---------------------------------------------------------------------------

// TestHaulWishSharesLiveCap pins that wish convoys and authored lanes
// share one cap: wishes fill trafficMaxLive (one spawn per poll, canonical
// order) and the next wish waits for a slot instead of exceeding it.
func TestHaulWishSharesLiveCap(t *testing.T) {
	s := newWishSim(t, 9)
	for i := 1; i <= 7; i++ {
		addTestAgent(s, "ag-0"+itoaN(i), RoleIndustrialist, "cel-a-dock", 100_000)
	}
	for i := 1; i <= 7; i++ {
		addInv(agentOf(s, "ag-0"+itoaN(i)), "min-ferrite", 20)
	}
	stepPasses(s, 1)
	if n := len(s.HaulWishes()); n != 7 {
		t.Fatalf("wishes = %d, want 7 (one per agent)", n)
	}

	var evs []Event
	if !runUntil(t, s, &evs, 8*trafficWishInterval, func() bool {
		return wishHaulers(s) == trafficMaxLive
	}) {
		t.Fatalf("live wish convoys = %d, want %d", wishHaulers(s), trafficMaxLive)
	}
	if n := s.Traffic.Len(); n != trafficMaxLive {
		t.Fatalf("live haulers = %d, want %d", n, trafficMaxLive)
	}
	// The seventh wish waits: cap full, nothing over the line.
	stepN(s, 3*trafficWishInterval)
	if n := s.Traffic.Len(); n > trafficMaxLive {
		t.Fatalf("live haulers = %d, want <= %d", n, trafficMaxLive)
	}
	if n := wishHaulers(s); n != trafficMaxLive {
		t.Fatalf("wish convoys = %d, want %d (no 7th spawned)", n, trafficMaxLive)
	}
	if n := len(agentOf(s, "ag-07").InFlight); n != 0 {
		t.Fatalf("seventh agent in flight = %d, want 0", n)
	}
	if n := len(s.HaulWishes()); n != 7 {
		t.Fatalf("wishes = %d, want 7 (six shipping, one waiting)", n)
	}
}

// TestHaulWishCoexistsWithAuthoredLane pins that the wish seam never
// displaces the authored schedules: with a wish convoy in flight the
// interval lane still fires, still sources from its origin book and still
// delivers into the destination book — and the shared live cap is never
// exceeded at any tick.
func TestHaulWishCoexistsWithAuthoredLane(t *testing.T) {
	c := wishContent()
	// One authored lane back on: Haven Rest → Dock Two, min-ferrite.
	c.Traffic["sys-a"].Routes = []content.RouteDef{{
		From: "cel-station-a", To: "cel-a-dock", Hull: "ship-convoy",
		Cargo:         content.RouteCargo{Item: "min-ferrite", Qty: 100},
		IntervalTicks: 600,
	}}
	s, err := New(c, testWorld(), 9)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	setPlayerPos(s, core.Vec2{X: -5e6, Y: -5e6})
	drain(s)
	seedWishAgent(t, s, "ag-haul", "cel-a-dock", "min-ferrite", 20)

	var evs []Event
	var delivered *Event
	sawWish := false
	for i := 0; i < 6000 && !(sawWish && delivered != nil); i++ {
		s.Step()
		evs = append(evs, drain(s)...)
		if n := s.Traffic.Len(); n > trafficMaxLive {
			t.Fatalf("live haulers = %d, want <= %d (shared cap)", n, trafficMaxLive)
		}
		if wishHaulers(s) > 0 {
			sawWish = true
		}
		if delivered == nil {
			delivered = firstPhase(evs, EvTraffic, TrafficDelivered)
		}
	}
	if !sawWish {
		t.Fatalf("wish convoy never spawned; %v", trafficTexts(evs))
	}
	if !hasEvent(evs, EvTraffic, "bound for Dock Two") {
		t.Fatalf("authored lane never departed; %v", trafficTexts(evs))
	}
	if delivered == nil || delivered.Cel != "cel-a-dock" {
		t.Fatalf("authored delivery = %+v, want Dock Two", delivered)
	}
}

// ---------------------------------------------------------------------------
// Determinism
// ---------------------------------------------------------------------------

// TestHaulWishTwinRunsIdentical is the determinism gate with wishes
// active: two sims (same seed, same agent, same wish, same mid-flight
// jump) stay in lockstep — hashes at checkpoints AND event sequences —
// and a snapshot taken mid-journey restores into a third sim that then
// continues in lockstep with the live run (the owner tag and the agent's
// in-flight commitment both round-trip).
func TestHaulWishTwinRunsIdentical(t *testing.T) {
	newSim := func() *Sim {
		s, err := New(wishContent(), testWorld(), 21)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		setPlayerPos(s, core.Vec2{X: -5e6, Y: -5e6})
		addTestAgent(s, "ag-haul", RoleIndustrialist, "cel-a-dock", 100_000)
		addInv(agentOf(s, "ag-haul"), "min-ferrite", 20)
		drain(s)
		return s
	}
	step := func(s *Sim, evs *[]Event, hashes *[]uint64, from, to int) {
		for i := from; i < to; i++ {
			if i == 3000 {
				// The player follows the wish shipment through the gate.
				gate := entityByName(s, "Gate to Beta")
				if gate.IsZero() {
					t.Fatal("no sys-a gate on grid")
				}
				setPlayerPos(s, gatePos(s, gate))
				s.Queue(CmdJump{Gate: gate})
			}
			s.Step()
			*evs = append(*evs, drain(s)...)
			if i%1500 == 0 {
				*hashes = append(*hashes, s.Hash())
			}
		}
	}
	same := func(tag string, hA, hB []uint64, eA, eB []Event) {
		t.Helper()
		if len(hA) != len(hB) {
			t.Fatalf("%s: hash checkpoint counts differ (%d vs %d)", tag, len(hA), len(hB))
		}
		for i := range hA {
			if hA[i] != hB[i] {
				t.Fatalf("%s: checkpoint %d: %016x vs %016x", tag, i, hA[i], hB[i])
			}
		}
		if len(eA) != len(eB) {
			t.Fatalf("%s: event counts differ: %d vs %d", tag, len(eA), len(eB))
		}
		for i := range eA {
			a, b := eA[i], eB[i]
			if a.Kind != b.Kind || a.Phase != b.Phase || a.Text != b.Text ||
				a.Time != b.Time || a.System != b.System || a.Value != b.Value {
				t.Fatalf("%s: event %d differs:\n%+v\n%+v", tag, i, a, b)
			}
		}
	}

	// First leg: two fresh runs in lockstep while the wish convoy is in
	// flight (spawned, debited, flying toward the gate).
	a, b := newSim(), newSim()
	var evA, evB []Event
	var hA, hB []uint64
	step(a, &evA, &hA, 0, 2000)
	step(b, &evB, &hB, 0, 2000)
	same("twin (first leg)", hA, hB, evA, evB)
	if !hasEvent(evA, EvTraffic, "bound for Dock Beta") {
		t.Fatalf("no wish departure; %v", trafficTexts(evA))
	}
	if len(agentOf(a, "ag-haul").InFlight) != 1 {
		t.Fatalf("in-flight at the snapshot = %+v, want the committed shipment",
			agentOf(a, "ag-haul").InFlight)
	}

	// Post-restore twin: a fresh sim restores that mid-flight snapshot and
	// then continues in lockstep with the live run AND with the untouched
	// twin (gate hop, park/resume in sys-b, delivery, settlement).
	save, err := a.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	restored := newSim()
	if err := restored.Restore(save); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.Hash() != a.Hash() {
		t.Fatalf("hash after restore = %016x, want %016x", restored.Hash(), a.Hash())
	}
	var evA2, evB2, evR []Event
	var hA2, hB2, hR []uint64
	step(a, &evA2, &hA2, 2000, 9000)
	step(b, &evB2, &hB2, 2000, 9000)
	step(restored, &evR, &hR, 2000, 9000)
	same("twin (continuation)", hA2, hB2, evA2, evB2)
	same("post-restore", hA2, hR, evA2, evR)
	evA = append(evA, evA2...)

	// The run genuinely exercised the seam: the shipment parked
	// owner-tagged, the player's jump resumed it, and it delivered into
	// the agent's own ask with the fee settled.
	if firstPhase(evA, EvTraffic, TrafficJumped) == nil {
		t.Fatal("wish shipment never hopped the gate")
	}
	delivered := firstPhase(evA, EvTraffic, TrafficDelivered)
	if delivered == nil || delivered.Cel != "cel-b-dock" {
		t.Fatalf("wish delivery = %+v, want cel-b-dock", delivered)
	}
	if n := len(agentOf(a, "ag-haul").InFlight); n != 0 {
		t.Fatalf("in-flight = %d, want 0 (settled)", n)
	}
}
