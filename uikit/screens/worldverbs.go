package screens

import (
	"sort"
	"strconv"
	"strings"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/widgets"
)

// World verb (CONTEXT.md): one row per in-space command. The key path
// (game.worldKey), the Selected Item buttons and the overview row actions
// all resolve, gate and emit through this table, so the three surfaces
// cannot disagree about what a verb targets or when it is offered. Fixed
// chords and session/camera keys are not verbs and stay out.
//
// Eligible says "may act at all": both surfaces check it, and the nearest
// fallback only picks entities it accepts. The key path leaves a refusal
// to the sim (GAMEPLAY's refusal lines are the feedback); Ready is the
// panel's disabled state and why text only.
type TargetRule int

const (
	TargetSelected            TargetRule = iota // the selection, or nothing
	TargetSelectedElseNearest                   // the selection when eligible, else the nearest eligible
	TargetNearest                               // the nearest eligible, whatever is selected
	TargetCycle                                 // the next lockable after the selection, distance-ordered
	TargetSelf                                  // the player / no target (module slots, stop)
)

// Resolved is one row's target after resolution. Slot and Active are the
// slot-carrying verbs': mine's fitted mining laser, high-N's slot and its
// live state; a zero Slot means the fit has none.
type Resolved struct {
	Sel    ecs.Entity
	Entity ecs.Entity
	Info   sim.Info
	Slot   sim.SlotRef
	Active bool
}

// WorldVerb is one row. Emit is pure: it builds the intent and never
// queues — the sink does that.
type WorldVerb struct {
	Action   string // the frozen worldKeyMap id
	Label    string
	Rule     TargetRule
	Eligible func(Resolved) bool
	Ready    func(Resolved) (ok bool, why string)
	Emit     func(Resolved) widgets.Intent
}

// WorldVerbs is the table, in worldKeyMap order (a slice: the cycle's
// distance sort and every surface's scan must stay deterministic).
var WorldVerbs = []WorldVerb{
	{Action: "lock-nearest", Label: "Cycle-lock nearest target", Rule: TargetCycle,
		Eligible: lockable, Emit: func(r Resolved) widgets.Intent { return widgets.Lock{Entity: r.Entity} }},
	{Action: "lock-selected", Label: "Lock selected target", Rule: TargetSelected,
		Eligible: lockable, Emit: func(r Resolved) widgets.Intent { return widgets.Lock{Entity: r.Entity} }},
	highVerb(1), highVerb(2), highVerb(3), highVerb(4),
	highVerb(5), highVerb(6), highVerb(7), highVerb(8),
	{Action: "approach", Label: "Approach selected", Rule: TargetSelected,
		Emit: func(r Resolved) widgets.Intent { return widgets.Approach{Entity: r.Entity} }},
	{Action: "orbit", Label: "Orbit selected", Rule: TargetSelected,
		Emit: func(r Resolved) widgets.Intent { return widgets.Orbit{Entity: r.Entity} }},
	{Action: "stop", Label: "Stop", Rule: TargetSelf, Ready: needsSelection,
		Emit: func(Resolved) widgets.Intent { return widgets.Stop{} }},
	{Action: "align", Label: "Align to selected", Rule: TargetSelected,
		Emit: func(r Resolved) widgets.Intent { return widgets.Align{Entity: r.Entity} }},
	{Action: "warp", Label: "Warp to selected", Rule: TargetSelected,
		Eligible: func(r Resolved) bool { return r.Info.CanWarp }, Ready: warpReady,
		Emit: func(r Resolved) widgets.Intent { return widgets.WarpTo{Entity: r.Entity} }},
	{Action: "dock", Label: "Dock (selected station, else nearest)", Rule: TargetSelectedElseNearest,
		Eligible: func(r Resolved) bool { return r.Info.CanDock },
		Emit:     func(r Resolved) widgets.Intent { return widgets.Dock{Entity: r.Entity} }},
	{Action: "jump", Label: "Jump through gate (selected, else nearest)", Rule: TargetSelectedElseNearest,
		Eligible: func(r Resolved) bool { return r.Info.CanJump },
		Emit:     func(r Resolved) widgets.Intent { return widgets.Jump{Entity: r.Entity} }},
	{Action: "loot", Label: "Loot nearest wreck", Rule: TargetNearest,
		Eligible: func(r Resolved) bool { return r.Info.CanLoot },
		Emit:     func(r Resolved) widgets.Intent { return widgets.Loot{Entity: r.Entity} }},
	{Action: "mine", Label: "Mine selected asteroid", Rule: TargetSelected,
		Eligible: func(r Resolved) bool { return r.Info.Kind == sim.KindAsteroid && r.Slot.Class != "" },
		Ready:    func(r Resolved) (bool, string) { return r.Info.CanMine, "" },
		Emit:     func(r Resolved) widgets.Intent { return widgets.Mine{Entity: r.Entity, Slot: r.Slot} }},
}

