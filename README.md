# Entieve

**A single-player 2D space sandbox, modelled on EVE Online, running entirely
on your machine.** One ship, seven systems, no subscriptions, no server, no
other players — just the frontier and its ledgers.

You take jobs, mine asteroids, fight rats, haul ore, trade on real order
books, and spend the money on ships and modules. The rules you know from EVE
are here: four damage types, shield / armour / structure, capacitor pressure,
turret tracking and falloff, ammo with purpose. Everything is simulated
locally at 60 ticks per second — the universe keeps its books whether you
watch or not.

```
Download: https://github.com/maxwelljens/entieve/releases
Play from source: go run ./cmd/entieve run
```

---

![Vaštrek next to Jalvrek station, with the overview, log and ship HUD open](screenshots/1.png)

## Fly

Keyboard and mouse, top-down on one system at a time: approach, orbit,
align, warp, dock, jump gates, lock targets, run modules. The overview shows
what is on your grid; the scanner sweeps for anonymous long-range contacts
you have to go see for yourself. Warp to 0, 10 or 50 km — your landing is
your choice. Time acceleration (1×–8×) shortens the trips.

Everything lives in windows you toggle from the keyboard (`I`, `F`, `G`,
`B`, `C`, `N`, `H`, `P`, `Shift+L`) or open from the navigation rail on the
left — hover for the name and hotkey, grayed entries tell you what you
cannot use yet, and the rail collapses when you want the screen back.
Controls and concepts: [docs/GAMEPLAY.md](docs/GAMEPLAY.md).

![Station interior with contracts, market, inventory and fitting windows open](screenshots/2.png)

## Trade and build

Docking opens the station services view: Undock, Cargo, and one button per
service *this* station actually offers — over its contract board. Contracts
give you specific work (kill, mine, deliver) and pay money and standing.

The market is a real order book: broker fees, sales tax, and prices that
move only when real goods change hands — yours, the agents', or a convoy's
fresh supply arriving. Refining turns ore into minerals; industry stations
run timed manufacturing jobs (minerals → hulls, modules and charges) and
their craft grows with every completed run; the hangar keeps your ships
between trips.

Fitting is slot-based — turrets and mining lasers up front, propulsion and
electronic warfare in the middle, tank in the back — and the window shows
what every change does to the hull: shield, armour, structure, capacitor
and recharge, speed, alignment, signature and per-type resists. Four player
hulls to work toward; the market is where the parts come from.

![Fight at the Kvonice belt with a Scuttler locked, asteroids in the overview and the EMP turret stats open](screenshots/3.png)

## Fight and mine

Mining is honest: lock the rock, switch on the laser, fill the ore hold.
Combat is decided by numbers you can see — tracking against your target's
speed and angle, range against falloff, ammo chosen for the resists you are
shooting, and a capacitor that the guns and the tank share. Wrecking shots
happen. So do losses: your pod is recovered and the insurer issues a fresh
Vaštrek.

Rats work the belts and defend them — skittish in Charter space, fighting to
the death out in the Unincorporated systems — and civilian miners work them
too, if you let them. Combat anomalies hide the boss rats and the best loot.

## Yours to change

Drop a PNG named after a content ID into `assets/` and the game draws it
instead of the built-in art: `assets/hulls/ship-sloop.png`,
`assets/stations/cel-haven-rest.png`. Any size works, missing files fall
back to the procedural art, and an empty `assets/` plays exactly like the
stock game. Sizing and fallback rules: [docs/ART.md](docs/ART.md).

## Universe

Seven systems in the Wild Fields frontier — Struvok and Rydnok (Charter
space), Kvonice, Młondan and Kamžal (the Fringe), Pušťov and Glůšan
(Unincorporated) — with escalating danger between the stargates. The
economy is populated: traders, miners and industrialists trade, mine and
manufacture alongside you, and convoys haul real cargo system to system.

## Legal

Licence: **European Union Public Licence 1.2** (see [LICENSE](LICENSE)) —
© 2026 The Entieve Authors, distributed without warranty.

Set in **IBM Plex Sans** (SIL Open Font License 1.1). Interface icons from
[game-icons.net](https://game-icons.net) (CC BY 3.0); every icon's author
credit is kept in [`uikit/icons/CREDITS.md`](uikit/icons/CREDITS.md).

---

For developers and agents: [AGENTS.md](AGENTS.md) (commands, layout,
release flow). Design contract: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md);
simulation reference: [docs/SIM.md](docs/SIM.md); release history:
[CHANGELOG.md](CHANGELOG.md).
