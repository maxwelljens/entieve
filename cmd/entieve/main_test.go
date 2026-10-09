package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"version"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "entieve") {
		t.Fatalf("output %q missing binary name", out.String())
	}
}

func TestNoArgsShowsUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run(nil, &out, &errOut)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "Usage:") {
		t.Fatalf("stderr %q missing usage", errOut.String())
	}
}

func TestHelpCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	for _, arg := range []string{"help", "-h", "--help"} {
		code := run([]string{arg}, &out, &errOut)
		if code != 0 {
			t.Fatalf("%s: exit = %d", arg, code)
		}
		if !strings.Contains(out.String(), "entieve simulate") {
			t.Fatalf("%s: usage missing simulate subcommand", arg)
		}
	}
}

func TestUnknownSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"frobnicate"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "unknown subcommand") {
		t.Fatalf("stderr %q missing error", errOut.String())
	}
}

func TestValidateEmbeddedContent(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"validate"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("validate exit = %d\nstderr:\n%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "content OK") {
		t.Fatalf("output %q missing summary", out.String())
	}
	for _, want := range []string{"systems", "worlds", "ships", "modules"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("summary %q missing %s count", out.String(), want)
		}
	}
}

func TestValidateMissingDir(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"validate", "--dir", "/nonexistent/content"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
}

func TestDescribeKnownIDs(t *testing.T) {
	cases := map[string]string{
		"ship-sloop":       `"class"`,
		"mod-emp-turret":   `"tracking"`,
		"min-ferrite":      `"category"`,
		"npc-rat-scuttler": `"aggro_range"`,
		"sys-solace":       `"celestials"`,
		"world-showcase":   `"scenario"`,
	}
	for id, want := range cases {
		var out, errOut bytes.Buffer
		code := run([]string{"describe", id}, &out, &errOut)
		if code != 0 {
			t.Fatalf("describe %s: exit = %d, stderr = %q", id, code, errOut.String())
		}
		if !strings.Contains(out.String(), want) {
			t.Fatalf("describe %s: output missing %s", id, want)
		}
		// Output must be parseable JSON.
		var v map[string]any
		if err := json.Unmarshal(out.Bytes(), &v); err != nil {
			t.Fatalf("describe %s: invalid JSON: %v", id, err)
		}
	}
}

func TestDescribeUnknownSuggests(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"describe", "emp-turret"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "did you mean") || !strings.Contains(errOut.String(), "mod-emp-turret") {
		t.Fatalf("stderr %q missing suggestion", errOut.String())
	}
}

func TestDescribeWantsOneArg(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"describe"}, &out, &errOut); code != 2 {
		t.Fatalf("no arg: exit = %d, want 2", code)
	}
	if code := run([]string{"describe", "a", "b"}, &out, &errOut); code != 2 {
		t.Fatalf("two args: exit = %d, want 2", code)
	}
}
