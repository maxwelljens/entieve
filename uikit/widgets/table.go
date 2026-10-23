package widgets

import (
	"image"
	"image/color"
	"math"
	"reflect"
	"sort"
	"time"

	eimage "github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// Table layout constants (derived from the legacy ui padding scale).
const (
	// TableHeaderH is the height of the sort-button header strip. 40
	// (design units, scaled by Px) gives every sort header a ≥40 px hit
	// strip; the width stays the fitted column width.
	TableHeaderH = 40
	// headerSpace is the gap between the header strip and the rows
	// (the furniture token; the row metrics below stay dense on purpose).
	headerSpace = theme.SpSm
	// cellPad is the horizontal padding inside every column cell (the
	// header title and the row cells share it, so their text agrees).
	cellPad = theme.SpMd
	// entryPadV is the vertical padding inside a row.
	entryPadV = 5
	// selectionAlpha is the accent tint alpha of the selected row.
	selectionAlpha = 64
	// wheelRows is how many rows one mouse-wheel notch scrolls.
	wheelRows = 3
)

// doubleClickWindow is the maximum gap between two row activations that
// still counts as a double click (activate).
const doubleClickWindow = 350 * time.Millisecond

// Column describes one table column: its sort key/title, its preferred
// (content) width and shrink floor, its shrink priority when space is
// short (higher drops first), and its text alignment.
type Column struct {
	Key        string  // sort key passed to SetSort/SetSortFunc
	Title      string  // header label
	Width      float64 // preferred (content) width; 0 = measure the content
	MinWidth   float64 // never shrink below this (the title's own width is an implicit floor)
	Priority   int     // shrink order when over space (higher drops first)
	AlignRight bool    // numeric columns: fixed right-aligned anchors
}

// TableRow is one displayed row: one cell string per Column, an optional
// background tint (zero = none), an optional cell-text tint (zero = the
// palette's text; the market's orderless item rows dim through it) and an
// opaque payload handed to the row callbacks (e.g. sim.Info,
// content.ItemID). Icons, when set, runs parallel
// to Cells: an icon name per cell ("" = none) drawn in a fixed box to the
// left of that cell's text. Right-aligned columns ignore it — numeric
// anchors stay exact — so icons belong in the left-aligned cells. Spec is
// the row's declared interactions (RowSpec): the table applies it through
// the installer — right-click menu, lazy tooltip, strict double-click
// Show Info — so surfaces declare data instead of wiring handlers.
//
// Stack, when set, makes the row RICH: it renders as that line stack
// instead of one text per column (see the rich-row section below), with
// Icons[0] leading its first line and Spec.Actions drawn as trailing
// buttons beside it. Cells stay the row's sort/measure source either way.
type TableRow struct {
	Cells    []string
	Icons    []string
	IconTint color.RGBA // icon palette role; zero = ColText
	Tint     color.RGBA
	TextTint color.RGBA // cell text colour; zero = the palette's text
	Data     any
	Spec     RowSpec
	Stack    []RowLine
	// CellRuns, when a cell's entry is non-empty, renders that cell as a
	// run of styled segments instead of one label — the market's dim
	// "· N more" availability suffix rides its Qty cell this way. The
	// cell string stays the fit/sort source either way. Runs are drawn
	// flush with the cell's own edge (left or right per the column).
	CellRuns [][]RowSeg
}

// iconOf returns row cell i's icon name ("" when the row carries none).
func iconOf(row TableRow, i int) string {
	if i < 0 || i >= len(row.Icons) {
		return ""
	}
	return row.Icons[i]
}

// ---------------------------------------------------------------------------
// Rich rows: line stacks, leading icons and trailing action buttons
// ---------------------------------------------------------------------------

// Rich-row geometry (design units, scaled by theme.Px): the line stack's
// own metrics — the look the contract board established and every rich
// row keeps. Plain cells keep cellPad/entryPadV above; a row is either
// rich or plain, never a mix.
const (
	// RichPad is the horizontal padding inside a rich row.
	RichPad = 3
	// RichGap is the gap between the cells of a line, and between a
	// line's last cell and the trailing action buttons.
	RichGap = 6
	// RichBtnW is the trailing action button box: the widest label the
	// RowSpec seam renders ("Accepted") in the emphasis face, so every
	// button — gated or not — measures the same and the line stays flush
	// with the row's right edge.
	RichBtnW = 92
	// RichBtnH is that button's minimum hit area (menuBtnH).
	RichBtnH = 40
)

// RowSeg is one text run inside a line of a rich row: its own type-scale
// step ("" = the row's default face), colour (zero = the palette's text
// colour) and minimum width in LIVE pixels (0 = the text's own width,
// already scaled by the caller like every widget minimum here).
type RowSeg struct {
	Text string
	Tint color.RGBA
	Step string
	MinW int
}

// RowLine is one line of a rich row's stack: a run of cells drawn left to
// right, plus the line's own Ref. A ref'd line is its own gesture target —
// the installer's line half applies to it (right-click menu with the
// bold-first Show Info, double-click opens it strictly), exactly like a
// chain ref in the Show Info window — while the row itself keeps its own
// menu from Spec.
type RowLine struct {
	Segs []RowSeg
	Ref  *InfoRef
}

// StackRow is one rendered rich row: the container the owner tiles (a
// table body, a board section), its trailing action buttons (nil when the
// row declares none) and the icon box leading line 1 (nil without an
// icon). The handles the owner needs afterwards: a table's selection tint
// moves the icon, a board's test surface reads the button.
type StackRow struct {
	Root    *widget.Container
	Buttons []*widget.Button
	Icon    *theme.IconBox
}

// NewStackRow builds a rich row's widget tree from its TableRow: a
// vertical run of lines (Padding RichPad; a lone line centres itself in
// the strip, the way a plain row centres its text), each line a
// horizontal run of its cells (Spacing RichGap), the row's icon boxing
// with the first cell of line 1 (the icon's own IconGap) and its
// RowSpec.Actions rendered as trailing buttons on line 1 (RichBtnW×H,
// Disabled grayed and silent — the grayed-not-hidden convention).
//
// Only the LINES are wired here: a line carrying a Ref gets the shared
// installer's line half (right-click menu, double-click Show Info).
// Root-level clicks belong to the owner — a table selects and opens the
// row menu, a board only opens the menu — because a child's handler runs
// after its parent's, so the line's menu wins over the row's exactly as
// the seam prescribes.
//
// face is the default face for cells that name no step (the table's own
// cell face; nil = the small display step). Cell text arrives ready to
// draw: owners ellipsize against the width they measured, the builder
// never re-fits a stack (its lines are not columns).
func NewStackRow(env Seams, row TableRow, face *text.Face) *StackRow {
	p := env.Pal()
	out := &StackRow{}
	cellFace := func(step string) *text.Face {
		if step == "" {
			if face != nil {
				return face
			}
			return env.Face(theme.StepSmall)
		}
		return env.Face(step)
	}
	seg := func(s RowSeg) *widget.Text {
		col := s.Tint
		if col.A == 0 {
			col = p.Text
		}
		opts := []widget.TextOpt{
			widget.TextOpts.Text(s.Text, cellFace(s.Step), theme.Solid(col)),
		}
		if s.MinW > 0 {
			opts = append(opts, widget.TextOpts.WidgetOpts(widget.WidgetOpts.MinSize(s.MinW, 0)))
		}
		return widget.NewText(opts...)
	}
	// A ref'd line is its own gesture target (the installer's line half).
	// The double-click state is per line: like every surface, one build's
	// clicks have to arm against the line they landed on.
	wire := func(w *widget.Widget, spec RowSpec) {
		var dbl RowDouble
		w.MouseButtonClickedEvent.AddHandler(func(args any) {
			a, ok := args.(*widget.WidgetMouseButtonClickedEventArgs)
			if !ok {
				return
			}
			switch a.Button {
			case ebiten.MouseButtonRight:
				OpenRowMenu(env, env, spec, ClickPoint(a))
			case ebiten.MouseButtonLeft:
				dbl.Click(env, env, spec)
			}
		})
	}

	// A lone line centres itself in the strip (the stretched row a board
	// section hands a "+N more" note); a stack tiles from the top.
	dir := widget.DirectionHorizontal
	if len(row.Stack) > 1 {
		dir = widget.DirectionVertical
	}
	root := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewRowLayout(
		widget.RowLayoutOpts.Direction(dir),
		widget.RowLayoutOpts.Padding(&widget.Insets{
			Left: theme.Px(RichPad), Right: theme.Px(RichPad),
		}),
	)))
	// A rich row owns buttons, and ebitenui validates a button against a
	// theme ("Image is required") — which can happen the moment the row's
	// strip or tint is set, before it has joined any window. Bind the
	// session's widget theme here: the same object the surface's windows
	// ride, rebuilt together with them.
	root.GetWidget().SetTheme(env.WidgetTheme())
	out.Root = root

	iconTint := p.Text
	if row.IconTint.A != 0 {
		iconTint = row.IconTint
	}
	for li, line := range row.Stack {
		var cells []widget.PreferredSizeLocateableWidget
		// The row's icon (Icons[0]) leads line 1: it boxes with that
		// line's first cell at the icon gap and centres in the line —
		// the row can be as tall as its button, and a top-flush glyph
		// reads as a misalignment.
		inBox := 0
		if li == 0 && iconOf(row, 0) != "" {
			box := theme.NewIconBox(theme.IconInline)
			box.Set(iconOf(row, 0), iconTint)
			out.Icon = box
			lead := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewRowLayout(
				widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
				widget.RowLayoutOpts.Spacing(theme.Px(theme.IconGap)),
			)))
			for _, w := range []widget.PreferredSizeLocateableWidget{box.Widget(), lead} {
				w.GetWidget().LayoutData = widget.RowLayoutData{Position: widget.RowLayoutPositionCenter}
			}
			lead.AddChild(box.Widget())
			if len(line.Segs) > 0 {
				lead.AddChild(seg(line.Segs[0]))
				inBox = 1
			}
			cells = append(cells, lead)
		}
		for i, s := range line.Segs {
			if i < inBox {
				continue // already drawn inside the icon box
			}
			cells = append(cells, seg(s))
		}
		if li == 0 {
			for i, a := range row.Spec.Actions {
				if i == 2 {
					break // a row offers at most two trailing buttons
				}
				action := a
				btn := widget.NewButton(
					widget.ButtonOpts.TextLabel(action.Label),
					widget.ButtonOpts.WidgetOpts(widget.WidgetOpts.MinSize(
						theme.Px(RichBtnW), theme.Px(RichBtnH))),
					widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) {
						// Grayed rows emit nothing — belt and braces
						// with the disabled widget (a harness may fire
						// the click event straight past the gate).
						if !action.Disabled && action.Intent != nil {
							env.Do(action.Intent)
						}
					}),
				)
				btn.GetWidget().Disabled = action.Disabled
				out.Buttons = append(out.Buttons, btn)
				cells = append(cells, btn)
			}
		}

		var lineW widget.PreferredSizeLocateableWidget
		if len(cells) == 1 {
			lineW = cells[0] // a lone cell draws as itself (the plain row's own text)
		} else {
			ctr := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewRowLayout(
				widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
				widget.RowLayoutOpts.Spacing(theme.Px(RichGap)),
			)))
			ctr.AddChild(cells...)
			lineW = ctr
		}
		if dir == widget.DirectionHorizontal {
			// The line centres itself in the strip it is the only child of.
			lineW.GetWidget().LayoutData = widget.RowLayoutData{Position: widget.RowLayoutPositionCenter}
		}
		root.AddChild(lineW)
		if line.Ref != nil {
			wire(lineW.GetWidget(), RowSpec{Ref: line.Ref})
		}
	}
	return out
}

