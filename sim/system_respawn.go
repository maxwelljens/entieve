package sim

import (
	"math"
	"sort"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
)

const (
	// respawnRetryTicks is the requeue delay for a requisition that could
	// not be funded: 600 ticks (10 sim seconds), retried indefinitely (no
	// cap on retries) — a replacement never spawns from nothing.
	respawnRetryTicks = int64(600)
	// factionEndowment is the one-time seed of a faction's requisition
	// ledger, granted the first time that faction requisitions. It is
	// never refilled: every purchase debits it, so respawns are a real,
	// finite sink.
	factionEndowment = 100_000_000.0
	// ratOreDivisor: a rat replacement requisitions 1 m³ of belt ore per
	// ratOreDivisor mineral units of its hull recipe's bill — the bulk-ore
	// form of the approved "hullRecipe mineral equivalent" cost (the raw
	// bill in refined units would be ~50,000 m³ per hull; the belt hands
	// out unrefined rock).
	ratOreDivisor = 200.0
	// ratOreFallback is the ore cost of a rat hull with no recipe.
	ratOreFallback = 50.0
)

// systemRespawn fires pending respawn timers for the CURRENT system (only
// the live system's spawn groups tick; timers for other systems are kept
// untouched in the Save and resume when their system is loaded again).
// A timer whose requisition could not be funded re-queues itself 600 ticks
// later (no cap) instead of spawning.
func (s *Sim) systemRespawn() {
	if len(s.respawns) == 0 {
		return
	}
	kept := make([]RespawnTimer, 0, len(s.respawns))
	for _, t := range s.respawns {
		if t.System == s.systemID && s.time >= t.At {
			if s.respawnTimer(t) {
				t.At = s.time + float64(respawnRetryTicks)*DT
				kept = append(kept, t)
			}
			continue
		}
		kept = append(kept, t)
	}
	s.respawns = kept
}

// respawnTimer spawns one NPC for an expired timer near its celestial and
// emits exactly one info event when the NPC is actually created, so players
// and tests can observe respawn timers working.
//
// The replacement is funded (world metabolism R7 — nothing spawns from
// nowhere): a guard (NpcDef.IsGuard) BUYS its hull and fitted modules on
// the respawn station's book from its faction's ledger; a rat pays ore from
// the system's belt state, or buys local ore asks when the belt cannot pay.
// The 1000-round charge basket is likewise bought when listed (best effort:
// the hull spawns with whatever could be bought, possibly 0). An unfunded
// requisition spends nothing, warns, and returns true so the caller
// re-queues the timer. Traffic raiders never reach this path (recordRespawn
// excludes them).
func (s *Sim) respawnTimer(t RespawnTimer) bool {
	sys := s.System()
	if sys == nil {
		return false
	}
	var cel *content.CelestialDef
	for i := range sys.Celestials {
		if sys.Celestials[i].ID == t.Cel {
			cel = &sys.Celestials[i]
			break
		}
	}
	if cel == nil {
		return false // the celestial no longer exists: drop the timer
	}
	def, ok := s.Content.Npc(t.Npc)
	if !ok {
		return false
	}
	hull, ok := s.Content.Ship(def.Ship)
	if !ok {
		return false
	}
	if _, err := s.buildFitting(def.Fit, hull); err != nil {
		return false // can never spawn: drop instead of retrying forever
	}

	station := requisitionStation(sys, cel)
	if def.IsGuard() {
		cost, funded := s.requisitionGuard(def, station)
		if !funded {
			s.emitRequisitionDelayed(def, hull, station, cel, false)
			return true
		}
		s.emit(Event{Kind: EvLog, Severity: SevInfo, Actor: s.celEntity(cel.ID), Item: content.ItemID(def.Ship),
			Text:  def.Faction + " requisitioned a " + hull.Name + " at " + s.requisitionPlace(station, cel),
			Value: cost})
	} else {
		if !s.requisitionRat(t.System, def, station) {
			s.emitRequisitionDelayed(def, hull, station, cel, true)
			return true
		}
	}
	charges := s.buyChargeBasket(def, station)

	// The replacement flies in: it spawns off-grid on the anchor's +X axis
	// (WarpMinRange out, the undock side) and warps onto the exact spot a
	// static spawn would have used — the scatter rolls are taken around the
	// insertion point, then carried back, so the placement rolls and their
	// count are unchanged. Respawn still SPEAKS at timer time ("respawned
	// near …"); the hull is simply still in warp when it does.
	insert := core.Vec2{X: WarpMinRange}
	e := s.spawnNpc(t.Npc, cel.Position.Add(insert), celestialSpread(cel))
	if e.IsZero() {
		return false
	}
	if tr, ok := s.Transforms.Get(e); ok {
		// Land exactly on today's static placement (insert + scatter − insert).
		s.warpToPoint(e, tr.Pos.Sub(insert))
	}
	if ai, ok := s.AIs.Get(e); ok {
		ai.Home = cel.Position // the anchor, not the off-grid insertion point
	}
	// The hull carries exactly what the requisition bought — the free
	// default grant is replaced, possibly by no charges at all.
	if c, ok := s.Cargos.Get(e); ok {
		c.Stacks = charges
	}
	s.emit(Event{Kind: EvLog, Severity: SevInfo, Actor: e,
		Text: s.nameOf(e) + " respawned near " + cel.Name})
	return false
}

