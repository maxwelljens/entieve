package screens

import (
	"image"
	"image/color"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/icons"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/sahilm/fuzzy"
)

const (
	// mktTreeW is the browse pane's opening width in design px at the
	// landing rect: the split's opening share, not a fixed width — the
	// tree seam trades it with the catalog.
	mktTreeW = 176

	// mktSideW is the right pane's opening width: the old fixed 240 px
	// column, now the split's right pane (the order book over My orders).
	mktSideW = 240

	// mktDefaultW/H are the window's landing rect (see the Location
	// below): wider than tall for three panes, and still inside the
	// 1024×700 minimum right of the rail gutter.
	mktDefaultW = 760
	mktDefaultH = 560

	// mktTreeShare / mktSideShare are the two seams' opening ratios at
	// that landing width: the split spans the window minus the body
	// padding and both seam tracks. The seam stores the fraction BEFORE
	// it, so the second seam's default is 1-mktSideShare.
	mktTreeShare = float64(mktTreeW) / float64(mktDefaultW-2*theme.ContentsPad-2*widgets.SplitSeamH)
	mktSideShare = float64(mktSideW) / float64(mktDefaultW-2*theme.ContentsPad-2*widgets.SplitSeamH)

	// mktSearchH is the search field's design-px height above the tree.
	mktSearchH = 28

	// mineWheelRows is how many rows of the right pane's column one wheel
	// notch scrolls. Neither section is capped (the selected item's whole
	// book, every standing order the player has), so both ride the one
	// scroll viewport with the rail's reserved gutter beside it.
	mineWheelRows = 3
)

// setStepperValue writes a readout label and re-lays its row out.
// widget.Text.Label changes do NOT invalidate layout, so without the
// relayout the box keeps the width measured for its previous value and a
// wider number draws over its neighbour. The label carries the tabular
// contents face, so digit widths stay stable while the value moves.
func setStepperValue(bar *widget.Container, lbl *widget.Text, s string) {
	if lbl.Label == s {
		return
	}
	lbl.Label = s
	bar.RequestRelayout()
}

// Market is the station market window: a browse tree (the content's
// group paths), the catalog list and the right pane, which leads with the
// selected item's icon and description over its order book and My orders.
//
// The catalog lists every item of the tradeable catalog once (canonical
// order) with its ladder quote and two-side quantity. Selecting an item
// fills the right pane: the item's hero icon and description lead, then
// its full resting book as an aligned table (sell orders
// price-ascending, then buy orders price-descending, one row per resting
// order), then My orders. Buy/Sell execute as market orders against the
// book: each opens the quantity picker (CONTEXT.md "Quantity picker") on
// the selected item and Confirm emits Buy/Sell. They sit in a pinned bar
// at the right pane's bottom, acting on the selection. The title bar stays
// the base display name (window titles are furniture); the book's own
// columns carry the price voice.
//
// The browse tree scopes the catalog to a branch (All is the whole
// catalog) and the search field filters that branch live, ranked by fuzzy
// score with the matched letters highlighted. Selecting an item
// highlights its branch back. Trade is only possible while the player is
// docked; Buy also needs a local sell order to lift, so both buttons can
// grey while docked.
//
// An item with no orders anywhere still gets its row — dimmed, but live:
// placing the first order is how a market wakes up (Buy order
// works from an empty book). Placing a hull sell order is refused by the
// sim, so hull rows trade the way they always did. The columns stay local
// to the docked book.
//
// Below the book sits My orders: only THIS station's player orders, each
// with a Cancel button. It acts on the docked book, exactly like
// Buy/Sell. Posting a new standing order is the typed order mini-window's
// job (OrderWin, launched from a row's context menu), not a panel here.
type Market struct {
	th  theme.Theme
	cv  widgets.ContentView
	as  widgets.ActionSink
	ptr *widgets.Pointer
	win *widget.Window

	tbl   *widgets.Table
	title *theme.ChromeText
	buy   *widget.Button
	sell  *widget.Button

	// split is the browse | catalog | right pane row: two seams (tree and
	// orders) trade width with the middle pane (the old fixed 240 px
	// column is the right pane's opening share).
	split *widgets.SplitPane

	// The catalog: items is the tradeable universe (built once — content
	// never changes under a session), group each item's market tree path.
	items []content.ItemID
	group map[content.ItemID]string
	maker map[content.ItemID]string

	// The browse pane: the shared browse tree (its nodes, fold state,
	// branch and highlight) over the one-column table and the live search.
	browseTree
	tree   *widgets.Table
	search *widget.TextInput

	// Search results: the ranked hits of the current query, in order, and
	// hit the keyboard-highlighted index (-1 = none).
	results []mktHit
	query   string
	hit     int

	// The right pane: the selected item's head over two labelled sections in
	// ONE scroll column. bookBar/bookLbl and mineBar/mineLbl are the section
	// bars (a retitling label handle each); bookTbl is the book's aligned
	// table and book its rows. The My orders rows are one rich row per
	// player order at the docked station, every one listed. The pane's
	// scroll and its house rail cover the whole column.
	bookBar *widget.Container
	bookLbl *theme.ChromeText
	bookTbl *widgets.PlainTable
	book    []widgets.PlainRow

	mineBar  *widget.Container
	mineLbl  *theme.ChromeText
	mineBox  *widget.Container
	mineRows []*widgets.StackRow

	// The head: the item's hero icon and its description above the book (the
	// hint line replaces them with no selection). The shared item head the
	// manufacturing inspector mounts too.
	*itemHead

	sideBox    *widget.Container
	sideScroll *widget.ScrollContainer
	sideRail   *widgets.ScrollRail
	sideKey    string // change signature of the right pane's content

	sel    content.ItemID
	hasSel bool
}

