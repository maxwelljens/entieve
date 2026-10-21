package screens

import (
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/internal/uitest"
)

// TestRefineryWindow: the ore list, the whole-batch yield preview (the sim's
// own floor(RefineYield × qty) math), the 100-ore stepper ladder, the typed
// CmdRefine enqueue end-to-end, and the hangar destination (a full ship hold
// never blocks a refine).
func TestRefineryWindow(t *testing.T) {
	env, cmds := newChromeEnv(t)
	r := NewRefinery(env)
	ui := newWindowUI(t, env, r.Win())
	frame := func() { uitest.PumpWindow(t, ui, r.Sync, 1024, 700) }
	corditeRow := func() int {
		for i, tr := range r.ores.Rows {
			if d, ok := tr.Data.(refineryRow); ok && d.ore == "ore-cordite" {
				return i
			}
		}
		t.Fatalf("no cordite row in %+v", r.ores.Rows)
		return -1
	}

	const home = sim.StationID("cel-haven-rest") // shipped: refine service
	giveItem(env.Sim, "ore-cordite", 40)
	frame()

	// Undocked: the ore is listed, Refine is visible but grayed.
	row := corditeRow()
	if got := r.ores.Rows[row].Cells[1]; got != "100" {
		t.Errorf("held cell = %q, want 100 (60 shipped + 40 added)", got)
	}
	// The preview is one batch: floor(0.35×100)=35 ferrite, floor(0.3×100)=30 nickel.
	if got := r.ores.Rows[row].Cells[2]; got != "35 Ferrite · 30 Nickel" {
		t.Errorf("yield cell = %q, want 35 Ferrite · 30 Nickel", got)
	}
	if !r.refine.GetWidget().Disabled {
		t.Fatal("Refine enabled while undocked")
	}
	r.refineClicked()
	if len(*cmds) != 0 {
		t.Fatalf("undocked refine enqueued %d commands", len(*cmds))
	}

	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: home})
	clickRow(ui, r.ores, row)
	frame()
	if !r.hasSel || r.sel.ore != "ore-cordite" {
		t.Fatalf("selection = %+v (hasSel=%v), want cordite", r.sel, r.hasSel)
	}
	if r.refine.GetWidget().Disabled {
		t.Fatal("Refine grayed at a refine station with a whole batch")
	}
	if got := r.stepper.Label().Label; got != "Qty 100" {
		t.Errorf("stepper = %q, want Qty 100", got)
	}
	if !strings.Contains(r.status.Label, "Refine 100 × Cordite Ore → 35 Ferrite · 30 Nickel → hangar") {
		t.Errorf("status = %q, want the batch yield preview with its destination", r.status.Label)
	}

	// The stepper offers whole batches only, capped at the stock: 100 held
	// stays 100.
	uitest.FireButton(ui, r.stepper.Plus())
	frame()
	if got := r.stepper.Label().Label; got != "Qty 100" {
		t.Errorf("stepper after + at 100 held = %q, want Qty 100", got)
	}

	// 250 held: the cap is 200 (two batches), never a partial 250.
	giveItem(env.Sim, "ore-cordite", 150)
	frame()
	uitest.FireButton(ui, r.stepper.Plus())
	frame()
	if got := r.stepper.Label().Label; got != "Qty 200" {
		t.Errorf("stepper after + at 250 held = %q, want Qty 200", got)
	}
	uitest.FireButton(ui, r.stepper.Plus())
	frame()
	if got := r.stepper.Label().Label; got != "Qty 200" {
		t.Errorf("stepper over the cap = %q, want Qty 200", got)
	}
	if !strings.Contains(r.status.Label, "Refine 200 × Cordite Ore → 70 Ferrite · 60 Nickel") {
		t.Errorf("status = %q, want the 200-ore preview", r.status.Label)
	}

	// The button enqueues the CmdRefine the sim executes.
	uitest.FireButton(ui, r.refine)
	if len(*cmds) != 1 {
		t.Fatalf("refine enqueued %d commands, want 1", len(*cmds))
	}
	cmd, ok := (*cmds)[0].(sim.CmdRefine)
	if !ok || cmd.Item != "ore-cordite" || cmd.Qty != 200 {
		t.Fatalf("cmd = %#v, want CmdRefine{ore-cordite 200}", (*cmds)[0])
	}
	env.Sim.Queue(cmd)
	env.Sim.Step()
	if got := giveItemCount(env, "ore-cordite"); got != 50 {
		t.Errorf("ore left = %d, want 50", got)
	}
	if got := hangarItemCount(env, "min-ferrite"); got != 70 {
		t.Errorf("ferrite in the hangar = %d, want 70", got)
	}
	if got := hangarItemCount(env, "min-nickel"); got != 60 {
		t.Errorf("nickel in the hangar = %d, want 60", got)
	}
	if got := giveItemCount(env, "min-ferrite") + giveItemCount(env, "min-nickel"); got != 0 {
		t.Errorf("minerals landed in the ship's cargo (%d units)", got)
	}

	// 50 left is under one batch: no valid quantity, Refine grayed and the
	// status says what a batch needs.
	frame()
	if !r.refine.GetWidget().Disabled {
		t.Fatal("Refine enabled with less than one batch")
	}
	if got := r.stepper.Label().Label; got != "Qty 0" {
		t.Errorf("stepper = %q, want Qty 0 below one batch", got)
	}
	if !strings.Contains(r.status.Label, "One batch needs 100 ore — you hold 50 Cordite Ore.") {
		t.Errorf("status = %q, want the one-batch message", r.status.Label)
	}

	// A packed-to-capacity hold never blocks a refine: the minerals always
	// land in the station hangar, so the window makes no cargo-full prediction.
	giveItem(env.Sim, "ore-cordite", 150)
	frame()
	cargo, _ := env.Sim.Cargos.Get(env.Sim.Player())
	used := 0.0
	for _, st := range cargo.Stacks {
		if def, ok := env.Content.Item(st.Item); ok {
			used += def.Volume * float64(st.Qty)
		}
	}
	cargo.Capacity = used // zero free space
	frame()
	if strings.Contains(r.status.Label, "cargo full") {
		t.Errorf("status = %q, want no cargo-full warning", r.status.Label)
	}
	if !strings.Contains(r.status.Label, "→ hangar") {
		t.Errorf("status = %q, want the hangar destination", r.status.Label)
	}
	uitest.FireButton(ui, r.refine)
	ui.Update()
	if len(*cmds) != 2 {
		t.Fatalf("refine enqueued %d commands total, want 2", len(*cmds))
	}
	env.Sim.Queue((*cmds)[1])
	env.Sim.Step()
	if got := giveItemCount(env, "ore-cordite"); got != 100 {
		t.Errorf("ore after refining a full hold = %d, want 100", got)
	}
	if got := hangarItemCount(env, "min-ferrite"); got != 105 {
		t.Errorf("ferrite in the hangar = %d, want 105 (70 + floor(0.35×100))", got)
	}
}

