package sim

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
)

// ---------------------------------------------------------------------------
// Save migration: the retired laser turrets + charges map onto the shell-era
// guns + small-calibre shells at restore. Schema stays 2 (a content-id
// remap, not a state change).
// ---------------------------------------------------------------------------

// legacyOf maps a current id back to the retired id it replaced (the inverse
// of the registry, so the test fixtures cannot drift from it).
func legacyOfItem(id content.ItemID) content.ItemID {
	for old, cur := range content.LegacyIDs.Items {
		if cur == id {
			return old
		}
	}
	return id
}

func legacyOfModule(id content.ModuleID) content.ModuleID {
	for old, cur := range content.LegacyIDs.Modules {
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
			// Ore is an ItemID, so it rides the same registry as cargo.
			Asteroid: &Asteroid{Ore: legacyOfItem("ammo-emp-s")},
			Wreck:    &Wreck{Loot: []content.CargoStack{old("ammo-plutonium-s")}},
			Traffic:  &Traffic{Item: legacyOfItem("ammo-fusion-s")},
		}},
		// Belt state carries its own ore per rock.
		Belts: []BeltState{{System: "sys-a", Cel: "cel-belt-a",
			Rocks: []BeltRock{{Ore: legacyOfItem("ammo-ap-s")}}}},
		Jobs: []Job{{ID: 1, Station: "cel-station-a", Recipe: "rec-charge-emp", Runs: 1}},
		Agents: []SavedAgent{{
			ID:        "agent-1",
			Inventory: []content.CargoStack{old("ammo-fusion-s")},
			Beliefs:   []SavedAgentVal{{Item: legacyOfItem("ammo-emp-s"), Val: 5}},
			Wishes:    []HaulWish{{Owner: "agent-1", Item: legacyOfItem("ammo-ap-s")}},
		}},
		TrafficJourneys: []TrafficJourney{{Item: legacyOfItem("ammo-fusion-s"), Hull: "ship-sloop"}},
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
	if se.Asteroid.Ore != "ammo-emp-s" {
		t.Errorf("asteroid ore = %s, want ammo-emp-s", se.Asteroid.Ore)
	}
	if save.Belts[0].Rocks[0].Ore != "ammo-ap-s" {
		t.Errorf("belt rock ore = %s, want ammo-ap-s", save.Belts[0].Rocks[0].Ore)
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
	// No hull has been renamed, so a ship id passes through verbatim — the
	// walk covers ShipID regardless (TestWalkContentIDsCoversEveryField).
	if save.TrafficJourneys[0].Hull != "ship-sloop" {
		t.Errorf("journey hull = %s, want ship-sloop (no ship id is retired)", save.TrafficJourneys[0].Hull)
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

// ---------------------------------------------------------------------------
// The walker is the migration: every id-bearing field is found by reflection,
// not by a second hand-written list.
// ---------------------------------------------------------------------------

// walkCanary marks every content id in a materialized Save until the walk
// rewrites it.
const walkCanary = "id-walk-canary"

// materialize builds a value of t with every container populated — pointers
// allocated, one element in each slice/array/map — and every content id set to
// walkCanary, so the id walk meets a value at every place the Save type graph
// can carry one.
func materialize(rt reflect.Type) reflect.Value {
	v := reflect.New(rt).Elem()
	switch {
	case contentIDTypes[rt]:
		v.SetString(walkCanary)
	case rt.Kind() == reflect.Pointer:
		p := reflect.New(rt.Elem())
		p.Elem().Set(materialize(rt.Elem()))
		v.Set(p)
	case rt.Kind() == reflect.Struct:
		for i := 0; i < rt.NumField(); i++ {
			if f := v.Field(i); f.CanSet() { // exported only: Save is a JSON type
				f.Set(materialize(rt.Field(i).Type))
			}
		}
	case rt.Kind() == reflect.Slice:
		s := reflect.MakeSlice(rt, 1, 1)
		s.Index(0).Set(materialize(rt.Elem()))
		v.Set(s)
	case rt.Kind() == reflect.Array:
		for i := 0; i < v.Len(); i++ {
			v.Index(i).Set(materialize(rt.Elem()))
		}
	case rt.Kind() == reflect.Map:
		m := reflect.MakeMap(rt)
		m.SetMapIndex(reflect.New(rt.Key()).Elem(), materialize(rt.Elem()))
		v.Set(m)
	}
	return v
}

// leftoverIDs lists the paths still holding walkCanary: every place the type
// graph carries an id that walkContentIDs never visited.
func leftoverIDs(v reflect.Value, path string, out []string) []string {
	switch v.Kind() {
	case reflect.Pointer:
		if !v.IsNil() {
			out = leftoverIDs(v.Elem(), path, out)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			out = leftoverIDs(v.Field(i), path+"."+v.Type().Field(i).Name, out)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			out = leftoverIDs(v.Index(i), fmt.Sprintf("%s[%d]", path, i), out)
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			out = leftoverIDs(v.MapIndex(k), fmt.Sprintf("%s[%v]", path, k), out)
		}
	case reflect.String:
		if contentIDTypes[v.Type()] && v.String() == walkCanary {
			out = append(out, path)
		}
	}
	return out
}

// TestWalkContentIDsCoversEveryField is what keeps the walk deep: a
// materialized Save carries walkCanary in every content id its type graph can
// reach, the walk must rewrite every one of them, and any leftover names the
// id-bearing field the walker cannot see. A new id field anywhere in Save
// fails here instead of quietly escaping restore migration.
func TestWalkContentIDsCoversEveryField(t *testing.T) {
	save := materialize(reflect.TypeOf(Save{})).Addr().Interface().(*Save)
	planted := len(leftoverIDs(reflect.ValueOf(save).Elem(), "Save", nil))
	if planted == 0 {
		t.Fatal("fixture planted no ids")
	}
	visited := 0
	walkContentIDs(save, func(id reflect.Value) {
		if id.String() == walkCanary {
			id.SetString("walked")
			visited++
		}
	})
	if left := leftoverIDs(reflect.ValueOf(save).Elem(), "Save", nil); len(left) > 0 {
		t.Errorf("walkContentIDs missed %d id field(s): %s", len(left), strings.Join(left, ", "))
	}
	if visited != planted {
		t.Errorf("walk visited %d of %d planted ids", visited, planted)
	}
}

// TestRestoreMigratesOreAndHullIDs pins the three id fields the hand-written
// walk never visited: an asteroid's ore, a belt rock's ore and a parked
// journey's hull. Ore fields are ItemID, so a retired item id stored there now
// migrates; hulls have no retired ids yet and ride through verbatim.
func TestRestoreMigratesOreAndHullIDs(t *testing.T) {
	s := newTestSim(t)
	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	legacyOre := legacyOfItem("ammo-emp-s") // "charge-emp": a retired item id
	rocks := 0
	for i := range save.Entities {
		if a := save.Entities[i].Asteroid; a != nil {
			a.Ore = legacyOre
			rocks++
		}
	}
	if rocks == 0 {
		t.Fatal("fixture: no asteroid entities on the grid")
	}
	if len(save.Belts) == 0 || len(save.Belts[0].Rocks) == 0 {
		t.Fatal("fixture: no belt state")
	}
	save.Belts[0].Rocks[0].Ore = legacyOre
	save.TrafficJourneys = []TrafficJourney{{System: save.System, Item: legacyOre, Qty: 1, Hull: "ship-sloop"}}

	restored := newTestSim(t)
	if err := restored.Restore(save); err != nil {
		t.Fatalf("restore legacy ore save: %v", err)
	}
	got, err := restored.Snapshot()
	if err != nil {
		t.Fatalf("re-snapshot: %v", err)
	}
	mapped := 0
	for i := range got.Entities {
		if a := got.Entities[i].Asteroid; a != nil {
			if a.Ore == legacyOre {
				t.Errorf("entity %d asteroid ore still %s", got.Entities[i].ID, a.Ore)
			}
			if a.Ore == "ammo-emp-s" {
				mapped++
			}
		}
	}
	if mapped != rocks {
		t.Errorf("migrated %d of %d asteroid ores", mapped, rocks)
	}
	if ore := got.Belts[0].Rocks[0].Ore; ore != "ammo-emp-s" {
		t.Errorf("belt rock ore = %s, want ammo-emp-s", ore)
	}
	if j := got.TrafficJourneys[0]; j.Item != "ammo-emp-s" || j.Hull != "ship-sloop" {
		t.Errorf("journey = %+v, want the migrated item and the hull verbatim", j)
	}
}

// ---------------------------------------------------------------------------
// Schema v2 -> v3: shell bays and per-slot ammo selections arrived with v3.
// A v2 save has no bay state, so its rounds all sit in cargo; restoring it
// migrates in place (schema 3, empty bays) and the bay rule then resolves
// each gun — one shell type auto-loads, 2+ wait for a choice.
// ---------------------------------------------------------------------------

// TestRestoreMigratesV2Bays is the red-first pin for the schema bump.
func TestRestoreMigratesV2Bays(t *testing.T) {
	s := newTestSim(t)
	slot := SlotRef{Class: content.SlotHigh, Index: 0}
	if got, want := ammoTotal(s, "ammo-emp-s"), 20; got != want {
		t.Fatalf("fixture total=%d want %d", got, want)
	}

	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	// Rewrite the snapshot as a v2 save: no bays, no selections, every round
	// back in the hold (exactly what the pre-bay engine persisted).
	save.Schema = saveSchemaV2
	for i := range save.Entities {
		se := &save.Entities[i]
		if se.Fitting == nil {
			continue
		}
		for _, rts := range [][]ModuleRuntime{se.Fitting.HighRT, se.Fitting.MidRT, se.Fitting.LowRT} {
			for j := range rts {
				if rts[j].Clip.Qty > 0 && se.Cargo != nil {
					se.Cargo.Stacks = addStacks(se.Cargo.Stacks, rts[j].Clip.Item, rts[j].Clip.Qty)
				}
				rts[j].Clip = content.CargoStack{}
				rts[j].Selection = ""
			}
		}
	}

	b, err := New(testContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Restore(save); err != nil {
		t.Fatalf("restore v2: %v", err)
	}
	// The migration stamped the save v3 and left the bays empty…
	if got, want := b.schema, saveSchemaV3; got != want {
		t.Fatalf("restored schema=%d want %d", got, want)
	}
	if got, want := ammoTotal(b, "ammo-emp-s"), 20; got != want {
		t.Fatalf("restored total=%d want %d (rounds conserved)", got, want)
	}
	if got := bayRounds(b, slot); got != 0 {
		t.Fatalf("migrated bay=%d want 0 before the bay rule runs", got)
	}
	// …and the next tick starts the single type's load: the rounds move in at
	// once, the selection lands when the load completes.
	b.Step()
	if got := bayRounds(b, slot); got != 5 {
		t.Fatalf("bay after migrate+step=%d want 5 (rounds in at the start)", got)
	}
	if cs, _ := b.Clip(b.player, slot); !cs.Loading {
		t.Fatalf("clip = %+v, want a load in flight", cs)
	}
	stepN(b, 120) // the load cycle
	if got := b.ammoPref(b.player, slot); got != "ammo-emp-s" {
		t.Fatalf("selection=%q want ammo-emp-s", got)
	}
	// The migrated save keeps working: the gun fires from its bay.
	npc := b.spawnNpc("npc-rat-scuttler", core.Vec2{X: 1e7, Y: 2e3}, 100)
	forceLock(b, npc)
	b.Queue(CmdActivate{Slot: slot, Target: npc})
	b.Step()
	if got := bayRounds(b, slot); got != 4 {
		t.Fatalf("bay after the migrated gun fired=%d want 4", got)
	}
}
