package screens

import (
	"fmt"
	"image"
	"math"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// SettingsActions is the settings window's seam to its host (the game):
// the panel draws no state of its own, it reads and pokes these callbacks
// every Sync. The host owns the ladders, the persistence and the live
// apply (game.setUIScale / setFullscreen / setVsync / setRailCollapsed /
// setMasterVolume / setMusicVolume / setUIVolume / persistSettings /
// setShuffle / resetLayout / resetAllSettings); the window only renders
// and routes. All must be non-nil.
type SettingsActions struct {
	// UIScale returns the current user UI scale (the persisted setting);
	// UIScaleLadder is the six-rung ladder the notched slider rides and
	// SetUIScale applies one rung absolutely (the host persists it and
	// rebuilds the interface at the new scale).
	UIScale       func() float64
	UIScaleLadder []float64
	SetUIScale    func(rung float64)
	// Fullscreen reports the current window mode; SetFullscreen flips it.
	Fullscreen    func() bool
	SetFullscreen func(bool)
	// RailCollapsed reports the navigation rail's parked state;
	// SetRailCollapsed flips it (the host persists it).
	RailCollapsed    func() bool
	SetRailCollapsed func(bool)
	// CloseHint is the live chord that closes this window ("" while the
	// action is unbound): the footer's "· P closes" clause follows it, and
	// an unbound action drops the clause rather than printing a dash.
	CloseHint func() string
	// MasterVolume, MusicVolume and UIVolume are whole percents (0..100).
	// The Set callbacks apply a drag's value live WITHOUT writing the
	// sidecar; PersistSettings is the release/persist point, so one drag
	// writes once however many values it streamed. The host multiplies
	// master with each channel's own volume at the audio seams.
	MasterVolume    func() int
	SetMasterVolume func(v int)
	MusicVolume     func() int
	SetMusicVolume  func(v int)
	UIVolume        func() int
	SetUIVolume     func(v int)
	PersistSettings func()
	// Shuffle reports the shuffled-pool flag; SetShuffle flips it.
	Shuffle    func() bool
	SetShuffle func(bool)
	// ResetLayout clears the persisted window arrangement (sidecar) and
	// recomposes the built-in defaults.
	ResetLayout func()
	// ResetAll restores every setting to its default and re-applies live.
	ResetAll func()
}

// Settings is the settings window: a notched UI-scale slider, fullscreen
// and rail toggles, freeflow master/music/UI volume sliders, a shuffle
// toggle and the two reset buttons. It mounts like every other panel (Screen id
// "settings" — its sidecar key — key P) and rebuilds with the interface, so all
// of its displayed state is re-read from the actions in Sync — including
// right after a live scale change rebuilt it under itself (the replacement
// slider starts idle; the drag is over, never re-fired).
type Settings struct {
	th  theme.Theme
	ptr *widgets.Pointer
	act SettingsActions
	win *widget.Window

	scaleRow    *widget.Container // relaid when the value text changes
	scaleSlider *widgets.Slider
	scaleVal    *widget.Text
	masterRow   *widget.Container
	masterSlide *widgets.Slider
	masterVal   *widget.Text
	musicRow    *widget.Container
	musicSlide  *widgets.Slider
	musicVal    *widget.Text
	uiRow       *widget.Container
	uiSlide     *widgets.Slider
	uiVal       *widget.Text
	fsBtn       *widget.Button
	fsState     bool // last text written, to keep Sync a compare
	railBtn     *widget.Button
	railState   bool
	shufBtn     *widget.Button
	shufState   bool
	// resetAll is the two-step destructive button (the save manager's
	// Delete pattern): the first press swaps the label, the second acts.
	resetAll   *widget.Button
	confirmAll bool
	// hint is the footer line (the apply note plus the live close chord);
	// hintState is the text it last drew, to keep Sync a compare.
	hint      *widget.Text
	hintState string
}

// NewSettings builds the Settings window over the shared environment.
// Mount it on a Kit with Kit.AddWindow and call Sync every frame.
func NewSettings(d interface {
	theme.Theme
	Pointer() *widgets.Pointer
}, act SettingsActions) *Settings {
	p := d.Pal()
	s := &Settings{th: d, ptr: d.Pointer(), act: act}

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
	// newValue is one row's readout: the tabular contents face (mono
	// digits do not jitter while a handle moves) and the row relays out
	// when the text changes (setStepperValue), so a wider value never
	// draws under the slider.
	newValue := func(text string) *widget.Text {
		return widget.NewText(
			widget.TextOpts.Text(text, s.th.FaceMono(theme.StepSmall), theme.Solid(p.Accent)),
			widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				HorizontalPosition: widget.GridLayoutPositionEnd,
				VerticalPosition:   widget.GridLayoutPositionCenter,
			})),
		)
	}
	// newSliderRow is the settings row idiom for a numeric setting:
	// label, the slider (stretched across the row's middle column) and
	// the value at the right. The grid is what stretches the slider: the
	// house RowLayout only stretches on its cross axis.
	newSliderRow := func(label string, sl *widgets.Slider, val *widget.Text) *widget.Container {
		lbl := widget.NewText(
			widget.TextOpts.Text(label, s.th.Face(theme.StepSmall), theme.Solid(p.Text)),
			widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.GridLayoutData{
				VerticalPosition: widget.GridLayoutPositionCenter,
			})),
		)
		row := widget.NewContainer(
			widget.ContainerOpts.Layout(widget.NewGridLayout(
				widget.GridLayoutOpts.Columns(3),
				widget.GridLayoutOpts.Stretch([]bool{false, true, false}, []bool{false}),
				widget.GridLayoutOpts.Spacing(theme.Px(theme.ContentsPad), 0),
			)),
			// Full body width: the vertical RowLayout stretches its
			// children on the cross axis only when asked.
			widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
		)
		row.AddChild(lbl, sl, val)
		return row
	}

	// Row 1: the UI scale slider — the six-rung ladder, notched. The
	// host applies a rung absolutely and rebuilds the interface at it;
	// the replacement slider starts idle (the rebuild ends the drag).
	s.scaleVal = newValue(fmt.Sprintf("%.4g×", act.UIScale()))
	s.scaleSlider = widgets.NewSlider(d, widgets.SliderOpts{
		Mode:     widgets.SliderNotches,
		Notches:  append([]float64(nil), act.UIScaleLadder...),
		Value:    act.UIScale(),
		OnChange: act.SetUIScale,
	})
	s.scaleRow = newSliderRow("UI scale", s.scaleSlider, s.scaleVal)

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

	// Rows 4–6: audio. The three volumes are freeflow sliders (whole
	// percents, 1% steps): the drag applies live through Set*, the release
	// is the one persist point. Shuffle is a toggle, default on.
	s.masterVal = newValue(fmt.Sprintf("%d%%", act.MasterVolume()))
	s.masterSlide = widgets.NewSlider(d, widgets.SliderOpts{
		Mode: widgets.SliderFreeflow, Min: 0, Max: 100, Step: 1,
		Value:     float64(act.MasterVolume()),
		OnChange:  func(v float64) { act.SetMasterVolume(int(math.Round(v))) },
		OnRelease: func(float64) { act.PersistSettings() },
	})
	s.masterRow = newSliderRow("Master", s.masterSlide, s.masterVal)
	s.musicVal = newValue(fmt.Sprintf("%d%%", act.MusicVolume()))
	s.musicSlide = widgets.NewSlider(d, widgets.SliderOpts{
		Mode: widgets.SliderFreeflow, Min: 0, Max: 100, Step: 1,
		Value:     float64(act.MusicVolume()),
		OnChange:  func(v float64) { act.SetMusicVolume(int(math.Round(v))) },
		OnRelease: func(float64) { act.PersistSettings() },
	})
	s.musicRow = newSliderRow("Music", s.musicSlide, s.musicVal)
	s.uiVal = newValue(fmt.Sprintf("%d%%", act.UIVolume()))
	s.uiSlide = widgets.NewSlider(d, widgets.SliderOpts{
		Mode: widgets.SliderFreeflow, Min: 0, Max: 100, Step: 1,
		Value:     float64(act.UIVolume()),
		OnChange:  func(v float64) { act.SetUIVolume(int(math.Round(v))) },
		OnRelease: func(float64) { act.PersistSettings() },
	})
	s.uiRow = newSliderRow("UI", s.uiSlide, s.uiVal)
	s.shufBtn = newToggle(
		// Default state is shuffle on, the fresh-install default.
		"Shuffle: on",
		func() { act.SetShuffle(!act.Shuffle()) },
	)

	// Rows 8–9: the two resets. Reset all is destructive, so it arms on
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
	s.hintState = hintText(act.CloseHint())
	s.hint = widget.NewText(
		widget.TextOpts.Text(s.hintState, s.th.Face(theme.StepTiny), theme.Solid(p.Dim)),
	)

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.WindowBodyBox(p, theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200))),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(8)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	body.AddChild(s.scaleRow, s.fsBtn, s.railBtn, s.masterRow, s.musicRow, s.uiRow, s.shufBtn, resetBtn, s.resetAll, s.hint)

	title, _ := theme.NewTitleBar(d, "Settings", "")

	s.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, theme.Px(theme.TitleBarH)),
		// Content floor: title + nine 40 px rows + the hint line (the
		// frame does not auto-grow, so the fallback is exactly this tall),
		// and wide enough for a slider's full travel beside its label and
		// readout (the three-way grid leaves the middle column the rest).
		widget.WindowOpts.MinSize(theme.Px(380), theme.Px(508)),
		// Fallback only: the game re-centers it from placeDefaults like
		// the other overlays; the row declares the same rect
		// (SettingsRect) as its size.
		widget.WindowOpts.Location(pxRect(SettingsRect())),
	)
	return s
}

