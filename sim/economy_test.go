package sim

import (
	"math"
	"testing"

	"github.com/entieve/entieve/content"
)

// dockAtHaven docks the player at Haven Rest and drains events.
func dockAtHaven(t *testing.T, s *Sim) []Event {
	t.Helper()
	station := entityByName(s, "Haven Rest")
	if station.IsZero() {
		t.Fatal("no station entity")
	}
	s.Queue(CmdDock{Station: station})
	s.Step()
	evs := drain(s)
	if !hasEvent(evs, EvDocked, "Docked at Haven Rest") {
		t.Fatalf("docking failed: %v", evs)
	}
	return evs
}

// marketQty returns the best ask's remaining qty and price for an item in
// sys-a (the station's founding supply; asks rank before bids canonically).
func marketQty(s *Sim, item content.ItemID) (qty int, price float64, ok bool) {
	for _, st := range s.Market("sys-a") {
		if st.Item == item && st.Side == OrderSell {
			return st.Qty, st.Price, true
		}
	}
	return 0, 0, false
}

// bidPrice returns an item's best standing bid in sys-a (0 when the
// station bids nothing for it).
func bidPrice(s *Sim, item content.ItemID) float64 {
	for _, st := range s.Market("sys-a") {
		if st.Item == item && st.Side == OrderBuy {
			return st.Price // canonical order: best bid first
		}
	}
	return 0
}

// bidQty returns the qty resting on an item's best bid in sys-a.
func bidQty(s *Sim, item content.ItemID) int {
	for _, st := range s.Market("sys-a") {
		if st.Item == item && st.Side == OrderBuy {
			return st.Qty
		}
	}
	return 0
}

func hangarCount(s *Sim, item content.ItemID) int { return stacksCount(s.Hangar.Items, item) }

// hangarHulls counts stored hulls of one type in the station hangar.
func hangarHulls(s *Sim, hull content.ShipID) int {
	n := 0
	for _, sh := range s.Hangar.Ships {
		if sh.Hull == hull {
			n++
		}
	}
	return n
}

// TestBuyHullQtyDeliversEveryHull pins the hull branch of CmdBuy: N hulls
// cost N prices, take N units off the shelf and deliver N StoredShips (the
// item branch stacks; sell takes hulls one per unit — the buy side must
// match), and the qty > stock clamp plus the out-of-stock refusal stay
// exactly as documented.
func TestBuyHullQtyDeliversEveryHull(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	s.wallet = 300000 // budget for three 50k hulls
	stockBefore, price, _ := marketQty(s, "ship-barge")
	if stockBefore < 3 {
		t.Fatalf("fixture: stock=%d want >= 3", stockBefore)
	}
	walletBefore := s.Wallet()

	// Qty 3: charge 3x plus the broker fee, stock -3, three hulls delivered.
	s.Queue(CmdBuy{Item: "ship-barge", Qty: 3})
	s.Step()
	evs := drain(s)
	cost, fee := 3*price, MarketBrokerFee*(3*price)
	if s.Wallet() != walletBefore-(cost+fee) {
		t.Fatalf("wallet=%v want %v (3 x %v + %v broker fee)", s.Wallet(),
			walletBefore-(cost+fee), cost, fee)
	}
	stockAfter, _, _ := marketQty(s, "ship-barge")
	if stockAfter != stockBefore-3 {
		t.Fatalf("stock=%d want %d", stockAfter, stockBefore-3)
	}
	if n := hangarHulls(s, "ship-barge"); n != 3 {
		t.Fatalf("hangar holds %d hulls, want 3 (one per unit bought)", n)
	}
	for _, sh := range s.Hangar.Ships {
		if sh.Hull != "ship-barge" {
			t.Fatalf("unexpected stored hull %q", sh.Hull)
		}
	}
	// Hulls report their display name in market text (itemName resolves
	// items, modules and hulls alike).
	if !hasEvent(evs, EvMarket, "Bought 3 x Ore Barge for 150,000.00 €") {
		t.Fatalf("missing buy event: %v", evs)
	}

	// Qty above the ask clamps to what the book holds (immediate-or-cancel):
	// charge and deliver exactly the resting units, never a phantom hull.
	// (Top the ask back up — the founding ask was exhausted by the first
	// buy — so the clamp has something to work against.)
	if _, ok := s.PlaceOrder(StationID("cel-station-a"), OrderSell, "ship-barge", 2, price, OwnerSystem); !ok {
		t.Fatal("fixture: could not reseed the ask")
	}
	s.wallet = 300000
	s.Queue(CmdBuy{Item: "ship-barge", Qty: 5})
	s.Step()
	evs = drain(s)
	cost, fee = 2*price, MarketBrokerFee*(2*price)
	if s.Wallet() != 300000-(cost+fee) {
		t.Fatalf("wallet=%v want %v (clamped to the 2 units on the ask, %v + %v fee)",
			s.Wallet(), 300000-(cost+fee), cost, fee)
	}
	if n := hangarHulls(s, "ship-barge"); n != 5 {
		t.Fatalf("hangar holds %d hulls, want 5", n)
	}
	stockAfter, _, _ = marketQty(s, "ship-barge")
	if stockAfter != 0 {
		t.Fatalf("stock=%d want 0", stockAfter)
	}

	// Empty book: refusal, nothing moves (mirrors the unlisted refusal).
	walletRefused := s.Wallet()
	hullsRefused := hangarHulls(s, "ship-barge")
	s.Queue(CmdBuy{Item: "ship-barge", Qty: 1})
	s.Step()
	evs = drain(s)
	if findEvent(evs, EvLog, "Ore Barge is not sold here") == nil {
		t.Fatalf("missing empty-book refusal: %v", evs)
	}
	if s.Wallet() != walletRefused || hangarHulls(s, "ship-barge") != hullsRefused {
		t.Fatalf("refusal moved state: wallet=%v hulls=%d", s.Wallet(),
			hangarHulls(s, "ship-barge"))
	}
}

