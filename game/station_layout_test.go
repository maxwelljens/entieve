package game

import (
	"image"
	"os"
	"path/filepath"
	"testing"
)

// Docked windows are ordinary windows now: the station view opens only the
// services view, and every service window the player opens lands on its
// own per-type default (its Screen row's DefaultRect + Anchor), exactly as
// it does in space. The band composition (stationLayout's market band and
// the inventory/fitting bottom row) is gone; stationRect is the only
// docked rect left, and it is just the services view's column.

// stationBoardWorstCaseH is the docked contract board's content height
// with every section at its row cap: the services column then reads
// (73,8)-(404,434), its worst case (73 = 10 + the rail gutter).
const stationBoardWorstCaseH = 426

// serviceDefaults are the three service windows' per-type defaults (their
// Screen rows), in design units == px at the identity scale: the market's
// three-pane 760×560 rect on the rail gutter's clear line (it overlaps the
// log pocket at the minimum — the big trade window wins that strip), the
// inventory rect and the fitting window's 644×682 column at the screen's
// top-right (the EVE fitting shape needs the room).
var serviceDefaults = map[panelID]image.Rectangle{
	pMarket:    image.Rect(97, 8, 857, 568),
	pInventory: image.Rect(396, 256, 701, 690),
	pFitting:   image.Rect(370, 8, 1014, 690),
}

// TestServiceDefaultsAreDeclared pins the spec rows themselves: each
// service window declares its per-type default rect (AnchorFixed). A
// change to a row's geometry must be deliberate.
func TestServiceDefaultsAreDeclared(t *testing.T) {
	for id, want := range serviceDefaults {
		row := screens[id]
		if row.DefaultRect != want {
			t.Errorf("%s DefaultRect = %v, want %v", row.Title, row.DefaultRect, want)
		}
		if row.Anchor != AnchorFixed {
			t.Errorf("%s Anchor = %v, want AnchorFixed", row.Title, row.Anchor)
		}
	}
	if mkt := serviceDefaults[pMarket]; mkt.Dx() != 760 || mkt.Dy() != 560 {
		t.Errorf("market default %v is not the three-pane 760×560", mkt)
	}
}

// TestServiceWindowsOpenAtTheirDefaults: docked, with the market,
// inventory and fitting opened the way the player does, each sits on its
// own per-type default at every design size — on screen, right of the
// rail gutter, clear of the log pocket and at or above its MinSize — not
// in a composed band.
func TestServiceWindowsOpenAtTheirDefaults(t *testing.T) {
	for _, size := range [][2]int{{1024, 700}, {1280, 720}, {1920, 1080}} {
		w, h := size[0], size[1]
		g := newShowcaseGame(t)
		g.applySize(w, h)
		dockAt(t, g)
		g.syncScenes()
		g.open[pMarket], g.open[pInventory], g.open[pFitting] = true, true, true
		g.syncMounts()
		pump(t, g, w, h)

		screen := image.Rect(0, 0, w, h)
		log := g.logP.Win().GetContainer().GetWidget().Rect
		floor := railClearX()
		for _, id := range []panelID{pMarket, pInventory, pFitting} {
			win := g.screenWin[id]
			r := win.GetContainer().GetWidget().Rect
			if want := serviceDefaults[id]; r != want {
				t.Errorf("%dx%d: %s at %v, want its per-type default %v",
					w, h, panelNames[id], r, want)
			}
			if !r.In(screen) {
				t.Errorf("%dx%d: %s %v is not fully on screen", w, h, panelNames[id], r)
			}
			if r.Min.X < floor {
				t.Errorf("%dx%d: %s %v starts inside the rail gutter (%d)", w, h, panelNames[id], r, floor)
			}
			// The market is the one window allowed over the log pocket:
			// its three panes need the width, and the log raises back on
			// a click (windows are a stack).
			if id != pMarket && r.Overlaps(log) {
				t.Errorf("%dx%d: %s %v overlaps the log pocket %v", w, h, panelNames[id], r, log)
			}
			if min := win.MinSize; min != nil && (r.Dx() < min.X || r.Dy() < min.Y) {
				t.Errorf("%dx%d: %s %v is below MinSize %v", w, h, panelNames[id], r, min)
			}
		}

		// The services view is the only docked rect: the station column.
		station := g.svcWin.GetContainer().GetWidget().Rect
		if want := stationRect(station.Dy()); station != want {
			t.Errorf("%dx%d: services view %v, want the station column %v", w, h, station, want)
		}
	}
}

