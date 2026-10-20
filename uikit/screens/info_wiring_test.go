package screens

import (
	"github.com/entieve/entieve/uikit/widgets"
	"image"
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/internal/uitest"
)

// TestMarketRowMenu: a market row's RowSpec declares Show Info plus the
// row-targeted Buy/Sell (grayed while undocked, armed once docked), and
// right-clicking opens exactly that menu — pressing Buy enqueues CmdBuy
// for the clicked row at the stepper quantity — and a double-click opens
// Show Info on the row's item (right-click itself moves no selection: the
// actions carry the row's payload).
func TestMarketRowMenu(t *testing.T) {
	env, cmds := newTestEnv(t)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	frame := func() { uitest.PumpWindow(t, ui, m.Sync, 1024, 700) }
	frame()
	frame()
	if len(m.tbl.Rows) == 0 {
		t.Fatal("fixture: empty market")
	}
	wantItem := m.tbl.Rows[0].Data.(content.ItemID)

	// Undocked: the trade verbs and the two order-window launches are
	// visible but disabled.
	rightClickRow(t, ui, m.tbl, 0)
	for _, label := range []string{"Show Info", "Buy", "Sell", "Sell on Market", "Request on Market"} {
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

	// The declared data behind that menu: the row's RowSpec carries
	// Buy/Sell plus the two order-window launches, gated from the same
	// docked state, targeting this row's item — no click-time selection
	// side effect.
	spec := m.tbl.Rows[0].Spec
	if ref, ok := env.InfoRefOf(wantItem); !ok || spec.Ref == nil || *spec.Ref != ref {
		t.Fatalf("row Ref = %v (%v), want the row item's ref for %s", spec.Ref, ok, wantItem)
	}
	wantLabels := []string{"Buy", "Sell", "Sell on Market", "Request on Market"}
	if len(spec.Actions) != len(wantLabels) {
		t.Fatalf("row actions = %+v, want %v", spec.Actions, wantLabels)
	}
	for i, want := range wantLabels {
		if got := spec.Actions[i]; got.Label != want || !got.Disabled {
			t.Errorf("action %d = %+v, want %s grayed while undocked", i, got, want)
		}
	}
	// The two launches carry the row item and the right side.
	if a := spec.Actions[2]; a.Intent != (widgets.OpenOrder{Item: wantItem, Side: sim.OrderSell, Qty: env.Sim.PlayerStock(wantItem)}) {
		t.Errorf("Sell on Market intent = %#v, want OpenOrder{sell %s}", a.Intent, wantItem)
	}
	if a := spec.Actions[3]; a.Intent != (widgets.OpenOrder{Item: wantItem, Side: sim.OrderBuy, Qty: 1}) {
		t.Errorf("Request on Market intent = %#v, want OpenOrder{buy %s qty 1}", a.Intent, wantItem)
	}

	// Docked: the same menu, now armed — data and entries agree.
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-solace"})
	frame()
	for i, a := range m.tbl.Rows[0].Spec.Actions {
		if a.Disabled {
			t.Errorf("action %d (%s) still grayed after the docked frame", i, a.Label)
		}
	}
	rightClickRow(t, ui, m.tbl, 0)
	if e := entry(t, env, "Buy"); e.Disabled {
		t.Fatal("Buy still disabled while docked")
	}
	if e := entry(t, env, "Request on Market"); e.Disabled {
		t.Fatal("Request on Market still disabled while docked")
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

// TestOverviewEntityMenu: every overview row declares the four entity
// verbs in its RowSpec (the data assertion), and that spec's menu leads
// with Show Info (disabled for entities without a def) and offers
// exactly the verbs the Selected Item window enqueues — warp/orbit at
// the selectors' values, approach and lock — gated the way those
// buttons gate.
func TestOverviewEntityMenu(t *testing.T) {
	k, cmds := newTestKit(t)
	env := k.Env()
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

	// The declared data: every built row carries the four verbs in the
	// entity menu's order (the same overviewSpec function declares them).
	wantLabels := []string{"Warp to", "Orbit", "Approach", "Lock target"}
	for _, r := range k.ov.tbl.Rows {
		if len(r.Spec.Actions) != len(wantLabels) {
			t.Fatalf("overview row %v declares %d actions, want %d (%v)",
				r.Cells, len(r.Spec.Actions), len(wantLabels), r.Spec.Actions)
		}
		for i, want := range wantLabels {
			if got := r.Spec.Actions[i].Label; got != want {
				t.Errorf("overview row %v action %d = %q, want %q", r.Cells, i, got, want)
			}
		}
		if r.Spec.Ref == nil {
			t.Errorf("overview row %v declares no Ref", r.Cells)
		}
	}
	celSpec := k.overviewSpec(&ovRow{info: celestial})
	if celSpec.Ref == nil {
		t.Fatal("celestial row declares no Ref (its Show Info entry must exist, grayed)")
	}

	env.OpenRowMenu(celSpec, image.Point{X: 100, Y: 100})
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
	shipSpec := k.overviewSpec(&ovRow{info: ship})
	env.OpenRowMenu(shipSpec, image.Point{X: 100, Y: 100})
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
	env.OpenRowMenu(shipSpec, image.Point{X: 100, Y: 100}) // the menu closed with the press
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

// TestInventoryAndFittingMenus: the inventory's rows declare Show Info
// plus the full-stack "Sell on Market" launch (grayed, never hidden, while
// undocked — orders rest on the docked station's book); a fitting slot row
// declares Show Info plus Fit/Unefit gated on that row — the menu's
// actions carry the clicked row's payload, so Unefit arms for a filled slot
// and Fit does not.
func TestInventoryAndFittingMenus(t *testing.T) {
	env, cmds := newTestEnv(t)

	inv := NewInventory(env)
	uiInv := newWindowUI(t, env, inv.Win())
	uitest.PumpWindow(t, uiInv, inv.Sync, 1024, 700)
	uitest.PumpWindow(t, uiInv, inv.Sync, 1024, 700)
	if len(inv.cargo.Rows) == 0 {
		t.Fatal("fixture: empty cargo")
	}
	cargo, hasCargo := env.Cargo(env.Sim.Player())
	stackQty := 0
	if hasCargo && len(cargo.Stacks) > 0 {
		stackQty = cargo.Stacks[0].Qty
	}
	rightClickRow(t, uiInv, inv.cargo, 0)
	ents := env.Menu().Entries()
	if len(ents) != 3 || ents[0].Label != "Show Info" || !ents[0].Bold || !ents[1].Sep {
		t.Fatalf("inventory menu = %+v, want Show Info / sep / Sell on Market", ents)
	}
	if e := entry(t, env, "Sell on Market"); !e.Disabled {
		t.Error("Sell on Market enabled while undocked")
	}
	// The declared launch carries the clicked stack's item and quantity.
	if got := inv.cargo.Rows[0].Spec.Actions[0].Intent; got != (widgets.OpenOrder{
		Item: inv.cargo.Rows[0].Data.(content.ItemID), Side: sim.OrderSell, Qty: stackQty}) {
		t.Errorf("inventory Sell on Market intent = %#v, want OpenOrder on the clicked stack (qty %d)", got, stackQty)
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
	uitest.PumpWindow(t, uiFit, f.Sync, 1024, 700)
	uitest.PumpWindow(t, uiFit, f.Sync, 1024, 700)
	fitted := -1
	for i, r := range f.slots.Rows {
		if pick, ok := r.Data.(widgets.SlotPick); ok && pick.Mod != "" {
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

// TestRowSpecMenuShape pins the derivation: the Show Info group first
// (Ref bold, SecRef as "Show Info — <name>"), one MenuSep between the
// groups, the actions after it — grayed when disabled, never hidden —
// plus the degenerate specs: actions-only opens with no leading
// separator (the contract row), ref-only is the single entry (the
// objective label / Show Info link), and an empty spec opens no menu at
// all. An unresolvable Ref keeps its entry, grayed.
func TestRowSpecMenuShape(t *testing.T) {
	env, cmds := newTestEnv(t)
	ref := widgets.ItemRef("ammo-emp-s")
	sec := widgets.ItemRef("ammo-fusion-s")
	spec := widgets.RowSpec{
		Ref:    &ref,
		SecRef: &sec,
		Actions: []widgets.RowAction{
			{Label: "Buy", Intent: widgets.Buy{Item: "ammo-emp-s", Qty: 1}},
			{Label: "Sell", Disabled: true, Intent: widgets.Sell{Item: "ammo-emp-s", Qty: 1}},
		},
	}

	ents := widgets.RowMenuEntries(env, env, spec)
	if len(ents) != 5 {
		t.Fatalf("entries = %+v, want Show Info / secondary / sep / Buy / Sell", ents)
	}
	if ents[0].Label != "Show Info" || !ents[0].Bold || ents[0].Disabled {
		t.Errorf("entry 0 = %+v, want a bold, enabled Show Info", ents[0])
	}
	wantSecond := "Show Info — " + widgets.ChainName(env, "ammo-fusion-s")
	if ents[1].Label != wantSecond || ents[1].Bold {
		t.Errorf("entry 1 = %+v, want the non-bold secondary %q", ents[1], wantSecond)
	}
	if !ents[2].Sep {
		t.Errorf("entry 2 = %+v, want the separator between the info and action groups", ents[2])
	}
	if ents[3].Label != "Buy" || ents[3].Disabled {
		t.Errorf("entry 3 = %+v, want an enabled Buy", ents[3])
	}
	if ents[4].Label != "Sell" || !ents[4].Disabled {
		t.Errorf("entry 4 = %+v, want Sell grayed (visible, not hidden)", ents[4])
	}

	// Dispatch through the seam: a disabled entry closes without emitting,
	// an enabled one emits its intent — here through the session's own
	// sink, which maps it onto the command the trade button fires.
	env.OpenRowMenu(spec, image.Pt(50, 60))
	if !env.Menu().IsOpen() || env.Menu().Rect().Min != (image.Pt(50, 60)) {
		t.Fatalf("menu open=%v origin=%v, want open at the click point",
			env.Menu().IsOpen(), env.Menu().Rect().Min)
	}
	clickMenuEntry(t, env, "Sell")
	if len(*cmds) != 0 {
		t.Fatalf("disabled Sell emitted %v, want nothing", *cmds)
	}
	env.OpenRowMenu(spec, image.Pt(50, 60))
	clickMenuEntry(t, env, "Buy")
	if len(*cmds) != 1 {
		t.Fatalf("enabled Buy emitted %v, want the single CmdBuy", *cmds)
	}
	if c, ok := (*cmds)[0].(sim.CmdBuy); !ok || c.Item != "ammo-emp-s" || c.Qty != 1 {
		t.Fatalf("cmd = %#v, want CmdBuy{ammo-emp-s 1}", (*cmds)[0])
	}

	// Actions only: the single Accept entry, no leading separator.
	actOnly := widgets.RowMenuEntries(env, env, widgets.RowSpec{Actions: []widgets.RowAction{{Label: "Accept"}}})
	if len(actOnly) != 1 || actOnly[0].Label != "Accept" || actOnly[0].Sep {
		t.Errorf("actions-only = %+v, want the lone Accept entry", actOnly)
	}
	// Ref only: the single bold Show Info entry.
	refOnly := widgets.RowMenuEntries(env, env, widgets.RowSpec{Ref: widgets.PtrRef(widgets.ModuleRef("mod-autocannon-s"))})
	if len(refOnly) != 1 || refOnly[0].Label != "Show Info" || !refOnly[0].Bold {
		t.Errorf("ref-only = %+v, want the lone bold Show Info", refOnly)
	}
	// Empty spec: no popup at all.
	env.Menu().Close()
	env.OpenRowMenu(widgets.RowSpec{}, image.Pt(0, 0))
	if env.Menu().IsOpen() {
		t.Error("an empty spec opened a menu")
	}
	// Unresolvable Ref: the entry stays (grayed), and pressing it opens
	// nothing.
	bad := widgets.InfoRef{Kind: widgets.InfoItem, ID: "item-nowhere"}
	gone := widgets.RowSpec{Ref: &bad}
	entries := widgets.RowMenuEntries(env, env, gone)
	if len(entries) != 1 || entries[0].Label != "Show Info" || !entries[0].Bold || !entries[0].Disabled {
		t.Fatalf("unresolvable spec = %+v, want one grayed Show Info", entries)
	}
	env.OpenRowMenu(gone, image.Pt(10, 10))
	clickMenuEntry(t, env, "Show Info")
	if env.InfoWindow().IsOpen() {
		t.Error("a grayed Show Info opened the window")
	}
}

// TestRowSpecDoubleClickStrict pins the double-click half: one click is
// bookkeeping, a repeat inside the window opens Show Info on the row's
// Ref and fires NOTHING else (no commands — Show Info is the whole
// gesture), and a row whose Ref does not resolve never fires.
func TestRowSpecDoubleClickStrict(t *testing.T) {
	env, cmds := newTestEnv(t)
	ref := widgets.ModuleRef("mod-autocannon-s")
	bad := widgets.InfoRef{Kind: widgets.InfoItem, ID: "item-nowhere"}
	tbl := widgets.NewTable(env, []widgets.Column{{Key: "A", Title: "A"}})
	tbl.SetRows([]widgets.TableRow{
		{Cells: []string{"gun"}, Data: 0, Spec: widgets.RowSpec{Ref: &ref}},
		{Cells: []string{"ghost"}, Data: 1, Spec: widgets.RowSpec{Ref: &bad}},
	})
	ui := &ebitenui.UI{Container: tbl.Container(), PrimaryTheme: env.WidgetTheme()}
	ui.Update()

	clickRow(ui, tbl, 0)
	if env.InfoWindow().IsOpen() {
		t.Fatal("a single click opened Show Info")
	}
	clickRow(ui, tbl, 0)
	if !env.InfoWindow().IsOpen() {
		t.Fatal("repeat click did not open Show Info")
	}
	if got, _ := env.InfoWindow().Ref(); got != ref {
		t.Errorf("Show Info ref = %v, want the row's %v", got, ref)
	}
	if len(*cmds) != 0 {
		t.Fatalf("double-click fired %d commands, want Show Info only", len(*cmds))
	}

	env.InfoWindow().Close()
	clickRow(ui, tbl, 1)
	clickRow(ui, tbl, 1)
	if env.InfoWindow().IsOpen() {
		t.Error("a row with an unresolvable Ref fired Show Info")
	}
	if len(*cmds) != 0 {
		t.Errorf("unresolvable double-click fired %d commands, want 0", len(*cmds))
	}
}
