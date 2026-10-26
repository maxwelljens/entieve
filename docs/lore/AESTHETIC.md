# Look, voice and naming

The register book for Entieve content — binding for prose, names and art
direction. Setting facts live in [UNIVERSE.md](UNIVERSE.md), people in
[FACTIONS.md](FACTIONS.md), the name registry in [NAMING.md](NAMING.md).
Mechanics of art and UI stay in [../ART.md](../ART.md); this file canonizes
its direction, not its machinery.

Target register: **gritty corporate frontier.** Dark, grounded, human; EVE-
like; megacorps, freeports, hazard tape; beauty in machinery and void. It
matches the game's amber-on-black look.

## Prose

- Plain and hard-edged. Short sentences. Concrete nouns: welds, berths,
  holds, cycles, invoices, fine print, shift rosters.
- Dry wit over jokes. Understatement over emphasis. The funny line is the
  true one ("everyone prices in euro because converting on the frontier is
  worse").
- Money, paperwork and downtime are the drama. Bounties pay, premiums rise,
  forms arrive.
- Machines are equipment. Write the Old Works like weather or heavy plant —
  present, hungry, uninterested — never like monsters or villains.
- Beauty where it occurs: the precision of machinery, the scale of the void.
  Show it in a detail; never announce it.
- Never epic, mythic or destiny language. No legends, no prophecies, no
  "ancient", no "unleash", no "chosen", no "glory". See the anti-examples.

### In voice

1. "Jalvrek's dock crew tapes over everything that moves, and half of what
   doesn't."
2. "Everyone prices in euro because converting on the frontier is worse."
3. "Pušťov has no patrols, no courts, and no insurance adjusters."
4. "A Hierarch's crest buys a hauler. It does not buy you somewhere to spend
   it."
5. "Charter patrols answer in three minutes in Struvok. In the Fringe,
   bring friends."

### Not in voice

1. "Ancient and terrible, the Old Works stir from their long slumber to
   claim the stars." — mythic. Machines here cut rock; they do not slumber.
2. "You are the chosen pilot, the last hope of the Wild Fields." — destiny. The
   player is an insured working pilot ([UNIVERSE.md — Daily
   life](UNIVERSE.md#daily-life-and-the-pilots-place)).
3. "A glorious odyssey awaits among the constellations!" — travel-brochure
   epic. The rush is a job.

## The naming layers

Every name belongs to exactly one layer. Tags in [NAMING.md](NAMING.md)'s
registry come from this matrix; the layers follow the settlement history in
[UNIVERSE.md — The settlement waves](UNIVERSE.md#the-settlement-waves).

| Layer | Rule | Examples |
|---|---|---|
| Places: regions, constellations, systems, stations, celestials | **Slavic/Nordic blend, weathered to the second depth** — the live stem is erased, only the family shape survives | the Wild Fields, Struvok, Kvonice, Pušťov, Jalvrek, Hauvnok, Gnazvor |
| Institutions: authorities, corporations | **weathered root, optionally + an English form-word** — Latinate houses keep the classical shape, never the Latin word | Wild Fields Charter Authority; the ten manufacturing houses: Bůral, Noldvar Guard, Iškoven, Kumbřan Steelworks, Dvužek, Bríždan Factory, Cylvar, Preštor, Alďmar, Vantera |
| Ship hulls | **drifted heritage trade nouns** — origin-tagged, **one origin per hull**, live stem erased, the name must read as that origin's trade | Hušnek (Moravian), Ždanek, Furdžan, Tobřek (Polish), Vaštrek (Nordic), Kuštor (Latinate) |
| Machine names | the caste names are canon and untouchable | Scuttler, Marauder, Hierarch |
| Products: modules, charges, minerals, ores, loot | plain trade nouns of the lingua franca, mechanics-aligned; a mineral or ore may be coined from a real root when no trade word survives | Small Autocannon, 40mm ion, Sideron, Sterngrit, Alloy Scrap |

Supporting rules:

- **Display-only.** Renaming a name is free; renaming an id is forbidden.
  Ids (`sys-*`, `cel-*`, `ship-*`, `mod-*`, `ore-*`, …) are frozen
  implementation artifacts, including when they embed retired slugs.
- **No placeholder echoes.** Solace, Penumbra, Gloam, Haven Rest, Thevek
  Reach, Loomfall, Mire Corsairs and their fragments — nor any name the
  time-depth drift retired (Starogród, Rudnik, Kuźnica, Młynów, Kamieniec,
  Pustkowie, Głusza, Jernborg, Przystań, Gniazdo, Halda, Huta, Doki, Kantor,
  Väktare, Kosa, Havíř, Furman, Tabor, Custos, Borealis, Nordvakt, Iskald,
  Družstvo, Celeritas), nor the names the second depth retired (Sterhov,
  Rudnek, Kuźbor, Młyndal, Kamenvik, Pustheim, Głusholm, Jernvold, Havstov,
  Gniazvor, Malmhalda, Hutov, Dokhavn, Kantov, Vaktr, Kosar, Havřek, Furmek,
  Taborek, Custor, Boreall, Nordvakten, Iskeld, Kombinat Ferrum, Družin,
  Zakłady Burza, Celerita, Praetor, Aldemar Group, Far Moravia, Kresy) —
  must not survive as names, slang or secondary names.
- **Real roots, erased.** Prefer words with lexical roots; note the root in
  [NAMING.md](NAMING.md), and weather it past recognition in the name itself
  ([Time-depth drift](NAMING.md#time-depth-drift)).
- **Diacritics are correct, not risky.** The game font renders all Polish
  and Czech diacritics (ą ć ę ł ń ó ś ź ż č ď ě ň ř š ť ů ž, plus the Czech
  long í; upper and lower) plus `€` and `¢`. Keep names sayable by an English
  reader.

## Origins and traits

Four heritage cultures. Traits reach hull stat lines as **soft stats**:
flavour plus a visible stat skew — no new mechanics, no origin stats, no
gameplay classes.

| Origin | Identity | Trait skew |
|---|---|---|
| **Moravian** | the founding commons; communal, resilient | armor, industry, mining/logistics |
| **Polish** | functional workhorses; the yards and the haulers | cargo, ruggedness, cheap to run |
| **Nordic** | sophisticated, precise | electronics, ewar, shield/cap systems elegance |
| **Latinate** | the charter register; institutional | shields, sensors, speed (couriers, executives, patrol cutters) |

The two "shield" rows are different questions: **Nordic** is *how it is
built* — clean, elegant shield and capacitor engineering; **Latinate** is
*what it is for* — fast institutional hulls that stand out on sensors.

Origin tags used in the registry: `Moravian`, `Polish`, `Nordic`,
`Latinate`, `Old Works`, `place`, `trade`. Hulls carry exactly one heritage
tag; the Old Works carry none — their castes predate the tags.

## Money strings

Amounts display `1,234.56 €`; anything under one euro displays `50¢`. In
prose: "euro(s)" and "cent(s)". The full replacement spec for today's
placeholder "credits" / "cr" strings — the mechanical mapping content passes
will apply — is [NAMING.md — Money strings](NAMING.md#money-strings).

## Visual canon

Direction for all future art. [../ART.md](../ART.md) remains authoritative
for mechanics (sizes, fallbacks, directories); this section binds the look.

- **Palette — amber on black, EVE-dark.** Near-black surfaces (`#0a0e14`
  panel fill), amber accent and selection (`#ffb454`), pale text on dim
  greys, functional colours reserved for status: green good, amber warning,
  red danger, blue info. Nothing pastel, nothing white, no colour without a
  job. Per-system nebula palettes in `content/data/systems/*.json` stay
  dark washes behind this chrome.
- **Type — IBM Plex Sans** everywhere (OFL, embedded in `render/fonts/`):
  Regular for body, Bold for emphasis. Diacritics and `€`/`¢` are covered;
  use authentic spellings.
- **Icons — monochrome game-icons silhouettes** (`uikit/icons/`, CC BY 3.0,
  credited in `uikit/icons/CREDITS.md`): single white silhouettes tinted
  from palette roles at draw time. Never multicolour, never photographic.
- **Sprites — drop-in, procedural fallback.** Custom art is a PNG named for
  its content id in `assets/<kind>/`; anything missing falls back to the
  built-in vector art. Hull sprites point right. Art is machinery with
  service lights and panel lines — freighters, cutters, drones, hazard
  markings — not sleek space-fantasy curves. Beauty comes from precision,
  scale and wear.

**Binding rules**

- MUST write in the gritty corporate register: plain, hard-edged, dry.
- MUST place every new name in one layer of the naming matrix and give every
  new hull exactly one origin tag.
- MUST keep trait skew soft: flavour and stat skew only.
- MUST keep the visual canon: amber-on-black, IBM Plex Sans, monochrome
  silhouettes, drop-in sprites.
- MUST keep chrome on the form tokens — radius 14/8/5, title-bar dot rings,
  panel depth, tracked mono chrome labels (see Form language).
- MUST write amounts as `1,234.56 €` / `50¢` in UI and logs, "euro(s)" /
  "cent(s)" in prose.
- NEVER use epic, mythic, destiny or chosen-one language in any content.
- NEVER echo the retired placeholder names (see naming layers) in display
  text.
- NEVER colour outside the palette roles or introduce a second typeface or
  multicolour icons.

## Form language (Cyber Terminal-inspired)

Chrome shapes and structure follow the **Cyber Terminal UI Kit** (kit by
Kerem Bolek, 2026; form vocabulary only — no kit file is shipped, and no
kit colour is used). Everything below is implemented in `uikit/theme`
(`round.go`, `dots.go`, `tracked.go`). The palette canon above is binding
and untouched: every fill, border and ink on these forms is a palette role.

- **Rounded chrome — three radius tokens** (design px, scaled with the UI):
  `14` window pieces, `8` panels and cards, `5` controls. Window title bars
  are rounded on their TOP corners only and window bodies on their BOTTOM
  corners only, so the seam between them stays square and a window reads as
  one frame. Inner scroll viewports keep square masks (a rectangular clip)
  — the one place the kit's rounding stops.
- **Window dots.** Every title bar opens with the kit's signature mark:
  three hollow 9 px rings (1 px border-role stroke, nothing filled), then the
  window icon and the tracked title.
- **Panel depth.** Panel and window-body fills ramp from a hair lighter at
  the top (the body lifted 35 % toward `HeadBg`) to the body itself four
  alpha steps more opaque at the bottom. The step is capped at ΔRGB 6 and
  ±4 alpha around the window translucency token: depth, never a colour
  change.
- **Tracked mono chrome labels.** Window titles, panel heads and
  info/tooltip section heads are set in IBM Plex Mono — the existing mono
  contents face, not a second family — with 0.12 em tracking. Body, row and
  list text stays IBM Plex Sans.
- **Grid motif.** The starmap carries the kit's 28 px lattice in the accent
  role at 3.5 % alpha: structure on the one technical surface, while
  windows keep clean hairlines.
- **Pulse.** The module rack's waiting dot breathes through eight
  pre-rendered phases on a deterministic frame count (the kit's blinking
  cursor read as a status mark).
