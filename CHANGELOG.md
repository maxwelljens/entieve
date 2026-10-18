# Changelog

All notable changes to Entieve are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- **Star systems rescaled to real-solar-system proportions**
  (`content/data/systems/*.json`, `content/data/worlds/*.json`): bodies
  spread across tens of AU instead of the old ~0.1 AU cloud — inner
  planets 0.4–2 AU from the star, asteroid belts (and belt-like anomalies
  like Gniazdo) 2–10 AU, stations 1–10 AU near their planet or
  independent, stargates 20–40 AU on the rim. Ids, names, radii, kinds
  and every other field are untouched: only positions moved. Each body
  keeps its authored direction from the star (gate directions still read
  as the Kresy ring on the starmap), distances come from per-system
  seeded radial slots (system id as the variation seed — scheme in
  `docs/SIM.md` §7), and stations/belts/anomalies hold a 2 AU separation
  floor (gates 5 AU) so landmarks can never bunch again — pinned by
  `content/layout_test.go`. Warp physics is unchanged (hull `WarpSpeed`
  was already AU/s), so transit across a system now takes tens of seconds
  EVE-style, and convoy/miner legs dominate their economy cycles until
  the cadence knobs are balanced live. World starts ride their anchor
  bodies (Jernborg, the Starogród Belt). The space camera's zoom-out
  range now frames the whole system (`game/scene.go`, pinned in
  `game/scene_test.go`); grid-scale UX is untouched. Showcase tour
  timings drift with the new legs (its recalibration follows separately).

- **Distance display switches to AU past 99,999 km** (`uikit/theme/format.go`,
  `render/art.go`, `uikit/screens/selected.go`, `sim/command.go`): the four
  in-sync distance formatters now read meters as "42 m" under 2 km, 1-decimal
  km through 99,999 km, the literal "<0.01 AU" in the gap below 0.01 AU, and
  two-decimal AU beyond (1 AU = 1.496e11 m, house convention) — so 100,000 km
  shows "0.01 AU" instead of "100,000.0 km". The hint/refusal `kmLabel` pair
  keeps its whole-km style inside the km band ("150 km" for `WarpMinRange`).
  AU figures are fixed two decimals ("0.10 AU", "2.01 AU"), matching the
  house figure style and tabular columns. Golden pins updated and extended in
  `uikit/theme/theme_test.go`, `uikit/screens/selected_test.go`,
  `sim/helpers_test.go`, `render/art_test.go`.

## [0.4.0] - 2026-10-18

### Added

- **Title-bar close buttons, window open/close fades, translucent window
  bodies** (`uikit/screens/winanim.go`, `uikit/screens/winchrome.go`,
  `uikit/theme/chrome.go`, registration in `game/game.go`): every window
  with a title bar carries a × at its right edge, wired through that
  window's EXISTING close semantics — a Screen row flips its own toggle
  (modal rows pop their scene), the two core windows close through the
  rail's toggle, Show Info keeps its persisted rect and its
  never-auto-open rule, and the title-less Station Services view stays
  as it was. The press is excluded from the chrome-owned title drag (the
  resize grip keeps corner precedence, and a drag that started on the
  strip never counts as a × click) and rides Kit.Update's menu guard, so
  a modal context menu's frame can never close a window. Opens and
  closes animate, render-only: a mount fades + slides in over 12 frames
  with input live from frame 0, a × close commits immediately (the
  window is inert for its 10-frame fade) and defers its unmount to
  settle — the animating window is parked out of ebitenui's window pass
  and composited through one reusable offscreen with alpha/translate (no
  rects move, no layout or simulation state changes, no RNG), and any
  raise, fresh mount or drag-arming press settles a running fade so
  paint order always matches the stack. Window bodies draw through ONE
  translucency token (`theme.WindowAlpha`, ~0.90) — title bars stay
  opaque, HUD/rack/rail keep the palette alpha. Vocabulary pinned in
  `CONTEXT.md` ("close button", "window animation", "settle"); pins in
  `uikit/screens/winclose_test.go`, `uikit/screens/winanim_test.go`,
  `uikit/screens/winchrome_capture_test.go`,
  `uikit/theme/windowbody_test.go`, `game/closebutton_test.go`.

- **Organic pace before every commencement** (`sim/system_traffic.go`,
  `sim/system_agent.go`, `sim/agent.go`): convoys and economy agents no
  longer act the instant it becomes possible — every window is tick-based
  and deterministically jittered from the existing pure hashes (no RNG
  draws, no new save fields: the running deadlines ride the already
  persisted `Traffic.Next`/`Traffic.Phase`), so twin-run hashes and
  save/load stay bit-identical while traffic gains a beat:
  - *departure beat* — a spawned, resumed or arrived hauler holds
    120–480 ticks before its leg warp commences (the warp physics are
    untouched: align, tunnel and landing keep their pinned timeline);
  - *gate cycle* — a cross-system hauler dwells 120–300 ticks at the gate
    before the hop, and the parked journey carries a jump cycle of
    60–180 ticks between hop-out and materializing at the destination
    gate;
  - *turnaround* — a station endpoint runs approach (120–240 ticks) → a
    docked dwell (300–900 ticks) whose cargo/market action fires partway
    through, never at the instant of contact (the hold leaves the hauler
    exactly when the book gains it — the mass balance stays closed) →
    undock + departure beat;
  - *agent cooldown* — each ledger agent's behavior act fires on its own
    hashed 60–600-tick cooldown, and one act performs at most one
    capital-moving step (commission / quote / contract slots rotate on a
    hashed act ordinal), so an agent never moves capital twice inside one
    cooldown.
  Vocabulary pinned in `CONTEXT.md` ("departure beat", "gate cycle",
  "turnaround", "hold", "agent cooldown"); pins and conservation gates in
  `sim/pacing_test.go`.

