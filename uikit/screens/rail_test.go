package screens

import (
	"image/color"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/icons"
	"github.com/entieve/entieve/uikit/internal/uitest"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// The navigation rail's own tests: geometry through the scale funnel
// (the design gutter and the collapsed edge tab), the context rule that
// grays an entry, the toggle emission (and its silence when grayed), the
// open-state glow, the group separators and the collapse.

// railEntries3 is a three-section fixture: Ship (one open, one closed),
// Trade (station-dependent), World (space-only).
func railEntries3(open *bool) []RailEntry {
	return []RailEntry{
		{Icon: "kind_ship", Title: "Overview", Group: "Ship", Open: func() bool { return *open }},
		{Icon: "svc_fitting", Title: "Fitting", Hint: func() string { return "F — toggle fitting" }, Group: "Ship"},
		{Icon: "svc_market", Title: "Market", Hint: func() string { return "G — toggle market" }, Group: "Trade", Service: "market"},
		{Icon: "svc_scanner", Title: "Scanner", Hint: func() string { return "C — toggle scanner" }, Group: "World", SpaceOnly: true},
	}
}

// TestRailGeometry pins the rail's geometry at the design viewport,
// at scale 1 and through the scale funnel (the design units the Px
// funnel scales — the rail's metrics must follow it like every other
// panel): the column starts at the screen's left edge, is exactly
// RailInset wide, sits level with the window row, and every entry cell
// is a RailInset square. The collapsed rail keeps only the slim edge tab.
func TestRailGeometry(t *testing.T) {
	for _, tc := range []struct {
		name  string
		scale float64
	}{{"scale 1", 1}, {"scale 1.5", 1.5}} {
		t.Run(tc.name, func(t *testing.T) {
			uitest.UseScale(t, tc.scale)
			env, _ := newChromeEnv(t)
			c := newChromeUI(env)
			open := false
			r := NewRail(env, railEntries3(&open))
			c.AddPanel(r.Container(), r)

			c.Frame(t, 1280, 720)
			got := r.Container().GetWidget().Rect
			if got.Min.X != 0 || got.Min.Y != theme.Px(RailTopInset) {
				t.Errorf("rail at %v, want the left edge at x=0, top at y=%d", got, theme.Px(RailTopInset))
			}
			if got.Dx() != theme.Px(RailInset) {
				t.Errorf("rail width = %d, want RailInset %d (design %d × the funnel)",
					got.Dx(), theme.Px(RailInset), RailInset)
			}
			if got.Max.X > 1280 || got.Max.Y > 720 || got.Min.Y < 0 {
				t.Errorf("rail %v out of the 1280×720 frame", got)
			}
			for _, title := range []string{"Overview", "Fitting", "Market", "Scanner"} {
				b := r.Button(title)
				if b == nil {
					t.Fatalf("no button for %q", title)
				}
				br := b.GetWidget().Rect
				// The cell IS the 63×63 entry square: the rail's outer
				// width is RailInset = cells + the scrollbar's reserved
				// ScrollGutter, which rides beside the cells.
				if br.Dx() != theme.Px(RailInset-theme.ScrollGutter) || br.Dy() != theme.Px(RailInset-theme.ScrollGutter) {
					t.Errorf("%s cell = %v, want %d² (RailInset − ScrollGutter)",
						title, br, theme.Px(RailInset-theme.ScrollGutter))
				}
			}

			// Collapse: the entries vanish (a zero-width column) and only
			// the slim edge tab remains.
			r.SetCollapsed(true)
			c.Frame(t, 1280, 720)
			if r.list.GetWidget().IsVisible() {
				t.Error("collapsed rail still shows its entry list")
			}
			if w := r.Container().GetWidget().Rect.Dx(); w != theme.Px(railTabW) {
				t.Errorf("collapsed rail width = %d, want the edge tab %d", w, theme.Px(railTabW))
			}
			r.SetCollapsed(false)
			c.Frame(t, 1280, 720)
			if !r.list.GetWidget().IsVisible() || r.Collapsed() {
				t.Error("expanded rail did not bring the entries back")
			}
		})
	}
}

// TestRailIconsInsideCells pins the icon anchoring class the playtest
// caught (defect: glyphs hung over the cell's right edge): every entry's
// glyph rect sits strictly inside its button square with EQUAL margins
// on all four sides (±1 px for integer centring), at scale 1 and 1.5.
// The same cell draws the open-state glow and, collapsed, the rail keeps
// only the edge tab — neither may move the glyph or leave the column.
func TestRailIconsInsideCells(t *testing.T) {
	for _, tc := range []struct {
		name  string
		scale float64
	}{{"scale 1", 1}, {"scale 1.5", 1.5}} {
		t.Run(tc.name, func(t *testing.T) {
			uitest.UseScale(t, tc.scale)
			env, _ := newChromeEnv(t)
			c := newChromeUI(env)
			open := true // the glow cell: the glyph must not move with it
			r := NewRail(env, railEntries3(&open))
			c.AddPanel(r.Container(), r)
			c.Frame(t, 1280, 720)

			railRect := r.Container().GetWidget().Rect
			for i, title := range []string{"Overview", "Fitting", "Market", "Scanner"} {
				btn := r.Button(title)
				if btn == nil {
					t.Fatalf("no button for %q", title)
				}
				cell := btn.GetWidget().Rect
				glyph := r.icons[i].Widget().GetWidget().Rect
				if glyph.Dx() != theme.Px(railIcon) || glyph.Dy() != theme.Px(railIcon) {
					t.Errorf("%s: glyph box = %v, want %d² design (the Px funnel)", title, glyph, railIcon)
				}
				if !glyph.In(cell) {
					t.Errorf("%s: glyph %v escapes its cell %v", title, glyph, cell)
					continue
				}
				l, rr := glyph.Min.X-cell.Min.X, cell.Max.X-glyph.Max.X
				tp, b := glyph.Min.Y-cell.Min.Y, cell.Max.Y-glyph.Max.Y
				// The cell is a square (63×63, the gutter rides beside
				// it), so the glyph is centred: each opposing pair of
				// margins matches ±1.
				if abs(l-rr) > 1 || abs(tp-b) > 1 {
					t.Errorf("%s: glyph margins L%d R%d T%d B%d in %v, want each pair equal ±1",
						title, l, rr, tp, b, cell)
				}
				if l < 1 || rr < 1 || tp < 1 || b < 1 {
					t.Errorf("%s: glyph %v touches its cell edge %v (want visible air)", title, glyph, cell)
				}
				if cell.Dx() != theme.Px(RailInset-theme.ScrollGutter) || cell.Dy() != theme.Px(RailInset-theme.ScrollGutter) {
					t.Errorf("%s: cell = %v, want %d² (RailInset − ScrollGutter)",
						title, cell, theme.Px(RailInset-theme.ScrollGutter))
				}
			}

			// The collapse tab rides inside the column with equal side
			// margins (it stretches across the rail).
			tab := r.Tab().GetWidget().Rect
			if !tab.In(railRect) {
				t.Errorf("tab %v outside the rail %v", tab, railRect)
			}
			if tl, tr := tab.Min.X-railRect.Min.X, railRect.Max.X-tab.Max.X; abs(tl-tr) > 1 {
				t.Errorf("tab margins L%d R%d in %v, want equal ±1", tl, tr, railRect)
			}
		})
	}
}

// TestRailColumnCapsAndScrolls: the host's viewport cap (what the game
// passes at the design sizes) keeps the rail column inside its max
// height — a tall entry table clips into a wheel-scrollable viewport
// (the tables' convention: a notch moves the offset and the entries
// with it) instead of running past the bottom edge — and no cap
// restores the full column.
func TestRailColumnCapsAndScrolls(t *testing.T) {
	env, _ := newChromeEnv(t)
	c := newChromeUI(env)
	open := false
	r := NewRail(env, railEntries3(&open))
	c.AddPanel(r.Container(), r)
	c.Frame(t, 1280, 720)
	full := r.Container().GetWidget().Rect.Dy()
	// Cap below the full column: tab + head gap + one cell of list.
	capPx := theme.Px(railTabH + railHeadGap + RailInset)
	r.SetMaxHeight(capPx)
	c.Frame(t, 1280, 720)
	got := r.Container().GetWidget().Rect
	if got.Dy() != capPx {
		t.Errorf("capped column height = %d, want the cap %d (full was %d)", got.Dy(), capPx, full)
	}
	if got.Dy() >= full {
		t.Fatalf("fixture: cap %d does not bind the full column %d", got.Dy(), full)
	}
	if over := r.scrollOverflow(); over <= 0 {
		t.Fatalf("capped column overflow = %v, want > 0", over)
	}
	// Wheel up: the offset moves and the entries ride it.
	top0 := r.icons[3].Widget().GetWidget().Rect.Min.Y
	r.scroll.GetWidget().ScrolledEvent.Fire(&widget.WidgetScrolledEventArgs{Y: -2})
	c.Update(t)
	c.Frame(t, 1280, 720)
	if r.scroll.ScrollTop <= 0 {
		t.Fatalf("wheel: ScrollTop = %v, want > 0", r.scroll.ScrollTop)
	}
	if got := r.icons[3].Widget().GetWidget().Rect.Min.Y; got >= top0 {
		t.Errorf("wheel: entry 3 at %d, want above %d (column scrolled)", got, top0)
	}
	// Uncap: the column returns to its full height.
	r.SetMaxHeight(0)
	c.Frame(t, 1280, 720)
	if got := r.Container().GetWidget().Rect.Dy(); got != full {
		t.Errorf("uncapped column height = %d, want the full %d", got, full)
	}
}

// TestRailGutterAndGlyphLiterals pins the +75 % itself: the geometry
// pins above derive from RailInset, so this one pins the constants —
// the 63×63 entry square sits beside the 24 px scrollbar gutter, so
// RailInset (cells + gutter) is 87, and the glyph grown with the cell
// (20 → 35, the Button glyph's own ×1.75).
func TestRailGutterAndGlyphLiterals(t *testing.T) {
	if RailInset != 87 {
		t.Errorf("RailInset = %d, want 87 (63 cells + 24 gutter)", RailInset)
	}
	if railIcon != 35 {
		t.Errorf("railIcon = %d, want 35 (IconButton 20 × 1.75)", railIcon)
	}
	// The collapsed edge tab carries the furniture token's air: 20 =
	// the "<" glyph (≈10) + SpSm on each side.
	if railTabW != 20 {
		t.Errorf("railTabW = %d, want 20 (glyph + 2×SpSm)", railTabW)
	}
}

// TestRailScrollbarGutter: the entry column's scrollbar rides a reserved
// gutter inside the rail — the rail keeps its RailInset width, the
// reserved column is exactly ScrollGutter wide even while the rail is
// hidden (hide-when-fits), and once the column overflows the handle
// shows and never intersects the entry viewport — at scale 1 and 1.5.
func TestRailScrollbarGutter(t *testing.T) {
	for _, tc := range []struct {
		name  string
		scale float64
	}{{"scale 1", 1}, {"scale 1.5", 1.5}} {
		t.Run(tc.name, func(t *testing.T) {
			uitest.UseScale(t, tc.scale)
			env, _ := newChromeEnv(t)
			c := newChromeUI(env)
			open := false
			r := NewRail(env, railEntries3(&open))
			c.AddPanel(r.Container(), r)
			c.Frame(t, 1280, 720) // lays the viewport out
			c.Frame(t, 1280, 720) // Sync sees the laid-out overflow

			// Fitting: the rail hides, the gutter stays reserved.
			if r.rail.Bar().GetWidget().IsVisible() {
				t.Error("rail scrollbar visible while the entries fit")
			}
			railW := r.Container().GetWidget().Rect.Dx()
			gutter := railW - r.scroll.ViewRect().Dx()
			if gutter != theme.Px(theme.ScrollGutter) {
				t.Errorf("reserved gutter = %d, want ScrollGutter %d", gutter, theme.Px(theme.ScrollGutter))
			}
			if railW != theme.Px(RailInset) {
				t.Errorf("rail width = %d, want RailInset %d (the gutter is carved inside)", railW, theme.Px(RailInset))
			}

			// Cap below the full column: the entries overflow, the rail
			// shows and the handle clears the viewport.
			r.SetMaxHeight(theme.Px(railTabH + railHeadGap + RailInset))
			c.Frame(t, 1280, 720)
			c.Frame(t, 1280, 720)
			if !r.rail.Bar().GetWidget().IsVisible() {
				t.Fatalf("rail scrollbar hidden over an overflowing column (overflow %v)", r.scrollOverflow())
			}
			bar := r.rail.Bar().GetWidget().Rect
			if bar.Overlaps(r.scroll.ViewRect()) {
				t.Errorf("scrollbar %v intersects the entry viewport %v", bar, r.scroll.ViewRect())
			}
			if !bar.In(r.Container().GetWidget().Rect) {
				t.Errorf("scrollbar %v leaves the rail %v", bar, r.Container().GetWidget().Rect)
			}
		})
	}
}

// abs is |v| (the geometry pins' tolerance helper).
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// TestRailCollapseTabClicks: the edge tab toggles the collapse — the
// gesture the settings sidecar then persists (the game mirrors
// Collapsed into settings.json).
func TestRailCollapseTabClicks(t *testing.T) {
	env, _ := newChromeEnv(t)
	c := newChromeUI(env)
	open := false
	r := NewRail(env, railEntries3(&open))
	c.AddPanel(r.Container(), r)
	c.Frame(t, 1280, 720)

	fireRailClick(t, c, r.Tab())
	if !r.Collapsed() {
		t.Fatal("edge tab click did not collapse the rail")
	}
	fireRailClick(t, c, r.Tab())
	if r.Collapsed() {
		t.Error("second edge tab click did not expand the rail")
	}
}

// TestRailGatingAndToggle: an entry is available in its context and
// grayed outside it — grayed entries emit NOTHING (the established
// convention), available ones emit exactly one Toggle per click, and the
// accent glow tracks the host's open flag only while the entry is
// available.
func TestRailGatingAndToggle(t *testing.T) {
	th := theme.NewSeam(theme.DefaultPalette(), theme.DefaultTypeScale())
	open := false
	entries := []RailEntry{
		{Icon: "kind_ship", Title: "Overview", Group: "Ship", Open: func() bool { return open }},
		// Always "open" in the host, so its glow can only be the gating
		// that suppresses it (grayed wins over the open state).
		{Icon: "svc_scanner", Title: "Scanner", Group: "World", SpaceOnly: true, Open: func() bool { return true }},
		{Icon: "kind_station", Title: "Station Services", Group: "Station", DockedOnly: true},
		{Icon: "svc_fitting", Title: "Fitting", Group: "Trade", Service: "fitting"},
	}

	// In space: the DockedOnly entry is grayed, the rest are live. The
	// rail mounts on a UI so its handlers join the deferred queue the
	// way a real frame registers them.
	cv := &stubView{}
	sink := &recordingSink{}
	env := widgets.NewSeams(th, cv, sink, nil)
	c := newChromeUI(env)
	r := NewRail(env, entries)
	c.AddPanel(r.Container(), r)
	c.Frame(t, 1280, 720)

	if r.Button("Station Services").GetWidget().Disabled != true {
		t.Error("DockedOnly entry is not grayed in space")
	}
	fireRailClick(t, c, r.Button("Station Services"))
	if len(sink.got) != 0 {
		t.Errorf("grayed entry emitted %v, want nothing", sink.got)
	}
	fireRailClick(t, c, r.Button("Overview"))
	if len(sink.got) != 1 {
		t.Fatalf("click emitted %v, want the single Toggle", sink.got)
	}
	if tg, ok := sink.got[0].(widgets.Toggle); !ok || tg.Screen != "Overview" {
		t.Errorf("intent = %#v, want Toggle{Overview}", sink.got[0])
	}

	// Open-state glow: closed → idle, open → the accent set, grayed →
	// idle even when the host says open. (The image sets are merged with
	// the theme on validate, so the pin is the nine-slice the cell
	// draws — stable on both paths.)
	glowOf := func(title string) *widget.ButtonImage { return r.Button(title).Image() }
	if glowOf("Overview").Idle != r.imgIdle.Idle {
		t.Error("closed entry does not draw the idle cell")
	}
	if glowOf("Scanner").Idle != r.imgOpen.Idle {
		t.Error("host-open entry does not draw the accent glow")
	}
	open = true
	r.Sync()
	if glowOf("Overview").Idle != r.imgOpen.Idle {
		t.Error("open entry does not draw the accent glow")
	}
	cv.docked = true
	cv.station = widgets.StationInfo{Name: "Stub", Services: []string{"market"}}
	r.Sync()
	if r.Button("Scanner").GetWidget().Disabled != true {
		t.Error("SpaceOnly entry is not grayed while docked")
	}
	if r.Button("Fitting").GetWidget().Disabled != true {
		t.Error("entry grayed at a station lacking its service")
	}
	if glowOf("Scanner").Idle != r.imgIdle.Idle {
		t.Error("grayed entry still draws the open glow (grayed wins)")
	}
	if r.Button("Station Services").GetWidget().Disabled {
		t.Error("DockedOnly entry grayed while docked")
	}
	// At a station WITH fitting the entry comes back.
	cv.station.Services = []string{"market", "fitting"}
	r.Sync()
	if r.Button("Fitting").GetWidget().Disabled {
		t.Error("entry still grayed at a station offering its service")
	}
}

// TestRailTooltips: the hover model is icon + title + the host's live hint,
// swapped for the "unavailable" line when the entry is grayed here, and the
// tooltip's change key follows the hint — an open tip must not keep the
// chord it was built with across a rebind.
func TestRailTooltips(t *testing.T) {
	hint := "G — toggle market"
	e := RailEntry{Icon: "svc_market", Title: "Market", Hint: func() string { return hint }}
	m := entryTip(e, true)
	if m.Icon != "svc_market" || m.Title != "Market" || len(m.Hints) != 1 || m.Hints[0] != hint {
		t.Errorf("tip = %+v, want the icon, title and %q", m, hint)
	}
	key := entryTipKey(e, true)
	hint = "U — toggle market"
	if entryTipKey(e, true) == key {
		t.Error("the tooltip change key ignores the hint (an open tip keeps a stale chord)")
	}
	if m = entryTip(e, true); len(m.Hints) != 1 || m.Hints[0] != hint {
		t.Errorf("rebuilt tip = %+v, want the live %q", m, hint)
	}
	m = entryTip(e, false)
	if len(m.Hints) != 1 || m.Hints[0] != "Unavailable here" {
		t.Errorf("grayed tip = %+v, want the unavailable line", m)
	}
	// An entry the host gave no hint for draws none.
	if m = entryTip(RailEntry{Title: "Overview"}, true); len(m.Hints) != 0 {
		t.Errorf("hintless tip = %+v, want no hint line", m)
	}
}

// TestRailGroupsAndOrder: entries draw in the host's order with one
// hairline per group change, and the fixture's icons all resolve in the
// manifest (the rail draws one glyph per entry — a missing one would be
// a broken image, so the derivation pins it here too).
func TestRailGroupsAndOrder(t *testing.T) {
	env, _ := newChromeEnv(t)
	c := newChromeUI(env)
	open := false
	entries := railEntries3(&open)
	r := NewRail(env, entries)
	c.AddPanel(r.Container(), r)
	c.Frame(t, 1280, 720)

	got := r.Entries()
	if len(got) != len(entries) {
		t.Fatalf("entries = %d, want %d", len(got), len(entries))
	}
	for i, e := range got {
		if e.Title != entries[i].Title || e.Group != entries[i].Group {
			t.Errorf("entry %d = %q/%q, want %q/%q in host order",
				i, e.Title, e.Group, entries[i].Title, entries[i].Group)
		}
		if !icons.Has(e.Icon) {
			t.Errorf("%s: icon %q not in the manifest", e.Title, e.Icon)
		}
	}
	// Ship | Trade | World → two separators among the four buttons (the
	// edge tab is the list's sibling, not a list child).
	seps, buttons := 0, 0
	for _, ch := range r.list.Children() {
		if ch.GetWidget().Rect.Dy() == theme.Px(1) {
			seps++
		} else {
			buttons++
		}
	}
	if seps != 2 || buttons != len(entries) {
		t.Errorf("list children: %d hairlines + %d entries, want 2 + %d", seps, buttons, len(entries))
	}
}

// TestEntryEnabledRules pins the pure context rule: SpaceOnly grays
// docked, DockedOnly grays in space, a station-dependent entry grays
// only while docked at a station lacking its service, and everything
// else is available.
func TestEntryEnabledRules(t *testing.T) {
	for _, tc := range []struct {
		name     string
		entry    RailEntry
		docked   bool
		services []string
		want     bool
	}{
		{"plain in space", RailEntry{}, false, nil, true},
		{"plain docked", RailEntry{}, true, nil, true},
		{"space-only in space", RailEntry{SpaceOnly: true}, false, nil, true},
		{"space-only docked", RailEntry{SpaceOnly: true}, true, nil, false},
		{"docked-only in space", RailEntry{DockedOnly: true}, false, nil, false},
		{"docked-only docked", RailEntry{DockedOnly: true}, true, nil, true},
		{"service undocked", RailEntry{Service: "fitting"}, false, nil, true},
		{"service present", RailEntry{Service: "fitting"}, true, []string{"market", "fitting"}, true},
		{"service absent", RailEntry{Service: "fitting"}, true, []string{"market"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := entryEnabled(tc.entry, tc.docked, tc.services); got != tc.want {
				t.Errorf("entryEnabled = %v, want %v", got, tc.want)
			}
		})
	}
}

// fireRailClick runs one button's widget-level click the way ebitenui
// would — Fire queues it, the UI update flushes it (the same seam the
// game's click helper pumps through kit.Update).
func fireRailClick(t *testing.T, c *chromeUI, b *widget.Button) {
	t.Helper()
	w := b.GetWidget()
	w.MouseButtonClickedEvent.Fire(&widget.WidgetMouseButtonClickedEventArgs{
		Widget: w,
		Button: ebiten.MouseButtonLeft,
	})
	c.Update(t)
}

// TestRailGroupTints: the navbar glyphs are colour-coded by section —
// theme.RailGroupColor's six palette roles (Ship cyan / Trade green /
// Industry orange / Station gold / World blue / Help & System silver).
// State wins over the group: a grayed entry draws the disabled dim, and
// the open-state cell chrome (the accent glow) leaves the tint alone.
// The tint feeds icons.Get, which caches per name|size|tint — six fixed
// tints per glyph keep the cache small (the rail's memory pin,
// TestMemoryFootprintPump, covers the draw path).
func TestRailGroupTints(t *testing.T) {
	env, _ := newChromeEnv(t)
	c := newChromeUI(env)
	open := false
	entries := []RailEntry{
		{Icon: "kind_ship", Title: "Overview", Group: "Ship", Open: func() bool { return open }},
		{Icon: "svc_market", Title: "Market", Group: "Trade"},
		{Icon: "svc_fitting", Title: "Fitting", Group: "Industry"},
		{Icon: "kind_station", Title: "Services", Group: "Station"},
		{Icon: "svc_scanner", Title: "Scanner", Group: "World"},
		{Icon: "act_lock", Title: "Help", Group: "Help & System"},
		// Grayed here (space), so its group colour never reaches the glyph.
		{Icon: "kind_station", Title: "Gated", Group: "Station", DockedOnly: true},
	}
	r := NewRail(env, entries)
	c.AddPanel(r.Container(), r)
	c.Frame(t, 1280, 720)

	p := env.Pal()
	glyph := func(name string, tint color.RGBA) *ebiten.Image {
		return icons.Get(name, theme.Px(railIcon), tint)
	}
	for i, e := range entries[:len(entries)-1] {
		want := theme.RailGroupColor(p, e.Group)
		if want == p.Text {
			t.Errorf("%s: group %q has no role colour (fell back to Text)", e.Title, e.Group)
		}
		if got := glyph(e.Icon, want); r.icons[i].Widget().Image != got {
			t.Errorf("%s: glyph is not the %s group tint", e.Title, e.Group)
		}
	}
	// The six roles are six distinct colours.
	seen := map[color.RGBA]string{}
	for _, e := range entries[:len(entries)-1] {
		tint := theme.RailGroupColor(p, e.Group)
		if prev, dup := seen[tint]; dup {
			t.Errorf("groups %q and %q share the tint %v", prev, e.Group, tint)
		}
		seen[tint] = e.Group
	}
	// Grayed wins over the group: the Gated entry is docked-only.
	gated := len(entries) - 1
	want := iconColor(p.Dim, theme.IconDisabledA)
	if got := glyph(entries[gated].Icon, want); r.icons[gated].Widget().Image != got {
		t.Error("grayed entry does not draw the disabled dim over its group colour")
	}
	// Open-state chrome: the accent cell glow while the tint keeps its role.
	open = true
	r.Sync()
	if got := glyph(entries[0].Icon, theme.RailGroupColor(p, entries[0].Group)); r.icons[0].Widget().Image != got {
		t.Error("open state changed the glyph tint")
	}
	if r.Button("Overview").Image().Idle != r.imgOpen.Idle {
		t.Error("open state lost the accent cell chrome")
	}
}

// TestDefaultWindowRectsClearTheRailGutter: every default window rect a
// uikit constructor ships starts right of the rail's gutter — the
// construction-time Location literals, at the design scale and through
// the scale funnel (1.5×). These are the rects a host sees before its
// first placement pass (and what uikit-only integrations render); the
// game's composed defaults are pinned the same way in
// game/rail_gutter_test.go.
func TestDefaultWindowRectsClearTheRailGutter(t *testing.T) {
	for _, tc := range []struct {
		name  string
		scale float64
	}{{"scale 1", 1}, {"scale 1.5", 1.5}} {
		t.Run(tc.name, func(t *testing.T) {
			uitest.UseScale(t, tc.scale)
			k, _ := newTestKit(t)
			env := k.Env()
			cases := []struct {
				name string
				win  *widget.Window
			}{
				{"overview", k.OverviewWindow()},
				{"selected item", k.SelectedWindow()},
				{"show info", k.InfoWindow()},
				{"log", NewLog(env).Win()},
				{"market", NewMarket(env).Win()},
				{"inventory", NewInventory(env).Win()},
				{"fitting", NewFitting(env).Win()},
				{"manufacturing", NewManufacturing(env).Win()},
				{"refinery", NewRefinery(env).Win()},
				{"scanner", NewScanner(env).Win()},
				{"starmap", NewMapView(env).Win()},
				{"help", NewHelp(env, keyMapRows(8)).Win()},
				{"settings", NewSettings(env, staticSettingsActions()).Win()},
			}
			floor := theme.Px(RailInset)
			for _, c := range cases {
				r := c.win.GetContainer().GetWidget().Rect
				if r.Dx() <= 0 || r.Dy() <= 0 {
					t.Errorf("%s: empty default rect %v", c.name, r)
					continue
				}
				if r.Min.X < floor {
					t.Errorf("%s default rect = %v, left edge %d starts inside the rail gutter (%d = RailInset through Px)",
						c.name, r, r.Min.X, floor)
				}
			}
		})
	}
}
