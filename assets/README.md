# assets/

Drop-in art. PNGs placed here replace the game's procedural vector art for
the matching content ID — no config needed. Anything missing falls back to
the built-in art. `artwork/` holds the full-window plates (JPEG allowed).

```
assets/
├── hulls/<hull-id>.png            e.g. ship-sloop.png        (rotated to heading)
├── stations/<celestial-id>.png    e.g. cel-haven-rest.png    (unrotated)
├── asteroids/<ore-id>.png         e.g. ore-cordite.png       (unrotated)
├── backgrounds/<system-id>.png    e.g. default.png          (unrotated, tiles; default.png serves all systems)
├── artwork/spaceship_hangar.jpg   the docked-station backdrop (JPEG, cover-fit)
├── artwork/title_background.jpg    the title screen's backdrop (JPEG, cover-fit, embedded via artwork.go)
└── audio/*.ogg                    the six music tracks (embedded, not walked by the art loader)
```

The file name is exactly the content ID used by the game's JSON data.
PNG with alpha, any pixel size (drawn at ≥2× its on-screen size for
crispness); hull sprites point right (+X), the nose direction.

## Music (`audio/`)

The six space-ambient tracks live here as tracked `.ogg` files
(`astro_reverie`, `celestial_echoes`, `galactic_odyssey`, `lunar_serenity`,
`orbital_echoes`, `stellar_drift`; all 44.1 kHz stereo). They are embedded
into the binary by `audio.go` in this directory (`//go:embed *.ogg` — the
shim has to sit beside the files because `go:embed` cannot reach a parent
directory), then played as one shuffled pool everywhere by
`game.NewMusic`. Dropping more `.ogg` files here adds them to the pool at
build time; the art pipeline (`render.LoadAssets`) walks PNG/JPEG only and
ignores both the tracks and the shim. Audio provenance is still unknown, so
the credits screen does not list the tracks yet.

Full convention, sizing and fallback rules: [../docs/ART.md](../docs/ART.md).
