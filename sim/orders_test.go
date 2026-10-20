package sim

import (
	"testing"

	"github.com/entieve/entieve/content"
)

// ---------------------------------------------------------------------------
// Player standing orders (the command layer over PlaceOrder/CancelOrder):
// both sides, partial fills accumulating across ticks, the slot cap, the
// free-market prices, cancel semantics, station locality and persistence.
// ---------------------------------------------------------------------------

// playerOrdersAt returns the player-owned resting orders of one station, in
// book order.
func playerOrdersAt(s *Sim, station StationID) []MarketStock {
	var out []MarketStock
	for _, o := range s.BookOrders(station) {
		if o.Owner == OwnerPlayer {
			out = append(out, o)
		}
	}
	return out
}

// TestPlaceOrderCommandRestsAndCancels: a bid that does not cross posts
// through the command layer, escrows price×qty + the 3% broker fee, and a
// cancel refunds the escrow while keeping the fee (the engine's rule).
func TestPlaceOrderCommandRestsAndCancels(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	station := StationID("cel-station-a")
	walletBefore := s.Wallet()

	// 3.00 is under the founding 5.00 ask: nothing crosses, all 10 rest.
	evs := placeCmd(s, OrderBuy, "min-nickel", 10, 3)
	if !hasEvent(evs, EvMarket, "Placed buy order: 10 x Nickel at 3.00 €") {
		t.Fatalf("no placement line: %v", evs)
	}
	mine := playerOrdersAt(s, station)
	if len(mine) != 1 || mine[0].Side != OrderBuy || mine[0].Qty != 10 || mine[0].Price != 3 {
		t.Fatalf("resting = %+v, want one 10 @ 3 bid", mine)
	}
	escrow := 3 * 10.0
	fee := MarketBrokerFee * escrow
	if s.Wallet() != walletBefore-(escrow+fee) {
		t.Fatalf("wallet = %v, want %v (escrow %v + fee %v)",
			s.Wallet(), walletBefore-(escrow+fee), escrow, fee)
	}

	// Cancel: refund the escrow, keep the fee (EVE's rule).
	s.Queue(CmdCancelOrder{ID: mine[0].ID})
	s.Step()
	evs = drain(s)
	if !hasEvent(evs, EvMarket, "Order cancelled") {
		t.Fatalf("no cancel line: %v", evs)
	}
	if s.Wallet() != walletBefore-fee {
		t.Fatalf("wallet = %v, want %v (escrow back, %v fee kept)",
			s.Wallet(), walletBefore-fee, fee)
	}
	if len(playerOrdersAt(s, station)) != 0 {
		t.Fatal("cancelled order still rests")
	}
	// A second cancel of the same id is a plain refusal.
	s.Queue(CmdCancelOrder{ID: mine[0].ID})
	s.Step()
	if evs := drain(s); !hasEvent(evs, EvLog, "No such order") {
		t.Fatalf("expected a No such order refusal: %v", evs)
	}
	assertBooksSane(t, s)
}

// TestPlaceOrderCommandAskEscrowsGoods: a sell command escrows the goods
// (out of cargo), and its cancel returns them to the station hangar.
func TestPlaceOrderCommandAskEscrowsGoods(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	station := StationID("cel-station-a")
	giveCargo(s, "min-nickel", 10)

	// No bid for nickel: the whole ask rests (goods escrowed in it).
	evs := placeCmd(s, OrderSell, "min-nickel", 10, 10)
	if !hasEvent(evs, EvMarket, "Placed sell order: 10 x Nickel at 10.00 €") {
		t.Fatalf("no placement line: %v", evs)
	}
	if cargoCount(s, "min-nickel") != 0 {
		t.Fatalf("cargo = %d, want 0 (goods escrowed)", cargoCount(s, "min-nickel"))
	}
	mine := playerOrdersAt(s, station)
	if len(mine) != 1 || mine[0].Side != OrderSell {
		t.Fatalf("resting = %+v, want one ask", mine)
	}
	s.Queue(CmdCancelOrder{ID: mine[0].ID})
	s.Step()
	drain(s)
	if hangarCount(s, "min-nickel") != 10 {
		t.Fatalf("hangar = %d, want 10 (escrow returned)", hangarCount(s, "min-nickel"))
	}
}

