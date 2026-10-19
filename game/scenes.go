package game

import (
	"image"
	"image/color"

	eimage "github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	uikit "github.com/entieve/entieve/uikit/screens"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// Window chrome metrics for the game-built windows (menu, services):
// mirrored from the uikit title-bar scale so they read as one theme.
const (
	winTitleH = 44
	menuBtnW  = 220
	menuBtnH  = 40 // minimum hit area
)

// nrgba converts a palette RGBA to an NRGBA with the given alpha.
func nrgba(c color.RGBA, a uint8) color.NRGBA {
	return color.NRGBA{R: c.R, G: c.G, B: c.B, A: a}
}

// solid converts a palette RGBA to a full-alpha NRGBA nine-slice color.
func solid(c color.RGBA) color.NRGBA { return nrgba(c, c.A) }

// bordered builds the body+edge nine-slice every uikit panel uses.
func bordered(body, edge color.NRGBA, w int) *eimage.NineSlice {
	return eimage.NewBorderedNineSliceColor(body, edge, w)
}

// ---------------------------------------------------------------------------
// StationScene (pushed while docked)
// ---------------------------------------------------------------------------

// StationScene shows the station view: the world stays veiled behind it
// and the services view (identity / Undock / Cargo / services, over the
// contract board) rides the uikit window stack. World input is off while
// it is on top.
type StationScene struct{ g *Game }

// OnEnter implements Scene: docking opens exactly one window — the
// station services view. The market, fitting and inventory stay closed
// unless the player opened them; they are ordinary windows now, opening
// at their own per-type defaults when the player does.
func (sc *StationScene) OnEnter() { sc.g.open[pServices] = true }

// OnLeave implements Scene: undocking closes the services view and
// leaves every player-opened window exactly as it was.
func (sc *StationScene) OnLeave() { sc.g.open[pServices] = false }

// Update is a no-op: buttons live in the interface, world input stays off
// because this scene is on top.
func (sc *StationScene) Update(in Input) error { return nil }

// Draw veils the world behind the station view (legacy alpha).
func (sc *StationScene) Draw(dst *ebiten.Image) {
	dimScreen(dst, sc.g.w, sc.g.h, 150)
}

// ---------------------------------------------------------------------------
// MapScene (starmap overlay)
// ---------------------------------------------------------------------------

// MapScene shows the MapView window over a dimmed screen. The window is
// modal, so no click leaks to the world or the panels underneath.
type MapScene struct{ g *Game }

// OnEnter implements Scene (the window follows the scene).
func (sc *MapScene) OnEnter() { sc.g.open[pMap] = true }

// OnLeave implements Scene: closing the scene closes the window.
func (sc *MapScene) OnLeave() { sc.g.open[pMap] = false }

// Update is a no-op: N / Esc close the scene, clicks stay in the window.
func (sc *MapScene) Update(in Input) error { return nil }

// Draw renders the dimmed backdrop; the window itself is drawn by the kit.
func (sc *MapScene) Draw(dst *ebiten.Image) {
	dimScreen(dst, sc.g.w, sc.g.h, 170)
}

// ---------------------------------------------------------------------------
// HelpScene (key map overlay)
// ---------------------------------------------------------------------------

// HelpScene shows the key map from KeyMap (the single source shared with
// docs/GAMEPLAY.md) as a modal, closable window. It does not pause the
// simulation.
type HelpScene struct{ g *Game }

// OnEnter implements Scene.
func (sc *HelpScene) OnEnter() { sc.g.open[pHelp] = true }

// OnLeave implements Scene.
func (sc *HelpScene) OnLeave() { sc.g.open[pHelp] = false }

// Update is a no-op: H / Esc close the scene.
func (sc *HelpScene) Update(in Input) error { return nil }

// Draw renders the dimmed backdrop; the window is drawn by the kit.
func (sc *HelpScene) Draw(dst *ebiten.Image) {
	dimScreen(dst, sc.g.w, sc.g.h, 180)
}

// ---------------------------------------------------------------------------
// MenuScene (Esc)
// ---------------------------------------------------------------------------

// menuItems are the Esc menu buttons, in draw order.
var menuItems = []string{"Resume", "Save", "Load", "Quit"}

// MenuScene is the Esc menu: a small modal uikit window with Resume /
// Save / Load / Quit. Scene and window toggle together.
type MenuScene struct{ g *Game }

// OnEnter implements Scene: mounts the menu window.
func (sc *MenuScene) OnEnter() { sc.g.open[pMenu] = true }

// OnLeave implements Scene: unmounts the menu window.
func (sc *MenuScene) OnLeave() { sc.g.open[pMenu] = false }

// Update is a no-op: the buttons run inside Kit.Update.
func (sc *MenuScene) Update(in Input) error { return nil }

// Draw dims the screen behind the menu window.
func (sc *MenuScene) Draw(dst *ebiten.Image) {
	dimScreen(dst, sc.g.w, sc.g.h, 170)
}

// ---------------------------------------------------------------------------
// Game-built windows (menu, station)
// ---------------------------------------------------------------------------

// titleBar builds a uikit-style title bar for a game-owned window.
func (g *Game) titleBar(label string) *widget.Container {
	p := g.env.Pal()
	title := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BGHard), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)
	title.AddChild(widget.NewText(
		widget.TextOpts.Text(label, g.env.FaceBold(theme.StepBase), solid(p.Accent)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionStart,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
			Padding:            widget.NewInsetsSimple(theme.Px(5)),
		})),
	))
	return title
}

