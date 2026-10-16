package render

// Text faces: IBM Plex Sans is the display typeface Entieve ships with,
// IBM Plex Mono the mono face for tabular contents.
//
// The TTFs and their license files live in render/fonts/ and are embedded
// as one directory, so the licenses travel with the fonts inside the
// binary as well as in the repo (see docs/ART.md). Rendering happens on
// the game goroutine only, so the parse and face caches need no locking.
//
// Weight → role (20 px type scale — Plex's line box runs ~19 % taller
// than the face it replaced, so the base came down one pixel):
//
//	Regular  body text: log lines, labels, tooltips, input, table cells
//	         outside the mono tables (see Mono below)
//	Bold     emphasis: window titles, table header sort buttons, buttons,
//	         HUD readouts — Medium sat too close to Regular at the 16 px
//	         button size to read as emphasis (compared by eye, see ART.md)
//	Mono     tabular contents: inventory and market table rows, where
//	         monospaced digits and entries line the columns up (MonoFace)
//
// IBM Plex Sans has no U+25B2/U+25BC triangles, so the table sort
// indicators are the ↑/↓ arrows it does carry (uikit/table.go).

import (
	"bytes"
	"embed"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

//go:embed fonts
var fontFS embed.FS

// The weights the interface draws with, plus the mono contents face.
const (
	regularFile  = "fonts/IBMPlexSans-Regular.ttf"
	emphasisFile = "fonts/IBMPlexSans-Medium.ttf"
	lightFile    = "fonts/IBMPlexSans-Light.ttf"
	// monoFile is IBM Plex Mono, the tabular contents face: a static
	// Regular 400 TTF — exactly the weight table contents need, and
	// nothing the interface asks a variation for.
	monoFile = "fonts/IBMPlexMono-Regular.ttf"
)

// faceKey identifies one cached face: weight file + pixel size.
type faceKey struct {
	file string
	size float64
}

var (
	// sources caches the parsed TTF per file (parsing twice would waste
	// ~50 kB per face); faceCache caches the sized face per key.
	sources   = map[string]*text.GoTextFaceSource{}
	faceCache = map[faceKey]*text.GoTextFace{}
)

// source parses one embedded weight once. The files are constant data:
// a parse failure means a broken build, so it panics rather than
// degrading to silently missing text.
func source(file string) *text.GoTextFaceSource {
	if s, ok := sources[file]; ok {
		return s
	}
	raw, err := fontFS.ReadFile(file)
	if err != nil {
		panic("render: read embedded font " + file + ": " + err.Error())
	}
	s, err := text.NewGoTextFaceSource(bytes.NewReader(raw))
	if err != nil {
		panic("render: parse " + file + ": " + err.Error())
	}
	sources[file] = s
	return s
}

// face returns a cached face of the given weight at the given pixel size.
func face(file string, size float64) *text.GoTextFace {
	k := faceKey{file: file, size: size}
	if f, ok := faceCache[k]; ok {
		return f
	}
	f := &text.GoTextFace{Source: source(file), Size: size}
	faceCache[k] = f
	return f
}

// Face returns the body face (IBM Plex Sans Regular) at the given pixel
// size: everything the interface writes by default.
func Face(size float64) *text.GoTextFace { return face(regularFile, size) }

// FaceBold returns the emphasis face (emphasisFile, the emphasis weight
// of IBM Plex Sans) at the given pixel size: window titles, table header
// sort buttons, buttons and HUD emphasis. Additive — Face is unchanged,
// callers opt in per role.
func FaceBold(size float64) *text.GoTextFace { return face(emphasisFile, size) }

// MonoFace returns the tabular contents face (IBM Plex Mono Regular) at
// the given pixel size: table rows that carry numbers and listed entries
// (inventory, market), where monospaced cells line the columns up.
// Additive — Face is unchanged, callers opt in per table (uikit's
// ContentsFace option).
func MonoFace(size float64) *text.GoTextFace { return face(monoFile, size) }
