// Package script runs tick-scheduled command scripts against a sim.Engine.
//
// Scripts are the AI-agent controllability layer: a JSON file of named steps
// ("at tick 300, warp to 'Cel Solace Belt'") drives the simulation without
// any UI. Both the `entieve simulate` CLI and the headless showcase
// integration tests are built on this package.
//
// Target entities are addressed by display name (sim.Info.Name) and resolved
// lazily at execution time, so entities that appear later (wrecks, respawns)
// can be referenced. Unresolvable steps are reported, never fatal.
package script

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/sim"
)

// Engine is the simulation surface a script needs. *sim.Sim implements it;
// tests may substitute a fake.
type Engine interface {
	Queue(cmd sim.Command)
	Step()
	Events() []sim.Event
	Grid() []sim.Info
}

// Step is one scheduled command. JSON field names are the public script DSL.
type Step struct {
	AtTick int64   `json:"at_tick"`
	Action string  `json:"action"`
	Target string  `json:"target,omitempty"` // entity display name
	Item   string  `json:"item,omitempty"`   // item/ship/module ID
	Qty    int     `json:"qty,omitempty"`
	Slot   string  `json:"slot,omitempty"`  // "high0", "mid1", "low2"...
	Ship   string  `json:"ship,omitempty"`  // ship ID (assemble)
	From   string  `json:"from,omitempty"`  // transfer source: "cargo"|"ore"|"hangar"
	To     string  `json:"to,omitempty"`    // transfer destination: "cargo"|"ore"|"hangar"
	Range  float64 `json:"range,omitempty"` // orbit range, m
	DirX   float64 `json:"dir_x,omitempty"`
	DirY   float64 `json:"dir_y,omitempty"`
	Speed  float64 `json:"speed,omitempty"` // manual throttle 0..1
	Note   string  `json:"note,omitempty"`  // ignored; documentation for humans
}

// Valid action names.
var validActions = map[string]bool{
	"stop": true, "manual": true, "approach": true, "orbit": true,
	"warp": true, "align": true, "lock": true, "unlock": true,
	"activate": true, "deactivate": true, "loot": true, "mine": true,
	"dock": true, "undock": true, "jump": true,
	"buy": true, "sell": true, "refine": true, "repair": true,
	"assemble": true, "fit": true, "unfit": true, "transfer": true,
	"setammo": true,
}

// Parse decodes a JSON step array (an object with a "steps" array is also
// accepted) and validates its DSL.
func Parse(data []byte) ([]Step, error) {
	var steps []Step
	if err := json.Unmarshal(data, &steps); err != nil {
		var wrap struct {
			Steps []Step `json:"steps"`
		}
		if err2 := json.Unmarshal(data, &wrap); err2 != nil {
			return nil, fmt.Errorf("script: parse: %w", err)
		}
		steps = wrap.Steps
	}
	for i, st := range steps {
		if st.AtTick < 0 {
			return nil, fmt.Errorf("script: step %d: at_tick must be >= 0", i)
		}
		if !validActions[st.Action] {
			return nil, fmt.Errorf("script: step %d: unknown action %q", i, st.Action)
		}
		if err := st.validate(i); err != nil {
			return nil, err
		}
	}
	sort.SliceStable(steps, func(a, b int) bool { return steps[a].AtTick < steps[b].AtTick })
	return steps, nil
}

// needsTarget actions require a resolvable entity name.
var needsTarget = map[string]bool{
	"approach": true, "orbit": true, "warp": true, "align": true,
	"lock": true, "unlock": true, "loot": true, "mine": true,
	"dock": true, "jump": true,
}

// needsItem actions require an item/ship/module ID.
var needsItem = map[string]bool{
	"buy": true, "sell": true, "refine": true, "fit": true, "transfer": true,
}

