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
	c       *content.Content
	orders  []sim.MarketStock
	docked  bool
	wallet  float64
	station widgets.StationInfo // docked-at identity + services (nil = none)
}

func (v *stubView) Defs() *content.Content { return v.c }

func (v *stubView) PlayerDocked() bool { return v.docked }

// DockedStation hands the stub's station back while "docked" — the read
// the services view and the rail's gray-out share.
func (v *stubView) DockedStation() (widgets.StationInfo, bool) {
	if !v.docked {
		return widgets.StationInfo{}, false
	}
	return v.station, true
}

func (v *stubView) DockedMarket() sim.StationID { return "cel-stub" }

// PlayerStock is the player's holdings read the market row uses for the
// order window's sell default (a fixed 5 in the stub).
func (v *stubView) PlayerStock(content.ItemID) int { return 5 }

func (v *stubView) BookOrders(sim.StationID) []sim.MarketStock { return v.orders }

func (v *stubView) Market(content.SystemID) []sim.MarketStock { return v.orders }

func (v *stubView) System() *content.SystemDef { return &content.SystemDef{ID: "sys-stub"} }

func (v *stubView) MarketPrice(sim.StationID, content.ItemID) float64 { return 42 }

func (v *stubView) Wallet() float64 { return v.wallet }

func (v *stubView) InfoRefOf(data any) (widgets.InfoRef, bool) {
	if id, ok := data.(content.ItemID); ok {
		return widgets.ItemRef(id), true
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
	m := NewMarket(widgets.NewSeams(th, cv, rec))
	m.Sync() // reads the stub everywhere; a sim in the path would panic

	if len(m.tbl.Rows) != 2 {
		t.Fatalf("rows = %d, want the 2 stub orders", len(m.tbl.Rows))
	}
	if got := m.tbl.Rows[0].Cells[0]; got != "Stub Ore" {
		t.Errorf("item cell = %q, want the stub def's name", got)
	}
	if want := "Market — " + theme.FormatCredits(1234.5); m.title.Label != want {
		t.Errorf("title = %q, want %q (the stub wallet)", m.title.Label, want)
	}

	// The row's menu presses through a REAL Menu: the derived entries
	// route their intents into the sink, never a sim.
	entries := widgets.RowMenuEntries(cv, rec, m.tbl.Rows[0].Spec)
	menu := widgets.NewMenu(th)
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
	if b, ok := rec.got[0].(widgets.Buy); !ok || b.Item != "item-stub-ore" || b.Qty != 1 {
		t.Fatalf("intent = %#v, want Buy{item-stub-ore 1}", rec.got[0])
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

	m := widgets.NewMenu(th)
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