// TestPlaceOrderCommandCrossesAndRests: a marketable bid fills against the
// resting ask at the RESTING price; only the remainder rests, and the
// broker fee rides on the placed value.
func TestPlaceOrderCommandCrossesAndRests(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	station := StationID("cel-station-a")
	walletBefore := s.Wallet()

	// 100 ammo-emp-s @ 5.00 rest founding; bid 120 @ 5.00: 100 fill, 20 rest.
	evs := placeCmd(s, OrderBuy, "ammo-emp-s", 120, 5)
	if !hasEvent(evs, EvMarket, "Bought 100 x EMP Charge for 500.00 €") {
		t.Fatalf("no fill line: %v", evs)
	}
	if hangarCount(s, "ammo-emp-s") != 100 {
		t.Fatalf("hangar = %d, want 100 delivered", hangarCount(s, "ammo-emp-s"))
	}
	fee := MarketBrokerFee * (5 * 120.0) // fee on the placed value
	if s.Wallet() != walletBefore-(600+fee) {
		t.Fatalf("wallet = %v, want %v (600 escrowed for the full placed value + %v fee)",
			s.Wallet(), walletBefore-(600+fee), fee)
	}
	var rest []MarketStock
	for _, o := range playerOrdersAt(s, station) {
		if o.Item == "ammo-emp-s" {
			rest = append(rest, o)
		}
	}
	if len(rest) != 1 || rest[0].Qty != 20 || rest[0].Price != 5 {
		t.Fatalf("remainder = %+v, want 20 @ 5 resting", rest)
	}
	assertBooksSane(t, s)
}

// TestRestingOrderPartialFillsAccumulate: a resting player bid shrinks as
// later orders cross it tick by tick — partial fills accumulate on the one
// order until it is exhausted, then it is gone.
func TestRestingOrderPartialFillsAccumulate(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	station := StationID("cel-station-a")
	giveCargo(s, "min-nickel", 20)

	// Tick 1: rest a 10 @ 3 bid (nickel has no founding counterparty).
	placeCmd(s, OrderBuy, "min-nickel", 10, 3)
	bid := playerOrdersAt(s, station)[0]

	// Tick 2: an ask at 3 crosses it for 6, leaving 4 resting.
	placeCmd(s, OrderSell, "min-nickel", 6, 3)
	mine := playerOrdersAt(s, station)
	if len(mine) != 1 || mine[0].ID != bid.ID || mine[0].Qty != 4 {
		t.Fatalf("after first fill = %+v, want the SAME bid down to 4", mine)
	}

	// Tick 3: a 5-unit ask takes the last 4 and rests 1 of its own.
	placeCmd(s, OrderSell, "min-nickel", 5, 3)
	mine = playerOrdersAt(s, station)
	if len(mine) != 1 || mine[0].Side != OrderSell || mine[0].Qty != 1 {
		t.Fatalf("after second fill = %+v, want a lone 1-unit ask", mine)
	}
	// 6 + 4 = 10 units received into the hangar across the two fills.
	if hangarCount(s, "min-nickel") != 10 {
		t.Fatalf("hangar = %d, want 10 filled", hangarCount(s, "min-nickel"))
	}
	assertBooksSane(t, s)
}

// TestPlaceOrderCap: the 21st open player order is refused with the house
// SevWarning line; the first 20 rest.
func TestPlaceOrderCap(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	station := StationID("cel-station-a")

	for range maxPlayerOrders + 1 {
		s.Queue(CmdPlaceOrder{Item: "min-nickel", Side: OrderBuy, Price: 1, Qty: 1})
	}
	s.Step()
	evs := drain(s)

	if !hasEvent(evs, EvLog, "Maximum number of open orders reached (20)") {
		t.Fatalf("no cap refusal line: %v", evs)
	}
	if n := len(playerOrdersAt(s, station)); n != maxPlayerOrders {
		t.Fatalf("resting player orders = %d, want %d", n, maxPlayerOrders)
	}
	// The cap is universe-wide order slots, not per station.
	if n := s.playerOrderCount(); n != maxPlayerOrders {
		t.Fatalf("playerOrderCount = %d, want %d", n, maxPlayerOrders)
	}
	assertBooksSane(t, s)
}