func TestBuyReducesStockAndWallet(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	qtyBefore, price, _ := marketQty(s, "ammo-emp-s")
	walletBefore := s.Wallet()

	s.Queue(CmdBuy{Item: "ammo-emp-s", Qty: 10})
	s.Step()
	evs := drain(s)

	if s.Wallet() != walletBefore-(price*10+MarketBrokerFee*(price*10)) {
		t.Fatalf("wallet=%v want %v", s.Wallet(),
			walletBefore-(price*10+MarketBrokerFee*(price*10)))
	}
	qtyAfter, _, _ := marketQty(s, "ammo-emp-s")
	if qtyAfter != qtyBefore-10 {
		t.Fatalf("stock=%d want %d", qtyAfter, qtyBefore-10)
	}
	if hangarCount(s, "ammo-emp-s") != 10 {
		t.Fatalf("hangar=%d want 10", hangarCount(s, "ammo-emp-s"))
	}
	if !hasEvent(evs, EvMarket, "Bought 10 x EMP Charge for 50.00 €") {
		t.Fatalf("missing buy event: %v", evs)
	}
}

func TestBuyRequiresDockedMarketService(t *testing.T) {
	s := newTestSim(t)
	// Not docked.
	s.Queue(CmdBuy{Item: "ammo-emp-s", Qty: 1})
	s.Step()
	evs := drain(s)
	if findEvent(evs, EvLog, "must be docked") == nil {
		t.Fatalf("expected not-docked warning: %v", evs)
	}
	if s.Wallet() != 100000 {
		t.Fatal("wallet changed while not docked")
	}
	// Unlisted item.
	dockAtHaven(t, s)
	s.Queue(CmdBuy{Item: "loot-navdata", Qty: 1})
	s.Step()
	evs = drain(s)
	if findEvent(evs, EvLog, "not sold here") == nil {
		t.Fatalf("expected not-for-sale warning: %v", evs)
	}
}

