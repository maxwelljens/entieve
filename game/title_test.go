package game

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/hajimehoshi/ebiten/v2"
)

// The title phase: a flag-gated boot (Options.StartInTitle) over the
// already-built world. These tests pin the seam (sim frozen, chrome
// suppressed), the menu composition, entry paths (new game / continue /
// manager load), the save manager and the save-as rule.

func newTitleGameAt(t *testing.T, savePath string) *Game {
	t.Helper()
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	wd, ok := c.World("world-showcase")
	if !ok {
		t.Fatal("world-showcase missing from content")
	}
	g, err := New(Options{
		Content:      c,
		World:        wd,
		Seed:         42,
		SavePath:     savePath,
		StartInTitle: true,
		Version:      "1.2.3",
	})
	if err != nil {
		t.Fatalf("game.New: %v", err)
	}
	return g
}

// writeStateSave marshals the current state to path and returns its hash.
func writeStateSave(t *testing.T, g *Game, path string, steps int) uint64 {
	t.Helper()
	for i := 0; i < steps; i++ {
		g.s.Step()
	}
	save, err := g.s.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	data, err := json.MarshalIndent(save, "", "  ")
	if err != nil {
		t.Fatalf("marshal save: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return g.s.Hash()
}

func titleItemByLabel(g *Game, label string) *titleItem {
	for i := range g.titleUI.items {
		if g.titleUI.items[i].Label == label {
			return &g.titleUI.items[i]
		}
	}
	return nil
}

// TestTitleFreezesSim: the title phase never steps the simulation — the
// world behind the menu is a still frame.
func TestTitleFreezesSim(t *testing.T) {
	g := newTitleGameAt(t, filepath.Join(t.TempDir(), "save.json"))
	g.applySize(1280, 720)
	img := ebiten.NewImage(1280, 720)
	h0, t0 := g.s.Hash(), g.s.Time()
	for i := 0; i < 5; i++ {
		if err := g.Update(); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		g.Draw(img)
	}
	if g.s.Hash() != h0 || g.s.Time() != t0 {
		t.Errorf("title stepped the sim: hash %016x → %016x, time %v → %v", h0, g.s.Hash(), t0, g.s.Time())
	}
}

// TestTitleSuppressesGameplayChrome: no gameplay window, HUD, rack, core
// window or rail is mounted or painted during the title phase.
func TestTitleSuppressesGameplayChrome(t *testing.T) {
	g := newTitleGameAt(t, filepath.Join(t.TempDir(), "save.json"))
	g.applySize(1280, 720)
	img := ebiten.NewImage(1280, 720)
	if err := g.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	g.Draw(img)

	if g.hud.Container().GetWidget().IsVisible() {
		t.Error("HUD visible at the title")
	}
	if g.rack.Container().GetWidget().IsVisible() {
		t.Error("module rack visible at the title")
	}
	if g.kit.OverviewWindow().GetContainer().GetWidget().IsVisible() {
		t.Error("Overview visible at the title")
	}
	if g.kit.SelectedWindow().GetContainer().GetWidget().IsVisible() {
		t.Error("Selected Item visible at the title")
	}
	if g.rail.Container().GetWidget().IsVisible() {
		t.Error("navigation rail visible at the title")
	}
	if g.mounted[pLog] != nil {
		t.Error("log window mounted at the title")
	}
	if g.titleUI == nil || g.titleUI.menu == nil || !g.titleUI.menu.GetWidget().IsVisible() {
		t.Error("title menu not visible")
	}
}

// TestTitleMenuComposition: New game / Save manager / Settings / Credits
// / Quit always, Continue only while a save exists.
func TestTitleMenuComposition(t *testing.T) {
	dir := t.TempDir()
	g := newTitleGameAt(t, filepath.Join(dir, "save.json"))
	labels := g.titleUI.labels()
	for _, want := range []string{"New game", "Save manager", "Settings", "Credits", "Quit"} {
		found := false
		for _, l := range labels {
			found = found || l == want
		}
		if !found {
			t.Errorf("menu has no %q row (%v)", want, labels)
		}
	}
	if titleItemByLabel(g, "Continue") != nil {
		t.Fatal("Continue shown with an empty saves directory")
	}

	if err := g.SaveAs(); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	g.titleUI.buildMenu()
	if titleItemByLabel(g, "Continue") == nil {
		t.Fatalf("Continue not shown after a save exists (%v)", g.titleUI.labels())
	}
}

// TestTitleNewGameEnters: the New game click ends the title phase; the
// chrome comes back and the sim steps from the next frame.
func TestTitleNewGameEnters(t *testing.T) {
	g := newTitleGameAt(t, filepath.Join(t.TempDir(), "save.json"))
	g.applySize(1280, 720)
	if err := g.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	item := titleItemByLabel(g, "New game")
	if item == nil {
		t.Fatal("no New game row")
	}
	item.Btn.ClickedEvent.Fire(&widget.ButtonClickedEventArgs{Button: item.Btn})
	if err := g.Update(); err != nil {
		t.Fatalf("Update after New game: %v", err)
	}
	if g.title {
		t.Fatal("New game did not leave the title")
	}
	if !g.hud.Container().GetWidget().IsVisible() {
		t.Error("HUD still hidden after entering the game")
	}
	if g.mounted[pLog] == nil {
		t.Error("log not mounted after entering the game")
	}
	h0 := g.s.Hash()
	for i := 0; i < 3; i++ {
		if err := g.Update(); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
	}
	if g.s.Hash() == h0 {
		t.Error("sim never stepped after entering the game")
	}
}

// TestTitleContinueLoadsNewest: Continue loads the most recently modified
// save (mtime order) and enters on that exact state.
func TestTitleContinueLoadsNewest(t *testing.T) {
	dir := t.TempDir()
	src := newShowcaseGameAt(t, filepath.Join(dir, "save.json"))
	src.applySize(1280, 720)
	old := filepath.Join(dir, "save.json")
	newer := filepath.Join(dir, "save-2.json")
	writeStateSave(t, src, old, 3)
	want := writeStateSave(t, src, newer, 6)
	base := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(old, base, base); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	if err := os.Chtimes(newer, base.Add(time.Hour), base.Add(time.Hour)); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	g := newTitleGameAt(t, filepath.Join(dir, "save.json"))
	g.applySize(1280, 720)
	g.titleUI.continueMostRecent()
	if err := g.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if g.title {
		t.Fatal("Continue did not enter the game")
	}
	if got := g.s.Hash(); got != want {
		t.Errorf("loaded hash %016x, want the newest save %016x", got, want)
	}
	if filepath.Clean(g.activeSavePath()) != filepath.Clean(newer) {
		t.Errorf("active save %q, want %q", g.activeSavePath(), newer)
	}
}

// TestSaveManagerList: *.json only (both sidecar kinds excluded), newest
// first, prettified names.
func TestSaveManagerList(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-time.Hour)
	for i, name := range []string{"save.json", "save-2.json", "save.json.layout.json", "save.json.settings.json", "notes.txt"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		mt := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatalf("Chtimes: %v", err)
		}
	}
	saves := listSaves(dir)
	if len(saves) != 2 {
		t.Fatalf("listSaves = %d entries (%+v), want 2", len(saves), saves)
	}
	if saves[0].Name != "Save 2" || saves[1].Name != "Save" {
		t.Errorf("order/names = %q, %q; want newest first (Save 2, Save)", saves[0].Name, saves[1].Name)
	}
	if saves := listSaves(t.TempDir()); len(saves) != 0 {
		t.Errorf("empty dir lists %d saves", len(saves))
	}
}

