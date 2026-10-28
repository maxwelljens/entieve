package game

import (
	"image"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
)

// TestEscMenuWearsTheHouseTitleBar: the Esc menu's bar is theme.NewTitleBar's
// — the kit's ring dots, the tracked ChromeText label and the bar-bound
// ellipsis every uikit window wears — not the plain text bar the game used to
// type out for itself. The title phase's save manager shares the builder, so
// it wears the same bar.
func TestEscMenuWearsTheHouseTitleBar(t *testing.T) {
	g := newShowcaseGame(t)
	g.push(&MenuScene{g: g})
	pump(t, g, 1280, 720)

	// The bar leads with its own children (dots, then the title) and the kit
	// appends the × last.
	kids := g.host.Window(idMenu).TitleBar.Children()
	if len(kids) < 3 {
		t.Fatalf("menu title bar has %d children, want the ring dots + the tracked title + the ×", len(kids))
	}
	if _, ok := kids[0].(*widget.Graphic); !ok {
		t.Errorf("menu bar child 0 = %T, want the ring dots (*widget.Graphic)", kids[0])
	}
	txt, ok := kids[1].(*theme.ChromeText)
	if !ok {
		t.Fatalf("menu bar child 1 = %T, want the tracked chrome title (*theme.ChromeText)", kids[1])
	}
	if got := txt.Shown(); got != "Menu" {
		t.Errorf("menu title draws %q, want Menu (a fitting title is verbatim)", got)
	}

	// A title longer than the bar ellipsizes clear of the × — the plain
	// widget.Text it replaced had no such bound.
	long := "Menu — Resume · Save · Save as… · Load · Quit · Settings · Keybindings"
	win := widget.NewWindow(
		widget.WindowOpts.Contents(widget.NewContainer(widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(theme.Px(120), theme.Px(80))))),
		widget.WindowOpts.TitleBar(g.titleBar(long), theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(200), theme.Px(140)),
		widget.WindowOpts.Location(image.Rect(theme.Px(300), theme.Px(40), theme.Px(500), theme.Px(220))),
	)
	g.kit.SetWindowClose(win, func() {})
	g.kit.AddWindow(win)
	pump(t, g, 1280, 720)

	label := win.TitleBar.Children()[1].(*theme.ChromeText)
	closeBtn := g.kit.CloseButtonOf(win).GetWidget().Rect
	rect := label.GetWidget().Rect
	if closeBtn.Empty() || rect.Empty() {
		t.Fatalf("fixture: × %v / label %v never laid out", closeBtn, rect)
	}
	face, size := *g.env.FaceMono(theme.StepBase), g.env.TS().Base
	avail := float64(closeBtn.Min.X-theme.Px(theme.SpSm)) - float64(rect.Min.X)
	if m := theme.MeasureTracked(long, face, size); m <= avail {
		t.Fatalf("fixture: the title measures %v in %v of room — it cannot overflow", m, avail)
	}
	shown := label.Shown()
	if shown == long || !strings.HasSuffix(shown, "…") {
		t.Errorf("the menu title draws %q, want the ellipsized form of %q", shown, long)
	}
	if got := theme.MeasureTracked(shown, face, size); got > avail {
		t.Errorf("the menu title draws %v wide in %v of room: it runs under the ×", got, avail)
	}
}
