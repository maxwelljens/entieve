package screens

import (
	"image"
	"image/color"
	"strings"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
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
)

// ProfInfoWin is the family card. It holds the family id it was last
// pointed at; a new Show swaps the page in place and the Kit raises the
// one window instead of mounting a second.
type ProfInfoWin struct {
	th theme.Theme

	dialogShell
	title *theme.ChromeText

	family   string
	status   *widget.Text
	desc     *widget.Text
	practice *widget.Text
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

	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.WindowBodyBox(p, theme.WindowBody(p.BG), theme.NRGBA(p.Border, 200))),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{false, false, false, false, true, false}),
			widget.GridLayoutOpts.Spacing(0, theme.Px(theme.SpSm)),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	c.status = wrapped(env.Face(theme.StepSmall), p.Text)
	c.desc = wrapped(env.Face(theme.StepSmall), p.Text)
	c.practice = wrapped(env.Face(theme.StepTiny), p.Dim)
	hint := widget.NewText(
		widget.TextOpts.Text("Esc or × closes. Click another family to switch.", env.Face(theme.StepTiny), theme.Solid(p.Head)),
		widget.TextOpts.MaxWidth(maxW),
	)
	body.AddChild(c.status, head("WHAT IT IS FOR"), c.desc, head("PRACTICE"), c.practice, hint)

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

// Show points the card at one family and reveals it. level and gate are the
// tree node's own readouts, so the card can never disagree with the node it
// was opened from.
func (c *ProfInfoWin) Show(family string, level int, gate string) {
	card := content.FamilyInfo(family)
	c.family = family
	c.title.Label = content.FamilyLabel(family)
	c.status.Label = profCardStatus(level, gate)
	c.desc.Label = card.Desc
	lines := make([]string, 0, len(card.Practice))
	for _, line := range card.Practice {
		lines = append(lines, "· "+line)
	}
	c.practice.Label = strings.Join(lines, "\n")
	theme.ShowWidget(c.win.GetContainer().GetWidget(), true)
}

// profCardStatus is the card's first line: the node's level, then the node's
// unmet-prerequisite text ("Level II of V · needs Autocannons II").
func profCardStatus(level int, gate string) string {
	line := "Untrained (0 of " + content.LevelLabel(content.ProficiencyMax) + ")"
	if level > 0 {
		line = "Level " + content.LevelLabel(level) + " of " + content.LevelLabel(content.ProficiencyMax)
	}
	if gate != "" {
		line += " · " + gate
	}
	return line
}
