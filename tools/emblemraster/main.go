// Command emblemraster rasterizes Entieve's corporation emblem SVGs
// (uikit/emblems/svg/*.svg) into the arena PNGs the UI embeds.
//
// It is a BUILD-TIME tool: it lives in its own module so the game binary
// never links a rasterizer. The SVG subset parser and the rasterizer are
// shared with tools/hullraster in tools/svgraster; this command fixes only
// the sizes, the design viewBox and the manifest shape. The output is
// deterministic (fixed shape order, single flat fills, the stdlib PNG
// encoder), so re-running it over unchanged sources reproduces the committed
// bytes; uikit/emblems/emblems_test.go pins that with the hashes recorded in
// the manifest.
//
// Regenerate (from the repository root):
//
//	cd tools/emblemraster && go run . -svg ../../uikit/emblems/svg -out ../../uikit/emblems
//
// or `go generate ./uikit/emblems`.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/entieve/entieve/tools/svgraster"
)

// sizes are the raster sizes the UI embeds, in design px.
var sizes = []int{24, 32, 48}

// viewBox is the SVG design size every emblem source is authored at.
const viewBox = 48.0

// generator records this command in the manifest, which uikit/emblems pins.
const generator = "tools/emblemraster"

// manifest mirrors uikit/emblems/manifest.json.
type manifest struct {
	Credit    string                     `json:"credit"`
	Generator string                     `json:"generator"`
	Sizes     []int                      `json:"sizes"`
	Emblems   map[string]svgraster.Entry `json:"emblems"`
}

func main() {
	svgDir := flag.String("svg", "svg", "directory of <slug>.svg sources")
	out := flag.String("out", ".", "output directory for <slug>-<size>.png + manifest.json")
	flag.Parse()
	if err := run(*svgDir, *out); err != nil {
		fmt.Fprintln(os.Stderr, "emblemraster:", err)
		os.Exit(1)
	}
}

func run(svgDir, out string) error {
	emblems, err := svgraster.Build(svgDir, out, viewBox, sizes)
	if err != nil {
		return err
	}
	enc, err := json.MarshalIndent(manifest{
		Credit:    svgraster.Credit,
		Generator: generator,
		Sizes:     sizes,
		Emblems:   emblems,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "manifest.json"), append(enc, '\n'), 0o644)
}
