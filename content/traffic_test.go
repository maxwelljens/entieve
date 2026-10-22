package content

import (
	"math"
	"strings"
	"testing"
	"testing/fstest"
)

// ---------------------------------------------------------------------------
// Embedded traffic defs: every shipped def validates structurally on top of
// the cross-reference pass in Validate.
// ---------------------------------------------------------------------------

// TestEmbeddedTrafficValid runs structural checks over every shipped
// traffic def: endpoints live in their (destination) system, cargo fits
// the hull, and the security/raider/patrol rules hold.
func TestEmbeddedTrafficValid(t *testing.T) {
	c := MustLoad()
	if len(c.Traffic) != 7 {
		t.Fatalf("traffic defs = %d, want 7 (solace, penumbra, gloam + the Kresy ring: rudnik, mlynow, kamieniec, glusza)", len(c.Traffic))
	}
	for _, sys := range keys(c.Traffic) {
		td := c.Traffic[sys]
		ctx := "traffic/" + string(sys)
		sysDef, ok := c.Systems[sys]
		if !ok {
			t.Errorf("%s: unknown system", ctx)
			continue
		}
		if len(td.Routes) == 0 {
			t.Errorf("%s: no routes", ctx)
		}
		if sysDef.Security != "high" && td.Raider == "" {
			t.Errorf("%s: security %q without a raider", ctx, sysDef.Security)
		}
		for i, r := range td.Routes {
			if r.IntervalTicks < 1 {
				t.Errorf("%s/routes[%d]: interval %d < 1", ctx, i, r.IntervalTicks)
			}
			if r.Cargo.Qty < 1 {
				t.Errorf("%s/routes[%d]: cargo qty %d < 1", ctx, i, r.Cargo.Qty)
			}
			// `from` lives in this system; `to` lives in to_system (cross-
			// system lanes) or, by default, in this system.
			toSys, toSysDef := sys, sysDef
			if r.ToSystem != "" && r.ToSystem != sys {
				toSys, toSysDef = r.ToSystem, c.Systems[r.ToSystem]
			}
			for _, end := range []struct {
				id  CelID
				sys SystemID
				def *SystemDef
			}{{r.From, sys, sysDef}, {r.To, toSys, toSysDef}} {
				if end.def == nil {
					t.Errorf("%s/routes[%d]: endpoint %q not in %s (unknown system)", ctx, i, end.id, end.sys)
					continue
				}
				cel := celestialIn(end.def, end.id)
				if cel == nil {
					t.Errorf("%s/routes[%d]: endpoint %q not in %s", ctx, i, end.id, end.sys)
					continue
				}
				switch cel.Kind {
				case CelStation, CelGate, CelBelt:
				default:
					t.Errorf("%s/routes[%d]: endpoint %q is kind %q", ctx, i, end.id, cel.Kind)
				}
			}
			hull, ok := c.Ships[r.Hull]
			if !ok {
				t.Errorf("%s/routes[%d]: unknown hull %q", ctx, i, r.Hull)
				continue
			}
			item, ok := c.Items[r.Cargo.Item]
			if !ok {
				t.Errorf("%s/routes[%d]: unknown cargo %q", ctx, i, r.Cargo.Item)
				continue
			}
			if vol := item.Volume * float64(r.Cargo.Qty); vol > hull.Cargo {
				t.Errorf("%s/routes[%d]: cargo volume %v exceeds hull capacity %v", ctx, i, vol, hull.Cargo)
			}
		}
		if td.Patrol != nil {
			for _, h := range td.Patrol.Hulls {
				if _, ok := c.GuardFor(h); !ok {
					t.Errorf("%s/patrol: hull %q has no npc-guard-* def", ctx, h)
				}
			}
		}
	}
	// Registry hulls and the guard registry resolve.
	for _, id := range []ShipID{"ship-npc-hauler", "ship-npc-patrol"} {
		if _, ok := c.Ships[id]; !ok {
			t.Errorf("registry hull %q missing", id)
		}
	}
	if def, ok := c.GuardFor("ship-npc-patrol"); !ok || def.ID != "npc-guard-patrol" {
		t.Errorf("GuardFor(ship-npc-patrol) = %v, %v; want npc-guard-patrol", def, ok)
	}
	if _, ok := c.GuardFor("ship-sloop"); ok {
		t.Error("GuardFor(ship-sloop) resolved, must not (combat hull is no patrol)")
	}
}

// TestTrafficForAccessor pins the accessor: resolves for configured
// systems, misses cleanly otherwise.
func TestTrafficForAccessor(t *testing.T) {
	c := MustLoad()
	if _, ok := c.TrafficFor("sys-solace"); !ok {
		t.Error("TrafficFor(sys-solace) missing")
	}
	if _, ok := c.TrafficFor("sys-nowhere"); ok {
		t.Error("TrafficFor(sys-nowhere) resolved but must not")
	}
}

