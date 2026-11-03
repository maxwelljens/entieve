package screens

import (
	"image"
	"image/color"
	"strings"

	eimage "github.com/ebitenui/ebitenui/image"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// The proficiency card: one single-instance dialog opened by a node click on
// the tree (CONTEXT.md "Proficiency card"). It names the proficiency, the
// pilot's level in it, what it is for and which acts pay it practice. The
// prose is content (content.FamilyInfo), the level is the live read
// (ContentView.Proficiency), so retargeting on another node is a handful of
// label writes.
//
// It hugs its page up to profCardMaxH; a taller page scrolls in the rail's
// gutter instead of growing over the tree, so no page can hide its own
// practice list behind the frame.
const (
	profCardW = 470
	// profCardH is the design floor: the first mount's placement fallback
	// and the shortest the frame goes.
	profCardH = 160
	// profCardMaxH caps the frame. 400 keeps the card under 56% of a 720 px
	// viewport, so the clamped placement still leaves the clicked node and
	// its neighbours visible around it; the tallest shipped page (a hull
	// card with four practice lines) needs about 230 and never scrolls.
	profCardMaxH = 400

	// profCardPipsH is the level line's height: the pips plus the tiny
	// face's line box. The line carries the tree node's own three level
	// reads (the pips, the "2/5" readout and the sim's gate sentence), so
	// the card and the node it was opened from cannot word a level
	// differently: the pips come from drawProfPips, the readout from
	// profLevelText, the gate from the sim. It stays above the viewport:
	// the level is the headline a long page must not scroll away.
	profCardPipsH = 20

	// profCardInnerW is the prose's wrap width: the frame minus the body's
	// padding, the rail's reserved gutter and the page's own padding.
	profCardInnerW = profCardW - 2*theme.ContentsPad - theme.ScrollGutter - 2*theme.SpSm
)

// ProfInfoWin is the card. It holds the family id it was last pointed at; a
// new Show swaps the page in place and the Kit raises the one window instead
// of mounting a second.
type ProfInfoWin struct {
	th theme.Theme

	dialogShell
	title *theme.ChromeText

	family   string
	gate     string
	level    *widget.Graphic
	levelImg *ebiten.Image
	desc     *widget.Text
	practice *widget.Text

	page   *widget.Container
	scroll *widget.ScrollContainer
	rail   *widgets.ScrollRail

	// The level line's last spec (the tests compare it with the node's).
	pipLevel, pipMax int
	pipLocked        bool
}

// NewProfInfoWin builds the closed card over the shared environment. Mount it
// through the Kit (the tree's node click emits ShowProfFamily).
func NewProfInfoWin(env widgets.Seams) *ProfInfoWin {
	p := env.Pal()
	c := &ProfInfoWin{th: env}

	maxW := float64(theme.Px(profCardInnerW))
	wrapped := func(face *text.Face, col color.RGBA) *widget.Text {
		return widget.NewText(
			widget.TextOpts.Text("", face, theme.Solid(col)),
			widget.TextOpts.MaxWidth(maxW),
		)
	}
	head := func(label string) *widget.Text {
		return widget.NewText(
			widget.TextOpts.Text(label, env.Face(theme.StepTiny), theme.Solid(p.Head)),
		)
	}

	c.levelImg = ebiten.NewImage(theme.Px(profCardW-2*theme.ContentsPad), theme.Px(profCardPipsH))
	c.level = widget.NewGraphic(widget.GraphicOpts.Image(c.levelImg),
		widget.GraphicOpts.WidgetOpts(widget.WidgetOpts.MinSize(c.levelImg.Bounds().Dx(), c.levelImg.Bounds().Dy())))

	c.page = widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.SpSm)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.SpSm))),
		)),
	)
	c.desc = wrapped(env.Face(theme.StepSmall), p.Text)
	c.practice = wrapped(env.Face(theme.StepTiny), p.Dim)
	c.page.AddChild(head("WHAT IT IS FOR"), c.desc, head("PRACTICE"), c.practice)

	// The page rides a scroll viewport (the Show Info idiom), so a long
	// description or practice list scrolls in the rail's gutter instead of
	// being clipped by the frame.
	c.scroll = widget.NewScrollContainer(
		widget.ScrollContainerOpts.Content(c.page),
		widget.ScrollContainerOpts.StretchContentWidth(),
		widget.ScrollContainerOpts.Image(&widget.ScrollContainerImage{
			Idle:     theme.Bordered(theme.WindowBody(p.BG), theme.NRGBA(p.Border, 160), 1),
			Disabled: theme.Bordered(theme.NRGBA(p.Cell, 140), theme.NRGBA(p.Border, 110), 1),
			Mask:     eimage.NewNineSliceColor(theme.WindowBody(p.BG)),
		}),
	)
	c.scroll.GetWidget().ScrolledEvent.AddHandler(func(args any) {
		if ev, ok := args.(*widget.WidgetScrolledEventArgs); ok {
			c.scrollBy(ev.Y)
		}
	})
	widgets.AttachScrollContent(c.scroll, c.page)
	c.rail = widgets.NewScrollRail(env, c.scroll, c.scroll, c.page)

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.WindowBodyBox(p, theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200))),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, true}),
			widget.GridLayoutOpts.Spacing(0, theme.Px(theme.SpSm)),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	body.AddChild(c.level, c.rail.Container())

	bar, title := theme.NewTitleBar(env, "Proficiency", "stat_tracking")
	c.title = title
	c.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(bar, theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(profCardW), theme.Px(profCardH)),
		// Fallback only: the kit centres it on the real viewport.
		widget.WindowOpts.Location(image.Rect(theme.Px(405), theme.Px(225),
			theme.Px(405+profCardW), theme.Px(225+profCardH))),
	)
	theme.ShowWidget(c.win.GetContainer().GetWidget(), false)
	return c
}

