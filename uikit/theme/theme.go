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

// bordered builds a bordered nineslice with the border width scaled: a
// 1 design-unit hairline is 1 DIP, so it must thicken with the rest of
// the interface (at 2× it is 2 device px = 1 DIP, still a hairline).
func Bordered(body, edge color.NRGBA, w int) *eimage.NineSlice {
	return eimage.NewBorderedNineSliceColor(body, edge, Px(w))
}

// NewTheme builds a complete ebitenui widget theme for the EVE-dark look.
// Every widget theme entry is populated with nineslices generated from the
// palette; button states visibly differ (idle = cell fill, hover = accent
// tint, pressed = deeper accent, disabled = dimmed).
func NewTheme(p Palette, ts TypeScale) *widget.Theme {
	f := NewFaces(ts)

	buttonImage := &widget.ButtonImage{
		Idle:         Bordered(Solid(p.Cell), NRGBA(p.Border, 220), 1),
		Hover:        Bordered(NRGBA(p.Hover, 18), NRGBA(p.Accent, 200), 1),
		Pressed:      Bordered(NRGBA(p.Hover, 45), NRGBA(p.Border, 220), 1),
		PressedHover: Bordered(NRGBA(p.Hover, 45), NRGBA(p.Accent, 200), 1),
		Disabled:     Bordered(NRGBA(p.Cell, 140), NRGBA(p.Border, 110), 1),
	}
	buttonText := &widget.ButtonTextColor{
		Idle:     Solid(p.Text),
		Hover:    Solid(p.Text),
		Pressed:  Solid(p.Text),
		Disabled: NRGBA(p.Dim, 160),
	}
	scrollImage := &widget.ScrollContainerImage{
		Idle:     Bordered(Solid(p.BG), NRGBA(p.Border, 160), 1),
		Disabled: Bordered(NRGBA(p.Cell, 140), NRGBA(p.Border, 110), 1),
		Mask:     NineSlice(Solid(p.BG)),
	}
	slider := newSliderParams(p)

	th := &widget.Theme{
		DefaultFace:      facePtr(f.Base),
		DefaultTextColor: Solid(p.Text),

		ButtonTheme: &widget.ButtonParams{
			Image:        buttonImage,
			TextColor:    buttonText,
			TextFace:     facePtr(f.SmallBold), // buttons are emphasis text
			TextPadding:  &widget.Insets{Left: Px(8), Right: Px(8), Top: Px(4), Bottom: Px(4)},
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
					Left: Px(10), Right: Px(10), Top: Px(4), Bottom: Px(4),
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
			TextPadding:            &widget.Insets{Right: Px(6)},
			ScrollContainerPadding: widget.NewInsetsSimple(Px(4)),
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
		Image: &widget.ButtonImage{
			Idle:         Bordered(Solid(p.Cell), NRGBA(p.Border, 220), 1),
			Hover:        Bordered(NRGBA(p.Hover, 18), NRGBA(p.Accent, 200), 1),
			Pressed:      Bordered(NRGBA(p.Hover, 45), NRGBA(p.Border, 220), 1),
			PressedHover: Bordered(NRGBA(p.Hover, 45), NRGBA(p.Accent, 200), 1),
			Disabled:     Bordered(NRGBA(p.Cell, 140), NRGBA(p.Border, 110), 1),
		},
		TextColor: &widget.ButtonTextColor{
			Idle:     Solid(p.Text),
			Hover:    Solid(p.Text),
			Pressed:  Solid(p.Text),
			Disabled: NRGBA(p.Dim, 160),
		},
		TextFace:     facePtr(f.SmallBold),
		TextPadding:  &widget.Insets{Left: Px(8), Right: Px(8), Top: Px(4), Bottom: Px(4)},
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
		ScrollContainerPadding: widget.NewInsetsSimple(0),
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
		TrackPadding: widget.NewInsetsSimple(0),
		// ebitenui sizes the handle with round(FixedHandleSize) floored at
		// MinHandleSize (default 16) but POSITIONS it with FixedHandleSize
		// alone: at the bottom of the travel a 6 px position plus the 16 px
		// floor pushed the handle 10 px past the track — through the bottom
		// edge of any window whose scrollbar sat at its end (the log's tail
		// leaked 80 px past the frame that way). Pin both to the same value
		// so size and position math agree; 16 matches what has always been
		// rendered (the floor already won), so pixels do not change. The
		// handle is a visible length: it scales with the UI (Px).
		FixedHandleSize: constantutil.ConstantToPointer(Px(16)),
		MinHandleSize:   constantutil.ConstantToPointer(Px(16)),
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
