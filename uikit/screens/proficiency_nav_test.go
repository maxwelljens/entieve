package screens

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// profNavFixture mounts the tree over a kit whose pointer the test scripts:
// pan, zoom and the node click all read the one frame.
func profNavFixture(t *testing.T) (*Kit, *Proficiency, *scriptPointer, func()) {
	t.Helper()
	uitest.UseScale(t, 1)
	in := &scriptPointer{}
	k, _ := newTestKitWith(t, widgets.NewPointer(in))
	k.SetCoreVisible(false)
	p := NewProficiency(k.Env(), proficiencyChain, nil, k.ProfFamily)
	p.Sync()
	k.AddBackdropWindow(p.Win())
	// frame is one whole frame: the host's pointer sample, the panels'
	// Sync, the UI pump and the draw (the draw is what sets the window
	// input layers the next frame's hit test reads).
	frame := func() {
		k.Pointer().Begin()
		k.Sync()
		p.Sync()
		if err := k.Update(); err != nil {
			t.Fatalf("Update: %v", err)
		}
		k.Draw(ebiten.NewImage(1280, 720))
	}
	frame()
	return k, p, in, frame
}

// TestProficiencyDragPansAndWheelZooms pins the navigation: a press on a
// node that travels past the slop pans the graph (it must not open the
// card), a release under the slop is a click, the wheel zooms inside its
// band, and the camera is session state.
func TestProficiencyDragPansAndWheelZooms(t *testing.T) {
	k, p, in, frame := profNavFixture(t)

	// A press on a node, held and dragged: pan, no card.
	node := mid(p.nodes[0].Rect)
	in.x, in.y = node.X, node.Y
	in.edge[widgets.Left] = true
	in.down[widgets.Left] = true
	frame()
	in.edge[widgets.Left] = false
	for _, d := range []image.Point{{30, 0}, {60, 20}} {
		in.x, in.y = node.X+d.X, node.Y+d.Y
		frame()
	}
	zoom, pan := p.View()
	if zoom != 1 {
		t.Fatalf("a drag changed the zoom to %v", zoom)
	}
	if pan != image.Pt(60, 20) {
		t.Fatalf("pan after a 60,20 drag = %v, want 60,20", pan)
	}
	if k.ProfFamilyOpen() {
		t.Fatal("a drag from a node opened the family card: the slop did not hold")
	}
	in.down[widgets.Left] = false
	frame()
	if _, got := p.View(); got != image.Pt(60, 20) {
		t.Fatalf("release moved the pan to %v", got)
	}

	// The wheel zooms in, around the cursor, and clamps at the band's ends.
	in.x, in.y = 640, 360
	in.wheel = 1
	for range 20 {
		frame()
	}
	if z, _ := p.View(); z != profZoomMax {
		t.Fatalf("wheel up zoomed to %v, want the %v band top", z, profZoomMax)
	}
	in.wheel = -1
	for range 40 {
		frame()
	}
	in.wheel = 0
	if z, _ := p.View(); z != profZoomMin {
		t.Fatalf("wheel down zoomed to %v, want the %v band bottom", z, profZoomMin)
	}

	// Session state: leaving the tree resets the camera.
	p.Unmounted()
	frame()
	if z, pan := p.View(); z != 1 || pan != (image.Point{}) {
		t.Fatalf("camera after unmount = %v/%v, want the fitted default", z, pan)
	}
}

