// Package game wires the Entieve client together: Ebitengine window and
// loop, fixed-timestep sim stepping with time acceleration, the scene stack,
// input mapping, save/load, and the uikit interface host (the ebitenui-based
// window stack: Overview, Selected Item, HUD, module rack, log, scanner,
// map, help, fitting, inventory, market, station and menu windows).
package game

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"
	"strings"

	enuiinput "github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Default window geometry. 1024×700 is the minimum supported window: the
// layout tests pin every property at 1280×720 AND 1024×700.
const (
	defaultW = 1280
	defaultH = 720
	minW     = 1024
	minH     = 700
)

// defaultSeed seeds a fresh simulation (replays/saves pin their own seed).
const defaultSeed = int64(1)

// Options configures a game session.
type Options struct {
	Content  *content.Content  // required: loaded game data
	World    *content.WorldDef // optional: defaults to world-default
	Seed     int64             // sim RNG seed (0 = default seed)
	Save     *sim.Save         // optional: restore on boot (--load)
	Title    string            // window title (default "Entieve")
	SavePath string            // F5/F9 path (default: user config dir)
}

// panelID identifies one toggleable interface window. The always-on
// chrome (Overview, Selected Item, HUD, rack) and the docked station
// strip are managed separately.
type panelID int

const (
	pInventory panelID = iota
	pFitting
	pMarket
	pScanner
	pMap
	pHelp
	pLog
	pMenu
	numPanels
)

// Game implements ebiten.Game and hosts the uikit interface.
type Game struct {
	opts  Options
	s     *sim.Sim
	ren   *render.Renderer
	clock Clock

	// kit owns the interface (Overview + Selected Item are built into it);
	// env is the panel environment the extra panels are constructed from.
	kit *uikit.Kit
	env uikit.Env

	// Anchored chrome panels, mounted directly on the kit's root overlay
	// (Kit.AddContainer); the HUD and rack are hidden — not unmounted —
	// while docked.
	hud  *uikit.Hud
	rack *uikit.Rack

	// Window panels (the log joined them: pLog below).
	logP *uikit.LogPanel
	inv  *uikit.Inventory
	fitt *uikit.Fitting
	mkt  *uikit.Market
	scan *uikit.Scanner
	mp   *uikit.MapView
	help *uikit.HelpPanel

	menuWin  *widget.Window
	stationW *widget.Window
	stName   *widget.Text

	// The docked station strip is the only game-built strip that still
	// rides the window stack (the chrome above lives on the root overlay).
	stMount widget.RemoveWindowFunc
	stShown bool

	// open is the player-visible state of each toggleable window;
	// mounted is its current AddWindow handle (nil = unmounted).
	open    [numPanels]bool
	mounted [numPanels]widget.RemoveWindowFunc

	// Selection lives in the kit: world clicks, the Tab cycle-lock and
	// the overview all feed Kit.Select, the game reads Kit.Selected.
	tabIdx       int
	orbitIdx     int
	stationPrior [numPanels]bool

	pendingLoad bool
	quit        bool

	scenes []Scene
	space  *SpaceScene
	w, h   int // logical screen size (Layout)
	sized  bool
}

// New builds a game session from options.
func New(opts Options) (*Game, error) {
	if opts.Content == nil {
		return nil, errors.New("game: Options.Content is required")
	}
	world := opts.World
	if world == nil {
		world = pickWorld(opts.Content)
		if world == nil {
			return nil, errors.New("game: no world available (content has none)")
		}
	}
	seed := opts.Seed
	if seed == 0 {
		seed = defaultSeed
	}
	s, err := sim.New(opts.Content, world, seed)
	if err != nil {
		return nil, fmt.Errorf("game: sim.New: %w", err)
	}
	if opts.Save != nil {
		if err := s.Restore(opts.Save); err != nil {
			return nil, fmt.Errorf("game: restore save: %w", err)
		}
	}
	g := &Game{
		opts: opts,
		s:    s,
		ren:  render.New(opts.Content),
		w:    defaultW,
		h:    defaultH,
	}
	// The log is open by default (it used to be non-closable chrome);
	// the saved layout may override this once the first screen size
	// lands (see applyLayout).
	g.open[pLog] = true
	if err := g.buildKit(); err != nil {
		return nil, err
	}
	g.space = &SpaceScene{g: g, zoom: 1}
	g.scenes = []Scene{g.space}
	g.space.OnEnter()
	return g, nil
}

