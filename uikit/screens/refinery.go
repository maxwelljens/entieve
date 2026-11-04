package screens

import (
	"image"
	"image/color"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/sahilm/fuzzy"
)

// Refinery window metrics in design units: this window's own opening shares at
// the landing rect (the market and manufacturing panes declare theirs the same
// way). The row geometry is the manufacturing list's, since the ore list opens
// at the same 288 px between the panes.
const (
	rfTreeW   = 184
	rfInspW   = 264
	rfSearchH = 28
)

// rfDefaultW/H are the window's landing rect (see RefineryRect): the
// manufacturing board's box, a touch shorter because the ore list carries no
// job strip under it.
const (
	rfDefaultW = 760
	rfDefaultH = 520
)

// Refinery is the station refinery window: the ore groups as a browse tree
// with a live search (the manufacturing board's idiom, over ore groups), the
// ore list (every stack in the general cargo and the ore hold) whose rows
// carry the one-glance verdict and the batch's value-share strip, and the
// inspector for the selected ore: the item head, the money question ("refine
// this or sell it raw") priced off the docked book, and the whole-batch yield
// bill.
//
// The tree scopes the list to a branch (All is every held ore) and the search
// filters that branch live over the ore's name, its group and its OUTPUT
// MINERALS, so "sideron" finds every ore that yields it. Selecting an ore
// opens it in the inspector; Refine (the inspector's button) opens the
// quantity picker (CONTEXT.md "Quantity picker") on its notched ladder 100,
// 200, 300, … capped at that ore's stock. Refined minerals always land in the
// station hangar (never the ship's hold), which is unlimited, so nothing is
// warned.
//
// Gating follows the game's menu convention: controls stay VISIBLE and gray
// out (widget Disabled) while undocked, at a station without the refine
// service or holding less than one whole batch of the ore, never hidden. The
// row's verdict glyph and the inspector head carry the same state as one
// shape; no hint line names the reason (the manufacturing board's ruling).
// Every action enqueues a sim.Command (the kit never mutates sim state).
type Refinery struct {
	th  theme.Theme
	cv  widgets.ContentView
	as  widgets.ActionSink
	ptr *widgets.Pointer
	win *widget.Window

	ores *widgets.Table
	cols *widgets.SplitPane // browse | ores | inspector (seams refinery-tree, refinery-inspector)

	// The browse pane: the shared browse tree (its nodes, fold state, branch
	// and highlight) over the one-column table and the live search. group is
	// each ore's own group path, built once (content never changes under a
	// session).
	browseTree
	tree   *widgets.Table
	search *widget.TextInput
	group  map[content.ItemID]string
	query  string

	// The inspector: one grid rebuilt whenever its key moves (the selected
	// ore, the station gate, the holds, the pane's width or the book the money
	// table reads), the page in a scroll viewport with the house rail beside
	// it, Refine pinned below. The money table is built once and re-filled.
	insp       *widget.Container
	inspKey    string
	head       *itemHead
	math       *widgets.PlainTable
	billLbl    *widget.Text
	billBox    *widget.Container
	billScroll *widget.ScrollContainer
	billRail   *widgets.ScrollRail
	billRows   []*widgets.StackRow
	refineBtn  *widget.Button

	sel    refineryRow
	hasSel bool
}

// refineryRow is one ore row: the ore id plus its Show Info ref.
type refineryRow struct {
	ore content.ItemID
	ref widgets.InfoRef
}

// InfoRefOf resolves the row payload inside the seam's Show Info resolver.
func (r refineryRow) InfoRefOf(cv widgets.ContentView) (widgets.InfoRef, bool) {
	return r.ref, widgets.InfoRefExists(cv, r.ref)
}

// rfOut is one mineral of an ore's whole batch: the sim's floor(yield ×
// RefineBatch) amount and its base-price value — the manufacturing cost
// strip's own basis, so both strips stay content-priced until the owner says
// otherwise.
type rfOut struct {
	Item  content.ItemID
	Qty   int
	Value float64
}

