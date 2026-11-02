package content

// LegacyIDs is the retired-id registry: each id a rename retired maps onto
// the id that replaced it. The shell-weapons rename replaced the laser
// turrets, their charges and their production runs; the hull rename made
// every ship id the slug of its display name and retired the machinery
// prefixes (docs/CONTENT_AUTHORING.md — Weapons and shells, Hull ids). Save
// restore remaps saves carrying these ids in place (sim/save.go
// migrateLegacyIDs); validation rejects them in shipped content (RetiredIDs).
var LegacyIDs = struct {
	Items   map[ItemID]ItemID
	Modules map[ModuleID]ModuleID
	Recipes map[RecipeID]RecipeID
	Ships   map[ShipID]ShipID
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
	// The retired production runs map onto the shell recipes and the hull
	// recipes onto their renamed hulls.
	Recipes: map[RecipeID]RecipeID{
		"rec-charge-emp":        "rec-ammo-emp-s",
		"rec-charge-fusion":     "rec-ammo-fusion-s",
		"rec-charge-ap":         "rec-ammo-ap-s",
		"rec-charge-plutonium":  "rec-ammo-plutonium-s",
		"rec-ship-rat-scuttler": "rec-ship-scuttler",
		"rec-ship-rat-marauder": "rec-ship-marauder",
		"rec-ship-rat-hierarch": "rec-ship-hierarch",
		"rec-ship-npc-miner":    "rec-ship-kutrek",
		"rec-ship-npc-patrol":   "rec-ship-kustor",
		"rec-ship-npc-hauler":   "rec-ship-tobrek",
		"rec-ship-npc-fleet":    "rec-ship-clastor",
		"rec-ship-sloop":        "rec-ship-vastrek",
		"rec-ship-cutlass":      "rec-ship-zdanek",
		"rec-ship-hauler":       "rec-ship-furdzan",
		"rec-ship-barge":        "rec-ship-husnek",
	},
	// The hull rename: display-name slugs, machinery prefix dropped. A hull
	// id is also an item id (shipyard stock, hangar slots), so the save
	// migration applies this table in both id spaces.
	Ships: map[ShipID]ShipID{
		"ship-rat-scuttler": "ship-scuttler",
		"ship-rat-marauder": "ship-marauder",
		"ship-rat-hierarch": "ship-hierarch",
		"ship-npc-miner":    "ship-kutrek",
		"ship-npc-patrol":   "ship-kustor",
		"ship-npc-hauler":   "ship-tobrek",
		"ship-npc-fleet":    "ship-clastor",
		"ship-sloop":        "ship-vastrek",
		"ship-cutlass":      "ship-zdanek",
		"ship-hauler":       "ship-furdzan",
		"ship-barge":        "ship-husnek",
	},
}

// RetiredIDs is every id LegacyIDs retires: they must never appear in
// shipped content.
var RetiredIDs = func() map[string]bool {
	m := make(map[string]bool, len(LegacyIDs.Items)+len(LegacyIDs.Modules)+
		len(LegacyIDs.Recipes)+len(LegacyIDs.Ships))
	for old := range LegacyIDs.Items {
		m[string(old)] = true
	}
	for old := range LegacyIDs.Modules {
		m[string(old)] = true
	}
	for old := range LegacyIDs.Recipes {
		m[string(old)] = true
	}
	for old := range LegacyIDs.Ships {
		m[string(old)] = true
	}
	return m
}()
