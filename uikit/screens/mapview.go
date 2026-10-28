package screens

import (
	"image"
	"image/color"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
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

	mapHitR  = 10  // node click radius (design px)
	mapCardW = 330 // selected-system card width (design px)

	mapGridStep = 28 // the kit's lattice pitch (design px)
)

// mapNode is one system's starmap plane position (light-years) and its
// display identity.
type mapNode struct {
	id       content.SystemID
	name     string // display name (Struvok, Kvonice, …), never the id
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
// image is redrawn only when what it shows changes (current system, hover,
// selection, destination).
//
// Interaction is view state — session-only, never persisted and never a sim
// command: the pointer is read through the host's frame (the SplitPane
// idiom) once per Sync; a press on a node selects
// it, a press on the already-selected node makes it the destination (or
// clears it), and the route from the current system is BFS over the gates.
type MapView struct {
	th      theme.Theme
	cv      widgets.ContentView
	ptr     *widgets.Pointer
	win     *widget.Window
	img     *ebiten.Image
	gfx     *widget.Graphic
	drawn   bool
	dirty   bool              // selection/hover changed since the last paint
	nodes   []mapNode         // the laid-out graph (hit-test geometry)
	pts     []mapPoint        // projected node centres (image px)
	labels  []image.Rectangle // label bands, parallel to pts (image px)
	redraws int               // graph repaint count (TestMapViewRedraws watches it)
	system  content.SystemID
	hover   content.SystemID // pointer's node ("" = none)
	sel     content.SystemID // inspected node ("" = none)
	dest    content.SystemID // planned destination ("" = none)
}

// NewMapView builds the starmap window (draggable, fixed size) over the
// given environment. Call Sync after construction to paint the graph.
func NewMapView(env interface {
	theme.Theme
	widgets.ContentView
	Pointer() *widgets.Pointer
}) *MapView {
	p := env.Pal()
	m := &MapView{th: env, cv: env, ptr: env.Pointer()}
	m.img = ebiten.NewImage(theme.Px(mapImgW), theme.Px(mapImgH))
	m.gfx = widget.NewGraphic(widget.GraphicOpts.Image(m.img))

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.WindowBodyBox(p, theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200))),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	body.AddChild(m.gfx)

	title := "Starmap"
	if u := env.Defs().Universe; u != nil {
		title = starmapTitle(u.Region, u.Constellation)
	}
	titleBar, _ := theme.NewTitleBar(env, title, "svc_map")

	m.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(titleBar, theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(400), theme.Px(300)),
		// The landing rect the game's Starmap Screen row declares
		// (MapViewRect); the integrator re-centers it on real layout.
		widget.WindowOpts.Location(pxRect(MapViewRect())),
	)
	return m
}

// MapViewRect is the starmap window's landing rect in design units: the
// one number its Screen row's DefaultRect and its constructor share.
func MapViewRect() image.Rectangle {
	return image.Rect(160, 80, 160+mapImgW+2*theme.ContentsPad, 80+theme.TitleBarH+mapImgH+2*theme.ContentsPad)
}

// Win returns the starmap window for the interface to add.
func (m *MapView) Win() *widget.Window { return m.win }

// Sync repaints the graph when what it shows changes (first sync always
// paints). Pointer input is consumed here, before the host's Kit.Update —
// the same frame order the split panes use.
func (m *MapView) Sync() {
	if m.cv == nil || m.cv.Defs() == nil {
		return
	}
	cur := content.SystemID("")
	if sys := m.cv.System(); sys != nil {
		cur = sys.ID
	}
	m.poll(cur)
	if m.drawn && cur == m.system && !m.dirty {
		return
	}
	m.dirty = false
	m.drawn = true
	m.system = cur
	m.redraws++
	m.drawMap(cur)
}

// poll reads one frame of pointer input: the cursor drives hover, and a
// left press selects the node under it — a press on the already-selected
// node sets it as the destination, or clears it when it already is the
// destination or is the current system (a route to where you are is none).
func (m *MapView) poll(cur content.SystemID) {
	if len(m.pts) == 0 {
		return
	}
	id, on := m.nodeAt(m.ptr.At())
	if id != m.hover {
		m.hover = id
		m.dirty = true
	}
	if !on || !m.ptr.Press(widgets.Left) {
		return
	}
	switch {
	case m.sel != id:
		m.sel = id
	case m.dest == id || id == cur:
		m.dest = ""
	default:
		m.dest = id
	}
	m.dirty = true
}