// StackLayout tiles a section's rows the way a board section stacks its
// rich rows: equal heights, Gap pixels apart, the first row flush with
// the top — every row pairwise disjoint and fully inside the rect. A
// section reports the tallest row times the count as its preferred
// height (a zero-width preferred rect would be dropped by the parent's
// union, shortening the window below its rows), so a "+N more" note
// never shrinks its section and rows stretch together when the window
// floors. n <= 0 lays out nothing.
//
// Plain table bodies keep their spacing-0 natural tiling: every plain
// row is one rowH tall, so the two agree wherever they could both apply.
type StackLayout struct{ Gap int }

// PreferredSize is the stack: widest row by n rows of the tallest, gaps
// included.
func (l StackLayout) PreferredSize(ws []widget.PreferredSizeLocateableWidget) (int, int) {
	if len(ws) == 0 {
		return 0, 0
	}
	w, h := 0, 0
	for _, c := range ws {
		cw, ch := c.PreferredSize()
		if cw > w {
			w = cw
		}
		if ch > h {
			h = ch
		}
	}
	return w, len(ws)*h + (len(ws)-1)*l.Gap
}

// Layout places every row at its StackRects slot.
func (l StackLayout) Layout(ws []widget.PreferredSizeLocateableWidget, rect image.Rectangle) {
	for i, r := range StackRects(rect, len(ws), l.Gap) {
		ws[i].SetLocation(r)
	}
}

// StackRects is the pure tiling behind StackLayout: n equal-height rows
// inside inner, gap pixels apart, the first flush with inner's top.
// n <= 0 (or an inner too small to hold even one pixel-high row) returns
// no rects; when the inner cannot hold the gaps too, they collapse to
// zero rather than the rows.
func StackRects(inner image.Rectangle, n, gap int) []image.Rectangle {
	if n <= 0 || inner.Dx() <= 0 || inner.Dy() <= 0 {
		return nil
	}
	if (inner.Dy()-(n-1)*gap)/n < 1 {
		gap = 0 // degenerate: no room for gaps, stack 1px rows
	}
	h := (inner.Dy() - (n-1)*gap) / n
	if h < 1 {
		return nil // not even one row fits
	}
	rows := make([]image.Rectangle, n)
	y := inner.Min.Y
	for i := range rows {
		rows[i] = image.Rect(inner.Min.X, y, inner.Max.X, y+h)
		y += h + gap
	}
	return rows
}

// Table is a sortable, scrollable column table: a header row of sort
// buttons over a widget.ScrollContainer of per-row widget.Containers.
// Cells draw in the contents face — the display body face by default,
// the mono contents face via ContentsFace — while the header sort
// buttons always keep the emphasis face.
//
// Unlike widget.List it owns its scroll instance (ScrollTop is readable),
// every row is a real container (full per-row background tinting) and the
// columns are real fixed-width labels (right-aligned cells share exact
// anchors). Events in ebitenui are deferred to the next UI.Update, so
// handlers registered here go live on the first update pump.
type Table struct {
	th          theme.Theme
	cv          ContentView
	as          ActionSink
	cols        []Column
	Face        *text.Face
	measure     func(string) float64 // cell width, contents face
	headMeasure func(string) float64 // header width, emphasis weight
	rowH        float64

	root    *widget.Container
	rail    *ScrollRail // the reserved-gutter scrollbar beside the viewport
	header  *widget.Container
	hdrRow  *widget.Container // the sort buttons, tiling the strip
	hdrBtn  []*widget.Button
	grips   []*widget.Button // one resize strip per column (headerLayout)
	menuBtn *widget.Button   // the ⋯ column menu at the strip's right end
	hdrIdle *widget.ButtonImage
	hdrOn   *widget.ButtonImage
	scroll  *widget.ScrollContainer
	body    *widget.Container

	Rows      []TableRow // display order (sorted)
	rawRows   []TableRow // cell/icon/stack content last applied to the widgets
	Texts     [][]*widget.Text
	Cells     [][]*tableCell // per-cell widget pair (nil for right-aligned)
	RowCts    []*widget.Container
	rich      []*StackRow  // rich row handles (nil at plain rows), parallel to RowCts
	applied   []color.RGBA // background last painted per row widget
	Tips      []RowTip     // per-row tooltip slot (the spec's Tip data)
	widths    []float64    // fitted column widths
	fittedFor float64      // available width the fit was computed for
	base      []ColWidth   // the frozen per-column measure (see ensureFrozen)
	frozen    bool         // base is the first fit's tiling, mine to keep
	hidden    []bool       // columns the display filter leaves out
	prefs     map[string]ColumnPref
	prefsID   string
	grip      int     // column whose grip is dragged (-1 = none)
	gripX     int     // cursor x at the drag's press
	gripFrom  float64 // the grabbed column's width at the press
	sortKey   string
	sortAsc   bool
	sortFn    func(key string, asc bool, rows []TableRow)
	onSelect  func(row TableRow)
	onDouble  func(row TableRow) // double-click override (nil = Show Info)

	selIdx  int // selected row index, -1 = none
	selData any // Data of the selected row (identity across rebuilds)

	dbl RowDouble // double-click bookkeeping: the installer's half

	// plateBand, when set, is the artwork band a window paints behind this
	// table (screens.headerPlateBand): the viewport leaves its body fill off
	// above the band's bottom edge so the plate reads through the first rows.
	// plateTop is the fill's device-px offset into the viewport, 0 = today.
	plateBand func() image.Rectangle
	plateTop  int
}

