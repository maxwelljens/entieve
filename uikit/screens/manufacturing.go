package screens

import (
	"image"
	"image/color"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/sahilm/fuzzy"
)

// Manufacturing window metrics in design units.
const (
	// mfgTreeW / mfgInspW are the browse and inspector panes' opening
	// widths at the landing rect: the row split's opening shares, not fixed
	// widths: the seams trade them with the recipe list.
	mfgTreeW   = 184
	mfgInspW   = 264
	mfgSearchH = 28

	// mfgBoardShare is the vertical seam's opening share: the board (tree,
	// list, inspector) over the job list: the jobs keep the history strip
	// at the bottom, the new panes take the room.
	mfgBoardShare = 0.68

	// mfgNameClip is a recipe row's name budget in runes: the list's 288 px
	// opening fits 20 runes of the small face beside the row icon and the
	// verdict, and a rich row never ellipsizes its own text.
	// shortcut: a rune budget, not a measured fit; upgrade to a face measure
	// if the list pane or the row's faces change.
	mfgNameClip = 20

	// mfgStripW is the recipe row's value-share strip width: the segment
	// widths always sum to exactly this (widgets.ShareWidths).
	mfgStripW = 128
	// mfgMatNameW / mfgMatBarW are a material row's name column and have/need
	// bar widths, so every row's bar shares one left edge and one length. The
	// name column is sized to keep the 48 px icon's row inside the inspector
	// at the window's floor (the bill's own scroll viewport).
	mfgMatNameW = 80
	mfgMatBarW  = 96

	// mfgMatIcon is the material bill's icon box: three times the inline row
	// icon (16 design px) the bill opened with, so the mineral's art reads at
	// a glance (the owner's size; the row height follows the box).
	mfgMatIcon = 48
)

// mfgDefaultW/H are the window's landing rect (see ManufacturingRect):
// wider than tall for three panes at the market's precedent, still inside
// the 1024×700 minimum right of the rail gutter.
const (
	mfgDefaultW = 760
	mfgDefaultH = 560
)

// Manufacturing is the station manufacturing window: the content's recipe
// products as a browse tree (the market window's own browse column over
// recipe groups) with a live search, a recipe list whose rows carry the
// one-glance verdict and a value-share strip, and an inspector for the
// selected recipe (the item head, the market-priced profit table and the
// material bill), over the GLOBAL job list (every station, grouped by
// station) with per-job progress and the Collect / Cancel actions.
//
// The browse tree scopes the list to a branch (All is the whole catalog) and
// the search field filters that branch live, ranked by fuzzy score over the
// product name, its group, its family and its inputs, so "sideron" finds
// every recipe that eats Sideron. Selecting a recipe highlights its branch
// back and opens it in the inspector; Start (the inspector's button or the
// row's menu action) opens the quantity picker on its runs (CONTEXT.md
// "Quantity picker"), whose Confirm emits StartJob.
//
// Gating follows the game's menu convention: controls stay VISIBLE and
// gray out (widget Disabled) when the player is undocked, the station
// offers no industry service, the materials or wallet are short or the
// slots are full, never hidden: the row and inspector verdicts carry a tick
// (Start would accept) or a cross (it would refuse), and the bill's Danger
// rows name which minerals are short. Jobs at another
// station are listed but only the docked station's own jobs can be
// collected or cancelled. Every action enqueues a sim.Command (the kit
// never mutates sim state); the sim logs the refusal line if a gated action
// still gets through.
type Manufacturing struct {
	th  theme.Theme
	cv  widgets.ContentView
	as  widgets.ActionSink
	ptr *widgets.Pointer
	win *widget.Window

	recipes *widgets.Table
	jobs    *widgets.Table
	split   *widgets.SplitPane // board over jobs (seam manufacturing-jobs)
	cols    *widgets.SplitPane // browse | list | inspector

	jobsLbl *widget.Text
	title   *theme.ChromeText

	// The browse pane: the shared browse tree (its nodes, fold state, branch
	// and highlight) over the one-column table and the live search. group is
	// each recipe's product group path, built once (content never changes
	// under a session).
	browseTree
	tree   *widgets.Table
	search *widget.TextInput
	group  map[content.RecipeID]string
	query  string
	// stations is the universe's market roster (the fallback ladder's read).
	stations []sim.StationID

	// The inspector: one grid rebuilt whenever its key moves (the selected
	// recipe, the station, the slots, the pane's width or the book the
	// profit table reads), the material rows in a scroll viewport with the
	// house rail beside it. The profit table is built once and re-filled.
	insp       *widget.Container
	inspKey    string
	head       *itemHead
	profit     *widgets.PlainTable
	profitNote *widget.Text
	matsLbl    *widget.Text
	matsBox    *widget.Container
	matsScroll *widget.ScrollContainer
	matsRail   *widgets.ScrollRail
	matsRows   []*widgets.StackRow
	startBtn   *widget.Button

	collect, cancel *widget.Button

	sel    manufacturingRow
	hasSel bool
}

// mfgNeed is one mineral of one run's material bill: the sim's committed
// need, the player's have side and the mineral's cost weight (base price ×
// need, the number the fee derives from).
type mfgNeed struct {
	Item  content.ItemID
	Need  int
	Have  int
	Value float64
}

// manufacturingRow is the manufacturing tables' row payload: the recipe id
// (recipe rows) or job id (job rows) plus the product ref both resolve Show
// Info through (InfoRefOf handles the type; see infowin.go).
type manufacturingRow struct {
	recipe content.RecipeID
	job    uint64
	ref    widgets.InfoRef
}

// InfoRefOf resolves the row payload inside the seam's Show Info
// resolver (widgets/infoRefPayload): the row already carries its ref.
func (r manufacturingRow) InfoRefOf(cv widgets.ContentView) (widgets.InfoRef, bool) {
	return r.ref, widgets.InfoRefExists(cv, r.ref)
}

