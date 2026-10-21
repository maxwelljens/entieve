package screens

import (
	"image"
	"image/color"
	"math"
	"strconv"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/hulls"
	"github.com/entieve/entieve/uikit/icons"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// HUD geometry. The ship-status panel and the module rack form one
// bottom-center pair (status left, rack right); the time/wallet strip sits
// above the status. Widths are the 1280×720 / 1024×700 no-overlap contract:
// the pair is 556 design px, which keeps its left edge right of the log's
// 361 px pocket at 1280 wide.
const (
	hudPortrait  = 120  // hull portrait box (design px)
	hudBarH      = 18   // one health/capacitor bar height (px)
	hudBarMin    = 128  // bar width: fits the "1,800.0/1,800.0 GJ" value (px)
	hudHpGap     = 2    // gap between the three health bars (px)
	hudColGap    = 4    // gap between readout groups in the status column (px)
	hudStatusGap = 6    // portrait → readout column gap (px)
	hudPairGap   = 8    // status column → rack gap (px)
	hudStripH    = 26   // time/wallet strip height (px)
	hudHintH     = 18   // reserved recharge-hint line (no layout jump) (px)
	hudWedgeW    = 26   // velocity wedge width (px)
	hudWedgeH    = 72   // velocity wedge height (px)
	hudCapMax    = 1000 // capacitor bar resolution (0.1 % steps)
	hudBoxPool   = 7    // target-box slots (hulls top out at 4 locks)
	hudBoxW      = 132  // one target box width (px)
	hudBoxH      = 83   // one target box height (px, fixed)
	hudBoxGap    = 5    // gap between target boxes
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
	return card.Inset(theme.Px(hudBoxPad))
}

// targetCardText returns the name line at the top of the body.
func targetCardText(card image.Rectangle) image.Rectangle {
	b := targetCardBody(card)
	return image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+theme.Px(hudBoxTextH))
}

// hudBoxIcon is the kind glyph box on a card's name line: the inline size
// of the type grid, vertically centred in the line.
const hudBoxIcon = theme.IconInline

// targetCardIcon returns the kind glyph box at the start of the name line.
func targetCardIcon(card image.Rectangle) image.Rectangle {
	b := targetCardBody(card)
	y := b.Min.Y + theme.Px((hudBoxTextH-hudBoxIcon)/2)
	return image.Rect(b.Min.X, y, b.Min.X+theme.Px(hudBoxIcon), y+theme.Px(hudBoxIcon))
}

// targetCardName returns the name text region: everything right of the
// icon box, so the glyph and the label can never overlap.
func targetCardName(card image.Rectangle) image.Rectangle {
	ic := targetCardIcon(card)
	line := targetCardText(card)
	return image.Rect(ic.Max.X+theme.Px(theme.IconGap), line.Min.Y, line.Max.X, line.Max.Y)
}

// targetCardDist returns the distance line under the name.
func targetCardDist(card image.Rectangle) image.Rectangle {
	b := targetCardBody(card)
	y := b.Min.Y + theme.Px(hudBoxTextH) + theme.Px(hudBoxDistGap)
	return image.Rect(b.Min.X, y, b.Max.X, y+theme.Px(hudBoxDistH))
}

// targetCardLock returns the lock-progress strip under the distance line.
func targetCardLock(card image.Rectangle) image.Rectangle {
	b := targetCardBody(card)
	y := b.Min.Y + theme.Px(hudBoxTextH) + theme.Px(hudBoxDistGap) + theme.Px(hudBoxDistH) + theme.Px(hudBoxLockGap)
	return image.Rect(b.Min.X, y, b.Max.X, y+theme.Px(hudBoxLockH))
}

// targetCardBars returns the layer-bar stack: from under the lock strip
// down to the bottom of the body.
func targetCardBars(card image.Rectangle) image.Rectangle {
	b := targetCardBody(card)
	y := b.Min.Y + theme.Px(hudBoxTextH) + theme.Px(hudBoxDistGap) + theme.Px(hudBoxDistH) + theme.Px(hudBoxLockGap) +
		theme.Px(hudBoxLockH) + theme.Px(hudBoxBarsGap)
	return image.Rect(b.Min.X, y, b.Max.X, b.Max.Y)
}