// TableOption adjusts a table at construction; a table built without
// options is the display-font table.
type TableOption func(*Table)

// ContentsFace draws every row cell — labels and values alike — in the
// given face instead of the display face, and measures the columns with
// it (IBM Plex Mono via Theme.FaceMono in the inventory and market
// windows: monospaced cells line the columns up). Only the contents move:
// the header sort buttons keep the emphasis face.
func ContentsFace(face *text.Face) TableOption {
	return func(t *Table) { t.Face = face }
}

// PlateBand makes the scroll viewport show a window's artwork band through its
// top rows: the viewport body fill starts at band's bottom edge instead of its
// top, so the plate behind the table reads untinted under those rows. Only the
// fill moves — the border and every row's hover/selection tint stay as they
// are. band reports the band's screen rect (screens.headerPlateBand; an empty
// rect means no band or no plate loaded, which leaves the viewport as today).
// Opt-in: a table built without it paints exactly as before.
func PlateBand(band func() image.Rectangle) TableOption {
	return func(t *Table) { t.plateBand = band }
}

// NewTable builds a table with the given columns over the table's three
// seams (faces, the row refs, the row-menu sink). Rows arrive via SetRows; embed it in a
// window with Container(). Options apply at construction — ContentsFace
// moves the row cells (and the column fit) to another face.
func NewTable(env Seams, cols []Column, opts ...TableOption) *Table {
	p := env.Pal()
	t := &Table{
		th:     env,
		cv:     env,
		as:     env,
		cols:   append([]Column(nil), cols...),
		Face:   env.Face(theme.StepSmall), // cell face: display body by default
		selIdx: -1,
		grip:   -1,
	}
	t.hidden = make([]bool, len(t.cols))
	for _, opt := range opts {
		opt(t)
	}
	// The cell measure and the row height follow t.Face (set above or by
	// an option), so the fitted column widths and the drawn cells agree.
	face := t.Face
	// Headers draw in the emphasis weight (IBM Plex Sans Bold), so their
	// widths are measured with it too — the fit and the drawn label have
	// to agree or the header overflows its column.
	headFace := env.FaceBold(theme.StepSmall)
	t.measure = func(s string) float64 {
		w, _ := text.Measure(s, *face, 0)
		return w
	}
	t.headMeasure = func(s string) float64 {
		w, _ := text.Measure(s, *headFace, 0)
		return w
	}
	m := (*face).Metrics()
	t.rowH = math.Ceil(m.HAscent+m.HDescent) + 2*float64(theme.Px(entryPadV))

	t.hdrIdle = &widget.ButtonImage{
		Idle:         theme.Bordered(theme.Solid(p.HeadBg), theme.NRGBA(p.Border, 160), 1),
		Hover:        theme.Bordered(theme.NRGBA(p.Hover, 18), theme.NRGBA(p.Accent, 200), 1),
		Pressed:      theme.Bordered(theme.NRGBA(p.Hover, 45), theme.NRGBA(p.Border, 220), 1),
		PressedHover: theme.Bordered(theme.NRGBA(p.Hover, 45), theme.NRGBA(p.Accent, 200), 1),
		Disabled:     theme.Bordered(theme.NRGBA(p.Cell, 140), theme.NRGBA(p.Border, 110), 1),
	}
	t.hdrOn = &widget.ButtonImage{
		Idle:         theme.Bordered(theme.NRGBA(p.Accent, 36), theme.NRGBA(p.Accent, 200), 1),
		Hover:        theme.Bordered(theme.NRGBA(p.Accent, 60), theme.NRGBA(p.Accent, 220), 1),
		Pressed:      theme.Bordered(theme.NRGBA(p.Accent, 80), theme.NRGBA(p.Accent, 220), 1),
		PressedHover: theme.Bordered(theme.NRGBA(p.Accent, 80), theme.NRGBA(p.Accent, 220), 1),
		Disabled:     theme.Bordered(theme.NRGBA(p.Cell, 140), theme.NRGBA(p.Border, 110), 1),
	}

	// Header: one sort button per column in a row that tiles the strip; the
	// column chrome (the resize grips and the ⋯ menu) is anchored over it
	// by headerLayout, so the sort buttons keep their exact rects.
	t.header = widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(eimage.NewNineSliceColor(theme.Solid(p.HeadBg))),
		widget.ContainerOpts.Layout(headerLayout{t: t}),
	)
	t.hdrRow = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(0),
		)),
	)
	t.header.AddChild(t.hdrRow)
	for i := range t.cols {
		i := i
		c := t.cols[i]
		align := widget.TextPositionStart
		if c.AlignRight {
			align = widget.TextPositionEnd
		}
		btn := widget.NewButton(
			widget.ButtonOpts.TextLabel(c.Title),
			widget.ButtonOpts.TextFace(headFace),
			widget.ButtonOpts.TextColor(&widget.ButtonTextColor{
				Idle:     theme.Solid(p.Text),
				Hover:    theme.Solid(p.Text),
				Pressed:  theme.Solid(p.Text),
				Disabled: theme.NRGBA(p.Dim, 160),
			}),
			widget.ButtonOpts.TextPosition(align, widget.TextPositionCenter),
			// The label anchors exactly like a left cell's text (the
			// container's cellPad inset) and a right cell's glyphs (see
			// the right-cell padding note in newRow): both land Px(cellPad)
			// inside the fitted column, at every scale.
			widget.ButtonOpts.TextPadding(&widget.Insets{
				Left: theme.Px(cellPad), Right: theme.Px(cellPad), Top: theme.Px(4), Bottom: theme.Px(4),
			}),
			widget.ButtonOpts.Image(t.hdrIdle),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(0, theme.Px(TableHeaderH)),
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
			widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) {
				t.headerClicked(i)
			}),
		)
		t.hdrBtn = append(t.hdrBtn, btn)
		t.hdrRow.AddChild(btn)
	}
	t.buildColumnChrome()

	// Rows: one container per row inside the scroll content.
	t.body = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(0),
		)),
	)
	t.scroll = widget.NewScrollContainer(
		widget.ScrollContainerOpts.Content(t.body),
		// Columns are fitted to the view width, so the content stretches
		// to fill it instead of scrolling horizontally at rest.
		widget.ScrollContainerOpts.StretchContentWidth(),
		widget.ScrollContainerOpts.Image(t.scrollImage(0)),
	)
	// Wheel scrolls the viewport; ScrollTop is a 0..1 fraction of the
	// content overflow, so the step is scaled by the measured overflow.
	t.scroll.GetWidget().ScrolledEvent.AddHandler(func(args any) {
		if ev, ok := args.(*widget.WidgetScrolledEventArgs); ok {
			t.scrollBy(ev.Y)
		}
	})
	// Let the window chrome's translate walk carry the body with the
	// viewport (see AttachScrollContent): a title drag must not
	// re-locate — and thereby relayout — 300+ rows every frame.
	AttachScrollContent(t.scroll, t.body)

	// Root: header row (fixed) over the scroll row (fills the rest).
	// A grid keeps the viewport bounded by the window — the scroll
	// content's own preferred height must never leak into it.
	t.root = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, true}),
			widget.GridLayoutOpts.Spacing(0, theme.Px(headerSpace)),
		)),
	)
	t.root.AddChild(t.header, t.scroll)

	// The rail: a reserved gutter at the table's right edge with the
	// house scrollbar in it (ScrollRail). The viewport and the header are
	// inset past it, so the handle can never overhang a cell, and the
	// gutter is reserved while the rows fit too — the viewport never jumps
	// when a long list starts overflowing.
	t.rail = NewScrollRail(env, t.root, t.scroll, t.body)
	return t
}

