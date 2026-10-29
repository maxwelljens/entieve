package game

import (
	"image"
	"path/filepath"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// rightClick fires a widget-level right-button click (with the widget, so
// the menu anchors where the cursor was) and flushes the deferred queue.
func rightClick(t *testing.T, g *Game, w *widget.Widget) {
	t.Helper()
	w.MouseButtonClickedEvent.Fire(&widget.WidgetMouseButtonClickedEventArgs{
		Widget:  w,
		Button:  ebiten.MouseButtonRight,
		OffsetX: 9,
		OffsetY: 5,
	})
	if err := g.kit.Update(); err != nil {
		t.Fatalf("kit.Update: %v", err)
	}
}

// menuEntry returns the open menu's entry with the label (fails when the
// menu is down or the entry is missing).
func menuEntry(t *testing.T, g *Game, label string) widgets.MenuEntry {
	t.Helper()
	m := g.kit.Menu()
	if !m.IsOpen() {
		t.Fatalf("no menu open (want %q)", label)
	}
	for _, e := range m.Entries() {
		if e.Label == label {
			return e
		}
	}
	t.Fatalf("menu has no %q entry: %+v", label, m.Entries())
	return widgets.MenuEntry{}
}

// pressMenuEntry picks the labelled entry in the open menu (fails when
// it is disabled).
func pressMenuEntry(t *testing.T, g *Game, label string) {
	t.Helper()
	m := g.kit.Menu()
	if e := menuEntry(t, g, label); e.Disabled {
		t.Fatalf("menu entry %q is disabled", label)
	}
	for i, e := range m.Entries() {
		if e.Label == label {
			m.Click(m.RowRect(i).Min.Add(image.Pt(1, 1)))
			return
		}
	}
}

// TestContractRowContextMenu: a board row's right-click opens the row
// menu — Accept on an available row, Abandon on an active one — through
// the same command and gating as the buttons; a standing-gated row
// carries a disabled entry that enqueues nothing, note rows carry no
// menu at all, and the objective label opens its own Show Info menu.
func TestContractRowContextMenu(t *testing.T) {
	g := dockedShowcaseGame(t, 1280, 720)
	var got []sim.Command
	g.enqueue = func(c sim.Command) { got = append(got, c) }

	// Ungated available row: one enabled Accept entry.
	row := findRow(t, stationBoard(t, g).Rows(false), "contract-solace-cordite-1")
	rightClick(t, g, row.Root.GetWidget())
	if e := menuEntry(t, g, "Accept"); e.Disabled {
		t.Fatal("ungated Accept entry is disabled")
	}
	if len(g.kit.Menu().Entries()) != 1 {
		t.Errorf("row menu = %+v, want the single Accept entry", g.kit.Menu().Entries())
	}
	pressMenuEntry(t, g, "Accept")
	if len(got) != 1 {
		t.Fatalf("menu Accept enqueued %d commands, want 1", len(got))
	}
	if c, ok := got[0].(sim.CmdAcceptMission); !ok || c.ID != "contract-solace-cordite-1" {
		t.Fatalf("cmd = %#v, want CmdAcceptMission{contract-solace-cordite-1}", got[0])
	}

	// A standing-gated row: the entry is visible but disabled, and
	// pressing it (the widget-level click lands anyway) enqueues nothing.
	g.s.Docked.Set(g.s.Player(), sim.Docked{At: "cel-penumbra-port"})
	g.syncScenes()
	g.syncMounts()
	pump(t, g, 1280, 720)
	got = nil
	gated := findRow(t, stationBoard(t, g).Rows(false), "contract-penumbra-den-hunt-1")
	rightClick(t, g, gated.Root.GetWidget())
	if e := menuEntry(t, g, "Accept"); !e.Disabled {
		t.Fatal("gated Accept entry is enabled")
	}
	m := g.kit.Menu()
	for i, e := range m.Entries() {
		if e.Label == "Accept" {
			m.Click(m.RowRect(i).Min.Add(image.Pt(1, 1)))
		}
	}
	if len(got) != 0 {
		t.Fatalf("gated menu Accept enqueued %v, want nothing", got)
	}

	// Back at the offering station: an active row offers Abandon.
	g.s.Docked.Set(g.s.Player(), sim.Docked{At: "cel-solace"})
	g.syncScenes()
	g.syncMounts()
	setMissions(t, g, sim.MissionState{ID: "contract-solace-cordite-1"})
	g.syncMounts()
	pump(t, g, 1280, 720)
	active := findRow(t, stationBoard(t, g).Rows(true), "contract-solace-cordite-1")
	rightClick(t, g, active.Root.GetWidget())
	if e := menuEntry(t, g, "Abandon"); e.Disabled {
		t.Fatal("Abandon entry is disabled on an active row")
	}
	pressMenuEntry(t, g, "Abandon")
	if len(got) != 1 {
		t.Fatalf("menu Abandon enqueued %d commands, want 1", len(got))
	}
	if c, ok := got[0].(sim.CmdAbandonMission); !ok || c.ID != "contract-solace-cordite-1" {
		t.Fatalf("cmd = %#v, want CmdAbandonMission", got[0])
	}
}

// TestContractObjectiveShowInfo: the objective label (line 2 of an
// available row) is its own gesture target — right-click opens the
// Show Info menu for the objective's def, double-click swaps the window —
// and its menu replaces the row's Accept menu when the click lands there.
func TestContractObjectiveShowInfo(t *testing.T) {
	g := dockedShowcaseGame(t, 1280, 720)
	var got []sim.Command
	g.enqueue = func(c sim.Command) { got = append(got, c) }

	row := findRow(t, stationBoard(t, g).Rows(false), "contract-solace-cordite-1")
	if len(row.Root.Children()) < 2 {
		t.Fatalf("available row has %d lines, want title/summary/reward", len(row.Root.Children()))
	}
	objective := row.Root.Children()[1].GetWidget() // line 2: the summary
	def, ok := g.opts.Content.Contract("contract-solace-cordite-1")
	if !ok {
		t.Fatal("fixture: contract missing from content")
	}

	rightClick(t, g, objective)
	entries := g.kit.Menu().Entries()
	if len(entries) != 1 || entries[0].Label != "Show Info" || !entries[0].Bold {
		t.Fatalf("objective menu = %+v, want the single Show Info entry", entries)
	}
	pressMenuEntry(t, g, "Show Info")
	if !g.kit.InfoOpen() {
		t.Fatal("objective Show Info did not open the window")
	}
	switch def.Kind {
	case content.ContractKill:
		if got, _ := g.env.InfoWindow().Ref(); got != widgets.NpcRef(def.Objective.Target) {
			t.Errorf("info ref = %v, want the objective caste %s", got, def.Objective.Target)
		}
	default:
		if got, _ := g.env.InfoWindow().Ref(); got != widgets.ItemRef(def.Objective.Item) {
			t.Errorf("info ref = %v, want the objective item %s", got, def.Objective.Item)
		}
	}
	// The row's Accept never fired: the child's menu replaced it.
	if len(got) != 0 {
		t.Fatalf("objective right-click enqueued %v, want nothing", got)
	}

	// Double-click swaps the window the same way.
	g.kit.CloseInfo()
	objective.MouseButtonClickedEvent.Fire(&widget.WidgetMouseButtonClickedEventArgs{
		Widget: objective, Button: ebiten.MouseButtonLeft,
	})
	if err := g.kit.Update(); err != nil {
		t.Fatalf("kit.Update: %v", err)
	}
	objective.MouseButtonClickedEvent.Fire(&widget.WidgetMouseButtonClickedEventArgs{
		Widget: objective, Button: ebiten.MouseButtonLeft,
	})
	if err := g.kit.Update(); err != nil {
		t.Fatalf("kit.Update: %v", err)
	}
	if !g.kit.InfoOpen() {
		t.Fatal("double-click on the objective label did not open Show Info")
	}
}

// TestInfoWindowSidecarGeometryOnly: the "infowin" entry persists the
// window's rect across a save/restart cycle and never carries (or
// restores) an open state — the window appears on demand only.
func TestInfoWindowSidecarGeometryOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	g := newShowcaseGameAt(t, path)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)

	// The player's arrangement: a moved, resized, shown window.
	want := image.Rect(500, 180, 940, 680)
	g.kit.InfoWindow().SetLocation(want)
	g.kit.ShowInfo(widgets.ModuleRef("mod-autocannon-s"))
	pump(t, g, 1280, 720)
	if !g.kit.InfoOpen() {
		t.Fatal("fixture: window did not open")
	}
	if err := g.SaveGame(); err != nil {
		t.Fatalf("SaveGame: %v", err)
	}

	doc := readDoc(t, g.layoutPath())
	e, ok := doc.Windows["infowin"]
	if !ok {
		t.Fatal("infowin missing from the sidecar")
	}
	if e.X != want.Min.X || e.Y != want.Min.Y || e.W != want.Dx() || e.H != want.Dy() {
		t.Errorf("infowin sidecar = %+v, want %+v", e, want)
	}
	if e.Open {
		t.Error("infowin persisted open — it is geometry-only and always writes closed")
	}

	// Restart on the same path: the rect comes back, the window does not.
	g2 := newShowcaseGameAt(t, path)
	g2.applySize(1280, 720)
	pump(t, g2, 1280, 720)
	if g2.kit.InfoOpen() {
		t.Fatal("infowin restored open (it must appear on demand)")
	}
	if got := g2.kit.InfoWindow().GetContainer().GetWidget().Rect; got != want {
		t.Errorf("restored rect = %v, want %v", got, want)
	}
	// The sidecar still says closed on the next write, whatever the
	// session did with the window.
	if err := g2.SaveGame(); err != nil {
		t.Fatalf("SaveGame: %v", err)
	}
	if again := readDoc(t, g2.layoutPath()).Windows["infowin"]; again.Open {
		t.Error("second write carried open=true")
	}
}

