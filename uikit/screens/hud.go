package screens

import (
	"image"
	"image/color"
	"math"
	"strconv"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
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
// the pair is 666 design px on a four-slot hull (600 on a three-slot one),
// which no longer fits a viewport-centred seat, so the pair centres in the
// band the log's 264 px pocket leaves free (see the pair's anchor).
const (
	hudPortraitW = 240  // hull portrait box width (design px): the owner's twofold frame, 4:1
	hudPortraitH = 60   // a quarter of the width: the owner's ships are side views (contained, padded)
	hudBarH      = 18   // one health/capacitor bar height (px)
	hudBarMin    = 128  // capacitor bar width: fits the "1,800.0/1,800.0 GJ" value (px)
	hudHpGap     = 2    // gap between the three health bars (px)
	hudColGap    = 4    // gap between readout groups in the status column (px)
	hudStatusGap = 6    // portrait → readout column gap (px)
	hudPairGap   = 8    // status column → rack gap (px)
	hudStripH    = 26   // time/wallet strip height (px)
	hudHintH     = 19   // reserved recharge-hint line: one StepTiny line box (no layout jump) (px)
	hudWedgeW    = 26   // velocity wedge width (px)
	hudCapMax    = 1000 // capacitor bar resolution (0.1 % steps)
	hudBoxPool   = 7    // target-box slots (hulls top out at 4 locks)
	hudBoxW      = 132  // one target box width (px)
	hudBoxH      = 83   // one target box height (px, fixed)
	hudBoxGap    = 5    // gap between target boxes
	hudBoxMods   = 3    // module mini-cells a card shows on its distance line
	// A card's module mini-cell is the rack's cell at a third of the scale:
	// the module's glyph with its depleting cycle ring. It fits the 19 px
	// distance line, so the cap and the distance text's clearance rule hold.
	hudBoxModCell  = 18 // mini-cell size: the glyph plus its ring (px)
	hudBoxModGlyph = 14 // module glyph inside a mini-cell (px)
	hudBoxModRingW = 1  // cycle-ring stroke (px)
	// The EWAR effect boxes above the ship status: a rack cell scaled down —
	// a tile with the effect glyph and a depleting cycle ring, its
	// remaining-time line under it (the rack's tile + name layout).
	hudEffW     = 44 // box width: the tile plus air for the countdown line (px)
	hudEffTile  = 34 // tile size (px)
	hudEffNameH = 19 // countdown line height (px, fits StepTiny)
	hudEffRingW = 2  // depletion ring stroke (px)

	// The combat feed above the ship status: hudFeedLines brief lines, in the
	// status column's own width. It is an overlay (TOP chrome), so it never
	// grows the bottom column the virgin-placement pins measure.
	hudFeedLines  = 4
	hudFeedW      = 264 // line width: the log's / status column's own (px)
	hudFeedLineH  = 19  // one line (fits StepTiny, like the effect box's name)
	hudFeedFrames = 300 // frames a line stays up: ~5 s of watching, whatever the time scale
	hudFeedGap    = 4   // gap between the strip's top edge and the feed (px)
)

// The velocity wedge shares the health block's band: it is exactly as tall
// as the three bars, and the bars fill the column left of it, so bars + gap
// + wedge add up to the capacitor bar's full width.
//
// shortcut: the 98 px bars fit the content's largest "1,200 / 1,200" value;
// a hull past ~1,300 HP overruns them — widen the column (and re-check the
// 1280 log clearance) before authoring one.
const (
	hudWedgeH  = hudHealthH
	hudHealthW = hudBarMin - hudColGap - hudWedgeW
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

	// A card whose target is gone blinks before it disappears (destroyed,
	// warped off, lock dropped): frame-counted, so the cadence is the same at
	// every time acceleration. 90 frames alternating every 15 is 1.5 s of
	// two on/off cycles a second.
	hudGhostFrames = 90
	hudBlinkFrames = 15
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

// targetCardMod returns the box of the i-th module mini-cell counted from
// the distance line's right edge (i = 0 is the outermost), so the cells hug
// the card's edge however many there are.
func targetCardMod(card image.Rectangle, i int) image.Rectangle {
	dl := targetCardDist(card)
	size := theme.Px(hudBoxModCell)
	y := dl.Min.Y + (dl.Dy()-size)/2
	x2 := dl.Max.X - i*(size+theme.Px(theme.IconGap))
	return image.Rect(x2-size, y, x2, y+size)
}

// cardModsWidth is the room n module mini-cells take on the distance line (0
// for none): what the distance text is ellipsized clear of.
func cardModsWidth(n int) int {
	if n <= 0 {
		return 0
	}
	return n*theme.Px(hudBoxModCell) + (n-1)*theme.Px(theme.IconGap)
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
// top-center target-box row. The HUD container holds the pair (added as a
// stretched root child); the cards mount separately as TOP chrome (Cards),
// because the doubled Overview's body covers their band. The game slots the
// rack into the pair through MountRack.
type Hud struct {
	th theme.Theme
	cv widgets.ContentView

	container *widget.Container
	targetRow *widget.Container
	cards     *hudChrome        // targetRow as TOP chrome (Cards)
	feed      *hudFeed          // the combat feed as TOP chrome (Feed)
	pair      *widget.Container // bottom-center: status column + rack slot
	strip     *widget.Container
	effects   *widget.Container // EWAR icons above the status (hidden when none)
	eff       [2]*hudEffect     // stasis web, warp scram
	status    *widget.Container
	boxes     [hudBoxPool]*hudBox

	// selectFn is where a target-card click reports the card's entity: the
	// game wires Kit.Select, the same door a world click uses. pick reads
	// the selection back, so the selected target's card draws the border.
	selectFn func(ecs.Entity)
	pick     func() ecs.Entity

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
	wedgeGfx   *widget.Graphic
	wedgeFrac  float64
	wedgeDraws int // wedge repaint count (fraction changes only)
	speedLbl   *widget.Text
	timeLbl    *widget.Text
	walletLbl  *widget.Text
	walletIcon *widget.Graphic
	walletFace *text.Face

	measure     func(string) float64
	measureTiny func(string) float64
	timeScale   int
}

// hudFeed is the combat feed: brief lines scrolling up above the ship status,
// the important span of each in red (what hit or scrambled you) or blue (what
// you dealt). It is TOP chrome, so it paints over whatever window sits there
// and never joins the HUD's bottom column — the pair's top edge is a pinned
// clearance. With no live line the rows hide, the column collapses and claims
// nothing.
type hudFeed struct {
	hudChrome
	rows   *widget.Container // the panel: hidden when no line is live
	lines  []hudFeedLine     // staged, oldest first
	rowsAt [hudFeedLines]hudFeedRow
}

// hudFeedLine is one staged feed line: plain text around the important span
// (loss red / gain blue) and the frames it has left on screen.
type hudFeedLine struct {
	pre, key, post string
	loss           bool
	left           int
}

// hudFeedRow is one line's three text runs: the log colours whole lines, so
// the key span needs its own widget to carry the palette's colour.
type hudFeedRow struct {
	box            *widget.Container // the line's row (hidden while unused)
	pre, key, post *widget.Text
	staged         hudFeedLine
	painted        bool
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
	draws   int  // image repaint count (tests watch it)
	live    bool // this frame's card is a real lock: a click may select it
	ghost   int  // blink frames left after the lock went away (0 = none)
	ghostOn bool
}

// targetCard is one card's rendered state. It is deliberately comparable:
// Sync skips the repaint when nothing moved.
type targetCard struct {
	entity   ecs.Entity // the locked entity a click selects
	selected bool       // the player's current selection: draw the border
	name     string
	dist     string
	kind     string // semantic icon name for the target's kind ("" = none)
	tint     color.RGBA
	lock     float64               // 0..1 lock progress
	layers   [3]float64            // shield / armor / hull fractions (ships); health in [0] otherwise
	ships    bool                  // three layers (true) vs a single health fraction
	mods     [hudBoxMods]hudBoxMod // my active modules cycling on this target
	nmods    int
}

// hudBoxMod is one module mini-cell on a card's distance line: the module's
// class glyph and the cycle ring the rack draws on its cell. ring is the
// remaining fraction (0..1) and -1 when no cycle is in flight; lit is the
// rack's rule (on for an active module or a loading bay, greyed while a
// cancelled cycle runs out).
type hudBoxMod struct {
	glyph string
	ring  float64
	lit   bool
}

// hudEffect is one live EWAR effect on the player: a rack-cell miniature —
// the effect's glyph on a tile with a depleting cycle ring, its remaining
// time on the line under it. One box per effect kind, hidden while off.
type hudEffect struct {
	glyph string // manifest key
	tint  color.RGBA
	root  *widget.Container
	img   *ebiten.Image
	gfx   *widget.Graphic
	lbl   *widget.Text
	peak  float64 // longest remaining seen this spell: the ring's full circle
	drawn float64 // ring fraction at the last paint (-1 = never painted)
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
	// palette color). They share one band with the velocity wedge.
	h.health = ebiten.NewImage(theme.Px(hudHealthW), theme.Px(hudHealthH))
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
	capMark := widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout()))
	// The capacitor bar is shorter than a doubled glyph: cap it to the
	// health bars' inner height (the bar less the outline inset).
	capGlyph := theme.NewIcon("stat_capacitor", min(theme.StatIconSize, hudBarH-4), p.Cap)
	capGlyph.GetWidget().LayoutData = widget.AnchorLayoutData{
		HorizontalPosition: widget.AnchorLayoutPositionStart,
		VerticalPosition:   widget.AnchorLayoutPositionCenter,
		Padding:            &widget.Insets{Left: theme.Px(4)},
	}
	capMark.AddChild(capGlyph)
	capStack.AddChild(h.capBar, capMark, h.capValue)
	h.recharging = widget.NewText(widget.TextOpts.Text("recharging", env.Face(theme.StepTiny), theme.Solid(p.Warn)))
	rechargeRow := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
		)),
		// A column-wide floor keeps the reserved line in the column's
		// preferred height while the caption is hidden.
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(hudBarMin), theme.Px(hudHintH))),
	)
	rechargeRow.AddChild(h.recharging)

	// Velocity: a vertical wedge widening bottom→top, filled bottom-up with
	// current / base max speed (a stable top; EffectiveMaxSpeed would move
	// the ceiling with afterburner/web). It sits in the health bars' band,
	// with the m/s figure right-aligned under the wedge.
	h.wedge = ebiten.NewImage(theme.Px(hudWedgeW), theme.Px(hudWedgeH))
	h.wedgeGfx = widget.NewGraphic(
		widget.GraphicOpts.Image(h.wedge),
		widget.GraphicOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
			Position: widget.RowLayoutPositionCenter,
		})),
	)
	h.speedLbl = widget.NewText(
		widget.TextOpts.Text("", env.FaceBold(theme.StepBase), theme.Solid(p.Text)),
		widget.TextOpts.Position(widget.TextPositionEnd, widget.TextPositionStart),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	speedRow := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(hudColGap)),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
	)
	speedRow.AddChild(h.healthGfx, h.wedgeGfx)

	readouts := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(hudColGap)),
		)),
	)
	readouts.AddChild(speedRow, h.speedLbl, capStack, rechargeRow)

	// Hull portrait: the left slot of the status panel, the owner's 4:1 box
	// painted from the hulls art (repainted on hull change). A quarter of the
	// width tall: the pair's seat carries the growth (see the pair's anchor).
	h.portrait = ebiten.NewImage(theme.Px(hudPortraitW), theme.Px(hudPortraitH))
	h.portraitGfx = widget.NewGraphic(
		widget.GraphicOpts.Image(h.portrait),
		widget.GraphicOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(theme.Px(hudPortraitW), theme.Px(hudPortraitH)),
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Position: widget.RowLayoutPositionCenter}),
		),
	)

	h.status = widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.PanelBox(p, theme.Solid(p.BG), theme.NRGBA(p.Border, 200))),
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
	// The economy coin rides just left of the figure: its right inset is
	// the wallet's own width plus the gutter, updated in Sync (the wallet is
	// a live number, so the glyph tracks it and never crowds the text).
	coin := theme.NewIcon("stat_cash", theme.IconInline, p.Good)
	coin.GetWidget().LayoutData = widget.AnchorLayoutData{
		HorizontalPosition: widget.AnchorLayoutPositionEnd,
		VerticalPosition:   widget.AnchorLayoutPositionCenter,
		Padding:            &widget.Insets{Right: theme.Px(theme.ContentsPad)},
	}
	h.walletIcon = coin
	h.walletFace = env.FaceMono(theme.StepSmall)
	clock := theme.NewIcon("hud_time", theme.IconInline, p.Accent)
	clock.GetWidget().LayoutData = widget.AnchorLayoutData{
		HorizontalPosition: widget.AnchorLayoutPositionStart,
		VerticalPosition:   widget.AnchorLayoutPositionCenter,
		Padding:            &widget.Insets{Left: theme.Px(theme.ContentsPad)},
	}
	h.strip = widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.PanelBox(p, theme.Solid(p.BG), theme.NRGBA(p.Border, 200))),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(0, theme.Px(hudStripH)),
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)
	h.strip.AddChild(clock, coin, h.timeLbl, h.walletLbl)

	// The EWAR effect boxes: one per effect kind, above the ship status and
	// hidden while the effect is off (the target row's visibility pattern).
	// Their glyphs are the module class icons — the same keys the rack
	// draws for the modules that project them.
	h.effects = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(hudColGap)),
		)),
	)
	h.eff[0] = h.newEffect(theme.ModuleIcon(content.KindStasisWeb), p.Warn)
	h.eff[1] = h.newEffect(theme.ModuleIcon(content.KindWarpScram), p.Danger)
	h.effects.AddChild(h.eff[0].root, h.eff[1].root)
	h.effects.GetWidget().SetVisibility(widget.Visibility_Hide)

	// The left column: strip over status, bottom-aligned in the pair (the
	// rack is taller, so the pair's own bottom is the shared baseline). The
	// effects row sits directly above the status, under the strip.
	left := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(hudColGap)),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{
			Position: widget.RowLayoutPositionEnd,
		})),
	)
	left.AddChild(h.strip, h.effects, h.status)

	h.pair = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			widget.RowLayoutOpts.Spacing(theme.Px(hudPairGap)),
		)),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionCenter,
			VerticalPosition:   widget.AnchorLayoutPositionEnd,
			// The 240 px portrait makes the pair 666 px on a four-slot hull
			// (600 on the player's three): a viewport-centred seat would put
			// its left edge 54 px inside the log's pocket at 1280×720 (21 px
			// on a three-slot hull). The left padding carries the log's own
			// width, so the pair centres in the band the pocket leaves free —
			// 439..1105 at 1280 (log right edge 361) and still fully on
			// screen at the 1024×700 minimum (311..977), where the pocket
			// overlap is the log's documented one.
			Padding: &widget.Insets{Left: theme.Px(10 + LogDefaultW), Right: theme.Px(10), Bottom: theme.Px(10)},
		})),
	)
	h.pair.AddChild(left)

	// Target boxes: a top-center row, hidden while nothing is locked. It
	// mounts as its own TOP chrome column (h.cards, see Cards) because the
	// doubled Overview now covers the band at its default placement.
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
		// Hidden until Sync sees a lock: an all-hidden row lays out 0×0 and
		// so claims no band (the top chrome's empty state).
		h.boxes[i].root.GetWidget().SetVisibility(widget.Visibility_Hide)
		h.targetRow.AddChild(h.boxes[i].root)
	}
	h.cards = &hudChrome{col: h.targetRow}
	h.feed = h.newFeed()

	h.container = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			StretchHorizontal: true,
			StretchVertical:   true,
		})),
	)
	h.container.AddChild(h.pair)
	return h
}

