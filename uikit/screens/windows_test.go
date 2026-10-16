package screens

import (
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"image"
	"math"
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// equalInts compares two int slices.
func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestMarketGatingAndEnqueue: market buttons are disabled while
// undocked, and Buy/Sell enqueue the selected order at the stepper
// quantity once docked.
func TestMarketGatingAndEnqueue(t *testing.T) {
	env, cmds := newTestEnv(t)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	frame := func() { uitest.PumpWindow(t, ui, m.Sync, 1024, 700) }
	frame()
	frame()

	if len(m.tbl.Rows) == 0 {
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
	uitest.FireButton(ui, m.plus)
	if m.qty() != 10 {
		t.Fatalf("qty = %d, want 10", m.qty())
	}
	uitest.FireButton(ui, m.buy)
	if len(*cmds) != 1 {
		t.Fatalf("buy enqueued %d commands, want 1", len(*cmds))
	}
	buy, ok := (*cmds)[0].(sim.CmdBuy)
	if !ok {
		t.Fatalf("cmd = %T, want sim.CmdBuy", (*cmds)[0])
	}
	// The table sorts by name, so the selected row — not the sim's
	// canonical stock order — is the trade target.
	wantItem := m.tbl.Rows[0].Data.(content.ItemID)
	if buy.Item != wantItem || buy.Qty != 10 {
		t.Errorf("CmdBuy = %+v, want {%s 10}", buy, wantItem)
	}

	uitest.FireButton(ui, m.sell)
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
	frame := func() { uitest.PumpWindow(t, ui, inv.Sync, 1024, 700) }
	frame()
	frame()

	if len(inv.cargo.Rows) == 0 {
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

	uitest.FireButton(ui, inv.toHangar)
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

	uitest.FireButton(ui, inv.sell)
	if len(*cmds) != 2 {
		t.Fatalf("sell enqueued %d commands total, want 2", len(*cmds))
	}
	sl, ok := (*cmds)[1].(sim.CmdSell)
	if !ok || sl.Item != inv.sel || sl.Qty != 1 {
		t.Errorf("cmd = %#v, want CmdSell{%s 1}", (*cmds)[1], inv.sel)
	}
}

// TestMarketSelectionRevalidatesOnSync: a selected order that leaves the
// board stops reading as selected — the click-time cache alone would
// keep Buy/Sell armed on an item the market no longer offers.
func TestMarketSelectionRevalidatesOnSync(t *testing.T) {
	env, cmds := newTestEnv(t)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	frame := func() { uitest.PumpWindow(t, ui, m.Sync, 1024, 700) }
	frame()
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-solace"})
	frame()
	clickRow(ui, m.tbl, 0)
	frame()
	if !m.hasSel {
		t.Fatal("fixture: selection did not take")
	}
	item := m.sel

	// Drop that order from the board (save surgery) and sync.
	sv, err := env.Sim.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	sys := env.Sim.System().ID
	var kept []sim.MarketStock
	for _, st := range sv.Market[sys] {
		if st.Item != item {
			kept = append(kept, st)
		}
	}
	if len(kept) == len(sv.Market[sys]) {
		t.Fatalf("fixture: selected item %s is not on the board", item)
	}
	sv.Market[sys] = kept
	if err := env.Sim.Restore(sv); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	frame()

	if m.hasSel {
		t.Errorf("selection %s survived its row leaving the board", item)
	}
	if _, ok := m.tbl.Selected(); ok {
		t.Error("table still reports a selection for a row that is gone")
	}
	m.buyClicked()
	m.sellClicked()
	if len(*cmds) != 0 {
		t.Errorf("stale selection enqueued %d commands, want 0", len(*cmds))
	}
}

// TestInventorySelectionRevalidatesOnSync: a stack that leaves the hold
// it was selected in stops gating the transfer/sell actions.
func TestInventorySelectionRevalidatesOnSync(t *testing.T) {
	env, cmds := newTestEnv(t)
	inv := NewInventory(env)
	ui := newWindowUI(t, env, inv.Win())
	frame := func() { uitest.PumpWindow(t, ui, inv.Sync, 1024, 700) }
	frame()
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-solace"})
	frame()
	clickRow(ui, inv.cargo, 0)
	frame()
	if !inv.hasSel {
		t.Fatal("fixture: selection did not take")
	}
	if inv.toHangar.GetWidget().Disabled {
		t.Fatal("fixture: To Hangar disabled for a cargo selection while docked")
	}

	// Empty the hold: the selected row disappears from its section.
	cargo, ok := env.Sim.Cargos.Get(env.Sim.Player())
	if !ok {
		t.Fatal("player has no cargo hold")
	}
	cargo.Stacks = nil
	frame()

	if inv.hasSel {
		t.Error("selection survived its stack leaving the hold")
	}
	if !inv.toHangar.GetWidget().Disabled {
		t.Error("To Hangar still enabled with no selection")
	}
	inv.transfer("hangar")
	inv.sellClicked()
	if len(*cmds) != 0 {
		t.Errorf("stale selection enqueued %d commands, want 0", len(*cmds))
	}
}

// TestFittingActions walks the fitting flow: selection gating, unfit a
// fitted slot, fit a hangar module into the freed slot and assemble an
// owned ship.
func TestFittingActions(t *testing.T) {
	env, cmds := newTestEnv(t)
	f := NewFitting(env)
	ui := newWindowUI(t, env, f.Win())
	frame := func() { uitest.PumpWindow(t, ui, f.Sync, 1024, 700) }
	frame()
	frame()

	if len(f.slots.Rows) == 0 {
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
	for i, r := range f.slots.Rows {
		pick := r.Data.(widgets.SlotPick)
		if pick.Mod != "" && fitted < 0 {
			fitted = i
		}
		if pick.Mod == "" && pick.Ref.Class == content.SlotHigh && empty < 0 {
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
	uitest.FireButton(ui, f.unfit)
	if len(*cmds) != 1 {
		t.Fatalf("unfit enqueued %d commands, want 1", len(*cmds))
	}
	if c, ok := (*cmds)[0].(sim.CmdUnfitModule); !ok || c.Slot != f.selSlot.Ref {
		t.Errorf("cmd = %#v, want CmdUnfitModule{%+v}", (*cmds)[0], f.selSlot.Ref)
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
	for i, r := range f.hangar.Rows {
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
	uitest.FireButton(ui, f.fitMod)
	if len(*cmds) != 2 {
		t.Fatalf("fit enqueued %d commands, want 2", len(*cmds))
	}
	fc, ok := (*cmds)[1].(sim.CmdFitModule)
	if !ok {
		t.Fatalf("cmd = %T, want sim.CmdFitModule", (*cmds)[1])
	}
	if fc.Item != def.ID || fc.Slot != f.selSlot.Ref {
		t.Errorf("CmdFitModule = %+v, want {%s %+v}", fc, def.ID, f.selSlot.Ref)
	}

	// Assemble an owned ship from the hangar.
	env.Sim.Hangar.Ships = append(env.Sim.Hangar.Ships,
		sim.StoredShip{Hull: "ship-sloop"})
	frame()
	shipRow := -1
	for i, r := range f.hangar.Rows {
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
	uitest.FireButton(ui, f.assemble)
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
	env := k.Env()
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

		for name, tbl := range map[string]*widgets.Table{
			"market.cargo":  market.tbl,
			"inv.cargo":     inventory.cargo,
			"inv.ore":       inventory.ore,
			"inv.hangar":    inventory.hangar,
			"fit.resists":   fitting.resists,
			"fit.slots":     fitting.slots,
			"fit.hangar":    fitting.hangar,
			"overview.rows": k.ov.tbl,
		} {
			if tbl.FitWidthForTest() <= 0 {
				t.Errorf("%dx%d: %s never fitted (fittedFor=%v)",
					size[0], size[1], name, tbl.FitWidthForTest())
			}
		}
	}
}

// TestTableSelectVsActivate drives the real deferred-event path: a
// click selects, a repeat click on the same row inside the window runs
// the row's declared double-click — Show Info, strictly, from its
// RowSpec.Ref — and a different row (its own ref) does not fire on one
// click.
func TestTableSelectVsActivate(t *testing.T) {
	env, _ := newTestEnv(t)
	tbl := widgets.NewTable(env, []widgets.Column{{Key: "A", Title: "A"}})
	var selected []int
	tbl.OnSelect(func(r widgets.TableRow) { selected = append(selected, r.Data.(int)) })
	refBravo := widgets.ModuleRef("mod-mining-laser")
	refCharlie := widgets.ModuleRef("mod-afterburner")
	tbl.SetRows([]widgets.TableRow{
		{Cells: []string{"alpha"}, Data: 0, Spec: widgets.RowSpec{Ref: widgets.PtrRef(widgets.ModuleRef("mod-emp-turret"))}},
		{Cells: []string{"bravo"}, Data: 1, Spec: widgets.RowSpec{Ref: &refBravo}},
		{Cells: []string{"charlie"}, Data: 2, Spec: widgets.RowSpec{Ref: &refCharlie}},
	})

	ui := &ebitenui.UI{Container: tbl.Container(), PrimaryTheme: env.WidgetTheme()}
	ui.Update() // registers the deferred constructor handlers

	clickRow(ui, tbl, 1)
	if !equalInts(selected, []int{1}) {
		t.Fatalf("first click: selected=%v, want [1]", selected)
	}
	if tbl.SelIdxForTest() != 1 {
		t.Errorf("selIdx = %d, want 1", tbl.SelIdxForTest())
	}
	if env.InfoWindow().IsOpen() {
		t.Fatal("a single click opened Show Info (the swap is a double-click)")
	}

	// Repeat click: the double-click half fires Show Info on the row's ref.
	clickRow(ui, tbl, 1)
	if !equalInts(selected, []int{1, 1}) {
		t.Fatalf("repeat click: selected=%v, want [1 1]", selected)
	}
	if !env.InfoWindow().IsOpen() {
		t.Fatal("repeat click did not open Show Info")
	}
	if ref, _ := env.InfoWindow().Ref(); ref != refBravo {
		t.Fatalf("Show Info ref = %v, want the repeated row's %v", ref, refBravo)
	}

	// A different row selects but does not fire the held gesture.
	clickRow(ui, tbl, 2)
	if !equalInts(selected, []int{1, 1, 2}) {
		t.Fatalf("other row: selected=%v, want [1 1 2]", selected)
	}
	if ref, _ := env.InfoWindow().Ref(); ref != refBravo {
		t.Errorf("Show Info ref after one click on another row = %v, want it held at %v", ref, refBravo)
	}
}

// TestSettingsPanelWiring pins the panel's seam: Sync renders the host's
// state (scale value, fullscreen label) and the controls call back with
// the right intents — nothing is stored panel-side, which is what lets a
// live scale change rebuild the window under itself.
func TestSettingsPanelWiring(t *testing.T) {
	env, _ := newTestEnv(t)
	var (
		scale     = 1.0
		fs        = false
		stepDelta []int
		resets    int
	)
	s := NewSettings(env, SettingsActions{
		UIScale:       func() float64 { return scale },
		StepScale:     func(d int) { stepDelta = append(stepDelta, d) },
		Fullscreen:    func() bool { return fs },
		SetFullscreen: func(on bool) { fs = on },
		ResetLayout:   func() { resets++ },
	})
	ui := newWindowUI(t, env, s.Win())
	uitest.PumpWindow(t, ui, s.Sync, 1280, 720)

	if got := s.scaleVal.Label; got != "1×" {
		t.Errorf("scale readout = %q, want %q", got, "1×")
	}
	if got := s.fsBtn.Text().Label; got != "Fullscreen: off" {
		t.Errorf("fullscreen label = %q, want off state", got)
	}

	// Buttons: the scale stepper's −/+ are the row's first/last children.
	kids := s.scaleRow.Children()
	if len(kids) != 4 {
		t.Fatalf("scale row children = %d, want 4 (− label value +)", len(kids))
	}
	uitest.FireButton(ui, kids[3].(*widget.Button)) // +
	if len(stepDelta) != 1 || stepDelta[0] != 1 {
		t.Errorf("after +: step deltas = %v, want [1]", stepDelta)
	}
	uitest.FireButton(ui, kids[0].(*widget.Button)) // −
	if len(stepDelta) != 2 || stepDelta[1] != -1 {
		t.Errorf("after −: step deltas = %v, want [1 -1]", stepDelta)
	}

	// The window's second child is the body: [scale row, fullscreen,
	// reset, hint].
	body, ok := s.win.GetContainer().Children()[1].(widget.Containerer)
	if !ok {
		t.Fatal("settings body is not a container")
	}
	bkids := body.Children()
	if len(bkids) < 4 {
		t.Fatalf("settings body children = %d, want ≥ 4", len(bkids))
	}
	uitest.FireButton(ui, bkids[1].(*widget.Button)) // fullscreen toggle (host currently off)
	fs = false
	s.Sync()
	uitest.FireButton(ui, bkids[1].(*widget.Button))
	if !fs {
		t.Error("fullscreen button did not flip the host flag")
	}
	s.Sync()
	if got := s.fsBtn.Text().Label; got != "Fullscreen: on" {
		t.Errorf("fullscreen label after click = %q, want on state", got)
	}
	uitest.FireButton(ui, bkids[2].(*widget.Button)) // reset layout
	if resets != 1 {
		t.Errorf("reset calls = %d, want 1", resets)
	}
}

// TestScaleDoublesConstructionMetrics: at 2× every construction-time
// metric doubles — window MinSize, the fallback Location, the title-bar
// height, button hit boxes, the sort-header strip — and the type scale
// doubles with it (faces are built from the scaled TypeScale).
func TestScaleDoublesConstructionMetrics(t *testing.T) {
	// Measure the design-size face and a design-size panel first (scale
	// 1), then their 2× twins.
	env1, _ := newTestEnv(t)
	w1, _ := text.Measure("Entieve", *env1.Face(theme.StepBase), 0)
	inv1 := NewInventory(env1)
	// Pump once: the buttons carry no images of their own and validate
	// against the window's theme (an unthemed PreferredSize panics).
	uitest.PumpWindow(t, newWindowUI(t, env1, inv1.Win()), nil, 1280, 720)
	bar1W, _ := inv1.qtyBar.PreferredSize()

	uitest.UseScale(t, 2)
	env, _ := newTestEnv(t)

	if env.TS().Base != 40 || env.TS().Small != 32 {
		t.Errorf("env TS at 2 = %v, want Base 40 / Small 32 (design 20/16)", env.TS())
	}
	w2, _ := text.Measure("Entieve", *env.Face(theme.StepBase), 0)
	// Glyph advances carry a sub-1/1000 px quantization, so "double"
	// holds to well under a tenth of a pixel, not bit-exactly.
	if math.Abs(w2-2*w1) > 0.05 {
		t.Errorf("measure at 2× = %v, want ≈%v (double within glyph quantization)", w2, 2*w1)
	}

	inv := NewInventory(env)
	if inv.win.MinSize == nil || inv.win.MinSize.X != 600 || inv.win.MinSize.Y != 840 {
		t.Fatalf("inventory MinSize at 2 = %v, want (600,840) (2× the design 300×420)", inv.win.MinSize)
	}
	if loc := inv.Win().GetContainer().GetWidget().Rect; loc != image.Rect(theme.Px(396), theme.Px(256), theme.Px(701), theme.Px(690)) {
		t.Errorf("fallback Location at 2 = %v, want %v", loc, image.Rect(theme.Px(396), theme.Px(256), theme.Px(701), theme.Px(690)))
	}
	if w := inv.plus.GetWidget().MinWidth; w != 80 || inv.plus.GetWidget().MinHeight != 80 {
		t.Errorf("stepper + MinSize at 2 = %d×%d, want 80×80 (40 design hit box)", w, inv.plus.GetWidget().MinHeight)
	}

	// Title bar: pump the window — the title child is exactly Px(44).
	ui := newWindowUI(t, env, inv.Win())
	uitest.PumpWindow(t, ui, nil, 1280, 720)
	kids := inv.Win().GetContainer().Children()
	h := -1
	if len(kids) > 0 {
		h = kids[0].GetWidget().Rect.Dy()
	}
	if h != theme.Px(theme.TitleBarH) {
		t.Errorf("title bar height at 2 = %d, want %d (Px(44))", h, theme.Px(theme.TitleBarH))
	}

	// Sort header strip: the table's header buttons carry MinSize(0, 40).
	tbl := widgets.NewTable(env, []widgets.Column{{Key: "A", Title: "A", Priority: 0}})
	if h := tbl.HdrBtnForTest()[0].GetWidget().MinHeight; h != theme.Px(widgets.TableHeaderH) || h != 80 {
		t.Errorf("header MinHeight at 2 = %d, want 80 (Px(design 40))", h)
	}

	// PreferredSize doubles: the stepper row's preferred width is all
	// scaled pieces (40 px floors, gaps, measured labels). Glyph
	// advances quantize per size, so "2×" holds to a couple of percent.
	bar2W, _ := inv.qtyBar.PreferredSize()
	if math.Abs(float64(bar2W)-2*float64(bar1W)) > 0.05*float64(bar2W) {
		t.Errorf("stepper row preferred width 1×=%d 2×=%d, want ≈2× (±5%%)", bar1W, bar2W)
	}
}

// TestInventoryAndMarketRowsUseMonoFace pins the wiring: the inventory
// window's three hold tables and the market window's order book draw
// their rows in the mono contents face (IBM Plex Mono), so every
// name, quantity and price lines up column-wise.
func TestInventoryAndMarketRowsUseMonoFace(t *testing.T) {
	env, _ := newTestEnv(t)
	mono := env.FaceMono(theme.StepSmall)

	inv := NewInventory(env)
	for name, tbl := range map[string]*widgets.Table{
		"inventory.cargo":  inv.cargo,
		"inventory.ore":    inv.ore,
		"inventory.hangar": inv.hangar,
	} {
		if *tbl.Face != *mono {
			t.Errorf("%s face = %v, want the mono contents face", name, *tbl.Face)
		}
	}

	m := NewMarket(env)
	if *m.tbl.Face != *mono {
		t.Errorf("market order book face = %v, want the mono contents face", *m.tbl.Face)
	}
}
