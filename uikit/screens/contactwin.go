package screens

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// The Details dialog — the click behind an offer row (CONTEXT.md "Dialog",
// "Contact offer"): the contact's face, their own words with the sim's
// numbers filled in, the contract at a glance and the verbs that act on it.
// It is one instance re-targeted in place, closed by its × / Decline and
// mounted by the Kit (a bare session only reveals it).
//
// Widths: the dialog is fixed (the board column it opens over is); the
// portrait is the art's 2:3 and the identity column beside it takes the
// rest, so a long contact name wraps instead of widening the window.
const (
	contactW     = 552
	contactH     = 400 // the floor that holds every shipped offer's page (pinned by test)
	contactPortW = 144
	contactPortH = 216 // 2:3
	contactGap   = theme.SpMd
	contactField = 74 // contract label column

	contactInnerW = contactW - 2*theme.ContentsPad
	// The page is two columns: the face and its identity on the left, the
	// words, the contract and the verbs on the right.
	contactLeftW = contactPortW
	contactColW  = contactInnerW - contactPortW - contactGap
	contactValW  = contactColW - contactField - contactGap

	// contactGlyph is the fallback mark's design box: half a hero glyph,
	// centred in the portrait's dark plate.
	contactGlyph     = theme.IconHero / 2
	portraitFallback = "svc_contracts"
)

// portraitBox is one 2:3 contact portrait: the dropped bust
// (assets/portraits/<contact-id>, png or jpg) cover-fit into the box, else the
// contract glyph on the same box — no person silhouette ships, so the
// office's own mark stands in.
type portraitBox struct {
	box     *widget.Container
	img     *ebiten.Image
	gfx     *widget.Graphic
	glyph   *theme.IconBox
	pal     theme.Palette
	id      content.ContactID
	dropped bool
}

// newPortraitBox builds the empty box at a w×h design size; Set fills it.
func newPortraitBox(env widgets.Seams, w, h, glyph int) *portraitBox {
	p := env.Pal()
	b := &portraitBox{pal: p, dropped: true}
	b.box = widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.SquareBox(theme.NRGBA(p.BGHard, 220), theme.NRGBA(p.Border, 160), 1)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(w), theme.Px(h))),
	)
	b.img = ebiten.NewImage(theme.Px(w), theme.Px(h))
	b.gfx = widget.NewGraphic(
		widget.GraphicOpts.Image(b.img),
		widget.GraphicOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(w), theme.Px(h))),
	)
	b.glyph = theme.NewIconBox(glyph)
	for _, c := range []widget.PreferredSizeLocateableWidget{b.gfx, b.glyph.Widget()} {
		c.GetWidget().LayoutData = widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionCenter,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
		}
		b.box.AddChild(c)
	}
	b.Set("")
	return b
}

// Widget returns the box for a container.
func (b *portraitBox) Widget() *widget.Container { return b.box }

// Set points the box at a contact: the dropped portrait when there is one,
// the fallback glyph otherwise.
func (b *portraitBox) Set(id content.ContactID) {
	dropped := paintPortrait(b.img, id)
	if id == b.id && dropped == b.dropped {
		return
	}
	b.id, b.dropped = id, dropped
	theme.ShowWidget(b.gfx.GetWidget(), dropped)
	if dropped {
		b.glyph.Set("", b.pal.Dim)
		return
	}
	b.glyph.Set(portraitFallback, b.pal.Dim)
}

// paintPortrait covers dst with the contact's dropped portrait, or reports
// false so the caller draws the fallback glyph. The source is the portrait's
// mip level that covers dst (render.ContactPortraitFor), so a 56×84 thumb
// samples a level built for it instead of the full-size photo. A stray
// aspect is cropped, never letterboxed — a portrait fills its frame.
func paintPortrait(dst *ebiten.Image, id content.ContactID) bool {
	db := dst.Bounds()
	src := render.ContactPortraitFor(id, db.Dx(), db.Dy())
	if src == nil {
		return false
	}
	dst.Clear()
	sb := src.Bounds()
	s := math.Max(float64(db.Dx())/float64(sb.Dx()), float64(db.Dy())/float64(sb.Dy()))
	var op ebiten.DrawImageOptions
	op.GeoM.Scale(s, s)
	op.GeoM.Translate(
		(float64(db.Dx())-float64(sb.Dx())*s)/2,
		(float64(db.Dy())-float64(sb.Dy())*s)/2,
	)
	dst.DrawImage(src, &op)
	return true
}

