package screens

import (
	"image/color"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/icons"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// parseCredits reads back a FormatCredits cell ("1,234.56 €", "50¢").
func parseCredits(t *testing.T, s string) float64 {
	t.Helper()
	if strings.HasSuffix(s, "¢") {
		cents, err := strconv.ParseFloat(strings.TrimSuffix(s, "¢"), 64)
		if err != nil {
			t.Fatalf("bad cents cell %q: %v", s, err)
		}
		return cents / 100
	}
	s = strings.TrimSuffix(strings.TrimSpace(s), " €")
	s = strings.ReplaceAll(s, ",", "")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("bad credits cell %q: %v", s, err)
	}
	return v
}

// TestMarketRowIcons pins the market's row glyphs: every catalog item row
// (branch view or search hit) carries a 20 px icon — a module's own icon,
// else the item's icon (drop-in art, else its category silhouette) — while
// the browse-tree branch rows and the right pane's compact book rows stay
// text-only.
func TestMarketRowIcons(t *testing.T) {
	env, _ := newTestEnv(t)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	uitest.PumpWindow(t, ui, m.Sync, 1024, 700)

	seen := map[string]bool{}
	for _, r := range m.tbl.Rows {
		it, ok := r.Data.(mktItemRow)
		if !ok {
			continue
		}
		if r.IconSize != mktRowIcon {
			t.Errorf("%s: icon size = %d, want %d", it.Item, r.IconSize, mktRowIcon)
		}
		if len(r.Icons) != 1 || r.Icons[0] != m.itemIcon(it.Item) {
			t.Errorf("%s: icon = %v, want %q", it.Item, r.Icons, m.itemIcon(it.Item))
		}
		seen[r.Icons[0]] = true
	}
	for _, want := range []string{"mod-autocannon-s", "ammo", "loot-navdata", "min-ferrite", "ore-sterngrit", theme.HullIcon} {
		if !seen[want] {
			t.Errorf("no catalog row carries %q (saw %v)", want, seen)
		}
	}

	// Branch rows (All, Ammunition, …) are text-only.
	for _, r := range m.tree.Rows {
		if len(r.Icons) != 0 || r.IconSize != 0 {
			t.Errorf("tree row %q carries an icon (%v)", r.Cells, r.Icons)
		}
	}

	// The right pane's book rows are aligned table cells: the head above
	// already carries the item's art, so repeating it down every order is
	// noise.
	clickRow(ui, m.tbl, 0)
	uitest.PumpWindow(t, ui, m.Sync, 1024, 700)
	if len(m.book) == 0 {
		t.Fatal("fixture: no book rows for the first selected item")
	}
	for i, r := range m.book {
		if len(r.Cells) != 3 {
			t.Errorf("book row %d cells = %v, want side/price/qty", i, r.Cells)
		}
	}
}

