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

1. "Jernborg's dock crew tapes over everything that moves, and half of what
   doesn't."
2. "Everyone prices in euro because converting on the frontier is worse."
3. "Pustkowie has no patrols, no courts, and no insurance adjusters."
4. "A Hierarch's crest buys a hauler. It does not buy you somewhere to spend
   it."
5. "Charter patrols answer in three minutes in Starogród. In the Fringe,
   bring friends."

### Not in voice

1. "Ancient and terrible, the Old Works stir from their long slumber to
   claim the stars." — mythic. Machines here cut rock; they do not slumber.
2. "You are the chosen pilot, the last hope of Far Moravia." — destiny. The
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
| Places: regions, constellations, systems, stations, celestials | Polish / Old Slavic, **Nordic traces in the oldest towns** | Far Moravia, Kresy, Starogród, Kuźnica, Pustkowie, Jernborg, Przystań, Gniazdo |
| Institutions: authorities, corporations | Latinate register; one heritage form-word or root may carry the origin flavour | Far Moravia Charter Authority; the ten manufacturing houses: Borealis, Nordvakt, Iskald, Kombinat Ferrum, Družstvo, Zakłady Burza, Celeritas, Praetor, Aldemar Group, Vantera |
| Ship hulls | origin-tagged trade nouns — **one origin per hull**, the name must read as that origin's trade | Havíř (Moravian), Kosa, Furman, Tabor (Polish), Väktare (Nordic), Custos (Latinate) |
| Machine names | the caste names are canon and untouchable | Scuttler, Marauder, Hierarch |
| Products: modules, charges, minerals, ores, loot | plain trade nouns of the lingua franca, mechanics-aligned; a mineral or ore may be coined from a real root when no trade word survives | Small Autocannon, 40mm ion, Sideron, Sterngrit, Alloy Scrap |

Supporting rules:

- **Display-only.** Renaming a name is free; renaming an id is forbidden.
  Ids (`sys-*`, `cel-*`, `ship-*`, `mod-*`, `ore-*`, …) are frozen
  implementation artifacts, including when they embed retired slugs.
- **No placeholder echoes.** Solace, Penumbra, Gloam, Haven Rest, Thevek
  Reach, Loomfall, Mire Corsairs and their fragments must not survive as
  names, slang or secondary names.
- **Real roots.** Prefer words with lexical roots; note the root in
  [NAMING.md](NAMING.md).
- **Diacritics are correct, not risky.** The game font renders all Polish
  and Czech diacritics (ą ć ę ł ń ó ś ź ż č ď ě ň ř š ť ů ž, upper and
  lower) plus `€` and `¢`. Keep names sayable by an English reader.

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

**Palette — mint on near-black, the Cyber Terminal register.** The tokens
are the Cyber Terminal UI Kit's (c) 2026 Kerem Bolek; the palette and chrome
language are derived from that kit under its license (derived from, not
shipped: no kit files live in this repo). Page black `#070b0f`, panel
`#0b1116`, title bar `#0a0f14`, raised panel `#0e161d`, hairline **mint
`#71f3c4` at ~16 %** on every border, text `#d9f7ef`, muted `#6e8c87`,
secondary `#86a59f` / `#b8d7cf`. Chrome is a **single accent — mint**: the
accent role is `#71f3c4` and every chrome label, hairline, selection and
glow works from it. Status roles are kit hexes: **good `#80efb1`, warning
`#ffc76b`, danger `#ff647c`**. Two roles the kit lacks are cool extensions
of its hue space — **Info `#74c7d9`** (informational text, the rack's
waiting dot) and **Player `#8fe9ff`** (your ship's identity tint) — and
**Shield stays blue `#6fa8ff`**: the HUD layer's own identity, tuned against
the shield-bar capture thresholds. Nothing pastel beyond these tokens, no
colour without a job; per-system nebula palettes in
`content/data/systems/*.json` stay dark washes behind the chrome. The
interface sits on a soft radial mint top-wash (palette-derived, under all
UI), and the starmap alone carries the kit's 28 px mint grid at ~3.5 %.

- **Type — IBM Plex Sans body, IBM Plex Mono chrome.** Body text, table
  rows, list entries and tooltip titles stay Plex Sans (Regular body, Bold
  for the tooltip title). Chrome labels — window titles, panel heads,
  section/stat heads, tooltip section labels — are **Plex Mono with a
  0.12 em letter-tracking step** (`uikit/theme` tracked labels), drawn in
  the mint accent or the head role. Uppercase only where the string is
  already authored uppercase (tooltip and info sections); never re-case a
  name. Diacritics and `€`/`¢` are covered; use authentic spellings.
- **Icons — monochrome game-icons silhouettes** (`uikit/icons/`, CC BY 3.0,
  credited in `uikit/icons/CREDITS.md`): single white silhouettes tinted
  from palette roles at draw time. Never multicolour, never photographic.
- **Sprites — drop-in, procedural fallback.** Custom art is a PNG named for
  its content id in `assets/<kind>/`; anything missing falls back to the
  built-in vector art. Hull sprites point right. Art is machinery with
  service lights and panel lines — freighters, cutters, drones, hazard
  markings — not sleek space-fantasy curves. Beauty comes from precision,
  scale and wear. In-world hues stay physical (stars, planets, rocks,
  nebulae, bolts, mining green); only the UI-coupled accents (selection
  brackets, the station running lights) speak the mint.

**Binding rules**

- MUST write in the gritty corporate register: plain, hard-edged, dry.
- MUST place every new name in one layer of the naming matrix and give every
  new hull exactly one origin tag.
- MUST keep trait skew soft: flavour and stat skew only.
- MUST keep the visual canon: mint-on-black Cyber Terminal palette, IBM Plex
  Sans body with tracked IBM Plex Mono chrome, monochrome silhouettes,
  drop-in sprites.
- MUST write amounts as `1,234.56 €` / `50¢` in UI and logs, "euro(s)" /
  "cent(s)" in prose.
- NEVER use epic, mythic, destiny or chosen-one language in any content.
- NEVER echo the retired placeholder names (see naming layers) in display
  text.
- NEVER colour outside the palette roles, introduce a second typeface, or
  give body/list text the tracked chrome treatment.
