package screens

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// The manufacturing board's pins: the browse tree and its live search scope
// the recipe list (the market window's idiom), the inspector renders the
// selected recipe's material bill, the rows carry the value-share strip, and
// the board answers the three acceptance questions without a number being
// read (the research brief's yardstick, docs/research/manufacturing-ux-brief.md
// §4 "Shared acceptance test").

// boardEnv docks the showcase game at the patched industry station and
// builds the window there, so the sim's own craft and material reads are
// real. It returns the window, the session, its UI, the captured commands
// and the frame pump.
func boardEnv(t *testing.T) (*Manufacturing, *testEnv, *ebitenui.UI, *[]sim.Command, func()) {
	t.Helper()
	env, cmds := factoryEnv(t)
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: factoryStation})
	f := NewManufacturing(env)
	ui := newWindowUI(t, env, f.Win())
	frame := func() { uitest.PumpWindow(t, ui, f.Sync, 1024, 700) }
	frame()
	frame()
	return f, env, ui, cmds, frame
}

// treeRowFor is the display index of a browse row by path.
func treeRowFor(t *testing.T, f *Manufacturing, path string) int {
	t.Helper()
	for i, r := range f.tree.Rows {
		if k, ok := r.Data.(mfgNodeKey); ok && k.Path == path {
			return i
		}
	}
	t.Fatalf("no browse row for %q (rows %+v)", path, f.tree.Rows)
	return -1
}

// TestManufacturingBrowseTree: the tree opens on All (the whole catalog),
// lists the recipe products' groups, expands on a click and scopes the list
// to the branch's subtree; selecting a recipe highlights its branch back.
func TestManufacturingBrowseTree(t *testing.T) {
	f, env, ui, _, frame := boardEnv(t)

	// All is the opening selection and the whole catalog is listed.
	if got, want := len(f.recipes.Rows), len(env.Content.RecipesInOrder()); got != want {
		t.Fatalf("All lists %d recipes, want the whole catalog (%d)", got, want)
	}
	if sel, ok := f.tree.Selected(); !ok || sel.Data.(mfgNodeKey).Path != "" {
		t.Fatalf("opening tree selection = %+v, want the All root", sel.Data)
	}
	roots := map[string]bool{}
	for _, r := range f.tree.Rows {
		k := r.Data.(mfgNodeKey)
		if k.Path != "" && !strings.Contains(k.Path, "/") {
			roots[k.Path] = true
		}
	}
	for _, want := range []string{"Ammunition", "Modules", "Ships"} {
		if !roots[want] {
			t.Errorf("browse tree lacks the %q branch (%+v)", want, f.tree.Rows)
		}
	}

	// Expanding Ammunition shows its children; selecting it scopes the list
	// to exactly its subtree.
	clickRow(ui, f.tree, treeRowFor(t, f, "Ammunition"))
	frame()
	want := 0
	for _, rec := range env.Content.RecipesInOrder() {
		if g := f.group[rec.ID]; g == "Ammunition" || strings.HasPrefix(g, "Ammunition/") {
			want++
		}
	}
	if want == 0 {
		t.Fatal("fixture: no Ammunition recipes")
	}
	if got := len(f.recipes.Rows); got != want {
		t.Fatalf("Ammunition branch lists %d recipes, want its subtree's %d", got, want)
	}
	for _, r := range f.recipes.Rows {
		row := r.Data.(manufacturingRow)
		if g := f.group[row.recipe]; g != "Ammunition" && !strings.HasPrefix(g, "Ammunition/") {
			t.Errorf("Ammunition branch lists %q from %q", row.recipe, g)
		}
	}

	// The deeper branch narrows further (Cap cells is the single charge).
	clickRow(ui, f.tree, treeRowFor(t, f, "Ammunition/Cap cells"))
	frame()
	if got := len(f.recipes.Rows); got != 1 {
		t.Fatalf("Cap cells lists %d recipes, want the one charge", got)
	}

	// Selecting a recipe highlights its own branch back without moving the
	// branch scope.
	clickRow(ui, f.recipes, 0)
	frame()
	if !f.hasSel {
		t.Fatal("recipe click did not select")
	}
	if got, want := f.treeSel, f.group[f.sel.recipe]; got != want {
		t.Errorf("tree highlight = %q, want the selected recipe's group %q", got, want)
	}
	if f.branch != "Ammunition/Cap cells" {
		t.Errorf("branch = %q, want the Cap cells scope kept", f.branch)
	}
}

