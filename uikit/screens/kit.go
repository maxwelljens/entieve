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
// paints above windows). The kit builds the two always-on panels
// (Overview and Selected Item) and hands them to the Screen host as
// OverviewPanel/SelectedPanel: the host mounts, places, persists and
// hides them like every other row.
package screens

import (
	"errors"
	"image"

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

	// pointer is this kit's pointer frame — the host's (Game.opts.Pointer,
	// so it survives a kit rebuild) or a fresh OS one. Begin is the host's
	// (first line of Update); Draw applies the frame's one cursor write.
	pointer *widgets.Pointer

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
	// (the navigation rail and the HUD's target-card row —
	// AddTopContainer): Kit.Draw parks it out of ui.Draw and paints it
	// after every window, under the menu, and lifts its input layer to the
	// top of the layer stack each frame. The rest of the HUD (status pair,
	// rack) keeps its under-window position (AddContainer).
	tops []TopChrome

	// backdrop is the one full-viewport layer below every window
	// (AddBackdropWindow, the proficiency tree): Kit.Draw paints it over
	// the world before ui.Draw and parks it for the window pass, Own
	// resolves its space after the window stack and the root chrome, and
	// it carries no window chrome of its own.
	backdrop *widget.Window

	// rows is each mounted window's registered row surface (SetWindowRows):
	// the hit test consults it after the window's widget walk fails, because
	// a table's rows sit in a scroll viewport the walk cannot descend into.
	rows map[*widget.Window]func(image.Point) bool

	// held is the popup latch (the context menu and the quantity picker own
	// the frame): the world gate, the key switches and the raise all read
	// it. popupAt/popupControl are the popup's own answer for the frame's
	// press point, sampled before Poll acted — the press that runs an entry
	// closes the popup and runs its action, and the click cue must still
	// hear that entry.
	held         bool
	popupAt      image.Point
	popupControl bool
	polled       bool // the host's MenuInput already polled this frame

	// ctrl is the frame's Ctrl key, set by the host from its Input snapshot
	// (SetCtrl): a row click reads it for the world's ctrl+click lock.
	ctrl bool

	// svc is the shared interaction state every panel built from this
	// kit's Env reaches: the context menu and the Show Info window.
	svc *services

	// The four dialogs (kitdialog.go): Show Info, the order mini-window,
	// the quantity picker and the contact Details page. Each is one
	// instance, re-targeted and raised on every launch, mounted on first
	// open and unmounted by its close — one lifecycle, one × registration
	// site.
	info    *dialog
	order   *dialog
	qty     *dialog
	contact *dialog
	// ovFilt is the overview's filter window (the title bar's cog): one
	// instance, transient, never in the layout sidecar — its STATE rides
	// the overview's sidecar row instead.
	ovFilt    *dialog
	ovFiltWin *OvFilterWin
	// friendlyFire is the friendly-fire confirmation (built on first open,
	// like the Details page): one instance, transient, opened by the sim's
	// EvFriendlyFire and closed by Confirm, Cancel or ×.
	friendlyFire    *dialog
	friendlyFireWin *FriendlyFireWin
	// profCard is the proficiency card (built on first open): one
	// instance, transient, re-targeted by every node click and closed by
	// Esc, the × or the tree's own close. profCardAt is the click point
	// the card opens beside.
	profCard    *dialog
	profCardWin *ProfInfoWin
	profCardAt  image.Point
	// ovCog is the overview title bar's filter button (the dialog's only
	// door), kept for the tests that press it; ovCogGlyph is the glyph box
	// its cell centres, kept for the alignment pin.
	ovCog      *widget.Button
	ovCogGlyph *widget.Graphic

	// orderWin is the player market-order mini-window's panel. It is
	// deliberately NOT in the layout sidecar: a transient dialog, never
	// restored. The task queue gates the game's world hotkeys on its
	// OrderTyping.
	orderWin *OrderWin

	// qtyWin is the quantity picker's panel (services.qty — the
	// replacement for every retired ± ticker), also deliberately out of
	// the layout sidecar: a transient dialog, never restored.
	qtyWin *QtyWin

	// contactWin is the contact Details dialog's panel (services.contact):
	// transient like the picker, never restored.
	contactWin *ContactWin

	// topsHidden suppresses the top chrome entirely (the title screen
	// draws no gameplay chrome): drawTops parks the rail instead of
	// re-showing it every frame.
	topsHidden bool
}

