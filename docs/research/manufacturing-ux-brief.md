# Research Brief: Manufacturing Window UX, Category Browse and Rich Input Visualisation

**Question:** How do games present crafting/industry *browsing* (category tree vs search vs grid), how do
they *visualise a recipe's inputs and outputs*, what does the UX literature say about recipe information
design, and when do graphs help versus clutter? Every pattern is weighed against Entieve's house idioms
(rich rows, section bars, split panes, palette-only, deterministic, frozen icon manifest) so the owner can
pick one of four candidate directions before any code is written.

**Scope decisions taken (assumptions):** the brief names the files, so all repo paths below were read
directly and are cited as `path:line` (read-only; the single exception is creating this file). The brief's
five deliverable parts ((a) problem statement, (b) pattern inventory, (c) 3-4 ASCII design directions,
(d) icon appendix, (e) sources) are the skeleton of this document. External claims are quoted with URLs,
retrieved 2026-11-03 through the session's search/fetch tooling plus `curl` for two pages whose
extractors dropped words; pages that returned 403/Cloudflare are labelled snippet-only and quoted from
the search index rather than the page. Weak sources (community wikis, forums, fan calculators) are
labelled at the point of use. `docs/research/skills-tech-trees-brief.md` sets the tone: quotes first,
inference marked, no unsourced numbers.

**Fact vs. inference:** sections 1 and 2 are facts read out of this repository (file paths, quoted code
comments). Section 3's quoted lines come from the cited URLs (official studio pages, official game wikis,
standards documents and peer-reviewed papers, each labelled). Section 4, every "fit verdict", and any
sentence marked *(inference)* or *(opinion)* are my synthesis, not a source's claim.

---

## 1. Problem statement: what the Inputs column actually says today

`uikit/screens/manufacturing.go` builds a five-column recipe board:

```go
recipeCols := []widgets.Column{
    {Key: "Product", Title: "Product", Priority: 0},
    {Key: "Family", Title: "Family", Priority: 1},
    {Key: "Time", Title: "Time", Icon: "hud_time", Priority: 2, AlignRight: true},
    {Key: "Fee", Title: "Fee", Icon: "stat_cash", Priority: 3, AlignRight: true},
    {Key: "Inputs", Title: "Inputs", Priority: 4},
}
```

and the Inputs cell is one string, built by `inputsText` (`uikit/screens/manufacturing.go:337`):

```go
// inputsText renders the have/need line of a job's materials: one
// "Name have/need" pair per input, joined by the table's ellipsis rules.
// The board and the picker's preview share it.
func inputsText(cv widgets.ContentView, mats []content.CargoStack) string {
	...
	out += name + " " + theme.FormatInt(int64(cv.PlayerStock(m.Item))) +
		"/" + theme.FormatInt(int64(m.Qty))
	...
	return out
}
```

Rendered, the heaviest recipe in the shipped content (`rec-mod-artillery-l`, five inputs) reads:

```
Sideron 12,400/18,490 · Nixium 1,620/1,620 · Zaffrite 300/1,030 · Vetril 0/670 · Helion 0/390
```

Facts about that cell, all read from the tree:

- **It is the first column to die.** `Column.Priority` is documented as "shrink order when over space
  (higher drops first)" (`uikit/widgets/table.go:52`), and Inputs carries `Priority: 4`, the highest on
  the board. Under a narrow window the thing the owner calls "way too poor" is also the thing the layout
  throws away first.
- **It is text-only, no icons.** The row's icon array is
  `Icons: []string{widgets.TradeableIcon(...), "", "", "", ""}` (`manufacturing.go:252`): the product gets
  a glyph, the five minerals get none, although every mineral has art (appendix, §5).
- **It is one flat line per row, ellipsized by the table's rules** (`inputsText` comment, "joined by the
  table's ellipsis rules"). Two to five materials at very different magnitudes
  (`18,490` vs `390`) must be compared *across rows* as strings. There is no proportion, no state colour,
  no "can I run this now" signal: the player parses numbers to find the blocker. *(inference)*
- **It is doing two jobs.** The same function feeds the quantity picker's preview:
  `case widgets.QtyStart: return inputsText(...)` (`uikit/screens/qtypicker.go:362`), so any replacement
  is a shared component, not a column tweak.
- **The data behind it is already live and deterministic.** The board quotes one run
  (`runs := 1`, `manufacturing.go:237`) through `s.JobMaterials(station, rec.ID, runs)`, which is the
  sim's craft-discounted material read ("each level cutting job time 4 % and materials 2 % below the L1
  baseline", `docs/SIM.md` §3.3), and `cv.PlayerStock(m.Item)` for the have side. Nothing needs to change
  in the sim to make this visual. *(fact)*
- **Content scale (from `content/data/recipes/*.json`, counted):** 54 recipes total, families `modules` 18,
  `charges` 25, `hulls` 11; inputs per recipe are 2 (16), 3 (27), 4 (6), 5 (5); the entire economy runs on
  **six** minerals: `min-ferrite` Sideron (6 ISK, used by 54 recipes), `min-nickel` Nixium (14, 31),
  `min-cobalt` Zaffrite (22, 30), `min-silex` Vetril (34, 23), `min-argon` Helion (58, 21),
  `min-glacite` Cryolith (46, 3). *(fact)* The whole input space fits in one row of six icon chips, which
  is what makes a rich visual treatment tractable here and would not be in a game with 200 reagents.
  *(inference)*
