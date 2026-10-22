package sim

import (
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
)

// ---------------------------------------------------------------------------
// Per-station continuous double auction: book invariants, escrow, ticks,
// fees, isolation, persistence. These tests are the acceptance gate of the
// market tranche (determinism discipline: no timing, no randomness).
// ---------------------------------------------------------------------------

// canonicalOrders returns an independently re-sorted copy of an order list
// (item, asks before bids, asks ascending, bids descending, arrival id) —
// the oracle assertBooksSane compares book order against.
func canonicalOrders(orders []MarketStock) []MarketStock {
	cp := append([]MarketStock(nil), orders...)
	sideRank := func(s OrderSide) int {
		if s == OrderSell {
			return 0
		}
		return 1
	}
	sort.Slice(cp, func(i, j int) bool {
		a, b := cp[i], cp[j]
		if a.Item != b.Item {
			return a.Item < b.Item
		}
		if sideRank(a.Side) != sideRank(b.Side) {
			return sideRank(a.Side) < sideRank(b.Side)
		}
		if a.Price != b.Price {
			if sideRank(a.Side) == 0 {
				return a.Price < b.Price
			}
			return a.Price > b.Price
		}
		return a.ID < b.ID
	})
	return cp
}

// assertBooksSane checks every book in the sim: canonical order, positive
// quantities, unique arrival ids, and the never-crossed invariant (every
// bid of an item strictly below every ask of the same item).
func assertBooksSane(t *testing.T, s *Sim) {
	t.Helper()
	for _, station := range s.stationsSorted() {
		book := s.market[station]
		if book == nil {
			continue
		}
		canon := canonicalOrders(book.Orders)
		if len(canon) != len(book.Orders) || (len(canon) > 0 && !slices.Equal(book.Orders, canon)) {
			t.Fatalf("station %s: book not in canonical order:\n%+v", station, book.Orders)
		}
		ask := map[content.ItemID]float64{}
		bid := map[content.ItemID]float64{}
		seen := map[uint64]bool{}
		for _, o := range book.Orders {
			if o.Qty <= 0 {
				t.Fatalf("station %s: order %+v has non-positive qty", station, o)
			}
			if o.ID == 0 || seen[o.ID] {
				t.Fatalf("station %s: order %+v has a zero or duplicate arrival id", station, o)
			}
			seen[o.ID] = true
			if o.Station != station {
				t.Fatalf("order %+v rests outside its book %s", o, station)
			}
			if o.Side == OrderBuy {
				if p, ok := bid[o.Item]; !ok || o.Price > p {
					bid[o.Item] = o.Price
				}
			} else {
				if p, ok := ask[o.Item]; !ok || o.Price < p {
					ask[o.Item] = o.Price
				}
			}
		}
		for item, a := range ask {
			if b, ok := bid[item]; ok && b >= a {
				t.Fatalf("station %s: book crossed for %s (bid %v >= ask %v)",
					station, item, b, a)
			}
		}
	}
}

// TestBooksStartSane: the seeded fixture books are canonical and
// never-crossed before a single order arrives.
func TestBooksStartSane(t *testing.T) {
	s := newTestSim(t)
	assertBooksSane(t, s)
	if orders := len(s.BookOrders("cel-station-a")); orders != 7 {
		t.Fatalf("founding orders = %d, want 7 (5 asks + 2 bids)", orders)
	}
}

