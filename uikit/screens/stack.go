package screens

import (
	"fmt"
	"image"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// Stack picker: a cluster of stacked entities is unreachable by the world's
// one-entity pick, so hovering one opens a vertical column of the entities
// themselves — each cell draws the entity as the world does (its own body art
// inside its own bracket, see render.DrawEntityBracket), the entity under the
// cursor, and everything the cursor also covers.
//
// The column is transient TOP chrome (the rail's contract: above every
// window, under the context menu) and nothing else: no Screen row, no
// keybinding, no sidecar, no RNG. Its order is the pick's own (the paint
// order, nearest to the camera first), so the column leads with what the
// world draws on top; the cap leaves the tail unaddressable by design — the
// Overview stays the full list. A click selects, a ctrl+click locks (the
// world's chord), a right-click opens the entity's menu, and hovering a cell
// names the entity and its distance (identical kinds stay tellable).
const (
	// Both are frames, never wall clock (the hudFeedFrames precedent): ~200 ms
	// and ~150 ms at 60 fps, whatever the time acceleration.
	stackDwellFrames = 12 // the cursor must rest on a cluster this long
	stackGraceFrames = 9  // off both the column and the cluster this long: it drops

	stackCap     = 12 // cells listed; the tail is unaddressable via the column by design
	stackCellPx  = 32 // design px per cell: room for the bracket frame around the body art
	stackGap     = theme.SpSm
	stackCellPad = theme.SpSm
	stackEdge    = 16 // column offset from the cursor (px)
)

// stackPicker is the column itself: a fixed pool of cells (no text rows, no
// scrolling, no expander), refreshed from the hovered cluster every frame.
// Zero entries costs nothing: the panel hides and the column lays out
// nothing, so the layer below keeps its presses.
type stackPicker struct {
	hudChrome
	k     *Kit
	ren   *render.Renderer // draws the cells' entity art (built with the panel)
	root  *widget.Container
	panel *widget.Container
	cells [stackCap]stackCell

	entries []sim.Info // the hovered cluster, depth-ordered by the pick
	seat    image.Point
	shown   bool
	n       int // cells shown (0 when hidden)
	dwell   int // frames the cursor has rested on a cluster
	grace   int // frames it has been off both column and cluster
}

// stackCell is one cell: the entity as the world draws it, in its own image
// (never a slot in the shared icon atlas), plus its hover tooltip.
type stackCell struct {
	g   *widget.Graphic
	img *ebiten.Image
	tip widgets.RowTip
	art string // the entity the art was drawn for: re-drawn only when it changes
	key string // the tooltip state key: re-set only when it moves
}

// newStackPicker builds the chrome column only: the panel and its twelve-cell
// pool come on the first open (see build), so a session that never hovers a
// cluster never spends an image on it.
func newStackPicker(k *Kit, root *widget.Container) *stackPicker {
	p := &stackPicker{k: k, root: root}
	p.col = widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout()))
	return p
}

// build makes the panel and its cell pool on the first open — the overview
// filter's rule: a boot-time build would rasterize into the shared icon atlas
// and shift the title phase's text anti-aliasing (a capture baseline no
// feature should move).
func (p *stackPicker) build() {
	if p.panel != nil {
		return
	}
	pal := p.k.th.Pal()
	// The picker draws the world's own art, so it keeps a renderer of its
	// own: the kit has no handle on the world's, and every sprite it needs
	// (procedural art, drop-in hulls and rocks) is package-level state.
	p.ren = render.New(p.k.Content)
	p.panel = widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.PanelBox(pal, theme.Solid(pal.BG), theme.NRGBA(pal.Border, 160))),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(stackGap)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(stackCellPad))),
		)),
	)
	for i := range p.cells {
		c := &p.cells[i]
		size := theme.Px(stackCellPx)
		c.img = ebiten.NewImage(size, size)
		c.g = widget.NewGraphic(
			widget.GraphicOpts.Image(c.img),
			widget.GraphicOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Position: widget.RowLayoutPositionCenter,
			})),
		)
		c.g.GetWidget().MouseButtonClickedEvent.AddHandler(func(args any) {
			if a, ok := args.(*widget.WidgetMouseButtonClickedEventArgs); ok {
				p.click(i, a)
			}
		})
		p.panel.AddChild(c.g)
		theme.ShowWidget(c.g.GetWidget(), false)
	}
	theme.ShowWidget(p.panel.GetWidget(), false)
	p.col.AddChild(p.panel)
}

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
	theme.ShowWidget(p.panel.GetWidget(), true)
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
	if p.panel != nil {
		theme.ShowWidget(p.panel.GetWidget(), false)
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

// owns reports whether pt sits on the open column.
func (p *stackPicker) owns(pt image.Point) bool {
	return p.shown && hitRect(p.col.GetWidget(), pt)
}

// click runs one cell's press: right opens the entity's menu (the same spec
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

// sync refreshes the cells from the entries: the entity drawn as the world
// draws it (its body art in its bracket, re-drawn only when the entry moves),
// the hover tooltip naming the entity and its distance, and the column's
// geometry (it is exactly as tall as its cells).
func (p *stackPicker) sync() {
	if !p.shown {
		return
	}
	n := min(len(p.entries), stackCap)
	for i := range p.cells {
		c := &p.cells[i]
		theme.ShowWidget(c.g.GetWidget(), i < n)
		if i >= n {
			continue
		}
		inf := p.entries[i]
		// The art is the entity's own (drawn once per entry: a moving cluster
		// must not re-render twelve miniatures a frame), the tooltip follows
		// its distance.
		art := fmt.Sprintf("%d/%s", inf.Entity, inf.Kind)
		if art != c.art {
			c.art = art
			c.img.Clear()
			p.ren.DrawEntityBracket(c.img, p.k.Sim, inf)
		}
		key := fmt.Sprintf("%s/%s/%t/%s", art, inf.Name, inf.Hostile,
			theme.FormatDistance(inf.Distance))
		if key != c.key {
			c.key = key
			glyph := theme.KindIcon(inf.Kind)
			tint := render.BracketColor(inf.Kind, inf.Hostile)
			if inf.Entity == p.k.cv.Player() {
				tint = p.k.th.Pal().Player
			}
			c.tip.Set(p.k.th, c.g.GetWidget(), key, func() widgets.TooltipModel {
				return widgets.TooltipModel{
					Icon: glyph, IconTint: tint,
					Title: inf.Name,
					Sub:   theme.FormatDistance(inf.Distance) + " · " + theme.KindLabel(inf.Kind),
				}
			})
		}
	}
	if n != p.n {
		p.n = n
		p.place()
	}
}

// place seats the shown column beside the hover point, flipped left and
// clamped inside the root when the cursor sits near an edge. The seat is the
// one taken when the column opened, so a resting cursor does not drag it.
func (p *stackPicker) place() {
	if !p.shown || p.n == 0 {
		return
	}
	cell, gap, pad := theme.Px(stackCellPx), theme.Px(stackGap), theme.Px(stackCellPad)
	cw := cell + 2*pad
	ch := p.n*cell + (p.n-1)*gap + 2*pad
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