- **The window's furniture:** header artwork band (`render.ManufacturingHeader`), a craft readout line
  ("Craft: hulls L1 · modules L2 · charges L1 · time −4%/lvl · mats −2%/lvl"), a job table under a
  draggable seam named `manufacturing-jobs`, landing rect `image.Rect(312, 120, 792, 600)` (480×480
  design units) with `MinSize(420, 440)` (`manufacturing.go:141-180`).

## 2. The "similar view" that already exists: the market window

The owner points at `uikit/screens/market.go` as the target idiom. What it actually ships, from its own
doc comment (`market.go:73-103`) and `CONTEXT.md`:

- **A browse tree built from content's own category paths.** "The browse tree scopes the catalog to a
  branch (All is the whole catalog) and the search field filters that branch live, ranked by fuzzy score
  with the matched letters highlighted. Selecting an item highlights its branch back." `mktNodes` flattens
  `Ships/Frigates`, `Modules/High/Turrets`, `Materials/Ore`, ... into depth-first nodes with indent and
  `→ / ↓` expansion markers (`market.go:265-370`).
- **Live search inside the branch**, `sahilm/fuzzy` ranked, diacritic-folded (`fold.go`), matches split
  into accent-tinted runs (`mktNameSegs`, `market.go:1098`).
- **Rich rows and dim-but-live rows.** Search hits render as `Stack` rows with a dim trailing meta run;
  an item with no orders "still gets its row, dimmed, but live"; the `· N more` availability suffix rides
  `CellRuns` (`market.go:1061-1096`).
- **Three panes with two persisted seams**, `NewSplitPaneRow`, opening shares written into layout
  constants `mktTreeW = 176`, `mktDefaultW/H = 760/560` (`market.go:29-48`).
- `docs/GAMEPLAY.md` §The market window: "The tree is the content's own category path: `All` at the root,
  then `Ships`, `Modules`, `Ammunition`, `Materials` and `Loot` with their children ... Clicking a branch
  expands it and scopes the catalog to its subtree."

**The recipes already have that category path.** Every one of the 54 recipe products carries a `group`
(counted from content): `Ships/Frigates` 2, `Ships/Destroyers` 2, `Ships/Industrials` 3, `Ships/Barges` 1,
`Ships/Rat hulls` 3, `Modules/High/Turrets` 7, `Modules/High/Launchers` 1, `Modules/Mid/Shield` 2,
`Modules/Mid/EWAR` 2, `Modules/Mid/Capacitor` 1, `Modules/Mid/Propulsion` 1, `Modules/Low/Armor` 3,
`Modules/Low/Capacitor` 1, `Ammunition/Autocannon rounds` 12, `Ammunition/Artillery shells` 12,
`Ammunition/Cap cells` 1. So the owner's "search by categories of items" can reuse `mktNodes` /
`scopeItems` logic verbatim over `RecipesInOrder()`, with the product's group as the tree, or the
`family` field (`hulls`/`modules`/`charges`) as a two-level alternative. *(inference)*

## 3. Pattern inventory

Each entry: the pattern, a quote from a source, what it solves, then a one-line fit verdict
*(opinion)*. Sources are numbered in §7.

### A. Browsing and finding

**A1. Category tree plus branch-scoped live search (the market idiom).** EVE's market is the ancestor of
Entieve's tree, and players are taught to work it by category: "Click on a market category, and start
looking at the orders and market details for each item. Switch to another category, and keep looking at
each item." [S10, EVE University wiki, community] The in-repo version is §2 above.
*Solves:* 54+ rows become a navigable hierarchy, with search as the shortcut for people who know the name.
*Fit:* **fits directly**, `mktNodes`/`scopeItems`/`searchHits` are already generic over "id → group path";
one new tree pane and one seam named for the layout sidecar.

**A2. Keep the list as the navigation substrate; visualise, but do not replace, the list.** CCP's own
writeup of the industry redesign is explicit: "Of course not all things are better represented using
visualizations (there is a reason why people prefer spreadsheets for some things). Thus core navigation
for things like selecting blueprints or installations will remain in a list view, because that is the
most efficient way to navigate, compare, and finally select the one you want." [S1, primary]
*Solves:* the fear that "rich visual language" means throwing away the sortable table.
*Fit:* **keep `widgets.Table` as the board**, put the visual language into rows and a detail area.

