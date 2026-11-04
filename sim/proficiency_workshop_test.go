package sim

import (
	"math"
	"testing"

	"github.com/entieve/entieve/content"
)

// The two workshop lines (CONTEXT.md: Proficiency, Manufacturing and
// Refining): edges on the player's industry and refinery paths, gate 0, and
// practice from the pilot's own jobs and batches. These are the RED-first
// pins for the edge maths, the level-0 identity, the agent exclusion and the
// awards.

// TestWorkshopFamiliesShipFromTheStart pins the family graph: both lines are
// in the vocabulary, open at level 0 with no prerequisite edge, carry a card
// with an EDGES line, and never gate gear.
func TestWorkshopFamiliesShipFromTheStart(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(c, c.Worlds["world-default"], 42)
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range []string{"manufacturing", "refining"} {
		if !content.ValidProficiencyFamily(family) {
			t.Fatalf("%s is not a proficiency family", family)
		}
		if edges := content.Prerequisites(family); len(edges) != 0 {
			t.Errorf("%s has prerequisite edges %+v, want none (gate 0, owner decision)", family, edges)
		}
		card := content.FamilyInfo(family)
		if card.Desc == "" || len(card.Practice) == 0 || card.Edges == "" {
			t.Errorf("%s card = %+v, want description, practice and an edges line", family, card)
		}
		found := false
		for _, f := range s.ProficiencyFamilies() {
			if f.Family != family {
				continue
			}
			found = true
			if f.Name != content.FamilyLabel(family) || f.Level != 0 || !f.Meets ||
				f.Gate != "" || len(f.Prereqs) != 0 {
				t.Errorf("%s as the tree reads it = %+v, want open at level 0", family, f)
			}
		}
		if !found {
			t.Errorf("%s missing from the proficiency read API", family)
		}
		// No def is gated on the workshop lines, and neither line needs a
		// prerequisite to meet its own gate.
		if req, ok := s.proficiencyCheck(family, 0); !ok {
			t.Errorf("%s gate 0 refused: %q", family, req)
		}
	}
}

// TestManufacturingMatsEdgeIsAdditive pins the bill: the Manufacturing
// reduction is 2% per level, capped at 10% at V, and it COMPOSES ADDITIVELY
// with the station's craft discount. Level 0 is an exact no-op (the factor
// is 1.0 and the bill is the craft-only one the game shipped before).
func TestManufacturingMatsEdgeIsAdditive(t *testing.T) {
	s := newIndustrySim(t)
	st := StationID("cel-station-a")
	const rec = content.RecipeID("rec-ammo-emp-s")

	// Level 0: byte-identical to the craft-only bill (ceil(5.5×2) = 11,
	// ceil(3×2) = 6 at L1).
	if got := craftMatFactor(1) - s.mfgMatEdge(); got != 1 {
		t.Fatalf("level 0 factor = %v, want exactly 1", got)
	}
	if got := s.JobMaterials(st, rec, 2); len(got) != 2 || got[0].Qty != 11 || got[1].Qty != 6 {
		t.Fatalf("level 0 bill = %+v, want the craft-only 11 ferrite / 6 nickel", got)
	}
	// A ten-run bill shows the percent: 55 ferrite at L1.
	if got := s.JobMaterials(st, rec, 10)[0].Qty; got != 55 {
		t.Fatalf("level 0 ten-run ferrite = %d, want 55", got)
	}

	// Manufacturing II (-4%): ceil(55 × 0.96) = ceil(52.8) = 53.
	s.awardProficiency("manufacturing", 40)
	if got := s.Proficiency("manufacturing"); got != 2 {
		t.Fatalf("fixture: Manufacturing level = %d, want 2", got)
	}
	if got := s.JobMaterials(st, rec, 10)[0].Qty; got != 53 {
		t.Fatalf("Manufacturing II ferrite = %d, want 53 (ceil(55 × 0.96))", got)
	}

	// Manufacturing V (-10%): ceil(55 × 0.90) = ceil(49.5) = 50.
	s.awardProficiency("manufacturing", 210)
	if got := s.Proficiency("manufacturing"); got != 5 {
		t.Fatalf("fixture: Manufacturing level = %d, want 5", got)
	}
	if got := s.JobMaterials(st, rec, 10)[0].Qty; got != 50 {
		t.Fatalf("Manufacturing V ferrite = %d, want 50 (ceil(55 × 0.90))", got)
	}

	// Craft V plus Manufacturing V composes ADDITIVELY: 1 - 0.08 - 0.10 =
	// 0.82. (The spec's -20% figure is not reachable: the craft ladder's
	// top step is -8%, so the two maxima sum to -18%.)
	s.craftComplete(st, content.FamilyCharges, 100)
	if got := s.CraftLevel(st, content.FamilyCharges); got != 5 {
		t.Fatalf("fixture: craft level = %d, want 5", got)
	}
	if got := craftMatFactor(5) - s.mfgMatEdge(); math.Abs(got-0.82) > 1e-12 {
		t.Fatalf("craft V + Manufacturing V factor = %v, want 0.82", got)
	}
	// ceil(55 × 0.82) = ceil(45.1) = 46, not the multiplicative 0.828.
	if got := s.JobMaterials(st, rec, 10)[0].Qty; got != 46 {
		t.Fatalf("craft V + Manufacturing V ferrite = %d, want 46 (ceil(55 × 0.82))", got)
	}
}