// NewManufacturing builds the Manufacturing window over the shared environment. Mount
// it on a Kit with Kit.AddWindow and call Sync every frame.
func NewManufacturing(env widgets.Seams) *Manufacturing {
	f := &Manufacturing{
		th: env, cv: env, as: env, ptr: env.Pointer(),
		expanded: map[string]bool{"": true},
		group:    map[content.RecipeID]string{},
	}
	p := env.Pal()

	// The recipe groups, in the content's own canonical recipe order.
	recipes := env.Defs().RecipesInOrder()
	ids := make([]content.RecipeID, 0, len(recipes))
	for _, rec := range recipes {
		ids = append(ids, rec.ID)
		f.group[rec.ID] = mfgGroup(env.Defs(), rec.Product)
	}
	f.browseTree = newBrowseTree(browseNodes(ids, f.group))
	f.stations = marketStations(env.Defs())

	// The recipe list: one column of rich rows (product, verdict, cost strip),
	// its cells only the fit source. The plate band reads through its first
	// rows like the old board's.
	f.recipes = widgets.NewTable(env, []widgets.Column{{Key: "Recipe", Title: "Recipes"}},
		widgets.PrefsID("recipes"), widgets.PlateBand(func() image.Rectangle {
			return headerPlateBand(f.win)
		}))
	f.recipes.SetSortFunc(func(string, bool, []widgets.TableRow) {})

	jobCols := []widgets.Column{
		{Key: "Product", Title: "Product", Priority: 0},
		{Key: "Station", Title: "Station", Priority: 1},
		{Key: "Runs", Title: "Runs", Priority: 4, AlignRight: true},
		{Key: "Progress", Title: "Progress", Priority: 2, AlignRight: true},
		{Key: "State", Title: "State", Priority: 1},
	}
	f.jobs = widgets.NewTable(env, jobCols, widgets.PrefsID("jobs"))

	// Canonical display order: the sim's own job-id order (reversing on a
	// second header click, any other header click falls back to plain cell
	// sorting (the market table's rule).
	f.jobs.SetSort("Product", true)
	f.jobs.SetSortFunc(canonicalFirst("Product", jobCols))

	// The recipe list's selection opens the inspector (the picker is the
	// Start gesture, not the click). Its declared RowSpec still carries the
	// gated Start action, so the menu and double-click work per row.
	f.recipes.OnSelect(func(r widgets.TableRow) {
		f.search.Focus(false)
		if row, ok := r.Data.(manufacturingRow); ok && row.recipe != "" {
			f.sel, f.hasSel = row, true
			f.treeSel = f.group[row.recipe]
			f.highlightTree()
		}
	})

	// The browse pane: the house's live search field over the shared
	// one-column browse tree.
	f.search = newBrowseSearch(env, "Search recipes or minerals", mfgSearchH)
	f.tree = newBrowseTable(env, "recipe-tree", "Browse")
	f.tree.OnSelect(func(r widgets.TableRow) {
		f.search.Focus(false)
		if n, ok := r.Data.(browseNodeKey); ok {
			f.selectBranch(n.Path)
		}
	})
	browse := paneStack([]bool{false, true}, f.search, f.tree.Container())

	// The inspector: a fixed three-row grid (the item head, the body
	// viewport, Start), inset ContentsPad all round so the content never
	// touches the pane's edges. The body viewport scrolls the
	// profit table and the material bill as one page; Start stays pinned
	// below it, so a small window never overlaps the two. Its content is
	// rebuilt by syncInspector when the bill moves; the grid itself never
	// changes, so the row split's pane stays one widget.
	f.insp = widget.NewContainer(widget.ContainerOpts.Layout(widget.NewGridLayout(
		widget.GridLayoutOpts.Columns(1),
		widget.GridLayoutOpts.Stretch([]bool{true},
			[]bool{false, true, false}),
		widget.GridLayoutOpts.Spacing(0, theme.Px(theme.SpSm)),
		widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
	)))
	// The profit table: one run's cost math as aligned label/value rows (the
	// Finances holdings' decorless table; the label column stretches, the
	// value column ends on the table's right edge like every figure here).
	f.profit = widgets.NewPlainTable(env, []widgets.PlainColumn{{}, {AlignRight: true}})

	// Job list: the PLAYER's jobs at every station (the global slot pool).
	f.jobsLbl = widget.NewText(
		widget.TextOpts.Text("Jobs", env.Face(theme.StepTiny), theme.Solid(p.Accent)),
	)
	f.collect = theme.ActionButton("Collect", f.collectClicked)
	f.cancel = theme.ActionButton("Cancel", f.cancelClicked)
	collectBar := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.ContentsPad)),
		)),
	)
	collectBar.AddChild(f.collect, f.cancel)

	// Body: the two splits: the board row (browse | list | inspector, two
	// seams) over the job list (one seam). The board takes the larger
	// share; every seam is draggable and persists in the layout sidecar.
	f.cols = widgets.NewSplitPaneRow(env, browse, f.recipes.Container(), f.insp)
	f.cols.SetDefaultRatio(0, mfgTreeShare())
	f.cols.SetDefaultRatio(1, 1-mfgInspShare())
	f.split = widgets.NewSplitPane(env,
		paneStack([]bool{true}, f.cols.Container()),
		paneStack([]bool{false, true, false}, f.jobsLbl, f.jobs.Container(), collectBar),
	)
	f.split.SetDefaultRatio(0, mfgBoardShare)

	body := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	body.AddChild(f.split.Container())

	title, titleText := theme.NewTitleBar(env, "Manufacturing", "svc_manufacturing")
	f.title = titleText

	// The body panel is the header band's: it paints the panel, the
	// manufacturing plate over it and then the body (docs/ART.md "Window
	// headers").
	panel := theme.WindowBodyBox(p, theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200))

	f.win = widget.NewWindow(
		widget.WindowOpts.Contents(newHeaderBand(body, panel, render.ManufacturingHeader)),
		widget.WindowOpts.TitleBar(title, theme.Px(theme.TitleBarH)),
		// Content floor: the board's three panes (the material bill is
		// scrollable, the rest is not) and the job strip.
		widget.WindowOpts.MinSize(theme.Px(640), theme.Px(460)),
		// Construction landing rect: the game centers the window on first
		// open (placeDefaults, AnchorCenter); the row declares the same
		// rect (ManufacturingRect) as its size.
		widget.WindowOpts.Location(pxRect(ManufacturingRect())),
	)
	return f
}

