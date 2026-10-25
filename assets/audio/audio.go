// Package audio embeds the game's music tracks. The .ogg files stay at
// this path (go:embed cannot reach a parent directory) and are
// byte-untouchable material: this file only exposes them to the engine
// (game.NewMusic), so releases play music without shipping assets/ next
// to the binary. The art pipeline (render.LoadAssets) walks assets/ for
// PNG/JPEG only and ignores this file and the .ogg data.
package audio

import "embed"

// FS holds the six tracks (*.ogg, flat, sorted by name).
//
//go:embed *.ogg
var FS embed.FS