// mktNoSide is an item row's Side cell: an item has no side of its own.
// The Qty column already reads 0 for a dead market, so the cell stays a
// dash rather than an empty box.
const mktNoSide = "—"

// mktHit is one ranked search result: the item and the matched byte
// positions within its display name (for the highlight runs).
type mktHit struct {
	Item  content.ItemID
	Match []int
}

// marketInfo maps each catalog item to its market tree path and its
// maker's display name ("" for the unattributed defs) — the tree's rows
// and two of the search's fields.
func marketInfo(d *content.Content, items []content.ItemID) (group, maker map[content.ItemID]string) {
	group = make(map[content.ItemID]string, len(items))
	maker = make(map[content.ItemID]string, len(items))
	if d == nil {
		return group, maker
	}
	for _, id := range items {
		var g, m string
		switch {
		case d.Items[id] != nil:
			g, m = d.Items[id].Group, d.Items[id].Maker
		case d.Modules[content.ModuleID(id)] != nil:
			g, m = d.Modules[content.ModuleID(id)].Group, d.Modules[content.ModuleID(id)].Maker
		case d.Ships[content.ShipID(id)] != nil:
			g, m = d.Ships[content.ShipID(id)].Group, d.Ships[content.ShipID(id)].Maker
		}
		group[id] = g
		maker[id] = content.CorpName(m)
	}
	return group, maker
}

// itemIcon picks one tradeable's row glyph through the one resolver (a
// module's own icon, an item's icon, a hull's drop-in art or silhouette).
// A stale hull id — one whose def is gone but whose market group still says
// Ships/ — keeps the hull silhouette; branch/parent rows stay text-only and
// any other unknown group returns "".
func (m *Market) itemIcon(item content.ItemID) string {
	t := widgets.ResolveTradeable(m.cv.Defs(), item)
	if !t.Found && strings.HasPrefix(m.group[item], "Ships/") {
		return theme.HullIcon
	}
	return t.Icon
}

// mktRowIcon is every market item row's icon box: 20 px (30 px of content in
// the 31 px row — no density change).
const mktRowIcon = theme.IconButton

// scopeItems is the catalog narrowed to the selected branch: All is the
// whole catalog, any other branch its own subtree.
func (m *Market) scopeItems() []content.ItemID {
	if m.branch == "" {
		return m.items
	}
	out := make([]content.ItemID, 0, len(m.items))
	for _, item := range m.items {
		if inBranch(m.group[item], m.branch) {
			out = append(out, item)
		}
	}
	return out
}

// searchHits ranks the branch's items by fuzzy score over the display
// name, the full group path and the maker — sahilm/fuzzy's Find, so the
// ranking is the library's (descending score, stable for ties). Both the
// query and the haystack are diacritic-folded first (fold.go), so an
// ASCII keyboard matches accented names and a typed diacritic matches
// back; the library's byte offsets are remapped to the original's rune
// starts so the highlight tints the whole letter.
func (m *Market) searchHits(q string, scope []content.ItemID) []mktHit {
	fq, _ := foldString(q)
	fnames := make([]string, len(scope))
	folds := make([][]int, len(scope))
	hay := make([]string, len(scope))
	for i, item := range scope {
		fname, nmap := foldString(m.cv.TradeableName(item))
		fnames[i], folds[i] = fname, nmap
		fg, _ := foldString(m.group[item])
		fm, _ := foldString(m.maker[item])
		hay[i] = fname + " " + fg + " " + fm
	}
	var out []mktHit
	for _, mt := range fuzzy.Find(fq, hay) {
		item := scope[mt.Index]
		match := make([]int, 0, len(mt.MatchedIndexes))
		for _, i := range mt.MatchedIndexes {
			if i < len(fnames[mt.Index]) {
				if folds[mt.Index] != nil {
					i = folds[mt.Index][i]
				}
				match = append(match, i)
			}
		}
		out = append(out, mktHit{Item: item, Match: match})
	}
	return out
}

// FocusSearch focuses the browse pane's search field (Ctrl+F, or a click
// on the field). The field stays focused until Esc or another click.
func (m *Market) FocusSearch() { m.search.Focus(true) }

// SearchFocused reports whether the search field holds focus — the host's
// world-key gate.
func (m *Market) SearchFocused() bool { return m.search.IsFocused() }

