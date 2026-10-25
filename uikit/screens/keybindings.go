package screens

import (
	"image"
	"image/color"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// KeybindRow is one row of the keybindings window: a rebindable action
// (its stable id, label and live key text) or, with Fixed set, one row of
// the informational fixed chord layer. Group names the section it draws
// under (the host derives it from which table declared the action); Key
// "" on a rebindable row means the action is unbound.
type KeybindRow struct {
	Action  string
	Label   string
	Key     string
	Group   string
	Fixed   bool
	Rebound bool // differs from the action's default
	Armed   bool
	Notice  string
}

// KeybindActions is the keybindings window's seam to its host (the game):
// the window owns no binding state, it renders the host's rows and routes
// the verbs. Rows is re-read every Sync; Begin arms one action's capture,
// ResetRow restores one action's default, Reset clears the player's
// overrides. All must be non-nil.
type KeybindActions struct {
	Rows     func() []KeybindRow
	Begin    func(action string)
	ResetRow func(action string)
	Reset    func()
}

// The controls window's geometry, in DESIGN units (scaled through Px where
// measured). The chip and Reset columns are fixed so every row's key and
// control line up; a fixed row keeps an empty third column, so its chip
// sits on the same edge as the rebindable ones above it.
const (
	keybindW        = 480
	keybindH        = 640
	keybindChipW    = 104
	keybindTailW    = 56
	keybindRowH     = 28
	keybindResetAll = 150 // the footer's Reset-all control
	keybindScroll   = 60  // one wheel notch in design px
	keybindHintText = "Click a row, then press a key\nBackspace clears · Esc cancels"
)

// keybindRowImages is the label button's chrome: transparent at rest and a
// quiet accent wash under the cursor. Reset and the chips keep the house
// bordered control look; a row that framed itself would read as a form.
func keybindRowImages(p theme.Palette) *widget.ButtonImage {
	quiet := theme.NRGBA(p.Accent, 0)
	return &widget.ButtonImage{
		Idle:         theme.Bordered(theme.NRGBA(p.BG, 0), quiet, 1),
		Hover:        theme.Bordered(theme.NRGBA(p.Accent, 22), quiet, 1),
		Pressed:      theme.Bordered(theme.NRGBA(p.Accent, 34), quiet, 1),
		PressedHover: theme.Bordered(theme.NRGBA(p.Accent, 40), theme.NRGBA(p.Accent, 140), 1),
		Disabled:     theme.Bordered(theme.NRGBA(p.BG, 0), quiet, 1),
	}
}

// chipState is one row's visual state (the chip's paint and the Reset
// control's visibility).
type chipState string

const (
	chipDefault   chipState = "default"
	chipRebound   chipState = "rebound"
	chipUnbound   chipState = "unbound"
	chipCapturing chipState = "capturing"
	chipFixed     chipState = "fixed"
)

// keybindRow is one built row: the clickable label, the key chip, the
// trailing Reset (a reserved spacer on a fixed row) and the last state and
// text written — Sync compares instead of restyling every frame.
type keybindRow struct {
	row   KeybindRow
	label widget.PreferredSizeLocateableWidget
	chip  *widget.Container
	chipT *widget.Text
	reset *widget.Button // nil on a fixed row

	state chipState
	key   string
}

// Keybindings is the rebinding window: a sectioned list of clickable rows,
// each with its live key in a status chip. Clicking a row arms it ("press
// a key…"); the host's key pass assigns the next key, Esc cancels and
// Backspace clears the binding. A rebound row offers Reset (its default
// back) and the footer's Reset all is two-step, the house destructive
// pattern. Like every panel it is rebuilt with the interface, so all
// displayed state is re-read from the actions in Sync.
type Keybindings struct {
	th  theme.Theme
	act KeybindActions
	win *widget.Window

	scroll *widget.ScrollContainer
	rail   *widgets.ScrollRail
	rows   []*keybindRow
	status *widget.Text
	note   string // last status line written
	reset  *widget.Button
	armed  bool

	face      *text.Face
	inner     float64 // the chip's text room
	labelRoom float64 // the label column's text room
	measure   func(string) float64

	footer  *widget.Container
	statusW float64
}

// NewKeybindings builds the Keybindings window over the shared
// environment. Mount it on a Kit with Kit.AddWindow and call Sync every
// frame.
func NewKeybindings(d widgets.Seams, act KeybindActions) *Keybindings {
	p := d.Pal()
	keyFace := d.Face(theme.StepSmall)
	measure := func(s string) float64 { return text.Advance(s, *keyFace) }
	k := &Keybindings{
		th: d, act: act,
		face: keyFace, inner: float64(theme.Px(keybindChipW) - 2*theme.Px(theme.SpSm)), measure: measure,
		labelRoom: float64(theme.Px(keybindW - 2*theme.ContentsPad - theme.ScrollGutter - 2*theme.ContentsPad -
			keybindChipW - keybindTailW - 2*theme.SpMd - 2*theme.SpSm)),
		statusW: float64(theme.Px(keybindW - 2*theme.ContentsPad - keybindResetAll - theme.SpMd)),
	}

	page := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(2)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)

	// One section per Group change, in the row stream's order: a tracked
	// header and the house hairline, then its rows.
	section := ""
	for _, r := range act.Rows() {
		if r.Group != section {
			section = r.Group
			if len(k.rows) > 0 {
				page.AddChild(widget.NewContainer(widget.ContainerOpts.WidgetOpts(
					widget.WidgetOpts.MinSize(0, theme.Px(theme.SpMd)))))
			}
			page.AddChild(theme.NewChromeText(d, theme.StepSmall, r.Group, p.Head))
			page.AddChild(widget.NewContainer(
				widget.ContainerOpts.BackgroundImage(theme.NineSlice(theme.NRGBA(p.Accent, widgets.TipRuleA))),
				widget.ContainerOpts.WidgetOpts(
					widget.WidgetOpts.MinSize(0, theme.Px(widgets.TipRuleH)),
					widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
				),
			))
		}
		kr := k.newRow(d, r)
		k.rows = append(k.rows, kr)
		page.AddChild(kr.rowWidget())
	}

	k.scroll = widget.NewScrollContainer(
		widget.ScrollContainerOpts.Content(page),
		widget.ScrollContainerOpts.StretchContentWidth(),
		widget.ScrollContainerOpts.Image(&widget.ScrollContainerImage{
			Idle: theme.Bordered(theme.WindowBody(p.BG), theme.NRGBA(p.Border, 160), 1),
			Mask: theme.NineSlice(theme.WindowBody(p.BG)),
		}),
	)
	// The viewport itself does not scroll: one wheel notch moves ScrollTop
	// (the InfoWin / title-window idiom).
	k.scroll.GetWidget().ScrolledEvent.AddHandler(func(args any) {
		ev, ok := args.(*widget.WidgetScrolledEventArgs)
		if !ok {
			return
		}
		_, ch := page.PreferredSize()
		over := ch - k.scroll.ViewRect().Dy()
		if over <= 0 {
			k.scroll.ScrollTop = 0
			return
		}
		k.scroll.ScrollTop = max(0, min(1, k.scroll.ScrollTop-ev.Y*float64(theme.Px(keybindScroll))/float64(over)))
	})
	widgets.AttachScrollContent(k.scroll, page)
	k.rail = widgets.NewScrollRail(d, k.scroll, k.scroll, page)

	// The footer: the status line left (the standing hint, or a capture's
	// refusal notice) and the two-step Reset all right.
	k.note = keybindHintText
	k.status = widget.NewText(
		widget.TextOpts.Text(k.note, d.Face(theme.StepTiny), theme.Solid(p.Dim)),
		// The status column wraps rather than truncating: the hint and the
		// refusal notices stay readable at every scale.
		widget.TextOpts.MaxWidth(k.statusW),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.GridLayoutData{
			VerticalPosition: widget.GridLayoutPositionCenter,
		})),
	)
	k.reset = widget.NewButton(
		widget.ButtonOpts.TextLabel("Reset all bindings"),
		widget.ButtonOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(keybindResetAll), theme.Px(keybindRowH))),
		widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) {
			if !k.armed {
				k.armed = true
				k.reset.SetText("Confirm reset all")
				return
			}
			k.armed = false
			k.reset.SetText("Reset all bindings")
			act.Reset()
		}),
	)
	k.reset.GetWidget().SetTheme(d.WidgetTheme())
	footer := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(2),
			widget.GridLayoutOpts.Stretch([]bool{true, false}, []bool{false}),
			widget.GridLayoutOpts.Spacing(theme.Px(theme.SpMd), 0),
		)),
	)
	footer.AddChild(k.status, k.reset)
	k.footer = footer

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.WindowBodyBox(p, theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200))),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true, false}),
			widget.GridLayoutOpts.Spacing(0, theme.Px(theme.SpMd)),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	body.AddChild(k.rail.Container(), footer)

	title, _ := theme.NewTitleBar(d, "Keybindings", "")

	k.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, theme.Px(theme.TitleBarH)),
		// Content floor: the window keeps this size; the list scrolls.
		widget.WindowOpts.MinSize(theme.Px(keybindW), theme.Px(keybindH)),
		// Fallback only: centred at the 1280×720 design size (the game
		// re-centres it from placeDefaults like the other overlays).
		widget.WindowOpts.Location(image.Rect(theme.Px(410), theme.Px(40), theme.Px(410+keybindW), theme.Px(40+keybindH))),
	)
	return k
}

