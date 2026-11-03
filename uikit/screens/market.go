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
// group paths), the catalog list and the right pane, which carries the
// selected item's order book over My orders.
//
// The catalog lists every item of the tradeable catalog once (canonical
// order) with its ladder quote and two-side quantity. Selecting an item
// (the same selection the toolbar's Buy/Sell act on) fills the right
// pane's book section: that item's full resting book, asks (price
// ascending) above bids (price descending), each order in the book's own
// [price, qty, side] form — the fold the catalog used to hide now always
// visible, so the window needs no per-item dropdown. Buy/Sell execute as
// market orders against the book: each opens the quantity picker
// (CONTEXT.md "Quantity picker") on the selected item and Confirm emits
// Buy/Sell. The title bar stays the base display name (window titles are
// furniture); the book's own columns carry the price voice.
//
// The browse tree scopes the catalog to a branch (All is the whole
// catalog) and the search field filters that branch live, ranked by fuzzy
// score with the matched letters highlighted. Selecting an item
// highlights its branch back. Trade is only possible while the player is
// docked; Buy also needs a local ask to lift, so both buttons can grey
// while docked.
//
// An item with no orders anywhere still gets its row — dimmed, but live:
// placing the first order is how a market wakes up (Request on Market
// works from an empty book). Placing a hull ask is refused by the sim, so
// hull rows trade the way they always did. Other stations' books are read
// read-only for the availability suffix; the columns stay local.
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
	nodes []mktNode

	// The browse pane: tree is the branch list, search its live filter,
	// branch the selected path ("" = All), expanded the session-only
	// branch state, treeSel the highlighted branch (a list selection
	// highlights the item's own group).
	tree     *widgets.Table
	search   *widget.TextInput
	branch   string
	treeSel  string
	expanded map[string]bool

	// Search results: the ranked hits of the current query, in order, and
	// hit the keyboard-highlighted index (-1 = none).
	results []mktHit
	query   string
	hit     int

	// Availability: every other market station's asks per item (the Qty
	// suffix), rebuilt each
	// Sync while docked. stations is the universe's market roster.
	stations []sim.StationID
	avail    map[content.ItemID]mktAvail

	// The right pane: two labelled sections in one scroll column. book is
	// the selected item's resting orders (row specs, book order) and the
	// My orders rows are one rich row per player order at the docked
	// station, every one of them listed. The pane's scroll and its house
	// rail cover both sections.
	bookLbl  *widget.Text
	bookBox  *widget.Container
	book     []widgets.TableRow
	bookRows []*widgets.StackRow

	mineLbl  *widget.Text
	mineBox  *widget.Container
	mineRows []*widgets.StackRow

	sideScroll *widget.ScrollContainer
	sideRail   *widgets.ScrollRail
	sideKey    string // change signature of the right pane's rows

	sel    content.ItemID
	hasSel bool
}

// mktNoSide is an item row's Side cell: an item has no side of its own.
// The Qty column already reads 0 for a dead market, so the cell stays a
// dash rather than an empty box.
const mktNoSide = "—"

// mktNode is one branch of the browse tree: Path is the full group path
// ("" is never a node — the window renders its own All root), Label the
// last segment, Depth its indent level (top-level groups are 1).
type mktNode struct {
	Path  string
	Label string
	Depth int
}

// mktNodeKey is a tree row's payload (the table rows' stable identity).
type mktNodeKey struct{ Path string }

// mktHit is one ranked search result: the item and the matched byte
// positions within its display name (for the highlight runs).
type mktHit struct {
	Item  content.ItemID
	Match []int
}

// mktAvail is one item's cross-station availability: the number of OTHER
// market stations whose books hold an ask, plus the cheapest such ask and
// its station.
type mktAvail struct {
	Stations int
	Cheapest float64
	Station  sim.StationID
}

