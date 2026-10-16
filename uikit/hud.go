package uikit

import (
	"image"
	"image/color"
	"math"
	"strconv"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/icons"
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
	hudTickLen  = 6.0  // gauge tick length across a ring (px)
	hudCapMax   = 1000 // capacitor bar resolution (0.1 % steps)
	hudBarMin   = 140  // capacitor bar minimum width (px)
	hudBoxPool  = 7    // target-box slots (hulls top out at 4 locks)
	hudBoxW     = 132  // one target box width (px)
	hudBoxH     = 83   // one target box height (px, fixed)
	hudBoxGap   = 5    // gap between target boxes
)

// Target-card layout constants. The card (hudBoxW × hudBoxH) insets by
// hudBoxPad; the body then stacks the name line, the distance line, a
// lock-progress strip and the three layer bars. At the default size the
// numbers tile exactly: body 75 = name 21 + gap 2 + dist 19 + gap 1 +
// lock 2 + gap 2 + bars 28, and 28 = 3 × bar 8 + 2 × bar gap 2. The two
// line heights fit IBM Plex Sans line boxes (1.30 em: 20.8 at StepSmall,
// 18.2 at StepTiny).
const (
	hudBoxPad      = 4  // card padding around the body (px)
	hudBoxTextH    = 21 // name line height (px, fits StepSmall)
	hudBoxDistH    = 19 // distance line height (px, fits StepTiny)
	hudBoxDistGap  = 2  // gap between name and distance lines (px)
	hudBoxLockH    = 2  // lock-progress strip height (px)
	hudBoxLockGap  = 1  // gap between distance line and lock strip (px)
	hudBoxBarsGap  = 2  // gap between lock strip and bars (px)
	hudBoxBarGap   = 2  // gap between layer bars (px)
	hudBoxMinNameW = 8  // minimum room left for the name (px)
)

// targetCardBody returns the content area of a target card: the card rect
// inset by hudBoxPad.
func targetCardBody(card image.Rectangle) image.Rectangle {
	return card.Inset(Px(hudBoxPad))
}

// targetCardText returns the name line at the top of the body.
func targetCardText(card image.Rectangle) image.Rectangle {
	b := targetCardBody(card)
	return image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+Px(hudBoxTextH))
}

// hudBoxIcon is the kind glyph box on a card's name line: the inline size
// of the type grid, vertically centred in the line.
const hudBoxIcon = IconInline

// targetCardIcon returns the kind glyph box at the start of the name line.
func targetCardIcon(card image.Rectangle) image.Rectangle {
	b := targetCardBody(card)
	y := b.Min.Y + Px((hudBoxTextH-hudBoxIcon)/2)
	return image.Rect(b.Min.X, y, b.Min.X+Px(hudBoxIcon), y+Px(hudBoxIcon))
}

// targetCardName returns the name text region: everything right of the
// icon box, so the glyph and the label can never overlap.
func targetCardName(card image.Rectangle) image.Rectangle {
	ic := targetCardIcon(card)
	line := targetCardText(card)
	return image.Rect(ic.Max.X+Px(IconGap), line.Min.Y, line.Max.X, line.Max.Y)
}

// targetCardDist returns the distance line under the name.
func targetCardDist(card image.Rectangle) image.Rectangle {
	b := targetCardBody(card)
	y := b.Min.Y + Px(hudBoxTextH) + Px(hudBoxDistGap)
	return image.Rect(b.Min.X, y, b.Max.X, y+Px(hudBoxDistH))
}

// targetCardLock returns the lock-progress strip under the distance line.
func targetCardLock(card image.Rectangle) image.Rectangle {
	b := targetCardBody(card)
	y := b.Min.Y + Px(hudBoxTextH) + Px(hudBoxDistGap) + Px(hudBoxDistH) + Px(hudBoxLockGap)
	return image.Rect(b.Min.X, y, b.Max.X, y+Px(hudBoxLockH))
}

// targetCardBars returns the layer-bar stack: from under the lock strip
// down to the bottom of the body.
func targetCardBars(card image.Rectangle) image.Rectangle {
	b := targetCardBody(card)
	y := b.Min.Y + Px(hudBoxTextH) + Px(hudBoxDistGap) + Px(hudBoxDistH) + Px(hudBoxLockGap) +
		Px(hudBoxLockH) + Px(hudBoxBarsGap)
	return image.Rect(b.Min.X, y, b.Max.X, b.Max.Y)
}

