package uikit

import (
	"image/color"
	"math"
	"strconv"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// HUD geometry. The ring diameter, arc radii and gaps mirror ui/layout.go's
// hudRingR/hudRingStep at the 21 px type scale (hudH = 132 there, so the
// 120 px ring image leaves the same margins inside the 130 px panel).
const (
	hudRingSize = 120  // ring graphic diameter (px)
	hudRingR    = 56.0 // shield arc radius (outermost layer)
	hudRingStep = 12.0 // radial gap between the three layers
	hudArcW     = 4.0  // arc stroke width (px)
	hudCapMax   = 1000 // capacitor bar resolution (0.1 % steps)
	hudBarMin   = 140  // capacitor bar minimum width (px)
	hudBoxPool  = 7    // target-box slots (hulls top out at 4 locks)
	hudBoxW     = 132  // one target box width (px)
	hudBoxH     = 60   // one target box height (px, fixed)
	hudBoxGap   = 5    // gap between target boxes
)

// Hud is the always-on ship HUD: the shield/armor/structure arc stack around
// a hull silhouette, the capacitor, speed/time/wallet readouts and the
// top-center target-box row. One root container holds both halves — the bar
// anchors bottom-center, the target boxes top-center — so the integrator adds
// a single stretched child to the interface root.
type Hud struct {
	env Env

	container *widget.Container
	targetRow *widget.Container
	bar       *widget.Container
	boxes     [hudBoxPool]*hudBox

	ring     *ebiten.Image
	ringFrac [3]float64
	redraws  int // ring repaint count (TestHudRingRedraws watches it)

	shieldLbl  *widget.Text
	armorLbl   *widget.Text
	structLbl  *widget.Text
	capBar     *widget.ProgressBar
	capValue   *widget.Text
	recharging *widget.Text
	speedLbl   *widget.Text
	timeLbl    *widget.Text
	walletLbl  *widget.Text

	measure   func(string) float64
	timeScale int
}

// hudBox is one locked-target readout: name, lock progress and the three HP
// fraction mini-bars (a single bar for non-ship targets).
type hudBox struct {
	root  *widget.Container
	name  *widget.Text
	lock  *widget.ProgressBar
	layer [3]*widget.ProgressBar
}

// NewHud builds the ship HUD over the given environment. Add the returned
// container to the interface root with an anchor layout; Sync refreshes every
// readout from the sim.
func NewHud(env Env) *Hud {
	p := env.Pal
	h := &Hud{env: env, timeScale: 1}
	faceSmall := env.Face(StepSmall)
	h.measure = func(s string) float64 {
		w, _ := text.Measure(s, *faceSmall, 0)
		return w
	}

	h.shieldLbl = widget.NewText(widget.TextOpts.Text("", faceSmall, solid(p.Shield)))
	h.armorLbl = widget.NewText(widget.TextOpts.Text("", faceSmall, solid(p.Warn)))
	h.structLbl = widget.NewText(widget.TextOpts.Text("", faceSmall, solid(p.Struct)))

	h.capBar = widget.NewProgressBar(
		widget.ProgressBarOpts.Values(0, hudCapMax, hudCapMax),
		widget.ProgressBarOpts.FillImage(&widget.ProgressBarImage{Idle: ns(solid(p.Cap))}),
		widget.ProgressBarOpts.WidgetOpts(widget.WidgetOpts.MinSize(hudBarMin, 20)),
	)
	h.capValue = widget.NewText(widget.TextOpts.Text("", env.Face(StepTiny), solid(p.BGHard)),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter))
	h.recharging = widget.NewText(widget.TextOpts.Text("recharging", env.Face(StepTiny), solid(p.Warn)))
	capStack := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewStackedLayout()),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	capStack.AddChild(h.capBar, h.capValue)

	capCaption := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(5),
		)),
	)
	capCaption.AddChild(
		widget.NewText(widget.TextOpts.Text("capacitor", env.Face(StepTiny), solid(p.Dim))),
		h.recharging,
	)

	mid := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
		)),
	)
	mid.AddChild(h.shieldLbl, h.armorLbl, h.structLbl, capStack, capCaption)

	h.speedLbl = widget.NewText(
		widget.TextOpts.Text("", env.Face(StepBase), solid(p.Text)),
		widget.TextOpts.Position(widget.TextPositionEnd, widget.TextPositionStart),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	h.timeLbl = widget.NewText(
		widget.TextOpts.Text("", faceSmall, solid(p.Accent)),
		widget.TextOpts.Position(widget.TextPositionEnd, widget.TextPositionStart),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	h.walletLbl = widget.NewText(
		widget.TextOpts.Text("", faceSmall, solid(p.Good)),
		widget.TextOpts.Position(widget.TextPositionEnd, widget.TextPositionStart),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	right := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
		)),
	)
	right.AddChild(h.speedLbl, h.timeLbl, h.walletLbl)

	h.ring = ebiten.NewImage(hudRingSize, hudRingSize)
	ringGfx := widget.NewGraphic(widget.GraphicOpts.Image(h.ring))

	bar := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BG), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(10),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(contentsPad)),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionCenter,
			VerticalPosition:   widget.AnchorLayoutPositionEnd,
			Padding:            &widget.Insets{Left: 10, Right: 10, Bottom: 10},
		})),
	)
	bar.AddChild(ringGfx, mid, right)
	h.bar = bar

	// Target boxes: a top-center row, hidden while nothing is locked.
	h.targetRow = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(hudBoxGap),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionCenter,
			VerticalPosition:   widget.AnchorLayoutPositionStart,
			Padding:            &widget.Insets{Top: 10},
		})),
	)
	for i := range h.boxes {
		h.boxes[i] = h.newBox(env)
		h.targetRow.AddChild(h.boxes[i].root)
	}
	h.targetRow.GetWidget().SetVisibility(widget.Visibility_Hide)

	h.container = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			StretchHorizontal: true,
			StretchVertical:   true,
		})),
	)
	h.container.AddChild(h.targetRow, bar)
	return h
}

