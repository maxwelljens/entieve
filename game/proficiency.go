package game

import uikit "github.com/entieve/entieve/uikit/screens"

// proficiencyModel is the proficiency tree's read of the pilot's state: the
// sim's families as-is (uikit aliases sim.ProfFamily, so there is no copy to
// keep in step). The screen re-reads it every Sync, so a level-up lights the
// tree on the frame it happens.
func (g *Game) proficiencyModel() []uikit.ProfFamily {
	return g.s.ProficiencyFamilies()
}
