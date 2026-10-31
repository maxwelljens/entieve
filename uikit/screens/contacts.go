package screens

import (
	"fmt"
	"image"
	"math"
	"slices"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// The contacts view — the dock screen (CONTEXT.md "Contact", "Station
// board") — is a Screen panel of its own: the station services row (the
// docked station's face and its one navigation: Undock — every window it
// opens is the rail's own row, so the row carries no window buttons) over
// the contact offers the sim derives from world state, then the accepted
// work.
//
// It is the ONE window docking opens. The host asks it for its content floor
// (FitSize, DESIGN units) and mirrors that onto the window's MinSize and the
// declared column through the placement tracker — the panel never places or
// resizes itself, so a player-moved window keeps its position and a grip
// drag stops at the board's own height.
//
// Geometry (docs/GAMEPLAY.md, "Contacts & standing"): the station column
// while docked: x ∈ [StationX, StationX+StationWinW) starts clear of the
// navigation rail's gutter and keeps its right edge at 452. The board's
// content height (row caps included) is the window's floor; the column is
// at least StationMinH tall. Both are pinned by tests at 1280×720,
// 1024×700 and at the window minimum.
//
// Width is live, height is content: the window's resize grips change the
// column's width, and Sync re-derives every row's columns from the body's
// laid-out width (columnWidths), so Details lands on the row's right edge at
// any width. A taller window keeps the board top-anchored — the extra height
// stays empty rather than stretching the rows.
const (
	StationX     = 10 + RailInset // window left edge: rail gutter first
	StationY     = 8
	StationWinW  = 442 - RailInset // window width: 355, right edge 452 (the content floor)
	StationMinW  = StationWinW     // window MinSize — the "minimum window" pin
	StationMinH  = 200
	boardPad     = 9 // body padding
	boardSpacing = 7 // vertical gap between body children
	boardRowGap  = 5 // gap between two board rows

	// The rows themselves are rich rows of the table module, which owns
	// their internals (widgets.Rich*): the board's column budgets below
	// derive from them, so a rendered row and its ellipsis budgets can
	// never drift apart.
	boardBtnGap  = widgets.RichGap  // gap between a row's cells
	boardBtnW    = widgets.RichBtnW // action button width (fits the widest label, "Accepted", set in the emphasis face)
	availMaxRows = 3                // visible offer rows ("+N more" beyond)
	actMaxRows   = 2                // visible active rows ("+N more" beyond)

	// An offer row leads with the contact's 2:3 portrait, then the rich
	// row, whose lines are the title and the type chip beside the payout.
	// The row height is the driver: boardRowH is every row's floor and the
	// portrait spans it whole, so its width is two thirds of the height.
	// shortcut: a row taller than boardRowH (more aside lines than a
	// shipped offer has) would leave its portrait short of the row;
	// upgrade when a richer offer row ships.
	boardRowH     = 84
	boardRowInset = 6  // between the row's edge and the portrait
	boardThumbGap = 10 // between the portrait and the text stack
	boardThumbH   = boardRowH
	boardThumbW   = boardRowH * 2 / 3 // 2:3
	boardKindW    = 60                // the type chip's column: every reward starts on the same x
	stackPad      = 2 * widgets.RichPad

	boardInnerW = StationWinW - 2*boardPad // body content width at the floor: 337
)

// boardCols are one rebuild's row column budgets, in live pixels, derived
// from the body's content width and the portrait's width. A rich row's
// cells are single-line and ellipsized by the caller, so the columns are
// pure arithmetic — nothing here has to be measured first.
type boardCols struct {
	line   int // the rich stack's line width (portrait rows)
	text   int // line 1's title column, icon box included
	reward int // line 2's payout column
	title  int // active work's one-line title (no portrait)
}

// columnWidths is the whole row budget: the portrait row's stack width
// (row inset, portrait, gap and the stack's own padding reserved), then the
// title and payout columns inside it, and active work's title across the
// full body width. Every term is at least one pixel — a degenerate width
// ellipsizes rather than producing negative text boxes.
func columnWidths(inner, thumbW int) boardCols {
	line := max(1, inner-boardRowInset-thumbW-boardThumbGap-stackPad)
	return boardCols{
		line:   line,
		text:   max(1, line-theme.IconInline-theme.IconGap-boardBtnGap-boardBtnW),
		reward: max(1, line-boardKindW-boardBtnGap),
		title:  max(1, inner-stackPad-boardBtnGap-boardBtnW),
	}
}

// StationRect is the contacts window's rect in live px: the left column,
// clear of the navigation rail's gutter, at the board's content height (never
// below StationMinH). The game's Screen row declares the same column (its
// DefaultRect) and the host's FitSize path composes it from this height, so
// the declared rect and the fitted one can never disagree.
func StationRect(stationH int) image.Rectangle {
	return image.Rect(theme.Px(StationX), theme.Px(StationY),
		theme.Px(StationX+StationWinW), theme.Px(StationY)+max(stationH, theme.Px(StationMinH)))
}

// ContactsPanel is the docked station's contact view: the service row
// (identity / Undock / Cargo / this station's services), the live contact
// offers and the accepted work. Sync re-reads the derived offers every
// mounted frame and rebuilds the rows when the model moves or the window's
// width does.
type ContactsPanel struct {
	env widgets.Seams
	cv  widgets.ContentView

	win    *widget.Window
	svc    *Services
	body   *widget.Container
	offers *widget.Container
	active *widget.Container

	model     contactsModel
	offerRows []ContactRow
	actRows   []ContactRow

	// inner is the body's content width in DESIGN units, re-read from its
	// laid-out rect every Sync; cols are the row budgets it produces.
	inner int
	cols  boardCols

	// fit is the content floor in DESIGN units, cached at the last
	// rebuild (FitSize's answer — never a per-frame PreferredSize).
	fit int
}

// NewContacts builds the view over the given seams. The window is
// constructed empty at the declared column with the static floor; the host
// mounts it, runs Sync and then applies FitSize.
func NewContacts(env widgets.Seams) *ContactsPanel {
	p := env.Pal()
	b := &ContactsPanel{
		env: env, cv: env, fit: StationMinH,
		inner: boardInnerW, cols: columnWidths(boardInnerW, boardThumbW),
	}
	b.svc = NewServices(env, boardInnerW)
	b.offers = newBoardSection()
	b.active = newBoardSection()

	offersHead := widget.NewText(
		widget.TextOpts.Text("Offers", env.Face(theme.StepSmall), theme.Solid(p.Accent)),
	)
	activeHead := widget.NewText(
		widget.TextOpts.Text("Active work", env.Face(theme.StepSmall), theme.Solid(p.Accent)),
	)

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.SquareBox(theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(boardSpacing)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(boardPad))),
		)),
	)
	body.AddChild(b.svc.Container(), offersHead, b.offers, activeHead, b.active)

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
func (b *ContactsPanel) Win() *widget.Window { return b.win }