// layerBars tiles r with exactly three stacked bars separated by
// hudBoxBarGap. Heights divide evenly; any division remainder lands on
// the last bar, so the three rects cover r with no gaps and no overlaps.
func layerBars(r image.Rectangle) [3]image.Rectangle {
	var out [3]image.Rectangle
	gap := Px(hudBoxBarGap)
	h := (r.Dy() - 2*gap) / 3
	if h < 1 {
		h = 1
	}
	y := r.Min.Y
	for i := range out {
		bot := y + h
		if i == len(out)-1 {
			bot = r.Max.Y // last bar absorbs the remainder
		}
		if bot > r.Max.Y {
			bot = r.Max.Y
		}
		out[i] = image.Rect(r.Min.X, y, r.Max.X, bot)
		y = bot + gap
	}
	return out
}

// barFill returns the filled part of bar for frac: the left edge stays,
// the right edge moves to Min.X + round(clamp(frac) × width).
func barFill(bar image.Rectangle, frac float64) image.Rectangle {
	w := int(math.Round(hudFrac01(frac) * float64(bar.Dx())))
	return image.Rect(bar.Min.X, bar.Min.Y, bar.Min.X+w, bar.Max.Y)
}

// ringSweep maps an HP fraction to the fill arc's sweep in radians,
// clockwise from the 12 o'clock start. Clamped to 0..1 → 0..2π.
func ringSweep(frac float64) float64 {
	return hudFrac01(frac) * 2 * math.Pi
}

// ringTicks returns the gauge tick angles (radians clockwise from 12
// o'clock) at the quarter, half and three-quarter marks — EVE-style ticks
// that let the eye read a sweep without the numeric label.
func ringTicks() [3]float64 {
	return [3]float64{ringSweep(0.25), ringSweep(0.5), ringSweep(0.75)}
}

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

	measure     func(string) float64
	measureTiny func(string) float64
	timeScale   int
}

// hudBox is one locked-target readout: a self-drawn card image rendered
// from a targetCard state (name, distance, lock progress, layer fractions)
// through the pure geometry above. Sync repaints only on a state change.
type hudBox struct {
	root    *widget.Container
	img     *ebiten.Image
	card    targetCard // staged state (Sync writes)
	drawn   targetCard // last painted state
	painted bool
	draws   int // image repaint count (tests watch it)
}

// targetCard is one card's rendered state. It is deliberately comparable:
// Sync skips the repaint when nothing moved.
type targetCard struct {
	name   string
	dist   string
	kind   string // semantic icon name for the target's kind ("" = none)
	tint   color.RGBA
	lock   float64    // 0..1 lock progress
	layers [3]float64 // shield / armor / hull fractions (ships); health in [0] otherwise
	ships  bool       // three layers (true) vs a single health fraction
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
	faceTiny := env.Face(StepTiny)
	h.measureTiny = func(s string) float64 {
		w, _ := text.Measure(s, *faceTiny, 0)
		return w
	}

	h.shieldLbl = widget.NewText(widget.TextOpts.Text("", faceSmall, solid(p.Shield)))
	h.armorLbl = widget.NewText(widget.TextOpts.Text("", faceSmall, solid(p.Warn)))
	h.structLbl = widget.NewText(widget.TextOpts.Text("", faceSmall, solid(p.Struct)))

	h.capBar = widget.NewProgressBar(
		widget.ProgressBarOpts.Values(0, hudCapMax, hudCapMax),
		widget.ProgressBarOpts.FillImage(&widget.ProgressBarImage{Idle: ns(solid(p.Cap))}),
		widget.ProgressBarOpts.WidgetOpts(widget.WidgetOpts.MinSize(Px(hudBarMin), Px(20))),
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
			widget.RowLayoutOpts.Spacing(Px(5)),
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
		widget.TextOpts.Text("", env.FaceBold(StepBase), solid(p.Text)),
		widget.TextOpts.Position(widget.TextPositionEnd, widget.TextPositionStart),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	h.timeLbl = widget.NewText(
		widget.TextOpts.Text("", env.FaceBold(StepSmall), solid(p.Accent)),
		widget.TextOpts.Position(widget.TextPositionEnd, widget.TextPositionStart),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	h.walletLbl = widget.NewText(
		// Tabular contents face: the wallet is a dynamic number and mono
		// digits keep its right edge steady while it ticks.
		widget.TextOpts.Text("", env.FaceMono(StepSmall), solid(p.Good)),
		widget.TextOpts.Position(widget.TextPositionEnd, widget.TextPositionStart),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	right := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
		)),
	)
	right.AddChild(h.speedLbl, h.timeLbl, h.walletLbl)

	h.ring = ebiten.NewImage(Px(hudRingSize), Px(hudRingSize))
	ringGfx := widget.NewGraphic(widget.GraphicOpts.Image(h.ring))

	bar := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(bordered(solid(p.BG), nrgba(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(Px(10)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(Px(contentsPad))),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionCenter,
			VerticalPosition:   widget.AnchorLayoutPositionEnd,
			Padding:            &widget.Insets{Left: Px(10), Right: Px(10), Bottom: Px(10)},
		})),
	)
	bar.AddChild(ringGfx, mid, right)
	h.bar = bar

	// Target boxes: a top-center row, hidden while nothing is locked.
	h.targetRow = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(Px(hudBoxGap)),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionCenter,
			VerticalPosition:   widget.AnchorLayoutPositionStart,
			Padding:            &widget.Insets{Top: Px(10)},
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

