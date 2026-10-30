package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	uikit "github.com/entieve/entieve/uikit/screens"
)

// sidecarFilter parses the overview row's tick-box state out of a sidecar.
func sidecarFilter(t *testing.T, path string) map[string]uikit.OvFlag {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read sidecar: %v", err)
	}
	var doc struct {
		Windows map[string]struct {
			Filter map[string]uikit.OvFlag `json:"filter"`
		} `json:"windows"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse sidecar: %v", err)
	}
	return doc.Windows["overview"].Filter
}

// TestOverviewFilterPersists: the overview's per-kind filters ride the
// layout sidecar's own overview row (the one window channel that already
// carries rects, open state and column prefs). A fresh state writes nothing
// — that is why the byte golden does not move — and a switched-off kind is
// written and restored on the next session.
func TestOverviewFilterPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	g := newShowcaseGameAt(t, path)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)

	// Default: the filter key is absent, byte for byte.
	if err := g.SaveGame(); err != nil {
		t.Fatalf("SaveGame: %v", err)
	}
	if got := sidecarFilter(t, g.layoutPath()); len(got) != 0 {
		t.Fatalf("default filter state wrote %v, want no entry at all", got)
	}

	// Two switches off, one of each kind.
	g.kit.SetOverviewFilter(uikit.OvFilter{
		"station":  {Hidden: true},
		"asteroid": {Plain: true},
	})
	if err := g.SaveGame(); err != nil {
		t.Fatalf("SaveGame: %v", err)
	}
	got := sidecarFilter(t, g.layoutPath())
	if len(got) != 2 {
		t.Fatalf("persisted filter = %v, want the two switched-off kinds", got)
	}
	if !got["station"].Hidden || got["station"].Plain {
		t.Errorf("station flags = %+v, want hidden only", got["station"])
	}
	if got["asteroid"].Hidden || !got["asteroid"].Plain {
		t.Errorf("asteroid flags = %+v, want plain only", got["asteroid"])
	}

	// The next session restores exactly that state.
	g2 := newShowcaseGameLoaded(t, path)
	g2.applySize(1280, 720)
	pump(t, g2, 1280, 720)
	f := g2.kit.OverviewFilter()
	if f.Shown("station") {
		t.Error("restored state still shows stations")
	}
	if f.Coloured("asteroid") {
		t.Error("restored state still colours asteroids")
	}
	if !f.Shown("gate") || !f.Coloured("gate") {
		t.Error("restored state switched off a kind nobody touched")
	}
}
