package screens

// The class-quota table: a hull's per-class grid and cpu allowances as counts
// of filled boxes, one treatment for the ship page's FITTING section and the
// fitting window's HULL LIMITS block. A quota is a capacity, not a level, so
// every box is filled and nothing draws a track; a zero inside a present row
// is a dim dash, never an empty box. The boxes are the proficiency pip
// painter (level == max fills them all), so the two readouts cannot drift.

import (
	"image"
	"slices"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// infoQuota is one hull's quota table: the axes it allows as columns, and one
// row per module class it allows any of.
type infoQuota struct {
	Axes []string // the axes the hull allows, in table order ("Grid", "CPU")
	Rows []infoQuotaRow
}

// infoQuotaRow is one module class's allowance, one count per axis in the
// table's column order (0 = no allowance on that axis: a dim dash).
type infoQuotaRow struct {
	Label  string
	Counts []int
}

// quotaAxisStat is an axis column's census row: the grid and cpu stat rows'
// own glyph and tint (the fitting window's STATS block reads the same two).
var quotaAxisStat = map[string]string{"Grid": "Grid class", "CPU": "CPU class"}

// shipQuota builds a hull's quota table: an axis with no allowance at all is
// no column, a class with no allowance on any axis is no row (so a hull with
// no cpu quota has no CPU column, and an all-zero hull has no table at all).
func shipQuota(grid, cpu content.ClassQuota) *infoQuota {
	type axis struct {
		label string
		q     content.ClassQuota
	}
	q := &infoQuota{}
	var cols []axis
	for _, a := range []axis{{"Grid", grid}, {"CPU", cpu}} {
		if a.q != (content.ClassQuota{}) {
			cols = append(cols, a)
			q.Axes = append(q.Axes, a.label)
		}
	}
	for _, class := range []string{content.ClassI, content.ClassII, content.ClassIII} {
		row := infoQuotaRow{Label: content.ClassLabel(class)}
		for _, a := range cols {
			row.Counts = append(row.Counts, a.q.Count(class))
		}
		if len(row.Counts) > 0 && slices.Max(row.Counts) > 0 {
			q.Rows = append(q.Rows, row)
		}
	}
	if len(q.Rows) == 0 {
		return nil
	}
	return q
}

// quotaTableWidget lays a quota table into one grid, so a box column lines up
// under its header down the whole block: the class label leads its row, and
// each box strip ends on the block's right edge like every other value on
// these two surfaces (the label column takes the stretch, the strips keep
// their natural width).
func quotaTableWidget(env theme.Theme, step string, q infoQuota) *widget.Container {
	face := env.Face(step)
	cols := 1 + len(q.Axes)
	stretch := make([]bool, cols)
	stretch[0] = true
	grid := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(cols),
			widget.GridLayoutOpts.Stretch(stretch, nil),
			widget.GridLayoutOpts.Spacing(theme.Px(theme.SpLg), 0),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	grid.AddChild(widget.NewContainer()) // the label column's blank header cell
	for _, ax := range q.Axes {
		grid.AddChild(quotaCell(quotaHead(env, face, ax), false))
	}
	for _, r := range q.Rows {
		grid.AddChild(quotaCell(widget.NewText(
			widget.TextOpts.Text(r.Label, face, theme.Solid(env.Pal().Dim)),
		), true))
		for _, n := range r.Counts {
			grid.AddChild(quotaCell(quotaBoxes(env, n), false))
		}
	}
	return grid
}

// quotaHead is one axis's column header: its stat glyph over the axis label.
func quotaHead(env theme.Theme, face *text.Face, axis string) *widget.Container {
	pal := env.Pal()
	stat := quotaAxisStat[axis]
	row := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewGridLayout(
		widget.GridLayoutOpts.Columns(2),
		widget.GridLayoutOpts.Spacing(theme.Px(theme.StatIconGap), 0),
	)))
	g := theme.NewIcon(theme.StatIcon(stat), theme.StatIconSize, theme.StatTint(pal, stat))
	g.GetWidget().LayoutData = widget.GridLayoutData{VerticalPosition: widget.GridLayoutPositionCenter}
	head := widget.NewText(widget.TextOpts.Text(axis, face, theme.Solid(pal.Head)))
	head.GetWidget().LayoutData = widget.GridLayoutData{VerticalPosition: widget.GridLayoutPositionCenter}
	row.AddChild(g, head)
	return row
}

// quotaCell anchors one cell's content inside its grid column: the box strips
// and their headers end on the table's right edge, and the class keys start in
// the glyph gutter the block's rows above them use, so a key lines up with
// them instead of hanging at the container's edge.
func quotaCell(child widget.PreferredSizeLocateableWidget, key bool) *widget.Container {
	data := widget.AnchorLayoutData{
		HorizontalPosition: widget.AnchorLayoutPositionEnd,
		VerticalPosition:   widget.AnchorLayoutPositionCenter,
	}
	if key {
		data.HorizontalPosition = widget.AnchorLayoutPositionStart
		data.Padding = &widget.Insets{Left: theme.Px(theme.StatIconSize + theme.StatIconGap)}
	}
	child.GetWidget().LayoutData = data
	ctr := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout()))
	ctr.AddChild(child)
	return ctr
}

// quotaBoxes is one cell's strip: n filled boxes, or the dim dash an empty
// cell inside a present row draws (the border tone the pip painter leaves an
// unfilled box in, one box wide).
func quotaBoxes(env theme.Theme, n int) *widget.Graphic {
	pal := env.Pal()
	size, step := theme.Px(profPipW), theme.Px(profPipW+profPipGap)
	if n == 0 {
		h := max(1, theme.Px(1))
		img := ebiten.NewImage(size, h)
		theme.FillRect(img, image.Rect(0, 0, size, h), theme.NRGBA(pal.Border, 200))
		return widget.NewGraphic(widget.GraphicOpts.Image(img))
	}
	img := ebiten.NewImage(profPipRowW(n, step, size), size)
	drawProfPips(img, 0, 0, n, n, step, size, false, pal)
	return widget.NewGraphic(widget.GraphicOpts.Image(img))
}
