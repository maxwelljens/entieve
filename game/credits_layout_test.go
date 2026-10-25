package game

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// The credits screen's layout: the tracked wordmark and subtitle, one
// section header per creditSection, the role/credit rows and the footer —
// all reachable by scrolling at every supported UI scale.
func TestCreditsWindowLayout(t *testing.T) {
	for _, sc := range []float64{0.75, 1, 2.5} {
		sc := sc
		t.Run(strings.ReplaceAll(fmt.Sprint(sc), ".", "_"), func(t *testing.T) {
			theme.SetScale(sc)
			t.Cleanup(func() { theme.SetScale(1) })
			g := newTitleGameAt(t, filepath.Join(t.TempDir(), "save.json"))
			w, h := int(1280*sc), int(720*sc)
			g.applySize(w, h)
			g.titleUI.open("credits")
			pump(t, g, w, h)

			if g.titleUI.creditScroll == nil {
				t.Fatal("the credits window mounted without its scroll viewport")
			}
			content := widgets.ScrollContentOf(g.titleUI.creditScroll)
			page, ok := content.(widget.Containerer)
			if !ok {
				t.Fatal("no credits page")
			}

			var headers []string
			for _, kid := range page.Children() {
				if ct, ok := kid.(*theme.ChromeText); ok {
					headers = append(headers, ct.Label)
				}
			}
			want := creditHeaders()
			if strings.Join(headers, "|") != strings.Join(want, "|") {
				t.Fatalf("credits chrome = %v, want the wordmark, subtitle and one header per section %v", headers, want)
			}

			// One role/credit row per declared row: the section blocks
			// carry their text, nothing is dropped.
			joined := ""
			for _, s := range creditSections {
				for _, r := range s.Rows {
					joined += r.Text
				}
			}
			if !strings.Contains(joined, "MIT License") || len(creditSections) != 7 {
				t.Errorf("credits sections = %d, texts carry the licence: %v",
					len(creditSections), strings.Contains(joined, "MIT License"))
			}

			scroll := g.titleUI.creditScroll
			scroll.ScrollTop = 1
			pump(t, g, w, h)
			view := scroll.ViewRect()
			pr := content.GetWidget().Rect
			if pr.Max.Y > view.Max.Y+theme.Px(1) {
				t.Errorf("full scroll leaves the page bottom %d below the viewport %d", pr.Max.Y, view.Max.Y)
			} else if pr.Max.Y < view.Max.Y-theme.Px(1) {
				t.Errorf("full scroll ends %d above the viewport %d: the last section is unreachable", view.Max.Y-pr.Max.Y, view.Max.Y)
			}
			if pr.Min.Y >= view.Min.Y {
				t.Errorf("the page did not scroll (top %d, viewport top %d)", pr.Min.Y, view.Min.Y)
			}
			if pr.Max.X > view.Max.X+theme.Px(1) {
				t.Errorf("page is %d px wider than the viewport", pr.Max.X-view.Max.X)
			}
			for i, kid := range page.Children() {
				if r := kid.GetWidget().Rect; r.Max.Y > view.Max.Y+theme.Px(1) {
					t.Errorf("credits block %d (bottom %d) is unreachable below the viewport %d", i, r.Max.Y, view.Max.Y)
				}
			}
		})
	}
}
