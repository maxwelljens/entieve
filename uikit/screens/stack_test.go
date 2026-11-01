package screens

import (
	"image"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// The stack picker's pins: a cluster opens the entry column after the dwell,
// a single entity never does, an entry's art is the entity at the grid's own
// zoom with its name beside it (no frame, no panel, no tooltip), an entry
// click selects, a ctrl+click locks, a right-click opens the entity's menu,
// the cap hides the tail from the column but not from the world, and leaving
// both the column and the cluster drops it after the grace.

// stackView is the world framing the picker tests report: the 1280×720 design
// screen at zoom 1 (game.zoomBase px per meter).
var stackView = render.Camera{Zoom: 0.02, ScreenW: 1280, ScreenH: 720}

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

// visibleCells is the number of entries the open column shows.
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
		k.StackView(stackView)
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
		t.Fatalf("open column shows %d entries, want 3", got)
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

// TestStackColumnCapHidesTheTail: the column lists at most stackCap entries in
// the pick's order and hides exactly those from the world; the tail past the
// cap was never listed, so it stays in the grid (and in the Overview).
func TestStackColumnCapHidesTheTail(t *testing.T) {
	k, _ := newTestKit(t)
	infos := stackInfos(stackCap + 3)
	openStack(t, k, infos)
	if got := visibleCells(k); got != stackCap {
		t.Fatalf("column shows %d entries, want the cap %d", got, stackCap)
	}
	hidden := k.StackHidden()
	if len(hidden) != stackCap {
		t.Fatalf("the grid hides %d entities, want the %d listed", len(hidden), stackCap)
	}
	for i, e := range hidden {
		if e != infos[i].Entity {
			t.Errorf("hidden[%d] = %v, want the pick's order %v", i, e, infos[i].Entity)
		}
	}
	for i := stackCap; i < len(infos); i++ {
		for _, e := range hidden {
			if e == infos[i].Entity {
				t.Errorf("the tail entry %d (%v) was hidden: it was never listed", i, e)
			}
		}
	}
	k.StackDismiss()
	if got := k.StackHidden(); len(got) != 0 {
		t.Errorf("a dismissed column hides %d entities, want none", len(got))
	}
}

// TestStackEntryIsArtAndName: an entry is the entity's art at the grid's own
// size with its name beside it — the image is wider than the art box, the box
// is as tall as the art (no 32 px cell), and entries of different kinds get
// their own heights.
func TestStackEntryIsArtAndName(t *testing.T) {
	k, _ := newTestKit(t)
	infos := stackInfos(2) // a ship and an asteroid
	openStack(t, k, infos)
	gap := theme.Px(stackGap)
	for i, inf := range infos {
		c := &k.column.cells[i]
		if c.img == nil || c.g.Image != c.img {
			t.Fatalf("entry %d does not paint into its own image", i)
		}
		b := c.img.Bounds()
		if c.art <= 0 || c.art != c.w-gap-int(math.Ceil(render.Measure(inf.Name, render.LabelSize))) {
			t.Errorf("entry %d image %v, art box %d: the name does not sit right of the art", i, b, c.art)
		}
		if b.Dx() != c.w || b.Dy() != c.h {
			t.Errorf("entry %d image %v does not match its layout %dx%d", i, b, c.w, c.h)
		}
		if c.h < c.art {
			t.Errorf("entry %d is %d px tall for %d px of art: the art is cut", i, c.h, c.art)
		}
	}
	if k.column.cells[0].h == k.column.cells[1].h || k.column.cells[0].art == k.column.cells[1].art {
		t.Errorf("a ship and a rock got the same box (%d px and %d px of art): the entry takes the art's own size",
			k.column.cells[0].art, k.column.cells[1].art)
	}
}

// TestStackEntryFollowsTheWorldZoom: the entry is the art at the grid's zoom,
// so a zoom change resizes it on the next sync (no cached miniature at the
// old size).
func TestStackEntryFollowsTheWorldZoom(t *testing.T) {
	k, _ := newTestKit(t)
	k.Sync()
	var station sim.Info
	var other sim.Info
	for _, inf := range k.Sim.Grid() {
		if !inf.Visible || inf.Entity.IsZero() {
			continue
		}
		if inf.Kind == sim.KindStation {
			station = inf
		} else if other.Entity.IsZero() {
			other = inf
		}
	}
	if station.Entity.IsZero() || other.Entity.IsZero() {
		t.Skip("fixture: the showcase needs a station and one more entity")
	}
	openStack(t, k, []sim.Info{station, other})
	before := k.column.cells[0].art
	// One world zoom notch in (the wheel's own 1.15 is not needed: the station
	// is uncapped at both framings, so doubling the zoom doubles its art).
	k.StackView(render.Camera{Zoom: 2 * stackView.Zoom, ScreenW: 1280, ScreenH: 720})
	k.Sync()
	after := k.column.cells[0].art
	if after <= before {
		t.Errorf("the station's art stayed %d px at zoom %v (was %d at %v): the entry must follow the world zoom",
			after, 2*stackView.Zoom, before, stackView.Zoom)
	}
	if ratio := float64(after) / float64(before); math.Abs(ratio-2) > 0.1 {
		t.Errorf("art %d px → %d px at double the zoom (ratio %.2f), want the world's own scale",
			before, after, ratio)
	}
}

// TestStackEntriesAreBare: no frame around an entry and no panel behind the
// column (the owner's rule: the entries float over the world), and no tooltip
// (the name beside the art is the label). The pixels are pinned by the stack
// picker capture; this pins the source the column is built from.
func TestStackEntriesAreBare(t *testing.T) {
	src, err := os.ReadFile("stack.go")
	if err != nil {
		t.Skipf("source not available: %v", err)
	}
	code := string(src)
	for _, gone := range []string{"PanelBox(", "DrawEntityBracket(", "BracketColor(", "RowTip", "Tooltip"} {
		if strings.Contains(code, gone) {
			t.Errorf("stack.go still uses %s: an entry is the bare art with its name, nothing framed or explained", gone)
		}
	}
	if !strings.Contains(code, "DrawEntityBody(") {
		t.Error("no entry draws through render.DrawEntityBody (the grid's own art)")
	}
}

// TestStackColumnClickSelectsAndCtrlClickLocks: an entry is the overview row
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
		t.Fatalf("Selected() = %v after an entry click, want %v", k.Selected(), infos[1].Entity)
	}
	if len(*cmds) != 0 {
		t.Fatalf("a plain entry click enqueued %d commands, want none", len(*cmds))
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

// TestStackColumnRightClickOpensEntityMenu: the entry's menu is the row menu —
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
		t.Fatal("right-click on a stack entry opened no menu")
	}
	entries := m.Entries()
	if len(entries) == 0 || entries[0].Label != "Show Info" || !entries[0].Bold {
		t.Fatalf("menu entries = %+v, want a bold Show Info first", entries)
	}
}