// pickWorld chooses world-default, else the lowest world ID (deterministic).
func pickWorld(c *content.Content) *content.WorldDef {
	if w, ok := c.Worlds["world-default"]; ok {
		return w
	}
	ids := make([]string, 0, len(c.Worlds))
	for id := range c.Worlds {
		ids = append(ids, string(id))
	}
	if len(ids) == 0 {
		return nil
	}
	sort.Strings(ids)
	return c.Worlds[content.WorldID(ids[0])]
}

// Run opens the window and plays until Quit.
func (g *Game) Run() error {
	title := g.opts.Title
	if title == "" {
		title = "Entieve"
	}
	ebiten.SetWindowTitle(title)
	ebiten.SetWindowSize(defaultW, defaultH)
	ebiten.SetWindowResizable(true)
	ebiten.SetVsyncEnabled(true)
	return ebiten.RunGame(g)
}

// Layout returns the logical screen size (clamped to the layout minimum so
// the HUD regions always fit) and — on the first real size — (re)places the
// movable default window positions.
func (g *Game) Layout(width, height int) (int, int) {
	if width < minW {
		width = minW
	}
	if height < minH {
		height = minH
	}
	g.applySize(width, height)
	return width, height
}

// applySize records the screen size and — on the first real size —
// (re)places the movable default window positions and restores the
// saved layout over them (user-dragged positions survive later
// resizes). The root-overlay chrome anchors itself, so a resize needs
// no refit from the game.
func (g *Game) applySize(w, h int) {
	if g.sized && w == g.w && h == g.h {
		return
	}
	first := !g.sized
	g.sized, g.w, g.h = true, w, h
	if first {
		g.placeDefaults(w, h)
		g.applyLayout(w, h) // startup: read the sidecar once the screen is known
	}
}

// ---------------------------------------------------------------------------
// Frame update
// ---------------------------------------------------------------------------

// Update polls input, drives scenes, steps the sim on a fixed timestep,
// drains events into the log and the scene reactions, then syncs and
// advances the interface.
func (g *Game) Update() error {
	in := g.buildInput()

	if err := g.handleKeys(in); err != nil {
		return err
	}

	// Top scene consumes the rest of the input (world controls).
	if err := g.top().Update(in); err != nil {
		return err
	}

	// Fixed-timestep stepping at the current time acceleration.
	for i, n := 0, g.clock.Advance(frameSec()); i < n; i++ {
		g.s.Step()
	}

	if evs := g.s.Events(); len(evs) > 0 {
		g.onEvents(evs)
		g.logP.Push(evs)
	}

	g.syncScenes()
	g.syncPanels()
	g.syncMounts()

	if err := g.kit.Update(); err != nil {
		return err
	}

	// Menu-initiated load: rebuild after the UI pump is done (the menu
	// window's handlers must not swap the kit mid-Update).
	if g.pendingLoad {
		g.pendingLoad = false
		g.report(g.LoadGame(), "Game loaded")
	}
	if g.quit {
		// Clean quit: persist the window layout so it survives the
		// restart even without a manual save (best-effort — the loop is
		// ending and there is nowhere left to report an error).
		g.writeLayout()
		return ebiten.Termination
	}
	return nil
}