// TestSaveManagerWindows: the empty state, the two-click delete and the
// row list a manager open sees.
func TestSaveManagerWindows(t *testing.T) {
	dir := t.TempDir()
	g := newTitleGameAt(t, filepath.Join(dir, "save.json"))
	g.applySize(1280, 720)
	g.titleUI.open("manager")
	if err := g.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if g.titleUI.manager == nil || g.titleUI.managerPage == nil {
		t.Fatal("save manager did not mount")
	}
	if kids := g.titleUI.managerPage.Children(); len(kids) != 1 {
		t.Fatalf("empty manager has %d rows, want the empty state", len(kids))
	} else if txt, ok := kids[0].(*widget.Text); !ok || txt.Label != "No saves yet." {
		t.Errorf("empty state = %v, want %q", kids[0], "No saves yet.")
	}

	src := newShowcaseGameAt(t, filepath.Join(dir, "save.json"))
	writeStateSave(t, src, filepath.Join(dir, "save.json"), 2)
	writeStateSave(t, src, filepath.Join(dir, "save-2.json"), 4)
	g.titleUI.saves = g.listSaves()
	g.titleUI.refreshManager()
	if kids := g.titleUI.managerPage.Children(); len(kids) != 2 {
		t.Fatalf("manager lists %d rows, want 2", len(kids))
	}

	victim := filepath.Join(dir, "save-2.json")
	g.titleUI.deleteSave(victim) // arm
	if _, err := os.Stat(victim); err != nil {
		t.Fatal("first Delete click removed the file (needs a confirm)")
	}
	g.titleUI.deleteSave(victim) // confirm
	if _, err := os.Stat(victim); !os.IsNotExist(err) {
		t.Fatalf("confirm did not remove %s: %v", victim, err)
	}
	if len(g.titleUI.saves) != 1 {
		t.Errorf("one delete left %d saves", len(g.titleUI.saves))
	}

	g.titleUI.deleteSave(filepath.Join(dir, "save.json"))
	g.titleUI.deleteSave(filepath.Join(dir, "save.json"))
	if kids := g.titleUI.managerPage.Children(); len(g.titleUI.saves) != 0 || len(kids) != 1 {
		t.Errorf("deleting the only save left %d saves, %d rows", len(g.titleUI.saves), len(kids))
	}
}

