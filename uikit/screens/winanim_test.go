package screens

import (
	"image"
	"testing"

	"github.com/entieve/entieve/uikit/theme"
)

// Open/close animation (CONTEXT.md "Window animation", "Settle"):
// render-only, deterministic frame counting — the endpoints are pinned
// as pure math and as frame counts, the close fade defers its unmount
// exactly to the last frame while the window stays inert, and an open
// fade leaves input live from its very first frame.

// bodySample is a point inside a window's body background band: two
// design px past the title bar's bottom border and three px inside the
// left edge — every kit window's contents padding keeps it clear of
// content.
func bodySample(r image.Rectangle) image.Point {
	return image.Pt(r.Min.X+3, r.Min.Y+theme.Px(theme.TitleBarH)+3)
}

// TestWindowAnimationEndpoints pins the animation's timing as pure
// math: frame 0 of an open fade is fully transparent at full slide,
// frame openFrames is the settled endpoint (opaque, no offset) — which
// is exactly the normal paint path — and the close fade runs 255 → 0
// over closeFrames frames. The sweep keeps both curves monotone, so no
// intermediate frame can flash backwards.
func TestWindowAnimationEndpoints(t *testing.T) {
	if a, dy := openAnim(0); a != 0 || dy != theme.Px(openSlidePx) {
		t.Errorf("openAnim(0) = (%d, %d), want (0, %d)", a, dy, theme.Px(openSlidePx))
	}
	if a, dy := openAnim(openFrames); a != 255 || dy != 0 {
		t.Errorf("openAnim(%d) = (%d, %d), want (255, 0)", openFrames, a, dy)
	}
	if a := closeAnim(0); a != 255 {
		t.Errorf("closeAnim(0) = %d, want 255", a)
	}
	if a := closeAnim(closeFrames); a != 0 {
		t.Errorf("closeAnim(%d) = %d, want 0", closeFrames, a)
	}
	var lastA uint8
	lastDy := 1 << 30
	for f := 0; f <= openFrames; f++ {
		a, dy := openAnim(f)
		if a < lastA {
			t.Errorf("open alpha flashed back at frame %d: %d < %d", f, a, lastA)
		}
		if dy > lastDy {
			t.Errorf("open offset flashed back at frame %d: %d > %d", f, dy, lastDy)
		}
		lastA, lastDy = a, dy
	}
	lastA = 255
	for f := 0; f <= closeFrames; f++ {
		a := closeAnim(f)
		if a > lastA {
			t.Errorf("close alpha flashed forward at frame %d: %d > %d", f, a, lastA)
		}
		lastA = a
	}
	// The timing itself: 12 and 10 frames at 60 TPS is the ~0.2 s / ~0.17 s
	// the feature promises — pin the constants, not just their shape.
	if openFrames != 12 || closeFrames != 10 {
		t.Errorf("animation timing = %d open / %d close frames, want 12/10", openFrames, closeFrames)
	}
}

// TestOpenAnimationFrameCounting: a mount starts at open frame 0, every
// Draw advances exactly one frame (deterministic counting — no wall
// clock), and the animation is gone after exactly openFrames frames,
// from which the window paints through the normal path. The visual
// half of the endpoints (frame 0 reaches no pixels, the settled fill
// reads the translucency token) is pinned in winchrome_capture_test.go
// — pixel reads need a running ebiten loop, the repo's capture gate.
func TestOpenAnimationFrameCounting(t *testing.T) {
	k, _ := newTestKit(t)
	log := NewLog(k.Env())
	k.AddWindow(log.Win())
	win := log.Win()

	a, ok := k.anims[win]
	if !ok || !a.open || a.frame != 0 {
		t.Fatalf("fresh mount anim = %+v ok=%v, want open frame 0", a, ok)
	}
	for i := 1; i <= openFrames; i++ {
		pumpKit(t, k, 1280, 720)
		if i < openFrames {
			if a := k.anims[win]; a == nil || !a.open || a.frame != i {
				t.Fatalf("after %d Draws anim = %+v, want open frame %d", i, a, i)
			}
			continue
		}
		if _, running := k.anims[win]; running {
			t.Errorf("animation still running after %d frames", openFrames)
		}
	}
}