// SettingsRect is the settings window's landing rect in design units: the
// one number its Screen row's DefaultRect and its constructor share.
func SettingsRect() image.Rectangle { return image.Rect(450, 106, 830, 614) }

// Win returns the settings window for Kit.AddWindow.
func (s *Settings) Win() *widget.Window { return s.win }

// Sync polls the sliders, mirrors the host's values into idle
// handles and refreshes the readouts and toggle labels. Call it every
// frame while mounted.
//
// The sliders own their value while grabbed (the mirror is skipped), so a
// volume drag cannot be snapped back under the pointer between the live
// Set and the host's next read. A notched scale change rebuilds the whole
// interface inside Poll: the object running this Sync is the one being
// replaced, and the fresh panel starts with idle sliders — the old drag
// never fires another change.
func (s *Settings) Sync() {
	s.scaleSlider.Poll(s.ptr)
	s.masterSlide.Poll(s.ptr)
	s.musicSlide.Poll(s.ptr)
	s.uiSlide.Poll(s.ptr)
	if !s.scaleSlider.Grabbed() {
		s.scaleSlider.SetValue(s.act.UIScale())
	}
	if !s.masterSlide.Grabbed() {
		s.masterSlide.SetValue(float64(s.act.MasterVolume()))
	}
	if !s.musicSlide.Grabbed() {
		s.musicSlide.SetValue(float64(s.act.MusicVolume()))
	}
	if !s.uiSlide.Grabbed() {
		s.uiSlide.SetValue(float64(s.act.UIVolume()))
	}
	setStepperValue(s.scaleRow, s.scaleVal, fmt.Sprintf("%.4g×", s.scaleSlider.Value()))
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
	setStepperValue(s.masterRow, s.masterVal, fmt.Sprintf("%d%%", int(math.Round(s.masterSlide.Value()))))
	setStepperValue(s.musicRow, s.musicVal, fmt.Sprintf("%d%%", int(math.Round(s.musicSlide.Value()))))
	setStepperValue(s.uiRow, s.uiVal, fmt.Sprintf("%d%%", int(math.Round(s.uiSlide.Value()))))
	if on := s.act.Shuffle(); on != s.shufState {
		s.shufState = on
		if on {
			s.shufBtn.SetText("Shuffle: on")
		} else {
			s.shufBtn.SetText("Shuffle: off")
		}
	}
	if h := hintText(s.act.CloseHint()); h != s.hintState {
		s.hintState, s.hint.Label = h, h
	}
}

// hintText is the footer line: the apply note, plus the live close chord
// while the Close action has one.
func hintText(close string) string {
	if close == "" {
		return "Applies immediately"
	}
	return "Applies immediately · " + close + " closes"
}

// ScaleSliderForTest, MasterSliderForTest, MusicSliderForTest and
// UISliderForTest expose the four controls (geometry, value and Step) to
// surface tests — the Table.RowWidgetForTest precedent, never called by
// production code.
func (s *Settings) ScaleSliderForTest() *widgets.Slider  { return s.scaleSlider }
func (s *Settings) MasterSliderForTest() *widgets.Slider { return s.masterSlide }
func (s *Settings) MusicSliderForTest() *widgets.Slider  { return s.musicSlide }
func (s *Settings) UISliderForTest() *widgets.Slider     { return s.uiSlide }

// CloseHintForTest returns the footer line as drawn — the live close chord
// in place of the construction-time literal.
func (s *Settings) CloseHintForTest() string { return s.hint.Label }
