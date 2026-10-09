package ui

import "testing"

// TestClampRectToScreen: dragged windows stay fully inside the screen and
// oversized rects shrink to fit.
func TestClampRectToScreen(t *testing.T) {
	const sw, sh = 1280.0, 720.0
	r := ClampRectToScreen(Rect{X: 1200, Y: 700, W: 300, H: 200}, sw, sh)
	if r.Right() > sw || r.Bottom() > sh {
		t.Fatalf("clamped rect escapes screen: %v", r)
	}
	r = ClampRectToScreen(Rect{X: -50, Y: -10, W: 100, H: 50}, sw, sh)
	if r.X != 0 || r.Y != 0 {
		t.Fatalf("negative position not clamped: %v", r)
	}
	oversized := ClampRectToScreen(Rect{X: 0, Y: 0, W: 2000, H: 900}, sw, sh)
	if oversized.W != sw || oversized.H != sh {
		t.Fatalf("oversized rect not shrunk: %v", oversized)
	}
	if oversized.Right() > sw || oversized.Bottom() > sh {
		t.Fatalf("shrunk rect still escapes: %v", oversized)
	}
}

// TestDragRectKeepsGrabOffset: the point that started the drag follows the
// cursor, and the result is screen-clamped.
func TestDragRectKeepsGrabOffset(t *testing.T) {
	r := Rect{X: 100, Y: 100, W: 300, H: 200}
	got := DragRect(r, 50, 10, 300, 300, 1280, 720)
	if got.X != 250 || got.Y != 290 {
		t.Fatalf("drag moved to %v, want {250 290 300 200}", got)
	}
	if got.W != r.W || got.H != r.H {
		t.Fatalf("drag changed size: %v", got)
	}
	// Pointer near the top-left: position clamps to the screen corner.
	got = DragRect(r, 50, 10, 5, 5, 1280, 720)
	if got.X != 0 || got.Y != 0 {
		t.Fatalf("drag not clamped: %v", got)
	}
	// Pointer past the bottom-right: clamped to the far corner.
	got = DragRect(r, 50, 10, 5000, 5000, 1280, 720)
	if got.Right() != 1280 || got.Bottom() != 720 {
		t.Fatalf("drag not clamped at far edge: %v", got)
	}
}

// TestRaisePromotesToFront: click-to-raise moves a window to the front and
// is idempotent; unknown ids leave the order alone.
func TestRaisePromotesToFront(t *testing.T) {
	z := []WinID{WinStation, WinOverview, WinLog, WinFitting}
	got := Raise(z, WinOverview)
	want := []WinID{WinStation, WinLog, WinFitting, WinOverview}
	if len(got) != len(want) {
		t.Fatalf("raise produced %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("raise produced %v, want %v", got, want)
		}
	}
	// Idempotent at the front.
	if again := Raise(got, WinOverview); again[len(again)-1] != WinOverview ||
		len(again) != len(got) {
		t.Fatalf("re-raise changed order: %v", again)
	}
	// Unknown id: unchanged order.
	unknown := Raise(z, "nope")
	for i := range z {
		if unknown[i] != z[i] {
			t.Fatalf("unknown id reordered: %v", unknown)
		}
	}
	// Original slice not mutated in place (Raise builds a new slice).
	if z[0] != WinStation || z[1] != WinOverview {
		t.Fatalf("Raise mutated its input: %v", z)
	}
}

// TestTitleGrabOffset: drags start anywhere on the title bar except the
// close/filter buttons, and the grab offset is preserved.
func TestTitleGrabOffset(t *testing.T) {
	r := Rect{X: 200, Y: 50, W: 300, H: 200}
	close := CloseButtonRect(r)
	filter := OverviewFilterButtonRect(r)
	if dx, dy, ok := TitleGrabOffset(r, 250, 60); !ok || dx != 50 || dy != 10 {
		t.Fatalf("grab on title: dx=%v dy=%v ok=%v", dx, dy, ok)
	}
	if _, _, ok := TitleGrabOffset(r, close.X+2, close.Y+2, close); ok {
		t.Fatal("grab over close button must be rejected")
	}
	if _, _, ok := TitleGrabOffset(r, filter.X+2, filter.Y+2, filter); ok {
		t.Fatal("grab over filter button must be rejected")
	}
	if _, _, ok := TitleGrabOffset(r, 250, 300); ok {
		t.Fatal("grab below the title bar must be rejected")
	}
}

