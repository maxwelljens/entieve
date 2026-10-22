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
| `B` | Toggle manufacturing window (manufacture while docked) |
| `N` | Toggle starmap |
| `H` | Toggle help window |
| `P` | Toggle settings (UI scale, fullscreen, window layout) |
| `K` | Toggle finances window (holdings and transactions) |
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
| Left double-click | Show Info — open the item's details window directly, anywhere the item appears (rack, fitting slots and hangar modules, market, inventory, overview, contract rows); on a **hangar module** it fits that module into the first empty slot of its class; on empty space, fly straight toward that point |
| Right click | Orbit target under cursor (2,500 m) — in the world; unchanged |
| Right click | On an item anywhere it appears: flat context menu — **Show Info** first (always bold), then the surface's actions (grayed-but-visible when impossible); on a weapon cell in the module rack: Show Info / Show Info — `<charge>` / ─ / **Cycle charge** |
| Middle drag | Pan camera |
| Wheel | Zoom (2×10⁻⁹ – 4×: the whole system down to a grid); over a table (overview, market, inventory, fitting, hangar), scroll it — wheel up moves toward the top |

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

**Multi-pane windows** — Inventory, Fitting and Manufacturing — carry
**draggable seams** between their lists: the seam is a recessed track with
a dotted grip in its middle (the grip lights up under the pointer) and an
about 8 px grab band (the cursor turns into the up/down arrow over it),
and dragging it trades height between the two lists it separates, so a
long hangar or job list can be given the room. Each pane keeps a minimum
height, so nothing collapses, and the proportions are saved with the
window's layout (see **Your layout persists**).

Toggle keys open and close their window (`I`, `F`, `G`, `B`, `C`, `N`, `H`,
`P`, `K`, `Shift+L`) — windows have no separate close button. The same
toggles live on the **left navigation rail**: one glyph per window in
six sections — **Ship** (Overview, Selected Item, Fitting), **Trade**
(Market, Inventory, Finances), **Industry** (Manufacturing, Refinery), **Station** (Station
Services), **World** (Scanner, Starmap, Log) and **Help & System**
(Help, Settings). Hover an entry for its tooltip (icon, name and hotkey
hint); click once to open — a freshly opened window lands on top — and
again to close it. An entry you cannot use right now is **grayed, never
hidden** (the services view and manufacturing in space, the scanner while
docked, fitting at a station without the fitting service); clicking a
grayed entry does nothing. A window that is open wears the entry's amber
glow, and the glyphs are colour-coded by section — **Ship** cyan,
**Trade** green, **Industry** orange, **Station** gold, **World** blue,
**Help & System** silver — so the rail reads group-first at a glance
(a grayed entry stays dim whatever its group). The slim tab at the rail's
top edge collapses the rail to its
tab — a setting that persists as `rail_collapsed` in
`<save>.settings.json` (a missing key means expanded). The ship status
and the module rack are permanent ship chrome and have no rail entries, and
the Esc menu keeps its own key — the rail is not its launcher.