// layerBars tiles r with exactly three stacked bars separated by
// hudBoxBarGap. Heights divide evenly; any division remainder lands on
// the last bar, so the three rects cover r with no gaps and no overlaps.
func layerBars(r image.Rectangle) [3]image.Rectangle {
	var out [3]image.Rectangle
	gap := theme.Px(hudBoxBarGap)
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

// Hud is the always-on ship HUD: the bottom-center pair (ship status beside
// the module rack), the time/wallet strip above the status, and the
// top-center target-box row. One root container holds them all, so the
// integrator adds a single stretched child to the interface root; the game
// slots the rack into the pair through MountRack.
type Hud struct {
	th theme.Theme
	cv widgets.ContentView

	container *widget.Container
	targetRow *widget.Container
	pair      *widget.Container // bottom-center: status column + rack slot
	strip     *widget.Container
	status    *widget.Container
	boxes     [hudBoxPool]*hudBox

	portrait      *ebiten.Image
	portraitGfx   *widget.Graphic
	portraitHull  content.ShipID
	portraitDraws int // portrait repaint count (hull changes only)

	health     *ebiten.Image
	healthGfx  *widget.Graphic
	hp         [3]hudHealth // staged: shield / armor / structure
	hpDrawn    [3]hudHealth // last painted
	hpPainted  bool
	hpDraws    int // health repaint count (state changes only)
	capBar     *widget.ProgressBar
	capValue   *widget.Text
	recharging *widget.Text
	wedge      *ebiten.Image
	wedgeFrac  float64
	wedgeDraws int // wedge repaint count (fraction changes only)
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
func NewHud(env interface {
	theme.Theme
	widgets.ContentView
}) *Hud {
	p := env.Pal()
	h := &Hud{th: env, cv: env, timeScale: 1, wedgeFrac: -1}
	faceSmall := env.Face(theme.StepSmall)
	h.measure = func(s string) float64 {
		w, _ := text.Measure(s, *faceSmall, 0)
		return w
	}
	faceTiny := env.Face(theme.StepTiny)
	h.measureTiny = func(s string) float64 {
		w, _ := text.Measure(s, *faceTiny, 0)
		return w
	}

	// Health: three self-drawn bars with "current / max" printed inside
	// (the capacitor-bar styling, drawn together so each keeps its own
	// palette color).
	h.health = ebiten.NewImage(theme.Px(hudBarMin), theme.Px(hudHealthH))
	h.healthGfx = widget.NewGraphic(
		widget.GraphicOpts.Image(h.health),
		widget.GraphicOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)

	// Capacitor: the same bar idiom, value inside; the "capacitor" caption
	// is gone (the green bar is the capacitor). The recharge hint keeps a
	// reserved line so the pair never jumps when it appears.
	h.capBar = widget.NewProgressBar(
		widget.ProgressBarOpts.Values(0, hudCapMax, 0),
		widget.ProgressBarOpts.FillImage(&widget.ProgressBarImage{Idle: theme.NineSlice(theme.Solid(p.Cap))}),
		widget.ProgressBarOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(hudBarMin), theme.Px(hudBarH))),
	)
	h.capValue = widget.NewText(widget.TextOpts.Text("", env.Face(theme.StepTiny), theme.Solid(p.BGHard)),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter))
	capStack := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewStackedLayout()),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	capStack.AddChild(h.capBar, h.capValue)
	h.recharging = widget.NewText(widget.TextOpts.Text("recharging", env.Face(theme.StepTiny), theme.Solid(p.Warn)))
	rechargeRow := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(0, theme.Px(hudHintH))),
	)
	rechargeRow.AddChild(h.recharging)

	// Velocity: a vertical wedge widening bottom→top, filled bottom-up with
	// current / base max speed (a stable top; EffectiveMaxSpeed would move
	// the ceiling with afterburner/web) and the m/s figure under it.
	h.wedge = ebiten.NewImage(theme.Px(hudWedgeW), theme.Px(hudWedgeH))
	wedgeGfx := widget.NewGraphic(
		widget.GraphicOpts.Image(h.wedge),
		widget.GraphicOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
			Position: widget.RowLayoutPositionCenter,
		})),
	)
	h.speedLbl = widget.NewText(
		widget.TextOpts.Text("", env.FaceBold(theme.StepBase), theme.Solid(p.Text)),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionStart),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	velocity := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(2)),
		)),
	)
	velocity.AddChild(wedgeGfx, h.speedLbl)

	readouts := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(hudColGap)),
		)),
	)
	readouts.AddChild(h.healthGfx, capStack, rechargeRow, velocity)

	// Hull portrait: the left slot of the status panel, drawn from the
	// hulls package into a self-drawn image (repainted on hull change).
	h.portrait = ebiten.NewImage(theme.Px(hudPortrait), theme.Px(hudPortrait))
	h.portraitGfx = widget.NewGraphic(
		widget.GraphicOpts.Image(h.portrait),
		widget.GraphicOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(theme.Px(hudPortrait), theme.Px(hudPortrait)),
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Position: widget.RowLayoutPositionCenter}),
		),
	)

	h.status = widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.Bordered(theme.Solid(p.BG), theme.NRGBA(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(hudStatusGap)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	h.status.AddChild(h.portraitGfx, readouts)

	// Time/wallet strip above the status: clock + multiplier left, the
	// wallet right. No sim clock (the multiplier is the settled readout).
	h.timeLbl = widget.NewText(
		widget.TextOpts.Text("", env.FaceBold(theme.StepSmall), theme.Solid(p.Accent)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionStart,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
			Padding:            &widget.Insets{Left: theme.Px(theme.ContentsPad + theme.IconInline + theme.IconGap)},
		})),
	)
	h.walletLbl = widget.NewText(
		// Tabular contents face: the wallet is a dynamic number and mono
		// digits keep its right edge steady while it ticks.
		widget.TextOpts.Text("", env.FaceMono(theme.StepSmall), theme.Solid(p.Good)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionEnd,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
			Padding:            &widget.Insets{Right: theme.Px(theme.ContentsPad)},
		})),
	)
	clock := theme.NewIcon("hud_time", theme.IconInline, p.Accent)
	clock.GetWidget().LayoutData = widget.AnchorLayoutData{
		HorizontalPosition: widget.AnchorLayoutPositionStart,
		VerticalPosition:   widget.AnchorLayoutPositionCenter,
		Padding:            &widget.Insets{Left: theme.Px(theme.ContentsPad)},
	}
	h.strip = widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.Bordered(theme.Solid(p.BG), theme.NRGBA(p.Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(0, theme.Px(hudStripH)),
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)
	h.strip.AddChild(clock, h.timeLbl, h.walletLbl)

	// The left column: strip over status, bottom-aligned in the pair (the
	// rack is taller, so the pair's own bottom is the shared baseline).
	left := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(hudColGap)),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
			Position: widget.RowLayoutPositionEnd,
		})),
	)
	left.AddChild(h.strip, h.status)

	h.pair = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(hudPairGap)),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionCenter,
			VerticalPosition:   widget.AnchorLayoutPositionEnd,
			Padding:            &widget.Insets{Left: theme.Px(10), Right: theme.Px(10), Bottom: theme.Px(10)},
		})),
	)
	h.pair.AddChild(left)

	// Target boxes: a top-center row, hidden while nothing is locked.
	h.targetRow = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(hudBoxGap)),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionCenter,
			VerticalPosition:   widget.AnchorLayoutPositionStart,
			Padding:            &widget.Insets{Top: theme.Px(10)},
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
	h.container.AddChild(h.targetRow, h.pair)
	return h
}