func TestBuyInsufficientFundsRejected(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	qtyBefore, price, _ := marketQty(s, "ship-barge")
	// 3 barges at 50k = 150k > 100k wallet.
	s.Queue(CmdBuy{Item: "ship-barge", Qty: qtyBefore})
	s.Step()
	evs := drain(s)
	if findEvent(evs, EvMarket, "Insufficient funds") == nil {
		t.Fatalf("expected insufficient-funds event: %v", evs)
	}
	if s.Wallet() != 100000 {
		t.Fatalf("wallet=%v want unchanged", s.Wallet())
	}
	qtyAfter, _, _ := marketQty(s, "ship-barge")
	if qtyAfter != qtyBefore {
		t.Fatalf("stock=%d want unchanged %d @ %v", qtyAfter, qtyBefore, price)
	}
	// One barge (50k + broker fee) is affordable and lands in the hangar.
	s.Queue(CmdBuy{Item: "ship-barge", Qty: 1})
	s.Step()
	drain(s)
	want := 100000 - (50000 + MarketBrokerFee*50000)
	if s.Wallet() != want || len(s.Hangar.Ships) != 1 {
		t.Fatalf("wallet=%v want %v ships=%d", s.Wallet(), want, len(s.Hangar.Ships))
	}
	if s.Hangar.Ships[0].Hull != "ship-barge" {
		t.Fatalf("hangar ship=%s", s.Hangar.Ships[0].Hull)
	}
}

// TestSellFillsBidMinusTax pins the sell side of the double auction: the
// player sells into the station's standing BID at the bid's price (the
// resting order's price wins), pays the sales tax on execution, and the
// bid — not some shelf — absorbs the goods.
func TestSellFillsBidMinusTax(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	walletBefore := s.Wallet()
	bid := bidPrice(s, "ammo-emp-s")
	if bid <= 0 {
		t.Fatal("fixture: no standing bid for ammo-emp-s")
	}
	bidBefore := bidQty(s, "ammo-emp-s")
	ammoBefore := cargoCount(s, "ammo-emp-s")

	s.Queue(CmdSell{Item: "ammo-emp-s", Qty: 5})
	s.Step()
	evs := drain(s)

	proceeds := bid * 5
	fee := MarketBrokerFee * proceeds
	tax := MarketSalesTax * proceeds
	received := proceeds - tax - fee
	if s.Wallet() != walletBefore+(received) {
		t.Fatalf("wallet=%v want %v (bid %v x 5, %v broker fee, %v sales tax)", s.Wallet(),
			walletBefore+received, bid, fee, tax)
	}
	if cargoCount(s, "ammo-emp-s") != ammoBefore-5 {
		t.Fatal("cargo not reduced")
	}
	if got := bidQty(s, "ammo-emp-s"); got != bidBefore-5 {
		t.Fatalf("bid qty=%d want %d (the bid absorbed the sale)", got, bidBefore-5)
	}
	// The ask side is untouched: selling never restocks supply.
	if qty, _, _ := marketQty(s, "ammo-emp-s"); qty == 0 {
		t.Fatal("selling consumed the ask side")
	}
	if !hasEvent(evs, EvMarket, "Sold 5 x EMP Charge for "+euro(received)) {
		t.Fatalf("missing sell event: %v", evs)
	}
}

// TestSellWithoutBidsRefuses: an item nobody bids on cannot be sold. The
// book is the price former — there is no shelf to restock and no base
// price to conjure a sale from (the old unlisted-base-price sale is gone
// with the per-system stock list).
func TestSellWithoutBidsRefuses(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	giveCargo(s, "min-nickel", 3) // nobody bids on nickel here
	walletBefore := s.Wallet()
	s.Queue(CmdSell{Item: "min-nickel", Qty: 3})
	s.Step()
	evs := drain(s)
	if findEvent(evs, EvLog, "No buy orders for Nickel") == nil {
		t.Fatalf("expected no-bids refusal: %v", evs)
	}
	if s.Wallet() != walletBefore {
		t.Fatalf("wallet=%v want unchanged", s.Wallet())
	}
	if cargoCount(s, "min-nickel") != 3 {
		t.Fatal("refusal moved cargo")
	}
}

