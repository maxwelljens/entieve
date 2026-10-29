package screens

import (
	"image"
	"image/color"
	"math"
	"strings"

	eimage "github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/hulls"
	"github.com/entieve/entieve/uikit/icons"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// Fitting geometry, in design units through the Px funnel. The window is
// the EVE fitting shape: an identity/stat column on the left, the hull's
// slot grid over the hangar's split on the right, and the derived fitting
// summary (DPS, capacitor verdict) as a strip under both. The left column
// runs 300 px, wide enough for the two-column stat rows; the tiles are the
// HUD rack's 62×62 cells (a local copy — the rack's own metrics are its
// business) with a StepTiny name line under each.
const (
	fitColW       = 300 + theme.ScrollGutter // identity + stats column, its scroll rail included
	fitPortrait   = 120                      // hull portrait box, hudPortrait's twin
	fitTileW      = 62                       // slot tile width (rackCellW's twin: IconTile + 6)
	fitTileH      = 62                       // slot tile height (theme.IconTile + 6)
	fitTileNameH  = 19                       // name line under a filled tile (StepTiny line box)
	fitTileLabelW = 30                       // "High"/"Mid"/"Low" label column
	fitTileGap    = 4                        // gap between tiles
	fitMaxCells   = 4                        // widest slot row across the hull roster
	fitRowGap     = 4                        // gap between the class rows (the frozen window height)
	// fitIconPad centres a tile glyph: ebitenui lays a graphic button out
	// as [empty text][10 px spacing][glyph], so paddingRight =
	// paddingLeft + spacing is the centring condition (rackIconPad's twin).
	fitIconPad = 20
	// fitSlotShare is the slots pane's opening share of the split: the
	// 62 px tile rows do not fit the equal half at the window's frozen
	// height (682 px), and the grid would draw over the hangar's header.
	fitSlotShare = 0.6
	fitBarW      = 160 // capacitor verdict bar width
	fitBarH      = 8   // capacitor verdict bar height
	// capacitor endurance integration: the mirror of the sim's recharge
	// curve, stepped at a fixed second; past the horizon an unstable cap
	// reads as the horizon (a fit that lasts an hour is stable in play).
	fitCapStep    = 1.0
	fitCapHorizon = 3600.0
)

// fitClasses and fitRowLabels index the three slot rows (the grid's shape).
var (
	fitClasses   = [3]string{content.SlotHigh, content.SlotMid, content.SlotLow}
	fitRowLabels = [3]string{"High", "Mid", "Low"}
)

// fitStatLabels is the stats panel's row order (two per line). The values
// are refreshed by statValues; a read the ship does not have stays blank.
var fitStatLabels = []string{
	"Shield", "Armor", "Structure", "Capacitor", "Recharge", "Speed",
	"Align", "Signature", "Warp", "Targets", "Scan", "Cargo", "Ore hold",
}

// fitEHPLabels is the per-type EHP row order.
var fitEHPLabels = []string{"EM", "Thermal", "Kinetic", "Explosive"}

// Fitting is the ship fitting window: the hull's identity block (portrait,
// class · maker, emblem), the derived stats with a per-type EHP row, the
// high/mid/low slot tile grids, the fitting summary (predicted DPS and the
// capacitor verdict) and the hangar's modules and ships with the
// Unfit/Fit/Assemble actions.
type Fitting struct {
	th  theme.Theme
	cv  widgets.ContentView
	as  widgets.ActionSink
	ptr *widgets.Pointer
	win *widget.Window

	measure func(string) float64 // the StepTiny face, for tile-name ellipsis

	title *theme.ChromeText // the static title bar label

	// Zone 1 — ship identity.
	portrait      *ebiten.Image
	portraitGfx   *widget.Graphic
	portraitHull  content.ShipID
	portraitDraws int // portrait repaints (hull changes only)
	name          *widget.Text
	idRow         *widget.Container // emblem (when the hull has a maker) + class · maker
	idText        *widget.Text
	emblemSlug    string

	// Zone 2 — the structured stats, the per-type EHP rows and the hull's
	// fitting limits.
	stats  []fitRow
	ehp    []fitRow
	limits []fitRow
	// fitDefs is this frame's fitted module defs (the FitProblem input).
	fitDefs []*content.ModuleDef

	// Zone 3 — the slot tile grid and its interactions.
	tiles   [3][fitMaxCells]*fitTile
	slotDbl widgets.RowDouble

	// Zone 4 — the fitting summary.
	sum       []fitRow
	verdict   *widget.Text
	capBar    *ebiten.Image
	capBarGfx *widget.Graphic
	capFrac   float64
	capLit    bool
	capStable bool

	hangar   *widgets.Table
	split    *widgets.SplitPane
	left     *widget.ScrollContainer
	leftRail *widgets.ScrollRail

	unfit    *widget.Button
	fitMod   *widget.Button
	assemble *widget.Button

	selSlot widgets.SlotPick
	hasSlot bool
	selMod  content.ModuleID
	hasMod  bool
	selShip content.ShipID
	hasShip bool
}

// fitRow is one dense label/value line: the label on the left, the value
// right-anchored on the row's shared edge. Both are live cells (the EHP
// labels carry the resist they price in).
type fitRow struct {
	icon  *widget.Graphic // the row's stat glyph (nil on an iconless row)
	label *widget.Text
	value *widget.Text
}

// fitTile is one slot tile: a 62×62 tile carrying the module's class glyph
// (a dashed placeholder while empty) with the short name under it, its
// declared interactions (RowSpec) and its structured tooltip slot.
type fitTile struct {
	root  *widget.Container
	btn   *widget.Button
	name  *widget.Text
	slot  sim.SlotRef
	mod   content.ModuleID
	icon  string
	empty bool // the tile's chrome at the last Sync
	tip   widgets.RowTip
	spec  widgets.RowSpec
}

// NewFitting builds the Fitting window over the shared environment.
// Mount it on a Kit with Kit.AddWindow and call Sync every frame.
func NewFitting(env widgets.Seams) *Fitting {
	f := &Fitting{th: env, cv: env, as: env, ptr: env.Pointer()}
	p := env.Pal()
	faceTiny := env.Face(theme.StepTiny)
	f.measure = func(s string) float64 {
		w, _ := text.Measure(s, *faceTiny, 0)
		return w
	}

	// Zone 1 + 2: the identity block and the stats panel share the left
	// column, stacked identity-first (the portrait is the window's anchor).
	left := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.SpSm)),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(fitColW), 0)),
	)
	left.AddChild(f.newIdentity(), f.newRule(p), f.newStats(), f.newEHP(), f.newLimits())

	// The stats column scrolls (its own rail, the info page's pattern): a
	// window dragged shorter than the design height clips the tail behind a
	// scrollbar instead of letting EHP and the limits collide with the
	// summary strip.
	f.left = widget.NewScrollContainer(
		widget.ScrollContainerOpts.Content(left),
		widget.ScrollContainerOpts.StretchContentWidth(),
		widget.ScrollContainerOpts.Image(&widget.ScrollContainerImage{
			// No fill of its own (the window body already paints): the
			// scroll container is here to clip and scroll, not to restyle.
			Idle:     theme.NineSlice(color.NRGBA{}),
			Disabled: theme.NineSlice(color.NRGBA{}),
			Mask:     theme.NineSlice(color.NRGBA{A: 255}),
		}),
	)
	widgets.AttachScrollContent(f.left, left)
	f.leftRail = widgets.NewScrollRail(env, f.left, f.left, left)
	leftCol := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(fitColW), 0)),
	)
	leftCol.AddChild(f.leftRail.Container())

	// Zone 3 over the hangar: the split's two panes, the grid above the
	// hangar table, still the "fitting-hangar" seam the sidecar stores.
	f.hangar = widgets.NewTable(env, []widgets.Column{
		{Key: "Item", Title: "Hangar", Priority: 1},
		{Key: "Qty", Title: "Qty", Priority: 0, AlignRight: true},
	}, widgets.PrefsID("hangar"))
	f.hangar.OnSelect(f.selectHangar)
	// Double-click fits a hangar module into the first empty slot of its
	// class (the row menu's Fit-into-selected-slot stays the fallback);
	// a stored hull keeps Show Info.
	f.hangar.OnDouble(f.hangarDouble)

	label := func(s string) *widget.Text {
		return widget.NewText(
			widget.TextOpts.Text(s, env.Face(theme.StepTiny), theme.Solid(p.Accent)),
		)
	}
	grid := f.newGrid()
	f.split = widgets.NewSplitPane(env,
		paneStack([]bool{false, false}, label("Slots"), grid),
		paneStack([]bool{false, true}, label("Hangar"), f.hangar.Container()),
	)
	f.split.SetDefaultRatio(0, fitSlotShare)
	// The body's main row: the fixed identity/stat column beside the split
	// (the grid layout gives the left column its width and the split the
	// rest — a RowLayout stretches only across its axis).
	main := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Stretch([]bool{false, true}, []bool{true}),
			widget.GridLayoutOpts.Spacing(theme.Px(theme.SpMd), 0),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	main.AddChild(leftCol, f.split.Container())

	newBtn := func(label string, clicked func()) *widget.Button {
		return widget.NewButton(
			widget.ButtonOpts.TextLabel(label),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(0, theme.Px(40)),
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
			widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) { clicked() }),
		)
	}
	f.unfit = newBtn("Unfit slot", f.unfitClicked)
	f.fitMod = newBtn("Fit module", f.fitClicked)
	f.assemble = newBtn("Assemble", f.assembleClicked)
	buttons := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.ContentsPad)),
		)),
	)
	buttons.AddChild(f.unfit, f.fitMod, f.assemble)

	// Zone 4: the fitting summary strip across the window's foot — the
	// three derived figures over the capacitor verdict and its bar.
	summary := f.newSummary()

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.WindowBodyBox(p, theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200))),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true, false, false}),
			widget.GridLayoutOpts.Spacing(0, theme.Px(4)),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	body.AddChild(main, summary, buttons)

	title, fitTitle := theme.NewTitleBar(env, "Fitting", "svc_fitting")
	f.title = fitTitle

	f.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, theme.Px(theme.TitleBarH)),
		// Content floor: the left column's identity + two-column stats and
		// the slot grid beside it, the summary strip and the buttons.
		widget.WindowOpts.MinSize(theme.Px(580), theme.Px(660)),
		// The landing rect the game's Fitting Screen row also declares
		// (FittingRect), so it opens here on first use, docked and in
		// space alike.
		widget.WindowOpts.Location(pxRect(FittingRect())),
	)
	return f
}

