package render

// Text faces: Turret Road is the display typeface Entieve ships with,
// Source Code Pro the mono face for tabular contents.
//
// The TTFs and their license files live in render/fonts/ and are embedded
// as one directory, so the licenses travel with the fonts inside the
// binary as well as in the repo (see docs/ART.md). Rendering happens on
// the game goroutine only, so the parse and face caches need no locking.
//
// Weight → role (the 21 px type scale is unchanged — only the typeface
// and the emphasis face are new):
//
//	Regular  body text: log lines, labels, tooltips, input, table cells
//	         outside the mono tables (see Mono below)
//	Bold     emphasis: window titles, table header sort buttons, buttons,
//	         HUD readouts — Medium sat too close to Regular at the 16.8 px
//	         button size to read as emphasis (compared by eye, see ART.md)
//	Mono     tabular contents: inventory and market table rows, where
//	         monospaced digits and entries line the columns up (MonoFace)
//
// Turret Road has no U+25B2/U+25BC triangles, so the table sort
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
	regularFile  = "fonts/TurretRoad-Regular.ttf"
	emphasisFile = "fonts/TurretRoad-Medium.ttf"
	lightFile    = "fonts/TurretRoad-Light.ttf"
	// monoFile is Source Code Pro, the tabular contents face. It is a
	// variable font (wght axis); text/v2's GoTextFace draws the default
	// instance (Regular 400) — exactly the weight table contents need,
	// and nothing the interface asks a variation for.
	monoFile = "fonts/SourceCodePro-VariableFont_wght.ttf"
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

// Face returns the body face (Turret Road Regular) at the given pixel
// size: everything the interface writes by default.
func Face(size float64) *text.GoTextFace { return face(regularFile, size) }

// FaceBold returns the emphasis face (Turret Road Bold) at the given
// pixel size: window titles, table header sort buttons, buttons and HUD
// emphasis. Additive — Face is unchanged, callers opt in per role.
func FaceBold(size float64) *text.GoTextFace { return face(emphasisFile, size) }

// MonoFace returns the tabular contents face (Source Code Pro Regular —
// the variable font's default instance) at the given pixel size: table
// rows that carry numbers and listed entries (inventory, market), where
// monospaced cells line the columns up. Additive — Face is unchanged,
// callers opt in per table (uikit's ContentsFace option).
func MonoFace(size float64) *text.GoTextFace { return face(monoFile, size) }
