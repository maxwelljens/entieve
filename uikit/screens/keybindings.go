package screens

import (
	"image"

	eimage "github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// KeybindRow is one rebindable action as the keybindings window draws it:
// the action's stable id, its label, the current key text ("" = unbound)
// and — while the row is armed for capture — the waiting/refusal notice.
type KeybindRow struct {
	Action string
	Label  string
	Key    string
	Armed  bool
	Notice string
}

// KeybindActions is the keybindings window's seam to its host (the game):
// the window owns no binding state, it renders the host's rows and routes
// the two verbs. Rows is re-read every Sync; Begin arms one action's
// capture, Reset clears the player's overrides. Both must be non-nil.
type KeybindActions struct {
	Rows  func() []KeybindRow
	Begin func(action string)
	Reset func()
}

// Keybindings is the rebinding window: one clickable row per rebindable
// action, showing its current key. Clicking a row arms it ("press a
// key…"); the host's key pass assigns the next key, Esc cancels. The
// Reset all control is two-step, the house destructive-button pattern.
// Like every panel it is rebuilt with the interface, so all displayed
// state is re-read from the actions in Sync.
type Keybindings struct {
	act KeybindActions
	win *widget.Window

	scroll *widget.ScrollContainer
	rail   *widgets.ScrollRail
	rows   []*widget.Button
	text   []string // last label written, to keep Sync a compare
	reset  *widget.Button
	armed  bool
}

// keybindScrollStep is one wheel notch, in design px (the title windows'
// step).
const keybindScrollStep = 60

// NewKeybindings builds the Keybindings window over the shared
// environment. Mount it on a Kit with Kit.AddWindow and call Sync every
// frame.
func NewKeybindings(d widgets.Seams, act KeybindActions) *Keybindings {
	p := d.Pal()
	k := &Keybindings{act: act}

	page := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(4)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	page.AddChild(widget.NewText(
		widget.TextOpts.Text("Click a row, then press a key · Esc cancels (reserved)", d.Face(theme.StepTiny), theme.Solid(p.Dim)),
	))
	// Reset all is destructive: it arms on the first press and fires on
	// the second (the Settings / save-manager pattern).
	k.reset = widget.NewButton(
		widget.ButtonOpts.TextLabel("Reset all bindings"),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(0, theme.Px(30)),
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
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
	page.AddChild(k.reset)

	rows := act.Rows()
	k.rows = make([]*widget.Button, 0, len(rows))
	k.text = make([]string, len(rows))
	for i, r := range rows {
		action := r.Action
		txt := keybindText(r)
		k.text[i] = txt
		btn := widget.NewButton(
			widget.ButtonOpts.TextLabel(txt),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(0, theme.Px(26)),
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
			widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) { act.Begin(action) }),
		)
		btn.GetWidget().SetTheme(d.WidgetTheme())
		k.rows = append(k.rows, btn)
		page.AddChild(btn)
	}

	k.scroll = widget.NewScrollContainer(
		widget.ScrollContainerOpts.Content(page),
		widget.ScrollContainerOpts.StretchContentWidth(),
		widget.ScrollContainerOpts.Image(&widget.ScrollContainerImage{
			Idle: theme.Bordered(theme.WindowBody(p.BG), theme.NRGBA(p.Border, 160), 1),
			Mask: eimage.NewNineSliceColor(theme.WindowBody(p.BG)),
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
		k.scroll.ScrollTop = max(0, min(1, k.scroll.ScrollTop-ev.Y*float64(theme.Px(keybindScrollStep))/float64(over)))
	})
	widgets.AttachScrollContent(k.scroll, page)
	k.rail = widgets.NewScrollRail(d, k.scroll, k.scroll, page)

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.WindowBodyBox(p, theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200))),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	body.AddChild(k.rail.Container())

	title, _ := theme.NewTitleBar(d, "Keybindings", "")

	k.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, theme.Px(theme.TitleBarH)),
		// Content floor: the window keeps this size; the list scrolls.
		widget.WindowOpts.MinSize(theme.Px(480), theme.Px(650)),
		// Fallback only: centred at the 1280×720 design size (the game
		// re-centres it from placeDefaults like the other overlays).
		widget.WindowOpts.Location(image.Rect(theme.Px(400), theme.Px(40), theme.Px(880), theme.Px(690))),
	)
	return k
}

// Win returns the keybindings window for Kit.AddWindow.
func (k *Keybindings) Win() *widget.Window { return k.win }

// Sync re-reads the rows and rewrites only the labels that changed.
func (k *Keybindings) Sync() {
	rows := k.act.Rows()
	for i := range k.rows {
		if i >= len(rows) {
			break
		}
		if txt := keybindText(rows[i]); txt != k.text[i] {
			k.text[i] = txt
			k.rows[i].SetText(txt)
		}
	}
	k.rail.Sync()
}

// keybindText is one row's button label: the action and its live key,
// the capture prompt, or the refusal notice while capture stays armed.
func keybindText(r KeybindRow) string {
	switch {
	case r.Armed && r.Notice != "":
		return r.Label + " — " + r.Notice
	case r.Armed:
		return r.Label + " — press a key…"
	case r.Key == "":
		return r.Label + " — unbound"
	default:
		return r.Label + "  ·  " + r.Key
	}
}
