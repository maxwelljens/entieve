package game

import (
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// The composed docked layout, pinned at the three design sizes. The
// station rect is the board's worst case (both sections capped); at
// runtime the station hugs whatever the live board needs and the test
// compares against stationLayout(w, h, thatHeight).

// stationBoardWorstCaseH is the docked contract board's content height
// with every section at its row cap: the station window then reads
// (10,8)-(404,434), its worst case.
const stationBoardWorstCaseH = 426

// logPocket is the log's default bottom-left pocket at any design size:
// 288×240 flush into the 10 px left/bottom margins (uikit's
// logDefaultRect). The log is the one anchored chrome window that stays
// on screen while docked, so the composed set must stay clear of it.
func logPocket(h int) image.Rectangle {
	return image.Rect(10, h-10-240, 10+288, h-10)
}

// TestStationLayoutDisjoint1024 pins the composed rect tuples at the
// minimum window and checks the invariants: pairwise disjoint, fully
// on-screen, at least each window's MinSize and clear of the log pocket.
//
// (The HUD bar and the scanner pockets belong to the space view — both
// are hidden/unmounted while docked — so the composed set may sit over
// them; see the clearance note in station_layout.go.)
func TestStationLayoutDisjoint1024(t *testing.T) {
	checkStationLayout(t, 1024, 700, dockedLayout{
		Station:   image.Rect(46, 8, 404, 434), // 46 = 10 + the rail gutter
		Market:    image.Rect(406, 8, 1014, 248),
		Inventory: image.Rect(406, 256, 706, 690),
		Fitting:   image.Rect(714, 256, 1014, 690),
	})
}

// TestStationLayoutDisjoint1280 pins the default window size.
func TestStationLayoutDisjoint1280(t *testing.T) {
	checkStationLayout(t, 1280, 720, dockedLayout{
		Station:   image.Rect(46, 8, 404, 434), // 46 = 10 + the rail gutter
		Market:    image.Rect(406, 8, 1270, 248),
		Inventory: image.Rect(406, 256, 834, 710),
		Fitting:   image.Rect(842, 256, 1270, 710),
	})
}

// TestStationLayoutDisjoint1920 pins a large viewport: the market band
// grows with the height, the bottom row keeps its content minimum.
func TestStationLayoutDisjoint1920(t *testing.T) {
	checkStationLayout(t, 1920, 1080, dockedLayout{
		Station:   image.Rect(46, 8, 404, 434), // 46 = 10 + the rail gutter
		Market:    image.Rect(406, 8, 1910, 359),
		Inventory: image.Rect(406, 367, 1154, 1070),
		Fitting:   image.Rect(1162, 367, 1910, 1070),
	})
}

// checkStationLayout asserts the exact tuples and every layout invariant
// for one viewport.
func checkStationLayout(t *testing.T, w, h int, want dockedLayout) {
	t.Helper()
	got := stationLayout(w, h, stationBoardWorstCaseH)
	for _, c := range []struct {
		name string
		got  image.Rectangle
		want image.Rectangle
	}{
		{"station", got.Station, want.Station},
		{"market", got.Market, want.Market},
		{"inventory", got.Inventory, want.Inventory},
		{"fitting", got.Fitting, want.Fitting},
	} {
		if c.got != c.want {
			t.Errorf("%dx%d: %s = %v, want %v", w, h, c.name, c.got, c.want)
		}
	}

	screen := image.Rect(0, 0, w, h)
	log := logPocket(h)
	rects := []struct {
		name string
		r    image.Rectangle
		min  image.Point
	}{
		{"station", got.Station, image.Point{X: stationMinW, Y: stationMinH}},
		{"market", got.Market, image.Point{X: marketMinW, Y: marketMinH}},
		{"inventory", got.Inventory, image.Point{X: inventoryMinW, Y: inventoryMinH}},
		{"fitting", got.Fitting, image.Point{X: fittingMinW, Y: fittingMinH}},
	}
	for i, a := range rects {
		if !a.r.In(screen) {
			t.Errorf("%dx%d: %s %v is not fully on screen", w, h, a.name, a.r)
		}
		if a.r.Dx() < a.min.X || a.r.Dy() < a.min.Y {
			t.Errorf("%dx%d: %s %v is %d×%d, below its MinSize %d×%d",
				w, h, a.name, a.r, a.r.Dx(), a.r.Dy(), a.min.X, a.min.Y)
		}
		if a.r.Overlaps(log) {
			t.Errorf("%dx%d: %s %v overlaps the log pocket %v", w, h, a.name, a.r, log)
		}
		for _, b := range rects[i+1:] {
			if a.r.Overlaps(b.r) {
				t.Errorf("%dx%d: %s %v overlaps %s %v", w, h, a.name, a.r, b.name, b.r)
			}
		}
	}

	// Readability: the market band carries the trade columns. The
	// shipped boards measure ≈357 px of natural column width, so the
	// 560 px target leaves headroom — 618 px at the 1024×700 minimum,
	// 874 px at 1280 and 1510 px at 1920 (the achievable state at 1024
	// is a full, un-truncated table; the truncation the old fixed
	// 300 px rect caused is gone at every design size).
	if got := got.Market.Dx(); got < 560 {
		t.Errorf("%dx%d: market band is %d px, want ≥ 560 for un-truncated columns", w, h, got)
	}
}

// TestWindowMinSizesMatch pins game's mirrored content minimums to the
// real uikit windows, so the composition can never drift below what a
// window actually accepts.
func TestWindowMinSizesMatch(t *testing.T) {
	g := newShowcaseGame(t)
	for _, c := range []struct {
		name string
		win  *widget.Window
		want image.Point
	}{
		{"market", g.mkt.Win(), image.Point{X: marketMinW, Y: marketMinH}},
		{"inventory", g.inv.Win(), image.Point{X: inventoryMinW, Y: inventoryMinH}},
		{"fitting", g.fitt.Win(), image.Point{X: fittingMinW, Y: fittingMinH}},
	} {
		if c.win.MinSize == nil || *c.win.MinSize != c.want {
			t.Errorf("%s MinSize = %v, want %v (mirror in station_layout.go)",
				c.name, c.win.MinSize, c.want)
		}
	}
}

// TestStationLayoutAppliedAtDesignSizes: the composed tuples are the
// rects the docked windows actually occupy — pumped at every design
// size, each window sits exactly on its composed rect, the set stays
// pairwise disjoint (the live log included), on screen and at or above
// each window's real MinSize.
func TestStationLayoutAppliedAtDesignSizes(t *testing.T) {
	for _, size := range [][2]int{{1024, 700}, {1280, 720}, {1920, 1080}} {
		w, h := size[0], size[1]
		g := newShowcaseGame(t)
		g.applySize(w, h)
		dockAt(t, g)
		g.syncScenes()
		g.syncMounts()
		pump(t, g, w, h)

		station := g.svcWin.GetContainer().GetWidget().Rect
		lay := stationLayout(w, h, station.Dy())
		got := dockedLayout{
			Station:   station,
			Market:    g.mkt.Win().GetContainer().GetWidget().Rect,
			Inventory: g.inv.Win().GetContainer().GetWidget().Rect,
			Fitting:   g.fitt.Win().GetContainer().GetWidget().Rect,
		}
		for _, c := range []struct {
			name string
			got  image.Rectangle
			want image.Rectangle
		}{
			{"station", got.Station, lay.Station},
			{"market", got.Market, lay.Market},
			{"inventory", got.Inventory, lay.Inventory},
			{"fitting", got.Fitting, lay.Fitting},
		} {
			if c.got != c.want {
				t.Errorf("%dx%d: %s window %v, composed %v", w, h, c.name, c.got, c.want)
			}
		}

		screen := image.Rect(0, 0, w, h)
		log := g.logP.Win().GetContainer().GetWidget().Rect
		rects := []struct {
			name string
			win  *widget.Window
			r    image.Rectangle
		}{
			{"station", g.svcWin, got.Station},
			{"market", g.mkt.Win(), got.Market},
			{"inventory", g.inv.Win(), got.Inventory},
			{"fitting", g.fitt.Win(), got.Fitting},
		}
		for i, a := range rects {
			if !a.r.In(screen) {
				t.Errorf("%dx%d: %s %v out of bounds", w, h, a.name, a.r)
			}
			if min := a.win.MinSize; min != nil &&
				(a.r.Dx() < min.X || a.r.Dy() < min.Y) {
				t.Errorf("%dx%d: %s %v below MinSize %v", w, h, a.name, a.r, min)
			}
			if a.r.Overlaps(log) {
				t.Errorf("%dx%d: %s %v overlaps the log %v", w, h, a.name, a.r, log)
			}
			for _, b := range rects[i+1:] {
				if a.r.Overlaps(b.r) {
					t.Errorf("%dx%d: %s %v overlaps %s %v", w, h, a.name, a.r, b.name, b.r)
				}
			}
		}
	}
}

// TestStationLayoutMarketFitsColumns: the composed market band is wide
// enough for the shipped market's columns to render whole — the trade
// table's natural width (measured exactly like uikit's fitColumns:
// widest cell or title per column + cell padding) always fits the band.
func TestStationLayoutMarketFitsColumns(t *testing.T) {
	g := newSmokeGame(t)
	g.applySize(1920, 1080)
	dockAt(t, g)
	g.syncScenes()
	g.syncMounts()
	pump(t, g, 1920, 1080)

	sys := g.s.System()
	if sys == nil {
		t.Fatal("no current system")
	}
	measure := func(s string) float64 {
		w, _ := text.Measure(s, *g.env.Face(theme.StepSmall), 0)
		return w
	}
	const cellPad = 5.0 // uikit's per-cell horizontal padding
	natural := []float64{
		measure("Item ↓") + 2*cellPad, // sorted column carries the ↓
		measure("Price") + 2*cellPad,
		measure("Qty") + 2*cellPad,
	}
	for _, st := range g.s.Market(sys.ID) {
		name := string(st.Item)
		if def, ok := g.opts.Content.Item(st.Item); ok {
			name = def.Name
		}
		for i, cell := range []string{name, theme.FormatCredits(st.Price), theme.FormatInt(int64(st.Qty))} {
			if w := measure(cell) + 2*cellPad; w > natural[i] {
				natural[i] = w
			}
		}
	}
	want := natural[0] + natural[1] + natural[2]
	for _, size := range [][2]int{{1024, 700}, {1280, 720}, {1920, 1080}} {
		band := stationLayout(size[0], size[1], stationBoardWorstCaseH).Market.Dx()
		// The table sits inside the window body's padding (2× contentsPad).
		if avail := float64(band - 10); avail < want {
			t.Errorf("%dx%d: market band %d leaves %.0f px for %.0f px of columns (truncation)",
				size[0], size[1], band, avail, want)
		}
	}
}

// TestStationLayoutSavedWins: a sidecar entry pins its window — a later
// viewport change must not recompose it — while windows the sidecar
// never mentions are virgin and do compose.
func TestStationLayoutSavedWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	// Partial sidecar: only the market, at a hand-arranged spot.
	if err := os.WriteFile(path+".layout.json", []byte(
		`{"version":2,"windows":{
			"market":{"x":400,"y":380,"w":360,"h":300,"open":true}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	g := newShowcaseGameAt(t, path)
	g.applySize(1280, 720)
	dockAt(t, g)
	g.syncScenes()
	g.syncMounts()
	pump(t, g, 1280, 720)

	saved := image.Rect(400, 380, 760, 680)
	if got := g.mkt.Win().GetContainer().GetWidget().Rect; got != saved {
		t.Fatalf("startup: market = %v, want the saved %v", got, saved)
	}
	// The sidecar says nothing about the inventory: it composes.
	if got, want := g.inv.Win().GetContainer().GetWidget().Rect,
		stationLayout(1280, 720, g.svcWin.GetContainer().GetWidget().Rect.Dy()).Inventory; got != want {
		t.Errorf("startup: virgin inventory = %v, want %v", got, want)
	}

	// Resize: the saved market stays put, the virgin windows recompose.
	g.applySize(1024, 700)
	pump(t, g, 1024, 700)
	lay := stationLayout(1024, 700, g.svcWin.GetContainer().GetWidget().Rect.Dy())
	if got := g.mkt.Win().GetContainer().GetWidget().Rect; got != saved {
		t.Errorf("after resize: market = %v, want the saved %v (sidecar wins)", got, saved)
	}
	for _, c := range []struct {
		name string
		win  *widget.Window
		want image.Rectangle
	}{
		{"inventory", g.inv.Win(), lay.Inventory},
		{"fitting", g.fitt.Win(), lay.Fitting},
	} {
		if got := c.win.GetContainer().GetWidget().Rect; got != c.want {
			t.Errorf("after resize: virgin %s = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestPlaceDefaultsKeepsUserMoves: recomposing for a new viewport only
// touches virgin windows — a dragged overview/log keep their rects,
// untouched ones (the centered help, the service trio) re-place.
func TestPlaceDefaultsKeepsUserMoves(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)

	// The player drags two windows off their defaults.
	movedOverview := image.Rect(120, 60, 500, 360) // 380×300 ≥ MinSize
	movedLog := image.Rect(640, 300, 1000, 540)    // 360×240 ≥ MinSize, on screen at 1024
	g.kit.OverviewWindow().SetLocation(movedOverview)
	g.logP.Win().SetLocation(movedLog)
	helpBefore := g.help.Win().GetContainer().GetWidget().Rect

	g.applySize(1024, 700)
	pump(t, g, 1024, 700)

	if got := g.kit.OverviewWindow().GetContainer().GetWidget().Rect; got != movedOverview {
		t.Errorf("user-moved overview = %v, want %v (resize stomped it)", got, movedOverview)
	}
	if got := g.logP.Win().GetContainer().GetWidget().Rect; got != movedLog {
		t.Errorf("user-moved log = %v, want %v (resize stomped it)", got, movedLog)
	}

	// Virgin windows re-place for the new viewport.
	lay := stationLayout(1024, 700, g.svcWin.GetContainer().GetWidget().Rect.Dy())
	if got, want := g.mkt.Win().GetContainer().GetWidget().Rect, lay.Market; got != want {
		t.Errorf("virgin market = %v, want the composed %v", got, want)
	}
	help := g.help.Win().GetContainer().GetWidget().Rect
	if want := (1024 - help.Dx()) / 2; help.Min.X != want {
		t.Errorf("virgin help x = %d, want %d (re-centered for 1024)", help.Min.X, want)
	}
	if help.Min.X == helpBefore.Min.X {
		t.Errorf("help did not move on resize (still at x=%d)", help.Min.X)
	}

	// Second resize: the held windows stay held, the virgin ones follow.
	g.applySize(1920, 1080)
	pump(t, g, 1920, 1080)
	if got := g.kit.OverviewWindow().GetContainer().GetWidget().Rect; got != movedOverview {
		t.Errorf("user-moved overview = %v after the second resize, want %v", got, movedOverview)
	}
	if got := g.logP.Win().GetContainer().GetWidget().Rect; got != movedLog {
		t.Errorf("user-moved log = %v after the second resize, want %v", got, movedLog)
	}
	lay = stationLayout(1920, 1080, g.svcWin.GetContainer().GetWidget().Rect.Dy())
	if got, want := g.inv.Win().GetContainer().GetWidget().Rect, lay.Inventory; got != want {
		t.Errorf("virgin inventory = %v, want the composed %v", got, want)
	}
}