// NewRefinery builds the refinery window over the shared environment. Mount
// it on a Kit with Kit.AddWindow and call Sync every frame.
func NewRefinery(env widgets.Seams) *Refinery {
	r := &Refinery{th: env, cv: env, as: env, ptr: env.Pointer(),
		group: map[content.ItemID]string{}}
	p := env.Pal()

	// The ore groups, in canonical item order (content never changes under a
	// session): the tree is a stable catalog of the ore branch, not a view of
	// the holds, so nodes never appear and vanish as cargo moves.
	var ores []content.ItemID
	for id, def := range env.Defs().Items {
		if def != nil && def.Category == content.CatOre {
			ores = append(ores, id)
			r.group[id] = def.Group
		}
	}
	slices.Sort(ores)
	r.browseTree = newBrowseTree(browseNodes(ores, r.group))

	// The ore list: one column of rich rows (ore, verdict, held count, value
	// strip), its cells only the fit source. The plate band reads through its
	// first rows like the old board's.
	r.ores = widgets.NewTable(env, []widgets.Column{{Key: "Ore", Title: "Ores"}},
		widgets.PrefsID("ores"), widgets.PlateBand(func() image.Rectangle {
			return headerPlateBand(r.win)
		}))
	r.ores.SetSortFunc(func(string, bool, []widgets.TableRow) {})

	// The ore list's selection opens the inspector (Refine is the inspector's
	// button, not the click) and highlights the ore's branch.
	r.ores.OnSelect(func(row widgets.TableRow) {
		r.search.Focus(false)
		if v, ok := row.Data.(refineryRow); ok && v.ore != "" {
			r.sel, r.hasSel = v, true
			r.treeSel = r.group[v.ore]
			r.highlightTree()
		}
	})

	// The browse pane: the house's live search field over the shared
	// one-column browse tree.
	r.search = newBrowseSearch(env, "Search ores or minerals", rfSearchH)
	r.tree = newBrowseTable(env, "ore-tree", "Browse")
	r.tree.OnSelect(func(row widgets.TableRow) {
		r.search.Focus(false)
		if n, ok := row.Data.(browseNodeKey); ok {
			r.selectBranch(n.Path)
		}
	})
	browse := paneStack([]bool{false, true}, r.search, r.tree.Container())

	// The inspector: a fixed three-row grid (the item head, the page
	// viewport, Refine), inset ContentsPad all round so the content never
	// touches the pane's edges. The page scrolls the money table and the
	// yield bill as one; Refine stays pinned below it, so a small window never
	// overlaps the two. Its content is rebuilt by syncInspector when it moves;
	// the grid itself never changes, so the row split's pane stays one widget.
	r.insp = widget.NewContainer(widget.ContainerOpts.Layout(widget.NewGridLayout(
		widget.GridLayoutOpts.Columns(1),
		widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, true, false}),
		widget.GridLayoutOpts.Spacing(0, theme.Px(theme.SpSm)),
		widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
	)))
	// The money table: the batch's ore value against its minerals as aligned
	// label/value rows (the profit table's decorless table).
	r.math = widgets.NewPlainTable(env, []widgets.PlainColumn{{}, {AlignRight: true}})

	// Body: the one row split (browse | ores | inspector, two seams), each
	// pane draggable and persisted in the layout sidecar.
	r.cols = widgets.NewSplitPaneRow(env, browse, r.ores.Container(), r.insp)
	r.cols.SetDefaultRatio(0, rfTreeShare())
	r.cols.SetDefaultRatio(1, 1-rfInspShare())

	body := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	body.AddChild(r.cols.Container())

	title, _ := theme.NewTitleBar(env, "Refinery", "svc_refinery")

	// The body panel is the header band's: it paints the panel, the refining
	// plate over it and then the body (docs/ART.md "Window headers").
	panel := theme.WindowBodyBox(p, theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200))

	r.win = widget.NewWindow(
		widget.WindowOpts.Contents(newHeaderBand(body, panel, render.RefiningHeader)),
		widget.WindowOpts.TitleBar(title, theme.Px(theme.TitleBarH)),
		// Content floor: the three panes (the inspector page is scrollable,
		// the rest is not).
		widget.WindowOpts.MinSize(theme.Px(640), theme.Px(420)),
		// Construction landing rect: the game centers the window on first
		// open (placeDefaults, AnchorCenter); the row declares the same rect
		// (RefineryRect) as its size.
		widget.WindowOpts.Location(pxRect(RefineryRect())),
	)
	return r
}

