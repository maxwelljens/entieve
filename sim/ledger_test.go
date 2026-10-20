package sim

import (
	"math"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// The transaction ledger: every wallet movement is recorded exactly once
// (one entry per transaction, its net amount, with a trade's broker fee
// and sales tax itemized inside the row), the ring caps at LedgerCap with
// the oldest evicted, and the ring round-trips through the Save.
//
// The sites audited here are every s.wallet debit/credit in the package:
// market IOC buy/sell (system_economy.go execBuy/execSell), a resting
// player order filled by an incoming one (settle), order placement both
// sides (PlaceOrder — the same engine seam CmdPlaceOrder rides), escrow
// release on cancel (CancelOrder — the CmdCancelOrder seam), station
// repairs (execRepair), bounties (system_damage.go) and contract payouts
// (system_mission.go).
// ---------------------------------------------------------------------------

// runRecorded runs f and checks the ledger grew by want entries whose
// summed amounts equal the wallet's own movement — the exactly-once
// invariant, stated as arithmetic rather than as a list of calls.
func runRecorded(t *testing.T, s *Sim, want int, f func()) []LedgerEntry {
	t.Helper()
	before, n := s.Wallet(), len(s.ledger)
	f()
	got := append([]LedgerEntry(nil), s.ledger[n:]...)
	if len(got) != want {
		t.Fatalf("recorded %d entries, want %d: %+v", len(got), want, got)
	}
	var sum float64
	for _, e := range got {
		sum += e.Amount
	}
	if delta := s.Wallet() - before; math.Abs(sum-delta) > 1e-6 {
		t.Fatalf("ledger sum %v != wallet delta %v (%+v)", sum, delta, got)
	}
	return got
}

// hasLedger reports whether any entry matches a What substring.
func hasLedger(entries []LedgerEntry, what string) bool {
	for _, e := range entries {
		if strings.Contains(e.What, what) {
			return true
		}
	}
	return false
}

func TestLedgerMarketBuyAndSell(t *testing.T) {
	// IOC buy: cost + broker fee out, one row (ammo-emp-s asks at 5).
	s := newMissionSim(t)
	dockNamed(t, s, "Haven Rest")
	got := runRecorded(t, s, 1, func() {
		s.Queue(CmdBuy{Item: "ammo-emp-s", Qty: 10})
		s.Step()
		drain(s)
	})
	e := got[0]
	if !strings.Contains(e.What, "Market buy — 10 x EMP Charge") {
		t.Errorf("buy row = %q, want the market buy", e.What)
	}
	if e.Where != "Haven Rest" {
		t.Errorf("buy where = %q, want the docked station", e.Where)
	}
	if want := -(10*5.0 + MarketBrokerFee*10*5); math.Abs(e.Amount-want) > 1e-9 {
		t.Errorf("buy amount = %v, want %v (10 x 5 + broker fee)", e.Amount, want)
	}
	if want := MarketBrokerFee * 10 * 5; math.Abs(e.Fee-want) > 1e-9 {
		t.Errorf("buy fee = %v, want %v itemized", e.Fee, want)
	}

	// IOC sell: proceeds minus broker fee and sales tax, both itemized
	// (the world-start cargo holds 20 ammo-emp-s, the book bids 4).
	s = newMissionSim(t)
	dockNamed(t, s, "Haven Rest")
	got = runRecorded(t, s, 1, func() {
		s.Queue(CmdSell{Item: "ammo-emp-s", Qty: 10})
		s.Step()
		drain(s)
	})
	e = got[0]
	if !strings.Contains(e.What, "Market sell — 10 x EMP Charge") {
		t.Errorf("sell row = %q, want the market sell", e.What)
	}
	if want := 10*4.0 - MarketSalesTax*10*4 - MarketBrokerFee*10*4; math.Abs(e.Amount-want) > 1e-9 {
		t.Errorf("sell amount = %v, want %v", e.Amount, want)
	}
	if want := MarketSalesTax * 10 * 4; math.Abs(e.Tax-want) > 1e-9 {
		t.Errorf("sell tax = %v, want %v itemized", e.Tax, want)
	}
	if want := MarketBrokerFee * 10 * 4; math.Abs(e.Fee-want) > 1e-9 {
		t.Errorf("sell fee = %v, want %v itemized", e.Fee, want)
	}
}

func TestLedgerRestingOrderFillAndPlacement(t *testing.T) {
	// A player sell order rests above the bid (6 vs 4): the placement row
	// carries the broker fee alone, and the fill that comes later — an
	// incoming system buy crossing it — is its own row (proceeds minus
	// the sales tax; the fee was already paid at placement).
	s := newMissionSim(t)
	// Clear the founding ask first (a system buy, no player movement), so
	// the player's ask is the only supply the crossing buy can reach.
	if _, ok := s.PlaceOrder("cel-station-a", OrderBuy, "ammo-emp-s", 100, 5, OwnerSystem); !ok {
		t.Fatal("could not clear the founding ask")
	}
	res, ok := s.PlaceOrder("cel-station-a", OrderSell, "ammo-emp-s", 5, 6, OwnerPlayer)
	if !ok {
		t.Fatal("player ask refused")
	}
	if res.Filled != 0 || res.Remaining != 5 {
		t.Fatalf("ask = %+v, want it resting whole", res)
	}
	got := runRecorded(t, s, 1, func() {
		s.PlaceOrder("cel-station-a", OrderBuy, "ammo-emp-s", 5, 6, OwnerSystem)
	})
	e := got[0]
	if !strings.Contains(e.What, "Market sell — 5 x EMP Charge") {
		t.Errorf("fill row = %q, want the resting ask's sale", e.What)
	}
	if want := 5 * 6.0 * MarketSalesTax; math.Abs(e.Tax-want) > 1e-9 {
		t.Errorf("fill tax = %v, want %v", e.Tax, want)
	}
	if e.Fee != 0 {
		t.Errorf("fill fee = %v, want none (charged at placement)", e.Fee)
	}
}

func TestLedgerBuyOrderEscrowAndRelease(t *testing.T) {
	s := newMissionSim(t)
	before := s.Wallet()
	var placed OrderResult
	got := runRecorded(t, s, 1, func() {
		var ok bool
		placed, ok = s.PlaceOrder("cel-station-a", OrderBuy, "min-ferrite", 10, 9, OwnerPlayer)
		if !ok {
			t.Fatal("player bid refused")
		}
	})
	// Escrow (limit x qty) + broker fee left the wallet, nothing traded.
	escrow := 10 * 9.0
	fee := MarketBrokerFee * escrow
	if want := -(escrow + fee); math.Abs(got[0].Amount-want) > 1e-9 {
		t.Errorf("placement row = %v, want %v", got[0].Amount, want)
	}
	if math.Abs(s.Wallet()-(before-(escrow+fee))) > 1e-9 {
		t.Errorf("wallet = %v, want %v", s.Wallet(), before-(escrow+fee))
	}
	// The escrow is the capital the hold shows as deployed.
	if d := s.Deployed(); math.Abs(d-escrow) > 1e-9 {
		t.Errorf("Deployed = %v, want the resting escrow %v", d, escrow)
	}
	// Cancel: the escrow comes back whole (the fee was kept).
	got = runRecorded(t, s, 1, func() {
		if !s.CancelOrder("cel-station-a", placed.ID, OwnerPlayer) {
			t.Fatal("cancel refused")
		}
	})
	if !strings.Contains(got[0].What, "Escrow released — 10 x Ferrite") {
		t.Errorf("cancel row = %q, want the escrow release", got[0].What)
	}
	if math.Abs(got[0].Amount-escrow) > 1e-9 {
		t.Errorf("cancel amount = %v, want %v", got[0].Amount, escrow)
	}
	if d := s.Deployed(); d != 0 {
		t.Errorf("Deployed = %v after the cancel, want 0", d)
	}
	if math.Abs(s.Wallet()-(before-fee)) > 1e-9 {
		t.Errorf("wallet = %v, want the kept fee only: %v", s.Wallet(), before-fee)
	}
}

// TestLedgerOrderCommandsRideTheSameSeats drives CmdPlaceOrder /
// CmdCancelOrder — the standing-order commands of the market tranche —
// and checks the ledger rows come out of the SAME engine sites
// (PlaceOrder / CancelOrder): one escrow row at placement, one release
// row at cancel, and the invariant holding across the pair.
func TestLedgerOrderCommandsRideTheSameSeats(t *testing.T) {
	s := newMissionSim(t)
	dockNamed(t, s, "Haven Rest")
	var id uint64
	got := runRecorded(t, s, 1, func() {
		s.Queue(CmdPlaceOrder{Side: OrderBuy, Item: "min-ferrite", Qty: 10, Price: 9})
		s.Step()
		drain(s)
	})
	if !strings.Contains(got[0].What, "Buy order — 10 x Ferrite") || got[0].Fee <= 0 {
		t.Errorf("placement row = %+v, want the escrow row with the fee itemized", got[0])
	}
	for _, o := range s.BookOrders("cel-station-a") {
		if o.Owner == OwnerPlayer {
			id = o.ID
		}
	}
	if id == 0 {
		t.Fatal("the command rested no player order")
	}
	if d := s.Deployed(); math.Abs(d-90) > 1e-9 {
		t.Errorf("Deployed = %v, want 90", d)
	}
	got = runRecorded(t, s, 1, func() {
		s.Queue(CmdCancelOrder{ID: id})
		s.Step()
		drain(s)
	})
	if !strings.Contains(got[0].What, "Escrow released — 10 x Ferrite") {
		t.Errorf("cancel row = %+v, want the escrow release", got[0])
	}
	if d := s.Deployed(); d != 0 {
		t.Errorf("Deployed = %v after the command cancel, want 0", d)
	}
}

func TestLedgerBuyOrderThatCrossesPaysOnlyWhatTraded(t *testing.T) {
	// A buy limit at the ask (10) crosses whole: the row's net is the
	// traded value + fee, and nothing is left deployed.
	s := newMissionSim(t)
	var got []LedgerEntry
	var placed OrderResult
	got = runRecorded(t, s, 1, func() {
		var ok bool
		placed, ok = s.PlaceOrder("cel-station-a", OrderBuy, "min-ferrite", 10, 10, OwnerPlayer)
		if !ok {
			t.Fatal("crossing bid refused")
		}
	})
	if placed.Filled != 10 || placed.Remaining != 0 {
		t.Fatalf("crossing bid = %+v, want a full fill", placed)
	}
	if want := -(10*10.0 + MarketBrokerFee*100); math.Abs(got[0].Amount-want) > 1e-9 {
		t.Errorf("row = %v, want %v (value + fee)", got[0].Amount, want)
	}
	if d := s.Deployed(); d != 0 {
		t.Errorf("Deployed = %v, want 0 (nothing rests)", d)
	}
}

func TestLedgerSellOrderPlacementIsOneRowEitherWay(t *testing.T) {
	// Resting sell (limit 6 above the 4 bid): the row is the broker fee
	// alone — the goods, not the wallet, are what rests.
	s := newMissionSim(t)
	var got []LedgerEntry
	got = runRecorded(t, s, 1, func() {
		res, ok := s.PlaceOrder("cel-station-a", OrderSell, "ammo-emp-s", 5, 6, OwnerPlayer)
		if !ok || res.Filled != 0 || res.Remaining != 5 {
			t.Fatalf("resting sell = %+v ok=%v, want it resting whole", res, ok)
		}
	})
	if want := -(MarketBrokerFee * 5 * 6); math.Abs(got[0].Amount-want) > 1e-9 {
		t.Errorf("resting sell row = %v, want the broker fee alone %v", got[0].Amount, want)
	}
	if got[0].Tax != 0 {
		t.Errorf("resting sell tax = %v, want none (nothing traded)", got[0].Tax)
	}

	// Crossed sell (limit at the 4 bid): one row, proceeds minus fee and
	// tax, both itemized — never one row for the fee and another for the
	// sale.
	s = newMissionSim(t)
	got = runRecorded(t, s, 1, func() {
		res, ok := s.PlaceOrder("cel-station-a", OrderSell, "ammo-emp-s", 5, 4, OwnerPlayer)
		if !ok || res.Filled != 5 || res.Remaining != 0 {
			t.Fatalf("crossing sell = %+v ok=%v, want a full fill", res, ok)
		}
	})
	e := got[0]
	if want := 5*4.0 - MarketBrokerFee*5*4 - MarketSalesTax*5*4; math.Abs(e.Amount-want) > 1e-9 {
		t.Errorf("sell placement row = %v, want %v", e.Amount, want)
	}
	if e.Fee <= 0 || e.Tax <= 0 {
		t.Errorf("fee/tax = %v/%v, want both itemized in the one row", e.Fee, e.Tax)
	}
}

func TestLedgerRepairsAndBountiesAndContracts(t *testing.T) {
	// Repairs: the station bill is its own row.
	s := newMissionSim(t)
	dockNamed(t, s, "Haven Rest")
	ship, _ := s.Ships.Get(s.player)
	ship.Shield = ship.MaxShield - 200
	got := runRecorded(t, s, 1, func() {
		s.Queue(CmdRepair{})
		s.Step()
		drain(s)
	})
	if !strings.Contains(got[0].What, "Repairs — 200.0 HP") || math.Abs(got[0].Amount+400) > 1e-9 {
		t.Errorf("repair row = %+v, want -400 for 200 HP", got[0])
	}
	if got[0].Where != "Haven Rest" {
		t.Errorf("repair where = %q, want the station", got[0].Where)
	}

	// Bounties: only the player's kills pay, and they are their own row in
	// the system they happened in.
	s = newMissionSim(t)
	got = runRecorded(t, s, 1, func() { killOne(t, s, "npc-rat-scuttler") })
	if !strings.Contains(got[0].What, "Bounty — Rat Scuttler") || got[0].Amount != 15000 {
		t.Errorf("bounty row = %+v, want 15,000.00 €", got[0])
	}
	if got[0].Where != "Fixture Prime" {
		t.Errorf("bounty where = %q, want the current system", got[0].Where)
	}

	// Contract payout: the reward lands as one row, at the claim station.
	s = newMissionSim(t)
	dockNamed(t, s, "Haven Rest")
	acceptMission(t, s, "contract-test-kill")
	undock(t, s)
	for range 3 {
		killOne(t, s, "npc-rat-scuttler")
	}
	got = runRecorded(t, s, 1, func() { dockNamed(t, s, "Haven Rest") })
	if !strings.Contains(got[0].What, "Contract — Test Cleanup") || got[0].Amount != 30000 {
		t.Errorf("payout row = %+v, want 30,000.00 €", got[0])
	}
	if got[0].Where != "Haven Rest" {
		t.Errorf("payout where = %q, want the claim station", got[0].Where)
	}
}

func TestLedgerRecordsNothingWithoutAMovement(t *testing.T) {
	s := newMissionSim(t)
	dockNamed(t, s, "Haven Rest")
	ship, _ := s.Ships.Get(s.player)
	ship.Shield = ship.MaxShield // already at full strength
	s.wallet = 10                // and broke
	runRecorded(t, s, 0, func() {
		s.Queue(CmdRepair{})                         // nothing to repair
		s.Queue(CmdBuy{Item: "min-ferrite", Qty: 1}) // refused: broke
		s.Step()
		drain(s)
	})
	if s.Wallet() != 10 {
		t.Fatalf("wallet = %v, want the refusals to leave it alone", s.Wallet())
	}
	// Undocked selling moves nothing either.
	s = newMissionSim(t)
	runRecorded(t, s, 0, func() {
		s.Queue(CmdSell{Item: "ammo-emp-s", Qty: 1})
		s.Step()
		drain(s)
	})
}

func TestLedgerHoldingComposition(t *testing.T) {
	s := newMissionSim(t)
	dockNamed(t, s, "Haven Rest")
	acceptMission(t, s, "contract-test-kill")
	s.PlaceOrder("cel-station-a", OrderBuy, "min-ferrite", 10, 9, OwnerPlayer)

	if got, want := s.Receivables(), 30000.0; got != want {
		t.Errorf("Receivables = %v, want the accepted contract's %v", got, want)
	}
	if got, want := s.Deployed(), 90.0; got != want {
		t.Errorf("Deployed = %v, want the resting escrow %v", got, want)
	}
	if s.Wallet()+s.Deployed()+s.Receivables() <= s.Wallet() {
		t.Error("holdings total must exceed the cash alone")
	}
}

func TestLedgerRingCapsAtTwohundredAndEvictsOldest(t *testing.T) {
	s := newMissionSim(t)
	dockNamed(t, s, "Haven Rest")
	before := s.Wallet()
	// 150 placements + 150 escrow releases = 300 movements, each pair
	// costing the broker fee alone (limit 1 rests under the 5 ask).
	for i := 1; i <= 150; i++ {
		res, ok := s.PlaceOrder("cel-station-a", OrderBuy, "ammo-emp-s", i, 1, OwnerPlayer)
		if !ok {
			t.Fatalf("placement %d refused", i)
		}
		if !s.CancelOrder("cel-station-a", res.ID, OwnerPlayer) {
			t.Fatalf("cancel %d refused", i)
		}
	}
	if len(s.ledger) != LedgerCap {
		t.Fatalf("ring = %d entries, want %d", len(s.ledger), LedgerCap)
	}
	// The newest 200 survive: the ring opens on movement 101 (the escrow
	// debit of placement 51) and closes on the release of placement 150.
	if !strings.Contains(s.ledger[0].What, "Buy order — 51 x EMP Charge") {
		t.Errorf("oldest kept = %q, want placement 51", s.ledger[0].What)
	}
	last := s.ledger[len(s.ledger)-1]
	if !strings.Contains(last.What, "Escrow released — 150 x EMP Charge") {
		t.Errorf("newest = %q, want the release of placement 150", last.What)
	}
	if hasLedger(s.Ledger(), "Buy order — 50 x EMP Charge") {
		t.Error("the evicted placement 50 is still in the ring")
	}
	// The ring is a record, not a movement: the wallet lost exactly the
	// broker fees of all 150 placements.
	if want := 0.03 * (150 * 151 / 2); math.Abs(s.Wallet()-(before-want)) > 1e-6 {
		t.Errorf("wallet = %v, want %v (fees only)", s.Wallet(), before-want)
	}
}

func TestLedgerSurvivesSaveRestore(t *testing.T) {
	s := newMissionSim(t)
	dockNamed(t, s, "Haven Rest")
	s.Queue(CmdBuy{Item: "ammo-emp-s", Qty: 10})
	s.Step()
	drain(s)
	if len(s.ledger) == 0 {
		t.Fatal("fixture recorded nothing")
	}
	wantHash := s.Hash()
	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(save.Ledger) != len(s.ledger) {
		t.Fatalf("Save.Ledger = %d entries, want %d", len(save.Ledger), len(s.ledger))
	}

	b := newMissionSim(t)
	if err := b.Restore(save); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if got := b.Hash(); got != wantHash {
		t.Fatalf("restored hash %016x, want %016x (the ledger must round-trip)", got, wantHash)
	}
	if len(b.ledger) != len(s.ledger) {
		t.Fatalf("restored ledger = %d entries, want %d", len(b.ledger), len(s.ledger))
	}
	for i := range s.ledger {
		if b.ledger[i] != s.ledger[i] {
			t.Fatalf("entry %d = %+v, want %+v", i, b.ledger[i], s.ledger[i])
		}
	}
	// A hand-edited save cannot grow the ring past its cap.
	save.Ledger = append(save.Ledger, make([]LedgerEntry, LedgerCap)...)
	if err := b.Restore(save); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if len(b.ledger) != LedgerCap {
		t.Fatalf("restored ring = %d entries, want the cap %d", len(b.ledger), LedgerCap)
	}
}