// ---------------------------------------------------------------------------
// Validator rejection paths
// ---------------------------------------------------------------------------

// baseTraffic returns a valid traffic def used as the mutation base (high
// security: no raider required, no patrol required).
func baseTraffic() *TrafficDef {
	return &TrafficDef{
		System: "sys-a",
		Routes: []RouteDef{{
			From: "cel-a-station", To: "cel-a-belt", Hull: "ship-hull",
			Cargo:         RouteCargo{Item: "min-fer", Qty: 10},
			IntervalTicks: 600,
		}},
	}
}

// withTraffic attaches a traffic def (plus a patrol-capable hull/guard pair)
// to a fresh valid baseContent.
func withTraffic(mutate func(*TrafficDef)) *Content {
	c := baseContent()
	c.Ships["ship-guard"] = &ShipDef{
		ID: "ship-guard", Name: "Guard", Class: "frigate", Group: "Ships/Frigates",
		Slots:  Slots{High: 1, Mid: 1, Low: 1},
		Shield: 100, Armor: 100, Structure: 100,
		Resists:   Resists{EM: 0.1, Thermal: 0.1, Kinetic: 0.1, Explosive: 0.1},
		Capacitor: 300, Recharge: 200,
		MaxSpeed: 300, Agility: 0.5, Mass: 1000000, SigRadius: 40,
		Cargo: 100, WarpSpeed: 3, MaxTargetRange: 40000, MaxTargets: 2, ScanResolution: 500,
	}
	// The new hull is a manufactured product too: recipe completeness applies.
	c.Recipes["rec-ship-guard"] = &RecipeDef{
		ID: "rec-ship-guard", Product: "ship-guard", Qty: 1,
		Inputs: []RecipeInput{{Item: "min-fer", Qty: 50}}, Time: 120, Family: FamilyHulls,
	}
	c.NPCs["npc-guard-test"] = &NpcDef{
		ID: "npc-guard-test", Name: "Guard", Faction: "Test", Ship: "ship-guard",
		Fit: []ModuleID{"mod-gun"}, AggroRange: 5000, OrbitRange: 1000,
		FleeThreshold: 0.2, Bounty: 0,
	}
	// A rat that exists but spawns nowhere: the raider-must-spawn check needs
	// a positive and a negative reference.
	c.NPCs["npc-rat-loose"] = &NpcDef{
		ID: "npc-rat-loose", Name: "Loose Rat", Faction: "Test", Ship: "ship-hull",
		Fit: []ModuleID{"mod-gun"}, AggroRange: 5000, OrbitRange: 1000,
		FleeThreshold: 0.2, Bounty: 1000,
	}
	td := baseTraffic()
	mutate(td)
	c.Traffic = map[SystemID]*TrafficDef{td.System: td}
	return c
}

// TestTrafficNegativeCases pins every validator rejection for traffic.
func TestTrafficNegativeCases(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*TrafficDef)
		want   string
	}{
		{"unknown system", func(td *TrafficDef) { td.System = "sys-ghost" },
			`unknown system "sys-ghost"`},
		{"empty routes", func(td *TrafficDef) { td.Routes = nil },
			"no routes (at least one route required)"},
		{"unknown from endpoint", func(td *TrafficDef) { td.Routes[0].From = "cel-ghost" },
			`unknown from endpoint "cel-ghost"`},
		{"unknown to endpoint", func(td *TrafficDef) { td.Routes[0].To = "cel-ghost" },
			`unknown to endpoint "cel-ghost"`},
		{"missing endpoint", func(td *TrafficDef) { td.Routes[0].To = "" },
			"missing to endpoint"},
		{"from equals to", func(td *TrafficDef) {
			td.Routes[0].To = td.Routes[0].From
		}, "from and to must differ"},
		{"endpoint kind", func(td *TrafficDef) { td.Routes[0].From = "cel-a-star" },
			`kind "star" (want station|gate|belt)`},
		{"unknown hull", func(td *TrafficDef) { td.Routes[0].Hull = "ship-ghost" },
			`unknown hull "ship-ghost"`},
		{"bad interval", func(td *TrafficDef) { td.Routes[0].IntervalTicks = 0 },
			"interval_ticks 0 must be >= 1"},
		{"negative interval", func(td *TrafficDef) { td.Routes[0].IntervalTicks = -5 },
			"interval_ticks -5 must be >= 1"},
		{"zero cargo qty", func(td *TrafficDef) { td.Routes[0].Cargo.Qty = 0 },
			"cargo qty 0 must be >= 1"},
		{"unknown cargo item", func(td *TrafficDef) { td.Routes[0].Cargo.Item = "item-ghost" },
			`unknown cargo item "item-ghost"`},
		{"cargo exceeds hull", func(td *TrafficDef) { td.Routes[0].Cargo.Qty = 100000 },
			"exceeds hull ship-hull capacity"},
		{"unknown raider", func(td *TrafficDef) { td.Raider = "npc-ghost" },
			`unknown raider npc "npc-ghost"`},
		{"raider not in system", func(td *TrafficDef) { td.Raider = "npc-rat-loose" },
			"is not spawned anywhere in this system"},
		{"patrol no hulls", func(td *TrafficDef) {
			td.Patrol = &PatrolDef{Hulls: nil, IntervalTicks: 600}
		}, "no hulls (at least one required)"},
		{"patrol unknown hull", func(td *TrafficDef) {
			td.Patrol = &PatrolDef{Hulls: []ShipID{"ship-ghost"}, IntervalTicks: 600}
		}, `hulls[0] unknown hull "ship-ghost"`},
		{"patrol hull without guard", func(td *TrafficDef) {
			td.Patrol = &PatrolDef{Hulls: []ShipID{"ship-hull"}, IntervalTicks: 600}
		}, `hulls[0] "ship-hull" has no npc-guard-* definition`},
		{"patrol bad interval", func(td *TrafficDef) {
			td.Patrol = &PatrolDef{Hulls: []ShipID{"ship-guard"}, IntervalTicks: 0}
		}, "interval_ticks 0 must be >= 1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := withTraffic(tc.mutate).Validate()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error does not contain %q:\n%s", tc.want, err)
			}
		})
	}
}

