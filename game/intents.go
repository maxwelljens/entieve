package game

import (
	"github.com/entieve/entieve/uikit/widgets"
)

// The write seam (CONTEXT.md: ActionSink): every screen expresses "do
// this" as a typed intent, and Game.Do executes it. buildKit installs
// this sink on the kit (Kit.SetSink), so the panels the kit builds and
// every extra panel constructed from g.env land here.

// Do executes one intent — Game's ActionSink implementation. The game
// handles only the intents only it can: the Screen-spec toggle and the
// Esc menu's verbs. Everything else — the session intents (Show Info,
// the context menu) and the sim commands — runs on the kit's own
// executor (Kit.RunIntent), which maps intents onto commands through the
// single shared table (uikit's commandFor) and queues them on the
// game's enqueue tail. The world verbs (uikit/screens/worldverbs.go)
// emit these same intents, so the key path, the Selected Item buttons
// and the world clicks share this one dispatch.
func (g *Game) Do(i widgets.Intent) {
	switch v := i.(type) {
	case widgets.Toggle:
		g.toggleScreen(v.Screen)
	case widgets.Save:
		g.report(g.SaveGame(), "Game saved")
	case widgets.Load:
		g.pendingLoad = true
	case widgets.Quit:
		g.quit = true
	case widgets.Buy, widgets.Sell, widgets.Transfer, widgets.StartJob, widgets.Refine:
		// The quantity verbs: routed exactly as before, with the confirm
		// cue on the press that lands them.
		g.ui.Play(sfxConfirm)
		g.kit.RunIntent(v)
	default:
		g.kit.RunIntent(i)
	}
}

// toggleScreen flips the Screen row titled name (case-insensitive). The
// non-modal rows flip their host flag exactly like their key does; the
// modal rows are scene-owned, so they push/pop the scene their key drives
// (Settings is the plain flag, except inside the pause menu's session).
func (g *Game) toggleScreen(name string) {
	id, ok := g.host.ByTitle(name)
	if !ok {
		// Not a Screen row: the two core windows (the rail's Overview /
		// Selected Item entries) toggle through the kit; anything else
		// is not a window this game opens.
		g.kit.ToggleCore(name)
		return
	}
	g.toggleScreenID(id)
}

// toggleScreenID is the close semantics one row's title/rail key/× runs.
func (g *Game) toggleScreenID(id ScreenID) {
	switch id {
	case idHelp:
		g.toggleHelp()
	case idMenu:
		if g.menuUp() {
			g.closeMenu()
		} else {
			g.push(&MenuScene{g: g})
		}
	case idSettings:
		// Within the menu's settings session the row belongs to the
		// session: closing it (×) returns to the pause menu. From the
		// pause menu it opens that session; everywhere else — the P key,
		// the title host — it is the plain flag toggle.
		switch {
		case g.settingsUp():
			g.exitSettings()
		case g.menuUp():
			g.enterSettings()
		default:
			g.host.Toggle(id)
		}
	default:
		g.host.Toggle(id)
	}
}

// menuUp reports whether the pause menu is on the scene stack. The menu can
// sit UNDER another scene (the docked station is pushed above it), so the top
// scene is not the test: a top-only check turns every menu verb into a second
// menu pushed above the station, leaving the modal window and its dim up.
func (g *Game) menuUp() bool {
	return g.findScene(func(s Scene) bool { _, ok := s.(*MenuScene); return ok })
}

// closeMenu removes the pause menu wherever it sits; its OnLeave unmounts the
// menu window, so neither the modal input layer nor MenuScene.Draw's dim
// survives it.
func (g *Game) closeMenu() {
	g.removeScene(func(s Scene) bool { _, ok := s.(*MenuScene); return ok })
}

// enterSettings turns the pause menu into its settings session: the menu
// scene and its window go, the SettingsScene takes over the settings window
// as a modal, dimmed session.
func (g *Game) enterSettings() {
	if g.settingsUp() {
		return
	}
	g.closeMenu()
	g.push(&SettingsScene{g: g})
}

// exitSettings ends the session and brings the pause menu back: the
// settings scene closes the window (restoring its plain flag) and the menu
// remounts above the world.
func (g *Game) exitSettings() {
	g.removeScene(func(s Scene) bool { _, ok := s.(*SettingsScene); return ok })
	g.push(&MenuScene{g: g})
}

// settingsUp reports whether the settings session is on the scene stack.
func (g *Game) settingsUp() bool {
	return g.findScene(func(s Scene) bool { _, ok := s.(*SettingsScene); return ok })
}
