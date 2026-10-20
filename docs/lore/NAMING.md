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
- Authentic diacritics are correct, not risky: the game font (IBM Plex Sans)
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
| `sys-rudnik` | — | **Rudnik** | place | Polish *ruda* "ore"; *rudnik* "ore-works"; the Charter-space anchor of the ring; approved 2026-10-13 |
| `sys-mlynow` | — | **Młynów** | place | Polish *młyn* "mill" + *-ów* settlement suffix; the Fringe; approved 2026-10-13 |
| `sys-kamieniec` | — | **Kamieniec** | place | Polish *kamień* "stone" + *-iec* (cf. historic Kamieniec, a Kresy town); the Fringe; approved 2026-10-13 |
| `sys-glusza` | — | **Głusza** | place | Polish *głusza* "remote wilderness, the hush"; Unincorporated; approved 2026-10-13 |

### Celestials

| id | current name | proposed name | origin | notes |
|---|---|---|---|---|
| `cel-solace-star` | Solace | Starogród | place | star carries its system's name (existing convention) |
| `cel-solace-prime` | Solace Prime | Starogród Prime | place | "<system> Prime" convention |
| `cel-solace-belt` | Solace Belt | Starogród Belt | place | "<system> Belt" convention |
| `cel-haven-rest` | Haven Rest | **Jernborg** | Nordic | Danish/Norwegian *jern* "iron" + *borg* "fort"; oldest town, Nordic trace |
| `cel-gate-penumbra` | Penumbra Gate | Kuźnica Gate | place | gates named for the destination; four instances share this id (`sys-solace`, `sys-gloam`, `sys-mlynow`, `sys-kamieniec`) per [../CONTENT_AUTHORING.md](../CONTENT_AUTHORING.md#id-naming-rules-mandatory) |
| `cel-penumbra-star` | Penumbra | Kuźnica | place | |
| `cel-penumbra-belt` | Penumbra Belt | Kuźnica Belt | place | |
| `cel-penumbra-den` | Penumbra Den | **Gniazdo** | place | "nest" (Polish); the Old Works anomaly in the system |
| `cel-gate-solace` | Solace Gate | Starogród Gate | place | |
| `cel-gate-gloam` | Gloam Gate | Pustkowie Gate | place | |
| `cel-penumbra-port` | Penumbra Port | **Przystań** | place | *przystań* "harbour, landing" (Polish); the freeport |
| `cel-gloam-star` | Gloam | Pustkowie | place | |
| `cel-gloam-belt` | Gloam Belt | Pustkowie Belt | place | |
| `cel-rudnik-star` | — | Rudnik | place | star carries its system's name (existing convention); approved 2026-10-13 |
| `cel-rudnik-prime` | — | Rudnik Prime | place | "<system> Prime" convention; approved 2026-10-13 |
| `cel-halda` | — | **Halda** | place | Polish *halda* "slag heap, spoil tip" (mining term); Rudnik's mining outpost; approved 2026-10-13 |
| `cel-rudnik-belt` | — | Rudnik Belt | place | "<system> Belt" convention; approved 2026-10-13 |
| `cel-gate-rudnik` | — | Rudnik Gate | place | gates named for the destination; two instances (`sys-solace`, `sys-mlynow`) per [../CONTENT_AUTHORING.md](../CONTENT_AUTHORING.md#id-naming-rules-mandatory); approved 2026-10-13 |
| `cel-mlynow-star` | — | Młynów | place | star carries its system's name (existing convention); approved 2026-10-13 |
| `cel-mlynow-prime` | — | Młynów Prime | place | "<system> Prime" convention; approved 2026-10-13 |
| `cel-huta` | — | **Huta** | place | Polish *huta* "smelter, ironworks" (← German *Hütte*); Młynów's refinery platform; approved 2026-10-13 |
| `cel-mlynow-belt` | — | Młynów Belt | place | "<system> Belt" convention; approved 2026-10-13 |
| `cel-gate-mlynow` | — | Młynów Gate | place | gates named for the destination; two instances (`sys-rudnik`, `sys-penumbra`) per [../CONTENT_AUTHORING.md](../CONTENT_AUTHORING.md#id-naming-rules-mandatory); approved 2026-10-13 |
| `cel-kamieniec-star` | — | Kamieniec | place | star carries its system's name (existing convention); approved 2026-10-13 |
| `cel-kamieniec-prime` | — | Kamieniec Prime | place | "<system> Prime" convention; approved 2026-10-13 |
| `cel-doki` | — | **Doki** | place | Polish *doki* "the docks" (← *dok*); Kamieniec's shipyard & depot; approved 2026-10-13 |
| `cel-kamieniec-belt` | — | Kamieniec Belt | place | "<system> Belt" convention; approved 2026-10-13 |
| `cel-gate-kamieniec` | — | Kamieniec Gate | place | gates named for the destination; two instances (`sys-penumbra`, `sys-glusza`) per [../CONTENT_AUTHORING.md](../CONTENT_AUTHORING.md#id-naming-rules-mandatory); approved 2026-10-13 |
| `cel-glusza-star` | — | Głusza | place | star carries its system's name (existing convention); approved 2026-10-13 |
| `cel-glusza-prime` | — | Głusza Prime | place | "<system> Prime" convention; approved 2026-10-13 |
| `cel-kantor` | — | **Kantor** | place | Polish *kantor* "trading post, exchange" (← German *Kontor*); Głusza's Unincorporated freeport; approved 2026-10-13 |
| `cel-glusza-belt` | — | Głusza Belt | place | "<system> Belt" convention; approved 2026-10-13 |
| `cel-gate-glusza` | — | Głusza Gate | place | gates named for the destination; two instances (`sys-kamieniec`, `sys-gloam`) per [../CONTENT_AUTHORING.md](../CONTENT_AUTHORING.md#id-naming-rules-mandatory); approved 2026-10-13 |

### Corporations — manufacturers

**APPROVED 2026-10-18.** The ten manufacturing houses whose `maker` slug
(`corp-*`) every manufactured def carries. The `corp-*` prefix is a
registered extension of the frozen id list (user-approved), like the other
kind prefixes; the slugs live in `content/corps.go` (`AllCorps`) and their
emblems in `uikit/emblems`. They are independent suppliers, **not** power
blocs ([FACTIONS.md — Corporate members](FACTIONS.md#corporate-members)).
Rat hulls, ores, minerals and loot stay unattributed.

| id | current name | proposed name | origin | notes |
|---|---|---|---|---|
| `corp-borealis` | Borealis Systems | **Borealis** | Nordic | Latin *borealis* "northern"; renames the house Borealis Systems (drops the form-word); sensors, avionics, electronic warfare |
| `corp-nordvakt` | — | **Nordvakt** | Nordic | Norwegian *nord* "north" + *vakt* "watch, guard"; ordnance, turrets, guidance |
| `corp-iskald` | — | **Iskald** | Nordic | Danish/Norwegian *is* "ice" + *kald* "cold"; energy systems, capacitors, cells |
| `corp-ferrum` | Kombinat Ferrum | **Kombinat Ferrum** | Polish | Polish *kombinat* "combine" + Latin *ferrum* "iron"; keeps its name; heavy hulls, armor, structures |
| `corp-druzstvo` | Družstvo Concordia | **Družstvo** | Moravian | Czech *družstvo* "cooperative, team"; renames the house Družstvo Concordia (drops the Latinate tail); extraction, refining, barging |
| `corp-burza` | — | **Zakłady Burza** | Polish/Moravian | Polish *zakłady* "works, plant" + *burza* "storm"; munitions, energetic materials |
| `corp-celeritas` | Celeritas Courier | **Celeritas** | Latinate | Latin *celeritas* "swiftness"; renames the house Celeritas Courier (drops the form-word); propulsion, express logistics, recovery |
| `corp-praetor` | — | **Praetor** | Latinate | Latin *praetor* "one who goes before; commander"; security services, patrol craft, protective systems |
| `corp-aldemar` | — | **Aldemar Group** | indistinct European | Continental personal-name stock, Germanic *ald-* "old" + *-mar* "renowned"; instrumentation, field repair, components |
| `corp-vantera` | — | **Vantera** | indistinct European | coined from Latin *vānus* "hollow, empty" (*vantera* "the hollowing"); industrial automation, shielding, station systems |

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
| `mod-autocannon-s` | — | **Small Autocannon** | trade | proposed 2026-10-20: calibre + projectile family (fast cycle, short reach) |
| `mod-autocannon-m` | — | **Medium Autocannon** | trade | proposed 2026-10-20: same family, medium calibre |
| `mod-autocannon-l` | — | **Large Autocannon** | trade | proposed 2026-10-20: same family, large calibre |
| `mod-artillery-s` | — | **Small Artillery** | trade | proposed 2026-10-20: calibre + projectile family (slow cycle, long reach) |
| `mod-artillery-m` | — | **Medium Artillery** | trade | proposed 2026-10-20: same family, medium calibre |
| `mod-artillery-l` | — | **Large Artillery** | trade | proposed 2026-10-20: same family, large calibre |
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
| `ammo-emp-s` | — | **Small EMP Shell** | trade | proposed 2026-10-20: calibre + damage profile (was EMP Charge) |
| `ammo-fusion-s` | — | **Small Fusion Shell** | trade | proposed 2026-10-20: (was Fusion Charge) |
| `ammo-ap-s` | — | **Small Armor-Piercing Shell** | trade | proposed 2026-10-20: (was Armor-Piercing Charge) |
| `ammo-plutonium-s` | — | **Small Plutonium Shell** | trade | proposed 2026-10-20: (was Plutonium Charge) |
| `ammo-emp-m` | — | **Medium EMP Shell** | trade | proposed 2026-10-20 |
| `ammo-fusion-m` | — | **Medium Fusion Shell** | trade | proposed 2026-10-20 |
| `ammo-ap-m` | — | **Medium Armor-Piercing Shell** | trade | proposed 2026-10-20 |
| `ammo-plutonium-m` | — | **Medium Plutonium Shell** | trade | proposed 2026-10-20 |
| `ammo-emp-l` | — | **Large EMP Shell** | trade | proposed 2026-10-20 |
| `ammo-fusion-l` | — | **Large Fusion Shell** | trade | proposed 2026-10-20 |
| `ammo-ap-l` | — | **Large Armor-Piercing Shell** | trade | proposed 2026-10-20 |
| `ammo-plutonium-l` | — | **Large Plutonium Shell** | trade | proposed 2026-10-20 |
| `charge-cap-cell` | Capacitor Booster Cell | keep | trade | cap booster fuel, not gun ammo |

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
| `contract-manufacturing-shield-1` | — | **Shield booster order** | trade | durable manufactured goods (Shield Booster, a non-weapon module); offering Doki, delivery Przystań |
| `contract-manufacturing-web-1` | — | **Web units for the yard** | trade | durable manufactured goods (Stasis Web, a non-weapon module); offering Huta, delivery Doki |

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

### Interface strings

| field | current value | proposed value | origin | notes |
|---|---|---|---|---|
| Finances window title (`game/screens.go`, `uikit/screens/finances.go`) | — | **Finances** | trade | the wallet window: holdings over the transaction ledger. Plain trade vocabulary, no coined term. Approved 2026-10-18 |
| Finances holdings labels | — | cash · deployed in orders · receivables · total | trade | the three pools the player's money can sit in: the wallet, the escrow held by open buy orders (refundable on cancel) and the rewards still owed on accepted contracts, plus their total. Approved 2026-10-18 |
| Order mini-window item entry (`uikit/screens/inventory.go`, `uikit/screens/market.go`) | — | **Sell on Market** | trade | posts a standing ask for the clicked stack/item on the docked station's book through the typed order mini-window; grayed (never hidden) while undocked. Approved 2026-10-18 |
| Order mini-window item entry (market rows) | — | **Request on Market** | trade | posts a standing bid for the clicked item through the same window (holding the item is not required). Approved 2026-10-18 |
| Order mini-window mode toggle (`uikit/screens/orderwin.go`) | — | **Sell** / **Request** | trade | the two order sides in the player's words — the book itself keeps Bid/Ask. Re-toggling re-applies the mode's default price. Approved 2026-10-18 |
| Order mini-window title (`uikit/screens/orderwin.go`) | — | **&lt;name&gt; — Order** | trade | follows the "&lt;name&gt; — Info" title convention. Approved 2026-10-18 |
| Manufacturing window title (`game/screens.go`, `uikit/screens/manufacturing.go`) | Factory | **Manufacturing** | trade | the industry window: recipe board, up-front job fee, the three global job slots and Start/Collect/Cancel. User-approved rename of Factory; key B unchanged. Approved 2026-10-20 |
| Refinery window title (`game/screens.go`, `uikit/screens/refinery.go`) | — | **Refinery** | trade | the ore→mineral service window at a `refine` station: ore list, yield preview, Refine. Plain trade noun. Approved 2026-10-20 |
| Market item row's Side cell (`uikit/screens/market.go`) | — | **—** | trade | the tree's parent rows carry no side of their own (the order rows folded beneath them keep Ask/Bid); the em dash reads as "none" while keeping the column visibly present. Approved 2026-10-20 |

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