The **Settings**
window (`P`) holds the **UI scale** stepper (0.75× – 2.5×, applied live —
every window, control and font rescales and keeps its arrangement), a
**fullscreen** toggle and **Reset window layout**; the scale, the
fullscreen flag and the rail's collapse state persist next to the save
in `<save>.settings.json`, and
the window arrangement itself persists in design units, so one saved
layout works at every scale. Icons (from
[game-icons.net](https://game-icons.net), credited per-icon in
`uikit/icons/CREDITS.md`) mark service titles, action buttons, overview
kinds and contract types — monochrome glyphs tinted to the theme, and
every icon keeps its text label. Two panels are
**always-on chrome** and cannot be closed or moved: the **ship status**
panel (bottom center left half: the hull portrait, then a readout
column — shield / armor / structure bars with their current/max values
inside, the velocity wedge and its m/s figure immediately right of them
on the same band, and the capacitor bar spanning the full column
beneath) and the **module rack**
(bottom center right half; each cell is a class-glyph tile with the
module's name under it and a cycle ring that depletes clockwise over the
tile while the module runs). A small strip above the status carries the
time multiplier and the wallet, and the **target-box row** sits at top
center. The **log** is a normal window now —
bottom left by default, toggle with `Shift+L`, draggable and resizable
like the rest. The **Overview** (top right) and **Selected Item** window
(top left) are open by default (the rail's Ship entries close and
reopen them) and draggable — a click in space selects the
target that **Selected Item** then shows and that every
`selected`-targeted key acts on. Its action buttons (Approach, Warp,
Orbit, Stop, Lock, Dock, Jump, Loot, Mine) act on that selection; the two
selectors — **Orbit range** (2.5k–50k m) and **Warp to within** (0/10/50 km)
— are pure state: clicking one only cycles its value and never issues a
command. Warp, Orbit, `Z` and `Q` all act at the range their selector
currently shows.

Default positions: overview top-right, selected item top-left, scanner
in the center column under the top row, log bottom-left, the HUD pair
(ship status + module rack) bottom-center, starmap and the Esc menu
centered, help top-center. Every window type also carries its own default
size and position for its first open — the market's square, the inventory
and fitting rects, the manufacturing and refinery, finances and settings — and a genuinely
new window type with no declared position opens centred. While docked the
Station window (contract board included) takes the top-left column
(overview and selected item step aside); the service windows open at
those same per-type defaults, not tiled into a docked band.

**Resizing never stomps your arrangement.** Those defaults only
ever apply to windows you have never touched: drag any window somewhere
else, resize the game window, and it stays exactly where you put it
while the untouched ones re-place on their defaults.

**Your layout persists.** Every named window's position, size and
open/closed state (overview, selected item, log, market, inventory,
fitting, manufacturing, refinery, scanner, map, help) is saved to a sidecar file next to the
save — `<save path>.layout.json`, e.g.
`os.UserConfigDir()/entieve/save.json.layout.json` — whenever you save
(`Ctrl+S` / menu **Save**) and again when you quit cleanly from the
menu. The pane proportions of the split windows (inventory, fitting,
manufacturing) ride along in the same file, so a dragged seam comes back
with the rest of the arrangement. It is read back on startup and after
loading a save, so a
favorite arrangement survives restarts and loads. Restored windows are
clamped to their minimum size and to the screen (always fully
on-screen) — the clamp is re-established whenever you resize the game
window too, so a layout saved at a larger resolution can never strand a
window off-screen — and only what no longer fits slides back in; a
missing, corrupt, wrong-version or partial sidecar simply falls back to
the defaults, and unknown entries in the file are ignored. **Your saved
arrangement wins**: a window with a sidecar entry is never recomposed —
the defaults only fill in the windows the sidecar does not mention. Every
window is ordinary, so a saved overlap is honored as-is (it is your
arrangement), and open/closed state is always honored.
The layout sidecar is deliberately separate from the save itself — the
simulation save format is unchanged for the CLI tools.

### Docked at a station

Docking (`D`) opens exactly one window: the **Station Services** view —
the docked-at identity (station name · system), the **Undock** button,
a **Cargo** button and one button per service *this* station offers
(Market, Fitting, Industry — a service the station lacks is not offered
at all) over the contract board (see
[Contracts & standing](#contracts--standing)) — and hides the space
chrome (ship status, module rack, scanner) together with the
**Overview** and **Selected Item** windows. Nothing else auto-opens:
the market, fitting, inventory and manufacturing are ordinary windows you
open from the services view, their keys, or the rail, and they open at
their own per-type defaults exactly as they do in space — the services
view keeps the station's left column. Any arrangement you saved or
dragged wins over the defaults (see **Your layout persists**).
Undocking closes the services view and leaves every window you opened
exactly as it was, restores the space chrome and the overview/selected
pair, and resets the camera view. The layout sidecar records the state
you last saw (nothing is force-open any more), so quitting while docked
reopens precisely that arrangement. The log stays mounted while docked
so its history comes with you.

While docked the **hangar backdrop** fades in over 12 frames under every
window and the rail — `assets/artwork/spaceship_hangar.jpg`, cover-fit to
the window, captioned **Docked: &lt;station name&gt;** in the lower-centre band
(see [ART.md](ART.md#docked-backdrop)). Without that asset (released
binaries ship no `assets/`) the station view keeps its plain black veil.
Undocking cuts straight back to the world — the fade is entry-only.

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

Every window opens with the def's description prose, word-wrapped to the
window's own width and re-wrapped whenever the window is resized narrower
or wider; long pages scroll inside the body instead of pushing the rows
out of view. The menus per
surface: rack cells — Show Info (module) / Show Info — `<charge>` / ─ /
**Cycle charge**; fitting slot tiles and hangar rows — Show Info / Fit /
Unequip; market rows —
Show Info / Buy / Sell (grayed while undocked); inventory rows — Show
Info; overview rows — Show Info / Warp to / Orbit / Approach / Lock target
(the entity's hull or caste info plus the usual commands); contract board
rows — Accept / Abandon (grayed when gated); contract objective labels —
Show Info.

## EVE concepts primer

**Slots.** Hulls have **high**, **mid** and **low** slots. High slots hold
weapons and mining lasers (toggled with `F1…F8`). Mid slots hold propulsion,
ewar and capacitor modules. Low slots hold tank (armor/resist) modules.

**Mounts and fitting limits.** Every high slot is a **mount**: a **turret**
mount or a **launcher** mount, of a size — small, medium or large. Guns and
the mining laser are turret-mounted; rockets and missiles are
launcher-mounted. A module fits a mount only when the kinds match and the
module is no bigger than the mount, so a hull's mounts are its design
identity: a frigate's small turrets never take a large autocannon, a
destroyer's battery of small turrets is a brawler, and only a heavy machine
hull carries large mounts. Modules also declare a **grid class** and a **cpu
class** (Class I, II or III, or none on an axis they do not use), and each
hull carries a **class quota** per axis: a fit is legal only while no class's
module count exceeds the hull's allowance. Slot count is therefore not the
only limit — a hull with empty slots can still refuse another module
because its grid or cpu class is spoken for. Refused fits log a warning and
leave the module in the hangar; unfitting a module frees what it demanded.

**The fitting window** (`F`) is laid out like EVE's: the hull's portrait,
name, class · maker and corporation emblem head the left column, with the
derived stats beneath it — shield / armor / structure maxima, capacitor
capacity and recharge, speed / align / signature, warp / targets / scan,
cargo — as dense label/value rows, plus a per-type **EHP** row (the three
layer maxima over the penetrating share each of the four resists leaves).
The **slot grid** sits right of it: one 50×40 tile per hull slot, the
module's class glyph and short name on the fitted ones, a dashed
placeholder where a slot is empty, so the grid always shows the hull's
shape. Click a tile to select it, right-click for **Show Info** /
**Fit module** / **Unefit slot**, double-click a fitted tile for its
details. Under the grid sits the **Hangar**: **fit** modules from it into
their first free slot (double-click a module does exactly that) and
**unfit** them back, and **assemble** stored hulls. A **Fitting summary**
strip closes the window: the fit's **Total DPS** (each weapon's loaded or
selected shell over its fire period, exactly the shell the bay would fire),
the **Cap drain** of the running modules against the
**Cap recharge** curve's peak, and the verdict — **Cap stable** or
**Cap unstable (~Ns)** — over a drain-vs-recharge bar. The slots/hangar
seam still drags, and the arrangement persists.

**Weapons and shells.** Projectile guns come in two families:
**autocannons** (fast cycle, high tracking, short optimal range) and
**artillery** (slow cycle, poor tracking, long optimal range, a heavier
volley out of a longer shell). Both are built in three calibres — small,
medium, large — so there are six **shell families** (small/medium/large ×
autocannon/artillery), each with four damage profiles (EMP, fusion,
armor-piercing, plutonium). A gun loads only the shells of its own family.
The shell carries the damage mix; the gun's multiplier and cycle decide how
hard and how often it lands. Projectile guns draw no capacitor.

**Bays.** Every gun that loads shells — and the cap booster — carries a
**bay** sized in m³. A **load cycle** fills it from cargo with a full bay of
one shell type and costs one firing cycle, so a reload pauses the gun for a
beat (about every forty small autocannon rounds, or every ~120 s of
sustained fire; twenty-five cap booster cells). With one shell type in
cargo the bay looks after itself: pre-loaded when fitted, topped up while
that type lasts, and switched when it runs out and one other type is left.
With two or more types in cargo the gun **waits for a choice** — the rack's
**Cycle charge** picks the type (`CmdSetAmmo`), dumping the loaded rounds
back to cargo and loading the new ones. A gun with no matching shells at
all is dry and warns `no charges`; a waiting gun stays quiet. Loaded rounds
travel with the ship: unfitting the gun returns them to cargo, and a wreck
carries whatever was in its bays when it came apart.

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

**Cycles are bound by time, not by toggling.** Activating a module resolves
its first cycle at once and then repeats every cycle time. Switching it off
(`F1…F8` again) stops it immediately but does not reset the cycle: switching
it back on inside the same cycle resumes at the original boundary, so
flicking modules on and off can never buy extra shots, repairs or boosts.

**Docking restores the ship.** Docking switches every module off and brings
shield, armor, structure and capacitor back to full, free of charge — and
undocking always leaves with everything switched off.

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
shell, and **Cycle charge**, which steps the bay's shell **none → each
shell of that weapon's family that sits in your cargo → none** (the sim
validates the family and cargo, dumps the old rounds back and logs the
load). "None" is the waiting state: the gun reloads by itself only when a
single type is left in cargo.
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

## Manufacturing

Manufacturing is a station service like any other. Press **`B`** for the
**Manufacturing** window (or its Manufacturing button on the services
view) — docked at a station with the **industry** service it opens there
like the other service windows (Przystań, Huta at Młynów, Doki at
Kamieniec), and it only ever mounts while docked.

The window turns **minerals into products**: pick a recipe — a hull, a
module or a batch of charges — set the number of **runs**, and press start.
The materials for every run are committed up front from your stock (the
have/need column shows exactly what you hold against what the job eats),
and the job pays a **fee of 3% of the recipe's mineral value** when it
starts. All stations share **three job slots**, so a yard running a hull
still leaves room elsewhere. The job list shows every job you own, grouped
by station; you can collect or cancel only the jobs at the station you are
docked at. Cancelling a running job returns its materials to the hangar
and keeps the fee. Collect the output into your hangar when it is done.
Ore is not a manufacturing input: it refines into minerals at a refinery
first — the two-tier chain.

## The refinery

The **Refinery** window (its button on the services view, or the rail's
Industry section) lists every ore stack in your cargo and ore hold and
previews exactly what **Refine** would yield — `floor(refine_yield × qty)`
per mineral. Refining runs in whole **100-ore batches**: the stepper offers
100, 200, 300, … capped at the selected ore's stock, so 250 ore refines as
two batches and 50 ore cannot be refined at all (the window says a batch
needs 100 ore). Refined minerals always land in the station hangar — never
in your ship's hold — and the hangar is unlimited, so a full hold never
blocks a refine; the yield preview and status say `→ hangar`. **Huta**
at Młynów and **Jernborg** run refineries.

Stations get better at their own work: every completed run raises that
station's **craft** in the recipe's family (hulls, modules, charges), up
to five levels. Higher craft means faster jobs and less material per run.
Unused craft decays slowly, so a station left idle slides back — and a
job already running is never re-timed by a mid-job level-up.

## The economy

Every station's market is an **order book**: your Buy and Sell are market
orders that take the best resting prices at that station and fill only
what the book actually holds — nothing is conjured for you. A **broker
fee** (3 % of the value) rides every buy, a **sales tax** (2.5 %) rides
every sell; the market window shows the fill and the fee line by line.
Prices are displayed on a ladder — last
trade, best ask, best bid, then the item's base price — and they move
only when real goods trade hands.

The book is not yours alone. A roster of **economy agents** — traders,
miners and industrialists with their own capital, stock and beliefs —
works every market station in the cluster: miners cut rock and sell ore,
industrialists buy minerals and run manufacturing jobs, traders quote between
the two. They post real orders, pay the same fees you do, and when a
spread between two stations is worth the freight they post **haul
wishes** that the convoy traffic works off — cargo bought at one station,
delivered and sold at another. Agents live in the normal game
(`world-default`); the showcase tour runs without them.

The rest of the ring buys and sells too. Convoy haulers carry real cargo
sourced from the stations and belts they serve — and the goods you shoot
down stay shot down. Guard and rat replacements are **requisitioned**
from the same economy: the Charter's patrols buy their replacement hulls,
fits and ammo on the local books, machine castes pay for theirs in belt
ore. Nothing spawns from nowhere, and every euro in the ring is spent
somewhere.

## The systems of Entieve

Three zones grade the map by **charter reach** — how far the Charter
Authority's patrols, courts and insurers actually work: **Charter space**,
**the Fringe** and **Unincorporated**, the lanes an EVE pilot would call
high-sec, low-sec and null-sec.

The playable map is one constellation, **Kresy** ("the borderlands") — the
Kresy ring: seven systems out from Starogród.

**Charter space.** You start in **Starogród** (constellation *Kresy*): the
first-wave heartland, safe lanes and working paperwork. *Jernborg* station
(market, inventory and fitting) is the oldest town's berth, an asteroid
belt sits close, and gates run on to Kuźnica and Rudnik. **Rudnik** is the
ring's ore country — Polish for "ore-works" — anchored by its mining
outpost **Halda** (market and repairs), with gates back to Starogród and
on to Młynów.

**The Fringe.** **Kuźnica** is the junction of the rush — four gates, NPCs
aggressive, the belt holding richer ore (cordite, lucidite), and the
anomaly **Gniazdo** hiding Marauders plus a Hierarch boss with the best
loot. Its station, **Przystań**, is a freeport: it runs the level-2 board,
doubles as the local insurance point (see Contracts & standing) and builds
things (see Manufacturing). **Młynów** is the refinery step of the chain —
its platform **Huta** refines and manufactures (market, refine, industry).
**Kamieniec** is the shipyard country: **Doki**, its depot (market,
fitting, industry), and the lane on to Głusza.

**Unincorporated.** **Pustkowie** is the dead-end frontier — Old Works
country, deliberately dangerous: glacierite belt ore and heavy spawns
including Hierarchs, with no patrols and no station at all. **Głusza** is
quiet in the wrong way; its Unincorporated freeport **Kantor** runs a
market and little else. Travel system-to-system by flying to a stargate
and pressing `J` — the starmap (`N`) shows the gate graph with every
system under its own name and your current system highlighted.

**The starmap is an interactive window** (a normal one: no dimmed
backdrop, draggable and stacking with the rest). Hovering a system lights
it and its name; **click** it to open its card in the window's top-left
corner — display name, zone, its stations and every gate destination.
**Click the selected system again** to set it as your **destination**: the
shortest gate route from your current system is drawn in amber, the
destination wears the widest ring, and the card counts the jumps. Click
the destination once more (or set the system you are already in) to clear
it. Destination, selection and hover are session-only view state — they
are never saved and never command your ship.

## Living universe

The systems keep moving while you do something else. Overlapping systems
run underneath the economy:

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
Fringe, and Unincorporated Pustkowie has no patrols at all. In Charter
space they also work the belts: a static guard takes on the pirate rats
anchored at its own celestial, and a patrol sweeps an occupied belt once a
beat, hunts the rats down and flies home only once the belt is empty. Free
rats come first — a belt you are working stays yours while there are others
to shoot — but security finishes the job: if yours is the last rat left, the
sweep takes it too. A rat they kill leaves the same lootable wreck as any
other kill; the bounty is paid only when the kill is yours.

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

**Rats also hold the belts.** They sit on every belt, the Charter-space
belts included — that is what the belt-cleanup contracts are for — and in
Charter space the patrols grind them down on their own: the wreck is left
for whoever gets there first, and the belt repopulates on its own timers.

**Deliveries land in the market.** A convoy's cargo is real: it is bought
from the origin station's own stock (or drawn from the belt, or imported
from outside the ring) at spawn and posted as fresh supply on the
destination station's order book when it arrives — listed just under the
local cost basis — which is why lanes that run get cheaper. A shipment
lost to an ambush touches no book at all: the loss surfaces as a log line
("Risk premium — …") and, if you are quick, a lootable cargo wreck with
the exact shipment. Prices form in the books alone (see
[The economy](#the-economy)); nothing drifts or decays behind your back.
Convoys also run multi-system lanes: a hauler bound for a station in
another system hops the gates and finishes its run there.

## Time acceleration

`]` / `=` cycle time forward (1× → 2× → 4× → 8×), `[` / `-` cycle back. The
strip above the ship status shows the current `×N` (with a clock glyph)
beside the wallet. Acceleration simply
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