// ManufacturingRect is the manufacturing window's landing rect in design
// units: the one number its Screen row's DefaultRect and its constructor
// share.
func ManufacturingRect() image.Rectangle {
	return image.Rect(140, 60, 140+mfgDefaultW, 60+mfgDefaultH)
}

// mfgTreeShare / mfgInspShare are the row split's opening ratios at the
// landing width: the split spans the window minus the body padding and both
// seam tracks (the market's own seam maths). The seam stores the fraction
// BEFORE it.
func mfgTreeShare() float64 {
	return float64(mfgTreeW) / float64(mfgDefaultW-2*theme.ContentsPad-2*widgets.SplitSeamH)
}

func mfgInspShare() float64 {
	return float64(mfgInspW) / float64(mfgDefaultW-2*theme.ContentsPad-2*widgets.SplitSeamH)
}

// Win returns the manufacturing window for Kit.AddWindow.
func (f *Manufacturing) Win() *widget.Window { return f.win }

// seams repackages the three the window holds as one value, for the widget
// helpers that take the union (NewStackRow, NewScrollRail).
func (f *Manufacturing) seams() widgets.Seams {
	return widgets.NewSeams(f.th, f.cv, f.as, f.ptr)
}

// Splits names the window's seams for the layout sidecar: the board over the
// job list, and the board row's browse | list | inspector pair.
func (f *Manufacturing) Splits() []widgets.Seam {
	return []widgets.Seam{
		{Name: "manufacturing-jobs", Split: f.split, Index: 0},
		{Name: "manufacturing-tree", Split: f.cols, Index: 0},
		{Name: "manufacturing-inspector", Split: f.cols, Index: 1},
	}
}

// Tables are the window's tables for the layout sidecar: the recipe list,
// the job list and the browse tree are separate lists.
func (f *Manufacturing) Tables() []*widgets.Table {
	return []*widgets.Table{f.recipes, f.jobs, f.tree}
}

// canonicalFirst is the market table's sort rule: the given key keeps the
// rows in the order Sync delivered them (canonical sim order), any other
// column sorts its cells lexicographically.
func canonicalFirst(key string, cols []widgets.Column) func(string, bool, []widgets.TableRow) {
	return func(k string, asc bool, rows []widgets.TableRow) {
		if k == key {
			if !asc {
				slices.Reverse(rows)
			}
			return
		}
		idx := -1
		for i, c := range cols {
			if c.Key == k {
				idx = i
				break
			}
		}
		if idx < 0 {
			return
		}
		sort.SliceStable(rows, func(a, b int) bool {
			x, y := rows[a].Cells[idx], rows[b].Cells[idx]
			if asc {
				return x < y
			}
			return y < x
		})
	}
}

// ---------------------------------------------------------------------------
// The browse tree (the market window's idiom over recipe groups)
// ---------------------------------------------------------------------------

// mfgGroup resolves a product's browse group from whichever def backs it:
// an item, a module or a hull, the market tree's own ladder.
func mfgGroup(d *content.Content, product content.ItemID) string {
	if d == nil {
		return ""
	}
	switch {
	case d.Items[product] != nil:
		return d.Items[product].Group
	case d.Modules[content.ModuleID(product)] != nil:
		return d.Modules[content.ModuleID(product)].Group
	case d.Ships[content.ShipID(product)] != nil:
		return d.Ships[content.ShipID(product)].Group
	}
	return ""
}

// scopeRecipes is the recipe catalog (canonical order) narrowed to the
// selected branch: All is the whole catalog, any other branch its subtree.
func (f *Manufacturing) scopeRecipes() []content.RecipeID {
	all := f.cv.Defs().RecipesInOrder()
	out := make([]content.RecipeID, 0, len(all))
	for _, rec := range all {
		if inBranch(f.group[rec.ID], f.branch) {
			out = append(out, rec.ID)
		}
	}
	return out
}

