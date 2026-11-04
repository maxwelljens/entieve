package game

// The interface rebuild's carry-over contract, one subtest per caller:
// boot installs no scale and restores no layout, load restores the layout
// and recentres, a scale change round-trips the placement, reset is the
// scale change back to 1×. In every path the log and the host's open flags
// survive and the selection resets; only the declared caller resets the
// camera.

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
	uikit "github.com/entieve/entieve/uikit/screens"
	"github.com/entieve/entieve/uikit/theme"
)

// rebuildLogHas reports whether the live log carries want.
func rebuildLogHas(g *Game, want string) bool {
	for _, l := range bodyOf[*uikit.LogPanel](g.host, idLog).Lines() {
		if strings.Contains(l, want) {
			return true
		}
	}
	return false
}

// rebuildSeed parks state every rebuild must either carry or reset: a log
// line, an open window, a camera pan and a selection.
func rebuildSeed(t *testing.T, g *Game, line string) {
	t.Helper()
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)
	bodyOf[*uikit.LogPanel](g.host, idLog).PushLine(sim.SevInfo, line)
	g.host.SetOpen(idInventory, true)
	g.syncMounts()
	g.space.pan = core.V(120, -40)
	g.setSelected(ecs.Entity(42))
}

func TestRebuildCarryOverBoot(t *testing.T) {
	resetScale(t)
	path := filepath.Join(t.TempDir(), "save.json")

	// A saved layout from an earlier session.
	first := newShowcaseGameAt(t, path)
	first.applySize(1280, 720)
	pump(t, first, 1280, 720)
	first.kit.OverviewWindow().SetLocation(image.Rect(40, 60, 440, 360))
	if err := first.writeLayout(); err != nil {
		t.Fatalf("writeLayout: %v", err)
	}

	// Boot: the build happens, the log starts empty, the camera and the
	// selection start at zero, only the default windows are open, and the
	// sidecar is NOT restored yet (the first real size does that).
	g := newShowcaseGameAt(t, path)
	if g.kit == nil || g.host == nil {
		t.Fatal("boot built no interface")
	}
	if lines := bodyOf[*uikit.LogPanel](g.host, idLog).Lines(); len(lines) != 0 {
		t.Errorf("boot log = %v, want empty (nothing to carry yet)", lines)
	}
	if g.space.pan != (core.Vec2{}) {
		t.Errorf("boot camera pan = %v, want zero", g.space.pan)
	}
	if got := g.selected(); !got.IsZero() {
		t.Errorf("boot selection = %v, want zero", got)
	}
	for _, id := range []ScreenID{idLog, idOverview, idSelected} {
		if !g.host.IsOpen(id) {
			t.Errorf("boot: %s not open (the default set)", id)
		}
	}
	if g.host.IsOpen(idInventory) {
		t.Error("boot: inventory open before anyone opened it")
	}
	if r := g.kit.OverviewWindow().GetContainer().GetWidget().Rect; r.Min.X == 40 && r.Min.Y == 60 {
		t.Error("boot restored the layout sidecar before the first size")
	}
	// The first real size restores it through the normal path.
	g.applySize(1280, 720)
	if r := g.kit.OverviewWindow().GetContainer().GetWidget().Rect; r.Min.X != railClearX() || r.Min.Y != 60 {
		t.Errorf("first size did not restore the sidecar: %v", r)
	}
}

func TestRebuildCarryOverLoad(t *testing.T) {
	resetScale(t)
	g := newShowcaseGameAt(t, filepath.Join(t.TempDir(), "save.json"))
	rebuildSeed(t, g, "before the load")
	oldKit := g.kit
	if err := g.SaveGame(); err != nil {
		t.Fatalf("SaveGame: %v", err)
	}
	// Drop the sidecar so the open flags can only come from the host.
	if err := os.Remove(g.layoutPath()); err != nil {
		t.Fatalf("remove layout: %v", err)
	}
	bodyOf[*uikit.LogPanel](g.host, idLog).PushLine(sim.SevInfo, "after the save")

	if err := g.LoadGame(); err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	if g.kit == oldKit {
		t.Fatal("load did not rebuild the kit")
	}
	if !rebuildLogHas(g, "before the load") || !rebuildLogHas(g, "after the save") {
		t.Errorf("log not carried across the load: %v", bodyOf[*uikit.LogPanel](g.host, idLog).Lines())
	}
	if !g.host.IsOpen(idInventory) || !g.host.Mounted(idInventory) {
		t.Error("open window did not survive the load")
	}
	if g.space.pan != (core.Vec2{}) {
		t.Errorf("load camera pan = %v, want recentred", g.space.pan)
	}
	if got := g.selected(); !got.IsZero() {
		t.Errorf("load selection = %v, want reset", got)
	}
}

