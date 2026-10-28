package docstore

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testDoc is a minimal versioned document: the store only knows the
// version key, the fields stay the caller's.
type testDoc struct {
	Version int    `json:"version"`
	Name    string `json:"name"`
}

const (
	testVersion  = 3
	testReadFrom = 2
)

// TestLoadTable pins the whole read contract on both document kinds:
// missing / garbage / below readFrom / current / above current, each with
// the error kind and the value that survives.
func TestLoadTable(t *testing.T) {
	cases := []struct {
		name         string
		body         string // "" = the file does not exist
		versionedErr error  // want for Versioned (nil = loads)
		plainErr     error  // want for Plain (nil = loads)
		versioned    string
		plain        string
	}{
		{"missing", "", ErrMissing, ErrMissing, "", ""},
		{"garbage", "{not json", ErrFormat, ErrFormat, "", ""},
		{"version 0", `{"version":0,"name":"x"}`, ErrVersion, nil, "", "x"},
		{"version 1", `{"version":1,"name":"x"}`, ErrVersion, nil, "", "x"},
		{"version current", `{"version":3,"name":"x"}`, nil, nil, "x", "x"},
		{"version+1", `{"version":4,"name":"x"}`, ErrVersion, nil, "", "x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "doc.json")
			if tc.body != "" {
				if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			for _, v := range []struct {
				label string
				doc   Doc
				err   error
				name  string
			}{
				{"versioned", Versioned(path, testVersion, testReadFrom), tc.versionedErr, tc.versioned},
				{"plain", Plain(path), tc.plainErr, tc.plain},
			} {
				var got testDoc
				err := v.doc.Load(&got)
				if !errors.Is(err, v.err) {
					t.Fatalf("%s: Load = %v, want %v", v.label, err, v.err)
				}
				if got.Name != v.name {
					t.Errorf("%s: decoded name = %q, want %q", v.label, got.Name, v.name)
				}
			}
		})
	}
}

// TestSaveIsAtomic pins the write contract: exact MarshalIndent bytes (no
// trailing newline), mode 0644, no temp file left behind — and on a failed
// write the old document stays byte-identical with no temp lingering.
func TestSaveIsAtomic(t *testing.T) {
	t.Run("success writes exact bytes", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "save.json")
		v := testDoc{Version: testVersion, Name: "kept"}
		if err := Plain(path).Save(v); err != nil {
			t.Fatalf("Save: %v", err)
		}
		want, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if string(data) != string(want) {
			t.Errorf("bytes = %q, want MarshalIndent %q", data, want)
		}
		if strings.HasSuffix(string(data), "\n") {
			t.Error("document ends with a newline")
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if mode := info.Mode().Perm(); mode != 0o644 {
			t.Errorf("mode = %o, want 644", mode)
		}
		assertNoTemp(t, dir)
	})

	t.Run("failed write keeps the old bytes", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root ignores directory permissions")
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "save.json")
		old := `{"version":3,"name":"old"}`
		if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(dir, 0o755) })
		if err := Plain(path).Save(testDoc{Version: testVersion, Name: "new"}); err == nil {
			t.Fatal("Save into a read-only directory reported no error")
		}
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if string(data) != old {
			t.Errorf("document after a failed write = %q, want the old %q", data, old)
		}
		assertNoTemp(t, dir)
	})

	t.Run("failed rename removes the temp", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "target.json")
		// A non-empty directory at the destination makes Rename fail
		// AFTER the temp file has been written.
		if err := os.MkdirAll(filepath.Join(path, "blocker"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := Plain(path).Save(testDoc{Version: testVersion, Name: "x"}); err == nil {
			t.Fatal("Save onto a directory reported no error")
		}
		assertNoTemp(t, dir)
	})
}

// TestPathAndRemove: Path is the document it was built for, and Remove
// treats a missing file as success.
func TestPathAndRemove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.json")
	doc := Versioned(path, testVersion, testReadFrom)
	if got := doc.Path(); got != path {
		t.Errorf("Path = %q, want %q", got, path)
	}
	if err := doc.Remove(); err != nil {
		t.Errorf("Remove on a missing document = %v, want nil", err)
	}
	if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := doc.Remove(); err != nil {
		t.Errorf("Remove = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("document still present after Remove: %v", err)
	}
}

// assertNoTemp fails when a save temp file lingers in dir.
func assertNoTemp(t *testing.T, dir string) {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}
