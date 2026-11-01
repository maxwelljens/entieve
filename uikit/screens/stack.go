package screens

import (
	"image"
	"math"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// Stack picker: a cluster of stacked entities is unreachable by the world's
// one-entity pick, so hovering one opens a vertical column of the entities
// themselves. An entry is the entity as the grid draws it, at the grid's own
// zoom (its real pixels, never scaled to fit), with its name beside the art;
// there is no frame around an entry and no panel behind the column, so the
// entries float over the world. While the column is up the entities it lists
// stop drawing in space: the column is where they are (render.SetHidden), and
// the tail past the cap was never listed, so it stays in the grid.
//
// The column is transient TOP chrome (the rail's contract: above every
// window, under the context menu) and nothing else: no Screen row, no
// keybinding, no sidecar, no RNG. Its order is the pick's own (the paint
// order, nearest to the camera first), so the column leads with what the
// world draws on top. A click selects, a ctrl+click locks (the world's
// chord), a right-click opens the entity's menu.
const (
	// Both are frames, never wall clock (the hudFeedFrames precedent): ~200 ms
	// and ~150 ms at 60 fps, whatever the time acceleration.
	stackDwellFrames = 12 // the cursor must rest on a cluster this long
	stackGraceFrames = 9  // off both the column and the cluster this long: it drops

	stackCap  = 12         // entries listed; the tail stays in space and in the Overview
	stackGap  = theme.SpSm // between entries, and between an entry's art and its name
	stackEdge = 16         // column offset from the cursor (px)
)

// stackPicker is the column itself: a fixed pool of entries (no text rows, no
// scrolling, no expander), refreshed from the hovered cluster every frame.
// Zero entries costs nothing: the column hides and lays out nothing, so the
// layer below keeps its presses.
type stackPicker struct {
	hudChrome
	k     *Kit
	ren   *render.Renderer // draws the entries' art (built with the column)
	root  *widget.Container
	rows  *widget.Container // the stack of entries: no background, no padding
	cells [stackCap]stackCell

	view    render.Camera // the world's framing: entries draw at its zoom and cap
	entries []sim.Info    // the hovered cluster, depth-ordered by the pick
	hiding  []ecs.Entity  // the displayed entries, the grid's hidden set
	seat    image.Point
	shown   bool
	n       int  // entries shown (0 when hidden)
	dirty   bool // an entry's size moved: the column re-seats
	dwell   int  // frames the cursor has rested on a cluster
	grace   int  // frames it has been off both column and cluster
}

// stackCell is one entry: the entity's art at the grid's own size with its
// name to the right, both painted into the cell's own image (never a slot in
// the shared icon atlas).
type stackCell struct {
	g   *widget.Graphic
	img *ebiten.Image
	art int // the art box's width; the name follows it
	w   int
	h   int
}

// newStackPicker builds the column shell only: the rows and their twelve-cell
// pool come on the first open (see build), so a session that never hovers a
// cluster never spends an image on it.
func newStackPicker(k *Kit, root *widget.Container) *stackPicker {
	p := &stackPicker{k: k, root: root}
	p.col = widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout()))
	p.rows = widget.NewContainer(widget.ContainerOpts.Layout(widget.NewRowLayout(
		widget.RowLayoutOpts.Direction(widget.DirectionVertical),
		widget.RowLayoutOpts.Spacing(theme.Px(stackGap)),
	)))
	p.col.AddChild(p.rows)
	return p
}

// build makes the row stack and its cells on the first open — the overview
// filter's rule: a boot-time build would rasterize into the shared glyph atlas
// and shift the title phase's text anti-aliasing (a capture baseline no
// feature should move).
func (p *stackPicker) build() {
	if p.ren != nil {
		return
	}
	// The picker draws the world's own art, so it keeps a renderer of its
	// own: the kit has no handle on the world's, and every sprite it needs
	// (procedural art, drop-in hulls and rocks) is package-level state.
	p.ren = render.New(p.k.Content)
	for i := range p.cells {
		c := &p.cells[i]
		c.img = ebiten.NewImage(1, 1)
		c.g = widget.NewGraphic(widget.GraphicOpts.Image(c.img))
		c.g.GetWidget().MouseButtonClickedEvent.AddHandler(func(args any) {
			if a, ok := args.(*widget.WidgetMouseButtonClickedEventArgs); ok {
				p.click(i, a)
			}
		})
		p.rows.AddChild(c.g)
		theme.ShowWidget(c.g.GetWidget(), false)
	}
	theme.ShowWidget(p.rows.GetWidget(), false)
}

