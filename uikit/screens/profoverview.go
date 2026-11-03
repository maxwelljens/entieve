package screens

import (
	"fmt"
	"image"
	"image/color"
	"reflect"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// The proficiency overview: the tree's own floating window (CONTEXT.md
// "Proficiency overview", titled simply Proficiency). It replaces the old
// header printed on the layer — a title and one dim line on a busy graph
// read poorly — with a house window that says how far the pilot has come
// and lists every proficiency's level at a glance. It opens and closes with the
// layer; the game keeps the pair in step, and its × closes only itself
// until the tree is reopened.
//
// Growth seam: the filter row the owner wants later (show only some
// categories) lands between the totals block and the SKILLS list. The
// painting below reserves that slot (profOvFilterRow) and draws nothing in
// it today; adding the control means filling that strip, not re-laying the
// window.
const (
	profOvW = 386
	// profOvH is the painted body's height: the totals, the bar and the two
	// skill columns, with no hint line under them (the tree says its own
	// controls; the owner had the line removed).
	profOvH    = 182
	profOvRows = 7 // skill rows per column (two columns fill today's 14 proficiencies)

	profOvLine = 13 // one skill row's height
	profOvCols = 2
	profOvBarH = 6

	// profOvFilterRow is the empty band a filter control will occupy: the
	// seam is here, no filter ships yet.
	profOvFilterRow = 0

	// profOvY is the window's landing y in design units: just under the
	// Selected Item window's default rect, so opening the tree does not
	// cover the verbs of the current selection. That rect is the top-left
	// corner (8+RailInset, 8) at its 464×260 floor (uikit's selectedPanel:
	// MinSize 464×240 plus the measured body), i.e. 268 tall, and the gap
	// is one SpMd.
	//
	// shortcut: the number is the Selected Item's composed default, not a
	// live read (the tree has no handle on the kit's windows); a test pins
	// the two rects apart, so a moved Selected Item default fails loudly.
	profOvY = 268 + theme.SpMd
)

// ProficiencyOverviewRect is the window's landing rect in design units:
// below the Selected Item window, in the left column clear of the rail
// gutter. The Screen row shares this rect.
func ProficiencyOverviewRect() image.Rectangle {
	return image.Rect(10, profOvY, 10+profOvW, profOvY+profOvH+theme.TitleBarH)
}

// profTotals is the overview's arithmetic: how many proficiencies are open,
// mastered at the cap, and how many levels of the possible total are
// earned.
type profTotals struct {
	Families, Open   int
	Levels           int
	MaxLevels, Maxed int
}

// proficiencyTotals computes the overview's numbers from the model (pure;
// the header used to print the same sums inline).
func proficiencyTotals(fams []ProfFamily) profTotals {
	var t profTotals
	t.Families = len(fams)
	for _, f := range fams {
		if f.Meets {
			t.Open++
		}
		t.Levels += f.Level
		t.MaxLevels += f.MaxLevel
		if f.MaxLevel > 0 && f.Level >= f.MaxLevel {
			t.Maxed++
		}
	}
	return t
}

// ProficiencyOverview is the summary window: proficiencies open, levels
// earned against the possible total over a progress bar, and the proficiency
// list with each level. It paints into its own canvas (the layer's idiom) and
// repaints only when the model moves.
type ProficiencyOverview struct {
	th    theme.Theme
	model func() []ProfFamily

	win  *widget.Window
	gfx  *widget.Graphic
	img  *ebiten.Image
	rect image.Rectangle

	drawn bool
	shown []ProfFamily
}

// NewProficiencyOverview builds the summary window over the pilot's model
// (nil reads as no proficiencies). Mount it with Kit.AddWindow and call Sync
// every frame.
func NewProficiencyOverview(env widgets.Seams, model func() []ProfFamily) *ProficiencyOverview {
	o := &ProficiencyOverview{th: env, model: model}
	o.rect = pxRect(image.Rect(0, 0, profOvW, profOvH))
	o.img = ebiten.NewImage(o.rect.Dx(), o.rect.Dy())
	o.gfx = widget.NewGraphic(widget.GraphicOpts.Image(o.img),
		widget.GraphicOpts.WidgetOpts(widget.WidgetOpts.MinSize(o.rect.Dx(), o.rect.Dy())))
	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.WindowBodyBox(env.Pal(), theme.WindowBody(env.Pal().BG), theme.NRGBA(env.Pal().Border, 200))),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	body.AddChild(o.gfx)
	bar, _ := theme.NewTitleBar(env, "Proficiency", "stat_tracking")
	o.win = widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(bar, theme.Px(theme.TitleBarH)),
		widget.WindowOpts.MinSize(theme.Px(profOvW+2*theme.ContentsPad), theme.Px(profOvH+2*theme.ContentsPad)),
		widget.WindowOpts.Location(pxRect(ProficiencyOverviewRect())),
	)
	return o
}

