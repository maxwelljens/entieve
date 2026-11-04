package screens

import (
	"image"
	"slices"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// The board scaffold's pins (CONTEXT.md "Board scaffold"): the pane drives
// head, sections, table, rail and verb from one model, the search machinery
// keeps each window's policy, and every page rebuilds only when its model
// moves. The stale-key class is the point: a displayed input outside the model
// would leave the card behind, so the pins walk the inputs.

// boardTestModel is the rebuild rule's fixture: a comparable model with the
// window's own change detector.
type boardTestModel struct{ N int }

func (m boardTestModel) same(o boardTestModel) bool { return m.N == o.N }

// TestBoardRebuildRule: the pane rebuilds on the model's move and never on a
// frame that repeats it (the Station board's rule, shared).
func TestBoardRebuildRule(t *testing.T) {
	var prev boardTestModel
	builds := 0
	rebuild := func(boardTestModel) { builds++ }

	boardSync(&prev, boardTestModel{N: 1}, rebuild)
	boardSync(&prev, boardTestModel{N: 1}, rebuild)
	boardSync(&prev, boardTestModel{N: 1}, rebuild)
	boardSync(&prev, boardTestModel{N: 1}, rebuild)
	if builds != 1 {
		t.Fatalf("four frames of one model rebuilt %d times, want the first frame's one", builds)
	}

	boardSync(&prev, boardTestModel{N: 2}, rebuild)
	boardSync(&prev, boardTestModel{N: 2}, rebuild)
	if builds != 2 {
		t.Fatalf("one move plus a repeat rebuilt %d times in total, want exactly one more (2)", builds)
	}
	if prev.N != 2 {
		t.Errorf("stored model = %+v, want the moved one", prev)
	}
}

// TestBoardSearchKeepsThePolicy: one machinery, two policies. The boards gate
// on a field CONTAINING the query (a subsequence scattered across two fields
// is not a hit); the market does not gate and gets the matched rune starts of
// the name for its highlight.
func TestBoardSearchKeepsThePolicy(t *testing.T) {
	rows := []searchRow{
		{id: "scattered", fields: []string{"alpha", "zeta"}}, // a..z spans two fields
		{id: "contained", fields: []string{"azur", "x"}},     // "az" sits in one field
	}
	fields := func(r searchRow) []string { return r.fields }

	gated := boardSearch("az", rows, fields, boardSearchSpec{Gate: true, Lead: true})
	if got := hitIDs(gated); !slices.Equal(got, []string{"contained"}) {
		t.Errorf("gated hits = %v, want only the field that contains the query", got)
	}
	for _, h := range gated {
		if h.Match != nil {
			t.Errorf("gated hit %q carries offsets, want none (the boards do not highlight)", h.Key)
		}
	}

	ungated := boardSearch("az", rows, fields, boardSearchSpec{Offsets: true})
	if got := hitIDs(ungated); !slices.Contains(got, "scattered") || !slices.Contains(got, "contained") {
		t.Errorf("ungated hits = %v, want both (the market ranks by score, never gates)", got)
	}

	// The offsets are rune starts of the ORIGINAL name, remapped through the
	// fold: "Źródło" folds to "Zrodlo", so the folded byte 1 ('r') is the
	// source byte 2.
	folded := []searchRow{{id: "zrodlo", fields: []string{"Źródło Ore", "Materials/Ore", ""}}}
	hits := boardSearch("zr", folded, fields, boardSearchSpec{Offsets: true})
	if len(hits) != 1 {
		t.Fatalf("folded name hits = %d, want one", len(hits))
	}
	if got, want := hits[0].Match, []int{0, 2}; !slices.Equal(got, want) {
		t.Errorf("matched starts = %v, want the original's %v", got, want)
	}
}

// searchRow is one search candidate in the machinery's pin.
type searchRow struct {
	id     string
	fields []string
}

// hitIDs reads the ranked keys.
func hitIDs(hits []boardHit[searchRow]) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.Key.id
	}
	return out
}

