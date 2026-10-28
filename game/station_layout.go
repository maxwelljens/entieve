package game

import (
	"image"

	uikit "github.com/entieve/entieve/uikit/screens"
)

// The station column's geometry is the board panel's (uikit/screens/
// stationboard.go): the panel builds its window there and answers the
// content floor the host fits. The names below are the Screen table's
// declared rect and the pins' spelling; stationRect is the one composition
// both use.
const (
	stationX    = uikit.StationX
	stationY    = uikit.StationY
	stationWinW = uikit.StationWinW
	stationMinH = uikit.StationMinH
)

// stationRect is the station board's docked rect: the left column, clear
// of the navigation rail's gutter, at the board's content height (never
// below stationMinH). It is the panel's own rule — the Screen row declares
// the same column and the host's fit composes it.
func stationRect(stationH int) image.Rectangle { return uikit.StationRect(stationH) }
