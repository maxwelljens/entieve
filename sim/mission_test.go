package sim

import (
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
)

// ---------------------------------------------------------------------------
// Fixture: the shared test content plus a second station (deliver
// destination), a second rat class, and contracts of every kind.
// ---------------------------------------------------------------------------

func missionContent(t *testing.T) *content.Content {
	t.Helper()
	c := testContent()

	// Second station on sys-a: the deliver destination.
	sys := c.Systems["sys-a"]
	sys.Celestials = append(sys.Celestials, content.CelestialDef{
		ID: "cel-station-b", Name: "Waypoint Two", Kind: content.CelStation,
		Position: core.Vec2{X: 2e7, Y: -1e7}, Radius: 500,
		Station: &content.StationDef{Services: []string{"repair"}},
	})

	// A second rat class so "wrong class does not count" is observable.
	c.NPCs["npc-rat-marauder"] = &content.NpcDef{
		ID: "npc-rat-marauder", Name: "Rat Marauder", Faction: "Test",
		Ship:          "ship-rat",
		Fit:           []content.ModuleID{"mod-autocannon-s"},
		AggroRange:    15000,
		OrbitRange:    1500,
		FleeThreshold: 0,
		Bounty:        65000,
	}

	kill := func(id, title string, target content.NpcID, count int, reward, gain, minStanding float64, station content.CelID) *content.ContractDef {
		return &content.ContractDef{
			ID: content.ContractID(id), Title: title, Kind: content.ContractKill,
			Level: 1, MinStanding: minStanding, Station: station,
			Objective: content.ContractObjective{Target: target, Count: count},
			Reward:    content.ContractReward{Credits: reward, StandingGain: gain},
		}
	}
	c.Contracts = map[content.ContractID]*content.ContractDef{
		"contract-test-kill": kill("contract-test-kill", "Test Cleanup",
			"npc-rat-scuttler", 3, 30000, 0.1, 0, "cel-station-a"),
		"contract-test-marauder": kill("contract-test-marauder", "Test Marauder Hit",
			"npc-rat-marauder", 1, 2000, 0.1, 0, "cel-station-a"),
		"contract-test-gated": kill("contract-test-gated", "Test Gated Job",
			"npc-rat-scuttler", 1, 1000, 0, 0.5, "cel-station-a"),
		"contract-test-elsewhere": kill("contract-test-elsewhere", "Test Foreign Job",
			"npc-rat-marauder", 1, 1000, 0, 0, "cel-station-b"),
		"contract-test-mine": {
			ID: "contract-test-mine", Title: "Test Ore Order", Kind: content.ContractMine,
			Level: 1, Station: "cel-station-a",
			Objective: content.ContractObjective{Item: "ore-sterngrit", Count: 25},
			Reward:    content.ContractReward{Credits: 5000, StandingGain: 0.05},
		},
		"contract-test-deliver": {
			ID: "contract-test-deliver", Title: "Test Delivery", Kind: content.ContractDeliver,
			Level: 1, Station: "cel-station-a",
			Objective: content.ContractObjective{Item: "min-ferrite", Count: 1, Destination: "cel-station-b"},
			Reward:    content.ContractReward{Credits: 12000, StandingGain: 0.1},
		},
	}
	return c
}