// handleKeys maps the global keyboard bindings: the KeyMap semantics the
// legacy shell had (Esc menu, N map, H help, Ctrl+S / F9, time
// acceleration) plus the panel toggles and, while no overlay scene is on
// top, the world action keys.
func (g *Game) handleKeys(in Input) error {
	ctrl := held(in.KeysDown, ebiten.KeyControl)
	shift := held(in.KeysDown, ebiten.KeyShift)
	_, menuOpen := g.top().(*MenuScene)
	world := false
	switch g.top().(type) {
	case *SpaceScene, *StationScene:
		world = true
	}

	for _, k := range in.JustPressed {
		switch {
		case k == ebiten.KeyEscape:
			if err := g.onEscape(); err != nil {
				return err
			}
		case ctrl && k == ebiten.KeyS:
			g.report(g.SaveGame(), "Game saved")
		case k == ebiten.KeyF9:
			g.report(g.LoadGame(), "Game loaded")
		case k == ebiten.KeyBracketLeft || k == ebiten.KeyMinus:
			g.clock.Bump(false)
		case k == ebiten.KeyBracketRight || k == ebiten.KeyEqual:
			g.clock.Bump(true)
		default:
			// UI-layer keys are dead while Ctrl is held (legacy parity:
			// only the game-layer chords above respond to Ctrl).
			if ctrl {
				continue
			}
			switch {
			case menuOpen:
				// The Esc menu is modal: nothing else toggles while it
				// is open (save/load/time above still work).
			case k == ebiten.KeyN:
				g.toggleMap()
			case k == ebiten.KeyH:
				g.toggleHelp()
			case k == ebiten.KeyI:
				g.open[pInventory] = !g.open[pInventory]
			case k == ebiten.KeyF:
				g.open[pFitting] = !g.open[pFitting]
			case k == ebiten.KeyC:
				g.open[pScanner] = !g.open[pScanner]
			case k == ebiten.KeyG:
				g.open[pMarket] = !g.open[pMarket]
			case shift && k == ebiten.KeyL:
				// Shift+L toggles the log window; plain L stays loot.
				g.open[pLog] = !g.open[pLog]
			case world:
				g.worldKey(k, shift)
			}
		}
	}
	return nil
}

// onEvents reacts to events the interface does not own (view resets on
// travel).
func (g *Game) onEvents(evs []sim.Event) {
	for _, e := range evs {
		switch e.Kind {
		case sim.EvJumped, sim.EvDocked, sim.EvUndocked:
			g.space.ResetView()
		}
	}
}

// syncScenes keeps the station scene mirroring the docked state.
func (g *Game) syncScenes() {
	docked := g.s.Docked.Has(g.s.Player())
	has := g.findScene(func(s Scene) bool { _, ok := s.(*StationScene); return ok })
	if docked && !has {
		g.push(&StationScene{g: g})
	} else if !docked && has {
		g.removeScene(func(s Scene) bool { _, ok := s.(*StationScene); return ok })
	}
}

// syncPanels refreshes every panel from the sim and adopts overview
// selection changes made during the previous Kit.Update.
func (g *Game) syncPanels() {
	g.hud.SetTimeScale(g.clock.Mult())
	g.kit.Sync()
	g.hud.Sync()
	g.rack.Sync()
	g.logP.Sync()
	g.inv.Sync()
	g.fitt.Sync()
	g.mkt.Sync()
	g.scan.Sync()
	g.mp.Sync()
	g.help.Sync()
}

// syncMounts reconciles the mounted window set with the desired state:
// toggle intent, docked-only visibility and the chrome/core visibility.
func (g *Game) syncMounts() {
	docked := g.s.Docked.Has(g.s.Player())

	g.setPanel(pInventory, g.open[pInventory])
	g.setPanel(pFitting, g.open[pFitting])
	g.setPanel(pMarket, g.open[pMarket])
	// The scanner is space chrome: hidden while docked (legacy parity).
	g.setPanel(pScanner, g.open[pScanner] && !docked)
	g.setPanel(pMap, g.open[pMap])
	g.setPanel(pHelp, g.open[pHelp])
	// The log is a normal window now: it rides both views (its history
	// persists across mounts) and mounts before the Esc menu so the menu
	// keeps the top of the draw order.
	g.setPanel(pLog, g.open[pLog])
	g.setPanel(pMenu, g.open[pMenu])

	// The ship HUD and module rack are space chrome: hidden on the root
	// overlay while docked (legacy parity), and the kit's core windows
	// (Overview / Selected Item) follow the same rule.
	g.setChromeVisible(!docked)
	g.kit.SetCoreVisible(!docked)

	// The station strip appears while docked.
	if docked {
		g.syncStationLabel()
	}
	g.setStrip(&g.stShown, &g.stMount, docked, g.stationW)
}