// TestProficiencyClickOpensAndRetargetsOneCard: a click under the slop on
// a node opens the family card; clicking another node swaps the one
// window's page; CloseProfFamily (the Esc seam) closes it.
func TestProficiencyClickOpensAndRetargetsOneCard(t *testing.T) {
	k, p, in, frame := profNavFixture(t)
	click := func(pt image.Point) {
		in.x, in.y = pt.X, pt.Y
		in.edge[widgets.Left] = true
		in.down[widgets.Left] = true
		frame()
		in.edge[widgets.Left] = false
		in.down[widgets.Left] = false
		frame()
	}

	click(mid(p.nodes[0].Rect))
	if !k.ProfFamilyOpen() {
		t.Fatal("a click on a node did not open the family card")
	}
	if got := k.ProfFamily(); got != p.nodes[0].Family {
		t.Fatalf("card names %q, want %q", got, p.nodes[0].Family)
	}
	win := k.profCardWin.Win()

	// The card opens beside the click, so the far node stays uncovered:
	// clicking it re-targets the same window.
	click(mid(p.nodes[2].Rect))
	if got := k.ProfFamily(); got != p.nodes[2].Family {
		t.Fatalf("a second click names %q, want %q", got, p.nodes[2].Family)
	}
	if got := k.profCardWin.Win(); got != win {
		t.Fatal("a second click mounted a second card instead of re-targeting the one")
	}

	k.CloseProfFamily()
	if k.ProfFamilyOpen() {
		t.Fatal("CloseProfFamily left the card open")
	}
}

// TestProficiencyWindowAboveKeepsItsPressAndWheel: the layer answers only
// where it is the top surface. A press over a window above it neither pans
// nor opens a card, and its wheel does not zoom the graph.
func TestProficiencyWindowAboveKeepsItsPressAndWheel(t *testing.T) {
	k, p, in, frame := profNavFixture(t)
	fin := NewFinances(k.Env())
	k.AddWindow(fin.Win())
	fin.Win().SetLocation(image.Rect(600, 120, 1000, 460))
	frame()
	frame()

	pt := mid(fin.Win().GetContainer().GetWidget().Rect)
	in.x, in.y = pt.X, pt.Y
	in.edge[widgets.Left] = true
	in.down[widgets.Left] = true
	frame()
	in.edge[widgets.Left] = false
	in.down[widgets.Left] = false
	frame()

	if k.ProfFamilyOpen() {
		t.Fatal("a press on a window above the tree opened a family card")
	}
	if _, pan := p.View(); pan != (image.Point{}) {
		t.Fatalf("a press on a window above panned the tree to %v", pan)
	}
	in.wheel = 3
	frame()
	in.wheel = 0
	if z, _ := p.View(); z != 1 {
		t.Fatalf("wheel over a window above zoomed the tree to %v", z)
	}
}

// TestProficiencyTotals pins the overview's arithmetic: open families,
// levels earned against the possible total and the families at the cap.
func TestProficiencyTotals(t *testing.T) {
	got := proficiencyTotals(proficiencyChain())
	want := profTotals{Families: 3, Open: 2, Levels: 4, MaxLevels: 15, Maxed: 0}
	if got != want {
		t.Fatalf("totals = %+v, want %+v", got, want)
	}
	if got, want := proficiencyTotals(nil), (profTotals{}); got != want {
		t.Errorf("empty model totals = %+v, want zero", got)
	}
}

// realProfModel is the shipped fourteen-family graph as the tree sees it:
// every content family, its authored prerequisites, nobody trained.
func realProfModel(t *testing.T) []ProfFamily {
	t.Helper()
	ids := content.ProficiencyFamilyIDs()
	out := make([]ProfFamily, 0, len(ids))
	for _, id := range ids {
		var pre []ProfPrereq
		for _, p := range content.Prerequisites(id) {
			pre = append(pre, ProfPrereq{Family: p.Family, Level: p.Level})
		}
		out = append(out, ProfFamily{
			Family: id, Name: content.FamilyLabel(id),
			MaxLevel: content.ProficiencyMax, Prereqs: pre, Meets: true,
		})
	}
	return out
}

