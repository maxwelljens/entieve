// Package screens is Entieve's ebitenui-based UI layer: every panel the
// player can open or see (successor to the retired hand-rolled ui/
// package), built on the uikit/widgets mechanics and uikit/theme
// constants. It imports both — never the other way round (the direction
// test in uikit/widgets pins it).
//
// The kit composes widgets from github.com/ebitenui/ebitenui (pinned
// v0.7.3) in the canonical EVE-dark look: see uikit/theme, pinned to
// golden values by TestPaletteGoldenValues. It follows the engine's
// controllability seam: it never mutates sim state. Every screen takes
// exactly the seams it uses — Theme, ContentView, ActionSink
// (uikit/widgets/seams.go) — and expresses every action as a typed
// intent (uikit/widgets/intents.go); the host installs the executor
// (Kit.SetSink), whose commands ride the enqueue callback.
//
// Integration: call Sync every frame before Update, then Draw after the
// world render. World clicks feed selection back through Kit.Select; always
// on chrome mounts via AddContainer (or AddTopContainer for chrome that
// paints above windows), the built-in windows are reachable via
// OverviewWindow/SelectedWindow and can be hidden with SetCoreVisible.
package screens

import (
	"errors"
	"image"
	"strings"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
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
	th   theme.Theme
	cv   widgets.ContentView
	sink widgets.ActionSink
	exec *widgets.Exec // uikit's built-in executor (RunIntent)
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

	// closes is the registered close semantics per window (the × button
	// runs it through startClose); closeBtns is the attached × itself,
	// the idempotence guard for attaching. anims is every running
	// open/close animation (winanim.go) — parked out of ebitenui's
	// window pass and composited by Kit.Draw — with animImg the one
	// offscreen that composite reuses while an animation runs.
	closes      map[*widget.Window]func()
	closeBtns   map[*widget.Window]*widget.Button
	anims       map[*widget.Window]*winAnim
	animImg     *ebiten.Image
	animImgSize image.Point

	// stack is every window this kit has mounted, bottom → top: the paint
	// and input order ebitenui keeps internally, mirrored here because
	// ebitenui exposes no way to read it back. The raise (raisePressed)
	// hit-tests and re-tops through it, and ebitenui's own relayering is
	// switched off for kit windows (AddWindow sets DisableRelayering), so
	// remove + re-mount is the single, deterministic top-of-stack path.
	stack []*kitMount

	// tops is the root-overlay chrome that lives ABOVE the window stack
	// (the navigation rail — AddTopContainer): Kit.Draw parks it out of
	// ui.Draw and paints it after every window, under the menu, and lifts
	// its input layer to the top of the layer stack each frame. The HUD
	// and module rack keep their under-window position (AddContainer).
	tops []TopChrome

	// svc is the shared interaction state every panel built from this
	// kit's Env reaches: the context menu and the Show Info window.
	svc *services

	// infoRemove unmounts the Show Info window (nil = closed). Show
	// remounts it, which is also what raises it to the top of the stack.
	infoRemove widget.RemoveWindowFunc

	// orderWin is the player market-order mini-window (Show-Info-style:
	// one instance, re-targeted and raised on every launch) and
	// orderRemove its mount handle (nil = closed). It is deliberately NOT
	// in the layout sidecar: a transient dialog, never restored. The task
	// queue gates the game's world hotkeys on its OrderTyping.
	orderWin    *OrderWin
	orderRemove widget.RemoveWindowFunc

	// qtyWin is the session's one quantity picker (services.qty — the
	// replacement for every retired ± ticker) and qtyRemove its mount
	// handle (nil = closed). Also deliberately out of the layout sidecar:
	// a transient dialog, never restored.
	qtyWin    *QtyWin
	qtyRemove widget.RemoveWindowFunc

	// core visibility (hidden e.g. while docked in a station) and the
	// player's own close of a core window (the rail's Overview /
	// Selected Item entries): a window shows when the host shows the
	// core (coreShown) AND the player has not closed it. State lives on
	// the kit, so a rebuild (load, scale change) starts open.
	coreClosed map[string]bool
	coreShown  bool

	// topsHidden suppresses the top chrome entirely (the title screen
	// draws no gameplay chrome): drawTops parks the rail instead of
	// re-showing it every frame.
	topsHidden bool
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
		th:      theme.NewSeam(theme.DefaultPalette(), theme.ScaleTypeScale(theme.DefaultTypeScale())),
	}
	// The kit is the app's only cursor writer (widgets.ClaimCursor /
	// ApplyCursor — one OS write per frame, after all input). ebitenui's
	// own AfterDraw reset would be a second writer alternating the shape
	// every frame; nothing in the app uses its cursor names or images, so
	// its management is switched off.
	input.CursorManagementEnabled = false
	k.cv = widgets.NewView(c, s)
	// The built-in executor: session intents on the kit's own services,
	// command intents on enqueue at the selectors' current ranges.
	k.exec = widgets.NewExec(
		k.ShowInfo,
		func(entries []widgets.MenuEntry, at image.Point) { k.svc.menu.Open(entries, at) },
		enqueue,
		func() (float64, float64) { return k.WarpRange(), k.OrbitRange() },
	)
	k.sink = widgets.SinkFunc(k.RunIntent)

	// Per-kit chrome registry: a kit built by a save/load rebuild must
	// not disturb any kit still pumping (they own disjoint windows).
	k.chromes = map[*widget.Window]*winChrome{}
	k.closes = map[*widget.Window]func(){}
	k.closeBtns = map[*widget.Window]*widget.Button{}
	k.anims = map[*widget.Window]*winAnim{}
	k.coreClosed = map[string]bool{}
	k.coreShown = true

	// Shared interaction services first: Kit.Env hands them to every
	// panel (menu + Show Info), and the kit takes over the window's
	// lifecycle (mount/raise on Show, unmount on Close).
	k.svc = &services{}
	k.sess = &session{Seams: k.seams(), svc: k.svc}
	k.svc.menu = widgets.NewMenu(k.sess)
	k.svc.info = NewInfoWin(k.sess)
	k.svc.show = k.ShowInfo
	k.svc.info.onClose = k.CloseInfo
	// Show Info's × registers the same close it always ran (CloseInfo:
	// unmount, rect survives for the sidecar, never auto-opens) — now
	// through the one kit-wide close seam, which adds the fade.
	k.SetWindowClose(k.svc.info.Win(), k.CloseInfo)

	// The order mini-window: one instance, launched from item menus
	// (OpenOrder), closed by its × / Cancel and by the host on undock.
	k.orderWin = NewOrderWin(k.seams())
	k.orderWin.onClose = k.CloseOrder
	k.SetWindowClose(k.orderWin.Win(), k.CloseOrder)

	// The quantity picker: one instance on the session's services, opened
	// by the panels' QtyPick intent (RunIntent), closed by its × / Cancel
	// / Esc and by Confirm.
	k.qtyWin = NewQtyWin(k.seams())
	k.svc.qty = k.qtyWin
	k.qtyWin.onClose = k.CloseQty
	k.svc.qtyShow = k.OpenQty
	k.SetWindowClose(k.qtyWin.Win(), k.CloseQty)

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
	// The two core windows' × closes through the rail's own toggle
	// (ToggleCore) — hiding, not unmounting; reopening stays the rail's.
	k.SetWindowClose(k.ov.win, func() { k.ToggleCore("Overview") })
	k.SetWindowClose(k.sp.win, func() { k.ToggleCore("Selected Item") })
	k.AddWindow(k.ov.win)
	k.AddWindow(k.sp.win)

	k.Sync()
	return k, nil
}

