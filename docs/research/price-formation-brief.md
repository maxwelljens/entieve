# Research Brief — Price Formation in an Agent-Based Game Economy

**Question:** How should prices form in a deterministic sandbox economy of simple autonomous
"ledger agents" (state: capital + inventory; behaviours: extract → refine → manufacture → haul →
sell), grounded in the Austrian school's theory of marginal utility (incl. imputation /
*Zurechnung*), with concrete operationalizations from the literature (ACE, zero-intelligence
trading), EVE Online market practice, and game-economy heuristics? Target: **convergence**, not
boom/bust. No developer-set base prices.

**Scope decisions taken (assumptions):** the brief covers (1) Austrian value/imputation theory in
its own texts, (2) ACE price-formation results that speak to convergence, (3) EVE market
microstructure + CCP economy practice, (4) shipping game-economy formulas (WoW TSM), and
(5) three implementable candidate mechanisms. "Operationalization" is interpreted as: an
explicit rule an agent can compute per tick (utility schedule, reservation price, order
placement, clearing rule). No repository paths were given in the brief; findings are saved at
`docs/research/price-formation-brief.md` (new `docs/research/` directory — repo has no prior
research-notes convention).

**Fact vs. inference:** everything under "Evidence" sections is quoted from a cited source.
Everything under "Candidate mechanisms", "Recommended operationalization" and any sentence
marked *(inference)* or *(opinion)* is my synthesis, not a source claim.

---

## 1. Austrian marginal utility & imputation, operationalized

### 1.1 What the theory actually says (primary texts)

**Subjective, marginal value (Menger).** Value is the importance of the *least* important
satisfaction that depends on a unit of the available stock — a computable "last-use" rule:

> "(5) The value of a particular good or of a given portion of the whole quantity of a good at the
> disposal of an economizing individual is thus for him equal to the importance of the least
> important of the satisfactions assured by the whole available quantity and achieved with any
> equal portion." — Menger, *Principles* (1871), §3.ii.b.35,
> https://mx.nthu.edu.tw/~cshwang/all-my-teaching/political-economics/PE03-Subjectivism/03-3-Menger%20on%20Goods%20and%20Money/Menger=ch3%20The%20Theory%20of%20Value.htm

Value is explicitly *not* inherited from inputs (the anti-cost-theory clause):

> "The value an economizing individual attributes to a good is equal to the importance of the
> particular satisfaction that depends on his command of the good. There is no necessary and direct
> connection between the value of a good and whether, or in what quantities, labor and other goods
> of higher order were applied to its production." — ibid., §3.ii.d.3.

**Imputation (Zurechnung).** Production goods ("higher-order goods") are valued backward from
expected output:

> "We therefore have the principle that the value of goods of higher order is dependent upon the
> expected value of the goods of lower order they serve to produce." — ibid., §3.iii.a.3
> (same sentence at https://mises.org/mises-daily/value-goods-higher-order).

Complementarity matters — a recipe needs *all* inputs, so single inputs are valued via the bundle
(Menger's own caveat):

> "[I]t is never in our power to make use of any particular good of higher order for the
> satisfaction of our needs unless we also have command of the other (complementary) goods of
> higher order." — Menger, *Principles*, §1.iii.a.2,
> https://mx.nthu.edu.tw/~cshwang/all-my-teaching/political-economics/PE03-Subjectivism/03-3-Menger%20on%20Goods%20and%20Money/Menger=ch1%20The%20General%20Theory%20of%20the%20Good.htm

Production takes time and is guided by *expected* (future) demand — valuation is forward-looking,
not accounting backward:

> "Goods of higher order acquire and maintain their goods-character, therefore, not with respect to
> needs of the immediate present, but as a result of human foresight, only with respect to needs
> that will be experienced when the process of production has been completed." — ibid., §1.iv.3.

**Wieser's completion of imputation** gives the two rules that make imputation *computable*:

- **Marginal law for inputs in stock** (this is the exact analogue of Menger's marginal law, one
  level up):
  > "Where production goods are in stocks imputation must follow the marginal law: although used to
  > produce different values of product, the value of all similar productive items must be similar,
  > and can only be that derived from the least valuable product." — Wieser, *Natural Value* (1893),
  > Book III, Ch. VIII, analysis of contents, https://praxeology.net/FW-NV.htm
- **Opportunity-cost definition of costs** (costs are themselves derived from value, never the
  reverse):
  > "In production, wealth already in existence in the form of production goods is destroyed. Thus
  > the idea of 'costs,' as sacrifice of wealth capable of other uses than that to which it is
  > actually put." — ibid., Book IV, Ch. II.
- **"Productive contribution" of complementary factors** resolved by simultaneous valuation:
  > "It is because the productive elements enter into innumerable combinations, each with different
  > values, that we get, by a method of equations, the contributions imputed to each – the
  > 'productive contribution.'" — ibid., Book III, Ch. V.

*(Inference — the operational core)*: for a mass-conserving production tree, imputation is a
fixpoint over recipes: for material `m`, imputed unit value
`V(m) = max over recipes r containing m of [ (MV(output_r) − Σ_{m'≠m} q_{m'}·V(m')) / q_m ]`,
where `MV` is the *marginal* (last-unit) value of the output. `max` = Wieser's opportunity cost
("sacrifice ... capable of other uses"); the subtraction of other inputs at their imputed values =
Wieser's productive-contribution problem; iterating to a fixpoint across the chain is his "method
of equations". Note Menger's imputation is explicitly about *expected* output value, so `MV` should
use expected selling conditions, not last realized price alone.

### 1.2 Computational translations of marginal utility / WTP

- **Sugarscape (Epstein & Axtell 1996) — the canonical "marginal utility → trade price"
  operationalization.** Agents hold stocks with per-capita metabolism; the marginal rate of
  substitution is `MRS = (w2/m2)/(w1/m1)` (expected duration of stock 2 over stock 1), trade occurs
  only if both gain, and:
  > "The trading price that the agents will set on is p = √MRSA\*MRSB. ... Before the trade is made,
  > both agents will be checked to benefit from the trade in their wealth level."
  > and the result: "this local trading, where prices are determined locally by bargaining, leads to
  > an average price of sugar and spice that is the same as the theoretical optimum when agents have
  > unlimited knowledge." — *Introduction to Agent-Based Modeling*, Ch. 9 "Sugarscape",
  > https://pressbooks.pub/intro2abm/chapter/sugarscape/ (restating Epstein & Axtell 1996, Ch. 4;
  > primary text excerpt: https://complexityexplorer.s3.amazonaws.com/Mesa+ABM/epstein_axtell.pdf —
  > "each agent trades not at the general equilibrium price but rather at a locally negotiated one").
  *(Inference)*: WTP for the k-th unit = the marginal-utility weight of the k-th use (Menger's
  ranked-uses schedule), scaled by the agent's marginal utility of capital; Sugarscape's Cobb-Douglas
  welfare `W = w1^(m1/(m1+m2)) · w2^(m2/(m1+m2))` is one concrete diminishing-MU schedule.
- **WoW's TradeSkillMaster — a shipping "imputation" formula.** Crafted-item cost is the sum of
  material prices, and destroyed items (≈ your "wrecks reprocess to materials") are valued at what
  the outputs currently fetch:
  > "the `Crafting` cost of an item is the sum of the material prices (`matPrice`) of each individual
  > item used to craft the recipe." and "A particular items `Destroy` value is the value of the items
  > you would receive by destroying it, on average. ... By default, the `Destroy` value is calculated
  > with the `DBMarket` values of the items you would receive"
  > — https://support.tradeskillmaster.com/en_US/custom-strings/which-price-sources-can-i-use-and-what-do-they-mean
  *(Inference)*: TSM's `matPrice` is *market-price-based* imputation (backward from observed output
  prices); the Austrian version is *marginal-utility-based* imputation (backward from consumer WTP).
  They coincide once consumer WTP is the thing setting output prices.
- **Gode & Sunder's budget constraint is exactly a reservation-price rule**: ZI traders are
  constrained "not permitting traders to sell below their costs or buy above their values" — i.e.
  every agent holds a reservation price and never crosses it (see §2). That is the cheapest
  possible instantiation of subjective value in an order book.

---

## 2. ACE price formation: what makes prices converge

### 2.1 Zero-intelligence traders (Gode & Sunder 1993)