// KitOption adjusts a Kit at construction (the host's pointer frame).
type KitOption func(*Kit)

// WithPointer gives the kit the host's pointer frame instead of a fresh
// OS one — the game passes the frame it Begins every Update, so the world
// and the interface share one sample.
func WithPointer(p *widgets.Pointer) KitOption {
	return func(k *Kit) { k.pointer = p }
}

// NewKit builds the interface layer over loaded content and a live
// simulation. enqueue receives every command the player issues (pass
// sim.Queue directly in the game).
func NewKit(c *content.Content, s *sim.Sim, enqueue func(sim.Command), opts ...KitOption) (*Kit, error) {
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
	for _, opt := range opts {
		opt(k)
	}
	if k.pointer == nil {
		k.pointer = widgets.NewPointer(nil)
	}
	// The kit is the app's only cursor writer (the pointer frame's Apply —
	// one OS write per frame, after all input). ebitenui's own AfterDraw
	// reset would be a second writer alternating the shape every frame;
	// nothing in the app uses its cursor names or images, so its
	// management is switched off.
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
	k.rows = map[*widget.Window]func(image.Point) bool{}

	// Shared interaction services first: Kit.Env hands them to every
	// panel (menu + Show Info), and the kit takes over the window's
	// lifecycle (mount/raise on Show, unmount on Close).
	k.svc = &services{}
	k.sess = &session{Seams: k.seams(), svc: k.svc}
	k.svc.menu = widgets.NewMenu(k.sess, k.sess.Pointer())
	k.svc.info = NewInfoWin(k.sess)
	k.svc.show = k.ShowInfo
	k.svc.info.onClose = k.CloseInfo
	// Show Info's × registers the same close it always ran (CloseInfo:
	// unmount, rect survives for the sidecar, never auto-opens) — now
	// through the one kit-wide close seam, which adds the fade.
	k.info = k.newDialog(k.svc.info.Win(), k.svc.info.hide, k.svc.info.IsOpen, nil, k.CloseInfo)

	// The order mini-window: one instance, launched from item menus
	// (OpenOrder), closed by its × / Cancel and by the host on undock.
	k.orderWin = NewOrderWin(k.seams())
	k.orderWin.onClose = k.CloseOrder
	k.order = k.newDialog(k.orderWin.Win(), k.orderWin.hide, k.orderWin.IsOpen, nil, k.CloseOrder)

	// The quantity picker: one instance on the session's services, opened
	// by the panels' QtyPick intent (RunIntent), closed by its × / Cancel
	// / Esc and by Confirm. It is the one dialog that re-places on every
	// show (centred on the viewport).
	k.qtyWin = NewQtyWin(k.seams())
	k.svc.qty = k.qtyWin
	k.qtyWin.onClose = k.CloseQty
	k.svc.qtyShow = k.OpenQty
	k.qty = k.newDialog(k.qtyWin.Win(), k.qtyWin.hide, k.qtyWin.IsOpen, k.centerDialog, k.CloseQty)

	// The Details dialog is built on first open (the overview filter's
	// rule): most sessions never open one, and its page is re-rendered on
	// every Show anyway. Only the open hook is wired now.
	k.svc.contactShow = k.OpenContact

	// Root container: transparent overlay over the world render; windows
	// float above it. The theme rides the widget parent chain.
	root := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout()))
	k.root = root
	k.ui = &ebitenui.UI{Container: root, PrimaryTheme: k.th.WidgetTheme()}

	k.buildOverview()
	k.buildSelected()
	// The two always-on windows are Screen rows (game/screens.go, the
	// Persistent facet): the kit builds them (construction, selection
	// wiring and their table) and the host mounts, places, persists and
	// hides them through the very same row path as every other window.
	// Their × is the row's flag (host.build registers it), so a close
	// hides the window and keeps it mounted; reopening stays the rail's.

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
// lies beneath the popup. Skipping ui.Update also skips its input pass,
// so that pass runs by hand in the else branch: ebitenui computes its
// press edge from that cache, and an unspent edge would fire on the
// widget under the popup the next time ui.Update runs — the row behind
// the entry would click too.
//
// This is the care-taker half of the consumed-gesture latch (the contract
// is on widgets.Menu): Menu.Poll sets it, TakeBlocked clears it — once per
// frame, here — so the next frame's poll is live again.
func (k *Kit) Update() error {
	if !k.polled { // the host's MenuInput did not poll this frame: fall back
		k.pollPopup()
	}
	k.polled = false
	menuBlocked := k.svc.menu.TakeBlocked()
	if !k.svc.menu.IsOpen() && !menuBlocked {
		k.raisePressed()
		k.ui.Update()
	} else {
		// The popup owns the frame, so no widget pass runs — but ebitenui's
		// cached input must still advance (ui.Update's own first step): an
		// unspent press edge would otherwise sit in the handler and fire on
		// whatever widget lies under the popup the next time ui.Update runs,
		// i.e. the row behind the entry would click too.
		input.Update()
		input.AfterUpdate()
	}
	k.stepChromes()
	return nil
}

