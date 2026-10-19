package game

import (
	"encoding/json"
	"errors"
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// layoutDoc is the raw sidecar shape the tests assert against (kept
// independent from layoutFile so a struct rename cannot silently break
// the on-disk format).
type layoutDoc struct {
	Version int `json:"version"`
	Windows map[string]struct {
		X    int  `json:"x"`
		Y    int  `json:"y"`
		W    int  `json:"w"`
		H    int  `json:"h"`
		Open bool `json:"open"`
	} `json:"windows"`
}

// readDoc parses the sidecar at path (test helper).
func readDoc(t *testing.T, path string) layoutDoc {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read sidecar: %v", err)
	}
	var doc layoutDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse sidecar: %v", err)
	}
	return doc
}

// TestClampLayoutRect pins the restore clamping math: sizes grow to
// MinSize first, shrink to the screen second, then the position slides
// fully on-screen; a MinSize larger than the screen wins.
func TestClampLayoutRect(t *testing.T) {
	screen := image.Rect(0, 0, 1024, 700)
	minSize := image.Point{X: 320, Y: 240}
	cases := []struct {
		name string
		r    image.Rectangle
		min  image.Point
		want image.Rectangle
	}{
		{"in bounds unchanged", image.Rect(10, 10, 330, 250), minSize,
			image.Rect(10, 10, 330, 250)},
		{"slid in from right/bottom", image.Rect(900, 600, 1300, 900), minSize,
			image.Rect(624, 400, 1024, 700)},
		{"slid in from negative", image.Rect(-50, -30, 370, 210), minSize,
			image.Rect(0, 0, 420, 240)},
		{"grown to MinSize (top-left held)", image.Rect(10, 10, 110, 60), minSize,
			image.Rect(10, 10, 330, 250)},
		{"grown then slid fully in", image.Rect(900, 600, 1000, 650), minSize,
			image.Rect(704, 460, 1024, 700)},
		{"shrunk to the screen", image.Rect(0, 0, 2000, 1500), minSize,
			image.Rect(0, 0, 1024, 700)},
		{"zero size (corrupt) grows to MinSize", image.Rect(500, 300, 500, 300), minSize,
			image.Rect(500, 300, 820, 540)},
		{"MinSize larger than screen wins", image.Rect(0, 0, 10, 10),
			image.Point{X: 1500, Y: 900}, image.Rect(0, 0, 1500, 900)},
	}
	for _, c := range cases {
		if got := clampLayoutRect(c.r, c.min, screen); got != c.want {
			t.Errorf("%s: clampLayoutRect(%v, %v) = %v, want %v",
				c.name, c.r, c.min, got, c.want)
		}
	}
}

// TestQuitWhileDockedPersistsLiveState: the sidecar records exactly what
// the player last saw — nothing force-opens on dock any more, so the
// services view's flag is the scene's (open while docked) and every
// service window's flag is the player's own intent. A session restored
// while docked surfaces the services view again, and undock closes only
// it.
func TestQuitWhileDockedPersistsLiveState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	g := newShowcaseGameAt(t, path)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)

	// Player intent: fitting open, market/inventory closed.
	g.open[pFitting] = true
	g.syncMounts()

	// Dock: exactly the services view opens.
	dockAt(t, g)
	g.syncScenes()
	g.syncMounts()
	pump(t, g, 1280, 720)
	if !g.open[pServices] {
		t.Fatal("services view not open while docked")
	}
	for _, p := range []panelID{pMarket, pInventory} {
		if g.open[p] {
			t.Fatalf("station panel %s auto-opened on dock — only the services view opens", panelNames[p])
		}
	}

	// Save: the sidecar carries the live state.
	if err := g.SaveGame(); err != nil {
		t.Fatalf("SaveGame: %v", err)
	}
	doc := readDoc(t, g.layoutPath())
	if doc.Windows["market"].Open {
		t.Error("closed market persisted open")
	}
	if doc.Windows["inventory"].Open {
		t.Error("closed inventory persisted open")
	}
	if !doc.Windows["fitting"].Open {
		t.Error("player-open fitting persisted closed")
	}
	if !doc.Windows["services"].Open {
		t.Error("open services view persisted closed while docked")
	}

	// Next session, restored while docked (the save carries the dock):
	// the services view surfaces, the player's intent survives.
	g2 := newShowcaseGameLoaded(t, path)
	g2.applySize(1280, 720)
	pump(t, g2, 1280, 720)
	if !g2.s.Docked.Has(g2.s.Player()) {
		t.Fatal("fixture: the save did not carry the dock")
	}
	if g2.mounted[pServices] == nil {
		t.Error("services view not surfaced on the docked restore")
	}
	if !g2.open[pFitting] || g2.mounted[pFitting] == nil {
		t.Error("player-open fitting closed by the restore")
	}
	if g2.open[pMarket] || g2.mounted[pMarket] != nil {
		t.Error("market reopened by the docked restore")
	}

	// Undock: the services view closes; the player's windows stay.
	g2.s.Docked.Remove(g2.s.Player())
	g2.syncScenes()
	g2.syncMounts()
	pump(t, g2, 1280, 720)
	if g2.open[pServices] || g2.mounted[pServices] != nil {
		t.Error("services view still open in space")
	}
	if !g2.open[pFitting] || g2.mounted[pFitting] == nil {
		t.Error("player-open fitting lost by the dock round-trip")
	}
}

