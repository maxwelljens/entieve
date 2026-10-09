// Package game wires the Entieve client together: Ebitengine window and
// loop, fixed-timestep sim stepping with time acceleration, the scene stack,
// input mapping and save/load.
package game

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/ui"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Default window geometry.
const (
	defaultW = 1280
	defaultH = 720
	minW     = 800
	minH     = 600
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

// Game implements ebiten.Game.
type Game struct {
	opts  Options
	s     *sim.Sim
	ren   *render.Renderer
	u     *ui.UI
	clock Clock

	scenes []Scene
	space  *SpaceScene
	w, h   int // logical screen size (Layout)
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
		u:    ui.New(opts.Content, s, s.Queue),
		w:    defaultW,
		h:    defaultH,
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
// the HUD regions always fit).
func (g *Game) Layout(width, height int) (int, int) {
	if width < minW {
		width = minW
	}
	if height < minH {
		height = minH
	}
	g.w, g.h = width, height
	return width, height
}

// ---------------------------------------------------------------------------
// Frame update
// ---------------------------------------------------------------------------

// Update polls input, drives scenes, steps the sim on a fixed timestep and
// hands drained events to the UI.
func (g *Game) Update() error {
	in := g.buildInput()
	ctrl := held(in.KeysDown, ebiten.KeyControl)

	for _, k := range in.JustPressed {
		// Recompute per key: the Esc branch may have just opened the menu.
		menuOpen := false
		_, menuOpen = g.top().(*MenuScene)
		switch {
		case k == ebiten.KeyEscape:
			if err := g.onEscape(); err != nil {
				return err
			}
		case k == ebiten.KeyN && !menuOpen && !ctrl:
			g.toggleMap()
		case k == ebiten.KeyH && !menuOpen && !ctrl:
			g.toggleHelp()
		case ctrl && k == ebiten.KeyS:
			g.report(g.SaveGame(), "Game saved")
		case k == ebiten.KeyF9:
			g.report(g.LoadGame(), "Game loaded")
		case k == ebiten.KeyBracketLeft || k == ebiten.KeyMinus:
			g.clock.Bump(false)
		case k == ebiten.KeyBracketRight || k == ebiten.KeyEqual:
			g.clock.Bump(true)
		}
	}

	// Top scene consumes the rest of the input (UI keys + scene controls).
	if err := g.top().Update(in); err != nil {
		return err
	}

	// Fixed-timestep stepping at the current time acceleration.
	tps := ebiten.ActualTPS()
	if tps <= 0 || tps > 500 {
		tps = 60
	}
	for i, n := 0, g.clock.Advance(1/tps); i < n; i++ {
		g.s.Step()
	}

	if evs := g.s.Events(); len(evs) > 0 {
		g.onEvents(evs)
		g.u.PushEvents(evs)
	}
	g.syncScenes()
	g.u.SetTimeScale(g.clock.Mult())
	return nil
}

// onEvents reacts to events the UI does not own (view resets on travel).
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

// report logs a save/load outcome into the event log.
func (g *Game) report(err error, okMsg string) {
	sev := sim.SevGood
	msg := okMsg
	if err != nil {
		sev, msg = sim.SevDanger, "Failed: "+err.Error()
	}
	g.u.PushEvents([]sim.Event{{Kind: sim.EvLog, Severity: sev, Time: g.s.Time(), Text: msg}})
}

// ---------------------------------------------------------------------------
// Draw
// ---------------------------------------------------------------------------

// Draw renders world, UI and any overlay scenes in stack order.
func (g *Game) Draw(dst *ebiten.Image) {
	for _, sc := range g.scenes {
		sc.Draw(dst)
	}
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

func (g *Game) toggleMap() {
	if _, ok := g.top().(*MapScene); ok {
		g.pop()
		return
	}
	g.push(&MapScene{g: g})
}

func (g *Game) toggleHelp() {
	if _, ok := g.top().(*HelpScene); ok {
		g.pop()
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

// buildInput snapshots ebiten input into a ui.Input (pure for the UI).
func (g *Game) buildInput() ui.Input {
	cx, cy := ebiten.CursorPosition()
	in := ui.Input{
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
// os.UserConfigDir()/entieve/save.json).
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
	return os.WriteFile(path, data, 0o644)
}

// LoadGame restores the sim from SavePath mid-run and rebinds the UI.
func (g *Game) LoadGame() error {
	data, err := os.ReadFile(g.SavePath())
	if err != nil {
		return err
	}
	var save sim.Save
	if err := json.Unmarshal(data, &save); err != nil {
		return err
	}
	if err := g.s.Restore(&save); err != nil {
		return err
	}
	g.u.Bind(g.s) // sim may be logically rebuilt; clear stale selection
	g.space.ResetView()
	return nil
}
