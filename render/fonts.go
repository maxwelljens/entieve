package render

// Text faces: Turret Road, the typeface Entieve ships with.
//
// The TTFs and their OFL.txt live in render/fonts/ and are embedded as
// one directory, so the license travels with the fonts inside the binary
// as well as in the repo (see docs/ART.md). Rendering happens on the game
// goroutine only, so the parse and face caches need no locking.
//
// Weight → role (the 21 px type scale is unchanged — only the typeface
// and the emphasis face are new):
//
//	Regular  body text: log lines, table cells, labels, tooltips, input
//	Bold     emphasis: window titles, table header sort buttons, buttons,
//	         HUD readouts — Medium sat too close to Regular at the 16.8 px
//	         button size to read as emphasis (compared by eye, see ART.md)
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

// The three weights the interface draws with.
const (
	regularFile  = "fonts/TurretRoad-Regular.ttf"
	emphasisFile = "fonts/TurretRoad-Medium.ttf"
	lightFile    = "fonts/TurretRoad-Light.ttf"
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
