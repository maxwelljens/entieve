package content

import (
	"math"
	"strings"
	"testing"
)

// TestShipSlotFloor: every hull must be able to shoot (>= 1 high slot) or be
// a hauler/industrial that exists to haul.
func TestShipSlotFloor(t *testing.T) {
	c := MustLoad()
	for _, id := range keys(c.Ships) {
		s := c.Ships[id]
		if s.Slots.High >= 1 {
			continue
		}
		class := strings.ToLower(s.Class)
		if strings.Contains(class, "industrial") || strings.Contains(class, "hauler") {
			continue
		}
		t.Errorf("ship %s (class %q) has no high slots and is not a hauler", id, s.Class)
	}
}

// TestWeaponsFire: every weapon needs a positive shot time and a non-zero
// base volley.
func TestWeaponsFire(t *testing.T) {
	c := MustLoad()
	found := 0
	for _, id := range keys(c.Modules) {
		m := c.Modules[id]
		if m.Kind != KindWeapon {
			continue
		}
		found++
		if m.Weapon == nil {
			t.Errorf("weapon %s has no weapon block", id)
			continue
		}
		if m.Weapon.ShotTime <= 0 {
			t.Errorf("weapon %s shot_time = %v, want > 0", id, m.Weapon.ShotTime)
		}
		if m.Weapon.Damage.Total() <= 0 {
			t.Errorf("weapon %s damage total = %v, want > 0", id, m.Weapon.Damage.Total())
		}
	}
	if found == 0 {
		t.Error("no weapon modules in content")
	}
}

// TestActiveModulesCycle: every active module runs cycles (positive
// activation time) and never has a negative cap cost.
func TestActiveModulesCycle(t *testing.T) {
	c := MustLoad()
	for _, id := range keys(c.Modules) {
		m := c.Modules[id]
		if !activeKinds[m.Kind] {
			continue
		}
		if m.ActivationTime <= 0 {
			t.Errorf("active module %s (kind %q) activation_time = %v, want > 0", id, m.Kind, m.ActivationTime)
		}
		if m.ActivationCost < 0 {
			t.Errorf("active module %s activation_cost = %v, want >= 0", id, m.ActivationCost)
		}
	}
}

// TestPassiveModulesHaveEffects: passive modules must actually change
// something.
func TestPassiveModulesHaveEffects(t *testing.T) {
	c := MustLoad()
	for _, id := range keys(c.Modules) {
		m := c.Modules[id]
		if !passiveKinds[m.Kind] {
			continue
		}
		if m.Passive == nil {
			t.Errorf("passive module %s has no passive block", id)
			continue
		}
		p := m.Passive
		if p.ShieldHP == 0 && p.ArmorHP == 0 && p.Capacitor == 0 && p.Recharge == 0 &&
			p.ResistBonus == (Resists{}) {
			t.Errorf("passive module %s has no non-zero passive effect", id)
		}
	}
}

// TestOresRefine: every ore must carry a refine yield.
func TestOresRefine(t *testing.T) {
	c := MustLoad()
	found := 0
	for _, id := range keys(c.Items) {
		it := c.Items[id]
		if it.Category != CatOre {
			continue
		}
		found++
		if len(it.RefineYield) == 0 {
			t.Errorf("ore %s has no refine_yield", id)
		}
	}
	if found == 0 {
		t.Error("no ores in content")
	}
}

// TestAmmoDamages: every ammo item must have a damage profile.
func TestAmmoDamages(t *testing.T) {
	c := MustLoad()
	found := 0
	for _, id := range keys(c.Items) {
		it := c.Items[id]
		if it.Category != CatAmmo {
			continue
		}
		found++
		if it.Damage == nil || it.Damage.Total() <= 0 {
			t.Errorf("ammo %s has no positive damage profile", id)
		}
	}
	if found == 0 {
		t.Error("no ammo in content")
	}
}

