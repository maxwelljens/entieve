package screens

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"reflect"
	"sort"

	"github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// ProfPrereq is one authored dependency edge and ProfFamily one practice
// family: the sim's own read types, aliased so the tree, the card and the
// overview read the sim's walk directly instead of through a UI-shaped
// copy that a field could silently drift from. sim.ProfFamily documents
// the fields (level, XP toward the next, prerequisites, Meets and the
// unmet Gate sentence — the tree and the card print that sentence and never
// word a gate themselves).
type (
	ProfPrereq = sim.ProfPrereq
	ProfFamily = sim.ProfFamily
)

// Proficiency geometry, in design units through the Px funnel. The graph
// is fitted into the layer and then panned/zoomed as one camera: zoom
// scales the whole drawing (type included), so a node's internal offsets
// stay proportional at any zoom.
const (
	profNodeW  = 240
	profNodeH  = 92
	profColGap = 64
	profRowGap = 18
	profPad    = 28
	profPipW   = 8
	profPipGap = 4

	// profZoomStep is the house wheel-notch factor (the world camera's
	// zoomStep); the band keeps a node readable at one end and lets a
	// sprawling graph be inspected at the other.
	profZoomStep = 1.15
	profZoomMin  = 0.5
	profZoomMax  = 2.0

	// profSlop is how far a press may travel and still count as a click
	// (design px): a jittery hand still opens the proficiency card, a drag pans.
	profSlop = 4

	// profGridStep is the backdrop's faint blueprint grid.
	profGridStep = 48
)

// Proficiency is the proficiency graph (the owner's "tech tree"): every
// proficiency as a node, its prerequisite edges as curved arrows
// (prerequisite → dependent), the pilot's level pips and XP readout on each
// node, and the gated proficiencies dimmed. Click a node for its proficiency card
// (what it is for and what pays it practice); the node keeps the hover look
// while its card is open, so the tree says which family you are reading.
// Drag the bare layer to pan, wheel to zoom.
//
// It is the house's backdrop layer (the starmap is the over-chrome one):
// mounted through Kit.AddBackdropWindow, it paints over the world and
// BELOW every window and all chrome (Kit.Draw), and Own resolves bare layer
// space after the window stack and the root chrome, so a click on a window
// above still reaches it while the world stays blocked. The layer re-fits itself to
// the viewport on every Sync: it is not a movable window (no title bar,
// no grips), and Esc / the Screen row's key close it. The pan/zoom camera
// is session state only: never persisted, reset when the layer unmounts.
type Proficiency struct {
	th       theme.Theme
	as       widgets.ActionSink
	ptr      *widgets.Pointer
	model    func() []ProfFamily
	viewport func() image.Rectangle
	// openFamily is the family the open card names ("" while closed): its
	// node wears the selected state. The card is the kit's, so the tree
	// asks instead of remembering (Esc and the × close it there).
	openFamily func() string

	win  *widget.Window
	gfx  *widget.Graphic
	img  *ebiten.Image
	rect image.Rectangle // the canvas the image and the window cover

	drawn  bool
	shown  []ProfFamily
	nodes  []profNode
	edges  []profEdge
	redraw int // repaint count (the determinism pin watches the model)

	// The camera: session-only, reset on unmount. pan is in screen px,
	// zoom around 1.
	zoom  float64
	pan   image.Point
	state profViewState // the last painted view/hover, for repaint gating

	// The gesture: a press arms a pan; a release under profSlop on a node
	// opens its card.
	armed      bool
	active     bool
	dragFrom   image.Point
	panFrom    image.Point
	pressNode  int
	hover      int
	sel        int             // node index of the open card's family (-1: none)
	lastLayout image.Rectangle // the canvas the current nodes were laid out for
}

// profViewState is what a repaint depends on besides the model.
type profViewState struct {
	zoom   float64
	pan    image.Point
	hover  int
	press  int
	sel    int
	locked bool
}