// newRow builds one list row: [label][chip][Reset]. A rebindable label is
// the click target (it arms the capture); a fixed row's label is plain,
// dim text — informational, nothing to press — and its third column is an
// empty spacer of the Reset control's width, so chips stay aligned.
func (k *Keybindings) newRow(d widgets.Seams, r KeybindRow) *keybindRow {
	p := d.Pal()
	kr := &keybindRow{row: r, state: chipStateOf(r)}

	if r.Fixed {
		kr.label = widget.NewText(
			widget.TextOpts.Text(widgets.Ellipsize(r.Label, k.labelRoom, k.measure), d.Face(theme.StepSmall), theme.Solid(p.Dim)),
			// The label column's own inset, so a fixed row's text lines up
			// with the rebindable rows' button text.
			widget.TextOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.SpSm))),
			widget.TextOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(0, theme.Px(keybindRowH)),
				widget.WidgetOpts.LayoutData(widget.GridLayoutData{VerticalPosition: widget.GridLayoutPositionCenter}),
			),
		)
	} else {
		action := r.Action
		btn := widget.NewButton(
			widget.ButtonOpts.TextLabel(widgets.Ellipsize(r.Label, k.labelRoom, k.measure)),
			// A row is not a form field: no frame at rest, an accent wash
			// on hover, so the chips stay the page's only boxes.
			widget.ButtonOpts.Image(keybindRowImages(p)),
			// Left-aligned like a list row, not the theme's centred button
			// text; the furniture inset, not the theme's wider padding, so
			// the longest action label fits the label column.
			widget.ButtonOpts.TextPadding(widget.NewInsetsSimple(theme.Px(theme.SpSm))),
			widget.ButtonOpts.TextPosition(widget.TextPositionStart, widget.TextPositionCenter),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(0, theme.Px(keybindRowH)),
				widget.WidgetOpts.LayoutData(widget.GridLayoutData{VerticalPosition: widget.GridLayoutPositionCenter}),
			),
			widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) { k.act.Begin(action) }),
		)
		btn.GetWidget().SetTheme(d.WidgetTheme())
		kr.label = btn
		kr.reset = widget.NewButton(
			widget.ButtonOpts.TextLabel("Reset"),
			widget.ButtonOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(keybindTailW), theme.Px(keybindRowH-6))),
			widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) { k.act.ResetRow(action) }),
		)
		kr.reset.GetWidget().SetTheme(d.WidgetTheme())
		theme.ShowWidget(kr.reset.GetWidget(), false)
	}

	kr.key = keybindChipText(r)
	kr.chipT = widget.NewText(
		widget.TextOpts.Text(widgets.Ellipsize(kr.key, k.inner, k.measure), k.face, theme.Solid(p.Text)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionCenter,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
		})),
	)
	kr.chip = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(keybindChipW), theme.Px(keybindRowH))),
	)
	kr.chip.AddChild(kr.chipT)
	kr.style(k.th)

	return kr
}

