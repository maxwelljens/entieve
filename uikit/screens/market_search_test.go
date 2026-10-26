package screens

import (
	"slices"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/sahilm/fuzzy"
)

// mktSearchEnv docks the showcase game at its market station and builds
// the window there: the browse tree, the catalog and the cross-station
// reads all have real content behind them.
func mktSearchEnv(t *testing.T) (*Market, *testEnv, *ebitenui.UI, func()) {
	t.Helper()
	env, _ := newTestEnv(t)
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: "cel-haven-rest"})
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	frame := func() { uitest.PumpWindow(t, ui, m.Sync, 1024, 700) }
	frame()
	frame()
	return m, env, ui, frame
}

// treeRowIndex is the display index of a browse row by its path.
func treeRowIndex(t *testing.T, m *Market, path string) int {
	t.Helper()
	for i, r := range m.tree.Rows {
		if k, ok := r.Data.(mktNodeKey); ok && k.Path == path {
			return i
		}
	}
	t.Fatalf("no browse row for %q", path)
	return -1
}

// TestMarketTreeBranches: the browse pane opens on All (the whole catalog
// listed), lists the five top-level branches and their children on a
// click, and selecting a branch scopes the catalog to its subtree.
func TestMarketTreeBranches(t *testing.T) {
	m, env, ui, frame := mktSearchEnv(t)

	roots := map[string]bool{}
	for _, r := range m.tree.Rows {
		k := r.Data.(mktNodeKey)
		if k.Path != "" && !strings.Contains(k.Path, "/") {
			roots[k.Path] = true
		}
	}
	for _, want := range []string{"Ammunition", "Loot", "Materials", "Modules", "Ships"} {
		if !roots[want] {
			t.Errorf("browse tree lacks the %q branch (rows %v)", want, m.tree.Rows)
		}
	}
	if got, want := len(m.tbl.Rows), len(marketCatalog(env)); got != want {
		t.Fatalf("All lists %d items, want the whole catalog (%d)", got, want)
	}
	// All is the opening selection and it starts expanded (the five roots
	// are visible above).
	if sel, ok := m.tree.Selected(); !ok || sel.Data.(mktNodeKey).Path != "" {
		t.Fatalf("opening tree selection = %+v, want the All root", sel.Data)
	}

	// Expanding Modules and selecting it scopes the catalog to modules
	// only, still in canonical order.
	clickRow(ui, m.tree, treeRowIndex(t, m, "Modules"))
	frame()
	mods := 0
	for _, item := range marketCatalog(env) {
		if def, ok := env.Defs().Modules[content.ModuleID(item)]; ok && def != nil {
			mods++
		}
	}
	if got := len(m.tbl.Rows); got != mods {
		t.Fatalf("Modules branch lists %d rows, want the %d modules", got, mods)
	}
	for _, r := range m.tbl.Rows {
		item := r.Data.(mktItemRow).Item
		if _, ok := env.Defs().Modules[content.ModuleID(item)]; !ok {
			t.Errorf("Modules branch lists %q, not a module", item)
		}
	}
	// The children are visible once the branch is expanded.
	if got := len(m.tree.Rows); got < 8 {
		t.Errorf("expanded Modules shows %d browse rows, want its children too", got)
	}

	// Selecting an item in the catalog syncs the highlight back to its
	// group branch (a module here), without moving the branch scope.
	clickRow(ui, m.tbl, 0)
	frame()
	if !m.hasSel {
		t.Fatal("item click did not select")
	}
	if got, want := m.treeSel, m.group[m.sel]; got != want {
		t.Errorf("tree highlight = %q, want the selected item's group %q", got, want)
	}
	if m.branch != "Modules" {
		t.Errorf("branch = %q, want the Modules scope kept", m.branch)
	}
}