// TestSaveAsRule: save.json while free, then the smallest free
// save-N.json; the write is byte-identical to a direct Snapshot and the
// active path follows.
func TestSaveAsRule(t *testing.T) {
	dir := t.TempDir()
	g := newShowcaseGameAt(t, filepath.Join(dir, "save.json"))

	if err := g.SaveAs(); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	first := filepath.Join(dir, "save.json")
	if filepath.Clean(g.activeSavePath()) != filepath.Clean(first) {
		t.Fatalf("first save-as → %q, want %q", g.activeSavePath(), first)
	}
	firstBytes, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("read first: %v", err)
	}
	want, err := g.s.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	wantBytes, _ := json.MarshalIndent(want, "", "  ")
	if !bytes.Equal(firstBytes, wantBytes) {
		t.Error("save-as bytes differ from a direct Snapshot")
	}

	if err := g.SaveAs(); err != nil {
		t.Fatalf("second SaveAs: %v", err)
	}
	second := filepath.Join(dir, "save-2.json")
	if filepath.Clean(g.activeSavePath()) != filepath.Clean(second) {
		t.Fatalf("second save-as → %q, want %q", g.activeSavePath(), second)
	}

	// Collision: a hand-placed save-3.json is skipped for save-4.json.
	if err := os.WriteFile(filepath.Join(dir, "save-3.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write save-3: %v", err)
	}
	if err := g.SaveAs(); err != nil {
		t.Fatalf("third SaveAs: %v", err)
	}
	third := filepath.Join(dir, "save-4.json")
	if filepath.Clean(g.activeSavePath()) != filepath.Clean(third) {
		t.Fatalf("third save-as → %q, want %q (save-3 exists)", g.activeSavePath(), third)
	}

	// Save now writes the active file, not the default one.
	g.s.Step()
	if err := g.SaveGame(); err != nil {
		t.Fatalf("SaveGame: %v", err)
	}
	if now, _ := os.ReadFile(first); !bytes.Equal(now, firstBytes) {
		t.Error("Save after a save-as overwrote the default save.json")
	}
	cur, err := g.s.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	curBytes, _ := json.MarshalIndent(cur, "", "  ")
	got, err := os.ReadFile(third)
	if err != nil {
		t.Fatalf("read active save: %v", err)
	}
	if !bytes.Equal(got, curBytes) {
		t.Error("Save did not write byte-identical JSON to the active path")
	}
}

