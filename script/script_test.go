package script

import (
	"fmt"
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
)

// fakeEngine is a minimal Engine for script-layer tests (no real sim needed).
type fakeEngine struct {
	grid   []sim.Info
	queued []sim.Command
	events []sim.Event
	ticks  int64
}

func (f *fakeEngine) Queue(c sim.Command) { f.queued = append(f.queued, c) }
func (f *fakeEngine) Step()               { f.ticks++ }
func (f *fakeEngine) Events() []sim.Event { e := f.events; f.events = nil; return e }
func (f *fakeEngine) Grid() []sim.Info    { return f.grid }

func TestParseValid(t *testing.T) {
	data := []byte(`[
		{"at_tick": 300, "action": "warp", "target": "Cel Solace Belt", "note": "go"},
		{"at_tick": 0, "action": "lock", "target": "Rat Scuttler"},
		{"at_tick": 60, "action": "activate", "slot": "high0", "target": "Rat Scuttler"},
		{"at_tick": 90, "action": "buy", "item": "mod-emp-turret", "qty": 1},
		{"at_tick": 120, "action": "orbit", "target": "Rat Scuttler", "range": 5000},
		{"at_tick": 150, "action": "repair"}
	]`)
	steps, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(steps) != 6 {
		t.Fatalf("got %d steps, want 6", len(steps))
	}
	// Sorted by at_tick ascending.
	for i := 1; i < len(steps); i++ {
		if steps[i-1].AtTick > steps[i].AtTick {
			t.Fatalf("steps not sorted: %v", steps)
		}
	}
	if steps[0].Action != "lock" {
		t.Fatalf("first step = %q, want lock (lowest at_tick)", steps[0].Action)
	}
}