// Draw renders the interface onto screen. Call it AFTER the world render.
//
// Draw order (the stacking contract, bottom → top): the backdrop layer,
// painted straight over the world and parked again for the window pass →
// the animating windows parked out of the window pass and the top chrome
// parked with them → ui.Draw (root overlay, then every settled window, then
// ebitenui's deferred popups) → the animations unparked and the chrome
// recompute → the fade composite (above the settled windows, below the
// rail: it paints windows, so windows are its place in the order) → the
// top chrome re-shown and painted over every window (its input layer
// lifts with it in drawTops) → the context menu, always the last pixels.
func (k *Kit) Draw(screen *ebiten.Image) {
	k.parkAnims()
	// The backdrop paints first and is parked for ui.Draw: ebitenui would
	// otherwise paint it with the window pass, above the root chrome and
	// possibly over windows opened before it.
	if b := k.backdrop; b != nil {
		b.Render(screen)
		b.GetContainer().GetWidget().SetVisibility(widget.Visibility_Hide_Blocking)
	}
	for _, t := range k.tops {
		t.SetWindowPass(false) // parked: ui.Draw must not paint it
	}
	k.ui.Draw(screen)
	// Shown again at once: the hit test and the next frame read it.
	if b := k.backdrop; b != nil {
		b.GetContainer().GetWidget().SetVisibility(widget.Visibility_Show)
	}
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
	// the same shape every frame (Apply repeats, never re-samples).
	k.pointer.Apply()
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
// reads the frame's gated press and pointer (the same frame Menu.Poll
// reads) and re-tops through the Show-Info seam: remove + re-mount. It runs inside Kit.Update's menu gate, before ui.Update,
// so a modal menu's dismiss click can never re-stack what lies beneath
// the popup.
//
// Selection: the walk's own answer (Own) picks the winner — the topmost
// visible window under the cursor, with the popup, a modal window, the top
// chrome (the rail) and the root chrome all claiming a point first. A
// window already at the top of the stack is a no-op: the remount leak
// (documented in showDialog and game.setPanel) is paid only when the
// stacking actually changes.
func (k *Kit) raisePressed() {
	p := k.pointer
	if !p.Press(widgets.Left) && !p.Press(widgets.Right) {
		return
	}
	o := k.Own(p.At())
	// Only a window owns a raise: the popup, the rail, the root chrome and
	// the hover backstop never re-stack anything, and a modal window owns
	// every point through the walk (nothing below it may rise).
	if o.Layer != LayerWindow || o.Window == nil || o.Window == k.stackTop() {
		return
	}
	k.raise(o.Window)
}

// Over reports whether pt sits on interface chrome the kit owns — any layer
// but the world (Own's one walk). Hidden windows keep their last rect but
// claim nothing, and an unmounted window is not in the stack at all.
func (k *Kit) Over(pt image.Point) bool { return k.Own(pt).Layer != LayerWorld }

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
func (k *Kit) modalUp() bool { return k.modalWindow() != nil }

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
	k.svc.qty.Sync()          // the picker's slider input + live preview
	if k.svc.contact != nil { // built on first open
		k.svc.contact.Sync() // the Details page follows the offer it shows
	}
	if k.ovFiltWin != nil { // built on first open
		k.ovFiltWin.Sync()
	}
	if k.profCardWin != nil { // built on first open: the page's rail follows its scroll
		k.profCardWin.Sync()
	}
}

