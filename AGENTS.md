# AGENTS.md — working in this repo

Entieve is a single-player 2D space sandbox in Go (Ebitengine). Domain and
module vocabulary is binding in [CONTEXT.md](CONTEXT.md); deep docs live in
[`docs/`](docs) (ARCHITECTURE, GAMEPLAY, SIM, CONTENT_AUTHORING, ART).

## Commands

Everything runs through the [justfile](justfile) — `just` lists the recipes.

| Command | What |
|---|---|
| `just run` | play the game (Ebitengine window; flags pass through) |
| `just check` | the gate: fmt-check, lint (go vet), test, content validate |
| `just test` | the whole test suite (headless, no GPU needed) |
| `just capture [name]` | render the gated capture PNGs (one ebiten loop per process) |
| `just validate` | lint and cross-check all game content |
| `just simulate …` | headless sim + state report (tick-scheduled `--script` steps, `--trace-npc NAME` decision trace) |
| `just describe <id>` | any content id as JSON (near-miss suggestions on typos) |
| `just build` | local version-stamped binary → `bin/entieve` |
| `just release <ver>` | gate → changelog fold → tag → push → GitHub release |

Keep the tree passing `just check` before handing work over. `just lint-deep`
runs golangci-lint (informational: known pre-existing findings) — the gate is
go vet + gofmt only. Beyond the table: `just test-race` / `just cover`,
`just lint` / `just fmt` / `just tidy`. Raw equivalents work too
(`go run ./cmd/entieve run|validate|describe|simulate`); `just build` stamps
the version from `git describe` into `bin/entieve`.

`just simulate` executes tick-scheduled command scripts — JSON steps like
`{"at_tick": 300, "action": "warp", "target": "Asteroid Belt I"}` — and
prints every event plus a final state report (wallet, grid, state hash).

## Layout

```
cmd/entieve → game → {render, uikit} → sim → content → (core, ecs)
```

- `sim/` — fully deterministic fixed-timestep simulation (60 ticks/s), no
  rendering dependency; driven by `sim.Command`, observed via `sim.Event`
  (the same interface lets tests and scripts play headlessly).
- `content/` — data-driven: ships, modules, items, NPCs, recipes, systems and
  worlds are JSON under `content/data/`, loaded, cross-validated, embedded.
- `ecs/` — minimal deterministic ECS; `core/` — vector math and the
  golden-pinned SplitMix64 PRNG (twin runs bit-identical).
- `render/`, `uikit/`, `game/` — the Ebitengine client.

Design contract: `docs/ARCHITECTURE.md`; simulation reference: `docs/SIM.md`
(tick anatomy, world metabolism, ledger-agent economy, price formation).

## Worlds

- `world-default` — the normal game start at Struvok.
- `world-showcase` — the test world: exercises **every** feature (its
  `scenario` checklist) and powers the integration suite in `tests/`.

## Testing policy

Heavy stdlib `testing` coverage per package; `sim` asserts twin-run hash
equality; `content` always loads and validates the real data; `tests/`
proves the showcase world exercises every feature end-to-end (the tour is
4 scenarios). Capture tests are gated (`ENTIEVE_CAPTURE=1`, own process
each — `just capture` runs the protocol).

## Art and type licensing

- **Project licence: European Union Public Licence 1.2** (LICENSE, © 2026
  The Entieve Authors). Third-party dependencies keep their own licences
  (ebitenui and sahilm/fuzzy are MIT) — that is what the credits line says.
- **IBM Plex Sans** is embedded from `render/fonts/` alongside its licence:
  SIL Open Font License 1.1 (`render/fonts/IBM_Plex_Sans/OFL.txt`, plus the
  per-family `IBMPlexSans_LICENCE.txt` notices).
- **Icons** are game-icons.net silhouettes (CC BY 3.0) in `uikit/icons/`,
  tinted from the palette. Every icon keeps its author credit in
  `uikit/icons/CREDITS.md`; a test pins each PNG to a manifest entry and a
  credits line. Required wording: *Icons made by {author}. Available on
  https://game-icons.net*.
- Drop-in art (`assets/**/*.png`, file name = content id) is documented in
  `docs/ART.md` and is gitignored — force-add to track.

## Conventions

- Ids (`sys-*`, `mod-*`, `ship-*`, …) are frozen artifacts; display names
  follow `docs/lore/NAMING.md`. New concepts get named in `CONTEXT.md`
  before they get built.
- `CHANGELOG.md` is Keep a Changelog: record user-visible changes under
  `[Unreleased]`; `just release` folds the section into the version heading —
  never edit old version sections by hand.
- `sim` stays deterministic: twin-run hash equality is asserted in tests.
- Content changes live in `content/data` and must pass `just validate`;
  adding content without touching engine code is the intended workflow
  (`docs/CONTENT_AUTHORING.md`).
- User-dropped art under `assets/**/*.png` is local material: gitignored,
  force-add to track.

## Releases

`just release <version>` is the only release path: it gates on `just check`,
folds the changelog, tags `vx.y.z`, pushes to Codeberg (origin) and GitHub,
then GoReleaser (`.goreleaser.yaml`) cross-compiles linux/windows/darwin on
amd64 + arm64 (cgo-free) and publishes the GitHub release with the CHANGELOG
section as notes. `just dist` builds every target into `dist/` without
publishing. Don't hand-edit `.goreleaser.yaml` release wiring (owner/repo:
`maxwelljens/entieve`) without checking where releases actually go.
