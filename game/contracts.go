package game

import (
	"fmt"
	"image"
	"math"
	"sort"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// Station contract-board geometry (docs/GAMEPLAY.md, "Contracts &
// standing"). The station window owns the top-left column while docked:
// x ∈ [stationX, stationX+stationWinW) keeps a gutter before the market at
// x=396, and the content-driven height (row caps included) keeps the
// bottom above the log, whose top sits at screen-height-250 — i.e. y=450
// at the 1024×700 minimum. Both are pinned by tests at1280×720, 1024×700
// and at the window minimum.
const (
	stationX     = 10 // window left/top edge (matches placeDefaults)
	stationY     = 8
	stationWinW  = 394 // window width; right edge 404, market starts at x=406
	stationMinW  = 394 // window MinSize — the "minimum window" row-rect pin
	stationMinH  = 200
	boardPad     = 6  // body padding
	boardSpacing = 5  // vertical gap between body children
	boardRowGap  = 4  // gap between two board rows
	boardRowPad  = 3  // horizontal padding inside a row
	boardBtnGap  = 6  // text ↔ action button gap inside a row
	boardBtnW    = 92 // action button width (fits the widest label, "Accepted", set in the emphasis face)
	boardBtnH    = 40 // minimum hit area, like menuBtnH
	availMaxRows = 3  // visible available rows ("+N more" beyond)
	actMaxRows   = 2  // visible active rows ("+N more" beyond)

	boardInnerW   = stationWinW - 2*boardPad                // body inner width: 382
	boardContentW = boardInnerW - 2*boardRowPad             // row content width: 376
	boardTextW    = boardContentW - boardBtnW - boardBtnGap // text column width: 278
)

// boardRowRects tiles up to n contract rows inside inner: equal heights,
// equal boardRowGap gaps, first row flush with inner's top — every rect
// pairwise disjoint and fully inside inner. n <= 0 (or an inner too small
// to hold even one pixel-high row) returns no rects.
//
// The station board sections size themselves to their rows, so the
// runtime rects come out at exactly the natural row height; the fill
// semantics keep the invariants for any inner rect the tests throw at it
// (design sizes, window minimum, degenerate n).
func boardRowRects(inner image.Rectangle, n int) []image.Rectangle {
	if n <= 0 || inner.Dx() <= 0 || inner.Dy() <= 0 {
		return nil
	}
	gap := boardRowGap
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

// boardLayout is the ebitenui Layouter behind the two board sections: it
// reports the section height from its rows and places them with
// boardRowRects — the pure function above IS the layout.
type boardLayout struct{}

// PreferredSize reports the widest row and the stacked section height.
// The width matters: a zero-width preferred rect is "empty" in Go's
// image.Rectangle, and the parent RowLayout's preferred-size union would
// then drop the section's height entirely (the window would come out
// shorter than its rows).
func (boardLayout) PreferredSize(ws []widget.PreferredSizeLocateableWidget) (int, int) {
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
	return w, len(ws)*h + (len(ws)-1)*boardRowGap
}

func (boardLayout) Layout(ws []widget.PreferredSizeLocateableWidget, rect image.Rectangle) {
	for i, r := range boardRowRects(rect, len(ws)) {
		ws[i].SetLocation(r)
	}
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

// stationRow is one rendered board row's action handle (its contract id,
// button and root container) so tests can find, read and press it.
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

// sig is the change detector for syncStationBoard.
func (m stationBoardModel) sig() string { return fmt.Sprintf("%v", m) }

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
			Icon:    uikit.ContractIcon(def.Kind),
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
			Icon:   uikit.ContractIcon(a.def.Kind),
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

// syncStationBoard rebuilds the station window's contract board when its
// model signature changes and resizes the window to the content. It is
// called on drained EvMission/EvDocked/EvUndocked events (the same path
// the log drains) and every frame while docked from syncMounts — the
// frame-level call also catches state that emits no mission event (cargo
// bought at the market feeds the deliver progress line).
func (g *Game) syncStationBoard() {
	if g.stationW == nil || g.stAvail == nil || g.stBody == nil {
		return
	}
	m := g.buildStationBoard()
	if sig := m.sig(); sig != g.stSig {
		g.stSig = sig
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
	// The station column is part of the composed docked layout — same
	// source of truth as placeDefaults (stationLayout), so the board's
	// content hug and the viewport composition can never disagree — and
	// it goes through the placement tracker like every other composed
	// rect: a direct SetLocation here would leave g.placed stale and the
	// window would read as player-moved (never recomposed again).
	g.placeIfVirgin(g.stationW, stationLayout(g.w, g.h, h).Station)
}

// buildContractRow renders one board row: text lines left, action button
// right. Note rows render their text alone, vertically centred. Returns a
// nil button for note rows.
func (g *Game) buildContractRow(r boardRowModel, active bool) (*widget.Container, *widget.Button) {
	p := g.env.Pal
	rowBG := bordered(nrgba(p.Cell, 160), nrgba(p.Border, 140), 1)
	rowPad := &widget.Insets{Left: boardRowPad, Right: boardRowPad}

	if r.Note {
		row := widget.NewContainer(
			widget.ContainerOpts.BackgroundImage(rowBG),
			widget.ContainerOpts.Layout(widget.NewRowLayout(
				widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
				widget.RowLayoutOpts.Padding(rowPad),
			)),
		)
		row.AddChild(widget.NewText(
			widget.TextOpts.Text(g.ellip(r.Title, uikit.StepTiny, boardContentW), g.env.Face(uikit.StepTiny), solid(p.Dim)),
			widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Position: widget.RowLayoutPositionCenter,
			})),
		))
		return row, nil
	}

	btn := g.newBtn(r.Action, boardBtnW, boardBtnH, func() {
		if r.Disabled { // gated/accepted rows enqueue nothing
			return
		}
		if active {
			g.env.Enqueue(sim.CmdAbandonMission{ID: r.ID})
			return
		}
		g.env.Enqueue(sim.CmdAcceptMission{ID: r.ID})
	})
	btn.GetWidget().Disabled = r.Disabled

	// The active line is title + progress ("Belt cleanup — 1/3") and only
	// fits the title column at the tiny step in Turret Road (at small the
	// em-dash suffix alone costs 56 px); available titles, which carry no
	// suffix, stay at small.
	step := uikit.StepSmall
	if active {
		step = uikit.StepTiny
	}
	// The kind glyph leads the title; the title's fixed width gives back
	// the icon box, so every row keeps the same total width whether it
	// carries an icon or not (note rows never do).
	titleW := boardTextW
	if r.Icon != "" {
		titleW -= uikit.IconInline + uikit.IconGap
	}
	title := widget.NewText(
		widget.TextOpts.Text(g.ellip(r.Title, step, float64(titleW)), g.env.Face(step), solid(p.Accent)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.MinSize(titleW, 0)),
	)
	var titleCell widget.PreferredSizeLocateableWidget = title
	if r.Icon != "" {
		ctr := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(uikit.IconGap),
		)))
		icon := uikit.NewIcon(r.Icon, uikit.IconInline, p.Text)
		// Centre the glyph in the line: the row is as tall as its
		// button, and a top-flush icon reads as a misalignment.
		for _, w := range []widget.PreferredSizeLocateableWidget{icon, ctr} {
			w.GetWidget().LayoutData = widget.RowLayoutData{Position: widget.RowLayoutPositionCenter}
		}
		ctr.AddChild(icon, title)
		titleCell = ctr
	}

	if active {
		// One line: "Belt cleanup — 1/3" + Abandon.
		row := widget.NewContainer(
			widget.ContainerOpts.BackgroundImage(rowBG),
			widget.ContainerOpts.Layout(widget.NewRowLayout(
				widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
				widget.RowLayoutOpts.Spacing(boardBtnGap),
				widget.RowLayoutOpts.Padding(rowPad),
			)),
		)
		row.AddChild(titleCell, btn)
		return row, btn
	}

	// Three lines: title + Accept / summary [requirement] / reward. A
	// gated row parks its standing requirement beside the objective
	// (line 2) so line 3 gives the payout the whole row — the reward is
	// the one string on a board row that must never ellipsize.
	summaryW := float64(boardContentW)
	if r.Require != "" {
		reqW := g.textW(r.Require, uikit.StepTiny)
		if capW := summaryW - boardBtnGap - 40; reqW > capW {
			r.Require = g.ellip(r.Require, uikit.StepTiny, capW)
			reqW = g.textW(r.Require, uikit.StepTiny)
		}
		summaryW -= reqW + boardBtnGap
	}
	var line2 widget.PreferredSizeLocateableWidget = widget.NewText(
		widget.TextOpts.Text(g.ellip(r.Summary, uikit.StepTiny, summaryW), g.env.Face(uikit.StepTiny), solid(p.Dim)),
	)
	if r.Require != "" {
		req := widget.NewText(
			widget.TextOpts.Text(r.Require, g.env.Face(uikit.StepTiny), solid(p.Warn)),
		)
		ctr := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(boardBtnGap),
		)))
		ctr.AddChild(line2, req)
		line2 = ctr
	}

	reward := widget.NewText(
		widget.TextOpts.Text(g.ellip(r.Reward, uikit.StepTiny, float64(boardContentW)), g.env.Face(uikit.StepTiny), solid(p.Good)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.MinSize(boardContentW, 0)),
	)

	line1 := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewRowLayout(
		widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
		widget.RowLayoutOpts.Spacing(boardBtnGap),
	)))
	line1.AddChild(titleCell, btn)

	row := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(rowBG),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Padding(rowPad),
		)),
	)
	row.AddChild(line1, line2, reward)
	return row, btn
}

// objectiveSummary renders the contract's kind + objective, e.g.
// "Destroy 1× Scuttler", "Deliver 2 m³ EMP Charge to Penumbra Port",
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
// "40,000.00 cr · +0.10 standing" — the shared credit presentation
// (uikit.FormatCredits), the one every readout uses.
func rewardText(def *content.ContractDef) string {
	s := uikit.FormatCredits(def.Reward.Credits)
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
	return uikit.Ellipsize(s, w, func(x string) float64 {
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
