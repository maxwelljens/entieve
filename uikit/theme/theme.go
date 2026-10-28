package theme

import (
	"image/color"

	img "image"

	eimage "github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/utilities/constantutil"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/render"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// Palette holds every color the kit paints with. DefaultPalette mirrors the
// legacy ui package's EVE-dark chrome colors exactly; TestThemeMatchesLegacy
// pins the two together while ui/ still exists.
type Palette struct {
	BG     color.RGBA // near-black translucent panel fill
	BGHard color.RGBA // opaque variant for layering
	Border color.RGBA // thin panel borders
	Accent color.RGBA // amber accents / selection
	Text   color.RGBA // primary text
	Dim    color.RGBA // secondary text
	Head   color.RGBA // table headers
	Good   color.RGBA // positive / shield-ish green
	Warn   color.RGBA // warnings / armor amber
	Danger color.RGBA // hostile red
	Info   color.RGBA // info blue
	Shield color.RGBA // shield blue
	Struct color.RGBA // structure grey
	Cell   color.RGBA // module cell background
	Cap    color.RGBA // capacitor green
	Player color.RGBA // the player's own ship
	Hover  color.RGBA // hover tint (drawn translucent)
	HeadBg color.RGBA // table header strip
}

// DefaultPalette returns the EVE-dark palette used by the current game UI
// (hex values copied from ui/style.go).
func DefaultPalette() Palette {
	return Palette{
		BG:     mustHex("#0a0e14ee"),
		BGHard: mustHex("#0a0e14"),
		Border: mustHex("#3a4452"),
		Accent: mustHex("#ffb454"),
		Text:   mustHex("#d8dee9"),
		Dim:    mustHex("#7a8494"),
		Head:   mustHex("#5b677a"),
		Good:   mustHex("#5fcf80"),
		Warn:   mustHex("#e0a33c"),
		Danger: mustHex("#e05555"),
		Info:   mustHex("#8fb8e0"),
		Shield: mustHex("#6fa8ff"),
		Struct: mustHex("#c9ced6"),
		Cell:   mustHex("#141a24"),
		Cap:    mustHex("#5fcf80"),
		Player: mustHex("#7fe3ff"),
		Hover:  mustHex("#ffb454"),
		HeadBg: mustHex("#131926"),
	}
}

// OwnedRowAlpha is the alpha of the background tint a table row wears
// when it belongs to the player (the market book's own orders). It is
// the same 60 the overview's own-ship (Player) and hostile (Danger) rows
// already use: a row hint that leaves the dense mono figures readable.
// A selection's Accent tint (selectionAlpha 64) still wins over it.
const OwnedRowAlpha = 60

// OwnedRowTint is that tint for a row the player owns: the palette's
// Good green — the HUD's positive/success role — at OwnedRowAlpha.
func OwnedRowTint(p Palette) color.RGBA {
	c := p.Good
	c.A = OwnedRowAlpha
	return c
}

// RailGroupColor returns the navigation rail's glyph tint for one section:
// the palette role that section's windows serve. The names are the rail's
// six section names (game/rail.go railGroups, docs/GAMEPLAY.md); any other
// group falls back to the body text colour.
func RailGroupColor(p Palette, group string) color.RGBA {
	switch group {
	case "Ship":
		return p.Player // cyan — your ship
	case "Trade":
		return p.Good // green
	case "Industry":
		return p.Warn // orange
	case "Station":
		return p.Accent // gold
	case "World":
		return p.Info // blue
	case "Help & System":
		return p.Struct // silver
	}
	return p.Text
}

// mustHex parses a palette color; invalid constants would be a build bug.
func mustHex(s string) color.RGBA {
	c, err := render.ParseHex(s)
	if err != nil {
		panic("uikit: bad color constant " + s)
	}
	return c
}

// TypeScale holds the UI type sizes in pixels: Base is the default text
// size and Small/Large/Tiny are its 0.8×/1.3×/0.7× derivatives, matching
// ui.FontBase and friends.
type TypeScale struct {
	Base, Small, Large, Tiny float64
}

// DefaultTypeScale returns the current game type scale (20 px base —
// one pixel under the 21 px IBM Plex Sans base: IBM Plex Sans sets ~19 %
// taller at the same size, so the base gave a pixel back).
func DefaultTypeScale() TypeScale {
	const base = 20.0
	return TypeScale{
		Base:  base,
		Small: 0.8 * base,
		Large: 1.3 * base,
		Tiny:  0.7 * base,
	}
}

// kitFaces holds the shared IBM Plex Sans faces (render.Face/FaceBold
// cache them), boxed as text.Face for direct use with text.Measure and
// ebitenui widgets: the regular weight is body text, Bold is the
// emphasis weight (titles, headers, buttons, HUD readouts).
type kitFaces struct {
	Base, Small, Large, Tiny       text.Face
	BaseBold, SmallBold, LargeBold text.Face
}

func NewFaces(ts TypeScale) kitFaces {
	return kitFaces{
		Base:      render.Face(ts.Base),
		Small:     render.Face(ts.Small),
		Large:     render.Face(ts.Large),
		Tiny:      render.Face(ts.Tiny),
		BaseBold:  render.FaceBold(ts.Base),
		SmallBold: render.FaceBold(ts.Small),
		LargeBold: render.FaceBold(ts.Large),
	}
}

// facePtr boxes a face for ebitenui's pointer-to-interface parameters.
func facePtr(f text.Face) *text.Face { return &f }

// nrgba converts a palette color to a straight-alpha NRGBA with the given
// alpha — the color type ebitenui's nineslices are built from everywhere.
func NRGBA(c color.RGBA, a uint8) color.NRGBA {
	return color.NRGBA{R: c.R, G: c.G, B: c.B, A: a}
}

func Solid(c color.RGBA) color.NRGBA { return NRGBA(c, c.A) }

func NineSlice(c color.NRGBA) *eimage.NineSlice { return eimage.NewNineSliceColor(c) }

// Bordered builds the control chrome: a bordered nineslice with the
// control corner radius (5 design px) and the border width scaled — a 1
// design-unit hairline is 1 DIP, so it must thicken with the rest of the
// interface (at 2× it is 2 device px = 1 DIP, still a hairline).
// Panels, window pieces and square strips call Rounded/PanelBox/
// WindowBodyBox/WindowTitleBox/SquareBox for their own radii (round.go).
func Bordered(body, edge color.NRGBA, w int) *eimage.NineSlice {
	return Rounded(body, edge, w, AllRadii(RadiusControl))
}

// buttonState is one state of a five-state button image: the nineslice
// body and edge the chrome paints.
type buttonState struct{ body, edge color.NRGBA }

// houseStates is the house control recipe: cell fill idle, accent-tinted
// hover, deeper accent press, dimmed disabled.
func houseStates(p Palette) [5]buttonState {
	return [5]buttonState{
		{Solid(p.Cell), NRGBA(p.Border, 220)},
		{NRGBA(p.Hover, 18), NRGBA(p.Accent, 200)},
		{NRGBA(p.Hover, 45), NRGBA(p.Border, 220)},
		{NRGBA(p.Hover, 45), NRGBA(p.Accent, 200)},
		{NRGBA(p.Cell, 140), NRGBA(p.Border, 110)},
	}
}

// cellStates is the cell recipe: accent-tinted hover/press states and the
// house disabled over an idle that glows accent when highlighted (a running
// module, an open window, a fitted slot) and stays a quiet BG cell otherwise.
func cellStates(p Palette, highlighted bool) [5]buttonState {
	idle := buttonState{NRGBA(p.BG, 210), NRGBA(p.Border, 200)}
	if highlighted {
		idle = buttonState{NRGBA(p.Accent, 60), Solid(p.Accent)}
	}
	return [5]buttonState{
		idle,
		{NRGBA(p.Accent, 95), Solid(p.Accent)},
		{NRGBA(p.Accent, 110), NRGBA(p.Accent, 220)},
		{NRGBA(p.Accent, 110), Solid(p.Accent)},
		{NRGBA(p.Cell, 140), NRGBA(p.Border, 110)},
	}
}

// buttonImage turns the five state pairs into ebitenui's button chrome.
func buttonImage(s [5]buttonState) *widget.ButtonImage {
	slice := func(st buttonState) *eimage.NineSlice { return Bordered(st.body, st.edge, 1) }
	return &widget.ButtonImage{
		Idle:         slice(s[0]),
		Hover:        slice(s[1]),
		Pressed:      slice(s[2]),
		PressedHover: slice(s[3]),
		Disabled:     slice(s[4]),
	}
}

// HouseButton is the standard five-state control chrome (theme buttons, tabs,
// combo buttons and the table header's shared states).
func HouseButton(p Palette) *widget.ButtonImage { return buttonImage(houseStates(p)) }

// CellImage is the cell chrome shared by the rack, rail, services strip and
// fitting grid: highlight sets the accent-glow idle, else it is a quiet cell.
func CellImage(p Palette, highlighted bool) *widget.ButtonImage {
	return buttonImage(cellStates(p, highlighted))
}

// NewTheme builds a complete ebitenui widget theme for the EVE-dark look.
// Every widget theme entry is populated with nineslices generated from the
// palette; button states visibly differ (idle = cell fill, hover = accent
// tint, pressed = deeper accent, disabled = dimmed).
func NewTheme(p Palette, ts TypeScale) *widget.Theme {
	f := NewFaces(ts)

	buttonImage := HouseButton(p)
	buttonText := &widget.ButtonTextColor{
		Idle:     Solid(p.Text),
		Hover:    Solid(p.Text),
		Pressed:  Solid(p.Text),
		Disabled: NRGBA(p.Dim, 160),
	}
	scrollImage := &widget.ScrollContainerImage{
		Idle:     Bordered(WindowBody(p.BG), NRGBA(p.Border, 160), 1),
		Disabled: Bordered(NRGBA(p.Cell, 140), NRGBA(p.Border, 110), 1),
		Mask:     NineSlice(WindowBody(p.BG)),
	}
	slider := newSliderParams(p)

	th := &widget.Theme{
		DefaultFace:      facePtr(f.Base),
		DefaultTextColor: Solid(p.Text),

		ButtonTheme: &widget.ButtonParams{
			Image:        buttonImage,
			TextColor:    buttonText,
			TextFace:     facePtr(f.SmallBold), // buttons are emphasis text
			TextPadding:  &widget.Insets{Left: Px(SpMd), Right: Px(SpMd), Top: Px(SpSm), Bottom: Px(SpSm)},
			TextPosition: &widget.TextPositioning{HTextPosition: widget.TextPositionCenter, VTextPosition: widget.TextPositionCenter},
		},

		PanelTheme: &widget.PanelParams{
			BackgroundImage: Bordered(Solid(p.BG), NRGBA(p.Border, 200), 1),
		},

		LabelTheme: &widget.LabelParams{
			Face: facePtr(f.Base),
			Color: &widget.LabelColor{
				Idle:     Solid(p.Text),
				Disabled: NRGBA(p.Dim, 160),
			},
		},

		TextTheme: &widget.TextParams{
			Face:  facePtr(f.Base),
			Color: Solid(p.Text),
		},

		CheckboxTheme: &widget.CheckboxParams{
			Label: &widget.LabelParams{
				Face: facePtr(f.Small),
				Color: &widget.LabelColor{
					Idle:     Solid(p.Text),
					Disabled: NRGBA(p.Dim, 160),
				},
			},
			Image: newCheckboxImages(p),
		},

		ProgressBarTheme: &widget.ProgressBarParams{
			TrackPadding: widget.NewInsetsSimple(Px(2)),
			TrackImage: &widget.ProgressBarImage{
				Idle:     Bordered(Solid(p.Cell), NRGBA(p.Border, 160), 1),
				Hover:    Bordered(Solid(p.Cell), NRGBA(p.Accent, 160), 1),
				Disabled: Bordered(NRGBA(p.Cell, 140), NRGBA(p.Border, 110), 1),
			},
			FillImage: &widget.ProgressBarImage{
				Idle:     NineSlice(Solid(p.Accent)),
				Hover:    NineSlice(Solid(p.Accent)),
				Disabled: NineSlice(NRGBA(p.Accent, 140)),
			},
		},

		SliderTheme: slider,

		TabbookTheme: &widget.TabBookParams{
			TabButton: &widget.ButtonParams{
				Image:     buttonImage,
				TextColor: buttonText,
				TextFace:  facePtr(f.SmallBold),
				TextPadding: &widget.Insets{
					Left: Px(SpMd), Right: Px(SpMd), Top: Px(SpSm), Bottom: Px(SpSm),
				},
				TextPosition: &widget.TextPositioning{
					HTextPosition: widget.TextPositionCenter,
					VTextPosition: widget.TextPositionCenter,
				},
				MinSize: &img.Point{X: Px(98), Y: Px(40)},
			},
			TabSpacing: constantutil.ConstantToPointer(Px(1)),
		},

		TabTheme: &widget.TabParams{
			BackgroundImage: NineSlice(Solid(p.BGHard)),
		},

		TextInputTheme: &widget.TextInputParams{
			Face: facePtr(f.Base),
			Image: &widget.TextInputImage{
				Idle:     Bordered(Solid(p.Cell), NRGBA(p.Border, 200), 1),
				Disabled: Bordered(NRGBA(p.Cell, 140), NRGBA(p.Border, 110), 1),
			},
			Color: &widget.TextInputColor{
				Idle:          Solid(p.Text),
				Disabled:      NRGBA(p.Dim, 160),
				Caret:         Solid(p.Accent),
				DisabledCaret: NRGBA(p.Dim, 160),
			},
			Padding: widget.NewInsetsSimple(Px(5)),
		},

		TextAreaTheme: &widget.TextAreaParams{
			Face:                   facePtr(f.Base),
			ForegroundColor:        Solid(p.Text),
			TextPadding:            &widget.Insets{Right: Px(SpMd)},
			ScrollContainerPadding: widget.NewInsetsSimple(Px(SpSm)),
			ScrollContainerImage:   scrollImage,
			Slider:                 slider,
		},

		ListTheme: newListParams(p, f),

		ListComboButtonTheme: &widget.ListComboButtonParams{
			List:             newListParams(p, f),
			Button:           newButtonParams(p, f),
			MaxContentHeight: constantutil.ConstantToPointer(Px(200)),
		},
	}
	return th
}

// newButtonParams builds a standalone ButtonParams (for ListComboButton).
func newButtonParams(p Palette, f kitFaces) *widget.ButtonParams {
	return &widget.ButtonParams{
		Image: HouseButton(p),
		TextColor: &widget.ButtonTextColor{
			Idle:     Solid(p.Text),
			Hover:    Solid(p.Text),
			Pressed:  Solid(p.Text),
			Disabled: NRGBA(p.Dim, 160),
		},
		TextFace:     facePtr(f.SmallBold),
		TextPadding:  &widget.Insets{Left: Px(SpMd), Right: Px(SpMd), Top: Px(SpSm), Bottom: Px(SpSm)},
		TextPosition: &widget.TextPositioning{HTextPosition: widget.TextPositionCenter, VTextPosition: widget.TextPositionCenter},
	}
}

// newListParams builds the list styling shared by ListTheme and
// ListComboButtonTheme. EntryColor sets every field: the list derives its
// hover/press/selection images from them unconditionally.
func newListParams(p Palette, f kitFaces) *widget.ListParams {
	return &widget.ListParams{
		EntryFace:                   facePtr(f.Small),
		EntryTextPadding:            widget.NewInsetsSimple(Px(5)),
		EntryTextHorizontalPosition: constantutil.ConstantToPointer(widget.TextPositionStart),
		EntryTextVerticalPosition:   constantutil.ConstantToPointer(widget.TextPositionCenter),
		EntryColor: &widget.ListEntryColor{
			Unselected:         Solid(p.Text),
			Selected:           Solid(p.Text),
			DisabledUnselected: NRGBA(p.Dim, 160),
			DisabledSelected:   NRGBA(p.Dim, 160),

			// Selected: amber accent behind the row (matches ui's overview).
			SelectedBackground:        NRGBA(p.Accent, 36),
			SelectedFocusedBackground: NRGBA(p.Accent, 64),

			// Hover: subtle accent tint; press: a little deeper.
			FocusedBackground:          NRGBA(p.Accent, 22),
			SelectingBackground:        NRGBA(p.Accent, 44),
			SelectingFocusedBackground: NRGBA(p.Accent, 64),
			DisabledSelectedBackground: NRGBA(p.Cell, 140),
		},
		ScrollContainerPadding: widget.NewInsetsSimple(Px(SpSm)),
		ScrollContainerImage: &widget.ScrollContainerImage{
			Idle:     NineSlice(Solid(p.BG)),
			Disabled: NineSlice(NRGBA(p.BG, 140)),
			Mask:     NineSlice(Solid(p.BG)),
		},
		Slider: newSliderParams(p),
	}
}

// newSliderParams builds the scrollbar/slider chrome.
func newSliderParams(p Palette) *widget.SliderParams {
	handle := &widget.ButtonImage{
		Idle:    Bordered(Solid(p.Cell), NRGBA(p.Border, 220), 1),
		Hover:   Bordered(Solid(p.Head), NRGBA(p.Accent, 220), 1),
		Pressed: Bordered(Solid(p.Head), NRGBA(p.Accent, 220), 1),
	}
	return &widget.SliderParams{
		// The track keeps SpSm of air around the handle: the handle's
		// preferred width is then handle + 2×SpSm — exactly
		// ScrollGutter — so a rail authored at that width can never be
		// overhung by its own handle.
		TrackPadding: widget.NewInsetsSimple(Px(SpSm)),
		// ebitenui sizes the handle with round(FixedHandleSize) floored at
		// MinHandleSize (default 16) but POSITIONS it with FixedHandleSize
		// alone: at the bottom of the travel a 6 px position plus the 16 px
		// floor pushed the handle 10 px past the track — through the bottom
		// edge of any window whose scrollbar sat at its end (the log's tail
		// leaked 80 px past the frame that way). Pin both to the same value
		// so size and position math agree; SpLg matches what has always
		// been rendered (the floor already won), so pixels do not change.
		// The handle is a visible length: it scales with the UI (Px).
		FixedHandleSize: constantutil.ConstantToPointer(Px(SpLg)),
		MinHandleSize:   constantutil.ConstantToPointer(Px(SpLg)),
		TrackOffset:     constantutil.ConstantToPointer(0),
		PageSizeFunc:    func() int { return 1 },
		TrackImage: &widget.SliderTrackImage{
			Idle:     Bordered(Solid(p.Cell), NRGBA(p.Border, 160), 1),
			Hover:    Bordered(Solid(p.Cell), NRGBA(p.Accent, 160), 1),
			Disabled: Bordered(NRGBA(p.Cell, 140), NRGBA(p.Border, 110), 1),
		},
		HandleImage: handle,
	}
}

// newCheckboxImages builds the checkbox states: an empty bordered box, an
// amber-filled box when checked, and a dimmed box when disabled.
func newCheckboxImages(p Palette) *widget.CheckboxImage {
	unchecked := Bordered(Solid(p.Cell), NRGBA(p.Border, 255), 2)
	checked := Bordered(Solid(p.Accent), NRGBA(p.Border, 255), 2)
	greyed := Bordered(NRGBA(p.Cell, 160), NRGBA(p.Border, 140), 2)
	return &widget.CheckboxImage{
		Unchecked:         unchecked,
		Checked:           checked,
		Greyed:            greyed,
		UncheckedHovered:  unchecked,
		CheckedHovered:    checked,
		GreyedHovered:     greyed,
		UncheckedDisabled: unchecked,
		CheckedDisabled:   checked,
		GreyedDisabled:    greyed,
	}
}