// Container returns the table's widget tree for embedding in a window or
// the root container. Give it the full cell of a stretched grid row (or
// an AnchorLayoutData with both stretches) so the viewport is bounded.
// The returned root wraps the header/scroll column and the scrollbar
// rail; the header and viewport never reach the rail's gutter.
func (t *Table) Container() widget.Containerer { return t.rail.Container() }

// Rail returns the table's scrollbar slider (drag writes the scroll
// offset back; the geometry pins read its rect).
func (t *Table) Rail() *widget.Slider { return t.rail.Bar() }

// SelectData selects the row carrying the given Data identity (nil clears
// the selection) and refreshes row highlights. For selection that
// originates outside the table — world clicks, keyboard cycles.
func (t *Table) SelectData(v any) {
	t.selIdx, t.selData = -1, nil
	if v != nil {
		for i, r := range t.Rows {
			if sameData(r.Data, v) {
				t.selIdx, t.selData = i, v
				break
			}
		}
	}
	t.updateRows()
}

// OnSelect registers the row-click callback (fires on every click).
// OnDouble replaces the table's Show Info double-click with fn: the second
// click on the same row runs fn instead of the Show Info gesture (the
// fitting hangar's Fit verb). The row menu still offers Show Info.
func (t *Table) OnDouble(fn func(row TableRow)) { t.onDouble = fn }

func (t *Table) OnSelect(fn func(row TableRow)) { t.onSelect = fn }

// Selected returns the currently selected row in display order and
// whether there is one. It reports the reconciled state: a row whose
// Data vanished from the table since the click reports no selection, so
// owners caching a selection across Syncs can revalidate against it.
func (t *Table) Selected() (TableRow, bool) {
	if t.selIdx < 0 || t.selIdx >= len(t.Rows) {
		return TableRow{}, false
	}
	return t.Rows[t.selIdx], true
}

// OnActivate and OnMenu are retired: a row's double-click and menu now
// derive from its TableRow.Spec through the installer (rowspec.go), so
// surfaces declare data instead of wiring handlers.

// SetSortFunc overrides how rows are ordered for a column key: fn must
// order rows in place (stable). The default orders lexicographically by
// the sort column's cell. Passing nil restores the default.
func (t *Table) SetSortFunc(fn func(key string, asc bool, rows []TableRow)) {
	t.sortFn = fn
}

// SetSort activates the given sort column and direction: it re-orders the
// current rows, moves the ↑/↓ indicator and re-fits the header widths.
func (t *Table) SetSort(key string, asc bool) {
	t.sortKey, t.sortAsc = key, asc
	t.applySort()
	t.reconcileSelection()
	t.updateRows()
	t.applyWidths()
}

// SetRows replaces the table's rows (a copy), applies the active sort,
// keeps the selection when its row still exists and rebuilds or updates
// the row widgets. The columns are re-fitted only when the row set, a
// cell's text or an icon actually changed (or on the first fit): an
// identical SetRows is a visual no-op and costs no measuring or layout.
func (t *Table) SetRows(rows []TableRow) {
	t.Rows = append([]TableRow(nil), rows...)
	t.applySort()
	t.reconcileSelection()
	// t.widths is nil until the first fit — that one always lays out,
	// whatever the rows are (the header buttons need their boxes).
	if t.updateRows() || t.widths == nil {
		t.applyWidths()
	}
}

// Sync re-fits the columns when the table's width changed (window
// resize/drag), re-pins the scroll offset to the current overflow
// (viewport growth can make a scrolled list fit again) and steps the
// column grip drag. Owners call it once per frame; it is cheap when
// nothing changed.
func (t *Table) Sync() {
	t.Poll()
	t.clampScroll()
	t.rail.Sync()
	t.syncPlateBand()
	if avail := t.columnsWidth(); avail > 0 && avail != t.fittedFor {
		t.applyWidths()
	}
}

// Refit re-applies the column widths immediately (for owners that resize
// the table outside the normal frame pump). The measurement itself is
// frozen — a refit only re-runs the fit over the frozen base.
func (t *Table) Refit() { t.applyWidths() }

// scrollImage is the viewport's background: today's bordered body fill, or —
// with the plate knob — the same border with the fill starting top device px
// down, so the artwork band reads through the rows above that line.
func (t *Table) scrollImage(top int) *widget.ScrollContainerImage {
	p := t.th.Pal()
	idle := theme.Bordered(theme.WindowBody(p.BG), theme.NRGBA(p.Border, 160), 1)
	if top > 0 {
		idle = theme.BandedBox(theme.WindowBody(p.BG), theme.NRGBA(p.Border, 160), top)
	}
	return &widget.ScrollContainerImage{
		Idle:     idle,
		Disabled: theme.Bordered(theme.NRGBA(p.Cell, 140), theme.NRGBA(p.Border, 110), 1),
		Mask:     eimage.NewNineSliceColor(theme.WindowBody(p.BG)),
	}
}

// syncPlateBand rebuilds the viewport background when the artwork band's
// offset into the viewport moves (first layout, resize, scale, plate unloaded).
func (t *Table) syncPlateBand() {
	if t.plateBand == nil {
		return
	}
	top := plateFillTop(t.plateBand(), t.scroll.GetWidget().Rect)
	if top > 0 {
		// The band never outgrows the viewport: the bottom border stays put.
		top = min(top, t.scroll.GetWidget().Rect.Dy()-theme.Px(1))
	}
	if top == t.plateTop {
		return
	}
	t.plateTop = top
	widget.ScrollContainerOpts.Image(t.scrollImage(top))(t.scroll)
}

// plateFillTop is how many px of the viewport the artwork band covers: the
// fill starts below it. A band ending above the viewport leaves today's fill;
// one covering all of it leaves no fill at all.
func plateFillTop(band, view image.Rectangle) int {
	switch {
	case view.Empty() || band.Empty():
		return 0
	case band.Max.Y <= view.Min.Y, band.Min.Y >= view.Max.Y:
		return 0 // no band over the viewport: today's fill
	case band.Max.Y >= view.Max.Y:
		return view.Dy() // the band covers the rest: no fill at all
	default:
		return band.Max.Y - view.Min.Y
	}
}

// headerClicked toggles the sort column/direction on a header click.
func (t *Table) headerClicked(i int) {
	key := t.cols[i].Key
	if t.sortKey == key {
		t.SetSort(key, !t.sortAsc)
		return
	}
	t.SetSort(key, true)
}

// applySort orders t.Rows by the active sort (stable).
func (t *Table) applySort() {
	if t.sortKey == "" {
		return
	}
	idx := t.colIndex(t.sortKey)
	if idx < 0 {
		return
	}
	if t.sortFn != nil {
		t.sortFn(t.sortKey, t.sortAsc, t.Rows)
		return
	}
	sort.SliceStable(t.Rows, func(a, b int) bool {
		x, y := cellOf(t.Rows[a], idx), cellOf(t.Rows[b], idx)
		if t.sortAsc {
			return x < y
		}
		return y < x
	})
}