// hudChrome is one TOP-chrome column of the HUD (the target-card row, the
// combat feed): mounted through Kit.AddTopContainer, so it paints after every
// window and its presses win over a window underneath — the rail's contract.
// With nothing to show its content collapses, the column lays out 0×0 and
// claims no band, so the layer below keeps its clicks.
type hudChrome struct{ col *widget.Container }

// Container is the column: the root anchor layout positions it, the topmost
// input layer claims it.
func (c *hudChrome) Container() *widget.Container { return c.col }

// SetWindowPass parks the column out of ui.Draw's window pass and brings it
// back for Kit.Draw's top pass — the rail's parking verbatim: the root
// layout still lays it out, only the paint stops.
func (c *hudChrome) SetWindowPass(on bool) {
	vis := widget.Visibility_Show
	if !on {
		vis = widget.Visibility_Hide_Blocking
	}
	c.col.GetWidget().SetVisibility(vis)
}

// Draw paints the column — Kit.Draw's top pass, after every window.
func (c *hudChrome) Draw(screen *ebiten.Image) { c.col.Render(screen) }

// Cards returns the target-card row's TOP chrome column for
// Kit.AddTopContainer (the game mounts it with the row's own anchor data).
func (h *Hud) Cards() TopChrome { return h.cards }

// Feed returns the combat feed's TOP chrome column for Kit.AddTopContainer.
// The HUD re-anchors it above the ship status every Sync (the strip's top
// edge and left edge), so the mount only has to seed a bottom-centre seat.
func (h *Hud) Feed() TopChrome { return &h.feed.hudChrome }