// marketStations lists every station that offers the market service, in
// deterministic order (system id, then celestial order) — the roster the
// availability read walks with BookOrders.
func marketStations(d *content.Content) []sim.StationID {
	if d == nil {
		return nil
	}
	sysIDs := make([]content.SystemID, 0, len(d.Systems))
	for id := range d.Systems {
		sysIDs = append(sysIDs, id)
	}
	slices.Sort(sysIDs)
	var out []sim.StationID
	for _, id := range sysIDs {
		for _, cel := range d.Systems[id].Celestials {
			if cel.Station != nil && slices.Contains(cel.Station.Services, content.SvcMarket) {
				out = append(out, sim.StationID(cel.ID))
			}
		}
	}
	return out
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

// mktNodes flattens the group tree in depth-first, label-sorted order: a
// parent always precedes its children.
func mktNodes(items []content.ItemID, group map[content.ItemID]string) []mktNode {
	type node struct {
		label string
		path  string
		kids  map[string]*node
	}
	root := &node{kids: map[string]*node{}}
	for _, item := range items {
		g := group[item]
		if g == "" {
			continue
		}
		cur := root
		path := ""
		for _, seg := range strings.Split(g, "/") {
			if path == "" {
				path = seg
			} else {
				path += "/" + seg
			}
			kid := cur.kids[seg]
			if kid == nil {
				kid = &node{label: seg, path: path, kids: map[string]*node{}}
				cur.kids[seg] = kid
			}
			cur = kid
		}
	}
	var out []mktNode
	var walk func(n *node, depth int)
	walk = func(n *node, depth int) {
		keys := make([]string, 0, len(n.kids))
		for k := range n.kids {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			kid := n.kids[k]
			out = append(out, mktNode{Path: kid.path, Label: kid.label, Depth: depth})
			walk(kid, depth+1)
		}
	}
	walk(root, 1)
	return out
}

// mktParent is a group path's parent ("" for a top-level path).
func mktParent(path string) string {
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		return path[:i]
	}
	return ""
}

// hasKids reports whether any node hangs below path.
func (m *Market) hasKids(path string) bool {
	for _, n := range m.nodes {
		if mktParent(n.Path) == path {
			return true
		}
	}
	return false
}

// highlightTree expands the highlighted branch's ancestors so the
// highlight is visible.
func (m *Market) highlightTree() {
	for p := m.treeSel; p != ""; p = mktParent(p) {
		m.expanded[p] = true
	}
}

// treeRows renders the visible browse rows: All first (always), then every
// node whose ancestors are all expanded.
func (m *Market) treeRows() []widgets.TableRow {
	rows := []widgets.TableRow{m.treeRow("", "All", 0)}
	vis := map[string]bool{"": true}
	for _, n := range m.nodes {
		p := mktParent(n.Path)
		if !vis[p] || !m.expanded[p] {
			continue
		}
		vis[n.Path] = true
		rows = append(rows, m.treeRow(n.Path, n.Label, n.Depth))
	}
	return rows
}

// treeRow is one browse row: indent by depth, an expansion marker when the
// branch has children, then the label.
func (m *Market) treeRow(path, label string, depth int) widgets.TableRow {
	mark := "  "
	if m.hasKids(path) {
		if m.expanded[path] {
			mark = "↓ "
		} else {
			mark = "→ "
		}
	}
	return widgets.TableRow{
		Cells: []string{strings.Repeat("    ", depth) + mark + label},
		Data:  mktNodeKey{Path: path},
	}
}

