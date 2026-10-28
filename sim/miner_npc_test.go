package sim

import (
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// ---------------------------------------------------------------------------
// Civilian belt miners (npc-miner-*): they cut real volume out of the
// persistent belt state, haul a full hold to the nearest station's market
// (minting nothing), die like any other ship, and never turn on the player.
// ---------------------------------------------------------------------------

// spawnMiner places one fixture miner at pos (its Home anchor).
func spawnMiner(t *testing.T, s *Sim, pos core.Vec2) ecs.Entity {
	t.Helper()
	e := s.spawnNpc("npc-miner-test", pos, 0)
	if e.IsZero() {
		t.Fatal("spawnNpc(npc-miner-test) failed")
	}
	return e
}

// TestMinerDepletesBeltRock pins extraction: a miner cuts a rock it did not
// spawn next to, the volume drops on the grid AND in the belt state, and the
// per-cycle log lines stay player-only.
func TestMinerDepletesBeltRock(t *testing.T) {
	s := newTestSim(t)
	beltPos := mustPos(s, s.celEntity("cel-belt-a"))
	miner := spawnMiner(t, s, beltPos)
	drain(s)

	const celID = content.CelID("cel-belt-a")
	st := beltState(s, "sys-a", celID)

	evs := drain(s)
	cut := -1
	for i := 0; i < 1200 && cut < 0; i++ {
		s.Step()
		evs = append(evs, drain(s)...)
		for j := range st.Rocks {
			if st.Rocks[j].Remaining < 100 {
				cut = j
				break
			}
		}
		if ai, ok := s.AIs.Get(miner); ok && ai.State != NpcMine {
			t.Fatalf("tick %d: miner state=%q, want %q", i+1, ai.State, NpcMine)
		}
	}
	if cut < 0 {
		t.Fatal("miner never cut its rock")
	}
	rock := rockByIndex(t, s, celID, cut)
	ast, _ := s.Asteroids.Get(rock)
	if st.Rocks[cut].Remaining != ast.Remaining {
		t.Fatalf("belt state %v, want the entity's %v (sync)", st.Rocks[cut].Remaining, ast.Remaining)
	}
	if ast.Remaining >= 100 {
		t.Fatalf("rock volume = %v, want below the def 100", ast.Remaining)
	}
	// Log noise: the player's log carries no mining lines for the miner.
	for _, ev := range evs {
		if ev.Kind == EvLog && (hasText(ev, "Mined") || hasText(ev, "depleted")) {
			t.Fatalf("miner leaked a mining log line: %q", ev.Text)
		}
	}
}

// TestMinerDeliversOreToNearestStation pins the haul leg: a full hold makes
// the miner depart for the NEAREST station, and on arrival its ore rests on
// that station's market as supply — with no money minted anywhere.
func TestMinerDeliversOreToNearestStation(t *testing.T) {
	c := testContent()
	// A second, far station: the miner must choose the near one.
	sysA := c.Systems["sys-a"]
	far := content.CelestialDef{
		ID: "cel-station-far", Name: "Far Port", Kind: content.CelStation,
		Position: core.Vec2{X: 9e7, Y: 9e7}, Radius: 500,
		Station: &content.StationDef{Services: []string{"market"}},
	}
	sysA.Celestials = append(sysA.Celestials, far)

	s, err := New(c, testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	beltPos := mustPos(s, s.celEntity("cel-belt-a"))
	miner := spawnMiner(t, s, beltPos)
	cargo, _ := s.Cargos.Get(miner)
	cargo.Ore = []content.CargoStack{{Item: "ore-sterngrit", Qty: 995}}
	s.Cargos.Set(miner, *cargo)
	wallet := s.Wallet()
	drain(s)

	var hauled bool
	for i := 0; i < 3000; i++ {
		s.Step()
		drain(s)
		if ai, ok := s.AIs.Get(miner); ok && ai.State == NpcHaul {
			hauled = true
		}
		if !s.World.Alive(miner) {
			break
		}
	}
	if !hauled {
		t.Fatal("full hold never switched the miner to the haul state")
	}
	if s.World.Alive(miner) {
		t.Fatal("miner never reached the station")
	}
	const near = StationID("cel-station-a")
	if got := bookAskQty(s, near, "ore-sterngrit"); got != 1000 {
		t.Fatalf("ore asks at the near station = %d, want the delivered 1000", got)
	}
	if got := bookAskQty(s, "cel-station-far", "ore-sterngrit"); got != 0 {
		t.Fatalf("ore asks at the far station = %d, want 0 (nearest wins)", got)
	}
	if got := s.Wallet(); got != wallet {
		t.Fatalf("wallet moved by the delivery: %v -> %v (a delivery must mint nothing)", wallet, got)
	}
}

// TestMinerWreckCarriesItsOre pins the ordinary wreck path: a killed miner
// leaves its hold behind as lootable ore.
func TestMinerWreckCarriesItsOre(t *testing.T) {
	s := newTestSim(t)
	miner := spawnMiner(t, s, playerPos(s).Add(v(60000, 0)))
	cargo, _ := s.Cargos.Get(miner)
	cargo.Ore = []content.CargoStack{{Item: "ore-sterngrit", Qty: 7}}
	s.Cargos.Set(miner, *cargo)
	drain(s)

	damageNPC(s, s.player, miner, content.Damage{EM: 1e9}, false)
	s.Step()
	drain(s)

	w := entityByName(s, "Fixture Miner Wreck")
	if w.IsZero() {
		t.Fatal("no wreck for the killed miner")
	}
	wreck, _ := s.Wrecks.Get(w)
	if got := lootQty(wreck.Loot, "ore-sterngrit"); got != 7 {
		t.Fatalf("wreck ore = %d, want the hold's 7", got)
	}
}

// TestMinerNeverTurnsOnThePlayer pins the disposition: the miner stays in its
// extraction loop and holds no lock even parked next to the player, and
// player damage does not aggro it either.
func TestMinerNeverTurnsOnThePlayer(t *testing.T) {
	s := newTestSim(t)
	beltPos := mustPos(s, s.celEntity("cel-belt-a"))
	miner := spawnMiner(t, s, beltPos)
	setPlayerPos(s, beltPos.Add(v(500, 0)))
	drain(s)

	stepAndCheck := func(ticks int) {
		for i := 0; i < ticks; i++ {
			s.Step()
			evs := drain(s)
			ai, ok := s.AIs.Get(miner)
			if !ok {
				return
			}
			if ai.State != NpcMine && ai.State != NpcHaul {
				t.Fatalf("miner state=%q, want mine/haul", ai.State)
			}
			if tg, ok := s.Targetings.Get(miner); ok && len(tg.Locks) != 0 {
				t.Fatalf("miner holds %d lock(s), want none", len(tg.Locks))
			}
			if findEvent(evs, EvLog, "is attacking you") != nil {
				t.Fatal("miner announced an attack")
			}
		}
	}
	stepAndCheck(600)
	damageNPC(s, s.player, miner, scratchDamage(), false)
	stepAndCheck(600)
	if infoHostile(s, miner) {
		t.Fatal("miner reads hostile")
	}
}
