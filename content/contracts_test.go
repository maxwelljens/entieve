package content

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Embedded contract defs: every shipped def validates and boards resolve
// per station.
// ---------------------------------------------------------------------------

// TestEmbeddedContractsValid runs structural checks over every shipped
// contract (on top of the cross-reference pass in Validate).
func TestEmbeddedContractsValid(t *testing.T) {
	c := MustLoad()
	if len(c.Contracts) == 0 {
		t.Fatal("no contracts loaded")
	}
	for _, id := range keys(c.Contracts) {
		ct := c.Contracts[id]
		ctx := "contracts/" + string(id)
		if !strings.HasPrefix(string(ct.ID), "contract-") {
			t.Errorf("%s: id must use the contract- prefix", ctx)
		}
		if ct.Title == "" || ct.Description == "" {
			t.Errorf("%s: missing title/description", ctx)
		}
		if ct.Level < 1 {
			t.Errorf("%s: level %d < 1", ctx, ct.Level)
		}
		if ct.Reward.Credits <= 0 || ct.Reward.StandingGain <= 0 {
			t.Errorf("%s: reward must pay credits and standing (got %v/%v)",
				ctx, ct.Reward.Credits, ct.Reward.StandingGain)
		}
		if ct.Objective.Count < 1 {
			t.Errorf("%s: count %d < 1", ctx, ct.Objective.Count)
		}
	}
}

// TestStationBoardsResolve pins board resolution: Jernvold carries one
// contract of each kind (the showcase world's board), Havstov carries
// the standing-gated level-2 board, unknown stations have empty boards.
func TestStationBoardsResolve(t *testing.T) {
	c := MustLoad()

	haven := c.Board("cel-haven-rest")
	if len(haven) != 3 {
		t.Fatalf("Jernvold board = %d contracts, want 3", len(haven))
	}
	kinds := map[string]int{}
	for _, ct := range haven {
		kinds[ct.Kind]++
	}
	for _, want := range []string{ContractKill, ContractDeliver, ContractMine} {
		if kinds[want] != 1 {
			t.Errorf("Jernvold board kinds = %v, want one %s", kinds, want)
		}
	}
	for _, ct := range haven {
		if ct.Level != 1 || ct.MinStanding != 0 {
			t.Errorf("%s: level/min_standing = %d/%v, want level-1 board", ct.ID, ct.Level, ct.MinStanding)
		}
	}

	port := c.Board("cel-penumbra-port")
	if len(port) != 2 {
		t.Fatalf("Havstov board = %d contracts, want 2", len(port))
	}
	for _, ct := range port {
		if ct.Level != 2 || ct.MinStanding < 0.2 {
			t.Errorf("%s: level/min_standing = %d/%v, want level-2 gated by standing 0.2",
				ct.ID, ct.Level, ct.MinStanding)
		}
	}

	if got := c.Board("cel-nowhere"); len(got) != 0 {
		t.Errorf("unknown station board = %v, want empty", got)
	}
	// Contract() accessor resolves and rejects.
	if _, ok := c.Contract("contract-solace-cleanup-1"); !ok {
		t.Error("Contract(contract-solace-cleanup-1) not found")
	}
	if _, ok := c.Contract("contract-nope"); ok {
		t.Error("Contract(contract-nope) resolved but must not")
	}
}

// ---------------------------------------------------------------------------
// Validator rejection paths
// ---------------------------------------------------------------------------

// baseContract returns a valid deliver contract used as the mutation base.
func baseContract() *ContractDef {
	return &ContractDef{
		ID: "contract-test-base", Title: "Base", Kind: ContractDeliver,
		Level: 1, Station: "cel-a-station",
		Objective: ContractObjective{Item: "min-fer", Count: 1, Destination: "cel-a-dock"},
		Reward:    ContractReward{Credits: 1000, StandingGain: 0.1},
	}
}

// withContracts attaches contracts to a fresh valid baseContent (the
// mutate hook may also break the surrounding content).
func withContracts(mutate func(*Content, *ContractDef)) *Content {
	c := baseContent()
	// A second station so the deliver destination resolves by default.
	sys := c.Systems["sys-a"]
	sys.Celestials = append(sys.Celestials, CelestialDef{
		ID: "cel-a-dock", Name: "Dock", Kind: CelStation, Radius: 500,
		Station: &StationDef{Services: []string{"repair"}},
	})
	ct := baseContract()
	mutate(c, ct)
	c.Contracts = map[ContractID]*ContractDef{ct.ID: ct}
	return c
}

