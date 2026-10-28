package game

import (
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
)

// worldKey runs the in-space command a live key table resolved to. It
// runs only while the space or station scene is on top and Ctrl is not
// held (Ctrl combos belong to the game layer). The rows are uikit's one
// world-verb table (uikit/screens/worldverbs.go) — the same rows the
// Selected Item buttons and the overview row actions act through — and
// the intent they emit lands on Game.Do like every other surface's.
func (g *Game) worldKey(action string) {
	g.kit.DoWorldVerb(action)
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
