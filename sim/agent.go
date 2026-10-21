package sim

// ---------------------------------------------------------------------------
// Ledger agents — the economic agent layer (system 18): autonomous traders,
// miners and manufacturers that make the economy player-driven without
// players. Approved design: docs/research/price-formation-brief.md §5
// candidate A ("Imputed-Marginal-Utility") layered over ZIP placement and
// trade-price memory.
//
// Rules (few rules, emergent outcomes — the Game-of-Life criterion):
//
//   - Valuation (Menger/Wieser imputation). Consumption-anchored goods are
//     valued at the agent's EWMA belief of EXECUTED trade prices (the
//     brief's layer-3 market memory; beliefs bootstrap from the price
//     ladder's base-price rung). Inputs are backward-imputed through the
//     recipe tree to a fixpoint: V(m) = max over recipes r ∋ m of
//     ((MV(out_r) − Σ other inputs V·qty) / qty), with MV = belief net of
//     the 5.5% round-trip fee load (broker + tax) and a refine-yield floor
//     for ore. Reservation prices spread ±needPremium around V by inventory
//     need — deficit raises the willingness to pay, surplus discounts the
//     ask — so no order ever crosses the agent's own valuation
//     (Gode-Sunder's budget constraint).
//
//   - Placement (ZIP). One margin µ per item; quotes rest at V·(1±µ)
//     through PlaceOrder, so 4-sig-fig ticks, the 3% broker fee (both
//     sides, at placement) and the 2.5% sales tax are real friction. Each
//     pass µ moves by Widrow-Hoff toward the target implied by observed
//     prices (last trade, else the quote midpoint) with momentum over the
//     agent's own last shout: µ += β(µ* − µ) + α(µ − µ_implied), clamped
//     to [zipMarginMin, zipMarginMax].
//
//   - Cadence. Resting fills settle every tick (agentSettle); the full
//     behavior pass fires on each agent's OWN stateless cooldown
//     (agentCooldownTicks: 1–10 s jittered from a pure hash — CONTEXT.md
//     "agent cooldown"), and at most one capital-moving step runs per act
//     (the job / quotes / wish slot rotates on the same hashes), so an
//     agent never moves capital twice inside one cooldown. Station upkeep
//     every agentUpkeepTicks. Canonical order everywhere — agents
//     ascending id, items/recipes/stations sorted, maps point-looked-up
//     only — and ZERO RNG draws (imputation, ZIP and upkeep are pure math).
//
// Bootstrap leanings (roles) are starting behaviors, not classes: miners
// draw belt ore and sell it, industrialists buy materials / run jobs / sell
// products, traders market-make and stockpile. Capital growth is the
// selector: order sizes scale with Capital/Start (clamped), and an agent
// under dormantCapital takes no actions at all (v1 never removes a roster
// member from the ledger).
//
// Agent inventory is a single universe-wide pool (documented v1
// simplification — physical station location is the logistics workstream's
// concern, see HaulWish).
// ---------------------------------------------------------------------------

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/entieve/entieve/content"
)

// --- tunable constants (the exact numbers the report cites) ---

const (
	// agentRosterSize is the roster target spread over every market
	// station: floor(agentRosterSize / #stations) agents per station
	// (≥1), so the shipped 6-station universe gets 4×6 = 24 agents.
	agentRosterSize = 24
	// agentStartCapital is the bootstrap capital (also the order-size
	// selector's denominator).
	agentStartCapital = 250_000.0

	// Cadence, in ticks: resting fills settle every tick; the behavior pass
	// fires per agent on its own cooldown (agentCooldownMin..Max — a pure
	// hash of tick + agent id, no persisted timer); station upkeep every
	// agentUpkeepTicks.
	agentPassTicks   = int64(300) // legacy pass cadence: the per-test stepping helper
	agentUpkeepTicks = int64(3600)

	// agentCooldownMin/Max is the agent pacing window (CONTEXT.md "agent
	// cooldown"): one jittered cooldown in ticks per agent — 1..10 sim
	// seconds — bounding how often an agent's behavior act fires and,
	// with the one-capital-step-per-act rule in agentAct, how often it
	// can move capital at all.
	agentCooldownMin = int64(60)
	agentCooldownMax = int64(600)

	// beliefGamma is the EWMA weight on the last executed trade price
	// (trade-price memory; own quotes never move a belief).
	beliefGamma = 0.1

	// ZIP constants: Widrow-Hoff learning rate β and momentum α (inside
	// the brief's cited ranges β ∈ [0.1, 0.5], α ∈ [0, 0.1]), opening
	// margin, and the never-cross-reservation clamps.
	zipBeta       = 0.25
	zipAlpha      = 0.05
	zipMarginInit = 0.10
	zipMarginMin  = 0.01
	zipMarginMax  = 1.0

	// needPremium is how far reservations spread around V by inventory
	// need: a full deficit bids up to +25% over V, a full surplus asks
	// down to −25% under V. Nothing else moves a reservation.
	needPremium = 0.25
	// imputeHaircut is the round-trip fee load (broker 3% + sales tax
	// 2.5%) the expected sale value of a product is taken net of when
	// inputs are imputed from it.
	imputeHaircut = MarketBrokerFee + MarketSalesTax
	// imputeMaxIter/imputeTol cap and gate the imputation fixpoint (16
	// iterations: enough for the alternating claim path to reach its
	// clamped fixed point on shipped content — cost then equals the
	// product's fee-net value on the binding recipe, margin zero).
	imputeMaxIter = 16
	imputeTol     = 1e-9

	// orderValueShare caps one order's placed value at a quarter of the
	// agent's free (unescrowed) capital; qtyScaleMin/Max bound the
	// capital-growth selector Capital/Start.
	orderValueShare = 0.25
	qtyScaleMin     = 0.1
	qtyScaleMax     = 5.0

	// Per-pass order sizes and stock targets by leaning.
	traderBaseQty      = 20  // trader quote size before the capital scale
	traderStockTarget  = 100 // trader stockpile target per basket item
	traderBasketN      = 6   // basket: first N sorted content item/module ids
	materialBaseQty    = 100 // industrialist material restock size per pass
	materialRunsTarget = 2   // industrialist: materials for two runs of stock
	jobRunsCap         = 4   // jobs per agent capped at 1; runs capped here

	// Miner: batch drawn per pass, ore stock cap, and the dry-belt ore
	// restock target that turns a miner into a bidder.
	minerBatchUnits = 40
	oreStockCap     = 300
	dryBeltTarget   = 50
	minerBidQty     = 10

	// Upkeep: one-time (never refilled) station endowment, the fee burned
	// every cycle, the basket price slack, and the basket quantities
	// (rearm = every ammo item, repair = the lowest-id armor rep module).
	upkeepEndowment = 100_000_000.0
	upkeepFee       = 5_000.0
	upkeepSlack     = 0.25
	upkeepChargeQty = 50
	upkeepRepairQty = 1

	// dormantCapital: below this the agent takes no actions at all.
	dormantCapital = 1.0

	// agentMarginEps: the industrialist produces at margin ≥ 0 (within
	// float noise) — imputation makes the binding recipe break-even by
	// construction, so an exact-zero margin must still count as viable.
	agentMarginEps = 1e-6
)

// agentWorlds is the world gate: the roster spawns only in the listed
// free-play worlds (the showcase tour and test fixtures run without
// agents, so their economies cannot drift). Tests that want a roster call
// spawnAgents directly.
var agentWorlds = map[content.WorldID]bool{
	"world-default": true,
}