// Update advances the UI (input, layout bookkeeping, deferred events)
// and pumps the window chrome (resize grips and title dragging). Call it
// from ebiten.Game.Update.
//
// The context menu owns its frame while it is up: its own input is
// polled here (the host's earlier MenuInput pass already spent the
// frame's click/Esc — this one is the fallback for hosts that do not
// drive it) and ui.Update is skipped, so the click or Esc that
// dismisses the popup never reaches the widgets underneath. The window
// raise is gated the same way: a dismiss click can never re-stack what
// lies beneath the popup.
//
// This is the care-taker half of the consumed-gesture latch (the contract
// is on widgets.Menu): Menu.Poll sets it, TakeBlocked clears it — once per
// frame, here — so the next frame's poll is live again.
func (k *Kit) Update() error {
	k.svc.menu.Poll()
	menuBlocked := k.svc.menu.TakeBlocked()
	k.pickPoll() // the picker's own Esc (hosts that skip MenuInput still cancel)
	if !k.svc.menu.IsOpen() && !menuBlocked {
		k.raisePressed()
		k.ui.Update()
	}
	k.stepChromes()
	return nil
}

// Draw renders the interface onto screen. Call it AFTER the world render.
//
// Draw order (the stacking contract, bottom → top): the animating
// windows parked out of the window pass and the top chrome parked with
// them → ui.Draw (root overlay, then every settled window, then
// ebitenui's deferred popups) → the animations unparked and the chrome
// recompute → the fade composite (above the settled windows, below the
// rail: it paints windows, so windows are its place in the order) → the
// top chrome re-shown and painted over every window (its input layer
// lifts with it in drawTops) → the context menu, always the last pixels.
func (k *Kit) Draw(screen *ebiten.Image) {
	k.parkAnims()
	for _, t := range k.tops {
		t.SetWindowPass(false) // parked: ui.Draw must not paint it
	}
	k.ui.Draw(screen)
	// Back to shown before the chrome pass: a dragging press must arm
	// against live geometry (see unparkAnims).
	k.unparkAnims()
	k.relayAnimWindows()
	// Second chrome pass: input may have changed while drawing, and both
	// drags recompute their rect absolutely from the press — this pass
	// applies the fresh cursor (or derives an identical rect) so the
	// next frame's hit tests see clean geometry.
	k.stepChromes()
	k.ov.drawTints(screen, k)
	k.drawAnims(screen)
	k.drawTops(screen)
	// The context menu paints last: it is always the topmost pixels,
	// above every window and tooltip.
	k.svc.menu.Draw(screen)
	// The frame's one OS cursor write, after every claimer ran (the pane
	// polls and both chrome passes). ebitenui's own cursor management is
	// off (NewKit), so nothing resets the shape afterwards: exactly one
	// write reaches the OS per frame and a stationary pointer presents
	// the same shape every frame.
	widgets.ApplyCursor()
}

