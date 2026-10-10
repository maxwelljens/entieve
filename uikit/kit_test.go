package uikit

import (
	"image"
	"math"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// listSelectedArgs builds the event ebitenui fires when a list entry is
// activated (PreviousEntry nil means "nothing was selected before").
func listSelectedArgs(entry, prev any) *widget.ListEntrySelectedEventArgs {
	return &widget.ListEntrySelectedEventArgs{Entry: entry, PreviousEntry: prev}
}

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

// row builds a synthetic overview row for sorting tests.
func row(name string, dist float64, kind string, vel float64) *Row {
	return &Row{info: sim.Info{
		Entity:   ecs.Entity(len(name)*1000 + int(dist)),
		Name:     name,
		Distance: dist,
		Kind:     kind,
		Vel:      core.V(vel, 0),
	}}
}

func names(rows []*Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.info.Name
	}
	return out
}

// TestSortRows checks every column in both directions, including the
// stability guarantee for equal keys.
func TestSortRows(t *testing.T) {
	base := func() []*Row {
		return []*Row{
			row("bravo", 10, "ship", 30),
			row("alpha", 10, "asteroid", 30),
			row("charlie", 5, "station", 10),
		}
	}
	tests := []struct {
		name string
		col  SortColumn
		asc  bool
		want []string
	}{
		{"name asc", SortName, true, []string{"alpha", "bravo", "charlie"}},
		{"name desc", SortName, false, []string{"charlie", "bravo", "alpha"}},
		{"distance asc", SortDistance, true, []string{"charlie", "bravo", "alpha"}},
		{"distance desc stable", SortDistance, false, []string{"bravo", "alpha", "charlie"}},
		{"kind asc", SortKind, true, []string{"alpha", "bravo", "charlie"}},
		{"velocity asc stable", SortVelocity, true, []string{"charlie", "bravo", "alpha"}},
		{"velocity desc stable", SortVelocity, false, []string{"bravo", "alpha", "charlie"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := base()
			sortRows(rows, tt.col, tt.asc)
			got := names(rows)
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("order = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

// TestRowActivationSelectsThenApproaches: first activation selects, a
// second activation of the same row inside the double-click window
// enqueues CmdApproach with the right target.
func TestRowActivationSelectsThenApproaches(t *testing.T) {
	k, cmds := newTestKit(t)
	r := &Row{info: sim.Info{Entity: ecs.Entity(777), Name: "Scuttler"}}

	first := listSelectedArgs(r, nil)
	k.entrySelected(first)
	if k.Selected() != ecs.Entity(777) {
		t.Fatalf("Selected() = %v, want 777", k.Selected())
	}
	if len(*cmds) != 0 {
		t.Fatalf("first activation enqueued %d commands, want 0", len(*cmds))
	}

	second := listSelectedArgs(r, r)
	k.entrySelected(second)
	if len(*cmds) != 1 {
		t.Fatalf("second activation enqueued %d commands, want 1", len(*cmds))
	}
	cmd, ok := (*cmds)[0].(sim.CmdApproach)
	if !ok {
		t.Fatalf("command type = %T, want sim.CmdApproach", (*cmds)[0])
	}
	if cmd.Target != ecs.Entity(777) {
		t.Fatalf("CmdApproach.Target = %v, want 777", cmd.Target)
	}

	// A different row within the window is just a selection.
	other := &Row{info: sim.Info{Entity: ecs.Entity(778), Name: "Rock"}}
	k.entrySelected(listSelectedArgs(other, r))
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

	// No selection: approach/warp are guarded.
	k.approachSelected()
	k.warpSelected()
	if len(*cmds) != 0 {
		t.Fatalf("no-selection click enqueued %d commands, want 0", len(*cmds))
	}

	k.selected = ecs.Entity(42)
	k.approachSelected()
	k.warpSelected()
	k.stopSelected()
	if len(*cmds) != 3 {
		t.Fatalf("enqueued %d commands, want 3", len(*cmds))
	}
	if c, ok := (*cmds)[0].(sim.CmdApproach); !ok || c.Target != ecs.Entity(42) {
		t.Errorf("cmd[0] = %#v, want CmdApproach{42}", (*cmds)[0])
	}
	if c, ok := (*cmds)[1].(sim.CmdWarpTo); !ok || c.Target != ecs.Entity(42) || c.Range != 10000 {
		t.Errorf("cmd[1] = %#v, want CmdWarpTo{42, 10000}", (*cmds)[1])
	}
	if _, ok := (*cmds)[2].(sim.CmdStop); !ok {
		t.Errorf("cmd[2] = %T, want sim.CmdStop", (*cmds)[2])
	}
}

// TestOverviewMinSizeAndRefit covers the window min-size clamp and the
// resize handler reflowing the elastic columns.
func TestOverviewMinSizeAndRefit(t *testing.T) {
	k, _ := newTestKit(t)
	k.Sync()
	if err := k.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}

	win := k.ov.win
	win.SetLocation(image.Rect(0, 0, 100, 50))
	r := win.GetContainer().GetWidget().Rect
	if r.Dx() != 320 || r.Dy() != 240 {
		t.Fatalf("min-size clamp rect = %v, want 320x240", r)
	}

	// Synthetic short rows so the natural fit is well under the width and
	// the surplus goes entirely to Name (sum == avail, exactly).
	k.ov.order = []*Row{
		row("A", 1, "ship", 1),
		row("B", 2, "ship", 2),
	}
	win.SetLocation(image.Rect(0, 0, 900, 600))
	win.ResizeEvent.Fire(&widget.WindowChangedEventArgs{
		Window: win,
		Rect:   win.GetContainer().GetWidget().Rect,
	})
	if err := k.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}

	want := 900.0 - 2*contentsPad
	sum := 0.0
	for _, c := range k.ov.cols {
		sum += c.Width
	}
	if math.Abs(sum-want) > 1e-6 {
		t.Fatalf("fitted column sum = %v, want %v (window width minus padding)", sum, want)
	}
	for i, btn := range k.ov.btns {
		// applyHeader tiles buttons with cumulative rounding.
		want := int(math.Round(sumPrefix(k.ov.cols[:i+1]))) -
			int(math.Round(sumPrefix(k.ov.cols[:i])))
		if btn.GetWidget().MinWidth != want {
			t.Errorf("header button %d MinWidth = %d, want %d",
				i, btn.GetWidget().MinWidth, want)
		}
	}
}

// sumPrefix adds the first n fitted column widths.
func sumPrefix(cols []ColWidth) float64 {
	s := 0.0
	for _, c := range cols {
		s += c.Width
	}
	return s
}

// TestDrawSmoke draws the kit at both design sizes headless: no panics,
// both windows stay in bounds, and the windowed rows fit the list.
func TestDrawSmoke(t *testing.T) {
	sizes := [][2]int{{1280, 720}, {1024, 700}}
	for _, size := range sizes {
		k, _ := newTestKit(t)
		k.Sync()
		if err := k.Update(); err != nil {
			t.Fatalf("%dx%d Update: %v", size[0], size[1], err)
		}
		img := ebiten.NewImage(size[0], size[1])
		k.Draw(img)

		// A second frame exercises Sync after layout (page/refit paths).
		k.Sync()
		if err := k.Update(); err != nil {
			t.Fatalf("%dx%d second Update: %v", size[0], size[1], err)
		}
		k.Draw(img)

		if len(k.ov.order) == 0 {
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
		// The virtualized page must fit inside the window: if it ever
		// overflows, rows render past the panel and tints misalign.
		lr := k.ov.list.GetWidget().Rect
		wr := k.ov.win.GetContainer().GetWidget().Rect
		if lr.Dy() > 0 && lr.Max.Y > wr.Max.Y {
			t.Errorf("%dx%d: list rect %v overflows window %v",
				size[0], size[1], lr, wr)
		}
		if len(k.ov.page) == 0 || k.ov.rowH <= 0 {
			t.Fatalf("%dx%d: page=%d rowH=%v, want a fitted page",
				size[0], size[1], len(k.ov.page), k.ov.rowH)
		}

		// A wheel-page rebuild must re-apply the selection that
		// SetEntries silently dropped.
		if len(k.ov.order) > k.ov.visibleN && k.ov.visibleN > 3 {
			r := k.ov.page[3]
			k.selected = r.info.Entity
			k.ov.scrollBy(-3) // page forward
			k.Sync()
			if got := k.ov.list.SelectedEntry(); got != any(r) {
				t.Errorf("%dx%d: selection not re-applied after page rebuild (%T)",
					size[0], size[1], got)
			}
		}
	}
}
