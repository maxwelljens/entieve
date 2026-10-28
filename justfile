# Entieve — task runner. Run `just` for this list.
#
#   just run                 play the game
#   just check               the full pre-release gate
#   just release 0.5.0       check → changelog → tag → push → GitHub release
#
# Recipe flags pass through to the game: just run --load save.json --seed 7

set shell := ["sh", "-eu", "-c"]

# Version stamped into the binary (git describe, no leading v).
version := `git describe --tags --always --dirty 2>/dev/null | sed 's/^v//'`

# List the recipes (default).
default:
    @just --list

# ---- play ---------------------------------------------------------------

# Play the game (flags pass through: --world, --seed, --load, --save).
run *ARGS:
    go run ./cmd/entieve run {{ ARGS }}

# ---- verify ---------------------------------------------------------------

# Run the whole test suite (headless, no GPU needed).
test:
    go test ./...

# Run the test suite under the race detector.
test-race:
    go test -race ./...

# Test with coverage → coverage.out + coverage.html.
cover:
    go test -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html
    @echo "coverage.html"

# Lint (the gate): go vet.
lint:
    go vet ./...

# Lint deeply with golangci-lint (informational: known pre-existing findings).
lint-deep:
    golangci-lint run ./...

# Rewrite Go sources in place (gofmt).
fmt:
    gofmt -w .

# Fail if anything is unformatted.
fmt-check:
    #!/usr/bin/env bash
    set -euo pipefail
    out=$(gofmt -l .)
    if [ -n "$out" ]; then
        echo "$out"
        echo "just fmt-check: unformatted files above" >&2
        exit 1
    fi

# Render the gated capture PNGs — every capture test in its own process (one
# ebiten loop per process) → $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
# `just capture hud` runs the captures whose test name matches. New captures
# must be added to scripts/captures.sh.
capture *ARGS:
    scripts/captures.sh {{ ARGS }}

# Tidy and verify the module files.
tidy:
    go mod tidy
    go mod verify

# The gate a release must pass: format, lint, test, content validate.
check: fmt-check lint test validate

# ---- content & simulation ---------------------------------------------------

# Lint and cross-check all game content.
validate:
    go run ./cmd/entieve validate

# Run the simulation headlessly (flags pass through: --world, --ticks, --script).
simulate *ARGS:
    go run ./cmd/entieve simulate {{ ARGS }}

# Print one content definition as JSON: just describe mod-emp-turret
describe id:
    go run ./cmd/entieve describe {{ id }}

# ---- build ---------------------------------------------------------------

# Build the game for this machine → bin/entieve, version-stamped.
build:
    mkdir -p bin
    go build -trimpath -ldflags "-s -w -X main.version={{ version }}" -o bin/entieve ./cmd/entieve
    @echo "bin/entieve ({{ version }})"

# Build every release target locally → dist/ (same config the release uses).
dist:
    goreleaser build --snapshot --clean

# Extract one version's CHANGELOG section (optional output path).
release-notes ver output="dist/release-notes.md":
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p "$(dirname "{{ output }}")"
    awk -v ver="{{ ver }}" '
        index($0, "## [" ver "]") == 1 { grab = 1; next }
        grab && /^## \[/ { exit }
        grab { print }
    ' CHANGELOG.md | sed -e '/./,$!d' > "{{ output }}"
    if ! grep -q . "{{ output }}"; then
        echo "just release-notes: no section '## [{{ ver }}]' in CHANGELOG.md" >&2
        exit 1
    fi
    echo "{{ output }}"

# Cut a release: check → changelog → tag → push → publish GitHub release.
release ver:
    #!/usr/bin/env bash
    set -euo pipefail
    version="{{ ver }}"
    case "$version" in
        v*) echo "just release: version without the v prefix (got $version)" >&2; exit 2 ;;
    esac
    tag="v$version"
    date=$(date +%F)

    if [ -n "$(git status --porcelain)" ]; then
        echo "just release: working tree is dirty" >&2; exit 2
    fi
    if git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
        echo "just release: tag $tag already exists" >&2; exit 2
    fi

    just check

    # CHANGELOG.md: fold [Unreleased] into the new version heading, then fix
    # the compare links at the foot of the file.
    prev=$(git describe --tags --abbrev=0)
    awk -v ver="$version" -v date="$date" '
        { print }
        $0 == "## [Unreleased]" { print ""; print "## [" ver "] - " date }
    ' CHANGELOG.md > CHANGELOG.md.new
    mv CHANGELOG.md.new CHANGELOG.md
    awk -v ver="$version" -v prev="$prev" '
        $0 ~ /^\[Unreleased\]:/ {
            print "[Unreleased]: https://github.com/maxwelljens/entieve/compare/v" ver "...HEAD"
            print "[" ver "]: https://github.com/maxwelljens/entieve/compare/" prev "...v" ver
            next
        }
        { print }
    ' CHANGELOG.md > CHANGELOG.md.new
    mv CHANGELOG.md.new CHANGELOG.md

    git add CHANGELOG.md
    git commit -m "release: $tag"
    git tag -a "$tag" -m "$tag"

    # Push to origin (Codeberg) and to GitHub (the release home).
    git push origin master --follow-tags
    if ! git remote get-url github >/dev/null 2>&1; then
        git remote add github https://github.com/maxwelljens/entieve.git
    fi
    git push github master --tags

    # Build all platforms and publish the GitHub release.
    notes=$(mktemp "${TMPDIR:-/tmp}/entieve-release-notes.XXXXXX.md")
    just release-notes "$version" "$notes"
    GH_TOKEN=$(gh auth token) goreleaser release --clean --release-notes "$notes"

    # Belt-and-braces: goreleaser has silently dropped --release-notes before
    # (changelog.disable short-circuits the reader pipe — v0.5.0..v0.9.0 all
    # published empty bodies). Verify what landed and repair from the folded
    # notes if the body came out empty.
    body_len=$(gh release view "$tag" --repo maxwelljens/entieve --json body -q '.body|length' 2>/dev/null | tr -cd '0-9' || true)
    if [ "${body_len:-0}" -le 1 ]; then
        echo "just release: published body is empty; repairing from $notes" >&2
        gh release edit "$tag" --repo maxwelljens/entieve --notes-file "$notes"
    fi

# Remove build artifacts.
clean:
    rm -rf bin dist coverage.out coverage.html