// newBtn builds a themed button (the kit's PrimaryTheme styles it once
// the window is mounted).
func (g *Game) newBtn(label string, minW, minH int, clicked func()) *widget.Button {
	return widget.NewButton(
		widget.ButtonOpts.TextLabel(label),
		// minW/minH arrive as DESIGN units (callers pass the raw consts).
		widget.ButtonOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(minW), theme.Px(minH))),
		widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) { clicked() }),
	)
}

// buildMenuWindow constructs the Esc menu window: modal so no click leaks
// past it while it is open.
func (g *Game) buildMenuWindow() *widget.Window {
	p := g.env.Pal()
	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(theme.WindowBody(p.BG), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(10)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(10))),
		)),
	)
	// The menu's verbs are intents too: Resume closes the menu screen
	// itself (Toggle{Menu}), the rest are the game-level intents Game.Do
	// executes — one dispatch path for buttons and screens alike.
	actions := map[string]func(){
		"Resume": func() { g.Do(widgets.Toggle{Screen: "Menu"}) },
		"Save":   func() { g.Do(widgets.Save{}) },
		"Load":   func() { g.Do(widgets.Load{}) },
		"Quit":   func() { g.Do(widgets.Quit{}) },
	}
	for _, item := range menuItems {
		item := item
		body.AddChild(g.newBtn(item, menuBtnW, menuBtnH, actions[item]))
	}
	return widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(g.titleBar("Menu"), theme.Px(winTitleH)),
		widget.WindowOpts.Modal(),
		widget.WindowOpts.Location(image.Rect(0, 0,
			theme.Px(int(menuBtnW)+20), theme.Px(int(4*menuBtnH+3*10+20)+winTitleH))),
	)
}

// buildServicesWindow constructs the station services view — the ONE
// window docking opens: uikit.NewServices' service row (docked-at
// identity, Undock, Cargo and one button per service this station
// offers) over the two contract-board sections — Available contracts
// (this station's board) and Active contracts (every accepted contract).
// The sections' rows are laid out by boardRowRects (contracts.go);
// syncStationBoard rebuilds them and hugs the window to the content
// height. The window owns no title bar — the service row is its header,
// exactly as tall as the strip's header was, so the column's worst-case
// height (and its clearance over the log) is unchanged.
func (g *Game) buildServicesWindow() *widget.Window {
	p := g.env.Pal()
	g.svcUI = uikit.NewServices(g.env, boardInnerW)

	g.stAvail = g.newBoardSection()
	g.stActive = g.newBoardSection()
	availHead := widget.NewText(
		widget.TextOpts.Text("Available contracts", g.env.Face(theme.StepSmall), solid(p.Accent)),
	)
	actHead := widget.NewText(
		widget.TextOpts.Text("Active contracts", g.env.Face(theme.StepSmall), solid(p.Accent)),
	)

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(theme.WindowBody(p.BG), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(boardSpacing)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(boardPad))),
		)),
	)
	body.AddChild(g.svcUI.Container(), availHead, g.stAvail, actHead, g.stActive)
	g.stBody = body
	g.stModel, g.stAvailRows, g.stActRows = stationBoardModel{}, nil, nil // fresh window: rebuild on next sync

	win := widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.MinSize(theme.Px(stationMinW), theme.Px(stationMinH)),
		widget.WindowOpts.Location(image.Rect(theme.Px(stationX), theme.Px(stationY), theme.Px(stationX+stationWinW), theme.Px(stationY+stationMinH))),
	)
	// Bind the theme now: syncStationBoard sizes the window from the body's
	// preferred size before the window is first mounted, and ebitenui
	// validates (and themes) buttons during that measurement. Same palette
	// and type scale as the kit's own theme.
	win.GetContainer().GetWidget().SetTheme(g.env.WidgetTheme())
	g.svcWin = win
	return win
}

// newBoardSection builds one contract-row section: a bare container
// whose rows are tiled by the table module's row stack — equal heights,
// boardRowGap apart, the pure pin of which is boardRowRects.
func (g *Game) newBoardSection() *widget.Container {
	return widget.NewContainer(
		widget.ContainerOpts.Layout(widgets.StackLayout{Gap: theme.Px(boardRowGap)}),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
}
