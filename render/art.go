package render

import (
	"bytes"
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"

	"github.com/entieve/entieve/core"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/gofont/goregular"
)

// ---------------------------------------------------------------------------
// Procedural sprites (deterministic math, generated once and cached)
// ---------------------------------------------------------------------------

// glowSprite returns a radial glow of the given size and color: alpha falls
// off quadratically from an opaque center. Pure math, so results are stable
// across runs; callers cache by (size, color).
func glowSprite(size int, col color.RGBA) *ebiten.Image {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	c := float64(size-1) / 2
	r2 := c * c
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)-c, float64(y)-c
			d2 := dx*dx + dy*dy
			if d2 >= r2 {
				continue
			}
			t := math.Sqrt(d2) / c // 0 center .. 1 edge
			f := (1 - t) * (1 - t) // smooth falloff
			a := float64(col.A) * f
			if a > 255 {
				a = 255
			}
			img.SetRGBA(x, y, color.RGBA{col.R, col.G, col.B, uint8(a + 0.5)})
		}
	}
	return ebiten.NewImageFromImage(img)
}

// bodySprite returns a shaded disc (planet/moon): lambert-ish lighting from
// the upper-left with a soft limb, all deterministic math.
func bodySprite(col color.RGBA) *ebiten.Image {
	const size = 96
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	c := float64(size-1) / 2
	// Light direction (normalized, z toward viewer).
	lx, ly, lz := -0.5, -0.55, 0.67
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			nx := (float64(x) - c) / c
			ny := (float64(y) - c) / c
			r2 := nx*nx + ny*ny
			if r2 > 1 {
				continue
			}
			nz := math.Sqrt(1 - r2)
			lam := nx*lx + ny*ly + nz*lz
			if lam < 0 {
				lam = 0
			}
			shade := 0.22 + 0.78*lam
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(clamp(float64(col.R)*shade, 0, 255)),
				G: uint8(clamp(float64(col.G)*shade, 0, 255)),
				B: uint8(clamp(float64(col.B)*shade, 0, 255)),
				A: col.A,
			})
		}
	}
	return ebiten.NewImageFromImage(img)
}

// ---------------------------------------------------------------------------
// Shape templates (unit space: nose along +X, y down like screen space)
// ---------------------------------------------------------------------------

var (
	frigatePoly = []core.Vec2{
		{X: 1.0, Y: 0}, {X: 0.15, Y: 0.35}, {X: -0.6, Y: 0.42}, {X: -0.9, Y: 0.15},
		{X: -0.9, Y: -0.15}, {X: -0.6, Y: -0.42}, {X: 0.15, Y: -0.35},
	}
	destroyerPoly = []core.Vec2{
		{X: 1.1, Y: 0}, {X: 0.5, Y: 0.12}, {X: 0.2, Y: 0.3}, {X: -0.5, Y: 0.36},
		{X: -0.8, Y: 0.5}, {X: -1.0, Y: 0.2}, {X: -1.0, Y: -0.2}, {X: -0.8, Y: -0.5},
		{X: -0.5, Y: -0.36}, {X: 0.2, Y: -0.3}, {X: 0.5, Y: -0.12},
	}
	bargePoly = []core.Vec2{
		{X: 0.8, Y: 0.25}, {X: 0.95, Y: 0}, {X: 0.8, Y: -0.25}, {X: 0.2, Y: -0.5},
		{X: -0.7, Y: -0.55}, {X: -0.9, Y: -0.3}, {X: -0.9, Y: 0.3}, {X: -0.7, Y: 0.55},
		{X: 0.2, Y: 0.5},
	}
	industrialPoly = []core.Vec2{
		{X: 0.7, Y: 0.3}, {X: 1.0, Y: 0.12}, {X: 1.0, Y: -0.12}, {X: 0.7, Y: -0.3},
		{X: 0.2, Y: -0.45}, {X: -0.8, Y: -0.45}, {X: -1.0, Y: -0.2}, {X: -1.0, Y: 0.2},
		{X: -0.8, Y: 0.45}, {X: 0.2, Y: 0.45},
	}
	ratPoly = []core.Vec2{
		{X: 1.05, Y: 0}, {X: 0.4, Y: 0.28}, {X: 0.0, Y: 0.18}, {X: -0.3, Y: 0.5},
		{X: -0.9, Y: 0.35}, {X: -0.7, Y: 0}, {X: -0.9, Y: -0.35}, {X: -0.3, Y: -0.5},
		{X: 0.0, Y: -0.18}, {X: 0.4, Y: -0.28},
	}
)

// hullPoly returns the unit silhouette of a hull class.
func hullPoly(class string) []core.Vec2 {
	switch class {
	case "destroyer":
		return destroyerPoly
	case "barge":
		return bargePoly
	case "industrial":
		return industrialPoly
	case "rat":
		return ratPoly
	default:
		return frigatePoly
	}
}

// hullSize returns the visual radius in meters of a hull class.
func hullSize(class string) float64 {
	switch class {
	case "destroyer":
		return 36
	case "barge", "industrial":
		return 42
	case "rat":
		return 28
	default:
		return 26
	}
}

// rockPoly returns a jagged asteroid silhouette deterministically derived
// from seed (same seed, same rock).
func rockPoly(seed uint64, n int) []core.Vec2 {
	rng := core.RestoreRNG(seed ^ 0xA5701101)
	pts := make([]core.Vec2, n)
	for i := range pts {
		a := float64(i) / float64(n) * 2 * math.Pi
		r := 0.7 + 0.4*rng.Float64()
		pts[i] = core.Vec2{X: math.Cos(a) * r, Y: math.Sin(a) * r}
	}
	return pts
}

