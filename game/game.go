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

	enuiinput "github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	uikit "github.com/entieve/entieve/uikit/screens"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
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
	// StartInTitle boots into the title screen (cmd-only; never together
	// with Save — --load bypasses the title). Tests and `simulate` leave it
	// false and see the unchanged boot.
	StartInTitle bool
	// Version is the title screen's version line ("" = omit it).
	Version string
	// DeviceScale is the display's DPI factor (0 = read the real monitor
	// in Run). Tests pin it explicitly so the baseline suite runs hermetically
	// at 1.0 no matter what display the suite runs on.
	DeviceScale float64
	// Music is the audio seam. Nil installs the silent nop — every
	// headless path (tests, simulate) and the documented default; the
	// real engine is built only in cmdRun (see Music).
	Music Music
	// UISound is the UI cue seam, installed the same way: nil is the
	// silent nop, and cmdRun builds the real one beside the music
	// (NewAudio, one shared context).
	UISound UISound
}

// Game implements ebiten.Game and hosts the uikit interface.
type Game struct {
	opts  Options
	s     *sim.Sim
	ren   *render.Renderer
	clock Clock

	// kit owns the interface (Overview + Selected Item are built into it);
	// env is the kit's own session (Kit.Env) — the three seams and the
	// popup services, one theme/view/sink per session — that the extra
	// panels are constructed from.
	kit *uikit.Kit
	env uikit.Session

	// enqueue is the executor's tail onto the sim: the intent dispatch
	// queues every command through it (tests swap it to observe what a
	// button fired). It defaults to g.s.Queue in New.
	enqueue func(sim.Command)

	// Anchored chrome panels, mounted directly on the kit's root overlay
	// (Kit.AddContainer); the HUD and rack are hidden — not unmounted —
	// while docked.
	hud  *uikit.Hud
	rack *uikit.Rack

	// host owns the Screen table's live state: open flags, mounts, the
	// committed-close handshake and the placement tracker. The concrete
	// panels are reached through bodyOf when the game calls into them
	// (the log's feed, the market's search field).
	host *ScreenHost

	// The station services view (the one window docking opens): its
	// window rides the panel registry like every Screen row, and svcUI
	// is the uikit row it is built from — identity, Undock, Cargo and
	// this station's service buttons (uikit/screens/services.go).
	svcWin *widget.Window
	svcUI  *uikit.Services

	// The left navigation rail (uikit/screens/rail.go): root-overlay
	// chrome like the HUD and rack, but mounted TOP (Kit.AddTopContainer):
	// it paints above windows and its clicks win over a window dragged
	// onto its gutter. Derived from the Screen spec by railEntries
	// (game/rail.go). railCollapsed is its persisted settings.json state
	// (missing key = expanded).
	rail          *uikit.Rail
	railCollapsed bool

	// The persisted vsync flag and the last windowed size (window
	// pixels). vsyncApply is the ebiten seam (nil = the real call) so
	// headless tests can pin the apply path; flushSize gates the size
	// keys — only a clean quit writes them.
	vsync      bool
	vsyncApply func(bool)
	winW, winH int
	flushSize  bool

	// The music seam and the three audio settings behind it (percent
	// 0..100 for the volumes; the engine multiplies them). music is
	// never nil after New: nil Options.Music installs nopMusic, so the
	// real audio context can only be reached from cmdRun. ui is the UI
	// cue seam beside it (the UI volume rides the same master), nop
	// unless cmdRun injected the real one.
	music               Music
	ui                  UISound
	masterVol, musicVol int
	uiVol               int
	shuffle             bool

	// Keybindings (game/keys.go): the live overrides, the armed row and
	// its refusal notice. The window's action seam rides the host's
	// screenServices (Settings and Keys actions are assembled there).
	binds         map[string]binding
	captureAction string
	keyNotice     string

	// The station window's contract board (contracts.go): the body, the
	// two row sections, the signature of the last rebuild and the
	// actionable rows (Accept/Abandon buttons by contract id).
	stBody      *widget.Container
	stAvail     *widget.Container
	stActive    *widget.Container
	stModel     stationBoardModel
	stAvailRows []stationRow
	stActRows   []stationRow

	// mktSearchFocus opens the market for Ctrl+F and focuses its search
	// field once the panel exists (the mount happens after the key pass).
	mktSearchFocus bool

	// Selection lives in the kit: world clicks, the Tab cycle-lock and
	// the overview all feed Kit.Select, the game reads Kit.Selected. The
	// placement tracker (placed/restored) lives on the host.

	pendingLoad bool
	quit        bool

	// activeSave is where Save writes ("" = Options.SavePath): set by a
	// load or a save-as. Layout and settings sidecars stay keyed to the
	// configured SavePath (UI prefs are global).
	activeSave string

	// title is the boot phase: the world renders statically behind the
	// title UI, the sim never steps and gameplay chrome stays unmounted.
	title             bool
	titleUI           *titleScreen
	pendingTitleEnter bool
	pendingTitleLoad  string

	// menuModal latches the frame's popup state (the context menu and the
	// quantity picker): Game.Update samples it (Kit.MenuInput) before any
	// scene or key handling, so a popup that is up — or that consumed this
	// frame's click/Esc — gates world input (overUI) and the key switches
	// for the whole frame.
	menuModal bool

	// menuWasOpen is the previous frame's popup state: the popup's open
	// EDGE is its appearance cue. The menu is kit-level (never a mounted
	// window), so no window-mount edge speaks for it; a reopen inside one
	// frame (a column toggle re-opens the popup) is not a new appearance.
	// Dismissal stays silent — the press that closes the popup is the same
	// gesture whose landing the click cue already reports.
	menuWasOpen bool

	scenes []Scene
	space  *SpaceScene
	w, h   int // logical screen size (Layout), in device pixels
	sized  bool

	// Resolution independence: dev is the display's device scale factor
	// (window px → device px, see Layout) and uiScale the persisted user
	// setting; their product is what theme.SetScale installs as the global
	// UI scale, before the interface is built and again on every change.
	dev        float64
	uiScale    float64
	fullscreen bool // persisted; applied in Run via ebiten.SetFullscreen
}

