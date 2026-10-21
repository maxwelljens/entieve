package widgets

import (
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
)

// The derived ContentView reads are implemented once, over the sim, and
// asserted here through the seam: no screen re-derives the rule, and the
// screens' stub views stay tiny (they return a fixed value, as the market
// stub does).

// newViewTest builds the production view over the embedded content and the
// showcase world.
func newViewTest(t *testing.T) (ContentView, *content.Content) {
	t.Helper()
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	wd, ok := c.World("world-showcase")
	if !ok {
		t.Fatal("world-showcase missing")
	}
	s, err := sim.New(c, wd, 42)
	if err != nil {
		t.Fatalf("sim.New: %v", err)
	}
	return NewView(c, s), c
}

// TestViewTradeableName: items, modules, hulls resolve to their def names;
// an unknown id reads as itself.
func TestViewTradeableName(t *testing.T) {
	cv, c := newViewTest(t)
	item, _ := c.Item("ore-cordite")
	mod, _ := c.Module("mod-autocannon-s")
	hull, _ := c.Ship("ship-sloop")
	for _, tc := range []struct{ id, want string }{
		{"ore-cordite", item.Name},
		{"mod-autocannon-s", mod.Name},
		{"ship-sloop", hull.Name}, // the hull branch contracts.go used to miss
		{"not-a-thing", "not-a-thing"},
	} {
		if got := cv.TradeableName(content.ItemID(tc.id)); got != tc.want {
			t.Errorf("TradeableName(%q) = %q, want %q", tc.id, got, tc.want)
		}
	}
}

// TestViewHold: used volume is the item-volume sum (volume-less ids count
// nothing), capacity is the hull's, and a hold that is not fitted reads
// ok=false.
func TestViewHold(t *testing.T) {
	cv, c := newViewTest(t)
	v := cv.(*view)
	player := v.s.Player()
	ferrite, _ := c.Item("min-ferrite")
	v.s.Cargos.Set(player, sim.Cargo{
		Capacity:    400,
		OreCapacity: 100,
		Stacks:      []content.CargoStack{{Item: "ore-cordite", Qty: 10}}, // vol 1
		Ore:         []content.CargoStack{{Item: "min-ferrite", Qty: 20}}, // vol 0.45
	})

	if used, capacity, ok := cv.Hold("cargo"); !ok || used != 10 || capacity != 400 {
		t.Errorf("Hold(cargo) = %v/%v ok=%v, want 10/400", used, capacity, ok)
	}
	if used, capacity, ok := cv.Hold("ore"); !ok || used != 20*ferrite.Volume || capacity != 100 {
		t.Errorf("Hold(ore) = %v/%v ok=%v, want %v/100", used, capacity, ok, 20*ferrite.Volume)
	}

	// No ore hold fitted: the same rule as the hull's 0 m³.
	v.s.Cargos.Set(player, sim.Cargo{Capacity: 400})
	if _, _, ok := cv.Hold("ore"); ok {
		t.Error("Hold(ore) ok with OreCapacity 0, want not fitted")
	}
	if _, _, ok := cv.Hold("bogus"); ok {
		t.Error("Hold(bogus) ok, want false")
	}
}

// TestViewPriceLadder: a station book gives its ladder; the undocked view
// walks the shown system's orders; no order anywhere falls to the base
// price.
func TestViewPriceLadder(t *testing.T) {
	cv, c := newViewTest(t)
	// cel-haven-rest seeds min-argon asks at 64 and bids at 48; either way
	// the ask is the rung.
	if got, want := cv.PriceLadder("cel-haven-rest", "min-argon"), 64.0; got != want {
		t.Errorf("docked PriceLadder(min-argon) = %v, want the seeded best ask %v", got, want)
	}
	if got, want := cv.PriceLadder("", "min-argon"), 64.0; got != want {
		t.Errorf("undocked PriceLadder(min-argon) = %v, want %v from the shown system's orders", got, want)
	}
	// loot-navdata is bid-only: the bid rung answers both views.
	if got := cv.PriceLadder("cel-haven-rest", "loot-navdata"); got != 20000 {
		t.Errorf("docked PriceLadder(loot-navdata) = %v, want the bid 20000", got)
	}
	if got := cv.PriceLadder("", "loot-navdata"); got != 20000 {
		t.Errorf("undocked PriceLadder(loot-navdata) = %v, want the bid 20000", got)
	}
	// No orders anywhere: the def base price, in both views.
	base, _ := c.Item("loot-iced-core")
	if want := base.BasePrice; cv.PriceLadder("cel-haven-rest", "loot-iced-core") != want ||
		cv.PriceLadder("", "loot-iced-core") != want {
		t.Errorf("PriceLadder(loot-iced-core) = %v/%v, want the base %v",
			cv.PriceLadder("cel-haven-rest", "loot-iced-core"), cv.PriceLadder("", "loot-iced-core"), want)
	}
}

// TestViewDeliverProgress: the objective's own unit (m³ for items, whole
// units for modules), both holds counted, capped at the objective.
func TestViewDeliverProgress(t *testing.T) {
	cv, c := newViewTest(t)
	v := cv.(*view)

	itemDef, _ := c.Contract("contract-solace-supply-run-1") // 2 m³ of ammo-emp-s
	ammo, _ := c.Item(itemDef.Objective.Item)
	v.s.Cargos.Set(v.s.Player(), sim.Cargo{Stacks: []content.CargoStack{
		{Item: itemDef.Objective.Item, Qty: 100},
	}})
	if got, want := cv.DeliverProgress(itemDef), 100*ammo.Volume; got != want {
		t.Errorf("DeliverProgress = %v m³, want %v (100 rounds)", got, want)
	}
	// In the ore hold too.
	v.s.Cargos.Set(v.s.Player(), sim.Cargo{Ore: []content.CargoStack{
		{Item: itemDef.Objective.Item, Qty: 100},
	}})
	if got, want := cv.DeliverProgress(itemDef), 100*ammo.Volume; got != want {
		t.Errorf("DeliverProgress(ore) = %v m³, want %v", got, want)
	}
	// Volume-less cargo (a module) counts as whole units and caps.
	modDef, _ := c.Contract("contract-manufacturing-shield-1") // 4 units
	v.s.Cargos.Set(v.s.Player(), sim.Cargo{Stacks: []content.CargoStack{
		{Item: modDef.Objective.Item, Qty: 9},
	}})
	if got, want := cv.DeliverProgress(modDef), float64(modDef.Objective.Count); got != want {
		t.Errorf("DeliverProgress(module) = %v, want the capped %v units", got, want)
	}
}

// TestViewMeetsStanding: the sim's accept gate, epsilon included.
func TestViewMeetsStanding(t *testing.T) {
	cv, _ := newViewTest(t) // a fresh pilot stands at 0
	if !cv.MeetsStanding(0) || !cv.MeetsStanding(1e-10) {
		t.Error("MeetsStanding(0 / 1e-10) = false, want the sim's epsilon to clear both")
	}
	if cv.MeetsStanding(1e-8) {
		t.Error("MeetsStanding(1e-8) = true, want false (standing 0)")
	}
}