// RefineryRect is the refinery window's landing rect in design units: the one
// number its Screen row's DefaultRect and its constructor share. The origin is
// the 1280×720 design viewport's centre for the window's size, the way every
// centred Screen row declares it (placeDefaults re-centres anyway).
func RefineryRect() image.Rectangle {
	return image.Rect(260, 100, 260+rfDefaultW, 100+rfDefaultH)
}

// rfTreeShare / rfInspShare are the row split's opening ratios at the landing
// width: the split spans the window minus the body padding and both seam
// tracks (the manufacturing board's own seam maths). The seam stores the
// fraction BEFORE it.
func rfTreeShare() float64 {
	return float64(rfTreeW) / float64(rfDefaultW-2*theme.ContentsPad-2*widgets.SplitSeamH)
}

func rfInspShare() float64 {
	return float64(rfInspW) / float64(rfDefaultW-2*theme.ContentsPad-2*widgets.SplitSeamH)
}

// Win returns the refinery window for Kit.AddWindow.
func (r *Refinery) Win() *widget.Window { return r.win }

// seams repackages the environment for the widget helpers that take the union
// (NewStackRow, NewScrollRail).
func (r *Refinery) seams() widgets.Seams {
	return widgets.NewSeams(r.th, r.cv, r.as, r.ptr)
}

// Splits names the window's seams for the layout sidecar: the ore list's
// browse pane and its inspector.
func (r *Refinery) Splits() []widgets.Seam {
	return []widgets.Seam{
		{Name: "refinery-tree", Split: r.cols, Index: 0},
		{Name: "refinery-inspector", Split: r.cols, Index: 1},
	}
}

// Tables are the window's tables for the layout sidecar: the ore list and the
// browse tree are separate lists.
func (r *Refinery) Tables() []*widgets.Table { return []*widgets.Table{r.ores, r.tree} }

// headerPlateBand is the band a table's PlateBand knob reads: the window's
// artwork header rect, or empty when it carries no band or the build ships no
// plate — the tables then paint exactly as before (docs/ART.md).
func headerPlateBand(w *widget.Window) image.Rectangle {
	if w == nil {
		return image.Rectangle{}
	}
	hb := headerBandOf(w)
	if hb == nil || hb.plate == nil || hb.plate() == nil {
		return image.Rectangle{}
	}
	return headerBandRect(hb.GetWidget().Rect)
}

// oreStacks merges the general cargo and ore hold stacks into the ore list
// in first-seen order (duplicates summed) — the two places CmdRefine
// consumes from.
func (r *Refinery) oreStacks() []content.CargoStack {
	var out []content.CargoStack
	add := func(st content.CargoStack) {
		def, ok := r.cv.Defs().Item(st.Item)
		if !ok || def.Category != content.CatOre || st.Qty <= 0 {
			return
		}
		for i := range out {
			if out[i].Item == st.Item {
				out[i].Qty += st.Qty
				return
			}
		}
		out = append(out, st)
	}
	for _, st := range r.cv.CargoStacks() {
		add(st)
	}
	for _, st := range r.cv.OreStacks() {
		add(st)
	}
	return out
}

// scopeOres is the ore list narrowed to the selected branch: All is every held
// ore, any other branch its subtree.
func (r *Refinery) scopeOres() []content.CargoStack {
	all := r.oreStacks()
	out := make([]content.CargoStack, 0, len(all))
	for _, st := range all {
		if inBranch(r.group[st.Item], r.branch) {
			out = append(out, st)
		}
	}
	return out
}

