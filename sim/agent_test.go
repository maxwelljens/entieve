package sim

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
)

// ---------------------------------------------------------------------------
// Ledger-agent test fixtures and drivers (system 19).
//
// Roster agents only spawn in agentWorlds (world-default); these tests
// construct agents directly so every fixture stays explicit and every
// existing fixture test keeps running agentless.
// ---------------------------------------------------------------------------

// addTestAgent appends one agent to the roster (kept sorted by id) and
// agentOf looks it up again — fetch pointers only after all inserts.
func addTestAgent(s *Sim, id string, role AgentRole, home StationID, capital float64) {
	s.agents = append(s.agents, Agent{
		ID: id, Home: home, Role: role, Capital: capital, Start: capital,
		Inventory: map[content.ItemID]int{},
		Beliefs:   map[content.ItemID]float64{},
		Margin:    map[content.ItemID]float64{},
	})
	// canonical: ascending id
	for i := len(s.agents) - 1; i > 0 && s.agents[i].ID < s.agents[i-1].ID; i-- {
		s.agents[i], s.agents[i-1] = s.agents[i-1], s.agents[i]
	}
}

func agentOf(s *Sim, id string) *Agent {
	i := s.agentIndex(id)
	if i < 0 {
		panic("test: agent " + id + " not found")
	}
	return &s.agents[i]
}

// stepToTick advances until the tick counter reaches n.
func stepToTick(s *Sim, n int64) {
	for s.ticks < n {
		s.Step()
	}
}

// stepPasses advances n legacy pass windows (agentPassTicks each) — pure
// wall-clock stepping. Paced agents act on their own cooldowns, not on
// these boundaries: tests that need an agent to ACT use stepAgentAct.
func stepPasses(s *Sim, n int) {
	for i := 0; i < n; i++ {
		for {
			s.Step()
			if s.ticks%agentPassTicks == 1 && s.ticks > 1 {
				break
			}
		}
	}
}

// stepAgentAct advances until id's next paced behavior act fires. Act
// ticks are a pure hash of (tick, agent id) — pass boundaries no longer
// mark acts — so this is the only honest "run the agent's pass" wait.
func stepAgentAct(s *Sim, id string) {
	stepAgentActs(s, id, 1)
}

// stepAgentActs advances until id has performed n paced behavior acts
// (systems run on the pre-increment tick, so the act fired on ticks-1).
func stepAgentActs(s *Sim, id string, n int) {
	idx := s.agentIndex(id)
	if idx < 0 {
		panic("test: agent " + id + " not found")
	}
	a := &s.agents[idx]
	for done := 0; done < n; {
		s.Step()
		if s.ticks > 1 && trafficDue(s.ticks-1, agentKey(a), agentCooldownTicks(a)) {
			done++
		}
	}
}

// stepUntil advances up to max ticks or until cond holds, whichever comes
// first (the runUntil-style wait for paced, jittered timing).
func stepUntil(t *testing.T, s *Sim, max int, cond func() bool, what string) {
	t.Helper()
	for i := 0; i < max; i++ {
		s.Step()
		if cond() {
			return
		}
	}
	t.Fatalf("condition not met within %d ticks: %s", max, what)
}

// lastTrade returns a book's last executed price for an item.
func lastTrade(s *Sim, station StationID, item content.ItemID) (float64, bool) {
	book := s.market[station]
	if book == nil {
		return 0, false
	}
	p, ok := book.lastPrice(item)
	return p, ok
}

// restOrder finds a resting order by side/item/owner on a book.
func restOrder(s *Sim, station StationID, side OrderSide, item content.ItemID, owner string) (MarketStock, bool) {
	for _, o := range s.BookOrders(station) {
		if o.Side == side && o.Item == item && o.Owner == owner {
			return o, true
		}
	}
	return MarketStock{}, false
}

// ---------------------------------------------------------------------------
// 1. Imputation: mineral V tracks product V; the fixpoint is stable
// ---------------------------------------------------------------------------

