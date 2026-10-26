package screens

import (
	"image"
	"math"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// kitFrame pumps one full kit frame with a panel's Sync (the game's order).
func kitFrame(t *testing.T, k *Kit, panels []widgets.Panel, w, h int) {
	t.Helper()
	k.Sync()
	for _, p := range panels {
		p.Sync()
	}
	if err := k.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	k.Draw(ebiten.NewImage(w, h))
}

// TestQtyPickerFreeflowClamps: the freeflow surfaces offer 1..qtyMax in
// single steps — a drag to either end lands on the end, a programmatic
// value past it clamps, and the picker never proposes 0 or a negative.
func TestQtyPickerFreeflowClamps(t *testing.T) {
	env, _ := newTestEnv(t)
	inv := NewInventory(env)
	ui := newQtyUI(t, env, inv.Win())
	q := env.QtyWindow()
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-solace"})
	pumpQty(t, ui, env, inv.Sync, 1024, 700)
	clickRow(ui, inv.cargo, 0)
	pumpQty(t, ui, env, inv.Sync, 1024, 700)
	inv.sellClicked()
	pumpQty(t, ui, env, inv.Sync, 1024, 700)
	if !q.IsOpen() {
		t.Fatal("Sell opened no picker")
	}
	if got, want := q.TitleText(), env.TradeableName(inv.sel)+" — Sell"; got != want {
		t.Errorf("title = %q, want %q", got, want)
	}

	// Slider drags: the ends are exact, and the readout follows.
	qtyClick(t, ui, env, inv.Sync, qtyMax)
	if got := q.Value(); got != qtyMax {
		t.Errorf("drag to the right end = %d, want %d", got, qtyMax)
	}
	if got := q.DetailText(); !strings.HasPrefix(got, theme.FormatInt(qtyMax)+" × ") {
		t.Errorf("preview = %q, want the %d-unit trade", got, qtyMax)
	}
	qtyClick(t, ui, env, inv.Sync, 1)
	if got := q.Value(); got != 1 {
		t.Errorf("drag to the left end = %d, want 1", got)
	}

	// A value off the range clamps to it instead of proposing an impossible
	// quantity.
	for _, c := range []struct {
		in   float64
		want int
	}{{0, 1}, {-5, 1}, {999, qtyMax}, {1000, qtyMax}, {7.4, 7}, {7.6, 8}} {
		q.Slider().SetValue(c.in)
		pumpQty(t, ui, env, inv.Sync, 1024, 700)
		if got := q.Value(); got != c.want {
			t.Errorf("SetValue(%v) → %d, want %d", c.in, got, c.want)
		}
	}
}

// TestQtyPickerCancelWritesNothing: Cancel and the picker's own Esc close
// it with no intent and no memory: the next open seeds at 1 again.
func TestQtyPickerCancelWritesNothing(t *testing.T) {
	env, cmds := newTestEnv(t)
	inv := NewInventory(env)
	ui := newQtyUI(t, env, inv.Win())
	q := env.QtyWindow()
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-solace"})
	pumpQty(t, ui, env, inv.Sync, 1024, 700)
	clickRow(ui, inv.cargo, 0)
	pumpQty(t, ui, env, inv.Sync, 1024, 700)

	inv.sellClicked()
	pumpQty(t, ui, env, inv.Sync, 1024, 700)
	q.Slider().SetValue(9)
	pumpQty(t, ui, env, inv.Sync, 1024, 700)
	uitest.FireButton(ui, q.cancel)
	if q.IsOpen() {
		t.Error("Cancel left the picker open")
	}
	inv.sellClicked()
	pumpQty(t, ui, env, inv.Sync, 1024, 700)
	if got := q.Value(); got != 1 {
		t.Errorf("after Cancel the picker opened at %d, want the unseeded 1 (Cancel wrote no memory)", got)
	}
	q.Close()
	if len(*cmds) != 0 {
		t.Fatalf("Cancel enqueued %d commands, want none", len(*cmds))
	}
}

// TestQtyPickerMemorySeedsNextOpen: Confirm stores the quantity per
// surface, so the next open starts there (the retired ± tickers'
// persistence) and the refinery clamps it to the live ladder.
func TestQtyPickerMemorySeedsNextOpen(t *testing.T) {
	env, _ := newTestEnv(t)
	inv := NewInventory(env)
	ui := newQtyUI(t, env, inv.Win())
	q := env.QtyWindow()
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-solace"})
	pumpQty(t, ui, env, inv.Sync, 1024, 700)
	clickRow(ui, inv.cargo, 0)
	pumpQty(t, ui, env, inv.Sync, 1024, 700)

	inv.transfer("hangar")
	pumpQty(t, ui, env, inv.Sync, 1024, 700)
	q.Slider().SetValue(42)
	pumpQty(t, ui, env, inv.Sync, 1024, 700)
	uitest.FireButton(ui, q.Confirm())

	inv.transfer("hangar")
	pumpQty(t, ui, env, inv.Sync, 1024, 700)
	if got := q.Value(); got != 42 {
		t.Fatalf("second transfer picker opened at %d, want the confirmed 42", got)
	}
	// A different surface starts from its own memory (Sell has none here).
	uitest.FireButton(ui, q.cancel)
	inv.sellClicked()
	pumpQty(t, ui, env, inv.Sync, 1024, 700)
	if got := q.Value(); got != 1 {
		t.Errorf("sell picker opened at %d, want its own unseeded 1", got)
	}
}

// TestQtyPickerRefineLadder: the refinery's ladder is the whole 100-ore
// batches of the picked stack and nothing else — three rungs at 350 held,
// a clamped seed when the stock shrinks, and an empty ladder below one
// batch.
func TestQtyPickerRefineLadder(t *testing.T) {
	env, _ := newChromeEnv(t)
	r := NewRefinery(env)
	ui := newQtyUI(t, env, r.Win())
	q := env.QtyWindow()
	frame := func() { pumpQty(t, ui, env, r.Sync, 1024, 700) }
	giveItem(env.Sim, "ore-cordite", 290) // 350 held: batches of 100, 200, 300
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-haven-rest"})
	frame()
	var row int
	for i, tr := range r.ores.Rows {
		if d, ok := tr.Data.(refineryRow); ok && d.ore == "ore-cordite" {
			row = i
		}
	}
	clickRow(ui, r.ores, row)
	frame()
	if !q.IsOpen() {
		t.Fatal("selecting an ore opened no picker")
	}
	ticks := q.Slider().TickPositions()
	if len(ticks) != 3 {
		t.Fatalf("ladder ticks = %d, want 3 whole batches at 350 held", len(ticks))
	}
	// Every rung is a multiple of the batch: drag across the track and the
	// value only ever rests on 100 / 200 / 300.
	for v := qtyMax; v >= 1; v-- {
		q.Slider().SetValue(float64(v) * 3) // 300…3 units
		pumpQty(t, ui, env, r.Sync, 1024, 700)
		got := q.Value()
		if got%sim.RefineBatch != 0 || got < sim.RefineBatch || got > 300 {
			t.Fatalf("SetValue(%d) rested at %d, want a whole batch of 100/200/300", v*3, got)
		}
	}

	// The seed is the last confirmed rung, clamped to the live ladder.
	q.Slider().SetValue(300)
	pumpQty(t, ui, env, r.Sync, 1024, 700)
	uitest.FireButton(ui, q.Confirm())
	cargo, _ := env.Sim.Cargos.Get(env.Sim.Player())
	for i := range cargo.Stacks {
		if cargo.Stacks[i].Item == "ore-cordite" {
			cargo.Stacks[i].Qty = 50 // the holds drop to a sub-batch stock
		}
	}
	for i := range cargo.Ore {
		if cargo.Ore[i].Item == "ore-cordite" {
			cargo.Ore[i].Qty = 0
		}
	}
	clickRow(ui, r.ores, row)
	frame()
	if !q.IsOpen() {
		t.Fatal("the picker must still open below one batch")
	}
	if got := len(q.Slider().TickPositions()); got != 0 {
		t.Errorf("ladder ticks = %d, want none below one batch", got)
	}
	if !q.Confirm().GetWidget().Disabled {
		t.Error("Confirm enabled below one batch")
	}
	if got := q.DetailText(); !strings.Contains(got, "One batch needs 100 ore") {
		t.Errorf("preview = %q, want the one-batch message", got)
	}
}

// TestQtyPickerVerbPerSurface: the picker's Confirm carries the surface's
// own verb, so the popup says what it will do (and the transfer names its
// destination hold).
func TestQtyPickerVerbPerSurface(t *testing.T) {
	env, _ := newTestEnv(t)
	inv := NewInventory(env)
	ui := newQtyUI(t, env, inv.Win())
	q := env.QtyWindow()
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-solace"})
	pumpQty(t, ui, env, inv.Sync, 1024, 700)
	clickRow(ui, inv.cargo, 0)
	pumpQty(t, ui, env, inv.Sync, 1024, 700)
	name := env.TradeableName(inv.sel)
	for _, c := range []struct {
		open func()
		want string
	}{
		{func() { inv.transfer("hangar") }, name + " — To Hangar"},
		{func() { inv.sellClicked() }, name + " — Sell"},
	} {
		c.open()
		pumpQty(t, ui, env, inv.Sync, 1024, 700)
		if got := q.TitleText(); got != c.want {
			t.Errorf("title = %q, want %q", got, c.want)
		}
		q.Close()
	}
	// The freeflow surfaces say Qty, the runs surface says Runs.
	if got := q.qtyLbl.Label; got != "Qty" {
		t.Errorf("transfer row label = %q, want Qty", got)
	}
	env2, _ := factoryEnv(t)
	f := NewManufacturing(env2)
	env2.Sim.Docked.Set(env2.Sim.Player(), sim.Docked{At: factoryStation})
	ui2 := newQtyUI(t, env2, f.Win())
	f.Sync()
	clickRow(ui2, f.recipes, recipeRowFor(f, "ammo-emp-s"))
	env2.QtyWindow().Sync()
	if got := env2.QtyWindow().qtyLbl.Label; got != "Runs" {
		t.Errorf("runs row label = %q, want Runs", got)
	}
}

// TestQtyWindowFits: the picker's content fits the window it declares, so
// no line is clipped at the design size or at 1.5× — the order window's
// constant-with-a-fit-test rule.
func TestQtyWindowFits(t *testing.T) {
	defer theme.SetScale(1)
	for _, sc := range []float64{1, 1.5} {
		theme.SetScale(sc)
		env, _ := newTestEnv(t)
		inv := NewInventory(env)
		ui := newQtyUI(t, env, inv.Win())
		q := env.QtyWindow()
		env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-solace"})
		pumpQty(t, ui, env, inv.Sync, 1280, 720)
		clickRow(ui, inv.cargo, 0)
		pumpQty(t, ui, env, inv.Sync, 1280, 720)
		inv.transfer("hangar")
		pumpQty(t, ui, env, inv.Sync, 1280, 720)
		if !q.IsOpen() {
			t.Fatalf("scale %v: no picker up", sc)
		}
		win := q.Win().GetContainer().GetWidget().Rect
		if win.Dx() < theme.Px(qtyMinW) || win.Dy() < theme.Px(qtyMinH) {
			t.Errorf("scale %v: picker %v under its %d×%d floor", sc, win,
				theme.Px(qtyMinW), theme.Px(qtyMinH))
		}
		for _, w := range []*widget.Widget{
			q.info.GetWidget(), q.sliderRow.GetWidget(), q.detail.GetWidget(),
			q.confirm.GetWidget(), q.cancel.GetWidget(),
		} {
			r := w.Rect
			if r.Empty() || r.Min.X < win.Min.X || r.Max.X > win.Max.X ||
				r.Min.Y < win.Min.Y || r.Max.Y > win.Max.Y {
				t.Errorf("scale %v: row %v outside the picker %v (content clipped)", sc, r, win)
			}
		}
		// The readout never collides with the slider.
		if q.read.GetWidget().Rect.Min.X < q.Slider().GetWidget().Rect.Max.X {
			t.Errorf("scale %v: readout overlaps the slider", sc)
		}
		q.Close()
	}
}

// TestQtyPickerOnAKit: the kit owns the mount — OpenQty adds the window to
// the stack and centres it, Confirm closes and unmounts it, and Esc closes
// it BEFORE the host's Esc handling (MenuInput reports the frame owned, so
// the window beneath and the Esc menu never see it).
func TestQtyPickerOnAKit(t *testing.T) {
	k, cmds := newTestKit(t)
	ref := NewRefinery(k.Env())
	k.AddWindow(ref.Win())
	ref.Win().SetLocation(image.Rect(40, 40, 40+theme.Px(430), 40+theme.Px(430)))
	panels := []widgets.Panel{ref}
	kitFrame(t, k, panels, 1024, 700)
	kitFrame(t, k, panels, 1024, 700)

	giveItem(k.Sim, "ore-cordite", 100)
	k.Sim.Docked.Set(k.Sim.Player(), sim.Docked{At: "cel-haven-rest"})
	kitFrame(t, k, panels, 1024, 700)

	row := -1
	for i, tr := range ref.ores.Rows {
		if d, ok := tr.Data.(refineryRow); ok && d.ore == "ore-cordite" {
			row = i
		}
	}
	if row < 0 {
		t.Fatalf("no cordite row in %+v", ref.ores.Rows)
	}
	clickRow(k.ui, ref.ores, row)
	kitFrame(t, k, panels, 1024, 700)
	if !k.QtyOpen() {
		t.Fatal("the selection gesture mounted no picker on the kit")
	}
	win := k.QtyWindow()
	if _, mounted := k.mountOf(win); !mounted {
		t.Fatal("picker open but not mounted on the kit's stack")
	}
	if !win.Modal {
		t.Error("the picker window is not modal — a click outside would leak to the window beneath")
	}
	// Centred on the viewport.
	root := k.root.GetWidget().Rect
	r := win.GetContainer().GetWidget().Rect
	if dx := math.Abs(float64((r.Min.X + r.Max.X) - (root.Min.X + root.Max.X))); dx > 2 {
		t.Errorf("picker x-centre off by %v (picker %v, viewport %v)", dx/2, r, root)
	}

	// Esc cancels: MenuInput reports the frame owned, nothing is enqueued
	// and the panel window beneath is untouched.
	esc := SetQtyEscForTest(func() bool { return true })
	if !k.MenuInput() {
		t.Error("MenuInput did not latch the Esc frame while the picker was up")
	}
	esc()
	if k.QtyOpen() {
		t.Error("Esc did not cancel the picker")
	}
	kitFrame(t, k, panels, 1024, 700)
	if _, mounted := k.mountOf(win); mounted {
		t.Error("a cancelled picker stayed mounted")
	}
	if len(*cmds) != 0 {
		t.Fatalf("Esc enqueued %d commands, want none", len(*cmds))
	}
	if !ref.Win().GetContainer().GetWidget().IsVisible() {
		t.Error("Esc closed the window beneath the picker")
	}

	// Confirm on the kit emits the picker's intent and unmounts.
	clickRow(k.ui, ref.ores, row)
	kitFrame(t, k, panels, 1024, 700)
	if !k.QtyOpen() {
		t.Fatal("re-select opened no picker")
	}
	uitest.FireButton(k.ui, k.QtyWin().Confirm())
	kitFrame(t, k, panels, 1024, 700)
	if len(*cmds) != 1 {
		t.Fatalf("Confirm enqueued %d commands, want 1", len(*cmds))
	}
	if cmd, ok := (*cmds)[0].(sim.CmdRefine); !ok || cmd.Item != "ore-cordite" || cmd.Qty != 100 {
		t.Fatalf("cmd = %#v, want CmdRefine{ore-cordite 100}", (*cmds)[0])
	}
	if k.QtyOpen() {
		t.Error("Confirm left the picker open")
	}
}

// TestQtyPickerEverySurfaceIntent: one Confirm per surface emits exactly
// the intent the ± tickers used to, with the picked quantity — the picker
// adds no new command and no new payload shape.
func TestQtyPickerEverySurfaceIntent(t *testing.T) {
	// Refine.
	{
		env, cmds := newChromeEnv(t)
		r := NewRefinery(env)
		ui := newQtyUI(t, env, r.Win())
		giveItem(env.Sim, "ore-cordite", 40)
		env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-haven-rest"})
		pumpQty(t, ui, env, r.Sync, 1024, 700)
		row := -1
		for i, tr := range r.ores.Rows {
			if d, ok := tr.Data.(refineryRow); ok && d.ore == "ore-cordite" {
				row = i
			}
		}
		if row < 0 {
			t.Fatalf("no cordite row in %+v", r.ores.Rows)
		}
		clickRow(ui, r.ores, row)
		pumpQty(t, ui, env, r.Sync, 1024, 700)
		uitest.FireButton(ui, env.QtyWindow().Confirm())
		if len(*cmds) != 1 {
			t.Fatalf("refine: %d commands, want 1", len(*cmds))
		}
		if got, want := (*cmds)[0], (sim.Command)(sim.CmdRefine{Item: "ore-cordite", Qty: 100}); got != want {
			t.Errorf("refine: command = %#v, want %#v", got, want)
		}
	}
	// Start a job.
	{
		env, cmds := factoryEnv(t)
		f := NewManufacturing(env)
		ui := newQtyUI(t, env, f.Win())
		env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: factoryStation})
		rec := env.Content.Recipes["rec-ammo-emp-s"]
		for _, m := range env.Sim.JobMaterials(factoryStation, rec.ID, 1) {
			giveItem(env.Sim, m.Item, m.Qty)
		}
		pumpQty(t, ui, env, f.Sync, 1024, 700)
		clickRow(ui, f.recipes, recipeRowFor(f, "ammo-emp-s"))
		pumpQty(t, ui, env, f.Sync, 1024, 700)
		uitest.FireButton(ui, env.QtyWindow().Confirm())
		if len(*cmds) != 1 {
			t.Fatalf("start: %d commands, want 1", len(*cmds))
		}
		if got, want := (*cmds)[0], (sim.Command)(sim.CmdStartJob{Recipe: rec.ID, Runs: 1}); got != want {
			t.Errorf("start: command = %#v, want %#v", got, want)
		}
	}
	// Transfer and Sell on one docked inventory selection. (Buy is the
	// market toolbar and row menu, pinned in TestMarketGatingAndEnqueue.)
	{
		env, cmds := newTestEnv(t)
		inv := NewInventory(env)
		ui := newQtyUI(t, env, inv.Win())
		q := env.QtyWindow()
		env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-solace"})
		pumpQty(t, ui, env, inv.Sync, 1024, 700)
		clickRow(ui, inv.cargo, 0)
		pumpQty(t, ui, env, inv.Sync, 1024, 700)
		item := inv.sel
		for _, c := range []struct {
			open func()
			want sim.Command
		}{
			{func() { inv.transfer("hangar") }, sim.CmdTransfer{Item: item, Qty: 1, From: "cargo", To: "hangar"}},
			{func() { inv.sellClicked() }, sim.CmdSell{Item: item, Qty: 1}},
		} {
			c.open()
			pumpQty(t, ui, env, inv.Sync, 1024, 700)
			uitest.FireButton(ui, q.Confirm())
			if got := (*cmds)[len(*cmds)-1]; got != c.want {
				t.Errorf("command = %#v, want %#v", got, c.want)
			}
		}
		if got := q.req.Item; got != item {
			t.Errorf("picker target = %q, want the selected %q", got, item)
		}
	}
}

// TestQtyPickIsAHostIntent: QtyPick carries no sim command, so the one
// intent→command table leaves it to the host (a bare session's sink opens
// the picker; the sim never sees it).
func TestQtyPickIsAHostIntent(t *testing.T) {
	env, cmds := newTestEnv(t)
	if env.QtyWindow() == nil || env.QtyWindow().IsOpen() {
		t.Fatal("a fresh session shows the picker (or has none at all)")
	}
	env.Do(widgets.QtyPick{Mode: widgets.QtySell, Item: "item-stub-ore"})
	if !env.QtyWindow().IsOpen() {
		t.Error("the session sink did not open the picker")
	}
	if len(*cmds) != 0 {
		t.Fatalf("opening the picker enqueued %d commands, want none", len(*cmds))
	}
	if got := env.QtyWindow().Win().GetContainer().GetWidget().IsVisible(); !got {
		t.Error("a picker window hidden for a bare session")
	}
	// The order window is untouched by all of this.
	if env.Menu() == nil || env.InfoWindow() == nil {
		t.Error("session services lost")
	}
}
