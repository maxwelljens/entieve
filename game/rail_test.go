package game

import (
	"image"
	"strings"
	"testing"

	enuiinput "github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/icons"
	uikit "github.com/entieve/entieve/uikit/screens"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// The navigation rail as the game wires it: the entries table derived
// from the Screen spec, the toggle semantics through the real frame
// pipeline, the grayed entries' silence, the one window docking opens
// and the composition that places what the player opens while docked.

// TestRailDerivesFromScreenSpec: every non-transient spec row is
// exactly one rail entry — its icon, display key and group — in section
// order, next to the two core windows; the Esc menu, the HUD and the
// rack have no entries (the pause menu keeps its key, the ship chrome
// has no window to open), and every entry names a manifest icon.
func TestRailDerivesFromScreenSpec(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)
	entries := g.rail.Entries()

	// One entry per openable spec row + the two core windows.
	wantRows := 0
	for _, s := range screens {
		if s.DockPolicy != DockTransient {
			wantRows++
		}
	}
	if len(entries) != wantRows+2 {
		t.Fatalf("rail has %d entries, want %d spec rows + 2 core windows", len(entries), wantRows+2)
	}

	byTitle := map[string]int{}
	for _, e := range entries {
		byTitle[e.Title]++
		if !icons.Has(e.Icon) {
			t.Errorf("%s: icon %q not in the manifest", e.Title, e.Icon)
		}
	}
	for _, s := range screens {
		if s.DockPolicy == DockTransient {
			if byTitle[s.Title] != 0 {
				t.Errorf("transient %q is in the rail (the Esc menu is not a launcher)", s.Title)
			}
			continue
		}
		if byTitle[s.Title] != 1 {
			t.Fatalf("%s appears %d times in the rail, want exactly 1", s.Title, byTitle[s.Title])
		}
		var e uikit.RailEntry
		for _, cand := range entries {
			if cand.Title == s.Title {
				e = cand
			}
		}
		if e.Icon != s.Icon {
			t.Errorf("%s: entry icon = %q, spec says %q", s.Title, e.Icon, s.Icon)
		}
		if e.Key != keyLabel(s) {
			t.Errorf("%s: entry key = %q, want %q", s.Title, e.Key, keyLabel(s))
		}
		if e.Group != s.Group {
			t.Errorf("%s: entry group = %q, spec says %q", s.Title, e.Group, s.Group)
		}
		if e.Open == nil {
			t.Errorf("%s: entry has no open-state closure", s.Title)
		}
	}
	// The core windows: Ship section, host-owned open state.
	for _, title := range []string{"Overview", "Selected Item"} {
		if byTitle[title] != 1 {
			t.Fatalf("core window %q appears %d times, want 1", title, byTitle[title])
		}
	}
	if entries[0].Title != "Overview" || entries[1].Title != "Selected Item" || entries[2].Title != "Fitting" {
		t.Errorf("Ship section = %q/%q/%q, want Overview, Selected Item, Fitting (core first)",
			entries[0].Title, entries[1].Title, entries[2].Title)
	}

	// Section order: entries run by railGroups, spec order inside one.
	groupOf := map[string]int{}
	for i, e := range entries {
		gi, ok := groupOf[e.Group]
		if !ok {
			for j, name := range railGroups {
				if name == e.Group {
					gi, ok = j+1, true
				}
			}
			if !ok {
				t.Fatalf("%s: group %q is not one of %v", e.Title, e.Group, railGroups)
			}
			groupOf[e.Group] = gi
		}
		if i > 0 && gi < groupOf[entries[i-1].Group] {
			t.Errorf("%s: group %q appears after %q — entries must run in railGroups order",
				e.Title, e.Group, entries[i-1].Group)
		}
	}
	// The permanent chrome and the pause menu never enter the rail.
	for _, title := range []string{"HUD", "Modules", "Menu"} {
		if byTitle[title] != 0 {
			t.Errorf("%q has a rail entry (permanent chrome / pause menu)", title)
		}
	}
}