// SearchKey handles the keys the field's own editing does not: the result
// selection's arrows and Enter, and the two-step Esc (clear the query
// first, then blur and let the host handle it).
func (m *Market) SearchKey(k ebiten.Key) bool {
	switch k {
	case ebiten.KeyArrowDown:
		m.moveHit(1)
		return true
	case ebiten.KeyArrowUp:
		m.moveHit(-1)
		return true
	case ebiten.KeyEnter:
		if m.query != "" && m.hit < 0 && len(m.results) > 0 {
			m.hit = 0
		}
		m.pickHit()
		return true
	case ebiten.KeyEscape:
		if m.search.GetText() != "" {
			m.search.SelectAll()
			m.search.DeleteSelectedText()
			return true
		}
		m.search.Focus(false)
		return false
	}
	return false
}

// moveHit walks the ranked results and highlights the row it lands on.
func (m *Market) moveHit(d int) {
	if len(m.results) == 0 {
		return
	}
	m.hit = min(max(m.hit+d, 0), len(m.results)-1)
	m.selectItem(m.results[m.hit].Item)
}

// pickHit selects the keyboard-highlighted result (Enter).
func (m *Market) pickHit() {
	if m.hit < 0 || m.hit >= len(m.results) {
		return
	}
	m.selectItem(m.results[m.hit].Item)
}

// selectItem makes item the window's selection and syncs the browse
// highlight back to its group branch.
func (m *Market) selectItem(item content.ItemID) {
	m.sel, m.hasSel = item, true
	m.treeSel = m.group[item]
	m.highlightTree()
	m.tbl.SelectData(mktItemRow{Item: item})
}

// mktItemRow is an item's catalog row payload: the item the row names.
type mktItemRow struct{ Item content.ItemID }

// sortBookOrders puts mixed orders into canonical display order: item
// ascending, asks before bids, asks by price ascending, bids by price
// descending. Stable, so equal prices keep arrival (price-time) order.
func sortBookOrders(orders []sim.MarketStock) {
	sort.SliceStable(orders, func(i, j int) bool {
		a, b := orders[i], orders[j]
		if a.Item != b.Item {
			return a.Item < b.Item
		}
		if a.Side != b.Side {
			return a.Side == sim.OrderSell
		}
		if a.Price != b.Price {
			if a.Side == sim.OrderSell {
				return a.Price < b.Price
			}
			return a.Price > b.Price
		}
		return false
	})
}