// searchHits ranks the branch's recipes over the product name, its group
// path, its family and its inputs' names, so a mineral query finds every
// recipe that eats it (search by ingredient, the inverse lookup a
// six-mineral economy needs). A recipe is a hit when one field contains the
// query, then the hits are ordered by sahilm/fuzzy's score (a matched
// subsequence spanning two fields is not a hit: "vetril" cannot match an
// artillery round through scattered letters). The query and every field are
// diacritic-folded first (fold.go), so an ASCII keyboard matches accented
// names.
func (f *Manufacturing) searchHits(q string, scope []content.RecipeID) []content.RecipeID {
	fq, _ := foldString(q)
	lq := strings.ToLower(fq)
	ids := make([]content.RecipeID, 0, len(scope))
	hay := make([]string, 0, len(scope))
	for _, id := range scope {
		rec, ok := f.cv.Defs().Recipes[id]
		if !ok || rec == nil {
			continue
		}
		fields := []string{f.cv.TradeableName(rec.Product), f.group[id], rec.Family}
		for _, in := range rec.Inputs {
			if def, ok := f.cv.Defs().Item(in.Item); ok {
				fields = append(fields, def.Name)
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
		ids = append(ids, id)
		hay = append(hay, line)
	}
	var out []content.RecipeID
	for _, m := range fuzzy.Find(fq, hay) {
		out = append(out, ids[m.Index])
	}
	return out
}

// ---------------------------------------------------------------------------
// The material bill: shares, verdicts and the inspector
// ---------------------------------------------------------------------------

// needs is one recipe's material bill at runs: the sim's committed need per
// mineral (craft-discounted, the numbers StartJob takes) with the player's
// have side and the mineral's cost weight, in canonical mineral-id order so
// a row's strip never reorders between frames, recipes or authored input
// order.
func (f *Manufacturing) needs(rec content.RecipeID, runs int) []mfgNeed {
	mats := f.cv.JobMaterials(f.cv.DockedIndustry(), rec, runs)
	out := make([]mfgNeed, 0, len(mats))
	for _, m := range mats {
		n := mfgNeed{Item: m.Item, Need: m.Qty, Have: f.cv.PlayerStock(m.Item)}
		if def, ok := f.cv.Defs().Item(m.Item); ok {
			n.Value = def.BasePrice * float64(n.Need)
		}
		out = append(out, n)
	}
	slices.SortFunc(out, func(a, b mfgNeed) int { return strings.Compare(string(a.Item), string(b.Item)) })
	return out
}

// verdictGlyph is the board's one-glance start state, a glyph and nothing
// else: a tick when the sim would accept the recipe now, a cross when it
// would refuse. The shape carries the state (not colour alone); the greyed
// Start and the bill's Danger rows say the rest.
func verdictGlyph(ready bool) string {
	if ready {
		return "✓"
	}
	return "×"
}

// bookValue prices qty units of item on one side of a book: the resting
// orders walked best first (the book's canonical order), each at its own
// price, and the remainder at the last price seen when the depth runs out.
// The second result is false when that side holds nothing for the item.
//
// shortcut: a book too thin to fill the quantity is priced as if its last
// price held for the tail; upgrade when a depth-aware quote is worth the reads.
func bookValue(book []sim.MarketStock, item content.ItemID, side sim.OrderSide, qty int) (float64, bool) {
	if qty <= 0 {
		return 0, true
	}
	left, spend, last := qty, 0.0, 0.0
	for _, o := range book {
		if o.Item != item || o.Side != side {
			continue
		}
		take := min(left, o.Qty)
		spend += float64(take) * o.Price
		left -= take
		last = o.Price
		if left == 0 {
			break
		}
	}
	if last == 0 {
		return 0, false
	}
	return spend + float64(left)*last, true
}

// profitRows is the inspector's profit table, priced off the docked book: the
// asks pay for one run's bill (walked to the depth), the craft fee is the
// sim's own, and the product's revenue is the bids it would meet. A bill line
// the book cannot fill prices from the cheapest ask of any other market
// station (the note says so); a line with no ask anywhere is left out and
// named. A product with no local bid dashes its revenue: no content base
// price stands in for a book that would not pay it. The note is the table's
// footnote ("" when every figure is local and complete).
func (f *Manufacturing) profitRows(book []sim.MarketStock, fallback map[content.ItemID]float64, needs []mfgNeed, def *content.RecipeDef) ([]widgets.PlainRow, string) {
	p := f.th.Pal()
	row := func(label, value string, tint color.RGBA) widgets.PlainRow {
		return widgets.PlainRow{Cells: []string{label, value}, Tint: tint}
	}
	if def == nil || !f.hasSel {
		out := make([]widgets.PlainRow, 0, 6)
		for _, label := range []string{"Materials", "Craft fee", "Total cost", "Sale price", "Profit", "Margin"} {
			out = append(out, row(label, "—", p.Dim))
		}
		return out, ""
	}

	mats, fee := 0.0, f.cv.JobFee(def.ID, 1)
	marked := false
	var unpriced []string
	for _, n := range needs {
		cost, ok := bookValue(book, n.Item, sim.OrderSell, n.Need)
		if !ok {
			price, remote := fallback[n.Item]
			if !remote {
				unpriced = append(unpriced, f.cv.TradeableName(n.Item))
				continue
			}
			cost, marked = price*float64(n.Need), true
		}
		mats += cost
	}
	var notes []string
	if marked {
		notes = append(notes, "no local ask: another station's cheapest ask")
	}
	if len(unpriced) > 0 {
		notes = append(notes, "no ask anywhere: "+strings.Join(unpriced, ", ")+" left out")
	}
	matsCell := mfgMoney(mats)
	if len(notes) > 0 {
		matsCell += " †"
		for i, note := range notes {
			notes[i] = "† " + note
		}
	}
	total := mats + fee
	out := []widgets.PlainRow{
		row("Materials", matsCell, color.RGBA{}),
		row("Craft fee", mfgMoney(fee), color.RGBA{}),
		row("Total cost", mfgMoney(total), color.RGBA{}),
	}
	note := strings.Join(notes, " · ")
	sale, hasBid := bookValue(book, def.Product, sim.OrderBuy, def.Qty)
	if !hasBid {
		return append(out,
			row("Sale price", "— · no bid", p.Dim),
			row("Profit", "—", p.Dim),
			row("Margin", "—", p.Dim)), note
	}
	profit := sale - total
	tint := p.Good
	if profit < 0 {
		tint = p.Danger
	}
	margin := "—"
	if sale > 0 {
		margin = theme.PctLabel(profit / sale)
	}
	return append(out,
		row("Sale price", mfgMoney(sale), color.RGBA{}),
		row("Profit", mfgSigned(profit), tint),
		row("Margin", margin, tint)), note
}

// mfgMoney renders an unsigned amount on the value lines' own scale:
// "201,680 €".
func mfgMoney(v float64) string {
	return theme.GroupFloat(math.Round(v), 0) + " €"
}

// mfgSigned renders a signed amount with the typographic minus:
// "+147,270 €" / "−4,200 €" (the owner's sample).
func mfgSigned(v float64) string {
	if v < 0 {
		return "−" + mfgMoney(-v)
	}
	return "+" + mfgMoney(v)
}

// stripOf is a recipe row's value-share strip: one segment per mineral in
// the bill's canonical order, its width the mineral's share of the run's
// material value and its tint the have/need state: the green part is what
// the hold already covers, so a row reads "mostly covered, a red tail short"
// at a glance.
func stripOf(needs []mfgNeed, p theme.Palette) []widgets.RowSplit {
	out := make([]widgets.RowSplit, 0, len(needs))
	for _, n := range needs {
		tint := p.Good
		if n.Have < n.Need {
			tint = p.Danger
		}
		out = append(out, widgets.RowSplit{Share: n.Value, Tint: tint})
	}
	return out
}

// Sync refreshes the board, the tree, the inspector, the job list and every
// gate from the simulation. Call it every frame before Kit.Update.
func (f *Manufacturing) Sync() {
	f.split.Poll(f.ptr)
	f.cols.Poll(f.ptr)
	s := f.cv
	station := s.DockedIndustry()
	slotsFree := len(s.AllJobs()) < sim.IndustrySlots()
	// The board quotes one run: the chosen run count lives in the quantity
	// picker (its preview re-prices at the picked value), so the rows stay a
	// stable per-run reference.
	runs := 1

	// The recipe list: the branch (or its ranked search hits), one rich row
	// per recipe: product and verdict glyph on line 1, the run time and the
	// value-share strip on line 2.
	f.query = strings.TrimSpace(f.search.GetText())
	scope := f.scopeRecipes()
	list := scope
	if f.query != "" {
		list = f.searchHits(f.query, scope)
	}
	recRows := make([]widgets.TableRow, 0, len(list))
	for _, id := range list {
		recRows = append(recRows, f.recipeRow(station, id, runs))
	}
	if len(recRows) == 0 {
		recRows = []widgets.TableRow{{Cells: []string{"No matches"}, TextTint: f.th.Pal().Dim}}
	}
	f.recipes.SetRows(recRows)
	f.recipes.Sync()

	// Job list: the PLAYER's jobs at every station (the global slot pool),
	// grouped by station and ascending by id within a group; only the
	// docked station's rows act.
	all := s.AllJobs()
	jobs := make([]sim.Job, 0, len(all))
	for _, j := range all {
		if j.Owner == sim.OwnerPlayer {
			jobs = append(jobs, j)
		}
	}
	sort.SliceStable(jobs, func(i, j int) bool {
		if jobs[i].Station != jobs[j].Station {
			return jobs[i].Station < jobs[j].Station
		}
		return jobs[i].ID < jobs[j].ID
	})
	jobRows := make([]widgets.TableRow, 0, len(jobs))
	for _, j := range jobs {
		row := manufacturingRow{job: j.ID, ref: f.productRef(f.productOfJob(j))}
		jobRows = append(jobRows, widgets.TableRow{
			Cells: []string{
				f.cv.TradeableName(f.productOfJob(j)),
				s.StationName(j.Station),
				theme.FormatInt(int64(j.Runs)),
				f.progressOf(j),
				jobStateLabel(j.State),
			},
			Icons: []string{widgets.TradeableIcon(f.cv.Defs(), f.productOfJob(j)), "", "", "", ""},
			Data:  row,
			Spec:  f.rowSpec(row),
		})
	}
	f.jobs.SetRows(jobRows)
	f.jobs.Sync()

	// Revalidate the cached selection against the rows themselves (a
	// recipe/job that left a table must stop gating the actions).
	f.hasSel = false
	if r, ok := f.recipes.Selected(); ok {
		if row, isRow := r.Data.(manufacturingRow); isRow && row.recipe != "" {
			f.sel, f.hasSel = row, true
		}
	}
	if r, ok := f.jobs.Selected(); ok {
		if row, isRow := r.Data.(manufacturingRow); isRow && row.job != 0 {
			f.sel, f.hasSel = row, true
		}
	}
	if !f.hasSel {
		f.sel = manufacturingRow{} // a vanished row must stop gating the actions
	}

	// The browse pane: the branch rows and the highlighted branch.
	f.tree.SetRows(f.treeRows())
	f.tree.SelectData(browseNodeKey{Path: f.treeSel})
	f.tree.Sync()

	// The inspector and its start gate.
	needs := f.needs(f.sel.recipe, runs)
	f.syncInspector(station, slotsFree, needs)

	// Readouts and gates (visible-but-grayed, never hidden).
	slots := sim.IndustrySlots()
	f.jobsLbl.Label = "Jobs: " + theme.FormatInt(int64(len(all))) + "/" + theme.FormatInt(int64(slots)) + " slots"

	f.collect.GetWidget().Disabled = !f.collectReadyFor(f.sel)
	f.cancel.GetWidget().Disabled = !f.cancelReadyFor(f.sel)
	if f.startBtn != nil {
		f.startBtn.GetWidget().Disabled = !f.startReadyFor(f.sel)
	}

	// The material viewport rides the house rail beside it (the tables'
	// clamp rule: a stale offset would bottom-anchor a list that fits).
	if f.matsRail != nil {
		f.matsRail.Sync()
		if f.matsRail.Overflow() <= 0 {
			f.matsScroll.ScrollTop = 0
		}
	}

	// The title is the base display name only (a window title is
	// furniture): the selected recipe, its time, fee and material bill are
	// the inspector's own readouts.
	f.title.Label = "Manufacturing"
}

// recipeRow is one recipe as a rich row: the product and its one-glance
// verdict glyph on line 1, the per-run time and the value-share strip on line
// 2. The icon is the market list's item icon size (the owner's binding
// target). The cells are only the row's fit source (the list has one column).
func (f *Manufacturing) recipeRow(station sim.StationID, rec content.RecipeID, runs int) widgets.TableRow {
	p := f.th.Pal()
	def, _ := f.cv.Defs().Recipes[rec]
	if def == nil {
		return widgets.TableRow{Cells: []string{string(rec)}, TextTint: p.Dim}
	}
	name := f.cv.TradeableName(def.Product)
	needs := f.needs(rec, runs)
	ready := station != "" && f.cv.CanStartJob(station, rec, runs)
	tint := p.Danger
	if ready {
		tint = p.Good
	}
	payload := manufacturingRow{recipe: rec, ref: f.productRef(def.Product)}
	return widgets.TableRow{
		Cells:    []string{name},
		Icons:    []string{widgets.TradeableIcon(f.cv.Defs(), def.Product)},
		IconSize: mktRowIcon,
		Data:     payload,
		Spec:     f.rowSpec(payload),
		Stack: []widgets.RowLine{
			{Segs: []widgets.RowSeg{
				{Text: mfgClip(name, mfgNameClip), Tint: p.Accent, Step: theme.StepSmall},
				{Text: verdictGlyph(ready), Tint: tint, Step: theme.StepSmall},
			}},
			{Segs: []widgets.RowSeg{
				{Text: theme.GroupFloat(f.cv.JobDuration(station, rec, runs), 1) + " s", Tint: p.Dim, Step: theme.StepTiny},
				{Split: stripOf(needs, p), MinW: theme.Px(mfgStripW)},
			}},
		},
	}
}

// syncInspector rebuilds the inspector when its content moved: the selected
// recipe, the station, the slot state, the pane's width or the book its
// profit table reads.
func (f *Manufacturing) syncInspector(station sim.StationID, slotsFree bool, needs []mfgNeed) {
	inner := f.inspInnerW()
	book, fallback := f.quotes(station, needs)
	product := f.recipeProduct(f.sel.recipe)
	key := mfgInspKey(station, slotsFree, f.sel, needs, inner, book, fallback, product)
	if key == f.inspKey {
		return
	}
	f.inspKey = key
	f.insp.RemoveChildren()
	f.matsRows = f.matsRows[:0]
	f.startBtn, f.profitNote = nil, nil

	p := f.th.Pal()

	// Head: the one item head (itemhead.go) the market's right pane leads
	// with: the product's hero art, its name carrying the one-glance verdict
	// glyph, and the product's own description wrapped to the pane.
	def, _ := f.cv.Defs().Recipes[f.sel.recipe]
	f.head = newItemHead(f.seams(), "")
	if def != nil && f.hasSel {
		ready := station != "" && f.cv.CanStartJob(station, f.sel.recipe, 1)
		f.head.set(true, widgets.TradeableIcon(f.cv.Defs(), def.Product),
			f.cv.TradeableName(def.Product)+"  "+verdictGlyph(ready),
			itemProse(f.cv, def.Product), inner)
	} else {
		f.head.set(false, "", "No recipe selected", "", inner)
	}
	rows, note := f.profitRows(book, fallback, needs, def)
	f.profit.SetRows(rows)

	// The body page: the profit table's bar and rows (with the footnote the
	// market-priced figures need), then the materials label and the bill,
	// one row per mineral, the cost driver first (the acceptance answer to
	// "which mineral dominates"), each row the mineral's own art, its share
	// of the run's value and its have/need bar.
	f.matsLbl = widget.NewText(
		widget.TextOpts.Text("MATERIALS · most value first", f.th.Face(theme.StepTiny), theme.Solid(p.Accent)),
	)
	// The page's children keep their own heights (the table is not a bill
	// row), so the box is a plain vertical run, not the equal-row stack.
	f.matsBox = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.SpSm)),
		)),
	)
	f.matsBox.AddChild(sectionBar(f.th, "PROFIT"), f.profit.Container())
	if note != "" {
		f.profitNote = widget.NewText(
			widget.TextOpts.Text(note, f.th.Face(theme.StepTiny), theme.Solid(p.Dim)))
		f.profitNote.MaxWidth = float64(inner)
		f.matsBox.AddChild(f.profitNote)
	}
	f.matsBox.AddChild(f.matsLbl)
	bill := append([]mfgNeed(nil), needs...)
	total := 0.0
	for _, n := range bill {
		total += n.Value
	}
	sort.SliceStable(bill, func(i, j int) bool { return bill[i].Value > bill[j].Value })
	for _, n := range bill {
		r := f.matRow(n, total, p)
		f.matsRows = append(f.matsRows, r)
		f.matsBox.AddChild(r.Root)
	}
	f.matsScroll, f.matsRail = newScrollColumn(f.seams(), f.matsBox)
	f.matsScroll.GetWidget().ScrolledEvent.AddHandler(func(args any) {
		if ev, ok := args.(*widget.WidgetScrolledEventArgs); ok {
			f.scrollMats(ev.Y)
		}
	})

	f.startBtn = theme.ActionButton("Start job", f.startClicked)
	f.startBtn.GetWidget().Disabled = true // armed by Sync's gate
	startBar := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
		)),
	)
	startBar.AddChild(f.startBtn)

	f.insp.AddChild(f.head.headBox, f.matsRail.Container(), startBar)
	f.insp.RequestRelayout()
}

