package screens

import (
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// itemHead is one tradeable's head, the lead both item-carrying panes open
// with: the hero icon (theme.IconHero, the Show Info item size) over the name
// and the wrapped description, and the hint line alone while nothing is
// selected. The market's right pane and the manufacturing inspector mount
// this one builder, so the two heads cannot drift.
//
// The field names keep their historical "head" prefix: the pane's pins read
// the widgets through them.
type itemHead struct {
	headBox   *widget.Container
	headIcon  *theme.IconBox
	headName  *widget.Text
	headProse *widget.Text
	headHint  *widget.Text

	pal theme.Palette
}

// newItemHead builds the head. hint is the line it shows alone while nothing
// is selected ("" for a pane that names its empty state instead).
func newItemHead(env widgets.Seams, hint string) *itemHead {
	p := env.Pal()
	h := &itemHead{
		pal:       p,
		headIcon:  theme.NewIconBox(theme.IconHero),
		headName:  widget.NewText(widget.TextOpts.Text("", env.Face(theme.StepSmall), theme.Solid(p.Accent))),
		headProse: widget.NewText(widget.TextOpts.Text("", env.Face(theme.StepTiny), theme.Solid(p.Text))),
		headHint:  widget.NewText(widget.TextOpts.Text(hint, env.Face(theme.StepTiny), theme.Solid(p.Dim))),
	}
	h.headIcon.Set("", p.Text)
	h.headIcon.Widget().GetWidget().LayoutData = widget.AnchorLayoutData{
		HorizontalPosition: widget.AnchorLayoutPositionCenter,
		VerticalPosition:   widget.AnchorLayoutPositionCenter,
	}
	iconRow := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout()))
	iconRow.AddChild(h.headIcon.Widget())
	h.headBox = widget.NewContainer(widget.ContainerOpts.Layout(widget.NewGridLayout(
		widget.GridLayoutOpts.Columns(1),
		widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, false, false, false}),
		widget.GridLayoutOpts.Spacing(0, theme.Px(theme.SpSm)),
	)))
	h.headBox.AddChild(iconRow, h.headName, h.headProse, h.headHint)
	return h
}

// set fills the head for one tradeable: the icon box shows while hasSel, the
// name and the description while they carry text, the hint while nothing is
// selected. The description wraps to inner design px.
func (h *itemHead) set(hasSel bool, icon, name, prose string, inner int) {
	h.headIcon.Set(icon, h.pal.Text)
	h.headName.Label = name
	h.headProse.Label = prose
	theme.ShowWidget(h.headIcon.Widget().GetWidget(), hasSel)
	theme.ShowWidget(h.headName.GetWidget(), name != "")
	theme.ShowWidget(h.headProse.GetWidget(), prose != "")
	theme.ShowWidget(h.headHint.GetWidget(), !hasSel && h.headHint.Label != "")
	if prose != "" {
		h.headProse.MaxWidth = float64(inner)
	}
	h.headBox.RequestRelayout()
}

// itemProse is a tradeable's description ("" when no def or no prose backs
// it): the item, module or hull def's own text, the head's own read.
func itemProse(cv widgets.ContentView, item content.ItemID) string {
	d := cv.Defs()
	if d == nil {
		return ""
	}
	if def, ok := d.Item(item); ok {
		return def.Description
	}
	if def, ok := d.Module(content.ModuleID(item)); ok {
		return def.Description
	}
	if def, ok := d.Ship(content.ShipID(item)); ok {
		return def.Description
	}
	return ""
}