// searchHits ranks the branch's ores over the ore name, its group path and the
// names of the minerals it yields, so a mineral query finds every ore that
// produces it (search by output, the inverse lookup of manufacturing's
// ingredient search). An ore is a hit when one field contains the query, then
// the hits are ordered by sahilm/fuzzy's score. The query and every field are
// diacritic-folded first (fold.go), so an ASCII keyboard matches accented
// names.
func (r *Refinery) searchHits(q string, scope []content.CargoStack) []content.CargoStack {
	fq, _ := foldString(q)
	lq := strings.ToLower(fq)
	ids := make([]content.CargoStack, 0, len(scope))
	hay := make([]string, 0, len(scope))
	for _, st := range scope {
		def, ok := r.cv.Defs().Item(st.Item)
		if !ok || def == nil {
			continue
		}
		fields := []string{r.cv.TradeableName(st.Item), r.group[st.Item]}
		for _, id := range yieldIDs(def) {
			if m, ok := r.cv.Defs().Item(id); ok && m != nil {
				fields = append(fields, m.Name)
			}
		}
		line, hit := "", false
		for _, fld := range fields {
			ff, _ := foldString(fld)
			line += " " + ff
			if strings.Contains(strings.ToLower(ff), lq) {
				hit = true
			}
		}
		if !hit {
			continue
		}
		ids = append(ids, st)
		hay = append(hay, line)
	}
	var out []content.CargoStack
	for _, m := range fuzzy.Find(fq, hay) {
		out = append(out, ids[m.Index])
	}
	return out
}