// TestPlayerOrderAnyPrice: the free market clamps nothing — the 0.01 € and
// 1e12 € extremes both rest exactly where they were asked to.
func TestPlayerOrderAnyPrice(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	station := StationID("cel-station-a")
	s.wallet = 1e15 // fund the 1e12 escrow

	placeCmd(s, OrderBuy, "min-nickel", 1, 0.01)
	placeCmd(s, OrderBuy, "min-nickel", 1, 1e12)

	got := map[float64]bool{}
	for _, o := range playerOrdersAt(s, station) {
		got[o.Price] = true
	}
	if !got[0.01] || !got[1e12] {
		t.Fatalf("resting prices = %v, want both 0.01 and 1e12 legal", got)
	}
}

// TestPlaceOrderCommandRefusals: bad side, zero quantity and non-positive
// price are refused up front with SevWarning lines and no state change.
func TestPlaceOrderCommandRefusals(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	walletBefore := s.Wallet()

	for _, tc := range []struct {
		name string
		cmd  CmdPlaceOrder
		want string
	}{
		{"bad side", CmdPlaceOrder{Item: "min-nickel", Side: OrderSide("hold"), Price: 3, Qty: 1}, "Invalid order side"},
		{"zero qty", CmdPlaceOrder{Item: "min-nickel", Side: OrderBuy, Price: 3, Qty: 0}, "Invalid quantity"},
		{"zero price", CmdPlaceOrder{Item: "min-nickel", Side: OrderBuy, Price: 0, Qty: 1}, "Invalid price"},
		{"negative price", CmdPlaceOrder{Item: "min-nickel", Side: OrderBuy, Price: -1, Qty: 1}, "Invalid price"},
		{"unknown item", CmdPlaceOrder{Item: "item-nope", Side: OrderBuy, Price: 3, Qty: 1}, "Unknown item"},
	} {
		s.Queue(tc.cmd)
		s.Step()
		if evs := drain(s); findEvent(evs, EvLog, tc.want) == nil {
			t.Fatalf("%s: no %q refusal line: %v", tc.name, tc.want, evs)
		}
	}
	if s.Wallet() != walletBefore || len(playerOrdersAt(s, StationID("cel-station-a"))) != 0 {
		t.Fatal("a refused placement moved state")
	}

	// Undocked: the market gate refuses before anything else.
	s.Queue(CmdUndock{})
	s.Step()
	drain(s)
	s.Queue(CmdPlaceOrder{Item: "min-nickel", Side: OrderBuy, Price: 3, Qty: 1})
	s.Step()
	if evs := drain(s); findEvent(evs, EvLog, "must be docked") == nil {
		t.Fatalf("undocked placement not refused: %v", evs)
	}
}

