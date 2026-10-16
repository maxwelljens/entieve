package game

import (
	"github.com/entieve/entieve/uikit/widgets"
	"strings"
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
// game's enqueue tail.
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
	default:
		g.kit.RunIntent(i)
	}
}

// toggleScreen flips the Screen spec row titled name (case-insensitive).
// The non-modal rows flip their open flag exactly like their key does
// (pressScreen — syncMounts mounts/unmounts from it); the modal rows are
// scene-owned, so they push/pop the scene their key drives.
func (g *Game) toggleScreen(name string) {
	// The title index, not the screens table: the Esc menu's Build
	// reaches Do, and reading the table back through Do would make the
	// screens initializer depend on itself (Go's init cycle).
	// screenByTitle is populated in init, with the other registries.
	s, ok := screenByTitle[strings.ToLower(name)]
	if !ok {
		return
	}
	if s.Modal {
		switch s.ID {
		case pMap:
			g.toggleMap()
		case pHelp:
			g.toggleHelp()
		case pMenu:
			if _, top := g.top().(*MenuScene); top {
				g.pop()
			} else {
				g.push(&MenuScene{g: g})
			}
		}
		return
	}
	g.open[s.ID] = !g.open[s.ID]
}
