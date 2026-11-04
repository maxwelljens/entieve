package screens

import (
	"math"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
)

// TestMarketSectionHeaders pins the right pane's section headers to the
// infowin bar anatomy: the glyph cell, the tracked label at the small step
// (larger than the old tiny one) and the hairline rule after the text.
func TestMarketSectionHeaders(t *testing.T) {
	m, _, _, frame, _ := mktTreeEnv(t)
	frame()
	frame()

	tiny := (*m.th.Face(theme.StepTiny)).Metrics()
	small := (*m.th.Face(theme.StepSmall)).Metrics()
	tinyH := int(math.Ceil(tiny.HAscent + tiny.HDescent))
	smallH := int(math.Ceil(small.HAscent + small.HDescent))
	if smallH <= tinyH {
		t.Fatalf("fixture: small step (%d px) is not larger than tiny (%d px)", smallH, tinyH)
	}
	for _, c := range []struct {
		name string
		bar  *widget.Container
		lbl  *theme.ChromeText
		want string
	}{
		{"book", m.bookBar, m.bookLbl, "ORDER BOOK"},
		{"mine", m.mineBar, m.mineLbl, "MY ORDERS · " + theme.FormatInt(int64(len(m.mineRows)))},
	} {
		if got := c.lbl.Label; got != c.want {
			t.Errorf("%s header = %q, want %q", c.name, got, c.want)
		}
		if got := len(c.bar.Children()); got != 3 {
			t.Errorf("%s bar children = %d, want the mark, label and rule", c.name, got)
		}
		if _, h := c.lbl.PreferredSize(); h < smallH {
			t.Errorf("%s label line = %d px, want the small step's %d px", c.name, h, smallH)
		}
		if rule := c.bar.Children()[2].GetWidget().Rect; rule.Dx() <= 0 {
			t.Errorf("%s rule = %v, want a separator after the label", c.name, rule)
		}
	}
}

// TestMarketItemHeadLeads pins the pane's lead-in: the selected item's hero
// icon at the Show Info item size and its own description above the book;
// no selection hides both and shows the hint.
func TestMarketItemHeadLeads(t *testing.T) {
	env, _ := newTestEnv(t)
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())
	frame := func() { uitest.PumpWindow(t, ui, m.Sync, 1024, 700) }
	frame()
	frame()

	if !m.headHint.GetWidget().IsVisible() || m.headIcon.Widget().GetWidget().IsVisible() {
		t.Fatal("empty pane = hint hidden or icon shown, want the hint alone")
	}
	m.selectItem("min-silex")
	frame()
	if !m.headIcon.Widget().GetWidget().IsVisible() || m.headHint.GetWidget().IsVisible() {
		t.Fatal("selected pane = icon hidden or hint shown, want the item's head")
	}
	// The item's own description, wrapped inside the pane's inset.
	prose := m.cv.Defs().Items["min-silex"].Description
	if prose == "" {
		t.Fatal("fixture: item-stub-ore carries no description")
	}
	if got := m.headProse.Label; got != prose {
		t.Errorf("head description = %q, want the content's own %q", got, prose)
	}
	if want := float64(m.sideInnerW()); m.headProse.MaxWidth != want {
		t.Errorf("description wrap width = %v, want the pane's inner %v", m.headProse.MaxWidth, want)
	}
	if got := m.headProse.GetWidget().Rect.Dx(); got > m.sideScroll.ViewRect().Dx() {
		t.Errorf("description %d px wide overflows the %d px viewport", got, m.sideScroll.ViewRect().Dx())
	}
	// The head leads: it sits above the book's header bar.
	if head, bar := m.headBox.GetWidget().Rect, m.bookBar.GetWidget().Rect; head.Max.Y > bar.Min.Y {
		t.Errorf("head %v overlaps the book bar %v (want the head first)", head, bar)
	}
}

// TestMarketBookTable pins the book's table form: one aligned [side, price,
// qty] row per resting order, sell orders price-ascending before buy orders
// price-descending, the figures ending on the table's right edge.
func TestMarketBookTable(t *testing.T) {
	m, _, _, frame, click := mktTreeEnv(t)
	frame()
	frame()
	click(mktParentRow(t, m, "item-stub-ore"))
	frame()

	want := []struct {
		side  string
		price float64
		qty   string
	}{
		{"Sell", 100, "5"},
		{"Sell", 120, "7"},
		{"Buy", 90, "2"},
		{"Buy", 80, "3"},
	}
	if got, w := len(m.book), len(want); got != w {
		t.Fatalf("book rows = %d, want %d", got, w)
	}
	askPhase := true
	for i, w := range want {
		r := m.book[i]
		if len(r.Cells) != 3 {
			t.Fatalf("book row %d cells = %v, want side/price/qty", i, r.Cells)
		}
		if r.Cells[0] != w.side || r.Cells[2] != w.qty {
			t.Errorf("book row %d = %v, want %s/%s", i, r.Cells, w.side, w.qty)
		}
		if got := parseCredits(t, r.Cells[1]); got != w.price {
			t.Errorf("book row %d price = %v, want %v", i, got, w.price)
		}
		switch r.Cells[0] {
		case "Sell":
			if !askPhase {
				t.Errorf("book row %d: ask after a bid (order moved)", i)
			}
		case "Buy":
			askPhase = false
		default:
			t.Errorf("book row %d side = %q", i, r.Cells[0])
		}
	}
	// Rendered through the plain table: every row's price and qty cells end
	// on the table's right edge (the aligned columns, not a string).
	for i := range want {
		row := m.bookTbl.RowWidgetForTest(i).GetWidget().Rect
		qtyCell := m.bookTbl.RowWidgetForTest(i).Children()[2].GetWidget().Rect
		if qtyCell.Max.X != row.Max.X {
			t.Errorf("book row %d qty ends at %d, want the table edge %d", i, qtyCell.Max.X, row.Max.X)
		}
	}
	// The pane's own texts stay free of the old verdict/count wording.
	for _, s := range []string{m.bookLbl.Label, m.mineLbl.Label, m.headName.Label, m.headProse.Label} {
		low := strings.ToLower(s)
		if strings.Contains(low, "ready") || strings.Contains(low, "short") {
			t.Errorf("market pane renders %q", s)
		}
	}
}
