# Naming registry and rename spec

**Status: APPROVED canon — author-approved on 2026-10-11.** The names below
are binding for all content (they were reviewed as a draft and signed off).
New rows are proposed from the rules in
[AESTHETIC.md](AESTHETIC.md#the-naming-layers) and need author approval
before use.

Every entity in `content/data/**` appears below exactly once. Register new
entities here in the same format before their content lands.

## Quick reference

| Layer | Rule | Origin tags |
|---|---|---|
| Places | Polish / Old Slavic; Nordic traces in the oldest towns | `place`, `Nordic` |
| Institutions / corporations | Latinate register; one heritage form-word allowed | `Latinate` |
| Ship hulls | origin-tagged trade nouns, **one origin per hull** | `Moravian`, `Polish`, `Nordic`, `Latinate` |
| Machine names | caste names are canon: Scuttler, Marauder, Hierarch | `Old Works` |
| Products (modules, charges, minerals, ores, loot) | plain trade nouns of the lingua franca, mechanics-aligned | `trade` |

- Retired names that must not survive in display text: **Solace, Penumbra,
  Gloam, Haven Rest, Thevek Reach, Loomfall, Loomfall Security, Mire
  Corsairs, Gloam Hierarchy** (and their fragments).
- Authentic diacritics are correct, not risky: the game font (Turret Road)
  renders all Polish/Czech diacritics plus `€` and `¢`. Keep names sayable
  for English readers; note the lexical root in `notes`.
- Full rules and the trait matrix: [AESTHETIC.md](AESTHETIC.md). Setting
  facts: [UNIVERSE.md](UNIVERSE.md). Who the names belong to:
  [FACTIONS.md](FACTIONS.md).

## Registry

### Systems

| id | current name | proposed name | origin | notes |
|---|---|---|---|---|
| `sys-solace` | Solace | **Starogród** | place | *gród* "fortified town" + *stary* "old" (Polish/Old Slavic); first-wave heartland, Charter space |
| `sys-penumbra` | Penumbra | **Kuźnica** | place | *kuźnia* "forge", historic term for a smithing settlement (Polish); the Fringe junction |
| `sys-gloam` | Gloam | **Pustkowie** | place | "wasteland, wilderness" (Polish); Unincorporated dead-end |

### Celestials

| id | current name | proposed name | origin | notes |
|---|---|---|---|---|
| `cel-solace-star` | Solace | Starogród | place | star carries its system's name (existing convention) |
| `cel-solace-prime` | Solace Prime | Starogród Prime | place | "<system> Prime" convention |
| `cel-solace-belt` | Solace Belt | Starogród Belt | place | "<system> Belt" convention |
| `cel-haven-rest` | Haven Rest | **Jernborg** | Nordic | Danish/Norwegian *jern* "iron" + *borg* "fort"; oldest town, Nordic trace |
| `cel-gate-penumbra` | Penumbra Gate | Kuźnica Gate | place | gates named for the destination; two instances share this id (`sys-solace`, `sys-gloam`) per [../CONTENT_AUTHORING.md](../CONTENT_AUTHORING.md#id-naming-rules-mandatory) |
| `cel-penumbra-star` | Penumbra | Kuźnica | place | |
| `cel-penumbra-belt` | Penumbra Belt | Kuźnica Belt | place | |
| `cel-penumbra-den` | Penumbra Den | **Gniazdo** | place | "nest" (Polish); the Old Works anomaly in the system |
| `cel-gate-solace` | Solace Gate | Starogród Gate | place | |
| `cel-gate-gloam` | Gloam Gate | Pustkowie Gate | place | |
| `cel-penumbra-port` | Penumbra Port | **Przystań** | place | *przystań* "harbour, landing" (Polish); the freeport |
| `cel-gloam-star` | Gloam | Pustkowie | place | |
| `cel-gloam-belt` | Gloam Belt | Pustkowie Belt | place | |

### Ship hulls

| id | current name | proposed name | origin | notes |
|---|---|---|---|---|
| `ship-sloop` | Sloop | **Väktare** | Nordic | Swedish *väktare* "watchman"; precise electronics-heavy frigate |
| `ship-cutlass` | Cutlass | **Kosa** | Polish | *kosa* "scythe"; close-in yard-built brawler |
| `ship-barge` | Barge | **Havíř** | Moravian | Czech *havíř* "miner"; the mining barge |
| `ship-hauler` | Hauler | **Furman** | Polish | *furman* "carter"; the workhorse hauler |
| `ship-npc-hauler` | Convoy Hauler | **Tabor** | Polish | *tabor* "wagon train"; civilian convoy freighter. Overview role labels documented in [../GAMEPLAY.md](../GAMEPLAY.md) ride along in the content pass |
| `ship-npc-patrol` | Patrol | **Custos** | Latinate | Latin *custos* "guardian"; charter patrol cutter. Same caveat as `ship-npc-hauler` |
| `ship-rat-scuttler` | Scuttler | Scuttler | Old Works | caste name retained (canon); description says "corsair skiff" — machine framing in the content pass |
| `ship-rat-marauder` | Marauder | Marauder | Old Works | caste name retained (canon); description says "corsair gunship" — same pass |
| `ship-rat-hierarch` | Hierarch | Hierarch | Old Works | caste name retained (canon); description implies a human "crew" — same pass |

### Modules

| id | current name | proposed name | origin | notes |
|---|---|---|---|---|
| `mod-emp-turret` | EMP Turret | keep | trade | mechanics-aligned trade name |
| `mod-rail-turret` | Rail Turret | keep | trade | |
| `mod-rocket-pod` | Rocket Pod | keep | trade | |
| `mod-shield-boost` | Shield Booster | keep | trade | |
| `mod-shield-extender` | Shield Extender | keep | trade | |
| `mod-armor-rep` | Armor Repairer | keep | trade | |
| `mod-armor-plate` | Armor Plate | keep | trade | |
| `mod-resist-coat` | Resist Coat | keep | trade | |
| `mod-cap-booster` | Cap Booster | keep | trade | |
| `mod-cap-recharger` | Cap Recharger | keep | trade | |
| `mod-afterburner` | Afterburner | keep | trade | |
| `mod-mining-laser` | Mining Laser | keep | trade | |
| `mod-stasis-web` | Stasis Web | keep | trade | |
| `mod-warp-scram` | Warp Scrambler | keep | trade | |

### Items — charges and ammo

| id | current name | proposed name | origin | notes |
|---|---|---|---|---|
| `charge-emp` | EMP Charge | keep | trade | |
| `charge-fusion` | Fusion Charge | keep | trade | |
| `charge-ap` | Armor-Piercing Charge | keep | trade | |
| `charge-plutonium` | Plutonium Charge | keep | trade | |
| `charge-cap-cell` | Capacitor Booster Cell | keep | trade | |

### Items — minerals

| id | current name | proposed name | origin | notes |
|---|---|---|---|---|
| `min-ferrite` | Ferrite | keep | trade | |
| `min-nickel` | Nickel | keep | trade | |
| `min-cobalt` | Cobalt | keep | trade | |
| `min-silex` | Silex | keep | trade | Latin *silex* "flint, silica" |
| `min-glacite` | Glacite | keep | trade | coined from Latin *glacies* "ice" |
| `min-argon` | Argon | keep | trade | |

### Items — ores

| id | current name | proposed name | origin | notes |
|---|---|---|---|---|
| `ore-sterngrit` | Sterngrit Ore | keep | trade | *stern* "star" (Germanic/Nordic root) + grit; miner's name |
| `ore-cordite` | Cordite Ore | keep | trade | nit for review: shares its name with the historical propellant; acceptable as miner's slang, rename at the author's discretion |
| `ore-lucidite` | Lucidite Ore | keep | trade | Latin *lucidus* "clear, glassy"; description says "low-sec favorite" → "Fringe favourite" in the content pass |
| `ore-glacierite` | Glacierite Ore | keep | trade | |

### Items — loot

| id | current name | proposed name | origin | notes |
|---|---|---|---|---|
| `loot-navdata` | Encrypted Navdata | keep | trade | |
| `loot-alloy-scrap` | Alloy Scrap | keep | trade | |
| `loot-iced-core` | Iced Core | keep | trade | |
| `loot-hierarch-crest` | Hierarch's Crest | keep | Old Works | caste name; description says "Gloam hierarch" → "Pustkowie hierarch" in the content pass |

### NPCs

| id | current name | proposed name | origin | notes |
|---|---|---|---|---|
| `npc-guard-patrol` | Patrol | keep | trade | role label; its `faction` string is renamed below |
| `npc-rat-scuttler` | Scuttler | Scuttler | Old Works | caste name retained (canon) |
| `npc-rat-marauder` | Marauder | Marauder | Old Works | caste name retained (canon) |
| `npc-rat-hierarch` | Hierarch | Hierarch | Old Works | caste name retained (canon) |

### Contracts

| id | current name | proposed name | origin | notes |
|---|---|---|---|---|
| `contract-solace-cleanup-1` | Belt cleanup | keep | trade | description: "Mire Corsairs", "Solace belts", "Haven Rest" → "the Old Works", "Starogród belt", "Jernborg" |
| `contract-solace-supply-run-1` | Supply run to Penumbra Port | **Supply run to Przystań** | place | description: "Haven Rest market", "low security" → "Jernborg market", "the Fringe" |
| `contract-solace-cordite-1` | Cordite order | keep | trade | description: "Haven Rest's refinery", "Solace belt" → "Jernborg's refinery", "Starogród belt" |
| `contract-penumbra-den-hunt-1` | Den hunt | **Gniazdo hunt** | place | description: "the Penumbra Den", "the Port office" → "Gniazdo", "the Przystań office" |
| `contract-penumbra-lucidite-1` | Lucidite contract | keep | trade | description: "the Port", "the Penumbra belt" → "Przystań", "the Kuźnica belt" |

### Worlds

| id | current name | proposed name | origin | notes |
|---|---|---|---|---|
| `world-default` | New Pilot | keep | trade | out-of-universe scenario name; description names "Haven Rest" and "Solace" → content pass |
| `world-showcase` | Showcase | keep | trade | test world; scenario details name retired placeholders → content pass (tests pin ids, not names) |

### Strings without ids

| field | current value | proposed value | origin | notes |
|---|---|---|---|---|
| `universe.json` `region` | Thevek Reach | **Far Moravia** | place | settled canon |
| `universe.json` `constellation` | Loomfall | **Kresy** | place | Polish *kresy* "borderlands" |
| `npcs/guards.json` `faction` | Loomfall Security | **Charter Authority** | Latinate | full form "Far Moravia Charter Authority" ([FACTIONS.md](FACTIONS.md#the-charter-authority)) |
| `npcs/rats.json` `faction` (Scuttler, Marauder) | Mire Corsairs | **Old Works** | Old Works | two placeholder strings collapse into one faction |
| `npcs/rats.json` `faction` (Hierarch) | Gloam Hierarchy | **Old Works** | Old Works | same |

`traffic/*.json` carry no display names (keyed by system id) and need no row.

## Money strings

The euro is the unit of account ([UNIVERSE.md — The
euro](UNIVERSE.md#the-euro)). Today's placeholder text says "credits" / "cr"
and will be replaced in a later content pass; the mapping is mechanical:

| Placeholder today | Replacement |
|---|---|
| `12,345.67 cr` | `12,345.67 €` |
| `40,000.00 cr · +0.10 standing` | `40,000.00 € · +0.10 standing` |
| "credit" / "credits" in prose | "euro" / "euros" |
| "40,000 credits" in prose | "forty thousand euro" |
| any amount under one euro | `50¢` (cents sign: `99¢`, `50¢`, `1¢`) |
| JSON keys `reward.credits`, `base_price`, `price`, `wallet` | unchanged — implementation, like `high`/`low`/`null` |

Format: two decimals for euro amounts, thousands separators, `€` **after**
the amount with exactly one space — `12,345.67 €`; cents display bare with
`¢` suffix and no decimals; the minus sign stays in front of the number
(`-1,234.50 €`). UI and log strings use the symbol forms; prose uses the
words. (Position rule: author amendment 2026-10-11 — the `€` suffix is canon
like the rest of this file.)

## How to name new things

For future (human or AI) authors adding content:

1. **Pick the layer** from the quick reference above. If the thing spans two
   layers (a station is a place *and* an institution), the display name
   follows the place layer and the institutional name lives in prose.
2. **Hulls get exactly one origin tag**, and the name must read as that
   origin's trade noun — a Polish hauler, a Nordic ewar frigate and a
   charter cutter must not sound alike. Caste names (Scuttler, Marauder,
   Hierarch) are reserved for the Old Works.
3. **Use real lexical roots** and note the root in `notes` (language + gloss,
   e.g. `*przystań* "harbour, landing" (Polish)`). Coin words only from real
   morphemes.
4. **Write the name with authentic diacritics** and check it is sayable by
   an English reader. Diacritics render; they are not a risk.
5. **Never mint a name that echoes a retired one** (list above), including
   as a nickname or a joke.
6. **Never rename an id.** Add a row here; leave the slug alone.
7. **Register the entity here** in the exact table format: one row per id,
   `proposed name` as a draft, `origin` from the tag vocabulary.
8. **Money and zones follow the specs**: `1,234.56 €` / `50¢`, and Charter
   space / the Fringe / Unincorporated — never high-sec/low-sec/null-sec in
   display text.
9. **Prose follows** [AESTHETIC.md](AESTHETIC.md): plain, hard-edged, dry;
   no epic, mythic or destiny language.

## Binding rules

- MUST register every entity of `content/data/**` here, one row per id, in
  the exact table format, before its content lands.
- MUST assign every display name to one layer and tag it with one origin from
  the vocabulary above; every hull gets exactly one heritage origin.
- MUST keep names display-only: **ids are frozen** and never renamed, even
  when a slug embeds a retired name.
- MUST note the lexical root of every invented name in `notes`.
- MUST use the `€` / `¢` string conventions here in all UI and log text, and
  Charter space / the Fringe / Unincorporated for zones.
- NEVER leave a retired placeholder name (Solace, Penumbra, Gloam, Haven
  Rest, Thevek Reach, Loomfall, Loomfall Security, Mire Corsairs, Gloam
  Hierarchy) in display text — names, titles, descriptions, UI strings,
  log lines.
- NEVER reserve the caste names Scuttler, Marauder and Hierarch for anything
  but the Old Works.
