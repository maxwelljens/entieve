package screens

import (
	"strings"
	"testing"

	"github.com/entieve/entieve/uikit/theme"
)

// The shared pane seams' pins: the one browse tree both windows embed and the
// one item head both panes lead with. The windows' own integration pins
// (TestMarketTree*, TestMarketItemHeadLeads, TestManufacturingBrowseTree,
// TestManufacturingInspectorHead) prove the same code renders both surfaces.

// TestBrowseTreeRows pins the shared browse column: All at the root, a
// depth-sorted branch list with fold markers, a branch click scoping and
// folding, and the highlight expanding its ancestors.
func TestBrowseTreeRows(t *testing.T) {
	nodes := browseNodes([]string{"ship-b", "mod-a", "ship-a"}, map[string]string{
		"ship-b": "Ships/Frigates",
		"ship-a": "Ships/Cruisers",
		"mod-a":  "Modules/High",
	})
	b := newBrowseTree(nodes)
	if got := len(nodes); got != 5 {
		t.Fatalf("nodes = %d, want the five branches (%+v)", got, nodes)
	}
	rows := b.treeRows()
	opened := []string{"↓ All", "    → Modules", "    → Ships"}
	if len(rows) != len(opened) {
		t.Fatalf("opened rows = %+v, want the root branches %v", rows, opened)
	}
	for i, w := range opened {
		if got := rows[i].Cells[0]; got != w {
			t.Errorf("opened row %d = %q, want %q", i, got, w)
		}
	}
	if k, ok := rows[0].Data.(browseNodeKey); !ok || k.Path != "" {
		t.Fatalf("root data = %+v, want the empty path", rows[0].Data)
	}

	// A branch click scopes the list and unfolds the branch; a second click
	// folds it again.
	b.selectBranch("Ships")
	if b.branch != "Ships" || !b.expanded["Ships"] {
		t.Fatalf("branch = %q expanded = %v, want Ships open", b.branch, b.expanded["Ships"])
	}
	rows = b.treeRows()
	want := []string{"↓ All", "    → Modules", "    ↓ Ships", "          Cruisers", "          Frigates"}
	if len(rows) != len(want) {
		t.Fatalf("rows = %+v, want %v", rows, want)
	}
	for i, w := range want {
		if got := rows[i].Cells[0]; got != w {
			t.Errorf("row %d = %q, want %q", i, got, w)
		}
	}
	b.selectBranch("Ships")
	if b.expanded["Ships"] {
		t.Error("a second click left the branch open")
	}

	// The highlight expands every ancestor of the selected branch.
	b.treeSel = "Ships/Frigates"
	b.highlightTree()
	if !b.expanded["Ships"] || !b.expanded["Ships/Frigates"] {
		t.Errorf("expanded = %v, want the highlight's ancestors open", b.expanded)
	}

	// reset forgets the session state (the committed close).
	b.reset()
	if b.branch != "" || b.treeSel != "" || len(b.expanded) != 1 || !b.expanded[""] {
		t.Errorf("after reset: branch %q highlight %q expanded %v", b.branch, b.treeSel, b.expanded)
	}

	// The branch predicate both catalogs scope with.
	for _, c := range []struct {
		group, branch string
		want          bool
	}{
		{"Ships/Frigates", "", true},
		{"Ships/Frigates", "Ships", true},
		{"Ships/Frigates", "Ships/Frigates", true},
		{"Ships/Frigates", "Modules", false},
		{"Shipships", "Ships", false}, // a prefix only counts at a segment boundary
	} {
		if got := inBranch(c.group, c.branch); got != c.want {
			t.Errorf("inBranch(%q, %q) = %v, want %v", c.group, c.branch, got, c.want)
		}
	}
}