// seams packages the kit's three seams for the panels it builds: the
// sink rides a stable forwarder, so a host that installs its executor
// afterwards (SetSink) reaches every panel built from this kit.
func (k *Kit) seams() widgets.Seams {
	return widgets.NewSeams(k.th, k.cv, widgets.SinkFunc(func(i widgets.Intent) {
		k.sink.Do(i)
	}), k.pointer)
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
	case widgets.ShowContact:
		k.OpenContact(v.Offer)
		return
	case widgets.ShowProfFamily:
		k.OpenProfFamily(v)
		return
	}
	k.exec.Do(i)
}

// Selected returns the entity selected in the overview (zero if none).
func (k *Kit) Selected() ecs.Entity { return k.selected }

// SetCtrl records this frame's Ctrl key (the host's Input snapshot): a
// ctrl+click on a row locks, exactly as ctrl+click in the world does.
func (k *Kit) SetCtrl(on bool) { k.ctrl = on }

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
// take input above it (the HUD's status pair and the module rack — ship
// chrome at the screen's bottom, where no default window sits).
func (k *Kit) AddContainer(c widget.Containerer, layoutData any) {
	c.GetWidget().LayoutData = layoutData
	k.root.AddChild(c)
}

// TopChrome is root-overlay chrome mounted ABOVE the window stack — the
// navigation rail and the HUD's target-card row (Kit.AddTopContainer).
// The rail's left gutter is what default windows keep clear
// (uikit.RailInset); the cards sit where the doubled Overview's body
// now lands. When a window covers either anyway, this is the contract
// that keeps the chrome on top: paint in Kit's own pass, input in its
// own topmost layer, tooltips following it, the context menu still last
// of all.
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

