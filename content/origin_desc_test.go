package content

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// ---------------------------------------------------------------------------
// Lore fields: hull origin tags (ships/*.json "origin") and NPC dossier
// prose (npcs/*.json "description"). Vocabulary: docs/lore/NAMING.md;
// voice: docs/lore/AESTHETIC.md. The schema allows both fields to be empty
// (fixtures and test worlds omit them); the shipped data may not.
// ---------------------------------------------------------------------------

// TestShippedHullsCarryOrigin requires every shipped hull to carry an origin
// tag from the NAMING.md vocabulary. Mirrors the contract title/description
// rule in TestEmbeddedContractsValid.
func TestShippedHullsCarryOrigin(t *testing.T) {
	c := MustLoad()
	if len(c.Ships) == 0 {
		t.Fatal("no ships loaded")
	}
	for _, id := range keys(c.Ships) {
		s := c.Ships[id]
		ctx := "ships/" + string(id)
		if s.Origin == "" {
			t.Errorf("%s: missing origin", ctx)
		} else if !validOrigins[s.Origin] {
			t.Errorf("%s: unknown origin %q (want moravian|polish|nordic|latinate|old-works)",
				ctx, s.Origin)
		}
	}
}

// TestShippedNpcsCarryDescription requires every shipped NPC to carry
// non-empty dossier prose. Mirrors the contract title/description rule in
// TestEmbeddedContractsValid.
func TestShippedNpcsCarryDescription(t *testing.T) {
	c := MustLoad()
	if len(c.NPCs) == 0 {
		t.Fatal("no npcs loaded")
	}
	for _, id := range keys(c.NPCs) {
		n := c.NPCs[id]
		ctx := "npcs/" + string(id)
		if n.Description == "" {
			t.Errorf("%s: missing description", ctx)
		}
	}
}

// retiredNames are the placeholder names (and their fragments) that must
// never survive in display text — docs/lore/NAMING.md binding rules. Matched
// case-insensitively, whole word only: several current names carry a retired
// one as a prefix (Kosa/Kosar, Halda/Malmhalda, Tabor/Taborek,
// Nordvakt/Nordvakten), and Doki/Huta/Kantor are ordinary enough to need it.
var retiredNames = []string{
	"solace", "penumbra", "gloam", "haven rest", "thevek", "loomfall", "corsair",
	// Time-depth drift, 2026-10-26 (NAMING.md).
	"starogród", "rudnik", "kuźnica", "młynów", "kamieniec", "pustkowie", "głusza",
	"jernborg", "przystań", "gniazdo", "halda", "huta", "doki", "kantor",
	"väktare", "kosa", "havíř", "furman", "tabor", "custos",
	"borealis", "nordvakt", "iskald", "družstvo", "celeritas",
}

// containsWord reports whether hay holds needle as a whole word (letters on
// either side disqualify a hit; unicode-aware, so ń/ź/ř end a word).
func containsWord(hay, needle string) bool {
	for i := 0; ; i++ {
		j := strings.Index(hay[i:], needle)
		if j < 0 {
			return false
		}
		j += i
		before, _ := utf8.DecodeLastRuneInString(hay[:j])
		after, _ := utf8.DecodeRuneInString(hay[j+len(needle):])
		if !unicode.IsLetter(before) && !unicode.IsLetter(after) {
			return true
		}
	}
}

// TestShippedTextAvoidsRetiredNames guards the no-placeholder-echo rule across
// every display name and prose field the content ships: names, titles and
// descriptions. Worlds are lead-owned and reported separately.
func TestShippedTextAvoidsRetiredNames(t *testing.T) {
	// The whole-word rule has to bite on the retired name and stay quiet on
	// the current one that carries it as a prefix.
	boundary := []struct {
		text, needle string
		hit          bool
	}{
		{"Kosa", "kosa", true},
		{"Kosar", "kosa", false},
		{"Malmhalda", "halda", false},
		{"Taborek", "tabor", false},
		{"Nordvakten", "nordvakt", false},
		{"Doki's yard", "doki", true},
		{"Huta's platform", "huta", true},
		{"the Gniazvor", "gniazdo", false},
		{"the Havstov office", "przystań", false},
		{"Pustheim", "pustkowie", false},
	}
	for _, b := range boundary {
		if got := containsWord(strings.ToLower(b.text), b.needle); got != b.hit {
			t.Errorf("containsWord(%q, %q) = %v, want %v", b.text, b.needle, got, b.hit)
		}
	}

	c := MustLoad()
	check := func(ctx string, text string) {
		hay := strings.ToLower(text)
		for _, needle := range retiredNames {
			if containsWord(hay, needle) {
				t.Errorf("%s: echoes retired placeholder name %q", ctx, needle)
			}
		}
	}
	for _, id := range keys(c.Systems) {
		s := c.Systems[id]
		check("systems/"+string(id), s.Name)
		for _, cel := range s.Celestials {
			check("systems/"+string(id)+"/"+string(cel.ID), cel.Name)
		}
	}
	for _, id := range keys(c.Ships) {
		s := c.Ships[id]
		check("ships/"+string(id), s.Name)
		check("ships/"+string(id), s.Description)
	}
	for _, id := range keys(c.NPCs) {
		n := c.NPCs[id]
		check("npcs/"+string(id), n.Name)
		check("npcs/"+string(id), n.Description)
	}
	for _, id := range keys(c.Contracts) {
		ct := c.Contracts[id]
		check("contracts/"+string(id), ct.Title)
		check("contracts/"+string(id), ct.Description)
	}
	for _, id := range keys(c.Items) {
		it := c.Items[id]
		check("items/"+string(id), it.Name)
		check("items/"+string(id), it.Description)
	}
	for _, id := range keys(c.Modules) {
		m := c.Modules[id]
		check("modules/"+string(id), m.Name)
		check("modules/"+string(id), m.Description)
	}
	for _, corp := range AllCorps {
		check("corps/"+corp.ID, corp.Name)
	}
}
