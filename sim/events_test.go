package sim

import (
	"testing"

	"github.com/entieve/entieve/content"
)

func TestEventsTravelAndStationKinds(t *testing.T) {
	s := newTestSim(t)
	var all []Event

	// EvTargetLocked (real lock path).
	npc := spawnNearPlayer(t, s)
	s.Queue(CmdLock{Target: npc})
	for i := 0; i < 450; i++ {
		s.Step()
		all = append(all, drain(s)...)
	}
	if !hasEvent(all, EvTargetLocked, "Locked Rat Scuttler") {
		t.Fatalf("missing EvTargetLocked: %v", all)
	}

	// EvModule (activation).
	s.Queue(CmdActivate{Slot: SlotRef{Class: content.SlotHigh, Index: 0}, Target: npc})
	s.Step()
	all = append(all, drain(s)...)
	if !hasEvent(all, EvModule, "EMP Turret activated on Rat Scuttler") {
		t.Fatalf("missing EvModule: %v", all)
	}

	// EvDocked, market buy while docked, then EvUndocked.
	station := entityByName(s, "Haven Rest")
	s.Queue(CmdDock{Station: station})
	s.Step()
	all = append(all, drain(s)...)
	if !hasEvent(all, EvDocked, "Docked at Haven Rest") {
		t.Fatalf("missing EvDocked: %v", all)
	}
	s.Queue(CmdBuy{Item: "min-ferrite", Qty: 2})
	s.Step()
	all = append(all, drain(s)...)
	if !hasEvent(all, EvMarket, "Bought 2 x Ferrite") {
		t.Fatalf("missing EvMarket: %v", all)
	}
	s.Queue(CmdUndock{})
	s.Step()
	all = append(all, drain(s)...)
	if !hasEvent(all, EvUndocked, "Undocked from Haven Rest") {
		t.Fatalf("missing EvUndocked: %v", all)
	}

	// EvJumped.
	gate := entityByName(s, "Gate to Beta")
	setPlayerPos(s, gatePos(s, gate))
	s.Queue(CmdJump{Gate: gate})
	s.Step()
	all = append(all, drain(s)...)
	if !hasEvent(all, EvJumped, "Jumped to Fixture Beta") {
		t.Fatalf("missing EvJumped: %v", all)
	}
}

func TestEventsCombatUtilityAndWarpKinds(t *testing.T) {
	s := newTestSim(t)
	var all []Event

	// EvDestroyed + bounty for a player kill.
	npc := spawnNearPlayer(t, s)
	damageNPC(s, s.player, npc, contentDamage(), false)
	s.Step()
	all = append(all, drain(s)...)
	if !hasEvent(all, EvDestroyed, "Rat Scuttler destroyed") {
		t.Fatalf("missing EvDestroyed: %v", all)
	}
	if !hasEvent(all, EvLog, "Bounty paid: €15,000.00") {
		t.Fatalf("missing bounty log: %v", all)
	}

	// EvLooted.
	w := makeWreck(t, s, []content.CargoStack{{Item: "loot-alloy-scrap", Qty: 1}}, 300)
	s.Queue(CmdLoot{Wreck: w})
	s.Step()
	all = append(all, drain(s)...)
	if !hasEvent(all, EvLooted, "Looted 1 x Alloy Scrap") {
		t.Fatalf("missing EvLooted: %v", all)
	}

	// EvWarp start + land.
	setPlayerPos(s, v(1e7, 1e7))
	gate := entityByName(s, "Gate to Beta")
	s.Queue(CmdWarpTo{Target: gate})
	for i := 0; i < 1200; i++ {
		s.Step()
		all = append(all, drain(s)...)
		if hasEvent(all, EvWarp, "Warp complete") {
			break
		}
	}
	if !hasEvent(all, EvWarp, "Warp drive active") {
		t.Fatalf("missing warp start: %v", all)
	}

	// EvAnomalyCleared.
	setPlayerPos(s, v(5e6+3000, 5e6+500))
	anom := anomalyNPC(t, s)
	damageNPC(s, s.player, anom, contentDamage(), false)
	s.Step()
	all = append(all, drain(s)...)
	if !hasEvent(all, EvAnomalyCleared, "Fixture Den cleared") {
		t.Fatalf("missing EvAnomalyCleared: %v", all)
	}

	// EvCargoFull via mining into a nearly-full hold.
	rock := firstAsteroid(t, s)
	rp, _ := s.posOf(rock)
	setPlayerPos(s, rp)
	laser := injectModule(t, s, "mod-mining-laser")
	cargo, _ := s.Cargos.Get(s.player)
	cargo.Capacity = s.stacksVolume(cargo.Stacks) + 0.5
	s.Queue(CmdMine{Asteroid: rock, Slot: laser})
	s.Step()
	all = append(all, drain(s)...)
	if !hasEvent(all, EvCargoFull, "full") {
		t.Fatalf("missing EvCargoFull: %v", all)
	}
}

func TestEventsCappedAt512(t *testing.T) {
	s := newTestSim(t)
	for i := 0; i < 600; i++ {
		s.emit(Event{Kind: EvLog, Text: "event-" + itoaN(i)})
	}
	if len(s.events) != 512 {
		t.Fatalf("pending=%d want 512", len(s.events))
	}
	if s.events[0].Text != "event-88" {
		t.Fatalf("oldest kept=%q want event-88 (600-512)", s.events[0].Text)
	}
	evs := s.Events()
	if len(evs) != 512 {
		t.Fatalf("drained=%d", len(evs))
	}
	if len(s.events) != 0 {
		t.Fatal("Events() must drain")
	}
	// DrainEvents is the documented alias.
	s.emit(Event{Kind: EvLog, Text: "x"})
	if len(s.DrainEvents()) != 1 {
		t.Fatal("DrainEvents alias broken")
	}
}

