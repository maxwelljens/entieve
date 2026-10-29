package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/script"
	"github.com/entieve/entieve/sim"
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
		"mod-autocannon-s": `"tracking"`,
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
	code := run([]string{"describe", "autocannon"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "did you mean") || !strings.Contains(errOut.String(), "mod-autocannon-s") {
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

// TestDescribeContract: contract-* ids resolve to their full definition —
// id, kind, level, station, objective, reward and standing requirement.
func TestDescribeContract(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"describe", "contract-solace-cleanup-1"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("describe contract: exit = %d, stderr = %q", code, errOut.String())
	}
	var def map[string]any
	if err := json.Unmarshal(out.Bytes(), &def); err != nil {
		t.Fatalf("describe contract: invalid JSON: %v", err)
	}
	for _, key := range []string{"id", "kind", "level", "station", "objective", "reward", "min_standing"} {
		if _, ok := def[key]; !ok {
			t.Errorf("describe contract: missing %q in %s", key, out.String())
		}
	}
	if got, want := def["id"], "contract-solace-cleanup-1"; got != want {
		t.Errorf("id = %v, want %v", got, want)
	}
	if got, want := def["kind"], "kill"; got != want {
		t.Errorf("kind = %v, want %v", got, want)
	}
	// Near-miss suggestions cover contracts too.
	errOut.Reset()
	if code := run([]string{"describe", "contract-solace"}, &out, &errOut); code != 1 {
		t.Fatalf("partial contract id: exit = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "contract-solace-cleanup-1") {
		t.Errorf("stderr %q missing contract suggestions", errOut.String())
	}
}

// TestValidateCountsContracts: the summary counts contracts alongside the
// other content kinds.
func TestValidateCountsContracts(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"validate"}, &out, &errOut); code != 0 {
		t.Fatalf("validate exit = %d, stderr = %q", code, errOut.String())
	}
	want := fmt.Sprintf("contracts: %d", len(content.MustLoad().Contracts))
	if !strings.Contains(out.String(), want) {
		t.Errorf("output %q missing %q", out.String(), want)
	}
}

// TestReportStandingAndMissions: the simulate report prints standing and
// the active/completed missions next to the wallet.
func TestReportStandingAndMissions(t *testing.T) {
	c := content.MustLoad()
	wd, ok := c.World("world-showcase")
	if !ok {
		t.Fatal("world-showcase missing")
	}
	s, err := sim.New(c, wd, 42)
	if err != nil {
		t.Fatalf("sim.New: %v", err)
	}
	// Dock, accept a contract, then raise standing (save surgery).
	s.Docked.Set(s.Player(), sim.Docked{At: "cel-haven-rest"})
	s.Queue(sim.CmdAcceptMission{ID: "contract-solace-cordite-1"})
	s.Step()
	sv, err := s.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	sv.Standing = 0.25
	if err := s.Restore(sv); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	res := &script.Result{Events: []sim.Event{{
		Kind: sim.EvMission, Phase: sim.MissionCompleted, ID: "contract-solace-cordite-1",
	}}}

	var buf bytes.Buffer
	printReport(&buf, s, res, 60)
	report := buf.String()
	for _, want := range []string{
		"standing: 0.25",
		"missions: 1 active, 1 completed",
		"  active contract-solace-cordite-1 0/400",
		"  completed contract-solace-cordite-1",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q:\n%s", want, report)
		}
	}
	// The standing/mission block sits next to the wallet.
	if strings.Index(report, "standing:") < strings.Index(report, "wallet:") ||
		strings.Index(report, "standing:") > strings.Index(report, "state hash:") {
		t.Errorf("standing not printed next to the wallet:\n%s", report)
	}
}

// TestSimulateReportWiring: the simulate subcommand prints the report
// with standing and missions.
func TestSimulateReportWiring(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"simulate", "--ticks", "10", "--quiet"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("simulate exit = %d, stderr = %q", code, errOut.String())
	}
	for _, want := range []string{"== simulation report ==", "standing:", "missions:"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report missing %q:\n%s", want, out.String())
		}
	}
}
