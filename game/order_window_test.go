package game

import (
	"testing"

	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestOrderWindowKeyboardGate pins the typing seam: while a price/qty
// field of the order mini-window holds focus, the world hotkeys yield —
// 's' does not fire the stop, 'n' does not open the map — and Esc clears
// the focus (it must not push the Esc menu either). Unfocused, every
// hotkey works normally again, even with the window open.
func TestOrderWindowKeyboardGate(t *testing.T) {
	g := dockedShowcaseGame(t, 1280, 720)
	g.kit.OpenOrder("min-ferrite", sim.OrderSell, 1)
	if !g.kit.OrderOpen() {
		t.Fatal("fixture: order window did not open")
	}
	before, hadBefore := g.s.Intents.Get(g.s.Player())

	// Focused: 's' yields (the intent is untouched) and 'n' does not open
	// the map.
	g.kit.OrderWin().FocusPrice()
	if !g.kit.OrderTyping() {
		t.Fatal("a focused field does not read as typing")
	}
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyS}})
	g.s.Step()
	if after, ok := g.s.Intents.Get(g.s.Player()); ok != hadBefore || after != before {
		t.Fatalf("focused 's' moved the intent: %+v (was %+v)", after, before)
	}
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyN}})
	if _, ok := g.top().(*MapScene); ok {
		t.Fatal("focused 'n' opened the map")
	}

	// Esc while typing clears the focus and pushes nothing.
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyEscape}})
	if _, ok := g.top().(*MenuScene); ok {
		t.Fatal("Esc while typing opened the Esc menu")
	}
	if g.kit.OrderTyping() {
		t.Fatal("Esc while typing did not clear the field focus")
	}

	// Unfocused: 's' fires the world stop again, and 'n' opens the map.
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyS}})
	g.s.Step()
	if it, ok := g.s.Intents.Get(g.s.Player()); !ok || it.Kind != sim.IntentStop {
		t.Fatalf("unfocused 's' did not fire the world stop: %+v (ok=%v)", it, ok)
	}
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyN}})
	if _, ok := g.top().(*MapScene); !ok {
		t.Fatal("unfocused 'n' did not open the map")
	}
}

// TestOrderWindowClosesOnUndock pins the transient lifecycle: the order
// window is docked-only and closes as the player leaves the station
// (the same docked state syncMounts reconciles every frame).
func TestOrderWindowClosesOnUndock(t *testing.T) {
	g := dockedShowcaseGame(t, 1280, 720)
	g.kit.OpenOrder("min-ferrite", sim.OrderSell, 1)
	if !g.kit.OrderOpen() {
		t.Fatal("fixture: order window did not open")
	}

	g.s.Docked.Remove(g.s.Player())
	g.syncMounts()
	if g.kit.OrderOpen() {
		t.Fatal("order window stayed open after undocking")
	}
}

// TestOrderWindowOpensCentered pins the launch geometry: the mini-window
// opens centred on the viewport (the host places it with the same rule the
// other on-demand overlays use), and no sidecar key persists it.
func TestOrderWindowOpensCentered(t *testing.T) {
	g := dockedShowcaseGame(t, 1280, 720)
	w := g.kit.OrderWindow()
	if w == nil {
		t.Fatal("no order window")
	}
	// Open it (mounted + themed, like any real launch) and look at the
	// placed rect: the host centres the shown dialog once, and its size is
	// the content's own.
	g.kit.OpenOrder("min-ferrite", sim.OrderSell, 1)
	if !g.kit.OrderOpen() {
		t.Fatal("fixture: order window did not open")
	}
	pump(t, g, 1280, 720)
	r := w.GetContainer().GetWidget().Rect
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	if cx != 1280/2 || cy != 720/2 {
		t.Errorf("order window centre = %d,%d, want the viewport centre %d,%d (rect %v)",
			cx, cy, 1280/2, 720/2, r)
	}

	// The frame is sized to its content: the window must be at least the
	// container's preferred size, so no row (the qty field, the buttons)
	// overflows it. A content growth past the pinned size fails here.
	if pw, ph := w.GetContainer().PreferredSize(); r.Dx() < pw || r.Dy() < ph {
		t.Errorf("order window %v is smaller than its content %dx%d", r, pw, ph)
	}

	// Not persisted: the sidecar name list holds only the core windows and
	// the Screen rows — never the transient order dialog.
	for _, name := range layoutNames {
		if name == "order" || name == "orderwin" {
			t.Errorf("order window leaked into the layout sidecar as %q", name)
		}
	}
}
