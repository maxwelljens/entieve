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
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	enuiinput "github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Default window geometry. 1024×700 is the minimum supported window: the
// layout tests pin every property at 1280×720 AND 1024×700. These are
// WINDOW (device-independent) pixels: SetWindowSize takes them as-is and
// Layout converts them to device pixels via the device scale factor —
// never scale the SetWindowSize call itself (window chrome is window px).
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
	// DeviceScale is the display's DPI factor (0 = read the real monitor
	// in Run). Tests pin it explicitly so the baseline suite runs hermetically
	// at 1.0 no matter what display the suite runs on.
	DeviceScale float64
}

// panelID identifies one toggleable interface window. The always-on
// chrome (Overview, Selected Item, HUD, rack) and the docked station
// strip are managed separately.
type panelID int

const (
	pInventory panelID = iota
	pFitting
	pMarket
	pFactory
	pScanner
	pMap
	pHelp
	pLog
	pSettings
	pMenu
	numPanels
)

// panelDef is one toggleable panel's whole registration: the single
// place a panel has to appear in for the frame, mount and persistence
// paths to agree (the key bindings stay hand-written in handleKeys, the
// dock-time trio in stationPanels).
//
//	name   sidecar key ("" = not persisted — the Esc menu is scene-owned
//	       and must not reopen on the next boot)
//	window the window backing the id
//	open   the player's toggle flag (scene-driven for map/help/menu)
//	want   the desired mount for this frame: (game, docked)
//	sync   refresh the panel's widgets from the sim (nil = static
//	       content); run every frame while mounted and once when the
//	       window mounts, so a freshly mounted panel draws current data
//
// syncMounts, syncPanels, panelWindow and the layout sidecar all read
// this table; TestPanelDefsCoverEnum fails if an id has no row.
type panelDef struct {
	name   string
	window func(*Game) *widget.Window
	open   func(*Game) *bool
	want   func(*Game, bool) bool
	sync   func(*Game)
}

// panelDefs is indexed by panelID in enum order — syncMounts iterates
// it, so the mount order follows the enum (the log lands before the Esc
// menu, which keeps the top of the draw order).
var panelDefs = [numPanels]panelDef{
	pInventory: {
		name:   "inventory",
		window: func(g *Game) *widget.Window { return g.inv.Win() },
		open:   func(g *Game) *bool { return &g.open[pInventory] },
		want:   func(g *Game, docked bool) bool { return g.open[pInventory] },
		sync:   func(g *Game) { g.inv.Sync() },
	},
	pFitting: {
		name:   "fitting",
		window: func(g *Game) *widget.Window { return g.fitt.Win() },
		open:   func(g *Game) *bool { return &g.open[pFitting] },
		want:   func(g *Game, docked bool) bool { return g.open[pFitting] },
		sync:   func(g *Game) { g.fitt.Sync() },
	},
	pMarket: {
		name:   "market",
		window: func(g *Game) *widget.Window { return g.mkt.Win() },
		open:   func(g *Game) *bool { return &g.open[pMarket] },
		want:   func(g *Game, docked bool) bool { return g.open[pMarket] },
		sync:   func(g *Game) { g.mkt.Sync() },
	},
	pFactory: {
		name:   "factory",
		window: func(g *Game) *widget.Window { return g.fact.Win() },
		open:   func(g *Game) *bool { return &g.open[pFactory] },
		// The factory is a station service: it only mounts while docked
		// (the scanner's docked counterpart). Its own gating (industry
		// service, materials, slots) lives inside the panel.
		want: func(g *Game, docked bool) bool { return g.open[pFactory] && docked },
		sync: func(g *Game) { g.fact.Sync() },
	},
	pScanner: {
		name:   "scanner",
		window: func(g *Game) *widget.Window { return g.scan.Win() },
		open:   func(g *Game) *bool { return &g.open[pScanner] },
		// The scanner is space chrome: hidden while docked (legacy parity).
		want: func(g *Game, docked bool) bool { return g.open[pScanner] && !docked },
		sync: func(g *Game) { g.scan.Sync() },
	},
	pMap: {
		name:   "map",
		window: func(g *Game) *widget.Window { return g.mp.Win() },
		open:   func(g *Game) *bool { return &g.open[pMap] },
		want:   func(g *Game, docked bool) bool { return g.open[pMap] },
		sync:   func(g *Game) { g.mp.Sync() },
	},
	pHelp: {
		name:   "help",
		window: func(g *Game) *widget.Window { return g.help.Win() },
		open:   func(g *Game) *bool { return &g.open[pHelp] },
		want:   func(g *Game, docked bool) bool { return g.open[pHelp] },
		sync:   func(g *Game) { g.help.Sync() },
	},
	pLog: {
		name:   "log",
		window: func(g *Game) *widget.Window { return g.logP.Win() },
		open:   func(g *Game) *bool { return &g.open[pLog] },
		want:   func(g *Game, docked bool) bool { return g.open[pLog] },
		sync:   func(g *Game) { g.logP.Sync() },
	},
	pSettings: {
		name:   "settings",
		window: func(g *Game) *widget.Window { return g.sett.Win() },
		open:   func(g *Game) *bool { return &g.open[pSettings] },
		want:   func(g *Game, docked bool) bool { return g.open[pSettings] },
		sync:   func(g *Game) { g.sett.Sync() },
	},
	pMenu: {
		name:   "", // not persisted: the Esc menu is pushed as a scene
		window: func(g *Game) *widget.Window { return g.menuWin },
		open:   func(g *Game) *bool { return &g.open[pMenu] },
		want:   func(g *Game, docked bool) bool { return g.open[pMenu] },
		sync:   nil, // scene-owned static buttons
	},
}