// TestContextMenuGatesWorldInput: while a menu is up its popup rect
// deterministically gates the world (the rect uikit exposes), and the
// frame latch gates everything else for the frame a dismiss gesture is
// spent on — including swallowing the Esc that would otherwise push the
// menu scene behind the popup.
func TestContextMenuGatesWorldInput(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)

	// Off-panel point (clear space above the bottom-center pair, and clear
	// of the popup this test opens at (600,300)).
	const cx, cy = 640.0, 200.0
	if g.overUI(cx, cy) {
		t.Fatalf("fixture: overUI(%v, %v) already true", cx, cy)
	}

	m := g.kit.Menu()
	m.Open([]widgets.MenuEntry{{Label: "Show Info", Bold: true}}, image.Pt(600, 300))
	r := m.Rect()
	if r.Empty() {
		t.Fatal("open menu exposes no rect")
	}
	// Before the frame's poll, only the popup's own rect gates (the rect
	// half of the gate).
	if !g.overUI(float64(r.Min.X+2), float64(r.Min.Y+2)) {
		t.Error("a point on the popup reaches the world")
	}
	if g.overUI(cx, cy) {
		t.Error("the rect gate covers points off the popup")
	}

	// The frame latch: polling with the popup up gates everywhere (the
	// dismiss gesture's frame, and every frame the popup holds).
	if !g.kit.MenuInput() {
		t.Fatal("the open popup did not take the frame")
	}
	if !g.kit.Held() {
		t.Error("the popup's poll did not latch the frame")
	}
	if !g.overUI(cx, cy) {
		t.Error("the frame latch does not gate off-popup points")
	}

	// Esc: consumed while the menu owns the frame, plain after it.
	if err := g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyEscape}}); err != nil {
		t.Fatalf("handleKeys: %v", err)
	}
	if _, ok := g.top().(*MenuScene); ok {
		t.Error("Esc behind a context menu pushed the menu scene")
	}
	m.Close()
	g.kit.MenuInput() // the popup is gone: the next frame's poll drops the latch
	if g.kit.Held() {
		t.Error("a closed popup still holds the frame")
	}
	if err := g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyEscape}}); err != nil {
		t.Fatalf("handleKeys: %v", err)
	}
	if _, ok := g.top().(*MenuScene); !ok {
		t.Error("plain Esc did not push the menu scene")
	}
}
