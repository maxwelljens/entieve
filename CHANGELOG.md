# Changelog

All notable changes to Entieve are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **UI sounds** (`game`, `assets/sfx`): five short cues from a local SFX
  pack (click, open, close, confirm, refuse), embedded through an
  `assets/sfx/sfx.go` shim and played by a new `game.UISound` seam. A mouse
  press that lands on an interactive control clicks (a button, a text
  field, a slider's grab band, a table row, a context-menu entry — window
  bodies, title bars and dead space stay silent); a window mounting or
  unmounting (Esc menu, station services, every floating window, the
  title's Settings/Keybindings) opens/closes; the quantity verbs (Refine,
  Start Job, Buy, Sell, Transfer), save-as, a keybinding assignment, a
  save-manager load or delete and the two resets confirm; a rejected
  keybind chord refuses. The settings window gains a **UI** volume slider
  (0–100%, 80% fresh install, `ui_volume` in `<save>.settings.json`) mixed
  at `master × UI`; the real players are built beside the music on the
  same one audio context (`game.NewAudio`), and every headless path gets a
  silent nop — no test decodes or plays a cue. Pack licence unknown, so it
  stays uncredited (source path documented in `assets/README.md`).

### Changed

- **Market search folds diacritics** (`uikit/screens`): the query and the
  catalog match through the canon name alphabet in both directions — an
  ASCII keyboard finds accented names ("zaklady" → Zakłady Burza,
  "druzstvo" → Družstvo) and a diacritic-typed query still matches; the
  match highlight remaps to whole letters. The fold table is the game
  font's verified diacritic set.

- **The click cue speaks for controls, not for windows** (`game`,
  `uikit`): `Game.uiClick` now asks `Kit.PressTarget` instead of `overUI` —
  the widget tree is walked from the deepest widget under the press, and
  only a button, a text input, a slider band, a table row (host tables are
  asked directly: their rows sit inside a scroll viewport the walk cannot
  descend into) or the context-menu popup is loud. Pressing a title bar,
  body, background or dead space is silent again, and so is a disabled
  button.

- **Every −/+ quantity ticker replaced by a slider picker** (`uikit`,
  `game`): the Refinery, Manufacturing, Market and Inventory windows no
  longer carry a `[-] Qty N [+]` row. A quantity is now asked for in one
  small, modal popup — the **quantity picker** — with the item line, a
  slider, the live amount, a live preview (refinery yield, run time/fee/
  inputs, trade total) and Confirm/Cancel. The Refinery and Manufacturing
  windows open it on the row you select (an ore, a recipe); Market and
  Inventory open it per verb (Buy/Sell, To Cargo/To Ore/To Hangar/Sell),
  and every row's right-click quantity action routes through the same
  picker. The refinery keeps its notched whole-100-ore ladder (sub-batch
  stock opens an empty slider with Confirm grayed and the old wording);
  everything else is freeflow 1–100. The picker re-opens at the last
  confirmed value per action, `Esc` cancels it before any window close,
  and Confirm emits exactly the intents the tickers did — no simulation
  change. Documented in `docs/GAMEPLAY.md` ("Setting a quantity"),
  `CONTEXT.md` ("Quantity picker") and `docs/ARCHITECTURE.md`.

- **Window titles are static base names** (`uikit/screens`, `uikit/theme`):
  every window's title bar now reads its own name — `Market`,
  `Manufacturing`, `Quantity`, `Order`, `Info`, `Fitting` — instead of the
  live suffix it composed (the market's wallet / price-ladder / fee
  schedule / cheapest-elsewhere quote, the manufacturing product ·
  duration · fee · level · slots blob, the picker's `<Item> — <Verb>`, the
  order window's `<Item> — Order`, Show Info's `<Page> — Info`, the
  fitting hull name and class). The figures those suffixes repeated live in
  the window bodies (board columns, the picker's preview, the book's own
  columns, the body heads). The retitle mechanism is untouched: a setter
  still re-lays and redraws the tracked label (`theme.ChromeText.Label`),
  production just passes constants.

- **Title screen backdrop is the artwork** (`game`, `assets/artwork`):
  the boot title now draws `assets/artwork/title_background.jpg` (2736×1536,
  embedded through an `assets/artwork/artwork.go` shim so releases show it
  without shipping `assets/`), cover-fit to the window under a lighter veil
  tuned to the plate — the menu, wordmark and windows sit on the artwork
  instead of the static starfield. Decoded once on the first title frame;
  with no plate (or a corrupt one) the phase keeps today's static world
  render under the old dim, and the simulation still never steps.

### Fixed

- **A long window title no longer overflows its bar** (`uikit/theme`): the
  tracked title label is now ellipsized to the bar's safe width — from its
  own start (clear of the ring dots and the leading icon) to the title
  bar's end-anchored chrome, i.e. the `×` strip — so a title can never
  paint under the close button or out of the window. A title that fits is
  drawn verbatim; `theme.ChromeText.Shown` is the drawn form (the one
  `Ellipsize` implementation now lives in `theme`, with `widgets.Ellipsize`
  delegating to it).

- **The context menu's appearance has a cue** (`game`): the ⋯ column menu
  and every row context menu now play the `open` cue when the popup
  appears. Dismissal stays silent on purpose — the press that dismisses the
  popup is the same gesture whose landing the click cue already reports.
  Pinned by `TestUISoundMenuOpen` through a real frame loop.

- **UI cues are audible and quiet where they should be** (`game`,
  `assets/sfx`, `game/screenhost.go`): the two Air FX cues the pack ships
  open/close from peaked at −44 dBFS, ~30 dB under the click, so they
  fired but could not be heard — both are gain-normalised to −14 dBFS
  (pinned by `TestUISoundCueLevels`). Mount cues stay silent for the boot
  composition and for the remount a rebuild (scale change, load) performs:
  those windows were already open.

## [0.9.0] - 2026-10-25

### Added

- **Unbind and per-row Reset in Keybindings** (`game`, `uikit/screens`):
  pressing `Backspace`/`Delete` while a row waits for its key clears that
  action's binding — it triggers nothing until rebound, and the sidecar
  stores the existing explicit `null` (no schema change). A rebound row
  offers its own **Reset**, which restores that action's default (taking
  the key back from whoever holds it); the two-step **Reset all bindings**
  stays. Documented in `docs/GAMEPLAY.md`.

- **Slider widget family** (`uikit/widgets`, `uikit/screens`): a reusable
  house slider in two modes — **freeflow** (a continuous range quantised
  only by its own step: the settings volume sliders) and **notched** (an
  explicit ladder the handle snaps to, with one dim tick per rung and the
  handle resting exactly on them: the UI-scale slider over its six rungs).
  Both share one track (thin rounded border, accent fill to the handle), a
  24 design-px grab band, click-to-position and absolute pointer drag with
  release ending the grab; `OnChange` streams the live value and
  `OnRelease` fires once per drag (the persistence point). The `P`
  Settings window's **UI scale** control is now the notched slider
  (replacing the −/+ stepper) and its window widens to 380×460 to give the
  sliders their travel; settings actions take absolute values
  (`SetUIScale(rung)`, `SetMasterVolume/MusicVolume`, `PersistSettings`).

- **Music with master/music volume and shuffle** (`game`, `cmd/entieve`,
  `uikit/screens`, `assets/audio`): the game now plays the six space
  tracks as one shuffled pool everywhere — title, space and a docked
  station — advancing to the next track when the current one ends. The
  `P` Settings window grows two freeflow volume sliders (**Master**,
  **Music**; 0–100% in 1% steps, defaults 100% and 70%) and a **Shuffle**
  toggle (default on) ahead of the reset buttons; dragging applies the
  volume live and the sidecar is written once on release, and the engine's
  output volume is master × music, re-applied live and on every track
  change. Reset all
  restores 100/70/shuffle-on, and `master_volume`, `music_volume` and the
  inverted `shuffle_off` persist in `<save>.settings.json` (absent keys
  read as the fresh-install defaults).
  - The six `.ogg` files are embedded into the binary from
    `assets/audio/` (they stay where they are; a shim beside them carries
    the `//go:embed`), so releases need no `assets/` folder and the
    binary grows by the ~14.5 MB of audio. Playback is cgo-free on every
    release target via Ebitengine's `audio`/`vorbis` packages.
  - The audio engine is constructed only on the `run` path; tests inject
    a silent fake, so no headless path creates the one audio context.

- **Rebindable keys** (`game`, `uikit/screens`): screen toggles and world
  commands can be rebound in a new **Keybindings** window (`O`, the rail's
  *Help & System* entry, or the title menu between Settings and Credits).
  Click a row and press a key; `Esc` cancels; **replace policy** means a key
  belongs to one action and stealing it leaves the old holder unbound
  (`—`). The window shows every current binding and offers a two-step
  **Reset all bindings**. Overrides persist immediately to a new
  `<save>.keys.json` sidecar (excluded from the save manager), holding only
  the player's deviations; a missing or damaged file means the defaults. The
  global chords (`Esc`, `Ctrl+S`, `Ctrl+F`, `F9`, `[` `]` `-` `=`) are a
  fixed layer and refuse capture with a notice; `Esc` stays reserved as a
  plain binding. The help overlay follows the live bindings.

- **Expanded settings window** (`game`, `uikit/screens`): the `P` window
  now also carries a **vsync** toggle (default on — an absent key in an
  existing `settings.json` keeps today's behaviour), a **rail** collapse
  toggle wired to the same persisted state as the rail's edge tab, and a
  two-step **Reset all settings** that restores 1× windowed, vsync on, the
  rail expanded and no saved window size. The window grows from three rows
  to five (300×320 default rect); **Reset window layout** is unchanged.
  - The last **windowed size** is written to `<save>.settings.json` on a
    clean quit and restored when booting windowed (clamped to 1024×700;
    fullscreen is unaffected). `vsync_off` is the inverted sidecar key, so
    files from before the setting read as vsync on.

- **Title screen with save manager, settings and credits** (`game`,
  `cmd/entieve`): `entieve run` now boots into a title screen over a static
  world backdrop — tracked amber wordmark, version line, and New game /
  Continue / Save manager / Settings / Credits / Quit. The simulation is
  frozen and gameplay chrome stays hidden until you enter; `--load`
  bypasses the title.
  - **Save manager**: the saves directory (`*.json`, sidecars excluded),
    newest first with the active save marked; Load and a two-click Delete.
  - **Save as…** in the Esc menu writes the next free auto-numbered file
    (`save.json`, else `save-N.json`, N ≥ 2) and makes it the active save;
    Save / Ctrl+S follow it. Layout and settings sidecars stay global.
  - Credits window authored from the in-repo licences (icons, module
    renders, hull/emblem art, form language, typefaces, dependencies), with
    the key strings pinned against their source files.

- **Per-module weapon art** (`uikit/icons`, `content`, `uikit`): the five
  autocannon/artillery modules show their own full-colour render instead of
  the shared class glyph.
  - The icon manifest gained a `"mode": "art"` sub-mode (full colour, no
    tint, cached per name+size) and five CC0 256×256 downscales of the
    project's renders; the game-icons glyph set is unchanged.
  - `ModuleDef.icon` names an icon-manifest key. `theme.ModuleIconFor` uses
    it when the registry knows it and falls back to the class glyph
    otherwise; the fitting/rack tiles draw it at 32 px, the Show Info module
    header and module tooltip at 24 px. `just validate` rejects an unknown
    key (`content.WithIconKeys`).
  - Every market item row (catalog, expanded orders, search hits) now carries
    a 20 px icon — a module's art, else its category glyph (ammunition, loot,
    minerals, ore, hulls) — through the new `TableRow.IconSize`; browse-tree
    branch rows stay text-only.

### Changed

- Internal window-host restructuring: every Screen row now builds a
  `widgets.WindowPanel` and a single Game-free **Screen host** mounts,
  syncs, dismisses and places it (the title's Settings/Keybindings ride a
  second host instance). The layout sidecar format and bytes are
  unchanged; a committed × close still clears the market's folds while a
  hotkey close keeps them. No player-visible change.

- **Keybindings window redesigned as a controls screen** (`uikit/screens`,
  `game`): the flat list is now sectioned — **Ship controls** (the world
  commands), **Windows** (the Screen toggles) and the dim, informational
  **Fixed shortcuts** layer — with tracked section headers, hairline rules
  and a reserved scroll gutter. Each row is the action label left and a
  fixed-width key chip right, so the keys line up down the page: a neutral
  chip for the default, an accent chip with a per-row **Reset** when the
  action was rebound, an accent *press a key…* while it waits, `—` when it
  is unbound and the disabled look for the fixed layer. The refusal notice
  now reads in a bottom status bar beside the two-step **Reset all
  bindings**; rows are 28 px and the window 480×640, scrolling through the
  whole list at every UI scale.

