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

// UndockRect returns the undock button rect below the station panel.
func UndockRect(reg ui.Regions) ui.Rect {
	return ui.Rect{X: reg.Station.X + reg.Station.W/2 - 70, Y: reg.Station.Bottom() + 8, W: 140, H: 30}
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

// MapScene shows the universe graph; travel itself happens at stargates.
type MapScene struct{ g *Game }

// OnEnter implements Scene.
func (sc *MapScene) OnEnter() {}

// OnLeave implements Scene.
func (sc *MapScene) OnLeave() {}

// Update handles starmap clicks.
func (sc *MapScene) Update(in ui.Input) error {
	if in.JustClicked[0] {
		sc.g.u.ClickStarmap(in.MouseX, in.MouseY)
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
// with docs/GAMEPLAY.md). It does not pause the simulation.
type HelpScene struct{ g *Game }

// OnEnter implements Scene.
func (sc *HelpScene) OnEnter() {}

// OnLeave implements Scene.
func (sc *HelpScene) OnLeave() {}

// Update: closed globally via H or Esc.
func (sc *HelpScene) Update(in ui.Input) error { return nil }

// Draw renders the key map in two columns plus credits.
func (sc *HelpScene) Draw(dst *ebiten.Image) {
	g := sc.g
	dimScreen(dst, g.w, g.h, 180)
	ph := float64(g.h) - 80
	if ph < 300 {
		ph = 300
	}
	panel := ui.Rect{X: float64(g.w)/2 - 290, Y: 40, W: 580, H: ph}
	ui.Panel(dst, panel)
	ui.PanelTitle(dst, panel, "Help — Entieve key map")

	const perCol = 18
	y := panel.Y + 30
	for i, b := range KeyMap {
		col, row := i/perCol, i%perCol
		x := panel.X + 12 + float64(col)*282
		ui.Text(dst, b.Desc, x, y+float64(row)*15, 11, ui.ColText)
	}
	y += perCol*15 + 8

	ui.Text(dst, "Mouse", panel.X+12, y, 11, ui.ColAccent)
	y += 16
	for _, b := range MouseMap {
		line := b.Button
		if b.Mod != "" {
			line += " (" + b.Mod + ")"
		}
		line += " — " + b.Desc
		ui.Text(dst, line, panel.X+12, y, 11, ui.ColText)
		y += 15
	}
	y += 8
	ui.Text(dst, "Entieve — a single-player 2D space sandbox, a love letter to EVE Online.",
		panel.X+12, y, 11, ui.ColDim)
	y += 15
	ui.Text(dst, "Currency: credits (cr). Fly, fight, mine, fit, trade and jump the gate network.",
		panel.X+12, y, 11, ui.ColDim)
}

// ---------------------------------------------------------------------------
// MenuScene (Esc)
// ---------------------------------------------------------------------------

var menuItems = []string{"Resume", "Save", "Load", "Quit"}

// MenuScene is the small Esc menu: Resume / Save / Load / Quit.
type MenuScene struct{ g *Game }

// OnEnter implements Scene.
func (sc *MenuScene) OnEnter() {}

// OnLeave implements Scene.
func (sc *MenuScene) OnLeave() {}

// menuRects lays out the menu buttons centered on screen.
func (sc *MenuScene) menuRects() []ui.Rect {
	const bw, bh, gap = 220.0, 36.0, 10.0
	total := float64(len(menuItems))*bh + gap*float64(len(menuItems)-1)
	y := float64(sc.g.h)/2 - total/2
	out := make([]ui.Rect, len(menuItems))
	for i := range out {
		out[i] = ui.Rect{X: float64(sc.g.w)/2 - bw/2, Y: y, W: bw, H: bh}
		y += bh + gap
	}
	return out
}

// Update handles menu button clicks.
func (sc *MenuScene) Update(in ui.Input) error {
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
	top := rects[0].Y - 56
	bottom := rects[len(rects)-1].Bottom() + 44
	dimScreen(dst, g.w, g.h, 170)
	panel := ui.Rect{X: rects[0].X - 24, Y: top, W: rects[0].W + 48, H: bottom - top}
	ui.Panel(dst, panel)
	ui.PanelTitle(dst, panel, "Menu — "+g.SavePath())
	for i, r := range rects {
		ui.Button(dst, r, menuItems[i], true)
	}
}
