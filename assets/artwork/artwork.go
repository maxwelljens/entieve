// Package artwork embeds the title screen's background plate. The .jpg
// stays at this path (go:embed cannot reach a parent directory) and is
// byte-untouchable material: this file only exposes it to the engine
// (game.titleArtwork), so releases show the artwork without shipping
// assets/ next to the binary. The art pipeline (render.LoadAssets) walks
// this directory for PNG/JPEG and ignores this file.
package artwork

import _ "embed"

// Title is the title screen's backdrop, assets/artwork/title_background.jpg
// (2736×1536, 16:9, drawn cover-fit).
//
//go:embed title_background.jpg
var Title []byte
