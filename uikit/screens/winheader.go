package screens

import (
	"image"

	eimage "github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// Window artwork headers (docs/ART.md: "Window headers").
//
// A service window may carry one plate — the Refinery its refining plate, the
// Manufacturing its manufacturing one — in a band directly under the title
// bar, as part of the window background: the band paints the panel, the plate
// over it and only then the body, so the first rows of the list sit ON the
// image and the lists scroll over it (the body is painted whole above the
// band, never clipped out of it). The table leaves its own viewport fill off
// under the band (widgets.PlateBand), so the plate reads through those rows;
// the fill returns at the band's bottom edge.
//
// The plate is a centre crop of the 16:9 source at alpha headerTopAlpha on
// the title edge, fading linearly to nothing at the band's bottom edge, and
// fades in over the house 12-frame open (openFrames, winanim.go). No plate in
// the asset root = no band: the panel paints alone, the window exactly as
// before (release builds ship no assets/).

const (
	headerBandPx   = 120 // band height under the title bar, design px
	headerTopAlpha = 180 // plate alpha at the title edge (0 at the band bottom)
)

// headerBand is a window body wrapper that owns the body's panel background,
// so it can paint the plate between the panel and the content — the one draw
// order a Container cannot express (it draws its background, then children).
//
// Contract:
//   - ONE child, a stretched grid: the body is the wrapper's only child, so
//     the wrapper adds no geometry — it supplies the draw order and tracks
//     hover for the body (whose background image used to do that).
//   - One plate per window: the window's constructor passes the resolver. A
//     nil resolver (no assets/, no plate for the kind) is the no-band case —
//     the panel paints alone, byte-identical to the plain body it replaced.
//   - The fade counter (0..openFrames) lives here and advances in Render: it
//     scales the plate's alpha only, never the layout.
//   - No clip interaction: the body renders whole over the band, so rows
//     scroll OVER the pinned plate; the viewport's own fill is what steps
//     aside (widgets.PlateBand), never a clip.
type headerBand struct {
	*widget.Container

	panel *eimage.NineSlice
	plate func() *ebiten.Image // resolved per draw: the asset root probes lazily

	fade int // open fade, 0..openFrames
}

// newHeaderBand wraps a window body with its artwork header. body must carry
// no BackgroundImage: the band paints the panel it used to carry.
func newHeaderBand(body widget.PreferredSizeLocateableWidget, panel *eimage.NineSlice, plate func() *ebiten.Image) *headerBand {
	h := &headerBand{
		Container: widget.NewContainer(
			// The pointer tracks the body panel still: its background image
			// used to be what made the body hoverable.
			widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.TrackHover(true)),
			widget.ContainerOpts.Layout(widget.NewGridLayout(
				widget.GridLayoutOpts.Columns(1),
				widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
			)),
		),
		panel: panel,
		plate: plate,
	}
	h.AddChild(body)
	return h
}

// Render paints the body's surface in the one order the header needs: panel,
// plate, content.
func (h *headerBand) Render(screen *ebiten.Image) {
	if h.fade < openFrames {
		h.fade++
	}
	if r := h.GetWidget().Rect; !r.Empty() {
		if h.panel != nil {
			h.panel.Draw(screen, r.Dx(), r.Dy(), func(o *ebiten.DrawImageOptions) {
				o.GeoM.Translate(float64(r.Min.X), float64(r.Min.Y))
			})
		}
		if h.plate != nil {
			if plate := h.plate(); plate != nil {
				drawHeaderBand(screen, r, plate, h.fade)
			}
		}
	}
	h.Container.Render(screen) // lays the body out, then paints it over the band
}

// headerBandRect is the band's rect: the body's full width, its top
// headerBandPx design px (the body hangs directly off the title bar).
func headerBandRect(body image.Rectangle) image.Rectangle {
	if body.Empty() {
		return image.Rectangle{}
	}
	return image.Rect(body.Min.X, body.Min.Y, body.Max.X, body.Min.Y+min(theme.Px(headerBandPx), body.Dy()))
}

// headerAlpha is the plate's alpha at row y of a band h rows tall at open
// fade f: headerTopAlpha on the title edge, linear down to 0 at the bottom.
func headerAlpha(y, h, f int) uint8 {
	if h <= 0 || y >= h || f <= 0 {
		return 0
	}
	return uint8(headerTopAlpha * (h - y) * min(f, openFrames) / (h * openFrames))
}

// drawHeaderBand paints the plate cover-fit into the band, its source
// centre-cropped to the band's aspect so the two vertex alphas anchor the
// gradient to the band's own edges (one quad, linear between them).
func drawHeaderBand(dst *ebiten.Image, body image.Rectangle, plate *ebiten.Image, fade int) {
	band := headerBandRect(body)
	src := plate.Bounds()
	s := render.CoverScale(src.Dx(), src.Dy(), band.Dx(), band.Dy())
	if band.Empty() || s <= 0 {
		return
	}
	// The source rect the band shows: the cover crop, centred.
	cw, ch := float64(band.Dx())/s, float64(band.Dy())/s
	sx := float64(src.Min.X) + (float64(src.Dx())-cw)/2
	sy := float64(src.Min.Y) + (float64(src.Dy())-ch)/2
	a := float32(headerAlpha(0, band.Dy(), fade)) / 255
	x0, x1 := float32(band.Min.X), float32(band.Max.X)
	y0, y1 := float32(band.Min.Y), float32(band.Max.Y)
	vs := []ebiten.Vertex{
		{DstX: x0, DstY: y0, SrcX: float32(sx), SrcY: float32(sy), ColorR: 1, ColorG: 1, ColorB: 1, ColorA: a},
		{DstX: x1, DstY: y0, SrcX: float32(sx + cw), SrcY: float32(sy), ColorR: 1, ColorG: 1, ColorB: 1, ColorA: a},
		{DstX: x1, DstY: y1, SrcX: float32(sx + cw), SrcY: float32(sy + ch), ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 0},
		{DstX: x0, DstY: y1, SrcX: float32(sx), SrcY: float32(sy + ch), ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 0},
	}
	dst.DrawTriangles(vs, []uint16{0, 1, 2, 0, 2, 3}, plate, nil)
}

// headerBandOf finds a window's artwork header, if its body carries one (a
// scroll-less body is wrapped in winClip, so look one level down).
func headerBandOf(w *widget.Window) *headerBand {
	if hb, ok := w.Contents.(*headerBand); ok {
		return hb
	}
	if w.Contents == nil {
		return nil
	}
	for _, ch := range w.Contents.Children() {
		if hb, ok := ch.(*headerBand); ok {
			return hb
		}
	}
	return nil
}