func (st Step) validate(i int) error {
	need := func(cond bool, msg string) error {
		if cond {
			return fmt.Errorf("script: step %d (%s): %s", i, st.Action, msg)
		}
		return nil
	}
	if err := need(needsTarget[st.Action] && st.Target == "", "target name required"); err != nil {
		return err
	}
	if err := need(needsItem[st.Action] && st.Item == "", "item id required"); err != nil {
		return err
	}
	switch st.Action {
	case "activate", "deactivate":
		if _, err := ParseSlot(st.Slot); err != nil {
			return fmt.Errorf("script: step %d (%s): %v", i, st.Action, err)
		}
	case "setammo":
		if _, err := ParseSlot(st.Slot); err != nil {
			return fmt.Errorf("script: step %d (setammo): %v", i, err)
		}
		// Item may be empty: that means "auto" (first charge in cargo order).
	case "mine":
		if _, err := ParseSlot(st.Slot); err != nil {
			return fmt.Errorf("script: step %d (%s): %v", i, st.Action, err)
		}
	case "fit":
		if _, err := ParseSlot(st.Slot); err != nil {
			return fmt.Errorf("script: step %d (%s): %v", i, st.Action, err)
		}
	case "unfit":
		if _, err := ParseSlot(st.Slot); err != nil {
			return fmt.Errorf("script: step %d (%s): %v", i, st.Action, err)
		}
	case "assemble":
		if err := need(st.Ship == "", "ship id required"); err != nil {
			return err
		}
	case "buy", "sell", "refine":
		if err := need(st.Qty < 1, "qty must be >= 1"); err != nil {
			return err
		}
	case "transfer":
		if err := need(st.Qty < 1, "qty must be >= 1"); err != nil {
			return err
		}
		for _, side := range []string{st.From, st.To} {
			if side != "cargo" && side != "ore" && side != "hangar" {
				return fmt.Errorf("script: step %d (transfer): from/to must be cargo|ore|hangar", i)
			}
		}
		if st.From == st.To {
			return fmt.Errorf("script: step %d (transfer): from and to must differ", i)
		}
	case "orbit":
		if err := need(st.Range <= 0, "range must be > 0"); err != nil {
			return err
		}
	}
	return nil
}

// ParseSlot decodes "high0"/"mid1"/"low2" style slot addresses.
func ParseSlot(s string) (sim.SlotRef, error) {
	for _, class := range []string{content.SlotHigh, content.SlotMid, content.SlotLow} {
		if strings.HasPrefix(s, class) {
			n := 0
			digits := s[len(class):]
			if digits == "" {
				return sim.SlotRef{}, fmt.Errorf("slot %q: missing index", s)
			}
			for _, d := range digits {
				if d < '0' || d > '9' {
					return sim.SlotRef{}, fmt.Errorf("slot %q: bad index", s)
				}
				n = n*10 + int(d-'0')
			}
			return sim.SlotRef{Class: class, Index: n}, nil
		}
	}
	return sim.SlotRef{}, fmt.Errorf("slot %q: want highN/midN/lowN", s)
}

// Result reports what a run did.
type Result struct {
	Events     []sim.Event
	Unresolved []Step
	Ran        int // steps successfully queued
}

// Hooks are optional observers for RunH.
type Hooks struct {
	OnEvent func(sim.Event)
	OnTick  func(tick int64, eng Engine) // called after each Step
}

// RunH is Run with observer hooks (used by tests to probe state).
func RunH(eng Engine, steps []Step, ticks int64, hooks Hooks) *Result {
	res := &Result{}
	byTick := map[int64][]Step{}
	var keys []int64
	for _, st := range steps {
		if _, ok := byTick[st.AtTick]; !ok {
			keys = append(keys, st.AtTick)
		}
		byTick[st.AtTick] = append(byTick[st.AtTick], st)
	}
	sort.Slice(keys, func(a, b int) bool { return keys[a] < keys[b] })
	for tick := int64(0); tick < ticks; tick++ {
		for _, st := range byTick[tick] {
			cmd, ok := st.command(eng)
			if !ok {
				res.Unresolved = append(res.Unresolved, st)
				continue
			}
			eng.Queue(cmd)
			res.Ran++
		}
		eng.Step()
		for _, ev := range eng.Events() {
			res.Events = append(res.Events, ev)
			if hooks.OnEvent != nil {
				hooks.OnEvent(ev)
			}
		}
		if hooks.OnTick != nil {
			hooks.OnTick(tick, eng)
		}
	}
	return res
}