// AddBackdropWindow mounts a full-viewport layer: it paints above the
// world and BELOW the root chrome and every window (Kit.Draw's first
// pass), takes no window chrome (no grips, no drag, no fade: the layer is
// the viewport, and its panel re-fits it), and never raises. Own resolves
// its space after the window stack, so a window above it keeps its
// presses while bare layer space never reaches the world. One layer at a
// time; the returned func unmounts it.
func (k *Kit) AddBackdropWindow(w *widget.Window) widget.RemoveWindowFunc {
	if w == nil {
		return func() {}
	}
	k.backdrop = w
	remove := k.addWindow(w, false) // no fade: a layer is not an open window
	if ch, ok := k.chromes[w]; ok {
		ch.cancel()
		delete(k.chromes, w)
	}
	return func() {
		if k.backdrop == w {
			k.backdrop = nil
		}
		remove()
	}
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

// OverviewFilter returns the overview's per-kind filter state — the layout
// sidecar's read/write side. A kind absent from the map is shown and
// coloured (the default, and what a fresh session holds).
func (k *Kit) OverviewFilter() OvFilter { return k.ov.filter }

// SetOverviewFilter restores the state (a sidecar read): default entries
// are dropped so the next write stays empty, and the list re-syncs at once.
// The filter window re-reads its ticks the next time it opens.
func (k *Kit) SetOverviewFilter(f OvFilter) {
	k.ov.filter = normalizeOvFilter(f)
	k.ov.sync(k)
}

// OpenOverviewFilter opens the overview's filter window — the cog in the
// overview title bar is its only door — and raises it when already up. The
// panel is built on this first open, not at boot: its labels would put their
// glyphs in the shared face atlas and shift the title phase's text
// anti-aliasing (a capture baseline no feature should move).
func (k *Kit) OpenOverviewFilter() {
	f := k.filterWin()
	k.showDialog(k.ovFilt, func() bool { f.Show(); return true })
}

// filterWin builds the filter dialog on first use (one instance, like every
// dialog).
func (k *Kit) filterWin() *OvFilterWin {
	if k.ovFiltWin == nil {
		k.ovFiltWin = NewOvFilterWin(k.seams(),
			func(kind string) (bool, bool) { return k.ov.filter.Shown(kind), k.ov.filter.Coloured(kind) },
			func(kind string, show, colour bool) { k.ov.filter.set(kind, show, colour) },
			func() { k.ov.sync(k) })
		k.ovFilt = k.newDialog(k.ovFiltWin.Win(), k.ovFiltWin.hide, k.ovFiltWin.IsOpen, nil, k.CloseOverviewFilter)
	}
	return k.ovFiltWin
}

// CloseOverviewFilter closes the filter window: unmount and hide. Its rect
// is transient (never persisted).
func (k *Kit) CloseOverviewFilter() {
	if k.ovFilt != nil {
		k.closeDialog(k.ovFilt)
	}
}

// OverviewFilterOpen reports whether the filter window is up.
func (k *Kit) OverviewFilterOpen() bool { return k.ovFilt != nil && k.ovFilt.isOpen() }

// OpenFriendlyFire opens the confirmation for one aggressive activation the
// sim refused (EvFriendlyFire carries the slot, the target and the module
// name): the primary button re-dispatches that same CmdActivate with Confirm
// set, Cancel and × decline. The panel is built on first open, like the
// Details page, so no session that never fires on a friend pays for it.
func (k *Kit) OpenFriendlyFire(slot sim.SlotRef, target ecs.Entity, module string) {
	f := k.friendlyFireDialog()
	name := ""
	if info, ok := k.cv.Info(target); ok {
		name = info.Name
	}
	k.showDialog(k.friendlyFire, func() bool {
		f.Show(module, name, func() {
			k.enqueue(sim.CmdActivate{Slot: slot, Target: target, Confirm: true})
			k.CloseFriendlyFire()
		})
		return true
	})
}

// friendlyFireDialog builds the confirm dialog on first use (one instance,
// like every dialog).
func (k *Kit) friendlyFireDialog() *FriendlyFireWin {
	if k.friendlyFireWin == nil {
		k.friendlyFireWin = NewFriendlyFireWin(k.seams())
		k.friendlyFireWin.onClose = k.CloseFriendlyFire
		k.friendlyFire = k.newDialog(k.friendlyFireWin.Win(), k.friendlyFireWin.hide,
			k.friendlyFireWin.IsOpen, k.centerDialog, k.CloseFriendlyFire)
	}
	return k.friendlyFireWin
}

// CloseFriendlyFire closes the confirm dialog (declining: no command is
// sent). Idempotent.
func (k *Kit) CloseFriendlyFire() {
	if k.friendlyFire != nil {
		k.closeDialog(k.friendlyFire)
	}
}

// FriendlyFireOpen reports whether the confirm dialog is up.
func (k *Kit) FriendlyFireOpen() bool {
	return k.friendlyFire != nil && k.friendlyFire.isOpen()
}

// OpenProfFamily opens (or re-targets) the proficiency card on one
// family id, beside the click that asked for it — the tree's node click
// (widgets.ShowProfFamily). One instance, transient like the Details page:
// a click on another node swaps the page, re-places the card beside that
// click and raises the same window.
func (k *Kit) OpenProfFamily(req widgets.ShowProfFamily) {
	if req.Family == "" {
		return
	}
	if k.profCardWin == nil {
		k.profCardWin = NewProfInfoWin(k.seams())
		k.profCardWin.onClose = k.CloseProfFamily
		k.profCard = k.newDialog(k.profCardWin.Win(), k.profCardWin.hide,
			k.profCardWin.IsOpen, k.placeProfCard, k.CloseProfFamily)
	}
	k.profCardAt = req.At
	k.showDialog(k.profCard, func() bool {
		k.profCardWin.Show(req.Family, req.Level, req.Gate)
		return true
	})
}

// placeProfCard puts the card just right and below the click that opened
// it, clamped fully on screen (the tooltip's rule): an info card that opened
// over the graph's middle would hide the nodes the player is picking from.
// The click point is the intent's; a zero point (a bare-session or test
// open) centres it.
func (k *Kit) placeProfCard(win *widget.Window) {
	root := k.root.GetWidget().Rect
	w := win.GetContainer().GetWidget().Rect
	if root.Empty() {
		return
	}
	if w.Empty() {
		// The first mount has not been laid out yet: the panel's floor is
		// its design size.
		w = image.Rect(0, 0, theme.Px(profCardW+2*theme.ContentsPad),
			theme.Px(profCardH+2*theme.ContentsPad+theme.TitleBarH))
	}
	at := k.profCardAt
	if at == (image.Point{}) {
		k.centerDialog(win)
		return
	}
	r := image.Rect(at.X+theme.Px(16), at.Y-theme.Px(12), at.X+theme.Px(16)+w.Dx(), at.Y-theme.Px(12)+w.Dy())
	win.SetLocation(widgets.ClampTipRect(r, root))
}

// CloseProfFamily closes the proficiency card. Idempotent (the tree's close and
// Esc both call it).
func (k *Kit) CloseProfFamily() {
	if k.profCard != nil {
		k.closeDialog(k.profCard)
	}
}

// ProfFamilyOpen reports whether a proficiency card is up (the game's Esc
// cascade backs out of it before the tree).
func (k *Kit) ProfFamilyOpen() bool {
	return k.profCard != nil && k.profCard.isOpen()
}

// ProfFamily returns the family the open card names ("" while closed).
func (k *Kit) ProfFamily() string {
	if k.profCardWin == nil || !k.profCard.isOpen() {
		return ""
	}
	return k.profCardWin.Family()
}

// InfoOpen reports whether the Show Info window is up.
func (k *Kit) InfoOpen() bool { return k.info.isOpen() }

// ShowInfo fills the Show Info window with ref's details and raises it:
// the previous mount (if any) is removed and the window re-added, which
// puts it on top of the window stack. Unresolvable refs change nothing —
// the window keeps whatever it already showed rather than going hollow.
// See showDialog for the raise/reopen rules (and the remount leak).
func (k *Kit) ShowInfo(ref widgets.InfoRef) {
	k.showDialog(k.info, func() bool { return k.svc.info.Show(ref) })
}

// CloseInfo closes the Show Info window: unmount (its rect survives for
// the sidecar) and hide.
func (k *Kit) CloseInfo() { k.closeDialog(k.info) }

// OpenOrder opens (or re-targets) the player market-order mini-window on
// item: Show-Info-style, one instance — a second launch swaps the target
// in place and raises the window, a first open fades it in (its rect is
// never persisted). The item's side and its sell default quantity come
// from the clicked row; an undocked player opens nothing.
func (k *Kit) OpenOrder(item content.ItemID, side sim.OrderSide, qty int) {
	k.showDialog(k.order, func() bool { return k.orderWin.Show(item, side, qty) })
}

// CloseOrder closes the order mini-window: unmount and hide (its rect is
// transient and never persisted). Idempotent — safe to call every frame,
// e.g. while the player is undocked.
func (k *Kit) CloseOrder() { k.closeDialog(k.order) }

// OrderOpen reports whether the order mini-window is up.
func (k *Kit) OrderOpen() bool { return k.order.isOpen() }

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
	if k.order == nil {
		return nil
	}
	return k.order.win
}

