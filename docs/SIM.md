# Entieve — Simulation Reference

> **Scope note.** This document is a *reference*, not a contract: it explains
> what the simulation does and why, and has **no binding authority**.
> [ARCHITECTURE.md](ARCHITECTURE.md) remains the binding cross-workstream
> contract, [CONTENT_AUTHORING.md](CONTENT_AUTHORING.md) the data cookbook,
> and `docs/lore/` the canon. Where this file and the code disagree, the code
> wins — file names are cited in passing so you can check every claim.

Everything below is the deterministic core in `sim/`: the fixed-step
simulation, its command/event surfaces, the world metabolism that keeps
goods flowing, the ledger-agent economy, and the order books that form
prices. It runs headless; the Ebitengine client is a viewer of the read
model described in ARCHITECTURE.md.

## 1. Tick anatomy

`Sim.Step()` (`sim/api.go`) advances exactly one tick — `sim.TickRate` = 60
ticks/second, `sim.DT` = 1/60 s of simulated time. `Time()` is derived from
the tick count (never accumulated), so it is an exact function of `Ticks()`
across snapshot/restore. Queued commands are consumed first, then systems
run in this fixed order:

| # | System | File | Does |
|---|--------|------|------|
| 1 | command | `sim/api.go` | drains the command queue into intents / deferred buckets |
| 2 | movement | `sim/system_move.go` | sublight accel toward desired velocity, web effects |
| 3 | warp | `sim/system_warp.go` | align → warp → land state machine |
| 4 | targeting | `sim/system_target.go` | lock progress (scan res vs. signature) |
| 5 | modules | `sim/system_modules.go` | cycle timers, activation cost, passive aggregation |
| 6 | weapons | `sim/system_weapons.go` | turret math, ammo consumption |
| 7 | damage | `sim/system_damage.go` | shield → armor → structure; death → wreck |
| 8 | capacitor | `sim/system_cap.go` | EVE recharge curve |
| 9 | mining | `sim/system_mining.go` | cycle yields into the ore hold; belt-state decrement |
| 10 | npc | `sim/system_npc.go` | rat/guard behavior state machine |
| 11 | loot | `sim/system_loot.go` | loot command → cargo; bounties |
| 12 | economy | `sim/system_economy.go` | station services: buy/sell/refine/repair/fitting |
| 13 | travel | `sim/system_travel.go` | dock/undock/jump: grid swaps |
| 14 | mission | `sim/system_mission.go` | contract accept/abandon, objectives, auto-claim |
| 15 | respawn | `sim/system_respawn.go` | funded NPC replacement timers |
| 16 | traffic | `sim/system_traffic.go` | convoys, ambushes, patrols, book supply, journeys |
| 17 | industry | `sim/system_industry.go` | manufacturing jobs, craft growth |
| 18 | agents | `sim/system_agent.go` | ledger economy: settle, behavior pass, upkeep |

After all systems, the **deferred player death respawn** runs
(`pendingPlayerRespawn`): a cross-system clone-bay respawn rebuilds the
grid, which would otherwise yank entities out from under the systems still
running this tick. Events are not a system — they are capped at emit time
(`maxEvents` = 512 in `sim/helpers.go`, oldest dropped), so there is
nothing to trim at the end of a Step. The tick counter and derived clock
close the Step.

Ordering is load-bearing, not decorative: traffic runs late because it
posts supply to the books; industry and agents run after everything that
can fill their orders or requisition their goods; the death respawn runs
last so the rebuilt grid is never observed mid-Step.

## 2. Contracts

### Determinism

`New(c, w, seed)` plus the same command sequence produces an identical
`Hash()` at every checkpoint. The machinery behind that guarantee:

- **SplitMix64 master stream + fork registry.** All randomness goes through
  `core.RNG` (SplitMix64, golden-pinned in `core/rng_test.go`).
  `Fork(stream)` derives independent streams; every NPC holds its own fork,
  and the living-universe stream (`trafficRNG`, keyed `trafficStreamID` in
  `sim/system_traffic.go`) is forked from the master at `New`.