**A3. Browse grids in codex-style windows (conflicting evidence, flagged).** A studio surveying its
competitors' codex screens found the opposite bias: "Almost all of them used a two-panel design where one
part of the screen let you select an object and the rest of the screen showed information about the
object. No Man's Sky and Deep Rock Galactic used lists to browse the objects, but the majority (and the
ones I liked most) all used a grid format where each cell showed some kind of image or icon for the
object. This grid format makes it super easy to pick out the object you want and quickly navigate to it."
[S5, Automation Station devlog #27, primary studio dev blog]
*Solves:* icon grids beat lists when identity is visual (you recognise a hull, you do not read its name).
*Fit:* **conflict to resolve, not to paper over.** EVE says list for navigation, Automation Station says
grid for browsing. Entieve has 54 recipes, six of which are hulls with real portraits, so *(opinion)* a
list with a 20 px product icon (today's `mktRowIcon`) is the safer default, with the grid option held in
reserve for a "hulls only" branch.

**A4. Search by ingredient, not only by product.** TradeSkillMaster shipped this and documented it:
"Searching the Crafting UI by a material now also returns all crafts that uses that particular material.
For example searching for 'windwool cloth' in your Tailoring profession will no longer simply return
'Bolt of Windwool Cloth' but also any crafts that require that item." [S6, official addon blog]
*Solves:* the real question is often "what can I do with the Sideron I just mined", the inverse lookup.
*Fit:* **cheap and high value**: the search haystack already covers name, path and maker
(`searchHits`); adding the product's input item ids covers the whole recipe space, and
`recipesUsing` (`infowin.go`) proves the index exists in the UI layer already.

**A5. Expandable rows for rates, sections for kinds.** Satisfactory's codex groups recipes and expands
them: "It has three sections, Parts, Equipment, and Buildings ... All recipes can now be expanded to show
the per-minute stats of all ingredients and products. You can also simulate overclocking in there now."
[S4, official wiki]
*Solves:* dense default, detail on demand.
*Fit:* Entieve's equivalent of "per-minute stats" is "per run at this station's craft level"; the picker
already re-prices at N runs, so the expansion is the picker itself plus a row-level fold. *(opinion)*

**A6. Progressive disclosure as the governing rule.** "Progressive disclosure defers advanced or rarely
used features to a secondary screen, making applications easier to learn and less error-prone ...
Initially, show users only a few of the most important options. Offer a larger set of specialized options
upon request." The same article warns: "designs that go beyond 2 disclosure levels typically have low
usability because users often get lost when moving between the levels." [S11, NN/g, primary UX
literature]
*Solves:* how much of the recipe goes in the row versus the detail pane.
*Fit:* **two levels, no more**: level 1 = board row (state + share strip), level 2 = inspector/quantity
picker (full material table). A third level (a modal graph) would break NN/g's own caution. *(inference)*

**A7. Filters where lists get long (jobs).** "Each tab will have filters available to help those that
have massive jobs lists." [S1, CCP]
*Solves:* the global job list is every station's player jobs, unfiltered today.
*Fit:* optional; three global slots make it low priority now. *(opinion)*

### B. Visualising inputs and outputs

**B1. Tri-state material colouring (have / craftable / missing).** The official Factorio wiki documents
the pattern players learn by heart: "If the image of the item is grayed out, ensure you have the
materials necessary to craft the object. If you lack a resource, it will be shown highlighted in red text
if you mouse-over the product. Materials that you don't have but you can craft will be highlighted in
orange text, and will be chain-crafted to reach the product you selected." [S2, primary]
*Solves:* state becomes pre-attentive; the player does not subtract numbers.
*Fit:* **three palette roles already exist** (`Good #5fcf80`, `Warn #e0a33c`, `Danger #e05555`,
`uikit/theme/theme.go:52-63`); Entieve has no chain-crafting, so the orange tier maps to "refinable ore I
hold" (the two-tier chain, `docs/GAMEPLAY.md`) or collapses to two states. *(inference)*

**B2. Ready / Not Ready state icons on the material path.** "The input requirements area will have clear
distinctions between items that are missing and those where the quota has been met ... Those connection
lines will also have state icons for Ready and Not Ready states and the lines themselves will also have
clear indicators of the ready or not ready state of everything beneath it. There is one indicator icon
for each group and a single larger one that represents all groups." [S1, primary]
*Solves:* per-material state plus one aggregate verdict, so a glance answers "can I start".
*Fit:* one aggregate chip per row (and one in the inspector) is the single most valuable pixel here;
Entieve already gates `Start` through `CanStartJob`, so the state is computable without new sim logic.
*(inference)*

**B3. Values, colour and bars together, never a bar alone.** "For ME (Material Efficiency) and TE (Time
Efficiency) visualization we will have two specific boxes ... We use values, color and bars to represent
the values to make them easily readable at a glance. Positive values will have a bar growing from left to
right and negatives will go from right to left ... Since we display the values with + or - as well as
color it should still be very clear." [S1, primary]
*Solves:* number + hue + length is redundant encoding (see C3), and CCP deliberately avoided centred
split bars to save horizontal room.
*Fit:* **the primitive exists**: `widgets.RowSeg{Text, Tint, Icon, Bar, HasBar}` renders "a progress
strip instead of a text run ... the state readouts' meter" (`uikit/widgets/table.go:138-150`), and the
contacts board already uses it for the standing gate
(`uikit/screens/contacts.go:556`). A have/need bar needs no new widget. *(fact)*

**B4. Do not hide materials behind hover.** TSM grew its material list specifically for readability:
"more materials are displayed for a much clearer overview of what a recipe required without further
interaction", and reagents show "at-a-glance pricing info and inventory quantity ... helps you decide
what to use". [S6, official addon blog]
*Solves:* the tooltip-only recipe, which fails recognition-over-recall (C2).
*Fit:* keep at least the material identities visible on the row; put quantities in the bars. *(opinion)*

**B5. Aggregate "total raw" instead of a graph when the question is a sum.** Factorio's changelog shows
the same instinct in the base game: "0.2.10: Tool tip in technology preview now shows total raw" and
"0.2.8: Intermediate items in the crafting queue now show in a different color". [S2, primary wiki,
History section]
*Solves:* "what does one cost me overall" is a number, not a plot.
*Fit:* Entieve can print total material value (the fee is already "3% of a recipe's mineral value",
`CONTEXT.md`) as a one-line cost read next to the fee. *(opinion)*

**B6. Stacked proportion strip (material share by value or volume).** No game source claims this pattern
for me; it is the standard "one bar, one segment per component" form used for part-share readouts, and it
is what B3's "values, color and bars" degenerates to with five inputs. *(inference, offered as a design
candidate in §4-D3)* It answers "which mineral dominates this recipe" instantly, which the current text
cell cannot.

**B7. Flow / connection-line diagrams for the chain.** EVE's "connection lines" (B2) are the shipped
example of a node-link material path inside a production window. Entieve's chain is two-tier by design:
"Ore is not a manufacturing input: it refines into minerals at a refinery first; the two-tier chain."
[S12, `docs/GAMEPLAY.md`]
*Solves:* where does this thing come from, upstream one hop.
*Fit:* a two-node-per-input flow (ore → mineral → product) is shallow enough to read; deeper graphs are
risky (see D1/D2).

