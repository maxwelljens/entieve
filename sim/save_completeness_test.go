package sim

import (
	"reflect"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
)

// ---------------------------------------------------------------------------
// F5 — save completeness is a check, not prose. Every field of Sim is
// classified once (below) as carried by Save / derived from Save, per-Step
// transient, or an explicit documented exception; the tests then fail when a
// new field arrives unclassified, when a claimed Save field disappears, and
// when a transient survives a Step boundary.
// ---------------------------------------------------------------------------

// fieldClass is one Sim field's place in the persistence contract.
type fieldClass struct {
	save      []string // Save fields carrying this state (Hash fingerprints them)
	derived   bool     // recomputed from Save on restore (no Save field of its own)
	transient bool     // per-Step working state: empty at every Step boundary
	exempt    string   // non-empty: intentionally outside Save, with the reason
}

// simFieldClasses classifies EVERY field of Sim (see TestSimSaveCompleteness,
// which fails on any unclassified or stale entry).
var simFieldClasses = map[string]fieldClass{
	// Read surfaces rebuilt from Save.
	"Content":    {derived: true}, // resolved from Save.World/Seed
	"World":      {derived: true}, // rebuilt from Save.Entities + Save.FreeSlots
	"RNG":        {save: []string{"RNGState"}},
	"Hangar":     {save: []string{"Hangar"}},
	"Names":      {save: []string{"Entities"}},
	"Transforms": {save: []string{"Entities"}},
	"Intents":    {save: []string{"Entities"}},
	"Motions":    {save: []string{"Entities"}},
	"Ships":      {save: []string{"Entities"}},
	"Caps":       {save: []string{"Entities"}},
	"Cargos":     {save: []string{"Entities"}},
	"Fittings":   {save: []string{"Entities"}},
	"Targetings": {save: []string{"Entities"}},
	"Pilots":     {save: []string{"Entities"}},
	"AIs":        {save: []string{"Entities"}},
	"Asteroids":  {save: []string{"Entities"}},
	"Celestials": {save: []string{"Entities"}},
	"Gates":      {save: []string{"Entities"}},
	"Stations":   {save: []string{"Entities"}},
	"Docked":     {save: []string{"Entities"}},
	"Warps":      {save: []string{"Entities"}},
	"Wrecks":     {save: []string{"Entities"}},
	"Traffic":    {save: []string{"Entities"}},
	"Patrols":    {save: []string{"Entities"}},

	// Unowned simulation state.
	"trafficRNG":    {save: []string{"TrafficRNGState"}},
	"worldID":       {save: []string{"World"}},
	"systemID":      {save: []string{"System"}},
	"seed":          {save: []string{"Seed"}},
	"ticks":         {save: []string{"Ticks"}},
	"time":          {derived: true}, // restore: Save.Ticks * DT
	"wallet":        {save: []string{"Wallet"}},
	"standing":      {save: []string{"Standing"}},
	"missions":      {save: []string{"Missions"}},
	"player":        {derived: true}, // the Pilot.Player entity in Save.Entities
	"lastStation":   {save: []string{"LastStation", "LastStationCel"}},
	"market":        {save: []string{"Market"}},
	"respawns":      {save: []string{"Respawn"}},
	"trafficLeaves": {save: []string{"TrafficLeaves"}},
	"modTargets":    {save: []string{"ModTargets"}},
	"free":          {save: []string{"FreeSlots"}},

	// Per-Step transients (asserted empty at Step boundaries).
	"queue":                {transient: true},
	"deferLoot":            {transient: true},
	"deferEcon":            {transient: true},
	"deferTravel":          {transient: true},
	"deferMission":         {transient: true},
	"damage":               {transient: true},
	"pendingKills":         {transient: true},
	"pendingOre":           {transient: true},
	"dockedThisTick":       {transient: true},
	"pendingPlayerRespawn": {transient: true},

	// Documented exceptions: persistent, deliberately not in Save.
	"events":         {exempt: "drained by Events(), capped at emit time — observation surface, not sim state"},
	"ammoPrefs":      {exempt: "frozen Save schema: per-slot charge preferences reset on restore (intentional)"},
	"lastPlayerWork": {exempt: "frozen Save schema: the recent-work aggro marker resets on restore (intentional)"},
}

