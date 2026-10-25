package game

import (
	"image"
	"image/color"
	"path/filepath"

	eimage "github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/render"
	uikit "github.com/entieve/entieve/uikit/screens"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
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

// stationFadeFrames is the docked backdrop's fade-in length in frames at
// 60 TPS — the house window open fade's 12 (uikit/screens/winanim.go),
// counted in Update.
const stationFadeFrames = 12

// stationVeilAlpha is the black veil over the world while docked: the
// pre-artwork look, kept verbatim as the fallback when the asset root ships
// no artwork/ plate (release builds don't carry assets/).
const stationVeilAlpha = 150

// StationScene shows the station view: the docked hangar artwork fades in
// over the world (or, with no artwork, the legacy veil dims it) and the
// services view (identity / Undock / Cargo / services, over the contract
// board) rides the uikit window stack. World input is off while it is on
// top.
type StationScene struct {
	g    *Game
	fade int // frames since entry, 0..stationFadeFrames
}

// OnEnter implements Scene: docking opens exactly one window — the
// station services view. The market, fitting and inventory stay closed
// unless the player opened them; they are ordinary windows now, opening
// at their own per-type defaults when the player does. The backdrop starts
// its fade-in from the world.
func (sc *StationScene) OnEnter() {
	sc.fade = 0
	sc.g.open[pServices] = true
}

// OnLeave implements Scene: undocking closes the services view and
// leaves every player-opened window exactly as it was. The frame is
// removed with it, so the artwork cuts straight back to the world — the
// fade is entry-only by design.
func (sc *StationScene) OnLeave() { sc.g.open[pServices] = false }

// Update advances the backdrop's fade-in and nothing else: world input
// stays off because this scene is on top. The counter settles at full and
// holds (a modal above it pauses the fade — accepted).
func (sc *StationScene) Update(in Input) error {
	if sc.fade < stationFadeFrames {
		sc.fade++
	}
	return nil
}

// stationBackdrop is the docked backdrop's plate plus the veil alpha used
// when it has none: the artwork when assets/artwork/ carries one, else
// (nil, stationVeilAlpha) — today's veil, byte for byte.
func stationBackdrop() (*ebiten.Image, uint8) {
	if art := render.DockedArtwork(); art != nil {
		return art, 0
	}
	return nil, stationVeilAlpha
}

// Draw paints the docked backdrop. It runs as this scene's Draw — after the
// world scene, before kit.Draw — so every window and the rail paint above
// it, and the services view stays readable over the plate.
func (sc *StationScene) Draw(dst *ebiten.Image) {
	art, veil := stationBackdrop()
	if art == nil {
		dimScreen(dst, sc.g.w, sc.g.h, veil)
		return
	}
	sc.drawArtwork(dst, art)
}

// drawArtwork draws the plate cover-fit (aspect kept, centred, overflow
// cropped) at the current fade, then its caption.
func (sc *StationScene) drawArtwork(dst *ebiten.Image, art *ebiten.Image) {
	a := backdropAlpha(sc.fade)
	b := art.Bounds()
	w, h := sc.g.w, sc.g.h
	s := coverScale(b.Dx(), b.Dy(), w, h)
	if s <= 0 {
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(-float64(b.Dx())/2, -float64(b.Dy())/2)
	op.GeoM.Scale(s, s)
	op.GeoM.Translate(float64(w)/2, float64(h)/2)
	op.ColorScale.ScaleAlpha(a)
	dst.DrawImage(art, op)
	sc.drawDockedTitle(dst, a)
}

// backdropAlpha maps the scene's fade counter to the backdrop's opacity:
// linear from the world (0) to the plate (1) over stationFadeFrames, then
// pinned at 1.
func backdropAlpha(frame int) float32 {
	if frame <= 0 {
		return 0
	}
	if frame >= stationFadeFrames {
		return 1
	}
	return float32(frame) / stationFadeFrames
}

// coverScale is the cover fit of a sw×sh plate into a w×h window, delegated to
// render's one implementation (the docked plate and the art loader agree).
func coverScale(sw, sh, w, h int) float64 { return render.CoverScale(sw, sh, w, h) }

// dockedTitle is the backdrop's caption line (NAMING.md interface strings).
func dockedTitle(station string) string { return "Docked: " + station }

// dockedTitleY is the caption's top edge: 7/8 down the window, the
// lower-centre band clear of the window row and the log.
func dockedTitleY(h int) float64 { return float64(h) * 7 / 8 }

// dockedTitleX centres a textW-wide caption in a w-wide window.
func dockedTitleX(w int, textW float64) float64 { return (float64(w) - textW) / 2 }

// drawDockedTitle draws the caption in the house display face (Plex Bold,
// StepLarge): a dark offset shadow keeps it readable over any plate, and a
// short accent rule ties it to the chrome. Both fade with the plate.
func (sc *StationScene) drawDockedTitle(dst *ebiten.Image, a float32) {
	info, ok := sc.g.env.DockedStation()
	if !ok || info.Name == "" {
		return
	}
	p := sc.g.env.Pal()
	face := *sc.g.env.FaceBold(theme.StepLarge)
	title := dockedTitle(info.Name)
	tw, th := text.Measure(title, face, 0)
	x, y := dockedTitleX(sc.g.w, tw), dockedTitleY(sc.g.h)

	draw := func(dx, dy float64, col color.RGBA) {
		opts := &text.DrawOptions{}
		opts.GeoM.Translate(x+dx, y+dy)
		opts.ColorScale.ScaleWithColor(col)
		opts.ColorScale.ScaleAlpha(a)
		text.Draw(dst, title, face, opts)
	}
	shadow := theme.Px(2)
	draw(float64(shadow), float64(shadow), color.RGBA{A: 200})
	draw(0, 0, p.Text)
	rule := p.Accent
	rule.A = uint8(200 * a)
	vector.DrawFilledRect(dst, float32(x), float32(y+th+float64(theme.Px(4))),
		float32(tw), float32(theme.Px(1)), rule, false)
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
var menuItems = []string{"Resume", "Save", "Save as…", "Load", "Quit"}

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
		"Save as…": func() {
			err := g.SaveAs()
			g.report(err, "Saved as "+filepath.Base(g.activeSavePath()))
		},
		"Load": func() { g.Do(widgets.Load{}) },
		"Quit": func() { g.Do(widgets.Quit{}) },
	}
	for _, item := range menuItems {
		item := item
		body.AddChild(g.newBtn(item, menuBtnW, menuBtnH, actions[item]))
	}
	locH := theme.Px(len(menuItems)*int(menuBtnH) + (len(menuItems)-1)*10 + 20 + winTitleH)
	return widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(g.titleBar("Menu"), theme.Px(winTitleH)),
		widget.WindowOpts.Modal(),
		widget.WindowOpts.Location(image.Rect(0, 0, theme.Px(int(menuBtnW)+20), locH)),
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