func TestSellFallsBackToHangar(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	s.Hangar.Items = addStacks(s.Hangar.Items, "min-ferrite", 7)
	walletBefore := s.Wallet()
	bid := bidPrice(s, "min-ferrite")
	if bid <= 0 {
		t.Fatal("fixture: no standing bid for min-ferrite")
	}
	s.Queue(CmdSell{Item: "min-ferrite", Qty: 7})
	s.Step()
	evs := drain(s)
	// Sold into the station's bid at 9, minus the broker fee and the 2.5%
	// sales tax (both come out of the proceeds).
	proceeds := bid * 7
	fee := MarketBrokerFee * proceeds
	tax := MarketSalesTax * proceeds
	received := proceeds - tax - fee
	if s.Wallet() != walletBefore+received {
		t.Fatalf("wallet=%v want %v (bid %v x 7, %v broker fee, %v sales tax)", s.Wallet(),
			walletBefore+received, bid, fee, tax)
	}
	if hangarCount(s, "min-ferrite") != 0 {
		t.Fatal("hangar not drained")
	}
	if !hasEvent(evs, EvMarket, "Sold 7 x Ferrite for "+euro(received)) {
		t.Fatalf("missing event: %v", evs)
	}
	// Nothing to sell warns.
	s.Queue(CmdSell{Item: "min-ferrite", Qty: 7})
	s.Step()
	evs = drain(s)
	if findEvent(evs, EvLog, "Nothing to sell") == nil {
		t.Fatalf("expected warning: %v", evs)
	}
}

func TestRefineWholeBatches(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	giveCargo(s, "ore-sterngrit", 100)
	s.Queue(CmdRefine{Item: "ore-sterngrit", Qty: 100})
	s.Step()
	evs := drain(s)

	if cargoCount(s, "ore-sterngrit") != 0 {
		t.Fatal("ore not consumed")
	}
	// floor(0.5*100) = 50 ferrite, floor(0.25*100) = 25 nickel — a batch
	// makes the fixture's fractions exact.
	if cargoCount(s, "min-ferrite") != 50 {
		t.Fatalf("ferrite=%d want 50", cargoCount(s, "min-ferrite"))
	}
	if cargoCount(s, "min-nickel") != 25 {
		t.Fatalf("nickel=%d want 25", cargoCount(s, "min-nickel"))
	}
	if !hasEvent(evs, EvLog, "Refined 100 x Sterngrit into 50 x Ferrite, 25 x Nickel") {
		t.Fatalf("missing refine event: %v", evs)
	}

	// Partial lots are refused untouched: a unit or two would floor to
	// nothing while still eating the ore.
	giveCargo(s, "ore-sterngrit", 100)
	for _, qty := range []int{1, 10, 50} {
		s.Queue(CmdRefine{Item: "ore-sterngrit", Qty: qty})
		s.Step()
		evs = drain(s)
		if findEvent(evs, EvLog, "partial batch") == nil {
			t.Fatalf("qty %d: expected the batch refusal: %v", qty, evs)
		}
		if got := cargoCount(s, "ore-sterngrit"); got != 100 {
			t.Fatalf("qty %d: refused refine consumed ore (left %d)", qty, got)
		}
	}

	// A stock that is not a multiple of the batch never refines partially:
	// asking 300 with 150 held refuses the clamped 150 outright.
	giveCargo(s, "ore-sterngrit", 50)
	s.Queue(CmdRefine{Item: "ore-sterngrit", Qty: 300})
	s.Step()
	evs = drain(s)
	if findEvent(evs, EvLog, "150 x Sterngrit is a partial batch") == nil {
		t.Fatalf("expected the partial-batch refusal: %v", evs)
	}
	if got := cargoCount(s, "ore-sterngrit"); got != 150 {
		t.Fatalf("refused refine consumed ore (left %d)", got)
	}

	// One whole batch still goes through, leaving the odd 50 behind.
	s.Queue(CmdRefine{Item: "ore-sterngrit", Qty: 100})
	s.Step()
	drain(s)
	if got := cargoCount(s, "ore-sterngrit"); got != 50 {
		t.Fatalf("ore left=%d want 50", got)
	}
	if got := cargoCount(s, "min-ferrite"); got != 100 {
		t.Fatalf("ferrite=%d want 100", got)
	}
}

func TestRefineRequiresOreAndService(t *testing.T) {
	s := newTestSim(t)
	// Not docked.
	s.Queue(CmdRefine{Item: "ore-sterngrit", Qty: 100})
	s.Step()
	evs := drain(s)
	if findEvent(evs, EvLog, "must be docked") == nil {
		t.Fatalf("expected not-docked warning: %v", evs)
	}
	// Docked but no ore.
	dockAtHaven(t, s)
	s.Queue(CmdRefine{Item: "ore-sterngrit", Qty: 100})
	s.Step()
	evs = drain(s)
	if findEvent(evs, EvLog, "No Sterngrit to refine") == nil {
		t.Fatalf("expected no-ore warning: %v", evs)
	}
	// Non-ore item.
	giveCargo(s, "min-ferrite", 5)
	s.Queue(CmdRefine{Item: "min-ferrite", Qty: 5})
	s.Step()
	evs = drain(s)
	if findEvent(evs, EvLog, "Cannot refine Ferrite") == nil {
		t.Fatalf("expected cannot-refine warning: %v", evs)
	}
}

