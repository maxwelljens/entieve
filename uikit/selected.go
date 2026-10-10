package uikit

import (
	"image"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
)

// selectedPanel owns the "Selected Item" window: identity labels plus
// the command buttons that prove the enqueue seam.
type selectedPanel struct {
	win  *widget.Window
	name *widget.Text
	dist *widget.Text

	approach, warp, stop *widget.Button
	lock, dock, jump     *widget.Button
	loot, mine           *widget.Button
}

// buildSelected constructs the Selected Item window and its handlers.
func (k *Kit) buildSelected() {
	p := k.palette
	sp := &selectedPanel{}
	k.sp = sp

	sp.name = widget.NewText(
		widget.TextOpts.Text("No selection", facePtr(k.faces.large), solid(p.Text)),
	)
	sp.dist = widget.NewText(
		widget.TextOpts.Text("", facePtr(k.faces.tiny), solid(p.Dim)),
	)

	newBtn := func(label string, clicked func(*widget.ButtonClickedEventArgs)) *widget.Button {
		return widget.NewButton(
			widget.ButtonOpts.TextLabel(label),
			widget.ButtonOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(0, 40),
				widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			),
			widget.ButtonOpts.ClickedHandler(clicked),
		)
	}
	// Core movement actions (kept from the spike).
	sp.approach = newBtn("Approach", func(*widget.ButtonClickedEventArgs) { k.approachSelected() })
	sp.warp = newBtn("Warp to 10 km", func(*widget.ButtonClickedEventArgs) { k.warpSelected() })
	sp.stop = newBtn("Stop", func(*widget.ButtonClickedEventArgs) { k.stopSelected() })
	// Context actions, gated by the selection's sim.Info flags in
	// syncSelected (disabled buttons take the theme's disabled look).
	sp.lock = newBtn("Lock", func(*widget.ButtonClickedEventArgs) { k.lockSelected() })
	sp.dock = newBtn("Dock", func(*widget.ButtonClickedEventArgs) { k.dockSelected() })
	sp.jump = newBtn("Jump", func(*widget.ButtonClickedEventArgs) { k.jumpSelected() })
	sp.loot = newBtn("Loot", func(*widget.ButtonClickedEventArgs) { k.lootSelected() })
	sp.mine = newBtn("Mine", func(*widget.ButtonClickedEventArgs) { k.mineSelected() })

	row := func(btns ...*widget.Button) *widget.Container {
		c := widget.NewContainer(
			widget.ContainerOpts.Layout(widget.NewRowLayout(
				widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
				widget.RowLayoutOpts.Spacing(contentsPad),
			)),
		)
		for _, b := range btns {
			c.AddChild(b)
		}
		return c
	}

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BG), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(contentsPad),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(contentsPad)),
		)),
	)
	body.AddChild(sp.name)
	body.AddChild(sp.dist)
	body.AddChild(row(sp.approach, sp.warp, sp.stop))
	body.AddChild(row(sp.lock, sp.dock, sp.jump, sp.loot, sp.mine))

	title := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BGHard), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)
	title.AddChild(widget.NewText(
		widget.TextOpts.Text("Selected Item", facePtr(k.faces.base), solid(p.Accent)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionStart,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
			Padding:            widget.NewInsetsSimple(contentsPad),
		})),
	))

	sp.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(title, titleBarH),
		widget.WindowOpts.Draggable(),
		// Wide enough for the two button rows: RowLayout does not wrap,
		// so the min size must keep the content inside the panel.
		widget.WindowOpts.MinSize(300, 240),
		widget.WindowOpts.Location(image.Rect(8, 436, 388, 696)),
	)
}

