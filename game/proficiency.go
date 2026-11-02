package game

import uikit "github.com/entieve/entieve/uikit/screens"

// proficiencyModel is the proficiency tree's read of the pilot's state.
//
// TODO(integration): the proficiency wave's sim model maps onto
// uikit.ProfFamily here (family id, display name, level, XP/XPNext, cap,
// authored prerequisites, Meets, Starter). Until that branch merges this
// returns the fixture below, so the screen builds, draws and tests green
// without the sim side. The coordinator replaces the body; the Screen
// row's wiring does not change.
func (g *Game) proficiencyModel() []uikit.ProfFamily {
	return proficiencyFixture()
}

// proficiencyFixture is the placeholder model: twelve families, four
// ranks, three starters, one diamond (Rigging needs two branches) and two
// gated nodes (Manufacturing waits on Refining 3, Rigging on Manufacturing
// 1). Meets is authored here exactly as the sim will derive it.
func proficiencyFixture() []uikit.ProfFamily {
	return []uikit.ProfFamily{
		{Family: "navigation", Name: "Navigation", Level: 5, MaxLevel: 5, Meets: true, Starter: true},
		{Family: "shield", Name: "Shield Operation", Level: 3, MaxLevel: 5, XP: 100, XPNext: 240, Meets: true,
			Prereqs: []uikit.ProfPrereq{{Family: "navigation", Level: 2}}},
		{Family: "armour", Name: "Armour Plating", Level: 1, MaxLevel: 5, XP: 40, XPNext: 120, Meets: true,
			Prereqs: []uikit.ProfPrereq{{Family: "shield", Level: 1}}},
		{Family: "autocannon", Name: "Autocannons", Level: 3, MaxLevel: 5, XP: 240, XPNext: 300, Meets: true, Starter: true},
		{Family: "artillery", Name: "Artillery", Level: 2, MaxLevel: 5, XP: 90, XPNext: 160, Meets: true, Starter: true},
		{Family: "salvage", Name: "Salvage", Level: 0, MaxLevel: 3, XP: 0, XPNext: 60, Meets: true,
			Prereqs: []uikit.ProfPrereq{{Family: "autocannon", Level: 2}}},
		{Family: "mining", Name: "Mining", Level: 4, MaxLevel: 5, XP: 700, XPNext: 900, Meets: true, Starter: true},
		{Family: "prospecting", Name: "Prospecting", Level: 1, MaxLevel: 3, XP: 30, XPNext: 60, Meets: true,
			Prereqs: []uikit.ProfPrereq{{Family: "mining", Level: 1}}},
		{Family: "refining", Name: "Refining", Level: 2, MaxLevel: 4, XP: 20, XPNext: 120, Meets: true,
			Prereqs: []uikit.ProfPrereq{{Family: "mining", Level: 2}}},
		{Family: "manufacturing", Name: "Manufacturing", Level: 1, MaxLevel: 4, XP: 0, XPNext: 100, Meets: false,
			Prereqs: []uikit.ProfPrereq{{Family: "refining", Level: 3}}},
		{Family: "haulage", Name: "Haulage", Level: 2, MaxLevel: 4, XP: 150, XPNext: 200, Meets: true},
		{Family: "rigging", Name: "Rigging", Level: 0, MaxLevel: 3, XP: 0, XPNext: 80, Meets: false,
			Prereqs: []uikit.ProfPrereq{{Family: "manufacturing", Level: 2}, {Family: "haulage", Level: 2}}},
	}
}