// OrderWin returns the order mini-window panel (the host's key-gate tests
// reach its fields through it).
func (k *Kit) OrderWin() *OrderWin { return k.orderWin }

// OpenQty opens (or re-targets) the quantity picker on req: Show-Info
// style, one instance — a second launch swaps the target in place and
// raises the window, a first open fades it in. It is centred on the
// viewport each time (a transient dialog with no persisted rect).
func (k *Kit) OpenQty(req widgets.QtyPick) {
	k.showDialog(k.qty, func() bool { return k.qtyWin.Show(req) })
}

// CloseQty closes the quantity picker: unmount and hide (its rect is
// transient and never persisted). Idempotent — Cancel, Esc and Confirm all
// land here, and a Cancel/× writes nothing anywhere.
func (k *Kit) CloseQty() { k.closeDialog(k.qty) }

// QtyOpen reports whether the quantity picker is up.
func (k *Kit) QtyOpen() bool { return k.qty.isOpen() }

// QtyWindow returns the quantity picker's window (the world-input gate
// hit-tests it, like InfoWindow).
func (k *Kit) QtyWindow() *widget.Window {
	if k.qty == nil {
		return nil
	}
	return k.qty.win
}

// QtyWin returns the quantity picker panel (tests reach its slider and
// Confirm through it).
func (k *Kit) QtyWin() *QtyWin { return k.qtyWin }