- **Credits window rebuilt as a proper credits screen** (`game`): a tracked
  **ENTIEVE** wordmark and a *Credits* subtitle head seven ruled sections —
  Development, Iconography, Module renders, Hull & emblem artwork,
  Interface form, Typefaces, Built with — each as a role/credit
  two-column block with generous air, closed by a licence footer. The
  window is 400×520 with the house scrollbar and title-bar chrome; every
  provenance sentence is unchanged, and `TestCreditsPinSources` still
  reads them against `LICENSE`, `uikit/icons/CREDITS.md`,
  `docs/lore/AESTHETIC.md` and `go.mod`.

### Fixed

- **The × on a title-screen window now closes it and returns to the menu**
  (`game`): the title host was handed the game's close hook, so the
  Keybindings/Settings × ran the *game* host's toggle — the title window
  stayed up, the menu stayed hidden, and the row was silently left open in
  the game host for when the player entered. The title host now gets the
  action bundle without that hook (`Game.titleServices`), so its × is the
  plain toggle its contract documents. Pinned by
  `TestTitleCloseButtonReturnsToMenu` (both title rows) and
  `TestTitleOwnWindowsCloseButtons` (Credits, Save manager).

## [0.8.0] - 2026-10-23

### Added

- **Column prefs — frozen widths, drag resize, ⋯ column menu** (`uikit`,
  `game`): every `widgets.Table` (overview, the three inventory holds, the
  market book and browse tree, the fitting hangar, the manufacturing
  recipe and job lists, the refinery ore list and the finances ledger)
  measures its columns **once** and freezes them.
  - The measurement is today's algorithm, run on the first fit — the
    first-open layout is byte-identical. From then on the frozen widths are
    what the fit consumes, so a cell growing (an overview row's Velocity
    mid-warp) or the sort arrow moving to another header **never moves an
    anchor**: content that no longer fits ellipsizes instead.
  - **Drag a column's right edge** (the 6 design-px strip in the header) to
    set its width: the strip sits on the boundary it drags, and that
    boundary follows the cursor — the next visible column gives the delta
    back, so the row keeps tiling the table exactly. A column with no column
    right of it (the last one) has no strip: the table edge is the fit's.
    The strip has its own blocking input layer, so a press there never
    sorts; the cursor is the split row's EW-resize. Floor is the column's
    header title; no ceiling.
  - A **⋯ button** in its own strip at the header's right end opens a flat
    menu with one ticked row per column: a row hides or shows that column
    (display filter — layout, draw and hit tests only; row data and cell
    indices never change), then reopens the menu at the same point. The
    entry that would hide the last visible column is grayed. The columns
    tile the header less that strip, so no column's cells or header label
    ever run under the dots; a header label wider than its column
    ellipsizes, keeping the sort arrow. The ticks and dots are drawn, not
    font glyphs.
  - Widths and hidden columns persist in the layout sidecar next to the
    window rects and seam ratios (`tables`, additive — no version bump):
    design units, so one saved set restores at any UI scale. The inventory's
    three holds share one set; the market's two lists keep their own.
    "Reset window layout" clears them with the rest.

- **Ammo selection on the rack — picker, Load entries, waiting badge, bay
  row** (`uikit`): the magazine feature's UI half.
  - A charge-fed cell whose bay **waits for a choice** (empty bay, two or
    more round types in cargo, none selected) now says so: an `Info`-blue
    corner dot on the tile (static, its own overlay — the glyph, the cycle
    ring and the 50×40 hit area are untouched), and a left click opens the
    cell's menu — the charge picker — instead of toggling. Every other
    state toggles exactly as before; double-click is still Show Info.
  - The cell menu lists one **Load** `<round name>` entry per round type
    the bay could take that sits in cargo (item-id order; the cap booster
    offers its cell too), in addition to Show Info, the charge ref and Cycle
    charge. Every entry emits the same `CmdSetAmmo` Cycle charge does.
  - **Cycle charge no longer passes through auto**: the `""` (none) entry
    is gone and cycling walks only real round types in cargo, wrapping at
    the end. The tooltip's charge row reads `None` instead of `Auto` while
    nothing is chosen.
  - The module tooltip gains a bay line on charge-fed modules — `Clip`
    `0.2 m³ · 40 rounds · 3.0 s` (bay capacity, whole rounds of the round
    it would load, one load cycle) or `waiting for ammo selection`. The
    round count reuses content's calibration arithmetic through the new
    `content.ModuleDef.BayRounds` (also used by `TestBayCalibration`); a
    module with no bay shows no row.
  - New read-only seam `widgets.ContentView.Clip` over `sim.Clip`; there is
    one implementation (`view`), so no stub needed updating. Docs: GAMEPLAY
    "Bays" / "Ammo cycling", NAMING rows (proposed).
- **Reload ring and ready marker on the rack** (`sim`, `uikit`): loads are
  now visible as they happen, and an armed bay says so.
  - **Every bay load runs a cycle.** An idle bay's top-up, an auto-switch or
    a `CmdSetAmmo` now takes the same one firing cycle an active gun's reload
    always cost at its boundary: the rounds move into the bay at the start
    (conservation unchanged) and the gun is not ready until the deadline.
    The `<module> loaded with <round> (N rounds)` line now fires at
    completion. A fresh fit at boot is still pre-loaded instantly (no UI
    exists yet). Pacing: an idle reload that used to finish in one tick now
    costs a cycle, and an NPC's first shot after a fresh bay load shifted by
    one cycle. Saves are untouched (schema 3; a load in flight restores into
    the same load — `ClipState.Loading` derives from the bay and the
    not-yet-written selection).
  - The rack cell draws the load's **cycle ring lit** (a cancelled cycle
    with its clock still running stays greyed), so a picked round visibly
    cycles in, and shows a small **shell marker** in the tile's bottom-right
    corner once the bay is loaded and not loading: armed, where the
    top-right `Info` dot means the waiting state (`needs you`). The marker
    is its own overlay — tile, name, hit area, glyph, ring and dot are
    untouched. New `uikit/icons/ammo.png` (`artillery-shell` by Quoting,
    CC BY 3.0, tinted from the palette).
- **Window artwork headers** (`render`, `uikit`): the Refinery and
  Manufacturing windows can carry a plate of their own,
  `assets/artwork/refining_header.jpg` and `manufacturing_header.jpg`
  (1280×720 JPEG), in a 120 design-px band directly under the title bar —
  part of the window background and behind the content, so the first rows of
  the list sit on the image and the band stays pinned while they scroll. The
  plate is cover-fitted (centre-cropped), composited at α180 on the title
  edge fading linearly to nothing at the band's bottom, and fades in over the
  window's 12-frame open. The list lets the plate through: the tables' scroll
  viewport leaves its body fill off under the band (a `widgets.PlateBand`
  option only these two windows pass), so the first rows read on the art and
  the fill — and every row below the band — stays exactly as before. No
  `assets/` (released binaries) or no plate: no band, the plain window body
  byte for byte. Docs: ART "Window headers",
  GAMEPLAY "Windows".
- **Market category tree and search** (`content`, `uikit`, `game`): every
  marketable def (`ItemDef`, `ModuleDef`, `ShipDef`) now carries a validated
  **`group`** path (1–3 segments, e.g. `Ships/Frigates`,
  `Modules/High/Turrets`, `Materials/Ore`), and the market window is three
  panes — **browse tree | catalog | My orders** — with two persisted seams
  (`market-tree`, `market-orders`) at a 760×560 landing rect. The tree is
  the content's own hierarchy (`All` root over Ships, Modules, Ammunition,
  Materials and Loot); a branch click scopes the catalog, an item click
  highlights its branch, and the expansion state is session-only like the
  order folds. A live search field above the tree filters the selected
  branch by name, group path and maker through
  [`github.com/sahilm/fuzzy`](https://github.com/sahilm/fuzzy) (MIT,
  stdlib-only): flat ranked results with the matched letters highlighted,
  the header sort restored on clear, `Ctrl+F` to focus, `↑`/`↓` to walk the
  results, `Enter` to select one and a two-step `Esc` (clear, then leave).
  Cross-station availability is read-only: the Qty cell adds a dim
  `· N more` suffix for the other books stocking the item and the title
  line names the cheapest such ask; **Buy** greys while no local ask
  exists (Request on Market stays live). Hulls can no longer quote 0 € in
  the market: `ShipDef` gained **`base_price`**, the price ladder's last
  rung. Docs: GAMEPLAY "The market window", CONTENT_AUTHORING "The market
  tree", CONTEXT "Market tree", NAMING rows (proposed).
- **Shell bays (clips)** (`content`, `sim`): every charge-fed module — the
  six projectile guns and the cap booster — now carries a m³ **bay** that
  holds whole rounds of exactly one shell type. A **load cycle** fills it
  from cargo with a full bay in exactly one firing cycle (no capacitor
  cost), deferring the next shot by that cycle; each shot draws one round
  from the bay. A fresh fit pre-loads, and the bay resolves itself while the
  selected type lasts: it reloads from that type, and when it runs out it
  switches if exactly one other type remains in cargo or waits (quietly, one
  line) when two or more compete. `CmdSetAmmo` picks a type — dumping the
  bay back to cargo, loading the new one and resuming after its load cycle —
  and `""` now clears the selection (the old auto-follow-cargo mode is
  gone). A gun with no matching rounds anywhere still logs `no charges` and
  deactivates, unchanged. Loaded rounds travel with the ship: unfitting or
  assembling returns them to cargo, and a wreck carries whatever was in its
  bays. The UI reads a bay through `sim.Clip` (`ClipState`: loaded item,
  rounds, capacity, selection, competing cargo types). Bays are calibrated
  for a ~120 s sustained-fire reload: small autocannon 0.2 m³ / 40 rounds,
  small artillery 0.104 / 13, medium autocannon 0.33 / 33, medium artillery
  0.275 / 11, large autocannon 0.594 / 27, large artillery 0.297 / 9, cap
  booster 0.25 m³ / 25 cells. Vocabulary: CONTEXT.md "Shell family", "Bay"
  (clip), "Load cycle", "Waiting state"; authoring table in
  [docs/CONTENT_AUTHORING.md](docs/CONTENT_AUTHORING.md); rules in
  [docs/SIM.md](docs/SIM.md#shell-bays-clips).
- **Six shell families** (`content/data/items/ammo.json`,
  `content/data/modules/weapons.json`): ammunition splits into
  family-calibres so an autocannon and an artillery of the same size no
  longer share shells. The twelve existing `ammo-*-s/m/l` ids stay and become
  the **autocannon** families (`small-autocannon`, `medium-autocannon`,
  `large-autocannon`) at 0.005 / 0.01 / 0.022 m³ per shell; twelve new
  artillery shells (`ammo-art-<profile>-<size>`) cover the artillery
  families at 0.008 / 0.025 / 0.033 m³, with recipes and display names. One
  family is one calibre: every shell of it has the same volume, which
  validation enforces alongside the bay rules.
- **Save schema 3** (`sim/save.go`): per-slot loaded rounds and ammo
  selections persist with the fitting runtime (`ModuleRuntime.Clip` /
  `.Selection`). A v2 save is migrated in place on restore — empty bays, no
  selections, and the bay rule then auto-resolves each gun — while v0/v1
  saves are still rejected. This supersedes the old "schema stays 2" note in
  [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

- **Fitting limits: mounts and class quotas** (`content/fit.go`,
  `content/data/{ships,modules}`, `sim`): high slots are now **mounts** — a
  turret or launcher of small, medium or large size — and a module fits only a
  mount of its own kind at or above its size, so a large autocannon no longer
  drops onto a frigate and a hull's mount sizes are its design identity
  (small turrets on the frigate, destroyer and patrol; medium on the Old Works
  Marauder; large on the Hierarch). Every module also declares a **grid
  class** and a **cpu class** (Class I/II/III, or none on an axis it does not
  use, a mining laser or ewar module paying on both) and every hull a
  per-class **quota** per axis; a fit is legal only while no class's count
  exceeds its quota, so free slots alone no longer buy another active module.
  A refused fit logs why (`No free launcher mount for Rocket Pod`, `Large
  Autocannon is oversize for the hull's turret mounts`, `Cap Booster exceeds
  the hull's grid Class I quota`) and leaves the module in the hangar;
  unfitting frees what it demanded. `just validate` rejects an authored fit
  (NPC, traffic or world start) that breaks either rule. Vocabulary:
  CONTEXT.md "Mount", "Turret mount / launcher mount", "Grid class / cpu
  class", "Class quota"; authoring tables in
  [docs/CONTENT_AUTHORING.md](docs/CONTENT_AUTHORING.md); the rule seam is
  described in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md). Saves are
  untouched (schema 2: defs carry the limits, saves carry module ids).

### Fixed
- Rounded chrome's top band carries the panel/title fill again (the backdrop no longer shows through where the corners begin).
- **Column widths dragged in a table now stick, and hiding a column no longer
  freezes resizing** (`uikit/widgets`): the frozen bases and the widths on
  screen used to drift apart after the first fit — a wider table gave the
  whole surplus to the lowest-priority "grow" column and a narrower one took
  the shortage from the Priority order — so a grip edited a number the player
  could not see. In the market, the Item column bottomed out at its floor plus
  the surplus ("cannot make Item smaller than a very large size", with prices
  and quantities stuck ellipsized); in the overview, hiding a column freed
  width that no grip could pass on, and the columns sitting on their header
  floors made every other boundary inert. Every layout after the freeze now
  spreads the table width over the frozen bases **proportionally** (ratios
  preserved, rows still tiling exactly; a short table still gives width back
  in Priority order toward the floors), and a grip press adopts the laid-out
  widths as the base, so a drag moves exactly the boundary on screen. Default
  layouts are unchanged — the first fit is byte-identical — and a hidden
  column still restores its frozen width when shown again.

### Changed
- **Chrome adopts the Cyber Terminal kit's form language** (`uikit`, `docs`) — shapes only, the amber-on-black palette is pixel-identical (its golden values and the log severity hexes pass untouched): rounded chrome on three radius tokens (14 window pieces, 8 panels/cards, 5 controls), window title bars top-rounded and bodies bottom-rounded around a square seam; three hollow 9 px ring dots at every title bar's left; a subtle top→bottom depth ramp on panel and window-body fills (ΔRGB ≤ 6, ±4 alpha around the 230 translucency token); tracked mono (Plex Mono, 0.12 em) window titles, panel heads and section heads; the starmap's 28 px lattice at 3.5 % accent; and the rack's waiting dot pulsing through eight pre-rendered phases. Kit credit (Kerem Bolek) and the new form canon: [docs/lore/AESTHETIC.md](docs/lore/AESTHETIC.md), implementation substrate in [docs/ART.md](docs/ART.md).
- Volume in lists and aggregates (inventory rows and totals, fitting capacities, tooltip stats) floors to two decimals with trailing zeroes trimmed — a cargo sum of `0.7000000000000001 m³` reads `0.7 m³`; the Show Info window keeps full precision.

- **Naval calibre rename** (`content`, `docs`): ammunition display names now
  carry calibre and payload (`40mm ion`, `57mm armour-piercing`,
  `152mm high-explosive`, `203mm plutonium`, …; autocannon 40/57/76 mm,
  artillery 127/152/203 mm) and the autocannon market branch reads
  **Ammunition/Autocannon rounds**. Minerals read **Sideron**, **Nixium**,
  **Zaffrite**, **Vetril**, **Cryolith**, **Helion**; the ores **Kerngrit
  Ore** and **Borealite Ore** (contract "Kerngrit order"). Ids and all
  balance numbers are unchanged; generic ammunition prose now says round.

- **Station books stock artillery shells** (`content/data/systems`): every
  station that sells an artillery gun now lists its family's shells too, and
  the systems whose belts/anomalies spawn Hierarchs (Kuźnica, Głusza) stock
  large artillery shells so respawn requisitions can buy them. The Jernborg
  supply-run contract's objective is 1 m³ (200 small autocannon shells at the
  new 0.005 m³ volume) for the same 86,000 € reward.

