package screens

import (
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/widgets"
)

// TestTradeableAcrossSurfaces pins that one id reads and draws the same
// {name, icon, ref} on every surface that shows it: the market row, the
// manufacturing recipe row, the Show Info hero and the module tooltip. They
// all go through the one resolver — a surface that re-derives its own ladder
// is the disagreement this pins.
func TestTradeableAcrossSurfaces(t *testing.T) {
	env, _ := factoryEnv(t)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	uitest.PumpWindow(t, ui, m.Sync, 1024, 700)

	f := NewManufacturing(env)
	fui := newQtyUI(t, env, f.Win())
	pumpQty(t, fui, env, f.Sync, 1024, 700)

	for _, id := range []content.ItemID{"mod-autocannon-s", "ship-sloop"} {
		tr := widgets.ResolveTradeable(env.Defs(), id)
		if !tr.Found {
			t.Fatalf("%s: not in content", id)
		}
		ref, ok := tr.Ref()
		if !ok {
			t.Fatalf("%s: no ref", id)
		}

		if got := m.itemIcon(id); got != tr.Icon {
			t.Errorf("%s: market icon = %q, want %q", id, got, tr.Icon)
		}

		ri := recipeRowFor(f, id)
		if ri < 0 {
			t.Fatalf("%s: no recipe row", id)
		}
		if got := f.recipes.Rows[ri].Icons[0]; got != tr.Icon {
			t.Errorf("%s: manufacturing icon = %q, want %q", id, got, tr.Icon)
		}

		p, ok := infoPageFor(env, ref)
		if !ok {
			t.Fatalf("%s: info page did not resolve", id)
		}
		if p.Hero != tr.Icon || p.Title != tr.Name {
			t.Errorf("%s: info hero = %q/%q, want %q/%q", id, p.Title, p.Hero, tr.Name, tr.Icon)
		}

		// Only modules carry a tooltip; where one exists it must agree too.
		if tr.Kind == widgets.InfoModule {
			def, _ := env.Defs().Module(content.ModuleID(id))
			if tip := widgets.ModuleTip(def, "", "", 0); tip.Icon != tr.Icon || tip.Title != tr.Name {
				t.Errorf("%s: tooltip = %q/%q, want %q/%q", id, tip.Title, tip.Icon, tr.Name, tr.Icon)
			}
		}
	}
}
