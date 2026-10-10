# Entieve — How to Play

*Entieve* is a single-player 2D space sandbox, a love letter to EVE Online:
top-down star systems, ship fitting, shield/armor/structure combat, capacitor
warfare, mining, markets, stations and a stargate network.

Run it with:

```
entieve run [--world world-default] [--seed N] [--save save.json] [--load save.json]
```

The window opens at 1280×720 (resizable, vsync, minimum 1024×700 — smaller
windows clamp up rather than breaking the HUD layout). Everything in the
keyboard table below is also available in-game via the help window (`H`).

## Controls

The key map below is generated from `game.KeyMap` — the same source the
in-game help window renders.

### Keyboard

| Key | Action |
|---|---|
| `Tab` | Cycle-lock nearest target (distance-ordered) |
| `Shift+Tab` | Lock the currently selected target |
| `F1` … `F8` | Toggle high-slot module 1…8 (activate/deactivate) |
| `A` | Approach selected |
| `Q` | Orbit selected — cycles 1k → 2.5k → 5k → 10k m |
| `S` | Stop |
| `W` | Align to selected |
| `Z` | Warp to selected (lands 10 km short of the target) |
| `D` | Dock (selected station, else nearest dockable one) |
| `J` | Jump through gate (selected gate, else nearest) |
| `L` | Loot nearest wreck |
| `M` | Mine selected asteroid (first fitted mining laser) |
| `I` | Toggle inventory (cargo + ore hold) |
| `F` | Toggle fitting window |
| `C` | Toggle scanner (slow off-grid sweep) |
| `G` | Toggle market window (trade while docked) |
| `N` | Toggle starmap |
| `H` | Toggle help window |
| `Esc` | Menu: Resume / Save / Load / Quit |
| `[` or `-` | Time acceleration down (8 → 4 → 2 → 1×) |
| `]` or `=` | Time acceleration up (1 → 2 → 4 → 8×) |
| `Ctrl+S` | Save game |
| `F9` | Load game |
| Arrow keys | Pan camera |

### Mouse

| Input | Action |
|---|---|
| Left click | Select entity / press UI buttons; drag a window's title bar to move it (click also raises it to the front) |
| Left click | On an overview column header: sort (click again flips direction) |
| `Ctrl` + left click | Lock target under cursor |
| Left double-click | Approach entity; on empty space, fly straight toward that point |
| Right click | Orbit target under cursor (2,500 m) |
| Right click | On a weapon cell in the module rack: cycle the loaded charge (Auto → each cargo ammo of the weapon's class) |
| Middle drag | Pan camera |
| Wheel | Zoom (0.05× – 4×); over the overview, scroll the list |

Mouse handling is fully context-aware: a click, drag or wheel over *any*
window or the HUD chrome never reaches the world behind it.

**Resolved key conflicts** (documented decisions):

- `F1–F8` are high-slot modules (EVE-style), so the classic "F5 = save" moved
  to **`Ctrl+S`**. `F9 = load` has no conflict and stays.
- `M` is **mine** (EVE muscle memory); the market window is **`G`**.
- Lock-under-cursor uses **`Ctrl`+click** so `L` stays free for looting.
- `C` is **scanner** (a new window; no legacy binding to displace).

### Windows

The interface is a stack of **draggable, resizable windows** with title
bars: drag a title bar to move a window, grab its right/bottom edge to
resize, and click anywhere on a window to raise it to the front. Window
positions are remembered for the session.

Toggle keys open and close their window (`I`, `F`, `G`, `C`, `N`, `H`) —
windows have no separate close button. Three panels are **always-on
chrome** and cannot be closed or moved: the ship **HUD bar** (bottom
center, with the target-box row at top center), the **module rack**
(bottom right) and the **log** (bottom left). The **Overview** (top left)
and **Selected Item** window (top right) stay open but are draggable —
a click in space selects the target that **Selected Item** then shows
and that every `selected`-targeted key acts on.

Default positions: overview top-left, selected item top-right, scanner
in the center column under the top row, log bottom-left, HUD
bottom-center, module rack bottom-right, starmap and the Esc menu
centered, help top-center; while docked the market and inventory fill the
center column, the fitting window takes the right edge and the station
strip takes the top-left corner (the overview and selected item step
aside while docked). The defaults never overlap: the in-space layout and
the docked set are checked pairwise at both 1280×720 and the 1024×700
minimum. Positions are remembered for the session — drag windows wherever
you like; loading a save snaps them back to these defaults.

### Docked at a station

Docking (`D`) opens the **Market**, **Inventory** and **Fitting**
windows plus a small **Station** strip with the **Undock** button, and
hides the space chrome (HUD bar, module rack, scanner) together with the
**Overview** and **Selected Item** windows. Undocking closes the station
windows that were closed before docking, restores the space chrome and
the overview/selected pair, and resets the camera view. The log stays
mounted while docked so its history comes with you.

### Overview (top-left)

A sortable, **scrollable** table of everything you can currently perceive:
name, distance, kind, velocity. Click a column header to sort (click again to
flip direction; the active column tints amber and shows ▲/▼), hover the wheel
over it to scroll (rows clip to the panel, scrollbar thumb included), click a
row to select. Numbers are right-aligned on fixed column anchors, so digits
changing never shift the columns.

Rows tint **red for hostiles** and **blue for your own ship**; the Kind
column names the rest (Ship, Asteroid, Wreck, Station, Gate, …).

## EVE concepts primer

**Slots.** Hulls have **high**, **mid** and **low** slots. High slots hold
weapons and mining lasers (toggled with `F1…F8`). Mid slots hold propulsion,
ewar and capacitor modules. Low slots hold tank (armor/resist) modules. The
fitting window (`F`) shows the hull, the slots and the derived stats; its
**Hangar** section lets you **fit** modules from the hangar into their first
free slot and **unfit** them back, and **assemble** stored hulls.

**Damage layers.** Damage hits **shield → armor → structure**, each reduced by
the ship's four **resists** (EM, thermal, kinetic, explosive). Entieve uses a
documented simplification: overflow damage carries to the next layer at *full*
value (no EVE-style bleed-through halving). Structure at zero means the ship
is destroyed.

**Capacitor.** Active modules consume GJ from the capacitor, which recharges
on EVE's curve (peak recharge near 25% of capacity; the fitting window shows
capacity and recharge time). Running dry means guns and tanks stop cycling —
manage it like EVE: inactive modules when you don't need them, cap boosters
when you do.