// newFeed builds the combat feed column: one panel of preallocated line rows,
// each three text runs (plain, the important span, plain). It starts hidden.
func (h *Hud) newFeed() *hudFeed {
	p := h.th.Pal()
	f := &hudFeed{}
	f.rows = widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.PanelBox(p, theme.Solid(p.BG), theme.NRGBA(p.Border, 160))),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.SpSm))),
		)),
	)
	for i := range f.rowsAt {
		row := &f.rowsAt[i]
		box := widget.NewContainer(
			widget.ContainerOpts.Layout(widget.NewRowLayout(
				widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
			)),
			widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(hudFeedW), theme.Px(hudFeedLineH))),
		)
		run := func() *widget.Text {
			return widget.NewText(
				widget.TextOpts.Text("", h.th.Face(theme.StepTiny), theme.Solid(p.Text)),
				widget.TextOpts.Position(widget.TextPositionStart, widget.TextPositionCenter),
			)
		}
		row.pre, row.key, row.post = run(), run(), run()
		box.AddChild(row.pre, row.key, row.post)
		row.box = box
		f.rows.AddChild(box)
		theme.ShowWidget(box.GetWidget(), false)
	}
	theme.ShowWidget(f.rows.GetWidget(), false)
	f.col = widget.NewContainer(widget.ContainerOpts.Layout(widget.NewAnchorLayout()))
	f.col.AddChild(f.rows)
	return f
}

