// Package uikit is Entieve's ebitenui-based UI layer (successor to the
// retired hand-rolled ui/ package).
//
// The kit composes widgets from github.com/ebitenui/ebitenui (pinned
// v0.7.3) in the canonical EVE-dark look: see theme.go, pinned to golden
// values by TestPaletteGoldenValues. It follows the engine's
// controllability seam: it never mutates sim state. Every screen takes
// exactly the seams it uses — Theme, ContentView, ActionSink (seams.go) —
// and expresses every action as a typed intent (intents.go); the host
// installs the executor (Kit.SetSink), whose commands ride the enqueue
// callback.
//
// Integration: call Sync every frame before Update, then Draw after the
// world render. World clicks feed selection back through Kit.Select; always
// on chrome mounts via AddContainer; the built-in windows are reachable via
// OverviewWindow/SelectedWindow and can be hidden with SetCoreVisible.
package uikit

import (
	"errors"
	"image"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// Kit is the ebitenui-based interface layer. It never mutates sim state:
// every screen action becomes a typed intent on the ActionSink, which
// maps onto a sim.Command through enqueue.
type Kit struct {
	// Content and Sim are the kit's construction inputs: hosts and tests
	// fixture-write through them. The panels this kit builds read through
	// the seams below (th, cv) and emit through the sink — no screen
	// touches the simulation itself.
	Content *content.Content
	Sim     *sim.Sim

	ui *ebitenui.UI

	// The three seams every panel is built from and emits through: th is
	// the session's one Theme, cv its read-only view, sink the executor
	// the host installed (SetSink) — reached through Kit.seams' stable
	// forwarder, so a panel built before SetSink still lands on it.
	th   Theme
	cv   ContentView
	sink ActionSink
	exec *exec // uikit's built-in executor (RunIntent)
	sess Session

	enqueue func(sim.Command)

	selected ecs.Entity

	ov   *overview
	sp   *selectedPanel
	root *widget.Container

	// chromes is this kit's resize-chrome registry: one entry per window
	// currently mounted through AddWindow, detached again by the remove
	// func it returns. It lives on the kit (never in a package global) so
	// two kits can pump side by side and an unmounted window — hidden
	// mid-drag or closed — is never stepped again. Pumped every frame
	// from Update/Draw.
	chromes map[*widget.Window]*winChrome

	// svc is the shared interaction state every panel built from this
	// kit's Env reaches: the context menu and the Show Info window.
	svc *services

	// infoRemove unmounts the Show Info window (nil = closed). Show
	// remounts it, which is also what raises it to the top of the stack.
	infoRemove widget.RemoveWindowFunc

	// core visibility (hidden e.g. while docked in a station)
	coreHidden bool
}

// NewKit builds the interface layer over loaded content and a live
// simulation. enqueue receives every command the player issues (pass
// sim.Queue directly in the game).
func NewKit(c *content.Content, s *sim.Sim, enqueue func(sim.Command)) (*Kit, error) {
	if c == nil {
		return nil, errors.New("uikit: nil content")
	}
	if s == nil {
		return nil, errors.New("uikit: nil sim")
	}
	if enqueue == nil {
		return nil, errors.New("uikit: nil enqueue callback")
	}
	// Scaled once here: every face, line height and metric derived from
	// the Theme follows the current UI scale (rebuild the kit to change
	// it — see game.applyUIScale).
	k := &Kit{
		Content: c,
		Sim:     s,
		enqueue: enqueue,
		th:      newTheme(DefaultPalette(), scaleTypeScale(DefaultTypeScale())),
	}
	k.cv = newView(c, s)
	// The built-in executor: session intents on the kit's own services,
	// command intents on enqueue at the selectors' current ranges.
	k.exec = &exec{
		show:   k.ShowInfo,
		open:   func(entries []MenuEntry, at image.Point) { k.svc.menu.Open(entries, at) },
		queue:  enqueue,
		ranges: func() (float64, float64) { return k.WarpRange(), k.OrbitRange() },
	}
	k.sink = sinkFunc(k.RunIntent)

	// Per-kit chrome registry: a kit built by a save/load rebuild must
	// not disturb any kit still pumping (they own disjoint windows).
	k.chromes = map[*widget.Window]*winChrome{}

	// Shared interaction services first: Kit.Env hands them to every
	// panel (menu + Show Info), and the kit takes over the window's
	// lifecycle (mount/raise on Show, unmount on Close).
	k.svc = &services{}
	k.sess = &session{Seams: k.seams(), svc: k.svc}
	k.svc.menu = NewMenu(k.sess)
	k.svc.info = NewInfoWin(k.sess)
	k.svc.show = k.ShowInfo
	k.svc.info.onClose = k.CloseInfo

	// Root container: transparent overlay over the world render; windows
	// float above it. The theme rides the widget parent chain.
	root := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout()))
	k.root = root
	k.ui = &ebitenui.UI{Container: root, PrimaryTheme: k.th.WidgetTheme()}

	k.buildOverview()
	k.buildSelected()
	// The kit's own windows mount through AddWindow — not the raw
	// ui.AddWindow — so their resize chrome joins k.chromes too (the
	// kit's AddWindow is what attaches a chrome, see overview.go) and
	// the window trees get validated against the theme. The chrome also
	// makes the resize persistent: the spike has no close affordance.
	k.AddWindow(k.ov.win)
	k.AddWindow(k.sp.win)

	k.Sync()
	return k, nil
}

