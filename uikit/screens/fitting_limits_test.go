package screens

import (
	"image"
	"slices"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
)

// TestFittingLimitsAreTheQuotaTable pins the HULL LIMITS block: the mounts
// line stays text and the class quotas are the ship page's own box table
// (axis header cells over one row per class, every cell its count's box strip,
// a zero a dim dash) inside the 300 px column, with no worded "Grid quotas"
// or "CPU quotas" row left anywhere in the window.
func TestFittingLimitsAreTheQuotaTable(t *testing.T) {
	env, _ := newChromeEnv(t)
	f := NewFitting(env)
	ui := newWindowUI(t, env, f.Win())
	uitest.PumpWindow(t, ui, f.Sync, 1024, 700)
	uitest.PumpWindow(t, ui, f.Sync, 1024, 700)

	ship, ok := env.ShipState(env.Sim.Player())
	if !ok {
		t.Fatal("player has no ship")
	}
	def, ok := env.Defs().Ship(ship.Hull)
	if !ok {
		t.Fatalf("%s is not a def", ship.Hull)
	}
	want := shipQuota(def.GridClasses, def.CPUClasses)
	if want == nil {
		t.Fatalf("%s allows no class quota", ship.Hull)
	}

	// The mounts row is still the one text row of the block.
	if r := limitsRow(f, "Mounts"); r == nil || r.value.Label != mountsText(def.Mounts) {
		t.Fatalf("mounts row = %+v, want %q", r, mountsText(def.Mounts))
	}
	// The block prints no quota words: the table replaced both rows.
	for _, w := range windowTexts(f.Win().GetContainer()) {
		for _, banned := range []string{"Grid quotas", "CPU quotas", "SLOTS"} {
			if strings.Contains(w, banned) {
				t.Errorf("fitting window still prints %q (%q)", banned, w)
			}
		}
	}

	texts, strips := tableContent(t, f.quota)
	for _, ax := range want.Axes {
		if !slices.Contains(texts, ax) {
			t.Errorf("limits table headers = %v, want the %q column", texts, ax)
		}
	}
	for _, r := range want.Rows {
		if !slices.Contains(texts, r.Label) {
			t.Errorf("limits table = %v, want the %q row", texts, r.Label)
		}
	}
	// Every cell is a box strip as wide as its count (the two axis glyphs are
	// the table's only square images).
	size, step := theme.Px(profPipW), theme.Px(profPipW+profPipGap)
	var wantStrips []image.Point
	for range want.Axes {
		wantStrips = append(wantStrips, image.Pt(theme.Px(theme.StatIconSize), theme.Px(theme.StatIconSize)))
	}
	for _, r := range want.Rows {
		for _, n := range r.Counts {
			if n == 0 {
				wantStrips = append(wantStrips, image.Pt(size, max(1, theme.Px(1))))
				continue
			}
			wantStrips = append(wantStrips, image.Pt(profPipRowW(n, step, size), size))
		}
	}
	sortPoints(strips)
	sortPoints(wantStrips)
	if !slices.Equal(strips, wantStrips) {
		t.Errorf("limits table strips = %v, want %v", strips, wantStrips)
	}
	// It fits the column: no cell of the table crosses the scroll viewport.
	fitsColumn(t, f.quota)
	if got, maxW := f.quota.GetWidget().Rect.Dx(), f.left.ViewRect().Dx(); got > maxW {
		t.Errorf("limits table is %d px wide, want under the %d px column", got, maxW)
	}

	// The roster's only cpu-less hull: the rebuild drops the CPU column (the
	// table's shape is the hull's).
	scut := ship
	scut.Hull = "ship-scuttler"
	env.Sim.Ships.Set(env.Sim.Player(), scut)
	uitest.PumpWindow(t, ui, f.Sync, 1024, 700)
	texts, strips = tableContent(t, f.quota)
	if slices.Contains(texts, "CPU") {
		t.Errorf("scuttler limits table = %v, want no CPU column", texts)
	}
	if !slices.Contains(texts, "Grid") {
		t.Errorf("scuttler limits table = %v, want the grid column", texts)
	}
	sdef, ok := env.Defs().Ship("ship-scuttler")
	if !ok {
		t.Fatal("fixture: ship-scuttler missing")
	}
	if sq := shipQuota(sdef.GridClasses, sdef.CPUClasses); len(strips) != 1+len(sq.Rows) {
		t.Errorf("scuttler limits table has %d strips, want the grid header plus %d cells", len(strips), len(sq.Rows))
	}
	fitsColumn(t, f.quota)
}

// sortPoints orders image sizes by width, then height (the strip pin's read).
func sortPoints(ps []image.Point) {
	slices.SortFunc(ps, func(a, b image.Point) int {
		if a.X != b.X {
			return a.X - b.X
		}
		return a.Y - b.Y
	})
}

// limitsRow finds one of the block's text rows by label.
func limitsRow(f *Fitting, label string) *fitRow {
	for i := range f.limits {
		if f.limits[i].label.Label == label {
			return &f.limits[i]
		}
	}
	return nil
}

// windowTexts is every text a window draws (the sweep's read).
func windowTexts(root widget.PreferredSizeLocateableWidget) []string {
	var out []string
	var walk func(w widget.PreferredSizeLocateableWidget)
	walk = func(w widget.PreferredSizeLocateableWidget) {
		if txt, ok := w.(*widget.Text); ok && txt.Label != "" {
			out = append(out, txt.Label)
		}
		if c, ok := w.(*widget.Container); ok {
			for _, k := range c.Children() {
				walk(k)
			}
		}
	}
	walk(root)
	return out
}

// tableContent is a quota table's texts and the image bounds of its cells.
func tableContent(t *testing.T, table *widget.Container) ([]string, []image.Point) {
	t.Helper()
	if table == nil {
		t.Fatal("no quota table mounted")
	}
	var texts []string
	var strips []image.Point
	var walk func(w widget.PreferredSizeLocateableWidget)
	walk = func(w widget.PreferredSizeLocateableWidget) {
		switch v := w.(type) {
		case *widget.Text:
			texts = append(texts, v.Label)
		case *widget.Graphic:
			if v.Image != nil {
				strips = append(strips, v.Image.Bounds().Size())
			}
		case *widget.Container:
			for _, k := range v.Children() {
				walk(k)
			}
		}
	}
	walk(table)
	return texts, strips
}

// fitsColumn fails when a cell of the table crosses the table's own right edge
// (the 300 px column would clip it).
func fitsColumn(t *testing.T, table *widget.Container) {
	t.Helper()
	box := table.GetWidget().Rect
	grid := table.Children()[0].(*widget.Container)
	for _, cell := range grid.Children() {
		if r := cell.GetWidget().Rect; r.Max.X > box.Max.X || r.Min.X < box.Min.X {
			t.Errorf("table cell %v crosses the block's %v", r, box)
		}
	}
}
