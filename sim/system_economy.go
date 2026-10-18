package sim

import (
	"math"
	"slices"
	"sort"
	"strconv"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
)

// repairCostPerHP is the station repair fee in euro per missing hit
// point across shield, armor and structure (documented simplification).
const repairCostPerHP = 2

// ---------------------------------------------------------------------------
// Market constants — the institutional load on every trade (EVE-like,
// tunable, stateless). Together a round trip costs 5.5% of trade value,
// inside the 5-11% target band.
// ---------------------------------------------------------------------------

const (
	// MarketBrokerFee is charged on the placed value of a buy order at
	// placement (EVE's broker fee). It is not refunded on cancel — the
	// escrow itself is, in full, for whatever never trades.
	MarketBrokerFee = 0.03
	// MarketSalesTax is charged on the executed value of a sell at
	// execution (EVE's sales tax).
	MarketSalesTax = 0.025
	// marketTickDigits is EVE's price-tick rule: every order price carries
	// four significant figures (48 ticks in 0.01 steps, 195000 in 100s).
	marketTickDigits = 4
	// maxPlayerOrders is the player's standing-order slot cap, universe-
	// wide (EVE-style order slots): a 21st open order is refused. It bounds
	// the bookkeeping the UI renders and, with the non-refundable broker
	// fee, the free-for-all price spam the free market allows.
	maxPlayerOrders = 20
)

// TickStep is the price-tick step at p — one unit of the 4-significant-
// figure grid every resting order price lands on. The market UI steps its
// price stepper by it; tickDown undercuts with it. Non-positive, NaN or
// infinite p reports 0 (no step).
func TickStep(p float64) float64 {
	q := quantizeTick(p)
	if q <= 0 || math.IsNaN(q) || math.IsInf(q, 0) {
		return 0
	}
	return math.Pow(10, math.Floor(math.Log10(q))-(marketTickDigits-1))
}

// quantizeTick rounds a price onto the 4-significant-figure tick grid: the
// only price an order may rest at. Non-positive prices pass through
// untouched (they are refused at placement).
func quantizeTick(p float64) float64 {
	if p <= 0 || math.IsNaN(p) || math.IsInf(p, 0) {
		return p
	}
	q, err := strconv.ParseFloat(strconv.FormatFloat(p, 'g', marketTickDigits, 64), 64)
	if err != nil || q <= 0 {
		return p
	}
	return q
}

// tickDown returns the next tick strictly below p: the undercut step a
// fresh seller (convoy supply) lists at.
func tickDown(p float64) float64 {
	q := quantizeTick(p)
	if q <= 0 {
		return q
	}
	return quantizeTick(q - TickStep(q))
}

// systemEconomy executes deferred station-service commands (only while
// docked at a station offering the required service).
func (s *Sim) systemEconomy() {
	for _, cmd := range s.deferEcon {
		switch c := cmd.(type) {
		case CmdBuy:
			s.execBuy(c)
		case CmdSell:
			s.execSell(c)
		case CmdPlaceOrder:
			s.execPlaceOrder(c)
		case CmdCancelOrder:
			s.execCancelOrder(c)
		case CmdRefine:
			s.execRefine(c)
		case CmdRepair:
			s.execRepair()
		case CmdFitModule:
			s.execFit(c)
		case CmdUnfitModule:
			s.execUnfit(c)
		case CmdAssemble:
			s.execAssemble(c)
		}
	}
	s.deferEcon = nil
}

// stationAccess validates that the player is docked at a station offering
// the given service; it warns and reports false otherwise.
func (s *Sim) stationAccess(svc string) (ecs.Entity, bool) {
	if !s.playerDocked() {
		s.log(SevWarning, s.player, "You must be docked to use station services")
		return 0, false
	}
	st := s.dockedStation()
	if st.IsZero() {
		s.log(SevWarning, s.player, "No station services here")
		return 0, false
	}
	station, ok := s.Stations.Get(st)
	if !ok || !hasService(station, svc) {
		s.log(SevWarning, s.player, "This station does not offer "+svc)
		return 0, false
	}
	return st, true
}

// ---------------------------------------------------------------------------
// Market — per-station continuous double auction (the price former)
// ---------------------------------------------------------------------------

// dockedStationID returns the celestial id of the station the player is
// docked at ("" when docked nowhere) — the station stationAccess just
// validated for the market service.
func (s *Sim) dockedStationID() StationID {
	d, ok := s.Docked.Get(s.player)
	if !ok {
		return ""
	}
	return d.At
}