// Services returns the service row above the board (the panel's identity
// line and per-service buttons).
func (b *ContactsPanel) Services() *Services { return b.svc }

// FitSize is the board's content floor in DESIGN units: the column's fixed
// width and the height the last rebuild measured (never below StationMinH).
// The host mirrors it onto the window's MinSize and composes the declared
// column from it.
func (b *ContactsPanel) FitSize() (w, h int) { return StationWinW, b.fit }

// Sync refreshes the service row every frame and rebuilds the contact rows
// when the model moves or the window's width does — the one refresh path
// (the host syncs mounted panels). The width read is what makes a grip drag
// reach the rows: the body is laid out at the window's new width, and the
// next Sync re-derives every column from it. A frame where neither moved
// rebuilds nothing.
func (b *ContactsPanel) Sync() {
	if b.cv == nil {
		return
	}
	resized := false
	if w := b.liveWidth(); w != b.inner {
		b.inner = w
		b.svc.SetWidth(w)
		resized = true
	}
	b.svc.Sync()
	m := b.build()
	if resized || !m.same(b.model) {
		b.model = m
		b.rebuild(m)
	}
}

// liveWidth is the body's content width in DESIGN units, read from its
// laid-out rect — the window's chrome resizes the rect, and everything the
// board draws derives from this width. A body not laid out yet (the mount
// frame) answers the column width it was built at.
func (b *ContactsPanel) liveWidth() int {
	d := b.body.GetWidget().Rect.Dx()
	if d <= 0 {
		return b.inner
	}
	return max(1, int(math.Round(float64(d)/theme.Scale()))-2*boardPad)
}