// setChromeVisible shows or hides the space chrome (HUD + module rack) on
// the root overlay. Hidden widgets keep their last rect, so every hit-test
// must check visibility first (see overUI).
func (g *Game) setChromeVisible(show bool) {
	setVisibility(g.hud.Container().GetWidget(), show)
	setVisibility(g.rack.Container().GetWidget(), show)
}

// setVisibility flips a widget's visibility (no-op when already there).
func setVisibility(w *widget.Widget, show bool) {
	vis := widget.Visibility_Show
	if !show {
		vis = widget.Visibility_Hide
	}
	if w.GetVisibility() != vis {
		w.SetVisibility(vis)
	}
}

// setPanel mounts or unmounts one toggleable window.
func (g *Game) setPanel(p panelID, want bool) {
	have := g.mounted[p] != nil
	if want == have {
		return
	}
	if want {
		g.mounted[p] = g.kit.AddWindow(g.panelWindow(p))
		return
	}
	g.mounted[p]()
	g.mounted[p] = nil
}

// setStrip mounts or unmounts one chrome/station window.
func (g *Game) setStrip(shown *bool, remove *widget.RemoveWindowFunc, want bool, win *widget.Window) {
	if *shown == want {
		return
	}
	if want {
		*remove = g.kit.AddWindow(win)
		*shown = true
		return
	}
	(*remove)()
	*remove, *shown = nil, false
}

// panelWindow returns the window backing a panel id.
func (g *Game) panelWindow(p panelID) *widget.Window {
	switch p {
	case pInventory:
		return g.inv.Win()
	case pFitting:
		return g.fitt.Win()
	case pMarket:
		return g.mkt.Win()
	case pScanner:
		return g.scan.Win()
	case pMap:
		return g.mp.Win()
	case pHelp:
		return g.help.Win()
	case pLog:
		return g.logP.Win()
	case pMenu:
		return g.menuWin
	}
	return nil
}

// overUI reports whether the pointer sits on any interface panel: the
// exact rects of every measurable panel (the kit's core windows, the
// root-overlay chrome, the station and menu strips, all mounted toggle
// windows — the log among them while it is open) plus ebitenui's own
// hover flag, which covers what the game cannot measure (rack tooltips,
// ephemeral windows). The hover flag only
// refreshes in Draw, so it lags a frame behind the pointer — the exact
// rects are the honest gate, the flag is a backstop.
// ebitenui does not gate raw ebiten input, so world mouse handling must
// ask this first.
func (g *Game) overUI(x, y float64) bool {
	if enuiinput.UIHovered {
		return true
	}
	pt := image.Point{X: int(x), Y: int(y)}
	if hitWidget(pt, g.kit.OverviewWindow().GetContainer().GetWidget()) {
		return true
	}
	if hitWidget(pt, g.kit.SelectedWindow().GetContainer().GetWidget()) {
		return true
	}
	if g.hud != nil && isVisible(g.hud.Container().GetWidget()) {
		for _, ch := range g.hud.Container().Children() {
			if hitWidget(pt, ch.GetWidget()) {
				return true
			}
		}
	}
	if g.rack != nil && hitWidget(pt, g.rack.Container().GetWidget()) {
		return true
	}
	// The log is a mounted window like the rest: when it is closed the
	// loop below skips it, so a closed log never gates the world.
	for p := panelID(0); p < numPanels; p++ {
		if g.mounted[p] == nil {
			continue
		}
		if hitWidget(pt, g.panelWindow(p).GetContainer().GetWidget()) {
			return true
		}
	}
	if g.stShown && hitWidget(pt, g.stationW.GetContainer().GetWidget()) {
		return true
	}
	return false
}