**B8. Sankey-style cost breakdowns: usable, but clutter fast.** Peer-reviewed work exists specifically
because flow diagrams degrade with volume: "We propose a method to reduce visual clutter in Sankey
diagrams when the data volume is large" [S17, *Computers & Graphics*, abstract, paywalled beyond the
abstract]. An HCI comparison paper notes "Visual clutter from crossing links can be a problem for both
diagram types." [S18, ACM, abstract visible in search index only, snippet-level]. A separate study reports
that "the Sankey diagram may begin to lose its readability as the number of nodes increases"
[S19, academic PDF, secondary].
*Solves:* nothing Entieve needs at 5-6 inputs, but it bounds the ambition.
*Fit:* **not in the row, and not as a window-wide graph**; at most a per-selected-recipe mini-flow with
2 tiers. *(opinion, supported by D1's own caution)*

### C. What the UX literature says about recipe information design

**C1. Progressive disclosure:** see A6 (NN/g [S11]). Applied here: row = state, inspector = detail,
picker = quantity maths.

**C2. Recognition rather than recall, and minimalist rows.** "Minimize the user's memory load by making
elements, actions, and options visible. The user should not have to remember information from one part of
the interface to another." and "Interfaces should not contain information that is irrelevant or rarely
needed." [S12, NN/g 10 heuristics]
*Solves:* the current cell forces the player to hold "which mineral was short" in memory while scanning;
the fix is to make state visible in the row itself.
*Fit:* every number the player needs to decide Start/Not stays in the row; deep detail moves to level 2.
*(opinion)*

**C3. Colour is never the only channel.** W3C WCAG SC 1.4.1 (Level A): "Color is not used as the only
visual means of conveying information, indicating an action, prompting a response, or distinguishing a
visual element", with the plain restatement "Use information in addition to color, such as shape or text,
to convey meaning." [S13, primary standard]
*Solves:* colour-blind players and the house's own "greyed, never hidden" convention.
*Fit:* each material state gets colour **plus** a number **plus** bar length (B3); the Ready chip gets a
glyph, not just green. This is free because `RowSeg` carries Text, Tint and Icon together. *(fact)*

**C4. Game-specific usability heuristics.** The PLAY heuristics (validated with 54 gamers rating high- and
low-ranked titles on 116 candidate heuristics) include, verbatim from the paper's Table 1 (OCR in the
hosted PDF is rough; wording preserved as printed): "B2. Status score Indicators are seamless, obvious,
available and do not interfere with game play", "E1. The game does not put an unnecessary burden on the
player", "F1. Screen layout is efficient, integrated, and visually pleasing", "F2. The player experiences
the user interface as consistent (in controller, color, typographic, dialogue and user interface
design)", "G1. Navigation is consistent, logical and minimalist." [S14, Desurvire & Wiberg, primary
academic]
*Solves:* the vocabulary to defend the constraints: no new colour families, no second navigation model
beside the market tree, no extra clicks to reach Start.
*Fit:* consistency with the market tree and contacts rows *is* the heuristic. *(opinion)*

**C5. Glanceability is the stated goal of the reference implementation.** CCP: "we wanted information to
be represented in a graphical manner, making it readable at a glance", and prototypes were judged so "that
information would be readable at a glance and always give the important information while making sure the
screen would not become too cluttered." [S1]
*Solves:* the acceptance test for any candidate in §4: does the board answer "which one can I build now"
without reading numbers? *(opinion)*

### D. Ratios and graphs in game economy UIs: when they help, when they clutter

**D1. Graphs belong to rates over time, in their own screen, and they need filters.** The official
Factorio wiki: "The production screen is a window that shows details about the rate at which your factory
produces and consumes items. It also includes graphs to show how this information has changed over time."
And its failure mode, documented by the developers: "If some items are produced at a significantly higher
rate than others (this often happens with iron and copper plates), the graphs get compressed vertically so
they can fit in the space. This makes it very hard to see how production has changed over time for other
items. To alleviate the issue, the player can filter which items are to be shown on the graph by clicking
the icons of the resources." [S3, primary]
*Solves:* the crisp rule for Entieve: a line chart of what? The sim has no time series the player needs in
this window, and one dominant axis (ferrite, every recipe) would compress the rest exactly as Factorio
describes.
*Fit:* **no line charts in the manufacturing window now.** Ratio/share bars (B3/B6) are immune to that
compression because each material gets its own bar. *(inference)*

**D2. CCP's caution is the same rule, from the other direction:** "not all things are better represented
using visualizations (there is a reason why people prefer spreadsheets for some things)"
[S1, primary]. The window still grew: "we had to make the default window for Industry larger than most
other windows, but the benefits definitely outweigh the downsides" [S1]. *(inference)* If Entieve adopts
§4-D2 or §4-D4, the landing rect (`ManufacturingRect`, 480×480) will need to widen the way the market
already did (`mktDefaultW/H = 760/560`); the precedent for enlarging a window in this codebase is the
market, and the precedent for *justifying* it is CCP.