// FittingRect is the fitting window's landing rect in design units: the
// one number its Screen row's DefaultRect and its constructor share.
func FittingRect() image.Rectangle { return image.Rect(370, 8, 1014, 690) }

// Win returns the fitting window for Kit.AddWindow.
func (f *Fitting) Win() *widget.Window { return f.win }

// Splits names the window's draggable seam for the layout sidecar (the
// hangar pane below it).
func (f *Fitting) Splits() []widgets.Seam {
	return []widgets.Seam{{Name: "fitting-hangar", Split: f.split, Index: 0}}
}

// Tables are the window's tables for the layout sidecar (the hangar
// list's column widths).
func (f *Fitting) Tables() []*widgets.Table { return []*widgets.Table{f.hangar} }

// newIdentity builds zone 1: the framed hull portrait, the hull's name and
// its class · maker line with the corporation's emblem.
func (f *Fitting) newIdentity() *widget.Container {
	p := f.th.Pal()
	f.portrait = ebiten.NewImage(theme.Px(fitPortrait), theme.Px(fitPortrait))
	f.portraitGfx = widget.NewGraphic(
		widget.GraphicOpts.Image(f.portrait),
		widget.GraphicOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(theme.Px(fitPortrait), theme.Px(fitPortrait)),
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				HorizontalPosition: widget.AnchorLayoutPositionCenter,
				VerticalPosition:   widget.AnchorLayoutPositionCenter,
			}),
		),
	)
	frame := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.WindowTitleBox(theme.Solid(p.BGHard), theme.NRGBA(p.Border, 200))),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(theme.Px(fitPortrait)+2, theme.Px(fitPortrait)+2),
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Position: widget.RowLayoutPositionCenter}),
		),
	)
	// A graphic needs a layout datum inside the anchor frame; the frame
	// itself centres in the column through the row's position datum.
	frame.AddChild(f.portraitGfx)

	f.name = widget.NewText(
		widget.TextOpts.Text("", f.th.FaceBold(theme.StepSmall), theme.Solid(p.Text)),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	f.idText = widget.NewText(
		widget.TextOpts.Text("", f.th.Face(theme.StepSmall), theme.Solid(p.Dim)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
			Position: widget.RowLayoutPositionCenter,
		})),
	)
	f.idRow = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.IconGap)),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
			Position: widget.RowLayoutPositionCenter,
		})),
	)
	f.idRow.AddChild(f.idText)

	col := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.SpSm)),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	col.AddChild(frame, f.name, f.idRow)
	return col
}

