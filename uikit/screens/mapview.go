package screens

import (
	"image"
	"image/color"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/uikit/icons"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Starmap geometry, in design units through the Px funnel: the layer's
// three columns, the graph's own inset, and the grid pitch.
const (
	mapPadOuter = 14 // the layer's outer margin
	mapHeadH    = 46 // the canvas header band
	mapInset    = 8  // inside each column
	mapLeftW    = 268
	mapRightW   = 312
	mapSearchH  = 26
	mapRowH     = 26
	mapBodyH    = 22

	mapPad  = 48 // the graph keeps nodes and labels off its edges
	mapHitR = 10

	mapGridStep = 28 // the kit's lattice pitch
)

// mapNode is one system's starmap plane position (light-years) and its
// display identity.
type mapNode struct {
	id       content.SystemID
	name     string // display name (Struvok, Kvonice, …), never the id
	x, y     float64
	security string
}

// mapPoint is a system's projected position inside the graph column (px).
type mapPoint struct {
	x, y float64
}

// mapHit is one clickable canvas region: a browser row, a gate chip or a
// facility glyph (the last one only for its hover label).
type mapHit struct {
	rect image.Rectangle
	id   content.SystemID
	text string
}

// overviewBody is one celestial as the overview panel reads it: its kind
// and icon, a station's services, a belt's ore, a gate's destination.
// Built from content only (overviewBodies), never authored in the view.
type overviewBody struct {
	Kind     string
	Name     string
	Icon     string
	Services []string
	Ores     []overviewOre
	Gate     content.SystemID
}

// overviewOre is one asteroid type in a belt: the ore item and the belt's
// available count and per-asteroid volume.
type overviewOre struct {
	Ore    content.ItemID
	Count  int
	Volume float64
}

// MapView is the starmap: the house's second full-viewport layer. It is
// three zones over one canvas: the system browser (search and list) on the
// left, the gate graph in the middle and the selected system's overview on
// the right. The graph keeps its node security colours, the planned route
// in accent, control shapes and contested front rings; the overview names
// every celestial, a station's facilities, a belt's available ore and a
// gate's destination, each as an icon row rather than a sentence.
//
// Like the proficiency tree it is mounted through Kit.AddBackdropWindow:
// it paints over the world and below every window and all chrome, takes no
// title bar, blocks the world through the hit test (Own) and re-fits itself
// to the live viewport every Sync. N, the rail entry and Esc close it.
//
// Interaction is view state only, never persisted and never a sim command:
// a press on a node or a browser row selects a system, a second press on
// the same one sets it as the destination (or clears it), and a press on a
// gate chip selects the neighbour. The search field is the layer's one
// typing surface: while focused, arrows walk the list and Enter sets the
// destination (game.handleKeys yields the frame to SearchKey).
type MapView struct {
	th       theme.Theme
	cv       widgets.ContentView
	ptr      *widgets.Pointer
	viewport func() image.Rectangle

	win *widget.Window
	gfx *widget.Graphic
	img *ebiten.Image
	// search is the browser's live filter: the house text field (the same
	// one the market's panes use), drawn over the canvas.
	search *widget.TextInput

	rect     image.Rectangle // the canvas (and window) the layer covers
	graph    image.Rectangle // the graph column inside rect
	layout   mapZones
	drawn    bool
	dirty    bool // selection/hover/search changed since the last paint
	nodes    []mapNode
	pts      []mapPoint
	labels   []image.Rectangle // label bands, parallel to pts
	redraws  int               // graph repaint count (TestMapViewRedraws watches it)
	system   content.SystemID
	hover    content.SystemID // pointer's node or row ("" = none)
	sel      content.SystemID // inspected node ("" = none)
	dest     content.SystemID // planned destination ("" = none)
	war      widgets.WarView
	warShown widgets.WarView

	shownQuery string   // the search text the last paint read
	rows       []mapHit // browser rows, for the poll
	gates      []mapHit // gate chips, for the poll
	facs       []mapHit // facility glyphs, for their hover label
	tip        string   // the facility label under the pointer ("" = none)
}

// mapZones is the layer's three columns.
type mapZones struct {
	left, graph, right image.Rectangle
}

// NewMapView builds the starmap layer. viewport is the live screen rect in
// device pixels (nil keeps the design rect, which the chrome harness and
// the unit tests use). Call Sync after construction to paint the graph.
func NewMapView(env widgets.Seams, viewport func() image.Rectangle) *MapView {
	m := &MapView{th: env, cv: env, ptr: env.Pointer(), viewport: viewport}
	m.rect = pxRect(MapViewRect())
	m.img = ebiten.NewImage(m.rect.Dx(), m.rect.Dy())
	m.gfx = widget.NewGraphic(widget.GraphicOpts.Image(m.img),
		widget.GraphicOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			StretchHorizontal: true,
			StretchVertical:   true,
		})))

	m.search = widget.NewTextInput(
		widget.TextInputOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(theme.Px(mapLeftW-2*mapInset), theme.Px(mapSearchH)),
		),
		widget.TextInputOpts.Placeholder("Search systems"),
		widget.TextInputOpts.Face(env.Face(theme.StepSmall)),
		widget.TextInputOpts.Padding(&widget.Insets{
			Left: theme.Px(4), Right: theme.Px(4), Top: theme.Px(2), Bottom: theme.Px(2),
		}),
	)
	m.search.GetWidget().LayoutData = widget.AnchorLayoutData{
		Padding: &widget.Insets{
			Left: theme.Px(mapPadOuter + mapInset),
			Top:  theme.Px(mapHeadH + mapInset),
		},
	}

	body := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout()))
	body.AddChild(m.gfx)
	body.AddChild(m.search)

	m.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.Location(m.rect),
		// The layer blocks the world through the hit test (Own), never
		// through ebitenui's input layers: windows above it keep their
		// presses.
		widget.WindowOpts.BlockLower(false),
	)
	return m
}

