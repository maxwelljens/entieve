# The Wild Fields

The binding lore rulebook for Entieve's setting. Everything under
`docs/lore/` applies to all content — hand-written or AI-authored — and
nothing may contradict it. Companions: [FACTIONS.md](FACTIONS.md) (who lives
here), [AESTHETIC.md](AESTHETIC.md) (how everything sounds and looks),
[NAMING.md](NAMING.md) (every entity's name). Mechanics stay in
[../GAMEPLAY.md](../GAMEPLAY.md); this file is setting only.

## The Wild Fields

**The Wild Fields** is the region Entieve is set in, and the name of its
constellation too — one frontier gold-rush district: small, industrial, and a
long way from anywhere that matters. The playable map is one district of it,
seven systems on the ring of stargates:

| System | Zone | What it is |
|---|---|---|
| **Struvok** | Charter space | the first-wave heartland; the safe lanes |
| **Rydnok** | Charter space | the ore-works country; the ring's mining anchor |
| **Kvonice** | the Fringe | the junction of the rush: yards, richer ore, a freeport |
| **Młondan** | the Fringe | the refinery step of the chain |
| **Kamžal** | the Fringe | shipyard country; the depot at the ring's edge |
| **Pušťov** | Unincorporated | the dead-end frontier — Old Works country |
| **Glůšan** | Unincorporated | the hush: one freeport and not much else |

Everything else in the Wild Fields is off the map: other districts, other
constellations, other rushes. Mention them if you must; name nothing.

The Zone column is where each system stands at the start of a run. The war
moves those answers ([Zones: charter reach](#zones-charter-reach)); the names
never move.

**Binding rules**

- MUST keep the Wild Fields a working frontier district — mines, refineries,
  freeports, hazard tape, invoices.
- MUST keep every named system, constellation and region registered in
  [NAMING.md](NAMING.md); unregistered geography does not exist.
- NEVER present the region as a kingdom, empire, realm, or prophecy. It is a
  place where people work.

## The rush and the ore

The rush is about ore. The Wild Fields' belts carry exotic ores that turned out
to be the feedstock for a manufacturing breakthrough in the old worlds — and
that was enough. Capital, contractors and pilots arrived to cut rock, and
they have not stopped arriving.

The economy is the whole story. Mining lasers cut ore out of belts;
refineries turn it into minerals; workshops turn minerals into hulls,
modules and charges; contract boards pay for culling, hauling and delivery;
markets remember what arrived and what was shot down. A pilot is a link in
that chain, not the hero of it.

What the breakthrough actually makes is nobody's business on the frontier.
The feedstock pays. That is the entire economic argument.

**Binding rules**

- MUST treat the ores as the reason everyone in the Wild Fields is here.
- MUST keep the economy quotidian: bounties, contracts, market stock,
  refinery yields, insurance premiums.
- NEVER name the manufacturing breakthrough or its product. It is a demand
  curve, not a plot.
- NEVER make any single ore sacred, cursed or unique. Rocks are commodities.

## The settlement waves

Two waves of people live in the Wild Fields, one on top of the other.

**The first wave** arrived generations ago: European settlers of Moravian,
Polish and Nordic heritage, in numbers large enough to found towns. Those
towns endure — many thinned out, some abandoned. Their stations are still
the region's infrastructure: sturdy, patched, and older than everyone using
them.

**The second wave** is now: corporates under charter, independents, and
pilots — the player among them — chasing the ore rush. They sleep in the
first wave's berths, weld onto the first wave's hulls, and pay in a currency
the first wave carried with it.

The waves did not stay separate. The second wave moved into the first wave's
buildings, started working, and — under the Commission — started a war:
corporate fleets against the Old Works, fought over the old towns' berths and
belts. The first wave's people are not a side in it. They are the
infrastructure it happens to.

**Binding rules**

- MUST keep the first wave old and unfinished: towns thinned, places
  abandoned, infrastructure worn.
- MUST keep the second wave current: corporates, independents, pilots,
  freeports.
- NEVER give the waves dates, calendars or timelines. "Generations" is as
  precise as lore gets.
- NEVER make the first wave a lost golden age. It was settlers with tools.

## The old worlds

The homelands the settlers left are **the old worlds** — plural, distant, and
deliberately unnamed. "Moravian", "Polish", "Nordic" and "Latinate" are
heritages: language, work songs, building styles, and the formal register the
charter institutions still write in. They are not states, not governments,
not flags. The manufacturers take their names from these heritages (see
[FACTIONS.md — Corporate members](FACTIONS.md#corporate-members)) — the
heritage shapes the house, never the other way round.

Nobody in the Wild Fields sends mail home and gets an answer.

The one institution that reaches across the heritage lines is not an old-world
government either. **The Svaz** is a league *of* the corporations — a treaty, a
dues schedule and the Commission — with no territory, no office and no officer
in the Wild Fields. It is the closest thing the region has to an empire, and
the least able to act like one. It sends warrants and invoices, not mail.

**Binding rules**

- MUST refer to origins as heritage cultures, never as countries or powers.
- NEVER name Earth, Terra, Sol, or any homeworld or star of origin.
- NEVER use the old worlds as a plot lever: no embassies, no reinforcements,
  no recall orders.

## Zones: charter reach

Security status is **charter reach** — how far the colonial authority's
patrols, courts and insurers actually function. The Wild Fields speaks of three
zones:

| Zone | Code value | What it means |
|---|---|---|
| **Charter space** | `high` | patrols answer, courts sit, insurers pay |
| **the Fringe** | `low` | contested; law is a suggestion |
| **Unincorporated** | `null` | no charter presence; Old Works territory in practice |

| System | Zone |
|---|---|
| Struvok | Charter space |
| Rydnok | Charter space |
| Kvonice | the Fringe |
| Młondan | the Fringe |
| Kamžal | the Fringe |
| Pušťov | Unincorporated |
| Glůšan | Unincorporated |

UI copy uses the zone words with a complementary status label — "Charter ·
secure", "Fringe · contested", "Unincorporated · no charter". The values
`high` / `low` / `null` survive only in code and JSON (`"security"` in
`content/data/systems/*.json`) as implementation; they are never
world-facing words.

Charter reach is not a fixed map any more. Since the **Long Attrition** the war
moves it: a system the charter loses drops to Unincorporated, a system it takes
back returns to its authored tier, and the **front** — the systems where the
two answers meet across a gate — is where that happens. The zones are the war's
scoreboard, not its constitution.

The starmap carries the war as a second layer over the zone words. These are
the words for it:

| Word | What it marks |
|---|---|
| **Charter-held** | a system the charter answers in today: its authored zone word is live |
| **Works-held** | a system the Old Works hold: Unincorporated in practice, belts hostile, its stations remnants |
| **Front** | a system gated to the other side, carrying a contest meter — the war's moving edge |

A front that moves reports as "Contested — Kvonice holds at 28%"; a flip is
"Fall — Kvonice lost to the Old Works" or "Reclaimed — Kvonice returns to the
charter". The overlay never replaces the zone word: a Works-held system is
Unincorporated and says so.

**Binding rules**

- MUST use the zone words in all lore, UI text and description fields.
- NEVER write "high-sec", "low-sec", "null-sec" or "security status" in
  world-facing text.
- NEVER give the zones governments, capitals or borders. They are degrees of
  reach, not countries: Charter space thins out, the Fringe frays,
  Unincorporated simply stops.
- MUST let the war move the reach: a system's zone word is today's answer, not
  a border, and the front overlay (Charter-held / Works-held / Front) is the
  only place the war is drawn.

## Daily life and the pilot's place

Life runs on paperwork and throughput. Dock crews, refinery shifts, convoy
schedules, contract boards, berthing fees. Charter space is orderly and slow.
The Fringe is loud and priced accordingly. Pušťov is quiet in the wrong
way.

The player is an **ordinary pilot**: skilled, mortal, insured. Licensed work —
culling pests, hauling cargo, mining ore, running the odd delivery into worse
places — plus whatever the Fringe lets you get away with. Pilots buy their
hulls, fit their racks and pay their fines like everyone else. Freeports take
anyone who pays the fee.

**Binding rules**

- MUST keep the player ordinary: a working pilot with a wallet, a hull and a
  to-do list.
- MUST ground day-to-day texture in work: shifts, holds, cycles, invoices,
  schedules.
- NEVER frame the player as chosen, destined, prophesied or secretly
  important. Nobody in the Wild Fields is waiting for anyone.

## Death and insurance

Ships die; pilots mostly do not. Losing a hull ends with the escape pod
getting clear and being picked up. The hull insurer — the charter authority
or the charter the pilot flies under — then issues a replacement: a fresh
hull of the insured class, handed over where the pilot wakes up.

Death in Entieve is paperwork. The log line `Respawned at <station> in a
<hull>` is the insurer's replacement notice arriving. Clone banks, uploaded
minds and resurrection technology do not exist here.

**Binding rules**

- MUST treat player death as pod rescue plus an insurance claim.
- MUST treat the replacement hull as ordinary insurance: premiums,
  deductibles and forms, not miracles.
- NEVER add clone bodies, backups, mind uploads, resurrections or any
  transhuman machinery.

## The euro

The currency is the **euro**, counted in euros and eurocents (`€` / `¢`) —
an old-world remnant that came with the settlement waves and never left.
Notes, price tags and accounting software all crossed over with the first
wave; the second wave inherited the habit and kept it, because everyone
prices in euro and converting on the frontier is worse.

Amounts read `1,234.56 €` (the sign trails the amount); amounts under one
euro read `50¢`. In prose use
"euro(s)" and "cent(s)" — "forty thousand euro", "fifty cents". The full
string convention, including what replaces today's placeholder "credits" /
"cr" text, is specified in [NAMING.md](NAMING.md#money-strings).

**Binding rules**

- MUST price everything in euros and eurocents.
- MUST keep the euro a habit, not a plot: it is money, not a symbol.
- NEVER introduce another currency, crypto-credit or frontier scrip as a
  unit of account. Slang for money is fine; the unit is the euro.
