package game

import (
	"image"
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
					{"station services", g.host.Window(idServices)},
				}
				for _, s := range screens {
					cases = append(cases, struct {
						name string
						win  *widget.Window
					}{s.Title, g.host.Window(s.ID)})
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

// TestVirginPlacementTable pins the whole virgin placement table at both
// design viewports and both scales (device 1 and 1.5: the framebuffer
// grows with the scale, so the design layout is scale-invariant):
//
//   - every window placeDefaults composes — the kit's core windows, the
//     Show Info window, every Screen row and the docked services column —
//     starts fully on-screen and right of the navigation rail's gutter;
//   - the always-on boot set (overview, selected item, log) is pairwise
//     disjoint, so the screen a fresh save shows has no overlap;
//   - the Overview holds the viewport's upper-right corner and the
//     Selected Item window the top-left (the user's default: the table
//     the player reads first gets the corner), the log its bottom-left
//     pocket;
//   - the market's construction-time default rect (760×560) is on screen
//     and right of the rail gutter; it lies over the log pocket — the big
//     three-pane trade window wins that strip, and the log raises back on
//     a click.
//
// One deliberate limit, pinned by the fact it names:
//   - the market's default covers the bottom-center HUD pair
//     (status + module rack) at both sizes: no width ≥ the three panes
//     need stays out of both a centered pair's x range and the log
//     pocket, so the default keeps the body usable and the pair loses.
//     (The old right-anchored rack cleared the old square at 1280×720;
//     the centered pair cannot.)
//   - the log overlaps the HUD pair at the 1024×700 minimum, in space
//     only and pre-existing: the log's content floor (logMinW = 260
//     design px) is wider than the pocket left of the pair there (its
//     left edge is 234 while the log starts at 97), and raising the log
//     above the pair would put it under the docked station column
//     instead.
func TestVirginPlacementTable(t *testing.T) {
	for _, tc := range []struct {
		name string
		dev  float64
	}{{"device 1", 1}, {"device 1.5", 1.5}} {
		t.Run(tc.name, func(t *testing.T) {
			resetScale(t)
			for _, size := range [][2]int{{1280, 720}, {1024, 700}} {
				w, h := size[0], size[1]
				g := newScaleGameAt(t, filepath.Join(t.TempDir(), "save.json"), tc.dev)
				layoutPump(t, g, w, h)
				screen := image.Rect(0, 0, g.w, g.h)
				floor := theme.Px(uikit.RailInset)

				// Every composed default window: laid out, on screen,
				// clear of the rail's gutter.
				check := func(name string, r image.Rectangle) {
					t.Helper()
					if r.Dx() <= 0 || r.Dy() <= 0 {
						t.Errorf("%dx%d @%v: %s has an empty default rect %v", w, h, tc.dev, name, r)
						return
					}
					if !r.In(screen) {
						t.Errorf("%dx%d @%v: %s default %v is not fully on screen", w, h, tc.dev, name, r)
					}
					if r.Min.X < floor {
						t.Errorf("%dx%d @%v: %s default %v starts inside the rail gutter (%d)",
							w, h, tc.dev, name, r, floor)
					}
				}
				check("overview", g.kit.OverviewWindow().GetContainer().GetWidget().Rect)
				check("selected item", g.kit.SelectedWindow().GetContainer().GetWidget().Rect)
				check("show info", g.kit.InfoWindow().GetContainer().GetWidget().Rect)
				check("station services", g.host.Window(idServices).GetContainer().GetWidget().Rect)
				for _, s := range screens {
					check(s.Title, g.host.Window(s.ID).GetContainer().GetWidget().Rect)
				}

				// The boot set is pairwise disjoint…
				ov := g.kit.OverviewWindow().GetContainer().GetWidget().Rect
				sp := g.kit.SelectedWindow().GetContainer().GetWidget().Rect
				lg := bodyOf[*uikit.LogPanel](g.host, idLog).Win().GetContainer().GetWidget().Rect
				for _, p := range []struct {
					a, b   string
					ra, rb image.Rectangle
				}{
					{"overview", "selected item", ov, sp},
					{"overview", "log", ov, lg},
					{"selected item", "log", sp, lg},
				} {
					if p.ra.Overlaps(p.rb) {
						t.Errorf("%dx%d @%v: default %s %v overlaps %s %v",
							w, h, tc.dev, p.a, p.ra, p.b, p.rb)
					}
				}
				// …and each holds its corner of the user's default.
				if want := image.Rect(g.w-theme.Px(10)-ov.Dx(), theme.Px(8),
					g.w-theme.Px(10), theme.Px(8)+ov.Dy()); ov != want {
					t.Errorf("%dx%d @%v: overview %v, want the upper-right corner %v", w, h, tc.dev, ov, want)
				}
				if sp.Min != image.Pt(theme.Px(8+uikit.RailInset), theme.Px(8)) {
					t.Errorf("%dx%d @%v: selected item at %v, want the top-left corner (%d,%d)",
						w, h, tc.dev, sp.Min, theme.Px(8+uikit.RailInset), theme.Px(8))
				}
				if lg.Min.X != theme.Px(10+uikit.RailInset) || lg.Max.Y != g.h-theme.Px(10) {
					t.Errorf("%dx%d @%v: log %v, want the bottom-left pocket (x %d, bottom %d)",
						w, h, tc.dev, lg, theme.Px(10+uikit.RailInset), g.h-theme.Px(10))
				}
				// The corner windows must stay clear of the bottom chrome
				// they sit over (the overview's height is what clears the
				// rack at the minimum); the log's HUD-bar overlap at
				// 1024×700 is the documented exception above.
				for _, cr := range mountedRects(g) {
					if cr.name != "hud.1" {
						continue
					}
					for _, bw := range []struct {
						name string
						r    image.Rectangle
					}{{"overview", ov}, {"selected item", sp}, {"log", lg}} {
						if bw.name == "log" && cr.name == "hud.1" {
							continue // documented: the log's content floor cannot clear the pair
						}
						if bw.r.Overlaps(cr.rect) {
							t.Errorf("%dx%d @%v: default %s %v overlaps the %s %v",
								w, h, tc.dev, bw.name, bw.r, cr.name, cr.rect)
						}
					}
				}

				// The market's default rect: on screen and clear of the
				// rail gutter; it may lie over the log pocket (the big
				// three-pane trade window — the log raises back).
				mkt := uikit.NewMarket(g.env).Win().GetContainer().GetWidget().Rect
				check("market default", mkt)
			}
		})
	}
}
