# Entieve — How to Play

*Entieve* is a single-player 2D space sandbox, a love letter to EVE Online:
top-down star systems, ship fitting, shield/armor/structure combat, capacitor
warfare, mining, markets, stations and a stargate network.

Run it with:

```
entieve run [--world world-default] [--seed N] [--save save.json] [--load save.json]
```

The window opens at 1280×720 (resizable, vsync). Everything below is also
available in-game via the help overlay (`H`).

## Controls

The key map below is generated from `game.KeyMap` — the same source the
in-game help overlay renders.

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
| `Z` | Warp to selected |
| `D` | Dock (selected station, else nearest dockable one) |
| `J` | Jump through gate (selected gate, else nearest) |
| `L` | Loot nearest wreck |
| `M` | Mine selected asteroid (first fitted mining laser) |
| `I` | Toggle inventory (cargo + ore hold) |
| `F` | Toggle fitting window |
| `G` | Station services — jump to the Market tab (while docked) |
| `N` | Toggle starmap |
| `H` | Toggle help overlay |
| `Esc` | Menu: Resume / Save / Load / Quit |
| `[` or `-` | Time acceleration down (8 → 4 → 2 → 1×) |
| `]` or `=` | Time acceleration up (1 → 2 → 4 → 8×) |
| `Ctrl+S` | Save game |
| `F9` | Load game |
| Arrow keys | Pan camera |

### Mouse

| Input | Action |
|---|---|
| Left click | Select entity / press UI buttons; sort overview by clicking column headers |
| `Ctrl` + left click | Lock target under cursor |
| Left double-click | Approach entity; on empty space, fly straight toward that point |
| Right click | Orbit target under cursor (2,500 m) |
| Middle drag | Pan camera |
| Wheel | Zoom (0.05× – 4×) |

**Resolved key conflicts** (documented decisions):

- `F1–F8` are high-slot modules (EVE-style), so the classic "F5 = save" moved
  to **`Ctrl+S`**. `F9 = load` has no conflict and stays.
- `M` is **mine** (EVE muscle memory); station services / market is **`G`**.
- Lock-under-cursor uses **`Ctrl`+click** so `L` stays free for looting.

### Overview (top-right)

A sortable table of everything on grid: icon, name, distance, velocity.
Click a column header to sort (click again to flip direction), click a row to
select. Colors: red = hostile, orange = neutral NPC, blue = station,
green = gate, grey = asteroid, yellow = wreck, cyan = you.

## EVE concepts primer

**Slots.** Hulls have **high**, **mid** and **low** slots. High slots hold
weapons and mining lasers (toggled with `F1…F8`). Mid slots hold propulsion,
ewar and capacitor modules. Low slots hold tank (armor/resist) modules. The
fitting window (`F`) shows the hull, the slots and the derived stats; while
docked, the station's *Fitting + Hangar* tab lets you **fit** modules from the
hangar into their first free slot and **unfit** them back.

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

**Warp rules.** `Z` (Warp To) makes you align first, then warp, landing within
2,500 m of the target. Warp scramblers on the target block warp and cancel an
ongoing warp. Sublight movement is limited by agility (align time ≈
agility × 10 s); the afterburner multiplies speed while its cycle runs.

**Mining.** Select an asteroid and press `M`: the first mining laser cycles,
ore lands in the ore hold, and depleted asteroids vanish. Refine ore at a
station (Refine tab) or sell it raw.

**Overview distances** follow EVE conventions: meters under 2 km
(`1,250 m`), kilometers under 0.1 AU (`3.4 km`), AU beyond (`1.2 AU`).

## Currency

The currency of Entieve is the **credit**, displayed as
`12,345.67 cr`. You earn credits from NPC bounties, selling loot and market
trades, and spend them on repairs, market buys and fees.

## The systems of Entieve

You start in **Solace** (high-security, constellation *Loomfall*): a friendly
star with *Haven Rest* station (repair / market / refine / fitting), an
asteroid belt and a gate to Penumbra.

**Penumbra** is low-security: NPCs are aggressive, the belt holds richer ore
(cordite, lucidite), and an anomaly hides marauders plus a hierarch boss with
the best loot.

**Gloam** is a dead-end frontier, deliberately dangerous: glacierite belt ore
and heavy spawns including hierarchs. Travel system-to-system by flying to a
stargate and pressing `J` — the starmap (`N`) shows the gate graph; clicking a
system shows its security status and connections.

## Time acceleration

`]` / `=` cycle time forward (1× → 2× → 4× → 8×), `[` / `-` cycle back. The
HUD shows the current `×N` next to the speed readout. Acceleration simply
runs more fixed 60 Hz simulation steps per rendered frame — determinism and
replays are unaffected.

## Save / load

- **`Ctrl+S`** writes a full JSON snapshot (`sim.Snapshot()`) to
  `os.UserConfigDir()/entieve/save.json` (override with `--save`).
- **`F9`** restores that snapshot mid-run and rebinds the UI.
- `--load <file>` restores a save at startup.
- The `Esc` menu offers the same Save / Load plus Resume and Quit.

Saving is the only way progress persists: quitting from the menu does not
autosave.
