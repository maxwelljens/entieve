package sim

import (
	"math"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
)

// firstAsteroid returns one belt rock entity.
func firstAsteroid(t *testing.T, s *Sim) ecs.Entity {
	t.Helper()
	rocks := sortedEntities(s.Asteroids)
	if len(rocks) == 0 {
		t.Fatal("no asteroids in the fixture belt")
	}
	return rocks[0]
}

func TestMiningYieldWithRemainderAccumulation(t *testing.T) {
	s := newTestSim(t)
	rock := firstAsteroid(t, s)
	rp, _ := s.posOf(rock)
	setPlayerPos(s, rp.Add(v(800, 0)))

	laser := injectModule(t, s, "mod-mining-laser")
	// Fractional volume: 0.4 m³ per cycle, ore volume 1 m³ per unit.
	def := s.Content.Modules["mod-mining-laser"]
	def.Mining.VolumePerCycle = 0.4

	s.Queue(CmdMine{Asteroid: rock, Slot: laser})
	s.Step()
	evs := drain(s)
	if !hasEvent(evs, EvLog, "Mined 0.40 m³ Sterngrit") {
		t.Fatalf("missing mining event: %v", evs)
	}
	cargo, _ := s.Cargos.Get(s.player)
	if cargoCount(s, "ore-sterngrit") != 0 {
		t.Fatal("0.9 m³ should not yet form a whole unit")
	}
	if math.Abs(cargo.Remainder-0.4) > 1e-9 {
		t.Fatalf("remainder=%v want 0.4", cargo.Remainder)
	}
	// Cycle 2: 0.8 accumulated. Cycle 3: 1.2 -> one unit, 0.2 remainder.
	stepN(s, 180)
	cargo, _ = s.Cargos.Get(s.player)
	if cargoCount(s, "ore-sterngrit") != 0 || math.Abs(cargo.Remainder-0.8) > 1e-9 {
		t.Fatalf("units=%d remainder=%v want 0/0.8",
			cargoCount(s, "ore-sterngrit"), cargo.Remainder)
	}
	stepN(s, 180)
	cargo, _ = s.Cargos.Get(s.player)
	if cargoCount(s, "ore-sterngrit") != 1 {
		t.Fatalf("units=%d want 1", cargoCount(s, "ore-sterngrit"))
	}
	if math.Abs(cargo.Remainder-0.2) > 1e-9 {
		t.Fatalf("remainder=%v want 0.2", cargo.Remainder)
	}
	// The asteroid lost exactly the mined volume (1.2 m³), no rounding loss.
	ast, _ := s.Asteroids.Get(rock)
	if math.Abs(ast.Remaining-98.8) > 1e-9 {
		t.Fatalf("asteroid remaining=%v want 98.8", ast.Remaining)
	}
}

func TestMiningOreHoldFallbackToCargo(t *testing.T) {
	s := newTestSim(t) // sloop: OreHold = 0 -> ore lands in general cargo
	rock := firstAsteroid(t, s)
	rp, _ := s.posOf(rock)
	setPlayerPos(s, rp.Add(v(500, 0)))
	laser := injectModule(t, s, "mod-mining-laser")

	s.Queue(CmdMine{Asteroid: rock, Slot: laser})
	s.Step()
	drain(s)
	cargo, _ := s.Cargos.Get(s.player)
	if len(cargo.Ore) != 0 {
		t.Fatalf("sloop has no ore hold, got %+v", cargo.Ore)
	}
	if stacksCount(cargo.Stacks, "ore-sterngrit") != 10 {
		t.Fatalf("cargo ore=%d want 10", stacksCount(cargo.Stacks, "ore-sterngrit"))
	}
}