// Game implements ebiten.Game and hosts the uikit interface.
type Game struct {
	opts  Options
	s     *sim.Sim
	ren   *render.Renderer
	clock Clock

	// kit owns the interface (Overview + Selected Item are built into it);
	// env is the kit's own environment (Kit.Env) — one theme, palette and
	// face set per session — that the extra panels are constructed from.
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
	fact *uikit.Factory
	scan *uikit.Scanner
	mp   *uikit.MapView
	help *uikit.HelpPanel

	menuWin  *widget.Window
	stationW *widget.Window
	stName   *widget.Text

	// The settings window (uikit/settings.go): UI scale stepper,
	// fullscreen toggle, reset-layout. Like every panel it is rebuilt by
	// buildKit, so its live state is re-read from g in Sync.
	sett *uikit.Settings

	// The station window's contract board (contracts.go): the body, the
	// two row sections, the signature of the last rebuild and the
	// actionable rows (Accept/Abandon buttons by contract id).
	stBody      *widget.Container
	stAvail     *widget.Container
	stActive    *widget.Container
	stModel     stationBoardModel
	stAvailRows []stationRow
	stActRows   []stationRow

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
	stationPrior [numPanels]bool

	// placed is the last rect this session composed for each window,
	// restored the windows positioned from the layout sidecar. A window
	// is virgin — safe to (re)place when the viewport changes — while it
	// carries no restored flag and still sits exactly where we last put
	// it; anything else is the player's arrangement and is never stomped.
	placed   map[*widget.Window]image.Rectangle
	restored map[*widget.Window]bool

	pendingLoad bool
	quit        bool

	// menuModal latches the frame's context-menu state: Game.Update
	// samples it (Kit.MenuInput) before any scene or key handling, so a
	// menu that is up — or that consumed this frame's click/Esc — gates
	// world input (overUI) and the key switches for the whole frame.
	menuModal bool

	// objClick is the contract-objective label's double-click state.
	objClick struct {
		id content.ContractID
		at time.Time
	}

	scenes []Scene
	space  *SpaceScene
	w, h   int // logical screen size (Layout), in device pixels
	sized  bool

	// Resolution independence: dev is the display's device scale factor
	// (window px → device px, see Layout) and uiScale the persisted user
	// setting; their product is what uikit.SetScale installs as the global
	// UI scale, before the interface is built and again on every change.
	dev        float64
	uiScale    float64
	fullscreen bool // persisted; applied in Run via ebiten.SetFullscreen
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
	}
	// Scale before anything is built: the device factor (pinned by tests,
	// read from the display in Run) times the persisted user UI scale is
	// the global uikit scale every metric and face below is built against.
	g.dev = opts.DeviceScale
	if g.dev <= 0 {
		g.dev = 1
	}
	g.loadSettings() // <SavePath>.settings.json: ui_scale, fullscreen
	uikit.SetScale(g.uiScale * g.dev)
	g.w = int(math.Round(float64(defaultW) * g.dev))
	g.h = int(math.Round(float64(defaultH) * g.dev))
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
	// The display's device scale factor, read on the main thread before
	// the main loop (Options.DeviceScale pins it for tests). If it moves
	// the scale off what New built against (New runs at 1.0 unless
	// pinned), rebuild the interface once, before the first Layout.
	if g.opts.DeviceScale > 0 {
		g.dev = g.opts.DeviceScale
	} else {
		// Documented pre-loop entry: DeviceScaleFactor must be called on
		// the main thread before the main loop (deprecated in favor of
		// Monitor().DeviceScaleFactor, which needs a window).
		g.dev = ebiten.DeviceScaleFactor()
	}
	g.applyScale(false)
	ebiten.SetWindowTitle(title)
	// Window chrome is in WINDOW pixels: never multiply this by the UI or
	// device scale (Layout converts the outside size separately).
	ebiten.SetWindowSize(defaultW, defaultH)
	ebiten.SetWindowResizable(true)
	ebiten.SetVsyncEnabled(true)
	ebiten.SetFullscreen(g.fullscreen)
	return ebiten.RunGame(g)
}