// quotes is the inspector's market read: the docked station's book and, for
// every bill line that book cannot fill, the cheapest ask any other market
// station rests (the market's own cross-station availability read).
//
// shortcut: the fallback walk scans every market book each frame it is
// needed; the table itself still rebuilds only when its change key moves.
func (f *Manufacturing) quotes(station sim.StationID, needs []mfgNeed) ([]sim.MarketStock, map[content.ItemID]float64) {
	if station == "" {
		return nil, nil
	}
	book := f.cv.BookOrders(station)
	short := false
	for _, n := range needs {
		if _, ok := bookValue(book, n.Item, sim.OrderSell, n.Need); !ok {
			short = true
			break
		}
	}
	if !short {
		return book, nil
	}
	fallback := make(map[content.ItemID]float64)
	for item, a := range crossAvail(f.cv, f.stations, station) {
		fallback[item] = a.Cheapest
	}
	return book, fallback
}

// inspInnerW is the head's text width inside the pane's ContentsPad inset:
// the bill viewport's own width plus the rail gutter (the head spans the
// gutter the bill gives up), or the landing width before the first layout.
func (f *Manufacturing) inspInnerW() int {
	if f.matsScroll != nil {
		if r := f.matsScroll.ViewRect(); r.Dx() > 0 {
			return r.Dx() + theme.Px(theme.ScrollGutter)
		}
	}
	return theme.Px(mfgInspW - 2*theme.ContentsPad)
}

