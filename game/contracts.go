package game

import (
	"fmt"
	"image"
	"math"
	"slices"
	"sort"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	uikit "github.com/entieve/entieve/uikit/screens"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// Station contract-board geometry (docs/GAMEPLAY.md, "Contracts &
// standing"). The station services window owns the left column while
// docked: x ∈ [stationX, stationX+stationWinW) starts clear of the
// navigation rail's gutter and keeps its right edge at 404, so the
// market still starts at x=406 — the composition right of the column is
// bit-identical to the pre-rail layout. The content-driven height (row
// caps included) keeps the bottom above the log, whose top sits at
// screen-height-250 — i.e. y=450 at the 1024×700 minimum. Both are
// pinned by tests at 1280×720, 1024×700 and at the window minimum.
const (
	stationX     = 10 + uikit.RailInset // window left edge: rail gutter first
	stationY     = 8
	stationWinW  = 394 - uikit.RailInset // window width; right edge still 404
	stationMinW  = 394 - uikit.RailInset // window MinSize — the "minimum window" row-rect pin
	stationMinH  = 200
	boardPad     = 6 // body padding
	boardSpacing = 5 // vertical gap between body children
	boardRowGap  = 4 // gap between two board rows

	// The rows themselves are rich rows of the table module, which owns
	// their metrics (widgets.Rich*): the board's column budgets below
	// derive from them, so a rendered row and its ellipsis budgets can
	// never drift apart.
	boardRowPad  = widgets.RichPad  // horizontal padding inside a row
	boardBtnGap  = widgets.RichGap  // text ↔ action button gap inside a row
	boardBtnW    = widgets.RichBtnW // action button width (fits the widest label, "Accepted", set in the emphasis face)
	availMaxRows = 3                // visible available rows ("+N more" beyond)
	actMaxRows   = 2                // visible active rows ("+N more" beyond)

	boardInnerW   = stationWinW - 2*boardPad                // body inner width: 382
	boardContentW = boardInnerW - 2*boardRowPad             // row content width: 376
	boardTextW    = boardContentW - boardBtnW - boardBtnGap // text column width: 278
)

// boardRowRects tiles up to n contract rows inside inner: equal heights,
// equal boardRowGap gaps, first row flush with inner's top — every rect
// pairwise disjoint and fully inside inner. n <= 0 (or an inner too small
// to hold even one pixel-high row) returns no rects.
//
// The tiling itself is the table module's row stack (widgets.StackRects
// at the board's gap — the layout newBoardSection installs); this
// wrapper is the pure pin the geometry tests assert against, in the
// board's own units. The station board sections size themselves to their
// rows, so the runtime rects come out at exactly the natural row height;
// the fill semantics keep the invariants for any inner rect the tests
// throw at it (design sizes, window minimum, degenerate n).
func boardRowRects(inner image.Rectangle, n int) []image.Rectangle {
	return widgets.StackRects(inner, n, theme.Px(boardRowGap))
}

// capRows splits n rows into the visible count and the hidden remainder:
// at most cap rows render, and when truncated the last visible slot
// becomes a "+N more" note (so a section's height never depends on n
// exceeding the cap).
func capRows(n, cap int) (show, hidden int) {
	if n <= cap {
		return n, 0
	}
	return cap - 1, n - (cap - 1)
}

// stationRow is one rendered board row's action handle — its contract id
// plus the two things widgets.StackRow rendered (the root the section
// tiles, the trailing action button) — so tests can find, read and press
// it. The row itself is a rich row built by the table module; only this
// handle over it lives here.
type stationRow struct {
	id   content.ContractID
	btn  *widget.Button
	root *widget.Container
}

// boardRowModel is one contract-board row in pure, comparable form — the
// widget tree is rebuilt from the model whenever its signature changes,
// which makes the refresh rule simple: any state the rows display must
// appear in the model.
type boardRowModel struct {
	ID       content.ContractID
	Title    string // line 1 (active rows: "Belt cleanup — 1/3")
	Summary  string // line 2, available rows only
	Reward   string // line 3, available rows only
	Require  string // line 3 right, gated rows only
	Icon     string // uikit icon name for the objective kind ("" = none)
	Action   string // button label; "" for note rows
	Disabled bool
	Note     bool // "+N more" / empty-state row (no button)
}

// stationBoardModel is the whole board: the docked station's offerings
// plus every accepted contract. Comparable via sig (fmt of the struct).
type stationBoardModel struct {
	Station  content.CelID
	Standing float64
	Avail    []boardRowModel
	Active   []boardRowModel
}

// same is the change detector for syncStationBoard: field-wise equality
// against the last rebuilt model (boardRowModel is comparable, so the
// rows compare through slices.Equal). It replaces a fmt.Sprintf("%v", m)
// signature that ran every docked frame just to detect no change.
func (m stationBoardModel) same(o stationBoardModel) bool {
	return m.Station == o.Station && m.Standing == o.Standing &&
		slices.Equal(m.Avail, o.Avail) && slices.Equal(m.Active, o.Active)
}

func noteRow(text string) boardRowModel {
	return boardRowModel{Title: text, Note: true}
}

// buildStationBoard derives the board model from the simulation: the
// docked station's Content.Board() offerings (gated by standing, disabled
// once accepted) and the active contracts with their live progress. It
// never mutates the sim.
func (g *Game) buildStationBoard() stationBoardModel {
	var m stationBoardModel
	at, ok := g.s.Docked.Get(g.s.Player())
	if !ok {
		return m // undocked: empty board (the window is unmounted anyway)
	}
	c := g.opts.Content
	m.Station = at.At
	m.Standing = g.s.Standing()

	accepted := map[content.ContractID]bool{}
	for _, ms := range g.s.Missions() {
		accepted[ms.ID] = true
	}

	board := c.Board(at.At)
	show, hidden := capRows(len(board), availMaxRows)
	for i := 0; i < show; i++ {
		def := board[i]
		r := boardRowModel{
			ID:      def.ID,
			Title:   def.Title,
			Summary: objectiveSummary(c, def),
			Reward:  rewardText(def),
			Icon:    theme.ContractIcon(def.Kind),
			Action:  "Accept",
		}
		if accepted[def.ID] {
			r.Action, r.Disabled = "Accepted", true
		} else if m.Standing+1e-9 < def.MinStanding { // same epsilon as sim's gate
			r.Require = fmt.Sprintf("Requires standing %.2f", def.MinStanding)
			r.Disabled = true
		}
		m.Avail = append(m.Avail, r)
	}
	switch {
	case hidden > 0:
		m.Avail = append(m.Avail, noteRow(fmt.Sprintf("+%d more", hidden)))
	case len(board) == 0:
		m.Avail = append(m.Avail, noteRow("No contracts on this board."))
	}

	type activeContract struct {
		def *content.ContractDef
		ms  sim.MissionState
	}
	var active []activeContract
	for _, ms := range g.s.Missions() {
		if def, ok := c.Contract(ms.ID); ok { // stale save refs are skipped, not rendered
			active = append(active, activeContract{def, ms})
		}
	}
	show, hidden = capRows(len(active), actMaxRows)
	for i := 0; i < show; i++ {
		a := active[i]
		have := a.ms.Progress
		if a.def.Kind == content.ContractDeliver {
			have = g.cargoProgress(a.def) // the sim counts deliver objectives only at the claim
		}
		m.Active = append(m.Active, boardRowModel{
			ID:     a.ms.ID,
			Title:  a.def.Title + " — " + fmt.Sprintf("%d/%d", int(math.Round(have)), a.def.Objective.Count),
			Icon:   theme.ContractIcon(a.def.Kind),
			Action: "Abandon",
		})
	}
	switch {
	case hidden > 0:
		m.Active = append(m.Active, noteRow(fmt.Sprintf("+%d more", hidden)))
	case len(active) == 0:
		m.Active = append(m.Active, noteRow("No active contracts."))
	}
	return m
}

// syncStationBoard rebuilds the services view's contract board when its
// model signature changes and resizes the window to the content. It is
// called on drained EvMission/EvDocked/EvUndocked events (the same path
// the log drains) and every frame while docked from syncMounts — the
// frame-level call also catches state that emits no mission event (cargo
// bought at the market feeds the deliver progress line).
func (g *Game) syncStationBoard() {
	if g.svcWin == nil || g.stAvail == nil || g.stBody == nil {
		return
	}
	m := g.buildStationBoard()
	if !m.same(g.stModel) {
		g.stModel = m
		g.rebuildStationBoard(m)
	}
}

// rebuildStationBoard replaces both sections' row widgets from the model
// and hugs the window to the new content height (the row caps in the
// model bound it well above the log at the minimum screen size).
func (g *Game) rebuildStationBoard(m stationBoardModel) {
	g.stAvail.RemoveChildren()
	g.stActive.RemoveChildren()
	g.stAvailRows, g.stActRows = nil, nil
	for _, r := range m.Avail {
		root, btn := g.buildContractRow(r, false)
		g.stAvail.AddChild(root)
		if btn != nil {
			g.stAvailRows = append(g.stAvailRows, stationRow{id: r.ID, btn: btn, root: root})
		}
	}
	for _, r := range m.Active {
		root, btn := g.buildContractRow(r, true)
		g.stActive.AddChild(root)
		if btn != nil {
			g.stActRows = append(g.stActRows, stationRow{id: r.ID, btn: btn, root: root})
		}
	}
	_, h := g.stBody.PreferredSize()
	// The services column is part of the composed docked layout — same
	// source of truth as placeDefaults (stationLayout), so the board's
	// content hug and the viewport composition can never disagree — and
	// it goes through the placement tracker like every other composed
	// rect: a direct SetLocation here would leave g.placed stale and the
	// window would read as player-moved (never recomposed again).
	g.placeIfVirgin(g.svcWin, stationLayout(g.w, g.h, h).Station)
}

// buildContractRow renders one board row as a rich row of the table
// module (widgets.NewStackRow): the line stack below, the kind icon
// leading line 1 and the row's own RowSpec.Actions drawn as its trailing
// button — the declaration carries the same action, gating and command
// the context menu runs, so button and menu can never disagree on the
// verb. The row's context menu rides the RowSpec seam: a right-click on
// the objective label (wired by the builder, as that stack line's Ref)
// replaces this menu — handlers run parent-first, so the child's
// OpenMenu lands last and wins. Returns a nil button for note rows.
func (g *Game) buildContractRow(r boardRowModel, active bool) (*widget.Container, *widget.Button) {
	p := g.env.Pal()
	row := widgets.TableRow{}
	if r.Icon != "" {
		row.Icons = []string{r.Icon}
	}
	// The menu's declaration (the seam's verbs the RowSpec test pins:
	// Accept / Abandon, grayed at the gate) and the button's copy of it:
	// the button wears the model's own label on top, "Accepted" marking a
	// taken contract — the board marker GAMEPLAY documents and no menu
	// entry reads. Note rows declare nothing: no button, no menu.
	menuSpec := widgets.RowSpec{}
	if !r.Note {
		menuSpec = g.contractRowSpec(r, active)
		row.Spec = menuSpec
		if r.Action != "" && r.Action != menuSpec.Actions[0].Label {
			row.Spec.Actions = append([]widgets.RowAction(nil), menuSpec.Actions...)
			row.Spec.Actions[0].Label = r.Action
		}
		row.Stack = g.boardRowStack(r, active)
	} else {
		// "+N more" / empty state: one dim line, no verbs, no menu.
		row.Stack = []widgets.RowLine{{Segs: []widgets.RowSeg{{
			Text: g.ellip(r.Title, theme.StepTiny, float64(theme.Px(boardContentW))),
			Tint: p.Dim,
			Step: theme.StepTiny,
		}}}}
	}
	sr := widgets.NewStackRow(g.env, row, g.env.Face(theme.StepSmall))
	// The board's own row strip: the bordered translucent band every
	// board row has worn (the table paints its row tints; this chrome is
	// the board's). The builder already bound the session's widget theme
	// to the row, so setting the strip may validate buttons and all
	// before the row joins the window.
	sr.Root.SetBackgroundImage(bordered(nrgba(p.Cell, 160), nrgba(p.Border, 140), 1))
	sr.Root.GetWidget().MouseButtonClickedEvent.AddHandler(func(args any) {
		a, ok := args.(*widget.WidgetMouseButtonClickedEventArgs)
		if !ok || a.Button != ebiten.MouseButtonRight {
			return
		}
		g.env.OpenRowMenu(menuSpec, widgets.ClickPoint(a))
	})
	if len(sr.Buttons) == 0 {
		return sr.Root, nil
	}
	return sr.Root, sr.Buttons[0]
}

// boardRowStack is one contract row's lines: the title over the objective
// summary, the gated standing requirement and the reward on an available
// row — the reward owns line 3 alone, so the payout is the one string on
// a board row that must never ellipsize — and just the title + progress
// line on an active one. The title keeps its fixed width (the ellipsis
// budget the row reserves, icon box included), so every row measures the
// same with or without a glyph.
func (g *Game) boardRowStack(r boardRowModel, active bool) []widgets.RowLine {
	p := g.env.Pal()
	// The active line is title + progress ("Belt cleanup — 1/3") and only
	// fits the title column at the tiny step in IBM Plex Sans (at small
	// the em-dash suffix alone costs 56 px); available titles, which
	// carry no suffix, stay at small. The station column narrowed with
	// the rail, so the longest shipped title's progress ellipsizes.
	step := theme.StepSmall
	if active {
		step = theme.StepTiny
	}
	titleW := theme.Px(boardTextW)
	if r.Icon != "" {
		titleW -= theme.Px(theme.IconInline) + theme.Px(theme.IconGap)
	}
	lines := []widgets.RowLine{{Segs: []widgets.RowSeg{{
		Text: g.ellip(r.Title, step, float64(titleW)),
		Tint: p.Accent,
		Step: step,
		MinW: titleW,
	}}}}
	if active {
		return lines // one line: "Belt cleanup — 1/3" + Abandon
	}

	// A gated row parks its standing requirement beside the objective
	// (line 2) so line 3 gives the payout the whole row.
	summaryW := float64(theme.Px(boardContentW))
	if r.Require != "" {
		reqW := g.textW(r.Require, theme.StepTiny)
		if capW := summaryW - float64(theme.Px(boardBtnGap)) - float64(theme.Px(40)); reqW > capW {
			r.Require = g.ellip(r.Require, theme.StepTiny, capW)
			reqW = g.textW(r.Require, theme.StepTiny)
		}
		summaryW -= reqW + float64(theme.Px(boardBtnGap))
	}
	summary := widgets.RowLine{Segs: []widgets.RowSeg{{
		Text: g.ellip(r.Summary, theme.StepTiny, summaryW),
		Tint: p.Dim,
		Step: theme.StepTiny,
	}}}
	if r.Require != "" {
		summary.Segs = append(summary.Segs, widgets.RowSeg{
			Text: r.Require, Tint: p.Warn, Step: theme.StepTiny,
		})
	}
	// The objective label is its own gesture target: its Ref declares
	// Show Info on the objective's def (the caste it asks you to kill,
	// the item it wants), so right-click opens that menu and double-click
	// swaps the window — the installer's halves, strictly. It sits INSIDE
	// the row and child handlers run after the parent's, so this Show
	// Info menu replaces the row's Accept menu when the click lands here.
	if ref, showable := g.objectiveRef(r.ID); showable {
		summary.Ref = &ref
	}
	lines = append(lines, summary, widgets.RowLine{Segs: []widgets.RowSeg{{
		Text: g.ellip(r.Reward, theme.StepTiny, float64(theme.Px(boardContentW))),
		Tint: p.Good,
		Step: theme.StepTiny,
		MinW: theme.Px(boardContentW),
	}}})
	return lines
}

// contractRowSpec is one board row's declared interactions: the same
// action the button carries (Accept on an available row, Abandon on an
// active one), the same gating, through the same command — a contract
// has no def of its own, so the row declares no Ref (its menu is the
// single action entry, exactly as before).
func (g *Game) contractRowSpec(r boardRowModel, active bool) widgets.RowSpec {
	label, intent := "Accept", widgets.Intent(widgets.Accept{Contract: r.ID})
	if active {
		label, intent = "Abandon", widgets.Abandon{Contract: r.ID}
	}
	return widgets.RowSpec{Actions: []widgets.RowAction{{
		Label:    label,
		Disabled: r.Disabled,
		Intent:   intent,
	}}}
}

// objectiveRef resolves a contract's objective to the definition its
// Show Info window reads: a kill contract links its caste, deliver/mine
// contracts their item. The ref is validated against the loaded content
// (a stale save ref shows nothing rather than a hollow window).
func (g *Game) objectiveRef(id content.ContractID) (widgets.InfoRef, bool) {
	def, ok := g.opts.Content.Contract(id)
	if !ok {
		return widgets.InfoRef{}, false
	}
	var ref widgets.InfoRef
	switch {
	case def.Objective.Target != "":
		ref = widgets.NpcRef(def.Objective.Target)
	case def.Objective.Item != "":
		ref = widgets.ItemRef(def.Objective.Item)
	default:
		return widgets.InfoRef{}, false
	}
	return g.env.InfoRefOf(ref)
}

// objectiveSummary renders the contract's kind + objective, e.g.
// "Destroy 1× Scuttler", "Deliver 2 m³ EMP Charge to Przystań",
// "Mine 400 m³ Cordite Ore".
func objectiveSummary(c *content.Content, def *content.ContractDef) string {
	switch def.Kind {
	case content.ContractKill:
		name := string(def.Objective.Target)
		if npc, ok := c.Npc(def.Objective.Target); ok {
			name = npc.Name
		}
		return fmt.Sprintf("Destroy %d× %s", def.Objective.Count, name)
	case content.ContractDeliver:
		return fmt.Sprintf("Deliver %d m³ %s to %s",
			def.Objective.Count, itemName(c, def.Objective.Item),
			celName(c, def.Objective.Destination))
	case content.ContractMine:
		return fmt.Sprintf("Mine %d m³ %s",
			def.Objective.Count, itemName(c, def.Objective.Item))
	}
	return string(def.Kind) // validator-unknown kind: show the raw kind
}

// rewardText renders the board's reward line, e.g.
// "40,000.00 € · +0.10 standing" — the shared money presentation
// (theme.FormatCredits), the one every readout uses.
func rewardText(def *content.ContractDef) string {
	s := theme.FormatCredits(def.Reward.Credits)
	if def.Reward.StandingGain != 0 {
		s += fmt.Sprintf(" · %+.2f standing", def.Reward.StandingGain)
	}
	return s
}

// cargoProgress counts a deliver contract's objective cargo in the hold,
// clamped to the objective: the sim only counts deliver objectives at the
// claim, so the board shows what is actually aboard.
func (g *Game) cargoProgress(def *content.ContractDef) float64 {
	cargo, ok := g.s.Cargos.Get(g.s.Player())
	if !ok {
		return 0
	}
	qty := 0
	for _, st := range cargo.Stacks {
		if st.Item == def.Objective.Item {
			qty += st.Qty
		}
	}
	for _, st := range cargo.Ore {
		if st.Item == def.Objective.Item {
			qty += st.Qty
		}
	}
	item, ok := g.opts.Content.Item(def.Objective.Item)
	if !ok || item.Volume <= 0 {
		return 0
	}
	have := float64(qty) * item.Volume
	if want := float64(def.Objective.Count); have > want {
		have = want
	}
	return have
}

// itemName resolves an item id to its display name.
func itemName(c *content.Content, id content.ItemID) string {
	if def, ok := c.Item(id); ok {
		return def.Name
	}
	return string(id)
}

// celName resolves a celestial id to its display name across systems (a
// deliver destination may sit in another system). Systems are scanned in
// sorted id order so the result never depends on map order.
func celName(c *content.Content, id content.CelID) string {
	systems := make([]content.SystemID, 0, len(c.Systems))
	for sid := range c.Systems {
		systems = append(systems, sid)
	}
	sort.Slice(systems, func(i, j int) bool { return systems[i] < systems[j] })
	for _, sid := range systems {
		for _, cel := range c.Systems[sid].Celestials {
			if cel.ID == id {
				return cel.Name
			}
		}
	}
	return string(id)
}

// ellip truncates s with "…" so it measures at most w in the given
// type-scale step (single line: board rows never wrap).
func (g *Game) ellip(s, step string, w float64) string {
	face := g.env.Face(step)
	return widgets.Ellipsize(s, w, func(x string) float64 {
		tw, _ := text.Measure(x, *face, 0)
		return tw
	})
}

// textW measures s in the given type-scale step (row-column sizing).
func (g *Game) textW(s, step string) float64 {
	face := g.env.Face(step)
	w, _ := text.Measure(s, *face, 0)
	return w
}
