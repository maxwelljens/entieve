package screens

import (
	"image"
	"os"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// The stack picker's pins: a cluster opens the icon column after the dwell,
// a single entity never does, an icon click selects, a ctrl+click locks, a
// right-click opens the entity's menu, the tooltip names the icon, the cap
// hides the tail, and leaving both the column and the cluster drops it after
// the grace.

// stackInfos builds n synthetic cluster entries (distinct handles, two kinds
// alternating so the depth order is visible).
func stackInfos(n int) []sim.Info {
	out := make([]sim.Info, n)
	for i := range out {
		kind := sim.KindAsteroid
		if i%2 == 0 {
			kind = sim.KindShip
		}
		out[i] = sim.Info{
			Entity:   ecs.Entity(1000 + i),
			Name:     "Stack " + string(rune('A'+i)),
			Kind:     kind,
			Distance: float64(1000 * (i + 1)),
			Visible:  true,
		}
	}
	return out
}

// visibleCells is the number of icon cells the open column shows.
func visibleCells(k *Kit) int {
	n := 0
	for i := range k.column.cells {
		if k.column.cells[i].g.GetWidget().IsVisible() {
			n++
		}
	}
	return n
}

// openStack hovers entries until the dwell has passed, pumping the kit
// between frames (the column refreshes through Sync).
func openStack(t *testing.T, k *Kit, entries []sim.Info) {
	t.Helper()
	openStackAt(t, k, entries, 400, 300)
}

// openStackAt is openStack at an explicit hover point.
func openStackAt(t *testing.T, k *Kit, entries []sim.Info, x, y float64) {
	t.Helper()
	for i := 0; i < stackDwellFrames; i++ {
		k.StackHover(entries, x, y)
		k.Sync()
	}
	if !k.StackShown() {
		t.Fatalf("the column did not open after %d hover frames", stackDwellFrames)
	}
}

// clickStackCell fires one cell's press and dispatches it through the ui.
func clickStackCell(t *testing.T, k *Kit, i int, button ebiten.MouseButton) {
	t.Helper()
	w := k.column.cells[i].g.GetWidget()
	w.MouseButtonClickedEvent.Fire(&widget.WidgetMouseButtonClickedEventArgs{
		Widget: w, Button: button, OffsetX: 4, OffsetY: 4,
	})
	k.ui.Update()
}

// TestStackColumnDwellThenGrace: the column opens only after the dwell
// (~200 ms of frames, never wall clock) and drops only after the grace
// (~150 ms) once the cursor rests on neither it nor the cluster.
func TestStackColumnDwellThenGrace(t *testing.T) {
	k, _ := newTestKit(t)
	infos := stackInfos(3)
	for i := 1; i < stackDwellFrames; i++ {
		k.StackHover(infos, 400, 300)
		k.Sync()
		if k.StackShown() {
			t.Fatalf("column opened after %d hover frames, want %d", i, stackDwellFrames)
		}
	}
	openStack(t, k, infos)
	if got := visibleCells(k); got != 3 {
		t.Fatalf("open column shows %d icons, want 3", got)
	}
	// Off both: the grace holds it, then it drops.
	for i := 1; i < stackGraceFrames; i++ {
		k.StackLeave()
		k.Sync()
		if !k.StackShown() {
			t.Fatalf("column dropped after %d grace frames, want %d", i, stackGraceFrames)
		}
	}
	k.StackLeave()
	k.Sync()
	if k.StackShown() {
		t.Fatalf("column survived %d grace frames", stackGraceFrames)
	}
	// The cursor on the column keeps it up past the grace.
	openStack(t, k, infos)
	for i := 0; i < stackGraceFrames*2; i++ {
		k.StackKeep()
		k.Sync()
	}
	if !k.StackShown() {
		t.Fatal("column dropped while the cursor rested on it")
	}
	// Leaving the world scene drops it at once.
	k.StackDismiss()
	k.Sync()
	if k.StackShown() {
		t.Fatal("StackDismiss left the column up")
	}

	// The open column claims its band (top chrome, above the windows); a
	// dismissed one claims nothing, even after the root lays out again.
	// The hover point is found free of the kit's own windows, so the band's
	// ownership is the column's answer and nothing else's.
	free := image.Point{}
	for x := 1200; x >= 900 && free == (image.Point{}); x -= 10 {
		for y := 150; y <= 650; y += 10 {
			if !k.Over(image.Pt(x, y)) {
				free = image.Pt(x, y)
				break
			}
		}
	}
	if free == (image.Point{}) {
		t.Skip("the test kit mounts windows over every probe point")
	}
	openStackAt(t, k, infos, float64(free.X), float64(free.Y))
	pt := image.Point{}
	for x := free.X; x <= free.X+80 && pt == (image.Point{}); x++ {
		for y := free.Y - 80; y <= free.Y+80; y++ {
			if k.StackOwns(float64(x), float64(y)) {
				pt = image.Pt(x, y)
				break
			}
		}
	}
	if pt == (image.Point{}) {
		t.Fatalf("the open column owns no band beside the cursor %v", free)
	}
	if !k.Over(pt) {
		t.Fatalf("the open column's band %v is not interface", pt)
	}
	k.StackDismiss()
	k.root.RequestRelayout()
	pumpKit(t, k, 1280, 720)
	if k.Over(pt) {
		t.Fatalf("the dismissed column still claims %v", pt)
	}
}

// TestStackSingleEntityNeverOpens: one entity under the cursor behaves as it
// always did — no column, however long the hover lasts.
func TestStackSingleEntityNeverOpens(t *testing.T) {
	k, _ := newTestKit(t)
	one := stackInfos(1)
	for i := 0; i < stackDwellFrames*3; i++ {
		k.StackHover(one, 400, 300)
		k.Sync()
	}
	if k.StackShown() {
		t.Fatal("a single entity opened the column")
	}
}

// TestStackColumnCapHidesTheTail: the column lists at most stackCap icons in
// the pick's order; the tail is unaddressable by design (the Overview lists
// every entity).
func TestStackColumnCapHidesTheTail(t *testing.T) {
	k, _ := newTestKit(t)
	infos := stackInfos(stackCap + 3)
	openStack(t, k, infos)
	if got := visibleCells(k); got != stackCap {
		t.Fatalf("column shows %d icons, want the cap %d", got, stackCap)
	}
	for i := 0; i < stackCap; i++ {
		if got, want := k.column.cells[i].tip.View.Model().Title, infos[i].Name; got != want {
			t.Errorf("icon %d tooltip = %q, want %q (the pick's order)", i, got, want)
		}
	}
}

// TestStackColumnTooltipNamesAndDistances: each icon's tooltip carries the
// entity and its distance, so a column of identical kinds stays tellable.
func TestStackColumnTooltipNamesAndDistances(t *testing.T) {
	k, _ := newTestKit(t)
	infos := stackInfos(2)
	openStack(t, k, infos)
	for i, inf := range infos {
		m := k.column.cells[i].tip.View.Model()
		if m.Title != inf.Name {
			t.Errorf("icon %d tooltip title = %q, want %q", i, m.Title, inf.Name)
		}
		if want := theme.FormatDistance(inf.Distance); !strings.Contains(m.Sub, want) {
			t.Errorf("icon %d tooltip sub = %q, want the distance %q", i, m.Sub, want)
		}
	}
}

// TestStackColumnClickSelectsAndCtrlClickLocks: an icon is the overview row
// for one entity — a click selects it (Kit.Select), a ctrl+click also locks
// it through the same command seam the world's ctrl+click uses.
func TestStackColumnClickSelectsAndCtrlClickLocks(t *testing.T) {
	k, cmds := newTestKit(t)
	k.Sync()
	infos := make([]sim.Info, 0, 3)
	for _, inf := range k.Sim.Grid() {
		if inf.Visible && !inf.Entity.IsZero() {
			infos = append(infos, inf)
			if len(infos) == 3 {
				break
			}
		}
	}
	if len(infos) < 3 {
		t.Skipf("showcase grid has %d visible entities, want 3", len(infos))
	}
	openStack(t, k, infos)

	clickStackCell(t, k, 1, ebiten.MouseButtonLeft)
	if k.Selected() != infos[1].Entity {
		t.Fatalf("Selected() = %v after an icon click, want %v", k.Selected(), infos[1].Entity)
	}
	if len(*cmds) != 0 {
		t.Fatalf("a plain icon click enqueued %d commands, want none", len(*cmds))
	}

	k.SetCtrl(true)
	clickStackCell(t, k, 2, ebiten.MouseButtonLeft)
	if k.Selected() != infos[2].Entity {
		t.Errorf("Selected() = %v after ctrl+click, want %v", k.Selected(), infos[2].Entity)
	}
	if len(*cmds) != 1 {
		t.Fatalf("ctrl+click enqueued %d commands, want 1", len(*cmds))
	}
	if c, ok := (*cmds)[0].(sim.CmdLock); !ok || c.Target != infos[2].Entity {
		t.Errorf("cmd = %#v, want CmdLock{%v}", (*cmds)[0], infos[2].Entity)
	}
}

// TestStackColumnRightClickOpensEntityMenu: the icon's menu is the row menu —
// Show Info first, then the entity's verbs, via the one RowSpec installer.
func TestStackColumnRightClickOpensEntityMenu(t *testing.T) {
	k, _ := newTestKit(t)
	k.Sync()
	var infos []sim.Info
	for _, inf := range k.Sim.Grid() {
		if inf.Visible && !inf.Entity.IsZero() {
			infos = append(infos, inf)
			if len(infos) == 2 {
				break
			}
		}
	}
	if len(infos) < 2 {
		t.Skipf("showcase grid has %d visible entities, want 2", len(infos))
	}
	openStack(t, k, infos)

	clickStackCell(t, k, 0, ebiten.MouseButtonRight)
	m := k.svc.menu
	if !m.IsOpen() {
		t.Fatal("right-click on a stack icon opened no menu")
	}
	entries := m.Entries()
	if len(entries) == 0 || entries[0].Label != "Show Info" || !entries[0].Bold {
		t.Fatalf("menu entries = %+v, want a bold Show Info first", entries)
	}
}

// TestStackCellsDrawTheWorldNotIcons: a cell is the entity as the world draws
// it (render.DrawEntityBracket: its body art inside its own bracket), never a
// kind glyph from the UI icon set, and each cell paints into an image of its
// own (the shared icon atlas stays untouched).
func TestStackCellsDrawTheWorldNotIcons(t *testing.T) {
	src, err := os.ReadFile("stack.go")
	if err != nil {
		t.Skipf("source not available: %v", err)
	}
	if strings.Contains(string(src), "NewIconBox(") {
		t.Error("a cell is an icon box: cells must draw the entity, not a UI glyph")
	}
	if !strings.Contains(string(src), "DrawEntityBracket(") {
		t.Error("no cell draws through render.DrawEntityBracket (the world's own art)")
	}
	k, _ := newTestKit(t)
	openStack(t, k, stackInfos(2))
	for i := 0; i < 2; i++ {
		c := &k.column.cells[i]
		if c.img == nil || c.g.Image != c.img {
			t.Errorf("cell %d does not paint into its own image", i)
		}
		if b := c.img.Bounds(); b.Dx() != theme.Px(stackCellPx) || b.Dy() != theme.Px(stackCellPx) {
			t.Errorf("cell %d image = %v, want a %d px square", i, b, theme.Px(stackCellPx))
		}
	}
}
