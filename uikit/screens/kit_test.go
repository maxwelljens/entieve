package screens

import (
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"image"
	"math"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

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
	ts := theme.DefaultTypeScale()
	th := theme.NewTheme(theme.DefaultPalette(), ts)
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

	f := theme.NewFaces(ts)
	sizes := map[string]struct {
		face text.Face
		want float64
	}{
		"base":  {f.Base, ts.Base},
		"small": {f.Small, ts.Small},
		"large": {f.Large, ts.Large},
		"tiny":  {f.Tiny, ts.Tiny},
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
	if k.th.WidgetTheme() == nil || k.ui == nil || k.ov == nil || k.sp == nil {
		t.Fatal("kit missing theme/ui/windows")
	}
}

// TestOverviewRowClickSelectsThenShowsInfo: the overview Table's first
// click selects; a repeat click inside the double-click window opens
// Show Info on the row's definition — the old approach-on-activate moved
// to the world scene (and the row's context menu), and a row with no
// definition (a celestial) changes nothing.
func TestOverviewRowClickSelectsThenShowsInfo(t *testing.T) {
	k, cmds := newTestKit(t)
	row := &ovRow{info: sim.Info{Entity: ecs.Entity(777), Name: "Scuttler"}}
	k.ov.tbl.SetRows([]widgets.TableRow{
		{Cells: []string{"Scuttler", "10 m", "Ship", "0 m/s"}, Data: row, Spec: k.overviewSpec(row)},
	})

	k.ov.tbl.RowClickedForTest(0)
	if k.Selected() != ecs.Entity(777) {
		t.Fatalf("Selected() = %v, want 777", k.Selected())
	}
	if len(*cmds) != 0 {
		t.Fatalf("first click enqueued %d commands, want 0", len(*cmds))
	}

	// Repeat click: no approach command (that verb lives on the world
	// scene and in the row menu now), and an entity without a definition
	// opens no window.
	k.ov.tbl.RowClickedForTest(0)
	if len(*cmds) != 0 {
		t.Fatalf("repeat click enqueued %d commands, want 0", len(*cmds))
	}
	if k.InfoOpen() {
		t.Fatal("an entity with no definition opened Show Info")
	}

	// A real grid entity with a definition resolves (the player's own
	// hull always does): the repeat click swaps the window in.
	var target sim.Info
	for _, inf := range k.Sim.Grid() {
		if !inf.Visible {
			continue
		}
		if _, ok := k.Env().InfoRefFor(inf.Entity); ok {
			target = inf
			break
		}
	}
	if target.Entity.IsZero() {
		t.Fatal("fixture: no visible entity with a definition")
	}
	trow := &ovRow{info: target}
	k.ov.tbl.SetRows([]widgets.TableRow{
		{Cells: []string{target.Name}, Data: trow, Spec: k.overviewSpec(trow)},
	})
	k.ov.tbl.RowClickedForTest(0)
	k.ov.tbl.RowClickedForTest(0)
	if !k.InfoOpen() {
		t.Fatal("double-click on a resolvable row did not open Show Info")
	}
	ref, ok := k.Env().InfoRefFor(target.Entity)
	if !ok {
		t.Fatalf("fixture: %v (%s) resolves to no definition", target.Entity, target.Kind)
	}
	page, _ := infoPageFor(k.Env(), ref)
	if page.Title == "" || !strings.Contains(page.Title, "Vaštrek") {
		t.Errorf("page head = %q, want the entity name (the title no longer carries it)", page.Title)
	}
	if got := k.svc.info.TitleText(); got != "Info" {
		t.Errorf("info title = %q, want the base name Info", got)
	}
	if r := k.InfoWindow().GetContainer().GetWidget().Rect; r.Dx() == 0 {
		t.Error("info window never laid out")
	}
}

// TestSelectedButtonsEnqueueCommands covers the Selected Item window's
// command seam, which is now the world-verb table itself (the handlers
// are one-liners over DoWorldVerb): no selection silences the
// selected-target verbs, while Stop still fires (the panel — not the
// resolver — greys its button), and a real grid selection emits the
// intent's command at the selector ranges.
func TestSelectedButtonsEnqueueCommands(t *testing.T) {
	k, cmds := newTestKit(t)

	// No selection: the selected-target verbs stay quiet. (An arbitrary
	// entity id is no longer a target: the table resolves against the
	// live grid.)
	for _, action := range []string{"approach", "orbit", "lock-selected", "align", "mine"} {
		k.DoWorldVerb(action)
	}
	if len(*cmds) != 0 {
		t.Fatalf("no-selection click enqueued %d commands, want 0", len(*cmds))
	}
	// Stop needs no target, so the key path always stops; the button's
	// disabled state is the panel's own.
	k.DoWorldVerb("stop")
	if len(*cmds) != 1 {
		t.Fatalf("Stop enqueued %d commands, want 1", len(*cmds))
	}
	if _, ok := (*cmds)[0].(sim.CmdStop); !ok {
		t.Errorf("cmd[0] = %#v, want sim.CmdStop", (*cmds)[0])
	}
	*cmds = (*cmds)[:0]

	// A real lockable target: the first asteroid on the grid.
	var rock sim.Info
	for _, info := range k.Sim.Grid() {
		if info.Kind == sim.KindAsteroid {
			rock = info
			break
		}
	}
	if rock.Entity.IsZero() {
		t.Fatal("fixture: no asteroid on the grid")
	}
	k.Select(rock.Entity)
	for _, action := range []string{"approach", "orbit", "stop", "align", "lock-selected"} {
		k.DoWorldVerb(action)
	}
	if len(*cmds) != 5 {
		t.Fatalf("enqueued %d commands, want 5", len(*cmds))
	}
	if c, ok := (*cmds)[0].(sim.CmdApproach); !ok || c.Target != rock.Entity {
		t.Errorf("cmd[0] = %#v, want CmdApproach{%v}", (*cmds)[0], rock.Entity)
	}
	// The Orbit action carries the orbit selector's value (Kit.OrbitRange)
	// through the intent's zero Range.
	if c, ok := (*cmds)[1].(sim.CmdOrbit); !ok || c.Target != rock.Entity || c.Range != k.OrbitRange() {
		t.Errorf("cmd[1] = %#v, want CmdOrbit{%v, OrbitRange %v}", (*cmds)[1], rock.Entity, k.OrbitRange())
	}
	if _, ok := (*cmds)[2].(sim.CmdStop); !ok {
		t.Errorf("cmd[2] = %T, want sim.CmdStop", (*cmds)[2])
	}
	if c, ok := (*cmds)[3].(sim.CmdAlignTo); !ok || c.Target != rock.Entity {
		t.Errorf("cmd[3] = %#v, want CmdAlignTo{%v}", (*cmds)[3], rock.Entity)
	}
	if c, ok := (*cmds)[4].(sim.CmdLock); !ok || c.Target != rock.Entity {
		t.Errorf("cmd[4] = %#v, want CmdLock{%v}", (*cmds)[4], rock.Entity)
	}
	*cmds = (*cmds)[:0]

	// Mine resolves the first fitted mining laser at press and rides the
	// intent's Slot.
	k.DoWorldVerb("mine")
	if len(*cmds) != 1 {
		t.Fatalf("mine enqueued %d commands, want 1", len(*cmds))
	}
	if c, ok := (*cmds)[0].(sim.CmdMine); !ok || c.Asteroid != rock.Entity || c.Slot.Class == "" {
		t.Errorf("cmd = %#v, want CmdMine{%v, a fitted slot}", (*cmds)[0], rock.Entity)
	}
	*cmds = (*cmds)[:0]

	// Warp at the warp selector's value (Kit.WarpRange, the default 0) on
	// a real warpable target.
	target := warpableTarget(t, k, sim.WarpMinRange)
	k.Select(target.Entity)
	k.DoWorldVerb("warp")
	if len(*cmds) != 1 {
		t.Fatalf("warp enqueued %d commands, want 1", len(*cmds))
	}
	if c, ok := (*cmds)[0].(sim.CmdWarpTo); !ok || c.Target != target.Entity || c.Range != k.WarpRange() {
		t.Errorf("cmd = %#v, want CmdWarpTo{%v, WarpRange %v}", (*cmds)[0], target.Entity, k.WarpRange())
	}
	if c, ok := (*cmds)[0].(sim.CmdWarpTo); !ok || c.Range != WarpRanges[0] {
		t.Errorf("default WarpRange = %v, want WarpRanges[0] %v", k.WarpRange(), WarpRanges[0])
	}
}

// TestSelectedContextGating: the buttons are the verb table's panel
// state. With no selection the selected-target verbs are disabled; the
// fallback verbs (dock/jump/loot) are offered exactly when a nearest
// eligible target exists — TestDockOffersTheNearestWithoutSelection pins
// the other half.
func TestSelectedContextGating(t *testing.T) {
	k, _ := newTestKit(t)
	k.selected = 0
	k.Sync()
	for name, b := range map[string]*widget.Button{
		"approach": k.sp.approach, "warp": k.sp.warp, "stop": k.sp.stop,
		"lock": k.sp.lock, "orbit": k.sp.orbit, "mine": k.sp.mine,
	} {
		if !b.GetWidget().Disabled {
			t.Errorf("%s enabled with no selection", name)
		}
	}
	for name, b := range map[string]*widget.Button{
		"dock": k.sp.dock, "jump": k.sp.jump, "loot": k.sp.loot,
	} {
		if !b.GetWidget().Disabled {
			t.Errorf("%s offered with no selection and no eligible nearest target", name)
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
	// Dock answers the verb, not just the selected flag: the fallback may
	// still find a dockable station with a non-station selected.
	dockable := celestial.CanDock
	if !dockable {
		for _, info := range k.Sim.Grid() {
			if info.CanDock {
				dockable = true
				break
			}
		}
	}
	k.selected = celestial.Entity
	k.Sync()
	if k.sp.warp.GetWidget().Disabled {
		t.Error("warp disabled for a warpable celestial")
	}
	if got, want := k.sp.dock.GetWidget().Disabled, !dockable; got != want {
		t.Errorf("dock Disabled = %v, want %v (a dockable station resolves: %v)",
			got, want, dockable)
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

	want := 900.0 - 2*theme.ContentsPad
	if got := k.ov.tbl.Container().GetWidget().Rect.Dx(); float64(got) != want {
		t.Fatalf("table wrapper width = %v, want %v (window width minus padding)", got, want)
	}
	// The columns fit the table root — the wrapper minus the reserved
	// scrollbar gutter at its right edge and the ⋯ button's strip at the
	// header's right end (no column runs under the dots).
	sum := 0.0
	for _, w := range k.ov.tbl.WidthsForTest() {
		sum += w
	}
	if inner := want - float64(theme.Px(theme.ScrollGutter)+theme.Px(widgets.ColMenuW)); math.Abs(sum-inner) > 1e-6 {
		t.Fatalf("fitted column sum = %v, want %v (root minus the scrollbar gutter and the ⋯ strip)", sum, inner)
	}
	for i, btn := range k.ov.tbl.HdrBtnForTest() {
		// layoutCells tiles buttons with cumulative rounding.
		want := int(math.Round(uitest.SumPrefix(k.ov.tbl.WidthsForTest()[:i+1]))) -
			int(math.Round(uitest.SumPrefix(k.ov.tbl.WidthsForTest()[:i])))
		if btn.GetWidget().MinWidth != want {
			t.Errorf("header button %d MinWidth = %d, want %d",
				i, btn.GetWidget().MinWidth, want)
		}
	}
}

// TestDrawSmoke draws the kit at both design sizes headless: no panics,
// both windows stay in bounds and the overview table holds fitted rows.
func TestDrawSmoke(t *testing.T) {
	sizes := [][2]int{{1280, 720}, {1024, 700}}
	for _, size := range sizes {
		k, _ := newTestKit(t)
		img := pumpKit(t, k, size[0], size[1])
		img = pumpKit(t, k, size[0], size[1]) // second frame applies the fit

		if len(k.ov.tbl.RowsForTest()) == 0 {
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
		vr := k.ov.tbl.ScrollForTest().ViewRect()
		wr := k.ov.win.GetContainer().GetWidget().Rect
		if vr.Dy() > 0 && vr.Max.Y > wr.Max.Y {
			t.Errorf("%dx%d: scroll viewport %v overflows window %v",
				size[0], size[1], vr, wr)
		}
		// Columns were fitted to the table width (not left natural).
		if k.ov.tbl.FitWidthForTest() <= 0 {
			t.Fatalf("%dx%d: table never fitted (fittedFor=%v)",
				size[0], size[1], k.ov.tbl.FitWidthForTest())
		}
		if img == nil {
			t.Fatal("no image")
		}
	}
}
