package content

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Shell weapons: two families (autocannon, artillery) over three calibres
// and four shell damage profiles per calibre.
// ---------------------------------------------------------------------------

// shellClasses is the calibre vocabulary: shells are keyed by size, and both
// gun families at a calibre share the shells (EVE-faithful).
var shellClasses = []string{"small", "medium", "large"}

// TestShellRecipesCoverEveryCalibre pins full manufacturing coverage: every
// shell of every calibre has a recipe that produces it.
func TestShellRecipesCoverEveryCalibre(t *testing.T) {
	c := MustLoad()
	for _, class := range shellClasses {
		shells := 0
		for id, it := range c.Items {
			if it.Category != CatAmmo || it.AmmoClass != class {
				continue
			}
			shells++
			found := false
			for _, r := range c.Recipes {
				if r.Product == id {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("shell %s (%s) has no recipe", id, class)
			}
		}
		if shells != 4 {
			t.Errorf("%s calibre has %d shells, want 4 (one per damage type)", class, shells)
		}
	}
}

// TestGunFamiliesAndCalibres pins the two-family table: each calibre has one
// autocannon (fast cycle, high tracking, short reach) and one artillery
// (slow cycle, low tracking, long reach, heavier volley), both loading the
// shells of their own calibre and both cap-free.
func TestGunFamiliesAndCalibres(t *testing.T) {
	c := MustLoad()
	for _, class := range shellClasses {
		ac, ok := c.Modules[ModuleID("mod-autocannon-"+class[:1])]
		if !ok {
			t.Fatalf("no autocannon of calibre %s", class)
		}
		ar, ok := c.Modules[ModuleID("mod-artillery-"+class[:1])]
		if !ok {
			t.Fatalf("no artillery of calibre %s", class)
		}
		for _, m := range []*ModuleDef{ac, ar} {
			if m.Weapon == nil {
				t.Fatalf("%s has no weapon block", m.ID)
			}
			if m.Weapon.AmmoClass != class {
				t.Errorf("%s ammo class = %q, want %q", m.ID, m.Weapon.AmmoClass, class)
			}
			if m.ActivationCost != 0 {
				t.Errorf("%s activation cost = %v, want 0 (projectile identity)", m.ID, m.ActivationCost)
			}
			if m.Weapon.ShotTime <= 0 || m.Weapon.ShotTime != m.ActivationTime {
				t.Errorf("%s shot/activation time = %v/%v, want one positive cycle", m.ID, m.Weapon.ShotTime, m.ActivationTime)
			}
		}
		if ar.Weapon.ShotTime <= ac.Weapon.ShotTime {
			t.Errorf("%s cycle %v must be slower than %s cycle %v", ar.ID, ar.Weapon.ShotTime, ac.ID, ac.Weapon.ShotTime)
		}
		if ar.Weapon.Tracking >= ac.Weapon.Tracking {
			t.Errorf("%s tracking %v must be lower than %s tracking %v", ar.ID, ar.Weapon.Tracking, ac.ID, ac.Weapon.Tracking)
		}
		// Render side splits the families at a 6 s cycle (render/overlay.go
		// heavyCycle): autocannons rattle, artillery blooms.
		if ac.Weapon.ShotTime >= 6 || ar.Weapon.ShotTime < 6 {
			t.Errorf("%s/%s cycles %v/%v straddle the 6 s artillery render threshold",
				ac.ID, ar.ID, ac.Weapon.ShotTime, ar.Weapon.ShotTime)
		}
		if ar.Weapon.Optimal <= ac.Weapon.Optimal {
			t.Errorf("%s optimal %v must exceed %s optimal %v", ar.ID, ar.Weapon.Optimal, ac.ID, ac.Weapon.Optimal)
		}
		// Per-volley damage: the artillery throws a heavier shot out of the
		// same shell (its damage multiplier is the family's share).
		if ar.Weapon.Mult() <= ac.Weapon.Mult() {
			t.Errorf("%s multiplier %v must exceed %s multiplier %v", ar.ID, ar.Weapon.Mult(), ac.ID, ac.Weapon.Mult())
		}
	}
}

// TestNpcFitsMatchHullCalibre pins the hull-to-size fitting: the small rat
// and the patrol cutter load small guns, the marauder medium, the hierarch
// large. Every fitted gun's class must also have shells to load.
func TestNpcFitsMatchHullCalibre(t *testing.T) {
	c := MustLoad()
	want := map[NpcID]string{
		"npc-rat-scuttler": "small",
		"npc-rat-marauder": "medium",
		"npc-rat-hierarch": "large",
		"npc-guard-patrol": "small",
	}
	for id, class := range want {
		npc, ok := c.NPCs[id]
		if !ok {
			t.Fatalf("missing npc %s", id)
		}
		guns := 0
		for _, mid := range npc.Fit {
			m, ok := c.Modules[mid]
			if !ok || m.Weapon == nil {
				continue
			}
			guns++
			if m.Weapon.AmmoClass != class {
				t.Errorf("%s fits %s (ammo class %q), want the %s calibre", id, mid, m.Weapon.AmmoClass, class)
			}
		}
		if guns == 0 {
			t.Errorf("%s fits no gun", id)
		}
	}
}

// TestValidateRejectsRetiredShellIDs: the pre-rename ids never come back —
// no author may re-add them, and a save carrying them is migrated instead.
func TestValidateRejectsRetiredShellIDs(t *testing.T) {
	c := &Content{
		Items: map[ItemID]*ItemDef{
			"charge-emp": {ID: "charge-emp", Name: "EMP Charge", Category: CatAmmo,
				Volume: 0.01, BasePrice: 1, AmmoClass: "small"},
		},
		Modules: map[ModuleID]*ModuleDef{
			"mod-emp-turret": {ID: "mod-emp-turret", Name: "EMP Turret", Slot: SlotHigh,
				Kind: KindWeapon, Weapon: &WeaponAttrs{Damage: Damage{EM: 1}, ShotTime: 1}},
		},
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("retired ids validated clean")
	}
	for _, want := range []string{"items/charge-emp: id is retired", "modules/mod-emp-turret: id is retired"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("validation error missing %q:\n%v", want, err)
		}
	}
}