// New builds a game session from options.
func New(opts Options) (*Game, error) {
	if probs := screenSpecErrors(screens); len(probs) > 0 {
		return nil, fmt.Errorf("game: screen spec: %s", strings.Join(probs, "; "))
	}
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
		opts:  opts,
		s:     s,
		ren:   render.New(opts.Content),
		title: opts.StartInTitle,
	}
	g.enqueue = s.Queue
	// Scale before anything is built: the device factor (pinned by tests,
	// read from the display in Run) times the persisted user UI scale is
	// the global uikit scale every metric and face below is built against.
	g.dev = opts.DeviceScale
	if g.dev <= 0 {
		g.dev = 1
	}
	g.loadSettings() // <SavePath>.settings.json: ui_scale, fullscreen
	// The audio seams are installed before anything can read them: nil
	// (tests, simulate) is the silent nop, so no test can reach
	// audio.NewContext.
	g.music = opts.Music
	if g.music == nil {
		g.music = nopMusic{}
	}
	g.ui = opts.UISound
	if g.ui == nil {
		g.ui = nopUISound{}
	}
	g.applyAudioSettings()
	g.music.Start() // a real engine's Play is pending until the device opens
	g.loadKeys()    // <SavePath>.keys.json: the keybinding overrides
	theme.SetScale(g.uiScale * g.dev)
	g.w = int(math.Round(float64(defaultW) * g.dev))
	g.h = int(math.Round(float64(defaultH) * g.dev))
	if err := g.buildKit(); err != nil {
		return nil, err
	}
	g.space = &SpaceScene{g: g, zoom: 1}
	g.scenes = []Scene{g.space}
	g.space.OnEnter()
	if g.title {
		g.ensureTitleUI()
	}
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
	ebiten.SetWindowSize(g.windowSize())
	ebiten.SetWindowResizable(true)
	g.applyVsync(g.vsync) // persisted flag; absent in old files = on
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
// into them (theme.Px = uiScale × device), so grips, menu rows and
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
// placeDefaults only touches virgin windows, see ScreenHost.Place). The
// restored windows are re-clamped to the new screen on every size change
// (see ScreenHost.ClampRestored) so a sidecar written at a larger resolution can
// never strand a window off-screen. The root-overlay chrome anchors
// itself, so a resize needs no refit from the game.
func (g *Game) applySize(w, h int) {
	if g.sized && w == g.w && h == g.h {
		return
	}
	first := !g.sized
	g.sized, g.w, g.h = true, w, h
	// The last WINDOWED size is the outside size, not these device
	// pixels: divide back through the device factor (quitPersist writes
	// it, so the transient boot size is never persisted).
	g.winW = int(math.Round(float64(w) / g.deviceScale()))
	g.winH = int(math.Round(float64(h) / g.deviceScale()))
	g.placeDefaults(w, h)
	if first {
		g.applyLayout(w, h) // startup: read the sidecar once the screen is known
	}
	g.host.ClampRestored(w, h)
}