// TestManufacturingSearch: the query filters the selected branch live, over
// the product name, its family and its inputs, so a mineral finds every
// recipe that eats it; a query nobody matches leaves one dim no-match row.
func TestManufacturingSearch(t *testing.T) {
	f, env, _, _, frame := boardEnv(t)

	f.FocusSearch()
	f.search.Insert("ion")
	frame()
	if len(f.recipes.Rows) == 0 || len(f.recipes.Rows) >= len(env.Content.RecipesInOrder()) {
		t.Fatalf("query \"ion\" listed %d recipes, want a live subset", len(f.recipes.Rows))
	}
	names := map[content.ItemID]bool{}
	for _, r := range f.recipes.Rows {
		row, ok := r.Data.(manufacturingRow)
		if !ok {
			t.Fatalf("search row payload = %#v", r.Data)
		}
		rec, _ := env.Content.Recipes[row.recipe]
		names[rec.Product] = true
	}
	if !names["ammo-emp-s"] {
		t.Errorf("\"ion\" did not match the 40mm ion charge (got %v)", names)
	}

	// Ingredient search: QtyToolkit's pattern (A4 in the brief), the inverse
	// lookup a six-mineral economy needs.
	f.search.SelectAll()
	f.search.DeleteSelectedText()
	f.search.Insert("vetril")
	frame()
	if len(f.recipes.Rows) == 0 {
		t.Fatal("a mineral query matched no recipe")
	}
	for _, r := range f.recipes.Rows {
		row := r.Data.(manufacturingRow)
		rec, _ := env.Content.Recipes[row.recipe]
		eats := false
		for _, in := range rec.Inputs {
			if in.Item == "min-silex" {
				eats = true
			}
		}
		if !eats {
			t.Errorf("%q listed for \"vetril\" but does not eat it", row.recipe)
		}
	}

	// Clearing restores the branch view in canonical order.
	f.search.SelectAll()
	f.search.DeleteSelectedText()
	frame()
	if f.query != "" || len(f.recipes.Rows) != len(env.Content.RecipesInOrder()) {
		t.Fatalf("cleared query still filters: %q (%d rows)", f.query, len(f.recipes.Rows))
	}

	// No match: one dim row, no payload, nothing selected.
	f.FocusSearch()
	f.search.Insert("zzzznomatch")
	frame()
	if len(f.recipes.Rows) != 1 || f.recipes.Rows[0].Cells[0] != "No matches" {
		t.Fatalf("empty query rows = %+v, want the one no-match row", f.recipes.Rows)
	}
	if f.recipes.Rows[0].Data != nil || f.hasSel {
		t.Error("the no-match row reads as a recipe")
	}
}

