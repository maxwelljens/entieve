package screens

import (
	"image/color"
	"reflect"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// The refinery board's pins: the browse tree and its live search scope the ore
// list (the manufacturing board's idiom over ore groups and the minerals they
// yield), the rows carry the verdict glyph and the batch's value strip, and the
// inspector answers "refine this or sell it raw" off the docked book.

const (
	rfStation  = sim.StationID("cel-haven-rest") // shipped: market + refine
	rfNoRefine = sim.StationID("cel-halda")      // shipped: market + repair
)

// refineryEnv docks the showcase game at the refine station, stocks the three
// refinable ores and builds the window there. It returns the window, the
// session, its UI, the captured commands and the frame pump.
func refineryEnv(t *testing.T) (*Refinery, *testEnv, *ebitenui.UI, *[]sim.Command, func()) {
	t.Helper()
	env, cmds := newChromeEnv(t)
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: rfStation})
	for _, ore := range []content.ItemID{"ore-cordite", "ore-lucidite", "ore-sterngrit"} {
		giveItem(env.Sim, ore, 100)
	}
	r := NewRefinery(env)
	ui := newQtyUI(t, env, r.Win())
	frame := func() { pumpQty(t, ui, env, r.Sync, 1024, 700) }
	frame()
	frame()
	return r, env, ui, cmds, frame
}

// oreRowFor is the display index of an ore row (-1 when absent).
func oreRowFor(r *Refinery, ore content.ItemID) int {
	for i, tr := range r.ores.Rows {
		if d, ok := tr.Data.(refineryRow); ok && d.ore == ore {
			return i
		}
	}
	return -1
}

// oreRowOf is one ore's row (fails when the list does not carry it).
func oreRowOf(t *testing.T, r *Refinery, ore content.ItemID) widgets.TableRow {
	t.Helper()
	i := oreRowFor(r, ore)
	if i < 0 {
		t.Fatalf("no %q row (rows %+v)", ore, r.ores.Rows)
	}
	return r.ores.Rows[i]
}

// treeRowAt is the display index of a browse row by path.
func treeRowAt(t *testing.T, r *Refinery, path string) int {
	t.Helper()
	for i, tr := range r.tree.Rows {
		if k, ok := tr.Data.(browseNodeKey); ok && k.Path == path {
			return i
		}
	}
	t.Fatalf("no browse row for %q (rows %+v)", path, r.tree.Rows)
	return -1
}

// oreNames reads the ore list's payload order.
func oreNames(r *Refinery) []content.ItemID {
	var out []content.ItemID
	for _, tr := range r.ores.Rows {
		if d, ok := tr.Data.(refineryRow); ok {
			out = append(out, d.ore)
		}
	}
	return out
}

// oreVerdictText is one ore row's one-glance verdict glyph (line 1, second
// run).
func oreVerdictText(tr widgets.TableRow) string {
	if len(tr.Stack) == 0 || len(tr.Stack[0].Segs) < 2 {
		return ""
	}
	return tr.Stack[0].Segs[1].Text
}

// oreVerdictTint is that glyph's colour.
func oreVerdictTint(tr widgets.TableRow) color.RGBA {
	if len(tr.Stack) == 0 || len(tr.Stack[0].Segs) < 2 {
		return color.RGBA{}
	}
	return tr.Stack[0].Segs[1].Tint
}

// rfTitle is the inspector head's name line.
func rfTitle(r *Refinery) string {
	if r.head == nil {
		return ""
	}
	return r.head.headName.Label
}

// rfVerdict is the inspector head's trailing verdict glyph.
func rfVerdict(r *Refinery) string {
	switch title := rfTitle(r); {
	case strings.HasSuffix(title, "✓"):
		return "✓"
	case strings.HasSuffix(title, "×"):
		return "×"
	}
	return ""
}

// mathValue is one money-table row's value cell ("" when the row is gone).
func mathValue(r *Refinery, label string) string {
	for _, row := range r.math.RowsForTest() {
		if len(row.Cells) == 2 && row.Cells[0] == label {
			return row.Cells[1]
		}
	}
	return ""
}