// MapViewRect is the layer's landing rect in design units: the design
// viewport, the one number its Screen row's DefaultRect and its
// constructor share.
func MapViewRect() image.Rectangle { return ProficiencyRect() }

// Win returns the layer for Kit.AddBackdropWindow.
func (m *MapView) Win() *widget.Window { return m.win }

// Backdrop marks the panel as the kit's under-window layer (the Screen
// host mounts it through AddBackdropWindow).
func (m *MapView) Backdrop() bool { return true }

// Unmounted clears the search and its focus: the field must not keep the
// keyboard while the layer is gone, and a reopened map starts unfiltered.
// Selection and destination stay: they are the player's place in the map.
func (m *MapView) Unmounted() {
	if m.search != nil {
		m.search.Focus(false)
		m.search.SetText("")
	}
	m.hover, m.tip = "", ""
	m.dirty = true
}

// Sync fits the layer to the viewport, polls the frame's pointer and
// repaints when the system, the war, the search or a view state moved.
func (m *MapView) Sync() {
	m.fit()
	if m.cv == nil || m.cv.Defs() == nil {
		return
	}
	cur := content.SystemID("")
	if sys := m.cv.System(); sys != nil {
		cur = sys.ID
	}
	m.war = m.cv.War()
	m.poll(cur)
	q := m.searchText()
	if m.drawn && cur == m.system && q == m.shownQuery && !m.dirty && reflect.DeepEqual(m.war, m.warShown) {
		return
	}
	m.dirty = false
	m.drawn = true
	m.system = cur
	m.shownQuery = q
	m.redraws++
	m.drawMap(cur)
	m.warShown = m.war
}

// fit resizes the window and the canvas to the live viewport (nil keeps
// the design rect). It is the layer's only placement: the layer is the
// viewport, so a sidecar rect or a viewport change is re-fitted here.
func (m *MapView) fit() {
	r := pxRect(MapViewRect())
	if m.viewport != nil {
		if vp := m.viewport(); !vp.Empty() {
			r = image.Rect(0, 0, vp.Dx(), vp.Dy())
		}
	}
	if r == m.rect {
		return
	}
	m.rect = r
	m.img = ebiten.NewImage(max(r.Dx(), 1), max(r.Dy(), 1))
	m.gfx.Image = m.img
	m.win.SetLocation(r)
	m.rows, m.gates, m.facs = nil, nil, nil
	m.hover, m.tip = "", ""
	m.drawn = false
}

// SearchFocused reports whether the browser's field owns the keyboard.
func (m *MapView) SearchFocused() bool { return m.search != nil && m.search.IsFocused() }

// SearchKey handles the keys the field's own editing does not: arrows walk
// the visible list, Enter toggles the destination and Esc clears the query
// first, then blurs so the next Esc reaches the host's ladder.
func (m *MapView) SearchKey(k ebiten.Key) bool {
	if !m.SearchFocused() {
		return false
	}
	switch k {
	case ebiten.KeyArrowDown:
		m.moveSel(1)
		return true
	case ebiten.KeyArrowUp:
		m.moveSel(-1)
		return true
	case ebiten.KeyEnter:
		m.pickSystem(m.sel, m.system)
		return true
	case ebiten.KeyEscape:
		if m.search.GetText() != "" {
			m.search.SetText("")
			m.dirty = true
			return true
		}
		m.search.Focus(false)
		return false
	}
	return false
}

// moveSel walks the visible browser list and highlights the system it
// lands on (the graph and the overview follow the selection).
func (m *MapView) moveSel(d int) {
	nodes := m.listedNodes()
	if len(nodes) == 0 {
		return
	}
	i := -1
	for j := range nodes {
		if nodes[j].id == m.sel {
			i = j
			break
		}
	}
	switch {
	case i < 0 && d > 0:
		i = 0
	case i < 0:
		i = len(nodes) - 1
	default:
		i = min(max(i+d, 0), len(nodes)-1)
	}
	m.sel, m.hover = nodes[i].id, nodes[i].id
	m.dirty = true
}