- **Drop-in starfield background plates** (`assets/backgrounds/`): any star
  system's starfield can be dropped in as PNG layers — `<system-id>.png`
  (layer 0, near/bright), `<system-id>-01.png` (layer 1, far/faint), `-02`
  optional — with a cluster-wide `default.png` / `default-01.png` fallback
  serving every system; each plate replaces the procedural star layer of the
  same number and rides the same camera-parallax wrap and rate; systems with
  neither override nor default draw the untouched procedural starfield,
  byte-identical to before. The resolved star-plate set is the sky: layers
  without a plate draw nothing once art exists — no leftover procedural dots. Every plate
  is pixel-probed once at load: a transparent sky commits it to its starfield
  layer (over the nebula washes), an opaque-black sky draws it as the bottom
  layer under them. Plates are decoded for the current system only and
  evicted on system change (a two-layer system is ~52 MB decoded). Ships
  with `sys-solace.png` / `sys-solace-01.png` for Starogród; convention in
  `docs/ART.md`.

- **The left navigation rail (EVE-neocom).** Root-overlay chrome beside
  the HUD and module rack, pinned to the left screen edge: one icon per
  openable window, **derived from the Screen spec** — every
  non-transient row becomes exactly one entry carrying its `Icon`,
  `Title` display key and the row's new `Group` field
  (`game/rail.go`), rendered in section order **Ship · Trade ·
  Industry · Station · World · Help & System** with a hairline between
  sections. The two core windows join the Ship section (they are
  kit-owned chrome, not spec rows, and toggle through `Kit.ToggleCore`);
  the Esc menu (transient) and the HUD/module rack (permanent ship
  chrome) have no entries — `screenSpecErrors` now also rejects a
  non-transient row without a rail icon or a known group, and a `Service`
  field names the station service a window needs. A click emits
  `Toggle{Title}` on the ActionSink: the first click opens (a fresh
  mount lands on top of the stack), the second closes. Entries glow
  with the rack's active-cell accent while their window is open, carry
  the structured hover tooltip (icon + title + the row's `key — desc`
  hint), and are **grayed — clicks inert — outside their context**:
  `DockedOnly` in space, `SpaceOnly` while docked, and a
  station-dependent entry (`Service`) at a station lacking that service.
  A slim edge tab collapses the rail to its tab; the state persists as
  `rail_collapsed` in `<save>.settings.json` (omitempty — the
  documented default shape stays two keys, absent = expanded). Geometry
  runs through the scale funnel: the expanded width is
  `uikit.RailInset` (36 design px) and is also the left gutter every
  default window keeps clear — the overview, scanner, log and the
  docked station column all start right of it while their right edges
  stay put, so the composition right of the left column is unchanged.