func TestInfoCapabilityFlags(t *testing.T) {
	s := newTestSim(t)

	station := entityByName(s, "Haven Rest")
	info, ok := s.Info(station)
	if !ok || !info.CanDock || !info.CanWarp || info.CanJump || info.CanMine || info.CanLoot {
		t.Fatalf("station flags: %+v", info)
	}
	// Docking needs range: park far away first.
	setPlayerPos(s, v())
	info, _ = s.Info(station)
	if info.CanDock {
		t.Fatal("docked flag without range")
	}

	gate := entityByName(s, "Gate to Beta")
	info, _ = s.Info(gate)
	if info.CanJump || !info.CanWarp {
		t.Fatalf("far gate flags: %+v", info)
	}
	setPlayerPos(s, gatePos(s, gate).Add(v(100, 0)))
	info, _ = s.Info(gate)
	if !info.CanJump {
		t.Fatal("gate should be jumpable in range")
	}

	// Rocks: minable in range.
	rock := firstAsteroid(t, s)
	rp, _ := s.posOf(rock)
	setPlayerPos(s, rp.Add(v(1000, 0)))
	info, _ = s.Info(rock)
	if !info.CanMine || info.CanWarp || info.CanLoot {
		t.Fatalf("rock flags: %+v", info)
	}

	// Wreck: lootable in range, not when expired.
	w := makeWreck(t, s, []content.CargoStack{{Item: "loot-navdata", Qty: 1}}, 300)
	info, _ = s.Info(w)
	if !info.CanLoot {
		t.Fatalf("wreck flags: %+v", info)
	}
	wr, _ := s.Wrecks.Get(w)
	wr.ExpiresAt = s.time - 1
	info, _ = s.Info(w)
	if info.CanLoot {
		t.Fatal("expired wreck must not be lootable")
	}

	// Player: KindPlayer, no self-warp.
	pinfo, _ := s.Info(s.player)
	if pinfo.Kind != KindPlayer || pinfo.CanWarp || pinfo.Distance != 0 {
		t.Fatalf("player info: %+v", pinfo)
	}
	// Celestials are warpable, ships are not.
	if cinfo, _ := s.Info(station); !cinfo.CanWarp {
		t.Fatal("station should be warpable")
	}
	spawnNearPlayer(t, s)
	npc := nearestNPC(s)
	if ninfo, _ := s.Info(npc); ninfo.CanWarp || ninfo.Kind != KindShip {
		t.Fatalf("npc info: %+v", ninfo)
	}
}

func TestInfoKinds(t *testing.T) {
	s := newTestSim(t)
	cases := []struct {
		name string
		kind string
	}{
		{"Haven Rest", KindStation},
		{"Gate to Beta", KindGate},
		{"Fixture Star", KindStar},
		{"Fixture Belt", KindBelt}, // belt markers are landmarks (KindBelt)
		{"Fixture Den", KindAnomaly},
		{"Sterngrit Asteroid", KindAsteroid},
	}
	for _, c := range cases {
		e := entityByName(s, c.name)
		if e.IsZero() {
			t.Errorf("entity %q missing", c.name)
			continue
		}
		info, ok := s.Info(e)
		if !ok || info.Kind != c.kind {
			t.Errorf("%q kind=%q want %q", c.name, info.Kind, c.kind)
		}
	}
	p, _ := s.Info(s.player)
	if p.Kind != KindPlayer {
		t.Errorf("player kind=%q", p.Kind)
	}
	// Health fraction: damaged ship reports below 1.
	ship, _ := s.Ships.Get(s.player)
	ship.Shield = 0
	ship.Armor = 0
	ship.Structure = 0
	p, _ = s.Info(s.player)
	if p.Health != 0 {
		t.Errorf("dead health=%v", p.Health)
	}
	ship.Shield = ship.MaxShield
	ship.Armor = ship.MaxArmor
	ship.Structure = ship.MaxStructure
	p, _ = s.Info(s.player)
	if p.Health != 1 {
		t.Errorf("full health=%v", p.Health)
	}
}

func TestGridOrderDeterministic(t *testing.T) {
	s := newTestSim(t)
	g1 := s.Grid()
	g2 := s.Grid()
	if len(g1) != len(g2) || len(g1) == 0 {
		t.Fatalf("grid sizes %d/%d", len(g1), len(g2))
	}
	for i := range g1 {
		if g1[i].Entity != g2[i].Entity {
			t.Fatalf("grid order unstable at %d", i)
		}
	}
	// Handle order, not distance order: Grid is the command name-resolution
	// surface, and distance ordering flips as targets circle each other.
	for i := 1; i < len(g1); i++ {
		if g1[i].Entity <= g1[i-1].Entity {
			t.Fatalf("grid not handle-ordered: %v before %v", g1[i-1].Entity, g1[i].Entity)
		}
	}
	// Distance is still reported from the player: the player itself is 0.
	var found bool
	for _, in := range g1 {
		if in.Entity == s.player {
			found = true
			if in.Distance != 0 {
				t.Fatalf("player distance = %v, want 0", in.Distance)
			}
		}
	}
	if !found {
		t.Fatal("player missing from grid")
	}
}