// newBoardSection builds one row section: a bare container whose rows are
// tiled by the table module's row stack — equal heights, boardRowGap apart,
// the pure pin of which is boardRowRects.
func newBoardSection() *widget.Container {
	return widget.NewContainer(
		widget.ContainerOpts.Layout(widgets.StackLayout{Gap: theme.Px(boardRowGap)}),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
}

// boardRowRects tiles up to n rows inside inner: equal heights, equal
// boardRowGap gaps, first row flush with inner's top — every rect pairwise
// disjoint and fully inside inner. n <= 0 (or an inner too small to hold even
// one pixel-high row) returns no rects.
func boardRowRects(inner image.Rectangle, n int) []image.Rectangle {
	return widgets.StackRects(inner, n, theme.Px(boardRowGap))
}

// capRows splits n rows into the visible count and the hidden remainder: at
// most cap rows render, and when truncated the last visible slot becomes a
// "+N more" note (so a section's height never depends on n exceeding the cap).
func capRows(n, cap int) (show, hidden int) {
	if n <= cap {
		return n, 0
	}
	return cap - 1, n - (cap - 1)
}

// ContactRow is one rendered row's handle — its offer id plus the two things
// widgets.StackRow rendered (the root the section tiles, the trailing action
// button) — so the host's tests and captures can find, read and press it.
// Note rows ("+N more", the empty states) carry a nil button.
type ContactRow struct {
	ID   content.ContractID
	Btn  *widget.Button
	Root *widget.Container
}

// Rows returns the rows rendered into one section, in order: offers (active
// false) or accepted work. It is the handle tests and captures use; the
// panel's model stays unexported.
func (b *ContactsPanel) Rows(active bool) []ContactRow {
	if active {
		return b.actRows
	}
	return b.offerRows
}

// contactRowModel is one row in pure, comparable form — the widget tree is
// rebuilt from the model whenever its signature changes, which makes the
// refresh rule simple: any state the rows display must appear in the model.
type contactRowModel struct {
	ID       content.ContractID
	Contact  content.ContactID // portrait key, offers only
	Title    string            // line 1
	Kind     string            // the mission-type chip, offers only
	Reward   string            // line 2, offers only (the money and the item)
	Require  string            // the standing gate, gated offers only
	Gain     string            // the standing gain, offers only
	Stock    string            // the destination's requisition stock, deliveries only
	Icon     string            // uikit icon name for the objective kind ("" = none)
	Action   string            // button label; "" for note rows
	Disabled bool
	Note     bool // "+N more" / empty-state row (no button)
}

// contactsModel is the whole view: the docked station's live offers plus
// every accepted offer.
type contactsModel struct {
	Station  content.CelID
	Standing float64
	Offers   []contactRowModel
	Active   []contactRowModel
}

// same is the change detector for Sync: field-wise equality against the last
// rebuilt model (contactRowModel is comparable, so the rows compare through
// slices.Equal).
func (m contactsModel) same(o contactsModel) bool {
	return m.Station == o.Station && m.Standing == o.Standing &&
		slices.Equal(m.Offers, o.Offers) && slices.Equal(m.Active, o.Active)
}

func noteRow(text string) contactRowModel {
	return contactRowModel{Title: text, Note: true}
}

// build derives the view from the ContentView: the docked station's live
// offers (derived by the sim, gated by standing, disabled once accepted) and
// the accepted work with its live progress. It never mutates anything.
func (b *ContactsPanel) build() contactsModel {
	var m contactsModel
	at := b.cv.Contacts()
	if at.Station == "" {
		return m // undocked: empty view (the window is unmounted anyway)
	}
	m.Station = at.Station
	m.Standing = at.Standing

	accepted := map[content.ContractID]bool{}
	for _, ms := range at.Active {
		accepted[ms.ID] = true
	}

	show, hidden := capRows(len(at.Offers), availMaxRows)
	for i := 0; i < show; i++ {
		o := at.Offers[i]
		contact, _ := b.cv.Defs().ContactForOffer(o.Def.ID)
		r := contactRowModel{
			ID:      o.Def.ID,
			Contact: contact,
			Title:   o.Def.Title,
			Kind:    theme.ContractLabel(o.Def.Kind),
			Reward:  theme.FormatCredits(o.Def.Reward.Credits),
			Icon:    theme.ContractIcon(o.Def.Kind),
			Action:  "Details",
		}
		if o.Def.Reward.StandingGain != 0 {
			r.Gain = fmt.Sprintf("%+.2f standing", o.Def.Reward.StandingGain)
		}
		if o.Def.Reward.Item != "" {
			r.Reward += " · " + b.cv.TradeableName(o.Def.Reward.Item)
		}
		if o.Def.Kind == content.ContractDeliver {
			r.Stock = fmt.Sprintf("in stock %d", o.Stock)
		}
		if accepted[o.Def.ID] {
			r.Action, r.Disabled = "Accepted", true
		} else if !b.cv.MeetsStanding(o.MinStanding) { // the sim's accept gate, epsilon included
			r.Require = gateText(o.MinStanding)
		}
		m.Offers = append(m.Offers, r)
	}
	switch {
	case hidden > 0:
		m.Offers = append(m.Offers, noteRow(fmt.Sprintf("+%d more", hidden)))
	case len(at.Offers) == 0:
		m.Offers = append(m.Offers, noteRow("No contacts have work for you here."))
	}

	type acceptedWork struct {
		def *content.ContractDef
		ms  sim.MissionState
	}
	var work []acceptedWork
	for _, ms := range at.Active {
		def := ms.Def
		if def == nil {
			if d, ok := b.cv.Defs().Contract(ms.ID); ok { // pre-contacts save
				def = d
			}
		}
		if def == nil {
			continue // stale save refs are skipped, not rendered
		}
		work = append(work, acceptedWork{def, ms})
	}
	show, hidden = capRows(len(work), actMaxRows)
	for i := 0; i < show; i++ {
		a := work[i]
		have := a.ms.Progress
		if a.def.Kind == content.ContractDeliver {
			have = b.cv.DeliverProgress(a.def) // the sim counts deliver objectives only at the claim
		}
		m.Active = append(m.Active, contactRowModel{
			ID:     a.ms.ID,
			Title:  a.def.Title + ": " + fmt.Sprintf("%d/%d", int(math.Round(have)), a.def.Objective.Count),
			Icon:   theme.ContractIcon(a.def.Kind),
			Action: "Abandon",
		})
	}
	switch {
	case hidden > 0:
		m.Active = append(m.Active, noteRow(fmt.Sprintf("+%d more", hidden)))
	case len(work) == 0:
		m.Active = append(m.Active, noteRow("No active work."))
	}
	return m
}

// rebuild replaces both sections' row widgets from the model and caches the
// body's content height (the row caps in the model bound the board's height).
func (b *ContactsPanel) rebuild(m contactsModel) {
	b.offers.RemoveChildren()
	b.active.RemoveChildren()
	b.offerRows, b.actRows = nil, nil
	b.cols = columnWidths(b.inner, boardThumbW)
	for _, r := range m.Offers {
		root, btn := b.buildRow(r, false)
		b.offers.AddChild(root)
		b.offerRows = append(b.offerRows, ContactRow{ID: r.ID, Btn: btn, Root: root})
	}
	for _, r := range m.Active {
		root, btn := b.buildRow(r, true)
		b.active.AddChild(root)
		b.actRows = append(b.actRows, ContactRow{ID: r.ID, Btn: btn, Root: root})
	}
	_, h := b.body.PreferredSize()
	// The body's preferred height, in the DESIGN units the host fits
	// with: the floor the grip stops at and FitSize caches.
	b.fit = max(int(math.Round(float64(h)/theme.Scale())), StationMinH)
}

// buildRow renders one row: an offer is the contact's portrait beside a
// rich row of the table module (widgets.NewStackRow) — the title with the
// kind icon leading it, the mission-type chip and the payout under it and
// the row's own RowSpec.Action drawn as its trailing buttons (Details) —
// while active work keeps its one-line title/progress and Abandon. The row's
// context menu rides the RowSpec seam: a right-click opens the same single
// action the button carries. Returns a nil button for note rows.
func (b *ContactsPanel) buildRow(r contactRowModel, active bool) (*widget.Container, *widget.Button) {
	p := b.env.Pal()
	row := widgets.TableRow{}
	if r.Icon != "" {
		row.Icons = []string{r.Icon}
	}
	menuSpec := widgets.RowSpec{}
	if !r.Note {
		menuSpec = b.contactRowSpec(r, active)
		row.Spec = menuSpec
		if r.Action != "" && r.Action != menuSpec.Actions[0].Label {
			row.Spec.Actions = append([]widgets.RowAction(nil), menuSpec.Actions...)
			row.Spec.Actions[0].Label = r.Action
		}
		row.Stack = b.rowStack(r, active)
	} else {
		row.Stack = []widgets.RowLine{{Segs: []widgets.RowSeg{{
			Text: b.ellip(r.Title, theme.StepTiny, float64(theme.Px(b.cols.line))),
			Tint: p.Dim,
			Step: theme.StepTiny,
		}}}}
	}
	sr := widgets.NewStackRow(b.env, row, b.env.Face(theme.StepSmall))
	root := sr.Root
	if !active && !r.Note {
		root = b.offerRow(r, sr.Root)
	}
	root.SetBackgroundImage(theme.SquareBox(theme.NRGBA(p.Cell, 160), theme.NRGBA(p.Border, 140), 1))
	root.GetWidget().MouseButtonClickedEvent.AddHandler(func(args any) {
		a, ok := args.(*widget.WidgetMouseButtonClickedEventArgs)
		if !ok || a.Button != ebiten.MouseButtonRight {
			return
		}
		widgets.OpenRowMenu(b.cv, b.env, menuSpec, widgets.ClickPoint(a))
	})
	if len(sr.Buttons) == 0 {
		return root, nil
	}
	return root, sr.Buttons[0]
}

// offerRow wraps one offer's rich row beside the contact's portrait:
// [face] [title · kind · payout · Details]. The row's floor is boardRowH and
// the portrait spans it whole; both are centred, so the text floats in the
// portrait's height.
func (b *ContactsPanel) offerRow(r contactRowModel, stack *widget.Container) *widget.Container {
	port := newPortraitBox(b.env, boardThumbW, boardThumbH, theme.IconInline)
	port.Set(r.Contact)
	outer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(boardThumbGap)),
			widget.RowLayoutOpts.Padding(&widget.Insets{Left: theme.Px(boardRowInset)}),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(0, theme.Px(boardRowH))),
	)
	for _, c := range []widget.PreferredSizeLocateableWidget{port.Widget(), stack} {
		c.GetWidget().LayoutData = widget.RowLayoutData{Position: widget.RowLayoutPositionCenter}
	}
	outer.AddChild(port.Widget(), stack)
	return outer
}