// emitRequisitionDelayed warns that a replacement could not be funded and
// was re-queued: "<faction> requisition delayed — no <hull> at <place>"
// for guards, the ore-qualified form for rats (place: the respawn station
// when the system has one, else the respawn celestial).
func (s *Sim) emitRequisitionDelayed(def *content.NpcDef, hull *content.ShipDef,
	station StationID, cel *content.CelestialDef, rat bool) {
	place := s.requisitionPlace(station, cel)
	text := def.Faction + " requisition delayed — no "
	if rat {
		text += "ore for "
	}
	s.emit(Event{Kind: EvLog, Severity: SevWarning, Actor: s.celEntity(cel.ID), Item: content.ItemID(def.Ship),
		Text: text + hull.Name + " at " + place})
}

// requisitionPlace names the venue of a requisition: the respawn station
// when the system has one, else the respawn celestial itself.
func (s *Sim) requisitionPlace(station StationID, cel *content.CelestialDef) string {
	if station != "" {
		return s.stationName(station)
	}
	return cel.Name
}

// requisitionStation is the station a replacement requisitions through: the
// respawn celestial itself when it is a market station, else the system's
// first market station in celestial order ("" when the system has none).
func requisitionStation(sys *content.SystemDef, cel *content.CelestialDef) StationID {
	if cel != nil && cel.Station != nil && hasMarketService(cel.Station) {
		return cel.ID
	}
	for i := range sys.Celestials {
		c := &sys.Celestials[i]
		if c.Station != nil && hasMarketService(c.Station) {
			return c.ID
		}
	}
	return ""
}

// requisitionGuard buys a guard replacement's hull and fitted modules on
// the respawn station's book (PlaceOrder buy, OwnerSystem): the orders
// cross resting asks, drain their stock and pay the faction ledger. It is
// all-or-nothing across stock and balance — nothing is spent unless every
// item can be bought — and reports the amount debited. Safe to plan-then-
// execute within one call: no other order can move between the two passes.
func (s *Sim) requisitionGuard(def *content.NpcDef, station StationID) (float64, bool) {
	if station == "" {
		return 0, false
	}
	book := s.market[station]
	if book == nil {
		return 0, false
	}
	// One line per distinct item: the hull first, then the fit in slot
	// order (duplicates aggregate into one order).
	type want struct {
		item content.ItemID
		qty  int
	}
	wants := []want{{content.ItemID(def.Ship), 1}}
	for _, mid := range def.Fit {
		found := false
		for i := range wants {
			if wants[i].item == content.ItemID(mid) {
				wants[i].qty++
				found = true
				break
			}
		}
		if !found {
			wants = append(wants, want{content.ItemID(mid), 1})
		}
	}
	budget := s.factionBalance(def.Faction)
	type planned struct {
		item  content.ItemID
		qty   int
		limit float64
		cost  float64
	}
	var buys []planned
	total := 0.0
	for _, w := range wants {
		ms := book.matchCrossing(OrderBuy, w.item, w.qty, 0)
		filled, cost := matchValue(ms)
		if filled < w.qty || total+cost > budget {
			return 0, false
		}
		buys = append(buys, planned{w.item, w.qty, ms[len(ms)-1].order.Price, cost})
		total += cost
	}
	for _, b := range buys {
		res, _ := s.PlaceOrder(station, OrderBuy, b.item, b.qty, b.limit, OwnerSystem)
		s.debitLedger(def.Faction, res.Cost)
	}
	return total, true
}

// requisitionRat funds a rat replacement with mass: ore drawn from the
// system's belt state, or — when the belt cannot pay in full — bought from
// the respawn station's local ore asks on the faction ledger. All-or-
// nothing: a failed requisition leaves belt and ledger untouched.
func (s *Sim) requisitionRat(sys content.SystemID, def *content.NpcDef, station StationID) bool {
	cost := s.ratOreCost(def)
	if s.beltOreAvailable(sys, cost) {
		return s.drawBeltOre(sys, cost)
	}
	return s.buyLocalOre(sys, station, def.Faction, cost)
}

// ratOreCost is the m³ of belt ore a rat replacement consumes: the hull
// recipe's total mineral bill at one m³ per ratOreDivisor units, rounded
// up (a fixed fallback for hulls with no recipe).
func (s *Sim) ratOreCost(def *content.NpcDef) float64 {
	rec := hullRecipe(s.Content, def.Ship)
	if rec == nil {
		return ratOreFallback
	}
	bill := 0.0
	for _, in := range rec.Inputs {
		bill += in.Qty
	}
	if bill <= 0 {
		return ratOreFallback
	}
	return math.Ceil(bill / ratOreDivisor)
}