// newBox builds one target card from the fixed widget pool: a container
// holding a single Graphic that displays the card image.
func (h *Hud) newBox(env Env) *hudBox {
	b := &hudBox{img: ebiten.NewImage(Px(hudBoxW), Px(hudBoxH))}
	g := widget.NewGraphic(
		widget.GraphicOpts.Image(b.img),
		widget.GraphicOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	b.root = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(Px(hudBoxW), Px(hudBoxH))),
	)
	b.root.AddChild(g)
	h.paintBox(b)
	return b
}

// setCard stages a card state and repaints the image when it changed.
func (h *Hud) setCard(b *hudBox, c targetCard) {
	b.card = c
	h.paintBox(b)
}

// paintBox redraws the card image iff the staged state differs from the
// last painted one.
func (h *Hud) paintBox(b *hudBox) {
	if b.painted && b.drawn == b.card {
		return
	}
	b.painted = true
	b.drawn = b.card
	b.draws++
	drawTargetCard(b.img, b.card, h.env)
}

// boxLabels fits a card's text: the name to the room right of the kind
// glyph, the distance to its own line.
func (h *Hud) boxLabels(name, dist string) (string, string) {
	card := image.Rect(0, 0, Px(hudBoxW), Px(hudBoxH))
	room := float64(targetCardName(card).Dx())
	if room < float64(Px(hudBoxMinNameW)) {
		room = float64(Px(hudBoxMinNameW))
	}
	name = Ellipsize(name, room, h.measure)
	if w := h.measureTiny(dist); w > float64(targetCardDist(card).Dx()) {
		dist = Ellipsize(dist, float64(targetCardDist(card).Dx()), h.measureTiny)
	}
	return name, dist
}

// drawTargetCard paints one card image: background, border, the
// name/distance line, the lock strip and the layer bars. Every rect comes
// from the pure geometry above — this is a thin renderer over that math.
func drawTargetCard(dst *ebiten.Image, c targetCard, env Env) {
	p := env.Pal
	card := dst.Bounds()
	// Hairlines scale with the card image: stroke s px centred on the
	// s-pixel inset edge.
	s := float32(Scale())
	dst.Fill(solid(p.BG))
	vector.StrokeRect(dst, s/2, s/2, float32(card.Dx())-s, float32(card.Dy())-s,
		s, nrgba(p.Border, 200), true)

	// Kind glyph leads the name line, then the name — the two rects come
	// from the same pure geometry, so they can never overlap.
	if c.kind != "" {
		ic := targetCardIcon(card)
		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Translate(float64(ic.Min.X), float64(ic.Min.Y))
		dst.DrawImage(icons.Get(c.kind, Px(hudBoxIcon), c.tint), opts)
	}
	line := targetCardName(card)
	drawCardText(dst, c.name, env.FaceBold(StepSmall), line.Min.X, line.Min.Y, solid(p.Text))
	dl := targetCardDist(card)
	drawCardText(dst, c.dist, env.Face(StepTiny), dl.Min.X, dl.Min.Y, solid(p.Dim))

	// Lock-progress strip under the line.
	lk := targetCardLock(card)
	fillRect(dst, lk, nrgba(p.Accent, 60))
	if f := barFill(lk, c.lock); f.Dx() > 0 {
		fillRect(dst, f, solid(p.Accent))
	}

	// Layer bars: three for ships, a single health bar otherwise.
	bars := layerBars(targetCardBars(card))
	n := 1
	if c.ships {
		n = 3
	}
	cols := [3]color.RGBA{p.Shield, p.Warn, p.Struct}
	for i := 0; i < n; i++ {
		bar := bars[i]
		fillRect(dst, bar, solid(p.Cell))
		col := cols[i]
		if !c.ships {
			col = p.Struct // non-ships report one structure-like fraction
		}
		if f := barFill(bar, c.layers[i]); f.Dx() > 0 {
			fillRect(dst, f, solid(col))
		}
		vector.StrokeRect(dst,
			float32(bar.Min.X)+s/2, float32(bar.Min.Y)+s/2,
			float32(bar.Dx())-s, float32(bar.Dy())-s,
			s, nrgba(p.Border, 160), false)
	}
}

// fillRect fills r with a solid color.
func fillRect(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	vector.DrawFilledRect(dst,
		float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()),
		col, false)
}

