package screens

import (
	"fmt"
	"image"
	"math"
	"slices"
	"sort"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// The station board — the contract column under the service row — is a
// Screen panel of its own (CONTEXT.md "Station board"): the window, the
// service row and the two contract sections, rebuilt from one comparable
// model and fitted to its content.
//
// It is the ONE window docking opens. The host asks it for its content
// floor (FitSize, DESIGN units) and mirrors that onto the window's
// MinSize and the declared column through the placement tracker — the
// panel never places or resizes itself, so a player-moved window keeps its
// position and a grip drag stops at the board's own height.
//
// Geometry (docs/GAMEPLAY.md, "Contracts & standing"): the station
// services window owns the left column while docked: x ∈ [StationX,
// StationX+StationWinW) starts clear of the navigation rail's gutter and
// keeps its right edge at 404. The content-driven height (row caps
// included) keeps the bottom above the log, whose top sits at
// screen-height-250 — i.e. y=450 at the 1024×700 minimum. Both are pinned
// by tests at 1280×720, 1024×700 and at the window minimum.
const (
	StationX     = 10 + RailInset // window left edge: rail gutter first
	StationY     = 8
	StationWinW  = 394 - RailInset // window width; right edge still 404
	StationMinW  = 394 - RailInset // window MinSize — the "minimum window" pin
	StationMinH  = 200
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

	boardInnerW   = StationWinW - 2*boardPad                // body inner width: 382
	boardContentW = boardInnerW - 2*boardRowPad             // row content width: 376
	boardTextW    = boardContentW - boardBtnW - boardBtnGap // text column width: 278
)

// StationRect is the station board's window rect in live px: the left
// column, clear of the navigation rail's gutter, at the board's content
// height (never below StationMinH). The game's Screen row declares the
// same column (its DefaultRect) and the host's FitSize path composes it
// from this height, so the declared rect and the fitted one can never
// disagree.
func StationRect(stationH int) image.Rectangle {
	return image.Rect(theme.Px(StationX), theme.Px(StationY),
		theme.Px(StationX+StationWinW), theme.Px(StationY)+max(stationH, theme.Px(StationMinH)))
}

// StationBoard is the docked station's contract board panel: the service
// row (identity / Undock / Cargo / this station's services), the available
// contracts and the active ones. Sync re-reads the board every mounted
// frame and rebuilds the rows only when the model moves.
type StationBoard struct {
	env widgets.Seams
	cv  widgets.ContentView

	win    *widget.Window
	svc    *Services
	body   *widget.Container
	avail  *widget.Container
	active *widget.Container

	model     stationBoardModel
	availRows []BoardRow
	actRows   []BoardRow

	// fit is the content floor in DESIGN units, cached at the last
	// rebuild (FitSize's answer — never a per-frame PreferredSize).
	fit int
}

// NewStationBoard builds the board over the given seams. The window is
// constructed empty at the declared column with the static floor; the host
// mounts it, runs Sync and then applies FitSize.
func NewStationBoard(env widgets.Seams) *StationBoard {
	p := env.Pal()
	b := &StationBoard{env: env, cv: env, fit: StationMinH}
	b.svc = NewServices(env, boardInnerW)
	b.avail = newBoardSection()
	b.active = newBoardSection()

	availHead := widget.NewText(
		widget.TextOpts.Text("Available contracts", env.Face(theme.StepSmall), theme.Solid(p.Accent)),
	)
	actHead := widget.NewText(
		widget.TextOpts.Text("Active contracts", env.Face(theme.StepSmall), theme.Solid(p.Accent)),
	)

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.SquareBox(theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(boardSpacing)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(boardPad))),
		)),
	)
	body.AddChild(b.svc.Container(), availHead, b.avail, actHead, b.active)

	b.body = body
	b.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.MinSize(theme.Px(StationMinW), theme.Px(StationMinH)),
		widget.WindowOpts.Location(image.Rect(
			theme.Px(StationX), theme.Px(StationY),
			theme.Px(StationX+StationWinW), theme.Px(StationY+StationMinH))),
	)
	return b
}