// NewProficiency builds the proficiency layer over the pilot's model (nil
// reads as no families), the live viewport (nil keeps the design viewport)
// and the open proficiency card's family (nil reads as no card). Mount it with
// Kit.AddBackdropWindow and call Sync every frame.
func NewProficiency(env widgets.Seams, model func() []ProfFamily, viewport func() image.Rectangle, openFamily func() string) *Proficiency {
	p := &Proficiency{th: env, as: env, ptr: env.Pointer(), model: model, viewport: viewport,
		openFamily: openFamily, zoom: 1, hover: -1, pressNode: -1, sel: -1}
	p.rect = pxRect(ProficiencyRect())
	p.img = ebiten.NewImage(p.rect.Dx(), p.rect.Dy())
	p.gfx = widget.NewGraphic(widget.GraphicOpts.Image(p.img))
	// One grid cell, stretched: the image is the whole body, so the
	// window rect and the canvas are the same rectangle.
	body := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
		)),
	)
	body.AddChild(p.gfx)
	p.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.Location(p.rect),
		// The layer blocks the world through the hit test (Own), never
		// through ebitenui's input layers: windows above it keep their
		// presses.
		widget.WindowOpts.BlockLower(false),
	)
	return p
}

// ProficiencyRect is the layer's landing rect in design units: the design
// viewport, the one number its Screen row's DefaultRect and its
// constructor share.
func ProficiencyRect() image.Rectangle { return image.Rect(0, 0, 1280, 720) }

// Win returns the layer for Kit.AddBackdropWindow.
func (p *Proficiency) Win() *widget.Window { return p.win }

// Backdrop marks the panel as the kit's under-window layer (the Screen
// host mounts it through AddBackdropWindow).
func (p *Proficiency) Backdrop() bool { return true }

// Unmounted resets the camera and the gesture: pan and zoom are session
// state, so a reopened tree starts fitted and centred. Nothing is painted
// here (the layer is leaving); the next mounted Sync sees the moved state
// and repaints once.
func (p *Proficiency) Unmounted() {
	p.armed, p.active = false, false
	p.hover, p.pressNode, p.sel = -1, -1, -1
	p.zoom, p.pan = 1, image.Point{}
}

// Sync fits the layer to the viewport, polls the frame's gesture, and
// repaints when the model, the camera, the hover or the open proficiency card
// moved.
func (p *Proficiency) Sync() {
	p.fit()
	fams := []ProfFamily(nil)
	if p.model != nil {
		fams = p.model()
	}
	model := !p.drawn || !reflect.DeepEqual(fams, p.shown) || p.rect != p.lastLayout
	if model {
		p.shown = fams
		p.lastLayout = p.rect
		p.nodes, p.edges = proficiencyLayout(fams, p.rect)
	}
	p.sel = -1
	if p.openFamily != nil {
		if fam := p.openFamily(); fam != "" {
			for i, n := range p.nodes {
				if n.Family == fam {
					p.sel = i
					break
				}
			}
		}
	}
	p.poll()
	st := profViewState{zoom: p.zoom, pan: p.pan, hover: p.hover, press: p.press(), sel: p.sel, locked: p.armed && !p.active}
	if model || st != p.state {
		p.state = st
		p.redraw++
		p.draw()
		p.drawn = true
	}
}

// press is the node index the press is still hovering over during an armed
// click (-1 once the press became a pan or left the node).
func (p *Proficiency) press() int {
	if !p.armed || p.active {
		return -1
	}
	return p.pressNode
}

// Canvas is the painted layer (the reopen pin compares it byte for byte).
func (p *Proficiency) Canvas() *ebiten.Image { return p.img }

// View returns the camera (zoom, pan) for the tests and the game.
func (p *Proficiency) View() (float64, image.Point) { return p.zoom, p.pan }

// fit resizes the window and the canvas to the live viewport (nil keeps
// the design rect). It is the layer's only placement: the layer is the
// viewport, so a sidecar rect or a viewport change is re-fitted here.
func (p *Proficiency) fit() {
	r := pxRect(ProficiencyRect())
	if p.viewport != nil {
		if vp := p.viewport(); !vp.Empty() {
			r = image.Rect(0, 0, vp.Dx(), vp.Dy())
		}
	}
	if r == p.rect {
		return
	}
	p.rect = r
	p.img = ebiten.NewImage(max(r.Dx(), 1), max(r.Dy(), 1))
	p.gfx.Image = p.img
	p.win.SetLocation(r)
	p.drawn = false
}