// newBox builds one target box from a fixed widget pool.
func (h *Hud) newBox(env Env) *hudBox {
	p := env.Pal
	b := &hudBox{}
	b.name = widget.NewText(
		widget.TextOpts.Text("", env.Face(StepSmall), solid(p.Text)),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionStart),
	)
	b.lock = widget.NewProgressBar(
		widget.ProgressBarOpts.Values(0, hudCapMax, 0),
		widget.ProgressBarOpts.FillImage(&widget.ProgressBarImage{Idle: ns(solid(p.Accent))}),
		widget.ProgressBarOpts.WidgetOpts(widget.WidgetOpts.MinSize(0, 5)),
	)
	// Layer colors: shield blue, armor amber, structure grey.
	b.layer = [3]*widget.ProgressBar{
		tintedBar(env, p.Shield, 4),
		tintedBar(env, p.Warn, 4),
		tintedBar(env, p.Struct, 4),
	}

	b.root = widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BG), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(3),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(contentsPad)),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(hudBoxW, hudBoxH)),
	)
	b.root.AddChild(b.name, b.lock, b.layer[0], b.layer[1], b.layer[2])
	return b
}

// tintedBar builds a progress bar filled with a fixed palette color.
func tintedBar(env Env, col color.RGBA, minH int) *widget.ProgressBar {
	return widget.NewProgressBar(
		widget.ProgressBarOpts.Values(0, hudCapMax, hudCapMax),
		widget.ProgressBarOpts.FillImage(&widget.ProgressBarImage{Idle: ns(solid(col))}),
		widget.ProgressBarOpts.WidgetOpts(widget.WidgetOpts.MinSize(0, minH)),
	)
}

// Container returns the HUD's root widget for the interface root (anchor
// layout). It stretches to the root and anchors its own children itself.
func (h *Hud) Container() *widget.Container { return h.container }

// SetTimeScale reports the current time acceleration (1/2/4/8) so the HUD
// can label the multiplier; the game calls it when acceleration changes.
func (h *Hud) SetTimeScale(n int) {
	if n < 1 {
		n = 1
	}
	h.timeScale = n
}