// Push forwards a frame's sim events into the combat feed: only the
// player-relevant exchange becomes a line (the log keeps every line it
// already prints). Lines ride hudFeedFrames and the oldest drop past the cap,
// so the feed scrolls up.
func (h *Hud) Push(evs []sim.Event) {
	if h.cv == nil {
		return
	}
	player := h.cv.Player()
	for _, ev := range evs {
		l, ok := h.feedLine(ev, player)
		if !ok {
			continue
		}
		l.left = hudFeedFrames
		h.feed.lines = append(h.feed.lines, l)
	}
	if over := len(h.feed.lines) - hudFeedLines; over > 0 {
		h.feed.lines = append(h.feed.lines[:0], h.feed.lines[over:]...)
	}
}

// feedLine turns one sim event into a feed line, or reports false for events
// the feed does not speak for. The key span is the number dealt or taken,
// else the effect verb or the kill; a loss (the player was hit) is red, a
// gain (the player dealt it) blue.
func (h *Hud) feedLine(ev sim.Event, player ecs.Entity) (hudFeedLine, bool) {
	switch ev.Kind {
	case sim.EvCombat:
		switch ev.Phase {
		case sim.CombatDamage:
			if ev.Value <= 0 {
				return hudFeedLine{}, false
			}
			amt := hudHP(ev.Value)
			if ev.Target == player {
				return hudFeedLine{pre: ev.Name + " hits you for ", key: amt, loss: true}, true
			}
			if ev.Actor == player {
				if tgt, ok := h.cv.Info(ev.Target); ok {
					return hudFeedLine{pre: "You hit " + tgt.Name + " for ", key: amt}, true
				}
			}
		case sim.CombatWeb, sim.CombatScram:
			verb, hit := "web", "Webbed"
			if ev.Phase == sim.CombatScram {
				verb, hit = "scramble", "Scrambled"
			}
			if ev.Target == player {
				return hudFeedLine{key: hit, post: " by " + ev.Name, loss: true}, true
			}
			if ev.Actor == player {
				if tgt, ok := h.cv.Info(ev.Target); ok {
					return hudFeedLine{pre: "You ", key: verb + " " + tgt.Name}, true
				}
			}
		}
	case sim.EvDestroyed:
		if ev.Actor == player {
			return hudFeedLine{key: "You were destroyed", loss: true}, true
		}
		if ev.Target == player && ev.Name != "" {
			return hudFeedLine{key: ev.Name + " destroyed"}, true
		}
	}
	return hudFeedLine{}, false
}