// TestRestoredWindowsReclampOnResize: a sidecar written at 1920×1080
// restores there, then the window shrinks to 1024×700. Restored windows
// are re-clamped to the new screen on every applySize (not only at
// restore time): each one ends up fully on-screen and at/above its
// MinSize, keeps its restored flag (the arrangement stays the player's),
// stays tracked, and writeLayout no longer persists off-screen coords.
func TestRestoredWindowsReclampOnResize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	if err := os.WriteFile(path+".layout.json", []byte(`{"version":2,"windows":{
		"overview":{"x":1500,"y":780,"w":400,"h":300,"open":true},
		"log":{"x":1400,"y":600,"w":360,"h":240,"open":true},
		"market":{"x":1000,"y":400,"w":420,"h":300,"open":true}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	g := newShowcaseGameAt(t, path)
	g.applySize(1920, 1080)
	pump(t, g, 1920, 1080)
	ov := g.kit.OverviewWindow()
	if got := ov.GetContainer().GetWidget().Rect; got != image.Rect(1500, 780, 1900, 1080) {
		t.Fatalf("restore at 1920: overview = %v, want (1500,780)-(1900,1080)", got)
	}

	g.applySize(1024, 700)
	pump(t, g, 1024, 700)

	// Exact clamp math for the overview (400×300 keeps its size, both
	// coordinates slide to the 1024×700 screen).
	if got, want := ov.GetContainer().GetWidget().Rect, image.Rect(624, 400, 1024, 700); got != want {
		t.Errorf("overview after the shrink = %v, want the clamped %v", got, want)
	}
	if !g.restored[ov] {
		t.Error("re-clamp dropped the restored flag (the arrangement would recompose)")
	}
	if got := g.placed[ov]; got != ov.GetContainer().GetWidget().Rect {
		t.Errorf("tracker after re-clamp = %v, want the actual %v",
			got, ov.GetContainer().GetWidget().Rect)
	}

	// Every persisted window: on-screen and at/above its MinSize.
	screen := image.Rect(0, 0, 1024, 700)
	for _, name := range layoutNames {
		win, _ := g.layoutTarget(name)
		if win == nil {
			t.Fatalf("layout window %q does not resolve", name)
		}
		r := win.GetContainer().GetWidget().Rect
		if r.Dx() == 0 && r.Dy() == 0 {
			t.Errorf("%s: never laid out", name)
			continue
		}
		if !r.In(screen) {
			t.Errorf("%s: %v out of bounds at 1024×700", name, r)
		}
		if min := win.MinSize; min != nil && (r.Dx() < min.X || r.Dy() < min.Y) {
			t.Errorf("%s: %v below MinSize %v", name, r, min)
		}
	}

	// The sidecar must not carry the off-screen coordinates any more.
	if err := g.writeLayout(); err != nil {
		t.Fatalf("writeLayout: %v", err)
	}
	for name, e := range readDoc(t, g.layoutPath()).Windows {
		if e.X < 0 || e.Y < 0 || e.X+e.W > 1024 || e.Y+e.H > 700 {
			t.Errorf("sidecar %s entry %+v is off-screen after the shrink", name, e)
		}
	}

	// Growing back keeps the clamped arrangement: restored windows are
	// never recomposed, only clamped.
	g.applySize(1920, 1080)
	pump(t, g, 1920, 1080)
	if got := ov.GetContainer().GetWidget().Rect; got != image.Rect(624, 400, 1024, 700) {
		t.Errorf("overview after regrowing = %v, want the clamped rect kept", got)
	}
}

// TestPlacementTrackerMatchesActualRects: after any compose or restore
// g.placed holds each tracked window's REAL rect. A stale entry
// misclassifies the window as player-moved and freezes its composition
// forever — which is what the station board's content hug did when it
// called SetLocation directly.
func TestPlacementTrackerMatchesActualRects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	g := newShowcaseGameAt(t, path)
	assertTracked := func(stage string) {
		t.Helper()
		if len(g.placed) == 0 {
			t.Fatalf("%s: nothing is tracked", stage)
		}
		for win, r := range g.placed {
			if actual := win.GetContainer().GetWidget().Rect; actual != r {
				t.Errorf("%s: tracked %v, actual %v — this window now reads as player-moved",
					stage, r, actual)
			}
		}
	}

	g.applySize(1280, 720)
	pump(t, g, 1280, 720)
	assertTracked("compose")

	// Docked: the board's content hug recomposes the station column —
	// through the tracker, so the invariant survives it.
	dockAt(t, g)
	g.syncScenes()
	g.syncMounts()
	pump(t, g, 1280, 720)
	assertTracked("docked board hug")
	station := g.svcWin.GetContainer().GetWidget().Rect
	if want := stationLayout(1280, 720, station.Dy()); station != want.Station {
		t.Errorf("station = %v, want the composed %v", station, want.Station)
	}
	if g.placed[g.svcWin] != station {
		t.Errorf("station tracker = %v, want the actual %v", g.placed[g.svcWin], station)
	}

	// Restore path: a fresh session over this session's sidecar tracks
	// the rects it applied (including any MinSize growth).
	if err := g.SaveGame(); err != nil {
		t.Fatalf("SaveGame: %v", err)
	}
	g2 := newShowcaseGameAt(t, path)
	g2.applySize(1280, 720)
	pump(t, g2, 1280, 720)
	if len(g2.restored) == 0 {
		t.Fatal("fixture: nothing was restored from the sidecar")
	}
	if len(g2.placed) == 0 {
		t.Fatal("restore left the placement tracker empty")
	}
	for win, r := range g2.placed {
		if actual := win.GetContainer().GetWidget().Rect; actual != r {
			t.Errorf("restore: tracked %v, actual %v", r, actual)
		}
	}
}

// TestLayoutSidecarFormat: SaveGame writes <SavePath>.layout.json with
// version, one flat {x,y,w,h,open} entry per named window, and the
// sim save file itself stays untouched by layout data.
func TestLayoutSidecarFormat(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)

	if err := g.SaveGame(); err != nil {
		t.Fatalf("SaveGame: %v", err)
	}
	wantPath := g.SavePath() + ".layout.json"
	if g.layoutPath() != wantPath {
		t.Fatalf("layoutPath = %q, want %q", g.layoutPath(), wantPath)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("sidecar missing: %v", err)
	}

	doc := readDoc(t, wantPath)
	if doc.Version != layoutVersion {
		t.Errorf("version = %d, want %d", doc.Version, layoutVersion)
	}
	for _, name := range layoutNames {
		e, ok := doc.Windows[name]
		if !ok {
			t.Errorf("window %q missing from the sidecar", name)
			continue
		}
		if e.W <= 0 || e.H <= 0 {
			t.Errorf("window %q has degenerate size %dx%d", name, e.W, e.H)
		}
	}
	if !doc.Windows["overview"].Open || !doc.Windows["log"].Open {
		t.Error("always-open windows must persist open=true")
	}

	// The sim save stays a pure sim.Save (CLI readers depend on it).
	data, err := os.ReadFile(g.SavePath())
	if err != nil {
		t.Fatalf("read save: %v", err)
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		t.Fatalf("save is not JSON: %v", err)
	}
	for _, k := range []string{"windows", "layout"} {
		if _, bad := probe[k]; bad {
			t.Errorf("sim save leaked layout key %q", k)
		}
	}
}

// TestLayoutRoundTripAcrossRestart: move and close windows, write the
// sidecar, start a fresh session on the same save path at a different
// screen size — geometry restores (clamped to the new screen) and open
// flags rebuild the mounts.
func TestLayoutRoundTripAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	g := newShowcaseGameAt(t, path)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)

	g.kit.OverviewWindow().SetLocation(image.Rect(40, 60, 440, 360)) // 400×300
	g.logP.Win().SetLocation(image.Rect(700, 300, 1060, 540))        // 360×240, off a 1024 screen
	g.open[pInventory] = true
	g.open[pLog] = false
	g.syncMounts()
	if err := g.writeLayout(); err != nil {
		t.Fatalf("writeLayout: %v", err)
	}

	// Restart: same path, minimum screen (1024×700).
	g2 := newShowcaseGameAt(t, path)
	g2.applySize(1024, 700)
	pump(t, g2, 1024, 700)

	ov := g2.kit.OverviewWindow().GetContainer().GetWidget().Rect
	if ov != image.Rect(40, 60, 440, 360) {
		t.Errorf("overview restored to %v, want (40,60)-(440,360)", ov)
	}
	// The saved log rect overflows 1024 px: size kept, slid fully in.
	lr := g2.logP.Win().GetContainer().GetWidget().Rect
	if lr != image.Rect(664, 300, 1024, 540) {
		t.Errorf("log restored to %v, want (664,300)-(1024,540)", lr)
	}
	if !g2.open[pInventory] || g2.mounted[pInventory] == nil {
		t.Error("inventory open state not restored (flag or mount)")
	}
	if g2.open[pLog] || g2.mounted[pLog] != nil {
		t.Error("closed log re-opened on restart")
	}
	// Bounds only: a player-arranged layout is allowed to overlap (the
	// pairwise-disjoint guarantee belongs to the defaults).
	assertInBounds(t, g2, 1024, 700)
}

// TestLayoutCorruptAndPartialDegrade: a corrupt sidecar leaves the
// defaults alone, a future-version sidecar is ignored, a partial one
// restores only what it holds (unknown keys are skipped), and none of
// it ever errors or crashes.
func TestLayoutCorruptAndPartialDegrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")

	// Corrupt bytes at startup: defaults win.
	if err := os.WriteFile(path+".layout.json", []byte("{not json["), 0o644); err != nil {
		t.Fatal(err)
	}
	g := newShowcaseGameAt(t, path)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)
	if r := g.kit.OverviewWindow().GetContainer().GetWidget().Rect; r != defaultOverviewRect(1280) {
		t.Errorf("corrupt sidecar moved the overview to %v, want the default", r)
	}

	// A newer format version is ignored wholesale.
	if err := os.WriteFile(path+".layout.json",
		[]byte(`{"version":99,"windows":{"overview":{"x":1,"y":2,"w":400,"h":300,"open":false}}}`),
		0o644); err != nil {
		t.Fatal(err)
	}
	g.applyLayout(1280, 720)
	if r := g.kit.OverviewWindow().GetContainer().GetWidget().Rect; r != defaultOverviewRect(1280) {
		t.Errorf("version-99 sidecar was applied (%v)", r)
	}

	// Partial sidecar: only the log entry (plus an unknown name) — the
	// log restores, everything else keeps its default, and the unknown
	// key is ignored.
	if err := os.WriteFile(path+".layout.json", []byte(
		`{"version":2,"windows":{
			"log":{"x":500,"y":100,"w":300,"h":200,"open":false},
			"future-window":{"x":9,"y":9,"w":10,"h":10,"open":true}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	g.applyLayout(1280, 720)
	lr := g.logP.Win().GetContainer().GetWidget().Rect
	if lr != image.Rect(500, 100, 800, 300) {
		t.Errorf("log restored to %v, want (500,100)-(800,300)", lr)
	}
	if g.open[pLog] {
		t.Error("log open flag not restored from the partial sidecar")
	}
	if r := g.kit.OverviewWindow().GetContainer().GetWidget().Rect; r != defaultOverviewRect(1280) {
		t.Errorf("absent overview entry moved the window to %v", r)
	}
	if r := g.scan.Win().GetContainer().GetWidget().Rect; r.Dx() == 0 {
		t.Error("absent scanner entry broke its geometry")
	}
	assertInBounds(t, g, 1280, 720)
}

// TestLayoutWrittenOnSaveAndQuit: the sidecar rides along with Ctrl+S
// and is rewritten on a clean quit, so a layout survives a restart
// without any manual save.
func TestLayoutWrittenOnSaveAndQuit(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)

	g.kit.OverviewWindow().SetLocation(image.Rect(40, 60, 440, 360))
	if err := g.SaveGame(); err != nil {
		t.Fatalf("SaveGame: %v", err)
	}
	doc := readDoc(t, g.layoutPath())
	if e := doc.Windows["overview"]; e.X != 40 || e.Y != 60 || e.W != 400 || e.H != 300 {
		t.Errorf("after save: overview = %+v, want x40 y60 400×300", e)
	}

	// Rearrange without saving, then quit: the quit path writes it.
	g.kit.OverviewWindow().SetLocation(image.Rect(50, 70, 450, 370))
	g.quit = true
	if err := g.Update(); !errors.Is(err, ebiten.Termination) {
		t.Fatalf("quit Update error = %v, want ebiten.Termination", err)
	}
	doc = readDoc(t, g.layoutPath())
	if e := doc.Windows["overview"]; e.X != 50 || e.Y != 70 {
		t.Errorf("after quit: overview = %+v, want x50 y70", e)
	}
}

// TestLayoutRestoredOnLoad: LoadGame rebuilds the kit, then the saved
// layout lands on the fresh windows (geometry and open flags).
func TestLayoutRestoredOnLoad(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)

	g.kit.OverviewWindow().SetLocation(image.Rect(40, 60, 440, 360))
	g.open[pInventory] = true
	g.syncMounts()
	if err := g.SaveGame(); err != nil {
		t.Fatalf("SaveGame: %v", err)
	}

	// Rearrange after the save: loading must rewind the layout too.
	g.kit.OverviewWindow().SetLocation(image.Rect(500, 300, 900, 600))
	g.open[pInventory] = false
	g.syncMounts()
	if err := g.LoadGame(); err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	r := g.kit.OverviewWindow().GetContainer().GetWidget().Rect
	if r != image.Rect(40, 60, 440, 360) {
		t.Errorf("overview after load = %v, want (40,60)-(440,360)", r)
	}
	if !g.open[pInventory] || g.mounted[pInventory] == nil {
		t.Error("inventory open state not restored by the load")
	}
	pump(t, g, 1280, 720)
	assertInBounds(t, g, 1280, 720)
}

// TestLayoutPersistsHiddenCoreWindows: while docked the Overview and
// Selected Item windows are hidden — their geometry still round-trips
// (hidden widgets keep their rect, and writeLayout reads it).
func TestLayoutPersistsHiddenCoreWindows(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)

	g.kit.OverviewWindow().SetLocation(image.Rect(120, 40, 520, 340))
	dockAt(t, g)
	g.syncScenes()
	g.syncMounts()
	pump(t, g, 1280, 720)
	if g.kit.OverviewWindow().GetContainer().GetWidget().IsVisible() {
		t.Fatal("overview should be hidden while docked")
	}
	if err := g.writeLayout(); err != nil {
		t.Fatalf("writeLayout: %v", err)
	}
	e := readDoc(t, g.layoutPath()).Windows["overview"]
	if e.X != 120 || e.Y != 40 || e.W != 400 || e.H != 300 {
		t.Errorf("hidden overview persisted as %+v, want x120 y40 400×300", e)
	}
}

// TestLogWindowGating: the log gates world input through the mounted-
// window pass while open, and a closed log never gates.
func TestLogWindowGating(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)

	r := g.logP.Win().GetContainer().GetWidget().Rect
	if r.Dx() == 0 {
		t.Fatal("log window never laid out")
	}
	x, y := centerOf(t, r)
	if !g.overUI(x, y) {
		t.Errorf("open log not gated: overUI(%v, %v) = false", x, y)
	}

	g.open[pLog] = false
	g.syncMounts()
	pump(t, g, 1280, 720)
	if g.mounted[pLog] != nil {
		t.Fatal("log still mounted after close")
	}
	if g.overUI(x, y) {
		t.Errorf("closed log still gates the world at (%v, %v)", x, y)
	}
}