// scopeItems is the catalog narrowed to the selected branch: All is the
// whole catalog, any other branch its own subtree.
func (m *Market) scopeItems() []content.ItemID {
	if m.branch == "" {
		return m.items
	}
	out := make([]content.ItemID, 0, len(m.items))
	for _, item := range m.items {
		g := m.group[item]
		if g == m.branch || strings.HasPrefix(g, m.branch+"/") {
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

// crossAvail reads every other market station's asks for the catalog: the
// count of stations stocking each item and the cheapest of them. It only
// runs while docked (in space there is no local book to compare against).
func (m *Market) crossAvail(local sim.StationID) map[content.ItemID]mktAvail {
	if local == "" {
		return nil
	}
	out := make(map[content.ItemID]mktAvail)
	for _, st := range m.stations {
		if st == local {
			continue
		}
		for _, o := range m.cv.BookOrders(st) {
			if o.Side != sim.OrderSell {
				continue
			}
			a := out[o.Item]
			a.Stations++
			if a.Cheapest == 0 || o.Price < a.Cheapest {
				a.Cheapest, a.Station = o.Price, st
			}
			out[o.Item] = a
		}
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

// mktCell is a row's cell text by column index ("" when the row is short).
func mktCell(row widgets.TableRow, i int) string {
	if i < 0 || i >= len(row.Cells) {
		return ""
	}
	return row.Cells[i]
}

// NewMarket builds the Market window over the shared environment. Mount
// it on a Kit with Kit.AddWindow and call Sync every frame.
func NewMarket(env widgets.Seams) *Market {
	m := &Market{
		th: env, cv: env, as: env, ptr: env.Pointer(),
		items:    marketCatalog(env),
		expanded: map[string]bool{"": true},
		hit:      -1,
	}
	m.group, m.maker = marketInfo(env.Defs(), m.items)
	m.nodes = mktNodes(m.items, m.group)
	m.stations = marketStations(env.Defs())
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
	m.tbl.SetSortFunc(func(key string, asc bool, rows []widgets.TableRow) {
		if m.query != "" {
			return
		}
		idx := -1
		for i, c := range cols {
			if c.Key == key {
				idx = i
				break
			}
		}
		if idx < 0 {
			return
		}
		if key == "Item" {
			// The rows arrive in canonical id order: the Item key keeps
			// it, reversing on a second click.
			if !asc {
				slices.Reverse(rows)
			}
			return
		}
		sort.SliceStable(rows, func(a, b int) bool {
			x, y := mktCell(rows[a], idx), mktCell(rows[b], idx)
			if asc {
				return x < y
			}
			return y < x
		})
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

	// The browse pane: a live search field over the content tree. The
	// tree is a one-column table (the same wheel, selection and rail the
	// catalog has); its header is a fixed grouping label, so the sort
	// callback is a no-op and the branch order never moves.
	m.search = widget.NewTextInput(
		widget.TextInputOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(0, theme.Px(mktSearchH)),
		),
		widget.TextInputOpts.Placeholder("Search"),
		widget.TextInputOpts.Face(env.Face(theme.StepSmall)),
		widget.TextInputOpts.Padding(&widget.Insets{
			Left: theme.Px(4), Right: theme.Px(4), Top: theme.Px(2), Bottom: theme.Px(2),
		}),
	)
	m.tree = widgets.NewTable(env, []widgets.Column{{Key: "Tree", Title: "Browse"}}, widgets.PrefsID("tree"))
	m.tree.SetSortFunc(func(string, bool, []widgets.TableRow) {})
	m.tree.OnSelect(func(r widgets.TableRow) {
		m.search.Focus(false)
		n, ok := r.Data.(mktNodeKey)
		if !ok {
			return
		}
		m.branch, m.treeSel = n.Path, n.Path
		if m.hasKids(n.Path) {
			m.expanded[n.Path] = !m.expanded[n.Path]
		}
	})

	m.buy = theme.ActionButton("Buy", m.buyClicked)
	m.sell = theme.ActionButton("Sell", m.sellClicked)
	tradeBar := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.ContentsPad)),
		)),
	)
	tradeBar.AddChild(m.buy, m.sell)

	// The catalog row: the item table — widgets.Table owns its scroll
	// viewport, its wheel wiring AND the scrollbar rail beside it (the
	// reserved-gutter chrome, ScrollGutter design px: the handle can never
	// overhang a row, and the rail hides while the list fits). The title
	// line and the Buy/Sell row above stay exactly as they are.
	catalog := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout()))
	m.tbl.Container().GetWidget().LayoutData = widget.AnchorLayoutData{
		StretchHorizontal: true,
		StretchVertical:   true,
	}
	catalog.AddChild(m.tbl.Container())

	// The right pane: two labelled sections in ONE scroll column — the
	// selected item's order book over My orders — riding the house rail
	// (the reserved gutter hides while both sections fit). The rows are
	// rich rows tiled by the table module's StackLayout (the contract
	// board's row form); no fold and no tabs, so a selection shows its
	// book at once.
	m.bookLbl = widget.NewText(
		widget.TextOpts.Text("Order book", env.Face(theme.StepTiny), theme.Solid(p.Accent)),
	)
	m.bookBox = widget.NewContainer(
		widget.ContainerOpts.Layout(widgets.StackLayout{Gap: theme.Px(theme.ContentsPad)}),
	)
	m.mineLbl = widget.NewText(
		widget.TextOpts.Text("My orders", env.Face(theme.StepTiny), theme.Solid(p.Accent)),
	)
	m.mineBox = widget.NewContainer(
		widget.ContainerOpts.Layout(widgets.StackLayout{Gap: theme.Px(theme.ContentsPad)}),
	)
	sideBox := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, false, false, false}),
			widget.GridLayoutOpts.Spacing(0, theme.Px(theme.ContentsPad)),
		)),
	)
	sideBox.AddChild(m.bookLbl, m.bookBox, m.mineLbl, m.mineBox)
	m.sideScroll = widget.NewScrollContainer(
		widget.ScrollContainerOpts.Content(sideBox),
		widget.ScrollContainerOpts.StretchContentWidth(),
		// No chrome of the column's own: an invisible frame, the mask
		// clipping the rows to the viewport (the tables' own mask).
		widget.ScrollContainerOpts.Image(&widget.ScrollContainerImage{
			Idle: theme.NineSlice(color.NRGBA{}),
			Mask: theme.NineSlice(theme.WindowBody(p.BG)),
		}),
	)
	// Wheel scrolls the column; ScrollTop is a 0..1 fraction of the
	// overflow, so the step scales by the measured overflow (the tables'
	// convention). The rail beside it drags the same offset.
	m.sideScroll.GetWidget().ScrolledEvent.AddHandler(func(args any) {
		if ev, ok := args.(*widget.WidgetScrolledEventArgs); ok {
			m.scrollSide(ev.Y)
		}
	})
	// Same stash as the tables: the window chrome's translate walk carries
	// the rows with the viewport instead of re-locating them mid-drag.
	widgets.AttachScrollContent(m.sideScroll, sideBox)
	m.sideRail = widgets.NewScrollRail(env, m.sideScroll, m.sideScroll, sideBox)

	// The right pane spans the full body height (its one scroll column
	// carries the book and My orders); the catalog keeps the full row
	// height it always had, its scrollbar rail included.
	left := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, true}),
			widget.GridLayoutOpts.Spacing(0, theme.Px(4)),
		)),
	)
	left.AddChild(tradeBar, catalog)

	// The browse pane: the search field over the tree, both full height.
	browse := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, true}),
			widget.GridLayoutOpts.Spacing(0, theme.Px(4)),
		)),
	)
	browse.AddChild(m.search, m.tree.Container())

	// Body: browse | seam | catalog | seam | right pane. Two draggable
	// seams; the tree and right panes open at their historical widths
	// (176 px of browse, the old 240 px column) and each pane keeps
	// SplitMinPane.
	m.split = widgets.NewSplitPaneRow(env, browse, left, m.sideRail.Container())
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
	clear(m.expanded)
	m.expanded[""] = true
	m.branch, m.treeSel = "", ""
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
	// The other stations' books (dock the availability read): the Qty
	// suffix counts them.
	m.avail = m.crossAvail(docked)
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
			rows = append(rows, m.resultRow(h, total, quote(h.Item), ask(h.Item)))
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
	m.tree.SelectData(mktNodeKey{Path: m.treeSel})
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
	// Buy additionally needs a local ask to lift — an item no one sells
	// here stays visible and requestable, its Buy grayed.
	m.buy.GetWidget().Disabled = gated || !m.hasSel || !ask(m.sel)
	m.sell.GetWidget().Disabled = gated
	// The catalog's own rail mirrors the table's scroll in m.tbl.Sync
	// above: hidden while the list fits, thumb at ScrollTop otherwise; a
	// drag writes the offset back.
}