// yieldIDs is an ore's refine_yield minerals in canonical id order: the map is
// unordered and every list the window draws (strip, bill, search) walks this
// order, so a row never shuffles between frames.
func yieldIDs(def *content.ItemDef) []content.ItemID {
	if def == nil {
		return nil
	}
	ids := make([]content.ItemID, 0, len(def.RefineYield))
	for id := range def.RefineYield {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// batchOf is one ore's whole-batch conversion in canonical mineral-id order:
// the amounts the sim's own floor(yield × RefineBatch) math produces
// (refineOutputs, the picker's read too) with each mineral's base-price value.
func (r *Refinery) batchOf(ore content.ItemID) []rfOut {
	def, _ := r.cv.Defs().Item(ore)
	stacks := refineOutputs(r.cv, def, sim.RefineBatch)
	out := make([]rfOut, 0, len(stacks))
	for _, st := range stacks {
		o := rfOut{Item: st.Item, Qty: st.Qty}
		if m, ok := r.cv.Defs().Item(st.Item); ok && m != nil {
			o.Value = m.BasePrice * float64(o.Qty)
		}
		out = append(out, o)
	}
	return out
}

// refineOutputs is the exact conversion CmdRefine applies: floor(yield ×
// qty × the Refining edge) per mineral, in sorted mineral-id order, dropping
// zero yields. The sim computes it (the same function the command runs), so
// the preview cannot promise a batch the refinery would not pay.
func refineOutputs(cv widgets.ContentView, def *content.ItemDef, qty int) []content.CargoStack {
	return cv.RefineOutput(def, qty)
}

// outputsText renders an output list as "45 Sideron · 30 Nixium" ("" for
// an empty list).
func outputsText(cv widgets.ContentView, outs []content.CargoStack) string {
	s := ""
	for i, o := range outs {
		if i > 0 {
			s += " · "
		}
		s += theme.FormatInt(int64(o.Qty)) + " " + cv.TradeableName(o.Item)
	}
	return s
}

// oneBatchLine is the sub-batch wording the picker shares: a stock under one
// whole batch has no valid refine quantity.
func oneBatchLine(cv widgets.ContentView, ore content.ItemID, stock int) string {
	return "One batch needs " + theme.FormatInt(int64(sim.RefineBatch)) + " ore; you hold " +
		theme.FormatInt(int64(stock)) + " " + cv.TradeableName(ore) + "."
}

// Sync refreshes the tree, the ore list, the inspector and every gate from the
// simulation. Call it every frame before Kit.Update.
func (r *Refinery) Sync() {
	r.cols.Poll(r.ptr)
	p := r.th.Pal()

	// The ore list: the branch (or its ranked search hits), one rich row per
	// held ore.
	r.query = strings.TrimSpace(r.search.GetText())
	scope := r.scopeOres()
	list := scope
	if r.query != "" {
		list = r.searchHits(r.query, scope)
	}
	rows := make([]widgets.TableRow, 0, len(list))
	for _, st := range list {
		rows = append(rows, r.oreRow(st))
	}
	if len(rows) == 0 {
		empty := "No ore held"
		if r.query != "" || r.branch != "" {
			empty = "No matches"
		}
		rows = []widgets.TableRow{{Cells: []string{empty}, TextTint: p.Dim}}
	}
	r.ores.SetRows(rows)
	r.ores.Sync()

	// Revalidate the cached selection against the rows themselves (an ore
	// that left the holds must stop gating the button).
	r.hasSel = false
	if sel, ok := r.ores.Selected(); ok {
		if row, isRow := sel.Data.(refineryRow); isRow && row.ore != "" {
			r.sel, r.hasSel = row, true
		}
	}
	if !r.hasSel {
		r.sel = refineryRow{}
	}

	// The browse pane: the branch rows and the highlighted branch.
	r.tree.SetRows(r.treeRows())
	r.tree.SelectData(browseNodeKey{Path: r.treeSel})
	r.tree.Sync()

	// The inspector and its refine gate (visible-but-grayed, never hidden).
	outs := r.batchOf(r.sel.ore)
	station := r.cv.DockedMarket()
	ore := r.cv.Quote(station, r.sel.ore, sim.OrderSell, sim.RefineBatch)
	mine := make([]sim.Quote, len(outs))
	for i, o := range outs {
		mine[i] = r.cv.Quote(station, o.Item, sim.OrderSell, o.Qty)
	}
	r.syncInspector(r.cv.BookOrders(station), outs, ore, mine)
	if r.refineBtn != nil {
		r.refineBtn.GetWidget().Disabled = !r.refineReadyFor(r.sel)
	}

	// The page viewport rides the house rail beside it (the tables' clamp
	// rule: a stale offset would bottom-anchor a page that fits).
	if r.billRail != nil {
		r.billRail.Sync()
		if r.billRail.Overflow() <= 0 {
			r.billScroll.ScrollTop = 0
		}
	}
}

// oreRow is one held ore as a rich row: the ore and its one-glance verdict
// glyph on line 1, the held count and the batch's value-share strip on line 2.
// The icon is the market list's item icon size. The cells are only the row's
// fit source (the list has one column).
func (r *Refinery) oreRow(st content.CargoStack) widgets.TableRow {
	p := r.th.Pal()
	row := refineryRow{ore: st.Item}
	row.ref, _ = widgets.ResolveTradeable(r.cv.Defs(), st.Item).Ref()
	ready := r.refineReadyFor(row)
	tint := p.Danger
	if ready {
		tint = p.Good
	}
	name := r.cv.TradeableName(st.Item)
	return widgets.TableRow{
		Cells:    []string{name},
		Icons:    []string{widgets.TradeableIcon(r.cv.Defs(), st.Item)},
		IconSize: mktRowIcon,
		Data:     row,
		Spec:     widgets.RowSpec{Ref: &row.ref},
		Stack: []widgets.RowLine{
			{Segs: []widgets.RowSeg{
				{Text: mfgClip(name, mfgNameClip), Tint: p.Accent, Step: theme.StepSmall},
				{Text: verdictGlyph(ready), Tint: tint, Step: theme.StepSmall},
			}},
			{Segs: []widgets.RowSeg{
				{Text: theme.FormatInt(int64(st.Qty)) + " held", Tint: p.Dim, Step: theme.StepTiny},
				{Split: yieldStrip(r.batchOf(st.Item), p), MinW: theme.Px(mfgStripW)},
			}},
		},
	}
}

// yieldStrip is an ore row's whole-batch value split: one segment per output
// mineral in canonical id order, its width the mineral's share of the batch's
// value (base prices, the manufacturing cost strip's basis) and its tint the
// next stat family — outputs carry no have/need state, so the hues keep the
// segments readable and no new palette role is minted.
func yieldStrip(outs []rfOut, p theme.Palette) []widgets.RowSplit {
	fams := []theme.StatCategory{
		theme.StatEconomy, theme.StatUtility, theme.StatMining,
		theme.StatCapacitor, theme.StatFitting, theme.StatMobility, theme.StatDefence,
	}
	out := make([]widgets.RowSplit, 0, len(outs))
	for i, o := range outs {
		out = append(out, widgets.RowSplit{Share: o.Value, Tint: theme.StatColor(p, fams[i%len(fams)])})
	}
	return out
}

// syncInspector rebuilds the inspector when its content moved: the selected
// ore, the station's refine gate, the holds, the pane's width or the book rows
// its money table reads.
func (r *Refinery) syncInspector(book []sim.MarketStock, outs []rfOut, ore sim.Quote, mine []sim.Quote) {
	inner := r.inspInnerW()
	key := r.inspKeyFor(book, outs, inner)
	if key == r.inspKey {
		return
	}
	r.inspKey = key
	r.insp.RemoveChildren()
	r.billRows = r.billRows[:0]
	r.refineBtn = nil

	p := r.th.Pal()

	// Head: the one item head (itemhead.go) the market's right pane and the
	// manufacturing inspector lead with, its hint line alone while nothing is
	// selected (the window's empty state).
	r.head = newItemHead(r.seams(), "Select an ore to refine.")
	if r.hasSel {
		ready := r.refineReadyFor(r.sel)
		r.head.set(true, widgets.TradeableIcon(r.cv.Defs(), r.sel.ore),
			r.cv.TradeableName(r.sel.ore)+"  "+verdictGlyph(ready),
			itemProse(r.cv, r.sel.ore), inner)
	} else {
		r.head.set(false, "", "", "", inner)
	}
	r.math.SetRows(r.refineRows(ore, mine))

	// The page: the money table's bar and rows, then the yield bill, one row
	// per mineral, the most valuable first. The page's children keep their own
	// heights, so the box is a plain vertical run, not an equal-row stack.
	r.billLbl = widget.NewText(
		widget.TextOpts.Text("YIELD · most value first", r.th.Face(theme.StepTiny), theme.Solid(p.Accent)),
	)
	r.billBox = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.SpSm)),
		)),
	)
	r.billBox.AddChild(sectionBar(r.th, "PROFIT"), r.math.Container(), r.billLbl)
	bill := billOrder(outs)
	total := 0.0
	for _, o := range bill {
		total += o.Value
	}
	for _, o := range bill {
		row := r.yieldRow(o, total, p)
		r.billRows = append(r.billRows, row)
		r.billBox.AddChild(row.Root)
	}
	r.billScroll, r.billRail = newScrollColumn(r.seams(), r.billBox)
	r.billScroll.GetWidget().ScrolledEvent.AddHandler(func(args any) {
		if ev, ok := args.(*widget.WidgetScrolledEventArgs); ok {
			r.scrollBill(ev.Y)
		}
	})

	r.refineBtn = theme.ActionButton("Refine", r.refineClicked)
	r.refineBtn.GetWidget().Disabled = true // armed by Sync's gate
	refineBar := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
		)),
	)
	refineBar.AddChild(r.refineBtn)

	r.insp.AddChild(r.head.headBox, r.billRail.Container(), refineBar)
	r.insp.RequestRelayout()
}