// ContactWin is the Details dialog: one contact offer as a page — the face,
// the speech, the contract and the verbs. It re-reads the offer every Sync,
// so an accepted offer flips its verb on the next frame and a gate that
// moved re-disables it with its reason.
type ContactWin struct {
	th theme.Theme
	cv widgets.ContentView
	as widgets.ActionSink

	win   *widget.Window
	title *theme.ChromeText
	body  *widget.Container

	port  *portraitBox
	name  *widget.Text
	sub   *widget.Text
	place *widget.Text

	speech *widget.Text
	attr   *widget.Text

	kindMark  *theme.IconBox
	kind      *widget.Text
	kindRow   *widget.Container
	objective *widget.Text
	dest      *widget.Container // the destination row (deliveries only)
	destText  *widget.Text
	reward    *widget.Text
	gate      *widget.Container
	gateText  *widget.Text

	accept  *widget.Button
	inspect *widget.Button
	decline *widget.Button
	// The verbs' current labels: a button's text widget exists only once it
	// is initialized (mounted), so the page tracks them itself.
	acceptLabel  string
	inspectLabel string

	offer content.ContractID
	last  sim.Offer // the last page rendered (the offer copy Sync falls back to)
	open  bool

	onClose func() // Kit-hosted: unmount (a bare env only hides)
}