// TestMarketSearchScopesAndRanks: the query filters the selected branch,
// flat and ranked by fuzzy score (overriding the header sort); clearing it
// restores the branch rows in header-sort order.
func TestMarketSearchScopesAndRanks(t *testing.T) {
	m, env, ui, frame := mktSearchEnv(t)
	clickRow(ui, m.tree, treeRowIndex(t, m, "Ammunition"))
	frame()

	m.tbl.SetSort("Price", true)
	frame()
	before := m.tbl.RowsForTest()
	if len(before) < 2 {
		t.Fatal("fixture: Ammunition branch too small")
	}

	m.FocusSearch()
	if !m.SearchFocused() {
		t.Fatal("FocusSearch did not focus the field")
	}
	m.search.Insert("emp")
	frame()

	if len(m.results) == 0 {
		t.Fatal("query \"emp\" matched nothing in Ammunition")
	}
	for _, h := range m.results {
		if !strings.HasPrefix(m.group[h.Item], "Ammunition") {
			t.Errorf("result %q is outside the Ammunition branch", h.Item)
		}
	}
	// Ranked by fuzzy score over name + group + maker: recompute the
	// library's own ordering and compare (the header sort must yield).

	var hay, items []string
	scope := m.scopeItems()
	for _, item := range scope {
		items = append(items, string(item))
		hay = append(hay, env.TradeableName(item)+" "+m.group[item]+" "+m.maker[item])
	}
	var want []string
	for _, mt := range fuzzy.Find("emp", hay) {
		want = append(want, items[mt.Index])
	}
	if len(m.results) != len(want) {
		t.Fatalf("results = %d, want the library's %d", len(m.results), len(want))
	}
	for i := range want {
		if got := string(m.results[i].Item); got != want[i] {
			t.Fatalf("result %d = %s, want the ranked %s", i, got, want[i])
		}
	}
	// The ranked order is not the armed header sort's order (a coincidence
	// would weaken the proof, so pin the difference).
	if len(m.results) == len(before) {
		same := true
		for i, h := range m.results {
			if k, ok := before[i].Data.(mktItemRow); !ok || k.Item != h.Item {
				same = false
				break
			}
		}
		if same {
			t.Fatal("the ranked results coincide with the header-sorted branch — pick a sharper query")
		}
	}

	// Flat: no order child rows in the result list.
	for _, r := range m.tbl.Rows {
		if _, ok := r.Data.(mktOrderRow); ok {
			t.Fatal("search results fold orders — they must be flat")
		}
	}

	// Matched characters are highlighted: the first result's line carries
	// an accent run whose text is part of the name.
	first := m.tbl.Rows[0]
	if len(first.Stack) == 0 {
		t.Fatal("a result row is not a styled run row")
	}
	accent := m.th.Pal().Accent
	found := false
	for _, seg := range first.Stack[0].Segs {
		if seg.Tint == accent {
			found = true
		}
	}
	if !found {
		t.Errorf("result row has no highlighted run: %+v", first.Stack[0].Segs)
	}

	// Clearing restores the branch view and its header sort.
	m.search.SelectAll()
	m.search.DeleteSelectedText()
	frame()
	if m.query != "" || len(m.results) != 0 {
		t.Fatalf("cleared query still searches: %q (%d results)", m.query, len(m.results))
	}
	after := m.tbl.RowsForTest()
	if len(after) != len(before) {
		t.Fatalf("cleared rows = %d, want the branch view's %d", len(after), len(before))
	}
	// The header sort is back: the cleared order is the armed sort's order
	// (and the search order differed from it).
	for i := range after {
		if after[i].Data != before[i].Data {
			t.Fatalf("cleared row %d = %#v, want the sorted %#v", i, after[i].Data, before[i].Data)
		}
	}
}

// TestMarketSearchEmptyState: a query nobody matches renders one dim
// no-match row (not an empty table and not a selectable item).
func TestMarketSearchEmptyState(t *testing.T) {
	m, _, _, frame := mktSearchEnv(t)
	m.FocusSearch()
	m.search.Insert("zzzznomatch")
	frame()
	if len(m.tbl.Rows) != 1 {
		t.Fatalf("rows = %d, want the one no-match row", len(m.tbl.Rows))
	}
	row := m.tbl.Rows[0]
	if row.Cells[0] != "No matches" {
		t.Errorf("empty row = %q, want %q", row.Cells[0], "No matches")
	}
	if row.Data != nil {
		t.Errorf("no-match row carries %#v, want no payload", row.Data)
	}
	if m.hasSel {
		t.Error("no-match row reads as selected")
	}
}

