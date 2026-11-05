package screens

// The service gate's surface pins: a window opens anywhere, but its verbs
// follow the sim's effective station access (CONTEXT.md "Station access") —
// grey in space, grey where the station does not run the service, grey at a
// remnant that no longer does. The sim side of the same rule (the seam read
// against stationAccess) is pinned in sim/war_test.go.

import (
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/widgets"
)

// TestFittingVerbsFollowFittingService: the fitting window opens in space,
// but Fit/Unefit/Assemble follow the station service the sim refuses on.
func TestFittingVerbsFollowFittingService(t *testing.T) {
	env, cmds := newTestEnv(t)
	f := NewFitting(env)
	ui := newWindowUI(t, env, f.Win())
	frame := func() { uitest.PumpWindow(t, ui, f.Sync, 1024, 700) }
	frame()

	// One fitted slot and one owned hull selected: every verb is otherwise
	// legal, so only the station service can grey it.
	tile := fitTileAt(f, sim.SlotRef{Class: content.SlotHigh, Index: 0})
	if tile == nil || tile.mod == "" {
		t.Fatal("fixture: no fitted high slot")
	}
	clickFitTile(ui, tile)
	env.Sim.Hangar.Ships = append(env.Sim.Hangar.Ships, sim.StoredShip{Hull: "ship-vastrek"})
	frame()
	for i, r := range f.hangar.Rows {
		if _, isShip := r.Data.(content.ShipID); isShip {
			clickRow(ui, f.hangar, i)
		}
	}
	frame()

	// In space: the window is up, the verbs are grey, the row menu agrees.
	if !f.unfit.GetWidget().Disabled || !f.assemble.GetWidget().Disabled {
		t.Error("fitting verbs enabled in space")
	}
	for i, a := range f.slotSpec(f.selSlot).Actions {
		if !a.Disabled {
			t.Errorf("slot menu action %d (%s) enabled in space", i, a.Label)
		}
	}
	before := len(*cmds)

	// Docked at a station that runs fitting: the verbs arm again.
	dockAtStation(t, env.Sim)
	frame()
	if f.unfit.GetWidget().Disabled || f.assemble.GetWidget().Disabled {
		t.Error("fitting verbs greyed at a station that runs fitting")
	}
	if a := f.slotSpec(f.selSlot).Actions; a[1].Disabled {
		t.Error("slot menu Unefit greyed at a station that runs fitting")
	}
	uitest.FireButton(ui, f.unfit)
	if len(*cmds) != before+1 {
		t.Fatalf("docked Unfit enqueued %d commands, want 1", len(*cmds)-before)
	}
	if _, ok := (*cmds)[before].(sim.CmdUnfitModule); !ok {
		t.Errorf("cmd = %T, want sim.CmdUnfitModule", (*cmds)[before])
	}

	// Undocked again: grey again, the window still open.
	env.Sim.Docked.Remove(env.Sim.Player())
	frame()
	if !f.unfit.GetWidget().Disabled {
		t.Error("Unfit still armed after undocking")
	}
	if f.Win().GetContainer().GetWidget().GetVisibility() != widget.Visibility_Show {
		t.Error("the fitting window hid itself in space (visible, greyed, never hidden)")
	}
}

// TestTradingVerbsFollowMarketService: the trade verbs gate on the sim's
// market service, not on "is the player docked" — a docked player at a
// station without a market gets grey Buy/Sell/order launches and no order
// window.
func TestTradingVerbsFollowMarketService(t *testing.T) {
	cv, rec, env := orderStub(nil)
	cv.svc = map[string]bool{content.SvcFitting: true} // docked, but no market
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	uitest.PumpWindow(t, ui, m.Sync, 1024, 700)
	uitest.PumpWindow(t, ui, m.Sync, 1024, 700)
	if m.docked() {
		t.Fatal("the market's trade gate is open at a station without a market service")
	}

	o := NewOrderWin(env)
	if o.Show("item-stub-ore", sim.OrderSell, 1) {
		t.Error("the order window opened without the market service")
	}

	// The same dock, market back: the verbs arm.
	cv.svc = map[string]bool{content.SvcFitting: true, content.SvcMarket: true}
	if !m.docked() {
		t.Error("the market's trade gate is shut at a station that runs market")
	}
	if !o.Show("item-stub-ore", sim.OrderSell, 1) {
		t.Error("the order window refused at a station that runs market")
	}
	_ = rec
	_ = widgets.ContentView(cv)
}