func TestMiningUsesOreHoldWhenPresent(t *testing.T) {
	s := newTestSim(t)
	// Swap the hull to the barge (1000 m³ ore hold) the way assembly would.
	ship, _ := s.Ships.Get(s.player)
	ship.Hull = "ship-barge"
	s.recomputeFit(s.player)
	rock := firstAsteroid(t, s)
	rp, _ := s.posOf(rock)
	setPlayerPos(s, rp.Add(v(500, 0)))
	laser := injectModule(t, s, "mod-mining-laser")

	s.Queue(CmdMine{Asteroid: rock, Slot: laser})
	s.Step()
	drain(s)
	cargo, _ := s.Cargos.Get(s.player)
	if stacksCount(cargo.Ore, "ore-sterngrit") != 10 {
		t.Fatalf("ore hold=%d want 10", stacksCount(cargo.Ore, "ore-sterngrit"))
	}
	if stacksCount(cargo.Stacks, "ore-sterngrit") != 0 {
		t.Fatal("ore should not spill into general cargo")
	}
}

func TestAsteroidDepletionDestroysRock(t *testing.T) {
	s := newTestSim(t)
	rock := firstAsteroid(t, s)
	ast, _ := s.Asteroids.Get(rock)
	ast.Remaining = 25
	rp, _ := s.posOf(rock)
	setPlayerPos(s, rp.Add(v(500, 0)))
	laser := injectModule(t, s, "mod-mining-laser")

	s.Queue(CmdMine{Asteroid: rock, Slot: laser})
	s.Step() // cycle 1: 25 -> 15
	drain(s)
	stepN(s, 180) // cycle 2: 15 -> 5
	if !s.World.Alive(rock) {
		t.Fatal("asteroid died early")
	}
	stepN(s, 180) // cycle 3 mines the last 5 m³
	evs := drain(s)
	if findEvent(evs, EvLog, "depleted") == nil {
		t.Fatalf("missing depletion event: %v", evs)
	}
	if s.World.Alive(rock) {
		t.Fatal("depleted asteroid should be destroyed")
	}
	// The still-active laser deactivates on the next cycle.
	stepN(s, 200)
	rt, _ := hudModule(s, laser)
	if rt.Active {
		t.Fatal("laser should deactivate after depletion")
	}
}

func TestMiningCargoFullEmitsEvent(t *testing.T) {
	s := newTestSim(t)
	rock := firstAsteroid(t, s)
	rp, _ := s.posOf(rock)
	setPlayerPos(s, rp.Add(v(500, 0)))
	laser := injectModule(t, s, "mod-mining-laser")
	// Squeeze the general hold below one ore unit (1 m³).
	cargo, _ := s.Cargos.Get(s.player)
	cargo.Capacity = s.stacksVolume(cargo.Stacks) + 0.5

	s.Queue(CmdMine{Asteroid: rock, Slot: laser})
	s.Step()
	evs := drain(s)
	if findEvent(evs, EvCargoFull, "full") == nil {
		t.Fatalf("missing cargo-full event: %v", evs)
	}
	rt, _ := hudModule(s, laser)
	if rt.Active {
		t.Fatal("laser should deactivate when the hold is full")
	}
	ast, _ := s.Asteroids.Get(rock)
	if ast.Remaining != 100 {
		t.Fatalf("asteroid mined despite full hold: %v", ast.Remaining)
	}
}

// ---------------------------------------------------------------------------
// Looting
// ---------------------------------------------------------------------------

// makeWreck drops a wreck near the player.
func makeWreck(t *testing.T, s *Sim, loot []content.CargoStack, expiresIn float64) ecs.Entity {
	t.Helper()
	w := s.create()
	s.Names.Set(w, Name{Value: "Rat Scuttler Wreck"})
	s.Transforms.Set(w, Transform{Pos: playerPos(s).Add(v(500, 0))})
	s.Wrecks.Set(w, Wreck{Loot: loot, ExpiresAt: s.time + expiresIn})
	return w
}