// TestRailClicksToggleWindows: a click on an available entry opens the
// window (a fresh mount lands on top of the stack) and the second click
// closes it — through the real pipeline, from the button's widget event
// to Game.Do to setPanel.
func TestRailClicksToggleWindows(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)
	if g.open[pMarket] || g.mounted[pMarket] != nil {
		t.Fatal("fixture: market already open in space")
	}

	click(t, g, g.rail.Button("Market"))
	if !g.open[pMarket] {
		t.Fatal("rail click did not open the market")
	}
	g.syncMounts()
	if g.mounted[pMarket] == nil {
		t.Fatal("opened market did not mount (a mount is the raise to the top)")
	}
	// The entry's open-state closure tracks the window (the accent glow
	// reads this same flag; the glow itself is pinned in
	// uikit/screens/rail_test.go).
	if !entryByTitle(t, g, "Market").Open() {
		t.Error("entry's open state does not follow the mounted window")
	}

	click(t, g, g.rail.Button("Market"))
	if g.open[pMarket] {
		t.Fatal("second rail click did not close the market")
	}
	g.syncMounts()
	if g.mounted[pMarket] != nil {
		t.Fatal("closed market stayed mounted")
	}

	// The core windows toggle the same way through Kit.ToggleCore: the
	// rail closes the always-on overview and brings it back.
	ov := g.kit.OverviewWindow().GetContainer().GetWidget()
	if !ov.IsVisible() {
		t.Fatal("fixture: overview hidden in space")
	}
	click(t, g, g.rail.Button("Overview"))
	g.syncMounts()
	if g.kit.CoreOpen("Overview") || ov.IsVisible() {
		t.Error("rail did not close the overview")
	}
	if entryByTitle(t, g, "Overview").Open() {
		t.Error("overview entry still glows after closing")
	}
	click(t, g, g.rail.Button("Overview"))
	g.syncMounts()
	if !g.kit.CoreOpen("Overview") || !ov.IsVisible() {
		t.Error("rail did not reopen the overview")
	}
}

// TestRailGrayedClicksDoNothing: an entry unavailable in this context is
// grayed and a click on it emits NOTHING — the DockedOnly services view
// in space, and the SpaceOnly scanner while docked.
func TestRailGrayedClicksDoNothing(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)

	if g.open[pServices] {
		t.Fatal("fixture: services view open in space")
	}
	click(t, g, g.rail.Button("Station Services"))
	if g.open[pServices] {
		t.Error("grayed services entry toggled its window in space")
	}

	g = dockedShowcaseGame(t, 1280, 720)
	if g.open[pScanner] {
		t.Fatal("fixture: scanner open while docked")
	}
	click(t, g, g.rail.Button("Scanner"))
	if g.open[pScanner] {
		t.Error("grayed scanner entry toggled its window while docked")
	}
	if g.rail.Button("Station Services").GetWidget().Disabled {
		t.Error("services entry grayed while docked")
	}
	// The core windows are space chrome too: their entries gray with
	// them while docked (and the station-dependent fitting grays at a
	// station without the fitting service — the start station has it).
	if !g.rail.Button("Overview").GetWidget().Disabled {
		t.Error("core window entry not grayed while docked")
	}
	if g.rail.Button("Fitting").GetWidget().Disabled {
		t.Error("fitting entry grayed at a station offering the service")
	}
}

// TestDockOpensExactlyTheServicesView: the docking behaviour change in
// one pin — the scene opens exactly the services view and NOTHING else
// (no market, no fitting, no inventory, no factory), and undocking
// closes it again, leaving every other flag at its pre-dock state.
func TestDockOpensExactlyTheServicesView(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)

	before := g.open
	dockAt(t, g)
	pump(t, g, 1280, 720)

	if !g.open[pServices] || g.mounted[pServices] == nil {
		t.Fatal("services view did not open on dock")
	}
	for p := panelID(0); p < numPanels; p++ {
		if p == pServices {
			continue
		}
		if g.open[p] != before[p] {
			t.Errorf("docking flipped %s (%v → %v) — only the services view may open",
				panelNames[p], before[p], g.open[p])
		}
		if g.mounted[p] != nil && !before[p] && p != pServices {
			t.Errorf("docking mounted %s without the player opening it", panelNames[p])
		}
	}

	g.s.Docked.Remove(g.s.Player())
	pump(t, g, 1280, 720)
	if g.open[pServices] || g.mounted[pServices] != nil {
		t.Error("undocking did not close the services view")
	}
	for p := panelID(0); p < numPanels; p++ {
		if g.open[p] != before[p] {
			t.Errorf("undocking left %s at %v, want its pre-dock state %v", panelNames[p], g.open[p], before[p])
		}
	}
}