// poll reads the frame's gesture. Only the layer's own pixels answer (a
// window above keeps its wheel and its press): OwnsAt asks whether this
// widget's input layer is on top at the point.
func (p *Proficiency) poll() {
	p.hover = -1
	if p.ptr == nil || p.gfx == nil {
		return
	}
	pt := p.ptr.At()
	over := widgets.OwnsAt(p.gfx.GetWidget(), pt, input.LayerEventTypeMouseButton)

	if over && p.ptr.Wheel() != 0 {
		p.zoomTo(pt, p.zoom*math.Pow(profZoomStep, p.ptr.Wheel()))
	}
	if over && p.ptr.Press(widgets.Left) {
		p.armed, p.active = true, false
		p.dragFrom, p.panFrom = pt, p.pan
		p.pressNode = p.nodeAt(pt)
	}
	if p.armed && p.ptr.Down(widgets.Left) {
		if !p.active {
			dx, dy := pt.X-p.dragFrom.X, pt.Y-p.dragFrom.Y
			p.active = dx*dx+dy*dy > theme.Px(profSlop)*theme.Px(profSlop)
		}
		if p.active {
			p.pan = image.Pt(p.panFrom.X+pt.X-p.dragFrom.X, p.panFrom.Y+pt.Y-p.dragFrom.Y)
			p.clampPan()
		}
	}
	if p.armed && !p.ptr.Down(widgets.Left) {
		// The release is the click: under the slop, on a node, it opens
		// that family's card. A drag never opens anything.
		if !p.active && p.pressNode >= 0 && p.pressNode < len(p.nodes) {
			if p.as != nil {
				n := p.nodes[p.pressNode]
				p.as.Do(widgets.ShowProfFamily{Family: n.Family, At: pt, Level: n.Level, Gate: n.GateText})
			}
			p.hover = p.pressNode
		}
		p.armed, p.active, p.pressNode = false, false, -1
	}
	if over {
		if n := p.nodeAt(pt); n >= 0 {
			p.hover = n
			p.ptr.ClaimShape(ebiten.CursorShapePointer)
		}
	}
}

// nodeAt is the family node under a screen point (-1 for bare layer
// space), through the pan/zoom camera.
func (p *Proficiency) nodeAt(pt image.Point) int {
	gx := float64(pt.X-p.pan.X) / p.zoom
	gy := float64(pt.Y-p.pan.Y) / p.zoom
	for i, n := range p.nodes {
		if float64(n.Rect.Min.X) <= gx && gx <= float64(n.Rect.Max.X) &&
			float64(n.Rect.Min.Y) <= gy && gy <= float64(n.Rect.Max.Y) {
			return i
		}
	}
	return -1
}

// zoomTo applies a new zoom, keeping the graph point under pt in place, and
// re-clamps the pan.
func (p *Proficiency) zoomTo(pt image.Point, z float64) {
	z = math.Max(profZoomMin, math.Min(profZoomMax, z))
	if z == p.zoom {
		return
	}
	fx := float64(pt.X-p.pan.X) / p.zoom
	fy := float64(pt.Y-p.pan.Y) / p.zoom
	p.zoom = z
	p.pan = image.Pt(
		pt.X-int(math.Round(fx*z)),
		pt.Y-int(math.Round(fy*z)))
	p.clampPan()
}

// clampPan keeps the graph reachable: the content's own bounds may not
// leave the viewport. A graph bigger than the viewport may be scrolled to
// its edges; a smaller one may be moved only inside.
func (p *Proficiency) clampPan() {
	b := p.contentBounds()
	r := p.rect
	axis := func(pos, lo, hi int) int {
		if lo > hi {
			lo, hi = hi, lo
		}
		return max(lo, min(hi, pos))
	}
	p.pan.X = axis(p.pan.X, -int(math.Round(float64(b.Min.X)*p.zoom)), r.Max.X-int(math.Round(float64(b.Max.X)*p.zoom)))
	p.pan.Y = axis(p.pan.Y, -int(math.Round(float64(b.Min.Y)*p.zoom)), r.Max.Y-int(math.Round(float64(b.Max.Y)*p.zoom)))
}