// colIndex returns the column index of a sort key (-1 if unknown).
func (t *Table) colIndex(key string) int {
	for i, c := range t.cols {
		if c.Key == key {
			return i
		}
	}
	return -1
}

// reconcileSelection re-resolves the selection after a reorder/rebuild:
// Data is the row's stable identity, index only as long as Data is nil.
func (t *Table) reconcileSelection() {
	if t.selIdx < 0 {
		return
	}
	if t.selData != nil {
		t.selIdx = -1
		for i, r := range t.Rows {
			if sameData(r.Data, t.selData) {
				t.selIdx = i
				return
			}
		}
		t.selData = nil
		return
	}
	if t.selIdx >= len(t.Rows) {
		t.selIdx = -1
	}
}

// updateRows creates the row widgets when the row count changed — or when
// a rich row's rendered content (its line stack, cells, icon, or the label
// and gate of a trailing action) moved — and refreshes cell text and tints
// in place otherwise (diff-friendly). It reports whether any cell text,
// icon or stack changed — the input the column refit needs; tints and
// selection repaint through updateBg/updateIcon without touching the
// layout.
func (t *Table) updateRows() bool {
	changed := len(t.RowCts) != len(t.Rows) || t.stackMoved() || t.cellRunsMoved()
	if changed {
		t.body.RemoveChildren()
		t.RowCts, t.Texts, t.Cells, t.rich, t.applied, t.rawRows, t.Tips = nil, nil, nil, nil, nil, nil, nil
		for i := range t.Rows {
			ctr := t.newRow(i)
			t.RowCts = append(t.RowCts, ctr)
			t.body.AddChild(ctr)
		}
	}
	for i, row := range t.Rows {
		if i >= len(t.rawRows) {
			t.rawRows = append(t.rawRows, TableRow{})
		}
		// Diff against the raw cells/icons/stack last applied (never
		// against the widget labels: those hold the ellipsized form, so a
		// truncated cell would read as "changed" forever).
		prev := &t.rawRows[i]
		cellsMoved := !sameCellsIcons(*prev, row, len(t.cols))
		if cellsMoved || !sameStack(*prev, row) {
			changed = true
			*prev = snapshotRow(row)
			// Plain rows patch their labels in place (widgets survive);
			// a rich row draws its own stack — when it moved, the rebuild
			// above already replaced its widgets, and its cells are only
			// ever the fit/sort source.
			if i >= len(t.rich) || t.rich[i] == nil {
				for j := range t.cols {
					cell := cellOf(row, j)
					if tx := t.Texts[i][j]; tx != nil && tx.Label != cell {
						// Raw cell; applyWidths ellipsizes it to the fitted column.
						tx.Label = cell
					}
				}
			}
		}
		t.updateIcon(i)
		t.updateBg(i)
		t.updateTextTint(i)
		// The installer's tooltip half: attach the row's declared Tip to
		// its widget (rowTip rebuilds the panel only when the key moves;
		// a spec with no Tip drops any attachment).
		if i < len(t.Tips) {
			t.Tips[i].Set(t.th, t.RowCts[i].GetWidget(), row.Spec.Tip.Key, row.Spec.Tip.Build)
		}
	}
	// Row count changed → the content may no longer overflow: drop any
	// leftover scroll offset now, before the next render positions rows.
	t.clampScroll()
	return changed
}

// stackMoved reports whether any RICH row's rendered content differs from
// the applied snapshot — the change gate that keeps an identical SetRows
// a no-op while a moved stack (or the label/gate of one of its trailing
// buttons) earns a rebuild. Plain rows — no stack on either side — never
// move here: their cells refresh in place inside updateRows; a row that
// gained or dropped its stack does, so its widgets follow the form.
func (t *Table) stackMoved() bool {
	for i, row := range t.Rows {
		if i >= len(t.rawRows) {
			continue
		}
		prev := t.rawRows[i]
		if len(prev.Stack) == 0 && len(row.Stack) == 0 {
			continue
		}
		if !sameCellsIcons(prev, row, len(t.cols)) || !sameStack(prev, row) {
			return true
		}
	}
	return false
}

// cellRunsMoved reports whether a row's styled cell runs changed — a run
// cell's widgets are built at row creation, so a moved suffix rebuilds the
// row (the plain cells' own labels patch in place).
func (t *Table) cellRunsMoved() bool {
	for i, row := range t.Rows {
		if i >= len(t.rawRows) {
			continue
		}
		if !reflect.DeepEqual(t.rawRows[i].CellRuns, row.CellRuns) {
			return true
		}
	}
	return false
}

// snapshotRow copies the parts of a row its widgets render from — cell
// text, icons and, for a rich row, its line stack plus the label and gate
// of each trailing action — so the diff compares against data the caller
// can no longer mutate (rows arrive by value over shared slices).
func snapshotRow(row TableRow) TableRow {
	s := TableRow{
		Cells: append([]string(nil), row.Cells...),
		Icons: append([]string(nil), row.Icons...),
	}
	if len(row.CellRuns) > 0 {
		s.CellRuns = make([][]RowSeg, len(row.CellRuns))
		for i, runs := range row.CellRuns {
			s.CellRuns[i] = append([]RowSeg(nil), runs...)
		}
	}
	if len(row.Stack) > 0 {
		s.Stack = copyStack(row.Stack)
		s.Spec.Actions = append([]RowAction(nil), row.Spec.Actions...)
	}
	return s
}

// sameStack reports whether a row's line stack (and, when it is rich, the
// label and gate of each trailing action button) still matches the
// snapshot. A plain row carries no stack and always matches.
func sameStack(applied, row TableRow) bool {
	if !reflect.DeepEqual(applied.Stack, row.Stack) {
		return false
	}
	if len(row.Stack) == 0 {
		return true
	}
	if len(applied.Spec.Actions) != len(row.Spec.Actions) {
		return false
	}
	for i, a := range row.Spec.Actions {
		if applied.Spec.Actions[i].Label != a.Label || applied.Spec.Actions[i].Disabled != a.Disabled {
			return false
		}
	}
	return true
}

// copyStack deep-copies a row's line stack (segment runs included) so the
// snapshot can never be edited through the caller's slices.
func copyStack(s []RowLine) []RowLine {
	out := make([]RowLine, len(s))
	for i, l := range s {
		out[i] = l
		out[i].Segs = append([]RowSeg(nil), l.Segs...)
	}
	return out
}

// sameCellsIcons reports whether row's cell text and icons across the
// first n columns match the snapshot (an empty snapshot only matches an
// all-empty row).
func sameCellsIcons(applied, row TableRow, n int) bool {
	for j := 0; j < n; j++ {
		if cellOf(row, j) != cellOf(applied, j) || iconOf(row, j) != iconOf(applied, j) {
			return false
		}
	}
	return true
}