// TestInventorySellFollowsMarketService: the inventory's Sell order and its
// Sell button read the market service, not the bare dock (a dock with no
// market book greys both).
func TestInventorySellFollowsMarketService(t *testing.T) {
	env, _ := newTestEnv(t)
	inv := NewInventory(env)
	ui := newWindowUI(t, env, inv.Win())
	frame := func() { uitest.PumpWindow(t, ui, inv.Sync, 1024, 700) }
	frame()
	if len(inv.cargo.Rows) == 0 {
		t.Fatal("fixture: empty cargo")
	}

	// Docked, but not at a station that runs a market: the verbs are grey.
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-solace-star"})
	frame()
	if a := inv.cargo.Rows[0].Spec.Actions[0]; !a.Disabled {
		t.Error("the inventory's Sell order is armed without a market book")
	}
	inv.sellClicked()
	if env.QtyWindow().IsOpen() {
		t.Error("the Sell button opened the picker without a market book")
	}

	// Docked at the station: both arm.
	dockAtStation(t, env.Sim)
	frame()
	if a := inv.cargo.Rows[0].Spec.Actions[0]; a.Disabled {
		t.Error("the inventory's Sell order stayed grey at a market station")
	}
	clickRow(ui, inv.cargo, 0)
	frame()
	if !inv.hasSel {
		t.Fatal("fixture: the row click selected nothing")
	}
	inv.sellClicked()
	if !env.QtyWindow().IsOpen() {
		t.Error("the Sell button opened no picker at a market station")
	}
}

// TestRemnantGreysCharterServices is the real remnant fixture: the showcase
// world's start system falls to the Works, so its docked station keeps the
// market and the refinery and loses industry and fitting. The rail's entries
// and the fitting window's verbs follow the sim's own answer — the same
// station, before and after the fall.
func TestRemnantGreysCharterServices(t *testing.T) {
	env, _ := newTestEnv(t)
	dockAtStation(t, env.Sim)
	if env.Sim.War() == nil {
		t.Fatal("fixture: world-showcase carries no war")
	}
	r := NewRail(env, []RailEntry{
		{Icon: "svc_market", Title: "Market", Group: "Trade", Service: content.SvcMarket},
		{Icon: "svc_fitting", Title: "Fitting", Group: "Trade", Service: content.SvcFitting},
		{Icon: "svc_manufacturing", Title: "Manufacturing", Group: "Industry", Service: content.SvcIndustry},
		{Icon: "svc_refinery", Title: "Refinery", Group: "Industry", Service: content.SvcRefine},
	})
	c := newChromeUI(env)
	c.AddPanel(r.Container(), r)
	c.Frame(t, 1280, 720)

	f := NewFitting(env)
	ui := newWindowUI(t, env, f.Win())
	frame := func() { uitest.PumpWindow(t, ui, f.Sync, 1024, 700) }
	frame()
	clickFitTile(ui, fitTileAt(f, sim.SlotRef{Class: content.SlotHigh, Index: 0}))
	env.Sim.Hangar.Ships = append(env.Sim.Hangar.Ships, sim.StoredShip{Hull: "ship-vastrek"})
	frame()
	for i, row := range f.hangar.Rows {
		if _, isShip := row.Data.(content.ShipID); isShip {
			clickRow(ui, f.hangar, i)
		}
	}
	frame()

	// Charter space: every service live, every fitting verb armed.
	for _, title := range []string{"Market", "Fitting", "Manufacturing", "Refinery"} {
		if r.Button(title).GetWidget().Disabled {
			t.Errorf("%s grayed at a charter station", title)
		}
	}
	if f.unfit.GetWidget().Disabled || f.assemble.GetWidget().Disabled {
		t.Fatal("fitting verbs grayed at a charter station")
	}

	// The system falls: the sim's own remnant mask, no UI state touched.
	env.Sim.War().Control["sys-solace"] = "works" // sim's controlWorks
	frame()
	c.Frame(t, 1280, 720)
	for _, tc := range []struct {
		title string
		live  bool
	}{
		{"Market", true}, {"Refinery", true},
		{"Fitting", false}, {"Manufacturing", false},
	} {
		if got := !r.Button(tc.title).GetWidget().Disabled; got != tc.live {
			t.Errorf("%s live = %v at a remnant, want %v", tc.title, got, tc.live)
		}
	}
	if !f.unfit.GetWidget().Disabled || !f.fitMod.GetWidget().Disabled || !f.assemble.GetWidget().Disabled {
		t.Error("a fitting verb is still armed at a remnant")
	}
	if a := f.slotSpec(f.selSlot).Actions; !a[0].Disabled || !a[1].Disabled {
		t.Error("a fitting row menu verb is still armed at a remnant")
	}
}