// stationSystem resolves a station celestial id to the system it lives in.
// Station ids are unique universe-wide (content validation), so sorted
// iteration never changes the answer — only keeps the lookup reproducible.
func (s *Sim) stationSystem(station StationID) (content.SystemID, bool) {
	ids := make([]content.SystemID, 0, len(s.Content.Systems))
	for id := range s.Content.Systems {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		for _, cel := range s.Content.Systems[id].Celestials {
			if cel.ID == station && cel.Station != nil {
				return id, true
			}
		}
	}
	return "", false
}

// hasMarketService reports whether a content station offers the market
// service (the sim-component helper reads live Station components; this is
// the definition side, for celestials that may be off-grid).
func hasMarketService(st *content.StationDef) bool {
	for _, svc := range st.Services {
		if svc == "market" {
			return true
		}
	}
	return false
}

// marketPrice is the price ladder for an item at a book: last trade price,
// then best ask, then best bid, then the def base price (ItemDef.BasePrice
// falling back to ModuleDef.Price; ships have no base price and report 0).
func (s *Sim) marketPrice(book *MarketBook, item content.ItemID) float64 {
	if book != nil {
		if p, ok := book.lastPrice(item); ok {
			return p
		}
		if o, ok := book.bestAsk(item); ok {
			return o.Price
		}
		if o, ok := book.bestBid(item); ok {
			return o.Price
		}
	}
	return s.marketBasePrice(item)
}

// marketVoice emits the market's price line when an execution or a
// posting moved an item's displayed price: "price falls to" under surplus
// supply, "price rises to" under demand, both stamped with the station.
func (s *Sim) marketVoice(book *MarketBook, item content.ItemID, before float64, where string) {
	after := s.marketPrice(book, item)
	if before <= 0 || after <= 0 || after == before {
		return
	}
	verb, direction := "Surplus", "falls to"
	if after > before {
		verb, direction = "Demand", "rises to"
	}
	s.emit(Event{Kind: EvMarket, Severity: SevInfo, System: book.System, Item: item, Value: after,
		Text: verb + " — " + s.itemName(item) + " price " + direction + " " +
			euro(after) + " at " + where})
}

// matchValue sums the executed quantity and value of a fill set.
func matchValue(ms []match) (qty int, value float64) {
	for _, m := range ms {
		qty += m.qty
		value += m.order.Price * float64(m.qty)
	}
	return qty, value
}

// settle applies a fill set to the book, settles the RESTING side's
// obligations (a player's filled bid receives its goods into the hangar, a
// player's filled ask is paid the proceeds minus the sales tax) and
// records the last trade on the price ladder. The incoming side settles
// itself in execBuy/execSell/PlaceOrder. Returns quantity and value.
func (s *Sim) settle(book *MarketBook, ms []match) (int, float64) {
	qty, value := matchValue(ms)
	if qty == 0 {
		return 0, 0
	}
	book.applyMatches(ms)
	if book.Lasts == nil {
		book.Lasts = map[content.ItemID]float64{}
	}
	// Last traded unit: price-time priority's final (worst) fill.
	book.Lasts[ms[0].order.Item] = ms[len(ms)-1].order.Price
	for _, m := range ms {
		if m.order.Owner != OwnerPlayer {
			continue
		}
		if m.order.Side == OrderBuy {
			s.storePlayerStock(m.order.Item, m.qty)
			v := m.order.Price * float64(m.qty)
			s.emit(Event{Kind: EvMarket, Severity: SevGood, Actor: s.player, Item: m.order.Item,
				Text: "Bought " + itoaN(m.qty) + " x " + s.itemName(m.order.Item) + " for " +
					euro(v), Value: v})
			continue
		}
		v := m.order.Price * float64(m.qty)
		tax := MarketSalesTax * v
		received := v - tax
		s.wallet += received
		s.emit(Event{Kind: EvMarket, Severity: SevGood, Actor: s.player, Item: m.order.Item,
			Text: "Sold " + itoaN(m.qty) + " x " + s.itemName(m.order.Item) + " for " +
				euro(received) + " (" + euro(tax) + " sales tax)",
			Value: received})
	}
	return qty, value
}

// marketWithdraw removes up to qty units of item from a station's asks in
// price-time order — the origin-side fulfillment of a shipment (a convoy
// takes what the origin station had for sale). It is a transfer, not a
// trade: no trade price prints at the origin. Returns the units taken and
// what they cost (0 when the station carries no stock of it).
func (s *Sim) marketWithdraw(station StationID, item content.ItemID, qty int) (int, float64) {
	book := s.market[station]
	if book == nil || qty <= 0 {
		return 0, 0
	}
	ms := book.matchCrossing(OrderBuy, item, qty, 0)
	taken, paid := matchValue(ms)
	book.applyMatches(ms)
	return taken, paid
}

