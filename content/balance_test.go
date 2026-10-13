package content

import (
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
