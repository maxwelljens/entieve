package game

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	uikit "github.com/entieve/entieve/uikit/screens"
)

// Pins for the docked service trio's sidecar behavior (the stacked-
// windows regression). The trio's composed tuples are pinned by
// TestStationLayoutDisjoint1024/1280/1920; because Market/Inventory/
// Fitting do not depend on stationH, stationLayout(w, h, anything)
// yields the same trio (TestStationLayoutDegenerateStationH pins that).

// trioActual returns the live rects of the three service windows.
func trioActual(g *Game) dockedLayout {
	return dockedLayout{
		Market:    g.mkt.Win().GetContainer().GetWidget().Rect,
		Inventory: g.inv.Win().GetContainer().GetWidget().Rect,
		Fitting:   g.fitt.Win().GetContainer().GetWidget().Rect,
	}
}

// assertTrioTuples checks the three service windows against the exact
// composed tuples and the invariants: pairwise disjoint, fully on-screen
// at w×h and at/above each window's MinSize.
func assertTrioTuples(t *testing.T, stage string, got dockedLayout, want dockedLayout, w, h int) {
	t.Helper()
	screen := image.Rect(0, 0, w, h)
	for _, c := range []struct {
		name string
		got  image.Rectangle
		want image.Rectangle
		min  image.Point
	}{
		{"market", got.Market, want.Market, image.Point{X: marketMinW, Y: marketMinH}},
		{"inventory", got.Inventory, want.Inventory, image.Point{X: inventoryMinW, Y: inventoryMinH}},
		{"fitting", got.Fitting, want.Fitting, image.Point{X: fittingMinW, Y: fittingMinH}},
	} {
		if c.got != c.want {
			t.Errorf("%s: %s = %v, want the composed %v", stage, c.name, c.got, c.want)
		}
		if !c.got.In(screen) {
			t.Errorf("%s: %s %v is not fully on screen at %d×%d", stage, c.name, c.got, w, h)
		}
		if c.got.Dx() < c.min.X || c.got.Dy() < c.min.Y {
			t.Errorf("%s: %s %v is %d×%d, below MinSize %d×%d",
				stage, c.name, c.got, c.got.Dx(), c.got.Dy(), c.min.X, c.min.Y)
		}
	}
	for _, c := range []struct {
		a, b   image.Rectangle
		an, bn string
	}{
		{got.Market, got.Inventory, "market", "inventory"},
		{got.Market, got.Fitting, "market", "fitting"},
		{got.Inventory, got.Fitting, "inventory", "fitting"},
	} {
		if c.a.Overlaps(c.b) {
			t.Errorf("%s: %s %v overlaps %s %v", stage, c.an, c.a, c.bn, c.b)
		}
	}
}

// dockAndFrame opens the service trio the way the player now does (the
// rail or their keys — docking itself opens only the services view),
// docks, and runs the production frame pipeline (Update reconciles the
// scene stack and mounts itself), then checks the StationScene is
// actually active with the services view AND the opened trio mounted —
// the composition places whatever the player opened.
func dockAndFrame(t *testing.T, g *Game, w, h int) {
	t.Helper()
	for _, p := range stationPanels {
		g.open[p] = true
	}
	dockAt(t, g)
	pump(t, g, w, h)
	if !g.findScene(func(s Scene) bool { _, ok := s.(*StationScene); return ok }) {
		t.Fatal("docked: StationScene is not on the scene stack after a frame")
	}
	for _, p := range stationPanels {
		if g.mounted[p] == nil {
			t.Errorf("docked: %s is not mounted after a frame", panelNames[p])
		}
	}
}

