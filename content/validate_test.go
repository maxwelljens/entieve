package content

import (
	"strings"
	"testing"
)

// TestValidateRealContent is the critical gate: the shipped embedded data
// must load AND validate with zero problems.
func TestValidateRealContent(t *testing.T) {
	c, err := Load(FS())
	if err != nil {
		t.Fatalf("Load(FS()): unexpected error:\n%v", err)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("embedded content does not validate:\n%v", err)
	}
}

// baseContent builds a minimal, fully valid in-memory world used as the
// starting point for negative validation cases.
func baseContent() *Content {
	return &Content{
		Items: map[ItemID]*ItemDef{
			"min-fer":     {ID: "min-fer", Name: "Ferrite", Category: CatMineral, Volume: 0.01, BasePrice: 6},
			"ore-rock":    {ID: "ore-rock", Name: "Test Rock", Category: CatOre, Volume: 1, BasePrice: 20, RefineYield: map[ItemID]float64{"min-fer": 0.5}},
			"charge-cell": {ID: "charge-cell", Name: "Cell", Category: CatCharge, Volume: 0.01, BasePrice: 100},
			"charge-bolt": {ID: "charge-bolt", Name: "Bolt", Category: CatAmmo, Volume: 0.01, BasePrice: 10, Damage: &Damage{EM: 10}, AmmoClass: "light"},
			"loot-bit":    {ID: "loot-bit", Name: "Bit", Category: CatLoot, Volume: 0.1, BasePrice: 1000},
		},
		Ships: map[ShipID]*ShipDef{
			"ship-hull": {
				ID: "ship-hull", Name: "Hull", Class: "frigate",
				Slots:  Slots{High: 2, Mid: 2, Low: 2},
				Shield: 100, Armor: 100, Structure: 100,
				Resists:   Resists{EM: 0.2, Thermal: 0.2, Kinetic: 0.2, Explosive: 0.2},
				Capacitor: 300, Recharge: 200,
				MaxSpeed: 300, Agility: 0.5, Mass: 1000000, SigRadius: 40,
				Cargo: 400, WarpSpeed: 3, MaxTargetRange: 40000, MaxTargets: 3, ScanResolution: 500,
			},
		},
		Modules: map[ModuleID]*ModuleDef{
			"mod-gun": {
				ID: "mod-gun", Name: "Gun", Slot: SlotHigh, Kind: KindWeapon,
				ActivationCost: 10, ActivationTime: 2, Price: 10000,
				Weapon: &WeaponAttrs{Damage: Damage{EM: 10}, Optimal: 8000, Falloff: 4000,
					Tracking: 0.4, AmmoClass: "light", ShotTime: 2, BayM3: 0.1},
			},
			"mod-engine": {
				ID: "mod-engine", Name: "Engine", Slot: SlotMid, Kind: KindAfterburner,
				ActivationCost: 20, ActivationTime: 5, Price: 20000,
				Boost: &BoostAttrs{SpeedMultiplier: 1.5},
			},
			"mod-plate": {
				ID: "mod-plate", Name: "Plate", Slot: SlotLow, Kind: KindArmorPlate,
				Price: 15000, Passive: &PassiveAttrs{ArmorHP: 200},
			},
		},
		NPCs: map[NpcID]*NpcDef{
			"npc-rat": {
				ID: "npc-rat", Name: "Rat", Faction: "Test", Ship: "ship-hull",
				Fit:           []ModuleID{"mod-gun", "mod-engine", "mod-plate"},
				AggroRange:    10000,
				OrbitRange:    2000,
				FleeThreshold: 0.1, Bounty: 1000,
				Loot: []LootDrop{{Item: "loot-bit", Chance: 0.5, MinQty: 1, MaxQty: 2}},
			},
		},
		Systems: map[SystemID]*SystemDef{
			"sys-a": {
				ID: "sys-a", Name: "A", Security: "high", Constellation: "C", Region: "R",
				Celestials: []CelestialDef{
					{ID: "cel-a-star", Name: "Star", Kind: CelStar, Radius: 1e8},
					{ID: "cel-a-station", Name: "Station", Kind: CelStation, Radius: 500,
						Station: &StationDef{
							Services: []string{"repair", "market", "refine", "fitting", "industry"},
							Market: []MarketOrderDef{
								{Item: "min-fer", Qty: 10, Price: 6},
								{Item: "ship-hull", Qty: 2, Price: 350000},
							},
						}},
					{ID: "cel-a-belt", Name: "Belt", Kind: CelBelt, Radius: 1000,
						Belt:   &BeltDef{Asteroids: []AsteroidDef{{Ore: "ore-rock", Count: 3, Volume: 1000}}},
						Spawns: []SpawnDef{{NpcID: "npc-rat", Count: 1, RespawnSec: 60}}},
				},
			},
		},
		Worlds: map[WorldID]*WorldDef{
			"world-showcase": {
				ID: "world-showcase", Name: "Showcase",
				Start: StartDef{
					System: "sys-a", Ship: "ship-hull",
					Fit:    []ModuleID{"mod-gun", "mod-engine", "mod-plate"},
					Cargo:  []CargoStack{{Item: "min-fer", Qty: 5}},
					Wallet: 1000,
				},
				Scenario: checkpoints(10),
			},
		},
		Recipes: map[RecipeID]*RecipeDef{
			"rec-ship-hull": {ID: "rec-ship-hull", Product: "ship-hull", Qty: 1,
				Inputs: []RecipeInput{{Item: "min-fer", Qty: 100}}, Time: 120, Family: FamilyHulls},
			"rec-mod-gun": {ID: "rec-mod-gun", Product: "mod-gun", Qty: 1,
				Inputs: []RecipeInput{{Item: "min-fer", Qty: 50}}, Time: 60, Family: FamilyModules},
			"rec-mod-engine": {ID: "rec-mod-engine", Product: "mod-engine", Qty: 1,
				Inputs: []RecipeInput{{Item: "min-fer", Qty: 60}}, Time: 45, Family: FamilyModules},
			"rec-mod-plate": {ID: "rec-mod-plate", Product: "mod-plate", Qty: 1,
				Inputs: []RecipeInput{{Item: "min-fer", Qty: 40}}, Time: 35, Family: FamilyModules},
			"rec-charge-cell": {ID: "rec-charge-cell", Product: "charge-cell", Qty: 100,
				Inputs: []RecipeInput{{Item: "min-fer", Qty: 500}}, Time: 45, Family: FamilyCharges},
			"rec-charge-bolt": {ID: "rec-charge-bolt", Product: "charge-bolt", Qty: 100,
				Inputs: []RecipeInput{{Item: "min-fer", Qty: 400}}, Time: 35, Family: FamilyCharges},
		},
		Universe: &UniverseDef{Region: "R", Constellation: "C", StartSystem: "sys-a"},
	}
}