// paintPortrait repaints the hull portrait: the hull's raster scaled into
// the design-square box (the same machinery as the HUD's portrait; Sync
// calls it on a hull change only).
func (f *Fitting) paintPortrait(hull content.ShipID) {
	f.portraitDraws++
	f.portrait.Clear()
	src := hulls.Get(hull, theme.Px(fitPortrait))
	if src == nil {
		return
	}
	b := src.Bounds()
	box := float64(theme.Px(fitPortrait))
	scale := math.Min(box/float64(b.Dx()), box/float64(b.Dy()))
	var opts ebiten.DrawImageOptions
	opts.GeoM.Scale(scale, scale)
	opts.GeoM.Translate((box-float64(b.Dx())*scale)/2, (box-float64(b.Dy())*scale)/2)
	f.portrait.DrawImage(ebiten.NewImageFromImage(src), &opts)
}

// newRule is the hairline under the identity block (the tooltip's rule).
func (f *Fitting) newRule(p theme.Palette) *widget.Container {
	return widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.NineSlice(theme.NRGBA(p.Accent, widgets.TipRuleA))),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			widget.WidgetOpts.MinSize(0, theme.Px(widgets.TipRuleH)),
		),
	)
}

// newStats builds zone 2's stat rows, two label/value lines per row.
func (f *Fitting) newStats() *widget.Container {
	col := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	col.AddChild(f.caption("STATS", f.th.Pal()))
	col.AddChild(f.newPairs(fitStatLabels, &f.stats, 2))
	return col
}

// newEHP builds the per-type EHP rows: each label carries the resistance
// the figure is priced against.
func (f *Fitting) newEHP() *widget.Container {
	col := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.SpSm)),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	col.AddChild(f.caption("EHP", f.th.Pal()))
	col.AddChild(f.newPairs(fitEHPLabels, &f.ehp, 2))
	return col
}

// newLimits builds the HULL LIMITS block: the hull's mounts and its per-class
// grid/CPU allowances, in the words the info page and the sim's refusals use.
func (f *Fitting) newLimits() *widget.Container {
	// No gap under the caption: the design height is exactly full, and this
	// block is the last one in the column.
	col := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	col.AddChild(f.caption("HULL LIMITS", f.th.Pal()))
	// One row per limit: a mount list and a class-quota pair are both too
	// wide for a 300 px column's half.
	col.AddChild(f.newPairs([]string{"Mounts", "Grid quotas", "CPU quotas"}, &f.limits, 1))
	return col
}

// caption is a section label in the info window's head style.
func (f *Fitting) caption(s string, p theme.Palette) *widget.Text {
	return widget.NewText(
		widget.TextOpts.Text(s, f.th.Face(theme.StepSmall), theme.Solid(p.Head)),
	)
}

// newPairs lays labels out cols per line (equal shares of the width) and
// records each row's cells.
func (f *Fitting) newPairs(labels []string, rows *[]fitRow, cols int) *widget.Container {
	col := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	stretch := make([]bool, cols)
	for i := range stretch {
		stretch[i] = true
	}
	for i := 0; i < len(labels); i += cols {
		line := widget.NewContainer(
			widget.ContainerOpts.Layout(widget.NewGridLayout(
				widget.GridLayoutOpts.Columns(cols),
				widget.GridLayoutOpts.Stretch(stretch, nil),
				widget.GridLayoutOpts.Spacing(theme.Px(theme.SpLg), 0),
			)),
			widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
		)
		for j := i; j < i+cols && j < len(labels); j++ {
			line.AddChild(f.newRow(labels[j], rows))
		}
		col.AddChild(line)
	}
	return col
}

// newRow is one dense label/value line (the house stat row): StepTiny
// labels, right-anchored values on the row's shared edge.
func (f *Fitting) newRow(label string, rows *[]fitRow) *widget.Container {
	p := f.th.Pal()
	face := f.th.Face(theme.StepTiny)
	icon := theme.StatIcon(label)
	var mark *widget.Graphic
	labelData := widget.AnchorLayoutData{
		HorizontalPosition: widget.AnchorLayoutPositionStart,
		VerticalPosition:   widget.AnchorLayoutPositionCenter,
	}
	if icon != "" {
		mark = theme.NewIcon(icon, theme.StatIconSize, theme.StatTint(p, label))
		mark.GetWidget().LayoutData = widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionStart,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
		}
		labelData.Padding = &widget.Insets{Left: theme.Px(theme.StatIconSize + theme.StatIconGap)}
	}
	row := fitRow{
		icon: mark,
		label: widget.NewText(
			widget.TextOpts.Text(label, face, theme.Solid(p.Dim)),
			widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
			widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(labelData)),
		),
		value: widget.NewText(
			widget.TextOpts.Text("", face, theme.Solid(p.Text)),
			widget.TextOpts.Position(widget.TextPositionEnd, widget.TextPositionCenter),
			widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				HorizontalPosition: widget.AnchorLayoutPositionEnd,
				VerticalPosition:   widget.AnchorLayoutPositionCenter,
			})),
		),
	}
	*rows = append(*rows, row)
	ctr := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			widget.WidgetOpts.MinSize(0, widgets.TipLineH(face)),
		),
	)
	if row.icon != nil {
		ctr.AddChild(row.icon)
	}
	ctr.AddChild(row.label, row.value)
	return ctr
}

