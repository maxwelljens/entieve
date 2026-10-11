# Changelog

All notable changes to Entieve are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

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

### Changed

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
- Module tooltips, hover/pressed states, 40 px minimum hit areas,
  right-aligned tabular number columns and soft window shadows.

### Fixed

- The overview no longer reveals every ship, rock and wreck in the system
  from anywhere (EVE-grade sensor tension restored).
- Rats are no longer inert: they patrol, notice intruders and defend their
  belts against miners and shooters.

### Added

- Deterministic simulation core: fixed 60-tick timestep, golden-pinned
  SplitMix64 RNG, minimal deterministic ECS (`core`, `ecs`).
- Data-driven content system: JSON schema for items, ships, modules, NPCs,
  star systems and worlds with full cross-reference validation (`content`).
- EVE-faithful simulation (`sim`): shield/armor/structure with four damage
  types and resists, capacitor recharge curve, turret tracking formula with
  wrecking shots and ammo damage profiles, targeting, warping, mining,
  looting, NPC behavior, station services (repair, market, refine, fitting,
  cargo transfer between holds and the station hangar), stargate travel,
  respawns, clone-bay death respawn (home station across systems), exact
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