// TestBoardPaneScaffold drives one model's page through the pane: the head
// pins above the scroll column, the page holds the section bar, the table and
// its rows, the rail rides the column and the verb sits in the pinned bar.
func TestBoardPaneScaffold(t *testing.T) {
	env, _ := newChromeEnv(t)
	pane := newBoardPane(env, boardPaneSpec{OpenW: 264})
	pane.setHead(true, widgets.TradeableIcon(env.Defs(), "ore-cordite"),
		"Kerngrit Ore  ✓", "A description long enough to wrap inside the pane.")
	verb := theme.ActionButton("Refine", func() {})
	pane.bar.AddChild(verb)

	page := pane.beginPage()
	table := widgets.NewPlainTable(env, []widgets.PlainColumn{{}, {AlignRight: true}})
	rows := make([]widgets.PlainRow, 40)
	for i := range rows {
		rows[i] = widgets.PlainRow{Cells: []string{"Ore value", "7,200 €"}}
	}
	table.SetRows(rows)
	page.table("PROFIT", table)
	page.label("YIELD · most value first")
	page.stack(widgets.NewStackRow(env, widgets.TableRow{
		Cells: []string{"Vetril"},
		Stack: []widgets.RowLine{{Segs: []widgets.RowSeg{{Text: "20 per batch"}}}},
	}, env.Face(theme.StepSmall)))
	pane.done()

	win := widget.NewWindow(
		widget.WindowOpts.Contents(pane.box),
		widget.WindowOpts.MinSize(theme.Px(300), theme.Px(300)),
		widget.WindowOpts.Location(image.Rect(0, 0, theme.Px(400), theme.Px(400))),
	)
	ui := newWindowUI(t, env, win)
	frame := func() { uitest.PumpWindow(t, ui, pane.clampScroll, 1024, 700) }
	frame()
	frame()

	// The three zones in order: head, viewport, verb bar.
	head := pane.head.headBox.GetWidget().Rect
	view := pane.scroll.ViewRect()
	button := verb.GetWidget().Rect
	if head.Empty() || view.Empty() || button.Empty() {
		t.Fatalf("pane zones not laid out: head %v, viewport %v, verb %v", head, view, button)
	}
	if head.Max.Y > view.Min.Y {
		t.Errorf("head %v overlaps the page viewport %v", head, view)
	}
	if view.Max.Y > button.Min.Y {
		t.Errorf("page viewport %v overlaps the pinned verb %v", view, button)
	}
	if !button.In(pane.bar.GetWidget().Rect) {
		t.Errorf("verb %v sits outside its bar %v", button, pane.bar.GetWidget().Rect)
	}

	// The page's order: the PROFIT bar, the table, the caption, the row.
	kids := pane.page.Children()
	if len(kids) != 4 {
		t.Fatalf("page children = %d, want the bar, table, caption and row", len(kids))
	}
	if got, want := kids[1].GetWidget(), table.Container().GetWidget(); got != want {
		t.Errorf("page child 1 is not the table (the section bar leads)")
	}
	if kids[2] == nil || kids[3].GetWidget() == nil {
		t.Error("the caption or the rich row never reached the page")
	}

	// The head was first filled before the pane was laid out (the model's
	// fallback width); the measured inner width then moves the model, and the
	// rebuild re-wraps the prose to it.
	pane.setHead(true, widgets.TradeableIcon(env.Defs(), "ore-cordite"),
		"Kerngrit Ore  ✓", "A description long enough to wrap inside the pane.")
	frame()
	frame()
	if got, want := pane.head.headProse.MaxWidth, float64(pane.innerW()); got != want {
		t.Errorf("prose wrap = %v, want the pane's inner %v", got, want)
	}
	if got := pane.innerW(); got != view.Dx()+theme.Px(theme.ScrollGutter) {
		t.Errorf("pinned inner width = %d, want the viewport %d plus the gutter", got, view.Dx())
	}

	// A page opens at its top (a new model never keeps the old offset).
	if !pane.rail.Bar().GetWidget().IsVisible() {
		t.Error("rail hidden over an overflowing page")
	}
	pane.scroll.ScrollTop = 0.5
	pane.beginPage()
	if pane.scroll.ScrollTop != 0 {
		t.Errorf("scroll offset = %v after a rebuild, want the page's top", pane.scroll.ScrollTop)
	}

	// The in-page arithmetic (the market's right pane): the head rides the
	// page's inset, so the fallback gives the rail gutter up too.
	inPage := newBoardPane(env, boardPaneSpec{OpenW: 240, InPage: true})
	if got, want := inPage.innerW(), theme.Px(240-theme.ScrollGutter-2*theme.ContentsPad); got != want {
		t.Errorf("in-page inner width = %d, want the landing width less the gutter and both pads %d", got, want)
	}
}