// newRow builds one clickable row container with one cell widget per
// column — or, for a rich row, its line stack. The click handler resolves
// the row by index at fire time, so deferred events always hit the
// currently displayed row (widgets are positional: row i's container never
// moves).
func (t *Table) newRow(i int) *widget.Container {
	if len(t.Rows[i].Stack) > 0 {
		return t.newStackRow(i)
	}
	ctr := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(0),
		)),
		widget.ContainerOpts.WidgetOpts(
			// Full width: the row background covers the whole strip even
			// when the columns underfill it.
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			widget.WidgetOpts.MouseButtonClickedHandler(t.rowHandler(i)),
		),
	)
	var texts []*widget.Text
	var cells []*tableCell
	for j, c := range t.cols {
		var runs []RowSeg
		if j < len(t.Rows[i].CellRuns) {
			runs = t.Rows[i].CellRuns[j]
		}
		if len(runs) > 0 {
			// A run cell: its segments in one line, the line anchored to
			// the cell's edge (right for a numeric column) so the runs
			// share the plain cell's exact anchor.
			pos := widget.AnchorLayoutPositionStart
			pad := widget.Insets{Left: theme.Px(cellPad), Right: theme.Px(cellPad)}
			if c.AlignRight {
				pos = widget.AnchorLayoutPositionEnd
			}
			line := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewRowLayout(
				widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
				widget.RowLayoutOpts.Spacing(0),
			)))
			for _, s := range runs {
				col := s.Tint
				if col.A == 0 {
					col = t.th.Pal().Text
					if tt := t.Rows[i].TextTint; tt.A != 0 {
						col = tt
					}
				}
				face := t.Face
				if s.Step != "" {
					face = t.th.Face(s.Step)
				}
				line.AddChild(widget.NewText(
					widget.TextOpts.Text(s.Text, face, theme.Solid(col)),
					widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
				))
			}
			line.GetWidget().LayoutData = widget.AnchorLayoutData{
				HorizontalPosition: pos,
				VerticalPosition:   widget.AnchorLayoutPositionCenter,
				Padding:            &pad,
			}
			cell := widget.NewContainer(
				widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
				widget.ContainerOpts.WidgetOpts(
					widget.WidgetOpts.MinSize(0, int(t.rowH)),
					widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
				),
			)
			cell.AddChild(line)
			texts = append(texts, nil) // runs own their colours; no label to tint
			cells = append(cells, &tableCell{ctr: cell, Box: theme.NewIconBox(theme.IconInline)})
			ctr.AddChild(cell)
			continue
		}
		// Left-aligned cells are icon cells: a fixed icon box then the
		// label, both inside the column box. Right-aligned cells stay
		// plain text so their numeric anchors never move — and they keep
		// the horizontal padding themselves, while an icon cell's lives
		// on its container.
		//
		// Right-cell padding (Defect A fix): ebitenui adds a Text's
		// horizontal Padding into its measured line width AND subtracts
		// Padding.Right again when drawing End-positioned text, so a
		// right-anchored glyph run lands Padding.Left+2×Padding.Right
		// left of the rect edge. With Right: 0 the anchor is exactly
		// rect.Max − cellPad — the same place the header label's
		// anchor padding puts it, so headers and cells share one x at
		// every scale (pinned by TestHeaderCellAnchorAgreement; the
		// quirk is ebitenui v0.7.3, re-verify if the dependency moves).
		align, pad := widget.TextPositionStart, widget.Insets{
			Top: theme.Px(entryPadV), Bottom: theme.Px(entryPadV),
		}
		if c.AlignRight {
			align = widget.TextPositionEnd
			pad = widget.Insets{Left: theme.Px(cellPad), Top: theme.Px(entryPadV), Bottom: theme.Px(entryPadV)}
		}
		tx := widget.NewText(
			widget.TextOpts.Text("", t.Face, t.textColor(t.Rows[i])),
			widget.TextOpts.Position(align, widget.TextPositionCenter),
			widget.TextOpts.Padding(&pad),
			widget.TextOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(0, int(t.rowH)),
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
		)
		if c.AlignRight {
			texts = append(texts, tx)
			cells = append(cells, nil)
			ctr.AddChild(tx)
			continue
		}
		cell := widget.NewContainer(
			widget.ContainerOpts.Layout(widget.NewRowLayout(
				widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
				widget.RowLayoutOpts.Spacing(theme.Px(theme.IconGap)),
				widget.RowLayoutOpts.Padding(&widget.Insets{
					Left: theme.Px(cellPad), Right: theme.Px(cellPad), Top: theme.Px(entryPadV), Bottom: theme.Px(entryPadV),
				}),
			)),
			widget.ContainerOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(0, int(t.rowH)),
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
		)
		box := theme.NewIconBox(theme.IconInline)
		cell.AddChild(box.Widget(), tx)
		texts = append(texts, tx)
		cells = append(cells, &tableCell{ctr: cell, Box: box})
		ctr.AddChild(cell)
	}
	t.Texts = append(t.Texts, texts)
	t.Cells = append(t.Cells, cells)
	t.rich = append(t.rich, nil)
	t.applied = append(t.applied, color.RGBA{})
	t.Tips = append(t.Tips, RowTip{})
	return ctr
}

// newStackRow builds a rich row: its line stack through the shared
// builder, wired with the same root click handling every row form gets —
// left selects (and runs the installer's double-click half against the
// row's spec), right opens the row's declared menu. The lines themselves
// are wired by the builder.
func (t *Table) newStackRow(i int) *widget.Container {
	sr := NewStackRow(NewSeams(t.th, t.cv, t.as), t.Rows[i], t.Face)
	click := t.rowHandler(i)
	sr.Root.GetWidget().LayoutData = widget.RowLayoutData{Stretch: true}
	sr.Root.GetWidget().MouseButtonClickedEvent.AddHandler(func(args any) {
		if a, ok := args.(*widget.WidgetMouseButtonClickedEventArgs); ok {
			click(a)
		}
	})
	t.Texts = append(t.Texts, nil)
	t.Cells = append(t.Cells, nil)
	t.rich = append(t.rich, sr)
	t.applied = append(t.applied, color.RGBA{})
	t.Tips = append(t.Tips, RowTip{})
	return sr.Root
}

// rowHandler is the root click wiring every row form shares (resolved by
// index at fire time, like the row widgets themselves).
func (t *Table) rowHandler(i int) widget.WidgetMouseButtonClickedHandlerFunc {
	return func(args *widget.WidgetMouseButtonClickedEventArgs) {
		switch args.Button {
		case ebiten.MouseButtonLeft:
			t.rowClicked(i)
		case ebiten.MouseButtonRight:
			t.rowMenu(i, ClickPoint(args))
		}
	}
}

// tableCell pairs a left-aligned cell's container with its icon box.
type tableCell struct {
	ctr *widget.Container
	Box *theme.IconBox
}

// cellRoot returns the widget that owns row r column j's Box: the icon
// cell container for left-aligned columns, the text itself otherwise. The
// anchor (MinWidth/fitted width) is pinned on this widget.
func (t *Table) cellRoot(r, j int) *widget.Widget {
	if r < 0 || r >= len(t.Cells) || j < 0 || j >= len(t.Cells[r]) {
		return nil
	}
	if c := t.Cells[r][j]; c != nil {
		return c.ctr.GetWidget()
	}
	return t.Texts[r][j].GetWidget()
}

// rowWidget returns the i-th row container, or nil.
func (t *Table) RowWidget(i int) *widget.Container {
	if i < 0 || i >= len(t.RowCts) {
		return nil
	}
	return t.RowCts[i]
}

// rowClicked selects the row and, on a repeat click inside the
// double-click window, runs the installer's double-click half against
// the row's declared spec — strictly, only a resolvable Ref opens Show
// Info (the spike's timing state — ebitenui has no double-click event).
func (t *Table) rowClicked(i int) {
	if i < 0 || i >= len(t.Rows) {
		return
	}
	row := t.Rows[i]
	prev := t.selIdx
	t.selIdx, t.selData = i, row.Data
	if prev != i {
		if prev >= 0 {
			t.updateBg(prev)
			t.updateIcon(prev)
		}
		t.updateBg(i)
		t.updateIcon(i)
	}
	if t.onSelect != nil {
		t.onSelect(row)
	}
	if t.onDouble != nil {
		t.dbl.Gesture(t.cv, row.Spec, func() { t.onDouble(row) })
		return
	}
	t.dbl.Click(t.cv, t.as, row.Spec)
}

// rowMenu resolves the right-clicked row (by index, the same way
// rowClicked does) and opens its declared menu — the installer derives
// the shape from the row's RowSpec. A right-click selects nothing — the
// menu acts on the row it was opened on (its Actions carry that row's
// payload), so the highlight never moves.
func (t *Table) rowMenu(i int, at image.Point) {
	if i < 0 || i >= len(t.Rows) {
		return
	}
	OpenRowMenu(t.cv, t.as, t.Rows[i].Spec, at)
}