// TestMarketSearchKeyboard: while the field is focused, Arrow keys walk
// the ranked results, Enter selects the highlighted one (list + tree
// sync), and Esc clears the query first — a second Esc on the empty field
// blurs and falls through (SearchKey reports false).
func TestMarketSearchKeyboard(t *testing.T) {
	m, _, _, frame := mktSearchEnv(t)
	m.FocusSearch()
	m.search.Insert("shell")
	frame()
	if len(m.results) < 2 {
		t.Fatalf("fixture: %d results for \"shell\"", len(m.results))
	}

	if !m.SearchKey(ebiten.KeyArrowDown) {
		t.Fatal("ArrowDown not consumed")
	}
	if m.hit != 0 {
		t.Fatalf("hit = %d, want the first result", m.hit)
	}
	m.SearchKey(ebiten.KeyArrowDown)
	if m.hit != 1 {
		t.Fatalf("hit = %d, want 1 after two downs", m.hit)
	}
	if !m.hasSel || m.sel != m.results[1].Item {
		t.Fatalf("arrow highlight = %q, want the second result %q", m.sel, m.results[1].Item)
	}
	if got, want := m.treeSel, m.group[m.sel]; got != want {
		t.Errorf("tree highlight = %q, want %q", got, want)
	}
	m.SearchKey(ebiten.KeyArrowUp)
	if m.hit != 0 {
		t.Fatalf("hit = %d after up, want 0", m.hit)
	}
	// Up at the top clamps.
	m.SearchKey(ebiten.KeyArrowUp)
	if m.hit != 0 {
		t.Fatalf("hit = %d at the top, want the clamped 0", m.hit)
	}

	if !m.SearchKey(ebiten.KeyEnter) {
		t.Fatal("Enter not consumed")
	}
	if !m.hasSel || m.sel != m.results[0].Item {
		t.Fatalf("Enter selected %q, want the highlighted %q", m.sel, m.results[0].Item)
	}

	// Esc with text clears the query and keeps the focus.
	if !m.SearchKey(ebiten.KeyEscape) {
		t.Fatal("first Esc was not consumed")
	}
	if got := m.search.GetText(); got != "" {
		t.Fatalf("Esc left %q in the field, want it cleared", got)
	}
	if !m.SearchFocused() {
		t.Fatal("the first Esc dropped the focus")
	}
	// Esc on the empty field blurs and reports false (the host's normal
	// Esc handling follows).
	if m.SearchKey(ebiten.KeyEscape) {
		t.Fatal("second Esc was consumed, want a fall-through")
	}
	if m.SearchFocused() {
		t.Fatal("second Esc did not blur the field")
	}
}

// TestMarketSearchRescopesOnBranchClick: the query survives a branch
// click and re-scopes instantly to the new subtree.
func TestMarketSearchRescopesOnBranchClick(t *testing.T) {
	m, _, ui, frame := mktSearchEnv(t)
	m.FocusSearch()
	m.search.Insert("emp")
	frame()
	all := len(m.results)
	if all == 0 {
		t.Fatal("fixture: no EMP matches")
	}
	clickRow(ui, m.tree, treeRowIndex(t, m, "Ships"))
	frame()
	if got := m.search.GetText(); got != "emp" {
		t.Fatalf("branch click changed the query to %q", got)
	}
	for _, h := range m.results {
		if !strings.HasPrefix(m.group[h.Item], "Ships") {
			t.Errorf("result %q escaped the Ships branch", h.Item)
		}
	}
}

// TestMarketBuyGreysWithoutLocalAsk pins the visible gate: an item with no
// local ask keeps a dim, live row whose Buy is grayed (Request on Market
// stays live), and the toolbar Buy follows the selected row.
func TestMarketBuyGreysWithoutLocalAsk(t *testing.T) {
	m, env, _, _ := mktSearchEnv(t)

	ask := map[content.ItemID]bool{}
	for _, o := range env.Sim.BookOrders("cel-haven-rest") {
		if o.Side == sim.OrderSell {
			ask[o.Item] = true
		}
	}
	var dead, live content.ItemID
	for _, item := range marketCatalog(env) {
		if ask[item] {
			live = item
		} else if dead == "" {
			dead = item
		}
	}
	if dead == "" || live == "" {
		t.Fatal("fixture: need one stocked and one unstocked item")
	}

	rowOf := func(item content.ItemID) widgets.TableRow {
		for _, r := range m.tbl.Rows {
			if k, ok := r.Data.(mktItemRow); ok && k.Item == item {
				return r
			}
		}
		t.Fatalf("no row for %q", item)
		return widgets.TableRow{}
	}
	verbs := func(r widgets.TableRow, label string) bool {
		for _, a := range r.Spec.Actions {
			if a.Label == label {
				return a.Disabled
			}
		}
		t.Fatalf("no %q verb", label)
		return false
	}
	deadRow := rowOf(dead)
	if !verbs(deadRow, "Buy") {
		t.Errorf("Buy live on %q with no local ask", dead)
	}
	if verbs(deadRow, "Request on Market") {
		t.Errorf("Request on Market grayed on %q", dead)
	}
	if verbs(rowOf(live), "Buy") {
		t.Errorf("Buy grayed on stocked %q", live)
	}

	m.selectItem(dead)
	m.Sync()
	if !m.buy.GetWidget().Disabled {
		t.Error("toolbar Buy enabled on an item the book does not sell")
	}
	m.selectItem(live)
	m.Sync()
	if m.buy.GetWidget().Disabled {
		t.Error("toolbar Buy disabled on a stocked item")
	}
}