// contentBounds is the union of the node rects (empty without a graph).
func (p *Proficiency) contentBounds() image.Rectangle {
	if len(p.nodes) == 0 {
		return image.Rect(0, 0, p.rect.Dx(), p.rect.Dy())
	}
	b := p.nodes[0].Rect
	for _, n := range p.nodes[1:] {
		b = b.Union(n.Rect)
	}
	return b
}

// profNode is one family's laid-out node: its rect, its readouts and its
// gate state, all derived from the model (never authored).
type profNode struct {
	Family, Name string
	Rect         image.Rectangle
	Rank         int

	Level, MaxLevel int
	XP, XPNext      int
	Starter, Locked bool

	LevelText string // "2/5"
	XPText    string // "40/160 XP", "max" or "no levels"
	GateText  string // "Requires Autocannon II" while locked, else ""
}

// profEdge is one prerequisite arrow, by node index.
type profEdge struct {
	From, To int // prerequisite → dependent
	Met      bool
}

// proficiencyLayout lays the families out as a DAG and returns the nodes
// (rank then family id) and the edges. Pure and deterministic: the model
// and the canvas are the only inputs, ties break on the family id, and no
// RNG or clock is read anywhere.
//
// One rank is one column, its members stacked top to bottom, and the whole
// graph takes ONE fit factor into the canvas. A rank never wraps into a
// second column: the roots stay aligned in the leftmost column and every
// arrow runs rightwards to the next rank, so the graph reads as a tree
// rather than a jumble of columns.
func proficiencyLayout(fams []ProfFamily, canvas image.Rectangle) ([]profNode, []profEdge) {
	if len(fams) == 0 {
		return nil, nil
	}
	rank := proficiencyRanks(fams)
	// Group by rank, each group sorted by family id: the row order.
	byRank := map[int][]int{}
	maxRank := 0
	for i := range fams {
		byRank[rank[i]] = append(byRank[rank[i]], i)
		maxRank = max(maxRank, rank[i])
	}
	cols := maxRank + 1
	for _, g := range byRank {
		sort.Slice(g, func(a, b int) bool { return fams[g[a]].Family < fams[g[b]].Family })
	}
	maxRows := 1
	for _, g := range byRank {
		maxRows = max(maxRows, len(g))
	}

	// The graph's natural size, then the one fit factor into the canvas.
	// Geometry shrinks with the fit; the type does not (a node's internals
	// keep their design offsets, so a shrunk box holds a full-size card).
	//
	// shortcut: a graph far taller or wider than the layer crowds its
	// labels; pan and zoom are the way out, not a smaller fit.
	nodeW, nodeH := float64(theme.Px(profNodeW)), float64(theme.Px(profNodeH))
	colGap, rowGap := float64(theme.Px(profColGap)), float64(theme.Px(profRowGap))
	naturalW := float64(cols)*nodeW + float64(cols-1)*colGap
	naturalH := float64(maxRows)*nodeH + float64(maxRows-1)*rowGap

	pad := float64(theme.Px(profPad))
	avail := image.Rect(canvas.Min.X+int(pad), canvas.Min.Y+int(pad),
		canvas.Max.X-int(pad), canvas.Max.Y-int(pad))
	if avail.Dx() <= 0 || avail.Dy() <= 0 {
		avail = canvas
	}
	f := 1.0
	if naturalW > 0 && naturalH > 0 {
		f = math.Min(1, math.Min(float64(avail.Dx())/naturalW, float64(avail.Dy())/naturalH))
	}
	nodeW, nodeH, colGap, rowGap = nodeW*f, nodeH*f, colGap*f, rowGap*f
	totalW := float64(cols)*nodeW + float64(cols-1)*colGap
	totalH := float64(maxRows)*nodeH + float64(maxRows-1)*rowGap
	x0 := float64(avail.Min.X) + (float64(avail.Dx())-totalW)/2
	y0 := float64(avail.Min.Y) + (float64(avail.Dy())-totalH)/2

	nodes := make([]profNode, 0, len(fams))
	nodeOf := make([]int, len(fams)) // model index → node index
	for r := 0; r < cols; r++ {
		for row, i := range byRank[r] {
			nodeOf[i] = len(nodes)
			fam := fams[i]
			x := x0 + float64(r)*(nodeW+colGap)
			y := y0 + float64(row)*(nodeH+rowGap)
			nodes = append(nodes, profNode{
				Family: fam.Family, Name: fam.Name,
				Rect: image.Rect(int(math.Round(x)), int(math.Round(y)),
					int(math.Round(x))+int(math.Round(nodeW)), int(math.Round(y))+int(math.Round(nodeH))),
				Rank: r, Level: fam.Level, MaxLevel: fam.MaxLevel,
				XP: fam.XP, XPNext: fam.XPNext,
				Starter: fam.Starter, Locked: !fam.Meets,
				LevelText: profLevelText(fam), XPText: profXPText(fam),
				GateText: fam.Gate,
			})
		}
	}

	// Edges by node index: the prerequisite's node → the dependent's.
	index := make(map[string]int, len(nodes))
	for i, fam := range fams {
		index[fam.Family] = nodeOf[i]
	}
	var edges []profEdge
	for i, fam := range fams {
		for _, pre := range fam.Prereqs {
			from, ok := index[pre.Family]
			if !ok {
				continue // a prerequisite outside the model draws nothing
			}
			met := false
			if pf := famAt(fams, pre.Family); pf != nil {
				met = pf.Level >= pre.Level
			}
			edges = append(edges, profEdge{From: from, To: nodeOf[i], Met: met})
		}
	}
	return nodes, edges
}

