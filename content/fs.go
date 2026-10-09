package content

import (
	"embed"
	"io/fs"
)

// dataFS embeds the whole content/data directory at build time.
//
//go:embed data
var dataFS embed.FS

// FS returns the embedded game-data filesystem with the data directory as
// its root, so paths look like "items/minerals.json", "systems/solace.json",
// "universe.json". This is the filesystem passed to Load for the shipping
// game: content.Load(content.FS()).
func FS() fs.FS {
	sub, err := fs.Sub(dataFS, "data")
	if err != nil {
		// Impossible while //go:embed data matches at least one file.
		panic("content: embedded data subtree missing: " + err.Error())
	}
	return sub
}