// buyLocalOre buys `need` m³ of the system's belt ores from the station's
// asks, funded from the faction ledger. All-or-nothing: the plan is built
// first (canonical ore order, price-time within each ore) and nothing is
// executed unless the full amount is covered by stock and balance.
func (s *Sim) buyLocalOre(sys content.SystemID, station StationID, faction string, need float64) bool {
	if station == "" || need <= beltDepletedEpsilon {
		return false
	}
	book := s.market[station]
	if book == nil {
		return false
	}
	budget := s.factionBalance(faction)
	remaining := need
	type oreBuy struct {
		item  content.ItemID
		qty   int
		limit float64
		cost  float64
	}
	var plan []oreBuy
	for _, ore := range s.localOres(sys) {
		if remaining <= beltDepletedEpsilon {
			break
		}
		iv := s.mineralUnitVolume(ore)
		if iv <= 0 {
			continue
		}
		want := int(math.Ceil(remaining / iv))
		ms := book.matchCrossing(OrderBuy, ore, want, 0)
		if len(ms) == 0 {
			continue
		}
		b := oreBuy{item: ore}
		for _, m := range ms {
			take := min(m.qty, want-b.qty)
			if afford := int(budget / m.order.Price); take > afford {
				take = afford
			}
			if take <= 0 {
				break
			}
			b.qty += take
			b.cost += float64(take) * m.order.Price
			b.limit = m.order.Price
			budget -= float64(take) * m.order.Price
			if b.qty >= want {
				break
			}
		}
		if b.qty <= 0 {
			continue
		}
		plan = append(plan, b)
		remaining -= float64(b.qty) * iv
	}
	if remaining > beltDepletedEpsilon {
		return false // not covered: execute nothing
	}
	for _, b := range plan {
		res, _ := s.PlaceOrder(station, OrderBuy, b.item, b.qty, b.limit, OwnerSystem)
		s.debitLedger(faction, res.Cost)
	}
	return true
}

// localOres is the system's belt ore types — the ores a station may stock
// as "local ore" — in canonical order (sorted, unique).
func (s *Sim) localOres(sys content.SystemID) []content.ItemID {
	seen := make(map[content.ItemID]bool)
	var out []content.ItemID
	for _, k := range s.beltKeys() {
		if k.System != sys {
			continue
		}
		for _, r := range s.belts[k].Rocks {
			if r.Ore == "" || seen[r.Ore] {
				continue
			}
			seen[r.Ore] = true
			out = append(out, r.Ore)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// buyChargeBasket buys the standard 1000-round grant for each fitted
// weapon's ammo class (the spawn-time npcCharges list) when the station
// lists it, from the faction ledger. Best effort: returns the stacks
// actually bought — possibly none — and never blocks the spawn.
func (s *Sim) buyChargeBasket(def *content.NpcDef, station StationID) []content.CargoStack {
	if station == "" {
		return nil
	}
	book := s.market[station]
	if book == nil {
		return nil
	}
	var got []content.CargoStack
	for _, want := range s.npcCharges(def.Fit) {
		ms := book.matchCrossing(OrderBuy, want.Item, want.Qty, 0)
		if len(ms) == 0 {
			continue
		}
		budget := s.factionBalance(def.Faction)
		qty, cost, limit := 0, 0.0, 0.0
		for _, m := range ms {
			take := m.qty
			if afford := int(budget / m.order.Price); take > afford {
				take = afford
			}
			if take > want.Qty-qty {
				take = want.Qty - qty
			}
			if take <= 0 {
				break
			}
			qty += take
			cost += float64(take) * m.order.Price
			limit = m.order.Price
			budget -= float64(take) * m.order.Price
			if qty >= want.Qty {
				break
			}
		}
		if qty <= 0 {
			continue
		}
		res, ok := s.PlaceOrder(station, OrderBuy, want.Item, qty, limit, OwnerSystem)
		if !ok || res.Filled <= 0 {
			continue
		}
		s.debitLedger(def.Faction, res.Cost)
		got = append(got, content.CargoStack{Item: want.Item, Qty: res.Filled})
	}
	return got
}

// factionBalance returns a faction's requisition ledger, lazily seeding the
// one-time endowment on first use (pure state, no RNG).
func (s *Sim) factionBalance(faction string) float64 {
	if v, ok := s.factionLedger[faction]; ok {
		return v
	}
	if s.factionLedger == nil {
		s.factionLedger = make(map[string]float64)
	}
	s.factionLedger[faction] = factionEndowment
	return factionEndowment
}

// debitLedger subtracts one requisition purchase from a faction's balance.
func (s *Sim) debitLedger(faction string, amount float64) {
	if s.factionLedger == nil {
		s.factionLedger = make(map[string]float64)
	}
	s.factionLedger[faction] = s.factionBalance(faction) - amount
}