// AgentRole is one agent's bootstrap leaning — a starting behavior, not a
// class; capital growth is the selector.
type AgentRole string

const (
	RoleMiner         AgentRole = "miner"
	RoleIndustrialist AgentRole = "industrialist"
	RoleTrader        AgentRole = "trader"
)

// ownerAgentPrefix is the opaque market owner tag of agent orders/jobs.
const ownerAgentPrefix = "agent:"

// HaulWish is one cross-node arbitrage request a trader/industrialist
// appends when the spread between two stations' prices exceeds fees. The
// agent layer produces them and keeps the committed one alive until it is
// settled: system_traffic.go consumes wishes (goods sourced at spawn,
// delivered as agent-owned asks, settled through FulfillHaul). A wish is
// one shipment — never partial.
type HaulWish struct {
	Owner    string
	From, To StationID
	Item     content.ItemID
	Qty      int
	MaxCost  float64
}

// AgentOrder is one of an agent's resting orders: the escrow a resting buy
// holds out of Capital and the fill detector agentSettle compares the book
// against. Kept ascending by id (canonical order). Not serialized: a Save
// lands on Step boundaries where every fill has settled, so restore
// rebuilds the ledger from the books themselves.
type AgentOrder struct {
	ID      uint64
	Station StationID
	Side    OrderSide
	Item    content.ItemID
	Price   float64
	Qty     int
}

// Agent is one ledger agent (see the file header for the rule shapes).
// State per the approved design: ID, Home, Capital, Inventory, Beliefs,
// Margin, Wishes — plus Role (the bootstrap leaning), Start (the
// order-size denominator), Orders (the resting-order ledger) and InFlight
// (the wishes a convoy is already carrying).
type Agent struct {
	ID        string
	Home      StationID
	Role      AgentRole
	Capital   float64
	Start     float64
	Inventory map[content.ItemID]int
	Beliefs   map[content.ItemID]float64
	Margin    map[content.ItemID]float64
	Wishes    []HaulWish
	Orders    []AgentOrder
	// InFlight are wishes already riding a convoy hauler: their goods left
	// Inventory at spawn, so the behavior pass can rebuild the wish list
	// from live spreads without losing the commitment (retainInFlight puts
	// the committed copy back). Cleared on delivery (FulfillHaul) and on
	// loss (the goods are gone, the wish is dropped unpaid).
	InFlight []HaulWish
}

// AgentView is the read-only roster view (system 18's reader surface).
type AgentView struct {
	ID      string
	Home    StationID
	Role    AgentRole
	Capital float64
	Dormant bool
}

// ---------------------------------------------------------------------------
// Roster
// ---------------------------------------------------------------------------

// marketStations returns every content station offering the market
// service, sorted by station id — the canonical roster and upkeep order.
func marketStations(c *content.Content) []StationID {
	var ids []StationID
	sysIDs := make([]content.SystemID, 0, len(c.Systems))
	for id := range c.Systems {
		sysIDs = append(sysIDs, id)
	}
	sortSystemIDs(sysIDs)
	for _, id := range sysIDs {
		for _, cel := range c.Systems[id].Celestials {
			if cel.Station != nil && hasMarketService(cel.Station) {
				ids = append(ids, cel.ID)
			}
		}
	}
	sortStationIDs(ids)
	return ids
}

// agentRoleFor assigns agent slot j's bootstrap leaning at a station:
// industry stations lead with two industrialists, then alternate
// trader/miner; market-only stations alternate trader/miner from the top.
// Deterministic, service-aware, documented (the report's roster split).
func agentRoleFor(j int, industry bool) AgentRole {
	if industry && j < 2 {
		return RoleIndustrialist
	}
	if j%2 == 0 {
		return RoleTrader
	}
	return RoleMiner
}

// spawnAgents seeds the deterministic roster (and the station upkeep
// ledgers) for a world whose market stations can host it. No RNG: ids,
// homes, roles and capital are pure functions of content.
func (s *Sim) spawnAgents() {
	stations := marketStations(s.Content)
	if len(stations) == 0 {
		return
	}
	per := max(1, agentRosterSize/len(stations))
	for _, st := range stations {
		industry := s.stationService(st, content.SvcIndustry)
		for j := 0; j < per; j++ {
			s.agents = append(s.agents, Agent{
				ID:        fmt.Sprintf("ag-%03d", len(s.agents)+1),
				Home:      st,
				Role:      agentRoleFor(j, industry),
				Capital:   agentStartCapital,
				Start:     agentStartCapital,
				Inventory: map[content.ItemID]int{},
				Beliefs:   map[content.ItemID]float64{},
				Margin:    map[content.ItemID]float64{},
			})
		}
	}
	if s.upkeep == nil {
		s.upkeep = make(map[StationID]float64, len(stations))
	}
	for _, st := range stations {
		s.upkeep[st] = upkeepEndowment
	}
}

// agentOwnerTag is the opaque market owner tag of an agent's orders and
// jobs ("agent:<id>").
func agentOwnerTag(id string) string { return ownerAgentPrefix + id }

// agentIndex finds the roster slot of an id (-1 when absent).
func (s *Sim) agentIndex(id string) int {
	for i := range s.agents {
		if s.agents[i].ID == id {
			return i
		}
	}
	return -1
}

// agentByOwner resolves an opaque owner tag to its agent (nil for
// OwnerPlayer/OwnerSystem/the upkeep account/unknown tags) — the seam
// StartJob/CollectJob route agent-owned work through.
func (s *Sim) agentByOwner(owner string) *Agent {
	id, ok := strings.CutPrefix(owner, ownerAgentPrefix)
	if !ok {
		return nil
	}
	if i := s.agentIndex(id); i >= 0 {
		return &s.agents[i]
	}
	return nil
}

// ---------------------------------------------------------------------------
// Ledger helpers
// ---------------------------------------------------------------------------

// freeCapital is Capital minus the escrow held by the agent's resting buy
// orders (the money is still in Capital; committed once, never twice).
func (a *Agent) freeCapital() float64 {
	free := a.Capital
	for _, o := range a.Orders {
		if o.Side == OrderBuy {
			free -= o.Price * float64(o.Qty)
		}
	}
	return max(0, free)
}

// freeStock is an agent's inventory minus the units escrowed in its own
// resting sell orders.
func (a *Agent) freeStock(item content.ItemID) int {
	n := a.Inventory[item]
	for _, o := range a.Orders {
		if o.Side == OrderSell && o.Item == item {
			n -= o.Qty
		}
	}
	return max(0, n)
}

// agentNeed is the inventory-target gap in [−1, 1]: >0 a net buyer
// (deficit), <0 a net seller (surplus), 0 exactly on target.
func agentNeed(target, stock int) float64 {
	return clamp((float64(target)-float64(stock))/float64(max(max(target, stock), 1)), -1, 1)
}

// agentQtyScale is the capital-growth selector: profitable agents (Capital
// above the bootstrap) post larger orders, shrinking ones post smaller,
// clamped so a losing agent still quotes and a winner cannot monopolize.
func agentQtyScale(a *Agent) float64 {
	if a.Start <= 0 {
		return 1
	}
	return clamp(a.Capital/a.Start, qtyScaleMin, qtyScaleMax)
}