> "This paper reports market experiments in which human traders are replaced by 'zero-intelligence'
> programs that submit random bids and offers. Imposing a budget constraint (i.e., not permitting
> traders to sell below their costs or buy above their values) is sufficient to raise the allocative
> efficiency of these auctions close to 100 percent. Allocative efficiency of a double auction
> derives largely from its structure, independent of traders' motivation, intelligence, or
> learning. Adam Smith's invisible hand may be more powerful than some have thought; it can generate
> aggregate rationality not only from individual rationality but also from individual
> irrationality." — Gode & Sunder, *JPE* 101(1), 1993, abstract,
> https://ideas.repec.org/a/ucp/jpolec/v101y1993i1p119-37.html (full text mirror:
> https://faculty.sites.iastate.edu/tesfatsi/archive/tesfatsi/ZITraders.GodeSunder.JPE1993.pdf;
> DOI 10.1086/261868).

**Load-bearing for us:** the two ingredients are (a) a continuous double auction institution and
(b) *reservations never crossed* ("budget constraint"). Randomness inside reservation bounds is
enough for near-100% allocative efficiency in Smith-style supply/demand designs.

### 2.2 The counter-evidence: ZI prices are wrong when supply/demand are asymmetric

> "the average transaction prices of ZI traders can vary significantly from the theoretical
> equilibrium value when the market supply and demand are asymmetric, and ... the degree of
> difference from equilibrium is predictable from a priori probabilistic analysis. In this sense, it
> is shown here that Gode and Sunder's results are artefacts of their experimental regime." — Cliff
> & Bruten, *Zero is not enough: on the lower limit of agent intelligence for continuous double
> auction markets*, HP Labs HPL-97-141, 1997,
> https://shiftleft.com/mirrors/www.hpl.hp.com/techreports/97/HPL-97-141.pdf (same claim in Cliff's
> longer report *Minimal-Intelligence Agents for Bargaining Behaviors in Market-Based Environments*,
> http://www.sci.brooklyn.cuny.edu/~parsons/courses/840-spring-2005/notes/cliff.pdf).

*(Inference)*: convergence of *allocations* ≠ convergence of *prices* to the competitive level.
If the user wants prices that track competitive equilibrium (not just efficient trades), random
ZI placement is not enough; agents must adapt toward observed prices.

### 2.3 ZIP (Zero-Intelligence-Plus) — minimal intelligence, strong convergence

- Mechanism, from the primary report: each trader has a fixed limit price `λ_i` (its reservation
  price — cost or value) and a variable **profit margin**; the shouted price is derived from both;
  when a trader must adjust, "a 'target price' (denoted by τ_i(t)) will be calculated for each
  trader, and the Widrow-Hoff rule will then be applied to take the trader's shout-price on the
  next time-step (p_i(t+1)) closer to the target price τ_i(t)." Parameter ranges used by Cliff &
  Bruten: learning rate "uniformly distributed over [0.1, 0.5]", momentum coefficient
  "uniform distribution over [0.0, 0.1]" (HPL-97-141, §4).
- The adjustment heuristic (HPL-97-141, pseudocode): sellers raise their margin when the last
  accepted trade was at or below their shout price, lower it when outbid; buyers mirror this.
- Independent restatement:
  > "Each ZIP trader agent maintains a real-valued profit margin and employs simple heuristic
  > mechanisms to adjust their margin using market data. In this context, the profit margin
  > represents the difference between the agent's limit price and the shout price, that is the price
  > that the agent sends to the market to buy or sell the commodity." and "ZIP [was created] to
  > investigate the minimum level of intelligence required to achieve convergence to market
  > equilibrium price" — De Luca & Cliff, IJCAI 2011, §2.1,
  > https://www.ijcai.org/Proceedings/11/Papers/041.pdf
- Result: ZIP groups perform "significantly closer to the human data than is the performance of Gode
  and Sunder's ZI traders" and ZIP/GD/GDX have since been used as the benchmark CDA strategies;
  ZIP-derived agents were shown to outperform human traders in real-time CDA markets
  (HPL-97-141 abstract; De Luca & Cliff 2011, §1).
- Handy target metric from De Luca & Cliff (Eq. 1): the competitive equilibrium price
  `p* = argmax_p { Σ_i Σ_n max(0, l_i,n − p) + Σ_j Σ_m max(0, p − c_j,m) }`, with each trader's
  maximum theoretical profit `Π*_i = Σ_n max(0, l_i,n − p*)`. Use this to *measure* convergence in
  the sim.

### 2.4 What dominates price statistics: institutions and order flow

