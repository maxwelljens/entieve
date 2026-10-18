package game

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestFinancesWiring: the Finances window is one Screen row, like every
// sibling — the row carries the title, the sidecar key, the rail section
// and one free toggle key, the key flips the panel's flag through
// handleKeys, the rail lists it, and mounting it lays it out on screen at
// both design sizes.
func TestFinancesWiring(t *testing.T) {
	row := screens[pFinances]
	if row.Title != "Finances" {
		t.Errorf("title = %q, want Finances", row.Title)
	}
	if row.SidecarName != "finances" || row.Group != "Trade" {
		t.Errorf("sidecar/group = %q/%q, want finances/Trade", row.SidecarName, row.Group)
	}
	if row.Key == keyNone || row.Action == "" {
		t.Errorf("row %+v is unkeyed", row)
	}
	if row.DockedOnly || row.SpaceOnly {
		t.Error("a read-only ledger window must open in space and docked alike")
	}

	g := newShowcaseGame(t)
	// The rail derives one entry for the row (game/rail.go).
	found := false
	for _, e := range railEntries(g) {
		if e.Title == "Finances" {
			found = true
			if e.Icon != row.Icon || e.Group != row.Group {
				t.Errorf("rail entry %+v does not match the row", e)
			}
		}
	}
	if !found {
		t.Fatal("the rail has no Finances entry")
	}

	// The key toggles the panel and back (the spec's own path).
	press := Input{JustPressed: []ebiten.Key{row.Key}}
	before := g.open[pFinances]
	if err := g.handleKeys(press); err != nil {
		t.Fatalf("handleKeys: %v", err)
	}
	if g.open[pFinances] == before {
		t.Fatalf("%v did not toggle the panel", row.Key)
	}

	// Mounted, synced and in bounds at both design sizes.
	for _, size := range [][2]int{{1280, 720}, {1024, 700}} {
		g.applySize(size[0], size[1])
		g.open[pFinances] = true
		g.syncMounts()
		pump(t, g, size[0], size[1])
		if g.fin == nil || g.panelWindow(pFinances) != g.fin.Win() {
			t.Fatalf("%dx%d: the row did not build the window", size[0], size[1])
		}
		assertInBounds(t, g, size[0], size[1])
	}
}
