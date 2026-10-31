package content

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Contacts: every shipped contact and offer validates, and every station's
// contacts resolve with their offers flattened into the engine's templates.
// ---------------------------------------------------------------------------

// TestEmbeddedOffersValid runs structural checks over every shipped offer
// template (on top of the cross-reference pass in Validate).
func TestEmbeddedOffersValid(t *testing.T) {
	c := MustLoad()
	if len(c.Contacts) == 0 {
		t.Fatal("no contacts loaded")
	}
	if len(c.Contracts) == 0 {
		t.Fatal("contacts produced no offer templates")
	}
	for _, id := range keys(c.Contracts) {
		ct := c.Contracts[id]
		ctx := "contacts/" + string(id)
		if !strings.HasPrefix(string(ct.ID), "offer-") {
			t.Errorf("%s: id must use the offer- prefix", ctx)
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

// TestContactsAtResolve pins the converted pools: every contact lands at its
// station with its offer templates inheriting the contact's station and
// standing gate, and unknown stations carry no contacts.
func TestContactsAtResolve(t *testing.T) {
	c := MustLoad()

	want := map[CelID][]int{
		"cel-haven-rest":    {2},    // Jalvrek: supply run + Kerngrit order
		"cel-huta":          {1, 1}, // Hovžan: cleanup (charter) + web units (line office)
		"cel-doki":          {2, 1}, // Dukšan: Commission liaison + yard office
		"cel-penumbra-port": {2},    // Hauvnok: den hunt + lucidite
	}
	for cel, counts := range want {
		cs := c.ContactsAt(cel)
		if len(cs) != len(counts) {
			t.Fatalf("%s contacts = %d, want %d", cel, len(cs), len(counts))
		}
		for i, n := range counts {
			if len(cs[i].Offers) != n {
				t.Errorf("%s/%s offers = %d, want %d", cel, cs[i].ID, len(cs[i].Offers), n)
			}
		}
		for _, ct := range cs {
			if ct.Station != cel {
				t.Errorf("%s: station = %s", ct.ID, ct.Station)
			}
			for _, of := range ct.Offers {
				def, ok := c.Contract(of.ID)
				if !ok {
					t.Fatalf("%s: offer %s missing from the flattening", ct.ID, of.ID)
				}
				if def.Station != cel || def.MinStanding != ct.MinStanding {
					t.Errorf("%s: template inherited station/standing %s/%v, want %s/%v",
						of.ID, def.Station, def.MinStanding, cel, ct.MinStanding)
				}
			}
		}
	}
	if got := c.ContactsAt("cel-nowhere"); len(got) != 0 {
		t.Errorf("unknown station contacts = %v, want empty", got)
	}
	if _, ok := c.Contact("contact-nope"); ok {
		t.Error("Contact(contact-nope) resolved but must not")
	}
}

// baseContact returns a valid contact with one deliver stock offer, used as
// the mutation base for the validator's rejection paths.
func baseContact() *ContactDef {
	return &ContactDef{
		ID: "contact-test-base", Name: "Base Office", Patron: PatronCharter,
		Station: "cel-a-station",
		Offers: []ContactOffer{{
			ContractDef: ContractDef{
				ID: "offer-test-base", Title: "Base", Kind: ContractDeliver, Level: 1,
				Station:   "cel-a-station",
				Objective: ContractObjective{Item: "min-fer", Count: 1, Destination: "cel-a-dock"},
				Reward:    ContractReward{Credits: 1000, StandingGain: 0.1},
			},
			Source: OfferSource{Kind: "stock", Item: "min-fer", Target: 10, Base: 1, Max: 10},
		}},
	}
}

// withContact attaches a contact to a fresh valid baseContent and flattens
// its offers, so Validate sees the whole wiring.
func withContact(mutate func(*Content, *ContactDef)) *Content {
	c := baseContent()
	sys := c.Systems["sys-a"]
	sys.Celestials = append(sys.Celestials, CelestialDef{
		ID: "cel-a-dock", Name: "Dock", Kind: CelStation, Radius: 500,
		Station: &StationDef{Services: []string{"repair"}},
	})
	ct := baseContact()
	mutate(c, ct)
	c.Contacts = map[ContactID]*ContactDef{ct.ID: ct}
	c.flattenOffers()
	return c
}

// TestContactNegativeCases pins every validator rejection for contacts and
// their offer sources.
func TestContactNegativeCases(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Content, *ContactDef)
		want   string
	}{
		{"unknown patron", func(_ *Content, ct *ContactDef) {
			ct.Patron = "syndicate"
		}, `unknown patron "syndicate"`},
		{"unknown station", func(_ *Content, ct *ContactDef) {
			ct.Station = "cel-ghost"
		}, `station "cel-ghost" is not a station`},
		{"empty pool", func(_ *Content, ct *ContactDef) {
			ct.Offers = nil
		}, "empty offer pool"},
		{"bad contact prefix", func(_ *Content, ct *ContactDef) {
			ct.ID = "broker-test"
			ct.Offers[0].ID = "offer-test-base"
		}, "id must use the contact- prefix"},
		{"offer prefix", func(_ *Content, ct *ContactDef) {
			ct.Offers[0].ID = "contract-test-base"
		}, "offer id must use the offer- prefix"},
		{"unknown source kind", func(_ *Content, ct *ContactDef) {
			ct.Offers[0].Source.Kind = "rumour"
		}, `unknown source kind "rumour"`},
		{"stock item mismatch", func(_ *Content, ct *ContactDef) {
			ct.Offers[0].Source.Item = "min-nickel"
		}, "stock item \"min-nickel\" must be the objective item \"min-fer\""},
		{"stock target below one", func(_ *Content, ct *ContactDef) {
			ct.Offers[0].Source.Target = 0
		}, "stock target 0 must be >= 1"},
		{"rats on a deliver offer", func(_ *Content, ct *ContactDef) {
			ct.Offers[0].Source.Kind = "rats"
			ct.Offers[0].Source.System = "sys-a"
		}, `rats source sits on a kill offer, not "deliver"`},
		{"rats unknown system", func(_ *Content, ct *ContactDef) {
			of := &ct.Offers[0]
			of.Kind, of.Source.Kind, of.Source.System = ContractKill, "rats", "sys-ghost"
			of.Objective = ContractObjective{Target: "npc-rat-scuttler", Count: 1}
		}, `unknown watched system "sys-ghost"`},
		{"rats non-works target", func(_ *Content, ct *ContactDef) {
			of := &ct.Offers[0]
			of.Kind, of.Source.Kind, of.Source.System = ContractKill, "rats", "sys-a"
			of.Objective = ContractObjective{Target: "npc-guard", Count: 1}
		}, `target "npc-guard" is not an Old Works NPC`},
		{"war-clear on deliver", func(_ *Content, ct *ContactDef) {
			ct.Offers[0].Source.Kind = "war-clear"
		}, `war-clear source sits on a kill offer, not "deliver"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := withContact(tc.mutate).Validate()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error does not contain %q:\n%s", tc.want, err)
			}
		})
	}
}

// TestContactValidFixture guards the positive baseline (a clean contact set
// must not produce errors).
func TestContactValidFixture(t *testing.T) {
	if err := withContact(func(*Content, *ContactDef) {}).Validate(); err != nil {
		t.Fatalf("base contact must validate clean:\n%v", err)
	}
}

// ---------------------------------------------------------------------------
// Validator rejection paths for the flattened offer templates
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
			ct.Objective.Target = "npc-rat-scuttler"
		}, "kill contract must not set item/destination"},
		{"negative credits", func(_ *Content, ct *ContractDef) {
			ct.Reward.Credits = -1
		}, "negative credits"},
		{"reward item without qty", func(_ *Content, ct *ContractDef) {
			ct.Reward.Item = "loot-navdata"
		}, "needs qty >= 1"},
		{"negative standing gain", func(_ *Content, ct *ContractDef) {
			ct.Reward.StandingGain = -0.1
		}, "negative standing_gain"},
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

// Payout bands, in reference-value multiples. Every shipped offer must land
// inside its kind's band — this is the calibration contract future mission
// content must follow.
const (
	killRatioMin, killRatioMax = 2.0, 3.0 // kill: bounty + expected loot per target
	mineRatioMin, mineRatioMax = 1.9, 2.1 // mine: ore base value x 2.0 premium
	deliverMargin              = 1.5      // deliver: cargo base value margin
	deliverJumpFee             = 50000    // ...plus a flat fee per gate jump
)

// TestContractPayoutRatios pins the shipped rewards against payout math
// derived from the live economy data (bounties, loot tables, base prices).
// The derived offers scale these authored numbers with their count, so the
// per-unit calibration holds.
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
		"offer-hovzan-cleanup":   40000,
		"offer-jalvrek-supply":   86000,
		"offer-jalvrek-kerngrit": 38400,
		"offer-hauvnok-hunt":     240000,
		"offer-hauvnok-lucidite": 85000,
	}
	for id, want := range pinned {
		ct, ok := c.Contract(id)
		if !ok {
			t.Fatalf("missing offer %s", id)
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
					id, reward, reference, jumps, deliverJumpFee, want)
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
	return -1
}