// inspInnerW is the head's text width inside the pane's ContentsPad inset: the
// page viewport's own width plus the rail gutter (the head spans the gutter
// the bill gives up), or the landing width before the first layout.
func (r *Refinery) inspInnerW() int {
	if r.billScroll != nil {
		if rect := r.billScroll.ViewRect(); rect.Dx() > 0 {
			return rect.Dx() + theme.Px(theme.ScrollGutter)
		}
	}
	return theme.Px(rfInspW - 2*theme.ContentsPad)
}

// billOrder is the yield bill's reading order: the batch's minerals by value,
// most first (the cost driver's own rule, the bill's acceptance read), ties in
// canonical id order so a row never swaps between frames.
func billOrder(outs []rfOut) []rfOut {
	out := append([]rfOut(nil), outs...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Value > out[j].Value })
	return out
}

// yieldRow is one mineral of the batch as a built rich row.
func (r *Refinery) yieldRow(o rfOut, total float64, p theme.Palette) *widgets.StackRow {
	return widgets.NewStackRow(r.seams(), r.yieldRowDef(o, total, p), r.th.Face(theme.StepSmall))
}

// yieldRowDef is one mineral of the batch: its own art at the bill's icon box
// and name, its share of the batch's mineral value (line 1), the sim's amount
// per batch and the mineral's base-price value (line 2). No have/need bar: a
// batch's outputs are not a commitment, so the second line carries the
// numbers the material bill draws as a bar.
func (r *Refinery) yieldRowDef(o rfOut, total float64, p theme.Palette) widgets.TableRow {
	name := string(o.Item)
	if def, ok := r.cv.Defs().Item(o.Item); ok {
		name = def.Name
	}
	share := 0.0
	if total > 0 {
		share = o.Value / total
	}
	return widgets.TableRow{
		Cells:    []string{name},
		Icons:    []string{widgets.TradeableIcon(r.cv.Defs(), o.Item)},
		IconSize: mfgMatIcon,
		Stack: []widgets.RowLine{
			{Segs: []widgets.RowSeg{
				{Text: name, MinW: theme.Px(mfgMatNameW), Step: theme.StepSmall},
				{Text: theme.PctLabel(share), MinW: theme.Px(34), Step: theme.StepTiny, Tint: p.Dim},
			}},
			{Segs: []widgets.RowSeg{
				{Text: theme.FormatInt(int64(o.Qty)) + " per batch", MinW: theme.Px(mfgMatBarW),
					Step: theme.StepTiny, Tint: p.Text},
				{Text: mfgMoney(o.Value), Step: theme.StepTiny, Tint: p.Dim},
			}},
		},
	}
}