// StackView is the scene's per-frame report of the world's framing: every
// entry draws at the grid's own zoom, capped against the grid's own screen, so
// an entry is the entity's pixels and nothing is scaled to fit.
func (p *stackPicker) SetView(cam render.Camera) { p.view = cam }

// Hover is the scene's per-frame report: the cursor rests on a cluster of
// entries (depth order, as pick resolved it) at the screen point x, y. A
// single entity is no cluster and never opens the column.
func (p *stackPicker) Hover(entries []sim.Info, x, y float64) {
	if len(entries) < 2 {
		p.Leave()
		return
	}
	p.entries = append(p.entries[:0], entries...)
	p.grace = 0
	if p.shown {
		return
	}
	if p.dwell++; p.dwell < stackDwellFrames {
		return
	}
	p.build()
	p.seat = image.Pt(int(x), int(y))
	p.shown = true
	theme.ShowWidget(p.rows.GetWidget(), true)
	p.sync()
}

// Keep holds the column up: the cursor sits on it.
func (p *stackPicker) Keep() { p.dwell, p.grace = 0, 0 }

// Leave is the report that the cursor rests on neither the column nor a
// cluster: the column drops once the grace is spent.
func (p *stackPicker) Leave() {
	p.dwell = 0
	if !p.shown {
		return
	}
	if p.grace++; p.grace >= stackGraceFrames {
		p.Dismiss()
	}
}

// Dismiss drops the column at once (a scene change: the hover is gone).
func (p *stackPicker) Dismiss() {
	p.dwell, p.grace, p.n = 0, 0, 0
	p.shown = false
	p.entries = p.entries[:0]
	p.hiding = p.hiding[:0]
	if p.rows != nil {
		theme.ShowWidget(p.rows.GetWidget(), false)
	}
	// An empty rect claims no press, and the seat anchor is cleared so a
	// later root layout cannot re-place the hidden column over the world.
	w := p.col.GetWidget()
	w.LayoutData = widget.AnchorLayoutData{
		HorizontalPosition: widget.AnchorLayoutPositionStart,
		VerticalPosition:   widget.AnchorLayoutPositionStart,
	}
	w.SetLocation(image.Rectangle{})
	p.col.RequestRelayout()
}

// Hidden is the entities the open column displays: the grid stops drawing them
// while it is up, so the column replaces them in space. Exactly the listed
// ones: the tail past the cap never left the grid.
func (p *stackPicker) Hidden() []ecs.Entity {
	if !p.shown {
		return nil
	}
	p.hiding = p.hiding[:0]
	for _, inf := range p.entries[:min(len(p.entries), stackCap)] {
		if !inf.Entity.IsZero() {
			p.hiding = append(p.hiding, inf.Entity)
		}
	}
	return p.hiding
}

// owns reports whether pt sits on the open column.
func (p *stackPicker) owns(pt image.Point) bool {
	return p.shown && hitRect(p.col.GetWidget(), pt)
}

// click runs one entry's press: right opens the entity's menu (the same spec
// an Overview row declares), left selects and, with Ctrl held, locks.
func (p *stackPicker) click(i int, a *widget.WidgetMouseButtonClickedEventArgs) {
	if !p.shown || i >= len(p.entries) {
		return
	}
	inf := p.entries[i]
	if inf.Entity.IsZero() {
		return
	}
	if a.Button == ebiten.MouseButtonRight {
		widgets.OpenRowMenu(p.k.cv, p.k.sink, p.k.overviewSpec(inf), widgets.ClickPoint(a))
		return
	}
	p.k.Select(inf.Entity)
	if p.k.ctrl {
		p.k.RunIntent(widgets.Lock{Entity: inf.Entity})
	}
}

