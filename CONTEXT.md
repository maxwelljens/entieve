# CONTEXT.md — shared vocabulary

Domain and architecture terms used across Entieve. Domain canon lives in
`docs/lore/` (UNIVERSE, FACTIONS, AESTHETIC, NAMING — binding); this file
names the *modules and seams* we build that vocabulary into.

## Domain (the world)

| Term | Meaning |
|---|---|
| **Corporation** | a named manufacturing house (`corp-*`, `content.AllCorps`): the maker of a hull, module or charge, carrying a display name and an emblem; a supplier layer, never a power bloc |
| **Hull** | a ship chassis (`ship-*`); flyable or machine |
| **Hull portrait** | top-down full-colour art of a hull for the HUD/Show Info (`uikit/hulls`): a flat SVG source in `svg/` rasterized at build time to committed 120/240 px PNGs, keyed by hull id (`Get`/`Has`) |
| **Module** | a fitted component (`mod-*`); slot-classed high/mid/low |
| **Mount** | a hull's high slot, declared as a kind and a size (`high_mounts`); a mounted module needs a mount of its own kind at or above its size, so a large gun never sits on a frigate |
| **Turret mount / launcher mount** | the two mount kinds: guns and the mining laser are turret-mounted, rockets and missiles launcher-mounted |
| **Grid class / cpu class** | a module's powergrid or cpu demand as one of three classes — Class I / Class II / Class III — never a number; a module demanding both axes consumes one unit on each |
| **Class quota** | a hull's per-class allowance on each axis (`grid_classes` / `cpu_classes`); a fit is legal while no class's module count exceeds its quota, which is what bounds a full rack |
| **Item** | everything else tradeable: ore, mineral, charge, round, loot (`ore-*`/`min-*`/`ammo-*`/`charge-*`/`loot-*`) |
| **Round family** | a round size class **and** gun family as one `ammo_class`: `small-autocannon`, `small-artillery`, `medium-autocannon`, `medium-artillery`, `large-autocannon`, `large-artillery`. One family is one calibre: every round of it has the same m³ volume, and only guns of that family load it (autocannon = rounds, artillery = shells) |
| **Round** | the ammunition item (`ammo-*`) a projectile gun loads: its family + one of the four damage profiles; the gun's `damage_multiplier` is its share of the round's volley (autocannon light and fast, artillery heavy and slow) |
| **Bay** (clip) | a charge-consuming module's fixed m³ ammunition space (`weapon.bay_m3`, `boost.bay_m3` for the cap booster): whole rounds of exactly one round type, sized so sustained fire reloads about every 120 s |
| **Load cycle** | filling a bay from cargo: a full bay's rounds, taking exactly one firing cycle (no capacitor cost), so the next shot is deferred by it. The rounds move in at the start; the bay is ready when the cycle completes (the rack draws the ring lit meanwhile and a shell marker once armed). Also what `CmdSetAmmo` starts when it switches a round type; only the fresh fit at boot is pre-loaded instantly |
| **Waiting state** | a bay sitting empty while two or more round types compete in cargo and none is selected: the gun logs one `<module> waiting for ammo selection` line, deactivates quietly, and fires only once `CmdSetAmmo` picks a type. Distinct from truly dry (empty bay, no matching cargo rounds = the `no charges` warning) |
| **Caste** | an Old Works machine archetype (`npc-*`: Scuttler, Marauder, Hierarch) + the Charter patrol (Custos) |
| **Station** | a market/service node (a celestial); stations hold the order books |
| **Service** | a station's offered capability, owned by one table (`content.Services`): the id content files spell, its display label and icon, and the Screen window its services-row button toggles (`repair` opens none) |
| **Order book** | per-station continuous double auction: asks + bids, never-crossed |
| **Market tree** | the market window's browse hierarchy, built from every marketable def's `group` path (`Ships/Frigates`, `Modules/High/Turrets`, `Materials/Ore`, …): `All` at the root, a branch click scopes the catalog, and the search field filters the selected branch by name, path and maker |
| **Bid** | a standing *buy* order on a station book: buy at your price or better, resting until filled or cancelled |
| **Ask** | a standing *sell* order on a station book: sell at your price or better, resting until filled or cancelled |
| **Open order** | a player's standing bid or ask resting on a book — up to 20 universe-wide, partial fills accumulate, cancelled by id (3% broker fee charged at placement and kept on cancel; 2.5% sales tax on executed sells) |
| **Recipe** | two-tier manufacturing: minerals → products (`rec-*`) |
| **Job** | one manufacturing run queue at any station (industry); all stations share three global job slots |
| **Manufacturing** | the industry window (key B): recipe board with per-run time and up-front fee, the three global slots, Start/Collect/Cancel. Renamed from Factory; its layout sidecar key is `manufacturing` (old `factory` files migrate on read) |
| **Job fee** | 3% of a recipe's mineral value at base prices, per run, debited from the wallet at job start; kept on cancel |
| **Refinery** | the ore→mineral window at a `refine` station: ore list from cargo + ore hold, `floor(refine_yield × qty)` preview, Refine. Ore refines in whole **100-ore batches** (the stepper offers 100/200/300…, capped at the selected ore's stock; under 100 ore nothing refines) and the minerals always land in the station hangar, never the ship's hold |
| **Craft level** | a station's learned skill per product family (grows with volume) |
| **Belief** | an agent's EWMA of executed trade prices (its market view) |
| **Haul wish** | an agent's cross-node arbitrage intent, flown as a real convoy |
| **Warp process** | align → accel → cruise → decel; spatial easing over a frozen timeline |
| **Departure beat** | the jittered 2–8 s hold a convoy keeps after spawning, resuming or arriving before it commences its warp — a stateless tick hash, never a persisted timer |
| **Gate cycle** | a stargate transit in two paced parts: the *dwell* (2–5 s at the gate before the hop) and the *jump cycle* (1–3 s between hop-out and materializing at the destination gate) |
| **Turnaround** | a station visit in stages: approach (2–4 s) → docked dwell (5–15 s, the cargo/market action fires partway through) → undock + departure beat |
| **Hold** | a shipment parked upstream of a full gate queue: it waits in the journey ledger until the far queue drains, logging one line on entering the hold and one when it finally jumps |
| **Agent cooldown** | per-agent jittered 1–10 s between behavior acts; one act moves capital at most once (its job / quotes / wish slot rotates on a stateless tick hash) |
| **Belt sweep** | the security response to pirate rats parked on a Charter-space (high-sec) belt: a static guard clears the rats anchored at its own celestial, and a traffic patrol sweeps an occupied belt once per **patrol beat** (the authored patrol cadence, re-armed after every sweep) and flies home. A sweep stays until the belt is empty and hunts stragglers off the anchor; free rats are engaged first, and a fight a pilot is already in is theirs only while another rat is free. The Fringe / Unincorporated get no sweep. The last rat of a belt dying logs one `<belt> cleared` line; the belt's respawn timers re-arm it |
| **Legacy id** | a content id a rename retired, mapped onto its replacement in `content.LegacyIDs` (the keys are `content.RetiredIDs`): validation rejects them in shipped content, and restore remaps them in place — `sim.walkContentIDs` finds every id a save carries by reflection, so a new id-bearing field needs no second list |

## Architecture (the UI tranche)

| Term | Meaning |
|---|---|
| **Screen** | a window the player can open: declared as one **Screen spec** row (id, title, icon, key, default rect, dock policy, sync, build) from which the registries derive |
| **Keybindings** | the player's key overrides: a `{version, binds}` sidecar beside the save (`<save>.keys.json`) holding **only** the deviations from the defaults, action id → key (or `null` for an action left unbound). Rebindable actions are the **Screen** rows' toggles (their `Action` ids) and the world commands (the lowercase ids declared in `worldKeyMap`: `lock-nearest`, `high-1`…`high-8`, `approach`, `orbit`, `stop`, `align`, `warp`, `dock`, `jump`, `loot`, `mine`, `pan-up`/`down`/`left`/`right`). **Replace policy**: a key belongs to one action, so binding an occupied combination steals it from its holder (which becomes unbound, shown `—`); replaying the overrides over the defaults on load reproduces the same state. The global chords are a fixed, never-rebindable layer — `Esc` (reserved even as a plain binding), `Ctrl+S`, `Ctrl+F`, `F9`, `[` `]` `-` `=` — handled literally before the data layer |
| **Title screen** | the boot phase (`Options.StartInTitle`, set by the `run` command only and never with `--load`): the already-built world renders as a static backdrop under the title menu, the simulation does not step, and gameplay chrome (rail, HUD, rack, core windows, log) stays unmounted; leaving it runs one normal boot reconcile |
| **Save manager** | the title screen's window over the saves directory (`filepath.Dir` of the configured save path): `*.json` save files only (layout/settings/keys sidecars excluded), newest first, with **Load** and a two-click **Delete** |
| **Active save path** | where **Save** writes (Esc-menu Save / Ctrl+S): the configured save path until a load or a **Save as…** makes another file active; layout and settings sidecars stay keyed to the configured path |
| **DockPolicy** | where a Screen's window lives: `floating` (placed by `placeDefaults` from its declared rect — every window, the docked services view included) or `transient` (scene-owned, never persisted — the Esc menu) |
| **Anchor** | which edges of the viewport a floating Screen hangs its design-unit default rect on: centred, a fixed design coordinate, bottom-left insets, or top-centred |
| **Row** | one listed entry in a table; its interactions derive from its **RowSpec** |
| **Market tree** | the market window's order book as one parent row per catalog item (every tradeable item, module and hull, always listed; dimmed while it has no orders, still fully actionable) with that item's resting orders folded out beneath it on a click — asks ascending, then bids descending — and the item row's own price (the station ladder) and two-side quantity. The fold state is session-only: never saved, forgotten when the window closes |
| **RowSpec** | the row's declared data: `Ref` (what it *is*, for Show Info) + `Actions` (typed intents it offers). One installer applies menu, tooltip and double-click uniformly |
| **Rich row** | a `TableRow` carrying a **line stack** (`Stack []RowLine`) instead of one text per column: runs of cells with their own step/colour/width, `Icons[0]` boxing with line 1, `Spec.Actions` drawn as trailing buttons (1–2, grayed-not-hidden) and a per-line `Ref` for the installer — the contract board's row form (`widgets.StackRow` builds one, `widgets.StackLayout` tiles a section of them) |
| **Plain table** | a decorless column list: aligned columns, no header, no row tints/zebra, no borders — dense label/value rows for readouts like the Finances holdings block. `widgets.PlainTable`: leading columns pack at their measured widths, the last column stretches to the row's right edge, so a value column's End anchor lands on one shared x in the mono tabular face |
| **Theme** | faces + scaled metrics + palette — the visual constants a widget draws itself with (the seam interface and its implementation live together in `uikit/theme`); a Theme-only widget takes it alone |
| **Spacing tokens** | the furniture rhythm in `uikit/theme` design units: `SpSm`/`SpMd`/`SpLg` = 4/8/16 (insets, button/title-bar padding, the scrollbar handle). Furniture takes its air from them; **data rows keep their own denser literals** (the table's `cellPad`/`entryPadV`) — table density is deliberate |
| **Scroll gutter** | `theme.ScrollGutter` (SpLg + 2×SpSm = 24) — the column reserved beside every scroll viewport for its scrollbar, so the handle can never overhang the content and the viewport never jumps between fit and overflow. Built once as `widgets.ScrollRail` (slider in the gutter, hides while the content fits); carried by `widgets.Table`, the station rail's entry column and the Show Info page |
| **Split pane** | `widgets.SplitPane`: panes divided by draggable **seams** instead of GridLayout's equal shares — a vertical stack (`NewSplitPane`) or a row (`NewSplitPaneRow`, the market's book / My orders). A seam is a recessed 4 px track (dark fill edged in the border color) with a centred three-dot sash grip — dim at rest, lit while hovered or dragging — and an 8 px grab band classified from the cursor every frame (the window chrome's grip idiom); the cursor is NS-resize in a stack, EW-resize in a row. Each pane keeps a 72 design-px floor so nothing collapses. Every seam is named for the layout sidecar (`inventory-ore`, `inventory-hangar`, `fitting-hangar`, `manufacturing-jobs`, `market-orders`) and stores **the fraction of the stack before it** — above in a stack, left of it in a row; equal shares when nothing is stored (the market opens at the book's 240/526 share, today's fixed side column) |
| **Column prefs** | a table's per-column state (`widgets.ColumnPref`): the width and hidden flag the player sets through the header's **grips** (6 design-px strip at each column's right edge, its own blocking input layer, EW cursor) and the **⋯ column menu** (flat, one ticked row per column, last visible column disabled). Two policies: the **first fit** measures the content once and freezes each column's base — later content changes ellipsize, never move an anchor; **every later layout** (table resize, hide/show, stored prefs) distributes the table width over the bases **proportionally**, so a player-made width persists, a hidden column restores its base and a grip drag or table resize re-bases at the widths on screen. Every table tags itself with a sidecar id (`widgets.PrefsID`); widths and hidden flags ride the layout sidecar's `tables` map in design units |
| **ContentView** | the read-only seam: defs + observable sim state as screens see it, implemented over `*content.Content` + `*sim.Sim` — screens never see the simulation |
| **ActionSink** | the write seam: screens emit typed **intents** (`Buy{Item,Qty}`, `StartJob{…}`) through `Do(Intent)`; the game executes them (`Game.Do`); later the AI surface will emit the same ones |
| **Seams** | the union `Theme` + `ContentView` + `ActionSink` — the constructor param of a screen that draws, reads *and* acts (`uikit/widgets.Seams`; the dissolved `Env` is only this alias) |
| **Session** | one host's environment: the three seams plus its shared context menu and Show Info window (`uikit/screens.Session`, from `Kit.Env` / `NewEnv`) |
| **uikit modules** | `uikit/theme` (visual constants + the Theme seam) ← `uikit/widgets` (mechanics: table, menu, tooltip, rowspec, intents, seams) ← `uikit/screens` (the panels + Kit): each layer imports only downward, pinned by `TestImportDirection` |
| **Deep module** | a module whose interface is small and whose implementation absorbs complexity (the goal of every seam here) |
| **Close button** | the `×` affordance at a title bar's right edge: registered per window through `Kit.SetWindowClose`, wired to that window's existing close semantics (Screen-row toggle, core toggle, Show Info close). Its hit strip is excluded from the chrome-owned title drag; the window's resize grip keeps precedence where the strip meets a grip band |
| **Window animation** | the render-only open/close fade: a mount fades + slides in over 12 frames, a close-button close fades out over 10 frames before its unmount runs. The animating window is parked out of ebitenui's window pass and composited through one offscreen with alpha/translate — input is live throughout the open fade, inert throughout the close fade; no rects, layout or simulation state move |
| **Settle** | an animation completing: an open window snaps to the normal paint path, a closing window runs its deferred unmount. A raise settles every animation (nothing may fade below a retopped window); a new open mount settles the close fades only (the other open fades below it keep running — the composite walks the stack in paint order); a drag-arming press settles that window's open fade, so a fade's park flips can never relayout a subtree a drag is translating |

## Vocabulary rules

- Display names follow `docs/lore/NAMING.md`; ids (`sys-*`, `mod-*`, …) are frozen artifacts.
- Money renders trailing-euro (`12,345.67 €`, `50¢`); zones are Charter space / the Fringe / Unincorporated.
- New concepts get named here before they get built.
