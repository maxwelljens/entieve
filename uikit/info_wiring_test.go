package uikit

import (
	"image"
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// rightClickRow fires a row's right-button click (with the widget and an
// offset, so the menu anchors where the cursor was) and dispatches it.
func rightClickRow(t *testing.T, ui *ebitenui.UI, tbl *Table, i int) {
	t.Helper()
	w := tbl.rowCts[i].GetWidget()
	w.MouseButtonClickedEvent.Fire(&widget.WidgetMouseButtonClickedEventArgs{
		Widget:  w,
		Button:  ebiten.MouseButtonRight,
		OffsetX: 12,
		OffsetY: 6,
	})
	ui.Update()
}

// entry returns the menu entry with the label (fails when missing).
func entry(t *testing.T, env Env, label string) MenuEntry {
	t.Helper()
	m := env.Menu()
	if !m.IsOpen() {
		t.Fatalf("no menu open (want %q)", label)
	}
	for _, e := range m.Entries() {
		if e.Label == label {
			return e
		}
	}
	t.Fatalf("menu has no %q entry: %+v", label, m.Entries())
	return MenuEntry{}
}

// TestMarketRowMenu: right-clicking a market row opens the menu on it
// (Show Info first and bold, then Buy/Sell driving the toolbar's own
// handlers), Buy and Sell read disabled while undocked and enabled once
// docked, pressing Buy enqueues CmdBuy for the clicked row at the stepper
// quantity, and a double-click opens Show Info on the row's item.
func TestMarketRowMenu(t *testing.T) {
	env, cmds := newTestEnv(t)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	frame := func() { pumpWindow(t, ui, m.Sync, 1024, 700) }
	frame()
	frame()
	if len(m.tbl.rows) == 0 {
		t.Fatal("fixture: empty market")
	}
	wantItem := m.tbl.rows[0].Data.(content.ItemID)

	// Undocked: Buy/Sell visible but disabled.
	rightClickRow(t, ui, m.tbl, 0)
	for _, label := range []string{"Show Info", "Buy", "Sell"} {
		if e := entry(t, env, label); label == "Show Info" {
			if !e.Bold || e.Disabled {
				t.Errorf("Show Info = %+v, want bold and enabled", e)
			}
		} else if !e.Disabled {
			t.Errorf("%s enabled while undocked", label)
		}
	}
	clickMenuEntry(t, env, "Buy") // disabled: closes, enqueues nothing
	if len(*cmds) != 0 {
		t.Fatalf("undocked menu Buy enqueued %d commands, want 0", len(*cmds))
	}

	// The clicked row became the trade target.
	if !m.hasSel || m.sel != wantItem {
		t.Fatalf("right-click selection = %v/%v, want %s", m.hasSel, m.sel, wantItem)
	}

	// Docked: the same menu, now armed.
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-solace"})
	frame()
	rightClickRow(t, ui, m.tbl, 0)
	if e := entry(t, env, "Buy"); e.Disabled {
		t.Fatal("Buy still disabled while docked")
	}
	pressMenuEntry(t, env, "Buy")
	if env.Menu().IsOpen() {
		t.Fatal("menu did not close after Buy")
	}
	rightClickRow(t, ui, m.tbl, 0)
	pressMenuEntry(t, env, "Sell")

	if len(*cmds) != 2 {
		t.Fatalf("menu trade enqueued %d commands, want 2", len(*cmds))
	}
	buy, ok := (*cmds)[0].(sim.CmdBuy)
	if !ok || buy.Item != wantItem || buy.Qty != 1 {
		t.Errorf("cmd[0] = %#v, want CmdBuy{%s 1}", (*cmds)[0], wantItem)
	}
	sell, ok := (*cmds)[1].(sim.CmdSell)
	if !ok || sell.Item != wantItem || sell.Qty != 1 {
		t.Errorf("cmd[1] = %#v, want CmdSell{%s 1}", (*cmds)[1], wantItem)
	}

	// Double-click: Show Info on the row's item (no menu).
	env.InfoWindow().Close()
	clickRow(ui, m.tbl, 0)
	clickRow(ui, m.tbl, 0)
	if !env.InfoWindow().IsOpen() {
		t.Fatal("double-click on a market row did not open Show Info")
	}
	if got, want := env.InfoWindow().TitleText(), itemLabel(env, wantItem)+" — Info"; got != want {
		t.Errorf("info title = %q, want %q", got, want)
	}
}

// TestOverviewEntityMenu: the overview's right-click menu leads with
// Show Info (disabled for entities without a def) and offers exactly the
// verbs the Selected Item window enqueues — warp/orbit at the selectors'
// values, approach and lock — gated the way those buttons gate.
func TestOverviewEntityMenu(t *testing.T) {
	k, cmds := newTestKit(t)
	env := k.panelEnv()
	k.Sync()

	// A warpable celestial (landmarks resolve to no def → Show Info
	// disabled there) and a lockable ship (resolves to a def).
	var celestial, ship sim.Info
	for _, inf := range k.Sim.Grid() {
		if inf.CanWarp && celestial.Entity == 0 {
			celestial = inf
		}
		if inf.Kind == sim.KindShip && inf.Entity != k.Sim.Player() && ship.Entity == 0 {
			ship = inf
		}
	}
	if celestial.Entity.IsZero() {
		t.Fatal("fixture: no warpable celestial")
	}

	k.entityMenu(celestial.Entity, image.Point{X: 100, Y: 100})
	m := env.Menu()
	if !m.IsOpen() {
		t.Fatal("entity menu did not open")
	}
	first := m.Entries()[0]
	if first.Label != "Show Info" || !first.Bold || !first.Disabled {
		t.Fatalf("first entry = %+v, want a bold, disabled Show Info (celestial)", first)
	}
	wantWarpOff := !celestial.CanWarp || celestial.Distance < sim.WarpMinRange
	if e := entry(t, env, "Warp to"); e.Disabled != wantWarpOff {
		t.Errorf("Warp to disabled = %v, want %v (CanWarp=%v distance=%v)",
			e.Disabled, wantWarpOff, celestial.CanWarp, celestial.Distance)
	}
	if e := entry(t, env, "Approach"); e.Disabled {
		t.Error("Approach disabled for a celestial")
	}
	// Landmarks are not lockable kinds.
	if e := entry(t, env, "Lock target"); !e.Disabled {
		t.Error("Lock target enabled for a landmark")
	}

	pressMenuEntry(t, env, "Orbit")
	if len(*cmds) != 1 {
		t.Fatalf("Orbit enqueued %d commands, want 1", len(*cmds))
	}
	if c, ok := (*cmds)[0].(sim.CmdOrbit); !ok || c.Target != celestial.Entity || c.Range != k.OrbitRange() {
		t.Errorf("cmd = %#v, want CmdOrbit{%v @ OrbitRange %v}",
			(*cmds)[0], celestial.Entity, k.OrbitRange())
	}

	// A ship: warp gated off (not a celestial), lock armed, Show Info
	// resolves to the caste/hull def.
	if ship.Entity.IsZero() {
		t.Skip("showcase grid has no visible ship")
	}
	k.entityMenu(ship.Entity, image.Point{X: 100, Y: 100})
	if e := entry(t, env, "Warp to"); !e.Disabled {
		t.Error("Warp to enabled for a non-warpable ship")
	}
	if e := entry(t, env, "Lock target"); e.Disabled {
		t.Error("Lock target disabled for a ship")
	}
	if e := entry(t, env, "Show Info"); e.Disabled {
		t.Error("Show Info disabled for a ship with a hull def")
	}
	pressMenuEntry(t, env, "Lock target")
	k.entityMenu(ship.Entity, image.Point{X: 100, Y: 100}) // the menu closed with the press
	pressMenuEntry(t, env, "Show Info")
	if len(*cmds) != 2 {
		t.Fatalf("Lock target enqueued %d commands, want 2", len(*cmds))
	}
	if c, ok := (*cmds)[1].(sim.CmdLock); !ok || c.Target != ship.Entity {
		t.Errorf("cmd = %#v, want CmdLock{%v}", (*cmds)[1], ship.Entity)
	}
	if !k.InfoOpen() {
		t.Error("Show Info from the entity menu did not open the window")
	}
}

// TestInventoryAndFittingMenus: the inventory's rows carry one Show Info
// entry (and the double-click swap); a fitting slot row carries Show Info
// plus the Fit/Unefit actions driving the toolbar's handlers and gating.
func TestInventoryAndFittingMenus(t *testing.T) {
	env, cmds := newTestEnv(t)

	inv := NewInventory(env)
	uiInv := newWindowUI(t, env, inv.Win())
	pumpWindow(t, uiInv, inv.Sync, 1024, 700)
	pumpWindow(t, uiInv, inv.Sync, 1024, 700)
	if len(inv.cargo.rows) == 0 {
		t.Fatal("fixture: empty cargo")
	}
	rightClickRow(t, uiInv, inv.cargo, 0)
	ents := env.Menu().Entries()
	if len(ents) != 1 || ents[0].Label != "Show Info" || !ents[0].Bold {
		t.Fatalf("inventory menu = %+v, want the single Show Info entry", ents)
	}
	pressMenuEntry(t, env, "Show Info")
	if !env.InfoWindow().IsOpen() {
		t.Fatal("inventory Show Info did not open the window")
	}

	// Double-click: the same swap, no menu.
	env.InfoWindow().Close()
	clickRow(uiInv, inv.cargo, 0)
	clickRow(uiInv, inv.cargo, 0)
	if !env.InfoWindow().IsOpen() {
		t.Fatal("double-click on an inventory row did not open Show Info")
	}

	f := NewFitting(env)
	uiFit := newWindowUI(t, env, f.Win())
	pumpWindow(t, uiFit, f.Sync, 1024, 700)
	pumpWindow(t, uiFit, f.Sync, 1024, 700)
	fitted := -1
	for i, r := range f.slots.rows {
		if pick, ok := r.Data.(slotPick); ok && pick.mod != "" {
			fitted = i
			break
		}
	}
	if fitted < 0 {
		t.Fatal("fixture: no fitted slot row")
	}
	before := len(*cmds)
	rightClickRow(t, uiFit, f.slots, fitted)
	// The right-clicked row is now the selection: Unefit is armed, Fit is
	// not (the slot is filled).
	if e := entry(t, env, "Unefit slot"); e.Disabled {
		t.Error("Unefit disabled for a filled slot row")
	}
	if e := entry(t, env, "Fit module"); !e.Disabled {
		t.Error("Fit enabled for a filled slot row")
	}
	pressMenuEntry(t, env, "Unefit slot")
	if len(*cmds) != before+1 {
		t.Fatalf("Unefit enqueued %d commands, want 1", len(*cmds)-before)
	}
	if _, ok := (*cmds)[before].(sim.CmdUnfitModule); !ok {
		t.Errorf("cmd = %T, want sim.CmdUnfitModule", (*cmds)[before])
	}
}