// hitWidget reports whether pt sits on w: visible (a hidden widget keeps
// its last rect) and inside it. A never-laid-out zero rect never hits.
func hitWidget(pt image.Point, w *widget.Widget) bool {
	return w.IsVisible() && pt.In(w.Rect)
}

// isVisible reports whether w is shown (it and its parents are visible).
func isVisible(w *widget.Widget) bool { return w.IsVisible() }

// report logs a save/load outcome into the event log.
func (g *Game) report(err error, okMsg string) {
	sev := sim.SevGood
	msg := okMsg
	if err != nil {
		sev, msg = sim.SevDanger, "Failed: "+err.Error()
	}
	g.logP.PushLine(sev, msg)
}

// ---------------------------------------------------------------------------
// Interface construction
// ---------------------------------------------------------------------------

// buildKit constructs the kit, every panel and the root-overlay chrome
// against the current sim. LoadGame re-runs it: panels hold sim
// references, so a restore rebuilds the whole interface (selection
// resets; the toggle intent in g.open survives, LoadGame carries the
// log history across, and LoadGame re-applies the saved window layout
// after this returns).
func (g *Game) buildKit() error {
	kit, err := uikit.NewKit(g.opts.Content, g.s, g.s.Queue)
	if err != nil {
		return fmt.Errorf("game: uikit.NewKit: %w", err)
	}
	g.kit = kit
	g.env = uikit.NewEnv(g.opts.Content, g.s, g.s.Queue)
	env := g.env

	g.hud = uikit.NewHud(env)
	g.rack = uikit.NewRack(env)
	g.logP = uikit.NewLog(env)
	g.inv = uikit.NewInventory(env)
	g.fitt = uikit.NewFitting(env)
	g.mkt = uikit.NewMarket(env)
	g.scan = uikit.NewScanner(env)
	g.mp = uikit.NewMapView(env)
	g.help = uikit.NewHelp(env, helpRows())
	g.menuWin = g.buildMenuWindow()
	g.stationW = g.buildStationWindow()

	// The map and help windows are modal overlays while their scene is up.
	g.mp.Win().Modal = true
	g.help.Win().Modal = true

	// Always-on chrome mounts straight onto the kit's root overlay — no
	// full-screen wrapper windows: the HUD stretches across the root and
	// anchors its own bar/target row, the rack takes the bottom-right
	// corner. (The log used to anchor here too; it is a window now and
	// mounts through syncMounts like the other panels.)
	kit.AddContainer(g.hud.Container(), widget.AnchorLayoutData{
		StretchHorizontal: true,
		StretchVertical:   true,
	})
	kit.AddContainer(g.rack.Container(), widget.AnchorLayoutData{
		HorizontalPosition: widget.AnchorLayoutPositionEnd,
		VerticalPosition:   widget.AnchorLayoutPositionEnd,
		Padding:            &widget.Insets{Right: 10, Bottom: 10},
	})
	// Rack tracks hover so input.UIHovered covers its padding too. The
	// HUD container must not: it spans the whole screen. (The log window
	// gets hover tracking from the window chrome itself.)
	g.rack.Container().GetWidget().TrackHover = true

	// Panels are fresh: no mounts, no selection (documented).
	g.mounted = [numPanels]widget.RemoveWindowFunc{}
	g.stShown = false
	g.stMount = nil

	g.placeDefaults(g.w, g.h)
	g.syncMounts()
	return nil
}

