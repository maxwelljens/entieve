package game

import (
	"image"

	"github.com/entieve/entieve/uikit/theme"
)

// stationRect is the station services view's docked rect: the left column,
// clear of the navigation rail's gutter, at the contract board's content
// height (never below stationMinH). It is the single source of truth for
// the column — placeDefaults places the services row there (it is an
// ordinary floating window now) and the board hug re-places it as the
// board grows (contracts.go), so the two can never disagree.
func stationRect(stationH int) image.Rectangle {
	return image.Rect(theme.Px(stationX), theme.Px(stationY),
		theme.Px(stationX+stationWinW), theme.Px(stationY)+max(stationH, theme.Px(stationMinH)))
}