// MountRack slots the module rack into the bottom-center pair, immediately
// right of the status column. The rack keeps its own container (Sync, hover,
// hit rects); only its layout slot changes.
func (h *Hud) MountRack(c *widget.Container) {
	c.GetWidget().LayoutData = widget.RowLayoutData{Position: widget.RowLayoutPositionEnd}
	h.pair.AddChild(c)
}

// hudHealth is one staged health bar: its fraction and its "current / max"
// label. Deliberately comparable so paintHealth skips unchanged frames.
type hudHealth struct {
	frac  float64
	label string
}

// hudHealthH is the three-bar block's height: three bars and the gaps.
const hudHealthH = 3*hudBarH + 2*hudHpGap

// setHealth stages the three layer bars and repaints on a state change.
func (h *Hud) setHealth(shield, armor, structure hudHealth) {
	h.hp = [3]hudHealth{shield, armor, structure}
	if h.hpPainted && h.hpDrawn == h.hp {
		return
	}
	h.hpPainted = true
	h.hpDrawn = h.hp
	h.hpDraws++
	h.paintHealth()
}

// paintHealth paints the three bars: palette track, the layer color up to the
// fraction, a hairline border and the centered "current / max" value — the
// capacitor-bar look, self-drawn so each bar keeps its own color (ebitenui's
// progress bars share one theme fill struct).
func (h *Hud) paintHealth() {
	h.health.Clear()
	p := h.th.Pal()
	cols := [3]color.RGBA{p.Shield, p.Warn, p.Struct}
	s := float32(theme.Scale())
	face := h.th.Face(theme.StepTiny)
	for i, st := range h.hp {
		y := theme.Px(i * (hudBarH + hudHpGap))
		bar := image.Rect(0, y, theme.Px(hudBarMin), y+theme.Px(hudBarH))
		theme.FillRect(h.health, bar, theme.Solid(p.Cell))
		if f := barFill(bar, st.frac); f.Dx() > 0 {
			theme.FillRect(h.health, f, theme.Solid(cols[i]))
		}
		vector.StrokeRect(h.health,
			float32(bar.Min.X)+s/2, float32(bar.Min.Y)+s/2,
			float32(bar.Dx())-s, float32(bar.Dy())-s,
			s, theme.NRGBA(p.Border, 160), false)
		x := bar.Min.X + (bar.Dx()-int(math.Round(h.measureTiny(st.label))))/2
		theme.DrawCardText(h.health, st.label, face, x, bar.Min.Y, theme.Solid(p.BGHard))
	}
}