// NewMarket builds the Market window over the shared environment. Mount
// it on a Kit with Kit.AddWindow and call Sync every frame.
func NewMarket(env widgets.Seams) *Market {
	m := &Market{
		th: env, cv: env, as: env, ptr: env.Pointer(),
		items: marketCatalog(env),
		hit:   -1,
	}
	m.group, m.maker = marketInfo(env.Defs(), m.items)
	m.browseTree = newBrowseTree(browseNodes(m.items, m.group))
	p := env.Pal()

	cols := []widgets.Column{
		{Key: "Item", Title: "Item", Priority: 0},
		{Key: "Price", Title: "Price", Icon: "svc_market", Priority: 1, AlignRight: true},
		{Key: "Qty", Title: "Qty", Priority: 2, AlignRight: true},
		{Key: "Side", Title: "Side", Priority: 3},
	}
	// Contents face: the order book is names, prices and quantities —
	// IBM Plex Mono lines the columns up (title, toolbar and buttons
	// stay IBM Plex Sans).
	m.tbl = widgets.NewTable(env, cols, widgets.ContentsFace(env.FaceMono(theme.StepSmall)), widgets.PrefsID("book"))
	m.tbl.SetSort("Item", true)
	// Sync builds one row per catalog item in canonical order; the Item
	// sort keeps that order — reversing the items on a second click — and
	// any other header sorts the items by that column's cell. While a
	// search query is active the rows arrive ranked and flat: the header
	// sort yields.
	itemSort := canonicalFirst("Item", cols)
	m.tbl.SetSortFunc(func(key string, asc bool, rows []widgets.TableRow) {
		if m.query != "" {
			return
		}
		itemSort(key, asc, rows)
	})
	// A click selects the row's item: the toolbar reads it, Buy/Sell trade
	// it and the right pane shows its order book. The row's declared
	// RowSpec already carries its own item, so the menu never depends on
	// this selection; a double-click still opens Show Info.
	m.tbl.OnSelect(func(r widgets.TableRow) {
		m.search.Focus(false)
		item, ok := r.Data.(mktItemRow)
		if !ok {
			return
		}
		m.sel, m.hasSel = item.Item, true
		m.treeSel = m.group[item.Item]
		m.highlightTree()
	})

	// The browse pane: the house's live search field over the shared
	// one-column browse tree.
	m.search = newBrowseSearch(env, "Search", mktSearchH)
	m.tree = newBrowseTable(env, "tree", "Tree")
	m.tree.OnSelect(func(r widgets.TableRow) {
		m.search.Focus(false)
		if n, ok := r.Data.(browseNodeKey); ok {
			m.selectBranch(n.Path)
		}
	})

	// The trade actions: side by side in the right pane's pinned bar. Buy
	// wears the Good role and Sell the Danger role on its label and glyph
	// (the face stays house chrome); both grey while gated, never hidden.
	// The glyphs are the closest existing pair: an open chest (acquire) and
	// a hand-off (put on the book).
	mkTrade := func(label, icon string, tint color.RGBA, clicked func()) *widget.Button {
		return widget.NewButton(
			widget.ButtonOpts.TextLabel(label),
			widget.ButtonOpts.TextColor(&widget.ButtonTextColor{
				Idle:     theme.Solid(tint),
				Hover:    theme.Solid(tint),
				Pressed:  theme.Solid(tint),
				Disabled: theme.NRGBA(p.Dim, 160),
			}),
			widget.ButtonOpts.Graphic(&widget.GraphicImage{
				Idle:     icons.Get(icon, theme.Px(theme.IconButton), tint),
				Hover:    icons.Get(icon, theme.Px(theme.IconButton), tint),
				Pressed:  icons.Get(icon, theme.Px(theme.IconButton), tint),
				Disabled: icons.Get(icon, theme.Px(theme.IconButton), p.Dim),
			}),
			widget.ButtonOpts.GraphicPadding(widget.Insets{Right: theme.Px(theme.StatIconGap)}),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(theme.Px(40), theme.Px(40)),
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
			widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) { clicked() }),
		)
	}
	m.buy = mkTrade("Buy", "act_loot", p.Good, m.buyClicked)
	m.sell = mkTrade("Sell", "ctr_deliver", p.Danger, m.sellClicked)

	// The catalog row: the item table — widgets.Table owns its scroll
	// viewport, its wheel wiring AND the scrollbar rail beside it (the
	// reserved-gutter chrome, ScrollGutter design px: the handle can never
	// overhang a row, and the rail hides while the list fits).
	catalog := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout()))
	m.tbl.Container().GetWidget().LayoutData = widget.AnchorLayoutData{
		StretchHorizontal: true,
		StretchVertical:   true,
	}
	catalog.AddChild(m.tbl.Container())

	// The right pane: the selected item's head (its hero icon and its
	// description) over two labelled sections in ONE scroll column: the
	// item's order book (aligned cells, sell orders then buy orders) over
	// My orders, riding the house rail (the reserved gutter hides while
	// everything fits). No fold and no tabs, so a selection shows its book
	// at once. The head is the one item head (itemhead.go); with no
	// selection its hint names the gesture.
	m.itemHead = newItemHead(env, "Select an item to see its book")

	// The section headers are the infowin section bars (glyph, tracked label
	// at the small step, hairline rule); My orders retitles with its count,
	// so its label handle comes back.
	m.bookBar, m.bookLbl = sectionBarText(env, "ORDER BOOK")
	m.bookTbl = widgets.NewPlainTable(env, []widgets.PlainColumn{{}, {AlignRight: true}, {AlignRight: true}})
	m.mineBar, m.mineLbl = sectionBarText(env, "MY ORDERS")
	m.mineBox = widget.NewContainer(
		widget.ContainerOpts.Layout(widgets.StackLayout{Gap: theme.Px(theme.ContentsPad)}),
	)
	m.sideBox = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, false, false, false, false}),
			widget.GridLayoutOpts.Spacing(0, theme.Px(theme.ContentsPad)),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	m.sideBox.AddChild(m.headBox, m.bookBar, m.bookTbl.Container(), m.mineBar, m.mineBox)
	m.sideScroll, m.sideRail = newScrollColumn(env, m.sideBox)
	// Wheel scrolls the column; ScrollTop is a 0..1 fraction of the
	// overflow, so the step scales by the measured overflow (the tables'
	// convention). The rail beside it drags the same offset.
	m.sideScroll.GetWidget().ScrolledEvent.AddHandler(func(args any) {
		if ev, ok := args.(*widget.WidgetScrolledEventArgs); ok {
			m.scrollSide(ev.Y)
		}
	})

	// The pinned trade bar: the pane's full width less the inset and the
	// rail gutter, so the buttons end on the book's right edge and ride
	// below the scroll column however far it scrolls.
	tradeBar := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.ContentsPad)),
			widget.RowLayoutOpts.Padding(&widget.Insets{
				Left:   theme.Px(theme.ContentsPad),
				Right:  theme.Px(theme.ContentsPad + theme.ScrollGutter),
				Bottom: theme.Px(theme.ContentsPad),
			}),
		)),
	)
	tradeBar.AddChild(m.buy, m.sell)
	side := paneStack([]bool{true, false}, m.sideRail.Container(), tradeBar)

	// The catalog keeps the full row height, its scrollbar rail included.
	// The browse pane is the search field over the tree, both full height.
	left := paneStack([]bool{true}, catalog)
	browse := paneStack([]bool{false, true}, m.search, m.tree.Container())

	// Body: browse | seam | catalog | seam | right pane. Two draggable
	// seams; the tree and right panes open at their historical widths
	// (176 px of browse, the old 240 px column) and each pane keeps
	// SplitMinPane.
	m.split = widgets.NewSplitPaneRow(env, browse, left, side)
	m.split.SetDefaultRatio(0, mktTreeShare)
	m.split.SetDefaultRatio(1, 1-mktSideShare)
	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.WindowBodyBox(p, theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200))),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	body.AddChild(m.split.Container())

	title, marketTitle := theme.NewTitleBar(env, "Market", "svc_market")
	m.title = marketTitle

	m.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(300), theme.Px(240)), // content floor
		// The window's landing rect — wider than tall for three panes:
		// 760×560 at (97,8) starts on the rail gutter's clear line, ends
		// inside the 1024×700 minimum with an 8 px margin and has the
		// height the search field, the catalog and the My-orders column
		// need. It overlaps the log pocket at the minimum (the trade
		// window is the big one; the log raises back over it on a click).
		// The game's Market Screen row declares the same rect
		// (MarketRect), so it opens here on first use, docked and in
		// space alike.
		widget.WindowOpts.Location(pxRect(MarketRect())),
	)
	return m
}