// TestManufacturingOutputEdgeOnlyOnBatchRecipes pins the output edge: it
// applies only where the recipe makes more than one unit per run, floors its
// yield, and leaves a one-unit recipe exactly one unit at every level.
func TestManufacturingOutputEdgeOnlyOnBatchRecipes(t *testing.T) {
	s := newIndustrySim(t)
	const rec = content.RecipeID("rec-ammo-emp-s")

	if got := s.PlayerJobOutput(rec, 1); got != 100 {
		t.Fatalf("level 0 output = %d, want the recipe's 100", got)
	}
	s.awardProficiency("manufacturing", 40) // II: +4%
	if got := s.PlayerJobOutput(rec, 1); got != 104 {
		t.Fatalf("Manufacturing II output = %d, want 104", got)
	}
	if got := s.PlayerJobOutput(rec, 3); got != 312 {
		t.Fatalf("Manufacturing II three-run output = %d, want 312", got)
	}
	s.awardProficiency("manufacturing", 210) // V: +10%
	if got := s.PlayerJobOutput(rec, 1); got != 110 {
		t.Fatalf("Manufacturing V output = %d, want 110", got)
	}

	// A hull or module job is never edged: a hull job yields one hull.
	for _, qtyOne := range []content.RecipeID{"rec-mod-autocannon-s", "rec-ship-vastrek"} {
		if got := s.PlayerJobOutput(qtyOne, 3); got != 3 {
			t.Errorf("%s at Manufacturing V delivered %d units, want the recipe's 3", qtyOne, got)
		}
	}

	// The floor is the refinery's rule, not rounding: a three-unit batch at
	// +2% is 3.06 and pays 3.
	batch := *s.Content.Recipes["rec-mod-autocannon-s"]
	batch.ID, batch.Qty = "rec-batch-three", 3
	s.Content.Recipes[batch.ID] = &batch
	s.awardProficiency("manufacturing", 250)
	s.prof = nil
	s.awardProficiency("manufacturing", 10) // I: +2%
	if got := s.PlayerJobOutput("rec-batch-three", 1); got != 3 {
		t.Fatalf("floor(3 × 1.02) = %d, want 3", got)
	}
}

// TestRefiningYieldEdge pins the batch conversion: level 0 is the authored
// floor, V adds 10% to every mineral, and the ore eaten does not move.
func TestRefiningYieldEdge(t *testing.T) {
	s := newIndustrySim(t)
	def, ok := s.Content.Item("ore-sterngrit")
	if !ok || def == nil {
		t.Fatal("fixture: no sterngrit")
	}
	// The fixture's ore yields 0.5 ferrite and 0.25 nickel per unit, so a
	// batch is exactly 50 and 25.
	lvl0 := s.RefineOutput(def, RefineBatch)
	if len(lvl0) != 2 || lvl0[0].Qty != 50 || lvl0[1].Qty != 25 {
		t.Fatalf("level 0 batch = %+v, want 50 ferrite / 25 nickel", lvl0)
	}
	s.awardProficiency("refining", 250) // V: +10%
	lvl5 := s.RefineOutput(def, RefineBatch)
	if len(lvl5) != 2 || lvl5[0].Qty != 55 || lvl5[1].Qty != 27 {
		t.Fatalf("Refining V batch = %+v, want 55 ferrite / 27 nickel (floor 27.5)", lvl5)
	}
	if got := s.RefineOutput(def, 0); got != nil {
		t.Fatalf("zero-quantity batch = %+v, want nil", got)
	}
}