// TestWindowOpenAndDefaults: windows open/close through their toggle state,
// opening raises them, and default rects come from the layout.
func TestWindowOpenAndDefaults(t *testing.T) {
	u := &UI{}
	u.initWindows()
	u.screenW, u.screenH = 1280, 720
	reg := u.reg()

	if !u.WindowOpen(WinOverview) || u.WindowOpen(WinInventory) {
		t.Fatal("default open state wrong (HUD on, overlays off)")
	}
	u.ToggleWindow(WinInventory)
	if !u.WindowOpen(WinInventory) {
		t.Fatal("ToggleWindow did not open the inventory")
	}
	if u.zorder[len(u.zorder)-1] != WinInventory {
		t.Fatalf("opening did not raise: z = %v", u.zorder)
	}
	u.ToggleWindow(WinInventory)
	if u.WindowOpen(WinInventory) {
		t.Fatal("ToggleWindow did not close the inventory")
	}

	// Default rect equals the layout rect.
	if got := u.WindowRect(WinOverview, reg); got != reg.Overview {
		t.Fatalf("default overview rect %v, want %v", got, reg.Overview)
	}
	// A dragged position persists across close/open (per-session memory).
	st := u.winStateOf(WinSelected)
	st.rect = Rect{X: 400, Y: 300, W: reg.Selected.W, H: reg.Selected.H}
	st.moved = true
	if got := u.WindowRect(WinSelected, reg); got.X != 400 || got.Y != 300 {
		t.Fatalf("moved rect not persisted: %v", got)
	}
	// But it is re-clamped if the screen shrank underneath it.
	st.rect = Rect{X: 1100, Y: 600, W: reg.Selected.W, H: reg.Selected.H}
	u.screenW, u.screenH = 1024, 700
	got := u.WindowRect(WinSelected, reg)
	if got.Right() > 1024 || got.Bottom() > 700 {
		t.Fatalf("persisted rect not re-clamped: %v", got)
	}
}

// TestScrollMaxAndClamp: scroll offsets clamp to the content height.
func TestScrollMaxAndClamp(t *testing.T) {
	if got := ScrollMax(1000, 300); got != 700 {
		t.Fatalf("ScrollMax = %v, want 700", got)
	}
	if got := ScrollMax(200, 300); got != 0 {
		t.Fatalf("ScrollMax underflow = %v, want 0", got)
	}
	if got := ClampScroll(-50, 1000, 300); got != 0 {
		t.Fatalf("clamp low = %v, want 0", got)
	}
	if got := ClampScroll(5000, 1000, 300); got != 700 {
		t.Fatalf("clamp high = %v, want 700", got)
	}
	if got := ClampScroll(123, 1000, 300); got != 123 {
		t.Fatalf("clamp passthrough = %v, want 123", got)
	}
}

// TestScrollThumbGeometry: the thumb keeps a minimum size, fills the track
// when everything fits, stays inside the track at both extremes and grows
// with the view/content ratio.
func TestScrollThumbGeometry(t *testing.T) {
	track := Rect{X: 100, Y: 50, W: 12, H: 300}
	// Everything fits: thumb fills the track.
	if got := ScrollThumbRect(track, 250, 300, 0); got != track {
		t.Fatalf("fit thumb = %v, want full track %v", got, track)
	}
	content, view := 3000.0, 300.0
	// Proportional height: track * view/content = 30.
	thumb := ScrollThumbRect(track, content, view, 0)
	if thumb.H != 30 {
		t.Fatalf("thumb h = %v, want 30", thumb.H)
	}
	if thumb.Y != track.Y {
		t.Fatalf("thumb at scroll 0 = %v, want track top", thumb)
	}
	// Scroll to the end: thumb sits at the track bottom, inside the track.
	end := ScrollThumbRect(track, content, view, ScrollMax(content, view))
	if end.Bottom() > track.Bottom()+0.001 {
		t.Fatalf("thumb escapes track bottom: %v (track %v)", end, track)
	}
	if end.Y < thumb.Y {
		t.Fatalf("thumb went backwards: %v", end)
	}
	// Tiny content: minimum thumb size, still inside the track.
	mini := ScrollThumbRect(track, 3000, 10, 0)
	if mini.H < padLarge {
		t.Fatalf("thumb %v below the %v minimum", mini.H, padLarge)
	}
	if mini.Y < track.Y || mini.Bottom() > track.Bottom()+0.001 {
		t.Fatalf("min thumb escapes track: %v", mini)
	}
	// Overscrolled offset: still clamped inside the track.
	over := ScrollThumbRect(track, content, view, 999999)
	if over.Bottom() > track.Bottom()+0.001 {
		t.Fatalf("overscrolled thumb escapes: %v", over)
	}
}