// TestTrafficSecurityRules pins the security-dependent validation: low/null
// systems require a raider, high systems do not; the raider must be a rat
// class that actually spawns in that system.
func TestTrafficSecurityRules(t *testing.T) {
	// Low security without a raider fails.
	c := withTraffic(func(*TrafficDef) {})
	c.Systems["sys-a"].Security = "low"
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), `security "low" traffic requires a raider npc id`) {
		t.Fatalf("low security without raider: %v", err)
	}

	// With the local rat class it passes.
	c = withTraffic(func(td *TrafficDef) { td.Raider = "npc-rat" })
	c.Systems["sys-a"].Security = "low"
	if err := c.Validate(); err != nil {
		t.Fatalf("low security with local raider must validate:\n%v", err)
	}

	// Null security without a raider fails the same way.
	c = withTraffic(func(*TrafficDef) {})
	c.Systems["sys-a"].Security = "null"
	err = c.Validate()
	if err == nil || !strings.Contains(err.Error(), `security "null" traffic requires a raider npc id`) {
		t.Fatalf("null security without raider: %v", err)
	}

	// High security passes without a raider (no ambushes in high-sec).
	if err := withTraffic(func(*TrafficDef) {}).Validate(); err != nil {
		t.Fatalf("high security without raider must validate:\n%v", err)
	}
}

// TestTrafficValidFixture guards the positive baseline.
func TestTrafficValidFixture(t *testing.T) {
	if err := withTraffic(func(*TrafficDef) {}).Validate(); err != nil {
		t.Fatalf("base traffic must validate clean:\n%v", err)
	}
}

