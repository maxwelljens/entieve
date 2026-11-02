package game

import uikit "github.com/entieve/entieve/uikit/screens"

// proficiencyModel is the proficiency tree's read of the pilot's state:
// the sim's families mapped field-for-field onto the panel's model
// (uikit.ProfFamily mirrors sim.ProfFamily; XP/XPNext are totals and 0 at
// max, exactly as sim.ProficiencyFamilies documents). The screen re-reads
// it every Sync, so a level-up lights the tree on the frame it happens.
func (g *Game) proficiencyModel() []uikit.ProfFamily {
	families := g.s.ProficiencyFamilies()
	out := make([]uikit.ProfFamily, 0, len(families))
	for _, f := range families {
		prereqs := make([]uikit.ProfPrereq, 0, len(f.Prereqs))
		for _, p := range f.Prereqs {
			prereqs = append(prereqs, uikit.ProfPrereq{Family: p.Family, Level: p.Level})
		}
		out = append(out, uikit.ProfFamily{
			Family:   f.Family,
			Name:     f.Name,
			Level:    f.Level,
			XP:       f.XP,
			XPNext:   f.XPNext,
			MaxLevel: f.MaxLevel,
			Prereqs:  prereqs,
			Meets:    f.Meets,
			Gate:     f.Gate,
			Starter:  f.Starter,
		})
	}
	return out
}
