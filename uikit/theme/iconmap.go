package theme

import (
	"image/color"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/icons"
)

// Icon sizes on the type grid (see uikit/icons): 16 px inline in table rows
// and readouts, 20 px on buttons, 24 px in title bars and headers.
const (
	IconInline = icons.Inline
	IconButton = icons.Button
	IconTitle  = icons.Title
)

// IconGap is the gap between an icon box and the label beside it.
const IconGap = 4

// IconDisabledA is the opacity a disabled control's icon draws at (40% of
// 255) — the kit's disabled look for icon-only and icon+label controls.
const IconDisabledA = 102

// KindIcon maps an entity kind (sim.Info.Kind and the content celestial
// kinds, which share the same strings) to its semantic icon name. Every
// known kind is mapped; anything else falls back to the neutral gear so a
// new kind can never render a missing-image artifact.
func KindIcon(kind string) string {
	switch kind {
	case sim.KindPlayer, sim.KindShip:
		return "kind_ship" // a pilot's kind is a hull; the row tint says whose
	case sim.KindAsteroid:
		return "kind_asteroid"
	case sim.KindWreck:
		return "kind_wreck"
	case sim.KindStation:
		return "kind_station"
	case sim.KindGate:
		return "kind_gate"
	case sim.KindBelt:
		return "kind_belt"
	case sim.KindStar:
		return "kind_star"
	case sim.KindPlanet:
		return "kind_planet"
	case sim.KindMoon:
		return "kind_moon"
	case sim.KindAnomaly:
		return "kind_anomaly"
	default:
		return icons.Fallback
	}
}

// ModuleIcon maps a module's content kind to its class icon: weapons,
// mining, ewar (web/scram), tank (shield/armor/resist) and utility
// (capacitor/propulsion). An unrecognised kind still is a module, so it
// falls back to the generic module gear rather than the neutral gear.
func ModuleIcon(kind string) string {
	switch kind {
	case content.KindWeapon:
		return "mod_weapon"
	case content.KindMiningLaser:
		return "mod_mining"
	case content.KindStasisWeb:
		return "mod_web"
	case content.KindWarpScram:
		return "mod_scram"
	case content.KindShieldBoost, content.KindShieldExtender,
		content.KindArmorRep, content.KindArmorPlate, content.KindResistCoat:
		return "mod_defense"
	case content.KindCapBooster, content.KindCapRecharger, content.KindAfterburner:
		return "mod_utility"
	default:
		return "row_module"
	}
}

// ContractIcon maps a contract's objective kind (kill/deliver/mine) to its
// board-row icon.
func ContractIcon(kind string) string {
	switch kind {
	case content.ContractKill:
		return "ctr_kill"
	case content.ContractDeliver:
		return "ctr_deliver"
	case content.ContractMine:
		return "ctr_mine"
	default:
		return icons.Fallback
	}
}

// NewIcon builds a fixed size×size icon graphic showing name, tinted with
// tint. Graphics built here are static (a title bar, a rebuilt row); widgets
// that swap icons every sync keep an iconBox instead.
//
// The image is set through GraphicOpts.Image (not Images) because
// Graphic.PreferredSize reports 50×50 unless Image is set — the layout must
// see the real box size.
func NewIcon(name string, size int, tint color.RGBA) *widget.Graphic {
	return widget.NewGraphic(widget.GraphicOpts.Image(icons.Get(name, Px(size), tint)))
}

// newIconBox builds a hidden icon graphic of the given box size; Set shows
// and points it at an icon. size is a DESIGN unit (IconInline/…): the box
// drawn scales with the UI, and the table's width reserves use the same
// Px(size), so the cell math and the box agree at every scale.
func NewIconBox(size int) *IconBox {
	g := widget.NewGraphic(
		widget.GraphicOpts.Image(icons.Get(icons.Fallback, Px(size), color.RGBA{})),
		widget.GraphicOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
			Position: widget.RowLayoutPositionCenter,
		})),
	)
	b := &IconBox{g: g, size: size}
	ShowWidget(g.GetWidget(), false)
	return b
}

// iconBox is an icon graphic plus the state needed to swap what it shows
// cheaply: Sync runs every frame, so Set must cost a string compare when
// nothing changed.
type IconBox struct {
	g    *widget.Graphic
	name string
	size int
	tint color.RGBA
}

// Set points the box at name with the given tint. An empty name hides the
// box, which the row layouts skip entirely (no leftover gap).
func (b *IconBox) Set(name string, tint color.RGBA) {
	vis := widget.Visibility_Show
	if name == "" {
		vis = widget.Visibility_Hide
	}
	if b.name == name && b.tint == tint && b.g.GetWidget().GetVisibility() == vis {
		return
	}
	b.name, b.tint = name, tint
	if name != "" {
		b.g.Image = icons.Get(name, Px(b.size), tint)
	}
	ShowWidget(b.g.GetWidget(), vis == widget.Visibility_Show)
}

// Widget returns the graphic to add to a container (never nil).
func (b *IconBox) Widget() *widget.Graphic { return b.g }

// Name returns the icon currently shown ("" when hidden).
func (b *IconBox) Name() string { return b.name }

// NewTitleBar builds the standard window title bar: a leading 24 px icon
// (icon == "" draws the bar text-only) beside the accent title. Returns the
// bar for WindowOpts.TitleBar and the title label, which windows that
// retitle keep a handle on. The title is a tracked mono chrome label.
func NewTitleBar(d Theme, label, icon string) (*widget.Container, *ChromeText) {
	p := d.Pal()
	title := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(Bordered(Solid(p.BGHard), NRGBA(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)
	// The icon is anchored to the bar's start and the title is padded past
	// it, so the two can never overlap however wide the bar gets.
	// Title bars are furniture: they take the SpMd token (the panel
	// margin ContentsPad is denser — the bar gets air the body does not).
	titlePad := SpMd
	if icon != "" {
		g := NewIcon(icon, IconTitle, p.Accent)
		g.GetWidget().LayoutData = widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionStart,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
			Padding:            widget.NewInsetsSimple(Px(SpMd)),
		}
		title.AddChild(g)
		titlePad = SpMd + IconTitle + IconGap
	}
	txt := NewChromeText(d, StepBase, label, p.Accent)
	txt.w.LayoutData = widget.AnchorLayoutData{
		HorizontalPosition: widget.AnchorLayoutPositionStart,
		VerticalPosition:   widget.AnchorLayoutPositionCenter,
		Padding:            widget.NewInsetsSimple(Px(titlePad)),
	}
	title.AddChild(txt)
	return title, txt
}
