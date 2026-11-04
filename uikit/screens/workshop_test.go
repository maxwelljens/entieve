package screens

import (
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
)

// The workshop lines' interface pins (CONTEXT.md: Proficiency card and the
// manufacturing RESULT): the numbers the player reads are the sim's own
// edged figures, at level 0 exactly the shipped ones.

// trainWorkshop sets the player's level in a family through the save the way
// a real run does (the sim's award path is unexported to this package).
func trainWorkshop(t *testing.T, s *sim.Sim, family string, xp float64) {
	t.Helper()
	save, err := s.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	save.Proficiency = append(save.Proficiency, sim.SavedProficiency{Family: family, XP: xp})
	if err := s.Restore(save); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := s.Proficiency(family); got == 0 {
		t.Fatalf("fixture: %s is still level 0 at %v xp", family, xp)
	}
}

// resultFigure is the RESULT section's row for a recipe as the inspector
// builds it: outputsOf feeds the row, so this reads the same figure the
// player does.
func resultFigure(t *testing.T, f *Manufacturing, env *testEnv, rec content.RecipeID) string {
	t.Helper()
	def, ok := env.Content.Recipes[rec]
	if !ok {
		t.Fatalf("fixture: no recipe %q", rec)
	}
	outs := outputsOf(f.cv, def)
	if len(outs) != 1 {
		t.Fatalf("%s: %d outputs, want 1", rec, len(outs))
	}
	return lastSeg(f.resRowDef(outs[0], theme.DefaultPalette())).Text
}

// TestManufacturingResultShowsTheEffectiveOutput pins the RESULT figure: the
// content's 100 at level 0 and the sim's edged 104 at Manufacturing II, so
// the inspector never promises a batch the job would not deliver.
func TestManufacturingResultShowsTheEffectiveOutput(t *testing.T) {
	f, env, ui, _, frame := boardEnv(t)
	ammo := recipeIDFor(env, "ammo-art-ap-s")
	clickRow(ui, f.recipes, recipeRowFor(f, "ammo-art-ap-s"))
	frame()
	if got := resultFigure(t, f, env, ammo); got != "×100" {
		t.Fatalf("level 0 RESULT = %q, want ×100", got)
	}

	trainWorkshop(t, env.Sim, "manufacturing", 40) // II: +4%
	frame()
	if len(f.resRows) != 1 {
		t.Fatalf("after the level-up: %d result rows, want 1", len(f.resRows))
	}
	if got := resultFigure(t, f, env, ammo); got != "×104" {
		t.Fatalf("Manufacturing II RESULT = %q, want ×104", got)
	}

	// A hull job keeps its one hull: the edge never lands on a qty-1 recipe.
	hull := recipeIDFor(env, "ship-husnek")
	clickRow(ui, f.recipes, recipeRowFor(f, "ship-husnek"))
	frame()
	if got := resultFigure(t, f, env, hull); got != "×1" {
		t.Fatalf("Manufacturing II hull RESULT = %q, want ×1", got)
	}
}

// TestProficiencyCardEdgesLine pins the card's EDGES band: the workshop
// families carry the authored line, every other family hides the band (its
// page is exactly the page it had before the row existed).
func TestProficiencyCardEdgesLine(t *testing.T) {
	k, _ := newTestKit(t)
	c := NewProfInfoWin(k.Env())
	for _, family := range []string{"manufacturing", "refining"} {
		c.Show(family, 0, "")
		if got := c.Edges(); got != content.FamilyInfo(family).Edges {
			t.Errorf("%s edges line = %q, want the authored %q", family, got, content.FamilyInfo(family).Edges)
		}
		if got := c.edges.GetWidget().GetVisibility(); got != widget.Visibility_Show {
			t.Errorf("%s: the EDGES band is hidden", family)
		}
	}
	c.Show("mining", 5, "")
	if got := c.Edges(); got != "" {
		t.Errorf("mining edges line = %q, want none", got)
	}
	if got := c.edges.GetWidget().GetVisibility(); got != widget.Visibility_Hide {
		t.Error("mining keeps a visible (or spaced) EDGES band")
	}
	// A hidden band takes no space: showing it again (even empty) grows the
	// page the frame hugs.
	_, hidden := c.page.PreferredSize()
	c.showEdges(true)
	_, shown := c.page.PreferredSize()
	if shown <= hidden {
		t.Errorf("hidden EDGES band took space: page %d px hidden, %d px shown", hidden, shown)
	}
}

// TestRefineryPreviewShowsTheEffectiveYield pins the batch preview: at
// Refining V the window's own read promises the sim's conversion, not the
// authored fraction.
func TestRefineryPreviewShowsTheEffectiveYield(t *testing.T) {
	env, _ := newChromeEnv(t)
	r := NewRefinery(env)
	giveItem(env.Sim, "ore-cordite", sim.RefineBatch)
	def, _ := env.Content.Item("ore-cordite")

	before := refineOutputs(env, def, sim.RefineBatch)
	trainWorkshop(t, env.Sim, "refining", 250) // V: +10%
	after := refineOutputs(env, def, sim.RefineBatch)
	if len(after) != len(before) {
		t.Fatalf("mineral count moved: %+v → %+v", before, after)
	}
	moved := false
	for i := range after {
		if after[i].Qty < before[i].Qty {
			t.Fatalf("%s yielded less at V: %d → %d", after[i].Item, before[i].Qty, after[i].Qty)
		}
		if after[i].Qty > before[i].Qty {
			moved = true
		}
	}
	if !moved {
		t.Fatalf("Refining V did not move the preview: %+v", before)
	}
	// The window's own batch read is the same conversion, and the sim's
	// command agrees with it.
	r.Sync()
	batch := r.batchOf("ore-cordite")
	if len(batch) != len(after) {
		t.Fatalf("batch strip = %+v, want %+v", batch, after)
	}
	for i := range after {
		if batch[i].Qty != after[i].Qty {
			t.Fatalf("batch strip[%d] = %d, want %d", i, batch[i].Qty, after[i].Qty)
		}
	}
}

// TestProficiencyOverviewHoldsEveryFamily pins the summary's capacity: the
// two column grid must hold every shipped family, so the SKILLS list never
// truncates (the overview drops the tail beyond cols × rows silently).
func TestProficiencyOverviewHoldsEveryFamily(t *testing.T) {
	if got, want := len(content.ProficiencyFamilyIDs()), profOvCols*profOvRows; got > want {
		t.Fatalf("%d families, the overview holds %d: raise profOvRows and profOvH", got, want)
	}
	if got, want := proficiencyTotals(realProfModel(t)).Families, len(content.ProficiencyFamilyIDs()); got != want {
		t.Fatalf("overview totals %d families, want %d", got, want)
	}
}