// TestSimSaveCompleteness puts "Hash covers every persistent field" behind
// one check: a field added to Sim without a classification fails, a Save
// field claimed by no Sim field fails (Hash would fingerprint state no one
// accounts for), and a claimed Save field that no longer exists fails.
func TestSimSaveCompleteness(t *testing.T) {
	simRT := reflect.TypeOf(Sim{})
	saveRT := reflect.TypeOf(Save{})

	seen := make(map[string]bool, simRT.NumField())
	for i := 0; i < simRT.NumField(); i++ {
		f := simRT.Field(i)
		seen[f.Name] = true
		class, ok := simFieldClasses[f.Name]
		if !ok {
			t.Errorf("Sim.%s (%s) is unclassified: add it to simFieldClasses as "+
				"save/derived/transient/exempt", f.Name, f.Type)
			continue
		}
		if class.transient && len(class.save) > 0 {
			t.Errorf("Sim.%s is both transient and saved — pick one", f.Name)
		}
		if class.exempt != "" && (class.transient || len(class.save) > 0) {
			t.Errorf("Sim.%s is exempt (%s) but also classified as saved/transient",
				f.Name, class.exempt)
		}
		for _, sf := range class.save {
			if _, ok := saveRT.FieldByName(sf); !ok {
				t.Errorf("Sim.%s claims Save.%s, which does not exist", f.Name, sf)
			}
		}
	}
	for name := range simFieldClasses {
		if !seen[name] {
			t.Errorf("simFieldClasses lists Sim.%s, which no longer exists", name)
		}
	}

	// Reverse coverage: every Save field must be somebody's responsibility.
	claimed := make(map[string]bool)
	for _, class := range simFieldClasses {
		for _, sf := range class.save {
			claimed[sf] = true
		}
	}
	for i := 0; i < saveRT.NumField(); i++ {
		name := saveRT.Field(i).Name
		if !claimed[name] {
			t.Errorf("Save.%s is not claimed by any Sim field: Hash() covers state "+
				"nobody accounts for", name)
		}
	}
}

// assertTransientsEmpty fails when any transient field is non-empty at a
// Step boundary (the other half of the classification).
func assertTransientsEmpty(t *testing.T, s *Sim) {
	t.Helper()
	rv := reflect.ValueOf(s).Elem()
	for name, class := range simFieldClasses {
		if !class.transient {
			continue
		}
		fv := rv.FieldByName(name)
		if !fv.IsValid() {
			t.Errorf("Sim.%s no longer exists", name)
			continue
		}
		if !fv.IsZero() {
			t.Errorf("Sim.%s survives the Step boundary: %v", name, fv.Interface())
		}
	}
}

// TestStepTransientsEmptyAtBoundaries drives a script that exercises every
// transient (deferred command buckets, damage, kill/ore observations, the
// dock stamp, a death respawn) and checks the boundary after each Step.
func TestStepTransientsEmptyAtBoundaries(t *testing.T) {
	s := newTestSim(t)
	station := entityByName(s, "Haven Rest")
	if station.IsZero() {
		t.Fatal("no station")
	}
	var npc ecs.Entity
	for i := 0; i < 120; i++ {
		switch i {
		case 5: // economy + travel buckets, dock stamp
			s.Queue(CmdDock{Station: station})
			s.Queue(CmdBuy{Item: "charge-emp", Qty: 1})
		case 6:
			s.Queue(CmdUndock{})
		case 10: // mission bucket
			s.Queue(CmdAcceptMission{ID: "contract-ghost"})
		case 15: // loot bucket
			w := s.create()
			s.Names.Set(w, Name{Value: "Script Wreck"})
			s.Transforms.Set(w, Transform{Pos: playerPos(s)})
			s.Wrecks.Set(w, Wreck{
				Loot:      []content.CargoStack{{Item: "loot-alloy-scrap", Qty: 1}},
				ExpiresAt: s.time + 300,
			})
			s.Queue(CmdLoot{Wreck: w})
		case 20: // pending damage + pendingKills
			npc = s.spawnNpc("npc-rat-scuttler", playerPos(s).Add(v(4000, 0)), 10)
			damageNPC(s, s.player, npc, content.Damage{EM: 1e6}, false)
		case 30: // pendingOre observation
			s.pendingOre = append(s.pendingOre, pendingOreYield{item: "ore-sterngrit", vol: 10})
		case 40: // pendingPlayerRespawn (death respawn at the end of the Step)
			damageNPC(s, npc, s.player, content.Damage{EM: 1e6}, false)
		}
		s.Step()
		assertTransientsEmpty(t, s)
	}
	if !s.playerDocked() && s.lastStation == "" {
		t.Fatal("script never touched the respawn anchor")
	}
}

// twoStationContent extends the fixture with a second station in sys-a: the
// respawn anchor is only observable when losing it picks a different station
// (an anchor-less respawn falls back to the system's FIRST station, Haven
// Rest).
func twoStationContent() *content.Content {
	c := testContent()
	sysA := c.Systems["sys-a"]
	sysA.Celestials = append(sysA.Celestials, content.CelestialDef{
		ID: "cel-station-b", Name: "Anvil Stop", Kind: content.CelStation,
		Position: core.Vec2{X: 1.2e7, Y: 3e6}, Radius: 500,
		Station: &content.StationDef{Services: []string{"repair", "market", "fitting"}},
	})
	return c
}

