# Custom art

Entieve ships with procedural vector art for everything on grid. Drop a PNG
into `assets/` and that sprite is drawn instead — no config, no manifest, no
code. Anything you don't provide (or a file the game can't read) falls back
to the built-in art, so an empty `assets/` directory plays exactly like the
stock game.

## Quick start

```sh
mkdir -p assets/hulls
cp my-sloop.png assets/hulls/ship-sloop.png   # file name = the content ID
go run ./cmd/entieve run                      # the sloop now draws your PNG
```

## Convention

The file name is the content ID; the directory decides what kind of object
it decorates. IDs are the same kebab-case strings the content JSON uses
(registry: [ARCHITECTURE.md](ARCHITECTURE.md#id-registry-authoritative--content-files-must-use-these-ids)).

| Directory | ID source | Example filename | Rotation |
|---|---|---|---|
| `assets/hulls/` | hull ID = ship content ID (`content/data/ships/*.json`) | `ship-sloop.png`, `ship-rat-marauder.png` | rotated to heading |
| `assets/stations/` | celestial ID (`CelestialDef.id` in `content/data/systems/*.json`) — stations, gates, belts, other structures | `cel-haven-rest.png`, `cel-gate-penumbra.png` | unrotated |
| `assets/asteroids/` | ore ID the belt's asteroids carry (`items/ores.json`) | `ore-cordite.png` | unrotated |
| `assets/backgrounds/` | system ID (`sys-*.json`) or `default` | `default.png` | unrotated |
| `assets/artwork/` | fixed names (no content id) | `spaceship_hangar.jpg` | unrotated, cover-fit |

Sub-JSON lookup is exactly `<directory>/<id>.png`; other files and
directories are ignored. `assets/artwork/` is the one kind that also accepts
JPEG (the docked-station plate ships as a `.jpg`); sprites stay PNG-only.

## Docked backdrop

While the player is docked, `assets/artwork/spaceship_hangar.jpg` (2512×1680,
JPEG) fades in over 12 frames as the full-window backdrop *under* every window
and the rail, with the caption **Docked: &lt;station name&gt;** in the lower-centre
band. The plate is scaled to *cover* the window (aspect preserved, centred,
overflow cropped) and decoded once on the first dock (~17 MB for the shipped
file). No `assets/` directory (released binaries don't ship one) or no
`artwork/` plate: the station view keeps its plain black veil, exactly as
before. The caption's format string is registered in
[lore/NAMING.md](lore/NAMING.md#interface-strings).

## Window headers

The **Refinery** and **Manufacturing** windows carry a plate of their own, in
a band directly under the title bar — part of the window background, drawn
behind the content, so the first rows of the list sit on the image:

| Window | File | Band |
|---|---|---|
| Refinery | `assets/artwork/refining_header.jpg` | 120 design px under the title, full body width |
| Manufacturing | `assets/artwork/manufacturing_header.jpg` | same |

The 16:9 plate is scaled to *cover* the band (aspect preserved, centred,
overflow cropped) and composited at 180/255 on the title edge, fading linearly
to fully transparent at the band's bottom edge, so the list further down reads
on the plain panel. It fades in over the house 12 frames when the window
opens, and the band is pinned under the title while the lists scroll over it
(the body is drawn whole above it). No `assets/` directory (released binaries)
or no plate: no band, the plain window body, exactly as before — the plate is
just another `assets/artwork/` file, decoded eagerly with the rest.

Because the band is behind the content, the list's viewport leaves its own
body fill off under the band: the plate reads at full strength through the
first rows and in the body's padding, and the fill returns — unchanged — at the
band's bottom edge, so the rows further down sit on the plain panel.

## PNG requirements

- **Format**: PNG with an alpha channel, straight (non-premultiplied) alpha —
  what every PNG encoder writes by default. Transparent background for
  sprites; the game composites them exactly like the vector art. The one
  exception is `assets/artwork/`, whose full-window plates may be JPEG (no
  alpha; the plate is opaque).
- **Size**: any pixel size — sprites are scaled at draw time, so **bigger is
  always safe**. Author at ≥2× the largest size the game will draw at for
  crispness: 512×512 works for every kind, 256×256 is the practical floor.
- **Hull sprites point right (+X)**, the nose direction, matching the
  built-in silhouettes; the engine rotates them to the ship's heading.
  Celestials, asteroids and backgrounds are never rotated.
- **Backgrounds are starfield plates, in layers, with a cluster-wide
  default.** Per layer the lookup is `assets/backgrounds/<system-id>.png`
  (layer 0, near, bright), `<system-id>-01.png` (layer 1, far, faint),
  `<system-id>-02.png` (optional layer 2) — falling back to `default.png` /
  `default-01.png` / `default-02.png`, which serve every system. Each plate
  replaces the procedural star layer of the same number: it draws at its
  native size on that layer's camera-parallax rate and wrap, over the nebula
  washes. **The resolved star-plate set is the sky** — layers without a plate
  draw nothing once any star plate resolves, so no stock procedural dots
  bleed through next to author art; only a system with no plates at all keeps
  the untouched procedural starfield.
- **Give plate skies real transparency** (dots opaque, sky transparent) and
  author them to tile edge-to-edge: each plate is pixel-probed once at load
  and a transparent sky commits it to its starfield layer, while an
  opaque-black sky draws it as the *bottom* layer instead — scaled to *cover*
  the screen (aspect preserved, cropped at the edges), above the flat
  background fill and below the nebula washes. Shared defaults stay decoded
  across system switches (~52 MB for the two shipped default layers);
  per-system plates decode on demand and drop with their system, so never
  rely on another system's override still being in memory.

## Sizing (what "fits" means)

A sprite is scaled to fit inside the same square footprint the procedural
art uses for that object — side `2 × logicalRadius` in screen pixels — with
the aspect ratio preserved. World spacing, warp landings and overview scale
therefore never change. The logical radii come from the existing art
constants:

| Kind | Logical radius (draw pixels) |
|---|---|
| ship | hull class: 26 m (frigate) / 36 (destroyer) / 42 (barge, industrial) / 28 (rat), × zoom, clamped to 7–160 px |
| station | `radius` or 2500 m, × zoom, clamped to 14–25% of the screen |
| gate | `radius` or 1500 m, × zoom, clamped to 16–30% of the screen |
| belt / structure | `radius` or 40000 m, × zoom, clamped to 20–40% of the screen |
| asteroid | 70 m, × zoom, clamped to 4–60 px |

## Fallback rules

- Missing `assets/` directory (or a missing file for an ID): no error, no
  log — the procedural vector art draws, exactly as before.
- Corrupt / non-PNG file: skipped silently, that ID falls back (the
  `artwork/` kind also reads JPEG; a corrupt one falls back to the veil).
- Search order for the root: `./assets` (working directory) first, then
  `assets/` next to the executable; the first one that exists wins. The
  probe happens once, lazily, on the first draw.
- Everything above is `render`-internal; the additive API
  `render.LoadAssets(dir)` loads an explicit directory instead (used by
  tests), and marks the lazy probe as done.

The full sprite draw path and its tests live in `render/assets.go` and
`render/assets_test.go`.

## Interface icons

The chrome carries a monochrome glyph set from
[game-icons.net](https://game-icons.net) — kind markers in the overview and
HUD, action glyphs on the Selected Item buttons, module classes on the rack
tiles, objective kinds on contract rows, and a leading glyph on every service
window title.

- **Where**: `uikit/icons/*.png` (512×512 transparent renders) + the runtime
  registry `uikit/icons/manifest.json`; loaded by `uikit/icons` via
  `//go:embed`.
- **License — CC BY 3.0**: each icon is credited to its author. The full
  table (name, slug, author, source URL, license) is
  [`uikit/icons/CREDITS.md`](../uikit/icons/CREDITS.md); a test pins every PNG
  to a manifest entry *and* a credits line, so no art ships uncredited.
  Required wording: *Icons made by {author}. Available on
  https://game-icons.net*.
- **Rendering**: a PNG becomes a white silhouette at load (RGB → white, alpha
  preserved exactly), drawn into a fixed square box on the type grid —
  16 px inline (table rows, target cards), 20 px buttons, 24 px title bars —
  then tinted from the palette role at draw (ColText / ColDim / ColAccent /
  ColDanger / ColInfo; disabled controls 40 % alpha) and cached per
  name+size+tint. Icons are never multicolour and never stretch.
- **Optical fit**: glyphs that sit off-centre in their frame carry a
  per-icon `nudge` (fractions of the box) in `manifest.json`.
- **Fallback**: an unknown name renders the neutral gear, so a new kind can
  never show a missing-image artifact.

## Hull portraits

The HUD and Show Info carry a top-down portrait of the selected hull beside
its name — the fleet's nine hulls, drawn to be told apart by silhouette at
72 px.

- **Where**: `uikit/hulls/svg/<hull-id>.svg` (sources) → `uikit/hulls/<hull-id>-120.png`
  and `-240.png` (committed rasters) + `manifest.json` (hashes). The package
  embeds all three via `//go:embed`; any hull id in `content/data/ships`
  resolves, and `uikit/hulls.Get` returns the smallest committed raster at or
  above the requested size (else the largest), `nil` for another id.
- **Look**: flat vector, nose to the right (+X, like the world sprites),
  mirrored about the hull's centreline, filling the frame with a slight
  margin. Steel/hull greys (`#8fa3bd` family, matching the world fill) plus
  **one restrained accent light per hull** (engine glow / running lights /
  core), 3–6 flat fills, no gradients. Class silhouettes follow the world
  render: sloop sleek frigate, patrol blunt cutter, cutlass long destroyer,
  barge drill-and-gantry miner, hauler refined box, npc-hauler caged crate
  ship, scuttler spindly worker, marauder clawed heavy, hierarch layered
  overseer (accents: ice, amber, ember, safety orange, pale cyan, rust,
  acid green, ember orange, violet).
- **Pipeline**: build-time only. `tools/hullraster` is a separate module (the
  game never links a rasterizer) that rasterizes the SVG sources through
  `srwiley/rasterx`. It accepts the filled subset — `rect`, `circle`,
  `ellipse`, `polygon`, `path` (`M/L/H/V/C/Q/Z`, absolute and relative) with a
  flat hex fill; no strokes, transforms, gradients or text. Output is
  deterministic (same bytes on a rerun), which the manifest hashes pin.
- **Sizes**: 120 and 240 px (2× headroom for the 0.75–2.5 UI scale ladder).
  Authoring viewBox is `0 0 120 120`, one design unit per pixel of the small
  raster.
- **Regenerate** after editing a source:

  ```sh
  cd tools/hullraster && go run . -svg ../../uikit/hulls/svg -out ../../uikit/hulls
  ```

  (or `go generate ./uikit/hulls`). Same pattern as the corporation emblems
  in `uikit/emblems` + `tools/emblemraster`; the portraits are original
  artwork and carry the manifest's `original artwork` credit.

## Typeface

The interface is set in **IBM Plex Sans** (the display face) with **IBM
Plex Mono** as the mono face for tabular contents; both are embedded from
`render/fonts/` (TTFs + their license files, `//go:embed` in
`render/fonts.go`). The type scale moved one pixel with the swap: the
base is 20 px, because Plex sets ~19 % taller at the size the old face
did (`uikit.DefaultTypeScale`).

| Weight | Role |
|---|---|
| Regular | body text: log lines, labels, tooltip rows and values, text input, table cells (outside the mono tables below) |
| Medium | emphasis: window titles, table header sort buttons, buttons, HUD readouts, tooltip titles (`render.FaceBold` / `Env.FaceBold` — the constant name is historical) |
| Light, Bold | shipped with the family in `render/fonts/`, not used by the UI |

- **License — IBM Plex Sans, SIL Open Font License 1.1**: `Copyright ©
  2017 IBM Corp. with Reserved Font Name "Plex"`. The full
  `IBMPlexSans_LICENCE.txt` travels in the same directory as the TTFs (and
  is embedded with them), so any redistribution of the binary carries the
  license.
- **Glyph coverage**: IBM Plex Sans has no U+25B2/U+25BC triangles, so the
  table sort indicators are the ↑/↓ arrows it does carry
  (`uikit/table.go`, `indicator`). Bold was chosen over Medium by rendering
  both at the 16 px button size: Medium sits too close to Regular to read
  as emphasis.

**IBM Plex Mono** is the second family — the **tabular contents** face.
The inventory window's three hold tables and the market window's order
book draw every row cell in it: item names, quantities, volumes and
prices alike (`render.MonoFace` / `Env.FaceMono`, opted into per table
with the `uikit.ContentsFace` table option). It ships as a single static
Regular TTF, drawn as-is — monospace is the point, digits and listed
entries line the columns up. Everything around the rows stays IBM Plex
Sans: window titles, section labels, the table header sort buttons,
buttons and the stepper readouts.

- **Rule of thumb**: display font (IBM Plex Sans) for chrome and prose —
  titles, labels, buttons, tooltips, logs; mono (IBM Plex Mono) for
  tabular data — numbers and listed entries in tables.
- **License — IBM Plex Mono, SIL Open Font License 1.1**: `Copyright ©
  2017 IBM Corp. with Reserved Font Name "Plex"`. The full
  `IBMPlexMono_LICENCE.txt` travels in `render/fonts/` beside its TTF
  (embedded with it), so any redistribution of the binary carries the
  license.
- **Glyph coverage**: digits, `€`/`¢`, `×`/`³` and the Polish/Czech
  diacritics the listed names carry are probed over the embedded TTF in
  `render/fonts_test.go` (`TestMonoFaceResolvesAndCovers`); the sort
  indicators stay IBM Plex Sans's ↑/↓ — only the row cells change face.

## Table chrome

The table header's own furniture is **drawn, not set**: there is no font
glyph for any of it, so nothing depends on a cmap probe.

- **Resize grips**: a 6 design-px strip at each column's right edge, inside
  the 40 px header — a 1 px column boundary line at rest (palette `Border` at 160)
  that lights accent under the pointer and while dragged, with the split
  row's EW-resize cursor. Each strip is its own blocking input layer over the
  sort buttons, so a press there resizes and never sorts.
- **Column menu**: three 2 px dots, 2 px apart (the split pane's sash-grip
  dot language) centred in a 22 px button at the header's right end, dim at
  rest and lit on hover. Its popup marks the visible columns with a tick —
  two 1 px strokes, accent at rest and grayed for the row that only reports
  (the last visible column). The tick gutter is reserved per menu, so every
  label in a column menu starts on one x.
- **Frozen bases**: the columns are measured once on the first fit and that
  tiling becomes each column's base — a wider value ellipsizes inside its
  column rather than re-fitting the row (docs/GAMEPLAY.md, Overview). A later
  table width scales the bases proportionally and a grip edits them, so what
  the player sees is always the base (CONTEXT.md, Column prefs).

## Tooltip anatomy

Hover tips are structured panels, not text blobs. One content model
(`uikit.TooltipModel` in `uikit/tooltip.go`) and one pure layout function
turn data into fixed rects, and the panel draws exactly those rects — so
what the layout tests pin is what ships. The same module builder feeds the
rack cells, the Fitting window's module rows and the Warp gate hint.

| Piece | Treatment |
|---|---|
| Icon | 20 px (`icons.Button`), the class glyph, manifest `nudge` applied, tinted ColText |
| Title | **Bold** at the base step — the only bold line in a tooltip |
| Sub line | Regular small, ColDim — `High slot 1 · Weapon` (slot · class) |
| Hairline | 2 px ColAccent at low alpha (80), full content width |
| Section label | Regular small, ColHead, stored uppercase (`DAMAGE`, `CHARGE`, `ACTIVATION`) |
| Row | label ColDim left · value right-anchored on one shared x, ColText (or ColGood/ColDanger for a signed modifier) |
| Hint footer | Regular tiny, ColDim — `Right-click: module menu`, `Click: activate` |
| Surface | the existing panel treatment: opaque `BGHard` fill, 1 px Border edge at 220, 8/5 px padding |

Rules: content-sized up to 320 px wide; rows tile exactly; values never
ellipsis (labels give way first); absent data never becomes a row — no
"N/A" placeholders; hover keeps the shared 300 ms delay
(`rackTipDelay`), the cursor-sticky position and a 12 px offset, and the
drawn panel is clamped inside the window at all four edges.

Geometry is pinned by `TestTipLayoutTilesAtBothWidths`,
`TestTipFits320px` and `TestTipPlacementClamp`; the drawn panel by
`TestTipPanelDrawSmoke` and, under `ENTIEVE_CAPTURE=1`,
`TestTooltipCaptureFrame` (→ `/tmp/opencode/tooltip_capture.png`).

## Chrome and form

Chrome shapes follow the **Cyber Terminal UI Kit**'s form vocabulary (kit
by Kerem Bolek; borrowed shapes only — the palette stays the house's amber
on black, and no kit file or colour is used). The canon is
[AESTHETIC.md — Form language](lore/AESTHETIC.md#form-language-cyber-terminal-inspired);
this section is the implementation substrate.

**Radius tokens** (`uikit/theme/round.go`, design px, all through
`theme.Px`):

| Token | Value | Chrome |
|---|---|---|
| `RadiusWindow` | 14 | window pieces: title bar TOP corners, body BOTTOM corners |
| `RadiusPanel` | 8 | in-window panels and cards, HUD boxes, starmap card, context menu* |
| `RadiusControl` | 5 | buttons, text inputs, lists, sliders, checkbox, tooltip, menu |

\* the menu and tooltip use the control radius (5): they are popups, not
panels.

**Per-corner rule.** A window is two pieces: the title bar rounds its top
two corners only, the body rounds its bottom two only; the seam between
them stays square (`theme.TopRadii(RadiusWindow)` /
`theme.BottomRadii(RadiusWindow)`). Panels and controls round all four.
A box smaller than twice its radius is out of the scale — thin chrome
(4 px split seams, meter bars, scroll viewport masks) keeps a square or
1 px corner. The generator pads each side by its own corner cell
(`max(radius, border)`), so a nine-slice never leaks past its own rect.

**Dots anatomy** (`uikit/theme/dots.go`). Three hollow rings, 9 px outside
diameter, 1 px stroke, 7 px apart (41 px block), drawn in the bar's border
role at alpha 200 — nothing filled. They sit `SpMd` from the bar's left
edge; the icon starts after the block plus `SpMd`, the tracked title after
the icon plus `IconGap`. Every `theme.NewTitleBar` window carries them, and
so do the hand-built window bars (Overview, Selected Item) through
`theme.TitleDotsWidget`. Two headers deliberately lead with their own mark
instead: the fitting window's hull-portrait header and the HUD rack's
"Modules" panel head.

**Panel depth** (`theme.PanelTop` / `PanelBottom`). Panel and window-body
fills ramp top→bottom: the body lifted 35 % toward `HeadBg` at the top
(ΔRGB ≤ 6) to the body itself 4 alpha steps more opaque at the bottom
(±4 around `WindowAlpha`, whose value stays the ramp's midpoint). Title
bars are flat. The ramp lives in the nine-slice's centre row (256 source
rows, stretched), so every panel inherits it at any size.

**Tracked chrome labels** (`uikit/theme/tracked.go`). Window titles, panel
heads and info/tooltip section heads draw in IBM Plex Mono (the mono
contents face) with 0.12 em tracking, glyph by glyph: `theme.ChromeText`
for widgets, `theme.DrawTracked` inside hand-drawn panels. Body and row
text stays IBM Plex Sans.

Pinned by `TestRadiusTokens`, `TestWindowPiecesArePerCornerRounded`,
`TestChromeClassMinSizes`, `TestPanelDepthToken`,
`TestDepthRampDrawsTopToBottom`, `TestTitleDots` (raster reads, no game
loop needed) and, under `ENTIEVE_CAPTURE=1`,
`TestWindowFadeAndTranslucencyCapture` (corner cut-away + depth top end),
`TestMapCaptureFrame` (28 px lattice) and `TestHeaderCaptureFrame`.

Which generator a screen asks for, and what each piece derives from, is the
map in [ARCHITECTURE.md — UI chrome architecture](ARCHITECTURE.md#ui-chrome-architecture-map);
this section is the visual canon.