// TestManufacturingInspectorBill: the inspector is driven by the selected
// recipe's own bill: one row per mineral, the cost driver first, each bar
// the have/need fraction clamped at one.
func TestManufacturingInspectorBill(t *testing.T) {
	f, env, ui, _, frame := boardEnv(t)

	// A three-input recipe renders three material rows.
	rec := env.Content.Recipes["rec-ship-husnek"]
	if rec == nil || len(rec.Inputs) != 3 {
		t.Fatalf("fixture: rec-ship-husnek with three inputs, got %+v", rec)
	}
	row := recipeRowFor(f, rec.Product)
	if row < 0 {
		t.Fatal("no barge recipe row")
	}
	clickRow(ui, f.recipes, row)
	frame()
	if got := len(f.matsRows); got != 3 {
		t.Fatalf("inspector rows = %d, want the recipe's 3 inputs", got)
	}
	if got := f.inspName.Label; got != env.TradeableName(rec.Product) {
		t.Errorf("inspector title = %q, want %q", got, env.TradeableName(rec.Product))
	}

	// Half the bill: every bar is have/need, and the numbers are the sim's
	// committed need.
	needs := f.needs(rec.ID, 1)
	for _, n := range needs {
		giveItem(env.Sim, n.Item, n.Need/2)
	}
	frame()
	total := 0.0
	for _, n := range needs {
		total += n.Value
	}
	for _, n := range f.needs(rec.ID, 1) {
		want := 0.0
		if n.Need > 0 {
			want = float64(n.Have) / float64(n.Need)
		}
		bars := materialRowBars(f.matRowDef(n, total, theme.DefaultPalette()))
		if len(bars) != 1 {
			t.Fatalf("material %q carries %d bars, want its have/need bar", n.Item, len(bars))
		}
		if d := bars[0].Bar - want; d > 1e-9 || d < -1e-9 {
			t.Errorf("%s bar = %v, want have/need %v", n.Item, bars[0].Bar, want)
		}
	}

	// A full hold clamps at 1 (the pin's "clamped"); the bill is then all
	// green and the verdict reads ready.
	for _, n := range f.needs(rec.ID, 1) {
		if n.Have < n.Need {
			giveItem(env.Sim, n.Item, n.Need-n.Have)
		}
	}
	frame()
	if got := len(f.matsRows); got != 3 {
		t.Fatalf("inspector rows after stocking = %d, want 3", got)
	}
	if got := f.inspVerdict.Label; got != "✓" {
		t.Errorf("stocked verdict = %q, want the tick", got)
	}
	for _, n := range f.needs(rec.ID, 1) {
		for _, b := range materialRowBars(f.matRowDef(n, total, theme.DefaultPalette())) {
			if b.Bar != 1 {
				t.Errorf("%s bar = %v with a full hold, want the clamped 1", n.Item, b.Bar)
			}
		}
	}
}

// materialRowBars is a material row's have/need bar segments (line by line,
// left to right).
func materialRowBars(r widgets.TableRow) []widgets.RowSeg {
	var out []widgets.RowSeg
	for _, line := range r.Stack {
		for _, s := range line.Segs {
			if s.HasBar {
				out = append(out, s)
			}
		}
	}
	return out
}

