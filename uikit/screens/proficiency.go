package screens

import (
	"fmt"
	"image"
	"math"
	"reflect"
	"sort"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// ProfPrereq is one authored dependency edge: a family and the level of it
// the dependent needs. The ids are the sim's family ids; the screen never
// resolves them against content.
type ProfPrereq struct {
	Family string
	Level  int
}

// ProfFamily is one proficiency family as the pilot sees it: the level
// granted, the XP banked toward the next (XP/XPNext, 0 at the cap), the
// authored prerequisites and whether the gate is satisfied. It is the
// panel's whole model; the sim maps its own family type onto it.
type ProfFamily struct {
	Family   string // stable family id
	Name     string // display name
	Level    int    // granted levels (0..MaxLevel)
	XP       int    // XP banked toward the next level
	XPNext   int    // XP the next level needs (0 at MaxLevel)
	MaxLevel int
	Prereqs  []ProfPrereq // authored dependency edges
	Meets    bool         // gate satisfied: own level + every prereq
	Starter  bool         // starter essential (gate 0)
}

// Proficiency geometry, in design units through the Px funnel. The graph
// is fit into the layer, never scrolled: a dozen families land in a few
// ranks and the fit factor stays 1.
const (
	profNodeW  = 240
	profNodeH  = 92
	profColGap = 64
	profRowGap = 18
	profPad    = 28
	profHeader = 86 // the title band above the graph
	profPipW   = 8
	profPipGap = 4
)

// Proficiency is the proficiency graph (the owner's "tech tree"): every
// proficiency family as a node, its prerequisite edges as arrows
// (prerequisite → dependent), the pilot's level pips and XP readout on
// each node, and the gated families dimmed.
//
// It is the house's one full-viewport layer: mounted through
// Kit.AddBackdropWindow, it paints over the world and BELOW every window
// and all chrome (Kit.Draw), and Own resolves bare layer space after the
// window stack and the root chrome, so a click on a window above still
// reaches it while the world stays blocked. The layer re-fits itself to
// the viewport on every Sync: it is not a movable window (no title bar,
// no grips), and Esc / the Screen row's key close it.
type Proficiency struct {
	th       theme.Theme
	model    func() []ProfFamily
	viewport func() image.Rectangle

	win  *widget.Window
	gfx  *widget.Graphic
	img  *ebiten.Image
	rect image.Rectangle // the canvas the image and the window cover

	drawn  bool
	shown  []ProfFamily
	nodes  []profNode
	edges  []profEdge
	redraw int // repaint count (the determinism pin watches the model)
}

// NewProficiency builds the proficiency layer over the pilot's model (nil
// reads as no families) and the live viewport (nil keeps the design
// viewport). Mount it with Kit.AddBackdropWindow and call Sync every frame.
func NewProficiency(env widgets.Seams, model func() []ProfFamily, viewport func() image.Rectangle) *Proficiency {
	p := &Proficiency{th: env, model: model, viewport: viewport}
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

// Sync fits the layer to the viewport and repaints when the model changed.
func (p *Proficiency) Sync() {
	p.fit()
	fams := []ProfFamily(nil)
	if p.model != nil {
		fams = p.model()
	}
	if p.drawn && reflect.DeepEqual(fams, p.shown) {
		return
	}
	p.shown = fams
	p.nodes, p.edges = proficiencyLayout(fams, p.rect)
	p.redraw++
	p.draw(fams)
	p.drawn = true
}

// Canvas is the painted layer (the reopen pin compares it byte for byte).
func (p *Proficiency) Canvas() *ebiten.Image { return p.img }

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
	GateText  string // "needs Autocannons 2" while locked, else ""
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

	// The graph's natural size, then the one fit factor into the band the
	// header leaves. Geometry shrinks with the fit; the type does not:
	// shortcut: a graph far wider than the layer would crowd its labels,
	// add pan/zoom if the family count ever outgrows one screen.
	nodeW, nodeH := float64(theme.Px(profNodeW)), float64(theme.Px(profNodeH))
	colGap, rowGap := float64(theme.Px(profColGap)), float64(theme.Px(profRowGap))
	naturalW := float64(cols)*nodeW + float64(cols-1)*colGap
	naturalH := float64(maxRows)*nodeH + float64(maxRows-1)*rowGap

	pad := float64(theme.Px(profPad))
	avail := image.Rect(canvas.Min.X+int(pad), canvas.Min.Y+theme.Px(profHeader),
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
				GateText: profGateText(fam, fams),
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

// profGateText names the first unmet prerequisite, by its display name
// and level ("needs Autocannons 2"); empty while the family is open.
func profGateText(f ProfFamily, fams []ProfFamily) string {
	if f.Meets {
		return ""
	}
	for _, p := range f.Prereqs {
		pf := famAt(fams, p.Family)
		name := p.Family
		if pf != nil && pf.Name != "" {
			name = pf.Name
		}
		if pf == nil || pf.Level < p.Level {
			return fmt.Sprintf("needs %s %d", name, p.Level)
		}
	}
	return "locked"
}

// draw paints the whole layer: the opaque backdrop, the header band, the
// arrows and every node.
func (p *Proficiency) draw(fams []ProfFamily) {
	pal := p.th.Pal()
	b := p.img.Bounds()
	p.img.Clear()
	// The layer is opaque: it covers the world, not the windows above it.
	vector.DrawFilledRect(p.img, 0, 0, float32(b.Dx()), float32(b.Dy()), theme.Solid(pal.BGHard), false)

	p.drawHeader(fams)

	for _, e := range p.edges {
		a, z := p.nodes[e.From], p.nodes[e.To]
		x0, y0 := float64(a.Rect.Max.X), float64(a.Rect.Min.Y)+float64(a.Rect.Dy())/2
		x1, y1 := float64(z.Rect.Min.X), float64(z.Rect.Min.Y)+float64(z.Rect.Dy())/2
		col := theme.NRGBA(pal.Border, 150)
		if e.Met {
			col = theme.NRGBA(pal.Accent, 150)
		}
		w := float32(theme.Px(1))
		vector.StrokeLine(p.img, float32(x0), float32(y0), float32(x1), float32(y1), w, col, true)
		// Arrowhead: two short strokes back from the dependent's edge.
		wing := float32(theme.Px(7))
		vector.StrokeLine(p.img, float32(x1), float32(y1), float32(x1)-wing, float32(y1)-wing/2, w, col, true)
		vector.StrokeLine(p.img, float32(x1), float32(y1), float32(x1)-wing, float32(y1)+wing/2, w, col, true)
	}

	for _, n := range p.nodes {
		p.drawNode(n)
	}
}

// drawNode paints one family: the box (lit for an open family, unlit for
// a gated one), the name, the starter tag, the level pips, the level and
// XP readouts and the unmet prerequisite's line.
func (p *Proficiency) drawNode(n profNode) {
	pal := p.th.Pal()
	pad := theme.Px(12)
	name, edge := pal.Text, theme.NRGBA(pal.Border, 220)
	body := theme.Solid(pal.Cell)
	if n.Locked {
		name, edge = pal.Dim, theme.NRGBA(pal.Border, 110)
		body = theme.Solid(pal.BGHard)
	}
	theme.DrawRoundBox(p.img, n.Rect, body, edge, theme.Px(1), theme.AllRadii(theme.RadiusPanel))

	x := n.Rect.Min.X + pad
	y := n.Rect.Min.Y
	theme.DrawCardText(p.img, n.Name, p.th.FaceBold(theme.StepSmall), x, y+theme.Px(8), name)
	if n.Starter {
		theme.DrawCardText(p.img, "starter", p.th.Face(theme.StepTiny),
			n.Rect.Max.X-pad-theme.Px(44), y+theme.Px(10), pal.Head)
	}

	// Level pips: one box per level, filled up to the granted level.
	pips := max(n.MaxLevel, 0)
	for i := 0; i < pips; i++ {
		r := image.Rect(x+i*theme.Px(profPipW+profPipGap), y+theme.Px(38),
			x+i*theme.Px(profPipW+profPipGap)+theme.Px(profPipW), y+theme.Px(38)+theme.Px(profPipW))
		lit := i < n.Level
		col := theme.NRGBA(pal.Border, 200)
		if lit && !n.Locked {
			col = theme.Solid(pal.Accent)
		} else if lit {
			col = theme.NRGBA(pal.Accent, 120) // earned, but the branch is shut
		}
		vector.DrawFilledRect(p.img, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), col, false)
	}
	lv := n.LevelText
	theme.DrawCardText(p.img, lv, p.th.Face(theme.StepTiny),
		n.Rect.Max.X-pad-theme.Px(int(render.Measure(lv, p.th.TS().Tiny))), y+theme.Px(34),
		pal.Dim)

	theme.DrawCardText(p.img, n.XPText, p.th.Face(theme.StepTiny), x, y+theme.Px(52), pal.Dim)
	if n.GateText != "" {
		theme.DrawCardText(p.img, n.GateText, p.th.Face(theme.StepTiny), x, y+theme.Px(70), pal.Dim)
	}
}

// drawHeader paints the band above the graph: the layer's name, the case
// count and the fixed way out. The toggle's own chord is never printed
// (it is rebindable); Esc is the fixed layer and always closes.
func (p *Proficiency) drawHeader(fams []ProfFamily) {
	pal := p.th.Pal()
	pad := theme.Px(profPad)
	open, levels, maxLevels := 0, 0, 0
	for _, f := range fams {
		if f.Meets {
			open++
		}
		levels += f.Level
		maxLevels += f.MaxLevel
	}
	theme.DrawCardText(p.img, "Proficiency", p.th.FaceBold(theme.StepLarge), pad, theme.Px(18), pal.Text)
	line := fmt.Sprintf("%d of %d families open · %d of %d levels earned", open, len(fams), levels, maxLevels)
	if open < len(fams) {
		line += " · dimmed families await a prerequisite"
	}
	theme.DrawCardText(p.img, line, p.th.Face(theme.StepTiny), pad, theme.Px(56), pal.Dim)
	hint := "Esc closes"
	theme.DrawCardText(p.img, hint, p.th.Face(theme.StepTiny),
		p.img.Bounds().Dx()-pad-theme.Px(int(render.Measure(hint, p.th.TS().Tiny))),
		theme.Px(56), pal.Head)
}
