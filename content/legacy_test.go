package content

import (
	"strings"
	"testing"
)

// TestLegacyRegistry pins the retired-id registry's shape: RetiredIDs is
// exactly the keys of LegacyIDs (validation and restore read the same list),
// and no replacement is itself retired, so a remap is idempotent.
func TestLegacyRegistry(t *testing.T) {
	want := make(map[string]bool)
	for old, cur := range LegacyIDs.Items {
		want[string(old)] = true
		if RetiredIDs[string(cur)] {
			t.Errorf("items: %s -> %s, but %s is itself retired", old, cur, cur)
		}
	}
	for old, cur := range LegacyIDs.Modules {
		want[string(old)] = true
		if RetiredIDs[string(cur)] {
			t.Errorf("modules: %s -> %s, but %s is itself retired", old, cur, cur)
		}
	}
	for old, cur := range LegacyIDs.Recipes {
		want[string(old)] = true
		if RetiredIDs[string(cur)] {
			t.Errorf("recipes: %s -> %s, but %s is itself retired", old, cur, cur)
		}
	}
	for old, cur := range LegacyIDs.Ships {
		want[string(old)] = true
		if RetiredIDs[string(cur)] {
			t.Errorf("ships: %s -> %s, but %s is itself retired", old, cur, cur)
		}
	}
	if len(RetiredIDs) != len(want) {
		t.Fatalf("RetiredIDs has %d ids, registry keys %d", len(RetiredIDs), len(want))
	}
	for id := range want {
		if !RetiredIDs[id] {
			t.Errorf("RetiredIDs misses registry key %s", id)
		}
	}
}

// TestLegacyIDsResolveInShippedContent pins the registry against the shipped
// data: every replacement names content of its kind, and no retired id is
// still shipped (a rename that missed a file would load the old id).
func TestLegacyIDsResolveInShippedContent(t *testing.T) {
	c := MustLoad()
	for old, cur := range LegacyIDs.Ships {
		if _, ok := c.Ships[cur]; !ok {
			t.Errorf("ships: %s -> %s, but %s is not shipped", old, cur, cur)
		}
		if _, ok := c.Ships[old]; ok {
			t.Errorf("ships: retired id %s is still shipped", old)
		}
	}
	for old, cur := range LegacyIDs.Items {
		if _, ok := c.Items[cur]; !ok {
			t.Errorf("items: %s -> %s, but %s is not shipped", old, cur, cur)
		}
	}
	for old, cur := range LegacyIDs.Modules {
		if _, ok := c.Modules[cur]; !ok {
			t.Errorf("modules: %s -> %s, but %s is not shipped", old, cur, cur)
		}
	}
	for old, cur := range LegacyIDs.Recipes {
		if _, ok := c.Recipes[cur]; !ok {
			t.Errorf("recipes: %s -> %s, but %s is not shipped", old, cur, cur)
		}
	}
}

// TestValidateRejectsRetiredIDs keeps the validation behaviour the registry
// was folded out of: a retired item or module id in shipped content is a
// validation problem, never a silent load.
func TestValidateRejectsRetiredIDs(t *testing.T) {
	c := baseContent()
	c.Items["charge-emp"] = &ItemDef{
		ID: "charge-emp", Name: "EMP Charge", Group: "Ammunition/Cap cells", Category: CatCharge, Volume: 0.01, BasePrice: 100,
	}
	c.Modules["mod-emp-turret"] = &ModuleDef{
		ID: "mod-emp-turret", Name: "EMP Turret", Group: "Modules/High/Turrets", Slot: SlotHigh, Kind: KindWeapon,
		ActivationCost: 10, ActivationTime: 2, Price: 10000,
		Weapon: &WeaponAttrs{Damage: Damage{EM: 10}, Optimal: 8000, Falloff: 4000,
			Tracking: 0.4, AmmoClass: "light", ShotTime: 2, BayM3: 0.1},
	}
	// A retired hull: the rename demands the display-name slug, not the
	// machinery prefix.
	c.Ships["ship-sloop"] = c.Ships["ship-hull"]
	c.Ships["ship-sloop"].ID = "ship-sloop"
	delete(c.Ships, "ship-hull")
	err := c.Validate()
	if err == nil {
		t.Fatal("retired ids validate clean")
	}
	for _, want := range []string{
		"items/charge-emp: id is retired",
		"modules/mod-emp-turret: id is retired",
		"ships/ship-sloop: id is retired",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("validation error misses %q:\n%v", want, err)
		}
	}
}
