package screens

import (
	"image"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// mktTreeEnv is the tree tests' fixture: a stub book with one traded item
// (two asks, two player bids) and one item nobody trades at all, so the
// parent row, the folds and the dimmed-but-live empty item are all pinned
// on the same surface.
func mktTreeEnv(t *testing.T) (*Market, *stubView, *recordingSink, func(), func(int)) {
	t.Helper()
	th := theme.NewSeam(theme.DefaultPalette(), theme.DefaultTypeScale())
	cv := &stubView{
		c: &content.Content{Items: map[content.ItemID]*content.ItemDef{
			"item-stub-ore":   {Name: "Stub Ore", Category: content.CatOre, BasePrice: 40},
			"item-stub-empty": {Name: "Stub Mineral", Category: content.CatMineral, BasePrice: 20},
		}},
		orders: []sim.MarketStock{
			{Item: "item-stub-ore", Price: 100, Qty: 5, Side: sim.OrderSell, ID: 5},
			{Item: "item-stub-ore", Price: 120, Qty: 7, Side: sim.OrderSell, ID: 6},
			{Item: "item-stub-ore", Price: 90, Qty: 2, Side: sim.OrderBuy, Owner: sim.OwnerPlayer, ID: 7},
			{Item: "item-stub-ore", Price: 80, Qty: 3, Side: sim.OrderBuy, Owner: sim.OwnerPlayer, ID: 8},
		},
		docked: true,
		wallet: 1000,
	}
	rec := &recordingSink{}
	env := widgets.NewSeams(th, cv, rec)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	frame := func() { uitest.PumpWindow(t, ui, m.Sync, 1024, 700) }
	click := func(i int) {
		clickRow(ui, m.tbl, i)
		frame()
	}
	return m, cv, rec, frame, click
}

// mktParentRow is the display index of an item's parent row.
func mktParentRow(t *testing.T, m *Market, item content.ItemID) int {
	t.Helper()
	for i, r := range m.tbl.Rows {
		if row, ok := r.Data.(mktItemRow); ok && row.Item == item {
			return i
		}
	}
	t.Fatalf("no parent row for %q", item)
	return -1
}

// TestMarketTreeParents pins the tree's item rows: one parent per catalog
// item (always, in canonical order), the ladder price, the summed quantity
// over both sides and the side cell an item row does not have. An item
// with no orders anywhere is DIMMED but keeps its verbs live — placing the
// first order is how a market wakes up.
func TestMarketTreeParents(t *testing.T) {
	m, _, _, frame, _ := mktTreeEnv(t)
	frame()
	frame()
	p := m.th.Pal()

	if len(m.tbl.Rows) != 2 {
		t.Fatalf("rows = %d, want one parent per stub item (2)", len(m.tbl.Rows))
	}
	empty := m.tbl.Rows[mktParentRow(t, m, "item-stub-empty")]
	live := m.tbl.Rows[mktParentRow(t, m, "item-stub-ore")]

	// The dimmed, orderless item: every cell but the item name says "no
	// book"; the price still quotes the ladder (the stub's MarketPrice).
	if empty.TextTint != p.Dim {
		t.Errorf("orderless item row tint = %#v, want the palette's Dim", empty.TextTint)
	}
	if got := empty.Cells[2]; got != "0" {
		t.Errorf("orderless item qty = %q, want 0", got)
	}
	if empty.Cells[3] != mktNoSide {
		t.Errorf("item side cell = %q, want %q", empty.Cells[3], mktNoSide)
	}
	if got, want := live.Cells[2], "17"; got != want {
		t.Errorf("traded item qty = %q, want %q (both sides summed)", got, want)
	}
	if live.TextTint.A != 0 {
		t.Errorf("traded item row tint = %#v, want the plain text colour", live.TextTint)
	}
	for name, row := range map[string]widgets.TableRow{"empty": empty, "traded": live} {
		if got, want := row.Cells[1], theme.FormatCredits(42); got != want {
			t.Errorf("%s item price = %q, want the ladder quote %q", name, got, want)
		}
		if row.Spec.Ref == nil {
			t.Errorf("%s item row declares no Show Info ref", name)
		}
		// Grayed is not gated: every verb stays live while docked except
		// Buy on a row whose local book holds no ask to lift.
		if len(row.Spec.Actions) != 4 {
			t.Fatalf("%s item row offers %d verbs, want 4", name, len(row.Spec.Actions))
		}
		for _, a := range row.Spec.Actions {
			wantOff := a.Label == "Buy" && name == "empty"
			if a.Disabled != wantOff {
				t.Errorf("%s item row %q Disabled = %v, want %v", name, a.Label, a.Disabled, wantOff)
			}
		}
	}

	// Canonical item order: ascending by id.
	if got, want := m.tbl.Rows[0].Data.(mktItemRow).Item, content.ItemID("item-stub-empty"); got != want {
		t.Errorf("first parent = %q, want %q", got, want)
	}
}

// TestMarketTreeExpandCollapse pins the fold: a click on an item row shows
// exactly that item's orders beneath it — asks ascending, then bids
// descending, in the book's own [price, qty, side] cells — and a second
// click folds them away again.
func TestMarketTreeExpandCollapse(t *testing.T) {
	m, _, _, frame, click := mktTreeEnv(t)
	frame()
	frame()
	idx := mktParentRow(t, m, "item-stub-ore")

	click(idx)
	if len(m.tbl.Rows) != 6 {
		t.Fatalf("expanded rows = %d, want 2 parents + 4 orders", len(m.tbl.Rows))
	}
	want := []struct {
		price float64
		qty   string
		side  string
	}{
		{100, "5", "Ask"},
		{120, "7", "Ask"},
		{90, "2", "Bid"},
		{80, "3", "Bid"},
	}
	for i, w := range want {
		r := m.tbl.Rows[idx+1+i]
		row, ok := r.Data.(mktOrderRow)
		if !ok || row.Item != "item-stub-ore" {
			t.Fatalf("row %d payload = %#v, want an order row of item-stub-ore", idx+1+i, r.Data)
		}
		if got := parseCredits(t, r.Cells[1]); got != w.price {
			t.Errorf("child %d price = %v, want %v", i, got, w.price)
		}
		if r.Cells[2] != w.qty || r.Cells[3] != w.side {
			t.Errorf("child %d = %q/%q, want %q/%q", i, r.Cells[2], r.Cells[3], w.qty, w.side)
		}
	}
	// The empty item keeps its lone parent row: expanding an item never
	// invents orders for it (it sorts first, so it stays row 0).
	if got := m.tbl.Rows[0].Data.(mktItemRow).Item; got != "item-stub-empty" {
		t.Fatalf("row 0 after expand = %q, want the orderless item", got)
	}

	click(idx)
	if len(m.tbl.Rows) != 2 {
		t.Fatalf("collapsed rows = %d, want the 2 parent rows", len(m.tbl.Rows))
	}
}

// TestMarketTreeForgetsFoldsOnClose: the fold state is session-only — a
// closed window (the kit hides it on close) forgets what was open and
// reopens collapsed. It is never written to a save or a layout sidecar.
func TestMarketTreeForgetsFoldsOnClose(t *testing.T) {
	m, _, _, frame, click := mktTreeEnv(t)
	frame()
	frame()
	click(mktParentRow(t, m, "item-stub-ore"))
	if !m.open["item-stub-ore"] {
		t.Fatal("fixture: the fold did not open")
	}

	// The browse branch state and the search query are session-only the
	// same way.
	m.branch, m.treeSel = "Modules", "Modules/High"
	m.expanded["Modules"] = true
	m.search.Insert("turret")

	// What a committed × close does: the host's Dismissed hook runs once
	// while the close fades, then the panel syncs once more.
	m.Dismissed()
	m.Sync()
	if len(m.open) != 0 {
		t.Fatalf("folds = %v after the window closed, want none", m.open)
	}
	if got := m.search.GetText(); got != "" {
		t.Fatalf("search query = %q after the window closed, want it cleared", got)
	}
	if m.branch != "" || m.treeSel != "" || len(m.expanded) != 1 || !m.expanded[""] {
		t.Fatalf("browse state = branch %q, highlight %q, expanded %v — want All only",
			m.branch, m.treeSel, m.expanded)
	}
	theme.ShowWidget(m.Win().GetContainer().GetWidget(), true)
	frame()
	if got := len(m.tbl.Rows); got != 2 {
		t.Fatalf("reopened rows = %d, want the collapsed 2", got)
	}
}

// TestMarketTreeOwnedRowsTint: the player's own resting orders wear the
// owned-row green inside the folds, NPC orders keep the plain background.
func TestMarketTreeOwnedRowsTint(t *testing.T) {
	m, _, _, frame, click := mktTreeEnv(t)
	frame()
	frame()
	click(mktParentRow(t, m, "item-stub-ore"))

	want := theme.OwnedRowTint(m.th.Pal())
	owned, plain := 0, 0
	for _, r := range m.tbl.Rows {
		if parent, isParent := r.Data.(mktItemRow); isParent {
			if r.Tint.A != 0 {
				t.Errorf("item row %q carries a tint %#v", parent.Item, r.Tint)
			}
			continue
		}
		switch {
		case r.Tint == want:
			owned++
		case r.Tint.A == 0:
			plain++
		default:
			t.Errorf("order row tint = %#v, want the owned green or none", r.Tint)
		}
	}
	if owned != 2 || plain != 2 {
		t.Errorf("folded tints: %d owned, %d plain, want 2/2", owned, plain)
	}
}

// TestMarketTreeRequestFromGrayedRow: Request on Market works from an item
// with no orders anywhere — the row's menu emits the order-window launch
// for that item, the way the traded rows do.
func TestMarketTreeRequestFromGrayedRow(t *testing.T) {
	m, cv, rec, frame, _ := mktTreeEnv(t)
	frame()
	frame()
	row := m.tbl.Rows[mktParentRow(t, m, "item-stub-empty")]
	if row.TextTint != m.th.Pal().Dim {
		t.Fatal("fixture: the requested row is not the grayed one")
	}

	entries := widgets.RowMenuEntries(cv, rec, row.Spec)
	menu := widgets.NewMenu(m.th)
	menu.Open(entries, image.Pt(10, 10))
	found := false
	for i, e := range menu.Entries() {
		if e.Label != "Request on Market" {
			continue
		}
		found = true
		if e.Disabled {
			t.Fatal("Request on Market grayed on a live item row")
		}
		menu.Click(menu.RowRect(i).Min.Add(image.Pt(1, 1)))
	}
	if !found {
		t.Fatalf("menu has no Request on Market entry: %+v", menu.Entries())
	}
	if len(rec.got) != 1 {
		t.Fatalf("emitted %#v, want one OpenOrder", rec.got)
	}
	if got := rec.got[0]; got != (widgets.OpenOrder{Item: "item-stub-empty", Side: sim.OrderBuy, Qty: 1}) {
		t.Errorf("Request on Market emitted %#v, want OpenOrder{buy item-stub-empty 1}", got)
	}
}

// TestMarketTreeSortKeepsGroups: a header sort orders the ITEMS by that
// column and keeps every item's orders beneath their own parent — the tree
// never tears apart.
func TestMarketTreeSortKeepsGroups(t *testing.T) {
	m, _, _, frame, click := mktTreeEnv(t)
	frame()
	frame()
	click(mktParentRow(t, m, "item-stub-ore"))

	before := m.tbl.Rows
	m.tbl.SetSort("Qty", true)
	after := m.tbl.RowsForTest()
	if len(after) != len(before) {
		t.Fatalf("sort changed the row count: %d → %d", len(before), len(after))
	}
	parent := content.ItemID("")
	for i, r := range after {
		if row, isParent := r.Data.(mktItemRow); isParent {
			parent = row.Item
			continue
		}
		if row := r.Data.(mktOrderRow); row.Item != parent {
			t.Fatalf("row %d: order of %q sits under parent %q (the sort split a group)", i, row.Item, parent)
		}
	}
	// Qty sorting is the cell sort it always was: the item with 0 units
	// first, the traded one (17) last.
	if got := after[0].Data.(mktItemRow).Item; got != "item-stub-empty" {
		t.Errorf("first item after a Qty sort = %q, want item-stub-empty (0)", got)
	}
}
