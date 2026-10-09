package ui

import (
	"testing"

	"github.com/entieve/entieve/core"
)

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
// design size and the small supported size.
func TestRegionsFitScreen(t *testing.T) {
	for _, size := range [][2]float64{{1280, 720}, {800, 600}} {
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

// TestBaseRegionsDoNotOverlap: the base HUD panels never overlap.
func TestBaseRegionsDoNotOverlap(t *testing.T) {
	for _, size := range [][2]float64{{1280, 720}, {800, 600}} {
		reg := ComputeRegions(size[0], size[1])
		base := baseRegions(reg)
		names := []string{"Selected", "Overview", "Targets", "HUD", "Modules", "Log"}
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

// TestOverviewRowRectsInsidePanel: header + rows + F-key labels stay within
// their panels at both sizes.
func TestOverviewRowRectsInsidePanel(t *testing.T) {
	for _, size := range [][2]float64{{1280, 720}, {800, 600}} {
		reg := ComputeRegions(size[0], size[1])
		r := reg.Overview
		if OverviewMaxRows(r) <= 0 {
			t.Fatalf("%vx%v: no overview rows fit", size[0], size[1])
		}
		for i := 0; i < OverviewMaxRows(r); i++ {
			row := OverviewRowRect(r, i)
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
// panel at both sizes.
func TestActionRectsInsidePanel(t *testing.T) {
	for _, size := range [][2]float64{{1280, 720}, {800, 600}} {
		r := ComputeRegions(size[0], size[1]).Selected
		for i, ar := range ActionRects(r, 9) {
			if ar.X < r.X || ar.Right() > r.Right()+0.001 ||
				ar.Y < r.Y || ar.Bottom() > r.Bottom()+0.001 {
				t.Errorf("%vx%v: action %d %v escapes panel %v", size[0], size[1], i, ar, r)
			}
		}
	}
}

// TestModuleCellsInsidePanel: every drawn rack cell fits its panel.
func TestModuleCellsInsidePanel(t *testing.T) {
	for _, size := range [][2]float64{{1280, 720}, {800, 600}} {
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
			}
		}
	}
}

// TestStationRowsInsideBody: station table rows stay inside the body.
func TestStationRowsInsideBody(t *testing.T) {
	for _, size := range [][2]float64{{1280, 720}, {800, 600}} {
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