// famAt finds a family by id (nil when absent).
func famAt(fams []ProfFamily, id string) *ProfFamily {
	for i := range fams {
		if fams[i].Family == id {
			return &fams[i]
		}
	}
	return nil
}

// proficiencyRanks assigns each family its longest prerequisite chain: a
// family with no prerequisites in the model sits at rank 0, every other
// one one past its deepest prerequisite. Kahn's algorithm with the family
// id as the pick order, so the ranks never depend on input order; a cycle
// (invalid content) leaves its members at rank 0 instead of hanging.
func proficiencyRanks(fams []ProfFamily) []int {
	present := make(map[string]bool, len(fams))
	for _, f := range fams {
		present[f.Family] = true
	}
	indeg := make([]int, len(fams))
	for i, f := range fams {
		for _, p := range f.Prereqs {
			if present[p.Family] {
				indeg[i]++
			}
		}
	}
	rank := make([]int, len(fams))
	done := make([]bool, len(fams))
	for n := 0; n < len(fams); n++ {
		pick := -1
		for i, f := range fams {
			if done[i] || indeg[i] > 0 {
				continue
			}
			if pick < 0 || f.Family < fams[pick].Family {
				pick = i
			}
		}
		if pick < 0 {
			break
		}
		done[pick] = true
		for i, f := range fams {
			if done[i] || i == pick {
				continue
			}
			for _, p := range f.Prereqs {
				if p.Family == fams[pick].Family {
					rank[i] = max(rank[i], rank[pick]+1)
					indeg[i]--
				}
			}
		}
	}
	return rank
}

// profLevelText is the node's level readout: the granted level over the
// cap ("2/5"), or the cap alone for a family with no levels.
func profLevelText(f ProfFamily) string {
	if f.MaxLevel <= 0 {
		return "0"
	}
	return fmt.Sprintf("%d/%d", f.Level, f.MaxLevel)
}

// profXPText is the node's XP readout: the pair toward the next level,
// "max" at the cap (XPNext is 0 there).
func profXPText(f ProfFamily) string {
	switch {
	case f.MaxLevel <= 0:
		return "no levels"
	case f.Level >= f.MaxLevel || f.XPNext <= 0:
		return "max"
	default:
		return fmt.Sprintf("%d/%d XP", f.XP, f.XPNext)
	}
}