// TestTitleSettingsAndCredits: both windows mount from the title, the
// settings actions fire (and the window survives the rebuild a live
// scale change causes), and Esc closes the top window only.
func TestTitleSettingsAndCredits(t *testing.T) {
	resetScale(t)
	dir := t.TempDir()
	g := newTitleGameAt(t, filepath.Join(dir, "save.json"))
	g.applySize(1280, 720)

	g.titleUI.open("settings")
	if err := g.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if g.titleUI.sett == nil || g.titleUI.settRemove == nil {
		t.Fatal("settings window did not mount from the title")
	}
	// The title mounts the same grown settings window: five rows + hint
	// (the kit wraps a plain body in its clip container, so unwrap once).
	clip := g.titleUI.sett.Win().Contents.(widget.Containerer)
	body := clip.Children()[0].(widget.Containerer)
	if n := len(body.Children()); n != 6 {
		t.Errorf("title settings body rows = %d, want 6 (stepper, fullscreen, rail, reset, reset all, hint)", n)
	}
	// The new rail row is wired from the title too (same action bundle).
	g.settActions.SetRailCollapsed(true)
	if !g.railCollapsed || !g.rail.Collapsed() {
		t.Fatal("the title's settings rail row did not park the rail")
	}
	if raw := readSettingsMap(t, g.settingsPath()); raw["rail_collapsed"] != true {
		t.Errorf("title rail toggle sidecar = %v, want rail_collapsed: true", raw)
	}
	before := g.uiScale
	g.settActions.StepScale(1)
	if g.uiScale == before {
		t.Fatal("StepScale did not move the UI scale")
	}
	if err := g.Update(); err != nil { // rebuild + remount after the scale change
		t.Fatalf("Update after scale change: %v", err)
	}
	if g.titleUI.sett == nil || g.titleUI.settRemove == nil {
		t.Error("settings window lost across the scale rebuild")
	}

	g.titleUI.open("credits")
	if err := g.Update(); err != nil {
		t.Fatalf("Update credits: %v", err)
	}
	if g.titleUI.credits == nil || g.titleUI.creditsRem == nil {
		t.Fatal("credits window did not mount from the title")
	}
	g.Draw(ebiten.NewImage(1280, 720)) // layout the window once
	if r := g.titleUI.credits.GetContainer().GetWidget().Rect; r.Dx() <= 0 || r.Dy() <= 0 {
		t.Errorf("credits window never laid out (%v)", r)
	}
	if g.titleUI.settRemove != nil {
		t.Error("opening credits left the settings window mounted")
	}
	g.titleUI.closeTop()
	if err := g.Update(); err != nil {
		t.Fatalf("Update after Esc: %v", err)
	}
	if g.titleUI.credits != nil {
		t.Error("Esc did not close the credits window")
	}
	if !g.title {
		t.Error("Esc at the title left the title phase")
	}
}

// TestTitlePSettingsToggle: P behaves at the title exactly as the
// settings window's hint says — it closes that window (and opens it when
// closed), so the in-game component's "P closes" line stays true in both
// places.
func TestTitlePSettingsToggle(t *testing.T) {
	g := newTitleGameAt(t, filepath.Join(t.TempDir(), "save.json"))
	g.applySize(1280, 720)

	if err := g.updateTitle(Input{JustPressed: []ebiten.Key{ebiten.KeyP}}); err != nil {
		t.Fatalf("P open: %v", err)
	}
	if !g.titleUI.settOpen || g.titleUI.sett == nil || g.titleUI.settRemove == nil {
		t.Fatal("P did not open the title's settings window")
	}
	if err := g.updateTitle(Input{JustPressed: []ebiten.Key{ebiten.KeyP}}); err != nil {
		t.Fatalf("P close: %v", err)
	}
	if g.titleUI.settOpen || g.titleUI.sett != nil {
		t.Error("P did not close the title's settings window (the hint's claim)")
	}
	if !g.title {
		t.Error("P at the title left the title phase")
	}
}

// TestEscMenuSaveAsButton: the new Esc-menu row exists and is wired.
func TestEscMenuSaveAsButton(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	g.push(&MenuScene{g: g})
	g.syncMounts()
	// One frame: ebitenui flushes deferred handlers (built-in ones
	// included) during UI.Update.
	if err := g.kit.Update(); err != nil {
		t.Fatalf("kit.Update: %v", err)
	}
	btn := menuButton(t, g, "Save as…")
	if btn == nil {
		t.Fatal("no Save as… button in the Esc menu")
	}
	btn.ClickedEvent.Fire(&widget.ButtonClickedEventArgs{Button: btn})
	if err := g.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(g.SavePath()), "save.json")); err != nil {
		t.Errorf("Save as… wrote no save.json: %v (active %q, log %v)", err, g.activeSavePath(), g.logP.Lines())
	}
	if filepath.Base(g.activeSavePath()) != "save.json" {
		t.Errorf("active save after Save as… = %q", g.activeSavePath())
	}
}
