package ui

import (
	"testing"

	"github.com/entieve/entieve/core"
)

// designSizes are the two window sizes every layout property must hold at:
// the default and the minimum supported window.
var designSizes = [][2]float64{{1280, 720}, {1024, 700}}

// baseRegions are the always-visible HUD windows (overlays excluded: they
// sit above the base HUD by design).
func baseRegions(r Regions) map[string]Rect {
	return map[string]Rect{
		"Selected": r.Selected,
		"Overview": r.Overview,
		"Targets":  r.Targets,
		"HUD":      r.HUD,
		"Modules":  r.Modules,
		"Log":      r.Log,
		"Scanner":  r.Scanner,
	}
}

func allRegions(r Regions) map[string]Rect {
	all := baseRegions(r)
	all["Inventory"] = r.Inventory
	all["Fitting"] = r.Fitting
	all["Station"] = r.Station
	all["Starmap"] = r.Starmap
	all["Help"] = r.Help
	return all
}

// TestRegionsFitScreen: every window fits inside the screen at both the
// design size and the minimum supported size.
func TestRegionsFitScreen(t *testing.T) {
	for _, size := range designSizes {
		w, h := size[0], size[1]
		reg := ComputeRegions(w, h)
		for name, r := range allRegions(reg) {
			if r.X < 0 || r.Y < 0 || r.Right() > w+0.001 || r.Bottom() > h+0.001 {
				t.Errorf("%vx%v: %s rect %v escapes the screen", w, h, name, r)
			}
			if r.W <= 0 || r.H <= 0 {
				t.Errorf("%vx%v: %s rect %v has non-positive size", w, h, name, r)
			}
		}
	}
}

// TestBaseRegionsDoNotOverlap: the base HUD panels plus the scanner's
// default position never overlap (1280×720 and 1024×700).
func TestBaseRegionsDoNotOverlap(t *testing.T) {
	names := []string{"Selected", "Overview", "Targets", "HUD", "Modules", "Log", "Scanner"}
	for _, size := range designSizes {
		reg := ComputeRegions(size[0], size[1])
		base := baseRegions(reg)
		for i := 0; i < len(names); i++ {
			for j := i + 1; j < len(names); j++ {
				a, b := base[names[i]], base[names[j]]
				if a.Overlaps(b) {
					t.Errorf("%vx%v: %s %v overlaps %s %v", size[0], size[1],
						names[i], a, names[j], b)
				}
			}
		}
	}
}

// TestFittingDoesNotCoverOverview: the fitting window (default position)
// must never cover the overview column — the explicit UX requirement.
func TestFittingDoesNotCoverOverview(t *testing.T) {
	for _, size := range designSizes {
		reg := ComputeRegions(size[0], size[1])
		if reg.Fitting.Overlaps(reg.Overview) {
			t.Errorf("%vx%v: fitting %v overlaps overview %v",
				size[0], size[1], reg.Fitting, reg.Overview)
		}
		if reg.Fitting.Overlaps(reg.Selected) {
			t.Errorf("%vx%v: fitting %v overlaps selected %v",
				size[0], size[1], reg.Fitting, reg.Selected)
		}
	}
}

// TestStarmapCentered: the starmap overlay is centered on screen and fits.
func TestStarmapCentered(t *testing.T) {
	for _, size := range designSizes {
		reg := ComputeRegions(size[0], size[1])
		sm := reg.Starmap
		if d := (sm.X + sm.W/2) - size[0]/2; d > 0.5 || d < -0.5 {
			t.Errorf("%vx%v: starmap x-center off by %v", size[0], size[1], d)
		}
		if d := (sm.Y + sm.H/2) - size[1]/2; d > 0.5 || d < -0.5 {
			t.Errorf("%vx%v: starmap y-center off by %v", size[0], size[1], d)
		}
		if sm.Right() > size[0] || sm.Bottom() > size[1] {
			t.Errorf("%vx%v: starmap %v escapes screen", size[0], size[1], sm)
		}
	}
}

