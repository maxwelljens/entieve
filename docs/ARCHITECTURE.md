# Entieve — Architecture

> *Entieve* is a single-player 2D space sandbox: a love letter to EVE Online.
> Top-down star systems, EVE-style ship fitting (high/mid/low slots), shield /
> armor / structure with four damage types, capacitor warfare, turret tracking
> math, mining, hauling, loot, NPC rats with bounties, stations with repair /
> market / refine / fitting services, stargate travel across a star cluster,
> and a proper overview UI.

This document is the **binding contract** between all workstreams. Every
package compiles against the interfaces described here.

## Module

```
github.com/entieve/entieve
```

Go 1.24+. Dependencies: `github.com/hajimehoshi/ebiten/v2` (rendering/input),
`golang.org/x/image` (font rasterization). Everything else is standard
library. **Unit tests use only the standard `testing` package.**

## Package layout

| Package       | Responsibility |
|---------------|----------------|
| `core/`       | Deterministic math (`Vec2`) and PRNG (`RNG`, SplitMix64). |
| `ecs/`        | Minimal deterministic ECS: `World`, `Entity`, `Store[T]`. |
| `content/`    | Data schema (`model.go`), the station service vocabulary (`services.go`: ids, labels, icons, windows), JSON loading (`load.go`), cross-reference validation (`validate.go`), embedded game data (`data/`). |
| `sim/`        | The deterministic game simulation: components, commands, events, systems. Headless — never imports Ebitengine. |
| `render/`     | Draws `sim` state: starfield, grid, ships (vector art), brackets, effects. |
| `uikit/`      | The UI platform (ebitenui v0.7.3): EVE-style windows — overview, HUD, module rack, selected item, inventory, fitting, manufacturing, refinery, station services, starmap, log — plus the Show Info window (`infowin.go`), the flat context menu (`menu.go`) and the icon set (`icons/`). The legacy hand-drawn `ui/` package is retired. |
| `game/`       | Ebiten `Game` implementation, scene stack (menu / space / station / starmap), input→`sim` commands, camera, time acceleration, save/load wiring, the manufacturing panel wiring. |
| `cmd/entieve` | CLI entrypoint: `run` (play), `validate` (content lint), `simulate` (headless run with report), `version`. |

Dependency direction (never invert):

```
cmd/entieve → game → {render, uikit} → sim → content → (core, ecs)
                          └────────────→ content
```

`sim` is fully playable/testable with no window: an AI agent drives the game
by issuing `sim` commands and reading `sim` events. This is the *engine
controllability seam*.

## Time model

- Fixed timestep: `sim.TickRate = 60` ticks/second, `Sim.Step()` advances
  exactly one tick (`1/60 s` of sim time).
- All randomness goes through `*core.RNG` seeded per `Sim`. Two sims built
  from the same content/world/seed and given the same command sequence must
  produce identical state — verified by `Sim.Hash()` in tests.
- The client accumulates real time and calls `Step` accordingly; time
  acceleration (1×/2×/4×/8×) simply runs more `Step` calls per frame.
- No floating-point wall-clock reads anywhere inside `sim`.

## Coordinate model

- 2D world space in **meters**, `core.Vec2`. Y grows downward (screen space),
  angles in radians, `0 = +X` (`Vec2.Angle` convention).
- Each star system is one continuous "grid": positions are per-system, e.g. a
  star at the origin, planets at `1e9–1e10 m`, belts and stations at
  `1e8–1e10 m` scale. Warp drive crosses these distances quickly; sublight
  is meters/tick.
- Starmap coordinates (`SystemDef.Position`) are a separate 2D plane
  (light-years) used only by the map screen.

## `core` and `ecs`

See package docs. Key invariants:

- `ecs.Store[T]` iterates in **component insertion order** (deterministic).
  `Each` iterates a snapshot: creating/destroying entities mid-iteration is
  safe.
- `core.RNG` (SplitMix64) is **stable API** — golden tests pin its stream.
  `Fork(stream)` derives independent streams (e.g. one per NPC).

## `content` data schema (`content/model.go`)

All game data is JSON under `content/data/`, embedded with `go:embed`, loaded
by `content.Load(fsys fs.FS) (*Content, error)` and validated by
`(*Content).Validate() error` (aggregated multi-error). IDs are
kebab-case strings, globally unique per kind, and cross-referenced. Display
names and prose are canon, not free text: `docs/lore/NAMING.md` binds every
name (ids stay frozen), money and zone words, and `docs/lore/AESTHETIC.md`
binds the voice.

### ID registry (authoritative — content files MUST use these IDs)

**Damage types** (EVE-style four): `em`, `thermal`, `kinetic`, `explosive` —
typed as `content.DamageType` with constants `DmgEM, DmgThermal, DmgKinetic,
DmgExplosive`. `Damage` is a struct `{EM, Thermal, Kinetic, Explosive float64}`;
`Resists` is four fractions in `[0,1)`.

**Minerals** (`content/data/items/minerals.json`):
`min-ferrite`, `min-nickel`, `min-cobalt`, `min-silex`, `min-glacite`,
`min-argon`.

**Ores** (`content/data/items/ores.json`):
`ore-sterngrit` (yields sideron), `ore-cordite` (sideron, nixium),
`ore-lucidite` (nixium, zaffrite, vetril), `ore-glacierite` (cryolith, helion).
Ore defs carry `refine_yield: {"min-ferrite": 0.45, ...}` fractions per m³.

**Ammunition** (`content/data/items/ammo.json`) — six **round families**, one
`ammo_class` per family-calibre, four damage profiles per family
(`emp`/ion EM-biased, `fusion`/high-explosive explosive-biased,
`ap`/armour-piercing kinetic-biased, `plutonium` thermal-biased). The autocannon families (`small-autocannon`,
`medium-autocannon`, `large-autocannon`) are the original `ammo-*-s/m/l` ids;
the artillery families are `ammo-art-<profile>-<size>` (e.g.
`ammo-art-emp-s`). Every round of a family has the same m³ volume —
small-autocannon 0.005, small-artillery 0.008, medium-autocannon 0.01,
medium-artillery 0.025, large-autocannon 0.022, large-artillery 0.033 —
so a bay's whole-round count is family-wide. `charge-cap-cell` (0.01 m³) is
the cap booster's fuel, not gun ammo.

**Loot** (`content/data/items/loot.json`):
`loot-navdata`, `loot-alloy-scrap`, `loot-iced-core`, `loot-hierarch-crest`.

**Ships** (`content/data/ships/*.json`, one file per hull):
`ship-sloop` (fast combat frigate: 3 high, 3 mid, 2 low),
`ship-cutlass` (brawler: 4 high, 3 mid, 4 low),
`ship-barge` (mining: 2 high, 2 mid, 3 low, big ore hold),
`ship-hauler` (industrial: 1 high, 2 mid, 4 low, huge cargo),
`ship-rat-scuttler`, `ship-rat-marauder`, `ship-rat-hierarch` (NPC hulls),
`ship-npc-hauler` (civilian convoy freighter, unarmed),
`ship-npc-patrol` (system security interceptor).

**Modules** (`content/data/modules/*.json`, grouped by category):
- weapons: `mod-autocannon-s/m/l` (fast cycle, high tracking, short reach)
  and `mod-artillery-s/m/l` (slow cycle, low tracking, long reach, heavier
  volley), each loading the rounds of its own family-calibre and carrying a
  m³ round bay (`weapon.bay_m3`, ~40 rounds ≈ 0.2 m³ for the small
  autocannon, calibrated so sustained fire reloads about every 120 s);
  `mod-rocket-pod` (no ammo, explosive rockets, no bay);
- tank: `mod-shield-boost` (active), `mod-shield-extender` (passive raw HP),
  `mod-armor-rep` (active), `mod-armor-plate` (passive raw HP),
  `mod-resist-coat` (passive resist buff);
- cap: `mod-cap-booster` (active, burns `charge-cap-cell` from a 0.25 m³
  cell bay — 25 cells),
  `mod-cap-recharger` (passive recharge buff);
- propulsion: `mod-afterburner` (active speed boost, cap cost);
- mining: `mod-mining-laser`;
- ewar: `mod-stasis-web` (active, slows target), `mod-warp-scram`
  (active, blocks target warp).

**NPCs** (`content/data/npcs/rats.json`):
`npc-rat-scuttler`, `npc-rat-marauder`, `npc-rat-hierarch` — each refs a rat
hull, fit, behavior (aggro range, orbit range, flee threshold), bounty, loot
table. Guard loadouts (`content/data/npcs/guards.json`, prefix
`npc-guard-`) are non-rat NPCs: zero bounty, no loot table, spawned by the
traffic system or as a celestial's static spawn group — `npc-guard-patrol`
(hull `ship-npc-patrol`, registered per patrol hull via `Content.GuardFor`).
That same prefix classifies them as friendly: `NpcDef.IsGuard` gates the
aggro state machine, so guards never *initiate* on the player (the
proximity/intrusion/recent-work triggers are rat-only) — only
player-sourced damage turns a patrol hostile. In high security they do
clear the pirate rats anchored at their own celestial (the belt sweep,
`sim/system_npc.go` / `sim/system_traffic.go`, pinned by
`sim/patrol_belt_test.go`): free rats are engaged first, but the sweep
finishes the belt — a player-touched rat is hunted once it is the last one,
and stragglers are chased off the anchor.

