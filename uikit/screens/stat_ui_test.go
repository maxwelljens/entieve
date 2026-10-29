package screens

import (
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// TestShipPagePairsRows pins the ship page's pairing: the survival block
// opens with a three-cell layer line and continues with the 2×2 resist grid,
// and the new FITTING block carries the hull's limits.
func TestShipPagePairsRows(t *testing.T) {
	env, _ := newTestEnv(t)
	player := env.Sim.Player()
	ship, ok := env.ShipState(player)
	if !ok {
		t.Fatal("player has no ship")
	}
	page, ok := infoPageFor(env, widgets.ShipRef(ship.Hull))
	if !ok {
		t.Fatalf("no page for %s", ship.Hull)
	}

	surv := sectionOf(page, "SURVIVAL")
	if surv == nil {
		t.Fatal("no SURVIVAL section")
	}
	if len(surv.Lines) < 3 || len(surv.Lines[0]) != 3 {
		t.Fatalf("SURVIVAL lines = %v, want a three-cell layer line first", surv.Lines)
	}
	for i, line := range surv.Lines[1:] {
		if len(line) != 2 {
			t.Errorf("resist line %d = %d cells, want 2 (the 2×2 grid)", i, len(line))
		}
	}

	fit := sectionOf(page, "FITTING")
	if fit == nil {
		t.Fatal("ship page has no FITTING section")
	}
	r, ok := rowOf(fit, "Grid quotas")
	if !ok || r.Value == "" {
		t.Fatalf("grid quota row = %+v (ok=%v), want the hull's allowances", r, ok)
	}
	if len(fit.Lines) < 2 || len(fit.Lines[len(fit.Lines)-1]) != 2 {
		t.Errorf("FITTING lines = %v, want the quota pair on one line", fit.Lines)
	}
}

// TestSectionBarsCarryGlyphs pins the section bar's composition: a stat
// glyph, the tracked label and the rule — with an empty mark on the identity
// blocks.
func TestSectionBarsCarryGlyphs(t *testing.T) {
	env, _ := newTestEnv(t)
	w := NewInfoWin(env)
	for _, tc := range []struct {
		label string
		icon  string
	}{
		{"SURVIVAL", "mod_defense"},
		{"FITTING", "row_module"},
		{"CARGO", "stat_cargo"},
		{"CONSTRUCTION", ""},
	} {
		bar := w.sectionBar(tc.label)
		if n := len(bar.Children()); n != 3 {
			t.Fatalf("%s bar has %d cells, want mark/label/rule", tc.label, n)
		}
		mark := bar.Children()[0].(*widget.Container)
		if tc.icon == "" {
			if len(mark.Children()) != 0 {
				t.Errorf("%s identity bar carries a glyph", tc.label)
			}
			continue
		}
		if len(mark.Children()) != 1 {
			t.Fatalf("%s mark has %d children, want one glyph", tc.label, len(mark.Children()))
		}
		if got, _ := theme.SectionBar(env.Pal(), tc.label); got != tc.icon {
			t.Errorf("%s bar glyph = %q, want %q", tc.label, got, tc.icon)
		}
	}
}

// TestFitActionQuotesHullLimit is the greyed-Fit gate: a module the hull's
// fitting rules refuse keeps the row action disabled and quotes the rule in
// the sim's own wording (the menu has no sub-line, so the label carries it).
func TestFitActionQuotesHullLimit(t *testing.T) {
	env, _ := newTestEnv(t)
	f := NewFitting(env)
	ui := newWindowUI(t, env, f.Win())
	frame := func() { uitest.PumpWindow(t, ui, f.Sync, 1024, 700) }
	frame()
	frame()

	// Free the last high slot so an empty tile exists to aim the action at.
	fit, ok := env.Sim.Fittings.Get(env.Sim.Player())
	if !ok {
		t.Fatal("player has no fitting")
	}
	fit.High = fit.High[:len(fit.High)-1]
	frame()

	// Prefer a class-quota refusal (the rule this test is about); any other
	// fitting rule is the fallback so the test survives content changes.
	var mod content.ModuleID
	var def *content.ModuleDef
	var issue, fallback *content.FitIssue
	var fallbackID content.ModuleID
	var fallbackDef *content.ModuleDef
	for id, d := range env.Content.Modules {
		if d.Slot != content.SlotHigh {
			continue
		}
		iss := f.fitIssue(d)
		if iss == nil {
			continue
		}
		if iss.Rule == content.RuleGrid || iss.Rule == content.RuleCPU {
			mod, def, issue = id, d, iss
			break
		}
		if fallback == nil {
			fallbackID, fallbackDef, fallback = id, d, iss
		}
	}
	if issue == nil {
		mod, def, issue = fallbackID, fallbackDef, fallback
	}
	if issue == nil {
		t.Fatal("shipped content has no high module the showcase hull refuses")
	}

	// Aim at the empty high tile of the module's own class.
	var pick widgets.SlotPick
	for _, tile := range f.tiles[0] {
		if fitTileShown(tile) && tile.mod == "" {
			pick = widgets.SlotPick{Ref: tile.slot}
			break
		}
	}
	if pick.Ref.Class == "" {
		t.Fatal("no empty high slot after freeing one")
	}
	f.selMod, f.hasMod = mod, true
	f.selSlot, f.hasSlot = pick, true
	frame()

	if f.fitMod.GetWidget().Disabled != true {
		t.Error("toolbar Fit enabled for a module the hull refuses")
	}
	spec := f.slotSpec(pick)
	if len(spec.Actions) == 0 || !spec.Actions[0].Disabled {
		t.Fatal("slot menu Fit action is not disabled")
	}
	want := content.FitIssueText(def.Name, issue)
	if (issue.Rule == content.RuleGrid || issue.Rule == content.RuleCPU) && !strings.Contains(want, "quota") {
		t.Fatalf("class refusal %q does not quote a quota", want)
	}
	if !strings.Contains(spec.Actions[0].Label, want) {
		t.Errorf("fit action label = %q, want it to quote %q", spec.Actions[0].Label, want)
	}
}
