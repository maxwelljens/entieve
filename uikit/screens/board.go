package screens

// The board scaffold: the frame the three-pane boards share (CONTEXT.md
// "Board scaffold"). The market window, the recipe board and the refinery all
// read the same way: a browse column (the search field over the group tree),
// a list, and a content pane that leads with the item head, scrolls one page
// with the house rail and pins its verb below. Only the page differs per
// window, so the frame lives here once and each window supplies a comparable
// page model: the pane rebuilds when that model moves and never otherwise,
// which makes a displayed input outside the model a stale-page bug by
// construction (the Station board's rule, contacts.go).

import (
	"math"
	"strings"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/sahilm/fuzzy"
)

// ---------------------------------------------------------------------------
// Layout: the browse column over the list and the content pane
// ---------------------------------------------------------------------------

// paneShare is a row split's opening ratio for one pane: the split spans the
// window minus the body padding and both seam tracks, and the seam stores the
// fraction BEFORE it, so a right pane opens at 1-paneShare.
func paneShare(openW, windowW int) float64 {
	return float64(openW) / float64(windowW-2*theme.ContentsPad-2*widgets.SplitSeamH)
}

// newBoardSplit builds a board's three-pane row: the browse column (the live
// search field over the group tree) | the caller's list | the caller's content
// pane, with the two opening shares. list and content arrive built: the list
// is the window's own table (or the wrapper that stretches it), the content is
// its boardPane's box. The seam names stay the window's own (Splits()).
func newBoardSplit(env widgets.Seams, search *widget.TextInput, tree *widgets.Table,
	list, content widget.PreferredSizeLocateableWidget, browseW, paneW, windowW int) *widgets.SplitPane {
	browse := paneStack([]bool{false, true}, search, tree.Container())
	split := widgets.NewSplitPaneRow(env, browse, list, content)
	split.SetDefaultRatio(0, paneShare(browseW, windowW))
	split.SetDefaultRatio(1, 1-paneShare(paneW, windowW))
	return split
}

// ---------------------------------------------------------------------------
// Search: the shared machinery, the caller's policy
// ---------------------------------------------------------------------------

// boardSearchSpec is one board's search policy. The machinery is shared; the
// policy never is: the two inspectors gate on a field that CONTAINS the query
// (a match scattered across two fields is not a hit), the market does not gate
// and draws the matched letters instead.
type boardSearchSpec struct {
	Gate    bool // a hit needs one field containing the query
	Offsets bool // return the matched rune starts of the first field
	// Lead prefixes the haystack with a space: the boards' haystack has led
	// with one since they shipped and their rank is pinned on it.
	Lead bool
}

// boardHit is one ranked result: the caller's key and, when the policy asks
// for them, the matched rune starts of the key's first field (the market's
// highlight runs).
type boardHit[K any] struct {
	Key   K
	Match []int
}

// boardSearch is the boards' one ranked search: fold the query and every
// field (fold.go), apply the caller's gate, rank by sahilm/fuzzy's score and
// map the ranked indexes back to the caller's keys. fields returns the
// candidate's searchable fields in the caller's own order; the haystack is
// their space-joined fold, which is what the library scores.
func boardSearch[K any](q string, keys []K, fields func(K) []string, spec boardSearchSpec) []boardHit[K] {
	fq, _ := foldString(q)
	lq := strings.ToLower(fq)
	hay := make([]string, 0, len(keys))
	kept := make([]K, 0, len(keys))
	names := make([]int, 0, len(keys))   // the first field's folded length
	folds := make([][]int, 0, len(keys)) // ... and its fold map
	for _, k := range keys {
		fs := fields(k)
		hit := !spec.Gate
		var name string
		var fold []int
		line := make([]string, 0, len(fs))
		for i, f := range fs {
			ff, fm := foldString(f)
			if i == 0 {
				name, fold = ff, fm
			}
			line = append(line, ff)
			if !hit && strings.Contains(strings.ToLower(ff), lq) {
				hit = true
			}
		}
		if !hit {
			continue
		}
		h := strings.Join(line, " ")
		if spec.Lead {
			h = " " + h
		}
		hay = append(hay, h)
		kept = append(kept, k)
		names = append(names, len(name))
		folds = append(folds, fold)
	}
	var out []boardHit[K]
	for _, m := range fuzzy.Find(fq, hay) {
		hit := boardHit[K]{Key: kept[m.Index]}
		if spec.Offsets {
			for _, i := range m.MatchedIndexes {
				if i >= names[m.Index] {
					continue
				}
				if folds[m.Index] != nil {
					i = folds[m.Index][i]
				}
				hit.Match = append(hit.Match, i)
			}
		}
		out = append(out, hit)
	}
	return out
}