// TestItemHeadBuilds pins the one item head: the hero icon box, the name and
// the description wrapped to the given width, the hint alone with no
// selection, and the name over an empty state.
func TestItemHeadBuilds(t *testing.T) {
	env, _ := newTestEnv(t)
	h := newItemHead(env, "Select something")
	if n := len(h.headBox.Children()); n != 4 {
		t.Fatalf("head cells = %d, want the icon row, name, prose and hint", n)
	}
	if got := h.headIcon.Widget().Image.Bounds().Dx(); got != theme.Px(theme.IconHero) {
		t.Errorf("icon box = %d px, want the %d px hero", got, theme.Px(theme.IconHero))
	}

	// Nothing selected: the hint alone.
	h.set(false, "", "", "", 200)
	if !h.headHint.GetWidget().IsVisible() || h.headIcon.Widget().GetWidget().IsVisible() ||
		h.headName.GetWidget().IsVisible() || h.headProse.GetWidget().IsVisible() {
		t.Fatal("empty head shows more than its hint")
	}

	// Selected: the icon, the name and the description at the given width.
	prose := strings.Repeat("prose ", 40)
	h.set(true, "svc_market", "Name", prose, 200)
	if !h.headIcon.Widget().GetWidget().IsVisible() || h.headHint.GetWidget().IsVisible() {
		t.Fatal("selected head shows the hint or hides the icon")
	}
	if h.headName.Label != "Name" || h.headProse.Label != prose {
		t.Errorf("head = %q / %q, want the name and the prose", h.headName.Label, h.headProse.Label)
	}
	if h.headProse.MaxWidth != 200 {
		t.Errorf("prose wrap = %v, want 200", h.headProse.MaxWidth)
	}

	// An empty state names itself without claiming a selection.
	h.set(false, "", "Nothing selected", "", 200)
	if !h.headName.GetWidget().IsVisible() || h.headIcon.Widget().GetWidget().IsVisible() {
		t.Error("the empty state's name is hidden with the hint")
	}
}

// TestManufacturingInspectorHead: the inspector leads with the product's head
// the same way the market's right pane does: the hero icon, the name with the
// verdict glyph and the product's own description, wrapped to the pane, above
// the profit table.
func TestManufacturingInspectorHead(t *testing.T) {
	f, env, ui, _, frame := boardEnv(t)

	// No selection: the head names the empty state and shows no art.
	if got := inspTitle(f); got != "No recipe selected" {
		t.Errorf("empty inspector title = %q, want the empty state's name", got)
	}
	if f.head.headIcon.Widget().GetWidget().IsVisible() {
		t.Error("the empty inspector shows an icon")
	}

	rec := env.Content.Recipes["rec-mod-artillery-l"]
	clickRow(ui, f.recipes, recipeRowFor(f, rec.Product))
	frame()
	frame()

	icon := f.head.headIcon.Widget()
	if got := icon.Image.Bounds().Dx(); got != theme.Px(theme.IconHero) {
		t.Errorf("inspector icon = %d px, want the market head's %d", got, theme.Px(theme.IconHero))
	}
	if !icon.GetWidget().IsVisible() {
		t.Error("inspector head icon hidden with a recipe selected")
	}
	if got, want := inspTitle(f), env.TradeableName(rec.Product)+"  "+inspVerdict(f); got != want {
		t.Errorf("inspector title = %q, want %q", got, want)
	}
	if got := inspVerdict(f); got != "×" {
		t.Errorf("inspector verdict = %q, want the cross", got)
	}
	prose := env.Content.Modules["mod-artillery-l"].Description
	if prose == "" {
		t.Fatal("fixture: the artillery module carries no description")
	}
	if got := f.head.headProse.Label; got != prose {
		t.Errorf("inspector description = %q, want the content's own %q", got, prose)
	}
	if want := float64(f.inspInnerW()); f.head.headProse.MaxWidth != want {
		t.Errorf("description wrap = %v, want the pane's inner %v", f.head.headProse.MaxWidth, want)
	}
	if got, pane := f.head.headProse.GetWidget().Rect.Dx(), f.matsScroll.ViewRect().Dx(); got > pane+theme.Px(theme.ScrollGutter) {
		t.Errorf("description %d px wide overflows the %d px pane", got, pane)
	}
	// The head leads: it sits above the profit table's first row.
	head := f.head.headBox.GetWidget().Rect
	profit := f.profit.RowWidgetForTest(0).GetWidget().Rect
	if head.Max.Y > profit.Min.Y {
		t.Errorf("head %v overlaps the profit table %v (want the head first)", head, profit)
	}
}