// NewContactWin builds the closed dialog over the given environment.
func NewContactWin(env widgets.Seams) *ContactWin {
	p := env.Pal()
	w := &ContactWin{th: env, cv: env, as: env}

	text := func(face *text.Face, col color.RGBA, maxW int) *widget.Text {
		return widget.NewText(
			widget.TextOpts.Text("", face, theme.Solid(col)),
			widget.TextOpts.MaxWidth(float64(theme.Px(maxW))),
		)
	}
	w.name = text(env.FaceBold(theme.StepSmall), p.Accent, contactLeftW)
	w.sub = text(env.Face(theme.StepTiny), p.Dim, contactLeftW)
	w.place = text(env.Face(theme.StepTiny), p.Dim, contactLeftW)
	w.speech = text(env.Face(theme.StepSmall), p.Text, contactColW)
	w.attr = text(env.Face(theme.StepTiny), p.Dim, contactColW)

	w.port = newPortraitBox(env, contactPortW, contactPortH, contactGlyph)

	// The identity column: who is talking, who they answer to, where.
	ident := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.SpSm)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(0)),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(contactLeftW), 0)),
	)
	ident.AddChild(w.name, w.sub, w.place)
	left := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(contactGap)),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Position: widget.RowLayoutPositionStart})),
	)
	left.AddChild(w.port.Widget(), ident)

	// The contract's field rows: a fixed label column so every value starts
	// on the same x.
	field := func(label string) *widget.Text {
		return widget.NewText(
			widget.TextOpts.Text(label, env.Face(theme.StepTiny), theme.Solid(p.Head)),
			widget.TextOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(contactField), 0)),
		)
	}
	w.kind = text(env.Face(theme.StepTiny), p.Text, contactValW)
	w.objective = text(env.Face(theme.StepTiny), p.Text, contactValW)
	w.destText = text(env.Face(theme.StepTiny), p.Dim, contactValW)
	w.reward = text(env.Face(theme.StepTiny), p.Good, contactValW)
	w.gateText = text(env.Face(theme.StepTiny), p.Warn, contactValW)

	row := func(label string, cells ...widget.PreferredSizeLocateableWidget) *widget.Container {
		c := widget.NewContainer(
			widget.ContainerOpts.Layout(widget.NewRowLayout(
				widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
				widget.RowLayoutOpts.Spacing(theme.Px(contactGap)),
			)),
		)
		c.AddChild(field(label))
		c.AddChild(cells...)
		return c
	}
	// The type row leads with the kind's own glyph (the same manifest mark
	// the board row carries), then the label.
	w.kindMark = theme.NewIconBox(theme.IconInline)
	w.kindRow = row("Type", w.kindMark.Widget(), w.kind)
	w.dest = row("Destination", w.destText)
	w.gate = row("Standing", w.gateText)

	// The verbs: every one acts. Accept rides the same intent the board's
	// old button did; Inspect reuses the Show Info seam; Decline just closes.
	btn := func(text string, clicked func()) *widget.Button {
		return widget.NewButton(
			widget.ButtonOpts.TextLabel(text),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(theme.Px(120), theme.Px(40)),
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
			widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) { clicked() }),
		)
	}
	w.acceptLabel, w.inspectLabel = "Accept work", "Inspect"
	w.accept = btn("Accept work", func() {
		// Grayed rows emit nothing — belt and braces with the disabled
		// widget (a harness may fire the click event straight past the gate).
		if w.accept.GetWidget().Disabled {
			return
		}
		w.as.Do(widgets.Accept{Contract: w.offer})
	})
	w.inspect = btn("Inspect", func() {
		if ref, ok := w.inspectRef(); ok {
			w.as.Do(widgets.ShowInfo{Ref: ref})
		}
	})
	w.decline = btn("Decline", w.Close)
	verbs := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(contactGap)),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	verbs.AddChild(w.accept, w.inspect, w.decline)

	// The right column: the contact's words, then the contract and the verbs.
	right := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.SpSm)),
		)),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			widget.WidgetOpts.MinSize(theme.Px(contactColW), 0),
		),
	)
	right.AddChild(
		sectionBar(env, "BRIEFING"), w.speech, w.attr,
		sectionBar(env, "CONTRACT"), w.kindRow, row("Objective", w.objective), w.dest, row("Reward", w.reward), w.gate,
		verbs,
	)

	cols := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(contactGap)),
		)),
	)
	cols.AddChild(left, right)

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.WindowBodyBox(p, theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200))),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.SpSm)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	body.AddChild(cols)
	w.body = body

	bar, title := theme.NewTitleBar(env, "Contact", "")
	w.title = title
	w.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(bar, theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(contactW), theme.Px(contactH)),
		// Fallback only: centred for a 1024×700 viewport, the smallest the
		// game runs (a transient dialog's rect is never persisted).
		widget.WindowOpts.Location(image.Rect(
			theme.Px((1024-contactW)/2), theme.Px((700-contactH)/2),
			theme.Px((1024-contactW)/2+contactW), theme.Px((700-contactH)/2+contactH))),
	)
	theme.ShowWidget(w.win.GetContainer().GetWidget(), false)
	return w
}

// Win returns the window for Kit.AddWindow / the host's hit test.
func (w *ContactWin) Win() *widget.Window { return w.win }

// Show re-targets the dialog on one live offer and reveals it. It reports
// whether the offer is live: an unknown id (the gap closed) opens nothing.
func (w *ContactWin) Show(offer content.ContractID) bool {
	o, ok := w.live(offer)
	if !ok {
		return false
	}
	w.offer, w.open = offer, true
	w.render(o)
	theme.ShowWidget(w.win.GetContainer().GetWidget(), true)
	return true
}

// Sync re-reads the shown offer: an accepted one flips its verb, a standing
// gate that moved re-disables it with its reason, and the delivery stock
// follows the destination's book. An offer that vanished from the board
// (its gap closed) keeps the page it had — the player closes the window.
func (w *ContactWin) Sync() {
	if !w.open || w.cv == nil {
		return
	}
	if o, ok := w.live(w.offer); ok {
		w.refresh(o)
		return
	}
	if w.accepted() {
		if w.last.Def != nil {
			w.gateText.Label = w.claimLine(w.last.Def)
		}
		w.setAccept("Accepted", true)
	} else {
		w.setAccept("Accept work", true) // no longer offered: nothing to accept
	}
}