// syncFeed stages the feed: a line's frames run out onto the floor, the newest
// hudFeedLines show newest-at-the-bottom (a fresh line pushes the rest up),
// rows are rewritten only when their line changed, and the column sits above
// the ship status.
func (h *Hud) syncFeed() {
	live := h.feed.lines[:0]
	for _, l := range h.feed.lines {
		if l.left--; l.left > 0 {
			live = append(live, l)
		}
	}
	h.feed.lines = live
	p := h.th.Pal()
	off := len(h.feed.rowsAt) - len(h.feed.lines) // spare rows stay hidden on top
	for i := range h.feed.rowsAt {
		row := &h.feed.rowsAt[i]
		src := i - off
		var staged hudFeedLine
		shown := src >= 0 && src < len(h.feed.lines)
		if shown {
			staged = h.feed.lines[src]
		}
		staged.left = 0 // the countdown is not the row's content: never a repaint
		if row.painted && row.staged == staged {
			continue
		}
		row.painted, row.staged = true, staged
		theme.ShowWidget(row.pre.GetWidget(), shown && staged.pre != "")
		theme.ShowWidget(row.key.GetWidget(), shown && staged.key != "")
		theme.ShowWidget(row.post.GetWidget(), shown && staged.post != "")
		row.pre.Label, row.key.Label, row.post.Label = staged.pre, staged.key, staged.post
		col := p.Info
		if staged.loss {
			col = p.Danger
		}
		row.key.SetColor(col)
		theme.ShowWidget(row.box.GetWidget(), shown)
	}
	theme.ShowWidget(h.feed.rows.GetWidget(), len(h.feed.lines) > 0)
	// Seat it above the strip (the top of the ship-info column) with its left
	// edge on that column's: a live read, because the rack's height and the UI
	// scale both move the pair. The anchor data keeps a later root layout on
	// the same seat; the location applies it now, because a layout only runs
	// when something changed size.
	if root, strip := h.container.GetWidget().Rect, h.strip.GetWidget().Rect; !root.Empty() && !strip.Empty() {
		pw, ph := h.feed.rows.PreferredSize()
		pad := &widget.Insets{Left: strip.Min.X, Bottom: root.Max.Y + theme.Px(hudFeedGap) - strip.Min.Y}
		h.feed.col.GetWidget().LayoutData = widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionStart,
			VerticalPosition:   widget.AnchorLayoutPositionEnd,
			Padding:            pad,
		}
		bottom := pad.Apply(root).Max.Y
		// Moving the column does not dirty its own layout: re-lay its rows out
		// at the new origin (the root's layout only places the column). Both
		// only when the seat actually moved.
		w := h.feed.col.GetWidget()
		rect := image.Rect(strip.Min.X, bottom-ph, strip.Min.X+pw, bottom)
		if w.Rect != rect {
			w.SetLocation(rect)
			h.feed.col.RequestRelayout()
		}
	}
}

// MountRack slots the module rack into the bottom-center pair, immediately
// right of the status column. The rack keeps its own container (Sync, hover,
// hit rects); only its layout slot changes.
func (h *Hud) MountRack(c *widget.Container) {
	c.GetWidget().LayoutData = widget.RowLayoutData{Position: widget.RowLayoutPositionEnd}
	h.pair.AddChild(c)
}

// SetSelector installs where a target-card click reports its entity — the
// game passes Kit.Select, so a card click is exactly a world click (the
// Selected Item panel and the overview highlight follow).
func (h *Hud) SetSelector(fn func(ecs.Entity)) { h.selectFn = fn }

// SetTargetPicker installs the source of the player's current selection
// (the game passes Kit.Selected): the locked target's card then carries the
// selection border, the same selection the overview row and world click set.
func (h *Hud) SetTargetPicker(fn func() ecs.Entity) { h.pick = fn }

// newEffect builds one effect box: a tile (glyph + cycle ring) over its
// remaining-time line, the rack cell's tile-and-name layout in miniature.
func (h *Hud) newEffect(glyph string, tint color.RGBA) *hudEffect {
	e := &hudEffect{glyph: glyph, tint: tint, drawn: -1}
	size := theme.Px(hudEffTile)
	e.img = ebiten.NewImage(size, size)
	e.gfx = widget.NewGraphic(
		widget.GraphicOpts.Image(e.img),
		widget.GraphicOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(size, size),
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				HorizontalPosition: widget.AnchorLayoutPositionCenter,
				VerticalPosition:   widget.AnchorLayoutPositionCenter,
			}),
		),
	)
	tile := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(hudEffW), size)),
	)
	tile.AddChild(e.gfx)
	e.lbl = widget.NewText(
		widget.TextOpts.Text("", h.th.Face(theme.StepTiny), theme.Solid(h.th.Pal().Dim)),
		widget.TextOpts.Position(widget.TextPositionCenter, widget.TextPositionCenter),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(hudEffW), theme.Px(hudEffNameH))),
	)
	e.root = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
		)),
	)
	e.root.AddChild(tile, e.lbl)
	theme.ShowWidget(e.root.GetWidget(), false)
	return e
}

