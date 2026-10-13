# Changelog

All notable changes to Entieve are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Show Info** item inspection: right-click any item anywhere it appears
  (rack, fitting, market, inventory, overview, scanner, contract rows) for
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

### Changed

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

### Fixed

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

### Added

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

[Unreleased]: https://github.com/entieve/entieve/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/entieve/entieve/releases/tag/v0.1.0