// itemRow is one item's catalog row: the item, its displayed price (the
// station's ladder: last trade, best ask, best bid, base), the total
// quantity resting on both sides (with the dim "· N more" suffix for the
// other stations that stock it) and the dash an item's side cell reads.
// An item with no orders anywhere is dimmed but stays actionable —
// placing its first order is how a market wakes up. Buy greys while no
// local ask exists to lift; Request on Market stays live.
func (m *Market) itemRow(item content.ItemID, ords []sim.MarketStock, price float64, hasAsk, gated bool) widgets.TableRow {
	total := 0
	for _, o := range ords {
		total += o.Qty
	}
	qty := theme.FormatInt(int64(total))
	row := widgets.TableRow{
		Cells: []string{
			m.cv.TradeableName(item),
			theme.FormatCredits(price),
			qty,
			mktNoSide,
		},
		Icons:    []string{m.itemIcon(item)},
		IconSize: mktRowIcon,
		Data:     mktItemRow{Item: item},
		Spec:     m.rowSpec(item, gated, !hasAsk),
	}
	if a, ok := m.avail[item]; ok && a.Stations > 0 {
		row.CellRuns = make([][]widgets.RowSeg, 4)
		row.CellRuns[2] = []widgets.RowSeg{
			{Text: qty},
			{Text: " · " + strconv.Itoa(a.Stations) + " more", Tint: m.th.Pal().Dim, Step: theme.StepTiny},
		}
	}
	if len(ords) == 0 {
		row.TextTint = m.th.Pal().Dim
	}
	return row
}

