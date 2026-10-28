package game

import (
	"encoding/json"
	"errors"
	"image"
	"math"
	"os"
	"path/filepath"
	"testing"

	uikit "github.com/entieve/entieve/uikit/screens"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// layoutDoc is the raw sidecar shape the tests assert against (kept
// independent from layoutFile so a struct rename cannot silently break
// the on-disk format).
type layoutDoc struct {
	Version int `json:"version"`
	Windows map[string]struct {
		X      int                `json:"x"`
		Y      int                `json:"y"`
		W      int                `json:"w"`
		H      int                `json:"h"`
		Open   bool               `json:"open"`
		Splits map[string]float64 `json:"splits"`
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
	g.host.SetOpen(idFitting, true)
	g.syncMounts()

	// Dock: exactly the services view opens.
	dockAt(t, g)
	g.syncScenes()
	g.syncMounts()
	pump(t, g, 1280, 720)
	if !g.host.IsOpen(idServices) {
		t.Fatal("services view not open while docked")
	}
	for _, p := range []ScreenID{idMarket, idInventory} {
		if g.host.IsOpen(p) {
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
	if !g2.host.Mounted(idServices) {
		t.Error("services view not surfaced on the docked restore")
	}
	if !g2.host.IsOpen(idFitting) || !g2.host.Mounted(idFitting) {
		t.Error("player-open fitting closed by the restore")
	}
	if g2.host.IsOpen(idMarket) || g2.host.Mounted(idMarket) {
		t.Error("market reopened by the docked restore")
	}

	// Undock: the services view closes; the player's windows stay.
	g2.s.Docked.Remove(g2.s.Player())
	g2.syncScenes()
	g2.syncMounts()
	pump(t, g2, 1280, 720)
	if g2.host.IsOpen(idServices) || g2.host.Mounted(idServices) {
		t.Error("services view still open in space")
	}
	if !g2.host.IsOpen(idFitting) || !g2.host.Mounted(idFitting) {
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
	if !g.host.restored[ov] {
		t.Error("re-clamp dropped the restored flag (the arrangement would recompose)")
	}
	if got := g.host.placed[ov]; got != ov.GetContainer().GetWidget().Rect {
		t.Errorf("tracker after re-clamp = %v, want the actual %v",
			got, ov.GetContainer().GetWidget().Rect)
	}

	// Every persisted window: on-screen and at/above its MinSize.
	screen := image.Rect(0, 0, 1024, 700)
	for _, name := range layoutNames {
		entry := g.layoutEntry(name)
		if entry == nil || entry.Win() == nil {
			t.Fatalf("layout window %q does not resolve", name)
		}
		r := entry.Win().GetContainer().GetWidget().Rect
		if r.Dx() == 0 && r.Dy() == 0 {
			t.Errorf("%s: never laid out", name)
			continue
		}
		if !r.In(screen) {
			t.Errorf("%s: %v out of bounds at 1024×700", name, r)
		}
		if min := entry.Win().MinSize; min != nil && (r.Dx() < min.X || r.Dy() < min.Y) {
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
// g.host.placed holds each tracked window's REAL rect. A stale entry
// misclassifies the window as player-moved and freezes its composition
// forever — which is what the station board's content hug did when it
// called SetLocation directly.
func TestPlacementTrackerMatchesActualRects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	g := newShowcaseGameAt(t, path)
	assertTracked := func(stage string) {
		t.Helper()
		if len(g.host.placed) == 0 {
			t.Fatalf("%s: nothing is tracked", stage)
		}
		for win, r := range g.host.placed {
			if actual := win.GetContainer().GetWidget().Rect; actual != r {
				t.Errorf("%s: tracked %v, actual %v — this window now reads as player-moved",
					stage, r, actual)
			}
		}
	}

	g.applySize(1280, 720)
	pump(t, g, 1280, 720)
	assertTracked("compose")

	// Docked: the board's content hug re-places the station column —
	// through the tracker, so the invariant survives it.
	dockAt(t, g)
	g.syncScenes()
	g.syncMounts()
	pump(t, g, 1280, 720)
	assertTracked("docked board hug")
	svcWin := g.host.Window(idServices)
	station := svcWin.GetContainer().GetWidget().Rect
	if want := stationRect(station.Dy()); station != want {
		t.Errorf("station = %v, want the station column %v", station, want)
	}
	if g.host.placed[svcWin] != station {
		t.Errorf("station tracker = %v, want the actual %v", g.host.placed[svcWin], station)
	}

	// Restore path: a fresh session over this session's sidecar tracks
	// the rects it applied (including any MinSize growth).
	if err := g.SaveGame(); err != nil {
		t.Fatalf("SaveGame: %v", err)
	}
	g2 := newShowcaseGameAt(t, path)
	g2.applySize(1280, 720)
	pump(t, g2, 1280, 720)
	if len(g2.host.restored) == 0 {
		t.Fatal("fixture: nothing was restored from the sidecar")
	}
	if len(g2.host.placed) == 0 {
		t.Fatal("restore left the placement tracker empty")
	}
	for win, r := range g2.host.placed {
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

	g.kit.OverviewWindow().SetLocation(image.Rect(40, 60, 440, 360))                          // 400×300
	bodyOf[*uikit.LogPanel](g.host, idLog).Win().SetLocation(image.Rect(700, 300, 1060, 540)) // 360×240, off a 1024 screen
	g.host.SetOpen(idInventory, true)
	g.host.SetOpen(idLog, false)
	g.syncMounts()
	if err := g.writeLayout(); err != nil {
		t.Fatalf("writeLayout: %v", err)
	}

	// Restart: same path, minimum screen (1024×700).
	g2 := newShowcaseGameAt(t, path)
	g2.applySize(1024, 700)
	pump(t, g2, 1024, 700)

	// The saved x is under the rail gutter (railClearX 97): the restore
	// floors the left edge, keeping the saved size and y.
	ov := g2.kit.OverviewWindow().GetContainer().GetWidget().Rect
	if want := image.Rect(railClearX(), 60, railClearX()+400, 360); ov != want {
		t.Errorf("overview restored to %v, want %v (floored clear of the rail)", ov, want)
	}
	// The saved log rect overflows 1024 px: size kept, slid fully in.
	lr := bodyOf[*uikit.LogPanel](g2.host, idLog).Win().GetContainer().GetWidget().Rect
	if lr != image.Rect(664, 300, 1024, 540) {
		t.Errorf("log restored to %v, want (664,300)-(1024,540)", lr)
	}
	if !g2.host.IsOpen(idInventory) || !g2.host.Mounted(idInventory) {
		t.Error("inventory open state not restored (flag or mount)")
	}
	if g2.host.IsOpen(idLog) || g2.host.Mounted(idLog) {
		t.Error("closed log re-opened on restart")
	}
	// Bounds only: a player-arranged layout is allowed to overlap (the
	// pairwise-disjoint guarantee belongs to the defaults).
	assertInBounds(t, g2, 1024, 700)
}

// TestRestoredRectsClearTheRailGutter pins the load-time sanitize of the
// sidecar: a title bar saved under the rail gutter (the layouts written
// before RailInset grew to 87) restores right of railClearX, an
// off-screen one restores on-screen, and a reachable one restores
// exactly as saved. Only reachability moves a rect — the saved size and
// y, and the market's saved (possibly old band) size, are user layout.
func TestRestoredRectsClearTheRailGutter(t *testing.T) {
	write := func(t *testing.T, path, windows string) {
		t.Helper()
		if err := os.WriteFile(path+".layout.json", []byte(
			`{"version":2,"windows":{`+windows+`}}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("tucked under the rail", func(t *testing.T) {
		for _, entry := range []struct {
			name  string
			win   func(*Game) image.Rectangle
			json  string
			wantW int
		}{
			{"overview", func(g *Game) image.Rectangle {
				return g.kit.OverviewWindow().GetContainer().GetWidget().Rect
			}, `"overview":{"x":8,"y":60,"w":400,"h":300,"open":true}`, 400},
			{"log", func(g *Game) image.Rectangle {
				return bodyOf[*uikit.LogPanel](g.host, idLog).Win().GetContainer().GetWidget().Rect
			}, `"log":{"x":95,"y":300,"w":360,"h":240,"open":true}`, 360},
		} {
			path := filepath.Join(t.TempDir(), "save.json")
			write(t, path, entry.json)
			g := newShowcaseGameAt(t, path)
			g.applySize(1024, 700)
			r := entry.win(g)
			if r.Min.X != railClearX() {
				t.Errorf("%s restored at x=%d, want railClearX %d", entry.name, r.Min.X, railClearX())
			}
			if r.Dx() != entry.wantW {
				t.Errorf("%s width = %d, want the saved %d (size is user layout)", entry.name, r.Dx(), entry.wantW)
			}
			if entry.name == "overview" && r.Min.Y != 60 {
				t.Errorf("overview y = %d, want the saved 60", r.Min.Y)
			}
		}
	})

	t.Run("off-screen", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "save.json")
		write(t, path, `"overview":{"x":1500,"y":800,"w":400,"h":300,"open":true}`)
		g := newShowcaseGameAt(t, path)
		g.applySize(1024, 700)
		r := g.kit.OverviewWindow().GetContainer().GetWidget().Rect
		if want := image.Rect(624, 400, 1024, 700); r != want {
			t.Errorf("off-screen overview restored to %v, want the clamped %v", r, want)
		}
		if r.Min.X < 0 || r.Min.Y < 0 || r.Max.X > 1024 || r.Min.Y+theme.TitleBarH > 700 {
			t.Errorf("title bar not fully on-screen: %v", r)
		}
	})

	t.Run("reachable layout honored", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "save.json")
		write(t, path, `"overview":{"x":200,"y":100,"w":400,"h":300,"open":true}`)
		g := newShowcaseGameAt(t, path)
		g.applySize(1024, 700)
		if r := g.kit.OverviewWindow().GetContainer().GetWidget().Rect; r != image.Rect(200, 100, 600, 400) {
			t.Errorf("reachable overview restored to %v, want it exactly as saved", r)
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "save.json")
		write(t, path, `"overview":{"x":8,"y":60,"w":400,"h":300,"open":true}`)
		g := newShowcaseGameAt(t, path)
		g.applySize(1024, 700)
		once := g.kit.OverviewWindow().GetContainer().GetWidget().Rect
		if err := g.writeLayout(); err != nil {
			t.Fatalf("writeLayout: %v", err)
		}
		g.applyLayout(1024, 700)
		if twice := g.kit.OverviewWindow().GetContainer().GetWidget().Rect; twice != once {
			t.Errorf("re-sanitized rect = %v, want the already-sanitized %v", twice, once)
		}
	})

	t.Run("market saved size kept", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "save.json")
		// The market's pre-square 618×240 band: the exact saved size
		// restores untouched (reset layout, not restore, re-defaults it).
		write(t, path, `"market":{"x":400,"y":200,"w":618,"h":240,"open":true}`)
		g := newShowcaseGameAt(t, path)
		g.applySize(1280, 720)
		if r := bodyOf[*uikit.Market](g.host, idMarket).Win().GetContainer().GetWidget().Rect; r != image.Rect(400, 200, 1018, 440) {
			t.Errorf("market restored to %v, want the saved 618×240 rect at (400,200)", r)
		}
	})
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
	lr := bodyOf[*uikit.LogPanel](g.host, idLog).Win().GetContainer().GetWidget().Rect
	if lr != image.Rect(500, 100, 800, 300) {
		t.Errorf("log restored to %v, want (500,100)-(800,300)", lr)
	}
	if g.host.IsOpen(idLog) {
		t.Error("log open flag not restored from the partial sidecar")
	}
	if r := g.kit.OverviewWindow().GetContainer().GetWidget().Rect; r != defaultOverviewRect(1280) {
		t.Errorf("absent overview entry moved the window to %v", r)
	}
	if r := bodyOf[*uikit.Scanner](g.host, idScanner).Win().GetContainer().GetWidget().Rect; r.Dx() == 0 {
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
	g.host.SetOpen(idInventory, true)
	g.syncMounts()
	if err := g.SaveGame(); err != nil {
		t.Fatalf("SaveGame: %v", err)
	}

	// Rearrange after the save: loading must rewind the layout too.
	g.kit.OverviewWindow().SetLocation(image.Rect(500, 300, 900, 600))
	g.host.SetOpen(idInventory, false)
	g.syncMounts()
	if err := g.LoadGame(); err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	r := g.kit.OverviewWindow().GetContainer().GetWidget().Rect
	if want := image.Rect(railClearX(), 60, railClearX()+400, 360); r != want {
		t.Errorf("overview after load = %v, want %v (floored clear of the rail)", r, want)
	}
	if !g.host.IsOpen(idInventory) || !g.host.Mounted(idInventory) {
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

	r := bodyOf[*uikit.LogPanel](g.host, idLog).Win().GetContainer().GetWidget().Rect
	if r.Dx() == 0 {
		t.Fatal("log window never laid out")
	}
	x, y := centerOf(t, r)
	if !g.overUI(x, y) {
		t.Errorf("open log not gated: overUI(%v, %v) = false", x, y)
	}

	g.host.SetOpen(idLog, false)
	g.syncMounts()
	pump(t, g, 1280, 720)
	if g.host.Mounted(idLog) {
		t.Fatal("log still mounted after close")
	}
	if g.overUI(x, y) {
		t.Errorf("closed log still gates the world at (%v, %v)", x, y)
	}
}

// TestWriteLayoutNeverPersistsDegenerateRects: the writer must never
// persist a zero/degenerate geometry — a named window whose rect was
// zeroed round-trips at at least its MinSize (grown top-left held,
// exactly what the read-side clamp would apply), so no future read can
// pin a zero rect.
func TestWriteLayoutNeverPersistsDegenerateRects(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)

	bodyOf[*uikit.HelpPanel](g.host, idHelp).Win().GetContainer().GetWidget().Rect = image.Rectangle{}
	if err := g.writeLayout(); err != nil {
		t.Fatalf("writeLayout: %v", err)
	}
	e := readDoc(t, g.layoutPath()).Windows["help"]
	if e.W <= 0 || e.H <= 0 {
		t.Errorf("help persisted as %+v: degenerate %d×%d geometry", e, e.W, e.H)
	}
	if min := bodyOf[*uikit.HelpPanel](g.host, idHelp).Win().MinSize; min != nil && (e.W < min.X || e.H < min.Y) {
		t.Errorf("help persisted as %+v, below MinSize %v", e, min)
	}
}

// TestStaleLayoutVersionIgnored: version 1 files (written by builds
// predating the current sidecar) are ignored wholesale: defaults win
// instead of the stale geometry being pinned as saved-wins. The old
// docked service-trio fold is gone with the composition, so the market,
// inventory and fitting simply take their per-type defaults.
func TestStaleLayoutVersionIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	if err := os.WriteFile(path+".layout.json", []byte(
		`{"version":1,"windows":{`+
			`"overview":{"x":700,"y":500,"w":400,"h":300,"open":false},`+
			`"market":{"x":396,"y":8,"w":874,"h":240,"open":false},`+
			`"inventory":{"x":396,"y":8,"w":874,"h":240,"open":false},`+
			`"fitting":{"x":396,"y":8,"w":874,"h":240,"open":false}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	g := newShowcaseGameAt(t, path)
	g.applySize(1280, 720)
	if r := g.kit.OverviewWindow().GetContainer().GetWidget().Rect; r != defaultOverviewRect(1280) {
		t.Errorf("version-1 sidecar was applied: overview = %v, want the default %v", r, defaultOverviewRect(1280))
	}
	for id, want := range serviceDefaults {
		if got := g.host.Window(id).GetContainer().GetWidget().Rect; got != want {
			t.Errorf("version-1 sidecar was applied: %s = %v, want the default %v",
				panelNames[id], got, want)
		}
	}
}

// assertSplitTiled: a split's panes and hairlines fill its rect: no pane
// collapsed and no gap (a resize or a restored ratio must never leave the
// stack short).
func assertSplitTiled(t *testing.T, name string, split *widgets.SplitPane) {
	t.Helper()
	root := split.Container().GetWidget().Rect
	total := 0
	for _, ch := range split.Container().Children() {
		h := ch.GetWidget().Rect.Dy()
		if h == 0 {
			t.Errorf("%s: a split row is 0 px tall (%v)", name, ch.GetWidget().Rect)
		}
		total += h
	}
	if total != root.Dy() {
		t.Errorf("%s: split rows fill %d px, want the %d px rect", name, total, root.Dy())
	}
}

// seamRatio reads a window's named seam ratio through its panel.
func seamRatio(name string, seams []widgets.Seam) float64 {
	for _, sm := range seams {
		if sm.Name == name {
			return sm.Split.Ratio(sm.Index)
		}
	}
	return -1
}

// TestSplitRatiosPersistAcrossRestart: a dragged seam writes its ratio
// into the version-3 sidecar and a fresh session restores it exactly,
// while a seam nobody touched keeps its equal share.
func TestSplitRatiosPersistAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	g := newShowcaseGameAt(t, path)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)

	// Drag inventory's first seam and fitting's only one (through the same
	// panels the windows expose to the sidecar).
	inv := g.host.Slot(string(idInventory)).Splits()
	inv[0].Split.SetRatio(inv[0].Index, 0.42)
	fit := g.host.Slot(string(idFitting)).Splits()
	fit[0].Split.SetRatio(fit[0].Index, 0.7)

	if err := g.SaveGame(); err != nil {
		t.Fatalf("SaveGame: %v", err)
	}
	doc := readDoc(t, g.layoutPath())
	if doc.Version != 3 {
		t.Fatalf("sidecar version = %d, want 3", doc.Version)
	}
	if got := doc.Windows["inventory"].Splits["inventory-ore"]; got != 0.42 {
		t.Errorf("inventory-ore = %v, want the dragged 0.42", got)
	}
	if got := doc.Windows["inventory"].Splits["inventory-hangar"]; got != float64(2)/3 {
		t.Errorf("inventory-hangar = %v, want the untouched equal share %v", got, float64(2)/3)
	}
	if got := doc.Windows["fitting"].Splits["fitting-hangar"]; got != 0.7 {
		t.Errorf("fitting-hangar = %v, want the dragged 0.7", got)
	}
	if got := doc.Windows["manufacturing"].Splits["manufacturing-jobs"]; got != 0.5 {
		t.Errorf("manufacturing-jobs = %v, want the equal-share default 0.5", got)
	}

	// Next session: the ratios come back exactly, then a viewport change
	// still tiles every split (nothing collapsed by the pane floor).
	g2 := newShowcaseGameAt(t, path)
	g2.applySize(1280, 720)
	// Mount the two windows whose splits are checked (both closed in the
	// sidecar: a drag persists ratios, never the open flag).
	g2.host.SetOpen(idInventory, true)
	g2.host.SetOpen(idFitting, true)
	g2.syncMounts()
	pump(t, g2, 1280, 720)
	if got := seamRatio("inventory-ore", g2.host.Slot(string(idInventory)).Splits()); got != 0.42 {
		t.Errorf("restored inventory-ore = %v, want 0.42", got)
	}
	if got := seamRatio("inventory-hangar", g2.host.Slot(string(idInventory)).Splits()); got != float64(2)/3 {
		t.Errorf("restored inventory-hangar = %v, want the equal share", got)
	}
	if got := seamRatio("fitting-hangar", g2.host.Slot(string(idFitting)).Splits()); got != 0.7 {
		t.Errorf("restored fitting-hangar = %v, want 0.7", got)
	}
	g2.applySize(1024, 700)
	pump(t, g2, 1024, 700)
	assertSplitTiled(t, "inventory", g2.host.Slot(string(idInventory)).Splits()[0].Split)
	assertSplitTiled(t, "fitting", g2.host.Slot(string(idFitting)).Splits()[0].Split)
}

// TestSidecarV2MigratesToSplits: a hand-written version-2 sidecar (the
// files existing players have) restores every rect and open flag exactly,
// leaves the seams at their equal-share defaults, and is rewritten as
// version 3 on the next save.
func TestSidecarV2MigratesToSplits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	if err := os.WriteFile(path+".layout.json", []byte(`{"version":2,"windows":{
		"overview":{"x":200,"y":100,"w":400,"h":300,"open":true},
		"inventory":{"x":120,"y":140,"w":320,"h":440,"open":true},
		"factory":{"x":300,"y":200,"w":500,"h":400,"open":false}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	g := newShowcaseGameAt(t, path)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)

	if got, want := bodyOf[*uikit.Inventory](g.host, idInventory).Win().GetContainer().GetWidget().Rect, image.Rect(120, 140, 440, 580); got != want {
		t.Errorf("v2 inventory rect = %v, want the saved %v", got, want)
	}
	if !g.host.IsOpen(idInventory) {
		t.Error("v2 inventory open flag not restored")
	}
	// The rename migration still applies to v2 files (the saved 500×400
	// grows to the window's 420×440 content floor).
	if got, want := bodyOf[*uikit.Manufacturing](g.host, idManufacturing).Win().GetContainer().GetWidget().Rect, image.Rect(300, 200, 800, 640); got != want {
		t.Errorf("v2 factory rect migrated to %v, want %v", got, want)
	}
	if g.host.IsOpen(idManufacturing) {
		t.Error("v2 manufacturing open flag not restored (want closed)")
	}
	for _, sm := range g.host.Slot(string(idInventory)).Splits() {
		if got, want := sm.Split.Ratio(sm.Index), float64(sm.Index+1)/3; got != want {
			t.Errorf("v2 %s = %v, want the equal-share default %v", sm.Name, got, want)
		}
	}
	// The fitting split opens at 0.6: its tile grid needs the taller slots
	// pane at the window's frozen height (see screens.NewFitting).
	if got := seamRatio("fitting-hangar", g.host.Slot(string(idFitting)).Splits()); got != 0.6 {
		t.Errorf("v2 fitting-hangar = %v, want the 0.6 slots share", got)
	}

	// The next write is version 3, rects and flags unchanged.
	if err := g.writeLayout(); err != nil {
		t.Fatalf("writeLayout: %v", err)
	}
	doc := readDoc(t, g.layoutPath())
	if doc.Version != 3 {
		t.Errorf("rewritten sidecar version = %d, want 3", doc.Version)
	}
	if e := doc.Windows["inventory"]; e.X != 120 || e.Y != 140 || e.W != 320 || e.H != 440 || !e.Open {
		t.Errorf("rewritten inventory = %+v, want the v2 rect and flag kept", e)
	}
	if got := doc.Windows["inventory"].Splits["inventory-ore"]; got != float64(1)/3 {
		t.Errorf("rewritten inventory-ore = %v, want the default %v", got, float64(1)/3)
	}
	if got := doc.Windows["fitting"].Splits["fitting-hangar"]; got != 0.6 {
		t.Errorf("rewritten fitting-hangar = %v, want the 0.6 slots share", got)
	}
}

// TestSplitRatioOutOfRangeClamps: a corrupt sidecar ratio (far negative,
// far over one) restores clamped to the widget's sane band, never a
// collapsed or negative pane.
func TestSplitRatioOutOfRangeClamps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	if err := os.WriteFile(path+".layout.json", []byte(`{"version":3,"windows":{
		"fitting":{"x":700,"y":260,"w":300,"h":430,"open":true,
			"splits":{"fitting-hangar":2}},
		"manufacturing":{"x":400,"y":120,"w":420,"h":440,"open":false,
			"splits":{"manufacturing-jobs":-3}}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	g := newShowcaseGameAt(t, path)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)

	if got := seamRatio("fitting-hangar", g.host.Slot(string(idFitting)).Splits()); got != widgets.SplitMaxRatio {
		t.Errorf("fitting-hangar = %v, want the clamped max %v", got, widgets.SplitMaxRatio)
	}
	if got := seamRatio("manufacturing-jobs", g.host.Slot(string(idManufacturing)).Splits()); got != widgets.SplitMinRatio {
		t.Errorf("manufacturing-jobs = %v, want the clamped min %v", got, widgets.SplitMinRatio)
	}
	// Fitting restores open, so its split is laid out; manufacturing is
	// docked-only and closed here, so only its stored ratio is checked.
	assertSplitTiled(t, "fitting", g.host.Slot(string(idFitting)).Splits()[0].Split)
}

// assertSplitRowTiled: a row split's panes and hairlines fill its rect
// across the width — no pane collapsed, no gap (the horizontal twin of
// assertSplitTiled).
func assertSplitRowTiled(t *testing.T, name string, split *widgets.SplitPane) {
	t.Helper()
	root := split.Container().GetWidget().Rect
	total := 0
	for _, ch := range split.Container().Children() {
		w := ch.GetWidget().Rect.Dx()
		if w == 0 {
			t.Errorf("%s: a split column is 0 px wide (%v)", name, ch.GetWidget().Rect)
		}
		total += w
	}
	if total != root.Dx() {
		t.Errorf("%s: split columns fill %d px, want the %d px rect", name, total, root.Dx())
	}
}

// TestMarketSplitRatioPersists: the market's two seams (tree | catalog
// and catalog | My orders) are exposed to the sidecar as market-tree and
// market-orders, round-trip their ratios through a version-3 file, restore
// them on a fresh session (tiling at the 1024 minimum), and "Reset window
// layout" returns each to the panel's own opening share — 176 px of browse
// and the old 240 px orders column of the 760 px landing rect.
func TestMarketSplitRatioPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	g := newShowcaseGameAt(t, path)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)

	// The opening shares: 176 and (176+320) of the 736 px the split spans
	// at the 760 px landing rect (760 − 2·ContentsPad − both 4 px seams).
	const treeOpening = 176.0 / 736
	const ordersOpening = (176.0 + 320) / 736
	seams := g.host.Slot(string(idMarket)).Splits()
	if len(seams) != 2 || seams[0].Name != "market-tree" || seams[1].Name != "market-orders" {
		t.Fatalf("market seams = %+v, want market-tree then market-orders", seams)
	}
	if got := seamRatio("market-tree", seams); math.Abs(got-treeOpening) > 1e-9 {
		t.Errorf("opening market-tree ratio = %v, want %v", got, treeOpening)
	}
	if got := seamRatio("market-orders", seams); math.Abs(got-ordersOpening) > 1e-9 {
		t.Errorf("opening market-orders ratio = %v, want %v", got, ordersOpening)
	}

	seams[0].Split.SetRatio(seams[0].Index, 0.32)
	seams[1].Split.SetRatio(seams[1].Index, 0.71)
	if err := g.SaveGame(); err != nil {
		t.Fatalf("SaveGame: %v", err)
	}
	doc := readDoc(t, g.layoutPath())
	if doc.Version != 3 {
		t.Fatalf("sidecar version = %d, want 3", doc.Version)
	}
	if got := doc.Windows["market"].Splits["market-tree"]; math.Abs(got-0.32) > 1e-9 {
		t.Errorf("market-tree = %v, want the dragged 0.32", got)
	}
	if got := doc.Windows["market"].Splits["market-orders"]; math.Abs(got-0.71) > 1e-9 {
		t.Errorf("market-orders = %v, want the dragged 0.71", got)
	}

	// Next session: the ratios come back, mounted and laid out; a resize
	// still tiles the row.
	g2 := newShowcaseGameAt(t, path)
	g2.applySize(1280, 720)
	g2.host.SetOpen(idMarket, true)
	g2.syncMounts()
	pump(t, g2, 1280, 720)
	if got := seamRatio("market-tree", g2.host.Slot(string(idMarket)).Splits()); math.Abs(got-0.32) > 1e-9 {
		t.Errorf("restored market-tree = %v, want 0.32", got)
	}
	if got := seamRatio("market-orders", g2.host.Slot(string(idMarket)).Splits()); math.Abs(got-0.71) > 1e-9 {
		t.Errorf("restored market-orders = %v, want 0.71", got)
	}
	assertSplitRowTiled(t, "market", g2.host.Slot(string(idMarket)).Splits()[0].Split)
	g2.applySize(1024, 700)
	pump(t, g2, 1024, 700)
	assertSplitRowTiled(t, "market", g2.host.Slot(string(idMarket)).Splits()[0].Split)

	// Reset window layout: back to the market's opening shares (176 px of
	// browse, the old 240 px column), not the widget's equal thirds.
	g2.resetLayout()
	seams = g2.host.Slot(string(idMarket)).Splits()
	if got := seamRatio("market-tree", seams); math.Abs(got-treeOpening) > 1e-9 {
		t.Errorf("after reset market-tree = %v, want the opening share %v", got, treeOpening)
	}
	if got := seamRatio("market-orders", seams); math.Abs(got-ordersOpening) > 1e-9 {
		t.Errorf("after reset market-orders = %v, want the opening share %v", got, ordersOpening)
	}
}

// TestMarketSplitV2Default: a version-2 sidecar (no splits) restores the
// market geometry and leaves its seam at the opening share; the next
// write is version 3 and records that share.
func TestMarketSplitV2Default(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	if err := os.WriteFile(path+".layout.json", []byte(`{"version":2,"windows":{
		"market":{"x":97,"y":8,"w":760,"h":560,"open":true}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	const opening = (176.0 + 320) / 736

	g := newShowcaseGameAt(t, path)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)
	if got := seamRatio("market-orders", g.host.Slot(string(idMarket)).Splits()); math.Abs(got-opening) > 1e-9 {
		t.Errorf("v2 market-orders = %v, want the opening share %v", got, opening)
	}
	if err := g.writeLayout(); err != nil {
		t.Fatalf("writeLayout: %v", err)
	}
	doc := readDoc(t, g.layoutPath())
	if doc.Version != 3 {
		t.Errorf("rewritten sidecar version = %d, want 3", doc.Version)
	}
	if got := doc.Windows["market"].Splits["market-orders"]; math.Abs(got-opening) > 1e-9 {
		t.Errorf("rewritten market-orders = %v, want the opening share %v", got, opening)
	}
}
