package screens

import (
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"image"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
)

// Seam purity (CONTEXT.md: Theme / ContentView / ActionSink): screens
// construct from the narrow seams they use — a stub view and a recording
// sink are enough to run one, with no *sim.Sim anywhere in the test —
// and a Theme-only widget cannot reach the view at all (its signature
// has no room for one).

// recordingSink is the ActionSink a seam-purity test wires in: every
// intent a screen emits lands here, in order.
type recordingSink struct{ got []widgets.Intent }

func (r *recordingSink) Do(i widgets.Intent) { r.got = append(r.got, i) }

// stubView is a hand-written ContentView: the defs registry the market
// renders from plus exactly the reads the market makes. It embeds a nil
// ContentView, so a read the market grows beyond this list panics here
// instead of quietly finding a sim — no simulation exists in this test.
type stubView struct {
	widgets.ContentView
	c        *content.Content
	orders   []sim.MarketStock
	docked   bool
	wallet   float64
	station  widgets.StationInfo // docked-at identity (nil = none)
	svc      map[string]bool     // service access table (nil = every service while docked)
	contacts widgets.Contacts    // the docked contact view read
	deliver  float64             // deliver-objective progress the stub reports
}

func (v *stubView) Defs() *content.Content { return v.c }

func (v *stubView) PlayerDocked() bool { return v.docked }

// ServiceAvailable is the stub's access table: undocked is nothing, docked
// with no table is every service, and a named table is exactly what it says
// (the remnant case: market live, industry and fitting dark).
func (v *stubView) ServiceAvailable(svc string) bool {
	if !v.docked {
		return false
	}
	if v.svc == nil {
		return true
	}
	return v.svc[svc]
}

// Contacts hands the stub's read to the contact panel.
func (v *stubView) Contacts() widgets.Contacts { return v.contacts }

// MeetsStanding applies the stub board's own standing, epsilon included —
// the same gate the view derives from the sim.
func (v *stubView) MeetsStanding(min float64) bool { return v.contacts.Standing+1e-9 >= min }

// DeliverProgress reports the stub's fixed deliver count (the board's
// active progress line).
func (v *stubView) DeliverProgress(*content.ContractDef) float64 { return v.deliver }

// DockedStation hands the stub's station back while "docked" — the read
// the services view and the rail's gray-out share.
func (v *stubView) DockedStation() (widgets.StationInfo, bool) {
	if !v.docked {
		return widgets.StationInfo{}, false
	}
	return v.station, true
}

// Market is the stub's market domain: the fixed book, the fixed ladder and
// the stub's own docked rule. The adapter embeds a nil MarketView, so a read
// beyond this list panics here instead of quietly finding a sim.
func (v *stubView) Market() widgets.MarketView { return stubMarket{v: v} }

// stubMarket is the stub's market reads (a dock that runs the market: an
// explicit service table without "market" is a station that does not).
type stubMarket struct {
	widgets.MarketView
	v *stubView
}

func (m stubMarket) Docked() sim.StationID {
	if !m.v.docked || (m.v.svc != nil && !m.v.svc["market"]) {
		return ""
	}
	return "cel-stub"
}

func (m stubMarket) Orders(sim.StationID) []sim.MarketStock { return m.v.orders }

func (m stubMarket) System(content.SystemID) []sim.MarketStock { return m.v.orders }

func (m stubMarket) PriceLadder(sim.StationID, content.ItemID) float64 { return 42 }

// StationName resolves a station id to the stub's own spelling: the Details
// dialog's destination and claim lines read it.
func (v *stubView) StationName(st sim.StationID) string { return string(st) }

// PlayerStock is the player's holdings read the market row uses for the
// order window's sell default (a fixed 5 in the stub).
func (v *stubView) PlayerStock(content.ItemID) int { return 5 }

func (v *stubView) System() *content.SystemDef { return &content.SystemDef{ID: "sys-stub"} }

func (v *stubView) TradeableName(id content.ItemID) string {
	return widgets.TradeableName(v.c, id)
}

func (v *stubView) Wallet() float64 { return v.wallet }

func (v *stubView) InfoRefOf(data any) (widgets.InfoRef, bool) {
	switch d := data.(type) {
	case content.ItemID:
		return widgets.ItemRef(d), true
	case widgets.InfoRef:
		return d, d.Valid()
	}
	return widgets.InfoRef{}, false
}