// ---------------------------------------------------------------------------
// Drawing helpers
// ---------------------------------------------------------------------------

// fillPoly fills a convex polygon using the renderer's cached path.
func (r *Renderer) fillPoly(dst *ebiten.Image, pts []core.Vec2, col color.Color) {
	if len(pts) < 3 {
		return
	}
	r.path.Reset()
	r.path.MoveTo(float32(pts[0].X), float32(pts[0].Y))
	for _, p := range pts[1:] {
		r.path.LineTo(float32(p.X), float32(p.Y))
	}
	r.path.Close()
	vector.FillPath(dst, &r.path, nil, &vector.DrawPathOptions{
		AntiAlias:  true,
		ColorScale: colorScale(col),
	})
}

// strokePoly strokes the outline of a polygon.
func (r *Renderer) strokePoly(dst *ebiten.Image, pts []core.Vec2, width float32, col color.Color) {
	if len(pts) < 2 {
		return
	}
	r.path.Reset()
	r.path.MoveTo(float32(pts[0].X), float32(pts[0].Y))
	for _, p := range pts[1:] {
		r.path.LineTo(float32(p.X), float32(p.Y))
	}
	r.path.Close()
	vector.StrokePath(dst, &r.path, &vector.StrokeOptions{Width: width},
		&vector.DrawPathOptions{AntiAlias: true, ColorScale: colorScale(col)})
}

// Arc strokes a circular arc from start to end angles (radians, y-down).
func Arc(dst *ebiten.Image, cx, cy, rad, start, end, width float32, col color.Color) {
	strokeArc(dst, cx, cy, rad, start, end, width, col)
}

// strokeArc strokes a circular arc from start to end angles (radians).
func strokeArc(dst *ebiten.Image, cx, cy, rad, start, end, width float32, col color.Color) {
	if rad <= 0 {
		return
	}
	var p vector.Path
	const segs = 24
	p.MoveTo(cx+float32(math.Cos(float64(start)))*rad, cy+float32(math.Sin(float64(start)))*rad)
	for i := 1; i <= segs; i++ {
		a := float64(start) + (float64(end-start))*float64(i)/segs
		p.LineTo(cx+float32(math.Cos(a))*rad, cy+float32(math.Sin(a))*rad)
	}
	vector.StrokePath(dst, &p, &vector.StrokeOptions{
		Width:   width,
		LineCap: vector.LineCapRound,
	}, &vector.DrawPathOptions{AntiAlias: true, ColorScale: colorScale(col)})
}

// colorScale converts a color to an ebiten.ColorScale.
func colorScale(col color.Color) ebiten.ColorScale {
	r, g, b, a := col.RGBA()
	var cs ebiten.ColorScale
	cs.Scale(float32(r)/0xffff, float32(g)/0xffff, float32(b)/0xffff, float32(a)/0xffff)
	return cs
}

// ---------------------------------------------------------------------------
// Text (gofont via ebiten text/v2)
// ---------------------------------------------------------------------------

// faceCache holds shared goregular faces by pixel size. Rendering happens on
// the game goroutine only, so no locking is needed.
var faceCache = map[float64]*text.GoTextFace{}

// Face returns a cached goregular face at the given pixel size.
func Face(size float64) *text.GoTextFace {
	if f, ok := faceCache[size]; ok {
		return f
	}
	src, err := text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	if err != nil {
		// goregular is constant data; a parse failure means a broken build.
		panic("render: parse goregular: " + err.Error())
	}
	f := &text.GoTextFace{Source: src, Size: size}
	faceCache[size] = f
	return f
}

// Text draws a left-top aligned string.
func Text(dst *ebiten.Image, s string, x, y, size float64, col color.Color) {
	if s == "" {
		return
	}
	opts := &text.DrawOptions{}
	opts.GeoM.Translate(x, y)
	cr, cg, cb, ca := col.RGBA()
	opts.ColorScale.Scale(float32(cr)/0xffff, float32(cg)/0xffff, float32(cb)/0xffff, float32(ca)/0xffff)
	text.Draw(dst, s, Face(size), opts)
}

// TextCentered draws a string horizontally centered on x.
func TextCentered(dst *ebiten.Image, s string, x, y, size float64, col color.Color) {
	Text(dst, s, x-Measure(s, size)/2, y, size, col)
}

// Measure returns the pixel width of s at the given size.
func Measure(s string, size float64) float64 {
	w, _ := text.Measure(s, Face(size), 0)
	return w
}

// fmtDist formats a distance in meters for world labels. Keep in sync with
// ui.FormatDistance (render must not import ui, so the logic is duplicated).
func fmtDist(m float64) string {
	switch {
	case m < 2e3:
		return groupInt(int64(math.Round(m))) + " m"
	case m < 1.496e10:
		return commaFloat(m/1000) + " km"
	default:
		return commaFloat(m/1.496e11) + " AU"
	}
}

// commaFloat formats v with one decimal and commas in the integer part.
func commaFloat(v float64) string {
	return addCommas(strconv.FormatFloat(v, 'f', 1, 64))
}

// groupInt formats n with thousands separators.
func groupInt(n int64) string {
	return addCommas(strconv.FormatInt(n, 10))
}

// addCommas inserts thousands separators into the integer part of a decimal
// string like "12345.6" or "-12345".
func addCommas(s string) string {
	head, tail, found := strings.Cut(s, ".")
	neg := strings.HasPrefix(head, "-")
	if neg {
		head = head[1:]
	}
	var b strings.Builder
	for i, c := range head {
		if i > 0 && (len(head)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	out := b.String()
	if neg {
		out = "-" + out
	}
	if found {
		out += "." + tail
	}
	return out
}
