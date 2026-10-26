// Package sfx embeds the five UI sound cues (click, open, close, confirm,
// refuse). The .wav files stay at this path (go:embed cannot reach a
// parent directory) and are the user's local SFX pack, transcoded to
// 16-bit/44.1 kHz so ebiten's wav decoder can read them; see assets/README
// for provenance. Only game.NewUISounds reads them.
package sfx

import "embed"

// FS holds the five cues (*.wav, flat, sorted by name).
//
//go:embed *.wav
var FS embed.FS
