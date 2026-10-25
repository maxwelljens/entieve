package game

import (
	"encoding/json"
	uikit "github.com/entieve/entieve/uikit/screens"
	"os"
	"path/filepath"
	"testing"

	"github.com/entieve/entieve/uikit/widgets"
)

// columnDoc is the sidecar's tables slice, kept independent from
// layoutFile so a struct rename cannot silently break the format.
type columnDoc struct {
	Windows map[string]struct {
		Tables map[string]map[string]widgets.ColumnPref `json:"tables"`
	} `json:"windows"`
}

func readColumnDoc(t *testing.T, path string) columnDoc {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read sidecar: %v", err)
	}
	var doc columnDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse sidecar: %v", err)
	}
	return doc
}

// TestColumnPrefsPersistAcrossRestart: a dragged column width and a hidden
// column ride the layout sidecar (design units, additive — no version
// bump) and come back on the next session; the inventory's three holds
// share one stored set, the market's two lists stay separate.
func TestColumnPrefsPersistAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	g := newShowcaseGameAt(t, path)
	g.host.SetOpen(idInventory, true)
	g.host.SetOpen(idMarket, true)
	pump(t, g, 1280, 720)

	inv := bodyOf[*uikit.Inventory](g.host, idInventory).Tables()[0]
	if id := inv.PrefsID(); id != "inventory" {
		t.Fatalf("inventory table id = %q, want the shared %q", id, "inventory")
	}
	inv.SetPrefs(map[string]widgets.ColumnPref{"Item": {Width: 180}, "Vol": {Off: true}})
	if ids := []string{bodyOf[*uikit.Market](g.host, idMarket).Tables()[0].PrefsID(), bodyOf[*uikit.Market](g.host, idMarket).Tables()[1].PrefsID()}; ids[0] != "book" || ids[1] != "tree" {
		t.Fatalf("market table ids = %v, want book/tree", ids)
	}
	bodyOf[*uikit.Market](g.host, idMarket).Tables()[1].SetPrefs(map[string]widgets.ColumnPref{"Tree": {Width: 160}})
	pump(t, g, 1280, 720)

	if got := inv.Prefs()["Item"].Width; got != 180 {
		t.Errorf("live inventory width = %v, want the set 180", got)
	}
	if w := inv.WidthsForTest()[2]; w != 0 {
		t.Errorf("hidden m³ column width = %v, want the display filter's 0", w)
	}

	if err := g.SaveGame(); err != nil {
		t.Fatalf("SaveGame: %v", err)
	}
	doc := readColumnDoc(t, g.layoutPath())
	if doc.Windows["inventory"].Tables == nil {
		t.Fatal("sidecar carries no table prefs")
	}
	if got := doc.Windows["inventory"].Tables["inventory"]["Item"].Width; got != 180 {
		t.Errorf("stored Item width = %v, want 180 design px", got)
	}
	if !doc.Windows["inventory"].Tables["inventory"]["Vol"].Off {
		t.Error("stored prefs lost the hidden column")
	}
	if got := doc.Windows["market"].Tables["book"]["Item"].Width; got == 0 {
		t.Errorf("market book width not stored: %+v", doc.Windows["market"].Tables)
	}
	if got := doc.Windows["market"].Tables["tree"]["Tree"].Width; got != 160 {
		t.Errorf("market tree width = %v, want its own 160 (distinct id)", got)
	}

	// Next session: every hold table gets the stored set (the three share
	// it), and the hidden column is filtered, not removed from the rows.
	g2 := newShowcaseGameAt(t, path)
	g2.host.SetOpen(idInventory, true)
	pump(t, g2, 1280, 720)
	for i, tbl := range bodyOf[*uikit.Inventory](g2.host, idInventory).Tables() {
		p := tbl.Prefs()
		if got := p["Item"].Width; got != 180 {
			t.Errorf("restored hold %d Item width = %v, want 180", i, got)
		}
		if !p["Vol"].Off {
			t.Errorf("restored hold %d lost the hidden m³ column", i)
		}
	}
	cargo := bodyOf[*uikit.Inventory](g2.host, idInventory).Tables()[0]
	rows := cargo.RowsForTest()
	if len(rows) == 0 || len(rows[0].Cells) != 3 {
		t.Fatalf("restored rows = %d, want positional three-cell rows", len(rows))
	}
}