// deviceScale returns the display factor (1 for a Game built without one,
// e.g. a struct-level test harness).
func (g *Game) deviceScale() float64 {
	if g.dev > 0 {
		return g.dev
	}
	return 1
}

// Layout returns the screen size (clamped to the layout minimum in WINDOW
// pixels first) in DEVICE pixels and (re)places the movable default window
// positions whenever that size changes (virgin windows only).
//
// The outside size ebiten passes in is device-independent (window) pixels;
// multiplying by the device scale factor makes the framebuffer match the
// monitor, so text rasterizes at its final size — crisp on HiDPI instead
// of rendered-small-and-upscaled by the OS. ebiten reports cursor
// positions in these same Layout units, and every UI metric is scaled
// into them (uikit.Px = uiScale × device), so grips, menu rows and
// overUI rects all agree with the cursor by construction.
func (g *Game) Layout(width, height int) (int, int) {
	if width < minW {
		width = minW
	}
	if height < minH {
		height = minH
	}
	w := int(math.Round(float64(width) * g.deviceScale()))
	h := int(math.Round(float64(height) * g.deviceScale()))
	g.applySize(w, h)
	return w, h
}

// applySize records the screen size and — on every size change —
// (re)places the movable default windows for the new viewport, then
// restores the saved layout over them on the first real size
// (user-dragged positions and sidecar entries survive later resizes:
// placeDefaults only touches virgin windows, see placeIfVirgin). The
// restored windows are re-clamped to the new screen on every size change
// (see clampRestored) so a sidecar written at a larger resolution can
// never strand a window off-screen. The root-overlay chrome anchors
// itself, so a resize needs no refit from the game.
func (g *Game) applySize(w, h int) {
	if g.sized && w == g.w && h == g.h {
		return
	}
	first := !g.sized
	g.sized, g.w, g.h = true, w, h
	g.placeDefaults(w, h)
	if first {
		g.applyLayout(w, h) // startup: read the sidecar once the screen is known
	}
	g.clampRestored(w, h)
}

// ---------------------------------------------------------------------------
// Frame update
// ---------------------------------------------------------------------------

