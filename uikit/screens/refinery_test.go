package screens

import (
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/internal/uitest"
)

// TestRefineryWindow: the ore list, the per-batch yield column, the
// selection gesture opening the quantity picker on the whole-batch
// ladder (100, 200, … capped at the stock), the Confirm's CmdRefine
// end-to-end, and the hangar destination (a full ship hold never blocks
// a refine).
func TestRefineryWindow(t *testing.T) {
	env, cmds := newChromeEnv(t)
	r := NewRefinery(env)
	ui := newQtyUI(t, env, r.Win())
	q := env.QtyWindow()
	frame := func() { pumpQty(t, ui, env, r.Sync, 1024, 700) }
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

	// Undocked: the ore is listed with its one-batch yield, the row's
	// Refine verb is grayed, and selecting it opens nothing.
	row := corditeRow()
	if got := r.ores.Rows[row].Cells[1]; got != "100" {
		t.Errorf("held cell = %q, want 100 (60 shipped + 40 added)", got)
	}
	// The column quotes one batch: floor(0.35×100)=35 sideron, floor(0.3×100)=30 nixium.
	if got := r.ores.Rows[row].Cells[2]; got != "35 Sideron · 30 Nixium" {
		t.Errorf("yield cell = %q, want 35 Sideron · 30 Nixium", got)
	}
	if act := r.ores.Rows[row].Spec.Actions[0]; !act.Disabled {
		t.Fatal("Refine verb enabled while undocked")
	}
	clickRow(ui, r.ores, row)
	frame()
	if q.IsOpen() {
		t.Fatal("picker opened away from a refine station")
	}

	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: home})
	clickRow(ui, r.ores, row)
	frame()
	if !r.hasSel || r.sel.ore != "ore-cordite" {
		t.Fatalf("selection = %+v (hasSel=%v), want cordite", r.sel, r.hasSel)
	}
	if !q.IsOpen() {
		t.Fatal("selecting an ore opened no picker")
	}
	if got := q.TitleText(); got != "Quantity" {
		t.Errorf("picker title = %q, want the base name Quantity", got)
	}
	if got := q.Value(); got != 100 {
		t.Errorf("picker opens at %d, want the first whole batch 100", got)
	}
	if got := len(q.Slider().TickPositions()); got != 1 {
		t.Errorf("ladder ticks = %d, want 1 (one whole batch held)", got)
	}
	if got := q.DetailText(); !strings.Contains(got, "Refine 100 × Kerngrit Ore → 35 Sideron · 30 Nixium") {
		t.Errorf("preview = %q, want the batch conversion", got)
	}
	if got := q.NoteText(); !strings.Contains(got, "hangar") {
		t.Errorf("preview note = %q, want the hangar destination", got)
	}
	if q.Confirm().GetWidget().Disabled {
		t.Fatal("Confirm grayed at a refine station with a whole batch")
	}

	// 250 held: the ladder grows to two rungs, capped at 200 — never a
	// partial 250.
	giveItem(env.Sim, "ore-cordite", 150)
	frame()
	if got := len(q.Slider().TickPositions()); got != 2 {
		t.Fatalf("ladder ticks = %d, want 2 at 250 held", got)
	}
	qtyClick(t, ui, env, r.Sync, 200)
	if got := q.Value(); got != 200 {
		t.Fatalf("picker after picking the top rung = %d, want 200", got)
	}
	if got := q.DetailText(); !strings.Contains(got, "Refine 200 × Kerngrit Ore → 70 Sideron · 60 Nixium") {
		t.Errorf("preview = %q, want the 200-ore yield", got)
	}

	// Confirm enqueues the CmdRefine the sim executes.
	uitest.FireButton(ui, q.Confirm())
	if len(*cmds) != 1 {
		t.Fatalf("refine enqueued %d commands, want 1", len(*cmds))
	}
	cmd, ok := (*cmds)[0].(sim.CmdRefine)
	if !ok || cmd.Item != "ore-cordite" || cmd.Qty != 200 {
		t.Fatalf("cmd = %#v, want CmdRefine{ore-cordite 200}", (*cmds)[0])
	}
	if q.IsOpen() {
		t.Error("picker stayed open after Confirm")
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

	// 50 left is under one batch: the selection still opens the picker, on
	// an empty ladder with Confirm grayed and the wording a batch needs —
	// and the window's status line says the same.
	frame()
	clickRow(ui, r.ores, row)
	frame()
	if !q.IsOpen() {
		t.Fatal("the picker must still open below one batch")
	}
	if !q.Confirm().GetWidget().Disabled {
		t.Fatal("Confirm enabled with less than one batch")
	}
	if got := len(q.Slider().TickPositions()); got != 0 {
		t.Errorf("ladder ticks = %d, want the empty ladder below one batch", got)
	}
	if got := q.DetailText(); !strings.Contains(got, "One batch needs 100 ore — you hold 50 Kerngrit Ore.") {
		t.Errorf("preview = %q, want the one-batch message", got)
	}
	if !strings.Contains(r.status.Label, "One batch needs 100 ore — you hold 50 Kerngrit Ore.") {
		t.Errorf("status = %q, want the one-batch message", r.status.Label)
	}
	uitest.FireButton(ui, q.Confirm())
	if len(*cmds) != 1 {
		t.Fatalf("grayed Confirm enqueued a command (%d total)", len(*cmds))
	}
	q.Close()

	// A packed-to-capacity hold never blocks a refine: the minerals always
	// land in the station hangar, so the window makes no cargo-full
	// prediction.
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
	clickRow(ui, r.ores, row)
	frame()
	if !strings.Contains(q.NoteText(), "hangar") {
		t.Errorf("preview note = %q, want the hangar destination", q.NoteText())
	}
	// The seed is the last confirmed 200; pick one batch again for this
	// check's figures.
	q.Slider().SetValue(100)
	frame()
	uitest.FireButton(ui, q.Confirm())
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
	want := map[content.ItemID]string{
		"ore-sterngrit":  "45 Sideron",
		"ore-cordite":    "35 Sideron · 30 Nixium",
		"ore-lucidite":   "25 Zaffrite · 30 Nixium · 20 Vetril",
		"ore-glacierite": "25 Helion · 35 Cryolith",
	}
	for ore, w := range want {
		def, ok := env.Content.Item(ore)
		if !ok {
			t.Fatalf("missing %s", ore)
		}
		if got := outputsText(env, refineOutputs(env, def, sim.RefineBatch)); got != w {
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
