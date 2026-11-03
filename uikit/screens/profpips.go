package screens

import (
	"image/color"

	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// profPipColor is one level pip's fill: a pip past the level is the border
// role at 200, a filled one the accent solid, and a filled pip on a shut
// branch the accent at 120: a banked level behind a closed gate, or a
// required level the pilot does not meet yet (Show Info's row).
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
// level, filled up to the given level, step apart. The tree's node and the
// proficiency card's level line pass the pilot's level, and Show Info's
// PROFICIENCY row passes the def's required level; one painter means a level
// cannot read two ways on one surface set.
func drawProfPips(dst *ebiten.Image, x, y, level, max, step, size int, locked bool, pal theme.Palette) {
	for k := 0; k < max; k++ {
		col := profPipColor(k, level, locked, pal)
		vector.DrawFilledRect(dst, float32(x+k*step), float32(y), float32(size), float32(size), col, false)
	}
}

// profPipRowW is the width of a full pips row: max boxes, gaps between only.
func profPipRowW(max, step, size int) int { return max*step - (step - size) }