// MarketRect is the market window's landing rect in design units: the
// one number its Screen row's DefaultRect and its constructor share.
func MarketRect() image.Rectangle {
	return image.Rect(97, 8, 97+mktDefaultW, 8+mktDefaultH)
}

// Win returns the market window for Kit.AddWindow.
func (m *Market) Win() *widget.Window { return m.win }

// Splits names the window's two draggable seams for the layout sidecar:
// tree | catalog ("market-tree") and catalog | right pane ("market-orders",
// the seam name the old two-pane window carried).
func (m *Market) Splits() []widgets.Seam {
	return []widgets.Seam{
		{Name: "market-tree", Split: m.split, Index: 0},
		{Name: "market-orders", Split: m.split, Index: 1},
	}
}

// Tables are the window's tables for the layout sidecar: the order book
// and the browse tree are separate lists with separate column sets.
func (m *Market) Tables() []*widgets.Table {
	return []*widgets.Table{m.tbl, m.tree}
}

// orders returns the orders the window shows: the docked station's own
// book when it offers the market, else every station's book in the
// current system (the undocked fallback view).
func (m *Market) orders() []sim.MarketStock {
	if st := m.cv.DockedMarket(); st != "" {
		return m.cv.BookOrders(st)
	}
	sys := m.cv.System()
	if sys == nil {
		return nil
	}
	return m.cv.Market(sys.ID)
}

// marketCatalog is the market's item universe in canonical order: every
// item (ore, mineral, loot, ammo, charge), module and hull the book can
// list, ascending by id — the order the book always used. Built once; the
// catalog never changes under a session.
func marketCatalog(cv widgets.ContentView) []content.ItemID {
	d := cv.Defs()
	if d == nil {
		return nil
	}
	out := make([]content.ItemID, 0, len(d.Items)+len(d.Modules)+len(d.Ships))
	for id := range d.Items {
		out = append(out, id)
	}
	for id := range d.Modules {
		out = append(out, content.ItemID(id))
	}
	for id := range d.Ships {
		out = append(out, content.ItemID(id))
	}
	slices.Sort(out)
	return out
}

// Dismissed is the committed-chrome-close hook (the ×): the expanded
// branches, the tree selection and the query are session-only, so a
// window closed by its title bar forgets them and reopens on All with
// everything collapsed. The host runs it once per mount cycle before
// Sync; a hotkey unmount never reports a committed close, so that path
// keeps them.
func (m *Market) Dismissed() {
	m.reset()
	m.search.Focus(false)
	if m.search.GetText() != "" {
		m.search.SelectAll()
		m.search.DeleteSelectedText()
	}
}

