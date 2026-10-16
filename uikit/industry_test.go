package uikit

import (
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// ---------------------------------------------------------------------------
// Factory panel fixtures. Content is read-only in production: the test
// session patches ITS OWN copy of the shipped content (the start station
// gains the industry service — shipped solace offers none, penumbra does).
// ---------------------------------------------------------------------------

const factoryStation = content.CelID("cel-haven-rest")

func factoryEnv(t *testing.T) (*testEnv, *[]sim.Command) {
	t.Helper()
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	var patched bool
	for _, cel := range c.Systems["sys-solace"].Celestials {
		if cel.ID == factoryStation && cel.Station != nil {
			cel.Station.Services = append(cel.Station.Services, "industry")
			patched = true
		}
	}
	if !patched {
		t.Fatal("fixture: cel-haven-rest missing from sys-solace")
	}
	wd, ok := c.World("world-showcase")
	if !ok {
		t.Fatal("world-showcase missing")
	}
	s, err := sim.New(c, wd, 42)
	if err != nil {
		t.Fatalf("sim.New: %v", err)
	}
	var cmds []sim.Command
	sess := NewEnv(c, s, func(cmd sim.Command) { cmds = append(cmds, cmd) })
	return &testEnv{Session: sess, Sim: s, Content: c}, &cmds
}

// giveItem stacks qty units of an item into the player's general cargo.
func giveItem(s *sim.Sim, item content.ItemID, qty int) {
	cargo, ok := s.Cargos.Get(s.Player())
	if !ok {
		return
	}
	for i := range cargo.Stacks {
		if cargo.Stacks[i].Item == item {
			cargo.Stacks[i].Qty += qty
			return
		}
	}
	cargo.Stacks = append(cargo.Stacks, content.CargoStack{Item: item, Qty: qty})
}

// recipeRowFor returns the recipe-table row index whose payload produces
// product (-1 when absent).
func recipeRowFor(f *Factory, product content.ItemID) int {
	for i, r := range f.recipes.rows {
		row, ok := r.Data.(factoryRow)
		if !ok {
			continue
		}
		if rec, ok := f.cv.Defs().Recipes[row.recipe]; ok && rec.Product == product {
			return i
		}
	}
	return -1
}

// TestFactoryPanelGatingAndEnqueue walks the panel like the market tests
// walk theirs: rows from the sim, visible-but-grayed Start while undocked
// / unstocked / over the slot cap, the stepper's have/need readout, and
// the two enqueued commands.
func TestFactoryPanelGatingAndEnqueue(t *testing.T) {
	env, cmds := factoryEnv(t)
	f := NewFactory(env)
	ui := newWindowUI(t, env, f.Win())
	frame := func() { pumpWindow(t, ui, f.Sync, 1024, 700) }
	frame()
	frame()

	// Recipe board: one row per shipped recipe, in the content's canonical
	// order, carrying the product name, family, per-run time and inputs.
	want := env.Content.RecipesInOrder()
	if len(f.recipes.rows) != len(want) {
		t.Fatalf("recipe rows = %d, want %d", len(f.recipes.rows), len(want))
	}
	for i, r := range f.recipes.rows {
		if len(r.Cells) != 4 {
			t.Fatalf("row %d has %d cells, want 4", i, len(r.Cells))
		}
		if r.Cells[0] != itemLabel(env, want[i].Product) {
			t.Fatalf("row %d = %q, want %q (canonical order)", i, r.Cells[0], want[i].Product)
		}
		if r.Cells[1] != want[i].Family {
			t.Errorf("row %d family = %q, want %q", i, r.Cells[1], want[i].Family)
		}
		if r.Icons[0] == "" {
			t.Errorf("row %d carries no product icon", i)
		}
		if !strings.HasSuffix(r.Cells[2], " s") {
			t.Errorf("row %d time = %q, want a per-run figure", i, r.Cells[2])
		}
	}
	// The charge row's inputs read have/need at the stepper quantity.
	chargeRow := recipeRowFor(f, "charge-emp")
	if chargeRow < 0 {
		t.Fatal("no charge-emp recipe row")
	}
	if !strings.Contains(f.recipes.rows[chargeRow].Cells[3], "Ferrite 0/720") {
		t.Errorf("inputs cell = %q, want the have/need readout", f.recipes.rows[chargeRow].Cells[3])
	}

	// Undocked: Start is visible but grayed, and a direct press enqueues
	// nothing (the market buttons' rule).
	if !f.start.GetWidget().Disabled {
		t.Fatal("Start enabled while undocked")
	}
	f.startClicked()
	if len(*cmds) != 0 {
		t.Fatalf("undocked start enqueued %d commands, want 0", len(*cmds))
	}

	// Docked at the (patched) industry station: still grayed — no
	// materials — but the craft line and slot readout light up.
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: factoryStation})
	frame()
	if !strings.Contains(f.craftLbl.Label, "charges L1") ||
		!strings.Contains(f.craftLbl.Label, "hulls L1") {
		t.Errorf("craft line = %q, want per-family levels", f.craftLbl.Label)
	}
	if f.jobsLbl.Label != "Jobs — 0/2 slots" {
		t.Errorf("jobs line = %q, want the 0/2 slot readout", f.jobsLbl.Label)
	}
	if !f.start.GetWidget().Disabled {
		t.Fatal("Start enabled with an empty hold")
	}

	// Stock the charge recipe and select its row: Start arms.
	rec := env.Content.Recipes["rec-charge-emp"]
	if rec == nil {
		t.Fatal("fixture: rec-charge-emp missing")
	}
	mats := env.Sim.JobMaterials(factoryStation, rec.ID, 1)
	if len(mats) == 0 {
		t.Fatal("fixture: no materials for one run")
	}
	for _, m := range mats {
		giveItem(env.Sim, m.Item, m.Qty)
	}
	clickRow(ui, f.recipes, chargeRow)
	frame()
	if !f.hasSel || f.sel.recipe != rec.ID {
		t.Fatalf("selection = %+v (hasSel=%v), want the charge recipe", f.sel, f.hasSel)
	}
	if f.start.GetWidget().Disabled {
		t.Fatal("Start still grayed with materials in the hold")
	}
	fireButton(ui, f.start)
	if len(*cmds) != 1 {
		t.Fatalf("start enqueued %d commands, want 1", len(*cmds))
	}
	start, ok := (*cmds)[0].(sim.CmdStartJob)
	if !ok || start.Recipe != rec.ID || start.Runs != 1 {
		t.Fatalf("cmd = %#v, want CmdStartJob{%s 1}", (*cmds)[0], rec.ID)
	}

	// Stepping to 10 runs re-prices the need: the same cell now quotes the
	// ten-run commit and Start grays out again (materials for 10 missing).
	fireButton(ui, f.plus)
	frame()
	if f.qty() != 10 {
		t.Fatalf("qty = %d, want 10", f.qty())
	}
	cell := f.recipes.rows[chargeRow].Cells[3]
	if !strings.Contains(cell, "/7,200") {
		t.Errorf("inputs cell at qty 10 = %q, want /7,200", cell)
	}
	if !f.start.GetWidget().Disabled {
		t.Fatal("Start enabled over the material budget")
	}
	f.startClicked()
	if len(*cmds) != 1 {
		t.Fatalf("grayed start enqueued a command (%d total)", len(*cmds))
	}

	// A real job (the agent seam): the job row appears, Collect stays
	// grayed until the clock runs it down, then it enqueues CmdCollectJob.
	if _, ok := env.Sim.StartJob(factoryStation, rec.ID, 1, sim.OwnerPlayer); !ok {
		t.Fatal("start refused with stocked materials")
	}
	frame()
	if len(f.jobs.rows) != 1 {
		t.Fatalf("job rows = %d, want 1", len(f.jobs.rows))
	}
	if f.jobs.rows[0].Cells[3] != "Running" {
		t.Errorf("job state cell = %q, want Running", f.jobs.rows[0].Cells[3])
	}
	if !f.collect.GetWidget().Disabled {
		t.Fatal("Collect enabled before the job row is selected")
	}
	// Select the job row: Collect stays grayed while the clock runs, then
	// arms the moment the job finishes.
	clickRow(ui, f.jobs, 0)
	if !f.collect.GetWidget().Disabled {
		t.Fatal("Collect enabled for a running job")
	}
	for i := 0; i < 6000 && f.collect.GetWidget().Disabled; i++ {
		env.Sim.Step()
		frame()
	}
	if f.collect.GetWidget().Disabled {
		t.Fatal("Collect never armed (job did not complete)")
	}
	if f.jobs.rows[0].Cells[3] != "Ready" || f.jobs.rows[0].Cells[2] != "100%" {
		t.Errorf("done job row = %+v, want Ready/100%%", f.jobs.rows[0].Cells)
	}
	fireButton(ui, f.collect)
	if len(*cmds) != 2 {
		t.Fatalf("collect enqueued %d commands total, want 2", len(*cmds))
	}
	collect, ok := (*cmds)[1].(sim.CmdCollectJob)
	if !ok || collect.ID == 0 {
		t.Fatalf("cmd = %#v, want CmdCollectJob{id}", (*cmds)[1])
	}
}