// TestOverviewRowRectsInsidePanel: header + rows + F-key labels stay within
// their panels at both sizes.
func TestOverviewRowRectsInsidePanel(t *testing.T) {
	for _, size := range designSizes {
		reg := ComputeRegions(size[0], size[1])
		r := reg.Overview
		if OverviewMaxRows(r) <= 0 {
			t.Fatalf("%vx%v: no overview rows fit", size[0], size[1])
		}
		for i := 0; i < OverviewMaxRows(r); i++ {
			row := OverviewRowRect(r, i, 0)
			if row.X < r.X || row.Right() > r.Right()+0.001 ||
				row.Y < r.Y || row.Bottom() > r.Bottom()+0.001 {
				t.Errorf("%vx%v: row %d %v escapes panel %v", size[0], size[1], i, row, r)
			}
		}
		for c := 0; c < 4; c++ {
			col := OverviewColumn(r, c)
			if col.Right() > r.Right()+0.001 {
				t.Errorf("%vx%v: column %d escapes panel", size[0], size[1], c)
			}
		}
		// Column tiles must not overlap each other.
		for c := 0; c < 3; c++ {
			if OverviewColumn(r, c).Right() > OverviewColumn(r, c+1).X+0.001 {
				t.Errorf("%vx%v: columns %d/%d overlap", size[0], size[1], c, c+1)
			}
		}
	}
}

// TestActionRectsInsidePanel: the 9-button worst case fits the selected
// panel at both sizes, with every button at least the minimum hit area.
func TestActionRectsInsidePanel(t *testing.T) {
	for _, size := range designSizes {
		r := ComputeRegions(size[0], size[1]).Selected
		for i, ar := range ActionRects(r, 9) {
			if ar.X < r.X || ar.Right() > r.Right()+0.001 ||
				ar.Y < r.Y || ar.Bottom() > r.Bottom()+0.001 {
				t.Errorf("%vx%v: action %d %v escapes panel %v", size[0], size[1], i, ar, r)
			}
			if ar.W < minHit || ar.H < minHit {
				t.Errorf("%vx%v: action %d hit area %vx%v below %v",
					size[0], size[1], i, ar.W, ar.H, minHit)
			}
		}
	}
}

// TestModuleCellsInsidePanel: every drawn rack cell fits its panel and
// keeps the 40×40 minimum hit area.
func TestModuleCellsInsidePanel(t *testing.T) {
	for _, size := range designSizes {
		reg := ComputeRegions(size[0], size[1])
		mr := reg.Modules
		for row := 0; row < modRows; row++ {
			for i := 0; i < ModuleMaxCells(mr); i++ {
				cell := ModuleCellRect(mr, row, i)
				if cell.X < mr.X || cell.Right() > mr.Right()+0.001 ||
					cell.Y < mr.Y || cell.Bottom() > mr.Bottom()+0.001 {
					t.Errorf("%vx%v: cell (%d,%d) %v escapes rack %v",
						size[0], size[1], row, i, cell, mr)
				}
				if cell.W < minHit || cell.H < minHit {
					t.Errorf("%vx%v: cell (%d,%d) below minimum hit area",
						size[0], size[1], row, i)
				}
			}
		}
	}
}

// TestStationRowsInsideBody: station table rows stay inside the body.
func TestStationRowsInsideBody(t *testing.T) {
	for _, size := range designSizes {
		body := StationBody(ComputeRegions(size[0], size[1]).Station)
		if StationMaxRows(body) <= 0 {
			t.Fatalf("%vx%v: no station rows fit", size[0], size[1])
		}
		table := MarketTable(body)
		for i := 0; i < StationMaxRows(table); i++ {
			row := StationRowRect(table, i)
			if row.Right() > body.Right()+0.001 || row.Bottom() > body.Bottom()+0.001 {
				t.Errorf("%vx%v: station row %d escapes body", size[0], size[1], i)
			}
		}
	}
}

// TestMinimumHitAreas: every full-size interactive control keeps a 40×40
// hit area at both window sizes (warp button, segments, close, filter,
// steppers, tabs, trade buttons, undock).
func TestMinimumHitAreas(t *testing.T) {
	for _, size := range designSizes {
		reg := ComputeRegions(size[0], size[1])
		sel := reg.Selected
		for i, sr := range WarpSegRects(sel, len(WarpRanges)) {
			if sr.W < minHit || sr.H < minHit {
				t.Errorf("%vx%v: warp segment %d %v below min hit", size[0], size[1], i, sr)
			}
		}
		if wb := WarpButtonRect(sel, len(WarpRanges)); wb.W < minHit || wb.H < minHit {
			t.Errorf("%vx%v: warp button %v below min hit", size[0], size[1], wb)
		}
		if cb := CloseButtonRect(reg.Inventory); cb.W != minHit || cb.H != minHit {
			t.Errorf("%vx%v: close button %v not 40x40", size[0], size[1], cb)
		}
		if fb := OverviewFilterButtonRect(reg.Overview); fb.W < minHit || fb.H < minHit {
			t.Errorf("%vx%v: filter button %v below min hit", size[0], size[1], fb)
		}
		body := StationBody(reg.Station)
		mn, pl := StepperRects(body)
		for _, sr := range []Rect{mn, pl} {
			if sr.W < minHit || sr.H < minHit {
				t.Errorf("%vx%v: stepper %v below min hit", size[0], size[1], sr)
			}
		}
		for _, tr := range TabRects(reg.Station, numStationTabs) {
			if tr.W < minHit || tr.H < minHit {
				t.Errorf("%vx%v: tab %v below min hit", size[0], size[1], tr)
			}
		}
		table := MarketTable(body)
		row := StationRowRect(table, 0)
		for _, br := range []Rect{MarketBuyRect(row), MarketSellRect(row)} {
			if br.W < minHit || br.H < minHit {
				t.Errorf("%vx%v: trade button %v below min hit", size[0], size[1], br)
			}
		}
	}
}