// command converts a step into a sim.Command, resolving target names against
// the live grid. Returns ok=false when the target cannot be resolved.
func (st Step) command(eng Engine) (sim.Command, bool) {
	need := func() (sim.Info, bool) {
		for _, in := range eng.Grid() {
			if in.Name == st.Target {
				return in, true
			}
		}
		return sim.Info{}, false
	}
	switch st.Action {
	case "stop":
		return sim.CmdStop{}, true
	case "manual":
		return sim.CmdManualVel{Dir: core.V(st.DirX, st.DirY), SpeedFrac: st.Speed}, true
	case "deactivate":
		slot, _ := ParseSlot(st.Slot)
		return sim.CmdDeactivate{Slot: slot}, true
	case "fit":
		slot, _ := ParseSlot(st.Slot)
		return sim.CmdFitModule{Item: content.ModuleID(st.Item), Slot: slot}, true
	case "unfit":
		slot, _ := ParseSlot(st.Slot)
		return sim.CmdUnfitModule{Slot: slot}, true
	case "buy":
		return sim.CmdBuy{Item: content.ItemID(st.Item), Qty: st.Qty}, true
	case "transfer":
		return sim.CmdTransfer{Item: content.ItemID(st.Item), Qty: st.Qty, From: st.From, To: st.To}, true
	case "setammo":
		slot, _ := ParseSlot(st.Slot)
		return sim.CmdSetAmmo{Slot: slot, Item: content.ItemID(st.Item)}, true
	case "sell":
		return sim.CmdSell{Item: content.ItemID(st.Item), Qty: st.Qty}, true
	case "refine":
		return sim.CmdRefine{Item: content.ItemID(st.Item), Qty: st.Qty}, true
	case "repair":
		return sim.CmdRepair{}, true
	case "undock":
		return sim.CmdUndock{}, true
	case "assemble":
		return sim.CmdAssemble{Ship: content.ShipID(st.Ship)}, true
	case "activate":
		slot, _ := ParseSlot(st.Slot)
		if st.Target == "" {
			return sim.CmdActivate{Slot: slot}, true // self-targeted module
		}
	}
	target, ok := need()
	if !ok {
		return nil, false
	}
	switch st.Action {
	case "approach":
		return sim.CmdApproach{Target: target.Entity}, true
	case "orbit":
		return sim.CmdOrbit{Target: target.Entity, Range: st.Range}, true
	case "warp":
		return sim.CmdWarpTo{Target: target.Entity, Range: st.Range}, true
	case "align":
		return sim.CmdAlignTo{Target: target.Entity}, true
	case "lock":
		return sim.CmdLock{Target: target.Entity}, true
	case "unlock":
		return sim.CmdUnlock{Target: target.Entity}, true
	case "loot":
		return sim.CmdLoot{Wreck: target.Entity, Item: content.ItemID(st.Item)}, true
	case "mine":
		slot, _ := ParseSlot(st.Slot)
		return sim.CmdMine{Asteroid: target.Entity, Slot: slot}, true
	case "dock":
		return sim.CmdDock{Station: target.Entity}, true
	case "jump":
		return sim.CmdJump{Gate: target.Entity}, true
	case "activate":
		slot, _ := ParseSlot(st.Slot)
		return sim.CmdActivate{Slot: slot, Target: target.Entity}, true
	}
	return nil, false
}

// Run advances eng for `ticks` Steps, queueing each step at its AtTick and
// collecting every emitted event. Steps whose target name does not resolve
// at their tick land in Result.Unresolved (never fatal).
func Run(eng Engine, steps []Step, ticks int64, onEvent func(sim.Event)) *Result {
	return RunH(eng, steps, ticks, Hooks{OnEvent: onEvent})
}