// TestMarketRunsOnStubView: the market screen builds, syncs and trades
// against a stub ContentView and a recording sink — the L4 seam: no sim,
// no game, and every emitted payload is a typed intent.
func TestMarketRunsOnStubView(t *testing.T) {
	th := theme.NewSeam(theme.DefaultPalette(), theme.DefaultTypeScale())
	cv := &stubView{
		c: &content.Content{Items: map[content.ItemID]*content.ItemDef{
			"item-stub-ore": {Name: "Stub Ore", Category: content.CatOre},
		}},
		orders: []sim.MarketStock{
			{Item: "item-stub-ore", Price: 100, Qty: 5, Side: sim.OrderSell},
			{Item: "item-stub-ore", Price: 90, Qty: 2, Side: sim.OrderBuy},
		},
		docked: true,
		wallet: 1234.5,
	}
	rec := &recordingSink{}
	m := NewMarket(widgets.NewSeams(th, cv, rec, nil))
	m.Sync() // reads the stub everywhere; a sim in the path would panic

	if len(m.tbl.Rows) != 1 {
		t.Fatalf("rows = %d, want the stub catalog's one item row", len(m.tbl.Rows))
	}
	if got := m.tbl.Rows[0].Cells[0]; got != "Stub Ore" {
		t.Errorf("item cell = %q, want the stub def's name", got)
	}
	if got := m.title.Label; got != "Market" {
		t.Errorf("title = %q, want the base name Market", got)
	}

	// The row's menu presses through a REAL Menu: the derived entries
	// route their intents into the sink, never a sim.
	entries := widgets.RowMenuEntries(cv, rec, m.tbl.Rows[0].Spec)
	menu := widgets.NewMenu(th, nil)
	pressEntry := func(label string) {
		t.Helper()
		menu.Open(entries, image.Pt(10, 10))
		for i, e := range menu.Entries() {
			if e.Label == label {
				menu.Click(menu.RowRect(i).Min.Add(image.Pt(1, 1)))
				return
			}
		}
		t.Fatalf("menu has no %q entry: %+v", label, menu.Entries())
	}

	pressEntry("Buy")
	if len(rec.got) != 1 {
		t.Fatalf("Buy emitted %#v, want the single intent", rec.got)
	}
	if b, ok := rec.got[0].(widgets.QtyPick); !ok || b.Mode != widgets.QtyBuy || b.Item != "item-stub-ore" {
		t.Fatalf("intent = %#v, want QtyPick{buy item-stub-ore} (the picker asks for the quantity)", rec.got[0])
	}

	// Gated (undocked): the row's verbs gray out and pressing them —
	// through the menu and straight at the entry's closure — emits
	// nothing at all.
	cv.docked = false
	m.Sync()
	entries = widgets.RowMenuEntries(cv, rec, m.tbl.Rows[0].Spec)
	for _, e := range entries {
		if e.Label == "Buy" && !e.Disabled {
			t.Fatal("undocked Buy row is enabled")
		}
	}
	pressEntry("Buy")
	for _, e := range entries {
		if e.Label == "Buy" {
			e.Action() // the installer's own guard, bypassing the menu gate
		}
	}
	if len(rec.got) != 1 {
		t.Fatalf("disabled Buy emitted %#v, want nothing", rec.got)
	}
}

// TestThemeOnlyWidgetsTakeNoView: the context menu, the help window and
// the title bar take a Theme and nothing else — the parameter type is
// the proof they can never touch the view or the sink — and they build
// and behave from it alone.
func TestThemeOnlyWidgetsTakeNoView(t *testing.T) {
	th := theme.NewSeam(theme.DefaultPalette(), theme.DefaultTypeScale())

	m := widgets.NewMenu(th, nil)
	if m.IsOpen() {
		t.Error("a fresh menu is open")
	}
	m.Open([]widgets.MenuEntry{{Label: "Stub"}}, image.Pt(4, 4))
	if !m.IsOpen() || m.Rect().Min != (image.Pt(4, 4)) {
		t.Errorf("menu open=%v origin=%v, want open at the point",
			m.IsOpen(), m.Rect().Min)
	}
	m.Close()

	h := NewHelp(th, []KeyRow{{Key: "I", Desc: "toggle inventory"}})
	if h == nil || h.Win() == nil {
		t.Fatal("help window missing")
	}

	bar, title := theme.NewTitleBar(th, "Stub", "")
	if bar == nil || title == nil || title.Label != "Stub" {
		t.Errorf("title bar = %v/%v, want the labelled bar", bar, title)
	}
}