// highVerb is one F-key module toggle: no target of its own (the player),
// the selection rides along as the module's target on activate.
func highVerb(n int) WorldVerb {
	label := strconv.Itoa(n)
	return WorldVerb{
		Action:   "high-" + label,
		Label:    "Toggle high slot " + label,
		Rule:     TargetSelf,
		Eligible: func(r Resolved) bool { return r.Slot.Class != "" },
		Emit: func(r Resolved) widgets.Intent {
			if r.Active {
				return widgets.Deactivate{Slot: r.Slot}
			}
			return widgets.Activate{Slot: r.Slot, Target: r.Sel}
		},
	}
}

// canLock is the set of targetable kinds (ships, rocks, wrecks).
func canLock(kind string) bool {
	switch kind {
	case sim.KindShip, sim.KindAsteroid, sim.KindWreck:
		return true
	}
	return false
}

// lockable is the lock rows' eligibility.
func lockable(r Resolved) bool { return canLock(r.Info.Kind) }

// warpReady is the Warp button's gate inside sim.WarpMinRange; its why
// text is the hover hint's line.
func warpReady(r Resolved) (bool, string) {
	if r.Info.Distance < sim.WarpMinRange {
		return false, "Target within warp minimum range (" + sim.KmLabel(sim.WarpMinRange) + ")"
	}
	return true, ""
}

// needsSelection is Stop's panel gate: the button greys out with nothing
// selected, while the key still stops the ship (the sim needs no target).
func needsSelection(r Resolved) (bool, string) { return !r.Sel.IsZero(), "" }

// resolveVerb resolves one row against the view: the selection, the live
// grid and, for TargetCycle, the candidate list (nil = derive from the
// grid). Pure — no Kit, no queue. ok is false when the row has no
// eligible target at all.
func resolveVerb(v WorldVerb, cv widgets.ContentView, sel ecs.Entity, cycle []sim.Info) (Resolved, bool) {
	try := func(e ecs.Entity, info sim.Info) (Resolved, bool) {
		r := Resolved{Sel: sel, Entity: e, Info: info}
		if slot, active, ok := slotFor(v.Action, cv); ok {
			r.Slot, r.Active = slot, active
		}
		if v.Eligible != nil && !v.Eligible(r) {
			return r, false
		}
		return r, true
	}
	switch v.Rule {
	case TargetSelf:
		info, _ := cv.Info(cv.Player())
		return try(cv.Player(), info)
	case TargetCycle:
		if cycle == nil {
			cycle = lockCands(cv)
		}
		if len(cycle) == 0 {
			return Resolved{Sel: sel}, false
		}
		c := cycle[nextLockIdx(cycle, sel)]
		return try(c.Entity, c)
	}
	if v.Rule != TargetNearest && !sel.IsZero() {
		if info, ok := cv.Info(sel); ok {
			if r, ok := try(sel, info); ok {
				return r, true
			}
		}
	}
	if v.Rule == TargetSelected {
		return Resolved{Sel: sel}, false
	}
	// The fallback filter: the nearest perceivable entity Eligible accepts.
	var best sim.Info
	found := false
	for _, info := range cv.Grid() {
		if !info.Visible {
			continue
		}
		if _, ok := try(info.Entity, info); !ok {
			continue
		}
		if !found || info.Distance < best.Distance {
			best, found = info, true
		}
	}
	if !found {
		return Resolved{Sel: sel}, false
	}
	return try(best.Entity, best)
}