// Win returns the window for the kit's dialog mount.
func (c *ProfInfoWin) Win() *widget.Window { return c.win }

// Family returns the family the card currently names ("" before the first
// show); the tests read it to prove a click retargeted the one window.
func (c *ProfInfoWin) Family() string { return c.family }

// Gate is the gate sentence the level line carries ("" while the family is
// open): the tests pin it against the sim's and the tree node's wording.
func (c *ProfInfoWin) Gate() string { return c.gate }

// Level is the card's painted level line (the tests read the pips off it).
func (c *ProfInfoWin) Level() *ebiten.Image { return c.levelImg }

// Pips is the level line's last spec: the level, the cap and whether the
// branch is shut, the same three values the tree's node hands the painter.
func (c *ProfInfoWin) Pips() (level, max int, locked bool) { return c.pipLevel, c.pipMax, c.pipLocked }

// Sync mirrors the page's scroll offset onto the rail and hides the rail
// while the page fits (the Show Info idiom; Kit.Sync drives it).
func (c *ProfInfoWin) Sync() { c.rail.Sync() }

// Show points the card at one family and reveals it. level and gate are the
// tree node's own readouts, so the card can never disagree with the node it
// was opened from: the same pips, the same level readout, the same gate
// sentence.
func (c *ProfInfoWin) Show(family string, level int, gate string) {
	card := content.FamilyInfo(family)
	c.family, c.gate = family, gate
	c.title.Label = content.FamilyLabel(family)
	c.paintLevel(level, gate)
	c.desc.Label = card.Desc
	lines := make([]string, 0, len(card.Practice))
	for _, line := range card.Practice {
		lines = append(lines, "· "+line)
	}
	c.practice.Label = strings.Join(lines, "\n")
	c.scroll.ScrollTop = 0 // a new page starts at its top
	c.fit()
	theme.ShowWidget(c.win.GetContainer().GetWidget(), true)
}

// fit sizes the frame to the page it now holds: it hugs the prose up to
// profCardMaxH and caps there. Show runs it before the Kit places the window
// (the dialog's place hook), so the on-screen clamp sees the height the card
// will paint rather than the previous family's.
func (c *ProfInfoWin) fit() {
	_, page := c.page.PreferredSize()
	h := page + 2*theme.Px(theme.ContentsPad) + theme.Px(profCardPipsH) +
		theme.Px(theme.SpSm) + theme.Px(theme.TitleBarH)
	h = min(h, theme.Px(profCardMaxH))
	r := c.win.GetContainer().GetWidget().Rect
	if r.Empty() {
		r = image.Rect(theme.Px(405), theme.Px(225), theme.Px(405+profCardW), theme.Px(225)+h)
	} else {
		r.Max = image.Pt(r.Min.X+theme.Px(profCardW), r.Min.Y+h)
	}
	c.win.SetLocation(r) // the MinSize floor (profCardH) still applies
}

// scrollBy turns wheel notches into rows of scroll offset over the page
// overflow, the Show Info convention (positive notch scrolls up).
func (c *ProfInfoWin) scrollBy(notches float64) {
	over := c.rail.Overflow()
	if over <= 0 {
		c.scroll.ScrollTop = 0
		return
	}
	step := notches * infoWheelN * float64(widgets.TipLineH(c.th.Face(theme.StepSmall)))
	top := c.scroll.ScrollTop - step/over
	c.scroll.ScrollTop = max(0, min(1, top))
}

// paintLevel draws the card's level line the way the tree draws its node:
// the pips (filled to the level, dim on a shut branch), the level readout
// beside them ("2/5", the node's own words) and the sim's gate sentence.
// One line carries all three, so the card keeps its fixed height (the words
// it replaces took a line of their own).
func (c *ProfInfoWin) paintLevel(level int, gate string) {
	c.pipLevel, c.pipMax, c.pipLocked = level, content.ProficiencyMax, gate != ""
	c.levelImg.Clear()
	pal := c.th.Pal()
	step, size := theme.Px(profPipW+profPipGap), theme.Px(profPipW)
	y := theme.Px(profCardPipsH)/2 - size/2
	drawProfPips(c.levelImg, 0, y, level, c.pipMax, step, size, c.pipLocked, pal)
	lv := profLevelText(ProfFamily{Level: level, MaxLevel: c.pipMax})
	x := profPipRowW(c.pipMax, step, size) + theme.Px(theme.SpMd)
	theme.DrawCardText(c.levelImg, lv, c.th.Face(theme.StepTiny), x, 0, pal.Dim)
	if gate != "" {
		x += theme.Px(int(render.Measure(lv, c.th.TS().Tiny))) + theme.Px(theme.SpMd)
		theme.DrawCardText(c.levelImg, "· "+gate, c.th.Face(theme.StepTiny), x, 0, pal.Dim)
	}
}