// TestProficiencyLayoutOneColumnPerRank: the shipped graph has ten root
// families and three ranks, so it lands as three columns with every root in
// the leftmost one, and the ONE fit factor shrinks the graph to the canvas
// (a rank never wraps into a second column). No two nodes overlap and every
// node stays on the canvas.
func TestProficiencyLayoutOneColumnPerRank(t *testing.T) {
	uitest.UseScale(t, 1)
	canvas := image.Rect(0, 0, 1280, 720)
	nodes, _ := proficiencyLayout(realProfModel(t), canvas)
	if len(nodes) != len(content.ProficiencyFamilyIDs()) {
		t.Fatalf("layout has %d nodes, want every family", len(nodes))
	}
	colOf := map[int]int{}
	box := image.Point{}
	for _, n := range nodes {
		if box == (image.Point{}) {
			box = n.Rect.Size()
		}
		if n.Rect.Size() != box {
			t.Fatalf("%s is %v, want every node at the one fit's %v", n.Family, n.Rect.Size(), box)
		}
		if !n.Rect.In(canvas) {
			t.Errorf("%s node %v is off the canvas %v", n.Family, n.Rect, canvas)
		}
		// One column per rank: every node of a rank shares its left edge,
		// and a deeper rank starts right of the previous one.
		if x, ok := colOf[n.Rank]; ok && x != n.Rect.Min.X {
			t.Errorf("%s starts at x %d, its rank's column starts at %d", n.Family, n.Rect.Min.X, x)
		}
		colOf[n.Rank] = n.Rect.Min.X
	}
	for r := 1; r < len(colOf); r++ {
		if colOf[r-1] >= colOf[r] {
			t.Errorf("rank %d starts at x %d, not right of rank %d's %d", r, colOf[r], r-1, colOf[r-1])
		}
	}
	for i := range nodes {
		for j := i + 1; j < len(nodes); j++ {
			if nodes[i].Rect.Overlaps(nodes[j].Rect) {
				t.Errorf("%s %v overlaps %s %v", nodes[i].Family, nodes[i].Rect, nodes[j].Family, nodes[j].Rect)
			}
		}
	}
}

// TestProficiencyCardPages pins the family card's page: the title, the
// description and every practice line come from the content's card table; a
// level meets the prerequisite edge naming. The level itself is the pips
// TestProficiencyPipsMatchTheTree compares against the tree.
func TestProficiencyCardPages(t *testing.T) {
	env, _ := newChromeEnv(t)
	c := NewProfInfoWin(env)
	c.Show("mining", 0, "")
	if c.title.Label != "Mining" {
		t.Errorf("title = %q, want Mining", c.title.Label)
	}
	if got := c.Gate(); got != "" {
		t.Errorf("open family gate line = %q, want none", got)
	}
	card := content.FamilyInfo("mining")
	if c.desc.Label != card.Desc || c.desc.Label == "" {
		t.Errorf("description = %q, want the content card", c.desc.Label)
	}
	for _, line := range card.Practice {
		if !strings.Contains(c.practice.Label, "· "+line) {
			t.Errorf("practice page misses %q:\n%s", line, c.practice.Label)
		}
	}
	if !c.IsOpen() {
		t.Error("Show left the card hidden")
	}
	c.Close()
	if c.IsOpen() {
		t.Error("Close left the card open")
	}
	// The machine line says plainly that nothing trains it: the owner's
	// catalogue copy marks that with "N/A" (registry voice), the older
	// wording with "None"; both are accepted, an act list is not.
	c.Show("rat", 0, "")
	if !strings.Contains(c.practice.Label, "None") && !strings.Contains(c.practice.Label, "N/A") {
		t.Errorf("rat page = %q, want the no-practice line", c.practice.Label)
	}
}

// TestProficiencyCardGateLine: the card's gate line is the sim's own
// sentence, printed under the level pips; an open family has none.
func TestProficiencyCardGateLine(t *testing.T) {
	env, _ := newChromeEnv(t)
	c := NewProfInfoWin(env)
	c.Show("artillery", 1, "Requires Autocannon II")
	if got, want := c.Gate(), "Requires Autocannon II"; got != want {
		t.Fatalf("gate line = %q, want %q", got, want)
	}
	c.Show("artillery", 2, "")
	if got := c.Gate(); got != "" {
		t.Errorf("open family gate line = %q, want none", got)
	}
}

