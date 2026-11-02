package screens

import (
	"image/color"

	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// profPipColor is one level pip's fill: an empty pip is the border role at
// 200, an earned one the accent solid, and an earned pip on a shut branch
// (a locked family) the accent at 120, because the practice is banked but
// the branch stays closed.
func profPipColor(k, level int, locked bool, pal theme.Palette) color.Color {
	switch {
	case k >= level:
		return theme.NRGBA(pal.Border, 200)
	case !locked:
		return theme.Solid(pal.Accent)
	default:
		return theme.NRGBA(pal.Accent, 120)
	}
}

// drawProfPips paints the level pips from (x, y): one box per reachable
// level, filled up to the granted level, step apart. The tree's node, the
// family card's level line and Show Info's PROFICIENCY level row all read
// this painter, so one family's pips cannot say two things in two places.
func drawProfPips(dst *ebiten.Image, x, y, level, max, step, size int, locked bool, pal theme.Palette) {
	for k := 0; k < max; k++ {
		col := profPipColor(k, level, locked, pal)
		vector.DrawFilledRect(dst, float32(x+k*step), float32(y), float32(size), float32(size), col, false)
	}
}

// profPipRowW is the width of a full pips row: max boxes, gaps between only.
func profPipRowW(max, step, size int) int { return max*step - (step - size) }
