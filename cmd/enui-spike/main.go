// Command enui-spike is a standalone visual-evaluation demo for uikit, the
// ebitenui-based interface layer: the world-showcase simulation rendered
// through render, with the live Overview and Selected Item windows on top.
//
// Run on a desktop with:
//
//	go run ./cmd/enui-spike
//
// Keys: Esc quits, +/– change time acceleration. Everything else (row
// clicks, sort headers, window drag/resize, wheel paging) lives in the UI.
package main

import (
	"errors"
	"fmt"
	"log"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit"
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
	sim   *sim.Sim
	ren   *render.Renderer
	kit   *uikit.Kit
	zoom  float64
	accel int
	acc   float64 // fixed-timestep accumulator (seconds)
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
	kit, err := uikit.NewKit(c, s, s.Queue)
	if err != nil {
		log.Fatalf("uikit.NewKit: %v", err)
	}

	ebiten.SetWindowTitle(titleBase)
	ebiten.SetWindowSize(screenW, screenH)
	ebiten.SetWindowResizable(true)

	g := &spike{sim: s, ren: render.New(c), kit: kit, zoom: zoomStart}
	if err := ebiten.RunGame(g); err != nil && !errors.Is(err, ebiten.Termination) {
		log.Fatal(err)
	}
}

// Update quits on Esc, handles time acceleration, syncs the kit and steps
// the simulation on a fixed 60 tps timestep.
func (g *spike) Update() error {
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
	}

	// Ease the camera out toward the belt (frame-rate based, fine for a
	// spike; a real camera would be time-based with bounds).
	g.zoom *= zoomEase
	if g.zoom < zoomFloor {
		g.zoom = zoomFloor
	}

	g.kit.Sync()
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