// TestFactoryRowsShowInfo: recipe and job rows carry the product ref —
// right-click opens the Show Info menu (plus the gated action entry) and
// double-click swaps the window.
func TestFactoryRowsShowInfo(t *testing.T) {
	env, cmds := factoryEnv(t)
	f := NewFactory(env)
	ui := newWindowUI(t, env, f.Win())
	frame := func() { pumpWindow(t, ui, f.Sync, 1024, 700) }
	frame()
	frame()

	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: factoryStation})
	frame()
	chargeRow := recipeRowFor(f, "charge-emp")
	if chargeRow < 0 {
		t.Fatal("no charge-emp recipe row")
	}

	// Double-click (two clicks inside the window) swaps Show Info onto the
	// product — the ref resolves through InfoRefOf's factoryRow case.
	ref, ok := env.InfoRefOf(f.recipes.rows[chargeRow].Data)
	if !ok || ref != ItemRef("charge-emp") {
		t.Fatalf("row ref = %v (%v), want item/charge-emp", ref, ok)
	}
	clickRow(ui, f.recipes, chargeRow)
	clickRow(ui, f.recipes, chargeRow)
	frame()
	if w := env.InfoWindow(); w == nil || !w.IsOpen() {
		t.Fatal("double-click did not open Show Info")
	} else if got, _ := w.Ref(); got != ref {
		t.Errorf("Show Info ref = %v, want %v", got, ref)
	}

	// Right-click menu: bold Show Info plus the action entry — Start here,
	// visible but gated (no materials stocked in this test).
	f.recipes.rowCts[chargeRow].GetWidget().MouseButtonClickedEvent.Fire(
		&widget.WidgetMouseButtonClickedEventArgs{Button: ebiten.MouseButtonRight})
	ui.Update()
	m := env.Menu()
	if !m.IsOpen() {
		t.Fatal("right-click opened no menu")
	}
	entries := m.Entries()
	if len(entries) != 3 || entries[0].Label != "Show Info" || !entries[0].Bold ||
		entries[1].Label != "" || !entries[1].Sep ||
		entries[2].Label != "Start" || !entries[2].Disabled {
		t.Fatalf("menu = %+v, want Show Info / sep / gated Start", entries)
	}
	if len(*cmds) != 0 {
		t.Fatalf("menu opened with %d commands queued", len(*cmds))
	}
}

