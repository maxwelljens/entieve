package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// versionDev is the unstamped build's sentinel. The justfile and
// GoReleaser stamp real versions via -ldflags "-X main.version=...";
// a source run without the stamp falls back to git's own describe
// (resolveVersion), so this string only survives where neither the
// build nor the checkout can say what it is.
const versionDev = "0.1.0-dev"

// resolveVersion picks the version the game reports: a build-time stamp
// always wins (release binaries, `just build`); an unstamped run asks
// describe - git, on a source checkout - for the same
// `git describe --tags --always --dirty` string the build recipes
// stamp; with neither, the dev sentinel. describe is injectable so the
// unit tests never shell out.
func resolveVersion(stamped string, describe func() (string, error)) string {
	if stamped != versionDev {
		return stamped
	}
	if v, err := describe(); err == nil {
		return v
	}
	return stamped
}

// gitDescribe is the real describe seam. Output() keeps git's stderr out
// of the console; any failure (no git, no checkout, empty output) is an
// error the caller degrades on. The leading v is trimmed to match the
// justfile's stamp form exactly - versionLine puts it back for display.
func gitDescribe() (string, error) {
	out, err := exec.Command("git", "describe", "--tags", "--always", "--dirty").Output()
	if err != nil {
		return "", err
	}
	v := strings.TrimPrefix(strings.TrimSpace(string(out)), "v")
	if v == "" {
		return "", fmt.Errorf("git describe: empty output")
	}
	return v, nil
}

// versionString is what both version doors report: the `version`
// subcommand and the title screen's caption (opts.Version).
func versionString() string {
	return resolveVersion(version, gitDescribe)
}
