package game

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/entieve/entieve/sim"
)

// The document-store adapters (docstore): every game document reads
// through the tolerant contract and writes through the atomic one, with
// the on-disk bytes unchanged (TestLayoutSidecarBytesStable is the arbiter
// for the layout; the save's own MarshalIndent shape is pinned here).

// TestSaveWriteIsAtomic: a save leaves exact MarshalIndent bytes behind
// (no trailing newline) and no <doc>.<random>.tmp file; when the directory
// refuses the write the previous save stays byte-identical and SaveGame
// reports the failure.
func TestSaveWriteIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "save.json")
	g := newShowcaseGameAt(t, path)
	pump(t, g, 1280, 720)
	if err := g.SaveGame(); err != nil {
		t.Fatalf("SaveGame: %v", err)
	}

	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read save: %v", err)
	}
	var save sim.Save
	if err := json.Unmarshal(saved, &save); err != nil {
		t.Fatalf("parse save: %v", err)
	}
	want, err := json.MarshalIndent(save, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved, want) {
		t.Error("save bytes are not exactly MarshalIndent(…, \"  \") (trailing newline?)")
	}
	layout, err := os.ReadFile(g.layoutPath())
	if err != nil {
		t.Fatalf("read layout: %v", err)
	}
	if bytes.HasSuffix(layout, []byte("\n")) {
		t.Error("layout sidecar ends with a newline")
	}
	assertNoTemps(t, dir)

	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	if err := g.SaveGame(); err == nil {
		t.Fatal("SaveGame into a read-only directory reported no error")
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read save after the failed write: %v", err)
	}
	if !bytes.Equal(after, saved) {
		t.Error("a failed save changed the previous save bytes")
	}
	assertNoTemps(t, dir)
}