// nodeAt resolves a screen point to the system under it: a node's hit disc
// or its label band (a click on the name resolves the same way). The
// closest candidate wins; a point outside the graph image is no node.
func (m *MapView) nodeAt(pt image.Point) (content.SystemID, bool) {
	r := m.gfx.GetWidget().Rect
	if len(m.pts) == 0 || !pt.In(r) {
		return "", false
	}
	lx, ly := float64(pt.X-r.Min.X), float64(pt.Y-r.Min.Y)
	local := pt.Sub(r.Min)
	hit, best := -1, math.Inf(1)
	for i, p := range m.pts {
		d := math.Hypot(lx-p.x, ly-p.y)
		if d <= float64(theme.Px(mapHitR)) || (i < len(m.labels) && local.In(m.labels[i])) {
			if d < best {
				hit, best = i, d
			}
		}
	}
	if hit < 0 {
		return "", false
	}
	return m.nodes[hit].id, true
}

// route is the shortest gate path from cur to dest, both ends included
// (nil when either is unknown, they are equal, or no gate chain connects
// them). Gates are authored both ways; the reverse edge is added so a
// one-way slip can never hide a route the player can actually fly.
func (m *MapView) route(cur, dest content.SystemID) []content.SystemID {
	if cur == "" || dest == "" || cur == dest {
		return nil
	}
	adj := make(map[content.SystemID][]content.SystemID, len(m.nodes))
	for _, n := range m.nodes {
		def, ok := m.cv.Defs().System(n.id)
		if !ok {
			continue
		}
		for i := range def.Celestials {
			if g := def.Celestials[i].Gate; g != nil {
				adj[n.id] = append(adj[n.id], g.ToSystem)
				adj[g.ToSystem] = append(adj[g.ToSystem], n.id)
			}
		}
	}
	prev := make(map[content.SystemID]content.SystemID, len(m.nodes))
	seen := map[content.SystemID]bool{cur: true}
	queue := []content.SystemID{cur}
	for len(queue) > 0 {
		a := queue[0]
		queue = queue[1:]
		if a == dest {
			path := []content.SystemID{dest}
			for path[0] != cur {
				path = append([]content.SystemID{prev[path[0]]}, path...)
			}
			return path
		}
		for _, b := range adj[a] {
			if !seen[b] {
				seen[b] = true
				prev[b] = a
				queue = append(queue, b)
			}
		}
	}
	return nil
}