// matRow is one mineral of the bill as a built rich row.
func (f *Manufacturing) matRow(n mfgNeed, total float64, p theme.Palette) *widgets.StackRow {
	return widgets.NewStackRow(f.seams(), f.matRowDef(n, total, p), f.th.Face(theme.StepSmall))
}

// matRowDef is one mineral of the bill: its own art at the bill's own icon box
// and name, its share of the run's material value (line 1) and its have/need
// bar with the exact numbers (line 2). The bar's length is have/need clamped
// and its tint the state.
func (f *Manufacturing) matRowDef(n mfgNeed, total float64, p theme.Palette) widgets.TableRow {
	name := string(n.Item)
	if def, ok := f.cv.Defs().Item(n.Item); ok {
		name = def.Name
	}
	share := 0.0
	if total > 0 {
		share = n.Value / total
	}
	frac := 0.0
	if n.Need > 0 {
		frac = math.Min(1, float64(n.Have)/float64(n.Need))
	}
	state := p.Good
	if n.Have < n.Need {
		state = p.Danger
	}
	row := widgets.TableRow{
		Cells:    []string{name},
		Icons:    []string{widgets.TradeableIcon(f.cv.Defs(), n.Item)},
		IconSize: mfgMatIcon,
		Stack: []widgets.RowLine{
			{Segs: []widgets.RowSeg{
				{Text: name, MinW: theme.Px(mfgMatNameW), Step: theme.StepSmall},
				{Text: theme.PctLabel(share), MinW: theme.Px(34), Step: theme.StepTiny, Tint: p.Dim},
			}},
			{Segs: []widgets.RowSeg{
				{HasBar: true, Bar: frac, MinW: theme.Px(mfgMatBarW), Tint: state},
				{Text: theme.FormatInt(int64(n.Have)) + " / " + theme.FormatInt(int64(n.Need)),
					Step: theme.StepTiny, Tint: state},
			}},
		},
	}
	return row
}

