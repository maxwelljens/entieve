package screens

import (
	"image"
	"image/color"
	"strings"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// The proficiency family card: one single-instance dialog opened by a node
// click on the tree (CONTEXT.md "Proficiency family card"). It names the
// family, the pilot's level in it, what the family is for and which acts
// pay it practice. The prose is content (content.FamilyInfo), the level is
// the live read (ContentView.Proficiency), so retargeting on another node
// is a handful of label writes.
const (
	profCardW = 470
	profCardH = 270

	// profCardPipsH is the level line's height: the pips plus the tiny
	// face's line box. The line carries the tree node's own three level
	// reads (the pips, the "2/5" readout and the sim's gate sentence), so
	// the card and the node it was opened from cannot word a level
	// differently: the pips come from drawProfPips, the readout from
	// profLevelText, the gate from the sim.
	profCardPipsH = 20
)

// ProfInfoWin is the family card. It holds the family id it was last
// pointed at; a new Show swaps the page in place and the Kit raises the
// one window instead of mounting a second.
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

	// The level line's last spec (the tests compare it with the node's).
	pipLevel, pipMax int
	pipLocked        bool
}

// NewProfInfoWin builds the closed family card over the shared environment.
// Mount it through the Kit (the tree's node click emits ShowProfFamily).
func NewProfInfoWin(env widgets.Seams) *ProfInfoWin {
	p := env.Pal()
	c := &ProfInfoWin{th: env}

	maxW := float64(theme.Px(profCardW - 2*theme.ContentsPad))
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

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.WindowBodyBox(p, theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200))),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, false, false, false, true, false}),
			widget.GridLayoutOpts.Spacing(0, theme.Px(theme.SpSm)),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	c.desc = wrapped(env.Face(theme.StepSmall), p.Text)
	c.practice = wrapped(env.Face(theme.StepTiny), p.Dim)
	hint := widget.NewText(
		widget.TextOpts.Text("Esc or × closes. Click another family to switch.", env.Face(theme.StepTiny), theme.Solid(p.Head)),
		widget.TextOpts.MaxWidth(maxW),
	)
	body.AddChild(c.level, head("WHAT IT IS FOR"), c.desc, head("PRACTICE"), c.practice, hint)

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
	theme.ShowWidget(c.win.GetContainer().GetWidget(), true)
}

// paintLevel draws the card's level line the way the tree draws its node:
// the pips (filled to the level, dim on a shut branch), the level readout
// beside them ("2/5", the node's own words) and the sim's gate sentence.
// One line carries all three, so the card keeps its fixed height (the
// words it replaces took a line of their own).
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
