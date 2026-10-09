package game

import (
	"image/color"
	"math"
	"time"

	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/ui"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Scene is one entry of the game's scene stack. Scenes are drawn bottom-up;
// only the top scene receives Update.
type Scene interface {
	Update(input ui.Input) error
	Draw(dst *ebiten.Image)
	OnEnter()
	OnLeave()
}

// Camera tuning for the space scene.
const (
	zoomBase          = 0.02 // px/m at 1x zoom multiplier
	zoomMin           = 0.05
	zoomMax           = 4.0
	zoomStep          = 1.15 // wheel notch factor
	panSpeed          = 10.0 // screen px per arrow press
	orbitDefaultRange = 2500.0
	pickRadiusPx      = 18.0
	doubleClickWindow = 400 * time.Millisecond
)

// SpaceScene is the hub: world rendering, camera and world clicks.
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

// Camera builds the render camera for the current screen size.
func (sc *SpaceScene) Camera(w, h float64) render.Camera {
	return render.Camera{
		Center:  sc.playerPos().Add(sc.pan),
		Zoom:    zoomBase * sc.zoom,
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
	sc.pan = sc.pan.Add(core.V(-dxPx/(zoomBase*sc.zoom), -dyPx/(zoomBase*sc.zoom)))
}

// Update handles camera controls and world clicks; the UI gets the input
// first so panel clicks and UI keys are consumed there.
func (sc *SpaceScene) Update(in ui.Input) error {
	g := sc.g
	if err := g.u.Update(in); err != nil {
		return err
	}

	// Arrow-key panning (screen space).
	step := panSpeed / (zoomBase * sc.zoom)
	if held(in.KeysDown, ebiten.KeyArrowUp) {
		sc.pan.Y -= step
	}
	if held(in.KeysDown, ebiten.KeyArrowDown) {
		sc.pan.Y += step
	}
	if held(in.KeysDown, ebiten.KeyArrowLeft) {
		sc.pan.X -= step
	}
	if held(in.KeysDown, ebiten.KeyArrowRight) {
		sc.pan.X += step
	}

	// Wheel zoom — suppressed over any UI panel (the overview scrolls
	// itself when hovered; chrome swallows the wheel elsewhere).
	if in.Scroll != 0 && !g.u.PanelAt(in.MouseX, in.MouseY) {
		sc.zoom = clampF(sc.zoom*math.Pow(zoomStep, in.Scroll), zoomMin, zoomMax)
	}

	// Middle-drag panning.
	if in.MouseButton[2] {
		if !sc.middle {
			sc.middle, sc.midX, sc.midY = true, in.MouseX, in.MouseY
		} else {
			sc.panBy(in.MouseX-sc.midX, in.MouseY-sc.midY)
			sc.midX, sc.midY = in.MouseX, in.MouseY
		}
	} else {
		sc.middle = false
	}

	// The UI owns clicks that land on its panels.
	if g.u.PanelAt(in.MouseX, in.MouseY) {
		return nil
	}
	cam := sc.Camera(float64(g.w), float64(g.h))

	if in.JustClicked[0] {
		ctrl := held(in.KeysDown, ebiten.KeyControl)
		inf, picked := sc.pick(cam, in.MouseX, in.MouseY)
		switch {
		case ctrl && picked:
			g.u.Select(inf.Entity)
			g.s.Queue(sim.CmdLock{Target: inf.Entity})
		case picked:
			g.u.Select(inf.Entity)
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
			g.u.Select(inf.Entity)
			g.s.Queue(sim.CmdOrbit{Target: inf.Entity, Range: orbitDefaultRange})
		}
	}
	return nil
}

// doubleClick reports whether this click repeats the previous one closely
// in time and space.
func (sc *SpaceScene) doubleClick(x, y float64) bool {
	return time.Since(sc.lastClickAt) <= doubleClickWindow &&
		math.Abs(x-sc.lastClickX) < 6 && math.Abs(y-sc.lastClickY) < 6
}

// pick returns the perceivable grid entity nearest the cursor within
// pickRadiusPx (off-grid entities cannot be clicked — the client never
// sees what the sensors do not).
func (sc *SpaceScene) pick(cam render.Camera, x, y float64) (sim.Info, bool) {
	bestDist := pickRadiusPx * pickRadiusPx
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

// Draw renders the world, the UI panels and the selection highlight.
func (sc *SpaceScene) Draw(dst *ebiten.Image) {
	g := sc.g
	cam := sc.Camera(float64(g.w), float64(g.h))
	g.ren.SetSelected(g.u.Selected)
	g.ren.Draw(dst, cam, g.s)
	g.u.Draw(dst, cam, g.s)
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