// updateIcon refreshes one row's cell icons and their palette role. It runs
// alongside updateBg: selection moves the icon tint to the accent just as it
// moves the row background. A rich row's icon leads its line 1 (Icons[0]
// through the StackRow handle).
func (t *Table) updateIcon(i int) {
	if i < 0 || i >= len(t.Rows) {
		return
	}
	row := t.Rows[i]
	tint := t.iconTint(i, row)
	for j, c := range t.Cells[i] {
		if c != nil {
			c.Box.Set(iconOf(row, j), tint)
		}
	}
	if i < len(t.rich) && t.rich[i] != nil && t.rich[i].Icon != nil {
		t.rich[i].Icon.Set(iconOf(row, 0), tint)
	}
}

// updateBg paints the row background (selection accent wins over the
// row's own tint), skipping rows whose background is already painted.
func (t *Table) updateBg(i int) {
	want := t.rowTint(i)
	if i < len(t.applied) && want == t.applied[i] {
		return
	}
	if i < len(t.applied) {
		t.applied[i] = want
	}
	if ctr := t.RowWidget(i); ctr != nil {
		ctr.SetBackgroundImage(theme.NineSlice(theme.Solid(want)))
	}
}

// rowTint is the background the row should show right now.
func (t *Table) rowTint(i int) color.RGBA {
	if i == t.selIdx {
		return WithA(t.th.Pal().Accent, selectionAlpha)
	}
	if i < 0 || i >= len(t.Rows) {
		return color.RGBA{}
	}
	return t.Rows[i].Tint
}

// textColor is a row's cell text colour: the row's own TextTint when it
// declares one, else the palette's primary text.
func (t *Table) textColor(row TableRow) color.Color {
	if row.TextTint.A == 0 {
		return theme.Solid(t.th.Pal().Text)
	}
	return theme.Solid(row.TextTint)
}

// updateTextTint re-applies the row's text tint. It runs every pass because
// the tint can flip without a cell changing (a dimmed item row gaining its
// first order changes the Qty cell, but the reverse need not); SetColor only
// writes the draw colour, it never re-measures or relayouts. Rich rows draw
// their own segments and own no cell labels here.
func (t *Table) updateTextTint(i int) {
	if i < 0 || i >= len(t.Rows) || i >= len(t.Texts) || t.Texts[i] == nil {
		return
	}
	col := t.textColor(t.Rows[i])
	for _, tx := range t.Texts[i] {
		if tx != nil {
			tx.SetColor(col)
		}
	}
}

// scrollBy moves the viewport by notches wheel-notches of rows. The wheel
// convention is explicit: a positive notch is wheel up (ebiten's delta for
// a wheel rolled towards the user), which scrolls towards the TOP of the
// list — ScrollTop decreases towards 0 and earlier rows are revealed; a
// negative notch scrolls towards the end of the list (ScrollTop grows
// towards 1). ScrollTop is the 0..1 fraction of the content overflow, so
// the step is wheelRows rows of that overflow.
func (t *Table) scrollBy(notches float64) {
	over := t.scrollOverflow()
	if over <= 0 {
		// The rows fit the viewport: nothing to scroll and no stale
		// offset — ebitenui would otherwise bottom-anchor the list.
		t.scroll.ScrollTop = 0
		return
	}
	top := t.scroll.ScrollTop - notches*wheelRows*t.rowH/over
	t.scroll.ScrollTop = math.Max(0, math.Min(1, top))
}

// scrollOverflow is the scrollable distance in pixels: content height
// minus viewport height. It is <= 0 exactly when the rows fit the
// viewport, in which case the list is top-anchored (ScrollTop 0) — the
// content height comes from the body's preferred size, the same value
// renderContent positions with, so it stays correct between SetRows and
// the next layout pass.
func (t *Table) scrollOverflow() float64 {
	_, h := t.body.PreferredSize()
	return float64(h) - float64(t.scroll.ViewRect().Dy())
}

// clampScroll pins ScrollTop to the current overflow: 0 while the rows
// fit the viewport (a leftover offset would bottom-anchor the list), else
// within the 0..1 fraction. Rows are re-measured on every SetRows and the
// viewport can grow on window resize, so both paths call this.
func (t *Table) clampScroll() {
	if over := t.scrollOverflow(); over <= 0 {
		t.scroll.ScrollTop = 0
		return
	}
	t.scroll.ScrollTop = math.Max(0, math.Min(1, t.scroll.ScrollTop))
}

// applyWidths fits the columns into the available width and pushes the
// result into the header buttons and row cells. The natural measure runs
// once (ensureFrozen): every later fit — a resize, a sort click, a cell
// growing — feeds the frozen base back through the same policy, so content
// can never move an anchor.
func (t *Table) applyWidths() {
	avail := t.columnsWidth()
	t.ensureFrozen(avail)
	sub := make([]ColWidth, 0, len(t.cols))
	prio := make([]int, 0, len(t.cols))
	idx := make([]int, 0, len(t.cols))
	for i := range t.cols {
		if t.hidden[i] {
			continue
		}
		sub = append(sub, t.base[i])
		prio = append(prio, t.cols[i].Priority)
		idx = append(idx, i)
	}
	if avail > 0 && len(sub) > 0 {
		sub = fitColumns(sub, prio, growColumn(prio), avail)
	}
	t.fittedFor = avail
	t.widths = make([]float64, len(t.cols))
	for k, i := range idx {
		t.widths[i] = sub[k].Width
	}
	t.layoutCells()
}

// columnsWidth is the width the columns tile: the header strip less the ⋯
// button's own strip at its right end, so no column (and no header label)
// ever runs under the dots.
func (t *Table) columnsWidth() float64 {
	return float64(t.root.GetWidget().Rect.Dx()) - float64(theme.Px(ColMenuW))
}

// ensureFrozen measures every column's natural width (the header plus the
// widest cell of the current rows) and its floor ONCE — the first fit's
// tiling — and freezes the result. The first fit is exactly the algorithm
// the table always ran (over the column area, beside the ⋯ strip), so a
// first-open layout is unchanged; from then on the fit consumes the frozen
// base, and later content changes (a cell widening mid-warp, the sort
// arrow) only ever ellipsize.
func (t *Table) ensureFrozen(avail float64) {
	if t.frozen {
		return
	}
	natural := make([]ColWidth, len(t.cols))
	prio := make([]int, len(t.cols))
	for i, c := range t.cols {
		w := t.headMeasure(c.Title+t.indicator(i)) + 2*float64(theme.Px(cellPad))
		if c.Width > w {
			w = c.Width
		}
		for _, row := range t.Rows {
			// A run cell is measured by its drawn segments (the plain
			// string is only its sort key): the dim suffix counts.
			if i < len(row.CellRuns) && len(row.CellRuns[i]) > 0 {
				m := 2 * float64(theme.Px(cellPad))
				for _, s := range row.CellRuns[i] {
					face := t.Face
					if s.Step != "" {
						face = t.th.Face(s.Step)
					}
					rw, _ := text.Measure(s.Text, *face, 0)
					m += rw
				}
				if m > w {
					w = m
				}
				continue
			}
			m := t.measure(cellOf(row, i)) + 2*float64(theme.Px(cellPad))
			if iconOf(row, i) != "" && !c.AlignRight {
				m += float64(theme.Px(theme.IconInline) + theme.Px(theme.IconGap))
			}
			if m > w {
				w = m
			}
		}
		min := t.headMeasure(c.Title+t.indicator(i)) + 2*float64(theme.Px(cellPad))
		if c.MinWidth > min {
			min = c.MinWidth
		}
		natural[i] = ColWidth{Label: c.Title, Width: w, Min: min}
		prio[i] = c.Priority
	}
	t.base = natural
	// Nothing to freeze yet: the table has no width (built before its
	// first frame) or no rows (a list that opened empty). The natural
	// widths are today's provisional answer; the freeze lands on the
	// first fit that has content to measure.
	if avail <= 0 || len(t.Rows) == 0 {
		return
	}
	t.base = fitColumns(natural, prio, growColumn(prio), avail)
	t.frozen = true
	t.applyPrefs()
}