// agentBelief is the item's EWMA trade-price memory: the stored belief,
// else the price ladder's base-price rung (0 when the content has none —
// the item stays unvalued until a trade prints).
func (s *Sim) agentBelief(a *Agent, item content.ItemID) float64 {
	if b, ok := a.Beliefs[item]; ok && b > 0 {
		return b
	}
	return s.marketBasePrice(item)
}

// valueOf is V[item] with a belief fallback for items outside the pass's
// candidate set.
func (s *Sim) valueOf(a *Agent, V map[content.ItemID]float64, item content.ItemID) float64 {
	if v, ok := V[item]; ok {
		return v
	}
	return s.agentBelief(a, item)
}

// addInv/takeInv mutate an agent's inventory, dropping empty stacks.
func addInv(a *Agent, item content.ItemID, qty int) {
	if qty <= 0 {
		return
	}
	if a.Inventory == nil {
		a.Inventory = map[content.ItemID]int{}
	}
	a.Inventory[item] += qty
}

func takeInv(a *Agent, item content.ItemID, qty int) {
	if qty <= 0 {
		return
	}
	if a.Inventory[item] <= qty {
		delete(a.Inventory, item)
		return
	}
	a.Inventory[item] -= qty
}

// ownOrder returns the agent's lowest-id resting HOME order for a
// side/item. Deliveries listed at other stations (haul-wish cargo that
// arrived at its market) are not home shouts: quoting must neither cross
// nor reprice them — they rest until sold.
func (a *Agent) ownOrder(side OrderSide, item content.ItemID) (AgentOrder, bool) {
	var best AgentOrder
	found := false
	for _, o := range a.Orders {
		if o.Side != side || o.Item != item || o.Station != a.Home {
			continue
		}
		if !found || o.ID < best.ID {
			best, found = o, true
		}
	}
	return best, found
}

// ownQuote returns the agent's lowest-id resting HOME order for an item
// (either side) — the previous shout the ZIP momentum term reads.
func (a *Agent) ownQuote(item content.ItemID) (AgentOrder, bool) {
	var best AgentOrder
	found := false
	for _, o := range a.Orders {
		if o.Item != item || o.Station != a.Home {
			continue
		}
		if !found || o.ID < best.ID {
			best, found = o, true
		}
	}
	return best, found
}

// dropOrder removes a settled/cancelled order id from the ledger.
func (a *Agent) dropOrder(id uint64) {
	kept := a.Orders[:0]
	for _, o := range a.Orders {
		if o.ID != id {
			kept = append(kept, o)
		}
	}
	a.Orders = kept
}

// ---------------------------------------------------------------------------
// Belt seam (over sim/system_mining.go's belt state)
// ---------------------------------------------------------------------------

// BeltOreVolume reports the total remaining belt volume (m³) of a system —
// the reader the miner leaning draws against.
func (s *Sim) BeltOreVolume(sys content.SystemID) float64 {
	have := 0.0
	for _, k := range s.beltKeys() {
		if k.System != sys {
			continue
		}
		for _, r := range s.belts[k].Rocks {
			have += r.Remaining
		}
	}
	return have
}

// ExtractBeltOre removes up to vol m³ from a system's belts (all-or-nothing:
// beltOreAvailable is checked first so drawBeltOre never half-draws) and
// returns the volume actually drawn. Pure math, zero RNG.
func (s *Sim) ExtractBeltOre(sys content.SystemID, vol float64) float64 {
	if vol <= beltDepletedEpsilon || !s.beltOreAvailable(sys, vol) {
		return 0
	}
	if !s.drawBeltOre(sys, vol) {
		return 0
	}
	return vol
}

// ensureSystemBelts seeds persistent belt state for every belt of a system
// from its content defs (the same first-generation semantics loadSystem
// uses), so miners extract from systems the player has never visited and
// depletion/regrowth stay tracked there.
func (s *Sim) ensureSystemBelts(sys content.SystemID) {
	def, ok := s.Content.System(sys)
	if !ok {
		return
	}
	for i := range def.Celestials {
		cel := &def.Celestials[i]
		if cel.Belt == nil {
			continue
		}
		if s.belts[beltKey{System: sys, Cel: cel.ID}] == nil {
			s.ensureBeltStateAt(sys, cel)
		}
	}
}

// firstBeltRock returns the first belt rock of a system with volume left,
// in the exact canonical order drawBeltOre draws from (celestial id, then
// rock index): the ore a miner credits must come from this rock.
func (s *Sim) firstBeltRock(sys content.SystemID) (content.ItemID, float64, bool) {
	for _, k := range s.beltKeys() {
		if k.System != sys {
			continue
		}
		st := s.belts[k]
		for i := range st.Rocks {
			if st.Rocks[i].Remaining > beltDepletedEpsilon {
				return st.Rocks[i].Ore, st.Rocks[i].Remaining, true
			}
		}
	}
	return "", 0, false
}