// Sync refreshes every readout — arcs, capacitor, speed, wallet and target
// boxes — from the sim. Call it once per frame before Update.
func (h *Hud) Sync() {
	s := h.env.Sim
	if s == nil {
		return
	}
	player := s.Player()

	// 1 — the three damage-layer arcs (redrawn only when a fraction moved).
	shield, armor, structure := 1.0, 1.0, 1.0
	if sh, ok := s.Ships.Get(player); ok {
		shield = hudFrac(sh.Shield, sh.MaxShield)
		armor = hudFrac(sh.Armor, sh.MaxArmor)
		structure = hudFrac(sh.Structure, sh.MaxStructure)
		h.shieldLbl.Label = "Shield " + hudHP(sh.Shield) + "/" + hudHP(sh.MaxShield)
		h.armorLbl.Label = "Armor " + hudHP(sh.Armor) + "/" + hudHP(sh.MaxArmor)
		h.structLbl.Label = "Structure " + hudHP(sh.Structure) + "/" + hudHP(sh.MaxStructure)
	} else {
		h.shieldLbl.Label = "No ship"
		h.armorLbl.Label = ""
		h.structLbl.Label = ""
	}
	if want := [3]float64{shield, armor, structure}; h.ringFrac != want {
		h.ringFrac = want
		h.drawRing(shield, armor, structure)
	}

	// 2 — capacitor bar + value label + recharge state.
	frac, capLabel := 1.0, ""
	if cap, ok := s.Caps.Get(player); ok && cap.Capacity > 0 {
		frac = hudFrac(cap.Current, cap.Capacity)
		capLabel = groupFloat(cap.Current, 1) + "/" + groupFloat(cap.Capacity, 1) + " GJ"
	}
	h.capBar.SetCurrent(int(frac * hudCapMax))
	h.capValue.Label = capLabel
	showWidget(h.recharging.GetWidget(), frac < 0.999)

	// 3 — speed, time acceleration and wallet.
	speed := 0.0
	if m, ok := s.Motions.Get(player); ok {
		speed = m.Vel.Len()
	}
	h.speedLbl.Label = formatSpeed(speed)
	h.timeLbl.Label = "×" + strconv.Itoa(h.timeScale) + " time"
	h.walletLbl.Label = h.creditsLabel(s.Wallet())

	// 4 — locked-target boxes.
	var locks []sim.Lock
	if tg, ok := s.Targetings.Get(player); ok {
		locks = tg.Locks
	}
	if len(locks) == 0 {
		showWidget(h.targetRow.GetWidget(), false)
		return
	}
	showWidget(h.targetRow.GetWidget(), true)
	for i, box := range h.boxes {
		if i >= len(locks) {
			showWidget(box.root.GetWidget(), false)
			continue
		}
		showWidget(box.root.GetWidget(), true)
		l := locks[i]
		info, hasInfo := s.Info(l.Target)
		name := "…"
		if hasInfo {
			name = Ellipsize(info.Name, hudBoxW-2*contentsPad, h.measure)
		}
		box.name.Label = name
		box.lock.SetCurrent(int(hudFrac01(l.Progress) * hudCapMax))
		if sh, ok := s.Ships.Get(l.Target); ok {
			// Ships report all three layers; other targets fall back to
			// the read model's single health fraction.
			box.layer[0].SetCurrent(int(hudFrac(sh.Shield, sh.MaxShield) * hudCapMax))
			box.layer[1].SetCurrent(int(hudFrac(sh.Armor, sh.MaxArmor) * hudCapMax))
			box.layer[2].SetCurrent(int(hudFrac(sh.Structure, sh.MaxStructure) * hudCapMax))
		} else {
			health := 1.0
			if hasInfo {
				health = info.Health
			}
			box.layer[0].SetCurrent(int(health * hudCapMax))
			// Non-ships: one bar only — the empty rows keep the box height.
			box.layer[1].SetCurrent(0)
			box.layer[2].SetCurrent(0)
		}
	}
}

// creditsLabel renders a wallet balance as "10,000,000.00 cr".
func (h *Hud) creditsLabel(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	v = math.Round(v*100) / 100
	ip := int64(v)
	frac := int64(math.Round((v - float64(ip)) * 100))
	if frac >= 100 { // carry from rounding, e.g. 0.999 -> 1.00
		ip++
		frac = 0
	}
	fs := strconv.FormatInt(frac, 10)
	if frac < 10 {
		fs = "0" + fs
	}
	s := groupInt(ip) + "." + fs + " cr"
	if neg {
		s = "-" + s
	}
	return s
}

// drawRing repaints the ring image: a dim track plus a fraction fill per
// layer, starting at 12 o'clock, and the hull silhouette in the middle.
func (h *Hud) drawRing(shield, armor, structure float64) {
	h.redraws++
	h.ring.Clear()
	c := float64(hudRingSize) / 2
	const twoPi = 2 * math.Pi
	start := -math.Pi / 2
	p := h.env.Pal
	layers := [...]struct {
		rad, frac float64
		col       color.RGBA
	}{
		{hudRingR, shield, p.Shield},
		{hudRingR - hudRingStep, armor, p.Warn},
		{hudRingR - 2*hudRingStep, structure, p.Struct},
	}
	for _, l := range layers {
		track := l.col
		track.A = 55
		render.Arc(h.ring, float32(c), float32(c), float32(l.rad),
			float32(start), float32(start+twoPi), float32(hudArcW), track)
		if l.frac > 0 {
			render.Arc(h.ring, float32(c), float32(c), float32(l.rad),
				float32(start), float32(start+twoPi*l.frac), float32(hudArcW), l.col)
		}
	}
	// Hull silhouette (the legacy four-point ship glyph, scaled to the rings).
	pts := [][2]float64{
		{0, -hudRingR * 0.30},
		{hudRingR * 0.19, hudRingR * 0.24},
		{0, hudRingR * 0.08},
		{-hudRingR * 0.19, hudRingR * 0.24},
	}
	for i := range pts {
		a, b := pts[i], pts[(i+1)%len(pts)]
		vector.StrokeLine(h.ring,
			float32(c+a[0]), float32(c+a[1]),
			float32(c+b[0]), float32(c+b[1]),
			1.5, p.Struct, true)
	}
}

// hudFrac returns cur/max clamped to 0..1 (1 when max is unknown, so an
// unfitted layer reads as "full" like the legacy HUD).
func hudFrac(cur, max float64) float64 {
	if max <= 0 {
		return 1
	}
	return hudFrac01(cur / max)
}

// hudFrac01 clamps f to 0..1.
func hudFrac01(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

// hudHP formats whole hit points from a float HP value.
func hudHP(v float64) string {
	if v < 0 {
		v = 0
	}
	return groupInt(int64(v + 0.5))
}
