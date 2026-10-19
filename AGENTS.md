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
| `just validate` | lint and cross-check all game content |
| `just simulate …` | headless sim + state report (tick-scheduled `--script` steps) |
| `just describe <id>` | any content id as JSON (near-miss suggestions on typos) |
| `just build` | local version-stamped binary → `bin/entieve` |
| `just release <ver>` | gate → changelog fold → tag → push → GitHub release |

Keep the tree passing `just check` before handing work over. `just lint-deep`
runs golangci-lint (informational: known pre-existing findings) — the gate is
go vet + gofmt only.

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
