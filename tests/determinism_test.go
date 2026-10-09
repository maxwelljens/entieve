package tests

import (
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/script"
	"github.com/entieve/entieve/sim"
)

// TestDeterminismTwinRuns builds two simulations from the same content, world
// and seed, drives them with the same command script and asserts their state
// hashes stay identical at every checkpoint. This is the bedrock guarantee:
// Entieve simulations are replayable and AI-controllable.
func TestDeterminismTwinRuns(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("load content: %v", err)
	}
	wd, ok := c.World("world-showcase")
	if !ok {
		t.Fatal("world-showcase missing")
	}
	steps := loadScript(t, "scripts/showcase.json")

	const seed = 1234
	const ticks = 3600 // one minute of sim time is enough for drift to show
	a, err := sim.New(c, wd, seed)
	if err != nil {
		t.Fatalf("sim A: %v", err)
	}
	b, err := sim.New(c, wd, seed)
	if err != nil {
		t.Fatalf("sim B: %v", err)
	}
	if a.Hash() != b.Hash() {
		t.Fatalf("fresh sims with equal seed differ: %x != %x", a.Hash(), b.Hash())
	}

	runBoth := func() {
		script.Run(a, steps, ticks/2, nil)
		script.Run(b, steps, ticks/2, nil)
	}
	runBoth()
	if a.Hash() != b.Hash() {
		t.Fatalf("twin runs diverged: %x != %x", a.Hash(), b.Hash())
	}
	runBoth()
	if a.Hash() != b.Hash() {
		t.Fatalf("twin runs diverged later: %x != %x", a.Hash(), b.Hash())
	}

	// A different seed must actually play out differently.
	d, err := sim.New(c, wd, seed+1)
	if err != nil {
		t.Fatalf("sim D: %v", err)
	}
	script.Run(d, steps, ticks, nil)
	if d.Hash() == a.Hash() {
		t.Fatal("different seed produced identical state — RNG is not wired in")
	}
}

// TestDeterminismScriptIndependent verifies that runs without any scripted
// commands are also deterministic (NPC streams must not depend on wall clock
// or map ordering).
func TestDeterminismScriptIndependent(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("load content: %v", err)
	}
	wd, ok := c.World("world-showcase")
	if !ok {
		t.Fatal("world-showcase missing")
	}
	a, err := sim.New(c, wd, 7)
	if err != nil {
		t.Fatal(err)
	}
	b, err := sim.New(c, wd, 7)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6000; i++ {
		a.Step()
		b.Step()
	}
	if a.Hash() != b.Hash() {
		t.Fatalf("idle twin runs diverged: %x != %x", a.Hash(), b.Hash())
	}
}

func loadScript(t *testing.T, path string) []script.Step {
	t.Helper()
	data, err := readFile(path)
	if err != nil {
		t.Fatalf("read script %s: %v", path, err)
	}
	steps, err := script.Parse(data)
	if err != nil {
		t.Fatalf("parse script %s: %v", path, err)
	}
	return steps
}