// TestWorkshopPracticeAwards pins the two acts: a completed player job pays
// the product's rarity, a completed batch pays the ore's rarity, and neither
// the level-up line nor the hull cut is invented for them.
func TestWorkshopPracticeAwards(t *testing.T) {
	s := newIndustrySim(t)
	dockAtHaven(t, s)
	giveCargo(s, "min-ferrite", 1000)
	giveCargo(s, "min-nickel", 1000)

	id, ok := s.StartJob("cel-station-a", "rec-ammo-emp-s", 1, OwnerPlayer)
	if !ok {
		t.Fatal("player job refused")
	}
	stepUntilDone(t, s, "cel-station-a", id)
	product := s.Content.Recipes["rec-ammo-emp-s"].Product
	if got, want := s.proficiencyXP("manufacturing"), s.oreRarity(product); got != want {
		t.Fatalf("job prize = %v, want the product's rarity %v", got, want)
	}
	if got := s.proficiencyXP("frigate"); got != 0 {
		t.Fatalf("the hull took a cut of a job act: %v", got)
	}

	evs := drain(s)
	if hasEvent(evs, EvLog, "Proficiency: Manufacturing") {
		t.Fatal("one job leveled the line (rarity 1 pays 1)")
	}
	if !s.CollectJob("cel-station-a", id, OwnerPlayer) {
		t.Fatal("collect refused")
	}

	// Nine more jobs take it to I, and the level-up line is the house idiom.
	for i := 0; i < 9; i++ {
		id, ok := s.StartJob("cel-station-a", "rec-ammo-emp-s", 1, OwnerPlayer)
		if !ok {
			t.Fatalf("job %d refused", i)
		}
		stepUntilDone(t, s, "cel-station-a", id)
		if !s.CollectJob("cel-station-a", id, OwnerPlayer) {
			t.Fatalf("job %d collect refused", i)
		}
	}
	if got := s.Proficiency("manufacturing"); got != 1 {
		t.Fatalf("ten one-point jobs = level %d, want 1", got)
	}
	if !hasEvent(drain(s), EvLog, "Proficiency: Manufacturing I") {
		t.Fatal("missing the Manufacturing I level-up line")
	}

	// The batch: the ore's rarity, once per batch, the ore eaten unchanged.
	giveCargo(s, "ore-sterngrit", 2*RefineBatch)
	before := s.proficiencyXP("refining")
	s.Queue(CmdRefine{Item: "ore-sterngrit", Qty: RefineBatch})
	s.Step()
	drain(s)
	if got, want := s.proficiencyXP("refining")-before, s.oreRarity("ore-sterngrit"); got != want {
		t.Fatalf("batch prize = %v, want the ore's rarity %v", got, want)
	}
}

// TestAgentPathsIgnoreThePilotEdges is the NPC-economy pin: with the pilot
// at Manufacturing V, a ledger agent's bill, delivery and practice are
// exactly the untrained ones (agent behaviour is hashed).
func TestAgentPathsIgnoreThePilotEdges(t *testing.T) {
	trained := newIndustrySim(t)
	home := StationID("cel-station-a")
	untrained := newIndustrySim(t)
	const rec = content.RecipeID("rec-ammo-emp-s")

	for _, s := range []*Sim{trained, untrained} {
		addTestAgent(s, "ag-job", RoleIndustrialist, home, 500_000)
		a := agentOf(s, "ag-job")
		a.Inventory["min-ferrite"] = 1000
		a.Inventory["min-nickel"] = 1000
	}
	trained.awardProficiency("manufacturing", 250)
	trained.awardProficiency("refining", 250)

	tag := agentOwnerTag("ag-job")
	gotBill := trained.jobBill(home, rec, 10, tag)
	wantBill := untrained.JobMaterials(home, rec, 10)
	if len(gotBill) != len(wantBill) {
		t.Fatalf("agent bill = %+v, want the untrained %+v", gotBill, wantBill)
	}
	for i := range wantBill {
		if gotBill[i] != wantBill[i] {
			t.Fatalf("agent bill[%d] = %+v, want the untrained %+v", i, gotBill[i], wantBill[i])
		}
	}
	// The pilot's own bill did move: the pin above is about the agent, not
	// about a dead edge.
	if trained.JobMaterials(home, rec, 10)[0].Qty == wantBill[0].Qty {
		t.Fatal("the pilot's own bill did not take the Manufacturing edge")
	}

	// An agent job pays the agent's inventory the authored qty and pays the
	// pilot no practice.
	a := agentOf(trained, "ag-job")
	id, ok := trained.StartJob(home, rec, 2, tag)
	if !ok {
		t.Fatal("agent job refused")
	}
	before := a.Inventory["ammo-emp-s"]
	job := stepUntilDone(t, trained, home, id)
	if !trained.CollectJob(home, id, tag) {
		t.Fatalf("agent collect refused on job %d", job.ID)
	}
	// 2 runs × 100, un-edged even though the pilot is at V (+10%).
	if got := a.Inventory["ammo-emp-s"] - before; got != 200 {
		t.Fatalf("agent delivery = %d, want the authored 200", got)
	}
	if got := trained.proficiencyXP("manufacturing"); got != 250 {
		t.Fatalf("agent job paid the pilot %v practice, want none past the cap's 250", got)
	}
}

// TestWorkshopEdgeOnlyTouchesThePlayerHangar pins the delivery side at level
// 0: the player's job delivers exactly the authored qty × runs, so the
// capture-era figure is unchanged.
func TestWorkshopEdgeOnlyTouchesThePlayerHangar(t *testing.T) {
	s := newIndustrySim(t)
	dockAtHaven(t, s)
	giveCargo(s, "min-ferrite", 1000)
	giveCargo(s, "min-nickel", 1000)
	id, ok := s.StartJob("cel-station-a", "rec-ammo-emp-s", 2, OwnerPlayer)
	if !ok {
		t.Fatal("job refused")
	}
	stepUntilDone(t, s, "cel-station-a", id)
	before := hangarCount(s, "ammo-emp-s")
	if !s.CollectJob("cel-station-a", id, OwnerPlayer) {
		t.Fatal("collect refused")
	}
	if got := hangarCount(s, "ammo-emp-s") - before; got != 200 {
		t.Fatalf("level-0 delivery = %d, want the authored 200", got)
	}
}