// TestContractNegativeCases pins every validator rejection for contracts.
func TestContractNegativeCases(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Content, *ContractDef)
		want   string
	}{
		{"unknown rat class", func(_ *Content, ct *ContractDef) {
			ct.Objective.Target = "npc-ghost"
			ct.Objective.Item, ct.Objective.Destination = "", ""
			ct.Kind = ContractKill
		}, `unknown rat class "npc-ghost"`},
		{"unknown cargo item", func(_ *Content, ct *ContractDef) {
			ct.Objective.Item = "item-ghost"
		}, `unknown cargo item "item-ghost"`},
		{"unknown destination station", func(_ *Content, ct *ContractDef) {
			ct.Objective.Destination = "cel-ghost"
		}, `destination "cel-ghost" is not a station`},
		{"unknown offering station", func(_ *Content, ct *ContractDef) {
			ct.Station = "cel-ghost"
		}, `offering station "cel-ghost" is not a station`},
		{"negative count", func(_ *Content, ct *ContractDef) {
			ct.Objective.Count = -5
		}, "count -5 must be >= 1"},
		{"zero count", func(_ *Content, ct *ContractDef) {
			ct.Objective.Count = 0
		}, "count 0 must be >= 1"},
		{"unknown kind", func(_ *Content, ct *ContractDef) {
			ct.Kind = "escort"
		}, `unknown kind "escort"`},
		{"level below one", func(_ *Content, ct *ContractDef) {
			ct.Level = 0
		}, "level 0 must be >= 1"},
		{"destination equals offering station", func(_ *Content, ct *ContractDef) {
			ct.Objective.Destination = ct.Station
		}, "destination must differ from the offering station"},
		{"count not whole units", func(c *Content, ct *ContractDef) {
			c.Items["loot-bit"].Volume = 0.3 // 1 m³ / 0.3 m³ per unit
			ct.Objective.Item = "loot-bit"
			ct.Objective.Count = 1
		}, "not a whole number of units"},
		{"mine item not ore", func(_ *Content, ct *ContractDef) {
			ct.Kind = ContractMine
			ct.Objective.Item = "min-fer"
			ct.Objective.Destination = ""
		}, "is not category ore"},
		{"unknown ore", func(_ *Content, ct *ContractDef) {
			ct.Kind = ContractMine
			ct.Objective.Item = "ore-ghost"
			ct.Objective.Destination = ""
		}, `unknown ore "ore-ghost"`},
		{"kill with destination", func(_ *Content, ct *ContractDef) {
			ct.Kind = ContractKill
			ct.Objective.Target = "npc-rat"
			ct.Objective.Item = ""
		}, "kill contract must not set item/destination"},
		{"negative credits", func(_ *Content, ct *ContractDef) {
			ct.Reward.Credits = -1
		}, "negative credits -1"},
		{"negative standing gain", func(_ *Content, ct *ContractDef) {
			ct.Reward.StandingGain = -0.1
		}, "negative standing_gain -0.1"},
		{"unknown reward item", func(_ *Content, ct *ContractDef) {
			ct.Reward.Item = "item-ghost"
			ct.Reward.Qty = 1
		}, `reward references unknown item "item-ghost"`},
		{"reward item without qty", func(_ *Content, ct *ContractDef) {
			ct.Reward.Item = "loot-bit"
			ct.Reward.Qty = 0
		}, "needs qty >= 1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := withContracts(tc.mutate).Validate()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error does not contain %q:\n%s", tc.want, err)
			}
		})
	}
}

// TestContractValidFixture guards the positive baseline (a clean contract
// set must not produce errors).
func TestContractValidFixture(t *testing.T) {
	if err := withContracts(func(*Content, *ContractDef) {}).Validate(); err != nil {
		t.Fatalf("base contract must validate clean:\n%v", err)
	}
}

// ---------------------------------------------------------------------------
// Payout balance: the calibration contract (docs/CONTENT_AUTHORING.md)
// ---------------------------------------------------------------------------

// Payout bands, in reference-value multiples. Every shipped contract must
// land inside its kind's band — this is the calibration contract future
// mission content must follow.
const (
	killRatioMin, killRatioMax = 2.0, 3.0 // kill: bounty + expected loot per target
	mineRatioMin, mineRatioMax = 1.9, 2.1 // mine: ore base value x 2.0 premium
	deliverMargin              = 1.5      // deliver: cargo base value margin
	deliverJumpFee             = 50000    // ...plus a flat fee per gate jump
)

