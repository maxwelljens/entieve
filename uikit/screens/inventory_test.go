package screens

import (
	"fmt"
	"image"
	"math"
	"sort"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// dragStack drags one section's stack (row i in list view) to the middle of
// another hold's section and pumps the release: press, move, release, one
// frame each. It leaves the picker open when the drop is valid.
func dragStack(t *testing.T, ui *ebitenui.UI, env *testEnv, inv *Inventory, from string, row int, to string) {
	t.Helper()
	pump := func() {
		env.Pointer().Begin()
		pumpQty(t, ui, env, inv.Sync, 1024, 700)
	}
	fromRect := inv.tableFor(from).RowWidgetForTest(row).Rect
	toRect := map[string]*widget.Container{
		"cargo": inv.cargoBody, "ore": inv.oreBody, "hangar": inv.hangBody,
	}[to].GetWidget().Rect
	at := image.Pt(fromRect.Min.X+fromRect.Dx()/2, fromRect.Min.Y+fromRect.Dy()/2)
	over := image.Pt(toRect.Min.X+toRect.Dx()/2, toRect.Min.Y+toRect.Dy()/2)

	env.in.x, env.in.y = at.X, at.Y
	env.in.edge, env.in.down = [3]bool{true, false, false}, [3]bool{true, false, false}
	pump()
	env.in.x, env.in.y = over.X, over.Y
	env.in.edge = [3]bool{}
	pump()
	env.in.down = [3]bool{}
	pump()
}

// clickStack is one full selection GESTURE on a list-view stack row: the
// press half runs through the production frame with mods held (the press
// applies the Selection set's Shift/Ctrl semantics), then the release
// half, in the game's own order (the pointer release frame's Sync first,
// the row's click event after it). The click half is fired by hand
// because a headless pump's ebitenui input reads the OS cursor, not the
// script: clickRow IS the release production would deliver.
func clickStack(t *testing.T, ui *ebitenui.UI, env *testEnv, inv *Inventory, hold string, row int, mods widgets.Mods) {
	t.Helper()
	r := inv.tableFor(hold).RowWidgetForTest(row).Rect
	at := image.Pt(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)
	env.in.x, env.in.y = at.X, at.Y
	env.in.mods = mods
	env.in.edge, env.in.down = [3]bool{true}, [3]bool{true}
	env.Pointer().Begin()
	pumpQty(t, ui, env, inv.Sync, 1024, 700) // the press half: the frame applies mods

	env.in.edge, env.in.down = [3]bool{}, [3]bool{}
	env.Pointer().Begin()
	pumpQty(t, ui, env, inv.Sync, 1024, 700) // the pointer release (drag ends, no drop)
	clickRow(ui, inv.tableFor(hold), row)    // the release's click half
	pumpQty(t, ui, env, inv.Sync, 1024, 700)
	env.in.mods = 0
}

// clickTile is clickStack for the thumbnail view: tiles declare no click
// handler (the press is the whole gesture), so the frame alone decides.
func clickTile(t *testing.T, ui *ebitenui.UI, env *testEnv, inv *Inventory, hold string, row int, mods widgets.Mods) {
	t.Helper()
	r := inv.tilesFor(hold).tiles[row].GetWidget().Rect
	at := image.Pt(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)
	env.in.x, env.in.y = at.X, at.Y
	env.in.mods = mods
	env.in.edge, env.in.down = [3]bool{true}, [3]bool{true}
	env.Pointer().Begin()
	pumpQty(t, ui, env, inv.Sync, 1024, 700)
	env.in.edge = [3]bool{}
	env.Pointer().Begin()
	pumpQty(t, ui, env, inv.Sync, 1024, 700)
	env.in.down = [3]bool{}
	env.Pointer().Begin()
	pumpQty(t, ui, env, inv.Sync, 1024, 700)
	env.in.mods = 0
}

// setIDs is the selection set as sorted ids (order-independent asserts).
func setIDs(inv *Inventory) []content.ItemID {
	out := make([]content.ItemID, 0, len(inv.selSet))
	for id := range inv.selSet {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// wantSet fails unless the set holds exactly ids.
func wantSet(t *testing.T, inv *Inventory, want ...content.ItemID) {
	t.Helper()
	got := setIDs(inv)
	sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
	if len(got) != len(want) {
		t.Fatalf("selection set = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("selection set = %v, want %v", got, want)
		}
	}
}

// withOreHold fits the showcase sloop with an ore hold (and one ore
// stack), the fixture the section-present pins drive.
func withOreHold(env *testEnv, cap float64) {
	cargo, ok := env.Sim.Cargos.Get(env.Sim.Player())
	if !ok {
		return
	}
	cargo.OreCapacity = cap
	cargo.Ore = []content.CargoStack{{Item: "ore-cordite", Qty: 7}}
}

// TestInventoryViewToggleSwitchesEverySection: the toolbar toggle moves
// the WHOLE inventory between the three tables and three tile grids, and
// back; N stacks make N tiles, each carrying the item's icon and name.
func TestInventoryViewToggleSwitchesEverySection(t *testing.T) {
	env, _ := newTestEnv(t)
	withOreHold(env, 20)
	dockAtStation(t, env.Sim)
	inv := NewInventory(env)
	ui := newWindowUI(t, env, inv.Win())
	frame := func() { uitest.PumpWindow(t, ui, inv.Sync, 1024, 700) }
	frame()
	frame()

	if !inv.oreShown || len(inv.Splits()) != 2 {
		t.Fatalf("ore section absent with an ore hold fitted (shown=%v, seams=%d)", inv.oreShown, len(inv.Splits()))
	}
	if inv.ViewTiles() {
		t.Fatal("the inventory opened in tile view, want the list default")
	}
	for _, c := range []struct {
		hold string
		body *widget.Container
	}{
		{"cargo", inv.cargoBody}, {"ore", inv.oreBody}, {"hangar", inv.hangBody},
	} {
		if got := c.body.Children()[0]; !sameWidget(got, inv.tableFor(c.hold).Container()) {
			t.Errorf("%s: body child = %T, want the table (list view)", c.hold, got)
		}
	}

	uitest.FireButton(ui, inv.view)
	frame()
	frame()
	if !inv.ViewTiles() {
		t.Fatal("the toggle did not switch to tiles")
	}
	for _, c := range []struct {
		hold   string
		body   *widget.Container
		stacks int
	}{
		{"cargo", inv.cargoBody, len(inv.cv.Holds().Cargo())},
		{"ore", inv.oreBody, len(inv.cv.Holds().Ore())},
		{"hangar", inv.hangBody, len(inv.cv.Hangar().Items)},
	} {
		grid := inv.tilesFor(c.hold)
		if got := c.body.Children()[0]; !sameWidget(got, grid.Container()) {
			t.Errorf("%s: body child = %T, want the tile grid", c.hold, got)
			continue
		}
		if len(grid.tiles) != c.stacks {
			t.Errorf("%s: %d tiles for %d stacks", c.hold, len(grid.tiles), c.stacks)
			continue
		}
		// Every tile is an icon + name + count, and every name is the
		// item's display name.
		for i, tile := range grid.tiles {
			kids := tile.Children()
			if len(kids) != 3 {
				t.Fatalf("%s tile %d: %d children, want icon/name/count", c.hold, i, len(kids))
			}
			if _, ok := kids[0].(*widget.Graphic); !ok {
				t.Errorf("%s tile %d: first child = %T, want the icon", c.hold, i, kids[0])
			}
			name, ok := kids[1].(*widget.Text)
			if !ok || name.Label == "" {
				t.Errorf("%s tile %d: name = %+v, want a label", c.hold, i, kids[1])
			}
			r := tile.GetWidget().Rect
			if r.Dx() != theme.Px(tileW) || r.Dy() < theme.Px(tileH) {
				t.Errorf("%s tile %d: %v, want the %d-wide box at least %d tall", c.hold, i, r, theme.Px(tileW), theme.Px(tileH))
			}
			if i > 0 {
				prev := grid.tiles[i-1].GetWidget().Rect
				if r.Min.Y == prev.Min.Y && r.Min.X <= prev.Min.X {
					t.Errorf("%s tile %d wraps backwards: %v after %v", c.hold, i, r, prev)
				}
				if r.Min.Y != prev.Min.Y && r.Min.X != grid.tiles[0].GetWidget().Rect.Min.X {
					t.Errorf("%s tile %d starts row at x=%d, want the column origin", c.hold, i, r.Min.X)
				}
			}
		}
	}

	// And back: the tables return, whole inventory.
	uitest.FireButton(ui, inv.view)
	frame()
	if inv.ViewTiles() {
		t.Fatal("the toggle did not switch back to the list")
	}
	for _, c := range []struct {
		hold string
		body *widget.Container
	}{
		{"cargo", inv.cargoBody}, {"ore", inv.oreBody}, {"hangar", inv.hangBody},
	} {
		if got := c.body.Children()[0]; !sameWidget(got, inv.tableFor(c.hold).Container()) {
			t.Errorf("%s: body child = %T, want the table back", c.hold, got)
		}
	}
}

// TestInventoryOreSectionFollowsTheHold: a hull without an ore hold has no
// ore section and no ore seam (the stack reflows to Cargo/Hangar); fitting
// one brings the section and its seam back, and empty reads as present.
func TestInventoryOreSectionFollowsTheHold(t *testing.T) {
	env, _ := newTestEnv(t)
	inv := NewInventory(env)
	ui := newWindowUI(t, env, inv.Win())
	frame := func() { uitest.PumpWindow(t, ui, inv.Sync, 1280, 720) }
	frame()
	frame()

	if inv.oreShown || inv.orePane != nil {
		t.Fatal("the sloop has no ore hold but the section is built")
	}
	if seams := inv.Splits(); len(seams) != 1 || seams[0].Name != "inventory-hangar" || seams[0].Index != 0 {
		t.Errorf("seams = %+v, want only the hangar seam at index 0", seams)
	}
	if panes := splitPanes(inv.split); len(panes) != 2 {
		t.Errorf("%d panes, want 2 (cargo, hangar)", len(panes))
	}

	withOreHold(env, 20)
	frame()
	if !inv.oreShown || inv.orePane == nil {
		t.Fatal("fitting an ore hold did not bring the section back")
	}
	if seams := inv.Splits(); len(seams) != 2 || seams[0].Name != "inventory-ore" || seams[1].Name != "inventory-hangar" {
		t.Errorf("seams = %+v, want ore at 0 and hangar at 1", seams)
	}
	if panes := splitPanes(inv.split); len(panes) != 3 {
		t.Errorf("%d panes, want 3 (cargo, ore, hangar)", len(panes))
	}
	// An empty ore hold still shows its section (the hold is fitted, the
	// section is not a capacity report).
	cargo, _ := env.Sim.Cargos.Get(env.Sim.Player())
	cargo.Ore = nil
	frame()
	if !inv.oreShown || inv.orePane == nil || len(inv.ore.Rows) != 0 {
		t.Errorf("empty ore hold lost its section (shown=%v rows=%d)", inv.oreShown, len(inv.ore.Rows))
	}
	if got, want := inv.oreLbl.Label, "Ore hold"; got != want {
		t.Errorf("ore label = %q, want %q", got, want)
	}
}

// TestInventoryHangarHatch: undocked the hangar section is the procedural
// hatch ("Not docked", the table gone); docked it is the table again, and
// the tile view swaps inside the same body.
func TestInventoryHangarHatch(t *testing.T) {
	env, _ := newTestEnv(t)
	inv := NewInventory(env)
	ui := newWindowUI(t, env, inv.Win())
	frame := func() { uitest.PumpWindow(t, ui, inv.Sync, 1280, 720) }
	frame()
	frame()

	if !inv.hatchShown() {
		t.Fatal("undocked hangar is not the hatch")
	}
	if got := inv.hatch.text.Label; got != hatchLabel || hatchLabel != "Not docked" {
		t.Errorf("hatch label = %q, want %q", got, hatchLabel)
	}
	if b := inv.hatch.img.Bounds(); b.Dx() != inv.hangBody.GetWidget().Rect.Dx() || b.Dy() != inv.hangBody.GetWidget().Rect.Dy() {
		t.Errorf("hatch raster %v does not fill the hangar body %v", b, inv.hangBody.GetWidget().Rect)
	}
	// The hatch text is laid over the whole hangar band and centred there.
	if tr, br := inv.hatch.text.GetWidget().Rect, inv.hangBody.GetWidget().Rect; tr != br {
		t.Errorf("hatch text box %v, want the hangar band %v", tr, br)
	}

	dockAtStation(t, env.Sim)
	frame()
	if inv.hatchShown() {
		t.Fatal("docked hangar still shows the hatch")
	}
	if got := inv.hangBody.Children()[0]; !sameWidget(got, inv.hangar.Container()) {
		t.Errorf("docked hangar body = %T, want the table", got)
	}

	// The tile view keeps the hatch as long as the player is in space.
	env.Sim.Docked.Remove(env.Sim.Player())
	uitest.FireButton(ui, inv.view)
	frame()
	frame()
	if got := inv.hangBody.Children()[0]; !sameWidget(got, inv.hatch.Container()) {
		t.Errorf("undocked tile view hangar = %T, want the hatch", got)
	}
	dockAtStation(t, env.Sim)
	frame()
	if got := inv.hangBody.Children()[0]; !sameWidget(got, inv.hangTiles.Container()) {
		t.Errorf("docked tile view hangar = %T, want the tile grid", got)
	}
}

// TestInventoryFullnessBars: the cargo/ore headers carry the HUD bar seam
// with the used/capacity figures overlaid, the fill pixel width is the
// used/capacity ratio (clamped), and the hangar keeps its stack-count
// label with no bar (unlimited capacity).
func TestInventoryFullnessBars(t *testing.T) {
	env, _ := newTestEnv(t)
	inv := NewInventory(env)
	ui := newWindowUI(t, env, inv.Win())
	frame := func() { uitest.PumpWindow(t, ui, inv.Sync, 1280, 720) }
	frame()
	frame()

	cargo, _ := env.Sim.Cargos.Get(env.Sim.Player())
	item := content.ItemID("")
	for _, st := range cargo.Stacks {
		if def, ok := env.Content.Item(st.Item); ok && def.Volume > 0 {
			item = st.Item
			break
		}
	}
	def, _ := env.Content.Item(item)

	set := func(capacity, volume float64, ore bool) {
		c, _ := env.Sim.Cargos.Get(env.Sim.Player())
		if ore {
			c.OreCapacity = capacity
			c.Ore = []content.CargoStack{{Item: item, Qty: int(math.Ceil(volume / def.Volume))}}
		} else {
			c.Capacity = capacity
			c.Stacks = []content.CargoStack{{Item: item, Qty: int(math.Ceil(volume / def.Volume))}}
		}
		frame()
		frame()
	}
	inspect := func(name string, capacity, used float64) {
		t.Helper()
		b := inv.cargoBar
		if name == "ore" {
			b = inv.oreBar
		}
		inner := b.Bar.GetWidget().Rect.Inset(theme.Px(2))
		want := int(float64(inner.Dx()) * math.Max(0, math.Min(1, used/capacity)))
		if got := b.FillRect().Dx(); got != want {
			t.Errorf("%s: fill %d px, want %d (%.3f of %d inner px)", name, got, want, used/capacity, inner.Dx())
		}
		if b.FillRect() != (image.Rectangle{}) {
			if got, want := b.FillRect().Min, inner.Min; got != want {
				t.Errorf("%s: fill starts at %v, want the track inset %v", name, got, want)
			}
		}
		wantLabel := theme.FormatVolumeShort(used) + " / " + theme.FormatVolumeShort(capacity)
		if got := b.Value.Label; got != wantLabel {
			t.Errorf("%s: overlay = %q, want %q", name, got, wantLabel)
		}
	}

	set(100, 0, false)
	inspect("cargo", 100, 0)
	set(100, 50, false)
	inspect("cargo", 100, 50)
	set(100, 100, false)
	inspect("cargo", 100, 100)
	set(100, 240, false)
	inspect("cargo", 100, 240)
	withOreHold(env, 40)
	frame()
	set(40, 10, true)
	inspect("ore", 40, 10)

	// The hangar has no capacity: label only, no bar row in its pane.
	if n := len(inv.hangPane.Children()); n != 2 {
		t.Errorf("hangar pane has %d rows, want label + body (no bar)", n)
	}
	if n := len(inv.cargoPane.Children()); n != 3 {
		t.Errorf("cargo pane has %d rows, want label + bar + body", n)
	}

	// A hull with no cargo hold: the bar is hidden and the label says so.
	c, _ := env.Sim.Cargos.Get(env.Sim.Player())
	c.Capacity = 0
	frame()
	if inv.cargoBar.Container.GetWidget().IsVisible() {
		t.Error("cargo bar visible without a cargo hold")
	}
	if got := inv.cargoLbl.Label; got != "Cargo · no cargo hold" {
		t.Errorf("label = %q, want the no-hold note", got)
	}
}

// TestInventoryDragPopup: the drag's popup is the picker surfaced with the
// item's mark, its name, the destination and a 1..stack slider defaulting
// to the whole stack; Confirm sends the picked quantity, the same command
// the old move buttons sent.
func TestInventoryDragPopup(t *testing.T) {
	env, cmds := newTestEnv(t)
	inv := NewInventory(env)
	ui := newQtyUI(t, env, inv.Win())
	q := env.QtyWindow()
	dockAtStation(t, env.Sim)
	pump := func() { env.Pointer().Begin(); pumpQty(t, ui, env, inv.Sync, 1024, 700) }
	pump()
	pump()

	stackQty := 0
	c, _ := env.Sim.Cargos.Get(env.Sim.Player())
	for _, st := range c.Stacks {
		if st.Item == inv.cargo.Rows[0].Data.(content.ItemID) {
			stackQty += st.Qty
		}
	}
	dragStack(t, ui, env, inv, "cargo", 0, "hangar")
	if !q.IsOpen() {
		t.Fatal("drag release on the hangar opened no picker")
	}
	if got := q.Value(); got != stackQty {
		t.Fatalf("slider opened at %d, want the whole stack (%d)", got, stackQty)
	}
	if q.mark.Widget().GetWidget().GetVisibility() != widget.Visibility_Show {
		t.Error("the transfer popup shows no item icon")
	}
	name := env.QtyWindow().cv.TradeableName(inv.cargo.Rows[0].Data.(content.ItemID))
	if got := q.infoLine(); !contains(got, name) || !contains(got, "Station hangar") {
		t.Errorf("popup line = %q, want the item name and destination", got)
	}
	// The slider's top is the stack: past it clamps, not 100.
	q.Slider().SetValue(1e9)
	if got := q.Slider().Value(); got != float64(stackQty) {
		t.Errorf("slider top = %v, want the stack (%d)", got, stackQty)
	}
	k := max(1, stackQty/2)
	q.Slider().SetValue(float64(k))
	pump()
	uitest.FireButton(ui, q.Confirm())
	if len(*cmds) != 1 {
		t.Fatalf("%d commands, want 1", len(*cmds))
	}
	tr, ok := (*cmds)[0].(sim.CmdTransfer)
	if !ok || tr.Qty != k || tr.From != "cargo" || tr.To != "hangar" {
		t.Errorf("command = %#v, want CmdTransfer{cargo→hangar, qty %d}", (*cmds)[0], k)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// heldByItem counts the player's units of every item across the three holds
// AND the fitted modules' bays. A bay takes its rounds out of cargo at the
// start of a load (sim/clips.go), so a hold-only count reads the owner's
// "the third stack disappeared" as a loss when the rounds are in the gun.
func heldByItem(env *testEnv) map[content.ItemID]int {
	out := map[content.ItemID]int{}
	add := func(stacks []content.CargoStack) {
		for _, st := range stacks {
			out[st.Item] += st.Qty
		}
	}
	c, _ := env.Sim.Cargos.Get(env.Sim.Player())
	add(c.Stacks)
	add(c.Ore)
	add(env.Sim.Hangar.Items)
	if fit, ok := env.Sim.Fittings.Get(env.Sim.Player()); ok {
		for _, class := range []string{"high", "mid", "low"} {
			rts := fit.Runtimes(class)
			for i := range rts {
				if rts[i].Clip.Item != "" {
					out[rts[i].Clip.Item] += rts[i].Clip.Qty
				}
			}
		}
	}
	return out
}

// dragTile drags one section's tile i to another hold's section, the tile
// view's twin of dragStack.
func dragTile(t *testing.T, ui *ebitenui.UI, env *testEnv, inv *Inventory, from string, i int, to string) {
	t.Helper()
	pump := func() {
		env.Pointer().Begin()
		pumpQty(t, ui, env, inv.Sync, 1024, 700)
	}
	fromRect := inv.tilesFor(from).tiles[i].GetWidget().Rect
	body := inv.hangBody.GetWidget().Rect
	if to == "cargo" {
		body = inv.cargoBody.GetWidget().Rect
	}
	at := image.Pt(fromRect.Min.X+fromRect.Dx()/2, fromRect.Min.Y+fromRect.Dy()/2)
	over := image.Pt(body.Min.X+body.Dx()/2, body.Min.Y+body.Dy()/2)

	env.in.x, env.in.y = at.X, at.Y
	env.in.edge, env.in.down = [3]bool{true, false, false}, [3]bool{true, false, false}
	pump()
	env.in.x, env.in.y = over.X, over.Y
	env.in.edge = [3]bool{}
	pump()
	env.in.down = [3]bool{}
	pump()
}

// TestInventoryThreeStacksKeepEveryUnit is the owner's repro: three ammo
// stacks in cargo (the starting autocannon rounds), two drags to the hangar.
// The transfer resolves by item id in both views, the popup names the stack
// that was pressed, and no unit is ever lost: the third stack stays in cargo,
// because a bay loads only a round the player picked (sim/clips.go).
func TestInventoryThreeStacksKeepEveryUnit(t *testing.T) {
	rounds := []content.ItemID{"ammo-emp-s", "ammo-fusion-s", "ammo-ap-s"}
	for _, tiles := range []bool{false, true} {
		name := "list"
		if tiles {
			name = "tiles"
		}
		t.Run(name, func(t *testing.T) {
			env, cmds := newTestEnv(t)
			c, _ := env.Sim.Cargos.Get(env.Sim.Player())
			c.Stacks = []content.CargoStack{
				{Item: rounds[0], Qty: 80}, {Item: rounds[1], Qty: 40}, {Item: rounds[2], Qty: 20},
			}
			dockAtStation(t, env.Sim)
			inv := NewInventory(env)
			ui := newQtyUI(t, env, inv.Win())
			q := env.QtyWindow()
			pump := func() { env.Pointer().Begin(); pumpQty(t, ui, env, inv.Sync, 1024, 700) }
			pump()
			if tiles {
				uitest.FireButton(ui, inv.view)
				pump()
			}
			pump()

			want := heldByItem(env)
			move := func(item content.ItemID, qty int) {
				t.Helper()
				if tiles {
					dragTile(t, ui, env, inv, "cargo", 0, "hangar")
				} else {
					dragStack(t, ui, env, inv, "cargo", 0, "hangar")
				}
				if !q.IsOpen() {
					t.Fatalf("%s: the drop opened no picker", item)
				}
				// The popup's mark, name and quantity all come from the id of
				// the stack that was pressed.
				if q.req.Item != item {
					t.Fatalf("popup item=%q want the pressed stack %q", q.req.Item, item)
				}
				if got := q.Value(); got != qty {
					t.Fatalf("popup opens at %d, want the whole %s stack (%d)", got, item, qty)
				}
				if got := q.infoLine(); !contains(got, env.TradeableName(item)) {
					t.Fatalf("popup line %q does not name %q", got, env.TradeableName(item))
				}
				if got, want := q.mark.Name(), widgets.TradeableIcon(env.Defs(), item); got != want {
					t.Fatalf("popup mark=%q want %q (the pressed stack's icon)", got, want)
				}
				uitest.FireButton(ui, q.Confirm())
				if len(*cmds) != 1 {
					t.Fatalf("%d commands for one Confirm, want 1", len(*cmds))
				}
				for _, cmd := range *cmds {
					env.Sim.Queue(cmd)
				}
				*cmds = (*cmds)[:0]
				env.Sim.Step()
				for got, n := range heldByItem(env) {
					if n != want[got] {
						t.Errorf("after %s: %s held=%d want %d (every unit must still exist)", item, got, n, want[got])
					}
				}
				pump()
				pump()
			}

			move(rounds[0], 80)
			move(rounds[1], 40)

			// The two moved stacks are whole in the hangar.
			hangar := env.Sim.Hangar.Items
			if len(hangar) != 2 || hangar[0].Item != rounds[0] || hangar[0].Qty != 80 ||
				hangar[1].Item != rounds[1] || hangar[1].Qty != 40 {
				t.Errorf("hangar=%+v want 80 %s + 40 %s", hangar, rounds[0], rounds[1])
			}
			// The third stack is whole somewhere: in cargo, because no bay
			// loads a round nobody picked (sim/clips.go).
			c, _ = env.Sim.Cargos.Get(env.Sim.Player())
			if got := heldByItem(env)[rounds[2]]; got != 20 {
				t.Errorf("%s held=%d want 20", rounds[2], got)
			}
			inCargo := 0
			for _, st := range c.Stacks {
				if st.Item == rounds[2] {
					inCargo += st.Qty
				}
			}
			if inCargo != 20 {
				t.Errorf("%s in cargo=%d want 20 (no bay may take it)", rounds[2], inCargo)
			}
			t.Logf("%s: cargo=%d of 20 after the two moves (the third stack stayed in cargo)",
				rounds[2], inCargo)
		})
	}
}

// sameWidget compares a body child with a panel's container across the
// two ebitenui interfaces (Containerer does not declare GetWidget).
func sameWidget(a widget.PreferredSizeLocateableWidget, c widget.Containerer) bool {
	w, ok := c.(interface{ GetWidget() *widget.Widget })
	return ok && a.GetWidget() == w.GetWidget()
}

// TestInventoryDragGhostFollows: while a stack is dragged the chip tracks the
// pointer across two move frames at scale 1 and 1.5, carries the item's name,
// and the hold under it lights as the drop target; release clears both. The
// two moves stay inside one drop target on purpose: the old pin crossed into
// a NEW target, whose highlight flip happened to re-lay the overlay out, so
// it passed while the chip stood still everywhere else in the real game.
//
// This pins the placed box. The DRAWN chip's accent-border centroid is pinned
// in TestInventoryCaptureFrame's verify (headless tests cannot read pixels
// back: ebiten's ReadPixels refuses before the loop starts, which is why
// every pixel assertion in the house is capture-gated).
func TestInventoryDragGhostFollows(t *testing.T) {
	defer theme.SetScale(1)
	for _, sc := range []float64{1, 1.5} {
		t.Run(fmt.Sprintf("scale %v", sc), func(t *testing.T) {
			theme.SetScale(sc)
			runDragGhostFollow(t)
		})
	}
}

func runDragGhostFollow(t *testing.T) {
	env, _ := newTestEnv(t)
	dockAtStation(t, env.Sim)
	inv := NewInventory(env)
	ui := newWindowUI(t, env, inv.Win())
	w, h := theme.Px(1024), theme.Px(700)
	frame := func() {
		env.Pointer().Begin()
		inv.Sync()
		ui.Update()
		ui.Draw(ebiten.NewImage(w, h))
	}
	frame()
	frame()

	r := inv.cargo.RowWidgetForTest(0).Rect
	at := image.Pt(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)
	env.in.x, env.in.y = at.X, at.Y
	env.in.edge, env.in.down = [3]bool{true}, [3]bool{true}
	frame()

	band := inv.hangBody.GetWidget().Rect
	p1 := image.Pt(band.Min.X+theme.Px(40), band.Max.Y-theme.Px(30))
	env.in.x, env.in.y = p1.X, p1.Y
	env.in.edge = [3]bool{}
	frame()
	g1 := inv.ghost.container.GetWidget().Rect
	if want := ghostBox(inv, p1); g1.Min != want.Min {
		t.Fatalf("chip at %v, want the pointer offset %v (first move)", g1.Min, want.Min)
	}

	// The second move stays inside the same (already lit) target: no
	// highlight flip, so the chip must move on its own.
	delta := image.Pt(theme.Px(120), 0)
	p2 := p1.Add(delta)
	env.in.x, env.in.y = p2.X, p2.Y
	frame()
	g2 := inv.ghost.container.GetWidget().Rect
	if want := ghostBox(inv, p2); g2.Min != want.Min {
		t.Errorf("chip at %v, want the pointer offset %v (second move)", g2.Min, want.Min)
	}
	if got := g2.Min.Sub(g1.Min); got != delta {
		t.Errorf("chip moved %v, want the pointer delta %v", got, delta)
	}

	if name := inv.ghost.name.Label; name != env.TradeableName(inv.cargo.Rows[0].Data.(content.ItemID)) {
		t.Errorf("ghost names %q, want the dragged item", name)
	}
	if !inv.hangFrame.GetWidget().IsVisible() {
		t.Error("the hovered hold is not lit as the drop target")
	}
	if inv.cargoFrame.GetWidget().IsVisible() {
		t.Error("the source hold stayed lit")
	}

	env.in.down = [3]bool{}
	frame()
	if inv.ghost.container.GetWidget().IsVisible() || inv.hangFrame.GetWidget().IsVisible() {
		t.Error("release left the drag chrome up")
	}
}

// ghostBox is where the overlay puts the chip for pointer p: p + the 12 px
// offset, in the chip's own measured size (the placement RULE, not the placed
// rect, so a stale layout cannot satisfy it).
func ghostBox(inv *Inventory, p image.Point) image.Rectangle {
	gw, gh := inv.ghost.container.PreferredSize()
	at := p.Add(image.Pt(theme.Px(12), theme.Px(12)))
	return image.Rect(at.X, at.Y, at.X+gw, at.Y+gh)
}

// TestInventoryDropOnEmptySpace: a release that lands outside every hold's
// section (the title bar here) is a no-op, like a release on the source.
func TestInventoryDropOnEmptySpace(t *testing.T) {
	env, _ := newTestEnv(t)
	dockAtStation(t, env.Sim)
	inv := NewInventory(env)
	ui := newQtyUI(t, env, inv.Win())
	q := env.QtyWindow()
	pump := func() { env.Pointer().Begin(); pumpQty(t, ui, env, inv.Sync, 1024, 700) }
	pump()
	pump()

	r := inv.cargo.RowWidgetForTest(0).Rect
	env.in.x, env.in.y = r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
	env.in.edge, env.in.down = [3]bool{true}, [3]bool{true}
	pump()
	title := inv.win.GetContainer().GetWidget().Rect
	env.in.x, env.in.y = title.Min.X+20, title.Min.Y+theme.Px(theme.TitleBarH)/2
	env.in.edge = [3]bool{}
	pump()
	env.in.down = [3]bool{}
	pump()
	if q.IsOpen() {
		t.Error("release outside every section opened the transfer popup")
	}
}

// TestInventoryToolbarKeepsOnlySell: the three move buttons are gone (the
// drag replaced them) and the view toggle moved to the title bar, so the
// toolbar carries Sell alone, greyed until a docked selection.
func TestInventoryToolbarKeepsOnlySell(t *testing.T) {
	env, _ := newTestEnv(t)
	inv := NewInventory(env)
	ui := newWindowUI(t, env, inv.Win())
	uitest.PumpWindow(t, ui, inv.Sync, 1024, 700)

	toolbar := inv.body.Children()[1].(*widget.Container)
	var labels []string
	for _, ch := range toolbar.Children() {
		if btn, ok := ch.(*widget.Button); ok {
			labels = append(labels, btn.Text().Label)
		}
	}
	if len(labels) != 1 || labels[0] != "Sell" {
		t.Fatalf("toolbar = %v, want Sell alone (the toggle is title-bar furniture)", labels)
	}
	if !inv.sell.GetWidget().Disabled {
		t.Error("Sell armed with no selection")
	}
	clickRow(ui, inv.cargo, 0)
	dockAtStation(t, env.Sim)
	uitest.PumpWindow(t, ui, inv.Sync, 1024, 700)
	if inv.sell.GetWidget().Disabled {
		t.Error("Sell still greyed with a docked selection")
	}
}

// TestInventoryViewToggleIsTitleBarFurniture: the view switch is the title
// bar's action slot immediately left of the × (one SpSm gap), not a toolbar
// button: a closeBtnH square, icon-only, its glyph naming the view it
// switches to (the tooltip carries the word).
func TestInventoryViewToggleIsTitleBarFurniture(t *testing.T) {
	k, _ := newTestKit(t)
	inv := NewInventory(k.Env())
	k.SetWindowClose(inv.Win(), func() {})
	k.AddWindow(inv.Win())
	frame := func() {
		k.Pointer().Begin()
		inv.Sync()
		k.Sync()
		if err := k.Update(); err != nil {
			t.Fatalf("Update: %v", err)
		}
		k.Draw(ebiten.NewImage(1024, 700))
	}
	frame()
	frame()

	closeBtn := k.CloseButtonOf(inv.Win()).GetWidget().Rect
	view := inv.view.GetWidget().Rect
	bar := inv.win.TitleBar.GetWidget().Rect
	if closeBtn.Empty() || view.Empty() {
		t.Fatalf("fixture: × %v / toggle %v never laid out", closeBtn, view)
	}
	if !view.In(bar) {
		t.Errorf("view toggle %v is not in the title bar %v", view, bar)
	}
	if want := closeBtn.Min.X - theme.Px(theme.SpSm); view.Max.X != want {
		t.Errorf("toggle right edge %d, want %d (SpSm clear of the × at %d)", view.Max.X, want, closeBtn.Min.X)
	}
	if view.Min.Y != closeBtn.Min.Y || view.Dy() != theme.Px(closeBtnH) || view.Dx() != view.Dy() {
		t.Errorf("toggle %v, want the ×'s %d×%d band (%v)", view, theme.Px(closeBtnH), theme.Px(closeBtnH), closeBtn)
	}
	if got := inv.viewIcon.Name(); got != "stat_structure" {
		t.Errorf("toggle glyph = %q, want stat_structure (the view it switches to)", got)
	}
	if got := inv.viewTip.Model().Title; got != "Tiles" {
		t.Errorf("toggle tooltip = %q, want Tiles", got)
	}

	// The title bar's furniture is a button like any other: its click
	// handler is what a press on it runs (ebitenui's own input reads the
	// OS mouse, which a headless test cannot script; the house fires the
	// button event instead).
	click := func() {
		inv.view.ClickedEvent.Fire(&widget.ButtonClickedEventArgs{Button: inv.view})
		if err := k.Update(); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}
	click()
	if !inv.ViewTiles() {
		t.Fatal("the title bar's toggle did not switch to the tile view")
	}
	if got, want := inv.viewIcon.Name(), "row_module"; got != want {
		t.Errorf("toggle glyph after the switch = %q, want %q", got, want)
	}
	if got := inv.viewTip.Model().Title; got != "List" {
		t.Errorf("toggle tooltip after the switch = %q, want List", got)
	}
	click()
	if inv.ViewTiles() {
		t.Error("the title bar's toggle did not switch back to the list view")
	}
	if got, want := inv.viewIcon.Name(), "stat_structure"; got != want {
		t.Errorf("toggle glyph back = %q, want %q", got, want)
	}
}

// TestValueBarIsTheSharedSeam: the HUD capacitor and the inventory's hold
// bars are one construction (a progress bar under a centred value text),
// so the bar painter is not forked.
func TestValueBarIsTheSharedSeam(t *testing.T) {
	env, _ := newTestEnv(t)
	h := NewHud(env)
	inv := NewInventory(env)
	uitest.PumpWindow(t, newWindowUI(t, env, inv.Win()), inv.Sync, 1280, 720)

	if hudCapMax != widgets.ValueBarMax || hudBarH != widgets.BarH {
		t.Fatalf("HUD bar constants drifted from the seam: %d/%d vs %d/%d",
			hudCapMax, hudBarH, widgets.ValueBarMax, widgets.BarH)
	}
	if h.capBar == nil || h.capValue == nil {
		t.Fatal("the HUD capacitor is not on the shared seam")
	}
	// Both surfaces: a ProgressBar and its centred Text in one stacked
	// container (the HUD adds its glyph after the pair).
	check := func(name string, stack *widget.Container, bar *widget.ProgressBar, text *widget.Text) {
		t.Helper()
		found := 0
		for _, ch := range stack.Children() {
			if ch == bar {
				found |= 1
			}
			if ch == text {
				found |= 2
			}
		}
		if found != 3 {
			t.Errorf("%s: bar/text are not siblings in one stack (%d)", name, found)
		}
	}
	check("hud", h.capStack, h.capBar, h.capValue)
	check("cargo bar", inv.cargoBar.Container, inv.cargoBar.Bar, inv.cargoBar.Value)
	check("ore bar", inv.oreBar.Container, inv.oreBar.Bar, inv.oreBar.Value)

	// One dialog: the drag and Sell both surface the session's picker.
	if inv.pane == nil || inv.ghost == nil {
		t.Error("the drag chrome did not wire")
	}
}

// ---------------------------------------------------------------------------
// The Selection set: modifier selection and the collective actions
// ---------------------------------------------------------------------------

// multiStacksEnv is the Selection-set fixture: four whole cargo stacks
// (the three ammo rounds plus one mineral, so one row is never a member),
// an ore hold (the hold-switch case presses into it) and the docked player
// the Sell and hangar gates need. Pumped twice, so rows exist.
func multiStacksEnv(t *testing.T) (*testEnv, *[]sim.Command, *Inventory, *ebitenui.UI) {
	t.Helper()
	env, cmds := newTestEnv(t)
	c, _ := env.Sim.Cargos.Get(env.Sim.Player())
	c.Stacks = []content.CargoStack{
		{Item: "ammo-emp-s", Qty: 80}, {Item: "ammo-fusion-s", Qty: 40},
		{Item: "ammo-ap-s", Qty: 20}, {Item: "min-ferrite", Qty: 60},
	}
	withOreHold(env, 30)
	dockAtStation(t, env.Sim)
	inv := NewInventory(env)
	// The window's tall landing rect, like the capture's: with three
	// sections every pane must hold its header plus its rows, or a row's
	// centre falls below the pane rect and the drag's section gate (as
	// ever) never arms.
	inv.Win().SetLocation(image.Rect(40, 20, 740, 700))
	ui := newQtyUI(t, env, inv.Win())
	frame := func() { env.Pointer().Begin(); pumpQty(t, ui, env, inv.Sync, 1024, 700) }
	frame()
	frame()
	if len(inv.cargo.Rows) != 4 {
		t.Fatalf("fixture: %d cargo rows, want 4", len(inv.cargo.Rows))
	}
	return env, cmds, inv, ui
}

// rounds4 is the fixture's stacks in row (display) order.
var rounds4 = []content.ItemID{"ammo-emp-s", "ammo-fusion-s", "ammo-ap-s", "min-ferrite"}

// TestInventorySelectionSetFromModifiers is the standard inventory
// selection grammar: plain replaces (but a press on a MEMBER keeps the
// set, which is what drags it), Shift takes the display-order range from
// the anchor, Ctrl toggles EXACTLY once per press (the release's click
// half shares the press's sample, so a double run would end empty),
// Ctrl+Shift adds the range instead of replacing it, and a first press in
// another hold starts that hold's set (one hold owns the selection: the
// transfer's From is single). The member rows wear the paint; the tables
// outside src wear none.
func TestInventorySelectionSetFromModifiers(t *testing.T) {
	env, _, inv, ui := multiStacksEnv(t)

	// Plain: one stack, anchored there, painted.
	clickStack(t, ui, env, inv, "cargo", 0, 0)
	wantSet(t, inv, rounds4[0])
	if inv.anchor != rounds4[0] || inv.src != "cargo" || !inv.hasSel || inv.sel != rounds4[0] {
		t.Fatalf("after plain: anchor=%q src=%q hasSel=%v sel=%q, want the pressed stack owning cargo",
			inv.anchor, inv.src, inv.hasSel, inv.sel)
	}
	if !inv.cargo.SelectedAtForTest(0) || inv.cargo.SelectedAtForTest(1) ||
		inv.cargo.SelectedAtForTest(2) || inv.cargo.SelectedAtForTest(3) {
		t.Errorf("paint = %v %v %v %v, want row 0 only",
			inv.cargo.SelectedAtForTest(0), inv.cargo.SelectedAtForTest(1),
			inv.cargo.SelectedAtForTest(2), inv.cargo.SelectedAtForTest(3))
	}
	if inv.ore.SelectedAtForTest(0) || inv.hangar.SelectedAtForTest(0) {
		t.Error("a section outside src wears the selection paint")
	}

	// Shift: the range from the anchor, ending primary on the shift-to row.
	clickStack(t, ui, env, inv, "cargo", 2, widgets.ModShift)
	wantSet(t, inv, rounds4[0], rounds4[1], rounds4[2])
	if inv.sel != rounds4[2] {
		t.Errorf("primary = %q, want the shifted-to stack %q", inv.sel, rounds4[2])
	}
	for i, want := range []bool{true, true, true, false} {
		if got := inv.cargo.SelectedAtForTest(i); got != want {
			t.Errorf("range paint row %d = %v, want %v", i, got, want)
		}
	}

	// Ctrl off, Ctrl ON once, at a row whose PRESS arms (row1: the row's
	// centre sits inside its pane, so the press half runs and the
	// release half's dedupe is genuinely exercised — a double run would
	// drop row 1 right back out).
	clickStack(t, ui, env, inv, "cargo", 1, widgets.ModCtrl)
	wantSet(t, inv, rounds4[0], rounds4[2])
	if inv.selSet[rounds4[1]] {
		t.Error("ctrl+click left the row selected (toggle did not apply?)")
	}
	clickStack(t, ui, env, inv, "cargo", 1, widgets.ModCtrl)
	wantSet(t, inv, rounds4[0], rounds4[1], rounds4[2])
	if len(inv.selSet) != 3 {
		t.Fatalf("set after the second ctrl+click = %v, want 3 members (a double toggle would drop row 1)", setIDs(inv))
	}

	// Plain on a member keeps the whole set; on a non-member it collapses.
	clickStack(t, ui, env, inv, "cargo", 1, 0)
	wantSet(t, inv, rounds4[0], rounds4[1], rounds4[2])
	clickStack(t, ui, env, inv, "cargo", 3, 0)
	wantSet(t, inv, rounds4[3])

	// Shift backwards re-anchors from the plain click: row 3 down to 0.
	clickStack(t, ui, env, inv, "cargo", 0, widgets.ModShift)
	wantSet(t, inv, rounds4[0], rounds4[1], rounds4[2], rounds4[3])

	// Ctrl toggles one member out of the full range.
	clickStack(t, ui, env, inv, "cargo", 2, widgets.ModCtrl)
	wantSet(t, inv, rounds4[0], rounds4[1], rounds4[3])

	// Plain on a member keeps the whole set and re-anchors the range
	// there (this is also the drag-on-a-set rule).
	clickStack(t, ui, env, inv, "cargo", 1, 0)
	wantSet(t, inv, rounds4[0], rounds4[1], rounds4[3])

	// Ctrl+Shift unions the range into the set instead of replacing it:
	// rows 1..3 join, and row 0, outside the range, survives.
	clickStack(t, ui, env, inv, "cargo", 3, widgets.ModShift|widgets.ModCtrl)
	wantSet(t, inv, rounds4[0], rounds4[1], rounds4[2], rounds4[3])

	// A first press in the ore hold starts the ore hold's set (cargo's is
	// gone: one hold owns the selection).
	clickStack(t, ui, env, inv, "ore", 0, 0)
	if inv.src != "ore" || !inv.selSet["ore-cordite"] || len(inv.selSet) != 1 {
		t.Fatalf("after the ore press: src=%q set=%v, want the ore hold owning {ore-cordite}", inv.src, setIDs(inv))
	}
	if !inv.ore.SelectedAtForTest(0) || inv.cargo.SelectedAtForTest(0) {
		t.Error("the paint did not follow the selection into the ore hold")
	}
}

// TestInventoryShiftRangeSurvivesReleasingShiftBeforeTheClick pins the
// press's own sample: a Shift+click whose Shift is already up by the time
// the row's click half runs still takes the range (gestures are read at
// the press, like every other inventory list).
func TestInventoryShiftRangeSurvivesReleasingShiftBeforeTheClick(t *testing.T) {
	env, _, inv, ui := multiStacksEnv(t)
	r := inv.cargo.RowWidgetForTest(1).Rect
	at := image.Pt(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)

	clickStack(t, ui, env, inv, "cargo", 0, 0) // the anchor

	// Press half with Shift held (row1's centre sits inside its pane, so
	// the press arms and claims the gesture)...
	env.in.x, env.in.y = at.X, at.Y
	env.in.mods = widgets.ModShift
	env.in.edge, env.in.down = [3]bool{true}, [3]bool{true}
	env.Pointer().Begin()
	pumpQty(t, ui, env, inv.Sync, 1024, 700)
	// ...Shift gone before the release half and its click.
	env.in.mods = 0
	env.in.edge, env.in.down = [3]bool{}, [3]bool{}
	env.Pointer().Begin()
	pumpQty(t, ui, env, inv.Sync, 1024, 700)
	clickRow(ui, inv.cargo, 1)
	pumpQty(t, ui, env, inv.Sync, 1024, 700)

	wantSet(t, inv, rounds4[0], rounds4[1])
}

// TestInventoryBulkTransferMovesEveryStackWhole: dropping a MULTI-
// selection on another hold moves every selected stack whole with NO
// quantity prompt (the owner's ruling), conserving every unit through the
// sim's own transfer commands; the stacks that were not selected stay,
// and the emptied set disarms the actions.
func TestInventoryBulkTransferMovesEveryStackWhole(t *testing.T) {
	env, cmds, inv, ui := multiStacksEnv(t)
	q := env.QtyWindow()
	pump := func() { env.Pointer().Begin(); pumpQty(t, ui, env, inv.Sync, 1024, 700) }

	// Select rows 0..2 (plain anchor + shift range), leaving row 3 out.
	clickStack(t, ui, env, inv, "cargo", 0, 0)
	clickStack(t, ui, env, inv, "cargo", 2, widgets.ModShift)
	wantSet(t, inv, rounds4[0], rounds4[1], rounds4[2])

	// The chip names the stack count while the set is pressed (one stack
	// pressed: the set is what moves, so the chip says so).
	r := inv.cargo.RowWidgetForTest(0).Rect
	env.in.x, env.in.y = r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
	env.in.edge, env.in.down = [3]bool{true}, [3]bool{true}
	env.Pointer().Begin()
	pumpQty(t, ui, env, inv.Sync, 1024, 700)
	if got := inv.ghost.name.Label; !strings.Contains(got, "3 stacks") {
		t.Errorf("ghost label = %q, want the stack count named", got)
	}
	env.in.edge, env.in.down = [3]bool{}, [3]bool{}
	env.Pointer().Begin()
	pumpQty(t, ui, env, inv.Sync, 1024, 700)
	wantSet(t, inv, rounds4[0], rounds4[1], rounds4[2]) // the press on a member kept the set

	want := heldByItem(env)
	dragStack(t, ui, env, inv, "cargo", 0, "hangar")
	if q.IsOpen() {
		t.Fatal("the multi-drop opened the quantity picker (a set moves whole, no prompts)")
	}
	pump()
	if len(*cmds) != 3 {
		t.Fatalf("%d commands, want one Transfer per selected stack: %#v", len(*cmds), *cmds)
	}
	for i, cmd := range *cmds {
		tr, ok := cmd.(sim.CmdTransfer)
		if !ok {
			t.Fatalf("command %d = %#v, want CmdTransfer", i, cmd)
		}
		if tr.From != "cargo" || tr.To != "hangar" {
			t.Errorf("command %d = cargo %q -> %q, want the drop's holds", i, tr.From, tr.To)
		}
		if tr.Qty != wantedQty(rounds4[i]) {
			t.Errorf("command %d = %d x %s, want the WHOLE stack (%d)", i, tr.Qty, tr.Item, wantedQty(rounds4[i]))
		}
	}
	for _, cmd := range *cmds {
		env.Sim.Queue(cmd)
	}
	env.Sim.Step()
	*cmds = (*cmds)[:0]

	// Three whole stacks in the hangar, the unselected one untouched.
	hangar := env.Sim.Hangar.Items
	if len(hangar) != 3 {
		t.Fatalf("hangar = %+v, want the three selected stacks", hangar)
	}
	for i, st := range hangar {
		if st.Item != rounds4[i] || st.Qty != wantedQty(rounds4[i]) {
			t.Errorf("hangar[%d] = %+v, want %d x %s whole", i, st, wantedQty(rounds4[i]), rounds4[i])
		}
	}
	for got, n := range heldByItem(env) {
		if n != want[got] {
			t.Errorf("%s held=%d want=%d (the bulk move must conserve every unit)", got, n, want[got])
		}
	}

	// The stacks left their hold: the set empties and the actions disarm.
	pump()
	pump()
	if len(inv.selSet) != 0 || inv.hasSel {
		t.Errorf("after the move: set=%v hasSel=%v, want the selection emptied", setIDs(inv), inv.hasSel)
	}
	if !inv.sell.GetWidget().Disabled {
		t.Error("Sell stayed armed with nothing selected")
	}
	if q.IsOpen() {
		t.Error("a picker opened somewhere in the bulk path")
	}
}

// wantedQty is a fixture stack's quantity by item (the bulk assertions
// read the expected whole-stack figure from the same table the fixture
// built).
func wantedQty(item content.ItemID) int {
	for _, st := range []content.CargoStack{
		{Item: "ammo-emp-s", Qty: 80}, {Item: "ammo-fusion-s", Qty: 40},
		{Item: "ammo-ap-s", Qty: 20}, {Item: "min-ferrite", Qty: 60},
	} {
		if st.Item == item {
			return st.Qty
		}
	}
	return 0
}

// TestInventoryBulkSellSellsEveryStackWhole: the Sell button mirrors the
// drop rule. A multi-selection sells every selected stack whole with no
// prompts (one Sell intent per stack); ONE selected stack keeps its
// quantity picker, so partial sales stay possible.
func TestInventoryBulkSellSellsEveryStackWhole(t *testing.T) {
	env, cmds, inv, ui := multiStacksEnv(t)
	q := env.QtyWindow()
	pump := func() { env.Pointer().Begin(); pumpQty(t, ui, env, inv.Sync, 1024, 700) }

	// Two selected stacks: the bulk path.
	clickStack(t, ui, env, inv, "cargo", 0, 0)
	clickStack(t, ui, env, inv, "cargo", 1, widgets.ModCtrl)
	wantSet(t, inv, rounds4[0], rounds4[1])
	inv.sellClicked()
	pump()
	if q.IsOpen() {
		t.Fatal("the bulk sale opened the quantity picker (a set sells whole, no prompts)")
	}
	if len(*cmds) != 2 {
		t.Fatalf("%d commands, want one Sell per selected stack: %#v", len(*cmds), *cmds)
	}
	for i, cmd := range *cmds {
		sell, ok := cmd.(sim.CmdSell)
		if !ok {
			t.Fatalf("command %d = %#v, want CmdSell", i, cmd)
		}
		if want := wantedQty(rounds4[i]); sell.Qty != want || sell.Item != rounds4[i] {
			t.Errorf("sell %d = %d x %s, want the whole stack %d x %s", i, sell.Qty, sell.Item, want, rounds4[i])
		}
	}
	*cmds = (*cmds)[:0]

	// One selected stack: the picker stays (the partial-sale surface).
	clickStack(t, ui, env, inv, "cargo", 2, 0)
	wantSet(t, inv, rounds4[2])
	inv.sellClicked()
	pump()
	if !q.IsOpen() {
		t.Fatal("a single selected stack sold without its quantity picker")
	}
	if q.req.Item != rounds4[2] {
		t.Errorf("picker target = %q, want the single selection %q", q.req.Item, rounds4[2])
	}
	if len(*cmds) != 0 {
		t.Errorf("opening the picker enqueued %d commands, want none", len(*cmds))
	}
}

// TestInventoryTileViewMultiSelectAndBulkMove: the thumbnail view runs the
// same grammar through the press half alone (tiles declare no click
// handler): every member tile carries the accent frame (the membership
// mark in the grid's change signature) and the drop moves the set whole.
func TestInventoryTileViewMultiSelectAndBulkMove(t *testing.T) {
	env, cmds, inv, ui := multiStacksEnv(t)
	q := env.QtyWindow()
	pump := func() { env.Pointer().Begin(); pumpQty(t, ui, env, inv.Sync, 1024, 700) }

	uitest.FireButton(ui, inv.view)
	pump()
	pump()
	if !inv.ViewTiles() || len(inv.cargoTiles.tiles) != 4 {
		t.Fatalf("fixture: tiles=%v (%d tiles), want the 4-tile thumbnail view", inv.ViewTiles(), len(inv.cargoTiles.tiles))
	}

	// Press grammar: plain then Ctrl (no click half to dedupe against).
	clickTile(t, ui, env, inv, "cargo", 0, 0)
	clickTile(t, ui, env, inv, "cargo", 2, widgets.ModCtrl)
	wantSet(t, inv, rounds4[0], rounds4[2])
	if got := strings.Count(inv.cargoTiles.sig, "+"); got != 2 {
		t.Errorf("tile membership marks = %d, want 2 (the accent-framed tiles)", got)
	}
	if strings.Contains(inv.hangTiles.sig, "+") {
		t.Error("a tile grid outside src carries the membership mark")
	}

	// The drop moves the set whole: press on a member keeps it.
	want := heldByItem(env)
	dragTile(t, ui, env, inv, "cargo", 0, "hangar")
	if q.IsOpen() {
		t.Fatal("the tile multi-drop opened the quantity picker")
	}
	pump()
	if len(*cmds) != 2 {
		t.Fatalf("%d commands, want one Transfer per selected tile: %#v", len(*cmds), *cmds)
	}
	for _, cmd := range *cmds {
		env.Sim.Queue(cmd)
	}
	env.Sim.Step()
	for got, n := range heldByItem(env) {
		if n != want[got] {
			t.Errorf("%s held=%d want=%d (the bulk move must conserve every unit)", got, n, want[got])
		}
	}
	hangar := env.Sim.Hangar.Items
	if len(hangar) != 2 || hangar[0].Item != rounds4[0] || hangar[1].Item != rounds4[2] {
		t.Errorf("hangar = %+v, want the two selected stacks", hangar)
	}
}

// TestInventorySelectionRevalidatesAgainstItsHold: members whose stacks
// left the hold drop out of the set (the primary follows a member that
// remains), and an emptied set disarms Sell. The click-time cache alone
// would keep the actions armed on rows that no longer exist.
func TestInventorySelectionRevalidatesAgainstItsHold(t *testing.T) {
	env, _, inv, ui := multiStacksEnv(t)
	pump := func() { env.Pointer().Begin(); pumpQty(t, ui, env, inv.Sync, 1024, 700) }

	clickStack(t, ui, env, inv, "cargo", 0, 0)
	clickStack(t, ui, env, inv, "cargo", 2, widgets.ModShift)
	wantSet(t, inv, rounds4[0], rounds4[1], rounds4[2])
	if inv.sel != rounds4[2] {
		t.Fatalf("fixture: primary = %q, want the shift-to stack", inv.sel)
	}

	// The primary's stack leaves: the set drops it, the primary follows a
	// member that remains.
	c, _ := env.Sim.Cargos.Get(env.Sim.Player())
	c.Stacks = []content.CargoStack{
		{Item: rounds4[0], Qty: 80}, {Item: rounds4[1], Qty: 40}, {Item: rounds4[3], Qty: 60},
	}
	pump()
	wantSet(t, inv, rounds4[0], rounds4[1])
	if inv.sel != rounds4[0] {
		t.Errorf("primary after its stack left = %q, want the first remaining member %q", inv.sel, rounds4[0])
	}
	if !inv.hasSel || inv.sell.GetWidget().Disabled {
		t.Error("the actions disarmed while members remain")
	}

	// Every member leaves: the set empties and the actions disarm.
	c.Stacks = []content.CargoStack{{Item: rounds4[3], Qty: 60}}
	pump()
	if len(inv.selSet) != 0 || inv.hasSel {
		t.Fatalf("set=%v hasSel=%v after every member left, want empty", setIDs(inv), inv.hasSel)
	}
	if inv.sel != "" {
		t.Errorf("primary = %q, want cleared with the set", inv.sel)
	}
	if !inv.sell.GetWidget().Disabled {
		t.Error("Sell stayed armed with nothing selected")
	}
}