// newGrid builds zone 3: one row per slot class, a class label and the
// hull's cells — the HUD rack's visual language, drawn locally.
func (f *Fitting) newGrid() *widget.Container {
	p := f.th.Pal()
	box := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewRowLayout(
		widget.RowLayoutOpts.Direction(widget.DirectionVertical),
		widget.RowLayoutOpts.Spacing(theme.Px(fitRowGap)),
	)))
	for row, class := range fitClasses {
		line := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(fitTileGap)),
		)))
		line.AddChild(widget.NewText(
			widget.TextOpts.Text(fitRowLabels[row], f.th.Face(theme.StepTiny), theme.Solid(p.Head)),
			widget.TextOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(fitTileLabelW), 0)),
		))
		for idx := 0; idx < fitMaxCells; idx++ {
			t := f.newTile(class, idx)
			f.tiles[row][idx] = t
			line.AddChild(t.root)
		}
		box.AddChild(line)
	}
	return box
}

// newTile builds one persistent slot tile: the tile chrome, the class
// glyph and the name line under it. Clicks select the slot (the toolbar's
// Fit/Unfit gate reads it), right-click opens the row's menu and a
// double-click on a filled tile shows the module's info.
func (f *Fitting) newTile(class string, idx int) *fitTile {
	p := f.th.Pal()
	t := &fitTile{slot: sim.SlotRef{Class: class, Index: idx}}
	t.btn = widget.NewButton(
		widget.ButtonOpts.Image(fitTileImages(p, true)),
		widget.ButtonOpts.Graphic(&widget.GraphicImage{
			Idle:     fitGlyph("", color.RGBA{}),
			Disabled: fitGlyph("", color.RGBA{}),
		}),
		widget.ButtonOpts.GraphicPadding(widget.Insets{Right: theme.Px(fitIconPad)}),
		widget.ButtonOpts.TextPadding(&widget.Insets{
			Left: theme.Px(3), Right: theme.Px(3), Top: theme.Px(2), Bottom: theme.Px(2),
		}),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(theme.Px(fitTileW), theme.Px(fitTileH)),
			widget.WidgetOpts.MouseButtonClickedHandler(func(a *widget.WidgetMouseButtonClickedEventArgs) {
				switch a.Button {
				case ebiten.MouseButtonLeft:
					f.selSlot = widgets.SlotPick{Ref: t.slot, Mod: t.mod}
					f.hasSlot = true
					f.slotDbl.Click(f.cv, f.as, t.spec)
				case ebiten.MouseButtonRight:
					widgets.OpenRowMenu(f.cv, f.as, t.spec, widgets.ClickPoint(a))
				}
			}),
		),
	)
	t.name = widget.NewText(
		widget.TextOpts.Text("", f.th.Face(theme.StepTiny), theme.Solid(p.Dim)),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.MinSize(0, theme.Px(fitTileNameH))),
	)
	// The tile icon makes the button's inner row wider than the cell; the
	// cap keeps the cell geometry (the row's transparent padding overflows,
	// the glyph stays centred inside the tile).
	t.btn.GetWidget().LayoutData = widget.RowLayoutData{MaxWidth: theme.Px(fitTileW)}
	t.root = widget.NewContainer(widget.ContainerOpts.Layout(widget.NewRowLayout(
		widget.RowLayoutOpts.Direction(widget.DirectionVertical),
	)))
	t.root.AddChild(t.btn, t.name)
	return t
}

// fitGlyph resolves one tile image: the module's icon (art, else its class
// glyph) in the given tint ("" = an empty slot, which gets a transparent box
// of the same size so no cell ever changes size when a module is fitted).
func fitGlyph(name string, tint color.RGBA) *ebiten.Image {
	if name == "" {
		return icons.Get(icons.Fallback, theme.Px(theme.IconTile), color.RGBA{})
	}
	return icons.Get(name, theme.Px(theme.IconTile), tint)
}

// fitTileImages builds the slot tile chrome: the cell recipe's accent glow
// for a fitted module, and the dashed outline of an empty slot in every
// state (one per hull slot, so the grid always shows the hull's shape).
func fitTileImages(p theme.Palette, empty bool) *widget.ButtonImage {
	if empty {
		dash := fitDashTile(p)
		return &widget.ButtonImage{Idle: dash, Hover: dash, Pressed: dash, PressedHover: dash, Disabled: dash}
	}
	return theme.CellImage(p, true)
}

// fitDashTile draws the empty-slot placeholder: a dashed outline in the
// border colour, the grid's "nothing here, yet" cell.
func fitDashTile(p theme.Palette) *eimage.NineSlice {
	return eimage.NewNineSliceBorder(fitDashImage(p), 1)
}

// fitDashImage is the placeholder's raster: dashes along every edge, a
// hollow centre (the test reads its pixels).
func fitDashImage(p theme.Palette) *ebiten.Image {
	w, h := theme.Px(fitTileW), theme.Px(fitTileH)
	img := ebiten.NewImage(w, h)
	dash, gap := theme.Px(4), theme.Px(3)
	col := theme.NRGBA(p.Head, 220)
	for x := 0; x < w; x += dash + gap {
		x2 := min(x+dash, w)
		theme.FillRect(img, image.Rect(x, 0, x2, 1), col)
		theme.FillRect(img, image.Rect(x, h-1, x2, h), col)
	}
	for y := 0; y < h; y += dash + gap {
		y2 := min(y+dash, h)
		theme.FillRect(img, image.Rect(0, y, 1, y2), col)
		theme.FillRect(img, image.Rect(w-1, y, w, y2), col)
	}
	return img
}

// shortName is the label drawn under a tile: the module's first word,
// ellipsized to the tile (empty slots draw nothing).
func (f *Fitting) shortName(def *content.ModuleDef) string {
	if def == nil {
		return ""
	}
	name := def.Name
	if i := strings.IndexByte(name, ' '); i > 0 {
		name = name[:i]
	}
	return widgets.Ellipsize(name, float64(theme.Px(fitTileW-6)), f.measure)
}

