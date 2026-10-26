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
├── audio/*.ogg                    the six music tracks (embedded, not walked by the art loader)
└── sfx/*.wav                      the five UI cues (embedded, not walked by the art loader)
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

## UI sounds (`sfx/`)

The five interaction cues are copies of the user's local SFX pack
(`~/Obrazy/Assets/SCI-FI_UI_SFX_PACK/SCI-FI_UI_SFX_PACK/`), renamed to the
seam's cue names:

| File | Pack source | Fires on |
|---|---|---|
| `click.wav` | `Clicks/Click_2.wav` | a mouse press that lands on interface chrome |
| `open.wav` | `FX Sounds/Air_FX_Pitched_Up.wav` | a window mounting |
| `close.wav` | `FX Sounds/Air_FX_Pitched_Down.wav` | a window unmounting |
| `confirm.wav` | `Rings/Ring_Pitched_Up.wav` | a verb that lands (refine/buy/sell/…, save-as, a keybind, a save-manager load/delete, the two resets) |
| `refuse.wav` | `Glitches/Glitch_1.wav` | the keybindings window rejecting a chord |

The pack's originals are 24-bit/48 kHz PCM; ebiten's wav decoder reads only
8- and 16-bit PCM, so each copy is transcoded to 16-bit/44.1 kHz stereo
(the audio context's rate) — a byte-identical copy would not decode. They
are embedded by `sfx.go` beside them (`//go:embed *.wav`) and played by
`game.UISound`; `game.NewAudio` builds the players only on the `run` path,
so tests never decode a cue. The pack is **uncredited**: its licence is
unknown, so neither the credits screen nor `uikit/icons/CREDITS.md` claims
it.

Full convention, sizing and fallback rules: [../docs/ART.md](../docs/ART.md).
