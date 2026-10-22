package content

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Shell weapons: two families (autocannon, artillery) over three calibres,
// six shell families (family-calibre m³ volumes) and four shells per family.
// ---------------------------------------------------------------------------

// shellFamilies is the family-calibre table: one volume per family, shared by
// its four damage profiles, and the gun that loads it.
var shellFamilies = []struct {
	class  string
	volume float64 // m³ per shell
	gun    ModuleID
}{
	{"small-autocannon", 0.005, "mod-autocannon-s"},
	{"small-artillery", 0.008, "mod-artillery-s"},
	{"medium-autocannon", 0.01, "mod-autocannon-m"},
	{"medium-artillery", 0.025, "mod-artillery-m"},
	{"large-autocannon", 0.022, "mod-autocannon-l"},
	{"large-artillery", 0.033, "mod-artillery-l"},
}

// TestShellRecipesCoverEveryFamily pins full manufacturing coverage: every
// shell of every family has a recipe that produces it, four shells per family.
func TestShellRecipesCoverEveryFamily(t *testing.T) {
	c := MustLoad()
	for _, fam := range shellFamilies {
		shells := 0
		for id, it := range c.Items {
			if it.Category != CatAmmo || it.AmmoClass != fam.class {
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
				t.Errorf("shell %s (%s) has no recipe", id, fam.class)
			}
		}
		if shells != 4 {
			t.Errorf("%s has %d shells, want 4 (one per damage type)", fam.class, shells)
		}
	}
}

// TestShellFamilyVolumes pins the authored family-calibre table: one volume
// per family, shared by all four of its shells (a bay's whole-round count is
// family-wide), and the cap cell at its old 0.01 m³.
func TestShellFamilyVolumes(t *testing.T) {
	c := MustLoad()
	for _, fam := range shellFamilies {
		for id, it := range c.Items {
			if it.AmmoClass != fam.class {
				continue
			}
			if it.Volume != fam.volume {
				t.Errorf("%s volume = %v, want the %s family's %v", id, it.Volume, fam.class, fam.volume)
			}
		}
	}
	if cell, ok := c.Items["charge-cap-cell"]; !ok || cell.Volume != 0.01 {
		t.Errorf("charge-cap-cell volume = %v, want 0.01", cell.Volume)
	}
}