// rowWidget assembles the row's three columns.
func (kr *keybindRow) rowWidget() *widget.Container {
	tail := widget.PreferredSizeLocateableWidget(widget.NewContainer(
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(keybindTailW), theme.Px(1))),
	))
	if kr.reset != nil {
		tail = kr.reset
	}
	row := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(3),
			widget.GridLayoutOpts.Stretch([]bool{true, false, false}, []bool{false}),
			widget.GridLayoutOpts.Spacing(theme.Px(theme.SpMd), 0),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	row.AddChild(kr.label, kr.chip, tail)
	return row
}

// style paints the chip for the row's state: fixed reads as the disabled
// control, default as a quiet cell, rebound/capturing accent, unbound dim.
func (kr *keybindRow) style(th theme.Theme) {
	p := th.Pal()
	body, edge := theme.Solid(p.Cell), theme.NRGBA(p.Border, 220)
	var ink color.Color = theme.Solid(p.Text)
	switch kr.state {
	case chipFixed:
		body, edge, ink = theme.NRGBA(p.Cell, 140), theme.NRGBA(p.Border, 110), theme.NRGBA(p.Dim, 160)
	case chipUnbound:
		edge, ink = theme.NRGBA(p.Border, 160), theme.Solid(p.Dim)
	case chipRebound:
		body, edge, ink = theme.NRGBA(p.Accent, 40), theme.Solid(p.Accent), theme.Solid(p.Accent)
	case chipCapturing:
		body, edge, ink = theme.NRGBA(p.Accent, 60), theme.Solid(p.Accent), theme.Solid(p.Accent)
	}
	kr.chip.SetBackgroundImage(theme.Bordered(body, edge, 1))
	kr.chipT.SetColor(ink)
}

