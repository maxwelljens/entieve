package game

import (
	"os"
	"strings"
	"testing"
)

// TestCreditsPinSources: the credits window's key strings cannot drift
// from the provenance files they were authored from.
func TestCreditsPinSources(t *testing.T) {
	section := func(title string) []string {
		for _, s := range creditSections {
			if s.Title == title {
				out := []string{s.Title}
				for _, r := range s.Rows {
					out = append(out, r.Role, r.Text)
				}
				return out
			}
		}
		t.Fatalf("no %q credits section", title)
		return nil
	}
	joined := func(title string) string { return strings.Join(section(title), "\n") }
	read := func(path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		return string(data)
	}

	license := read("../LICENSE")
	// The project block is the Development section (it holds Entieve and
	// its licence line; the old single-purpose "Entieve" section folded
	// into it).
	entieve := joined("Development")
	for _, want := range []string{"MIT License", "2026 The Entieve Authors"} {
		if !strings.Contains(license, want) {
			t.Fatalf("LICENSE lost %q", want)
		}
		if !strings.Contains(entieve, want) {
			t.Errorf("Entieve credits lost %q", want)
		}
	}

	icons := read("../uikit/icons/CREDITS.md")
	iconLine := joined("Iconography")
	if !strings.Contains(icons, "Available on https://game-icons.net") || !strings.Contains(iconLine, "Available on https://game-icons.net") {
		t.Error("the game-icons.net attribution sentence drifted")
	}
	for _, author := range []string{"Delapouite", "Skoll", "Lorc", "sbed", "Cathelineau", "Quoting"} {
		if !strings.Contains(icons, "Icons made by "+author+".") {
			t.Fatalf("CREDITS.md lost author %q", author)
		}
		if !strings.Contains(iconLine, author) {
			t.Errorf("icon credits lost author %q", author)
		}
	}

	renders := joined("Module renders")
	for _, id := range []string{"mod_autocannon_s", "mod_autocannon_m", "mod_artillery_s", "mod_artillery_m", "mod_artillery_l"} {
		if !strings.Contains(icons, id) {
			t.Fatalf("CREDITS.md lost render %q", id)
		}
	}
	if !strings.Contains(renders, "CC0 1.0") || !strings.Contains(renders, "github.com/maxwelljens/entieve") {
		t.Error("module-render credits drifted from the CC0 manifest entries")
	}

	// The Cyber Terminal form-language clause is quoted from AESTHETIC.md.
	clause := "Cyber Terminal UI Kit (kit by Kerem Bolek, 2026; form vocabulary only — no kit file is shipped, and no kit colour is used)"
	doc := strings.Join(strings.Fields(strings.ReplaceAll(read("../docs/lore/AESTHETIC.md"), "**", "")), " ")
	if !strings.Contains(doc, clause) {
		t.Fatalf("AESTHETIC.md lost the form-language clause")
	}
	if !strings.Contains(joined("Interface form"), clause) {
		t.Error("interface-form credits drifted from AESTHETIC.md")
	}

	ofl := read("../render/fonts/IBM_Plex_Sans/OFL.txt")
	fonts := joined("Typefaces")
	for _, want := range []string{"2017 IBM Corp", "SIL Open Font License, Version 1.1"} {
		if !strings.Contains(ofl, want) {
			t.Fatalf("IBM Plex OFL lost %q", want)
		}
		if !strings.Contains(fonts, want) {
			t.Errorf("typeface credits lost %q", want)
		}
	}

	gomod := read("../go.mod")
	built := joined("Built with")
	for _, mod := range []string{"ebitenui", "sahilm/fuzzy", "hajimehoshi/ebiten"} {
		if !strings.Contains(gomod, mod) {
			t.Fatalf("go.mod lost %q", mod)
		}
	}
	if !strings.Contains(built, "Ebitengine") || !strings.Contains(built, "sahilm/fuzzy") {
		t.Error("built-with credits drifted from go.mod")
	}
}
