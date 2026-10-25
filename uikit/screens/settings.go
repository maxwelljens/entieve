package screens

import (
	"fmt"
	"image"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
)

// SettingsActions is the settings window's seam to its host (the game):
// the panel draws no state of its own, it reads and pokes these callbacks
// every Sync. The host owns the ladder, the persistence and the live
// apply (game.stepUIScale / setFullscreen / setVsync / setRailCollapsed /
// resetLayout / resetAllSettings); the window only renders and routes.
// All must be non-nil.
type SettingsActions struct {
	// UIScale returns the current user UI scale (the persisted setting).
	UIScale func() float64
	// StepScale moves the scale one rung along the host's ladder.
	StepScale func(delta int)
	// Fullscreen reports the current window mode; SetFullscreen flips it.
	Fullscreen    func() bool
	SetFullscreen func(bool)
	// RailCollapsed reports the navigation rail's parked state;
	// SetRailCollapsed flips it (the host persists it).
	RailCollapsed    func() bool
	SetRailCollapsed func(bool)
	// ResetLayout clears the persisted window arrangement (sidecar) and
	// recomposes the built-in defaults.
	ResetLayout func()
	// ResetAll restores every setting to its default and re-applies live.
	ResetAll func()
}

// Settings is the settings window: UI scale stepper, fullscreen and rail
// toggles and the two reset buttons. It mounts like every other panel
// (panelDefs, sidecar name "settings", key P) and rebuilds with the
// interface, so all of its displayed state is re-read from the actions in
// Sync — including right after a live scale change rebuilt it under
// itself.
type Settings struct {
	th  theme.Theme
	act SettingsActions
	win *widget.Window

	scaleRow  *widget.Container // relaid when the scale value text changes
	scaleVal  *widget.Text
	fsBtn     *widget.Button
	fsState   bool // last text written, to keep Sync a compare
	railBtn   *widget.Button
	railState bool
	// resetAll is the two-step destructive button (the save manager's
	// Delete pattern): the first press swaps the label, the second acts.
	resetAll   *widget.Button
	confirmAll bool
}

// NewSettings builds the Settings window over the shared environment.
// Mount it on a Kit with Kit.AddWindow and call Sync every frame.
func NewSettings(d theme.Theme, act SettingsActions) *Settings {
	p := d.Pal()
	s := &Settings{th: d, act: act}

	newBtn := func(label string, clicked func()) *widget.Button {
		return widget.NewButton(
			widget.ButtonOpts.TextLabel(label),
			widget.ButtonOpts.WidgetOpts(
				// ≥40×40 scaled hit area for every small control.
				widget.WidgetOpts.MinSize(theme.Px(40), theme.Px(40)),
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
			widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) { clicked() }),
		)
	}
	// Full-width toggle row: the label is the state readout.
	newToggle := func(label string, clicked func()) *widget.Button {
		return widget.NewButton(
			widget.ButtonOpts.TextLabel(label),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(0, theme.Px(40)),
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
			widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) { clicked() }),
		)
	}

	// Row 1: the scale stepper. The value is a dynamic number, so it
	// carries the tabular contents face (mono digits do not jitter while
	// stepping) and its row relays out on change (setStepperValue) —
	// otherwise a wider value would draw under the + button.
	s.scaleVal = widget.NewText(
		widget.TextOpts.Text("1×", s.th.FaceMono(theme.StepSmall), theme.Solid(p.Accent)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
			Position: widget.RowLayoutPositionCenter,
		})),
	)
	minus := newBtn("-", func() { act.StepScale(-1) })
	plus := newBtn("+", func() { act.StepScale(+1) })
	scaleLbl := widget.NewText(
		widget.TextOpts.Text("UI scale", s.th.Face(theme.StepSmall), theme.Solid(p.Text)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
			Position: widget.RowLayoutPositionCenter,
		})),
	)
	s.scaleRow = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.ContentsPad)),
		)),
	)
	s.scaleRow.AddChild(minus, scaleLbl, s.scaleVal, plus)

	// Rows 2–3: full-width toggles. The button's label is the state
	// readout, so it is rewritten whenever the state changes.
	s.fsBtn = newToggle(
		// The construction label matches the default state (off), so the
		// first Sync only rewrites it when the mode is actually on.
		"Fullscreen: off",
		func() { act.SetFullscreen(!act.Fullscreen()) },
	)
	s.railBtn = newToggle(
		// Default state is expanded, the rail's boot state.
		"Rail: expanded",
		func() { act.SetRailCollapsed(!act.RailCollapsed()) },
	)

	// Rows 4–5: the two resets. Reset all is destructive, so it arms on
	// the first press and fires on the second.
	resetBtn := newToggle("Reset window layout", func() { act.ResetLayout() })
	s.resetAll = newToggle("Reset all settings", func() {
		if !s.confirmAll {
			s.confirmAll = true
			s.resetAll.SetText("Confirm reset all")
			return
		}
		s.confirmAll = false
		s.resetAll.SetText("Reset all settings")
		act.ResetAll()
	})
	hint := widget.NewText(
		widget.TextOpts.Text("Applies immediately · P closes", s.th.Face(theme.StepTiny), theme.Solid(p.Dim)),
	)

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.WindowBodyBox(p, theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200))),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(8)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	body.AddChild(s.scaleRow, s.fsBtn, s.railBtn, resetBtn, s.resetAll, hint)

	title, _ := theme.NewTitleBar(d, "Settings", "")

	s.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, theme.Px(theme.TitleBarH)),
		// Content floor: title + five 40 px rows + the hint line (the
		// frame does not auto-grow, so the fallback is exactly this tall).
		widget.WindowOpts.MinSize(theme.Px(300), theme.Px(320)),
		// Fallback only: centred at the 1280×720 design size (the game
		// re-centers it from placeDefaults like the other overlays).
		widget.WindowOpts.Location(image.Rect(theme.Px(490), theme.Px(200), theme.Px(790), theme.Px(520))),
	)
	return s
}

// Win returns the settings window for Kit.AddWindow.
func (s *Settings) Win() *widget.Window { return s.win }

// Sync refreshes the scale value and the toggle labels from the host.
// Call it every frame while mounted.
func (s *Settings) Sync() {
	setStepperValue(s.scaleRow, s.scaleVal, fmt.Sprintf("%.4g×", s.act.UIScale()))
	if on := s.act.Fullscreen(); on != s.fsState {
		s.fsState = on
		if on {
			s.fsBtn.SetText("Fullscreen: on")
		} else {
			s.fsBtn.SetText("Fullscreen: off")
		}
	}
	if on := s.act.RailCollapsed(); on != s.railState {
		s.railState = on
		if on {
			s.railBtn.SetText("Rail: collapsed")
		} else {
			s.railBtn.SetText("Rail: expanded")
		}
	}
}