func TestParseWrappedObject(t *testing.T) {
	data := []byte(`{"steps": [{"at_tick": 1, "action": "stop"}]}`)
	steps, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(steps) != 1 || steps[0].Action != "stop" {
		t.Fatalf("unexpected: %+v", steps)
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name, data, want string
	}{
		{"garbage", `not json`, "parse"},
		{"unknown action", `[{"at_tick":0,"action":"teleport"}]`, "unknown action"},
		{"negative tick", `[{"at_tick":-1,"action":"stop"}]`, "at_tick"},
		{"missing target", `[{"at_tick":0,"action":"warp"}]`, "target name required"},
		{"missing item", `[{"at_tick":0,"action":"buy","qty":1}]`, "item id required"},
		{"bad qty", `[{"at_tick":0,"action":"buy","item":"x","qty":0}]`, "qty"},
		{"bad slot", `[{"at_tick":0,"action":"activate","slot":"rig0"}]`, "slot"},
		{"missing slot index", `[{"at_tick":0,"action":"activate","slot":"high"}]`, "missing index"},
		{"missing ship", `[{"at_tick":0,"action":"assemble"}]`, "ship id required"},
		{"missing orbit range", `[{"at_tick":0,"action":"orbit","target":"x"}]`, "range"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.data))
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tc.want)
			}
			if !contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestParseSlot(t *testing.T) {
	cases := []struct {
		in      string
		want    sim.SlotRef
		wantErr bool
	}{
		{"high0", sim.SlotRef{Class: content.SlotHigh, Index: 0}, false},
		{"mid12", sim.SlotRef{Class: content.SlotMid, Index: 12}, false},
		{"low3", sim.SlotRef{Class: content.SlotLow, Index: 3}, false},
		{"high", sim.SlotRef{}, true},
		{"highx", sim.SlotRef{}, true},
		{"rig0", sim.SlotRef{}, true},
		{"", sim.SlotRef{}, true},
	}
	for _, tc := range cases {
		got, err := ParseSlot(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseSlot(%q): want error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseSlot(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseSlot(%q) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestRunQueuesAndSteps(t *testing.T) {
	f := &fakeEngine{grid: []sim.Info{{Name: "Rat Scuttler", Entity: 7}}}
	f.events = []sim.Event{{Kind: sim.EvLog, Text: "hello"}}
	steps := []Step{
		{AtTick: 0, Action: "lock", Target: "Rat Scuttler"},
		{AtTick: 1, Action: "stop"},
		{AtTick: 1, Action: "warp", Target: "Missing Thing"},
	}
	var seen []string
	res := Run(f, steps, 3, func(ev sim.Event) { seen = append(seen, ev.Text) })
	if f.ticks != 3 {
		t.Fatalf("ticks = %d, want 3", f.ticks)
	}
	if res.Ran != 2 {
		t.Fatalf("Ran = %d, want 2", res.Ran)
	}
	if len(res.Unresolved) != 1 || res.Unresolved[0].Target != "Missing Thing" {
		t.Fatalf("Unresolved = %+v", res.Unresolved)
	}
	if len(f.queued) != 2 {
		t.Fatalf("queued %d commands, want 2", len(f.queued))
	}
	if _, ok := f.queued[0].(sim.CmdLock); !ok {
		t.Fatalf("queued[0] = %T, want CmdLock", f.queued[0])
	}
	if len(seen) != 1 || seen[0] != "hello" {
		t.Fatalf("seen events = %v", seen)
	}
	// A step scheduled past the run horizon is never attempted: add one and
	// confirm it does not show up as unresolved.
	later := []Step{{AtTick: 99, Action: "warp", Target: "Missing Thing"}}
	res2 := Run(&fakeEngine{}, later, 3, nil)
	if len(res2.Unresolved) != 0 {
		t.Fatalf("out-of-horizon step must not be reported: %+v", res2.Unresolved)
	}
}

func TestRunCommandMapping(t *testing.T) {
	f := &fakeEngine{grid: []sim.Info{
		{Name: "Haven's Rest", Entity: 3},
		{Name: "Cel Solace Belt", Entity: 4},
		{Name: "Wreck", Entity: 5},
		{Name: "Rock", Entity: 6},
		{Name: "Gate", Entity: 7},
		{Name: "Rat", Entity: 8},
	}}
	steps := []Step{
		{AtTick: 0, Action: "approach", Target: "Rat"},
		{AtTick: 0, Action: "orbit", Target: "Rat", Range: 2500},
		{AtTick: 0, Action: "align", Target: "Gate"},
		{AtTick: 0, Action: "unlock", Target: "Rat"},
		{AtTick: 0, Action: "activate", Slot: "high1", Target: "Rat"},
		{AtTick: 0, Action: "deactivate", Slot: "mid0"},
		{AtTick: 0, Action: "loot", Target: "Wreck"},
		{AtTick: 0, Action: "mine", Target: "Rock", Slot: "high1"},
		{AtTick: 0, Action: "dock", Target: "Haven's Rest"},
		{AtTick: 0, Action: "jump", Target: "Gate"},
		{AtTick: 0, Action: "sell", Item: "min-ferrite", Qty: 5},
		{AtTick: 0, Action: "refine", Item: "ore-sterngrit", Qty: 10},
		{AtTick: 0, Action: "repair"},
		{AtTick: 0, Action: "assemble", Ship: "ship-barge"},
		{AtTick: 0, Action: "fit", Item: "mod-mining-laser", Slot: "high0"},
		{AtTick: 0, Action: "unfit", Slot: "low1"},
		{AtTick: 0, Action: "manual", DirX: 1, DirY: 0, Speed: 0.5},
	}
	res := Run(f, steps, 1, nil)
	if len(res.Unresolved) != 0 {
		t.Fatalf("unresolved: %+v", res.Unresolved)
	}
	if res.Ran != len(steps) {
		t.Fatalf("Ran = %d, want %d", res.Ran, len(steps))
	}
	wantTypes := []any{
		sim.CmdApproach{}, sim.CmdOrbit{}, sim.CmdAlignTo{}, sim.CmdUnlock{},
		sim.CmdActivate{}, sim.CmdDeactivate{}, sim.CmdLoot{}, sim.CmdMine{},
		sim.CmdDock{}, sim.CmdJump{}, sim.CmdSell{}, sim.CmdRefine{},
		sim.CmdRepair{}, sim.CmdAssemble{}, sim.CmdFitModule{}, sim.CmdUnfitModule{},
		sim.CmdManualVel{},
	}
	if len(f.queued) != len(wantTypes) {
		t.Fatalf("queued %d, want %d", len(f.queued), len(wantTypes))
	}
	for i, want := range wantTypes {
		gotType := fmt.Sprintf("%T", f.queued[i])
		wantType := fmt.Sprintf("%T", want)
		if gotType != wantType {
			t.Errorf("queued[%d] = %s, want %s", i, gotType, wantType)
		}
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