// systemOreItems returns the distinct ores of a system's belt defs,
// sorted — the miner's tradeables.
func (s *Sim) systemOreItems(sys content.SystemID) []content.ItemID {
	def, ok := s.Content.System(sys)
	if !ok {
		return nil
	}
	seen := map[content.ItemID]bool{}
	var out []content.ItemID
	for _, cel := range def.Celestials {
		if cel.Belt == nil {
			continue
		}
		for _, ad := range cel.Belt.Asteroids {
			if ad.Ore != "" && !seen[ad.Ore] {
				seen[ad.Ore] = true
				out = append(out, ad.Ore)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// ---------------------------------------------------------------------------
// Valuation — beliefs, imputation fixpoint, refine floors
// ---------------------------------------------------------------------------

// agentCandidates is the sorted set of items this agent observes and
// values: home-book activity, its own inventory and orders, plus the
// role's standing watchlist (miner ores + their refine minerals, every
// recipe input/product, the trader basket).
func (s *Sim) agentCandidates(a *Agent) []content.ItemID {
	set := map[content.ItemID]bool{}
	if book := s.market[a.Home]; book != nil {
		for _, o := range book.Orders {
			set[o.Item] = true
		}
		for item := range book.Lasts {
			set[item] = true
		}
	}
	for item := range a.Inventory {
		set[item] = true
	}
	for _, o := range a.Orders {
		set[o.Item] = true
	}
	add := func(item content.ItemID) {
		if item != "" {
			set[item] = true
		}
	}
	switch a.Role {
	case RoleMiner:
		if sys, ok := s.stationSystem(a.Home); ok {
			for _, ore := range s.systemOreItems(sys) {
				add(ore)
				if def, ok := s.Content.Item(ore); ok {
					for yield := range def.RefineYield {
						add(yield) // the ore's floor depends on its minerals
					}
				}
			}
		}
	case RoleIndustrialist:
		for _, rec := range s.Content.RecipesInOrder() {
			add(rec.Product)
			for _, in := range rec.Inputs {
				add(in.Item)
			}
		}
	case RoleTrader:
		for _, item := range s.traderBasket() {
			add(item)
		}
	}
	out := make([]content.ItemID, 0, len(set))
	for item := range set {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// traderBasket is the trader leaning's standing market-making basket: the
// first traderBasketN sorted content item/module ids (content-derived and
// therefore stable across passes — on shipped content that is the combat
// supplies: charges first, then the first loot/mineral ids).
func (s *Sim) traderBasket() []content.ItemID {
	ids := make([]content.ItemID, 0, len(s.Content.Items)+len(s.Content.Modules))
	for id := range s.Content.Items {
		ids = append(ids, id)
	}
	for id := range s.Content.Modules {
		ids = append(ids, content.ItemID(id))
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) > traderBasketN {
		ids = ids[:traderBasketN]
	}
	return ids
}

// agentUpdateBeliefs refreshes the agent's EWMA trade-price memory for its
// candidate set: bootstraps unset beliefs from the base-price rung, then
// pulls every item with an executed trade toward its last trade price.
// Own quotes never move beliefs (only trades do — no self-reinforcement).
func (s *Sim) agentUpdateBeliefs(a *Agent, candidates []content.ItemID) {
	book := s.market[a.Home]
	for _, item := range candidates {
		if _, ok := a.Beliefs[item]; !ok {
			if base := s.marketBasePrice(item); base > 0 {
				a.Beliefs[item] = base
			}
		}
		if book == nil {
			continue
		}
		last, ok := book.lastPrice(item)
		if !ok || last <= 0 {
			continue
		}
		cur, held := a.Beliefs[item]
		if !held || cur <= 0 {
			a.Beliefs[item] = last
			continue
		}
		a.Beliefs[item] = cur + beliefGamma*(last-cur)
	}
}

// agentValues runs one valuation pass over the candidates:
//
//  1. V starts at each item's belief;
//  2. inputs (recipe inputs) backward-impute to a fixpoint in sorted item
//     order, at most imputeMaxIter iterations, each step taking Wieser's
//     max over recipes — inputs track what their products are worth AT THE
//     MARGIN, net of fees;
//  3. ore floors at its refine yield (TSM's Destroy logic).
func (s *Sim) agentValues(a *Agent, candidates []content.ItemID) map[content.ItemID]float64 {
	V := make(map[content.ItemID]float64, len(candidates))
	for _, item := range candidates {
		V[item] = s.agentBelief(a, item)
	}
	recipes := s.Content.RecipesInOrder()
	inputSet := map[content.ItemID]bool{}
	for _, rec := range recipes {
		for _, in := range rec.Inputs {
			if in.Qty > 0 {
				inputSet[in.Item] = true
			}
		}
	}
	var inputs []content.ItemID
	for _, item := range candidates {
		if inputSet[item] {
			inputs = append(inputs, item)
		}
	}
	for iter := 0; iter < imputeMaxIter; iter++ {
		delta := 0.0
		for _, m := range inputs { // candidates are sorted: canonical order
			best := 0.0
			for _, rec := range recipes { // RecipesInOrder: canonical order
				qty, has := 0.0, false
				for _, in := range rec.Inputs {
					if in.Item == m && in.Qty > 0 {
						qty, has = in.Qty, true
						break
					}
				}
				if !has {
					continue
				}
				// MV: the run's expected output value net of fees
				// (the recipe's whole yield, not one unit).
				mv := s.agentBelief(a, rec.Product) * (1 - imputeHaircut) * float64(rec.Qty)
				other := 0.0
				for _, in := range rec.Inputs {
					if in.Item == m {
						continue
					}
					other += in.Qty * s.valueOf(a, V, in.Item)
				}
				best = max(best, (mv-other)/qty)
			}
			nv := max(best, 0)
			delta = max(delta, math.Abs(nv-V[m]))
			V[m] = nv
		}
		if delta <= imputeTol {
			break
		}
	}
	for _, item := range candidates {
		def, ok := s.Content.Item(item)
		if !ok || len(def.RefineYield) == 0 {
			continue
		}
		sum := 0.0
		for yield, frac := range def.RefineYield {
			sum += frac * s.valueOf(a, V, yield)
		}
		V[item] = max(V[item], sum)
	}
	return V
}

// ---------------------------------------------------------------------------
// Behavior plan (role rules) and ZIP placement
// ---------------------------------------------------------------------------

// agentPlan is one pass's role decision: the industrialist's best-margin
// recipe and material targets, the trader's stockpile targets, the miner's
// dry-belt flag and ore targets. Inputs mark which held items are
// production inputs (only their surplus is sellable while producing).
type agentPlan struct {
	recipe  content.RecipeID
	margin  float64
	inputs  map[content.ItemID]bool
	targets map[content.ItemID]int
	dry     bool
}

func (s *Sim) agentPlan(a *Agent, V map[content.ItemID]float64) agentPlan {
	p := agentPlan{inputs: map[content.ItemID]bool{}, targets: map[content.ItemID]int{}}
	switch a.Role {
	case RoleMiner:
		sys, ok := s.stationSystem(a.Home)
		if !ok {
			break
		}
		s.ensureSystemBelts(sys)
		_, _, hasVolume := s.firstBeltRock(sys)
		p.dry = !hasVolume
		for _, ore := range s.systemOreItems(sys) {
			p.inputs[ore] = p.dry // while dry the miner restocks ore, it does not dump it
			p.targets[ore] = 0
			if p.dry {
				p.targets[ore] = dryBeltTarget
			}
		}
	case RoleIndustrialist:
		if !s.stationService(a.Home, content.SvcIndustry) {
			break
		}
		found := false
		for _, rec := range s.Content.RecipesInOrder() { // ties keep the lowest id
			m := s.recipeMargin(a, rec, V)
			if !found || m > p.margin {
				p.recipe, p.margin, found = rec.ID, m, true
			}
		}
		if !found || p.margin < -agentMarginEps {
			break // negative-margin production: buy nothing, sell everything
		}
		for _, m := range s.JobMaterials(a.Home, p.recipe, materialRunsTarget) {
			p.inputs[m.Item] = true
			p.targets[m.Item] = m.Qty
		}
	case RoleTrader:
		for _, item := range s.traderBasket() {
			p.targets[item] = traderStockTarget
		}
	}
	return p
}

// recipeMargin is the expected per-run margin the industrialist selects
// with: expected sale value of the output (belief net of fees) minus the
// imputed value of the committed bill. Positive margin ⇒ produce.
func (s *Sim) recipeMargin(a *Agent, rec *content.RecipeDef, V map[content.ItemID]float64) float64 {
	mv := s.agentBelief(a, rec.Product) * (1 - imputeHaircut) * float64(rec.Qty)
	cost := 0.0
	for _, m := range s.JobMaterials(a.Home, rec.ID, 1) {
		cost += float64(m.Qty) * s.valueOf(a, V, m.Item)
	}
	return mv - cost
}

// agentQuote is one computed posting: reservation price (pre-tick) and a
// quantity already capped by the need rule.
type agentQuote struct {
	side  OrderSide
	item  content.ItemID
	price float64
	qty   int
}

// agentObservedPrice is the ZIP target τ for an item at the agent's home
// book: last executed trade, else the resting-quote midpoint, else the one
// resting side (agents' own quotes included — they hold τ where it is).
func (s *Sim) agentObservedPrice(a *Agent, item content.ItemID) (float64, bool) {
	book := s.market[a.Home]
	if book == nil {
		return 0, false
	}
	if p, ok := book.lastPrice(item); ok && p > 0 {
		return p, true
	}
	bid, hasBid := book.bestBid(item)
	ask, hasAsk := book.bestAsk(item)
	if hasBid && hasAsk {
		return (bid.Price + ask.Price) / 2, true
	}
	if hasBid {
		return bid.Price, true
	}
	if hasAsk {
		return ask.Price, true
	}
	return 0, false
}

// agentUpdateMargin is the ZIP rule: Widrow-Hoff toward the target margin
// implied by observed prices (β), with momentum over the agent's own last
// shout (α), clamped to the never-cross-reservation band. Returns the
// margin to quote with this pass.
func (s *Sim) agentUpdateMargin(a *Agent, item content.ItemID, need, wtp, res float64) float64 {
	µ, ok := a.Margin[item]
	if !ok || µ <= 0 {
		µ = zipMarginInit
	}
	τ, observed := s.agentObservedPrice(a, item)
	if observed && wtp > 0 && res > 0 {
		var µstar float64
		switch {
		case need > 0: // net buyer: bid toward τ
			µstar = clamp(1-τ/wtp, zipMarginMin, zipMarginMax)
		case need < 0: // net seller: ask toward τ
			µstar = clamp(τ/res-1, zipMarginMin, zipMarginMax)
		default: // on target: straddle τ so neither side overshoots
			µstar = max(clamp(τ/res-1, zipMarginMin, zipMarginMax),
				clamp(1-τ/wtp, zipMarginMin, zipMarginMax))
		}
		µ += zipBeta * (µstar - µ)
		if o, ok := a.ownQuote(item); ok { // momentum over the last shout
			var implied float64
			if o.Side == OrderSell {
				implied = o.Price/res - 1
			} else {
				implied = 1 - o.Price/wtp
			}
			µ += zipAlpha * (µ - implied)
		}
		µ = clamp(µ, zipMarginMin, zipMarginMax)
	}
	a.Margin[item] = µ
	return µ
}

// agentQuotes computes this pass's postings for the candidate set. The
// quantity rules fall out of the need sign: only a deficit buys (target
// above stock, capped by the role's per-pass size) and only a surplus
// sells (stock above target for production inputs, all liquid stock
// otherwise), so an agent never quotes both sides of one item fresh —
// reservations never cross themselves.
func (s *Sim) agentQuotes(a *Agent, candidates []content.ItemID,
	V map[content.ItemID]float64, p *agentPlan) []agentQuote {
	scale := agentQtyScale(a)
	var out []agentQuote
	for _, item := range candidates { // sorted: canonical order
		v, ok := V[item]
		if !ok || v <= 0 {
			continue // unvalued (no belief, no trade): never quote it
		}
		stock := a.freeStock(item)
		target := p.targets[item]
		need := agentNeed(target, stock)
		var buyQty, sellQty int
		if target > stock { // only a deficit buys...
			buyQty = min(a.roleBuyQty(scale), target-stock)
		}
		if p.inputs[item] { // production inputs: only their surplus sells
			sellQty = max(0, stock-target)
		} else if need <= 0 { // everything else: on-target or surplus sells
			sellQty = stock
		}
		if buyQty <= 0 && sellQty <= 0 {
			continue
		}
		wtp := v * (1 + needPremium*max(need, 0))
		res := v * (1 + needPremium*min(need, 0))
		µ := s.agentUpdateMargin(a, item, need, wtp, res)
		if sellQty > 0 {
			out = append(out, agentQuote{side: OrderSell, item: item,
				price: res * (1 + µ), qty: sellQty})
		}
		if buyQty > 0 {
			out = append(out, agentQuote{side: OrderBuy, item: item,
				price: wtp * (1 - µ), qty: buyQty})
		}
	}
	return out
}

// roleBuyQty is the per-pass buy size before the target cap, scaled by
// the capital selector.
func (a *Agent) roleBuyQty(scale float64) int {
	switch a.Role {
	case RoleIndustrialist:
		return max(1, int(materialBaseQty*scale))
	case RoleMiner:
		return max(1, int(minerBidQty*scale))
	default:
		return max(1, int(traderBaseQty*scale))
	}
}

// agentApplyQuotes executes the pass's postings: unwanted resting orders
// are cancelled first (releasing escrow), then each quote is placed at
// its tick — keeping a resting quote whose price has not moved (patience
// instead of 0.01-undercut churn) and relisting when it has. Every order
// goes through PlaceOrder, so fees, escrow, ticks and crossing all apply.
// Caps: one order never exceeds orderValueShare of free capital (fee
// included) nor the agent's unescrowed stock.
func (s *Sim) agentApplyQuotes(a *Agent, quotes []agentQuote) {
	tag := agentOwnerTag(a.ID)
	sort.Slice(quotes, func(i, j int) bool { // sells first, then item asc
		if quotes[i].side != quotes[j].side {
			return quotes[i].side == OrderSell
		}
		return quotes[i].item < quotes[j].item
	})
	type quoteKey struct {
		side OrderSide
		item content.ItemID
	}
	desired := make(map[quoteKey]bool, len(quotes))
	for _, q := range quotes {
		desired[quoteKey{q.side, q.item}] = true
	}
	if len(a.Orders) > 0 {
		var keep []AgentOrder
		for _, o := range a.Orders {
			// A listing at another station is haul-wish cargo that
			// arrived at its market: the pass never retracts it (its
			// units are escrowed there until sold), it only owns the
			// quotes on its own book.
			if desired[quoteKey{o.Side, o.Item}] || o.Station != a.Home {
				keep = append(keep, o)
				continue
			}
			s.CancelOrder(o.Station, o.ID, tag)
		}
		a.Orders = keep
	}
	for _, q := range quotes {
		limit := quantizeTick(q.price)
		if limit <= 0 {
			continue
		}
		// Never cross one's own resting quote (legacy orders after λ moved).
		if q.side == OrderBuy {
			if o, ok := a.ownOrder(OrderSell, q.item); ok && o.Price <= limit {
				continue
			}
		} else if o, ok := a.ownOrder(OrderBuy, q.item); ok && o.Price >= limit {
			continue
		}
		qty := q.qty
		if o, ok := a.ownOrder(q.side, q.item); ok {
			if o.Price == limit {
				continue // same price: the resting quote stands
			}
			s.CancelOrder(o.Station, o.ID, tag)
			a.dropOrder(o.ID)
		}
		budget := orderValueShare * a.freeCapital()
		if maxQty := int(budget / (limit * (1 + MarketBrokerFee))); maxQty < qty {
			qty = maxQty
		}
		if qty <= 0 {
			continue
		}
		res, ok := s.PlaceOrder(a.Home, q.side, q.item, qty, limit, tag)
		if !ok {
			continue
		}
		a.Capital -= MarketBrokerFee * limit * float64(qty)
		switch q.side {
		case OrderBuy:
			a.Capital -= res.Cost
			addInv(a, q.item, res.Filled)
			if res.Remaining > 0 {
				a.Orders = append(a.Orders, AgentOrder{ID: res.ID, Station: a.Home,
					Side: OrderBuy, Item: q.item, Price: limit, Qty: res.Remaining})
			}
		case OrderSell:
			a.Capital += res.Cost * (1 - MarketSalesTax)
			takeInv(a, q.item, res.Filled)
			if res.Remaining > 0 {
				a.Orders = append(a.Orders, AgentOrder{ID: res.ID, Station: a.Home,
					Side: OrderSell, Item: q.item, Price: limit, Qty: res.Remaining})
			}
		}
	}
	sort.Slice(a.Orders, func(i, j int) bool { return a.Orders[i].ID < a.Orders[j].ID })
}

// ---------------------------------------------------------------------------
// Role actions
// ---------------------------------------------------------------------------

// agentCollect delivers every JobDone job of this agent at its home
// station into its inventory (the industry seam: agent-owned jobs collect
// to the agent, not the player hangar).
func (s *Sim) agentCollect(a *Agent) {
	tag := agentOwnerTag(a.ID)
	for _, j := range s.Jobs(a.Home) { // ascending id: canonical
		if j.Owner == tag && j.State == JobDone {
			s.CollectJob(j.Station, j.ID, tag)
		}
	}
}

// agentStartJob starts one job (jobs per agent capped at 1) for the
// plan's positive-margin recipe, as many runs as free stock affords up to
// jobRunsCap. CanStartJob's silent check runs first — refusals never spam
// the log.
func (s *Sim) agentStartJob(a *Agent, p *agentPlan) {
	if a.Role != RoleIndustrialist || p.recipe == "" {
		return
	}
	tag := agentOwnerTag(a.ID)
	for _, j := range s.jobs {
		if j.Owner == tag {
			return // one job at a time
		}
	}
	runs := 0
	for r := jobRunsCap; r >= 1; r-- {
		ok := true
		for _, m := range s.JobMaterials(a.Home, p.recipe, r) {
			if a.freeStock(m.Item) < m.Qty {
				ok = false
				break
			}
		}
		if ok {
			runs = r
			break
		}
	}
	if runs == 0 {
		return
	}
	if _, _, ok := s.startJobCheck(a.Home, p.recipe, runs, tag); !ok {
		return
	}
	s.StartJob(a.Home, p.recipe, runs, tag)
}

// agentDrawOre is the miner's extraction: draw up to minerBatchUnits of
// the first belt rock that still has volume (capped by the ore stock cap),
// depleting persistent belt state exactly like player mining. When the
// belt is dry the miner draws nothing here and bids for ore instead
// (targets set dryBeltTarget in agentPlan) — it only idles if capital
// cannot back the bid.
func (s *Sim) agentDrawOre(a *Agent, p agentPlan) {
	if p.dry {
		return
	}
	sys, ok := s.stationSystem(a.Home)
	if !ok {
		return
	}
	ore, remaining, ok := s.firstBeltRock(sys)
	if !ok {
		return
	}
	if free := a.freeStock(ore); free < oreStockCap {
		unit := s.mineralUnitVolume(ore)
		if unit > 0 {
			want := min(minerBatchUnits, oreStockCap-free, int(remaining/unit))
			if vol := float64(want) * unit; vol > 0 && s.ExtractBeltOre(sys, vol) == vol {
				addInv(a, ore, want)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Haul wishes (the logistics seam)
// ---------------------------------------------------------------------------

// sortWishes orders wishes canonically (From, To, Item) — the list order
// HaulWishes() exposes and every rebuild re-asserts.
func sortWishes(ws []HaulWish) {
	sort.Slice(ws, func(i, j int) bool {
		if ws[i].From != ws[j].From {
			return ws[i].From < ws[j].From
		}
		if ws[i].To != ws[j].To {
			return ws[i].To < ws[j].To
		}
		return ws[i].Item < ws[j].Item
	})
}

// sameWishShipment reports whether two wishes are one shipment (owner,
// ends, item, quantity). MaxCost is deliberately ignored: it is recomputed
// from live spreads on every pass, while the commitment stands.
func sameWishShipment(a, b HaulWish) bool {
	return a.Owner == b.Owner && a.From == b.From && a.To == b.To &&
		a.Item == b.Item && a.Qty == b.Qty
}

// inFlightFor finds the committed shipment of qty units of item bound for
// station: the hauler/journey side carries only the cargo and the final
// destination (Traffic.Owner + Item/Qty/DestCel), so that is the match key.
func (a *Agent) inFlightFor(item content.ItemID, qty int, to StationID) (HaulWish, bool) {
	for _, w := range a.InFlight {
		if w.Item == item && w.Qty == qty && w.To == to {
			return w, true
		}
	}
	return HaulWish{}, false
}

// beginShipment commits a wish to the hauler now carrying its goods.
func (a *Agent) beginShipment(w HaulWish) { a.InFlight = append(a.InFlight, w) }

// dropInFlight releases a committed shipment — delivery (after FulfillHaul
// paid) or loss (unpaid). No-op when nothing matches.
func (a *Agent) dropInFlight(w HaulWish) {
	kept := a.InFlight[:0]
	for _, x := range a.InFlight {
		if !sameWishShipment(x, w) {
			kept = append(kept, x)
		}
	}
	a.InFlight = kept
}

// dropWish removes a wish from the pending list WITHOUT payment: the loss
// path (its goods were debited at spawn and never arrived). Any rebuilt
// twin of the same shipment goes with it.
func (a *Agent) dropWish(w HaulWish) {
	kept := a.Wishes[:0]
	for _, x := range a.Wishes {
		if !sameWishShipment(x, w) {
			kept = append(kept, x)
		}
	}
	a.Wishes = kept
}

// retainInFlight re-asserts committed shipments in a freshly rebuilt wish
// list: the pass recomputes wishes from live spreads, but one already
// riding a hauler stays pending until it delivers or is lost. The
// committed copy wins any collision with a rebuilt twin, so FulfillHaul's
// field-exact match still finds it.
func retainInFlight(a *Agent, out []HaulWish) []HaulWish {
	if len(a.InFlight) == 0 {
		return out
	}
	kept := make([]HaulWish, 0, len(out)+len(a.InFlight))
	for _, w := range out {
		dup := false
		for _, f := range a.InFlight {
			if sameWishShipment(w, f) {
				dup = true
				break
			}
		}
		if !dup {
			kept = append(kept, w)
		}
	}
	kept = append(kept, a.InFlight...)
	sortWishes(kept)
	return kept
}

// agentHaulAsk is the ask price a delivered haul-wish lists at: the
// agent's EWMA belief for the item marked up by its current ZIP margin
// (the opening margin until a pass sets one) — the same reservation×(1+µ)
// rule agentQuotes posts with, so a convoy delivery prices off the
// AGENT's beliefs and never off the system's one-tick-under-basis rule.
// Returns 0 for an item with no valuation at all (no belief, no base
// price — the caller then falls back to the convoy rule).
func (s *Sim) agentHaulAsk(a *Agent, item content.ItemID) float64 {
	v := s.agentBelief(a, item)
	if v <= 0 {
		return 0
	}
	µ, ok := a.Margin[item]
	if !ok || µ <= 0 {
		µ = zipMarginInit
	}
	return quantizeTick(v * (1 + clamp(µ, zipMarginMin, zipMarginMax)))
}

// haulSourceGoods moves a wish shipment's cargo out of the agent's stock
// at SPAWN time: the agent's own resting asks at the origin go first
// (cancelled whole — the surplus stays free in its stock), the remainder
// comes out of free stock. False when the agent cannot cover the full
// quantity: a wish is one shipment, never a partial, and nothing is
// mutated before the check passes.
func (s *Sim) haulSourceGoods(a *Agent, w HaulWish) bool {
	if w.Qty <= 0 {
		return false
	}
	avail := a.freeStock(w.Item)
	orders := append([]AgentOrder(nil), a.Orders...) // snapshot: dropOrder compacts in place
	for _, o := range orders {                       // ascending id: canonical
		if avail >= w.Qty {
			break
		}
		if o.Station != w.From || o.Side != OrderSell || o.Item != w.Item {
			continue
		}
		if !s.CancelOrder(w.From, o.ID, agentOwnerTag(a.ID)) {
			continue
		}
		a.dropOrder(o.ID) // ledger first: agentSettle must never see it as a fill
		avail += o.Qty
	}
	if avail < w.Qty {
		return false
	}
	takeInv(a, w.Item, w.Qty)
	return true
}

// haulPlaceAgentAsk lists a delivered wish shipment at its destination
// station under the agent's owner tag: the cargo re-enters the agent's
// stock and is escrowed straight into the ask (the ledger keeps it, so
// agentSettle credits the agent when it fills). An immediately crossing
// fill settles like any other agent sell — proceeds in (net of the sales
// tax), units out.
func (s *Sim) haulPlaceAgentAsk(a *Agent, station StationID, item content.ItemID,
	qty int, price float64) {
	addInv(a, item, qty)
	res, ok := s.PlaceOrder(station, OrderSell, item, qty, price, agentOwnerTag(a.ID))
	if !ok { // price <= 0 or no such station: the cargo stays as stock
		return
	}
	takeInv(a, item, res.Filled)
	a.Capital += res.Cost * (1 - MarketSalesTax)
	if res.Remaining > 0 {
		a.Orders = append(a.Orders, AgentOrder{ID: res.ID, Station: station,
			Side: OrderSell, Item: item, Price: price, Qty: res.Remaining})
		sort.Slice(a.Orders, func(i, j int) bool { return a.Orders[i].ID < a.Orders[j].ID })
	}
}

// stationItemPrice is a station's displayed price for an item ONLY when
// that station's book actually has data for it (trade, ask or bid) —
// data-less stations must not manufacture a spread off the base price.
func (s *Sim) stationItemPrice(station StationID, item content.ItemID) (float64, bool) {
	book := s.market[station]
	if book == nil {
		return 0, false
	}
	if _, ok := book.lastPrice(item); ok {
		return s.marketPrice(book, item), true
	}
	if _, ok := book.bestAsk(item); ok {
		return s.marketPrice(book, item), true
	}
	if _, ok := book.bestBid(item); ok {
		return s.marketPrice(book, item), true
	}
	return 0, false
}

// agentUpdateWishes rebuilds a trader's/industrialist's wish list for this
// pass: for every item it holds or quotes, when the cheapest and
// dearest data-bearing stations differ by more than the round-trip fee
// load it appends a HaulWish for the gap (MaxCost = spread − fees — what
// is left for the hauler). Miners produce no wishes; the list is rebuilt,
// never accumulated, so it cannot grow without bound — except that a wish
// already riding a hauler is re-asserted verbatim (retainInFlight), since
// its commitment outlives the spread that spawned it.
func (s *Sim) agentUpdateWishes(a *Agent) {
	set := map[content.ItemID]bool{}
	for item := range a.Inventory {
		set[item] = true
	}
	for _, o := range a.Orders {
		set[o.Item] = true
	}
	if len(set) == 0 {
		// Nothing left to quote or hold — but a shipment already on the
		// road stays pending until it delivers (or is lost).
		a.Wishes = retainInFlight(a, nil)
		return
	}
	items := make([]content.ItemID, 0, len(set))
	for item := range set {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i] < items[j] })

	stations := marketStations(s.Content)
	scale := agentQtyScale(a)
	qty := max(1, int(traderBaseQty*scale))
	var out []HaulWish
	for _, item := range items {
		var loStation, hiStation StationID
		lo, hi := 0.0, 0.0
		found := false
		for _, st := range stations { // sorted: canonical tie-breaks
			p, ok := s.stationItemPrice(st, item)
			if !ok {
				continue
			}
			if !found {
				lo, hi, loStation, hiStation, found = p, p, st, st, true
				continue
			}
			if p < lo || (p == lo && st < loStation) {
				lo, loStation = p, st
			}
			if p > hi || (p == hi && st > hiStation) {
				hi, hiStation = p, st
			}
		}
		if !found || loStation == hiStation || hi <= lo {
			continue
		}
		fees := (MarketBrokerFee + MarketSalesTax) * lo
		if hi-lo <= fees {
			continue // spread inside cost + fees: nothing to haul
		}
		maxCost := hi - lo - fees
		if maxCost > a.freeCapital() {
			continue // cannot fund the haul: skipped now, pruned by this rebuild
		}
		out = append(out, HaulWish{
			Owner: agentOwnerTag(a.ID), From: loStation, To: hiStation,
			Item: item, Qty: qty, MaxCost: maxCost,
		})
	}
	sortWishes(out)
	a.Wishes = retainInFlight(a, out)
}

// ---------------------------------------------------------------------------
// Station upkeep (the metabolism's demand side / money sink)
// ---------------------------------------------------------------------------

// upkeepBasket is one station's periodic repair/rearm basket, derived from
// what combat uses: every ammo item (rearm) plus the lowest-id armor
// repair module (repair), in sorted id order.
func (s *Sim) upkeepBasket() []content.CargoStack {
	var ammo []content.ItemID
	for id, def := range s.Content.Items {
		if def.Category == content.CatAmmo {
			ammo = append(ammo, id)
		}
	}
	sort.Slice(ammo, func(i, j int) bool { return ammo[i] < ammo[j] })
	out := make([]content.CargoStack, 0, len(ammo)+1)
	for _, id := range ammo {
		out = append(out, content.CargoStack{Item: id, Qty: upkeepChargeQty})
	}
	var mods []content.ModuleID
	for id, def := range s.Content.Modules {
		if def.Kind == content.KindArmorRep {
			mods = append(mods, id)
		}
	}
	sort.Slice(mods, func(i, j int) bool { return mods[i] < mods[j] })
	if len(mods) > 0 {
		out = append(out, content.CargoStack{Item: content.ItemID(mods[0]), Qty: upkeepRepairQty})
	}
	return out
}

// upkeepPass runs one upkeep cycle for every market station (canonical
// order): burn the maintenance fee from the station's ledger (the money
// sink — the endowment is seeded once and never refilled), then buy the
// basket against the local book at up to (displayed price × upkeepSlack),
// consuming the goods outright. The remainder is cancelled immediately:
// upkeep posts no standing bids. The ledger floors at zero — an exhausted
// station simply stops rearming.
func (s *Sim) upkeepPass() {
	if s.upkeep == nil {
		s.upkeep = map[StationID]float64{}
	}
	basket := s.upkeepBasket()
	for _, st := range marketStations(s.Content) {
		led, ok := s.upkeep[st]
		if !ok {
			led = upkeepEndowment
		}
		led -= min(upkeepFee, led)
		for _, line := range basket {
			if led <= 0 {
				break
			}
			limit := quantizeTick(s.MarketPrice(st, line.Item) * (1 + upkeepSlack))
			if limit <= 0 {
				continue
			}
			qty := line.Qty
			if maxQty := int(led / limit); maxQty < qty {
				qty = maxQty
			}
			if qty <= 0 {
				continue
			}
			res, ok := s.PlaceOrder(st, OrderBuy, line.Item, qty, limit,
				"upkeep:"+string(st))
			if !ok {
				continue
			}
			led -= res.Cost
			if res.ID != 0 {
				s.CancelOrder(st, res.ID, "upkeep:"+string(st))
			}
		}
		s.upkeep[st] = max(0, led)
	}
}

// ---------------------------------------------------------------------------
// The behavior pass
// ---------------------------------------------------------------------------

// agentKey is the pure schedule key of one agent (cooldown gate + act
// slot): a string no map iteration can reorder.
func agentKey(a *Agent) string { return "agent|" + a.ID }

// agentCooldownTicks is one agent's pacing window (CONTEXT.md "agent
// cooldown"): a jittered value in [agentCooldownMin, agentCooldownMax]
// derived from the same pure hash the traffic schedules use. It is a
// function of the id alone — no entropy, no persisted timer — so a
// restored roster resumes on exactly its own cadence.
func agentCooldownTicks(a *Agent) int64 {
	const span = agentCooldownMax - agentCooldownMin + 1
	return agentCooldownMin + int64(trafficPhase(agentKey(a))%uint64(span))
}

// agentAct runs one agent's full pass in canonical order: settle its view
// of the world (beliefs → values → plan), collect finished jobs, extract
// (miner — work, not a capital move), then ONE capital-moving step: the
// applicable steps of {commission a job, quote orders, contract a haul
// wish} rotate on the agent's own stateless slot (act ordinal + id phase),
// so an agent never performs two capital-moving actions inside one
// cooldown. A dormant agent (capital under dormantCapital) is fully inert.
func (s *Sim) agentAct(a *Agent) {
	if a.Capital < dormantCapital {
		return
	}
	s.agentCollect(a)
	candidates := s.agentCandidates(a)
	s.agentUpdateBeliefs(a, candidates)
	V := s.agentValues(a, candidates)
	p := s.agentPlan(a, V)
	if a.Role == RoleMiner {
		s.agentDrawOre(a, p)
	}
	quotes := s.agentQuotes(a, candidates, V, &p)

	key := agentKey(a)
	c := agentCooldownTicks(a)
	ord := (s.ticks + int64(trafficPhase(key)%uint64(c))) / c // this firing's ordinal: pure tick math
	var kinds []string
	if a.Role == RoleIndustrialist && p.recipe != "" {
		kinds = append(kinds, "commission")
	}
	kinds = append(kinds, "quote")
	if a.Role != RoleMiner {
		kinds = append(kinds, "contract")
	}
	switch kinds[(uint64(ord)+trafficPhase(key))%uint64(len(kinds))] {
	case "commission":
		s.agentStartJob(a, &p)
	case "quote":
		s.agentApplyQuotes(a, quotes)
	default: // contract: rebuild the haul-wish list
		s.agentUpdateWishes(a)
	}
	if a.Role == RoleMiner {
		a.Wishes = nil
	}
}

// agentPass runs every agent whose stateless cooldown fires this tick, in
// ascending id order (the canonical roster iteration — never map order).
// The gate is a pure hash of (tick, agent id): a skipped tick never
// reschedules anything, and restore reproduces the cadence from the tick
// count alone.
func (s *Sim) agentPass() {
	for i := range s.agents {
		a := &s.agents[i]
		if !trafficDue(s.ticks, agentKey(a), agentCooldownTicks(a)) {
			continue
		}
		s.agentAct(a)
	}
}

// ---------------------------------------------------------------------------
// Saved form
// ---------------------------------------------------------------------------

// SavedAgentVal is one (item, value) pair of an agent's belief/margin
// maps, persisted sorted by item.
type SavedAgentVal struct {
	Item content.ItemID `json:"item"`
	Val  float64        `json:"val"`
}

// SavedAgent is one agent's canonical Save form: sorted inventory
// (CargoStack), beliefs and margins, wishes and the shipments already on
// the road verbatim. Resting-order ledgers are derived from the saved
// books on restore (Step-boundary invariant), so they carry no field.
type SavedAgent struct {
	ID        string               `json:"id"`
	Home      StationID            `json:"home"`
	Role      AgentRole            `json:"role"`
	Capital   float64              `json:"capital"`
	Start     float64              `json:"start"`
	Inventory []content.CargoStack `json:"inventory,omitempty"`
	Beliefs   []SavedAgentVal      `json:"beliefs,omitempty"`
	Margin    []SavedAgentVal      `json:"margin,omitempty"`
	Wishes    []HaulWish           `json:"wishes,omitempty"`
	InFlight  []HaulWish           `json:"in_flight,omitempty"`
}

// saved converts live state to the canonical Save form (sorted keys).
func (a Agent) saved() SavedAgent {
	sv := SavedAgent{ID: a.ID, Home: a.Home, Role: a.Role,
		Capital: a.Capital, Start: a.Start}
	if len(a.Inventory) > 0 {
		for item, qty := range a.Inventory {
			if qty > 0 {
				sv.Inventory = append(sv.Inventory, content.CargoStack{Item: item, Qty: qty})
			}
		}
		sort.Slice(sv.Inventory, func(i, j int) bool {
			return sv.Inventory[i].Item < sv.Inventory[j].Item
		})
	}
	if len(a.Beliefs) > 0 {
		for item, v := range a.Beliefs {
			sv.Beliefs = append(sv.Beliefs, SavedAgentVal{Item: item, Val: v})
		}
		sort.Slice(sv.Beliefs, func(i, j int) bool { return sv.Beliefs[i].Item < sv.Beliefs[j].Item })
	}
	if len(a.Margin) > 0 {
		for item, v := range a.Margin {
			sv.Margin = append(sv.Margin, SavedAgentVal{Item: item, Val: v})
		}
		sort.Slice(sv.Margin, func(i, j int) bool { return sv.Margin[i].Item < sv.Margin[j].Item })
	}
	if len(a.Wishes) > 0 {
		sv.Wishes = append([]HaulWish(nil), a.Wishes...)
	}
	if len(a.InFlight) > 0 {
		sv.InFlight = append([]HaulWish(nil), a.InFlight...)
	}
	return sv
}

// agentFromSaved validates and rebuilds one agent from its Save form
// (trust boundary: a save is user-editable — reject anything no live run
// could produce). Orders are not part of the form; the caller rebuilds
// them from the books.
func agentFromSaved(sv SavedAgent) (Agent, error) {
	if sv.ID == "" || sv.Home == "" {
		return Agent{}, errf("sim: save has an agent with no id or home")
	}
	if sv.Capital < 0 || sv.Start < 0 {
		return Agent{}, errf("sim: save agent %s has negative capital", sv.ID)
	}
	a := Agent{
		ID: sv.ID, Home: sv.Home, Role: sv.Role,
		Capital: sv.Capital, Start: sv.Start,
		Inventory: map[content.ItemID]int{},
		Beliefs:   map[content.ItemID]float64{},
		Margin:    map[content.ItemID]float64{},
	}
	for _, st := range sv.Inventory {
		if st.Qty < 0 {
			return Agent{}, errf("sim: save agent %s holds negative %s", sv.ID, st.Item)
		}
		a.Inventory[st.Item] += st.Qty
	}
	for _, b := range sv.Beliefs {
		if b.Val < 0 {
			return Agent{}, errf("sim: save agent %s has a negative belief", sv.ID)
		}
		a.Beliefs[b.Item] = b.Val
	}
	for _, m := range sv.Margin {
		if m.Val < 0 {
			return Agent{}, errf("sim: save agent %s has a negative margin", sv.ID)
		}
		a.Margin[m.Item] = m.Val
	}
	if !validWishList(sv.Wishes) || !validWishList(sv.InFlight) {
		return Agent{}, errf("sim: save agent %s has an invalid haul wish", sv.ID)
	}
	a.Wishes = append([]HaulWish(nil), sv.Wishes...)
	a.InFlight = append([]HaulWish(nil), sv.InFlight...)
	return a, nil
}

// validWishList is the trust-boundary check on a saved wish list: a save
// is user-editable, so anything no live run could produce is rejected.
func validWishList(ws []HaulWish) bool {
	for _, w := range ws {
		if w.Qty < 0 || w.MaxCost < 0 || w.Owner == "" {
			return false
		}
	}
	return true
}