// playerStock counts the units of item the player can sell: general hold,
// ore hold, hangar stacks, then stored hulls (one unit per hull).
func (s *Sim) playerStock(item content.ItemID) int {
	n := 0
	if cargo, ok := s.Cargos.Get(s.player); ok {
		n += stacksCount(cargo.Stacks, item) + stacksCount(cargo.Ore, item)
	}
	n += stacksCount(s.Hangar.Items, item)
	for _, sh := range s.Hangar.Ships {
		if sh.Hull == content.ShipID(item) {
			n++
		}
	}
	return n
}

// takePlayerStock removes up to qty units in sell order — general hold,
// ore hold, hangar, stored hulls (one StoredShip per hull, the route
// execSell has always taken) — and returns how many it got.
func (s *Sim) takePlayerStock(item content.ItemID, qty int) int {
	taken := 0
	if cargo, ok := s.Cargos.Get(s.player); ok && taken < qty {
		var n int
		cargo.Stacks, n = takeStacks(cargo.Stacks, item, qty-taken)
		taken += n
	}
	if cargo, ok := s.Cargos.Get(s.player); ok && taken < qty {
		var n int
		cargo.Ore, n = takeStacks(cargo.Ore, item, qty-taken)
		taken += n
	}
	if taken < qty {
		var n int
		s.Hangar.Items, n = takeStacks(s.Hangar.Items, item, qty-taken)
		taken += n
	}
	if taken < qty {
		kept := s.Hangar.Ships[:0]
		for _, sh := range s.Hangar.Ships {
			if taken < qty && sh.Hull == content.ShipID(item) {
				taken++
				continue
			}
			kept = append(kept, sh)
		}
		s.Hangar.Ships = kept
	}
	return taken
}

// storePlayerStock puts units into the station hangar: the delivery side
// of a purchase and the refund of a cancelled sell order (hulls become
// fresh StoredShips — market goods never carry a fit).
func (s *Sim) storePlayerStock(item content.ItemID, qty int) {
	if qty <= 0 {
		return
	}
	if hull, ok := s.Content.Ship(content.ShipID(item)); ok {
		for range qty {
			s.Hangar.Ships = append(s.Hangar.Ships, StoredShip{Hull: hull.ID})
		}
		return
	}
	s.Hangar.Items = addStacks(s.Hangar.Items, item, qty)
}

// execBuy fills CmdBuy as an immediate-or-cancel market order against the
// docked station's book: price-time priority at the resting orders'
// prices, a broker fee on the executed value, nothing rests — whatever
// the book cannot fill simply stays with the buyer (never conjured).
func (s *Sim) execBuy(c CmdBuy) {
	if _, ok := s.stationAccess("market"); !ok {
		return
	}
	if c.Qty <= 0 {
		s.log(SevWarning, s.player, "Invalid quantity")
		return
	}
	station := s.dockedStationID()
	book := s.market[station]
	var ms []match
	if book != nil {
		ms = book.matchCrossing(OrderBuy, c.Item, c.Qty, 0)
	}
	if len(ms) == 0 {
		s.log(SevWarning, s.player, s.itemName(c.Item)+" is not sold here")
		return
	}
	qty, cost := matchValue(ms)
	fee := MarketBrokerFee * cost
	if s.wallet < cost+fee {
		s.emit(Event{Kind: EvMarket, Severity: SevWarning, Actor: s.player, Item: c.Item,
			Text: "Insufficient funds for " + s.itemName(c.Item), Value: cost + fee})
		return
	}
	before := s.marketPrice(book, c.Item)
	s.wallet -= cost + fee
	s.settle(book, ms)
	s.storePlayerStock(c.Item, qty)
	s.emit(Event{Kind: EvMarket, Severity: SevGood, Actor: s.player, Item: c.Item,
		Text: "Bought " + itoaN(qty) + " x " + s.itemName(c.Item) + " for " + euro(cost) +
			" (" + euro(fee) + " broker fee)",
		Value: cost})
	s.marketVoice(book, c.Item, before, s.stationName(station))
}