// TestManufacturingStripShares: a row's strip is the run's material value
// split per mineral in canonical id order: the widths sum to the strip's
// exact pixel width, the order never depends on the authored input order,
// and the tint is the have/need state.
func TestManufacturingStripShares(t *testing.T) {
	env, _ := factoryEnv(t)
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: factoryStation})
	f := NewManufacturing(env)
	p := theme.DefaultPalette()

	// rec-mod-artillery-l: five minerals, Sideron 55% of the run's value.
	needs := f.needs("rec-mod-artillery-l", 1)
	if len(needs) != 5 {
		t.Fatalf("artillery L bill = %d minerals, want 5", len(needs))
	}
	yield := []content.ItemID{"min-argon", "min-cobalt", "min-ferrite", "min-nickel", "min-silex"}
	for i, n := range needs {
		if n.Item != yield[i] {
			t.Fatalf("segment %d = %q, want the canonical %q", i, n.Item, yield[i])
		}
	}
	shares := make([]float64, len(needs))
	for i, s := range stripOf(needs, p) {
		shares[i] = s.Share
	}
	// share = base_price × need over the run's material value.
	wantValue := map[content.ItemID]float64{
		"min-argon": 390 * 58, "min-cobalt": 1030 * 22, "min-ferrite": 18490 * 6,
		"min-nickel": 1620 * 14, "min-silex": 670 * 34,
	}
	const total = 201680.0
	for i, n := range needs {
		if n.Value != wantValue[n.Item] {
			t.Errorf("%s value = %v, want base_price×need %v", n.Item, n.Value, wantValue[n.Item])
		}
		if got := shares[i] / total; got != n.Value/total {
			t.Errorf("%s share = %v, want its value's share %v", n.Item, got, n.Value/total)
		}
	}
	// The exact pixel split of the row's strip: whole pixels, sum == width,
	// the dominant mineral widest.
	widths := widgets.ShareWidths(theme.Px(mfgStripW), shares)
	if got, want := widths, []int{14, 14, 71, 14, 15}; !reflect.DeepEqual(got, want) {
		t.Errorf("strip widths = %v, want the exact %v", got, want)
	}
	sum := 0
	for _, w := range widths {
		sum += w
	}
	if sum != theme.Px(mfgStripW) {
		t.Errorf("strip widths sum to %d, want the %d px strip", sum, theme.Px(mfgStripW))
	}
	if widths[2] != slicesMax(widths) {
		t.Errorf("widest segment = %d px, want the ferrite segment %d", widths[2], slicesMax(widths))
	}

	// State rides the tint, never the width: an empty hold is all short.
	for _, s := range stripOf(needs, p) {
		if s.Tint != p.Danger {
			t.Errorf("empty-hold segment tint = %v, want Danger", s.Tint)
		}
	}
	for _, n := range needs {
		giveItem(env.Sim, n.Item, n.Need)
	}
	for _, s := range stripOf(f.needs("rec-mod-artillery-l", 1), p) {
		if s.Tint != p.Good {
			t.Errorf("stocked segment tint = %v, want Good", s.Tint)
		}
	}

	// The authored input order never moves a segment: two fixture recipes
	// with the same minerals written in opposite order render one strip.
	fixtureRecipe(t, env, "rec-strip-a", &content.RecipeDef{
		Product: "ammo-emp-s", Qty: 1, Family: content.FamilyCharges, Time: 1,
		Inputs: []content.RecipeInput{{Item: "min-ferrite", Qty: 100}, {Item: "min-nickel", Qty: 10}},
	})
	fixtureRecipe(t, env, "rec-strip-b", &content.RecipeDef{
		Product: "ammo-emp-s", Qty: 1, Family: content.FamilyCharges, Time: 1,
		Inputs: []content.RecipeInput{{Item: "min-nickel", Qty: 10}, {Item: "min-ferrite", Qty: 100}},
	})
	a, b := stripOf(f.needs("rec-strip-a", 1), p), stripOf(f.needs("rec-strip-b", 1), p)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("authored order moved the strip:\n a = %+v\n b = %+v", a, b)
	}
}

// slicesMax is the largest entry of a non-empty int slice.
func slicesMax(v []int) int {
	m := v[0]
	for _, n := range v[1:] {
		if n > m {
			m = n
		}
	}
	return m
}

// TestManufacturingVerdict: the row's verdict is the sim's start gate said in
// one glyph, a tick when Start would accept and a cross when it would refuse,
// the tint the state; no word and no count ride it (the bill names what is
// short).
func TestManufacturingVerdict(t *testing.T) {
	p := theme.DefaultPalette()
	if got := verdictGlyph(true); got != "✓" {
		t.Errorf("ready glyph = %q, want the tick", got)
	}
	if got := verdictGlyph(false); got != "×" {
		t.Errorf("blocked glyph = %q, want the cross", got)
	}

	// Live: the row flips at have >= need, and only there; the glyph is the
	// only verdict text and its tint carries the state.
	f, env, _, _, frame := boardEnv(t)
	rec := env.Content.Recipes["rec-ammo-emp-s"]
	row := recipeRowFor(f, rec.Product)
	need := env.Sim.JobMaterials(factoryStation, rec.ID, 1)
	for _, m := range need {
		giveItem(env.Sim, m.Item, m.Qty-1) // one short everywhere
	}
	frame()
	if got := recipeVerdict(f.recipes.Rows[row]); got != "×" {
		t.Errorf("one-short verdict = %q, want the cross", got)
	}
	if got := recipeVerdictTint(f.recipes.Rows[row]); got != p.Danger {
		t.Errorf("one-short verdict tint = %v, want Danger", got)
	}
	for _, m := range need {
		giveItem(env.Sim, m.Item, 1)
	}
	frame()
	if got := recipeVerdict(f.recipes.Rows[row]); got != "✓" {
		t.Errorf("exact-stock verdict = %q, want the tick at have == need", got)
	}
	if got := recipeVerdictTint(f.recipes.Rows[row]); got != p.Good {
		t.Errorf("exact-stock verdict tint = %v, want Good", got)
	}
}