// TestMarketOrderBookRows pins the catalog and the whole station book: one
// row per item of the tradeable catalog (canonical order, always listed),
// and every order of the station's book present exactly once across the
// items' right-pane books — sell orders (price ascending) above the buy
// orders (price descending) inside each item.
func TestMarketOrderBookRows(t *testing.T) {
	env, _ := newTestEnv(t)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	frame := func() { uitest.PumpWindow(t, ui, m.Sync, 1024, 700) }
	frame()
	frame()

	want := marketCatalog(env)
	if len(m.tbl.Rows) != len(want) {
		t.Fatalf("rows = %d, want one row per catalog item (%d)", len(m.tbl.Rows), len(want))
	}
	for i, r := range m.tbl.Rows {
		if len(r.Cells) != 4 {
			t.Fatalf("row %d has %d cells, want 4 (Item/Price/Qty/Side)", i, len(r.Cells))
		}
		row, ok := r.Data.(mktItemRow)
		if !ok {
			t.Fatalf("row %d payload is %#v, want an item row", i, r.Data)
		}
		if row.Item != want[i] {
			t.Fatalf("row %d item = %q, want %q (canonical catalog order)", i, row.Item, want[i])
		}
		if r.Cells[3] != mktNoSide {
			t.Errorf("row %d side = %q, want %q", i, r.Cells[3], mktNoSide)
		}
	}

	// Selecting each item in turn shows the station's whole book, in book
	// order inside the item: sell orders ascending, then buy orders
	// descending.
	sys := env.Sim.System()
	if sys == nil {
		t.Fatal("no current system")
	}
	orders := env.Sim.Market(sys.ID)
	if len(orders) == 0 {
		t.Fatal("fixture: empty book")
	}
	seen := 0
	for _, item := range want {
		m.selectItem(item)
		frame()
		if got := len(m.tbl.Rows); got != len(want) {
			t.Fatalf("%q: catalog rows = %d, want the plain %d", item, got, len(want))
		}
		var ords []sim.MarketStock
		for _, o := range orders {
			if o.Item == item {
				ords = append(ords, o)
			}
		}
		if len(m.book) != len(ords) {
			t.Fatalf("%q: book rows = %d, want the item's %d resting orders", item, len(m.book), len(ords))
		}
		askPhase, lastAsk, lastBid := true, -1.0, -1.0
		for i, r := range m.book {
			seen++
			side := r.Cells[0]
			price := parseCredits(t, r.Cells[1])
			switch side {
			case "Sell":
				if !askPhase {
					t.Fatalf("%q row %d: ask after bids (book out of order)", item, i)
				}
				if price < lastAsk {
					t.Fatalf("%q row %d: ask %v below a previous ask %v", item, i, price, lastAsk)
				}
				lastAsk = price
			case "Buy":
				askPhase = false
				if lastBid >= 0 && price > lastBid {
					t.Fatalf("%q row %d: bid %v above a previous bid %v", item, i, price, lastBid)
				}
				lastBid = price
			default:
				t.Fatalf("%q row %d side = %q, want Sell or Buy", item, i, side)
			}
		}
	}
	if seen != len(orders) {
		t.Fatalf("book rows = %d, want every resting order (%d) listed once", seen, len(orders))
	}

	// The founding books carry both sides: the showcase station seeds sell
	// orders (hulls, modules, charges, materials) and standing buy orders
	// (loot and the raw materials its industry buys).
	sides := map[string]bool{}
	for _, item := range want {
		m.selectItem(item)
		frame()
		for _, r := range m.book {
			sides[r.Cells[0]] = true
		}
	}
	if !sides["Sell"] || !sides["Buy"] {
		t.Fatalf("books show %v, want both Sell and Buy rows", sides)
	}
}

// TestMarketCatalogScrolls pins the catalog's scrollability (playtest: "the
// marketplace window does not have a scrollbar"), in the pattern the table
// scroll tests use: the list rides the standard ScrollContainer with real
// overflow at the window's own size, a wheel notch moves the offset and the
// rows with it, and the scrollbar rail beside the viewport mirrors the
// offset — and writes drags back — while the title line and the Buy/Sell
// row keep their place above the list.
func TestMarketCatalogScrolls(t *testing.T) {
	env, _ := newTestEnv(t)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	frame := func() { uitest.PumpWindow(t, ui, m.Sync, 1280, 720) }
	frame()
	frame()

	sc := m.tbl.ScrollForTest()
	if sc == nil {
		t.Fatal("market table has no scroll container")
	}
	view := sc.ViewRect()
	if !view.In(m.Win().GetContainer().GetWidget().Rect) {
		t.Fatalf("scroll viewport %v outside the window %v", view, m.Win().GetContainer().GetWidget().Rect)
	}
	rows := m.tbl.RowCts
	if len(rows) < 3 {
		t.Fatalf("rows = %d, want a catalog long enough to overflow", len(rows))
	}
	union := rows[0].GetWidget().Rect
	for _, r := range rows[1:] {
		union = union.Union(r.GetWidget().Rect)
	}
	if union.Dy() <= view.Dy() {
		t.Fatalf("catalog content %d px does not overflow the %d px viewport", union.Dy(), view.Dy())
	}
	rail := m.tbl.Rail()
	if !rail.GetWidget().IsVisible() {
		t.Error("scrollbar hidden over an overflowing catalog")
	}

	// Wheel: the table's scroll handler moves the offset and the rows.
	row0 := rows[0].GetWidget().Rect
	sc.GetWidget().ScrolledEvent.Fire(&widget.WidgetScrolledEventArgs{Y: -3})
	frame()
	frame()
	if sc.ScrollTop <= 0 {
		t.Fatalf("wheel: ScrollTop = %v, want > 0", sc.ScrollTop)
	}
	if got := rows[0].GetWidget().Rect; got.Min.Y >= row0.Min.Y {
		t.Errorf("wheel: row 0 at %d, want above %d (content moved)", got.Min.Y, row0.Min.Y)
	}
	// The rail mirrors the offset the wheel set.
	if want := int(math.Round(sc.ScrollTop * widgets.RailSteps)); rail.Current != want {
		t.Errorf("rail position = %d, want the mirrored ScrollTop %d", rail.Current, want)
	}

	// Drag: the rail writes the offset back through its changed event
	// (what a handle drag fires mid-frame), and the mirror re-converges.
	sc.ScrollTop = 0
	rail.ChangedEvent.Fire(&widget.SliderChangedEventArgs{Slider: rail, Current: 500, Dragging: true})
	ui.Update() // dispatch the deferred event before the next frame's mirror
	frame()
	frame()
	if got := sc.ScrollTop; math.Abs(got-0.5) > 1e-9 {
		t.Errorf("rail drag: ScrollTop = %v, want 0.5", got)
	}
	if rail.Current != 500 {
		t.Errorf("rail position = %d, want 500 (mirror of ScrollTop 0.5)", rail.Current)
	}

	// The trade bar is pinned under the right pane's scroll column, inside
	// that pane, side by side and clear of the catalog.
	title := m.title.GetWidget().Rect
	trade := m.buy.GetWidget().Rect.Union(m.sell.GetWidget().Rect)
	pane := m.sideRail.Container().GetWidget().Rect
	table := m.tbl.Container().GetWidget().Rect
	if title.Max.Y > table.Min.Y {
		t.Errorf("title %v overlaps the catalog %v (want the title above)", title, table)
	}
	if !trade.In(m.Win().GetContainer().GetWidget().Rect) {
		t.Errorf("trade bar %v outside the window", trade)
	}
	if trade.Min.X < pane.Min.X || trade.Max.X > pane.Max.X {
		t.Errorf("trade bar %v not aligned under the right pane %v", trade, pane)
	}
	if scroll := m.sideScroll.ViewRect(); scroll.Max.Y > trade.Min.Y {
		t.Errorf("scroll column %v overlaps the pinned trade bar %v", scroll, trade)
	}
	if trade.Overlaps(table) {
		t.Errorf("trade bar %v overlaps the catalog %v", trade, table)
	}
	for name, b := range map[string]*widget.Button{"Buy": m.buy, "Sell": m.sell} {
		if r := b.GetWidget().Rect; !r.In(trade) {
			t.Errorf("%s button %v outside the trade bar %v", name, r, trade)
		}
	}
	if m.buy.GetWidget().Rect.Overlaps(m.sell.GetWidget().Rect) {
		t.Error("Buy and Sell overlap, want them side by side")
	}
}