// mfgInspKey is the inspector's change signature: the selection, the station
// gate, the slot state, the pane's width (the head re-wraps with the seam),
// every material's need/have, the book rows the profit table reads (the
// bill's asks and the product's bids) and the fallback prices. A book move
// rebuilds the card; an idle book does not.
func mfgInspKey(station sim.StationID, slotsFree bool, row manufacturingRow, needs []mfgNeed,
	inner int, book []sim.MarketStock, fallback map[content.ItemID]float64, product content.ItemID) string {
	var b strings.Builder
	b.WriteString(string(row.recipe))
	b.WriteByte(';')
	b.WriteString(string(station))
	b.WriteByte(';')
	if slotsFree {
		b.WriteByte('s')
	}
	b.WriteString(strconv.Itoa(inner))
	b.WriteByte(';')
	for _, n := range needs {
		b.WriteString(string(n.Item))
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(n.Need))
		b.WriteByte('/')
		b.WriteString(strconv.Itoa(n.Have))
		b.WriteByte(';')
	}
	priced := func(item content.ItemID) bool {
		if item == product {
			return true
		}
		for _, n := range needs {
			if n.Item == item {
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
	for _, n := range needs {
		if price, ok := fallback[n.Item]; ok {
			b.WriteString("f:")
			b.WriteString(string(n.Item))
			b.WriteByte(':')
			b.WriteString(strconv.FormatFloat(price, 'g', -1, 64))
			b.WriteByte(';')
		}
	}
	return b.String()
}

// scrollMats turns wheel notches into the material viewport's offset: two
// rows of the measured row height per notch, the same 0..1-fraction-of-
// overflow convention the tables use (a positive notch scrolls towards the
// top of the list).
func (f *Manufacturing) scrollMats(notches float64) {
	rowH := 0
	if len(f.matsRows) > 0 {
		_, rowH = f.matsRows[0].Root.PreferredSize()
	}
	scrollNotches(f.matsScroll, f.matsRail, rowH, 2, notches)
}

// inputsText renders the have/need line of a job's materials: one
// "Name have/need" pair per input, joined by the table's ellipsis rules.
// The quantity picker's preview renders it (the board's rows carry the
// material bill as bars instead).
func inputsText(cv widgets.ContentView, mats []content.CargoStack) string {
	if len(mats) == 0 {
		return "—"
	}
	out := ""
	for i, m := range mats {
		if i > 0 {
			out += " · "
		}
		name := string(m.Item)
		if def, ok := cv.Defs().Item(m.Item); ok {
			name = def.Name
		}
		out += name + " " + theme.FormatInt(int64(cv.PlayerStock(m.Item))) +
			"/" + theme.FormatInt(int64(m.Qty))
	}
	return out
}

// productRef resolves a recipe's product for the row's Show Info entry.
func (f *Manufacturing) productRef(product content.ItemID) widgets.InfoRef {
	ref, _ := widgets.ResolveTradeable(f.cv.Defs(), product).Ref()
	return ref
}

// productOfJob resolves a job's recipe product ("" when the recipe no
// longer loads — the sim keeps such jobs collectable-guarded).
func (f *Manufacturing) productOfJob(j sim.Job) content.ItemID {
	if rec, ok := f.cv.Defs().Recipes[j.Recipe]; ok {
		return rec.Product
	}
	return ""
}

// progressOf renders a job's progress: "100%" once done, else the elapsed
// fraction of its clock window.
func (f *Manufacturing) progressOf(j sim.Job) string {
	if j.State == sim.JobDone {
		return "100%"
	}
	span := j.DoneAt - j.StartedAt
	if span <= 0 {
		return "100%"
	}
	pct := (f.cv.Time() - j.StartedAt) / span
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	return strconv.Itoa(int(pct*100)) + "%"
}

// jobStateLabel is the jobs table's State cell.
func jobStateLabel(state sim.JobState) string {
	if state == sim.JobDone {
		return "Ready"
	}
	return "Running"
}

// startReadyFor is the Start gate for one recipe row: the sim would accept
// it right now at one run (CanStartJob covers the industry service, the
// slots and the materials) — the picker re-checks its own run count.
func (f *Manufacturing) startReadyFor(row manufacturingRow) bool {
	if row.recipe == "" {
		return false
	}
	return f.cv.CanStartJob(f.cv.DockedIndustry(), row.recipe, 1)
}

// collectReadyFor is the Collect gate for one job row: the docked
// station's own, finished, player-owned job.
func (f *Manufacturing) collectReadyFor(row manufacturingRow) bool {
	if row.job == 0 {
		return false
	}
	for _, j := range f.cv.AllJobs() {
		if j.ID == row.job {
			return j.Station == f.cv.DockedIndustry() && j.State == sim.JobDone &&
				j.Owner == sim.OwnerPlayer
		}
	}
	return false
}

// cancelReadyFor is the Cancel gate for one job row: a running,
// player-owned job at the docked station (a finished one is collected).
func (f *Manufacturing) cancelReadyFor(row manufacturingRow) bool {
	if row.job == 0 {
		return false
	}
	for _, j := range f.cv.AllJobs() {
		if j.ID == row.job {
			return j.Station == f.cv.DockedIndustry() && j.State == sim.JobRunning &&
				j.Owner == sim.OwnerPlayer
		}
	}
	return false
}

// rowSpec is one manufacturing row's declared interactions: the product's
// Show Info (grayed when the id no longer resolves), plus a job row's
// Collect / Cancel with the same gate as the toolbar button. A recipe row
// declares no action: a rich row draws its actions as 92 px trailing
// buttons, a third of the list pane, and the recipe's Start is the
// inspector's own button (the row's click opens the recipe there).
func (f *Manufacturing) rowSpec(row manufacturingRow) widgets.RowSpec {
	spec := widgets.RowSpec{Ref: &row.ref}
	if row.job != 0 {
		spec.Actions = f.rowActions(row)
	}
	return spec
}

// rowActions is a job row's context-menu action entry (Collect for a done
// job, Cancel for a running one) with the same gate as the toolbar button.
func (f *Manufacturing) rowActions(row manufacturingRow) []widgets.RowAction {
	switch {
	case row.job != 0:
		if f.collectReadyFor(row) {
			return []widgets.RowAction{{Label: "Collect",
				Intent: widgets.CollectJob{Job: row.job}}}
		}
		return []widgets.RowAction{{Label: "Cancel", Disabled: !f.cancelReadyFor(row),
			Intent: widgets.CancelJob{Job: row.job}}}
	}
	return nil
}

// mfgClip shortens a name to max runes, marking the cut with an ellipsis
// (a rich row's own text never ellipsizes, so the owner measures).
func mfgClip(name string, max int) string {
	if max < 1 || utf8.RuneCountInString(name) <= max {
		return name
	}
	r := []rune(name)
	return string(r[:max-1]) + "…"
}

// recipeProduct is a recipe's product id ("" when the recipe no longer
// loads) — the picker's title and item line.
func (f *Manufacturing) recipeProduct(rec content.RecipeID) content.ItemID {
	if def, ok := f.cv.Defs().Recipes[rec]; ok {
		return def.Product
	}
	return ""
}

// FocusSearch focuses the browse pane's search field (a click on the field
// does the same).
func (f *Manufacturing) FocusSearch() { f.search.Focus(true) }

// openPicker opens the quantity picker on a recipe's runs: the Start
// gesture (the inspector's button). Nothing opens away from an industry
// station.
func (f *Manufacturing) openPicker(row manufacturingRow) {
	if row.recipe == "" || f.cv.DockedIndustry() == "" {
		return
	}
	f.as.Do(widgets.QtyPick{Mode: widgets.QtyStart, Item: f.recipeProduct(row.recipe), Recipe: row.recipe})
}

// startClicked opens the picker on the inspected recipe (the inspector's own
// Start button acts on the selection). Grayed is grayed: a disabled button
// opens nothing, even when a harness fires the click past the gate.
func (f *Manufacturing) startClicked() {
	if !f.startReadyFor(f.sel) {
		return
	}
	f.openPicker(f.sel)
}

// collectClicked emits CollectJob for the selected job.
func (f *Manufacturing) collectClicked() { f.collectWith(f.sel) }

// collectWith emits CollectJob for one job row (gated even when called
// directly).
func (f *Manufacturing) collectWith(row manufacturingRow) {
	if !f.collectReadyFor(row) {
		return
	}
	f.as.Do(widgets.CollectJob{Job: row.job})
}

// cancelClicked emits CancelJob for the selected job.
func (f *Manufacturing) cancelClicked() { f.cancelWith(f.sel) }

// cancelWith emits CancelJob for one job row (gated even when called
// directly).
func (f *Manufacturing) cancelWith(row manufacturingRow) {
	if !f.cancelReadyFor(row) {
		return
	}
	f.as.Do(widgets.CancelJob{Job: row.job})
}