// TestManufacturingStartGateNames: a greyed Start carries no hint line (the
// grey is the gate), the row and inspector verdicts are the cross, and the
// button itself opens nothing.
func TestManufacturingStartGateNames(t *testing.T) {
	f, env, ui, cmds, frame := boardEnv(t)
	rec := env.Content.Recipes["rec-mod-artillery-l"]
	clickRow(ui, f.recipes, recipeRowFor(f, rec.Product))
	frame()

	if !f.startBtn.GetWidget().Disabled {
		t.Fatal("Start enabled with an empty hold")
	}
	if got := f.inspVerdict.Label; got != "×" {
		t.Errorf("blocked inspector verdict = %q, want the cross", got)
	}
	for _, lbl := range inspectorLabels(f) {
		if strings.Contains(strings.ToLower(lbl), "start:") {
			t.Errorf("inspector renders a gate hint %q, want none", lbl)
		}
	}
	uitest.FireButton(ui, f.startBtn)
	if len(*cmds) != 0 || env.QtyWindow().IsOpen() {
		t.Fatal("a greyed Start opened the picker")
	}

	// Stocking exactly the bill arms the button and flips the glyph.
	for _, n := range f.needs(rec.ID, 1) {
		giveItem(env.Sim, n.Item, n.Need)
	}
	frame()
	if got := f.inspVerdict.Label; got != "✓" {
		t.Errorf("stocked verdict = %q, want the tick", got)
	}
	if f.startBtn.GetWidget().Disabled {
		t.Fatal("Start still greyed with the whole bill stocked")
	}
}

// TestManufacturingAcceptanceRead is the research brief's yardstick as a
// pin: without reading a number, the board says (1) which recipes are ready
// (the row verdict glyphs), (2) what blocks the rest (the cross and the
// inspector bill's Danger rows) and (3) which mineral dominates the cost (the
// widest strip segment is the inspector's first row, and it is Sideron at
// 55%).
func TestManufacturingAcceptanceRead(t *testing.T) {
	f, env, ui, _, frame := boardEnv(t)

	// Nothing stocked: every recipe short, the artillery bill blocked,
	// every material row in the Danger tint.
	art := recipeRowFor(f, "mod-artillery-l")
	clickRow(ui, f.recipes, art)
	frame()
	if got := f.inspVerdict.Label; got != "×" {
		t.Errorf("(1) verdict = %q, want the cross", got)
	}
	short := 0
	for _, n := range f.needs("rec-mod-artillery-l", 1) {
		if n.Have < n.Need {
			short++
		}
	}
	if short == 0 {
		t.Fatal("fixture: the artillery bill is stocked")
	}
	for _, r := range f.matsRows {
		if _, h := r.Root.PreferredSize(); h == 0 {
			t.Error("(2) material row never measured")
		}
	}

	// (3) The cost driver is the wide segment and the first material row.
	needs := f.needs("rec-mod-artillery-l", 1)
	shares := make([]float64, len(needs))
	for i, s := range stripOf(needs, theme.DefaultPalette()) {
		shares[i] = s.Share
	}
	widths := widgets.ShareWidths(theme.Px(mfgStripW), shares)
	wide := 0
	for i, w := range widths {
		if w > widths[wide] {
			wide = i
		}
	}
	if needs[wide].Item != "min-ferrite" {
		t.Errorf("(3) widest segment = %q, want Sideron", needs[wide].Item)
	}
	if got := f.inspValue.Label; !strings.Contains(got, "201,680") {
		t.Errorf("value line = %q, want the run's material value", got)
	}

	// (1)+(2) together: stock the bill and the same row and inspector flip
	// to the tick.
	for _, n := range needs {
		giveItem(env.Sim, n.Item, n.Need)
	}
	frame()
	if got := recipeVerdict(f.recipes.Rows[art]); got != "✓" {
		t.Errorf("(1) stocked row verdict = %q, want the tick", got)
	}
	if got := f.inspVerdict.Label; got != "✓" {
		t.Errorf("(1) stocked inspector verdict = %q, want the tick", got)
	}
}