// newSummary builds zone 4: the predicted DPS, the capacitor drain against
// the recharge curve's peak, and the verdict with its bar.
func (f *Fitting) newSummary() *widget.Container {
	p := f.th.Pal()
	pairs := f.newPairs([]string{"Total DPS", "Cap drain", "Cap recharge"}, &f.sum, 3)

	// The verdict line: the registered interface string on the left, the
	// drain-vs-recharge bar on the right (filled at drain/peak, capped).
	f.verdict = widget.NewText(
		widget.TextOpts.Text("", f.th.Face(theme.StepTiny), theme.Solid(p.Text)),
		widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionStart,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
		})),
	)
	f.capBar = ebiten.NewImage(theme.Px(fitBarW), theme.Px(fitBarH))
	f.capBarGfx = widget.NewGraphic(
		widget.GraphicOpts.Image(f.capBar),
		widget.GraphicOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(theme.Px(fitBarW), theme.Px(fitBarH)),
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				HorizontalPosition: widget.AnchorLayoutPositionEnd,
				VerticalPosition:   widget.AnchorLayoutPositionCenter,
			}),
		),
	)
	verdictRow := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			widget.WidgetOpts.MinSize(0, widgets.TipLineH(f.th.Face(theme.StepTiny))),
		),
	)
	verdictRow.AddChild(f.verdict, f.capBarGfx)

	strip := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.SpSm)),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	strip.AddChild(f.caption("FITTING SUMMARY", p), pairs, verdictRow)
	return strip
}

// Sync refreshes the identity block, the stats and EHP rows, the slot
// tiles, the fitting summary, the hangar rows and the action gating. Call
// it every frame before Kit.Update.
func (f *Fitting) Sync() {
	f.split.Poll(f.ptr)
	s := f.cv
	player := s.Player()
	f.fitDefs = nil
	if fit, ok := s.Fitting(player); ok {
		for _, class := range fitClasses {
			for _, id := range fit.Defs(class) {
				if def, ok := f.cv.Defs().Module(id); ok {
					f.fitDefs = append(f.fitDefs, def)
				}
			}
		}
	}

	ship, hasShip := s.ShipState(player)
	f.syncIdentity(ship, hasShip)
	f.syncStats(player, ship, hasShip)
	f.syncTiles(ship, hasShip)
	f.syncSummary(player, ship, hasShip)

	f.hangar.SetRows(f.hangarRows())
	f.hangar.Sync()

	// Action gating: unfit needs a fitted slot, fit needs a hangar
	// module plus an empty slot of its class, assemble needs a ship —
	// the same fitReady/unequipReady rules the row menus read.
	f.unfit.GetWidget().Disabled = !f.hasSlot || !unequipReady(f.selSlot)
	f.fitMod.GetWidget().Disabled = !f.hasMod || !f.fitReady(f.selMod, f.selSlot)
	f.assemble.GetWidget().Disabled = !f.hasShip
	f.leftRail.Sync()
}

// syncIdentity refreshes the title, the portrait (repainted on a hull
// change only) and the class · maker line with the maker's corporation mark.
func (f *Fitting) syncIdentity(ship sim.ShipState, hasShip bool) {
	hull, maker := content.ShipID(""), ""
	name, sub := "No ship", ""
	if hasShip {
		hull = ship.Hull
		if def, ok := f.cv.Defs().Ship(hull); ok {
			name = def.Name
			maker = def.Maker
			sub = makerSub(def.Class, def.Maker)
		}
	}
	// The title is the base display name only (a window title is
	// furniture): the hull name and class are the body's own heads.
	f.title.Label = "Fitting"
	f.name.Label = name
	f.idText.Label = sub

	if hull != f.portraitHull {
		f.portraitHull = hull
		f.paintPortrait(hull)
	}
	if maker != f.emblemSlug {
		f.emblemSlug = maker
		f.idRow.RemoveChildren()
		if maker != "" {
			if g := corpMark(maker, theme.IconCorp, f.th.Pal().Text); g != nil {
				g.GetWidget().LayoutData = widget.RowLayoutData{Position: widget.RowLayoutPositionCenter}
				f.idRow.AddChild(g)
			}
		}
		f.idRow.AddChild(f.idText)
	}
}

// statValues is the stats panel's display math: the hull's derived maxima
// and the ship's live performance, formatted for the label/value rows.
func (f *Fitting) statValues(player ecs.Entity, ship sim.ShipState, hasShip bool) map[string]string {
	if !hasShip {
		return nil
	}
	s := f.cv
	v := map[string]string{
		"Shield":    theme.FormatInt(int64(ship.MaxShield)),
		"Armor":     theme.FormatInt(int64(ship.MaxArmor)),
		"Structure": theme.FormatInt(int64(ship.MaxStructure)),
	}
	if cap, ok := s.Capacitor(player); ok {
		v["Capacitor"] = theme.GroupFloat(cap.Capacity, 0) + " GJ"
		v["Recharge"] = theme.GroupFloat(cap.Tau, 1) + " s"
	}
	if m, ok := s.Motion(player); ok {
		v["Speed"] = theme.FormatSpeed(m.MaxSpeed)
		v["Align"] = theme.GroupFloat(m.Agility*10, 1) + " s"
		v["Signature"] = theme.GroupFloat(m.SigRadius, 0) + " m"
	}
	if cg, ok := s.Cargo(player); ok {
		v["Cargo"] = theme.FormatVolumeShort(cg.Capacity)
		v["Ore hold"] = theme.FormatVolumeShort(cg.OreCapacity)
	}
	if tg, ok := s.Targeting(player); ok {
		v["Targets"] = theme.FormatInt(int64(tg.MaxTargets))
		v["Scan"] = theme.GroupFloat(tg.ScanRes, 0)
	}
	if def, ok := f.cv.Defs().Ship(ship.Hull); ok {
		v["Warp"] = theme.GroupFloat(def.WarpSpeed, 1) + " AU/s"
		v["Mounts"] = mountsText(def.Mounts)
		v["Grid quotas"] = quotaText(def.GridClasses)
		v["CPU quotas"] = quotaText(def.CPUClasses)
	}
	return v
}