**Systems** (`content/data/systems/*.json`, one file per system; display
names are registered in `docs/lore/NAMING.md` — the ids below are frozen):
- `sys-solace` — **Struvok**, starting system (the Wild Fields,
  Charter space). Star `cel-solace-star`, planet `cel-solace-prime`,
  station `cel-haven-rest` (Jalvrek; services: repair, market, refine,
  fitting), belt `cel-solace-belt` (ore-sterngrit, ore-cordite), gates
  `cel-gate-penumbra` → `sys-penumbra` and `cel-gate-rudnik` → `sys-rudnik`,
  rat spawns (scuttler), founding market orders on `cel-haven-rest`'s book
  (all player ships + core modules + charges).
- `sys-rudnik` — **Rydnok**, Charter space, the ring's ore anchor. Station
  `cel-halda` (Halžek, mining outpost: market, repair), belt
  `cel-rudnik-belt`, gates → `sys-solace` and `sys-mlynow`.
- `sys-penumbra` — **Kvonice**, the Fringe junction (NPCs aggressive).
  Belt `cel-penumbra-belt` (ore-cordite, ore-lucidite), anomaly
  `cel-penumbra-den` (Gnazvor: marauders + boss hierarch, best loot),
  station `cel-penumbra-port` (Hauvnok, freeport: repair, market, refine,
  fitting, industry; hosts the level-2 contract board), gates → `sys-solace`,
  `sys-gloam`, `sys-mlynow`, `sys-kamieniec`.
- `sys-mlynow` — **Młondan**, the Fringe. Station `cel-huta` (Hovžan, refinery
  platform: market, refine, industry), belt `cel-mlynow-belt`, gates →
  `sys-rudnik` and `sys-penumbra`.
- `sys-kamieniec` — **Kamžal**, the Fringe. Station `cel-doki` (Dukšan,
  shipyard & depot: market, fitting, industry), belt `cel-kamieniec-belt`,
  gates → `sys-penumbra` and `sys-glusza`.
- `sys-gloam` — **Pušťov**, Unincorporated dead-end frontier, dangerous.
  Belt `cel-gloam-belt` (ore-glacierite), heavy spawns incl.
  `npc-rat-hierarch`; no station; gates → `sys-penumbra` and `sys-glusza`.
- `sys-glusza` — **Glůšan**, Unincorporated. Station `cel-kantor` (Kanšev,
  freeport: market), belt `cel-glusza-belt`, gates → `sys-kamieniec` and
  `sys-gloam`.

**Universe** (`content/data/universe.json`): region/constellation names,
system list with starmap positions, `start_system: "sys-solace"`.

**Worlds** (`content/data/worlds/*.json`) — playable scenarios:
- `world-default` — normal game start (a `ship-sloop`, modest fit/cargo/wallet at
  `sys-solace` near `cel-haven-rest`).
- `world-showcase` — **the test world**: exercises *every* feature. Starts
  with a `ship-sloop` fitted with one of each module kind, full charge
  stocks, ores + loot in cargo, fat wallet, positioned in `sys-solace` near
  the belt. Its `scenario` block lists feature checkpoints (see
  "Showcase world" below).

**Contracts** (`content/data/contracts/*.json`) — mission-board offerings
pinned to one station (`station` = offering station; kinds `kill`, `deliver`,
`mine`; objectives reference NPC IDs / cargo items / stations, validated):
- `contract-solace-cleanup-1` (kill 1 scuttler), `contract-solace-supply-run-1`
  (deliver 2 m³ ammo-emp-s to `cel-penumbra-port`), `contract-solace-cordite-1`
  (mine 400 m³ ore-cordite) — the `cel-haven-rest` board (level 1, standing 0).
- `contract-penumbra-den-hunt-1` (kill 1 marauder), `contract-penumbra-lucidite-1`
  (mine 500 m³ ore-lucidite) — the `cel-penumbra-port` board (level 2,
  `min_standing` 0.2).
Rewards are calibrated by the payout ratio table in
`docs/CONTENT_AUTHORING.md` ("add a contract").

**Traffic** (`content/data/traffic/*.json`, one def per system, keyed by
its `system` field): convoy `routes` (station/gate/belt endpoints;
`to_system` marks a cross-system lane and the validator checks the gate
path; hull, cargo item+qty, authored
`interval_ticks`), an optional
`patrol` wing (hulls + cadence), and — required in low/null security —
the `raider` rat class that ambushes convoys (must spawn in that system).
Cadences and probabilities derive from the system's security through the
calibration tables in `docs/CONTENT_AUTHORING.md`.

### Struct summary (`content/model.go` is authoritative)

```go
type ItemDef struct {
    ID ItemID; Name, Description string
    Category string            // "mineral" | "ore" | "ammo" | "charge" | "loot"
    Volume float64             // m³
    BasePrice float64          // ISK-analog "euro"
    RefineYield map[ItemID]float64 // ore only, fractions summing to <= 1
    Damage *Damage             // ammo only, applied per shot
    AmmoClass string           // ammo only: round family (small-autocannon …); "" for non-ammo
}
type ShipDef struct {
    ID ShipID; Name, Class, Description string
    Origin string                  // naming-layer heritage: "moravian"|"polish"|"nordic"|"latinate"|"old-works"
    Group string                   // market tree path, e.g. "Ships/Frigates" (1..3 segments)
    BasePrice float64              // credits: the price ladder's last rung for a hull
    Slots struct{ High, Mid, Low int }
    Mounts []Mount                 // high_mounts: one per high slot, {Kind "turret"|"launcher"; Size "s"|"m"|"l"}
    GridClasses, CPUClasses ClassQuota // grid_classes / cpu_classes: {I,II,III int} fit allowance per module class
    Shield, Armor, Structure float64   // raw HP per layer
    Resists Resists                    // per-layer? No: single Resists struct applied to all layers (keep simple), fields EM/Thermal/Kinetic/Explosive in [0,1)
    Capacitor float64; Recharge float64  // capacity (GJ) and peak-ish recharge time constant tau (seconds)
    MaxSpeed float64; Agility float64    // m/s; align time derived: t = tau_align * ln(2) style (see sim)
    Mass float64; SigRadius float64      // m; used by turret tracking formula & warp
    Cargo float64; OreHold float64       // m³ (ore hold 0 = uses cargo)
    WarpSpeed float64                    // AU/s equivalent (m/s in-system: WarpSpeed * 1.496e11)
    MaxTargetRange float64; MaxTargets int; ScanResolution float64
}
type ModuleDef struct {
    ID ModuleID; Name, Description string
    Group string               // market tree path, e.g. "Modules/High/Turrets"
    Slot string                // "high" | "mid" | "low"
    Kind string                // "weapon" | "shield_boost" | "shield_extender" | "armor_rep" | "armor_plate" | "resist_coat" | "cap_booster" | "cap_recharger" | "afterburner" | "mining_laser" | "stasis_web" | "warp_scram"
    Mount string               // high only: "turret" | "launcher" (guns and the mining laser are turrets)
    Size string                // mount size demand: "s" | "m" | "l" ("" fits any mount of its kind)
    GridClass, CPUClass string // class demand on each axis: "i" | "ii" | "iii" | "" (axis unused)
    ActivationCost float64     // GJ per activation (0 for passive)
    ActivationTime float64     // seconds per cycle (0 for passive)
    Passive struct{ ShieldHP, ArmorHP float64; Capacitor float64; Recharge float64; ShieldBoost, ArmorRep float64; ResistBonus Resists; SpeedBonus float64 } // passive modules fill relevant fields
    Weapon *WeaponAttrs        // Damage; Optimal, Falloff, Tracking float64; AmmoClass string (round family); ShotTime float64 (s); DamageMult float64 (0 reads as 1); BayM3 float64 (round bay m³)
    Mining *MiningAttrs        // VolumePerCycle float64 (m³); CycleTime float64
    Boost  *BoostAttrs         // ShieldBoostPerCycle / ArmorRepPerCycle float64; SpeedMultiplier float64; WebStrength float64; ScramDuration float64; CapRestore float64; BayM3 float64 (cap booster cell bay)
}
type NpcDef struct {
    ID NpcID; Name, Faction, Description string // Description: machine-caste dossier prose
    Ship ShipID; Fit []ModuleID
    AggroRange, OrbitRange float64
    FleeThreshold float64      // structure fraction 0..1
    Bounty float64
    Loot []LootDrop            // {ItemID, Chance, MinQty, MaxQty}
}
type CelestialDef struct {
    ID CelID; Name string
    Kind string                // "star"|"planet"|"moon"|"station"|"belt"|"gate"|"anomaly"
    Position core.Vec2; Radius float64
    Gate *GateDef              // {ToSystem SystemID; ToCel CelID}
    Station *StationDef        // {Services []string; Market []MarketOrderDef}
    Belt *struct{ Asteroids []struct{ Ore ItemID; Count int; Volume float64 } }
    Anomaly *struct{ Spawns []SpawnDef; RespawnSec float64 }
    Spawns []SpawnDef          // ambient spawns for any celestial: {NpcID, Count, RespawnSec}
}
type MarketOrderDef       // one FOUNDING order on a station's book: {ItemID or ShipID or ModuleID, Qty, Price, Side "sell"|"buy"}
type SystemDef struct {
    ID SystemID; Name, Security string; Constellation, Region string
    Position core.Vec2        // starmap plane (ly)
    Palette map[string]string // hex colors for rendering
    Celestials []CelestialDef
    // Founding market orders live on StationDef.Market (per station, per
    // book); a system-level Market block is rejected by Validate.
}
type UniverseDef struct { Region, Constellation string; StartSystem SystemID; Systems []SystemDef }
type ContractDef struct {                        // contracts/*.json
    ID ContractID; Title, Kind, Description string // Kind: "kill"|"deliver"|"mine"
    Level int; MinStanding float64                 // board gate (career standing)
    Station CelID                                 // offering station (the board)
    Objective struct{ Target NpcID; Item ItemID; Destination CelID; Count int }
    // kill: Target+Count rats; deliver: Item+Count m³ to Destination;
    // mine: Item (ore) + Count m³ mined into the hold.
    Reward struct{ Credits float64; Item ItemID; Qty int; StandingGain float64 }
}
type RouteCargo struct{ Item ItemID; Qty int }
type RouteDef struct {                            // traffic/*.json
    From, To CelID; ToSystem SystemID; Hull ShipID; Cargo RouteCargo
    // ToSystem names the destination system for a cross-system lane
    // (empty = this def's own system); the validator checks the gate path.
    IntervalTicks int64                           // authored cadence; security scales it
}
type PatrolDef struct{ Hulls []ShipID; IntervalTicks int64 }
type TrafficDef struct {                          // one def per system, keyed by system
    System SystemID; Routes []RouteDef
    Patrol *PatrolDef; Raider NpcID               // raider required when security is low|null
}
type RecipeDef struct {                           // recipes/*.json (rec-* ids)
    ID RecipeID; Product ItemID; Qty int           // minerals in, one ship/module/charge out
    Inputs []struct{ Item ItemID; Qty float64 }
    Time float64                                   // seconds per run at craft L1
    Family string                                  // "hulls" | "modules" | "charges"
}
type WorldDef struct {
    ID WorldID; Name, Description string
    Start struct{ System SystemID; Position core.Vec2; Ship ShipID; Fit []ModuleID; Cargo []CargoStack; Wallet float64 }
    Scenario []Checkpoint      // showcase only: {Name, Feature, Detail string}
}
type Content struct {
    Items map[ItemID]*ItemDef; Ships map[ShipID]*ShipDef; Modules map[ModuleID]*ModuleDef
    NPCs map[NpcID]*NpcDef; Systems map[SystemID]*SystemDef
    Contracts map[ContractID]*ContractDef; Traffic map[SystemID]*TrafficDef
    Universe *UniverseDef
    Worlds map[WorldID]*WorldDef
}
func Load(fsys fs.FS) (*Content, error)      // reads content/data/** JSON, embed.FS by default via content.FS
func LoadDir(dir string) (*Content, error)   // os.DirFS convenience for dev/hot-reload
func (c *Content) Validate() error           // aggregated cross-ref validation
func (c *Content) Item(id ItemID) (*ItemDef, bool) // + Ship/Module/Npc/System/World/Contract accessors
func (c *Content) Board(cel CelID) []*ContractDef   // a station's mission board, sorted by ID
func (c *Content) TrafficFor(id SystemID) (*TrafficDef, bool) // a system's traffic config
func (c *Content) GuardFor(hull ShipID) (*NpcDef, bool)       // npc-guard-* loadout for a patrol hull
```