// execSell fills CmdSell as an immediate-or-cancel market order: it takes
// only what the station's bids actually buy (nothing rests), credits the
// bids' prices minus the sales tax, and removes exactly the filled
// quantity from the general hold, then the ore hold, then the hangar —
// stored hulls sell one at a time.
func (s *Sim) execSell(c CmdSell) {
	if _, ok := s.stationAccess("market"); !ok {
		return
	}
	if c.Qty <= 0 {
		s.log(SevWarning, s.player, "Invalid quantity")
		return
	}
	station := s.dockedStationID()
	book := s.market[station]
	avail := s.playerStock(c.Item)
	if avail == 0 {
		s.log(SevWarning, s.player, "Nothing to sell")
		return
	}
	var ms []match
	if book != nil {
		ms = book.matchCrossing(OrderSell, c.Item, min(c.Qty, avail), 0)
	}
	if len(ms) == 0 {
		s.log(SevWarning, s.player, "No buy orders for "+s.itemName(c.Item))
		return
	}
	qty, _ := matchValue(ms)
	if taken := s.takePlayerStock(c.Item, qty); taken != qty {
		// Cannot happen: the count and the take run back-to-back in one
		// tick. Restore what was taken and refuse rather than credit
		// money for goods the player does not have.
		s.storePlayerStock(c.Item, taken)
		s.log(SevWarning, s.player, "Sell count mismatch")
		return
	}
	before := s.marketPrice(book, c.Item)
	_, proceeds := s.settle(book, ms)
	// Institutional load on the sell side: broker fee on the placed value
	// plus the sales tax on execution (both come out of the proceeds).
	fee := MarketBrokerFee * proceeds
	tax := MarketSalesTax * proceeds
	received := proceeds - tax - fee
	s.wallet += received
	s.emit(Event{Kind: EvMarket, Severity: SevGood, Actor: s.player, Item: c.Item,
		Text: "Sold " + itoaN(qty) + " x " + s.itemName(c.Item) + " for " + euro(received) +
			" (" + euro(fee) + " broker fee, " + euro(tax) + " sales tax)",
		Value: received})
	s.marketVoice(book, c.Item, before, s.stationName(station))
}

// playerOrderCount counts the player's open resting orders across every
// station book — the universe-wide order-slot cap (maxPlayerOrders). Books
// iterate in sorted station order, so the count is deterministic.
func (s *Sim) playerOrderCount() int {
	n := 0
	for _, st := range s.stationsSorted() {
		book := s.market[st]
		if book == nil {
			continue
		}
		for _, o := range book.Orders {
			if o.Owner == OwnerPlayer {
				n++
			}
		}
	}
	return n
}

// execPlaceOrder posts a standing limit order (bid or ask) for the player
// on the docked station's book — the book the market window shows, the
// same reach CmdBuy/CmdSell have. Strict validation up front (every
// refusal a SevWarning log line, no state touched); the placement itself
// rides the existing PlaceOrder with owner=OwnerPlayer, which escrows the
// wallet (bid: price×qty + 3% broker fee) or the goods (ask) and emits the
// fill and placement lines. Any positive price is legal.
func (s *Sim) execPlaceOrder(c CmdPlaceOrder) {
	if _, ok := s.stationAccess("market"); !ok {
		return
	}
	if c.Side != OrderBuy && c.Side != OrderSell {
		s.log(SevWarning, s.player, "Invalid order side")
		return
	}
	if c.Qty < 1 {
		s.log(SevWarning, s.player, "Invalid quantity")
		return
	}
	if c.Price <= 0 || math.IsNaN(c.Price) || math.IsInf(c.Price, 0) {
		s.log(SevWarning, s.player, "Invalid price")
		return
	}
	// The item must exist: a book never rests a phantom id. Items, modules
	// and hulls are all addressable as ids (as in execTransfer).
	if _, ok := s.Content.Item(c.Item); !ok {
		if _, ok := s.Content.Module(content.ModuleID(c.Item)); !ok {
			if _, ok := s.Content.Ship(content.ShipID(c.Item)); !ok {
				s.log(SevWarning, s.player, "Unknown item "+string(c.Item))
				return
			}
		}
	}
	if s.playerOrderCount() >= maxPlayerOrders {
		s.log(SevWarning, s.player, "Maximum number of open orders reached ("+itoaN(maxPlayerOrders)+")")
		return
	}

	// Clear reasons for the two things PlaceOrder would otherwise reject
	// without a word: the escrow arithmetic mirrors its own check (the
	// placed value + the broker fee on it, EVE's rule) read from the same
	// constants, so it can never drift ahead of or behind the engine.
	station := s.dockedStationID()
	limit := quantizeTick(c.Price)
	switch c.Side {
	case OrderBuy:
		if s.wallet < limit*float64(c.Qty)*(1+MarketBrokerFee) {
			s.emit(Event{Kind: EvMarket, Severity: SevWarning, Actor: s.player, Item: c.Item,
				Text:  "Insufficient funds for " + s.itemName(c.Item),
				Value: limit * float64(c.Qty)})
			return
		}
	case OrderSell:
		if _, isShip := s.Content.Ship(content.ShipID(c.Item)); isShip {
			s.log(SevWarning, s.player, "Fitted hulls cannot rest in a sell order")
			return
		}
		if s.wallet < MarketBrokerFee*limit*float64(c.Qty) {
			s.emit(Event{Kind: EvMarket, Severity: SevWarning, Actor: s.player, Item: c.Item,
				Text:  "Insufficient funds for the order fee",
				Value: MarketBrokerFee * limit * float64(c.Qty)})
			return
		}
		if s.playerStock(c.Item) < c.Qty {
			s.log(SevWarning, s.player, "Not enough "+s.itemName(c.Item)+" to list")
			return
		}
	}

	before := s.marketPrice(s.market[station], c.Item)
	if _, ok := s.PlaceOrder(station, c.Side, c.Item, c.Qty, c.Price, OwnerPlayer); !ok {
		s.log(SevWarning, s.player, "Order refused")
		return
	}
	// A crossing placement moved the book: print the price voice exactly
	// like execBuy/execSell do (no-op when nothing traded).
	s.marketVoice(s.market[station], c.Item, before, s.stationName(station))
}

