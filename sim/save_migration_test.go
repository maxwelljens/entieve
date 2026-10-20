package sim

import (
	"testing"

	"github.com/entieve/entieve/content"
)

// ---------------------------------------------------------------------------
// Save migration: the retired laser turrets + charges map onto the shell-era
// guns + small-calibre shells at restore. Schema stays 2 (a content-id
// remap, not a state change).
// ---------------------------------------------------------------------------

// legacyOf maps a current id back to the retired id it replaced (the inverse
// of the production tables, so the test fixtures cannot drift from them).
func legacyOfItem(id content.ItemID) content.ItemID {
	for old, cur := range legacyItemIDs {
		if cur == id {
			return old
		}
	}
	return id
}

func legacyOfModule(id content.ModuleID) content.ModuleID {
	for old, cur := range legacyModuleIDs {
		if cur == id {
			return old
		}
	}
	return id
}

// TestMigrateLegacyIDsEveryField is the field-level pin: every place a Save
// carries a content id is remapped, and a second pass changes nothing (so a
// restore is idempotent like the rest of the load path).
func TestMigrateLegacyIDsEveryField(t *testing.T) {
	old := func(item content.ItemID) content.CargoStack {
		return content.CargoStack{Item: legacyOfItem(item), Qty: 1}
	}
	save := &Save{
		Hangar: Hangar{
			Items: []content.CargoStack{old("ammo-emp-s")},
			Ships: []StoredShip{{
				Hull:  "ship-sloop",
				Fit:   []content.ModuleID{legacyOfModule("mod-autocannon-s")},
				Cargo: []content.CargoStack{old("ammo-ap-s")},
			}},
		},
		Market: map[content.SystemID][]MarketStock{
			"sys-a": {{Station: "cel-station-a", Item: legacyOfItem("ammo-emp-s"), Qty: 1, Price: 5, Side: OrderSell, ID: 1}},
		},
		MarketLasts: []SavedLastTrade{{System: "sys-a", Station: "cel-station-a", Item: legacyOfItem("ammo-ap-s"), Price: 4}},
		Entities: []SavedEntity{{
			ID: 1,
			Cargo: &Cargo{
				Stacks: []content.CargoStack{old("ammo-emp-s")},
				Ore:    []content.CargoStack{old("ammo-ap-s")},
			},
			Fitting: &Fitting{
				High:   []content.ModuleID{legacyOfModule("mod-artillery-s")},
				Low:    []content.ModuleID{legacyOfModule("mod-autocannon-s")},
				HighRT: []ModuleRuntime{{Def: legacyOfModule("mod-artillery-s"), Active: true}},
			},
			Wreck:   &Wreck{Loot: []content.CargoStack{old("ammo-plutonium-s")}},
			Traffic: &Traffic{Item: legacyOfItem("ammo-fusion-s")},
		}},
		Jobs: []Job{{ID: 1, Station: "cel-station-a", Recipe: "rec-charge-emp", Runs: 1}},
		Agents: []SavedAgent{{
			ID:        "agent-1",
			Inventory: []content.CargoStack{old("ammo-fusion-s")},
			Beliefs:   []SavedAgentVal{{Item: legacyOfItem("ammo-emp-s"), Val: 5}},
			Wishes:    []HaulWish{{Owner: "agent-1", Item: legacyOfItem("ammo-ap-s")}},
		}},
		TrafficJourneys: []TrafficJourney{{Item: legacyOfItem("ammo-fusion-s")}},
	}
	migrateLegacyIDs(save)

	if save.Hangar.Items[0].Item != "ammo-emp-s" {
		t.Errorf("hangar item = %s, want ammo-emp-s", save.Hangar.Items[0].Item)
	}
	if save.Hangar.Ships[0].Fit[0] != "mod-autocannon-s" || save.Hangar.Ships[0].Cargo[0].Item != "ammo-ap-s" {
		t.Errorf("stored ship = %+v, want the shell-era fit and cargo", save.Hangar.Ships[0])
	}
	if save.Market["sys-a"][0].Item != "ammo-emp-s" {
		t.Errorf("market order item = %s, want ammo-emp-s", save.Market["sys-a"][0].Item)
	}
	if save.MarketLasts[0].Item != "ammo-ap-s" {
		t.Errorf("last trade item = %s, want ammo-ap-s", save.MarketLasts[0].Item)
	}
	se := save.Entities[0]
	if se.Cargo.Stacks[0].Item != "ammo-emp-s" || se.Cargo.Ore[0].Item != "ammo-ap-s" {
		t.Errorf("cargo = %+v, want the shell-era stacks", se.Cargo)
	}
	if se.Fitting.High[0] != "mod-artillery-s" || se.Fitting.Low[0] != "mod-autocannon-s" ||
		se.Fitting.HighRT[0].Def != "mod-artillery-s" {
		t.Errorf("fitting = %+v, want the shell-era guns (runtime Def included)", se.Fitting)
	}
	if se.Wreck.Loot[0].Item != "ammo-plutonium-s" {
		t.Errorf("wreck loot = %+v, want ammo-plutonium-s", se.Wreck.Loot)
	}
	if se.Traffic.Item != "ammo-fusion-s" {
		t.Errorf("convoy shipment = %s, want ammo-fusion-s", se.Traffic.Item)
	}
	if save.Jobs[0].Recipe != "rec-ammo-emp-s" {
		t.Errorf("job recipe = %s, want rec-ammo-emp-s", save.Jobs[0].Recipe)
	}
	a := save.Agents[0]
	if a.Inventory[0].Item != "ammo-fusion-s" || a.Beliefs[0].Item != "ammo-emp-s" || a.Wishes[0].Item != "ammo-ap-s" {
		t.Errorf("agent = %+v, want migrated inventory/beliefs/wishes", a)
	}
	if save.TrafficJourneys[0].Item != "ammo-fusion-s" {
		t.Errorf("journey item = %s, want ammo-fusion-s", save.TrafficJourneys[0].Item)
	}

	// Idempotent: the mapping is one-way and current ids are not keys.
	again := *save
	migrateLegacyIDs(&again)
	if again.Hangar.Items[0].Item != "ammo-emp-s" || again.Entities[0].Fitting.High[0] != "mod-artillery-s" {
		t.Error("second migration pass changed state")
	}
}