// drawMap repaints the whole graph: gate edges (the planned route in
// accent), system nodes, labels and the selected system's card.
// Every coordinate below lands in the scaled image, so lengths multiply
// by s (the same scale the image dims went through Px with).
func (m *MapView) drawMap(cur content.SystemID) {
	m.img.Clear()
	s := theme.Scale()
	nodes := m.mapNodes()
	pts := m.project(nodes)
	m.nodes, m.pts = nodes, pts
	m.labels = make([]image.Rectangle, len(nodes))
	byID := make(map[content.SystemID]mapPoint, len(nodes))
	for i, n := range nodes {
		byID[n.id] = pts[i]
	}
	p := m.th.Pal()

	// The kit's 28 px lattice under the graph: the starmap is the one
	// technical surface that carries it (windows keep clean hairlines).
	// Accent at 3.5 % — structure, not colour.
	grid := theme.NRGBA(p.Accent, 9)
	step := float64(theme.Px(mapGridStep))
	b := m.img.Bounds()
	for x := 0.0; x < float64(b.Dx()); x += step {
		vector.StrokeLine(m.img, float32(x), 0, float32(x), float32(b.Dy()), float32(s), grid, false)
	}
	for y := 0.0; y < float64(b.Dy()); y += step {
		vector.StrokeLine(m.img, 0, float32(y), float32(b.Dx()), float32(y), float32(s), grid, false)
	}

	// The planned route, edge by edge: nil while no destination is set or
	// the gates do not connect the two.
	route := m.route(cur, m.dest)
	onRoute := make(map[[2]content.SystemID]bool, len(route))
	for i := 0; i+1 < len(route); i++ {
		a, b := route[i], route[i+1]
		onRoute[[2]content.SystemID{a, b}] = true
		onRoute[[2]content.SystemID{b, a}] = true
	}

	// Gate edges from every system's gate celestials. The planned route
	// wins over the current-system adjacency accent.
	base := color.RGBA{90, 100, 118, 160}
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
			var col color.Color = base
			w := float32(s)
			switch {
			case onRoute[[2]content.SystemID{n.id, g.ToSystem}]:
				col, w = theme.NRGBA(p.Accent, 230), float32(2.5*s)
			case n.id == cur || g.ToSystem == cur:
				col = theme.NRGBA(p.Accent, 170)
			}
			vector.StrokeLine(m.img, float32(a.x), float32(a.y),
				float32(b.x), float32(b.y), w, col, true)
		}
	}

	// System nodes: hover ring, selection ring, destination ring, current
	// ring, then the zone-colored disc and the label band.
	face := m.th.Face(theme.StepSmall)
	for i, n := range nodes {
		pt := pts[i]
		rad := float32(5 * s)
		switch {
		case n.id == cur:
			rad = float32(7 * s)
			vector.StrokeCircle(m.img, float32(pt.x), float32(pt.y), float32(12.5*s), float32(1.5*s), p.Accent, true)
		case n.id == m.dest:
			rad = float32(6.5 * s)
		case n.id == m.hover:
			rad = float32(6.5 * s)
		}
		if n.id == m.hover && n.id != m.dest {
			vector.StrokeCircle(m.img, float32(pt.x), float32(pt.y), float32(9*s), float32(s), theme.NRGBA(p.Text, 160), true)
		}
		if n.id == m.sel {
			vector.StrokeCircle(m.img, float32(pt.x), float32(pt.y), float32(10*s), float32(1.5*s), p.Accent, true)
		}
		if n.id == m.dest {
			// The destination wears the widest ring: the route ends here.
			vector.StrokeCircle(m.img, float32(pt.x), float32(pt.y), float32(15*s), float32(2*s), p.Accent, true)
		}
		vector.DrawFilledCircle(m.img, float32(pt.x), float32(pt.y), rad, m.mapSecurity(n.security), true)
		col := p.Dim
		switch {
		case n.id == cur || n.id == m.dest:
			col = p.Accent
		case n.id == m.hover || n.id == m.sel:
			col = p.Text
		}
		w, _ := text.Measure(n.name, *face, 0)
		render.Text(m.img, n.name, pt.x-w/2, pt.y+14*s, m.th.TS().Small, col)
		m.labels[i] = image.Rect(int(pt.x-w/2)-theme.Px(2), int(pt.y+11*s),
			int(pt.x+w/2)+theme.Px(2), int(pt.y+34*s))
	}

	// Zone legend: the node colors decoded in the canon's zone words with
	// their complementary status labels (docs/lore/UNIVERSE.md — Zones).
	for i, sec := range zoneSecs {
		y := float64(theme.Px(mapImgH-mapPad)) + float64(i)*16*s
		vector.DrawFilledCircle(m.img, float32(12*s), float32(y)+float32(6*s), float32(4*s), m.mapSecurity(sec), true)
		render.Text(m.img, zoneLabel(sec), 24*s, y, m.th.TS().Tiny, p.Dim)
	}
	// How the pointer works, tucked into the empty bottom-right corner.
	hint := "Click a system · click again to set destination"
	hw := render.Measure(hint, m.th.TS().Tiny)
	render.Text(m.img, hint, float64(theme.Px(mapImgW))-hw-float64(theme.Px(12)),
		float64(theme.Px(mapImgH-28)), m.th.TS().Tiny, p.Dim)

	if m.sel != "" {
		m.drawCard(cur)
	}
}

// mapCardLine is one row of the selected-system card.
type mapCardLine struct {
	text string
	bold bool
	tiny bool
	col  color.RGBA
}