// TestDockedTrioDisjointEndToEnd: after a real frame with the docked
// scene active, the three service windows' ACTUAL rects are the exact
// composed tuples, pairwise disjoint, on-screen and at/above MinSize at
// 1024×700, 1280×720 and 1920×1080 — for a fresh session AND for a
// same-size sidecar round-trip (the application path that regressed:
// sidecar present + dock).
func TestDockedTrioDisjointEndToEnd(t *testing.T) {
	for _, size := range [][2]int{{1024, 700}, {1280, 720}, {1920, 1080}} {
		w, h := size[0], size[1]
		want := stationLayout(w, h, stationBoardWorstCaseH)
		t.Run(fmt.Sprintf("%dx%d", w, h), func(t *testing.T) {
			// Fresh session: no sidecar, compose, dock, real frames.
			g := newShowcaseGame(t)
			g.applySize(w, h)
			dockAndFrame(t, g, w, h)
			assertTrioTuples(t, "fresh docked", trioActual(g), want, w, h)

			// Round-trip: quit writes this session's layout, a fresh
			// session on the same path restores it and docks.
			if err := g.writeLayout(); err != nil {
				t.Fatalf("writeLayout: %v", err)
			}
			g2 := newShowcaseGameAt(t, g.SavePath())
			g2.applySize(w, h)
			dockAndFrame(t, g2, w, h)
			assertTrioTuples(t, "round-trip docked", trioActual(g2), want, w, h)
			for _, c := range []struct {
				name string
				win  *widget.Window
			}{{"market", g2.mkt.Win()}, {"inventory", g2.inv.Win()}, {"fitting", g2.fitt.Win()}} {
				if !g2.restored[c.win] {
					t.Errorf("round-trip docked: %s lost its restored flag (saved wins broken)", c.name)
				}
			}
		})
	}
}

// TestStationTrioSidecarRoundTrip: dock → quit (writeLayout) → fresh
// Game (readLayout) → the three windows are still disjoint at their
// saved positions.
func TestStationTrioSidecarRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	g := newShowcaseGameAt(t, path)
	g.applySize(1280, 720)
	dockAndFrame(t, g, 1280, 720)
	saved := trioActual(g)
	if err := g.writeLayout(); err != nil {
		t.Fatalf("writeLayout: %v", err)
	}

	g2 := newShowcaseGameAt(t, path)
	g2.applySize(1280, 720)
	dockAndFrame(t, g2, 1280, 720)
	got := trioActual(g2)
	if got != saved {
		t.Errorf("round-trip trio = %+v, want the saved %+v", got, saved)
	}
	assertTrioTuples(t, "round-trip", got,
		stationLayout(1280, 720, stationBoardWorstCaseH), 1280, 720)
}

