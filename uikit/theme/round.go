package theme

import (
	"image"
	"image/color"
	"math"
	"sync"

	eimage "github.com/ebitenui/ebitenui/image"
	"github.com/hajimehoshi/ebiten/v2"
)

// Rounded chrome — the Cyber Terminal kit's form language, in the house
// palette (docs/lore/AESTHETIC.md, "Form language"). Three radius tokens,
// all design px through Px:
//
//	14  window pieces: the title bar's TOP corners, the body's BOTTOM corners
//	 8  in-window panels and cards
//	 5  controls: buttons, inputs, menus, tooltips, checkbox, slider
//
// Inner scroll viewports keep square masks: ebiten's clip is rectangular,
// so a rounded mask is the one corner the kit look cannot reach here.
const (
	RadiusWindow  = 14
	RadiusPanel   = 8
	RadiusControl = 5
)

// The generator families — a screen asks for a PIECE, never for a radius
// (docs/ART.md, "Chrome and form", carries the visual canon; this is the
// derivation the code guarantees):
//
//	window body piece   WindowBodyBox: bottom corners, panel depth. The
//	                    colour is WindowBody(palette) — the WindowAlpha token.
//	window title piece  WindowTitleBox: top corners, flat opaque BGHard fill.
//	                    Body + title share the seam: square there, rounded out.
//	panel               PanelBox: 8 px all round, PanelTop→PanelBottom depth.
//	control             Bordered (theme.go): 5 px all round, flat. Buttons,
//	                    inputs, lists, tooltips, the context menu.
//	banded viewport     BandedBox: control radii with the fill starting under
//	                    a plate band (widgets.PlateBand); flat, so the chrome
//	                    does not change when a plate unloads.
//	well                WellBox: panel radii, the opaque BGHard fill and a
//	                    hairline edge — a dark plate laid on the body.
//	square              Rounded(…, Radii{}): view masks, meter strips, seams.
//
// Naming map: WindowAlpha is the one translucency token and WindowBody the
// palette colour it applies to (chrome.go); PanelTop/PanelBottom are the depth
// ramp's two ends derived FROM that body colour — not a second translucency
// rule. Radii are per box, so no screen passes a token where a piece exists.
//
// Radii are a box's four corner radii in design px, clockwise from the top
// left. Zero is square, which is what the window seam and the few
// deliberately square strips (view masks, meter bars) use.
type Radii struct{ TL, TR, BR, BL int }

func AllRadii(r int) Radii    { return Radii{r, r, r, r} }
func TopRadii(r int) Radii    { return Radii{r, r, 0, 0} }
func BottomRadii(r int) Radii { return Radii{0, 0, r, r} }

// depthRows is the source resolution of the panel-depth gradient: the
// nine-slice's centre row keeps these rows and is stretched to the box, so
// the ramp is rendered (not a two-tone edge band) without knowing the
// panel's height at generation time.
const depthRows = 256

// PanelTop and PanelBottom are the panel-depth gradient's ends, derived
// from existing roles only: the top lifts the body a hair toward HeadBg
// (the palette's lighter panel tone), the bottom keeps the body's own RGB
// four alpha steps more opaque. On the default palette the ramp is
// #101523→#0a0e14 with alpha 226→234: a delta no colour pick can read as a
// hue change (ΔRGB ≤ 6, both sides of the 230 WindowAlpha midpoint).
func PanelTop(p Palette, body color.NRGBA) color.NRGBA {
	const lift = 0.35
	mix := func(b, h uint8) uint8 {
		return uint8(math.Round(float64(b)*(1-lift) + float64(h)*lift))
	}
	return color.NRGBA{
		R: mix(body.R, p.HeadBg.R),
		G: mix(body.G, p.HeadBg.G),
		B: mix(body.B, p.HeadBg.B),
		A: uint8(max(int(body.A)-4, 0)),
	}
}

func PanelBottom(p Palette, body color.NRGBA) color.NRGBA {
	c := body
	c.A = uint8(min(int(body.A)+4, 255))
	return c
}

// Rounded builds a flat bordered box with per-corner radii. The border is
// w design px, drawn inside the box edge.
func Rounded(body, edge color.NRGBA, w int, radii Radii) *eimage.NineSlice {
	return roundedBox(body, body, edge, w, radii)
}

// PanelBox is an in-window panel or card: 8 px all round, body fill
// deepening from PanelTop to PanelBottom.
func PanelBox(p Palette, body, edge color.NRGBA) *eimage.NineSlice {
	return roundedBox(PanelTop(p, body), PanelBottom(p, body), edge, 1, AllRadii(RadiusPanel))
}

// WellFill is the fill of a dark recessed plate laid ON a window body (the
// Show Info icon well): the opaque BGHard role — the palette's solid dark,
// the surface title bars and tab backs layer with — never the translucent
// body it has to stay apart from.
func WellFill(p Palette) color.NRGBA { return Solid(p.BGHard) }

