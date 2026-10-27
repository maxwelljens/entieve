# assets/

Drop-in art. PNGs placed here replace the game's procedural vector art for
the matching content ID — no config needed. Anything missing falls back to
the built-in art. `artwork/` holds the full-window plates (JPEG allowed).

```
assets/
├── hulls/<hull-id>.png            e.g. ship-sloop.png        (rotated to heading)
├── stations/<celestial-id>.png    e.g. cel-haven-rest.png    (unrotated)
├── asteroids/<ore-id>.png         e.g. ore-cordite.png       (unrotated)
├── icons/<content-id>.png         e.g. mod-autocannon-s.png  (UI icon, square box)
├── backgrounds/<system-id>.png    e.g. default.png          (unrotated, tiles; default.png serves all systems)
├── artwork/spaceship_hangar.jpg   the docked-station backdrop (JPEG, cover-fit)
├── artwork/title_background.jpg    the title screen's backdrop (JPEG, cover-fit, embedded via artwork.go)
├── audio/*.ogg                    the six music tracks (embedded, not walked by the art loader)
└── sfx/*.wav                      the five UI cues (embedded, not walked by the art loader)
```

The file name is exactly the content ID used by the game's JSON data.
PNG with alpha, any pixel size (drawn at ≥2× its on-screen size for
crispness); hull sprites point right (+X), the nose direction.

## Drop-in icons (`icons/`)

The player's own icon art, one PNG per content ID, flat in this directory.
The file name is **exactly** the content ID (`assets/README.md`'s one rule),
so a name that is a display string ("kerngrit.png") is embedded and never
drawn; `assets/icons/icons_test.go` fails the build until it parses as a real
ID. The set embeds through `icons.go` (`//go:embed *.png`) and registers
itself in the icon registry (`uikit/icons`) as **art-mode** art keyed by the
content ID — full-colour, drawn as-is, never tinted — so the drop-in wins
every slot that content ID reaches:

| Family | IDs | Slots the art reaches |
|---|---|---|
| modules | `mod-autocannon-s/-m`, `mod-artillery-s/-m/-l`, `mod-shield-boost`, `mod-shield-extender`, `mod-armor-rep`, `mod-armor-plate`, `mod-cap-recharger`, `mod-afterburner`, `mod-mining-laser`, `mod-stasis-web`, `mod-warp-scram` | fitting tiles, module rack, market rows, manufacturing/refining rows, module tooltip, Show Info hero |
| corporations | `corp-aldemar`, `corp-burza`, `corp-borealis`, `corp-celeritas`, `corp-druzstvo`, `corp-iskald`, `corp-ferrum`, `corp-nordvakt`, `corp-praetor`, `corp-vantera` | the Show Info hero's corporation logo (40 px) — the small emblems on the Sub line and the hull decals stay vector |
| minerals | `min-ferrite`, `min-nickel`, `min-cobalt`, `min-silex`, `min-glacite`, `min-argon` | market rows, manufacturing/refining rows, Show Info hero |
| ores | `ore-sterngrit`, `ore-cordite`, `ore-lucidite`, `ore-glacierite` | same |
| loot | `loot-alloy-scrap`, `loot-iced-core`, `loot-navdata`, `loot-hierarch-crest` | same |

Anything without a file falls back exactly as before (a module's class glyph,
an item's category silhouette, the corporation's vector emblem). `_alt/`
holds three alternate renders the owner set aside: **ignored by decision** —
`go:embed` only reaches `*.png` in this directory, so nothing under `_alt/`
is embedded or drawn. A dropped PNG is 720²/512² RGBA and may be any size;
the registry scales it into the square icon box.

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
(the audio context's rate) — a byte-identical copy would not decode. The
two Air FX files also ship ~30 dB under the rest of the pack (they peak at
−44 dBFS against the click's −14), which made them inaudible at any sane
volume: `open.wav` and `close.wav` carry +30 and +27.6 dB of gain so every
cue peaks at about −14 dBFS (`game.TestUISoundCueLevels` pins the floor). They
are embedded by `sfx.go` beside them (`//go:embed *.wav`) and played by
`game.UISound`; `game.NewAudio` builds the players only on the `run` path,
so tests never decode a cue. The pack is **uncredited**: its licence is
unknown, so neither the credits screen nor `uikit/icons/CREDITS.md` claims
it.

Full convention, sizing and fallback rules: [../docs/ART.md](../docs/ART.md).
