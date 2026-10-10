package content

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// Load reads game data from fsys.
//
// It walks items/, ships/, modules/, npcs/, systems/, worlds/, contracts/,
// traffic/ plus universe.json under the filesystem root. Every JSON file may
// contain a single object or an array of objects. universe.json must contain
// exactly one UniverseDef whose "systems" field is empty (systems come from
// systems/*.json; the universe file only supplies region/constellation/
// start_system).
//
// Errors never fail at the first problem: every parse error names its file
// and duplicate-ID errors accumulate, all reported in one joined error.
func Load(fsys fs.FS) (*Content, error) {
	c := &Content{
		Items:     map[ItemID]*ItemDef{},
		Ships:     map[ShipID]*ShipDef{},
		Modules:   map[ModuleID]*ModuleDef{},
		NPCs:      map[NpcID]*NpcDef{},
		Systems:   map[SystemID]*SystemDef{},
		Contracts: map[ContractID]*ContractDef{},
		Traffic:   map[SystemID]*TrafficDef{},
		Worlds:    map[WorldID]*WorldDef{},
	}
	l := &loader{c: c, defined: map[string]string{}}

	loadObjects[ItemDef](l, fsys, "items", "item",
		func(d *ItemDef) string { return string(d.ID) },
		func(d *ItemDef) { c.Items[d.ID] = d })
	loadObjects[ShipDef](l, fsys, "ships", "ship",
		func(d *ShipDef) string { return string(d.ID) },
		func(d *ShipDef) { c.Ships[d.ID] = d })
	loadObjects[ModuleDef](l, fsys, "modules", "module",
		func(d *ModuleDef) string { return string(d.ID) },
		func(d *ModuleDef) { c.Modules[d.ID] = d })
	loadObjects[NpcDef](l, fsys, "npcs", "npc",
		func(d *NpcDef) string { return string(d.ID) },
		func(d *NpcDef) { c.NPCs[d.ID] = d })
	loadObjects[SystemDef](l, fsys, "systems", "system",
		func(d *SystemDef) string { return string(d.ID) },
		func(d *SystemDef) { c.Systems[d.ID] = d })
	loadObjects[WorldDef](l, fsys, "worlds", "world",
		func(d *WorldDef) string { return string(d.ID) },
		func(d *WorldDef) { c.Worlds[d.ID] = d })
	loadObjects[ContractDef](l, fsys, "contracts", "contract",
		func(d *ContractDef) string { return string(d.ID) },
		func(d *ContractDef) { c.Contracts[d.ID] = d })
	// Traffic defs are keyed by their system (one def per system), so the
	// system ID doubles as the duplicate-detection identity.
	loadObjects[TrafficDef](l, fsys, "traffic", "traffic",
		func(d *TrafficDef) string { return string(d.System) },
		func(d *TrafficDef) { c.Traffic[d.System] = d })

	l.universe(fsys)

	if len(l.errs) > 0 {
		return nil, errors.Join(l.errs...)
	}
	return c, nil
}

// LoadDir loads game data from a directory on disk (os.DirFS), e.g.
// LoadDir("content/data") during development or hot-reload.
func LoadDir(dir string) (*Content, error) {
	return Load(os.DirFS(dir))
}

// MustLoad loads the embedded game data and panics on error. Intended for
// package-level initialization in cmd/.
func MustLoad() *Content {
	c, err := Load(FS())
	if err != nil {
		panic("content: embedded data failed to load: " + err.Error())
	}
	return c
}

// loader accumulates errors across files instead of failing fast.
type loader struct {
	c       *Content
	defined map[string]string // "kind/id" -> file that first defined it
	errs    []error
}

// declare records an object ID, reporting a duplicate rather than
// overwriting. It returns false when the ID was already defined elsewhere.
func (l *loader) declare(kind, id, file string) bool {
	key := kind + "/" + id
	if prev, ok := l.defined[key]; ok {
		if prev == file {
			l.errs = append(l.errs, fmt.Errorf("duplicate %s id %q in %s (defined twice in the same file)", kind, id, file))
		} else {
			l.errs = append(l.errs, fmt.Errorf("duplicate %s id %q in %s (first defined in %s)", kind, id, file, prev))
		}
		return false
	}
	l.defined[key] = file
	return true
}

// jsonFiles lists all .json files under dir in lexical order. A missing
// directory is not an error (fixture directories are partial).
func (l *loader) jsonFiles(fsys fs.FS, dir string) []string {
	var files []string
	err := fs.WalkDir(fsys, dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			l.errs = append(l.errs, fmt.Errorf("%s: %w", path, err))
			return nil
		}
		if !d.IsDir() && strings.HasSuffix(path, ".json") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		l.errs = append(l.errs, fmt.Errorf("%s: %w", dir, err))
	}
	return files
}

// loadObjects parses every JSON file in dir into T. Each file may hold a
// single object or an array of objects.
func loadObjects[T any](l *loader, fsys fs.FS, dir, kind string, idOf func(*T) string, add func(*T)) {
	for _, file := range l.jsonFiles(fsys, dir) {
		objs, err := decodeObjects[T](fsys, file)
		if err != nil {
			l.errs = append(l.errs, fmt.Errorf("%s: %w", file, err))
			continue
		}
		for i := range objs {
			d := &objs[i]
			if !l.declare(kind, idOf(d), file) {
				continue
			}
			add(d)
		}
	}
}

// decodeObjects parses a JSON file containing either one object or an array
// of objects.
func decodeObjects[T any](fsys fs.FS, file string) ([]T, error) {
	raw, err := fs.ReadFile(fsys, file)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, errors.New("empty file")
	}
	switch trimmed[0] {
	case '[':
		var arr []T
		if err := json.Unmarshal(trimmed, &arr); err != nil {
			return nil, err
		}
		return arr, nil
	case '{':
		var one T
		if err := json.Unmarshal(trimmed, &one); err != nil {
			return nil, err
		}
		return []T{one}, nil
	default:
		snippet := string(trimmed)
		if len(snippet) > 40 {
			snippet = snippet[:40] + "..."
		}
		return nil, fmt.Errorf("expected a JSON object or array, got %s", snippet)
	}
}

// universe loads universe.json (a single object; arrays are rejected).
func (l *loader) universe(fsys fs.FS) {
	const file = "universe.json"
	raw, err := fs.ReadFile(fsys, file)
	if err != nil {
		l.errs = append(l.errs, errors.New("universe.json: not found"))
		return
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		l.errs = append(l.errs, errors.New("universe.json: empty file"))
		return
	}
	if trimmed[0] == '[' {
		l.errs = append(l.errs, errors.New("universe.json: expected a single object, found an array"))
		return
	}
	var u UniverseDef
	if err := json.Unmarshal(trimmed, &u); err != nil {
		l.errs = append(l.errs, fmt.Errorf("universe.json: %w", err))
		return
	}
	l.c.Universe = &u
}