// TestProficiencyCardFitsEveryPage: each shipped family card lays out inside
// its window. The hull cards carry four practice lines (the tallest page), so
// this is the content-floor pin: a page that outgrew the dialog would clip
// its own practice list.
func TestProficiencyCardFitsEveryPage(t *testing.T) {
	uitest.UseScale(t, 1)
	env, _ := newChromeEnv(t)
	c := NewProfInfoWin(env)
	outer := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout()))
	outer.AddChild(c.Win().GetContainer())
	cui := newChromeUI(env)
	cui.root.AddChild(outer)
	for _, id := range content.ProficiencyFamilyIDs() {
		c.Show(id, 2, "")
		cui.Frame(t, 1280, 720)
		win := c.Win().GetContainer().GetWidget().Rect
		for name, w := range map[string]*widget.Text{"desc": c.desc, "practice": c.practice} {
			r := w.GetWidget().Rect
			if r.Empty() {
				t.Errorf("%s: %s has no rect", id, name)
				continue
			}
			if !r.In(win) {
				t.Errorf("%s: %s %v escapes the card %v", id, name, r, win)
			}
		}
	}
}

// TestProficiencyHoverRepaintsOncePerMovedNode: the hover is part of the
// painted state, so moving onto a node repaints once, a still pointer is
// free, and leaving the node repaints again. The pointer frame is the only
// input (the layout and the model never move here).
func TestProficiencyHoverRepaintsOncePerMovedNode(t *testing.T) {
	_, p, in, frame := profNavFixture(t)
	node := mid(p.nodes[0].Rect)

	in.x, in.y = node.X, node.Y
	frame()
	first := p.redraw
	if first == 0 {
		t.Fatal("fixture: the layer never painted")
	}
	frame()
	if p.redraw != first {
		t.Errorf("a still hover repainted again (%d → %d)", first, p.redraw)
	}
	in.x, in.y = 2, 2 // bare layer space
	frame()
	if p.redraw != first+1 {
		t.Errorf("leaving the node painted %d times, want one", p.redraw-first)
	}
}

// TestProficiencyPipStateHasOneOwner: the tree's node, the family card and
// Show Info's level row fill a pip through one rule and hand it the same
// three values for the same family, so a level cannot read two ways. The
// rule's table is pinned here; the shared painter is drawProfPips.
func TestProficiencyPipStateHasOneOwner(t *testing.T) {
	uitest.UseScale(t, 1)
	env, _ := newChromeEnv(t)
	pal := env.Pal()
	for _, tc := range []struct {
		name     string
		k, level int
		locked   bool
		want     color.Color
	}{
		{"empty pip", 2, 2, false, theme.NRGBA(pal.Border, 200)},
		{"earned pip", 1, 2, false, theme.Solid(pal.Accent)},
		{"earned pip on a shut branch", 1, 2, true, theme.NRGBA(pal.Accent, 120)},
		{"empty pip, shut branch", 4, 2, true, theme.NRGBA(pal.Border, 200)},
	} {
		if got := profPipColor(tc.k, tc.level, tc.locked, pal); got != tc.want {
			t.Errorf("%s: pip %d = %v, want %v", tc.name, tc.k, got, tc.want)
		}
	}

	// The two surfaces that show a whole family: the node and the card.
	model := []ProfFamily{
		{Family: "mining", Name: "Mining", Level: 2, MaxLevel: content.ProficiencyMax, XP: 1, XPNext: 4, Meets: true},
		{Family: "artillery", Name: "Artillery", Level: 2, MaxLevel: content.ProficiencyMax, Meets: false, Gate: "Requires Autocannon II"},
	}
	k, _ := newTestKit(t)
	p := NewProficiency(k.Env(), func() []ProfFamily { return model }, nil, nil)
	p.Sync()
	c := NewProfInfoWin(k.Env())
	for _, n := range p.nodes {
		c.Show(n.Family, n.Level, n.GateText)
		level, max, locked := c.Pips()
		if level != n.Level || max != n.MaxLevel || locked != n.Locked {
			t.Errorf("%s: card pips %d/%d locked=%v, node %d/%d locked=%v",
				n.Family, level, max, locked, n.Level, n.MaxLevel, n.Locked)
		}
	}
}

