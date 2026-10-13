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
| `Q` | Orbit selected at the Selected Item **Orbit range** selector's value (2.5k → 5k → 10k → 20k → 50k m; the key never changes it) |
| `S` | Stop |
| `W` | Align to selected |
| `Z` | Warp to selected at the Selected Item **Warp to within** range (0/10/50 km); refused inside 150 km |
| `D` | Dock (selected station, else nearest dockable one) |
| `J` | Jump through gate (selected gate, else nearest) |
| `L` | Loot nearest wreck |
| `Shift+L` | Toggle log window |
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
| Left drag | On any window **edge or corner** (≈8 px grip): resize from all four sides — the opposite edge stays fixed, the window never shrinks past its minimum |
| Left click | On an overview column header: sort (click again flips direction) |
| `Ctrl` + left click | Lock target under cursor |
| Left double-click | Show Info — open the item's details window directly, anywhere the item appears (rack, fitting, market, inventory, overview, scanner, contract rows); on empty space, fly straight toward that point |
| Right click | Orbit target under cursor (2,500 m) — in the world; unchanged |
| Right click | On an item anywhere it appears: flat context menu — **Show Info** first (always bold), then the surface's actions (grayed-but-visible when impossible); on a weapon cell in the module rack: Show Info / Show Info — `<charge>` / ─ / **Cycle charge** |
| Middle drag | Pan camera |
| Wheel | Zoom (0.05× – 4×); over a table (overview, market, inventory, fitting), scroll it — wheel up moves toward the top |

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
bars: drag a title bar to move a window, grab **any edge or corner** to
resize it (about an 8 px grip — all four edges and all four corners
work; the opposite edge stays fixed and the window never shrinks below
its minimum size), and click anywhere on a window to raise it to the
front.