- **Belts are numbered, gates name their destination** (`content/data/systems`):
  every belt is now **Asteroid Belt I** (numbered per system in celestial
  order — the next belt in a system becomes II), and every gate is
  **Gate (<destination>)**, e.g. `Gate (Kuźnica)` where the old form read
  `Kuźnica Gate`. Ids are unchanged; [docs/lore/NAMING.md](docs/lore/NAMING.md)
  carries the new convention.
- **Security forces clear the belts** (`sim/system_npc.go`,
  `sim/system_traffic.go`, `sim/system_damage.go`): a static guard now
  engages the pirate rats anchored at its own celestial in a high-security
  system, and a traffic patrol sweeps an occupied high-sec belt once a
  patrol beat — warping there, killing the rats and flying home only when
  the belt is empty. A sweep hunts stragglers off the anchor instead of
  holding station, engages free rats first (a belt you are working stays
  yours while others are free) and finishes the belt over kill-stealing
  when a player-touched rat is the last one left; the Fringe and
  Unincorporated get no response, and the last rat of a belt dying logs one
  `<belt> cleared` line while the belt's own respawn timers re-arm it. The
  rats return fire, and their wrecks are lootable exactly as after any
  other kill — the bounty is still paid only for kills attributed to you.
  Rudnik (Charter space) now fields the standard patrol schedule too, so
  its belts get sweeps instead of static guards alone. New vocabulary:
  CONTEXT.md "Belt sweep", [docs/SIM.md](docs/SIM.md) ("The beat"),
  [docs/GAMEPLAY.md](docs/GAMEPLAY.md) ("Guard patrols …").
- **Small autocannon reach is 10 km** (`content/data/modules/weapons.json`):
  the small autocannon's falloff drops 9000 → 6000 m (optimal 4000 m and the
  3 s cycle unchanged), so its 50 % range is 10 km and the tail dies by
  ~16 km — the Scuttler's 17 km pokes no longer land. Medium and large
  autocannons and the artillery family are unchanged, pending their own
  review. The fitting tooltip and the weapons pin follow the new number.
### Added

- **Rail glyphs are colour-coded by section**: Ship cyan, Trade green,
  Industry orange, Station gold, World blue, Help & System silver
  (`theme.RailGroupColor`, palette roles only). A grayed entry keeps its
  dim look whatever its group, and the open-state amber cell glow is
  unchanged.
- **Docked hangar backdrop**: while docked, `assets/artwork/spaceship_hangar.jpg`
  fades in over 12 frames as the full-window backdrop under every window and
  the rail, cover-fit to the window and captioned **Docked: &lt;station name&gt;**
  in the lower-centre band. The `assets/artwork/` kind accepts JPEG; with no
  artwork the station view keeps its plain black veil, exactly as before.
  Undocking cuts straight to the world (the fade is entry-only).
### Added

- **The starmap is interactive and named** (`uikit/screens/mapview.go`):
  nodes carry their system's display name (Starogród, Kuźnica, Pustkowie …)
  instead of the internal `sys-*` id. Hovering a system lights it and its
  name; a click opens that system's card in the window's top-left corner
  (display name, zone, its stations, every gate destination); clicking the
  selected system again sets it as the destination and draws the shortest
  gate route from your current system in amber, with the destination ringed
  and the jump count on the card (click again, or pick the system you are
  in, to clear it). Pointer input is read through the kit's shared cursor /
  press seams, so clicks reach the window body and the window rect keeps
  gating the world. Hover, selection and destination are session-only view
  state — never saved, never a sim command.

### Changed

- **The starmap is an ordinary window, not a modal overlay**: `N` toggles it
  like every other keyed window (the rail entry, the × and the sidecar
  restore behave the same way), and the dimmed full-screen `MapScene`
  backdrop is gone, so the map no longer blocks the windows under it or Esc.

## [0.7.0] - 2026-10-21

### Added

- **The Fitting window, rebuilt on EVE's shape** (`uikit/screens/fitting.go`):
  four zones over the same three actions and the same `fitting-hangar`
  seam. The left column leads with the fitted hull's framed portrait, its
  name and a `class · maker` line carrying the corporation's emblem; the
  old stat text blob becomes dense label/value rows (shield / armor /
  structure, capacitor, navigation, targeting, cargo) with a per-type
  **EHP** row, `(max shield + armor + structure) / (1 − resist)`. The high
  / mid / low slot table becomes a **tile grid** in the rack's visual
  language: one 50×40 tile per hull slot, the module's class glyph and
  short name on fitted cells, a dashed placeholder where a slot is empty.
  A **Fitting summary** strip shows the fit's **Total DPS** (each fitted
  weapon's loaded shell over its fire period — the ammo preference and
  cargo order the sim itself uses), the **Cap drain** of the running
  modules against the **Cap recharge** curve's peak, and the **Cap stable**
  / **Cap unstable (~Ns)** verdict over a drain-vs-recharge bar; the window
  moves to a 644×682 default at (370, 8) to hold it (was 305×434 at 709, 256).
- **Double-click a hangar module to fit it** into the first empty slot of
  its class; the row menu's Fit-into-selected-slot stays as the fallback,
  and Show Info stays on the hangar row menu (double-click on a slot tile
  still opens it).