`Validate()` must catch at minimum: duplicate IDs; unknown references (fit
modules, ship hulls in NPC defs, gate destinations both ways, market items,
loot items, ore refine yields, ammo classes used by weapons, contract rat
classes / cargo items / stations); traffic route endpoints (station/gate/belt
kinds — `to_system` lanes must have a gate path to the destination),
hulls, cargo items (volume must fit the
hull), intervals >= 1, non-empty route lists, raider presence for low/null
security (a rat class spawned in that system), patrol hulls with an
`npc-guard-*` definition; slot overflow
in world fits and NPC fits; fittings that break the **fitting limits** — a
module whose mount kind/size the hull's racks cannot take, and a fit whose
per-class grid/cpu demand exceeds the hull's class quotas (see the seam
below); refine yields outside `[0,1]`; resists outside
`[0,1]`; negative numbers; worlds referencing unknown systems; `world-showcase`
present with a non-empty `scenario`.

### The fitting-limit seam (`content/fit.go`)

Fitting legality is one rule function, not arithmetic: `ShipDef.FitProblem(def,
fitted)` returns the first rule a module would break alongside the modules
already fitted — `RuleMount` (a high-slot module needs a free mount of its own
kind at or above its size) or `RuleGrid`/`RuleCPU` (one unit of its class on
that axis per module, bounded by the hull's `ClassQuota`). It is the only place
the rules live: `content/validate.go`'s `checkFit` runs it over every authored
fit (NPC fits, world starts) and `sim` runs it on both equip paths —
`execFit` (CmdFitModule, refusal logged as a `SevWarning`) and `buildFitting`
(load/assemble/stored fits, returning an error). Nothing numeric is stored:
mounts and quotas live on the defs, and a save carries fitted module ids
(the slots' bays and ammo selections ride the same fitting runtime, schema
3).

## `sim` contract (`sim/model.go`, `sim/api.go`)

See `docs/SIM.md` for the whole-simulation reference (tick anatomy, world
metabolism, ledger agents, price formation); this section is the binding
surface.

Components (plain structs in `Store[T]`):

| Component | Purpose |
|-----------|---------|
| `Transform` | `Pos core.Vec2`, `Facing float64` |
| `Motion` | `Vel core.Vec2`, `Mass`, `MaxSpeed`, `Agility`, `SigRadius`, `SpeedMultiplier` (prop mod), `WebbedUntil` |
| `ShipState` | hull `ShipID`, layer HP: `Shield, Armor, Structure float64` (current), derived maxes/resists cached from fit |
| `Capacitor` | `Current, Capacity, Tau` |
| `Cargo` | `Stacks []CargoStack`, `Capacity`, `OreStacks []CargoStack`, `OreCapacity` |
| `Fitting` | `High, Mid, Low []ModuleID`, runtime `[]ModuleRuntime` (active/cycle state, charges loaded) |
| `Targeting` | `Locks []Lock` (`Entity`, `LockProgress`), `MaxTargets`, `MaxRange`, `ScanRes` |
| `Pilot` | player-controlled flag + current `Command` |
| `NpcAI` | `NpcID`, behavior state machine state, `*core.RNG` fork, home `Pos`, target |
| `Asteroid` | `Ore ItemID`, `Remaining float64` (m³) |
| `Celestial` | `Kind`, `CelID`, `Radius` |
| `Stargate` | `ToSystem`, `ToCel` |
| `Station` | `Services []string` |
| `Docked` | marker: entity is inside station (hidden from grid) |
| `WarpDrive` | warp state machine (align → warp → land) |
| `Wreck` | `Loot []CargoStack`, `BountyFor` |
| `Traffic` | convoy hauler leg: origin/dest cel, cargo item+qty, phase, per-leg ambush roll consumed, warp retry time, live `Attack` state (raiders, flee/response clocks, outcome) |
| `Patrol` | security patrol response state: `idle`/`respond`/`return` + incident hauler handle + warp retry time |
| `Scannable`? | *(not used — reserved)* |
| `Name` | display name string |

Commands (`sim/command.go`) — the AI/player control surface. All player
actions are commands consumed at the start of `Step`:

```go
type Command interface{ isCommand() }
CmdStop, CmdManualVel{Dir core.Vec2, SpeedFrac float64},
CmdApproach{Target}, CmdOrbit{Target, Range}, CmdWarpTo{Target, Range}, // Range>0 lands that far short
CmdAlignTo{Target}, CmdLock{Target}, CmdUnlock{Target},
CmdActivate{Slot SlotRef, Target}, CmdDeactivate{Slot SlotRef}, CmdSetAmmo{Slot SlotRef, Item},
CmdLoot{Wreck, Item ItemID|""}, CmdMine{Asteroid, Slot SlotRef},
CmdDock{Station}, CmdUndock{}, CmdJump{Gate},
CmdBuy{Item, Qty}, CmdSell{Item, Qty},
CmdPlaceOrder{Item, Side, Price, Qty}, CmdCancelOrder{ID uint64}, // standing market orders (docked book)
CmdTransfer{Item, Qty, From, To}, // holds: "cargo" | "ore" | "hangar"
CmdRefine{Item, Qty}, CmdRepair{}, CmdAssemble{ShipID},
CmdFitModule{ModuleID, Slot SlotRef}, CmdUnfitModule{Slot SlotRef},
CmdAcceptMission{ID content.ContractID}, CmdAbandonMission{ID content.ContractID},
CmdStartJob{Recipe content.RecipeID, Runs int}, CmdCollectJob{ID uint64},
CmdCancelJob{ID uint64} // industry (system 17)
```
(`Target ecs.Entity`; module index = flat slot index within its slot class.
The two mission commands validate against the docked station's contract
board — docked check, board check, `min_standing` check — and report
refusals as `EvMission` phase `"refused"`. The three industry commands ride
`deferIndustry` to `systemIndustry` and report refusals as `SevWarning`
"Industry: …" log lines.)

Events (`sim/model.go`) — read via `(*Sim).Events() []Event` (drained each
call) or `DrainEvents()`. Kinds: `EvLog` (combat/mining/system log lines
with severity), `EvDestroyed`, `EvWarp{Phase}`, `EvLooted`, `EvDocked`,
`EvUndocked`, `EvJumped`, `EvMarket`, `EvModule`, `EvTargetLocked`,
`EvCargoFull`, `EvAnomalyCleared`, `EvMission`, `EvTraffic`.
`EvMission{Phase, ID, Name}` reports the contract lifecycle with phases
`"accepted" | "progress" | "ready" | "completed" | "abandoned" | "refused"`
(ID/Name carry the contract ID and title; Value carries the payout).
`EvTraffic{Phase, System, Cel, Item, Value, Text}` reports the
living-universe convoy lifecycle with phases `"departed" | "jumped" |
"delivered" | "attacked" | "destroyed" | "escaped" | "patrol-response" |
"saved"` (System carries the system ID, Cel the route endpoint / incident
celestial, Item/Value the shipment). Book moves print on the existing
`EvMarket` kind with `System`/`Item`/`Value` (new displayed price) and
text `"Surplus — … price falls to …"` / `"Demand — … price rises to …"`;
a shipment lost en route is an event-only signal
(`"Risk premium — …"`), never a price step.

API (`sim/api.go`):

```go
type Sim struct{ /* World *ecs.World; stores…; RNG; Content; time */ }
func New(c *content.Content, w *content.WorldDef, seed int64) (*Sim, error)
func NewSystemless(...) // NOT needed
func (s *Sim) Step()                          // one tick; consumes queued commands first
func (s *Sim) Queue(cmd Command)              // enqueue player/agent command
func (s *Sim) Events() []Event                // drain pending events
func (s *Sim) Time() float64                  // sim seconds
func (s *Sim) Ticks() int64
func (s *Sim) Hash() uint64                   // deterministic state fingerprint (for tests/replays)
func (s *Sim) Player() ecs.Entity
func (s *Sim) System() *content.SystemDef     // current system
func (s *Sim) LoadSystem(id content.SystemID, entryAtCel content.CelID) error // gate jumps / new game
func (s *Sim) Snapshot() (*Save, error)       // full serializable state
func (s *Sim) Restore(*Save) error
```

Read model: `(*Sim).Grid() []Info` lists on-grid entities (docked excluded)
in **entity-handle order**, NOT distance order — it is the name-resolution
surface for commands and scripts, and distance ordering flips as targets
circle each other (two rats named "Scuttler" would swap places mid-fight).
Distance sorting is a presentation concern: the overview sorts its own rows.
`Restore` works on a private deep copy of the save, so several sims may be
restored from one `Save` without aliasing its data.

Visibility: `Info.Visible` is presentation state for the UI — true for system
landmarks (star, planet, moon, station, gate, belt/anomaly markers — kinds
`KindStar..KindAnomaly` incl. `KindBelt`) and for everything within
`SensorRange` (250 km) of the player. Ships, asteroids and wrecks beyond
sensor range stay hidden from the overview (no system-wide omniscience);
`Grid()` still returns them for command/script name resolution.

NPC life: idle rats patrol deterministically around their spawn anchor
(handle-hashed phase — no RNG draws). An NPC aggros when the player enters
its effective aggro range, damages it, fires or mines within 30 km of its
home within 20 s, or comes within 8 km of its home. `SystemDef.Security`
tempers this: high = 0.4× aggro range and flee at ≥35% structure, low =
1.0×, null = 1.3× and never flee. Aggroed NPCs pursue up to 2.5× their
effective aggro range from home before breaking off.

Systems (`sim/` internal, called by `Step` in this order; the full
anatomy lives in `docs/SIM.md`):
1. `command` (apply queued commands to intent),
2. `movement` (accel toward desired velocity per agility; web effect),
3. `warp` (align → warp → land state machine),
4. `targeting` (lock progress by scan res & sig radius),
5. `modules` (cycle timers, activation cost, passive aggregation),
6. `weapons` (turret math below, ammo consumption),
7. `damage` (shield → armor → structure with resists; death → wreck),
8. `capacitor` (recharge curve),
9. `mining` (cycle yields into ore hold; persistent belt-state decrement),
10. `npc` (state machine: idle→approach→orbit→fire→flee; uses Fork'd RNG),
11. `loot` (loot command → cargo; bounty on NPC death → wallet),
12. `economy` (buy/sell/refine/repair/fitting at station; the per-station
    order books; wallet),
13. `travel` (dock/undock/jump: swaps system content),
14. `mission` (contract accept/abandon against the docked station's board,
    folds kill/ore objective observations, auto-claims ready contracts on
    docking at the claim station → wallet + standing),
15. `respawn` (funded NPC replacement timers — requisitioned through books
    and ledgers, see "world metabolism" in `docs/SIM.md`),
16. `traffic` (the living universe: convoy spawn/warp/deliver schedules,
    real-cargo provenance, per-leg ambush rolls, raider and patrol combat,
    cross-system journey parking/resume, book supply),
17. `industry` (manufacturing jobs from recipes, the 3-slot global pool,
    job fees/cancels, craft growth and idle decay),
18. `agents` (the ledger economy: settle resting fills every tick, the
    agent behavior pass on per-agent cooldowns of 60-600 ticks, the station upkeep cycle every
    3600 ticks).

After all systems, the **deferred player death respawn** runs (so a
cross-system clone-bay respawn can rebuild the grid without yanking
entities out from under the systems still running this tick); events are
capped at emit time, so there is no end-of-Step event system.

The **contracts loop** ties the activities together: undock → accept a
contract from a station board → count objectives from the existing flows
(rat kills by class, ore mined into the hold, cargo delivered to the
destination station) → return to the claim station → payout + career
standing (`Save.Standing` / `Save.Missions`, both persisted). Contracts are
repeatable board jobs (v1: no expiry, no once-only claim).

The **living universe** layer (`systemTraffic`, data in
`content/data/traffic/`) runs the three interlocking systems the contracts
economy sits on top of:

- **NPC cargo traffic** — per-system convoy routes spawn haulers
  (`ship-npc-hauler`) on security-scaled schedules, fly one-way legs
  through the normal warp profile (align → tunnel → land), deliver cargo
  at station endpoints and despawn at same-system gates/belts. Cargo is
  **sourced at spawn, never conjured**: a station origin withdraws the
  units from that station's own asks (weighted price = cost basis), a belt
  origin draws the persistent belt state, a gate origin is the economy's
  one sanctioned import (the named world boundary — undebited supply at
  the destination). Cross-system legs (`to_system` lanes) hop gates and
  **multi-hop journeys exist**: a hauler that crossed a gate — or whose
  grid was rebuilt under it — parks as a journey (`Save.TrafficJourneys`)
  with its provenance and destination intact, and flies again when its
  system loads next. Live haulers are capped at 6 per system, patrols at
  4; schedules are stateless hashes (no timers to persist). Haulers and
  patrols are ordinary entities: the player can lock, shoot and loot them
  like anything else.
- **Security patrols** (`npc-guard-*` loadouts on `ship-npc-patrol`) idle
  on a guard circle 20–30 km east of the anchor station (outside the
  8 km NPC guard radius so station traffic never trips them), and respond
  to convoy ambushes: instant 3-ship wings in high security, a single
  responder after 15 s in low security, nothing in null. Responding
  patrols warp to the incident and fight raiders through the normal
  lock/activate/turret/damage pipeline — NPC-vs-NPC combat reuses the
  player pipeline with an explicit activation target (`npcActivate(e, ai,
  target)`); only the *aggro* state machine remains player-centric.
- **Sporadic attacks** — every convoy tunnel entry in the Fringe or
  Unincorporated rolls `p = (1-σ)/3` (1-in-6 Kvonice, 1-in-3 Pušťov, 0
  in Charter space); on a
  hit, 1–3 raiders of the system's `raider` rat class drop in mid-leg
  (renamed `<Name> Ambusher` so name-addressed scripts stay
  unambiguous), the hauler is dragged out of warp and burns away for 20 s
  before attempting its warp-out. Scramblers/webs pin it (lock ≈ 15 s vs
  the 20 s flee), the patrol's arrival pulls the raiders' guns off the
  convoy, and the incident resolves as **destroyed** (wreck with the
  exact cargo, lootable via `CmdLoot`, and the loss prints the event-only
  risk-premium line),
  **escaped** (warps clear and delivers anyway), or **saved** (all
  raiders dead while the hauler lives). Raiding NPCs never queue respawn
  timers against nearby spawn groups.
- **Order-book pricing** — the per-station order books are the only price
  former (the old per-system "market memory" band/decay model is gone).
  A delivery posts the exact units as supply on the destination station's
  book at one tick under `min(cost basis, best ask)`, so arriving supply
  always improves the displayed price; a lost inbound shipment is an
  event-only supply-risk signal ("Risk premium — …"), never a price step.
  All traffic constants are derived/pinned in the calibration tables in
  `docs/CONTENT_AUTHORING.md` ("traffic calibration contract").

**Determinism**: all living-universe randomness (spawn scatter, ambush
rolls, raider counts, patrol scatter) draws from a dedicated traffic RNG
stream forked from the master stream at `New` (`Save.TrafficRNGState`) —
traffic never consumes the player's combat rolls, so existing seed-pinned
outcomes are untouched. Hauler legs/ambushes persist as `Traffic`/`Patrol`
components and `Save.TrafficLeaves` (scheduled raider break-offs); prices
persist in `Save.Market`.

**Save schema v3** (`Save.Schema` = 3, `saveSchemaV3`; `Restore` accepts and
migrates v2 in place — `migrateSaveV2ToV3` clears every bay and selection,
and the bay rule then resolves each gun — and rejects anything older, the
clean break of the per-station market tranche, v0.2.0 era). v3 added the
round-bay state, which rides the existing fitting runtime instead of a new
Save field: each `ModuleRuntime` carries `Clip` (`{Item, Qty}` loaded rounds)
and `Selection` (the round type the bay tops up from, `""` = none/waiting).
The living-economy fields riding the schema: `Market` / `MarketLasts` /
`MarketSeq` (per-station books and the price-time clock), `Belts`
(persistent belt state), `FactionLedger` (respawn requisition money),
`Jobs` / `JobSeq` / `Craft` (manufacturing jobs and station craft
counters), `Agents` / `Upkeep` (ledger agents and station upkeep
ledgers), `TrafficJourneys` (shipments parked between grids). All lists
serialize in canonical order and map keys marshal sorted;
`Sim.Hash()` is FNV-1a over the full canonical Save JSON
(`sim/hash.go`), with no degraded fallback (it panics on a
non-serializable state instead of fingerprinting partial state).
`sim/save_completeness_test.go` classifies every `Sim` field as
save-carried / derived / per-Step transient / documented exception and
fails on unclassified additions.
Saves written before a content-id rename are remapped at restore:
`sim.walkContentIDs` reflects over the `Save` type graph (every
`content.ItemID` / `ModuleID` / `ShipID` / `RecipeID`, through slices,
arrays, map values and nested structs) and applies the one registry
(`content.LegacyIDs`), which is also what `content/validate.go` reads to
reject retired ids — one list, two consumers. An id remap is not a state
change, so it needs no schema bump; `sim/save_migration_test.go`'s
`TestWalkContentIDsCoversEveryField` materializes the whole type graph and
fails on any id field the walk cannot reach, so a new id-bearing field is
migrated without a second hand-written list.

### Turret formula (EVE-faithful)

```
chance = 0.5 ^ ( (angularVel / (tracking * (sigRadius/40000))) ^2
               + (max(0, dist - optimal) / falloff) ^2 )
```
- `angularVel` = transversal speed / distance (rad/s), `sigRadius` in m,
  `tracking` in rad/s per the module def.
- Roll `RNG.Chance(chance)`; on hit, damage multiplier is
  `roll in [0.01, 0.99]`-derived (`damage = volley * (0.01 + roll*0.99)`, roll
  uniform); **wrecking shot** (×3) when the roll lands in the lowest
  `0.01 * chance` tail (classic EVE wrecking mechanic approximated as: if
  `roll < 0.01` always wrecking ×3).
- Ammo modifies the volley's `Damage` mix; dry guns (no charges) do not fire
  and emit `EvLog` once per cycle.

### Capacitor curve (EVE-faithful)

```
dC/dt = (2.5 * Capacity / Tau) * sqrt(C/Capacity) * (1 - sqrt(C/Capacity))
```
Peak recharge ≈ 25% of Capacity. Applied per tick.

### Movement & warp

- Sublight: desired velocity = heading * `MaxSpeed * SpeedMultiplier`
  (`SpeedMultiplier` from afterburner; web multiplies by `(1-strength)` while
  `WebbedUntil > now`). Acceleration limited by `Agility` (EVE-style align):
  `maxDeltaV = MaxSpeed * (dt / alignTime)` with `alignTime = Agility * 10`
  (documented simplification of EVE's `agility * mass` inertia).
- Warp: `CmdWarpTo` → align (rotate/accelerate toward vector) → `WarpDrive`
  accel/decel profile with `WarpSpeed` AU/s → land within 2500 m of target.
  `mod-warp-scram` on the target blocks warp start and cancels warp
  (`EvLog` "warp scrambled").

### Damage

Layer order shield → armor → structure. Each layer has the ship's `Resists`
(single set applied per layer; modules may add `ResistBonus` per layer).
Overflow damage carries to the next layer at full value (documented
simplification of EVE's 50% bleed-through between shield/armor is NOT used —
keep full carry-over, note it in `docs/GAMEPLAY.md`). Structure ≤ 0 → ship
dies: the player's escape pod gets clear and the insurer issues a
replacement — a fresh `ship-sloop` handed over at the last docked station,
or the home station in the universe's start system when the current system
has none (insurance paperwork, not clone tech; `EvDestroyed` is
emitted, cargo is lost, wallet and hangar keep). NPC → `Wreck` with rolled
loot + bounty credited to killer's wallet.

### Showcase world (`world-showcase`)

`docs/CONTENT_AUTHORING.md` lists the checklist. Every feature below must be
exercised by `tests/showcase_test.go` (integration, headless):
fitting (all module kinds), activation/cycles, ammo consumption + dry gun,
turret hit/miss/wrecking, resist damage across all 4 types, capacitor drain
and recharge, cap booster, shield/armor tanking + repair, afterburner,
stasis web + warp scram, targeting/locking/unlocking, warp (start/cancel/
land), sublight approach/orbit/manual piloting, NPC AI (aggro/orbit/flee/
death), bounty payout, wreck looting, mining + asteroid depletion + ore hold,
cargo transfer, gate jump both directions, dock/undock, station repair,
market buy/sell, refine, ship assembly/swap, module fit/unfit, respawn
timers, save/restore round-trip, determinism (Hash equality of twin runs),
contract accept → objective progress → auto-claim payout + standing,
NPC traffic delivery + destination-book supply posting, ambush on NPC traffic
(Kvonice legs, showcase seed), security patrol response + convoy escape.

> **Sim hardening registry (2026-10, `fix/sim-hardening`).** Append-only
> entries for the contract changes made by the fragility review:
>
> - **Events:** `EvTraffic` gains phase `"saved"` (`TrafficSaved` in the
>   `sim/model.go` registry — the same string as `TrafficOutcomeSaved`),
>   emitted exactly once when an ambush resolves with every raider gone and
>   the hauler alive: `System` = current system, `Cel` = nearest landmark of
>   the incident, `Item`/`Value` = shipment, `SevGood`, text
>   `"Convoy hauler saved near …"`.
> - **Save schema:** `Save.LastStationCel` (`last_station_cel`) always carries
>   the respawn anchor as a celestial ID, so an anchor in another system
>   (handle `LastStation` = 0) survives save/restore; restore prefers the
>   celestial ID and falls back to the `LastStation` handle for saves written
>   before the field. `SavedNpcAI` gains `last_hit_by` (`NpcAI.LastHitBy`),
>   the damage source behind NPC aggro trigger (b).
> - **Hash:** no degraded fallback — `Hash()` panics (`hashFail`) when the
>   canonical Save cannot be serialized (non-finite float) instead of
>   returning a partial-state fingerprint, so twin-run comparisons can never
>   silently agree on two meaningless hashes.
> - **Warp:** the `align` phase lands at `warpLandRange` (2.5 km) of the
>   landing point exactly like the `warp` phase, so a sub-tolerance hop can no
>   longer sit in `align` forever. Player path unchanged: `CmdWarpTo` still
>   refuses inside `WarpMinRange` (150 km).
> - **NPC engagement ownership:** aggro trigger (b) is source-checked
>   (`NpcAI.LastHitBy == s.player` — third-party fire never aggroes an NPC
>   onto the player), and a `PatrolRespond` patrol holds station: the
>   idle-patrol loop skips it and `patrolRespondStep` stamps `IntentStop`
>   while it works the incident.
> - **RNG stream roles:** the master stream is documented (api.go) as exactly
>   two roles — player combat rolls and spawn entropy (asteroid/NPC scatter +
>   NPC fork seeds); `sim/rng_roles_test.go` pins its draw set.
> - **Save completeness check:** `sim/save_completeness_test.go` classifies
>   every `Sim` field as save-carried / derived / per-Step transient / explicit
>   exception (`events`, `ammoPrefs`, `lastPlayerWork` — the last two reset on
>   restore by design, pinned by `TestRestoreResetsDocumentedPreferences`) and
>   fails on unclassified fields, unclaimed `Save` fields, or a transient that
>   survives a Step boundary.

## `render` / `uikit` / `game` contract

> **Note (2026-10 — warp minimum + typeface).** (1) `sim.WarpMinRange =
> 150_000` m is the exported single source of EVE's warp-engage rule:
> `CmdWarpTo` refuses a target closer than that *at command time*
> (command-refusal pattern, `SevWarning` "Target within warp minimum range
> (150 km)"), and the Selected Item Warp action gates on the same
> constant with a hover hint. (2) The interface typeface is **IBM Plex Sans**
> (`render/fonts/*.ttf` + `OFL.txt`, SIL OFL 1.1, `//go:embed` in
> `render/fonts.go`): Regular for body text, Medium via `render.FaceBold` /
> `uikit.Env.FaceBold` for titles, table header sort buttons, buttons and
> HUD readouts; `goregular` no longer appears in `render`. IBM Plex Sans has
> no U+25B2/U+25BC glyphs, so table sort indicators are ↑/↓.

> **Resolution independence (2026-10).** The UI scale is
> `uikit.SetScale(deviceFactor × uiScale)`, installed by `game` at boot
> (the settings sidecar loads before the first layout) and on every
> Settings-window change: every visible length goes through `uikit.Px` /
> `uikit.Scale` (design units → live px; identity at scale 1, which is
> what the baseline suite pins), and `TypeScale` is scaled once at
> kit/env build so faces, line heights and measures follow. `Game.Layout`
> returns the outside size × the device factor — the framebuffer is
> device-resolution and text rasterizes at its final size (crisp on
> HiDPI; never rendered small and upscaled) — while `SetWindowSize` stays
> in window px. ebiten reports cursor positions in those same Layout
> units, so resize grips, menu rows and `overUI` agree with the scaled
> chrome by construction. The layout sidecar persists **design units**
> (÷ scale on write, × scale on read): one arrangement restores at any
> scale, and a live change round-trips through it (`Game.applyUIScale`)
> while rebuilding the whole interface against the new scale. World zoom
> (`zoomBase`) carries the device factor only — scaling the UI never
> zooms the world.

> **Music seam (2026-10).** The soundtrack is a `game.Music` interface
> (`game/music.go`) with exactly two implementations: the silent
> `nopMusic` that `game.New` installs when `Options.Music` is nil (every
> test, `simulate` and `validate` path) and `realMusic`, built only by
> `cmdRun` (`game.NewAudio`). The six `.ogg` files stay under
> `assets/audio/` byte-for-byte and are embedded by the shim beside them
> (`assets/audio/audio.go`, `//go:embed *.ogg`) — `go:embed` cannot reach
> a parent directory, and releases carry no `assets/` folder.
> `realMusic` decodes one embedded track at a time with
> `ebiten/v2/audio/vorbis` (`oggvorbis` is already an ebiten dependency;
> `hajimehoshi/go-vorbis` is cgo and is never used), creates one
> `NewPlayerF32` per track and polls `IsPlaying()` in `Tick` to advance —
> there is no end-of-stream callback. `Game.Update` ticks the seam on the
> mainline **before** the title branch, so the title, space and a docked
> station share one shuffled pool; `Current()`, the volumes and the
> shuffle flag are state only and never enter the event log or the save.
> The volume applied to the player is `master × music` (whole percents
> from the settings sidecar, 100/70 fresh), re-applied on every change
> and on every track start. **Structural invariant:** `audio.NewContext`
> may exist at most once per process (a second call panics) and its
> device only opens in ebiten's before-update hook on the first game-loop
> update, so it is created exactly once in `cmdRun`'s construction path
> (`sync.Once` inside the seam for belt and braces); no test may reach
> it, which is why `game.New` never constructs `realMusic` or
> the real cue players.
> Ordering is the pure `trackQueue` (injectable seed; tests pin
> permutations and no-immediate-repeat), and decode failures skip the
> track and surface through the `warn` callback (`cmdRun` prints them to
> stderr) — all six failing leaves the game silent, never broken.
>
> **UI sounds (2026-10).** The five interaction cues are a `game.UISound`
> interface (`game/uisound.go`) with the same two-implementation rule: a
> silent `nopUISound` (every test, `simulate`, `validate`) and
> `realUISound`, built only on the run path. Both real channels come from
> **one factory**, `game.NewAudio(seed, warn)` in `cmdRun`: it returns the
> music engine and the cue players over the *same* `audioCtx()`
> (`sync.Once`) context — two independent factories would risk the
> once-per-process `NewContext` panic the music seam already guards.
> `realUISound` decodes each `assets/sfx/*.wav` to PCM bytes once
> (`ebiten/v2/audio/wav`, 16-bit; a differing rate is resampled to the
> context's 44100) and keeps one `NewPlayerF32FromBytes` player per cue,
> re-triggered by `Rewind`+`Play`, so a cue cuts its own tail while
> distinct cues overlap. The volume is `master × ui`, re-applied on every
> change and again on every fire (the music seam's idiom). `Game.Update`
> fires `click` on a press edge that lands on `overUI` (one helper,
> `Game.uiClick`), the `ScreenHost` fires `open`/`close` from its mount
> and unmount edges (`onMount`/`onUnmount`, set by `Game.wireUISounds` on
> both hosts — the game's and the title's), and `Game.Do`'s quantity
> intents plus the verbs that land (save-as, a keybind assignment, the
> save-manager load/delete, the two resets) fire `confirm`; the
> keybindings refusals fire `refuse`. Kit-level one-offs that never mount
> through the host (the order mini-window, Show Info) stay silent.
>
> **UI platform (2026-10, complete):** the chrome lives in `uikit/`, built on
> **ebitenui v0.7.3** (pinned in go.mod); the legacy hand-drawn `ui/` package
> is retired (golden-value tests pin the EVE-dark palette and type scale).
> Two upstream gaps shape the layer: `widget.List` cannot do columns,
> per-row styling, or scroll introspection — so data tables use
> `widget.ScrollContainer` + per-row containers (we own the scroll
> instance); custom graphics (HUD arcs, starmap) render into `*ebiten.Image`
> shown via `widget.Graphic`. `uikit.Env` + `Panel`/`WindowPanel` are the
> seam every chrome panel implements; `Kit.Select/Root/AddContainer/
> OverviewWindow/SelectedWindow/SetCoreVisible` are the integration hooks
> the game shell drives (world-click selection, anchored chrome, dock-hide).
> `sim`/`content`/`script` and the showcase suite are unaffected by the UI
> layer.

> **Note (2026-10 — upstream window-remount leak, documented not
> "fixed"):** every `Kit.AddWindow` goes through ebitenui v0.7.3's
> `UI.AddWindowQuietly` (`ui.go`), and `removeWindow` undoes none of what
> it registers. Two things accumulate per mount cycle (measured in a
> 20k-cycle kit mount/unmount loop, `HeapInuse` after GC): (1) the four
> window event handlers `AddWindowQuietly` adds — context menu, focus,
> tooltip, drag-and-drop — whose `RemoveHandlerFunc`s upstream drops,
> ~0.2 KB/cycle; and (2) the big one:
> `addWindow` calls `Container.Validate()`, which re-runs
> `Button.initWidget()` for every button in the window on **every mount**
> — a fresh internal container per button, re-adding the (persistent)
> text/graphic children, whose `addChildInit` registers four more child
> event handlers each time — and those handler closures keep every
> generation's container alive. Net: **≈5.3 KB leaked per mount cycle**
> (~19 MB/hour at one dock/toggle per second; single-digit MB per heavy
> play-day). The handlers are idempotent, so this is memory only — no
> behavioural drift. Not mitigated on purpose: keeping a closed panel
> mounted-but-hidden would change visible semantics (a hidden window
> keeps its rect and would keep gating world input in `overUI`, and mount
> order feeds the draw stack), and forking ebitenui is out of scope.
> Revisit upstream (`removeWindow` should remove handlers; `Validate`
> should not rebuild buttons); `TestMemoryFootprintPump` watches the
> steady-state heap, not this toggle-paced line.

> **Icon set (2026-10):** `uikit/icons/` embeds the interface's monochrome
> glyph set — 512×512 transparent renders from
> [game-icons.net](https://game-icons.net) (**CC BY 3.0**: every icon keeps
> its author credit, `uikit/icons/CREDITS.md`, machine-readable in
> `manifest.json` + `uikit/icons/icons_test.go`). `icons.Get(name, size,
> tint)` converts a glyph PNG to a white-alpha silhouette at load, draws it
> into a fixed square box on the 16/20/24 size grid with the manifest's
> optional optical `nudge`, tints it from the palette role and caches per
> name+size+tint; art-mode entries (`"mode":"art"`, the five module renders)
> are drawn full-colour as-is — no flattening, no tint, cached per name+size —
> and are CC0 (the glyph set stays CC BY 3.0); unknown names resolve to the
> neutral gear (`icons.Fallback`), never a missing-image artifact.
> `theme.ModuleIconFor(def)` is the module-icon rule: a `content` module's
> `icon` key when the registry knows it, else its class glyph — every
> module-specific surface (fitting/rack tiles, hangar rows, Show Info module
> header, module tooltip, market module rows) goes through it, so a bad key
> degrades to the glyph. The widget surface that grew with it:
> `TableRow.Icons`/`TableRow.IconTint`/`TableRow.IconSize` (icon box + label
> cells in left-aligned columns — `IconSize` 0 is the 16 px `IconInline`, the
> market rows set 20, and the width reservation/ellipsize budget follow it;
> right-aligned numeric anchors are untouched),
> `uikit.NewTitleBar(env, label, icon)` (leading 24 px title-bar glyph),
> `uikit.KindIcon` / `ModuleIcon` / `ContractIcon` (semantic name → kind),
> `uikit.NewIcon` for shell-owned widgets (game contract rows, Undock), and
> `uikit.IconInline/IconButton/IconTitle/IconGap/IconDisabledA` as the size
> grid constants (plus `uikit.IconTile` 32 px for the fitting/rack module
> art). Surfaces that carry icons: overview Kind column, Selected
> Item header + action buttons, HUD target cards, scanner contacts, fitting
> slot/hangar rows, contract board rows, service title bars, module-rack
> tiles.

> **Window titles (2026-10):** a title bar carries no live state — every
> window passes its base display name (`Market`, `Manufacturing`,
> `Quantity`, `Order`, `Info`, `Fitting`) and never composes a runtime
> suffix. The figures the old suffixes repeated live in the bodies (the
> order book's columns, the picker's preview, the body heads); the one
> deliberate loss is the market's fee schedule, which had no other surface
> and is documented in GAMEPLAY instead. `theme.NewTitleBar` binds its
> `theme.ChromeText` to the bar, and `ChromeText.Shown` ellipsizes the
> drawn form against the bar's safe width — from the label's own start
> (clear of the dots and leading icon) to the end-anchored chrome, i.e. the
> `×` — so even a long or retitled label can never paint under the close
> button or out of the window; the fit case draws verbatim, and
> `theme.Ellipsize` is the one implementation (`widgets.Ellipsize`
> delegates). The retitle mechanism stays: setting `Label` still asks for
> a relayout and redraws (`TestTitleBarStillRetitles`).

> **Show Info (2026-10):** `uikit/menu.go` is the flat EVE-style popup
> context-menu primitive (first entry "Show Info", bold; the other entries
> are the surface's context actions, grayed-but-visible when currently
> impossible) and `uikit/infowin.go` is the one reusable floating details
> window (titled `Info`, body headed with the name) with per-kind content builders (hull / module /
> item / machine caste); item references inside the window are themselves
> right-clickable (Show Info swaps the window's content). Content schema
> gains `NpcDef.Description` (dossier prose — the window is its first UI
> consumer) and `ShipDef.Origin` (naming-layer heritage tag). The layout
> sidecar's `"infowin"` entry is geometry only — the rect is persisted but
> the window is never restored open. Its mount, raise and close ride the
> kit's one dialog lifecycle (`screens/kitdialog.go`, CONTEXT.md "Dialog"),
> shared with the order mini-window and the quantity picker. The popup is
> custom rather than ebitenui's own context-menu windows because those bypass the
> deterministic world-input gate: the menu rect joins `Game.overUI`'s
> exact-rect list, so a click over the popup can never reach world input,
> and the press itself is spent on the popup for the rest of the pass —
> `Kit.Update` still advances ebitenui's cached input while it skips
> `ui.Update`, and `Menu.Click`/`Esc` claim `widgets.PressEdge` until
> `Menu.TakeBlocked` — so no widget, table row, starmap node or slider
> under the popup sees the gesture either.

> **Split panes (2026-10):** a multi-pane window divides its leftover
> height — or width — by **ratio**, not ebitenui's equal `GridLayout`
> shares, which have no weights. `widgets.SplitPane` is a stack
> (`NewSplitPane`) or a row (`NewSplitPaneRow`) with a draggable seam
> between each pair: its own `Layouter` (so no upstream change), a
> recessed 4 px track seam — a dark fill edged with the border color and a
> centred three-dot sash grip, dim at rest and lit while the pointer is in
> the band or dragging — with an 8 px grab band classified from the cursor
> every frame (the window chrome's `gripStart` idiom and its topmost-input-layer
> test, so a seam under another window is inert), an NS-resize cursor over
> a stack and EW over a row, and a 72 design-px pane floor. The cursor has
> one writer: ebitenui's cursor management is off (`Kit`), seams and the
> window grips only claim a shape (`widgets.ClaimCursor` — chrome steps
> last, so a grip outranks a seam; the chrome also re-claims inside Draw)
> and the kit's `widgets.ApplyCursor` at the end of Draw makes the frame's
> single OS write. A claim belongs to the input pass, not the draw:
> `widgets.BeginCursorFrame` on the first line of `Game.Update` starts the
> pass, so a draw-only frame (ebiten draws every display frame, updates
> only ~60/s — on a fast display half the draws have no tick) repeats the
> pass's decision instead of falling back to the arrow. A host that never
> announces a pass (the `uitest` pump) keeps the last claim. Two writers per frame
> (ebitenui's AfterDraw arrow plus ours) alternated the shape and flickered. Each panel
> names its seams (`Inventory.Splits`, `Fitting.Splits`,
> `Manufacturing.Splits`, `Market.Splits` → `widgets.Seam`) and the layout
> sidecar persists them in the version-3 `splits` map (name → the fraction
> of the stack before that seam — above in a stack, left of it in a row;
> equal shares when a seam is absent, or the panel's own opening share when
> it sets one, as the market does). Version-2 sidecars still load — they
> carry no ratios, so every seam starts at its default and the next write
> is v3. Wired: Inventory (cargo/ore, ore/hangar), Fitting
> (slots/hangar), Manufacturing (recipes/jobs), Market (browse tree /
> catalog / My orders — two seams, `market-tree` and `market-orders`: 176 px
> of browse and the old fixed 240 px column are their opening shares of the
> 760×560 landing rect; the pane floor is `SplitMinPane`).

> **Table columns (2026-10 — frozen bases, two layout policies).** Every
> `widgets.Table` runs ONE layout pass (`Table.applyWidths` →
> `layoutColumns`); the header buttons, grip strips and row cells are all
> derived from that result (`layoutCells`/`headerLayout` anchor on the same
> `colOffsets` tiling), so no other site computes a column width. Two
> policies are kept apart on purpose. **(1) First fit:** the first fit with a
> width and rows measures the content once and freezes it — the pre-freeze
> elastic fit, unchanged (content widths, surplus to the lowest-Priority grow
> column, Priority shrink toward the header floors) — so every default frame
> stays byte-identical and the result IS each column's base; the bases never
> re-measure, so a cell growing mid-warp or the sort arrow moving only
> ellipsizes. **(2) Every later layout** (table resize, visibility change,
> stored pref): the table's width is distributed over the frozen bases
> **proportionally** — the ratios hold, so a column the player made smaller
> stays smaller and freed width spreads over the others, the rows still
> tiling exactly — and a table narrower than the bases gives width back in
> Priority order toward the floors, unchanged. A table resize adopts the new
> tiling as the base, and a grip press does the same (`adoptWidths`): a drag
> must edit the widths the player sees, or the surplus/shrink lives outside
> the base and the column bottoms out at floor+surplus (the market Item bug);
> content changes and visibility toggles never adopt, so a hidden column
> keeps its base width and restores it when shown. Header chrome: a 6
> design-px grip strip (`colGripW`) only on a boundary two visible columns
> share (the last visible column has no grip — that edge is the table's), and
> the ⋯ button owns its own 22 px strip (`ColMenuW`) at the header's right
> end, so no column tiles or paints under it. Per-column state — the width
> and the hidden flag — persists in the layout sidecar's `tables` map in
> design units (`widgets.ColumnPref`).

> **Market tree and search (2026-10):** the market's browse pane renders
> `ItemDef`/`ModuleDef`/`ShipDef` `group` paths (content-validated, 1–3
> segments) as a one-column table; a branch click scopes the catalog and the
> search field filters the branch live. Matching is
> `github.com/sahilm/fuzzy` (MIT, stdlib-only, the UI layer's one new
> dependency) over display name + full group path + maker, ranked by its
> score while a query is up (the header sort yields, restored on clear).
> Result rows are rich stack rows whose name is split into accent-tinted
> runs at `Match.MatchedIndexes`; the plain table grew
> `TableRow.CellRuns` for the dim `· N more` cross-station Qty suffix
> (read through the existing `BookOrders`/`StationName`/`Defs` seams — no
> new seam). The search `TextInput` is the second typing surface:
> `game.handleKeys` yields world keys while `Market.SearchFocused()` and
> routes the arrows/Enter/two-step Esc through `Market.SearchKey`;
> `Ctrl+F` opens the market and focuses the field.

### UI chrome architecture (map)

The chrome is a set of derivations, each with **one owner**; the notes above
are the change log, this is the index. [ART.md](ART.md) carries the visual
canon (form language, radii table, dot/depth anatomy, tooltip anatomy) — this
map only says where each thing derives from.

| Concern | Owner | Contract |
|---|---|---|
| Generator families | `uikit/theme/round.go` | One rasteriser behind `Bordered` (control, 5 px, flat), `PanelBox` (panel, 8 px, depth), `WindowTitleBox` (top corners only) + `WindowBodyBox` (bottom corners only, depth) — the two window pieces — `BandedBox` (control radii, fill starts under a plate band) and `Rounded` (explicit per-corner radii; square = zero). A screen asks for a **piece**, never a radius. |
| Radius / depth / dots / tracking tokens | `theme/round.go`, `theme/dots.go`, `theme/tracked.go` | `RadiusWindow/Panel/Control` are design px through `Px`; `depthRows` is the ramp's source resolution (the nine-slice centre row stretches it); `PanelTop`/`PanelBottom` are the ramp's ends derived from the body colour; `TitleDot*` the ring mark; `trackEm` the 0.12 em chrome tracking. One home each — no call site re-types them. |
| Tracked chrome labels | `theme/tracked.go` | `ChromeText` (widget), `DrawTracked` (hand-drawn) and `MeasureTracked` are the only tracking path; body/row text stays untracked Sans. A tracked label measures with the same step it draws. |
| Table columns | `widgets/columns.go` + `table.go` | One layout pass (`applyWidths` → `layoutColumns`) owns every width; header buttons, grips, the ⋯ strip (`ColMenuW`) and row cells all derive from `colOffsets`. Frozen bases + proportional later layouts (the "Table columns" note above). |
| Plate band | `screens/winheader.go` + `widgets.PlateBand` | Two halves of one boundary: the window wrapper paints panel → plate → body (draw order), and the table's viewport drops its fill under the band (`plateFillTop`). Both are paint-only: no width, rect or scroll offset moves. |
| Slider family | `widgets/slider.go` | One widget, two value models: **freeflow** (a min/max range quantised only by its Step — the settings Master/Music volumes, 1% steps) and **notched** (a snap-to ladder with one dim tick per rung and the handle resting exactly on them — the six-rung UI scale). The widget is a `PreferredSizeLocateableWidget` painted with palette roles and theme tokens (rounded track with accent fill, dim ticks, handle), so it needs no ebitenui theme images. Input is the pane-seam discipline: the panel's `Sync` calls `Poll`, which reads the shared raw cursor/press seams; a press grabs only inside the 24 design-px hit band on the widget's own active input layer (a covered slider is inert), a held grab follows the absolute cursor x with a 24 px band even where the track is thinner, and release drops it. `OnChange` streams the live value (the settings volumes apply it at the music seam; the scale host applies a rung and rebuilds the interface, after which the replacement slider starts idle); `OnRelease` is the single persist point. |

| Rack badge / ready marker | `screens/rack.go` | The waiting dot is one of `rackBadgePhases` pre-rendered images, swapped by pointer once per `rackBadgeTicks` (deterministic frame count, no per-frame Clear); the ready marker is a static overlay in the tile's other corner; the cycle ring is drawn under both. Both overlays are visibility-flipped, never rebuilt. |
| Quantity picker | `screens/qtypicker.go` + `screens/env.go` + `screens/kit.go` | The one replacement for every −/+ ticker. The popup is a session **service** (like the menu and Show Info), so a bare `NewEnv` and a `Kit` share one instance and one per-mode memory of the last confirmed value; panels never hold it — they emit the host-owned `widgets.QtyPick{Mode, Item, Recipe, From, To}` intent through the sink, which the Kit's `RunIntent` (or a bare session's wrapped sink) turns into `Session.OpenQty`. `Kit.OpenQty` mounts the window on first open, centres it on the root overlay and raises it; `Kit.CloseQty` unmounts and hides, so Cancel, Esc, `×` and Confirm all end in the same idempotent place. That is the kit's one **dialog** lifecycle (`screens/kitdialog.go`, Kit-internal): Show Info, the order mini-window and the picker all go through `newDialog` (the single `×` registration) + `showDialog` (re-target, optional `place`, then remove + re-add on top with `animate=false` — a launch on the open window never restarts the fade, and a launch during a close fade drops the pending unmount) + `closeDialog` (idempotent unmount + hide). Only the picker sets `place` (it centres on every show); a re-target of Show Info or the order window never moves it. The window is **modal** (ebitenui's fullscreen input layer: a click outside cannot reach the window beneath) and its slider is the house `widgets.Slider` polled from `Kit.Sync` — the refinery's notched whole-batch ladder, everything else freeflow 1..100. Esc is the one key it consumes, read in `Kit.MenuInput`/`Kit.Update` (`pickPoll`) ahead of any window close, so the host's Esc-menu latch already owns that frame. Confirm emits the surface's existing intent (`Refine`, `StartJob`, `Buy`, `Sell`, `Transfer`) — no new sim command, no new payload |
| Menu latch | `widgets/menu.go` + `screens/kit.go` | The **consumed-gesture latch**: `Menu.Click/Esc` set it, `Menu.Poll` reads it (one gesture, one action per frame), `Kit.Update` clears it via `Menu.TakeBlocked` and skips `ui.Update` that frame. Contract documented on `Menu`. |
| Clip seam | `sim/api.go` (`ClipState`) | The bay as the UI reads it: loaded rounds, capacity for the reference shell, selection/options, `Loading`, plus the derived bay spec `BayM3`/`CycleSeconds` (recomputed per call from `sim/clips.go`'s privates — never persisted). The rack's waiting dot, ready marker and tooltip bay row read this one state; the cycle ring reads the module runtime. |
| Screen host | `game/screenhost.go` + `game/screens.go` | One row (frozen id = the layout-sidecar key), one built panel. The host owns the mount order (spec order), the same-frame mount Sync, the × `Dismissed` hook — committed chrome close only, so a hotkey unmount keeps session folds — and the layout entry (geometry always; splits/tables only when the panel implements the facet). Two instances: the game host and the title host (lazy, exclusive, fresh on reopen, never persisted). The owner's close hook (`screenServices.Close`) rides only the game host — its modals close through their scene; the title host's rows close by a plain toggle on its own flag. |
| Title phase | `game/title.go` + `Options.StartInTitle` | The boot menu (wordmark, New game / Continue / Save manager / Settings / Keybindings / Credits / Quit) and its scene-owned windows; Settings/Keybindings are the title-scoped Screen rows mounted through the title's own host. The phase freezes the sim, applies one chrome rule (HUD/rack/core follow space, rail follows gameplay; panels unmounted) and leaves through one reconcile (`leaveTitle`, which never touches the player's open flags). Save plumbing (`SaveAs`, the save list, the active save path) lives here; the sim format is untouched. |

Rules that keep it one system: a screen names a piece or a token, never a
literal radius/alpha of its own; anything measured is measured by the same
function that draws it; every option that changes paint only (`PlateBand`,
`ContentsFace`) leaves geometry alone; and a value that lives in `sim` is read
through `sim`'s read side rather than re-derived in `uikit`.

- `render.Renderer` created with `(*content.Content)`; `Draw(dst *ebiten.Image,
  cam Camera, s *sim.Sim)` reads sim stores read-only. Vector art: ships as
  polygons per hull class, star glow, parallax starfield (seeded), grid dots,
  brackets (EVE-style corner brackets colored by standing: red NPC, blue
  celestial, white player), warp tunnel lines, mining beam, gun fire as a
  per-shot muzzle flash + travelling bolts + impact glow (never a
  muzzle→target beam), wreck icons.
- `uikit` widgets are pure layout+input over `*sim.Sim` state; window chrome
  EVE-dark (near-black panels, amber/white text, mono-ish gofont).
  Windows: Overview (table: icon, name, distance, velocity — sortable),
  Selected Item (actions: approach/orbit/warp/lock/dock/jump/loot), HUD
  (shield/armor/structure arcs, capacitor bar, speed, targeting list),
  Module rack (high/mid/low rows, F1–F8 activates high, deactivated state
  shown), Inventory (cargo + ore hold), Fitting (hull identity + stats
  with per-type EHP, the slot tile grid, the DPS/capacitor summary and the
  hangar),
  Manufacturing (jobs, fees, three global slots, craft levels and material
  counts at a docked `industry` station), Refinery (whole 100-ore batch
  ore→mineral yield preview at a `refine` station), Station services (repair/market/refine/
  fitting tabs), Starmap (system graph, current system highlighted —
  click selects, gates are used in-system), Log (event feed).
- `game`: scene stack. Space scene is the hub. `Options.StartInTitle`
  (set by `entieve run` when `--load` is absent) opens a **title phase**
  before the normal loop: the space scene renders statically as the
  backdrop, the sim does not step, gameplay chrome stays unmounted, and
  the title's own menu/windows (`game/title.go`) take the input; leaving
  the title (New game, Continue, a manager load) runs one normal boot
  reconcile. The keyboard map is
  `game.KeyMap` (`game/input.go`, the single source shared with the
  in-game help overlay `H`) and is documented in `docs/GAMEPLAY.md`. The
  input seam has two layers: the **fixed chord layer** (Esc menu, `Ctrl+S`,
  `Ctrl+F`, `F9`, `[` `]` `-` `=`) is matched literally in
  `handleKeys`, and the **data layer** — the Screen rows' toggles and the
  world commands — resolves `(key, shift)` through the session's live
  bindings (`Game.actionFor`, `game/keys.go`: defaults overlaid with the
  `<save>.keys.json` overrides, replace policy).
- Save/load: `game` writes `Save` JSON to
  `os.UserConfigDir()/entieve/save.json` on `Ctrl+S` and loads on `F9` (also
  `--load` CLI flag).

## Testing policy

- Unit tests per package (stdlib `testing`, table-driven). `sim` tests build
  small in-memory `content.Content` fixtures — no JSON needed.
- `content` tests load the **real embedded data** (must always validate) plus
  broken fixtures under `content/testdata/` for error paths.
- `tests/` package: integration — `showcase_test.go` runs `world-showcase`
  headlessly through scripted commands covering the feature checklist;
  `determinism_test.go` runs twin sims and compares `Hash()`.
- `go test ./...` must be green with no display server required.

## Content authoring (AI agents)

`docs/CONTENT_AUTHORING.md` is the cookbook: "to add a star system, create
`content/data/systems/<id>.json` with these fields, add it to
`universe.json`, run `go test ./content/...` and
`go run ./cmd/entieve validate`." The engine is deliberately data-driven so
that an AI agent can add systems/ships/modules/NPCs/items without touching
engine code.