// TestGunFamiliesAndCalibres pins the two-family table: each calibre has one
// autocannon (fast cycle, high tracking, short reach) and one artillery
// (slow cycle, low tracking, long reach, heavier volley), both loading the
// shells of their own family and both cap-free.
func TestGunFamiliesAndCalibres(t *testing.T) {
	c := MustLoad()
	for _, size := range []string{"s", "m", "l"} {
		ac, ok := c.Modules[ModuleID("mod-autocannon-"+size)]
		if !ok {
			t.Fatalf("no autocannon of size %s", size)
		}
		ar, ok := c.Modules[ModuleID("mod-artillery-"+size)]
		if !ok {
			t.Fatalf("no artillery of size %s", size)
		}
		for _, m := range []*ModuleDef{ac, ar} {
			if m.Weapon == nil {
				t.Fatalf("%s has no weapon block", m.ID)
			}
			if m.ActivationCost != 0 {
				t.Errorf("%s activation cost = %v, want 0 (projectile identity)", m.ID, m.ActivationCost)
			}
			if m.Weapon.ShotTime <= 0 || m.Weapon.ShotTime != m.ActivationTime {
				t.Errorf("%s shot/activation time = %v/%v, want one positive cycle", m.ID, m.Weapon.ShotTime, m.ActivationTime)
			}
			if m.Weapon.BayM3 <= 0 {
				t.Errorf("%s has no shell bay", m.ID)
			}
		}
		if !strings.HasSuffix(ac.Weapon.AmmoClass, "-autocannon") {
			t.Errorf("%s ammo class = %q, want an autocannon family", ac.ID, ac.Weapon.AmmoClass)
		}
		if !strings.HasSuffix(ar.Weapon.AmmoClass, "-artillery") {
			t.Errorf("%s ammo class = %q, want an artillery family", ar.ID, ar.Weapon.AmmoClass)
		}
		acCal := strings.TrimSuffix(ac.Weapon.AmmoClass, "-autocannon")
		arCal := strings.TrimSuffix(ar.Weapon.AmmoClass, "-artillery")
		if acCal != arCal {
			t.Errorf("%s/%s load different calibres (%q/%q)", ac.ID, ar.ID, ac.Weapon.AmmoClass, ar.Weapon.AmmoClass)
		}
		// Both guns of the size share the calibre's name (small/medium/large).
		wantSize := map[string]string{"s": "small", "m": "medium", "l": "large"}[size]
		if acCal != wantSize || arCal != wantSize {
			t.Errorf("%s/%s calibres = %q/%q, want %q", ac.ID, ar.ID, acCal, arCal, wantSize)
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
		// The small autocannon was retuned to a 10 km reach (optimal 4 km +
		// falloff 6 km); the medium/large are awaiting their own review.
		if ac.ID == "mod-autocannon-s" && ac.Weapon.Falloff != 6000 {
			t.Errorf("%s falloff = %v, want 6000", ac.ID, ac.Weapon.Falloff)
		}
		// Per-volley damage: the artillery throws a heavier shot out of the
		// same shell (its damage multiplier is the family's share).
		if ar.Weapon.Mult() <= ac.Weapon.Mult() {
			t.Errorf("%s multiplier %v must exceed %s multiplier %v", ar.ID, ar.Weapon.Mult(), ac.ID, ac.Weapon.Mult())
		}
	}
}

// TestBayCalibration pins the shell bays: each charge-fed gun carries a m³
// bay that holds whole rounds of its family, calibrated so sustained fire
// reloads about every two minutes (bay rounds × cycle time ≈ 120 s), and the
// cap booster holds 25 cells.
func TestBayCalibration(t *testing.T) {
	c := MustLoad()
	wantRounds := map[ModuleID]int{
		"mod-autocannon-s": 40, // 3 s × 40 = 120 s
		"mod-artillery-s":  13, // 9 s × 13 = 117 s
		"mod-autocannon-m": 33, // 3.6 s × 33 = 118.8 s
		"mod-artillery-m":  11, // 10.8 s × 11 = 118.8 s
		"mod-autocannon-l": 27, // 4.5 s × 27 = 121.5 s
		"mod-artillery-l":  9,  // 13.5 s × 9 = 121.5 s
	}
	for mid, want := range wantRounds {
		m := c.Modules[mid]
		vol := 0.0
		for _, it := range c.Items {
			if it.AmmoClass == m.Weapon.AmmoClass {
				vol = it.Volume
				break
			}
		}
		if got := m.BayRounds(vol); got != want {
			t.Errorf("%s bay %v m³ / %v m³ = %d rounds, want %d", mid, m.Weapon.BayM3, vol, got, want)
		}
		if secs := float64(want) * m.Weapon.ShotTime; secs < 110 || secs > 130 {
			t.Errorf("%s sustained fire = %v s per reload, want ~120 s", mid, secs)
		}
	}
	booster := c.Modules["mod-cap-booster"]
	if booster == nil || booster.Boost == nil || booster.Boost.BayM3 != 0.25 {
		t.Fatalf("cap booster bay = %+v, want 0.25 m³", booster)
	}
	if got := booster.BayRounds(c.Items["charge-cap-cell"].Volume); got != 25 {
		t.Errorf("cap booster holds %d cells, want 25", got)
	}
	// The arithmetic lives in one place: no shell volume holds nothing, and
	// a bayless module holds nothing whatever it is offered.
	if got := booster.BayRounds(0); got != 0 {
		t.Errorf("bay rounds of a volume-less shell = %d, want 0", got)
	}
	if pod := c.Modules["mod-rocket-pod"]; pod.BayRounds(0.005) != 0 {
		t.Errorf("bayless weapon holds %d rounds", pod.BayRounds(0.005))
	}
	// The rocket pod stays charge-free: no bay, no ammo class.
	if pod := c.Modules["mod-rocket-pod"]; pod.Weapon.AmmoClass != "" || pod.Weapon.BayM3 != 0 {
		t.Errorf("rocket pod = class %q bay %v, want charge-free", pod.Weapon.AmmoClass, pod.Weapon.BayM3)
	}
}

// TestNpcFitsMatchHullCalibre pins the hull-to-family fitting: the small rat
// and the patrol cutter load small autocannons, the marauder medium
// autocannons, the hierarch large artillery and large autocannons. Every
// fitted gun's family must also have shells to load.
func TestNpcFitsMatchHullCalibre(t *testing.T) {
	c := MustLoad()
	want := map[NpcID][]string{
		"npc-rat-scuttler": {"small-autocannon"},
		"npc-rat-marauder": {"medium-autocannon"},
		"npc-rat-hierarch": {"large-artillery", "large-autocannon"},
		"npc-guard-patrol": {"small-autocannon"},
	}
	for id, classes := range want {
		npc, ok := c.NPCs[id]
		if !ok {
			t.Fatalf("missing npc %s", id)
		}
		allowed := map[string]bool{}
		for _, cl := range classes {
			allowed[cl] = true
		}
		guns := 0
		for _, mid := range npc.Fit {
			m, ok := c.Modules[mid]
			if !ok || m.Weapon == nil {
				continue
			}
			guns++
			if !allowed[m.Weapon.AmmoClass] {
				t.Errorf("%s fits %s (ammo class %q), want one of %v", id, mid, m.Weapon.AmmoClass, classes)
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
			"charge-emp": {ID: "charge-emp", Name: "EMP Charge", Group: "Ammunition/Autocannon shells", Category: CatAmmo,
				Volume: 0.01, BasePrice: 1, AmmoClass: "small"},
		},
		Modules: map[ModuleID]*ModuleDef{
			"mod-emp-turret": {ID: "mod-emp-turret", Name: "EMP Turret", Group: "Modules/High/Turrets", Slot: SlotHigh,
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

// TestValidateRejectsBadBays: a charge-fed gun needs a bay that holds at
// least one whole round, a charge-free gun no bay at all, the cap booster a
// cell bay, and every shell family one shared volume.
func TestValidateRejectsBadBays(t *testing.T) {
	base := func() *Content {
		return &Content{
			Items: map[ItemID]*ItemDef{
				"ammo-a": {ID: "ammo-a", Name: "A", Group: "Ammunition/Autocannon shells", Category: CatAmmo, Volume: 0.01,
					BasePrice: 1, AmmoClass: "small-autocannon", Damage: &Damage{EM: 1}},
				"ammo-b": {ID: "ammo-b", Name: "B", Group: "Ammunition/Autocannon shells", Category: CatAmmo, Volume: 0.01,
					BasePrice: 1, AmmoClass: "small-autocannon", Damage: &Damage{EM: 1}},
				"charge-cap-cell": {ID: "charge-cap-cell", Name: "Cell", Group: "Ammunition/Cap cells", Category: CatCharge,
					Volume: 0.01, BasePrice: 1},
			},
			Modules: map[ModuleID]*ModuleDef{
				"mod-gun": {ID: "mod-gun", Name: "Gun", Group: "Modules/High/Turrets", Slot: SlotHigh, Kind: KindWeapon,
					Weapon: &WeaponAttrs{Damage: Damage{EM: 1}, ShotTime: 1,
						AmmoClass: "small-autocannon", BayM3: 0.1}},
				"mod-pod": {ID: "mod-pod", Name: "Pod", Group: "Modules/High/Launchers", Slot: SlotHigh, Kind: KindWeapon,
					Weapon: &WeaponAttrs{Damage: Damage{EM: 1}, ShotTime: 1}},
				"mod-boost": {ID: "mod-boost", Name: "Boost", Group: "Modules/Mid/Capacitor", Slot: SlotLow, Kind: KindCapBooster,
					Boost: &BoostAttrs{CapRestore: 1, BayM3: 0.25}},
			},
		}
	}
	cases := []struct {
		name string
		mut  func(*Content)
		want string
	}{
		{"no bay", func(c *Content) { c.Modules["mod-gun"].Weapon.BayM3 = 0 },
			"loads \"small-autocannon\" but declares no bay"},
		{"bay under one round", func(c *Content) { c.Modules["mod-gun"].Weapon.BayM3 = 0.005 },
			"holds no whole round"},
		{"charge-free gun with a bay", func(c *Content) { c.Modules["mod-pod"].Weapon.BayM3 = 0.1 },
			"needs no ammo but declares a bay"},
		{"booster without a bay", func(c *Content) { c.Modules["mod-boost"].Boost.BayM3 = 0 },
			"cap booster declares no cell bay"},
		{"family volume split", func(c *Content) { c.Items["ammo-b"].Volume = 0.02 },
			"one family = one calibre"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base()
			tc.mut(c)
			err := c.Validate()
			if err == nil {
				t.Fatal("validated clean")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error missing %q:\n%v", tc.want, err)
			}
		})
	}
	if err := base().Validate(); err != nil &&
		(strings.Contains(err.Error(), "bay") || strings.Contains(err.Error(), "ammo class")) {
		t.Fatalf("well-formed bays rejected: %v", err)
	}
}

// TestRespawnAmmoIsOnTheLocalBook pins the requisition path: a respawned NPC
// buys its 1,000-round shell grant from the station book of the system it
// died in (sim npcCharges/buyChargeBasket), so every system whose belt or
// anomaly spawns rats must list at least one shell of each local raider's
// family on its founding book — otherwise the replacement spawns dry.
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