func newMissionSim(t *testing.T) *Sim {
	t.Helper()
	s, err := New(missionContent(t), testWorld(), testSeed)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// dockNamed teleports the player onto a named station and docks (docking
// needs the 2,500 m session range).
func dockNamed(t *testing.T, s *Sim, name string) []Event {
	t.Helper()
	st := entityByName(s, name)
	if st.IsZero() {
		t.Fatalf("no station entity %q", name)
	}
	if pos, ok := s.posOf(st); ok {
		setPlayerPos(s, pos)
	}
	s.Queue(CmdDock{Station: st})
	s.Step()
	evs := drain(s)
	if !hasEvent(evs, EvDocked, "Docked at "+name) {
		t.Fatalf("docking at %s failed: %v", name, evs)
	}
	return evs
}

// undock leaves the station.
func undock(t *testing.T, s *Sim) {
	t.Helper()
	s.Queue(CmdUndock{})
	s.Step()
	if !hasEvent(drain(s), EvUndocked, "Undocked") {
		t.Fatal("undock failed")
	}
}

// acceptMission queues an acceptance and returns the drained events.
func acceptMission(t *testing.T, s *Sim, id content.ContractID) []Event {
	t.Helper()
	s.Queue(CmdAcceptMission{ID: id})
	s.Step()
	return drain(s)
}

// findMission returns the first EvMission event with the given phase.
func findMission(evs []Event, phase string) *Event {
	for i := range evs {
		if evs[i].Kind == EvMission && evs[i].Phase == phase {
			return &evs[i]
		}
	}
	return nil
}

// killOne spawns a rat next to the player and destroys it this tick.
func killOne(t *testing.T, s *Sim, npc content.NpcID) []Event {
	t.Helper()
	e := s.spawnNpc(npc, core.Vec2{X: 1e7, Y: 2000}, 100)
	if e.IsZero() {
		t.Fatalf("spawnNpc(%s) failed", npc)
	}
	damageNPC(s, s.player, e, contentDamage(), false)
	s.Step()
	return drain(s)
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// TestKillContractLifecycle pins the full kill loop: accept at the board,
// progress by rat class, ready, auto-claim on docking at the offering
// station, exact wallet + standing credit.
func TestKillContractLifecycle(t *testing.T) {
	s := newMissionSim(t)
	dockNamed(t, s, "Haven Rest")

	evs := acceptMission(t, s, "contract-test-kill")
	if findMission(evs, MissionAccepted) == nil {
		t.Fatalf("no accepted event: %v", evs)
	}
	if len(s.Missions()) != 1 || s.Missions()[0].Progress != 0 {
		t.Fatalf("missions after accept = %+v", s.Missions())
	}
	undock(t, s)

	for i := 1; i <= 3; i++ {
		evs = killOne(t, s, "npc-rat-scuttler")
		m := s.Missions()
		if len(m) != 1 || m[0].Progress != float64(i) {
			t.Fatalf("after kill %d: missions=%+v", i, m)
		}
		pr := findMission(evs, MissionProgress)
		if pr == nil || !strings.Contains(pr.Text, "Contract progress: Test Cleanup") {
			t.Fatalf("kill %d: missing progress event: %v", i, evs)
		}
		if i < 3 && findMission(evs, MissionReady) != nil {
			t.Fatalf("ready fired before objectives met: %v", evs)
		}
	}
	if findMission(drain(s), MissionReady) != nil {
		t.Fatal("ready already queued from the previous step")
	}

	wallet := s.Wallet()
	dockNamed(t, s, "Haven Rest")
	// The ready event was emitted with the third kill; the dock emits claim.
	// (Events from kill 3 were drained above, so re-check via state.)
	if len(s.Missions()) != 0 {
		t.Fatalf("missions after claim = %+v", s.Missions())
	}
	if got := s.Wallet(); got != wallet+30000 {
		t.Fatalf("wallet=%v want %v (kill reward 30000)", got, wallet+30000)
	}
	if s.Standing() < 0.1-1e-9 {
		t.Fatalf("standing=%v want >= 0.1", s.Standing())
	}
}

// TestKillContractReadyEvent pins that readiness is announced when the last
// objective lands (before the player docks).
func TestKillContractReadyEvent(t *testing.T) {
	s := newMissionSim(t)
	dockNamed(t, s, "Haven Rest")
	acceptMission(t, s, "contract-test-kill")
	undock(t, s)
	var evs []Event
	for i := 0; i < 3; i++ {
		evs = append(evs, killOne(t, s, "npc-rat-scuttler")...)
	}
	ready := findMission(evs, MissionReady)
	if ready == nil || !strings.Contains(ready.Text, "dock at Haven Rest to claim") {
		t.Fatalf("missing ready event: %v", evs)
	}
	if ready.ID != "contract-test-kill" || ready.Name != "Test Cleanup" {
		t.Fatalf("ready event id/name = %q/%q", ready.ID, ready.Name)
	}
}

// TestMineContractLifecycle pins ore-volume progress and the claim payout.
func TestMineContractLifecycle(t *testing.T) {
	s := newMissionSim(t)
	dockNamed(t, s, "Haven Rest")
	if evs := acceptMission(t, s, "contract-test-mine"); findMission(evs, MissionAccepted) == nil {
		t.Fatalf("no accepted event: %v", evs)
	}
	undock(t, s)

	rock := firstAsteroid(t, s)
	rp, _ := s.posOf(rock)
	setPlayerPos(s, rp.Add(v(800, 0)))
	laser := injectModule(t, s, "mod-mining-laser")
	s.Queue(CmdMine{Asteroid: rock, Slot: laser})
	s.Step()
	drain(s)

	var evs []Event
	for i := 0; i < 3; i++ { // 10 m³ per 3 s cycle: 10 -> 20 -> 25 (clamped)
		stepN(s, 180)
		evs = append(evs, drain(s)...)
	}
	m := s.Missions()
	if len(m) != 1 || m[0].Progress != 25 {
		t.Fatalf("mine progress = %+v want 25/25", m)
	}
	if findMission(evs, MissionProgress) == nil || findMission(evs, MissionReady) == nil {
		t.Fatalf("missing mine progress/ready events: %v", evs)
	}

	wallet := s.Wallet()
	dockNamed(t, s, "Haven Rest")
	if len(s.Missions()) != 0 {
		t.Fatalf("missions after claim = %+v", s.Missions())
	}
	if got := s.Wallet(); got != wallet+5000 {
		t.Fatalf("wallet=%v want %v (mine reward 5000)", got, wallet+5000)
	}
	if s.Standing() < 0.05-1e-9 {
		t.Fatalf("standing=%v want >= 0.05", s.Standing())
	}
}

// TestDeliverContractLifecycle pins: cargo counted only at the destination,
// consumed on completion, payout + standing credited.
func TestDeliverContractLifecycle(t *testing.T) {
	s := newMissionSim(t)
	dockNamed(t, s, "Haven Rest")
	if evs := acceptMission(t, s, "contract-test-deliver"); findMission(evs, MissionAccepted) == nil {
		t.Fatalf("no accepted event: %v", evs)
	}
	undock(t, s)
	giveCargo(s, "min-ferrite", 10) // 10 x 0.1 m³ = the 1 m³ objective

	// Docking at the OFFERING station must not claim a deliver contract.
	dockNamed(t, s, "Haven Rest")
	if len(s.Missions()) != 1 {
		t.Fatalf("deliver claimed at the offering station: %+v", s.Missions())
	}
	if n := cargoCount(s, "min-ferrite"); n != 10 {
		t.Fatalf("cargo consumed at wrong station: %d", n)
	}
	undock(t, s)

	wallet := s.Wallet()
	st := entityByName(s, "Waypoint Two")
	if st.IsZero() {
		t.Fatal("no destination station entity")
	}
	if pos, ok := s.posOf(st); ok {
		setPlayerPos(s, pos)
	}
	s.Queue(CmdDock{Station: st})
	s.Step()
	evs := drain(s)

	if findMission(evs, MissionCompleted) == nil {
		t.Fatalf("no completed event at destination: %v", evs)
	}
	if len(s.Missions()) != 0 {
		t.Fatalf("missions after claim = %+v", s.Missions())
	}
	if n := cargoCount(s, "min-ferrite"); n != 0 {
		t.Fatalf("delivered cargo not consumed: %d left", n)
	}
	if got := s.Wallet(); got != wallet+12000 {
		t.Fatalf("wallet=%v want %v (deliver reward 12000)", got, wallet+12000)
	}
	if s.Standing() < 0.1-1e-9 {
		t.Fatalf("standing=%v want >= 0.1", s.Standing())
	}
}

// ---------------------------------------------------------------------------
// Refusals and abandonment
// ---------------------------------------------------------------------------

func TestMissionRefusalPaths(t *testing.T) {
	t.Run("not docked", func(t *testing.T) {
		s := newMissionSim(t) // starts undocked
		evs := acceptMission(t, s, "contract-test-kill")
		ref := findMission(evs, MissionRefused)
		if ref == nil || !strings.Contains(ref.Text, "docked to accept") {
			t.Fatalf("expected docked refusal, got %v", evs)
		}
		if len(s.Missions()) != 0 {
			t.Fatal("mission accepted while undocked")
		}
	})

	t.Run("unknown id", func(t *testing.T) {
		s := newMissionSim(t)
		dockNamed(t, s, "Haven Rest")
		evs := acceptMission(t, s, "contract-test-nope")
		ref := findMission(evs, MissionRefused)
		if ref == nil || !strings.Contains(ref.Text, "No such contract contract-test-nope") {
			t.Fatalf("expected unknown-contract refusal, got %v", evs)
		}
	})

	t.Run("wrong station", func(t *testing.T) {
		s := newMissionSim(t)
		dockNamed(t, s, "Haven Rest")
		evs := acceptMission(t, s, "contract-test-elsewhere") // offered at Waypoint Two
		ref := findMission(evs, MissionRefused)
		if ref == nil || !strings.Contains(ref.Text, "not offered here") {
			t.Fatalf("expected board refusal, got %v", evs)
		}
	})

	t.Run("insufficient standing", func(t *testing.T) {
		s := newMissionSim(t)
		dockNamed(t, s, "Haven Rest")
		evs := acceptMission(t, s, "contract-test-gated")
		ref := findMission(evs, MissionRefused)
		if ref == nil || !strings.Contains(ref.Text, "Requires standing 0.50") {
			t.Fatalf("expected standing refusal, got %v", evs)
		}
	})

	t.Run("duplicate accept", func(t *testing.T) {
		s := newMissionSim(t)
		dockNamed(t, s, "Haven Rest")
		acceptMission(t, s, "contract-test-kill")
		evs := acceptMission(t, s, "contract-test-kill")
		ref := findMission(evs, MissionRefused)
		if ref == nil || !strings.Contains(ref.Text, "already accepted") {
			t.Fatalf("expected duplicate refusal, got %v", evs)
		}
		if len(s.Missions()) != 1 {
			t.Fatalf("missions = %+v want exactly 1", s.Missions())
		}
	})
}

// TestAbandonMissionMidProgress pins: abandon works at the board, removes
// progress, refuses elsewhere and for unknown/inactive ids, and prevents a
// later claim.
func TestAbandonMissionMidProgress(t *testing.T) {
	s := newMissionSim(t)
	dockNamed(t, s, "Haven Rest")
	acceptMission(t, s, "contract-test-kill")
	undock(t, s)
	killOne(t, s, "npc-rat-scuttler")
	if s.Missions()[0].Progress != 1 {
		t.Fatalf("progress = %+v want 1", s.Missions())
	}

	// Abandoning while undocked is refused.
	s.Queue(CmdAbandonMission{ID: "contract-test-kill"})
	s.Step()
	evs := drain(s)
	if ref := findMission(evs, MissionRefused); ref == nil || !strings.Contains(ref.Text, "docked to abandon") {
		t.Fatalf("expected docked refusal, got %v", evs)
	}

	// Abandoning from a station that does not issue it is refused.
	st := entityByName(s, "Waypoint Two")
	if pos, ok := s.posOf(st); ok {
		setPlayerPos(s, pos)
	}
	s.Queue(CmdDock{Station: st})
	s.Step()
	drain(s)
	s.Queue(CmdAbandonMission{ID: "contract-test-kill"})
	s.Step()
	evs = drain(s)
	if ref := findMission(evs, MissionRefused); ref == nil || !strings.Contains(ref.Text, "not offered here") {
		t.Fatalf("expected board refusal, got %v", evs)
	}

	// Unknown/inactive ids are refused at the issuing board.
	undock(t, s)
	dockNamed(t, s, "Haven Rest")
	s.Queue(CmdAbandonMission{ID: "contract-test-nope"})
	s.Step()
	evs = drain(s)
	if ref := findMission(evs, MissionRefused); ref == nil || !strings.Contains(ref.Text, "No active contract") {
		t.Fatalf("expected inactive refusal, got %v", evs)
	}

	// Real abandon at the issuing board.
	wallet := s.Wallet()
	s.Queue(CmdAbandonMission{ID: "contract-test-kill"})
	s.Step()
	evs = drain(s)
	if ab := findMission(evs, MissionAbandoned); ab == nil || !strings.Contains(ab.Text, "Contract abandoned: Test Cleanup") {
		t.Fatalf("no abandoned event: %v", evs)
	}
	if len(s.Missions()) != 0 {
		t.Fatalf("missions after abandon = %+v", s.Missions())
	}
	if s.Wallet() != wallet {
		t.Fatal("abandon must not pay")
	}
	if s.Standing() != 0 {
		t.Fatalf("standing=%v want 0 (abandon must not grant standing)", s.Standing())
	}
}

// ---------------------------------------------------------------------------
// Objective accounting
// ---------------------------------------------------------------------------

// TestKillCountsOnlyMatchingClass pins that kills of the wrong rat class do
// not advance a kill contract (but do advance the matching one).
func TestKillCountsOnlyMatchingClass(t *testing.T) {
	s := newMissionSim(t)
	dockNamed(t, s, "Haven Rest")
	acceptMission(t, s, "contract-test-kill")     // scuttler x3
	acceptMission(t, s, "contract-test-marauder") // marauder x1
	undock(t, s)

	for i := 0; i < 3; i++ {
		killOne(t, s, "npc-rat-scuttler")
	}
	var scuttler, marauder float64
	for _, m := range s.Missions() {
		switch m.ID {
		case "contract-test-kill":
			scuttler = m.Progress
		case "contract-test-marauder":
			marauder = m.Progress
		}
	}
	if scuttler != 3 {
		t.Fatalf("scuttler contract progress = %v want 3", scuttler)
	}
	if marauder != 0 {
		t.Fatalf("marauder contract progress = %v want 0 (wrong class counted)", marauder)
	}

	// The dock claims only the matching contract.
	wallet := s.Wallet()
	dockNamed(t, s, "Haven Rest")
	if s.Wallet() != wallet+30000 {
		t.Fatalf("wallet delta = %v want 30000 (only the scuttler contract)", s.Wallet()-wallet)
	}
	if len(s.Missions()) != 1 || s.Missions()[0].ID != "contract-test-marauder" {
		t.Fatalf("missions = %+v want only the marauder contract", s.Missions())
	}
}

// ---------------------------------------------------------------------------
// Persistence and determinism
// ---------------------------------------------------------------------------

// TestMissionSaveRestoreRoundTrip pins that a mid-progress mission (2 of 3
// kills) and the standing scalar survive Snapshot -> Restore exactly, and
// that the restored sim can finish the contract.
func TestMissionSaveRestoreRoundTrip(t *testing.T) {
	s := newMissionSim(t)
	dockNamed(t, s, "Haven Rest")
	acceptMission(t, s, "contract-test-kill")
	undock(t, s)
	killOne(t, s, "npc-rat-scuttler")
	killOne(t, s, "npc-rat-scuttler")
	if s.Missions()[0].Progress != 2 {
		t.Fatalf("progress = %v want 2", s.Missions()[0].Progress)
	}
	wantHash := s.Hash()
	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	b := newMissionSim(t)
	if err := b.Restore(save); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if b.Hash() != wantHash {
		t.Fatalf("hash after restore=%016x want %016x", b.Hash(), wantHash)
	}
	m := b.Missions()
	if len(m) != 1 || m[0].ID != "contract-test-kill" || m[0].Progress != 2 {
		t.Fatalf("restored missions = %+v want [kill 2/3]", m)
	}
	if b.Standing() != s.Standing() {
		t.Fatalf("standing = %v want %v", b.Standing(), s.Standing())
	}

	// The restored sim finishes the contract: 2/3 -> 3/3 -> claim.
	killOne(t, b, "npc-rat-scuttler")
	if b.Missions()[0].Progress != 3 {
		t.Fatalf("progress after restore+kill = %+v want 3", b.Missions())
	}
	wallet := b.Wallet()
	dockNamed(t, b, "Haven Rest")
	if len(b.Missions()) != 0 || b.Wallet() != wallet+30000 {
		t.Fatalf("claim after restore failed: missions=%+v wallet=%v", b.Missions(), b.Wallet()-wallet)
	}
}

// missionScriptTick drives a twin-run determinism check over a full contract
// loop: dock, accept, undock, three kills, dock to claim.
func missionScriptTick(s *Sim, i int) {
	switch i {
	case 2:
		if st := entityByName(s, "Haven Rest"); !st.IsZero() {
			s.Queue(CmdDock{Station: st})
		}
	case 4:
		s.Queue(CmdAcceptMission{ID: "contract-test-kill"})
	case 6:
		s.Queue(CmdUndock{})
	case 10, 14, 18:
		npc := s.spawnNpc("npc-rat-scuttler", core.Vec2{X: 1e7, Y: 2000}, 100)
		if !npc.IsZero() {
			damageNPC(s, s.player, npc, contentDamage(), false)
		}
	case 22:
		if st := entityByName(s, "Haven Rest"); !st.IsZero() {
			if pos, ok := s.posOf(st); ok {
				setPlayerPos(s, pos)
			}
			s.Queue(CmdDock{Station: st})
		}
	}
	s.Step()
}

// TestMissionTwinRunsIdentical pins that a scripted mission loop hashes
// identically across two seeded runs at every checkpoint.
func TestMissionTwinRunsIdentical(t *testing.T) {
	checkpoints := map[int]bool{1: true, 5: true, 9: true, 21: true, 30: true}
	run := func() (map[int]uint64, *Sim) {
		s, err := New(missionContent(t), testWorld(), 99)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		out := make(map[int]uint64)
		if checkpoints[0] {
			out[0] = s.Hash()
		}
		for i := 0; i < 30; i++ {
			missionScriptTick(s, i)
			if checkpoints[i+1] {
				out[i+1] = s.Hash()
			}
		}
		return out, s
	}
	a, sa := run()
	b, sb := run()
	for cp := range checkpoints {
		if a[cp] != b[cp] {
			t.Fatalf("checkpoint %d: hashes differ (%016x vs %016x)", cp, a[cp], b[cp])
		}
	}
	// The scripted run actually completed the loop in both sims.
	if len(sa.Missions()) != 0 || len(sb.Missions()) != 0 {
		t.Fatalf("mission not completed by the script: %v / %v", sa.Missions(), sb.Missions())
	}
	if sa.Wallet() != sb.Wallet() || sa.Standing() != sb.Standing() {
		t.Fatalf("payout diverged: wallet %v/%v standing %v/%v",
			sa.Wallet(), sb.Wallet(), sa.Standing(), sb.Standing())
	}
}