// poll reads one frame of pointer input: the cursor drives hover over a
// node, a browser row or a gate chip, and a press selects. A press on the
// already-selected system sets the destination, or clears it when it
// already is the destination or is the current system.
func (m *MapView) poll(cur content.SystemID) {
	if m.ptr == nil || m.gfx == nil || len(m.pts) == 0 {
		return
	}
	pt := m.ptr.At()
	r := m.gfx.GetWidget().Rect
	over := widgets.OwnsAt(m.gfx.GetWidget(), pt, input.LayerEventTypeMouseButton)
	local := pt.Sub(r.Min)

	hover, tip := content.SystemID(""), ""
	if over {
		if to, ok := m.gateAt(local); ok {
			hover = to
		}
		if id, ok := m.hitAt(m.rows, local); ok {
			hover = id
		}
		if label, ok := m.tipAt(local); ok {
			tip = label
		}
		if id, ok := m.nodeAt(pt); ok {
			hover = id
		}
		if hover != "" {
			m.ptr.ClaimShape(ebiten.CursorShapePointer)
		}
	}
	if hover != m.hover || tip != m.tip {
		m.hover, m.tip = hover, tip
		m.dirty = true
	}
	if !over || !m.ptr.Press(widgets.Left) {
		return
	}
	// A press on the canvas takes the keyboard back from the search
	// field, so N and Esc are the layer's keys again after a click.
	if m.SearchFocused() {
		m.search.Focus(false)
	}
	if id, ok := m.hitAt(m.rows, local); ok {
		m.pickSystem(id, cur)
		return
	}
	if to, ok := m.gateAt(local); ok {
		m.sel = to
		m.dirty = true
		return
	}
	if id, ok := m.nodeAt(pt); ok {
		m.pickSystem(id, cur)
	}
}