// resultRow is one ranked search hit: flat, no folds, its name split into
// styled runs at the fuzzy-matched character positions (the accent tint),
// with the price, the qty (and its cross-station suffix) and the group
// path trailing in the dim step. The row keeps Show Info; trade verbs
// stay in the branch view the highlight leads to.
func (m *Market) resultRow(h mktHit, total int, price float64, hasAsk bool) widgets.TableRow {
	name := m.cv.TradeableName(h.Item)
	segs := mktNameSegs(name, h.Match, m.th.Pal().Accent, m.th.Pal().Text)
	meta := theme.FormatCredits(price) + " · " + theme.FormatInt(int64(total))
	if a, ok := m.avail[h.Item]; ok && a.Stations > 0 {
		meta += " · " + strconv.Itoa(a.Stations) + " more"
	}
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

// bookRow is one resting order as a row of the right pane's book section:
// the book's own [price, qty, side] cells with a one-line rich-row run
// (side, price, ×qty - the same form My orders' row line uses). The
// player's own orders carry the owned-row green — one glance separates
// them from the NPC/agent rows (the overview's own-ship/hostile tint
// convention).
func (m *Market) bookRow(o sim.MarketStock) widgets.TableRow {
	p := m.th.Pal()
	side, tint := "Ask", p.Warn
	if o.Side == sim.OrderBuy {
		side, tint = "Bid", p.Info
	}
	if o.Owner == sim.OwnerPlayer {
		tint = theme.OwnedRowTint(p)
	}
	price := theme.FormatCredits(o.Price)
	qty := theme.FormatInt(int64(o.Qty))
	return widgets.TableRow{
		Cells: []string{"", price, qty, side},
		Stack: []widgets.RowLine{{Segs: []widgets.RowSeg{
			{Text: side, Tint: tint, Step: theme.StepTiny},
			{Text: price, Tint: tint, Step: theme.StepTiny},
			{Text: "×" + qty, Tint: tint, Step: theme.StepTiny},
		}}},
	}
}

// rowSpec is every market row's declared interactions — an item row offers
// the trade verbs (Buy/Sell open the quantity picker on it, the two
// order-window launches post to its book). The menu's actions carry the
// row's own item, so Buy/Sell always trade the row they were opened on —
// no click-time selection side effects. buyOff grays Buy while the local
// book holds no ask to lift (Request on Market stays live).
func (m *Market) rowSpec(item content.ItemID, gated, buyOff bool) widgets.RowSpec {
	return widgets.RowSpec{
		Ref: widgets.RowRef(m.cv, item),
		Actions: []widgets.RowAction{
			{Label: "Buy", Disabled: gated || buyOff, Intent: widgets.QtyPick{Mode: widgets.QtyBuy, Item: item}},
			{Label: "Sell", Disabled: gated, Intent: widgets.QtyPick{Mode: widgets.QtySell, Item: item}},
			{Label: "Sell on Market", Disabled: gated,
				Intent: widgets.OpenOrder{Item: item, Side: sim.OrderSell, Qty: m.cv.PlayerStock(item)}},
			{Label: "Request on Market", Disabled: gated,
				Intent: widgets.OpenOrder{Item: item, Side: sim.OrderBuy, Qty: 1}},
		},
	}
}

// docked reports whether the player is docked (the trade precondition).
func (m *Market) docked() bool {
	return m.cv.PlayerDocked()
}

// buyClicked opens the quantity picker on the selected item's Buy (the
// toolbar's own button acts on the left-click selection). The row menu's
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

// syncSide rebuilds the right pane's two sections only when their content
// (or the selection, or the docked gate) moved: the selected item's full
// book — asks ascending, then bids descending, the table's canonical
// order — over one rich row per player order at the docked station, every
// order listed, its Cancel button the row's trailing action. Both carry
// the same docked gate the trade actions do. No selection means the book
// bar names the gesture and no book rows exist.
func (m *Market) syncSide(gated bool, by map[content.ItemID][]sim.MarketStock) {
	orders := m.mineOrders()
	var book []sim.MarketStock
	if m.hasSel {
		book = by[m.sel]
	}
	key := sideKey(m.sel, m.hasSel, book, orders, gated)
	if key == m.sideKey {
		return
	}
	m.sideKey = key

	m.bookBox.RemoveChildren()
	m.book, m.bookRows = m.book[:0], m.bookRows[:0]
	if m.hasSel {
		m.bookLbl.Label = "Order book: " + m.cv.TradeableName(m.sel)
		for _, o := range book {
			r := m.bookRow(o)
			m.book = append(m.book, r)
			sr := widgets.NewStackRow(widgets.NewSeams(m.th, m.cv, m.as, m.ptr), r, m.th.FaceMono(theme.StepSmall))
			m.bookRows = append(m.bookRows, sr)
			m.bookBox.AddChild(sr.Root)
		}
	} else {
		m.bookLbl.Label = "Order book: select an item"
	}

	m.mineBox.RemoveChildren()
	m.mineRows = m.mineRows[:0]
	for _, o := range orders {
		r := m.orderRow(o, gated)
		m.mineRows = append(m.mineRows, r)
		m.mineBox.AddChild(r.Root)
	}
	if len(orders) == 0 {
		m.mineLbl.Label = "My orders: none"
	} else {
		m.mineLbl.Label = "My orders: " + theme.FormatInt(int64(len(orders)))
	}
}

// orderRow is one of the player's orders as a rich row: the item beside
// the Cancel button on line 1, the side, price and remaining quantity on
// line 2.
func (m *Market) orderRow(o sim.MarketStock, gated bool) *widgets.StackRow {
	p := m.th.Pal()
	side, tint := "Ask", p.Warn
	if o.Side == sim.OrderBuy {
		side, tint = "Bid", p.Info
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
// the taller of the two sections' rows, so either section scrolls at the
// same pace.
func (m *Market) scrollSide(notches float64) {
	over := m.sideRail.Overflow()
	if over <= 0 {
		m.sideScroll.ScrollTop = 0
		return
	}
	rowH := 0
	for _, rows := range [][]*widgets.StackRow{m.bookRows, m.mineRows} {
		if len(rows) == 0 {
			continue
		}
		if _, h := rows[0].Root.PreferredSize(); h > rowH {
			rowH = h
		}
	}
	if rowH <= 0 {
		return
	}
	top := m.sideScroll.ScrollTop - notches*mineWheelRows*float64(rowH)/over
	m.sideScroll.ScrollTop = math.Max(0, math.Min(1, top))
}

// sideKey is the right pane's change signature: the selection, the docked
// gate and the book's and My orders' every order id/side/price/remaining
// (the book is canonical, so the signature is stable across equal states).
func sideKey(sel content.ItemID, hasSel bool, book, mine []sim.MarketStock, gated bool) string {
	var b strings.Builder
	// Non-empty even with no rows, so the first Sync always builds (the
	// zero-value key must never read as "unchanged").
	b.WriteString("s;")
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
