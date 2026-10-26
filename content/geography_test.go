package content

import (
	"slices"
	"testing"
)

// ---------------------------------------------------------------------------
// Geography sanity: the Kresy ring shipped in content/data/systems/*.json.
// Validate cross-checks every reference; these tests pin the shipped shape —
// the canon gate topology, station services/books, founding-seed ordering
// and the zone strings every system file repeats.
// ---------------------------------------------------------------------------

// gateEdges is the canon gate topology: one entry per edge, system ids
// sorted inside the key. The ring runs Sterhov — Rudnek — Młyndal —
// Kuźbor — Kamenvik — Głusholm — Pustheim, with Rudnek the Charter anchor.
var gateEdges = []string{
	"sys-gloam|sys-glusza",
	"sys-gloam|sys-penumbra",
	"sys-glusza|sys-kamieniec",
	"sys-kamieniec|sys-penumbra",
	"sys-mlynow|sys-penumbra",
	"sys-mlynow|sys-rudnik",
	"sys-penumbra|sys-solace",
	"sys-rudnik|sys-solace",
}

// TestGateTopologyPairs: every gate has exactly one counter-gate pointing
// back at it, each edge is crossed by exactly one gate per side, and the
// shipped edge set is exactly the canon topology.
func TestGateTopologyPairs(t *testing.T) {
	c := MustLoad()
	got := map[string]int{}
	for _, id := range keys(c.Systems) {
		sys := c.Systems[id]
		for i := range sys.Celestials {
			cel := &sys.Celestials[i]
			if cel.Gate == nil {
				continue
			}
			dest, ok := c.Systems[cel.Gate.ToSystem]
			if !ok {
				t.Errorf("%s/%s: unknown destination system %q", id, cel.ID, cel.Gate.ToSystem)
				continue
			}
			back := 0
			for j := range dest.Celestials {
				g := dest.Celestials[j].Gate
				if g != nil && g.ToSystem == sys.ID && g.ToCel == cel.ID {
					back++
				}
			}
			if back != 1 {
				t.Errorf("%s/%s: %d counter-gates in %s pointing back, want 1", id, cel.ID, back, cel.Gate.ToSystem)
			}
			a, b := string(sys.ID), string(cel.Gate.ToSystem)
			if a > b {
				a, b = b, a
			}
			got[a+"|"+b]++
		}
	}
	for edge, n := range got {
		if n != 2 {
			t.Errorf("edge %s crossed by %d gates, want 2 (one per side)", edge, n)
		}
	}
	for _, edge := range gateEdges {
		if got[edge] == 0 {
			t.Errorf("canon edge %s missing from the shipped topology", edge)
		}
	}
	for edge := range got {
		if !slices.Contains(gateEdges, edge) {
			t.Errorf("edge %s is not part of the canon topology", edge)
		}
	}
}

// beltNumerals numbers a system's belts in celestial order; extend the table
// when a system ships a sixth belt.
var beltNumerals = []string{"I", "II", "III", "IV", "V"}

// TestCelestialNamesFollowConventions: belt names number per system ("Asteroid
// Belt I", II, …) and gate names carry their destination's display name — the
// registry rows in docs/lore/NAMING.md ([Celestials]).
func TestCelestialNamesFollowConventions(t *testing.T) {
	c := MustLoad()
	for _, id := range keys(c.Systems) {
		sys := c.Systems[id]
		belts := 0
		for i := range sys.Celestials {
			cel := &sys.Celestials[i]
			if cel.Kind == CelBelt {
				belts++
				if belts > len(beltNumerals) {
					t.Errorf("%s/%s: belt %d, extend beltNumerals", id, cel.ID, belts)
				} else if want := "Asteroid Belt " + beltNumerals[belts-1]; cel.Name != want {
					t.Errorf("%s/%s: name %q, want %q", id, cel.ID, cel.Name, want)
				}
			}
			if cel.Gate != nil {
				if want := "Gate (" + c.Systems[cel.Gate.ToSystem].Name + ")"; cel.Name != want {
					t.Errorf("%s/%s: name %q, want %q", id, cel.ID, cel.Name, want)
				}
			}
		}
	}
}

