package sim

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/entieve/entieve/format"

	"github.com/entieve/entieve/content"
)

// Proficiency (CONTEXT.md): practice gates defs and grants edges. These are
// the RED-first pins for the gate seams, the quality-weighted acts and the
// save shape.

func TestProficiencyLevelCurve(t *testing.T) {
	cases := []struct {
		xp   float64
		want int
	}{
		{0, 0}, {9.9, 0}, {10, 1}, {39.9, 1}, {40, 2}, {90, 3}, {160, 4}, {250, 5}, {1e6, 5},
	}
	for _, tc := range cases {
		if got := proficiencyLevel(tc.xp); got != tc.want {
			t.Errorf("proficiencyLevel(%v)=%d want %d", tc.xp, got, tc.want)
		}
	}
}

// TestFitRefusedWithoutFamilyLevel is the gate pin: a medium autocannon needs
// Autocannon II, the refusal names the requirement, and the module stays in
// the hangar. Before this gate it fitted like any mount-legal module.
func TestFitRefusedWithoutFamilyLevel(t *testing.T) {
	s := realFittingSim(t)
	// The starter gun's line is gated above the pilot's level; the sloop's
	// small turret mount still accepts it, so proficiency is the only rule
	// left to refuse.
	s.Content.Modules["mod-autocannon-s"].Gate = 2
	emptyRack(t, s)

	evs := fitOnce(s, "mod-autocannon-s", content.SlotHigh, 0)
	if ev := warnEvent(evs, "Requires Autocannon II to fit Small Autocannon"); ev == nil {
		t.Fatalf("missing proficiency refusal: %v", evs)
	}
	if got := hangarCount(s, "mod-autocannon-s"); got != 1 {
		t.Fatalf("module left the hangar on a gated fit: %d", got)
	}
	if f, _ := s.Fittings.Get(s.player); len(f.High) != 0 {
		t.Fatalf("gated module was fitted: %v", f.High)
	}
}

// TestMarketSellsWhatThePilotCannotFit: the market is never gated (EVE's
// own-what-you-cannot-run rule). Buying the medium autocannon above works.
func TestMarketSellsWhatThePilotCannotFit(t *testing.T) {
	s := realFittingSim(t)
	if _, ok := s.PlaceOrder(s.dockedStationID(), OrderSell, "mod-autocannon-m", 1, 1000, OwnerSystem); !ok {
		t.Fatal("fixture: could not post the ask")
	}
	s.Queue(CmdBuy{Item: "mod-autocannon-m", Qty: 1})
	s.Step()
	evs := drain(s)
	if hasEvent(evs, EvLog, "Requires Autocannon") {
		t.Fatalf("market purchase was proficiency-gated: %v", evs)
	}
	if got := hangarCount(s, "mod-autocannon-m"); got != 1 {
		t.Fatalf("hangar holds %d medium autocannons, want 1", got)
	}
}