// ---------------------------------------------------------------------------
// Frame update
// ---------------------------------------------------------------------------

// Update polls input, drives scenes, steps the sim on a fixed timestep,
// drains events into the log and the scene reactions, then syncs and
// advances the interface.
func (g *Game) Update() error {
	// One input pass per Update: the previous pass's cursor claim is spent
	// (a pointer that left a seam falls back to the arrow). ebiten may run
	// Draw without an Update, and ApplyCursor then repeats this pass's
	// decision instead of clearing it.
	widgets.BeginCursorFrame()
	// The context menu appearing is its own cue: the popup is kit-level
	// (no mount edge speaks for it), so the closed→open edge is read here —
	// before this frame's poll, which may dismiss it.
	_, menuUp := g.kit.MenuRect()
	if menuUp && !g.menuWasOpen {
		g.ui.Play(sfxOpen)
	}
	g.menuWasOpen = menuUp
	// The context menu owns its frame: sample it before anything reads
	// input (it may consume this frame's Esc or click), then gate the
	// scene and key switches on the latch.
	g.menuModal = g.kit.MenuInput()

	in := g.buildInput()

	// UI sounds ride the frame's own input: one click cue per press edge
	// that lands on the interface (title or game — the kit's windows are
	// the same), silent for world presses and drag frames.
	g.uiClick(in)

	// Music advances identically at the title and in-game: the tick is on
	// the mainline, before the title-phase branch.
	g.music.Tick()

	if g.title {
		return g.updateTitle(in)
	}

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
		bodyOf[*uikit.LogPanel](g.host, idLog).Push(evs)
	}

	g.syncScenes()
	g.syncPanels()
	g.syncMounts()
	if g.mktSearchFocus {
		if mkt := bodyOf[*uikit.Market](g.host, idMarket); mkt != nil {
			mkt.FocusSearch()
		}
		g.mktSearchFocus = false
	}

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
		g.quitPersist()
		return ebiten.Termination
	}
	return nil
}

// quitPersist is the clean-quit flush: the layout sidecar plus the
// settings sidecar carrying the last windowed size (best-effort — the
// loop is ending and there is nowhere left to report an error).
func (g *Game) quitPersist() {
	g.music.Stop()
	g.writeLayout()
	g.flushSize = true
	g.writeSettings()
}