// stationMarket returns the founding-order list of baseContent's station
// (the market orders live on the station since the v2 market schema).
func stationMarket(c *Content) *[]MarketOrderDef {
	return &c.Systems["sys-a"].Celestials[1].Station.Market
}

// checkpoints builds n valid, uniquely named scenario checkpoints.
func checkpoints(n int) []Checkpoint {
	out := make([]Checkpoint, n)
	for i := range out {
		out[i] = Checkpoint{
			Name:    "Checkpoint " + itoa(i),
			Feature: "feature-" + itoa(i),
			Detail:  "observe it",
		}
	}
	return out
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// loadFixtureErr loads a testdata fixture directory and returns the first
// error from Load or Validate (whichever fires).
func loadFixtureErr(t *testing.T, dir string) error {
	t.Helper()
	c, err := LoadDir(dir)
	if err != nil {
		return err
	}
	return c.Validate()
}

// TestValidateNegativeCases runs at least a dozen broken-content fixtures
// (JSON directories under testdata/ and in-code mutations) through the
// loader/validator and checks each produces its expected error substring.
func TestValidateNegativeCases(t *testing.T) {
	cases := []struct {
		name string
		// dir, when non-empty, loads a fixture directory under testdata/.
		dir string
		// mutate edits the in-memory baseContent (when dir is empty).
		mutate func(*Content)
		want   string
	}{
		// ---- JSON fixture directories (loader + validator path) ----
		{name: "duplicate id across files", dir: "testdata/duplicate_id", want: "duplicate item id \"min-dup\""},
		{name: "parse error names file", dir: "testdata/bad_json", want: "items/items.json"},
		{name: "broken gate destination", dir: "testdata/broken_gate", want: "gate destination celestial"},
		{name: "non-reciprocal gate", dir: "testdata/non_reciprocal_gate", want: "no reciprocal gate"},
		{name: "unreachable system", dir: "testdata/unreachable", want: "unreachable from start system"},

		// ---- in-code mutations of a valid world ----
		{
			name: "unknown module in fit",
			mutate: func(c *Content) {
				c.Worlds["world-showcase"].Start.Fit = append(c.Worlds["world-showcase"].Start.Fit, "mod-ghost")
			},
			want: "unknown module \"mod-ghost\"",
		},
		{
			name: "fit slot overflow",
			mutate: func(c *Content) {
				c.Worlds["world-showcase"].Start.Fit = []ModuleID{"mod-gun", "mod-gun", "mod-gun", "mod-engine", "mod-plate"}
			},
			want: "exceed hull ship-hull high slots (2)",
		},
		{
			name: "module in wrong slot class",
			mutate: func(c *Content) {
				c.Worlds["world-showcase"].Start.Fit = []ModuleID{"mod-engine", "mod-gun"}
			},
			want: "fit must list high, then mid, then low",
		},
		{
			name: "refine yield above 1",
			mutate: func(c *Content) {
				c.Items["ore-rock"].RefineYield["min-fer"] = 1.5
			},
			want: "refine yield for min-fer is 1.5",
		},
		{
			name: "resist above 1",
			mutate: func(c *Content) {
				c.Ships["ship-hull"].Resists.EM = 1.2
			},
			want: "resist em 1.2 outside [0,1)",
		},
		{
			name: "unknown npc in spawn",
			mutate: func(c *Content) {
				c.Systems["sys-a"].Celestials[2].Spawns[0].NpcID = "npc-ghost"
			},
			want: "unknown npc \"npc-ghost\"",
		},
		{
			name: "world references unknown system",
			mutate: func(c *Content) {
				c.Worlds["world-showcase"].Start.System = "sys-ghost"
			},
			want: "unknown start system \"sys-ghost\"",
		},
		{
			name: "showcase missing checkpoints",
			mutate: func(c *Content) {
				c.Worlds["world-showcase"].Scenario = checkpoints(2)
			},
			want: "need at least 10",
		},
		{
			name: "unknown market item",
			mutate: func(c *Content) {
				m := stationMarket(c)
				*m = append(*m, MarketOrderDef{Item: "item-ghost", Qty: 1, Price: 10})
			},
			want: "is not an item, ship, or module",
		},
		{
			name: "unknown item category",
			mutate: func(c *Content) {
				c.Items["min-fer"].Category = "material"
			},
			want: "unknown category \"material\"",
		},
		{
			name: "empty required field",
			mutate: func(c *Content) {
				c.Ships["ship-hull"].Name = ""
			},
			want: "ships/ship-hull: missing name",
		},
		{
			name: "negative ship hp",
			mutate: func(c *Content) {
				c.Ships["ship-hull"].Shield = -1
			},
			want: "negative shield -1",
		},
		{
			// Empty origin stays allowed (see TestBaseContentIsValid); the
			// vocabulary itself is checked whenever a hull sets one.
			name: "unknown hull origin",
			mutate: func(c *Content) {
				c.Ships["ship-hull"].Origin = "martian"
			},
			want: "unknown origin \"martian\"",
		},
		{
			// The maker vocabulary is the corp-* roster; an unknown slug is
			// rejected on every manufactured def kind.
			name:   "unknown ship maker",
			mutate: func(c *Content) { c.Ships["ship-hull"].Maker = "corp-ghost" },
			want:   "unknown maker \"corp-ghost\"",
		},
		{
			name:   "unknown module maker",
			mutate: func(c *Content) { c.Modules["mod-gun"].Maker = "megacorp" },
			want:   "unknown maker \"megacorp\"",
		},
		{
			name:   "unknown item maker",
			mutate: func(c *Content) { c.Items["charge-bolt"].Maker = "corp-ghost" },
			want:   "unknown maker \"corp-ghost\"",
		},
		{
			name: "loot chance out of range",
			mutate: func(c *Content) {
				c.NPCs["npc-rat"].Loot[0].Chance = 1.5
			},
			want: "chance 1.5 outside [0,1]",
		},
		{
			name: "loot min qty above max qty",
			mutate: func(c *Content) {
				c.NPCs["npc-rat"].Loot[0].MinQty = 3
				c.NPCs["npc-rat"].Loot[0].MaxQty = 1
			},
			want: "min_qty 3 > max_qty 1",
		},
		{
			name: "unknown npc ship",
			mutate: func(c *Content) {
				c.NPCs["npc-rat"].Ship = "ship-ghost"
			},
			want: "unknown ship \"ship-ghost\"",
		},
		{
			name: "unknown station service",
			mutate: func(c *Content) {
				c.Systems["sys-a"].Celestials[1].Station.Services =
					append(c.Systems["sys-a"].Celestials[1].Station.Services, "banking")
			},
			want: "unknown station service \"banking\"",
		},
		{
			name: "weapon ammo class matches nothing",
			mutate: func(c *Content) {
				c.Modules["mod-gun"].Weapon.AmmoClass = "heavy"
			},
			want: "weapon ammo class \"heavy\" matches no ammo or charge item",
		},
		{
			name: "gate to unknown system",
			mutate: func(c *Content) {
				sys := c.Systems["sys-a"]
				sys.Celestials = append(sys.Celestials, CelestialDef{
					ID: "cel-a-gate", Name: "Gate", Kind: CelGate,
					Gate: &GateDef{ToSystem: "sys-z", ToCel: "cel-x"},
				})
			},
			want: "gate destination system \"sys-z\" does not exist",
		},
		{
			name: "universe inline systems forbidden",
			mutate: func(c *Content) {
				c.Universe.Systems = []SystemDef{{ID: "sys-inline", Name: "Inline"}}
			},
			want: "systems must be empty",
		},
		{
			name: "missing showcase world",
			mutate: func(c *Content) {
				delete(c.Worlds, "world-showcase")
			},
			want: "missing required world \"world-showcase\"",
		},
		{
			name: "negative wallet",
			mutate: func(c *Content) {
				c.Worlds["world-showcase"].Start.Wallet = -1
			},
			want: "negative wallet -1",
		},
		{
			name: "unknown cargo item",
			mutate: func(c *Content) {
				c.Worlds["world-showcase"].Start.Cargo[0].Item = "item-ghost"
			},
			want: "unknown item \"item-ghost\"",
		},
		{
			name: "unknown belt ore",
			mutate: func(c *Content) {
				c.Systems["sys-a"].Celestials[2].Belt.Asteroids[0].Ore = "min-fer"
			},
			want: "not category ore",
		},
		{
			name: "negative spawn respawn timer",
			mutate: func(c *Content) {
				c.Systems["sys-a"].Celestials[2].Spawns[0].RespawnSec = -5
			},
			want: "respawn_sec -5 must be >= 0",
		},
		{
			name: "unknown security",
			mutate: func(c *Content) {
				c.Systems["sys-a"].Security = "med"
			},
			want: "unknown security \"med\"",
		},
		{
			name: "duplicate celestial id",
			mutate: func(c *Content) {
				sys := c.Systems["sys-a"]
				sys.Celestials = append(sys.Celestials, CelestialDef{ID: "cel-a-star", Name: "Impostor", Kind: CelPlanet})
			},
			want: "duplicate celestial id \"cel-a-star\"",
		},
		{
			name: "market qty below one",
			mutate: func(c *Content) {
				(*stationMarket(c))[0].Qty = 0
			},
			want: "qty 0 must be >= 1",
		},
		{
			name: "negative market price",
			mutate: func(c *Content) {
				(*stationMarket(c))[0].Price = -5
			},
			want: "price -5 must be >= 0",
		},
		{
			name: "system-level market block rejected",
			mutate: func(c *Content) {
				c.Systems["sys-a"].Market = []MarketOrderDef{{Item: "min-fer", Qty: 1, Price: 6}}
			},
			want: "system-level market block is not allowed",
		},
		{
			name: "market orders without market service",
			mutate: func(c *Content) {
				c.Systems["sys-a"].Celestials[1].Station.Services = []string{"repair"}
			},
			want: "without the market service",
		},
		{
			name: "unknown market side",
			mutate: func(c *Content) {
				m := stationMarket(c)
				*m = append(*m, MarketOrderDef{Item: "charge-bolt", Qty: 1, Price: 5, Side: "hold"})
			},
			want: `unknown side "hold"`,
		},
		{
			name: "crossing founding orders",
			mutate: func(c *Content) {
				m := stationMarket(c)
				*m = append(*m, MarketOrderDef{Item: "min-fer", Qty: 5, Price: 6, Side: "buy"})
			},
			want: "crosses its ask",
		},
		{
			name: "station id shared across systems",
			mutate: func(c *Content) {
				sysB := *c.Systems["sys-a"]
				sysB.ID = "sys-b"
				sysB.Celestials = []CelestialDef{
					{ID: "cel-a-station", Name: "Impostor", Kind: CelStation, Radius: 500,
						Station: &StationDef{Services: []string{"market"}}},
				}
				c.Systems["sys-b"] = &sysB
			},
			want: "is already used by system sys-a",
		},
		{
			name: "module kind missing attribute block",
			mutate: func(c *Content) {
				c.Modules["mod-gun"].Weapon = nil
			},
			want: "requires a weapon block",
		},
		{
			name: "checkpoint missing feature",
			mutate: func(c *Content) {
				c.Worlds["world-showcase"].Scenario[3].Feature = ""
			},
			want: "checkpoint[3] missing feature",
		},
		{
			name: "recipe missing id",
			mutate: func(c *Content) {
				c.Recipes[""] = &RecipeDef{Product: "ship-hull", Qty: 1,
					Inputs: []RecipeInput{{Item: "min-fer", Qty: 1}}, Time: 10, Family: FamilyHulls}
			},
			want: "recipes: missing id",
		},
		{
			name: "recipe id without rec prefix",
			mutate: func(c *Content) {
				c.Recipes["ship-hull-plan"] = &RecipeDef{ID: "ship-hull-plan", Product: "ship-hull", Qty: 1,
					Inputs: []RecipeInput{{Item: "min-fer", Qty: 1}}, Time: 10, Family: FamilyHulls}
			},
			want: "id must use the rec- prefix",
		},
		{
			name: "recipe product resolves to nothing",
			mutate: func(c *Content) {
				c.Recipes["rec-ghost"] = &RecipeDef{ID: "rec-ghost", Product: "ship-ghost", Qty: 1,
					Inputs: []RecipeInput{{Item: "min-fer", Qty: 1}}, Time: 10, Family: FamilyHulls}
			},
			want: `product "ship-ghost" is not an item, ship, or module`,
		},
		{
			name: "recipe product is loot",
			mutate: func(c *Content) {
				c.Recipes["rec-loot"] = &RecipeDef{ID: "rec-loot", Product: "loot-bit", Qty: 1,
					Inputs: []RecipeInput{{Item: "min-fer", Qty: 1}}, Time: 10, Family: FamilyModules}
			},
			want: `product "loot-bit" is category "loot"`,
		},
		{
			name: "recipe input is ore",
			mutate: func(c *Content) {
				c.Recipes["rec-ship-hull"].Inputs[0].Item = "ore-rock"
			},
			want: `inputs[0] item "ore-rock" is category "ore"`,
		},
		{
			name: "recipe input unknown item",
			mutate: func(c *Content) {
				c.Recipes["rec-ship-hull"].Inputs[0].Item = "min-ghost"
			},
			want: `inputs[0] references unknown item "min-ghost"`,
		},
		{
			name: "recipe input without qty",
			mutate: func(c *Content) {
				c.Recipes["rec-ship-hull"].Inputs[0].Qty = 0
			},
			want: "inputs[0] qty 0 must be > 0",
		},
		{
			name: "recipe without inputs",
			mutate: func(c *Content) {
				c.Recipes["rec-ship-hull"].Inputs = nil
			},
			want: "rec-ship-hull: no inputs",
		},
		{
			name: "recipe zero product qty",
			mutate: func(c *Content) {
				c.Recipes["rec-mod-gun"].Qty = 0
			},
			want: "qty 0 must be >= 1",
		},
		{
			name: "recipe zero time",
			mutate: func(c *Content) {
				c.Recipes["rec-mod-gun"].Time = 0
			},
			want: "time 0 must be > 0",
		},
		{
			name: "recipe unknown family",
			mutate: func(c *Content) {
				c.Recipes["rec-mod-gun"].Family = "weapons"
			},
			want: `unknown family "weapons" (want hulls|modules|charges)`,
		},
		{
			name: "recipe missing product",
			mutate: func(c *Content) {
				c.Recipes["rec-mod-gun"].Product = ""
			},
			want: "rec-mod-gun: missing product",
		},
		{
			name: "ship without recipe",
			mutate: func(c *Content) {
				delete(c.Recipes, "rec-ship-hull")
			},
			want: `no recipe produces ship "ship-hull"`,
		},
		{
			name: "charge item without recipe",
			mutate: func(c *Content) {
				delete(c.Recipes, "rec-charge-bolt")
			},
			want: `no recipe produces item "charge-bolt"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			if tc.dir != "" {
				err = loadFixtureErr(t, tc.dir)
			} else {
				c := baseContent()
				tc.mutate(c)
				err = c.Validate()
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error does not contain %q:\n%s", tc.want, err)
			}
		})
	}
}

// TestBaseContentIsValid guards the fixture baseline itself.
func TestBaseContentIsValid(t *testing.T) {
	if err := baseContent().Validate(); err != nil {
		t.Fatalf("baseContent must validate clean:\n%v", err)
	}
}
