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
| Places | Slavic/Nordic blend, time-drifted to the second depth — roots weathered past recognition | `place`, `Nordic` |
| Institutions / corporations | weathered root, optionally + an English form-word; Latinate houses keep the classical shape, never the Latin word | `Latinate` |
| Ship hulls | drifted heritage trade nouns, **one origin per hull**, live stem erased | `Moravian`, `Polish`, `Nordic`, `Latinate` |
| Machine names | caste names are canon: Scuttler, Marauder, Hierarch | `Old Works` |
| Products (modules, charges, minerals, ores, loot) | plain trade nouns of the lingua franca, mechanics-aligned | `trade` |

- Retired names that must not survive in display text: **Solace, Penumbra,
  Gloam, Haven Rest, Thevek Reach, Loomfall, Loomfall Security, Mire
  Corsairs, Gloam Hierarchy**; everything the first drift retired
  (**Starogród, Rudnik, Kuźnica, Młynów, Kamieniec, Pustkowie, Głusza,
  Jernborg, Przystań, Gniazdo, Halda, Huta, Doki, Kantor, Väktare, Kosa,
  Havíř, Furman, Tabor, Custos, Borealis, Nordvakt, Iskald, Družstvo,
  Celeritas**); and everything the second depth retired (**Sterhov, Rudnek,
  Kuźbor, Młyndal, Kamenvik, Pustheim, Głusholm, Jernvold, Havstov,
  Gniazvor, Malmhalda, Hutov, Dokhavn, Kantov, Vaktr, Kosar, Havřek, Furmek,
  Taborek, Custor, Boreall, Nordvakten, Iskeld, Kombinat Ferrum, Družin,
  Zakłady Burza, Celerita, Praetor, Aldemar Group, Far Moravia, Kresy**) —
  names and fragments alike. A retired name is matched whole-word: retired
  forms have lived on as prefixes (Kosar/Kosa, Malmhalda/Halda,
  Taborek/Tabor).
- Authentic diacritics are correct, not risky: the game font (IBM Plex Sans)
  renders all Polish/Czech diacritics plus `€` and `¢`. Keep names sayable
  for English readers; note the lexical root in `notes`.