// profitValue is one profit-table row's value cell ("" when the row is gone).
func profitValue(f *Manufacturing, label string) string {
	for _, r := range f.profit.RowsForTest() {
		if len(r.Cells) == 2 && r.Cells[0] == label {
			return r.Cells[1]
		}
	}
	return ""
}

// inspectorLabels is every text the inspector draws (the verdict surfaces a
// rendered-text sweep reads).
func inspectorLabels(f *Manufacturing) []string {
	out := []string{f.inspName.Label, f.inspVerdict.Label, f.inspValue.Label, f.matsLbl.Label}
	for _, r := range f.profit.RowsForTest() {
		out = append(out, r.Cells...)
	}
	return out
}

// TestManufacturingProfitTable pins the inspector's profit table to the
// content's own numbers: one run's material bill and craft fee against the
// product's base-price sale value, the total, the profit and its margin, and
// the formulas both ways (a base_price edit must fail here).
func TestManufacturingProfitTable(t *testing.T) {
	f, env, ui, _, frame := boardEnv(t)
	rec := env.Content.Recipes["rec-mod-artillery-l"]
	clickRow(ui, f.recipes, recipeRowFor(f, rec.Product))
	frame()

	// The fixture's figures, read from the content: 201,680 € of minerals,
	// a 6,050 € fee, the module's 355,000 € base price at one unit.
	needs := f.needs(rec.ID, 1)
	mats := 0.0
	for _, n := range needs {
		mats += n.Value
	}
	fee := env.JobFee(rec.ID, 1)
	price := env.Content.Modules["mod-artillery-l"].Price
	sale := price * float64(rec.Qty)

	for _, c := range []struct{ label, want string }{
		{"Materials", "201,680 €"},
		{"Craft fee", "6,050 €"},
		{"Total cost", "207,730 €"},
		{"Sale price", "355,000 €"},
		{"Profit", "+147,270 €"},
		{"Margin", "41%"},
	} {
		if got := profitValue(f, c.label); got != c.want {
			t.Errorf("profit row %q = %q, want %q", c.label, got, c.want)
		}
	}
	// The formulas both ways, on the same content read.
	if got, want := profitValue(f, "Total cost"), mfgMoney(mats+fee); got != want {
		t.Errorf("total = %q, want materials + fee %q", got, want)
	}
	if got, want := profitValue(f, "Sale price"), mfgMoney(price*float64(rec.Qty)); got != want {
		t.Errorf("sale = %q, want base price × qty %q", got, want)
	}
	if got, want := profitValue(f, "Profit"), mfgSigned(sale-(mats+fee)); got != want {
		t.Errorf("profit = %q, want sale - total %q", got, want)
	}
	if profit := sale - (mats + fee); profit < 0 {
		t.Errorf("fixture: profit is negative (%v), want the positive pin", profit)
	}
	// The profit row carries the state tint, not colour alone (the sign is
	// in the string).
	var profitRow widgets.PlainRow
	for _, r := range f.profit.RowsForTest() {
		if len(r.Cells) == 2 && r.Cells[0] == "Profit" {
			profitRow = r
		}
	}
	if got := profitRow.Tint; got != theme.DefaultPalette().Good {
		t.Errorf("profit tint = %v, want Good", got)
	}
}

