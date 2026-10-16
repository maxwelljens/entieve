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
| `assets/backgrounds/` | system ID (`sys-*.json`) | `sys-solace.png` | unrotated |

Sub-JSON lookup is exactly `<directory>/<id>.png`; other files and
directories are ignored.

## PNG requirements

- **Format**: PNG with an alpha channel, straight (non-premultiplied) alpha —
  what every PNG encoder writes by default. Transparent background for
  sprites; the game composites them exactly like the vector art.
- **Size**: any pixel size — sprites are scaled at draw time, so **bigger is
  always safe**. Author at ≥2× the largest size the game will draw at for
  crispness: 512×512 works for every kind, 256×256 is the practical floor.
- **Hull sprites point right (+X)**, the nose direction, matching the
  built-in silhouettes; the engine rotates them to the ship's heading.
  Celestials, asteroids and backgrounds are never rotated.
- **Backgrounds** are scaled to *cover* the screen (aspect preserved, cropped
  at the edges), drawn above the flat background colour and below the nebula
  washes and starfield.

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
- Corrupt / non-PNG file: skipped silently, that ID falls back.
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
| Bold | emphasis: window titles, table header sort buttons, buttons, HUD readouts, tooltip titles (`render.FaceBold` / `Env.FaceBold`) |
| Light, Medium | shipped with the family in `render/fonts/`, not used by the UI |

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