// TestPlaceOrderNeverCrossesBook pins the never-crossed invariant: an
// incoming order executes against every crossing resting order at the
// RESTING order's price, and only the unfilled remainder rests — so a bid
// can never land at or above a living ask.
func TestPlaceOrderNeverCrossesBook(t *testing.T) {
	s := newTestSim(t)
	station := StationID("cel-station-a")

	// The fixture asks 100 ammo-emp-s @ 5.00; cross it with a 120-unit bid
	// at 5.00: 100 fill at the resting price, 20 rest.
	res, ok := s.PlaceOrder(station, OrderBuy, "ammo-emp-s", 120, 5, OwnerSystem)
	if !ok {
		t.Fatal("placement refused")
	}
	if res.Filled != 100 || res.Remaining != 20 || res.Cost != 500 {
		t.Fatalf("result = %+v, want {Filled:100 Remaining:20 Cost:500}", res)
	}
	assertBooksSane(t, s)

	// The ask is gone; the remainder rests above the founding 4.00 bid
	// (bids rank descending: the new 20 @ 5.00 first).
	var chargeEmp []MarketStock
	for _, o := range s.BookOrders(station) {
		if o.Item == "ammo-emp-s" {
			chargeEmp = append(chargeEmp, o)
		}
	}
	if len(chargeEmp) != 2 || chargeEmp[0].Side != OrderBuy ||
		chargeEmp[0].Qty != 20 || chargeEmp[0].Price != 5 ||
		chargeEmp[1].Side != OrderBuy || chargeEmp[1].Price != 4 {
		t.Fatalf("ammo-emp-s orders = %+v, want bids [20 @ 5, 40 @ 4] and no asks", chargeEmp)
	}
	// A system owner pays no escrow (the book just moves).
	if s.Wallet() != 100000 {
		t.Fatalf("wallet = %v, want unchanged", s.Wallet())
	}
}

// TestPriceTimePriority: two resting orders at the same price fill in
// arrival order — the earlier id is consumed first.
func TestPriceTimePriority(t *testing.T) {
	s := newTestSim(t)
	station := StationID("cel-station-a")

	first, ok := s.PlaceOrder(station, OrderSell, "min-nickel", 10, 10, OwnerSystem)
	if !ok {
		t.Fatal("first placement refused")
	}
	second, ok := s.PlaceOrder(station, OrderSell, "min-nickel", 10, 10, OwnerSystem)
	if !ok || second.ID <= first.ID {
		t.Fatalf("ids must increase: first=%d second=%d", first.ID, second.ID)
	}
	// Take 12 units: all of the first order, 2 of the second.
	res, ok := s.PlaceOrder(station, OrderBuy, "min-nickel", 12, 10, OwnerSystem)
	if !ok || res.Filled != 12 || res.Cost != 120 {
		t.Fatalf("fill = %+v, want 12 units for 120", res)
	}
	var asks []MarketStock
	for _, o := range s.BookOrders(station) {
		if o.Item == "min-nickel" && o.Side == OrderSell {
			asks = append(asks, o)
		}
	}
	if len(asks) != 1 || asks[0].ID != second.ID || asks[0].Qty != 8 {
		t.Fatalf("remaining asks = %+v, want the SECOND order with 8 left", asks)
	}
	assertBooksSane(t, s)
}

// TestWeightedAverageMarketOrderFill: a player market buy that spans two
// price levels pays the volume-weighted total (100 @ 5 + 20 @ 6 = 620 for
// 120 units) plus the broker fee, and the event line says exactly that.
func TestWeightedAverageMarketOrderFill(t *testing.T) {
	s := newTestSim(t)
	station := StationID("cel-station-a")
	if _, ok := s.PlaceOrder(station, OrderSell, "ammo-emp-s", 50, 6, OwnerSystem); !ok {
		t.Fatal("second ask refused")
	}
	dockAtHaven(t, s)
	walletBefore := s.Wallet()

	s.Queue(CmdBuy{Item: "ammo-emp-s", Qty: 120})
	s.Step()
	evs := drain(s)

	cost := 100*5.0 + 20*6.0 // 620
	fee := MarketBrokerFee * cost
	if s.Wallet() != walletBefore-(cost+fee) {
		t.Fatalf("wallet = %v, want %v (weighted cost %v + fee %v)",
			s.Wallet(), walletBefore-(cost+fee), cost, fee)
	}
	if hangarCount(s, "ammo-emp-s") != 120 {
		t.Fatalf("hangar = %d, want 120", hangarCount(s, "ammo-emp-s"))
	}
	if !hasEvent(evs, EvMarket, "Bought 120 x EMP Charge for 620.00 €") {
		t.Fatalf("missing weighted-fill event: %v", evs)
	}
	assertBooksSane(t, s)
}