// syncSelected refreshes the labels and button enablement from the
// current selection: every button is disabled while nothing is
// selected, and the context actions additionally require their
// sim.Info flag (dock/jump/loot/mine/warp), mirroring ui.ActionsFor.
func (k *Kit) syncSelected() {
	sp := k.sp
	info, ok := k.Sim.Info(k.selected)
	if !ok {
		sp.name.Label = "No selection"
		sp.dist.Label = ""
	} else {
		sp.name.Label = info.Name
		sp.dist.Label = formatDistance(info.Distance) + "  ·  " + kindLabel(info.Kind)
	}
	noSel := !ok
	sp.approach.GetWidget().Disabled = noSel
	sp.stop.GetWidget().Disabled = noSel
	sp.warp.GetWidget().Disabled = noSel || !info.CanWarp
	sp.lock.GetWidget().Disabled = noSel || !canLock(info.Kind)
	sp.dock.GetWidget().Disabled = noSel || !info.CanDock
	sp.jump.GetWidget().Disabled = noSel || !info.CanJump
	sp.loot.GetWidget().Disabled = noSel || !info.CanLoot
	sp.mine.GetWidget().Disabled = noSel || !info.CanMine
}

// canLock mirrors ui.canLock: the entity kinds that can be targeted.
func canLock(kind string) bool {
	switch kind {
	case sim.KindShip, sim.KindAsteroid, sim.KindWreck:
		return true
	default:
		return false
	}
}

// approachSelected enqueues CmdApproach for the selected entity.
func (k *Kit) approachSelected() {
	if k.selected.IsZero() {
		return
	}
	k.enqueue(sim.CmdApproach{Target: k.selected})
}

// warpSelected enqueues a 10 km warp to the selected entity.
func (k *Kit) warpSelected() {
	if k.selected.IsZero() {
		return
	}
	k.enqueue(sim.CmdWarpTo{Target: k.selected, Range: 10000})
}

// stopSelected enqueues CmdStop (cancels movement intent).
func (k *Kit) stopSelected() {
	k.enqueue(sim.CmdStop{})
}

// lockSelected enqueues CmdLock for the selected entity.
func (k *Kit) lockSelected() {
	if k.selected.IsZero() {
		return
	}
	k.enqueue(sim.CmdLock{Target: k.selected})
}

// dockSelected enqueues CmdDock at the selected station.
func (k *Kit) dockSelected() {
	if k.selected.IsZero() {
		return
	}
	k.enqueue(sim.CmdDock{Station: k.selected})
}

// jumpSelected enqueues CmdJump through the selected gate.
func (k *Kit) jumpSelected() {
	if k.selected.IsZero() {
		return
	}
	k.enqueue(sim.CmdJump{Gate: k.selected})
}

// lootSelected enqueues CmdLoot (empty item = everything) at the wreck.
func (k *Kit) lootSelected() {
	if k.selected.IsZero() {
		return
	}
	k.enqueue(sim.CmdLoot{Wreck: k.selected})
}

// mineSelected fires the first fitted mining laser at the selected
// asteroid (mirrors ui.UI.mineSelected).
func (k *Kit) mineSelected() {
	if k.selected.IsZero() {
		return
	}
	info, ok := k.Sim.Info(k.selected)
	if !ok || info.Kind != sim.KindAsteroid {
		return
	}
	slot, ok := k.firstMiningSlot()
	if !ok {
		return
	}
	k.enqueue(sim.CmdMine{Asteroid: k.selected, Slot: slot})
}

// firstMiningSlot finds the first fitted mining laser.
func (k *Kit) firstMiningSlot() (sim.SlotRef, bool) {
	fit, ok := k.Sim.Fittings.Get(k.Sim.Player())
	if !ok {
		return sim.SlotRef{}, false
	}
	for _, class := range []string{content.SlotHigh, content.SlotMid} {
		for i, id := range fit.Defs(class) {
			if def, ok := k.Content.Module(id); ok && def.Kind == content.KindMiningLaser {
				return sim.SlotRef{Class: class, Index: i}, true
			}
		}
	}
	return sim.SlotRef{}, false
}
