# Entieve Content Authoring

The cookbook for adding game content — written for humans and AI agents.
Everything in Entieve is data-driven: you add JSON files under
`content/data/`, the loader (`content/load.go`) parses them, and the
validator (`content/validate.go`) cross-checks the whole graph. You never
need to touch engine code to add a system, ship, module, NPC, item, or world.

The schema structs in `content/model.go` (and `content/model_npc.go`) are
**authoritative** — when this doc and the structs disagree, the structs win.

**Naming and voice are binding.** Every display name, faction string,
description and log line you author MUST follow `docs/lore/AESTHETIC.md`
(gritty corporate register) and `docs/lore/NAMING.md` (the name registry,
the `1,234.56 €` / `50¢` money strings, and the zone words Charter space /
the Fringe / Unincorporated). That covers the structured lore fields too:
hull `origin` tags are copied from NAMING.md's registry, and NPC
`description` prose MUST read in AESTHETIC's voice — see
[Lore fields](#lore-fields-hull-origin-maker-and-npc-description) below. Ids are
frozen implementation slugs and stay as they are even when the slug embeds
a retired name.

## Where things live

```
content/data/
├── universe.json            # region, constellation, start_system (NO systems list)
├── items/
│   ├── minerals.json        # min-* IDs
│   ├── ores.json            # ore-* IDs (carry refine_yield)
│   ├── ammo.json            # ammo-* rounds: damage + calibre (category: ammo)
│   ├── charges.json         # charge-cap-cell (category: charge)
│   └── loot.json            # loot-* IDs
├── ships/                   # one hull per file, ship-* IDs
├── modules/                 # grouped by category, mod-* IDs
│   ├── weapons.json  tank.json  cap.json
│   ├── propulsion.json  mining.json  ewar.json
├── npcs/rats.json           # npc-rat-* IDs
│   └── guards.json          # npc-guard-* IDs (security patrol loadouts)
├── systems/                 # one system per file, sys-* IDs
│   ├── solace.json  penumbra.json  gloam.json
├── contracts/               # mission-board offerings, contract-* IDs
│   ├── solace.json  penumbra.json
├── traffic/                 # living-universe convoys/patrols, one def per system
│   ├── solace.json  penumbra.json  gloam.json
├── recipes/                 # manufacturing: minerals -> products, rec-* IDs
│   ├── ships.json  modules.json  charges.json
└── worlds/                  # one world per file, world-* IDs
    ├── default.json  showcase.json
```

## ID naming rules (mandatory)

All IDs are kebab-case strings, cross-referenced everywhere:

| Kind        | Prefix      | Example              | Uniqueness             |
|-------------|-------------|----------------------|------------------------|
| Item        | `min-`, `ore-`, `ammo-`, `charge-`, `loot-` | `min-ferrite` | global (all items share one map) |
| Round (ammo) | `ammo-` (artillery: `ammo-art-`) | `ammo-emp-s`, `ammo-art-emp-s` | global; `ammo_class` = round family (`small-autocannon` … `large-artillery`): a calibre AND a gun family. Every round of a family shares one volume |
| Corporation | `corp-`     | `corp-ferrum`        | global (the ten-firm maker roster, `content.AllCorps`; a registered extension of the frozen id list) |
| Ship        | `ship-`     | `ship-sloop`         | global                 |
| Module      | `mod-`      | `mod-autocannon-s`   | global (weapon families: autocannon, artillery — three calibres each) |
| NPC (rat)   | `npc-rat-`  | `npc-rat-scuttler`   | global                 |
| NPC (guard) | `npc-guard-` | `npc-guard-patrol`  | global (non-rat NPC loadouts: zero bounty, no loot; traffic patrols) |
| System      | `sys-`      | `sys-solace`         | global                 |
| Celestial   | `cel-`      | `cel-haven-rest`     | **unique within its system** (gate `to_cel` resolves inside the destination system; `cel-gate-penumbra` legitimately exists in both `sys-solace` and `sys-gloam`) |
| Contract    | `contract-` | `contract-solace-cleanup-1` | global            |
| Traffic     | *(no id)*   | keyed by `system`    | one def per system (`traffic/*.json`) |
| Recipe      | `rec-`      | `rec-ship-sloop`     | global (frozen once shipped) |
| World       | `world-`    | `world-showcase`     | global                 |

Never invent new IDs for things listed in `docs/ARCHITECTURE.md`'s registry;
extend around them.

## File format

Every JSON file under `items/`, `ships/`, `modules/`, `npcs/`, `systems/`,
`contracts/`, `traffic/`, `recipes/`, `worlds/` may contain **either a single
object or an array of objects**:

```json
{ "id": "min-ferrite", "name": "Sideron", "category": "mineral", "volume": 0.01, "base_price": 6 }
```
```json
[ { "id": "min-ferrite", ... }, { "id": "min-nickel", ... } ]
```

Use one object for "one thing per file" (systems, ships, worlds) and arrays
for themed collections (minerals, rats). `universe.json` is the exception:
exactly **one object**, and its `systems` field must be **empty or omitted** —
systems come only from `systems/*.json`.

## Units and conventions

| Quantity        | Unit            | Example                        |
|-----------------|-----------------|--------------------------------|
| Distance (grid) | meters          | station at `{ "X": 830000000000, "Y": 490000000000 }` (AU-scale — see below) |
| Distance (starmap) | light-years   | `"position": { "X": 6.2, "Y": -1.8 }` |
| Volume          | m³              | `"volume": 0.01`, cargo `400`  |
| Energy          | GJ              | `"capacitor": 420`, `"activation_cost": 15` |
| Time            | seconds         | `"activation_time": 3`, `"respawn_sec": 90` |
| Speed           | m/s             | `"max_speed": 385`             |
| Mass            | kg              | `"mass": 1200000`              |
| Currency        | euro            | `"base_price": 6`, bounties, wallets (display `1,234.56 €` / `50¢`) |
| Angles          | radians         | `"tracking": 0.45`             |
| Fractions       | [0,1) resists, (0,1] yields | `"em": 0.15` |

Positions are `core.Vec2` JSON: `{"X": ..., "Y": ...}` (Y grows downward).
System bodies are placed at real-solar-system scale (1 AU = 1.496e11 m):
planets 0.4–2 AU from the star, belts/belt-like anomalies 2–10 AU, stations
1–10 AU, gates 20–40 AU, with separation floors keeping landmarks off each
other's grids — the scheme is described in [../SIM.md](../SIM.md) §7 and
pinned by `content/layout_test.go`, which a system outside the bands fails.

## Lore fields: hull `origin`, `maker` and NPC `description`

Three schema fields carry lore into the UI. Each is a single-line JSON
string on one line each, next to `name`:

| Field | Struct | Shape | Source of truth |
|-------|--------|-------|-----------------|
| `origin` | `ShipDef` (`ships/*.json`) | one lowercase tag: `moravian` \| `polish` \| `nordic` \| `latinate` \| `old-works` | the `origin` column of `docs/lore/NAMING.md` — copy it, never invent |
| `maker` | `ShipDef`, `ModuleDef`, ammo/charge `ItemDef` | one `corp-*` slug (the ten-firm roster) | [NAMING.md — Corporations](../docs/lore/NAMING.md#corporations--manufacturers) / `content.AllCorps` — copy it, never invent |
| `description` | `NpcDef` (`npcs/*.json`) | one plain single-line string, 1–2 sentences | `docs/lore/AESTHETIC.md` voice; caste framing per `docs/lore/FACTIONS.md` |

**`origin`** is the hull's heritage tag from AESTHETIC's naming layers and
trait matrix: exactly one origin per hull, and the Old Works machine hulls
(`ship-rat-*`) are always `old-works` (their castes predate the heritage
tags). Register the hull's row in `docs/lore/NAMING.md` first and copy its
`origin` value. The schema accepts an omitted origin so fixtures and test
worlds stay small, but the validator rejects any tag outside the vocabulary
(`content/validate.go`) and `TestShippedHullsCarryOrigin` requires every
shipped hull to carry one.

**`maker`** is the manufacturer — the corporation that builds the def (the
"Corporation" concept of `CONTEXT.md`). It is required on every manufactured
product: ships, modules and ammo/charge items. Rat hulls (`ship-rat-*`),
ores, minerals and loot stay **unattributed** (no `maker`). The value must
be one of the ten registered slugs; the validator rejects anything else
(`content/validate.go`) and `content/maker_test.go` enforces completeness.
Show Info renders the maker's display name with its emblem
(`uikit/emblems`, keyed by the same slug). Emblems are original artwork:
edit the SVG in `uikit/emblems/svg/` and regenerate (see the note below).

Regenerate the emblems after adding or editing one:

```sh
cd tools/emblemraster && go run . -svg ../../uikit/emblems/svg -out ../../uikit/emblems
```

**`description`** on an NPC is dossier prose: what the machine is and does
from the frontier's working perspective — how it behaves, what it does to a
hull, when it shows up. NOT stat recitation (the UI shows stats
separately), NOT human corsairs: machine castes are framed as equipment,
per AESTHETIC's voice rules, with no epic language and no retired
placeholder names (Solace, Penumbra, Gloam, Haven Rest, Thevek Reach,
Loomfall, Mire Corsairs, Gloam Hierarchy). One or two sentences, same
shape as a ship/module `description`. Shipped NPCs must all have one
(`TestShippedNpcsCarryDescription`, enforced the same way as the contract
title/description rule).

## Market orders: the ship-order convention

`MarketOrderDef` has a single `item` field. An order's `item` resolves in
this order:

1. an **ItemID** (`min-ferrite`, `ammo-emp-s`, ...),
2. else a **ShipID cast to ItemID** (`ship-sloop`) — ships are listed on the
   market this way; a founding ask's price lives on the order, and the
   hull's own `base_price` is the price ladder's last rung when its book
   has no trade and no order,
3. else a **ModuleID** (`mod-autocannon-s`).

Anything that resolves to none of the three is a validation error.

## The market tree: `group`

Every marketable def (`ItemDef`, `ModuleDef`, `ShipDef`) declares **`group`**:
its path in the market window's browse tree, one to three non-empty segments
separated by single slashes. The tree is rendered straight from these paths,
so the whole catalog is navigable without touching engine code.

```json
{ "id": "mod-autocannon-s", "group": "Modules/High/Turrets", ... }
{ "id": "ore-cordite",      "group": "Materials/Ore", ... }
{ "id": "ship-sloop",       "group": "Ships/Frigates", ... }
```

Rules (`content/validate.go`): required on every item, module and hull;
missing, empty, a leading/trailing/doubled slash, or more than three
segments is a validation error. The shipped paths are frozen in
`content/groups_test.go` (`TestShippedMarketTree`) — moving a def between
branches is a deliberate edit of that table, never a free re-home. Current
top-level branches: `Ships`, `Modules`, `Ammunition`, `Materials`, `Loot`
(plus the window's own `All` root). Every hull also carries **`base_price`**
(credits, ≥ 0): the market's last-resort quote for it.

---

## Recipe 1: add a star system

**1. Create `content/data/systems/<shortname>.json`** (full example — a
complete, valid two-celestial system with a gate back to Starogród):

```json
{
  "id": "sys-cinder",
  "name": "Cinder",
  "security": "low",
  "constellation": "Kresy",
  "region": "Far Moravia",
  "position": { "X": 4.0, "Y": 2.5 },
  "palette": {
    "nebula1": "#1a0c0c",
    "nebula2": "#331414",
    "nebula3": "#3d1c14",
    "star": "#ff9a5c"
  },
  "celestials": [
    {
      "id": "cel-cinder-star",
      "name": "Cinder",
      "kind": "star",
      "position": { "X": 0, "Y": 0 },
      "radius": 600000000,
      "colors": ["#ffd0a1"]
    },
    {
      "id": "cel-cinder-belt",
      "name": "Asteroid Belt I",
      "kind": "belt",
      "position": { "X": -700000000000, "Y": 550000000000 },
      "radius": 250000,
      "colors": ["#8f8475"],
      "belt": {
        "asteroids": [
          { "ore": "ore-cordite", "count": 10, "volume": 5500 }
        ]
      },
      "spawns": [
        { "npc_id": "npc-rat-scuttler", "count": 2, "respawn_sec": 120 }
      ]
    },
    {
      "id": "cel-gate-solace",
      "name": "Gate (Starogród)",
      "kind": "gate",
      "position": { "X": 3200000000000, "Y": -2240000000000 },
      "radius": 2500,
      "colors": ["#7fd0ff"],
      "gate": { "to_system": "sys-solace", "to_cel": "cel-gate-cinder" }
    }
  ]
}
```

**2. Make the gate pair reciprocal (both ways).** Add a matching gate to
`content/data/systems/solace.json`'s `celestials` array:

```json
{
  "id": "cel-gate-cinder",
  "name": "Gate (Cinder)",
  "kind": "gate",
  "position": { "X": -3200000000000, "Y": 1200000000000 },
  "radius": 2500,
  "colors": ["#ff9a5c"],
  "gate": { "to_system": "sys-cinder", "to_cel": "cel-gate-solace" }
}
```

Rules enforced by the validator: `to_system` must exist; `to_cel` must exist
**in that destination system**; the destination must contain a gate whose
`to_system` points back at you and whose `to_cel` is *your* gate's ID.

**3. Keep `universe.json` untouched** except when your system should be the
start: systems are discovered from `systems/*.json`, and reachability is
computed by walking gates from `start_system`. Your new system is reachable
as soon as both gate directions exist (start from any reachable system).

**4. Verify:**

```sh
go test ./content/...
go run ./cmd/entieve validate
```

Both must be clean. Typical failures and their fixes:

| Error line                                            | Fix |
|-------------------------------------------------------|-----|
| `systems/sys-cinder: gate destination celestial ...`  | typo in `to_cel`, or the destination file doesn't define it |
| `no reciprocal gate in ... pointing back to ...`      | add/fix the return gate in the other system |
| `systems/sys-x: unreachable from start system ...`    | no gate path from `sys-solace` — chain gates from a reachable system |
| `system-level market block is not allowed ...`        | founding orders belong on the station's own `"station": {"market": [...]}` block |
| `market orders on a station without the market service` | add `"market"` to that station's `"services"` |

## Recipe 2: add a ship

Create `content/data/ships/ship-<name>.json` (one file per hull). Populate
**every** field — zeros are only for `ore_hold` (0 = ore goes to cargo):

```json
{
  "id": "ship-kestrel",
  "name": "Kestrel",
  "class": "frigate",
  "description": "A nimble escort frigate with a strong shield bias.",
  "origin": "nordic",
  "maker": "corp-borealis",
  "slots": { "high": 2, "mid": 4, "low": 2 },
  "high_mounts": [
    { "kind": "turret", "size": "s" },
    { "kind": "launcher", "size": "s" }
  ],
  "grid_classes": { "i": 4, "ii": 2 },
  "cpu_classes": { "ii": 1 },
  "shield": 260,
  "armor": 140,
  "structure": 220,
  "resists": { "em": 0.2, "thermal": 0.3, "kinetic": 0.2, "explosive": 0.15 },
  "capacitor": 380,
  "recharge": 210,
  "max_speed": 420,
  "agility": 0.4,
  "mass": 1100000,
  "sig_radius": 38,
  "cargo": 350,
  "ore_hold": 0,
  "warp_speed": 3.6,
  "max_target_range": 42000,
  "max_targets": 4,
  "scan_resolution": 640
}
```

Balance rules of thumb: small hull total HP (shield+armor+structure) 300–800;
frigate price 200k–400k euro (`base_price` on the hull, and a founding
market order around it); resists each in `[0,1)`; align time = `agility * 10` s, so
0.4 ≈ 4 s for a frigate, 2.4 ≈ 24 s for a hauler. Every hull needs ≥ 1 high
slot unless it is an industrial/hauler.

**Fitting limits are the hull's identity** (`content/fit.go`). `high_mounts`
declares one mount per high slot, in slot order — `kind` `turret|launcher`,
`size` `s|m|l`; a mounted module needs a free mount of its own kind at or
above its size (guns and the mining laser are turrets, rockets/missiles are
launchers). Sizes are a design decision, not a function of hull mass: the
frigate and destroyer carry small mounts, the Marauder medium, the Hierarch
large. `grid_classes` / `cpu_classes` are per-class quotas (`i`, `ii`, `iii`;
omit an axis or a class for zero allowance): fitting a module consumes one
unit of its demanded class on each axis it uses, and a fit is legal while no
class's count exceeds its quota. Quotas must leave every authored fit of the
hull legal (the validator rejects one that does not), and a full rack with no
spare class is the normal budget: the frigate's `{ "i": 5, "ii": 3 }` grid is
exactly its showcase fit.

`origin` must be one of `moravian | polish | nordic | latinate | old-works`
(Old Works machine hulls are `old-works`) — register the hull in
`docs/lore/NAMING.md` and copy the row's origin column, per
[Lore fields](#lore-fields-hull-origin-maker-and-npc-description). The validator
rejects unknown tags; shipped hulls may not omit the field. `maker` must be
one of the ten `corp-*` slugs (rat hulls stay unattributed).

## Recipe 3: add a module

Append to an existing `content/data/modules/<category>.json` array (or add a
new file). `slot` ∈ `high|mid|low`; `kind` must be one of the `Kind*`
constants in `model.go`, and the matching attribute block is **required**:

| kind                | required block | slot (canonical) |
|---------------------|----------------|------------------|
| `weapon`            | `weapon`       | high             |
| `mining_laser`      | `mining`       | high             |
| `shield_boost`, `afterburner`, `stasis_web`, `warp_scram`, `cap_booster` | `boost` | mid |
| `shield_extender`   | `passive`      | mid              |
| `armor_rep`, `armor_plate`, `resist_coat`, `cap_recharger` | `passive` | low |

```json
{
  "id": "mod-flak-turret",
  "name": "Flak Turret",
  "description": "Medium-calibre flak thrower: slow cycle, heavy explosive volley.",
  "maker": "corp-nordvakt",
  "slot": "high",
  "kind": "weapon",
  "mount": "turret",
  "size": "m",
  "grid_class": "ii",
  "activation_cost": 0,
  "activation_time": 7,
  "weapon": {
    "damage": { "em": 2, "thermal": 4, "kinetic": 5, "explosive": 28 },
    "optimal": 30000,
    "falloff": 20000,
    "tracking": 30,
    "ammo_class": "medium-artillery",
    "shot_time": 7,
    "damage_multiplier": 2.75,
    "bay_m3": 0.275
  },
  "price": 300000
}
```

Rules: passive modules use `activation_cost: 0, activation_time: 0`; active
modules need `activation_time > 0`; `weapon.ammo_class` must match
`ammo_class` of at least one existing round (or `""` for ammo-less weapons
like `mod-rocket-pod`); `damage_multiplier` is the gun's share of a loaded
round's damage (`0` reads as `1`, so artillery throws a heavier shot from the
same round than an autocannon); `price` 10k–400k for standard mods.
`weapon.bay_m3` is the gun's round bay (required for every gun with an ammo
class, forbidden on an ammo-less one) and must hold at least one whole round
of its family. Calibrate it as `rounds × round volume` for a sustained-fire
reload near 120 s — the shipped table: small autocannon 0.2 m³ / 40 rounds,
small artillery 0.104 / 13, medium autocannon 0.33 / 33, medium artillery
0.275 / 11, large autocannon 0.594 / 27, large artillery 0.297 / 9; the cap
booster's `boost.bay_m3` is 0.25 m³ (25 cells). Validation rejects a family
whose rounds disagree on volume, a bay under one round, a missing bay, and a
bay on a module that loads nothing.

**Mount and class demand.** `mount` and `size` exist only on high-slot
modules (`mount` `turret|launcher`, `size` `s|m|l`): guns and the mining laser
are turrets, rockets/missiles launchers; omit `size` only for a module that
should take any mount of its kind. `grid_class` / `cpu_class`
(`i|ii|iii`, omit an unused axis) state what one unit of the module costs
against a hull's class quota. The shipped mapping is the convention:

| family | `mount` / `size` | `grid_class` | `cpu_class` |
|---|---|---|---|
| autocannon / artillery (s, m, l) | turret / s, m, l | i, ii, iii | — |
| rocket pod | launcher / s | i | — |
| mining laser | turret / s | i | ii |
| shield boost, armor rep (active tank) | — | ii | — |
| shield extender, armor plate, resist coat (passive tank) | — | i | — |
| afterburner | — | ii | — |
| stasis web, warp scrambler (ewar) | — | i | ii |
| cap booster, cap recharger | — | i | — |

## Recipe 4: add an NPC (and its loot)

Append to `content/data/npcs/rats.json`:

```json
{
  "id": "npc-rat-interceptor",
  "name": "Scuttler",
  "description": "Old Works worker frame: it strips what the swarm can carry off, and it does not care who is standing in the way.",
  "faction": "Old Works",
  "ship": "ship-rat-scuttler",
  "fit": ["mod-autocannon-s", "mod-autocannon-s", "mod-afterburner"],
  "aggro_range": 25000,
  "orbit_range": 5000,
  "flee_threshold": 0.2,
  "bounty": 12000,
  "loot": [
    { "item": "loot-alloy-scrap", "chance": 0.7, "min_qty": 1, "max_qty": 3 },
    { "item": "loot-navdata", "chance": 0.2, "min_qty": 1, "max_qty": 1 }
  ]
}
```

Rules: `ship` must exist; the `fit` must exist module-by-module and fit the
hull's slot counts (list high-slot modules first, then mid, then low) **and
the hull's fitting limits** (mount kind/size and class quotas — the validator
says which rule broke);
`aggro_range`/`orbit_range` in meters; `flee_threshold` is a structure
fraction in [0,1] (0 = never flees); `bounty` in euro (rats: 5k–250k);
each loot row: existing item, `chance` in [0,1], `1 <= min_qty <= max_qty`.
`description` is required on shipped NPCs: 1–2 sentences of dossier prose
about the machine — what it does and how it behaves, no stat recitation,
AESTHETIC's voice, no retired placeholder names (see
[Lore fields](#lore-fields-hull-origin-maker-and-npc-description)).
Machine classes take an Old Works caste name (Scuttler above) while the id
keeps its slug — ids are frozen (`docs/lore/NAMING.md`).
Then reference it from a belt spawn or anomaly spawn (Recipe 1) — NPCs only
exist where spawned.

## Recipe 5: add an item / ore

Append to the matching `content/data/items/*.json`:

```json
{
  "id": "min-vanadium",
  "name": "Vanadium",
  "description": "Light alloy metal used in reactor shielding.",
  "category": "mineral",
  "volume": 0.01,
  "base_price": 28
}
```

Ore (note `refine_yield`: each value in `(0,1]`, **sum ≤ 1**). Refining
runs in whole 100-ore batches, so a batch yields exactly
`floor(refine_yield × 100)` of each mineral — author yields that give whole
minerals per batch:

```json
{
  "id": "ore-embergrit",
  "name": "Embergrit Ore",
  "description": "Rust-red ore from inner-system belts.",
  "category": "ore",
  "volume": 1,
  "base_price": 60,
  "refine_yield": { "min-ferrite": 0.4, "min-nickel": 0.25 }
}
```

Ammo (needs `damage` + `ammo_class`; ammo items are category `ammo`, and
`ammo_class` is the **round family** — `small-autocannon`, `small-artillery`,
`medium-autocannon`, `medium-artillery`, `large-autocannon`,
`large-artillery` — which is both the calibre and the gun family that loads
it. Every round of one family must carry the same `volume`, because a bay's
whole-round count is family-wide. Shipped volumes: small-autocannon 0.005,
small-artillery 0.008, medium-autocannon 0.01, medium-artillery 0.025,
large-autocannon 0.022, large-artillery 0.033 m³. Existing ids are the
autocannon families; artillery uses `ammo-art-<profile>-<size>`):

```json
{
  "id": "ammo-thermite-s",
  "name": "40mm thermite",
  "description": "Burns hot. Thermal-biased round.",
  "maker": "corp-burza",
  "category": "ammo",
  "volume": 0.005,
  "base_price": 140,
  "damage": { "em": 2, "thermal": 20, "kinetic": 3, "explosive": 2 },
  "ammo_class": "small-autocannon"
}
```

`category` ∈ `mineral|ore|ammo|charge|loot`. Prices: minerals 5–60, ores
20–120, ammo tens to hundreds of euro per unit.

## Recipe 6: add a market listing

Founding orders live on the **station** (the station celestial's `station`
block → `market`) — one order book per station, so list them where the
trade should happen (the system must have ≥ 1 station):

```json
"station": {
  "services": ["repair", "market", "refine", "fitting"],
  "market": [
    { "item": "mod-flak-turret", "qty": 15, "price": 275000 }
  ]
}
```

- `item`: ItemID, **ShipID cast to ItemID** (e.g. `"ship-kestrel"`), or
  ModuleID — see "Market orders" above.
- `services`: `content.Services` ids only (`repair | market | refine |
  fitting | industry`); that table (`content/services.go`) owns each id's
  display label, icon and the window its services-row button toggles.
  `just validate` rejects anything else.
- `qty`: finite stock, ≥ 1; `price`: euro per unit, ≥ 0.
- `side`: `"sell"` (an ask — the default when omitted) or `"buy"` (a bid).
- Put hull orders at their balance price (frigate 200k–400k) and module
  orders at the module's `price`.

These are the book's **founding** endowment: finite, depleting with trades
(no infinite restock). Ongoing flow — convoy deliveries, agent orders,
respawn requisitions — is never conjured from data; it comes through the
economy (see `docs/SIM.md`).

### Material seeds: both sides at every market station

Every station that offers the `market` service seeds **both sides of every
ore and mineral** — one ask and one bid per item (`min-*`, `ore-*`). The
market window's item tree lists each of those items as a row whatever the
book holds, so a single missing side reads as a dead market to the player:
a mined hold must be sellable and materials must be buyable at every port.

- Prices stay anchored to `base_price`: the bid sits just under it
  (sideron 5 vs base 6), the ask just over it (sideron 7) — the existing
  style, and the reason a founding book never opens crossed.
- Quantities follow value: tens of thousands for cheap minerals
  (sideron 25,000 bid / 12,000 ask), thousands for ore and the dear
  minerals, hundreds for the rarest. Asks are modest stock; bids are the
  local industry's standing demand.
- Przystań (`cel-penumbra-port`) had no `market` block at all before this
  pass: its freeport now carries the same materials set plus the rounds its
  own raiders need (`TestRespawnAmmoIsOnTheLocalBook`).
- The materials rows live at the end of each station's `market` array in
  two blocks — asks first, then bids, each ascending by item id. The
  validator (`TestMarketStationsSeedBothMaterialSides`) keeps every
  market-service station complete.
- These are **seeds only**: nothing replenishes them. Industrialist agents'
  demand and the player's buying are the only ongoing flow; a system whose
  materials are bought out stays dry until someone sells back.

## Recipe 7: add a world / scenario checkpoint

```json
{
  "id": "world-speedrun",
  "name": "Speedrun",
  "description": "A compact challenge world: kill the Gniazdo, dock, repeat.",
  "start": {
    "system": "sys-penumbra",
    "position": { "X": 530000000000, "Y": -300000000000 },
    "ship": "ship-sloop",
    "fit": ["mod-autocannon-s", "mod-afterburner", "mod-armor-rep"],
    "cargo": [{ "item": "ammo-emp-s", "qty": 100 }],
    "wallet": 750000
  },
  "scenario": [
    {
      "name": "Clear the den",
      "feature": "npc-death",
      "detail": "Warp to cel-penumbra-den, kill all 5 rats; EvDestroyed x5."
    }
  ]
}
```

Rules: `start.system`/`start.ship` must exist; `fit` modules must exist,
listed **high slots first, then mid, then low**, each class within the hull's
slot counts **and its fitting limits** (mounts, class quotas); cargo items
must exist with `qty >= 1`; wallet ≥ 0;
`scenario` checkpoints need `name` + `feature` (kebab-case).
`world-showcase` must always exist with **≥ 10 checkpoints** — it drives
`tests/showcase_test.go` — so when adding a feature, add a checkpoint there
too with a `detail` that says how to observe it (event names, commands).

---

## Recipe 8: add a contract (mission board)

Contracts are the motivation layer: `undock → accept → do the activity →
dock at the claim station → euro + standing`. Each contract lives on one
station's board (`station` = the offering station) and is loaded from
`content/data/contracts/*.json` (object or array, like every other dir):

```json
{
  "id": "contract-solace-cleanup-1",
  "title": "Belt cleanup",
  "kind": "kill",
  "level": 1,
  "min_standing": 0.0,
  "station": "cel-haven-rest",
  "objective": { "target": "npc-rat-scuttler", "count": 1 },
  "reward": { "credits": 40000, "item": null, "standing_gain": 0.1 },
  "description": "The Old Works are shadowing the ore haulers out of the Starogród belt. Cull the Scuttler and dock at Jernborg to collect the bounty office's fee."
}
```

Fields:

| Field | Meaning |
|-------|---------|
| `kind` | `kill` \| `deliver` \| `mine` (exactly three kinds) |
| `level` | board tier, `>= 1` (flavor + doc convention) |
| `min_standing` | player career standing required to accept |
| `station` | offering station celestial ID — the board (must exist, kind `station`) |
| `objective` | per-kind block (below) |
| `reward.credits` | paid to the wallet on claim (must be `>= 0`) |
| `reward.item` / `reward.qty` | optional bonus item granted to the station hangar (`qty >= 1` when set) |
| `reward.standing_gain` | career standing credited on claim |

Objective per kind (the validator rejects anything else):

- `kill`: `{"target": "<npc-id>", "count": N}` — N rats of that exact class
  killed **by the player**; kills of other classes never count.
- `deliver`: `{"item": "<item-id>", "count": <m³>, "destination": "<station cel-id>"}` —
  dock at the destination carrying at least `count` m³ of `item` in the hold
  (count must be a whole number of units: `count / volume` is an integer);
  the cargo is consumed on claim. Destination must exist and differ from
  `station`.
- `mine`: `{"item": "<ore-id>", "count": <m³>}` — `count` m³ of that ore
  mined into the hold while the contract is accepted (transfers don't count).

Lifecycle: `offered` (board) → `accepted` (`CmdAcceptMission`, docked at the
offering station with standing ≥ `min_standing`) → `progress` → `ready` →
`completed` — auto-claimed the moment you dock at the **claim station**
(offering station for kill/mine, `objective.destination` for deliver), which
pays `reward.credits`, grants standing and consumes deliver cargo — or
`abandoned` (`CmdAbandonMission`, again from the offering station). No
expiry; completed/abandoned contracts may be re-accepted (repeatable board
jobs). Refusals emit `EvMission` phase `refused`.

### Payout ratio table (the calibration contract)

Rewards are derived from live economy values, never vibes. The ratio bands
below are pinned by `content.TestContractPayoutRatios`; future mission
content must land inside them:

| Kind | Reference value | Payout rule | Shipped contracts |
|------|-----------------|-------------|-------------------|
| `kill` | `count × (bounty + Σ chance × avg_qty × base_price)` per target (bounty + expected loot) | **2.0–3.0 × reference** | cleanup-1: 1 × (7,500 + 7,350) = 14,850 → **40,000** (2.69×); den-hunt-1: 1 × (65,000 + 31,125) = 96,125 → **240,000** (2.50×) |
| `mine` | `count(m³) × ore base_price` | **1.9–2.1 × reference** (2.0× ore premium) | cordite-1: 400 × 48 = 19,200 → **38,400** (2.00×); lucidite-1: 500 × 85 = 42,500 → **85,000** (2.00×) |
| `deliver` | `units × item base_price`, `units = count(m³) / volume` | **1.5 × cargo value + 50,000 × gate jumps** (offer system → destination system) | supply-run-1: 200 × 120 = 24,000, 1 jump → 1.5 × 24,000 + 50,000 = **86,000** |

Standing conventions: level-1 boards carry `min_standing: 0.0` and
`standing_gain: 0.10`; level-2 boards gate at `min_standing: 0.2` (two
level-1 completions) and pay `standing_gain: 0.15`. Expected loot is
`chance × (min_qty+max_qty)/2 × base_price` summed over the rat's loot
table.

## Recipe 9: add NPC traffic (the living universe)

Convoy routes, security patrols and rat ambushes are data-driven per
system. Create `content/data/traffic/<system>.json` (a single object or an
array, like every other dir — the def is **keyed by its `system` field**,
no separate id):

```json
{
  "system": "sys-solace",
  "routes": [
    { "from": "cel-haven-rest", "to": "cel-gate-penumbra",
      "hull": "ship-npc-hauler",
      "cargo": { "item": "ammo-emp-s", "qty": 600 },
      "interval_ticks": 5400 },
    { "from": "cel-gate-penumbra", "to": "cel-haven-rest",
      "hull": "ship-npc-hauler",
      "cargo": { "item": "ore-cordite", "qty": 120 },
      "interval_ticks": 5400 }
  ],
  "patrol": { "hulls": ["ship-npc-patrol"], "interval_ticks": 9000 },
  "raider": "npc-rat-marauder"
}
```

Fields:

| Field | Meaning |
|-------|---------|
| `system` | the system this def configures (must exist; one def per system) |
| `routes` | ≥ 1 one-way convoy legs. A hauler spawns at `from`, warps the leg through the normal align/cruise/decel profile, **delivers** `cargo` at `to` when `to` is a station, and despawns when `to` is a gate (leaves the system) or belt |
| `routes[].from` / `to` | celestials (of this system when the leg is in-system), kind `station`\|`gate`\|`belt`, and different from each other |
| `routes[].to_system` | optional: the destination system of a **cross-system lane** (empty = this def's own system). `to` must then exist in that system, and the validator checks there is a **gate path** between the two. Cross-system hauls hop the gate and continue from the journey ledger (they survive save/restore and grid rebuilds) |
| `routes[].hull` | an existing hull (civilian haulers: `ship-npc-hauler`); cargo `qty × volume` must fit its `cargo` |
| `routes[].cargo` | `{ "item": ..., "qty": ... }` — existing item, `qty >= 1` |
| `routes[].interval_ticks` | authored cadence **in ticks** (60 ticks = 1 s), `>= 1`; security scales it (table below) |
| `patrol` | optional wing config: `hulls` (each needs an `npc-guard-*` def — `Content.GuardFor`), `interval_ticks >= 1`, security-scaled |
| `raider` | the local ambush rat class. **Required when `security` is `low`/`null`**; must be an NPC that actually spawns in this system (belt/anomaly spawn groups). Leave unset in high security (ambush chance is 0) |

Lifecycle: spawn at `from` (EvTraffic `departed`) → per-leg ambush roll at
tunnel entry → deliver (`delivered`: the shipment posts as supply on the
destination station's book, one tick under `min(cost basis, best ask)`) or
despawn at a gate/belt. Ambushed haulers burn away 20 s and
then attempt the warp-out; scrambler/web-carrying raiders pin them until
the patrol responds or they die. Outcomes: `destroyed` (wreck with the
exact cargo, lootable via `CmdLoot`, plus an event-only risk-premium
line), `escaped`, or `saved`.

Validation rejects: unknown/empty system, empty routes, endpoints of the
wrong kind, `to_system` without a gate path, `from == to`, unknown
hull/cargo/interval < 1/qty < 1, cargo volume exceeding the hull, missing
raider in low/null, unknown or non-local raider, patrol hulls without a
guard def.

### Traffic calibration contract (the binding tables)

Every constant is pinned by `sim.TestTrafficSecurityMathPinned`,
`sim.TestTrafficSpawnRateBySecurity`, `sim.TestTrafficAttackChanceEmpirical`
and the book-behavior tests `sim.TestTrafficDeliveriesTickTheBookDown` /
`sim.TestTrafficNoDecayWithoutTrades` / `sim.TestTrafficSoakBooksNeverCross`;
the shipped TTK/DPS numbers are pinned by
`content.TestConvoyTimeToKillPinned`. Future traffic content must land
inside them. The single derived quantity is the **security status σ**:

| `security` | σ |
|------------|---|
| `high` | 1.0 |
| `low`  | 0.5 |
| `null` | 0.0 |

**Spawn cadence** — effective interval = `round(authored × 4^(1-σ))`:

| security | multiplier | authored 5400 | authored 1800 | authored 9000 (patrol) |
|----------|-----------|----------------|----------------|------------------------|
| high | ×1 | 5400 t (90 s) | 1800 t | 9000 t (150 s) |
| low  | ×2 | 10800 t (180 s) | 3600 t | 18000 t (300 s) |
| null | ×4 | 21600 t (360 s) | 7200 t | *no patrols* |

Schedules are stateless hashes (due when `(ticks + phase(key)) mod
interval == 0`); a skipped attempt (live cap: 6 haulers, 4 patrols) never
reschedules anything.

**Ambush probability per transit leg** — `p = (1-σ)/3`:

| security | p | shipped example |
|----------|---|-----------------|
| high | 0 | Starogród convoys are never ambushed |
| low  | 1/6 ≈ 16.7 % | Kuźnica: ≈1 ambush per 6 legs |
| null | 1/3 ≈ 33.3 % | Pustkowie: 1 ambush per 3 legs |

Raiders: `1 + Intn(3)` ships of the def's `raider` class, spawned within
3 km of the ambushed hauler. Flee math: the hauler burns 20 s before its
warp-out, while a raider locks a sig-100 hauler in ≈15 s (scan 440) — so
scramblers/webs bite before the escape attempt and pin it.

**Patrol response** — cadence `round(authored / σ)` (none at σ = 0);
ambush response delay `1800×(1-σ)` ticks, wing `floor(3σ)`:

| security | response delay | wing |
|----------|----------------|------|
| high | 0 (same tick) | 3 ships |
| low  | 900 t (15 s) | 1 ship |
| null | never | 0 |

**Delivery pricing** — the per-station order book is the price former (the
old per-system price band / decay model is **deleted**; there is no drift
to calibrate). A delivered shipment posts as an ask on the destination
station's book at **one tick under `min(cost basis, best ask)`** — cost
basis being the weighted price the origin's asks were withdrawn at, or the
item's `base_price` for belt/gate (import) shipments — so arriving supply
always improves the displayed price, and a posting that crosses the
destination's bids executes immediately at the resting bid (never-crossed
invariant). Stock changes are always exact integers (units), never floats.
A shipment lost in an ambush touches no book at all: it surfaces only as
the event line `Risk premium — <item> shipment lost en route to <station>`
(a supply-risk signal, not a price step). Worked example (shipped): 600
`ammo-emp-s` imported through a gate land as a 600-unit ask one tick under
its base price (`4.999 €` for a `5.00 €` base). The mechanics behind all
of this are documented in [SIM.md](SIM.md).

## Recipes (manufacturing)

Manufacturing is the **second tier of a two-step economy** — the two-tier
rule, enforced by the validator:

| Tier | Input | Mechanism |
|------|-------|-----------|
| 1. Refining | `ore-*` | `refine_yield` on the ore turns it into `min-*` minerals in 100-ore batches at a `refine` station |
| 2. Manufacturing | `min-*` minerals | `recipes/*.json` turns minerals into products at an `industry` station |

Ore never goes straight into a product (recipes take minerals only), and
`loot-*` is never a product — salvage stays salvage. Products are exactly:
ships, modules, and `ammo`/`charge` items. Everything else — ores, minerals,
loot — has no recipe.

Recipes live in `content/data/recipes/` (arrays of objects, like every other
dir; one themed file per product family):

```json
[
  {
    "id": "rec-ship-sloop",
    "product": "ship-sloop",
    "qty": 1,
    "inputs": [{ "item": "min-ferrite", "qty": 20800 }, { "item": "min-nickel", "qty": 3700 }, { "item": "min-silex", "qty": 1500 }],
    "time": 600,
    "family": "hulls"
  }
]
```

| Field | Meaning |
|-------|---------|
| `id` | `rec-` prefix + the product id — **frozen once shipped**, like every ID in the registry table above |
| `product` | the ItemID / ShipID / ModuleID produced. Resolves like a market order: item first, then ship, then module. Unknown product, or a product in category `ore`/`mineral`/`loot`, is a validation error |
| `qty` | units produced per run (1 for hulls and modules; charge runs are batched — shipped batches are 100) |
| `inputs` | minerals only (`min-*` ids, category `mineral`), each `item` + `qty` in whole units |
| `time` | seconds per run at the craft-L1 baseline (each craft level above L1 cuts 4 %): **~30 s** for charges, **~3 min** for modules, **~10 min** for hulls (the approved per-family ladder; runs multiply linearly) |
| `family` | the station craft-growth key (`CraftLevel` in `sim`): `hulls` \| `modules` \| `charges` |

v1 ships **exactly one recipe per product**, but the schema is a list:
extra `rec-*` rows for the same product are legal data when the crafting
sim grows multi-recipe support. Recipe iteration is canonical — always
`Content.RecipesInOrder()`, sorted by ID (pinned by
`content.TestRecipeIterationIsCanonical`).

### The envelope (binding)

Material cost per produced unit — `Σ(inputs.qty × mineral base_price) ÷ qty`
— must land inside **55–75 % of the product's reference price**:

- item products: `ItemDef.base_price`;
- modules: `ModuleDef.price`;
- hulls: the shipped market order price, else the hull's `base_price`; hulls
  that ship no order use the pinned reference table in
  `content/balance_test.go`.

Worked example (the `rec-ship-sloop` above): `20800 × 6 + 3700 × 14 +
1500 × 34 = 227,600` against `ship-sloop`'s order price `350,000` →
**65.0 %**, inside the band. `content.TestRecipeMaterialCostEnvelope` fails
with the actual percentage when a recipe drifts out.

Rounding: whole mineral units for charges/modules, hundreds/thousands for
hull inputs. Two monotonicity rules complete the envelope, both test-pinned:
every **hull** recipe costs more mineral value than every **module** recipe
(`TestHullRecipesCostMoreThanModules`), and the hull ladder is strictly
increasing — a barge burns more minerals than a sloop
(`TestHullRecipeMaterialLadder`).

### `family` vocabulary

`hulls` | `modules` | `charges` — deliberately tiny. The family is the
craft-growth key a station's `CraftLevel` hangs off (`docs/SIM.md`,
"world metabolism"), so it groups by how the product is built, not by what
it does. Adding a family is a schema
change (the `Family*` constants in `content/model.go`), not free-form data;
the validator rejects anything outside the three.

### Binding voice

Recipes carry **no display name of their own** — the UI shows the
product's registered name (`ShipDef`/`ModuleDef`/`ItemDef` `name`, all in
`docs/lore/NAMING.md`'s registry). If a recipe label, log line or progress
string is ever authored for a recipe, it falls under the same binding rules
as every other display string: `docs/lore/AESTHETIC.md`'s voice, the zone
words from `docs/lore/NAMING.md`, and never a retired placeholder name.

Manufacturing runs at stations offering the `industry` service (the closed
set lives in `content/validate.go`: `repair | market | refine | fitting |
industry`). Shipped, Przystań (`cel-penumbra-port`, the Kuźnica freeport),
Huta (`cel-huta`, Młynów) and Doki (`cel-doki`, Kamieniec) carry it.

Every job commits its minerals up front and costs a fee of **3 % of the
recipe's mineral value at base prices**, per run, debited from the wallet
when it starts (`Sim.JobFee`; the Manufacturing window quotes it). All
stations share **three global job slots**; cancelling a running job returns
the materials to the hangar and keeps the fee. The four-minute and
ten-minute families are deliberate pacing, not a per-recipe knob: keep a
new module near 180 s and a new hull near 600 s so the ladder reads at a
glance.

Deliver contracts may demand **module ids** as cargo (modules trade as
whole units, no volume), which is how the manufacturing contracts
(`contracts/manufacturing.json`) ask for manufactured goods; ammo and
weapon ids are not stable targets for them.

## Checklist before you finish

1. **`go test ./content/...` is green** — the embedded real data must load
   *and* validate with zero problems.
2. **`go run ./cmd/entieve validate` passes** (CLI wrapper over
   `Content.Validate()`).
3. **IDs unique** per the table above; you reused registry IDs instead of
   inventing near-duplicates.
4. **Cross-references resolve**: gates both ways, fits exist and fit the
   hull (high→mid→low order, mounts and class quotas), belts reference ores, spawns reference NPCs,
   loot/market/cargo reference existing items-or-ships-or-modules,
   weapon `ammo_class` matches some ammo/charge, contract targets/destinations
   are real NPCs/items/stations; traffic endpoints/hulls/cargo/raiders
   resolve inside their system (Recipe 9); every recipe product resolves,
   its inputs are minerals, and every ship/module/ammo/charge product is
   covered by at least one recipe
   [Recipes (manufacturing)](#recipes-manufacturing).
5. **Reachability**: every system reachable from `start_system` via gates.
6. **Balance rules of thumb**: small hull 300–800 total HP; frigates
   200k–400k; modules 10k–400k; minerals 5–60/unit; ores 20–120/unit;
   bounties 5k–250k; resists in [0,1); refine yields in (0,1] summing ≤ 1;
   weapons `shot_time > 0` with non-zero damage; active modules
   `activation_time > 0`; contract rewards inside the Recipe 8 payout
   ratio table; traffic cadences/probabilities inside the
   Recipe 9 calibration tables; recipe material cost inside the 55–75 %
   envelope with hull recipes costing more than module recipes
   [Recipes (manufacturing)](#recipes-manufacturing).
7. **Every field filled** — no silent zeros where a real value belongs
   (only legitimate zeros: passive `activation_*`, `ore_hold: 0` on
   non-barges, ammo-less `ammo_class: ""`, guard NPCs' `bounty: 0`).
   Every shipped hull carries an `origin` from the vocabulary and every
   shipped NPC a `description` (both test-enforced — see
   [Lore fields](#lore-fields-hull-origin-maker-and-npc-description)), and
   every manufactured def (ships, modules, ammo/charge items) a `maker`
   from the ten-firm roster (rat hulls, ores, minerals and loot stay
   unattributed — `content/maker_test.go`).