// ---------------------------------------------------------------------------
// Window raise
// ---------------------------------------------------------------------------

// raisePressed brings the window under the cursor to the top of the
// stack on any mouse press — left or right, body, title bar and resize
// chrome alike. ebitenui's own click-to-raise never sees presses in a
// window body: the body's scroll viewport elevates its input layer over
// the window container, so only title-bar presses ever fired it (and it
// is switched off for kit windows anyway — see AddWindow). So the raise
// polls the raw press edge and cursor (widgets' shared seams — the same
// ones Menu.Poll reads) and re-tops through the Show-Info seam: remove
// + re-mount. It runs inside Kit.Update's menu gate, before ui.Update,
// so a modal menu's dismiss click can never re-stack what lies beneath
// the popup.
//
// Selection: the topmost mounted window under the cursor wins when
// windows overlap — that is the window receiving the click, so that is
// the one that rises. Top chrome (the rail) owns its rect outright, and
// a modal window between the top and the candidate owns every gesture
// (its ebitenui layer is fullscreen — input below it is gated, so
// nothing below it may rise either). A window already at the top of the
// stack is a no-op: the remount leak (documented in ShowInfo and
// game.setPanel) is paid only when the stacking actually changes.
func (k *Kit) raisePressed() {
	if !widgets.PressEdge(ebiten.MouseButtonLeft) && !widgets.PressEdge(ebiten.MouseButtonRight) {
		return
	}
	x, y := widgets.PollCursor()
	pt := image.Pt(x, y)
	if k.overTop(pt) {
		return // top chrome (the rail) owns this point — no raise
	}
	for i := len(k.stack) - 1; i >= 0; i-- {
		w := k.stack[i].win.GetContainer().GetWidget()
		if !w.IsVisible() {
			continue // a hidden window keeps its rect but claims nothing
		}
		if pt.In(w.Rect) {
			if i == len(k.stack)-1 {
				return // already the topmost window
			}
			k.raise(k.stack[i].win)
			return
		}
		if k.stack[i].win.Modal {
			return // a modal above the candidate owns every gesture
		}
	}
}