// ---------------------------------------------------------------------------
// Drawing (through the pan/zoom camera)
// ---------------------------------------------------------------------------

// sx/sy map graph coordinates to screen pixels through the camera.
func (p *Proficiency) sx(x int) float32 { return float32(float64(x)*p.zoom) + float32(p.pan.X) }
func (p *Proficiency) sy(y int) float32 { return float32(float64(y)*p.zoom) + float32(p.pan.Y) }

// srect maps a graph rect to the screen.
func (p *Proficiency) srect(r image.Rectangle) image.Rectangle {
	return image.Rect(int(p.sx(r.Min.X)), int(p.sy(r.Min.Y)), int(p.sx(r.Max.X)), int(p.sy(r.Max.Y)))
}

// text draws s at a graph position, scaled with the camera (type zooms with
// the graph: a magnified node keeps its proportions).
func (p *Proficiency) text(s string, face *text.Face, x, y int, col color.Color) {
	if s == "" || face == nil {
		return
	}
	opts := &text.DrawOptions{}
	opts.GeoM.Translate(float64(x), float64(y))
	opts.GeoM.Scale(p.zoom, p.zoom)
	opts.GeoM.Translate(float64(p.pan.X), float64(p.pan.Y))
	opts.ColorScale.ScaleWithColor(col)
	text.Draw(p.img, s, *face, opts)
}

// draw paints the whole layer: the opaque backdrop, the graph and every
// node.
func (p *Proficiency) draw() {
	pal := p.th.Pal()
	b := p.img.Bounds()
	p.img.Clear()
	// The layer is opaque: it covers the world, not the windows above it.
	vector.DrawFilledRect(p.img, 0, 0, float32(b.Dx()), float32(b.Dy()), theme.Solid(pal.BGHard), false)
	p.drawGrid()

	for _, e := range p.edges {
		a, z := p.nodes[e.From], p.nodes[e.To]
		x0, y0 := float64(p.sx(a.Rect.Max.X)), float64((p.sy(a.Rect.Min.Y)+p.sy(a.Rect.Max.Y))/2)
		x1, y1 := float64(p.sx(z.Rect.Min.X)), float64((p.sy(z.Rect.Min.Y)+p.sy(z.Rect.Max.Y))/2)
		col := theme.NRGBA(pal.Border, 150)
		if e.Met {
			col = theme.NRGBA(pal.Accent, 150)
		}
		p.curve(x0, y0, x1, y1, max(float32(1), float32(p.zoom)), col)
		// Arrowhead at the dependent's left edge: the curve arrives
		// horizontally, so the wings open vertically.
		wing := float32(theme.Px(7)) * float32(p.zoom)
		vector.StrokeLine(p.img, float32(x1), float32(y1), float32(x1)-wing, float32(y1)-wing/2, max(float32(1), float32(p.zoom)), col, true)
		vector.StrokeLine(p.img, float32(x1), float32(y1), float32(x1)-wing, float32(y1)+wing/2, max(float32(1), float32(p.zoom)), col, true)
	}

	for i, n := range p.nodes {
		p.drawNode(i, n)
	}
}

// drawGrid is the backdrop's faint blueprint: a deterministic set of
// hairlines in the border role, far below any node.
func (p *Proficiency) drawGrid() {
	pal := p.th.Pal()
	col := theme.NRGBA(pal.Border, 12)
	step := float32(theme.Px(profGridStep))
	b := p.img.Bounds()
	for x := float32(0); x < float32(b.Dx()); x += step {
		vector.StrokeLine(p.img, x, 0, x, float32(b.Dy()), 1, col, false)
	}
	for y := float32(0); y < float32(b.Dy()); y += step {
		vector.StrokeLine(p.img, 0, y, float32(b.Dx()), y, 1, col, false)
	}
}