// TestEscrowDebitAndRefund pins the 100% escrow rule: a player's resting
// buy debits wallet by price×qty + broker fee at placement; cancelling
// refunds the escrow in full while the broker fee stays (EVE's rule).
func TestEscrowDebitAndRefund(t *testing.T) {
	s := newTestSim(t)
	station := StationID("cel-station-a")
	walletBefore := s.Wallet()

	// Bid under the 5.00 ask: nothing crosses, the order rests.
	res, ok := s.PlaceOrder(station, OrderBuy, "ammo-emp-s", 10, 3, OwnerPlayer)
	if !ok || res.Filled != 0 || res.Remaining != 10 || res.ID == 0 {
		t.Fatalf("result = %+v, want a resting 10-unit bid", res)
	}
	escrow := 3 * 10.0
	fee := MarketBrokerFee * escrow
	if s.Wallet() != walletBefore-(escrow+fee) {
		t.Fatalf("wallet = %v, want %v (escrow %v + fee %v)",
			s.Wallet(), walletBefore-(escrow+fee), escrow, fee)
	}

	// A stranger cannot cancel it; the owner can.
	if s.CancelOrder(station, res.ID, OwnerSystem) {
		t.Fatal("a non-owner cancelled the player's order")
	}
	if !s.CancelOrder(station, res.ID, OwnerPlayer) {
		t.Fatal("owner could not cancel")
	}
	if s.Wallet() != walletBefore-fee {
		t.Fatalf("wallet = %v, want %v (escrow refunded, %v fee kept)",
			s.Wallet(), walletBefore-fee, fee)
	}
	if len(s.BookOrders(station)) != 7 {
		t.Fatalf("orders = %d, want the founding 7 back", len(s.BookOrders(station)))
	}
	assertBooksSane(t, s)
}

// TestMarketableLimitRefundsPriceImprovement: an incoming player buy that
// crosses resting asks at better prices pays only for what it executes —
// the surplus of the escrow comes back immediately.
func TestMarketableLimitRefundsPriceImprovement(t *testing.T) {
	s := newTestSim(t)
	station := StationID("cel-station-a")
	walletBefore := s.Wallet()

	// Limit 10 against the 5.00 ask: 6 units at the resting price.
	res, ok := s.PlaceOrder(station, OrderBuy, "ammo-emp-s", 6, 10, OwnerPlayer)
	if !ok || res.Filled != 6 || res.Remaining != 0 {
		t.Fatalf("result = %+v, want a full 6-unit fill", res)
	}
	cost := 6 * 5.0
	fee := MarketBrokerFee * (10 * 6.0) // fee rides on the placed value
	want := walletBefore - (10*6.0 + fee) + res.Refund
	if res.Refund != 10*6.0-cost {
		t.Fatalf("refund = %v, want %v (escrow minus execution)", res.Refund, 10*6.0-cost)
	}
	if s.Wallet() != want {
		t.Fatalf("wallet = %v, want %v", s.Wallet(), want)
	}
	if hangarCount(s, "ammo-emp-s") != 6 {
		t.Fatalf("hangar = %d, want 6", hangarCount(s, "ammo-emp-s"))
	}
}