**D3. Money-ratio views live in third-party tools today (weak sources, labelled).** For EVE, profit-per-hour
and margin charts are fan calculators, e.g. `eve-industry.org` ("Blueprint Calculator"), `fuzzwork.co.uk/blueprint/`
("Profit isk/hr (Normalized to 24 hours)"), and `adam4eve.eu` ("ISK per Hour: Max. Profit at current
market price"). [S20-S22, community tools, snippets and landing pages only] *(inference)* players want the
ratio, and games tend to omit it; Entieve already computes the pieces (material value, fee = 3% of it,
`JobFee`), so a share strip can carry cost structure without inventing an economy model.

## 4. Candidate design directions *(opinion; for the owner to choose)*

All four keep: the quantity picker as the runs gesture; visible-but-grayed gating; `Splits()`/`Tables()`
sidecar persistence for any new pane or column; palette-only tints; no per-frame randomness (every value
comes from `Sync` reads of sim state); only existing art (§5). Names below are placeholders, not
CONTEXT.md vocabulary: per `CONTEXT.md`, new concepts get named before they get built.

### D1. Market-mirror board (the literal ask, smallest change)

Reuse the market's tree + search verbatim over recipe product groups; keep the five-column table; replace
the Inputs *string* with a rich-row line of mineral chips and micro-bars.

```
┌ Manufacturing ──────────────────────────────────────────────── ─ ─ ┐
│ Craft: hulls L1 · modules L2 · charges L1 · time −4%/lvl · mats −2% │
├──────────────┬─────────────────────────────────────────────────────┤
│ [search.....]│ Product          Family  Time   Fee      Inputs     │
│ All        ↓ │ ▣ Artillery  L    modules 240 s  1,240 €  ● ● ▮▮▯  │
│  Ammunition ↓│   cannon                                       Ready│
│   Autocannon │ ▣ EMP charge L    charges  30 s    118 €  ● ▮▯     │
│   Artillery  │ ▣ Ion 40mm  L    charges  30 s     96 €  ● ▮▮▮    │
│  Modules   ↓ │ ▣ Barge          hulls  1,800 s 4,600 €  ● ● ▮▯  no│
│   High/Turrets│  ...                                              │
│  Ships       │                                                    │
├──────────────┴─────────────────────────────────────────────────────┤
│ Jobs: 2/3 slots                                                   │
│ ▣ Artillery L · Hauvnok · 1 · 62% · Running   [Collect] [Cancel]   │
└────────────────────────────────────────────────────────────────────┘
  Inputs cell = per-mineral 16px art chips (trailing dim qty) + a 6px
  have/need strip per mineral, colour Good/Dim + bar length (no state
  colour alone). Ready/Not glyph at the cell's right.
```

*Patterns combined:* A1 + A2 + A4 (tree, list substrate, ingredient search) with B1 + B2 + B3 (tri-state,
ready chip, value-colour-bar).
*What it solves:* category browse, no string parsing, state at a glance, keeps sort/column prefs.
*Fit/cost:* cheapest of the four; rich rows already carry `RowSeg.Icon` and `HasBar`. Risk: five micro-bars
in a column is busy at 420 px width, and the column still drops first under pressure (lower its
`Priority`). *(opinion)*

### D2. Browse + inspector (two-panel, progressive disclosure)

Tree and search left; a *narrow* recipe list in the middle (product, time, fee, ready chip); the selected
recipe gets an inspector card with the full material table. Jobs move under the inspector or stay in the
bottom seam.

```
┌ Manufacturing ─────────────────────────────────────────────────────┐
│ [search recipes or minerals....]          Craft: hulls L1 · ...    │
├───────────────┬──────────────────┬──────────────────────────────────┤
│ All         ↓ │ RECIPES          │  ▣ Artillery cannon launcher     │
│  Ammunition   │ ▣ Artillery  L ● │  runs 1 · 240 s · fee 1,240 €   │
│  Modules      │ ▣ EMP charge L ● │  craft L1 (−2% mats, −4% time)  │
│  Ships        │ ▣ Ion 40mm  L ●  │──────────────────────────────────│
│               │ ▣ Barge      ○  │  MATERIALS (1 run)      Ready     │
│  by mineral:  │ ▣ Frigate    ○  │  ⛁ Sideron  12,400   ▮▮▮▮▮▮▮▯  │
│   ▦ Ferrite   │  ...             │  ⛁ Nixium    1,620   ▮▮▮▮▮▮▮▮  │
│   ▦ Nickel    │                  │  ⛁ Zaffrite    300   ▮▮▯▯▯▯▯▯  │
│   ▦ Cobalt ...│                  │  ⛁ Vetril        0   ▯▯▯▯▯▯▯▯  │
│               │                  │  ⛁ Helion        0   ▯▯▯▯▯▯▯▯  │
│               │                  │  [ Start job ]  (grayed: Vetril)│
├───────────────┴──────────────────┴──────────────────────────────────┤
│ Jobs: 2/3 slots   ▣ Artillery L · Hauvnok · 62% · [Collect][Cancel] │
└────────────────────────────────────────────────────────────────────┘
```

*Patterns combined:* A1 + A4 + A6 (tree, ingredient search, two-level disclosure), A5 (sections), B2 +
B3 + B4 (ready verdict, bars, materials never hidden), C2 (recognition).
*What it solves:* the Inputs column stops being a column; each material gets a legible have/need bar and
a name, and the "greyed with reason" gate can say *which* mineral is short.
*Fit/cost:* strongest reading of the literature and closest to EVE's window framing [S1]; needs a wider
landing rect (precedent D2 note above) and an inspector widget, but no new table mechanics. Section
headings reuse `sectionBar` / `theme.SectionBar` (`infowin.go:1115`). *(opinion: my recommended default)*

### D3. Value-share strip + craftable-first sorting (the "graph" that stays readable)

Each board row becomes a rich row: line 1 = product, time, fee, ready chip; line 2 = **one** 100%-wide
stacked strip, one segment per mineral, width = that mineral's share of the run's material value
(`base_price × qty`, the same number the fee is derived from), tint = have/need state, with a six-chip
legend pinned above the board.

```
┌ Manufacturing ─────────────────────────────────────────────────────┐
│ [search...]  legend: ▦Sideron ▦Nixium ▦Zaffrite ▦Vetril ▦Cryolith ▦Helion │
│ sort: Ready ▾ | Product ▾                     Jobs: 2/3 slots      │
├────────────────────────────────────────────────────────────────────┤
│ ▣ Artillery L   240 s   1,240 €   ████▓▓░░░░░░░░░░  68% ● READY    │
│ ▣ EMP charge L  30 s      118 €   ████████████░░░░  91% ● READY    │
│ ▣ Barge       1,800 s   4,600 €   ████▓▓▓░░░░░░░░░  41% ○ short 2 │
│ ▣ Frigate       900 s   2,100 €   ████████░░░░░░░░  55% ● READY    │
├────────────────────────────────────────────────────────────────────┤
│ Jobs: ▣ Artillery L · Hauvnok · 1 · 62% · Running  [Collect][Cancel]│
└────────────────────────────────────────────────────────────────────┘
  short 2 = "two materials short" (number + glyph, WCAG 1.4.1)
  bars are state-tinted segments; percentages are have/need by value
```

*Patterns combined:* A1 (tree, can still sit left), A2 + B5 (list substrate, aggregate sums), B3 (values
+ colour + bars), B6 (stacked share), C3 (redundant encoding), D3 (money ratio shown, not implied).
*What it solves:* "which recipe can I run right now" and "which one am I actually paying for" become one
glance; a single bar per row is far harder to clutter than a chart (D1).
*Fit/cost:* uses only `RowSeg` + `PaintBar` plus a segment-width splitter; needs a written rule for
segment ordering (canonical mineral id order, like `refineOutputs`) so rows never shuffle. Deterministic
by construction. *(opinion)*

### D4. Two-tier flow card (the ambitious one)

The selected recipe expands into a small flow diagram drawn in the window's lower pane or inside the
inspector: ore → mineral → product, with EVE-style connector state icons, plus a reverse "used in" edge
to `Show Info`'s existing chain.

```
┌ Manufacturing ─────────────────────────────────────────────────────┐
│ [search...]  tree │  RECIPES (list, as D1/D2)          Jobs 2/3    │
├────────────────────┴───────────────────────────────────────────────┤
│ SELECTED: ▣ Artillery cannon launcher   240 s · fee 1,240 € · L1   │
│                                                               ●   │
│  ore-sterngrit ─┐                                                   │
│  ore-cordite  ──┼─▶ ⛁ Sideron 12,400 ─┐                            │
│                 │  ⛁ Nixium   1,620 ──┼─▶ ▣ Artillery L  → hangar  │
│  (refine:       │  ⛁ Zaffrite   300 ──┤      ● Ready               │
│   furnace ⚙)    │  ⛁ Vetril       0 ──┘      ┌──────────────┐      │
│                 │  ⛁ Helion       0 ─  ✗      │ [ Start job ]│      │
│  └───────────────┘  ▮▮▮▯ have/need per node   └──────────────┘      │
│  USED IN ▸ mod-autocannon-m, mod-autocannon-s   (Show Info links)   │
└────────────────────────────────────────────────────────────────────┘
  connectors are line segments with a state glyph at each end (B2),
  never a free-form node graph: depth is exactly two tiers (B7)
```

*Patterns combined:* B2 + B7 (connection lines with Ready/Not Ready, two-tier flow), A5 (expansion),
the repo's own `MANUFACTURED FROM` / `USED IN` rows (`infowin.go:222,259`), and the refinery's yield
readout (`refinery.go:183`).
*What it solves:* "where does this come from" including the ore step the window currently never mentions,
which `docs/GAMEPLAY.md` has to spell out in prose.
*Fit/cost:* highest risk of the four: node-link layouts fight the table's density, and both the sankey
literature [S17-S19] and CCP's clutter note [S1] warn about it. If taken, restrict the flow to the
selected recipe, fix node positions deterministically (canonical id order, no layout jitter), and keep it
as a *pane* the seam can collapse. *(opinion: sequence this last, behind D1 or D2)*

### Shared acceptance test *(opinion)*

Whatever is chosen, the board must answer three questions without the player reading a number: which
recipes are ready, what is blocking the ones that are not, and which mineral dominates the cost. That is
the yardstick CCP set for itself ("readable at a glance ... making sure the screen would not become too
cluttered", [S1]) and it is measurable in a capture test, which this repo already uses.

## 5. Appendix: art and primitives available today

### 5.1 Icon manifest (`uikit/icons/manifest.json`, 72 keys, CC BY 3.0 from game-icons.net)

Grouped as the keys actually are (full list printed from the manifest):

- **Actions:** `act_approach, act_dock, act_jump, act_lock, act_loot, act_mine, act_orbit, act_stop, act_undock, act_warp`
- **Kinds (world):** `kind_anomaly, kind_asteroid, kind_belt, kind_gate, kind_moon, kind_planet, kind_ship, kind_star, kind_station, kind_unknown, kind_wreck`
- **Module classes:** `mod_artillery_l, mod_artillery_m, mod_artillery_s, mod_autocannon_m, mod_autocannon_s, mod_defense, mod_mining, mod_scram, mod_utility, mod_weapon, mod_web`
- **Rows/service:** `row_hull, row_module, ammo, hud_time, svc_contracts, svc_finances, svc_fitting, svc_help, svc_inventory, svc_log, svc_manufacturing, svc_map, svc_market, svc_refinery, svc_scanner`
- **Contract objectives:** `ctr_deliver, ctr_kill, ctr_mine`
- **Stat labels:** `stat_align, stat_armor, stat_burst, stat_capacitor, stat_cargo, stat_cash, stat_cpu, stat_em, stat_explosive, stat_falloff, stat_flee, stat_grid, stat_kinetic, stat_mass, stat_range, stat_signature, stat_size, stat_speed, stat_structure, stat_thermal, stat_tracking, stat_warp`

Mapping the names the brief asked about (manifest `file` field): `svc_manufacturing` = `factory.png`,
`svc_refinery` = `furnace.png`, `stat_structure` = `cubes` slug (`stat_structure.png`), `kind_belt` =
`stone-pile.png`, `kind_asteroid` = `asteroid.png`, `hud_time` = `stopwatch.png`, `stat_cash` = `cash`,
`svc_market` = `coins.png`, `stat_cargo` = `cargo-crate.png`, `row_module` = `gears.png`,
`row_hull` = `iron-hulled-warship.png`, `ammo` = `artillery-shell.png`, `mod_mining` = `miner.png`.
**There is no per-mineral manifest glyph.** All 72 keys and their author credits are pinned by
`uikit/icons/manifest.json` and `CREDITS.md`, and a test pins each PNG to a manifest entry (AGENTS.md).

### 5.2 How item icons actually resolve (`uikit/theme/iconmap.go:116`, `uikit/widgets/tradeable.go:34`)

```go
// ItemIconFor resolves one item's icon: the player's drop-in art for its
// content id first, else its category's silhouette (ammo, loot, mineral,
// ore), else the neutral gear.
func ItemIconFor(def *content.ItemDef) string {
	if id := string(def.ID); id != "" && icons.Has(id) { return id }
	switch def.Category {
	case content.CatAmmo, content.CatCharge: return "ammo"
	case content.CatLoot:                    return "act_loot"
	case content.CatMineral:                 return "kind_belt"
	case content.CatOre:                     return "kind_asteroid"
	}
	return icons.Fallback
}
```

Consequences for the design:

- **Every mineral and ore already has its own checked-in art**, keyed by content id, so
  `widgets.TradeableIcon` returns `min-ferrite` etc. and draws full-colour:
  `assets/icons/min-ferrite.png, min-nickel.png, min-cobalt.png, min-silex.png, min-glacite.png,
  min-argon.png` and `assets/icons/ore-sterngrit.png, ore-cordite.png, ore-lucidite.png,
  ore-glacierite.png` (all tracked by git, unlike the other `assets/**` drop-ins). Loot
  (`loot-alloy-scrap.png`, `loot-hierarch-crest.png`, `loot-iced-core.png`, `loot-navdata.png`) and most
  `mod-*.png` are there too.
- **Art mode ignores tint** ("an art-mode entry ... is drawn full-colour as-is: the tint is ignored",
  `uikit/icons/icons.go`), so mineral chips will be full-colour, not palette silhouettes. That is already
  house-consistent (product icons in market/industry rows are the same art), but it means the six chips
  cannot be recoloured to signal state: state must ride the bar/number/glyph beside the chip (WCAG C3).
- **Without art a mineral falls back to `kind_belt`**, i.e. all six look identical, so any design that
  identifies materials *by silhouette alone* fails on a fresh checkout without the drop-ins. Use
  icon + label + bar, never icon alone. *(inference)*
- Sizes: 16 inline in rows, 20 buttons, 24 title bars, `IconHero` 128 (`theme/iconmap.go:14-31`).

### 5.3 Palette roles available for state (`uikit/theme/theme.go:18-63`)

`Accent #ffb454` (selection/ready emphasis), `Good #5fcf80`, `Warn #e0a33c`, `Danger #e05555`,
`Info #8fb8e0`, `Dim #7a8494`, `Cell` (bar track, painted at 160 alpha in `PaintBar`), `Head`/`HeadBg`.
Stat families add `StatOffence/StatMobility/Stat*` hues. House rule: palette only, no new colours.

### 5.4 Widgets the four directions may lean on (all existing)

`widgets.SplitPane` / `NewSplitPaneRow` with named seams; `widgets.Table` columns
(`Icon`, `Priority`, `AlignRight`, `PrefsID`), `CellRuns` for styled suffixes, `Stack` rich rows with
`RowLine`/`RowSeg{Text,Tint,Step,MinW,Icon,Bar,HasBar}` and trailing `RowSpec` action buttons;
`widgets.NewStackRow`; `PaintBar`; `sectionBar`/`theme.SectionBar` for section headings; `ScrollRail`;
`widgets.ResolveTradeable`/`TradeableIcon`; `Dialog`; `QtyWin` (quantity picker); the fuzzy search
helpers in `market.go` (`mktNodes`, `scopeItems`, `searchHits`, `mktNameSegs`) which are currently
market-private and would need lifting into a shared helper if reused. *(fact + inference: the helper
lift is my assessment of the code shape, not a source claim)*

## 6. Open questions for the owner *(opinion)*

1. **Which axis is the tree?** Product `group` paths (exact market mirror, 16 branches, reuses
   `mktNodes`), or `family` (3 branches: hulls/modules/charges, matches the craft levels on the readout),
   or both (family tabs over group tree, which is two levels of navigation and edges toward NN/g's
   two-level caution)?
2. **Ingredient-first or product-first?** A4's "search by mineral" is cheap; making it a *branch* (the
   "by mineral:" column in D2) makes the six-mineral economy the primary navigation, which no reference
   game does but which fits a six-mineral game better than a 200-reagent one.
3. **List or grid for browsing** given the direct conflict between [S1] and [S5]?
4. **Does the window widen?** D2 and D4 need it; the market's landing rect is the in-repo precedent and
   CCP enlarged theirs deliberately [S1].
5. **Do we show money in the strip** (material value share, fee share) or quantities only? The fee is
   already "3% of a recipe's mineral value" (`CONTEXT.md`), so value share is honest, but it introduces
   price literacy into a window that today shows none.
6. **Keep the Family column?** It is a raw lowercase string (`"hulls"`) today; if the tree owns category,
   the column is redundant. *(fact: `rec.Family` is printed unformatted, `manufacturing.go:248`)*

## 7. Sources (accessed 2026-11-03)

Official / primary studio:
1. CCP Arrow, *Industry UI* (dev blog, 2014-04-28). https://www.eveonline.com/news/view/industry-ui
2. Official Factorio Wiki, *Crafting*. https://wiki.factorio.com/Crafting
3. Official Factorio Wiki, *Production statistics*. https://wiki.factorio.com/Production_statistics
4. Official Satisfactory Wiki, *Codex*. https://satisfactory.wiki.gg/wiki/Codex
5. Automation Station, *Devlog #27: The Codex* (studio dev blog). https://automationstationgame.com/blog/devlog-27/
6. TradeSkillMaster, *Announcements archive* and *TSM4* (official addon blog; the two quotes verified
   against the raw HTML). https://blog.tradeskillmaster.com/category/announcement/page/2/ ; https://blog.tradeskillmaster.com/category/tsm4/
7. Square Enix, FFXIV *Patch 7.3 Notes (Preliminary)* (crafting log search results). https://eu.finalfantasyxiv.com/lodestone/topics/detail/d59ffe73a7f15c0991239761c7c0b715a17e785c/
8. Square Enix, FFXIV *Eorzea Database: Crafting Log* (category as a search condition). https://na.finalfantasyxiv.com/lodestone/playguide/db/recipe/
9. Blizzard, *Dragonflight Preview: More on Professions*. https://news.blizzard.com/en-us/article/23826545/dragonflight-preview-more-on-professions

UX literature and standards:
10. EVE University Wiki (community), *Trading*. https://wiki.eveuniversity.org/Trading
11. Nielsen Norman Group, *Progressive Disclosure* (2006). https://www.nngroup.com/articles/progressive-disclosure/
12. Nielsen Norman Group, *10 Usability Heuristics for User Interface Design* (1994, reviewed 2024). https://www.nngroup.com/articles/ten-usability-heuristics/
13. W3C WAI, *Understanding Success Criterion 1.4.1: Use of Color*. https://www.w3.org/WAI/WCAG21/Understanding/use-of-color.html
14. Desurvire & Wiberg, *Game Usability Heuristics (PLAY) for Evaluating and Designing Better Games: The Next Iteration* (PDF, hosted copy; published version https://dl.acm.org/doi/10.1007/978-3-642-02774-1_60). https://ocw.metu.edu.tr/pluginfile.php/4129/mod_resource/content/0/ceit706_2/10/game_usability-_heuristics.pdf
15. Game UI Database 2.0, *Crafting: Browse* (reference library, used as a pointer, not as a claim). https://gameuidatabase.com/index.php?&scrn=75&set=1&series=56&plat=1

Graphs, ratios, clutter:
16. *NeatSankey: Sankey diagrams with improved readability based on ...*, Computers & Graphics (ScienceDirect, abstract only, paywalled). https://www.sciencedirect.com/science/article/abs/pii/S0097849323000481
17. *Showing Flow: Comparing Usability of Chord and Sankey Diagrams*, ACM (abstract visible in search index only; page returns 403). https://dl.acm.org/doi/10.1145/3544548.3581119
18. *Hierarchical Sankey Diagram: Design and Evaluation* (PDF). https://academicweb.nd.edu/~cwang11/papers/isvc21-hsd.pdf

Weak sources (labelled at point of use; pages behind Cloudflare, quoted from search-index snippets only):
19. Factorio forums, *Search in crafting recipes should search in ingredients, too* (403 on fetch). https://forums.factorio.com/viewtopic.php?t=33669
20. Satisfactory official Q&A, *Go back in codex* (post no longer resolvable). https://questions.satisfactorygame.com/post/6393171fca608e0803528b9
21. Steam discussions, Captain of Industry, *Individual tree (like a wiki) on items to produce*. https://steamcommunity.com/app/1594320/discussions/3/603036300513258748/
22. Fan calculators: https://eve-industry.org/ ; https://www.fuzzwork.co.uk/blueprint/ ; https://www.adam4eve.eu/manu_rank.php

Not sourced (dropped rather than guessed): Dyson Sphere Program's replicator, Mindustry's build-cost
display, shapez's unlock bar and FFXIV's in-game crafting log layout (community wikis only, and
fandom.com/forums blocked every fetch path). Nothing in §3-§4 rests on them; if one of them matters to
the owner, it needs a screenshot-level review rather than a text source.

Repo sources: `uikit/screens/manufacturing.go`, `uikit/screens/market.go`, `uikit/screens/refinery.go`,
`uikit/screens/qtypicker.go`, `uikit/screens/infowin.go`, `uikit/screens/contacts.go`,
`uikit/widgets/table.go`, `uikit/widgets/tradeable.go`, `uikit/theme/iconmap.go`, `uikit/theme/theme.go`,
`uikit/theme/stat.go`, `uikit/icons/icons.go`, `uikit/icons/manifest.json`, `assets/icons/*`,
`content/data/recipes/*.json`, `content/data/items/{minerals,ores,charges,ammo}.json`, `CONTEXT.md`,
`docs/GAMEPLAY.md`, `docs/SIM.md`.