// TestOpenFadeKeepsInputLive: input works from the first frame of the
// open fade — a window you just opened accepts the × press mid-fade,
// which also switches the running animation straight into the close
// phase (no flash to full alpha first).
func TestOpenFadeKeepsInputLive(t *testing.T) {
	k, _ := newTestKit(t)
	fm := useFakeMouse(t)
	// Park the core windows clear of the log's × so the press cannot
	// raise another window (a raise settles fades by design — this test
	// needs the open fade RUNNING under the press).
	k.OverviewWindow().SetLocation(image.Rect(700, 8, 1080, 428))
	k.SelectedWindow().SetLocation(image.Rect(700, 436, 1150, 696))
	log := NewLog(k.Env())
	closes := 0
	k.AddWindow(log.Win())
	k.SetWindowClose(log.Win(), func() { closes++ })
	win := log.Win()

	pumpKit(t, k, 1024, 700) // one frame in: still deep inside the fade
	if a := k.anims[win]; a == nil || !a.open || a.frame != 1 {
		t.Fatalf("fixture: anim = %+v, want open frame 1 (mid-fade)", a)
	}
	clickAt(t, k, fm, 1024, 700, closeCenter(t, win))
	if closes != 1 {
		t.Fatalf("× press mid-fade fired %d closes, want 1 — input must be live during the open animation", closes)
	}
	if a := k.anims[win]; a == nil || a.open {
		t.Errorf("animation did not switch to the close phase: %+v", a)
	}
}

// TestCloseFadeDefersUnmountAndStaysInert: the × commits the close
// (the window goes inert immediately — a second press changes nothing),
// the host's remove is DEFERRED until the fade has run exactly
// closeFrames frames, and the settle performs the unmount: chrome, stack
// entry and animation all gone.
func TestCloseFadeDefersUnmountAndStaysInert(t *testing.T) {
	k, _ := newTestKit(t)
	fm := useFakeMouse(t)
	// Park the core windows clear of the log: a press that lands over
	// ANOTHER visible window raises that window (the raise contract),
	// and a raise settles running fades by design — the fixture must
	// keep this test's presses to the closing window alone.
	k.OverviewWindow().SetLocation(image.Rect(700, 8, 1080, 428))
	k.SelectedWindow().SetLocation(image.Rect(700, 436, 1150, 696))
	log := NewLog(k.Env())
	closes := 0
	k.AddWindow(log.Win())
	k.SetWindowClose(log.Win(), func() { closes++ })
	win := log.Win()
	settlePump(t, k, 1024, 700)

	// Grab the detach handle the host holds (AddWindow's return — the
	// game stores it in g.mounted).
	var hostRemove func()
	for _, m := range k.stack {
		if m.win == win {
			hostRemove = m.remove
		}
	}
	if hostRemove == nil {
		t.Fatal("fixture: log window not on the stack")
	}

	clickAt(t, k, fm, 1024, 700, closeCenter(t, win))
	if closes != 1 {
		t.Fatalf("close count = %d, want 1", closes)
	}
	a, ok := k.anims[win]
	if !ok || a.open {
		t.Fatalf("anim after × = %+v ok=%v, want a close fade", a, ok)
	}
	if _, mounted := k.mountOf(win); !mounted {
		t.Fatal("window unmounted before its fade settled")
	}
	if win.GetContainer().GetWidget().IsVisible() {
		t.Error("closing window is still visible — it must be inert for the whole fade")
	}

	// The host unmounts (what syncMounts does): deferred to settle.
	hostRemove()
	if _, mounted := k.mountOf(win); !mounted {
		t.Error("host remove was not deferred — the window must fade before it unmounts")
	}
	if _, chromed := k.chromes[win]; !chromed {
		t.Error("chrome detached before the fade settled")
	}

	// Inert: another × press during the fade fires nothing (the window
	// is hidden, so its button sees no events at all).
	clickAt(t, k, fm, 1024, 700, closeCenter(t, win))
	if closes != 1 {
		t.Errorf("close count during the fade = %d, want 1 — a closing window is inert", closes)
	}

	// The fade runs exactly closeFrames Draw frames from the commit —
	// deterministic frame counting (no wall clock). Every pump above
	// advanced it once; this counts the remaining frames off the anim's
	// own counter, so the unmount lands on the closeFrames-th draw and
	// not one frame earlier.
	drawn := k.anims[win].frame
	if drawn >= closeFrames {
		t.Fatalf("fixture: fade already at frame %d", drawn)
	}
	for f := drawn + 1; f < closeFrames; f++ {
		pumpKit(t, k, 1024, 700)
		if _, mounted := k.mountOf(win); !mounted {
			t.Fatalf("unmounted at fade frame %d, want the full %d-frame fade", f, closeFrames)
		}
	}
	pumpKit(t, k, 1024, 700) // the closeFrames-th draw: settle performs the unmount
	if _, mounted := k.mountOf(win); mounted {
		t.Error("window still mounted after the close fade settled")
	}
	if _, chromed := k.chromes[win]; chromed {
		t.Error("chrome still attached after settle")
	}
	if _, animating := k.anims[win]; animating {
		t.Error("animation still registered after settle")
	}
	if closes != 1 {
		t.Errorf("close count at settle = %d, want 1", closes)
	}
}