// mathTint is one money-table row's tint.
func mathTint(r *Refinery, label string) color.RGBA {
	for _, row := range r.math.RowsForTest() {
		if len(row.Cells) == 2 && row.Cells[0] == label {
			return row.Tint
		}
	}
	return color.RGBA{}
}

// TestRefineryBrowseTree: the tree opens on All (every held ore), lists the
// ore groups, expands on a click and scopes the list to the branch's subtree;
// selecting an ore highlights its branch back without moving the scope.
func TestRefineryBrowseTree(t *testing.T) {
	env, _ := newChromeEnv(t)
	// Fixture: the shipped ores all sit in Materials/Ore, so the session's own
	// content copy splits one onto a second branch — a branch click must have
	// something to scope.
	env.Content.Items["ore-glacierite"].Group = "Materials/Ice"
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: rfStation})
	giveItem(env.Sim, "ore-cordite", 100)
	giveItem(env.Sim, "ore-glacierite", 100)
	r := NewRefinery(env)
	ui := newQtyUI(t, env, r.Win())
	frame := func() { pumpQty(t, ui, env, r.Sync, 1024, 700) }
	frame()
	frame()

	// All is the opening selection and both held ores are listed.
	sel, ok := r.tree.Selected()
	if !ok {
		t.Fatal("the tree opens with no selection")
	}
	if k := sel.Data.(browseNodeKey); k.Path != "" {
		t.Fatalf("opening tree selection = %q, want the All root", k.Path)
	}
	for _, want := range []content.ItemID{"ore-cordite", "ore-glacierite"} {
		if oreRowFor(r, want) < 0 {
			t.Fatalf("All lists %v, want the held %q", oreNames(r), want)
		}
	}
	if oreRowFor(r, "ore-lucidite") >= 0 {
		t.Fatalf("All lists an ore nobody holds %v", oreNames(r))
	}
	treeRowAt(t, r, "Materials")

	// Expanding Materials shows both ore groups; clicking the Ice branch
	// scopes the list to it.
	clickRow(ui, r.tree, treeRowAt(t, r, "Materials"))
	frame()
	for _, want := range []string{"Materials/Ice", "Materials/Ore"} {
		treeRowAt(t, r, want)
	}
	clickRow(ui, r.tree, treeRowAt(t, r, "Materials/Ice"))
	frame()
	if got := oreNames(r); !reflect.DeepEqual(got, []content.ItemID{"ore-glacierite"}) {
		t.Fatalf("Ice branch lists %v, want the one glacierite", got)
	}

	// The Ore branch lists the other and nothing more.
	clickRow(ui, r.tree, treeRowAt(t, r, "Materials/Ore"))
	frame()
	if oreRowFor(r, "ore-cordite") < 0 || oreRowFor(r, "ore-glacierite") >= 0 {
		t.Fatalf("Ore branch lists %v, want cordite (and the shipped sterngrit) only", oreNames(r))
	}

	// Selecting an ore highlights its own branch back and keeps the scope.
	clickRow(ui, r.ores, oreRowFor(r, "ore-cordite"))
	frame()
	if !r.hasSel {
		t.Fatal("the ore click did not select")
	}
	if got, want := r.treeSel, "Materials/Ore"; got != want {
		t.Errorf("tree highlight = %q, want the ore's group %q", got, want)
	}
	if r.branch != "Materials/Ore" {
		t.Errorf("branch = %q, want the Ore scope kept", r.branch)
	}
}