- **The station services view** (`Station Services`, a `DockedOnly`
  `DockStation` Screen row): the one window docking opens. Built from
  `uikit.NewServices` — the docked-at identity (station name · system),
  an **Undock** button, a **Cargo** button and one button per service
  the station actually offers (Market / Fitting / Industry; absent
  services are hidden, never grayed) — over the contract board, in the
  left column of the docked composition. Every button emits a typed
  intent (`Undock`, `Toggle{Screen}` with the spec's own titles); the
  row is exactly as tall as the strip header it replaces, so the
  column's worst-case height and its clearance over the log are
  unchanged.
- **Resolution-independent UI.** Every visible length — window floors and
  title bars, paddings and gaps, grip bands, icon boxes, tooltip cap,
  starmap, HUD ring/cards, the 21 px type scale — now flows through one
  scale funnel (`uikit.Px` / `uikit.Scale`): the display's device factor
  × a persisted user setting. The framebuffer renders at *device*
  resolution (`Layout` returns window size × device scale) so text
  rasterizes at its final size — crisp on HiDPI instead of
  rendered-small-and-upscaled — while hit tests (resize grips, menu rows,
  `overUI`) keep agreeing with the cursor, which ebiten reports in those
  same Layout units.
- **Settings window** (`P`): a UI-scale stepper (0.75× / 1× / 1.25× /
  1.5× / 2× / 2.5×, applied live — the interface rebuilds against the
  new scale and every window keeps its design-unit placement), a
  fullscreen toggle (`ebiten.SetFullscreen`) and **Reset window layout**.
  Persisted next to the save as `<save>.settings.json`
  (`{"ui_scale": 1.5, "fullscreen": false}`), loaded before the first
  layout, defaulting to 1.0 / windowed when absent or damaged.
- The layout sidecar now stores **design units** (÷ scale on write,
  × scale on read): existing v2 files (written at scale 1, where design
  unit == pixel) load seamlessly and one saved arrangement works at
  every scale, including across a live scale change.

- **Show Info** item inspection: right-click any item anywhere it appears
  (rack, fitting, market, inventory, overview, contract rows) for
  a flat EVE-style context menu — bold **Show Info** first, then the
  surface's actions, grayed-but-visible when currently impossible — and
  one reusable floating `<Name> — Info` window (dragable, resizable,
  non-modal; its rect persists in the layout sidecar but it never opens on
  its own) showing each def's description prose and per-kind content: hull
  construction / slots / survival / capacitor / propulsion / sensors /
  cargo; module activation / kind stats / charges / market; item
  properties / market / refines-to / damage; machine caste dossier /
  behavior / loot. Double-click an item to skip the menu and go straight
  to the window.
- `TestMemoryFootprintPump`: headless 6,000-frame memory regression guard
  over the production Update/Draw pipeline — the live heap must plateau
  (under 10 MB growth between the 3,000- and 6,000-frame marks).
- **Manufacturing (industry).** Two-tier recipes (`content/data/recipes/`,
  `rec-*`): minerals in, ships/modules/charges out. Timed jobs at stations
  with the `industry` service (`CmdStartJob`/`CmdCollectJob`, two
  concurrent slots per station, materials committed up front), the
  **factory** window (`B`, dock-only; surfaces automatically at industry
  stations) with its have/need material column, and per-station **craft
  growth** — `L = min(5, 1 + floor(sqrt(runs)/2))` per recipe family, each
  level 4 % faster and 2 % leaner, decaying one run-equivalent per 3,600
  idle ticks.
- **World metabolism.** Belt state is persistent (`Save.Belts`): rocks
  deplete as they are mined and regrow +1 % of their definition volume per
  600-tick window, capped at it, across save/restore and system reloads.
  Wreck reprocessing reclaims `floor(0.4 × hull-recipe mineral inputs)` on
  a destroyed ship's wreck (the value-added burns off). NPC replacement is
  **market-funded**: guards requisition hull, fit and charges from the
  respawn station's book against a per-faction ledger (`Save.FactionLedger`),
  rats pay belt ore or buy local ore asks; an unfunded requisition re-queues
  instead of spawning nothing-for-free.
- **The Kresy ring.** Four new systems — Rudnik (Charter space), Młynów and
  Kamieniec (the Fringe), Głusza (Unincorporated) — with four new nodes:
  Halda (Rudnik's mining outpost), Huta (Młynów's refinery platform), Doki
  (Kamieniec's shipyard & depot) and Kantor (Głusza's freeport). The map is
  seven systems.
- **Ledger agents.** A 24-agent roster (6 industrialists, 9 traders, 9
  miners — world-gated to `world-default`) with capital, inventory, EWMA
  trade-price beliefs and ZIP margins. Valuation backward-imputs recipe
  inputs to their products' fee-net value (Menger/Wieser fixpoint), quotes
  rest at `V·(1±µ)` with Widrow-Hoff margin updates (β = 0.25, α = 0.05),
  and agents act where the margin is positive. Fills settle every tick,
  the behavior pass every 300 ticks, station upkeep (repair/rearm basket +
  maintenance-fee money sink) every 3,600 ticks.
- **Cross-system and multi-hop convoys.** Traffic routes gain `to_system`
  lanes (the validator checks the gate path); a hauler that hops a gate —
  or whose grid is rebuilt under it — parks as a journey
  (`Save.TrafficJourneys`) with its provenance and destination intact and
  flies again when its system loads next.
- **Haul wishes.** Traders and industrialists append cross-node arbitrage
  wishes whenever two stations' prices differ by more than the round-trip
  fee load (`MaxCost` = spread − fees), exposed via `Sim.HaulWishes()`; the
  fulfillment seam (`Sim.FulfillHaul`) pays the hauling cost from the
  owning agent's free capital and drops the wish — a money sink. Wish-driven
  convoys are live: cargo debited from the agent's stock at spawn, delivered
  as agent-owned asks at the destination at the agent's reservation price,
  coexisting with authored interval lanes — one validated shipment per
  60-tick poll, settled through `FulfillHaul` at 2.5 € per m³ per transit
  leg on delivery (lost shipments drop the wish unpaid).

- Binding lore rulebook (`docs/lore/`): UNIVERSE, FACTIONS, AESTHETIC and
  NAMING — the Far Moravia canon (frontier gold rush for the ores,
  first-wave European heritage, the Old Works machine swarms,
  charter-reach zones, the euro as old-world money, four-origin trait
  matrix). NAMING.md holds the rename registry for every content id; the
  names are author-approved and the re-theme entry under Changed applies
  them.
- Living NPCs: deterministic patrol around their spawn anchor, threat
  response (proximity, damage, fire/mining near their home, intrusion
  within 8 km), security temperament (high-sec skittish, null-sec
  fight-to-the-death) and a pursuit leash with break-off.
- Sensor visibility: the overview shows system landmarks plus everything
  within a 250 km sensor radius — no more system-wide omniscience. The
  scanner window sweeps for anonymous long-range contacts ("Ship ×3").
- Ammo selection (`CmdSetAmmo`, right-click a weapon slot): preferred charge
  with automatic fallback and a clean reset to auto.
- Warp-to-within (`CmdWarpTo{Range}`, Selected Item 0/10/50 km control).
- Overview as a sortable, scrollable table (mouse wheel, sensor-gated
  rows); market, inventory and fitting share the same column table.
- Movable, resizable windows: drag by title bar, resize from any edge or
  corner, click-to-raise, composed overlap-free defaults; your arrangement
  persists in a `.layout.json` sidecar beside the save.
- `entieve describe <id>` content introspection with near-miss suggestions
  (AI-agent tooling).
- Contracts & Standing gameplay loop: data-driven station boards
  (kill / deliver / mine), accept → progress → auto-claim on docking at the
  claim station, standing-gated level-2 boards, payouts calibrated from
  economy values (rat-value multiple, ore premium, hauling margin).
- Drop-in art assets: name a PNG after a content ID in `assets/`
  (hulls/ stations/ asteroids/ backgrounds/) and it renders — missing
  files fall back to the procedural art (docs/ART.md).
- Living universe: NPC convoys haul cargo along data-driven routes (real
  warp profiles), guard patrols respond to incidents by security level,
  markets remember traffic (deliveries drift prices down, ambushes up with
  decay, clamped to security bands), and rats sporadically ambush convoys
  in low-sec/null — the hauler dies (lootable cargo wreck), escapes, or is
  saved by patrols; you can intervene (showcase checkpoints #49–51).
- Warp minimum engagement range (150 km, the EVE rule): `CmdWarpTo`
  refuses closer targets and the Warp button gates with a hint. Orbit and
  warp distances are pure selectors ("Orbit range", "Warp to within")
  with separate action buttons; `Q`/`Z` act at the selectors' values.
- Interface icons from **game-icons.net** (CC BY 3.0 — per-icon author
  credits ship in `uikit/icons/CREDITS.md`, completeness pinned by test):
  overview Kind column, Selected Item header and action buttons, HUD
  target cards, module rack class tiles, service title bars, contract
  rows, scanner contacts and fitting rows — monochrome silhouettes
  tinted from the palette on a 16/20/24 px grid with per-icon optical
  nudges. Consequences: Selected Item minimum width 300→450 px (icon
  buttons), station column 384→394 px (contract titles keep their
  budget).
- Structured hover tooltips: class icon + bold title + dim subtitle +
  sectioned stats with values right-anchored on one column + hint
  footers — shared by the module rack, the Fitting window's module rows
  and the warp-gate hint. Tooltips clamp fully inside the window at every
  screen edge.

- Deterministic simulation core: fixed 60-tick timestep, golden-pinned
  SplitMix64 RNG, minimal deterministic ECS (`core`, `ecs`).
- Data-driven content system: JSON schema for items, ships, modules, NPCs,
  star systems and worlds with full cross-reference validation (`content`).
- EVE-faithful simulation (`sim`): shield/armor/structure with four damage
  types and resists, capacitor recharge curve, turret tracking formula with
  wrecking shots and ammo damage profiles, targeting, warping, mining,
  looting, NPC behavior, station services (repair, market, refine, fitting,
  cargo transfer between holds and the station hangar), stargate travel,
  respawns, escape-pod death respawn with insurer replacement (home
  station across systems), exact
  save/restore with handle reconstruction, state hashing.
- Ebitengine client (`render`, `ui`, `game`): vector ships, starfield,
  brackets, EVE-style overview, ship HUD, module rack, inventory, fitting,
  station services, starmap, combat log, time acceleration (1x-8x).
- Entiverse cluster: three star systems (Solace, Penumbra, Gloam) with
  stations, belts, combat anomalies, stargates, markets and rat spawns.
- Worlds: `world-default` (normal start) and `world-showcase` (test world
  covering every feature).
- Tooling: `entieve run | validate | simulate | version`, tick-scheduled
  command scripts (`script`), headless simulation reports.
- Test suite: unit tests per package, twin-run determinism checks, showcase
  integration suite (`tests`).
- Docs: README, architecture contract, gameplay guide, content-authoring
  cookbook for AI agents.

### Changed

- **Docking opens exactly one window.** The station scene no longer
  force-opens the market/inventory/fitting trio (nor the factory at
  industry stations): `StationScene.OnEnter` opens only the new station
  services view and `OnLeave` closes only it, leaving every
  player-opened window exactly as it was. The trio became ordinary
  windows — the docked composition (`stationLayout` + the reclaim) still
  places them, but only when the player opens them (rail, keys or the
  services view's buttons). The `Screen.ForcedDockOpen` field is
  **deleted**: composed membership now derives from
  `DockPolicy == DockStation` alone, and with nothing force-open on
  dock the `stationPrior` pre-dock-intent machinery went with it —
  `persistOpen`/`applyLayout` record and restore the live state the
  player last saw (the services view's flag is the scene's). The old
  game-built station strip (`buildStationWindow`, `syncStationLabel`,
  `setStrip`/`stShown`) is deleted: its identity line and Undock button
  moved into the services view, its contract board rides the services
  window, and the window mounts through the ordinary panel path.
- **The contract board folds onto the table module** (`widgets.StackRow`,
  `widgets.StackLayout`): the table gains a modest rich-row capability —
  a `TableRow` may carry a **line stack** (`Stack []RowLine`) instead of
  one text per column: each line a run of cells (`RowSeg`, own type-scale
  step, colour and minimum width), the row's `Icons[0]` boxing with line
  1's first cell, and its `RowSpec.Actions` drawn as trailing buttons
  (1–2, `Disabled` grayed and silent — one declaration feeding the
  button, the context menu and the gate alike), with a per-line `Ref`
  that joins the shared installer (right-click menu with the bold-first
  Show Info, strict double-click). Everything else — change-gated
  rebuilds (a rich row's stack and its buttons' labels/gates are
  snapshotted like plain cells, so an identical `SetRows` stays a no-op),
  column fitting, sorting, selection and `TipData` tooltips — applies to
  rich rows as to plain ones. The station contract board now declares
  `TableRow` values and renders them through `widgets.NewStackRow`: the
  bespoke `boardLayout` layouter and the hand-built row widget tree it
  laid out are deleted — the equal-height stack behind them moves to
  `widgets.StackLayout` (the pure `boardRowRects` pin stays as the
  geometry tests' subject), and what survives beside it is only the
  contract-id/button/root handle the board's tests press. The look is
  preserved rect for rect — same window chrome, title line, row order,
  strings (the Accepted marker button and the menu's Accept/Abandon verb
  split included), button gating, objective-label Show Info and
  Accept/Abandon dispatch — pinned by the new `TestBoardOnTableRowRects`
  against the pre-port geometry, with the board's golden strings and
  mouse-flow tests passing byte-untouched.
- **`uikit` split into three modules with an enforced import direction**
  (`uikit/theme`, `uikit/widgets`, `uikit/screens`): the flat package
  became `theme` (visual constants and their plumbing — `Palette`,
  `TypeScale`, the `Px`/`SetScale`/`Scale` funnel, faces and the `Theme`
  seam with its implementation, chrome constants, draw helpers, icon
  maps, display formatters) ← `widgets` (reusable mechanics — table,
  menu, tooltip, rowspec, the `Intent` vocabulary and `commandFor`, the
  `ContentView`/`ActionSink`/`Seams` seam interfaces, the `InfoRef` row
  data and its resolvers) ← `screens` (every panel plus the `Kit` shell,
  the session/`NewEnv` wiring and the window-resize chrome). Each layer
  imports only downward (`screens → widgets → theme`, plus `uikit/icons`
  and `render` as before); `widgets` can never import `screens`, and the
  compiler's acyclicity plus a new parsing test
  (`uikit/widgets.TestImportDirection`) pin the rule. Names that now
  span packages are exported at their new home (e.g. `uikit.Px` →
  `theme.Px`, `theme.ContentsPad`, `widgets.RowRef`, `widgets.OpenRowMenu`)
  and call sites updated; a handful of table/tooltip test seams
  (`…ForTest` hooks, exported layout fields) let the screen suites assert
  widget geometry across the seam. Behaviour is unchanged — same menus,
  tooltips, double-clicks and golden pins, and `TestMarketRunsOnStubView`
  now lives with the screens it proves seam-only.
- **The `Env` god-context dissolved into three narrow seams — `Theme`,
  `ContentView`, `ActionSink`** (`uikit/seams.go`, `uikit/intents.go`).
  Every screen constructor now takes exactly the subset it uses: a
  tooltip-only widget a `Theme`, the HUD/log/scanner/map the theme plus
  the view, and the market, fitting, factory, inventory, rack, table and
  Show Info window the union `Seams`. `ContentView` is the read-only
  surface — the defs catalog plus the observable reads screens made off
  `Env.Sim`/`Env.Content` — so a screen runs against a stub view and a
  recording sink with no simulation anywhere near it
  (`TestMarketRunsOnStubView`); `ActionSink` is `Do(Intent)`: screens
  say *what* (`Buy{Item,Qty}`, `StartJob{Recipe,Runs}`, `WarpTo{Entity}`,
  …; the RowSpec payload retyped from `Do func()` to `Intent`) and the
  game executes it (`Game.Do` → `Kit.RunIntent`, one shared
  intent→command table in `commandFor`), the seam later AI-emitted
  intents will ride. `Env` survives only as the deprecated alias for the
  union of the three seams; `NewEnv`/`Kit.Env` return a `Session` (the
  seams plus the shared context menu and Show Info window hosts and
  tests drive). Behaviour is unchanged — same menus, tooltips,
  double-click, keymap, mounts and sidecars, and each intent dispatches
  to exactly the command its button used to queue.
- **One Screen spec per window** (`game/screens.go`): every window the
  player can open — inventory, fitting, market, factory, scanner,
  starmap, help, log, settings and the Esc menu — is now one
  declarative `Screen` row (id, title, icon, key, design-unit default
  rect + anchor, dock policy, modal, mount rule, sidecar name, sync,
  build) from which the window registries derive: the `panelDef` table
  `syncMounts`/`syncPanels` walk, `KeyMap`'s toggle rows and the help
  overlay text rendered from them, the layout sidecar's name list,
  `placeDefaults`, the docked station trio (composition + the scene's
  force-open set) and the tests' panel label map. Adding a window is one
  row plus its builder — it used to touch eight wiring sites. The table
  is validated when a session starts (`screenSpecErrors`: duplicate
  id/key/action/name, zero title, an unknown icon, help text without a
  key, a transient screen carrying a sidecar name, a station row outside
  the composition, `SpaceOnly`+`DockedOnly` together, …), and the tests
  drive the same rule with deliberately broken rows. Behaviour is
  unchanged except the help overlay's row order: the screen toggles now
  form one block after the world bindings (same rows, same text). Also
  removed the dead `Game.objClick` double-click latch (its only user
  moved to `uikit.RowDouble` in the RowSpec pass above).
- **New typefaces: IBM Plex Sans (display) and IBM Plex Mono (tabular).**
  Turret Road and Source Code Pro are replaced wholesale in
  `render/fonts/` — IBM Plex Sans ships Light/Regular/Medium/Bold, IBM
  Plex Mono a single static Regular — role for role: `render.Face`
  (body), `render.FaceBold` (emphasis), `render.MonoFace` (inventory and
  market table rows). Plex sets ~19 % taller at the same size, so
  `uikit.DefaultTypeScale`'s base moves 21 → 20 px (Small 16.8 → 16,
  Large 27.3 → 26, Tiny 14.7 → 14). Glyph coverage (ASCII, `€`/`¢`,
  `×`/`³`, the Polish/Czech diacritics, the ↑/↓ sort indicators) and
  uniform mono digit advances stay pinned in `render/fonts_test.go`;
  both SIL OFL 1.1 notices ship embedded beside the TTFs.

- **Row interactions declare one seam** (`uikit.RowSpec`): every listed
  row — market, inventory, fitting, rack, overview, factory, the
  contract board and Show Info's chain links — now declares its
  interactions as data (`Ref` for Show Info, `Actions` for its verbs,
  `Tip` for its hover panel), and one installer derives the context menu
  (bold **Show Info** first, `MenuSep`, grayed-but-visible actions), the
  lazy tooltip and strict double-click Show Info from that declaration,
  replacing the five hand-wired surfaces (24 wiring sites). User-visible
  consequences: a second click on a rack cell inside the double-click
  window opens Show Info **without also toggling** the module (the old
  toggle+info dual gesture; a lone click still toggles exactly once),
  right-clicking a row no longer silently moves the window's selection
  (menu actions target the row they were opened on — the market's
  Buy/Sell, fitting's Fit/Unefit and the factory's Start/Collect enqueue
  for the clicked row directly), and a stored ship's fit entry in the
  Fitting hangar now stays grayed instead of arming from a stale module
  selection. Menu shapes, gating rules, tooltip anatomy and the menu's
  world-input latch are unchanged.

- Rack right-click now opens the context menu instead of cycling the
  loaded charge directly; quick charge-cycling is the menu's **Cycle
  charge** entry. World right-click (orbit under cursor) is unchanged.
- Content: NPC caste dossier prose (`NpcDef.Description`) and hull origin
  tags (`ShipDef.Origin`) — both shown by the Show Info window.
- Icon silhouette cache now stores the alpha channel only instead of full
  512×512 white NRGBA bitmaps: 38.0 MiB → 9.5 MiB resident for the same
  pixels (RGB is a constant 255, so the white form is rebuilt per
  size-cache miss).
- Documented the upstream ebitenui v0.7.3 window-remount leak
  (`docs/ARCHITECTURE.md`, "upstream window-remount leak"): ≈5.3 KB leak
  per panel dock/toggle mount cycle — real but slow, deliberately not
  mitigated because hiding-instead-of-unmounting would change input
  gating and draw order.
- **Per-station order books replace market memory.** Every station runs
  its own continuous double auction (never-crossed matching, price-time
  priority, 4-significant-figure price ticks); the old per-system
  "market memory" model (price bands, supply steps, halving decay) is
  gone — the book is the only price former. Player Buy/Sell are
  immediate-or-cancel market orders against the docked station's book; a
  3 % broker fee rides buys at placement, a 2.5 % sales tax rides sells at
  execution, and placed orders escrow wallet or goods in full.
  **Save schema v2** (`Save.Schema` = 2) is a clean break: pre-v2 saves
  are rejected instead of restoring into a different market.
- **Convoys carry real cargo.** Route cargo is sourced at spawn, never
  conjured: a station origin withdraws the units from that station's own
  asks (the weighted price paid is the shipment's cost basis), a belt
  origin draws the persistent belt state (a dry belt ships nothing), and a
  gate origin is the economy's one sanctioned import — the named world
  boundary, undebited supply at the destination. Deliveries post as asks
  on the destination's book one tick under `min(cost basis, best ask)`;
  a destroyed hauler's goods are destroyed with it (lootable wreck, no
  reversal of the origin) and the loss surfaces as an event-only
  risk-premium line, never a price step.

- Far Moravia re-theme of content and docs, landing with the parallel
  content/data and code rename workstreams: every display name follows the
  author-approved `docs/lore/NAMING.md` registry (Solace / Penumbra /
  Gloam → Starogród / Kuźnica / Pustkowie, Haven Rest → Jernborg,
  Penumbra Port → Przystań, the Den → Gniazdo, hulls → Väktare / Kosa /
  Havíř / Furman / Tabor / Custos, Loomfall → Kresy, the rat factions
  collapse into the Old Works — ids stay frozen), money displays as
  `1,234.56 €` / `50¢` instead of `12,345.67 cr`, world-facing zone text
  says Charter space / the Fringe / Unincorporated instead of high-sec /
  low-sec / null-sec, and player death reads as escape pod plus insurer
  replacement. `docs/lore/` is now the named binding canon for content
  naming and voice across README, GAMEPLAY, ARCHITECTURE and
  CONTENT_AUTHORING.
- Typeface: **Turret Road** (SIL OFL 1.1 — six weights ship in
  `render/fonts/` with the license) — Regular for body text, Bold for
  window titles, table headers, buttons and HUD readouts. Table sort
  indicators are ↑/↓ (the family has no triangles).

- UI platform decision: chrome migrates to `uikit/` on **ebitenui v0.7.3**
  (pinned), keeping the EVE-dark theme (parity-tested against the legacy
  palette) and the sim command seam. The evaluation spike shipped: resizable
  draggable windows, elastic overview columns with priority shrink and
  name-last ellipsis, sortable headers, visible-only sensor-gated rows.
- Full uikit port: reusable column `Table` (scroll-container + per-row
  containers — solving ebitenui's column/per-row-styling gaps), Overview,
  Selected Item (context-gated actions), Market, Inventory (hold transfers),
  Fitting (stats/resists/slots + fit/unfit/assemble), HUD (layer arcs,
  capacitor, targets), module rack (cycle progress, stat tooltips,
  right-click ammo cycling), BBCode combat log, scanner sweep, starmap,
  help window.
- Warp travel is a process, not a jump: the align gate plus an
  accel → cruise → decel tunnel profile (distance-scaled cruise, minimum
  warp duration, no teleports) — scrams still cancel mid-warp.
- The game shell hosts uikit end-to-end: all eleven panels, preserved
  keymap, world input gated off chrome, save/load rebuilding the kit;
  integration hooks close the last gaps (world-click selection drives the
  Selected Item panel and overview highlight, chrome mounts on the root
  overlay, Overview/Selected hide while docked, log history survives a
  load). The legacy hand-drawn `ui/` package is retired — palette and type
  scale are pinned by golden-value tests.
- UI typography is ~75% larger and padding +20%, with all chrome metrics
  derived from type and spacing constants (no hand-tuned magic numbers).

### Fixed

- **Gate-queue hold chatter** (`sim/system_traffic.go`): the
  "gate queue full, shipment waits in …" line now fires **once per hold
  episode** — on entering the hold — with the shipment's eventual jump as
  its single follow-up. A held shipment now waits in the journey ledger
  (it stands at its gate; nothing respawns it just to re-hold), so retry
  ticks are silent instead of re-logging roughly every resume window.
  New pacing events follow the same discipline: state transitions only,
  never per-tick or per-retry chatter.

- **NPC fleet burst on first system entry** (`sim/system_traffic.go`): the
  cross-system journey ledger no longer dumps every parked shipment into a
  grid at once. Resume is capped and staggered — at most
  `trafficMaxLive − live haulers` respawns per tick, each ledger entry
  firing on its own stateless `trafficDue` schedule in canonical ledger
  order — so a system first loaded with a deep backlog shows ordinary
  traffic density and the backlog drains at the normal cadence behind it.
  A new per-system gate queue (`trafficMaxParkedJourneys` = 3 ×
  trafficMaxLive) refuses to hop-park past its ceiling: overflow shipments
  hold at their current system with cargo intact and retry the hop on
  later ticks, pushing backlog upstream across the ring instead of
  stacking unboundedly at one destination. Concurrent traffic raiders are
  capped per grid (`trafficMaxRaiders` = 6): an ambush whose raider count
  would break the ceiling does not spawn — the count draw is consumed in
  a fixed order first, keeping twin-run and save/restore determinism —
  while static spawn-group NPCs and patrols never count. All three caps
  are mass-neutral: no cargo is ever dropped, duplicated or created, and
  the caps never emit a lost-shipment event.

- **Window focus, default layout and the rail's front-of-stack seat**
  (three window-system defects, one change-set):
  1. *Click anywhere raises.* Any mouse press inside a window — body,
     title bar or resize chrome, left **or** right button — now raises
     that window to the top of the stack. ebitenui's built-in relayering
     only ever saw title-bar presses (the body's scroll viewport claims
     the input layer over the window container), so the kit re-tops
     through its own remove + re-mount seam instead, and kit windows run
     with `DisableRelayering` so that seam is the single, deterministic
     stacking path. When windows overlap, the **topmost** window under
     the cursor wins (that is the window receiving the click); the raise
     is gated exactly like input is — never through the modal context
     menu's frame, never through the rail's own rect, and never up
     through a mounted modal window. A raise re-tops without moving the
     rect, and pays the documented remount leak only when the stacking
     actually changes.
  2. *Defaults clear the rail gutter.* Every default window rect starts
     right of `uikit.RailInset` — the uikit construction defaults that
     still sat at their pre-rail x (overview 8, Selected Item 8, scanner
     10, log 10, help's fallback 10) are retargeted through the same
     constant, and the game's placement floors (`anchorRect`,
     `centerRect`) now clamp every anchored/centered default past the
     gutter: the help window's centred default landed at x=62 against
     the 63 px gutter at the 1024×700 minimum, and at x=93 vs 94.5 at
     device scale 1.5. Pinned at scale 1 and 1.5, both design sizes, for
     the construction rects and the placed set (user-dragged and
     sidecar-restored rects are not defaults and stay untouched).
  3. *The rail is top chrome.* The navigation rail now mounts through
     `Kit.AddTopContainer`: `Kit.Draw` parks it out of the ebitenui
     window pass (`Visibility_Hide_Blocking` — still laid out, never
     painted), paints it after every window with its hover-tooltip
     windows following it (the context menu remains the absolute topmost
     pixels), and re-adds its input layer on top of the window layers
     each frame — so a window dragged onto the gutter never paints over
     the rail, and clicks in the rail's area always reach the rail.
     Paint priority and input priority now agree; while a modal window
     is up the rail keeps its old under-window input gating. The HUD and
     module rack keep their under-window position (`AddContainer`).

- **Window dragging no longer costs a layout pass per frame**
  (uikit/screens/winchrome.go, uikit/widgets/scrollcontent.go): title-bar
  dragging moved from ebitenui's built-in `WindowOpts.Draggable()` to the
  kit's chrome, translate-only. The built-in drag relocated the window
  through `Container.SetLocation` from `Window.Render`, whose
  `RequestRelayout` re-laid out the whole window subtree — grid, rows,
  anchors, preferred-size re-measures and the scroll content's own
  re-location — on every frame of the drag (`doLayout` cost +0.26 s per
  300-frame drag window, `RowLayout.layout` +0.18 s). The chrome now
  rewrites the subtree's rects in place each frame (containers recurse, a
  TextArea re-lays its private interior, a `ScrollContainer` carries the
  content subtree recorded by the new `widgets.AttachScrollContent`
  stash so `renderContent` computes an equal rect and skips its
  `SetLocation` + `RequestRelayout` of the table body), so no layout
  runs: the kit-draw delta between idle and drag fell from 1.38× (heavy,
  325-row market) and 1.31× (light, log) to ≈1.0×, and the drag-vs-idle
  profile diff for every layout function is now zero. Behaviour is
  unchanged: the grip keeps precedence in corners and fires ResizeEvent
  as before, the title drag reports exactly one `Window.MoveEvent` per
  completed press (including a plain click's unchanged rect, matching
  the built-in handler), contents follow the window in the same frame,
  and the services window (no TitleBar) still never drags. Regression
  pins: the title-drag contract (`TestTitleDragBehavior`), a displaced
  title-bar canary proving no layout pass runs during a translate-only
  drag (`TestTitleDragSkipsRelayout`), an alloc bound on drag frames
  over idle (`TestDragFrameCost`), and the existing per-phase sidecar
  signature assertion (a drag still writes no sidecar).

## [0.3.0] - 2026-10-17

- **Navigation-rail icons centred, cells +75 %, station/market
  playtest batch**: ebitenui lays a graphic button out as
  `[empty label][10 px fixed spacing][graphic]` inside the button's
  padded anchor — a fixed right-offset that hung the rail's and the
  station services row's glyphs over their cells' right edges. Both now
  own their glyph: the house `theme.IconBox` anchored dead centre in an
  anchor-layout cell, so the box sits at `(cell − icon)/2` on both axes
  at every scale (`TestRailIconsInsideCells`,
  `TestSvcIconsInsideCells`, `TestStackRowIconInsideRow`). The rail's
  cell square and its gutter grew 36 → 63 px (+75 %, `RailInset` stays
  the windows' left inset) with the glyph grown to 35; twelve 63 px
  cells outrun the 720 design viewport, so the entry column caps to the
  room under its top inset and wheel-scrolls, clipped like the tables,
  when it overflows (`TestRailColumnCapsAndScrolls`,
  `TestRailGutterAndGlyphLiterals`, and the composition pins
  re-derived to the 63 px gutter: station tuple, board lefts, log
  pocket, scanner, overview/settings). The market order book shows a
  visible scrollbar rail beside its scroll viewport — wheel and drag
  both mirror the table's `ScrollTop` — while the title line, quantity
  steppers and Buy/Sell rows stay exactly where they were
  (`TestMarketOrderBookScrolls`).
- **Inventory/table headers aligned with their cells** (Defect A): right
  -aligned cell text anchored 3× the cell padding left of the column
  edge (ebitenui adds a Text's horizontal padding into its measured line
  width *and* subtracts `Padding.Right` again when drawing End-positioned
  text), so headers sat ~10 px right of their column values. Right cells
  now draw with `Padding.Right = 0`, landing every header label and cell
  on the same x at every scale (`TestHeaderCellAnchorAgreement`).
- **Quantity steppers resize their readout** (Defect B): the market /
  inventory / factory `Qty` readout kept the box measured for its
  previous value (changing `widget.Text.Label` does not invalidate
  layout), so wide values drew under the `+` button. The readout now
  relayouts its row on change and owns a box that fits its value; the
  readout and the HUD wallet render in the tabular mono face.

## [0.2.0] - 2026-10-13

- Per-frame interface garbage churn: panels that are not mounted no longer
  run their `Sync` every frame (a panel Syncs when it mounts, so it still
  draws current data in that same frame), `Table.SetRows` skips the column
  re-measure/re-layout when no cell text or icon changed, and the rack /
  fitting row tooltips build their model lazily when the hover key moves
  instead of every frame. Settled Update-path allocation on the showcase
  pump drops from ≈119 KB/frame to ≈14 KB/frame (−88 %).
- Station contract board change detection compares the model fields
  instead of `fmt.Sprintf("%v", …)` on every docked frame.

## [0.1.0] - 2026-10-12

- Lock-on target cards render again (name, distance, three hp-proportional
  layer bars — they collapsed to invisible 2 px notches).
- Ship HUD rings visibly deplete with shield/armor/hull levels; the root
  cause was repo-wide: straight-alpha colors were fed un-premultiplied to
  the compositor, erasing all dimming (`render` premultiplies now).
- Module rack cells toggle on left click (activate/deactivate, targeting
  the selected entity like the F-keys); right-click ammo cycling unchanged.
- Windows resize from every edge and corner (upstream could only grow
  right/bottom); the log is a regular resizable window (`Shift+L`).
- UI layout survives save and restart (geometry + open state in a sidecar
  written on save and clean quit; corrupted files degrade to defaults).
- Table wheel scrolling direction un-reversed; rows anchor to the top of
  the viewport instead of the bottom.
- Selected Item action buttons no longer overlap the selection; orbit
  (2.5–50 km) and warp-to-within (0/10/50 km) ranges are now selectable.
- Docked station windows tile to the viewport from content minimums
  (market columns fit un-truncated at the design sizes) instead of piling
  up at hard-coded positions; your saved arrangement still wins.
- Overview and log windows resize from every edge and corner like the
  rest (their full-bleed bodies used to swallow the grip presses).
- Warp controls no longer double-fire: picking a range is pure state and
  one Warp button acts at it (the duplicate fixed "Warp to 10 km" button
  is gone).
- Market: buying N hulls delivers N hulls (was: charged for N, delivered
  one); short warps no longer deadlock the drive in the align phase.
- NPC aggro reacts to damage from the player only; responding patrols
  hold station mid-fight; a saved convoy emits an event.
- Review-driven hardening (27 findings, 2 seams): windows restored from a
  saved layout re-clamp on viewport changes (no more off-screen strand),
  quitting while docked no longer reopens service windows in space, Tab
  locks the nearest target, one panel registry / credit formatter / kit
  env, the tests drive the real frame pipeline, and save completeness +
  RNG stream roles are pinned by reflection tests.
- Log content clips and scrolls inside its window (it used to paint
  thousands of pixels past the frame; wheel scrollback was dead). Auto
  tails to the newest line.
- The docked service trio can no longer stack: a saved layout that folds
  windows together while restoring at a smaller size is detected and
  re-tiled. Sidecar format bumped to v2 — existing `.layout.json` files
  reset to defaults once.
- Guard patrols are neutral: they no longer attack the player at close
  range (the ~5 km unprovoked strike); they defend themselves if attacked
  and keep answering convoy ambushes.
- Module tooltips, hover/pressed states, 40 px minimum hit areas,
  right-aligned tabular number columns and soft window shadows.

- The overview no longer reveals every ship, rock and wreck in the system
  from anywhere (EVE-grade sensor tension restored).
- Rats are no longer inert: they patrol, notice intruders and defend their
  belts against miners and shooters.

[Unreleased]: https://github.com/entieve/entieve/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/entieve/entieve/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/entieve/entieve/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/entieve/entieve/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/entieve/entieve/releases/tag/v0.1.0