- **Role pins.** The master stream carries exactly two roles — player
  combat rolls and spawn entropy (asteroid/NPC scatter plus NPC fork
  seeds) — and `sim/rng_roles_test.go` pins its draw set, so a future
  subsystem reaching for `s.RNG` fails a test. Traffic draws only its own
  stream (`TestTrafficNeverDrawsMasterStream`). The agent layer draws no
  RNG at all: imputation, ZIP and upkeep are pure math.
- **Canonical sorted iteration.** Systems iterate entities in handle order
  (`allEntities`, `sortEntities`), markets in ascending station id
  (`stationsSorted`), books in canonical order (item, side, price, arrival
  id), jobs ascending by id, craft by (station, family), agents ascending
  by id. Map iteration never reaches behavior or serialization.
- **Stateless schedules.** Convoy/patrol cadences are hashes of
  (tick, key, interval) (`trafficDue`), not persisted timers; agent acts
  fire on each agent's own hashed cooldown (`agentCooldownTicks`). A
  skipped spawn attempt never reschedules anything.
- **The beat — paced commencement.** Nothing commences the tick it first
  becomes possible (CONTEXT.md: departure beat / gate cycle / turnaround /
  agent cooldown): spawns and resumes hold a departure beat (120–480
  ticks), gate arrivals hold a gate cycle (dwell 120–300, then a jump
  cycle of 60–180 between hop-out and materializing at the destination
  gate), station arrivals run a turnaround (approach 120–240, then a
  docked dwell of 300–900 whose cargo action fires partway through, then
  undock + departure beat), and each agent's capital actions sit at least
  one jittered cooldown (60–600 ticks) apart, at most one capital-moving
  step per act. Every window is jittered by `trafficBeat` — a pure hash of
  (tick, canonical key), no entropy — and the running deadlines ride the
  already-persisted `Traffic.Next`/`Traffic.Phase` fields, so the beat
  changes no save schema.
- **Twin-run tests.** `sim/determinism_test.go`, `sim/world_metabolism_test.go`,
  `sim/traffic_test.go` and friends drive two sims through the same
  commands and compare `Hash()`.

### Event surface

`Sim.Events()` (alias `DrainEvents()`) drains the pending list. One struct
covers everything — `sim.Event` in `sim/model.go` — with kinds `EvLog`,
`EvDestroyed`, `EvWarp`, `EvLooted`, `EvDocked`, `EvUndocked`, `EvJumped`,
`EvMarket`, `EvModule`, `EvTargetLocked`, `EvCargoFull`, `EvAnomalyCleared`,
`EvMission`, `EvTraffic`. Phases: `EvWarp` carries `"start" | "land" |
"scrammed"`; `EvMission` carries `"accepted" | "progress" | "ready" |
"completed" | "abandoned" | "refused"` (with `ID`/`Name` for the contract);
`EvTraffic` carries `"departed" | "jumped" | "delivered" | "attacked" |
"destroyed" | "escaped" | "patrol-response" | "saved"` (with `System`/`Cel`
for the route endpoint or incident celestial, `Item`/`Value` for the
shipment). `Severity` is `SevInfo | SevGood | SevWarning | SevDanger`.

### Command surface

`sim.Command` (`sim/model.go`) is the only way to mutate game state:
`Queue` it, it executes at the start of the next `Step`. The full set:
`CmdStop`, `CmdManualVel`, `CmdApproach`, `CmdOrbit`, `CmdWarpTo`,
`CmdAlignTo`, `CmdLock`, `CmdUnlock`, `CmdActivate`, `CmdDeactivate`,
`CmdSetAmmo`, `CmdLoot`, `CmdMine`, `CmdDock`, `CmdUndock`, `CmdJump`,
`CmdBuy`, `CmdSell`, `CmdRefine`, `CmdRepair`, `CmdAssemble`,
`CmdFitModule`, `CmdUnfitModule`, `CmdTransfer`, `CmdAcceptMission`,
`CmdAbandonMission`, `CmdStartJob`, `CmdCollectJob`. Station services and
industry commands ride same-Step deferral buckets (`deferEcon`,
`deferIndustry`) to the systems that own them; refusals are events, never
silent.

### Save schema v2 and `Hash()`