// ---------------------------------------------------------------------------
// Show Info chain references (MANUFACTURED FROM / USED IN)
// ---------------------------------------------------------------------------

// TestInfoPagesCarryManufacturingChain pins the two new sections against
// the SHIPPED recipes: the charge page lists its mineral inputs as
// label-side ref rows with quantities, the mineral page links every
// product consuming it, hulls report family + base time, and defs no
// recipe touches get neither section.
func TestInfoPagesCarryManufacturingChain(t *testing.T) {
	env, _ := newChromeEnv(t)

	// Product page (charge): MANUFACTURED FROM with label-side links.
	page, ok := infoPageFor(env, ItemRef("charge-emp"))
	if !ok {
		t.Fatal("charge page did not resolve")
	}
	mf := sectionOf(page, "MANUFACTURED FROM")
	if mf == nil {
		t.Fatalf("charge page has no MANUFACTURED FROM (sections %v)", sectionLabels(page))
	}
	ferrite := mustRow(t, page, "MANUFACTURED FROM", "Ferrite")
	if ferrite.Value != "×720" {
		t.Errorf("Ferrite qty = %q, want ×720", ferrite.Value)
	}
	if ferrite.Ref == nil || *ferrite.Ref != ItemRef("min-ferrite") {
		t.Errorf("Ferrite row ref = %v, want a label-side link to item/min-ferrite", ferrite.Ref)
	}
	if ferrite.RefOnValue {
		t.Error("input rows must carry the link on the LABEL side")
	}
	if r := mustRow(t, page, "MANUFACTURED FROM", "Argon"); r.Value != "×60" {
		t.Errorf("Argon qty = %q, want ×60", r.Value)
	}
	if sectionOf(page, "USED IN") != nil {
		t.Error("charge page rendered USED IN (charges are never recipe inputs)")
	}

	// Input page (mineral): USED IN links every consuming product.
	page, ok = infoPageFor(env, ItemRef("min-ferrite"))
	if !ok {
		t.Fatal("mineral page did not resolve")
	}
	if sectionOf(page, "MANUFACTURED FROM") != nil {
		t.Error("mineral page rendered MANUFACTURED FROM (minerals are never products)")
	}
	ui := sectionOf(page, "USED IN")
	if ui == nil {
		t.Fatalf("mineral page has no USED IN (sections %v)", sectionLabels(page))
	}
	emp := rowIn(t, ui, "EMP Charge")
	if emp.Value != "×100" {
		t.Errorf("USED IN value = %q, want the product's per-run output ×100", emp.Value)
	}
	if emp.Ref == nil || *emp.Ref != ItemRef("charge-emp") {
		t.Errorf("USED IN ref = %v, want item/charge-emp", emp.Ref)
	}
	// The hull recipe consuming the same mineral is listed too (the
	// shipped barge hull is named Havíř).
	if r := rowIn(t, ui, "Havíř"); r.Ref == nil || *r.Ref != ShipRef("ship-barge") {
		t.Errorf("USED IN hull row = %+v, want a ship-barge link", r)
	}

	// Hull page: family + base time above the input links.
	page, ok = infoPageFor(env, ShipRef("ship-barge"))
	if !ok {
		t.Fatal("hull page did not resolve")
	}
	if r := mustRow(t, page, "MANUFACTURED FROM", "Family"); r.Value != "Hulls" {
		t.Errorf("family row = %+v, want Hulls", r)
	}
	if r := mustRow(t, page, "MANUFACTURED FROM", "Base time"); r.Value != "240.0 s" {
		t.Errorf("base time row = %+v, want 240.0 s", r)
	}
	if r := mustRow(t, page, "MANUFACTURED FROM", "Ferrite"); r.Value != "×44,700" {
		t.Errorf("input qty = %+v, want ×44,700", r)
	}
	if sectionOf(page, "USED IN") != nil {
		t.Error("hull page rendered USED IN (hulls are never recipe inputs)")
	}

	// Module page: manufactured from its minerals.
	page, ok = infoPageFor(env, ModuleRef("mod-emp-turret"))
	if !ok {
		t.Fatal("module page did not resolve")
	}
	if r := mustRow(t, page, "MANUFACTURED FROM", "Ferrite"); r.Value != "×13,750" {
		t.Errorf("module input qty = %+v, want ×13,750", r)
	}

	// No recipe touches it → neither section renders (never "N/A").
	page, ok = infoPageFor(env, ItemRef("loot-alloy-scrap"))
	if !ok {
		t.Fatal("loot page did not resolve")
	}
	if sectionOf(page, "MANUFACTURED FROM") != nil || sectionOf(page, "USED IN") != nil {
		t.Errorf("recipe-less def rendered chain sections: %v", sectionLabels(page))
	}
}