func TestRepairCostsTwoCreditsPerHP(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	ship, _ := s.Ships.Get(s.player)
	ship.Shield = 300 // 200 missing
	ship.Armor = 450  // 50 missing
	// Structure full.
	walletBefore := s.Wallet()

	s.Queue(CmdRepair{})
	s.Step()
	evs := drain(s)
	if ship.Shield != ship.MaxShield || ship.Armor != ship.MaxArmor ||
		ship.Structure != ship.MaxStructure {
		t.Fatalf("not fully repaired: %v/%v/%v", ship.Shield, ship.Armor, ship.Structure)
	}
	if math.Abs(s.Wallet()-(walletBefore-500)) > 1e-9 { // (200+50)*2
		t.Fatalf("wallet=%v want %v", s.Wallet(), walletBefore-500)
	}
	if !hasEvent(evs, EvLog, "Repaired 250.0 HP for 500.00 €") {
		t.Fatalf("missing repair event: %v", evs)
	}
	// Already full: informational only.
	s.Queue(CmdRepair{})
	s.Step()
	evs = drain(s)
	if findEvent(evs, EvLog, "already at full strength") == nil {
		t.Fatalf("expected info event: %v", evs)
	}
}

func TestRepairRejectsWhenBroke(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	ship, _ := s.Ships.Get(s.player)
	ship.Shield = 0
	s.wallet = 10 // insufficient for 1000 HP * 2
	s.Queue(CmdRepair{})
	s.Step()
	evs := drain(s)
	if findEvent(evs, EvLog, "Insufficient funds for repairs") == nil {
		t.Fatalf("expected funds warning: %v", evs)
	}
	if ship.Shield != 0 {
		t.Fatal("repair must not apply")
	}
}

func TestFitAndUnfitModule(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	ship, _ := s.Ships.Get(s.player)
	s.Hangar.Items = addStacks(s.Hangar.Items, "mod-armor-plate", 1)

	s.Queue(CmdFitModule{Item: "mod-armor-plate", Slot: SlotRef{Class: content.SlotLow, Index: 0}})
	s.Step()
	evs := drain(s)
	fit, _ := s.Fittings.Get(s.player)
	if len(fit.Low) != 1 || fit.Low[0] != "mod-armor-plate" {
		t.Fatalf("fit.Low=%v", fit.Low)
	}
	if ship.MaxArmor != 750 {
		t.Fatalf("MaxArmor=%v want 750", ship.MaxArmor)
	}
	if hangarCount(s, "mod-armor-plate") != 0 {
		t.Fatal("hangar not drained on fit")
	}
	if !hasEvent(evs, EvModule, "Armor Plate fitted") {
		t.Fatalf("missing fit event: %v", evs)
	}

	s.Queue(CmdUnfitModule{Slot: SlotRef{Class: content.SlotLow, Index: 0}})
	s.Step()
	evs = drain(s)
	if len(fit.Low) != 0 {
		t.Fatalf("fit.Low after unfit=%v", fit.Low)
	}
	if ship.MaxArmor != 500 {
		t.Fatalf("MaxArmor after unfit=%v", ship.MaxArmor)
	}
	if hangarCount(s, "mod-armor-plate") != 1 {
		t.Fatal("hangar not restocked on unfit")
	}
	if !hasEvent(evs, EvModule, "Armor Plate unfit") {
		t.Fatalf("missing unfit event: %v", evs)
	}
}