// TestRailPlacesOpenedWindowsWhileDocked: docking no longer composes a
// band — the rail opens each service window at its own per-type default,
// and the services view stays the station column.
func TestRailPlacesOpenedWindowsWhileDocked(t *testing.T) {
	g := dockedShowcaseGame(t, 1280, 720)

	if r := g.svcWin.GetContainer().GetWidget().Rect; r != stationRect(r.Dy()) {
		t.Errorf("services view at %v, want the station column %v", r, stationRect(r.Dy()))
	}

	wins := map[string]*widget.Window{
		"Market": g.mkt.Win(), "Inventory": g.inv.Win(), "Fitting": g.fitt.Win(),
	}
	defaults := map[string]image.Rectangle{
		"Market": serviceDefaults[pMarket], "Inventory": serviceDefaults[pInventory],
		"Fitting": serviceDefaults[pFitting],
	}
	for _, title := range []string{"Market", "Inventory", "Fitting"} {
		click(t, g, g.rail.Button(title))
		g.syncMounts()
		pump(t, g, 1280, 720)
		if got := wins[title].GetContainer().GetWidget().Rect; got != defaults[title] {
			t.Errorf("%s opened at %v, want its per-type default %v", title, got, defaults[title])
		}
		click(t, g, g.rail.Button(title))
		g.syncMounts()
	}
}

// TestServicesViewButtonsOnTheRealStations: at the start station
// (Jernborg: repair, market, refine, fitting, industry) the view offers
// market, fitting and industry; a repair-only service set hides them. Cargo toggles the
// inventory window, every service button's Toggle names a Screen spec
// row the host resolves, and Undock queues the sim's undock command.
func TestServicesViewButtonsOnTheRealStations(t *testing.T) {
	g := dockedShowcaseGame(t, 1280, 720)
	if !g.svcUI.Visible("market") || !g.svcUI.Visible("fitting") || !g.svcUI.Visible("industry") {
		t.Fatal("start station's market/fitting/industry buttons hidden")
	}
	if !g.svcUI.Visible("undock") || !g.svcUI.Visible("cargo") {
		t.Error("Undock/Cargo are not station services — they must always show")
	}
	if g.svcUI.Identity() == "" {
		t.Error("identity line empty while docked")
	}

	// Cargo toggles the inventory window.
	click(t, g, g.svcUI.Button("cargo"))
	g.syncMounts()
	if !g.open[pInventory] || g.mounted[pInventory] == nil {
		t.Error("Cargo did not toggle the inventory window")
	}
	click(t, g, g.svcUI.Button("cargo"))
	g.syncMounts()

	// Every service button toggles a real spec row (Toggle carries the
	// spec's own Title, so screenByTitle resolves it).
	for _, title := range []string{"Market", "Fitting", "Manufacturing", "Inventory"} {
		if _, ok := screenByTitle[strings.ToLower(title)]; !ok {
			t.Errorf("spec lost the row %q the services view toggles", title)
		}
	}

	// Undock queues exactly the sim command (the path the strip's
	// Undock button used).
	var got []sim.Command
	g.enqueue = func(c sim.Command) { got = append(got, c) }
	click(t, g, g.svcUI.Button("undock"))
	if len(got) != 1 {
		t.Fatalf("Undock queued %#v, want the single CmdUndock", got)
	}
	if _, ok := got[0].(sim.CmdUndock); !ok {
		t.Errorf("cmd = %#v, want CmdUndock", got[0])
	}

	// A second station's service set: the sim copies a station's
	// services into its Station component at build (sim/build.go), so
	// the second set goes on the docked station's component — a
	// repair-only outpost. The view follows the list: the trade and
	// industry buttons disappear.
	g.s.Stations.Set(dockedStationEntity(t, g, "cel-haven-rest"),
		sim.Station{Services: []string{"repair"}})
	pump(t, g, 1280, 720)
	if g.svcUI.Visible("industry") || g.svcUI.Visible("fitting") || g.svcUI.Visible("market") {
		t.Error("service button shown where the station lacks the service")
	}
}