// TestProficiencyOpenCardHighlightsItsNode: the family whose card is open
// is the tree's selected node (the painter argument drawNode's highlight
// reads), the selection follows a re-target and clears when the card
// closes, each move repainting the layer.
func TestProficiencyOpenCardHighlightsItsNode(t *testing.T) {
	k, p, in, frame := profNavFixture(t)
	in.x, in.y = 2, 2 // the pointer never sits on a node here
	famAt := func(fam string) int {
		for i, n := range p.nodes {
			if n.Family == fam {
				return i
			}
		}
		t.Fatalf("no %q node", fam)
		return -1
	}
	famA, famB := famAt("fam-a"), famAt("fam-b")
	if p.sel != -1 {
		t.Fatalf("a node is selected with no card open (%d)", p.sel)
	}

	k.OpenProfFamily(widgets.ShowProfFamily{Family: "fam-a", At: image.Pt(600, 200), Level: 2})
	frame()
	if p.sel != famA {
		t.Fatalf("selected node %d, want fam-a (%d)", p.sel, famA)
	}
	painted := p.redraw

	k.OpenProfFamily(widgets.ShowProfFamily{Family: "fam-b", At: image.Pt(600, 200), Level: 2})
	frame()
	if p.sel != famB {
		t.Fatalf("after the re-target the selected node is %d, want fam-b (%d)", p.sel, famB)
	}
	if p.redraw == painted {
		t.Error("the re-target moved the selection without repainting")
	}

	k.CloseProfFamily()
	frame()
	if p.sel != -1 {
		t.Fatalf("after the close the selected node is %d, want none", p.sel)
	}
	if p.redraw == painted+1 {
		t.Error("closing the card cleared the selection without repainting")
	}
}

// TestProficiencyCardHugsOrScrolls: the card never clips its page. A shipped
// page fits the frame (the frame hugs it under the cap, the rail stays
// hidden); a page taller than profCardMaxH stops growing at the cap and
// scrolls, the rail's handle present in the reserved gutter so the rest is
// reachable.
func TestProficiencyCardHugsOrScrolls(t *testing.T) {
	uitest.UseScale(t, 1)
	env, _ := newChromeEnv(t)
	c := NewProfInfoWin(env)
	cui := newChromeUI(env)
	cui.AddWindow(c)

	c.Show("frigate", 2, "")
	cui.Frame(t, 1280, 720) // the first frame lays the viewport out
	cui.Frame(t, 1280, 720) // the rail reads it on the next Sync
	win := c.Win().GetContainer().GetWidget().Rect
	if win.Dy() >= theme.Px(profCardMaxH) {
		t.Errorf("shipped page card = %v, want it under the %d cap", win, theme.Px(profCardMaxH))
	}
	if over := c.rail.Overflow(); over > 0 {
		t.Errorf("shipped page overflows by %v px, want the frame to hug it", over)
	}
	if c.rail.Bar().GetWidget().IsVisible() {
		t.Error("the rail shows while the page fits")
	}

	// A page far past the cap: the frame stops at profCardMaxH and the page
	// scrolls (the rail takes over from the frame).
	c.Show("mining", 2, "")
	c.desc.Label = strings.Repeat("The mining laser cuts the rock and pays by rarity. ", 12)
	c.practice.Label = strings.Repeat("· Each completed mining cycle.\n", 24)
	c.fit()
	cui.Frame(t, 1280, 720)
	cui.Frame(t, 1280, 720)
	win = c.Win().GetContainer().GetWidget().Rect
	if want := theme.Px(profCardMaxH); win.Dy() != want {
		t.Errorf("long page card height = %d, want the cap %d", win.Dy(), want)
	}
	if over := c.rail.Overflow(); over <= 0 {
		t.Errorf("long page overflow = %v, want a scroll range over the %d cap", over, theme.Px(profCardMaxH))
	}
	if !c.rail.Bar().GetWidget().IsVisible() {
		t.Error("the rail is hidden while the page overflows: the rest is unreachable")
	}
	if r := c.rail.Bar().GetWidget().Rect; r.Dx() != theme.Px(theme.ScrollGutter) || r.Empty() {
		t.Errorf("rail handle %v, want it in the %d px gutter column", r, theme.Px(theme.ScrollGutter))
	}
}