// TestContractPayoutRatios pins the shipped rewards against payout math
// derived from the live economy data (bounties, loot tables, base prices).
func TestContractPayoutRatios(t *testing.T) {
	c := MustLoad()

	killValue := func(npcID NpcID, count int) float64 {
		npc, ok := c.NPCs[npcID]
		if !ok {
			t.Fatalf("missing npc %s", npcID)
		}
		v := float64(count) * npc.Bounty
		for _, d := range npc.Loot {
			avg := float64(d.MinQty+d.MaxQty) / 2
			v += float64(count) * d.Chance * avg * c.Items[d.Item].BasePrice
		}
		return v
	}
	mineValue := func(item ItemID, count int) float64 {
		return float64(count) * c.Items[item].BasePrice
	}
	deliverValue := func(item ItemID, count int) float64 {
		if mod, ok := c.Modules[ModuleID(item)]; ok {
			return float64(count) * mod.Price
		}
		units := float64(count) / c.Items[item].Volume
		return units * c.Items[item].BasePrice
	}

	// Pinned rewards (the CONTENT_AUTHORING.md ratio table).
	pinned := map[ContractID]float64{
		"contract-solace-cleanup-1":    40000,
		"contract-solace-supply-run-1": 86000,
		"contract-solace-cordite-1":    38400,
		"contract-penumbra-den-hunt-1": 240000,
		"contract-penumbra-lucidite-1": 85000,
	}
	for id, want := range pinned {
		ct, ok := c.Contract(id)
		if !ok {
			t.Fatalf("missing contract %s", id)
		}
		if ct.Reward.Credits != want {
			t.Errorf("%s: credits = %v, want pinned %v", id, ct.Reward.Credits, want)
		}
	}

	for _, id := range keys(c.Contracts) {
		ct := c.Contracts[id]
		var reference, reward = 0.0, ct.Reward.Credits
		switch ct.Kind {
		case ContractKill:
			reference = killValue(ct.Objective.Target, ct.Objective.Count)
			if ratio := reward / reference; ratio < killRatioMin || ratio > killRatioMax {
				t.Errorf("%s: kill payout %v is %.2fx the target value %v, want within [%.1f, %.1f]x",
					id, reward, ratio, reference, killRatioMin, killRatioMax)
			}
		case ContractMine:
			reference = mineValue(ct.Objective.Item, ct.Objective.Count)
			if ratio := reward / reference; ratio < mineRatioMin || ratio > mineRatioMax {
				t.Errorf("%s: mine payout %v is %.2fx the ore value %v, want within [%.1f, %.1f]x",
					id, reward, ratio, reference, mineRatioMin, mineRatioMax)
			}
		case ContractDeliver:
			reference = deliverValue(ct.Objective.Item, ct.Objective.Count)
			jumps := gateJumps(t, c, ct.Station, ct.Objective.Destination)
			want := deliverMargin*reference + deliverJumpFee*float64(jumps)
			if diff := reward - want; diff > 0.5 || diff < -0.5 {
				t.Errorf("%s: deliver payout %v, want 1.5 x %v + %d x %d jumps = %v",
					id, reward, reference, deliverJumpFee, jumps, want)
			}
		}
	}
}

// gateJumps counts the stargate jumps between the systems holding two
// stations (BFS over the gate graph; -1 when unreachable).
func gateJumps(t *testing.T, c *Content, from, to CelID) int {
	t.Helper()
	systemOf := func(cel CelID) SystemID {
		for _, id := range keys(c.Systems) {
			for i := range c.Systems[id].Celestials {
				if c.Systems[id].Celestials[i].ID == cel {
					return id
				}
			}
		}
		return ""
	}
	src, dst := systemOf(from), systemOf(to)
	if src == "" || dst == "" {
		t.Fatalf("station not found: %s / %s", from, to)
	}
	if src == dst {
		return 0
	}
	seen := map[SystemID]bool{src: true}
	queue := []SystemID{src}
	hops := 0
	for len(queue) > 0 && hops < 32 {
		var next []SystemID
		for _, sys := range queue {
			for i := range c.Systems[sys].Celestials {
				g := c.Systems[sys].Celestials[i].Gate
				if g == nil || seen[g.ToSystem] {
					continue
				}
				if g.ToSystem == dst {
					return hops + 1
				}
				seen[g.ToSystem] = true
				next = append(next, g.ToSystem)
			}
		}
		queue = next
		hops++
	}
	t.Fatalf("no gate path %s -> %s", src, dst)
	return -1
}