// sync paints the entries from the cluster: each one's art at the grid's own
// zoom, its name beside it, and the column's geometry (exactly as tall and
// wide as its entries). Every frame: the art is the entity's live pixels (a
// ship turns, a glow breathes), and a zoom change resizes an entry the frame
// it lands, so no cached miniature can go stale.
func (p *stackPicker) sync() {
	if !p.shown {
		return
	}
	n := min(len(p.entries), stackCap)
	zoom := p.view.Zoom
	capDim := math.Max(p.view.ScreenW, p.view.ScreenH)
	name := render.LabelSize * theme.Scale()
	gap := theme.Px(stackGap)
	nameH := int(math.Ceil(name))
	for i := range p.cells {
		c := &p.cells[i]
		theme.ShowWidget(c.g.GetWidget(), i < n)
		if i >= n {
			continue
		}
		inf := p.entries[i]
		art := p.ren.EntityArtSize(p.k.Sim, inf, zoom, capDim)
		w := art + gap + int(math.Ceil(render.Measure(inf.Name, name)))
		h := max(art, nameH)
		if h%2 == 1 {
			h++ // an even box keeps the art centred on whole pixels
		}
		if c.img.Bounds().Dx() != w || c.img.Bounds().Dy() != h {
			c.img = ebiten.NewImage(w, h)
			c.g.Image = c.img
			c.art, c.w, c.h = art, w, h
			p.dirty = true
		}
		c.img.Clear()
		p.ren.DrawEntityBody(c.img, p.k.Sim, inf, render.Camera{
			Center: inf.Pos, Zoom: zoom,
			ScreenW: float64(art), ScreenH: float64(h), CapDim: capDim,
		})
		render.Text(c.img, inf.Name, float64(art+gap), float64((h-nameH)/2), name, p.k.th.Pal().Text)
	}
	if n != p.n {
		p.n = n
		p.dirty = true
	}
	if p.dirty {
		p.dirty = false
		p.place()
		p.col.RequestRelayout() // the entries re-measure at their new sizes
	}
}

// place seats the shown column beside the hover point, flipped left and
// clamped inside the root when the cursor sits near an edge. The seat is the
// one taken when the column opened, so a resting cursor does not drag it.
func (p *stackPicker) place() {
	if !p.shown || p.n == 0 {
		return
	}
	cw, ch := 0, 0
	for i := 0; i < p.n; i++ {
		cw = max(cw, p.cells[i].w)
		ch += p.cells[i].h
	}
	ch += (p.n - 1) * theme.Px(stackGap)
	root := p.root.GetWidget().Rect
	x0 := p.seat.X + theme.Px(stackEdge)
	if !root.Empty() && x0+cw > root.Max.X {
		x0 = p.seat.X - theme.Px(stackEdge) - cw
	}
	y0 := p.seat.Y - ch/2
	if !root.Empty() {
		x0 = clampPx(x0, root.Min.X, root.Max.X-cw)
		y0 = clampPx(y0, root.Min.Y, root.Max.Y-ch)
	}
	rect := image.Rect(x0, y0, x0+cw, y0+ch)
	w := p.col.GetWidget()
	w.LayoutData = widget.AnchorLayoutData{
		HorizontalPosition: widget.AnchorLayoutPositionStart,
		VerticalPosition:   widget.AnchorLayoutPositionStart,
		Padding:            &widget.Insets{Left: x0, Top: y0},
	}
	if w.Rect != rect {
		w.SetLocation(rect)
		p.col.RequestRelayout()
	}
}

// clampPx bounds v to [lo, hi] (lo when the band cannot fit at all).
func clampPx(v, lo, hi int) int {
	if v > hi {
		v = hi
	}
	if v < lo {
		v = lo
	}
	return v
}
