package screens

import (
	"fmt"

	"github.com/entieve/entieve/internal/uitest"
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
// undocked, and Buy/Sell open the quantity picker on the selected order —
// Confirm there enqueues the trade at the picked quantity.
func TestMarketGatingAndEnqueue(t *testing.T) {
	env, cmds := newTestEnv(t)
	m := NewMarket(env)
	ui := newQtyUI(t, env, m.Win())
	q := env.QtyWindow()
	frame := func() { pumpQty(t, ui, env, m.Sync, 1024, 700) }
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
	if len(*cmds) != 0 || q.IsOpen() {
		t.Fatalf("undocked trade enqueued %d commands / opened the picker (%v), want 0 / false", len(*cmds), q.IsOpen())
	}

	// Docked: Sell unlocks; Buy needs a row whose local book holds an ask
	// to lift (no selection yet, so it stays greyed).
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-haven-rest"})
	frame()
	if !m.buy.GetWidget().Disabled {
		t.Fatal("Buy enabled with no item selected")
	}
	if m.sell.GetWidget().Disabled {
		t.Fatal("Sell still disabled while docked")
	}
	ask := map[content.ItemID]bool{}
	for _, o := range env.Sim.BookOrders("cel-haven-rest") {
		if o.Side == sim.OrderSell {
			ask[o.Item] = true
		}
	}
	pick := -1
	for i, r := range m.tbl.Rows {
		if row, ok := r.Data.(mktItemRow); ok && ask[row.Item] {
			pick = i
			break
		}
	}
	if pick < 0 {
		t.Fatal("fixture: the docked book holds no ask")
	}

	// Select that item, pick 10 on the picker's slider and confirm.
	clickRow(ui, m.tbl, pick)
	frame()
	if m.buy.GetWidget().Disabled {
		t.Fatal("Buy still disabled on a row with a local ask")
	}
	uitest.FireButton(ui, m.buy)
	frame()
	if !q.IsOpen() {
		t.Fatal("Buy opened no picker")
	}
	if got := q.TitleText(); got != "Quantity" {
		t.Errorf("picker title = %q, want the base name Quantity", got)
	}
	qtyClick(t, ui, env, m.Sync, qtyMax)
	if q.Value() != qtyMax {
		t.Fatalf("qty = %d, want the slider's top %d", q.Value(), qtyMax)
	}
	uitest.FireButton(ui, q.Confirm())
	if len(*cmds) != 1 {
		t.Fatalf("buy enqueued %d commands, want 1", len(*cmds))
	}
	buy, ok := (*cmds)[0].(sim.CmdBuy)
	if !ok {
		t.Fatalf("cmd = %T, want sim.CmdBuy", (*cmds)[0])
	}
	// The table sorts by name, so the selected row — not the sim's
	// canonical stock order — is the trade target.
	wantItem := m.tbl.Rows[pick].Data.(mktItemRow).Item
	if buy.Item != wantItem || buy.Qty != qtyMax {
		t.Errorf("CmdBuy = %+v, want {%s %d}", buy, wantItem, qtyMax)
	}

	// Sell through the same picker (its own remembered quantity: 1 so far).
	uitest.FireButton(ui, m.sell)
	frame()
	if !q.IsOpen() {
		t.Fatal("Sell opened no picker")
	}
	if got := q.Value(); got != 1 {
		t.Errorf("sell picker opened at %d, want its unseeded 1", got)
	}
	uitest.FireButton(ui, q.Confirm())
	if len(*cmds) != 2 {
		t.Fatalf("sell enqueued %d commands total, want 2", len(*cmds))
	}
	sell, ok := (*cmds)[1].(sim.CmdSell)
	if !ok || sell.Item != wantItem || sell.Qty != 1 {
		t.Errorf("cmd = %#v, want CmdSell{%s 1}", (*cmds)[1], wantItem)
	}
}

// TestInventoryTransferAndSell: section selection drives the transfer and
// sell verbs, each opening the quantity picker with its destination
// pre-set (both gated by the hold's own rules: cargo present, ore hold
// fitted, hangar side and selling docked).
func TestInventoryTransferAndSell(t *testing.T) {
	env, cmds := newTestEnv(t)
	inv := NewInventory(env)
	ui := newQtyUI(t, env, inv.Win())
	q := env.QtyWindow()
	frame := func() { pumpQty(t, ui, env, inv.Sync, 1024, 700) }
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
	if len(*cmds) != 0 || q.IsOpen() {
		t.Fatalf("gated actions enqueued %d commands / opened the picker, want none", len(*cmds))
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
	frame()
	if !q.IsOpen() {
		t.Fatal("To Hangar opened no picker")
	}
	if got := q.TitleText(); got != "Quantity" {
		t.Errorf("picker title = %q, want the base name Quantity", got)
	}
	if q.Confirm().GetWidget().Disabled {
		t.Fatal("Confirm grayed on a live transfer")
	}
	uitest.FireButton(ui, q.Confirm())
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
	frame()
	if !q.IsOpen() {
		t.Fatal("Sell opened no picker")
	}
	uitest.FireButton(ui, q.Confirm())
	if len(*cmds) != 2 {
		t.Fatalf("sell enqueued %d commands total, want 2", len(*cmds))
	}
	sl, ok := (*cmds)[1].(sim.CmdSell)
	if !ok || sl.Item != inv.sel || sl.Qty != 1 {
		t.Errorf("cmd = %#v, want CmdSell{%s 1}", (*cmds)[1], inv.sel)
	}
}

// TestMarketSelectionRevalidatesOnSync: a selected order row that leaves
// the board stops reading as selected — the click-time cache alone would
// keep Buy/Sell armed on an order the market no longer offers (its item's
// own row stays: an orderless item is still listed).
func TestMarketSelectionRevalidatesOnSync(t *testing.T) {
	env, cmds := newTestEnv(t)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	frame := func() { uitest.PumpWindow(t, ui, m.Sync, 1024, 700) }
	frame()
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-solace"})
	frame()
	// Row 0 is the first catalog item's parent, row 1 its first order.
	clickRow(ui, m.tbl, 0)
	frame()
	if _, isOrder := m.tbl.Rows[1].Data.(mktOrderRow); !isOrder {
		t.Fatalf("row 1 payload = %#v, want an order row (the fixture book is empty)", m.tbl.Rows[1].Data)
	}
	clickRow(ui, m.tbl, 1)
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

	if !fitTileShown(f.tiles[0][0]) {
		t.Fatal("slot grid is empty")
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
	for idx, tile := range f.tiles[0] {
		if !fitTileShown(tile) {
			continue
		}
		if tile.mod != "" && fitted < 0 {
			fitted = idx
		}
		if tile.mod == "" && empty < 0 {
			empty = idx
		}
	}
	if fitted < 0 || empty < 0 {
		t.Fatalf("expected a fitted and an empty high slot (fitted=%d empty=%d)", fitted, empty)
	}

	// Unfit the selected fitted slot.
	clickFitTile(ui, f.tiles[0][fitted])
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
	def, ok := env.Content.Module("mod-artillery-s")
	if !ok || def.Slot != content.SlotHigh {
		t.Fatalf("fixture: mod-artillery-s slot = %q (ok=%v), want a high module", def.Slot, ok)
	}
	env.Sim.Hangar.Items = append(env.Sim.Hangar.Items,
		content.CargoStack{Item: "mod-artillery-s", Qty: 1})
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
	clickFitTile(ui, f.tiles[0][empty])
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
		{Cells: []string{"alpha"}, Data: 0, Spec: widgets.RowSpec{Ref: widgets.PtrRef(widgets.ModuleRef("mod-autocannon-s"))}},
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
// state (scale value, fullscreen and rail labels) and the controls call
// back with the right intents — nothing is stored panel-side, which is
// what lets a live scale change rebuild the window under itself.
func TestSettingsPanelWiring(t *testing.T) {
	env, _ := newTestEnv(t)
	var (
		scale     = 1.0
		fs        = false
		rail      = false
		master    = 100
		music     = 70
		uiVol     = 80
		shuffle   = true
		scaleHits []float64
		setMaster int
		persists  int
		resets    int
		resetAlls int
	)
	act := SettingsActions{
		UIScale:          func() float64 { return scale },
		UIScaleLadder:    []float64{0.75, 1.0, 1.25, 1.5, 2.0, 2.5},
		SetUIScale:       func(r float64) { scale = r; scaleHits = append(scaleHits, r) },
		Fullscreen:       func() bool { return fs },
		SetFullscreen:    func(on bool) { fs = on },
		RailCollapsed:    func() bool { return rail },
		SetRailCollapsed: func(on bool) { rail = on },
		CloseHint:        func() string { return "P" },
		MasterVolume:     func() int { return master },
		SetMasterVolume:  func(v int) { master = v; setMaster++ },
		MusicVolume:      func() int { return music },
		SetMusicVolume:   func(v int) { music = v },
		UIVolume:         func() int { return uiVol },
		SetUIVolume:      func(v int) { uiVol = v },
		PersistSettings:  func() { persists++ },
		Shuffle:          func() bool { return shuffle },
		SetShuffle:       func(on bool) { shuffle = on },
		ResetLayout:      func() { resets++ },
		ResetAll:         func() { resetAlls++ },
	}
	s := NewSettings(env, act)
	ui := newWindowUI(t, env, s.Win())
	uitest.PumpWindow(t, ui, s.Sync, 1280, 720)

	if got := s.scaleVal.Label; got != "1×" {
		t.Errorf("scale readout = %q, want %q", got, "1×")
	}
	if got := s.fsBtn.Text().Label; got != "Fullscreen: off" {
		t.Errorf("fullscreen label = %q, want off state", got)
	}
	if got := s.railBtn.Text().Label; got != "Rail: expanded" {
		t.Errorf("rail label = %q, want expanded state", got)
	}
	if got := s.masterVal.Label; got != "100%" {
		t.Errorf("master readout = %q, want 100%%", got)
	}
	if got := s.musicVal.Label; got != "70%" {
		t.Errorf("music readout = %q, want 70%%", got)
	}
	if got := s.shufBtn.Text().Label; got != "Shuffle: on" {
		t.Errorf("shuffle label = %q, want on state", got)
	}

	// The three numeric rows are [label, slider, value].
	for name, row := range map[string]*widget.Container{"scale": s.scaleRow, "master": s.masterRow, "music": s.musicRow} {
		if kids := row.Children(); len(kids) != 3 {
			t.Errorf("%s row children = %d, want 3 (label, slider, value)", name, len(kids))
		}
	}

	// Drive the sliders through their production input path — Poll reads
	// the session's pointer frame — so Sync and the grab interact exactly
	// as they do in a frame.
	frame := func(p image.Point, down bool) {
		env.in.x, env.in.y = p.X, p.Y
		env.in.edge = [3]bool{down && !env.in.down[0], false, false}
		env.in.down[0] = down
		env.Pointer().Begin()
		s.Sync()
		env.in.edge = [3]bool{}
	}

	// The scale slider is notched: clicking a rung applies it absolutely
	// and the readout follows it.
	if got := len(s.scaleSlider.TickPositions()); got != 6 {
		t.Errorf("scale ticks = %d, want the six rungs", got)
	}
	frame(sliderPoint(s.scaleSlider, 1.5), true)
	frame(sliderPoint(s.scaleSlider, 1.5), false)
	if len(scaleHits) != 1 || scaleHits[0] != 1.5 || scale != 1.5 {
		t.Fatalf("scale slider click = %v (host %v), want one absolute 1.5", scaleHits, scale)
	}
	s.Sync()
	if got := s.scaleVal.Label; got != "1.5×" {
		t.Errorf("scale readout after click = %q, want 1.5×", got)
	}

	// A volume drag streams the live Set and writes the sidecar ONCE, on
	// release — N changes, one persist, none mid-drag.
	masterBefore := setMaster
	frame(sliderPoint(s.masterSlide, 40), true)
	for _, v := range []float64{45, 50, 55} {
		frame(sliderPoint(s.masterSlide, v), true)
	}
	if got, want := s.masterVal.Label, fmt.Sprintf("%d%%", int(math.Round(s.masterSlide.Value()))); got != want {
		t.Errorf("master readout mid-drag = %q, want %q (the handle's value)", got, want)
	}
	if persists != 0 {
		t.Errorf("sidecar writes mid-drag = %d, want 0", persists)
	}
	if master != int(math.Round(s.masterSlide.Value())) {
		t.Errorf("host master = %d, want the dragged handle value %v", master, s.masterSlide.Value())
	}
	if setMaster-masterBefore < 3 {
		t.Errorf("live master sets = %d, want one per moved value (≥3)", setMaster-masterBefore)
	}
	frame(sliderPoint(s.masterSlide, 55), false)
	if persists != 1 {
		t.Errorf("sidecar writes after release = %d, want exactly 1", persists)
	}

	// Music is the same freeflow control on its own value.
	frame(sliderPoint(s.musicSlide, 30), true)
	frame(sliderPoint(s.musicSlide, 30), false)
	if music != int(math.Round(s.musicSlide.Value())) {
		t.Errorf("music after click = %d, want %v", music, s.musicSlide.Value())
	}
	s.Sync()
	if got, want := s.musicVal.Label, fmt.Sprintf("%d%%", music); got != want {
		t.Errorf("music readout after click = %q, want %q", got, want)
	}
	if persists != 2 {
		t.Errorf("music click persists = %d, want 2 (one per grab)", persists)
	}

	// UI is the third volume: the same freeflow control on the UI cue
	// channel (master × UI is the host's math; the row only routes).
	frame(sliderPoint(s.uiSlide, 35), true)
	frame(sliderPoint(s.uiSlide, 35), false)
	if uiVol != int(math.Round(s.uiSlide.Value())) {
		t.Errorf("UI volume after click = %d, want %v", uiVol, s.uiSlide.Value())
	}
	s.Sync()
	if got, want := s.uiVal.Label, fmt.Sprintf("%d%%", uiVol); got != want {
		t.Errorf("UI readout after click = %q, want %q", got, want)
	}
	if persists != 3 {
		t.Errorf("UI click persists = %d, want 3 (one per grab)", persists)
	}

	// The window's second child is the body: [scale row, fullscreen,
	// rail, master, music, UI, shuffle, reset layout, reset all, hint].
	body, ok := s.win.GetContainer().Children()[1].(widget.Containerer)
	if !ok {
		t.Fatal("settings body is not a container")
	}
	bkids := body.Children()
	if len(bkids) != 10 {
		t.Fatalf("settings body children = %d, want 10 (scale, fullscreen, rail, master, music, UI, shuffle, reset, reset all, hint)", len(bkids))
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
	uitest.FireButton(ui, bkids[2].(*widget.Button)) // rail toggle
	if !rail {
		t.Error("rail button did not flip the host flag")
	}
	s.Sync()
	if got := s.railBtn.Text().Label; got != "Rail: collapsed" {
		t.Errorf("rail label after click = %q, want collapsed state", got)
	}
	uitest.FireButton(ui, bkids[6].(*widget.Button)) // shuffle toggle (row 6 since the UI volume row landed at 5)
	if shuffle {
		t.Error("shuffle button did not flip the host flag")
	}
	s.Sync()
	if got := s.shufBtn.Text().Label; got != "Shuffle: off" {
		t.Errorf("shuffle label after click = %q, want off state", got)
	}
	uitest.FireButton(ui, bkids[7].(*widget.Button)) // reset layout (row 7: the UI row shifted the resets down)
	if resets != 1 {
		t.Errorf("reset calls = %d, want 1", resets)
	}
	// Reset all is two-step: the first press only arms (label swap), the
	// second fires exactly once and restores the label.
	uitest.FireButton(ui, bkids[8].(*widget.Button))
	if resetAlls != 0 {
		t.Fatalf("armed reset-all fired on the first press")
	}
	if got := s.resetAll.Text().Label; got != "Confirm reset all" {
		t.Errorf("armed reset-all label = %q, want the confirm state", got)
	}
	uitest.FireButton(ui, bkids[8].(*widget.Button))
	if resetAlls != 1 {
		t.Errorf("reset-all calls = %d, want 1", resetAlls)
	}
	if got := s.resetAll.Text().Label; got != "Reset all settings" {
		t.Errorf("reset-all label after confirm = %q, want the idle label", got)
	}
	// A rebuild under a drag (the host replaces this panel on a scale
	// change) leaves the replacement slider idle: a held pointer with no
	// fresh press cannot fire a stale change.
	s2 := NewSettings(env, act)
	uitest.PumpWindow(t, ui, s2.Sync, 1280, 720)
	if s2.scaleSlider.Grabbed() {
		t.Fatal("a fresh slider starts grabbed")
	}
	before := len(scaleHits)
	s2.Sync() // held pointer, no fresh press: the replacement slider stays idle
	if len(scaleHits) != before {
		t.Errorf("a held frame with no press changed the scale (%v)", scaleHits[before:])
	}
}

// clickSlider presses and releases the slider at value v's x.
func clickSlider(sl *widgets.Slider, v float64) {
	p := sliderPoint(sl, v)
	sl.Step(p, true, true)
	sl.Step(p, false, false)
}

// sliderPoint is the band's centre at value v's x.
func sliderPoint(sl *widgets.Slider, v float64) image.Point {
	b := sl.HitBand()
	return image.Pt(sl.ValueX(v), b.Min.Y+b.Dy()/2)
}

// TestSettingsRowsFitAtMinScale: at the shrunken 0.75 rung the nine rows
// and the hint still stack inside the window without overlap — the window
// grows with its content, and every row keeps its scaled 30 px floor.
func TestSettingsRowsFitAtMinScale(t *testing.T) {
	uitest.UseScale(t, 0.75)
	env, _ := newTestEnv(t)
	s := NewSettings(env, SettingsActions{
		UIScale:          func() float64 { return 1 },
		UIScaleLadder:    []float64{0.75, 1.0, 1.25, 1.5, 2.0, 2.5},
		SetUIScale:       func(float64) {},
		Fullscreen:       func() bool { return false },
		SetFullscreen:    func(bool) {},
		RailCollapsed:    func() bool { return false },
		SetRailCollapsed: func(bool) {},
		CloseHint:        func() string { return "P" },
		MasterVolume:     func() int { return 100 },
		SetMasterVolume:  func(int) {},
		MusicVolume:      func() int { return 70 },
		SetMusicVolume:   func(int) {},
		UIVolume:         func() int { return 80 },
		SetUIVolume:      func(int) {},
		PersistSettings:  func() {},
		Shuffle:          func() bool { return true },
		SetShuffle:       func(bool) {},
		ResetLayout:      func() {},
		ResetAll:         func() {},
	})
	ui := newWindowUI(t, env, s.Win())
	uitest.PumpWindow(t, ui, s.Sync, 1280, 720)

	body := s.win.Contents.(widget.Containerer)
	kids := body.Children()
	if len(kids) != 10 {
		t.Fatalf("settings body children = %d, want 10 (stepper, fullscreen, rail, master, music, UI, shuffle, reset, reset all, hint)", len(kids))
	}
	prev := image.Rectangle{}
	for i, k := range kids {
		r := k.GetWidget().Rect
		if r.Dy() <= 0 {
			t.Fatalf("row %d has no height: %v", i, r)
		}
		if i > 0 && r.Min.Y < prev.Max.Y {
			t.Errorf("row %d overlaps the one above: %v vs %v", i, r, prev)
		}
		prev = r
	}
	if body := body.GetWidget().Rect; prev.Max.Y > body.Max.Y || prev.Max.X > body.Max.X {
		t.Errorf("settings body %v clips its last row at %v", body, prev)
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
	bar1W := inv1.sell.GetWidget().Rect.Dx()

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
	if inv.win.MinSize == nil || inv.win.MinSize.X != 640 || inv.win.MinSize.Y != 840 {
		t.Fatalf("inventory MinSize at 2 = %v, want (640,840) (2× the design 320×420)", inv.win.MinSize)
	}
	if loc := inv.Win().GetContainer().GetWidget().Rect; loc != image.Rect(theme.Px(396), theme.Px(256), theme.Px(716), theme.Px(690)) {
		t.Errorf("fallback Location at 2 = %v, want %v", loc, image.Rect(theme.Px(396), theme.Px(256), theme.Px(716), theme.Px(690)))
	}
	if w := inv.sell.GetWidget().MinWidth; w != 80 || inv.sell.GetWidget().MinHeight != 80 {
		t.Errorf("toolbar button MinSize at 2 = %d×%d, want 80×80 (40 design hit box)", w, inv.sell.GetWidget().MinHeight)
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

	// PreferredSize doubles: the toolbar row's preferred width is all
	// scaled pieces (40 px floors, gaps, measured labels). Glyph
	// advances quantize per size, so "2×" holds to a couple of percent.
	bar2W := inv.sell.GetWidget().Rect.Dx()
	if math.Abs(float64(bar2W)-2*float64(bar1W)) > 0.05*float64(bar2W) {
		t.Errorf("toolbar button width 1×=%d 2×=%d, want ≈2× (±5%%)", bar1W, bar2W)
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

// staticSettingsActions is a harmless non-nil action set for construction
// tests that only measure the window (the Settings contract requires
// every callback).
func staticSettingsActions() SettingsActions {
	return SettingsActions{
		UIScale:          func() float64 { return 1 },
		UIScaleLadder:    []float64{0.75, 1.0, 1.25, 1.5, 2.0, 2.5},
		SetUIScale:       func(float64) {},
		Fullscreen:       func() bool { return false },
		SetFullscreen:    func(bool) {},
		RailCollapsed:    func() bool { return false },
		SetRailCollapsed: func(bool) {},
		CloseHint:        func() string { return "P" },
		MasterVolume:     func() int { return 100 },
		SetMasterVolume:  func(int) {},
		MusicVolume:      func() int { return 70 },
		SetMusicVolume:   func(int) {},
		UIVolume:         func() int { return 80 },
		SetUIVolume:      func(int) {},
		PersistSettings:  func() {},
		Shuffle:          func() bool { return true },
		SetShuffle:       func(bool) {},
		ResetLayout:      func() {},
		ResetAll:         func() {},
	}
}