> "there may be situations where constraints imposed by market institutions dominate strategic agent
> behavior. ... The model explains 96% of the variance of the gap between the best buying and selling
> prices (the spread) and 76% of the variance of the price diffusion rate, with only one free
> parameter." and "It suggests that institutions strongly shape our behavior, so that some of the
> properties of markets may depend more on the structure of institutions than on the rationality of
> individuals." — Farmer, Patelli & Zovko, *The predictive power of zero intelligence in financial
> markets*, PNAS 102(6), 2005, https://pmc.ncbi.nlm.nih.gov/articles/PMC548562/

*(Inference)*: spread and volatility in your game will be set mainly by **order-arrival and
cancellation rates** (agent "patience"), order size, and fees — tune those knobs before adding
agent intelligence. Note their model places limit orders randomly relative to the best prices and
still gets spread/volatility laws; the open problem (their "second, harder problem") is explaining
order flow itself — that is where your agents' subjective valuations come in.

### 2.5 What produces bubbles/oscillation (to avoid)

- Strategy mix matters: order-driven market with heterogeneous agents "trading via a central order
  matching mechanism... We investigate how different trading strategies may affect the dynamics of
  price, bid-ask spreads, trading volume and volatility" — Chiarella & Iori, *Quantitative Finance*
  2 (2002), abstract https://iopscience.iop.org/article/10.1088/1469-7688/2/5/303 (snippet-level
  quote). *(Inference, consistent with the paper's widely-cited result)*: trend-extrapolating
  ("chartist") rules add volatility/autocorrelation; fundamental/reservation-anchored rules damp it.
- Bubbles arise with **dividend/asset uncertainty + inexperience**, even in human markets:
  "Fourteen of twenty-two experiments exhibit price bubbles followed by crashes relative to intrinsic
  dividend value. When traders are experienced this reduces" — Smith, Suchanek & Williams,
  *Econometrica* 56 (1988), abstract
  https://www.econometricsociety.org/publications/econometrica/1988/09/01/bubbles-crashes-and-endogenous-expectations-experimental-spot
  (snippet-level quote from the abstract; the same sentence appears in the JSTOR and RePEc records).
  *(Inference)*: no-reinvestment assets + fixed-use consumption goods + 100%-escrowed orders remove
  the classic bubble channels.
- Reinforcement learning with simple experience-weighted attraction converges to equilibrium play
  in repeated games with unique equilibria: "The learning model we studied exhibited the same kind of
  behavior in each game as the experimental subjects did" — Erev & Roth, *AER* 88(4), 1998,
  https://ideas.repec.org/a/aea/aecrev/v88y1998i4p848-81.html (snippet-level quote). *(Inference)*:
  a per-action payoff-reinforcement rule (keep margin aggressiveness that paid off) is a
  theory-backed simple learning rule alternative to Widrow-Hoff.

---

## 3. EVE Online market microstructure & economy practice

- **Order book mechanics.** "The market is essentially an automated list of item exchange contracts:
  buy orders and sell orders. Since orders can be partially filled and searched easily..."
  "**Each region has its own regional market.** Each station and Upwell structure with market service
  module has access to the market." — EVE University Wiki, *Trading*,
  https://wiki.eveuniversity.org/Trading
- **Regional spreads are arbed by haulers, not by arbitrageurs at a distance.** "buy some goods at
  one station, load them up into your hauling ship, carry them off to another station and sell them
  for profit. This works because most players don't have the ships or the time to move everything
  themselves... hauling stuff from mission hubs, where mission runners dump their loots, to trading
  hubs where there is less supply." — ibid. *(Inference)*: your hauling agents are the
  law-of-one-price engine; spread = transport cost + risk + fees, which is exactly the competitive
  margin.
- **Taxes/fees are part of the price calculus.** "A station trader with zero standings will pay
  between 5.1% (at max skills) and 11% of the order price as fees/taxes." Sales tax base 7.5%,
  broker fee base 3% (NPC stations), broker fee charged at order creation and not refunded on
  expiry (ibid.).
- **CCP explicitly engineered anti-churn mechanics** (directly relevant to convergence):
  > "One of the guiding principles for the market in EVE Online is to keep intact how closely it
  > mirrors aspects of real-world economics and financial markets."
  > "the optimal strategy becomes 'Always create your orders at 0.01 ISK above/below the current best
  > order, and always update your order ASAP...' Competition between traders comes down to who (or
  > what) can micro-manage their orders for the longest period of time, rather than who is making the
  > most intelligent pricing decisions."
  — *Broker Relations* dev blog, 2020-02-24, https://www.eveonline.com/news/view/broker-relations
  The fixes and their exact formulas (same source):
  - **Tick size:** "Order prices can only be specified with a maximum precision of 4 significant
    figures."
  - **Relist fee:** `Fee = max(0, BR * (P2 - P1)) + (1 - RD) * BR * P2` (BR = broker rate,
    RD = relist discount ≥ 50%) — modifying orders now costs real money, throttling price-pacing.
  - **100% escrow:** "all WTB orders will require 100% escrow to be paid up front" (Margin Trading's
    partial-escrow leverage was removed).
  *(Inference)*: escrow = the Gode-Sunder budget constraint implemented as a rule; relist fees =
  a price-update friction that kills 0.01-undercutting limit cycles; ticks stop infinite precision
  races. All three are *institutional* convergence levers — exactly what Farmer et al. say dominates.