- **`ContentView.PredictedAmmo(sim.SlotRef)`**: the charge the sim would
  load into a fitted weapon slot right now (the `findCharge` rule once:
  the `CmdSetAmmo` preference while it sits in cargo and matches the
  gun's class, else the first cargo stack of that class).

- **Hull portraits** (`uikit/hulls`): nine original top-down portraits — one
  per fleet hull — embedded at 120 and 240 px and keyed by `ShipID`
  (`Get`/`Has`). Flat SVG sources under `uikit/hulls/svg/` are rasterized at
  build time by `tools/hullraster` (a separate module, no runtime
  dependency); the manifest hashes pin the committed PNGs to their sources.
  The art is original, in steel greys with one accent light per hull, and
  reads at 72 px.
- **Hull portrait** in the ship status panel: the fitted hull's 120 px
  portrait (art stream package `uikit/hulls`) replaces the placeholder
  diamond and the three concentric damage arcs, and repaints when the hull
  changes. A stub `uikit/hulls` ships this batch and is dropped at
  integration.
- **Velocity wedge**: a vertical triangle widening bottom→top, filled
  bottom-up with current / base max speed (`Motion.MaxSpeed`, a stable
  top), sitting in the health bars' band with the `m/s` figure beneath it
  (see Changed: the wedge moved beside the bars).
- **Clock icon** (`hud_time`, game-icons.net `stopwatch` by Lorc, CC BY
  3.0) on the new time/wallet strip.
- **Cycle rings** over the rack tiles: a thin ring depletes clockwise from
  full at activation (time remaining), shown only while the module runs.

### Changed

- **Cycles can be cancelled, not replayed**: deactivating a module stops it
  at once but keeps its cycle clock, so re-activating inside the running
  cycle resumes at the original boundary instead of restarting it. The old
  clock reset let a deactivate/activate pair replay the effect every tick —
  afterburners, tank, ewar, cap boosters, guns, mining lasers and NPCs (same
  `activate`/`deactivate` core) are now bounded by one effect per cycle time
  under any toggle pattern.
- **Docking restores the ship, free**: docking clears every module's active
  state and cycle clock and refills shield, armor, structure and capacitor;
  no wallet movement, no ledger entry, no repair service needed. Undocking
  always leaves with everything off.
- **The velocity wedge sits beside the health bars** (`uikit/screens/hud.go`):
  the wedge and its `m/s` figure moved from a stack under the bars onto the
  bars' own band, to their right. The health block narrows to 98 design px
  (bars + gap + wedge = the capacitor bar's 128 px, which still spans the
  column beneath), the status panel drops from 223 to 150 px of height, and
  the recharging hint no longer changes the panel's height when it appears.
  The bottom-center pair stays 556 design px wide, so its 1280×720 clearance
  from the log pocket is unchanged; the pair's top edge rises from 457 to
  514 (the rack sets the pair height now).
- **Show Info descriptions word-wrap to the window** (`uikit/screens/infowin.go`):
  the description paragraph wraps at the scroll viewport's inner width —
  not at the page body's own width, which a long paragraph used to pin
  wider than the window, clipping its tail — and re-wraps when the window
  is resized or a new page is swapped in at the same width.
- **One legacy-id registry, and restore finds every content id itself**:
  `content.LegacyIDs` is the single retired-id table — validation
  (`content.RetiredIDs`) and save restore read the same list — and restore
  walks the whole `Save` type graph by reflection instead of a hand-written
  field list, so retired ids in an ore hold, an asteroid or a belt rock now
  migrate too (they silently did not before) and a new id-bearing save
  field is covered the day it lands. Save schema stays 2, and saves using
  current ids still restore byte-identically.
- **HUD and module rack are one centered bottom pair**: the ship status
  (hull portrait, shield / armor / structure bars, capacitor, velocity)
  sits left, the rack immediately right, both on the 10 px bottom-center
  baseline; the target-box row stays top-center.
- **Health is three bars** with `current / max` printed inside each
  (shield/armor/structure palette), replacing the three text lines; the
  capacitor bar keeps its value inside and loses its "capacitor" caption
  (the recharge hint keeps a reserved line).
- **Time multiplier and wallet moved to a strip above the status**, with a
  clock glyph; the old `×N time` line leaves the status panel. No sim
  clock is shown.
- The rack's 4 px cycle bar is gone in favour of the tile cycle ring; tile
  size, glyph centering, names, hit areas, tooltips and menus are
  unchanged.
- **Station services have one owner** (`content/services.go`): the service
  ids, display labels, icons and the window each opens live in
  `content.Services`, and the rail's service gate, the services-row buttons
  and the sim's command gates derive from it. A renamed window now fails a
  test instead of leaving a button that toggles nothing.
- The four quantity steppers (Market, Inventory, Manufacturing, Refinery)
  are one `widgets.QtyStepper`: same ± hit boxes, tabular readout and
  relayout rule everywhere, with the refinery's batch ladder supplied as
  steps. Screens now read the derived figures (hold use/capacity, price
  ladder, contract progress/standing, tradeable name) from the ContentView
  seam instead of re-deriving them; the refine batch size (`sim.RefineBatch`)
  and the km distance label (`sim.KmLabel`) are single-sourced.

### Fixed

- **Refined minerals go to the station hangar, always** (`sim/system_economy.go`,
  `uikit/screens/refinery.go`): `CmdRefine` used to add its output to the
  ship's general cargo and refuse with a cargo-full event when it would not
  fit. The output now lands in the station hangar (the store market buys
  already land in, which has no capacity model), so a full ship hold never
  blocks a refine; the window's cargo-full prediction is gone and its yield
  preview and status name the destination (`→ hangar`). Ore input is
  unchanged — general cargo first, then the ore hold.

## [0.6.0] - 2026-10-21

### Added

- **Draggable split panes** in Inventory (cargo/ore, ore/hangar), Fitting
  (slots/hangar), Manufacturing (recipes/jobs) and the Market (order book /
  My orders): the pane seam is a recessed 4 px track with a centred
  three-dot grip that lights up under the pointer, an 8 px grab band and a
  resize cursor (up/down in the stacked windows, left/right in the market);
  dragging it trades height (or width) between the two lists, each pane
  keeps a minimum size, and nothing else in the window moves. The market
  opens where its fixed 240 px My-orders column stood. The proportions
  persist in the layout sidecar (now version 3) under named seams and are
  restored with the rest of the arrangement; version-2 sidecars still load
  and simply start at the default shares.

### Changed

- **Manufacturing and Refinery have their own icons** (game-icons.net
  `factory` and `furnace`, both by Delapouite, CC BY 3.0, credited in
  `uikit/icons/CREDITS.md`) on the window title bar, the navigation rail
  and the station-services strip, instead of borrowing the module and
  ore-belt glyphs.
- **Refining is atomic in 100-ore batches** (`sim/system_economy.go`,
  `uikit/screens/refinery.go`): ore refines only in whole batches of 100, so
  a fraction of a unit per ore can no longer floor to nothing while eating
  the ore. The Refinery stepper offers 100, 200, 300, … capped at the
  selected ore's stock (250 ore caps at 200), and under 100 ore the button
  is grayed with a line saying a batch needs 100. `CmdRefine` with any other
  quantity is refused untouched with a clear log line; a stock that is not a
  multiple never refines partially.
- **Factory is now Manufacturing** (`uikit/screens/manufacturing.go`, the
  `pManufacturing` Screen row, key `B` unchanged): the industry window is
  renamed per the approved design. The layout sidecar key follows
  (`factory` → `manufacturing`); version-2 sidecars still map the old key on
  read, so a saved window rect survives the rename.
- **Three global job slots**: manufacturing jobs at every station share one
  3-slot pool (was 2 per station). The job list now shows all of the
  player's jobs grouped by station; Collect and Cancel stay limited to the
  docked station.
- **Duration ladder retuned** (`content/data/recipes/modules.json`,
  `ships.json`): modules run ~3 minutes and hulls ~10 minutes per run at 1×
  (ammo's ~30 s rung is the parallel ammo pass). Runs still multiply
  linearly and craft levels still discount time 4% per level.
- **Saves from before the shell-weapons rename load cleanly**: restore
  remaps the retired content ids in place — `charge-emp/fusion/ap/plutonium`
  to the small-calibre shell of the same damage profile, `mod-emp-turret` to
  the small autocannon, `mod-rail-turret` to the small artillery, and the
  four charge recipes to their shell recipes — across hangars, cargo, fits
  (and their module runtimes), wreck loot, convoy shipments, market orders and
  last trades, industry jobs, agent ledgers and parked shipments. The save
  schema stays 2.
- **Gun fire renders as projectiles, not lasers** (`render/overlay.go`): the
  persistent muzzle-to-target line is gone for every weapon family. A shot is
  now a per-shot event — a muzzle flash, bolts travelling to the locked
  target and an impact glow on arrival: an autocannon rattles a three-bolt
  burst of thin bolts, an artillery piece throws one heavy round on a slower
  arc; both are silent for the rest of the cycle. Shot timing is read from
  the module's existing cycle clock (`CycleEnds`), so the sim, its RNG and
  the save schema (2) are untouched. Range stays readable in the target
  bracket's distance label. Mining beams and lock lines are unchanged.
- **Agent roster leaning shifts**: Jernborg joining the industry stations
  re-leans the 24 economy agents to 8 industrialists, 8 traders and 8 miners
  (was 6/9/9) — the same four per station, no ids or homes moved.

### Fixed

- **Splitter seams keep their resize cursor on draw-only frames**
  (`uikit/widgets/cursor.go`, `game/game.go`): ebiten runs `Draw` once per
  display frame but `Update` only ~60 times a second, so on a fast display
  roughly half the draws have no input pass. The cursor claim was cleared by
  `ApplyCursor` on every draw, and seam claims are made in the Update phase,
  so those draws fell back to the arrow and the resize shape alternated with
  it — still flickering after the single-writer fix. The claim now belongs to
  the input pass: `widgets.BeginCursorFrame` (first line of `Game.Update`)
  starts the pass and `ApplyCursor` repeats its decision on every draw until
  the next one, so the OS write is still exactly one per draw and no longer
  needs an Update tick to hold the shape.
- **The pane splitter follows the cursor and its resize cursor no longer
  flickers** (`uikit/widgets/splitpane.go`, `uikit/screens/kit.go`): a
  vertical seam moved opposite to the drag — pulling up pushed it down —
  and the splitter wrote its cursor from the Update phase while ebitenui's
  AfterDraw rewrote the arrow at the end of the frame, so the two
  alternated on every frame. The seam now trades extent with the cursor
  (down grows the pane above it, right grows the pane before it). The kit
  is the cursor's single writer: ebitenui's own cursor management is off,
  pane seams and the window edge/corner grips only declare a shape
  (`widgets.ClaimCursor`, grips outranking seams), and one post-Draw
  `widgets.ApplyCursor` makes the frame's only OS write — so the arrow no
  longer alternates with the resize shape over seams or window edges.
  Stored seam ratios need no migration: they are still the fraction above
  (or left of) the seam.
- **Fast scrolling no longer oscillates** (`uikit/widgets/scrollrail.go`):
  the house rail mirrored the viewport every frame, and ebitenui treated
  that mirror as a user change, firing a deferred Changed that wrote the
  previous frame's offset back — two wheel notches on consecutive frames
  locked mirror and write-back one frame apart and the Market, My orders,
  Inventory, Fitting, Manufacturing, Info and nav lists slid up and down a
  wheel step every frame. A rail-originated write now skips exactly one
  mirror; drags and wheel-over-the-gutter keep scrolling the list, and the
  rail keeps up from the frame after.

### Added

- **Market as a per-item tree** (`uikit/screens/market.go`): the order book
  now lists one parent row per item of the whole tradeable catalog — every
  item, module and hull, always (canonical order) — carrying the station's
  price ladder, the quantity resting on both sides and a `—` side cell.
  Clicking an item folds its resting orders out beneath it in the book's
  own [price, qty, Ask/Bid] rows (asks ascending, then bids descending;
  player orders in the owned green) and re-clicking folds them away; the
  fold state is session-only and never persisted. An item with no orders
  anywhere is dimmed but stays fully live: Request on Market opens on it,
  so a dead market can be woken. The My orders column is unchanged.
- **Raw-material liquidity**: every market-service station now seeds both
  sides of every ore and mineral (`content/data/systems/*.json`) — NPC stock
  asks just above base price and standing bids just under it, at the
  existing quantity scale (tens of thousands for cheap minerals, thousands
  for ore). Seeds only, no restock. **Przystań** had no market block at all;
  its freeport now carries the materials set plus the shells its raiders
  need.
- **Jernborg runs manufacturing**: the starter system's station now offers
  the `industry` service, so the factory is reachable from the first dock.
- **Job fee**: 3% of a recipe's mineral value at base prices, per run, paid
  from the wallet when the job starts and recorded in the ledger; the
  Manufacturing preview quotes it. Refusals check the wallet first.
- **Cancel job** (`CmdCancelJob`): cancels a running job at the docked
  station — materials return to the hangar, the fee is kept, the slot
  frees. Finished jobs must be collected.
- **Refinery window** (`uikit/screens/refinery.go`, `pRefinery` Screen row,
  service `refine`, rail section Industry): ore list from the general cargo
  and ore hold, the exact `floor(refine_yield × qty)` yield preview, a
  quantity stepper and Refine over the existing `CmdRefine`; the window
  warns when the minerals will not fit the hold (the sim's EvCargoFull).
  The station services row gains a Refinery button.
- **Two manufacturing contracts** (`content/data/contracts/manufacturing.json`):
  deliver Shield Boosters to Przystań and Stasis Webs to Doki — durable
  module goods, no weapon or ammo ids. Deliver contracts now accept module
  ids as cargo.
- **Shell weapons — autocannon and artillery families** (`content/data/modules/weapons.json`,
  `content/data/items/ammo.json`): the laser turrets and their charges are
  replaced by projectile guns in three calibres — `mod-autocannon-s/m/l`
  (fast cycle, high tracking, short optimal) and `mod-artillery-s/m/l`
  (slow cycle, low tracking, long optimal, a heavier volley from the same
  shell) — and twelve `ammo-*` shells: four damage profiles (EMP, fusion,
  armor-piercing, plutonium) per calibre, shared by both families, since
  ammunition is keyed by calibre. All six guns run capacitor-free; the
  damage-application math (optimal + falloff + tracking + signature) is
  unchanged. NPC fits moved to calibres matching their hulls (Scuttler and
  Custos small, Marauder medium, Hierarch large), the market books seed the
  shell set, and the retired ids are rejected by validation.

## [0.5.0] - 2026-10-19

### Added

- **Plain table — decorless column lists** (`uikit/widgets/plain.go`,
  `uikit/screens/finances.go`): a reusable decorless table — aligned
  columns, no header, no row tints or borders — for readouts that want a
  column list without the sortable table's chrome. The Finances window's
  holdings block (cash, capital deployed in open orders, receivables and
  their total) is now a two-column plain table: labels left, `FormatCredits`
  values right-aligned in the mono tabular face, the Total row in the
  accent.

- **Dev tooling — justfile + GoReleaser** (`justfile`, `.goreleaser.yaml`):
  `just` runs the game and everything around it (`test`, `validate`,
  `simulate`, `describe`, `check`, `build`, …), and `just release <version>`
  cuts a GitHub release end to end: gate → CHANGELOG fold → tag → push →
  cgo-free cross-builds for linux/windows/darwin on amd64 + arm64 published
  to github.com/maxwelljens/entieve with the CHANGELOG section as the
  release notes.

- **Finances window — holdings and the transaction ledger**
  (`uikit/screens/finances.go`, the `pFinances` Screen row in
  `game/screens.go`, key `K`, rail section Trade): a read-only wallet
  window opening in space and docked alike. The header shows the player's
  money in the three places it can sit — spendable **cash**, **capital
  deployed** in open buy orders (escrowed at placement, refundable in full
  on cancel; the 3% broker fee charged at placement is not), **receivables**
  (the credits accepted contracts still owe) and their total. Below it the
  transaction ledger: one row per wallet movement (`sim/ledger.go`,
  recorded at every debit/credit site — market buys/sells, order placement
  and escrow release, bounties, repairs and contract payouts), with the net
  amount, the broker fee and sales tax itemized inside the trade's own row,
  and columns when/what/where/amount (euro form, `-1,234.50 €`). The ring
  keeps the newest 200 entries in `Save.Ledger` (newest first in the
  window; empty on older saves).

- **Player standing market orders** (`sim/model.go`, `sim/command.go`,
  `sim/system_economy.go`, `uikit/widgets/intents.go`,
  `uikit/screens/market.go`): the market window now posts, lists and
  cancels *both* sides of the auction — a Bid (buy) or Ask (sell) limit
  order rests on the docked station's book until filled or cancelled,
  partial fills accumulating on the one order (no expiry). New commands
  `CmdPlaceOrder{Item, Side, Price, Qty}` (the station is the docked one,
  exactly like `CmdBuy`/`CmdSell` — station-local reach) and
  `CmdCancelOrder{ID}` dispatch to the engine's existing
  `PlaceOrder`/`CancelOrder` with `owner=OwnerPlayer`: strict SevWarning
  refusals, a 20-slot open-order cap (universe-wide) and **no price
  clamp** — any positive price is legal, the non-refundable 3% broker fee
  is the anti-spam. Fees keep the engine's rule (broker fee at placement,
  kept on cancel; 2.5% sales tax on executed sells). Resting orders —
  player-owned ones alike — already rode the v2 market persistence, so no
  save-schema change. The window gains a My orders list (this station's
  player orders, Cancel per row) and a Place order panel that the typed
  mini-window entry below supersedes in the same cycle. CONTEXT.md gains
  Bid / Ask / Open order.

- **Typed market-order mini-window launched from item context menus**
  (`uikit/screens/orderwin.go`, `uikit/screens/kit.go`,
  `uikit/widgets/intents.go`, `uikit/screens/{inventory,market}.go`,
  `game/game.go`): inventory stacks (cargo, ore hold and station hangar)
  and market book rows now offer **Sell on Market** / **Request on Market**
  in their right-click menus (grayed, never hidden, while undocked). Either
  opens one small draggable **&lt;name&gt; — Order** window, Show-Info-style — a
  second launch re-targets the single instance and raises it. Price and
  quantity are typed `TextInput` fields (no steppers): plain decimals, a
  live `theme.FormatCredits` preview, Accept armed only while the price is
  finite and > 0 and the quantity an integer ≥ 1. The mode toggle
  Sell ↔ Request re-applies its default price (Sell = best bid + 1 €,
  Request = best ask − 1 €, an empty side = the item's base price) and its
  default quantity (the clicked stack / 1). Accept emits the existing
  `PlaceOrder` intent; Cancel and the chrome × close without effect. The
  engine keeps every authority (escrow, 3% broker fee, oversell, the
  20-order cap) and answers with its own SevWarning lines. The window is
  transient: its rect is not written to the layout sidecar, it opens
  centred, and it closes on undock/jump. While a field holds focus the
  game's world hotkeys yield (typing "s" never stops the ship; Esc clears
  the focus first).

- **Ten named manufacturing corporations, with original emblems**
  (`content/corps.go`, `content/data/**`, `content/validate.go`,
  `uikit/emblems/**`, `uikit/screens/infowin.go`): the generic culture
  origins on the item display and in the lore are replaced by a named
  manufacturer layer — the roster approved 2026-10-18, registered in
  `docs/lore/NAMING.md`. Every manufactured def (`ShipDef`, `ModuleDef`,
  ammo/charge `ItemDef`s) now carries a `maker` slug (`corp-*`, a
  registered extension of the frozen id list); rat hulls, ores, minerals
  and loot stay unattributed. Show Info's Sub line reads "class · maker"
  ("Frigate · Kombinat Ferrum") and the CONSTRUCTION block shows a Maker
  row beside the maker's full-colour emblem — a 40 px header mark from
  `uikit/emblems`, rasterized from committed SVGs by the build-time tool
  `tools/emblemraster`. The `origin` heritage tag stays, unchanged, for
  the rat hulls (Old Works) and as name-heritage metadata.

- **Dedicated Finances icon** (`uikit/icons/wallet.png`,
  `uikit/icons/manifest.json`, `uikit/icons/CREDITS.md`,
  `uikit/screens/finances.go`, `game/screens.go`): the Finances window
  (rail entry, tooltip and title bar) no longer borrows the Log's book
  glyph — it carries its own wallet silhouette, fetched from
  game-icons.net and credited per file in `CREDITS.md` and
  `manifest.json` (CC BY 3.0, Delapouite). Like every icon it rides the
  shared white-alpha PNG pipeline and tints per palette role.

- **Market QoL — every standing order listed, your book rows in green**
  (`uikit/screens/market.go`, `uikit/theme/theme.go`): the My orders
  column lists *every* order the player has on the docked station's book
  (the 3-row cap and its "+N more" note are gone), scrolling through them
  on the house rail (wheel included) once they outgrow the column — each
  row keeps its own Cancel. The order book paints the player's own resting
  orders with the owned-row green (`theme.OwnedRowTint`: the palette's
  `Good` at `OwnedRowAlpha`), so they stand out among the NPC/agent rows.
  Fixed in the same pass: the My orders rows were built but never mounted,
  so the column drew nothing.

### Changed

- **Docked windows are ordinary windows — the station band composition is
  gone** (`game/screens.go`, `game/station_layout.go`, `game/layout.go`,
  `game/game.go`, `game/contracts.go`): while docked the market, inventory
  and fitting used to be tiled into a composed band (the market stretched
  across the top, inventory/fitting splitting the row below). They now open
  at their own per-type defaults, exactly as in space — the market's
  **546×546 square**, the inventory/fitting rects — on first use, and the
  sidecar persists wherever the player drags them. The station view still
  opens exactly one window, the Station Services view, whose station column
  (left edge, contract-board content height) is unchanged. The band helpers,
  the docked rect override, the trio-reclaim machinery and the `station`
  DockPolicy are deleted; a new window type without a declared position
  still opens centred.

- **Market window's Place order panel removed; posting moves to the item
  menu's order mini-window** (`uikit/screens/market.go`,
  `uikit/screens/orderwin.go`): the Bid/Ask side toggle, price stepper and
  Place button are gone (no stepper survives anywhere in the posting
  flow). The market keeps its immediate Buy/Sell (with the Qty stepper),
  the order book with its scroll rail, and the My orders list with a
  Cancel per row; its side column now belongs to My orders alone. Posting
  is the typed order mini-window, opened from a row's **Sell on Market** /
  **Request on Market** entries (and from inventory stacks, full stack).
  `market_orders_test` and the row-menu wiring pins were re-targeted.

- **Navigation rail buttons restored to full width; window padding and
  table-header air widened** (`uikit/screens/rail.go`,
  `uikit/theme/chrome.go`, `uikit/widgets/table.go`,
  `uikit/screens/{logpan,infowin,selected}.go`,
  `uikit/screens/finances.go`, `game/*`): the rail's scroll gutter no
  longer carves the entry cells — they are 63×63 again and the 24 px
  `ScrollGutter` rides beside them, so `RailInset` grows 63 → 87 and every
  default window's left edge moves +24 (the docked station column narrows
  to keep its right edge at 404; the log's default width follows to 264 so
  it still clears the HUD bar). Every window's interior padding grows
  `ContentsPad` 5 → 8 (the `SpMd` token), the table's header and cell
  horizontal padding joins it (`cellPad` 5 → `SpMd`; vertical density
  unchanged), and the Selected Item window's content floor widens 450 →
  464 so its icon+label buttons still fit their cells whole. Line and row
  packing stays untouched; the pins and the pixel captures were
  re-targeted in one sweep.

