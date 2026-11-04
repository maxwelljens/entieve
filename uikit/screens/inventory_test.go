package screens

import (
	"image"
	"math"
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
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
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-solace"})
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
		{"cargo", inv.cargoBody, len(inv.cv.CargoStacks())},
		{"ore", inv.oreBody, len(inv.cv.OreStacks())},
		{"hangar", inv.hangBody, len(inv.cv.HangarItems())},
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

	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-solace"})
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
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-solace"})
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
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-solace"})
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

// sameWidget compares a body child with a panel's container across the
// two ebitenui interfaces (Containerer does not declare GetWidget).
func sameWidget(a widget.PreferredSizeLocateableWidget, c widget.Containerer) bool {
	w, ok := c.(interface{ GetWidget() *widget.Widget })
	return ok && a.GetWidget() == w.GetWidget()
}

// TestInventoryDragGhostFollows: while a stack is dragged the ghost chip
// follows the pointer, carries the item's name, and the hold under it
// lights as the drop target; release clears both.
func TestInventoryDragGhostFollows(t *testing.T) {
	env, _ := newTestEnv(t)
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-solace"})
	inv := NewInventory(env)
	ui := newWindowUI(t, env, inv.Win())
	pump := func() { env.Pointer().Begin(); uitest.PumpWindow(t, ui, inv.Sync, 1024, 700) }
	pump()
	pump()

	r := inv.cargo.RowWidgetForTest(0).Rect
	at := image.Pt(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)
	env.in.x, env.in.y = at.X, at.Y
	env.in.edge, env.in.down = [3]bool{true}, [3]bool{true}
	pump()
	over := inv.hangBody.GetWidget().Rect
	to := image.Pt(over.Min.X+over.Dx()/2, over.Min.Y+over.Dy()/2)
	env.in.x, env.in.y = to.X, to.Y
	env.in.edge = [3]bool{}
	pump()

	ghost := inv.ghost.container.GetWidget()
	if !ghost.IsVisible() {
		t.Fatal("no drag ghost while dragging")
	}
	if name := inv.ghost.name.Label; name != env.TradeableName(inv.cargo.Rows[0].Data.(content.ItemID)) {
		t.Errorf("ghost names %q, want the dragged item", name)
	}
	if want := to.Add(image.Pt(theme.Px(12), theme.Px(12))); ghost.Rect.Min != want {
		t.Errorf("ghost at %v, want the pointer offset %v", ghost.Rect.Min, want)
	}
	if !inv.hangFrame.GetWidget().IsVisible() {
		t.Error("the hovered hold is not lit as the drop target")
	}
	if inv.cargoFrame.GetWidget().IsVisible() {
		t.Error("the source hold stayed lit")
	}

	env.in.down = [3]bool{}
	pump()
	if inv.ghost.container.GetWidget().IsVisible() || inv.hangFrame.GetWidget().IsVisible() {
		t.Error("release left the drag chrome up")
	}
}

// TestInventoryDropOnEmptySpace: a release that lands outside every hold's
// section (the title bar here) is a no-op, like a release on the source.
func TestInventoryDropOnEmptySpace(t *testing.T) {
	env, _ := newTestEnv(t)
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-solace"})
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

// TestInventoryToolbarHasNoMoveButtons: the three move buttons are gone
// (the drag replaced them) and Sell stays, greyed until a docked
// selection.
func TestInventoryToolbarHasNoMoveButtons(t *testing.T) {
	env, _ := newTestEnv(t)
	inv := NewInventory(env)
	ui := newWindowUI(t, env, inv.Win())
	uitest.PumpWindow(t, ui, inv.Sync, 1024, 700)

	toolbar := inv.body.Children()[1].(*widget.Container)
	var labels []string
	for _, ch := range toolbar.Children() {
		btn, ok := ch.(*widget.Button)
		if !ok {
			continue
		}
		labels = append(labels, btn.Text().Label)
	}
	if len(labels) != 2 || labels[0] != "Tiles" || labels[1] != "Sell" {
		t.Fatalf("toolbar = %v, want Tiles + Sell only (the move buttons are gone)", labels)
	}
	if !inv.sell.GetWidget().Disabled {
		t.Error("Sell armed with no selection")
	}
	clickRow(ui, inv.cargo, 0)
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-solace"})
	uitest.PumpWindow(t, ui, inv.Sync, 1024, 700)
	if inv.sell.GetWidget().Disabled {
		t.Error("Sell still greyed with a docked selection")
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
