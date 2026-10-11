package uikit

import (
	"image"
	"math"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// newTestKit builds a Kit over the embedded content and world-showcase,
// capturing every enqueued command.
func newTestKit(t *testing.T) (*Kit, *[]sim.Command) {
	t.Helper()
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	wd, ok := c.World("world-showcase")
	if !ok {
		t.Fatal("world-showcase missing")
	}
	s, err := sim.New(c, wd, 42)
	if err != nil {
		t.Fatalf("sim.New: %v", err)
	}
	var cmds []sim.Command
	k, err := NewKit(c, s, func(cmd sim.Command) { cmds = append(cmds, cmd) })
	if err != nil {
		t.Fatalf("NewKit: %v", err)
	}
	return k, &cmds
}

// pumpKit runs one frame (Sync, Update, Draw) at the given screen size
// and returns the rendered image: Update dispatches the deferred event
// queue, Draw performs the widget layout pass.
func pumpKit(t *testing.T, k *Kit, w, h int) *ebiten.Image {
	t.Helper()
	k.Sync()
	if err := k.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	img := ebiten.NewImage(w, h)
	k.Draw(img)
	return img
}

// TestSelectHighlightsFreshEntity: a world click on an entity that only
// just appeared in the grid — its overview row not built yet — still
// lands the highlight. Select syncs the rows BEFORE resolving the
// highlight; the other order updates the panel but clears the overview
// selection.
func TestSelectHighlightsFreshEntity(t *testing.T) {
	k, _ := newTestKit(t)
	k.Sync()

	var fresh ecs.Entity
	for _, info := range k.Sim.Grid() {
		if info.Visible && info.Entity != k.Sim.Player() {
			fresh = info.Entity
			break
		}
	}
	if fresh.IsZero() {
		t.Fatal("fixture: no visible entity besides the player")
	}
	// The rows map is rebuilt by the next Sync, so dropping the entry is
	// exactly the state of an entity that just became visible.
	delete(k.ov.rows, fresh)

	k.Select(fresh)
	if k.Selected() != fresh {
		t.Fatalf("Selected() = %v, want %v", k.Selected(), fresh)
	}
	row, ok := k.ov.tbl.Selected()
	if !ok {
		t.Fatal("fresh entity did not land an overview highlight")
	}
	got, isOvRow := row.Data.(*ovRow)
	if !isOvRow || got.info.Entity != fresh {
		t.Fatalf("highlighted row payload = %#v, want the ovRow of %v", row.Data, fresh)
	}
}

// TestNewKitConstruction asserts every theme entry is populated and the
// four type-scale faces exist at the expected sizes.
func TestNewKitConstruction(t *testing.T) {
	ts := DefaultTypeScale()
	th := NewTheme(DefaultPalette(), ts)
	entries := map[string]any{
		"DefaultFace":      th.DefaultFace,
		"ButtonTheme":      th.ButtonTheme,
		"PanelTheme":       th.PanelTheme,
		"LabelTheme":       th.LabelTheme,
		"TextTheme":        th.TextTheme,
		"CheckboxTheme":    th.CheckboxTheme,
		"ProgressBarTheme": th.ProgressBarTheme,
		"SliderTheme":      th.SliderTheme,
		"TabbookTheme":     th.TabbookTheme,
		"TabTheme":         th.TabTheme,
		"TextInputTheme":   th.TextInputTheme,
		"TextAreaTheme":    th.TextAreaTheme,
		"ListTheme":        th.ListTheme,
		"ListComboButton":  th.ListComboButtonTheme,
	}
	for name, v := range entries {
		if v == nil {
			t.Errorf("theme entry %s is nil", name)
		}
	}
	// Every entry carries a face where the widget type needs one.
	if th.ButtonTheme.TextFace == nil || th.LabelTheme.Face == nil ||
		th.TextTheme.Face == nil || th.TextInputTheme.Face == nil ||
		th.TextAreaTheme.Face == nil || th.ListTheme.EntryFace == nil ||
		th.ListComboButtonTheme.List.EntryFace == nil ||
		th.ListComboButtonTheme.Button.TextFace == nil ||
		th.TabbookTheme.TabButton.TextFace == nil ||
		th.CheckboxTheme.Label.Face == nil {
		t.Fatal("a theme entry is missing its face")
	}

	f := newFaces(ts)
	sizes := map[string]struct {
		face text.Face
		want float64
	}{
		"base":  {f.base, ts.Base},
		"small": {f.small, ts.Small},
		"large": {f.large, ts.Large},
		"tiny":  {f.tiny, ts.Tiny},
	}
	for name, s := range sizes {
		gt, ok := s.face.(*text.GoTextFace)
		if !ok {
			t.Fatalf("face %s: %T is not *text.GoTextFace", name, s.face)
		}
		if gt.Size != s.want {
			t.Errorf("face %s size = %v, want %v", name, gt.Size, s.want)
		}
	}

	// And the kit itself wires all of it up.
	k, _ := newTestKit(t)
	if k.theme == nil || k.ui == nil || k.ov == nil || k.sp == nil {
		t.Fatal("kit missing theme/ui/windows")
	}
}

// TestOverviewRowClickSelectsThenApproaches: the overview Table's first
// click selects, a repeat click inside the double-click window
// additionally enqueues CmdApproach with the right target.
func TestOverviewRowClickSelectsThenApproaches(t *testing.T) {
	k, cmds := newTestKit(t)
	row := &ovRow{info: sim.Info{Entity: ecs.Entity(777), Name: "Scuttler"}}
	k.ov.tbl.SetRows([]TableRow{
		{Cells: []string{"Scuttler", "10 m", "Ship", "0 m/s"}, Data: row},
	})

	k.ov.tbl.rowClicked(0)
	if k.Selected() != ecs.Entity(777) {
		t.Fatalf("Selected() = %v, want 777", k.Selected())
	}
	if len(*cmds) != 0 {
		t.Fatalf("first click enqueued %d commands, want 0", len(*cmds))
	}

	k.ov.tbl.rowClicked(0)
	if len(*cmds) != 1 {
		t.Fatalf("repeat click enqueued %d commands, want 1", len(*cmds))
	}
	cmd, ok := (*cmds)[0].(sim.CmdApproach)
	if !ok {
		t.Fatalf("command type = %T, want sim.CmdApproach", (*cmds)[0])
	}
	if cmd.Target != ecs.Entity(777) {
		t.Fatalf("CmdApproach.Target = %v, want 777", cmd.Target)
	}

	// A different row inside the window is just a selection.
	k.ov.tbl.SetRows([]TableRow{
		{Cells: []string{"Scuttler", "10 m", "Ship", "0 m/s"}, Data: row},
		{Cells: []string{"Rock", "12 m", "Asteroid", "0 m/s"}, Data: &ovRow{info: sim.Info{Entity: ecs.Entity(778)}}},
	})
	k.ov.tbl.rowClicked(1)
	if len(*cmds) != 1 {
		t.Fatalf("selection of another row enqueued %d commands, want 1", len(*cmds))
	}
	if k.Selected() != ecs.Entity(778) {
		t.Fatalf("Selected() = %v, want 778", k.Selected())
	}
}

// TestSelectedButtonsEnqueueCommands covers the Selected Item window's
// command seam, including the disabled/no-selection guards.
func TestSelectedButtonsEnqueueCommands(t *testing.T) {
	k, cmds := newTestKit(t)

	// No selection: the guarded handlers stay quiet.
	k.approachSelected()
	k.warpSelected()
	k.lockSelected()
	k.dockSelected()
	k.jumpSelected()
	k.lootSelected()
	k.mineSelected()
	if len(*cmds) != 0 {
		t.Fatalf("no-selection click enqueued %d commands, want 0", len(*cmds))
	}

	k.selected = ecs.Entity(42)
	k.approachSelected()
	k.warpSelected()
	k.stopSelected()
	k.lockSelected()
	k.dockSelected()
	k.jumpSelected()
	k.lootSelected()
	if len(*cmds) != 7 {
		t.Fatalf("enqueued %d commands, want 7", len(*cmds))
	}
	if c, ok := (*cmds)[0].(sim.CmdApproach); !ok || c.Target != ecs.Entity(42) {
		t.Errorf("cmd[0] = %#v, want CmdApproach{42}", (*cmds)[0])
	}
	// The Warp action carries the warp selector's value (Kit.WarpRange,
	// default WarpRanges[0] = 0) — not a range of its own.
	if c, ok := (*cmds)[1].(sim.CmdWarpTo); !ok || c.Target != ecs.Entity(42) || c.Range != k.WarpRange() {
		t.Errorf("cmd[1] = %#v, want CmdWarpTo{42, WarpRange %v}", (*cmds)[1], k.WarpRange())
	}
	if c, ok := (*cmds)[1].(sim.CmdWarpTo); !ok || c.Range != WarpRanges[0] {
		t.Errorf("default WarpRange = %v, want WarpRanges[0] %v", k.WarpRange(), WarpRanges[0])
	}
	if _, ok := (*cmds)[2].(sim.CmdStop); !ok {
		t.Errorf("cmd[2] = %T, want sim.CmdStop", (*cmds)[2])
	}
	if c, ok := (*cmds)[3].(sim.CmdLock); !ok || c.Target != ecs.Entity(42) {
		t.Errorf("cmd[3] = %#v, want CmdLock{42}", (*cmds)[3])
	}
	if c, ok := (*cmds)[4].(sim.CmdDock); !ok || c.Station != ecs.Entity(42) {
		t.Errorf("cmd[4] = %#v, want CmdDock{42}", (*cmds)[4])
	}
	if c, ok := (*cmds)[5].(sim.CmdJump); !ok || c.Gate != ecs.Entity(42) {
		t.Errorf("cmd[5] = %#v, want CmdJump{42}", (*cmds)[5])
	}
	if c, ok := (*cmds)[6].(sim.CmdLoot); !ok || c.Wreck != ecs.Entity(42) {
		t.Errorf("cmd[6] = %#v, want CmdLoot{42}", (*cmds)[6])
	}

	// Mine needs a real asteroid and a fitted mining laser.
	for _, info := range k.Sim.Grid() {
		if info.Kind == sim.KindAsteroid {
			k.selected = info.Entity
			break
		}
	}
	if k.selected != ecs.Entity(42) {
		k.mineSelected()
		if len(*cmds) != 8 {
			t.Fatalf("mine enqueued %d commands total, want 8", len(*cmds))
		}
		if c, ok := (*cmds)[7].(sim.CmdMine); !ok || c.Asteroid != k.selected {
			t.Errorf("cmd[7] = %#v, want CmdMine{%v}", (*cmds)[7], k.selected)
		}
	}
}

// TestSelectedContextGating: with no selection every action is disabled;
// with a live selection the context buttons follow the Info flags.
func TestSelectedContextGating(t *testing.T) {
	k, _ := newTestKit(t)
	k.selected = 0
	k.Sync()
	for name, b := range map[string]*widget.Button{
		"approach": k.sp.approach, "warp": k.sp.warp, "stop": k.sp.stop,
		"lock": k.sp.lock, "dock": k.sp.dock, "jump": k.sp.jump,
		"loot": k.sp.loot, "mine": k.sp.mine,
	} {
		if !b.GetWidget().Disabled {
			t.Errorf("%s enabled with no selection", name)
		}
	}

	// A warpable celestial: warp/lock flags come straight from Info.
	var celestial, ship *sim.Info
	for _, info := range k.Sim.Grid() {
		i := info
		if i.CanWarp && celestial == nil {
			celestial = &i
		}
		if i.Kind == sim.KindShip && i.Entity != k.Sim.Player() && ship == nil {
			ship = &i
		}
	}
	if celestial == nil {
		t.Fatal("no warpable celestial on the grid")
	}
	k.selected = celestial.Entity
	k.Sync()
	if k.sp.warp.GetWidget().Disabled {
		t.Error("warp disabled for a warpable celestial")
	}
	if got, want := k.sp.dock.GetWidget().Disabled, !celestial.CanDock; got != want {
		t.Errorf("dock Disabled = %v, want %v (Info.CanDock=%v)", got, want, celestial.CanDock)
	}
	if ship != nil {
		k.selected = ship.Entity
		k.Sync()
		// Inside sim.WarpMinRange the drive cannot engage either, so the
		// action reads disabled even with CanWarp set.
		want := !ship.CanWarp || ship.Distance < sim.WarpMinRange
		if got := k.sp.warp.GetWidget().Disabled; got != want {
			t.Errorf("warp Disabled = %v, want %v (CanWarp=%v distance=%v WarpMinRange=%v)",
				got, want, ship.CanWarp, ship.Distance, sim.WarpMinRange)
		}
	}
}

// TestOverviewMinSizeAndRefit covers the window min-size clamp and a
// resize reflowing the table's elastic columns.
func TestOverviewMinSizeAndRefit(t *testing.T) {
	k, _ := newTestKit(t)
	pumpKit(t, k, 1280, 720)

	win := k.ov.win
	win.SetLocation(image.Rect(0, 0, 100, 50))
	pumpKit(t, k, 1280, 720)
	r := win.GetContainer().GetWidget().Rect
	if r.Dx() != 320 || r.Dy() != 240 {
		t.Fatalf("min-size clamp rect = %v, want 320x240", r)
	}

	// Resize wide: the table refits its columns to the new width on the
	// next Sync (layout happens in Draw, refit in the following Sync).
	win.SetLocation(image.Rect(0, 0, 900, 600))
	pumpKit(t, k, 1280, 720) // lays the window out at the new size
	k.Sync()                 // detects the new table width
	k.Update()
	k.Draw(ebiten.NewImage(1280, 720))

	want := 900.0 - 2*contentsPad
	if got := k.ov.tbl.root.GetWidget().Rect.Dx(); float64(got) != want {
		t.Fatalf("table root width = %v, want %v (window width minus padding)", got, want)
	}
	sum := 0.0
	for _, w := range k.ov.tbl.widths {
		sum += w
	}
	if math.Abs(sum-want) > 1e-6 {
		t.Fatalf("fitted column sum = %v, want %v", sum, want)
	}
	for i, btn := range k.ov.tbl.hdrBtn {
		// layoutCells tiles buttons with cumulative rounding.
		want := int(math.Round(sumPrefix(k.ov.tbl.widths[:i+1]))) -
			int(math.Round(sumPrefix(k.ov.tbl.widths[:i])))
		if btn.GetWidget().MinWidth != want {
			t.Errorf("header button %d MinWidth = %d, want %d",
				i, btn.GetWidget().MinWidth, want)
		}
	}
}

// sumPrefix adds the first n fitted column widths.
func sumPrefix(cols []float64) float64 {
	s := 0.0
	for _, c := range cols {
		s += c
	}
	return s
}

// TestDrawSmoke draws the kit at both design sizes headless: no panics,
// both windows stay in bounds and the overview table holds fitted rows.
func TestDrawSmoke(t *testing.T) {
	sizes := [][2]int{{1280, 720}, {1024, 700}}
	for _, size := range sizes {
		k, _ := newTestKit(t)
		img := pumpKit(t, k, size[0], size[1])
		img = pumpKit(t, k, size[0], size[1]) // second frame applies the fit

		if len(k.ov.tbl.rows) == 0 {
			t.Fatalf("%dx%d: expected visible grid rows", size[0], size[1])
		}
		for _, w := range []struct {
			name string
			rect image.Rectangle
		}{
			{"overview", k.ov.win.GetContainer().GetWidget().Rect},
			{"selected", k.sp.win.GetContainer().GetWidget().Rect},
		} {
			if w.rect.Min.X < 0 || w.rect.Min.Y < 0 ||
				w.rect.Max.X > size[0] || w.rect.Max.Y > size[1] {
				t.Errorf("%dx%d: %s window %v out of bounds",
					size[0], size[1], w.name, w.rect)
			}
		}
		// The scroll viewport must fit inside the window: if it ever
		// overflows, rows render past the panel.
		vr := k.ov.tbl.scroll.ViewRect()
		wr := k.ov.win.GetContainer().GetWidget().Rect
		if vr.Dy() > 0 && vr.Max.Y > wr.Max.Y {
			t.Errorf("%dx%d: scroll viewport %v overflows window %v",
				size[0], size[1], vr, wr)
		}
		// Columns were fitted to the table width (not left natural).
		if k.ov.tbl.fittedFor <= 0 {
			t.Fatalf("%dx%d: table never fitted (fittedFor=%v)",
				size[0], size[1], k.ov.tbl.fittedFor)
		}
		if img == nil {
			t.Fatal("no image")
		}
	}
}