// TestManufacturingIconSizes pins the owner's icon bindings here: the recipe
// row's product icon is the market list's item icon size, the material bill's
// mineral icon is three times the inline row icon it opened with, and the bill
// row grows with it while staying inside the pane (the market head's hero-size
// match is pinned in TestMarketItemHeadLeads).
func TestManufacturingIconSizes(t *testing.T) {
	if mktRowIcon != theme.IconButton {
		t.Fatalf("market row icon = %d, want the 20 px button box", mktRowIcon)
	}
	if want := 3 * theme.IconInline; mfgMatIcon != want {
		t.Errorf("bill icon = %d, want three times the inline %d (%d)", mfgMatIcon, theme.IconInline, want)
	}

	f, _, ui, _, frame := boardEnv(t)
	clickRow(ui, f.recipes, recipeRowFor(f, "mod-artillery-l"))
	frame()
	frame()

	row := recipeRowFor(f, "mod-artillery-l")
	if got := f.recipes.Rows[row].IconSize; got != mktRowIcon {
		t.Errorf("recipe row icon size = %d, want the market list's %d", got, mktRowIcon)
	}
	if len(f.matsRows) == 0 {
		t.Fatal("no material rows")
	}
	if got := f.matsRows[0].Icon.Widget().Image.Bounds().Dx(); got != theme.Px(mfgMatIcon) {
		t.Errorf("bill icon = %d px, want the %d px box", got, theme.Px(mfgMatIcon))
	}
	bill := f.matsRows[0].Root.GetWidget().Rect
	if bill.Dy() <= theme.Px(mfgMatIcon) {
		t.Errorf("bill row = %d px tall, want it grown past the %d px icon",
			bill.Dy(), theme.Px(mfgMatIcon))
	}
	if view := f.matsScroll.ViewRect(); bill.Dx() > view.Dx() {
		t.Errorf("bill row %d px wide overflows the %d px viewport", bill.Dx(), view.Dx())
	}
}

// TestManufacturingVerdictTextSweep: neither verdict surface spells out the
// state any more — no "ready", no "short", only the glyphs (the bill's Danger
// rows still say which mineral is short by name).
func TestManufacturingVerdictTextSweep(t *testing.T) {
	f, env, ui, _, frame := boardEnv(t)
	rec := env.Content.Recipes["rec-mod-artillery-l"]
	clickRow(ui, f.recipes, recipeRowFor(f, rec.Product))
	frame()
	texts := inspectorLabels(f)
	for _, r := range f.recipes.Rows {
		for _, line := range r.Stack {
			for _, s := range line.Segs {
				texts = append(texts, s.Text)
			}
		}
	}
	for _, s := range texts {
		low := strings.ToLower(s)
		if strings.Contains(low, "ready") || strings.Contains(low, "short") {
			t.Errorf("verdict surface still renders %q", s)
		}
	}
	// The short minerals are still named where they matter: the bill rows
	// keep their name cell, their art and the Danger tint.
	short := 0
	for _, n := range f.needs(rec.ID, 1) {
		if n.Have >= n.Need {
			continue
		}
		short++
		row := f.matRowDef(n, 1, theme.DefaultPalette())
		if row.Cells[0] == "" || row.Icons[0] == "" {
			t.Errorf("bill row for %q lost its name or art", n.Item)
		}
		danger := false
		for _, b := range materialRowBars(row) {
			if b.Tint == theme.DefaultPalette().Danger {
				danger = true
			}
		}
		if !danger {
			t.Errorf("bill row for %q is not Danger-tinted", n.Item)
		}
	}
	if short == 0 {
		t.Fatal("fixture: the artillery bill is stocked")
	}
}