// newBox builds one target card from the fixed widget pool: a container
// holding a single Graphic that displays the card image.
func (h *Hud) newBox(env theme.Theme) *hudBox {
	b := &hudBox{img: ebiten.NewImage(theme.Px(hudBoxW), theme.Px(hudBoxH))}
	g := widget.NewGraphic(
		widget.GraphicOpts.Image(b.img),
		widget.GraphicOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	b.root = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(hudBoxW), theme.Px(hudBoxH))),
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
	drawTargetCard(b.img, b.card, h.th)
}

// boxLabels fits a card's text: the name to the room right of the kind
// glyph, the distance to its own line.
func (h *Hud) boxLabels(name, dist string) (string, string) {
	card := image.Rect(0, 0, theme.Px(hudBoxW), theme.Px(hudBoxH))
	room := float64(targetCardName(card).Dx())
	if room < float64(theme.Px(hudBoxMinNameW)) {
		room = float64(theme.Px(hudBoxMinNameW))
	}
	name = widgets.Ellipsize(name, room, h.measure)
	if w := h.measureTiny(dist); w > float64(targetCardDist(card).Dx()) {
		dist = widgets.Ellipsize(dist, float64(targetCardDist(card).Dx()), h.measureTiny)
	}
	return name, dist
}

// drawTargetCard paints one card image: background, border, the
// name/distance line, the lock strip and the layer bars. Every rect comes
// from the pure geometry above — this is a thin renderer over that math.
func drawTargetCard(dst *ebiten.Image, c targetCard, env theme.Theme) {
	p := env.Pal()
	card := dst.Bounds()
	// Hairlines scale with the card image: stroke s px centred on the
	// s-pixel inset edge.
	s := float32(theme.Scale())
	dst.Fill(theme.Solid(p.BG))
	vector.StrokeRect(dst, s/2, s/2, float32(card.Dx())-s, float32(card.Dy())-s,
		s, theme.NRGBA(p.Border, 200), true)

	// Kind glyph leads the name line, then the name — the two rects come
	// from the same pure geometry, so they can never overlap.
	if c.kind != "" {
		ic := targetCardIcon(card)
		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Translate(float64(ic.Min.X), float64(ic.Min.Y))
		dst.DrawImage(icons.Get(c.kind, theme.Px(hudBoxIcon), c.tint), opts)
	}
	line := targetCardName(card)
	theme.DrawCardText(dst, c.name, env.FaceBold(theme.StepSmall), line.Min.X, line.Min.Y, theme.Solid(p.Text))
	dl := targetCardDist(card)
	theme.DrawCardText(dst, c.dist, env.Face(theme.StepTiny), dl.Min.X, dl.Min.Y, theme.Solid(p.Dim))

	// Lock-progress strip under the line.
	lk := targetCardLock(card)
	theme.FillRect(dst, lk, theme.NRGBA(p.Accent, 60))
	if f := barFill(lk, c.lock); f.Dx() > 0 {
		theme.FillRect(dst, f, theme.Solid(p.Accent))
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
		theme.FillRect(dst, bar, theme.Solid(p.Cell))
		col := cols[i]
		if !c.ships {
			col = p.Struct // non-ships report one structure-like fraction
		}
		if f := barFill(bar, c.layers[i]); f.Dx() > 0 {
			theme.FillRect(dst, f, theme.Solid(col))
		}
		vector.StrokeRect(dst,
			float32(bar.Min.X)+s/2, float32(bar.Min.Y)+s/2,
			float32(bar.Dx())-s, float32(bar.Dy())-s,
			s, theme.NRGBA(p.Border, 160), false)
	}
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

// Sync refreshes every readout — health, capacitor, speed, time, wallet,
// portrait and target boxes — from the sim. Call it once per frame before
// Update.
func (h *Hud) Sync() {
	s := h.cv
	if s == nil {
		return
	}
	player := s.Player()

	// 1 — the three health bars and the hull portrait (the portrait
	// repaints only when the hull changes).
	if sh, ok := s.ShipState(player); ok {
		h.setHealth(
			hudHealth{hudFrac(sh.Shield, sh.MaxShield), hudHP(sh.Shield) + " / " + hudHP(sh.MaxShield)},
			hudHealth{hudFrac(sh.Armor, sh.MaxArmor), hudHP(sh.Armor) + " / " + hudHP(sh.MaxArmor)},
			hudHealth{hudFrac(sh.Structure, sh.MaxStructure), hudHP(sh.Structure) + " / " + hudHP(sh.MaxStructure)},
		)
		if sh.Hull != h.portraitHull {
			h.paintPortrait(sh.Hull)
		}
	} else {
		h.setHealth(hudHealth{}, hudHealth{}, hudHealth{})
	}

	// 2 — capacitor bar + value label + recharge state.
	frac, capLabel := 1.0, ""
	if cap, ok := s.Capacitor(player); ok && cap.Capacity > 0 {
		frac = hudFrac(cap.Current, cap.Capacity)
		capLabel = theme.GroupFloat(cap.Current, 1) + "/" + theme.GroupFloat(cap.Capacity, 1) + " GJ"
	}
	h.capBar.SetCurrent(int(frac * hudCapMax))
	h.capValue.Label = capLabel
	theme.ShowWidget(h.recharging.GetWidget(), frac < 0.999)

	// 3 — velocity wedge, speed figure, time acceleration and wallet.
	speed, maxSpeed := 0.0, 0.0
	if m, ok := s.Motion(player); ok {
		speed = m.Vel.Len()
		maxSpeed = m.MaxSpeed
	}
	wedge := 0.0
	if maxSpeed > 0 {
		wedge = hudFrac01(speed / maxSpeed)
	}
	if wedge != h.wedgeFrac {
		h.paintWedge(wedge)
	}
	h.speedLbl.Label = theme.FormatSpeed(speed)
	h.timeLbl.Label = "×" + strconv.Itoa(h.timeScale) + " time"
	h.walletLbl.Label = theme.FormatCredits(s.Wallet())

	// 4 — locked-target cards (name, distance, lock strip, layer bars).
	var locks []sim.Lock
	if tg, ok := s.Targeting(player); ok {
		locks = tg.Locks
	}
	if len(locks) == 0 {
		theme.ShowWidget(h.targetRow.GetWidget(), false)
		return
	}
	theme.ShowWidget(h.targetRow.GetWidget(), true)
	for i, box := range h.boxes {
		if i >= len(locks) {
			theme.ShowWidget(box.root.GetWidget(), false)
			continue
		}
		theme.ShowWidget(box.root.GetWidget(), true)
		l := locks[i]
		info, hasInfo := s.Info(l.Target)
		card := targetCard{lock: hudFrac01(l.Progress), name: "…", tint: h.th.Pal().Text}
		if hasInfo {
			card.dist = theme.FormatDistance(info.Distance)
			// The kind glyph leads the name line; hostiles wear
			// ColDanger, the player's own ship ColInfo.
			card.kind = theme.KindIcon(info.Kind)
			switch {
			case info.Hostile:
				card.tint = h.th.Pal().Danger
			case info.Entity == player:
				card.tint = h.th.Pal().Info
			}
			if sh, ok := s.ShipState(l.Target); ok {
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

// paintPortrait repaints the hull portrait: the hull's raster scaled into
// the design-square box (the image is built at Px(hudPortrait), so a scale
// change rebuilds the kit and the portrait with it).
func (h *Hud) paintPortrait(hull content.ShipID) {
	h.portraitHull = hull
	h.portraitDraws++
	h.portrait.Clear()
	src := hulls.Get(hull, theme.Px(hudPortrait))
	if src == nil {
		return
	}
	b := src.Bounds()
	box := float64(theme.Px(hudPortrait))
	scale := math.Min(box/float64(b.Dx()), box/float64(b.Dy()))
	var opts ebiten.DrawImageOptions
	opts.GeoM.Scale(scale, scale)
	opts.GeoM.Translate((box-float64(b.Dx())*scale)/2, (box-float64(b.Dy())*scale)/2)
	h.portrait.DrawImage(ebiten.NewImageFromImage(src), &opts)
}

// hudColorScale converts a straight-alpha palette color for vector fills.
func hudColorScale(c color.RGBA) ebiten.ColorScale {
	var cs ebiten.ColorScale
	cs.Scale(float32(c.R)/255, float32(c.G)/255, float32(c.B)/255, float32(c.A)/255)
	return cs
}

// paintWedge repaints the velocity wedge: a dim full triangle with a
// bottom-up fill up to frac (current / base max speed).
func (h *Hud) paintWedge(frac float64) {
	h.wedgeFrac = frac
	h.wedgeDraws++
	h.wedge.Clear()
	p := h.th.Pal()
	w, hh := float32(theme.Px(hudWedgeW)), float32(theme.Px(hudWedgeH))
	cx := w / 2
	// Track: apex at the bottom centre, base along the top edge.
	var track vector.Path
	track.MoveTo(cx, hh)
	track.LineTo(0, 0)
	track.LineTo(w, 0)
	track.Close()
	vector.FillPath(h.wedge, &track, nil, &vector.DrawPathOptions{AntiAlias: true, ColorScale: hudColorScale(p.Cell)})
	// Fill: the trapezoid between the frac line and the apex.
	if frac > 0 {
		y := hh - float32(frac)*hh
		half := (hh - y) / hh * cx
		var fill vector.Path
		fill.MoveTo(cx, hh)
		fill.LineTo(cx-half, y)
		fill.LineTo(cx+half, y)
		fill.Close()
		vector.FillPath(h.wedge, &fill, nil, &vector.DrawPathOptions{AntiAlias: true, ColorScale: hudColorScale(p.Accent)})
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
	return theme.FormatInt(int64(v + 0.5))
}
