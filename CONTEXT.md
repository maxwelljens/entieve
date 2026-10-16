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
| **DockPolicy** | where a Screen's window lives: `floating` (composed by `placeDefaults` from its declared rect), `station` (a member of the docked trio: `stationLayout` owns its rect and the scene force-opens it), `transient` (scene-owned, never persisted — the Esc menu) |
| **Anchor** | which edges of the viewport a floating Screen hangs its design-unit default rect on: centred, a fixed design coordinate, bottom-left insets, or top-centred |
| **Row** | one listed entry in a table; its interactions derive from its **RowSpec** |
| **RowSpec** | the row's declared data: `Ref` (what it *is*, for Show Info) + `Actions` (typed intents it offers). One installer applies menu, tooltip and double-click uniformly |
| **Rich row** | a `TableRow` carrying a **line stack** (`Stack []RowLine`) instead of one text per column: runs of cells with their own step/colour/width, `Icons[0]` boxing with line 1, `Spec.Actions` drawn as trailing buttons (1–2, grayed-not-hidden) and a per-line `Ref` for the installer — the contract board's row form (`widgets.StackRow` builds one, `widgets.StackLayout` tiles a section of them) |
| **Theme** | faces + scaled metrics + palette — the visual constants a widget draws itself with (the seam interface and its implementation live together in `uikit/theme`); a Theme-only widget takes it alone |
| **ContentView** | the read-only seam: defs + observable sim state as screens see it, implemented over `*content.Content` + `*sim.Sim` — screens never see the simulation |
| **ActionSink** | the write seam: screens emit typed **intents** (`Buy{Item,Qty}`, `StartJob{…}`) through `Do(Intent)`; the game executes them (`Game.Do`); later the AI surface will emit the same ones |
| **Seams** | the union `Theme` + `ContentView` + `ActionSink` — the constructor param of a screen that draws, reads *and* acts (`uikit/widgets.Seams`; the dissolved `Env` is only this alias) |
| **Session** | one host's environment: the three seams plus its shared context menu and Show Info window (`uikit/screens.Session`, from `Kit.Env` / `NewEnv`) |
| **uikit modules** | `uikit/theme` (visual constants + the Theme seam) ← `uikit/widgets` (mechanics: table, menu, tooltip, rowspec, intents, seams) ← `uikit/screens` (the panels + Kit): each layer imports only downward, pinned by `TestImportDirection` |
| **Deep module** | a module whose interface is small and whose implementation absorbs complexity (the goal of every seam here) |

## Vocabulary rules

- Display names follow `docs/lore/NAMING.md`; ids (`sys-*`, `mod-*`, …) are frozen artifacts.
- Money renders trailing-euro (`12,345.67 €`, `50¢`); zones are Charter space / the Fringe / Unincorporated.
- New concepts get named here before they get built.