// syncStats refreshes the stat rows and the per-type EHP rows (each EHP
// label names the resist it prices in).
func (f *Fitting) syncStats(player ecs.Entity, ship sim.ShipState, hasShip bool) {
	vals := f.statValues(player, ship, hasShip)
	for _, r := range f.stats {
		r.value.Label = vals[r.label.Label]
	}
	for _, r := range f.limits {
		r.value.Label = vals[r.label.Label]
	}
	for i, t := range []content.DamageType{
		content.DmgEM, content.DmgThermal, content.DmgKinetic, content.DmgExplosive,
	} {
		r := f.ehp[i]
		if !hasShip {
			r.label.Label = fitEHPLabels[i]
			r.value.Label = ""
			continue
		}
		r.label.Label = fitEHPLabels[i] + " " +
			theme.FormatPct(ship.Resists.Of(t))
		r.value.Label = theme.FormatInt(int64(fitEHP(ship, t) + 0.5))
	}
}

// syncTiles rewrites the slot grid from the hull's slot counts and the
// fitting: one tile per slot the hull has (empty ones dashed), the
// smallest tile count covering any over-fitted class.
func (f *Fitting) syncTiles(ship sim.ShipState, hasShip bool) {
	var slots content.Slots
	if hasShip {
		if def, ok := f.cv.Defs().Ship(ship.Hull); ok {
			slots = def.Slots
		}
	}
	fit, hasFit := f.cv.Fitting(f.cv.Player())
	want := [3]int{slots.High, slots.Mid, slots.Low}
	filled := [3]int{}
	if hasFit {
		filled = [3]int{len(fit.High), len(fit.Mid), len(fit.Low)}
	}
	for row := 0; row < 3; row++ {
		count := max(want[row], filled[row])
		if count > fitMaxCells {
			count = fitMaxCells
		}
		for idx, t := range f.tiles[row] {
			theme.ShowWidget(t.root.GetWidget(), idx < count)
			if idx >= count {
				continue
			}
			var id content.ModuleID
			if hasFit {
				if defs := fit.Defs(fitClasses[row]); idx < len(defs) {
					id = defs[idx]
				}
			}
			f.syncTile(t, id)
		}
	}
}

// syncTile rewrites one tile's chrome, glyph, name and interactions.
func (f *Fitting) syncTile(t *fitTile, id content.ModuleID) {
	var def *content.ModuleDef
	if id != "" {
		def, _ = f.cv.Defs().Module(id)
	}
	icon := ""
	if def != nil {
		icon = theme.ModuleIconFor(def)
	}
	t.mod = id
	if icon != t.icon || (def == nil) != t.empty {
		t.icon, t.empty = icon, def == nil
		t.btn.SetImage(fitTileImages(f.th.Pal(), t.empty))
		t.btn.SetGraphicImage(&widget.GraphicImage{
			Idle:     fitGlyph(icon, f.th.Pal().Text),
			Disabled: fitGlyph(icon, iconColor(f.th.Pal().Dim, theme.IconDisabledA)),
		})
	}
	t.name.Label = f.shortName(def)
	t.spec = f.slotSpec(widgets.SlotPick{Ref: t.slot, Mod: id})
	t.tip.Set(f.th, t.btn.GetWidget(), t.spec.Tip.Key, t.spec.Tip.Build)
}

// syncSummary refreshes the predicted DPS, the capacitor drain against the
// recharge curve's peak, and the verdict with its bar.
func (f *Fitting) syncSummary(player ecs.Entity, ship sim.ShipState, hasShip bool) {
	fit, hasFit := f.cv.Fitting(player)
	dps := f.totalDPS(fit, hasFit)
	drain := f.capDrain(fit, hasFit)

	cap, hasCap := f.cv.Capacitor(player)
	peak := 0.0
	if hasCap {
		peak = capRechargePeak(cap)
	}
	f.sum[0].value.Label = theme.GroupFloat(dps, 1)
	f.sum[1].value.Label = theme.GroupFloat(drain, 1) + " GJ/s"
	f.sum[2].value.Label = theme.GroupFloat(peak, 1) + " GJ/s"

	verdict := capVerdict(cap, hasCap, drain, peak)
	f.verdict.Label = verdict.text
	col := f.th.Pal().Good
	if !verdict.stable {
		col = f.th.Pal().Danger
	}
	f.verdict.SetColor(col)

	frac := 0.0
	if peak > 0 {
		frac = math.Min(drain/peak, 1)
	}
	f.paintCapBar(frac, verdict.stable)
}

// totalDPS is the fit's sustained damage: Σ over the fitted weapons of the
// loaded shell's volley over the fire period (ShotTime, else the cycle
// time). It mirrors the sim's shot resolution (systemWeapons): a dry gun
// contributes nothing, and a shell replaces the gun's base volley scaled by
// its damage multiplier.
func (f *Fitting) totalDPS(fit sim.Fitting, hasFit bool) float64 {
	if !hasFit {
		return 0
	}
	var dps float64
	for _, class := range fitClasses {
		defs := fit.Defs(class)
		for i, id := range defs {
			def, ok := f.cv.Defs().Module(id)
			if !ok || def.Kind != content.KindWeapon || def.Weapon == nil {
				continue
			}
			w := def.Weapon
			volley := w.Damage
			if w.AmmoClass != "" {
				ammo := f.cv.PredictedAmmo(sim.SlotRef{Class: class, Index: i})
				if ammo == "" {
					continue // dry: the sim fires nothing
				}
				if item, ok := f.cv.Defs().Item(ammo); ok && item.Damage != nil {
					volley = item.Damage.Scale(w.Mult())
				}
			}
			period := w.ShotTime
			if period <= 0 {
				period = def.ActivationTime
			}
			if period <= 0 {
				period = 1
			}
			dps += volley.Total() / period
		}
	}
	return dps
}

// capDrain is the fit's sustained capacitor draw in GJ/s: Σ over the
// running modules of their activation cost over their cycle time.
func (f *Fitting) capDrain(fit sim.Fitting, hasFit bool) float64 {
	if !hasFit {
		return 0
	}
	var drain float64
	for _, class := range fitClasses {
		defs := fit.Defs(class)
		rts := fit.Runtimes(class)
		for i, id := range defs {
			if i >= len(rts) || !rts[i].Active {
				continue
			}
			def, ok := f.cv.Defs().Module(id)
			if !ok || def.ActivationTime <= 0 {
				continue
			}
			drain += def.ActivationCost / def.ActivationTime
		}
	}
	return drain
}

