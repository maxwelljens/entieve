package widgets

import (
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/uikit/theme"
)

// TestResolveTradeable: the one tradeable ladder over shipped content —
// item, module and hull each resolve to their def's kind, name and themed
// icon (the drop-in art for the ids that have it, the class/category/hull
// glyph otherwise); an id no def backs reads as itself with no ref.
func TestResolveTradeable(t *testing.T) {
	cv, c := newViewTest(t)
	hull, _ := c.Ship("ship-sloop")
	for _, tc := range []struct {
		id    content.ItemID
		kind  InfoKind
		name  string
		icon  string
		found bool
	}{
		{"mod-autocannon-s", InfoModule, "Small Autocannon", "mod-autocannon-s", true},
		{"ore-sterngrit", InfoItem, "Sterngrit Ore", "ore-sterngrit", true},
		{"min-ferrite", InfoItem, "Sideron", "min-ferrite", true},
		{"ship-sloop", InfoShip, hull.Name, theme.HullIcon, true},
		{"npc-rat-scuttler", 0, "npc-rat-scuttler", "", false},
	} {
		tr := ResolveTradeable(c, tc.id)
		if tr.ID != tc.id || tr.Kind != tc.kind || tr.Name != tc.name || tr.Icon != tc.icon || tr.Found != tc.found {
			t.Errorf("ResolveTradeable(%q) = %+v, want kind=%v name=%q icon=%q found=%v",
				tc.id, tr, tc.kind, tc.name, tc.icon, tc.found)
		}
		ref, ok := tr.Ref()
		if ok != tc.found {
			t.Errorf("%s: Ref ok = %v, want %v", tc.id, ok, tc.found)
			continue
		}
		if ok && (ref.Kind != tc.kind || ref.ID != string(tc.id) || !InfoRefExists(cv, ref)) {
			t.Errorf("%s: Ref = %v, want the loaded %v ref", tc.id, ref, tc.kind)
		}
	}
	if got := TradeableName(c, "ship-sloop"); got != hull.Name {
		t.Errorf("TradeableName(ship-sloop) = %q, want %q", got, hull.Name)
	}
	if got := TradeableIcon(c, "ship-sloop"); got != theme.HullIcon {
		t.Errorf("TradeableIcon(ship-sloop) = %q, want %q", got, theme.HullIcon)
	}
	if got := TradeableIcon(c, "npc-rat-scuttler"); got != "" {
		t.Errorf("TradeableIcon(unknown) = %q, want no icon", got)
	}
}

// TestResolveTradeablePrecedence: one id reachable through every kind resolves
// item → module → hull, the sim's itemName order — ids are unique per kind, so
// this only decides colliding or stale data.
func TestResolveTradeablePrecedence(t *testing.T) {
	c := &content.Content{
		Items:   map[content.ItemID]*content.ItemDef{"dup": {ID: "dup", Name: "Item wins"}},
		Modules: map[content.ModuleID]*content.ModuleDef{"dup": {ID: "dup", Name: "Module loses", Kind: content.KindWeapon}},
		Ships:   map[content.ShipID]*content.ShipDef{"dup": {ID: "dup", Name: "Hull loses"}},
	}
	if tr := ResolveTradeable(c, "dup"); tr.Kind != InfoItem || tr.Name != "Item wins" {
		t.Errorf("colliding id = %+v, want the item %q", tr, "Item wins")
	}
	delete(c.Items, "dup")
	if tr := ResolveTradeable(c, "dup"); tr.Kind != InfoModule || tr.Name != "Module loses" {
		t.Errorf("item-less colliding id = %+v, want the module", tr)
	}
	if tr := ResolveTradeable(nil, "dup"); tr.Found || tr.Name != "dup" {
		t.Errorf("nil content = %+v, want the raw id with no def", tr)
	}
}
