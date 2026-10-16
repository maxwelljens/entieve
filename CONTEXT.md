# CONTEXT.md — shared vocabulary

Domain and architecture terms used across Entieve. Domain canon lives in
`docs/lore/` (UNIVERSE, FACTIONS, AESTHETIC, NAMING — binding); this file
names the *modules and seams* we build that vocabulary into.

## Domain (the world)

| Term | Meaning |
|---|---|
| **Hull** | a ship chassis (`ship-*`); flyable or machine |
| **Module** | a fitted component (`mod-*`); slot-classed high/mid/low |
| **Item** | everything else tradeable: ore, mineral, charge, ammo, loot (`item-*`/`ore-*`/`min-*`/`charge-*`/`loot-*`) |
| **Caste** | an Old Works machine archetype (`npc-*`: Scuttler, Marauder, Hierarch) + the Charter patrol (Custos) |
| **Station** | a market/service node (a celestial); stations hold the order books |
| **Order book** | per-station continuous double auction: asks + bids, never-crossed |
| **Recipe** | two-tier manufacturing: minerals → products (`rec-*`) |
| **Job** | one manufacturing run queue at a station (industry) |
| **Craft level** | a station's learned skill per product family (grows with volume) |
| **Belief** | an agent's EWMA of executed trade prices (its market view) |
| **Haul wish** | an agent's cross-node arbitrage intent, flown as a real convoy |
| **Warp process** | align → accel → cruise → decel; spatial easing over a frozen timeline |

## Architecture (the UI tranche)

| Term | Meaning |
|---|---|
| **Screen** | a window the player can open: declared as one **Screen spec** row (id, title, icon, key, default rect, dock policy, sync, build) from which the registries derive |
| **Row** | one listed entry in a table; its interactions derive from its **RowSpec** |
| **RowSpec** | the row's declared data: `Ref` (what it *is*, for Show Info) + `Actions` (typed intents it offers). One installer applies menu, tooltip and double-click uniformly |
| **Theme** | faces + scaled metrics (the visual constants; `uikit/theme`) |
| **ContentView** | the read-only seam: defs + observable sim state as screens see it |
| **ActionSink** | the write seam: screens emit typed **intents** (`Buy{Item,Qty}`, `StartJob{…}`); the game executes them; later the AI surface will emit the same ones |
| **Deep module** | a module whose interface is small and whose implementation absorbs complexity (the goal of every seam here) |

## Vocabulary rules

- Display names follow `docs/lore/NAMING.md`; ids (`sys-*`, `mod-*`, …) are frozen artifacts.
- Money renders trailing-euro (`12,345.67 €`, `50¢`); zones are Charter space / the Fringe / Unincorporated.
- New concepts get named here before they get built.