// setEffect stages one effect's remaining time (until = its sim-time expiry;
// until <= now = off) and repaints the tile only when the ring moved. The
// ring's full circle is the longest remaining time seen this spell (the
// effect is refreshed every cycle, so the ring refills on each one and
// depletes after the last — the rack's cycle ring, read from the sim's
// expiry rather than a module's cycle clock).
func (h *Hud) setEffect(e *hudEffect, now, until float64) {
	rem := until - now
	if rem <= 0 {
		if e.peak != 0 || e.drawn != -1 {
			e.peak, e.drawn, e.lbl.Label = 0, -1, ""
		}
		theme.ShowWidget(e.root.GetWidget(), false)
		return
	}
	theme.ShowWidget(e.root.GetWidget(), true)
	e.lbl.Label = theme.GroupFloat(rem, 1) + " s"
	if rem > e.peak {
		e.peak = rem
	}
	frac := math.Round(hudFrac01(rem/e.peak)*1000) / 1000
	if frac != e.drawn {
		h.paintEffect(e, frac)
	}
}

// paintEffect paints one effect tile: the palette cell with the effect's
// glyph and the depletion ring over it (the rack's ring geometry, scaled to
// the tile), tinted by the effect's own colour.
func (h *Hud) paintEffect(e *hudEffect, frac float64) {
	e.drawn = frac
	p := h.th.Pal()
	e.img.Clear()
	box := e.img.Bounds()
	s := float32(theme.Scale())
	theme.FillRect(e.img, box, theme.Solid(p.Cell))
	vector.StrokeRect(e.img,
		s/2, s/2, float32(box.Dx())-s, float32(box.Dy())-s,
		s, theme.NRGBA(p.Border, 160), false)
	glyph := icons.Get(e.glyph, theme.Px(theme.IconButton), e.tint)
	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Translate(
		float64((box.Dx()-glyph.Bounds().Dx())/2),
		float64((box.Dy()-glyph.Bounds().Dy())/2))
	e.img.DrawImage(glyph, opts)

	// The ring over the glyph: the rack's ring geometry, scaled to the tile.
	cx, cy := float64(box.Dx())/2, float64(box.Dy())/2
	rad := float64(theme.Px(hudEffTile)-2*hudEffRingW) / 2
	drawCycleRing(e.img, float32(cx), float32(cy), float32(rad),
		float32(hudEffRingW*theme.Scale()), frac, true, e.tint, p)
}

// clickCard is the card's click seam: a left click selects the card's locked
// entity through the host's selector — the same selection a world click or an
// overview row sets, never a lock or a camera move. A ghost (a card blinking
// out) answers nothing.
func (h *Hud) clickCard(b *hudBox, a *widget.WidgetMouseButtonClickedEventArgs) {
	if a.Button != ebiten.MouseButtonLeft || h.selectFn == nil || !b.live {
		return
	}
	if e := b.card.entity; e != 0 {
		h.selectFn(e)
	}
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
		bar := image.Rect(0, y, theme.Px(hudHealthW), y+theme.Px(hudBarH))
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
		drawBarGlyph(h.health, hudLayerGlyphs[i], bar, cols[i], p.BGHard, theme.StatIconSize)
	}
}

// hudLayerGlyphs are the three layer bars' glyphs (shield, armor, structure).
var hudLayerGlyphs = [3]string{"mod_defense", "stat_armor", "stat_structure"}

// drawBarGlyph paints one glyph at a bar's left inside edge, tinted the bar's
// own colour (never the stat category's) and clamped to the bar's height.
func drawBarGlyph(dst *ebiten.Image, name string, bar image.Rectangle, tint, dark color.RGBA, size int) {
	if size > bar.Dy()-4 {
		size = bar.Dy() - 4
	}
	if name == "" || size < 4 || bar.Dx() < size+6 {
		return
	}
	x := bar.Min.X + 3
	y := bar.Min.Y + (bar.Dy()-size)/2
	// The layer-coloured glyph reads against its own fill through a one-pixel
	// dark outline (the bars' value text is dark over the fill for the same
	// reason) — the glyph itself keeps the bar's colour.
	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Translate(float64(x-1), float64(y-1))
	dst.DrawImage(icons.Get(name, size+2, dark), opts)
	opts = &ebiten.DrawImageOptions{}
	opts.GeoM.Translate(float64(x), float64(y))
	dst.DrawImage(icons.Get(name, size, tint), opts)
}

