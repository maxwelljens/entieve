package screens

import (
	"image"
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/sim"
)

// TestProficiencyGateWordingHasOneOwner is the drift pin: one state (the
// pilot untrained, the artillery family gated on its autocannon
// prerequisite) must read the same sentence on every surface that words it:
// the sim's refusal, the tech tree node and the family card. Show Info prints
// no sentence: its one PROFICIENCY row draws the required level as the tree's
// bars, dimmed while the gate is shut (the row below). Any surface that
// starts walking the edges or wording the gate itself fails here.
func TestProficiencyGateWordingHasOneOwner(t *testing.T) {
	uitest.UseScale(t, 1)
	env, _ := newChromeEnv(t)

	want := "Requires Autocannon II"
	if got := env.Sim.ProficiencyRequirement("artillery", 0); got != want {
		t.Fatalf("sim gate = %q, want %q", got, want)
	}

	// The tree's model, mapped the way the game maps the sim's families.
	fams := env.Sim.ProficiencyFamilies()
	model := make([]ProfFamily, 0, len(fams))
	for _, f := range fams {
		prereqs := make([]ProfPrereq, 0, len(f.Prereqs))
		for _, p := range f.Prereqs {
			prereqs = append(prereqs, ProfPrereq{Family: p.Family, Level: p.Level})
		}
		model = append(model, ProfFamily{
			Family: f.Family, Name: f.Name, Level: f.Level, XP: f.XP,
			XPNext: f.XPNext, MaxLevel: f.MaxLevel, Prereqs: prereqs,
			Meets: f.Meets, Gate: f.Gate, Starter: f.Starter,
		})
	}
	nodes, _ := proficiencyLayout(model, image.Rect(0, 0, 1280, 720))
	var node profNode
	for _, n := range nodes {
		if n.Family == "artillery" {
			node = n
		}
	}
	if node.GateText != want {
		t.Fatalf("tree node gate = %q, want %q", node.GateText, want)
	}
	card := NewProfInfoWin(env)
	card.Show("artillery", node.Level, node.GateText)
	if card.Gate() != want {
		t.Fatalf("family card gate line = %q, want %q", card.Gate(), want)
	}
	req := proficiencyRows(env.Session, "artillery", 0)
	if len(req) != 1 || req[0].Label != "Artillery" || req[0].PipLevel != 0 || !req[0].PipLocked {
		t.Fatalf("Show Info requirement row = %+v, want one dim Artillery bar row (the gate is 0, the branch is shut)", req)
	}

	// The refusal the fit command actually prints: free a high slot, then
	// ask for the battery the gate refuses.
	at := content.CelID("")
	for _, cel := range env.Sim.System().Celestials {
		if cel.Station != nil {
			at = cel.ID
			break
		}
	}
	if at == "" {
		t.Fatal("fixture: the starting system has no station")
	}
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: at})
	env.Sim.Queue(sim.CmdUnfitModule{Slot: sim.SlotRef{Class: content.SlotHigh, Index: 0}})
	env.Sim.Step()
	env.Sim.Queue(sim.CmdFitModule{Item: "mod-artillery-s", Slot: sim.SlotRef{Class: content.SlotHigh, Index: 2}})
	env.Sim.Step()
	refused := ""
	for _, e := range env.Sim.DrainEvents() {
		if e.Kind == sim.EvLog && strings.HasPrefix(e.Text, want+" to fit ") {
			refused = e.Text
		}
	}
	if refused != want+" to fit Small Artillery" {
		t.Fatalf("fit refusal = %q, want %q", refused, want+" to fit Small Artillery")
	}
}
