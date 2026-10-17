package screens

import (
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
		{Icon: "kind_ship", Title: "Overview", Key: "", Desc: "", Group: "Ship", Open: func() bool { return *open }},
		{Icon: "svc_fitting", Title: "Fitting", Key: "F", Desc: "F — toggle fitting", Group: "Ship"},
		{Icon: "svc_market", Title: "Market", Key: "G", Desc: "G — toggle market", Group: "Trade", Service: "market"},
		{Icon: "svc_scanner", Title: "Scanner", Key: "C", Desc: "C — toggle scanner", Group: "World", SpaceOnly: true},
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
				if br.Dx() != theme.Px(RailInset) || br.Dy() != theme.Px(RailInset) {
					t.Errorf("%s cell = %v, want a %d² icon cell", title, br, theme.Px(RailInset))
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
	env := widgets.NewSeams(th, cv, sink)
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

// TestRailTooltips: the hover model is icon + title + the host's hint,
// swapped for the "unavailable" line when the entry is grayed here.
func TestRailTooltips(t *testing.T) {
	e := RailEntry{Icon: "svc_market", Title: "Market", Desc: "G — toggle market"}
	m := entryTip(e, true)
	if m.Icon != "svc_market" || m.Title != "Market" || len(m.Hints) != 1 || m.Hints[0] != e.Desc {
		t.Errorf("tip = %+v, want the icon, title and %q", m, e.Desc)
	}
	m = entryTip(e, false)
	if len(m.Hints) != 1 || m.Hints[0] != "Unavailable here" {
		t.Errorf("grayed tip = %+v, want the unavailable line", m)
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
