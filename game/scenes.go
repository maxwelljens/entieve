package game

import (
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/ui"
	"github.com/hajimehoshi/ebiten/v2"
)

// ---------------------------------------------------------------------------
// StationScene (pushed while docked)
// ---------------------------------------------------------------------------

// StationScene shows station services; the services panel itself is drawn
// by the UI (it switches on the docked state), this scene adds the
// undock control and routes input to the UI.
type StationScene struct{ g *Game }

// OnEnter implements Scene.
func (sc *StationScene) OnEnter() {}

// OnLeave implements Scene.
func (sc *StationScene) OnLeave() {}

// Undock chrome metrics (40 px minimum hit area).
const (
	undockW   = 160.0
	undockH   = 40.0
	undockGap = 10.0
)

// UndockRect returns the undock button rect below the station panel.
func UndockRect(reg ui.Regions) ui.Rect {
	return ui.Rect{
		X: reg.Station.X + reg.Station.W/2 - undockW/2,
		Y: reg.Station.Bottom() + undockGap,
		W: undockW,
		H: undockH,
	}
}

// Update routes input to the UI and handles the undock button.
func (sc *StationScene) Update(in ui.Input) error {
	if in.JustClicked[0] && UndockRect(sc.g.u.Layout()).Contains(in.MouseX, in.MouseY) {
		sc.g.s.Queue(sim.CmdUndock{})
		return nil
	}
	return sc.g.u.Update(in)
}

// Draw renders the undock button (panel is drawn by the UI pass).
func (sc *StationScene) Draw(dst *ebiten.Image) {
	ui.Button(dst, UndockRect(sc.g.u.Layout()), "Undock", true)
}

// ---------------------------------------------------------------------------
// MapScene (starmap overlay)
// ---------------------------------------------------------------------------

// MapScene shows the universe graph as a movable, closable window; travel
// itself happens at stargates. The window state (position, [×], z-order)
// lives in the UI.
type MapScene struct{ g *Game }

// OnEnter implements Scene.
func (sc *MapScene) OnEnter() {}

// OnLeave implements Scene (the window follows the scene: closing the
// scene closes the window, and vice versa via Update below).
func (sc *MapScene) OnLeave() { sc.g.u.SetWindowOpen(ui.WinStarmap, false) }

// Update handles window chrome first, then starmap node clicks.
func (sc *MapScene) Update(in ui.Input) error {
	u := sc.g.u
	if !u.WindowOpen(ui.WinStarmap) {
		sc.g.pop() // closed with the [×] button
		return nil
	}
	if u.OverlayInput(in, ui.WinStarmap) {
		return nil // title-bar drag / close consumed the press
	}
	if in.JustClicked[0] {
		u.ClickStarmap(in.MouseX, in.MouseY)
	}
	return nil
}

// Draw renders the starmap over a dimmed screen.
func (sc *MapScene) Draw(dst *ebiten.Image) {
	dimScreen(dst, sc.g.w, sc.g.h, 170)
	sc.g.u.DrawStarmap(dst)
}

// ---------------------------------------------------------------------------
// HelpScene (key map overlay)
// ---------------------------------------------------------------------------

// HelpScene shows the full key map from KeyMap (the single source shared
// with docs/GAMEPLAY.md) as a movable, closable window. It does not pause
// the simulation.
type HelpScene struct{ g *Game }

// OnEnter implements Scene.
func (sc *HelpScene) OnEnter() {}

// OnLeave implements Scene.
func (sc *HelpScene) OnLeave() { sc.g.u.SetWindowOpen(ui.WinHelp, false) }

// Update: closed globally via H or Esc; the title bar drags.
func (sc *HelpScene) Update(in ui.Input) error {
	u := sc.g.u
	if !u.WindowOpen(ui.WinHelp) {
		sc.g.pop() // closed with the [×] button
		return nil
	}
	u.OverlayInput(in, ui.WinHelp)
	return nil
}

// Help overlay line metrics (derived from the ui typography scale).
const (
	helpGap   = 10.0
	helpPitch = ui.FontTiny // line pitch helper: LineH(FontTiny)
)