// TestUndockRefusedBelowHullGate pins the hull seam on synthetic content: a
// hull gated above the player's class level cannot leave the station.
func TestUndockRefusedBelowHullGate(t *testing.T) {
	c := testContent()
	c.Ships["ship-vastrek"].Family = "frigate"
	c.Ships["ship-vastrek"].Gate = 1
	s, err := New(c, testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	station := entityByName(s, "Haven Rest")
	teleport(s, s.player, mustPos(s, station))
	s.Queue(CmdDock{Station: station})
	s.Step()
	drain(s)
	if !s.playerDocked() {
		t.Fatal("fixture: not docked")
	}

	s.Queue(CmdUndock{})
	s.Step()
	evs := drain(s)
	if ev := findEvent(evs, EvLog, "Requires Frigate I to fly Sloop"); ev == nil {
		t.Fatalf("missing hull gate refusal: %v", evs)
	}
	if !s.playerDocked() {
		t.Fatal("gated hull left the station")
	}
}

// TestMiningPracticeScalesWithOreRarity runs one real mining cycle on the
// same fixture rock at rarity 1 and rarity 4: the rare cycle pays four times
// the practice. This is the anti-farm curve: basic rock for the early levels,
// rare rock for the later ones.
func TestMiningPracticeScalesWithOreRarity(t *testing.T) {
	mine := func(rarity int) (mining, hull float64) {
		c := testContent()
		c.Items["ore-sterngrit"].Rarity = rarity
		c.Modules["mod-mining-laser"].Family = "mining"
		c.Ships["ship-vastrek"].Family = "frigate"
		s, err := New(c, testWorld(), testSeed)
		if err != nil {
			t.Fatal(err)
		}
		rock := firstAsteroid(t, s)
		rp, _ := s.posOf(rock)
		setPlayerPos(s, rp.Add(v(800, 0)))
		laser := injectModule(t, s, "mod-mining-laser")
		s.Queue(CmdMine{Asteroid: rock, Slot: laser})
		s.Step()
		return s.proficiencyXP("mining"), s.proficiencyXP("frigate")
	}
	basic, basicHull := mine(1)
	rare, rareHull := mine(4)
	if basic != 1 {
		t.Fatalf("basic ore cycle paid %v practice, want 1", basic)
	}
	if rare != 8*basic {
		t.Fatalf("rare ore cycle paid %v practice, want %v (8x basic)", rare, 8*basic)
	}
	if basicHull != basic*hullPracticeShare || rareHull != rare*hullPracticeShare {
		t.Fatalf("hull credit = %v/%v, want 50%% of %v/%v", basicHull, rareHull, basic, rare)
	}
}

// TestProficiencyPracticeMatchesTheActs pins every family card's practice
// list against the sim's own award constants: the rarity ladder comes from
// oreRarity, the hull cut from hullPracticeShare, the warp rate from
// flightXPMetres and the hop cap from flightHopCap, so changing an award
// without the card (or the card without the award) fails here. The machine
// line (rat) is the one family with no player act: the card must say so.
func TestProficiencyPracticeMatchesTheActs(t *testing.T) {
	c := testContent()
	s, err := New(c, testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	// The rarity ladder exactly as oreRarity pays it.
	ore := c.Items["ore-sterngrit"]
	pay := make([]int, 4)
	old := ore.Rarity
	for r := 1; r <= 4; r++ {
		ore.Rarity = r
		pay[r-1] = int(s.oreRarity("ore-sterngrit"))
	}
	ore.Rarity = old
	wantRarity := fmt.Sprintf("rarity 1 pays %d, 2 pays %d, 3 pays %d and 4 pays %d",
		pay[0], pay[1], pay[2], pay[3])
	wantCut := fmt.Sprintf("%d%%", int(hullPracticeShare*100))
	wantWarp := fmt.Sprintf("%d point per %d AU", 1, int(flightXPMetres/format.AU))
	wantHop := fmt.Sprintf("half a point a hop")
	if flightHopCap/flightXPMetres != 0.5 {
		t.Fatalf("fixture: one hop pays %v of a point, want half", flightHopCap/flightXPMetres)
	}

	text := func(family string) string {
		return strings.Join(content.FamilyInfo(family).Practice, " ")
	}
	modules := map[string]string{
		"autocannon": "autocannon",
		"artillery":  "artillery",
		"rocket":     "rocket",
		"mining":     "mining cycle",
		"shield":     "shield booster",
		"armor":      "armor repairer",
		"capacitor":  "cap booster",
		"propulsion": "afterburner",
		"ewar":       "web",
	}
	for family, act := range modules {
		got := text(family)
		if !strings.Contains(got, act) {
			t.Errorf("%s: practice does not name its act %q: %q", family, act, got)
		}
		if !strings.Contains(got, wantCut) {
			t.Errorf("%s: practice misses the hull cut %q", family, wantCut)
		}
	}
	if got := text("mining"); !strings.Contains(got, wantRarity) {
		t.Errorf("mining: practice %q does not match the rarity ladder %q", got, wantRarity)
	}
	if got := text("capacitor"); !strings.Contains(got, "recharger") {
		t.Errorf("capacitor: practice misses the recharger spend: %q", got)
	}
	for _, family := range []string{"frigate", "destroyer", "industrial", "barge"} {
		got := text(family)
		for _, want := range []string{wantCut, "kill", "delivery", wantWarp, wantHop} {
			if !strings.Contains(got, want) {
				t.Errorf("%s: practice %q misses %q", family, got, want)
			}
		}
	}
	// The machine line: no player act, and the card says exactly that.
	if got := text("rat"); !strings.Contains(got, "None") {
		t.Errorf("rat: practice %q should state there is none", got)
	}
	for _, hull := range c.Ships {
		if hull.Family == "rat" && hull.Class != "rat" {
			t.Errorf("rat is a machine line, but player hull %s is in it", hull.ID)
		}
	}
	// Every shipped family is covered by one of the two groups above.
	covered := len(modules) + 4 + 1
	if got := len(content.ProficiencyFamilyIDs()); got != covered {
		t.Errorf("%d families ship, the practice pin covers %d: teach it the new act", got, covered)
	}
}

// TestKillBonusCasteWeight: the flat kill bonus scales with the caste's
// authored bounty (Scuttler < Marauder < Hierarch); combat damage XP itself
// is damage-scaled, not caste-scaled.
func TestKillBonusCasteWeight(t *testing.T) {
	c := testContent()
	for id, bounty := range map[content.NpcID]float64{
		"npc-rat-scuttler": 7500,
		"npc-rat-marauder": 65000,
		"npc-rat-hierarch": 250000,
	} {
		def := *c.NPCs["npc-rat-scuttler"]
		def.ID, def.Name, def.Bounty = id, string(id), bounty
		c.NPCs[id] = &def
	}
	s, err := New(c, testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	w := func(id content.NpcID) float64 {
		return s.casteWeight(s.spawnNpc(id, playerPos(s).Add(v(4000, 0)), 10))
	}
	if sc, ma, hi := w("npc-rat-scuttler"), w("npc-rat-marauder"), w("npc-rat-hierarch"); !(sc < ma && ma < hi) {
		t.Fatalf("caste weights %v < %v < %v not increasing", sc, ma, hi)
	}
}

// TestProficiencyLogsOneLinePerLevel pins the level-up surface.
func TestProficiencyLogsOneLinePerLevel(t *testing.T) {
	s := newTestSim(t)
	s.awardProficiency("mining", 10)
	evs := drain(s)
	if ev := findEvent(evs, EvLog, "Proficiency: Mining I"); ev == nil {
		t.Fatalf("missing level-up line: %v", evs)
	}
	s.awardProficiency("mining", 240) // to the cap: II, III, IV, V
	evs = drain(s)
	for _, level := range []string{"II", "III", "IV", "V"} {
		if ev := findEvent(evs, EvLog, "Proficiency: Mining "+level); ev == nil {
			t.Errorf("missing level-up line %s: %v", level, evs)
		}
	}
}

// TestProficiencySurvivesSaveAndTwinRuns pins the save shape: counters
// round-trip field for field, and two runs that practice the same acts stay
// bit-identical.
func TestProficiencySurvivesSaveAndTwinRuns(t *testing.T) {
	s := newTestSim(t)
	s.awardProficiency("autocannon", 45)
	s.awardProficiency("frigate", 12)
	wantAutocannon, wantFrigate := s.Proficiency("autocannon"), s.Proficiency("frigate")
	if wantAutocannon != 2 || wantFrigate != 1 {
		t.Fatalf("levels %d/%d want 2/1", wantAutocannon, wantFrigate)
	}
	save, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(save.Proficiency) != 2 ||
		save.Proficiency[0].Family != "autocannon" || save.Proficiency[1].Family != "frigate" {
		t.Fatalf("canonical proficiency order = %+v", save.Proficiency)
	}
	b := newTestSim(t)
	if err := b.Restore(save); err != nil {
		t.Fatal(err)
	}
	if b.Proficiency("autocannon") != wantAutocannon || b.Proficiency("frigate") != wantFrigate {
		t.Fatal("proficiency did not round-trip")
	}
	if b.Hash() != s.Hash() {
		t.Fatalf("restored hash=%016x want %016x", b.Hash(), s.Hash())
	}

	a, c := newTestSim(t), newTestSim(t)
	for i := 0; i < 300; i++ {
		if i%30 == 0 {
			a.awardProficiency("autocannon", 3)
			c.awardProficiency("autocannon", 3)
		}
		a.Step()
		c.Step()
	}
	if a.Hash() != c.Hash() {
		t.Fatalf("twin runs diverged: %016x vs %016x", a.Hash(), c.Hash())
	}
}

// TestMiningEdgeAddsItsPercent pins the edge: a level-5 miner cuts 10% more
// per cycle than an untrained one, and the rock is still capped at what is
// left (the multiplier is applied before the remaining-volume clamp).
func TestMiningEdgeAddsItsPercent(t *testing.T) {
	mined := func(level5 bool) float64 {
		c := testContent()
		c.Modules["mod-mining-laser"].Family = "mining"
		s, err := New(c, testWorld(), testSeed)
		if err != nil {
			t.Fatal(err)
		}
		s.Content.Modules["mod-mining-laser"].Mining.VolumePerCycle = 100
		if level5 {
			s.awardProficiency("mining", 250)
		}
		rock := firstAsteroid(t, s)
		ast, _ := s.Asteroids.Get(rock)
		ast.Remaining = 200
		rp, _ := s.posOf(rock)
		setPlayerPos(s, rp.Add(v(800, 0)))
		laser := injectModule(t, s, "mod-mining-laser")
		s.Queue(CmdMine{Asteroid: rock, Slot: laser})
		s.Step()
		if ev := findEvent(drain(s), EvLog, "Mined "); ev != nil {
			return ev.Value
		}
		return 0
	}
	base, trained := mined(false), mined(true)
	if base != 100 {
		t.Fatalf("untrained cycle mined %v, want 100", base)
	}
	if math.Abs(trained-110) > 1e-9 {
		t.Fatalf("level-5 cycle mined %v, want 110", trained)
	}
}

// TestProficiencyFamiliesReadAPI pins the tech-tree contract: every family,
// ascending order, within-level XP progress, prerequisite edges, Meets and
// Starter, with copies a panel may keep.
func TestProficiencyFamiliesReadAPI(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(c, c.Worlds["world-default"], 42)
	if err != nil {
		t.Fatal(err)
	}
	fams := s.ProficiencyFamilies()
	if len(fams) != 14 {
		t.Fatalf("%d families, want 14", len(fams))
	}
	for i := 1; i < len(fams); i++ {
		if fams[i-1].Family >= fams[i].Family {
			t.Fatalf("families not ascending: %q before %q", fams[i-1].Family, fams[i].Family)
		}
	}
	byID := func(id string) ProfFamily {
		for _, f := range fams {
			if f.Family == id {
				return f
			}
		}
		t.Fatalf("family %q missing", id)
		return ProfFamily{}
	}

	auto := byID("autocannon")
	if auto.Name != "Autocannon" || auto.Level != 0 || auto.XP != 0 || auto.XPNext != 10 ||
		auto.MaxLevel != 5 || !auto.Meets || !auto.Starter || len(auto.Prereqs) != 0 {
		t.Fatalf("autocannon = %+v", auto)
	}
	art := byID("artillery")
	if art.Meets || art.Starter || len(art.Prereqs) != 1 ||
		art.Prereqs[0] != (ProfPrereq{Family: "autocannon", Level: 2}) {
		t.Fatalf("artillery = %+v", art)
	}
	if barge := byID("barge"); !barge.Meets || barge.Starter ||
		len(barge.Prereqs) != 1 || barge.Prereqs[0] != (ProfPrereq{Family: "industrial", Level: 0}) {
		t.Fatalf("barge = %+v", barge)
	}

	s.awardProficiency("autocannon", 45)
	fams = s.ProficiencyFamilies()
	auto, art = byID("autocannon"), byID("artillery")
	if auto.Level != 2 || auto.XP != 45 || auto.XPNext != 90 {
		t.Fatalf("autocannon after 45 xp = %+v, want L2 45/90", auto)
	}
	if !art.Meets {
		t.Fatalf("artillery still unmet after autocannon II: %+v", art)
	}

	// The slice and its prerequisite slices are copies.
	art.Prereqs[0].Level = 99
	fams = s.ProficiencyFamilies()
	if again := byID("artillery"); again.Prereqs[0].Level != 2 {
		t.Fatalf("caller mutated the authored graph: %+v", again.Prereqs)
	}

	twin, err := New(c, c.Worlds["world-default"], 42)
	if err != nil {
		t.Fatal(err)
	}
	twin.awardProficiency("autocannon", 45)
	if !reflect.DeepEqual(fams, twin.ProficiencyFamilies()) {
		t.Fatal("twin runs returned different family slices")
	}
}

// TestProficiencyMeetsOwnGate checks Meets against a family's own entry gate
// (nothing shipped is above 0, so the fixture raises one): it flips exactly
// on the level-up that clears it.
func TestProficiencyMeetsOwnGate(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(c, c.Worlds["world-default"], 42)
	if err != nil {
		t.Fatal(err)
	}
	s.Content.Modules["mod-mining-laser"].Gate = 2
	meets := func() bool {
		for _, f := range s.ProficiencyFamilies() {
			if f.Family == "mining" {
				return f.Meets
			}
		}
		return false
	}
	if meets() {
		t.Fatal("mining meets a level-2 entry gate at level 0")
	}
	s.awardProficiency("mining", 10)
	if meets() {
		t.Fatal("mining meets a level-2 entry gate at level 1")
	}
	s.awardProficiency("mining", 30)
	if !meets() {
		t.Fatal("mining does not meet its level-2 entry gate at level 2")
	}
}

// TestFitRefusedByMissingPrerequisite: small artillery is gate 0 but needs
// Autocannon II; the refusal names the prerequisite, and the fit lands once
// that prerequisite is practiced.
func TestFitRefusedByMissingPrerequisite(t *testing.T) {
	s := realFittingSim(t)
	emptyRack(t, s)
	giveHangar(s, "mod-artillery-s")

	evs := fitOnce(s, "mod-artillery-s", content.SlotHigh, 0)
	if ev := warnEvent(evs, "Requires Autocannon II to fit Small Artillery"); ev == nil {
		t.Fatalf("missing prerequisite refusal: %v", evs)
	}
	if got := hangarCount(s, "mod-artillery-s"); got != 1 {
		t.Fatalf("module left the hangar on a refused fit: %d", got)
	}

	s.awardProficiency("autocannon", 40)
	evs = fitOnce(s, "mod-artillery-s", content.SlotHigh, 0)
	if warnEvent(evs, "Requires ") != nil {
		t.Fatalf("fit still refused after Autocannon II: %v", evs)
	}
	if f, _ := s.Fittings.Get(s.player); len(f.High) != 1 || f.High[0] != "mod-artillery-s" {
		t.Fatalf("artillery not fitted: %v", f.High)
	}
}

// TestUndockRefusedByMissingPrerequisite pins the hull-side prerequisite: a
// destroyer-family hull needs Frigate II even at its own gate 0.
func TestUndockRefusedByMissingPrerequisite(t *testing.T) {
	c := testContent()
	c.Ships["ship-vastrek"].Family = "destroyer"
	s, err := New(c, testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	station := entityByName(s, "Haven Rest")
	teleport(s, s.player, mustPos(s, station))
	s.Queue(CmdDock{Station: station})
	s.Step()
	drain(s)
	s.Queue(CmdUndock{})
	s.Step()
	evs := drain(s)
	if ev := findEvent(evs, EvLog, "Requires Frigate II to fly Sloop"); ev == nil {
		t.Fatalf("missing hull prerequisite refusal: %v", evs)
	}
	if !s.playerDocked() {
		t.Fatal("hull with an unmet prerequisite left the station")
	}
}

// TestStarterLoadoutsLegalAtTick0 proves both world starts fit and fly
// without any practice, prerequisites included.
func TestStarterLoadoutsLegalAtTick0(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []content.WorldID{"world-default", "world-showcase"} {
		wd, ok := c.World(id)
		if !ok {
			t.Fatalf("world %s missing", id)
		}
		s, err := New(c, wd, 42)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		hull, _ := c.Ship(wd.Start.Ship)
		if req, ok := s.proficiencyCheck(hull.Family, hull.Gate); !ok {
			t.Fatalf("%s: start hull refused: %s", id, req)
		}
		for _, mid := range wd.Start.Fit {
			def, _ := c.Module(mid)
			if req, ok := s.proficiencyCheck(def.Family, def.Gate); !ok {
				t.Fatalf("%s: start module %s refused: %s", id, mid, req)
			}
		}
	}
}

// TestCombatPracticeScalesWithDamage pins the owner's damage-scaled combat
// XP: twice the applied damage pays twice the weapon practice, and the flown
// hull takes its job share.
func TestCombatPracticeScalesWithDamage(t *testing.T) {
	hit := func(dmg float64) (weapon, hull float64) {
		c := testContent()
		c.Modules["mod-autocannon-s"].Family = "autocannon"
		c.Ships["ship-vastrek"].Family = "frigate"
		s, err := New(c, testWorld(), testSeed)
		if err != nil {
			t.Fatal(err)
		}
		npc := s.spawnNpc("npc-rat-scuttler", playerPos(s).Add(v(4000, 0)), 0)
		s.damage = append(s.damage, pendingDamage{
			src: s.player, dst: npc, dmg: content.Damage{EM: dmg}, family: "autocannon",
		})
		s.systemDamage()
		return s.proficiencyXP("autocannon"), s.proficiencyXP("frigate")
	}
	w1, h1 := hit(50)
	w2, h2 := hit(100)
	if w1 <= 0 {
		t.Fatal("a damage hit paid no weapon practice")
	}
	if math.Abs(w2-2*w1) > 1e-9 {
		t.Fatalf("100 damage paid %v, want 2x the 50-damage %v", w2, w1)
	}
	if math.Abs(h1-w1*hullPracticeShare) > 1e-9 || math.Abs(h2-w2*hullPracticeShare) > 1e-9 {
		t.Fatalf("hull credit %v/%v, want 50%% of %v/%v", h1, h2, w1, w2)
	}
}

// TestFlightHopPractice pins distance-based hull practice: 25 AU pays a
// quarter point, one hop caps at 50 AU (half a point), so ordinary hopping is
// visible but a single warp cannot flood the hull ladder.
func TestFlightHopPractice(t *testing.T) {
	s := newTestSim(t)
	s.Content.Ships["ship-vastrek"].Family = "frigate"
	s.awardFlightHop(s.player, 25*format.AU)
	if got := s.proficiencyXP("frigate"); math.Abs(got-0.25) > 1e-9 {
		t.Fatalf("25 AU paid %v flight practice, want 0.25", got)
	}
	s.awardFlightHop(s.player, 500*format.AU)
	if got := s.proficiencyXP("frigate"); math.Abs(got-0.75) > 1e-9 {
		t.Fatalf("capped hop total %v, want 0.75", got)
	}
}

// TestProficiencyStopsAtMax pins the clamp: level V banks exactly 250, later
// acts add nothing, and a restore clamps a corrupt over-max counter.
func TestProficiencyStopsAtMax(t *testing.T) {
	s := newTestSim(t)
	s.awardProficiency("mining", 1000)
	if got := s.proficiencyXP("mining"); got != 250 {
		t.Fatalf("max counter = %v, want 250", got)
	}
	s.awardProficiency("mining", 100)
	if got := s.proficiencyXP("mining"); got != 250 {
		t.Fatalf("counter grew past max: %v", got)
	}
	fams := s.ProficiencyFamilies()
	for _, f := range fams {
		if f.Family == "mining" {
			if f.Level != 5 || f.XP != 250 || f.XPNext != 0 {
				t.Fatalf("max family read = %+v, want V 250/0", f)
			}
		}
	}
	save, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	for i := range save.Proficiency {
		if save.Proficiency[i].Family == "mining" {
			save.Proficiency[i].XP = 999
		}
	}
	b := newTestSim(t)
	if err := b.Restore(save); err != nil {
		t.Fatal(err)
	}
	if got := b.proficiencyXP("mining"); got != 250 {
		t.Fatalf("restored over-max counter = %v, want 250", got)
	}
}

// TestSupportCyclePractice pins the Round 1 support acts on the cycle seam:
// shield and armor reps, afterburner cycles, cap boosters and ewar cycles
// each train their family (and the hull at the job share).
func TestSupportCyclePractice(t *testing.T) {
	run := func(setup func(c *content.Content)) (xp, hull float64) {
		c := testContent()
		c.Ships["ship-vastrek"].Family = "frigate"
		setup(c)
		s, err := New(c, testWorld(), testSeed)
		if err != nil {
			t.Fatal(err)
		}
		slot := injectModule(t, s, "mod-shield-boost")
		s.Queue(CmdActivate{Slot: slot})
		s.Step()
		return s.proficiencyXP("shield"), s.proficiencyXP("frigate")
	}
	if xp, hull := run(func(c *content.Content) { c.Modules["mod-shield-boost"].Family = "shield" }); xp <= 0 || hull <= 0 {
		t.Fatalf("shield cycle paid %v, hull %v", xp, hull)
	}

	armor := testContent()
	armor.Ships["ship-vastrek"].Family = "frigate"
	armor.Modules["mod-armor-rep"].Family = "armor"
	sa, err := New(armor, testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	sa.Queue(CmdActivate{Slot: injectModule(t, sa, "mod-armor-rep")})
	sa.Step()
	if got := sa.proficiencyXP("armor"); got <= 0 {
		t.Fatalf("armor cycle paid %v", got)
	}

	prop := testContent()
	prop.Ships["ship-vastrek"].Family = "frigate"
	prop.Modules["mod-afterburner"].Family = "propulsion"
	sp, err := New(prop, testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	sp.Queue(CmdActivate{Slot: injectModule(t, sp, "mod-afterburner")})
	sp.Step()
	if got := sp.proficiencyXP("propulsion"); got <= 0 {
		t.Fatalf("afterburner cycle paid %v", got)
	}

	capb := testContent()
	capb.Ships["ship-vastrek"].Family = "frigate"
	capb.Modules["mod-cap-booster"].Family = "capacitor"
	sc, err := New(capb, testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	sc.Queue(CmdActivate{Slot: injectModule(t, sc, "mod-cap-booster")})
	sc.Step()
	stepN(sc, 1200) // the cell load cycle runs before the first booster cycle
	if got := sc.proficiencyXP("capacitor"); got <= 0 {
		t.Fatalf("cap booster cycle paid %v", got)
	}

	// A fitted cap recharger turns any capacitor spend into capacitor
	// practice, even without a booster cycle.
	rech := testContent()
	rech.Ships["ship-vastrek"].Family = "frigate"
	rech.Modules["mod-cap-recharger"].Family = "capacitor"
	rech.Modules["mod-shield-boost"].Family = "shield"
	sr, err := New(rech, testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	injectModule(t, sr, "mod-cap-recharger")
	sr.Queue(CmdActivate{Slot: injectModule(t, sr, "mod-shield-boost")})
	sr.Step()
	if got := sr.proficiencyXP("capacitor"); got <= 0 {
		t.Fatalf("cap recharger spend paid %v", got)
	}

	// Ewar: the cycle award is the same switch; exercise it directly since a
	// web needs a live locked target.
	ew := testContent()
	ew.Ships["ship-vastrek"].Family = "frigate"
	ew.Modules["mod-stasis-web"].Family = "ewar"
	se, err := New(ew, testWorld(), testSeed)
	if err != nil {
		t.Fatal(err)
	}
	se.awardSupportCycle(se.player, ew.Modules["mod-stasis-web"])
	if got := se.proficiencyXP("ewar"); got <= 0 {
		t.Fatalf("ewar cycle paid %v", got)
	}
}
