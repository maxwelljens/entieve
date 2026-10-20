package screens

import (
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/internal/uitest"
)

// TestRefineryWindow: the ore list, the yield preview (the sim's own
// floor(RefineYield × qty) math), the typed CmdRefine enqueue end-to-end and
// the EvCargoFull prediction.
func TestRefineryWindow(t *testing.T) {
	env, cmds := newChromeEnv(t)
	r := NewRefinery(env)
	ui := newWindowUI(t, env, r.Win())
	frame := func() { uitest.PumpWindow(t, ui, r.Sync, 1024, 700) }

	const home = sim.StationID("cel-haven-rest") // shipped: refine service
	giveItem(env.Sim, "ore-cordite", 40)
	frame()

	// Undocked: the ore is listed, Refine is visible but grayed.
	row := -1
	for i, tr := range r.ores.Rows {
		if d, ok := tr.Data.(refineryRow); ok && d.ore == "ore-cordite" {
			row = i
		}
	}
	if row < 0 {
		t.Fatalf("no cordite row in %+v", r.ores.Rows)
	}
	if got := r.ores.Rows[row].Cells[1]; got != "100" {
		t.Errorf("held cell = %q, want 100 (60 shipped + 40 added)", got)
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
		t.Fatal("Refine grayed at a refine station with ore")
	}

	// Stepper to 10: floor(0.35×10)=3 ferrite, floor(0.3×10)=3 nickel —
	// the exact CmdRefine conversion, in sorted mineral order.
	uitest.FireButton(ui, r.plus)
	frame()
	if got := r.ores.Rows[row].Cells[2]; got != "3 Ferrite · 3 Nickel" {
		t.Errorf("yield cell = %q, want 3 Ferrite · 3 Nickel", got)
	}
	if !strings.Contains(r.status.Label, "Refine 10 × Cordite Ore → 3 Ferrite · 3 Nickel") {
		t.Errorf("status = %q, want the yield preview", r.status.Label)
	}

	// The button enqueues the CmdRefine the sim executes.
	uitest.FireButton(ui, r.refine)
	if len(*cmds) != 1 {
		t.Fatalf("refine enqueued %d commands, want 1", len(*cmds))
	}
	cmd, ok := (*cmds)[0].(sim.CmdRefine)
	if !ok || cmd.Item != "ore-cordite" || cmd.Qty != 10 {
		t.Fatalf("cmd = %#v, want CmdRefine{ore-cordite 10}", (*cmds)[0])
	}
	env.Sim.Queue(cmd)
	env.Sim.Step()
	if got := giveItemCount(env, "ore-cordite"); got != 90 {
		t.Errorf("ore left = %d, want 90", got)
	}
	if got := giveItemCount(env, "min-ferrite"); got != 3 {
		t.Errorf("ferrite = %d, want 3", got)
	}
	if got := giveItemCount(env, "min-nickel"); got != 3 {
		t.Errorf("nickel = %d, want 3", got)
	}

	// Cargo full: the window predicts it from the same volume math the sim
	// uses, and the press still fires the sim's EvCargoFull (nothing moves).
	cargo, _ := env.Sim.Cargos.Get(env.Sim.Player())
	cargo.Capacity = 90 // exactly the ore volume already held: zero free space
	frame()
	if !strings.Contains(r.status.Label, "cargo full") {
		t.Errorf("status = %q, want the cargo-full warning", r.status.Label)
	}
	uitest.FireButton(ui, r.refine)
	if len(*cmds) != 2 {
		t.Fatalf("refine enqueued %d commands total, want 2", len(*cmds))
	}
	env.Sim.Queue((*cmds)[1])
	env.Sim.Step()
	full := false
	for _, ev := range env.Sim.Events() {
		if ev.Kind == sim.EvCargoFull {
			full = true
		}
	}
	if !full {
		t.Fatal("refining into a full hold did not emit EvCargoFull")
	}
	if got := giveItemCount(env, "ore-cordite"); got != 90 {
		t.Errorf("ore after the full-hold refusal = %d, want 90", got)
	}
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