// placeDefaults positions the movable windows for the current screen.
// The kit's core windows are placed too: Overview top-left, Selected
// Item top-right — the left column's bottom belongs to the log and the
// bottom row to the HUD bar, so nothing overlaps at 1280×720 or at the
// 1024×700 minimum. Scanner sits in the centre column under the top
// row; map/menu are centered, help top-center, the station strip takes
// the now-free top-left corner while docked.
func (g *Game) placeDefaults(w, h int) {
	ov := g.kit.OverviewWindow()
	if r := ov.GetContainer().GetWidget().Rect; r.Dx() > 0 {
		ov.SetLocation(image.Rect(8, 8, 8+r.Dx(), 8+r.Dy()))
	}
	sp := g.kit.SelectedWindow()
	if r := sp.GetContainer().GetWidget().Rect; r.Dx() > 0 {
		sp.SetLocation(image.Rect(w-10-r.Dx(), 8, w-10, 8+r.Dy()))
	}
	// The log docks bottom-left: flush with the left/bottom margins, its
	// default width fits the pocket left of the HUD bar (the 420 px bar
	// starts at x=302 at the 1024 minimum) and its top clears the
	// overview — so the default set stays pairwise disjoint.
	if r := g.logP.Win().GetContainer().GetWidget().Rect; r.Dx() > 0 {
		g.logP.Win().SetLocation(image.Rect(10, h-10-r.Dy(), 10+r.Dx(), h-10))
	}
	// Centre column, below the top row (Selected Item owns the top-right
	// corner at 1024×700) and above the HUD bar.
	if r := g.scan.Win().GetContainer().GetWidget().Rect; r.Dx() > 0 {
		g.scan.Win().SetLocation(image.Rect(396, 276, 396+r.Dx(), 276+r.Dy()))
	}
	center := func(win *widget.Window) {
		r := win.GetContainer().GetWidget().Rect
		if r.Dx() == 0 || r.Dy() == 0 {
			return
		}
		win.SetLocation(image.Rect((w-r.Dx())/2, (h-r.Dy())/2, (w-r.Dx())/2+r.Dx(), (h-r.Dy())/2+r.Dy()))
	}
	center(g.mp.Win())
	center(g.menuWin)
	if r := g.help.Win().GetContainer().GetWidget().Rect; r.Dx() > 0 {
		x := (w - r.Dx()) / 2
		g.help.Win().SetLocation(image.Rect(x, 24, x+r.Dx(), 24+r.Dy()))
	}
	if r := g.stationW.GetContainer().GetWidget().Rect; r.Dx() > 0 {
		g.stationW.SetLocation(image.Rect(10, 8, 10+r.Dx(), 8+r.Dy()))
	}
}

// helpRows converts KeyMap into the help panel's row shape (the shared
// "key — description" strings split on the em dash).
func helpRows() []uikit.KeyRow {
	rows := make([]uikit.KeyRow, 0, len(KeyMap))
	for _, b := range KeyMap {
		key, desc := b.Desc, ""
		if i := strings.Index(b.Desc, " — "); i >= 0 {
			key, desc = b.Desc[:i], b.Desc[i+len(" — "):]
		}
		rows = append(rows, uikit.KeyRow{Key: key, Desc: desc})
	}
	return rows
}

// ---------------------------------------------------------------------------
// Draw
// ---------------------------------------------------------------------------

// Draw renders the scene stack (world first, then overlay veils), then
// the whole interface on top.
func (g *Game) Draw(dst *ebiten.Image) {
	for _, sc := range g.scenes {
		sc.Draw(dst)
	}
	g.kit.Draw(dst)
}

// ---------------------------------------------------------------------------
// Scene stack
// ---------------------------------------------------------------------------

func (g *Game) top() Scene { return g.scenes[len(g.scenes)-1] }

func (g *Game) push(s Scene) {
	g.scenes = append(g.scenes, s)
	s.OnEnter()
}

func (g *Game) pop() {
	if len(g.scenes) <= 1 {
		return // never pop the space scene
	}
	s := g.scenes[len(g.scenes)-1]
	g.scenes = g.scenes[:len(g.scenes)-1]
	s.OnLeave()
}

func (g *Game) findScene(match func(Scene) bool) bool {
	for _, s := range g.scenes {
		if match(s) {
			return true
		}
	}
	return false
}

func (g *Game) removeScene(match func(Scene) bool) {
	for i, s := range g.scenes {
		if match(s) {
			g.scenes = append(g.scenes[:i], g.scenes[i+1:]...)
			s.OnLeave()
			return
		}
	}
}