// overTop reports whether pt sits on visible top chrome (the rail).
func (k *Kit) overTop(pt image.Point) bool {
	for _, t := range k.tops {
		if w := t.Container().GetWidget(); w.IsVisible() && pt.In(w.Rect) {
			return true
		}
	}
	return false
}

// Over reports whether pt sits on interface chrome the kit owns: visible
// top chrome or a visible mounted window. Hidden windows keep their last
// rect but claim nothing, and an unmounted window is not in the stack at
// all — the world-input gate's exact-conjunct rule.
func (k *Kit) Over(pt image.Point) bool {
	if k.overTop(pt) {
		return true
	}
	for _, m := range k.stack {
		if w := m.win.GetContainer().GetWidget(); w.IsVisible() && pt.In(w.Rect) {
			return true
		}
	}
	return false
}

// raise re-tops win: the kit's own remove closure (chrome, stack entry,
// ebitenui window) followed by a fresh AddWindow, which appends it on
// top — the deterministic top-of-stack ShowInfo already uses. Running
// from Update, before ui.Update, the next Draw rebuilds the input
// layers with the new order, so paint, input and stack agree.
func (k *Kit) raise(win *widget.Window) {
	m, ok := k.mountOf(win)
	if !ok {
		return
	}
	m.remove()
	// animate=false: a raise is not an open — the remount re-tops an
	// already-visible window and must not restart its fade (the fresh
	// append inside addWindow settles any other running animation, the
	// stack-change rule of CONTEXT.md "Settle").
	k.addWindow(win, false)
}

// modalUp reports whether a modal window is mounted: its ebitenui layer
// is fullscreen, so while it is up the top chrome must keep its normal
// under-window input (the modal keeps gating exactly as it did before
// the chrome was lifted). Paint is unaffected — no modal covers the
// rail's column.
func (k *Kit) modalUp() bool {
	for _, m := range k.stack {
		if m.win.Modal && m.win.GetContainer().GetWidget().IsVisible() {
			return true
		}
	}
	return false
}

// drawTops runs Kit.Draw's top pass: each top chrome comes back from its
// park, paints over every window (its tooltips with it), and its input
// layer is re-added on TOP of the window layers — ui.Draw rebuilt the
// layer stack moments ago, so setup here lands last, matching paint
// priority with input priority. Skipped while a modal window is up: the
// modal's fullscreen layer keeps gating the chrome exactly as before.
func (k *Kit) drawTops(screen *ebiten.Image) {
	if k.topsHidden {
		for _, t := range k.tops {
			t.SetWindowPass(false) // parked: never painted below the windows
		}
		return
	}
	// The deferred-setup executor ebitenui uses internally, wired to run
	// immediately: the subtree's layers must be added right here, in
	// place, not back on ui's queue (which has already drained).
	var next input.DeferredSetupInputLayerFunc
	next = func(s input.SetupInputLayerFunc) { s(next) }
	modal := k.modalUp()
	for _, t := range k.tops {
		t.SetWindowPass(true)
		t.Draw(screen)
		if modal {
			continue
		}
		c, w := t.Container(), t.Container().GetWidget()
		w.ElevateToNewInputLayer(&input.Layer{
			DebugLabel: "top chrome",
			EventTypes: input.LayerEventTypeAll,
			BlockLower: true,
			RectFunc:   func() image.Rectangle { return w.Rect },
		})
		c.SetupInputLayer(next)
	}
}

// Sync refreshes every row and label from the simulation. Call it every
// frame before Update.
func (k *Kit) Sync() {
	k.ov.sync(k)
	k.syncSelected()
	k.svc.info.Sync()
	k.svc.qty.Sync() // the picker's slider input + live preview
}

// seams packages the kit's three seams for the panels it builds: the
// sink rides a stable forwarder, so a host that installs its executor
// afterwards (SetSink) reaches every panel built from this kit.
func (k *Kit) seams() widgets.Seams {
	return widgets.NewSeams(k.th, k.cv, widgets.SinkFunc(func(i widgets.Intent) {
		k.sink.Do(i)
	}))
}

// SetSink installs the host's ActionSink — the game hands itself over so
// every screen's intent lands on Game.Do. nil keeps the current sink.
func (k *Kit) SetSink(s widgets.ActionSink) {
	if s != nil {
		k.sink = s
	}
}