// TestRefinerySearch: the query filters the selected branch live, over the ore
// name, its group and the minerals it YIELDS (the A4 inverse lookup), and a
// query nobody matches leaves one dim no-match row.
func TestRefinerySearch(t *testing.T) {
	env, _ := newChromeEnv(t)
	// The same second-branch fixture as the tree pin: a query cannot leave the
	// selected branch.
	env.Content.Items["ore-glacierite"].Group = "Materials/Ice"
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: rfStation})
	for _, ore := range []content.ItemID{"ore-cordite", "ore-lucidite", "ore-sterngrit", "ore-glacierite"} {
		giveItem(env.Sim, ore, 100)
	}
	r := NewRefinery(env)
	ui := newQtyUI(t, env, r.Win())
	frame := func() { pumpQty(t, ui, env, r.Sync, 1024, 700) }
	frame()
	frame()

	// A mineral query finds every ore that yields it, and no other.
	r.FocusSearch()
	r.search.Insert("sideron")
	frame()
	got := oreNames(r)
	if len(got) == 0 {
		t.Fatal("a mineral query matched no ore")
	}
	for _, ore := range got {
		def, _ := env.Content.Item(ore)
		if _, ok := def.RefineYield["min-ferrite"]; !ok {
			t.Errorf("%q listed for \"Sideron\" but does not yield it", ore)
		}
	}
	for _, want := range []content.ItemID{"ore-cordite", "ore-sterngrit"} {
		if oreRowFor(r, want) < 0 {
			t.Errorf("\"Sideron\" missed %q (got %v)", want, got)
		}
	}
	if oreRowFor(r, "ore-lucidite") >= 0 || oreRowFor(r, "ore-glacierite") >= 0 {
		t.Errorf("\"Sideron\" listed an ore that yields none (got %v)", got)
	}

	// By ore name.
	r.search.SelectAll()
	r.search.DeleteSelectedText()
	r.search.Insert("kerngrit")
	frame()
	if got := oreNames(r); !reflect.DeepEqual(got, []content.ItemID{"ore-cordite"}) {
		t.Fatalf("name query lists %v, want the one cordite", got)
	}

	// The search scopes with the tree: the same mineral query in the Ice
	// branch lists nothing.
	r.search.SelectAll()
	r.search.DeleteSelectedText()
	clickRow(ui, r.tree, treeRowAt(t, r, "Materials"))
	frame()
	clickRow(ui, r.tree, treeRowAt(t, r, "Materials/Ice"))
	frame()
	r.FocusSearch()
	r.search.Insert("sideron")
	frame()
	if got := len(oreNames(r)); got != 0 {
		t.Fatalf("a query outside the branch listed %v", oreNames(r))
	}

	// No match: one dim row, no payload, no selection.
	r.search.SelectAll()
	r.search.DeleteSelectedText()
	r.search.Insert("zzzznomatch")
	frame()
	if len(r.ores.Rows) != 1 || r.ores.Rows[0].Cells[0] != "No matches" {
		t.Fatalf("empty query rows = %+v, want the one no-match row", r.ores.Rows)
	}
	if r.ores.Rows[0].Data != nil || r.hasSel {
		t.Error("the no-match row reads as an ore")
	}
}