func TestLootAllStacks(t *testing.T) {
	s := newTestSim(t)
	w := makeWreck(t, s, []content.CargoStack{
		{Item: "loot-alloy-scrap", Qty: 2},
		{Item: "loot-navdata", Qty: 1},
	}, 300)
	s.Queue(CmdLoot{Wreck: w})
	s.Step()
	evs := drain(s)
	if cargoCount(s, "loot-alloy-scrap") != 2 || cargoCount(s, "loot-navdata") != 1 {
		t.Fatalf("cargo scrap=%d navdata=%d",
			cargoCount(s, "loot-alloy-scrap"), cargoCount(s, "loot-navdata"))
	}
	if s.World.Alive(w) {
		t.Fatal("emptied wreck should be removed")
	}
	if !hasEvent(evs, EvLooted, "Looted 2 x Alloy Scrap") {
		t.Fatalf("missing loot events: %v", evs)
	}
}

func TestLootFilteredByItem(t *testing.T) {
	s := newTestSim(t)
	w := makeWreck(t, s, []content.CargoStack{
		{Item: "loot-alloy-scrap", Qty: 2},
		{Item: "loot-navdata", Qty: 3},
	}, 300)
	s.Queue(CmdLoot{Wreck: w, Item: "loot-alloy-scrap"})
	s.Step()
	evs := drain(s)
	if cargoCount(s, "loot-alloy-scrap") != 2 {
		t.Fatalf("scrap=%d want 2", cargoCount(s, "loot-alloy-scrap"))
	}
	if cargoCount(s, "loot-navdata") != 0 {
		t.Fatal("filtered loot must not move other items")
	}
	if !s.World.Alive(w) {
		t.Fatal("wreck with remaining loot must survive")
	}
	wr, _ := s.Wrecks.Get(w)
	if len(wr.Loot) != 1 || wr.Loot[0].Item != "loot-navdata" || wr.Loot[0].Qty != 3 {
		t.Fatalf("remaining loot=%+v", wr.Loot)
	}
	// A filter for an absent item warns.
	s.Queue(CmdLoot{Wreck: w, Item: "min-ferrite"})
	s.Step()
	evs = drain(s)
	if findEvent(evs, EvLog, "No Ferrite in wreck") == nil {
		t.Fatalf("expected warning: %v", evs)
	}
}

func TestWreckExpiryRemovesWreck(t *testing.T) {
	s := newTestSim(t)
	w := makeWreck(t, s, []content.CargoStack{{Item: "loot-navdata", Qty: 1}}, 0)
	s.Step() // ExpiresAt == creation time -> expires immediately
	if s.World.Alive(w) {
		t.Fatal("expired wreck should be removed")
	}
}

func TestLootOutOfRangeWarns(t *testing.T) {
	s := newTestSim(t)
	w := s.create()
	s.Names.Set(w, Name{Value: "Distant Wreck"})
	s.Transforms.Set(w, Transform{Pos: playerPos(s).Add(v(30000, 0))})
	s.Wrecks.Set(w, Wreck{
		Loot:      []content.CargoStack{{Item: "loot-navdata", Qty: 1}},
		ExpiresAt: s.time + 300,
	})
	s.Queue(CmdLoot{Wreck: w})
	s.Step()
	evs := drain(s)
	if findEvent(evs, EvLog, "out of range") == nil {
		t.Fatalf("expected range warning: %v", evs)
	}
	if cargoCount(s, "loot-navdata") != 0 {
		t.Fatal("should not loot from 30 km away")
	}
	if !s.World.Alive(w) {
		t.Fatal("wreck must survive a failed loot")
	}
}

func TestLootWhileDockedRejected(t *testing.T) {
	s := newTestSim(t)
	w := makeWreck(t, s, []content.CargoStack{{Item: "loot-navdata", Qty: 1}}, 300)
	station := entityByName(s, "Haven Rest")
	s.Queue(CmdDock{Station: station})
	s.Step()
	drain(s)
	s.Queue(CmdLoot{Wreck: w})
	s.Step()
	evs := drain(s)
	if findEvent(evs, EvLog, "Cannot loot while docked") == nil {
		t.Fatalf("expected warning: %v", evs)
	}
}
