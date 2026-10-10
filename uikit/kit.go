// Package uikit is Entieve's ebitenui-based UI layer (successor to the
// retired hand-rolled ui/ package).
//
// The kit composes widgets from github.com/ebitenui/ebitenui (pinned
// v0.7.3) in the canonical EVE-dark look: see theme.go, pinned to golden
// values by TestPaletteGoldenValues. It follows the engine's
// controllability seam: it never mutates sim state — every player action
// becomes a sim.Command handed to the enqueue callback.
//
// Integration: call Sync every frame before Update, then Draw after the
// world render. World clicks feed selection back through Kit.Select; always
// on chrome mounts via AddContainer; the built-in windows are reachable via
// OverviewWindow/SelectedWindow and can be hidden with SetCoreVisible.
package uikit

import (
	"errors"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// Kit is the ebitenui-based interface layer. It never mutates sim state:
// every player action becomes a sim.Command through enqueue.
type Kit struct {
	// Content and Sim are the read surfaces the kit renders from.
	Content *content.Content
	Sim     *sim.Sim

	ui      *ebitenui.UI
	theme   *widget.Theme
	palette Palette
	scale   TypeScale
	faces   kitFaces
	enqueue func(sim.Command)

	selected ecs.Entity

	ov   *overview
	sp   *selectedPanel
	root *widget.Container

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
	k := &Kit{
		Content: c,
		Sim:     s,
		enqueue: enqueue,
		palette: DefaultPalette(),
		scale:   DefaultTypeScale(),
	}
	k.faces = newFaces(k.scale)
	k.theme = NewTheme(k.palette, k.scale)

	// Root container: transparent overlay over the world render; windows
	// float above it. The theme rides the widget parent chain.
	root := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout()))
	k.root = root
	k.ui = &ebitenui.UI{Container: root, PrimaryTheme: k.theme}

	k.buildOverview()
	k.buildSelected()
	// AddWindow validates the window trees against the theme and makes the
	// chrome persistent (the spike has no close affordance).
	k.ui.AddWindow(k.ov.win)
	k.ui.AddWindow(k.sp.win)

	k.Sync()
	return k, nil
}

// Update advances the UI (input, layout bookkeeping, deferred events).
// Call it from ebiten.Game.Update.
func (k *Kit) Update() error {
	k.ui.Update()
	return nil
}

// Draw renders the interface onto screen. Call it AFTER the world render.
func (k *Kit) Draw(screen *ebiten.Image) {
	k.ui.Draw(screen)
	k.ov.drawTints(screen, k)
}

// Sync refreshes every row and label from the simulation. Call it every
// frame before Update.
func (k *Kit) Sync() {
	k.ov.sync(k)
	k.syncSelected()
}

// Selected returns the entity selected in the overview (zero if none).
func (k *Kit) Selected() ecs.Entity { return k.selected }

// Select sets the selected entity from outside the overview (a world click,
// a keyboard cycle) so the Selected Item panel and the overview highlight
// follow it. Idempotent.
func (k *Kit) Select(e ecs.Entity) {
	k.selected = e
	k.ov.selectEntity(e)
	k.Sync()
}

// Root returns the kit's root overlay container, for integrations that mount
// always-on chrome (HUD, module rack, log) directly instead of windows.
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