`Save` (`sim/model.go`) is the full serializable snapshot: world/system/
ticks/seed, both RNG stream states, wallet, standing, missions, the hangar,
every entity's components, the free-slot mirror (exact future handles), the
respawn anchor, module target bindings — and the living-economy state the
v2 schema added: `Market` (per-station books), `MarketLasts`,
`MarketSeq`, `Belts`, `FactionLedger`, `Jobs`, `JobSeq`, `Craft`,
`Agents`, `Upkeep`, `TrafficJourneys` (plus `TrafficLeaves`).
Every list carries its canonical order and map keys marshal sorted, so the
JSON is canonical.

`Hash()` (`sim/hash.go`) is FNV-1a 64 over that canonical Save JSON — by
construction the whole state. There is no degraded fallback: a
non-finite float makes `hash()` panic through `hashFail` rather than
return a plausible-looking partial fingerprint. Because `hash()` and
`Restore()` share the serialization, a snapshot → restore round trip
preserves `Hash()` exactly.

`Save.Schema` is `saveSchemaV2` = 2 and `Restore` rejects anything else —
the **clean break** introduced with the per-station market tranche (the
v0.2.0 era). Pre-v2 saves carried per-system stock lists with drifting
prices; restoring one into a book-based economy would silently mean a
different market, so it fails loudly instead. Two pieces of state are
deliberately outside the versioned schema and reset on Restore by design:
per-(owner, slot) ammo preferences and the recent-player-work aggro
trigger (`sim/save_completeness_test.go` classifies every `Sim` field and
fails on unclassified additions).

## 3. World metabolism

Seven rules keep goods real: nothing is conjured, nothing disappears
without bookkeeping. The tests in `sim/world_metabolism_test.go` and
`sim/logistics_test.go` pin each.

1. **Extraction — persistent belts.** Every belt rock carries remaining
   volume in `belts` (`Save.Belts`), seeded from the `BeltDef` on first
   generation and never re-seeded by a reload. Mining and rat requisitions
   decrement it in step with the live asteroid entities; every 600-tick
   window a rock regrows **+1 % of its definition volume**, capped at that
   volume (`systemBelts`, `sim/system_mining.go`). A drained belt stays
   drained until regrowth — across save/restore and system reloads.
2. **Refining — ore to minerals.** `CmdRefine` at a `refine` station turns
   ore units into minerals at `floor(refine_yield × qty)` per mineral
   (`execRefine`, `sim/system_economy.go`). Tier 1 of the two-tier recipe
   economy; outputs land in the general hold.
3. **Manufacturing — jobs and craft growth.** Tier 2: `RecipeDef`
   (`content/data/recipes/`, `rec-*`) turns minerals into ships, modules or
   charges at an `industry` station. `CmdStartJob` commits the materials
   for every run up front (integer units) at a station's 2 concurrent
   slots; `CmdCollectJob` delivers the output to the hangar. Timing is
   pure clock math (`Job.StartedAt`/`DoneAt`) — zero RNG. Craft grows per
   (station, recipe family): `L = min(5, 1 + floor(sqrt(runs)/2))`, each
   level cutting job time 4 % and materials 2 % below the L1 baseline
   (both frozen at job start). Every 3600 idle ticks a family's counter
   decays one run-equivalent, floor 0 (`sim/system_industry.go`).