// Sync refreshes the rows, the tree and the docked gating from the
// simulation. Call it every frame before Kit.Update.
func (m *Market) Sync() {
	m.split.Poll(m.ptr)
	orders := m.orders()
	sortBookOrders(orders)
	// Trades only happen docked at a station — the same gate feeds the
	// buttons below and every row's declared Buy/Sell actions.
	gated := !m.docked()
	docked := m.cv.DockedMarket()
	// One quote per item for the parent rows, through the seam's ladder:
	// the station's book docked, the shown system's orders in space.
	quote := func(item content.ItemID) float64 { return m.cv.PriceLadder(docked, item) }
	by := make(map[content.ItemID][]sim.MarketStock, len(m.items))
	for _, o := range orders {
		by[o.Item] = append(by[o.Item], o)
	}
	// The other stations' books stay out of the catalog: the columns read
	// only the docked book.
	ask := func(item content.ItemID) bool {
		for _, o := range by[item] {
			if o.Side == sim.OrderSell {
				return true
			}
		}
		return false
	}

	m.query = strings.TrimSpace(m.search.GetText())
	scope := m.scopeItems()
	var rows []widgets.TableRow
	if m.query != "" {
		// Search: flat, ranked by score (the header sort yields, see the
		// table's sort func), each row's name carrying its matched
		// character positions as styled runs.
		m.results = m.searchHits(m.query, scope)
		if len(m.results) == 0 {
			rows = []widgets.TableRow{{
				Cells:    []string{"No matches", "", "", mktNoSide},
				TextTint: m.th.Pal().Dim,
			}}
		}
		for _, h := range m.results {
			ords := by[h.Item]
			total := 0
			for _, o := range ords {
				total += o.Qty
			}
			rows = append(rows, m.resultRow(h, total, quote(h.Item)))
		}
	} else {
		m.results = nil
		rows = make([]widgets.TableRow, 0, len(scope))
		for _, item := range scope {
			rows = append(rows, m.itemRow(item, by[item], quote(item), ask(item), gated))
		}
	}
	m.tbl.SetRows(rows)
	m.tbl.Sync()

	// Revalidate the cached selection against the table itself: a row
	// that left the board (sold out, station swapped) must stop reading
	// as selected, or Buy/Sell would keep acting on an item the market
	// no longer offers.
	m.hasSel = false
	if r, ok := m.tbl.Selected(); ok {
		if item, isRow := r.Data.(mktItemRow); isRow {
			m.sel, m.hasSel = item.Item, true
		}
	}
	// A search result's keyboard highlight follows the table selection.
	if m.query != "" {
		m.hit = -1
		for i, h := range m.results {
			if m.hasSel && h.Item == m.sel {
				m.hit = i
				break
			}
		}
	}

	// The browse pane: the branch rows and the highlighted branch.
	m.tree.SetRows(m.treeRows())
	m.tree.SelectData(browseNodeKey{Path: m.treeSel})
	m.tree.Sync()

	// The right pane: the selected item's book over My orders (rebuilt only
	// when the selection or either list moves).
	m.syncSide(gated, by)

	// The column rides the house rail beside it: mirror the offset onto the
	// handle, and drop a stale offset once the sections fit again — the
	// tables' clamp rule (ebitenui would otherwise bottom-anchor it).
	m.sideRail.Sync()
	if m.sideRail.Overflow() <= 0 {
		m.sideScroll.ScrollTop = 0
	}

	sys := m.cv.System()
	if sys != nil {
		// The title is the base display name only (a window title is
		// furniture): the price ladder is the book's own columns and the
		// wallet is the HUD readout, never the bar.
		m.title.Label = "Market"
	}
	// Trades only happen docked at a station (the gate declared above);
	// Buy additionally needs a local sell order to lift — an item no one
	// sells here stays visible and requestable, its Buy grayed.
	m.buy.GetWidget().Disabled = gated || !m.hasSel || !ask(m.sel)
	m.sell.GetWidget().Disabled = gated
	// The catalog's own rail mirrors the table's scroll in m.tbl.Sync
	// above: hidden while the list fits, thumb at ScrollTop otherwise; a
	// drag writes the offset back.
}

// itemRow is one item's catalog row: the item, its displayed price (the
// station's ladder: last trade, best sell order, best buy order, base),
// the total quantity resting on both sides and the dash an item's side
// cell reads. An item with no orders anywhere is dimmed but stays
// actionable — placing its first order is how a market wakes up. Buy greys
// while no local sell order exists to lift; Buy order stays live.
func (m *Market) itemRow(item content.ItemID, ords []sim.MarketStock, price float64, hasAsk, gated bool) widgets.TableRow {
	total := 0
	for _, o := range ords {
		total += o.Qty
	}
	row := widgets.TableRow{
		Cells: []string{
			m.cv.TradeableName(item),
			theme.FormatCredits(price),
			theme.FormatInt(int64(total)),
			mktNoSide,
		},
		Icons:    []string{m.itemIcon(item)},
		IconSize: mktRowIcon,
		Data:     mktItemRow{Item: item},
		Spec:     m.rowSpec(item, gated, !hasAsk),
	}
	if len(ords) == 0 {
		row.TextTint = m.th.Pal().Dim
	}
	return row
}

// resultRow is one ranked search hit: flat, no folds, its name split into
// styled runs at the fuzzy-matched character positions (the accent tint),
// with the price, the qty and the group path trailing in the dim step. The
// row keeps Show Info; trade verbs stay in the branch view the highlight
// leads to.
func (m *Market) resultRow(h mktHit, total int, price float64) widgets.TableRow {
	name := m.cv.TradeableName(h.Item)
	segs := mktNameSegs(name, h.Match, m.th.Pal().Accent, m.th.Pal().Text)
	meta := theme.FormatCredits(price) + " · " + theme.FormatInt(int64(total))
	meta += "  " + m.group[h.Item]
	segs = append(segs, widgets.RowSeg{Text: "   " + meta, Tint: m.th.Pal().Dim, Step: theme.StepTiny})
	return widgets.TableRow{
		Cells:    []string{name, theme.FormatCredits(price), theme.FormatInt(int64(total)), mktNoSide},
		Icons:    []string{m.itemIcon(h.Item)},
		IconSize: mktRowIcon,
		Data:     mktItemRow{Item: h.Item},
		Spec:     widgets.RowSpec{Ref: widgets.RowRef(m.cv, h.Item)},
		Stack:    []widgets.RowLine{{Segs: segs}},
	}
}