// dockedStationEntity resolves a celestial id to its grid entity — the
// same cel→entity pair view.DockedStation reads (celEntity is sim-
// private, so tests walk the exported stores).
func dockedStationEntity(t *testing.T, g *Game, cel content.CelID) ecs.Entity {
	t.Helper()
	var ent ecs.Entity
	g.s.Celestials.Each(func(e ecs.Entity, c *sim.Celestial) {
		if c.Cel == cel {
			ent = e
		}
	})
	if ent.IsZero() {
		t.Fatalf("no grid entity for %s", cel)
	}
	return ent
}

// entryByTitle finds one rail entry by its title (the pin helper above).
func entryByTitle(t *testing.T, g *Game, title string) uikit.RailEntry {
	t.Helper()
	for _, e := range g.rail.Entries() {
		if e.Title == title {
			return e
		}
	}
	t.Fatalf("no rail entry titled %q", title)
	return uikit.RailEntry{}
}

// TestRailClicksWinOverAWindowUnderneath: input priority matches paint
// priority. Drag a window onto the rail's gutter so it sits under a rail
// cell, click that cell through the real frame loop — the rail wins: the
// entry toggles its window, and the buried window underneath receives
// nothing (its table never moves the selection). Seams: the scripted
// mouse feeds ebitenui's cursor cache for the widgets, the raw press
// edge and live cursor feed uikit's polls (the menu-loop test's split).
func TestRailClicksWinOverAWindowUnderneath(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	mm := useLoopMouse(t)
	img := ebiten.NewImage(1280, 720)

	// The kit's raw press-edge seam, armed for the click frame only.
	clickEdge := false
	defer widgets.SetPressEdgeForTest(func(b ebiten.MouseButton) bool {
		return b == ebiten.MouseButtonLeft && clickEdge
	})()
	defer widgets.SetPollCursorForTest(func() (int, int) { return mm.x, mm.y })()

	frame := func() {
		t.Helper()
		if err := g.Update(); err != nil {
			t.Fatalf("Update: %v", err)
		}
		g.Draw(img)
	}

	// Drag the overview onto the gutter: it now covers the rail column.
	g.kit.OverviewWindow().SetLocation(image.Rect(0, 8, 400, 400))
	pump(t, g, 1280, 720)

	cell := g.rail.Button("Inventory").GetWidget().Rect
	ovRect := g.kit.OverviewWindow().GetContainer().GetWidget().Rect
	if cell.Dx() <= 0 || !cell.In(ovRect) {
		t.Fatalf("fixture: Inventory cell %v not under the dragged overview %v", cell, ovRect)
	}
	if g.open[pInventory] {
		t.Fatal("fixture: inventory already open")
	}
	before := g.kit.Selected()

	// Hover one frame, then press and release inside the cell.
	mm.moveTo(cell.Min.X+cell.Dx()/2, cell.Min.Y+cell.Dy()/2)
	frame()
	clickEdge, mm.left = true, true
	frame()
	clickEdge, mm.left = false, false
	frame()

	g.syncMounts()
	if !g.open[pInventory] || g.mounted[pInventory] == nil {
		t.Errorf("rail cell under a window did not toggle the inventory (open=%v mounted=%v) — "+
			"the window underneath won the click", g.open[pInventory], g.mounted[pInventory] != nil)
	}
	if got := g.kit.Selected(); got != before {
		t.Errorf("the buried overview received the click (selection %v → %v) — "+
			"input priority does not match paint priority", before, got)
	}

	// Leave the process clean: ebitenui's hover backstop (input.UIHovered)
	// refreshes only in AfterDraw, and this test's last frame hovered a
	// rail cell — park the cursor on nothing and draw once, so a later
	// test that reads the backstop without drawing sees the flag it
	// expects (the settings overUI pin reads it exactly that way).
	mm.moveTo(0, 0)
	frame()
	if enuiinput.UIHovered {
		t.Error("hover backstop still latched after parking the cursor clear of every panel")
	}
}
