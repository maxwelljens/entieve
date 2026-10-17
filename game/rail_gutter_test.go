package game

import (
	"path/filepath"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	uikit "github.com/entieve/entieve/uikit/screens"
	"github.com/entieve/entieve/uikit/theme"
)

// TestPlacedDefaultRectsClearTheRailGutter: every window placeDefaults
// composes starts right of the navigation rail's gutter — uikit.RailInset
// through theme.Px — at both design sizes, at the identity scale and
// through the scale funnel (device 1.5×). The composed set covers the kit
// core windows, the Show Info window, every Screen row (floating,
// centred, bottom-left and the transient Esc menu alike) and the docked
// station column; the anchor floors that keep them clear live in
// anchorRect/centerRect (game/screens.go).
//
// Only DEFAULT placements are checked: a window the player dragged, or
// one restored from the layout sidecar, skips placeIfVirgin and is never
// a default again.
func TestPlacedDefaultRectsClearTheRailGutter(t *testing.T) {
	for _, tc := range []struct {
		name string
		dev  float64
	}{{"device 1", 1}, {"device 1.5", 1.5}} {
		t.Run(tc.name, func(t *testing.T) {
			resetScale(t)
			for _, size := range [][2]int{{1280, 720}, {1024, 700}} {
				g := newScaleGameAt(t, filepath.Join(t.TempDir(), "save.json"), tc.dev)
				// The production size path: Layout converts window px to
				// device px and runs placeDefaults on the change.
				g.Layout(size[0], size[1])

				cases := []struct {
					name string
					win  *widget.Window
				}{
					{"overview", g.kit.OverviewWindow()},
					{"selected item", g.kit.SelectedWindow()},
					{"show info", g.kit.InfoWindow()},
					{"station services", g.svcWin},
				}
				for _, s := range screens {
					cases = append(cases, struct {
						name string
						win  *widget.Window
					}{s.Title, g.screenWin[s.ID]})
				}

				floor := theme.Px(uikit.RailInset)
				for _, c := range cases {
					r := c.win.GetContainer().GetWidget().Rect
					if r.Dx() <= 0 || r.Dy() <= 0 {
						t.Errorf("%dx%d @%v: %s has an empty default rect %v",
							size[0], size[1], tc.dev, c.name, r)
						continue
					}
					if r.Min.X < floor {
						t.Errorf("%dx%d @%v: %s default rect = %v, left edge %d starts inside the rail gutter (%d = RailInset through Px)",
							size[0], size[1], tc.dev, c.name, r, r.Min.X, floor)
					}
				}
			}
		})
	}
}