// TestSolaceMarketHasShipAndWeapon: the starting market must sell at least
// one hull and one weapon module.
func TestSolaceMarketHasShipAndWeapon(t *testing.T) {
	c := MustLoad()
	solace, ok := c.Systems["sys-solace"]
	if !ok {
		t.Fatal("sys-solace missing")
	}
	var founding []MarketOrderDef
	for _, cel := range solace.Celestials {
		if cel.Station != nil {
			founding = append(founding, cel.Station.Market...)
		}
	}
	var ships, weapons int
	for _, o := range founding {
		if _, isItem := c.Items[o.Item]; isItem {
			continue
		}
		if s, isShip := c.Ships[ShipID(o.Item)]; isShip && s != nil {
			ships++
			continue
		}
		if m, isMod := c.Modules[ModuleID(o.Item)]; isMod && m != nil && m.Kind == KindWeapon {
			weapons++
		}
	}
	if ships == 0 {
		t.Error("sys-solace market lists no ships")
	}
	if weapons == 0 {
		t.Error("sys-solace market lists no weapon modules")
	}
}

// ---------------------------------------------------------------------------
// Manufacturing balance: the recipe envelope (docs/CONTENT_AUTHORING.md)
// ---------------------------------------------------------------------------

// Material-cost envelope: the minerals one run consumes must be worth
// 55-75% of the product's reference price per produced unit. The calibration
// contract every shipped and future recipe must follow.
const (
	recipeMaterialPctMin = 55.0
	recipeMaterialPctMax = 75.0
)

// hullReferencePrices pins reference prices for hulls that ship no market
// order (hulls have no price field; the NPC hulls are not listed anywhere).
// Market-listed hulls use their shipped order price instead — see
// recipeReferencePrice.
var hullReferencePrices = map[ShipID]float64{
	"ship-rat-scuttler": 330000,
	"ship-npc-patrol":   400000,
	"ship-rat-marauder": 520000,
	"ship-npc-hauler":   600000,
	"ship-rat-hierarch": 900000,
}

// recipeReferencePrice resolves the product's reference price: item
// base_price, module price, or hull — the shipped market order price when
// the hull is listed, else the pinned table (hulls have no price field and
// the NPC hulls ship no order).
func recipeReferencePrice(t *testing.T, c *Content, product ItemID) float64 {
	t.Helper()
	if it, ok := c.Items[product]; ok {
		return it.BasePrice
	}
	if _, ok := c.Ships[ShipID(product)]; ok {
		for _, sysID := range keys(c.Systems) {
			for _, cel := range c.Systems[sysID].Celestials {
				if cel.Station == nil {
					continue
				}
				for _, o := range cel.Station.Market {
					if o.Item == product {
						return o.Price
					}
				}
			}
		}
		if p, pinned := hullReferencePrices[ShipID(product)]; pinned {
			return p
		}
		t.Fatalf("hull %s has no market order and no pinned reference price", product)
	}
	if m, ok := c.Modules[ModuleID(product)]; ok {
		return m.Price
	}
	t.Fatalf("recipe product %s resolves to no def", product)
	return 0
}

// recipeMaterialCost is the total base-price value of one run's inputs.
func recipeMaterialCost(t *testing.T, c *Content, r *RecipeDef) float64 {
	t.Helper()
	total := 0.0
	for _, in := range r.Inputs {
		it, ok := c.Items[in.Item]
		if !ok {
			t.Fatalf("%s: input %s does not resolve", r.ID, in.Item)
		}
		total += in.Qty * it.BasePrice
	}
	return total
}

