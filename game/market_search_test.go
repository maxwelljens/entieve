package game

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestMarketSearchKeyboardGate extends the typing gate to the market's
// search field: Ctrl+F opens the market and focuses the field; while it is
// focused 's' does not fire the world stop and 'n' does not open the map;
// Esc on the empty field blurs and falls through to the normal Esc menu.
// (The order window's own gate stays pinned by
// TestOrderWindowKeyboardGate.)
func TestMarketSearchKeyboardGate(t *testing.T) {
	g := dockedShowcaseGame(t, 1280, 720)
	before, hadBefore := g.s.Intents.Get(g.s.Player())

	// Ctrl+F opens the market and focuses its search field (the focus is
	// applied in the frame pass after the mount).
	g.handleKeys(Input{KeysDown: []ebiten.Key{ebiten.KeyControl}, JustPressed: []ebiten.Key{ebiten.KeyF}})
	if !g.open[pMarket] {
		t.Fatal("Ctrl+F did not open the market")
	}
	g.applySize(1280, 720)
	g.syncMounts()
	if !g.mktSearchFocus {
		t.Fatal("Ctrl+F did not queue the search focus")
	}
	g.Update() // consumes the pending focus
	if g.mkt == nil || !g.mkt.SearchFocused() {
		t.Fatal("Ctrl+F did not focus the search field")
	}
	g.s.Step()

	// Focused: 's' yields (the intent is untouched) and 'n' does not open
	// the map.
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyS}})
	g.s.Step()
	if after, ok := g.s.Intents.Get(g.s.Player()); ok != hadBefore || after != before {
		t.Fatalf("focused 's' moved the intent: %+v (was %+v)", after, before)
	}
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyN}})
	if g.open[pMap] {
		t.Fatal("focused 'n' opened the map")
	}

	// Esc on the empty field: the field blurs and the normal Esc handling
	// follows (the Esc menu).
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyEscape}})
	if g.mkt.SearchFocused() {
		t.Fatal("Esc did not blur the empty search field")
	}
	if _, ok := g.top().(*MenuScene); !ok {
		t.Fatal("Esc on the empty search field did not fall through to the Esc menu")
	}
}