// mktNameSegs splits a display name into styled runs at the matched byte
// positions: matched characters in the accent, the rest in text colour.
func mktNameSegs(name string, match []int, accent, text color.RGBA) []widgets.RowSeg {
	if len(match) == 0 {
		return []widgets.RowSeg{{Text: name}}
	}
	hit := make(map[int]bool, len(match))
	for _, i := range match {
		if i >= 0 && i < len(name) {
			hit[i] = true
		}
	}
	var out []widgets.RowSeg
	var b strings.Builder
	on := false
	flush := func() {
		if b.Len() == 0 {
			return
		}
		tint := text
		if on {
			tint = accent
		}
		out = append(out, widgets.RowSeg{Text: b.String(), Tint: tint})
		b.Reset()
	}
	for i, r := range name {
		if hit[i] != on {
			flush()
			on = hit[i]
		}
		b.WriteRune(r)
	}
	flush()
	return out
}

// bookRow is one resting order as a row of the right pane's book table: the
// book's own [side, price, qty] cells (aligned columns, the figures in the
// tabular face). The Side cell reads the compact verb, Buy or Sell. The
// player's own orders carry the owned-row green, so one glance separates
// them from the NPC/agent rows (the overview's own-ship/hostile tint
// convention).
func (m *Market) bookRow(o sim.MarketStock) widgets.PlainRow {
	p := m.th.Pal()
	side, tint := "Sell", p.Warn
	if o.Side == sim.OrderBuy {
		side, tint = "Buy", p.Info
	}
	if o.Owner == sim.OwnerPlayer {
		tint = theme.OwnedRowTint(p)
	}
	return widgets.PlainRow{
		Cells: []string{side, theme.FormatCredits(o.Price), theme.FormatInt(int64(o.Qty))},
		Tint:  tint,
	}
}

// rowSpec is every market row's declared interactions — an item row offers
// the trade verbs (Buy/Sell open the quantity picker on it, the two
// order-window launches post to its book). The menu's actions carry the
// row's own item, so Buy/Sell always trade the row they were opened on —
// no click-time selection side effects. buyOff grays Buy while the local
// book holds no sell order to lift (Buy order stays live).
func (m *Market) rowSpec(item content.ItemID, gated, buyOff bool) widgets.RowSpec {
	return widgets.RowSpec{
		Ref: widgets.RowRef(m.cv, item),
		Actions: []widgets.RowAction{
			{Label: "Buy", Disabled: gated || buyOff, Intent: widgets.QtyPick{Mode: widgets.QtyBuy, Item: item}},
			{Label: "Sell", Disabled: gated, Intent: widgets.QtyPick{Mode: widgets.QtySell, Item: item}},
			{Label: "Sell order", Disabled: gated,
				Intent: widgets.OpenOrder{Item: item, Side: sim.OrderSell, Qty: m.cv.PlayerStock(item)}},
			{Label: "Buy order", Disabled: gated,
				Intent: widgets.OpenOrder{Item: item, Side: sim.OrderBuy, Qty: 1}},
		},
	}
}

// docked reports whether the trade verbs are available: the sim's own
// market-service access (docked at a station that offers and still runs the
// market), never a bare "is the player docked".
func (m *Market) docked() bool {
	return m.cv.DockedMarket() != ""
}

// buyClicked opens the quantity picker on the selected item's Buy (the
// pane's pinned button acts on the selection). The row menu's
// trade verbs ride each row's RowSpec intent — same picker, same gate, on
// the row they were opened on.
func (m *Market) buyClicked() {
	if !m.docked() || !m.hasSel {
		return
	}
	m.as.Do(widgets.QtyPick{Mode: widgets.QtyBuy, Item: m.sel})
}

// sellClicked opens the quantity picker on the selected item's Sell.
func (m *Market) sellClicked() {
	if !m.docked() || !m.hasSel {
		return
	}
	m.as.Do(widgets.QtyPick{Mode: widgets.QtySell, Item: m.sel})
}

// mineOrders returns the player's resting orders at the docked station —
// the one book the market window shows (station-local, exactly like the
// trade buttons).
func (m *Market) mineOrders() []sim.MarketStock {
	st := m.cv.DockedMarket()
	if st == "" {
		return nil
	}
	var out []sim.MarketStock
	for _, o := range m.cv.BookOrders(st) {
		if o.Owner == sim.OwnerPlayer {
			out = append(out, o)
		}
	}
	return out
}

// syncSide rebuilds the right pane's content only when it moved: the selected
// item's head (icon, name, description), its full book — sell orders
// ascending, then buy orders descending, the table's canonical order — over
// one rich row per player
// order at the docked station, every order listed, its Cancel button the
// row's trailing action. Both sections carry the same docked gate the trade
// actions do. No selection hides the head's art and names the gesture.
func (m *Market) syncSide(gated bool, by map[content.ItemID][]sim.MarketStock) {
	orders := m.mineOrders()
	var book []sim.MarketStock
	if m.hasSel {
		book = by[m.sel]
	}
	inner := m.sideInnerW()
	key := sideKey(m.sel, m.hasSel, book, orders, gated, inner)
	if key == m.sideKey {
		return
	}
	m.sideKey = key

	// The head: the item's hero icon (the Show Info item size), its name and
	// its description; the hint alone with no selection.
	icon, name, prose := "", "", ""
	if m.hasSel {
		icon, name, prose = m.itemIcon(m.sel), m.cv.TradeableName(m.sel), itemProse(m.cv, m.sel)
	}
	m.set(m.hasSel, icon, name, prose, inner)

	m.book = m.book[:0]
	if m.hasSel {
		for _, o := range book {
			m.book = append(m.book, m.bookRow(o))
		}
	}
	m.bookTbl.SetRows(m.book)

	m.mineBox.RemoveChildren()
	m.mineRows = m.mineRows[:0]
	for _, o := range orders {
		r := m.orderRow(o, gated)
		m.mineRows = append(m.mineRows, r)
		m.mineBox.AddChild(r.Root)
	}
	m.retitle(m.mineLbl, "MY ORDERS · "+theme.FormatInt(int64(len(orders))))
	m.sideBox.RequestRelayout()
}

