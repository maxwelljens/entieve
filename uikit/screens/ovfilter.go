package screens

import (
	"image"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// The filter window's design-px floor and its fallback rect: the header and
// ten kind rows measure 285 px of body (TestOverviewFilterWindowFitsContent),
// so the floor carries a ContentsPad of cushion for scale rounding. The
// window opens at exactly that size; the edge grips only grow it.
const (
	ovFilterMinW = 280
	ovFilterMinH = 340
)

// ovFilterKinds is the filter window's row order: every kind the grid can
// list. KindPlayer is absent on purpose — the pilot's own hull carries no
// switch and is never hidden; the Ship switch governs KindShip alone (every
// other ship, NPC and player-flown alike).
var ovFilterKinds = []string{
	sim.KindShip, sim.KindWreck, sim.KindAsteroid, sim.KindBelt,
	sim.KindStation, sim.KindGate, sim.KindAnomaly, sim.KindStar,
	sim.KindPlanet, sim.KindMoon,
}

// ovFilterRow is one kind's two switches.
type ovFilterRow struct {
	kind string
	show *widgets.CheckBox
	col  *widgets.CheckBox
}

// OvFilterWin is the Overview's filter dialog: one row per overview kind
// with a Show and a Colour tick box. Its only door is the cog button in the
// overview's title bar; it never filters anything but the overview list, so
// the world, the Selected Item panel and target locks are untouched.
//
// The two switches read and write the overview's OvFilter through the
// callbacks (the panel owns no state of its own), and every toggle re-syncs
// the list through changed. Colour is forward-compatible: today only the
// player's and hostiles' Ship rows carry a tint, so switching it off for
// the other kinds is a no-op until those kinds gain one.
type OvFilterWin struct {
	ptr *widgets.Pointer
	win *widget.Window

	rows []ovFilterRow

	read    func(kind string) (show, colour bool)
	write   func(kind string, show, colour bool)
	changed func()
}

// NewOvFilterWin builds the closed filter window: read and write are the
// overview's filter state, changed is the re-sync (called after every
// toggle).
func NewOvFilterWin(env widgets.Seams, read func(kind string) (show, colour bool),
	write func(kind string, show, colour bool), changed func()) *OvFilterWin {
	p := env.Pal()
	f := &OvFilterWin{ptr: env.Pointer(), read: read, write: write, changed: changed}

	head := func(text string, pos widget.GridLayoutPosition) *widget.Text {
		return widget.NewText(
			widget.TextOpts.Text(text, env.Face(theme.StepTiny), theme.Solid(p.Dim)),
			widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				HorizontalPosition: pos,
				VerticalPosition:   widget.GridLayoutPositionCenter,
			})),
		)
	}

	// One grid for header and rows: per-row containers would each measure
	// their own columns and the switches would not line up down the window.
	grid := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(3),
			widget.GridLayoutOpts.Stretch([]bool{true, false, false}, nil),
			widget.GridLayoutOpts.Spacing(theme.Px(theme.ContentsPad), theme.Px(theme.SpSm)),
		)),
	)
	grid.AddChild(head("Kind", widget.GridLayoutPositionStart),
		head("Visible", widget.GridLayoutPositionCenter),
		head("Colour", widget.GridLayoutPositionCenter))

	for _, kind := range ovFilterKinds {
		row := ovFilterRow{kind: kind}
		row.show = widgets.NewCheckBox(env, widgets.CheckBoxOpts{
			Checked: true,
			OnChange: func(v bool) {
				f.write(row.kind, v, row.col.Checked())
				f.changed()
			},
		})
		row.col = widgets.NewCheckBox(env, widgets.CheckBoxOpts{
			Checked: true,
			OnChange: func(v bool) {
				f.write(row.kind, row.show.Checked(), v)
				f.changed()
			},
		})
		label := widget.NewText(
			widget.TextOpts.Text(theme.KindLabel(kind), env.Face(theme.StepSmall), theme.Solid(p.Text)),
			widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			})),
		)
		grid.AddChild(label)
		for _, cb := range []*widgets.CheckBox{row.show, row.col} {
			// A marker-only cell still needs a real hit box to aim at.
			cb.GetWidget().MinWidth = theme.Px(40)
			cb.GetWidget().LayoutData = widget.GridLayoutData{
				HorizontalPosition: widget.GridLayoutPositionCenter,
				VerticalPosition:   widget.GridLayoutPositionCenter,
			}
			grid.AddChild(cb)
		}
		f.rows = append(f.rows, row)
	}

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.WindowBodyBox(p, theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200))),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	body.AddChild(grid)

	// The title wears the same cog that opens it (manifest "cog", the key
	// the neutral kind glyph shares).
	bar, _ := theme.NewTitleBar(env, "Overview Filters", "kind_unknown")
	f.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(bar, theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(ovFilterMinW), theme.Px(ovFilterMinH)),
		// Fallback only: a transient dialog, never persisted (no place hook,
		// so a reopen does not jump). It lands clear of the overview's
		// default rect at the 1024×700 minimum.
		widget.WindowOpts.Location(image.Rect(
			theme.Px(495), theme.Px(8),
			theme.Px(495+ovFilterMinW), theme.Px(8+ovFilterMinH))),
	)
	theme.ShowWidget(f.win.GetContainer().GetWidget(), false)
	return f
}

// Win returns the window for Kit.AddWindow / the host's hit test.
func (f *OvFilterWin) Win() *widget.Window { return f.win }

// Show re-reads the switches from the overview's state and reveals the
// window (the cog's door — there is no key of its own).
func (f *OvFilterWin) Show() {
	for _, r := range f.rows {
		show, colour := f.read(r.kind)
		r.show.SetChecked(show)
		r.col.SetChecked(colour)
	}
	theme.ShowWidget(f.win.GetContainer().GetWidget(), true)
}

// hide flips the container invisible (the kit's unmount path).
func (f *OvFilterWin) hide() { theme.ShowWidget(f.win.GetContainer().GetWidget(), false) }

// IsOpen reports whether the window is revealed.
func (f *OvFilterWin) IsOpen() bool {
	return f.win != nil && f.win.GetContainer().GetWidget().IsVisible()
}

// Sync runs one frame of the window's input: every tick box polls the
// session's pointer frame. The kit calls it from Kit.Sync.
func (f *OvFilterWin) Sync() {
	if !f.IsOpen() {
		return
	}
	for _, r := range f.rows {
		r.show.Poll(f.ptr)
		r.col.Poll(f.ptr)
	}
}