// rowStack is one row's lines: an offer leads with its title (the kind icon
// in the row's icon box) and the mission-type chip beside the payout under
// it; active work keeps its single title + progress line. The title keeps its
// fixed width (the ellipsis budget the row reserves, icon box included), so
// every row measures the same with or without a glyph.
func (b *ContactsPanel) rowStack(r contactRowModel, active bool) []widgets.RowLine {
	p := b.env.Pal()
	if active {
		titleW := theme.Px(b.cols.title)
		if r.Icon != "" {
			titleW -= theme.Px(theme.IconInline) + theme.Px(theme.IconGap)
		}
		return []widgets.RowLine{{Segs: []widgets.RowSeg{{
			Text: b.ellip(r.Title, theme.StepTiny, float64(titleW)),
			Tint: p.Accent,
			Step: theme.StepTiny,
			MinW: titleW,
		}}}}
	}
	lines := []widgets.RowLine{{Segs: []widgets.RowSeg{{
		Text: b.ellip(r.Title, theme.StepSmall, float64(theme.Px(b.cols.text))),
		Tint: p.Accent,
		Step: theme.StepSmall,
		MinW: theme.Px(b.cols.text),
	}}}}

	// Line 2: the type chip, then the payout.
	lines = append(lines, widgets.RowLine{Segs: []widgets.RowSeg{
		{Text: r.Kind, Tint: p.Head, Step: theme.StepTiny, MinW: theme.Px(boardKindW)},
		{Text: b.ellip(r.Reward, theme.StepTiny, float64(theme.Px(b.cols.reward))), Tint: p.Good, Step: theme.StepTiny},
	}})

	// Line 3: the asides a summary row still owes the player — why the
	// accept inside Details is gated, the standing the work pays and a
	// delivery's live stock.
	var aside []widgets.RowSeg
	if r.Require != "" {
		aside = append(aside, widgets.RowSeg{Text: r.Require, Tint: p.Warn, Step: theme.StepTiny})
	}
	if r.Gain != "" {
		aside = append(aside, widgets.RowSeg{Text: r.Gain, Tint: p.Dim, Step: theme.StepTiny})
	}
	if r.Stock != "" {
		aside = append(aside, widgets.RowSeg{Text: r.Stock, Tint: p.Dim, Step: theme.StepTiny})
	}
	if len(aside) > 0 {
		lines = append(lines, widgets.RowLine{Segs: aside})
	}
	return lines
}