// onEscape implements the Esc menu toggle.
func (g *Game) onEscape() error {
	switch g.top().(type) {
	case *MenuScene:
		g.pop()
	case *MapScene, *HelpScene:
		g.pop()
	default:
		g.push(&MenuScene{g: g})
	}
	return nil
}

// toggleMap opens/closes the MapView overlay (N).
func (g *Game) toggleMap() {
	if _, ok := g.top().(*MapScene); ok {
		g.pop() // OnLeave closes the map window
		return
	}
	g.push(&MapScene{g: g})
}

// toggleHelp opens/closes the key-map overlay (H).
func (g *Game) toggleHelp() {
	if _, ok := g.top().(*HelpScene); ok {
		g.pop() // OnLeave closes the help window
		return
	}
	g.push(&HelpScene{g: g})
}

// ---------------------------------------------------------------------------
// Input snapshot
// ---------------------------------------------------------------------------

// modifierKeys are polled into Input.KeysDown for modifier-aware handling.
var modifierKeys = []ebiten.Key{
	ebiten.KeyShift, ebiten.KeyControl, ebiten.KeyAlt,
	ebiten.KeyArrowUp, ebiten.KeyArrowDown, ebiten.KeyArrowLeft, ebiten.KeyArrowRight,
}

// frameSec returns the real seconds of one frame (for the fixed-timestep
// clock), guarded against a stalled or unknown TPS.
func frameSec() float64 {
	tps := ebiten.ActualTPS()
	if tps <= 0 || tps > 500 {
		return 1.0 / 60.0
	}
	return 1.0 / tps
}

// buildInput snapshots ebiten input into a pure Input for the scenes.
func (g *Game) buildInput() Input {
	cx, cy := ebiten.CursorPosition()
	in := Input{
		MouseX:      float64(cx),
		MouseY:      float64(cy),
		JustPressed: inpututil.AppendJustPressedKeys(nil),
		MouseButton: [3]bool{
			ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft),
			ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight),
			ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle),
		},
		JustClicked: [2]bool{
			inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft),
			inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight),
		},
	}
	for _, k := range modifierKeys {
		if ebiten.IsKeyPressed(k) {
			in.KeysDown = append(in.KeysDown, k)
		}
	}
	_, wheelY := ebiten.Wheel()
	in.Scroll = wheelY
	return in
}

// held reports whether k is in keys.
func held(keys []ebiten.Key, k ebiten.Key) bool {
	for _, h := range keys {
		if h == k {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Save / load
// ---------------------------------------------------------------------------

// SavePath returns the JSON save file location.
func (g *Game) SavePath() string {
	if g.opts.SavePath != "" {
		return g.opts.SavePath
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(".", "entieve", "save.json")
	}
	return filepath.Join(dir, "entieve", "save.json")
}

// SaveGame snapshots the sim to SavePath (default
// os.UserConfigDir()/entieve/save.json) and writes the window-layout
// sidecar next to it, so a restart keeps the layout even without a
// second save.
func (g *Game) SaveGame() error {
	save, err := g.s.Snapshot()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(save, "", "  ")
	if err != nil {
		return err
	}
	path := g.SavePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	return g.writeLayout()
}

// LoadGame restores the sim from SavePath mid-run and rebuilds the
// interface against the restored sim (selection resets; open windows
// stay open; the log history is carried across the rebuild). The saved
// window layout is restored after the rebuild, over the fresh defaults.
func (g *Game) LoadGame() error {
	data, err := os.ReadFile(g.SavePath())
	if err != nil {
		return err
	}
	var save sim.Save
	if err := json.Unmarshal(data, &save); err != nil {
		return err
	}
	// The log is not part of the save: capture it before the rebuild and
	// restore it into the fresh panel afterwards.
	lines := g.logP.Lines()
	if err := g.s.Restore(&save); err != nil {
		return err
	}
	if err := g.buildKit(); err != nil {
		return err
	}
	g.logP.SetLines(lines)
	g.applyLayout(g.w, g.h) // AFTER the rebuild: fresh windows, fresh mounts
	if g.space != nil {
		g.space.ResetView()
	}
	return nil
}