// fitEHP is the effective HP against one damage type: the three layer
// maxima over the penetrating share the resistance leaves — the sim's own
// damage rule (applyDamageToShip applies the same resist multiplier).
func fitEHP(ship sim.ShipState, t content.DamageType) float64 {
	mult := ship.Resists.Multiplier(t)
	if mult <= 0 {
		return 0
	}
	return (ship.MaxShield + ship.MaxArmor + ship.MaxStructure) / mult
}

// capRechargePeak is the sim's recharge curve's maximum rate in GJ/s:
// dC/dt = (2.5*Capacity/Tau)*sqrt(c)*(1-sqrt(c)) peaks at c = 0.25
// (sim/system_cap.go), where sqrt(c)*(1-sqrt(c)) = 0.25.
func capRechargePeak(c sim.Capacitor) float64 {
	if c.Capacity <= 0 || c.Tau <= 0 {
		return 0
	}
	return 2.5 * c.Capacity / c.Tau * 0.25
}

// capEndure is how long the capacitor lasts under a constant drain: the
// sim's recharge curve integrated at a fixed step from the current charge
// (sim/system_cap.go), capped at the horizon.
func capEndure(c sim.Capacitor, drain float64) float64 {
	if drain <= 0 || c.Capacity <= 0 || c.Tau <= 0 {
		return 0
	}
	cur := math.Min(math.Max(c.Current, 0), c.Capacity)
	for t := 0.0; t < fitCapHorizon; t += fitCapStep {
		r := math.Sqrt(cur / c.Capacity)
		cur += (2.5*c.Capacity/c.Tau)*r*(1-r)*fitCapStep - drain*fitCapStep
		if cur <= 0 {
			return t + fitCapStep
		}
		cur = math.Min(cur, c.Capacity)
	}
	return fitCapHorizon
}

// fitCapVerdict is the summary's capacitor verdict: the registered string
// and whether the fit is sustainable.
type fitCapVerdict struct {
	text   string
	stable bool
}

// capVerdict names the fit's capacitor: stable while the drain stays under
// the recharge curve's peak rate, else the seconds the charge lasts.
func capVerdict(c sim.Capacitor, hasCap bool, drain, peak float64) fitCapVerdict {
	if !hasCap || drain <= peak {
		return fitCapVerdict{"Cap stable", true}
	}
	secs := capEndure(c, drain)
	return fitCapVerdict{"Cap unstable (~" + theme.FormatInt(int64(math.Ceil(secs))) + " s)", false}
}

// paintCapBar paints the drain-vs-peak bar: filled to frac (drain/peak,
// capped at 100 %), accent while stable and danger red past the peak.
func (f *Fitting) paintCapBar(frac float64, stable bool) {
	quant := math.Round(frac*200) / 200 // 0.5 % steps: no per-frame repaint
	if f.capLit && quant == f.capFrac && stable == f.capStable {
		return
	}
	f.capLit, f.capFrac, f.capStable = true, quant, stable
	p := f.th.Pal()
	w, h := theme.Px(fitBarW), theme.Px(fitBarH)
	f.capBar.Clear()
	theme.FillRect(f.capBar, image.Rect(0, 0, w, h), theme.NRGBA(p.Cell, 160))
	fill := p.Accent
	if !stable {
		fill = p.Danger
	}
	if fw := int(math.Round(frac * float64(w))); fw > 0 {
		theme.FillRect(f.capBar, image.Rect(0, 0, fw, h), fill)
	}
}

// fitReady reports whether mod fits into slot: a real module into a real
// empty slot of its class that also clears the hull's fitting limits
// (content.ShipDef.FitProblem — mount kind/size, grid and CPU quotas). The
// toolbar's Fit button and both row menus (slot tiles, hangar rows) gate
// through this one rule, so the seam keeps them from drifting apart.
func (f *Fitting) fitReady(mod content.ModuleID, slot widgets.SlotPick) bool {
	if mod == "" || slot.Mod != "" {
		return false
	}
	def, ok := f.cv.Defs().Module(mod)
	if !ok || def.Slot != slot.Ref.Class {
		return false
	}
	return f.fitIssue(def) == nil
}

// fitIssue is the hull's first fitting-limit refusal for def (nil = it
// fits). It is the sim's own rule on the same fitted modules, computed
// client-side: no command is needed to know the answer.
func (f *Fitting) fitIssue(def *content.ModuleDef) *content.FitIssue {
	ship, hasShip := f.cv.ShipState(f.cv.Player())
	if !hasShip {
		return nil
	}
	hull, ok := f.cv.Defs().Ship(ship.Hull)
	if !ok {
		return nil
	}
	return hull.FitProblem(def, f.fitDefs)
}

// fitRefusal is the rule that blocks mod from slot, in the sim's exact
// wording ("" = nothing to quote: wrong class, occupied slot or a legal
// fit). The row menu has no sub-line, so this rides the action's label.
func (f *Fitting) fitRefusal(mod content.ModuleID, slot widgets.SlotPick) string {
	if mod == "" || slot.Mod != "" {
		return ""
	}
	def, ok := f.cv.Defs().Module(mod)
	if !ok || def.Slot != slot.Ref.Class {
		return ""
	}
	if issue := f.fitIssue(def); issue != nil {
		return content.FitIssueText(def.Name, issue)
	}
	return ""
}

// unequipReady reports whether a slot can be unfit (it holds a module) —
// the toolbar and the tile menu's Unefit entry share it.
func unequipReady(slot widgets.SlotPick) bool { return slot.Mod != "" }