// Close closes the dialog: through the Kit it also unmounts; in a bare
// environment it just hides.
func (w *ContactWin) Close() {
	if w.onClose != nil {
		w.onClose()
		return
	}
	w.hide()
}

// hide flips the container invisible (the kit's unmount path).
func (w *ContactWin) hide() {
	w.open = false
	theme.ShowWidget(w.win.GetContainer().GetWidget(), false)
}

// IsOpen reports whether the dialog is revealed.
func (w *ContactWin) IsOpen() bool {
	return w.win != nil && w.win.GetContainer().GetWidget().IsVisible()
}

// Offer is the offer the dialog currently shows ("" before the first Show).
func (w *ContactWin) Offer() content.ContractID { return w.offer }

// AcceptButton is the accept verb (tests press it and read its gate).
func (w *ContactWin) AcceptButton() *widget.Button { return w.accept }

// InspectButton is the objective's Show Info verb (hidden when the
// objective has no definition to open).
func (w *ContactWin) InspectButton() *widget.Button { return w.inspect }

// GateText is the standing line the page states under the contract.
func (w *ContactWin) GateText() string { return w.gateText.Label }

// TitleText is the title-bar label (furniture: the page names the contact).
func (w *ContactWin) TitleText() string {
	if w.title == nil {
		return ""
	}
	return w.title.Label
}

// render fills the whole page from a live offer.
func (w *ContactWin) render(o sim.Offer) {
	w.last = o
	w.port.Set(w.contact(o))
	w.name.Label = o.ContactName
	w.sub.Label = theme.PatronLabel(o.Patron)
	w.place.Label = w.office()
	w.speech.Label = o.Prose
	w.attr.Label = "— " + o.ContactName
	w.refresh(o)
}

// refresh updates the contract block and the verbs from the live offer.
func (w *ContactWin) refresh(o sim.Offer) {
	w.last = o
	def := o.Def
	w.kindMark.Set(theme.ContractIcon(def.Kind), w.th.Pal().Dim)
	w.kind.Label = theme.ContractLabel(def.Kind) + "  ·  level " + strconv.Itoa(def.Level)
	w.objective.Label = w.objectiveText(def)
	w.reward.Label = rewardText(def)
	if def.Reward.Item != "" {
		w.reward.Label += " · " + w.cv.TradeableName(def.Reward.Item)
	}
	if def.Kind == content.ContractDeliver {
		theme.ShowWidget(w.dest.GetWidget(), true)
		w.destText.Label = fmt.Sprintf("%s · in stock %d",
			w.cv.StationName(sim.StationID(def.Objective.Destination)), o.Stock)
	} else {
		theme.ShowWidget(w.dest.GetWidget(), false)
	}
	// The gate is always stated: a gated offer explains its disabled
	// Accept, a cleared one names the standing the work asks for.
	switch {
	case w.accepted():
		w.gateText.Label = w.claimLine(def)
		w.setAccept("Accepted", true)
	case !w.cv.MeetsStanding(o.MinStanding):
		w.gateText.Label = gateText(o.MinStanding)
		w.setAccept("Accept work", true)
	default:
		w.gateText.Label = fmt.Sprintf("Requires standing %.2f — you have it", o.MinStanding)
		w.setAccept("Accept work", false)
	}
	w.setInspect(def)
}

// claimLine is the accepted offer's standing row: where the work is claimed.
func (w *ContactWin) claimLine(def *content.ContractDef) string {
	if def.Kind == content.ContractDeliver {
		return "Accepted — deliver it to " + w.cv.StationName(sim.StationID(def.Objective.Destination))
	}
	return "Accepted — claim it at " + w.cv.StationName(sim.StationID(def.Station))
}