// Win returns the panel's window for the host's mount and the layout
// sidecar.
func (b *StationBoard) Win() *widget.Window { return b.win }

// Services returns the service row above the board (the panel's identity
// line and per-service buttons).
func (b *StationBoard) Services() *Services { return b.svc }

// FitSize is the board's content floor in DESIGN units: the column's fixed
// width and the height the last rebuild measured (never below StationMinH).
// The host mirrors it onto the window's MinSize and composes the declared
// column from it.
func (b *StationBoard) FitSize() (w, h int) { return StationWinW, b.fit }

// Sync refreshes the service row every frame and rebuilds the board when
// its model moves — the one refresh path (the host syncs mounted panels).
func (b *StationBoard) Sync() {
	if b.cv == nil {
		return
	}
	b.svc.Sync()
	m := b.build()
	if !m.same(b.model) {
		b.model = m
		b.rebuild(m)
	}
}

// newBoardSection builds one contract-row section: a bare container whose
// rows are tiled by the table module's row stack — equal heights,
// boardRowGap apart, the pure pin of which is boardRowRects.
func newBoardSection() *widget.Container {
	return widget.NewContainer(
		widget.ContainerOpts.Layout(widgets.StackLayout{Gap: theme.Px(boardRowGap)}),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
}

// boardRowRects tiles up to n contract rows inside inner: equal heights,
// equal boardRowGap gaps, first row flush with inner's top — every rect
// pairwise disjoint and fully inside inner. n <= 0 (or an inner too small
// to hold even one pixel-high row) returns no rects.
//
// The tiling itself is the table module's row stack (widgets.StackRects at
// the board's gap — the layout newBoardSection installs); this wrapper is
// the pure pin the geometry tests assert against, in the board's own
// units. The sections size themselves to their rows, so the runtime rects
// come out at exactly the natural row height; the fill semantics keep the
// invariants for any inner rect the tests throw at it.
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

// BoardRow is one rendered board row's handle — its contract id plus the
// two things widgets.StackRow rendered (the root the section tiles, the
// trailing action button) — so the host's tests and captures can find,
// read and press it. The row itself is a rich row built by the table
// module; only this handle over it lives here. Note rows ("+N more", the
// empty states) carry a nil button.
type BoardRow struct {
	ID   content.ContractID
	Btn  *widget.Button
	Root *widget.Container
}

// Rows returns the rows rendered into one section, in order: available
// (active false) or accepted contracts. It is the handle tests and
// captures use; the panel's model stays unexported.
func (b *StationBoard) Rows(active bool) []BoardRow {
	if active {
		return b.actRows
	}
	return b.availRows
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
// plus every accepted contract.
type stationBoardModel struct {
	Station  content.CelID
	Standing float64
	Avail    []boardRowModel
	Active   []boardRowModel
}

// same is the change detector for Sync: field-wise equality against the
// last rebuilt model (boardRowModel is comparable, so the rows compare
// through slices.Equal).
func (m stationBoardModel) same(o stationBoardModel) bool {
	return m.Station == o.Station && m.Standing == o.Standing &&
		slices.Equal(m.Avail, o.Avail) && slices.Equal(m.Active, o.Active)
}

func noteRow(text string) boardRowModel {
	return boardRowModel{Title: text, Note: true}
}

// build derives the board model from the ContentView: the docked
// station's Board offerings (gated by standing, disabled once accepted)
// and the active contracts with their live progress. It never mutates
// anything.
func (b *StationBoard) build() stationBoardModel {
	var m stationBoardModel
	at := b.cv.Contracts()
	if at.Station == "" {
		return m // undocked: empty board (the window is unmounted anyway)
	}
	c := b.cv.Defs()
	m.Station = at.Station
	m.Standing = at.Standing

	accepted := map[content.ContractID]bool{}
	for _, ms := range at.Active {
		accepted[ms.ID] = true
	}

	board := at.Board
	show, hidden := capRows(len(board), availMaxRows)
	for i := 0; i < show; i++ {
		def := board[i]
		r := boardRowModel{
			ID:      def.ID,
			Title:   def.Title,
			Summary: objectiveSummary(b.cv, def),
			Reward:  rewardText(def),
			Icon:    theme.ContractIcon(def.Kind),
			Action:  "Accept",
		}
		if accepted[def.ID] {
			r.Action, r.Disabled = "Accepted", true
		} else if !b.cv.MeetsStanding(def.MinStanding) { // the sim's accept gate, epsilon included
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
	for _, ms := range at.Active {
		if def, ok := c.Contract(ms.ID); ok { // stale save refs are skipped, not rendered
			active = append(active, activeContract{def, ms})
		}
	}
	show, hidden = capRows(len(active), actMaxRows)
	for i := 0; i < show; i++ {
		a := active[i]
		have := a.ms.Progress
		if a.def.Kind == content.ContractDeliver {
			have = b.cv.DeliverProgress(a.def) // the sim counts deliver objectives only at the claim
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

// rebuild replaces both sections' row widgets from the model and caches
// the body's content height (the row caps in the model bound it well above
// the log at the minimum screen size).
func (b *StationBoard) rebuild(m stationBoardModel) {
	b.avail.RemoveChildren()
	b.active.RemoveChildren()
	b.availRows, b.actRows = nil, nil
	for _, r := range m.Avail {
		root, btn := b.buildRow(r, false)
		b.avail.AddChild(root)
		b.availRows = append(b.availRows, BoardRow{ID: r.ID, Btn: btn, Root: root})
	}
	for _, r := range m.Active {
		root, btn := b.buildRow(r, true)
		b.active.AddChild(root)
		b.actRows = append(b.actRows, BoardRow{ID: r.ID, Btn: btn, Root: root})
	}
	_, h := b.body.PreferredSize()
	// The body's preferred height, in the DESIGN units the host fits
	// with: the floor the grip stops at and FitSize caches.
	b.fit = max(int(math.Round(float64(h)/theme.Scale())), StationMinH)
}

// buildRow renders one board row as a rich row of the table module
// (widgets.NewStackRow): the line stack below, the kind icon leading line
// 1 and the row's own RowSpec.Actions drawn as its trailing button — the
// declaration carries the same action, gating and command the context
// menu runs, so button and menu can never disagree on the verb. The row's
// context menu rides the RowSpec seam: a right-click on the objective
// label (wired by the builder, as that stack line's Ref) replaces this
// menu — handlers run parent-first, so the child's OpenMenu lands last and
// wins. Returns a nil button for note rows.
func (b *StationBoard) buildRow(r boardRowModel, active bool) (*widget.Container, *widget.Button) {
	p := b.env.Pal()
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
		menuSpec = b.contractRowSpec(r, active)
		row.Spec = menuSpec
		if r.Action != "" && r.Action != menuSpec.Actions[0].Label {
			row.Spec.Actions = append([]widgets.RowAction(nil), menuSpec.Actions...)
			row.Spec.Actions[0].Label = r.Action
		}
		row.Stack = b.rowStack(r, active)
	} else {
		// "+N more" / empty state: one dim line, no verbs, no menu.
		row.Stack = []widgets.RowLine{{Segs: []widgets.RowSeg{{
			Text: b.ellip(r.Title, theme.StepTiny, float64(theme.Px(boardContentW))),
			Tint: p.Dim,
			Step: theme.StepTiny,
		}}}}
	}
	sr := widgets.NewStackRow(b.env, row, b.env.Face(theme.StepSmall))
	// The board's own row strip: the bordered translucent band every
	// board row has worn (the table paints its row tints; this chrome is
	// the board's). The builder already bound the session's widget theme
	// to the row, so setting the strip may validate buttons and all
	// before the row joins the window.
	sr.Root.SetBackgroundImage(theme.SquareBox(theme.NRGBA(p.Cell, 160), theme.NRGBA(p.Border, 140), 1))
	sr.Root.GetWidget().MouseButtonClickedEvent.AddHandler(func(args any) {
		a, ok := args.(*widget.WidgetMouseButtonClickedEventArgs)
		if !ok || a.Button != ebiten.MouseButtonRight {
			return
		}
		widgets.OpenRowMenu(b.cv, b.env, menuSpec, widgets.ClickPoint(a))
	})
	if len(sr.Buttons) == 0 {
		return sr.Root, nil
	}
	return sr.Root, sr.Buttons[0]
}

// rowStack is one contract row's lines: the title over the objective
// summary, the gated standing requirement and the reward on an available
// row — the reward owns line 3 alone, so the payout is the one string on
// a board row that must never ellipsize — and just the title + progress
// line on an active one. The title keeps its fixed width (the ellipsis
// budget the row reserves, icon box included), so every row measures the
// same with or without a glyph.
func (b *StationBoard) rowStack(r boardRowModel, active bool) []widgets.RowLine {
	p := b.env.Pal()
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
		Text: b.ellip(r.Title, step, float64(titleW)),
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
		reqW := b.textW(r.Require, theme.StepTiny)
		if capW := summaryW - float64(theme.Px(boardBtnGap)) - float64(theme.Px(40)); reqW > capW {
			r.Require = b.ellip(r.Require, theme.StepTiny, capW)
			reqW = b.textW(r.Require, theme.StepTiny)
		}
		summaryW -= reqW + float64(theme.Px(boardBtnGap))
	}
	summary := widgets.RowLine{Segs: []widgets.RowSeg{{
		Text: b.ellip(r.Summary, theme.StepTiny, summaryW),
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
	if ref, showable := b.objectiveRef(r.ID); showable {
		summary.Ref = &ref
	}
	lines = append(lines, summary, widgets.RowLine{Segs: []widgets.RowSeg{{
		Text: b.ellip(r.Reward, theme.StepTiny, float64(theme.Px(boardContentW))),
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
// single action entry).
func (b *StationBoard) contractRowSpec(r boardRowModel, active bool) widgets.RowSpec {
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
func (b *StationBoard) objectiveRef(id content.ContractID) (widgets.InfoRef, bool) {
	def, ok := b.cv.Defs().Contract(id)
	if !ok {
		return widgets.InfoRef{}, false
	}
	var ref widgets.InfoRef
	switch {
	case def.Objective.Target != "":
		ref = widgets.NpcRef(def.Objective.Target)
	case def.Objective.Item != "":
		ref, _ = widgets.ResolveTradeable(b.cv.Defs(), def.Objective.Item).Ref()
	default:
		return widgets.InfoRef{}, false
	}
	return b.cv.InfoRefOf(ref)
}

// objectiveSummary renders the contract's kind + objective, e.g.
// "Destroy 1× Scuttler", "Deliver 2 m³ 40mm ion to Hauvnok",
// "Mine 400 m³ Kerngrit Ore".
func objectiveSummary(cv widgets.ContentView, def *content.ContractDef) string {
	c := cv.Defs()
	switch def.Kind {
	case content.ContractKill:
		name := string(def.Objective.Target)
		if npc, ok := c.Npc(def.Objective.Target); ok {
			name = npc.Name
		}
		return fmt.Sprintf("Destroy %d× %s", def.Objective.Count, name)
	case content.ContractDeliver:
		// Modules trade as cargo units (no volume); items keep the m³ line.
		if _, ok := c.Module(content.ModuleID(def.Objective.Item)); ok {
			return fmt.Sprintf("Deliver %d× %s to %s",
				def.Objective.Count, cv.TradeableName(def.Objective.Item),
				celName(c, def.Objective.Destination))
		}
		return fmt.Sprintf("Deliver %d m³ %s to %s",
			def.Objective.Count, cv.TradeableName(def.Objective.Item),
			celName(c, def.Objective.Destination))
	case content.ContractMine:
		return fmt.Sprintf("Mine %d m³ %s",
			def.Objective.Count, cv.TradeableName(def.Objective.Item))
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
func (b *StationBoard) ellip(s, step string, w float64) string {
	face := b.env.Face(step)
	return widgets.Ellipsize(s, w, func(x string) float64 {
		tw, _ := text.Measure(x, *face, 0)
		return tw
	})
}

// textW measures s in the given type-scale step (row-column sizing).
func (b *StationBoard) textW(s, step string) float64 {
	face := b.env.Face(step)
	w, _ := text.Measure(s, *face, 0)
	return w
}
