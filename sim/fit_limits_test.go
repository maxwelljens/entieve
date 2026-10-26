package sim

import (
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
)

// ---------------------------------------------------------------------------
// Fitting limits: high-slot mounts (kind + size) and grid/cpu class quotas.
// Before the limits existed a large autocannon fitted a frigate and slot
// count was the only bound; these are the RED-first pins for the rejection.
// ---------------------------------------------------------------------------

// realFittingSim loads the shipped content and docks the default world's
// frigate at Jalvrek (the fitting service).
func realFittingSim(t *testing.T) *Sim {
	t.Helper()
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("load content: %v", err)
	}
	s, err := New(c, c.Worlds["world-default"], 1)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	st := entityByName(s, "Jalvrek")
	if st.IsZero() {
		t.Fatal("no Jalvrek station")
	}
	if tr, ok := s.Transforms.Get(st); ok {
		setPlayerPos(s, tr.Pos)
	}
	dockAtStation(t, s, "Jalvrek")
	return s
}

// giveHangar puts one of each module into the station hangar.
func giveHangar(s *Sim, ids ...content.ModuleID) {
	for _, id := range ids {
		s.Hangar.Items = addStacks(s.Hangar.Items, content.ItemID(id), 1)
	}
}

// emptyRack unfits every module of the player's ship.
func emptyRack(t *testing.T, s *Sim) {
	t.Helper()
	for _, class := range slotClasses {
		for {
			f, _ := s.Fittings.Get(s.player)
			if len(f.Defs(class)) == 0 {
				break
			}
			s.Queue(CmdUnfitModule{Slot: SlotRef{Class: class, Index: 0}})
			s.Step()
			drain(s)
		}
	}
}

// fitOnce queues one CmdFitModule and returns the tick's events.
func fitOnce(s *Sim, id content.ModuleID, class string, index int) []Event {
	s.Queue(CmdFitModule{Item: id, Slot: SlotRef{Class: class, Index: index}})
	s.Step()
	return drain(s)
}

// TestFitRejectsOversizeMount: a large autocannon needs a large turret mount.
// RED evidence: before the limits it logged "Large Autocannon fitted to high
// slot 1" on the frigate.
func TestFitRejectsOversizeMount(t *testing.T) {
	s := realFittingSim(t)
	emptyRack(t, s)
	giveHangar(s, "mod-autocannon-l")

	evs := fitOnce(s, "mod-autocannon-l", content.SlotHigh, 0)

	if ev := warnEvent(evs, "Large Autocannon is oversize for the hull's turret mounts"); ev == nil {
		t.Fatalf("missing oversize warning: %v", evs)
	}
	if f, _ := s.Fittings.Get(s.player); len(f.High) != 0 {
		t.Fatalf("oversize module was fitted: %v", f.High)
	}
	if got := hangarCount(s, "mod-autocannon-l"); got != 1 {
		t.Fatalf("module left the hangar on a refused fit: %d", got)
	}
}

// TestFitRejectsMissingMountKind: the frigate carries one launcher mount, so
// a second rocket pod has nothing to mount on.
func TestFitRejectsMissingMountKind(t *testing.T) {
	s := realFittingSim(t)
	giveHangar(s, "mod-rocket-pod", "mod-rocket-pod")

	if evs := fitOnce(s, "mod-rocket-pod", content.SlotHigh, 1); warnEvent(evs, "mount for Rocket Pod") != nil {
		t.Fatalf("first rocket pod refused: %v", evs)
	}
	evs := fitOnce(s, "mod-rocket-pod", content.SlotHigh, 2)
	if ev := warnEvent(evs, "No free launcher mount for Rocket Pod"); ev == nil {
		t.Fatalf("missing launcher-mount warning: %v", evs)
	}
	if f, _ := s.Fittings.Get(s.player); len(f.High) != 2 {
		t.Fatalf("refused module changed the rack: %v", f.High)
	}
}