**Turret tracking.** Chance to hit follows EVE's formula: angular velocity vs.
tracking (scaled by signature radius), plus distance beyond optimal vs.
falloff. Fast, small, transversal-moving targets are hard to hit; wrecking
shots deal ×3 damage.

**Warp rules.** `Z` (Warp To) and the Selected Item window's warp button
both warp to the selected target at a fixed **10 km landing range** (the
sim's landing tolerance applies around the chosen point). Warp scramblers
on the target block warp and cancel an ongoing warp. Sublight movement is
limited by agility (align time ≈ agility × 10 s); the afterburner
multiplies speed while its cycle runs.

**Mining.** Select an asteroid and press `M`: the first mining laser cycles,
ore lands in the ore hold, and depleted asteroids vanish. Sell the ore at a
station's market or stockpile it.

**Overview distances** follow EVE conventions: meters under 2 km
(`1,250 m`), kilometers under 0.1 AU (`3.4 km`), AU beyond (`1.2 AU`).

## Sensors and visibility

Entieve has **no omniscience**: the client only ever shows what your ship
could perceive. This is a deliberate design decision (documented anti-
omniscience), enforced in the sim's read model and the renderer alike.

- **Grid / landmark visibility.** System landmarks — the star, planets,
  moons, station, gates, belt markers and anomalies — are always `Visible`;
  they appear in the overview, on the starmap, and get in-world brackets.
- **Sensor range.** Ships, asteroids and wrecks are `Visible` only within
  **`sim.SensorRange` = 250 km** of your ship. Anything farther is off-grid:
  it does not appear in the overview, cannot be picked by clicking, is not
  drawn by the renderer, and Tab-locking / `D`/`J`/`L` nearest-target
  helpers only ever consider perceivable entities.
- **The Scanner (`C`).** A slow sweep with a **2-second cycle** and an
  animated progress bar probes out to **3 × sensor range (750 km)**. It
  reports **anonymous counts only** — e.g. `Ship ×3`, `Asteroid ×22`,
  `Wreck ×2` — with *no names and no distances*, refreshed on each completed
  sweep. You learn that something is out there, not what it is or where it
  sits; you still have to fly there to see it. (Deliberate: the scanner is a
  hint system, not a map.)
- **Warp-to-range** is a UI choice only: `Z` and the warp button issue the
  same `CmdWarpTo` with the same fixed 10 km landing range.

**Ammo cycling.** Right-click a weapon slot in the module rack to cycle its
loaded charge: **Auto → each ammo of that weapon's class that sits in your
cargo → Auto** (the sim validates class and cargo, and logs every load).
The module tooltip — hover any rack cell for 300 ms — shows the module name,
slot, key stats (damage / optimal / tracking for guns, boost numbers for
tanks and prop, yield for mining lasers) and the currently loaded charge.
Non-weapons don't cycle.

## Currency

The currency of Entieve is the **credit**, displayed as
`12,345.67 cr`. You earn credits from NPC bounties, selling loot and market
trades, and spend them on market buys.

## The systems of Entieve

You start in **Solace** (high-security, constellation *Loomfall*): a friendly
star with *Haven Rest* station (market, inventory and fitting), an
asteroid belt and a gate to Penumbra.

**Penumbra** is low-security: NPCs are aggressive, the belt holds richer ore
(cordite, lucidite), and an anomaly hides marauders plus a hierarch boss with
the best loot.

**Gloam** is a dead-end frontier, deliberately dangerous: glacierite belt ore
and heavy spawns including hierarchs. Travel system-to-system by flying to a
stargate and pressing `J` — the starmap (`N`) shows the gate graph with your
current system highlighted.

## Time acceleration

`]` / `=` cycle time forward (1× → 2× → 4× → 8×), `[` / `-` cycle back. The
HUD shows the current `×N` next to the speed readout. Acceleration simply
runs more fixed 60 Hz simulation steps per rendered frame — determinism and
replays are unaffected.

## Save / load

- **`Ctrl+S`** writes a full JSON snapshot (`sim.Snapshot()`) to
  `os.UserConfigDir()/entieve/save.json` (override with `--save`).
- **`F9`** restores that snapshot mid-run and rebuilds the interface
  against the restored world.
- `--load <file>` restores a save at startup.
- The `Esc` menu offers the same Save / Load plus Resume and Quit.

Saving is the only way progress persists: quitting from the menu does not
autosave.

**Restoring mid-run rebuilds the interface**: the selection clears and
window positions snap back to their defaults, while the log history
carries across the rebuild — the simulation itself comes back exactly,
including docked state and any windows you had open.
