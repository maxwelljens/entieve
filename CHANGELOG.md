# Changelog

All notable changes to Entieve are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