// TestRefineryVerdict: the row's verdict is the refine gate in one glyph, a
// tick when Refine would accept and a cross when it would refuse, the tint the
// state; no word and no count ride it.
func TestRefineryVerdict(t *testing.T) {
	p := theme.DefaultPalette()
	if got := verdictGlyph(true); got != "✓" {
		t.Errorf("ready glyph = %q, want the tick", got)
	}
	if got := verdictGlyph(false); got != "×" {
		t.Errorf("blocked glyph = %q, want the cross", got)
	}

	r, env, ui, _, frame := refineryEnv(t)
	clickRow(ui, r.ores, oreRowFor(r, "ore-cordite"))
	frame()

	// Docked at a refine station with a whole batch: the tick, Good.
	if got := oreVerdictText(oreRowOf(t, r, "ore-cordite")); got != "✓" {
		t.Errorf("docked verdict = %q, want the tick", got)
	}
	if got := oreVerdictTint(oreRowOf(t, r, "ore-cordite")); got != p.Good {
		t.Errorf("docked verdict tint = %v, want Good", got)
	}
	if r.refineBtn.GetWidget().Disabled {
		t.Fatal("Refine grayed at a refine station with a whole batch")
	}
	if got := rfVerdict(r); got != "✓" {
		t.Errorf("inspector verdict = %q, want the tick", got)
	}

	// Undocked: the cross, Danger, the button grayed.
	env.Sim.Docked.Remove(env.Sim.Player())
	frame()
	if got := oreVerdictText(oreRowOf(t, r, "ore-cordite")); got != "×" {
		t.Errorf("undocked verdict = %q, want the cross", got)
	}
	if got := oreVerdictTint(oreRowOf(t, r, "ore-cordite")); got != p.Danger {
		t.Errorf("undocked verdict tint = %v, want Danger", got)
	}
	if !r.refineBtn.GetWidget().Disabled {
		t.Fatal("Refine enabled while undocked")
	}
	if got := rfVerdict(r); got != "×" {
		t.Errorf("undocked inspector verdict = %q, want the cross", got)
	}

	// Docked at a station without the refine service: the cross again.
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: rfNoRefine})
	frame()
	if got := oreVerdictText(oreRowOf(t, r, "ore-cordite")); got != "×" {
		t.Errorf("no-service verdict = %q, want the cross", got)
	}
	if !r.refineBtn.GetWidget().Disabled {
		t.Fatal("Refine enabled at a station without the service")
	}

	// Below one whole batch: the cross, despite the station's service.
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: rfStation})
	cargo, _ := env.Sim.Cargos.Get(env.Sim.Player())
	for i := range cargo.Stacks {
		if cargo.Stacks[i].Item == "ore-cordite" {
			cargo.Stacks[i].Qty = 99
		}
	}
	frame()
	if got := oreVerdictText(oreRowOf(t, r, "ore-cordite")); got != "×" {
		t.Errorf("sub-batch verdict = %q, want the cross", got)
	}
	if got := oreVerdictTint(oreRowOf(t, r, "ore-cordite")); got != p.Danger {
		t.Errorf("sub-batch verdict tint = %v, want Danger", got)
	}
	if !r.refineBtn.GetWidget().Disabled {
		t.Fatal("Refine enabled below one whole batch")
	}
	if got := rfVerdict(r); got != "×" {
		t.Errorf("sub-batch inspector verdict = %q, want the cross", got)
	}
}