// RunIntent executes one intent on the kit's own behalf: session intents
// (Show Info, the context menu, the order mini-window, the quantity
// picker) run on the kit's services, every other intent maps onto the sim
// command it dispatches to (commandFor) and rides the enqueue tail.
// Game.Do delegates here for whatever it does not handle itself.
func (k *Kit) RunIntent(i widgets.Intent) {
	switch v := i.(type) {
	case widgets.OpenOrder:
		k.OpenOrder(v.Item, v.Side, v.Qty)
		return
	case widgets.QtyPick:
		k.OpenQty(v)
		return
	}
	k.exec.Do(i)
}

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
// with the given anchor layout data (widget.AnchorLayoutData). Root
// chrome mounted here keeps its ordinary position: windows paint and
// take input above it (the HUD and module rack — permanent ship chrome
// that no default window overlaps).
func (k *Kit) AddContainer(c widget.Containerer, layoutData any) {
	c.GetWidget().LayoutData = layoutData
	k.root.AddChild(c)
}

// TopChrome is root-overlay chrome mounted ABOVE the window stack — the
// navigation rail (Kit.AddTopContainer). Its left gutter is what
// default windows keep clear (uikit.RailInset); when a window is
// dragged over the column anyway, this is the contract that keeps the
// rail on top: paint in Kit's own pass, input in its own topmost layer,
// tooltips following it, the context menu still last of all.
type TopChrome interface {
	// Container is the laid-out column: the root anchor layout positions
	// it, the topmost input layer claims it, the raise gate reads it.
	Container() *widget.Container
	// SetWindowPass parks the chrome (and its tooltip windows) out of
	// ui.Draw's window pass (Visibility_Hide_Blocking — still laid out,
	// never painted) and brings it back for Kit.Draw's top pass, so its
	// pixels are composited exactly once, after every window.
	SetWindowPass(on bool)
	// Draw paints the chrome column and its tooltip windows onto screen.
	Draw(screen *ebiten.Image)
}

// AddTopContainer mounts top chrome into the root overlay with the given
// anchor layout data (widget.AnchorLayoutData) — see TopChrome for what
// "top" buys. The HUD and rack, which must stay under windows, mount
// through AddContainer instead.
func (k *Kit) AddTopContainer(t TopChrome, layoutData any) {
	t.Container().GetWidget().LayoutData = layoutData
	k.root.AddChild(t.Container())
	k.tops = append(k.tops, t)
}

// SetTopsVisible shows or hides the top chrome wholesale. It exists for
// the title phase, which draws no gameplay chrome: drawTops re-shows the
// rail every frame otherwise.
func (k *Kit) SetTopsVisible(on bool) { k.topsHidden = !on }

// OverviewWindow returns the built-in Overview window for hit-testing,
// repositioning and visibility control.
func (k *Kit) OverviewWindow() *widget.Window { return k.ov.win }

// SelectedWindow returns the built-in Selected Item window.
func (k *Kit) SelectedWindow() *widget.Window { return k.sp.win }

// InfoWindow returns the Show Info window (the layout sidecar and the
// world-input gate resolve it through here). It is not mounted until the
// first Show.
func (k *Kit) InfoWindow() *widget.Window { return k.svc.info.Win() }

// OverviewTable returns the built-in overview's table (the layout
// sidecar's column prefs resolve it through here).
func (k *Kit) OverviewTable() *widgets.Table { return k.ov.tbl }

// InfoOpen reports whether the Show Info window is up.
func (k *Kit) InfoOpen() bool { return k.svc.info.IsOpen() }

