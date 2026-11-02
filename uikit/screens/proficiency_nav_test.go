package screens

import (
	"image"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/internal/uitest"
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
	p := NewProficiency(k.Env(), proficiencyChain, nil)
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

// TestProficiencyLayoutWrapsTheBigRank: the shipped graph has ten root
// families in rank 0 and only three ranks, so without wrapping it shrinks
// to about half size. The layout splits that rank into adjacent columns,
// keeps the graph above 0.8 (the old one-column rank fit about 0.56), and
// no two nodes overlap or leave the canvas.
func TestProficiencyLayoutWrapsTheBigRank(t *testing.T) {
	uitest.UseScale(t, 1)
	canvas := image.Rect(0, 0, 1280, 720)
	nodes, _ := proficiencyLayout(realProfModel(t), canvas)
	if len(nodes) != len(content.ProficiencyFamilyIDs()) {
		t.Fatalf("layout has %d nodes, want every family", len(nodes))
	}
	for _, n := range nodes {
		if n.Scale < 0.8 {
			t.Fatalf("%s scaled to %.2f: the wrap did not lift the fit above 0.8", n.Family, n.Scale)
		}
		if !n.Rect.In(canvas) {
			t.Errorf("%s node %v is off the canvas %v", n.Family, n.Rect, canvas)
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

// TestProficiencyCardPages pins the family card's page: the title, the live
// level line, the description and every practice line come from the
// content's card table; a level meets the prerequisite edge naming.
func TestProficiencyCardPages(t *testing.T) {
	env, _ := newChromeEnv(t)
	c := NewProfInfoWin(env)
	c.Show("mining", 0, "")
	if c.title.Label != "Mining" {
		t.Errorf("title = %q, want Mining", c.title.Label)
	}
	if got := c.status.Label; got != "Untrained (0 of V)" {
		t.Errorf("status at level 0 = %q", got)
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
	// The machine line says plainly that nothing trains it.
	c.Show("rat", 0, "")
	if !strings.Contains(c.practice.Label, "None") {
		t.Errorf("rat page = %q, want the no-practice line", c.practice.Label)
	}
}

// TestProficiencyCardStatusNamesUnmetPrereq: the card's first line carries
// the first unmet authored edge ("Level 0 of V · needs Autocannons II").
func TestProficiencyCardStatusNamesUnmetPrereq(t *testing.T) {
	got := profCardStatus(1, "needs Autocannon II")
	if want := "Level I of V · needs Autocannon II"; got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}
	if got := profCardStatus(3, ""); strings.Contains(got, "needs") {
		t.Errorf("open family status = %q, want no gate", got)
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
