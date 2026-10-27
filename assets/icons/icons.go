// Package icons holds the player's drop-in icon art: one PNG per content id,
// flat in this directory, the file name exactly the id (assets/README.md).
// The set embeds itself, so a dropped file is wired at build time — and
// _alt/ is a subdirectory, which go:embed never reaches.
package icons

import (
	"embed"
	"image"
	_ "image/png" // the drop-in set is PNG
	"io/fs"
	"sort"
	"strings"
	"sync"
)

// FS is the embedded drop-in icon set (flat: *.png only).
//
//go:embed *.png
var FS embed.FS

var (
	idsOnce sync.Once
	ids     []string

	loadMu sync.Mutex
	cache  = map[string]image.Image{}
)

// IDs returns every embedded icon id — the file names without .png, sorted.
func IDs() []string {
	idsOnce.Do(func() {
		ents, err := fs.ReadDir(FS, ".")
		if err != nil {
			panic("icons: embedded set: " + err.Error())
		}
		for _, e := range ents {
			if name := e.Name(); strings.HasSuffix(name, ".png") {
				ids = append(ids, strings.TrimSuffix(name, ".png"))
			}
		}
		sort.Strings(ids)
	})
	return ids
}

// Load decodes the icon for id once and returns it; ok is false when the set
// ships no file for id (the caller then falls back to a manifest glyph). A
// corrupt embedded file is a build bug, so it panics loudly.
func Load(id string) (image.Image, bool) {
	loadMu.Lock()
	defer loadMu.Unlock()
	if img, ok := cache[id]; ok {
		return img, ok
	}
	f, err := FS.Open(id + ".png")
	if err != nil {
		return nil, false
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		panic("icons: decode " + id + ".png: " + err.Error())
	}
	cache[id] = img
	return img, true
}