// TestShippedStationsServeAndStock: every station carries at least one
// service from the allowed set and the market service; the four new
// stations ship a founding market block (their endowment is the only
// initial liquidity the world gets — Jernvold already carries one).
func TestShippedStationsServeAndStock(t *testing.T) {
	c := MustLoad()
	newSystems := map[SystemID]bool{
		"sys-rudnik": true, "sys-mlynow": true, "sys-kamieniec": true, "sys-glusza": true,
	}
	stations := 0
	for _, id := range keys(c.Systems) {
		for i := range c.Systems[id].Celestials {
			cel := &c.Systems[id].Celestials[i]
			if cel.Kind != CelStation {
				continue
			}
			stations++
			ctx := string(id) + "/" + string(cel.ID)
			if cel.Station == nil {
				t.Errorf("%s: station celestial without a station block", ctx)
				continue
			}
			if len(cel.Station.Services) == 0 {
				t.Errorf("%s: no services", ctx)
			}
			if !slices.Contains(cel.Station.Services, "market") {
				t.Errorf("%s: services %v lack the market service", ctx, cel.Station.Services)
			}
			for _, s := range cel.Station.Services {
				if !validServices[s] {
					t.Errorf("%s: unknown service %q", ctx, s)
				}
			}
			if newSystems[id] && len(cel.Station.Market) == 0 {
				t.Errorf("%s: founding market block missing", ctx)
			}
		}
	}
	if stations < 6 {
		t.Errorf("shipped stations = %d, want at least 6", stations)
	}
}

// TestFoundingBooksOpenNeverCrossed: on every shipped book the best founding
// bid stays strictly below the best founding ask for the same item — a
// founding book that opens crossed trades against itself.
func TestFoundingBooksOpenNeverCrossed(t *testing.T) {
	c := MustLoad()
	books := 0
	for _, id := range keys(c.Systems) {
		for i := range c.Systems[id].Celestials {
			cel := &c.Systems[id].Celestials[i]
			if cel.Station == nil || len(cel.Station.Market) == 0 {
				continue
			}
			books++
			ask := map[ItemID]float64{}
			bid := map[ItemID]float64{}
			for _, o := range cel.Station.Market {
				switch o.Side {
				case "buy":
					if p, ok := bid[o.Item]; !ok || o.Price > p {
						bid[o.Item] = o.Price
					}
				default:
					if p, ok := ask[o.Item]; !ok || o.Price < p {
						ask[o.Item] = o.Price
					}
				}
			}
			for _, item := range keys(bid) {
				if a, ok := ask[item]; ok && bid[item] >= a {
					t.Errorf("%s/%s: founding bid %v for %q crosses ask %v",
						id, cel.ID, bid[item], item, a)
				}
			}
		}
	}
	if books < 5 {
		t.Errorf("founding books = %d, want at least 5 (Jernvold + the four new stations)", books)
	}
}

// TestShippedSecurityAndZoneStrings: security codes stay in the allowed set
// and every system file repeats universe.json's region/constellation — plus
// the four new systems' zone assignments, as approved.
func TestShippedSecurityAndZoneStrings(t *testing.T) {
	c := MustLoad()
	if c.Universe == nil {
		t.Fatal("universe missing")
	}
	want := map[SystemID]string{
		"sys-rudnik":    "high",
		"sys-mlynow":    "low",
		"sys-kamieniec": "low",
		"sys-glusza":    "null",
	}
	for _, id := range keys(c.Systems) {
		sys := c.Systems[id]
		if !validSecurities[sys.Security] {
			t.Errorf("systems/%s: security %q outside high|low|null", id, sys.Security)
		}
		if sys.Region != c.Universe.Region {
			t.Errorf("systems/%s: region %q, want %q", id, sys.Region, c.Universe.Region)
		}
		if sys.Constellation != c.Universe.Constellation {
			t.Errorf("systems/%s: constellation %q, want %q", id, sys.Constellation, c.Universe.Constellation)
		}
		if sec, ok := want[id]; ok && sys.Security != sec {
			t.Errorf("systems/%s: security %q, want %q", id, sys.Security, sec)
		}
		delete(want, id)
	}
	for id := range want {
		t.Errorf("systems/%s: missing from the shipped content", id)
	}
}
