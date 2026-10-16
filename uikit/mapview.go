package uikit

import (
	"image"
	"image/color"
	"sort"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/render"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Starmap geometry: the graph image inside the window, and the padding that
// keeps nodes and labels off the edges (mirrors ui's StarmapNodes pad).
const (
	mapImgW = 700
	mapImgH = 500
	mapPad  = 48
)

// mapNode is one system's starmap plane position (light-years).
type mapNode struct {
	id       content.SystemID
	x, y     float64
	security string
}

// mapPoint is a system's projected position inside the map image (px).
type mapPoint struct {
	x, y float64
}

// MapView is the starmap window: the universe graph (systems from content,
// gate edges from their gate celestials) drawn into an image shown through
// a widget.Graphic, with the current system highlighted in accent. The
// image is redrawn only when the current system changes.
type MapView struct {
	th      Theme
	cv      ContentView
	win     *widget.Window
	img     *ebiten.Image
	gfx     *widget.Graphic
	drawn   bool
	redraws int // graph repaint count (TestMapViewRedraws watches it)
	system  content.SystemID
}

// NewMapView builds the starmap window (draggable, fixed size) over the
// given environment. Call Sync after construction to paint the graph.
func NewMapView(env interface {
	Theme
	ContentView
}) *MapView {
	p := env.Pal()
	m := &MapView{th: env, cv: env}
	m.img = ebiten.NewImage(Px(mapImgW), Px(mapImgH))
	m.gfx = widget.NewGraphic(widget.GraphicOpts.Image(m.img))

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BG), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(Px(contentsPad))),
		)),
	)
	body.AddChild(m.gfx)

	title := "Starmap"
	if u := env.Defs().Universe; u != nil && (u.Region != "" || u.Constellation != "") {
		title = "Starmap — " + u.Region + " / " + u.Constellation
	}
	titleBar, _ := NewTitleBar(env, title, "svc_map")

	m.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(titleBar, Px(titleBarH)),
		widget.WindowOpts.Draggable(),
		widget.WindowOpts.MinSize(Px(400), Px(300)),
		// Default: roughly centered at both design sizes (1024×700 and
		// 1280×720); the integrator re-centers it on real layout.
		widget.WindowOpts.Location(image.Rect(Px(160), Px(80), Px(160+mapImgW+2*contentsPad), Px(80+titleBarH+mapImgH+2*contentsPad))),
	)
	return m
}

// Win returns the starmap window for the interface to add.
func (m *MapView) Win() *widget.Window { return m.win }

// Sync repaints the graph when the current system changes (first sync
// always paints).
func (m *MapView) Sync() {
	if m.cv == nil || m.cv.Defs() == nil {
		return
	}
	cur := content.SystemID("")
	if sys := m.cv.System(); sys != nil {
		cur = sys.ID
	}
	if m.drawn && cur == m.system {
		return
	}
	m.drawn = true
	m.system = cur
	m.redraws++
	m.drawMap(cur)
}

// drawMap repaints the whole graph: gate edges, system nodes and labels.
// Every coordinate below lands in the scaled image, so lengths multiply
// by s (the same scale the image dims went through Px with).
func (m *MapView) drawMap(cur content.SystemID) {
	m.img.Clear()
	s := Scale()
	nodes := m.mapNodes()
	pts := m.project(nodes)
	byID := make(map[content.SystemID]mapPoint, len(nodes))
	for i, n := range nodes {
		byID[n.id] = pts[i]
	}

	// Gate edges from every system's gate celestials.
	for _, n := range nodes {
		def, ok := m.cv.Defs().System(n.id)
		if !ok {
			continue
		}
		a := byID[n.id]
		for i := range def.Celestials {
			g := def.Celestials[i].Gate
			if g == nil {
				continue
			}
			b, ok := byID[g.ToSystem]
			if !ok {
				continue
			}
			var col color.Color = color.RGBA{90, 100, 118, 160}
			if n.id == cur || g.ToSystem == cur {
				col = nrgba(m.th.Pal().Accent, 170)
			}
			vector.StrokeLine(m.img, float32(a.x), float32(a.y),
				float32(b.x), float32(b.y), float32(s), col, true)
		}
	}

	// System nodes, highlighted current system, then labels.
	face := m.th.Face(StepSmall)
	for i, n := range nodes {
		pt := pts[i]
		rad := float32(5 * s)
		if n.id == cur {
			rad = float32(7 * s)
			vector.StrokeCircle(m.img, float32(pt.x), float32(pt.y), float32(12*s), float32(1.5*s), m.th.Pal().Accent, true)
		}
		vector.DrawFilledCircle(m.img, float32(pt.x), float32(pt.y), rad, m.mapSecurity(n.security), true)
		label := string(n.id)
		col := m.th.Pal().Dim
		if n.id == cur {
			col = m.th.Pal().Accent
		}
		w, _ := text.Measure(label, *face, 0)
		render.Text(m.img, label, pt.x-w/2, pt.y+14*s, m.th.TS().Small, col)
	}

	// Zone legend: the node colors decoded in the canon's zone words with
	// their complementary status labels (docs/lore/UNIVERSE.md — Zones).
	for i, sec := range zoneSecs {
		y := float64(Px(mapImgH-mapPad)) + float64(i)*16*s
		vector.DrawFilledCircle(m.img, float32(12*s), float32(y)+float32(6*s), float32(4*s), m.mapSecurity(sec), true)
		render.Text(m.img, zoneLabel(sec), 24*s, y, m.th.TS().Tiny, m.th.Pal().Dim)
	}
}