// TestPlayerOrdersAreStationLocal: an order rests only on the docked
// station's book; the same item placed at another station never lists (or
// crosses) the first.
func TestPlayerOrdersAreStationLocal(t *testing.T) {
	s, err := New(twoStationContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	haven := StationID("cel-station-a")
	anvil := StationID("cel-station-b")

	dockAtHaven(t, s)
	placeCmd(s, OrderBuy, "min-nickel", 10, 3) // rests at Haven Rest

	// Travel to Anvil Stop and place the same item there.
	s.Queue(CmdUndock{})
	s.Step()
	drain(s)
	anvilE := entityByName(s, "Anvil Stop")
	if anvilE.IsZero() {
		t.Fatal("no Anvil Stop entity")
	}
	teleport(s, s.player, mustPos(s, anvilE))
	dockAtStation(t, s, "Anvil Stop")
	if got := s.DockedMarket(); got != anvil {
		t.Fatalf("docked market = %q, want %q", got, anvil)
	}

	if n := len(playerOrdersAt(s, anvil)); n != 0 {
		t.Fatalf("Anvil Stop already lists %d player orders (not station-local)", n)
	}
	placeCmd(s, OrderBuy, "min-nickel", 5, 2)

	if n := len(playerOrdersAt(s, haven)); n != 1 {
		t.Fatalf("Haven Rest player orders = %d, want the one placed there", n)
	}
	if n := len(playerOrdersAt(s, anvil)); n != 1 {
		t.Fatalf("Anvil Stop player orders = %d, want its own one", n)
	}
	assertBooksSane(t, s)
}

// TestPlayerOrdersSurviveSaveRestore: a partially filled player order (its
// escrow bookkeeping and remaining quantity included) rides the existing
// market persistence — player-owned orders already save through
// Save.Market — and the restored sim cancels it for exactly the same
// refund as the live one.
func TestPlayerOrdersSurviveSaveRestore(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	station := StationID("cel-station-a")
	giveCargo(s, "min-nickel", 20)

	// Rest a 10 @ 3 bid, then cross 6 with an own ask: 4 remain mid-flight.
	placeCmd(s, OrderBuy, "min-nickel", 10, 3)
	placeCmd(s, OrderSell, "min-nickel", 6, 3)
	var bid MarketStock
	for _, o := range playerOrdersAt(s, station) {
		if o.Side == OrderBuy {
			bid = o
		}
	}
	if bid.Qty != 4 {
		t.Fatalf("resting bid qty = %d, want 4 (partial fill)", bid.Qty)
	}
	wantHash := s.Hash()

	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	found := false
	for _, o := range save.Market["sys-a"] {
		if o.ID == bid.ID && o.Owner == OwnerPlayer && o.Qty == 4 && o.Side == OrderBuy {
			found = true
		}
	}
	if !found {
		t.Fatalf("the partially filled player order is missing from the save: %+v", save.Market["sys-a"])
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
	if n := len(playerOrdersAt(b, station)); n != 1 {
		t.Fatalf("restored player orders = %d, want 1", n)
	}
	// The escrow bookkeeping restored with the order: cancelling refunds
	// exactly what the live sim would.
	if !b.CancelOrder(station, bid.ID, OwnerPlayer) {
		t.Fatal("restored order not cancellable")
	}
	if !s.CancelOrder(station, bid.ID, OwnerPlayer) {
		t.Fatal("live order not cancellable")
	}
	if b.Wallet() != s.Wallet() {
		t.Fatalf("wallets diverged after matching cancels: %v vs %v", b.Wallet(), s.Wallet())
	}
}

// TestPlayerOrderTwinRuns: the same seed and the same scripted standing
// orders produce identical hashes — placement, partial fills and cancels
// draw no randomness and iterate no map order.
func TestPlayerOrderTwinRuns(t *testing.T) {
	run := func() *Sim {
		s := newTestSim(t)
		dockAtHaven(t, s)
		giveCargo(s, "min-nickel", 20)
		s.Queue(CmdPlaceOrder{Item: "min-nickel", Side: OrderBuy, Price: 3, Qty: 10})
		s.Queue(CmdPlaceOrder{Item: "min-nickel", Side: OrderSell, Price: 3, Qty: 6})
		s.Step()
		drain(s)
		for i := 0; i < 90; i++ {
			s.Step()
			drain(s)
		}
		return s
	}
	a, b := run(), run()
	if a.Hash() != b.Hash() {
		t.Fatalf("twin standing-order runs diverged: %016x vs %016x", a.Hash(), b.Hash())
	}
}

// placeCmd queues a CmdPlaceOrder and steps once, returning the events.
func placeCmd(s *Sim, side OrderSide, item content.ItemID, qty int, price float64) []Event {
	s.Queue(CmdPlaceOrder{Item: item, Side: side, Price: price, Qty: qty})
	s.Step()
	return drain(s)
}