// refineRows is the inspector's money table: one whole batch sold raw against
// its minerals, both sides the sim's quote walk over the docked book's bids
// (CONTEXT.md "Quote") — the question "refine this or sell it raw" in four
// lines. There is no fee line: the sim charges none. A side the book holds no
// bid for dashes with the marker, never a content base price (the profit
// table's rule), and the two derived lines dash with it.
func (r *Refinery) refineRows(ore sim.Quote, mine []sim.Quote) []widgets.PlainRow {
	p := r.th.Pal()
	row := func(label, value string, tint color.RGBA) widgets.PlainRow {
		return widgets.PlainRow{Cells: []string{label, value}, Tint: tint}
	}
	if !r.hasSel {
		out := make([]widgets.PlainRow, 0, 4)
		for _, label := range []string{"Ore value", "Minerals value", "Refine delta", "Margin"} {
			out = append(out, row(label, "—", p.Dim))
		}
		return out
	}

	raw, hasOre := ore.Walk, ore.Priced
	total, hasMine := 0.0, len(mine) > 0
	for _, q := range mine {
		if !q.Priced {
			hasMine = false
			break
		}
		total += q.Walk
	}
	oreCell, oreTint := mfgMoney(raw), color.RGBA{}
	mineCell, mineTint := mfgMoney(total), color.RGBA{}
	if !hasOre {
		oreCell, oreTint = "— · no buy order", p.Dim
	}
	if !hasMine {
		mineCell, mineTint = "— · no buy order", p.Dim
	}
	out := []widgets.PlainRow{
		row("Ore value", oreCell, oreTint),
		row("Minerals value", mineCell, mineTint),
	}
	if !hasOre || !hasMine {
		return append(out,
			row("Refine delta", "—", p.Dim),
			row("Margin", "—", p.Dim))
	}
	delta := total - raw
	tint := p.Good
	if delta < 0 {
		tint = p.Danger
	}
	margin := "—"
	if total > 0 {
		margin = theme.PctLabel(delta / total)
	}
	return append(out,
		row("Refine delta", mfgSigned(delta), tint),
		row("Margin", margin, tint))
}