// TestManufacturingInspectorRebuildsOnce: an idle frame keeps the card's row
// trees and one displayed input's move rebuilds them exactly once.
func TestManufacturingInspectorRebuildsOnce(t *testing.T) {
	f, env, ui, _, frame := boardEnv(t)
	clickRow(ui, f.recipes, recipeRowFor(f, "mod-artillery-l"))
	frame()
	frame()
	if len(f.matsRows) == 0 {
		t.Fatal("no material rows")
	}
	first := f.matsRows[0].Root
	frame()
	frame()
	if f.matsRows[0].Root != first {
		t.Fatal("a frame at an unchanged model rebuilt the inspector")
	}

	// One have/need figure moves: one rebuild, then quiet again.
	giveItem(env.Sim, f.needs("rec-mod-artillery-l", 1)[0].Item, 1)
	frame()
	moved := f.matsRows[0].Root
	if moved == first {
		t.Fatal("a hold move did not rebuild the inspector")
	}
	frame()
	frame()
	if f.matsRows[0].Root != moved {
		t.Error("the frame after a move rebuilt again")
	}
}

// TestRefineryInspectorRebuildsOnce: the same pin over the yield bill, with
// the stock as the moving input.
func TestRefineryInspectorRebuildsOnce(t *testing.T) {
	r, env, ui, _, frame := refineryEnv(t)
	clickRow(ui, r.ores, oreRowFor(r, "ore-cordite"))
	frame()
	frame()
	if len(r.billRows) == 0 {
		t.Fatal("no yield rows")
	}
	first := r.billRows[0].Root
	frame()
	frame()
	if r.billRows[0].Root != first {
		t.Fatal("a frame at an unchanged model rebuilt the inspector")
	}

	giveItem(env.Sim, "ore-cordite", 100)
	frame()
	moved := r.billRows[0].Root
	if moved == first {
		t.Fatal("a stock move did not rebuild the inspector")
	}
	frame()
	frame()
	if r.billRows[0].Root != moved {
		t.Error("the frame after a move rebuilt again")
	}
}

// TestMarketSideRebuildsOnce: the right pane's in-place refresh obeys the same
// rule, read through My orders' row trees (the pane's scroll and rail keep
// their identity, so a rebuild never drops the player's offset).
func TestMarketSideRebuildsOnce(t *testing.T) {
	m, env, ui, frame := mktSearchEnv(t)
	clickRow(ui, m.tbl, 0)
	frame()
	if _, ok := env.Sim.PlaceOrder("cel-haven-rest", sim.OrderBuy, "item-stub-ore", 1, 10, sim.OwnerPlayer); !ok {
		// The showcase content may not sell the stub item here; any player
		// order in the catalog will do.
		placed := false
		for _, item := range m.items {
			if _, ok := env.Sim.PlaceOrder("cel-haven-rest", sim.OrderBuy, item, 1, 1, sim.OwnerPlayer); ok {
				placed = true
				break
			}
		}
		if !placed {
			t.Fatal("fixture: no player order could rest at the station")
		}
	}
	frame()
	frame()
	if len(m.mineRows) == 0 {
		t.Fatal("no My orders row for the placed order")
	}
	first := m.mineRows[0].Root
	frame()
	frame()
	if m.mineRows[0].Root != first {
		t.Fatal("a frame at an unchanged model rebuilt the pane")
	}

	// One more resting order moves the model: one rebuild, then quiet.
	for _, item := range m.items {
		if _, ok := env.Sim.PlaceOrder("cel-haven-rest", sim.OrderBuy, item, 1, 2, sim.OwnerPlayer); ok {
			break
		}
	}
	frame()
	moved := m.mineRows
	if len(moved) == 0 || len(moved) == 1 && moved[0].Root == first {
		t.Fatal("a new order did not rebuild the pane")
	}
	after := moved[0].Root
	frame()
	frame()
	if len(m.mineRows) == 0 || m.mineRows[0].Root != after {
		t.Error("the frame after a move rebuilt again")
	}
}

