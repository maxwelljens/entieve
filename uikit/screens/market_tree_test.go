package screens

import (
	"image"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// mktTreeEnv is the tree tests' fixture: a stub book with one traded item
// (two asks, two player bids) and one item nobody trades at all, so the
// item rows, the right pane's book and the dimmed-but-live empty item are
// all pinned on the same surface.
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
	env := widgets.NewSeams(th, cv, rec, nil)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	frame := func() { uitest.PumpWindow(t, ui, m.Sync, 1024, 700) }
	click := func(i int) {
		clickRow(ui, m.tbl, i)
		frame()
	}
	return m, cv, rec, frame, click
}

// mktParentRow is the display index of an item's catalog row.
func mktParentRow(t *testing.T, m *Market, item content.ItemID) int {
	t.Helper()
	for i, r := range m.tbl.Rows {
		if row, ok := r.Data.(mktItemRow); ok && row.Item == item {
			return i
		}
	}
	t.Fatalf("no catalog row for %q", item)
	return -1
}

// TestMarketTreeParents pins the catalog's item rows: one row per catalog
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
		t.Fatalf("rows = %d, want one row per stub item (2)", len(m.tbl.Rows))
	}
	empty := m.tbl.Rows[mktParentRow(t, m, "item-stub-empty")]
	live := m.tbl.Rows[mktParentRow(t, m, "item-stub-ore")]

	// The dimmed, orderless item: every cell but the item name says "no
	// book"; the price still quotes the ladder (the stub's PriceLadder).
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
		t.Errorf("first item row = %q, want %q", got, want)
	}
}

// TestMarketTreeClicksNeverFold: a click selects (and a second click
// re-selects); the catalog never grows order rows and the right pane
// keeps showing the book of the selected item.
func TestMarketTreeClicksNeverFold(t *testing.T) {
	m, _, _, frame, click := mktTreeEnv(t)
	frame()
	frame()

	click(mktParentRow(t, m, "item-stub-ore"))
	if got := len(m.tbl.Rows); got != 2 {
		t.Fatalf("rows after a selection = %d, want the plain 2 catalog rows", got)
	}
	click(mktParentRow(t, m, "item-stub-ore"))
	if got := len(m.tbl.Rows); got != 2 {
		t.Fatalf("rows after a second selection = %d, want the plain 2 catalog rows", got)
	}
	if got := len(m.book); got != 4 {
		t.Fatalf("book rows = %d, want the selected item's 4", got)
	}
}

// TestMarketTreeForgetsStateOnClose: the expanded branches, the tree
// selection and the search query are session-only — a closed window (the
// kit hides it on close) forgets them and reopens on All. They are never
// written to a save or a layout sidecar.
func TestMarketTreeForgetsStateOnClose(t *testing.T) {
	m, _, _, frame, _ := mktTreeEnv(t)
	frame()
	frame()

	// The browse branch state and the search query are session-only.
	m.branch, m.treeSel = "Modules", "Modules/High"
	m.expanded["Modules"] = true
	m.search.Insert("turret")

	// What a committed × close does: the host's Dismissed hook runs once
	// while the close fades, then the panel syncs once more.
	m.Dismissed()
	m.Sync()
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
		t.Fatalf("reopened rows = %d, want the plain 2 catalog rows", got)
	}
}

// TestMarketTreeOwnedRowsTint: the player's own resting orders wear the
// owned-row green in the right pane's book, NPC orders keep their side
// colour.
func TestMarketTreeOwnedRowsTint(t *testing.T) {
	m, _, _, frame, click := mktTreeEnv(t)
	frame()
	frame()
	click(mktParentRow(t, m, "item-stub-ore"))

	want := theme.OwnedRowTint(m.th.Pal())
	owned, plain := 0, 0
	for _, r := range m.book {
		tint := r.Tint
		switch tint {
		case want:
			owned++
		case m.th.Pal().Warn, m.th.Pal().Info:
			plain++
		default:
			t.Errorf("book row %q tint = %#v, want the owned green or a side colour", r.Cells[3], tint)
		}
	}
	if owned != 2 || plain != 2 {
		t.Errorf("book tints: %d owned, %d plain, want 2/2", owned, plain)
	}
}

// TestMarketTreeBuyOrderFromGrayedRow: Buy order works from an item
// with no orders anywhere — the row's menu emits the order-window launch
// for that item, the way the traded rows do.
func TestMarketTreeBuyOrderFromGrayedRow(t *testing.T) {
	m, cv, rec, frame, _ := mktTreeEnv(t)
	frame()
	frame()
	row := m.tbl.Rows[mktParentRow(t, m, "item-stub-empty")]
	if row.TextTint != m.th.Pal().Dim {
		t.Fatal("fixture: the requested row is not the grayed one")
	}

	entries := widgets.RowMenuEntries(cv, rec, row.Spec)
	menu := widgets.NewMenu(m.th, nil)
	menu.Open(entries, image.Pt(10, 10))
	found := false
	for i, e := range menu.Entries() {
		if e.Label != "Buy order" {
			continue
		}
		found = true
		if e.Disabled {
			t.Fatal("Buy order grayed on a live item row")
		}
		menu.Click(menu.RowRect(i).Min.Add(image.Pt(1, 1)))
	}
	if !found {
		t.Fatalf("menu has no Buy order entry: %+v", menu.Entries())
	}
	if len(rec.got) != 1 {
		t.Fatalf("emitted %#v, want one OpenOrder", rec.got)
	}
	if got := rec.got[0]; got != (widgets.OpenOrder{Item: "item-stub-empty", Side: sim.OrderBuy, Qty: 1}) {
		t.Errorf("Buy order emitted %#v, want OpenOrder{buy item-stub-empty 1}", got)
	}
}

// TestMarketTreeSortsItems: a header sort orders the catalog's item rows
// by that column's cell — the tree has no order children to keep together
// any more.
func TestMarketTreeSortsItems(t *testing.T) {
	m, _, _, frame, _ := mktTreeEnv(t)
	frame()
	frame()

	before := m.tbl.Rows
	m.tbl.SetSort("Qty", true)
	after := m.tbl.RowsForTest()
	if len(after) != len(before) {
		t.Fatalf("sort changed the row count: %d → %d", len(before), len(after))
	}
	// Qty sorting is the cell sort it always was: the item with 0 units
	// first, the traded one (17) last.
	if got := after[0].Data.(mktItemRow).Item; got != "item-stub-empty" {
		t.Errorf("first item after a Qty sort = %q, want item-stub-empty (0)", got)
	}
	if got := after[len(after)-1].Data.(mktItemRow).Item; got != "item-stub-ore" {
		t.Errorf("last item after a Qty sort = %q, want item-stub-ore (17)", got)
	}
}
