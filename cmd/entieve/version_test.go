package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// RED: source runs (just run, go run) pass no ldflags, so every version
// door - the title screen's caption and `entieve version` - reported the
// "0.1.0-dev" sentinel instead of the actual version. Release binaries
// and `just build` were the only stamped paths.

func TestResolveVersionPrefersStamp(t *testing.T) {
	got := resolveVersion("0.12.0", func() (string, error) {
		t.Error("describe must not run when a build stamp exists")
		return "", nil
	})
	if got != "0.12.0" {
		t.Errorf("got %q, want the stamp", got)
	}
}

func TestResolveVersionDescribesSourceRun(t *testing.T) {
	got := resolveVersion(versionDev, func() (string, error) {
		return "0.11.0-14-ge1925cf", nil
	})
	if got != "0.11.0-14-ge1925cf" {
		t.Errorf("got %q, want git's describe", got)
	}
}

func TestResolveVersionKeepsSentinelWithoutGit(t *testing.T) {
	got := resolveVersion(versionDev, func() (string, error) {
		return "", errors.New("no git")
	})
	if got != versionDev {
		t.Errorf("got %q, want the dev sentinel", got)
	}
}

// The real seam must answer in the build-stamp form: no leading v
// (versionLine puts it back), no whitespace, non-empty.
func TestGitDescribeMatchesStampForm(t *testing.T) {
	v, err := gitDescribe()
	if err != nil {
		t.Skipf("no git checkout: %v", err)
	}
	if v == "" || strings.HasPrefix(v, "v") || strings.ContainsAny(v, " \n\t") {
		t.Errorf("describe %q is not in stamp form", v)
	}
}

// The stamped var still reaches the CLI door (and, wired the same way in
// cmdRun, the title caption): the var must stay constant-initialized for
// -ldflags -X to bind it.
func TestVersionCommandReportsStamp(t *testing.T) {
	old := version
	version = "9.9.9-test"
	defer func() { version = old }()

	var out, errOut bytes.Buffer
	if code := run([]string{"version"}, &out, &errOut); code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
	}
	if want := "entieve 9.9.9-test\n"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}