// OpenContact opens (or re-targets) the contact Details dialog on one live
// offer: Show-Info-style, one instance — a second launch swaps the offer in
// place and raises the window, a first open fades it in. An offer that is no
// longer live opens nothing.
func (k *Kit) OpenContact(offer content.ContractID) {
	if k.contactWin == nil {
		k.contactWin = NewContactWin(k.seams())
		k.contactWin.onClose = k.CloseContact
		k.svc.contact = k.contactWin
		k.contact = k.newDialog(k.contactWin.Win(), k.contactWin.hide, k.contactWin.IsOpen, nil, k.CloseContact)
	}
	k.showDialog(k.contact, func() bool { return k.contactWin.Show(offer) })
}

// CloseContact closes the Details dialog: unmount and hide (its rect is
// transient and never persisted). Idempotent — safe to call every frame,
// e.g. while the player is undocked, and before the dialog was ever built.
func (k *Kit) CloseContact() {
	if k.contact != nil {
		k.closeDialog(k.contact)
	}
}

// ContactOpen reports whether the Details dialog is up.
func (k *Kit) ContactOpen() bool { return k.contact != nil && k.contact.isOpen() }

// ContactWindow returns the Details window (the world-input gate hit-tests
// it, like InfoWindow). nil only before NewKit finishes.
func (k *Kit) ContactWindow() *widget.Window {
	if k.contact == nil {
		return nil
	}
	return k.contact.win
}

// ContactWin returns the Details panel (tests reach its page through it).
func (k *Kit) ContactWin() *ContactWin { return k.contactWin }

// centerDialog puts a dialog in the middle of the viewport: the root
// overlay is laid out to it, so its rect is the screen. The construction
// rect is the fallback before the first frame. It is the place hook of the
// dialogs that re-place on every show (the picker, the friendly-fire
// confirm) — the others are never re-placed.
func (k *Kit) centerDialog(win *widget.Window) {
	root := k.root.GetWidget().Rect
	w := win.GetContainer().GetWidget().Rect
	if root.Empty() || w.Empty() {
		return
	}
	x := root.Min.X + (root.Dx()-w.Dx())/2
	y := root.Min.Y + (root.Dy()-w.Dy())/2
	win.SetLocation(image.Rect(x, y, x+w.Dx(), y+w.Dy()))
}

// OverviewPanel is the Overview Screen row's panel: the kit built it, the
// Screen host mounts, places, persists and hides it. The window is the
// same one OverviewWindow returns.
func (k *Kit) OverviewPanel() widgets.WindowPanel { return overviewPanel{k} }

// SelectedPanel is the Selected Item Screen row's panel (see
// OverviewPanel).
func (k *Kit) SelectedPanel() widgets.WindowPanel { return selectedSurface{k} }

// MenuInput polls the host's pointer and Esc key for the context menu and,
// if it is up, the quantity picker. It must run BEFORE scene and key
// handling (Game.Update calls it first): it returns whether one of them
// owned this frame's input — Kit.Held, which the host reads to gate world
// input and its own key switches. Kit.Update polls the same seam afterwards,
// so a host that never calls it still gets dismissal — just a frame later
// and without the same-frame gate.
func (k *Kit) MenuInput() bool {
	k.polled = true
	return k.pollPopup()
}

// pollPopup is the frame's one popup poll (MenuInput, Kit.Update's
// fallback): the popup's answer for the frame's press point is sampled
// FIRST — the press that runs an entry closes the popup and runs its
// action, and the click cue must still hear it — then the menu and the
// picker act, and the frame's latch is set.
func (k *Kit) pollPopup() bool {
	k.popupAt = k.pointer.At()
	k.popupControl = k.popupHit(k.popupAt)
	k.held = k.svc.menu.Poll()
	if k.pickPoll() {
		k.held = true
	}
	return k.held
}

// pickPoll runs the quantity picker's modal frame: Esc cancels it — before
// any window close or the Esc menu, because this runs first and latches the
// host's gate — and while it is up the picker owns the frame (its modal
// layer owns the clicks; the keys go to the picker).
func (k *Kit) pickPoll() bool {
	if k.qtyWin == nil || !k.qtyWin.IsOpen() {
		return false
	}
	k.held = true
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

// Pointer returns the kit's pointer frame. The host Begins it once per
// Update (before MenuInput — the popup's first read) and the kit's Draw
// ends with its Apply.
func (k *Kit) Pointer() *widgets.Pointer { return k.pointer }
