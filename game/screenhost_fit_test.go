package game

import (
	"image"
	"testing"

	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// fittedPanel is a Screen panel that answers the Fitted facet: a window
// plus a content floor in DESIGN units, and a call counter for the
// "never asked of an unmounted panel" rule.
type fittedPanel struct {
	*fixturePanel
	w, h  int
	calls int
}

func (p *fittedPanel) FitSize() (int, int) {
	p.calls++
	return p.w, p.h
}

// fittedRow declares one row over a fitted fixture panel.
func fittedRow(id ScreenID, p *fittedPanel, column image.Rectangle, a Anchor) Screen {
	return Screen{
		ID: id, Title: string(id), DockPolicy: DockFloating,
		DefaultRect: column, Anchor: a,
		Build: func(*ScreenHost) widgets.WindowPanel { return p },
	}
}

// TestFittedPanelAppliesMinSizeThroughPlace: a mounted row whose panel
// answers FitSize gets the floor mirrored onto the window's MinSize and
// the row's declared column composed from that height — through the
// placement tracker only, so a player-moved window keeps its position.
func TestFittedPanelAppliesMinSizeThroughPlace(t *testing.T) {
	column := image.Rect(97, 8, 404, 208)
	p := &fittedPanel{fixturePanel: newFixturePanel(), w: 307, h: 300}
	h := newScreenHost([]Screen{fittedRow("board", p, column, AnchorFixed)}, gameScope, newFakeSink(), screenServices{})
	h.Warm()
	if p.calls != 0 {
		t.Fatalf("FitSize asked %d times before the mount", p.calls)
	}
	h.SetOpen("board", true)
	h.Reconcile(Room{})

	win := p.Win()
	if min := win.MinSize; min == nil || *min != (image.Point{X: theme.Px(307), Y: theme.Px(300)}) {
		t.Fatalf("MinSize = %v, want the fitted floor 307×300 (design units)", min)
	}
	if got, want := win.GetContainer().GetWidget().Rect, image.Rect(97, 8, 404, 308); got != want {
		t.Fatalf("window = %v, want the fitted column %v", got, want)
	}

	// A taller floor re-places the still-virgin window through the
	// tracker (the host composes, never SetLocation).
	p.h = 340
	h.Sync()
	if got, want := win.GetContainer().GetWidget().Rect, image.Rect(97, 8, 404, 348); got != want {
		t.Errorf("after a taller floor: window = %v, want %v", got, want)
	}

	// The player drags it: the floor still updates, the position does not.
	moved := image.Rect(500, 100, 807, 440)
	win.SetLocation(moved)
	p.h = 380
	h.Sync()
	if min := win.MinSize; min == nil || min.Y != theme.Px(380) {
		t.Errorf("MinSize after the move = %v, want the new floor", min)
	}
	if got := win.GetContainer().GetWidget().Rect; got != moved {
		t.Errorf("player-moved window = %v, want %v (fit stomped it)", got, moved)
	}
}

// TestFittedOnlyFixedAnchors: an unfixed anchor keeps its own placement
// rule — FitSize is never asked, no MinSize is imposed.
func TestFittedOnlyFixedAnchors(t *testing.T) {
	p := &fittedPanel{fixturePanel: newFixturePanel(), w: 307, h: 300}
	h := newScreenHost([]Screen{fittedRow("centred", p, image.Rect(97, 8, 404, 208), AnchorCenter)}, gameScope, newFakeSink(), screenServices{})
	h.Warm()
	h.SetOpen("centred", true)
	h.Reconcile(Room{})
	if p.calls != 0 {
		t.Errorf("FitSize asked %d times for an unfixed row", p.calls)
	}
	if min := p.Win().MinSize; min != nil {
		t.Errorf("MinSize = %v, want none (the row does not fit)", min)
	}
}