// sideInnerW is the right pane's text width inside its ContentsPad inset: the
// scroll viewport's own width once laid out, else the landing width less the
// rail gutter and the pane's inset.
func (m *Market) sideInnerW() int {
	if r := m.sideScroll.ViewRect(); r.Dx() > 0 {
		return r.Dx() - 2*theme.Px(theme.ContentsPad)
	}
	return theme.Px(mktSideW - theme.ScrollGutter - 2*theme.ContentsPad)
}

// retitle sets a section bar's tracked label and re-pins its cell to the new
// measurement, so the rule still starts after the text (sectionBarText's
// contract).
func (m *Market) retitle(head *theme.ChromeText, label string) {
	if head.Label == label {
		return
	}
	head.Label = label
	head.GetWidget().MinWidth = int(math.Ceil(
		theme.MeasureTracked(label, *m.th.FaceMono(theme.StepSmall), m.th.TS().Small)))
}

// orderRow is one of the player's orders as a rich row: the item beside
// the Cancel button on line 1, the side, price and remaining quantity on
// line 2.
func (m *Market) orderRow(o sim.MarketStock, gated bool) *widgets.StackRow {
	p := m.th.Pal()
	side, tint := "Sell", p.Warn
	if o.Side == sim.OrderBuy {
		side, tint = "Buy", p.Info
	}
	row := widgets.TableRow{
		Cells: []string{m.cv.TradeableName(o.Item), side, theme.FormatCredits(o.Price), theme.FormatInt(int64(o.Qty))},
		Data:  o.ID,
		Stack: []widgets.RowLine{
			{Segs: []widgets.RowSeg{{Text: m.cv.TradeableName(o.Item), Step: theme.StepSmall, Tint: p.Accent}}},
			{Segs: []widgets.RowSeg{
				{Text: side, Tint: tint, Step: theme.StepTiny},
				{Text: theme.FormatCredits(o.Price), Step: theme.StepTiny},
				{Text: "×" + theme.FormatInt(int64(o.Qty)), Step: theme.StepTiny},
			}},
		},
		Spec: widgets.RowSpec{Actions: []widgets.RowAction{
			{Label: "Cancel", Disabled: gated, Intent: widgets.CancelOrder{ID: o.ID}},
		}},
	}
	return widgets.NewStackRow(widgets.NewSeams(m.th, m.cv, m.as, m.ptr), row, m.th.Face(theme.StepSmall))
}

// scrollSide turns wheel notches into the right pane's offset:
// mineWheelRows rows of the measured row height per notch, the same
// 0..1-fraction-of-overflow convention the tables use (a positive notch
// is wheel up, scrolling towards the top of the column). The step uses
// the taller of the book's plain rows and My orders' rich rows, so either
// section scrolls at the same pace.
func (m *Market) scrollSide(notches float64) {
	face := (*m.th.Face(theme.StepSmall)).Metrics()
	rowH := int(math.Ceil(face.HAscent + face.HDescent))
	for _, r := range m.mineRows {
		if _, h := r.Root.PreferredSize(); h > rowH {
			rowH = h
		}
	}
	scrollNotches(m.sideScroll, m.sideRail, rowH, mineWheelRows, notches)
}

// sideKey is the right pane's change signature: the selection, the docked
// gate, the head's measured width and the book's and My orders' every order
// id/side/price/remaining (the book is canonical, so the signature is stable
// across equal states).
func sideKey(sel content.ItemID, hasSel bool, book, mine []sim.MarketStock, gated bool, inner int) string {
	var b strings.Builder
	// Non-empty even with no rows, so the first Sync always builds (the
	// zero-value key must never read as "unchanged").
	b.WriteString("s;")
	b.WriteString(strconv.Itoa(inner))
	b.WriteByte(';')
	if hasSel {
		b.WriteString(string(sel))
	}
	b.WriteByte(';')
	if gated {
		b.WriteString("g;")
	}
	for _, orders := range [][]sim.MarketStock{book, mine} {
		b.WriteByte('|')
		for _, o := range orders {
			b.WriteString(strconv.FormatUint(o.ID, 10))
			b.WriteByte(':')
			b.WriteString(string(o.Side))
			b.WriteByte(':')
			b.WriteString(strconv.FormatFloat(o.Price, 'g', -1, 64))
			b.WriteByte(':')
			b.WriteString(strconv.Itoa(o.Qty))
			b.WriteByte(';')
		}
	}
	return b.String()
}
