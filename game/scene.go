package game

import (
	"image/color"
	"math"
	"time"

	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	uikit "github.com/entieve/entieve/uikit/screens"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Input is one frame's snapshot of ebiten input, taken by Game and handed
// to the top scene. ebitenui reads the raw input itself through
// Kit.Update; scenes only see this pure snapshot.
type Input struct {
	MouseX, MouseY float64
	JustPressed    []ebiten.Key
	KeysDown       []ebiten.Key // every held key (modifiers, held actions, pan)
	MouseButton    [3]bool      // held: [0] left, [1] right, [2] middle
	JustClicked    [2]bool      // press edges: [0] left, [1] right
	Scroll         float64
}

// Scene is one entry of the game's scene stack. Scenes are drawn bottom-up;
// only the top scene receives Update. The interface itself (uikit windows)
// is pumped by Game regardless of scene — scenes only own world input and
// overlay dimming.
type Scene interface {
	Update(input Input) error
	Draw(dst *ebiten.Image)
	OnEnter()
	OnLeave()
}

// Camera tuning for the space scene.
const (
	zoomBase = 0.02 // px/m at 1x zoom multiplier
	// zoomMin lets the widest framing fit the whole system on the short edge
	// at the 1280×720 design size: 720 px / (zoomBase·zoomMin) ≈ 120 AU, so
	// gates out at 40 AU and the star are on screen together. The tight end
	// is untouched — grid-scale UX lives at zoomBase·zoomMax (≈ 16 km across).
	zoomMin           = 2e-9
	zoomMax           = 4.0
	zoomStep          = 1.15 // wheel notch factor
	panSpeed          = 10.0 // screen px per arrow press
	pickRadiusPx      = 18.0
	doubleClickWindow = 400 * time.Millisecond
)

// orbitDefaultRange is the fixed range of a right-click orbit: the first
// Selected Item orbit range, so the help overlay's number and the command
// can never drift apart from uikit.OrbitRanges (pinned by
// TestHelpRangeNumbersMatchExportedArrays).
var orbitDefaultRange = uikit.OrbitRanges[0]

// SpaceScene is the hub: world rendering, camera and world clicks. Every
// mouse interaction is suppressed while the pointer sits on any interface
// panel (Game.overUI).
type SpaceScene struct {
	g    *Game
	pan  core.Vec2 // camera offset from the player, world meters
	zoom float64   // zoom multiplier in [zoomMin, zoomMax]

	// double-click bookkeeping
	lastClickAt time.Time
	lastClickX  float64
	lastClickY  float64

	// middle-drag panning
	middle     bool
	midX, midY float64
}

// OnEnter implements Scene.
func (sc *SpaceScene) OnEnter() {}

// OnLeave implements Scene.
func (sc *SpaceScene) OnLeave() {}

// ResetView centers the camera on the player (used after travel/restore).
func (sc *SpaceScene) ResetView() { sc.pan = core.Vec2{} }

// screenZoom is zoomBase × the zoom multiplier × the display's device
// factor: Layout hands out DEVICE pixels, and the world must keep its
// physical size there (a HiDPI framebuffer covers the same physical area
// with more pixels). The player's UI-scale setting never touches it —
// scaling the interface does not zoom the world. Everything screen-space
// below (panning, picking, click tolerance) goes through the same factor.
func (sc *SpaceScene) screenZoom() float64 {
	return zoomBase * sc.zoom * sc.g.deviceScale()
}

// Camera builds the render camera for the current screen size.
func (sc *SpaceScene) Camera(w, h float64) render.Camera {
	return render.Camera{
		Center:  sc.playerPos().Add(sc.pan),
		Zoom:    sc.screenZoom(),
		ScreenW: w,
		ScreenH: h,
	}
}

// playerPos returns the player's world position (zero if unknown).
func (sc *SpaceScene) playerPos() core.Vec2 {
	if tr, ok := sc.g.s.Transforms.Get(sc.g.s.Player()); ok {
		return tr.Pos
	}
	return core.Vec2{}
}

// panBy moves the camera center in screen pixels (converted to meters).
func (sc *SpaceScene) panBy(dxPx, dyPx float64) {
	sc.pan = sc.pan.Add(core.V(-dxPx/sc.screenZoom(), -dyPx/sc.screenZoom()))
}

// Update handles camera controls and world clicks. Keyboard panning runs
// unconditionally; every mouse gesture is gated on the pointer being off
// the interface (ebitenui does not gate raw input for us).
func (sc *SpaceScene) Update(in Input) error {
	g := sc.g

	// Arrow-key panning (screen space; the four pan commands are
	// rebindable world actions).
	step := panSpeed / (zoomBase * sc.zoom)
	if g.actionHeld(in, "pan-up") {
		sc.pan.Y -= step
	}
	if g.actionHeld(in, "pan-down") {
		sc.pan.Y += step
	}
	if g.actionHeld(in, "pan-left") {
		sc.pan.X -= step
	}
	if g.actionHeld(in, "pan-right") {
		sc.pan.X += step
	}

	over := g.overUI(in.MouseX, in.MouseY)

	// Wheel zoom — suppressed over any panel (the overview scrolls
	// itself when hovered; the chrome strips swallow the wheel).
	if in.Scroll != 0 && !over {
		sc.zoom = clampF(sc.zoom*math.Pow(zoomStep, in.Scroll), zoomMin, zoomMax)
	}

	// Middle-drag panning: a drag may start only off-panel, but an
	// already-running drag keeps panning across panels.
	if in.MouseButton[2] {
		if !sc.middle {
			if !over {
				sc.middle, sc.midX, sc.midY = true, in.MouseX, in.MouseY
			}
		} else {
			sc.panBy(in.MouseX-sc.midX, in.MouseY-sc.midY)
			sc.midX, sc.midY = in.MouseX, in.MouseY
		}
	} else {
		sc.middle = false
	}

	// The interface owns every click that lands on a panel.
	if over {
		return nil
	}
	cam := sc.Camera(float64(g.w), float64(g.h))

	if in.JustClicked[0] {
		ctrl := held(in.KeysDown, ebiten.KeyControl)
		inf, picked := sc.pick(cam, in.MouseX, in.MouseY)
		switch {
		case ctrl && picked:
			g.setSelected(inf.Entity)
			g.s.Queue(sim.CmdLock{Target: inf.Entity})
		case picked:
			g.setSelected(inf.Entity)
			if sc.doubleClick(in.MouseX, in.MouseY) {
				g.s.Queue(sim.CmdApproach{Target: inf.Entity})
			}
		default:
			if sc.doubleClick(in.MouseX, in.MouseY) {
				sc.manualToward(cam, in.MouseX, in.MouseY)
			}
		}
		sc.lastClickAt = time.Now()
		sc.lastClickX, sc.lastClickY = in.MouseX, in.MouseY
	}

	if in.JustClicked[1] {
		if inf, picked := sc.pick(cam, in.MouseX, in.MouseY); picked {
			g.setSelected(inf.Entity)
			g.s.Queue(sim.CmdOrbit{Target: inf.Entity, Range: orbitDefaultRange})
		}
	}
	return nil
}

// doubleClick reports whether this click repeats the previous one closely
// in time and space.
func (sc *SpaceScene) doubleClick(x, y float64) bool {
	tol := 6 * sc.g.deviceScale() // click pair tolerance in screen px
	return time.Since(sc.lastClickAt) <= doubleClickWindow &&
		math.Abs(x-sc.lastClickX) < tol && math.Abs(y-sc.lastClickY) < tol
}

// pick returns the perceivable grid entity nearest the cursor within
// pickRadiusPx (off-grid entities cannot be clicked — the client never
// sees what the sensors do not).
func (sc *SpaceScene) pick(cam render.Camera, x, y float64) (sim.Info, bool) {
	r := pickRadiusPx * sc.g.deviceScale() // screen px: Layout units
	bestDist := r * r
	var best sim.Info
	found := false
	for _, inf := range sc.g.s.Grid() {
		if !inf.Visible {
			continue
		}
		sx, sy := cam.WorldToScreen(inf.Pos)
		dx, dy := sx-x, sy-y
		if d := dx*dx + dy*dy; d < bestDist {
			bestDist, best, found = d, inf, true
		}
	}
	return best, found
}

// manualToward issues manual thrust toward a double-clicked point.
func (sc *SpaceScene) manualToward(cam render.Camera, x, y float64) {
	dir := cam.ScreenToWorld(x, y).Sub(sc.playerPos()).Norm()
	if dir.IsZero() {
		return
	}
	sc.g.s.Queue(sim.CmdManualVel{Dir: dir, SpeedFrac: 1})
}

// Draw renders the world only — Game draws the interface on top of every
// scene afterwards.
func (sc *SpaceScene) Draw(dst *ebiten.Image) {
	g := sc.g
	cam := sc.Camera(float64(g.w), float64(g.h))
	g.ren.SetSelected(g.kit.Selected())
	g.ren.Draw(dst, cam, g.s)
}

// clampF bounds v to [lo, hi].
func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// dimScreen draws a translucent black veil over the whole screen (overlay
// scenes use it for readability).
func dimScreen(dst *ebiten.Image, w, h int, alpha uint8) {
	vector.DrawFilledRect(dst, 0, 0, float32(w), float32(h), color.RGBA{0, 0, 0, alpha}, false)
}