// contactRowSpec is one row's declared interactions: Details on a live offer
// (the dialog carries the accept), Abandon on active work — through the same
// command the board always emitted. The offer def is derived, so the row
// declares no Ref (Details is its one entry point).
func (b *ContactsPanel) contactRowSpec(r contactRowModel, active bool) widgets.RowSpec {
	label, intent := "Details", widgets.Intent(widgets.ShowContact{Offer: r.ID})
	if active {
		label, intent = "Abandon", widgets.Abandon{Contract: r.ID}
	}
	return widgets.RowSpec{Actions: []widgets.RowAction{{
		Label:    label,
		Disabled: r.Disabled,
		Intent:   intent,
	}}}
}

// objectiveRef moved to contactwin.go: the Details dialog resolves the
// objective for its Inspect verb.

// gateText is the standing gate stated the same way on the row and in the
// Details dialog.
func gateText(min float64) string { return fmt.Sprintf("Requires standing %.2f", min) }

// rewardText renders the offer's reward line, e.g.
// "40,000.00 € · +0.10 standing" — the shared money presentation
// (theme.FormatCredits), the one every readout uses.
func rewardText(def *content.ContractDef) string {
	s := theme.FormatCredits(def.Reward.Credits)
	if def.Reward.StandingGain != 0 {
		s += fmt.Sprintf(" · %+.2f standing", def.Reward.StandingGain)
	}
	return s
}

// ellip truncates s with "…" so it measures at most w in the given
// type-scale step (single line: board rows never wrap).
func (b *ContactsPanel) ellip(s, step string, w float64) string {
	face := b.env.Face(step)
	return widgets.Ellipsize(s, w, func(x string) float64 {
		tw, _ := text.Measure(x, *face, 0)
		return tw
	})
}

// textW measures s in the given type-scale step (row-column sizing).
func (b *ContactsPanel) textW(s, step string) float64 {
	face := b.env.Face(step)
	w, _ := text.Measure(s, *face, 0)
	return w
}
