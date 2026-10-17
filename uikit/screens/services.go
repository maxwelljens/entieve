package screens

import (
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/icons"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"slices"
)

// The station services view — the one window docking opens. It is the
// docked station's face: the docked-at identity (station name · system),
// an Undock button, a Cargo button and one button per service the
// station actually offers — buttons the station LACKS are hidden, never
// grayed, because an absent service is not offered at all. Every button
// emits a typed intent (Undock, Toggle{Screen}) on the ActionSink; the
// host maps the toggles onto its window table.
//
// The contract board the host composes under this row lives in game
// (contracts.go): the view is the row above it, sized like the strip
// header it replaces, so the docked column's content height — and
// therefore its clearance over the log — is unchanged.
const (
	// svcBtnW/svcBtnH are one icon button's design box: the rail's cell
	// width over the header height the strip's Undock button had, so a
	// services row is exactly as tall as the row it replaces.
	svcBtnW = 36
	svcBtnH = 40

	// svcGap is the horizontal gap between the identity and the buttons.
	svcGap = 4
)

// svcButton is one button of the view: its handle name, its glyph, the
// screen its Toggle names ("" = the undock intent instead) and whether
// the station must offer a service for it to appear ("" = always).
type svcButton struct {
	name    string
	icon    string
	screen  string
	service string
	label   string // tooltip title
	btn     *widget.Button
	visible bool
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
	// The row's buttons, in order: leave, cargo, then one per service.
	// The icon names match the Screen spec's own Icon per window, so the
	// services view and the rail draw the same glyph for the same
	// window. Icons are 20 px (IconButton), the cell box keeps the
	// strip header's 40 px row height.
	s.btns = []svcButton{
		{name: "undock", icon: "act_undock", label: "Undock"},
		{name: "cargo", icon: "svc_inventory", screen: "Inventory", label: "Cargo"},
		{name: "market", icon: "svc_market", screen: "Market", service: "market", label: "Market"},
		{name: "fitting", icon: "svc_fitting", screen: "Fitting", service: "fitting", label: "Fitting"},
		{name: "industry", icon: "row_module", screen: "Factory", service: "industry", label: "Industry"},
	}
	s.tips = make([]widgets.RowTip, len(s.btns))
	for i := range s.btns {
		b := &s.btns[i]
		b.btn = widget.NewButton(
			widget.ButtonOpts.Image(svcCellImages(p)),
			widget.ButtonOpts.Graphic(&widget.GraphicImage{
				Idle:     icons.Get(b.icon, theme.Px(theme.IconButton), p.Text),
				Disabled: icons.Get(b.icon, theme.Px(theme.IconButton), iconColor(p.Dim, theme.IconDisabledA)),
			}),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(theme.Px(svcBtnW), theme.Px(svcBtnH)),
				widget.WidgetOpts.MouseButtonClickedHandler(func(*widget.WidgetMouseButtonClickedEventArgs) { s.fire(b) }),
			),
		)
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
		centred(s.btns[i].btn)
		s.row.AddChild(s.btns[i].btn)
	}
	s.Sync()
	return s
}

// Container returns the view's row for the host window body.
func (s *Services) Container() *widget.Container { return s.row }

// Button returns one button by handle name — "undock", "cargo",
// "market", "fitting", "industry" (nil when there is no such button) —
// the handle tests click and captures pin.
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

// Visible reports whether the named button shows right now (the
// station's service set decides; "undock"/"cargo" always do).
func (s *Services) Visible(name string) bool {
	for i := range s.btns {
		if s.btns[i].name == name {
			return s.btns[i].visible
		}
	}
	return false
}

// fire emits one button's intent: Undock for the leave button, the
// screen's Toggle for everything else.
func (s *Services) fire(b *svcButton) {
	if b.name == "undock" {
		s.as.Do(widgets.Undock{})
		return
	}
	s.as.Do(widgets.Toggle{Screen: b.screen})
}

// Sync re-reads the docked station: the identity line, and which service
// buttons exist at THIS station (absent services hide, they are never
// grayed — a grayed button would offer something the station does not
// have). The identity ellipsizes to the room the visible buttons leave,
// so the row can never push the window past its composed column.
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
	visible := 0
	for i := range s.btns {
		b := &s.btns[i]
		show := b.service == "" || slices.Contains(info.Services, b.service)
		b.visible = show
		theme.ShowWidget(b.btn.GetWidget(), show)
		if show {
			visible++
		}
	}
	// Room for the identity: content width minus the visible buttons
	// and the gaps around them (the identity leads the row).
	children := 1 + visible
	room := theme.Px(s.innerWidth) - visible*theme.Px(svcBtnW) - (children-1)*theme.Px(svcGap)
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

// svcCellImages is the service button chrome — the strip's quiet
// bordered cells with the house hover/press states.
func svcCellImages(p theme.Palette) *widget.ButtonImage {
	return &widget.ButtonImage{
		Idle:         theme.Bordered(theme.NRGBA(p.BG, 210), theme.NRGBA(p.Border, 200), 1),
		Hover:        theme.Bordered(theme.NRGBA(p.Accent, 95), theme.Solid(p.Accent), 1),
		Pressed:      theme.Bordered(theme.NRGBA(p.Accent, 110), theme.NRGBA(p.Accent, 220), 1),
		PressedHover: theme.Bordered(theme.NRGBA(p.Accent, 110), theme.Solid(p.Accent), 1),
		Disabled:     theme.Bordered(theme.NRGBA(p.Cell, 140), theme.NRGBA(p.Border, 110), 1),
	}
}