// TestFitRejectsClassQuotaThenUnfitFrees: the frigate's grid allows five
// Class I modules and three Class II; a sixth Class I module is refused even
// with a free mid slot, and unfitting one frees the quota again.
func TestFitRejectsClassQuotaThenUnfitFrees(t *testing.T) {
	s := realFittingSim(t)
	giveHangar(s, "mod-mining-laser", "mod-rocket-pod", "mod-cap-recharger",
		"mod-stasis-web", "mod-cap-booster")
	// Start rack: autocannon-s (grid I) + afterburner, armor-rep (grid II).
	for _, step := range []struct {
		id    content.ModuleID
		class string
		index int
	}{
		{"mod-mining-laser", content.SlotHigh, 1}, // grid I: 2
		{"mod-rocket-pod", content.SlotHigh, 2},   // grid I: 3, high rack full
		{"mod-cap-recharger", content.SlotLow, 1}, // grid I: 4, low rack full
		{"mod-stasis-web", content.SlotMid, 1},    // grid I: 5, cpu II: 1
	} {
		if evs := fitOnce(s, step.id, step.class, step.index); warnEvent(evs, "quota") != nil {
			t.Fatalf("%s refused before the quota was full: %v", step.id, evs)
		}
	}

	evs := fitOnce(s, "mod-cap-booster", content.SlotMid, 2)
	if ev := warnEvent(evs, "Cap Booster exceeds the hull's grid Class I quota"); ev == nil {
		t.Fatalf("missing quota warning: %v", evs)
	}
	if f, _ := s.Fittings.Get(s.player); len(f.Mid) != 2 {
		t.Fatalf("refused module changed the rack: %v", f.Mid)
	}

	// Unfitting the web frees one grid Class I unit; the booster then fits.
	unfit := SlotRef{Class: content.SlotMid, Index: 1}
	s.Queue(CmdUnfitModule{Slot: unfit})
	s.Step()
	drain(s)
	if evs := fitOnce(s, "mod-cap-booster", content.SlotMid, 1); warnEvent(evs, "quota") != nil {
		t.Fatalf("quota not freed by unfit: %v", evs)
	}
}

// TestAuthoredFitsBuildOnTheirHulls: every NPC fit and every world start fit
// must build through the sim (the load/assemble path), not just validate.
func TestAuthoredFitsBuildOnTheirHulls(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("load content: %v", err)
	}
	s, err := New(c, c.Worlds["world-default"], 1)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for id, npc := range c.NPCs {
		hull := c.Ships[npc.Ship]
		if _, err := s.buildFitting(npc.Fit, hull); err != nil {
			t.Errorf("npcs/%s/fit: %v", id, err)
		}
	}
	for id, w := range c.Worlds {
		hull := c.Ships[w.Start.Ship]
		if _, err := s.buildFitting(w.Start.Fit, hull); err != nil {
			t.Errorf("worlds/%s/start.fit: %v", id, err)
		}
	}
}

// TestBuildFittingRejectsIllegalFit pins the assemble/load path: a stored fit
// that the new hull cannot carry is refused with the rule, not silently fitted.
func TestBuildFittingRejectsIllegalFit(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("load content: %v", err)
	}
	s, err := New(c, c.Worlds["world-default"], 1)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	sloop := c.Ships["ship-sloop"]
	if _, err := s.buildFitting([]content.ModuleID{"mod-autocannon-l"}, sloop); err == nil ||
		!strings.Contains(err.Error(), "oversize") {
		t.Errorf("large autocannon on the frigate: err=%v", err)
	}
	cutlass := c.Ships["ship-cutlass"]
	heavy := []content.ModuleID{"mod-armor-rep", "mod-armor-rep", "mod-armor-rep", "mod-armor-rep"}
	if _, err := s.buildFitting(heavy, cutlass); err == nil ||
		!strings.Contains(err.Error(), "Class II quota") {
		t.Errorf("four Class II grid modules on the destroyer: err=%v", err)
	}
}