// setAccept sets the accept verb's label and gate.
func (w *ContactWin) setAccept(label string, disabled bool) {
	if w.acceptLabel != label {
		w.acceptLabel = label
		w.accept.SetText(label)
	}
	w.accept.GetWidget().Disabled = disabled
}

// setInspect shows the Inspect verb only when the objective has a definition
// Show Info can open, and names what it opens.
func (w *ContactWin) setInspect(def *content.ContractDef) {
	_, ok := objectiveRef(w.cv, def)
	theme.ShowWidget(w.inspect.GetWidget(), ok)
	if !ok {
		return
	}
	label := "Inspect item"
	if def.Objective.Target != "" {
		label = "Inspect target"
	}
	if w.inspectLabel != label {
		w.inspectLabel = label
		w.inspect.SetText(label)
	}
}

// inspectRef resolves the shown offer's objective to its Show Info ref.
func (w *ContactWin) inspectRef() (widgets.InfoRef, bool) {
	o, ok := w.live(w.offer)
	if !ok {
		return widgets.InfoRef{}, false
	}
	return objectiveRef(w.cv, o.Def)
}

// live resolves one offer of the docked station's board.
func (w *ContactWin) live(id content.ContractID) (sim.Offer, bool) {
	if w.cv == nil || id == "" {
		return sim.Offer{}, false
	}
	for _, o := range w.cv.Contacts().Offers {
		if o.Def != nil && o.Def.ID == id {
			return o, true
		}
	}
	return sim.Offer{}, false
}

// accepted reports whether the shown offer is among the accepted work.
func (w *ContactWin) accepted() bool {
	for _, ms := range w.cv.Contacts().Active {
		if ms.ID == w.offer {
			return true
		}
	}
	return false
}

// contact is the offer's contact id (the portrait key). An offer whose
// contact cannot be resolved (a test fixture def) falls back to no art.
func (w *ContactWin) contact(o sim.Offer) content.ContactID {
	if c, ok := w.cv.Defs().ContactForOffer(o.Def.ID); ok {
		return c
	}
	return ""
}

// office is the posted-at line: the docked station and its system.
func (w *ContactWin) office() string {
	info, ok := w.cv.DockedStation()
	if !ok || info.Name == "" {
		return ""
	}
	if info.System == "" {
		return "Posted at " + info.Name
	}
	return "Posted at " + info.Name + " · " + info.System
}

// objectiveText is the objective in one scannable line: what, how many and —
// for a delivery — where.
func (w *ContactWin) objectiveText(def *content.ContractDef) string {
	c := def.Objective.Count
	switch {
	case def.Kind == content.ContractDeliver:
		return fmt.Sprintf("Haul %d × %s → %s", c,
			w.cv.TradeableName(def.Objective.Item),
			w.cv.StationName(sim.StationID(def.Objective.Destination)))
	case def.Objective.Target != "":
		name := string(def.Objective.Target)
		if npc, ok := w.cv.Defs().Npc(def.Objective.Target); ok {
			name = npc.Name
		}
		return fmt.Sprintf("Destroy %d × %s", c, name)
	case def.Objective.Item != "":
		return fmt.Sprintf("Mine %d × %s", c, w.cv.TradeableName(def.Objective.Item))
	}
	return "—"
}

// objectiveRef resolves an offer's objective to the definition its Show Info
// window reads: a kill offer links its caste, deliver/mine offers their item.
// The ref is validated against the loaded content (a stale ref shows nothing
// rather than a hollow window).
func objectiveRef(cv widgets.ContentView, def *content.ContractDef) (widgets.InfoRef, bool) {
	if def == nil || cv.Defs() == nil {
		return widgets.InfoRef{}, false
	}
	var ref widgets.InfoRef
	switch {
	case def.Objective.Target != "":
		ref = widgets.NpcRef(def.Objective.Target)
	case def.Objective.Item != "":
		ref, _ = widgets.ResolveTradeable(cv.Defs(), def.Objective.Item).Ref()
	default:
		return widgets.InfoRef{}, false
	}
	return cv.InfoRefOf(ref)
}