// drawCardText draws s with its rendering region's top-left at (x, y) —
// text/v2's default alignment for left-to-right faces.
func drawCardText(dst *ebiten.Image, s string, face *text.Face, x, y int, col color.Color) {
	if s == "" || face == nil {
		return
	}
	opts := &text.DrawOptions{}
	opts.GeoM.Translate(float64(x), float64(y))
	opts.ColorScale.ScaleWithColor(col)
	text.Draw(dst, s, *face, opts)
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
	h.walletLbl.Label = FormatCredits(s.Wallet())

	// 4 — locked-target cards (name, distance, lock strip, layer bars).
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
		card := targetCard{lock: hudFrac01(l.Progress), name: "…", tint: h.env.Pal.Text}
		if hasInfo {
			card.dist = formatDistance(info.Distance)
			// The kind glyph leads the name line; hostiles wear
			// ColDanger, the player's own ship ColInfo.
			card.kind = KindIcon(info.Kind)
			switch {
			case info.Hostile:
				card.tint = h.env.Pal.Danger
			case info.Entity == player:
				card.tint = h.env.Pal.Info
			}
			if sh, ok := s.Ships.Get(l.Target); ok {
				// Ships report all three layers; other targets fall
				// back to the read model's single health fraction.
				card.ships = true
				card.layers = [3]float64{
					hudFrac(sh.Shield, sh.MaxShield),
					hudFrac(sh.Armor, sh.MaxArmor),
					hudFrac(sh.Structure, sh.MaxStructure),
				}
				card.name = info.Name
			} else {
				card.layers[0] = hudFrac01(info.Health)
				card.name = info.Name
			}
		}
		card.name, card.dist = h.boxLabels(card.name, card.dist)
		h.setCard(box, card)
	}
}

// hudTrackDim scales a layer color down for the depleted remainder of its
// ring. Dimming happens in RGB, never in alpha: render.Arc keeps RGB at
// full strength, so an alpha-dimmed track composites to nearly the same
// pixel as the fill against the near-opaque panel behind the ring image
// (that exact bug is pinned by TestHudCaptureFrame).
const hudTrackDim = 0.25

// ringTrackColor returns the dim track color for a layer's fill color.
func ringTrackColor(col color.RGBA) color.RGBA {
	dim := func(v uint8) uint8 { return uint8(float64(v) * hudTrackDim) }
	return color.RGBA{R: dim(col.R), G: dim(col.G), B: dim(col.B), A: 255}
}

// drawRing repaints the ring image: a dim track plus a fraction fill per
// layer (sweep from ringSweep), quarter/half gauge ticks across every
// ring, and the hull silhouette in the middle.
func (h *Hud) drawRing(shield, armor, structure float64) {
	h.redraws++
	h.ring.Clear()
	// The ring image was built Px(hudRingSize) wide: every radius and
	// stroke below scales with it (identity at scale 1).
	s := Scale()
	c := float64(Px(hudRingSize)) / 2
	const twoPi = 2 * math.Pi
	start := -math.Pi / 2
	p := h.env.Pal
	layers := [...]struct {
		rad, frac float64
		col       color.RGBA
	}{
		{hudRingR * s, shield, p.Shield},
		{(hudRingR - hudRingStep) * s, armor, p.Warn},
		{(hudRingR - 2*hudRingStep) * s, structure, p.Struct},
	}
	for _, l := range layers {
		render.Arc(h.ring, float32(c), float32(c), float32(l.rad),
			float32(start), float32(start+twoPi), float32(hudArcW*s), ringTrackColor(l.col))
		if l.frac > 0 {
			render.Arc(h.ring, float32(c), float32(c), float32(l.rad),
				float32(start), float32(start+ringSweep(l.frac)), float32(hudArcW*s), l.col)
		}
	}
	// Gauge ticks at the quarter/half/three-quarter marks, crossing each
	// ring like EVE's HUD, drawn over track and fill alike.
	tick := p.Dim
	for _, t := range ringTicks() {
		a := start + t
		dx, dy := math.Cos(a), math.Sin(a)
		for _, l := range layers {
			r0, r1 := l.rad-hudTickLen*s/2, l.rad+hudTickLen*s/2
			vector.StrokeLine(h.ring,
				float32(c+dx*r0), float32(c+dy*r0),
				float32(c+dx*r1), float32(c+dy*r1),
				float32(1.5*s), tick, true)
		}
	}
	// Hull silhouette (the legacy four-point ship glyph, scaled to the rings).
	R := hudRingR * s
	pts := [][2]float64{
		{0, -R * 0.30},
		{R * 0.19, R * 0.24},
		{0, R * 0.08},
		{-R * 0.19, R * 0.24},
	}
	for i := range pts {
		a, b := pts[i], pts[(i+1)%len(pts)]
		vector.StrokeLine(h.ring,
			float32(c+a[0]), float32(c+a[1]),
			float32(c+b[0]), float32(c+b[1]),
			float32(1.5*s), p.Struct, true)
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
	return FormatInt(int64(v + 0.5))
}
