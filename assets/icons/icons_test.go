package icons

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/entieve/entieve/content"
)

// contentIDs is every id a drop-in icon may be named after: the loaded
// modules and items plus the corporation roster.
func contentIDs(t *testing.T) map[string]bool {
	t.Helper()
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content: %v", err)
	}
	known := map[string]bool{}
	for id := range c.Modules {
		known[string(id)] = true
	}
	for id := range c.Items {
		known[string(id)] = true
	}
	for _, corp := range content.AllCorps {
		known[corp.ID] = true
	}
	return known
}

// iconKeys is every icon key a drop-in may be named after (the registry's
// manifest, read as the file it is: this package cannot import
// uikit/icons, which embeds this one). The rail's own art ships this way —
// assets/icons/rail_*.png names the key the rail row draws — so a rail
// icon can be replaced without touching a content id.
func iconKeys(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile("../../uikit/icons/manifest.json")
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	var m struct {
		Icons map[string]json.RawMessage `json:"icons"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if len(m.Icons) == 0 {
		t.Fatal("manifest: no icon keys")
	}
	keys := make(map[string]bool, len(m.Icons))
	for k := range m.Icons {
		keys[k] = true
	}
	return keys
}

// TestDropinIconsAreContentIDs is the scheme guard: the file name is the
// content id (or an icon key), so a file named for a display name
// ("kerngrit.png") would be embedded but never drawn. _alt/ must stay
// outside the embed.
func TestDropinIconsAreContentIDs(t *testing.T) {
	known := contentIDs(t)
	keys := iconKeys(t)
	ids := IDs()
	if len(ids) == 0 {
		t.Fatal("no drop-in icons embedded")
	}
	for _, id := range ids {
		if !known[id] && !keys[id] {
			t.Errorf("%s.png: neither a content id (modules, items, corps) nor an icon key (uikit/icons/manifest.json) — the file name is one of those two", id)
		}
	}
	if _, err := FS.Open("_alt/armour_plating_alt.png"); err == nil {
		t.Error("_alt/ is embedded; go:embed must only reach *.png in this directory")
	}
}

// TestDropinLoadCaches: Load decodes once and answers false for an unknown
// id instead of a missing-image artifact.
func TestDropinLoadCaches(t *testing.T) {
	for _, id := range IDs() {
		img, ok := Load(id)
		if !ok {
			t.Fatalf("%s: Load failed for an embedded file", id)
		}
		if b := img.Bounds(); b.Dx() < 1 || b.Dx() != b.Dy() {
			t.Errorf("%s: bounds %v, want a square render", id, b)
		}
		again, _ := Load(id)
		if img != again {
			t.Errorf("%s: decoded twice (the cache missed)", id)
		}
		if _, ok := Load("not-a-content-id"); ok {
			t.Error("Load answered ok for a missing id")
		}
	}
}