// handleKeys maps the global keyboard bindings: the fixed chord layer
// (Esc menu, Ctrl+S / F9, Ctrl+F, time acceleration) stays literal; the
// rebindable layer resolves a press through the live key table (the
// Screen spec's toggles, the help scene and the world commands).
//
// While the order mini-window has a focused price/qty field, the keys are
// the field's: every hotkey yields so typing into it never fires a world
// action (Esc clears the focus first — closing needs Cancel / the ×). An
// armed keybindings row owns every key press until it cancels or assigns.
func (g *Game) handleKeys(in Input) error {
	ctrl := held(in.KeysDown, ebiten.KeyControl)
	shift := held(in.KeysDown, ebiten.KeyShift)
	_, menuOpen := g.top().(*MenuScene)
	world := false
	switch g.top().(type) {
	case *SpaceScene, *StationScene:
		world = true
	}
	typing := g.kit.OrderTyping()

	for _, k := range in.JustPressed {
		if g.captureKey(k, ctrl, shift) {
			continue
		}
		if typing {
			if k == ebiten.KeyEscape {
				g.kit.BlurOrder()
			}
			continue
		}
		// The market's search field is the second typing surface: while it
		// holds focus, arrows/Enter/Esc belong to the results and every
		// world hotkey yields. Esc with text clears the query first; Esc
		// on an empty field blurs (SearchKey's false) and falls through
		// to the normal Esc handling below.
		if mkt := bodyOf[*uikit.Market](g.host, idMarket); mkt != nil && mkt.SearchFocused() {
			consumed := mkt.SearchKey(k)
			if consumed || k != ebiten.KeyEscape {
				continue
			}
		}
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
		case ctrl && k == ebiten.KeyF:
			// Ctrl+F opens the market (if closed) and focuses its search
			// field; the focus lands after the mount, below.
			g.host.SetOpen(idMarket, true)
			g.mktSearchFocus = true
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
			default:
				// The data layer: the live key table decides whether this
				// is a screen toggle (help included), a world command, or
				// nothing (an unbound or fixed-layer key).
				action, ok := g.actionFor(k, shift)
				if !ok {
					continue
				}
				if !g.pressScreen(action) && world {
					g.worldKey(action)
				}
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
	g.syncRail()
	g.host.Sync()
}

// syncRail refreshes the navigation rail (context gating and the
// open-state glow) and mirrors its collapse into
// <SavePath>.settings.json when the player flips the edge tab — the one
// settings write that is not a settings-window control. The write is
// best-effort like every settings write; boot restores the state in
// buildKit (a missing key reads expanded).
func (g *Game) syncRail() {
	g.rail.Sync()
	if collapsed := g.rail.Collapsed(); collapsed != g.railCollapsed {
		g.railCollapsed = collapsed
		if err := g.writeSettings(); err != nil {
			g.report(err, "")
		}
	}
}

// syncMounts is the frame's composition: the room decides the mounts, one
// chrome rule decides visibility, then the dock-only jobs run. Mount order
// follows the spec (the host's slot order), so the draw stack does too.
func (g *Game) syncMounts() {
	docked := g.s.Docked.Has(g.s.Player())
	g.host.Reconcile(Room{Title: g.title, Docked: docked})
	// Space chrome (HUD, rack, the kit's core windows) follows the room;
	// top chrome (the rail) follows gameplay — the title shows neither.
	g.applyChrome(!docked && !g.title, !g.title)

	// The services view is an ordinary panel now (the station scene
	// opens it, the host mounts it); only the contract board keeps a
	// docked-only refresh — it follows the docked station and the live
	// mission state while the scene is up.
	if docked && !g.title {
		g.syncStationBoard()
	}

	// The order mini-window is transient and docked-only: a player who
	// leaves the station has no book to post to, so it closes with the
	// dock (undock and jump — a jump implies undocked — both land here).
	if !docked && !g.title {
		g.kit.CloseOrder()
	}
}

// applyChrome is the one visibility rule: the space chrome (HUD, rack,
// kit core windows) follows space; the top chrome (rail) follows gameplay.
func (g *Game) applyChrome(space, gameplay bool) {
	theme.ShowWidget(g.hud.Container().GetWidget(), space)
	theme.ShowWidget(g.rack.Container().GetWidget(), space)
	g.kit.SetCoreVisible(space)
	g.kit.SetTopsVisible(gameplay)
}

// overUI reports whether the pointer sits on interface panel chrome: the
// kit's own exact rects (the core windows, every mounted window — the log
// among them while it is open — and the rail), the root-overlay chrome
// (HUD, rack) and ebitenui's hover flag, which covers what the game cannot
// measure (rack tooltips, ephemeral windows). The hover flag only
// refreshes in Draw, so it lags a frame behind the pointer — the exact
// rects are the honest gate, the flag is a backstop.
// ebitenui does not gate raw ebiten input, so world mouse handling must
// ask this first.
func (g *Game) overUI(x, y float64) bool {
	if enuiinput.UIHovered {
		return true
	}
	pt := image.Point{X: int(x), Y: int(y)}
	// The popups gate in two deterministic parts: the context menu's popup
	// rect — uikit exposes it because an ebitenui-drawn popup would be
	// invisible to this rect test — and the frame latch, which is the
	// quantity picker's only gate (its window is in the kit's stack, so
	// kit.Over covers the rect below): a popup that was up when the frame's
	// input was sampled owns every gesture on it, so a click outside it
	// never reaches the world.
	if r, open := g.kit.MenuRect(); open && pt.In(r) {
		return true
	}
	if g.menuModal {
		return true
	}
	// The HUD's children are anchored root-overlay chrome, not windows.
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
	return g.kit.Over(pt)
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
	bodyOf[*uikit.LogPanel](g.host, idLog).PushLine(sev, msg)
}

// ---------------------------------------------------------------------------
// Interface construction
// ---------------------------------------------------------------------------

// buildKit constructs the kit, every panel and the root-overlay chrome
// against the current sim. LoadGame re-runs it: panels hold sim
// references, so a restore rebuilds the whole interface (selection
// resets; the host's open flags survive, LoadGame carries the log history
// across, and LoadGame re-applies the saved window layout after this
// returns).
func (g *Game) buildKit() error {
	kit, err := uikit.NewKit(g.opts.Content, g.s, func(cmd sim.Command) { g.enqueue(cmd) })
	if err != nil {
		return fmt.Errorf("game: uikit.NewKit: %w", err)
	}
	g.kit = kit
	// The write seam is installed before anything is built from it: every
	// screen's intent lands on Game.Do. One session per build — the kit's
	// own theme, view and sink, handed to every extra panel (uikit.NewEnv
	// would build a second one the kit never uses).
	kit.SetSink(g)
	g.env = kit.Env()
	env := g.env

	g.hud = uikit.NewHud(env)
	g.rack = uikit.NewRack(env)
	// The rack is the right half of the HUD's bottom-center pair: the HUD
	// owns the slot, the rack keeps its own container (Sync, hover, rects).
	g.hud.MountRack(g.rack.Container())
	// Click-activation targets the player's selected entity first — the
	// same source the F-key module toggles use — before falling back to
	// the first suitable lock.
	g.rack.SetTargetPicker(func() ecs.Entity { return kit.Selected() })

	// Always-on chrome mounts straight onto the kit's root overlay — no
	// full-screen wrapper windows: the HUD stretches across the root and
	// anchors its own pair (status + rack) and target row, the navigation
	// rail the left edge. The rack itself rides the HUD's pair
	// (MountRack), so it is not a root child. (The log used to anchor
	// here too; it is a window now and mounts through syncMounts like
	// the other panels.)
	kit.AddContainer(g.hud.Container(), widget.AnchorLayoutData{
		StretchHorizontal: true,
		StretchVertical:   true,
	})
	// The rail is derived from the spec in one pass (game/rail.go) and
	// pinned to the left edge, top-flush with the window row below the
	// top margin. It mounts as TOP chrome (AddTopContainer): above every
	// window in paint and input, under the context menu — its left
	// gutter (uikit.RailInset) is what every default window keeps clear.
	// Its collapse state comes back from settings.json on every rebuild
	// (boot, load, scale change).
	g.rail = uikit.NewRail(env, railEntries(g))
	kit.AddTopContainer(g.rail, widget.AnchorLayoutData{
		HorizontalPosition: widget.AnchorLayoutPositionStart,
		VerticalPosition:   widget.AnchorLayoutPositionStart,
		Padding:            &widget.Insets{Top: theme.Px(uikit.RailTopInset)},
	})
	g.rail.SetCollapsed(g.railCollapsed)
	// Rack and rail track hover so input.UIHovered covers their padding.
	// The HUD container must not: it spans the whole screen. (The log
	// window gets hover tracking from the window chrome itself.)
	g.rack.Container().GetWidget().TrackHover = true
	g.rail.Container().GetWidget().TrackHover = true

	// The Screen host: one row per window, built and ×-registered in
	// spec order before the first mount. A rebuild drops the old panels
	// (the old kit owns them) but keeps the player's open flags.
	svc := g.gameServices()
	if g.host == nil {
		g.host = newScreenHost(screens, gameScope, kit, svc)
		// The log is open by default (it used to be non-closable
		// chrome); a saved layout may override this once applyLayout
		// runs.
		g.host.SetOpen(idLog, true)
	} else {
		g.host.Rebind(kit, svc)
	}
	g.wireUISounds(g.host)
	g.host.Warm()

	g.placeDefaults(g.w, g.h)
	g.syncMounts()
	if g.title {
		// A rebuild (load, scale change) during the title phase: the old
		// title widgets belonged to the kit just dropped, and the fresh
		// kit's chrome stays hidden until the title is left.
		if g.titleUI != nil {
			g.titleUI.detach()
			g.titleUI.host.Rebind(kit, g.titleServices())
		}
	}
	return nil
}

// wireUISounds hooks a Screen host's mount and unmount edges to the UI
// cue seam: every window mounted through the host (the Esc menu, the
// docked services view, the overlays and the title's own rows) opens and
// closes with its cue. Kit-level one-offs that never mount through the
// host — the order mini-window and the Show Info window — stay silent.
func (g *Game) wireUISounds(h *ScreenHost) {
	h.onMount = func(ScreenID) { g.ui.Play(sfxOpen) }
	h.onUnmount = func(ScreenID) { g.ui.Play(sfxClose) }
}

// gameServices assembles the game-backed bundle a Screen row's Build may
// reach through the host.
func (g *Game) gameServices() screenServices {
	return screenServices{
		Env: g.env,
		Settings: uikit.SettingsActions{
			UIScale:          func() float64 { return g.uiScale },
			UIScaleLadder:    uiScaleSteps[:],
			SetUIScale:       func(rung float64) { g.setUIScale(rung) },
			Fullscreen:       func() bool { return g.fullscreen },
			SetFullscreen:    func(on bool) { g.setFullscreen(on) },
			RailCollapsed:    func() bool { return g.railCollapsed },
			SetRailCollapsed: func(on bool) { g.setRailCollapsed(on) },
			MasterVolume:     func() int { return g.masterVol },
			SetMasterVolume:  func(v int) { g.setMasterVolume(v) },
			MusicVolume:      func() int { return g.musicVol },
			SetMusicVolume:   func(v int) { g.setMusicVolume(v) },
			UIVolume:         func() int { return g.uiVol },
			SetUIVolume:      func(v int) { g.setUIVolume(v) },
			PersistSettings:  func() { g.persistSettings() },
			Shuffle:          func() bool { return g.shuffle },
			SetShuffle:       func(on bool) { g.setShuffle(on) },
			ResetLayout:      func() { g.resetLayout() },
			ResetAll:         func() { g.resetAllSettings() },
		},
		Keys: uikit.KeybindActions{
			Rows:     func() []uikit.KeybindRow { return g.keybindRows() },
			Begin:    func(action string) { g.beginCapture(action) },
			ResetRow: func(action string) { g.resetKey(action) },
			Reset:    func() { g.resetKeys() },
		},
		HelpRows: func() []uikit.KeyRow { return helpRowsFor(g) },
		Menu:     func() widgets.WindowPanel { return winPanel{win: g.buildMenuWindow()} },
		Station: func() widgets.WindowPanel {
			win := g.buildServicesWindow()
			return winPanel{win: win, sync: func() { g.svcUI.Sync() }}
		},
		Close:  g.toggleScreenID,
		Cancel: g.cancelCapture,
	}
}

// placeDefaults (re)places every movable default window for the current
// screen. The kit's core windows are placed too: Overview takes the
// viewport's upper-right corner, Selected Item the top-left — both clear
// of the rail's left gutter (uikit.RailInset — the rail is root chrome
// and windows never sit under it) — so the two never share a corner and
// the default set is pairwise disjoint at 1280×720 and at the 1024×700
// minimum (TestVirginPlacementTable).
// Every Screen row then places itself from its declared DefaultRect +
// Anchor (anchorRect): the log's bottom-left pocket, the scanner in the
// centre column under the top row, the on-demand overlays centred, help
// top-center, the market/inventory/fitting their own per-type defaults
// and the services view its station column (re-placed to the board's
// content height by contracts.go). Nothing is composed specially for the
// docked state.
//
// Every placement goes through ScreenHost.Place: a window the player
// dragged, or one restored from the layout sidecar, keeps its
// position — so recomposing on a viewport change never stomps an
// arrangement.
func (g *Game) placeDefaults(w, h int) {
	place := g.host.Place
	// The rail column fits the viewport: cap it to the room under its
	// top inset (plus the bottom margin every window keeps), so the
	// entry column clips + wheel-scrolls inside the screen instead of
	// running past the bottom edge at the design sizes (twelve 63 px
	// cells outrun 720). Design-unit margins through the scale funnel.
	if g.rail != nil {
		g.rail.SetMaxHeight(h - theme.Px(uikit.RailTopInset) - theme.Px(10))
	}
	// Design-unit margins composed through the scale funnel: at scale 1
	// these are exactly the historical pixels (the layout tests pin them).
	//
	// The two core windows hold opposite top corners: the Overview — the
	// table the player reads first — takes the viewport's upper-right
	// corner, and the Selected Item window (the verbs for the current
	// selection) the top-left, clear of the rail's gutter. The two are
	// 380 + 464 design px wide, so no corner can hold both at the
	// 1024×700 minimum; the split keeps every default disjoint there.
	ov := g.kit.OverviewWindow()
	if r := ov.GetContainer().GetWidget().Rect; r.Dx() > 0 {
		x, y := w-theme.Px(10)-r.Dx(), theme.Px(8)
		place(ov, image.Rect(x, y, x+r.Dx(), y+r.Dy()))
	}
	sp := g.kit.SelectedWindow()
	if r := sp.GetContainer().GetWidget().Rect; r.Dx() > 0 {
		x, y := theme.Px(8+uikit.RailInset), theme.Px(8)
		place(sp, image.Rect(x, y, x+r.Dx(), y+r.Dy()))
	}
	// The screens themselves: declared geometry, one rule per row. The
	// log keeps its bottom-left pocket — its content floor (logMinW = 260
	// design px) is wider than the pocket left of the HUD pair at the
	// 1024×700 minimum, so that one overlap is inherent (see
	// TestVirginPlacementTable); every other row clears the rail gutter
	// and stays on screen.
	g.host.PlaceDefaults(w, h)
	// The Show Info window floats: centered like the other on-demand
	// overlays (its sidecar rect wins once the player has one, and it is
	// hidden until the first Show anyway). It is a geometry-only sidecar
	// special, not a Screen row — see layoutEntry.
	if r := g.kit.InfoWindow().GetContainer().GetWidget().Rect; r.Dx() > 0 && r.Dy() > 0 {
		place(g.kit.InfoWindow(), centerRect(r.Dx(), r.Dy(), w, h))
	}
	// The order mini-window floats too: centred on first use. It is a
	// transient dialog with no sidecar rect (never persisted), and
	// ScreenHost.Place still refuses to stomp a dragged position.
	if ow := g.kit.OrderWindow(); ow != nil {
		if r := ow.GetContainer().GetWidget().Rect; r.Dx() > 0 && r.Dy() > 0 {
			place(ow, centerRect(r.Dx(), r.Dy(), w, h))
		}
	}
}

// helpRows converts KeyMap into the help panel's row shape (the shared
// "key — description" strings split on the em dash). The session variant
// (helpRowsFor) overlays the player's overrides.
func helpRows() []uikit.KeyRow { return helpRowsFor(nil) }

// ---------------------------------------------------------------------------
// Draw
// ---------------------------------------------------------------------------

// Draw renders the scene stack (world first, then overlay veils), then
// the whole interface on top. At the title the phase's own backdrop — the
// embedded artwork plate, or the static world when there is none — sits
// under the title chrome, and the kit draws only the title's own widgets:
// gameplay chrome is unmounted for the phase.
func (g *Game) Draw(dst *ebiten.Image) {
	if g.title {
		g.drawTitleBackdrop(dst)
		g.kit.Draw(dst)
		return
	}
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
	case *HelpScene:
		g.pop()
	default:
		g.push(&MenuScene{g: g})
	}
	return nil
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
		KeysDown:    inpututil.AppendPressedKeys(nil),
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

// activeSavePath is where Save writes and Load reads: the configured
// path until a load or a save-as makes another file active.
func (g *Game) activeSavePath() string {
	if g.activeSave != "" {
		return g.activeSave
	}
	return g.SavePath()
}

// SaveGame snapshots the sim to the active save path (default
// os.UserConfigDir()/entieve/save.json) and writes the window-layout
// sidecar next to the configured path, so a restart keeps the layout even
// without a second save.
func (g *Game) SaveGame() error {
	save, err := g.s.Snapshot()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(save, "", "  ")
	if err != nil {
		return err
	}
	path := g.activeSavePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	return g.writeLayout()
}

// SaveAs writes the next free auto-numbered save file in the saves
// directory and makes it the active save. No dialog: save.json while it
// is free, else the smallest save-N.json (N ≥ 2) that does not exist.
func (g *Game) SaveAs() error {
	prev := g.activeSave
	g.activeSave = g.nextSavePath()
	if err := g.SaveGame(); err != nil {
		g.activeSave = prev
		return err
	}
	g.ui.Play(sfxConfirm)
	return nil
}

// nextSavePath is the save-as rule: save.json, else save-2.json,
// save-3.json, … in the configured save's directory.
func (g *Game) nextSavePath() string {
	dir := filepath.Dir(g.SavePath())
	for i := 1; ; i++ {
		name := "save.json"
		if i > 1 {
			name = fmt.Sprintf("save-%d.json", i)
		}
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err != nil {
			return p
		}
	}
}

// LoadGame restores the sim from the active save path mid-run and rebuilds
// the interface against the restored sim (selection resets; open windows
// stay open; the log history is carried across the rebuild). The saved
// window layout is restored after the rebuild, over the fresh defaults.
func (g *Game) LoadGame() error {
	path := g.activeSavePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var save sim.Save
	if err := json.Unmarshal(data, &save); err != nil {
		return err
	}
	// The log is not part of the save: capture it before the rebuild and
	// restore it into the fresh panel afterwards.
	logPanel := bodyOf[*uikit.LogPanel](g.host, idLog)
	lines := logPanel.Lines()
	if err := g.s.Restore(&save); err != nil {
		return err
	}
	if err := g.buildKit(); err != nil {
		return err
	}
	bodyOf[*uikit.LogPanel](g.host, idLog).SetLines(lines)
	g.applyLayout(g.w, g.h) // AFTER the rebuild: fresh windows, fresh mounts
	if g.space != nil {
		g.space.ResetView()
	}
	return nil
}