// TestRefineryStripShares: a row's strip is one batch's mineral value split in
// canonical mineral-id order — the exact pixel widths (ShareWidths), the sum
// equal to the strip, and the batch's own floor(yield × 100) amounts.
func TestRefineryStripShares(t *testing.T) {
	r, _, _, _, _ := refineryEnv(t)
	p := theme.DefaultPalette()

	// Kerngrit: Sideron 35 × 6 € and Nixium 30 × 14 €, in that order.
	cordite := r.batchOf("ore-cordite")
	if len(cordite) != 2 || cordite[0].Item != "min-ferrite" || cordite[0].Qty != 35 ||
		cordite[0].Value != 210 || cordite[1].Item != "min-nickel" || cordite[1].Qty != 30 ||
		cordite[1].Value != 420 {
		t.Fatalf("cordite batch = %+v, want 35 Sideron 210 € then 30 Nixium 420 €", cordite)
	}
	// Lucidite: canonical order is cobalt, nickel, silex (the authored map is
	// unordered and must never reach a row).
	lucidite := r.batchOf("ore-lucidite")
	if len(lucidite) != 3 {
		t.Fatalf("lucidite batch = %+v, want three minerals", lucidite)
	}
	for i, id := range []content.ItemID{"min-cobalt", "min-nickel", "min-silex"} {
		if lucidite[i].Item != id {
			t.Fatalf("segment %d = %q, want the canonical %q", i, lucidite[i].Item, id)
		}
	}
	if got := []int{lucidite[0].Qty, lucidite[1].Qty, lucidite[2].Qty}; !reflect.DeepEqual(got, []int{25, 30, 20}) {
		t.Errorf("lucidite amounts = %v, want floor(yield × 100) = 25/30/20", got)
	}
	if got := []float64{lucidite[0].Value, lucidite[1].Value, lucidite[2].Value}; !reflect.DeepEqual(got, []float64{550, 420, 680}) {
		t.Errorf("lucidite values = %v, want 550/420/680", got)
	}

	// The exact pixel split: whole pixels, sum == the 128 px strip, one tint
	// per segment.
	cases := []struct {
		ore    content.ItemID
		widths []int
	}{
		{"ore-sterngrit", []int{128}},
		{"ore-cordite", []int{43, 85}},
		{"ore-lucidite", []int{43, 32, 53}},
		{"ore-glacierite", []int{61, 67}},
	}
	for _, c := range cases {
		strip := yieldStrip(r.batchOf(c.ore), p)
		shares := make([]float64, len(strip))
		for i, s := range strip {
			shares[i] = s.Share
		}
		widths := widgets.ShareWidths(theme.Px(mfgStripW), shares)
		if !reflect.DeepEqual(widths, c.widths) {
			t.Errorf("%s strip widths = %v, want the exact %v", c.ore, widths, c.widths)
		}
		sum := 0
		for _, w := range widths {
			sum += w
		}
		if sum != theme.Px(mfgStripW) {
			t.Errorf("%s strip widths sum to %d, want the %d px strip", c.ore, sum, theme.Px(mfgStripW))
		}
		for i := 1; i < len(strip); i++ {
			if strip[i].Tint == strip[i-1].Tint {
				t.Errorf("%s segments %d and %d share the tint %v", c.ore, i-1, i, strip[i].Tint)
			}
		}
	}

	// The held row draws that strip beside its held count.
	row := oreRowOf(t, r, "ore-lucidite")
	if got := row.Stack[1].Segs[0].Text; !strings.HasSuffix(got, " held") {
		t.Errorf("held line = %q, want the count", got)
	}
	split := row.Stack[1].Segs[1].Split
	if len(split) != 3 || split[2].Share != 680 {
		t.Errorf("lucidite row strip = %+v, want the batch's three shares", split)
	}
	if got := row.Stack[1].Segs[1].MinW; got != theme.Px(mfgStripW) {
		t.Errorf("strip width = %d, want the %d px row strip", got, theme.Px(mfgStripW))
	}
}