// rowIn returns a section row by label, failing when it is absent.
func rowIn(t *testing.T, s *infoSection, label string) infoRow {
	t.Helper()
	r, ok := rowOf(s, label)
	if !ok {
		t.Fatalf("section %q has no %q row (rows %v)", s.Label, label, s.Rows)
	}
	return r
}

// fixtureRecipe registers a recipe in the session's content (the shipped
// stream stays read-only; tests patch their own copy).
func fixtureRecipe(t *testing.T, env *testEnv, id content.RecipeID, def *content.RecipeDef) {
	t.Helper()
	def.ID = id
	if env.Content.Recipes == nil {
		env.Content.Recipes = map[content.RecipeID]*content.RecipeDef{}
	}
	env.Content.Recipes[id] = def
}

// TestInfoPageChainFixture: a fractional-input fixture recipe drives both
// sections end to end (label-side link, exact fractional quantity, the
// product link on the way back) — the shipped-recipe test above covers
// the shipped numbers; this one covers the fixture path and the fraction.
func TestInfoPageChainFixture(t *testing.T) {
	env, _ := newChromeEnv(t)
	part := fixtureItem(t, env, "item-fixture-part", &content.ItemDef{
		Name: "Fixture Part", Category: content.CatLoot, Volume: 1,
	})
	gadget := fixtureItem(t, env, "item-fixture-gadget", &content.ItemDef{
		Name: "Fixture Gadget", Category: content.CatCharge, Volume: 0.1,
	})
	fixtureRecipe(t, env, "rec-fixture-gadget", &content.RecipeDef{
		Product: gadget.ID, Qty: 7,
		Inputs: []content.RecipeInput{{Item: part.ID, Qty: 2.5}},
		Time:   12, Family: content.FamilyCharges,
	})

	page, ok := infoPageFor(env, ItemRef(gadget.ID))
	if !ok {
		t.Fatal("gadget page did not resolve")
	}
	r := mustRow(t, page, "MANUFACTURED FROM", "Fixture Part")
	if r.Value != "×2.5" {
		t.Errorf("fractional input = %q, want ×2.5 (truncating to ×2 would lie)", r.Value)
	}
	if r.Ref == nil || *r.Ref != ItemRef(part.ID) {
		t.Errorf("input ref = %v, want the part", r.Ref)
	}

	page, ok = infoPageFor(env, ItemRef(part.ID))
	if !ok {
		t.Fatal("part page did not resolve")
	}
	r = mustRow(t, page, "USED IN", "Fixture Gadget")
	if r.Value != "×7" {
		t.Errorf("product output = %q, want ×7", r.Value)
	}
	if r.Ref == nil || *r.Ref != ItemRef(gadget.ID) {
		t.Errorf("USED IN ref = %v, want the gadget", r.Ref)
	}
}