- Names drift with time depth, not with fashion: see [Time-depth
  drift](#time-depth-drift).
- Full rules and the trait matrix: [AESTHETIC.md](AESTHETIC.md). Setting
  facts: [UNIVERSE.md](UNIVERSE.md). Who the names belong to:
  [FACTIONS.md](FACTIONS.md).

## Registry

### Systems

**Time-depth drift, user-approved 2026-10-26.** The `current name` column now
holds the name this pass retired; `proposed name` is the new, checkably
non-dictionary blend (see [Time-depth drift](#time-depth-drift)).

| id | current name | proposed name | origin | notes |
|---|---|---|---|---|
| `sys-solace` | Sterhov | **Struvok** | place | old town, weathered past *stary*/*gród* — no live stem survives; first-wave heartland, Charter space |
| `sys-penumbra` | Kuźbor | **Kvonice** | place | forge + fort, weathered past *kuźnia*; the Fringe junction |
| `sys-gloam` | Pustheim | **Pušťov** | place | wasteland + home, weathered past *pustka* (ironic on purpose); Unincorporated dead-end |
| `sys-rudnik` | Rudnek | **Rydnok** | place | ore-works, weathered past *ruda*; the Charter-space anchor of the ring |
| `sys-mlynow` | Młyndal | **Młondan** | place | mill + valley, weathered past *młyn*/*dal*; the Fringe |
| `sys-kamieniec` | Kamenvik | **Kamžal** | place | stone + bay, weathered past *kamień*/*vik*; the Fringe |
| `sys-glusza` | Głusholm | **Glůšan** | place | hush + islet, weathered past *głusza*/*holm*; Unincorporated |

### Celestials

| id | current name | proposed name | origin | notes |
|---|---|---|---|---|
| `cel-solace-star` | Sterhov | **Struvok** | place | star carries its system's name (existing convention) |
| `cel-solace-prime` | Sterhov Prime | **Struvok Prime** | place | "<system> Prime" convention |
| `cel-solace-belt` | Solace Belt | Asteroid Belt I | place | "Asteroid Belt <Roman numeral>" convention: numbered per system in celestial order (first belt = I) |
| `cel-haven-rest` | Jernvold | **Jalvrek** | Nordic | iron + rule/keep, weathered past *jern*/*vold*; oldest town, Nordic trace |
| `cel-gate-penumbra` | Gate (Kuźbor) | Gate (Kvonice) | place | "Gate (<destination display name>)" convention; four instances share this id (`sys-solace`, `sys-gloam`, `sys-mlynow`, `sys-kamieniec`) per [../CONTENT_AUTHORING.md](../CONTENT_AUTHORING.md#id-naming-rules-mandatory) |
| `cel-penumbra-star` | Kuźbor | **Kvonice** | place | |
| `cel-penumbra-belt` | Penumbra Belt | Asteroid Belt I | place | "Asteroid Belt <Roman numeral>" convention: numbered per system in celestial order (first belt = I) |
| `cel-penumbra-den` | Gniazvor | **Gnazvor** | place | nest + devour, weathered past *gniazdo*/*vor-* (Old Works flavour); the anomaly in the system |
| `cel-gate-solace` | Gate (Sterhov) | Gate (Struvok) | place | "Gate (<destination display name>)" convention |
| `cel-gate-gloam` | Gate (Pustheim) | Gate (Pušťov) | place | "Gate (<destination display name>)" convention |
| `cel-penumbra-port` | Havstov | **Hauvnok** | place | sea + pier, weathered past *hav*/*stav*; the freeport |
| `cel-gloam-star` | Pustheim | **Pušťov** | place | |
| `cel-gloam-belt` | Gloam Belt | Asteroid Belt I | place | "Asteroid Belt <Roman numeral>" convention: numbered per system in celestial order (first belt = I) |
| `cel-rudnik-star` | Rudnek | **Rydnok** | place | star carries its system's name (existing convention) |
| `cel-rudnik-prime` | Rudnek Prime | **Rydnok Prime** | place | "<system> Prime" convention |
| `cel-halda` | Malmhalda | **Halžek** | place | ore + slag heap, weathered past *malm*/*hałda*; Rydnok's mining outpost |
| `cel-rudnik-belt` | Asteroid Belt I | Asteroid Belt I | place | "Asteroid Belt <Roman numeral>" convention: numbered per system in celestial order (first belt = I) |
| `cel-gate-rudnik` | Gate (Rudnek) | Gate (Rydnok) | place | "Gate (<destination display name>)" convention; two instances (`sys-solace`, `sys-mlynow`) per [../CONTENT_AUTHORING.md](../CONTENT_AUTHORING.md#id-naming-rules-mandatory) |
| `cel-mlynow-star` | Młyndal | **Młondan** | place | star carries its system's name (existing convention) |
| `cel-mlynow-prime` | Młyndal Prime | **Młondan Prime** | place | "<system> Prime" convention |
| `cel-huta` | Hutov | **Hovžan** | place | smelter, weathered past *hut-* (← German *Hütte*); Młondan's refinery platform |
| `cel-mlynow-belt` | Asteroid Belt I | Asteroid Belt I | place | "Asteroid Belt <Roman numeral>" convention: numbered per system in celestial order (first belt = I) |
| `cel-gate-mlynow` | Gate (Młyndal) | Gate (Młondan) | place | "Gate (<destination display name>)" convention; two instances (`sys-rudnik`, `sys-penumbra`) per [../CONTENT_AUTHORING.md](../CONTENT_AUTHORING.md#id-naming-rules-mandatory) |
| `cel-kamieniec-star` | Kamenvik | **Kamžal** | place | star carries its system's name (existing convention) |
| `cel-kamieniec-prime` | Kamenvik Prime | **Kamžal Prime** | place | "<system> Prime" convention |
| `cel-doki` | Dokhavn | **Dukšan** | place | dock + harbour, weathered past *dok*/*havn*; Kamžal's shipyard & depot |
| `cel-kamieniec-belt` | Asteroid Belt I | Asteroid Belt I | place | "Asteroid Belt <Roman numeral>" convention: numbered per system in celestial order (first belt = I) |
| `cel-gate-kamieniec` | Gate (Kamenvik) | Gate (Kamžal) | place | "Gate (<destination display name>)" convention; two instances (`sys-penumbra`, `sys-glusza`) per [../CONTENT_AUTHORING.md](../CONTENT_AUTHORING.md#id-naming-rules-mandatory) |
| `cel-glusza-star` | Głusholm | **Glůšan** | place | star carries its system's name (existing convention) |
| `cel-glusza-prime` | Głusholm Prime | **Glůšan Prime** | place | "<system> Prime" convention |
| `cel-kantor` | Kantov | **Kanšev** | place | trading post, weathered past *kantor* (← German *Kontor*); Glůšan's Unincorporated freeport |
| `cel-glusza-belt` | Asteroid Belt I | Asteroid Belt I | place | "Asteroid Belt <Roman numeral>" convention: numbered per system in celestial order (first belt = I) |
| `cel-gate-glusza` | Gate (Głusholm) | Gate (Glůšan) | place | "Gate (<destination display name>)" convention; two instances (`sys-kamieniec`, `sys-gloam`) per [../CONTENT_AUTHORING.md](../CONTENT_AUTHORING.md#id-naming-rules-mandatory) |

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
| `corp-borealis` | Boreall | **Bůral** | Nordic | northern, weathered past Latin *borealis* (classical *-al* shape kept); sensors, avionics, electronic warfare |
| `corp-nordvakt` | Nordvakten | **Noldvar Guard** | Nordic | north-watch, weathered past *nord*/*vakt* plus the English form-word; ordnance, turrets, guidance |
| `corp-iskald` | Iskeld | **Iškoven** | Nordic | ice-cold, weathered past *is*/*kald*; energy systems, capacitors, cells |
| `corp-ferrum` | Kombinat Ferrum | **Kumbřan Steelworks** | Polish | iron-combine, weathered past *kombinat*/*ferrum* plus the English form-word; heavy hulls, armor, structures |
| `corp-druzstvo` | Družin | **Dvužek** | Moravian | retinue, weathered past Old Slavic *druzina* (Družstvo Concordia → Družstvo → Družin → Dvužek); extraction, refining, barging |
| `corp-burza` | Zakłady Burza | **Bríždan Factory** | Polish/Moravian | storm-works, weathered past *zakłady*/*burza* plus the English form-word; munitions, energetic materials |
| `corp-celeritas` | Celerita | **Cylvar** | Latinate | swiftness, weathered past Latin *celeritas* (classical *-ar* shape kept); propulsion, express logistics, recovery |
| `corp-praetor` | Praetor | **Preštor** | Latinate | commander, weathered past Latin *praetor* (classical *-or* shape kept); security services, patrol craft, protective systems |
| `corp-aldemar` | Aldemar Group | **Alďmar** | indistinct European | old + renowned, weathered past *ald-*/*-mar*; instrumentation, field repair, components |
| `corp-vantera` | Vantera | **Vantera** | indistinct European | kept — already coined, so it needs no weathering; industrial automation, shielding, station systems |

### Ship hulls

| id | current name | proposed name | origin | notes |
|---|---|---|---|---|
| `ship-sloop` | Vaktr | **Vaštrek** | Nordic | watchman, weathered past Swedish *väktare*; precise electronics-heavy frigate |
| `ship-cutlass` | Kosar | **Ždanek** | Polish | scythe-worker, weathered past *kosa*; close-in yard-built brawler |
| `ship-barge` | Havřek | **Hušnek** | Moravian | miner, weathered past Czech *havíř*; the mining barge |
| `ship-hauler` | Furmek | **Furdžan** | Polish | carter, weathered past *furman*; the workhorse hauler |
| `ship-npc-hauler` | Taborek | **Tobřek** | Polish | wagon train, weathered past *tabor*; civilian convoy freighter. Overview role labels documented in [../GAMEPLAY.md](../GAMEPLAY.md) ride along in the content pass |
| `ship-npc-patrol` | Custor | **Kuštor** | Latinate | guardian, weathered past Latin *custos* (classical *-or* shape kept); charter patrol cutter. Same caveat as `ship-npc-hauler` |
| `ship-npc-miner` | — | **Kutřek** | Moravian | digging barge, weathered past Czech *kutat* "to dig"; civilian belt miner hull, shipped 2026-10-28 with `npc-miner-barge` |
| `ship-npc-fleet` | — | **Clastor** | Latinate | fleet, weathered past Latin *classis* (classical *-or* shape kept); the commissioned corporate fleet cutter, shipped 2026-10-28 with `npc-fleet-cutter` (FA `Corporate Fleets`) |
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
| `ammo-emp-s` | — | **40mm ion** | trade | approved 2026-10-22, naval calibre line; small-autocannon family (was Small EMP Shell) |
| `ammo-fusion-s` | — | **40mm high-explosive** | trade | approved 2026-10-22, naval calibre line; small-autocannon family (was Small Fusion Shell) |
| `ammo-ap-s` | — | **40mm armour-piercing** | trade | approved 2026-10-22, naval calibre line; small-autocannon family (was Small Armor-Piercing Shell) |
| `ammo-plutonium-s` | — | **40mm plutonium** | trade | approved 2026-10-22, naval calibre line; small-autocannon family (was Small Plutonium Shell) |
| `ammo-emp-m` | — | **57mm ion** | trade | approved 2026-10-22, naval calibre line; medium-autocannon family (was Medium EMP Shell) |
| `ammo-fusion-m` | — | **57mm high-explosive** | trade | approved 2026-10-22, naval calibre line; medium-autocannon family (was Medium Fusion Shell) |
| `ammo-ap-m` | — | **57mm armour-piercing** | trade | approved 2026-10-22, naval calibre line; medium-autocannon family (was Medium Armor-Piercing Shell) |
| `ammo-plutonium-m` | — | **57mm plutonium** | trade | approved 2026-10-22, naval calibre line; medium-autocannon family (was Medium Plutonium Shell) |
| `ammo-emp-l` | — | **76mm ion** | trade | approved 2026-10-22, naval calibre line; large-autocannon family (was Large EMP Shell) |
| `ammo-fusion-l` | — | **76mm high-explosive** | trade | approved 2026-10-22, naval calibre line; large-autocannon family (was Large Fusion Shell) |
| `ammo-ap-l` | — | **76mm armour-piercing** | trade | approved 2026-10-22, naval calibre line; large-autocannon family (was Large Armor-Piercing Shell) |
| `ammo-plutonium-l` | — | **76mm plutonium** | trade | approved 2026-10-22, naval calibre line; large-autocannon family (was Large Plutonium Shell) |
| `ammo-art-emp-s` | — | **127mm ion** | trade | approved 2026-10-22, naval calibre line; small-artillery family (was Small EMP Artillery Shell) |
| `ammo-art-fusion-s` | — | **127mm high-explosive** | trade | approved 2026-10-22, naval calibre line; small-artillery family (was Small Fusion Artillery Shell) |
| `ammo-art-ap-s` | — | **127mm armour-piercing** | trade | approved 2026-10-22, naval calibre line; small-artillery family (was Small Armor-Piercing Artillery Shell) |
| `ammo-art-plutonium-s` | — | **127mm plutonium** | trade | approved 2026-10-22, naval calibre line; small-artillery family (was Small Plutonium Artillery Shell) |
| `ammo-art-emp-m` | — | **152mm ion** | trade | approved 2026-10-22, naval calibre line; medium-artillery family (was Medium EMP Artillery Shell) |
| `ammo-art-fusion-m` | — | **152mm high-explosive** | trade | approved 2026-10-22, naval calibre line; medium-artillery family (was Medium Fusion Artillery Shell) |
| `ammo-art-ap-m` | — | **152mm armour-piercing** | trade | approved 2026-10-22, naval calibre line; medium-artillery family (was Medium Armor-Piercing Artillery Shell) |
| `ammo-art-plutonium-m` | — | **152mm plutonium** | trade | approved 2026-10-22, naval calibre line; medium-artillery family (was Medium Plutonium Artillery Shell) |
| `ammo-art-emp-l` | — | **203mm ion** | trade | approved 2026-10-22, naval calibre line; large-artillery family (was Large EMP Artillery Shell) |
| `ammo-art-fusion-l` | — | **203mm high-explosive** | trade | approved 2026-10-22, naval calibre line; large-artillery family (was Large Fusion Artillery Shell) |
| `ammo-art-ap-l` | — | **203mm armour-piercing** | trade | approved 2026-10-22, naval calibre line; large-artillery family (was Large Armor-Piercing Artillery Shell) |
| `ammo-art-plutonium-l` | — | **203mm plutonium** | trade | approved 2026-10-22, naval calibre line; large-artillery family (was Large Plutonium Artillery Shell) |
| `charge-cap-cell` | Capacitor Booster Cell | keep | trade | cap booster fuel, not gun ammo; burned from the booster's cell bay |

### Items — minerals

| id | current name | proposed name | origin | notes |
|---|---|---|---|---|
| `min-ferrite` | Ferrite | **Sideron** | trade | Greek *sideros* "iron"; renamed 2026-10-22 |
| `min-nickel` | Nickel | **Nixium** | trade | coined from *nickel*; renamed 2026-10-22 |
| `min-cobalt` | Cobalt | **Zaffrite** | trade | *zaffer*, the historical cobalt oxide pigment; renamed 2026-10-22 |
| `min-silex` | Silex | **Vetril** | trade | Latin *vitrum* "glass"; renamed 2026-10-22 |
| `min-glacite` | Glacite | **Cryolith** | trade | Greek *kryos* "ice" + *lithos* "stone"; renamed 2026-10-22 |
| `min-argon` | Argon | **Helion** | trade | coined from *helium*; renamed 2026-10-22 |

### Items — ores

| id | current name | proposed name | origin | notes |
|---|---|---|---|---|
| `ore-sterngrit` | Sterngrit Ore | keep | trade | *stern* "star" (Germanic/Nordic root) + grit; miner's name |
| `ore-cordite` | Cordite Ore | **Kerngrit Ore** | trade | *kern* "core" + grit, kin of Sterngrit; renamed 2026-10-22 |
| `ore-lucidite` | Lucidite Ore | keep | trade | Latin *lucidus* "clear, glassy"; description says "low-sec favorite" → "Fringe favourite" in the content pass |
| `ore-glacierite` | Glacierite Ore | **Borealite Ore** | trade | Latin *boreas* "north wind"; renamed 2026-10-22 |

### Items — loot

| id | current name | proposed name | origin | notes |
|---|---|---|---|---|
| `loot-navdata` | Encrypted Navdata | keep | trade | |
| `loot-alloy-scrap` | Alloy Scrap | keep | trade | |
| `loot-iced-core` | Iced Core | keep | trade | |
| `loot-hierarch-crest` | Hierarch's Crest | keep | Old Works | caste name; description names "a Pustheim hierarch" |

### NPCs

| id | current name | proposed name | origin | notes |
|---|---|---|---|---|
| `npc-guard-patrol` | Patrol | keep | trade | role label; its `faction` string is renamed below |
| `npc-miner-barge` | — | **Kutřek** | Moravian | digging barge, weathered past Czech *kutat* "to dig"; civilian belt miner (Dvužek), shipped 2026-10-28 |
| `npc-fleet-cutter` | — | **Clastor** | Latinate | the commissioned fleet cutter (Preštor hull, FA `Corporate Fleets`), shipped 2026-10-28 with `ship-npc-fleet`; shares the hull's name, the Kutřek precedent |
| `npc-rat-scuttler` | Scuttler | Scuttler | Old Works | caste name retained (canon) |
| `npc-rat-marauder` | Marauder | Marauder | Old Works | caste name retained (canon) |
| `npc-rat-hierarch` | Hierarch | Hierarch | Old Works | caste name retained (canon) |

### Contracts

| id | current name | proposed name | origin | notes |
|---|---|---|---|---|
| `contract-solace-cleanup-1` | Belt cleanup | keep | trade | description: "the Old Works", "Struvok belt", "Jalvrek" |
| `contract-solace-supply-run-1` | Supply run to Havstov | **Supply run to Hauvnok** | place | description: "Hauvnok", "the Jalvrek market", "the Fringe" |
| `contract-solace-cordite-1` | Cordite order | **Kerngrit order** | trade | renamed 2026-10-22 with the ore; description: "Jalvrek's refinery", "Struvok belt" |
| `contract-penumbra-den-hunt-1` | Gniazvor hunt | **Gnazvor hunt** | place | description: "the Gnazvor", "the Hauvnok office" |
| `contract-penumbra-lucidite-1` | Lucidite contract | keep | trade | description: "Hauvnok", "the Kvonice belt" |
| `contract-manufacturing-shield-1` | — | **Shield booster order** | trade | durable manufactured goods (Shield Booster, a non-weapon module); offering Dukšan, delivery Hauvnok |
| `contract-manufacturing-web-1` | — | **Web units for the yard** | trade | durable manufactured goods (Stasis Web, a non-weapon module); offering Hovžan, delivery Dukšan |
| `contract-war-materiel-1` | — | **Materiel for the front** | trade | the Commission's haul order (Medium Artillery, three units); offering Dukšan, delivery Hauvnok — the Kvonice staging yard; war contract, reward mints (pre-existing payout rule) |
| `contract-war-kill-1` | — | **Clear the front** | trade | the Commission's kill order (two Marauders); offering Dukšan, the Kamžal front; description cites the Kvonice belt |

### Worlds

| id | current name | proposed name | origin | notes |
|---|---|---|---|---|
| `world-default` | New Pilot | keep | trade | out-of-universe scenario name; description names "Jalvrek" and "Struvok" and is swept with the rest (lead-approved 2026-10-26) |
| `world-showcase` | Showcase | keep | trade | test world; scenario details name the current places and hulls, and are swept too (tests pin ids, not names) |

### Strings without ids

| field | current value | proposed value | origin | notes |
|---|---|---|---|---|
| `universe.json` `region` | Far Moravia | **the Wild Fields** | place | one weathering serves both levels; the article is part of the value, like "the Fringe" |
| `universe.json` `constellation` | Kresy | **the Wild Fields** | place | same value as the region — the map is one district of one wild country |
| `npcs/guards.json` `faction` | Loomfall Security | **Charter Authority** | Latinate | full form "Wild Fields Charter Authority" ([FACTIONS.md](FACTIONS.md#the-charter-authority)) — the same authority the region and constellation are named for. Far Moravia stays retired (line above): no record, full form included, carries it |
| `npcs/rats.json` `faction` (Scuttler, Marauder) | Mire Corsairs | **Old Works** | Old Works | two placeholder strings collapse into one faction |
| `npcs/rats.json` `faction` (Hierarch) | Gloam Hierarchy | **Old Works** | Old Works | same |
| `npcs/miners.json` `faction` | — | **Dvužek** | Moravian | corp roster name reused as a faction string (see the corporations table); the extraction/barging house its miners belong to |
| `npcs/fleets.json` `faction` | — | **Corporate Fleets** | Latinate | the commissioned bloc ([FACTIONS.md](FACTIONS.md#the-svaz-and-the-commission)); one faction string now, per-corp rows later when a corp needs its name on screen |
| war (log, contracts, starmap) | — | **the Long Attrition** | place | the war of the Wild Fields; the name echoes FACTIONS' "attrition; no negotiation" ([FACTIONS.md](FACTIONS.md#the-long-attrition)) |
| the confederation (+ "the empire" in prose) | — | **the Svaz** | Moravian | a weathered league (*svaz* "union"); heart in the old worlds, a dues line and a warrant at the rim — treaty, dues, the Commission; no office and no officer in the Wild Fields ([UNIVERSE.md](UNIVERSE.md#the-old-worlds)) |
| the war mandate | — | **the Commission** | Latinate | the Svaz's commission: the warrant the corporate fleets are raised and crewed under |
| the pre-settlement civilisation | — | **the Predecessors** | trade | the plain word, not a coined name: they are the Predecessors because that is what they were. The scholars' word for them is worse and stays offscreen; folklore gives them no other name |
| the Svaz's machine ban, in institutional form | — | **the Kanžar Inspectorate** | Latinate | *censura* "assessment, censure", weathered to *kanž-* + the classical *-ar* shape (Cylvar, Kuštor); the holder of the ban on machine minds and autonomous self-replicating systems. Offstage: no office in the Wild Fields, no officer, never seen |
| Svaz cantons beside the Wild Fields | — | **the Žemdan**, **the Lešvok**, **the Můstvan** | place | *ziemia*/*země* "land", *les* "forest", *most* "bridge", all weathered past recognition (*zem* → *žem-*, *les* → *leš-*, *most* → *můst-*); named-but-unmapped — prose flavour only, never a system, gate or celestial |
| a fallen system's station | — | **Remnant** | trade | berth, market and repair stay live, the charter's desk goes dark ([FACTIONS.md](FACTIONS.md#freeports-and-independents)) |

`traffic/*.json` carry no display names (keyed by system id) and need no row.

The Predecessors, the Kanžar Inspectorate and the three cantons are
author-confirmed 2026-10-28 (the lore pass); none of them reaches
`content/data/**` — they live in prose and, for the inspectorate at most, one
Show Info line.

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
| Refinery batch strings (`uikit/screens/refinery.go`, `sim/system_economy.go`) | — | **One batch needs 100 ore — you hold &lt;n&gt; &lt;ore&gt;.** / **Refining runs in whole batches of 100 ore** | trade | refining is atomic in 100-ore batches: the window's status names the shortfall (Refine grayed below one batch) and the sim's refusal log names the partial quantity. Approved 2026-10-20 |
| Refinery destination strings (`uikit/screens/refinery.go`, `sim/system_economy.go`) | — | **Yield @ qty → hangar** (column) / **→ hangar** (status suffix) / **(station hangar)** (log suffix) | trade | refined minerals always land in the station hangar, never the ship's hold: the preview column names where the yield goes, the status line ends input → output → destination, and the sim's "Refined …" log line closes with the store. Approved 2026-10-21 |
| Market item row's Side cell (`uikit/screens/market.go`) | — | **—** | trade | the tree's parent rows carry no side of their own (the order rows folded beneath them keep Ask/Bid); the em dash reads as "none" while keeping the column visibly present. Approved 2026-10-20 |
| Market browse tree (`uikit/screens/market.go`, `content/data/**` `group`) | — | **All** · **Ships** · **Modules** · **Ammunition** · **Materials** · **Loot** (+ children: **Frigates**, **Destroyers**, **Industrials**, **Barges**, **Rat hulls**; **High/Mid/Low** with **Turrets**, **Launchers**, **Propulsion**, **Shield**, **EWAR**, **Capacitor**, **Armor**; **Autocannon rounds**, **Artillery shells**, **Cap cells**; **Ore**, **Minerals**) | trade | the browse pane's branch labels: the window's own `All` root over the content `group` paths. Plain trade/class nouns — the same words the hull and module classes already use; `Rat hulls` names the Old Works hulls that stay listed though they cannot be bought. No coined term. Amended 2026-10-22: **Autocannon rounds** (was Autocannon shells), renamed with the naval calibre line. Proposed 2026-10-22, awaiting user approval |
| Market tree header (`uikit/screens/market.go`) | — | **Browse** | trade | the browse pane's fixed column header (it sorts nothing; the branch order is the content tree's). Proposed 2026-10-22, awaiting user approval |
| Market search field (`uikit/screens/market.go`) | — | placeholder **Search** | trade | the field above the tree: filters the selected branch live by name, group path and maker; `Ctrl+F` focuses it. Plain imperative noun, no legend. Proposed 2026-10-22, awaiting user approval |
| Market search empty state (`uikit/screens/market.go`) | — | **No matches** | trade | the one dim row a query with no hits renders (never an empty table); the clearing rule is the field's own. Proposed 2026-10-22, awaiting user approval |
| Market cross-station availability (`uikit/screens/market.go`) | — | Qty suffix **&lt;n&gt; · &lt;m&gt; more** · title detail **· cheapest &lt;station&gt; &lt;price&gt;** | trade | the Qty cell's dim suffix counts the OTHER market stations whose books hold an ask for the item; the selected row's title names the cheapest such station and ask (e.g. `· cheapest Dukšan 112 €`). Local columns stay local; the suffix is a lead, not a merged book. Proposed 2026-10-22, awaiting user approval |
| Fitting summary capacitor verdict (`uikit/screens/fitting.go`) | — | **Cap stable** / **Cap unstable (~<n> s)** | ship | the fitting window's projected capacitor verdict under the fit's running modules: stable while their summed GJ/s stays under the recharge curve's peak rate, else the seconds the charge lasts, with a drain-vs-recharge bar under it. The three summary row labels are **Total DPS** · **Cap drain** · **Cap recharge**. Proposed 2026-10-21, awaiting user approval |
| Station service labels (`content/services.go`) | — | **Repair** · **Market** · **Fitting** · **Manufacturing** · **Refinery** | trade | the `content.Services` table's display names: the services row's button tooltips and, for the four with a window, the window titles already registered above (Market/Fitting/Manufacturing/Refinery). `repair` has no window — the sim repairs on `CmdRepair` — so it draws no button. Approved 2026-10-21 |
| Bay load log (`sim/clips.go`) | — | **&lt;module&gt; loaded with &lt;round name&gt; (&lt;n&gt; rounds)** | trade | the one quiet line per completed bay load, `EvModule`, player modules only — the same line confirms a `CmdSetAmmo` choice (`<module> loaded with <round> (<n> rounds)`, the words the ammo-selection proof reads). Proposed 2026-10-22, awaiting user approval |
| Waiting-for-ammo log (`sim/system_weapons.go`, `sim/command.go`) | — | **&lt;module&gt; waiting for ammo selection** | trade | a bay empty with two or more round types in cargo and none selected: one `SevInfo` line, then a silent deactivate — never the dry-gun warning. Proposed 2026-10-22, awaiting user approval |
| Ammo selection cleared (`sim/command.go`) | — | **&lt;module&gt; ammo selection cleared** | trade | `CmdSetAmmo ""`: the bay dumped to cargo and the gun re-resolved by the bay rule (auto-load one type, wait on two or more). Replaces the retired "set to auto ammo". Proposed 2026-10-22, awaiting user approval |
| Bay refusals (`sim/command.go`) | — | **Cannot load &lt;round&gt; into &lt;module&gt;** | trade | unchanged wording; now also covers a round of another family and a reload with no rounds in cargo. Proposed 2026-10-22, awaiting user approval |
| Belt-cleared log (`sim/system_damage.go`) | — | **&lt;belt name&gt; cleared** | trade | the high-sec belt response's news line: the last live rat of a belt died, whoever landed the kill (the anomaly-cleared line already uses the same "&lt;place&gt; cleared" form). Proposed 2026-10-21, awaiting user approval |
| Docked backdrop caption (`game/scenes.go`) | — | **Docked: &lt;station name&gt;** | trade | the station view's backdrop caption: the docked-at station's display name after a plain colon, set in the display face lower-centre over the hangar plate. No coined term — the player reads where they are. Approved 2026-10-21 |
| High-slot mount labels (`uikit/screens/fitting.go`) | — | **Turret mount** / **Launcher mount** | ship | the two mount kinds drawn on an empty high-slot tile and in the hull's SLOTS section: guns and the mining laser are turret-mounted, rockets and missiles launcher-mounted (EVE's split). The short tile form is **Turret** / **Launcher**. Proposed 2026-10-22, awaiting user approval |
| Mount sizes (`uikit/screens/fitting.go`, `content/data/ships`) | — | **Small** / **Medium** / **Large** | trade | a mount's size class, rendered beside its kind (**Small turret mount**); the same three words the module names already use (Small/Medium/Large Autocannon). Proposed 2026-10-22, awaiting user approval |
| Module class labels (`content/fit.go` `ClassLabel`, fitting window quota readout) | — | **Class I** / **Class II** / **Class III** | trade | the grid/cpu class vocabulary in Roman numerals — never "tier" or a number of grid units. Proposed 2026-10-22, awaiting user approval |
| Class quota readout (`uikit/screens/fitting.go`) | — | **Grid Class I 5/5** · **CPU Class II 2/3** | trade | the fitting window's per-axis quota line: modules of a class fitted over the hull's allowance, per axis. Proposed 2026-10-22, awaiting user approval |
| Fitting refusal lines (`sim/system_economy.go` `fitIssueText`) | — | **No free &lt;kind&gt; mount for &lt;module&gt;** · **&lt;module&gt; is oversize for the hull's &lt;kind&gt; mounts** · **&lt;module&gt; exceeds the hull's &lt;axis&gt; Class &lt;n&gt; quota** | ship/trade | the SevWarning a refused `CmdFitModule` logs: the rack has no spare mount of that kind (or the module is too big for every mount of it), or the fit would push its grid/cpu class count past the hull's quota. Validator text says the same in its own voice ("needs a &lt;kind&gt; mount and hull &lt;id&gt; has none free", "is oversize for the hull &lt;id&gt; &lt;kind&gt; mounts", "exceeds hull &lt;id&gt;'s grid Class II quota (4 modules, quota 3)"). Proposed 2026-10-22, awaiting user approval |
| Rack charge menu entries (`uikit/screens/rack.go`) | — | **Load &lt;round name&gt;** | trade | one entry per round type the fitted bay could take and that sits in cargo, in item-id order (e.g. **Load 40mm ion**, **Load Capacitor Booster Cell**); the same verbs are the charge picker a waiting cell's left click opens, and each emits the `CmdSetAmmo` Cycle charge does. Proposed 2026-10-23, awaiting user approval |
| Rack bay row in tooltips (`uikit/widgets/tooltip.go` `ClipTip`) | — | **Clip** row: **&lt;bay&gt; m³ · &lt;n&gt; rounds · &lt;cycle&gt; s** | trade | the charge block's bay line on a charge-fed module, e.g. **Clip** · `0.2 m³ · 40 rounds · 3.0 s` — bay capacity, whole rounds of the round it would load, one load cycle. No round chosen reads **waiting for ammo selection** (no count); bayless modules carry no row. Proposed 2026-10-23, awaiting user approval |
| Rack waiting badge (`uikit/screens/rack.go`) | — | *(no text)* an `Info`-blue corner dot on the tile | trade | palette-only marker on a charge-fed cell in the waiting state (empty bay, two or more round types in cargo, none selected) — the same state where a left click opens the picker instead of toggling. No label; proposed 2026-10-23, awaiting user approval. **Amended 2026-10-23 (form pass):** the role stays `Info` (the palette is untouched) and the dot pulses on an eight-phase ladder like the kit's cursor — the naming status above is unchanged, only the visual treatment moved |

## Time-depth drift

The world names are not contemporary words pushed through a dictionary; they
are what those words became over generations. Approved 2026-10-26 for the
place, hull, corporation and region layers, in two depths: the first drift
blended Slavic and Nordic forms; the second — **morpheme erasure** —
weathered them past recognition, because a name that still holds a live stem
(*jern* in Jernvold, *młyn* in Młyndal) reads as a translation, not as a
history.

- **Morpheme erasure.** No segment of a name may survive as a live dictionary
  word in Polish, Czech, Swedish, Norwegian, Latin or English. Vowel rotation
  and consonant weathering carry the root away (*jern* → *jal-*, *młyn* →
  *młon-*, *kamień* → *kamž-*, *hav* → *hauv-*, *dok* → *duk-*), and
  compound seams fuse instead of stacking (Havstov → Hauvnok, Malmhalda →
  Halžek). What may survive is the shape of the family, not the word: the
  glue suffixes still trace who settled where, the way EVE's Minmatar names
  do. The first drift was too light — it kept its stems — and is retired.
- **The blend signal.** Slavic and Nordic weathering trade places: `-vok`,
  `-nok`, `-vice`, `-dan`, `-žal`, `-šan`, `-vrek`, `-šek`, `-žan`, `-šev`,
  `-řek`, `-džan`. A chain that reads *family recognisable, word dead* is the
  target: the Kamžal line is plainly stone-country and holds no word any
  dictionary knows.
- **Houses** take a weathered root, optionally plus an English form-word:
  Noldvar Guard, Kumbřan Steelworks and Bríždan Factory are canon as given;
  the bare roots stand alone (Bůral, Iškoven, Dvužek, Cylvar, Preštor,
  Alďmar). Vantera is already coined and keeps its name.
- **The Latinate rule.** Latinate-class names keep their classical *shape*
  while the Latin itself is unrecognisable: Bůral holds `-al`, Cylvar `-ar`,
  Kuštor and Preštor `-or` — but *borealis*, *celeritas*, *custos* and
  *praetor* are dead as segments.
- **Regions and constellations** weather like places: Far Moravia and Kresy
  are one country now, **the Wild Fields** — article included, like "the
  Fringe" — in both `universe.json` fields and every system file.
- **Diacritics stay inside the font-verified set** (Polish + Czech: ą ć ę ł ń
  ó ś ź ż č ď ě ň ř š ť ů ž, plus í for the Czech long i). No new glyph gets
  smuggled in with a name.
- **Every new name must be checkably not a live dictionary word** in Polish,
  Czech, Swedish, Norwegian, Latin or English. If a dictionary knows it, it
  has not weathered far enough.
- **Sayable for an English reader**, and the root is noted in the registry row
  like any other invented name.

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
3. **Weather a real root past recognition, then note it** in `notes`
   (language + gloss, e.g. `*hav* "sea" (Norse) + *-stav* "pier"`). Coin words
   only from real morphemes, and run [Time-depth drift](#time-depth-drift):
   the root may be traceable in `notes`, never in a segment of the shipped
   name — a name that still holds a dictionary word has not weathered.
4. **Write the name with authentic diacritics** and check it is sayable by
   an English reader. Diacritics render; they are not a risk.
5. **Never mint a name that echoes a retired one** (list above), including
   as a nickname or a joke. A retired name is guarded whole-word, so a
   current name may carry one as a prefix — but a *live stem* is what the
   second depth retired, so do not reintroduce one (no *-młyn-*, *-jern-*,
   *-hav-*).
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
- MUST note the lexical root of every invented name in `notes`, and MUST
  weather that root past recognition in the shipped name itself (no live
  dictionary segment; see [Time-depth drift](#time-depth-drift)).
- MUST use the `€` / `¢` string conventions here in all UI and log text, and
  Charter space / the Fringe / Unincorporated for zones.
- NEVER leave a retired name — Solace, Penumbra, Gloam, Haven Rest, Thevek
  Reach, Loomfall, Loomfall Security, Mire Corsairs, Gloam Hierarchy, or any
  name either time-depth pass retired (Starogród … Kresy, full list in the
  quick reference) — in display text: names, titles, descriptions, UI
  strings, log lines. Guarded by `content/origin_desc_test.go`; a new retired
  name extends that list.
- NEVER reserve the caste names Scuttler, Marauder and Hierarch for anything
  but the Old Works.
