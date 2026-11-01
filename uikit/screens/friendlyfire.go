package screens

import (
	"image"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// friendlyFireMinW/friendlyFireMinH are the confirm dialog's design-px floor
// and fallback rect: the title bar, the question line, the two named rows
// and the button row fit with a ContentsPad of cushion.
const (
	friendlyFireMinW = 340
	friendlyFireMinH = 220
)

// FriendlyFireWin is the friendly-fire confirmation: one single-instance
// dialog naming the target hull and the aggressive module the player just
// tried to fire, with exactly one way forward ("Confirm foolishness") and
// two ways back (Cancel, the × close). It holds no state of its own beyond
// the pending command's labels: the sim owns the gate, so the window only
// asks and hands the confirmation back.
type FriendlyFireWin struct {
	win   *widget.Window
	title *theme.ChromeText

	target *widget.Text
	module *widget.Text

	confirm *widget.Button
	cancel  *widget.Button

	onConfirm func()
	onClose   func()
}

// NewFriendlyFireWin builds the closed confirm window over the given
// environment.
func NewFriendlyFireWin(env widgets.Seams) *FriendlyFireWin {
	p := env.Pal()
	f := &FriendlyFireWin{}

	label := func(s string) *widget.Text {
		return widget.NewText(
			widget.TextOpts.Text(s, env.Face(theme.StepTiny), theme.Solid(p.Dim)),
			widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
				Position: widget.RowLayoutPositionCenter,
			})),
		)
	}
	newBtn := func(text string, clicked func()) *widget.Button {
		return widget.NewButton(
			widget.ButtonOpts.TextLabel(text),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(theme.Px(40), theme.Px(40)),
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
			widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) { clicked() }),
		)
	}
	f.confirm = newBtn("Confirm foolishness", func() {
		if f.onConfirm != nil {
			f.onConfirm()
		}
	})
	f.cancel = newBtn("Cancel", f.Close)
	f.target = widget.NewText(
		widget.TextOpts.Text("", env.Face(theme.StepSmall), theme.Solid(p.Text)),
	)
	f.module = widget.NewText(
		widget.TextOpts.Text("", env.Face(theme.StepSmall), theme.Solid(p.Text)),
	)

	row := func(children ...widget.PreferredSizeLocateableWidget) *widget.Container {
		c := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.ContentsPad)),
		)))
		c.AddChild(children...)
		return c
	}

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.WindowBodyBox(p, theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200))),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.ContentsPad)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	question := widget.NewText(
		widget.TextOpts.Text("Fire on a friendly ship?", env.FaceBold(theme.StepSmall), theme.Solid(p.Warn)),
	)
	body.AddChild(question, row(label("Target"), f.target), row(label("Module"), f.module),
		row(f.confirm, f.cancel))

	bar, title := theme.NewTitleBar(env, "Friendly fire", "")
	f.title = title
	f.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(bar, theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(friendlyFireMinW), theme.Px(friendlyFireMinH)),
		// Fallback only: the kit centres it on the real viewport.
		widget.WindowOpts.Location(image.Rect(theme.Px(342), theme.Px(230),
			theme.Px(342+friendlyFireMinW), theme.Px(230+friendlyFireMinH))),
	)
	theme.ShowWidget(f.win.GetContainer().GetWidget(), false)
	return f
}

// Win returns the window for Kit.AddWindow / the host's hit test.
func (f *FriendlyFireWin) Win() *widget.Window { return f.win }

// Show targets the dialog on one activation attempt and reveals it; confirm
// is the command the primary button re-dispatches.
func (f *FriendlyFireWin) Show(module, target string, confirm func()) {
	f.module.Label = module
	f.target.Label = target
	f.onConfirm = confirm
	theme.ShowWidget(f.win.GetContainer().GetWidget(), true)
}

// Close closes the dialog: through the Kit it also unmounts; in a bare
// environment it just hides. It declines the attack (no command is sent).
func (f *FriendlyFireWin) Close() {
	if f.onClose != nil {
		f.onClose()
		return
	}
	f.hide()
}

// hide flips the container invisible (the kit's unmount path).
func (f *FriendlyFireWin) hide() { theme.ShowWidget(f.win.GetContainer().GetWidget(), false) }

// IsOpen reports whether the window is revealed.
func (f *FriendlyFireWin) IsOpen() bool {
	return f.win != nil && f.win.GetContainer().GetWidget().IsVisible()
}

// Confirm is the primary button (tests press it).
func (f *FriendlyFireWin) Confirm() *widget.Button { return f.confirm }

// Cancel is the decline button (tests press it).
func (f *FriendlyFireWin) Cancel() *widget.Button { return f.cancel }