// TestInfoWindowChainRefSwap drives the window itself: the MANUFACTURED
// FROM rows are live ref rows — right-click → Show Info swaps onto the
// input, whose USED IN row double-clicks back to the product.
func TestInfoWindowChainRefSwap(t *testing.T) {
	env, _ := newChromeEnv(t)
	w := env.InfoWindow()
	if w == nil {
		t.Fatal("environment has no info window")
	}
	ui := newWindowUI(t, env, w.Win())
	frame := func() { pumpWindow(t, ui, w.Sync, 1024, 700) }
	frame()

	env.ShowInfo(ItemRef("charge-emp"))
	frame()
	if got, _ := w.Ref(); got != ItemRef("charge-emp") {
		t.Fatalf("window ref = %v, want the charge", got)
	}
	find := func(ref InfoRef) *InfoRefRow {
		for i := range w.RefRows() {
			if w.RefRows()[i].Ref == ref {
				return &w.RefRows()[i]
			}
		}
		return nil
	}
	inRow := find(ItemRef("min-ferrite"))
	if inRow == nil {
		t.Fatalf("charge page has no MANUFACTURED FROM ref row (rows %+v)", w.RefRows())
	}
	if inRow.Widget.Rect.Empty() {
		t.Fatal("chain ref row never laid out")
	}

	// Right-click → the Show Info menu → swap onto the input.
	inRow.Widget.MouseButtonClickedEvent.Fire(&widget.WidgetMouseButtonClickedEventArgs{
		Widget: inRow.Widget, Button: ebiten.MouseButtonRight,
	})
	ui.Update()
	if m := env.Menu(); !m.IsOpen() {
		t.Fatal("right-click on a chain row opened no menu")
	}
	pressMenuEntry(t, env, "Show Info")
	if got, _ := w.Ref(); got != ItemRef("min-ferrite") {
		t.Fatalf("window ref = %v, want item/min-ferrite after the swap", got)
	}
	if !w.IsOpen() {
		t.Fatal("window closed while swapping")
	}

	// The swapped page carries USED IN back to the product; double-click
	// its row to swap home.
	frame()
	back := find(ItemRef("charge-emp"))
	if back == nil {
		t.Fatalf("mineral page has no USED IN ref row (rows %+v)", w.RefRows())
	}
	click := func(rw *InfoRefRow) {
		t.Helper()
		rw.Widget.MouseButtonClickedEvent.Fire(&widget.WidgetMouseButtonClickedEventArgs{
			Widget: rw.Widget, Button: ebiten.MouseButtonLeft,
		})
		ui.Update()
	}
	click(back)
	if got, _ := w.Ref(); got != ItemRef("min-ferrite") {
		t.Fatalf("single click swapped the window (ref = %v)", got)
	}
	click(back)
	if got, _ := w.Ref(); got != ItemRef("charge-emp") {
		t.Fatalf("double-click ref = %v, want the charge back", got)
	}
}
