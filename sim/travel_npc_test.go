package sim

import (
	"math"
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// ---------------------------------------------------------------------------
// Travel
// ---------------------------------------------------------------------------

func TestDockUndockRoundTrip(t *testing.T) {
	s := newTestSim(t)
	station := entityByName(s, "Haven Rest")
	stPos, _ := s.posOf(station)

	// Player is on grid before docking.
	if !gridHas(s, s.player) {
		t.Fatal("player should be on grid")
	}
	s.Queue(CmdDock{Station: station})
	s.Step()
	evs := drain(s)
	if !hasEvent(evs, EvDocked, "Docked at Haven Rest") {
		t.Fatalf("missing dock event: %v", evs)
	}
	if !s.Docked.Has(s.player) {
		t.Fatal("Docked marker missing")
	}
	if gridHas(s, s.player) {
		t.Fatal("Grid() must hide docked entities")
	}
	if d, _ := s.posOf(s.player); d != stPos {
		t.Fatalf("docked pos=%v want %v", d, stPos)
	}
	// Docked players cannot warp.
	s.Queue(CmdWarpTo{Target: station})
	s.Step()
	drain(s)
	if s.Warps.Has(s.player) {
		t.Fatal("docked warp should be rejected")
	}

	// Undock: 5000 m off along +X.
	s.Queue(CmdUndock{})
	s.Step()
	evs = drain(s)
	if !hasEvent(evs, EvUndocked, "Undocked from Haven Rest") {
		t.Fatalf("missing undock event: %v", evs)
	}
	if s.Docked.Has(s.player) {
		t.Fatal("still docked")
	}
	if !gridHas(s, s.player) {
		t.Fatal("player should be back on grid")
	}
	want := stPos.Add(core.Vec2{X: 5000})
	if playerPos(s) != want {
		t.Fatalf("undock pos=%v want %v", playerPos(s), want)
	}
	// Undock again warns.
	s.Queue(CmdUndock{})
	s.Step()
	evs = drain(s)
	if findEvent(evs, EvLog, "not docked") == nil {
		t.Fatalf("expected warning: %v", evs)
	}
}

func TestDockRangeEnforced(t *testing.T) {
	s := newTestSim(t)
	station := entityByName(s, "Haven Rest")
	setPlayerPos(s, v(1e7+10000, 0)) // 10 km away
	s.Queue(CmdDock{Station: station})
	s.Step()
	evs := drain(s)
	if s.Docked.Has(s.player) {
		t.Fatal("docked from 10 km")
	}
	if findEvent(evs, EvLog, "out of range") == nil {
		t.Fatalf("expected range warning: %v", evs)
	}
}

func TestJumpRebuildsSystem(t *testing.T) {
	s := newTestSim(t)
	gate := entityByName(s, "Gate to Beta")
	setPlayerPos(s, gatePos(s, gate).Add(v(100, 0)))

	s.Queue(CmdJump{Gate: gate})
	s.Step()
	evs := drain(s)
	if !hasEvent(evs, EvJumped, "Jumped to Fixture Beta") {
		t.Fatalf("missing jump event: %v", evs)
	}
	if s.System().ID != "sys-b" {
		t.Fatalf("system=%s", s.System().ID)
	}
	// Player identity, ship and wallet survive the jump.
	ship, _ := s.Ships.Get(s.player)
	if ship.Hull != "ship-sloop" {
		t.Fatal("player ship lost on jump")
	}
	if s.Wallet() != 100000 {
		t.Fatal("wallet lost on jump")
	}
	if cargoCount(s, "ammo-emp-s") != 20 {
		t.Fatal("cargo lost on jump")
	}
	// Player placed at the destination gate.
	destGate := entityByName(s, "Gate to Alpha")
	if destGate.IsZero() {
		t.Fatal("destination gate missing")
	}
	destPos, _ := s.posOf(destGate)
	if playerPos(s) != destPos {
		t.Fatalf("arrival pos=%v want %v", playerPos(s), destPos)
	}
	// The new grid was rebuilt: rocks and NPCs of sys-b exist; sys-a's do not.
	if len(sortedEntities(s.Asteroids)) != 1 {
		t.Fatalf("rocks=%d want 1", len(sortedEntities(s.Asteroids)))
	}
	if entityByName(s, "Haven Rest") != 0 {
		t.Fatal("old system's station still on grid")
	}
	npcCount := 0
	for _, e := range sortedEntities(s.AIs) {
		if s.World.Alive(e) {
			npcCount++
		}
	}
	if npcCount != 1 {
		t.Fatalf("npcs=%d want 1 (sys-b belt spawn)", npcCount)
	}
	// Jumping back rebuilds sys-a.
	backGate := entityByName(s, "Gate to Alpha")
	setPlayerPos(s, gatePos(s, backGate).Add(v(50, 0)))
	s.Queue(CmdJump{Gate: backGate})
	s.Step()
	drain(s)
	if s.System().ID != "sys-a" {
		t.Fatalf("system after return=%s", s.System().ID)
	}
	if entityByName(s, "Haven Rest") == 0 {
		t.Fatal("station missing after return")
	}
	if len(sortedEntities(s.Asteroids)) != 2 {
		t.Fatalf("sys-a rocks=%d want 2", len(sortedEntities(s.Asteroids)))
	}
}

func TestJumpRangeAndDockedGuards(t *testing.T) {
	s := newTestSim(t)
	gate := entityByName(s, "Gate to Beta")
	// Too far.
	setPlayerPos(s, v())
	s.Queue(CmdJump{Gate: gate})
	s.Step()
	evs := drain(s)
	if findEvent(evs, EvLog, "out of range") == nil {
		t.Fatalf("expected range warning: %v", evs)
	}
	// Docked.
	station := entityByName(s, "Haven Rest")
	setPlayerPos(s, gatePos(s, station))
	s.Queue(CmdDock{Station: station})
	s.Step()
	drain(s)
	s.Queue(CmdJump{Gate: gate})
	s.Step()
	evs = drain(s)
	if findEvent(evs, EvLog, "Cannot jump while docked") == nil {
		t.Fatalf("expected docked warning: %v", evs)
	}
}

// ---------------------------------------------------------------------------
// Respawn
// ---------------------------------------------------------------------------

// anomalyNPC returns the NPC spawned at the anomaly celestial.
func anomalyNPC(t *testing.T, s *Sim) ecs.Entity {
	t.Helper()
	// The nearest celestial of the NPC's Home anchor is its spawn celestial.
	for _, e := range sortedEntities(s.AIs) {
		ai, _ := s.AIs.Get(e)
		near := s.nearestCelestial(ai.Home)
		c, ok := s.Celestials.Get(near)
		if ok && c.Cel == "cel-anomaly-a" {
			return e
		}
	}
	t.Fatal("no anomaly NPC")
	return 0
}

func TestRespawnTimerExpiresAndSpawns(t *testing.T) {
	s := newTestSim(t)
	npc := anomalyNPC(t, s)
	damageNPC(s, s.player, npc, contentDamage(), false)
	s.Step()
	evs := drain(s)
	if !hasEvent(evs, EvDestroyed, "Rat Scuttler destroyed") {
		t.Fatalf("missing destroy event: %v", evs)
	}
	if len(s.respawns) != 1 {
		t.Fatalf("timers=%d want 1", len(s.respawns))
	}
	rt := s.respawns[0]
	if rt.System != "sys-a" || rt.Cel != "cel-anomaly-a" || rt.Npc != "npc-rat-scuttler" {
		t.Fatalf("timer=%+v", rt)
	}
	if math.Abs(rt.At-(s.time+60)) > DT {
		t.Fatalf("timer at=%v want %v", rt.At, s.time+60)
	}
	// Anomaly cleared event (single-member group).
	if findEvent(evs, EvAnomalyCleared, "Fixture Den cleared") == nil {
		t.Fatalf("missing anomaly-cleared event: %v", evs)
	}

	// Not yet.
	stepN(s, 60*59)
	if len(s.respawns) != 1 {
		t.Fatalf("timer fired early: %d", len(s.respawns))
	}
	// 60s after the kill.
	stepN(s, 60*2)
	if len(s.respawns) != 0 {
		t.Fatalf("timer did not fire: %+v", s.respawns)
	}
	// The replacement warps in from off-grid: it exists (and has spoken)
	// at timer time, but it takes align + tunnel to land at the anchor.
	// Wait out the slide, then it sits near the anomaly.
	stepN(s, 60*20)
	home := v(5e6, 5e6)
	found := false
	for _, e := range sortedEntities(s.AIs) {
		tr, _ := s.Transforms.Get(e)
		if tr.Pos.Dist(home) <= celestialSpreadRadius(s, "cel-anomaly-a") {
			found = true
		}
	}
	if !found {
		t.Fatal("NPC did not respawn near the anomaly")
	}
}

// celestialSpreadRadius returns the scatter radius of a celestial.
func celestialSpreadRadius(s *Sim, cel content.CelID) float64 {
	for i := range s.System().Celestials {
		if s.System().Celestials[i].ID == cel {
			return celestialSpread(&s.System().Celestials[i])
		}
	}
	return 0
}

func TestRespawnTimerSurvivesJump(t *testing.T) {
	s := newTestSim(t)
	npc := anomalyNPC(t, s)
	damageNPC(s, s.player, npc, contentDamage(), false)
	s.Step()
	drain(s)
	if len(s.respawns) != 1 {
		t.Fatalf("timers=%d", len(s.respawns))
	}

	// Jump to sys-b; the timer must persist (it belongs to sys-a).
	gate := entityByName(s, "Gate to Beta")
	setPlayerPos(s, gatePos(s, gate))
	s.Queue(CmdJump{Gate: gate})
	s.Step()
	drain(s)
	if s.System().ID != "sys-b" {
		t.Fatal("jump failed")
	}
	// Ticking in sys-b must not fire the sys-a timer.
	stepN(s, 60*120)
	if len(s.respawns) != 1 {
		t.Fatalf("sys-a timer fired while abroad: %d", len(s.respawns))
	}

	// Jump back: the (long overdue) timer fires on the next respawn tick.
	backGate := entityByName(s, "Gate to Alpha")
	setPlayerPos(s, gatePos(s, backGate))
	s.Queue(CmdJump{Gate: backGate})
	s.Step()
	drain(s)
	if len(s.respawns) != 0 {
		t.Fatalf("timer did not fire on return: %+v", s.respawns)
	}
	// The replacement warps in from off-grid: wait out the align + tunnel
	// slide before looking for it near the anchor.
	stepN(s, 60*20)
	home := v(5e6, 5e6)
	found := false
	for _, e := range sortedEntities(s.AIs) {
		tr, _ := s.Transforms.Get(e)
		if tr.Pos.Dist(home) <= 3000 {
			found = true
		}
	}
	if !found {
		t.Fatal("respawned NPC missing near anomaly")
	}
}

// TestRespawnEmitsEvent pins the observation surface: exactly one info event
// per respawn, naming the NPC and the celestial, with the new NPC as actor.
func TestRespawnEmitsEvent(t *testing.T) {
	s := newTestSim(t)
	npc := anomalyNPC(t, s)
	damageNPC(s, s.player, npc, contentDamage(), false)
	s.Step()
	drain(s)

	stepN(s, 60*61) // the 60 s timer fires inside this window
	evs := drain(s)

	const want = "Rat Scuttler respawned near Fixture Den"
	n := 0
	for _, ev := range evs {
		if ev.Kind == EvLog && ev.Text == want {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("respawn events=%d want exactly 1: %v", n, evs)
	}
	ev := findEvent(evs, EvLog, want)
	if ev.Severity != SevInfo {
		t.Fatalf("severity=%d want SevInfo", ev.Severity)
	}
	if ev.Actor.IsZero() || !s.AIs.Has(ev.Actor) {
		t.Fatalf("actor=%v want the respawned NPC", ev.Actor)
	}
}

// ---------------------------------------------------------------------------
// NPC AI
// ---------------------------------------------------------------------------

func TestNpcAggroOrbitsAndFightsBack(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayerAt(t, s, v(1e7, 6000)) // 5.5 km out
	ai, _ := s.AIs.Get(npc)

	s.Step()
	if ai.State != NpcApproach {
		t.Fatalf("state=%q want approach", ai.State)
	}
	evs := drain(s)
	if findEvent(evs, EvLog, "is attacking you") == nil {
		t.Fatalf("missing aggro warning: %v", evs)
	}
	if !infoHostile(s, npc) {
		t.Fatal("Info.Hostile should be true for aggroed NPC")
	}

	// It closes in, locking and landing shots on the way (hits are near-
	// guaranteed during the radial approach; orbit tracking makes them
	// rare afterwards).
	shieldBefore := shieldOf(t, s, s.player)
	damaged, reached := false, false
	for i := 0; i < 3000; i++ {
		s.Step()
		drain(s)
		if shieldOf(t, s, s.player) < shieldBefore {
			damaged = true
		}
		if ai.State == NpcOrbit {
			reached = true
			if damaged {
				break
			}
		}
	}
	if !reached {
		t.Fatalf("NPC never reached orbit (state=%q dist=%v)", ai.State,
			s.distBetween(npc, s.player))
	}
	if !damaged {
		t.Fatal("NPC never damaged the player (lock/weapon chain broken?)")
	}
}

func TestNpcReturnHomeWhenPlayerLeaves(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayerAt(t, s, v(1e7, 4000))
	ai, _ := s.AIs.Get(npc)
	home := ai.Home

	stepN(s, 10)
	drain(s)
	if ai.State != NpcApproach && ai.State != NpcOrbit {
		t.Fatalf("state=%q want aggro", ai.State)
	}
	// Player leaves the aggro range.
	setPlayerPos(s, v(1e7+30000, 500))
	s.Step()
	drain(s)
	if ai.State != NpcReturn {
		t.Fatalf("state=%q want return", ai.State)
	}
	// It flies home and settles.
	settled := false
	for i := 0; i < 3600; i++ {
		s.Step()
		drain(s)
		tr, _ := s.Transforms.Get(npc)
		if ai.State == NpcIdle && tr.Pos.Dist(home) <= npcReturnRange {
			settled = true
			break
		}
	}
	if !settled {
		tr, _ := s.Transforms.Get(npc)
		t.Fatalf("NPC never returned: state=%q dist=%v", ai.State, tr.Pos.Dist(home))
	}
}

func TestNpcFleeDespawnsAfter20Seconds(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayerAt(t, s, v(1e7, 4000))
	ai, _ := s.AIs.Get(npc)
	stepN(s, 10) // aggro
	drain(s)

	// Structure below FleeThreshold (0.25 * 100 = 25) without dying:
	// 380 raw damage -> shield 200, armor 100 gone, structure 20.
	damageNPC(s, s.player, npc, structDamage(), false)
	s.Step()
	evs := drain(s)
	if ai.State != NpcFlee {
		t.Fatalf("state=%q want flee", ai.State)
	}
	if findEvent(evs, EvLog, "is fleeing") == nil {
		t.Fatalf("missing flee event: %v", evs)
	}
	if !infoHostile(s, npc) {
		t.Fatal("fleeing NPC still counts as hostile")
	}

	// It burns away for 20s, then despawns quietly.
	settle := false
	for i := 0; i < 22*60; i++ {
		s.Step()
		for _, ev := range drain(s) {
			if hasText(ev, "retreats and escapes") {
				settle = true
			}
		}
		if !s.World.Alive(npc) {
			break
		}
	}
	if s.World.Alive(npc) {
		t.Fatal("fleeing NPC did not despawn")
	}
	if !settle {
		t.Fatal("missing escape event")
	}
}

func TestNpcDoesNotAggroOutOfRange(t *testing.T) {
	s := newTestSim(t)
	npc := spawnNearPlayerAt(t, s, v(1e7, 60000)) // beyond 15 km aggro
	ai, _ := s.AIs.Get(npc)
	stepN(s, 300)
	if ai.State != NpcIdle {
		t.Fatalf("state=%q want idle", ai.State)
	}
	if infoHostile(s, npc) {
		t.Fatal("idle NPC must not be hostile")
	}
	// Damaging it aggros even from range.
	damageNPC(s, s.player, npc, scratchDamage(), false)
	s.Step()
	drain(s)
	if ai.State != NpcApproach {
		t.Fatalf("state=%q want approach after damage", ai.State)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func gridHas(s *Sim, e ecs.Entity) bool {
	for _, info := range s.Grid() {
		if info.Entity == e {
			return true
		}
	}
	return false
}

func infoHostile(s *Sim, e ecs.Entity) bool {
	info, ok := s.Info(e)
	return ok && info.Hostile
}

func shieldOf(t *testing.T, s *Sim, e ecs.Entity) float64 {
	t.Helper()
	sh, ok := s.Ships.Get(e)
	if !ok {
		t.Fatal("no ship")
	}
	return sh.Shield
}

// spawnNearPlayerAt spawns an NPC at an exact anchor (Home = anchor).
func spawnNearPlayerAt(t *testing.T, s *Sim, anchor core.Vec2) ecs.Entity {
	t.Helper()
	e := s.spawnNpc("npc-rat-scuttler", anchor, 100)
	if e.IsZero() {
		t.Fatal("spawnNpc failed")
	}
	return e
}

// contentDamage is a generic lethal hit.
func contentDamage() content.Damage { return content.Damage{EM: 10000} }

// structDamage deals 380 raw: shield 200 + armor 100 + 80 structure, leaving
// structure at 20 (below the 0.25 flee threshold) without killing.
func structDamage() content.Damage { return content.Damage{EM: 380} }

// scratchDamage chips the shield without threatening the hull.
func scratchDamage() content.Damage { return content.Damage{EM: 50} }

func hasText(ev Event, sub string) bool {
	return strings.Contains(ev.Text, sub)
}
