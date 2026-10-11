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
the `€1,234.56` / `50¢` money strings, and the zone words Charter space /
the Fringe / Unincorporated). Ids are frozen implementation slugs and stay
as they are even when the slug embeds a retired name.

## Where things live

```
content/data/
├── universe.json            # region, constellation, start_system (NO systems list)
├── items/
│   ├── minerals.json        # min-* IDs
│   ├── ores.json            # ore-* IDs (carry refine_yield)
│   ├── ammo.json            # charge-* IDs with damage (categories: ammo)
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
└── worlds/                  # one world per file, world-* IDs
    ├── default.json  showcase.json
```

## ID naming rules (mandatory)

All IDs are kebab-case strings, cross-referenced everywhere:

| Kind        | Prefix      | Example              | Uniqueness             |
|-------------|-------------|----------------------|------------------------|
| Item        | `min-`, `ore-`, `charge-`, `loot-` | `min-ferrite` | global (all items share one map) |
| Ship        | `ship-`     | `ship-sloop`         | global                 |
| Module      | `mod-`      | `mod-emp-turret`     | global                 |
| NPC (rat)   | `npc-rat-`  | `npc-rat-scuttler`   | global                 |
| NPC (guard) | `npc-guard-` | `npc-guard-patrol`  | global (non-rat NPC loadouts: zero bounty, no loot; traffic patrols) |
| System      | `sys-`      | `sys-solace`         | global                 |
| Celestial   | `cel-`      | `cel-haven-rest`     | **unique within its system** (gate `to_cel` resolves inside the destination system; `cel-gate-penumbra` legitimately exists in both `sys-solace` and `sys-gloam`) |
| Contract    | `contract-` | `contract-solace-cleanup-1` | global            |
| Traffic     | *(no id)*   | keyed by `system`    | one def per system (`traffic/*.json`) |
| World       | `world-`    | `world-showcase`     | global                 |

Never invent new IDs for things listed in `docs/ARCHITECTURE.md`'s registry;
extend around them.

## File format

Every JSON file under `items/`, `ships/`, `modules/`, `npcs/`, `systems/`,
`contracts/`, `traffic/`, `worlds/` may contain **either a single object or
an array of objects**:

```json
{ "id": "min-ferrite", "name": "Ferrite", "category": "mineral", "volume": 0.01, "base_price": 6 }
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
| Distance (grid) | meters          | station at `{ "X": 520000000, "Y": 310000000 }` |
| Distance (starmap) | light-years   | `"position": { "X": 6.2, "Y": -1.8 }` |
| Volume          | m³              | `"volume": 0.01`, cargo `400`  |
| Energy          | GJ              | `"capacitor": 420`, `"activation_cost": 15` |
| Time            | seconds         | `"activation_time": 3`, `"respawn_sec": 90` |
| Speed           | m/s             | `"max_speed": 385`             |
| Mass            | kg              | `"mass": 1200000`              |
| Currency        | euro            | `"base_price": 6`, bounties, wallets (display `€1,234.56` / `50¢`) |
| Angles          | radians         | `"tracking": 0.45`             |
| Fractions       | [0,1) resists, (0,1] yields | `"em": 0.15` |

Positions are `core.Vec2` JSON: `{"X": ..., "Y": ...}` (Y grows downward).

## Market orders: the ship-order convention

`MarketOrderDef` has a single `item` field. An order's `item` resolves in
this order:

1. an **ItemID** (`min-ferrite`, `charge-emp`, ...),
2. else a **ShipID cast to ItemID** (`ship-sloop`) — ships are listed on the
   market this way; the price lives only on the order (`ShipDef` has no price
   field),
3. else a **ModuleID** (`mod-emp-turret`).

Anything that resolves to none of the three is a validation error.

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
      "name": "Cinder Belt",
      "kind": "belt",
      "position": { "X": -900000000, "Y": 700000000 },
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
      "name": "Starogród Gate",
      "kind": "gate",
      "position": { "X": 2000000000, "Y": -1400000000 },
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
  "name": "Cinder Gate",
  "kind": "gate",
  "position": { "X": -2400000000, "Y": 900000000 },
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
| `market orders present but no station exists`         | add a station celestial with `"station": {"services": [...]}` |

## Recipe 2: add a ship

Create `content/data/ships/ship-<name>.json` (one file per hull). Populate
**every** field — zeros are only for `ore_hold` (0 = ore goes to cargo):

```json
{
  "id": "ship-kestrel",
  "name": "Kestrel",
  "class": "frigate",
  "description": "A nimble escort frigate with a strong shield bias.",
  "slots": { "high": 2, "mid": 4, "low": 2 },
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
frigate price 200k–400k euro (put the price on a market order — ships have
no price field); resists each in `[0,1)`; align time = `agility * 10` s, so
0.4 ≈ 4 s for a frigate, 2.4 ≈ 24 s for a hauler. Every hull needs ≥ 1 high
slot unless it is an industrial/hauler.

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
  "description": "Slow heavy turret firing explosive flak; uses light charges.",
  "slot": "high",
  "kind": "weapon",
  "activation_cost": 22,
  "activation_time": 5,
  "weapon": {
    "damage": { "em": 2, "thermal": 4, "kinetic": 5, "explosive": 28 },
    "optimal": 9000,
    "falloff": 7000,
    "tracking": 0.4,
    "ammo_class": "light",
    "shot_time": 5
  },
  "price": 275000
}
```

Rules: passive modules use `activation_cost: 0, activation_time: 0`; active
modules need `activation_time > 0`; `weapon.ammo_class` must match
`ammo_class` of at least one existing `charge`/`ammo` item (or `""` for
ammo-less weapons like `mod-rocket-pod`); `price` 10k–400k for standard mods.

## Recipe 4: add an NPC (and its loot)

Append to `content/data/npcs/rats.json`:

```json
{
  "id": "npc-rat-interceptor",
  "name": "Scuttler",
  "faction": "Old Works",
  "ship": "ship-rat-scuttler",
  "fit": ["mod-rail-turret", "mod-rail-turret", "mod-afterburner"],
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
hull's slot counts (list high-slot modules first, then mid, then low);
`aggro_range`/`orbit_range` in meters; `flee_threshold` is a structure
fraction in [0,1] (0 = never flees); `bounty` in euro (rats: 5k–250k);
each loot row: existing item, `chance` in [0,1], `1 <= min_qty <= max_qty`.
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

Ore (note `refine_yield`: each value in `(0,1]`, **sum ≤ 1**):

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

Ammo (needs `damage` + `ammo_class`; ammo items are category `ammo`):

```json
{
  "id": "charge-thermite",
  "name": "Thermite Charge",
  "description": "Burns hot. Thermal-biased light charge.",
  "category": "ammo",
  "volume": 0.01,
  "base_price": 140,
  "damage": { "em": 2, "thermal": 20, "kinetic": 3, "explosive": 2 },
  "ammo_class": "light"
}
```

`category` ∈ `mineral|ore|ammo|charge|loot`. Prices: minerals 5–60, ores
20–120, ammo tens to hundreds of euro per unit.

## Recipe 6: add a market listing

Orders live on the **system** (`systems/*.json` → `market`), shared by all
stations in that system (the system must have ≥ 1 station):

```json
"market": [
  { "item": "mod-flak-turret", "qty": 15, "price": 275000 }
]
```

- `item`: ItemID, **ShipID cast to ItemID** (e.g. `"ship-kestrel"`), or
  ModuleID — see "Market orders" above.
- `qty`: finite stock, ≥ 1; `price`: euro per unit, ≥ 0.
- Put hull orders at their balance price (frigate 200k–400k) and module
  orders at the module's `price`.

## Recipe 7: add a world / scenario checkpoint

```json
{
  "id": "world-speedrun",
  "name": "Speedrun",
  "description": "A compact challenge world: kill the Gniazdo, dock, repeat.",
  "start": {
    "system": "sys-penumbra",
    "position": { "X": 1500000000, "Y": -800000000 },
    "ship": "ship-sloop",
    "fit": ["mod-emp-turret", "mod-afterburner", "mod-armor-rep"],
    "cargo": [{ "item": "charge-emp", "qty": 100 }],
    "wallet": 750000
  },
  "scenario": [
    {
      "name": "Clear the den",
      "feature": "npc-death",
      "detail": "Warp to cel-penumbra-den, kill all 5 rats; EvtDestroyed x5."
    }
  ]
}
```

Rules: `start.system`/`start.ship` must exist; `fit` modules must exist,
listed **high slots first, then mid, then low**, each class within the hull's
slot counts; cargo items must exist with `qty >= 1`; wallet ≥ 0;
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
      "cargo": { "item": "charge-emp", "qty": 600 },
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
| `routes[].from` / `to` | celestials **of this system**, kind `station`\|`gate`\|`belt`, and different from each other |
| `routes[].hull` | an existing hull (civilian haulers: `ship-npc-hauler`); cargo `qty × volume` must fit its `cargo` |
| `routes[].cargo` | `{ "item": ..., "qty": ... }` — existing item, `qty >= 1` |
| `routes[].interval_ticks` | authored cadence **in ticks** (60 ticks = 1 s), `>= 1`; security scales it (table below) |
| `patrol` | optional wing config: `hulls` (each needs an `npc-guard-*` def — `Content.GuardFor`), `interval_ticks >= 1`, security-scaled |
| `raider` | the local ambush rat class. **Required when `security` is `low`/`null`**; must be an NPC that actually spawns in this system (belt/anomaly spawn groups). Leave unset in high security (ambush chance is 0) |

Lifecycle: spawn at `from` (EvTraffic `departed`) → per-leg ambush roll at
tunnel entry → deliver (`delivered`: exact integer stock + one supply price
step down) or despawn at a gate/belt. Ambushed haulers burn away 20 s and
then attempt the warp-out; scrambler/web-carrying raiders pin them until
the patrol responds or they die. Outcomes: `destroyed` (wreck with the
exact cargo, lootable via `CmdLoot`, plus a risk-premium price step up),
`escaped`, or `saved`.

Validation rejects: unknown/empty system, empty routes, endpoints outside
the system or of the wrong kind, `from == to`, unknown hull/cargo/interval
< 1/qty < 1, cargo volume exceeding the hull, missing raider in low/null,
unknown or non-local raider, patrol hulls without a guard def.

### Traffic calibration contract (the binding tables)

Every constant is pinned by `sim.TestTrafficSecurityMathPinned`,
`sim.TestTrafficSpawnRateBySecurity`, `sim.TestTrafficAttackChanceEmpirical`
and `sim.TestTrafficMarket*`; the shipped TTK/DPS numbers are pinned by
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

**Market drift** — band and step, all derived from σ and the shipment's
value share of its shelf ("turnover" = listed `qty × price` of that item
at that system; a brand-new shelf counts as share 1):

| quantity | formula | high | low | null |
|----------|---------|------|-----|------|
| price band | `0.05 × 4^(1-σ)` (half the 10 % sell cut, widened by insecurity) | ±5 % | ±10 % | ±20 % |
| delivery step `k` | `share × band / 2.5` | 2 % | 4 % | 8 % |
| attack step `k` | `2 × share × band / 2.5` (risk premium) | 4 % | 8 % | 16 % |
| decay | halve distance-to-base every **3600 ticks** (60 s) | — | — | — |

`price' = clamp(base·(1-band), base·(1+band), price·(1±k))` where `base`
is the authored market order's price (else the item's `base_price`); a
step that would leave the band clamps silently. Worked example (shipped):
120 `ore-cordite` (48 each) delivered to Przystań — no shelf yet, so
share = 1, low band = 10 %, k = 1 × 10 %/2.5 = 4 %: 48 → **46.08**
(the exact `EvMarket` "price falls to 46.08" line the showcase observes).
Stock changes are always exact integers (units), never floats.

