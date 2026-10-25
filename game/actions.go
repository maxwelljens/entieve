package game

import (
	"sort"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
)

// worldKey runs the in-space command a live key table resolved to. It
// runs only while the space or station scene is on top and Ctrl is not
// held (Ctrl combos belong to the game layer). Semantics mirror the
// legacy ui.Update key switch; the action ids are worldKeyMap's.
func (g *Game) worldKey(action string) {
	switch action {
	case "lock-nearest":
		g.cycleLock()
	case "lock-selected":
		g.lockSelected()
	case "high-1":
		g.toggleModule(content.SlotHigh, 0)
	case "high-2":
		g.toggleModule(content.SlotHigh, 1)
	case "high-3":
		g.toggleModule(content.SlotHigh, 2)
	case "high-4":
		g.toggleModule(content.SlotHigh, 3)
	case "high-5":
		g.toggleModule(content.SlotHigh, 4)
	case "high-6":
		g.toggleModule(content.SlotHigh, 5)
	case "high-7":
		g.toggleModule(content.SlotHigh, 6)
	case "high-8":
		g.toggleModule(content.SlotHigh, 7)
	case "approach":
		g.doApproach()
	case "orbit":
		g.doOrbit()
	case "stop":
		g.s.Queue(sim.CmdStop{})
	case "align":
		g.doAlign()
	case "warp":
		g.doWarp()
	case "dock":
		g.dockNearest()
	case "jump":
		g.jumpNearest()
	case "loot":
		g.lootNearest()
	case "mine":
		g.mineSelected()
	}
}

// setSelected records the selection in the kit: world clicks, the Tab
// cycle-lock and the overview table all share Kit.Selected as the single
// source, so the Selected Item panel's buttons never act on a stale
// selection.
func (g *Game) setSelected(e ecs.Entity) { g.kit.Select(e) }

// selected resolves the current selection (the kit owns it).
func (g *Game) selected() ecs.Entity { return g.kit.Selected() }

// selectedInfo resolves the current selection against the live grid.
func (g *Game) selectedInfo() (sim.Info, bool) {
	sel := g.selected()
	if sel.IsZero() {
		return sim.Info{}, false
	}
	return g.s.Info(sel)
}

// doApproach enqueues CmdApproach for the selection (A).
func (g *Game) doApproach() {
	if _, ok := g.selectedInfo(); ok {
		g.s.Queue(sim.CmdApproach{Target: g.selected()})
	}
}

// doOrbit enqueues CmdOrbit at the Selected Item orbit selector's
// current range (Q): the selector is the only thing that changes a
// range, the key and the Orbit button both act at Kit.OrbitRange().
func (g *Game) doOrbit() {
	if _, ok := g.selectedInfo(); ok {
		g.s.Queue(sim.CmdOrbit{Target: g.selected(), Range: g.kit.OrbitRange()})
	}
}

// doAlign enqueues CmdAlignTo for the selection (W).
func (g *Game) doAlign() {
	if _, ok := g.selectedInfo(); ok {
		g.s.Queue(sim.CmdAlignTo{Target: g.selected()})
	}
}

// doWarp warps to the selection at the Selected Item warp selector's
// current range (Z), gated on the Info flag like the Warp button; the
// sim enforces sim.WarpMinRange on top of that.
func (g *Game) doWarp() {
	if info, ok := g.selectedInfo(); ok && info.CanWarp {
		g.s.Queue(sim.CmdWarpTo{Target: g.selected(), Range: g.kit.WarpRange()})
	}
}

// lockSelected locks the current selection (Shift+Tab).
func (g *Game) lockSelected() {
	if _, ok := g.selectedInfo(); ok {
		g.s.Queue(sim.CmdLock{Target: g.selected()})
	}
}

// cycleLock locks the next-nearest lockable entity (Tab): distance-ordered
// cycling over every perceivable lockable on grid.
//
// The step is anchored to the CURRENT selection's position in the freshly
// sorted list, not to a remembered index: distances re-sort between
// presses, and a stale index would skip or repeat an entry.
func (g *Game) cycleLock() {
	var cands []sim.Info
	player := g.s.Player()
	for _, inf := range g.s.Grid() {
		if inf.Visible && canLock(inf.Kind) && inf.Entity != player {
			cands = append(cands, inf)
		}
	}
	if len(cands) == 0 {
		return
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].Distance < cands[j].Distance })
	next := nextLockIdx(cands, g.selected())
	g.setSelected(cands[next].Entity)
	g.s.Queue(sim.CmdLock{Target: cands[next].Entity})
}