// execCancelOrder cancels one of the player's resting orders on the docked
// station's book by arrival id. The unspent escrow comes back (a bid's
// leftover wallet, an ask's goods into the station hangar); the placement
// broker fee stays — EVE's rule, kept by CancelOrder.
func (s *Sim) execCancelOrder(c CmdCancelOrder) {
	if _, ok := s.stationAccess("market"); !ok {
		return
	}
	if c.ID == 0 || !s.CancelOrder(s.dockedStationID(), c.ID, OwnerPlayer) {
		s.log(SevWarning, s.player, "No such order")
		return
	}
	s.emit(Event{Kind: EvMarket, Severity: SevInfo, Actor: s.player, Text: "Order cancelled"})
}

// PlaceOrder posts a (limit) order on a station's book — the seam agents
// and later workstreams build on. It crosses every resting order it can
// first (price-time priority, always at the RESTING order's price) and
// only the unfilled remainder rests, so a book never crosses itself.
//
// Escrow: a player's buy debits wallet by price×qty + broker fee up front
// and refunds the escrow of whatever never trades (the fee stays, EVE's
// rule); a player's sell escrows the goods themselves (hulls are refused —
// a fitted hull cannot ride in an order). Any other owner is the system
// side: no wallet, no inventory, the book just moves.
func (s *Sim) PlaceOrder(station StationID, side OrderSide, item content.ItemID, qty int, price float64, owner string) (OrderResult, bool) {
	res := OrderResult{}
	if qty <= 0 || price <= 0 {
		return res, false
	}
	if side != OrderBuy && side != OrderSell {
		return res, false
	}
	sys, ok := s.stationSystem(station)
	if !ok {
		return res, false
	}
	book := s.market[station]
	if book == nil {
		book = &MarketBook{System: sys, Lasts: map[content.ItemID]float64{}}
		s.market[station] = book
	}
	limit := quantizeTick(price)
	ms := book.matchCrossing(side, item, qty, limit)
	filled, cost := matchValue(ms)
	rest := qty - filled
	res.Filled = filled
	res.Cost = cost
	res.Remaining = rest

	if owner == OwnerPlayer {
		if side == OrderBuy {
			escrow := limit * float64(qty)
			fee := MarketBrokerFee * escrow
			if s.wallet < escrow+fee {
				return OrderResult{}, false
			}
			s.wallet -= escrow + fee
			res.Fee = fee
			// Escrow covered every fill (at or below the limit) and the
			// resting remainder: only the price improvement is surplus.
			res.Refund = escrow - cost - limit*float64(rest)
			s.wallet += res.Refund
		} else {
			if _, isShip := s.Content.Ship(content.ShipID(item)); isShip {
				return OrderResult{}, false // fitted hulls cannot be escrowed
			}
			fee := MarketBrokerFee * limit * float64(qty)
			if s.wallet < fee {
				return OrderResult{}, false
			}
			if taken := s.takePlayerStock(item, qty); taken != qty {
				s.storePlayerStock(item, taken)
				return OrderResult{}, false
			}
			// Broker fee rides on the placed value (EVE charges it at
			// placement, both sides, and keeps it on cancel).
			s.wallet -= fee
			res.Fee = fee
		}
	}

	if filled > 0 {
		qtyFilled, value := s.settle(book, ms)
		if owner == OwnerPlayer {
			if side == OrderBuy {
				s.storePlayerStock(item, qtyFilled)
				s.emit(Event{Kind: EvMarket, Severity: SevGood, Actor: s.player, Item: item,
					Text: "Bought " + itoaN(qtyFilled) + " x " + s.itemName(item) + " for " +
						euro(value) + " (" + euro(res.Fee) + " broker fee)",
					Value: value})
			} else {
				tax := MarketSalesTax * value
				received := value - tax
				s.wallet += received
				s.emit(Event{Kind: EvMarket, Severity: SevGood, Actor: s.player, Item: item,
					Text: "Sold " + itoaN(qtyFilled) + " x " + s.itemName(item) + " for " +
						euro(received) + " (" + euro(tax) + " sales tax)",
					Value: received})
			}
		}
	}
	if rest > 0 {
		s.marketSeq++
		res.ID = s.marketSeq
		book.Orders = append(book.Orders, MarketStock{
			Item: item, Price: limit, Qty: rest, Side: side,
			Owner: owner, Station: station, ID: res.ID,
		})
		book.sortOrders()
		if owner == OwnerPlayer {
			verb := "sell"
			if side == OrderBuy {
				verb = "buy"
			}
			s.emit(Event{Kind: EvMarket, Severity: SevInfo, Actor: s.player, Item: item,
				Text: "Placed " + verb + " order: " + itoaN(rest) + " x " +
					s.itemName(item) + " at " + euro(limit)})
		}
	}
	return res, true
}

