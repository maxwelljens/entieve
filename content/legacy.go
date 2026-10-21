package content

// LegacyIDs is the retired-id registry: each id a rename retired maps onto
// the id that replaced it. The shell-weapons rename replaced the laser
// turrets, their charges and their production runs
// (docs/CONTENT_AUTHORING.md — Weapons and shells). Save restore remaps saves
// carrying these ids in place (sim/save.go migrateLegacyIDs); validation
// rejects them in shipped content (RetiredIDs).
var LegacyIDs = struct {
	Items   map[ItemID]ItemID
	Modules map[ModuleID]ModuleID
	Recipes map[RecipeID]RecipeID
}{
	// The retired charges map onto the small-calibre shell of the same
	// damage profile (the only calibre those guns could load).
	Items: map[ItemID]ItemID{
		"charge-emp":       "ammo-emp-s",
		"charge-fusion":    "ammo-fusion-s",
		"charge-ap":        "ammo-ap-s",
		"charge-plutonium": "ammo-plutonium-s",
	},
	// The retired turrets map onto the small-calibre gun of their family
	// (EMP turret: fast light turret -> autocannon; rail: long-range sniper
	// -> artillery).
	Modules: map[ModuleID]ModuleID{
		"mod-emp-turret":  "mod-autocannon-s",
		"mod-rail-turret": "mod-artillery-s",
	},
	// The retired production runs map onto the shell recipes.
	Recipes: map[RecipeID]RecipeID{
		"rec-charge-emp":       "rec-ammo-emp-s",
		"rec-charge-fusion":    "rec-ammo-fusion-s",
		"rec-charge-ap":        "rec-ammo-ap-s",
		"rec-charge-plutonium": "rec-ammo-plutonium-s",
	},
}

// RetiredIDs is every id LegacyIDs retires: they must never appear in
// shipped content.
var RetiredIDs = func() map[string]bool {
	m := make(map[string]bool, len(LegacyIDs.Items)+len(LegacyIDs.Modules)+len(LegacyIDs.Recipes))
	for old := range LegacyIDs.Items {
		m[string(old)] = true
	}
	for old := range LegacyIDs.Modules {
		m[string(old)] = true
	}
	for old := range LegacyIDs.Recipes {
		m[string(old)] = true
	}
	return m
}()