// TestMarketCatalogWheelSettles: a fast flick on the catalog lands and
// stays. Two notches on consecutive frames, then silence: the offset may
// not reverse and the rows may not jump — the rail's mirror and its
// Changed write-back locked two frames apart and the list oscillated one
// wheel step every frame.
func TestMarketCatalogWheelSettles(t *testing.T) {
	env, _ := newTestEnv(t)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	frame := func() { uitest.PumpWindow(t, ui, m.Sync, 1024, 700) }
	frame()
	frame()

	sc := m.tbl.ScrollForTest()
	for i := 0; i < 2; i++ {
		sc.GetWidget().ScrolledEvent.Fire(&widget.WidgetScrolledEventArgs{Y: -1})
		frame()
	}
	var tops []float64
	for f := 0; f < 30; f++ {
		frame()
		tops = append(tops, sc.ScrollTop)
	}
	if tops[len(tops)-1] <= 0 {
		t.Fatalf("the flick scrolled nothing (ScrollTop %.5f)", tops[len(tops)-1])
	}
	for i := 2; i < len(tops); i++ {
		if (tops[i]-tops[i-1])*(tops[i-1]-tops[i-2]) < 0 {
			t.Fatalf("offset reversed with no wheel input at frame %d: %.5f -> %.5f -> %.5f",
				i, tops[i-2], tops[i-1], tops[i])
		}
	}
	// ebitenui re-anchors the rows from ScrollTop every frame: a still
	// frame with a moving content top is the jump the user sees.
	last := sc.ContentRect().Min.Y
	for f := 0; f < 10; f++ {
		frame()
		y := sc.ContentRect().Min.Y
		if y != last {
			t.Fatalf("catalog content jumped from y=%d to y=%d with no input", last, y)
		}
	}
}