// writeTrioSidecar writes a sidecar whose market/inventory/fitting
// entries are all rect (the user's real symptom: three services stacked
// on one rect), current version included.
func writeTrioSidecar(t *testing.T, path string, rect image.Rectangle) {
	t.Helper()
	entry := `{"x":` + strconv.Itoa(rect.Min.X) + `,"y":` + strconv.Itoa(rect.Min.Y) +
		`,"w":` + strconv.Itoa(rect.Dx()) + `,"h":` + strconv.Itoa(rect.Dy()) + `,"open":false}`
	doc := `{"version":` + strconv.Itoa(layoutVersion) + `,"windows":{` +
		`"market":` + entry + `,"inventory":` + entry + `,"fitting":` + entry + `}}`
	if err := os.WriteFile(path+".layout.json", []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestStationTrioStackedSidecarReclaimed: a sidecar with the three
// service windows stacked on one rect — the user's real symptom — must
// converge to the composed tiling. Decision rule: the sidecar's service
// GEOMETRY is honored only while it restores pairwise disjoint; two or
// more restored service entries that overlap are stale (a clamp fold or
// a bad build's write) and are dropped, while the open/closed flags are
// still honored. The trio then composes from stationLayout.
func TestStationTrioStackedSidecarReclaimed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	// All three saved on one rect, exactly what the collapsed
	// sidecar carried (see the clamp-fold tests below for how it
	// got written).
	writeTrioSidecar(t, path, image.Rect(396, 8, 396+874, 8+240))

	g := newShowcaseGameAt(t, path)
	g.applySize(1280, 720)
	want := stationLayout(1280, 720, stationBoardWorstCaseH)
	assertTrioTuples(t, "stacked sidecar restored", trioActual(g), want, 1280, 720)
	for _, c := range []struct {
		name string
		win  *widget.Window
	}{{"market", g.mkt.Win()}, {"inventory", g.inv.Win()}, {"fitting", g.fitt.Win()}} {
		if g.restored[c.win] {
			t.Errorf("stacked sidecar: %s still flagged restored (stale geometry pinned)", c.name)
		}
	}

	dockAndFrame(t, g, 1280, 720)
	assertTrioTuples(t, "stacked sidecar docked", trioActual(g), want, 1280, 720)

	// The writer must not re-persist the collapse: the sidecar written
	// after the reclaim carries disjoint service entries again.
	if err := g.writeLayout(); err != nil {
		t.Fatalf("writeLayout: %v", err)
	}
	g.applyLayout(1280, 720)
	assertTrioTuples(t, "re-restored after rewrite", trioActual(g), want, 1280, 720)
}

// writeBigTrioSidecar writes the composed 1920×1080 trio — a perfectly
// valid, pairwise-disjoint arrangement — as the sidecar.
func writeBigTrioSidecar(t *testing.T, path string) {
	t.Helper()
	lay := stationLayout(1920, 1080, stationBoardWorstCaseH)
	e := func(r image.Rectangle) string {
		return `{"x":` + strconv.Itoa(r.Min.X) + `,"y":` + strconv.Itoa(r.Min.Y) +
			`,"w":` + strconv.Itoa(r.Dx()) + `,"h":` + strconv.Itoa(r.Dy()) + `,"open":false}`
	}
	doc := `{"version":` + strconv.Itoa(layoutVersion) + `,"windows":{` +
		`"market":` + e(lay.Market) + `,"inventory":` + e(lay.Inventory) +
		`,"fitting":` + e(lay.Fitting) + `}}`
	if err := os.WriteFile(path+".layout.json", []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestStationTrioClampFoldReclaimed: a sidecar saved at a LARGER
// viewport (valid, disjoint 1920×1080 rects) restored at 1024×700 used
// to clamp each window independently into the same corner — inventory
// and fitting landed on the identical rect (271,0)-(1024,700) — and the
// restored flag pinned the fold forever. Now the overlap is detected at
// restore and the trio recomposes to the exact 1024 tuples.
func TestStationTrioClampFoldReclaimed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	writeBigTrioSidecar(t, path)

	g := newShowcaseGameAt(t, path)
	g.applySize(1024, 700)
	want := stationLayout(1024, 700, stationBoardWorstCaseH)
	assertTrioTuples(t, "1920 sidecar restored at 1024", trioActual(g), want, 1024, 700)

	dockAndFrame(t, g, 1024, 700)
	assertTrioTuples(t, "1920 sidecar docked at 1024", trioActual(g), want, 1024, 700)
}

// TestStationTrioShrinkFoldReclaimed: mid-session viewport shrink —
// clampRestored used to slide the restored 1920 trio into the same
// corner at 1024×700 (and quit then persisted the folded rects back).
// While the saved arrangement fits (1920) it wins; once the shrink
// folds it, the trio re-tiles to the composed 1024 tuples.
func TestStationTrioShrinkFoldReclaimed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	writeBigTrioSidecar(t, path)

	g := newShowcaseGameAt(t, path)
	g.applySize(1920, 1080)
	saved := trioActual(g)
	// Saved wins while it fits: the restored 1920 rects are kept.
	big := stationLayout(1920, 1080, stationBoardWorstCaseH)
	if saved.Market != big.Market || saved.Inventory != big.Inventory || saved.Fitting != big.Fitting {
		t.Fatalf("at 1920 the saved trio = %+v, want it honored", saved)
	}
	for _, win := range []*widget.Window{g.mkt.Win(), g.inv.Win(), g.fitt.Win()} {
		if !g.restored[win] {
			t.Fatal("at 1920 the service entries lost their restored flag")
		}
	}

	g.applySize(1024, 700)
	pump(t, g, 1024, 700)
	want := stationLayout(1024, 700, stationBoardWorstCaseH)
	assertTrioTuples(t, "after shrink 1920→1024", trioActual(g), want, 1024, 700)

	// The fold must not be persisted back: a rewrite + re-read keeps
	// the trio composed, not collapsed.
	if err := g.writeLayout(); err != nil {
		t.Fatalf("writeLayout: %v", err)
	}
	g2 := newShowcaseGameAt(t, path)
	g2.applySize(1024, 700)
	assertTrioTuples(t, "sidecar rewritten after shrink", trioActual(g2), want, 1024, 700)
}

// TestStationLayoutDegenerateStationH: stationH feeds only the Station
// column rect (max(stationH, stationMinH)); the service trio is a pure
// function of w×h. StationH=0 floors at stationMinH; a huge stationH
// only stretches the Station column (its height comes from the board
// content hug, bounded by the row caps — worst case 426).
func TestStationLayoutDegenerateStationH(t *testing.T) {
	for _, size := range [][2]int{{1024, 700}, {1280, 720}, {1920, 1080}} {
		w, h := size[0], size[1]
		base := stationLayout(w, h, stationBoardWorstCaseH)
		for _, c := range []struct {
			name     string
			stationH int
		}{{"zero", 0}, {"huge", 1 << 20}} {
			lay := stationLayout(w, h, c.stationH)
			if lay.Market != base.Market || lay.Inventory != base.Inventory || lay.Fitting != base.Fitting {
				t.Errorf("%dx%d stationH=%d (%s): trio changed: %+v, want %+v",
					w, h, c.stationH, c.name,
					dockedLayout{Market: lay.Market, Inventory: lay.Inventory, Fitting: lay.Fitting},
					dockedLayout{Market: base.Market, Inventory: base.Inventory, Fitting: base.Fitting})
			}
			wantStationH := max(c.stationH, stationMinH)
			if lay.Station.Dy() != wantStationH {
				t.Errorf("%dx%d stationH=%d (%s): Station height = %d, want %d",
					w, h, c.stationH, c.name, lay.Station.Dy(), wantStationH)
			}
			if lay.Station.Min != image.Pt(stationX, stationY) {
				t.Errorf("%dx%d stationH=%d (%s): Station origin = %v, want (%d,%d)",
					w, h, c.stationH, c.name, lay.Station.Min, stationX, stationY)
			}
		}
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

	g.help.Win().GetContainer().GetWidget().Rect = image.Rectangle{}
	if err := g.writeLayout(); err != nil {
		t.Fatalf("writeLayout: %v", err)
	}
	e := readDoc(t, g.layoutPath()).Windows["help"]
	if e.W <= 0 || e.H <= 0 {
		t.Errorf("help persisted as %+v: degenerate %d×%d geometry", e, e.W, e.H)
	}
	if min := g.help.Win().MinSize; min != nil && (e.W < min.X || e.H < min.Y) {
		t.Errorf("help persisted as %+v, below MinSize %v", e, min)
	}
}

// TestStaleLayoutVersionIgnored: version 1 files (written by builds
// predating the service-trio reclaim — their service entries may carry
// the collapsed fold) are ignored wholesale: defaults win instead of
// the stale geometry being pinned as saved-wins.
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
	if r := g.kit.OverviewWindow().GetContainer().GetWidget().Rect; r != image.Rect(8+uikit.RailInset, 8, 388+uikit.RailInset, 428) {
		t.Errorf("version-1 sidecar was applied: overview = %v, want the default clear of the rail", r)
	}
	assertTrioTuples(t, "version-1 sidecar ignored", trioActual(g),
		stationLayout(1280, 720, stationBoardWorstCaseH), 1280, 720)
}