// TestAgentImputationFixpoint pins the theory layer: inputs are
// backward-imputed from their products' fee-net value (a mineral's V
// rises when the product's belief rises), the result satisfies its own
// fixpoint equation (V(m) == max over recipes of the residual claim), and
// a second valuation pass is bit-identical (stable).
func TestAgentImputationFixpoint(t *testing.T) {
	s := newIndustrySim(t)
	addTestAgent(s, "ag-imp", RoleIndustrialist, "cel-station-a", 1_000_000)
	a := agentOf(s, "ag-imp")

	cands := []content.ItemID{
		"ammo-emp-s", "min-ferrite", "min-nickel", "mod-autocannon-s",
		"ore-sterngrit", "ship-sloop",
	} // already sorted
	a.Beliefs["ammo-emp-s"] = 100
	a.Beliefs["mod-autocannon-s"] = 1000
	a.Beliefs["ship-sloop"] = 350_000

	V := s.agentValues(a, cands)
	if V["min-ferrite"] <= 0 {
		t.Fatalf("V(min-ferrite) = %v, want > 0", V["min-ferrite"])
	}

	// The fixpoint equation holds at the fixed point: for every recipe
	// input, V equals its best residual claim (computed against the final
	// V of its co-inputs), clamped at zero.
	for _, m := range []content.ItemID{"min-ferrite", "min-nickel"} {
		best := 0.0
		for _, rec := range s.Content.RecipesInOrder() {
			qty, has := 0.0, false
			for _, in := range rec.Inputs {
				if in.Item == m && in.Qty > 0 {
					qty, has = in.Qty, true
				}
			}
			if !has {
				continue
			}
			mv := s.agentBelief(a, rec.Product) * (1 - imputeHaircut) * float64(rec.Qty)
			other := 0.0
			for _, in := range rec.Inputs {
				if in.Item != m {
					other += in.Qty * s.valueOf(a, V, in.Item)
				}
			}
			best = max(best, (mv-other)/qty)
		}
		want := max(best, 0)
		if math.Abs(V[m]-want) > 1e-6 {
			t.Errorf("fixpoint residual for %s: V=%v want %v (not at a fixed point)", m, V[m], want)
		}
	}

	// Ore floors at its refine yield (V(ore) ≥ Σ yield × V(mineral)).
	floor := 0.5*V["min-ferrite"] + 0.25*V["min-nickel"]
	if V["ore-sterngrit"]+1e-9 < floor {
		t.Errorf("V(ore) = %v, want ≥ refine floor %v", V["ore-sterngrit"], floor)
	}

	// Tracking: raise the product's belief, revalue, mineral V rises.
	before := V["min-ferrite"]
	a.Beliefs["ammo-emp-s"] = 200
	V2 := s.agentValues(a, cands)
	if V2["min-ferrite"] <= before {
		t.Fatalf("mineral V did not track product V: %v -> %v", before, V2["min-ferrite"])
	}

	// Stability: the same state revalues bit-identically.
	V3 := s.agentValues(a, cands)
	if !reflect.DeepEqual(V2, V3) {
		t.Fatalf("fixpoint unstable across passes:\n%v\n%v", V2, V3)
	}
}

// ---------------------------------------------------------------------------
// 2. ZIP convergence (the ACE property): two agents, one item, N rounds
// ---------------------------------------------------------------------------

// zipContent is a two-agent laboratory: a blank book (the agents ARE the
// market) and one recipe whose product belief anchors both reservations:
// the seller (surplus ferrite, reservation 100) and the buyer
// (industrialist restocking, reservation 118.125) must converge toward
// the competitive band between them.
func zipContent() *content.Content {
	c := testContent()
	c.Recipes = map[content.RecipeID]*content.RecipeDef{
		"rec-zip-ferrite": {
			ID: "rec-zip-ferrite", Product: "ammo-emp-s", Qty: 10000,
			Inputs: []content.RecipeInput{{Item: "min-ferrite", Qty: 10000}},
			Time:   100000, Family: content.FamilyCharges, // never finishes in-test
		},
	}
	for i, cel := range c.Systems["sys-a"].Celestials {
		if cel.ID == "cel-station-a" && cel.Station != nil {
			c.Systems["sys-a"].Celestials[i].Station.Market = nil
			c.Systems["sys-a"].Celestials[i].Station.Services =
				append(cel.Station.Services, "industry")
		}
	}
	return c
}

