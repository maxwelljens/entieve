package screens

import (
	"image/color"
	"math"
	"slices"
	"strings"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// ---------------------------------------------------------------------------
// The browse column: the search field over the group tree
// ---------------------------------------------------------------------------

// browseNode is one branch of a browse tree: Path is the full group path
// ("Ships/Frigates", "Modules/High/Turrets", …), Label its last segment,
// Depth its indent level (top-level groups are 1).
type browseNode struct {
	Path  string
	Label string
	Depth int
}

// browseNodeKey is a browse row's payload (the table rows' stable identity).
type browseNodeKey struct{ Path string }

// browseTree is the market's and the manufacturing board's one browse
// column model: the group tree flattened from the content, the session-only
// fold and highlight state, and the branch scope. Both windows embed it, so
// the tree, its rows and its markers cannot drift apart.
type browseTree struct {
	nodes    []browseNode
	expanded map[string]bool
	branch   string // the selected scope ("" = All)
	treeSel  string // the highlighted branch
}

// newBrowseTree wraps the flattened nodes with All expanded.
func newBrowseTree(nodes []browseNode) browseTree {
	return browseTree{nodes: nodes, expanded: map[string]bool{"": true}}
}

// browseNodes flattens a group tree in depth-first, label-sorted order: a
// parent always precedes its children.
func browseNodes[K comparable](ids []K, group map[K]string) []browseNode {
	type node struct {
		label string
		path  string
		kids  map[string]*node
	}
	root := &node{kids: map[string]*node{}}
	for _, id := range ids {
		g := group[id]
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
	var out []browseNode
	var walk func(n *node, depth int)
	walk = func(n *node, depth int) {
		keys := make([]string, 0, len(n.kids))
		for k := range n.kids {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			kid := n.kids[k]
			out = append(out, browseNode{Path: kid.path, Label: kid.label, Depth: depth})
			walk(kid, depth+1)
		}
	}
	walk(root, 1)
	return out
}

// browseParent is a group path's parent ("" for a top-level path).
func browseParent(path string) string {
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		return path[:i]
	}
	return ""
}

// inBranch reports whether a group path sits in branch or its subtree.
func inBranch(group, branch string) bool {
	return branch == "" || group == branch || strings.HasPrefix(group, branch+"/")
}

// hasKids reports whether any node hangs below path.
func (b *browseTree) hasKids(path string) bool {
	for _, n := range b.nodes {
		if browseParent(n.Path) == path {
			return true
		}
	}
	return false
}

// highlightTree expands the highlighted branch's ancestors so the highlight is
// visible.
func (b *browseTree) highlightTree() {
	for p := b.treeSel; p != ""; p = browseParent(p) {
		b.expanded[p] = true
	}
}

// selectBranch scopes the list to a branch and folds/unfolds it (a click on
// a tree row).
func (b *browseTree) selectBranch(path string) {
	b.branch, b.treeSel = path, path
	if b.hasKids(path) {
		b.expanded[path] = !b.expanded[path]
	}
}

// reset forgets the session-only state: back to All with everything folded.
func (b *browseTree) reset() {
	clear(b.expanded)
	b.expanded[""] = true
	b.branch, b.treeSel = "", ""
}

// treeRows renders the visible browse rows: All first (always), then every node
// whose ancestors are all expanded.
func (b *browseTree) treeRows() []widgets.TableRow {
	rows := []widgets.TableRow{b.treeRow("", "All", 0)}
	vis := map[string]bool{"": true}
	for _, n := range b.nodes {
		p := browseParent(n.Path)
		if !vis[p] || !b.expanded[p] {
			continue
		}
		vis[n.Path] = true
		rows = append(rows, b.treeRow(n.Path, n.Label, n.Depth))
	}
	return rows
}

// treeRow is one browse row: indent by depth, an expansion marker when the
// branch has children, then the label.
func (b *browseTree) treeRow(path, label string, depth int) widgets.TableRow {
	mark := "  "
	if b.hasKids(path) {
		if b.expanded[path] {
			mark = "↓ "
		} else {
			mark = "→ "
		}
	}
	return widgets.TableRow{
		Cells: []string{strings.Repeat("    ", depth) + mark + label},
		Data:  browseNodeKey{Path: path},
	}
}

// newBrowseSearch is the browse pane's live search field, the same field in
// both windows (only the placeholder names the catalog).
func newBrowseSearch(env widgets.Seams, placeholder string, height int) *widget.TextInput {
	return widget.NewTextInput(
		widget.TextInputOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(0, theme.Px(height)),
		),
		widget.TextInputOpts.Placeholder(placeholder),
		widget.TextInputOpts.Face(env.Face(theme.StepSmall)),
		widget.TextInputOpts.Padding(&widget.Insets{
			Left: theme.Px(4), Right: theme.Px(4), Top: theme.Px(2), Bottom: theme.Px(2),
		}),
	)
}

// newBrowseTable is the tree itself: a one-column table (the wheel, selection
// and rail the catalogs have) whose header is a fixed grouping label, so the
// sort callback is a no-op and the branch order never moves. The prefs id and
// the column key are the window's own (the layout sidecar's identity).
func newBrowseTable(env widgets.Seams, prefsID, key string) *widgets.Table {
	t := widgets.NewTable(env, []widgets.Column{{Key: key, Title: "Browse"}},
		widgets.PrefsID(prefsID))
	t.SetSortFunc(func(string, bool, []widgets.TableRow) {})
	return t
}

// ---------------------------------------------------------------------------
// The plain scroll column
// ---------------------------------------------------------------------------

// newScrollColumn is the house's plain scroll column: an invisible frame
// over the window body (the mask clips the rows to the viewport, the tables'
// own mask) with the reserved-gutter rail beside it. The caller adds its own
// scrolled handler and calls rail.Sync each frame.
func newScrollColumn(env widgets.Seams, box *widget.Container) (*widget.ScrollContainer, *widgets.ScrollRail) {
	scroll := widget.NewScrollContainer(
		widget.ScrollContainerOpts.Content(box),
		widget.ScrollContainerOpts.StretchContentWidth(),
		widget.ScrollContainerOpts.Image(&widget.ScrollContainerImage{
			Idle: theme.NineSlice(color.NRGBA{}),
			Mask: theme.NineSlice(theme.WindowBody(env.Pal().BG)),
		}),
	)
	// Same stash as the tables: the window chrome's translate walk carries
	// the rows with the viewport instead of re-locating them mid-drag.
	widgets.AttachScrollContent(scroll, box)
	return scroll, widgets.NewScrollRail(env, scroll, scroll, box)
}

// scrollNotches turns wheel notches into a scroll column's offset: rows
// measured row heights per notch, in the tables' 0..1-fraction-of-overflow
// convention (a positive notch is wheel up, towards the top).
func scrollNotches(scroll *widget.ScrollContainer, rail *widgets.ScrollRail, rowH, rows int, notches float64) {
	over := rail.Overflow()
	if over <= 0 {
		scroll.ScrollTop = 0
		return
	}
	if rowH <= 0 {
		return
	}
	top := scroll.ScrollTop - notches*float64(rows)*float64(rowH)/over
	scroll.ScrollTop = math.Max(0, math.Min(1, top))
}