// ---------------------------------------------------------------------------
// The content pane
// ---------------------------------------------------------------------------

// boardPaneSpec is one board's content pane: the head's empty-state line, the
// pane's opening width (the inner width's fallback before the first layout)
// and where the head sits.
type boardPaneSpec struct {
	Hint  string
	OpenW int
	// InPage carries the head inside the scroll page (the market's right
	// pane) instead of pinning it above the page (the two inspectors).
	InPage bool
}

// boardPane is a board's content pane: the head, one scroll column with the
// house rail, the pinned action bar and the measured inner width. The page is
// the pane's own container. A pinned pane's window clears and refills it on a
// rebuild (a page opens at its top, so a new model never keeps the old offset);
// the market's in-page pane is filled once and its sections are refilled in
// place, so a book move never drops the player's scroll position. The scroll,
// the rail, the bar and their verbs stay the same widgets for the window's
// life.
type boardPane struct {
	env  widgets.Seams
	spec boardPaneSpec
	box  *widget.Container

	head   *itemHead
	page   *widget.Container
	scroll *widget.ScrollContainer
	rail   *widgets.ScrollRail
	bar    *widget.Container
}

// newBoardPane builds the content pane. The pinned shape is the inspectors':
// head, page, verb bar in one inset column. The in-page shape is the market's
// right pane: the page (which opens with the head) over the trade bar, no
// inset of its own because the page carries it.
func newBoardPane(env widgets.Seams, spec boardPaneSpec) *boardPane {
	b := &boardPane{env: env, spec: spec, head: newItemHead(env, spec.Hint)}
	b.bar = newBoardBar(env, spec.InPage)
	if spec.InPage {
		b.page = widget.NewContainer(widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, false, false, false, false}),
			widget.GridLayoutOpts.Spacing(0, theme.Px(theme.ContentsPad)),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)))
		b.scroll, b.rail = newScrollColumn(env, b.page)
		b.box = paneStack([]bool{true, false}, b.rail.Container(), b.bar)
		return b
	}
	b.page = newBoardPageBox()
	b.scroll, b.rail = newScrollColumn(env, b.page)
	b.box = widget.NewContainer(widget.ContainerOpts.Layout(widget.NewGridLayout(
		widget.GridLayoutOpts.Columns(1),
		widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, true, false}),
		widget.GridLayoutOpts.Spacing(0, theme.Px(theme.SpSm)),
		widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
	)))
	b.box.AddChild(b.head.headBox, b.rail.Container(), b.bar)
	return b
}

// newBoardPageBox is a page's body: a plain vertical run, so a section keeps
// its own height instead of sharing the column equally.
func newBoardPageBox() *widget.Container {
	return widget.NewContainer(widget.ContainerOpts.Layout(widget.NewRowLayout(
		widget.RowLayoutOpts.Direction(widget.DirectionVertical),
		widget.RowLayoutOpts.Spacing(theme.Px(theme.SpSm)),
	)))
}

// newBoardBar builds a board's pinned action bar: its verbs in one row under
// the scroll column. The market's bar pads itself clear of the pane's edge and
// the rail gutter (the buttons end on the book's right edge); the inspectors'
// bar sits flush at the pane's inset.
func newBoardBar(env widgets.Seams, inset bool) *widget.Container {
	opts := []widget.RowLayoutOpt{
		widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
	}
	if inset {
		opts = append(opts,
			widget.RowLayoutOpts.Spacing(theme.Px(theme.ContentsPad)),
			widget.RowLayoutOpts.Padding(&widget.Insets{
				Left:   theme.Px(theme.ContentsPad),
				Right:  theme.Px(theme.ContentsPad + theme.ScrollGutter),
				Bottom: theme.Px(theme.ContentsPad),
			}),
		)
	}
	return widget.NewContainer(widget.ContainerOpts.Layout(widget.NewRowLayout(opts...)))
}

// beginPage clears the page for a rebuild and returns its builder: a page opens
// at the top, so a new model never keeps the old one's offset.
func (b *boardPane) beginPage() *boardPage {
	b.page.RemoveChildren()
	b.scroll.ScrollTop = 0
	return &boardPage{env: b.env, box: b.page}
}

