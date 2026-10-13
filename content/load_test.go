package content

import (
	"strings"
	"testing"
	"testing/fstest"
)

// TestLoadEmbedded loads the real embedded data and spot-checks the ID
// registry from docs/ARCHITECTURE.md.
func TestLoadEmbedded(t *testing.T) {
	c, err := Load(FS())
	if err != nil {
		t.Fatalf("Load(FS()): unexpected error:\n%v", err)
	}

	maps := []struct {
		name string
		n    int
	}{
		{"Items", len(c.Items)},
		{"Ships", len(c.Ships)},
		{"Modules", len(c.Modules)},
		{"NPCs", len(c.NPCs)},
		{"Systems", len(c.Systems)},
		{"Worlds", len(c.Worlds)},
		{"Recipes", len(c.Recipes)},
	}
	for _, m := range maps {
		if m.n == 0 {
			t.Errorf("map %s is empty after load", m.name)
		}
	}
	if c.Universe == nil {
		t.Fatal("Universe is nil after load")
	}
	if c.Universe.StartSystem != "sys-solace" {
		t.Errorf("Universe.StartSystem = %q, want sys-solace", c.Universe.StartSystem)
	}

	// Spot-check registry IDs.
	for _, id := range []ItemID{"min-ferrite", "min-argon", "ore-sterngrit", "ore-glacierite",
		"charge-emp", "charge-plutonium", "charge-cap-cell",
		"loot-navdata", "loot-hierarch-crest"} {
		if _, ok := c.Items[id]; !ok {
			t.Errorf("Items missing %q", id)
		}
	}
	if len(c.Items) < 19 {
		t.Errorf("len(Items) = %d, want >= 19", len(c.Items))
	}

	sloop, ok := c.Ships["ship-sloop"]
	if !ok {
		t.Fatal("Ships missing ship-sloop")
	}
	if sloop.Slots.High != 3 || sloop.Slots.Mid != 3 || sloop.Slots.Low != 2 {
		t.Errorf("ship-sloop slots = %+v, want 3/3/2", sloop.Slots)
	}
	for _, id := range []ShipID{"ship-cutlass", "ship-barge", "ship-hauler",
		"ship-rat-scuttler", "ship-rat-marauder", "ship-rat-hierarch"} {
		if _, ok := c.Ships[id]; !ok {
			t.Errorf("Ships missing %q", id)
		}
	}

	turret, ok := c.Modules["mod-emp-turret"]
	if !ok {
		t.Fatal("Modules missing mod-emp-turret")
	}
	if turret.Weapon == nil {
		t.Fatal("mod-emp-turret has no weapon block")
	}
	if turret.Weapon.AmmoClass != "light" {
		t.Errorf("mod-emp-turret ammo class = %q, want light", turret.Weapon.AmmoClass)
	}
	if len(c.Modules) < 14 {
		t.Errorf("len(Modules) = %d, want >= 14", len(c.Modules))
	}

	hierarch, ok := c.NPCs["npc-rat-hierarch"]
	if !ok {
		t.Fatal("NPCs missing npc-rat-hierarch")
	}
	if hierarch.Bounty != 250000 {
		t.Errorf("npc-rat-hierarch bounty = %v, want 250000", hierarch.Bounty)
	}

	solace, ok := c.Systems["sys-solace"]
	if !ok {
		t.Fatal("Systems missing sys-solace")
	}
	var station *StationDef
	for i := range solace.Celestials {
		if solace.Celestials[i].ID == "cel-haven-rest" {
			station = solace.Celestials[i].Station
		}
	}
	if station == nil {
		t.Fatal("sys-solace missing cel-haven-rest station block")
	}
	if len(station.Services) != 4 {
		t.Errorf("cel-haven-rest services = %v, want all four", station.Services)
	}

	showcase, ok := c.Worlds["world-showcase"]
	if !ok {
		t.Fatal("Worlds missing world-showcase")
	}
	if len(showcase.Scenario) < 40 {
		t.Errorf("showcase has %d checkpoints, want >= 40", len(showcase.Scenario))
	}
	if _, ok := c.Worlds["world-default"]; !ok {
		t.Error("Worlds missing world-default")
	}
	if got := len(showcase.Start.Fit); got != 8 {
		t.Errorf("showcase fit has %d modules, want 8 (full 3/3/2 rack)", got)
	}
}

