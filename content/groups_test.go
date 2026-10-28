package content

import (
	"strings"
	"testing"
)

// marketGroups is the binding market tree (docs/lore/NAMING.md): every
// marketable id's group path, frozen with the tree feature. The market
// window's tree is rendered from these paths, so a re-homed def is a
// deliberate edit here — never a silent re-sort of the tree.
var marketGroups = map[string]string{
	// Ships
	"ship-barge":        "Ships/Barges",
	"ship-cutlass":      "Ships/Destroyers",
	"ship-hauler":       "Ships/Industrials",
	"ship-npc-fleet":    "Ships/Destroyers",
	"ship-npc-hauler":   "Ships/Industrials",
	"ship-npc-miner":    "Ships/Industrials",
	"ship-npc-patrol":   "Ships/Frigates",
	"ship-rat-hierarch": "Ships/Rat hulls",
	"ship-rat-marauder": "Ships/Rat hulls",
	"ship-rat-scuttler": "Ships/Rat hulls",
	"ship-sloop":        "Ships/Frigates",
	// Modules
	"mod-afterburner":     "Modules/Mid/Propulsion",
	"mod-armor-plate":     "Modules/Low/Armor",
	"mod-armor-rep":       "Modules/Low/Armor",
	"mod-artillery-l":     "Modules/High/Turrets",
	"mod-artillery-m":     "Modules/High/Turrets",
	"mod-artillery-s":     "Modules/High/Turrets",
	"mod-autocannon-l":    "Modules/High/Turrets",
	"mod-autocannon-m":    "Modules/High/Turrets",
	"mod-autocannon-s":    "Modules/High/Turrets",
	"mod-cap-booster":     "Modules/Mid/Capacitor",
	"mod-cap-recharger":   "Modules/Low/Capacitor",
	"mod-mining-laser":    "Modules/High/Turrets",
	"mod-resist-coat":     "Modules/Low/Armor",
	"mod-rocket-pod":      "Modules/High/Launchers",
	"mod-shield-boost":    "Modules/Mid/Shield",
	"mod-shield-extender": "Modules/Mid/Shield",
	"mod-stasis-web":      "Modules/Mid/EWAR",
	"mod-warp-scram":      "Modules/Mid/EWAR",
	// Ammunition
	"ammo-ap-l":            "Ammunition/Autocannon rounds",
	"ammo-ap-m":            "Ammunition/Autocannon rounds",
	"ammo-ap-s":            "Ammunition/Autocannon rounds",
	"ammo-emp-l":           "Ammunition/Autocannon rounds",
	"ammo-emp-m":           "Ammunition/Autocannon rounds",
	"ammo-emp-s":           "Ammunition/Autocannon rounds",
	"ammo-fusion-l":        "Ammunition/Autocannon rounds",
	"ammo-fusion-m":        "Ammunition/Autocannon rounds",
	"ammo-fusion-s":        "Ammunition/Autocannon rounds",
	"ammo-plutonium-l":     "Ammunition/Autocannon rounds",
	"ammo-plutonium-m":     "Ammunition/Autocannon rounds",
	"ammo-plutonium-s":     "Ammunition/Autocannon rounds",
	"ammo-art-ap-l":        "Ammunition/Artillery shells",
	"ammo-art-ap-m":        "Ammunition/Artillery shells",
	"ammo-art-ap-s":        "Ammunition/Artillery shells",
	"ammo-art-emp-l":       "Ammunition/Artillery shells",
	"ammo-art-emp-m":       "Ammunition/Artillery shells",
	"ammo-art-emp-s":       "Ammunition/Artillery shells",
	"ammo-art-fusion-l":    "Ammunition/Artillery shells",
	"ammo-art-fusion-m":    "Ammunition/Artillery shells",
	"ammo-art-fusion-s":    "Ammunition/Artillery shells",
	"ammo-art-plutonium-l": "Ammunition/Artillery shells",
	"ammo-art-plutonium-m": "Ammunition/Artillery shells",
	"ammo-art-plutonium-s": "Ammunition/Artillery shells",
	"charge-cap-cell":      "Ammunition/Cap cells",
	// Materials and loot
	"min-argon":           "Materials/Minerals",
	"min-cobalt":          "Materials/Minerals",
	"min-ferrite":         "Materials/Minerals",
	"min-glacite":         "Materials/Minerals",
	"min-nickel":          "Materials/Minerals",
	"min-silex":           "Materials/Minerals",
	"ore-cordite":         "Materials/Ore",
	"ore-glacierite":      "Materials/Ore",
	"ore-lucidite":        "Materials/Ore",
	"ore-sterngrit":       "Materials/Ore",
	"loot-alloy-scrap":    "Loot",
	"loot-hierarch-crest": "Loot",
	"loot-iced-core":      "Loot",
	"loot-navdata":        "Loot",
}

// TestShippedMarketTree pins every shipped def's group to the approved
// tree — the market's category pane renders exactly these paths, and no
// marketable def is missing one.
func TestShippedMarketTree(t *testing.T) {
	c := MustLoad()
	if got, want := len(c.Items)+len(c.Modules)+len(c.Ships), len(marketGroups); got != want {
		t.Fatalf("shipped marketables = %d, want the pinned %d", got, want)
	}
	for id, d := range c.Items {
		if want := marketGroups[string(id)]; d.Group != want {
			t.Errorf("items/%s group = %q, want %q", id, d.Group, want)
		}
	}
	for id, d := range c.Modules {
		if want := marketGroups[string(id)]; d.Group != want {
			t.Errorf("modules/%s group = %q, want %q", id, d.Group, want)
		}
	}
	for id, d := range c.Ships {
		if want := marketGroups[string(id)]; d.Group != want {
			t.Errorf("ships/%s group = %q, want %q", id, d.Group, want)
		}
	}
}

// TestShipBasePrices pins every hull's reference price: the ladder's last
// rung (the market quotes it before a book trades), positive on every hull
// so no hull row reads 0 €.
func TestShipBasePrices(t *testing.T) {
	c := MustLoad()
	for id, d := range c.Ships {
		if d.BasePrice <= 0 {
			t.Errorf("ships/%s base_price = %v, want > 0", id, d.BasePrice)
		}
	}
}

// TestValidateRejectsBadGroups: a market tree path is one to three
// non-empty slash-separated segments — missing, leading/trailing/doubled
// slash and an over-deep path are all validation problems.
func TestValidateRejectsBadGroups(t *testing.T) {
	cases := []struct{ name, group, want string }{
		{"missing", "", "missing group"},
		{"leading slash", "/Materials", "empty segment"},
		{"trailing slash", "Materials/", "empty segment"},
		{"doubled slash", "Materials//Ore", "empty segment"},
		{"too deep", "Materials/Ore/Deep/Too", "max 3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := baseContent()
			c.Items["min-fer"].Group = tc.group
			err := c.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("group %q: want an error containing %q, got:\n%v", tc.group, tc.want, err)
			}
		})
	}
}