- **Scrollbars sit in a reserved gutter beside their content; furniture
  gets air** (`uikit/theme`, `uikit/widgets/scrollrail.go`,
  `uikit/widgets/table.go`, `uikit/screens/{rail,infowin,market,services}.go`):
  every scrollbar now rides a `theme.ScrollGutter` design-px column beside
  its viewport, so the handle can no longer overhang the content it scrolls
  ("the scrollbar is laid over content" — the market's old 8 px rail was
  narrower than the theme's 16 px handle). The gutter is reserved whether
  or not the rail shows, so the viewport never jumps when a long list
  starts overflowing, and the rail hides while the content fits. The
  pattern — the market order book's rail — is now the shared
  `widgets.ScrollRail`, carried by every `widgets.Table` as well as the
  station rail's entry column and the Show Info page. A small spacing
  token set (`SpSm`/`SpMd`/`SpLg` = 4/8/16) gives the furniture air:
  button/title-bar padding, the slider's track padding, the rail's entry
  gaps. Data rows stay dense on purpose (the table's `cellPad`/`entryPadV`
  literals are pinned unchanged). The station rail grows to `RailInset`
  = cells (63) + gutter (24): the gutter rides beside the entry cells.

- **Star systems rescaled to real-solar-system proportions**
  (`content/data/systems/*.json`, `content/data/worlds/*.json`): bodies
  spread across tens of AU instead of the old ~0.1 AU cloud — inner
  planets 0.4–2 AU from the star, asteroid belts (and belt-like anomalies
  like Gniazdo) 2–10 AU, stations 1–10 AU near their planet or
  independent, stargates 20–40 AU on the rim. Ids, names, radii, kinds
  and every other field are untouched: only positions moved. Each body
  keeps its authored direction from the star (gate directions still read
  as the Kresy ring on the starmap), distances come from per-system
  seeded radial slots (system id as the variation seed — scheme in
  `docs/SIM.md` §7), and stations/belts/anomalies hold a 2 AU separation
  floor (gates 5 AU) so landmarks can never bunch again — pinned by
  `content/layout_test.go`. Warp physics is unchanged (hull `WarpSpeed`
  was already AU/s), so transit across a system now takes tens of seconds
  EVE-style, and convoy/miner legs dominate their economy cycles until
  the cadence knobs are balanced live. World starts ride their anchor
  bodies (Jernborg, the Starogród Belt). The space camera's zoom-out
  range now frames the whole system (`game/scene.go`, pinned in
  `game/scene_test.go`); grid-scale UX is untouched. Showcase tour
  timings drift with the new legs (its recalibration follows separately).

- **Distance display switches to AU past 99,999 km** (`uikit/theme/format.go`,
  `render/art.go`, `uikit/screens/selected.go`, `sim/command.go`): the four
  in-sync distance formatters now read meters as "42 m" under 2 km, 1-decimal
  km through 99,999 km, the literal "<0.01 AU" in the gap below 0.01 AU, and
  two-decimal AU beyond (1 AU = 1.496e11 m, house convention) — so 100,000 km
  shows "0.01 AU" instead of "100,000.0 km". The hint/refusal `kmLabel` pair
  keeps its whole-km style inside the km band ("150 km" for `WarpMinRange`).
  AU figures are fixed two decimals ("0.10 AU", "2.01 AU"), matching the
  house figure style and tabular columns. Golden pins updated and extended in
  `uikit/theme/theme_test.go`, `uikit/screens/selected_test.go`,
  `sim/helpers_test.go`, `render/art_test.go`.

### Fixed

- **Restored windows clear the navigation rail** (`game/layout.go`): a
  layout sidecar written before the rail widened (RailInset 63 → 87) can
  park a window's title bar under the rail gutter, where the top chrome
  paints over it and the window is unreachable. The restore path now
  floors every restored window's left edge at the same `railClearX` the
  defaults use (10 + RailInset), moving the rect right while keeping its
  saved size, y and every other part of the arrangement; a reachable
  sidecar restores exactly as saved and re-sanitizing is a no-op. Sizes
  are never re-defaulted (the market keeps its old saved band size) —
  reset layout remains the way back to the defaults.

- **The default window set no longer overlaps itself at startup**
  (`game/game.go`, `uikit/screens/overview.go`, `uikit/screens/market.go`):
  the Overview window now defaults to the viewport's **upper-right corner**
  and the Selected Item window takes the top-left, so the two core windows
  never share a corner; the Overview's default height stops 8 px above the
  module rack at the 1024×700 minimum. The **market window's default rect is
  square** (546×546 at the identity scale) instead of the old 618×240 band —
  the largest side that stays right of the rail gutter, fits the 1024×700
  minimum, clears the log pocket and the HUD bar and still lays the order
  book + My orders column out (a side below 522 overflows the stepper row).
  A new `TestVirginPlacementTable` pins the whole virgin table at 1280×720
  and 1024×700 at UI scale 1.0 and 1.5: every composed default window is on
  screen and clear of the rail gutter, the boot set (overview, selected
  item, log) is pairwise disjoint and each window holds its corner. The
  market default still covers the module rack at the 1024×700 minimum (it
  clears it at 1280×720), and the log still overlaps the HUD bar there —
  both are inherent to the fixed window sizes at that minimum and are
  documented at the pin.
- **Resize grips show the corner drag cursor** (`uikit/screens/winchrome.go`):
  hovering or holding a grip wrote ebitenui's `input.SetCursorShape`, whose
  name cache maps only the EW/NS names, so every corner advertised a plain
  horizontal arrow (its `AfterDraw` fell through to the default arrow for a
  diagonal). The chrome now writes ebiten's own shape — `NESWResize` /
  `NWSEResize` per corner, `EWResize` / `NSResize` on the edges — through a
  test seam, and keeps re-writing it every frame a grip is hovered, armed or
  dragged (`TestResizeGripCursorShapes`).
- **The typed market-order mini-window's price and quantity text now
  renders** (`uikit/screens/winclip.go`): the dialog's `TextInput` fields
  draw through ebitenui's `MaskedRenderBuffer`, which sizes its scratch
  images to the draw destination — and the window frame clip rendered the
  body into `screen.SubImage(bodyRect)`, so the typed glyphs were dropped.
  The clip wrapper is now skipped for any body whose subtree carries a
  `MaskedRenderBuffer`-backed widget (`TextInput`, and the scroll viewports
  `ScrollContainer`/`TextArea`/`List`); flat, buffer-free bodies keep their
  clip and the 0-px-outside-frame pin. A new gated capture
  (`uikit/screens/orderwin_capture_test.go`) blanks both fields, types into
  them and asserts the glyph pixels appear (blank field ink 0 → 423/119 px
  typed), and the wrap-coverage test pins that a `TextInput` body is left
  unwrapped.
- **The typed market-order mini-window's content no longer overflows its
  frame** (`uikit/screens/orderwin.go`): the dialog's fixed design height was
  guessed at 190 px while title bar + body need 263 (Sell↔Request toggle,
  price field, live preview, qty field, Accept/Cancel buttons + the
  `ContentsPad` margins), so the qty field and the button row hung past the
  bottom edge. The floor is now the measured requirement rounded up by one
  `ContentsPad` of cushion (300×272), the fallback rect matches it so the
  host's centring sees the real height, and every child rect is pinned
  inside the frame at UI scale 1.0 and 1.5 (`orderwin_test.go`
  `TestOrderWindowFitsContent`; the game-level
  `TestOrderWindowOpensCentered` now also asserts the frame holds the
  container's preferred size, so future content growth fails loudly instead
  of clipping).
- **My orders rows now render** (`uikit/screens/market.go`): the standing-
  order column built its rich rows but never mounted them, so the list drew
  nothing (the old pin passed vacuously — `image.Rectangle.In` is true for an
  empty rect). Every player order on the docked book is now shown, each with
  its Cancel.
- **Resizable windows can no longer be shrunk below their content, and
  their bodies cannot paint outside the frame** (`uikit/screens/winchrome.go`,
  `uikit/screens/winclip.go` new, `uikit/screens/{help,overview}.go`,
  `game/contracts.go`): a manual grip drag used to shrink the contracts
  (station) and help windows — every resizable window — until the layout
  overflowed, spilling rows and text outside the frame. `WindowOpts.MinSize`
  was an honest floor all along (`Window.SetLocation` and `resizeRect` both
  clamp to it), but the per-window literals (e.g. help 400×200, the board's
  200) sat far below the bodies they framed. The grip now consults the
  body's content minimum at resize time: `windowMin(win, pressRect)` raises
  the floor per axis to `Container.PreferredSize()` when that axis already
  fits at the press rect, so a drag stops at the frame that still holds the
  content and never grows a window whose body is elastic (a table's fitted
  columns, the Selected Item's compressing layouter) or scrolls. The
  contracts board keeps `MinSize` at its measured body height, rebuilt with
  the board; help sets it to the hugged size it already measures. On top,
  every mounted window body without a scroll viewport is wrapped in a
  frame-clip container (`winclip.go`) that renders into
  `screen.SubImage(bodyRect)`: ebiten clips a sub-image destination at zero
  cost (no offscreen composite) while keeping absolute coordinates, so
  layout, hit-testing and input layers are untouched. Pinned by
  `uikit/screens/winclip_test.go`
  (`TestHelpGripStopsAtContentMin` at 1×/1.5×, `TestClipWindowBodyWrapsFlatBodies`,
  the gated `TestWindowClipCapture`) and
  `game/contracts_test.go` `TestStationWindowGripStopsAtContentMin`; the
  window-body walkers in `game/{draw,actions}_test.go` now recurse through
  `widget.Containerer` so they cross the clip wrapper.

- **The world body pass no longer draws off-screen celestials**
  (`render/render.go`, `render/overlay.go`): a body whose disc cannot reach
  the viewport is skipped instead of drawn. ebiten's vector path flattener
  (`Arc → cubicTo → AddStroke`) degenerates when its input coordinates run
  into the 1e7..1e13 px band — exactly where an AU-scale system's world
  units land once the camera is tight — so an off-screen stroked circle cost
  ~5-20 ms against ~15 µs on-screen, every frame, for geometry no pixel sees
  (pprof put `StrokeCircle`/`AddStroke`/`Path.Arc` at 54% of an idle frame).
  The overlay's brackets, lock lines, selection highlight and ship effects
  gate the same way, sharing `render.offscreen`. Measured headless: the world
  draw drops from ~30 ms to ~0.1-0.4 ms per frame at a tight camera, and from
  ~180 ms to ~0.3 ms after a system jump; the whole-system framing still
  draws every on-screen body. Pinned by
  `game/draw_cull_test.go` `TestWorldDrawCullsOffscreenBodies` (a
  deterministic body-primitive counter, no wall clock).

## [0.4.0] - 2026-10-18

### Added

- **Title-bar close buttons, window open/close fades, translucent window
  bodies** (`uikit/screens/winanim.go`, `uikit/screens/winchrome.go`,
  `uikit/theme/chrome.go`, registration in `game/game.go`): every window
  with a title bar carries a × at its right edge, wired through that
  window's EXISTING close semantics — a Screen row flips its own toggle
  (modal rows pop their scene), the two core windows close through the
  rail's toggle, Show Info keeps its persisted rect and its
  never-auto-open rule, and the title-less Station Services view stays
  as it was. The press is excluded from the chrome-owned title drag (the
  resize grip keeps corner precedence, and a drag that started on the
  strip never counts as a × click) and rides Kit.Update's menu guard, so
  a modal context menu's frame can never close a window. Opens and
  closes animate, render-only: a mount fades + slides in over 12 frames
  with input live from frame 0, a × close commits immediately (the
  window is inert for its 10-frame fade) and defers its unmount to
  settle — the animating window is parked out of ebitenui's window pass
  and composited through one reusable offscreen with alpha/translate (no
  rects move, no layout or simulation state changes, no RNG), and any
  raise, fresh mount or drag-arming press settles a running fade so
  paint order always matches the stack. Window bodies draw through ONE
  translucency token (`theme.WindowAlpha`, ~0.90) — title bars stay
  opaque, HUD/rack/rail keep the palette alpha. Vocabulary pinned in
  `CONTEXT.md` ("close button", "window animation", "settle"); pins in
  `uikit/screens/winclose_test.go`, `uikit/screens/winanim_test.go`,
  `uikit/screens/winchrome_capture_test.go`,
  `uikit/theme/windowbody_test.go`, `game/closebutton_test.go`.

- **Organic pace before every commencement** (`sim/system_traffic.go`,
  `sim/system_agent.go`, `sim/agent.go`): convoys and economy agents no
  longer act the instant it becomes possible — every window is tick-based
  and deterministically jittered from the existing pure hashes (no RNG
  draws, no new save fields: the running deadlines ride the already
  persisted `Traffic.Next`/`Traffic.Phase`), so twin-run hashes and
  save/load stay bit-identical while traffic gains a beat:
  - *departure beat* — a spawned, resumed or arrived hauler holds
    120–480 ticks before its leg warp commences (the warp physics are
    untouched: align, tunnel and landing keep their pinned timeline);
  - *gate cycle* — a cross-system hauler dwells 120–300 ticks at the gate
    before the hop, and the parked journey carries a jump cycle of
    60–180 ticks between hop-out and materializing at the destination
    gate;
  - *turnaround* — a station endpoint runs approach (120–240 ticks) → a
    docked dwell (300–900 ticks) whose cargo/market action fires partway
    through, never at the instant of contact (the hold leaves the hauler
    exactly when the book gains it — the mass balance stays closed) →
    undock + departure beat;
  - *agent cooldown* — each ledger agent's behavior act fires on its own
    hashed 60–600-tick cooldown, and one act performs at most one
    capital-moving step (commission / quote / contract slots rotate on a
    hashed act ordinal), so an agent never moves capital twice inside one
    cooldown.
  Vocabulary pinned in `CONTEXT.md` ("departure beat", "gate cycle",
  "turnaround", "hold", "agent cooldown"); pins and conservation gates in
  `sim/pacing_test.go`.

- **Drop-in starfield background plates** (`assets/backgrounds/`): any star
  system's starfield can be dropped in as PNG layers — `<system-id>.png`
  (layer 0, near/bright), `<system-id>-01.png` (layer 1, far/faint), `-02`
  optional — with a cluster-wide `default.png` / `default-01.png` fallback
  serving every system; each plate replaces the procedural star layer of the
  same number and rides the same camera-parallax wrap and rate; systems with
  neither override nor default draw the untouched procedural starfield,
  byte-identical to before. The resolved star-plate set is the sky: layers
  without a plate draw nothing once art exists — no leftover procedural dots. Every plate
  is pixel-probed once at load: a transparent sky commits it to its starfield
  layer (over the nebula washes), an opaque-black sky draws it as the bottom
  layer under them. Plates are decoded for the current system only and
  evicted on system change (a two-layer system is ~52 MB decoded). Ships
  with `sys-solace.png` / `sys-solace-01.png` for Starogród; convention in
  `docs/ART.md`.

- **The left navigation rail (EVE-neocom).** Root-overlay chrome beside
  the HUD and module rack, pinned to the left screen edge: one icon per
  openable window, **derived from the Screen spec** — every
  non-transient row becomes exactly one entry carrying its `Icon`,
  `Title` display key and the row's new `Group` field
  (`game/rail.go`), rendered in section order **Ship · Trade ·
  Industry · Station · World · Help & System** with a hairline between
  sections. The two core windows join the Ship section (they are
  kit-owned chrome, not spec rows, and toggle through `Kit.ToggleCore`);
  the Esc menu (transient) and the HUD/module rack (permanent ship
  chrome) have no entries — `screenSpecErrors` now also rejects a
  non-transient row without a rail icon or a known group, and a `Service`
  field names the station service a window needs. A click emits
  `Toggle{Title}` on the ActionSink: the first click opens (a fresh
  mount lands on top of the stack), the second closes. Entries glow
  with the rack's active-cell accent while their window is open, carry
  the structured hover tooltip (icon + title + the row's `key — desc`
  hint), and are **grayed — clicks inert — outside their context**:
  `DockedOnly` in space, `SpaceOnly` while docked, and a
  station-dependent entry (`Service`) at a station lacking that service.
  A slim edge tab collapses the rail to its tab; the state persists as
  `rail_collapsed` in `<save>.settings.json` (omitempty — the
  documented default shape stays two keys, absent = expanded). Geometry
  runs through the scale funnel: the expanded width is
  `uikit.RailInset` (36 design px) and is also the left gutter every
  default window keeps clear — the overview, scanner, log and the
  docked station column all start right of it while their right edges
  stay put, so the composition right of the left column is unchanged.
- **The station services view** (`Station Services`, a `DockedOnly`
  `DockStation` Screen row): the one window docking opens. Built from
  `uikit.NewServices` — the docked-at identity (station name · system),
  an **Undock** button, a **Cargo** button and one button per service
  the station actually offers (Market / Fitting / Industry; absent
  services are hidden, never grayed) — over the contract board, in the
  left column of the docked composition. Every button emits a typed
  intent (`Undock`, `Toggle{Screen}` with the spec's own titles); the
  row is exactly as tall as the strip header it replaces, so the
  column's worst-case height and its clearance over the log are
  unchanged.
- **Resolution-independent UI.** Every visible length — window floors and
  title bars, paddings and gaps, grip bands, icon boxes, tooltip cap,
  starmap, HUD ring/cards, the 21 px type scale — now flows through one
  scale funnel (`uikit.Px` / `uikit.Scale`): the display's device factor
  × a persisted user setting. The framebuffer renders at *device*
  resolution (`Layout` returns window size × device scale) so text
  rasterizes at its final size — crisp on HiDPI instead of
  rendered-small-and-upscaled — while hit tests (resize grips, menu rows,
  `overUI`) keep agreeing with the cursor, which ebiten reports in those
  same Layout units.
- **Settings window** (`P`): a UI-scale stepper (0.75× / 1× / 1.25× /
  1.5× / 2× / 2.5×, applied live — the interface rebuilds against the
  new scale and every window keeps its design-unit placement), a
  fullscreen toggle (`ebiten.SetFullscreen`) and **Reset window layout**.
  Persisted next to the save as `<save>.settings.json`
  (`{"ui_scale": 1.5, "fullscreen": false}`), loaded before the first
  layout, defaulting to 1.0 / windowed when absent or damaged.
- The layout sidecar now stores **design units** (÷ scale on write,
  × scale on read): existing v2 files (written at scale 1, where design
  unit == pixel) load seamlessly and one saved arrangement works at
  every scale, including across a live scale change.

- **Show Info** item inspection: right-click any item anywhere it appears
  (rack, fitting, market, inventory, overview, contract rows) for
  a flat EVE-style context menu — bold **Show Info** first, then the
  surface's actions, grayed-but-visible when currently impossible — and
  one reusable floating `<Name> — Info` window (dragable, resizable,
  non-modal; its rect persists in the layout sidecar but it never opens on
  its own) showing each def's description prose and per-kind content: hull
  construction / slots / survival / capacitor / propulsion / sensors /
  cargo; module activation / kind stats / charges / market; item
  properties / market / refines-to / damage; machine caste dossier /
  behavior / loot. Double-click an item to skip the menu and go straight
  to the window.
- `TestMemoryFootprintPump`: headless 6,000-frame memory regression guard
  over the production Update/Draw pipeline — the live heap must plateau
  (under 10 MB growth between the 3,000- and 6,000-frame marks).
- **Manufacturing (industry).** Two-tier recipes (`content/data/recipes/`,
  `rec-*`): minerals in, ships/modules/charges out. Timed jobs at stations
  with the `industry` service (`CmdStartJob`/`CmdCollectJob`, two
  concurrent slots per station, materials committed up front), the
  **factory** window (`B`, dock-only; surfaces automatically at industry
  stations) with its have/need material column, and per-station **craft
  growth** — `L = min(5, 1 + floor(sqrt(runs)/2))` per recipe family, each
  level 4 % faster and 2 % leaner, decaying one run-equivalent per 3,600
  idle ticks.
- **World metabolism.** Belt state is persistent (`Save.Belts`): rocks
  deplete as they are mined and regrow +1 % of their definition volume per
  600-tick window, capped at it, across save/restore and system reloads.
  Wreck reprocessing reclaims `floor(0.4 × hull-recipe mineral inputs)` on
  a destroyed ship's wreck (the value-added burns off). NPC replacement is
  **market-funded**: guards requisition hull, fit and charges from the
  respawn station's book against a per-faction ledger (`Save.FactionLedger`),
  rats pay belt ore or buy local ore asks; an unfunded requisition re-queues
  instead of spawning nothing-for-free.
- **The Kresy ring.** Four new systems — Rudnik (Charter space), Młynów and
  Kamieniec (the Fringe), Głusza (Unincorporated) — with four new nodes:
  Halda (Rudnik's mining outpost), Huta (Młynów's refinery platform), Doki
  (Kamieniec's shipyard & depot) and Kantor (Głusza's freeport). The map is
  seven systems.
- **Ledger agents.** A 24-agent roster (6 industrialists, 9 traders, 9
  miners — world-gated to `world-default`) with capital, inventory, EWMA
  trade-price beliefs and ZIP margins. Valuation backward-imputs recipe
  inputs to their products' fee-net value (Menger/Wieser fixpoint), quotes
  rest at `V·(1±µ)` with Widrow-Hoff margin updates (β = 0.25, α = 0.05),
  and agents act where the margin is positive. Fills settle every tick,
  the behavior pass every 300 ticks, station upkeep (repair/rearm basket +
  maintenance-fee money sink) every 3,600 ticks.
- **Cross-system and multi-hop convoys.** Traffic routes gain `to_system`
  lanes (the validator checks the gate path); a hauler that hops a gate —
  or whose grid is rebuilt under it — parks as a journey
  (`Save.TrafficJourneys`) with its provenance and destination intact and
  flies again when its system loads next.
- **Haul wishes.** Traders and industrialists append cross-node arbitrage
  wishes whenever two stations' prices differ by more than the round-trip
  fee load (`MaxCost` = spread − fees), exposed via `Sim.HaulWishes()`; the
  fulfillment seam (`Sim.FulfillHaul`) pays the hauling cost from the
  owning agent's free capital and drops the wish — a money sink. Wish-driven
  convoys are live: cargo debited from the agent's stock at spawn, delivered
  as agent-owned asks at the destination at the agent's reservation price,
  coexisting with authored interval lanes — one validated shipment per
  60-tick poll, settled through `FulfillHaul` at 2.5 € per m³ per transit
  leg on delivery (lost shipments drop the wish unpaid).

- Binding lore rulebook (`docs/lore/`): UNIVERSE, FACTIONS, AESTHETIC and
  NAMING — the Far Moravia canon (frontier gold rush for the ores,
  first-wave European heritage, the Old Works machine swarms,
  charter-reach zones, the euro as old-world money, four-origin trait
  matrix). NAMING.md holds the rename registry for every content id; the
  names are author-approved and the re-theme entry under Changed applies
  them.
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
- Interface icons from **game-icons.net** (CC BY 3.0 — per-icon author
  credits ship in `uikit/icons/CREDITS.md`, completeness pinned by test):
  overview Kind column, Selected Item header and action buttons, HUD
  target cards, module rack class tiles, service title bars, contract
  rows, scanner contacts and fitting rows — monochrome silhouettes
  tinted from the palette on a 16/20/24 px grid with per-icon optical
  nudges. Consequences: Selected Item minimum width 300→450 px (icon
  buttons), station column 384→394 px (contract titles keep their
  budget).
- Structured hover tooltips: class icon + bold title + dim subtitle +
  sectioned stats with values right-anchored on one column + hint
  footers — shared by the module rack, the Fitting window's module rows
  and the warp-gate hint. Tooltips clamp fully inside the window at every
  screen edge.

- Deterministic simulation core: fixed 60-tick timestep, golden-pinned
  SplitMix64 RNG, minimal deterministic ECS (`core`, `ecs`).
- Data-driven content system: JSON schema for items, ships, modules, NPCs,
  star systems and worlds with full cross-reference validation (`content`).
- EVE-faithful simulation (`sim`): shield/armor/structure with four damage
  types and resists, capacitor recharge curve, turret tracking formula with
  wrecking shots and ammo damage profiles, targeting, warping, mining,
  looting, NPC behavior, station services (repair, market, refine, fitting,
  cargo transfer between holds and the station hangar), stargate travel,
  respawns, escape-pod death respawn with insurer replacement (home
  station across systems), exact
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

### Changed

- **Docking opens exactly one window.** The station scene no longer
  force-opens the market/inventory/fitting trio (nor the factory at
  industry stations): `StationScene.OnEnter` opens only the new station
  services view and `OnLeave` closes only it, leaving every
  player-opened window exactly as it was. The trio became ordinary
  windows — the docked composition (`stationLayout` + the reclaim) still
  places them, but only when the player opens them (rail, keys or the
  services view's buttons). The `Screen.ForcedDockOpen` field is
  **deleted**: composed membership now derives from
  `DockPolicy == DockStation` alone, and with nothing force-open on
  dock the `stationPrior` pre-dock-intent machinery went with it —
  `persistOpen`/`applyLayout` record and restore the live state the
  player last saw (the services view's flag is the scene's). The old
  game-built station strip (`buildStationWindow`, `syncStationLabel`,
  `setStrip`/`stShown`) is deleted: its identity line and Undock button
  moved into the services view, its contract board rides the services
  window, and the window mounts through the ordinary panel path.
- **The contract board folds onto the table module** (`widgets.StackRow`,
  `widgets.StackLayout`): the table gains a modest rich-row capability —
  a `TableRow` may carry a **line stack** (`Stack []RowLine`) instead of
  one text per column: each line a run of cells (`RowSeg`, own type-scale
  step, colour and minimum width), the row's `Icons[0]` boxing with line
  1's first cell, and its `RowSpec.Actions` drawn as trailing buttons
  (1–2, `Disabled` grayed and silent — one declaration feeding the
  button, the context menu and the gate alike), with a per-line `Ref`
  that joins the shared installer (right-click menu with the bold-first
  Show Info, strict double-click). Everything else — change-gated
  rebuilds (a rich row's stack and its buttons' labels/gates are
  snapshotted like plain cells, so an identical `SetRows` stays a no-op),
  column fitting, sorting, selection and `TipData` tooltips — applies to
  rich rows as to plain ones. The station contract board now declares
  `TableRow` values and renders them through `widgets.NewStackRow`: the
  bespoke `boardLayout` layouter and the hand-built row widget tree it
  laid out are deleted — the equal-height stack behind them moves to
  `widgets.StackLayout` (the pure `boardRowRects` pin stays as the
  geometry tests' subject), and what survives beside it is only the
  contract-id/button/root handle the board's tests press. The look is
  preserved rect for rect — same window chrome, title line, row order,
  strings (the Accepted marker button and the menu's Accept/Abandon verb
  split included), button gating, objective-label Show Info and
  Accept/Abandon dispatch — pinned by the new `TestBoardOnTableRowRects`
  against the pre-port geometry, with the board's golden strings and
  mouse-flow tests passing byte-untouched.
- **`uikit` split into three modules with an enforced import direction**
  (`uikit/theme`, `uikit/widgets`, `uikit/screens`): the flat package
  became `theme` (visual constants and their plumbing — `Palette`,
  `TypeScale`, the `Px`/`SetScale`/`Scale` funnel, faces and the `Theme`
  seam with its implementation, chrome constants, draw helpers, icon
  maps, display formatters) ← `widgets` (reusable mechanics — table,
  menu, tooltip, rowspec, the `Intent` vocabulary and `commandFor`, the
  `ContentView`/`ActionSink`/`Seams` seam interfaces, the `InfoRef` row
  data and its resolvers) ← `screens` (every panel plus the `Kit` shell,
  the session/`NewEnv` wiring and the window-resize chrome). Each layer
  imports only downward (`screens → widgets → theme`, plus `uikit/icons`
  and `render` as before); `widgets` can never import `screens`, and the
  compiler's acyclicity plus a new parsing test
  (`uikit/widgets.TestImportDirection`) pin the rule. Names that now
  span packages are exported at their new home (e.g. `uikit.Px` →
  `theme.Px`, `theme.ContentsPad`, `widgets.RowRef`, `widgets.OpenRowMenu`)
  and call sites updated; a handful of table/tooltip test seams
  (`…ForTest` hooks, exported layout fields) let the screen suites assert
  widget geometry across the seam. Behaviour is unchanged — same menus,
  tooltips, double-clicks and golden pins, and `TestMarketRunsOnStubView`
  now lives with the screens it proves seam-only.
- **The `Env` god-context dissolved into three narrow seams — `Theme`,
  `ContentView`, `ActionSink`** (`uikit/seams.go`, `uikit/intents.go`).
  Every screen constructor now takes exactly the subset it uses: a
  tooltip-only widget a `Theme`, the HUD/log/scanner/map the theme plus
  the view, and the market, fitting, factory, inventory, rack, table and
  Show Info window the union `Seams`. `ContentView` is the read-only
  surface — the defs catalog plus the observable reads screens made off
  `Env.Sim`/`Env.Content` — so a screen runs against a stub view and a
  recording sink with no simulation anywhere near it
  (`TestMarketRunsOnStubView`); `ActionSink` is `Do(Intent)`: screens
  say *what* (`Buy{Item,Qty}`, `StartJob{Recipe,Runs}`, `WarpTo{Entity}`,
  …; the RowSpec payload retyped from `Do func()` to `Intent`) and the
  game executes it (`Game.Do` → `Kit.RunIntent`, one shared
  intent→command table in `commandFor`), the seam later AI-emitted
  intents will ride. `Env` survives only as the deprecated alias for the
  union of the three seams; `NewEnv`/`Kit.Env` return a `Session` (the
  seams plus the shared context menu and Show Info window hosts and
  tests drive). Behaviour is unchanged — same menus, tooltips,
  double-click, keymap, mounts and sidecars, and each intent dispatches
  to exactly the command its button used to queue.
- **One Screen spec per window** (`game/screens.go`): every window the
  player can open — inventory, fitting, market, factory, scanner,
  starmap, help, log, settings and the Esc menu — is now one
  declarative `Screen` row (id, title, icon, key, design-unit default
  rect + anchor, dock policy, modal, mount rule, sidecar name, sync,
  build) from which the window registries derive: the `panelDef` table
  `syncMounts`/`syncPanels` walk, `KeyMap`'s toggle rows and the help
  overlay text rendered from them, the layout sidecar's name list,
  `placeDefaults`, the docked station trio (composition + the scene's
  force-open set) and the tests' panel label map. Adding a window is one
  row plus its builder — it used to touch eight wiring sites. The table
  is validated when a session starts (`screenSpecErrors`: duplicate
  id/key/action/name, zero title, an unknown icon, help text without a
  key, a transient screen carrying a sidecar name, a station row outside
  the composition, `SpaceOnly`+`DockedOnly` together, …), and the tests
  drive the same rule with deliberately broken rows. Behaviour is
  unchanged except the help overlay's row order: the screen toggles now
  form one block after the world bindings (same rows, same text). Also
  removed the dead `Game.objClick` double-click latch (its only user
  moved to `uikit.RowDouble` in the RowSpec pass above).
- **New typefaces: IBM Plex Sans (display) and IBM Plex Mono (tabular).**
  Turret Road and Source Code Pro are replaced wholesale in
  `render/fonts/` — IBM Plex Sans ships Light/Regular/Medium/Bold, IBM
  Plex Mono a single static Regular — role for role: `render.Face`
  (body), `render.FaceBold` (emphasis), `render.MonoFace` (inventory and
  market table rows). Plex sets ~19 % taller at the same size, so
  `uikit.DefaultTypeScale`'s base moves 21 → 20 px (Small 16.8 → 16,
  Large 27.3 → 26, Tiny 14.7 → 14). Glyph coverage (ASCII, `€`/`¢`,
  `×`/`³`, the Polish/Czech diacritics, the ↑/↓ sort indicators) and
  uniform mono digit advances stay pinned in `render/fonts_test.go`;
  both SIL OFL 1.1 notices ship embedded beside the TTFs.

- **Row interactions declare one seam** (`uikit.RowSpec`): every listed
  row — market, inventory, fitting, rack, overview, factory, the
  contract board and Show Info's chain links — now declares its
  interactions as data (`Ref` for Show Info, `Actions` for its verbs,
  `Tip` for its hover panel), and one installer derives the context menu
  (bold **Show Info** first, `MenuSep`, grayed-but-visible actions), the
  lazy tooltip and strict double-click Show Info from that declaration,
  replacing the five hand-wired surfaces (24 wiring sites). User-visible
  consequences: a second click on a rack cell inside the double-click
  window opens Show Info **without also toggling** the module (the old
  toggle+info dual gesture; a lone click still toggles exactly once),
  right-clicking a row no longer silently moves the window's selection
  (menu actions target the row they were opened on — the market's
  Buy/Sell, fitting's Fit/Unefit and the factory's Start/Collect enqueue
  for the clicked row directly), and a stored ship's fit entry in the
  Fitting hangar now stays grayed instead of arming from a stale module
  selection. Menu shapes, gating rules, tooltip anatomy and the menu's
  world-input latch are unchanged.

- Rack right-click now opens the context menu instead of cycling the
  loaded charge directly; quick charge-cycling is the menu's **Cycle
  charge** entry. World right-click (orbit under cursor) is unchanged.
- Content: NPC caste dossier prose (`NpcDef.Description`) and hull origin
  tags (`ShipDef.Origin`) — both shown by the Show Info window.
- Icon silhouette cache now stores the alpha channel only instead of full
  512×512 white NRGBA bitmaps: 38.0 MiB → 9.5 MiB resident for the same
  pixels (RGB is a constant 255, so the white form is rebuilt per
  size-cache miss).
- Documented the upstream ebitenui v0.7.3 window-remount leak
  (`docs/ARCHITECTURE.md`, "upstream window-remount leak"): ≈5.3 KB leak
  per panel dock/toggle mount cycle — real but slow, deliberately not
  mitigated because hiding-instead-of-unmounting would change input
  gating and draw order.
- **Per-station order books replace market memory.** Every station runs
  its own continuous double auction (never-crossed matching, price-time
  priority, 4-significant-figure price ticks); the old per-system
  "market memory" model (price bands, supply steps, halving decay) is
  gone — the book is the only price former. Player Buy/Sell are
  immediate-or-cancel market orders against the docked station's book; a
  3 % broker fee rides buys at placement, a 2.5 % sales tax rides sells at
  execution, and placed orders escrow wallet or goods in full.
  **Save schema v2** (`Save.Schema` = 2) is a clean break: pre-v2 saves
  are rejected instead of restoring into a different market.
- **Convoys carry real cargo.** Route cargo is sourced at spawn, never
  conjured: a station origin withdraws the units from that station's own
  asks (the weighted price paid is the shipment's cost basis), a belt
  origin draws the persistent belt state (a dry belt ships nothing), and a
  gate origin is the economy's one sanctioned import — the named world
  boundary, undebited supply at the destination. Deliveries post as asks
  on the destination's book one tick under `min(cost basis, best ask)`;
  a destroyed hauler's goods are destroyed with it (lootable wreck, no
  reversal of the origin) and the loss surfaces as an event-only
  risk-premium line, never a price step.

- Far Moravia re-theme of content and docs, landing with the parallel
  content/data and code rename workstreams: every display name follows the
  author-approved `docs/lore/NAMING.md` registry (Solace / Penumbra /
  Gloam → Starogród / Kuźnica / Pustkowie, Haven Rest → Jernborg,
  Penumbra Port → Przystań, the Den → Gniazdo, hulls → Väktare / Kosa /
  Havíř / Furman / Tabor / Custos, Loomfall → Kresy, the rat factions
  collapse into the Old Works — ids stay frozen), money displays as
  `1,234.56 €` / `50¢` instead of `12,345.67 cr`, world-facing zone text
  says Charter space / the Fringe / Unincorporated instead of high-sec /
  low-sec / null-sec, and player death reads as escape pod plus insurer
  replacement. `docs/lore/` is now the named binding canon for content
  naming and voice across README, GAMEPLAY, ARCHITECTURE and
  CONTENT_AUTHORING.
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

- **Gate-queue hold chatter** (`sim/system_traffic.go`): the
  "gate queue full, shipment waits in …" line now fires **once per hold
  episode** — on entering the hold — with the shipment's eventual jump as
  its single follow-up. A held shipment now waits in the journey ledger
  (it stands at its gate; nothing respawns it just to re-hold), so retry
  ticks are silent instead of re-logging roughly every resume window.
  New pacing events follow the same discipline: state transitions only,
  never per-tick or per-retry chatter.

- **NPC fleet burst on first system entry** (`sim/system_traffic.go`): the
  cross-system journey ledger no longer dumps every parked shipment into a
  grid at once. Resume is capped and staggered — at most
  `trafficMaxLive − live haulers` respawns per tick, each ledger entry
  firing on its own stateless `trafficDue` schedule in canonical ledger
  order — so a system first loaded with a deep backlog shows ordinary
  traffic density and the backlog drains at the normal cadence behind it.
  A new per-system gate queue (`trafficMaxParkedJourneys` = 3 ×
  trafficMaxLive) refuses to hop-park past its ceiling: overflow shipments
  hold at their current system with cargo intact and retry the hop on
  later ticks, pushing backlog upstream across the ring instead of
  stacking unboundedly at one destination. Concurrent traffic raiders are
  capped per grid (`trafficMaxRaiders` = 6): an ambush whose raider count
  would break the ceiling does not spawn — the count draw is consumed in
  a fixed order first, keeping twin-run and save/restore determinism —
  while static spawn-group NPCs and patrols never count. All three caps
  are mass-neutral: no cargo is ever dropped, duplicated or created, and
  the caps never emit a lost-shipment event.

- **Window focus, default layout and the rail's front-of-stack seat**
  (three window-system defects, one change-set):
  1. *Click anywhere raises.* Any mouse press inside a window — body,
     title bar or resize chrome, left **or** right button — now raises
     that window to the top of the stack. ebitenui's built-in relayering
     only ever saw title-bar presses (the body's scroll viewport claims
     the input layer over the window container), so the kit re-tops
     through its own remove + re-mount seam instead, and kit windows run
     with `DisableRelayering` so that seam is the single, deterministic
     stacking path. When windows overlap, the **topmost** window under
     the cursor wins (that is the window receiving the click); the raise
     is gated exactly like input is — never through the modal context
     menu's frame, never through the rail's own rect, and never up
     through a mounted modal window. A raise re-tops without moving the
     rect, and pays the documented remount leak only when the stacking
     actually changes.
  2. *Defaults clear the rail gutter.* Every default window rect starts
     right of `uikit.RailInset` — the uikit construction defaults that
     still sat at their pre-rail x (overview 8, Selected Item 8, scanner
     10, log 10, help's fallback 10) are retargeted through the same
     constant, and the game's placement floors (`anchorRect`,
     `centerRect`) now clamp every anchored/centered default past the
     gutter: the help window's centred default landed at x=62 against
     the 63 px gutter at the 1024×700 minimum, and at x=93 vs 94.5 at
     device scale 1.5. Pinned at scale 1 and 1.5, both design sizes, for
     the construction rects and the placed set (user-dragged and
     sidecar-restored rects are not defaults and stay untouched).
  3. *The rail is top chrome.* The navigation rail now mounts through
     `Kit.AddTopContainer`: `Kit.Draw` parks it out of the ebitenui
     window pass (`Visibility_Hide_Blocking` — still laid out, never
     painted), paints it after every window with its hover-tooltip
     windows following it (the context menu remains the absolute topmost
     pixels), and re-adds its input layer on top of the window layers
     each frame — so a window dragged onto the gutter never paints over
     the rail, and clicks in the rail's area always reach the rail.
     Paint priority and input priority now agree; while a modal window
     is up the rail keeps its old under-window input gating. The HUD and
     module rack keep their under-window position (`AddContainer`).

- **Window dragging no longer costs a layout pass per frame**
  (uikit/screens/winchrome.go, uikit/widgets/scrollcontent.go): title-bar
  dragging moved from ebitenui's built-in `WindowOpts.Draggable()` to the
  kit's chrome, translate-only. The built-in drag relocated the window
  through `Container.SetLocation` from `Window.Render`, whose
  `RequestRelayout` re-laid out the whole window subtree — grid, rows,
  anchors, preferred-size re-measures and the scroll content's own
  re-location — on every frame of the drag (`doLayout` cost +0.26 s per
  300-frame drag window, `RowLayout.layout` +0.18 s). The chrome now
  rewrites the subtree's rects in place each frame (containers recurse, a
  TextArea re-lays its private interior, a `ScrollContainer` carries the
  content subtree recorded by the new `widgets.AttachScrollContent`
  stash so `renderContent` computes an equal rect and skips its
  `SetLocation` + `RequestRelayout` of the table body), so no layout
  runs: the kit-draw delta between idle and drag fell from 1.38× (heavy,
  325-row market) and 1.31× (light, log) to ≈1.0×, and the drag-vs-idle
  profile diff for every layout function is now zero. Behaviour is
  unchanged: the grip keeps precedence in corners and fires ResizeEvent
  as before, the title drag reports exactly one `Window.MoveEvent` per
  completed press (including a plain click's unchanged rect, matching
  the built-in handler), contents follow the window in the same frame,
  and the services window (no TitleBar) still never drags. Regression
  pins: the title-drag contract (`TestTitleDragBehavior`), a displaced
  title-bar canary proving no layout pass runs during a translate-only
  drag (`TestTitleDragSkipsRelayout`), an alloc bound on drag frames
  over idle (`TestDragFrameCost`), and the existing per-phase sidecar
  signature assertion (a drag still writes no sidecar).

## [0.3.0] - 2026-10-17

- **Navigation-rail icons centred, cells +75 %, station/market
  playtest batch**: ebitenui lays a graphic button out as
  `[empty label][10 px fixed spacing][graphic]` inside the button's
  padded anchor — a fixed right-offset that hung the rail's and the
  station services row's glyphs over their cells' right edges. Both now
  own their glyph: the house `theme.IconBox` anchored dead centre in an
  anchor-layout cell, so the box sits at `(cell − icon)/2` on both axes
  at every scale (`TestRailIconsInsideCells`,
  `TestSvcIconsInsideCells`, `TestStackRowIconInsideRow`). The rail's
  cell square and its gutter grew 36 → 63 px (+75 %, `RailInset` stays
  the windows' left inset) with the glyph grown to 35; twelve 63 px
  cells outrun the 720 design viewport, so the entry column caps to the
  room under its top inset and wheel-scrolls, clipped like the tables,
  when it overflows (`TestRailColumnCapsAndScrolls`,
  `TestRailGutterAndGlyphLiterals`, and the composition pins
  re-derived to the 63 px gutter: station tuple, board lefts, log
  pocket, scanner, overview/settings). The market order book shows a
  visible scrollbar rail beside its scroll viewport — wheel and drag
  both mirror the table's `ScrollTop` — while the title line, quantity
  steppers and Buy/Sell rows stay exactly where they were
  (`TestMarketOrderBookScrolls`).
- **Inventory/table headers aligned with their cells** (Defect A): right
  -aligned cell text anchored 3× the cell padding left of the column
  edge (ebitenui adds a Text's horizontal padding into its measured line
  width *and* subtracts `Padding.Right` again when drawing End-positioned
  text), so headers sat ~10 px right of their column values. Right cells
  now draw with `Padding.Right = 0`, landing every header label and cell
  on the same x at every scale (`TestHeaderCellAnchorAgreement`).
- **Quantity steppers resize their readout** (Defect B): the market /
  inventory / factory `Qty` readout kept the box measured for its
  previous value (changing `widget.Text.Label` does not invalidate
  layout), so wide values drew under the `+` button. The readout now
  relayouts its row on change and owns a box that fits its value; the
  readout and the HUD wallet render in the tabular mono face.

## [0.2.0] - 2026-10-13

- Per-frame interface garbage churn: panels that are not mounted no longer
  run their `Sync` every frame (a panel Syncs when it mounts, so it still
  draws current data in that same frame), `Table.SetRows` skips the column
  re-measure/re-layout when no cell text or icon changed, and the rack /
  fitting row tooltips build their model lazily when the hover key moves
  instead of every frame. Settled Update-path allocation on the showcase
  pump drops from ≈119 KB/frame to ≈14 KB/frame (−88 %).
- Station contract board change detection compares the model fields
  instead of `fmt.Sprintf("%v", …)` on every docked frame.

## [0.1.0] - 2026-10-12

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
- Log content clips and scrolls inside its window (it used to paint
  thousands of pixels past the frame; wheel scrollback was dead). Auto
  tails to the newest line.
- The docked service trio can no longer stack: a saved layout that folds
  windows together while restoring at a smaller size is detected and
  re-tiled. Sidecar format bumped to v2 — existing `.layout.json` files
  reset to defaults once.
- Guard patrols are neutral: they no longer attack the player at close
  range (the ~5 km unprovoked strike); they defend themselves if attacked
  and keep answering convoy ambushes.
- Module tooltips, hover/pressed states, 40 px minimum hit areas,
  right-aligned tabular number columns and soft window shadows.

- The overview no longer reveals every ship, rock and wreck in the system
  from anywhere (EVE-grade sensor tension restored).
- Rats are no longer inert: they patrol, notice intruders and defend their
  belts against miners and shooters.

[Unreleased]: https://github.com/maxwelljens/entieve/compare/v0.9.0...HEAD
[0.9.0]: https://github.com/maxwelljens/entieve/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/maxwelljens/entieve/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/maxwelljens/entieve/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/maxwelljens/entieve/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/maxwelljens/entieve/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/maxwelljens/entieve/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/maxwelljens/entieve/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/maxwelljens/entieve/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/maxwelljens/entieve/releases/tag/v0.1.0
