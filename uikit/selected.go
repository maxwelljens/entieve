package uikit

import (
	"image"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/sim"
)

// selectedPanel owns the "Selected Item" window: identity labels plus the
// three command buttons that prove the enqueue seam.
type selectedPanel struct {
	win      *widget.Window
	name     *widget.Text
	dist     *widget.Text
	approach *widget.Button
	warp     *widget.Button
	stop     *widget.Button
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
	sp.approach = newBtn("Approach", func(*widget.ButtonClickedEventArgs) { k.approachSelected() })
	sp.warp = newBtn("Warp to 10 km", func(*widget.ButtonClickedEventArgs) { k.warpSelected() })
	sp.stop = newBtn("Stop", func(*widget.ButtonClickedEventArgs) { k.stopSelected() })

	buttons := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(contentsPad),
		)),
	)
	buttons.AddChild(sp.approach, sp.warp, sp.stop)

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
	body.AddChild(buttons)

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
		// Wide enough for the three-button row: RowLayout does not wrap,
		// so the min size must keep the content inside the panel.
		widget.WindowOpts.MinSize(300, 160),
		widget.WindowOpts.Location(image.Rect(556, 8, 556+300, 8+180)),
	)
}

// syncSelected refreshes the labels and button enablement from the current
// selection. Every button is disabled while nothing is selected.
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
	disabled := !ok
	sp.approach.GetWidget().Disabled = disabled
	sp.warp.GetWidget().Disabled = disabled
	sp.stop.GetWidget().Disabled = disabled
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