// ShowInfo fills the Show Info window with ref's details and raises it:
// the previous mount (if any) is removed and the window re-added, which
// puts it on top of the window stack. Unresolvable refs change nothing —
// the window keeps whatever it already showed rather than going hollow.
// The remount rides the documented ebitenui window-remount leak (a few
// KB per raise) — the price of a deterministic top-of-stack without
// ebitenui's own relayering.
func (k *Kit) ShowInfo(ref widgets.InfoRef) {
	if !k.svc.info.Show(ref) {
		return
	}
	raised := k.infoRemove != nil
	if raised {
		k.infoRemove()
		k.infoRemove = nil
	}
	// First open fades in; a re-raise of the already-open window is a
	// raise, not an open (addWindow with animate=false); a Show landing
	// while the × close is still fading re-opens it — addWindow drops
	// that pending close (its unmount is no longer owed) and the window
	// fades back in from zero.
	k.infoRemove = k.addWindow(k.svc.info.Win(), !raised)
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

// OpenOrder opens (or re-targets) the player market-order mini-window on
// item: Show-Info-style, one instance — a second launch swaps the target
// in place and raises the window, a first open fades it in (its rect is
// never persisted). The item's side and its sell default quantity come
// from the clicked row; an undocked player opens nothing.
func (k *Kit) OpenOrder(item content.ItemID, side sim.OrderSide, qty int) {
	if k.orderWin == nil || !k.orderWin.Show(item, side, qty) {
		return
	}
	raised := k.orderRemove != nil
	if raised {
		k.orderRemove()
		k.orderRemove = nil
	}
	k.orderRemove = k.addWindow(k.orderWin.Win(), !raised)
}

// CloseOrder closes the order mini-window: unmount and hide (its rect is
// transient and never persisted). Idempotent — safe to call every frame,
// e.g. while the player is undocked.
func (k *Kit) CloseOrder() {
	if k.orderWin == nil {
		return
	}
	if k.orderRemove != nil {
		k.orderRemove()
		k.orderRemove = nil
	}
	k.orderWin.hide()
}

// OrderOpen reports whether the order mini-window is up.
func (k *Kit) OrderOpen() bool { return k.orderWin != nil && k.orderWin.IsOpen() }

// OrderTyping reports whether a price/qty field of the order window holds
// focus — the game gates its world hotkeys on this so typing into a field
// never fires a world action.
func (k *Kit) OrderTyping() bool { return k.orderWin != nil && k.orderWin.Typing() }

// BlurOrder clears focus from the order window's fields (the game's
// Esc-while-typing).
func (k *Kit) BlurOrder() {
	if k.orderWin != nil {
		k.orderWin.Blur()
	}
}

// OrderWindow returns the order mini-window (the world-input gate
// hit-tests it, like InfoWindow). nil only before NewKit finishes.
func (k *Kit) OrderWindow() *widget.Window {
	if k.orderWin == nil {
		return nil
	}
	return k.orderWin.Win()
}

// OrderWin returns the order mini-window panel (the host's key-gate tests
// reach its fields through it).
func (k *Kit) OrderWin() *OrderWin { return k.orderWin }

// OpenQty opens (or re-targets) the quantity picker on req: Show-Info
// style, one instance — a second launch swaps the target in place and
// raises the window, a first open fades it in. It is centred on the
// viewport each time (a transient dialog with no persisted rect).
func (k *Kit) OpenQty(req widgets.QtyPick) {
	if k.qtyWin == nil || !k.qtyWin.Show(req) {
		return
	}
	k.centerQty()
	raised := k.qtyRemove != nil
	if raised {
		k.qtyRemove()
		k.qtyRemove = nil
	}
	k.qtyRemove = k.addWindow(k.qtyWin.Win(), !raised)
}

// CloseQty closes the quantity picker: unmount and hide (its rect is
// transient and never persisted). Idempotent — Cancel, Esc and Confirm all
// land here, and a Cancel/× writes nothing anywhere.
func (k *Kit) CloseQty() {
	if k.qtyWin == nil {
		return
	}
	if k.qtyRemove != nil {
		k.qtyRemove()
		k.qtyRemove = nil
	}
	k.qtyWin.hide()
}

// QtyOpen reports whether the quantity picker is up.
func (k *Kit) QtyOpen() bool { return k.qtyWin != nil && k.qtyWin.IsOpen() }

// QtyWindow returns the quantity picker's window (the world-input gate
// hit-tests it, like InfoWindow).
func (k *Kit) QtyWindow() *widget.Window {
	if k.qtyWin == nil {
		return nil
	}
	return k.qtyWin.Win()
}

// QtyWin returns the quantity picker panel (tests reach its slider and
// Confirm through it).
func (k *Kit) QtyWin() *QtyWin { return k.qtyWin }

// centerQty puts the picker in the middle of the viewport: the root
// overlay is laid out to it, so its rect is the screen. The construction
// rect is the fallback before the first frame.
func (k *Kit) centerQty() {
	root := k.root.GetWidget().Rect
	w := k.qtyWin.Win().GetContainer().GetWidget().Rect
	if root.Empty() || w.Empty() {
		return
	}
	x := root.Min.X + (root.Dx()-w.Dx())/2
	y := root.Min.Y + (root.Dy()-w.Dy())/2
	k.qtyWin.Win().SetLocation(image.Rect(x, y, x+w.Dx(), y+w.Dy()))
}

// SetCoreVisible shows or hides the always-mounted chrome (Overview and
// Selected Item), e.g. hide while docked in a station scene. The host's
// rule composes with the player's own close (ToggleCore): the docked
// veil can never be overridden from the rail, and a player close
// survives it.
func (k *Kit) SetCoreVisible(show bool) {
	k.coreShown = show
	k.applyCore()
}

// ToggleCore flips the player's own open state of a core window by its
// title — the rail's toggle for the two always-on windows, which are
// not Screen spec rows. Reports whether the name is a core window (false
// leaves it untouched).
func (k *Kit) ToggleCore(name string) bool {
	key := strings.ToLower(name)
	if key != "overview" && key != "selected item" {
		return false
	}
	k.coreClosed[key] = !k.coreClosed[key]
	k.applyCore()
	return true
}

// CoreOpen reports whether the player has NOT closed the core window
// titled name (unknown names read as open — they never toggle).
func (k *Kit) CoreOpen(name string) bool {
	return !k.coreClosed[strings.ToLower(name)]
}

// applyCore resolves the two core windows' visibility from the host rule
// and the player's close (state writes only; unchanged widgets are not
// touched, so the per-frame call stays free).
func (k *Kit) applyCore() {
	set := func(w *widget.Window, show bool) {
		if w == nil {
			return
		}
		if show {
			// A rail/dock re-show while the × close is still fading
			// cancels that close: the player reopened the window, so no
			// unmount is owed — it shows at once.
			k.cancelClose(w)
		} else {
			// A host hide (the docked veil) while the open fade runs
			// settles it: the unpark pass would otherwise show the window
			// every frame against the host's hide.
			k.settleOpen(w)
		}
		vis := widget.Visibility_Show
		if !show {
			vis = widget.Visibility_Hide
		}
		if c := w.GetContainer(); c != nil {
			if wc := c.GetWidget(); wc.GetVisibility() != vis {
				wc.SetVisibility(vis)
			}
		}
	}
	if k.ov != nil {
		set(k.ov.win, k.coreShown && k.CoreOpen("Overview"))
	}
	if k.sp != nil {
		set(k.sp.win, k.coreShown && k.CoreOpen("Selected Item"))
	}
}

// MenuInput polls the host's pointer and Esc key for the context menu and,
// if it is up, the quantity picker. It must run BEFORE scene and key
// handling (Game.Update calls it first): it returns whether one of them
// owned this frame's input, which the host latches to gate world input and
// its own key switches. Kit.Update polls the same seam afterwards, so a
// host that never calls it still gets dismissal — just a frame later and
// without the same-frame gate.
func (k *Kit) MenuInput() bool {
	if k.svc.menu.Poll() {
		return true
	}
	return k.pickPoll()
}

// pickPoll runs the quantity picker's modal frame: Esc cancels it — before
// any window close or the Esc menu, because this runs first and latches the
// host's gate — and while it is up the picker owns the frame (its modal
// layer owns the clicks; the keys go to the picker).
func (k *Kit) pickPoll() bool {
	if k.qtyWin == nil || !k.qtyWin.IsOpen() {
		return false
	}
	if qtyEsc() {
		k.CloseQty()
	}
	return true
}

// MenuRect exposes the active popup rect (zero when closed) for the
// host's world-input gate.
func (k *Kit) MenuRect() (image.Rectangle, bool) {
	m := k.svc.menu
	if m == nil || !m.IsOpen() {
		return image.Rectangle{}, false
	}
	return m.Rect(), true
}

// Menu returns the kit's context menu (nil in a bare environment).
func (k *Kit) Menu() *widgets.Menu { return k.svc.menu }
