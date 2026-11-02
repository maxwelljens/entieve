// Command hullraster rasterizes Entieve's hull portrait SVGs
// (uikit/hulls/svg/*.svg) into the PNGs the UI embeds.
//
// It is a BUILD-TIME tool: it lives in its own module so the game binary
// never links a rasterizer. The SVG subset parser and the rasterizer are
// shared with tools/emblemraster in tools/svgraster; this command fixes only
// the sizes, the design viewBox and the manifest shape. The output is
// deterministic (fixed shape order, single flat fills, the stdlib PNG
// encoder), so re-running it over unchanged sources reproduces the committed
// bytes; uikit/hulls/hulls_test.go pins that with the hashes recorded in the
// manifest.
//
// Regenerate (from the repository root):
//
//	cd tools/hullraster && go run . -svg ../../uikit/hulls/svg -out ../../uikit/hulls
//
// or `go generate ./uikit/hulls`.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/entieve/entieve/tools/svgraster"
)

// sizes are the raster sizes the UI embeds, in design px: the 120 px
// portrait plus a 2x raster for the UI scale ladder.
var sizes = []int{120, 240}

// viewBox is the SVG design size every portrait source is authored at
// (one design unit per pixel of the 120 px raster).
const viewBox = 120.0

// generator records this command in the manifest, which uikit/hulls pins.
const generator = "tools/hullraster"

// manifest mirrors uikit/hulls/manifest.json.
type manifest struct {
	Credit    string                     `json:"credit"`
	Generator string                     `json:"generator"`
	Sizes     []int                      `json:"sizes"`
	Hulls     map[string]svgraster.Entry `json:"hulls"`
}

func main() {
	svgDir := flag.String("svg", "svg", "directory of <slug>.svg sources")
	out := flag.String("out", ".", "output directory for <slug>-<size>.png + manifest.json")
	flag.Parse()
	if err := run(*svgDir, *out); err != nil {
		fmt.Fprintln(os.Stderr, "hullraster:", err)
		os.Exit(1)
	}
}

func run(svgDir, out string) error {
	hulls, err := svgraster.Build(svgDir, out, viewBox, sizes)
	if err != nil {
		return err
	}
	enc, err := json.MarshalIndent(manifest{
		Credit:    svgraster.Credit,
		Generator: generator,
		Sizes:     sizes,
		Hulls:     hulls,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "manifest.json"), append(enc, '\n'), 0o644)
}
