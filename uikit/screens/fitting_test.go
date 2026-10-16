package screens

import (
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/widgets"
	"testing"
)

// TestFittingRowTooltips pins the Fitting window's module rows to the
// rack's builder: a filled slot row carries the same structured panel (slot
// in the sub line, no rack-only hints), an empty slot carries none, and the
// hangar's not-yet-fitted modules get the slotless variant while stored
// hulls stay bare.
func TestFittingRowTooltips(t *testing.T) {
	env, _ := newChromeEnv(t)
	f := NewFitting(env)
	ui := newWindowUI(t, env, f.Win())
	frame := func() { uitest.PumpWindow(t, ui, f.Sync, 1024, 700) }
	frame()
	frame()

	if len(f.slots.Rows) == 0 {
		t.Fatal("slot table is empty")
	}
	fitted, empty := -1, -1
	for i, r := range f.slots.Rows {
		pick := r.Data.(widgets.SlotPick)
		if pick.Mod != "" && fitted < 0 {
			fitted = i
		}
		if pick.Mod == "" && empty < 0 {
			empty = i
		}
	}
	if fitted < 0 {
		t.Fatal("fixture: no fitted slot row")
	}

	// The first filled row is the rack's first high slot: same module, same
	// panel, same sub line — one builder, two surfaces.
	if got := f.slots.RowWidget(fitted).GetWidget().ToolTips; len(got) != 1 {
		t.Fatalf("fitted slot row carries %d tooltips, want 1", len(got))
	}
	m := f.slots.Tips[fitted].View.Model()
	if m.Title != "EMP Turret" || m.Sub != "High slot 1 · Weapon" {
		t.Errorf("slot row tooltip = %q/%q, want EMP Turret / High slot 1 · Weapon", m.Title, m.Sub)
	}
	if m.Icon != "mod_weapon" {
		t.Errorf("slot row icon = %q, want mod_weapon", m.Icon)
	}
	if len(m.Hints) != 0 {
		t.Errorf("slot row carries rack hints %q — a table row neither clicks nor cycles ammo", m.Hints)
	}
	if tipSection(m, "CHARGE") != nil {
		t.Error("slot row guesses a charge it cannot know")
	}

	// An empty slot row has no module to describe.
	if empty >= 0 {
		if got := f.slots.RowWidget(empty).GetWidget().ToolTips; len(got) != 0 {
			t.Errorf("empty slot row carries %d tooltips, want 0", len(got))
		}
	}

	// Hangar: fit-able modules get the slotless panel, stored hulls none.
	env.Sim.Hangar.Items = append(env.Sim.Hangar.Items,
		content.CargoStack{Item: "mod-rail-turret", Qty: 1})
	env.Sim.Hangar.Ships = append(env.Sim.Hangar.Ships, sim.StoredShip{Hull: "ship-sloop"})
	frame()
	if len(f.hangar.Rows) == 0 {
		t.Fatal("hangar table is empty after seeding it")
	}
	for i, r := range f.hangar.Rows {
		w := f.hangar.RowWidget(i)
		if w == nil {
			continue
		}
		switch data := r.Data.(type) {
		case content.ModuleID:
			if len(w.GetWidget().ToolTips) != 1 {
				t.Fatalf("hangar module row %d carries %d tooltips, want 1", i, len(w.GetWidget().ToolTips))
			}
			def, ok := env.Content.Module(data)
			if !ok {
				t.Fatalf("hangar module %s missing from content", data)
			}
			hm := f.hangar.Tips[i].View.Model()
			if hm.Title != def.Name || hm.Sub != widgets.TipSub("", def.Kind) {
				t.Errorf("hangar module tooltip = %q/%q, want %q/%q",
					hm.Title, hm.Sub, def.Name, widgets.TipSub("", def.Kind))
			}
			// No slot, no charge, no rack hints: a hangar row is neither.
			if len(hm.Hints) != 0 {
				t.Errorf("hangar module tooltip carries rack hints %q", hm.Hints)
			}
		case content.ShipID:
			if len(w.GetWidget().ToolTips) != 0 {
				t.Errorf("hangar hull row %d carries a module tooltip", i)
			}
		}
	}
}