// TestLoadTrafficObjectAndArray proves traffic files accept a single object
// or an array, like every other content directory, and that duplicate
// systems are rejected.
func TestLoadTrafficObjectAndArray(t *testing.T) {
	base := func(extra map[string]string) fstest.MapFS {
		fsys := fstest.MapFS{
			"universe.json": {Data: []byte(`{"region":"R","constellation":"C","start_system":"sys-x"}`)},
			"systems/x.json": {Data: []byte(
				`{"id":"sys-x","name":"X","security":"high","constellation":"C","region":"R","celestials":[]}`)},
			"systems/y.json": {Data: []byte(
				`{"id":"sys-y","name":"Y","security":"high","constellation":"C","region":"R","celestials":[]}`)},
		}
		for path, data := range extra {
			fsys[path] = &fstest.MapFile{Data: []byte(data)}
		}
		return fsys
	}
	c, err := Load(base(map[string]string{
		"traffic/one.json": `{"system":"sys-x","routes":[]}`,
		"traffic/two.json": `[{"system":"sys-y","routes":[]}]`,
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := c.TrafficFor("sys-x"); !ok {
		t.Error("object traffic file not loaded")
	}
	if _, ok := c.TrafficFor("sys-y"); !ok {
		t.Error("array traffic file not loaded")
	}

	_, err = Load(base(map[string]string{
		"traffic/a.json": `{"system":"sys-x","routes":[]}`,
		"traffic/b.json": `[{"system":"sys-x","routes":[]}]`,
	}))
	if err == nil || !strings.Contains(err.Error(), `duplicate traffic id "sys-x"`) {
		t.Fatalf("duplicate traffic system: %v", err)
	}
}

// TestConvoyTimeToKillPinned pins the DPS -> time-to-kill computation for
// shipped traffic content (the calibration contract in
// docs/CONTENT_AUTHORING.md): raiders fire their spawned light charge
// (charge-ap, 12 kinetic — the first sorted light charge, exactly what
// npcCharges loads) through the single-set resist model with full carry-over
// pooling, expected hit multiplier 0.5348505 (mean of the 0.01-0.99 roll
// plus the 0.01 wrecking tail at x3), against the hauler's pooled EHP.
func TestConvoyTimeToKillPinned(t *testing.T) {
	c := MustLoad()

	// E[mult] of rollShot's damage multiplier.
	meanMult := 0.01*3 + (0.01*(1-0.01) + 0.99*(1-0.01*0.01)/2)
	if math.Abs(meanMult-0.5348505) > 1e-9 {
		t.Fatalf("mean multiplier = %v, want 0.5348505", meanMult)
	}

	// The charge an NPC spawns with: first sorted item of the weapon's ammo
	// class (mirrors npcCharges/findCharge).
	spawnCharge := func(class string) *ItemDef {
		for _, id := range keys(c.Items) {
			it := c.Items[id]
			if (it.Category == CatAmmo || it.Category == CatCharge) && it.AmmoClass == class {
				return it
			}
		}
		return nil
	}

	// dps of an NPC def against a hull, assuming every shot lands
	// (stationary target inside optimal): summed pen-volley / shot time
	// times the mean multiplier.
	dps := func(npcID NpcID, hullID ShipID) float64 {
		npc, ok := c.NPCs[npcID]
		if !ok {
			t.Fatalf("missing npc %s", npcID)
		}
		hull, ok := c.Ships[hullID]
		if !ok {
			t.Fatalf("missing hull %s", hullID)
		}
		var d float64
		for _, mid := range npc.Fit {
			m, ok := c.Modules[mid]
			if !ok || m.Kind != KindWeapon || m.Weapon == nil {
				continue
			}
			volley := m.Weapon.Damage
			if m.Weapon.AmmoClass != "" {
				if ch := spawnCharge(m.Weapon.AmmoClass); ch != nil && ch.Damage != nil {
					volley = ch.Damage.Scale(m.Weapon.Mult())
				}
			}
			pen := volley.ApplyTo(hull.Resists).Total()
			period := m.Weapon.ShotTime
			if period <= 0 {
				period = m.ActivationTime
			}
			d += pen * meanMult / period
		}
		return d
	}

	hauler := c.Ships["ship-npc-hauler"]
	ehp := hauler.Shield + hauler.Armor + hauler.Structure
	if ehp != 2800 {
		t.Fatalf("Convoy Hauler EHP = %v, want 2800 (pooled)", ehp)
	}

	// Pinned effective DPS (the spawned first sorted shell of each class,
	// scaled by the gun's damage multiplier, against the hauler's
	// 0.1/0.15/0.35/0.3 resists): marauder 3x medium autocannon (3.6 s),
	// hierarch 2x large artillery + 2x large autocannon (13.5 s / 4.5 s),
	// scuttler 2x small autocannon (3 s).
	cases := []struct {
		raider  NpcID
		raiders int
		wantDPS float64
	}{
		{"npc-rat-marauder", 1, 15.7335},
		{"npc-rat-marauder", 3, 47.2006},
		{"npc-rat-hierarch", 1, 32.1663},
		{"npc-rat-scuttler", 2, 12.5868},
	}
	for _, tc := range cases {
		got := dps(tc.raider, "ship-npc-hauler") * float64(tc.raiders)
		if math.Abs(got-tc.wantDPS) > 0.001 {
			t.Errorf("%s x%d effective DPS = %.4f, want pinned %.4f",
				tc.raider, tc.raiders, got, tc.wantDPS)
		}
	}
	// Pinned headline: one Kuźnica marauder needs ~178 s of continuous fire
	// to kill a convoy hauler; three raiders ~59 s. The escape and patrol
	// windows (20 s flee, 15 s response) are measured against these.
	ttk := ehp / (dps("npc-rat-marauder", "ship-npc-hauler"))
	if math.Abs(ttk-177.96) > 0.1 {
		t.Errorf("marauder TTK = %.2fs, want ~177.96s", ttk)
	}
	if ttk3 := ehp / (dps("npc-rat-marauder", "ship-npc-hauler") * 3); math.Abs(ttk3-59.32) > 0.1 {
		t.Errorf("3x marauder TTK = %.2fs, want ~59.32s", ttk3)
	}
}