// inspKeyFor is the inspector's change signature: the selection, the refine
// gate, the holds, the pane's width (the head re-wraps with the seam) and the
// book rows the money table reads. A book move rebuilds the card; an idle book
// does not.
func (r *Refinery) inspKeyFor(book []sim.MarketStock, outs []rfOut, inner int) string {
	var b strings.Builder
	b.WriteString(string(r.sel.ore))
	b.WriteByte(';')
	if r.refineReadyFor(r.sel) {
		b.WriteByte('r')
	}
	b.WriteString(strconv.Itoa(r.stockOf(r.sel.ore)))
	b.WriteByte(';')
	b.WriteString(strconv.Itoa(inner))
	b.WriteByte(';')
	priced := func(item content.ItemID) bool {
		if item == r.sel.ore {
			return true
		}
		for _, o := range outs {
			if o.Item == item {
				return true
			}
		}
		return false
	}
	for _, o := range book {
		if !priced(o.Item) {
			continue
		}
		b.WriteString(string(o.Item))
		b.WriteByte(':')
		b.WriteString(string(o.Side))
		b.WriteByte(':')
		b.WriteString(strconv.FormatFloat(o.Price, 'g', -1, 64))
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(o.Qty))
		b.WriteByte(';')
	}
	return b.String()
}

// scrollBill turns wheel notches into the page viewport's offset: two rows of
// the measured row height per notch, the same 0..1-fraction-of-overflow
// convention the tables use.
func (r *Refinery) scrollBill(notches float64) {
	rowH := 0
	if len(r.billRows) > 0 {
		_, rowH = r.billRows[0].Root.PreferredSize()
	}
	scrollNotches(r.billScroll, r.billRail, rowH, 2, notches)
}

// stockOf is the units of one ore in the two holds (the sim's own
// availability read).
func (r *Refinery) stockOf(ore content.ItemID) int {
	n := 0
	for _, st := range r.oreStacks() {
		if st.Item == ore {
			n += st.Qty
		}
	}
	return n
}

// refineReadyFor is the Refine gate: a selected ore, the docked station's
// refine service and at least one whole batch of that ore in the holds.
func (r *Refinery) refineReadyFor(row refineryRow) bool {
	if row.ore == "" || !refineService(r.cv) {
		return false
	}
	return batchSteps(r.stockOf(row.ore)) > 0
}

// refineService reports whether the docked station offers refine (in space
// the sim's own stationAccess gate refuses).
func refineService(cv widgets.ContentView) bool {
	info, ok := cv.DockedStation()
	return ok && slices.Contains(info.Services, "refine")
}

// batchSteps is the number of whole batches an ore holding stock units
// has — the picker's ladder length, zero below a batch.
func batchSteps(stock int) int {
	return stock / sim.RefineBatch
}

// FocusSearch focuses the browse pane's search field (a click on the field
// does the same).
func (r *Refinery) FocusSearch() { r.search.Focus(true) }

// openPicker opens the quantity picker on an ore: the Refine gesture (the
// inspector's button). Nothing opens away from a refine station.
func (r *Refinery) openPicker(row refineryRow) {
	if row.ore == "" || !refineService(r.cv) {
		return
	}
	r.as.Do(widgets.QtyPick{Mode: widgets.QtyRefine, Item: row.ore})
}

// refineClicked opens the picker on the inspected ore. Grayed is grayed: a
// disabled button opens nothing, even when a harness fires the click past the
// gate.
func (r *Refinery) refineClicked() {
	if !r.refineReadyFor(r.sel) {
		return
	}
	r.openPicker(r.sel)
}