// TestRestoreMigratesLegacySave is the end-to-end pin: a save written before
// the rename (old turrets and charges everywhere a live sim carries them)
// restores into the same state as the current save it was made from — same
// Hash, byte-identical re-snapshot, same behaviour on the next steps.
func TestRestoreMigratesLegacySave(t *testing.T) {
	s := newTestSim(t)
	giveCargo(s, "ammo-ap-s", 5)
	s.Hangar.Items = append(s.Hangar.Items, content.CargoStack{Item: "ammo-emp-s", Qty: 3})
	giveCargo(s, "ammo-emp-s", 20) // second stack of the same shell: order is preserved

	wantHash := s.Hash()
	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	// Rewrite the save into the retired vocabulary: the fitted gun, its
	// preferred shells and the hangar stack.
	for i := range save.Entities {
		se := &save.Entities[i]
		if se.Fitting != nil {
			for j := range se.Fitting.High {
				se.Fitting.High[j] = legacyOfModule(se.Fitting.High[j])
			}
			for j := range se.Fitting.HighRT {
				se.Fitting.HighRT[j].Def = legacyOfModule(se.Fitting.HighRT[j].Def)
			}
		}
		if se.Cargo != nil {
			for j := range se.Cargo.Stacks {
				se.Cargo.Stacks[j].Item = legacyOfItem(se.Cargo.Stacks[j].Item)
			}
		}
	}
	for i := range save.Hangar.Items {
		save.Hangar.Items[i].Item = legacyOfItem(save.Hangar.Items[i].Item)
	}
	for _, orders := range save.Market {
		for i := range orders {
			orders[i].Item = legacyOfItem(orders[i].Item)
		}
	}

	// Restore into a fresh sim: the migrated state must equal the original.
	restored := newTestSim(t)
	if err := restored.Restore(save); err != nil {
		t.Fatalf("restore legacy save: %v", err)
	}
	if got := restored.Hash(); got != wantHash {
		t.Fatalf("hash after migration = %d, want %d (migration must not change state)", got, wantHash)
	}

	// Save/load byte identity: the migrated snapshot is the canonical one.
	again, err := restored.Snapshot()
	if err != nil {
		t.Fatalf("re-snapshot: %v", err)
	}
	second := newTestSim(t)
	if err := second.Restore(again); err != nil {
		t.Fatalf("restore migrated save: %v", err)
	}
	if got := second.Hash(); got != wantHash {
		t.Fatalf("hash after second round-trip = %d, want %d", got, wantHash)
	}

	// Determinism: the migrated sim and the original sim step identically.
	twin := newTestSim(t)
	if err := twin.Restore(save); err != nil {
		t.Fatalf("restore twin: %v", err)
	}
	stepN(s, 30)
	stepN(twin, 30)
	if s.Hash() != twin.Hash() {
		t.Fatal("twin runs diverged after restoring a legacy save")
	}
}
