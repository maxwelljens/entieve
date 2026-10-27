package icons

import (
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

// TestDropinIconsAreContentIDs is the scheme guard: the file name is the
// content id, so a file named for a display name ("kerngrit.png") would be
// embedded but never drawn. _alt/ must stay outside the embed.
func TestDropinIconsAreContentIDs(t *testing.T) {
	known := contentIDs(t)
	ids := IDs()
	if len(ids) == 0 {
		t.Fatal("no drop-in icons embedded")
	}
	for _, id := range ids {
		if !known[id] {
			t.Errorf("%s.png: not a content id (modules, items, corps) — the file name is the id", id)
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