// Win returns the keybindings window for Kit.AddWindow.
func (k *Keybindings) Win() *widget.Window { return k.win }

// chipStateOf classifies one row: fixed is informational, an armed row is
// capturing, an empty key is unbound, a changed key is rebound.
func chipStateOf(r KeybindRow) chipState {
	switch {
	case r.Fixed:
		return chipFixed
	case r.Armed:
		return chipCapturing
	case r.Key == "":
		return chipUnbound
	case r.Rebound:
		return chipRebound
	}
	return chipDefault
}

// keybindChipText is one chip's text: the key set, the capture prompt, or
// the em dash an unbound action shows.
func keybindChipText(r KeybindRow) string {
	switch {
	case r.Fixed:
		return r.Key
	case r.Armed:
		return "press a key…"
	case r.Key == "":
		return "—"
	}
	return r.Key
}

// Sync re-reads the rows and repaints only what changed: the chips (text
// and state), each row's Reset, and the footer status line (the capture's
// refusal notice while one is armed, the standing hint otherwise).
func (k *Keybindings) Sync() {
	rows := k.act.Rows()
	note := ""
	for i := range k.rows {
		if i >= len(rows) {
			break
		}
		r := rows[i]
		kr := k.rows[i]
		kr.row = r
		if s := chipStateOf(r); s != kr.state {
			kr.state = s
			kr.style(k.th)
		}
		if txt := keybindChipText(r); txt != kr.key {
			kr.key = txt
			kr.chipT.Label = widgets.Ellipsize(txt, k.inner, k.measure)
			kr.chip.RequestRelayout()
		}
		if kr.reset != nil {
			// Reset is offered whenever the row reads differently from its
			// default — a rebound action and an unbound one alike.
			theme.ShowWidget(kr.reset.GetWidget(), r.Rebound || r.Key == "")
		}
		if r.Armed && r.Notice != "" {
			note = r.Notice
		}
	}
	line, col := keybindHintText, k.th.Pal().Dim
	if note != "" {
		line, col = note, k.th.Pal().Warn
	}
	if line != k.note {
		k.note = line
		k.status.Label = line
		k.status.SetColor(theme.Solid(col))
		k.footer.RequestRelayout()
	}
	k.rail.Sync()
}

// ModelForTest returns the row shape the window last rendered, in draw
// order — the surface tests' handle on the sections and chips (never
// called in play).
func (k *Keybindings) ModelForTest() []KeybindRow {
	out := make([]KeybindRow, len(k.rows))
	for i, kr := range k.rows {
		out[i] = kr.row
	}
	return out
}

// ChipForTest returns one rebindable action's chip as drawn: its text and
// the state name it is painted in ("default", "rebound", "unbound",
// "capturing"). ok is false for an action the window does not list.
func (k *Keybindings) ChipForTest(action string) (text, state string, ok bool) {
	for _, kr := range k.rows {
		if kr.row.Action == action && !kr.row.Fixed {
			return kr.key, string(kr.state), true
		}
	}
	return "", "", false
}

// ResetVisibleForTest reports whether a row's per-row Reset control is on
// screen (a rebound or unbound row offers one; a default row does not).
func (k *Keybindings) ResetVisibleForTest(action string) bool {
	for _, kr := range k.rows {
		if kr.row.Action == action && kr.reset != nil {
			return kr.reset.GetWidget().IsVisible()
		}
	}
	return false
}

// ScrollForTest returns the list's scroll viewport (the reachability pins
// measure row strips against it).
func (k *Keybindings) ScrollForTest() *widget.ScrollContainer { return k.scroll }