// anchorOffGrid docks at the second station, undocks and jumps to sys-b, so
// the respawn anchor points at a station that is not on the current grid.
func anchorOffGrid(t *testing.T, s *Sim) {
	t.Helper()
	anvil := entityByName(s, "Anvil Stop")
	if anvil.IsZero() {
		t.Fatal("no second station")
	}
	teleport(s, s.player, mustPos(s, anvil))
	s.Queue(CmdDock{Station: anvil})
	s.Step()
	drain(s)
	if s.lastStation != "cel-station-b" {
		t.Fatalf("lastStation=%q want cel-station-b after docking", s.lastStation)
	}
	s.Queue(CmdUndock{})
	s.Step()
	drain(s)
	gate := entityByName(s, "Gate to Beta")
	teleport(s, s.player, gatePos(s, gate))
	s.Queue(CmdJump{Gate: gate})
	s.Step()
	drain(s)
	if s.systemID != "sys-b" {
		t.Fatalf("system=%q want sys-b", s.systemID)
	}
}

// jumpHomeAndDie returns through the gate and dies, reporting the respawn
// station line ("Respawned at <station> in a <hull>").
func jumpHomeAndDie(t *testing.T, s *Sim) string {
	t.Helper()
	gate := entityByName(s, "Gate to Alpha")
	if gate.IsZero() {
		t.Fatal("no return gate")
	}
	teleport(s, s.player, gatePos(s, gate))
	s.Queue(CmdJump{Gate: gate})
	s.Step()
	drain(s)
	if s.systemID != "sys-a" {
		t.Fatalf("system=%q want sys-a", s.systemID)
	}
	damageNPC(s, s.player, s.player, content.Damage{EM: 1e6}, false)
	s.Step()
	evs := drain(s)
	ev := findEvent(evs, EvLog, "Respawned at ")
	if ev == nil {
		t.Fatalf("no respawn event: %v", evs)
	}
	return ev.Text
}

// TestLastStationPersistsOffGrid: the anchor is saved even when its station
// is in another system (no handle on the current grid), so a restored player
// dies at the same station a live run would — in a system where losing it
// would silently pick the first station instead.
func TestLastStationPersistsOffGrid(t *testing.T) {
	s, err := New(twoStationContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	anchorOffGrid(t, s)

	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if save.LastStation != 0 {
		t.Fatalf("fixture: handle=%d, want 0 (the anchor is off-grid here)", save.LastStation)
	}
	if save.LastStationCel != "cel-station-b" {
		t.Fatalf("LastStationCel=%q want cel-station-b (anchor dropped at save time)", save.LastStationCel)
	}

	b, err := New(twoStationContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Restore(save); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if b.lastStation != "cel-station-b" {
		t.Fatalf("restored anchor=%q want cel-station-b", b.lastStation)
	}
	if got := b.Hash(); got != s.Hash() {
		t.Fatalf("restored hash=%016x want %016x (anchor round-trip)", got, s.Hash())
	}

	liveText := jumpHomeAndDie(t, s)
	restoredText := jumpHomeAndDie(t, b)
	const want = "Respawned at Anvil Stop in a Sloop"
	if liveText != want {
		t.Fatalf("live respawn = %q, want %q", liveText, want)
	}
	if restoredText != liveText {
		t.Fatalf("restored respawn = %q, live run = %q (anchor not persisted)", restoredText, liveText)
	}
}

// TestRestoreResetsDocumentedPreferences pins the two INTENTIONAL gaps in
// the frozen Save schema: per-slot ammo preferences and the recent-player-
// work marker survive in the live sim but reset on restore. A future change
// to either must fail here first (and update simFieldClasses + the schema
// registry note in docs/ARCHITECTURE.md).
func TestRestoreResetsDocumentedPreferences(t *testing.T) {
	s := newTestSim(t)
	dockAtHaven(t, s)
	slot := SlotRef{Class: content.SlotHigh, Index: 0}
	s.Queue(CmdSetAmmo{Slot: slot, Item: "charge-emp"})
	s.Step()
	drain(s)
	if got := s.ammoPref(s.player, slot); got != "charge-emp" {
		t.Fatalf("ammo pref=%q want charge-emp", got)
	}
	s.recordPlayerWork(v(1e7, 500))
	if s.lastPlayerWork.at == playerWorkNever {
		t.Fatal("fixture: no player work recorded")
	}

	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	b, err := New(testContent(), testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Restore(save); err != nil {
		t.Fatalf("restore: %v", err)
	}

	// The live sim keeps both.
	if got := s.ammoPref(s.player, slot); got != "charge-emp" {
		t.Fatalf("live ammo pref=%q, want it kept", got)
	}
	if s.lastPlayerWork.at == playerWorkNever {
		t.Fatal("live sim lost its player-work marker")
	}
	// The restored sim resets both — intentional, pinned.
	if got := b.ammoPref(b.player, slot); got != "" {
		t.Fatalf("restored ammo pref=%q want \"\" (automatic)", got)
	}
	if b.lastPlayerWork.at != playerWorkNever || b.lastPlayerWork.pos != (core.Vec2{}) {
		t.Fatalf("restored lastPlayerWork={%v %v} want cleared", b.lastPlayerWork.pos, b.lastPlayerWork.at)
	}
}