// TestRespawnAmmoIsOnTheLocalBook pins the requisition path: a respawned NPC
// buys its 1,000-round shell grant from the station book of the system it
// died in (sim npcCharges/buyChargeBasket), so every system whose belt or
// anomaly spawns rats must list at least one shell of each local raider's
// calibre on its founding book — otherwise the replacement spawns dry.
func TestRespawnAmmoIsOnTheLocalBook(t *testing.T) {
	c := MustLoad()
	for sysID, sys := range c.Systems {
		var classes = map[string]bool{}
		for _, cel := range sys.Celestials {
			for _, sp := range cel.Spawns {
				if npc, ok := c.NPCs[sp.NpcID]; ok {
					for _, mid := range npc.Fit {
						if m, ok := c.Modules[mid]; ok && m.Weapon != nil && m.Weapon.AmmoClass != "" {
							classes[m.Weapon.AmmoClass] = true
						}
					}
				}
			}
			if cel.Anomaly != nil {
				for _, sp := range cel.Anomaly.Spawns {
					if npc, ok := c.NPCs[sp.NpcID]; ok {
						for _, mid := range npc.Fit {
							if m, ok := c.Modules[mid]; ok && m.Weapon != nil && m.Weapon.AmmoClass != "" {
								classes[m.Weapon.AmmoClass] = true
							}
						}
					}
				}
			}
		}
		if len(classes) == 0 {
			continue
		}
		for _, cel := range sys.Celestials {
			if cel.Station == nil || len(cel.Station.Market) == 0 {
				continue // no founding book: nothing can be requisitioned here
			}
			listed := map[string]bool{}
			for _, o := range cel.Station.Market {
				if it, ok := c.Items[o.Item]; ok && it.AmmoClass != "" && o.Side != "buy" {
					listed[it.AmmoClass] = true
				}
			}
			for class := range classes {
				if !listed[class] {
					t.Errorf("%s: station %s lists no %s shell for its local rats", sysID, cel.ID, class)
				}
			}
		}
	}
}
