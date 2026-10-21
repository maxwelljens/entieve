// Command hullraster rasterizes Entieve's hull portrait SVGs
// (uikit/hulls/svg/*.svg) into the PNGs the UI embeds.
//
// It is a BUILD-TIME tool: it lives in its own module so the game binary
// never links a rasterizer. It implements only the SVG subset the portrait
// sources actually use — filled <rect>, <circle>, <ellipse>, <polygon> and
// <path> (M/L/H/V/C/Q/Z) with a flat fill colour, no strokes, no
// transforms, no gradients — and renders through srwiley/rasterx for
// antialiasing.
//
// The output is deterministic (fixed shape order, single flat fills, the
// stdlib PNG encoder), so re-running it over unchanged sources reproduces
// the committed bytes; uikit/hulls/hulls_test.go pins that with the hashes
// recorded in the manifest.
//
// Regenerate (from the repository root):
//
//	cd tools/hullraster && go run . -svg ../../uikit/hulls/svg -out ../../uikit/hulls
//
// or `go generate ./uikit/hulls`.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/srwiley/rasterx"
	"golang.org/x/image/math/fixed"
)

// sizes are the raster sizes the UI embeds, in design px: the 120 px
// portrait plus a 2x raster for the UI scale ladder.
var sizes = []int{120, 240}

// viewBox is the SVG design size every portrait source is authored at
// (one design unit per pixel of the 120 px raster).
const viewBox = 120.0

// manifest mirrors uikit/hulls/manifest.json.
type manifest struct {
	Credit    string                  `json:"credit"`
	Generator string                  `json:"generator"`
	Sizes     []int                   `json:"sizes"`
	Hulls     map[string]manifestHull `json:"hulls"`
}

type manifestHull struct {
	Credit string            `json:"credit"` // per-file provenance: "original artwork"
	SVG    string            `json:"svg"`
	SVGSHA string            `json:"svg_sha256"`
	PNG    map[string]string `json:"png_sha256"` // size -> sha256 of the png bytes
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
	entries, err := os.ReadDir(svgDir)
	if err != nil {
		return err
	}
	var slugs []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".svg") {
			slugs = append(slugs, strings.TrimSuffix(e.Name(), ".svg"))
		}
	}
	sort.Strings(slugs)
	if len(slugs) == 0 {
		return fmt.Errorf("no .svg sources in %s", svgDir)
	}

	m := manifest{
		Credit:    "original artwork",
		Generator: "tools/hullraster",
		Sizes:     sizes,
		Hulls:     map[string]manifestHull{},
	}
	for _, slug := range slugs {
		svgPath := filepath.Join(svgDir, slug+".svg")
		raw, err := os.ReadFile(svgPath)
		if err != nil {
			return err
		}
		doc, err := parseSVG(raw)
		if err != nil {
			return fmt.Errorf("%s: %w", svgPath, err)
		}
		mc := manifestHull{
			Credit: "original artwork",
			SVG:    "svg/" + slug + ".svg",
			SVGSHA: sha(raw),
			PNG:    map[string]string{},
		}
		for _, size := range sizes {
			img := render(doc, size)
			file := fmt.Sprintf("%s-%d.png", slug, size)
			var buf strings.Builder
			if err := png.Encode(&stringWriter{&buf}, img); err != nil {
				return err
			}
			b := []byte(buf.String())
			if err := os.WriteFile(filepath.Join(out, file), b, 0o644); err != nil {
				return err
			}
			mc.PNG[strconv.Itoa(size)] = sha(b)
		}
		m.Hulls[slug] = mc
	}
	enc, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "manifest.json"), append(enc, '\n'), 0o644)
}

// stringWriter is an io.Writer over a strings.Builder (io.Writer to a
// bytes.Buffer would be simpler; strings.Builder keeps the import list
// minimal alongside the parse).
type stringWriter struct{ b *strings.Builder }

func (w *stringWriter) Write(p []byte) (int, error) { return w.b.Write(p) }

func sha(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// ---------------------------------------------------------------------------
// SVG subset
// ---------------------------------------------------------------------------

// shape is one filled primitive in document order.
type shape struct {
	fill color.RGBA
	draw func(f *rasterx.Filler, s float64) // feeds the path at scale s
}

type svgDoc struct {
	shapes []shape
}

// parseSVG decodes the shapes of the subset, in document order.
func parseSVG(raw []byte) (svgDoc, error) {
	dec := xml.NewDecoder(strings.NewReader(string(raw)))
	var doc svgDoc
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return doc, err
		}
		if t, ok := tok.(xml.StartElement); ok {
			sh, ok, err := parseElement(t)
			if err != nil {
				return doc, err
			}
			if ok {
				doc.shapes = append(doc.shapes, sh)
			}
		}
	}
	return doc, nil
}