// TestRefineryInspector: the inspector is driven by the selected ore: the item
// head at the hero size with the ore's own prose, and the yield bill, one row
// per mineral, the most valuable first, at the material bill's icon size with
// amounts, shares and values — and no have/need bar.
func TestRefineryInspector(t *testing.T) {
	r, env, ui, _, frame := refineryEnv(t)
	clickRow(ui, r.ores, oreRowFor(r, "ore-lucidite"))
	frame()
	frame()

	def, _ := env.Content.Item("ore-lucidite")
	if got, want := rfTitle(r), "Lucidite Ore  ✓"; got != want {
		t.Errorf("head name = %q, want %q", got, want)
	}
	if got := r.head.headIcon.Widget().Image.Bounds().Dx(); got != theme.Px(theme.IconHero) {
		t.Errorf("head icon = %d px, want the %d px hero", got, theme.Px(theme.IconHero))
	}
	if got := r.head.headProse.Label; got != def.Description || got == "" {
		t.Errorf("head prose = %q, want the ore's own description", got)
	}
	if got, want := r.head.headProse.MaxWidth, float64(r.inspInnerW()); got != want {
		t.Errorf("prose wrap width = %v, want the pane's inner %v", got, want)
	}

	// The bill: three rows, the value leader first, each at 48 px art size.
	if got := len(r.billRows); got != 3 {
		t.Fatalf("bill rows = %d, want the batch's 3 minerals", got)
	}
	bill := billOrder(r.batchOf("ore-lucidite"))
	if got := []content.ItemID{bill[0].Item, bill[1].Item, bill[2].Item}; !reflect.DeepEqual(
		got, []content.ItemID{"min-silex", "min-cobalt", "min-nickel"}) {
		t.Fatalf("bill order = %v, want Vetril, Zaffrite, Nixium (value first)", got)
	}
	total := 0.0
	for _, o := range bill {
		total += o.Value
	}
	for i, o := range bill {
		row := r.yieldRowDef(o, total, theme.DefaultPalette())
		if row.IconSize != mfgMatIcon {
			t.Errorf("bill row %d icon size = %d, want the bill's %d", i, row.IconSize, mfgMatIcon)
		}
		if got := r.billRows[i].Icon.Widget().Image.Bounds().Dx(); got != theme.Px(mfgMatIcon) {
			t.Errorf("bill row %d art = %d px, want the %d px box", i, got, theme.Px(mfgMatIcon))
		}
		// Line 2 carries the amount per batch and the mineral's value; there
		// is no bar (have/need does not apply to an output).
		if got, want := row.Stack[1].Segs[0].Text, theme.FormatInt(int64(o.Qty))+" per batch"; got != want {
			t.Errorf("bill row %d amount = %q, want %q", i, got, want)
		}
		if got, want := row.Stack[1].Segs[1].Text, mfgMoney(o.Value); got != want {
			t.Errorf("bill row %d value = %q, want %q", i, got, want)
		}
		for _, line := range row.Stack {
			for _, seg := range line.Segs {
				if seg.HasBar {
					t.Errorf("bill row %d draws a have/need bar: outputs have no need", i)
				}
			}
		}
		if got, want := row.Stack[0].Segs[1].Text, theme.PctLabel(o.Value/total); got != want {
			t.Errorf("bill row %d share = %q, want %q", i, got, want)
		}
	}

	// The rows fit the pane's viewport.
	view := r.billScroll.ViewRect()
	for i, row := range r.billRows {
		if rect := row.Root.GetWidget().Rect; rect.Dx() > view.Dx() {
			t.Errorf("bill row %d (%d px) overflows the %d px viewport", i, rect.Dx(), view.Dx())
		}
	}
}

// TestRefineryMathTable pins the inspector's money question to the docked
// book: one whole batch sold raw against its minerals, both walked over the
// bids, in the four rows the window draws — and no fee row, since the sim
// charges none.
func TestRefineryMathTable(t *testing.T) {
	r, env, ui, _, frame := refineryEnv(t)
	clickRow(ui, r.ores, oreRowFor(r, "ore-lucidite"))
	frame()

	// The fixture's figures, from the book the table reads.
	book := env.Sim.BookOrders(rfStation)
	outs := r.batchOf("ore-lucidite")
	raw, ok := bookValue(book, "ore-lucidite", sim.OrderBuy, sim.RefineBatch)
	if !ok {
		t.Fatal("fixture: no buy order for the ore")
	}
	mine := 0.0
	for _, o := range outs {
		v, ok := bookValue(book, o.Item, sim.OrderBuy, o.Qty)
		if !ok {
			t.Fatalf("fixture: no buy order for %s", o.Item)
		}
		mine += v
	}
	// 100 ore at the 72 € bid; 25 Zaffrite at 18 €, 30 Nixium at 12 € and
	// 20 Vetril at 28 €.
	if raw != 7200 || mine != 1370 {
		t.Fatalf("fixture figures moved: ore %v, minerals %v (want 7,200 / 1,370)", raw, mine)
	}
	for _, c := range []struct{ label, want string }{
		{"Ore value", "7,200 €"},
		{"Minerals value", "1,370 €"},
		{"Refine delta", "−5,830 €"},
		{"Margin", "-426%"},
	} {
		if got := mathValue(r, c.label); got != c.want {
			t.Errorf("math row %q = %q, want %q", c.label, got, c.want)
		}
	}
	// The formulas both ways, on the same book read.
	if got, want := mathValue(r, "Ore value"), mfgMoney(raw); got != want {
		t.Errorf("ore value = %q, want the bids' walk %q", got, want)
	}
	if got, want := mathValue(r, "Minerals value"), mfgMoney(mine); got != want {
		t.Errorf("minerals value = %q, want the outputs' walk %q", got, want)
	}
	if got, want := mathValue(r, "Refine delta"), mfgSigned(mine-raw); got != want {
		t.Errorf("delta = %q, want minerals − ore %q", got, want)
	}
	if got, want := mathValue(r, "Margin"), theme.PctLabel((mine-raw)/mine); got != want {
		t.Errorf("margin = %q, want delta/mine %q", got, want)
	}
	// A loss is the Danger tint, and the state is in the string's sign too.
	if got := mathTint(r, "Refine delta"); got != theme.DefaultPalette().Danger {
		t.Errorf("delta tint = %v, want Danger", got)
	}
	// No fee exists in the sim: the table prints no fee line.
	for _, row := range r.math.RowsForTest() {
		if strings.Contains(strings.ToLower(row.Cells[0]), "fee") {
			t.Errorf("math table prints a fee row (%q), the sim charges none", row.Cells[0])
		}
	}
}

