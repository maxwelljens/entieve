package content

import (
	"slices"
	"testing"
)

// TestMarketStationsSeedBothMaterialSides pins the raw-material liquidity
// the market tree is built for: EVERY station offering the market service
// seeds both sides of every ore and mineral — a hold can always be sold and
// materials always bought back, at every port (Havstov included; its
// founding book carries materials and nothing else). The seeds are finite
// endowments: no restock lives in content.
func TestMarketStationsSeedBothMaterialSides(t *testing.T) {
	c := MustLoad()
	if err := c.Validate(); err != nil {
		t.Fatalf("content invalid: %v", err)
	}
	books := 0
	for _, sysID := range keys(c.Systems) {
		for _, cel := range c.Systems[sysID].Celestials {
			if cel.Station == nil || !slices.Contains(cel.Station.Services, "market") {
				continue
			}
			books++
			ctx := string(sysID) + "/" + string(cel.ID)
			ask, bid := map[ItemID]bool{}, map[ItemID]bool{}
			for _, o := range cel.Station.Market {
				if o.Price <= 0 || o.Qty <= 0 {
					t.Errorf("%s: order %q %v×%d is not a positive price/quantity", ctx, o.Item, o.Price, o.Qty)
				}
				switch o.Side {
				case "", "sell":
					ask[o.Item] = true
				case "buy":
					bid[o.Item] = true
				}
			}
			for _, item := range keys(c.Items) {
				cat := c.Items[item].Category
				if cat != CatOre && cat != CatMineral {
					continue
				}
				if !ask[item] {
					t.Errorf("%s: no ask for %s (materials must be buyable everywhere)", ctx, item)
				}
				if !bid[item] {
					t.Errorf("%s: no bid for %s (a mined hold must be sellable everywhere)", ctx, item)
				}
			}
		}
	}
	if books != 6 {
		t.Fatalf("market stations = %d, want the six shipped ports", books)
	}
}

// TestStarterSystemOffersIndustry: the starter system's station runs the
// manufacturing service — the player's first factory is in Sterhov.
func TestStarterSystemOffersIndustry(t *testing.T) {
	c := MustLoad()
	solace, ok := c.Systems["sys-solace"]
	if !ok {
		t.Fatal("sys-solace missing")
	}
	found := 0
	for _, cel := range solace.Celestials {
		if cel.Station == nil {
			continue
		}
		if slices.Contains(cel.Station.Services, "industry") {
			found++
		}
	}
	if found == 0 {
		t.Fatal("sys-solace has no station with the industry service")
	}
}