// curve strokes one prerequisite arrow as a smooth S from (x0,y0) to
// (x1,y1): the control points sit at the horizontal midpoint, so the line
// leaves and arrives flat and the arrowhead reads.
func (p *Proficiency) curve(x0, y0, x1, y1 float64, w float32, col color.Color) {
	mid := (x0 + x1) / 2
	var path vector.Path
	path.MoveTo(float32(x0), float32(y0))
	path.CubicTo(float32(mid), float32(y0), float32(mid), float32(y1), float32(x1), float32(y1))
	var cs ebiten.ColorScale
	cs.ScaleWithColor(col)
	vector.StrokePath(p.img, &path, &vector.StrokeOptions{Width: w, LineJoin: vector.LineJoinRound},
		&vector.DrawPathOptions{AntiAlias: true, ColorScale: cs})
}

// drawNode paints one family: the box (lit for an open family, unlit for
// a gated one, highlighted under the pointer, under a press or while its
// card is open), the name, the starter tag, the level pips, the level and
// XP readouts and the unmet prerequisite's line. Internals are spaced by
// the layout's fit, so they stay inside a shrunk node.
func (p *Proficiency) drawNode(i int, n profNode) {
	pal := p.th.Pal()
	// The internals sit at their design offsets: the type keeps its size
	// (zoom, not the fit, is what grows a node), so a shrunk box holds a
	// full-size card.
	sc := func(v int) int { return theme.Px(v) }
	pad := sc(12)
	name, edge := pal.Text, theme.NRGBA(pal.Border, 220)
	body := theme.Solid(pal.Cell)
	switch {
	case p.press() == i:
		name, edge = pal.Text, theme.Solid(pal.Accent)
		body = theme.Solid(pal.BGHard)
	case p.sel == i:
		// The family whose card is open wears the hover look: the pointer
		// is on the card, so the node itself cannot show it.
		name, edge = pal.Text, theme.Solid(pal.Accent)
		body = theme.NRGBA(pal.Hover, 26)
	case p.hover == i:
		name, edge = pal.Text, theme.Solid(pal.Accent)
		body = theme.NRGBA(pal.Hover, 26)
	case n.Locked:
		name, edge = pal.Dim, theme.NRGBA(pal.Border, 110)
		body = theme.Solid(pal.BGHard)
	}
	maxed := n.MaxLevel > 0 && n.Level >= n.MaxLevel
	if maxed && !n.Locked {
		edge = theme.NRGBA(pal.Accent, 180)
	}
	theme.DrawRoundBox(p.img, p.srect(n.Rect), body, edge, theme.Px(1), theme.AllRadii(theme.RadiusPanel))

	x := n.Rect.Min.X + pad
	y := n.Rect.Min.Y
	p.text(n.Name, p.th.FaceBold(theme.StepSmall), x, y+sc(8), name)

	// Level pips: one box per reachable level, filled up to the granted
	// level (the card's line reads the same painter).
	step := int(math.Round(float64(sc(profPipW+profPipGap)) * p.zoom))
	size := int(math.Round(float64(sc(profPipW)) * p.zoom))
	drawProfPips(p.img, int(p.sx(x)), int(p.sy(y+sc(38))), n.Level, n.MaxLevel,
		step, size, n.Locked, pal)
	lv := n.LevelText
	p.text(lv, p.th.Face(theme.StepTiny),
		n.Rect.Max.X-pad-theme.Px(int(render.Measure(lv, p.th.TS().Tiny))), y+sc(34),
		pal.Dim)

	xpCol := color.RGBA(pal.Dim)
	if maxed && !n.Locked {
		xpCol = pal.Accent
	}
	p.text(n.XPText, p.th.Face(theme.StepTiny), x, y+sc(52), xpCol)
	if n.Starter {
		// The tag shares the XP line, right-aligned: the name row is the
		// name's, and a long family name would run into a corner tag.
		p.text("starter", p.th.Face(theme.StepTiny),
			n.Rect.Max.X-pad-sc(int(render.Measure("starter", p.th.TS().Tiny))), y+sc(52), pal.Head)
	}
	if n.GateText != "" {
		p.text(n.GateText, p.th.Face(theme.StepTiny), x, y+sc(70), pal.Dim)
	}
}
