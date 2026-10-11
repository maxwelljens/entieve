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

func marketQty(s *Sim, item content.ItemID) (qty int, price float64, ok bool) {
	for _, st := range s.Market("sys-a") {
		if st.Item == item {
			return st.Qty, st.Price, true
		}
	}
	return 0, 0, false
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

	// Qty 3: charge 3x, stock -3, three hulls delivered.
	s.Queue(CmdBuy{Item: "ship-barge", Qty: 3})
	s.Step()
	evs := drain(s)
	if s.Wallet() != walletBefore-3*price {
		t.Fatalf("wallet=%v want %v (3 x %v)", s.Wallet(), walletBefore-3*price, price)
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
	// Hulls report their raw ID in market text (itemName only resolves items).
	if !hasEvent(evs, EvMarket, "Bought 3 x ship-barge for 150,000.00 credits") {
		t.Fatalf("missing buy event: %v", evs)
	}

	// Qty above the shelf clamps to what is in stock: charge and deliver
	// exactly the listed units, never a phantom hull. (Top the shelf back up
	// so the clamp has something to work against.)
	for i := range s.market["sys-a"] {
		if s.market["sys-a"][i].Item == "ship-barge" {
			s.market["sys-a"][i].Qty = 2
		}
	}
	s.wallet = 300000
	s.Queue(CmdBuy{Item: "ship-barge", Qty: 5})
	s.Step()
	evs = drain(s)
	if s.Wallet() != 300000-2*price {
		t.Fatalf("wallet=%v want %v (clamped to the 2 units on the shelf)",
			s.Wallet(), 300000-2*price)
	}
	if n := hangarHulls(s, "ship-barge"); n != 5 {
		t.Fatalf("hangar holds %d hulls, want 5", n)
	}
	stockAfter, _, _ = marketQty(s, "ship-barge")
	if stockAfter != 0 {
		t.Fatalf("stock=%d want 0", stockAfter)
	}

	// Empty shelf: refusal, nothing moves.
	walletRefused := s.Wallet()
	hullsRefused := hangarHulls(s, "ship-barge")
	s.Queue(CmdBuy{Item: "ship-barge", Qty: 1})
	s.Step()
	evs = drain(s)
	if findEvent(evs, EvLog, "Out of stock: ship-barge") == nil {
		t.Fatalf("missing out-of-stock refusal: %v", evs)
	}
	if s.Wallet() != walletRefused || hangarHulls(s, "ship-barge") != hullsRefused {
		t.Fatalf("refusal moved state: wallet=%v hulls=%d", s.Wallet(),
			hangarHulls(s, "ship-barge"))
	}
}

func TestBuyReducesStockAndWallet(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	qtyBefore, price, _ := marketQty(s, "charge-emp")
	walletBefore := s.Wallet()

	s.Queue(CmdBuy{Item: "charge-emp", Qty: 10})
	s.Step()
	evs := drain(s)

	if s.Wallet() != walletBefore-price*10 {
		t.Fatalf("wallet=%v want %v", s.Wallet(), walletBefore-price*10)
	}
	qtyAfter, _, _ := marketQty(s, "charge-emp")
	if qtyAfter != qtyBefore-10 {
		t.Fatalf("stock=%d want %d", qtyAfter, qtyBefore-10)
	}
	if hangarCount(s, "charge-emp") != 10 {
		t.Fatalf("hangar=%d want 10", hangarCount(s, "charge-emp"))
	}
	if !hasEvent(evs, EvMarket, "Bought 10 x EMP Charge for 50.00 credits") {
		t.Fatalf("missing buy event: %v", evs)
	}
}

func TestBuyRequiresDockedMarketService(t *testing.T) {
	s := newTestSim(t)
	// Not docked.
	s.Queue(CmdBuy{Item: "charge-emp", Qty: 1})
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
	// One barge (50k) is affordable and lands in the hangar.
	s.Queue(CmdBuy{Item: "ship-barge", Qty: 1})
	s.Step()
	drain(s)
	if s.Wallet() != 50000 || len(s.Hangar.Ships) != 1 {
		t.Fatalf("wallet=%v ships=%d", s.Wallet(), len(s.Hangar.Ships))
	}
	if s.Hangar.Ships[0].Hull != "ship-barge" {
		t.Fatalf("hangar ship=%s", s.Hangar.Ships[0].Hull)
	}
}

func TestSellCreditsNinetyPercent(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	walletBefore := s.Wallet()
	stockBefore, price, _ := marketQty(s, "charge-emp")
	ammoBefore := cargoCount(s, "charge-emp")

	s.Queue(CmdSell{Item: "charge-emp", Qty: 5})
	s.Step()
	evs := drain(s)

	want := 0.9 * price * 5
	if math.Abs(s.Wallet()-(walletBefore+want)) > 1e-9 {
		t.Fatalf("wallet=%v want %v", s.Wallet(), walletBefore+want)
	}
	if cargoCount(s, "charge-emp") != ammoBefore-5 {
		t.Fatal("cargo not reduced")
	}
	stockAfter, _, _ := marketQty(s, "charge-emp")
	if stockAfter != stockBefore+5 {
		t.Fatalf("stock=%d want %d", stockAfter, stockBefore+5)
	}
	if !hasEvent(evs, EvMarket, "Sold 5 x EMP Charge for 22.50 credits") {
		t.Fatalf("missing sell event: %v", evs)
	}
}

func TestSellUnlistedItemUsesBasePrice(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	giveCargo(s, "min-nickel", 3) // unlisted: BasePrice 12
	walletBefore := s.Wallet()
	s.Queue(CmdSell{Item: "min-nickel", Qty: 3})
	s.Step()
	drain(s)
	want := 0.9 * 12 * 3
	if math.Abs(s.Wallet()-(walletBefore+want)) > 1e-9 {
		t.Fatalf("wallet=%v want %v", s.Wallet(), walletBefore+want)
	}
}

func TestSellFallsBackToHangar(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	s.Hangar.Items = addStacks(s.Hangar.Items, "min-ferrite", 7)
	walletBefore := s.Wallet()
	s.Queue(CmdSell{Item: "min-ferrite", Qty: 7})
	s.Step()
	evs := drain(s)
	// Listed at 10 -> 0.9*10*7 = 63.
	if math.Abs(s.Wallet()-(walletBefore+63)) > 1e-9 {
		t.Fatalf("wallet=%v want %v", s.Wallet(), walletBefore+63)
	}
	if hangarCount(s, "min-ferrite") != 0 {
		t.Fatal("hangar not drained")
	}
	if !hasEvent(evs, EvMarket, "Sold 7 x Ferrite for 63.00 credits") {
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

func TestRefineFloorPerMineral(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	giveCargo(s, "ore-sterngrit", 10)
	s.Queue(CmdRefine{Item: "ore-sterngrit", Qty: 10})
	s.Step()
	evs := drain(s)

	if cargoCount(s, "ore-sterngrit") != 0 {
		t.Fatal("ore not consumed")
	}
	// floor(0.5*10) = 5 ferrite, floor(0.25*10) = 2 nickel.
	if cargoCount(s, "min-ferrite") != 5 {
		t.Fatalf("ferrite=%d want 5", cargoCount(s, "min-ferrite"))
	}
	if cargoCount(s, "min-nickel") != 2 {
		t.Fatalf("nickel=%d want 2", cargoCount(s, "min-nickel"))
	}
	if !hasEvent(evs, EvLog, "Refined 10 x Sterngrit into 5 x Ferrite, 2 x Nickel") {
		t.Fatalf("missing refine event: %v", evs)
	}

	// Floor test: 3 units -> ferrite floor(1.5)=1, nickel floor(0.75)=0.
	giveCargo(s, "ore-sterngrit", 3)
	s.Queue(CmdRefine{Item: "ore-sterngrit", Qty: 3})
	s.Step()
	drain(s)
	if cargoCount(s, "min-ferrite") != 6 || cargoCount(s, "min-nickel") != 2 {
		t.Fatalf("after 2nd refine: ferrite=%d nickel=%d", cargoCount(s, "min-ferrite"),
			cargoCount(s, "min-nickel"))
	}
}

func TestRefineRequiresOreAndService(t *testing.T) {
	s := newTestSim(t)
	// Not docked.
	s.Queue(CmdRefine{Item: "ore-sterngrit", Qty: 1})
	s.Step()
	evs := drain(s)
	if findEvent(evs, EvLog, "must be docked") == nil {
		t.Fatalf("expected not-docked warning: %v", evs)
	}
	// Docked but no ore.
	dockAtHaven(t, s)
	s.Queue(CmdRefine{Item: "ore-sterngrit", Qty: 5})
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
	if !hasEvent(evs, EvLog, "Repaired 250.0 HP for 500.00 credits") {
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
	ammoBefore := cargoCount(s, "charge-emp")

	// The hangar holds a barge with its own fit and cargo.
	s.Hangar.Ships = append(s.Hangar.Ships, StoredShip{
		Hull:  "ship-barge",
		Fit:   []content.ModuleID{"mod-shield-extender"},
		Cargo: []content.CargoStack{{Item: "charge-ap", Qty: 7}},
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
	if cargoCount(s, "charge-ap") != 7 {
		t.Fatalf("barge cargo=%d", cargoCount(s, "charge-ap"))
	}
	if cargoCount(s, "charge-emp") != 0 {
		t.Fatal("sloop cargo should have moved to the hangar")
	}
	if len(s.Hangar.Ships) != 1 || s.Hangar.Ships[0].Hull != "ship-sloop" {
		t.Fatalf("hangar=%+v", s.Hangar.Ships)
	}
	// The stored sloop kept its fit and cargo.
	stored := s.Hangar.Ships[0]
	if len(stored.Fit) != 1 || stored.Fit[0] != "mod-emp-turret" {
		t.Fatalf("stored fit=%v", stored.Fit)
	}
	if stacksCount(stored.Cargo, "charge-emp") != ammoBefore {
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
	if cargoCount(s, "charge-emp") != ammoBefore {
		t.Fatal("sloop cargo not restored")
	}
	fit, _ = s.Fittings.Get(s.player)
	if len(fit.High) != 1 || fit.High[0] != "mod-emp-turret" {
		t.Fatalf("sloop fit=%v", fit.High)
	}
	if len(s.Hangar.Ships) != 1 || s.Hangar.Ships[0].Hull != "ship-barge" {
		t.Fatalf("hangar after swap=%+v", s.Hangar.Ships)
	}
	barge := s.Hangar.Ships[0]
	if stacksCount(barge.Cargo, "charge-ap") != 7 || len(barge.Fit) != 1 {
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
