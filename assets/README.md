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
└── artwork/spaceship_hangar.jpg   the docked-station backdrop (JPEG, cover-fit)
```

The file name is exactly the content ID used by the game's JSON data.
PNG with alpha, any pixel size (drawn at ≥2× its on-screen size for
crispness); hull sprites point right (+X), the nose direction.

Full convention, sizing and fallback rules: [../docs/ART.md](../docs/ART.md).
