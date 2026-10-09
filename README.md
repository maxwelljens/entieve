# Entieve

**A love letter to EVE Online.** A production-grade 2D space sandbox in Go
(Ebitengine): fit ships, fight rats, mine belts, haul ore, trade, refine,
repair, and jump gates across a star cluster — all with EVE's soul intact:
four damage types, shield/armor/structure, capacitor warfare, turret tracking
math, and an overview that never lies.

```
go run ./cmd/entieve run        # play
go test ./...                   # the whole test suite (headless, no GPU needed)
```

## The game

- **Ships with slots.** High/mid/low fitting on four player hulls (sloop,
  cutlass, barge, hauler) — turrets and mining lasers up top, tank and ewar
  below.
- **EVE combat math.** Turrets use the classic tracking formula
  (`0.5^((angular/(tracking·sig/40k))² + (max(0,d−optimal)/falloff)²)`) with
  wrecking shots, ammo that changes your damage profile, and resists across
  EM/thermal/kinetic/explosive.
- **Shield / armor / structure** with per-layer resists and overflow
  carry-over. Lose your ship and you pod home in a fresh sloop.
- **Capacitor** with EVE's recharge curve (peak at ~25%), boosters with
  charges, active tank that competes for the same cap as your guns.
- **A living sandbox.** Mine asteroids into your ore hold, loot wrecks, claim
  bounties, dock at stations for repair/market/refine/fitting, buy and sell on
  a finite-stock market, refine ore into minerals, store ships and modules in
  your hangar.
- **A star cluster.** Three systems of escalating danger — Solace, Penumbra,
  Gloam — connected by stargates, plus combat anomalies with boss rats and the
  best loot. Rats patrol their belts, defend their homes against miners and
  shooters, and temperament follows security: high-sec rats are skittish,
  null-sec rats fight to the death.
- **Sensors with teeth.** The overview only shows what's on your grid
  (250 km) — distant rocks and rats stay hidden until you approach. The
  scanner window sweeps for anonymous long-range contacts ("Ship ×3"), and
  warp-to-within (0/10/50 km) shapes your landings.
- **Time acceleration** (1×–8×) because warp is long and life is short.

Controls and concepts: [docs/GAMEPLAY.md](docs/GAMEPLAY.md).

## Architecture

```
cmd/entieve → game → {render, ui} → sim → content → (core, ecs)
```

- [`sim/`](sim) is a fully deterministic fixed-timestep simulation
  (60 ticks/s) with **no rendering dependency**. Everything is driven through
  `sim.Command` values and observed through `sim.Event` values — the
  *controllability seam* that lets AI agents and tests play the game
  headlessly.
- [`content/`](content) is data-driven: ships, modules, items, NPCs, star
  systems and worlds are JSON under `content/data/`, loaded, cross-validated
  and embedded into the binary.
- [`ecs/`](ecs) is a minimal deterministic ECS; [`core/`](core) has the
  vector math and a golden-pinned SplitMix64 PRNG so twin runs are bit-identical.
- [`render/`](render), [`ui/`](ui), [`game/`](game) are the Ebitengine client.

Design contract: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Tooling (humans and AI agents alike)

```sh
go run ./cmd/entieve validate                 # lint all content, cross-check refs
go run ./cmd/entieve describe mod-emp-turret  # introspect any content id (JSON)
go run ./cmd/entieve simulate \
    --world world-showcase --ticks 36000 \
    --script scripts/showcase.json            # headless run + report
go run ./cmd/entieve run --world world-default
```

`simulate` executes tick-scheduled command scripts — JSON steps like
`{"at_tick": 300, "action": "warp", "target": "Cel Solace Belt"}` — and prints
every event plus a final state report (wallet, grid, state hash). Adding
content without touching engine code is the intended workflow:
[docs/CONTENT_AUTHORING.md](docs/CONTENT_AUTHORING.md) is the cookbook,
including "how to add a star system in 5 minutes".

## Worlds

- `world-default` — the normal game start at Solace.
- `world-showcase` — the test world: exercises **every** feature in the game
  (see its `scenario` checklist) and powers the integration suite in
  [`tests/`](tests).

## Development

```sh
go test ./...        # unit + integration tests, headless
go vet ./...
gofmt -l .
```

Testing policy: heavy stdlib `testing` coverage per package; `sim` is
deterministic (twin-run hash equality is asserted); `content` always loads and
validates the real data; `tests/` proves the showcase world exercises every
feature end-to-end.

License: MIT.