// nextLockIdx is the Tab step over the distance-ordered candidate list:
// the position after sel, wrapping to the nearest; no selection — or one
// that is not a candidate (an entity that left the list, a station, an
// empty selection) restarts the cycle at the nearest. Pure — unit-tested
// by TestNextLockIdx (the showcase grid spawns no lockable entities, so
// this is where the anchoring semantics are pinned exactly).
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

// canLock reports whether entities of this kind can be targeted.
func canLock(kind string) bool {
	switch kind {
	case sim.KindShip, sim.KindAsteroid, sim.KindWreck:
		return true
	}
	return false
}

// nearest returns the closest perceivable grid entity satisfying match.
func (g *Game) nearest(match func(sim.Info) bool) (sim.Info, bool) {
	var best sim.Info
	found := false
	for _, inf := range g.s.Grid() {
		if !inf.Visible || !match(inf) {
			continue
		}
		if !found || inf.Distance < best.Distance {
			best, found = inf, true
		}
	}
	return best, found
}

// dockNearest docks at the selected station, else the nearest dockable one.
func (g *Game) dockNearest() {
	if info, ok := g.selectedInfo(); ok && info.CanDock {
		g.s.Queue(sim.CmdDock{Station: g.selected()})
		return
	}
	if inf, ok := g.nearest(func(i sim.Info) bool { return i.CanDock }); ok {
		g.s.Queue(sim.CmdDock{Station: inf.Entity})
	}
}

// jumpNearest jumps through the selected gate, else the nearest jumpable one.
func (g *Game) jumpNearest() {
	if info, ok := g.selectedInfo(); ok && info.CanJump {
		g.s.Queue(sim.CmdJump{Gate: g.selected()})
		return
	}
	if inf, ok := g.nearest(func(i sim.Info) bool { return i.CanJump }); ok {
		g.s.Queue(sim.CmdJump{Gate: inf.Entity})
	}
}

// lootNearest loots the closest wreck (L).
func (g *Game) lootNearest() {
	if inf, ok := g.nearest(func(i sim.Info) bool { return i.CanLoot }); ok {
		g.s.Queue(sim.CmdLoot{Wreck: inf.Entity})
	}
}

// mineSelected fires the first mining laser at the selected asteroid (M).
func (g *Game) mineSelected() {
	info, ok := g.selectedInfo()
	if !ok || info.Kind != sim.KindAsteroid {
		return
	}
	slot, ok := g.firstMiningSlot()
	if !ok {
		return
	}
	g.s.Queue(sim.CmdMine{Asteroid: g.selected(), Slot: slot})
}

// firstMiningSlot finds the first fitted mining laser.
func (g *Game) firstMiningSlot() (sim.SlotRef, bool) {
	fit, ok := g.s.Fittings.Get(g.s.Player())
	if !ok {
		return sim.SlotRef{}, false
	}
	for _, class := range []string{content.SlotHigh, content.SlotMid} {
		for i, id := range fit.Defs(class) {
			if def, ok := g.opts.Content.Module(id); ok && def.Kind == content.KindMiningLaser {
				return sim.SlotRef{Class: class, Index: i}, true
			}
		}
	}
	return sim.SlotRef{}, false
}

// toggleModule activates/deactivates a fitted module by slot class and
// index (F1–F8). The target is the current selection (zero = self, per
// sim contract).
func (g *Game) toggleModule(class string, idx int) {
	fit, ok := g.s.Fittings.Get(g.s.Player())
	if !ok {
		return
	}
	rts := fit.Runtimes(class)
	if idx < 0 || idx >= len(rts) {
		return
	}
	slot := sim.SlotRef{Class: class, Index: idx}
	if rts[idx].Active {
		g.s.Queue(sim.CmdDeactivate{Slot: slot})
		return
	}
	g.s.Queue(sim.CmdActivate{Slot: slot, Target: g.selected()})
}