// Update polls input, drives scenes, steps the sim on a fixed timestep,
// drains events into the log and the scene reactions, then syncs and
// advances the interface.
func (g *Game) Update() error {
	// The context menu owns its frame: sample it before anything reads
	// input (it may consume this frame's Esc or click), then gate the
	// scene and key switches on the latch.
	g.menuModal = g.kit.MenuInput()

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
			// A context menu that was up this frame consumed the Esc
			// (Kit.MenuInput closed it) — it must not also push the menu
			// scene behind the dismissed popup.
			if g.menuModal {
				continue
			}
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
			case g.menuModal:
				// So is a context menu: it owns this frame's keys until
				// the click or Esc that dismisses it has been spent.
			case k == ebiten.KeyN:
				g.toggleMap()
			case k == ebiten.KeyH:
				g.toggleHelp()
			case k == ebiten.KeyP:
				g.open[pSettings] = !g.open[pSettings]
			case k == ebiten.KeyI:
				g.open[pInventory] = !g.open[pInventory]
			case k == ebiten.KeyF:
				g.open[pFitting] = !g.open[pFitting]
			case k == ebiten.KeyC:
				g.open[pScanner] = !g.open[pScanner]
			case k == ebiten.KeyG:
				g.open[pMarket] = !g.open[pMarket]
			case k == ebiten.KeyB:
				g.open[pFactory] = !g.open[pFactory]
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
// travel, contract-board refresh on mission/dock transitions — the board
// also re-syncs every frame while docked, see syncStationBoard).
func (g *Game) onEvents(evs []sim.Event) {
	for _, e := range evs {
		switch e.Kind {
		case sim.EvJumped:
			g.space.ResetView()
		case sim.EvDocked, sim.EvUndocked:
			g.space.ResetView()
			g.syncStationBoard() // the board follows the docked station
		case sim.EvMission:
			g.syncStationBoard() // accept / progress / ready / claim / refuse
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

// syncPanels refreshes the mounted panels from the sim and adopts
// overview selection changes made during the previous Kit.Update. An
// unmounted window's widgets are invisible, so their Sync is skipped —
// they get a fresh Sync the moment setPanel mounts them (same frame,
// before the first draw) instead of running every frame forever.
func (g *Game) syncPanels() {
	g.hud.SetTimeScale(g.clock.Mult())
	g.kit.Sync()
	g.hud.Sync()
	g.rack.Sync()
	for p := panelID(0); p < numPanels; p++ {
		if g.mounted[p] != nil {
			g.syncPanel(p)
		}
	}
}

// syncPanel runs one panel's registered Sync (nil = nothing to sync).
func (g *Game) syncPanel(p panelID) {
	if s := panelDefs[p].sync; s != nil {
		s(g)
	}
}

// syncMounts reconciles the mounted window set with the desired state:
// toggle intent, docked-only visibility and the chrome/core visibility.
// The per-panel rule lives in panelDefs[p].want (one table), iterated in
// enum order so the mount order — and the draw order it produces — is
// stable.
func (g *Game) syncMounts() {
	docked := g.s.Docked.Has(g.s.Player())

	for p := panelID(0); p < numPanels; p++ {
		g.setPanel(p, panelDefs[p].want(g, docked))
	}

	// The ship HUD and module rack are space chrome: hidden on the root
	// overlay while docked (legacy parity), and the kit's core windows
	// (Overview / Selected Item) follow the same rule.
	g.setChromeVisible(!docked)
	g.kit.SetCoreVisible(!docked)

	// The station strip appears while docked; the contract board follows
	// the docked station and the live mission state.
	if docked {
		g.syncStationLabel()
		g.syncStationBoard()
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

// setPanel mounts or unmounts one toggleable window. A mount runs the
// panel's Sync immediately: syncPanels (which runs before syncMounts)
// skipped it while unmounted, so this is what puts current data on the
// window before this frame's draw.
//
// Note (upstream): every Kit.AddWindow leaks a little on ebitenui's side
// — the four event handlers AddWindowQuietly never removes plus a
// Container.Validate button rebuild, ≈5.3 KB per mount cycle (measured;
// see docs/ARCHITECTURE.md, "upstream window-remount leak"). Unmounting
// is still the right semantics: a hidden window would keep gating world
// input and change the mount-order draw stack, so this is documented,
// not "fixed" by hiding panels instead.
func (g *Game) setPanel(p panelID, want bool) {
	have := g.mounted[p] != nil
	if want == have {
		return
	}
	if want {
		g.mounted[p] = g.kit.AddWindow(g.panelWindow(p))
		g.syncPanel(p)
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

// panelWindow returns the window backing a panel id (nil for an id
// outside the enum).
func (g *Game) panelWindow(p panelID) *widget.Window {
	if p < 0 || p >= numPanels {
		return nil
	}
	return panelDefs[p].window(g)
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
	// The context menu gates in two deterministic parts: its popup rect,
	// which uikit exposes (an ebitenui-drawn popup would be invisible to
	// this rect test), and the frame latch — a menu that was up when the
	// frame's input was sampled owns every gesture on it, so the click
	// that dismisses the popup cannot also act in the world.
	if r, open := g.kit.MenuRect(); open && pt.In(r) {
		return true
	}
	if g.menuModal {
		return true
	}
	if hitWidget(pt, g.kit.OverviewWindow().GetContainer().GetWidget()) {
		return true
	}
	if hitWidget(pt, g.kit.SelectedWindow().GetContainer().GetWidget()) {
		return true
	}
	// The Show Info window is a normal (non-modal) window, but only while
	// it is up: closed it is unmounted AND hidden, and a hidden widget
	// keeps its last rect (hitWidget checks visibility).
	if hitWidget(pt, g.kit.InfoWindow().GetContainer().GetWidget()) {
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
	// One environment per session: the kit's own theme, palette and face
	// set, handed to every extra panel (uikit.NewEnv would build a second
	// one the kit never uses).
	g.env = kit.Env()
	env := g.env

	g.hud = uikit.NewHud(env)
	g.rack = uikit.NewRack(env)
	// Click-activation targets the player's selected entity first — the
	// same source the F-key module toggles use — before falling back to
	// the first suitable lock.
	g.rack.SetTargetPicker(func() ecs.Entity { return kit.Selected() })
	g.logP = uikit.NewLog(env)
	g.inv = uikit.NewInventory(env)
	g.fitt = uikit.NewFitting(env)
	g.mkt = uikit.NewMarket(env)
	g.fact = uikit.NewFactory(env)
	g.scan = uikit.NewScanner(env)
	g.mp = uikit.NewMapView(env)
	g.help = uikit.NewHelp(env, helpRows())
	g.sett = uikit.NewSettings(env, uikit.SettingsActions{
		UIScale:       func() float64 { return g.uiScale },
		StepScale:     func(delta int) { g.stepUIScale(delta) },
		Fullscreen:    func() bool { return g.fullscreen },
		SetFullscreen: func(on bool) { g.setFullscreen(on) },
		ResetLayout:   func() { g.resetLayout() },
	})
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
		Padding:            &widget.Insets{Right: uikit.Px(10), Bottom: uikit.Px(10)},
	})
	// Rack tracks hover so input.UIHovered covers its padding too. The
	// HUD container must not: it spans the whole screen. (The log window
	// gets hover tracking from the window chrome itself.)
	g.rack.Container().GetWidget().TrackHover = true

	// Panels are fresh: no mounts, no selection (documented). The
	// placement records belong to the old windows — drop them with the
	// kit (a load rebuild re-composes the defaults, then applyLayout
	// brings the player's saved arrangement back over them). The dock
	// prior goes too: it is mount bookkeeping for the panels being
	// rebuilt (applyLayout re-adopts the persisted pre-dock intent for a
	// docked restore).
	g.mounted = [numPanels]widget.RemoveWindowFunc{}
	g.stShown = false
	g.stMount = nil
	g.stationPrior = [numPanels]bool{}
	g.placed = map[*widget.Window]image.Rectangle{}
	g.restored = map[*widget.Window]bool{}

	g.placeDefaults(g.w, g.h)
	g.syncMounts()
	return nil
}

// placeDefaults (re)places every movable default window for the current
// screen. The kit's core windows are placed too: Overview top-left,
// Selected Item top-right — the left column's bottom belongs to the log
// and the bottom row to the HUD bar, so nothing overlaps at 1280×720 or
// at the 1024×700 minimum. Scanner sits in the centre column under the
// top row; map/menu are centered, help top-center; the station strip
// takes the now-free top-left corner while docked and the docked
// service trio tiles the viewport beside it (stationLayout).
//
// Every placement goes through placeIfVirgin: a window the player
// dragged, or one restored from the layout sidecar, keeps its
// position — so recomposing on a viewport change never stomps an
// arrangement.
func (g *Game) placeDefaults(w, h int) {
	place := g.placeIfVirgin
	// Design-unit margins composed through the scale funnel: at scale 1
	// these are exactly the historical pixels (the layout tests pin them).
	ov := g.kit.OverviewWindow()
	if r := ov.GetContainer().GetWidget().Rect; r.Dx() > 0 {
		m := uikit.Px(8)
		place(ov, image.Rect(m, m, m+r.Dx(), m+r.Dy()))
	}
	sp := g.kit.SelectedWindow()
	if r := sp.GetContainer().GetWidget().Rect; r.Dx() > 0 {
		m, edge := uikit.Px(8), w-uikit.Px(10)
		place(sp, image.Rect(edge-r.Dx(), m, edge, m+r.Dy()))
	}
	// The log docks bottom-left: flush with the left/bottom margins, its
	// default width fits the pocket left of the HUD bar (the 420 px bar
	// starts at x=302 at the 1024 minimum) and its top clears the
	// overview — so the default set stays pairwise disjoint.
	if r := g.logP.Win().GetContainer().GetWidget().Rect; r.Dx() > 0 {
		m := uikit.Px(10)
		place(g.logP.Win(), image.Rect(m, h-m-r.Dy(), m+r.Dx(), h-m))
	}
	// Centre column, below the top row (Selected Item owns the top-right
	// corner at 1024×700) and above the HUD bar.
	if r := g.scan.Win().GetContainer().GetWidget().Rect; r.Dx() > 0 {
		x, y := uikit.Px(396), uikit.Px(276)
		place(g.scan.Win(), image.Rect(x, y, x+r.Dx(), y+r.Dy()))
	}
	center := func(win *widget.Window) {
		r := win.GetContainer().GetWidget().Rect
		if r.Dx() == 0 || r.Dy() == 0 {
			return
		}
		place(win, image.Rect((w-r.Dx())/2, (h-r.Dy())/2, (w-r.Dx())/2+r.Dx(), (h-r.Dy())/2+r.Dy()))
	}
	center(g.mp.Win())
	center(g.menuWin)
	// The factory window has no composed slot in the docked service trio
	// (the 1024×700 minimum has no room for a fourth window), so it
	// floats centered like the other on-demand overlays — the player
	// drags it where they want and the layout sidecar remembers.
	center(g.fact.Win())
	// The settings window floats with the other on-demand overlays too
	// (reset-layout recomposes it here as well).
	center(g.sett.Win())
	if r := g.help.Win().GetContainer().GetWidget().Rect; r.Dx() > 0 {
		x, y := (w-r.Dx())/2, uikit.Px(24)
		place(g.help.Win(), image.Rect(x, y, x+r.Dx(), y+r.Dy()))
	}
	// The Show Info window floats: centered like the other on-demand
	// overlays (its sidecar rect wins once the player has one, and it is
	// hidden until the first Show anyway).
	center(g.kit.InfoWindow())
	// Docked set: the station's own column (content height) and the
	// service trio composed for this viewport.
	lay := stationLayout(w, h, g.stationW.GetContainer().GetWidget().Rect.Dy())
	place(g.stationW, lay.Station)
	place(g.mkt.Win(), lay.Market)
	place(g.inv.Win(), lay.Inventory)
	place(g.fitt.Win(), lay.Fitting)
}

// placeIfVirgin moves win to rect unless the player owns its position:
// a window restored from the layout sidecar, or one that no longer sits
// on the last rect we composed (only a drag/resize moves a window off
// it — hidden and unmounted windows keep their rect).
func (g *Game) placeIfVirgin(win *widget.Window, rect image.Rectangle) {
	if g.restored[win] {
		return
	}
	if prev, ok := g.placed[win]; ok && win.GetContainer().GetWidget().Rect != prev {
		return // player-moved: never stomp it again
	}
	win.SetLocation(rect)
	// Read back what SetLocation actually applied: it grows a rect below
	// MinSize, and the tracker must hold the real rect or the window
	// would read as player-moved from here on.
	g.placed[win] = win.GetContainer().GetWidget().Rect
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
