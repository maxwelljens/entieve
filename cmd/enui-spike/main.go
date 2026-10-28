// Command enui-spike is a standalone visual-evaluation demo for uikit, the
// ebitenui-based interface layer: the world-showcase simulation rendered
// through render, with the live Overview, Selected Item, Market,
// Inventory and Fitting windows on top.
//
// Run on a desktop with:
//
//	go run ./cmd/enui-spike
//
// Keys: Esc quits, +/– change time acceleration, I toggles the
// inventory, F the fitting window and G the market. Everything else
// (row clicks, sort headers, window drag/resize, wheel scrolling) lives
// in the UI.
package main

import (
	"errors"
	"fmt"
	"log"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/screens"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

const (
	screenW = 1280
	screenH = 720

	titleBase = "Entieve — ebitenui spike"

	// The camera opens close on the player and eases out until the belt
	// and its neighbourhood fit the frame.
	zoomStart = 0.06
	zoomFloor = 0.004
	zoomEase  = 0.995 // per-frame factor at 60 fps (~9 s to the floor)
)

// accelLevels are the time-acceleration steps the +/– keys cycle through.
var accelLevels = []int{1, 2, 5, 10, 30}

// spike implements ebiten.Game for the evaluation demo.
type spike struct {
	ptr   *widgets.Pointer
	sim   *sim.Sim
	ren   *render.Renderer
	kit   *screens.Kit
	env   screens.Session // the session the station panels are built from
	zoom  float64
	accel int
	acc   float64 // fixed-timestep accumulator (seconds)

	// Station windows, toggled with I/F/G; remove closes them again.
	market    *screens.Market
	inventory *screens.Inventory
	fitting   *screens.Fitting
	mkShown   bool
	invShown  bool
	fitShown  bool
	mkRemove  widget.RemoveWindowFunc
	invRemove widget.RemoveWindowFunc
	fitRemove widget.RemoveWindowFunc
}

func main() {
	c, err := content.Load(content.FS())
	if err != nil {
		log.Fatalf("load content: %v", err)
	}
	wd, ok := c.World("world-showcase")
	if !ok {
		log.Fatal("world-showcase missing from content")
	}
	s, err := sim.New(c, wd, 42)
	if err != nil {
		log.Fatalf("sim.New: %v", err)
	}
	// One pointer frame for the whole spike: the kit and the bare session
	// the station windows are built from poll the same sample.
	ptr := widgets.NewPointer(nil)
	kit, err := screens.NewKit(c, s, s.Queue, screens.WithPointer(ptr))
	if err != nil {
		log.Fatalf("screens.NewKit: %v", err)
	}
	// The station windows ride the kit's window stack (same theme,
	// stacking and input pump) through the AddWindow seam.
	env := screens.NewEnv(c, s, s.Queue, ptr)
	g := &spike{
		ptr:       ptr,
		sim:       s,
		ren:       render.New(c),
		kit:       kit,
		env:       env,
		zoom:      zoomStart,
		market:    screens.NewMarket(env),
		inventory: screens.NewInventory(env),
		fitting:   screens.NewFitting(env),
	}
	g.mkRemove = kit.AddWindow(g.market.Win())
	g.invRemove = kit.AddWindow(g.inventory.Win())
	g.fitRemove = kit.AddWindow(g.fitting.Win())
	// The session's quantity picker: mounted here (hidden) because this
	// host builds its panels from a bare session, which only shows it.
	kit.AddWindow(env.QtyWindow().Win())
	g.mkShown, g.invShown, g.fitShown = true, true, true

	ebiten.SetWindowTitle(titleBase)
	ebiten.SetWindowSize(screenW, screenH)
	ebiten.SetWindowResizable(true)

	if err := ebiten.RunGame(g); err != nil && !errors.Is(err, ebiten.Termination) {
		log.Fatal(err)
	}
}

// Update quits on Esc, handles time acceleration and window toggles,
// syncs the kit and the station windows, and steps the simulation on a
// fixed 60 tps timestep.
func (g *spike) Update() error {
	// One pointer frame per Update: the sample the seam polls and cursor
	// claims below read (a claim applies to this frame only, so a pointer
	// that left a seam falls back to the arrow).
	g.ptr.Begin()
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return ebiten.Termination
	}
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyEqual):
		if g.accel < len(accelLevels)-1 {
			g.accel++
			g.refreshTitle()
		}
	case inpututil.IsKeyJustPressed(ebiten.KeyMinus):
		if g.accel > 0 {
			g.accel--
			g.refreshTitle()
		}
	case inpututil.IsKeyJustPressed(ebiten.KeyI):
		g.toggle(&g.invShown, &g.invRemove, g.inventory.Win())
	case inpututil.IsKeyJustPressed(ebiten.KeyF):
		g.toggle(&g.fitShown, &g.fitRemove, g.fitting.Win())
	case inpututil.IsKeyJustPressed(ebiten.KeyG):
		g.toggle(&g.mkShown, &g.mkRemove, g.market.Win())
	}

	// Ease the camera out toward the belt (frame-rate based, fine for a
	// spike; a real camera would be time-based with bounds).
	g.zoom *= zoomEase
	if g.zoom < zoomFloor {
		g.zoom = zoomFloor
	}

	g.kit.Sync()
	g.market.Sync()
	g.inventory.Sync()
	g.fitting.Sync()
	g.env.QtyWindow().Sync()
	if err := g.kit.Update(); err != nil {
		return err
	}

	g.acc += frameSec() * float64(accelLevels[g.accel])
	steps := int(g.acc / sim.DT)
	const maxSteps = 600 // a stalled frame must not spiral
	if steps > maxSteps {
		steps = maxSteps
		g.acc = 0
	} else {
		g.acc -= float64(steps) * sim.DT
	}
	for i := 0; i < steps; i++ {
		g.sim.Step()
	}
	g.sim.Events() // drain: the spike has no event log
	return nil
}

// toggle shows or hides one of the station windows.
func (g *spike) toggle(shown *bool, remove *widget.RemoveWindowFunc, win *widget.Window) {
	if *shown {
		(*remove)()
		*shown = false
		return
	}
	*remove = g.kit.AddWindow(win)
	*shown = true
}

// Draw renders the world first, then the interface on top.
func (g *spike) Draw(dst *ebiten.Image) {
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	cam := render.Camera{
		Center:  g.playerPos(),
		Zoom:    g.zoom,
		ScreenW: float64(w),
		ScreenH: float64(h),
	}
	g.ren.SetSelected(g.kit.Selected())
	g.ren.Draw(dst, cam, g.sim)
	g.kit.Draw(dst)
}

// Layout passes the (resizable) window size through unclamped.
func (g *spike) Layout(width, height int) (int, int) { return width, height }

// playerPos is the camera focus: the player's current grid position.
func (g *spike) playerPos() core.Vec2 {
	if tr, ok := g.sim.Transforms.Get(g.sim.Player()); ok {
		return tr.Pos
	}
	return core.Vec2{}
}

// refreshTitle stamps the current time acceleration onto the window title.
func (g *spike) refreshTitle() {
	ebiten.SetWindowTitle(fmt.Sprintf("%s [×%d]", titleBase, accelLevels[g.accel]))
}

// frameSec returns the real seconds of one frame, guarded against a
// stalled or unknown TPS.
func frameSec() float64 {
	tps := ebiten.ActualTPS()
	if tps <= 0 || tps > 500 {
		return 1.0 / 60.0
	}
	return 1.0 / tps
}