// drawCard paints the selected system's card (the map image's empty
// top-left corner): display name, zone, its stations and gate destinations,
// and the route line — how many gate jumps to the destination, or the offer
// to set one. Rows are one line each; the card grows with the content.
func (m *MapView) drawCard(cur content.SystemID) {
	def, ok := m.cv.Defs().System(m.sel)
	if !ok {
		return
	}
	p := m.th.Pal()
	name := def.Name
	if name == "" {
		name = string(def.ID)
	}
	lines := []mapCardLine{{text: name, bold: true, col: p.Text}}
	if z := zoneLabel(def.Security); z != "" {
		lines = append(lines, mapCardLine{text: z, tiny: true, col: p.Dim})
	}
	var stations []string
	for i := range def.Celestials {
		if def.Celestials[i].Kind == content.CelStation {
			stations = append(stations, def.Celestials[i].Name)
		}
	}
	if len(stations) == 0 {
		stations = []string{"none"}
	}
	lines = append(lines, mapCardLine{text: "Stations — " + strings.Join(stations, " · "), col: p.Text})
	for i := range def.Celestials {
		g := def.Celestials[i].Gate
		if g == nil {
			continue
		}
		lines = append(lines, mapCardLine{text: "Gate — " + m.systemName(g.ToSystem), col: p.Dim})
	}
	switch jumps := len(m.route(cur, m.sel)) - 1; {
	case m.dest != def.ID:
		lines = append(lines, mapCardLine{text: "Click again to set destination", tiny: true, col: p.Dim})
	case jumps > 0:
		lines = append(lines, mapCardLine{text: "Route — " + strconv.Itoa(jumps) + " jumps", tiny: true, col: p.Accent})
	default:
		lines = append(lines, mapCardLine{text: "Route — no gate path", tiny: true, col: p.Danger})
	}

	pad, lineH := theme.Px(10), theme.Px(21)
	height := 2 * pad
	for _, l := range lines {
		h := lineH
		if l.tiny {
			h = theme.Px(17)
		}
		height += h
	}
	rect := image.Rect(theme.Px(12), theme.Px(10), theme.Px(12)+theme.Px(mapCardW), theme.Px(10)+height)
	theme.DrawRoundBox(m.img, rect, theme.NRGBA(p.BGHard, 225), theme.NRGBA(p.Border, 210), 1, theme.AllRadii(theme.RadiusPanel))
	y := rect.Min.Y + pad
	for _, l := range lines {
		face := m.th.Face(theme.StepSmall)
		switch {
		case l.tiny:
			face = m.th.Face(theme.StepTiny)
		case l.bold:
			face = m.th.FaceBold(theme.StepSmall)
		}
		theme.DrawCardText(m.img, l.text, face, rect.Min.X+pad, y, l.col)
		if l.tiny {
			y += theme.Px(17)
		} else {
			y += lineH
		}
	}
}

// systemName is a system's display name, its id only when content has none.
func (m *MapView) systemName(id content.SystemID) string {
	if def, ok := m.cv.Defs().System(id); ok && def.Name != "" {
		return def.Name
	}
	return string(id)
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
// the inline universe systems with the file-based ones. Nodes carry the
// display name (the ids stay internal: they never reach the map's text).
func (m *MapView) mapNodes() []mapNode {
	c := m.cv.Defs()
	var out []mapNode
	seen := map[content.SystemID]bool{}
	if c.Universe != nil {
		for _, s := range c.Universe.Systems {
			out = append(out, mapNode{id: s.ID, name: s.Name, x: s.Position.X, y: s.Position.Y, security: s.Security})
			seen[s.ID] = true
		}
	}
	for id, s := range c.Systems {
		if !seen[id] {
			out = append(out, mapNode{id: id, name: s.Name, x: s.Position.X, y: s.Position.Y, security: s.Security})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	for i := range out {
		if out[i].name == "" {
			out[i].name = string(out[i].id)
		}
	}
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
	pad := float64(theme.Px(mapPad))
	availW, availH := float64(theme.Px(mapImgW))-2*pad, float64(theme.Px(mapImgH))-2*pad
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

// starmapTitle composes the window title from the universe's region and
// constellation. They usually carry one name (the Wild Fields serves both
// levels), so the pair never doubles; an empty half renders as its lone
// name rather than a dangling separator.
func starmapTitle(region, constellation string) string {
	switch {
	case region == "" && constellation == "":
		return "Starmap"
	case region == "":
		return "Starmap — " + constellation
	case constellation == "" || region == constellation:
		return "Starmap — " + region
	default:
		return "Starmap — " + region + " / " + constellation
	}
}
