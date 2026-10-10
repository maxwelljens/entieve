package uikit

import (
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// newWindowUI mounts windows on their own UI (the kit's AddWindow seam
// does the same on the kit's UI) and pumps once so the deferred
// constructor handlers register.
func newWindowUI(t *testing.T, env Env, wins ...*widget.Window) *ebitenui.UI {
	t.Helper()
	ui := &ebitenui.UI{Container: widget.NewContainer(), PrimaryTheme: env.Theme}
	for _, w := range wins {
		ui.AddWindow(w)
	}
	ui.Update()
	return ui
}

// pumpWindow runs one frame: Sync (rows + gating), Update (dispatch
// deferred events) and Draw (layout).
func pumpWindow(t *testing.T, ui *ebitenui.UI, sync func(), w, h int) {
	t.Helper()
	if sync != nil {
		sync()
	}
	ui.Update()
	ui.Draw(ebiten.NewImage(w, h))
}

// fireButton clicks a button through the deferred event queue.
func fireButton(ui *ebitenui.UI, b *widget.Button) {
	b.ClickedEvent.Fire(&widget.ButtonClickedEventArgs{Button: b})
	ui.Update()
}

// TestMarketGatingAndEnqueue: market buttons are disabled while
// undocked, and Buy/Sell enqueue the selected order at the stepper
// quantity once docked.
func TestMarketGatingAndEnqueue(t *testing.T) {
	env, cmds := newTestEnv(t)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	frame := func() { pumpWindow(t, ui, m.Sync, 1024, 700) }
	frame()
	frame()

	if len(m.tbl.rows) == 0 {
		t.Fatal("market table is empty")
	}
	if !m.buy.GetWidget().Disabled || !m.sell.GetWidget().Disabled {
		t.Fatal("trade buttons enabled while undocked")
	}
	m.buyClicked() // guarded even when called directly
	m.sellClicked()
	if len(*cmds) != 0 {
		t.Fatalf("undocked trade enqueued %d commands, want 0", len(*cmds))
	}

	// Docked: the buttons unlock.
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-solace"})
	frame()
	if m.buy.GetWidget().Disabled || m.sell.GetWidget().Disabled {
		t.Fatal("trade buttons still disabled while docked")
	}

	// Select the first order, step the quantity to 10 and trade.
	clickRow(ui, m.tbl, 0)
	fireButton(ui, m.plus)
	if m.qty() != 10 {
		t.Fatalf("qty = %d, want 10", m.qty())
	}
	fireButton(ui, m.buy)
	if len(*cmds) != 1 {
		t.Fatalf("buy enqueued %d commands, want 1", len(*cmds))
	}
	buy, ok := (*cmds)[0].(sim.CmdBuy)
	if !ok {
		t.Fatalf("cmd = %T, want sim.CmdBuy", (*cmds)[0])
	}
	// The table sorts by name, so the selected row — not the sim's
	// canonical stock order — is the trade target.
	wantItem := m.tbl.rows[0].Data.(content.ItemID)
	if buy.Item != wantItem || buy.Qty != 10 {
		t.Errorf("CmdBuy = %+v, want {%s 10}", buy, wantItem)
	}

	fireButton(ui, m.sell)
	if len(*cmds) != 2 {
		t.Fatalf("sell enqueued %d commands total, want 2", len(*cmds))
	}
	sell, ok := (*cmds)[1].(sim.CmdSell)
	if !ok || sell.Item != wantItem || sell.Qty != 10 {
		t.Errorf("cmd = %#v, want CmdSell{%s 10}", (*cmds)[1], wantItem)
	}
}

// TestInventoryTransferAndSell: section selection drives CmdTransfer
// between holds and CmdSell, both gated by the hold's own rules (cargo
// present, ore hold fitted, hangar side and selling docked).
func TestInventoryTransferAndSell(t *testing.T) {
	env, cmds := newTestEnv(t)
	inv := NewInventory(env)
	ui := newWindowUI(t, env, inv.Win())
	frame := func() { pumpWindow(t, ui, inv.Sync, 1024, 700) }
	frame()
	frame()

	if len(inv.cargo.rows) == 0 {
		t.Fatal("cargo section is empty")
	}
	// The showcase sloop has no dedicated ore hold; everything that
	// needs docking is gated while undocked.
	if !inv.toOre.GetWidget().Disabled {
		t.Error("To Ore enabled without an ore hold")
	}
	if !inv.toHangar.GetWidget().Disabled || !inv.sell.GetWidget().Disabled {
		t.Fatal("hangar transfer/sell enabled while undocked")
	}
	inv.transfer("hangar")
	inv.sellClicked()
	if len(*cmds) != 0 {
		t.Fatalf("gated actions enqueued %d commands, want 0", len(*cmds))
	}

	// Select a cargo row: its own hold is never a destination.
	clickRow(ui, inv.cargo, 0)
	frame()
	if !inv.toCargo.GetWidget().Disabled {
		t.Error("To Cargo enabled for a cargo selection")
	}

	// Docked: To Hangar and Sell unlock (the sloop still has no ore hold).
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-solace"})
	frame()
	if inv.toHangar.GetWidget().Disabled {
		t.Fatal("To Hangar still disabled while docked")
	}
	if inv.sell.GetWidget().Disabled {
		t.Fatal("Sell still disabled while docked")
	}

	fireButton(ui, inv.toHangar)
	if len(*cmds) != 1 {
		t.Fatalf("transfer enqueued %d commands, want 1", len(*cmds))
	}
	tr, ok := (*cmds)[0].(sim.CmdTransfer)
	if !ok {
		t.Fatalf("cmd = %T, want sim.CmdTransfer", (*cmds)[0])
	}
	if tr.From != "cargo" || tr.To != "hangar" || tr.Qty != 1 || tr.Item != inv.sel {
		t.Errorf("CmdTransfer = %+v, want from cargo to hangar, item %s qty 1", tr, inv.sel)
	}

	fireButton(ui, inv.sell)
	if len(*cmds) != 2 {
		t.Fatalf("sell enqueued %d commands total, want 2", len(*cmds))
	}
	sl, ok := (*cmds)[1].(sim.CmdSell)
	if !ok || sl.Item != inv.sel || sl.Qty != 1 {
		t.Errorf("cmd = %#v, want CmdSell{%s 1}", (*cmds)[1], inv.sel)
	}
}

// TestFittingActions walks the fitting flow: selection gating, unfit a
// fitted slot, fit a hangar module into the freed slot and assemble an
// owned ship.
func TestFittingActions(t *testing.T) {
	env, cmds := newTestEnv(t)
	f := NewFitting(env)
	ui := newWindowUI(t, env, f.Win())
	frame := func() { pumpWindow(t, ui, f.Sync, 1024, 700) }
	frame()
	frame()

	if len(f.slots.rows) == 0 {
		t.Fatal("slot table is empty")
	}
	if !f.unfit.GetWidget().Disabled || !f.fitMod.GetWidget().Disabled ||
		!f.assemble.GetWidget().Disabled {
		t.Fatal("fitting actions enabled with nothing selected")
	}

	// Free the last high slot so there is an empty slot to fit into
	// (the showcase sloop starts 8/8 fitted).
	fit, ok := env.Sim.Fittings.Get(env.Sim.Player())
	if !ok {
		t.Fatal("player has no fitting")
	}
	fit.High = fit.High[:len(fit.High)-1]
	frame()

	fitted := -1
	empty := -1
	for i, r := range f.slots.rows {
		pick := r.Data.(slotPick)
		if pick.mod != "" && fitted < 0 {
			fitted = i
		}
		if pick.mod == "" && pick.ref.Class == content.SlotHigh && empty < 0 {
			empty = i
		}
	}
	if fitted < 0 || empty < 0 {
		t.Fatalf("expected a fitted and an empty high slot (fitted=%d empty=%d)", fitted, empty)
	}

	// Unfit the selected fitted slot.
	clickRow(ui, f.slots, fitted)
	frame()
	if f.unfit.GetWidget().Disabled {
		t.Fatal("Unfit disabled for a fitted slot")
	}
	fireButton(ui, f.unfit)
	if len(*cmds) != 1 {
		t.Fatalf("unfit enqueued %d commands, want 1", len(*cmds))
	}
	if c, ok := (*cmds)[0].(sim.CmdUnfitModule); !ok || c.Slot != f.selSlot.ref {
		t.Errorf("cmd = %#v, want CmdUnfitModule{%+v}", (*cmds)[0], f.selSlot.ref)
	}

	// Fit a hangar module into the empty slot.
	def, ok := env.Content.Module("mod-rail-turret")
	if !ok || def.Slot != content.SlotHigh {
		t.Fatalf("fixture: mod-rail-turret slot = %q (ok=%v), want a high module", def.Slot, ok)
	}
	env.Sim.Hangar.Items = append(env.Sim.Hangar.Items,
		content.CargoStack{Item: "mod-rail-turret", Qty: 1})
	frame()
	modRow := -1
	for i, r := range f.hangar.rows {
		if _, isMod := r.Data.(content.ModuleID); isMod && modRow < 0 {
			modRow = i
		}
	}
	if modRow < 0 {
		t.Fatal("hangar module row missing")
	}
	clickRow(ui, f.hangar, modRow)
	clickRow(ui, f.slots, empty)
	frame()
	if f.fitMod.GetWidget().Disabled {
		t.Fatal("Fit disabled for a hangar module and a free slot")
	}
	fireButton(ui, f.fitMod)
	if len(*cmds) != 2 {
		t.Fatalf("fit enqueued %d commands, want 2", len(*cmds))
	}
	fc, ok := (*cmds)[1].(sim.CmdFitModule)
	if !ok {
		t.Fatalf("cmd = %T, want sim.CmdFitModule", (*cmds)[1])
	}
	if fc.Item != def.ID || fc.Slot != f.selSlot.ref {
		t.Errorf("CmdFitModule = %+v, want {%s %+v}", fc, def.ID, f.selSlot.ref)
	}

	// Assemble an owned ship from the hangar.
	env.Sim.Hangar.Ships = append(env.Sim.Hangar.Ships,
		sim.StoredShip{Hull: "ship-sloop"})
	frame()
	shipRow := -1
	for i, r := range f.hangar.rows {
		if _, isShip := r.Data.(content.ShipID); isShip {
			shipRow = i
			break
		}
	}
	if shipRow < 0 {
		t.Fatal("hangar ship row missing")
	}
	clickRow(ui, f.hangar, shipRow)
	frame()
	if f.assemble.GetWidget().Disabled {
		t.Fatal("Assemble disabled for an owned ship")
	}
	fireButton(ui, f.assemble)
	if len(*cmds) != 3 {
		t.Fatalf("assemble enqueued %d commands total, want 3", len(*cmds))
	}
	if c, ok := (*cmds)[2].(sim.CmdAssemble); !ok || c.Ship != "ship-sloop" {
		t.Errorf("cmd = %#v, want CmdAssemble{ship-sloop}", (*cmds)[2])
	}
}

// TestStationWindowsDrawSmoke mounts all five windows on one kit and
// draws them headless at both design sizes: no panics, every window
// stays in bounds and every table gets fitted to its cell.
func TestStationWindowsDrawSmoke(t *testing.T) {
	k, _ := newTestKit(t)
	env := k.panelEnv()
	market := NewMarket(env)
	inventory := NewInventory(env)
	fitting := NewFitting(env)
	k.AddWindow(market.Win())
	k.AddWindow(inventory.Win())
	k.AddWindow(fitting.Win())

	sizes := [][2]int{{1280, 720}, {1024, 700}}
	for _, size := range sizes {
		frame := func() {
			k.Sync()
			market.Sync()
			inventory.Sync()
			fitting.Sync()
			if err := k.Update(); err != nil {
				t.Fatalf("%dx%d Update: %v", size[0], size[1], err)
			}
			k.Draw(ebiten.NewImage(size[0], size[1]))
		}
		frame()
		frame()

		for _, w := range []struct {
			name string
			win  *widget.Window
		}{
			{"overview", k.ov.win},
			{"selected", k.sp.win},
			{"market", market.Win()},
			{"inventory", inventory.Win()},
			{"fitting", fitting.Win()},
		} {
			r := w.win.GetContainer().GetWidget().Rect
			if r.Min.X < 0 || r.Min.Y < 0 ||
				r.Max.X > size[0] || r.Max.Y > size[1] {
				t.Errorf("%dx%d: %s window %v out of bounds",
					size[0], size[1], w.name, r)
			}
		}

		for name, tbl := range map[string]*Table{
			"market.cargo":  market.tbl,
			"inv.cargo":     inventory.cargo,
			"inv.ore":       inventory.ore,
			"inv.hangar":    inventory.hangar,
			"fit.resists":   fitting.resists,
			"fit.slots":     fitting.slots,
			"fit.hangar":    fitting.hangar,
			"overview.rows": k.ov.tbl,
		} {
			if tbl.fittedFor <= 0 {
				t.Errorf("%dx%d: %s never fitted (fittedFor=%v)",
					size[0], size[1], name, tbl.fittedFor)
			}
		}
	}
}
