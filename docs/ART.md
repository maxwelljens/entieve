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

## Typeface

The interface is set in **Turret Road**, embedded from `render/fonts/`
(TTFs + `OFL.txt`, `//go:embed` in `render/fonts.go`). The 21 px type
scale is unchanged — only the typeface and the emphasis face are new.

| Weight | Role |
|---|---|
| Regular | body text: log lines, table cells, labels, tooltips, text input |
| Bold | emphasis: window titles, table header sort buttons, buttons, HUD readouts (`render.FaceBold` / `Env.FaceBold`) |
| Light, Medium, ExtraLight, ExtraBold | shipped with the family in `render/fonts/`, not used by the UI |

- **License — Turret Road, SIL Open Font License 1.1**: `Copyright 2018
  The Turret Road Project Authors (https://github.com/noponies/turret-road)`.
  The full `OFL.txt` travels in the same directory as the TTFs (and is
  embedded with them), so any redistribution of the binary carries the
  license.
- **Glyph coverage**: Turret Road has no U+25B2/U+25BC triangles, so the
  table sort indicators are the ↑/↓ arrows it does carry
  (`uikit/table.go`, `indicator`). Bold was chosen over Medium by rendering
  both at the 16.8 px button size: Medium sits too close to Regular to read
  as emphasis.