// pickSystem is the one select/destination rule the browser and the graph
// share: a new system selects, the selected one becomes the destination,
// and a second press on the destination (or on the current system) clears.
func (m *MapView) pickSystem(id, cur content.SystemID) {
	if id == "" {
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

// hitAt resolves a canvas point against one of the drawn hit lists.
func (m *MapView) hitAt(hits []mapHit, pt image.Point) (content.SystemID, bool) {
	for _, h := range hits {
		if pt.In(h.rect) {
			return h.id, true
		}
	}
	return "", false
}

// gateAt resolves a gate chip (its id is the destination system).
func (m *MapView) gateAt(pt image.Point) (content.SystemID, bool) { return m.hitAt(m.gates, pt) }

// tipAt resolves a facility glyph's hover label.
func (m *MapView) tipAt(pt image.Point) (string, bool) {
	for _, h := range m.facs {
		if pt.In(h.rect) {
			return h.text, true
		}
	}
	return "", false
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

// ---------------------------------------------------------------------------
// Layout and drawing
// ---------------------------------------------------------------------------

// zones divides the canvas into the browser, the graph and the overview.
func (m *MapView) zones() mapZones {
	b := m.img.Bounds()
	pad := theme.Px(mapPadOuter)
	head := theme.Px(mapHeadH)
	top, bottom := head, b.Dy()-pad
	left := image.Rect(pad, top, pad+theme.Px(mapLeftW), bottom)
	right := image.Rect(b.Dx()-pad-theme.Px(mapRightW), top, b.Dx()-pad, bottom)
	graph := image.Rect(left.Max.X+pad, top, right.Min.X-pad, bottom)
	if graph.Dx() < 1 {
		graph = image.Rect(left.Max.X, top, right.Min.X, bottom)
	}
	return mapZones{left: left, graph: graph, right: right}
}

// drawMap repaints the whole layer: the opaque backdrop, the header, the
// browser, the gate graph and the selected system's overview.
func (m *MapView) drawMap(cur content.SystemID) {
	m.img.Clear()
	b := m.img.Bounds()
	p := m.th.Pal()
	m.layout = m.zones()
	m.graph = m.layout.graph
	m.rows, m.gates, m.facs = m.rows[:0], m.gates[:0], m.facs[:0]

	// The layer is opaque: it covers the world, not the windows above it.
	vector.DrawFilledRect(m.img, 0, 0, float32(b.Dx()), float32(b.Dy()), theme.Solid(p.BGHard), false)

	m.drawHeader()
	m.drawBrowser(cur)
	m.drawGraph(cur)
	m.drawOverview(cur)
	m.drawTip()
}

// drawHeader is the canvas's own title band: the region and constellation
// the dropped title bar used to carry, with the map's glyph.
func (m *MapView) drawHeader() {
	p := m.th.Pal()
	title := "Starmap"
	if u := m.cv.Defs().Universe; u != nil {
		title = starmapTitle(u.Region, u.Constellation)
	}
	y := theme.Px(mapHeadH) / 2
	drawIcon(m.img, "svc_map", theme.Px(mapPadOuter), y-theme.Px(icons.Title)/2, theme.Px(icons.Title), p.Accent)
	theme.DrawCardText(m.img, title, m.th.FaceBold(theme.StepSmall),
		theme.Px(mapPadOuter)+theme.Px(icons.Title)+theme.Px(8), y-theme.Px(9), p.Text)
	vector.StrokeLine(m.img, float32(theme.Px(mapPadOuter)), float32(theme.Px(mapHeadH)-1),
		float32(m.img.Bounds().Dx()-theme.Px(mapPadOuter)), float32(theme.Px(mapHeadH)-1),
		float32(theme.Px(1)), theme.NRGBA(p.Border, 160), false)
}

// drawBrowser paints the system list under the search field. A row is a
// security dot, the display name and the two state markers (a hull for the
// current system, a pennant for the selected/destination one).
func (m *MapView) drawBrowser(cur content.SystemID) {
	z := m.layout.left
	p := m.th.Pal()
	theme.DrawRoundBox(m.img, z, theme.NRGBA(p.Cell, 235), theme.NRGBA(p.Border, 190), theme.Px(1), theme.AllRadii(theme.RadiusPanel))

	x0 := z.Min.X + theme.Px(6)
	x1 := z.Max.X - theme.Px(6)
	top := z.Min.Y + theme.Px(2*mapInset+mapSearchH)
	rowH := theme.Px(mapRowH)
	for i, n := range m.listedNodes() {
		r := image.Rect(x0, top+i*rowH, x1, top+(i+1)*rowH)
		if r.Max.Y > z.Max.Y-theme.Px(mapInset) {
			break // shortcut: the list clips, not scrolls; upgrade to the house scroll view when it outgrows the column.
		}
		switch {
		case n.id == m.sel:
			theme.FillRect(m.img, r, theme.NRGBA(p.Hover, 30))
			theme.FillRect(m.img, image.Rect(r.Min.X, r.Min.Y, r.Min.X+theme.Px(2), r.Max.Y), p.Accent)
		case n.id == m.hover:
			theme.FillRect(m.img, r, theme.NRGBA(p.Text, 12))
		}
		dot := float32(r.Min.Y + r.Dy()/2)
		vector.DrawFilledCircle(m.img, float32(r.Min.X+theme.Px(8)), dot, float32(theme.Px(4)), m.mapSecurity(n.security), true)
		col := p.Dim
		face := m.th.Face(theme.StepSmall)
		switch {
		case n.id == cur || n.id == m.dest || n.id == m.sel:
			col = p.Accent
		case n.id == m.hover:
			col = p.Text
		}
		theme.DrawCardText(m.img, n.name, face, r.Min.X+theme.Px(18), r.Min.Y+theme.Px(4), col)
		if n.id == m.dest {
			m.drawFlag(float32(r.Max.X-theme.Px(14)), float32(r.Min.Y+theme.Px(5)), true, p.Accent)
		} else if n.id == m.sel {
			m.drawFlag(float32(r.Max.X-theme.Px(14)), float32(r.Min.Y+theme.Px(5)), false, p.Accent)
		}
		if n.id == cur {
			drawIcon(m.img, "kind_ship", r.Max.X-theme.Px(40), r.Min.Y+theme.Px(5), theme.Px(icons.Inline), p.Accent)
		}
		m.rows = append(m.rows, mapHit{rect: r, id: n.id, text: n.name})
	}
}

// drawGraph paints the gate graph: the lattice, the gate edges, the nodes
// with their state rings and flags, the war key, the zone legend and the
// route's jump badge.
func (m *MapView) drawGraph(cur content.SystemID) {
	z := m.graph
	s := theme.Scale()
	p := m.th.Pal()

	// The kit's 28 px lattice under the graph: the starmap is the one
	// technical surface that carries it. Accent at 3.5 % — structure, not
	// colour.
	grid := theme.NRGBA(p.Accent, 9)
	step := float64(theme.Px(mapGridStep))
	for x := float64(z.Min.X); x < float64(z.Max.X); x += step {
		vector.StrokeLine(m.img, float32(x), float32(z.Min.Y), float32(x), float32(z.Max.Y), float32(s), grid, false)
	}
	for y := float64(z.Min.Y); y < float64(z.Max.Y); y += step {
		vector.StrokeLine(m.img, float32(z.Min.X), float32(y), float32(z.Max.X), float32(y), float32(s), grid, false)
	}

	nodes := m.mapNodes()
	pts := m.project(nodes)
	m.nodes, m.pts = nodes, pts
	m.labels = make([]image.Rectangle, len(nodes))
	byID := make(map[content.SystemID]mapPoint, len(nodes))
	for i, n := range nodes {
		byID[n.id] = pts[i]
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
	// ring, then the zone-colored disc and the label band. Control is the
	// disc's shape — filled while the charter holds, an outline under the
	// Old Works — and a front system wears the contested ring outside every
	// interaction ring.
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
		if _, contested := m.war.Contest[n.id]; contested {
			vector.StrokeCircle(m.img, float32(pt.x), float32(pt.y), float32(17*s), float32(1.5*s), theme.NRGBA(p.Info, 220), true)
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
		zone := m.mapSecurity(n.security)
		if m.war.WorksHeld(n.id) {
			vector.StrokeCircle(m.img, float32(pt.x), float32(pt.y), rad, float32(1.5*s), zone, true)
		} else {
			vector.DrawFilledCircle(m.img, float32(pt.x), float32(pt.y), rad, zone, true)
		}
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

		// The destination flag is the visual affordance the old hint
		// spelled out: hover carries the outline, the confirmed
		// destination the solid one.
		switch {
		case n.id == m.dest:
			m.drawFlag(float32(pt.x)+float32(10*s), float32(pt.y)-float32(18*s), true, p.Accent)
		case n.id == m.sel:
			m.drawFlag(float32(pt.x)+float32(10*s), float32(pt.y)-float32(18*s), false, p.Accent)
		case n.id == m.hover:
			m.drawFlag(float32(pt.x)+float32(10*s), float32(pt.y)-float32(18*s), false, theme.NRGBA(p.Text, 160))
		}
	}

	// Zone legend: the node colors decoded in the canon's zone words with
	// their complementary status labels (docs/lore/UNIVERSE.md — Zones).
	for i, sec := range zoneSecs {
		y := float64(z.Max.Y-theme.Px(mapPad)) + float64(i)*16*s
		vector.DrawFilledCircle(m.img, float32(z.Min.X)+float32(12*s), float32(y)+float32(6*s), float32(4*s), m.mapSecurity(sec), true)
		render.Text(m.img, zoneLabel(sec), float64(z.Min.X)+24*s, y, m.th.TS().Tiny, p.Dim)
	}
	m.drawWarKey()
	m.drawJumpBadge(cur)
}

// drawJumpBadge is the route summary as one content chip: the gate glyph
// and the jump count (or the no-path warning) in the graph's top-left.
func (m *MapView) drawJumpBadge(cur content.SystemID) {
	if m.dest == "" {
		return
	}
	p := m.th.Pal()
	label, col := "No gate path", p.Danger
	if jumps := len(m.route(cur, m.dest)) - 1; jumps > 0 {
		label, col = strconv.Itoa(jumps)+" jumps", p.Accent
	}
	x := m.graph.Min.X + theme.Px(12)
	y := m.graph.Min.Y + theme.Px(12)
	w := int(render.Measure(label, m.th.TS().Tiny)) + theme.Px(26)
	h := theme.Px(20)
	theme.DrawRoundBox(m.img, image.Rect(x, y, x+w, y+h), theme.NRGBA(col, 26), theme.NRGBA(col, 170), theme.Px(1), theme.AllRadii(theme.RadiusPanel))
	drawIcon(m.img, "kind_gate", x+theme.Px(4), y+theme.Px(3), theme.Px(icons.Inline), col)
	render.Text(m.img, label, float64(x+theme.Px(22)), float64(y+theme.Px(3)), m.th.TS().Tiny, col)
}

// drawFlag paints the destination/selection pennant: a pole and a small
// triangular flag, filled for the confirmed destination and outlined for
// the hover/selection affordance.
func (m *MapView) drawFlag(x, y float32, solid bool, col color.Color) {
	s := float32(theme.Scale())
	vector.StrokeLine(m.img, x, y, x, y+14*s, max(1, s), col, true)
	var path vector.Path
	path.MoveTo(x, y)
	path.LineTo(x+8*s, y+3*s)
	path.LineTo(x, y+6*s)
	path.Close()
	var cs ebiten.ColorScale
	cs.ScaleWithColor(col)
	opts := &vector.DrawPathOptions{AntiAlias: true, ColorScale: cs}
	if solid {
		vector.FillPath(m.img, &path, &vector.FillOptions{}, opts)
	} else {
		vector.StrokePath(m.img, &path, &vector.StrokeOptions{Width: max(1, s)}, opts)
	}
}

// drawTip paints the facility glyph's label beside the pointer (the canvas
// surrogate for the rail's tooltip).
func (m *MapView) drawTip() {
	if m.tip == "" || m.ptr == nil || m.gfx == nil {
		return
	}
	p := m.th.Pal()
	r := m.gfx.GetWidget().Rect
	pt := m.ptr.At().Sub(r.Min)
	face := m.th.Face(theme.StepTiny)
	tw, _ := text.Measure(m.tip, *face, 0)
	w := int(tw) + theme.Px(12)
	h := theme.Px(18)
	b := m.img.Bounds()
	x := min(max(pt.X+theme.Px(12), theme.Px(4)), b.Dx()-w-theme.Px(4))
	y := max(pt.Y-h-theme.Px(6), theme.Px(4))
	theme.DrawRoundBox(m.img, image.Rect(x, y, x+w, y+h), theme.NRGBA(p.BGHard, 240), theme.NRGBA(p.Border, 220), theme.Px(1), theme.AllRadii(theme.RadiusPanel))
	theme.DrawCardText(m.img, m.tip, face, x+theme.Px(6), y+theme.Px(4), p.Text)
}

// drawIcon paints one manifest glyph at a canvas position.
func drawIcon(dst *ebiten.Image, name string, x, y, size int, tint color.RGBA) {
	if size < 1 {
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(x), float64(y))
	dst.DrawImage(icons.Get(name, size, tint), op)
}

// drawOverview paints the selected system's overview: name and security
// badge, the region pair, the count chips, one row per celestial (with a
// station's facilities, a belt's ore chips and a gate's destination chip),
// and the war readout at the bottom.
func (m *MapView) drawOverview(cur content.SystemID) {
	z := m.layout.right
	p := m.th.Pal()
	theme.DrawRoundBox(m.img, z, theme.NRGBA(p.Cell, 235), theme.NRGBA(p.Border, 190), theme.Px(1), theme.AllRadii(theme.RadiusPanel))

	id := m.sel
	if id == "" {
		id = cur
	}
	def, ok := m.cv.Defs().System(id)
	if !ok {
		return
	}
	x := z.Min.X + theme.Px(mapInset)
	y := z.Min.Y + theme.Px(mapInset)

	name := def.Name
	if name == "" {
		name = string(def.ID)
	}
	face := m.th.FaceBold(theme.StepSmall)
	theme.DrawCardText(m.img, name, face, x, y, p.Text)
	if id == m.sel {
		nw, _ := text.Measure(name, *face, 0)
		m.drawFlag(float32(x)+float32(nw)+float32(theme.Px(8)), float32(y)+float32(theme.Px(2)), id == m.dest, p.Accent)
	}
	y += theme.Px(22)

	// Security is a colour badge in the map's own zone palette, the zone
	// words underneath it.
	if zl := zoneLabel(def.Security); zl != "" {
		col := m.mapSecurity(def.Security)
		tw := int(render.Measure(zl, m.th.TS().Tiny))
		theme.DrawRoundBox(m.img, image.Rect(x, y, x+tw+theme.Px(22), y+theme.Px(18)),
			theme.NRGBA(col, 36), theme.NRGBA(col, 190), theme.Px(1), theme.AllRadii(theme.RadiusPanel))
		vector.DrawFilledCircle(m.img, float32(x+theme.Px(8)), float32(y+theme.Px(9)), float32(theme.Px(4)), col, true)
		render.Text(m.img, zl, float64(x+theme.Px(16)), float64(y+theme.Px(3)), m.th.TS().Tiny, p.Text)
		y += theme.Px(22)
	}
	if rc := regionLine(def.Region, def.Constellation); rc != "" {
		render.Text(m.img, rc, float64(x), float64(y), m.th.TS().Tiny, p.Dim)
		y += theme.Px(16)
	}

	bodies := overviewBodies(def)
	stations, gates, belts, anomalies := overviewCounts(bodies)
	y += theme.Px(2)
	y = m.drawCounts(x, y, stations, gates, belts, anomalies)
	y += theme.Px(6)

	for _, b := range bodies {
		if y > z.Max.Y-theme.Px(mapInset)-theme.Px(30) {
			break // shortcut: the overview clips, not scrolls; upgrade when a system outgrows the column.
		}
		switch b.Kind {
		case content.CelStation:
			y = m.drawBodyRow(x, y, b.Icon, b.Name, p.Text)
			sz := theme.Px(icons.Inline)
			fx := x + theme.Px(22)
			for _, sid := range b.Services {
				drawIcon(m.img, serviceIcon(sid), fx, y, sz, p.Info)
				m.facs = append(m.facs, mapHit{rect: image.Rect(fx, y, fx+sz, y+sz), text: serviceLabel(sid)})
				fx += theme.Px(22)
			}
			y += theme.Px(16)
		case content.CelBelt:
			y = m.drawBodyRow(x, y, b.Icon, b.Name, p.Text)
			for _, ore := range b.Ores {
				if y > z.Max.Y-theme.Px(mapInset)-theme.Px(20) {
					break
				}
				y = m.drawOre(x, y, ore)
			}
		case content.CelGate:
			label := m.systemName(b.Gate)
			if label == "" {
				label = b.Name
			}
			w := int(render.Measure(label, m.th.TS().Small)) + theme.Px(30)
			r := image.Rect(x, y, x+w, y+theme.Px(20))
			theme.DrawRoundBox(m.img, r, theme.NRGBA(p.Info, 24), theme.NRGBA(p.Info, 130), theme.Px(1), theme.AllRadii(theme.RadiusPanel))
			drawIcon(m.img, b.Icon, x+theme.Px(4), y+theme.Px(2), theme.Px(icons.Inline), p.Info)
			render.Text(m.img, label, float64(x+theme.Px(24)), float64(y+theme.Px(1)), m.th.TS().Small, p.Text)
			m.gates = append(m.gates, mapHit{rect: r, id: b.Gate, text: label})
			y += theme.Px(24)
		case content.CelAnomaly:
			y = m.drawBodyRow(x, y, b.Icon, b.Name, p.Danger)
		default:
			y = m.drawBodyRow(x, y, b.Icon, b.Name, p.Text)
		}
	}

	// The war readout the card used to carry: the holder and, on a front,
	// the meter from the holder's side.
	if m.sel != "" && y < z.Max.Y-theme.Px(mapInset)-theme.Px(16) {
		y += theme.Px(4)
		for _, l := range m.readoutLines() {
			render.Text(m.img, l.text, float64(x), float64(y), m.th.TS().Tiny, l.col)
			y += theme.Px(15)
		}
	}
}

// drawCounts paints the header's density chips: one icon plus number per
// counted kind (stations, gates, belts, anomalies).
func (m *MapView) drawCounts(x, y int, stations, gates, belts, anomalies int) int {
	p := m.th.Pal()
	items := []struct {
		n    int
		icon string
	}{
		{stations, "kind_station"},
		{gates, "kind_gate"},
		{belts, "kind_belt"},
		{anomalies, "kind_anomaly"},
	}
	sz := theme.Px(icons.Inline)
	cx := x
	for _, it := range items {
		drawIcon(m.img, it.icon, cx, y, sz, p.Dim)
		label := strconv.Itoa(it.n)
		render.Text(m.img, label, float64(cx+theme.Px(18)), float64(y+1), m.th.TS().Tiny, p.Text)
		cx += theme.Px(18) + int(render.Measure(label, m.th.TS().Tiny)) + theme.Px(12)
	}
	return y + theme.Px(18)
}

// drawBodyRow paints one celestial row (icon and display name).
func (m *MapView) drawBodyRow(x, y int, icon, name string, tint color.RGBA) int {
	sz := theme.Px(icons.Inline)
	drawIcon(m.img, icon, x, y+theme.Px(1), sz, tint)
	render.Text(m.img, name, float64(x+sz+theme.Px(6)), float64(y), m.th.TS().Small, tint)
	return y + theme.Px(mapBodyH)
}

// drawOre paints one belt's ore chip: the ore's art (drop-in when the
// player has one, the manifest asteroid otherwise) and the belt's count and
// per-asteroid volume.
func (m *MapView) drawOre(x, y int, ore overviewOre) int {
	p := m.th.Pal()
	drawIcon(m.img, oreIcon(ore.Ore), x, y+theme.Px(1), theme.Px(icons.Inline), p.Warn)
	render.Text(m.img, m.oreName(ore.Ore), float64(x+theme.Px(20)), float64(y), m.th.TS().Tiny, p.Text)
	read := theme.FormatInt(int64(ore.Count)) + " · " + theme.FormatVolume(ore.Volume)
	right := m.layout.right.Max.X - theme.Px(mapInset)
	render.Text(m.img, read, float64(right)-render.Measure(read, m.th.TS().Tiny), float64(y), m.th.TS().Tiny, p.Dim)
	return y + theme.Px(17)
}

// readoutLines is the selected system's war readout: who holds it and, on
// a contested front, the meter (the same language the war key decodes). A
// destination's route is the header's jump badge, not a line here.
func (m *MapView) readoutLines() []mapCardLine {
	def, ok := m.cv.Defs().System(m.sel)
	if !ok {
		return nil
	}
	p := m.th.Pal()
	var lines []mapCardLine
	if _, holds := m.war.Control[def.ID]; holds {
		holder := "Charter-held"
		if m.war.WorksHeld(def.ID) {
			holder = "Works-held"
		}
		lines = append(lines, mapCardLine{text: holder, col: p.Text})
		// A quiet meter (under one percent) stays off the panel: the ring
		// already marks the front, and "press at 0%" is noise.
		if c, front := m.war.Contest[def.ID]; front && math.Round(c.Meter*100) >= 1 {
			lines = append(lines, mapCardLine{text: contestLine(m.war.WorksHeld(def.ID), c.Meter), col: p.Info})
		}
	}
	return lines
}

// mapCardLine is one row of the overview's readout block.
type mapCardLine struct {
	text string
	col  color.RGBA
}

// contestLine renders a front's meter from the holder's side: the number is
// the other side's progress toward the flip.
func contestLine(worksHeld bool, meter float64) string {
	if worksHeld {
		return "Front: the charter pushes at " + theme.PctLabel(meter)
	}
	return "Front: the Works press at " + theme.PctLabel(meter)
}

// warKeyMark is one war-legend marker shape: the node vocabulary the key
// decodes.
type warKeyMark int

const (
	warMarkNone warKeyMark = iota
	warMarkCharter
	warMarkWorks
	warMarkFront
)

// warKey is one war-legend row: the marker and the words.
type warKey struct {
	mark warKeyMark
	text string
	col  color.RGBA
}

// warKeyRows builds the war legend: control (filled = charter, outline =
// Old Works), the contested ring, and the war treasury — the war's health
// in one line. No war, no rows.
func warKeyRows(p theme.Palette, w widgets.WarView) []warKey {
	if !w.Has() {
		return nil
	}
	return []warKey{
		{mark: warMarkCharter, text: "Charter-held", col: p.Text},
		{mark: warMarkWorks, text: "Works-held", col: p.Text},
		{mark: warMarkFront, text: "Front", col: p.Info},
		{text: "War treasury: " + theme.FormatCredits(w.Treasury), col: p.Dim},
	}
}

// drawWarKey paints the war legend in the graph's top-right corner (the
// jump badge owns the top-left).
func (m *MapView) drawWarKey() {
	p := m.th.Pal()
	rows := warKeyRows(p, m.war)
	if len(rows) == 0 {
		return
	}
	s := theme.Scale()
	face := m.th.TS().Tiny
	width := 0.0
	for _, r := range rows {
		width = math.Max(width, render.Measure(r.text, face))
	}
	x := float64(m.graph.Max.X-theme.Px(12)) - width
	for i, r := range rows {
		y := float64(m.graph.Min.Y) + 12*s + float64(i)*16*s
		mx, my := float32(x-9*s), float32(y+6*s)
		switch r.mark {
		case warMarkCharter:
			vector.DrawFilledCircle(m.img, mx, my, float32(4*s), p.Dim, true)
		case warMarkWorks:
			vector.StrokeCircle(m.img, mx, my, float32(4*s), float32(1.5*s), p.Dim, true)
		case warMarkFront:
			vector.StrokeCircle(m.img, mx, my, float32(5*s), float32(1.5*s), p.Info, true)
		}
		render.Text(m.img, r.text, x, y, face, r.col)
	}
}

// ---------------------------------------------------------------------------
// The overview's model (content only, pure)
// ---------------------------------------------------------------------------

// overviewBodies lists a system's celestials for the overview, ordered
// star, planet, moon, station, belt, gate, anomaly (stable within a kind).
func overviewBodies(def *content.SystemDef) []overviewBody {
	if def == nil {
		return nil
	}
	out := make([]overviewBody, 0, len(def.Celestials))
	for i := range def.Celestials {
		c := &def.Celestials[i]
		b := overviewBody{Kind: c.Kind, Name: c.Name, Icon: celIcon(c.Kind)}
		if c.Station != nil {
			b.Services = append([]string(nil), c.Station.Services...)
		}
		if c.Belt != nil {
			for _, a := range c.Belt.Asteroids {
				b.Ores = append(b.Ores, overviewOre{Ore: a.Ore, Count: a.Count, Volume: a.Volume})
			}
		}
		if c.Gate != nil {
			b.Gate = c.Gate.ToSystem
		}
		out = append(out, b)
	}
	sort.SliceStable(out, func(i, j int) bool { return celRank(out[i].Kind) < celRank(out[j].Kind) })
	return out
}

// overviewCounts sums the density chips from the ordered bodies.
func overviewCounts(bodies []overviewBody) (stations, gates, belts, anomalies int) {
	for _, b := range bodies {
		switch b.Kind {
		case content.CelStation:
			stations++
		case content.CelGate:
			gates++
		case content.CelBelt:
			belts++
		case content.CelAnomaly:
			anomalies++
		}
	}
	return
}

// celRank is the overview's display order for the celestial kinds.
func celRank(kind string) int {
	switch kind {
	case content.CelStar:
		return 0
	case content.CelPlanet:
		return 1
	case content.CelMoon:
		return 2
	case content.CelStation:
		return 3
	case content.CelBelt:
		return 4
	case content.CelGate:
		return 5
	case content.CelAnomaly:
		return 6
	}
	return 7
}

// celIcon maps a celestial kind to its manifest silhouette.
func celIcon(kind string) string {
	switch kind {
	case content.CelStar:
		return "kind_star"
	case content.CelPlanet:
		return "kind_planet"
	case content.CelMoon:
		return "kind_moon"
	case content.CelStation:
		return "kind_station"
	case content.CelBelt:
		return "kind_belt"
	case content.CelGate:
		return "kind_gate"
	case content.CelAnomaly:
		return "kind_anomaly"
	}
	return icons.Fallback
}

// serviceIcon is a service's manifest glyph. The services table draws no
// glyph for repair, so the manifest's spanner stands in; an unknown id
// falls back like every icon.
func serviceIcon(id string) string {
	s, ok := content.Service(id)
	switch {
	case !ok:
		return icons.Fallback
	case s.Icon != "":
		return s.Icon
	}
	return "mod_utility"
}

// serviceLabel is a service's display name (the id only when unknown).
func serviceLabel(id string) string {
	if s, ok := content.Service(id); ok && s.Label != "" {
		return s.Label
	}
	return id
}

// oreIcon is an ore's icon: dropped art keyed by the content id when the
// player has it, the manifest's asteroid silhouette otherwise.
func oreIcon(id content.ItemID) string {
	if icons.Has(string(id)) {
		return string(id)
	}
	return "kind_asteroid"
}

// oreName is an ore's display name (the id only when content has none).
func (m *MapView) oreName(id content.ItemID) string {
	if it, ok := m.cv.Defs().Item(id); ok && it.Name != "" {
		return it.Name
	}
	return string(id)
}

// regionLine composes the region and constellation labels without ever
// doubling a name that serves both levels.
func regionLine(region, constellation string) string {
	switch {
	case region == "":
		return constellation
	case constellation == "" || constellation == region:
		return region
	}
	return region + " · " + constellation
}

// ---------------------------------------------------------------------------
// Graph inputs
// ---------------------------------------------------------------------------

// systemName is a system's display name, its id only when content has none.
func (m *MapView) systemName(id content.SystemID) string {
	if def, ok := m.cv.Defs().System(id); ok && def.Name != "" {
		return def.Name
	}
	return string(id)
}

// searchText is the browser's current filter.
func (m *MapView) searchText() string {
	if m.search == nil {
		return ""
	}
	return m.search.GetText()
}

// listedNodes filters the id-sorted systems by the search field, ranked by
// the house fuzzy search (the market's boardSearch: diacritics folded, so
// "glus" finds Glůšan). An empty query is the canonical id order.
func (m *MapView) listedNodes() []mapNode {
	nodes := m.mapNodes()
	q := strings.TrimSpace(m.searchText())
	if q == "" {
		return nodes
	}
	hits := boardSearch(q, nodes, func(n mapNode) []string { return []string{n.name} }, boardSearchSpec{})
	out := make([]mapNode, len(hits))
	for i, h := range hits {
		out[i] = h.Key
	}
	return out
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

// project maps graph positions (ly) into the graph column, preserving
// aspect ratio and centering the bounds (port of ui's StarmapNodes).
func (m *MapView) project(nodes []mapNode) []mapPoint {
	pts := make([]mapPoint, len(nodes))
	if len(nodes) == 0 {
		return pts
	}
	area := m.graph
	if area.Empty() {
		area = m.zones().graph
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
	pad := float64(theme.Px(mapPad))
	availW, availH := float64(area.Dx())-2*pad, float64(area.Dy())-2*pad
	scale := 1.0
	switch {
	case spanX > 0 && spanY > 0:
		scale = min(availW/spanX, availH/spanY)
	case spanX > 0:
		scale = availW / spanX
	case spanY > 0:
		scale = availH / spanY
	}
	offX := float64(area.Min.X) + pad + (availW-spanX*scale)/2
	offY := float64(area.Min.Y) + pad + (availH-spanY*scale)/2
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
		return "Starmap: " + constellation
	case constellation == "" || region == constellation:
		return "Starmap: " + region
	default:
		return "Starmap: " + region + " / " + constellation
	}
}
