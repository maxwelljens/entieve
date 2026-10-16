package widgets

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// The import-direction contract of the three uikit modules (the whole
// point of the split):
//
//	theme   → nothing above it in uikit (no widgets, no screens)
//	widgets → theme, never screens
//	screens → widgets + theme only, downward
//
// This test parses the sibling packages' NON-test sources: test files
// may import upward (the integration suites do), production code may
// not. The compiler enforces acyclicity on top of this — this pins the
// direction itself, not just the absence of cycles.
func TestImportDirection(t *testing.T) {
	const (
		theme   = "github.com/entieve/entieve/uikit/theme"
		widgets = "github.com/entieve/entieve/uikit/widgets"
		screens = "github.com/entieve/entieve/uikit/screens"
	)
	// forbidden[dir] = uikit modules that dir's production sources must
	// not import.
	forbidden := map[string][]string{
		"../theme":   {widgets, screens},
		"../widgets": {screens},
		"../screens": {}, // nothing above screens exists
	}
	// required[dir] = modules the dir must actually sit on.
	required := map[string][]string{
		"../widgets": {theme},
		"../screens": {widgets, theme},
	}
	for _, dir := range []string{"../theme", "../widgets", "../screens"} {
		got := uikitImports(t, dir)
		self := "github.com/entieve/entieve/uikit/" + filepath.Base(dir)
		if got[self] {
			t.Errorf("%s imports itself (%s)", dir, self)
		}
		for _, ban := range forbidden[dir] {
			if got[ban] {
				t.Errorf("%s imports %s — direction must be theme <- widgets <- screens", dir, ban)
			}
		}
		for _, want := range required[dir] {
			if !got[want] {
				t.Errorf("%s does not import %s (it must sit on it)", dir, want)
			}
		}
	}
}

// uikitImports parses dir's non-test sources and returns the uikit
// module paths it imports (module import path → true). icons and
// internal support packages are below/outside the three-module rule.
func uikitImports(t *testing.T, dir string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", dir, err)
	}
	out := map[string]bool{}
	for _, pk := range pkgs {
		for _, f := range pk.Files {
			for _, imp := range f.Imports {
				p := strings.Trim(imp.Path.Value, `"`)
				if !strings.HasPrefix(p, "github.com/entieve/entieve/uikit/") {
					continue
				}
				if p == "github.com/entieve/entieve/uikit/icons" || strings.Contains(p, "/internal/") {
					continue
				}
				out[p] = true
			}
		}
	}
	return out
}