// TestRefineryMathTableBranches pins the two branches the fixture cannot show:
// a batch worth refining (a good delta, tinted Good) and a side the book holds
// no buy order for (the dash and the marker, never a content base price).
func TestRefineryMathTableBranches(t *testing.T) {
	r, _, ui, _, frame := refineryEnv(t)
	clickRow(ui, r.ores, oreRowFor(r, "ore-cordite"))
	frame()
	outs := r.batchOf("ore-cordite")
	p := theme.DefaultPalette()

	// A book that pays more for the minerals than for the ore: 100 ore at 1 €
	// against 35 × 5 € and 30 × 12 €.
	good := []sim.MarketStock{
		{Item: "ore-cordite", Side: sim.OrderBuy, Price: 1, Qty: 5000},
		{Item: "min-ferrite", Side: sim.OrderBuy, Price: 5, Qty: 25000},
		{Item: "min-nickel", Side: sim.OrderBuy, Price: 12, Qty: 8000},
	}
	rows := r.refineRows(good, outs)
	if got, want := rowValue(rows, "Refine delta"), mfgSigned(535-100); got != want {
		t.Errorf("good delta = %q, want %q", got, want)
	}
	if got, want := rowValue(rows, "Margin"), theme.PctLabel(435.0/535.0); got != want {
		t.Errorf("good margin = %q, want %q", got, want)
	}
	for _, row := range rows {
		if row.Cells[0] == "Refine delta" && row.Tint != p.Good {
			t.Errorf("positive delta tint = %v, want Good", row.Tint)
		}
	}

	// No bid for the ore: ore and derived lines dash with the marker.
	rows = r.refineRows(good[1:], outs)
	if got, want := rowValue(rows, "Ore value"), "— · no buy order"; got != want {
		t.Errorf("no-bid ore value = %q, want %q", got, want)
	}
	for _, label := range []string{"Refine delta", "Margin"} {
		if got := rowValue(rows, label); got != "—" {
			t.Errorf("%q = %q with no ore bid, want a dash", label, got)
		}
	}
	for _, row := range rows {
		if row.Cells[0] == "Ore value" && row.Tint != p.Dim {
			t.Errorf("no-bid ore tint = %v, want Dim", row.Tint)
		}
	}

	// No bid for a mineral: the minerals side dashes and the derived lines
	// with it; the ore side still prices.
	rows = r.refineRows([]sim.MarketStock{good[0], good[1]}, outs)
	if got, want := rowValue(rows, "Minerals value"), "— · no buy order"; got != want {
		t.Errorf("no-bid minerals value = %q, want %q", got, want)
	}
	for _, label := range []string{"Refine delta", "Margin"} {
		if got := rowValue(rows, label); got != "—" {
			t.Errorf("%q = %q with a mineral bid missing, want a dash", label, got)
		}
	}
	if got, want := rowValue(rows, "Ore value"), mfgMoney(100); got != want {
		t.Errorf("ore value = %q, want the local bid's %q", got, want)
	}
	for _, row := range rows {
		if row.Cells[0] == "Minerals value" && row.Tint != p.Dim {
			t.Errorf("no-bid minerals tint = %v, want Dim", row.Tint)
		}
	}
}