// layoutCells applies the fitted widths: cumulative rounding gives every
// column a fixed anchor (no jitter), the header buttons tile the strip
// exactly, and every cell gets its width plus an ellipsized label. Hidden
// columns are filtered out of layout and drawing by index — their cells
// stay where they are, so showing them again restores them untouched.
func (t *Table) layoutCells() {
	prev := 0
	acc := 0.0
	for i, c := range t.cols {
		acc += t.widths[i]
		w := int(math.Round(acc)) - prev
		prev += w
		vis := widget.Visibility_Show
		if t.hidden[i] {
			vis = widget.Visibility_Hide
		}

		b := t.hdrBtn[i]
		b.GetWidget().MinWidth = w
		b.GetWidget().LayoutData = widget.RowLayoutData{Stretch: true, MaxWidth: w}
		b.SetText(t.headerLabel(i))
		if t.sortKey != "" && t.sortKey == c.Key {
			b.SetImage(t.hdrOn)
		} else {
			b.SetImage(t.hdrIdle)
		}
		setVis(b.GetWidget(), vis)

		// Ellipsize against the float width: the int-rounded cell is up
		// to half a pixel narrower, and rounding it down here would
		// ellipsize text that measurably fits ("25%" -> "2…"). An icon
		// cell reserves its icon box out of the label's share. Rich rows
		// own their lines end to end — no per-column text widget, no
		// column anchor to pin; they stretch across the row instead.
		for r := range t.Rows {
			if r < len(t.rich) && t.rich[r] != nil {
				continue
			}
			inner := t.widths[i] - 2*float64(theme.Px(cellPad))
			if t.Cells[r][i] != nil && iconOf(t.Rows[r], i) != "" {
				inner -= float64(theme.Px(theme.IconInline) + theme.Px(theme.IconGap))
			}
			if inner < 0 {
				inner = 0
			}
			tx := t.Texts[r][i]
			root := t.cellRoot(r, i)
			if root != nil {
				setVis(root, vis)
			}
			if tx == nil || t.hidden[i] {
				// A run cell draws its own segments; a hidden column
				// keeps its label as it was (it is re-ellipsized when
				// the column comes back).
				continue
			}
			tx.Label = Ellipsize(cellOf(t.Rows[r], i), inner, t.measure)
			root.MinWidth = w
			root.LayoutData = widget.RowLayoutData{Stretch: true, MaxWidth: w}
		}
	}
	t.header.RequestRelayout()
	for _, ctr := range t.RowCts {
		ctr.RequestRelayout()
	}
	t.body.RequestRelayout()
}

// setVis sets w's visibility without a relayout when it is already right.
func setVis(w *widget.Widget, v widget.Visibility) {
	if w.GetVisibility() != v {
		w.SetVisibility(v)
	}
}

// headerLabel is the sort button's text: the title ellipsized into the
// column's inner width (the cells' own budget), with the sort arrow kept —
// a column squeezed after the freeze must still show which way it sorts.
func (t *Table) headerLabel(i int) string {
	ind := t.indicator(i)
	avail := t.widths[i] - 2*float64(theme.Px(cellPad)) - t.headMeasure(ind)
	if avail < 0 {
		avail = 0
	}
	return Ellipsize(t.cols[i].Title, avail, t.headMeasure) + ind
}

// indicator is the active sort column's arrow suffix. IBM Plex Sans ships
// no U+25B2/U+25BC triangles, so the arrows are the ↑/↓ it does have.
func (t *Table) indicator(i int) string {
	if t.sortKey == "" || t.cols[i].Key != t.sortKey {
		return ""
	}
	if t.sortAsc {
		return " ↑"
	}
	return " ↓"
}

// cellOf returns row cell i ("" when the row is short).
func cellOf(row TableRow, i int) string {
	if i < 0 || i >= len(row.Cells) {
		return ""
	}
	return row.Cells[i]
}

// sameData compares row payloads safely for any dynamic type.
func sameData(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return reflect.DeepEqual(a, b)
}

// iconTint picks a row icon's palette role: the selected row wears the
// accent, a row with its own tint (own ship → ColInfo, hostiles → ColDanger)
// keeps it, everything else is primary text.
func (t *Table) iconTint(i int, row TableRow) color.RGBA {
	if i == t.selIdx {
		return t.th.Pal().Accent
	}
	if row.IconTint.A != 0 {
		return row.IconTint
	}
	return t.th.Pal().Text
}

// withA returns c with a replacement alpha channel.
func WithA(c color.RGBA, a uint8) color.RGBA {
	return color.RGBA{R: c.R, G: c.G, B: c.B, A: a}
}

// growColumn is the column that receives surplus width: the lowest
// priority one (the most important column; first wins ties).
func growColumn(prio []int) int {
	best := 0
	for i := 1; i < len(prio); i++ {
		if prio[i] < prio[best] {
			best = i
		}
	}
	return best
}

// fitColumns fits natural column widths into avail pixels. When there is
// surplus space it all goes to the grow column; when space is short,
// columns shrink toward their Min by descending priority (ties: lower
// index first) and any residual overflow is left alone — the leftover
// columns ellipsize/clip rather than dropping the header.
// Upgrading this means a horizontal scrollbar.
func fitColumns(cols []ColWidth, prio []int, grow int, avail float64) []ColWidth {
	sum := 0.0
	for _, c := range cols {
		sum += c.Width
	}
	if sum <= avail {
		if grow >= 0 && grow < len(cols) {
			cols[grow].Width += avail - sum
		}
		return cols
	}
	deficit := sum - avail
	order := make([]int, len(cols))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return prio[order[a]] > prio[order[b]] })
	for _, i := range order {
		take := cols[i].Width - cols[i].Min
		if take <= 0 {
			continue
		}
		if take > deficit {
			take = deficit
		}
		cols[i].Width -= take
		deficit -= take
		if deficit <= 0 {
			return cols
		}
	}
	return cols
}

// ---------------------------------------------------------------------------
// Test hooks — the internals a surface test asserts geometry/state on
// from another package (kit_test, scale_test). Test-only seams, the
// SetPollCursorForTest precedent; production code never calls them.
// ---------------------------------------------------------------------------

// RowWidgetForTest exposes row i's container widget so a surface test
// can fire a real click at it (the suite's row-click helpers).
func (t *Table) RowWidgetForTest(i int) *widget.Widget { return t.RowCts[i].GetWidget() }

// RowClickedForTest runs row i's click handling directly (no event
// pump): the double-click/show-info tests drive it as the handler does.
func (t *Table) RowClickedForTest(i int) { t.rowClicked(i) }

// RowsForTest is the fitted display-order rows (TestDrawSmoke).
func (t *Table) RowsForTest() []TableRow { return t.Rows }

// WidthsForTest is the fitted column widths (TestOverviewMinSizeAndRefit).
func (t *Table) WidthsForTest() []float64 { return t.widths }

// HdrBtnForTest is the header buttons column i's width laid out (the
// refit and construction-metric tests).
func (t *Table) HdrBtnForTest() []*widget.Button { return t.hdrBtn }

// ScrollForTest is the scroll viewport (overflow assertions).
func (t *Table) ScrollForTest() *widget.ScrollContainer { return t.scroll }

// FitWidthForTest is the width the current column fit was computed for
// (0 = never fitted).
func (t *Table) FitWidthForTest() float64 { return t.fittedFor }

// SelIdxForTest is the selected row index (-1 = none).
func (t *Table) SelIdxForTest() int { return t.selIdx }