// Draw renders the key map in two columns plus credits.
func (sc *HelpScene) Draw(dst *ebiten.Image) {
	g := sc.g
	dimScreen(dst, g.w, g.h, 180)
	u := g.u
	panel := u.WindowRect(ui.WinHelp, u.Layout())
	content := ui.DrawWindow(dst, panel, ui.WindowTitle(ui.WinHelp), true)

	pitch := ui.LineH(helpPitch)
	perCol := (len(KeyMap) + 1) / 2
	colW := (content.W - 3*helpGap) / 2
	y := content.Y + helpGap
	for i, b := range KeyMap {
		col, row := i/perCol, i%perCol
		x := content.X + helpGap + float64(col)*(colW+helpGap)
		ui.Text(dst, b.Desc, x, y+float64(row)*pitch, ui.FontTiny, ui.ColText)
	}
	y += float64(perCol)*pitch + helpGap

	ui.Text(dst, "Mouse", content.X+helpGap, y, ui.FontTiny, ui.ColAccent)
	y += pitch
	for _, b := range MouseMap {
		line := b.Button
		if b.Mod != "" {
			line += " (" + b.Mod + ")"
		}
		line += " — " + b.Desc
		ui.Text(dst, line, content.X+helpGap, y, ui.FontTiny, ui.ColText)
		y += pitch
	}
}

// ---------------------------------------------------------------------------
// MenuScene (Esc)
// ---------------------------------------------------------------------------

var menuItems = []string{"Resume", "Save", "Load", "Quit"}

// Menu chrome metrics.
const (
	menuBtnW = 220.0
	menuBtnH = 40.0 // minimum hit area
	menuGap  = 10.0
	menuHead = 56.0 // room for the title bar + margin
	menuFoot = 44.0
)

// MenuScene is the small Esc menu: Resume / Save / Load / Quit.
type MenuScene struct {
	g    *Game
	mx   float64
	my   float64
	down bool
}

// OnEnter implements Scene.
func (sc *MenuScene) OnEnter() {}

// OnLeave implements Scene.
func (sc *MenuScene) OnLeave() {}

// menuRects lays out the menu buttons centered on screen.
func (sc *MenuScene) menuRects() []ui.Rect {
	total := float64(len(menuItems))*menuBtnH + menuGap*float64(len(menuItems)-1)
	y := float64(sc.g.h)/2 - total/2
	out := make([]ui.Rect, len(menuItems))
	for i := range out {
		out[i] = ui.Rect{X: float64(sc.g.w)/2 - menuBtnW/2, Y: y, W: menuBtnW, H: menuBtnH}
		y += menuBtnH + menuGap
	}
	return out
}

// Update handles menu button clicks and tracks the pointer for hover
// feedback in Draw.
func (sc *MenuScene) Update(in ui.Input) error {
	sc.mx, sc.my, sc.down = in.MouseX, in.MouseY, in.MouseButton[0]
	if !in.JustClicked[0] {
		return nil
	}
	for i, r := range sc.menuRects() {
		if !r.Contains(in.MouseX, in.MouseY) {
			continue
		}
		switch menuItems[i] {
		case "Resume":
			sc.g.pop()
		case "Save":
			sc.g.report(sc.g.SaveGame(), "Game saved")
		case "Load":
			sc.g.report(sc.g.LoadGame(), "Game loaded")
		case "Quit":
			return ebiten.Termination
		}
		return nil
	}
	return nil
}

// Draw renders the dimmed menu.
func (sc *MenuScene) Draw(dst *ebiten.Image) {
	g := sc.g
	rects := sc.menuRects()
	top := rects[0].Y - menuHead
	bottom := rects[len(rects)-1].Bottom() + menuFoot
	dimScreen(dst, g.w, g.h, 170)
	panel := ui.Rect{X: rects[0].X - helpGap, Y: top, W: rects[0].W + 2*helpGap, H: bottom - top}
	ui.Panel(dst, panel)
	ui.PanelTitle(dst, panel, "Menu — "+g.SavePath())
	for i, r := range rects {
		hover := r.Contains(sc.mx, sc.my)
		ui.ButtonState(dst, r, menuItems[i], true, hover, hover && sc.down)
	}
}
