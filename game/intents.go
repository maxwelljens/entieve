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

// toggleScreen flips the Screen row titled name (case-insensitive) — the
// one path a title/rail key and the Esc menu's buttons share. Every
// window is a row (the modal ones included): the host owns what a modal
// row's toggle means, so no id needs a case here.
func (g *Game) toggleScreen(name string) {
	if id, ok := g.host.ByTitle(name); ok {
		g.host.Toggle(id)
	}
}