// TestPageModelsCarryEveryDisplayedInput pins each window's model to the
// figures its page draws: one displayed input moving must move the model, or
// the pane would keep the old card (the stale-key class the string keys had).
func TestPageModelsCarryEveryDisplayedInput(t *testing.T) {
	// The refinery: the batch's own output quantities (the old key's hole).
	rf := rfPageModel{
		HasSel: true, Ore: "ore-cordite", Ready: true, Stock: 100, Inner: 200,
		Outs: []rfOut{{Item: "min-ferrite", Qty: 35, Value: 210}, {Item: "min-nickel", Qty: 30, Value: 420}},
	}
	if !rf.same(rf) {
		t.Fatal("the refinery model differs from itself")
	}
	batch := rf
	batch.Outs = []rfOut{{Item: "min-ferrite", Qty: 35, Value: 210}, {Item: "min-nickel", Qty: 40, Value: 560}}
	if rf.same(batch) {
		t.Error("refinery: the batch's quantities are outside the model")
	}

	// The manufacturing inspector: the bill's have side, the priced walks,
	// the product's bid and the run's output.
	mfg := mfgPageModel{
		HasSel: true, Recipe: "rec-mod-artillery-l", Station: "cel-haven-rest",
		Inner: 200, Out: 1,
		Needs:  []mfgNeed{{Item: "min-ferrite", Need: 6, Have: 0, Value: 42}},
		Quotes: []mfgQuote{{Walk: 42, Priced: true}},
		Bid:    sim.Quote{Walk: 250000, Priced: true},
	}
	if !mfg.same(mfg) {
		t.Fatal("the manufacturing model differs from itself")
	}
	for name, move := range map[string]func(*mfgPageModel){
		"have":   func(m *mfgPageModel) { m.Needs = []mfgNeed{{Item: "min-ferrite", Need: 6, Have: 6, Value: 42}} },
		"walk":   func(m *mfgPageModel) { m.Quotes = []mfgQuote{{Walk: 43, Priced: true}} },
		"remote": func(m *mfgPageModel) { m.Quotes = []mfgQuote{{Walk: 42, Priced: true, Remote: true}} },
		"bid":    func(m *mfgPageModel) { m.Bid = sim.Quote{} },
		"output": func(m *mfgPageModel) { m.Out = 100 },
		"slots":  func(m *mfgPageModel) { m.SlotsFree = true },
		"width":  func(m *mfgPageModel) { m.Inner = 201 },
	} {
		next := mfg
		next.Needs, next.Quotes = slices.Clone(mfg.Needs), slices.Clone(mfg.Quotes)
		move(&next)
		if mfg.same(next) {
			t.Errorf("manufacturing: %q is outside the model", name)
		}
	}

	// The market's right pane: both order lists and the docked gate.
	mkt := mktPageModel{
		HasSel: true, Sel: "item-stub-ore", Inner: 200,
		Book: []sim.MarketStock{{Item: "item-stub-ore", Price: 100, Qty: 5, Side: sim.OrderSell}},
	}
	if !mkt.same(mkt) {
		t.Fatal("the market model differs from itself")
	}
	for name, move := range map[string]func(*mktPageModel){
		"book":  func(m *mktPageModel) { m.Book = append(m.Book, sim.MarketStock{Item: "item-stub-ore", Qty: 1}) },
		"mine":  func(m *mktPageModel) { m.Mine = []sim.MarketStock{{Item: "item-stub-ore", Qty: 1}} },
		"gated": func(m *mktPageModel) { m.Gated = true },
	} {
		next := mkt
		next.Book, next.Mine = slices.Clone(mkt.Book), slices.Clone(mkt.Mine)
		move(&next)
		if mkt.same(next) {
			t.Errorf("market: %q is outside the model", name)
		}
	}
}

// TestRefineryInspectorFollowsTheBatch is the stale-key class gone end to end:
// a batch whose output quantities move while the ore, the stock, the station
// and the book stand still rebuilds the card and draws the new amounts.
func TestRefineryInspectorFollowsTheBatch(t *testing.T) {
	r, env, ui, _, frame := refineryEnv(t)
	clickRow(ui, r.ores, oreRowFor(r, "ore-cordite"))
	frame()
	frame()

	before := r.billRows[0].Root
	def, ok := env.Content.Item("ore-cordite")
	if !ok || def == nil {
		t.Fatal("fixture: no kerngrit def")
	}
	// The session's own content copy: the ore's yield moves, so the batch's
	// amounts move and nothing the old key hashed does (same ore, same stock,
	// same book rows).
	def.RefineYield["min-nickel"] = 0.40
	frame()
	if r.billRows[0].Root == before {
		t.Fatal("the inspector kept the stale batch: the output quantities are not page inputs")
	}
	want := billOrder(r.batchOf("ore-cordite"))
	if len(want) != len(r.billRows) {
		t.Fatalf("bill rows = %d, want the batch's %d", len(r.billRows), len(want))
	}
	for i, o := range want {
		var texts []string
		collectTexts(r.billRows[i].Root, &texts)
		amount := theme.FormatInt(int64(o.Qty)) + " per batch"
		if !slices.Contains(texts, amount) {
			t.Errorf("bill row %d = %v, want the batch's %q", i, texts, amount)
		}
	}
	// The ore is unchanged (the fixture moved the yield, not the selection).
	if r.sel.ore != content.ItemID("ore-cordite") {
		t.Errorf("selection = %q after the yield change", r.sel.ore)
	}
}
