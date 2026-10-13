package content

import (
	"strings"
	"testing"
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

// retiredNameFragments are the placeholder names (and their fragments) that
// must never survive in display text — docs/lore/NAMING.md binding rules.
// Compares case-insensitively against shipped NPC descriptions.
var retiredNameFragments = []string{
	"solace", "penumbra", "gloam", "haven rest", "thevek", "loomfall", "corsair",
}

// TestShippedNpcDescriptionsAvoidRetiredNames guards the no-placeholder-echo
// rule for the prose shipped with the NPCs.
func TestShippedNpcDescriptionsAvoidRetiredNames(t *testing.T) {
	c := MustLoad()
	for _, id := range keys(c.NPCs) {
		n := c.NPCs[id]
		ctx := "npcs/" + string(id)
		hay := strings.ToLower(n.Description)
		for _, needle := range retiredNameFragments {
			if strings.Contains(hay, needle) {
				t.Errorf("%s: description echoes retired placeholder name %q", ctx, needle)
			}
		}
	}
}