// TestOverviewRowAtWithScroll: row hit-testing maps through the scroll
// offset into content rows and rejects points outside the body.
func TestOverviewRowAtWithScroll(t *testing.T) {
	r := ComputeRegions(1280, 720).Overview
	const total = 50 // pretend 50 rows of content
	body := OverviewBody(r)
	// Top of the body: content row 0.
	if got := OverviewRowAt(r, body.Y+1, 0, total); got != 0 {
		t.Fatalf("scrollTop row = %d, want 0", got)
	}
	// Scrolled one row: the first visible line is content row 1.
	if got := OverviewRowAt(r, body.Y+1, ovRowH, total); got != 1 {
		t.Fatalf("row at scroll=1 rowH = %d, want 1", got)
	}
	// Scrolled far: mapping keeps following content, still clamped by total.
	if got := OverviewRowAt(r, body.Y+1, float64(total-1)*ovRowH, total); got != total-1 {
		t.Fatalf("bottom row = %d, want %d", got, total-1)
	}
	if got := OverviewRowAt(r, body.Y+1, float64(total)*ovRowH, total); got != -1 {
		t.Fatalf("row past content = %d, want -1", got)
	}
	// Outside the body vertically.
	if got := OverviewRowAt(r, r.Y, 0, total); got != -1 {
		t.Fatalf("row in title bar = %d, want -1", got)
	}
	if got := OverviewRowAt(r, r.Bottom(), 0, total); got != -1 {
		t.Fatalf("row below panel = %d, want -1", got)
	}
}

// TestRectContainsAndOverlaps sanity-checks the rect primitives.
func TestRectContainsAndOverlaps(t *testing.T) {
	r := Rect{X: 10, Y: 10, W: 100, H: 50}
	if !r.Contains(10, 10) || !r.Contains(109.9, 59.9) {
		t.Fatal("Contains should include top-left and near-bottom-right")
	}
	if r.Contains(110, 30) || r.Contains(30, 60) || r.Contains(9, 30) {
		t.Fatal("Contains leaked outside the rect")
	}
	if !r.Overlaps(Rect{X: 100, Y: 50, W: 10, H: 10}) {
		t.Fatal("corner-adjacent overlap not detected")
	}
	if r.Overlaps(Rect{X: 110, Y: 10, W: 5, H: 5}) {
		t.Fatal("false overlap detected")
	}
	inset := r.Inset(10)
	if inset != (Rect{X: 20, Y: 20, W: 80, H: 30}) {
		t.Fatalf("Inset wrong: %v", inset)
	}
}

// TestStarmapNodesCentersAndFits: projected nodes land inside the panel and
// preserve relative geometry deterministically.
func TestStarmapNodesCentersAndFits(t *testing.T) {
	systems := []SystemNode{
		{ID: "a", Pos: core.V(0, 0)},
		{ID: "b", Pos: core.V(10, 0)},
		{ID: "c", Pos: core.V(5, 8)},
	}
	r := Rect{X: 20, Y: 20, W: 760, H: 560}
	nodes := StarmapNodes(r, systems)
	if len(nodes) != 3 {
		t.Fatalf("got %d nodes", len(nodes))
	}
	seen := map[MapNode]bool{}
	for i, n := range nodes {
		if n.X < r.X || n.X > r.Right() || n.Y < r.Y || n.Y > r.Bottom() {
			t.Fatalf("node %d %v outside panel", i, n)
		}
		if seen[n] {
			t.Fatalf("duplicate node %v", n)
		}
		seen[n] = true
	}
	// Determinism: same input, same output.
	again := StarmapNodes(r, systems)
	for i := range nodes {
		if nodes[i] != again[i] {
			t.Fatalf("starmap projection not deterministic at %d", i)
		}
	}
	// Relative order by X must match input order by Pos.X.
	if !(nodes[0].X < nodes[1].X) {
		t.Fatal("aspect-preserving projection reordered X")
	}
}