// Update advances the UI (input, layout bookkeeping, deferred events)
// and pumps the window resize chrome. Call it from ebiten.Game.Update.
//
// The context menu owns its frame while it is up: its own input is
// polled here (the host's earlier MenuInput pass already spent the
// frame's click/Esc — this one is the fallback for hosts that do not
// drive it) and ui.Update is skipped, so the click or Esc that
// dismisses the popup never reaches the widgets underneath.
func (k *Kit) Update() error {
	k.svc.menu.poll()
	menuBlocked := k.svc.menu.takeBlocked()
	if !k.svc.menu.IsOpen() && !menuBlocked {
		k.ui.Update()
	}
	k.stepChromes()
	return nil
}

// Draw renders the interface onto screen. Call it AFTER the world render.
func (k *Kit) Draw(screen *ebiten.Image) {
	k.ui.Draw(screen)
	// Second chrome pass: the built-in title-bar drag moved rects during
	// Render; recompute so the next frame's hit tests see clean geometry.
	k.stepChromes()
	k.ov.drawTints(screen, k)
	// The context menu paints last: it is always the topmost pixels,
	// above every window and tooltip.
	k.svc.menu.Draw(screen)
}

// Sync refreshes every row and label from the simulation. Call it every
// frame before Update.
func (k *Kit) Sync() {
	k.ov.sync(k)
	k.syncSelected()
	k.svc.info.Sync()
}

// seams packages the kit's three seams for the panels it builds: the
// sink rides a stable forwarder, so a host that installs its executor
// afterwards (SetSink) reaches every panel built from this kit.
func (k *Kit) seams() Seams {
	return seams{
		Theme:       k.th,
		ContentView: k.cv,
		ActionSink: sinkFunc(func(i Intent) {
			k.sink.Do(i)
		}),
	}
}

// SetSink installs the host's ActionSink — the game hands itself over so
// every screen's intent lands on Game.Do. nil keeps the current sink.
func (k *Kit) SetSink(s ActionSink) {
	if s != nil {
		k.sink = s
	}
}

// RunIntent executes one intent on the kit's own behalf: session intents
// (Show Info, the context menu) run on the kit's services, every other
// intent maps onto the sim command it dispatches to (commandFor) and
// rides the enqueue tail. Game.Do delegates here for whatever it does
// not handle itself.
func (k *Kit) RunIntent(i Intent) { k.exec.Do(i) }

// Selected returns the entity selected in the overview (zero if none).
func (k *Kit) Selected() ecs.Entity { return k.selected }

// Select sets the selected entity from outside the overview (a world click,
// a keyboard cycle) so the Selected Item panel and the overview highlight
// follow it. Idempotent. Sync runs first: the row objects are rebuilt from
// the grid there, so a click on an entity that only just appeared (its row
// not built yet) resolves to a highlight instead of clearing it.
func (k *Kit) Select(e ecs.Entity) {
	k.selected = e
	k.Sync()
	k.ov.selectEntity(e)
}

// Root returns the kit's root overlay container, for integrations that mount
// always-on chrome (HUD, module rack) directly instead of windows.
func (k *Kit) Root() widget.Containerer { return k.root }

// AddContainer mounts an always-on panel container into the root overlay
// with the given anchor layout data (widget.AnchorLayoutData).
func (k *Kit) AddContainer(c widget.Containerer, layoutData any) {
	c.GetWidget().LayoutData = layoutData
	k.root.AddChild(c)
}

// OverviewWindow returns the built-in Overview window for hit-testing,
// repositioning and visibility control.
func (k *Kit) OverviewWindow() *widget.Window { return k.ov.win }

// SelectedWindow returns the built-in Selected Item window.
func (k *Kit) SelectedWindow() *widget.Window { return k.sp.win }

// InfoWindow returns the Show Info window (the layout sidecar and the
// world-input gate resolve it through here). It is not mounted until the
// first Show.
func (k *Kit) InfoWindow() *widget.Window { return k.svc.info.Win() }

// InfoOpen reports whether the Show Info window is up.
func (k *Kit) InfoOpen() bool { return k.svc.info.IsOpen() }

// ShowInfo fills the Show Info window with ref's details and raises it:
// the previous mount (if any) is removed and the window re-added, which
// puts it on top of the window stack. Unresolvable refs change nothing —
// the window keeps whatever it already showed rather than going hollow.
// The remount rides the documented ebitenui window-remount leak (a few
// KB per raise) — the price of a deterministic top-of-stack without
// ebitenui's own relayering.
func (k *Kit) ShowInfo(ref InfoRef) {
	if !k.svc.info.Show(ref) {
		return
	}
	if k.infoRemove != nil {
		k.infoRemove()
		k.infoRemove = nil
	}
	k.infoRemove = k.AddWindow(k.svc.info.Win())
}

// CloseInfo closes the Show Info window: unmount (its rect survives for
// the sidecar) and hide.
func (k *Kit) CloseInfo() {
	if k.infoRemove != nil {
		k.infoRemove()
		k.infoRemove = nil
	}
	k.svc.info.hide()
}

// SetCoreVisible hides or shows the always-mounted chrome (Overview and
// Selected Item), e.g. hide while docked in a station scene.
func (k *Kit) SetCoreVisible(show bool) {
	if k.coreHidden == !show {
		return
	}
	k.coreHidden = !show
	vis := widget.Visibility_Show
	if !show {
		vis = widget.Visibility_Hide
	}
	if c := k.ov.win.GetContainer(); c != nil {
		c.GetWidget().SetVisibility(vis)
	}
	if c := k.sp.win.GetContainer(); c != nil {
		c.GetWidget().SetVisibility(vis)
	}
}
