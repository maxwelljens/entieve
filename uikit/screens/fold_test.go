package screens

import (
	"image/color"
	"reflect"
	"testing"

	"github.com/sahilm/fuzzy"
)

// TestFoldString: the fold maps the canon diacritics (AESTHETIC.md's
// font-verified set) to base letters keeping case, records every folded
// byte's source-rune offset, and leaves all-ASCII — and anything outside
// the set — byte-exact with a nil map (the allocation-free common path).
func TestFoldString(t *testing.T) {
	cases := []struct {
		in   string
		want string
		orig []int
	}{
		{"Bríždan", "Brizdan", []int{0, 1, 2, 4, 6, 7, 8}},
		{"Hušnek", "Husnek", []int{0, 1, 2, 4, 5, 6}},
		{"Młondan", "Mlondan", []int{0, 1, 3, 4, 5, 6, 7}},
		{"Pušťov", "Pustov", []int{0, 1, 2, 4, 6, 7}},
		{"Glůšan", "Glusan", []int{0, 1, 2, 4, 6, 7}},
		{"ŠŤŮ", "STU", []int{0, 2, 4}},
		{"Kumbřan", "Kumbran", []int{0, 1, 2, 3, 4, 6, 7}},
		{"Shield Booster", "Shield Booster", nil},
		{"40mm ion", "40mm ion", nil},
		{"é", "é", nil}, // outside the canon set: untouched
	}
	for _, c := range cases {
		got, orig := foldString(c.in)
		if got != c.want {
			t.Errorf("foldString(%q) = %q, want %q", c.in, got, c.want)
		}
		if !reflect.DeepEqual(orig, c.orig) {
			t.Errorf("foldString(%q) map = %v, want %v", c.in, orig, c.orig)
		}
	}
}

// TestFoldMatchRemapsToRuneStarts: a match over a folded name remaps the
// library's byte offsets into the folded string back to the ORIGINAL's
// rune starts, and the highlight consumer then tints the whole accented
// letter — an offset can never land mid-rune.
func TestFoldMatchRemapsToRuneStarts(t *testing.T) {
	name := "Kumbřan"
	folded, mp := foldString(name)

	matches := fuzzy.Find("ran", []string{folded})
	if len(matches) != 1 {
		t.Fatalf("fixture: %d matches for \"ran\" in %q", len(matches), folded)
	}
	var match []int
	for _, i := range matches[0].MatchedIndexes {
		if i < len(folded) {
			match = append(match, mp[i])
		}
	}
	// Folded r sits at byte 4, which is the rune start of ř in the name.
	if want := []int{4, 6, 7}; !reflect.DeepEqual(match, want) {
		t.Fatalf("remapped indexes = %v, want %v (rune starts)", match, want)
	}

	accent := color.RGBA{255, 180, 84, 255}
	text := color.RGBA{200, 200, 200, 255}
	segs := mktNameSegs(name, match, accent, text)
	var hits []string
	for _, s := range segs {
		if s.Tint == accent {
			hits = append(hits, s.Text)
		}
	}
	if !reflect.DeepEqual(hits, []string{"řan"}) {
		t.Errorf("accent runs = %q, want the whole letter in one run (\"řan\")", hits)
	}
}