func parseElement(t xml.StartElement) (shape, bool, error) {
	attr := map[string]string{}
	for _, a := range t.Attr {
		attr[a.Name.Local] = a.Value
	}
	fill, err := parseColor(attr["fill"])
	if err != nil {
		return shape{}, false, err
	}
	nums := func(keys ...string) ([]float64, bool) {
		out := make([]float64, 0, len(keys))
		for _, k := range keys {
			v, ok := attr[k]
			if !ok {
				return nil, false
			}
			f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err != nil {
				return nil, false
			}
			out = append(out, f)
		}
		return out, true
	}
	switch t.Name.Local {
	case "rect":
		v, ok := nums("x", "y", "width", "height")
		if !ok {
			return shape{}, false, fmt.Errorf("rect needs x/y/width/height")
		}
		x, y, w, h := v[0], v[1], v[2], v[3]
		return shape{fill: fill, draw: func(f *rasterx.Filler, s float64) {
			poly(f, s, [][2]float64{{x, y}, {x + w, y}, {x + w, y + h}, {x, y + h}})
		}}, true, nil
	case "circle":
		v, ok := nums("cx", "cy", "r")
		if !ok {
			return shape{}, false, fmt.Errorf("circle needs cx/cy/r")
		}
		return shape{fill: fill, draw: func(f *rasterx.Filler, s float64) {
			ellipse(f, s, v[0], v[1], v[2], v[2])
		}}, true, nil
	case "ellipse":
		v, ok := nums("cx", "cy", "rx", "ry")
		if !ok {
			return shape{}, false, fmt.Errorf("ellipse needs cx/cy/rx/ry")
		}
		return shape{fill: fill, draw: func(f *rasterx.Filler, s float64) {
			ellipse(f, s, v[0], v[1], v[2], v[3])
		}}, true, nil
	case "polygon", "polyline":
		pts, err := parsePoints(attr["points"])
		if err != nil {
			return shape{}, false, err
		}
		return shape{fill: fill, draw: func(f *rasterx.Filler, s float64) {
			poly(f, s, pts)
		}}, true, nil
	case "path":
		d, err := parsePath(attr["d"])
		if err != nil {
			return shape{}, false, err
		}
		return shape{fill: fill, draw: func(f *rasterx.Filler, s float64) {
			d(f, s)
		}}, true, nil
	}
	return shape{}, false, nil
}

func parseColor(v string) (color.RGBA, error) {
	v = strings.TrimSpace(v)
	if v == "" || v == "none" {
		return color.RGBA{0, 0, 0, 0}, nil
	}
	if !strings.HasPrefix(v, "#") || (len(v) != 7 && len(v) != 4) {
		return color.RGBA{}, fmt.Errorf("unsupported fill %q (hex only)", v)
	}
	hexv := v[1:]
	if len(hexv) == 3 {
		hexv = string([]byte{hexv[0], hexv[0], hexv[1], hexv[1], hexv[2], hexv[2]})
	}
	n, err := strconv.ParseUint(hexv, 16, 32)
	if err != nil {
		return color.RGBA{}, err
	}
	return color.RGBA{uint8(n >> 16), uint8(n >> 8), uint8(n), 255}, nil
}