// TestLoadDir loads the real data directory from disk.
func TestLoadDir(t *testing.T) {
	c, err := LoadDir("data")
	if err != nil {
		t.Fatalf("LoadDir(data): unexpected error:\n%v", err)
	}
	if len(c.Systems) == 0 || c.Universe == nil {
		t.Fatalf("LoadDir(data) incomplete: %d systems, universe nil=%v", len(c.Systems), c.Universe == nil)
	}
	if _, ok := c.Ships["ship-sloop"]; !ok {
		t.Error("LoadDir(data) missing ship-sloop")
	}
}

// TestMustLoad panics only on broken embedded data, so just prove it works.
func TestMustLoad(t *testing.T) {
	c := MustLoad()
	if c == nil || c.Universe == nil {
		t.Fatal("MustLoad returned incomplete content")
	}
}

// TestLoadAcceptsObjectOrArray proves each file may be a single object or an
// array of objects.
func TestLoadAcceptsObjectOrArray(t *testing.T) {
	fsys := fstest.MapFS{
		"items/object.json": {Data: []byte(`{"id":"min-one","name":"One","category":"mineral","volume":0.01,"base_price":5}`)},
		"items/array.json":  {Data: []byte(`[{"id":"min-two","name":"Two","category":"mineral","volume":0.01,"base_price":6},{"id":"min-three","name":"Three","category":"mineral","volume":0.01,"base_price":7}]`)},
		"ships/one.json":    {Data: []byte(`{"id":"ship-x","name":"X","class":"frigate","slots":{"high":1,"mid":1,"low":1}}`)},
		"universe.json":     {Data: []byte(`{"region":"R","constellation":"C","start_system":"sys-x"}`)},
		"systems/one.json":  {Data: []byte(`{"id":"sys-x","name":"X","security":"high","constellation":"C","region":"R","celestials":[]}`)},
	}
	c, err := Load(fsys)
	if err != nil {
		t.Fatalf("Load: unexpected error:\n%v", err)
	}
	for _, id := range []ItemID{"min-one", "min-two", "min-three"} {
		if _, ok := c.Items[id]; !ok {
			t.Errorf("Items missing %q", id)
		}
	}
	if len(c.Items) != 3 {
		t.Errorf("len(Items) = %d, want 3", len(c.Items))
	}
}

// TestLoadErrorsAccumulate proves parse errors name their file and
// duplicate-ID errors accumulate instead of failing at the first problem.
func TestLoadErrorsAccumulate(t *testing.T) {
	fsys := fstest.MapFS{
		"items/a.json":   {Data: []byte(`{"id":"min-dup","name":"Dup","category":"mineral","volume":0.01,"base_price":5}`)},
		"items/b.json":   {Data: []byte(`[{"id":"min-dup","name":"Dup Again","category":"mineral","volume":0.01,"base_price":5}]`)},
		"items/bad.json": {Data: []byte(`{"id": "min-truncated", `)},
		"universe.json":  {Data: []byte(`{"region":"R","constellation":"C","start_system":""}`)},
	}
	_, err := Load(fsys)
	if err == nil {
		t.Fatal("Load: expected error for broken fixtures")
	}
	msg := err.Error()
	for _, want := range []string{"duplicate item id \"min-dup\"", "items/a.json", "items/b.json", "items/bad.json"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error missing %q in:\n%s", want, msg)
		}
	}
}

// TestLoadUniverseRejectsArray proves universe.json must be a single object.
func TestLoadUniverseRejectsArray(t *testing.T) {
	fsys := fstest.MapFS{
		"universe.json": {Data: []byte(`[{"region":"R"}]`)},
	}
	_, err := Load(fsys)
	if err == nil || !strings.Contains(err.Error(), "expected a single object") {
		t.Fatalf("Load: expected single-object error, got %v", err)
	}
}

// TestLoadMissingUniverse proves a missing universe.json is reported.
func TestLoadMissingUniverse(t *testing.T) {
	fsys := fstest.MapFS{
		"items/x.json": {Data: []byte(`{"id":"min-x","name":"X","category":"mineral"}`)},
	}
	_, err := Load(fsys)
	if err == nil || !strings.Contains(err.Error(), "universe.json") {
		t.Fatalf("Load: expected universe.json error, got %v", err)
	}
}