// slotSpec is one slot tile's declared interactions: Show Info on the
// fitted module (declared even when empty — the entry shows grayed),
// Fit/Unefit gated on THIS slot (the menu acts on the tile it was opened
// on; the toolbar keeps acting on the selection) and the shared module
// tooltip with the slot in its sub line.
func (f *Fitting) slotSpec(pick widgets.SlotPick) widgets.RowSpec {
	fitLabel := "Fit module"
	if refused := f.fitRefusal(f.selMod, pick); refused != "" {
		fitLabel += " · " + refused
	}
	spec := widgets.RowSpec{
		Ref: widgets.RowRef(f.cv, pick),
		Actions: []widgets.RowAction{
			{Label: fitLabel, Disabled: !f.hasMod || !f.fitReady(f.selMod, pick),
				Intent: widgets.Fit{Module: f.selMod, Slot: pick.Ref}},
			{Label: "Unefit slot", Disabled: !unequipReady(pick),
				Intent: widgets.Unequip{Slot: pick.Ref}},
		},
	}
	if pick.Mod == "" {
		return spec // empty slot: no tooltip (and a grayed Fit/Unefit pair)
	}
	def, ok := f.cv.Defs().Module(pick.Mod)
	if !ok {
		return spec
	}
	slot := widgets.SlotLabel(pick.Ref)
	spec.Tip = widgets.TipData{Key: string(pick.Mod) + "|" + slot, Build: func() widgets.TooltipModel {
		return widgets.ModuleTip(def, slot, "", 0)
	}}
	return spec
}

// hangarRows renders the hangar's fit-able modules and stored ships
// (other hangar items live in the Inventory window).
func (f *Fitting) hangarRows() []widgets.TableRow {
	var rows []widgets.TableRow
	for _, st := range f.cv.HangarItems() {
		mod, ok := f.cv.Defs().Module(content.ModuleID(st.Item))
		if !ok || st.Qty <= 0 {
			continue
		}
		id := content.ModuleID(st.Item)
		rows = append(rows, widgets.TableRow{
			Cells: []string{mod.Name, "×" + theme.FormatInt(int64(st.Qty))},
			Icons: []string{theme.ModuleIconFor(mod), ""},
			Data:  id,
			Spec:  f.hangarSpec(id),
		})
	}
	for _, ss := range f.cv.HangarShips() {
		def, ok := f.cv.Defs().Ship(ss.Hull)
		name, icon := string(ss.Hull), theme.HullIcon
		if ok {
			name, icon = def.Name, theme.HullIconFor(def)
		}
		rows = append(rows, widgets.TableRow{
			Cells: []string{name, "×1"},
			Icons: []string{icon, ""},
			Data:  ss.Hull,
			Spec:  f.hangarSpec(ss.Hull),
		})
	}
	return rows
}

// hangarSpec is one hangar row's declared interactions: Show Info plus
// the Fit action into the SELECTED slot — gated by this row's own data
// (a stored hull is not a module, so its Fit stays grayed instead of
// arming from a stale module selection) and the slotless module tooltip
// (stored hulls carry none: they aren't fitted yet).
func (f *Fitting) hangarSpec(data any) widgets.RowSpec {
	spec := widgets.RowSpec{
		Ref: widgets.RowRef(f.cv, data),
	}
	mod, isMod := data.(content.ModuleID)
	fitLabel := "Fit module"
	if refused := f.fitRefusal(mod, f.selSlot); refused != "" {
		fitLabel += " · " + refused
	}
	spec.Actions = []widgets.RowAction{
		{Label: fitLabel, Disabled: !f.fitReady(mod, f.selSlot),
			Intent: widgets.Fit{Module: mod, Slot: f.selSlot.Ref}},
	}
	def, ok := f.cv.Defs().Module(mod)
	if !isMod || !ok {
		return spec
	}
	spec.Tip = widgets.TipData{Key: string(mod), Build: func() widgets.TooltipModel {
		return widgets.ModuleTip(def, "", "", 0)
	}}
	return spec
}

// hangarDouble is the hangar's double-click verb: a module fits into the
// first empty slot of its class (a no-op once the class is full — the
// row menu's Fit-into-selected-slot flow stays the fallback), and a
// stored hull keeps the Show Info gesture.
func (f *Fitting) hangarDouble(r widgets.TableRow) {
	switch data := r.Data.(type) {
	case content.ModuleID:
		f.fitFirstEmpty(data)
	case content.ShipID:
		if r.Spec.Ref != nil {
			f.as.Do(widgets.ShowInfo{Ref: *r.Spec.Ref})
		}
	}
}

// fitFirstEmpty emits Fit for mod into the lowest empty slot of its class
// the hull still has; the first open slot wins, exactly like EVE.
func (f *Fitting) fitFirstEmpty(mod content.ModuleID) {
	def, ok := f.cv.Defs().Module(mod)
	if !ok || def.Slot == "" {
		return
	}
	player := f.cv.Player()
	ship, hasShip := f.cv.ShipState(player)
	if !hasShip {
		return
	}
	hull, ok := f.cv.Defs().Ship(ship.Hull)
	if !ok {
		return
	}
	n := 0
	switch def.Slot {
	case content.SlotHigh:
		n = hull.Slots.High
	case content.SlotMid:
		n = hull.Slots.Mid
	case content.SlotLow:
		n = hull.Slots.Low
	}
	fit, hasFit := f.cv.Fitting(player)
	var defs []content.ModuleID
	if hasFit {
		defs = fit.Defs(def.Slot)
	}
	for i := 0; i < n && i < fitMaxCells; i++ {
		if i < len(defs) {
			continue
		}
		f.as.Do(widgets.Fit{Module: mod, Slot: sim.SlotRef{Class: def.Slot, Index: i}})
		return
	}
}

// selectHangar records the hangar row's payload as the pending fit or
// assemble target (the selection callback and the row menu share it).
func (f *Fitting) selectHangar(r widgets.TableRow) {
	switch id := r.Data.(type) {
	case content.ModuleID:
		f.selMod, f.hasMod = id, true
	case content.ShipID:
		f.selShip, f.hasShip = id, true
	}
}

// unfitClicked emits Unequip for the selected fitted slot.
func (f *Fitting) unfitClicked() { f.as.Do(widgets.Unequip{Slot: f.selSlot.Ref}) }

// fitClicked emits Fit: the selected hangar module into the selected
// empty slot of its class.
func (f *Fitting) fitClicked() {
	if !f.hasMod || !f.fitReady(f.selMod, f.selSlot) {
		return
	}
	f.as.Do(widgets.Fit{Module: f.selMod, Slot: f.selSlot.Ref})
}

// assembleClicked emits Assemble for the selected hangar ship.
func (f *Fitting) assembleClicked() {
	if !f.hasShip {
		return
	}
	f.as.Do(widgets.Assemble{Ship: f.selShip})
}