// newBox builds one target card from the fixed widget pool: a container
// holding a single Graphic that displays the card image. The graphic also
// carries the card's click (a press/release on the card selects its target).
func (h *Hud) newBox(env theme.Theme) *hudBox {
	b := &hudBox{img: ebiten.NewImage(theme.Px(hudBoxW), theme.Px(hudBoxH))}
	g := widget.NewGraphic(
		widget.GraphicOpts.Image(b.img),
		widget.GraphicOpts.WidgetOpts(
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
			widget.WidgetOpts.MouseButtonClickedHandler(func(a *widget.WidgetMouseButtonClickedEventArgs) {
				h.clickCard(b, a)
			}),
		),
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
// glyph, the distance to its own line less the module glyphs it carries.
func (h *Hud) boxLabels(name, dist string, mods int) (string, string) {
	card := image.Rect(0, 0, theme.Px(hudBoxW), theme.Px(hudBoxH))
	room := float64(targetCardName(card).Dx())
	if room < float64(theme.Px(hudBoxMinNameW)) {
		room = float64(theme.Px(hudBoxMinNameW))
	}
	name = widgets.Ellipsize(name, room, h.measure)
	distRoom := targetCardDist(card).Dx() - cardModsWidth(mods)
	if w := h.measureTiny(dist); w > float64(distRoom) {
		dist = widgets.Ellipsize(dist, float64(distRoom), h.measureTiny)
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
	dst.Clear()
	// The selection border: the palette's Accent at double weight, the one
	// token that already means "chosen" in the Windows and the lock strip.
	// Unselected cards keep the faint hairline they always had.
	edge, w := theme.NRGBA(p.Border, 200), 1
	if c.selected {
		edge, w = theme.NRGBA(p.Accent, 235), 2
	}
	theme.DrawRoundBox(dst, card, theme.Solid(p.BG), edge, w,
		theme.AllRadii(theme.RadiusPanel))

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
	// The modules cycling on this target, right-aligned on the distance
	// line (drawn after the text, which is already ellipsized clear of
	// them): each is the rack cell in miniature — glyph, depleting ring.
	for i := 0; i < c.nmods && i < hudBoxMods; i++ {
		m := c.mods[i]
		if m.glyph == "" {
			continue
		}
		box := targetCardMod(card, i)
		if m.ring >= 0 {
			drawModRing(dst, box, m.ring, m.lit, p)
		}
		size := theme.Px(hudBoxModGlyph)
		x := box.Min.X + (box.Dx()-size)/2
		y := box.Min.Y + (box.Dy()-size)/2
		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Translate(float64(x), float64(y))
		dst.DrawImage(icons.Get(m.glyph, size, p.Text), opts)
	}

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
		drawBarGlyph(dst, hudLayerGlyphs[i], bar, col, p.BGHard, theme.StatIconSize)
		vector.StrokeRect(dst,
			float32(bar.Min.X)+s/2, float32(bar.Min.Y)+s/2,
			float32(bar.Dx())-s, float32(bar.Dy())-s,
			s, theme.NRGBA(p.Border, 160), false)
	}
}

// drawModRing paints one module mini-cell's cycle ring over its box: a full
// dim track and the remaining fraction from the top, clockwise — the rack's
// ring (syncRing) scaled to the cell, in the same palette tokens.
func drawModRing(dst *ebiten.Image, box image.Rectangle, remaining float64, lit bool, p theme.Palette) {
	s := theme.Scale()
	cx, cy := float64(box.Min.X)+float64(box.Dx())/2, float64(box.Min.Y)+float64(box.Dy())/2
	rad := float64(theme.Px(hudBoxModCell)-2*hudBoxModRingW) / 2
	drawCycleRing(dst, float32(cx), float32(cy), float32(rad),
		float32(hudBoxModRingW*s), remaining, lit, p.Accent, p)
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
	// An engaged warp drive has no useful grid speed to print: the figure
	// reads WARP while any phase of the drive is live (the wedge keeps its
	// clamped gauge).
	if info, ok := s.Info(player); ok && info.Warping {
		h.speedLbl.Label = "WARP"
	} else {
		h.speedLbl.Label = theme.FormatSpeed(speed)
	}
	h.timeLbl.Label = "×" + strconv.Itoa(h.timeScale) + " time"
	h.walletLbl.Label = theme.FormatCredits(s.Wallet())
	if h.walletIcon != nil && h.walletFace != nil {
		walletW, _ := text.Measure(h.walletLbl.Label, *h.walletFace, 0)
		timeW, _ := text.Measure(h.timeLbl.Label, *h.th.FaceBold(theme.StepSmall), 0)
		coin := float64(theme.Px(theme.IconInline + 2*theme.IconGap))
		room := float64(h.strip.GetWidget().Rect.Dx()) - float64(theme.Px(2*theme.ContentsPad))
		// A long wallet in a narrow strip drops its coin rather than overlap
		// the time readout (the figure never moves).
		theme.ShowWidget(h.walletIcon.GetWidget(), room <= 0 || walletW+timeW+coin <= room)
		h.walletIcon.GetWidget().LayoutData = widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionEnd,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
			Padding:            &widget.Insets{Right: theme.Px(theme.ContentsPad) + int(math.Ceil(walletW)) + theme.Px(theme.IconGap)},
		}
	}

	// 4 — the EWAR effects on the player (stasis web, warp scramble): each
	// live expiry shows its box with the time left, nothing else has an
	// icon.
	now := s.Time()
	webUntil, scramUntil := 0.0, 0.0
	if m, ok := s.Motion(player); ok {
		webUntil = m.WebbedUntil
	}
	if sh, ok := s.ShipState(player); ok {
		scramUntil = sh.ScrambledUntil
	}
	h.setEffect(h.eff[0], now, webUntil)
	h.setEffect(h.eff[1], now, scramUntil)
	theme.ShowWidget(h.effects.GetWidget(), webUntil > now || scramUntil > now)

	// 5 — locked-target cards (name, distance, lock strip, layer bars, the
	// modules cycling on the target, and the selection border).
	var locks []sim.Lock
	if tg, ok := s.Targeting(player); ok {
		locks = tg.Locks
	}
	var selected ecs.Entity
	if h.pick != nil {
		selected = h.pick()
	}
	// The locks the boxes are about to show: a card that left the row only
	// blinks when its target is gone from the whole row (a lock list that
	// simply shifted down keeps every card live).
	var locked [hudBoxPool]ecs.Entity
	for i := range locked {
		if i < len(locks) {
			locked[i] = locks[i].Target
		}
	}
	for i, box := range h.boxes {
		if i >= len(locks) {
			h.blinkBox(box, locked)
			continue
		}
		theme.ShowWidget(box.root.GetWidget(), true)
		box.live, box.ghost, box.ghostOn = true, 0, false
		l := locks[i]
		info, hasInfo := s.Info(l.Target)
		card := targetCard{entity: l.Target, selected: selected != 0 && l.Target == selected,
			lock: hudFrac01(l.Progress), name: "…", tint: h.th.Pal().Text}
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
		card.mods, card.nmods = h.cardMods(player, l.Target)
		card.name, card.dist = h.boxLabels(card.name, card.dist, card.nmods)
		h.setCard(box, card)
	}

	// 6 — the combat feed: spent lines drop and the column re-seats above the
	// ship status (Sync refreshes it; Push stages the lines from events).
	h.syncFeed()
}

// blinkBox handles a card slot with no lock this frame. A card whose target
// left the row (destroyed, warped off-grid, unlocked) stays as a ghost for
// hudGhostFrames, alternating visibility every hudBlinkFrames — the "target
// gone" tell — then disappears. The ghost keeps the last painted image, is
// never repainted and never answers a click (live stays false in Sync's card
// branch only). A target that merely moved to another slot is still locked:
// nothing blinks.
func (h *Hud) blinkBox(b *hudBox, locked [hudBoxPool]ecs.Entity) {
	b.live = false
	still := false
	for _, e := range locked {
		if e != 0 && e == b.card.entity {
			still = true
			break
		}
	}
	if b.card.entity == 0 || still {
		b.ghost, b.ghostOn = 0, false
		theme.ShowWidget(b.root.GetWidget(), false)
		return
	}
	if b.ghost == 0 && b.root.GetWidget().GetVisibility() == widget.Visibility_Show {
		b.ghost = hudGhostFrames // the falling edge: the lock just went away
	}
	if b.ghost == 0 {
		theme.ShowWidget(b.root.GetWidget(), false)
		return
	}
	b.ghost--
	b.ghostOn = b.ghost > 0 && (b.ghost/hudBlinkFrames)%2 == 1
	theme.ShowWidget(b.root.GetWidget(), b.ghostOn)
}

// cardMods stages the mini-cells of my modules cycling on one target (the
// target card's module strip): every slot whose activation target is that
// entity, in slot order, capped at the room a card has. Each cell carries the
// rack's cycle ring state (syncCell's rule): a cycle in flight depletes its
// ring — lit while the module is active or its bay loads, greyed while a
// cancelled cycle runs out; an idle module keeps the bare glyph.
func (h *Hud) cardMods(player, tgt ecs.Entity) ([hudBoxMods]hudBoxMod, int) {
	var out [hudBoxMods]hudBoxMod
	fit, ok := h.cv.Fitting(player)
	if !ok || tgt == 0 {
		return out, 0
	}
	now := h.cv.Time()
	n := 0
	for _, class := range []string{content.SlotHigh, content.SlotMid, content.SlotLow} {
		if n >= hudBoxMods {
			break
		}
		rts := fit.Runtimes(class)
		for i := range rts {
			if n >= hudBoxMods {
				break
			}
			slot := sim.SlotRef{Class: class, Index: i}
			if e, ok := h.cv.ModuleTarget(player, slot); !ok || e != tgt {
				continue
			}
			active := rts[i].Active
			clip, hasClip := h.cv.Clip(slot)
			loading := hasClip && clip.Loading
			pending := !active && !loading && rts[i].CycleEnds > 0 && rts[i].CycleEnds > now
			// The activation binding outlives the cycle: an idle module keeps
			// its slot's target, so only a live cycle draws a cell (the rack
			// greys a cancelled one until its clock runs out).
			if !active && !pending && !loading {
				continue
			}
			def, ok := h.cv.Defs().Module(rts[i].Def)
			if !ok {
				continue
			}
			m := hudBoxMod{glyph: theme.ModuleIconFor(def), ring: -1}
			if rts[i].CycleEnds > 0 && (active || pending || loading) {
				period := def.ActivationTime
				if loading && def.Weapon != nil && def.Weapon.ShotTime > 0 {
					period = def.Weapon.ShotTime
				}
				if period > 0 {
					progress := hudFrac01((now - (rts[i].CycleEnds - period)) / period)
					m.ring = math.Round(hudFrac01(1-progress)*rackCycleMax) / rackCycleMax
					m.lit = active || loading
				}
			}
			out[n] = m
			n++
		}
	}
	return out, n
}

// paintPortrait repaints the hull portrait: the hull's art contained in the
// 4:1 design box — the image is built at Px(hudPortraitW)×Px(hudPortraitH),
// so a scale change rebuilds the kit and the portrait with it.
func (h *Hud) paintPortrait(hull content.ShipID) {
	h.portraitHull = hull
	h.portraitDraws++
	paintHullPortrait(h.portrait, hull)
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
	// The speed glyph sits inside the wedge's bottom-left edge, in the fill's
	// own colour (the wedge is the bar: no category tint here).
	size := theme.StatIconSize
	x, y := wedgeGlyphXY(int(w), int(hh), size)
	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Translate(float64(x), float64(y))
	h.wedge.DrawImage(icons.Get("stat_speed", size, p.Accent), opts)
}

// wedgeGlyphXY places the speed glyph at the wedge's bottom-left edge, pulled
// left when the doubled glyph would otherwise clip the image's right side.
func wedgeGlyphXY(w, h, size int) (int, int) {
	cx := float64(w) / 2
	y := h - size - 1
	half := float64(h-y) / float64(h) * cx
	x := cx - half + 1
	if x+float64(size) > float64(w-1) {
		x = float64(w - 1 - size)
	}
	if x < 1 {
		x = 1
	}
	return int(math.Round(x)), y
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
