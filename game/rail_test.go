package game

import (
	"image"
	"slices"
	"testing"

	enuiinput "github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/icons"
	uikit "github.com/entieve/entieve/uikit/screens"
	"github.com/hajimehoshi/ebiten/v2"
)

// The navigation rail as the game wires it: the entries table derived
// from the Screen spec, the toggle semantics through the real frame
// pipeline, the grayed entries' silence, the one window docking opens
// and the composition that places what the player opens while docked.

// TestRailDerivesFromScreenSpec: every non-transient, listed spec row is
// exactly one rail entry — its icon, live hint and group — in section
// order, the two always-on windows included (they are rows now, and they
// still lead the Ship section); the Esc menu, the HUD and the rack have no
// entries (the pause menu keeps its key, the ship chrome has no window to
// open), the two secondary windows (Settings, Keybindings) are not
// launchers, and every entry names a manifest icon.
func TestRailDerivesFromScreenSpec(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)
	entries := g.rail.Entries()

	// One entry per openable, listed spec row (the always-on windows
	// included: they are rows).
	wantRows := 0
	for _, s := range screens {
		if s.DockPolicy != DockTransient && !s.NoRail && !s.Unlisted {
			wantRows++
		}
	}
	if len(entries) != wantRows {
		t.Fatalf("rail has %d entries, want %d listed spec rows", len(entries), wantRows)
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
		if s.NoRail || s.Unlisted {
			if byTitle[s.Title] != 0 {
				t.Errorf("secondary %q is a rail launcher (it is opened from the Settings window)", s.Title)
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
		if e.Hint == nil {
			t.Errorf("%s: entry has no live hint closure", s.Title)
		} else if got, want := e.Hint(), g.hints.Hint(s.Action); got != want {
			t.Errorf("%s: entry hint = %q, want the live %q", s.Title, got, want)
		}
		if e.Group != s.Group {
			t.Errorf("%s: entry group = %q, spec says %q", s.Title, e.Group, s.Group)
		}
		if e.Open == nil {
			t.Errorf("%s: entry has no open-state closure", s.Title)
		}
	}
	// Rail order, pinned: the always-on windows still lead the Ship section
	// (they are the table's first rows, so their entries derive first).
	if entries[0].Title != "Overview" || entries[1].Title != "Selected Item" || entries[2].Title != "Fitting" {
		t.Errorf("Ship section = %q/%q/%q, want Overview, Selected Item, Fitting (unchanged)",
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

// TestRailHasNoSettingsOrKeybindingsEntries: the Settings and Keybindings
// windows left the navigation (they are opened from the Settings window's
// Keybindings button and from Settings' own key / title-menu row). The
// windows themselves are unchanged: both rows stay in the host, Keybindings
// keeps its sidecar rect, and the rail's Help & System section now holds
// exactly Help.
func TestRailHasNoSettingsOrKeybindingsEntries(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)

	for _, title := range []string{"Settings", "Keybindings"} {
		if g.rail.Button(title) != nil {
			t.Errorf("%q still has a rail entry", title)
		}
	}
	var helpSystem []string
	for _, e := range g.rail.Entries() {
		if e.Group == "Help & System" {
			helpSystem = append(helpSystem, e.Title)
		}
	}
	if !slices.Equal(helpSystem, []string{"Help"}) {
		t.Errorf("Help & System = %v, want just Help", helpSystem)
	}

	// B1: only the listings went. Both rows still mount through the host
	// and Keybindings still persists its rect.
	for _, id := range []ScreenID{idSettings, idKeybindings} {
		if g.host.Window(id) == nil {
			t.Errorf("%s: the window is gone, only its listings should be", id)
		}
	}
	if !slices.Contains(sidecarNames(screens), string(idKeybindings)) {
		t.Error("keybindings left the layout sidecar")
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
	if g.host.IsOpen(idMarket) || g.host.Mounted(idMarket) {
		t.Fatal("fixture: market already open in space")
	}

	click(t, g, g.rail.Button("Market"))
	if !g.host.IsOpen(idMarket) {
		t.Fatal("rail click did not open the market")
	}
	g.syncMounts()
	if !g.host.Mounted(idMarket) {
		t.Fatal("opened market did not mount (a mount is the raise to the top)")
	}
	// The entry's open-state closure tracks the window (the accent glow
	// reads this same flag; the glow itself is pinned in
	// uikit/screens/rail_test.go).
	if !entryByTitle(t, g, "Market").Open() {
		t.Error("entry's open state does not follow the mounted window")
	}

	click(t, g, g.rail.Button("Market"))
	if g.host.IsOpen(idMarket) {
		t.Fatal("second rail click did not close the market")
	}
	g.syncMounts()
	if g.host.Mounted(idMarket) {
		t.Fatal("closed market stayed mounted")
	}

	// The two always-on windows toggle through their Screen rows like every
	// other entry: the rail HIDES the overview (it stays mounted) and shows
	// it again.
	ov := g.kit.OverviewWindow().GetContainer().GetWidget()
	if !ov.IsVisible() {
		t.Fatal("fixture: overview hidden in space")
	}
	click(t, g, g.rail.Button("Overview"))
	g.syncMounts()
	if g.host.IsOpen(idOverview) || ov.IsVisible() {
		t.Error("rail did not close the overview")
	}
	if !g.host.Mounted(idOverview) {
		t.Error("closing the overview unmounted it: a Persistent row hides, it does not unmount")
	}
	if entryByTitle(t, g, "Overview").Open() {
		t.Error("overview entry still glows after closing")
	}
	click(t, g, g.rail.Button("Overview"))
	g.syncMounts()
	if !g.host.IsOpen(idOverview) || !ov.IsVisible() {
		t.Error("rail did not reopen the overview")
	}

	// The starmap's entry still opens the layer row; once it is up the
	// layer covers the rail, so Esc (or its key, or its own ×) is the way
	// back: the rail is a door only while the layer is closed.
	click(t, g, g.rail.Button("Starmap"))
	if !g.host.IsOpen(idMap) {
		t.Fatal("rail click did not open the starmap")
	}
	g.onEscape()
	g.syncMounts()
	if g.host.IsOpen(idMap) {
		t.Error("Esc did not close the starmap opened from the rail")
	}
}

// TestRailGrayedClicksDoNothing: an entry unavailable in this context is
// grayed and a click on it emits NOTHING — the DockedOnly services view
// in space, and the SpaceOnly scanner while docked.
func TestRailGrayedClicksDoNothing(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)

	if g.host.IsOpen(idServices) {
		t.Fatal("fixture: services view open in space")
	}
	click(t, g, g.rail.Button("Contacts"))
	if g.host.IsOpen(idServices) {
		t.Error("grayed services entry toggled its window in space")
	}

	g = dockedShowcaseGame(t, 1280, 720)
	if g.host.IsOpen(idScanner) {
		t.Fatal("fixture: scanner open while docked")
	}
	click(t, g, g.rail.Button("Scanner"))
	if g.host.IsOpen(idScanner) {
		t.Error("grayed scanner entry toggled its window while docked")
	}
	if g.rail.Button("Contacts").GetWidget().Disabled {
		t.Error("services entry grayed while docked")
	}
	// The always-on windows are space chrome too: their entries gray
	// while docked (and the station-dependent fitting grays at a
	// station without the fitting service — the start station has it).
	if !g.rail.Button("Overview").GetWidget().Disabled {
		t.Error("always-on window entry not grayed while docked")
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

	before := g.host.OpenFlags()
	dockAt(t, g)
	pump(t, g, 1280, 720)

	if !g.host.IsOpen(idServices) || !g.host.Mounted(idServices) {
		t.Fatal("services view did not open on dock")
	}
	for _, p := range g.host.IDs() {
		if p == idServices {
			continue
		}
		if g.host.IsOpen(p) != before[p] {
			t.Errorf("docking flipped %s (%v → %v) — only the services view may open",
				panelNames[p], before[p], g.host.IsOpen(p))
		}
		if g.host.Mounted(p) && !before[p] && p != idServices {
			t.Errorf("docking mounted %s without the player opening it", panelNames[p])
		}
	}

	g.s.Docked.Remove(g.s.Player())
	pump(t, g, 1280, 720)
	if g.host.IsOpen(idServices) || g.host.Mounted(idServices) {
		t.Error("undocking did not close the services view")
	}
	for _, p := range g.host.IDs() {
		if g.host.IsOpen(p) != before[p] {
			t.Errorf("undocking left %s at %v, want its pre-dock state %v", panelNames[p], g.host.IsOpen(p), before[p])
		}
	}
}

// TestRailPlacesOpenedWindowsWhileDocked: docking no longer composes a
// band — the rail opens each service window at its own per-type default,
// and the services view stays the station column.
func TestRailPlacesOpenedWindowsWhileDocked(t *testing.T) {
	g := dockedShowcaseGame(t, 1280, 720)

	if r := g.host.Window(idServices).GetContainer().GetWidget().Rect; r != stationRect(r.Dy()) {
		t.Errorf("services view at %v, want the station column %v", r, stationRect(r.Dy()))
	}

	wins := map[string]*widget.Window{
		"Market": bodyOf[*uikit.Market](g.host, idMarket).Win(), "Inventory": bodyOf[*uikit.Inventory](g.host, idInventory).Win(), "Fitting": bodyOf[*uikit.Fitting](g.host, idFitting).Win(),
	}
	defaults := map[string]image.Rectangle{
		"Market": serviceDefaults[idMarket], "Inventory": serviceDefaults[idInventory],
		"Fitting": serviceDefaults[idFitting],
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

// TestServicesViewOnTheRealStations: the contacts strip carries the docked
// station's face and Undock — no window buttons at all, at any station's
// service set (the rail lists every window; the strip is not a second
// index). Undock queues the sim's undock command.
func TestServicesViewOnTheRealStations(t *testing.T) {
	g := dockedShowcaseGame(t, 1280, 720)
	svc := stationBoard(t, g).Services()
	if !svc.Visible("undock") {
		t.Error("Undock hidden while docked")
	}
	if svc.Identity() == "" {
		t.Error("identity line empty while docked")
	}
	for _, svc := range content.Services {
		if b := stationBoard(t, g).Services().Button(svc.ID); b != nil {
			t.Errorf("%s: a window button is back on the strip — the rail owns that row", svc.ID)
		}
	}
	if b := stationBoard(t, g).Services().Button("cargo"); b != nil {
		t.Error("a Cargo button is back on the strip — Inventory is a rail row")
	}

	// Undock queues exactly the sim command.
	var got []sim.Command
	g.enqueue = func(c sim.Command) { got = append(got, c) }
	click(t, g, stationBoard(t, g).Services().Button("undock"))
	if len(got) != 1 {
		t.Fatalf("Undock queued %#v, want the single CmdUndock", got)
	}
	if _, ok := got[0].(sim.CmdUndock); !ok {
		t.Errorf("cmd = %#v, want CmdUndock", got[0])
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
	mm := useLoopMouse(t)
	g := newShowcaseGame(t, mm.pointer())
	g.applySize(1280, 720)
	img := ebiten.NewImage(1280, 720)

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
	if g.host.IsOpen(idInventory) {
		t.Fatal("fixture: inventory already open")
	}
	before := g.kit.Selected()

	// Hover one frame, then press and release inside the cell.
	mm.moveTo(cell.Min.X+cell.Dx()/2, cell.Min.Y+cell.Dy()/2)
	frame()
	mm.left = true
	frame()
	mm.left = false
	frame()

	g.syncMounts()
	if !g.host.IsOpen(idInventory) || !g.host.Mounted(idInventory) {
		t.Errorf("rail cell under a window did not toggle the inventory (open=%v mounted=%v) — "+
			"the window underneath won the click", g.host.IsOpen(idInventory), g.host.Mounted(idInventory))
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