## Checklist before you finish

1. **`go test ./content/...` is green** — the embedded real data must load
   *and* validate with zero problems.
2. **`go run ./cmd/entieve validate` passes** (CLI wrapper over
   `Content.Validate()`).
3. **IDs unique** per the table above; you reused registry IDs instead of
   inventing near-duplicates.
4. **Cross-references resolve**: gates both ways, fits exist and fit the
   hull (high→mid→low order), belts reference ores, spawns reference NPCs,
   loot/market/cargo reference existing items-or-ships-or-modules,
   weapon `ammo_class` matches some ammo/charge, contract targets/destinations
   are real NPCs/items/stations; traffic endpoints/hulls/cargo/raiders
   resolve inside their system (Recipe 9).
5. **Reachability**: every system reachable from `start_system` via gates.
6. **Balance rules of thumb**: small hull 300–800 total HP; frigates
   200k–400k; modules 10k–400k; minerals 5–60/unit; ores 20–120/unit;
   bounties 5k–250k; resists in [0,1); refine yields in (0,1] summing ≤ 1;
   weapons `shot_time > 0` with non-zero damage; active modules
   `activation_time > 0`; contract rewards inside the Recipe 8 payout
   ratio table; traffic cadences/probabilities/market drift inside the
   Recipe 9 calibration tables.
7. **Every field filled** — no silent zeros where a real value belongs
   (only legitimate zeros: passive `activation_*`, `ore_hold: 0` on
   non-barges, ammo-less `ammo_class: ""`, guard NPCs' `bounty: 0`).