- **Faucets/sinks and price indices are monitored as the economy's health metrics.** Monthly
  Economic Reports publish "ISK Sinks & Faucets" alongside "Mineral Price Index", "Ship Price
  Index", "Module Price Index", "Velocity of ISK", "Mining and Production Value" and "Destruction
  Value" — e.g. Feb 2026 MER:
  "The Mineral Price Index continues to decline... The Ship Price Index continues to decline...
  The Module Price Index continues to decline..." —
  https://forums.eveonline.com/t/monthly-economic-report-february-2026/507779 (raw CSV data linked
  from each MER post; historical compilations e.g. https://www.adam4eve.eu/mer_sinks_faucets.php).
  *(Inference)*: currency faucets/sinks must be measured separately from goods-price indices; your
  mass-conserving goods loop keeps material prices honest, but currency inflation still needs
  sinks (broker fees, taxes, repair/reprocess losses).
- **CCP's stated philosophy:** EVE's economy is "pure capitalist", "emergent, not constrained to a
  fixed state like those of most MMO games"; the market "become completely player-driven" as the
  single-server population grew — Dr. Eyjólfur Guðmundsson,
  https://www.gamedeveloper.com/game-platforms/ccp-economist-on-i-eve-online-i-s-pure-capitalist-market
  (2008). CCP staffed a PhD economist (2007–2014) and later a central-bank economist for EVE
  Frontier: https://www.gamedeveloper.com/business/ccp-hires-new-economy-head-to-legitimize-eve-frontier-s-in-game-economy.
- **Honest caveat from EVE practice — cost floors do not automatically hold:** "these markets are
  neither perfect nor efficient in the technical senses; volumes of many items are low enough that
  the market can be (and is) manipulated... there are a great number of Tech 1 items, modules
  especially, that are worth less than the cost of manufacture." — EVE University Wiki,
  *Manufacturing*, https://wiki.eveuniversity.org/Manufacturing
  *(Inference)*: thin markets break imputation discipline (producers dump below cost). Mitigations:
  minimum lot sizes, order expiry, and letting inventory-aware agents stop producing at a loss.

---

## 4. Practical game-economy heuristics with concrete formulas