// Win returns the overview window for the Screen host.
func (o *ProficiencyOverview) Win() *widget.Window { return o.win }

// Canvas is the painted summary (tests read it).
func (o *ProficiencyOverview) Canvas() *ebiten.Image { return o.img }

// Sync repaints when the model changed (a level-up, a gate opening).
func (o *ProficiencyOverview) Sync() {
	fams := []ProfFamily(nil)
	if o.model != nil {
		fams = o.model()
	}
	if o.drawn && reflect.DeepEqual(fams, o.shown) {
		return
	}
	o.shown = fams
	o.drawn = true
	o.draw(fams)
}

// draw paints the summary: the totals, the progress bar, the (empty) filter
// band and the skill list in two columns.
func (o *ProficiencyOverview) draw(fams []ProfFamily) {
	pal := o.th.Pal()
	o.img.Clear()
	// The canvas is transparent; the window body behind it is the surface.
	pad := theme.Px(theme.ContentsPad)
	// The list rhythm: the small face's line box is 21 px and the tiny
	// face's 19, so the two totals lines take 21 and 21, then the bar; the
	// family rows are a tight 15 px pitch.
	const rowH = 15
	t := proficiencyTotals(fams)
	y := 2

	o.text(fmt.Sprintf("%d of %d proficiencies open", t.Open, t.Families),
		o.th.FaceBold(theme.StepSmall), pad, y, pal.Text)
	y += 21
	o.text(fmt.Sprintf("%d of %d levels earned · %d at the cap", t.Levels, t.MaxLevels, t.Maxed),
		o.th.Face(theme.StepTiny), pad, y, pal.Dim)
	y += 21

	// The progress bar: the track in the border role, the fill in accent.
	w := o.rect.Dx() - 2*pad
	track := image.Rect(pad, y, pad+w, y+theme.Px(profOvBarH))
	vector.DrawFilledRect(o.img, float32(track.Min.X), float32(track.Min.Y),
		float32(track.Dx()), float32(track.Dy()), theme.NRGBA(pal.Border, 90), false)
	if t.MaxLevels > 0 {
		fill := track
		fill.Max.X = track.Min.X + w*t.Levels/t.MaxLevels
		vector.DrawFilledRect(o.img, float32(fill.Min.X), float32(fill.Min.Y),
			float32(fill.Dx()), float32(fill.Dy()), theme.Solid(pal.Accent), false)
	}
	y += theme.Px(profOvBarH) + theme.Px(6)
	// The filter row seam: profOvFilterRow tall, empty today.
	y += theme.Px(profOvFilterRow)

	o.text("SKILLS", o.th.Face(theme.StepTiny), pad, y, pal.Head)
	y += 17

	// Two columns: name left, level right-aligned on the column's edge. A
	// gated family is dim; one at the cap reads in the accent.
	colW := (w - theme.Px(theme.SpMd)) / profOvCols
	for i, f := range fams {
		col, row := i/profOvRows, i%profOvRows
		if col >= profOvCols {
			break // shortcut: more than 14 families needs a taller canvas
		}
		x := pad + col*(colW+theme.Px(theme.SpMd))
		ry := y + row*rowH
		name, level := pal.Text, pal.Dim
		switch {
		case f.MaxLevel > 0 && f.Level >= f.MaxLevel:
			name, level = pal.Accent, pal.Accent
		case !f.Meets:
			name, level = pal.Dim, pal.Dim
		}
		o.text(f.Name, o.th.Face(theme.StepTiny), x, ry, name)
		lv := profLevelText(f)
		o.text(lv, o.th.Face(theme.StepTiny),
			x+colW-theme.Px(int(render.Measure(lv, o.th.TS().Tiny))), ry, level)
	}
}

// text draws one line on the summary canvas.
func (o *ProficiencyOverview) text(s string, face *text.Face, x, y int, col color.Color) {
	theme.DrawCardText(o.img, s, face, x, y, col)
}