// CancelOrder removes a resting order of the given owner from a station's
// book and settles its escrow: a buy's unspent wallet escrow comes back,
// a player's sell gets its goods into the station hangar. It reports
// whether an order was found and owned by owner.
func (s *Sim) CancelOrder(station StationID, id uint64, owner string) bool {
	book := s.market[station]
	if book == nil || id == 0 {
		return false
	}
	idx := -1
	for i := range book.Orders {
		if book.Orders[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 || book.Orders[idx].Owner != owner {
		return false
	}
	o := book.Orders[idx]
	if owner == OwnerPlayer {
		if o.Side == OrderBuy {
			s.wallet += o.Price * float64(o.Qty)
		} else {
			s.storePlayerStock(o.Item, o.Qty)
		}
	}
	book.Orders = append(book.Orders[:idx], book.Orders[idx+1:]...)
	book.sortOrders()
	return true
}

// execRefine converts ore units into minerals via RefineYield at a refine
// service station. Output is floor(yield * qty) per mineral (documented);
// when the outputs do not fit in the general hold, nothing is refined and
// EvtCargoFull fires.
func (s *Sim) execRefine(c CmdRefine) {
	if _, ok := s.stationAccess("refine"); !ok {
		return
	}
	def, ok := s.Content.Item(c.Item)
	if !ok || def.Category != content.CatOre || len(def.RefineYield) == 0 {
		s.log(SevWarning, s.player, "Cannot refine "+s.itemName(c.Item))
		return
	}
	cargo, ok := s.Cargos.Get(s.player)
	if !ok || c.Qty <= 0 {
		s.log(SevWarning, s.player, "Invalid quantity")
		return
	}
	avail := stacksCount(cargo.Stacks, c.Item) + stacksCount(cargo.Ore, c.Item)
	qty := c.Qty
	if qty > avail {
		qty = avail
	}
	if qty <= 0 {
		s.log(SevWarning, s.player, "No "+def.Name+" to refine")
		return
	}

	// Deterministic output order: sorted mineral IDs.
	outs := make([]content.ItemID, 0, len(def.RefineYield))
	for id := range def.RefineYield {
		outs = append(outs, id)
	}
	sort.Slice(outs, func(i, j int) bool { return outs[i] < outs[j] })

	type outStack struct {
		item content.ItemID
		qty  int
	}
	var results []outStack
	var outVol float64
	for _, id := range outs {
		n := int(math.Floor(def.RefineYield[id] * float64(qty)))
		if n > 0 {
			results = append(results, outStack{id, n})
			outVol += s.itemVolume(id) * float64(n)
		}
	}
	if space := s.cargoSpace(cargo.Stacks, cargo.Capacity); outVol > space+1e-9 {
		s.emit(Event{Kind: EvCargoFull, Severity: SevWarning, Actor: s.player,
			Text: "Cargo hold full: cannot store refined minerals"})
		return
	}

	// Consume ore (general hold first, then ore hold).
	var t1, t2 int
	cargo.Stacks, t1 = takeStacks(cargo.Stacks, c.Item, qty)
	cargo.Ore, t2 = takeStacks(cargo.Ore, c.Item, qty-t1)
	if t1+t2 != qty {
		// Shouldn't happen (avail checked); restore nothing, just report.
		s.log(SevWarning, s.player, "Refine count mismatch")
		return
	}
	for _, r := range results {
		cargo.Stacks = addStacks(cargo.Stacks, r.item, r.qty)
	}

	parts := ""
	for i, r := range results {
		if i > 0 {
			parts += ", "
		}
		parts += itoaN(r.qty) + " x " + s.itemName(r.item)
	}
	if parts == "" {
		parts = "nothing"
	}
	s.emit(Event{Kind: EvLog, Severity: SevGood, Actor: s.player, Item: c.Item,
		Text:  "Refined " + itoaN(qty) + " x " + def.Name + " into " + parts,
		Value: float64(qty)})
}

// execRepair restores all three layers to maximum for 2 euro per missing HP.
func (s *Sim) execRepair() {
	if _, ok := s.stationAccess("repair"); !ok {
		return
	}
	ship, ok := s.Ships.Get(s.player)
	if !ok {
		return
	}
	missing := math.Max(0, ship.MaxShield-ship.Shield) +
		math.Max(0, ship.MaxArmor-ship.Armor) +
		math.Max(0, ship.MaxStructure-ship.Structure)
	if missing <= 0 {
		s.log(SevInfo, s.player, "Ship is already at full strength")
		return
	}
	cost := repairCostPerHP * missing
	if s.wallet < cost {
		s.emit(Event{Kind: EvLog, Severity: SevWarning, Actor: s.player,
			Text:  "Insufficient funds for repairs (" + euro(cost) + ")",
			Value: cost})
		return
	}
	s.wallet -= cost
	ship.Shield = ship.MaxShield
	ship.Armor = ship.MaxArmor
	ship.Structure = ship.MaxStructure
	s.emit(Event{Kind: EvLog, Severity: SevGood, Actor: s.player,
		Text:  "Repaired " + num(missing) + " HP for " + euro(cost),
		Value: cost})
}

// execFit moves a module from the hangar into a hull slot (fitting service).
func (s *Sim) execFit(c CmdFitModule) {
	if _, ok := s.stationAccess("fitting"); !ok {
		return
	}
	def, ok := s.Content.Module(c.Item)
	if !ok {
		s.log(SevWarning, s.player, "Unknown module "+string(c.Item))
		return
	}
	if def.Slot != c.Slot.Class {
		s.log(SevWarning, s.player, def.Name+" cannot fit a "+c.Slot.Class+" slot")
		return
	}
	fit, ok := s.Fittings.Get(s.player)
	if !ok {
		return
	}
	ship, ok := s.Ships.Get(s.player)
	if !ok {
		return
	}
	hull, ok := s.Content.Ship(ship.Hull)
	if !ok {
		return
	}
	defs := fit.Defs(c.Slot.Class)
	cap := slotCapacity(hull, c.Slot.Class)
	if len(defs) >= cap {
		s.log(SevWarning, s.player, "No free "+c.Slot.Class+" slot")
		return
	}
	if c.Slot.Index < 0 || c.Slot.Index > len(defs) {
		s.log(SevWarning, s.player, "Invalid slot index")
		return
	}
	hangar, n := takeStacks(s.Hangar.Items, content.ItemID(c.Item), 1)
	if n < 1 {
		s.log(SevWarning, s.player, def.Name+" is not in the hangar")
		return
	}
	s.Hangar.Items = hangar

	rt := ModuleRuntime{Def: c.Item}
	switch c.Slot.Class {
	case content.SlotHigh:
		fit.High = slices.Insert(fit.High, c.Slot.Index, c.Item)
		fit.HighRT = slices.Insert(fit.HighRT, c.Slot.Index, rt)
	case content.SlotMid:
		fit.Mid = slices.Insert(fit.Mid, c.Slot.Index, c.Item)
		fit.MidRT = slices.Insert(fit.MidRT, c.Slot.Index, rt)
	case content.SlotLow:
		fit.Low = slices.Insert(fit.Low, c.Slot.Index, c.Item)
		fit.LowRT = slices.Insert(fit.LowRT, c.Slot.Index, rt)
	}
	s.recomputeFit(s.player)
	s.emit(Event{Kind: EvModule, Actor: s.player,
		Text: def.Name + " fitted to " + c.Slot.Class + " slot " + itoaN(c.Slot.Index+1)})
}

// execUnfit moves a fitted module back into the hangar (fitting service).
func (s *Sim) execUnfit(c CmdUnfitModule) {
	if _, ok := s.stationAccess("fitting"); !ok {
		return
	}
	fit, ok := s.Fittings.Get(s.player)
	if !ok {
		return
	}
	i := c.Slot.Index
	if i < 0 {
		s.log(SevWarning, s.player, "Invalid slot index")
		return
	}
	var id content.ModuleID
	switch c.Slot.Class {
	case content.SlotHigh:
		if i >= len(fit.High) {
			s.log(SevWarning, s.player, "No module in that slot")
			return
		}
		id = fit.High[i]
		fit.High = slices.Delete(fit.High, i, i+1)
		fit.HighRT = slices.Delete(fit.HighRT, i, i+1)
	case content.SlotMid:
		if i >= len(fit.Mid) {
			s.log(SevWarning, s.player, "No module in that slot")
			return
		}
		id = fit.Mid[i]
		fit.Mid = slices.Delete(fit.Mid, i, i+1)
		fit.MidRT = slices.Delete(fit.MidRT, i, i+1)
	case content.SlotLow:
		if i >= len(fit.Low) {
			s.log(SevWarning, s.player, "No module in that slot")
			return
		}
		id = fit.Low[i]
		fit.Low = slices.Delete(fit.Low, i, i+1)
		fit.LowRT = slices.Delete(fit.LowRT, i, i+1)
	default:
		s.log(SevWarning, s.player, "Invalid slot class")
		return
	}
	// Drop and re-key remembered activation targets and ammo preferences
	// around the removed slot.
	s.shiftModTargets(s.player, c.Slot.Class, c.Slot.Index)
	s.shiftAmmoPrefs(s.player, c.Slot.Class, c.Slot.Index)
	s.Hangar.Items = addStacks(s.Hangar.Items, content.ItemID(id), 1)
	s.recomputeFit(s.player)
	name := string(id)
	if def, ok := s.Content.Module(id); ok {
		name = def.Name
	}
	s.emit(Event{Kind: EvModule, Actor: s.player, Text: name + " unfit"})
}

// execAssemble swaps the piloted hull with a stored hull in the hangar
// (fitting service). Cargo travels with each hull; stored hulls always carry
// full HP (StoredShip has no HP field — documented simplification).
func (s *Sim) execAssemble(c CmdAssemble) {
	if _, ok := s.stationAccess("fitting"); !ok {
		return
	}
	storedIdx := -1
	for i, sh := range s.Hangar.Ships {
		if sh.Hull == c.Ship {
			storedIdx = i
			break
		}
	}
	if storedIdx < 0 {
		s.log(SevWarning, s.player, "No "+s.itemName(content.ItemID(c.Ship))+" in the hangar")
		return
	}
	newHull, ok := s.Content.Ship(c.Ship)
	if !ok {
		s.log(SevWarning, s.player, "Unknown ship "+string(c.Ship))
		return
	}
	ship, ok := s.Ships.Get(s.player)
	if !ok {
		return
	}
	curHull, ok := s.Content.Ship(ship.Hull)
	if !ok {
		return
	}
	incoming := s.Hangar.Ships[storedIdx]
	newFit, err := s.buildFitting(incoming.Fit, newHull)
	if err != nil {
		s.log(SevWarning, s.player, "Stored fit is invalid: "+err.Error())
		return
	}

	// Snapshot the current hull + fit + cargo for the hangar.
	var curStored StoredShip
	if fit, ok := s.Fittings.Get(s.player); ok {
		curStored.Fit = flatFit(fit)
	}
	if cargo, ok := s.Cargos.Get(s.player); ok {
		merged := append([]content.CargoStack(nil), cargo.Stacks...)
		merged = append(merged, cargo.Ore...)
		curStored.Cargo = merged
	}
	curStored.Hull = curHull.ID
	s.Hangar.Ships[storedIdx] = curStored

	// Install the incoming hull: full layers, full capacitor, its cargo
	// (ore into the ore hold when the hull has one, else general cargo).
	newCargo := Cargo{Capacity: newHull.Cargo, OreCapacity: newHull.OreHold}
	for _, st := range incoming.Cargo {
		if st.Qty <= 0 {
			continue
		}
		to := &newCargo.Stacks
		if def, ok := s.Content.Item(st.Item); ok && def.Category == content.CatOre && newHull.OreHold > 0 {
			to = &newCargo.Ore
		}
		*to = addStacks(*to, st.Item, st.Qty)
	}
	s.Ships.Set(s.player, ShipState{
		Hull: newHull.ID, Shield: newHull.Shield, Armor: newHull.Armor, Structure: newHull.Structure,
		MaxShield: newHull.Shield, MaxArmor: newHull.Armor, MaxStructure: newHull.Structure,
		Resists: newHull.Resists,
	})
	s.Caps.Set(s.player, Capacitor{Current: newHull.Capacitor, Capacity: newHull.Capacitor, Tau: newHull.Recharge})
	s.Cargos.Set(s.player, newCargo)
	s.Fittings.Set(s.player, newFit)
	if t, ok := s.Targetings.Get(s.player); ok {
		t.Locks = nil
	}
	s.recomputeFit(s.player)
	s.emit(Event{Kind: EvLog, Severity: SevGood, Actor: s.player,
		Text: "Assembled " + newHull.Name})
}

// slotCapacity returns the hull's slot count for a class.
func slotCapacity(hull *content.ShipDef, class string) int {
	switch class {
	case content.SlotHigh:
		return hull.Slots.High
	case content.SlotMid:
		return hull.Slots.Mid
	case content.SlotLow:
		return hull.Slots.Low
	}
	return 0
}

// itoaN formats an int for event text.
func itoaN(n int) string { return strconv.Itoa(n) }