- **TSM AuctionDB "Market Value" (`DBMarket`)** — a production-grade market-price estimator with
  outlier rejection (usable verbatim as an agent's "market memory"):
  1. walk the cheapest listings, discarding the top after the lowest ~30%, with a 20%-jump outlier
     rule active after the first 15% of listings;
  2. drop points > 1.5 standard deviations from the mean of survivors;
  3. average the remainder → "current market value";
  4. blend with history: "a weighted average of the previous 14 days' market values plus today's...
     recent data is very heavily weighted. This results in the data having a 'half-life' of a little
     over 2 days."
  — https://support.tradeskillmaster.com/en_US/tsm-addon-documentation/auctiondb-market-value ;
  definition: "`DBMarket` is a weighted 14-day average of the auctions listed for a particular item
  ... favoring the most recent 3 days of pricing data. **Note this value does not indicate what an
  item will sell for, only what the item is currently being listed for**"
  — https://support.tradeskillmaster.com/en_US/custom-strings/which-price-sources-can-i-use-and-what-do-they-mean
  *(Inference)*: listings ≠ transactions; your agents should prefer executed-trade VWAP where
  available (the same doc offers `DBRegionSaleAvg` = "the average sale and purchase price ... data
  is collected from TSM Accounting").
- **Cost/destroy valuation:** `Crafting = Σ matPrice`, `Destroy` = `DBMarket` of outputs (quoted in
  §1.2) — i.e., ship-implementable imputation for both production and reprocessing.
- **EVE industry cost** is computed by the game engine as
  `Manufacturing Job Cost = Estimated Item Value × System Manufacturing Cost Index × Facility Tax × Runs`
  (community documentation of the official formula,
  https://forums.eveonline.com/t/formula-to-calc-cost-to-make-one-unit-of-a-thing/16661) — note the
  "Estimated Item Value" is a *CCP-computed smoothed price*, i.e. EVE's industry tax is anchored to
  an official price index, not to player subjective value. *(Inference)*: avoid copying this if the
  goal is emergent prices; it is a developer-computed price feeding a sink.
- **Eco (Strange Loop Games)** runs a fully player-priced economy on player-issued currency and
  store objects (https://wiki.play.eco/en/Economy) — cited only as existence proof of game-side
  player price-setting; no reusable formula found (skipped deeper reading).
- Satisfactory/Anno have no player markets (fixed/NPC pricing) — no formulas to borrow; skipped.

---

## 5. Candidate price mechanisms (synthesis)

All three assume the same venue per regional market node: a **continuous double auction** with
partial fills, order expiry, tick size, and per-trade fee `f` (broker+tax). Gode-Sunder and
Farmer et al. both say the venue does most of the work.

### A. **Imputed-marginal-utility (IMU)** — the faithful Austrian mechanism *(opinion: the one the
brief demands)*

- **Agent state:** capital `C`; inventory per good; role(s); per-consumption-good ranked use
  schedule `u_g(1) > u_g(2) > …` (Menger's sacks-of-grain; simplest: `u_g(k) = U_g · δ^(k-1)`);
  recipe list with quantities (fixed-proportion/Leontief, per Menger's complementarity §1.iii.a.2);
  per-good moving stats (recent trades).
- **Consumer rule:** WTP for the next unit = `u_g(k)` converted to currency by the agent's marginal
  utility of money (simple version: `WTP = u_g(k) / λ`, `λ` fixed per agent or scaled by `C`).
  Never bid above WTP (Gode-Sunder budget constraint).
- **Producer imputation rule (the Zurechnung step):** each tick (or on any price event),
  `V(m) = max_{r ∋ m} [ (MV(out_r) − Σ_{m'≠m} q_{m'}·V(m')) / q_m ]`, iterated to fixpoint over the
  production tree, where `MV(out_r)` is the expected marginal sale value of one output unit — taken
  as min(own WTP-schedule value, market ask-implied value) for goods the agent itself consumes,
  else expected sale price net of fees and hauling. Wrecks/reprocessing contribute an additional
  floor: `V(m) ≥ Σ outputs (reprocess yield × V(output))` (TSM `Destroy` logic).
- **Reservation & order placement:** buyer reservation `R_b = V(m)` (or WTP); seller reservation
  `R_s = max(imputed input cost + fee + target margin, alternative-use value)`. Post a limit order
  at `R × (1 ± ε)`, `ε ~ U[0, 0.05]` (ZI-in-bounds: never cross own reservation). Orders expire;
  repost at reservation, adjusted by small reinforcement toward recent trade prices (Erev-Roth-style
  or Widrow-Hoff).
- **Clearing:** standard CDA; spread emerges from order-arrival rates (Farmer et al.).
- **Convergence expectations:** allocation efficiency high (Gode-Sunder); price converges toward
  the equilibrium defined by consumer WTP schedules and physical recipe constraints, because
  reservations themselves move toward marginal values. Known risk *(inference)*: fixpoint + EWMA
  lags on long chains can ring/oscillate; damp by updating `V` slowly relative to trade frequency
  and by clipping `V` updates per tick (e.g. ≤ x%/tick).
- **Complexity:** highest of the three (fixpoint over recipes + utility schedules), but each rule is
  local and simple — fits the Game-of-Life criterion at the rule level.

### B. **ZIP margin-adapters** — the strong-convergence baseline

- **Agent state:** limit price `λ` (cost for sellers from their ledger, WTP for consumers),
  profit margin `µ(t)`, learning rate `β ∈ [0.1, 0.5]`, momentum `α ∈ [0, 0.1]` (Cliff & Bruten's
  ranges).
- **Rule:** shout `p(t) = λ·(1+µ(t))` (+ small noise); on market events compute target price `τ(t)`
  from the last accepted trade/quote and move `p` toward `τ` via Widrow-Hoff with momentum
  (exact update rule & raise/lower heuristics: HPL-97-141 §4; De Luca & Cliff §2.1).
- **Clearing:** CDA; convergence measured against `p* = argmax_p {Σ max(0, l−p) + Σ max(0, p−c)}`
  (De Luca & Cliff Eq. 1).
- **Convergence:** strongest documented evidence of the three (benchmark CDA strategy; "significantly
  closer to the human data" than ZI; designed "to achieve convergence to market equilibrium price").
- **Complexity:** low. **Caveat:** `λ` must come from somewhere — cost/WTP anchors still needed for
  multi-stage production, so B composes naturally with A's imputation layer.

### C. **Cost-plus + EWMA ledger ("EVE trader bot")** — cheapest baseline

- **Agent state:** per (good, node) EWMA price `P̂` computed like TSM `DBMarket` (trimmed mean of
  recent listings + time-weighted blend, half-life ≈ 2 days), own cost basis, target margin `m`.
- **Rule:** ask `= max(P̂·(1+m+f), cost basis·(1+m))`; bid `= min(P̂·(1−m), imputed value via
  Σ matPrice × P̂ of outputs)`; hauled when `P̂_dest − P̂_source > transport + fees + margin`.
- **Convergence:** fast to a stable band, but prices are self-referential (P̂ tracks P̂); without
  consumer WTP caps the system has no fundamental anchor and can drift with currency supply.
  **Complexity:** lowest. Use as the control condition in A/B tests of the simulation.

### Comparison table — recommended operationalization *(opinion)*

| | **A. IMU (imputed marginal utility)** | **B. ZIP margin-adapters** | **C. Cost-plus + EWMA** |
|---|---|---|---|
| Theory | Menger §3.ii.b.35 marginal valuation; §3.iii.a.3 imputation; Wieser marginal law & law of costs | Cliff & Bruten 1997 (Widrow-Hoff margin learning) | EVE/TSM practice (cost-plus, `DBMarket`) |
| Reservation price | Fixpoint `V(m)` from consumer MU schedules (never crossed) | `λ` = cost/WTP (never crossed) | cost basis + margin (soft) |
| Price driver | consumer WTP ⇒ imputed input values (backward through chain) | observed market prices (adaptation) | observed market prices (memory) |
| Order placement | limit at reservation ± ε, expiry, slow adjustment | shout `λ(1+µ)`, µ adapts to events | ask/bid around P̂ with margin |
| Clearing | CDA per regional node, fees, ticks, 100% escrow | same | same |
| Convergence evidence | Gode-Sunder (efficiency); anchor is definitional | strongest (benchmark CDA strategy) | fast but no anchor (self-referential) |
| Bubble/oscillation risk | low if `V` updates damped; lags can ring on long chains | low–medium (momentum term) | medium (P̂ echo chambers) |
| Complexity | medium–high (recipe fixpoint + MU schedules) | low | lowest |
| Faithful to brief's Austrian requirement | **yes** | partial (reservation prices only) | no |
| Recommended role *(opinion)* | **core valuation layer** | **order-placement layer on top of A's reservations** | **agent memory estimator (P̂) + baseline test** |

**Recommended stack *(opinion)*:** use A to compute every agent's reservation prices (this is where
subjective value and imputation live), use B's margin/Widrow-Hoff dynamics to place and revise
orders around those reservations, use C's `DBMarket`-style trimmed EWMA as each agent's market
memory. Institutional levers (all sourced): 100% escrow, order expiry, tick size, relist fee
proportional to new price (EVE formulas), per-trade fee as the currency sink. Measure convergence
with De Luca & Cliff's `p*` and allocative efficiency; measure spread/volatility against Farmer et
al.'s order-flow laws as a sanity check that the venue, not agent brains, is driving microstructure.

**Convergence levers checklist (all evidence-backed):** budget/escrow constraints (Gode & Sunder);
avoid asymmetric thin markets or accept deviation from `p*` (Cliff & Bruten); price-update friction
+ ticks (CCP dev blog); no credit/leverage and consumption-only goods to remove speculative demand
(Smith-Suchanek-Williams bubble conditions); keep trend-following rules out or heavily damped
(Chiarella & Iori); tune order-arrival/cancellation rates ("patience") before agent intelligence
(Farmer et al.).

---

## Source list

1. Menger, *Principles of Economics* (1871), Ch. 3 (The Theory of Value) — https://mx.nthu.edu.tw/~cshwang/all-my-teaching/political-economics/PE03-Subjectivism/03-3-Menger%20on%20Goods%20and%20Money/Menger=ch3%20The%20Theory%20of%20Value.htm
2. Menger, *Principles*, Ch. 1 (General Theory of the Good) — https://mx.nthu.edu.tw/~cshwang/all-my-teaching/political-economics/PE03-Subjectivism/03-3-Menger%20on%20Goods%20and%20Money/Menger=ch1%20The%20General%20Theory%20of%20the%20Good.htm
3. Menger, "The Value of Goods of Higher Order" (Mises Daily excerpt of *Principles*) — https://mises.org/mises-daily/value-goods-higher-order
4. Wieser, *Natural Value* (1893, trans. Malloch), analytical contents — https://praxeology.net/FW-NV.htm
5. Gode & Sunder (1993), *Allocative Efficiency of Markets with Zero-Intelligence Traders*, JPE 101(1):119–137 — https://ideas.repec.org/a/ucp/jpolec/v101y1993i1p119-37.html (PDF mirror: https://faculty.sites.iastate.edu/tesfatsi/archive/tesfatsi/ZITraders.GodeSunder.JPE1993.pdf)
6. Cliff & Bruten (1997), *Zero is not enough…*, HP Labs HPL-97-141 — https://shiftleft.com/mirrors/www.hpl.hp.com/techreports/97/HPL-97-141.pdf
7. Cliff (1997), *Minimal-Intelligence Agents for Bargaining Behaviors in Market-Based Environments* — http://www.sci.brooklyn.cuny.edu/~parsons/courses/840-spring-2005/notes/cliff.pdf
8. De Luca & Cliff (2011), *Human-Agent Auction Interactions*, IJCAI — https://www.ijcai.org/Proceedings/11/Papers/041.pdf
9. Farmer, Patelli & Zovko (2005), *The predictive power of zero intelligence in financial markets*, PNAS 102(6):2254–2259 — https://pmc.ncbi.nlm.nih.gov/articles/PMC548562/
10. Chiarella & Iori (2002), *A simulation analysis of the microstructure of double auction markets*, Quantitative Finance 2:346–353 — https://iopscience.iop.org/article/10.1088/1469-7688/2/5/303
11. Smith, Suchanek & Williams (1988), *Bubbles, Crashes, and Endogenous Expectations in Experimental Spot Asset Markets*, Econometrica 56:1119–1151 — https://www.econometricsociety.org/publications/econometrica/1988/09/01/bubbles-crashes-and-endogenous-expectations-experimental-spot
12. Erev & Roth (1998), *Predicting How People Play Games*, AER 88(4) — https://ideas.repec.org/a/aea/aecrev/v88y1998i4p848-81.html
13. Epstein & Axtell (1996), *Growing Artificial Societies* (Sugarscape trade rules) — https://complexityexplorer.s3.amazonaws.com/Mesa+ABM/epstein_axtell.pdf ; operational summary https://pressbooks.pub/intro2abm/chapter/sugarscape/
14. TradeSkillMaster docs: AuctionDB Market Value — https://support.tradeskillmaster.com/en_US/tsm-addon-documentation/auctiondb-market-value ; Price Sources — https://support.tradeskillmaster.com/en_US/custom-strings/which-price-sources-can-i-use-and-what-do-they-mean
15. EVE University Wiki: Trading — https://wiki.eveuniversity.org/Trading ; Manufacturing — https://wiki.eveuniversity.org/Manufacturing
16. CCP dev blog *Broker Relations* (2020) — https://www.eveonline.com/news/view/broker-relations
17. EVE Monthly Economic Report (Feb 2026) — https://forums.eveonline.com/t/monthly-economic-report-february-2026/507779
18. Interview with Dr. Eyjólfur Guðmundsson (2008) — https://www.gamedeveloper.com/game-platforms/ccp-economist-on-i-eve-online-i-s-pure-capitalist-market
19. Eco wiki, Economy — https://wiki.play.eco/en/Economy