// TestRefineryEmptyState: with no ore held the list says so, the inspector's
// head is its hint line alone (the market's empty idiom) and every number is a
// dash with the Refine button grayed.
func TestRefineryEmptyState(t *testing.T) {
	env, _ := newChromeEnv(t)
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: rfStation})
	cargo, _ := env.Sim.Cargos.Get(env.Sim.Player())
	cargo.Stacks, cargo.Ore = nil, nil
	r := NewRefinery(env)
	ui := newQtyUI(t, env, r.Win())
	frame := func() { pumpQty(t, ui, env, r.Sync, 1024, 700) }
	frame()
	frame()

	if len(r.ores.Rows) != 1 || r.ores.Rows[0].Cells[0] != "No ore held" {
		t.Fatalf("empty list rows = %+v, want the one no-ore row", r.ores.Rows)
	}
	if got := rfTitle(r); got != "" {
		t.Errorf("empty head name = %q, want none", got)
	}
	if got := r.head.headHint.Label; got != "Select an ore to refine." {
		t.Errorf("empty head hint = %q, want the selection hint", got)
	}
	for _, label := range []string{"Ore value", "Minerals value", "Refine delta", "Margin"} {
		if got := mathValue(r, label); got != "—" {
			t.Errorf("empty math row %q = %q, want a dash", label, got)
		}
	}
	if len(r.billRows) != 0 {
		t.Errorf("empty bill rows = %d, want none", len(r.billRows))
	}
	if !r.refineBtn.GetWidget().Disabled {
		t.Fatal("Refine enabled with no ore held")
	}
}

// TestRefineryRemovedHints: the static status line the window used to print
// under the list is gone from every text it draws — the gate is the grayed
// Refine button and the verdict glyph, and the empty state is the head's hint.
func TestRefineryRemovedHints(t *testing.T) {
	r, env, ui, _, frame := refineryEnv(t)
	sweep := func() []string {
		texts := []string{rfTitle(r), r.head.headProse.Label, r.head.headHint.Label, r.billLbl.Label}
		for _, row := range r.math.RowsForTest() {
			texts = append(texts, row.Cells...)
		}
		for _, tbl := range []*widgets.Table{r.tree, r.ores} {
			for _, row := range tbl.Rows {
				texts = append(texts, row.Cells...)
				for _, line := range row.Stack {
					for _, seg := range line.Segs {
						texts = append(texts, seg.Text)
					}
				}
			}
		}
		for _, o := range billOrder(r.batchOf(r.sel.ore)) {
			row := r.yieldRowDef(o, 1, theme.DefaultPalette())
			texts = append(texts, row.Cells...)
			for _, line := range row.Stack {
				for _, seg := range line.Segs {
					texts = append(texts, seg.Text)
				}
			}
		}
		return texts
	}
	check := func(stage string, texts []string) {
		for _, text := range texts {
			for _, gone := range []string{
				"Refine needs a station",
				"-ore batches only",
				"One batch needs",
				"opens the quantity picker",
			} {
				if strings.Contains(text, gone) {
					t.Errorf("%s renders %q, want the removed status line gone", stage, text)
				}
			}
		}
	}
	check("the empty window", sweep())

	clickRow(ui, r.ores, oreRowFor(r, "ore-cordite"))
	frame()
	check("the selected window", sweep())

	// The docked sub-batch case the status line used to spell out: the button
	// is the gate and it says nothing.
	cargo, _ := env.Sim.Cargos.Get(env.Sim.Player())
	for i := range cargo.Stacks {
		if cargo.Stacks[i].Item == "ore-cordite" {
			cargo.Stacks[i].Qty = 10
		}
	}
	frame()
	check("the sub-batch window", sweep())
	if !r.refineBtn.GetWidget().Disabled {
		t.Fatal("Refine enabled below one batch")
	}
}