Toggle keys open and close their window (`I`, `F`, `G`, `C`, `N`, `H`,
`Shift+L`) — windows have no separate close button. Icons (from
[game-icons.net](https://game-icons.net), credited per-icon in
`uikit/icons/CREDITS.md`) mark service titles, action buttons, overview
kinds and contract types — monochrome glyphs tinted to the theme, and
every icon keeps its text label. Two panels are
**always-on chrome** and cannot be closed or moved: the ship **HUD bar**
(bottom center, with the target-box row at top center) and the
**module rack** (bottom right; each cell is a class-glyph tile with the
module's name under it). The **log** is a normal window now —
bottom left by default, toggle with `Shift+L`, draggable and resizable
like the rest. The **Overview** (top left) and **Selected Item** window
(top right) stay open but are draggable — a click in space selects the
target that **Selected Item** then shows and that every
`selected`-targeted key acts on. Its action buttons (Approach, Warp,
Orbit, Stop, Lock, Dock, Jump, Loot, Mine) act on that selection; the two
selectors — **Orbit range** (2.5k–50k m) and **Warp to within** (0/10/50 km)
— are pure state: clicking one only cycles its value and never issues a
command. Warp, Orbit, `Z` and `Q` all act at the range their selector
currently shows.

Default positions: overview top-left, selected item top-right, scanner
in the center column under the top row, log bottom-left, HUD
bottom-center, module rack bottom-right, starmap and the Esc menu
centered, help top-center. While docked the Station window (contract
board included) takes the top-left corner (overview and selected item
step aside) and the **service windows tile themselves to your viewport**:
the market takes the full band beside the station — wide enough that its
Item/Price/Qty columns render whole instead of ellipsized — and
inventory and fitting share the row under it. The defaults never
overlap: the in-space layout and the docked set are checked pairwise at
1024×700, 1280×720 and 1920×1080.

**Resizing never stomps your arrangement.** Those composed defaults only
ever apply to windows you have never touched: drag any window somewhere
else, resize the game window, and it stays exactly where you put it
while the untouched ones re-tile around it.

**Your layout persists.** Every named window's position, size and
open/closed state (overview, selected item, log, market, inventory,
fitting, scanner, map, help) is saved to a sidecar file next to the
save — `<save path>.layout.json`, e.g.
`os.UserConfigDir()/entieve/save.json.layout.json` — whenever you save
(`Ctrl+S` / menu **Save**) and again when you quit cleanly from the
menu. It is read back on startup and after loading a save, so a
favorite arrangement survives restarts and loads. Restored windows are
clamped to their minimum size and to the screen (always fully
on-screen) — the clamp is re-established whenever you resize the game
window too, so a layout saved at a larger resolution can never strand a
window off-screen — and only what no longer fits slides back in; a
missing, corrupt, wrong-version or partial sidecar simply falls back to
the defaults, and unknown entries in the file are ignored. **Your saved
arrangement wins**: a window with a sidecar entry is never recomposed —
the automatic tiling only fills in the windows the sidecar does not
mention. One carve-out keeps that promise honest for the docked
services: if a saved market/inventory/fitting set comes back
overlapping itself (a layout file from an older game version, or one
saved at a resolution that no longer fits), the three re-tile to the
current viewport instead of restoring the overlap — open/closed state
is always honored either way.
The layout sidecar is deliberately separate from the save itself — the
simulation save format is unchanged for the CLI tools.

### Docked at a station

Docking (`D`) opens the **Market**, **Inventory** and **Fitting**
windows plus the **Station** window — the station identity, the
**Undock** button and the contract board (see
[Contracts & standing](#contracts--standing)) — and hides the space
chrome (HUD bar, module rack, scanner) together with the
**Overview** and **Selected Item** windows. The service windows **tile
automatically at your viewport size** — market on top, inventory and
fitting below — and any arrangement you saved or dragged wins over that
tiling (unless the saved set would come back overlapping itself, in
which case the three re-tile — see **Your layout persists**).
Undocking closes the station windows that were closed before
docking, restores the space chrome and the overview/selected pair, and
resets the camera view. The layout sidecar remembers that pre-dock
state (not the dock-forced open flags), so quitting while docked does
not reopen the market and friends in space on the next boot. The log
stays mounted while docked so its history comes with you.

### Overview (top-left)

A sortable, **scrollable** table of everything you can currently perceive:
name, distance, kind, velocity. Click a column header to sort (click again to
flip direction; the active column tints amber and shows ▲/▼), hover the wheel
over it to scroll (wheel up moves toward the top of the list; rows clip to
the panel), click a
row to select. Numbers are right-aligned on fixed column anchors, so digits
changing never shift the columns.

Rows tint **red for hostiles** and **blue for your own ship**; the Kind
column names the rest (Ship, Asteroid, Wreck, Station, Gate, …).

## Inspecting items (Show Info)

Anything you can see is inspectable. **Right-click** an item anywhere it
appears — rack cell, fitting row, market row, inventory row, overview or
scanner row, contract board row, contract objective label — for a flat
context menu: **Show Info** first, always bold, then that surface's
actions. Actions you cannot currently take are shown grayed, never
hidden. **Double-click** the item to skip the menu and go straight to the
details window. There is no keyboard key for this: inspecting is
mouse-only.

One reusable floating window, titled `<Name> — Info`, holds the details —
dragable, resizable, closeable, non-modal. Show Info swaps its content
instead of stacking windows, and the item references inside it (charges,
ores, hulls) are themselves right-clickable. Its position and size
persist in the layout sidecar, but it never opens on its own.

Only fields that exist are shown, grouped per kind:

| Kind | Sections |
|---|---|
| Hull | CONSTRUCTION (class · origin) / SLOTS / SURVIVAL / CAPACITOR / PROPULSION / SENSORS / CARGO |
| Module | ACTIVATION / kind stat sections / CHARGES (compatible charges) / MARKET |
| Item | PROPERTIES / MARKET / REFINES TO / DAMAGE |
| Machine caste | dossier prose / DOSSIER (faction, hull, bounty) / BEHAVIOR / LOOT |

Every window opens with the def's description prose. The menus per
surface: rack cells — Show Info (module) / Show Info — `<charge>` / ─ /
**Cycle charge**; fitting rows — Show Info / Fit / Unequip; market rows —
Show Info / Buy / Sell (grayed while undocked); inventory rows — Show
Info; overview and scanner rows — Show Info / Warp to / Orbit / Approach
(the entity's hull or caste info plus the usual commands); contract board
rows — Accept / Abandon (grayed when gated); contract objective labels —
Show Info.

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

**Warp rules.** `Z` and the Selected Item **Warp** button both warp to the
selected target at the **Warp to within** selector's range (0 / 10 / 50 km;
the sim's landing tolerance applies around the chosen point). The drive
cannot engage a target closer than **`sim.WarpMinRange` = 150 km** (EVE's
rule): inside that the command is refused with the log line "Target within
warp minimum range (150 km)", and the Warp button is disabled and says the
same on hover. Warp scramblers on the target block warp and cancel an
ongoing warp. Sublight movement is limited by agility (align time ≈
agility × 10 s); the afterburner multiplies speed while its cycle runs.

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
- **Warp-to-range**: `Z` and the Warp button land at the Selected Item
  **Warp to within** selector's range (0 / 10 / 50 km). The selector is
  pure state — clicking it only cycles the value; the Warp button (or `Z`)
  fires at that value, and **Orbit range** / the Orbit button / `Q` follow
  the same split. Both selectors start at their first entry, so the
  default warp lands *on* the target and the default orbit is 2,500 m.

**Ammo cycling.** Right-click a weapon cell in the module rack for its
context menu: **Show Info** for the module, **Show Info —** the loaded
charge, and **Cycle charge**, which steps the loaded charge **Auto → each
ammo of that weapon's class that sits in your cargo → Auto** (the sim
validates class and cargo, and logs every load).
The module tooltip — hover any rack cell for 300 ms — is a structured
panel: class icon, **bold** module name, a dim `slot · class` line, then
sectioned stats with values right-aligned on one column (damage mix,
range, tracking, loaded charge + cargo count, cycle/capacitor) and
footers for what the mouse does. The Fitting window's module rows show
the same panel, and a warp inside 150 km explains itself as "Warp
unavailable". Non-weapons don't cycle.

## Currency

The currency of Entieve is the **euro**, displayed as `12,345.67 €` (the
sign trails the amount; anything under one euro shows as `50¢`). You earn euros from NPC bounties,
selling loot and market trades, and spend them on market buys. Every readout
that quotes money uses that one format — the wallet on the HUD, the market
title and the contract boards — so a reward line on the board reads
`40,000.00 € · +0.10 standing`.

## Contracts & standing

Every station with a contract office runs a **board** of contracts — the
kind of repeatable work an EVE pilot knows: cull rats, haul cargo, mine
ore. Boards are **per station**: while docked, the **Station** window
shows two sections.

**Available contracts** — the docked station's board. Each row shows the
title, the objective (`Destroy 1× Scuttler`,
`Deliver 2 m³ EMP Charge to Przystań`, `Mine 400 m³ Cordite Ore` — every
objective label right-clicks to **Show Info** on the item it names) and
the reward (`40,000.00 € · +0.10 standing`). Press **Accept** to take it; a
contract above your standing shows its requirement
(`Requires standing 0.20`) with the button disabled, and an
already-accepted one is marked **Accepted**.

**Active contracts** — everything you have accepted, anywhere, with live
progress (`Belt cleanup — 1/3`) and an **Abandon** button. Accepting and
abandoning both happen while docked, at the station that issued the
contract; abandoning costs only the progress made so far.

**Lifecycle.** `accepted → progress → ready → auto-claimed`. Kills count
rats of exactly the requested class, mining counts the requested ore
mined into your hold while the contract is active, and a delivery wants
its cargo sitting in your hold. The claim is automatic the moment you
dock at the **claim station**: the offering station for kill and mining
contracts, the destination for a delivery — which consumes the delivered
cargo on the spot. There is **no expiry**: contracts wait on the board
until taken, and completed or abandoned ones can be accepted again
(boards are repeatable jobs).

**Payouts** are calibrated against the live economy rather than vibes: a
kill contract pays a multiple (2.0–3.0×) of its target's value — bounty
plus expected loot — so rat work has the highest ceiling; a mining
contract pays a flat ~2.0× premium over the ore's base value; a hauling
contract pays 1.5× the cargo's value plus a flat fee per gate jump, which
makes delivery the dependable-margin option.

**Standing** is the career score the boards gate on: every completed
contract raises it (`+0.10` on today's level-1 boards, `+0.15` on
level-2), and level-2 boards require **0.20** standing — two level-1
completions — before they hand out their orders.

Note that Kuźnica now has a station, **Przystań**, so the insurer can hand
over the replacement hull locally: a pilot destroyed over Kuźnica has their
escape pod picked up at Przystań — their last in-system docking, or the
system's first station otherwise — instead of being shipped back to
Starogród. Death is paperwork; the log line `Respawned at <station> in a
<hull>` is the insurer's replacement notice arriving.

## The systems of Entieve

Three zones grade the map by **charter reach** — how far the Charter
Authority's patrols, courts and insurers actually work: **Charter space**,
**the Fringe** and **Unincorporated**, the lanes an EVE pilot would call
high-sec, low-sec and null-sec.

You start in **Starogród** (Charter space, constellation *Kresy*): the
first-wave heartland, safe lanes and working paperwork. *Jernborg* station
(market, inventory and fitting) is the oldest town's berth, an asteroid
belt sits close, and a gate runs on to Kuźnica.

**Kuźnica** is the Fringe — the junction of the rush: NPCs are aggressive,
the belt holds richer ore (cordite, lucidite), and the anomaly **Gniazdo**
hides Marauders plus a Hierarch boss with the best loot. Its station,
**Przystań**, is a freeport: it runs the level-2 board and doubles as the
local insurance point (see Contracts & standing).

**Pustkowie** is Unincorporated dead-end frontier — Old Works country,
deliberately dangerous: glacierite belt ore and heavy spawns including
Hierarchs, with no patrols at all. Travel system-to-system by flying to a
stargate and pressing `J` — the starmap (`N`) shows the gate graph with
your current system highlighted.

## Living universe

The systems keep moving while you do something else. Three overlapping
systems run underneath the economy:

**NPC convoys haul cargo on the lanes.** Each system runs freighters
(along the trade routes — they appear in the overview as **Tabor**) on
its own schedule: they spawn at the route's origin, warp the leg exactly
like a player ship — align, tunnel, land — and deliver their cargo at the
station endpoint, or leave the system at a gate. They are ordinary ships:
lock one, shoot it, or just watch it work.

**Guard patrols respond to incidents, never the player.** Patrols are
friendly: they do not fire on you no matter how close you fly — they only
defend themselves if you attack them first. Charter patrols (overview name
**Custos**) circle their anchor station and warp to a convoy ambush when
one breaks out, scaled by zone: a three-ship wing lands on the spot in
Charter space, a single responder arrives after about 15 seconds in the
Fringe, and Unincorporated Pustkowie has no patrols at all.

**Rats ambush convoys in the Fringe and Unincorporated.** Every convoy
transit in Kuźnica has roughly a 1-in-6 chance of being jumped, Pustkowie
about 1 in 3, Starogród never. On a hit, one to three raiders of that
system's own rat class drop in mid-leg — renamed for the job, so a
Marauder hitting a haul becomes a **Marauder Ambusher** in your overview.
The hauler is dragged out of warp and burns for 20 seconds before
attempting its warp-out; scramblers pin it, and the arriving patrols pull
the raiders' guns off it. The incident ends one of three ways:

- **destroyed** — the hauler dies leaving a **lootable cargo wreck**
  holding the exact shipment (fly over it and press `L`);
- **escaped** — it warps clear and delivers anyway;
- **saved** — the raiders are all dead while the hauler still lives.

You can intervene in any of them: strip the wreck, guard the convoy
yourself, or let the rats do the work and take the loot.

**Markets remember traffic.** A delivery books its exact units into the
destination stock and nudges that item's price *down* (supply arrived);
a shipment lost to an ambush steps the destination's price *up* — the
risk premium. Both stay inside the system's charter-reach band, **±5% in
Charter space, ±10% in the Fringe, ±20% in Unincorporated**, and a
deviating price decays halfway back to its base every minute, so lanes
that run get cheaper, dangerous lanes get dearer, and nothing ever runs
away with the economy.

## Time acceleration

`]` / `=` cycle time forward (1× → 2× → 4× → 8×), `[` / `-` cycle back. The
HUD shows the current `×N` next to the speed readout. Acceleration simply
runs more fixed 60 Hz simulation steps per rendered frame — determinism and
replays are unaffected.

## Save / load

- **`Ctrl+S`** writes a full JSON snapshot (`sim.Snapshot()`) to
  `os.UserConfigDir()/entieve/save.json` (override with `--save`) and
  the window layout alongside it (`save.json.layout.json`).
- **`F9`** restores that snapshot mid-run and rebuilds the interface
  against the restored world, then re-applies the saved window layout.
- `--load <file>` restores a save at startup.
- The `Esc` menu offers the same Save / Load plus Resume and Quit.
  Quitting from the menu also writes the layout, so your window
  arrangement survives a restart even without saving the game.

Saving is the only way progress persists: quitting from the menu does not
autosave. (The window layout is the exception — it is written on every
clean quit as well.)

**Restoring mid-run rebuilds the interface**: the selection clears and
window positions return to the layout saved with the game (their
defaults, if there is no sidecar yet), while the log history
carries across the rebuild — the simulation itself comes back exactly,
including docked state and any windows you had open.
