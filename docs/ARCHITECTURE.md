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
| `content/`    | Data schema (`model.go`), JSON loading (`load.go`), cross-reference validation (`validate.go`), embedded game data (`data/`). |
| `sim/`        | The deterministic game simulation: components, commands, events, systems. Headless — never imports Ebitengine. |
| `render/`     | Draws `sim` state: starfield, grid, ships (vector art), brackets, effects. |
| `ui/`         | EVE-style windows: overview, HUD (cap/shield/armor/structure), module rack, selected item, inventory, fitting, station services, starmap, combat log. |
| `game/`       | Ebiten `Game` implementation, scene stack (menu / space / station / starmap), input→`sim` commands, camera, time acceleration, save/load wiring. |
| `cmd/entieve` | CLI entrypoint: `run` (play), `validate` (content lint), `simulate` (headless run with report), `version`. |

Dependency direction (never invert):

```
cmd/entieve → game → {render, ui} → sim → content → (core, ecs)
                          └──────────→ content
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
kebab-case strings, globally unique per kind, and cross-referenced.

### ID registry (authoritative — content files MUST use these IDs)

**Damage types** (EVE-style four): `em`, `thermal`, `kinetic`, `explosive` —
typed as `content.DamageType` with constants `DmgEM, DmgThermal, DmgKinetic,
DmgExplosive`. `Damage` is a struct `{EM, Thermal, Kinetic, Explosive float64}`;
`Resists` is four fractions in `[0,1)`.

**Minerals** (`content/data/items/minerals.json`):
`min-ferrite`, `min-nickel`, `min-cobalt`, `min-silex`, `min-glacite`,
`min-argon`.

**Ores** (`content/data/items/ores.json`):
`ore-sterngrit` (yields ferrite), `ore-cordite` (ferrite, nickel),
`ore-lucidite` (nickel, cobalt, silex), `ore-glacierite` (glacite, argon).
Ore defs carry `refine_yield: {"min-ferrite": 0.45, ...}` fractions per m³.

**Ammunition** (`content/data/items/ammo.json`) — each has a `damage`
profile; turrets consume 1 charge/shot:
`charge-emp` (EM-biased), `charge-fusion` (explosive-biased),
`charge-ap` (kinetic-biased), `charge-plutonium` (thermal-biased),
`charge-cap-cell` (cap booster fuel, not ammo).

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
- weapons: `mod-emp-turret` (uses `charge-*` ammo class `light`),
  `mod-rail-turret` (ammo class `light`), `mod-rocket-pod` (no ammo,
  explosive rockets);
- tank: `mod-shield-boost` (active), `mod-shield-extender` (passive raw HP),
  `mod-armor-rep` (active), `mod-armor-plate` (passive raw HP),
  `mod-resist-coat` (passive resist buff);
- cap: `mod-cap-booster` (active, consumes `charge-cap-cell` from cargo),
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
traffic system rather than spawn groups — `npc-guard-patrol` (hull
`ship-npc-patrol`, registered per patrol hull via `Content.GuardFor`).

**Systems** (`content/data/systems/*.json`, one file per system):
- `sys-solace` — starting system (constellation Loomfall). Star
  `cel-solace-star`, planet `cel-solace-prime`, station `cel-haven-rest`
  (services: repair, market, refine, fitting), belt `cel-solace-belt`
  (ore-sterngrit, ore-cordite), gate `cel-gate-penumbra` → `sys-penumbra`,
  rat spawns (scuttler), market orders (all player ships + core modules +
  charges).
- `sys-penumbra` — low-security (NPCs aggressive). Belt `cel-penumbra-belt`
  (ore-cordite, ore-lucidite), anomaly `cel-penumbra-den` (marauders +
  boss hierarch, best loot), station `cel-penumbra-port` (repair, market,
  refine, fitting; hosts the level-2 contract board), gates → `sys-solace`
  and `sys-gloam`.
- `sys-gloam` — dead-end frontier, dangerous. Belt `cel-gloam-belt`
  (ore-glacierite), heavy spawns incl. `npc-rat-hierarch`.

**Universe** (`content/data/universe.json`): region/constellation names,
system list with starmap positions, `start_system: "sys-solace"`.

**Worlds** (`content/data/worlds/*.json`) — playable scenarios:
- `world-default` — normal game start (sloop, modest fit/cargo/wallet at
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
  (deliver 2 m³ charge-emp to `cel-penumbra-port`), `contract-solace-cordite-1`
  (mine 400 m³ ore-cordite) — the `cel-haven-rest` board (level 1, standing 0).
- `contract-penumbra-den-hunt-1` (kill 1 marauder), `contract-penumbra-lucidite-1`
  (mine 500 m³ ore-lucidite) — the `cel-penumbra-port` board (level 2,
  `min_standing` 0.2).
Rewards are calibrated by the payout ratio table in
`docs/CONTENT_AUTHORING.md` ("add a contract").

**Traffic** (`content/data/traffic/*.json`, one def per system, keyed by
its `system` field): convoy `routes` (station/gate/belt endpoints inside
the system, hull, cargo item+qty, authored `interval_ticks`), an optional
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
    BasePrice float64          // ISK-analog "credits"
    RefineYield map[ItemID]float64 // ore only, fractions summing to <= 1
    Damage *Damage             // ammo only, applied per shot
    AmmoClass string           // ammo only, e.g. "light"; "" for non-ammo
}
type ShipDef struct {
    ID ShipID; Name, Class, Description string
    Slots struct{ High, Mid, Low int }
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
    Slot string                // "high" | "mid" | "low"
    Kind string                // "weapon" | "shield_boost" | "shield_extender" | "armor_rep" | "armor_plate" | "resist_coat" | "cap_booster" | "cap_recharger" | "afterburner" | "mining_laser" | "stasis_web" | "warp_scram"
    ActivationCost float64     // GJ per activation (0 for passive)
    ActivationTime float64     // seconds per cycle (0 for passive)
    Passive struct{ ShieldHP, ArmorHP float64; Capacitor float64; Recharge float64; ShieldBoost, ArmorRep float64; ResistBonus Resists; SpeedBonus float64 } // passive modules fill relevant fields
    Weapon *WeaponAttrs        // Damage Damage; Optimal, Falloff, Tracking float64; AmmoClass string; VolleyTime float64 (s); ChargeSize int
    Mining *MiningAttrs        // VolumePerCycle float64 (m³); CycleTime float64
    Boost  *BoostAttrs         // ShieldBoostPerCycle / ArmorRepPerCycle float64; SpeedMultiplier float64; WebStrength float64; ScramDuration float64; CapRestore float64
}
type NpcDef struct {
    ID NpcID; Name, Faction string
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
    Gate *struct{ ToSystem SystemID; ToCel CelID }
    Station *struct{ Services []string }            // "repair"|"market"|"refine"|"fitting"
    Belt *struct{ Asteroids []struct{ Ore ItemID; Count int; Volume float64 } }
    Anomaly *struct{ Spawns []SpawnDef; RespawnSec float64 }
    Spawns []SpawnDef          // ambient spawns for any celestial: {NpcID, Count, RespawnSec}
}
type MarketOrderDef struct    // station market listing: {ItemID or ShipID, Qty, Price}
type SystemDef struct {
    ID SystemID; Name, Security string; Constellation, Region string
    Position core.Vec2        // starmap plane (ly)
    Palette struct{ Nebula [3]string; Star string } // hex colors for rendering
    Celestials []CelestialDef
    Market []MarketOrderDef   // station-bound orders (all stations in system share for now)
}
type UniverseDef struct { Region, Constellation string; StartSystem SystemID; Systems []struct{ID SystemID; Position core.Vec2} }
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
    From, To CelID; Hull ShipID; Cargo RouteCargo
    IntervalTicks int64                           // authored cadence; security scales it
}
type PatrolDef struct{ Hulls []ShipID; IntervalTicks int64 }
type TrafficDef struct {                          // one def per system, keyed by system
    System SystemID; Routes []RouteDef
    Patrol *PatrolDef; Raider NpcID               // raider required when security is low|null
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
classes / cargo items / stations); traffic route endpoints inside the
system (station/gate/belt kinds), hulls, cargo items (volume must fit the
hull), intervals >= 1, non-empty route lists, raider presence for low/null
security (a rat class spawned in that system), patrol hulls with an
`npc-guard-*` definition; slot overflow
in world fits and NPC fits; refine yields outside `[0,1]`; resists outside
`[0,1]`; negative numbers; worlds referencing unknown systems; `world-showcase`
present with a non-empty `scenario`.

## `sim` contract (`sim/model.go`, `sim/api.go`)

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
CmdTransfer{Item, Qty, From, To}, // holds: "cargo" | "ore" | "hangar"
CmdRefine{Item, Qty}, CmdRepair{}, CmdAssemble{ShipID},
CmdFitModule{ModuleID, Slot SlotRef}, CmdUnfitModule{Slot SlotRef},
CmdAcceptMission{ID content.ContractID}, CmdAbandonMission{ID content.ContractID}
```
(`Target ecs.Entity`; module index = flat slot index within its slot class.
The two mission commands validate against the docked station's contract
board — docked check, board check, `min_standing` check — and report
refusals as `EvMission` phase `"refused"`.)

Events (`sim/model.go`) — read via `(*Sim).Events() []Event` (drained each
call) or `DrainEvents()`. Kinds: `EvtLog` (combat/mining/system log lines with
severity), `EvtDestroyed`, `EvtWarp{Phase}`, `EvtLooted`, `EvtDocked`,
`EvtUndocked`, `EvtJumped`, `EvtMarket`, `EvtModuleActive/Deactivated`,
`EvtTargetLocked`, `EvtCargoFull`, `EvtAnomalyCleared`, …
`EvMission{Phase, ID, Name}` reports the contract lifecycle with phases
`"accepted" | "progress" | "ready" | "completed" | "abandoned" | "refused"`
(ID/Name carry the contract ID and title; Value carries the payout).
`EvTraffic{Phase, System, Cel, Item, Value, Text}` reports the
living-universe convoy lifecycle with phases `"departed" | "delivered" |
"attacked" | "destroyed" | "escaped" | "patrol-response"` (System carries
the system ID, Cel the route endpoint / incident celestial, Item/Value the
shipment). Market moves driven by traffic ride the existing `EvtMarket`
kind with `System`/`Item`/`Value` (new price) and text
`"Surplus — … price falls to …"` / `"Risk premium — … price rises to …"`.

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

Systems (`sim/` internal, called by `Step` in this order):
1. `command` (apply queued commands to intent),
2. `movement` (accel toward desired velocity per agility; web effect),
3. `warp` (align → warp → land state machine),
4. `targeting` (lock progress by scan res & sig radius),
5. `modules` (cycle timers, activation cost, passive aggregation),
6. `weapons` (turret math below, ammo consumption),
7. `damage` (shield → armor → structure with resists; death → wreck),
8. `capacitor` (recharge curve),
9. `mining` (cycle yields into ore hold; asteroid depletion),
10. `npc` (state machine: idle→approach→orbit→fire→flee; uses Fork'd RNG),
11. `loot` (loot command → cargo; bounty on NPC death → wallet),
12. `economy` (buy/sell/refine/repair/fitting at station; wallet),
13. `travel` (dock/undock/jump: swaps system content),
14. `mission` (contract accept/abandon against the docked station's board,
    folds kill/ore objective observations, auto-claims ready contracts on
    docking at the claim station → wallet + standing),
15. `respawn` (belt/anomaly NPC respawn timers),
16. `traffic` (the living universe: convoy spawn/warp/deliver schedules,
   per-leg ambush rolls, raider and patrol combat, market drift + premium
   decay),
17. `events` (trim/cleanup).

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
  at station endpoints (exact integer stock) and despawn at gates/belts
  (v1: no cross-system NPC jumps). Live haulers are capped at 6 per
  system, patrols at 4; schedules are stateless hashes (no timers to
  persist). Haulers and patrols are ordinary entities: the player can
  lock, shoot and loot them like anything else.
- **Security patrols** (`npc-guard-*` loadouts on `ship-npc-patrol`) idle
  on a guard circle 20–30 km east of the anchor station (outside the
  8 km NPC guard radius so station traffic never trips them), and respond
  to convoy ambushes: instant 3-ship wings in high security, a single
  responder after 15 s in low security, nothing in null. Responding
  patrols warp to the incident and fight raiders through the normal
  lock/activate/turret/damage pipeline — NPC-vs-NPC combat reuses the
  player pipeline with an explicit activation target (`npcActivate(e, ai,
  target)`); only the *aggro* state machine remains player-centric.
- **Sporadic attacks** — every convoy tunnel entry in low/null security
  rolls `p = (1-σ)/3` (1-in-6 Penumbra, 1-in-3 Gloam, 0 high-sec); on a
  hit, 1–3 raiders of the system's `raider` rat class drop in mid-leg
  (renamed `<Name> Ambusher` so name-addressed scripts stay
  unambiguous), the hauler is dragged out of warp and burns away for 20 s
  before attempting its warp-out. Scramblers/webs pin it (lock ≈ 15 s vs
  the 20 s flee), the patrol's arrival pulls the raiders' guns off the
  convoy, and the incident resolves as **destroyed** (wreck with the
  exact cargo, lootable via `CmdLoot`; risk-premium price step up),
  **escaped** (warps clear and delivers anyway), or **saved** (all
  raiders dead while the hauler lives). Raiding NPCs never queue respawn
  timers against nearby spawn groups.
- **Market memory** — deliveries add exact units of stock and step the
  item's price DOWN one supply step; a lost inbound shipment steps the
  destination system's price UP (risk premium). Steps are
  `price' = clamp(base·(1±band), price·(1±k))` with `band = 0.05·4^(1-σ)`
  and `k = share·band/2.5` (×2 for losses); deviating prices halve their
  distance to base every 3,600 ticks. Prices can never leave the band.
  All constants are derived/pinned in the calibration tables in
  `docs/CONTENT_AUTHORING.md` ("traffic calibration contract").

**Determinism**: all living-universe randomness (spawn scatter, ambush
rolls, raider counts, patrol scatter) draws from a dedicated traffic RNG
stream forked from the master stream at `New` (`Save.TrafficRNGState`) —
traffic never consumes the player's combat rolls, so existing seed-pinned
outcomes are untouched. Hauler legs/ambushes persist as `Traffic`/`Patrol`
components and `Save.TrafficLeaves` (scheduled raider break-offs); prices
persist in `Save.Market`.

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
  and emit `EvtLog` once per cycle.

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
  (`EvtLog` "warp scrambled").

### Damage

Layer order shield → armor → structure. Each layer has the ship's `Resists`
(single set applied per layer; modules may add `ResistBonus` per layer).
Overflow damage carries to the next layer at full value (documented
simplification of EVE's 50% bleed-through between shield/armor is NOT used —
keep full carry-over, note it in `docs/GAMEPLAY.md`). Structure ≤ 0 → ship
dies: the player wakes up pod-less in a fresh `ship-sloop` at the clone bay —
the last docked station, or the home station in the universe's start system
when the current system has none (EVE clone semantics; `EvtDestroyed` is
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
NPC traffic delivery + market price drift, ambush on NPC traffic
(Penumbra legs, showcase seed), security patrol response + convoy escape.

## `render` / `ui` / `game` contract

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

- `render.Renderer` created with `(*content.Content)`; `Draw(dst *ebiten.Image,
  cam Camera, s *sim.Sim)` reads sim stores read-only. Vector art: ships as
  polygons per hull class, star glow, parallax starfield (seeded), grid dots,
  brackets (EVE-style corner brackets colored by standing: red NPC, blue
  celestial, white player), warp tunnel lines, mining beam, muzzle flash,
  wreck icons.
- `ui` widgets are pure layout+input over `*sim.Sim` state; window chrome
  EVE-dark (near-black panels, amber/white text, mono-ish gofont).
  Windows: Overview (table: icon, name, distance, velocity — sortable),
  Selected Item (actions: approach/orbit/warp/lock/dock/jump/loot), HUD
  (shield/armor/structure arcs, capacitor bar, speed, targeting list),
  Module rack (high/mid/low rows, F1–F8 activates high, deactivated state
  shown), Inventory (cargo + ore hold), Fitting (hull slots + stats),
  Station services (repair/market/refine/fitting tabs), Starmap (system
  graph, current system highlighted, click to set autopilot? — no autopilot:
  click selects, gates are used in-system), Combat log (event feed).
- `game`: scene stack. Space scene is the hub. Keys: `Tab` target
  closest, `F1..F8` high modules, `A` approach, `Q` orbit at range,
  `S` stop, `W` align, `X` approach & loot? — keep documented in
  `docs/GAMEPLAY.md` and shown in an in-game help overlay (`F1`?). Final key
  map lives in `game/input.go` and must be documented in `docs/GAMEPLAY.md`.
- Save/load: `game` writes `Save` JSON to
  `os.UserConfigDir()/entieve/save.json` on `F5` and loads on `F9` (also
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