func TestAgentZipConvergence(t *testing.T) {
	s, err := New(zipContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	addTestAgent(s, "ag-zip-sell", RoleTrader, "cel-station-a", 2_000_000)
	addTestAgent(s, "ag-zip-buy", RoleIndustrialist, "cel-station-a", 10_000_000)
	seller := agentOf(s, "ag-zip-sell")
	buyer := agentOf(s, "ag-zip-buy")

	// Reservations: seller is 99% in surplus (need −1 → asks at 0.75·V),
	// buyer fully deficit (need +1 → bids at 1.25·V). Beliefs make them
	// 100 and 118.125 exactly: V = (1−fee)·belief·Qty/inputQty = 0.945·b.
	seller.Inventory["min-ferrite"] = 10_000
	seller.Beliefs["ammo-emp-s"] = 100 / (0.75 * (1 - imputeHaircut))
	buyer.Beliefs["ammo-emp-s"] = 100

	// Paced rounds: each round advances the clock through one act of each
	// side (their cooldowns are 1–10 s, so a fixed pass window no longer
	// guarantees an act) plus the settling ticks between them.
	const rounds = 160
	var prices []float64
	trades := 0
	prevStock := buyer.Inventory["min-ferrite"]
	for i := 0; i < rounds; i++ {
		stepAgentAct(s, "ag-zip-sell")
		stepAgentAct(s, "ag-zip-buy")
		stepN(s, 60)
		// Count executions by the buyer's stock growth (it never sells
		// ferrite), and record each newly printed trade price.
		if got := buyer.Inventory["min-ferrite"]; got > prevStock {
			trades++
			prevStock = got
		}
		if p, ok := lastTrade(s, "cel-station-a", "min-ferrite"); ok {
			if len(prices) == 0 || prices[len(prices)-1] != p {
				prices = append(prices, p)
			}
		}
	}
	if trades < 10 {
		t.Fatalf("only %d executions in %d rounds — quotes never converged to a crossing", trades, rounds)
	}
	if len(prices) == 0 {
		t.Fatal("no trade ever printed")
	}

	// No-arbitrage band: every printed price sits between the two
	// reservations (± the drift the need rule allows as stocks move).
	for i, p := range prices {
		if p < 95 || p > 125 {
			t.Errorf("trade price %d = %v outside [95, 125]", i, p)
		}
	}
	// Convergence: the tail of the printed-price sequence is tight around
	// its own mean — the quotes have settled inside a band around the
	// equilibrium (~108, the harmonic balance of 100 and 118.125).
	tail := prices[max(0, len(prices)-10):]
	mean := 0.0
	for _, p := range tail {
		mean += p
	}
	mean /= float64(len(tail))
	lo, hi := tail[0], tail[0]
	for _, p := range tail {
		lo, hi = min(lo, p), max(hi, p)
	}
	if (hi-lo)/mean > 0.03 {
		t.Errorf("tail prices not converged: [%v, %v] mean %v (range > 3%%)", lo, hi, mean)
	}
	if mean < 100 || mean > 115 {
		t.Errorf("tail mean %v not near the equilibrium band of [100, 118]", mean)
	}
	// The posted book itself is tight at the end: best ask and best bid
	// (both present) straddle the equilibrium within 5%.
	if ask, hasAsk := s.market["cel-station-a"].bestAsk("min-ferrite"); hasAsk {
		if bid, hasBid := s.market["cel-station-a"].bestBid("min-ferrite"); hasBid {
			mid := (ask.Price + bid.Price) / 2
			if ask.Price-bid.Price > 0.05*mid {
				t.Errorf("posted spread %v..%v too wide around %v", bid.Price, ask.Price, mid)
			}
		}
	}

	// Budget constraints never broke: no negative capital or stock.
	for _, a := range s.agents {
		if a.Capital < 0 {
			t.Errorf("%s capital went negative: %v", a.ID, a.Capital)
		}
		for item, qty := range a.Inventory {
			if qty < 0 {
				t.Errorf("%s inventory %s = %d", a.ID, item, qty)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// 3. Miners: belt depletion, selling into the book, the dry-belt bid
// ---------------------------------------------------------------------------

func TestAgentMinerDepletesBeltAndSells(t *testing.T) {
	s := newTestSim(t)
	addTestAgent(s, "ag-mine", RoleMiner, "cel-station-a", 100_000)
	a := agentOf(s, "ag-mine")

	baseline := s.BeltOreVolume("sys-a")
	if baseline <= 0 {
		t.Fatalf("fixture belt volume = %v, want > 0", baseline)
	}

	stepAgentAct(s, "ag-mine")
	after := s.BeltOreVolume("sys-a")
	if after >= baseline {
		t.Fatalf("belt volume %v after a mining pass, want < %v", after, baseline)
	}
	if got := a.Inventory["ore-sterngrit"]; got <= 0 {
		t.Fatalf("miner inventory ore = %d, want > 0 (the drawn batch)", got)
	}
	ask, ok := restOrder(s, "cel-station-a", OrderSell, "ore-sterngrit", agentOwnerTag(a.ID))
	if !ok {
		t.Fatal("no resting ore ask from the miner")
	}
	if ask.Price <= 0 || ask.Qty <= 0 {
		t.Fatalf("bad ask: %+v", ask)
	}

	// Sell: the player buys 10 units off the miner's ask (incoming order,
	// settles on the next tick through agentSettle).
	stockBefore := a.Inventory["ore-sterngrit"]
	capitalBefore := a.Capital
	res, ok := s.PlaceOrder("cel-station-a", OrderBuy, "ore-sterngrit", 10, ask.Price, OwnerPlayer)
	if !ok || res.Filled != 10 {
		t.Fatalf("player buy: res=%+v ok=%v", res, ok)
	}
	stepToTick(s, s.ticks+1)
	if got := a.Inventory["ore-sterngrit"]; got != stockBefore-10 {
		t.Fatalf("miner stock %d after sale, want %d", got, stockBefore-10)
	}
	if a.Capital <= capitalBefore {
		t.Fatalf("miner capital %v did not rise above %v", a.Capital, capitalBefore)
	}
	// Price effect: the ladder's top rung is now the executed trade, not
	// the def base price.
	if p := s.MarketPrice("cel-station-a", "ore-sterngrit"); p != ask.Price {
		t.Fatalf("MarketPrice = %v, want last trade %v (base was %v)",
			p, ask.Price, s.marketBasePrice("ore-sterngrit"))
	}
	// The belt keeps depleting across acts (stock effect), even with
	// cycle boundary in play.
	stepAgentActs(s, "ag-mine", 3)
	if v := s.BeltOreVolume("sys-a"); v >= after {
		t.Fatalf("belt volume %v after three more passes, want < %v", v, after)
	}
}

// TestAgentMinerDryBeltBids: with its system's belt dry, the miner stops
// drawing and bids for ore at its need-adjusted reservation (the reported
// dry-belt rule) — here nothing is left to buy, so the bid rests.
func TestAgentMinerDryBeltBids(t *testing.T) {
	s := newTestSim(t)
	emptyBelt(t, s) // drained + cycle frozen
	addTestAgent(s, "ag-dry", RoleMiner, "cel-station-a", 100_000)
	a := agentOf(s, "ag-dry")

	stepAgentAct(s, "ag-dry")
	if got := a.Inventory["ore-sterngrit"]; got != 0 {
		t.Fatalf("dry belt: miner drew %d units, want 0", got)
	}
	bid, ok := restOrder(s, "cel-station-a", OrderBuy, "ore-sterngrit", agentOwnerTag(a.ID))
	if !ok {
		t.Fatal("dry belt: no resting ore bid — the miner should switch to bidding")
	}
	// Full need premium, opening margin: 1.25·V(ore)·(1−0.1), V floored
	// at the def base price.
	want := quantizeTick(1.25 * s.marketBasePrice("ore-sterngrit") * (1 - zipMarginInit))
	if bid.Price != want {
		t.Fatalf("dry-belt bid %v, want the need-adjusted reservation %v", bid.Price, want)
	}
}

// ---------------------------------------------------------------------------
// 4. Industrialist loop: buys materials → job → sells product
// ---------------------------------------------------------------------------

func TestAgentIndustrialistLoop(t *testing.T) {
	s := newIndustrySim(t)
	home := StationID("cel-station-a")
	// The player is the customer: a standing bid the industrialist's
	// product ask crosses on arrival.
	if _, ok := s.PlaceOrder(home, OrderBuy, "mod-autocannon-s", 1, 2000, OwnerPlayer); !ok {
		t.Fatal("fixture: player bid refused")
	}
	addTestAgent(s, "ag-ind", RoleIndustrialist, home, 1_000_000)
	a := agentOf(s, "ag-ind")
	walletBefore := s.Wallet()

	// The first quote act buys the two-run ferrite stock off the founding
	// ask (paced acts land on this agent's own cooldowns, so wait for the
	// outcome, not for a pass boundary).
	stepUntil(t, s, 30000, func() bool { return a.Inventory["min-ferrite"] > 0 },
		"industrialist never bought its ferrite stock")
	if p, ok := restOrder(s, home, OrderBuy, "min-ferrite", agentOwnerTag(a.ID)); ok && p.Qty > 0 {
		t.Fatalf("founding ask should have filled immediately, leftover bid %+v", p)
	}
	if s.playerStock("min-ferrite") != 0 {
		t.Fatal("industrialist took ferrite out of the player's storage")
	}

	// The commission act starts the job (jobs per agent capped at 1).
	stepUntil(t, s, 30000, func() bool { return len(s.Jobs(home)) > 0 },
		"industrialist never commissioned a job")
	jobs := s.Jobs(home)
	if len(jobs) != 1 || jobs[0].Owner != agentOwnerTag(a.ID) {
		t.Fatalf("jobs = %+v, want exactly the agent's", jobs)
	}
	job1 := jobs[0].ID
	if s.playerStock("min-ferrite") != 0 {
		t.Fatal("job materials came from the player, not the agent")
	}

	// Wait out the job (2 runs × 30 s): the agent collects it itself and
	// sells into the standing bid; the cap on one job at a time lets it
	// immediately commission the next run.
	stepToTick(s, 7000)
	stepUntil(t, s, 12000, func() bool {
		for _, j := range s.Jobs(home) {
			if j.ID == job1 {
				return false
			}
		}
		return true
	}, "job never collected")
	stepUntil(t, s, 12000, func() bool {
		return stacksCount(s.Hangar.Items, "mod-autocannon-s") >= 1
	}, "industrialist never sold into the standing bid")
	stepToTick(s, s.ticks+1)

	if got := a.Inventory["mod-autocannon-s"]; got != 1 {
		t.Fatalf("industrialist turret stock = %d, want 1 (2 made, 1 sold)", got)
	}
	if got := stacksCount(s.Hangar.Items, "mod-autocannon-s"); got != 1 {
		t.Fatalf("player hangar turrets = %d, want 1 (bought from the agent)", got)
	}
	if a.Capital <= 0 {
		t.Fatalf("industrialist capital = %v, want positive", a.Capital)
	}
	// The player's side of the loop: the bid's escrow (price + broker fee)
	// was paid at placement and never refunded, so the wallet cannot end
	// above where it stood after posting it; the hangar check above is the
	// delivery. Materials never left agent ledgers.
	if s.Wallet() > walletBefore {
		t.Fatalf("player wallet %v rose above %v — nothing pays a buyer back here",
			s.Wallet(), walletBefore)
	}
	if s.playerStock("min-ferrite") != 0 || s.playerStock("min-nickel") != 0 {
		t.Fatalf("player stock (ferrite=%d nickel=%d) — the agent drew on the player",
			s.playerStock("min-ferrite"), s.playerStock("min-nickel"))
	}
}

// ---------------------------------------------------------------------------
// 5. Station upkeep: demand for the basket + the fee money sink
// ---------------------------------------------------------------------------

func upkeepContent() *content.Content {
	c := testContent()
	for i, cel := range c.Systems["sys-a"].Celestials {
		if cel.ID == "cel-station-a" && cel.Station != nil {
			c.Systems["sys-a"].Celestials[i].Station.Market = nil // empty book
		}
	}
	return c
}

// moneySupply sums every ledger the sim calls money: the player wallet, all
// agent capital, the station upkeep ledgers, the faction ledgers and the war
// treasury.
func moneySupply(s *Sim) float64 {
	total := s.wallet
	for i := range s.agents {
		total += s.agents[i].Capital
	}
	for _, v := range s.upkeep {
		total += v
	}
	for _, v := range s.factionLedger {
		total += v
	}
	total += s.WarTreasury()
	return total
}

func TestAgentUpkeepDemandAndFeeSink(t *testing.T) {
	station := StationID("cel-station-a")

	t.Run("fee sink with no supply", func(t *testing.T) {
		s, err := New(upkeepContent(), testWorld(), testSeed)
		if err != nil {
			t.Fatal(err)
		}
		// A dormant agent keeps the system-18 gate open without trading.
		addTestAgent(s, "ag-dormant", RoleTrader, station, dormantCapital/2)
		s.upkeep = map[StationID]float64{station: upkeepEndowment}

		before := moneySupply(s)
		stepToTick(s, agentUpkeepTicks+1)
		got := moneySupply(s)
		want := before - upkeepFee
		if math.Abs(got-want) > 1e-9 {
			t.Fatalf("money supply %v, want %v (exactly the maintenance fee burned)", got, want)
		}
		if led := s.upkeep[station]; math.Abs(led-(upkeepEndowment-upkeepFee)) > 1e-9 {
			t.Fatalf("upkeep ledger = %v, want %v", led, upkeepEndowment-upkeepFee)
		}
		// Nothing was on sale: every basket line rested and was cancelled.
		if orders := s.BookOrders(station); len(orders) != 0 {
			t.Fatalf("upkeep left %d resting orders on an empty book", len(orders))
		}
	})

	t.Run("basket bought and paid for", func(t *testing.T) {
		s, err := New(upkeepContent(), testWorld(), testSeed)
		if err != nil {
			t.Fatal(err)
		}
		addTestAgent(s, "ag-dormant", RoleTrader, station, dormantCapital/2)
		s.upkeep = map[StationID]float64{station: upkeepEndowment}
		// The player supplies half the rearm basket: 50 ammo-ap-s at 8.
		giveCargo(s, "ammo-ap-s", 50)
		if _, ok := s.PlaceOrder(station, OrderSell, "ammo-ap-s", 50, 8, OwnerPlayer); !ok {
			t.Fatal("fixture: player ask refused")
		}
		before := moneySupply(s)

		stepToTick(s, agentUpkeepTicks+1)

		// The basket was bought: the goods left the player's escrow and
		// the proceeds landed in the wallet (net of the sales tax).
		if got := stacksCount(s.Hangar.Items, "ammo-ap-s"); got != 0 {
			t.Fatalf("player hangar holds %d ammo-ap-s, want 0 (sold to upkeep)", got)
		}
		if led := s.upkeep[station]; led > upkeepEndowment-upkeepFee {
			t.Fatalf("upkeep ledger %v never paid for the basket", led)
		}
		// Money supply shrank by the fee AND the sales tax on the basket
		// value — both are burns; the purchase itself is a transfer.
		want := upkeepFee + MarketSalesTax*(8*50)
		if got, gap := moneySupply(s), before-moneySupply(s); math.Abs(gap-want) > 1e-6 {
			t.Fatalf("money supply shrank by %v, want %v (fee + tax); supply %v -> %v",
				gap, want, before, got)
		}
	})
}

// ---------------------------------------------------------------------------
// 6. Haul wishes: spread > cost + fees, exposed and fulfilled via the stub
// ---------------------------------------------------------------------------

func spreadContent() *content.Content {
	c := testContent()
	sysA := c.Systems["sys-a"]
	sysA.Celestials = append(sysA.Celestials, content.CelestialDef{
		ID: "cel-station-b", Name: "Anvil Stop", Kind: content.CelStation,
		Position: core.Vec2{X: 1.2e7, Y: 3e6}, Radius: 500,
		Station: &content.StationDef{Services: []string{"market"},
			Market: []content.MarketOrderDef{{Item: "ammo-emp-s", Qty: 100, Price: 10}}},
	})
	return c
}

func TestAgentHaulWishSeam(t *testing.T) {
	s, err := New(spreadContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	home := StationID("cel-station-a")
	addTestAgent(s, "ag-haul", RoleTrader, home, 100_000)
	a := agentOf(s, "ag-haul")

	// Wait for the trader's contract act (paced: acts land on its own
	// cooldown, and only its wish slot rebuilds the list).
	stepUntil(t, s, 12000, func() bool { return len(s.HaulWishes()) > 0 },
		"trader never authored its wish")
	wishes := s.HaulWishes()
	if len(wishes) != 1 {
		t.Fatalf("wishes = %+v, want exactly the ammo-emp-s spread", wishes)
	}
	w := wishes[0]
	// ammo-emp-s: 5 at station A (last/base), 10 at station B → spread 5
	// minus the 5.5% round-trip fee load on the cheap leg.
	wantMax := (10.0 - 5.0) - (MarketBrokerFee+MarketSalesTax)*5.0
	if w.Owner != agentOwnerTag(a.ID) || w.From != home || w.To != "cel-station-b" ||
		w.Item != "ammo-emp-s" || w.Qty < 1 || math.Abs(w.MaxCost-wantMax) > 1e-9 {
		t.Fatalf("wish = %+v, want owner %s %s→%s ammo-emp-s MaxCost %v",
			w, agentOwnerTag(a.ID), home, "cel-station-b", wantMax)
	}

	// The stub pays the agent's free capital and drops the wish.
	before := a.Capital
	if !s.FulfillHaul(w, 3) {
		t.Fatal("FulfillHaul refused a known wish")
	}
	if a.Capital != before-3 {
		t.Fatalf("capital = %v, want %v (stub pays the cost)", a.Capital, before-3)
	}
	if got := s.HaulWishes(); len(got) != 0 {
		t.Fatalf("wishes after fulfillment = %+v, want none", got)
	}
	if s.FulfillHaul(w, 3) {
		t.Fatal("FulfillHaul accepted a wish twice")
	}
	if s.FulfillHaul(HaulWish{Owner: "agent:ghost"}, 1) {
		t.Fatal("FulfillHaul accepted an unknown owner")
	}
	if s.FulfillHaul(HaulWish{Owner: agentOwnerTag(a.ID)}, -1) {
		t.Fatal("FulfillHaul accepted a negative cost")
	}
}

// ---------------------------------------------------------------------------
// 7. The agent-materials seam on StartJob/CollectJob
// ---------------------------------------------------------------------------

func TestAgentStartJobDebitsAgentInventory(t *testing.T) {
	s := newIndustrySim(t)
	home := StationID("cel-station-a")
	addTestAgent(s, "ag-job", RoleIndustrialist, home, 500_000)
	a := agentOf(s, "ag-job")
	a.Inventory["min-ferrite"] = 100
	a.Inventory["min-nickel"] = 100

	// Player storage starts empty of the bill and must stay that way.
	if s.playerStock("min-ferrite") != 0 || s.playerStock("min-nickel") != 0 {
		t.Fatal("fixture: player already holds materials")
	}

	id, ok := s.StartJob(home, "rec-ammo-emp-s", 2, agentOwnerTag(a.ID))
	if !ok {
		t.Fatal("agent StartJob refused with materials in hand")
	}
	// JobMaterials(2): ceil(5.5·2)=11 ferrite, ceil(3·2)=6 nickel.
	if a.Inventory["min-ferrite"] != 89 || a.Inventory["min-nickel"] != 94 {
		t.Fatalf("agent bill debited to ferrite=%d nickel=%d, want 89/94",
			a.Inventory["min-ferrite"], a.Inventory["min-nickel"])
	}
	if s.playerStock("min-ferrite") != 0 || s.playerStock("min-nickel") != 0 ||
		len(s.Hangar.Items) != 0 {
		t.Fatal("agent job touched the player's storage")
	}
	jobs := s.Jobs(home)
	if len(jobs) != 1 || jobs[0].ID != id || jobs[0].Owner != agentOwnerTag(a.ID) {
		t.Fatalf("jobs = %+v", jobs)
	}

	// Refusal: a materials-starved start changes nothing.
	jobsBefore := len(s.Jobs(home))
	if _, ok := s.StartJob(home, "rec-ammo-emp-s", 100, agentOwnerTag(a.ID)); ok {
		t.Fatal("materials-starved start accepted")
	}
	if len(s.Jobs(home)) != jobsBefore || a.Inventory["min-ferrite"] != 89 {
		t.Fatalf("failed start changed state: jobs=%d ferrite=%d",
			len(s.Jobs(home)), a.Inventory["min-ferrite"])
	}

	// Collect: the agent's behavior pass delivers the output into its own
	// inventory (agentCollect → CollectJob), never the hangar — and then,
	// one job at a time, commissions the next batch. Wait for THIS job.
	for i := 0; i < 10000; i++ {
		found := false
		for _, j := range s.Jobs(home) {
			if j.ID == id {
				found = true
			}
		}
		if !found {
			break
		}
		s.Step()
	}
	for _, j := range s.Jobs(home) {
		if j.ID == id {
			t.Fatal("job never collected")
		}
	}
	if a.Inventory["ammo-emp-s"] != 200 {
		t.Fatalf("agent product = %d, want 200", a.Inventory["ammo-emp-s"])
	}
	if len(s.Hangar.Items) != 0 || len(s.Hangar.Ships) != 0 {
		t.Fatalf("agent output leaked into the player hangar: %+v", s.Hangar)
	}
}

// ---------------------------------------------------------------------------
// 8. Save / restore + hash
// ---------------------------------------------------------------------------

func TestAgentSaveRestoreAndHash(t *testing.T) {
	s := newIndustrySim(t)
	home := StationID("cel-station-a")
	if _, ok := s.PlaceOrder(home, OrderBuy, "mod-autocannon-s", 2, 2000, OwnerPlayer); !ok {
		t.Fatal("fixture: player bid refused")
	}
	addTestAgent(s, "ag-save-a", RoleIndustrialist, home, 1_000_000)
	addTestAgent(s, "ag-save-b", RoleTrader, home, 250_000)
	agentOf(s, "ag-save-b").Inventory["ammo-emp-s"] = 42
	s.upkeep = map[StationID]float64{home: upkeepEndowment}
	stepPasses(s, 2) // quotes placed, some fills settled

	wantHash := s.Hash()
	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(save.Agents) != 2 || save.Agents[0].ID != "ag-save-a" {
		t.Fatalf("Save.Agents = %+v", save.Agents)
	}
	if save.Upkeep[home] != upkeepEndowment {
		t.Fatalf("Save.Upkeep = %+v, want the seeded ledger", save.Upkeep)
	}

	b, err := New(industryContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Restore(save); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if got := b.Hash(); got != wantHash {
		t.Fatalf("restored hash %016x, want %016x", got, wantHash)
	}
	// Resting-order ledgers rebuild from the books.
	for i := range s.agents {
		if len(b.agents[i].Orders) != len(s.agents[i].Orders) {
			t.Fatalf("agent %s: restored %d tracked orders, want %d",
				s.agents[i].ID, len(b.agents[i].Orders), len(s.agents[i].Orders))
		}
	}
	if b.agents[1].Inventory["ammo-emp-s"] != 42 {
		t.Fatalf("restored inventory = %d, want 42", b.agents[1].Inventory["ammo-emp-s"])
	}

	// Continuation: both runs keep hashing identically.
	stepToTick(s, s.ticks+400)
	stepToTick(b, b.ticks+400)
	if s.Hash() != b.Hash() {
		t.Fatal("twin runs diverged after restore + continuation")
	}

	// Corrupt saves fail loudly (trust boundary).
	bad, _ := s.Snapshot()
	bad.Agents[0].Capital = -1
	if err := b.Restore(bad); err == nil {
		t.Fatal("negative capital restored without error")
	}
	bad2, _ := s.Snapshot()
	bad2.Agents = append(bad2.Agents, bad2.Agents[0])
	if err := b.Restore(bad2); err == nil {
		t.Fatal("duplicate agent id restored without error")
	}
	bad3, _ := s.Snapshot()
	bad3.Agents[0].Home = "cel-nowhere"
	if err := b.Restore(bad3); err == nil {
		t.Fatal("unknown home station restored without error")
	}
}

// ---------------------------------------------------------------------------
// 9. Twin-run determinism with a long agent soak
// ---------------------------------------------------------------------------

func TestAgentTwinRunSoakDeterminism(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("load content: %v", err)
	}
	wd, ok := c.World("world-default")
	if !ok {
		t.Fatal("world-default missing")
	}
	checkpoints := map[int64]bool{1: true, 600: true, 1800: true, 3601: true, 4500: true}
	run := func() (map[int64]uint64, []AgentView) {
		s, err := New(c, wd, 99)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if len(s.Agents()) != 24 {
			t.Fatalf("roster = %d agents, want 24", len(s.Agents()))
		}
		hashes := map[int64]uint64{}
		if checkpoints[0] {
			hashes[0] = s.Hash()
		}
		for s.ticks < 4500 {
			s.Step()
			if checkpoints[s.ticks] {
				hashes[s.ticks] = s.Hash()
			}
		}
		return hashes, s.Agents()
	}
	ha, va := run()
	hb, vb := run()
	for _, cp := range []int64{1, 600, 1800, 3601, 4500} {
		if ha[cp] != hb[cp] {
			t.Fatalf("checkpoint %d: hashes differ (a=%016x b=%016x)", cp, ha[cp], hb[cp])
		}
	}
	if !reflect.DeepEqual(va, vb) {
		t.Fatalf("agent ledgers differ between twins:\n%+v\n%+v", va, vb)
	}
	// The soak was not inert: at least one agent has priced something.
	s, err := New(c, wd, 99)
	if err != nil {
		t.Fatal(err)
	}
	stepN(s, int(agentCooldownMax)) // every roster agent acts at least once
	acted := false
	for i := range s.agents {
		if len(s.agents[i].Beliefs) > 0 || len(s.agents[i].Margin) > 0 {
			acted = true
		}
	}
	if !acted {
		t.Fatal("no agent valued or quoted anything in a full cooldown window")
	}
}

// ---------------------------------------------------------------------------
// 10. Roster split + world gate (the tour stays agentless)
// ---------------------------------------------------------------------------

func TestAgentRosterSplitAndWorldGate(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("load content: %v", err)
	}
	wd, ok := c.World("world-default")
	if !ok {
		t.Fatal("world-default missing")
	}
	s, err := New(c, wd, 7)
	if err != nil {
		t.Fatal(err)
	}
	views := s.Agents()
	if len(views) != 24 {
		t.Fatalf("roster = %d agents, want 24 (floor(24/6) per market station)", len(views))
	}
	stations := marketStations(c)
	if len(stations) != 6 {
		t.Fatalf("fixture: %d market stations, want 6", len(stations))
	}
	per := map[StationID]int{}
	roles := map[AgentRole]int{}
	for i, v := range views {
		if i > 0 && views[i-1].ID >= v.ID {
			t.Fatalf("roster not in ascending id order: %q then %q", views[i-1].ID, v.ID)
		}
		if !strings.HasPrefix(v.ID, "ag-") {
			t.Fatalf("odd agent id %q", v.ID)
		}
		per[v.Home]++
		roles[v.Role]++
		if v.Capital != agentStartCapital {
			t.Fatalf("%s capital = %v, want %v", v.ID, v.Capital, agentStartCapital)
		}
	}
	if len(per) != 6 {
		t.Fatalf("homes span %d stations, want all 6", len(per))
	}
	for st, n := range per {
		if n != 4 {
			t.Errorf("station %s hosts %d agents, want 4", st, n)
		}
	}
	// The documented split: industry stations lead with two
	// industrialists (4×2 = 8), the rest alternate trader/miner (8/8).
	if roles[RoleIndustrialist] != 8 || roles[RoleTrader] != 8 || roles[RoleMiner] != 8 {
		t.Fatalf("roles = %+v, want {industrialist:8 trader:8 miner:8}", roles)
	}
	// Upkeep ledgers seeded for every market station.
	if len(s.upkeep) != 6 {
		t.Fatalf("upkeep ledgers = %d, want 6", len(s.upkeep))
	}
	for _, st := range stations {
		if s.upkeep[st] != upkeepEndowment {
			t.Errorf("upkeep[%s] = %v, want the endowment", st, s.upkeep[st])
		}
	}

	// World gate: the showcase tour (and any non-listed world) runs
	// without agents — and therefore without upkeep — so its economy
	// cannot drift.
	ws, ok := c.World("world-showcase")
	if !ok {
		t.Fatal("world-showcase missing")
	}
	tour, err := New(c, ws, 7)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(tour.Agents()); n != 0 {
		t.Fatalf("tour roster = %d agents, want 0 (world-gated)", n)
	}
	if tour.upkeep != nil {
		t.Fatal("tour has upkeep ledgers (agent gate bypassed)")
	}
}
