package game

import (
	"image"
	"image/color"

	eimage "github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit"
	"github.com/entieve/entieve/uikit/icons"
	"github.com/hajimehoshi/ebiten/v2"
)

// Window chrome metrics for the game-built windows (menu, station):
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
// and the station chrome (Market / Inventory / Fitting / Undock) rides the
// uikit window stack. World input is off while it is on top.
type StationScene struct{ g *Game }

// stationPanels are the windows that auto-open on dock.
var stationPanels = []panelID{pMarket, pInventory, pFitting}

// isStationPanel reports whether p is one of the dock-forced services.
func isStationPanel(p panelID) bool {
	for _, s := range stationPanels {
		if s == p {
			return true
		}
	}
	return false
}

// OnEnter implements Scene: remember which station windows were open and
// surface the station services. The factory is the industry service's
// panel: it surfaces on its own when the docked station offers industry
// (B toggles it from there on).
func (sc *StationScene) OnEnter() {
	g := sc.g
	for _, p := range stationPanels {
		g.stationPrior[p] = g.open[p]
		g.open[p] = true
	}
	if g.s.DockedIndustry() != "" {
		g.open[pFactory] = true
	}
}

// OnLeave implements Scene: restore the pre-dock window state (windows
// that were closed before docking close again; already-open ones stay).
func (sc *StationScene) OnLeave() {
	g := sc.g
	for _, p := range stationPanels {
		if !g.stationPrior[p] {
			g.open[p] = false
		}
	}
}

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
	p := g.env.Pal
	title := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BGHard), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)
	title.AddChild(widget.NewText(
		widget.TextOpts.Text(label, g.env.FaceBold(uikit.StepBase), solid(p.Accent)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionStart,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
			Padding:            widget.NewInsetsSimple(5),
		})),
	))
	return title
}

// newBtn builds a themed button (the kit's PrimaryTheme styles it once
// the window is mounted).
func (g *Game) newBtn(label string, minW, minH int, clicked func()) *widget.Button {
	return widget.NewButton(
		widget.ButtonOpts.TextLabel(label),
		widget.ButtonOpts.WidgetOpts(widget.WidgetOpts.MinSize(minW, minH)),
		widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) { clicked() }),
	)
}

// newIconBtn is newBtn with a 20 px icon beside the label (ebitenui's
// TextAndImage path — the graphic trails the label, the theme's image
// states are untouched, and the disabled icon draws at 40%).
func (g *Game) newIconBtn(label, icon string, minW, minH int, clicked func()) *widget.Button {
	p := g.env.Pal
	dim := color.RGBA{R: p.Dim.R, G: p.Dim.G, B: p.Dim.B, A: uikit.IconDisabledA}
	return widget.NewButton(
		widget.ButtonOpts.TextAndImage(label, nil, &widget.GraphicImage{
			Idle:     icons.Get(icon, uikit.IconButton, p.Text),
			Disabled: icons.Get(icon, uikit.IconButton, dim),
		}, nil),
		widget.ButtonOpts.WidgetOpts(widget.WidgetOpts.MinSize(minW, minH)),
		widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) { clicked() }),
	)
}

// buildMenuWindow constructs the Esc menu window: modal so no click leaks
// past it while it is open.
func (g *Game) buildMenuWindow() *widget.Window {
	p := g.env.Pal
	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BG), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(10),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(10)),
		)),
	)
	actions := map[string]func(){
		"Resume": func() { g.pop() },
		"Save":   func() { g.report(g.SaveGame(), "Game saved") },
		"Load":   func() { g.pendingLoad = true },
		"Quit":   func() { g.quit = true },
	}
	for _, item := range menuItems {
		item := item
		body.AddChild(g.newBtn(item, menuBtnW, menuBtnH, actions[item]))
	}
	return widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(g.titleBar("Menu"), winTitleH),
		widget.WindowOpts.Draggable(),
		widget.WindowOpts.Modal(),
		widget.WindowOpts.Location(image.Rect(0, 0,
			int(menuBtnW)+20, int(4*menuBtnH+3*10+20)+winTitleH)),
	)
}

// buildStationWindow constructs the docked station strip: the station
// identity header (name + Undock) over the two contract-board sections —
// Available contracts (this station's board) and Active contracts (every
// accepted contract). The sections' rows are laid out by boardRowRects
// (contracts.go); syncStationBoard rebuilds them and hugs the window to
// the content height.
func (g *Game) buildStationWindow() *widget.Window {
	p := g.env.Pal
	g.stName = widget.NewText(
		widget.TextOpts.Text("Docked", g.env.Face(uikit.StepSmall), solid(p.Text)),
	)
	undock := g.newIconBtn("Undock", "act_undock", 150, menuBtnH, func() {
		g.s.Queue(sim.CmdUndock{})
	})
	header := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(10),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	// The station strip is the contracts service's face: its leading glyph
	// is the only "title" this window has (it owns no title bar).
	glyph := uikit.NewIcon("svc_contracts", uikit.IconTitle, p.Accent)
	glyph.GetWidget().LayoutData = widget.RowLayoutData{Position: widget.RowLayoutPositionCenter}
	header.AddChild(glyph, g.stName, undock)

	g.stAvail = g.newBoardSection()
	g.stActive = g.newBoardSection()
	availHead := widget.NewText(
		widget.TextOpts.Text("Available contracts", g.env.Face(uikit.StepSmall), solid(p.Accent)),
	)
	actHead := widget.NewText(
		widget.TextOpts.Text("Active contracts", g.env.Face(uikit.StepSmall), solid(p.Accent)),
	)

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BG), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(boardSpacing),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(boardPad)),
		)),
	)
	body.AddChild(header, availHead, g.stAvail, actHead, g.stActive)
	g.stBody = body
	g.stModel, g.stAvailRows, g.stActRows = stationBoardModel{}, nil, nil // fresh window: rebuild on next sync

	win := widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.MinSize(stationMinW, stationMinH),
		widget.WindowOpts.Location(image.Rect(stationX, stationY, stationX+stationWinW, stationY+stationMinH)),
	)
	// Bind the theme now: syncStationBoard sizes the window from the body's
	// preferred size before the window is first mounted, and ebitenui
	// validates (and themes) buttons during that measurement. Same palette
	// and type scale as the kit's own theme.
	win.GetContainer().GetWidget().SetTheme(g.env.Theme)
	return win
}

// newBoardSection builds one contract-row section: a bare container whose
// rows are placed by boardRowRects through the boardLayout layouter.
func (g *Game) newBoardSection() *widget.Container {
	return widget.NewContainer(
		widget.ContainerOpts.Layout(boardLayout{}),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
}

// syncStationLabel refreshes the station strip's identity line.
func (g *Game) syncStationLabel() {
	name := "Station"
	if d := g.s.System(); d != nil {
		if at, ok := g.s.Docked.Get(g.s.Player()); ok {
			for _, cel := range d.Celestials {
				if cel.ID == at.At {
					name = cel.Name
					break
				}
			}
		}
	}
	label := "Docked at " + name
	if g.stName.Label != label {
		g.stName.Label = label
	}
}