// zoneSecs are the security codes in canonical display order.
var zoneSecs = []string{"high", "low", "null"}

// zoneLabel renders a system's security code in the in-universe zone words
// with the complementary status label the lore canonizes (UNIVERSE.md —
// "Charter · secure", "Fringe · contested", "Unincorporated · no charter").
// The code values themselves never reach display text.
func zoneLabel(sec string) string {
	switch sec {
	case "high":
		return "Charter · secure"
	case "low":
		return "Fringe · contested"
	case "null":
		return "Unincorporated · no charter"
	}
	return ""
}

// mapNodes lists every system in deterministic (id-sorted) order, merging
// the inline universe systems with the file-based ones.
func (m *MapView) mapNodes() []mapNode {
	c := m.cv.Defs()
	var out []mapNode
	seen := map[content.SystemID]bool{}
	if c.Universe != nil {
		for _, s := range c.Universe.Systems {
			out = append(out, mapNode{id: s.ID, x: s.Position.X, y: s.Position.Y, security: s.Security})
			seen[s.ID] = true
		}
	}
	for id, s := range c.Systems {
		if !seen[id] {
			out = append(out, mapNode{id: id, x: s.Position.X, y: s.Position.Y, security: s.Security})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out
}

// project maps graph positions (ly) into image pixels, preserving aspect
// ratio and centering the bounds (port of ui's StarmapNodes).
func (m *MapView) project(nodes []mapNode) []mapPoint {
	pts := make([]mapPoint, len(nodes))
	if len(nodes) == 0 {
		return pts
	}
	minX, maxX := nodes[0].x, nodes[0].x
	minY, maxY := nodes[0].y, nodes[0].y
	for _, n := range nodes {
		if n.x < minX {
			minX = n.x
		}
		if n.x > maxX {
			maxX = n.x
		}
		if n.y < minY {
			minY = n.y
		}
		if n.y > maxY {
			maxY = n.y
		}
	}
	spanX, spanY := maxX-minX, maxY-minY
	// The image is sized Px(mapImgW/mapImgH); the pad goes through Px in
	// the same decomposition here (Px(img) − 2·Px(pad)) so nodes land
	// exactly Px(mapPad) inside the scaled image edges.
	pad := float64(Px(mapPad))
	availW, availH := float64(Px(mapImgW))-2*pad, float64(Px(mapImgH))-2*pad
	scale := 1.0
	switch {
	case spanX > 0 && spanY > 0:
		scale = min(availW/spanX, availH/spanY)
	case spanX > 0:
		scale = availW / spanX
	case spanY > 0:
		scale = availH / spanY
	}
	offX := pad + (availW-spanX*scale)/2
	offY := pad + (availH-spanY*scale)/2
	for i, n := range nodes {
		pts[i] = mapPoint{
			x: offX + (n.x-minX)*scale,
			y: offY + (n.y-minY)*scale,
		}
	}
	return pts
}

// mapSecurity colors a system node by its zone.
func (m *MapView) mapSecurity(sec string) color.RGBA {
	switch sec {
	case "high":
		return m.th.Pal().Good
	case "low":
		return m.th.Pal().Warn
	case "null":
		return m.th.Pal().Danger
	default:
		return m.th.Pal().Dim
	}
}