4. **Distribution — real-cargo convoys.** Route cargo is **sourced at
   spawn, never conjured** (`sim/system_traffic.go`): a *station* origin
   withdraws the units from that station's own asks (the weighted price
   paid is the shipment's cost basis); a *belt* origin draws the persistent
   belt state (dry belt ⇒ no shipment this cycle); a *gate* origin is the
   economy's one sanctioned import — the named world boundary, where
   shipments arrive undebited and post as supply only at the destination.
   An origin without stock spawns nothing. Delivery posts the shipment as
   an ask on the destination station's book one tick under
   `min(cost basis, best ask)`, so arriving supply always improves the
   displayed price; a posting that crosses the destination's bids executes
   immediately at the resting bid (never-crossed invariant) and only the
   remainder rests. Cross-system legs — and haulers whose grid was rebuilt
   under them — park as journeys (`Save.TrafficJourneys`) and continue when
   their system loads next; multi-hop routing finds the next gate toward
   the destination (`trafficNextHop`). Authored interval lanes
   (`content/data/traffic/`) coexist with this ledger.
   Everything commences on the beat (this file's §2): a departure beat
   after spawn/resume, the gate cycle at stargates (dwell → hop → jump
   cycle), and a turnaround at station endpoints — approach, then the
   docked dwell with the delivery posting partway through (the hold
   leaves the hauler exactly when the book gains it, so the units are
   counted once), then undock + departure beat before the hauler leaves
   the grid. A gate-queue hold announces itself once on entering the
   hold: the held shipment waits in the ledger and hops straight across
   once the far queue drains, closing the episode with one jump line —
   no respawn/re-hold cycle exists to re-log.
5. **Consumption — real burns.** Guns consume charges per shot (dry guns
   refuse to fire). Every 3600 ticks each market station's **upkeep** cycle
   burns a 5,000 € maintenance fee and buys a repair/rearm basket (every
   ammo item × 50 plus the lowest-id armor-repair module) against its own
   book at up to ×1.25 the displayed price, consuming the goods outright —
   a money sink seeded once with a 100,000,000 € endowment and never
   refilled (`upkeepPass`, `sim/agent.go`). Replacement NPCs are
   **requisitioned**: a guard buys its hull, fit and 1,000-round charge
   basket from the respawn station's book against its faction's ledger
   (`Save.FactionLedger`, likewise seeded once — only respawn requisitions
   debit it); a rat pays belt ore (1 m³ per 200 mineral units of its hull
   recipe bill) or buys local ore asks when the belt cannot pay. An
   unfunded requisition warns and re-queues 600 ticks later, forever — a
   replacement never spawns from nothing (`sim/system_respawn.go`).
6. **Destruction — losses destroy debited goods.** A hauler killed by
   raiders or by you takes its shipment to a **lootable wreck**; unlooted,
   those goods are gone. The origin book was debited at spawn, so no
   reversal ever touches it, and a shipment that never arrived never
   touched the destination book. The loss surfaces as an event-only
   supply-risk signal ("Risk premium — …" in the log), not a price step.
   Player cargo is lost on death; NPC death drops a wreck and pays the
   bounty.
7. **Reprocessing — wreck reclaim.** A destroyed ship's wreck carries
   `floor(0.4 × hull-recipe mineral inputs)` as reclaimed minerals — the
   Scuttler's 19,900 ferrite + 2,800 silex bill reclaims 7,960 + 1,120
   (`reclaimedMinerals`, `sim/system_damage.go`). The value-added part —
   fitting, labor, craft — burns off; rolled loot is a separate table.

## 4. Economy agents

System 18 runs the **ledger agents** (`sim/agent.go`): autonomous traders,
miners and industrialists that make the economy player-driven without
players. The design is the approved one in
[research/price-formation-brief.md](research/price-formation-brief.md)
§5 candidate A, layered over ZIP placement.

**Roster.** 24 agents (`agentRosterSize`) spread over every market station
— `floor(24 / #stations)` each, ≥ 1. The shipped six-station universe gets
four per station: industry stations lead with two **industrialists** and
then alternate trader/miner, market-only stations alternate from the top —
6 industrialists, 9 traders, 9 miners. Ids (`ag-001`…), homes, roles and
starting capital (250,000 €) are pure functions of content: no RNG. The
roster is world-gated to `world-default` (`agentWorlds`); the showcase tour
and test fixtures run without agents — and therefore without upkeep.

**State** per agent: `ID`, `Home` station, `Role` (a bootstrap leaning,
not a class), `Capital` and `Start`, `Inventory`, `Beliefs`, `Margin`
(one per item), `Wishes` (haul requests), `Orders` (the resting-order
ledger). Capital growth is the selector: order sizes scale with
`Capital/Start` (clamped 0.1–5.0), one order's value is capped at a
quarter of free capital, and an agent under 1 € takes no actions at all.

**Three rules** (few rules, emergent outcomes):

1. **Impute value** (Menger/Wieser). Consumption-anchored goods are valued
   at the agent's EWMA belief of executed trade prices. Recipe *inputs* are
   backward-imputed through the recipe tree to a fixpoint —
   `V(m) = max over recipes r containing m of ((MV(out_r) − Σ other inputs) / qty)`
   with `MV` = belief net of the 5.5 % round-trip fee load — and ore floors
   at its refine yield. Reservation prices spread ±25 % around `V` by
   inventory need: deficit raises the bid, surplus discounts the ask, and
   no order ever crosses the agent's own valuation.
2. **Post ZIP margins.** One margin µ per item; quotes rest at `V·(1±µ)`
   through the real order-book API (so ticks, broker fee and sales tax are
   real friction). Each pass µ moves by Widrow-Hoff toward the target
   implied by observed prices, with momentum over the agent's own last
   shout: `µ += β(µ* − µ) + α(µ − µ_implied)`, β = 0.25, α = 0.05, clamped
   to [0.01, 1.0] from an opening 0.10.
3. **Act where margin is positive.** Miners draw belt ore and sell it (or
   restock when their belt runs dry); industrialists commission the
   best-margin recipe and restock its materials while that margin is
   ≥ 0 (within float noise — imputation makes the binding recipe
   break-even by construction); traders market-make and stockpile a small
   basket.

**Cadence.** Resting fills settle every tick (`agentSettle`); each agent's
behavior act fires on its own stateless cooldown (a hashed 60–600-tick
window per agent id — CONTEXT.md "agent cooldown"), and one act performs
at most ONE capital-moving step: the applicable steps of {commission a
job, quote orders, contract a haul wish} rotate on a hashed slot of the
act ordinal, so two capital-moving actions are never less than one
cooldown apart. Station upkeep every 3600 ticks. Agents iterate in
ascending id order everywhere and draw zero RNG.

**Haul wishes and fulfillment.** Traders and industrialists rebuild a wish
list each pass: for every item they hold or quote, when the cheapest and
dearest data-bearing stations differ by more than the round-trip fee load,
they append a `HaulWish` for the gap (`MaxCost` = spread − fees — what is
left for the hauler). Miners produce no wishes; the list is rebuilt, never
accumulated. The fulfillment contract is wish-driven logistics: the
logistics side spawns a convoy per wish — the cargo is **debited from the
owning agent's stock at spawn**, delivered as **agent-owned asks at the
destination at the agent's reservation price**, and the **hauling cost is
paid by the agent** (charged against its free capital — resting-order
escrow is untouchable — and burned, a money sink). Authored interval lanes
coexist with the wish lane. *Landed:* `sim/system_traffic.go`'s wish poll
(`trafficSpawnWishHaulers`) consumes the seam — one validated shipment per
60-tick poll after the authored lanes, sourced from the owning agent's own
asks and stock (all-or-nothing), owner-tagged through the journey ledger,
delivered as asks at `belief × (1 + margin)`, and settled through
`FulfillHaul` at **2.5 € per m³ per transit leg**, charged on delivery —
a lost shipment drops the wish unpaid.

## 5. Price formation

**Per-station continuous double auction.** Each station keeps one order
book (`MarketBook`, keyed by `StationID` = the station's celestial id,
universe-wide — content validation rejects a station id shared by two
systems). The book is the *only* price former: there is no band, no decay,
no drift model.

- **Never-crossed.** An incoming order executes against the opposite side
  in price-time priority at the *resting* orders' prices; only the
  unfilled remainder rests (`matchCrossing` / `applyMatches` in
  `sim/model.go`), so a book can never cross itself.
- **Price-time priority.** Canonical book order — item ascending, asks
  before bids, best price first, arrival id (`MarketSeq`) breaking ties.
- **Price ticks.** Every order price is quantized to four significant
  figures (`quantizeTick`, `sim/system_economy.go`): 48 ticks in 0.01
  steps, 195000 in 100s. `tickDown` is the one-tick undercut fresh supply
  lists at.
- **Fees.** A 3 % broker fee rides the placed value of a buy at placement
  (never refunded, EVE's rule); a 2.5 % sales tax rides the executed value
  of a sell at execution. A round trip costs 5.5 % — the institutional
  load every quote and wish accounts for.
- **Full escrow.** A player buy debits price × qty + fee up front and
  refunds the escrow of whatever never trades (price improvement included);
  a player sell escrows the goods themselves (fitted hulls are refused).
  Agent buys escrow capital, agent sells escrow inventory.
- **Player Buy/Sell are market orders**: immediate-or-cancel against the
  docked station's book at the resting prices — whatever the book cannot
  fill simply stays with the seller/buyer (`execBuy`/`execSell`).
  `PlaceOrder` is the resting surface used by traffic supply, agents and
  upkeep.

**Displayed price** (`MarketPrice`) is a ladder: last trade → best ask →
best bid → the item's base price. Moves print as `EvMarket` lines —
"Surplus — … price falls to …" / "Demand — … price rises to …" — the
displayed price of the book moving, not an external model.

**Beliefs** are EWMA of executed trades (γ = 0.1 on the last trade price at
the agent's home book; own quotes never move a belief), bootstrapped from
the ladder's base-price rung. **Imputation anchors input value to product
value**, so a mineral is worth what its most profitable use is worth net
of fees — prices in the ring are chained through the recipe tree rather
than set by fiat.

Theory grounding — why marginal utility plus ZIP converges in agent-based
economies, and the evidence — lives in
[research/price-formation-brief.md](research/price-formation-brief.md)
(§1 marginal utility & imputation, §2 ACE convergence, §3 EVE market
microstructure, §5 the candidate mechanisms).

## 6. Extension guide (descriptive)

How the seams above are extended in practice. Not rules — patterns the
code already follows.

**A new sim system.** Systems are zero-argument methods called from
`Step` in numbered order. Give the new system the next number (or the
slot its ordering semantics demand: after everything that feeds it, before
everything it feeds — traffic before industry before agents is that
argument), add a numbered comment line in `sim/api.go`, and route its
commands through a `defer*` bucket the way `deferEcon`/`deferIndustry`
do. Keep same-Step handoffs inside the Step; never let per-tick working
state survive a Step boundary (the completeness test enforces it).

**Save/hash coverage.** Any new persistent state gets a `Save` field in
canonical order (sorted slices; maps marshal sorted), a classification in
`sim/save_completeness_test.go` (save-carried / derived / per-Step
transient / documented exception), and — automatically — hash coverage:
`Hash()` digests the whole Save JSON, so a field that serializes is a
field that hashes. Additive fields ride schema 2 with `omitempty` (as
`Belts`, `Jobs`, `Agents`, `Upkeep`, `TrafficJourneys` did); only a
semantic break earns a new `Save.Schema` value and a Restore rejection.

**RNG discipline.** No `math/rand`, no wall clock. Master stream = player
combat rolls + spawn entropy, nothing else (`rng_roles_test.go` pins it).
A stochastic subsystem forks its own stream with a stable stream-id
constant (`trafficStreamID` pattern) and never touches the master; prefer
pure-math subsystems (industry, agents) and stateless hashed schedules
over RNG and timers alike.

**Event voice.** Reuse existing `Event` kinds and phase registries before
adding new ones; new phases are registry constants in `sim/model.go`, not
ad-hoc strings. Money strings go through `euro()` (`sim/helpers.go`):
thousands separators, two decimals, trailing `15,000.00 €`, bare `50¢`
under one euro — the convention `docs/lore/NAMING.md` ("Money strings")
binds. Log text is world-facing prose: NAMING.md names, AESTHETIC.md
voice, zone words (Charter space / the Fringe / Unincorporated) in place
of `high`/`low`/`null`. Events are capped at 512 at emit time; spammy
repeats go through the warn-interval pattern.

**New content kinds.** Content ids are frozen once shipped
(CONTENT_AUTHORING.md, "ID naming rules"); data goes in `content/data/**`
and gets its own authoring recipe there. A genuinely new *kind* is a
schema change: `content/model.go` struct + `content/validate.go`
cross-checks + the ID registry in ARCHITECTURE.md + a NAMING.md registry
row — then a showcase checkpoint so `tests/` keeps proving the feature
end-to-end.
