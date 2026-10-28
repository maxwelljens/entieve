package screens

import (
	"image"

	"github.com/entieve/entieve/uikit/theme"
)

// pxRect scales a panel's landing rect (designed in units, the rect the
// game's Screen row declares) into live px for its constructor.
func pxRect(r image.Rectangle) image.Rectangle {
	return image.Rect(theme.Px(r.Min.X), theme.Px(r.Min.Y), theme.Px(r.Max.X), theme.Px(r.Max.Y))
}