// WellBox is that plate: WellFill, panel radius, hairline edge. Flat (no
// depth ramp): its job is to separate one thing from the panel around it.
func WellBox(p Palette, edge color.NRGBA) *eimage.NineSlice {
	return roundedBox(WellFill(p), WellFill(p), edge, 1, AllRadii(RadiusPanel))
}

// WindowBodyBox is a window's body under the title bar: bottom corners
// rounded only (the seam with the title bar stays square), panel depth.
func WindowBodyBox(p Palette, body, edge color.NRGBA) *eimage.NineSlice {
	return roundedBox(PanelTop(p, body), PanelBottom(p, body), edge, 1, BottomRadii(RadiusWindow))
}

// WindowTitleBox is a window's title bar: top corners rounded only, the
// opaque BGHard fill (title bars never take the body depth ramp).
func WindowTitleBox(body, edge color.NRGBA) *eimage.NineSlice {
	return roundedBox(body, body, edge, 1, TopRadii(RadiusWindow))
}

// BandedBox is Bordered with its body fill starting fillTop design px down:
// the rows above that line paint the border and no fill, so a window's
// artwork band reads through a viewport's top rows. It rounds like the
// plain box and stays flat like it too (scroll viewports carry no depth
// ramp), so the chrome does not change when a plate unloads.
//
// fillTop blanks the body fill only: the border — corner arcs included —
// always paints, so a deep band never opens the box's edge (the invariant a
// banded viewport's border ring depends on).
func BandedBox(body, edge color.NRGBA, fillTop int) *eimage.NineSlice {
	return roundedBoxAt(body, body, edge, 1, AllRadii(RadiusControl), Px(fillTop))
}

// roundedBox rasterises a bordered, per-corner-rounded box and wraps it as
// a nine-slice: the corner cells hold the arcs, the centre row is the
// depth ramp (top→bottom) and the centre column a one-px fill.
//
// Rasteriser contract (roundedBoxAt and below): input is device px except the
// radii, which scale through Px; each side's pad is max(its corner radius,
// border) so an arc never leaks out of the box; the nine-slice's centre row
// is depthRows source rows (the ramp) stretched to the box's height, so the
// depth is rendered without knowing the height here. Rows above fillTop lose
// the body FILL only (the banded viewport lets a plate read through); the
// border always paints, corner arcs included, so the box's edge never opens.
// fillTop is device px and is folded into the top pad.
func roundedBox(top, bottom, edge color.NRGBA, w int, radii Radii) *eimage.NineSlice {
	return roundedBoxAt(top, bottom, edge, w, radii, 0)
}

// roundedBoxAt is roundedBox with the fill suppressed for the first fillTop
// device px, which is how the banded viewport keeps its border ring while
// the artwork shows through. The top slice is padT rows — the corner arcs —
// and every one of them is filled (fillTop only blanks rows the caller asked
// for), so a box's top and bottom bands read its fill edge to edge.
func roundedBoxAt(top, bottom, edge color.NRGBA, w int, radii Radii, fillTop int) *eimage.NineSlice {
	img, padX, padY := roundedBoxRaster(top, bottom, edge, w, radii, fillTop)
	return eimage.NewNineSlice(ebiten.NewImageFromImage(img), padX, padY)
}

// roundedBoxRaster rasterises the box and returns it with its nine-slice
// cuts (columns, rows) — the seam round_test.go reads pixels through.
func roundedBoxRaster(top, bottom, edge color.NRGBA, w int, radii Radii, fillTop int) (*image.NRGBA, [3]int, [3]int) {
	r := scaleRadii(radii)
	b := max(Px(w), 1)
	padT := max(max(r.TL, r.TR), b, fillTop)
	padB := max(max(r.BL, r.BR), b)
	padL := max(max(r.TL, r.BL), b)
	padR := max(max(r.TR, r.BR), b)

	img := roundRectImage(padL+padR+1, padT+depthRows+padB, top, bottom, edge, b, r, padT, fillTop)
	return img, [3]int{padL, 1, padR}, [3]int{padT, depthRows, padB}
}

// scaleRadii converts design radii to device px.
func scaleRadii(r Radii) Radii {
	return Radii{Px(r.TL), Px(r.TR), Px(r.BR), Px(r.BL)}
}

