package sim

import (
	"math"
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// ---------------------------------------------------------------------------
// In-memory content fixtures: 2 systems, 1 station, a gate, a belt with
// rocks, an anomaly spawn, hulls, one module of every Kind, ammo, cap cells,
// ore + minerals with refine yields, and a market.
// ---------------------------------------------------------------------------

func testContent() *content.Content {
	dmg := func(em, th, ki, ex float64) *content.Damage {
		return &content.Damage{EM: em, Thermal: th, Kinetic: ki, Explosive: ex}
	}
	items := map[content.ItemID]*content.ItemDef{
		"ammo-emp-s": {ID: "ammo-emp-s", Name: "EMP Charge", Category: content.CatAmmo,
			Volume: 0.01, BasePrice: 5, AmmoClass: "small", Damage: dmg(10, 2, 0, 0)},
		"ammo-ap-s": {ID: "ammo-ap-s", Name: "AP Charge", Category: content.CatAmmo,
			Volume: 0.01, BasePrice: 6, AmmoClass: "small", Damage: dmg(0, 0, 12, 0)},
		"ammo-fusion-s": {ID: "ammo-fusion-s", Name: "Fusion Charge", Category: content.CatAmmo,
			Volume: 0.01, BasePrice: 7, AmmoClass: "small", Damage: dmg(0, 2, 0, 10)},
		"charge-cap-cell": {ID: "charge-cap-cell", Name: "Cap Booster Cell", Category: content.CatCharge,
			Volume: 0.01, BasePrice: 10},
		"ore-sterngrit": {ID: "ore-sterngrit", Name: "Sterngrit", Category: content.CatOre,
			Volume: 1, BasePrice: 20,
			RefineYield: map[content.ItemID]float64{"min-ferrite": 0.5, "min-nickel": 0.25}},
		"min-ferrite": {ID: "min-ferrite", Name: "Ferrite", Category: content.CatMineral,
			Volume: 0.1, BasePrice: 10},
		"min-nickel": {ID: "min-nickel", Name: "Nickel", Category: content.CatMineral,
			Volume: 0.1, BasePrice: 12},
		"loot-alloy-scrap": {ID: "loot-alloy-scrap", Name: "Alloy Scrap", Category: content.CatLoot,
			Volume: 0.5, BasePrice: 100},
		"loot-navdata": {ID: "loot-navdata", Name: "Nav Data", Category: content.CatLoot,
			Volume: 0.2, BasePrice: 250},
	}

	res05 := content.Resists{EM: 0.1, Thermal: 0.1, Kinetic: 0.1, Explosive: 0.1}
	ships := map[content.ShipID]*content.ShipDef{
		"ship-sloop": {
			ID: "ship-sloop", Name: "Sloop", Class: "frigate",
			Slots:  content.Slots{High: 4, Mid: 4, Low: 6},
			Shield: 500, Armor: 500, Structure: 400, Resists: res05,
			Capacitor: 400, Recharge: 200,
			MaxSpeed: 300, Agility: 0.4, Mass: 1e6, SigRadius: 40,
			Cargo: 400, OreHold: 0,
			WarpSpeed:      2,
			MaxTargetRange: 12000, MaxTargets: 2, ScanResolution: 500,
		},
		"ship-barge": {
			ID: "ship-barge", Name: "Ore Barge", Class: "barge",
			Slots:  content.Slots{High: 2, Mid: 2, Low: 2},
			Shield: 300, Armor: 300, Structure: 200, Resists: content.Resists{EM: 0.05, Thermal: 0.05, Kinetic: 0.05, Explosive: 0.05},
			Capacitor: 200, Recharge: 150,
			MaxSpeed: 120, Agility: 1.0, Mass: 2e6, SigRadius: 90,
			Cargo: 200, OreHold: 1000,
			WarpSpeed:      1.5,
			MaxTargetRange: 9000, MaxTargets: 1, ScanResolution: 300,
		},
		"ship-rat": {
			ID: "ship-rat", Name: "Rat Hull", Class: "rat",
			Slots:  content.Slots{High: 2, Mid: 3, Low: 2},
			Shield: 200, Armor: 100, Structure: 100, Resists: content.Resists{},
			Capacitor: 100, Recharge: 80,
			MaxSpeed: 250, Agility: 0.5, Mass: 5e5, SigRadius: 50,
			Cargo: 50, OreHold: 0,
			WarpSpeed:      1.5,
			MaxTargetRange: 8000, MaxTargets: 1, ScanResolution: 400,
		},
	}

	modules := map[content.ModuleID]*content.ModuleDef{
		// high: weapons + mining
		"mod-autocannon-s": {
			ID: "mod-autocannon-s", Name: "EMP Turret", Slot: content.SlotHigh, Kind: content.KindWeapon,
			ActivationCost: 10, ActivationTime: 2, Price: 1000,
			Weapon: &content.WeaponAttrs{
				Damage: content.Damage{EM: 10, Thermal: 2}, Optimal: 6000, Falloff: 3000,
				Tracking: 0.4, AmmoClass: "small", ShotTime: 2, BayM3: 0.05, // 5 rounds
			},
		},
		"mod-rocket-pod": {
			ID: "mod-rocket-pod", Name: "Rocket Pod", Slot: content.SlotHigh, Kind: content.KindWeapon,
			ActivationCost: 8, ActivationTime: 3, Price: 800,
			Weapon: &content.WeaponAttrs{
				Damage: content.Damage{Explosive: 20}, Optimal: 3000, Falloff: 4000,
				Tracking: 0.15, AmmoClass: "", ShotTime: 3,
			},
		},
		"mod-mining-laser": {
			ID: "mod-mining-laser", Name: "Mining Laser", Slot: content.SlotHigh, Kind: content.KindMiningLaser,
			ActivationCost: 12, ActivationTime: 3, Price: 500,
			Mining: &content.MiningAttrs{VolumePerCycle: 10, CycleTime: 3},
		},
		// mid: tank active / prop / ewar
		"mod-shield-boost": {
			ID: "mod-shield-boost", Name: "Shield Booster", Slot: content.SlotMid, Kind: content.KindShieldBoost,
			ActivationCost: 20, ActivationTime: 5, Price: 700,
			Boost: &content.BoostAttrs{ShieldBoostPerCycle: 50},
		},
		"mod-afterburner": {
			ID: "mod-afterburner", Name: "Afterburner", Slot: content.SlotMid, Kind: content.KindAfterburner,
			ActivationCost: 30, ActivationTime: 10, Price: 600,
			Boost: &content.BoostAttrs{SpeedMultiplier: 1.5},
		},
		"mod-stasis-web": {
			ID: "mod-stasis-web", Name: "Stasis Webifier", Slot: content.SlotMid, Kind: content.KindStasisWeb,
			ActivationCost: 10, ActivationTime: 5, Price: 900,
			Boost: &content.BoostAttrs{WebStrength: 0.5},
		},
		"mod-warp-scram": {
			ID: "mod-warp-scram", Name: "Warp Scrambler", Slot: content.SlotMid, Kind: content.KindWarpScram,
			ActivationCost: 10, ActivationTime: 5, Price: 900,
			Boost: &content.BoostAttrs{ScramDuration: 8},
		},
		// low: passive tank + reps + cap
		"mod-shield-extender": {
			ID: "mod-shield-extender", Name: "Shield Extender", Slot: content.SlotLow, Kind: content.KindShieldExtender,
			Passive: &content.PassiveAttrs{ShieldHP: 200}, Price: 400,
		},
		"mod-armor-rep": {
			ID: "mod-armor-rep", Name: "Armor Reprocessor", Slot: content.SlotLow, Kind: content.KindArmorRep,
			ActivationCost: 15, ActivationTime: 6, Price: 700,
			Boost: &content.BoostAttrs{ArmorRepPerCycle: 40},
		},
		"mod-armor-plate": {
			ID: "mod-armor-plate", Name: "Armor Plate", Slot: content.SlotLow, Kind: content.KindArmorPlate,
			Passive: &content.PassiveAttrs{ArmorHP: 250}, Price: 500,
		},
		"mod-resist-coat": {
			ID: "mod-resist-coat", Name: "Resist Coat", Slot: content.SlotLow, Kind: content.KindResistCoat,
			Passive: &content.PassiveAttrs{ResistBonus: content.Resists{EM: 0.2, Thermal: 0.2, Kinetic: 0.2, Explosive: 0.2}},
			Price:   800,
		},
		"mod-cap-booster": {
			ID: "mod-cap-booster", Name: "Cap Booster", Slot: content.SlotLow, Kind: content.KindCapBooster,
			ActivationCost: 0, ActivationTime: 10, Price: 300,
			Boost: &content.BoostAttrs{CapRestore: 100, BayM3: 0.25}, // 25 cells
		},
		"mod-cap-recharger": {
			ID: "mod-cap-recharger", Name: "Cap Recharger", Slot: content.SlotLow, Kind: content.KindCapRecharger,
			Passive: &content.PassiveAttrs{Recharge: -50}, Price: 350,
		},
	}

	npcs := map[content.NpcID]*content.NpcDef{
		"npc-rat-scuttler": {
			ID: "npc-rat-scuttler", Name: "Rat Scuttler", Faction: "Mire Corsairs",
			Ship:          "ship-rat",
			Fit:           []content.ModuleID{"mod-autocannon-s", "mod-shield-boost", "mod-stasis-web", "mod-warp-scram"},
			AggroRange:    15000,
			OrbitRange:    1500,
			FleeThreshold: 0.25,
			Bounty:        15000,
			Loot:          []content.LootDrop{{Item: "loot-alloy-scrap", Chance: 1.0, MinQty: 1, MaxQty: 2}},
		},
	}

	station := func(id content.CelID, name string, pos core.Vec2, services ...string) content.CelestialDef {
		return content.CelestialDef{
			ID: id, Name: name, Kind: content.CelStation, Position: pos, Radius: 500,
			Station: &content.StationDef{Services: services},
		}
	}

	stationA := station("cel-station-a", "Haven Rest", core.Vec2{X: 1e7}, "repair", "market", "refine", "fitting")
	// The fixture's founding orders live on the station (v2 market schema):
	// asks that seed supply and standing bids that seed demand — never
	// crossing (every bid strictly below its ask).
	stationA.Station.Market = []content.MarketOrderDef{
		{Item: "ammo-emp-s", Qty: 100, Price: 5},
		{Item: "charge-cap-cell", Qty: 50, Price: 10},
		{Item: "min-ferrite", Qty: 500, Price: 10},
		{Item: "mod-armor-plate", Qty: 10, Price: 1000},
		{Item: "ship-barge", Qty: 3, Price: 50000},
		{Item: "ammo-emp-s", Qty: 40, Price: 4, Side: "buy"},
		{Item: "min-ferrite", Qty: 200, Price: 9, Side: "buy"},
	}

	sysA := &content.SystemDef{
		ID: "sys-a", Name: "Fixture Prime", Security: "high",
		Constellation: "Test", Region: "Test",
		Celestials: []content.CelestialDef{
			{ID: "cel-star-a", Name: "Fixture Star", Kind: content.CelStar,
				Position: core.Vec2{}, Radius: 1e8},
			stationA,
			{ID: "cel-belt-a", Name: "Fixture Belt", Kind: content.CelBelt,
				Position: core.Vec2{X: 2e7, Y: 1e7}, Radius: 2000,
				Belt: &content.BeltDef{Asteroids: []content.AsteroidDef{
					{Ore: "ore-sterngrit", Count: 2, Volume: 100},
				}},
			},
			{ID: "cel-anomaly-a", Name: "Fixture Den", Kind: content.CelAnomaly,
				Position: core.Vec2{X: 5e6, Y: 5e6}, Radius: 1500,
				Anomaly: &content.AnomalyDef{
					Spawns:     []content.SpawnDef{{NpcID: "npc-rat-scuttler", Count: 1}},
					RespawnSec: 60,
				},
			},
			{ID: "cel-gate-b", Name: "Gate to Beta", Kind: content.CelGate,
				Position: core.Vec2{X: 3e7}, Radius: 300,
				Gate: &content.GateDef{ToSystem: "sys-b", ToCel: "cel-gate-a"},
			},
		},
	}

	sysB := &content.SystemDef{
		ID: "sys-b", Name: "Fixture Beta", Security: "low",
		Constellation: "Test", Region: "Test",
		Celestials: []content.CelestialDef{
			{ID: "cel-gate-a", Name: "Gate to Alpha", Kind: content.CelGate,
				Position: core.Vec2{}, Radius: 300,
				Gate: &content.GateDef{ToSystem: "sys-a", ToCel: "cel-gate-b"},
			},
			{ID: "cel-belt-b", Name: "Beta Belt", Kind: content.CelBelt,
				Position: core.Vec2{X: 1e7}, Radius: 1000,
				Belt: &content.BeltDef{Asteroids: []content.AsteroidDef{
					{Ore: "ore-sterngrit", Count: 1, Volume: 50},
				}},
				Spawns: []content.SpawnDef{{NpcID: "npc-rat-scuttler", Count: 1}},
			},
		},
	}

	return &content.Content{
		Items:   items,
		Ships:   ships,
		Modules: modules,
		NPCs:    npcs,
		Systems: map[content.SystemID]*content.SystemDef{"sys-a": sysA, "sys-b": sysB},
		Worlds: map[content.WorldID]*content.WorldDef{
			"world-test": testWorld(),
		},
	}
}

func testWorld() *content.WorldDef {
	return &content.WorldDef{
		ID:   "world-test",
		Name: "Test World",
		Start: content.StartDef{
			System:   "sys-a",
			Position: core.Vec2{X: 1e7, Y: 500},
			Ship:     "ship-sloop",
			Fit:      []content.ModuleID{"mod-autocannon-s"},
			Cargo: []content.CargoStack{
				{Item: "ammo-emp-s", Qty: 20},
				{Item: "charge-cap-cell", Qty: 5},
			},
			Wallet: 100000,
		},
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

const testSeed = int64(1234)

// v is a concise Vec2 constructor for tests (v() is the zero vector).
func v(xy ...float64) core.Vec2 {
	if len(xy) < 2 {
		return core.Vec2{}
	}
	return core.V(xy[0], xy[1])
}

func newTestSim(t *testing.T) *Sim {
	t.Helper()
	s, err := New(testContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func stepN(s *Sim, n int) {
	for i := 0; i < n; i++ {
		s.Step()
	}
}

// drain returns all pending events (and clears them).
func drain(s *Sim) []Event { return s.Events() }

// findEvent returns the first event matching kind and a substring of text.
func findEvent(evs []Event, kind EventKind, substr string) *Event {
	for i := range evs {
		if evs[i].Kind == kind && strings.Contains(evs[i].Text, substr) {
			return &evs[i]
		}
	}
	return nil
}

// hasEvent reports whether any event matches kind and a text substring.
func hasEvent(evs []Event, kind EventKind, substr string) bool {
	return findEvent(evs, kind, substr) != nil
}

// entityByName returns the on-grid entity with the given display name.
func entityByName(s *Sim, name string) ecs.Entity {
	for _, info := range s.Grid() {
		if info.Name == name {
			return info.Entity
		}
	}
	return 0
}

// nearestNPC returns the closest hostile NPC entity to the player.
func nearestNPC(s *Sim) ecs.Entity {
	best := ecs.Entity(0)
	bestD := math.MaxFloat64
	for _, info := range s.Grid() {
		if info.Kind == KindShip && info.Name != "Sloop" && info.Entity != s.player &&
			s.AIs.Has(info.Entity) && info.Distance < bestD {
			bestD, best = info.Distance, info.Entity
		}
	}
	return best
}

// injectModule fits a module into the player's first free matching slot by
// direct manipulation (no station needed), recomputes fit and returns the
// slot reference.
func injectModule(t *testing.T, s *Sim, id content.ModuleID) SlotRef {
	t.Helper()
	def, ok := s.Content.Module(id)
	if !ok {
		t.Fatalf("unknown module %s", id)
	}
	fit, ok := s.Fittings.Get(s.player)
	if !ok {
		t.Fatalf("player has no fitting")
	}
	rt := ModuleRuntime{Def: id}
	switch def.Slot {
	case content.SlotHigh:
		fit.High = append(fit.High, id)
		fit.HighRT = append(fit.HighRT, rt)
		s.recomputeFit(s.player)
		return SlotRef{Class: content.SlotHigh, Index: len(fit.High) - 1}
	case content.SlotMid:
		fit.Mid = append(fit.Mid, id)
		fit.MidRT = append(fit.MidRT, rt)
		s.recomputeFit(s.player)
		return SlotRef{Class: content.SlotMid, Index: len(fit.Mid) - 1}
	default:
		fit.Low = append(fit.Low, id)
		fit.LowRT = append(fit.LowRT, rt)
		s.recomputeFit(s.player)
		return SlotRef{Class: content.SlotLow, Index: len(fit.Low) - 1}
	}
}

// lockInstantly grants a full lock on target without lock time.
func lockInstantly(t *testing.T, s *Sim, target ecs.Entity) {
	t.Helper()
	forceLock(s, target)
}

// forceLock grants a full lock on target without lock time (test-script
// friendly, no *testing.T required).
func forceLock(s *Sim, target ecs.Entity) {
	if target.IsZero() {
		return
	}
	tg, ok := s.Targetings.Get(s.player)
	if !ok {
		return
	}
	for _, l := range tg.Locks {
		if l.Target == target {
			return
		}
	}
	tg.Locks = append(tg.Locks, Lock{Target: target, Progress: 1})
}

// spawnNearPlayer creates an NPC next to the player (station area, no
// spawn-group respawn association).
func spawnNearPlayer(t *testing.T, s *Sim) ecs.Entity {
	t.Helper()
	e := s.spawnNpc("npc-rat-scuttler", core.Vec2{X: 1e7, Y: 2000}, 100)
	if e.IsZero() {
		t.Fatalf("spawnNpc failed")
	}
	return e
}

// damageNPC applies raw (pre-resist) damage from src to dst this tick.
func damageNPC(s *Sim, src, dst ecs.Entity, dmg content.Damage, wrecking bool) {
	s.damage = append(s.damage, pendingDamage{src: src, dst: dst, dmg: dmg, wrecking: wrecking})
}

// teleport moves an entity directly.
func teleport(s *Sim, e ecs.Entity, pos core.Vec2) {
	tr, ok := s.Transforms.Get(e)
	if !ok {
		return
	}
	tr.Pos = pos
}

// playerPos returns the player's position.
func playerPos(s *Sim) core.Vec2 {
	tr, _ := s.Transforms.Get(s.player)
	return tr.Pos
}

// setPlayerPos moves the player directly.
func setPlayerPos(s *Sim, pos core.Vec2) { teleport(s, s.player, pos) }

// giveCargo adds items to the player's general cargo.
func giveCargo(s *Sim, item content.ItemID, qty int) {
	c, _ := s.Cargos.Get(s.player)
	c.Stacks = addStacks(c.Stacks, item, qty)
}

// cargoCount counts units of an item in the player's general cargo.
func cargoCount(s *Sim, item content.ItemID) int {
	c, _ := s.Cargos.Get(s.player)
	return stacksCount(c.Stacks, item)
}

// ammoTotal counts an item across the player's cargo and every fitted bay.
func ammoTotal(s *Sim, item content.ItemID) int {
	n := cargoCount(s, item)
	if fit, ok := s.Fittings.Get(s.player); ok {
		for _, class := range slotClasses {
			rts := fit.Runtimes(class)
			for i := range rts {
				if rts[i].Clip.Item == item {
					n += rts[i].Clip.Qty
				}
			}
		}
	}
	return n
}

// hudModule resolves a player module runtime by slot for assertions.
func hudModule(s *Sim, slot SlotRef) (ModuleRuntime, bool) {
	_, rt, ok := s.moduleRuntime(s.player, slot)
	if !ok {
		return ModuleRuntime{}, false
	}
	return *rt, true
}