// TestStationRect: stationRect is the services column — origin
// (stationX, stationY), width stationWinW, height max(stationH,
// stationMinH) — a pure function of the board height, floored at
// stationMinH and unbounded above (a huge board only stretches the
// column).
func TestStationRect(t *testing.T) {
	for _, h := range []int{0, stationMinH - 1, stationMinH, stationBoardWorstCaseH, 1 << 20} {
		got := stationRect(h)
		wantH := max(h, stationMinH)
		if got.Min != image.Pt(stationX, stationY) {
			t.Errorf("stationRect(%d) origin = %v, want (%d,%d)", h, got.Min, stationX, stationY)
		}
		if got.Dx() != stationWinW || got.Dy() != wantH {
			t.Errorf("stationRect(%d) = %v, want %dx%d", h, got, stationWinW, wantH)
		}
	}
}

// TestUndeclaredDefaultCenters: a Screen row with no declared position —
// a genuinely new window type: the zero DefaultRect and the zero Anchor
// (AnchorCenter) — opens centred, the order-window / Show-Info fallback.
// This is the one placement rule that must never drift to a corner.
func TestUndeclaredDefaultCenters(t *testing.T) {
	row := Screen{} // no DefaultRect, no Anchor
	for _, size := range [][2]int{{1280, 720}, {1024, 700}} {
		w, h := size[0], size[1]
		const dw, dh = 300, 240
		got := anchorRect(row.DefaultRect, row.Anchor, dw, dh, w, h)
		want := centerRect(dw, dh, w, h)
		if got != want {
			t.Errorf("%dx%d: undeclared default = %v, want the centred fallback %v", w, h, got, want)
		}
	}
}

// TestStationLayoutSavedWins: a sidecar entry pins its window — a later
// viewport change must not recompose it — while windows the sidecar never
// mentions are virgin and land on their per-type defaults.
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
	// The sidecar says nothing about the inventory: it lands on its own
	// per-type default.
	if got := g.inv.Win().GetContainer().GetWidget().Rect; got != serviceDefaults[pInventory] {
		t.Errorf("startup: virgin inventory = %v, want %v", got, serviceDefaults[pInventory])
	}

	// Resize: the saved market stays put, the virgin windows re-place.
	g.applySize(1024, 700)
	pump(t, g, 1024, 700)
	if got := g.mkt.Win().GetContainer().GetWidget().Rect; got != saved {
		t.Errorf("after resize: market = %v, want the saved %v (sidecar wins)", got, saved)
	}
	if got := g.fitt.Win().GetContainer().GetWidget().Rect; got != serviceDefaults[pFitting] {
		t.Errorf("after resize: virgin fitting = %v, want %v", got, serviceDefaults[pFitting])
	}
}

// TestPlaceDefaultsKeepsUserMoves: recomposing for a new viewport only
// touches virgin windows — a dragged overview/log keep their rects,
// untouched ones (the centered help, the service windows) re-place on
// their per-type defaults.
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

	// Virgin windows re-place on their per-type defaults.
	if got := g.mkt.Win().GetContainer().GetWidget().Rect; got != serviceDefaults[pMarket] {
		t.Errorf("virgin market = %v, want its per-type default %v", got, serviceDefaults[pMarket])
	}
	help := g.help.Win().GetContainer().GetWidget().Rect
	if want := max(railClearX(), (1024-help.Dx())/2); help.Min.X != want {
		t.Errorf("virgin help x = %d, want %d (re-centred for 1024, floored past the rail gutter)", help.Min.X, want)
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
	if got := g.inv.Win().GetContainer().GetWidget().Rect; got != serviceDefaults[pInventory] {
		t.Errorf("virgin inventory = %v, want its per-type default %v", got, serviceDefaults[pInventory])
	}
}

// TestSavedServiceOverlapHonored: every window is ordinary now, so a
// sidecar that parks two service windows on one rect restores exactly
// that — your saved arrangement wins, there is no docked-set reclaim any
// more (the old stacked-trio symptom is now just a hand-arranged
// overlap).
func TestSavedServiceOverlapHonored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	// Both at the same rect, ≥ each window's MinSize and on screen at 1280.
	if err := os.WriteFile(path+".layout.json", []byte(
		`{"version":2,"windows":{`+
			`"market":{"x":400,"y":200,"w":360,"h":420,"open":false},`+
			`"inventory":{"x":400,"y":200,"w":360,"h":420,"open":false}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	g := newShowcaseGameAt(t, path)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)

	overlap := image.Rect(400, 200, 760, 620)
	for _, id := range []panelID{pMarket, pInventory} {
		if got := g.screenWin[id].GetContainer().GetWidget().Rect; got != overlap {
			t.Errorf("%s = %v, want the saved overlapping rect %v (saved wins)",
				panelNames[id], got, overlap)
		}
	}
}
