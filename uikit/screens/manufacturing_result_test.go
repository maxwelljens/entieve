package screens

import (
	"image"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// The RESULT section's pins: the inspector ends on what one run actually
// makes, one row per output, the quantity read from the recipe (the figure
// the bill and the profit table never showed: "is one round or a hundred made
// per job?").

// TestManufacturingResultSection: the section sits after the bill, carries
// the one section-bar anatomy, and its row names the product with its art and
// its per-run quantity — the content's own Qty, never a hardcoded number.
func TestManufacturingResultSection(t *testing.T) {
	f, env, ui, _, frame := boardEnv(t)

	// Nothing selected: the head's empty state stands, no RESULT section.
	if f.resBar != nil || len(f.resRows) != 0 {
		t.Fatalf("empty inspector draws a RESULT section (bar %v, %d rows)", f.resBar, len(f.resRows))
	}

	// The three families: ammunition makes the content's batch, a module and
	// a hull one unit each.
	for _, c := range []struct {
		product content.ItemID
		qty     int
	}{
		{"ammo-art-ap-s", 100},
		{"mod-artillery-l", 1},
		{"ship-husnek", 1},
	} {
		rec, ok := env.Content.Recipes[recipeIDFor(env, c.product)]
		if !ok {
			t.Fatalf("fixture: no recipe for %q", c.product)
		}
		if rec.Qty != c.qty {
			t.Fatalf("fixture: %s qty = %d, want the content's %d", rec.ID, rec.Qty, c.qty)
		}
		clickRow(ui, f.recipes, recipeRowFor(f, c.product))
		frame()

		if f.resBar == nil {
			t.Fatalf("%s: no RESULT section", c.product)
		}
		if len(f.resRows) != 1 {
			t.Fatalf("%s: %d result rows, want the recipe's one output", c.product, len(f.resRows))
		}
		row := f.resRowDef(mfgOutput{Item: rec.Product, Qty: rec.Qty}, theme.DefaultPalette())
		if got, want := row.Cells[0], env.TradeableName(rec.Product); got != want {
			t.Errorf("%s: row name = %q, want %q", c.product, got, want)
		}
		if got, want := row.Icons[0], widgets.TradeableIcon(env.Content, rec.Product); got != want {
			t.Errorf("%s: row icon = %q, want %q", c.product, got, want)
		}
		if row.IconSize != mfgMatIcon {
			t.Errorf("%s: row icon box = %d, want the bill's %d", c.product, row.IconSize, mfgMatIcon)
		}
		// The figure is the content's qty in the house number format.
		want := "×" + theme.FormatInt(int64(c.qty))
		segs := row.Stack[len(row.Stack)-1].Segs
		if len(segs) == 0 || segs[len(segs)-1].Text != want {
			t.Fatalf("%s: row's figure line = %+v, want %q", c.product, segs, want)
		}
		if segs[len(segs)-1].Tint != theme.DefaultPalette().Accent {
			t.Errorf("%s: figure tint = %v, want the accent", c.product, segs[len(segs)-1].Tint)
		}
	}

	// A recipe with no authored qty dashes the figure rather than reading as
	// a free run (the section never invents a quantity).
	if got := f.resRowDef(mfgOutput{Item: "ammo-art-ap-s"}, theme.DefaultPalette()); lastSeg(got).Text != "—" {
		t.Errorf("no-qty figure = %q, want the dash", lastSeg(got).Text)
	}
}

// recipeIDFor is the shipped recipe whose product is p (the block's fixtures
// name products, not files).
func recipeIDFor(env *testEnv, p content.ItemID) content.RecipeID {
	for _, rec := range env.Content.RecipesInOrder() {
		if rec.Product == p {
			return rec.ID
		}
	}
	return ""
}

// lastSeg is a row's last run of its last line (the figure).
func lastSeg(r widgets.TableRow) widgets.RowSeg {
	segs := r.Stack[len(r.Stack)-1].Segs
	return segs[len(segs)-1]
}

// TestManufacturingResultOrder: the inspector reads head, PROFIT, MATERIALS,
// RESULT, and Start is pinned below the scroll viewport, so the new section
// never pushes the button out of the pane.
func TestManufacturingResultOrder(t *testing.T) {
	f, _, ui, _, frame := boardEnv(t)
	clickRow(ui, f.recipes, recipeRowFor(f, "mod-artillery-l"))
	// The capture's own height: the card grown to the height the whole page
	// needs (the landing height is short for the head alone).
	f.Win().SetLocation(image.Rect(theme.Px(140), theme.Px(16),
		theme.Px(140+mfgDefaultW), theme.Px(16+mfgDefaultH+124)))
	frame()
	frame()

	kids := f.matsBox.Children()
	pos := func(w any) int {
		for i, k := range kids {
			if k == w {
				return i
			}
		}
		return -1
	}
	firstBill := pos(f.matsRows[0].Root)
	lastBill := pos(f.matsRows[len(f.matsRows)-1].Root)
	res := pos(f.resBar)
	resRow := pos(f.resRows[0].Root)
	if firstBill < 0 || lastBill < 0 || res < 0 || resRow < 0 {
		t.Fatalf("sections missing from the page: %d/%d/%d/%d", firstBill, lastBill, res, resRow)
	}
	if !(firstBill < res && lastBill < res && res < resRow) {
		t.Errorf("page order = bill %d..%d, RESULT bar %d, row %d; want bill < RESULT bar < row",
			firstBill, lastBill, res, resRow)
	}
	if last := pos(f.profit.Container()); last < 0 || last > firstBill {
		t.Errorf("profit table at %d, want it above the bill's first row %d", last, firstBill)
	}

	// The pinned Start stays below the viewport (the section scrolls, it does
	// not push the button).
	view := f.matsScroll.ViewRect()
	start := f.startBtn.GetWidget().Rect
	if view.Empty() || start.Empty() || start.Min.Y < view.Max.Y {
		t.Errorf("Start %v sits inside the %v viewport, want it pinned below", start, view)
	}
	if got := len(f.resBar.Children()); got != 3 {
		t.Errorf("RESULT bar has %d cells, want the mark/label/rule anatomy", got)
	}
	if got := f.resBar.Children()[1].(*theme.ChromeText).Label; got != "RESULT · one run" {
		t.Errorf("RESULT bar label = %q, want the one-run basis", got)
	}
	if got, _ := theme.SectionBar(theme.DefaultPalette(), "RESULT"); got != "svc_manufacturing" {
		t.Errorf("RESULT bar glyph = %q, want the factory mark", got)
	}
}

// TestManufacturingResultFitsFloor: the result row stays inside the bill's
// viewport at the window's floor, the longest product name included.
func TestManufacturingResultFitsFloor(t *testing.T) {
	f, env, ui, _, frame := boardEnv(t)

	longest := env.Content.RecipesInOrder()[0]
	for _, rec := range env.Content.RecipesInOrder() {
		if n := len([]rune(env.TradeableName(rec.Product))); n > len([]rune(env.TradeableName(longest.Product))) {
			longest = rec
		}
	}
	clickRow(ui, f.recipes, recipeRowFor(f, longest.Product))
	frame()

	f.Win().SetLocation(image.Rect(0, 0, theme.Px(640), theme.Px(460)))
	frame()
	frame()

	view := f.matsScroll.ViewRect()
	if view.Empty() {
		t.Fatal("no bill viewport at the floor")
	}
	if len(f.resRows) != 1 {
		t.Fatalf("%d result rows at the floor, want 1", len(f.resRows))
	}
	row := f.resRows[0].Root.GetWidget().Rect
	if row.Empty() {
		t.Fatal("result row never laid out at the floor")
	}
	if row.Dx() > view.Dx() {
		t.Errorf("result row %d px wide overflows the %d px viewport at the floor", row.Dx(), view.Dx())
	}
	// The name clips, it does not vanish: the row keeps its name cell and its
	// figure at the floor.
	def := f.resRowDef(mfgOutput{Item: longest.Product, Qty: longest.Qty}, theme.DefaultPalette())
	if def.Cells[0] == "" || lastSeg(def).Text == "" {
		t.Errorf("result row at the floor lost its name %q or figure %q", def.Cells[0], lastSeg(def).Text)
	}
}