// TestPriceTickQuantization pins EVE's 4-significant-figure tick rule: all
// order prices land on the tick grid, and tickDown steps strictly down.
func TestPriceTickQuantization(t *testing.T) {
	for _, tc := range []struct{ in, want float64 }{
		{48, 48},
		{47.999, 48},
		{1.23456, 1.235},
		{0.123456, 0.1235},
		{1234.56, 1235},
		{195000, 195000},
		{95, 95},
	} {
		if got := quantizeTick(tc.in); got != tc.want {
			t.Errorf("quantizeTick(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
	for _, p := range []float64{48, 100, 1.005, 0.5, 195000} {
		down := tickDown(p)
		if down >= quantizeTick(p) {
			t.Errorf("tickDown(%v) = %v, want strictly below %v", p, down, quantizeTick(p))
		}
		if down != quantizeTick(down) {
			t.Errorf("tickDown(%v) = %v is off the tick grid", p, down)
		}
	}
	// An off-grid placement rests on the grid.
	s := newTestSim(t)
	if _, ok := s.PlaceOrder(StationID("cel-station-a"), OrderSell, "min-nickel", 1, 1.23456, OwnerSystem); !ok {
		t.Fatal("placement refused")
	}
	for _, o := range s.BookOrders("cel-station-a") {
		if o.Item == "min-nickel" && o.Price != 1.235 {
			t.Fatalf("resting price = %v, want 1.235 (tick-quantized)", o.Price)
		}
	}
}

// TestFeeLoadWithinBand: the institutional load the UI quotes is the code's
// own constants, and a round trip stays inside the approved 5-11% band.
func TestFeeLoadWithinBand(t *testing.T) {
	combined := MarketBrokerFee + MarketSalesTax
	if combined < 0.05 || combined > 0.11 {
		t.Fatalf("combined fee load = %.3f, want within [0.05, 0.11]", combined)
	}
	// Sales tax is charged on execution: sell 10 units into a 10.00 bid.
	s := newTestSim(t)
	station := StationID("cel-station-a")
	if _, ok := s.PlaceOrder(station, OrderBuy, "min-nickel", 10, 10, OwnerSystem); !ok {
		t.Fatal("bid placement refused")
	}
	dockAtHaven(t, s)
	giveCargo(s, "min-nickel", 10)
	walletBefore := s.Wallet()
	s.Queue(CmdSell{Item: "min-nickel", Qty: 10})
	s.Step()
	drain(s)
	fee := MarketBrokerFee * 100
	tax := MarketSalesTax * 100
	if s.Wallet() != walletBefore+(100-tax-fee) {
		t.Fatalf("wallet = %v, want %v (100 gross, %v broker fee, %v tax)",
			s.Wallet(), walletBefore+(100-tax-fee), fee, tax)
	}
}

// TestEmptyBookRefusals: with no orders on a side, nothing to buy and
// nothing to sell — refusal with zero state change (never conjure).
func TestEmptyBookRefusals(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	walletBefore := s.Wallet()

	// No ask for an item the fixture never seeded (ammo-ap-s exists but no
	// station here bids or asks it).
	s.Queue(CmdBuy{Item: "ammo-ap-s", Qty: 1})
	s.Step()
	if evs := drain(s); findEvent(evs, EvLog, "is not sold here") == nil {
		t.Fatalf("no refusal: %v", evs)
	}
	// No bid for an item nobody wants.
	giveCargo(s, "ammo-ap-s", 2)
	s.Queue(CmdSell{Item: "ammo-ap-s", Qty: 2})
	s.Step()
	if evs := drain(s); findEvent(evs, EvLog, "No buy orders for") == nil {
		t.Fatalf("no refusal: %v", evs)
	}
	if s.Wallet() != walletBefore {
		t.Fatalf("wallet = %v, want unchanged", s.Wallet())
	}
	if cargoCount(s, "ammo-ap-s") != 2 {
		t.Fatal("a refusal moved goods")
	}
}

// TestPlaceOrderRejectsBadInput: the API refuses nonsense outright (no
// state, no escrow, no half-placed orders).
func TestPlaceOrderRejectsBadInput(t *testing.T) {
	s := newTestSim(t)
	station := StationID("cel-station-a")
	walletBefore := s.Wallet()
	cases := []struct {
		name  string
		st    StationID
		side  OrderSide
		item  content.ItemID
		qty   int
		price float64
		owner string
	}{
		{"zero qty", station, OrderSell, "min-nickel", 0, 10, OwnerSystem},
		{"negative price", station, OrderSell, "min-nickel", 1, -1, OwnerSystem},
		{"bad side", station, OrderSide("hold"), "min-nickel", 1, 10, OwnerSystem},
		{"unknown station", "cel-nothing", OrderSell, "min-nickel", 1, 10, OwnerSystem},
	}
	for _, tc := range cases {
		if _, ok := s.PlaceOrder(tc.st, tc.side, tc.item, tc.qty, tc.price, tc.owner); ok {
			t.Errorf("%s: placement accepted", tc.name)
		}
	}
	if s.Wallet() != walletBefore {
		t.Fatalf("wallet = %v, want unchanged", s.Wallet())
	}
	assertBooksSane(t, s)
}

// TestPlayerSellEscrowAndCancel: a player sell order escrows the goods at
// placement and returns exactly the unfilled remainder on cancel.
func TestPlayerSellEscrowAndCancel(t *testing.T) {
	s := newTestSim(t)
	station := StationID("cel-station-a")
	giveCargo(s, "min-nickel", 10)
	ammoBefore := cargoCount(s, "min-nickel")

	// Bid under the resting order: a sell at 10 crosses the fixture's
	// min-ferrite bid only for min-ferrite — nickel has no bid, so all 10
	// rest (escrowed in the order, out of cargo).
	res, ok := s.PlaceOrder(station, OrderSell, "min-nickel", 10, 10, OwnerPlayer)
	if !ok || res.Filled != 0 || res.Remaining != 10 {
		t.Fatalf("result = %+v, want a resting 10-unit ask", res)
	}
	if cargoCount(s, "min-nickel") != ammoBefore-10 {
		t.Fatalf("cargo = %d, want %d (goods escrowed in the order)",
			cargoCount(s, "min-nickel"), ammoBefore-10)
	}
	if !s.CancelOrder(station, res.ID, OwnerPlayer) {
		t.Fatal("cancel refused")
	}
	// Escrowed goods come back to the station hangar (the market holds
	// them, not the cargo hold).
	if n := stacksCount(s.Hangar.Items, "min-nickel"); n != 10 {
		t.Fatalf("hangar = %d, want 10 (escrow returned)", n)
	}
	if cargoCount(s, "min-nickel") != 0 {
		t.Fatalf("cargo = %d, want 0 (returned goods go to the hangar)", cargoCount(s, "min-nickel"))
	}
	// Fitted hulls cannot ride in an order (their fit would be lost).
	s.Hangar.Ships = append(s.Hangar.Ships, StoredShip{Hull: "ship-sloop",
		Fit: []content.ModuleID{"mod-autocannon-s"}})
	if _, ok := s.PlaceOrder(station, OrderSell, "ship-sloop", 1, 1000, OwnerPlayer); ok {
		t.Fatal("a hull was accepted into a sell order (fit would be lost)")
	}
	if len(s.Hangar.Ships) != 1 {
		t.Fatalf("hangar = %d hulls, want 1", len(s.Hangar.Ships))
	}
}

// TestPerStationBooksAreIsolated: two stations can hold the same item at
// divergent prices; trading at one never touches the other (the market is
// per station, not per system).
func TestPerStationBooksAreIsolated(t *testing.T) {
	s, err := New(twoStationContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	anvil := StationID("cel-station-b")
	haven := StationID("cel-station-a")
	if _, ok := s.PlaceOrder(anvil, OrderSell, "ammo-emp-s", 30, 12, OwnerSystem); !ok {
		t.Fatal("anvil placement refused")
	}
	havenPrice := s.MarketPrice(haven, "ammo-emp-s")
	anvilPrice := s.MarketPrice(anvil, "ammo-emp-s")
	if havenPrice != 5 || anvilPrice != 12 {
		t.Fatalf("prices = haven %v / anvil %v, want 5 / 12", havenPrice, anvilPrice)
	}

	// Buy out Anvil's ask entirely: Haven Rest keeps every unit.
	anvilE := entityByName(s, "Anvil Stop")
	if anvilE.IsZero() {
		t.Fatal("no Anvil Stop entity")
	}
	teleport(s, s.player, mustPos(s, anvilE))
	dockAtStation(t, s, "Anvil Stop")
	walletBefore := s.Wallet()
	s.Queue(CmdBuy{Item: "ammo-emp-s", Qty: 30})
	s.Step()
	drain(s)
	if qty, price := askAtBook(s, haven, "ammo-emp-s"); qty != 100 || price != 5 {
		t.Fatalf("haven ask = %d @ %v, want 100 @ 5 (isolated)", qty, price)
	}
	if qty, _ := askAtBook(s, anvil, "ammo-emp-s"); qty != 0 {
		t.Fatalf("anvil ask = %d, want 0 (bought out)", qty)
	}
	if s.Wallet() >= walletBefore {
		t.Fatalf("wallet = %v, want a debit", s.Wallet())
	}
	// The ladder keeps the trade that emptied the book: an all-filled book
	// still remembers what the units went for (rung #1: last trade price).
	if p := s.MarketPrice(anvil, "ammo-emp-s"); p != 12 {
		t.Fatalf("anvil ladder = %v, want the last trade price 12", p)
	}
	// Haven Rest never traded: its ladder reads its own best ask (5.00).
	if p := s.MarketPrice(haven, "ammo-emp-s"); p != 5 {
		t.Fatalf("haven ladder = %v, want the best ask 5", p)
	}
	assertBooksSane(t, s)
}

// askAtBook returns the best ask at one station's book (test helper).
func askAtBook(s *Sim, station StationID, item content.ItemID) (int, float64) {
	for _, o := range s.BookOrders(station) {
		if o.Item == item && o.Side == OrderSell {
			return o.Qty, o.Price
		}
	}
	return 0, 0
}

// dockAtStation docks the player at a named station (any system) and
// drains the docking events.
func dockAtStation(t *testing.T, s *Sim, name string) {
	t.Helper()
	station := entityByName(s, name)
	if station.IsZero() {
		t.Fatalf("no station named %q", name)
	}
	s.Queue(CmdDock{Station: station})
	s.Step()
	if evs := drain(s); !hasEvent(evs, EvDocked, "Docked at "+name) {
		t.Fatalf("docking failed: %v", evs)
	}
}

// TestMarketBooksSurviveSaveRestore pins the v2 persistence of the books:
// orders (station, side, owner, arrival id), the next-order clock, the
// last-trade rungs — restored exactly, Hash identical, and twin runs stay
// in lockstep afterwards. The schema version itself is pinned too: v2 is
// written, anything else is rejected.
func TestMarketBooksSurviveSaveRestore(t *testing.T) {
	s := newTestSim(t)
	station := StationID("cel-station-a")

	// Leave a mixed state: a resting player bid (escrowed), a system ask
	// for an unseeded good, and one executed trade (a last-trade rung).
	bid, ok := s.PlaceOrder(station, OrderBuy, "min-nickel", 7, 3, OwnerPlayer)
	if !ok {
		t.Fatal("player bid refused")
	}
	if _, ok := s.PlaceOrder(station, OrderSell, "loot-alloy-scrap", 5, 40, OwnerSystem); !ok {
		t.Fatal("system ask refused")
	}
	trade, ok := s.PlaceOrder(station, OrderBuy, "loot-alloy-scrap", 5, 40, OwnerSystem)
	if !ok || trade.Filled != 5 {
		t.Fatalf("trade = %+v, want a 5-unit fill", trade)
	}
	assertBooksSane(t, s)
	wantHash := s.Hash()

	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if save.Schema != saveSchemaV3 {
		t.Fatalf("Save.Schema = %d, want %d", save.Schema, saveSchemaV3)
	}
	if save.MarketSeq != s.marketSeq {
		t.Fatalf("MarketSeq = %d, want %d", save.MarketSeq, s.marketSeq)
	}
	// 7 founding + the resting player bid + the system ask, minus the ask
	// the executed trade consumed.
	if orders := save.Market["sys-a"]; len(orders) != 8 {
		t.Fatalf("saved orders = %d, want 8: %+v", len(orders), orders)
	}
	var last SavedLastTrade
	for _, lt := range save.MarketLasts {
		if lt.Item == "loot-alloy-scrap" {
			last = lt
		}
	}
	if last.Price != 40 || last.Station != station {
		t.Fatalf("last trade = %+v, want %s 40 @ %s", last, "loot-alloy-scrap", station)
	}

	b, err := New(testContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Restore(save); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := b.Hash(); got != wantHash {
		t.Fatalf("hash after restore = %016x, want %016x", got, wantHash)
	}
	if !reflect.DeepEqual(b.BookOrders(station), s.BookOrders(station)) {
		t.Fatalf("restored book = %+v, want %+v", b.BookOrders(station), s.BookOrders(station))
	}
	if b.marketSeq != s.marketSeq {
		t.Fatalf("restored seq = %d, want %d", b.marketSeq, s.marketSeq)
	}
	// The escrow bookkeeping restored with the order: cancelling the bid in
	// the restored sim refunds exactly what the live sim escrowed.
	if !b.CancelOrder(station, bid.ID, OwnerPlayer) {
		t.Fatal("restored order not cancellable")
	}
	if !s.CancelOrder(station, bid.ID, OwnerPlayer) {
		t.Fatal("live order not cancellable")
	}
	if b.Wallet() != s.Wallet() {
		t.Fatalf("wallets diverged after matching cancels: %v vs %v", b.Wallet(), s.Wallet())
	}

	// Twin runs keep lockstep after the round-trip.
	for i := 0; i < 600; i++ {
		s.Step()
		b.Step()
	}
	if s.Hash() != b.Hash() {
		t.Fatalf("twin runs diverged: %016x vs %016x", s.Hash(), b.Hash())
	}
}

// TestRestoreRejectsOldSchemas pins the clean break: v2 saves are migrated to
// v3 (shell bays), and anything older (v0/v1 saves, which carried per-system
// stock lists) is rejected before touching state.
func TestRestoreRejectsOldSchemas(t *testing.T) {
	s := newTestSim(t)
	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if save.Schema != saveSchemaV3 || saveSchemaV3 != 3 {
		t.Fatalf("schema = %d (const %d), want 3", save.Schema, saveSchemaV3)
	}
	save.Schema = 1 // a pre-v2 save: no books, no sides, no arrival ids
	err = s.Restore(save)
	if err == nil {
		t.Fatal("a v1 save restored into a v2 sim")
	}
	if !strings.Contains(err.Error(), "schema") {
		t.Fatalf("error = %v, want a schema complaint", err)
	}
}

// TestEmptyBookKeepsItsLastTrade: a station whose orders were all filled
// still persists its book — the last-trade rung outlives the orders, and a
// save round-trip must agree on that (an all-filled book is not the same
// state as a book that never existed).
func TestEmptyBookKeepsItsLastTrade(t *testing.T) {
	s, err := New(twoStationContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	anvil := StationID("cel-station-b") // a station with no founding orders
	if _, ok := s.PlaceOrder(anvil, OrderSell, "min-nickel", 5, 7, OwnerSystem); !ok {
		t.Fatal("ask refused")
	}
	if _, ok := s.PlaceOrder(anvil, OrderBuy, "min-nickel", 5, 7, OwnerSystem); !ok {
		t.Fatal("crossing bid refused")
	}
	if orders := s.BookOrders(anvil); len(orders) != 0 {
		t.Fatalf("orders = %+v, want the book emptied by the trade", orders)
	}
	if p := s.MarketPrice(anvil, "min-nickel"); p != 7 {
		t.Fatalf("ladder = %v, want the last trade 7", p)
	}
	wantHash := s.Hash()

	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(save.MarketLasts) == 0 {
		t.Fatal("the last trade was not persisted")
	}
	b, err := New(twoStationContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Restore(save); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := b.Hash(); got != wantHash {
		t.Fatalf("hash after restore = %016x, want %016x", got, wantHash)
	}
	if p := b.MarketPrice(anvil, "min-nickel"); p != 7 {
		t.Fatalf("restored ladder = %v, want the last trade 7", p)
	}
}

// TestMarketTwinRunsIdentical: the same seed and the same scripted market
// activity produce identical hashes — placement, fills and cancels draw no
// randomness and iterate no map order.
func TestMarketTwinRunsIdentical(t *testing.T) {
	run := func() *Sim {
		s := newTestSim(t)
		dockAtHaven(t, s)
		s.Queue(CmdBuy{Item: "ammo-emp-s", Qty: 40})
		s.Queue(CmdSell{Item: "ammo-emp-s", Qty: 10})
		s.Step()
		drain(s)
		s.PlaceOrder(StationID("cel-station-a"), OrderBuy, "min-nickel", 5, 2, OwnerPlayer)
		s.PlaceOrder(StationID("cel-station-a"), OrderSell, "min-nickel", 5, 30, OwnerSystem)
		for i := 0; i < 120; i++ {
			s.Step()
			drain(s)
		}
		return s
	}
	a, b := run(), run()
	if a.Hash() != b.Hash() {
		t.Fatalf("twin market runs diverged: %016x vs %016x", a.Hash(), b.Hash())
	}
}
