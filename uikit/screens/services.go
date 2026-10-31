package screens

import (
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// The station services row — the header of the contacts view, the one
// window docking opens. It is the docked station's face and its one
// navigation: the docked-at identity (station name · system) and Undock.
// Every window a station offers is already a row of the navigation rail
// (Inventory, Market, Fitting, Manufacturing, Refinery, …), so the row
// carries no window buttons: a second door to the same window is a second
// thing to keep in sync. The button emits its typed intent (Undock) on the
// ActionSink.
//
// The contacts panel this row heads lives beside it in contacts.go:
// the panel composes the row first, then the two sections, so the row is
// sized like the strip header it replaced and the docked column's content
// height — and therefore its clearance over the log — is unchanged.
const (
	// svcBtnW/svcBtnH are one icon button's design box: the strip
	// header's height over a square the 20 px glyph centres in, so a
	// services row is exactly as tall as the row it replaces.
	svcBtnW = 36
	svcBtnH = 40

	// svcGap is the horizontal gap between the identity and the buttons
	// (the furniture token: the cells get air without growing the row).
	svcGap = theme.SpMd
)

// svcButton is one button of the view: its handle name, its glyph and its
// tooltip title.
type svcButton struct {
	name  string
	icon  string
	label string // tooltip title
	cell  *widget.Container
	btn   *widget.Button
	glyph *theme.IconBox
}

// Services is the docked station's service row: Theme + ContentView +
// ActionSink, like every screen. Sync re-reads the docked station each
// frame (identity line, which service buttons show).
type Services struct {
	th theme.Theme
	cv widgets.ContentView
	as widgets.ActionSink

	// innerWidth is the body content width in DESIGN units — the host
	// passes its column width so the identity line ellipsizes to the
	// room the buttons leave instead of spilling over the window.
	innerWidth int

	row     *widget.Container
	name    *widget.Text
	btns    []svcButton
	tips    []widgets.RowTip
	label   string
	measure func(string) float64
}

// NewServices builds the view over the given seams. innerWidth is the
// hosting column's content width in design units (the board column the
// game composes this window into).
func NewServices(env widgets.Seams, innerWidth int) *Services {
	p := env.Pal()
	face := env.Face(theme.StepSmall)
	s := &Services{
		th:         env,
		cv:         env,
		as:         env,
		innerWidth: innerWidth,
		measure: func(str string) float64 {
			w, _ := text.Measure(str, *face, 0)
			return w
		},
	}
	s.name = widget.NewText(
		widget.TextOpts.Text("", face, theme.Solid(p.Text)),
		widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
	)
	// The row's one button: leave. Windows are the rail's. Icons are 20 px
	// (IconButton), the cell box keeps the strip header's 40 px row height.
	s.btns = []svcButton{{name: "undock", icon: "act_undock", label: "Undock"}}
	s.tips = make([]widgets.RowTip, len(s.btns))
	for i := range s.btns {
		b := &s.btns[i]
		b.btn = widget.NewButton(
			// The strip's quiet cell: the theme's cell chrome, unlit.
			widget.ButtonOpts.Image(theme.CellImage(p, false)),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
					StretchHorizontal: true,
					StretchVertical:   true,
				}),
				widget.WidgetOpts.MouseButtonClickedHandler(func(*widget.WidgetMouseButtonClickedEventArgs) { s.fire(b) }),
			),
		)
		// The glyph is anchored by the cell, not ridden through the
		// button's graphic slot: ebitenui lays that slot out as
		// [empty label][10 px fixed spacing][graphic], a fixed
		// right-offset that hung the glyph over the cell's right edge —
		// the same defect the rail had (TestSvcIconsInsideCells pins the
		// centring). The house icon box reports its real Px box to the
		// layout (a bare Graphic would report 50×50).
		b.glyph = theme.NewIconBox(theme.IconButton)
		b.glyph.Widget().GetWidget().LayoutData = widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionCenter,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
		}
		b.glyph.Set(b.icon, p.Text)
		b.cell = widget.NewContainer(
			widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
			widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(svcBtnW), theme.Px(svcBtnH))),
		)
		b.cell.AddChild(b.btn, b.glyph.Widget()) // glyph last: it draws over the chrome
	}

	s.row = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(svcGap)),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	centred := func(w widget.PreferredSizeLocateableWidget) {
		w.GetWidget().LayoutData = widget.RowLayoutData{Position: widget.RowLayoutPositionCenter}
	}
	centred(s.name)
	s.row.AddChild(s.name)
	for i := range s.btns {
		// The row carries the cell (square + anchored glyph); Button()
		// still hands tests and tooltips the button inside it.
		centred(s.btns[i].cell)
		s.row.AddChild(s.btns[i].cell)
	}
	s.Sync()
	return s
}

// Container returns the view's row for the host window body.
func (s *Services) Container() *widget.Container { return s.row }

// Button returns one button by handle name — "undock" (nil when there is
// no such button) — the handle tests click and captures pin.
func (s *Services) Button(name string) *widget.Button {
	for i := range s.btns {
		if s.btns[i].name == name {
			return s.btns[i].btn
		}
	}
	return nil
}

// Identity is the identity line currently shown ("Station · System").
func (s *Services) Identity() string { return s.label }

// Visible reports whether the named button shows right now.
func (s *Services) Visible(name string) bool {
	for i := range s.btns {
		if s.btns[i].name == name {
			return s.btns[i].btn.GetWidget().IsVisible()
		}
	}
	return false
}

// fire emits the leave button's intent.
func (s *Services) fire(*svcButton) { s.as.Do(widgets.Undock{}) }

// Sync re-reads the docked station's identity. The identity ellipsizes to
// the room the button leaves, so the row can never push the window past its
// composed column.
func (s *Services) Sync() {
	if s.cv == nil {
		return
	}
	info, docked := s.cv.DockedStation()
	if docked && info.Name != "" {
		label := info.Name
		if info.System != "" {
			label += " · " + info.System
		}
		s.label = label
	}
	// Room for the identity: content width minus the button and the gap
	// between them (the identity leads the row).
	room := theme.Px(s.innerWidth) - theme.Px(svcBtnW) - theme.Px(svcGap)
	label := widgets.Ellipsize(s.label, float64(room), s.measure)
	if s.name.Label != label {
		s.name.Label = label
	}
	// Static tooltips: built on first sync, never rebuilt (the model
	// depends only on the button's identity).
	for i := range s.btns {
		b := &s.btns[i]
		s.tips[i].Set(s.th, b.btn.GetWidget(), b.name, func() widgets.TooltipModel {
			return widgets.TooltipModel{
				Icon:  b.icon,
				Title: b.label,
				Hints: []string{hintFor(b)},
			}
		})
	}
}

// hintFor is one button's tooltip footer line.
func hintFor(b *svcButton) string {
	if b.name == "undock" {
		return "Click: undock from the station"
	}
	return "Click: toggle " + b.label
}