// TestSaveAsSkipsTempFiles: an in-flight <doc>.<random>.tmp from an
// interrupted write is invisible to the save manager and never shifts the
// next free save name.
func TestSaveAsSkipsTempFiles(t *testing.T) {
	dir := t.TempDir()
	g := newShowcaseGameAt(t, filepath.Join(dir, "save.json"))
	tmp := filepath.Join(dir, ".save.json.123456.tmp")
	if err := os.WriteFile(tmp, []byte("{partial"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got, want := g.nextSavePath(), filepath.Join(dir, "save.json"); got != want {
		t.Errorf("nextSavePath with a stray temp = %q, want %q", got, want)
	}
	if saves := g.listSaves(); len(saves) != 0 {
		t.Errorf("listSaves = %v, want nothing (a temp is not a save)", saves)
	}

	if err := g.SaveAs(); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	if got, want := g.activeSavePath(), filepath.Join(dir, "save.json"); got != want {
		t.Errorf("active save = %q, want %q", got, want)
	}
	saves := g.listSaves()
	if len(saves) != 1 || saves[0].Path != filepath.Join(dir, "save.json") {
		t.Errorf("listSaves = %v, want exactly save.json", saves)
	}
	if got, want := g.nextSavePath(), filepath.Join(dir, "save-2.json"); got != want {
		t.Errorf("nextSavePath after the first save = %q, want %q", got, want)
	}
	for _, s := range saves {
		if strings.HasSuffix(s.Path, ".tmp") {
			t.Errorf("listSaves shows a temp file: %s", s.Path)
		}
	}
}

// wantSettings is one expected resolved settings state.
type wantSettings struct {
	scale             float64
	fullscreen, vsync bool
	rail, shuffle     bool
	master, music, ui int
}

// defaultsSettings is the documented boot state (no readable document).
func defaultsSettings() wantSettings {
	return wantSettings{
		scale: 1.0, fullscreen: false, vsync: true, rail: false, shuffle: true,
		master: 100, music: 70, ui: 80,
	}
}

// TestSidecarTolerantReadAll runs ONE table over all three sidecars: the
// same four damage modes must leave each document's documented defaults,
// ignore unknown keys and restore only what a partial file holds.
func TestSidecarTolerantReadAll(t *testing.T) {
	movedOverview := `"overview":{"x":700,"y":500,"w":400,"h":300,"open":false}`
	cases := []struct {
		name           string
		layout         string // "" = the file does not exist
		settings       string
		keys           string
		want           wantSettings
		layoutDefaults bool
		keysDefaults   bool
	}{
		{
			name: "missing", want: defaultsSettings(),
			layoutDefaults: true, keysDefaults: true,
		},
		{
			name: "garbage", layout: "{not json[", settings: "{not json[", keys: "{not json[",
			want: defaultsSettings(), layoutDefaults: true, keysDefaults: true,
		},
		{
			// Layout v1 and keys v0 are below the accepted floor; the
			// settings file predates the newer keys (no vsync_off etc.).
			name:     "old",
			layout:   `{"version":1,"windows":{` + movedOverview + `}}`,
			settings: `{"ui_scale":1,"fullscreen":false}`,
			keys:     `{"version":0,"binds":{"warp":{"key":"S"}}}`,
			want:     defaultsSettings(), layoutDefaults: true, keysDefaults: true,
		},
		{
			// Unknown versions are ignored wholesale; a Plain document
			// reads its known keys and ignores the rest.
			name:     "newer",
			layout:   `{"version":99,"windows":{` + movedOverview + `}}`,
			settings: `{"ui_scale":1,"fullscreen":true,"future_setting":7}`,
			keys:     `{"version":99,"binds":{"warp":{"key":"S"}}}`,
			want: wantSettings{
				scale: 1.0, fullscreen: true, vsync: true, rail: false, shuffle: true,
				master: 100, music: 70, ui: 80,
			},
			layoutDefaults: true, keysDefaults: true,
		},
		{
			// Partial documents restore only what they hold; unknown keys
			// and unknown ids/actions are dropped.
			name: "partial",
			layout: `{"version":3,"windows":{
				"log":{"x":500,"y":100,"w":300,"h":200,"open":false},
				"future-window":{"x":9,"y":9,"w":10,"h":10,"open":true}}}`,
			settings: `{"ui_scale":1.5,"future_setting":7}`,
			keys:     `{"version":1,"binds":{"warp":{"key":"F12"},"future-action":{"key":"F11"}}}`,
			want: wantSettings{
				scale: 1.5, fullscreen: false, vsync: true, rail: false, shuffle: true,
				master: 100, music: 70, ui: 80,
			},
			layoutDefaults: false, keysDefaults: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "save.json")
			for _, d := range []struct{ path, body string }{
				{path + ".layout.json", tc.layout},
				{path + ".settings.json", tc.settings},
				{path + ".keys.json", tc.keys},
			} {
				if d.body == "" {
					continue
				}
				if err := os.WriteFile(d.path, []byte(d.body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			g := newShowcaseGameAt(t, path)
			pump(t, g, 1280, 720)

			// The layout defaults of this row: a twin session with the same
			// settings (the UI scale moves the composed default rects) and
			// no layout sidecar.
			refPath := filepath.Join(t.TempDir(), "save.json")
			if tc.settings != "" {
				if err := os.WriteFile(refPath+".settings.json", []byte(tc.settings), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			ref := newShowcaseGameAt(t, refPath)
			pump(t, ref, 1280, 720)
			refLayout := layoutBytes(t, ref)

			// Settings.
			got := wantSettings{
				scale: g.uiScale, fullscreen: g.fullscreen, vsync: g.vsync,
				rail: g.railCollapsed, shuffle: g.shuffle,
				master: g.masterVol, music: g.musicVol, ui: g.uiVol,
			}
			if got != tc.want {
				t.Errorf("settings = %+v, want %+v", got, tc.want)
			}
			if w, h := g.windowSize(); w != defaultW || h != defaultH {
				t.Errorf("window size = %d×%d, want the %d×%d default", w, h, defaultW, defaultH)
			}

			// Keys.
			if tc.keysDefaults {
				for _, a := range keyActions {
					if b, ok := g.binds[a.ID]; !ok || b != a.Def {
						t.Errorf("key %s = %+v (bound %v), want the default %+v", a.ID, b, ok, a.Def)
					}
				}
				if _, ok := g.binds["future-action"]; ok {
					t.Error("unknown action id reached the live binds")
				}
			} else {
				if b, ok := g.binds["warp"]; !ok || b.Key.String() != "F12" {
					t.Errorf("warp = %+v (bound %v), want F12", b, ok)
				}
				if _, ok := g.binds["future-action"]; ok {
					t.Error("unknown action id reached the live binds")
				}
				for _, a := range keyActions {
					if a.ID == "warp" {
						continue
					}
					if b, ok := g.binds[a.ID]; !ok || b != a.Def {
						t.Errorf("key %s = %+v (bound %v), want the untouched default %+v", a.ID, b, ok, a.Def)
					}
				}
			}

			// Layout: the whole document, written back from the live state.
			if tc.layoutDefaults {
				if gotLayout := layoutBytes(t, g); !bytes.Equal(gotLayout, refLayout) {
					t.Errorf("layout after %s sidecar differs from the defaults:\n got: %s\nwant: %s",
						tc.name, gotLayout, refLayout)
				}
				return
			}
			// Write the live state back before comparing: the input file
			// itself is the partial document, the written one is what the
			// session restored.
			layoutBytes(t, g)
			gotDoc, wantDoc := readDoc(t, g.layoutPath()), readDoc(t, ref.layoutPath())
			if e := gotDoc.Windows["log"]; e.X != 500 || e.Y != 100 || e.W != 300 || e.H != 200 || e.Open {
				t.Errorf("partial log entry restored as %+v, want the saved (500,100)-(800,300) closed", e)
			}
			if _, ok := gotDoc.Windows["future-window"]; ok {
				t.Error("unknown window name reached the sidecar")
			}
			delete(gotDoc.Windows, "log")
			delete(wantDoc.Windows, "log")
			if !reflect.DeepEqual(gotDoc.Windows, wantDoc.Windows) {
				t.Errorf("partial layout moved other windows:\n got: %v\nwant: %v", gotDoc.Windows, wantDoc.Windows)
			}
		})
	}
}

// layoutBytes writes the layout sidecar through the production writer and
// returns its bytes.
func layoutBytes(t *testing.T, g *Game) []byte {
	t.Helper()
	if err := g.writeLayout(); err != nil {
		t.Fatalf("writeLayout: %v", err)
	}
	data, err := os.ReadFile(g.layoutPath())
	if err != nil {
		t.Fatalf("read layout: %v", err)
	}
	return data
}

// assertNoTemps fails when a document temp file (<doc>.<random>.tmp) is
// left behind in dir.
func assertNoTemps(t *testing.T, dir string) {
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