func TestRebuildCarryOverScale(t *testing.T) {
	resetScale(t)
	g := newShowcaseGame(t)
	rebuildSeed(t, g, "kept across the scale")
	oldKit := g.kit
	pan := g.space.pan

	g.applyUIScale(1.5)
	if g.kit == oldKit {
		t.Fatal("scale change did not rebuild the kit")
	}
	if !rebuildLogHas(g, "kept across the scale") {
		t.Error("log not carried across the scale change")
	}
	if !g.host.IsOpen(idInventory) || !g.host.Mounted(idInventory) {
		t.Error("open window did not survive the scale change")
	}
	if g.space.pan != pan {
		t.Errorf("camera pan = %v after the scale change, want %v (scale never recentres)", g.space.pan, pan)
	}
	if got := g.selected(); !got.IsZero() {
		t.Errorf("scale selection = %v, want reset", got)
	}
}

func TestRebuildCarryOverReset(t *testing.T) {
	resetScale(t)
	g := newShowcaseGame(t)
	rebuildSeed(t, g, "kept across the reset")
	g.applyUIScale(1.5)
	pan := g.space.pan

	g.resetAllSettings()
	if theme.Scale() != 1.0 || g.uiScale != 1.0 {
		t.Fatalf("reset scale = %v (global %v), want 1", g.uiScale, theme.Scale())
	}
	if !rebuildLogHas(g, "kept across the reset") {
		t.Error("log not carried across the reset rebuild")
	}
	if !g.host.IsOpen(idInventory) {
		t.Error("open window did not survive the reset rebuild")
	}
	if g.space.pan != pan {
		t.Errorf("camera pan = %v after reset, want %v", g.space.pan, pan)
	}
	if got := g.selected(); !got.IsZero() {
		t.Errorf("reset selection = %v, want reset", got)
	}

	// Already at 1× the reset short-circuits: no rebuild, same kit, rail
	// still expanded (the declared outside-the-rebuild step).
	kit := g.kit
	g.setRailCollapsed(true)
	g.resetAllSettings()
	if g.kit != kit {
		t.Error("reset at the installed scale rebuilt the interface")
	}
	if g.railCollapsed || g.rail.Collapsed() {
		t.Error("reset at the installed scale left the rail parked")
	}
}

// TestRebuildFailureReportsOnce: a rebuild that cannot build reports one
// failure and nothing else: today a reset could also announce success.
func TestRebuildFailureReportsOnce(t *testing.T) {
	resetScale(t)
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)
	g.applyUIScale(1.5)
	oldKit := g.kit
	before := len(bodyOf[*uikit.LogPanel](g.host, idLog).Lines())

	g.opts.Content = nil // the kit cannot be built
	g.resetAllSettings()

	lines := bodyOf[*uikit.LogPanel](g.host, idLog).Lines()[before:]
	failed, success := 0, 0
	for _, l := range lines {
		if strings.Contains(l, "Failed:") {
			failed++
		}
		if strings.Contains(l, "Settings reset") {
			success++
		}
	}
	if failed != 1 {
		t.Errorf("failed rebuild reported %d times, want 1: %v", failed, lines)
	}
	if success != 0 {
		t.Errorf("failed reset also claimed success: %v", lines)
	}
	if g.kit != oldKit {
		t.Error("a failed rebuild replaced the live kit")
	}
}

// TestTitleHostRebindsThroughRebuild: the title's own host rebinds with
// every rebuild (here a scale change), not by a load-time special case.
func TestTitleHostRebindsThroughRebuild(t *testing.T) {
	resetScale(t)
	g := newTitleGameAt(t, filepath.Join(t.TempDir(), "save.json"))
	g.applySize(1280, 720)
	g.titleUI.openID(idSettings)
	if err := g.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	before := g.titleUI.host.Window(idSettings)
	if before == nil {
		t.Fatal("settings window did not mount from the title")
	}

	g.applyUIScale(1.25)
	if err := g.Update(); err != nil {
		t.Fatalf("Update after scale change: %v", err)
	}
	after := g.titleUI.host.Window(idSettings)
	if after == nil || after == before {
		t.Fatal("title host did not rebind to the rebuilt kit")
	}
	if !g.titleUI.host.IsOpen(idSettings) || !g.titleUI.host.Mounted(idSettings) {
		t.Error("title settings window lost across the rebuild")
	}
	if !g.title {
		t.Error("a rebuild left the title phase")
	}
}