func parsePoints(s string) ([][2]float64, error) {
	fields := strings.FieldsFunc(strings.TrimSpace(s), func(r rune) bool {
		return r == ' ' || r == ',' || r == '\n' || r == '\t' || r == '\r'
	})
	if len(fields)%2 != 0 || len(fields) < 6 {
		return nil, fmt.Errorf("polygon points %q: want x,y pairs", s)
	}
	out := make([][2]float64, 0, len(fields)/2)
	for i := 0; i < len(fields); i += 2 {
		x, err1 := strconv.ParseFloat(fields[i], 64)
		y, err2 := strconv.ParseFloat(fields[i+1], 64)
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("polygon points %q: %v/%v", s, err1, err2)
		}
		out = append(out, [2]float64{x, y})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Path data (M/L/H/V/C/Q/Z, absolute and relative)
// ---------------------------------------------------------------------------

// parsePath compiles an SVG path (the supported commands only) into a filler
// callback. Every subpath is closed: the portraits are filled marks.
func parsePath(d string) (func(*rasterx.Filler, float64), error) {
	ops, err := tokenizePath(d)
	if err != nil {
		return nil, err
	}
	return func(f *rasterx.Filler, s float64) {
		runPath(f, s, ops)
	}, nil
}

type pathOp struct {
	cmd  byte
	args []float64
}

func tokenizePath(d string) ([]pathOp, error) {
	var ops []pathOp
	i := 0
	for i < len(d) {
		c := d[i]
		if c == ' ' || c == ',' || c == '\n' || c == '\t' || c == '\r' {
			i++
			continue
		}
		if !strings.ContainsRune("MmLlHhVvCcQqZz", rune(c)) {
			return nil, fmt.Errorf("unsupported path command %q in %q", string(c), d)
		}
		cmd := c
		i++
		var args []float64
		for i < len(d) {
			j := i
			for j < len(d) && (d[j] == ' ' || d[j] == ',' || d[j] == '\n' || d[j] == '\t' || d[j] == '\r') {
				j++
			}
			if j >= len(d) {
				i = j
				break
			}
			// Stop at the next command letter.
			if strings.ContainsRune("MmLlHhVvCcQqZz", rune(d[j])) {
				i = j
				break
			}
			k := j
			for k < len(d) && (d[k] == '.' || d[k] == '-' || d[k] == '+' || d[k] == 'e' || d[k] == 'E' || (d[k] >= '0' && d[k] <= '9')) {
				k++
			}
			v, err := strconv.ParseFloat(d[j:k], 64)
			if err != nil {
				return nil, fmt.Errorf("path number %q: %v", d[j:k], err)
			}
			args = append(args, v)
			i = k
		}
		ops = append(ops, pathOp{cmd: cmd, args: args})
	}
	return ops, nil
}

// argCount is the numbers each command consumes.
func argCount(cmd byte) int {
	switch cmd {
	case 'M', 'm', 'L', 'l':
		return 2
	case 'H', 'h', 'V', 'v':
		return 1
	case 'C', 'c':
		return 6
	case 'Q', 'q':
		return 4
	}
	return 0
}

// runPath feeds the ops to the filler at scale s, tracking the current
// point for relative commands.
func runPath(f *rasterx.Filler, s float64, ops []pathOp) {
	var cx, cy float64
	var startX, startY float64
	var penDown bool
	for _, op := range ops {
		c := op.cmd
		n := argCount(c)
		if n == 0 {
			if c == 'Z' || c == 'z' {
				if penDown {
					f.Line(pt(startX, startY, s))
					f.Stop(true)
					penDown = false
					cx, cy = startX, startY
				}
			}
			continue
		}
		for a := 0; a+n <= len(op.args); a += n {
			args := op.args[a : a+n]
			switch c {
			case 'M', 'm':
				x, y := args[0], args[1]
				if c == 'm' {
					x, y = cx+x, cy+y
				}
				if penDown {
					f.Stop(true) // an implicit close of the open subpath
				}
				f.Start(pt(x, y, s))
				penDown = true
				cx, cy, startX, startY = x, y, x, y
			case 'L', 'l':
				x, y := args[0], args[1]
				if c == 'l' {
					x, y = cx+x, cy+y
				}
				f.Line(pt(x, y, s))
				cx, cy = x, y
			case 'H', 'h':
				x := args[0]
				if c == 'h' {
					x = cx + x
				}
				f.Line(pt(x, cy, s))
				cx = x
			case 'V', 'v':
				y := args[0]
				if c == 'v' {
					y = cy + y
				}
				f.Line(pt(cx, y, s))
				cy = y
			case 'C', 'c':
				x1, y1, x2, y2, x, y := args[0], args[1], args[2], args[3], args[4], args[5]
				if c == 'c' {
					x1, y1, x2, y2, x, y = cx+x1, cy+y1, cx+x2, cy+y2, cx+x, cy+y
				}
				f.CubeBezier(pt(x1, y1, s), pt(x2, y2, s), pt(x, y, s))
				cx, cy = x, y
			case 'Q', 'q':
				x1, y1, x, y := args[0], args[1], args[2], args[3]
				if c == 'q' {
					x1, y1, x, y = cx+x1, cy+y1, cx+x, cy+y
				}
				f.QuadBezier(pt(x1, y1, s), pt(x, y, s))
				cx, cy = x, y
			}
		}
	}
	if penDown {
		f.Stop(true)
	}
}

func pt(x, y, s float64) fixed.Point26_6 {
	return fixed.Point26_6{
		X: fixed.Int26_6(math.Round(x * s * 64)),
		Y: fixed.Int26_6(math.Round(y * s * 64)),
	}
}

// poly feeds a closed polygon.
func poly(f *rasterx.Filler, s float64, pts [][2]float64) {
	if len(pts) == 0 {
		return
	}
	f.Start(pt(pts[0][0], pts[0][1], s))
	for _, p := range pts[1:] {
		f.Line(pt(p[0], p[1], s))
	}
	f.Stop(true)
}

// ellipse feeds an axis-aligned ellipse as four cubic beziers (kappa).
func ellipse(f *rasterx.Filler, s, cx, cy, rx, ry float64) {
	const k = 0.5522847498307936
	ox, oy := rx*k, ry*k
	f.Start(pt(cx+rx, cy, s))
	f.CubeBezier(pt(cx+rx, cy+oy, s), pt(cx+ox, cy+ry, s), pt(cx, cy+ry, s))
	f.CubeBezier(pt(cx-ox, cy+ry, s), pt(cx-rx, cy+oy, s), pt(cx-rx, cy, s))
	f.CubeBezier(pt(cx-rx, cy-oy, s), pt(cx-ox, cy-ry, s), pt(cx, cy-ry, s))
	f.CubeBezier(pt(cx+ox, cy-ry, s), pt(cx+rx, cy-oy, s), pt(cx+rx, cy, s))
	f.Stop(true)
}

// render rasterizes the document into an anti-aliased size×size NRGBA.
func render(doc svgDoc, size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	s := float64(size) / viewBox
	sc := rasterx.NewScannerGV(size, size, img, img.Bounds())
	f := rasterx.NewFiller(size, size, sc)
	f.SetWinding(true)
	for _, sh := range doc.shapes {
		if sh.fill.A == 0 {
			continue
		}
		sc.SetColor(sh.fill)
		sh.draw(f, s)
		f.Draw()
		f.Clear()
	}
	return img
}
