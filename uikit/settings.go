package uikit

import (
	"fmt"
	"image"

	"github.com/ebitenui/ebitenui/widget"
)

// SettingsActions is the settings window's seam to its host (the game):
// the panel draws no state of its own, it reads and pokes these callbacks
// every Sync. The host owns the ladder, the persistence and the live
// apply (game.stepUIScale / setFullscreen / resetLayout); the window only
// renders and routes. All five must be non-nil.
type SettingsActions struct {
	// UIScale returns the current user UI scale (the persisted setting).
	UIScale func() float64
	// StepScale moves the scale one rung along the host's ladder.
	StepScale func(delta int)
	// Fullscreen reports the current window mode; SetFullscreen flips it.
	Fullscreen    func() bool
	SetFullscreen func(bool)
	// ResetLayout clears the persisted window arrangement (sidecar) and
	// recomposes the built-in defaults.
	ResetLayout func()
}

// Settings is the settings window: UI scale stepper, fullscreen toggle
// and reset-layout button. It mounts like every other panel (panelDefs,
// sidecar name "settings", key P) and rebuilds with the interface, so all
// of its displayed state is re-read from the actions in Sync — including
// right after a live scale change rebuilt it under itself.
type Settings struct {
	th  Theme
	act SettingsActions
	win *widget.Window

	scaleRow *widget.Container // relaid when the scale value text changes
	scaleVal *widget.Text
	fsBtn    *widget.Button
	fsState  bool // last text written, to keep Sync a compare
}

// NewSettings builds the Settings window over the shared environment.
// Mount it on a Kit with Kit.AddWindow and call Sync every frame.
func NewSettings(d Theme, act SettingsActions) *Settings {
	p := d.Pal()
	s := &Settings{th: d, act: act}

	newBtn := func(label string, clicked func()) *widget.Button {
		return widget.NewButton(
			widget.ButtonOpts.TextLabel(label),
			widget.ButtonOpts.WidgetOpts(
				// ≥40×40 scaled hit area for every small control.
				widget.WidgetOpts.MinSize(Px(40), Px(40)),
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
		widget.TextOpts.Text("1×", s.th.FaceMono(StepSmall), solid(p.Accent)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
			Position: widget.RowLayoutPositionCenter,
		})),
	)
	minus := newBtn("-", func() { act.StepScale(-1) })
	plus := newBtn("+", func() { act.StepScale(+1) })
	scaleLbl := widget.NewText(
		widget.TextOpts.Text("UI scale", s.th.Face(StepSmall), solid(p.Text)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
			Position: widget.RowLayoutPositionCenter,
		})),
	)
	s.scaleRow = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(Px(contentsPad)),
		)),
	)
	s.scaleRow.AddChild(minus, scaleLbl, s.scaleVal, plus)

	// Rows 2–3: full-width toggles. The fullscreen button's label is the
	// state readout, so it is rewritten whenever the mode changes.
	s.fsBtn = widget.NewButton(
		// The construction label matches the default state (off), so the
		// first Sync only rewrites it when the mode is actually on.
		widget.ButtonOpts.TextLabel("Fullscreen: off"),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(0, Px(40)),
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
		widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) {
			act.SetFullscreen(!act.Fullscreen())
		}),
	)
	resetBtn := widget.NewButton(
		widget.ButtonOpts.TextLabel("Reset window layout"),
		widget.ButtonOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(0, Px(40)),
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
		widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) { act.ResetLayout() }),
	)
	hint := widget.NewText(
		widget.TextOpts.Text("Applies immediately · P closes", s.th.Face(StepTiny), solid(p.Dim)),
	)

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BG), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(Px(8)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(Px(contentsPad))),
		)),
	)
	body.AddChild(s.scaleRow, s.fsBtn, resetBtn, hint)

	title, _ := NewTitleBar(d, "Settings", "")

	s.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, Px(titleBarH)),
		widget.WindowOpts.Draggable(),
		// Content floor: title + three 40 px rows + the hint line (the
		// frame does not auto-grow, so the fallback is exactly this tall).
		widget.WindowOpts.MinSize(Px(300), Px(224)),
		// Fallback only: centred at the 1280×720 design size (the game
		// re-centers it from placeDefaults like the other overlays).
		widget.WindowOpts.Location(image.Rect(Px(490), Px(248), Px(790), Px(472))),
	)
	return s
}

// Win returns the settings window for Kit.AddWindow.
func (s *Settings) Win() *widget.Window { return s.win }

// Sync refreshes the scale value and the fullscreen label from the host.
// Call it every frame while mounted.
func (s *Settings) Sync() {
	setStepperValue(s.scaleRow, s.scaleVal, fmt.Sprintf("%.4g×", s.act.UIScale()))
	on := s.act.Fullscreen()
	if on != s.fsState {
		s.fsState = on
		if on {
			s.fsBtn.SetText("Fullscreen: on")
		} else {
			s.fsBtn.SetText("Fullscreen: off")
		}
	}
}
