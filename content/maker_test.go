package content

import (
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// The manufacturer layer: every manufactured def carries a `maker` corp-*
// slug (docs/lore/NAMING.md, APPROVED 2026-10-18); rat hulls, ores,
// minerals and loot stay unattributed.
// ---------------------------------------------------------------------------

// makerRoster is the binding roster (id → display name), frozen by the
// 2026-10-18 approval. The registry conformance test below also reads it
// out of docs/lore/NAMING.md.
var makerRoster = []struct{ id, name string }{
	{"corp-borealis", "Borealis"},
	{"corp-nordvakt", "Nordvakt"},
	{"corp-iskald", "Iskald"},
	{"corp-ferrum", "Kombinat Ferrum"},
	{"corp-druzstvo", "Družstvo"},
	{"corp-burza", "Zakłady Burza"},
	{"corp-celeritas", "Celeritas"},
	{"corp-praetor", "Praetor"},
	{"corp-aldemar", "Aldemar Group"},
	{"corp-vantera", "Vantera"},
}

// TestMakerVocabularyPinned pins content.AllCorps to the approved ten.
func TestMakerVocabularyPinned(t *testing.T) {
	if len(AllCorps) != len(makerRoster) {
		t.Fatalf("AllCorps has %d entries, want %d", len(AllCorps), len(makerRoster))
	}
	for i, want := range makerRoster {
		got := AllCorps[i]
		if got.ID != want.id || got.Name != want.name {
			t.Errorf("AllCorps[%d] = %s/%q, want %s/%q", i, got.ID, got.Name, want.id, want.name)
		}
		if c, ok := CorpByID(got.ID); !ok || c.Name != got.Name {
			t.Errorf("CorpByID(%s) = %+v (%v)", got.ID, c, ok)
		}
		if CorpName(got.ID) != got.Name {
			t.Errorf("CorpName(%s) = %q, want %q", got.ID, CorpName(got.ID), got.Name)
		}
	}
	if _, ok := CorpByID("corp-ghost"); ok {
		t.Error("CorpByID resolved an unknown slug")
	}
	if CorpName("") != "" || CorpName("nordic") != "" {
		t.Error("CorpName invented a name for a non-slug")
	}
}

// TestShippedMakersComplete requires every manufactured product — ships,
// modules and ammo/charge items — to carry a valid maker, and every
// unattributed kind (rat hulls, ores, minerals, loot) to carry none.
func TestShippedMakersComplete(t *testing.T) {
	c := MustLoad()
	if len(c.Ships) == 0 || len(c.Modules) == 0 || len(c.Items) == 0 {
		t.Fatal("no content loaded")
	}
	check := func(ctx, maker string) {
		if maker == "" {
			t.Errorf("%s: missing maker", ctx)
			return
		}
		if !validMakers[maker] {
			t.Errorf("%s: unknown maker %q", ctx, maker)
		}
	}
	for _, id := range keys(c.Ships) {
		s := c.Ships[id]
		ctx := "ships/" + string(id)
		if strings.HasPrefix(string(id), "ship-rat-") {
			if s.Maker != "" {
				t.Errorf("%s: rat hull carries maker %q (the Old Works are unattributed)", ctx, s.Maker)
			}
			continue
		}
		check(ctx, s.Maker)
	}
	for _, id := range keys(c.Modules) {
		check("modules/"+string(id), c.Modules[id].Maker)
	}
	for _, id := range keys(c.Items) {
		it := c.Items[id]
		ctx := "items/" + string(id)
		switch it.Category {
		case CatAmmo, CatCharge:
			check(ctx, it.Maker)
		default: // ore, mineral, loot
			if it.Maker != "" {
				t.Errorf("%s: %s carries maker %q (only manufacturers are attributed)", ctx, it.Category, it.Maker)
			}
		}
	}
}

// TestNamingRegistryListsCorps pins docs/lore/NAMING.md's corporation
// registry against content.AllCorps: one row per slug, matching display
// name and heritage, with the four absorbed houses' renames recorded.
func TestNamingRegistryListsCorps(t *testing.T) {
	raw, err := os.ReadFile("../docs/lore/NAMING.md")
	if err != nil {
		t.Fatalf("read NAMING.md: %v", err)
	}
	rows := map[string][]string{} // slug -> columns
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.HasPrefix(line, "| `corp-") {
			continue
		}
		cols := strings.Split(line, "|")
		if len(cols) < 6 {
			t.Fatalf("corp row has %d columns: %q", len(cols), line)
		}
		slug := strings.Trim(strings.TrimSpace(cols[1]), "`")
		rows[slug] = cols
	}
	if len(rows) != len(AllCorps) {
		t.Fatalf("NAMING.md lists %d corporations, want %d", len(rows), len(AllCorps))
	}
	for _, corp := range AllCorps {
		cols, ok := rows[corp.ID]
		if !ok {
			t.Errorf("NAMING.md has no row for %s", corp.ID)
			continue
		}
		cell := func(i int) string {
			return strings.TrimSpace(strings.ReplaceAll(cols[i], "*", ""))
		}
		if got := cell(3); got != corp.Name {
			t.Errorf("%s: registry name %q, AllCorps %q", corp.ID, got, corp.Name)
		}
		heritage := strings.ToLower(strings.ReplaceAll(cell(4), " ", "-"))
		heritage = strings.ReplaceAll(heritage, "/", "-")
		if heritage != corp.Heritage {
			t.Errorf("%s: registry heritage %q, AllCorps %q", corp.ID, heritage, corp.Heritage)
		}
	}
	// The four absorbed houses record their pre-corp names in the
	// "current name" column (col 2); Kombinat Ferrum keeps its name.
	for slug, old := range map[string]string{
		"corp-borealis":  "Borealis Systems",
		"corp-druzstvo":  "Družstvo Concordia",
		"corp-celeritas": "Celeritas Courier",
		"corp-ferrum":    "Kombinat Ferrum",
	} {
		if got := strings.TrimSpace(rows[slug][2]); got != old {
			t.Errorf("%s: current name column = %q, want %q", slug, got, old)
		}
	}
}

// TestShippedCorpCoverage pins the roster against the shipped data: every
// corporation makes at least one thing, and every maker slug used is one
// of the ten.
func TestShippedCorpCoverage(t *testing.T) {
	c := MustLoad()
	made := map[string]bool{}
	note := func(m string) {
		if m == "" {
			return
		}
		if !validMakers[m] {
			t.Errorf("shipped data uses unknown maker %q", m)
		}
		made[m] = true
	}
	for _, s := range c.Ships {
		note(s.Maker)
	}
	for _, m := range c.Modules {
		note(m.Maker)
	}
	for _, it := range c.Items {
		note(it.Maker)
	}
	for _, corp := range AllCorps {
		if !made[corp.ID] {
			t.Errorf("%s (%s) makes nothing in the shipped data", corp.ID, corp.Name)
		}
	}
}