func TestFitValidation(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	// Wrong slot class.
	s.Queue(CmdFitModule{Item: "mod-stasis-web", Slot: SlotRef{Class: content.SlotLow, Index: 0}})
	s.Step()
	evs := drain(s)
	if findEvent(evs, EvLog, "cannot fit a low slot") == nil {
		t.Fatalf("expected slot-class warning: %v", evs)
	}
	// Not in hangar.
	s.Queue(CmdFitModule{Item: "mod-shield-boost", Slot: SlotRef{Class: content.SlotMid, Index: 0}})
	s.Step()
	evs = drain(s)
	if findEvent(evs, EvLog, "is not in the hangar") == nil {
		t.Fatalf("expected hangar warning: %v", evs)
	}
	// Fit service only while docked (already docked here): unfit invalid index.
	s.Queue(CmdUnfitModule{Slot: SlotRef{Class: content.SlotLow, Index: 5}})
	s.Step()
	evs = drain(s)
	if findEvent(evs, EvLog, "No module in that slot") == nil {
		t.Fatalf("expected slot warning: %v", evs)
	}
}

func TestAssembleSwapsHullsAndPreservesBoth(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	ship, _ := s.Ships.Get(s.player)
	ammoBefore := cargoCount(s, "ammo-emp-s")

	// The hangar holds a barge with its own fit and cargo.
	s.Hangar.Ships = append(s.Hangar.Ships, StoredShip{
		Hull:  "ship-barge",
		Fit:   []content.ModuleID{"mod-shield-extender"},
		Cargo: []content.CargoStack{{Item: "ammo-ap-s", Qty: 7}},
	})

	s.Queue(CmdAssemble{Ship: "ship-barge"})
	s.Step()
	evs := drain(s)
	if ship.Hull != "ship-barge" {
		t.Fatalf("hull=%s", ship.Hull)
	}
	fit, _ := s.Fittings.Get(s.player)
	if len(fit.Low) != 1 || fit.Low[0] != "mod-shield-extender" {
		t.Fatalf("barge fit=%v", fit.Low)
	}
	if cargoCount(s, "ammo-ap-s") != 7 {
		t.Fatalf("barge cargo=%d", cargoCount(s, "ammo-ap-s"))
	}
	if cargoCount(s, "ammo-emp-s") != 0 {
		t.Fatal("sloop cargo should have moved to the hangar")
	}
	if len(s.Hangar.Ships) != 1 || s.Hangar.Ships[0].Hull != "ship-sloop" {
		t.Fatalf("hangar=%+v", s.Hangar.Ships)
	}
	// The stored sloop kept its fit and cargo.
	stored := s.Hangar.Ships[0]
	if len(stored.Fit) != 1 || stored.Fit[0] != "mod-autocannon-s" {
		t.Fatalf("stored fit=%v", stored.Fit)
	}
	if stacksCount(stored.Cargo, "ammo-emp-s") != ammoBefore {
		t.Fatalf("stored cargo=%+v", stored.Cargo)
	}
	if !hasEvent(evs, EvLog, "Assembled Ore Barge") {
		t.Fatalf("missing assemble event: %v", evs)
	}

	// Swap back: everything round-trips.
	s.Queue(CmdAssemble{Ship: "ship-sloop"})
	s.Step()
	drain(s)
	if ship.Hull != "ship-sloop" {
		t.Fatalf("hull after swap back=%s", ship.Hull)
	}
	if cargoCount(s, "ammo-emp-s") != ammoBefore {
		t.Fatal("sloop cargo not restored")
	}
	fit, _ = s.Fittings.Get(s.player)
	if len(fit.High) != 1 || fit.High[0] != "mod-autocannon-s" {
		t.Fatalf("sloop fit=%v", fit.High)
	}
	if len(s.Hangar.Ships) != 1 || s.Hangar.Ships[0].Hull != "ship-barge" {
		t.Fatalf("hangar after swap=%+v", s.Hangar.Ships)
	}
	barge := s.Hangar.Ships[0]
	if stacksCount(barge.Cargo, "ammo-ap-s") != 7 || len(barge.Fit) != 1 {
		t.Fatalf("stored barge corrupted: %+v", barge)
	}
}

func TestAssembleUnknownShipFails(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	s.Queue(CmdAssemble{Ship: "ship-cutlass"})
	s.Step()
	evs := drain(s)
	if findEvent(evs, EvLog, "in the hangar") == nil {
		t.Fatalf("expected warning: %v", evs)
	}
	ship, _ := s.Ships.Get(s.player)
	if ship.Hull != "ship-sloop" {
		t.Fatal("hull changed on failed assemble")
	}
}