// TestMarketTitleIsTheBaseName: the title bar is furniture — the window's
// base display name, before and after a selection, whatever the wallet or
// the selected item's ladder quote. The figures live in the body's own
// readouts, never in the bar.
func TestMarketTitleIsTheBaseName(t *testing.T) {
	env, _ := newTestEnv(t)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	frame := func() { uitest.PumpWindow(t, ui, m.Sync, 1024, 700) }
	frame()
	frame()

	if got := m.title.Label; got != "Market" {
		t.Fatalf("title = %q, want the base name Market", got)
	}

	clickRow(ui, m.tbl, 0)
	frame()
	if !m.hasSel {
		t.Fatal("selection did not take")
	}
	if got := m.title.Label; got != "Market" {
		t.Fatalf("title after selecting %s = %q, want the base name Market", m.sel, got)
	}
}

// TestPickerReadoutFitsItsValue (Defect B regression, carried over from the
// retired ± ticker): the picker's readout resizes with its value — the box
// always measures at least the text — and never reaches into the slider,
// at scale 1 and 1.5.
func TestPickerReadoutFitsItsValue(t *testing.T) {
	defer theme.SetScale(1)
	for _, sc := range []float64{1, 1.5} {
		theme.SetScale(sc)
		env, _ := newTestEnv(t)
		inv := NewInventory(env)
		ui := newQtyUI(t, env, inv.Win())
		q := env.QtyWindow()
		pumpQty(t, ui, env, inv.Sync, 1280, 720)

		check := func(want string) {
			t.Helper()
			pumpQty(t, ui, env, inv.Sync, 1280, 720)
			lbl := q.read.GetWidget().Rect
			slide := q.Slider().GetWidget().Rect
			if got := q.read.Label; got != want {
				t.Errorf("scale %v: readout = %q, want %q", sc, got, want)
			}
			// The box measures the value itself (same face, no h-padding).
			need, _ := text.Measure(want, *env.FaceMono(theme.StepSmall), 0)
			if float64(lbl.Dx()) < need {
				t.Errorf("scale %v: readout box %d wide, need ≥ %v for %q",
					sc, lbl.Dx(), need, want)
			}
			if lbl.Min.X < slide.Max.X {
				t.Errorf("scale %v: readout (%v) overlaps the slider (%v)", sc, lbl, slide)
			}
		}

		// Open the picker on the cargo stack and walk its value.
		dockAtStation(t, env.Sim)
		clickRow(ui, inv.cargo, 0)
		pumpQty(t, ui, env, inv.Sync, 1280, 720)
		inv.sellClicked()
		pumpQty(t, ui, env, inv.Sync, 1280, 720)
		if !q.IsOpen() {
			t.Fatalf("scale %v: no picker up", sc)
		}
		check("1")
		q.Slider().SetValue(qtyMax)
		pumpQty(t, ui, env, inv.Sync, 1280, 720)
		check(theme.FormatInt(qtyMax))
		q.Close()
	}
}

// TestMarketTradeButtons pins the pane's action bar: Buy and Sell sit side by
// side with their own glyphs, tinted Good and Danger respectively on the
// icon states (the label carries the same role), and grey rather than hide
// while gated.
func TestMarketTradeButtons(t *testing.T) {
	env, _ := newTestEnv(t)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	frame := func() { uitest.PumpWindow(t, ui, m.Sync, 1024, 700) }
	frame()
	frame()

	p := theme.DefaultPalette()
	for _, c := range []struct {
		name string
		btn  *widget.Button
		icon string
		tint color.RGBA
	}{
		{"Buy", m.buy, "act_loot", p.Good},
		{"Sell", m.sell, "ctr_deliver", p.Danger},
	} {
		g := c.btn.GraphicImage()
		if g == nil || g.Idle == nil {
			t.Fatalf("%s carries no icon", c.name)
		}
		if want := icons.Get(c.icon, theme.Px(theme.IconButton), c.tint); g.Idle != want {
			t.Errorf("%s glyph = %q, want %q tinted %v", c.name, c.icon, c.icon, c.tint)
		}
		if c.btn.Text().Label != c.name {
			t.Errorf("%s label = %q", c.name, c.btn.Text().Label)
		}
	}
	if m.buy.GraphicImage().Idle == m.sell.GraphicImage().Idle {
		t.Error("Buy and Sell share one glyph, want an honest pair")
	}
	if m.buy.GetWidget().Rect.Overlaps(m.sell.GetWidget().Rect) {
		t.Error("trade buttons overlap")
	}
	// Undocked: visible but greyed (never hidden).
	if !m.buy.GetWidget().IsVisible() || !m.sell.GetWidget().IsVisible() {
		t.Error("trade buttons hidden while gated")
	}
	if !m.buy.GetWidget().Disabled || !m.sell.GetWidget().Disabled {
		t.Error("trade buttons armed while undocked")
	}
}