// TestRefinePreviewWholeBatch pins the content table's per-batch yields: at
// 100 ore every fraction lands on whole minerals, so the preview is exact.
func TestRefinePreviewWholeBatch(t *testing.T) {
	env, _ := newChromeEnv(t)
	r := &Refinery{cv: env}
	want := map[content.ItemID]string{
		"ore-sterngrit":  "45 Ferrite",
		"ore-cordite":    "35 Ferrite · 30 Nickel",
		"ore-lucidite":   "25 Cobalt · 30 Nickel · 20 Silex",
		"ore-glacierite": "25 Argon · 35 Glacite",
	}
	for ore, w := range want {
		def, ok := env.Content.Item(ore)
		if !ok {
			t.Fatalf("missing %s", ore)
		}
		if got := r.outputsText(refineOutputs(env, def, sim.RefineBatch)); got != w {
			t.Errorf("%s at %d = %q, want %q", ore, sim.RefineBatch, got, w)
		}
	}
}

// hangarItemCount counts an item in the station hangar.
func hangarItemCount(env *testEnv, item content.ItemID) int {
	n := 0
	for _, st := range env.Sim.Hangar.Items {
		if st.Item == item {
			n += st.Qty
		}
	}
	return n
}

// giveItemCount counts an item in the player's general cargo.
func giveItemCount(env *testEnv, item content.ItemID) int {
	cargo, ok := env.Sim.Cargos.Get(env.Sim.Player())
	if !ok {
		return 0
	}
	n := 0
	for _, st := range cargo.Stacks {
		if st.Item == item {
			n += st.Qty
		}
	}
	return n
}