// roundRectImage rasterises a rounded rect of size w×h whose fill ramps
// from top to bottom starting gradTop px down (rows above it are the top
// colour, so a box's top band reads exact). Anti-aliased by signed
// distance coverage: no supersampling, one pass.
func roundRectImage(w, h int, top, bottom, edge color.NRGBA, border int, r Radii, gradTop, fillFrom int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	// Radii can never exceed half the box: clamp before the SDF.
	r = clampRadii(r, w, h)
	fw, fh := float64(w), float64(h)
	fb := float64(border)
	span := float64(max(h-gradTop, 1))
	for y := 0; y < h; y++ {
		t := 0.0
		if y > gradTop {
			t = math.Min(1, float64(y-gradTop)/span)
		}
		body := mixNRGBA(top, bottom, t)
		for x := 0; x < w; x++ {
			d := roundSDF(float64(x)+0.5, float64(y)+0.5, fw, fh, r)
			cov := clamp01(0.5 - d)          // box coverage
			inner := clamp01(0.5 - (d + fb)) // fill coverage (inside the border)
			fill := inner
			if y < fillFrom {
				fill = 0 // banded: the artwork shows through above the fill line
			}
			bodyA := fill * float64(body.A) / 255
			edgeA := (cov - inner) * float64(edge.A) / 255
			a := bodyA + edgeA
			if a <= 0 {
				continue
			}
			img.Pix[y*img.Stride+x*4+0] = uint8((float64(body.R)*bodyA + float64(edge.R)*edgeA) / a)
			img.Pix[y*img.Stride+x*4+1] = uint8((float64(body.G)*bodyA + float64(edge.G)*edgeA) / a)
			img.Pix[y*img.Stride+x*4+2] = uint8((float64(body.B)*bodyA + float64(edge.B)*edgeA) / a)
			img.Pix[y*img.Stride+x*4+3] = uint8(math.Round(a * 255))
		}
	}
	return img
}

// roundSDF is the signed distance to a per-corner rounded rect: negative
// inside, in device px.
func roundSDF(x, y, w, h float64, r Radii) float64 {
	cx, cy := w/2, h/2
	var rr float64
	switch {
	case x < cx && y < cy:
		rr = float64(r.TL)
	case x >= cx && y < cy:
		rr = float64(r.TR)
	case x >= cx && y >= cy:
		rr = float64(r.BR)
	default:
		rr = float64(r.BL)
	}
	qx := math.Abs(x-cx) - (cx - rr)
	qy := math.Abs(y-cy) - (cy - rr)
	return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - rr
}

func clampRadii(r Radii, w, h int) Radii {
	maxR := min(w, h) / 2
	cl := func(v int) int { return max(min(v, maxR), 0) }
	return Radii{cl(r.TL), cl(r.TR), cl(r.BR), cl(r.BL)}
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func mixNRGBA(a, b color.NRGBA, t float64) color.NRGBA {
	m := func(x, y uint8) uint8 { return uint8(math.Round(float64(x)*(1-t) + float64(y)*t)) }
	return color.NRGBA{R: m(a.R, b.R), G: m(a.G, b.G), B: m(a.B, b.B), A: m(a.A, b.A)}
}

// DrawRoundBox paints a bordered rounded box at r: the chrome hand-drawn
// surfaces (map card, menu, tooltip, HUD cards) use this where a widget
// background cannot reach. Images are cached by geometry and colour: the
// HUD repaints its cards every frame.
func DrawRoundBox(dst *ebiten.Image, r image.Rectangle, body, edge color.NRGBA, w int, radii Radii) {
	if r.Empty() {
		return
	}
	img := roundBoxImage(r.Dx(), r.Dy(), body, edge, w, radii)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(r.Min.X), float64(r.Min.Y))
	dst.DrawImage(img, op)
}

// roundKey is the cache's identity: everything that changes a pixel of the
// drawn box — device size, both colours, border and the four device radii.
// fillTop is deliberately absent: the cache serves DrawRoundBox only, which
// always draws fillTop 0; the banded nine-slice path rasterises once per
// BandedBox call and never reads this cache.
type roundKey struct {
	w, h           int
	body, edge     color.NRGBA
	border         int
	tl, tr, br, bl int
}

// roundCache bounds the HUD's per-card draws: a handful of distinct sizes per
// frame, so 128 entries never thrash; past that the map is cleared wholesale
// (simpler than an LRU, and the next frame re-rasterises the visible set).
var roundCache = struct {
	sync.Mutex
	m map[roundKey]*ebiten.Image
}{m: map[roundKey]*ebiten.Image{}}

func roundBoxImage(w, h int, body, edge color.NRGBA, border int, radii Radii) *ebiten.Image {
	r := scaleRadii(radii)
	k := roundKey{w, h, body, edge, max(Px(border), 1), r.TL, r.TR, r.BR, r.BL}
	roundCache.Lock()
	defer roundCache.Unlock()
	if img, ok := roundCache.m[k]; ok {
		return img
	}
	if len(roundCache.m) > 128 {
		roundCache.m = map[roundKey]*ebiten.Image{}
	}
	img := ebiten.NewImageFromImage(roundRectImage(w, h, body, body, edge, k.border, r, 0, 0))
	roundCache.m[k] = img
	return img
}