// done re-lays the pane after a rebuild: the head, the page and the bar all
// changed height.
func (b *boardPane) done() { b.box.RequestRelayout() }

// setHead fills the head for one tradeable, wrapped to the pane's inner width.
func (b *boardPane) setHead(hasSel bool, icon, name, prose string) {
	b.head.set(hasSel, icon, name, prose, b.innerW())
}

// innerW is the head's text width inside the pane: the scroll viewport's own
// width once laid out, else the opening width less the pane's inset. A pinned
// head spans the rail gutter the page gives up; a page-borne head rides the
// same inset as the page, so it gives the gutter up too.
func (b *boardPane) innerW() int {
	if r := b.scroll.ViewRect(); r.Dx() > 0 {
		if b.spec.InPage {
			return r.Dx() - 2*theme.Px(theme.ContentsPad)
		}
		return r.Dx() + theme.Px(theme.ScrollGutter)
	}
	if b.spec.InPage {
		return theme.Px(b.spec.OpenW - theme.ScrollGutter - 2*theme.ContentsPad)
	}
	return theme.Px(b.spec.OpenW - 2*theme.ContentsPad)
}

// clampScroll keeps the offset honest: the rail mirrors it, and a page that
// fits again loses a stale offset (the tables' rule).
func (b *boardPane) clampScroll() {
	b.rail.Sync()
	if b.rail.Overflow() <= 0 {
		b.scroll.ScrollTop = 0
	}
}

// wheel turns wheel notches into the page's offset: rows of rowH per notch, the
// tables' 0..1-fraction-of-overflow convention.
func (b *boardPane) wheel(rowH, rows int, notches float64) {
	scrollNotches(b.scroll, b.rail, rowH, rows, notches)
}

// boardPage is one page of a board's scroll column: the window adds section
// bars, aligned tables, labels and rich rows in reading order.
type boardPage struct {
	env widgets.Seams
	box *widget.Container
}

// bar adds a section bar and hands back the bar and its tracked label, for a
// caller that retitles it (setBarLabel).
func (p *boardPage) bar(label string) (*widget.Container, *theme.ChromeText) {
	bar, lbl := sectionBarText(p.env, label)
	p.box.AddChild(bar)
	return bar, lbl
}

// table adds a section bar over a plain table: the boards' money readouts.
func (p *boardPage) table(label string, t *widgets.PlainTable) {
	p.box.AddChild(sectionBar(p.env, label), t.Container())
}

// label adds the boards' sub-section line: the accent caption above a block of
// rich rows (the bill's and the yield's own heading).
func (p *boardPage) label(text string) *widget.Text {
	lbl := widget.NewText(widget.TextOpts.Text(text,
		p.env.Face(theme.StepTiny), theme.Solid(p.env.Pal().Accent)))
	p.box.AddChild(lbl)
	return lbl
}

// add appends any built child in reading order.
func (p *boardPage) add(ws ...widget.PreferredSizeLocateableWidget) { p.box.AddChild(ws...) }

// stack appends built rich rows in reading order.
func (p *boardPage) stack(rs ...*widgets.StackRow) {
	for _, r := range rs {
		p.box.AddChild(r.Root)
	}
}

// setBarLabel retitles a section bar and re-pins its label cell to the new
// tracked measure, so the rule still starts after the text (sectionBarText's
// contract).
func setBarLabel(th theme.Theme, head *theme.ChromeText, label string) {
	if head.Label == label {
		return
	}
	head.Label = label
	head.GetWidget().MinWidth = int(math.Ceil(
		theme.MeasureTracked(label, *th.FaceMono(theme.StepSmall), th.TS().Small)))
}

// ---------------------------------------------------------------------------
// The rebuild rule
// ---------------------------------------------------------------------------

// boardSync is the boards' refresh rule: compare the next page model with the
// last one and rebuild only when it moved. Every state a page displays belongs
// in its model, so an unchanged model means an unchanged page.
func boardSync[M interface{ same(M) bool }](prev *M, next M, rebuild func(M)) {
	if next.same(*prev) {
		return
	}
	*prev = next
	rebuild(next)
}

// quoteSame reports whether two quote reads draw the same figures: the walk
// and whether a book priced the side. The legs are the walk's own parts and
// never reach a page.
func quoteSame(a, b sim.Quote) bool { return a.Walk == b.Walk && a.Priced == b.Priced }