// lockCands is the Tab cycle's candidate list: every perceivable lockable
// entity except the player, nearest first.
func lockCands(cv widgets.ContentView) []sim.Info {
	var cands []sim.Info
	player := cv.Player()
	for _, inf := range cv.Grid() {
		if inf.Visible && canLock(inf.Kind) && inf.Entity != player {
			cands = append(cands, inf)
		}
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].Distance < cands[j].Distance })
	return cands
}

// nextLockIdx is the Tab step over the distance-ordered candidate list:
// the position after sel, wrapping to the nearest; no selection — or one
// that is not a candidate (an entity that left the list, a station, an
// empty selection) restarts the cycle at the nearest. Pure — pinned by
// TestNextLockIdx.
func nextLockIdx(cands []sim.Info, sel ecs.Entity) int {
	if !sel.IsZero() {
		for i, c := range cands {
			if c.Entity == sel {
				return (i + 1) % len(cands)
			}
		}
	}
	return 0
}

// slotFor resolves the slot a verb acts through: the first fitted mining
// laser for mine, high slot N (with its live runtime) for high-N. ok is
// false when the fit has no such slot, and the row's Eligible refuses.
func slotFor(action string, cv widgets.ContentView) (slot sim.SlotRef, active, ok bool) {
	if action == "mine" {
		slot, ok = firstMiningSlot(cv)
		return slot, false, ok
	}
	idx, isHigh := highIdx(action)
	if !isHigh {
		return sim.SlotRef{}, false, false
	}
	fit, found := cv.Fitting(cv.Player())
	if !found {
		return sim.SlotRef{}, false, false
	}
	rts := fit.Runtimes(content.SlotHigh)
	if idx >= len(rts) {
		return sim.SlotRef{}, false, false
	}
	return sim.SlotRef{Class: content.SlotHigh, Index: idx}, rts[idx].Active, true
}

// highIdx maps an action id to its high-slot index ("high-3" → 2).
func highIdx(action string) (int, bool) {
	if !strings.HasPrefix(action, "high-") {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(action, "high-"))
	if err != nil || n < 1 || n > 8 {
		return 0, false
	}
	return n - 1, true
}

// firstMiningSlot finds the first fitted mining laser.
func firstMiningSlot(cv widgets.ContentView) (sim.SlotRef, bool) {
	fit, ok := cv.Fitting(cv.Player())
	if !ok {
		return sim.SlotRef{}, false
	}
	for _, class := range []string{content.SlotHigh, content.SlotMid} {
		for i, id := range fit.Defs(class) {
			if def, ok := cv.Defs().Module(id); ok && def.Kind == content.KindMiningLaser {
				return sim.SlotRef{Class: class, Index: i}, true
			}
		}
	}
	return sim.SlotRef{}, false
}

// Verb returns the table row for an action id.
func (k *Kit) Verb(action string) (WorldVerb, bool) {
	for _, v := range WorldVerbs {
		if v.Action == action {
			return v, true
		}
	}
	return WorldVerb{}, false
}

// DoWorldVerb resolves a row against the live selection and emits its
// intent through the sink — the key path and every panel click. A cycle
// row moves the selection first: the lock is what it selected.
func (k *Kit) DoWorldVerb(action string) {
	v, ok := k.Verb(action)
	if !ok {
		return
	}
	res, ok := resolveVerb(v, k.cv, k.selected, nil)
	if !ok {
		return
	}
	if v.Rule == TargetCycle {
		k.Select(res.Entity)
	}
	k.sink.Do(v.Emit(res))
}

// VerbReady reports whether the Selected Item panel would offer a row
// here, plus its why text ("" when there is none). The key path must not
// call it: a refusal is the sim's to speak.
func (k *Kit) VerbReady(action string) (bool, string) {
	return k.verbReadyFor(action, k.selected)
}

// verbReadyFor is VerbReady against an explicit target: the overview's
// row actions gate on the ROW's entity, never on the selection.
func (k *Kit) verbReadyFor(action string, e ecs.Entity) (bool, string) {
	v, ok := k.Verb(action)
	if !ok {
		return false, ""
	}
	res, ok := resolveVerb(v, k.cv, e, nil)
	if !ok {
		return false, ""
	}
	if v.Ready == nil {
		return true, ""
	}
	return v.Ready(res)
}