// TestRecipeMaterialCostEnvelope pins every shipped recipe's material cost
// per produced unit inside the 55-75% envelope of the reference price.
func TestRecipeMaterialCostEnvelope(t *testing.T) {
	c := MustLoad()
	recipes := c.RecipesInOrder()
	if len(recipes) == 0 {
		t.Fatal("no recipes loaded")
	}
	for _, r := range recipes {
		ref := recipeReferencePrice(t, c, r.Product)
		pct := 100 * recipeMaterialCost(t, c, r) / (ref * float64(r.Qty))
		if pct < recipeMaterialPctMin || pct > recipeMaterialPctMax {
			t.Errorf("%s: material cost %.1f%% of reference price %v for %d unit(s), want %.0f-%.0f%%",
				r.ID, pct, ref, r.Qty, recipeMaterialPctMin, recipeMaterialPctMax)
		}
	}
}

// TestHullRecipesCostMoreThanModules pins the monotonicity sanity rule: the
// cheapest hull run must consume more mineral value than the priciest
// module run (hulls sit on top of the crafting ladder).
func TestHullRecipesCostMoreThanModules(t *testing.T) {
	c := MustLoad()
	var cheapestHull, priciestModule *RecipeDef
	minHull, maxModule := math.Inf(1), math.Inf(-1)
	for _, r := range c.RecipesInOrder() {
		cost := recipeMaterialCost(t, c, r)
		switch r.Family {
		case FamilyHulls:
			if cost < minHull {
				minHull, cheapestHull = cost, r
			}
		case FamilyModules:
			if cost > maxModule {
				maxModule, priciestModule = cost, r
			}
		}
	}
	if cheapestHull == nil || priciestModule == nil {
		t.Fatal("need at least one hull and one module recipe")
	}
	if minHull <= maxModule {
		t.Errorf("cheapest hull recipe %s costs %v, priciest module recipe %s costs %v; hull recipes must cost more than module recipes",
			cheapestHull.ID, minHull, priciestModule.ID, maxModule)
	}
}

// TestHullRecipeMaterialLadder pins the data rule "quantities are
// monotonically sensible": each hull's recipe must burn more mineral value
// than the next-cheaper one (a barge costs more minerals than a sloop).
// The ladder order follows the shipped reference-price ladder and every
// shipped hull must appear in it.
func TestHullRecipeMaterialLadder(t *testing.T) {
	c := MustLoad()
	ladder := []ShipID{
		"ship-rat-scuttler", "ship-sloop", "ship-npc-patrol", "ship-cutlass",
		"ship-rat-marauder", "ship-npc-hauler", "ship-hauler", "ship-barge",
		"ship-rat-hierarch",
	}
	if len(ladder) != len(c.Ships) {
		inLadder := map[ShipID]bool{}
		for _, id := range ladder {
			inLadder[id] = true
		}
		var missing []string
		for _, id := range keys(c.Ships) {
			if !inLadder[id] {
				missing = append(missing, string(id))
			}
		}
		t.Fatalf("ladder covers %d of %d shipped hulls, missing from ladder: %v",
			len(ladder), len(c.Ships), missing)
	}
	costOf := func(product ShipID) float64 {
		for _, r := range c.RecipesInOrder() {
			if r.Product == ItemID(product) {
				return recipeMaterialCost(t, c, r)
			}
		}
		t.Fatalf("no recipe produces %s", product)
		return 0
	}
	prevID, prev := ShipID("(none)"), 0.0
	for _, id := range ladder {
		cost := costOf(id)
		if cost <= prev {
			t.Errorf("hull ladder: %s costs %v, not more than %s at %v", id, cost, prevID, prev)
		}
		prevID, prev = id, cost
	}
}

// TestRecipeIterationIsCanonical pins RecipesInOrder: recipe iteration is
// sorted by ID, so UI, sim and test output stay deterministic.
func TestRecipeIterationIsCanonical(t *testing.T) {
	c := MustLoad()
	got := c.RecipesInOrder()
	if len(got) != len(c.Recipes) {
		t.Fatalf("RecipesInOrder returned %d of %d recipes", len(got), len(c.Recipes))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].ID >= got[i].ID {
			t.Fatalf("RecipesInOrder not sorted by id: %q before %q", got[i-1].ID, got[i].ID)
		}
	}
}