// TestMarketAvailabilitySuffix: the Qty cell carries the dim "· N more"
// suffix for the OTHER stations stocking the item.
func TestMarketAvailabilitySuffix(t *testing.T) {
	m, env, _, frame := mktSearchEnv(t)
	local := env.Sim.BookOrders("cel-haven-rest")

	// The count reads the same books the seam reads: pick an item stocked
	// both locally and elsewhere.
	var want content.ItemID
	var count int
	stations := marketStations(env.Defs())
	asks := map[content.ItemID]int{}
	for _, st := range stations {
		if st == "cel-haven-rest" {
			continue
		}
		for _, o := range env.Sim.BookOrders(st) {
			if o.Side == sim.OrderSell {
				asks[o.Item]++
			}
		}
	}
	local0 := map[content.ItemID]bool{}
	for _, o := range local {
		if o.Side == sim.OrderSell {
			local0[o.Item] = true
		}
	}
	for _, item := range marketCatalog(env) {
		if local0[item] && asks[item] >= 1 {
			want, count = item, asks[item]
			break
		}
	}
	if want == "" {
		t.Fatal("fixture: no item stocked both locally and elsewhere")
	}

	var row widgets.TableRow
	for _, r := range m.tbl.Rows {
		if k, ok := r.Data.(mktItemRow); ok && k.Item == want {
			row = r
		}
	}
	if len(row.CellRuns) == 0 || len(row.CellRuns[2]) < 2 {
		t.Fatalf("qty cell of %q has no suffix run: %+v", want, row.CellRuns)
	}
	suffix := row.CellRuns[2][1]
	if !strings.Contains(suffix.Text, "more") || !strings.Contains(suffix.Text, theme.FormatInt(int64(count))) {
		t.Errorf("qty suffix = %q, want the %d-station suffix", suffix.Text, count)
	}
	if suffix.Tint != m.th.Pal().Dim {
		t.Errorf("qty suffix tint = %#v, want the palette's Dim", suffix.Tint)
	}

	// The title stays the base name; the availability detail is the
	// column's suffix alone (the title bar carries no state).
	m.selectItem(want)
	frame()
	if got := m.title.Label; got != "Market" {
		t.Errorf("title = %q, want the base name Market", got)
	}
}

// TestMarketSearchFoldsDiacritics: the query matches through the canon
// diacritics in BOTH directions. The fixtures are derived, not hardcoded —
// every maker that carries a diacritic (folded first word = what a plain
// keyboard types, raw first word = the typed-diacritic direction) is
// queried both ways. The contract is recall plus symmetry, not an exact
// item set: sahilm/fuzzy is a SUBSEQUENCE matcher, so a maker word also
// spells through unrelated rows (a longer word hits more of them), and a
// pin on the exact set would break on any rename — which is how this test
// caught its own fixture the first time the houses drifted.
func TestMarketSearchFoldsDiacritics(t *testing.T) {
	m, env, _, frame := mktSearchEnv(t)

	type mset struct {
		raw   string // the maker, diacritics intact
		items []content.ItemID
	}
	sets := map[string]*mset{}
	for _, item := range marketCatalog(env) {
		mk := m.maker[item]
		folded, _ := foldString(mk)
		if folded == mk {
			continue
		}
		word := strings.SplitN(folded, " ", 2)[0]
		s := sets[word]
		if s == nil {
			s = &mset{raw: strings.SplitN(mk, " ", 2)[0]}
			sets[word] = s
		}
		s.items = append(s.items, item)
	}
	if len(sets) < 2 {
		t.Fatalf("fixture: %d diacritic-carrying makers, need at least 2", len(sets))
	}
	big := false
	for _, s := range sets {
		if len(s.items) >= 2 {
			big = true
		}
	}
	if !big {
		t.Fatal("fixture: every diacritic maker is a singleton — need one with a set")
	}

	m.FocusSearch()
	query := func(q string) []content.ItemID {
		t.Helper()
		// SetText, not SelectAll+DeleteSelectedText: the widget's
		// selection delete miscounts bytes on multi-byte runes and
		// leaves trailing bytes behind (observed: "zak\u0142ady" -> "y").
		m.search.SetText(q)
		frame()
		out := []content.ItemID{}
		for _, h := range m.results {
			out = append(out, h.Item)
		}
		return out
	}
	sameSet := func(got, want []content.ItemID) bool {
		if len(got) != len(want) {
			return false
		}
		set := map[content.ItemID]bool{}
		for _, item := range want {
			set[item] = true
		}
		for _, item := range got {
			if !set[item] {
				return false
			}
		}
		return true
	}

	for word, s := range sets {
		folded, typed := query(word), query(s.raw)
		for _, item := range s.items {
			if !slices.Contains(folded, item) {
				t.Errorf("ASCII %q missed one of its own %q items: %v", word, s.raw, folded)
			}
		}
		if !sameSet(folded, typed) {
			t.Errorf("%q and %q disagree: %v vs %v", word, s.raw, folded, typed)
		}
	}
}
